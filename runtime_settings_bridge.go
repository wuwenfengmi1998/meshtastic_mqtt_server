package main

// 桥接到 internal/runtimesettings — 让根目录代码继续使用旧的小写名字。

import (
	"github.com/gin-gonic/gin"

	rspkg "meshtastic_mqtt_server/internal/runtimesettings"
)

type runtimeSettingsCache = rspkg.Cache

func newRuntimeSettingsCache(s *store) (*runtimeSettingsCache, error) {
	return rspkg.New(s)
}

func registerAdminRuntimeSettingsRoutes(r gin.IRouter, s *store, c *runtimeSettingsCache) {
	rspkg.RegisterRoutes(r, s, c)
}
