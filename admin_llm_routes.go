package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func registerAdminLLMRoutes(r *gin.RouterGroup, store *store) {
	group := r.Group("/llm")
	{
		// LLM Message Queue
		group.GET("/messages", handleListLLMMessages(store))
		group.GET("/messages/:id", handleGetLLMMessage(store))
		group.PUT("/messages/:id/status", handleUpdateLLMMessageStatus(store))
		group.DELETE("/messages/:id", handleDeleteLLMMessage(store))
		group.DELETE("/bots/:bot_id/messages", handleDeleteLLMMessagesByBot(store))
		group.POST("/messages/cleanup", handleCleanupDeletedLLMMessages(store))

		// LLM Providers
		group.GET("/providers", handleListLLMProviders(store))
		group.GET("/providers/:name", handleGetLLMProvider(store))
		group.POST("/providers", handleCreateLLMProvider(store))
		group.PUT("/providers/:name", handleUpdateLLMProvider(store))
		group.DELETE("/providers/:name", handleDeleteLLMProvider(store))

		// LLM Tool Router
		group.GET("/tool-router", handleGetLLMToolRouter(store))
		group.PUT("/tool-router", handleUpdateLLMToolRouter(store))

		// LLM Primary Config - 主 AI 回复配置
		group.GET("/primary-config", handleGetLLMPrimaryConfig(store))
		group.PUT("/primary-config", handleUpdateLLMPrimaryConfig(store))
	}
}

func handleListLLMMessages(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}

		var botID uint64
		if botIDStr := c.Query("bot_id"); botIDStr != "" {
			id, err := strconv.ParseUint(botIDStr, 10, 64)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot_id"})
				return
			}
			botID = id
		}

		includeDeleted := c.Query("include_deleted") == "true"

		rows, total, err := store.ListLLMMessages(opts, botID, includeDeleted)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			items = append(items, llmMessageDTO(row))
		}

		c.JSON(http.StatusOK, gin.H{
			"items":  items,
			"limit":  opts.Limit,
			"offset": opts.Offset,
			"total":  total,
		})
	}
}

func handleGetLLMMessage(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
			return
		}

		record, err := store.GetLLMMessage(id)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"item": llmMessageDTO(*record)})
	}
}

func handleUpdateLLMMessageStatus(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
			return
		}

		var req struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		// 验证状态值
		validStatus := map[string]bool{
			llmMessageStatusPending:    true,
			llmMessageStatusProcessing: true,
			llmMessageStatusProcessed:  true,
			llmMessageStatusError:      true,
		}
		if !validStatus[req.Status] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status value"})
			return
		}

		if err := store.UpdateLLMMessageStatus(id, req.Status, req.Error); err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func handleDeleteLLMMessage(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message id"})
			return
		}

		if err := store.SoftDeleteLLMMessage(id); err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "message not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func handleDeleteLLMMessagesByBot(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		botID, err := strconv.ParseUint(c.Param("bot_id"), 10, 64)
		if err != nil || botID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot id"})
			return
		}

		if err := store.SoftDeleteLLMMessagesByBot(botID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func handleCleanupDeletedLLMMessages(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Days int `json:"days"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			req.Days = 7 // 默认清理 7 天前删除的消息
		}
		if req.Days <= 0 {
			req.Days = 7
		}

		before := time.Now().AddDate(0, 0, -req.Days)
		count, err := store.CleanupDeletedLLMMessages(before)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "deleted_count": count})
	}
}

// ============================================
// LLM Provider Handlers
// ============================================

func handleListLLMProviders(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		includeInactive := c.Query("include_inactive") == "true"

		rows, err := store.ListLLMProviders(includeInactive)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			items = append(items, llmProviderDTO(row))
		}

		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func handleGetLLMProvider(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider name is required"})
			return
		}

		record, err := store.GetLLMProvider(name)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"item": llmProviderDTO(*record)})
	}
}

func handleCreateLLMProvider(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name               string `json:"name"`
			Active             bool   `json:"active"`
			APIKey             string `json:"api_key"`
			BaseURL            string `json:"base_url"`
			Model              string `json:"model"`
			Timeout            int    `json:"timeout"`
			ContextWindowTokens int   `json:"context_window_tokens"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		if req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
			return
		}

		record := &llmProviderRecord{
			Name:               req.Name,
			Active:             req.Active,
			APIKey:             req.APIKey,
			BaseURL:            req.BaseURL,
			Model:              req.Model,
			Timeout:            req.Timeout,
			ContextWindowTokens: req.ContextWindowTokens,
		}

		if err := store.CreateLLMProvider(record); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "item": llmProviderDTO(*record)})
	}
}

