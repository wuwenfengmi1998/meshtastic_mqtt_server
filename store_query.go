package main

import (
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
)

type listOptions struct {
	Limit  int
	Offset int
	NodeID string
	Since  *time.Time
	Until  *time.Time
	MinLat *float64
	MaxLat *float64
	MinLng *float64
	MaxLng *float64
}

type mapReportViewportOptions struct {
	ListOptions      listOptions
	Zoom             int
	Limit            int
	ClusterThreshold int
	TargetCells      int
}

type mapReportViewportResult struct {
	Mode     string
	Total    int64
	Points   []mapReportRecord
	Clusters []mapReportClusterRecord
	Limit    int
	Zoom     int
}

type mapReportClusterRecord struct {
	ClusterID string
	Latitude  float64
	Longitude float64
	Count     int64
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
	q := applyMapReportFilters(s.db.Model(&mapReportRecord{}), opts).
		Order("updated_at DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountMapReports(opts listOptions) (int64, error) {
	var total int64
	q := applyMapReportFilters(s.db.Model(&mapReportRecord{}), opts)
	return total, q.Count(&total).Error
}

func (s *store) GetMapReport(nodeID string) (*mapReportRecord, error) {
	var row mapReportRecord
	if err := s.db.Where("node_id = ?", nodeID).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) ListMapReportViewport(opts mapReportViewportOptions) (*mapReportViewportResult, error) {
	opts = normalizeMapReportViewportOptions(opts)
	total, err := s.CountMapReports(opts.ListOptions)
	if err != nil {
		return nil, err
	}
	result := &mapReportViewportResult{Total: total, Limit: opts.Limit, Zoom: opts.Zoom}
	if total <= int64(opts.ClusterThreshold) {
		var points []mapReportRecord
		q := applyMapReportFilters(s.db.Model(&mapReportRecord{}), opts.ListOptions).
			Order("updated_at DESC").
			Limit(opts.Limit)
		if err := q.Find(&points).Error; err != nil {
			return nil, err
		}
		result.Mode = "points"
		result.Points = points
		return result, nil
	}
	clusters, err := s.ListMapReportClusters(opts)
	if err != nil {
		return nil, err
	}
	result.Mode = "clusters"
	result.Clusters = clusters
	return result, nil
}

func (s *store) ListMapReportClusters(opts mapReportViewportOptions) ([]mapReportClusterRecord, error) {
	opts = normalizeMapReportViewportOptions(opts)
	cellSize := mapReportClusterCellSize(opts.ListOptions, opts.TargetCells)
	var rows []struct {
		LatBucket int64
		LngBucket int64
		Latitude  float64
		Longitude float64
		Count     int64
	}
	q := applyMapReportFilters(s.db.Model(&mapReportRecord{}), opts.ListOptions).
		Select("CAST((latitude + 90.0) / ? AS INTEGER) AS lat_bucket, CAST((longitude + 180.0) / ? AS INTEGER) AS lng_bucket, AVG(latitude) AS latitude, AVG(longitude) AS longitude, COUNT(*) AS count", cellSize, cellSize).
		Group("lat_bucket, lng_bucket").
		Order("count DESC").
		Limit(opts.Limit)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	clusters := make([]mapReportClusterRecord, 0, len(rows))
	for _, row := range rows {
		clusters = append(clusters, mapReportClusterRecord{
			ClusterID: fmt.Sprintf("%d:%d", row.LatBucket, row.LngBucket),
			Latitude:  row.Latitude,
			Longitude: row.Longitude,
			Count:     row.Count,
		})
	}
	return clusters, nil
}

func normalizeMapReportViewportOptions(opts mapReportViewportOptions) mapReportViewportOptions {
	if opts.Limit <= 0 {
		opts.Limit = 1000
	}
	if opts.Limit > 2000 {
		opts.Limit = 2000
	}
	if opts.ClusterThreshold <= 0 {
		opts.ClusterThreshold = 500
	}
	if opts.ClusterThreshold > 5000 {
		opts.ClusterThreshold = 5000
	}
	if opts.TargetCells <= 0 {
		opts.TargetCells = 64
	}
	if opts.TargetCells > 256 {
		opts.TargetCells = 256
	}
	return opts
}

func mapReportClusterCellSize(opts listOptions, targetCells int) float64 {
	latSpan := 180.0
	if opts.MinLat != nil && opts.MaxLat != nil {
		latSpan = *opts.MaxLat - *opts.MinLat
	}
	lngSpan := 360.0
	if opts.MinLng != nil && opts.MaxLng != nil {
		if *opts.MinLng <= *opts.MaxLng {
			lngSpan = *opts.MaxLng - *opts.MinLng
		} else {
			lngSpan = 180 - *opts.MinLng + *opts.MaxLng + 180
		}
	}
	span := math.Max(latSpan, lngSpan)
	cellSize := span / float64(targetCells)
	if cellSize < 0.0001 {
		cellSize = 0.0001
	}
	return cellSize
}

func (s *store) DeleteNode(nodeID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		nodeResult := tx.Where("node_id = ?", nodeID).Delete(&nodeInfoRecord{})
		if nodeResult.Error != nil {
			return nodeResult.Error
		}
		reportResult := tx.Where("node_id = ?", nodeID).Delete(&mapReportRecord{})
		if reportResult.Error != nil {
			return reportResult.Error
		}
		if nodeResult.RowsAffected+reportResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
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

func applyMapReportFilters(q *gorm.DB, opts listOptions) *gorm.DB {
	q = applyNodeFilters(q, opts)
	if opts.MinLat != nil && opts.MaxLat != nil {
		q = q.Where("latitude IS NOT NULL AND latitude >= ? AND latitude <= ?", *opts.MinLat, *opts.MaxLat)
	}
	if opts.MinLng != nil && opts.MaxLng != nil {
		if *opts.MinLng <= *opts.MaxLng {
			q = q.Where("longitude IS NOT NULL AND longitude >= ? AND longitude <= ?", *opts.MinLng, *opts.MaxLng)
		} else {
			q = q.Where("longitude IS NOT NULL AND (longitude >= ? OR longitude <= ?)", *opts.MinLng, *opts.MaxLng)
		}
	}
	return q
}

func (s *store) ListTextMessages(opts listOptions) ([]textMessageRecord, error) {
	var rows []textMessageRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *store) ListDiscardDetails(opts listOptions) ([]discardDetailsRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []discardDetailsRecord
	q := applyDiscardDetailsFilters(s.db.Model(&discardDetailsRecord{}), opts).
		Order("created_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountDiscardDetails(opts listOptions) (int64, error) {
	var total int64
	q := applyDiscardDetailsFilters(s.db.Model(&discardDetailsRecord{}), opts)
	return total, q.Count(&total).Error
}

func applyDiscardDetailsFilters(q *gorm.DB, opts listOptions) *gorm.DB {
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return q
}

func (s *store) DeleteTextMessage(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&textMessageRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
