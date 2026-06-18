package store

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// 测试 helper —— 为从 main 包搬过来的测试提供它们原本依赖的小写函数。
// 这些 helper 不暴露给生产代码使用；它们的行为应当与 main 包对应实现保持一致。

// verifyPassword 复刻 auth.go 中的 bcrypt 校验，用于 user_store 的测试。
func verifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// publicMapTileSourceDTO 复刻 admin_map_source_routes.go 中的同名函数，
// 仅供 map_source_store_test.go 验证 ProxyEnabled 时 URL 是否被改写。
// 这里返回 map[string]any 而非 gin.H 以避免引入 gin 依赖。
func publicMapTileSourceDTO(row MapTileSourceRecord) map[string]any {
	urlTemplate := row.URLTemplate
	if row.ProxyEnabled {
		hash := row.URLTemplateHash
		if hash == "" {
			hash = MapTileSourceHash(row.URLTemplate)
		}
		urlTemplate = "/api/map/" + hash + "?x={x}&y={y}&z={z}"
	}
	return map[string]any{
		"id":            row.ID,
		"name":          row.Name,
		"url_template":  urlTemplate,
		"attribution":   row.Attribution,
		"max_zoom":      row.MaxZoom,
		"enabled":       row.Enabled,
		"is_default":    row.IsDefault,
		"proxy_enabled": row.ProxyEnabled,
	}
}

// newDBWriteQueue 是 db_write_queue_test.go 期望的旧名字。重新导出供测试使用。
var newDBWriteQueue = NewWriteQueue

// 让 strings 不会被 import-but-not-used（如果上面用不到，就算了——保留以应对将来扩展）
var _ = strings.TrimSpace
