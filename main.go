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
	"strconv"
	"syscall"
	"time"

	"meshtastic_mqtt_server/mqtpp"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
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
	key   []byte
	store *store
}

// ID 返回用于识别 Meshtastic payload 过滤器的 hook 名称。
func (h *meshtasticFilterHook) ID() string {
	return "meshtastic-filter"
}

// Provides 声明该 hook 只处理客户端发布消息。
func (h *meshtasticFilterHook) Provides(b byte) bool {
	return b == mqtt.OnPublish
}

// OnPublish 在 broker 转发消息前校验 payload；无效消息会被拒绝并丢弃。
func (h *meshtasticFilterHook) OnPublish(cl *mqtt.Client, pk packets.Packet) (packets.Packet, error) {
	valid, _, record := mqtpp.MQTTPP(pk.TopicName, pk.Payload, h.key)
	if !valid {
		return pk, packets.ErrRejectPacket
	}

	switch record["type"] {
	case "nodeinfo", "map_report":
		if h.store != nil {
			if err := h.store.UpsertNodeInfoMap(record); err != nil {
				printJSON(map[string]any{"event": "db_error", "type": record["type"], "from": record["from"], "error": err.Error()})
			}
		}
	case "text_message":
		if h.store != nil {
			if err := h.store.InsertTextMessage(record, mqttClientInfoFromClient(cl)); err != nil {
				printJSON(map[string]any{"event": "db_error", "type": record["type"], "from": record["from"], "error": err.Error()})
			}
		}
	case "position":
		if h.store != nil {
			if err := h.store.InsertPosition(record, mqttClientInfoFromClient(cl)); err != nil {
				printJSON(map[string]any{"event": "db_error", "type": record["type"], "from": record["from"], "error": err.Error()})
			}
		}
	case "telemetry":
		if h.store != nil {
			if err := h.store.InsertTelemetry(record, mqttClientInfoFromClient(cl)); err != nil {
				printJSON(map[string]any{"event": "db_error", "type": record["type"], "from": record["from"], "error": err.Error()})
			}
		}
	case "routing":
		if h.store != nil {
			if err := h.store.InsertRouting(record, mqttClientInfoFromClient(cl)); err != nil {
				printJSON(map[string]any{"event": "db_error", "type": record["type"], "from": record["from"], "error": err.Error()})
			}
		}
	case "traceroute":
		if h.store != nil {
			if err := h.store.InsertTraceroute(record, mqttClientInfoFromClient(cl)); err != nil {
				printJSON(map[string]any{"event": "db_error", "type": record["type"], "from": record["from"], "error": err.Error()})
			}
		}
	}
	if record["type"] != "empty_packet" {
		printJSON(record)
	}
	return pk, nil
}

func mqttClientInfoFromClient(cl *mqtt.Client) mqttClientInfo {
	if cl == nil {
		return mqttClientInfo{}
	}

	info := mqttClientInfo{
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
func parseArgs() (*config, error) {
	cfg, err := loadConfig(defaultConfigPath())
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
	flag.StringVar(&cfg.Web.Host, "web-host", cfg.Web.Host, "Web server listen host")
	flag.IntVar(&cfg.Web.Port, "web-port", cfg.Web.Port, "Web server listen port")
	flag.StringVar(&cfg.Web.StaticDir, "web-static-dir", cfg.Web.StaticDir, "Web frontend static files directory")
	flag.Parse()

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	key, err := mqtpp.ExpandPSK(cfg.Meshtastic.PSK)
	if err != nil {
		return nil, err
	}
	cfg.key = key
	return cfg, nil
}

// run 创建 MQTT broker 和 Web 服务，并阻塞等待退出信号。
func run(cfg *config) error {
	store, err := openStore(cfg.Database)
	if err != nil {
		return err
	}
	defer store.Close()

	server, _, err := startMQTTServer(cfg, store)
	if err != nil {
		return err
	}

	var httpServer *http.Server
	errCh := make(chan error, 1)
	if cfg.Web.Enabled {
		httpServer = newHTTPServer(cfg.Web, store)
		go func() {
			if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
		printJSON(map[string]any{"event": "web_started", "address": httpServer.Addr, "static_dir": cfg.Web.StaticDir})
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	var runErr error
	select {
	case <-sigCh:
	case runErr = <-errCh:
	}

	if httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil && runErr == nil {
			runErr = err
		}
	}
	if err := server.Close(); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}

func startMQTTServer(cfg *config, store *store) (*mqtt.Server, string, error) {
	server := mqtt.New(nil)
	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		return nil, "", err
	}
	if err := server.AddHook(&meshtasticFilterHook{key: cfg.key, store: store}, nil); err != nil {
		return nil, "", err
	}

	addr := net.JoinHostPort(cfg.MQTT.Host, strconv.Itoa(cfg.MQTT.Port))
	tlsConfig, err := buildTLSConfig(cfg.MQTT.TLS)
	if err != nil {
		return nil, "", err
	}
	listener := listeners.NewTCP(listeners.Config{ID: "tcp", Address: addr, TLSConfig: tlsConfig})
	if err := server.AddListener(listener); err != nil {
		return nil, "", err
	}
	if err := server.Serve(); err != nil {
		return nil, "", err
	}
	printJSON(map[string]any{"event": "broker_started", "address": addr, "tls": cfg.MQTT.TLS.Enabled})
	return server, addr, nil
}

// printJSON 将记录编码为 JSON 后按数据包类型着色输出。
func printJSON(record map[string]any) {
	printJSONBytes(record, mqtpp.MustJSON(record))
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
