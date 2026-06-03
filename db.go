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

type migrationQuery struct {
	name  string
	query string
}

type mqttClientInfo struct {
	ClientID   string
	Username   string
	Listener   string
	RemoteAddr string
	RemoteHost string
	RemotePort string
}

type nodeInfoMapRecord struct {
	NodeID                 string
	NodeNum                int64
	LatestType             string
	UserID                 any
	LongName               any
	ShortName              any
	HWModel                any
	Role                   any
	IsLicensed             any
	PublicKey              any
	FirmwareVersion        any
	Region                 any
	ModemPreset            any
	Latitude               any
	Longitude              any
	Altitude               any
	PositionPrecision      any
	NumOnlineLocalNodes    any
	HasOptedReportLocation any
	ContentJSON            []byte
}

type textMessageRecord struct {
	FromID         string
	FromNum        int64
	Text           any
	PayloadHex     any
	Topic          string
	ChannelID      any
	GatewayID      any
	PacketID       any
	PacketTo       any
	PacketToNum    any
	Portnum        any
	PayloadLen     any
	PayloadVariant any
	ViaMQTT        any
	PKIEncrypted   any
	DecryptSuccess any
	DecryptStatus  any
	MQTTClientID   any
	MQTTUsername   any
	MQTTListener   any
	MQTTRemoteAddr any
	MQTTRemoteHost any
	MQTTRemotePort any
	ContentJSON    []byte
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
	queries, err := s.migrationQueries()
	if err != nil {
		return err
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q.query); err != nil {
			return fmt.Errorf("migrate %s: %w", q.name, err)
		}
	}
	return nil
}

