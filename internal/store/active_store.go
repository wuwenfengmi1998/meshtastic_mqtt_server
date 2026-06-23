package store

import (
	"time"
)

// CountActiveNodes 统计指定时间后有更新记录的节点数量
func (s *Store) CountActiveNodes(since time.Time) (int64, error) {
	var count int64
	err := s.db.Model(&NodeInfoRecord{}).
		Where("updated_at >= ?", since).
		Count(&count).Error
	return count, err
}

// CountActiveUsers 统计指定时间后发送过消息的唯一用户数（按 from_id 去重）
func (s *Store) CountActiveUsers(since time.Time) (int64, error) {
	var count int64
	err := s.db.Model(&TextMessageRecord{}).
		Where("created_at >= ?", since).
		Distinct("from_id").
		Count(&count).Error
	return count, err
}
