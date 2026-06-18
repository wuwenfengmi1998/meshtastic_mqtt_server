package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	MQTTForwardDirectionSourceToTarget = "source_to_target"
	MQTTForwardDirectionBidirectional  = "bidirectional"
)

var (
	ErrMQTTForwarderAlreadyExists    = errors.New("mqtt forwarder already exists")
	ErrMQTTForwardTopicAlreadyExists = errors.New("mqtt forward topic already exists")
)

type MQTTForwarderInput struct {
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

type MQTTForwardTopicInput struct {
	Topic        string
	Enabled      bool
	Direction    string
	SourcePrefix string
	TargetPrefix string
	QoS          int
	Retain       bool
}

type MQTTForwarderConfig struct {
	Forwarder MQTTForwarderRecord
	Topics    []MQTTForwardTopicRecord
}

func (s *Store) ListMQTTForwarders(opts ListOptions) ([]MQTTForwarderRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []MQTTForwarderRecord
	q := s.db.Model(&MQTTForwarderRecord{}).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *Store) CountMQTTForwarders(opts ListOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&MQTTForwarderRecord{}).Count(&total).Error
}

func (s *Store) GetMQTTForwarder(id uint64) (*MQTTForwarderRecord, error) {
	var row MQTTForwarderRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) CreateMQTTForwarder(input MQTTForwarderInput) (*MQTTForwarderRecord, error) {
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

func (s *Store) UpdateMQTTForwarder(id uint64, input MQTTForwarderInput) (*MQTTForwarderRecord, error) {
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
	if err := s.db.Model(&MQTTForwarderRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetMQTTForwarder(id)
}

func (s *Store) DeleteMQTTForwarder(id uint64) error {
	if id == 0 {
		return fmt.Errorf("mqtt forwarder id is required")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("forwarder_id = ?", id).Delete(&MQTTForwardTopicRecord{}).Error; err != nil {
			return err
		}
		result := tx.Where("id = ?", id).Delete(&MQTTForwarderRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *Store) ListMQTTForwardTopics(forwarderID uint64, opts ListOptions) ([]MQTTForwardTopicRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []MQTTForwardTopicRecord
	q := s.db.Model(&MQTTForwardTopicRecord{}).
		Where("forwarder_id = ?", forwarderID).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *Store) CountMQTTForwardTopics(forwarderID uint64) (int64, error) {
	var total int64
	return total, s.db.Model(&MQTTForwardTopicRecord{}).Where("forwarder_id = ?", forwarderID).Count(&total).Error
}

func (s *Store) GetMQTTForwardTopic(id uint64) (*MQTTForwardTopicRecord, error) {
	var row MQTTForwardTopicRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) CreateMQTTForwardTopic(forwarderID uint64, input MQTTForwardTopicInput) (*MQTTForwardTopicRecord, error) {
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

func (s *Store) UpdateMQTTForwardTopic(id uint64, input MQTTForwardTopicInput) (*MQTTForwardTopicRecord, error) {
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
	if err := s.db.Model(&MQTTForwardTopicRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetMQTTForwardTopic(id)
}

func (s *Store) DeleteMQTTForwardTopic(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&MQTTForwardTopicRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) GetMQTTForwarderConfig(id uint64) (*MQTTForwarderConfig, error) {
	forwarder, err := s.GetMQTTForwarder(id)
	if err != nil {
		return nil, err
	}
	var topics []MQTTForwardTopicRecord
	if err := s.db.Where("forwarder_id = ? AND enabled = ?", id, true).Order("id ASC").Find(&topics).Error; err != nil {
		return nil, err
	}
	return &MQTTForwarderConfig{Forwarder: *forwarder, Topics: topics}, nil
}

func (s *Store) ListEnabledMQTTForwarderConfigs() ([]MQTTForwarderConfig, error) {
	var forwarders []MQTTForwarderRecord
	if err := s.db.Where("enabled = ?", true).Order("id ASC").Find(&forwarders).Error; err != nil {
		return nil, err
	}
	configs := make([]MQTTForwarderConfig, 0, len(forwarders))
	for _, forwarder := range forwarders {
		var topics []MQTTForwardTopicRecord
		if err := s.db.Where("forwarder_id = ? AND enabled = ?", forwarder.ID, true).Order("id ASC").Find(&topics).Error; err != nil {
			return nil, err
		}
		if len(topics) == 0 {
			continue
		}
		configs = append(configs, MQTTForwarderConfig{Forwarder: forwarder, Topics: topics})
	}
	return configs, nil
}

func (s *Store) ensureMQTTForwarderNameUnique(id uint64, name string) error {
	var existing MQTTForwarderRecord
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
	return ErrMQTTForwarderAlreadyExists
}

func (s *Store) ensureMQTTForwardTopicUnique(id, forwarderID uint64, topic string) error {
	var existing MQTTForwardTopicRecord
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
	return ErrMQTTForwardTopicAlreadyExists
}

func mqttForwarderFromInput(input MQTTForwarderInput, existing *MQTTForwarderRecord) (*MQTTForwarderRecord, error) {
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
	row := &MQTTForwarderRecord{
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

func mqttForwardTopicFromInput(forwarderID uint64, input MQTTForwardTopicInput) (*MQTTForwardTopicRecord, error) {
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
	return &MQTTForwardTopicRecord{
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
		direction = MQTTForwardDirectionSourceToTarget
	}
	switch direction {
	case MQTTForwardDirectionSourceToTarget, MQTTForwardDirectionBidirectional:
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
