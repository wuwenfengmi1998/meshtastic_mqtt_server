package main

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type mqttForwarderRequest struct {
	Name                string  `json:"name"`
	Enabled             bool    `json:"enabled"`
	SourceHost          string  `json:"source_host"`
	SourcePort          int     `json:"source_port"`
	SourceUsername      string  `json:"source_username"`
	SourcePassword      *string `json:"source_password"`
	SourcePasswordClear bool    `json:"source_password_clear"`
	SourceClientID      string  `json:"source_client_id"`
	SourceTLS           bool    `json:"source_tls"`
	TargetHost          string  `json:"target_host"`
	TargetPort          int     `json:"target_port"`
	TargetUsername      string  `json:"target_username"`
	TargetPassword      *string `json:"target_password"`
	TargetPasswordClear bool    `json:"target_password_clear"`
	TargetClientID      string  `json:"target_client_id"`
	TargetTLS           bool    `json:"target_tls"`
}

type mqttForwardTopicRequest struct {
	Topic        string `json:"topic"`
	Enabled      bool   `json:"enabled"`
	Direction    string `json:"direction"`
	SourcePrefix string `json:"source_prefix"`
	TargetPrefix string `json:"target_prefix"`
	QoS          int    `json:"qos"`
	Retain       bool   `json:"retain"`
}

func registerAdminMQTTForwardRoutes(r gin.IRouter, store *store, forwarder mqttForwardReloader) {
	r.GET("/mqtt-forward/forwarders", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListMQTTForwarders(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, mqttForwarderDTO)
			return
		}
		total, err := store.CountMQTTForwarders(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, mqttForwarderDTO)
	})
	r.POST("/mqtt-forward/forwarders", func(c *gin.Context) {
		var req mqttForwarderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mqtt forwarder request"})
			return
		}
		input := mqttForwarderInputFromRequest(req)
		row, err := store.CreateMQTTForwarder(input)
		writeMQTTForwardMutationResponse(c, http.StatusCreated, row, err, func() error {
			return reloadMQTTForwarder(forwarder, row.ID)
		})
	})
	r.PUT("/mqtt-forward/forwarders/:id", func(c *gin.Context) {
		id, ok := parseMQTTForwardID(c, "invalid mqtt forwarder id")
		if !ok {
			return
		}
		var req mqttForwarderRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mqtt forwarder request"})
			return
		}
		input := mqttForwarderInputFromRequest(req)
		row, err := store.UpdateMQTTForwarder(id, input)
		writeMQTTForwardMutationResponse(c, http.StatusOK, row, err, func() error {
			return reloadMQTTForwarder(forwarder, id)
		})
	})
	r.DELETE("/mqtt-forward/forwarders/:id", func(c *gin.Context) {
		id, ok := parseMQTTForwardID(c, "invalid mqtt forwarder id")
		if !ok {
			return
		}
		if forwarder != nil {
			forwarder.StopForwarder(id)
		}
		writeMQTTForwardDeleteResponse(c, store.DeleteMQTTForwarder(id), nil)
	})
	r.POST("/mqtt-forward/forwarders/:id/restart", func(c *gin.Context) {
		id, ok := parseMQTTForwardID(c, "invalid mqtt forwarder id")
		if !ok {
			return
		}
		if err := reloadMQTTForwarder(forwarder, id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/mqtt-forward/forwarders/:id/topics", func(c *gin.Context) {
		id, ok := parseMQTTForwardID(c, "invalid mqtt forwarder id")
		if !ok {
			return
		}
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListMQTTForwardTopics(id, opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, mqttForwardTopicDTO)
			return
		}
		total, err := store.CountMQTTForwardTopics(id)
		writeListResponseWithTotal(c, rows, opts, total, err, mqttForwardTopicDTO)
	})
	r.POST("/mqtt-forward/forwarders/:id/topics", func(c *gin.Context) {
		id, ok := parseMQTTForwardID(c, "invalid mqtt forwarder id")
		if !ok {
			return
		}
		var req mqttForwardTopicRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mqtt forward topic request"})
			return
		}
		row, err := store.CreateMQTTForwardTopic(id, mqttForwardTopicInputFromRequest(req))
		writeMQTTForwardTopicMutationResponse(c, http.StatusCreated, row, err, func() error {
			return reloadMQTTForwarder(forwarder, id)
		})
	})
	r.PUT("/mqtt-forward/topics/:id", func(c *gin.Context) {
		id, ok := parseMQTTForwardID(c, "invalid mqtt forward topic id")
		if !ok {
			return
		}
		var req mqttForwardTopicRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mqtt forward topic request"})
			return
		}
		row, err := store.UpdateMQTTForwardTopic(id, mqttForwardTopicInputFromRequest(req))
		writeMQTTForwardTopicMutationResponse(c, http.StatusOK, row, err, func() error {
			return reloadMQTTForwarder(forwarder, row.ForwarderID)
		})
	})
	r.DELETE("/mqtt-forward/topics/:id", func(c *gin.Context) {
		id, ok := parseMQTTForwardID(c, "invalid mqtt forward topic id")
		if !ok {
			return
		}
		row, err := store.GetMQTTForwardTopic(id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "mqtt forward topic not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		parentID := row.ForwarderID
		writeMQTTForwardDeleteResponse(c, store.DeleteMQTTForwardTopic(id), func() error {
			return reloadMQTTForwarder(forwarder, parentID)
		})
	})
	r.GET("/mqtt-forward/status", func(c *gin.Context) {
		items := []mqttForwardRuntimeStatus{}
		if forwarder != nil {
			items = forwarder.Status()
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	})
}

func mqttForwarderInputFromRequest(req mqttForwarderRequest) mqttForwarderInput {
	sourcePassword := req.SourcePassword
	if req.SourcePasswordClear {
		empty := ""
		sourcePassword = &empty
	}
	targetPassword := req.TargetPassword
	if req.TargetPasswordClear {
		empty := ""
		targetPassword = &empty
	}
	return mqttForwarderInput{Name: req.Name, Enabled: req.Enabled, SourceHost: req.SourceHost, SourcePort: req.SourcePort, SourceUsername: req.SourceUsername, SourcePassword: sourcePassword, SourceClientID: req.SourceClientID, SourceTLS: req.SourceTLS, TargetHost: req.TargetHost, TargetPort: req.TargetPort, TargetUsername: req.TargetUsername, TargetPassword: targetPassword, TargetClientID: req.TargetClientID, TargetTLS: req.TargetTLS}
}

func mqttForwardTopicInputFromRequest(req mqttForwardTopicRequest) mqttForwardTopicInput {
	return mqttForwardTopicInput{Topic: req.Topic, Enabled: req.Enabled, Direction: req.Direction, SourcePrefix: req.SourcePrefix, TargetPrefix: req.TargetPrefix, QoS: req.QoS, Retain: req.Retain}
}

func parseMQTTForwardID(c *gin.Context, message string) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": message})
		return 0, false
	}
	return id, true
}

