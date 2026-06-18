package main

// 桥接到 internal/bot — 让 main.go / web.go 中使用 botService /
// botTextSender / newBotService / newPKIKeyResolver / registerAdminBotRoutes
// 这些旧名字的代码继续可用。

import (
	mqtt "github.com/mochi-mqtt/server/v2"

	"github.com/gin-gonic/gin"

	botpkg "meshtastic_mqtt_server/internal/bot"
)

type (
	botService         = botpkg.Service
	botTextSender      = botpkg.TextSender
	botSendTextRequest = botpkg.SendTextRequest
)

func newBotService(s *store, server *mqtt.Server, key []byte) *botService {
	return botpkg.NewService(s, server, key)
}

func newPKIKeyResolver(s *store) func(toNodeNum, fromNodeNum uint32) ([]byte, []byte, bool) {
	return botpkg.NewPKIKeyResolver(s)
}

func registerAdminBotRoutes(r gin.IRouter, s *store, sender botTextSender) {
	botpkg.RegisterRoutes(r, s, sender)
}
