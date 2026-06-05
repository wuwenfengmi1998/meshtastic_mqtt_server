package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	databaseDriverSQLite = "sqlite"
	databaseDriverMySQL  = "mysql"
)

type store struct {
	db     *gorm.DB
	driver string
}

type mqttClientInfo struct {
	ClientID   string
	Username   string
	Listener   string
	RemoteAddr string
	RemoteHost string
	RemotePort string
}

type AppendPacketFields struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	FromID         string    `gorm:"column:from_id;not null;index"`
	FromNum        int64     `gorm:"column:from_num;not null;index"`
	Topic          string    `gorm:"column:topic;not null"`
	ChannelID      *string   `gorm:"column:channel_id"`
	GatewayID      *string   `gorm:"column:gateway_id"`
	PacketID       *int64    `gorm:"column:packet_id;index"`
	PacketTo       *string   `gorm:"column:packet_to"`
	PacketToNum    *int64    `gorm:"column:packet_to_num"`
	Portnum        *string   `gorm:"column:portnum"`
	PayloadLen     *int64    `gorm:"column:payload_len"`
	PayloadVariant *string   `gorm:"column:payload_variant"`
	ViaMQTT        *bool     `gorm:"column:via_mqtt"`
	PKIEncrypted   *bool     `gorm:"column:pki_encrypted"`
	DecryptSuccess *bool     `gorm:"column:decrypt_success"`
	DecryptStatus  *string   `gorm:"column:decrypt_status"`
	ContentJSON    string    `gorm:"column:content_json;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime;index"`
}

type MQTTClientRecordFields struct {
	MQTTClientID   *string `gorm:"column:mqtt_client_id"`
	MQTTUsername   *string `gorm:"column:mqtt_username"`
	MQTTListener   *string `gorm:"column:mqtt_listener"`
	MQTTRemoteAddr *string `gorm:"column:mqtt_remote_addr"`
	MQTTRemoteHost *string `gorm:"column:mqtt_remote_host"`
	MQTTRemotePort *string `gorm:"column:mqtt_remote_port"`
}

type userRecord struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string    `gorm:"column:username;not null;uniqueIndex"`
	PasswordHash string    `gorm:"column:password_hash;not null"`
	Role         string    `gorm:"column:role;not null;index"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (userRecord) TableName() string {
	return "users"
}

type loginLogRecord struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Username   string    `gorm:"column:username;index"`
	UserID     *uint64   `gorm:"column:user_id;index"`
	Success    bool      `gorm:"column:success;not null;index"`
	Reason     string    `gorm:"column:reason;not null"`
	RemoteAddr string    `gorm:"column:remote_addr"`
	RemoteHost string    `gorm:"column:remote_host"`
	UserAgent  string    `gorm:"column:user_agent"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime;index"`
}

func (loginLogRecord) TableName() string {
	return "login_log"
}

type helpContentRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Markdown  string    `gorm:"column:markdown;type:text;not null"`
	CreatedBy string    `gorm:"column:created_by;index"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index"`
}

func (helpContentRecord) TableName() string {
	return "help_content"
}

type runtimeSettingRecord struct {
	Key       string    `gorm:"column:key;primaryKey;size:128;not null"`
	Value     string    `gorm:"column:value;type:text;not null"`
	ValueType string    `gorm:"column:value_type;size:32;not null;index"`
	Label     string    `gorm:"column:label"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (runtimeSettingRecord) TableName() string {
	return "runtime_settings"
}

type discardDetailsRecord struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Topic          string    `gorm:"column:topic"`
	Error          string    `gorm:"column:error"`
	PayloadLen     int64     `gorm:"column:payload_len"`
	RawBase64      string    `gorm:"column:raw_base64;not null"`
	ContentJSON    string    `gorm:"column:content_json;not null"`
	MQTTClientID   *string   `gorm:"column:mqtt_client_id"`
	MQTTUsername   *string   `gorm:"column:mqtt_username"`
	MQTTListener   *string   `gorm:"column:mqtt_listener"`
	MQTTRemoteAddr *string   `gorm:"column:mqtt_remote_addr"`
	MQTTRemoteHost *string   `gorm:"column:mqtt_remote_host"`
	MQTTRemotePort *string   `gorm:"column:mqtt_remote_port"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime;index"`
}

func (discardDetailsRecord) TableName() string {
	return "discard_details"
}

type nodeBlockingRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	NodeID    string    `gorm:"column:node_id;not null;uniqueIndex"`
	NodeNum   *int64    `gorm:"column:node_num;index"`
	Reason    string    `gorm:"column:reason"`
	Enabled   bool      `gorm:"column:enabled;not null;index"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (nodeBlockingRecord) TableName() string {
	return "node_blocking"
}

type ipBlockingRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	IPValue   string    `gorm:"column:ip_value;not null;uniqueIndex"`
	Reason    string    `gorm:"column:reason"`
	Enabled   bool      `gorm:"column:enabled;not null;index"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (ipBlockingRecord) TableName() string {
	return "ip_blocking"
}

type forbiddenWordBlockingRecord struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Word          string    `gorm:"column:word;not null;uniqueIndex"`
	MatchType     string    `gorm:"column:match_type;not null;index"`
	CaseSensitive bool      `gorm:"column:case_sensitive;not null"`
	Reason        string    `gorm:"column:reason"`
	Enabled       bool      `gorm:"column:enabled;not null;index"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (forbiddenWordBlockingRecord) TableName() string {
	return "forbidden_word_blocking"
}

type mqttForwarderRecord struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Name           string    `gorm:"column:name;not null;uniqueIndex"`
	Enabled        bool      `gorm:"column:enabled;not null;index"`
	SourceHost     string    `gorm:"column:source_host;not null"`
	SourcePort     int       `gorm:"column:source_port;not null"`
	SourceUsername string    `gorm:"column:source_username"`
	SourcePassword string    `gorm:"column:source_password"`
	SourceClientID string    `gorm:"column:source_client_id"`
	SourceTLS      bool      `gorm:"column:source_tls;not null"`
	TargetHost     string    `gorm:"column:target_host;not null"`
	TargetPort     int       `gorm:"column:target_port;not null"`
	TargetUsername string    `gorm:"column:target_username"`
	TargetPassword string    `gorm:"column:target_password"`
	TargetClientID string    `gorm:"column:target_client_id"`
	TargetTLS      bool      `gorm:"column:target_tls;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (mqttForwarderRecord) TableName() string {
	return "mqtt_forwarders"
}

type mqttForwardTopicRecord struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	ForwarderID  uint64    `gorm:"column:forwarder_id;not null;index;uniqueIndex:idx_mqtt_forward_topic_unique,priority:1"`
	Topic        string    `gorm:"column:topic;not null;uniqueIndex:idx_mqtt_forward_topic_unique,priority:2"`
	Enabled      bool      `gorm:"column:enabled;not null;index"`
	Direction    string    `gorm:"column:direction;not null;index"`
	SourcePrefix string    `gorm:"column:source_prefix"`
	TargetPrefix string    `gorm:"column:target_prefix"`
	QoS          int       `gorm:"column:qos;not null"`
	Retain       bool      `gorm:"column:retain;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (mqttForwardTopicRecord) TableName() string {
	return "mqtt_forward_topics"
}

type nodeInfoRecord struct {
	NodeID      string    `gorm:"column:node_id;primaryKey;not null"`
	NodeNum     int64     `gorm:"column:node_num;not null;index"`
	UserID      *string   `gorm:"column:user_id"`
	LongName    *string   `gorm:"column:long_name"`
	ShortName   *string   `gorm:"column:short_name"`
	HWModel     *string   `gorm:"column:hw_model"`
	Role        *string   `gorm:"column:role"`
	IsLicensed  *bool     `gorm:"column:is_licensed"`
	PublicKey   *string   `gorm:"column:public_key"`
	ContentJSON string    `gorm:"column:content_json;not null"`
	FirstSeenAt time.Time `gorm:"column:first_seen_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (nodeInfoRecord) TableName() string {
	return "nodeinfo"
}

