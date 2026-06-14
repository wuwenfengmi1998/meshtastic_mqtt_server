package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"meshtastic_mqtt_server/mqtpp"

	"gorm.io/gorm"
)

const (
	botDefaultTopicPrefix              = "msh/CN"
	botDefaultPSK                      = "AQ=="
	botDefaultNodeInfoBroadcastSeconds = int64(3600)
	botMessageTypeChannel              = "channel"
	botMessageTypeDirect               = "direct"
	botMessageStatusPending            = "pending"
	botMessageStatusPublished          = "published"
	botMessageStatusFailed             = "failed"
)

var errBotNodeAlreadyExists = errors.New("bot node already exists")

type botNodeInput struct {
	NodeNum                          *int64
	LongName                         string
	ShortName                        string
	Enabled                          bool
	DefaultChannelID                 string
	TopicPrefix                      string
	PSK                              string
	NodeInfoBroadcastEnabled         bool
	NodeInfoBroadcastIntervalSeconds int64
}

type botMessageListOptions struct {
	listOptions
	BotID       uint64
	MessageType string
	ChannelID   string
}

func (s *store) ListBotNodes(opts listOptions) ([]botNodeRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []botNodeRecord
	q := s.db.Model(&botNodeRecord{}).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountBotNodes(opts listOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&botNodeRecord{}).Count(&total).Error
}

func (s *store) GetBotNode(id uint64) (*botNodeRecord, error) {
	var row botNodeRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) CreateBotNode(input botNodeInput) (*botNodeRecord, error) {
	row, err := s.normalizedBotNodeRecord(input)
	if err != nil {
		return nil, err
	}
	if err := s.ensureBotNodeUnique(0, row.NodeID, row.NodeNum); err != nil {
		return nil, err
	}
	if err := s.ensureBotNodeDoesNotConflictWithNodeInfo(row.NodeNum, row.NodeID); err != nil {
		return nil, err
	}
	if err := populateBotNodeKeys(row); err != nil {
		return nil, err
	}
	if err := s.db.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *store) UpdateBotNode(id uint64, input botNodeInput) (*botNodeRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("bot node id is required")
	}
	existing, err := s.GetBotNode(id)
	if err != nil {
		return nil, err
	}
	row, err := s.normalizedBotNodeRecord(input)
	if err != nil {
		return nil, err
	}
	if err := s.ensureBotNodeUnique(id, row.NodeID, row.NodeNum); err != nil {
		return nil, err
	}
	// 只有当 node_num 真的发生变化时，才需要校验和 nodeinfo 表的冲突。
	// 否则机器人自己广播 NodeInfo 回写到 nodeinfo 表后，UpdateBotNode 会把这条
	// 自己的记录当成外部节点冲突，导致 “already exists or conflicts” 报错。
	if row.NodeNum != existing.NodeNum {
		if err := s.ensureBotNodeDoesNotConflictWithNodeInfo(row.NodeNum, row.NodeID); err != nil {
			return nil, err
		}
	}
	updates := map[string]any{
		"node_id":                             row.NodeID,
		"node_num":                            row.NodeNum,
		"long_name":                           row.LongName,
		"short_name":                          row.ShortName,
		"enabled":                             row.Enabled,
		"default_channel_id":                  row.DefaultChannelID,
		"topic_prefix":                        row.TopicPrefix,
		"psk":                                 row.PSK,
		"nodeinfo_broadcast_enabled":          row.NodeInfoBroadcastEnabled,
		"nodeinfo_broadcast_interval_seconds": row.NodeInfoBroadcastIntervalSeconds,
		"updated_at":                          time.Now(),
	}
	if err := s.db.Model(&botNodeRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetBotNode(id)
}

func (s *store) DeleteBotNode(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&botNodeRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *store) InsertBotMessage(row *botMessageRecord) error {
	return s.db.Create(row).Error
}