func (s *store) migrationQueries() ([]migrationQuery, error) {
	switch s.driver {
	case databaseDriverSQLite:
		return []migrationQuery{
			{name: "nodeinfo_map table", query: `CREATE TABLE IF NOT EXISTS nodeinfo_map (
    node_id TEXT PRIMARY KEY,
    node_num INTEGER NOT NULL,
    latest_type TEXT NOT NULL,
    user_id TEXT,
    long_name TEXT,
    short_name TEXT,
    hw_model TEXT,
    role TEXT,
    is_licensed BOOLEAN,
    public_key TEXT,
    firmware_version TEXT,
    region TEXT,
    modem_preset TEXT,
    latitude REAL,
    longitude REAL,
    altitude INTEGER,
    position_precision INTEGER,
    num_online_local_nodes INTEGER,
    has_opted_report_location BOOLEAN,
    content_json TEXT NOT NULL,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);`},
			{name: "text_message table", query: `CREATE TABLE IF NOT EXISTS text_message (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_id TEXT NOT NULL,
    from_num INTEGER NOT NULL,
    text TEXT,
    payload_hex TEXT,
    topic TEXT NOT NULL,
    channel_id TEXT,
    gateway_id TEXT,
    packet_id INTEGER,
    packet_to TEXT,
    packet_to_num INTEGER,
    portnum TEXT,
    payload_len INTEGER,
    payload_variant TEXT,
    via_mqtt BOOLEAN,
    pki_encrypted BOOLEAN,
    decrypt_success BOOLEAN,
    decrypt_status TEXT,
    mqtt_client_id TEXT,
    mqtt_username TEXT,
    mqtt_listener TEXT,
    mqtt_remote_addr TEXT,
    mqtt_remote_host TEXT,
    mqtt_remote_port TEXT,
    content_json TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);`},
			{name: "text_message from_num index", query: `CREATE INDEX IF NOT EXISTS idx_text_message_from_num_created_at ON text_message (from_num, created_at);`},
			{name: "text_message created_at index", query: `CREATE INDEX IF NOT EXISTS idx_text_message_created_at ON text_message (created_at);`},
			{name: "text_message packet_id index", query: `CREATE INDEX IF NOT EXISTS idx_text_message_packet_id ON text_message (packet_id);`},
		}, nil
	case databaseDriverMySQL:
		return []migrationQuery{
			{name: "nodeinfo_map table", query: `CREATE TABLE IF NOT EXISTS nodeinfo_map (
    node_id VARCHAR(32) NOT NULL PRIMARY KEY,
    node_num BIGINT UNSIGNED NOT NULL,
    latest_type VARCHAR(32) NOT NULL,
    user_id VARCHAR(128),
    long_name TEXT,
    short_name VARCHAR(64),
    hw_model VARCHAR(128),
    role VARCHAR(128),
    is_licensed BOOLEAN,
    public_key TEXT,
    firmware_version VARCHAR(128),
    region VARCHAR(128),
    modem_preset VARCHAR(128),
    latitude DOUBLE,
    longitude DOUBLE,
    altitude INT,
    position_precision INT UNSIGNED,
    num_online_local_nodes INT UNSIGNED,
    has_opted_report_location BOOLEAN,
    content_json JSON NOT NULL,
    first_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);`},
			{name: "text_message table", query: `CREATE TABLE IF NOT EXISTS text_message (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    from_id VARCHAR(32) NOT NULL,
    from_num BIGINT UNSIGNED NOT NULL,
    text TEXT,
    payload_hex TEXT,
    topic TEXT NOT NULL,
    channel_id VARCHAR(128),
    gateway_id VARCHAR(128),
    packet_id BIGINT UNSIGNED,
    packet_to VARCHAR(32),
    packet_to_num BIGINT UNSIGNED,
    portnum VARCHAR(64),
    payload_len INT UNSIGNED,
    payload_variant VARCHAR(32),
    via_mqtt BOOLEAN,
    pki_encrypted BOOLEAN,
    decrypt_success BOOLEAN,
    decrypt_status VARCHAR(255),
    mqtt_client_id VARCHAR(255),
    mqtt_username VARCHAR(255),
    mqtt_listener VARCHAR(128),
    mqtt_remote_addr VARCHAR(255),
    mqtt_remote_host VARCHAR(255),
    mqtt_remote_port VARCHAR(16),
    content_json JSON NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_text_message_from_num_created_at (from_num, created_at),
    INDEX idx_text_message_created_at (created_at),
    INDEX idx_text_message_packet_id (packet_id)
);`},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported database driver %q", s.driver)
	}
}

func (s *store) UpsertNodeInfoMap(record map[string]any) error {
	node, err := nodeInfoMapFromRecord(record)
	if err != nil {
		return err
	}

	var query string
	switch s.driver {
	case databaseDriverSQLite:
		query = `INSERT INTO nodeinfo_map (
    node_id, node_num, latest_type, user_id, long_name, short_name,
    hw_model, role, is_licensed, public_key, firmware_version,
    region, modem_preset, latitude, longitude, altitude,
    position_precision, num_online_local_nodes, has_opted_report_location,
    content_json, first_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(node_id) DO UPDATE SET
    node_num = excluded.node_num,
    latest_type = excluded.latest_type,
    user_id = COALESCE(excluded.user_id, nodeinfo_map.user_id),
    long_name = COALESCE(excluded.long_name, nodeinfo_map.long_name),
    short_name = COALESCE(excluded.short_name, nodeinfo_map.short_name),
    hw_model = COALESCE(excluded.hw_model, nodeinfo_map.hw_model),
    role = COALESCE(excluded.role, nodeinfo_map.role),
    is_licensed = COALESCE(excluded.is_licensed, nodeinfo_map.is_licensed),
    public_key = COALESCE(excluded.public_key, nodeinfo_map.public_key),
    firmware_version = COALESCE(excluded.firmware_version, nodeinfo_map.firmware_version),
    region = COALESCE(excluded.region, nodeinfo_map.region),
    modem_preset = COALESCE(excluded.modem_preset, nodeinfo_map.modem_preset),
    latitude = COALESCE(excluded.latitude, nodeinfo_map.latitude),
    longitude = COALESCE(excluded.longitude, nodeinfo_map.longitude),
    altitude = COALESCE(excluded.altitude, nodeinfo_map.altitude),
    position_precision = COALESCE(excluded.position_precision, nodeinfo_map.position_precision),
    num_online_local_nodes = COALESCE(excluded.num_online_local_nodes, nodeinfo_map.num_online_local_nodes),
    has_opted_report_location = COALESCE(excluded.has_opted_report_location, nodeinfo_map.has_opted_report_location),
    content_json = excluded.content_json,
    updated_at = CURRENT_TIMESTAMP;`
	case databaseDriverMySQL:
		query = `INSERT INTO nodeinfo_map (
    node_id, node_num, latest_type, user_id, long_name, short_name,
    hw_model, role, is_licensed, public_key, firmware_version,
    region, modem_preset, latitude, longitude, altitude,
    position_precision, num_online_local_nodes, has_opted_report_location,
    content_json, first_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON DUPLICATE KEY UPDATE
    node_num = VALUES(node_num),
    latest_type = VALUES(latest_type),
    user_id = COALESCE(VALUES(user_id), user_id),
    long_name = COALESCE(VALUES(long_name), long_name),
    short_name = COALESCE(VALUES(short_name), short_name),
    hw_model = COALESCE(VALUES(hw_model), hw_model),
    role = COALESCE(VALUES(role), role),
    is_licensed = COALESCE(VALUES(is_licensed), is_licensed),
    public_key = COALESCE(VALUES(public_key), public_key),
    firmware_version = COALESCE(VALUES(firmware_version), firmware_version),
    region = COALESCE(VALUES(region), region),
    modem_preset = COALESCE(VALUES(modem_preset), modem_preset),
    latitude = COALESCE(VALUES(latitude), latitude),
    longitude = COALESCE(VALUES(longitude), longitude),
    altitude = COALESCE(VALUES(altitude), altitude),
    position_precision = COALESCE(VALUES(position_precision), position_precision),
    num_online_local_nodes = COALESCE(VALUES(num_online_local_nodes), num_online_local_nodes),
    has_opted_report_location = COALESCE(VALUES(has_opted_report_location), has_opted_report_location),
    content_json = VALUES(content_json),
    updated_at = CURRENT_TIMESTAMP;`
	default:
		return fmt.Errorf("unsupported database driver %q", s.driver)
	}

	_, err = s.db.Exec(query,
		node.NodeID,
		node.NodeNum,
		node.LatestType,
		node.UserID,
		node.LongName,
		node.ShortName,
		node.HWModel,
		node.Role,
		node.IsLicensed,
		node.PublicKey,
		node.FirmwareVersion,
		node.Region,
		node.ModemPreset,
		node.Latitude,
		node.Longitude,
		node.Altitude,
		node.PositionPrecision,
		node.NumOnlineLocalNodes,
		node.HasOptedReportLocation,
		string(node.ContentJSON),
	)
	if err != nil {
		return fmt.Errorf("upsert nodeinfo_map %s: %w", node.NodeID, err)
	}
	return nil
}

func (s *store) InsertTextMessage(record map[string]any, clientInfo mqttClientInfo) error {
	message, err := textMessageFromRecord(record, clientInfo)
	if err != nil {
		return err
	}

	query := `INSERT INTO text_message (
    from_id, from_num, text, payload_hex, topic, channel_id, gateway_id,
    packet_id, packet_to, packet_to_num, portnum, payload_len,
    payload_variant, via_mqtt, pki_encrypted, decrypt_success, decrypt_status,
    mqtt_client_id, mqtt_username, mqtt_listener, mqtt_remote_addr,
    mqtt_remote_host, mqtt_remote_port, content_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	_, err = s.db.Exec(query,
		message.FromID,
		message.FromNum,
		message.Text,
		message.PayloadHex,
		message.Topic,
		message.ChannelID,
		message.GatewayID,
		message.PacketID,
		message.PacketTo,
		message.PacketToNum,
		message.Portnum,
		message.PayloadLen,
		message.PayloadVariant,
		message.ViaMQTT,
		message.PKIEncrypted,
		message.DecryptSuccess,
		message.DecryptStatus,
		message.MQTTClientID,
		message.MQTTUsername,
		message.MQTTListener,
		message.MQTTRemoteAddr,
		message.MQTTRemoteHost,
		message.MQTTRemotePort,
		string(message.ContentJSON),
	)
	if err != nil {
		return fmt.Errorf("insert text_message from %s: %w", message.FromID, err)
	}
	return nil
}

func nodeInfoMapFromRecord(record map[string]any) (*nodeInfoMapRecord, error) {
	latestType, ok := record["type"].(string)
	if !ok || (latestType != "nodeinfo" && latestType != "map_report") {
		return nil, fmt.Errorf("record type %v is not nodeinfo or map_report", record["type"])
	}
	nodeID, ok := record["from"].(string)
	if !ok || nodeID == "" {
		return nil, fmt.Errorf("nodeinfo_map missing from")
	}
	nodeNum, err := int64FromAny(record["from_num"])
	if err != nil {
		return nil, fmt.Errorf("nodeinfo_map from_num: %w", err)
	}
	contentJSON, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode nodeinfo_map content_json: %w", err)
	}

	return &nodeInfoMapRecord{
		NodeID:                 nodeID,
		NodeNum:                nodeNum,
		LatestType:             latestType,
		UserID:                 nullableString(record["user_id"]),
		LongName:               nullableString(record["long_name"]),
		ShortName:              nullableString(record["short_name"]),
		HWModel:                nullableString(record["hw_model"]),
		Role:                   nullableString(record["role"]),
		IsLicensed:             nullableBool(record["is_licensed"]),
		PublicKey:              nullableString(record["public_key"]),
		FirmwareVersion:        nullableString(record["firmware_version"]),
		Region:                 nullableString(record["region"]),
		ModemPreset:            nullableString(record["modem_preset"]),
		Latitude:               nullableFloat64(record["latitude"]),
		Longitude:              nullableFloat64(record["longitude"]),
		Altitude:               nullableInt64(record["altitude"]),
		PositionPrecision:      nullableInt64(record["position_precision"]),
		NumOnlineLocalNodes:    nullableInt64(record["num_online_local_nodes"]),
		HasOptedReportLocation: nullableBool(record["has_opted_report_location"]),
		ContentJSON:            contentJSON,
	}, nil
}

func textMessageFromRecord(record map[string]any, clientInfo mqttClientInfo) (*textMessageRecord, error) {
	recordType, ok := record["type"].(string)
	if !ok || recordType != "text_message" {
		return nil, fmt.Errorf("record type %v is not text_message", record["type"])
	}
	fromID, ok := record["from"].(string)
	if !ok || fromID == "" {
		return nil, fmt.Errorf("text_message missing from")
	}
	fromNum, err := int64FromAny(record["from_num"])
	if err != nil {
		return nil, fmt.Errorf("text_message from_num: %w", err)
	}
	topic, ok := record["topic"].(string)
	if !ok || topic == "" {
		return nil, fmt.Errorf("text_message missing topic")
	}
	contentJSON, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode text_message content_json: %w", err)
	}

	return &textMessageRecord{
		FromID:         fromID,
		FromNum:        fromNum,
		Text:           nullableString(record["text"]),
		PayloadHex:     nullableString(record["payload_hex"]),
		Topic:          topic,
		ChannelID:      nullableString(record["channel_id"]),
		GatewayID:      nullableString(record["gateway_id"]),
		PacketID:       nullableInt64(record["packet_id"]),
		PacketTo:       nullableString(record["packet_to"]),
		PacketToNum:    nullableInt64(record["packet_to_num"]),
		Portnum:        nullableString(record["portnum"]),
		PayloadLen:     nullableInt64(record["payload_len"]),
		PayloadVariant: nullableString(record["payload_variant"]),
		ViaMQTT:        nullableBool(record["via_mqtt"]),
		PKIEncrypted:   nullableBool(record["pki_encrypted"]),
		DecryptSuccess: nullableBool(record["decrypt_success"]),
		DecryptStatus:  nullableString(record["decrypt_status"]),
		MQTTClientID:   nullableString(clientInfo.ClientID),
		MQTTUsername:   nullableString(clientInfo.Username),
		MQTTListener:   nullableString(clientInfo.Listener),
		MQTTRemoteAddr: nullableString(clientInfo.RemoteAddr),
		MQTTRemoteHost: nullableString(clientInfo.RemoteHost),
		MQTTRemotePort: nullableString(clientInfo.RemotePort),
		ContentJSON:    contentJSON,
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

func nullableBool(value any) any {
	b, ok := value.(bool)
	if !ok {
		return nil
	}
	return b
}

func nullableInt64(value any) any {
	if value == nil {
		return nil
	}
	v, err := int64FromAny(value)
	if err != nil {
		return nil
	}
	return v
}

func nullableFloat64(value any) any {
	switch v := value.(type) {
	case float32:
		return float64(v)
	case float64:
		return v
	case int:
		return float64(v)
	case int8:
		return float64(v)
	case int16:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case uint:
		return float64(v)
	case uint8:
		return float64(v)
	case uint16:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	default:
		return nil
	}
}
