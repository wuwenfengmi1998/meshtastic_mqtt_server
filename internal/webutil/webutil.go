// Package webutil 收集 admin/路由层共享的 HTTP 解析与响应工具。
//
// 这些函数原本散落在 web.go 中（parseListOptions、writeListResponse、
// parseMapReportListOptions 等），任何注册 admin 路由的领域包都依赖它们。
// 把它们抽离出来可以避免 internal/web 同时被 internal/blocking、
// internal/bot 等包反向引用造成循环依赖。
package webutil

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"meshtastic_mqtt_server/internal/store"
)

// ParseListOptions 从请求中读取 limit / offset / since / until / node_id /
// channel_id 等通用过滤参数；解析失败时它会写入 400 响应并返回 false。
func ParseListOptions(c *gin.Context) (store.ListOptions, bool) {
	limit, ok := ParseIntQuery(c, "limit", 100)
	if !ok {
		return store.ListOptions{}, false
	}
	offset, ok := ParseIntQuery(c, "offset", 0)
	if !ok {
		return store.ListOptions{}, false
	}
	nodeID := c.Query("node_id")
	if nodeID == "" {
		nodeID = c.Query("from")
	}
	channelID := c.Query("channel_id")
	var since, until *time.Time
	if value := c.Query("since"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since: use RFC3339"})
			return store.ListOptions{}, false
		}
		since = &parsed
	}
	if value := c.Query("until"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid until: use RFC3339"})
			return store.ListOptions{}, false
		}
		until = &parsed
	}
	return store.NormalizeListOptions(store.ListOptions{Limit: limit, Offset: offset, NodeID: nodeID, ChannelID: channelID, Since: since, Until: until}), true
}

// ParseMapReportListOptions 在 ParseListOptions 的基础上解析 4 个地图边界。
func ParseMapReportListOptions(c *gin.Context) (store.ListOptions, bool) {
	opts, ok := ParseListOptions(c)
	if !ok {
		return store.ListOptions{}, false
	}
	minLat, hasMinLat, ok := ParseOptionalFloatQuery(c, "min_lat")
	if !ok {
		return store.ListOptions{}, false
	}
	maxLat, hasMaxLat, ok := ParseOptionalFloatQuery(c, "max_lat")
	if !ok {
		return store.ListOptions{}, false
	}
	minLng, hasMinLng, ok := ParseOptionalFloatQuery(c, "min_lng")
	if !ok {
		return store.ListOptions{}, false
	}
	maxLng, hasMaxLng, ok := ParseOptionalFloatQuery(c, "max_lng")
	if !ok {
		return store.ListOptions{}, false
	}
	boundsCount := 0
	for _, present := range []bool{hasMinLat, hasMaxLat, hasMinLng, hasMaxLng} {
		if present {
			boundsCount++
		}
	}
	if boundsCount == 0 {
		return opts, true
	}
	if boundsCount != 4 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "map bounds require min_lat, max_lat, min_lng, and max_lng"})
		return store.ListOptions{}, false
	}
	if minLat < -90 || minLat > 90 || maxLat < -90 || maxLat > 90 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "latitude bounds must be between -90 and 90"})
		return store.ListOptions{}, false
	}
	if minLat > maxLat {
		c.JSON(http.StatusBadRequest, gin.H{"error": "min_lat must be <= max_lat"})
		return store.ListOptions{}, false
	}
	if minLng < -180 || minLng > 180 || maxLng < -180 || maxLng > 180 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "longitude bounds must be between -180 and 180"})
		return store.ListOptions{}, false
	}
	opts.MinLat = &minLat
	opts.MaxLat = &maxLat
	opts.MinLng = &minLng
	opts.MaxLng = &maxLng
	return opts, true
}

// ParseMapReportViewportOptions 在 ParseMapReportListOptions 之上解析 zoom /
// cluster_threshold / target_cells 等额外字段。
func ParseMapReportViewportOptions(c *gin.Context) (store.MapReportViewportOptions, bool) {
	opts, ok := ParseMapReportListOptions(c)
	if !ok {
		return store.MapReportViewportOptions{}, false
	}
	if opts.MinLat == nil || opts.MaxLat == nil || opts.MinLng == nil || opts.MaxLng == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "viewport bounds are required"})
		return store.MapReportViewportOptions{}, false
	}
	zoom, ok := ParseIntQuery(c, "zoom", 0)
	if !ok {
		return store.MapReportViewportOptions{}, false
	}
	if zoom < 0 || zoom > 24 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "zoom must be between 0 and 24"})
		return store.MapReportViewportOptions{}, false
	}
	limit, ok := ParseIntQuery(c, "limit", 1000)
	if !ok {
		return store.MapReportViewportOptions{}, false
	}
	clusterThreshold, ok := ParseIntQuery(c, "cluster_threshold", 500)
	if !ok {
		return store.MapReportViewportOptions{}, false
	}
	targetCells, ok := ParseIntQuery(c, "target_cells", 64)
	if !ok {
		return store.MapReportViewportOptions{}, false
	}
	if limit <= 0 || clusterThreshold <= 0 || targetCells <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit, cluster_threshold, and target_cells must be positive"})
		return store.MapReportViewportOptions{}, false
	}
	return store.NormalizeMapReportViewportOptions(store.MapReportViewportOptions{ListOptions: opts, Zoom: zoom, Limit: limit, ClusterThreshold: clusterThreshold, TargetCells: targetCells}), true
}

// ParseIntQuery 从请求查询字符串解析整数；缺省时返回 defaultValue。
func ParseIntQuery(c *gin.Context, name string, defaultValue int) (int, bool) {
	value := c.Query(name)
	if value == "" {
		return defaultValue, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return 0, false
	}
	return parsed, true
}

// ParseOptionalFloatQuery 解析可选浮点查询参数；返回 (value, present, ok)。
func ParseOptionalFloatQuery(c *gin.Context, name string) (float64, bool, bool) {
	value := c.Query(name)
	if value == "" {
		return 0, false, true
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return 0, true, false
	}
	return parsed, true, true
}

// WriteListResponse 把 rows 通过 convert 转成 gin.H 后包装成 {items, limit, offset}。
func WriteListResponse[T any](c *gin.Context, rows []T, opts store.ListOptions, err error, convert func(T) gin.H) {
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, convert(row))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "limit": opts.Limit, "offset": opts.Offset})
}

// WriteListResponseWithTotal 在 WriteListResponse 基础上额外携带 total 字段。
func WriteListResponseWithTotal[T any](c *gin.Context, rows []T, opts store.ListOptions, total int64, err error, convert func(T) gin.H) {
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, convert(row))
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "limit": opts.Limit, "offset": opts.Offset, "total": total})
}

// PtrString / PtrInt64 / PtrUint64 / PtrFloat64 / PtrBool 把指针解引用成 any，
// 用于把数据库可空字段转换成 JSON 时让 nil 序列化为 null。
func PtrString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func PtrInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func PtrUint64(value *uint64) any {
	if value == nil {
		return nil
	}
	return *value
}

func PtrFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func PtrBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}
