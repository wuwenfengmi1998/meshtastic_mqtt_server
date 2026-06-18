// Package mapsource 提供地图瓦片源的 admin 与公开路由。
package mapsource

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	storepkg "meshtastic_mqtt_server/internal/store"
	"meshtastic_mqtt_server/internal/webutil"
)

type mapTileSourceRequest struct {
	Name         string `json:"name"`
	URLTemplate  string `json:"url_template"`
	Attribution  string `json:"attribution"`
	MaxZoom      int    `json:"max_zoom"`
	Enabled      bool   `json:"enabled"`
	IsDefault    bool   `json:"is_default"`
	ProxyEnabled bool   `json:"proxy_enabled"`
}

// RegisterPublicRoutes 把对外可见的 GET /map-source/{default,enabled} 挂上去。
func RegisterPublicRoutes(r gin.IRouter, store *storepkg.Store) {
	r.GET("/map-source/default", func(c *gin.Context) {
		row, err := store.GetDefaultMapTileSource()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": PublicDTO(*row)})
	})
	r.GET("/map-source/enabled", func(c *gin.Context) {
		rows, err := store.ListEnabledMapTileSources()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		items := make([]gin.H, 0, len(rows))
		for _, row := range rows {
			items = append(items, PublicDTO(row))
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	})
}

// RegisterAdminRoutes 注册管理员侧 CRUD 与设默认。
func RegisterAdminRoutes(r gin.IRouter, store *storepkg.Store) {
	r.GET("/map-source", func(c *gin.Context) {
		opts, ok := webutil.ParseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListMapTileSources(opts)
		if err != nil {
			webutil.WriteListResponse(c, rows, opts, err, AdminDTO)
			return
		}
		total, err := store.CountMapTileSources(opts)
		webutil.WriteListResponseWithTotal(c, rows, opts, total, err, AdminDTO)
	})
	r.POST("/map-source", func(c *gin.Context) {
		var req mapTileSourceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid map source request"})
			return
		}
		row, err := store.CreateMapTileSource(mapTileSourceInputFromRequest(req))
		writeMapTileSourceMutationResponse(c, http.StatusCreated, row, err)
	})
	r.PUT("/map-source/:id", func(c *gin.Context) {
		id, ok := parseMapTileSourceID(c)
		if !ok {
			return
		}
		var req mapTileSourceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid map source request"})
			return
		}
		row, err := store.UpdateMapTileSource(id, mapTileSourceInputFromRequest(req))
		writeMapTileSourceMutationResponse(c, http.StatusOK, row, err)
	})
	r.DELETE("/map-source/:id", func(c *gin.Context) {
		id, ok := parseMapTileSourceID(c)
		if !ok {
			return
		}
		writeMapTileSourceDeleteResponse(c, store.DeleteMapTileSource(id))
	})
	r.POST("/map-source/:id/default", func(c *gin.Context) {
		id, ok := parseMapTileSourceID(c)
		if !ok {
			return
		}
		row, err := store.SetDefaultMapTileSource(id)
		writeMapTileSourceMutationResponse(c, http.StatusOK, row, err)
	})
}

func mapTileSourceInputFromRequest(req mapTileSourceRequest) storepkg.MapTileSourceInput {
	return storepkg.MapTileSourceInput{
		Name:         req.Name,
		URLTemplate:  req.URLTemplate,
		Attribution:  req.Attribution,
		MaxZoom:      req.MaxZoom,
		Enabled:      req.Enabled,
		IsDefault:    req.IsDefault,
		ProxyEnabled: req.ProxyEnabled,
	}
}

func parseMapTileSourceID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid map source id"})
		return 0, false
	}
	return id, true
}

func writeMapTileSourceMutationResponse(c *gin.Context, status int, row *storepkg.MapTileSourceRecord, err error) {
	if errors.Is(err, storepkg.ErrMapTileSourceAlreadyExists) {
		c.JSON(http.StatusConflict, gin.H{"error": "map source already exists"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "map source not found"})
		return
	}
	if errors.Is(err, storepkg.ErrMapTileSourceCannotDeleteDefault) || errors.Is(err, storepkg.ErrMapTileSourceCannotDisableDefault) || errors.Is(err, storepkg.ErrMapTileSourceDefaultMustBeEnabled) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, gin.H{"item": AdminDTO(*row)})
}

func writeMapTileSourceDeleteResponse(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "map source not found"})
		return
	}
	if errors.Is(err, storepkg.ErrMapTileSourceCannotDeleteDefault) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// AdminDTO 是管理后台展示的全字段视图。
func AdminDTO(row storepkg.MapTileSourceRecord) gin.H {
	return gin.H{"id": row.ID, "name": row.Name, "url_template": row.URLTemplate, "attribution": row.Attribution, "max_zoom": row.MaxZoom, "enabled": row.Enabled, "is_default": row.IsDefault, "proxy_enabled": row.ProxyEnabled, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}

// PublicDTO 是给前端用户使用的视图：当 ProxyEnabled 为 true 时，url 改写为
// 通过本服务的 /api/map/{hash} 代理路径，避免暴露上游瓦片地址。
func PublicDTO(row storepkg.MapTileSourceRecord) gin.H {
	urlTemplate := row.URLTemplate
	if row.ProxyEnabled {
		hash := row.URLTemplateHash
		if hash == "" {
			hash = storepkg.MapTileSourceHash(row.URLTemplate)
		}
		urlTemplate = "/api/map/" + hash + "?x={x}&y={y}&z={z}"
	}
	return gin.H{"id": row.ID, "name": row.Name, "url_template": urlTemplate, "attribution": row.Attribution, "max_zoom": row.MaxZoom}
}
