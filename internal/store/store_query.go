package store

import (
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
)

type ListOptions struct {
	Limit     int
	Offset    int
	NodeID    string
	ChannelID string
	Since     *time.Time
	Until     *time.Time
	MinLat    *float64
	MaxLat    *float64
	MinLng    *float64
	MaxLng    *float64
}

type MapReportViewportOptions struct {
	ListOptions      ListOptions
	Zoom             int
	Limit            int
	ClusterThreshold int
	TargetCells      int
}

type MapReportViewportResult struct {
	Mode     string
	Total    int64
	Points   []MapReportRecord
	Clusters []MapReportClusterRecord
	Limit    int
	Zoom     int
}

type MapReportClusterRecord struct {
	ClusterID string
	Latitude  float64
	Longitude float64
	Count     int64
}

func (s *Store) Ping() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Ping()
}

func NormalizeListOptions(opts ListOptions) ListOptions {
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

func (s *Store) ListNodeInfo(opts ListOptions) ([]NodeInfoRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []NodeInfoRecord
	q := applyNodeFilters(s.db.Model(&NodeInfoRecord{}), opts).
		Order("updated_at DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *Store) CountNodeInfo(opts ListOptions) (int64, error) {
	var total int64
	q := applyNodeFilters(s.db.Model(&NodeInfoRecord{}), opts)
	return total, q.Count(&total).Error
}

func (s *Store) GetNodeInfo(nodeID string) (*NodeInfoRecord, error) {
	var row NodeInfoRecord
	if err := s.db.Where("node_id = ?", nodeID).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) ListMapReports(opts ListOptions) ([]MapReportRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []MapReportRecord
	q := applyMapReportFilters(s.db.Model(&MapReportRecord{}), opts).
		Order("updated_at DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *Store) CountMapReports(opts ListOptions) (int64, error) {
	var total int64
	q := applyMapReportFilters(s.db.Model(&MapReportRecord{}), opts)
	return total, q.Count(&total).Error
}

func (s *Store) GetMapReport(nodeID string) (*MapReportRecord, error) {
	var row MapReportRecord
	if err := s.db.Where("node_id = ?", nodeID).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) ListMapReportViewport(opts MapReportViewportOptions) (*MapReportViewportResult, error) {
	opts = NormalizeMapReportViewportOptions(opts)
	total, err := s.CountMapReports(opts.ListOptions)
	if err != nil {
		return nil, err
	}
	result := &MapReportViewportResult{Total: total, Limit: opts.Limit, Zoom: opts.Zoom}
	if total <= int64(opts.ClusterThreshold) {
		var points []MapReportRecord
		q := applyMapReportFilters(s.db.Model(&MapReportRecord{}), opts.ListOptions).
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

func (s *Store) ListMapReportClusters(opts MapReportViewportOptions) ([]MapReportClusterRecord, error) {
	opts = NormalizeMapReportViewportOptions(opts)
	cellSize := mapReportClusterCellSize(opts.ListOptions, opts.TargetCells)
	var rows []struct {
		LatBucket int64
		LngBucket int64
		Latitude  float64
		Longitude float64
		Count     int64
	}
	q := applyMapReportFilters(s.db.Model(&MapReportRecord{}), opts.ListOptions).
		Select("CAST((latitude + 90.0) / ? AS INTEGER) AS lat_bucket, CAST((longitude + 180.0) / ? AS INTEGER) AS lng_bucket, AVG(latitude) AS latitude, AVG(longitude) AS longitude, COUNT(*) AS count", cellSize, cellSize).
		Group("lat_bucket, lng_bucket").
		Order("count DESC").
		Limit(opts.Limit)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	clusters := make([]MapReportClusterRecord, 0, len(rows))
	for _, row := range rows {
		clusters = append(clusters, MapReportClusterRecord{
			ClusterID: fmt.Sprintf("%d:%d", row.LatBucket, row.LngBucket),
			Latitude:  row.Latitude,
			Longitude: row.Longitude,
			Count:     row.Count,
		})
	}
	return clusters, nil
}

func NormalizeMapReportViewportOptions(opts MapReportViewportOptions) MapReportViewportOptions {
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

func mapReportClusterCellSize(opts ListOptions, targetCells int) float64 {
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

func (s *Store) DeleteNode(nodeID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		nodeResult := tx.Where("node_id = ?", nodeID).Delete(&NodeInfoRecord{})
		if nodeResult.Error != nil {
			return nodeResult.Error
		}
		reportResult := tx.Where("node_id = ?", nodeID).Delete(&MapReportRecord{})
		if reportResult.Error != nil {
			return reportResult.Error
		}
		if nodeResult.RowsAffected+reportResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// PurgeNode 在「删除节点」菜单触发时执行：除了 nodeinfo + map_report，
// 还要把 text_message（频道聊天）以及 position/telemetry/routing/traceroute
// 这些以 from_id 关联的数据包记录一起清理。
//
// 任一表删到记录就视为成功；全部为空才返回 ErrRecordNotFound。
func (s *Store) PurgeNode(nodeID string) error {
	if nodeID == "" {
		return gorm.ErrRecordNotFound
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var totalAffected int64

		nodeResult := tx.Where("node_id = ?", nodeID).Delete(&NodeInfoRecord{})
		if nodeResult.Error != nil {
			return nodeResult.Error
		}
		totalAffected += nodeResult.RowsAffected

		reportResult := tx.Where("node_id = ?", nodeID).Delete(&MapReportRecord{})
		if reportResult.Error != nil {
			return reportResult.Error
		}
		totalAffected += reportResult.RowsAffected

		// 以 from_id 关联：聊天消息 + 数据包流水
		fromIDTargets := []any{
			&TextMessageRecord{},
			&PositionRecord{},
			&TelemetryRecord{},
			&RoutingRecord{},
			&TracerouteRecord{},
		}
		for _, model := range fromIDTargets {
			res := tx.Where("from_id = ?", nodeID).Delete(model)
			if res.Error != nil {
				return res.Error
			}
			totalAffected += res.RowsAffected
		}

		if totalAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func applyNodeFilters(q *gorm.DB, opts ListOptions) *gorm.DB {
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

func applyMapReportFilters(q *gorm.DB, opts ListOptions) *gorm.DB {
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

func (s *Store) ListTextMessages(opts ListOptions) ([]TextMessageRecord, error) {
	var rows []TextMessageRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *Store) ListDiscardDetails(opts ListOptions) ([]DiscardDetailsRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []DiscardDetailsRecord
	q := applyDiscardDetailsFilters(s.db.Model(&DiscardDetailsRecord{}), opts).
		Order("created_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *Store) CountDiscardDetails(opts ListOptions) (int64, error) {
	var total int64
	q := applyDiscardDetailsFilters(s.db.Model(&DiscardDetailsRecord{}), opts)
	return total, q.Count(&total).Error
}

func applyDiscardDetailsFilters(q *gorm.DB, opts ListOptions) *gorm.DB {
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return q
}

func (s *Store) DeleteTextMessage(id uint64) error {
	result := s.db.Where("id = ?", id).Delete(&TextMessageRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Store) ListPositions(opts ListOptions) ([]PositionRecord, error) {
	var rows []PositionRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *Store) ListTelemetry(opts ListOptions) ([]TelemetryRecord, error) {
	var rows []TelemetryRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *Store) ListRouting(opts ListOptions) ([]RoutingRecord, error) {
	var rows []RoutingRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *Store) ListTraceroute(opts ListOptions) ([]TracerouteRecord, error) {
	var rows []TracerouteRecord
	return rows, s.listAppendRows(opts, &rows).Error
}

func (s *Store) listAppendRows(opts ListOptions, dest any) *gorm.DB {
	opts = NormalizeListOptions(opts)
	q := s.db.Order("created_at DESC").Order("id DESC").Limit(opts.Limit).Offset(opts.Offset)
	if opts.NodeID != "" {
		q = q.Where("from_id = ?", opts.NodeID)
	}
	if opts.ChannelID != "" {
		q = q.Where("channel_id = ?", opts.ChannelID)
	}
	if opts.Since != nil {
		q = q.Where("created_at >= ?", *opts.Since)
	}
	if opts.Until != nil {
		q = q.Where("created_at <= ?", *opts.Until)
	}
	return q.Find(dest)
}
