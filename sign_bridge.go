package main

// 桥接到 internal/sign — 让 web.go 中 registerAdminSignRoutes / signDTO /
// signDayCountDTO 旧名字仍可用。

import (
	"github.com/gin-gonic/gin"

	signpkg "meshtastic_mqtt_server/internal/sign"
)

func registerAdminSignRoutes(r gin.IRouter, s *store) {
	signpkg.RegisterAdminRoutes(r, s)
}

func signDTO(row signRecord) gin.H              { return signpkg.SignDTO(row) }
func signDayCountDTO(row signDayCount) gin.H    { return signpkg.SignDayCountDTO(row) }