type mapReportRecord struct {
	NodeID                 string    `gorm:"column:node_id;primaryKey;not null"`
	NodeNum                int64     `gorm:"column:node_num;not null;index"`
	LongName               *string   `gorm:"column:long_name"`
	ShortName              *string   `gorm:"column:short_name"`
	HWModel                *string   `gorm:"column:hw_model"`
	Role                   *string   `gorm:"column:role"`
	FirmwareVersion        *string   `gorm:"column:firmware_version"`
	Region                 *string   `gorm:"column:region"`
	ModemPreset            *string   `gorm:"column:modem_preset"`
	Latitude               *float64  `gorm:"column:latitude;index"`
	Longitude              *float64  `gorm:"column:longitude;index"`
	Altitude               *int64    `gorm:"column:altitude"`
	PositionPrecision      *int64    `gorm:"column:position_precision"`
	NumOnlineLocalNodes    *int64    `gorm:"column:num_online_local_nodes"`
	HasOptedReportLocation *bool     `gorm:"column:has_opted_report_location"`
	ContentJSON            string    `gorm:"column:content_json;not null"`
	FirstSeenAt            time.Time `gorm:"column:first_seen_at;autoCreateTime"`
	UpdatedAt              time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (mapReportRecord) TableName() string {
	return "map_report"
}

type textMessageRecord struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	FromID         string    `gorm:"column:from_id;not null"`
	FromNum        int64     `gorm:"column:from_num;not null;index:idx_text_message_from_num_created_at,priority:1"`
	Text           *string   `gorm:"column:text"`
	PayloadHex     *string   `gorm:"column:payload_hex"`
	Topic          string    `gorm:"column:topic;not null"`
	ChannelID      *string   `gorm:"column:channel_id"`
	GatewayID      *string   `gorm:"column:gateway_id"`
	PacketID       *int64    `gorm:"column:packet_id;index:idx_text_message_packet_id"`
	PacketTo       *string   `gorm:"column:packet_to"`
	PacketToNum    *int64    `gorm:"column:packet_to_num"`
	Portnum        *string   `gorm:"column:portnum"`
	PayloadLen     *int64    `gorm:"column:payload_len"`
	PayloadVariant *string   `gorm:"column:payload_variant"`
	ViaMQTT        *bool     `gorm:"column:via_mqtt"`
	PKIEncrypted   *bool     `gorm:"column:pki_encrypted"`
	DecryptSuccess *bool     `gorm:"column:decrypt_success"`
	DecryptStatus  *string   `gorm:"column:decrypt_status"`
	MQTTClientID   *string   `gorm:"column:mqtt_client_id"`
	MQTTUsername   *string   `gorm:"column:mqtt_username"`
	MQTTListener   *string   `gorm:"column:mqtt_listener"`
	MQTTRemoteAddr *string   `gorm:"column:mqtt_remote_addr"`
	MQTTRemoteHost *string   `gorm:"column:mqtt_remote_host"`
	MQTTRemotePort *string   `gorm:"column:mqtt_remote_port"`
	ContentJSON    string    `gorm:"column:content_json;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime;index:idx_text_message_from_num_created_at,priority:2;index:idx_text_message_created_at"`
}

func (textMessageRecord) TableName() string {
	return "text_message"
}

type positionRecord struct {
	AppendPacketFields        `gorm:"embedded"`
	MQTTClientRecordFields    `gorm:"embedded"`
	Latitude                  *float64 `gorm:"column:latitude"`
	Longitude                 *float64 `gorm:"column:longitude"`
	Altitude                  *int64   `gorm:"column:altitude"`
	PositionTime              *int64   `gorm:"column:position_time"`
	LocationSource            *string  `gorm:"column:location_source"`
	AltitudeSource            *string  `gorm:"column:altitude_source"`
	Timestamp                 *int64   `gorm:"column:timestamp"`
	TimestampMillisAdjust     *int64   `gorm:"column:timestamp_millis_adjust"`
	AltitudeHAE               *int64   `gorm:"column:altitude_hae"`
	AltitudeGeoidalSeparation *int64   `gorm:"column:altitude_geoidal_separation"`
	PDOP                      *float64 `gorm:"column:pdop"`
	HDOP                      *float64 `gorm:"column:hdop"`
	VDOP                      *float64 `gorm:"column:vdop"`
	GPSAccuracy               *int64   `gorm:"column:gps_accuracy"`
	GroundSpeed               *int64   `gorm:"column:ground_speed"`
	GroundTrack               *float64 `gorm:"column:ground_track"`
	FixQuality                *int64   `gorm:"column:fix_quality"`
	FixType                   *int64   `gorm:"column:fix_type"`
	SatsInView                *int64   `gorm:"column:sats_in_view"`
	SensorID                  *int64   `gorm:"column:sensor_id"`
	NextUpdate                *int64   `gorm:"column:next_update"`
	SeqNumber                 *int64   `gorm:"column:seq_number"`
	PrecisionBits             *int64   `gorm:"column:precision_bits"`
}

func (positionRecord) TableName() string {
	return "position"
}

type telemetryRecord struct {
	AppendPacketFields     `gorm:"embedded"`
	MQTTClientRecordFields `gorm:"embedded"`
	TelemetryTime          *int64  `gorm:"column:telemetry_time"`
	TelemetryType          *string `gorm:"column:telemetry_type;index"`
	MetricsJSON            *string `gorm:"column:metrics_json"`
}

