package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	mqttauth "github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"meshtastic_mqtt_server/internal/ai"
	"meshtastic_mqtt_server/internal/autoreply"
	"meshtastic_mqtt_server/internal/auth"
	blockingpkg "meshtastic_mqtt_server/internal/blocking"
	botpkg "meshtastic_mqtt_server/internal/bot"
	configpkg "meshtastic_mqtt_server/internal/config"
	mqttforwardpkg "meshtastic_mqtt_server/internal/mqttforward"
	rspkg "meshtastic_mqtt_server/internal/runtimesettings"
	storepkg "meshtastic_mqtt_server/internal/store"
	webpkg "meshtastic_mqtt_server/internal/web"
	"meshtastic_mqtt_server/internal/llm"
	"meshtastic_mqtt_server/internal/mqtpp"
)

const (
	ansiGreenBGWhiteText  = "\033[42;37m"
	ansiBlueBGWhiteText   = "\033[44;37m"
	ansiPurpleBGWhiteText = "\033[45;37m"
	ansiCyanBGBlackText   = "\033[46;30m"
	ansiYellowBGBlackText = "\033[43;30m"
	ansiGrayBGWhiteText   = "\033[100;37m"
	ansiRedBGWhiteText    = "\033[41;37m"
	ansiReset             = "\033[0m"
)

type meshtasticFilterHook struct {
	mqtt.HookBase
	key         []byte
	dbQueue     *storepkg.WriteQueue
	stats       *mqttforwardpkg.Stats
	blocking    *blockingpkg.Cache
	settings    *rspkg.Cache
	pkiResolver func(toNodeNum, fromNodeNum uint32) ([]byte, []byte, bool)
	autoAcker   func(record map[string]any)
}

// ID 返回用于识别 Meshtastic payload 过滤器的 hook 名称。
func (h *meshtasticFilterHook) ID() string {
	return "meshtastic-filter"
}

// Provides 声明该 hook 只处理客户端发布消息。
func (h *meshtasticFilterHook) Provides(b byte) bool {
	return b == mqtt.OnConnect || b == mqtt.OnPublish
}

// OnConnect 在 MQTT 会话建立前拒绝命中 IP 屏蔽表的客户端。
func (h *meshtasticFilterHook) OnConnect(cl *mqtt.Client, pk packets.Packet) error {
	info := mqttClientInfoFromClient(cl)
	if h.blocking != nil && h.blocking.IsIPBlocked(info.RemoteHost) {
		printJSON(map[string]any{"event": "mqtt_client_rejected", "reason": "blocked_ip", "client_id": info.ClientID, "remote_addr": info.RemoteAddr, "remote_host": info.RemoteHost})
		return packets.ErrNotAuthorized
	}
	return nil
}

// OnPublish 在 broker 转发消息前校验 payload；无效消息会被拒绝并丢弃。
func (h *meshtasticFilterHook) OnPublish(cl *mqtt.Client, pk packets.Packet) (packets.Packet, error) {
	valid, _, record := mqtpp.MQTTPP(pk.TopicName, pk.Payload, h.key, mqtpp.Options{
		AllowEncryptedForwarding: h.settings.AllowEncryptedForwarding(),
		PKIKeyResolver:           h.pkiResolver,
	})
	if !valid {
		h.rejectPublish(cl, pk, record)
		return pk, packets.ErrRejectPacket
	}
	if violation := blockingViolationForRecord(h.blocking, record); violation != nil {
		for key, value := range violation {
			record[key] = value
		}
		h.rejectPublish(cl, pk, record)
		return pk, packets.ErrRejectPacket
	}
	h.stats.IncForwarded()

	h.dbQueue.EnqueueRecord(record, mqttClientInfoFromClient(cl))
	if h.autoAcker != nil {
		h.autoAcker(record)
	}
	if record["type"] != "empty_packet" {
		printJSON(record)
	}
	return pk, nil
}

func (h *meshtasticFilterHook) rejectPublish(cl *mqtt.Client, pk packets.Packet, record map[string]any) {
	if h.stats != nil {
		h.stats.IncDropped()
	}
	if record == nil {
		record = map[string]any{}
	}
	record["topic"] = pk.TopicName
	h.dbQueue.EnqueueDiscard(record, pk.Payload, mqttClientInfoFromClient(cl))
}

