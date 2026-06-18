package main

// 桥接到 internal/mapsource — 让 web.go 中的 registerMapSourceRoutes /
// registerAdminMapSourceRoutes 旧名字仍可用。

import (
	"github.com/gin-gonic/gin"

	mspkg "meshtastic_mqtt_server/internal/mapsource"
)

func registerMapSourceRoutes(r gin.IRouter, s *store)       { mspkg.RegisterPublicRoutes(r, s) }
func registerAdminMapSourceRoutes(r gin.IRouter, s *store)  { mspkg.RegisterAdminRoutes(r, s) }
