package main

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
	errMapTileSourceAlreadyExists        = errors.New("map source already exists")
	errMapTileSourceCannotDeleteDefault  = errors.New("default map source cannot be deleted")
	errMapTileSourceCannotDisableDefault = errors.New("default map source cannot be disabled")
	errMapTileSourceDefaultMustBeEnabled = errors.New("default map source must be enabled")
)

type mapTileSourceInput struct {
	Name        string
	URLTemplate string
	Attribution string
	MaxZoom     int
	Enabled     bool
	IsDefault   bool
}

func (s *store) ListMapTileSources(opts listOptions) ([]mapTileSourceRecord, error) {
	opts = normalizeListOptions(opts)
	var rows []mapTileSourceRecord
	q := s.db.Model(&mapTileSourceRecord{}).
		Order("is_default DESC").
		Order("updated_at DESC").
		Order("id DESC").
		Limit(opts.Limit).
		Offset(opts.Offset)
	return rows, q.Find(&rows).Error
}

func (s *store) CountMapTileSources(opts listOptions) (int64, error) {
	var total int64
	return total, s.db.Model(&mapTileSourceRecord{}).Count(&total).Error
}

func (s *store) ListEnabledMapTileSources() ([]mapTileSourceRecord, error) {
	var rows []mapTileSourceRecord
	if err := s.db.Model(&mapTileSourceRecord{}).
		Where("enabled = ?", true).
		Order("is_default DESC").
		Order("updated_at DESC").
		Order("id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []mapTileSourceRecord{defaultMapTileSourceRecord()}, nil
	}
	return rows, nil
}

func (s *store) GetDefaultMapTileSource() (*mapTileSourceRecord, error) {
	var row mapTileSourceRecord
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

func (s *store) GetEnabledMapTileSourceByHash(hash string) (*mapTileSourceRecord, error) {
	var row mapTileSourceRecord
	if err := s.db.Where("enabled = ? AND url_template_hash = ?", true, hash).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) CreateMapTileSource(input mapTileSourceInput) (*mapTileSourceRecord, error) {
	row, err := mapTileSourceFromInput(input)
	if err != nil {
		return nil, err
	}
	if row.IsDefault && !row.Enabled {
		return nil, errMapTileSourceDefaultMustBeEnabled
	}
	if err := s.ensureMapTileSourceUnique(0, row.Name, row.URLTemplate); err != nil {
		return nil, err
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if row.IsDefault {
			if err := tx.Model(&mapTileSourceRecord{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(row).Error
	}); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *store) UpdateMapTileSource(id uint64, input mapTileSourceInput) (*mapTileSourceRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("map source id is required")
	}
	row, err := mapTileSourceFromInput(input)
	if err != nil {
		return nil, err
	}
	var updated mapTileSourceRecord
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing mapTileSourceRecord
		if err := tx.Where("id = ?", id).Take(&existing).Error; err != nil {
			return err
		}
		if existing.IsDefault && !row.Enabled {
			return errMapTileSourceCannotDisableDefault
		}
		if row.IsDefault && !row.Enabled {
			return errMapTileSourceDefaultMustBeEnabled
		}
		if !row.IsDefault && existing.IsDefault {
			row.IsDefault = true
		}
		if err := ensureMapTileSourceUniqueTx(tx, id, row.Name, row.URLTemplate); err != nil {
			return err
		}
		if row.IsDefault {
			if err := tx.Model(&mapTileSourceRecord{}).Where("id <> ? AND is_default = ?", id, true).Update("is_default", false).Error; err != nil {
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
			"updated_at":        time.Now(),
		}
		if err := tx.Model(&mapTileSourceRecord{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Take(&updated).Error
	}); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *store) DeleteMapTileSource(id uint64) error {
	if id == 0 {
		return fmt.Errorf("map source id is required")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var row mapTileSourceRecord
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			return err
		}
		if row.IsDefault {
			return errMapTileSourceCannotDeleteDefault
		}
		result := tx.Where("id = ?", id).Delete(&mapTileSourceRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (s *store) SetDefaultMapTileSource(id uint64) (*mapTileSourceRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("map source id is required")
	}
	var row mapTileSourceRecord
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Take(&row).Error; err != nil {
			return err
		}
		if !row.Enabled {
			return errMapTileSourceDefaultMustBeEnabled
		}
		if err := tx.Model(&mapTileSourceRecord{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&mapTileSourceRecord{}).Where("id = ?", id).Updates(map[string]any{"is_default": true, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Take(&row).Error
	}); err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *store) EnsureDefaultMapTileSource() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&mapTileSourceRecord{}).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			row := defaultMapTileSourceRecord()
			return tx.Create(&row).Error
		}

		var defaults []mapTileSourceRecord
		if err := tx.Where("enabled = ? AND is_default = ?", true, true).Order("id ASC").Find(&defaults).Error; err != nil {
			return err
		}
		if len(defaults) > 0 {
			return tx.Model(&mapTileSourceRecord{}).Where("id <> ? AND is_default = ?", defaults[0].ID, true).Update("is_default", false).Error
		}

		var enabled mapTileSourceRecord
		err := tx.Where("enabled = ?", true).Order("id ASC").Take(&enabled).Error
		if err == nil {
			return tx.Model(&mapTileSourceRecord{}).Where("id = ?", enabled.ID).Updates(map[string]any{"is_default": true, "updated_at": time.Now()}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		row := defaultMapTileSourceRecord()
		var existing mapTileSourceRecord
		err = tx.Where("name = ? OR url_template = ?", row.Name, row.URLTemplate).Order("id ASC").Take(&existing).Error
		if err == nil {
			return tx.Model(&mapTileSourceRecord{}).Where("id = ?", existing.ID).Updates(map[string]any{"enabled": true, "is_default": true, "updated_at": time.Now()}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(&row).Error
	})
}

func mapTileSourceHash(urlTemplate string) string {
	h := sha256.Sum256([]byte(urlTemplate))
	return hex.EncodeToString(h[:])
}

func defaultMapTileSourceRecord() mapTileSourceRecord {
	return mapTileSourceRecord{
		Name:            defaultMapTileSourceName,
		URLTemplate:     defaultMapTileSourceURLTemplate,
		URLTemplateHash: mapTileSourceHash(defaultMapTileSourceURLTemplate),
		Attribution:     defaultMapTileSourceAttribution,
		MaxZoom:         defaultMapTileSourceMaxZoom,
		Enabled:         true,
		IsDefault:       true,
	}
}

func mapTileSourceFromInput(input mapTileSourceInput) (*mapTileSourceRecord, error) {
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
	return &mapTileSourceRecord{
		Name:            name,
		URLTemplate:     urlTemplate,
		URLTemplateHash: mapTileSourceHash(urlTemplate),
		Attribution:     strings.TrimSpace(input.Attribution),
		MaxZoom:         maxZoom,
		Enabled:         input.Enabled,
		IsDefault:       input.IsDefault,
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

func (s *store) ensureMapTileSourceUnique(id uint64, name, urlTemplate string) error {
	return ensureMapTileSourceUniqueTx(s.db, id, name, urlTemplate)
}

func ensureMapTileSourceUniqueTx(tx *gorm.DB, id uint64, name, urlTemplate string) error {
	var existing mapTileSourceRecord
	q := tx.Where("name = ? OR url_template = ?", name, urlTemplate)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Take(&existing).Error
	if err == nil {
		return errMapTileSourceAlreadyExists
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}