func blockingViolationForRecord(blocking *blockingpkg.Cache, record map[string]any) map[string]any {
	if blocking == nil || record == nil {
		return nil
	}
	if blocking.IsNodeBlocked(record["from"], record["from_num"]) {
		return map[string]any{"error": "blocked node", "blocking_type": "node"}
	}
	var field string
	switch record["type"] {
	case "text_message":
		field = "text"
	case "nodeinfo", "map_report":
		field = "long_name"
	default:
		return nil
	}
	if word, ok := blocking.FindForbiddenWord(record[field]); ok {
		return map[string]any{"error": "forbidden word", "blocking_type": "forbidden_word", "blocking_field": field, "matched_word": word}
	}
	return nil
}

func mqttClientInfoFromClient(cl *mqtt.Client) storepkg.MQTTClientInfo {
	if cl == nil {
		return storepkg.MQTTClientInfo{}
	}

	info := storepkg.MQTTClientInfo{
		ClientID:   cl.ID,
		Username:   string(cl.Properties.Username),
		Listener:   cl.Net.Listener,
		RemoteAddr: cl.Net.Remote,
	}
	if info.RemoteAddr == "" && cl.Net.Conn != nil && cl.Net.Conn.RemoteAddr() != nil {
		info.RemoteAddr = cl.Net.Conn.RemoteAddr().String()
	}
	if host, port, err := net.SplitHostPort(info.RemoteAddr); err == nil {
		info.RemoteHost = host
		info.RemotePort = port
	} else {
		info.RemoteHost = info.RemoteAddr
	}
	return info
}

