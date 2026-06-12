package main

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type botNodeRequest struct {
	NodeNum          *int64 `json:"node_num"`
	LongName         string `json:"long_name"`
	ShortName        string `json:"short_name"`
	Enabled          bool   `json:"enabled"`
	DefaultChannelID string `json:"default_channel_id"`
	TopicPrefix      string `json:"topic_prefix"`
}

type botSendMessageRequest struct {
	BotID       uint64 `json:"bot_id"`
	MessageType string `json:"message_type"`
	ChannelID   string `json:"channel_id"`
	ToNodeID    string `json:"to_node_id"`
	ToNodeNum   *int64 `json:"to_node_num"`
	Text        string `json:"text"`
}

func registerAdminBotRoutes(r gin.IRouter, store *store, sender botTextSender) {
	r.GET("/bot/nodes", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListBotNodes(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, botNodeDTO)
			return
		}
		total, err := store.CountBotNodes(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, botNodeDTO)
	})
	r.POST("/bot/nodes", func(c *gin.Context) {
		var req botNodeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot node request"})
			return
		}
		row, err := store.CreateBotNode(botNodeInputFromRequest(req))
		writeBotNodeMutationResponse(c, http.StatusCreated, row, err)
	})
	r.PUT("/bot/nodes/:id", func(c *gin.Context) {
		id, ok := parseBotID(c, "invalid bot node id")
		if !ok {
			return
		}
		var req botNodeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot node request"})
			return
		}
		row, err := store.UpdateBotNode(id, botNodeInputFromRequest(req))
		writeBotNodeMutationResponse(c, http.StatusOK, row, err)
	})
	r.DELETE("/bot/nodes/:id", func(c *gin.Context) {
		id, ok := parseBotID(c, "invalid bot node id")
		if !ok {
			return
		}
		if err := store.DeleteBotNode(id); errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bot node not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/bot/messages", func(c *gin.Context) {
		opts, ok := parseBotMessageListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListBotMessages(opts)
		if err != nil {
			writeListResponse(c, rows, opts.listOptions, err, botMessageDTO)
			return
		}
		total, err := store.CountBotMessages(opts)
		writeListResponseWithTotal(c, rows, opts.listOptions, total, err, botMessageDTO)
	})
	r.POST("/bot/messages", func(c *gin.Context) {
		if sender == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "bot sender is not configured"})
			return
		}
		var req botSendMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot message request"})
			return
		}
		claims := c.MustGet("admin_claims").(*sessionClaims)
		row, err := sender.SendText(c.Request.Context(), botSendTextRequest{BotID: req.BotID, MessageType: req.MessageType, ChannelID: req.ChannelID, ToNodeID: req.ToNodeID, ToNodeNum: req.ToNodeNum, Text: req.Text, CreatedBy: claims.Username})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bot node not found"})
			return
		}
		if err != nil {
			status := http.StatusBadRequest
			if row != nil && row.ID != 0 {
				c.JSON(http.StatusAccepted, gin.H{"item": botMessageDTO(*row), "error": err.Error()})
				return
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"item": botMessageDTO(*row)})
	})
}

func botNodeInputFromRequest(req botNodeRequest) botNodeInput {
	return botNodeInput{NodeNum: req.NodeNum, LongName: req.LongName, ShortName: req.ShortName, Enabled: req.Enabled, DefaultChannelID: req.DefaultChannelID, TopicPrefix: req.TopicPrefix}
}

func parseBotID(c *gin.Context, message string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": message})
		return 0, false
	}
	return id, true
}

func parseBotMessageListOptions(c *gin.Context) (botMessageListOptions, bool) {
	listOpts, ok := parseListOptions(c)
	if !ok {
		return botMessageListOptions{}, false
	}
	opts := botMessageListOptions{listOptions: listOpts, MessageType: c.Query("message_type"), ChannelID: c.Query("channel_id")}
	if value := c.Query("bot_id"); value != "" {
		id, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot id"})
			return botMessageListOptions{}, false
		}
		opts.BotID = id
	}
	return opts, true
}

func writeBotNodeMutationResponse(c *gin.Context, status int, row *botNodeRecord, err error) {
	if errors.Is(err, errBotNodeAlreadyExists) {
		c.JSON(http.StatusConflict, gin.H{"error": "bot node already exists or conflicts with existing node"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "bot node not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, gin.H{"item": botNodeDTO(*row)})
}

func botNodeDTO(row botNodeRecord) gin.H {
	return gin.H{"id": row.ID, "node_id": row.NodeID, "node_num": row.NodeNum, "long_name": row.LongName, "short_name": row.ShortName, "enabled": row.Enabled, "default_channel_id": row.DefaultChannelID, "topic_prefix": row.TopicPrefix, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}

func botMessageDTO(row botMessageRecord) gin.H {
	return gin.H{"id": row.ID, "bot_id": row.BotID, "bot_node_id": row.BotNodeID, "bot_node_num": row.BotNodeNum, "message_type": row.MessageType, "channel_id": row.ChannelID, "to_node_id": row.ToNodeID, "to_node_num": row.ToNodeNum, "topic": row.Topic, "packet_id": row.PacketID, "text": row.Text, "payload_len": row.PayloadLen, "encrypted": row.Encrypted, "status": row.Status, "error": row.Error, "published_at": row.PublishedAt, "created_by": row.CreatedBy, "created_at": row.CreatedAt}
}
