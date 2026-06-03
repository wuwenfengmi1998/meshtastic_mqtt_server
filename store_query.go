package main

import (
	"time"

	"gorm.io/gorm"
)

type listOptions struct {
	Limit  int
	Offset int
	NodeID string
	Since  *time.Time
	Until  *time.Time
}

func (s *store) Ping() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Ping()
}

func normalizeListOptions(opts listOptions) listOptions {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 500 {
		opts.Limit = 500
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	return opts
}

func (s *store) ListNodeInfo(opts listOptions) ([]nodeInfoRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []nodeInfoRecord
	q := applyNodeFilters(s.db.Model(&nodeInfoRecord{}), opts).
		Order("updated_at DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountNodeInfo(opts listOptions) (int64, error) {
	var total int64
	q := applyNodeFilters(s.db.Model(&nodeInfoRecord{}), opts)
	return total, q.Count(&total).Error
}

func (s *store) GetNodeInfo(nodeID string) (*nodeInfoRecord, error) {
	var row nodeInfoRecord
	if err := s.db.Where("node_id = ?", nodeID).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) ListMapReports(opts listOptions) ([]mapReportRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []mapReportRecord
	q := applyNodeFilters(s.db.Model(&mapReportRecord{}), opts).
		Order("updated_at DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountMapReports(opts listOptions) (int64, error) {
	var total int64
	q := applyNodeFilters(s.db.Model(&mapReportRecord{}), opts)
	return total, q.Count(&total).Error
}

func (s *store) GetMapReport(nodeID string) (*mapReportRecord, error) {
	var row mapReportRecord
	if err := s.db.Where("node_id = ?", nodeID).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func applyNodeFilters(q *gorm.DB, opts listOptions) *gorm.DB {
	if opts.NodeID != "" {
		q = q.Where("node_id = ?", opts.NodeID)
	}
	if opts.Since != nil {
		q = q.Where("updated_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("updated_at <= ?", *opts.Until)
	}
	return q
}

func (s *store) ListTextMessages(opts listOptions) ([]textMessageRecord, error) {
	var rows []textMessageRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *store) ListPositions(opts listOptions) ([]positionRecord, error) {
	var rows []positionRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *store) ListTelemetry(opts listOptions) ([]telemetryRecord, error) {
	var rows []telemetryRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *store) ListRouting(opts listOptions) ([]routingRecord, error) {
	var rows []routingRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *store) ListTraceroute(opts listOptions) ([]tracerouteRecord, error) {
	var rows []tracerouteRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *store) listAppendRows(opts listOptions, dest any) *gorm.DB {
	opts = normalizeListOptions(opts)
	q := s.db.Order("created_at DESC").Order("id DESC").Limit(opts.Limit).Offset(opts.Offset)
	if opts.NodeID != "" {
		q = q.Where("from_id = ?", opts.NodeID)
	}
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return q.Find(dest)
}