func handleUpdateLLMProvider(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider name is required"})
			return
		}

		var req struct {
			Active             *bool   `json:"active"`
			APIKey             *string `json:"api_key"`
			BaseURL            *string `json:"base_url"`
			Model              *string `json:"model"`
			Timeout            *int    `json:"timeout"`
			ContextWindowTokens *int   `json:"context_window_tokens"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		updates := make(map[string]any)
		if req.Active != nil {
			updates["active"] = *req.Active
		}
		if req.APIKey != nil {
			updates["api_key"] = *req.APIKey
		}
		if req.BaseURL != nil {
			updates["base_url"] = *req.BaseURL
		}
		if req.Model != nil {
			updates["model"] = *req.Model
		}
		if req.Timeout != nil {
			updates["timeout"] = *req.Timeout
		}
		if req.ContextWindowTokens != nil {
			updates["context_window_tokens"] = *req.ContextWindowTokens
		}

		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
			return
		}

		if err := store.UpdateLLMProvider(name, updates); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		record, err := store.GetLLMProvider(name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "item": llmProviderDTO(*record)})
	}
}

func handleDeleteLLMProvider(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "provider name is required"})
			return
		}

		if err := store.DeleteLLMProvider(name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func llmProviderDTO(row llmProviderRecord) map[string]any {
	return map[string]any{
		"name":                 row.Name,
		"active":               row.Active,
		"base_url":             row.BaseURL,
		"model":                row.Model,
		"timeout":              row.Timeout,
		"context_window_tokens": row.ContextWindowTokens,
		"created_at":           row.CreatedAt,
		"updated_at":           row.UpdatedAt,
	}
}

// ============================================
// LLM Tool Router Handlers
// ============================================

func handleGetLLMToolRouter(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		record, err := store.GetLLMToolRouter()
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "tool router config not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"item": llmToolRouterDTO(*record)})
	}
}

func handleUpdateLLMToolRouter(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		record, err := store.GetLLMToolRouter()
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var req struct {
			Enabled      *bool   `json:"enabled"`
			OpenAIName   *string `json:"openai_name"`
			Timeout      *int    `json:"timeout"`
			MaxTokens    *int    `json:"max_tokens"`
			SystemPrompt *string `json:"system_prompt"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		updates := make(map[string]any)
		if req.Enabled != nil {
			updates["enabled"] = *req.Enabled
		}
		if req.OpenAIName != nil {
			updates["openai_name"] = *req.OpenAIName
		}
		if req.Timeout != nil {
			updates["timeout"] = *req.Timeout
		}
		if req.MaxTokens != nil {
			updates["max_tokens"] = *req.MaxTokens
		}
		if req.SystemPrompt != nil {
			updates["system_prompt"] = *req.SystemPrompt
		}

		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
			return
		}

		if record == nil {
			// 创建新配置
			newRecord := &llmToolRouterRecord{
				Enabled:      req.Enabled != nil && *req.Enabled,
				OpenAIName:   "",
				Timeout:      30,
				MaxTokens:    512,
				SystemPrompt: "",
			}
			if req.OpenAIName != nil {
				newRecord.OpenAIName = *req.OpenAIName
			}
			if req.Timeout != nil {
				newRecord.Timeout = *req.Timeout
			}
			if req.MaxTokens != nil {
				newRecord.MaxTokens = *req.MaxTokens
			}
			if req.SystemPrompt != nil {
				newRecord.SystemPrompt = *req.SystemPrompt
			}
			if err := store.CreateLLMToolRouter(newRecord); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			record = newRecord
		} else {
			// 更新现有配置
			if err := store.UpdateLLMToolRouter(record.ID, updates); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			record, err = store.GetLLMToolRouter()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "item": llmToolRouterDTO(*record)})
	}
}

