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

func newHTTPServer(cfg webConfig, store *store, sessions *sessionManager, mqttStatus mqttStatusProvider, blocking *blockingCache, forwarder mqttForwardReloader, settings *runtimeSettingsCache, botSender botTextSender) *http.Server {
	return &http.Server{
		Addr:    net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler: newRouter(cfg, store, sessions, mqttStatus, blocking, forwarder, settings, botSender),
	}
}

func serveHTTPUnixSocket(server *http.Server, socketPath string) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return err
	}
	if info, err := os.Stat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("web socket path exists and is not a socket")
		}
		if err := os.Remove(socketPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer os.Remove(socketPath)
	if err := os.Chmod(socketPath, 0660); err != nil {
		listener.Close()
		return err
	}
	return server.Serve(listener)
}

func newRouter(cfg webConfig, store *store, sessions *sessionManager, mqttStatus mqttStatusProvider, blocking *blockingCache, forwarder mqttForwardReloader, settings *runtimeSettingsCache, botSender botTextSender) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	api := r.Group("/api")
	registerAPIRoutes(api, store, cfg.MapTileCacheDir)
	registerAdminRoutes(api.Group("/admin"), store, sessions, mqttStatus, blocking, forwarder, settings, botSender)
	registerStaticRoutes(r, cfg.StaticDir)
	return r
}

func registerAPIRoutes(r gin.IRouter, store *store, mapTileCacheDir string) {
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

	registerNodeInfoRoutes(r, store, "/nodeinfo")
	registerNodeInfoRoutes(r, store, "/nodes")
	registerMapReportRoutes(r, store)
	registerMapSourceRoutes(r, store)
	registerMapTileProxyRoutes(r, store, mapTileCacheDir)
	registerHelpRoutes(r, store)
	r.GET("/text-messages", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListTextMessages(opts)
		writeListResponse(c, rows, opts, err, textMessageDTO)
	})
	r.GET("/discard-details", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListDiscardDetails(opts)
		writeListResponse(c, rows, opts, err, discardDetailsDTO)
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

func registerAdminRoutes(r gin.IRouter, store *store, sessions *sessionManager, mqttStatus mqttStatusProvider, blocking *blockingCache, forwarder mqttForwardReloader, settings *runtimeSettingsCache, botSender botTextSender) {
	type loginRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	type createUserRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	type updatePasswordRequest struct {
		Password string `json:"password"`
	}
	userDTO := func(user userRecord) gin.H {
		return gin.H{"id": user.ID, "username": user.Username, "role": user.Role, "created_at": user.CreatedAt, "updated_at": user.UpdatedAt}
	}
	loginLogDTO := func(row loginLogRecord) gin.H {
		return gin.H{"id": row.ID, "username": row.Username, "user_id": ptrUint64(row.UserID), "success": row.Success, "reason": row.Reason, "remote_addr": row.RemoteAddr, "remote_host": row.RemoteHost, "user_agent": row.UserAgent, "created_at": row.CreatedAt}
	}
	remoteInfo := func(c *gin.Context) (string, string) {
		remoteAddr := c.Request.RemoteAddr
		remoteHost, _, err := net.SplitHostPort(remoteAddr)
		if err != nil || remoteHost == "" {
			remoteHost = remoteAddr
		}
		return remoteAddr, remoteHost
	}
	recordLogin := func(c *gin.Context, username string, userID *uint64, success bool, reason string) {
		remoteAddr, remoteHost := remoteInfo(c)
		_ = store.InsertLoginLog(loginLogRecord{Username: username, UserID: userID, Success: success, Reason: reason, RemoteAddr: remoteAddr, RemoteHost: remoteHost, UserAgent: c.GetHeader("User-Agent")})
	}

	r.POST("/login", func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			recordLogin(c, "", nil, false, "invalid request")
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login request"})
			return
		}
		user, err := store.GetUserByUsername(req.Username)
		if err != nil || user.Role != adminRole || !verifyPassword(user.PasswordHash, req.Password) {
			recordLogin(c, req.Username, nil, false, "invalid username or password")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}
		cookie, err := sessions.newCookie(*user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		recordLogin(c, req.Username, &user.ID, true, "success")
		http.SetCookie(c.Writer, cookie)
		c.JSON(http.StatusOK, gin.H{"user": adminUserResponse(*user)})
	})
	r.POST("/logout", func(c *gin.Context) {
		http.SetCookie(c.Writer, sessions.clearCookie())
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	protected := r.Group("")
	protected.Use(requireAdmin(sessions))
	registerAdminBlockingRoutes(protected, store, blocking)
	registerAdminMQTTForwardRoutes(protected, store, forwarder)
	registerAdminRuntimeSettingsRoutes(protected, store, settings)
	registerAdminMapSourceRoutes(protected, store)
	registerAdminHelpRoutes(protected, store)
	registerAdminBotRoutes(protected, store, botSender)
	protected.GET("/me", func(c *gin.Context) {
		claims := c.MustGet("admin_claims").(*sessionClaims)
		c.JSON(http.StatusOK, gin.H{"user": adminUserDTO{Username: claims.Username, Role: claims.Role}})
	})
	protected.GET("/mqtt/status", func(c *gin.Context) {
		if mqttStatus == nil {
			c.JSON(http.StatusOK, adminMqttStatus{Running: false})
			return
		}
		status := mqttStatus.Status()
		discardCount, err := store.CountDiscardDetails(listOptions{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		status.MessagesDropped = discardCount
		c.JSON(http.StatusOK, status)
	})
	protected.GET("/users", func(c *gin.Context) {
		users, err := store.ListUsers()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items := make([]gin.H, 0, len(users))
		for _, user := range users {
			items = append(items, userDTO(user))
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	})
	protected.POST("/users", func(c *gin.Context) {
		var req createUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid create user request"})
			return
		}
		user, err := store.CreateAdminUser(req.Username, req.Password)
		if errors.Is(err, errUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "username already exists"})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"user": userDTO(*user)})
	})
	protected.PUT("/users/:id/password", func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}
		var req updatePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid password request"})
			return
		}
		user, err := store.UpdateUserPassword(id, req.Password)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user": userDTO(*user)})
	})
	protected.GET("/log/login", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListLoginLogs(opts)
		writeListResponse(c, rows, opts, err, loginLogDTO)
	})
	protected.DELETE("/text-messages/:id", func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
			return
		}
		if err := store.DeleteTextMessage(id); errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	protected.DELETE("/nodes/:id", func(c *gin.Context) {
		nodeID := c.Param("id")
		if nodeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
			return
		}
		if err := store.DeleteNode(nodeID); errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

func registerNodeInfoRoutes(r gin.IRouter, store *store, path string) {
	r.GET(path, func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListNodeInfo(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, nodeInfoDTO)
			return
		}
		total, err := store.CountNodeInfo(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, nodeInfoDTO)
	})
	r.GET(path+"/:id", func(c *gin.Context) {
		row, err := store.GetNodeInfo(c.Param("id"))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "nodeinfo not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, nodeInfoDTO(*row))
	})
}

