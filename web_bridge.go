package main

// 桥接到 internal/web — 让 main.go 中使用 newSessionManager / newRouter /
// mqttRuntimeStatus / serveHTTPUnixSocket 这些旧名字的代码继续编译。

import (
	"net/http"

	"github.com/gin-gonic/gin"
	mqtt "github.com/mochi-mqtt/server/v2"

	"meshtastic_mqtt_server/internal/auth"
	mqttforwardpkg "meshtastic_mqtt_server/internal/mqttforward"
	storepkg "meshtastic_mqtt_server/internal/store"
	webpkg "meshtastic_mqtt_server/internal/web"
)

// 旧类型/旧函数名 → 新位置的别名。

type mqttRuntimeStatusInternal = webpkg.MQTTRuntimeStatus

// mqttRuntimeStatus 旧名字保持小写、字段也是小写——这里用一个适配类型把
// main 包的旧字段写法包到 web 包导出的大写字段上。
type mqttRuntimeStatus struct {
	server  *mqtt.Server
	address string
	tls     bool
	stats   *meshtasticMessageStats
	dbQueue *dbWriteQueue
}

// 让 mqttRuntimeStatus 自动实现 webpkg.MQTTStatusProvider，把请求转给真正的实现。
func (m mqttRuntimeStatus) Status() webpkg.AdminMQTTStatus {
	return webpkg.MQTTRuntimeStatus{
		Server:  m.server,
		Address: m.address,
		TLS:     m.tls,
		Stats:   m.stats,
		DBQueue: m.dbQueue,
	}.Status()
}

// 让旧代码里 `mqttforwardpkg.Stats` 别名留作 main 包内可见。
var _ *mqttforwardpkg.Stats = (*meshtasticMessageStats)(nil)

func newSessionManager(cfg webAdminConfig) (*auth.Manager, error) {
	return auth.NewManager(cfg)
}

func newRouter(cfg webConfig, store *storepkg.Store, sessions *auth.Manager, mqttStatus webpkg.MQTTStatusProvider, blocking *blockingCache, forwarder mqttForwardReloader, settings *runtimeSettingsCache, botSender botTextSender) *gin.Engine {
	return webpkg.NewRouter(cfg, store, sessions, mqttStatus, blocking, forwarder, settings, botSender)
}

func serveHTTPUnixSocket(server *http.Server, socketPath string) error {
	return webpkg.ServeUnixSocket(server, socketPath)
}
