package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const allowEncryptedForwardingLabel = "Allow encrypted MQTT packets to be forwarded when they cannot be decrypted"

type runtimeSettingsRequest struct {
	AllowEncryptedForwarding bool `json:"allow_encrypted_forwarding"`
}

func registerAdminRuntimeSettingsRoutes(r gin.IRouter, store *store, settings *runtimeSettingsCache) {
	r.GET("/runtime-settings", func(c *gin.Context) {
		snapshot, err := store.GetRuntimeSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": runtimeSettingsDTO(snapshot)})
	})

	r.PUT("/runtime-settings", func(c *gin.Context) {
		var req runtimeSettingsRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid runtime settings request"})
			return
		}
		if _, err := store.SetBoolRuntimeSetting(runtimeSettingAllowEncryptedForwarding, req.AllowEncryptedForwarding, allowEncryptedForwardingLabel); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if settings != nil {
			if err := settings.Reload(store); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		snapshot, err := store.GetRuntimeSettings()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": runtimeSettingsDTO(snapshot)})
	})
}

func runtimeSettingsDTO(settings runtimeSettingsSnapshot) gin.H {
	return gin.H{"allow_encrypted_forwarding": settings.AllowEncryptedForwarding}
}
