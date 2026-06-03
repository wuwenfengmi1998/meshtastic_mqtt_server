package main

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
)

const (
	adminRole          = "admin"
	adminSessionCookie = "mesh_admin_session"
)

type adminUserDTO struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type sessionClaims struct {
	UserID   uint64 `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Expires  int64  `json:"expires"`
}

type sessionManager struct {
	secret []byte
	secure bool
	ttl    time.Duration
}

func newSessionManager(cfg webAdminConfig) (*sessionManager, error) {
	secret := strings.TrimSpace(cfg.SessionSecret)
	if secret == "" {
		generated := make([]byte, 32)
		if _, err := rand.Read(generated); err != nil {
			return nil, fmt.Errorf("generate admin session secret: %w", err)
		}
		return &sessionManager{secret: generated, secure: cfg.SessionSecure, ttl: 24 * time.Hour}, nil
	}
	return &sessionManager{secret: []byte(secret), secure: cfg.SessionSecure, ttl: 24 * time.Hour}, nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func adminUserResponse(user userRecord) adminUserDTO {
	return adminUserDTO{Username: user.Username, Role: user.Role}
}

func (sm *sessionManager) newCookie(user userRecord) (*http.Cookie, error) {
	claims := sessionClaims{UserID: user.ID, Username: user.Username, Role: user.Role, Expires: time.Now().Add(sm.ttl).Unix()}
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

func (sm *sessionManager) clearCookie() *http.Cookie {
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

func (sm *sessionManager) claimsFromRequest(c *gin.Context) (*sessionClaims, error) {
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
	var claims sessionClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, err
	}
	if claims.Expires <= time.Now().Unix() {
		return nil, errors.New("session expired")
	}
	if claims.Role != adminRole {
		return nil, errors.New("admin required")
	}
	return &claims, nil
}

func (sm *sessionManager) sign(payload string) string {
	mac := hmac.New(sha256.New, sm.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func requireAdmin(sm *sessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := sm.claimsFromRequest(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "admin login required"})
			c.Abort()
			return
		}
		c.Set("admin_claims", claims)
		c.Next()
	}
}