func (telemetryRecord) TableName() string {
	return "telemetry"
}

type routingRecord struct {
	AppendPacketFields     `gorm:"embedded"`
	MQTTClientRecordFields `gorm:"embedded"`
}

func (routingRecord) TableName() string {
	return "routing"
}

type tracerouteRecord struct {
	AppendPacketFields     `gorm:"embedded"`
	MQTTClientRecordFields `gorm:"embedded"`
}

func (tracerouteRecord) TableName() string {
	return "traceroute"
}

func openStore(cfg databaseConfig) (*store, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case databaseDriverSQLite:
		if err := os.MkdirAll(filepath.Dir(cfg.SQLite.Path), 0755); err != nil {
			return nil, fmt.Errorf("create sqlite directory %s: %w", filepath.Dir(cfg.SQLite.Path), err)
		}
		dialector = sqlite.Open(cfg.SQLite.Path)
	case databaseDriverMySQL:
		dialector = mysql.Open(cfg.MySQL.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", cfg.Driver, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get %s database handle: %w", cfg.Driver, err)
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping %s database: %w", cfg.Driver, err)
	}

	s := &store{db: db, driver: cfg.Driver}
	if err := s.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return s, nil
}

func (s *store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *store) migrate() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		migrator := tx.Migrator()
		for _, item := range []struct {
			label string
			model any
		}{
			{label: "users", model: &userRecord{}},
			{label: "login_log", model: &loginLogRecord{}},
			{label: "help_content", model: &helpContentRecord{}},
			{label: "runtime_settings", model: &runtimeSettingRecord{}},
			{label: "discard_details", model: &discardDetailsRecord{}},
			{label: "node_blocking", model: &nodeBlockingRecord{}},
			{label: "ip_blocking", model: &ipBlockingRecord{}},
			{label: "forbidden_word_blocking", model: &forbiddenWordBlockingRecord{}},
			{label: "mqtt_forwarders", model: &mqttForwarderRecord{}},
			{label: "mqtt_forward_topics", model: &mqttForwardTopicRecord{}},
			{label: "nodeinfo", model: &nodeInfoRecord{}},
			{label: "map_report", model: &mapReportRecord{}},
			{label: "text_message", model: &textMessageRecord{}},
			{label: "position", model: &positionRecord{}},
			{label: "telemetry", model: &telemetryRecord{}},
			{label: "routing", model: &routingRecord{}},
			{label: "traceroute", model: &tracerouteRecord{}},
		} {
			if !migrator.HasTable(item.model) {
				if err := migrator.CreateTable(item.model); err != nil {
					return fmt.Errorf("migrate %s table: %w", item.label, err)
				}
			}
		}
		for _, item := range []struct {
			label   string
			model   any
			indexes []string
		}{
			{label: "text_message", model: &textMessageRecord{}, indexes: []string{"idx_text_message_from_num_created_at", "idx_text_message_created_at", "idx_text_message_packet_id"}},
		} {
			if err := createMissingIndexes(migrator, item.model, item.label, item.indexes); err != nil {
				return err
			}
		}
		return nil
	})
}

func createMissingIndexes(migrator gorm.Migrator, model any, label string, indexNames []string) error {
	for _, indexName := range indexNames {
		if !migrator.HasIndex(model, indexName) {
			if err := migrator.CreateIndex(model, indexName); err != nil {
				return fmt.Errorf("migrate %s index %s: %w", label, indexName, err)
			}
		}
	}
	return nil
}

func (s *store) UpsertNodeInfo(record map[string]any) error {
	node, err := nodeInfoFromRecord(record)
	if err != nil {
		return err
	}
	if err := s.upsertNodeInfoRecord(node); err != nil {
		return fmt.Errorf("upsert nodeinfo %s: %w", node.NodeID, err)
	}
	if err := s.updateMapReportFromNodeInfo(node); err != nil {
		return fmt.Errorf("update map_report from nodeinfo %s: %w", node.NodeID, err)
	}
	return nil
}

func (s *store) UpsertMapReport(record map[string]any) error {
	report, err := mapReportFromRecord(record)
	if err != nil {
		return err
	}
	if err := s.upsertMapReportRecord(report); err != nil {
		return fmt.Errorf("upsert map_report %s: %w", report.NodeID, err)
	}
	if err := s.updateNodeInfoFromMapReport(report); err != nil {
		return fmt.Errorf("update nodeinfo from map_report %s: %w", report.NodeID, err)
	}
	return nil
}

