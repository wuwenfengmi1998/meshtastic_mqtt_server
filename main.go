package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	defaultHost     = "mqtt.meshtastic.org"
	defaultUsername = "meshdev"
	defaultPassword = "large4cats"
	defaultPSK      = "AQ=="
	defaultTopic    = "msh/US/#"

	ansiGreenBGWhiteText  = "\033[42;37m"
	ansiBlueBGWhiteText   = "\033[44;37m"
	ansiPurpleBGWhiteText = "\033[45;37m"
	ansiCyanBGBlackText   = "\033[46;30m"
	ansiYellowBGBlackText = "\033[43;30m"
	ansiGrayBGWhiteText   = "\033[100;37m"
	ansiRedBGWhiteText    = "\033[41;37m"
	ansiReset             = "\033[0m"
)

type config struct {
	host     string
	port     int
	username string
	password string
	psk      string
	topics   topicsFlag
	qos      int
	clientID string
	key      []byte
}

type topicsFlag []string

// String 将已配置的 topic 列表转换为字符串，供 flag 包显示默认值或帮助信息。
func (t *topicsFlag) String() string {
	if t == nil {
		return ""
	}
	b, _ := json.Marshal([]string(*t))
	return string(b)
}

// Set 追加一个 --topic 参数值，支持命令行重复传入多个订阅主题。
func (t *topicsFlag) Set(value string) error {
	*t = append(*t, value)
	return nil
}

// main 是程序入口，负责解析参数并启动 MQTT 订阅流程。
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

// parseArgs 解析命令行参数，并展开 Meshtastic channel PSK。
func parseArgs() (*config, error) {
	cfg := &config{}
	flag.StringVar(&cfg.host, "host", defaultHost, "MQTT broker hostname")
	flag.IntVar(&cfg.port, "port", 1883, "MQTT broker port")
	flag.StringVar(&cfg.username, "username", defaultUsername, "MQTT username")
	flag.StringVar(&cfg.password, "password", defaultPassword, "MQTT password")
	flag.StringVar(&cfg.psk, "psk", defaultPSK, "Base64 channel PSK used to try decrypting encrypted packets")
	flag.Var(&cfg.topics, "topic", "Topic to subscribe; may be repeated. Defaults to msh/US/#")
	flag.IntVar(&cfg.qos, "qos", 0, "MQTT subscription QoS (0, 1, or 2)")
	flag.StringVar(&cfg.clientID, "client-id", "meshtastic-nodeinfo-subscriber", "MQTT client id")
	flag.Parse()

	if len(cfg.topics) == 0 {
		cfg.topics = topicsFlag{defaultTopic}
	}
	if cfg.qos < 0 || cfg.qos > 2 {
		return nil, fmt.Errorf("invalid qos %d: must be 0, 1, or 2", cfg.qos)
	}
	key, err := expandPSK(cfg.psk)
	if err != nil {
		return nil, err
	}
	cfg.key = key
	return cfg, nil
}

// run 创建 MQTT 客户端，连接 broker，订阅 topic，并阻塞等待退出信号。
func run(cfg *config) error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", cfg.host, cfg.port))
	opts.SetClientID(cfg.clientID)
	opts.SetKeepAlive(60 * time.Second)
	if cfg.username != "" {
		opts.SetUsername(cfg.username)
		opts.SetPassword(cfg.password)
	}

	opts.OnConnect = func(client mqtt.Client) {
		printJSON(map[string]any{"event": "connected", "reason_code": "0"})
		for _, topic := range cfg.topics {
			token := client.Subscribe(topic, byte(cfg.qos), handleMessage(cfg.key))
			token.Wait()
			if err := token.Error(); err != nil {
				printJSON(map[string]any{"event": "subscribe_error", "topic": topic, "qos": cfg.qos, "error": err.Error()})
				continue
			}
			printJSON(map[string]any{"event": "subscribed", "topic": topic, "qos": cfg.qos})
		}
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	client.Disconnect(250)
	return nil
}

// handleMessage 返回 MQTT 消息回调，把原始 payload 交给 MQTTPP 处理后按类型输出。
func handleMessage(key []byte) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		valid, _, decodedJSON := MQTTPP(msg.Topic(), msg.Payload(), key)
		if !valid || len(decodedJSON) == 0 {
			return
		}

		var record map[string]any
		if err := json.Unmarshal(decodedJSON, &record); err != nil {
			printJSON(map[string]any{"topic": msg.Topic(), "error": "json decode failed: " + err.Error(), "payload_len": len(msg.Payload())})
			return
		}
		if record["type"] == "empty_packet" {
			return
		}
		printJSONBytes(record, decodedJSON)
	}
}

// printJSON 将记录编码为 JSON 后按数据包类型着色输出。
func printJSON(record map[string]any) {
	printJSONBytes(record, mustJSON(record))
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
