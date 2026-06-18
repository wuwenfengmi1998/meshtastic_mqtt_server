package main

// 桥接到 internal/llmadmin — 让 web.go 中的 registerAdminLLMRoutes 旧名仍可用。

import (
	"github.com/gin-gonic/gin"

	llmadminpkg "meshtastic_mqtt_server/internal/llmadmin"
)

func registerAdminLLMRoutes(r *gin.RouterGroup, s *store) {
	llmadminpkg.RegisterRoutes(r, s)
}