func (s *store) upsertNodeInfoRecord(node *nodeInfoRecord) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing nodeInfoRecord
		err := tx.Where("node_id = ?", node.NodeID).Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(node).Error; err != nil {
				return s.updateNodeInfoRecord(tx, node)
			}
			return nil
		}
		if err != nil {
			return err
		}
		return s.updateNodeInfoRecord(tx, node)
	})
}

func (s *store) upsertMapReportRecord(report *mapReportRecord) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.upsertMapReportRecordTx(tx, report)
	})
}

func (s *store) upsertMapReportRecordTx(tx *gorm.DB, report *mapReportRecord) error {
	var existing mapReportRecord
	err := tx.Where("node_id = ?", report.NodeID).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.Create(report).Error; err != nil {
			return s.updateMapReportRecord(tx, report)
		}
		return nil
	}
	if err != nil {
		return err
	}
	return s.updateMapReportRecord(tx, report)
}

func (s *store) updateNodeInfoRecord(tx *gorm.DB, node *nodeInfoRecord) error {
	updates := nodeInfoUpdates(node)
	return tx.Model(&nodeInfoRecord{}).Where("node_id = ?", node.NodeID).Updates(updates).Error
}

func (s *store) updateMapReportRecord(tx *gorm.DB, report *mapReportRecord) error {
	updates := mapReportUpdates(report)
	return tx.Model(&mapReportRecord{}).Where("node_id = ?", report.NodeID).Updates(updates).Error
}

func (s *store) updateMapReportFromNodeInfo(node *nodeInfoRecord) error {
	updates := map[string]any{
		"node_num":   node.NodeNum,
		"updated_at": time.Now(),
	}
	addStringUpdate(updates, "long_name", node.LongName)
	addStringUpdate(updates, "short_name", node.ShortName)
	addStringUpdate(updates, "hw_model", node.HWModel)
	addStringUpdate(updates, "role", node.Role)
	return s.db.Model(&mapReportRecord{}).Where("node_id = ?", node.NodeID).Updates(updates).Error
}

func (s *store) updateNodeInfoFromMapReport(report *mapReportRecord) error {
	updates := map[string]any{
		"node_num":   report.NodeNum,
		"updated_at": time.Now(),
	}
	addStringUpdate(updates, "long_name", report.LongName)
	addStringUpdate(updates, "short_name", report.ShortName)
	addStringUpdate(updates, "hw_model", report.HWModel)
	addStringUpdate(updates, "role", report.Role)
	return s.db.Model(&nodeInfoRecord{}).Where("node_id = ?", report.NodeID).Updates(updates).Error
}

func nodeInfoUpdates(node *nodeInfoRecord) map[string]any {
	updates := map[string]any{
		"node_num":     node.NodeNum,
		"content_json": node.ContentJSON,
		"updated_at":   time.Now(),
	}
	addStringUpdate(updates, "user_id", node.UserID)
	addStringUpdate(updates, "long_name", node.LongName)
	addStringUpdate(updates, "short_name", node.ShortName)
	addStringUpdate(updates, "hw_model", node.HWModel)
	addStringUpdate(updates, "role", node.Role)
	addBoolUpdate(updates, "is_licensed", node.IsLicensed)
	addStringUpdate(updates, "public_key", node.PublicKey)
	return updates
}

func mapReportUpdates(report *mapReportRecord) map[string]any {
	updates := map[string]any{
		"node_num":     report.NodeNum,
		"content_json": report.ContentJSON,
		"updated_at":   time.Now(),
	}
	addStringUpdate(updates, "long_name", report.LongName)
	addStringUpdate(updates, "short_name", report.ShortName)
	addStringUpdate(updates, "hw_model", report.HWModel)
	addStringUpdate(updates, "role", report.Role)
	addStringUpdate(updates, "firmware_version", report.FirmwareVersion)
	addStringUpdate(updates, "region", report.Region)
	addStringUpdate(updates, "modem_preset", report.ModemPreset)
	addFloat64Update(updates, "latitude", report.Latitude)
	addFloat64Update(updates, "longitude", report.Longitude)
	addInt64Update(updates, "altitude", report.Altitude)
	addInt64Update(updates, "position_precision", report.PositionPrecision)
	addInt64Update(updates, "num_online_local_nodes", report.NumOnlineLocalNodes)
	addBoolUpdate(updates, "has_opted_report_location", report.HasOptedReportLocation)
	return updates
}

