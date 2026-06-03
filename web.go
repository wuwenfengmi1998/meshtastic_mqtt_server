package main

import (
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newHTTPServer(cfg webConfig, store *store) *http.Server {
	return &http.Server{
		Addr:    net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler: newRouter(cfg, store),
	}
}

func newRouter(cfg webConfig, store *store) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	registerAPIRoutes(r.Group("/api"), store)
	registerStaticRoutes(r, cfg.StaticDir)
	return r
}

func registerAPIRoutes(r gin.IRouter, store *store) {
	r.GET("/health", func(c *gin.Context) {
		status := gin.H{"status": "ok", "database": "ok"}
		if err := store.Ping(); err != nil {
			status["status"] = "error"
			status["database"] = err.Error()
			c.JSON(http.StatusServiceUnavailable, status)
			return
		}
		c.JSON(http.StatusOK, status)
	})

	r.GET("/nodes", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListNodes(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, nodeDTO)
			return
		}
		total, err := store.CountNodes(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, nodeDTO)
	})
	r.GET("/nodes/:id", func(c *gin.Context) {
		row, err := store.GetNode(c.Param("id"))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, nodeDTO(*row))
	})
	r.GET("/text-messages", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListTextMessages(opts)
		writeListResponse(c, rows, opts, err, textMessageDTO)
	})
	r.GET("/positions", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListPositions(opts)
		writeListResponse(c, rows, opts, err, positionDTO)
	})
	r.GET("/telemetry", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListTelemetry(opts)
		writeListResponse(c, rows, opts, err, telemetryDTO)
	})
	r.GET("/routing", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListRouting(opts)
		writeListResponse(c, rows, opts, err, routingDTO)
	})
	r.GET("/traceroute", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListTraceroute(opts)
		writeListResponse(c, rows, opts, err, tracerouteDTO)
	})
}

func registerStaticRoutes(r *gin.Engine, staticDir string) {
	assetsDir := filepath.Join(staticDir, "assets")
	if info, err := os.Stat(assetsDir); err == nil && info.IsDir() {
		r.Static("/assets", assetsDir)
	}
	r.GET("/", func(c *gin.Context) {
		serveIndex(c, staticDir)
	})
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if filepath.Ext(c.Request.URL.Path) != "" {
			c.Status(http.StatusNotFound)
			return
		}
		serveIndex(c, staticDir)
	})
}

func serveIndex(c *gin.Context, staticDir string) {
	indexPath := filepath.Join(staticDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		c.String(http.StatusNotFound, "frontend dist not found: run npm run build in meshmap_frontend")
		return
	}
	c.File(indexPath)
}

func parseListOptions(c *gin.Context) (listOptions, bool) {
	limit, ok := parseIntQuery(c, "limit", 100)
	if !ok {
		return listOptions{}, false
	}
	offset, ok := parseIntQuery(c, "offset", 0)
	if !ok {
		return listOptions{}, false
	}
	nodeID := c.Query("node_id")
	if nodeID == "" {
		nodeID = c.Query("from")
	}
	var since, until *time.Time
	if value := c.Query("since"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since: use RFC3339"})
			return listOptions{}, false
		}
		since = &parsed
	}
	if value := c.Query("until"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid until: use RFC3339"})
			return listOptions{}, false
		}
		until = &parsed
	}
	return normalizeListOptions(listOptions{Limit: limit, Offset: offset, NodeID: nodeID, Since: since, Until: until}), true
}

func parseIntQuery(c *gin.Context, name string, defaultValue int) (int, bool) {
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

func writeListResponse[T any](c *gin.Context, rows []T, opts listOptions, err error, convert func(T) gin.H) {
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

func writeListResponseWithTotal[T any](c *gin.Context, rows []T, opts listOptions, total int64, err error, convert func(T) gin.H) {
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

func nodeDTO(row nodeInfoMapRecord) gin.H {
	return gin.H{"node_id": row.NodeID, "node_num": row.NodeNum, "latest_type": row.LatestType, "long_name": ptrString(row.LongName), "short_name": ptrString(row.ShortName), "hw_model": ptrString(row.HWModel), "role": ptrString(row.Role), "firmware_version": ptrString(row.FirmwareVersion), "latitude": ptrFloat64(row.Latitude), "longitude": ptrFloat64(row.Longitude), "altitude": ptrInt64(row.Altitude), "position_precision": ptrInt64(row.PositionPrecision), "num_online_local_nodes": ptrInt64(row.NumOnlineLocalNodes), "updated_at": row.UpdatedAt, "content_json": row.ContentJSON}
}

func textMessageDTO(row textMessageRecord) gin.H {
	return gin.H{"id": row.ID, "from_id": row.FromID, "from_num": row.FromNum, "text": ptrString(row.Text), "topic": row.Topic, "created_at": row.CreatedAt, "mqtt_remote_host": ptrString(row.MQTTRemoteHost), "content_json": row.ContentJSON}
}

func positionDTO(row positionRecord) gin.H {
	return gin.H{"id": row.ID, "from_id": row.FromID, "from_num": row.FromNum, "latitude": ptrFloat64(row.Latitude), "longitude": ptrFloat64(row.Longitude), "altitude": ptrInt64(row.Altitude), "created_at": row.CreatedAt, "content_json": row.ContentJSON}
}

func telemetryDTO(row telemetryRecord) gin.H {
	return gin.H{"id": row.ID, "from_id": row.FromID, "from_num": row.FromNum, "telemetry_type": ptrString(row.TelemetryType), "metrics_json": ptrString(row.MetricsJSON), "created_at": row.CreatedAt, "content_json": row.ContentJSON}
}

func routingDTO(row routingRecord) gin.H {
	return appendPacketDTO(row.ID, row.FromID, row.FromNum, row.PacketID, row.Portnum, row.CreatedAt, row.ContentJSON)
}

func tracerouteDTO(row tracerouteRecord) gin.H {
	return appendPacketDTO(row.ID, row.FromID, row.FromNum, row.PacketID, row.Portnum, row.CreatedAt, row.ContentJSON)
}

func appendPacketDTO(id uint64, fromID string, fromNum int64, packetID *int64, portnum *string, createdAt time.Time, contentJSON string) gin.H {
	return gin.H{"id": id, "from_id": fromID, "from_num": fromNum, "packet_id": ptrInt64(packetID), "portnum": ptrString(portnum), "created_at": createdAt, "content_json": contentJSON}
}

func ptrString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func ptrInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func ptrFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
