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

type nodeInfoMapRecord struct {
	NodeID                 string    `gorm:"column:node_id;primaryKey;not null"`
	NodeNum                int64     `gorm:"column:node_num;not null"`
	LatestType             string    `gorm:"column:latest_type;not null"`
	UserID                 *string   `gorm:"column:user_id"`
	LongName               *string   `gorm:"column:long_name"`
	ShortName              *string   `gorm:"column:short_name"`
	HWModel                *string   `gorm:"column:hw_model"`
	Role                   *string   `gorm:"column:role"`
	IsLicensed             *bool     `gorm:"column:is_licensed"`
	PublicKey              *string   `gorm:"column:public_key"`
	FirmwareVersion        *string   `gorm:"column:firmware_version"`
	Region                 *string   `gorm:"column:region"`
	ModemPreset            *string   `gorm:"column:modem_preset"`
	Latitude               *float64  `gorm:"column:latitude"`
	Longitude              *float64  `gorm:"column:longitude"`
	Altitude               *int64    `gorm:"column:altitude"`
	PositionPrecision      *int64    `gorm:"column:position_precision"`
	NumOnlineLocalNodes    *int64    `gorm:"column:num_online_local_nodes"`
	HasOptedReportLocation *bool     `gorm:"column:has_opted_report_location"`
	ContentJSON            string    `gorm:"column:content_json;not null"`
	FirstSeenAt            time.Time `gorm:"column:first_seen_at;autoCreateTime"`
	UpdatedAt              time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (nodeInfoMapRecord) TableName() string {
	return "nodeinfo_map"
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
		if !migrator.HasTable(&nodeInfoMapRecord{}) {
			if err := migrator.CreateTable(&nodeInfoMapRecord{}); err != nil {
				return fmt.Errorf("migrate nodeinfo_map table: %w", err)
			}
		}
		if !migrator.HasTable(&textMessageRecord{}) {
			if err := migrator.CreateTable(&textMessageRecord{}); err != nil {
				return fmt.Errorf("migrate text_message table: %w", err)
			}
		}
		for _, indexName := range []string{
			"idx_text_message_from_num_created_at",
			"idx_text_message_created_at",
			"idx_text_message_packet_id",
		} {
			if !migrator.HasIndex(&textMessageRecord{}, indexName) {
				if err := migrator.CreateIndex(&textMessageRecord{}, indexName); err != nil {
					return fmt.Errorf("migrate text_message index %s: %w", indexName, err)
				}
			}
		}
		return nil
	})
}

func (s *store) UpsertNodeInfoMap(record map[string]any) error {
	node, err := nodeInfoMapFromRecord(record)
	if err != nil {
		return err
	}
	if err := s.upsertNodeInfoMapRecord(node); err != nil {
		return fmt.Errorf("upsert nodeinfo_map %s: %w", node.NodeID, err)
	}
	return nil
}

func (s *store) upsertNodeInfoMapRecord(node *nodeInfoMapRecord) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing nodeInfoMapRecord
		err := tx.Where("node_id = ?", node.NodeID).Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(node).Error; err != nil {
				return s.updateNodeInfoMapRecord(tx, node)
			}
			return nil
		}
		if err != nil {
			return err
		}
		return s.updateNodeInfoMapRecord(tx, node)
	})
}

func (s *store) updateNodeInfoMapRecord(tx *gorm.DB, node *nodeInfoMapRecord) error {
	updates := nodeInfoMapUpdates(node)
	return tx.Model(&nodeInfoMapRecord{}).Where("node_id = ?", node.NodeID).Updates(updates).Error
}

func nodeInfoMapUpdates(node *nodeInfoMapRecord) map[string]any {
	updates := map[string]any{
		"node_num":     node.NodeNum,
		"latest_type":  node.LatestType,
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
	addStringUpdate(updates, "firmware_version", node.FirmwareVersion)
	addStringUpdate(updates, "region", node.Region)
	addStringUpdate(updates, "modem_preset", node.ModemPreset)
	addFloat64Update(updates, "latitude", node.Latitude)
	addFloat64Update(updates, "longitude", node.Longitude)
	addInt64Update(updates, "altitude", node.Altitude)
	addInt64Update(updates, "position_precision", node.PositionPrecision)
	addInt64Update(updates, "num_online_local_nodes", node.NumOnlineLocalNodes)
	addBoolUpdate(updates, "has_opted_report_location", node.HasOptedReportLocation)
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
		ContentJSON:            string(contentJSON),
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
		ContentJSON:    string(contentJSON),
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