func (s *store) InsertTextMessage(record map[string]any, clientInfo mqttClientInfo) error {
	message, err := textMessageFromRecord(record, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(message).Error; err != nil {
		return fmt.Errorf("insert text_message from %s: %w", message.FromID, err)
	}
	return nil
}

func (s *store) InsertPosition(record map[string]any, clientInfo mqttClientInfo) error {
	position, err := positionFromRecord(record, clientInfo)
	if err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(position).Error; err != nil {
			return fmt.Errorf("insert position from %s: %w", position.FromID, err)
		}
		if err := s.upsertMapReportFromPosition(tx, position); err != nil {
			return fmt.Errorf("upsert map_report from position %s: %w", position.FromID, err)
		}
		return nil
	})
}

func (s *store) upsertMapReportFromPosition(tx *gorm.DB, position *positionRecord) error {
	report := &mapReportRecord{
		NodeID:            position.FromID,
		NodeNum:           position.FromNum,
		Latitude:          position.Latitude,
		Longitude:         position.Longitude,
		Altitude:          position.Altitude,
		PositionPrecision: position.PrecisionBits,
		ContentJSON:       position.ContentJSON,
	}

	var existing mapReportRecord
	err := tx.Where("node_id = ?", position.FromID).Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(report).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]any{"node_num": position.FromNum, "updated_at": time.Now()}
	addFloat64Update(updates, "latitude", position.Latitude)
	addFloat64Update(updates, "longitude", position.Longitude)
	addInt64Update(updates, "altitude", position.Altitude)
	addInt64Update(updates, "position_precision", position.PrecisionBits)
	return tx.Model(&mapReportRecord{}).Where("node_id = ?", position.FromID).Updates(updates).Error
}

