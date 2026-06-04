package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"gorm.io/gorm"
)

const forbiddenWordMatchContains = "contains"

var errBlockingAlreadyExists = errors.New("blocking rule already exists")

func (s *store) ListNodeBlocking(opts listOptions) ([]nodeBlockingRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []nodeBlockingRecord
	q := s.db.Model(&nodeBlockingRecord{}).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountNodeBlocking(opts listOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&nodeBlockingRecord{}).Count(&total).Error
}

func (s *store) ListEnabledNodeBlocking() ([]nodeBlockingRecord, error) {
	var rows []nodeBlockingRecord
	return rows, s.db.Where("enabled = ?", true).Find(&rows).Error
}

func (s *store) CreateNodeBlocking(nodeID string, nodeNum *int64, reason string, enabled bool) (*nodeBlockingRecord, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("node id is required")
	}
	if err := s.ensureNodeBlockingUnique(0, nodeID); err != nil {
		return nil, err
	}
	row := nodeBlockingRecord{NodeID: nodeID, NodeNum: nodeNum, Reason: strings.TrimSpace(reason), Enabled: enabled}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) UpdateNodeBlocking(id uint64, nodeID string, nodeNum *int64, reason string, enabled bool) (*nodeBlockingRecord, error) {
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
	if err := s.db.Model(&nodeBlockingRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.getNodeBlockingByID(id)
}

func (s *store) DeleteNodeBlocking(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&nodeBlockingRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *store) ListIPBlocking(opts listOptions) ([]ipBlockingRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []ipBlockingRecord
	q := s.db.Model(&ipBlockingRecord{}).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountIPBlocking(opts listOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&ipBlockingRecord{}).Count(&total).Error
}

func (s *store) ListEnabledIPBlocking() ([]ipBlockingRecord, error) {
	var rows []ipBlockingRecord
	return rows, s.db.Where("enabled = ?", true).Find(&rows).Error
}

func (s *store) CreateIPBlocking(ipValue string, reason string, enabled bool) (*ipBlockingRecord, error) {
	value, err := normalizeIPBlockingValue(ipValue)
	if err != nil {
		return nil, err
	}
	if err := s.ensureIPBlockingUnique(0, value); err != nil {
		return nil, err
	}
	row := ipBlockingRecord{IPValue: value, Reason: strings.TrimSpace(reason), Enabled: enabled}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) UpdateIPBlocking(id uint64, ipValue string, reason string, enabled bool) (*ipBlockingRecord, error) {
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
	if err := s.db.Model(&ipBlockingRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.getIPBlockingByID(id)
}

func (s *store) DeleteIPBlocking(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&ipBlockingRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *store) ListForbiddenWordBlocking(opts listOptions) ([]forbiddenWordBlockingRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []forbiddenWordBlockingRecord
	q := s.db.Model(&forbiddenWordBlockingRecord{}).
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountForbiddenWordBlocking(opts listOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&forbiddenWordBlockingRecord{}).Count(&total).Error
}

func (s *store) ListEnabledForbiddenWordBlocking() ([]forbiddenWordBlockingRecord, error) {
	var rows []forbiddenWordBlockingRecord
	return rows, s.db.Where("enabled = ?", true).Find(&rows).Error
}

func (s *store) CreateForbiddenWordBlocking(word, matchType string, caseSensitive bool, reason string, enabled bool) (*forbiddenWordBlockingRecord, error) {
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
	row := forbiddenWordBlockingRecord{Word: word, MatchType: matchType, CaseSensitive: caseSensitive, Reason: strings.TrimSpace(reason), Enabled: enabled}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) UpdateForbiddenWordBlocking(id uint64, word, matchType string, caseSensitive bool, reason string, enabled bool) (*forbiddenWordBlockingRecord, error) {
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
	if err := s.db.Model(&forbiddenWordBlockingRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.getForbiddenWordBlockingByID(id)
}

func (s *store) DeleteForbiddenWordBlocking(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&forbiddenWordBlockingRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *store) getNodeBlockingByID(id uint64) (*nodeBlockingRecord, error) {
	var row nodeBlockingRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) getIPBlockingByID(id uint64) (*ipBlockingRecord, error) {
	var row ipBlockingRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) getForbiddenWordBlockingByID(id uint64) (*forbiddenWordBlockingRecord, error) {
	var row forbiddenWordBlockingRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) ensureNodeBlockingUnique(id uint64, nodeID string) error {
	var existing nodeBlockingRecord
	q := s.db.Where("node_id = ?", nodeID)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return errBlockingAlreadyExists
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (s *store) ensureIPBlockingUnique(id uint64, ipValue string) error {
	var existing ipBlockingRecord
	q := s.db.Where("ip_value = ?", ipValue)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return errBlockingAlreadyExists
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (s *store) ensureForbiddenWordBlockingUnique(id uint64, word string) error {
	var existing forbiddenWordBlockingRecord
	q := s.db.Where("word = ?", word)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return errBlockingAlreadyExists
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
		return forbiddenWordMatchContains, nil
	}
	if matchType != forbiddenWordMatchContains {
		return "", fmt.Errorf("unsupported forbidden word match type")
	}
	return matchType, nil
}