// main 是程序入口，负责解析参数并启动 MQTT broker。
func main() {
	cfg, err := parseArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// parseArgs 加载配置文件、解析命令行覆盖项，并展开 Meshtastic channel PSK。
func parseArgs() (*configpkg.Config, error) {
	cfg, err := configpkg.Load(configpkg.DefaultPath())
	if err != nil {
		return nil, err
	}

	flag.StringVar(&cfg.MQTT.Host, "host", cfg.MQTT.Host, "MQTT broker listen host")
	flag.IntVar(&cfg.MQTT.Port, "port", cfg.MQTT.Port, "MQTT broker listen port")
	flag.StringVar(&cfg.Meshtastic.PSK, "psk", cfg.Meshtastic.PSK, "Base64 channel PSK used to try decrypting encrypted packets")
	flag.BoolVar(&cfg.MQTT.TLS.Enabled, "tls", cfg.MQTT.TLS.Enabled, "Enable MQTT TLS listener")
	flag.StringVar(&cfg.MQTT.TLS.CertFile, "tls-cert", cfg.MQTT.TLS.CertFile, "MQTT TLS certificate file")
	flag.StringVar(&cfg.MQTT.TLS.KeyFile, "tls-key", cfg.MQTT.TLS.KeyFile, "MQTT TLS private key file")
	flag.StringVar(&cfg.Database.Driver, "db-driver", cfg.Database.Driver, "Database driver: sqlite or mysql")
	flag.StringVar(&cfg.Database.SQLite.Path, "sqlite-path", cfg.Database.SQLite.Path, "SQLite database file path")
	flag.StringVar(&cfg.Database.MySQL.DSN, "mysql-dsn", cfg.Database.MySQL.DSN, "MySQL database DSN")
	flag.BoolVar(&cfg.Web.Enabled, "web", cfg.Web.Enabled, "Enable Gin web server")
	flag.BoolVar(&cfg.Web.PortEnabled, "web-port-enabled", cfg.Web.PortEnabled, "Enable web server on TCP host and port")
	flag.BoolVar(&cfg.Web.SocketEnabled, "web-socket-enabled", cfg.Web.SocketEnabled, "Enable web server on Unix socket; unsupported on Windows")
	flag.StringVar(&cfg.Web.Host, "web-host", cfg.Web.Host, "Web server listen host")
	flag.IntVar(&cfg.Web.Port, "web-port", cfg.Web.Port, "Web server listen port")
	flag.StringVar(&cfg.Web.SocketPath, "web-socket-path", cfg.Web.SocketPath, "Web server Unix socket path; unsupported on Windows")
	flag.StringVar(&cfg.Web.StaticDir, "web-static-dir", cfg.Web.StaticDir, "Web frontend static files directory")
	flag.StringVar(&cfg.Web.MapTileCacheDir, "web-map-tile-cache-dir", cfg.Web.MapTileCacheDir, "Map tile disk cache root directory")
	flag.StringVar(&cfg.Web.Admin.Username, "admin-username", cfg.Web.Admin.Username, "Web admin username")
	flag.Parse()

	if value := os.Getenv("MESH_ADMIN_PASSWORD"); value != "" {
		cfg.Web.Admin.Password = value
	}
	if value := os.Getenv("MESH_ADMIN_SESSION_SECRET"); value != "" {
		cfg.Web.Admin.SessionSecret = value
	}
	configpkg.ClearWebSocketPathOnUnsupportedGOOS(cfg, runtime.GOOS)

	if err := configpkg.Validate(cfg); err != nil {
		return nil, err
	}
	key, err := mqtpp.ExpandPSK(cfg.Meshtastic.PSK)
	if err != nil {
		return nil, err
	}
	cfg.Key = key
	return cfg, nil
}

// run 创建 MQTT broker 和 Web 服务，并阻塞等待退出信号。
func run(cfg *configpkg.Config) error {
	store, err := storepkg.OpenStore(cfg.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	dbQueue := storepkg.NewWriteQueue(store)
	defer dbQueue.Close()
	if err := store.EnsureDefaultAdmin(cfg.Web.Admin.Username, cfg.Web.Admin.Password); err != nil {
		return err
	}

	blocking, err := blockingpkg.New(store)
	if err != nil {
		return err
	}
	settings, err := rspkg.New(store)
	if err != nil {
		return err
	}

	messageStats := &mqttforwardpkg.Stats{}
	server, mqttHook, mqttAddr, err := startMQTTServer(cfg, store, dbQueue, messageStats, blocking, settings)
	if err != nil {
		return err
	}
	botSender := botpkg.NewService(store, server, cfg.Key)
	mqttHook.autoAcker = botSender.MaybeAutoAck
	botCtx, stopBotBroadcaster := context.WithCancel(context.Background())
	defer stopBotBroadcaster()
	botSender.StartNodeInfoBroadcaster(botCtx)
	forwardManager := mqttforwardpkg.NewManager(store)
	if err := forwardManager.StartFromStore(); err != nil {
		server.Close()
		return err
	}
	defer forwardManager.StopAll()

	// Initialize AI Service
	var aiService *ai.Service
	if cfg.AI.Enabled {
		// Get LLM providers from database
		llmProviders, err := store.ListLLMProviders(true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load LLM providers: %v\n", err)
		} else if len(llmProviders) > 0 {
			// Convert database records to provider configs
			providerConfigs := make([]llm.ProviderConfig, 0, len(llmProviders))
			for _, p := range llmProviders {
				providerConfigs = append(providerConfigs, llm.ProviderConfig{
					Name:                p.Name,
					Active:              p.Active,
					APIKey:              p.APIKey,
					BaseURL:             p.BaseURL,
					Model:               p.Model,
					Timeout:             p.Timeout,
					ContextWindowTokens: p.ContextWindowTokens,
				})
			}

			// Create bot sender adapter - 支持频道消息和私聊消息两种发送方式
			botSenderAdapter := autoreply.NewBotServiceAdapter(
				// SendDirectText: 发送私聊消息
				func(ctx context.Context, botID uint64, toNodeNum int64, text string) error {
					_, err := botSender.SendText(ctx, botpkg.SendTextRequest{
						BotID:       botID,
						MessageType: "direct",
						ToNodeNum:   &toNodeNum,
						Text:        text,
					})
					return err
				},
				// SendChannelText: 发送频道消息
				func(ctx context.Context, botID uint64, channelID string, text string) error {
					_, err := botSender.SendText(ctx, botpkg.SendTextRequest{
						BotID:       botID,
						MessageType: "channel",
						ChannelID:   channelID,
						Text:        text,
					})
					return err
				},
			)

			aiService, err = ai.NewService(ai.Config{
				LLMProviders:    providerConfigs,
				DataDir:         cfg.DataDir,
				Enabled:         cfg.AI.Enabled,
				ToolConfigStore: store,
			}, store.DB(), botSenderAdapter)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to initialize AI service: %v\n", err)
			} else {
				if err := aiService.Start(botCtx); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to start AI service: %v\n", err)
				}
				defer aiService.Stop()
				printJSON(map[string]any{"event": "ai_service_started", "providers": len(providerConfigs)})
			}
		} else {
			fmt.Fprintf(os.Stderr, "Warning: AI service is enabled but no LLM providers configured\n")
		}
	}

	var httpServers []*http.Server
	errCh := make(chan error, 2)
	if cfg.Web.Enabled {
		sessions, err := auth.NewManager(cfg.Web.Admin)
		if err != nil {
			return err
		}
		mqttStatus := webpkg.MQTTRuntimeStatus{Server: server, Address: mqttAddr, TLS: cfg.MQTT.TLS.Enabled, Stats: messageStats, DBQueue: dbQueue}
		handler := webpkg.NewRouter(cfg.Web, store, sessions, mqttStatus, blocking, forwardManager, settings, botSender)
		webAddresses := []string{}
		if cfg.Web.PortEnabled {
			httpServer := &http.Server{
				Addr:    net.JoinHostPort(cfg.Web.Host, strconv.Itoa(cfg.Web.Port)),
				Handler: handler,
			}
			httpServers = append(httpServers, httpServer)
			webAddresses = append(webAddresses, httpServer.Addr)
			go func() {
				if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- err
				}
			}()
		}
		if cfg.Web.SocketEnabled {
			httpServer := &http.Server{Handler: handler}
			httpServers = append(httpServers, httpServer)
			webAddresses = append(webAddresses, cfg.Web.SocketPath)
			go func() {
				if err := webpkg.ServeUnixSocket(httpServer, cfg.Web.SocketPath); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- err
				}
			}()
		}
		webStarted := map[string]any{"event": "web_started", "addresses": webAddresses, "static_dir": cfg.Web.StaticDir}
		if len(webAddresses) > 0 {
			webStarted["address"] = webAddresses[0]
		}
		printJSON(webStarted)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	var runErr error
	select {
	case <-sigCh:
	case runErr = <-errCh:
	}

	for _, httpServer := range httpServers {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := httpServer.Shutdown(ctx); err != nil && runErr == nil {
			runErr = err
		}
		cancel()
	}
	if err := server.Close(); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}

