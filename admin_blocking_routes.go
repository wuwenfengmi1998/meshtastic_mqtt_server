package main

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type nodeBlockingRequest struct {
	NodeID  string `json:"node_id"`
	NodeNum *int64 `json:"node_num"`
	Reason  string `json:"reason"`
	Enabled bool   `json:"enabled"`
}

type ipBlockingRequest struct {
	IPValue string `json:"ip_value"`
	Reason  string `json:"reason"`
	Enabled bool   `json:"enabled"`
}

type forbiddenWordBlockingRequest struct {
	Word          string `json:"word"`
	MatchType     string `json:"match_type"`
	CaseSensitive bool   `json:"case_sensitive"`
	Reason        string `json:"reason"`
	Enabled       bool   `json:"enabled"`
}

func registerAdminBlockingRoutes(r gin.IRouter, store *store) {
	r.GET("/blocking/nodes", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListNodeBlocking(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, nodeBlockingDTO)
			return
		}
		total, err := store.CountNodeBlocking(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, nodeBlockingDTO)
	})
	r.POST("/blocking/nodes", func(c *gin.Context) {
		var req nodeBlockingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node blocking request"})
			return
		}
		row, err := store.CreateNodeBlocking(req.NodeID, req.NodeNum, req.Reason, req.Enabled)
		writeBlockingMutationResponse(c, http.StatusCreated, row, err, nodeBlockingDTO)
	})
	r.PUT("/blocking/nodes/:id", func(c *gin.Context) {
		id, ok := parseBlockingID(c)
		if !ok {
			return
		}
		var req nodeBlockingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node blocking request"})
			return
		}
		row, err := store.UpdateNodeBlocking(id, req.NodeID, req.NodeNum, req.Reason, req.Enabled)
		writeBlockingMutationResponse(c, http.StatusOK, row, err, nodeBlockingDTO)
	})
	r.DELETE("/blocking/nodes/:id", func(c *gin.Context) {
		id, ok := parseBlockingID(c)
		if !ok {
			return
		}
		writeBlockingDeleteResponse(c, store.DeleteNodeBlocking(id))
	})

	r.GET("/blocking/ips", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListIPBlocking(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, ipBlockingDTO)
			return
		}
		total, err := store.CountIPBlocking(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, ipBlockingDTO)
	})
	r.POST("/blocking/ips", func(c *gin.Context) {
		var req ipBlockingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ip blocking request"})
			return
		}
		row, err := store.CreateIPBlocking(req.IPValue, req.Reason, req.Enabled)
		writeBlockingMutationResponse(c, http.StatusCreated, row, err, ipBlockingDTO)
	})
	r.PUT("/blocking/ips/:id", func(c *gin.Context) {
		id, ok := parseBlockingID(c)
		if !ok {
			return
		}
		var req ipBlockingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ip blocking request"})
			return
		}
		row, err := store.UpdateIPBlocking(id, req.IPValue, req.Reason, req.Enabled)
		writeBlockingMutationResponse(c, http.StatusOK, row, err, ipBlockingDTO)
	})
	r.DELETE("/blocking/ips/:id", func(c *gin.Context) {
		id, ok := parseBlockingID(c)
		if !ok {
			return
		}
		writeBlockingDeleteResponse(c, store.DeleteIPBlocking(id))
	})

	r.GET("/blocking/words", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListForbiddenWordBlocking(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, forbiddenWordBlockingDTO)
			return
		}
		total, err := store.CountForbiddenWordBlocking(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, forbiddenWordBlockingDTO)
	})
	r.POST("/blocking/words", func(c *gin.Context) {
		var req forbiddenWordBlockingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid forbidden word blocking request"})
			return
		}
		row, err := store.CreateForbiddenWordBlocking(req.Word, req.MatchType, req.CaseSensitive, req.Reason, req.Enabled)
		writeBlockingMutationResponse(c, http.StatusCreated, row, err, forbiddenWordBlockingDTO)
	})
	r.PUT("/blocking/words/:id", func(c *gin.Context) {
		id, ok := parseBlockingID(c)
		if !ok {
			return
		}
		var req forbiddenWordBlockingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid forbidden word blocking request"})
			return
		}
		row, err := store.UpdateForbiddenWordBlocking(id, req.Word, req.MatchType, req.CaseSensitive, req.Reason, req.Enabled)
		writeBlockingMutationResponse(c, http.StatusOK, row, err, forbiddenWordBlockingDTO)
	})
	r.DELETE("/blocking/words/:id", func(c *gin.Context) {
		id, ok := parseBlockingID(c)
		if !ok {
			return
		}
		writeBlockingDeleteResponse(c, store.DeleteForbiddenWordBlocking(id))
	})
}

func parseBlockingID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid blocking rule id"})
		return 0, false
	}
	return id, true
}

func writeBlockingMutationResponse[T any](c *gin.Context, status int, row *T, err error, convert func(T) gin.H) {
	if errors.Is(err, errBlockingAlreadyExists) {
		c.JSON(http.StatusConflict, gin.H{"error": "blocking rule already exists"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "blocking rule not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, gin.H{"item": convert(*row)})
}

func writeBlockingDeleteResponse(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "blocking rule not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func nodeBlockingDTO(row nodeBlockingRecord) gin.H {
	return gin.H{"id": row.ID, "node_id": row.NodeID, "node_num": ptrInt64(row.NodeNum), "reason": row.Reason, "enabled": row.Enabled, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}

func ipBlockingDTO(row ipBlockingRecord) gin.H {
	return gin.H{"id": row.ID, "ip_value": row.IPValue, "reason": row.Reason, "enabled": row.Enabled, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}

func forbiddenWordBlockingDTO(row forbiddenWordBlockingRecord) gin.H {
	return gin.H{"id": row.ID, "word": row.Word, "match_type": row.MatchType, "case_sensitive": row.CaseSensitive, "reason": row.Reason, "enabled": row.Enabled, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}
