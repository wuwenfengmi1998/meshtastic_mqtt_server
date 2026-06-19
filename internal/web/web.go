package web

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

	"meshtastic_mqtt_server/internal/auth"
	blockingpkg "meshtastic_mqtt_server/internal/blocking"
	botpkg "meshtastic_mqtt_server/internal/bot"
	configpkg "meshtastic_mqtt_server/internal/config"
	helppkg "meshtastic_mqtt_server/internal/help"
	llmadminpkg "meshtastic_mqtt_server/internal/llmadmin"
	mappkg "meshtastic_mqtt_server/internal/mapsource"
	mqttforwardpkg "meshtastic_mqtt_server/internal/mqttforward"
	rspkg "meshtastic_mqtt_server/internal/runtimesettings"
	signpkg "meshtastic_mqtt_server/internal/sign"
	storepkg "meshtastic_mqtt_server/internal/store"
	"meshtastic_mqtt_server/internal/webutil"
)

func NewHTTPServer(cfg configpkg.WebConfig, consoleLog bool, store *storepkg.Store, sessions *auth.Manager, mqttStatus MQTTStatusProvider, blocking *blockingpkg.Cache, forwarder mqttforwardpkg.Reloader, settings *rspkg.Cache, botSender botpkg.TextSender) *http.Server {
	return &http.Server{
		Addr:    net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler: NewRouter(cfg, consoleLog, store, sessions, mqttStatus, blocking, forwarder, settings, botSender),
	}
}

func ServeUnixSocket(server *http.Server, socketPath string) error {
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

func NewRouter(cfg configpkg.WebConfig, consoleLog bool, store *storepkg.Store, sessions *auth.Manager, mqttStatus MQTTStatusProvider, blocking *blockingpkg.Cache, forwarder mqttforwardpkg.Reloader, settings *rspkg.Cache, botSender botpkg.TextSender) *gin.Engine {
	r := gin.New()
	if consoleLog {
		r.Use(gin.Logger(), gin.Recovery())
	} else {
		r.Use(gin.Recovery())
	}
	api := r.Group("/api")
	registerAPIRoutes(api, store, cfg.MapTileCacheDir)
	registerAdminRoutes(api.Group("/admin"), store, sessions, mqttStatus, blocking, forwarder, settings, botSender)
	registerStaticRoutes(r, cfg.StaticDir)
	return r
}

func registerAPIRoutes(r gin.IRouter, store *storepkg.Store, mapTileCacheDir string) {
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
	mappkg.RegisterPublicRoutes(r, store)
	registerMapTileProxyRoutes(r, store, mapTileCacheDir)
	helppkg.RegisterPublicRoutes(r, store)
	r.GET("/signs", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListSigns(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, signpkg.SignDTO)
			return
		}
		total, err := store.CountSigns(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, signpkg.SignDTO)
	})
	r.GET("/signs/daily", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.CountSignsByDay(opts)
		writeListResponse(c, rows, opts, err, signpkg.SignDayCountDTO)
	})
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

