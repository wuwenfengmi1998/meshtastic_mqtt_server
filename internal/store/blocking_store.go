package store

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"gorm.io/gorm"
)

const ForbiddenWordMatchContains = "contains"

var ErrBlockingAlreadyExists = errors.New("blocking rule already exists")

func (s *Store) ListNodeBlocking(opts ListOptions) ([]NodeBlockingRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []NodeBlockingRecord
	q := s.db.Model(&NodeBlockingRecord{}).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *Store) CountNodeBlocking(opts ListOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&NodeBlockingRecord{}).Count(&total).Error
}

func (s *Store) ListEnabledNodeBlocking() ([]NodeBlockingRecord, error) {
	var rows []NodeBlockingRecord
	return rows, s.db.Where("enabled = ?", true).Find(&rows).Error
}

func (s *Store) CreateNodeBlocking(nodeID string, nodeNum *int64, reason string, enabled bool) (*NodeBlockingRecord, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("node id is required")
	}
	if err := s.ensureNodeBlockingUnique(0, nodeID); err != nil {
		return nil, err
	}
	row := NodeBlockingRecord{NodeID: nodeID, NodeNum: nodeNum, Reason: strings.TrimSpace(reason), Enabled: enabled}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) UpdateNodeBlocking(id uint64, nodeID string, nodeNum *int64, reason string, enabled bool) (*NodeBlockingRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("blocking rule id is required")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("node id is required")
	}
	if _, err := s.getNodeBlockingByID(id); err != nil {
		return nil, err
	}
	if err := s.ensureNodeBlockingUnique(id, nodeID); err != nil {
		return nil, err
	}
	updates := map[string]any{"node_id": nodeID, "node_num": nodeNum, "reason": strings.TrimSpace(reason), "enabled": enabled, "updated_at": time.Now()}
	if err := s.db.Model(&NodeBlockingRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.getNodeBlockingByID(id)
}

func (s *Store) DeleteNodeBlocking(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&NodeBlockingRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) ListIPBlocking(opts ListOptions) ([]IPBlockingRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []IPBlockingRecord
	q := s.db.Model(&IPBlockingRecord{}).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *Store) CountIPBlocking(opts ListOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&IPBlockingRecord{}).Count(&total).Error
}

func (s *Store) ListEnabledIPBlocking() ([]IPBlockingRecord, error) {
	var rows []IPBlockingRecord
	return rows, s.db.Where("enabled = ?", true).Find(&rows).Error
}

func (s *Store) CreateIPBlocking(ipValue string, reason string, enabled bool) (*IPBlockingRecord, error) {
	value, err := normalizeIPBlockingValue(ipValue)
	if err != nil {
		return nil, err
	}
	if err := s.ensureIPBlockingUnique(0, value); err != nil {
		return nil, err
	}
	row := IPBlockingRecord{IPValue: value, Reason: strings.TrimSpace(reason), Enabled: enabled}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) UpdateIPBlocking(id uint64, ipValue string, reason string, enabled bool) (*IPBlockingRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("blocking rule id is required")
	}
	value, err := normalizeIPBlockingValue(ipValue)
	if err != nil {
		return nil, err
	}
	if _, err := s.getIPBlockingByID(id); err != nil {
		return nil, err
	}
	if err := s.ensureIPBlockingUnique(id, value); err != nil {
		return nil, err
	}
	updates := map[string]any{"ip_value": value, "reason": strings.TrimSpace(reason), "enabled": enabled, "updated_at": time.Now()}
	if err := s.db.Model(&IPBlockingRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.getIPBlockingByID(id)
}

func (s *Store) DeleteIPBlocking(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&IPBlockingRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) ListForbiddenWordBlocking(opts ListOptions) ([]ForbiddenWordBlockingRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []ForbiddenWordBlockingRecord
	q := s.db.Model(&ForbiddenWordBlockingRecord{}).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *Store) CountForbiddenWordBlocking(opts ListOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&ForbiddenWordBlockingRecord{}).Count(&total).Error
}

func (s *Store) ListEnabledForbiddenWordBlocking() ([]ForbiddenWordBlockingRecord, error) {
	var rows []ForbiddenWordBlockingRecord
	return rows, s.db.Where("enabled = ?", true).Find(&rows).Error
}

func (s *Store) CreateForbiddenWordBlocking(word, matchType string, caseSensitive bool, reason string, enabled bool) (*ForbiddenWordBlockingRecord, error) {
	word = strings.TrimSpace(word)
	if word == "" {
		return nil, fmt.Errorf("forbidden word is required")
	}
	matchType, err := normalizeForbiddenWordMatchType(matchType)
	if err != nil {
		return nil, err
	}
	if err := s.ensureForbiddenWordBlockingUnique(0, word); err != nil {
		return nil, err
	}
	row := ForbiddenWordBlockingRecord{Word: word, MatchType: matchType, CaseSensitive: caseSensitive, Reason: strings.TrimSpace(reason), Enabled: enabled}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) UpdateForbiddenWordBlocking(id uint64, word, matchType string, caseSensitive bool, reason string, enabled bool) (*ForbiddenWordBlockingRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("blocking rule id is required")
	}
	word = strings.TrimSpace(word)
	if word == "" {
		return nil, fmt.Errorf("forbidden word is required")
	}
	matchType, err := normalizeForbiddenWordMatchType(matchType)
	if err != nil {
		return nil, err
	}
	if _, err := s.getForbiddenWordBlockingByID(id); err != nil {
		return nil, err
	}
	if err := s.ensureForbiddenWordBlockingUnique(id, word); err != nil {
		return nil, err
	}
	updates := map[string]any{"word": word, "match_type": matchType, "case_sensitive": caseSensitive, "reason": strings.TrimSpace(reason), "enabled": enabled, "updated_at": time.Now()}
	if err := s.db.Model(&ForbiddenWordBlockingRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.getForbiddenWordBlockingByID(id)
}

func (s *Store) DeleteForbiddenWordBlocking(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&ForbiddenWordBlockingRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) getNodeBlockingByID(id uint64) (*NodeBlockingRecord, error) {
	var row NodeBlockingRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) getIPBlockingByID(id uint64) (*IPBlockingRecord, error) {
	var row IPBlockingRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) getForbiddenWordBlockingByID(id uint64) (*ForbiddenWordBlockingRecord, error) {
	var row ForbiddenWordBlockingRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) ensureNodeBlockingUnique(id uint64, nodeID string) error {
	var existing NodeBlockingRecord
	q := s.db.Where("node_id = ?", nodeID)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return ErrBlockingAlreadyExists
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (s *Store) ensureIPBlockingUnique(id uint64, ipValue string) error {
	var existing IPBlockingRecord
	q := s.db.Where("ip_value = ?", ipValue)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return ErrBlockingAlreadyExists
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (s *Store) ensureForbiddenWordBlockingUnique(id uint64, word string) error {
	var existing ForbiddenWordBlockingRecord
	q := s.db.Where("word = ?", word)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return ErrBlockingAlreadyExists
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func normalizeIPBlockingValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("ip value is required")
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String(), nil
	}
	_, ipNet, err := net.ParseCIDR(value)
	if err == nil {
		return ipNet.String(), nil
	}
	return "", fmt.Errorf("ip value must be a valid IP or CIDR")
}

func normalizeForbiddenWordMatchType(matchType string) (string, error) {
	matchType = strings.TrimSpace(matchType)
	if matchType == "" {
		return ForbiddenWordMatchContains, nil
	}
	if matchType != ForbiddenWordMatchContains {
		return "", fmt.Errorf("unsupported forbidden word match type")
	}
	return matchType, nil
}