func (s *store) InsertTelemetry(record map[string]any, clientInfo mqttClientInfo) error {
	telemetry, err := telemetryFromRecord(record, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(telemetry).Error; err != nil {
		return fmt.Errorf("insert telemetry from %s: %w", telemetry.FromID, err)
	}
	return nil
}

func (s *store) InsertRouting(record map[string]any, clientInfo mqttClientInfo) error {
	routing, err := routingFromRecord(record, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(routing).Error; err != nil {
		return fmt.Errorf("insert routing from %s: %w", routing.FromID, err)
	}
	return nil
}

func (s *store) InsertTraceroute(record map[string]any, clientInfo mqttClientInfo) error {
	traceroute, err := tracerouteFromRecord(record, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(traceroute).Error; err != nil {
		return fmt.Errorf("insert traceroute from %s: %w", traceroute.FromID, err)
	}
	return nil
}

func nodeInfoFromRecord(record map[string]any) (*nodeInfoRecord, error) {
	recordType, ok := record["type"].(string)
	if !ok || recordType != "nodeinfo" {
		return nil, fmt.Errorf("record type %v is not nodeinfo", record["type"])
	}
	nodeID, nodeNum, contentJSON, err := nodeRecordBase(record, "nodeinfo")
	if err != nil {
		return nil, err
	}

	return &nodeInfoRecord{
		NodeID:      nodeID,
		NodeNum:     nodeNum,
		UserID:      nullableString(record["user_id"]),
		LongName:    nullableString(record["long_name"]),
		ShortName:   nullableString(record["short_name"]),
		HWModel:     nullableString(record["hw_model"]),
		Role:        nullableString(record["role"]),
		IsLicensed:  nullableBool(record["is_licensed"]),
		PublicKey:   nullableString(record["public_key"]),
		ContentJSON: contentJSON,
	}, nil
}

func mapReportFromRecord(record map[string]any) (*mapReportRecord, error) {
	recordType, ok := record["type"].(string)
	if !ok || recordType != "map_report" {
		return nil, fmt.Errorf("record type %v is not map_report", record["type"])
	}
	nodeID, nodeNum, contentJSON, err := nodeRecordBase(record, "map_report")
	if err != nil {
		return nil, err
	}

	return &mapReportRecord{
		NodeID:                 nodeID,
		NodeNum:                nodeNum,
		LongName:               nullableString(record["long_name"]),
		ShortName:              nullableString(record["short_name"]),
		HWModel:                nullableString(record["hw_model"]),
		Role:                   nullableString(record["role"]),
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

func nodeRecordBase(record map[string]any, label string) (string, int64, string, error) {
	nodeID, ok := record["from"].(string)
	if !ok || nodeID == "" {
		return "", 0, "", fmt.Errorf("%s missing from", label)
	}
	nodeNum, err := int64FromAny(record["from_num"])
	if err != nil {
		return "", 0, "", fmt.Errorf("%s from_num: %w", label, err)
	}
	contentJSON, err := json.Marshal(record)
	if err != nil {
		return "", 0, "", fmt.Errorf("encode %s content_json: %w", label, err)
	}
	return nodeID, nodeNum, string(contentJSON), nil
}

func textMessageFromRecord(record map[string]any, clientInfo mqttClientInfo) (*textMessageRecord, error) {
	recordType, ok := record["type"].(string)
	if !ok || recordType != "text_message" {
		return nil, fmt.Errorf("record type %v is not text_message", record["type"])
	}
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "text_message", clientInfo)
	if err != nil {
		return nil, err
	}
	return &textMessageRecord{
		FromID:         common.FromID,
		FromNum:        common.FromNum,
		Text:           nullableString(record["text"]),
		PayloadHex:     nullableString(record["payload_hex"]),
		Topic:          common.Topic,
		ChannelID:      common.ChannelID,
		GatewayID:      common.GatewayID,
		PacketID:       common.PacketID,
		PacketTo:       common.PacketTo,
		PacketToNum:    common.PacketToNum,
		Portnum:        common.Portnum,
		PayloadLen:     common.PayloadLen,
		PayloadVariant: common.PayloadVariant,
		ViaMQTT:        common.ViaMQTT,
		PKIEncrypted:   common.PKIEncrypted,
		DecryptSuccess: common.DecryptSuccess,
		DecryptStatus:  common.DecryptStatus,
		MQTTClientID:   clientFields.MQTTClientID,
		MQTTUsername:   clientFields.MQTTUsername,
		MQTTListener:   clientFields.MQTTListener,
		MQTTRemoteAddr: clientFields.MQTTRemoteAddr,
		MQTTRemoteHost: clientFields.MQTTRemoteHost,
		MQTTRemotePort: clientFields.MQTTRemotePort,
		ContentJSON:    common.ContentJSON,
	}, nil
}

func positionFromRecord(record map[string]any, clientInfo mqttClientInfo) (*positionRecord, error) {
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "position", clientInfo)
	if err != nil {
		return nil, err
	}
	return &positionRecord{
		AppendPacketFields:        common,
		MQTTClientRecordFields:    clientFields,
		Latitude:                  nullableFloat64(record["latitude"]),
		Longitude:                 nullableFloat64(record["longitude"]),
		Altitude:                  nullableInt64(record["altitude"]),
		PositionTime:              nullableInt64(record["time"]),
		LocationSource:            nullableStringValue(record["location_source"]),
		AltitudeSource:            nullableStringValue(record["altitude_source"]),
		Timestamp:                 nullableInt64(record["timestamp"]),
		TimestampMillisAdjust:     nullableInt64(record["timestamp_millis_adjust"]),
		AltitudeHAE:               nullableInt64(record["altitude_hae"]),
		AltitudeGeoidalSeparation: nullableInt64(record["altitude_geoidal_separation"]),
		PDOP:                      nullableFloat64(record["pdop"]),
		HDOP:                      nullableFloat64(record["hdop"]),
		VDOP:                      nullableFloat64(record["vdop"]),
		GPSAccuracy:               nullableInt64(record["gps_accuracy"]),
		GroundSpeed:               nullableInt64(record["ground_speed"]),
		GroundTrack:               nullableFloat64(record["ground_track"]),
		FixQuality:                nullableInt64(record["fix_quality"]),
		FixType:                   nullableInt64(record["fix_type"]),
		SatsInView:                nullableInt64(record["sats_in_view"]),
		SensorID:                  nullableInt64(record["sensor_id"]),
		NextUpdate:                nullableInt64(record["next_update"]),
		SeqNumber:                 nullableInt64(record["seq_number"]),
		PrecisionBits:             nullableInt64(record["precision_bits"]),
	}, nil
}

func telemetryFromRecord(record map[string]any, clientInfo mqttClientInfo) (*telemetryRecord, error) {
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "telemetry", clientInfo)
	if err != nil {
		return nil, err
	}
	metricsJSON, err := nullableJSON(record["metrics"])
	if err != nil {
		return nil, fmt.Errorf("encode telemetry metrics_json: %w", err)
	}
	return &telemetryRecord{
		AppendPacketFields:     common,
		MQTTClientRecordFields: clientFields,
		TelemetryTime:          nullableInt64(record["time"]),
		TelemetryType:          nullableString(record["telemetry_type"]),
		MetricsJSON:            metricsJSON,
	}, nil
}

func routingFromRecord(record map[string]any, clientInfo mqttClientInfo) (*routingRecord, error) {
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "routing", clientInfo)
	if err != nil {
		return nil, err
	}
	return &routingRecord{AppendPacketFields: common, MQTTClientRecordFields: clientFields}, nil
}

func tracerouteFromRecord(record map[string]any, clientInfo mqttClientInfo) (*tracerouteRecord, error) {
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "traceroute", clientInfo)
	if err != nil {
		return nil, err
	}
	return &tracerouteRecord{AppendPacketFields: common, MQTTClientRecordFields: clientFields}, nil
}

