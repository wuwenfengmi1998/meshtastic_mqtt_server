package store

import "time"

func (s *Store) InsertLoginLog(log LoginLogRecord) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	return s.db.Create(&log).Error
}

func (s *Store) ListLoginLogs(opts ListOptions) ([]LoginLogRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []LoginLogRecord
	q := s.db.Order("id DESC").Limit(opts.Limit).Offset(opts.Offset)
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return rows, q.Find(&rows).Error
}
