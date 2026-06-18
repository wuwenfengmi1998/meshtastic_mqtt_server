package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
)

const (
	defaultMapTileSourceName        = "OpenStreetMap Japan"
	defaultMapTileSourceURLTemplate = "https://tile.openstreetmap.jp/{z}/{x}/{y}.png"
	defaultMapTileSourceAttribution = "&copy; OpenStreetMap contributors"
	defaultMapTileSourceMaxZoom     = 19
	maxMapTileSourceURLLength       = 2048
)

var (
	ErrMapTileSourceAlreadyExists        = errors.New("map source already exists")
	ErrMapTileSourceCannotDeleteDefault  = errors.New("default map source cannot be deleted")
	ErrMapTileSourceCannotDisableDefault = errors.New("default map source cannot be disabled")
	ErrMapTileSourceDefaultMustBeEnabled = errors.New("default map source must be enabled")
)

type MapTileSourceInput struct {
	Name         string
	URLTemplate  string
	Attribution  string
	MaxZoom      int
	Enabled      bool
	IsDefault    bool
	ProxyEnabled bool
}

func (s *Store) ListMapTileSources(opts ListOptions) ([]MapTileSourceRecord, error) {
	opts = NormalizeListOptions(opts)
	var rows []MapTileSourceRecord
	q := s.db.Model(&MapTileSourceRecord{}).
		Order("is_default DESC").
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *Store) CountMapTileSources(opts ListOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&MapTileSourceRecord{}).Count(&total).Error
}

func (s *Store) ListEnabledMapTileSources() ([]MapTileSourceRecord, error) {
	var rows []MapTileSourceRecord
	if err := s.db.Model(&MapTileSourceRecord{}).
		Where("enabled = ?", true).
		Order("is_default DESC").
		Order("updated_at DESC").
		Order("id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []MapTileSourceRecord{defaultMapTileSourceRecord()}, nil
	}
	return rows, nil
}

