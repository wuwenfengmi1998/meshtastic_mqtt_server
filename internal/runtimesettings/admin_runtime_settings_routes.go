package runtimesettings

import (
	"net/http"

	"github.com/gin-gonic/gin"

	storepkg "meshtastic_mqtt_server/internal/store"
)

const allowEncryptedForwardingLabel = "Allow encrypted MQTT packets to be forwarded when they cannot be decrypted"
const llmQueueEnabledLabel = "Enable LLM message queue"
const llmIncludeChannelLabel = "Include channel messages in LLM queue"

type runtimeSettingsRequest struct {
	AllowEncryptedForwarding  bool `json:"allow_encrypted_forwarding"`
	LLMQueueEnabled           bool `json:"llm_queue_enabled"`
	LLMIncludeChannelMessages bool `json:"llm_include_channel_messages"`
}

// RegisterRoutes 把 GET /runtime-settings 与 PUT /runtime-settings 挂到给定路由组下。
func RegisterRoutes(r gin.IRouter, store *storepkg.Store, settings *Cache) {
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
		if _, err := store.SetBoolRuntimeSetting(storepkg.RuntimeSettingAllowEncryptedForwarding, req.AllowEncryptedForwarding, allowEncryptedForwardingLabel); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if _, err := store.SetBoolRuntimeSetting(storepkg.RuntimeSettingLLMQueueEnabled, req.LLMQueueEnabled, llmQueueEnabledLabel); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if _, err := store.SetBoolRuntimeSetting(storepkg.RuntimeSettingLLMQueueIncludeChannel, req.LLMIncludeChannelMessages, llmIncludeChannelLabel); err != nil {
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

func runtimeSettingsDTO(settings storepkg.RuntimeSettingsSnapshot) gin.H {
	return gin.H{
		"allow_encrypted_forwarding":   settings.AllowEncryptedForwarding,
		"llm_queue_enabled":            settings.LLMQueueEnabled,
		"llm_include_channel_messages": settings.LLMIncludeChannel,
	}
}
