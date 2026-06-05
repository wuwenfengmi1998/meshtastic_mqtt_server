package main

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type mapTileSourceRequest struct {
	Name        string `json:"name"`
	URLTemplate string `json:"url_template"`
	Attribution string `json:"attribution"`
	MaxZoom     int    `json:"max_zoom"`
	Enabled     bool   `json:"enabled"`
	IsDefault   bool   `json:"is_default"`
}

func registerMapSourceRoutes(r gin.IRouter, store *store) {
	r.GET("/map-source/default", func(c *gin.Context) {
		row, err := store.GetDefaultMapTileSource()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": publicMapTileSourceDTO(*row)})
	})
}

func registerAdminMapSourceRoutes(r gin.IRouter, store *store) {
	r.GET("/map-source", func(c *gin.Context) {
		opts, ok := parseListOptions(c)
		if !ok {
			return
		}
		rows, err := store.ListMapTileSources(opts)
		if err != nil {
			writeListResponse(c, rows, opts, err, mapTileSourceDTO)
			return
		}
		total, err := store.CountMapTileSources(opts)
		writeListResponseWithTotal(c, rows, opts, total, err, mapTileSourceDTO)
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

func mapTileSourceInputFromRequest(req mapTileSourceRequest) mapTileSourceInput {
	return mapTileSourceInput{
		Name:        req.Name,
		URLTemplate: req.URLTemplate,
		Attribution: req.Attribution,
		MaxZoom:     req.MaxZoom,
		Enabled:     req.Enabled,
		IsDefault:   req.IsDefault,
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

func writeMapTileSourceMutationResponse(c *gin.Context, status int, row *mapTileSourceRecord, err error) {
	if errors.Is(err, errMapTileSourceAlreadyExists) {
		c.JSON(http.StatusConflict, gin.H{"error": "map source already exists"})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "map source not found"})
		return
	}
	if errors.Is(err, errMapTileSourceCannotDeleteDefault) || errors.Is(err, errMapTileSourceCannotDisableDefault) || errors.Is(err, errMapTileSourceDefaultMustBeEnabled) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, gin.H{"item": mapTileSourceDTO(*row)})
}

func writeMapTileSourceDeleteResponse(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "map source not found"})
		return
	}
	if errors.Is(err, errMapTileSourceCannotDeleteDefault) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func mapTileSourceDTO(row mapTileSourceRecord) gin.H {
	return gin.H{"id": row.ID, "name": row.Name, "url_template": row.URLTemplate, "attribution": row.Attribution, "max_zoom": row.MaxZoom, "enabled": row.Enabled, "is_default": row.IsDefault, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt}
}

func publicMapTileSourceDTO(row mapTileSourceRecord) gin.H {
	return gin.H{"id": row.ID, "name": row.Name, "url_template": row.URLTemplate, "attribution": row.Attribution, "max_zoom": row.MaxZoom}
}