func (s *store) UpdateBotMessageStatus(id uint64, status, errText string, publishedAt *time.Time) error {
	updates := map[string]any{"status": status, "error": strings.TrimSpace(errText), "published_at": publishedAt}
	result := s.db.Model(&botMessageRecord{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *store) UpdateBotNodeInfoBroadcastAt(id uint64, t time.Time) error {
	result := s.db.Model(&botNodeRecord{}).Where("id = ?", id).Updates(map[string]any{"last_nodeinfo_broadcast_at": &t, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *store) RegenerateBotNodeKeys(id uint64) (*botNodeRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("bot node id is required")
	}
	row, err := s.GetBotNode(id)
	if err != nil {
		return nil, err
	}
	if err := populateBotNodeKeys(row); err != nil {
		return nil, err
	}
	updates := map[string]any{"public_key": row.PublicKey, "private_key": row.PrivateKey, "updated_at": time.Now()}
	if err := s.db.Model(&botNodeRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetBotNode(id)
}

func (s *store) ListBotMessages(opts botMessageListOptions) ([]botMessageRecord, error) {
	opts.listOptions = normalizeListOptions(opts.listOptions)
	var rows []botMessageRecord
	q := applyBotMessageFilters(s.db.Model(&botMessageRecord{}), opts).
		Order("created_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountBotMessages(opts botMessageListOptions) (int64, error) {
	var total int64
	q := applyBotMessageFilters(s.db.Model(&botMessageRecord{}), opts)
	return total, q.Count(&total).Error
}

func applyBotMessageFilters(q *gorm.DB, opts botMessageListOptions) *gorm.DB {
	if opts.BotID != 0 {
		q = q.Where("bot_id = ?", opts.BotID)
	}
	if opts.MessageType != "" {
		q = q.Where("message_type = ?", opts.MessageType)
	}
	if opts.ChannelID != "" {
		q = q.Where("channel_id = ?", opts.ChannelID)
	}
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return q
}

func (s *store) normalizedBotNodeRecord(input botNodeInput) (*botNodeRecord, error) {
	longName := strings.TrimSpace(input.LongName)
	shortName := strings.TrimSpace(input.ShortName)
	channelID := strings.TrimSpace(input.DefaultChannelID)
	psk := strings.TrimSpace(input.PSK)
	if psk == "" {
		psk = botDefaultPSK
	}
	if _, err := mqtpp.ExpandPSK(psk); err != nil {
		return nil, err
	}
	topicPrefix := strings.Trim(strings.TrimSpace(input.TopicPrefix), "/")
	if topicPrefix == "" {
		topicPrefix = botDefaultTopicPrefix
	}
	if longName == "" {
		return nil, fmt.Errorf("long name is required")
	}
	if !utf8.ValidString(longName) {
		return nil, fmt.Errorf("long name must be valid utf-8")
	}
	if shortName == "" {
		return nil, fmt.Errorf("short name is required")
	}
	if !utf8.ValidString(shortName) {
		return nil, fmt.Errorf("short name must be valid utf-8")
	}
	if channelID == "" {
		return nil, fmt.Errorf("default channel id is required")
	}
	interval := input.NodeInfoBroadcastIntervalSeconds
	if interval <= 0 {
		interval = botDefaultNodeInfoBroadcastSeconds
	}
	if interval < 60 {
		return nil, fmt.Errorf("nodeinfo broadcast interval must be at least 60 seconds")
	}
	var nodeNum int64
	if input.NodeNum == nil || *input.NodeNum == 0 {
		generated, err := s.generateBotNodeNum()
		if err != nil {
			return nil, err
		}
		nodeNum = generated
	} else {
		nodeNum = *input.NodeNum
	}
	if err := validateBotNodeNum(nodeNum); err != nil {
		return nil, err
	}
	return &botNodeRecord{NodeID: mqtpp.NodeNumToID(uint32(nodeNum)), NodeNum: nodeNum, LongName: longName, ShortName: shortName, Enabled: input.Enabled, DefaultChannelID: channelID, TopicPrefix: topicPrefix, PSK: psk, NodeInfoBroadcastEnabled: input.NodeInfoBroadcastEnabled, NodeInfoBroadcastIntervalSeconds: interval}, nil
}

func populateBotNodeKeys(row *botNodeRecord) error {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	row.PrivateKey = base64.StdEncoding.EncodeToString(privateKey.Bytes())
	row.PublicKey = base64.StdEncoding.EncodeToString(privateKey.PublicKey().Bytes())
	return nil
}

func decodeBotPublicKey(row botNodeRecord) ([]byte, error) {
	if strings.TrimSpace(row.PublicKey) == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(row.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid bot public key: %w", err)
	}
	return key, nil
}

func validateBotNodeNum(nodeNum int64) error {
	if nodeNum <= 0 || nodeNum >= int64(mqtpp.NodeNumBroadcast) {
		return fmt.Errorf("node num must be between 1 and 4294967294")
	}
	return nil
}

func (s *store) generateBotNodeNum() (int64, error) {
	for i := 0; i < 32; i++ {
		var buf [4]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, err
		}
		nodeNum := int64(binary.LittleEndian.Uint32(buf[:]) & 0x7fffffff)
		if err := validateBotNodeNum(nodeNum); err != nil {
			continue
		}
		if err := s.ensureBotNodeUnique(0, mqtpp.NodeNumToID(uint32(nodeNum)), nodeNum); err != nil {
			if errors.Is(err, errBotNodeAlreadyExists) {
				continue
			}
			return 0, err
		}
		if err := s.ensureBotNodeDoesNotConflictWithNodeInfo(nodeNum, mqtpp.NodeNumToID(uint32(nodeNum))); err != nil {
			if errors.Is(err, errBotNodeAlreadyExists) {
				continue
			}
			return 0, err
		}
		return nodeNum, nil
	}
	return 0, fmt.Errorf("generate bot node num failed")
}

func (s *store) ensureBotNodeUnique(id uint64, nodeID string, nodeNum int64) error {
	var existing botNodeRecord
	q := s.db.Where("node_id = ? OR node_num = ?", nodeID, nodeNum)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return errBotNodeAlreadyExists
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (s *store) ensureBotNodeDoesNotConflictWithNodeInfo(nodeNum int64, selfNodeID string) error {
	var existing nodeInfoRecord
	q := s.db.Where("node_num = ?", nodeNum)
	if selfNodeID != "" {
		// 机器人自己广播 NodeInfo 后会以同样的 node_id/node_num 回写 nodeinfo；
		// 把这条自身记录从冲突检测中排除，避免把自己当成外部节点。
		q = q.Where("node_id <> ?", selfNodeID)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return errBotNodeAlreadyExists
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}