func AppendPacketFieldsFromRecord(record map[string]any, wantType string, clientInfo mqttClientInfo) (AppendPacketFields, MQTTClientRecordFields, error) {
	recordType, ok := record["type"].(string)
	if !ok || recordType != wantType {
		return AppendPacketFields{}, MQTTClientRecordFields{}, fmt.Errorf("record type %v is not %s", record["type"], wantType)
	}
	fromID, ok := record["from"].(string)
	if !ok || fromID == "" {
		return AppendPacketFields{}, MQTTClientRecordFields{}, fmt.Errorf("%s missing from", wantType)
	}
	fromNum, err := int64FromAny(record["from_num"])
	if err != nil {
		return AppendPacketFields{}, MQTTClientRecordFields{}, fmt.Errorf("%s from_num: %w", wantType, err)
	}
	topic, ok := record["topic"].(string)
	if !ok || topic == "" {
		return AppendPacketFields{}, MQTTClientRecordFields{}, fmt.Errorf("%s missing topic", wantType)
	}
	contentJSON, err := json.Marshal(record)
	if err != nil {
		return AppendPacketFields{}, MQTTClientRecordFields{}, fmt.Errorf("encode %s content_json: %w", wantType, err)
	}

	return AppendPacketFields{
			FromID:         fromID,
			FromNum:        fromNum,
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
			ContentJSON:    string(contentJSON),
		}, MQTTClientRecordFields{
			MQTTClientID:   nullableString(clientInfo.ClientID),
			MQTTUsername:   nullableString(clientInfo.Username),
			MQTTListener:   nullableString(clientInfo.Listener),
			MQTTRemoteAddr: nullableString(clientInfo.RemoteAddr),
			MQTTRemoteHost: nullableString(clientInfo.RemoteHost),
			MQTTRemotePort: nullableString(clientInfo.RemotePort),
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

func nullableString(value any) *string {
	if value == nil {
		return nil
	}
	s, ok := value.(string)
	if !ok || s == "" {
		return nil
	}
	return &s
}

func nullableStringValue(value any) *string {
	if value == nil {
		return nil
	}
	if s, ok := value.(string); ok {
		if s == "" {
			return nil
		}
		return &s
	}
	s := fmt.Sprint(value)
	if s == "" || s == "<nil>" {
		return nil
	}
	return &s
}

func nullableBool(value any) *bool {
	b, ok := value.(bool)
	if !ok {
		return nil
	}
	return &b
}

func nullableInt64(value any) *int64 {
	if value == nil {
		return nil
	}
	v, err := int64FromAny(value)
	if err != nil {
		return nil
	}
	return &v
}

func nullableFloat64(value any) *float64 {
	var out float64
	switch v := value.(type) {
	case float32:
		out = float64(v)
	case float64:
		out = v
	case int:
		out = float64(v)
	case int8:
		out = float64(v)
	case int16:
		out = float64(v)
	case int32:
		out = float64(v)
	case int64:
		out = float64(v)
	case uint:
		out = float64(v)
	case uint8:
		out = float64(v)
	case uint16:
		out = float64(v)
	case uint32:
		out = float64(v)
	case uint64:
		out = float64(v)
	default:
		return nil
	}
	return &out
}

func nullableJSON(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	s := string(data)
	return &s, nil
}

func addStringUpdate(updates map[string]any, column string, value *string) {
	if value != nil {
		updates[column] = *value
	}
}

func addBoolUpdate(updates map[string]any, column string, value *bool) {
	if value != nil {
		updates[column] = *value
	}
}

func addInt64Update(updates map[string]any, column string, value *int64) {
	if value != nil {
		updates[column] = *value
	}
}

func addFloat64Update(updates map[string]any, column string, value *float64) {
	if value != nil {
		updates[column] = *value
	}
}
