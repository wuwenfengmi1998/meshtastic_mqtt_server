package store

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"meshtastic_mqtt_server/internal/config"
)

func (s *Store) ListSigns(opts ListOptions) ([]SignRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []SignRecord
	q := applySignFilters(s.db.Model(&SignRecord{}), opts).
		Order("sign_time DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

type SignDayCount struct {
	Date  string `gorm:"column:sign_date"`
	Count int64  `gorm:"column:count"`
}

func (s *Store) CountSigns(opts ListOptions) (int64, error) {
	var total int64
	q := applySignFilters(s.db.Model(&SignRecord{}), opts)
	return total, q.Count(&total).Error
}

func (s *Store) CountSignsByDay(opts ListOptions) ([]SignDayCount, error) {
	var rows []SignDayCount
	dateExpr := "strftime('%Y-%m-%d', sign_time)"
	if s.driver == config.DriverMySQL {
		dateExpr = "DATE_FORMAT(sign_time, '%Y-%m-%d')"
	}
	q := applySignFilters(s.db.Model(&SignRecord{}), opts).
		Select(dateExpr + " AS sign_date, COUNT(*) AS count").
		Group(dateExpr).
		Order("sign_date DESC")
	return rows, q.Scan(&rows).Error
}

func (s *Store) GetSignByID(id uint64) (*SignRecord, error) {
	var row SignRecord
	if err := s.db.Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) CreateSign(nodeID string, longName, shortName *string, signText string, signTime time.Time) (*SignRecord, error) {
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
	row := SignRecord{NodeID: nodeID, LongName: trimNullableString(longName), ShortName: trimNullableString(shortName), SignText: signText, SignTime: signTime}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) UpdateSign(id uint64, nodeID string, longName, shortName *string, signText string, signTime time.Time) (*SignRecord, error) {
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
	if err := s.db.Model(&SignRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetSignByID(id)
}

func (s *Store) DeleteSign(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&SignRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func applySignFilters(q *gorm.DB, opts ListOptions) *gorm.DB {
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
