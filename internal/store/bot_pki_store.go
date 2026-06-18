package store

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// GetBotNodeByNodeNum 按节点号查找受管 bot 节点；用于 PKI 解密时把 to 字段映射回本地私钥。
func (s *Store) GetBotNodeByNodeNum(nodeNum int64) (*BotNodeRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store not configured")
	}
	var row BotNodeRecord
	if err := s.db.Where("node_num = ?", nodeNum).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &row, nil
}

// LookupNodeInfoPublicKey 在 nodeinfo 表中按 node_num 查 X25519 公钥，
// 兼容 hex 与 base64 两种历史存储格式。
func (s *Store) LookupNodeInfoPublicKey(nodeNum uint32) ([]byte, bool) {
	var row NodeInfoRecord
	if err := s.db.Where("node_num = ?", int64(nodeNum)).Take(&row).Error; err != nil {
		return nil, false
	}
	if row.PublicKey == nil {
		return nil, false
	}
	value := strings.TrimSpace(*row.PublicKey)
	if value == "" {
		return nil, false
	}
	if decoded, err := hex.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, true
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, true
	}
	return nil, false
}
