package store

import "golang.org/x/crypto/bcrypt"

// AdminRole 是管理员账号在用户表里的角色字符串。其它包通过这个常量与
// `users.role` 字段对齐，避免硬编码。
const AdminRole = "admin"

// printJSON 是 store 包内部的诊断输出钩子。当前实现为 noop——保持与
// 重构前 main.go 的行为一致；如需启用调试，可在调用方替换。
func printJSON(record map[string]any) {
	_ = record
}

// hashPassword 与 auth.go 中的散列实现保持一致（bcrypt 默认 cost）。
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// uint32FromRecord 把 map[string]any 中的整型字段安全转换为 uint32。
func uint32FromRecord(value any) (uint32, bool) {
	switch v := value.(type) {
	case uint32:
		return v, true
	case int:
		if v >= 0 {
			return uint32(v), true
		}
	case int64:
		if v >= 0 {
			return uint32(v), true
		}
	case uint64:
		return uint32(v), true
	case float64:
		if v >= 0 {
			return uint32(v), true
		}
	}
	return 0, false
}