func registerMapReportRoutes(r gin.IRouter, store *store) {
	r.GET("/map-reports/viewport", func(c *gin.Context) {
		opts, ok := parseMapReportViewportOptions(c)
		if !ok {
			return
		}
		result, err := store.ListMapReportViewport(opts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items := make([]gin.H, 0, len(result.Points)+len(result.Clusters))
		if result.Mode == "points" {
			for _, row := range result.Points {
				items = append(items, mapReportViewportPointDTO(row))
			}
		} else {
			for _, row := range result.Clusters {
				items = append(items, mapReportClusterDTO(row))
			}
		}
		c.JSON(http.StatusOK, gin.H{"mode": result.Mode, "items": items, "total": result.Total, "limit": result.Limit, "zoom": result.Zoom})
	})
	r.GET("/map-reports", func(c *gin.Context) {
		opts, ok := parseMapReportListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListMapReports(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, mapReportDTO)
			return
		}
		total, err := store.CountMapReports(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, mapReportDTO)
	})
	r.GET("/map-reports/:id", func(c *gin.Context) {
		row, err := store.GetMapReport(c.Param("id"))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "map report not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, mapReportDTO(*row))
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
	channelID := c.Query("channel_id")
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
	return normalizeListOptions(listOptions{Limit: limit, Offset: offset, NodeID: nodeID, ChannelID: channelID, Since: since, Until: until}), true
}

func parseMapReportListOptions(c *gin.Context) (listOptions, bool) {
	opts, ok := parseListOptions(c)
	if !ok {
		return listOptions{}, false
	}
	minLat, hasMinLat, ok := parseOptionalFloatQuery(c, "min_lat")
	if !ok {
		return listOptions{}, false
	}
	maxLat, hasMaxLat, ok := parseOptionalFloatQuery(c, "max_lat")
	if !ok {
		return listOptions{}, false
	}
	minLng, hasMinLng, ok := parseOptionalFloatQuery(c, "min_lng")
	if !ok {
		return listOptions{}, false
	}
	maxLng, hasMaxLng, ok := parseOptionalFloatQuery(c, "max_lng")
	if !ok {
		return listOptions{}, false
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
		return listOptions{}, false
	}
	if minLat < -90 || minLat > 90 || maxLat < -90 || maxLat > 90 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "latitude bounds must be between -90 and 90"})
		return listOptions{}, false
	}
	if minLat > maxLat {
		c.JSON(http.StatusBadRequest, gin.H{"error": "min_lat must be <= max_lat"})
		return listOptions{}, false
	}
	if minLng < -180 || minLng > 180 || maxLng < -180 || maxLng > 180 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "longitude bounds must be between -180 and 180"})
		return listOptions{}, false
	}
	opts.MinLat = &minLat
	opts.MaxLat = &maxLat
	opts.MinLng = &minLng
	opts.MaxLng = &maxLng
	return opts, true
}

