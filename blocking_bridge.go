package main

// 桥接到 internal/blocking — 让根目录其余文件可以继续使用旧名字
// blockingCache / newBlockingCache / registerAdminBlockingRoutes，
// 而无须改动 main.go / web.go 等十几处调用点。

import (
	"github.com/gin-gonic/gin"

	blockingpkg "meshtastic_mqtt_server/internal/blocking"
)

type blockingCache = blockingpkg.Cache

func newBlockingCache(s *store) (*blockingCache, error) {
	return blockingpkg.New(s)
}

func registerAdminBlockingRoutes(r gin.IRouter, s *store, b *blockingCache) {
	blockingpkg.RegisterRoutes(r, s, b)
}
