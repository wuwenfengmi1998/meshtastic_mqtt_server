// Package auth 实现 admin 端 cookie session 与密码散列。
//
// 拆离自原来 main 包的 auth.go：让所有 admin route 包都可以直接 import 这里的
// SessionClaims / Manager / RequireAdmin，而不是被锁在根 main 包里。
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"meshtastic_mqtt_server/internal/config"
	"meshtastic_mqtt_server/internal/store"
)

const (
	AdminRole          = "admin"
	adminSessionCookie = "mesh_admin_session"

	// AdminClaimsKey 是中间件挂在 gin.Context 上的 key，handler 可用
	// `c.MustGet(auth.AdminClaimsKey).(*auth.SessionClaims)` 取出。
	AdminClaimsKey = "admin_claims"
)

// AdminUserDTO 是 /me /login 等接口返回给前端的最小用户视图。
type AdminUserDTO struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// SessionClaims 是 cookie 中持久化的会话内容。
type SessionClaims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Expires  int64  `json:"expires"`
}

// Manager 持有签名密钥与 cookie 配置，是发布 / 校验 cookie 的入口。
type Manager struct {
	secret []byte
	secure bool
	ttl    time.Duration
}

// NewManager 根据配置构造 Manager。如果 SessionSecret 空，会随机生成 32 字节。
func NewManager(cfg config.WebAdminConfig) (*Manager, error) {
	secret := strings.TrimSpace(cfg.SessionSecret)
	if secret == "" {
		generated := make([]byte, 32)
		if _, err := rand.Read(generated); err != nil {
			return nil, fmt.Errorf("generate admin session secret: %w", err)
		}
		return &Manager{secret: generated, secure: cfg.SessionSecure, ttl: 24 * time.Hour}, nil
	}
	return &Manager{secret: []byte(secret), secure: cfg.SessionSecure, ttl: 24 * time.Hour}, nil
}

// HashPassword 用 bcrypt 默认 cost 散列；用于建账号、改密码。
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword 校验明文密码是否与散列匹配。
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// AdminUserResponse 把 store.UserRecord 转成对外 DTO。
func AdminUserResponse(user store.UserRecord) AdminUserDTO {
	return AdminUserDTO{Username: user.Username, Role: user.Role}
}

// NewCookie 为已登录用户构造一份带签名的 session cookie。
func (sm *Manager) NewCookie(user store.UserRecord) (*http.Cookie, error) {
	claims := SessionClaims{UserID: user.ID, Username: user.Username, Role: user.Role, Expires: time.Now().Add(sm.ttl).Unix()}
	data, err := json.Marshal(claims)
	if err != nil {
		return nil, err
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	signature := sm.sign(payload)
	return &http.Cookie{
		Name:     adminSessionCookie,
		Value:    payload + "." + signature,
		Path:     "/",
		MaxAge:   int(sm.ttl.Seconds()),
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

// ClearCookie 返回一个把 cookie 立即清掉的 *http.Cookie，供 logout 使用。
func (sm *Manager) ClearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     adminSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ClaimsFromRequest 校验请求 cookie 的签名 / 过期 / 角色。
func (sm *Manager) ClaimsFromRequest(c *gin.Context) (*SessionClaims, error) {
	cookie, err := c.Cookie(adminSessionCookie)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(cookie, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid session")
	}
	if !hmac.Equal([]byte(parts[1]), []byte(sm.sign(parts[0]))) {
		return nil, errors.New("invalid session signature")
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var claims SessionClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, err
	}
	if claims.Expires <= time.Now().Unix() {
		return nil, errors.New("session expired")
	}
	if claims.Role != AdminRole {
		return nil, errors.New("admin required")
	}
	return &claims, nil
}

func (sm *Manager) sign(payload string) string {
	mac := hmac.New(sha256.New, sm.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// RequireAdmin 是把校验结果挂在 c.Set(AdminClaimsKey, claims) 上的中间件。
func RequireAdmin(sm *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := sm.ClaimsFromRequest(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "admin login required"})
			c.Abort()
			return
		}
		c.Set(AdminClaimsKey, claims)
		c.Next()
	}
}
