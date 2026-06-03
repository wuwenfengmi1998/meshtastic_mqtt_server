package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"meshtastic_mqtt_server/mqtpp"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

const (
	defaultHost = "0.0.0.0"
	defaultPSK  = "AQ=="

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
	host string
	port int
	psk  string
	key  []byte
}

type meshtasticFilterHook struct {
	mqtt.HookBase
	key []byte
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
func (h *meshtasticFilterHook) OnPublish(_ *mqtt.Client, pk packets.Packet) (packets.Packet, error) {
	valid, _, record := mqtpp.MQTTPP(pk.TopicName, pk.Payload, h.key)
	if !valid {
		return pk, packets.ErrRejectPacket
	}

	if record["type"] != "empty_packet" {
		printJSON(record)
	}
	return pk, nil
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

// parseArgs 解析命令行参数，并展开 Meshtastic channel PSK。
func parseArgs() (*config, error) {
	cfg := &config{}
	flag.StringVar(&cfg.host, "host", defaultHost, "MQTT broker listen host")
	flag.IntVar(&cfg.port, "port", 1883, "MQTT broker listen port")
	flag.StringVar(&cfg.psk, "psk", defaultPSK, "Base64 channel PSK used to try decrypting encrypted packets")
	flag.Parse()

	key, err := mqtpp.ExpandPSK(cfg.psk)
	if err != nil {
		return nil, err
	}
	cfg.key = key
	return cfg, nil
}

// run 创建 MQTT broker，监听传入 publish，并阻塞等待退出信号。
func run(cfg *config) error {
	server := mqtt.New(nil)
	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		return err
	}
	if err := server.AddHook(&meshtasticFilterHook{key: cfg.key}, nil); err != nil {
		return err
	}

	addr := net.JoinHostPort(cfg.host, strconv.Itoa(cfg.port))
	listener := listeners.NewTCP(listeners.Config{ID: "tcp", Address: addr})
	if err := server.AddListener(listener); err != nil {
		return err
	}
	if err := server.Serve(); err != nil {
		return err
	}
	printJSON(map[string]any{"event": "broker_started", "address": addr})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	return server.Close()
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
