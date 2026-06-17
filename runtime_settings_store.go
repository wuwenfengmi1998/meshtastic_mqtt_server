package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	runtimeSettingAllowEncryptedForwarding = "mqtt.allow_encrypted_forwarding"
	runtimeSettingLLMQueueEnabled         = "llm.queue_enabled"
	runtimeSettingLLMQueueIncludeChannel  = "llm.include_channel_messages"
	runtimeSettingTypeBool                 = "bool"
)

type runtimeSettingsSnapshot struct {
	AllowEncryptedForwarding bool
	LLMQueueEnabled          bool
	LLMIncludeChannel        bool
}

func (s *store) GetRuntimeSettings() (runtimeSettingsSnapshot, error) {
	allowEncrypted, err := s.GetBoolRuntimeSetting(runtimeSettingAllowEncryptedForwarding, false)
	if err != nil {
		return runtimeSettingsSnapshot{}, err
	}
	llmQueueEnabled, err := s.GetBoolRuntimeSetting(runtimeSettingLLMQueueEnabled, true)
	if err != nil {
		return runtimeSettingsSnapshot{}, err
	}
	llmIncludeChannel, err := s.GetBoolRuntimeSetting(runtimeSettingLLMQueueIncludeChannel, false)
	if err != nil {
		return runtimeSettingsSnapshot{}, err
	}
	return runtimeSettingsSnapshot{
		AllowEncryptedForwarding: allowEncrypted,
		LLMQueueEnabled:          llmQueueEnabled,
		LLMIncludeChannel:        llmIncludeChannel,
	}, nil
}

func (s *store) GetBoolRuntimeSetting(key string, defaultValue bool) (bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, fmt.Errorf("runtime setting key is required")
	}

	var row runtimeSettingRecord
	err := s.db.Where("`key` = ?", key).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultValue, nil
	}
	if err != nil {
		return false, err
	}
	if row.ValueType != "" && row.ValueType != runtimeSettingTypeBool {
		return false, fmt.Errorf("runtime setting %s has type %s, want %s", key, row.ValueType, runtimeSettingTypeBool)
	}
	value, err := strconv.ParseBool(strings.TrimSpace(row.Value))
	if err != nil {
		return false, fmt.Errorf("parse runtime setting %s: %w", key, err)
	}
	return value, nil
}

func (s *store) SetBoolRuntimeSetting(key string, value bool, label string) (*runtimeSettingRecord, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("runtime setting key is required")
	}

	row := runtimeSettingRecord{
		Key:       key,
		Value:     strconv.FormatBool(value),
		ValueType: runtimeSettingTypeBool,
		Label:     strings.TrimSpace(label),
	}
	if err := s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"value":      row.Value,
			"value_type": row.ValueType,
			"label":      row.Label,
			"updated_at": time.Now(),
		}),
	}).Create(&row).Error; err != nil {
		return nil, err
	}
	if err := s.db.Where("`key` = ?", key).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
