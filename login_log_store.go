package main

func (s *store) InsertLoginLog(log loginLogRecord) error {
	return s.db.Create(&log).Error
}

func (s *store) ListLoginLogs(opts listOptions) ([]loginLogRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []loginLogRecord
	q := s.db.Order("created_at DESC").Order("id DESC").Limit(opts.Limit).Offset(opts.Offset)
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return rows, q.Find(&rows).Error
}
