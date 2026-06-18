package store

func (s *Store) InsertLoginLog(log LoginLogRecord) error {
	return s.db.Create(&log).Error
}

func (s *Store) ListLoginLogs(opts ListOptions) ([]LoginLogRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []LoginLogRecord
	q := s.db.Order("created_at DESC").Order("id DESC").Limit(opts.Limit).Offset(opts.Offset)
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return rows, q.Find(&rows).Error
}