func parseMapReportViewportOptions(c *gin.Context) (mapReportViewportOptions, bool) {
	opts, ok := parseMapReportListOptions(c)
	if !ok {
		return mapReportViewportOptions{}, false
	}
	if opts.MinLat == nil || opts.MaxLat == nil || opts.MinLng == nil || opts.MaxLng == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "viewport bounds are required"})
		return mapReportViewportOptions{}, false
	}
	zoom, ok := parseIntQuery(c, "zoom", 0)
	if !ok {
		return mapReportViewportOptions{}, false
	}
	if zoom < 0 || zoom > 24 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "zoom must be between 0 and 24"})
		return mapReportViewportOptions{}, false
	}
	limit, ok := parseIntQuery(c, "limit", 1000)
	if !ok {
		return mapReportViewportOptions{}, false
	}
	clusterThreshold, ok := parseIntQuery(c, "cluster_threshold", 500)
	if !ok {
		return mapReportViewportOptions{}, false
	}
	targetCells, ok := parseIntQuery(c, "target_cells", 64)
	if !ok {
		return mapReportViewportOptions{}, false
	}
	if limit <= 0 || clusterThreshold <= 0 || targetCells <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit, cluster_threshold, and target_cells must be positive"})
		return mapReportViewportOptions{}, false
	}
	return normalizeMapReportViewportOptions(mapReportViewportOptions{ListOptions: opts, Zoom: zoom, Limit: limit, ClusterThreshold: clusterThreshold, TargetCells: targetCells}), true
}

func parseOptionalFloatQuery(c *gin.Context, name string) (float64, bool, bool) {
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

func nodeInfoDTO(row nodeInfoRecord) gin.H {
	return gin.H{"node_id": row.NodeID, "node_num": row.NodeNum, "user_id": ptrString(row.UserID), "long_name": ptrString(row.LongName), "short_name": ptrString(row.ShortName), "hw_model": ptrString(row.HWModel), "role": ptrString(row.Role), "is_licensed": ptrBool(row.IsLicensed), "public_key": ptrString(row.PublicKey), "updated_at": row.UpdatedAt, "content_json": row.ContentJSON}
}

func mapReportDTO(row mapReportRecord) gin.H {
	return gin.H{"node_id": row.NodeID, "node_num": row.NodeNum, "long_name": ptrString(row.LongName), "short_name": ptrString(row.ShortName), "hw_model": ptrString(row.HWModel), "role": ptrString(row.Role), "firmware_version": ptrString(row.FirmwareVersion), "region": ptrString(row.Region), "modem_preset": ptrString(row.ModemPreset), "latitude": ptrFloat64(row.Latitude), "longitude": ptrFloat64(row.Longitude), "altitude": ptrInt64(row.Altitude), "position_precision": ptrInt64(row.PositionPrecision), "num_online_local_nodes": ptrInt64(row.NumOnlineLocalNodes), "has_opted_report_location": ptrBool(row.HasOptedReportLocation), "updated_at": row.UpdatedAt, "content_json": row.ContentJSON}
}

func mapReportViewportPointDTO(row mapReportRecord) gin.H {
	item := mapReportDTO(row)
	item["type"] = "point"
	return item
}

func mapReportClusterDTO(row mapReportClusterRecord) gin.H {
	return gin.H{"type": "cluster", "cluster_id": row.ClusterID, "latitude": row.Latitude, "longitude": row.Longitude, "count": row.Count}
}

func textMessageDTO(row textMessageRecord) gin.H {
	return gin.H{"id": row.ID, "from_id": row.FromID, "from_num": row.FromNum, "packet_id": ptrInt64(row.PacketID), "text": ptrString(row.Text), "topic": row.Topic, "channel_id": ptrString(row.ChannelID), "created_at": row.CreatedAt, "mqtt_remote_host": ptrString(row.MQTTRemoteHost), "content_json": row.ContentJSON}
}

func discardDetailsDTO(row discardDetailsRecord) gin.H {
	return gin.H{"id": row.ID, "topic": row.Topic, "error": row.Error, "payload_len": row.PayloadLen, "raw_base64": row.RawBase64, "mqtt_client_id": ptrString(row.MQTTClientID), "mqtt_username": ptrString(row.MQTTUsername), "mqtt_listener": ptrString(row.MQTTListener), "mqtt_remote_addr": ptrString(row.MQTTRemoteAddr), "mqtt_remote_host": ptrString(row.MQTTRemoteHost), "mqtt_remote_port": ptrString(row.MQTTRemotePort), "created_at": row.CreatedAt, "content_json": row.ContentJSON}
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

func ptrUint64(value *uint64) any {
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

func ptrBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}
