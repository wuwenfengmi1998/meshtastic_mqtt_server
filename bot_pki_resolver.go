package main

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// pkiKeyResolver 是 mqtpp 在解密 PKI 加密包时回调的接收者私钥/发送者公钥查询函数。
//
// to 是接收者节点号（应该匹配某个本地受管的 bot），from 是发送者节点号（应该已经有 nodeinfo 上报）。
// 返回的 ok=false 时调用方会跳过 PKI 路径并回落到 channel PSK 解密。
func newPKIKeyResolver(s *store) func(toNodeNum, fromNodeNum uint32) ([]byte, []byte, bool) {
	if s == nil {
		return nil
	}
	return func(toNodeNum, fromNodeNum uint32) ([]byte, []byte, bool) {
		bot, err := s.GetBotNodeByNodeNum(int64(toNodeNum))
		if err != nil {
			printJSON(map[string]any{
				"event":   "pki_resolve_bot_not_found",
				"to_num":  toNodeNum,
				"from_num": fromNodeNum,
			})
			return nil, nil, false
		}
		privateKeyB64 := strings.TrimSpace(bot.PrivateKey)
		if privateKeyB64 == "" {
			printJSON(map[string]any{
				"event":    "pki_resolve_no_private_key",
				"bot_id":   bot.NodeID,
				"bot_num":  bot.NodeNum,
				"from_num": fromNodeNum,
			})
			return nil, nil, false
		}
		privateKey, err := base64.StdEncoding.DecodeString(privateKeyB64)
		if err != nil || len(privateKey) != 32 {
			printJSON(map[string]any{
				"event":    "pki_resolve_invalid_private_key",
				"bot_id":   bot.NodeID,
				"bot_num":  bot.NodeNum,
				"from_num": fromNodeNum,
				"error":    err,
			})
			return nil, nil, false
		}
		fromPublic, ok := lookupNodeInfoPublicKey(s, fromNodeNum)
		if !ok {
			printJSON(map[string]any{
				"event":    "pki_resolve_no_sender_public_key",
				"bot_id":   bot.NodeID,
				"bot_num":  bot.NodeNum,
				"from_num": fromNodeNum,
			})
			return nil, nil, false
		}
		return privateKey, fromPublic, true
	}
}

// lookupNodeInfoPublicKey 在 nodeinfo 表中按 node_num 查 X25519 公钥，
// 兼容 hex 与 base64 两种历史存储格式。
func lookupNodeInfoPublicKey(s *store, nodeNum uint32) ([]byte, bool) {
	var row nodeInfoRecord
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

// GetBotNodeByNodeNum 按节点号查找受管 bot 节点；用于 PKI 解密时把 to 字段映射回本地私钥。
func (s *store) GetBotNodeByNodeNum(nodeNum int64) (*botNodeRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("store not configured")
	}
	var row botNodeRecord
	if err := s.db.Where("node_num = ?", nodeNum).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &row, nil
}
