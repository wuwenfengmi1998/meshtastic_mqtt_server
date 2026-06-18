package main

// 桥接到 internal/help — 让 web.go 中的 registerHelpRoutes /
// registerAdminHelpRoutes / renderHelpMarkdown 旧名字仍可用。

import (
	"github.com/gin-gonic/gin"

	helppkg "meshtastic_mqtt_server/internal/help"
)

func registerHelpRoutes(r gin.IRouter, s *store)      { helppkg.RegisterPublicRoutes(r, s) }
func registerAdminHelpRoutes(r gin.IRouter, s *store) { helppkg.RegisterAdminRoutes(r, s) }
func renderHelpMarkdown(markdown string) (string, error) {
	return helppkg.RenderMarkdown(markdown)
}
