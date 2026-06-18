// Package sign 提供签到记录的 admin 路由与对应的 DTO/列表查询。
//
// 拆离自原来 main 包的 admin_sign_routes.go 与 web.go 中的 signDTO /
// signDayCountDTO；其它 admin 路由也通过 SignDTO / SignDayCountDTO 复用。
package sign

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	storepkg "meshtastic_mqtt_server/internal/store"
	"meshtastic_mqtt_server/internal/webutil"
)

type signRequest struct {
	NodeID    string `json:"node_id"`
	LongName  string `json:"long_name"`
	ShortName string `json:"short_name"`
	SignText  string `json:"sign_text"`
	SignTime  string `json:"sign_time"`
}

// RegisterAdminRoutes 在 admin 路由组下挂 sign CRUD 端点。
func RegisterAdminRoutes(r gin.IRouter, store *storepkg.Store) {
	r.GET("/signs", func(c *gin.Context) {
		opts, ok := webutil.ParseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListSigns(opts)
		if err != nil {
			webutil.WriteListResponse(c, rows, opts, err, SignDTO)
			return
		}
		total, err := store.CountSigns(opts)
		webutil.WriteListResponseWithTotal(c, rows, opts, total, err, SignDTO)
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
		row, err := store.CreateSign(req.NodeID, storepkg.NullableString(req.LongName), storepkg.NullableString(req.ShortName), req.SignText, signTime)
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
		row, err := store.UpdateSign(id, req.NodeID, storepkg.NullableString(req.LongName), storepkg.NullableString(req.ShortName), req.SignText, signTime)
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

func writeSignMutationResponse(c *gin.Context, status int, row *storepkg.SignRecord, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "sign record not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, gin.H{"item": SignDTO(*row)})
}

// SignDTO 把 SignRecord 转成给前端的视图。
func SignDTO(row storepkg.SignRecord) gin.H {
	return gin.H{"id": row.ID, "node_id": row.NodeID, "long_name": webutil.PtrString(row.LongName), "short_name": webutil.PtrString(row.ShortName), "sign_text": row.SignText, "sign_time": row.SignTime}
}

// SignDayCountDTO 把按日聚合的签到数量转成视图。
func SignDayCountDTO(row storepkg.SignDayCount) gin.H {
	return gin.H{"date": row.Date, "count": row.Count}
}
