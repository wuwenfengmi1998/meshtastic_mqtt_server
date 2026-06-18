package help

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"meshtastic_mqtt_server/internal/auth"
	storepkg "meshtastic_mqtt_server/internal/store"
)

type helpContentRequest struct {
	Markdown string `json:"markdown"`
}

// RegisterPublicRoutes 把对外可见的 GET /help 挂到给定路由组下。
func RegisterPublicRoutes(r gin.IRouter, store *storepkg.Store) {
	r.GET("/help", func(c *gin.Context) {
		item, err := latestHelpContentDTO(store)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": item})
	})
}

// RegisterAdminRoutes 注册管理员侧 /help、/help、/help/preview 这三条路由。
func RegisterAdminRoutes(r gin.IRouter, store *storepkg.Store) {
	r.GET("/help", func(c *gin.Context) {
		item, err := latestHelpContentDTO(store)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"item": item})
	})
	r.POST("/help", func(c *gin.Context) {
		var req helpContentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid help content request"})
			return
		}
		claims := c.MustGet(auth.AdminClaimsKey).(*auth.SessionClaims)
		row, err := store.InsertHelpContent(req.Markdown, claims.Username)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		item, err := helpContentDTO(row.ID, row.Markdown, row.CreatedBy, &row.CreatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"item": item})
	})
	r.POST("/help/preview", func(c *gin.Context) {
		var req helpContentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid help preview request"})
			return
		}
		html, err := RenderMarkdown(req.Markdown)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"html": html})
	})
}

func latestHelpContentDTO(store *storepkg.Store) (gin.H, error) {
	row, err := store.GetLatestHelpContent()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return helpContentDTO(0, storepkg.DefaultHelpMarkdown, "", nil)
	}
	if err != nil {
		return nil, err
	}
	return helpContentDTO(row.ID, row.Markdown, row.CreatedBy, &row.CreatedAt)
}

func helpContentDTO(id uint64, markdown, createdBy string, createdAt *time.Time) (gin.H, error) {
	html, err := RenderMarkdown(markdown)
	if err != nil {
		return nil, err
	}
	return gin.H{"id": ptrHelpID(id), "markdown": markdown, "html": html, "created_by": createdBy, "created_at": ptrTime(createdAt)}, nil
}

func ptrHelpID(id uint64) any {
	if id == 0 {
		return nil
	}
	return id
}

func ptrTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