func (s *Store) GetDefaultMapTileSource() (*MapTileSourceRecord, error) {
	var row MapTileSourceRecord
	err := s.db.Where("enabled = ? AND is_default = ?", true, true).Order("id ASC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fallback := defaultMapTileSourceRecord()
		return &fallback, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) GetEnabledMapTileSourceByHash(hash string) (*MapTileSourceRecord, error) {
	var row MapTileSourceRecord
	if err := s.db.Where("enabled = ? AND proxy_enabled = ? AND url_template_hash = ?", true, true, hash).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) CreateMapTileSource(input MapTileSourceInput) (*MapTileSourceRecord, error) {
	row, err := mapTileSourceFromInput(input)
	if err != nil {
		return nil, err
	}
	if row.IsDefault && !row.Enabled {
		return nil, ErrMapTileSourceDefaultMustBeEnabled
	}
	if err := s.ensureMapTileSourceUnique(0, row.Name, row.URLTemplate); err != nil {
		return nil, err
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if row.IsDefault {
			if err := tx.Model(&MapTileSourceRecord{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(row).Error
	}); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *Store) UpdateMapTileSource(id uint64, input MapTileSourceInput) (*MapTileSourceRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("map source id is required")
	}
	row, err := mapTileSourceFromInput(input)
	if err != nil {
		return nil, err
	}
	var updated MapTileSourceRecord
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing MapTileSourceRecord
		if err := tx.Where("id = ?", id).Take(&existing).Error; err != nil {
			return err
		}
		if existing.IsDefault && !row.Enabled {
			return ErrMapTileSourceCannotDisableDefault
		}
		if row.IsDefault && !row.Enabled {
			return ErrMapTileSourceDefaultMustBeEnabled
		}
		if !row.IsDefault && existing.IsDefault {
			row.IsDefault = true
		}
		if err := ensureMapTileSourceUniqueTx(tx, id, row.Name, row.URLTemplate); err != nil {
			return err
		}
		if row.IsDefault {
			if err := tx.Model(&MapTileSourceRecord{}).Where("id <> ? AND is_default = ?", id, true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		updates := map[string]any{
			"name":              row.Name,
			"url_template":      row.URLTemplate,
			"url_template_hash": row.URLTemplateHash,
			"attribution":       row.Attribution,
			"max_zoom":          row.MaxZoom,
			"enabled":           row.Enabled,
			"is_default":        row.IsDefault,
			"proxy_enabled":     row.ProxyEnabled,
			"updated_at":        time.Now(),
		}
		if err := tx.Model(&MapTileSourceRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Take(&updated).Error
	}); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *Store) DeleteMapTileSource(id uint64) error {
	if id == 0 {
		return fmt.Errorf("map source id is required")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row MapTileSourceRecord
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			return err
		}
		if row.IsDefault {
			return ErrMapTileSourceCannotDeleteDefault
		}
		result := tx.Where("id = ?", id).Delete(&MapTileSourceRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *Store) SetDefaultMapTileSource(id uint64) (*MapTileSourceRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("map source id is required")
	}
	var row MapTileSourceRecord
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			return err
		}
		if !row.Enabled {
			return ErrMapTileSourceDefaultMustBeEnabled
		}
		if err := tx.Model(&MapTileSourceRecord{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&MapTileSourceRecord{}).Where("id = ?", id).Updates(map[string]any{"is_default": true, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Take(&row).Error
	}); err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) EnsureDefaultMapTileSource() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&MapTileSourceRecord{}).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			row := defaultMapTileSourceRecord()
			return tx.Create(&row).Error
		}

		var defaults []MapTileSourceRecord
		if err := tx.Where("enabled = ? AND is_default = ?", true, true).Order("id ASC").Find(&defaults).Error; err != nil {
			return err
		}
		if len(defaults) > 0 {
			return tx.Model(&MapTileSourceRecord{}).Where("id <> ? AND is_default = ?", defaults[0].ID, true).Update("is_default", false).Error
		}

		var enabled MapTileSourceRecord
		err := tx.Where("enabled = ?", true).Order("id ASC").Take(&enabled).Error
		if err == nil {
			return tx.Model(&MapTileSourceRecord{}).Where("id = ?", enabled.ID).Updates(map[string]any{"is_default": true, "updated_at": time.Now()}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		row := defaultMapTileSourceRecord()
		var existing MapTileSourceRecord
		err = tx.Where("name = ? OR url_template = ?", row.Name, row.URLTemplate).Order("id ASC").Take(&existing).Error
		if err == nil {
			return tx.Model(&MapTileSourceRecord{}).Where("id = ?", existing.ID).Updates(map[string]any{"enabled": true, "is_default": true, "updated_at": time.Now()}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(&row).Error
	})
}

func MapTileSourceHash(urlTemplate string) string {
	h := sha256.Sum256([]byte(urlTemplate))
	return hex.EncodeToString(h[:])
}

func defaultMapTileSourceRecord() MapTileSourceRecord {
	return MapTileSourceRecord{
		Name:            defaultMapTileSourceName,
		URLTemplate:     defaultMapTileSourceURLTemplate,
		URLTemplateHash: MapTileSourceHash(defaultMapTileSourceURLTemplate),
		Attribution:     defaultMapTileSourceAttribution,
		MaxZoom:         defaultMapTileSourceMaxZoom,
		Enabled:         true,
		IsDefault:       true,
		ProxyEnabled:    true,
	}
}

func mapTileSourceFromInput(input MapTileSourceInput) (*MapTileSourceRecord, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("map source name is required")
	}
	urlTemplate, err := normalizeMapTileSourceURLTemplate(input.URLTemplate)
	if err != nil {
		return nil, err
	}
	maxZoom := input.MaxZoom
	if maxZoom == 0 {
		maxZoom = defaultMapTileSourceMaxZoom
	}
	if maxZoom < 1 || maxZoom > 30 {
		return nil, fmt.Errorf("max zoom must be between 1 and 30")
	}
	return &MapTileSourceRecord{
		Name:            name,
		URLTemplate:     urlTemplate,
		URLTemplateHash: MapTileSourceHash(urlTemplate),
		Attribution:     strings.TrimSpace(input.Attribution),
		MaxZoom:         maxZoom,
		Enabled:         input.Enabled,
		IsDefault:       input.IsDefault,
		ProxyEnabled:    input.ProxyEnabled,
	}, nil
}

func normalizeMapTileSourceURLTemplate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("map source url template is required")
	}
	if len(value) > maxMapTileSourceURLLength {
		return "", fmt.Errorf("map source url template is too long")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", fmt.Errorf("map source url template must not contain whitespace or control characters")
		}
	}
	for _, placeholder := range []string{"{z}", "{x}", "{y}"} {
		if strings.Count(value, placeholder) != 1 {
			return "", fmt.Errorf("map source url template must contain %s exactly once", placeholder)
		}
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("map source url template is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("map source url template must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("map source url template host is required")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("map source url template must not contain credentials")
	}
	return value, nil
}

func (s *Store) ensureMapTileSourceUnique(id uint64, name, urlTemplate string) error {
	return ensureMapTileSourceUniqueTx(s.db, id, name, urlTemplate)
}

func ensureMapTileSourceUniqueTx(tx *gorm.DB, id uint64, name, urlTemplate string) error {
	var existing MapTileSourceRecord
	q := tx.Where("name = ? OR url_template = ?", name, urlTemplate)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return ErrMapTileSourceAlreadyExists
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}
