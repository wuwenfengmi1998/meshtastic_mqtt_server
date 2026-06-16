package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type signRequest struct {
	NodeID    string `json:"node_id"`
	LongName  string `json:"long_name"`
	ShortName string `json:"short_name"`
	SignText  string `json:"sign_text"`
	SignTime  string `json:"sign_time"`
}

func registerAdminSignRoutes(r gin.IRouter, store *store) {
	r.GET("/signs", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListSigns(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, signDTO)
			return
		}
		total, err := store.CountSigns(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, signDTO)
	})
	r.POST("/signs", func(c *gin.Context) {
		var req signRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sign request"})
			return
		}
		signTime, ok := parseSignRequestTime(c, req.SignTime)
		if !ok {
			return
		}
		row, err := store.CreateSign(req.NodeID, nullableString(req.LongName), nullableString(req.ShortName), req.SignText, signTime)
		writeSignMutationResponse(c, http.StatusCreated, row, err)
	})
	r.PUT("/signs/:id", func(c *gin.Context) {
		id, ok := parseSignID(c)
		if !ok {
			return
		}
		var req signRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sign request"})
			return
		}
		signTime, ok := parseSignRequestTime(c, req.SignTime)
		if !ok {
			return
		}
		row, err := store.UpdateSign(id, req.NodeID, nullableString(req.LongName), nullableString(req.ShortName), req.SignText, signTime)
		writeSignMutationResponse(c, http.StatusOK, row, err)
	})
	r.DELETE("/signs/:id", func(c *gin.Context) {
		id, ok := parseSignID(c)
		if !ok {
			return
		}
		err := store.DeleteSign(id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "sign record not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

func parseSignID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sign id"})
		return 0, false
	}
	return id, true
}

func parseSignRequestTime(c *gin.Context, value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sign_time: use RFC3339"})
		return time.Time{}, false
	}
	return parsed, true
}

func writeSignMutationResponse(c *gin.Context, status int, row *signRecord, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "sign record not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, gin.H{"item": signDTO(*row)})
}
