package main

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type botNodeRequest struct {
	NodeNum                          *int64 `json:"node_num"`
	LongName                         string `json:"long_name"`
	ShortName                        string `json:"short_name"`
	Enabled                          bool   `json:"enabled"`
	DefaultChannelID                 string `json:"default_channel_id"`
	TopicPrefix                      string `json:"topic_prefix"`
	PSK                              string `json:"psk"`
	NodeInfoBroadcastEnabled         bool   `json:"nodeinfo_broadcast_enabled"`
	NodeInfoBroadcastIntervalSeconds int64  `json:"nodeinfo_broadcast_interval_seconds"`
	LLMQueueEnabled                  bool   `json:"llm_queue_enabled"`
	LLMIncludeChannelMessages        bool   `json:"llm_include_channel_messages"`
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
	r.POST("/bot/nodes/:id/keys/regenerate", func(c *gin.Context) {
		id, ok := parseBotID(c, "invalid bot node id")
		if !ok {
			return
		}
		row, err := store.RegenerateBotNodeKeys(id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bot node not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": botNodeDTO(*row)})
	})
	r.POST("/bot/nodes/:id/nodeinfo", func(c *gin.Context) {
		if sender == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "bot sender is not configured"})
			return
		}
		id, ok := parseBotID(c, "invalid bot node id")
		if !ok {
			return
		}
		row, err := sender.PublishNodeInfoByID(c.Request.Context(), id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bot node not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": botNodeDTO(*row)})
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
	r.GET("/bot/direct-messages", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		botID, err := strconv.ParseUint(c.Query("bot_id"), 10, 64)
		if err != nil || botID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot id"})
			return
		}
		if _, err := store.GetBotNode(botID); errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bot node not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		target, err := strconv.ParseInt(c.Query("target_node_num"), 10, 64)
		if err != nil || target <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target node num"})
			return
		}
		dmOpts := botDirectMessageListOptions{listOptions: opts, BotID: botID, PeerNodeNum: target, Direction: c.Query("direction")}
		rows, err := store.ListBotDirectMessagesByConversation(dmOpts)
		if err != nil {
			writeListResponse(c, rows, opts, err, botDirectMessageDTO)
			return
		}
		total, err := store.CountBotDirectMessagesByConversation(dmOpts)
		writeListResponseWithTotal(c, rows, opts, total, err, botDirectMessageDTO)
	})
	// /bot/conversations 返回某个 bot 下所有会话的概要（最后一条消息 + 未读数），
	// 给前端侧边栏渲染会话列表使用。
	r.GET("/bot/conversations", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		botID, err := strconv.ParseUint(c.Query("bot_id"), 10, 64)
		if err != nil || botID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot id"})
			return
		}
		if _, err := store.GetBotNode(botID); errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bot node not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		rows, err := store.ListBotDirectConversations(botID, opts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		unread, err := store.CountBotDirectUnread(botID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items := make([]gin.H, 0, len(rows))
		for _, row := range rows {
			items = append(items, botDirectConversationDTO(row))
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "limit": opts.Limit, "offset": opts.Offset, "unread_total": unread})
	})
	// /bot/direct-messages/read 把指定 (bot, peer) 下所有未读 inbound 消息标记为已读。
	r.POST("/bot/direct-messages/read", func(c *gin.Context) {
		var req struct {
			BotID       uint64 `json:"bot_id"`
			PeerNodeNum int64  `json:"peer_node_num"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mark-read request"})
			return
		}
		if req.BotID == 0 || req.PeerNodeNum == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bot_id and peer_node_num are required"})
			return
		}
		if _, err := store.GetBotNode(req.BotID); errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bot node not found"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updated, err := store.MarkBotDirectMessagesRead(req.BotID, req.PeerNodeNum)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"updated": updated})
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
	return botNodeInput{NodeNum: req.NodeNum, LongName: req.LongName, ShortName: req.ShortName, Enabled: req.Enabled, DefaultChannelID: req.DefaultChannelID, TopicPrefix: req.TopicPrefix, PSK: req.PSK, NodeInfoBroadcastEnabled: req.NodeInfoBroadcastEnabled, NodeInfoBroadcastIntervalSeconds: req.NodeInfoBroadcastIntervalSeconds, LLMQueueEnabled: req.LLMQueueEnabled, LLMIncludeChannelMessages: req.LLMIncludeChannelMessages}
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
	return gin.H{"id": row.ID, "node_id": row.NodeID, "node_num": row.NodeNum, "long_name": row.LongName, "short_name": row.ShortName, "enabled": row.Enabled, "default_channel_id": row.DefaultChannelID, "topic_prefix": row.TopicPrefix, "psk": row.PSK, "public_key": row.PublicKey, "private_key_set": row.PrivateKey != "", "nodeinfo_broadcast_enabled": row.NodeInfoBroadcastEnabled, "nodeinfo_broadcast_interval_seconds": row.NodeInfoBroadcastIntervalSeconds, "last_nodeinfo_broadcast_at": row.LastNodeInfoBroadcastAt, "llm_queue_enabled": row.LLMQueueEnabled, "llm_include_channel_messages": row.LLMIncludeChannelMessages, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}

func botMessageDTO(row botMessageRecord) gin.H {
	return gin.H{"id": row.ID, "bot_id": row.BotID, "bot_node_id": row.BotNodeID, "bot_node_num": row.BotNodeNum, "message_type": row.MessageType, "channel_id": row.ChannelID, "to_node_id": row.ToNodeID, "to_node_num": row.ToNodeNum, "topic": row.Topic, "packet_id": row.PacketID, "text": row.Text, "payload_len": row.PayloadLen, "encrypted": row.Encrypted, "status": row.Status, "error": row.Error, "published_at": row.PublishedAt, "created_by": row.CreatedBy, "created_at": row.CreatedAt}
}

func botDirectMessageDTO(row botDirectMessageRecord) gin.H {
	return gin.H{
		"id":             row.ID,
		"bot_id":         row.BotID,
		"bot_node_id":    row.BotNodeID,
		"bot_node_num":   row.BotNodeNum,
		"peer_node_id":   row.PeerNodeID,
		"peer_node_num":  row.PeerNodeNum,
		"direction":      row.Direction,
		"topic":          row.Topic,
		"packet_id":      row.PacketID,
		"text":           row.Text,
		"payload_len":    row.PayloadLen,
		"pki_encrypted":  row.PKIEncrypted,
		"want_ack":       row.WantAck,
		"gateway_id":     row.GatewayID,
		"status":         row.Status,
		"error":          row.Error,
		"bot_message_id": row.BotMessageID,
		"created_by":     row.CreatedBy,
		"published_at":   row.PublishedAt,
		"received_at":    row.ReceivedAt,
		"read_at":        row.ReadAt,
		"created_at":     row.CreatedAt,
	}
}

func botDirectConversationDTO(row botDirectConversation) gin.H {
	return gin.H{
		"bot_id":          row.BotID,
		"peer_node_id":    row.PeerNodeID,
		"peer_node_num":   row.PeerNodeNum,
		"last_message_at": row.LastMessageAt,
		"last_text":       row.LastText,
		"last_direction":  row.LastDirection,
		"unread_count":    row.UnreadCount,
		"total_count":     row.TotalCount,
	}
}
