package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	mqttForwardDirectionSourceToTarget = "source_to_target"
	mqttForwardDirectionBidirectional  = "bidirectional"
)

var (
	errMQTTForwarderAlreadyExists    = errors.New("mqtt forwarder already exists")
	errMQTTForwardTopicAlreadyExists = errors.New("mqtt forward topic already exists")
)

type mqttForwarderInput struct {
	Name           string
	Enabled        bool
	SourceHost     string
	SourcePort     int
	SourceUsername string
	SourcePassword *string
	SourceClientID string
	SourceTLS      bool
	TargetHost     string
	TargetPort     int
	TargetUsername string
	TargetPassword *string
	TargetClientID string
	TargetTLS      bool
}

type mqttForwardTopicInput struct {
	Topic        string
	Enabled      bool
	Direction    string
	SourcePrefix string
	TargetPrefix string
	QoS          int
	Retain       bool
}

type mqttForwarderConfig struct {
	Forwarder mqttForwarderRecord
	Topics    []mqttForwardTopicRecord
}

func (s *store) ListMQTTForwarders(opts listOptions) ([]mqttForwarderRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []mqttForwarderRecord
	q := s.db.Model(&mqttForwarderRecord{}).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountMQTTForwarders(opts listOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&mqttForwarderRecord{}).Count(&total).Error
}

func (s *store) GetMQTTForwarder(id uint64) (*mqttForwarderRecord, error) {
	var row mqttForwarderRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) CreateMQTTForwarder(input mqttForwarderInput) (*mqttForwarderRecord, error) {
	row, err := mqttForwarderFromInput(input, nil)
	if err != nil {
		return nil, err
	}
	if err := s.ensureMQTTForwarderNameUnique(0, row.Name); err != nil {
		return nil, err
	}
	if err := s.db.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *store) UpdateMQTTForwarder(id uint64, input mqttForwarderInput) (*mqttForwarderRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("mqtt forwarder id is required")
	}
	existing, err := s.GetMQTTForwarder(id)
	if err != nil {
		return nil, err
	}
	row, err := mqttForwarderFromInput(input, existing)
	if err != nil {
		return nil, err
	}
	if err := s.ensureMQTTForwarderNameUnique(id, row.Name); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"name": row.Name, "enabled": row.Enabled,
		"source_host": row.SourceHost, "source_port": row.SourcePort, "source_username": row.SourceUsername,
		"source_password": row.SourcePassword, "source_client_id": row.SourceClientID, "source_tls": row.SourceTLS,
		"target_host": row.TargetHost, "target_port": row.TargetPort, "target_username": row.TargetUsername,
		"target_password": row.TargetPassword, "target_client_id": row.TargetClientID, "target_tls": row.TargetTLS,
		"updated_at": time.Now(),
	}
	if err := s.db.Model(&mqttForwarderRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetMQTTForwarder(id)
}

