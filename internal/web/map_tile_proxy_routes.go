package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	storepkg "meshtastic_mqtt_server/internal/store"
)

const (
	mapTileCacheControl = "public, max-age=86400"
	maxMapTileBytes     = 10 << 20
	maxMapTileRedirects = 2
)

type mapTileProxy struct {
	store    *storepkg.Store
	cacheDir string
	client   *http.Client
}

func registerMapTileProxyRoutes(r gin.IRouter, store *storepkg.Store, cacheDir string) {
	proxy := &mapTileProxy{
		store:    store,
		cacheDir: cacheDir,
		client:   newMapTileHTTPClient(),
	}
	r.GET("/map/:sourceHash", proxy.handle)
}

// newMapTileHTTPClient 构造带 SSRF 防护的 HTTP 客户端:
//   - DialContext 在连接时按解析后的 IP 拒绝内网/链路本地/元数据等地址,防 DNS rebinding;
//   - CheckRedirect 限制跳转次数,每次跳转同样经过 DialContext 检查。
func newMapTileHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if !isMapTileDialTargetAllowed(ctx, network, addr) {
				return nil, fmt.Errorf("blocked map tile dial target %q", addr)
			}
			return dialer.DialContext(ctx, network, addr)
		},
		MaxIdleConns:       8,
		IdleConnTimeout:    60 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxMapTileRedirects {
				return errors.New("too many map tile redirects")
			}
			return nil
		},
	}
}

// isMapTileDialTargetAllowed 检查拨号目标是否允许:
// 仅允许 tcp 协议,且解析出的每个 IP 均为公网地址。
func isMapTileDialTargetAllowed(ctx context.Context, network, addr string) bool {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return false
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return isPublicAddr(ip)
	}
	// 主机名:解析后要求所有结果均为公网地址,避免 DNS rebinding 落到内网。
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, r := range ips {
		addr, ok := netip.AddrFromSlice(r.IP)
		if !ok || !isPublicAddr(addr) {
			return false
		}
	}
	return true
}

// isPublicAddr 判定 IP 是否可安全外访;拒绝回环、私有、链路本地、CGNAT、
// 组播、未指定与 IPv6 ULA 地址。
func isPublicAddr(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	// CGNAT 100.64.0.0/10 不在 IsPrivate 范围内,单独拦截。
	if ip.Is4() {
		ipv4 := ip.As4()
		ipv4Int := uint32(ipv4[0])<<24 | uint32(ipv4[1])<<16 | uint32(ipv4[2])<<8 | uint32(ipv4[3])
		if ipv4Int >= 0x64400000 && ipv4Int <= 0x647fffff {
			return false
		}
	}
	return true
}

func (p *mapTileProxy) handle(c *gin.Context) {
	sourceHash := strings.ToLower(c.Param("sourceHash"))
	if !isMapTileSourceHash(sourceHash) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid map source hash"})
		return
	}

	row, err := p.store.GetEnabledMapTileSourceByHash(sourceHash)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "map source not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tile, ok := parseMapTileCoordinates(c, row.MaxZoom)
	if !ok {
		return
	}

	cachePath := mapTileCachePath(p.cacheDir, sourceHash, tile)
	if data, err := os.ReadFile(cachePath); err == nil {
		writeMapTile(c, data)
		return
	} else if !os.IsNotExist(err) {
		// Fall through to upstream fetch. A broken cache file should not prevent map rendering.
	}

	data, status, err := p.fetchRemoteTile(c.Request, row.URLTemplate, tile)
	if err != nil {
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	_ = writeMapTileCacheFile(cachePath, data)
	writeMapTile(c, data)
}

func (p *mapTileProxy) fetchRemoteTile(req *http.Request, template string, tile mapTileCoordinates) ([]byte, int, error) {
	remoteURL := expandMapTileURLTemplate(template, tile)
	upstreamReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("build upstream map tile request: %w", err)
	}
	upstreamReq.Header.Set("User-Agent", "mesh_mqtt_go map tile cache")

	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("fetch upstream map tile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, http.StatusNotFound, fmt.Errorf("upstream map tile not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, http.StatusBadGateway, fmt.Errorf("upstream map tile returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxMapTileBytes+1))
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("read upstream map tile: %w", err)
	}
	if len(data) > maxMapTileBytes {
		return nil, http.StatusBadGateway, fmt.Errorf("upstream map tile is too large")
	}
	return data, http.StatusOK, nil
}

type mapTileCoordinates struct {
	x int64
	y int64
	z int64
}

func parseMapTileCoordinates(c *gin.Context, maxZoom int) (mapTileCoordinates, bool) {
	x, ok := parseMapTileCoordinate(c, "x")
	if !ok {
		return mapTileCoordinates{}, false
	}
	y, ok := parseMapTileCoordinate(c, "y")
	if !ok {
		return mapTileCoordinates{}, false
	}
	z, ok := parseMapTileCoordinate(c, "z")
	if !ok {
		return mapTileCoordinates{}, false
	}
	if z > int64(maxZoom) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "map tile z exceeds max zoom"})
		return mapTileCoordinates{}, false
	}
	limit := int64(1) << z
	if x >= limit || y >= limit {
		c.JSON(http.StatusBadRequest, gin.H{"error": "map tile coordinates out of range"})
		return mapTileCoordinates{}, false
	}
	return mapTileCoordinates{x: x, y: y, z: z}, true
}

func parseMapTileCoordinate(c *gin.Context, name string) (int64, bool) {
	value := c.Query(name)
	if value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing map tile " + name})
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 || parsed > 30_000_000_000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid map tile " + name})
		return 0, false
	}
	return parsed, true
}

func isMapTileSourceHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func expandMapTileURLTemplate(template string, tile mapTileCoordinates) string {
	result := strings.ReplaceAll(template, "{x}", strconv.FormatInt(tile.x, 10))
	result = strings.ReplaceAll(result, "{y}", strconv.FormatInt(tile.y, 10))
	result = strings.ReplaceAll(result, "{z}", strconv.FormatInt(tile.z, 10))
	return result
}

func mapTileCachePath(cacheDir, sourceHash string, tile mapTileCoordinates) string {
	return filepath.Join(cacheDir, sourceHash, strconv.FormatInt(tile.z, 10), strconv.FormatInt(tile.x, 10), strconv.FormatInt(tile.y, 10)+".tile")
}

func writeMapTileCacheFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func writeMapTile(c *gin.Context, data []byte) {
	contentType := mapTileContentType(data)
	c.Header("Cache-Control", mapTileCacheControl)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, contentType, data)
}

// mapTileContentType 只放行 image/*,其余一律按二进制下发,
// 防止上游返回 HTML/脚本在本站同源渲染(存储型 XSS 面)。
func mapTileContentType(data []byte) string {
	detected := http.DetectContentType(data)
	if strings.HasPrefix(detected, "image/") {
		return detected
	}
	return "application/octet-stream"
}
