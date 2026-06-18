package main

// 桥接到 internal/auth — 让根目录其余文件继续用旧的小写名（sessionManager、
// sessionClaims、newSessionManager、requireAdmin、verifyPassword 等）。

import (
	"github.com/gin-gonic/gin"

	authpkg "meshtastic_mqtt_server/internal/auth"
)

// 类型别名
type (
	sessionManager = authpkg.Manager
	sessionClaims  = authpkg.SessionClaims
	adminUserDTO   = authpkg.AdminUserDTO
)

const adminRole = authpkg.AdminRole

func newSessionManager(cfg webAdminConfig) (*sessionManager, error) {
	return authpkg.NewManager(cfg)
}

func verifyPassword(hash, password string) bool { return authpkg.VerifyPassword(hash, password) }

func adminUserResponse(user userRecord) adminUserDTO { return authpkg.AdminUserResponse(user) }

func requireAdmin(sm *sessionManager) gin.HandlerFunc { return authpkg.RequireAdmin(sm) }