func (s *store) DeleteMQTTForwarder(id uint64) error {
	if id == 0 {
		return fmt.Errorf("mqtt forwarder id is required")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("forwarder_id = ?", id).Delete(&mqttForwardTopicRecord{}).Error; err != nil {
			return err
		}
		result := tx.Where("id = ?", id).Delete(&mqttForwarderRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *store) ListMQTTForwardTopics(forwarderID uint64, opts listOptions) ([]mqttForwardTopicRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []mqttForwardTopicRecord
	q := s.db.Model(&mqttForwardTopicRecord{}).
		Where("forwarder_id = ?", forwarderID).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountMQTTForwardTopics(forwarderID uint64) (int64, error) {
	var total int64
	return total, s.db.Model(&mqttForwardTopicRecord{}).Where("forwarder_id = ?", forwarderID).Count(&total).Error
}

func (s *store) GetMQTTForwardTopic(id uint64) (*mqttForwardTopicRecord, error) {
	var row mqttForwardTopicRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) CreateMQTTForwardTopic(forwarderID uint64, input mqttForwardTopicInput) (*mqttForwardTopicRecord, error) {
	if _, err := s.GetMQTTForwarder(forwarderID); err != nil {
		return nil, err
	}
	row, err := mqttForwardTopicFromInput(forwarderID, input)
	if err != nil {
		return nil, err
	}
	if err := s.ensureMQTTForwardTopicUnique(0, forwarderID, row.Topic); err != nil {
		return nil, err
	}
	if err := s.db.Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *store) UpdateMQTTForwardTopic(id uint64, input mqttForwardTopicInput) (*mqttForwardTopicRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("mqtt forward topic id is required")
	}
	existing, err := s.GetMQTTForwardTopic(id)
	if err != nil {
		return nil, err
	}
	row, err := mqttForwardTopicFromInput(existing.ForwarderID, input)
	if err != nil {
		return nil, err
	}
	if err := s.ensureMQTTForwardTopicUnique(id, existing.ForwarderID, row.Topic); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"topic": row.Topic, "enabled": row.Enabled, "direction": row.Direction,
		"source_prefix": row.SourcePrefix, "target_prefix": row.TargetPrefix,
		"qos": row.QoS, "retain": row.Retain, "updated_at": time.Now(),
	}
	if err := s.db.Model(&mqttForwardTopicRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetMQTTForwardTopic(id)
}

func (s *store) DeleteMQTTForwardTopic(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&mqttForwardTopicRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *store) GetMQTTForwarderConfig(id uint64) (*mqttForwarderConfig, error) {
	forwarder, err := s.GetMQTTForwarder(id)
	if err != nil {
		return nil, err
	}
	var topics []mqttForwardTopicRecord
	if err := s.db.Where("forwarder_id = ? AND enabled = ?", id, true).Order("id ASC").Find(&topics).Error; err != nil {
		return nil, err
	}
	return &mqttForwarderConfig{Forwarder: *forwarder, Topics: topics}, nil
}

func (s *store) ListEnabledMQTTForwarderConfigs() ([]mqttForwarderConfig, error) {
	var forwarders []mqttForwarderRecord
	if err := s.db.Where("enabled = ?", true).Order("id ASC").Find(&forwarders).Error; err != nil {
		return nil, err
	}
	configs := make([]mqttForwarderConfig, 0, len(forwarders))
	for _, forwarder := range forwarders {
		var topics []mqttForwardTopicRecord
		if err := s.db.Where("forwarder_id = ? AND enabled = ?", forwarder.ID, true).Order("id ASC").Find(&topics).Error; err != nil {
			return nil, err
		}
		if len(topics) == 0 {
			continue
		}
		configs = append(configs, mqttForwarderConfig{Forwarder: forwarder, Topics: topics})
	}
	return configs, nil
}

func (s *store) ensureMQTTForwarderNameUnique(id uint64, name string) error {
	var existing mqttForwarderRecord
	q := s.db.Where("name = ?", name)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return errMQTTForwarderAlreadyExists
}

func (s *store) ensureMQTTForwardTopicUnique(id, forwarderID uint64, topic string) error {
	var existing mqttForwardTopicRecord
	q := s.db.Where("forwarder_id = ? AND topic = ?", forwarderID, topic)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return errMQTTForwardTopicAlreadyExists
}

func mqttForwarderFromInput(input mqttForwarderInput, existing *mqttForwarderRecord) (*mqttForwarderRecord, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("mqtt forwarder name is required")
	}
	sourceHost := strings.TrimSpace(input.SourceHost)
	if sourceHost == "" {
		return nil, fmt.Errorf("source host is required")
	}
	if err := validateMQTTForwardPort(input.SourcePort, "source port"); err != nil {
		return nil, err
	}
	targetHost := strings.TrimSpace(input.TargetHost)
	if targetHost == "" {
		return nil, fmt.Errorf("target host is required")
	}
	if err := validateMQTTForwardPort(input.TargetPort, "target port"); err != nil {
		return nil, err
	}
	row := &mqttForwarderRecord{
		Name: name, Enabled: input.Enabled,
		SourceHost: sourceHost, SourcePort: input.SourcePort, SourceUsername: strings.TrimSpace(input.SourceUsername), SourceClientID: strings.TrimSpace(input.SourceClientID), SourceTLS: input.SourceTLS,
		TargetHost: targetHost, TargetPort: input.TargetPort, TargetUsername: strings.TrimSpace(input.TargetUsername), TargetClientID: strings.TrimSpace(input.TargetClientID), TargetTLS: input.TargetTLS,
	}
	if input.SourcePassword != nil {
		row.SourcePassword = *input.SourcePassword
	} else if existing != nil {
		row.SourcePassword = existing.SourcePassword
	}
	if input.TargetPassword != nil {
		row.TargetPassword = *input.TargetPassword
	} else if existing != nil {
		row.TargetPassword = existing.TargetPassword
	}
	return row, nil
}

func mqttForwardTopicFromInput(forwarderID uint64, input mqttForwardTopicInput) (*mqttForwardTopicRecord, error) {
	if forwarderID == 0 {
		return nil, fmt.Errorf("mqtt forwarder id is required")
	}
	topic := strings.TrimSpace(input.Topic)
	if err := validateMQTTTopicFilter(topic); err != nil {
		return nil, err
	}
	direction, err := normalizeMQTTForwardDirection(input.Direction)
	if err != nil {
		return nil, err
	}
	if input.QoS < 0 || input.QoS > 2 {
		return nil, fmt.Errorf("qos must be 0, 1, or 2")
	}
	return &mqttForwardTopicRecord{
		ForwarderID: forwarderID, Topic: topic, Enabled: input.Enabled, Direction: direction,
		SourcePrefix: strings.Trim(strings.TrimSpace(input.SourcePrefix), "/"),
		TargetPrefix: strings.Trim(strings.TrimSpace(input.TargetPrefix), "/"),
		QoS:          input.QoS, Retain: input.Retain,
	}, nil
}

func validateMQTTForwardPort(port int, label string) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", label)
	}
	return nil
}

func normalizeMQTTForwardDirection(direction string) (string, error) {
	direction = strings.TrimSpace(direction)
	if direction == "" {
		direction = mqttForwardDirectionSourceToTarget
	}
	switch direction {
	case mqttForwardDirectionSourceToTarget, mqttForwardDirectionBidirectional:
		return direction, nil
	default:
		return "", fmt.Errorf("invalid mqtt forward direction")
	}
}

func validateMQTTTopicFilter(topic string) error {
	if topic == "" {
		return fmt.Errorf("topic is required")
	}
	parts := strings.Split(topic, "/")
	for i, part := range parts {
		if strings.Contains(part, "#") {
			if part != "#" || i != len(parts)-1 {
				return fmt.Errorf("invalid topic filter: # must be the last level")
			}
		}
		if strings.Contains(part, "+") && part != "+" {
			return fmt.Errorf("invalid topic filter: + must occupy an entire level")
		}
	}
	return nil
}