func llmToolRouterDTO(row llmToolRouterRecord) map[string]any {
	return map[string]any{
		"id":            row.ID,
		"enabled":       row.Enabled,
		"openai_name":   row.OpenAIName,
		"timeout":       row.Timeout,
		"max_tokens":    row.MaxTokens,
		"system_prompt": row.SystemPrompt,
		"created_at":    row.CreatedAt,
		"updated_at":    row.UpdatedAt,
	}
}

// ============================================
// LLM Primary Config Handlers
// ============================================

func handleGetLLMPrimaryConfig(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		record, err := store.GetLLMPrimaryConfig()
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "primary config not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"item": llmPrimaryConfigDTO(*record)})
	}
}

func handleUpdateLLMPrimaryConfig(store *store) gin.HandlerFunc {
	return func(c *gin.Context) {
		record, err := store.GetLLMPrimaryConfig()
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var req struct {
			Enabled      *bool   `json:"enabled"`
			ProviderName *string `json:"provider_name"`
			Timeout      *int    `json:"timeout"`
			MaxTokens    *int    `json:"max_tokens"`
			SystemPrompt *string `json:"system_prompt"`
			EnableTool   *bool   `json:"enable_tool"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		updates := make(map[string]any)
		if req.Enabled != nil {
			updates["enabled"] = *req.Enabled
		}
		if req.ProviderName != nil {
			updates["provider_name"] = *req.ProviderName
		}
		if req.Timeout != nil {
			updates["timeout"] = *req.Timeout
		}
		if req.MaxTokens != nil {
			updates["max_tokens"] = *req.MaxTokens
		}
		if req.SystemPrompt != nil {
			updates["system_prompt"] = *req.SystemPrompt
		}
		if req.EnableTool != nil {
			updates["enable_tool"] = *req.EnableTool
		}

		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
			return
		}

		if record == nil {
			// 创建新配置
			newRecord := &llmPrimaryConfigRecord{
				Enabled:      req.Enabled != nil && *req.Enabled,
				ProviderName: "",
				Timeout:      120,
				MaxTokens:    1024,
				SystemPrompt: "",
				EnableTool:   false,
			}
			if req.ProviderName != nil {
				newRecord.ProviderName = *req.ProviderName
			}
			if req.Timeout != nil {
				newRecord.Timeout = *req.Timeout
			}
			if req.MaxTokens != nil {
				newRecord.MaxTokens = *req.MaxTokens
			}
			if req.SystemPrompt != nil {
				newRecord.SystemPrompt = *req.SystemPrompt
			}
			if req.EnableTool != nil {
				newRecord.EnableTool = *req.EnableTool
			}
			if err := store.CreateLLMPrimaryConfig(newRecord); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			record = newRecord
		} else {
			// 更新现有配置
			if err := store.UpdateLLMPrimaryConfig(record.ID, updates); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			record, err = store.GetLLMPrimaryConfig()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "item": llmPrimaryConfigDTO(*record)})
	}
}

func llmPrimaryConfigDTO(row llmPrimaryConfigRecord) map[string]any {
	return map[string]any{
		"id":             row.ID,
		"enabled":        row.Enabled,
		"provider_name":  row.ProviderName,
		"timeout":        row.Timeout,
		"max_tokens":     row.MaxTokens,
		"system_prompt":  row.SystemPrompt,
		"enable_tool":    row.EnableTool,
		"created_at":     row.CreatedAt,
		"updated_at":     row.UpdatedAt,
	}
}