func startMQTTServer(cfg *configpkg.Config, store *storepkg.Store, dbQueue *storepkg.WriteQueue, stats *mqttforwardpkg.Stats, blocking *blockingpkg.Cache, settings *rspkg.Cache) (*mqtt.Server, *meshtasticFilterHook, string, error) {
	server := mqtt.New(&mqtt.Options{InlineClient: true})
	if err := server.AddHook(new(mqttauth.AllowHook), nil); err != nil {
		return nil, nil, "", err
	}
	hook := &meshtasticFilterHook{
		key:         cfg.Key,
		dbQueue:     dbQueue,
		stats:       stats,
		blocking:    blocking,
		settings:    settings,
		pkiResolver: botpkg.NewPKIKeyResolver(store),
	}
	if err := server.AddHook(hook, nil); err != nil {
		return nil, nil, "", err
	}

	addr := net.JoinHostPort(cfg.MQTT.Host, strconv.Itoa(cfg.MQTT.Port))
	tlsConfig, err := configpkg.BuildTLS(cfg.MQTT.TLS)
	if err != nil {
		return nil, nil, "", err
	}
	listener := listeners.NewTCP(listeners.Config{ID: "tcp", Address: addr, TLSConfig: tlsConfig})
	if err := server.AddListener(listener); err != nil {
		return nil, nil, "", err
	}
	if err := server.Serve(); err != nil {
		return nil, nil, "", err
	}
	printJSON(map[string]any{"event": "broker_started", "address": addr, "tls": cfg.MQTT.TLS.Enabled})
	return server, hook, addr, nil
}

// printJSON 将记录编码为 JSON 后按数据包类型着色输出。
func printJSON(record map[string]any) {
	//printJSONBytes(record, mqtpp.MustJSON(record))
}

// printJSONBytes 使用已编码好的 JSON 文本，并根据记录 type 选择控制台颜色。
func printJSONBytes(record map[string]any, text []byte) {
	switch record["type"] {
	case "nodeinfo":
		fmt.Printf("%s%s%s\n", ansiGreenBGWhiteText, text, ansiReset)
	case "map_report":
		fmt.Printf("%s%s%s\n", ansiBlueBGWhiteText, text, ansiReset)
	case "text_message":
		fmt.Printf("%s%s%s\n", ansiPurpleBGWhiteText, text, ansiReset)
	case "position":
		fmt.Printf("%s%s%s\n", ansiCyanBGBlackText, text, ansiReset)
	case "telemetry":
		fmt.Printf("%s%s%s\n", ansiYellowBGBlackText, text, ansiReset)
	case "routing", "traceroute":
		fmt.Printf("%s%s%s\n", ansiGrayBGWhiteText, text, ansiReset)
	default:
		if record["error"] != nil {
			fmt.Printf("%s%s%s\n", ansiRedBGWhiteText, text, ansiReset)
			return
		}
		fmt.Println(string(text))
	}
}
