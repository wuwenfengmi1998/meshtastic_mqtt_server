package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func registerAdminLLMRoutes(r *gin.RouterGroup, store *store) {
	group := r.Group("/llm")
	{
		group.GET("/messages", handleListLLMMessages(store))
		group.GET("/messages/:id", handleGetLLMMessage(store))
		group.PUT("/messages/:id/status", handleUpdateLLMMessageStatus(store))
		group.DELETE("/messages/:id", handleDeleteLLMMessage(store))
		group.DELETE("/bots/:bot_id/messages", handleDeleteLLMMessagesByBot(store))
		group.POST("/messages/cleanup", handleCleanupDeletedLLMMessages(store))
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
