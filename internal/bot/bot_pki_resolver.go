package bot

import (
	"encoding/base64"
	"strings"

	storepkg "meshtastic_mqtt_server/internal/store"
)

// NewPKIKeyResolver 返回 mqtpp 在解密 PKI 加密包时使用的回调：根据接收者
// 节点号查找受管 bot 的私钥，并根据发送者节点号在 nodeinfo 表中查找其公钥。
// 返回 ok=false 时调用方会跳过 PKI 路径并回落到 channel PSK 解密。
func NewPKIKeyResolver(s *storepkg.Store) func(toNodeNum, fromNodeNum uint32) ([]byte, []byte, bool) {
	if s == nil {
		return nil
	}
	return func(toNodeNum, fromNodeNum uint32) ([]byte, []byte, bool) {
		bot, err := s.GetBotNodeByNodeNum(int64(toNodeNum))
		if err != nil {
			return nil, nil, false
		}
		privateKeyB64 := strings.TrimSpace(bot.PrivateKey)
		if privateKeyB64 == "" {
			return nil, nil, false
		}
		privateKey, err := base64.StdEncoding.DecodeString(privateKeyB64)
		if err != nil || len(privateKey) != 32 {
			return nil, nil, false
		}
		fromPublic, ok := s.LookupNodeInfoPublicKey(fromNodeNum)
		if !ok {
			return nil, nil, false
		}
		return privateKey, fromPublic, true
	}
}