func reloadMQTTForwarder(forwarder mqttForwardReloader, id uint64) error {
	if forwarder == nil {
		return nil
	}
	return forwarder.ReloadForwarder(id)
}

func writeMQTTForwardMutationResponse(c *gin.Context, status int, row *mqttForwarderRecord, err error, afterSuccess func() error) {
	if errors.Is(err, errMQTTForwarderAlreadyExists) {
		c.JSON(http.StatusConflict, gin.H{"error": "mqtt forwarder already exists"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "mqtt forwarder not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if afterSuccess != nil {
		if err := afterSuccess(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "mqtt forwarder saved but reload failed: " + err.Error()})
			return
		}
	}
	c.JSON(status, gin.H{"item": mqttForwarderDTO(*row)})
}

func writeMQTTForwardTopicMutationResponse(c *gin.Context, status int, row *mqttForwardTopicRecord, err error, afterSuccess func() error) {
	if errors.Is(err, errMQTTForwardTopicAlreadyExists) {
		c.JSON(http.StatusConflict, gin.H{"error": "mqtt forward topic already exists"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "mqtt forward topic not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if afterSuccess != nil {
		if err := afterSuccess(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "mqtt forward topic saved but reload failed: " + err.Error()})
			return
		}
	}
	c.JSON(status, gin.H{"item": mqttForwardTopicDTO(*row)})
}

func writeMQTTForwardDeleteResponse(c *gin.Context, err error, afterSuccess func() error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "mqtt forward item not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if afterSuccess != nil {
		if err := afterSuccess(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "mqtt forward item deleted but reload failed: " + err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func mqttForwarderDTO(row mqttForwarderRecord) gin.H {
	return gin.H{"id": row.ID, "name": row.Name, "enabled": row.Enabled, "source_host": row.SourceHost, "source_port": row.SourcePort, "source_username": row.SourceUsername, "source_password_set": row.SourcePassword != "", "source_client_id": row.SourceClientID, "source_tls": row.SourceTLS, "target_host": row.TargetHost, "target_port": row.TargetPort, "target_username": row.TargetUsername, "target_password_set": row.TargetPassword != "", "target_client_id": row.TargetClientID, "target_tls": row.TargetTLS, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}

func mqttForwardTopicDTO(row mqttForwardTopicRecord) gin.H {
	return gin.H{"id": row.ID, "forwarder_id": row.ForwarderID, "topic": row.Topic, "enabled": row.Enabled, "direction": row.Direction, "source_prefix": row.SourcePrefix, "target_prefix": row.TargetPrefix, "qos": row.QoS, "retain": row.Retain, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}
