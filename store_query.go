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

func (s *store) ListNodes(opts listOptions) ([]nodeInfoMapRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []nodeInfoMapRecord
	q := s.db.Order("updated_at DESC").Limit(opts.Limit).Offset(opts.Offset)
	if opts.NodeID != "" {
		q = q.Where("node_id = ?", opts.NodeID)
	}
	if opts.Since != nil {
		q = q.Where("updated_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("updated_at <= ?", *opts.Until)
	}
	return rows, q.Find(&rows).Error
}

func (s *store) GetNode(nodeID string) (*nodeInfoMapRecord, error) {
	var row nodeInfoMapRecord
	if err := s.db.Where("node_id = ?", nodeID).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
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
