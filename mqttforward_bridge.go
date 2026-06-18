package main

// 桥接到 internal/mqttforward — 让 main.go / web.go / mqtt_status.go 中
// 使用 mqttForwardManager / meshtasticMessageStats / mqttForwardReloader 等
// 旧名字的代码仍可工作。

import (
	"github.com/gin-gonic/gin"

	mfpkg "meshtastic_mqtt_server/internal/mqttforward"
)

type (
	mqttForwardManager       = mfpkg.Manager
	mqttForwardReloader      = mfpkg.Reloader
	mqttForwardRuntimeStatus = mfpkg.RuntimeStatus
	meshtasticMessageStats   = mfpkg.Stats
)

func newMQTTForwardManager(s *store) *mqttForwardManager {
	return mfpkg.NewManager(s)
}

func registerAdminMQTTForwardRoutes(r gin.IRouter, s *store, forwarder mqttForwardReloader) {
	mfpkg.RegisterRoutes(r, s, forwarder)
}
