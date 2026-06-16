package main

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

func (s *store) ListSigns(opts listOptions) ([]signRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []signRecord
	q := applySignFilters(s.db.Model(&signRecord{}), opts).
		Order("sign_time DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

type signDayCount struct {
	Date  string `gorm:"column:sign_date"`
	Count int64  `gorm:"column:count"`
}

func (s *store) CountSigns(opts listOptions) (int64, error) {
	var total int64
	q := applySignFilters(s.db.Model(&signRecord{}), opts)
	return total, q.Count(&total).Error
}

func (s *store) CountSignsByDay(opts listOptions) ([]signDayCount, error) {
	var rows []signDayCount
	dateExpr := "date(sign_time)"
	if s.driver == databaseDriverMySQL {
		dateExpr = "DATE(sign_time)"
	}
	q := applySignFilters(s.db.Model(&signRecord{}), opts).
		Select(dateExpr + " AS sign_date, COUNT(*) AS count").
		Group(dateExpr).
		Order("sign_date DESC")
	return rows, q.Scan(&rows).Error
}

func (s *store) GetSignByID(id uint64) (*signRecord, error) {
	var row signRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) CreateSign(nodeID string, longName, shortName *string, signText string, signTime time.Time) (*signRecord, error) {
	nodeID = strings.TrimSpace(nodeID)
	signText = strings.TrimSpace(signText)
	if nodeID == "" {
		return nil, fmt.Errorf("node id is required")
	}
	if signText == "" {
		return nil, fmt.Errorf("sign text is required")
	}
	if signTime.IsZero() {
		signTime = time.Now()
	}
	row := signRecord{NodeID: nodeID, LongName: trimNullableString(longName), ShortName: trimNullableString(shortName), SignText: signText, SignTime: signTime}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) UpdateSign(id uint64, nodeID string, longName, shortName *string, signText string, signTime time.Time) (*signRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("sign id is required")
	}
	nodeID = strings.TrimSpace(nodeID)
	signText = strings.TrimSpace(signText)
	if nodeID == "" {
		return nil, fmt.Errorf("node id is required")
	}
	if signText == "" {
		return nil, fmt.Errorf("sign text is required")
	}
	if signTime.IsZero() {
		signTime = time.Now()
	}
	if _, err := s.GetSignByID(id); err != nil {
		return nil, err
	}
	updates := map[string]any{"node_id": nodeID, "long_name": trimNullableString(longName), "short_name": trimNullableString(shortName), "sign_text": signText, "sign_time": signTime}
	if err := s.db.Model(&signRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetSignByID(id)
}

func (s *store) DeleteSign(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&signRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func applySignFilters(q *gorm.DB, opts listOptions) *gorm.DB {
	if opts.NodeID != "" {
		q = q.Where("node_id = ?", opts.NodeID)
	}
	if opts.Since != nil {
		q = q.Where("sign_time >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("sign_time <= ?", *opts.Until)
	}
	return q
}

func trimNullableString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