func registerAdminRoutes(r gin.IRouter, store *storepkg.Store, sessions *auth.Manager, mqttStatus MQTTStatusProvider, blocking *blockingpkg.Cache, forwarder mqttforwardpkg.Reloader, settings *rspkg.Cache, botSender botpkg.TextSender) {
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
	userDTO := func(user storepkg.UserRecord) gin.H {
		return gin.H{"id": user.ID, "username": user.Username, "role": user.Role, "created_at": user.CreatedAt, "updated_at": user.UpdatedAt}
	}
	loginLogDTO := func(row storepkg.LoginLogRecord) gin.H {
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
		_ = store.InsertLoginLog(storepkg.LoginLogRecord{Username: username, UserID: userID, Success: success, Reason: reason, RemoteAddr: remoteAddr, RemoteHost: remoteHost, UserAgent: c.GetHeader("User-Agent")})
	}

	r.POST("/login", func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			recordLogin(c, "", nil, false, "invalid request")
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login request"})
			return
		}
		user, err := store.GetUserByUsername(req.Username)
		if err != nil || user.Role != auth.AdminRole || !auth.VerifyPassword(user.PasswordHash, req.Password) {
			recordLogin(c, req.Username, nil, false, "invalid username or password")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}
		cookie, err := sessions.NewCookie(*user)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		recordLogin(c, req.Username, &user.ID, true, "success")
		http.SetCookie(c.Writer, cookie)
		c.JSON(http.StatusOK, gin.H{"user": auth.AdminUserResponse(*user)})
	})
	r.POST("/logout", func(c *gin.Context) {
		http.SetCookie(c.Writer, sessions.ClearCookie())
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	protected := r.Group("")
	protected.Use(auth.RequireAdmin(sessions))
	blockingpkg.RegisterRoutes(protected, store, blocking)
	signpkg.RegisterAdminRoutes(protected, store)
	mqttforwardpkg.RegisterRoutes(protected, store, forwarder)
	rspkg.RegisterRoutes(protected, store, settings)
	mappkg.RegisterAdminRoutes(protected, store)
	helppkg.RegisterAdminRoutes(protected, store)
	botpkg.RegisterRoutes(protected, store, botSender)
	llmadminpkg.RegisterRoutes(protected, store)
	protected.GET("/me", func(c *gin.Context) {
		claims := c.MustGet("admin_claims").(*auth.SessionClaims)
		c.JSON(http.StatusOK, gin.H{"user": auth.AdminUserDTO{Username: claims.Username, Role: claims.Role}})
	})
	protected.GET("/mqtt/status", func(c *gin.Context) {
		if mqttStatus == nil {
			c.JSON(http.StatusOK, AdminMQTTStatus{Running: false})
			return
		}
		status := mqttStatus.Status()
		discardCount, err := store.CountDiscardDetails(storepkg.ListOptions{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		status.MessagesDropped = discardCount
		c.JSON(http.StatusOK, status)
	})
	// 一键断开 MQTT 客户端并把它的远端 IP 加入屏蔽表。reason 由前端传入；写库前先查 IP 避免连接断开后查不到。
	protected.POST("/mqtt/clients/:client_id/disconnect-and-block", func(c *gin.Context) {
		if mqttStatus == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mqtt server not available"})
			return
		}
		clientID := c.Param("client_id")
		if clientID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid client id"})
			return
		}
		var req struct {
			Reason string `json:"reason"`
		}
		// 请求体允许为空——若没传 reason 就走默认占位。
		_ = c.ShouldBindJSON(&req)
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			reason = "manual disconnect from admin dashboard"
		}

		host, ok := mqttStatus.LookupClientRemoteHost(clientID)
		if !ok || host == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "mqtt client not found"})
			return
		}

		// 先写屏蔽规则，再断开连接：万一断开后客户端立刻重连，新规则已经生效。
		var ipRule *storepkg.IPBlockingRecord
		row, err := store.CreateIPBlocking(host, reason, true)
		switch {
		case err == nil:
			ipRule = row
		case errors.Is(err, storepkg.ErrBlockingAlreadyExists):
			// 已经存在则忽略——只确保是启用状态。
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if blocking != nil {
			if err := blocking.Reload(store); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		disconnected := mqttStatus.DisconnectClient(clientID)
		resp := gin.H{
			"status":       "ok",
			"client_id":    clientID,
			"ip_value":     host,
			"disconnected": disconnected,
		}
		if ipRule != nil {
			resp["ip_rule_id"] = ipRule.ID
			resp["ip_rule_created"] = true
		} else {
			resp["ip_rule_created"] = false
		}
		c.JSON(http.StatusOK, resp)
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
		if errors.Is(err, storepkg.ErrUserAlreadyExists) {
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
	// purge：除了 nodeinfo + map_report，还把 text_message 与 position/telemetry/routing/traceroute
	// 中按 from_id 关联到该节点的记录一并删除。
	protected.DELETE("/nodes/:id/purge", func(c *gin.Context) {
		nodeID := c.Param("id")
		if nodeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node id"})
			return
		}
		if err := store.PurgeNode(nodeID); errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

func registerNodeInfoRoutes(r gin.IRouter, store *storepkg.Store, path string) {
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

func registerMapReportRoutes(r gin.IRouter, store *storepkg.Store) {
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

func parseListOptions(c *gin.Context) (storepkg.ListOptions, bool) {
	return webutil.ParseListOptions(c)
}

func parseMapReportListOptions(c *gin.Context) (storepkg.ListOptions, bool) {
	return webutil.ParseMapReportListOptions(c)
}

func parseMapReportViewportOptions(c *gin.Context) (storepkg.MapReportViewportOptions, bool) {
	return webutil.ParseMapReportViewportOptions(c)
}

func parseIntQuery(c *gin.Context, name string, defaultValue int) (int, bool) {
	return webutil.ParseIntQuery(c, name, defaultValue)
}

func writeListResponse[T any](c *gin.Context, rows []T, opts storepkg.ListOptions, err error, convert func(T) gin.H) {
	webutil.WriteListResponse(c, rows, opts, err, convert)
}

func writeListResponseWithTotal[T any](c *gin.Context, rows []T, opts storepkg.ListOptions, total int64, err error, convert func(T) gin.H) {
	webutil.WriteListResponseWithTotal(c, rows, opts, total, err, convert)
}

func nodeInfoDTO(row storepkg.NodeInfoRecord) gin.H {
	return gin.H{"node_id": row.NodeID, "node_num": row.NodeNum, "user_id": ptrString(row.UserID), "long_name": ptrString(row.LongName), "short_name": ptrString(row.ShortName), "hw_model": ptrString(row.HWModel), "role": ptrString(row.Role), "is_licensed": ptrBool(row.IsLicensed), "public_key": ptrString(row.PublicKey), "updated_at": row.UpdatedAt, "content_json": row.ContentJSON}
}

func mapReportDTO(row storepkg.MapReportRecord) gin.H {
	return gin.H{"node_id": row.NodeID, "node_num": row.NodeNum, "long_name": ptrString(row.LongName), "short_name": ptrString(row.ShortName), "hw_model": ptrString(row.HWModel), "role": ptrString(row.Role), "firmware_version": ptrString(row.FirmwareVersion), "region": ptrString(row.Region), "modem_preset": ptrString(row.ModemPreset), "latitude": ptrFloat64(row.Latitude), "longitude": ptrFloat64(row.Longitude), "altitude": ptrInt64(row.Altitude), "position_precision": ptrInt64(row.PositionPrecision), "num_online_local_nodes": ptrInt64(row.NumOnlineLocalNodes), "has_opted_report_location": ptrBool(row.HasOptedReportLocation), "updated_at": row.UpdatedAt, "content_json": row.ContentJSON}
}

func mapReportViewportPointDTO(row storepkg.MapReportRecord) gin.H {
	item := mapReportDTO(row)
	item["type"] = "point"
	return item
}

func mapReportClusterDTO(row storepkg.MapReportClusterRecord) gin.H {
	return gin.H{"type": "cluster", "cluster_id": row.ClusterID, "latitude": row.Latitude, "longitude": row.Longitude, "count": row.Count}
}

func textMessageDTO(row storepkg.TextMessageRecord) gin.H {
	return gin.H{"id": row.ID, "from_id": row.FromID, "from_num": row.FromNum, "packet_id": ptrInt64(row.PacketID), "text": ptrString(row.Text), "topic": row.Topic, "channel_id": ptrString(row.ChannelID), "created_at": row.CreatedAt, "mqtt_remote_host": ptrString(row.MQTTRemoteHost), "content_json": row.ContentJSON}
}

func discardDetailsDTO(row storepkg.DiscardDetailsRecord) gin.H {
	return gin.H{"id": row.ID, "topic": row.Topic, "error": row.Error, "payload_len": row.PayloadLen, "raw_base64": row.RawBase64, "mqtt_client_id": ptrString(row.MQTTClientID), "mqtt_username": ptrString(row.MQTTUsername), "mqtt_listener": ptrString(row.MQTTListener), "mqtt_remote_addr": ptrString(row.MQTTRemoteAddr), "mqtt_remote_host": ptrString(row.MQTTRemoteHost), "mqtt_remote_port": ptrString(row.MQTTRemotePort), "created_at": row.CreatedAt, "content_json": row.ContentJSON}
}

func positionDTO(row storepkg.PositionRecord) gin.H {
	return gin.H{"id": row.ID, "from_id": row.FromID, "from_num": row.FromNum, "latitude": ptrFloat64(row.Latitude), "longitude": ptrFloat64(row.Longitude), "altitude": ptrInt64(row.Altitude), "created_at": row.CreatedAt, "content_json": row.ContentJSON}
}

func telemetryDTO(row storepkg.TelemetryRecord) gin.H {
	return gin.H{"id": row.ID, "from_id": row.FromID, "from_num": row.FromNum, "telemetry_type": ptrString(row.TelemetryType), "metrics_json": ptrString(row.MetricsJSON), "created_at": row.CreatedAt, "content_json": row.ContentJSON}
}

func routingDTO(row storepkg.RoutingRecord) gin.H {
	return appendPacketDTO(row.ID, row.FromID, row.FromNum, row.PacketID, row.Portnum, row.CreatedAt, row.ContentJSON)
}

func tracerouteDTO(row storepkg.TracerouteRecord) gin.H {
	return appendPacketDTO(row.ID, row.FromID, row.FromNum, row.PacketID, row.Portnum, row.CreatedAt, row.ContentJSON)
}

func appendPacketDTO(id uint64, fromID string, fromNum int64, packetID *int64, portnum *string, createdAt time.Time, contentJSON string) gin.H {
	return gin.H{"id": id, "from_id": fromID, "from_num": fromNum, "packet_id": ptrInt64(packetID), "portnum": ptrString(portnum), "created_at": createdAt, "content_json": contentJSON}
}

func ptrString(value *string) any  { return webutil.PtrString(value) }
func ptrInt64(value *int64) any    { return webutil.PtrInt64(value) }
func ptrUint64(value *uint64) any  { return webutil.PtrUint64(value) }
func ptrFloat64(value *float64) any { return webutil.PtrFloat64(value) }
func ptrBool(value *bool) any      { return webutil.PtrBool(value) }
