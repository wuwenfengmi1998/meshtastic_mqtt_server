package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

const (
	databaseDriverSQLite = "sqlite"
	databaseDriverMySQL  = "mysql"
)

type store struct {
	db     *sql.DB
	driver string
}

type nodeInfoRecord struct {
	NodeID      string
	NodeNum     int64
	UserID      any
	LongName    any
	ShortName   any
	HWModel     any
	Role        any
	IsLicensed  bool
	PublicKey   any
	ContentJSON []byte
}

func openStore(cfg databaseConfig) (*store, error) {
	var dsn string
	switch cfg.Driver {
	case databaseDriverSQLite:
		if err := os.MkdirAll(filepath.Dir(cfg.SQLite.Path), 0755); err != nil {
			return nil, fmt.Errorf("create sqlite directory %s: %w", filepath.Dir(cfg.SQLite.Path), err)
		}
		dsn = cfg.SQLite.Path
	case databaseDriverMySQL:
		dsn = cfg.MySQL.DSN
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}

	db, err := sql.Open(cfg.Driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", cfg.Driver, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s database: %w", cfg.Driver, err)
	}

	s := &store{db: db, driver: cfg.Driver}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *store) migrate() error {
	var query string
	switch s.driver {
	case databaseDriverSQLite:
		query = `CREATE TABLE IF NOT EXISTS nodeinfo (
    node_id TEXT PRIMARY KEY,
    node_num INTEGER NOT NULL,
    user_id TEXT,
    long_name TEXT,
    short_name TEXT,
    hw_model TEXT,
    role TEXT,
    is_licensed BOOLEAN NOT NULL DEFAULT FALSE,
    public_key TEXT,
    content_json TEXT NOT NULL,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);`
	case databaseDriverMySQL:
		query = `CREATE TABLE IF NOT EXISTS nodeinfo (
    node_id VARCHAR(32) NOT NULL PRIMARY KEY,
    node_num BIGINT UNSIGNED NOT NULL,
    user_id VARCHAR(128),
    long_name TEXT,
    short_name VARCHAR(64),
    hw_model VARCHAR(128),
    role VARCHAR(128),
    is_licensed BOOLEAN NOT NULL DEFAULT FALSE,
    public_key TEXT,
    content_json JSON NOT NULL,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);`
	default:
		return fmt.Errorf("unsupported database driver %q", s.driver)
	}

	if _, err := s.db.Exec(query); err != nil {
		return fmt.Errorf("migrate nodeinfo table: %w", err)
	}
	return nil
}

func (s *store) UpsertNodeInfo(record map[string]any) error {
	node, err := nodeInfoFromRecord(record)
	if err != nil {
		return err
	}

	var query string
	switch s.driver {
	case databaseDriverSQLite:
		query = `INSERT INTO nodeinfo (
    node_id, node_num, user_id, long_name, short_name,
    hw_model, role, is_licensed, public_key, content_json,
    first_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(node_id) DO UPDATE SET
    node_num = excluded.node_num,
    user_id = excluded.user_id,
    long_name = excluded.long_name,
    short_name = excluded.short_name,
    hw_model = excluded.hw_model,
    role = excluded.role,
    is_licensed = excluded.is_licensed,
    public_key = excluded.public_key,
    content_json = excluded.content_json,
    updated_at = CURRENT_TIMESTAMP;`
	case databaseDriverMySQL:
		query = `INSERT INTO nodeinfo (
    node_id, node_num, user_id, long_name, short_name,
    hw_model, role, is_licensed, public_key, content_json,
    first_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
    node_num = VALUES(node_num),
    user_id = VALUES(user_id),
    long_name = VALUES(long_name),
    short_name = VALUES(short_name),
    hw_model = VALUES(hw_model),
    role = VALUES(role),
    is_licensed = VALUES(is_licensed),
    public_key = VALUES(public_key),
    content_json = VALUES(content_json),
    updated_at = CURRENT_TIMESTAMP;`
	default:
		return fmt.Errorf("unsupported database driver %q", s.driver)
	}

	_, err = s.db.Exec(query,
		node.NodeID,
		node.NodeNum,
		node.UserID,
		node.LongName,
		node.ShortName,
		node.HWModel,
		node.Role,
		node.IsLicensed,
		node.PublicKey,
		string(node.ContentJSON),
	)
	if err != nil {
		return fmt.Errorf("upsert nodeinfo %s: %w", node.NodeID, err)
	}
	return nil
}

func nodeInfoFromRecord(record map[string]any) (*nodeInfoRecord, error) {
	if record["type"] != "nodeinfo" {
		return nil, fmt.Errorf("record type %v is not nodeinfo", record["type"])
	}
	nodeID, ok := record["from"].(string)
	if !ok || nodeID == "" {
		return nil, fmt.Errorf("nodeinfo missing from")
	}
	nodeNum, err := int64FromAny(record["from_num"])
	if err != nil {
		return nil, fmt.Errorf("nodeinfo from_num: %w", err)
	}
	contentJSON, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode nodeinfo content_json: %w", err)
	}

	return &nodeInfoRecord{
		NodeID:      nodeID,
		NodeNum:     nodeNum,
		UserID:      nullableString(record["user_id"]),
		LongName:    nullableString(record["long_name"]),
		ShortName:   nullableString(record["short_name"]),
		HWModel:     nullableString(record["hw_model"]),
		Role:        nullableString(record["role"]),
		IsLicensed:  boolFromAny(record["is_licensed"]),
		PublicKey:   nullableString(record["public_key"]),
		ContentJSON: contentJSON,
	}, nil
}

func int64FromAny(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		return int64(v), nil
	case float64:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("unsupported value %T", value)
	}
}

func nullableString(value any) any {
	if value == nil {
		return nil
	}
	s, ok := value.(string)
	if !ok || s == "" {
		return nil
	}
	return s
}

func boolFromAny(value any) bool {
	b, _ := value.(bool)
	return b
}
