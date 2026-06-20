package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"meshtastic_mqtt_server/internal/config"
)

type Store struct {
	db     *gorm.DB
	driver string
}

type MQTTClientInfo struct {
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

type UserRecord struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string    `gorm:"column:username;not null;uniqueIndex"`
	PasswordHash string    `gorm:"column:password_hash;not null"`
	Role         string    `gorm:"column:role;not null;index"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (UserRecord) TableName() string {
	return "users"
}

type LoginLogRecord struct {
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

func (LoginLogRecord) TableName() string {
	return "login_log"
}

type HelpContentRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Markdown  string    `gorm:"column:markdown;type:text;not null"`
	CreatedBy string    `gorm:"column:created_by;index"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index"`
}

func (HelpContentRecord) TableName() string {
	return "help_content"
}

type RuntimeSettingRecord struct {
	Key       string    `gorm:"column:key;primaryKey;size:128;not null"`
	Value     string    `gorm:"column:value;type:text;not null"`
	ValueType string    `gorm:"column:value_type;size:32;not null;index"`
	Label     string    `gorm:"column:label"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (RuntimeSettingRecord) TableName() string {
	return "runtime_settings"
}

type MapTileSourceRecord struct {
	ID              uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Name            string    `gorm:"column:name;not null;uniqueIndex"`
	URLTemplate     string    `gorm:"column:url_template;not null;uniqueIndex"`
	URLTemplateHash string    `gorm:"column:url_template_hash;size:64;not null;uniqueIndex"`
	Attribution     string    `gorm:"column:attribution"`
	MaxZoom         int       `gorm:"column:max_zoom;not null"`
	Enabled         bool      `gorm:"column:enabled;not null;index"`
	IsDefault       bool      `gorm:"column:is_default;not null;index"`
	ProxyEnabled    bool      `gorm:"column:proxy_enabled;not null;index"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (MapTileSourceRecord) TableName() string {
	return "map_tile_sources"
}

type DiscardDetailsRecord struct {
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

func (DiscardDetailsRecord) TableName() string {
	return "discard_details"
}

type SignRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	NodeID    string    `gorm:"column:node_id;not null;index"`
	LongName  *string   `gorm:"column:long_name"`
	ShortName *string   `gorm:"column:short_name"`
	SignText  string    `gorm:"column:sign_text;type:text;not null"`
	SignTime  time.Time `gorm:"column:sign_time;not null;index"`
}

func (SignRecord) TableName() string {
	return "signs"
}

type NodeBlockingRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	NodeID    string    `gorm:"column:node_id;not null;uniqueIndex"`
	NodeNum   *int64    `gorm:"column:node_num;index"`
	Reason    string    `gorm:"column:reason"`
	Enabled   bool      `gorm:"column:enabled;not null;index"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (NodeBlockingRecord) TableName() string {
	return "node_blocking"
}

type IPBlockingRecord struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	IPValue   string    `gorm:"column:ip_value;not null;uniqueIndex"`
	Reason    string    `gorm:"column:reason"`
	Enabled   bool      `gorm:"column:enabled;not null;index"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (IPBlockingRecord) TableName() string {
	return "ip_blocking"
}

type ForbiddenWordBlockingRecord struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Word          string    `gorm:"column:word;not null;uniqueIndex"`
	MatchType     string    `gorm:"column:match_type;not null;index"`
	CaseSensitive bool      `gorm:"column:case_sensitive;not null"`
	Reason        string    `gorm:"column:reason"`
	Enabled       bool      `gorm:"column:enabled;not null;index"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (ForbiddenWordBlockingRecord) TableName() string {
	return "forbidden_word_blocking"
}

type MQTTForwarderRecord struct {
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

func (MQTTForwarderRecord) TableName() string {
	return "mqtt_forwarders"
}

type MQTTForwardTopicRecord struct {
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

func (MQTTForwardTopicRecord) TableName() string {
	return "mqtt_forward_topics"
}

type BotNodeRecord struct {
	ID                               uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	NodeID                           string     `gorm:"column:node_id;not null;uniqueIndex"`
	NodeNum                          int64      `gorm:"column:node_num;not null;uniqueIndex"`
	LongName                         string     `gorm:"column:long_name;not null"`
	ShortName                        string     `gorm:"column:short_name;not null"`
	Enabled                          bool       `gorm:"column:enabled;not null;index"`
	DefaultChannelID                 string     `gorm:"column:default_channel_id;not null;index"`
	TopicPrefix                      string     `gorm:"column:topic_prefix;not null"`
	PSK                              string     `gorm:"column:psk;not null;size:64"`
	PublicKey                        string     `gorm:"column:public_key;type:text"`
	PrivateKey                       string     `gorm:"column:private_key;type:text"`
	NodeInfoBroadcastEnabled         bool       `gorm:"column:nodeinfo_broadcast_enabled;not null;index"`
	NodeInfoBroadcastIntervalSeconds int64      `gorm:"column:nodeinfo_broadcast_interval_seconds;not null"`
	LastNodeInfoBroadcastAt          *time.Time `gorm:"column:last_nodeinfo_broadcast_at;index"`
	LastPacketID                     int64      `gorm:"column:last_packet_id;not null"`
	LLMQueueEnabled                  bool       `gorm:"column:llm_queue_enabled;not null;default:1;index"`
	LLMIncludeChannelMessages        bool       `gorm:"column:llm_include_channel_messages;not null;default:0;index"`
	CreatedAt                        time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt                        time.Time  `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (BotNodeRecord) TableName() string {
	return "bot_nodes"
}

type BotMessageRecord struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	BotID       uint64     `gorm:"column:bot_id;not null;index:idx_bot_message_bot_created_at,priority:1"`
	BotNodeID   string     `gorm:"column:bot_node_id;not null;index"`
	BotNodeNum  int64      `gorm:"column:bot_node_num;not null;index"`
	MessageType string     `gorm:"column:message_type;not null;index"`
	ChannelID   string     `gorm:"column:channel_id;not null;index"`
	ToNodeID    *string    `gorm:"column:to_node_id;index"`
	ToNodeNum   *int64     `gorm:"column:to_node_num;index"`
	Topic       string     `gorm:"column:topic;not null"`
	PacketID    int64      `gorm:"column:packet_id;not null;index"`
	Text        string     `gorm:"column:text;type:text;not null"`
	PayloadLen  int64      `gorm:"column:payload_len;not null"`
	Encrypted   bool       `gorm:"column:encrypted;not null;index"`
	Status      string     `gorm:"column:status;not null;index"`
	Error       string     `gorm:"column:error;type:text"`
	PublishedAt *time.Time `gorm:"column:published_at;index"`
	CreatedBy   string     `gorm:"column:created_by;index"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime;index:idx_bot_message_bot_created_at,priority:2"`
}

func (BotMessageRecord) TableName() string {
	return "bot_messages"
}

// BotDirectMessageRecord 专门保存机器人参与的 PKI 私聊（DM）。
//
//   - 设计原因：text_message 表只存频道消息；DM 是端到端的，逻辑上属于 “一对会话”，需要按
//     bot+对端聚合渲染，与 text_message 全表浏览的形态不一样。
//   - direction = "outbound" 表示 bot → device；"inbound" 表示 device → bot。
//   - 出向消息在发送时插入 status=pending，发送成功后更新为 published；入向消息默认直接
//     published。两种方向都通过 bot_id/peer_node_num 索引快速回放会话。
type BotDirectMessageRecord struct {
	ID           uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	BotID        uint64     `gorm:"column:bot_id;not null;index:idx_bot_dm_bot_peer,priority:1;index:idx_bot_dm_bot_created_at,priority:1"`
	BotNodeID    string     `gorm:"column:bot_node_id;not null;index"`
	BotNodeNum   int64      `gorm:"column:bot_node_num;not null;index"`
	PeerNodeID   string     `gorm:"column:peer_node_id;not null;index:idx_bot_dm_bot_peer,priority:2"`
	PeerNodeNum  int64      `gorm:"column:peer_node_num;not null;index"`
	Direction    string     `gorm:"column:direction;not null;index"`
	Topic        string     `gorm:"column:topic;not null"`
	PacketID     int64      `gorm:"column:packet_id;not null;index"`
	Text         string     `gorm:"column:text;type:text;not null"`
	PayloadLen   int64      `gorm:"column:payload_len;not null"`
	PKIEncrypted bool       `gorm:"column:pki_encrypted;not null"`
	WantAck      bool       `gorm:"column:want_ack;not null"`
	GatewayID    *string    `gorm:"column:gateway_id"`
	Status       string     `gorm:"column:status;not null;index"`
	Error        string     `gorm:"column:error;type:text"`
	BotMessageID *uint64    `gorm:"column:bot_message_id;index"`
	CreatedBy    *string    `gorm:"column:created_by"`
	PublishedAt  *time.Time `gorm:"column:published_at;index"`
	ReceivedAt   *time.Time `gorm:"column:received_at;index"`
	// ReadAt 仅对 inbound 消息有意义：管理员在前端打开会话视为“已读”，会通过 read API 写入此字段。
	// 出向消息默认在创建时就设置为已读，避免出现在未读统计里。
	ReadAt      *time.Time `gorm:"column:read_at;index"`
	ContentJSON *string    `gorm:"column:content_json;type:text"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime;index:idx_bot_dm_bot_created_at,priority:2"`
}

func (BotDirectMessageRecord) TableName() string {
	return "bot_direct_messages"
}

const (
	BotDirectMessageDirectionInbound  = "inbound"
	BotDirectMessageDirectionOutbound = "outbound"
)

// LLMMessageQueueRecord 是 LLM 消息队列，用于暂存机器人收到的消息供 LLM 处理。
//
//   - 每个队列绑定一个 BotID，消息包含节点信息和消息内容
//   - deleted_at 用于标记软删除，实际保留一段时间供去重
//   - received_at 是消息接收时间，processed_at 是 LLM 处理完成时间
type LLMMessageQueueRecord struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	BotID       uint64     `gorm:"column:bot_id;not null;index:idx_llm_queue_bot_created,priority:1"`
	BotNodeID   string     `gorm:"column:bot_node_id;not null;index"`
	BotNodeNum  int64      `gorm:"column:bot_node_num;not null;index"`
	FromNodeID  string     `gorm:"column:from_node_id;not null;index"`
	FromNodeNum int64      `gorm:"column:from_node_num;not null;index"`
	LongName    *string    `gorm:"column:long_name"`
	ShortName   *string    `gorm:"column:short_name"`
	Text        string     `gorm:"column:text;type:text;not null"`
	PacketID    int64      `gorm:"column:packet_id;not null;index"`
	ChannelID   *string    `gorm:"column:channel_id"`
	Topic       string     `gorm:"column:topic;not null"`
	MessageType string     `gorm:"column:message_type;not null;default:'direct'"` // "channel" 或 "direct"
	Status      string     `gorm:"column:status;not null;index"`
	Error       string     `gorm:"column:error;type:text"`
	Reply       string     `gorm:"column:reply;type:text"`
	ReceivedAt  time.Time  `gorm:"column:received_at;not null;index"`
	ProcessedAt *time.Time `gorm:"column:processed_at;index"`
	DeletedAt   *time.Time `gorm:"column:deleted_at;index"`
	ContentJSON *string    `gorm:"column:content_json;type:text"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime;index:idx_llm_queue_bot_created,priority:2"`
}

func (LLMMessageQueueRecord) TableName() string {
	return "llm_message_queue"
}

const (
	LLMMessageStatusPending    = "pending"
	LLMMessageStatusProcessing = "processing"
	LLMMessageStatusProcessed  = "processed"
	LLMMessageStatusError      = "error"
)

// llmQueueProcessedDedupWindow 是 processed 消息软删除后仍参与去重的时间窗口。
// 处理完即软删除，但记录会保留至此窗口结束，防止网络延迟/重投导致同一包在刚处理完后又被重复入队。
const llmQueueProcessedDedupWindow = 15 * time.Second

type NodeInfoRecord struct {
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

func (NodeInfoRecord) TableName() string {
	return "nodeinfo"
}

type MapReportRecord struct {
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

func (MapReportRecord) TableName() string {
	return "map_report"
}

type TextMessageRecord struct {
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

func (TextMessageRecord) TableName() string {
	return "text_message"
}

// LLMProviderRecord 保存 LLM API 配置，支持多个 AI 提供商
type LLMProviderRecord struct {
	Name               string    `gorm:"column:name;primaryKey;size:64;not null"` // 配置名称，如 "default"、"openai"、"ark" 等
	Active             bool      `gorm:"column:active;not null;index"`            // 是否启用此配置
	APIKey             string    `gorm:"column:api_key;type:text;not null"`       // API 密钥
	BaseURL            string    `gorm:"column:base_url;type:text;not null"`      // API 基础 URL
	Model              string    `gorm:"column:model;not null"`                   // 模型名称
	Timeout            int       `gorm:"column:timeout;not null;default:120"`     // 超时时间（秒）
	ContextWindowTokens int      `gorm:"column:context_window_tokens;not null;default:262144"` // 上下文窗口 token 数
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (LLMProviderRecord) TableName() string {
	return "llm_providers"
}

// LLMToolRouterRecord 保存工具路由的配置
type LLMToolRouterRecord struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Enabled      bool      `gorm:"column:enabled;not null;index"`           // 是否启用工具路由
	OpenAIName   string    `gorm:"column:openai_name;size:64;not null"`     // 使用的 LLM 提供商名称（关联 llm_providers.name）
	Timeout      int       `gorm:"column:timeout;not null;default:30"`      // 工具调用超时时间（秒）
	MaxTokens    int       `gorm:"column:max_tokens;not null;default:512"`  // 工具调用最大 token 数
	SystemPrompt string    `gorm:"column:system_prompt;type:text;not null"` // 系统提示词
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (LLMToolRouterRecord) TableName() string {
	return "llm_tool_router"
}

// LLMTopicConfigRecord 保存话题选择的配置
type LLMTopicConfigRecord struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Enabled      bool      `gorm:"column:enabled;not null;index"`           // 是否启用话题选择
	OpenAIName   string    `gorm:"column:openai_name;size:64;not null"`     // 使用的 LLM 提供商名称（关联 llm_providers.name）
	Timeout      int       `gorm:"column:timeout;not null;default:30"`      // 话题判定超时时间（秒）
	MaxTokens    int       `gorm:"column:max_tokens;not null;default:512"`  // 话题判定最大 token 数
	SystemPrompt string    `gorm:"column:system_prompt;type:text;not null"` // 系统提示词
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (LLMTopicConfigRecord) TableName() string {
	return "llm_topic_config"
}

// LLMPrimaryConfigRecord 保存主 AI 回复的配置
type LLMPrimaryConfigRecord struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Enabled       bool      `gorm:"column:enabled;not null;index"`             // 是否启用 AI 回复
	ProviderName  string    `gorm:"column:provider_name;size:64;not null"`     // 使用的 LLM 提供商名称（关联 llm_providers.name）
	Timeout       int       `gorm:"column:timeout;not null;default:120"`       // 请求超时时间（秒）
	MaxTokens     int       `gorm:"column:max_tokens;not null;default:1024"`   // 回复最大 token 数
	SystemPrompt  string    `gorm:"column:system_prompt;type:text;not null"`   // 默认系统提示词
	EnableTool    bool      `gorm:"column:enable_tool;not null;default:false"` // 是否启用工具调用
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime;index"`
}

func (LLMPrimaryConfigRecord) TableName() string {
	return "llm_primary_config"
}

type PositionRecord struct {
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

func (PositionRecord) TableName() string {
	return "position"
}

type TelemetryRecord struct {
	AppendPacketFields     `gorm:"embedded"`
	MQTTClientRecordFields `gorm:"embedded"`
	TelemetryTime          *int64  `gorm:"column:telemetry_time"`
	TelemetryType          *string `gorm:"column:telemetry_type;index"`
	MetricsJSON            *string `gorm:"column:metrics_json"`
}

func (TelemetryRecord) TableName() string {
	return "telemetry"
}

type RoutingRecord struct {
	AppendPacketFields     `gorm:"embedded"`
	MQTTClientRecordFields `gorm:"embedded"`
}

func (RoutingRecord) TableName() string {
	return "routing"
}

type TracerouteRecord struct {
	AppendPacketFields     `gorm:"embedded"`
	MQTTClientRecordFields `gorm:"embedded"`
}

func (TracerouteRecord) TableName() string {
	return "traceroute"
}

func OpenStore(cfg config.DatabaseConfig, consoleLog bool) (*Store, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case config.DriverSQLite:
		if err := os.MkdirAll(filepath.Dir(cfg.SQLite.Path), 0755); err != nil {
			return nil, fmt.Errorf("create sqlite directory %s: %w", filepath.Dir(cfg.SQLite.Path), err)
		}
		dialector = sqlite.Open(cfg.SQLite.Path)
	case config.DriverMySQL:
		dialector = mysql.Open(cfg.MySQL.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}

	logLevel := gormlogger.Warn
	if !consoleLog {
		logLevel = gormlogger.Silent
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.New(log.New(os.Stderr, "\r\n", log.LstdFlags), gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
	})
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

	s := &Store{db: db, driver: cfg.Driver}
	if err := s.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return s, nil
}

// DB 返回底层 gorm 句柄，供 ai 等子系统在受控范围内直接执行查询。
// 应优先使用 Store 的高级方法；只有在确需新 schema 或自定义查询时才直接拿 DB。
func (s *Store) DB() *gorm.DB {
	return s.db
}

// Driver 返回当前使用的数据库驱动名（与 config.DriverSQLite/MySQL 一致）。
func (s *Store) Driver() string {
	return s.driver
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) migrate() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		migrator := tx.Migrator()
		for _, item := range []struct {
			label string
			model any
		}{
			{label: "users", model: &UserRecord{}},
			{label: "login_log", model: &LoginLogRecord{}},
			{label: "help_content", model: &HelpContentRecord{}},
			{label: "runtime_settings", model: &RuntimeSettingRecord{}},
			{label: "map_tile_sources", model: &MapTileSourceRecord{}},
			{label: "discard_details", model: &DiscardDetailsRecord{}},
			{label: "signs", model: &SignRecord{}},
			{label: "node_blocking", model: &NodeBlockingRecord{}},
			{label: "ip_blocking", model: &IPBlockingRecord{}},
			{label: "forbidden_word_blocking", model: &ForbiddenWordBlockingRecord{}},
			{label: "mqtt_forwarders", model: &MQTTForwarderRecord{}},
			{label: "mqtt_forward_topics", model: &MQTTForwardTopicRecord{}},
			{label: "bot_nodes", model: &BotNodeRecord{}},
			{label: "bot_messages", model: &BotMessageRecord{}},
			{label: "bot_direct_messages", model: &BotDirectMessageRecord{}},
			{label: "llm_message_queue", model: &LLMMessageQueueRecord{}},
			{label: "llm_providers", model: &LLMProviderRecord{}},
			{label: "llm_tool_router", model: &LLMToolRouterRecord{}},
			{label: "llm_topic_config", model: &LLMTopicConfigRecord{}},
			{label: "llm_primary_config", model: &LLMPrimaryConfigRecord{}},
			{label: "nodeinfo", model: &NodeInfoRecord{}},
			{label: "map_report", model: &MapReportRecord{}},
			{label: "text_message", model: &TextMessageRecord{}},
			{label: "position", model: &PositionRecord{}},
			{label: "telemetry", model: &TelemetryRecord{}},
			{label: "routing", model: &RoutingRecord{}},
			{label: "traceroute", model: &TracerouteRecord{}},
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
			{label: "text_message", model: &TextMessageRecord{}, indexes: []string{"idx_text_message_from_num_created_at", "idx_text_message_created_at", "idx_text_message_packet_id"}},
			{label: "bot_direct_messages", model: &BotDirectMessageRecord{}, indexes: []string{"idx_bot_dm_bot_peer", "idx_bot_dm_bot_created_at"}},
			{label: "llm_message_queue", model: &LLMMessageQueueRecord{}, indexes: []string{"idx_llm_queue_bot_created"}},
		} {
			if err := createMissingIndexes(migrator, item.model, item.label, item.indexes); err != nil {
				return err
			}
		}
		if err := migrateBotNodePSK(tx, migrator, s.driver); err != nil {
			return err
		}
		if err := migrateBotDirectMessages(tx, migrator); err != nil {
			return err
		}
		if err := migrateMapTileSourceHash(tx, migrator, s.driver); err != nil {
			return err
		}
		txStore := &Store{db: tx, driver: s.driver}
		if err := txStore.EnsureDefaultMapTileSource(); err != nil {
			return err
		}
		if err := txStore.EnsureDefaultLLMProvider(); err != nil {
			return err
		}
		if err := txStore.EnsureDefaultLLMToolRouter(); err != nil {
			return err
		}
		if err := txStore.EnsureDefaultLLMTopicConfig(); err != nil {
			return err
		}
		if err := txStore.EnsureDefaultLLMPrimaryConfig(); err != nil {
			return err
		}
		return nil
	})
}

func migrateBotNodePSK(tx *gorm.DB, migrator gorm.Migrator, driver string) error {
	if !migrator.HasTable(&BotNodeRecord{}) {
		return nil
	}
	if !migrator.HasColumn(&BotNodeRecord{}, "PSK") {
		if driver == config.DriverSQLite {
			if err := tx.Exec("ALTER TABLE bot_nodes ADD COLUMN psk TEXT NOT NULL DEFAULT 'AQ=='").Error; err != nil {
				return fmt.Errorf("migrate bot_nodes psk column: %w", err)
			}
		} else if err := tx.Exec("ALTER TABLE bot_nodes ADD COLUMN psk VARCHAR(64) NOT NULL DEFAULT 'AQ=='").Error; err != nil {
			return fmt.Errorf("migrate bot_nodes psk column: %w", err)
		}
	}
	if !migrator.HasColumn(&BotNodeRecord{}, "PublicKey") {
		if err := tx.Exec("ALTER TABLE bot_nodes ADD COLUMN public_key text").Error; err != nil {
			return fmt.Errorf("migrate bot_nodes public_key column: %w", err)
		}
	}
	if !migrator.HasColumn(&BotNodeRecord{}, "PrivateKey") {
		if err := tx.Exec("ALTER TABLE bot_nodes ADD COLUMN private_key text").Error; err != nil {
			return fmt.Errorf("migrate bot_nodes private_key column: %w", err)
		}
	}
	if !migrator.HasColumn(&BotNodeRecord{}, "NodeInfoBroadcastEnabled") {
		if err := tx.Exec("ALTER TABLE bot_nodes ADD COLUMN nodeinfo_broadcast_enabled numeric NOT NULL DEFAULT true").Error; err != nil {
			return fmt.Errorf("migrate bot_nodes nodeinfo_broadcast_enabled column: %w", err)
		}
	}
	if !migrator.HasColumn(&BotNodeRecord{}, "NodeInfoBroadcastIntervalSeconds") {
		if err := tx.Exec("ALTER TABLE bot_nodes ADD COLUMN nodeinfo_broadcast_interval_seconds bigint NOT NULL DEFAULT 3600").Error; err != nil {
			return fmt.Errorf("migrate bot_nodes nodeinfo_broadcast_interval_seconds column: %w", err)
		}
	}
	if !migrator.HasColumn(&BotNodeRecord{}, "LastNodeInfoBroadcastAt") {
		if err := tx.Exec("ALTER TABLE bot_nodes ADD COLUMN last_nodeinfo_broadcast_at datetime NULL").Error; err != nil {
			return fmt.Errorf("migrate bot_nodes last_nodeinfo_broadcast_at column: %w", err)
		}
	}
	if !migrator.HasColumn(&BotNodeRecord{}, "LLMQueueEnabled") {
		if err := tx.Exec("ALTER TABLE bot_nodes ADD COLUMN llm_queue_enabled numeric NOT NULL DEFAULT 1").Error; err != nil {
			return fmt.Errorf("migrate bot_nodes llm_queue_enabled column: %w", err)
		}
	}
	if !migrator.HasColumn(&BotNodeRecord{}, "LLMIncludeChannelMessages") {
		if err := tx.Exec("ALTER TABLE bot_nodes ADD COLUMN llm_include_channel_messages numeric NOT NULL DEFAULT 0").Error; err != nil {
			return fmt.Errorf("migrate bot_nodes llm_include_channel_messages column: %w", err)
		}
	}
	// 迁移 LLM 消息队列 reply 列
	if migrator.HasTable(&LLMMessageQueueRecord{}) && !migrator.HasColumn(&LLMMessageQueueRecord{}, "Reply") {
		if err := tx.Exec("ALTER TABLE llm_message_queue ADD COLUMN reply text").Error; err != nil {
			return fmt.Errorf("migrate llm_message_queue reply column: %w", err)
		}
	}
	// 迁移 LLM 消息队列 message_type 列
	if migrator.HasTable(&LLMMessageQueueRecord{}) && !migrator.HasColumn(&LLMMessageQueueRecord{}, "MessageType") {
		if err := tx.Exec("ALTER TABLE llm_message_queue ADD COLUMN message_type text NOT NULL DEFAULT 'direct'").Error; err != nil {
			return fmt.Errorf("migrate llm_message_queue message_type column: %w", err)
		}
	}
	return nil
}

func migrateBotDirectMessages(tx *gorm.DB, migrator gorm.Migrator) error {
	if !migrator.HasTable(&BotDirectMessageRecord{}) {
		return nil
	}
	if !migrator.HasColumn(&BotDirectMessageRecord{}, "ReadAt") {
		if err := tx.Exec("ALTER TABLE bot_direct_messages ADD COLUMN read_at datetime").Error; err != nil {
			return fmt.Errorf("migrate bot_direct_messages read_at column: %w", err)
		}
	}
	// 历史 outbound 消息默认视为已读，避免出现在未读统计里。
	if err := tx.Exec("UPDATE bot_direct_messages SET read_at = created_at WHERE direction = ? AND read_at IS NULL", BotDirectMessageDirectionOutbound).Error; err != nil {
		return fmt.Errorf("backfill bot_direct_messages outbound read_at: %w", err)
	}
	if !migrator.HasIndex(&BotDirectMessageRecord{}, "idx_bot_direct_messages_read_at") {
		// HasIndex 用 struct 字段名映射的索引名匹配，column 标签写的 read_at 不一定生成此名，
		// 所以直接 IF NOT EXISTS 创建（SQLite + MySQL 都支持）。
		if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_bot_direct_messages_read_at ON bot_direct_messages(read_at)").Error; err != nil {
			return fmt.Errorf("migrate bot_direct_messages read_at index: %w", err)
		}
	}
	return nil
}

func migrateMapTileSourceHash(tx *gorm.DB, migrator gorm.Migrator, driver string) error {
	if !migrator.HasColumn(&MapTileSourceRecord{}, "ProxyEnabled") {
		if driver == config.DriverSQLite {
			if err := tx.Exec("ALTER TABLE map_tile_sources ADD COLUMN proxy_enabled numeric NOT NULL DEFAULT true").Error; err != nil {
				return fmt.Errorf("migrate map_tile_sources proxy_enabled column: %w", err)
			}
		} else if err := migrator.AddColumn(&MapTileSourceRecord{}, "ProxyEnabled"); err != nil {
			return fmt.Errorf("migrate map_tile_sources proxy_enabled column: %w", err)
		}
	}
	if !migrator.HasColumn(&MapTileSourceRecord{}, "URLTemplateHash") {
		if driver == config.DriverSQLite {
			if err := tx.Exec("ALTER TABLE map_tile_sources ADD COLUMN url_template_hash TEXT NOT NULL DEFAULT ''").Error; err != nil {
				return fmt.Errorf("migrate map_tile_sources url_template_hash column: %w", err)
			}
		} else if err := migrator.AddColumn(&MapTileSourceRecord{}, "URLTemplateHash"); err != nil {
			return fmt.Errorf("migrate map_tile_sources url_template_hash column: %w", err)
		}
	}

	var rows []MapTileSourceRecord
	if err := tx.Model(&MapTileSourceRecord{}).Where("url_template_hash = '' OR url_template_hash IS NULL").Find(&rows).Error; err != nil {
		return fmt.Errorf("list map_tile_sources missing url_template_hash: %w", err)
	}
	for _, row := range rows {
		if err := tx.Model(&MapTileSourceRecord{}).Where("id = ?", row.ID).Update("url_template_hash", MapTileSourceHash(row.URLTemplate)).Error; err != nil {
			return fmt.Errorf("backfill map_tile_sources url_template_hash: %w", err)
		}
	}
	if !migrator.HasIndex(&MapTileSourceRecord{}, "idx_map_tile_sources_url_template_hash") {
		if err := migrator.CreateIndex(&MapTileSourceRecord{}, "idx_map_tile_sources_url_template_hash"); err != nil {
			return fmt.Errorf("migrate map_tile_sources index idx_map_tile_sources_url_template_hash: %w", err)
		}
	}
	return nil
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

func (s *Store) UpsertNodeInfo(record map[string]any) error {
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

func (s *Store) UpsertMapReport(record map[string]any) error {
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

func (s *Store) upsertNodeInfoRecord(node *NodeInfoRecord) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var existing NodeInfoRecord
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

func (s *Store) upsertMapReportRecord(report *MapReportRecord) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.upsertMapReportRecordTx(tx, report)
	})
}

func (s *Store) upsertMapReportRecordTx(tx *gorm.DB, report *MapReportRecord) error {
	var existing MapReportRecord
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

func (s *Store) updateNodeInfoRecord(tx *gorm.DB, node *NodeInfoRecord) error {
	updates := nodeInfoUpdates(node)
	return tx.Model(&NodeInfoRecord{}).Where("node_id = ?", node.NodeID).Updates(updates).Error
}

func (s *Store) updateMapReportRecord(tx *gorm.DB, report *MapReportRecord) error {
	updates := mapReportUpdates(report)
	return tx.Model(&MapReportRecord{}).Where("node_id = ?", report.NodeID).Updates(updates).Error
}

func (s *Store) updateMapReportFromNodeInfo(node *NodeInfoRecord) error {
	updates := map[string]any{
		"node_num":   node.NodeNum,
		"updated_at": time.Now(),
	}
	addStringUpdate(updates, "long_name", node.LongName)
	addStringUpdate(updates, "short_name", node.ShortName)
	addStringUpdate(updates, "hw_model", node.HWModel)
	addStringUpdate(updates, "role", node.Role)
	return s.db.Model(&MapReportRecord{}).Where("node_id = ?", node.NodeID).Updates(updates).Error
}

func (s *Store) updateNodeInfoFromMapReport(report *MapReportRecord) error {
	updates := map[string]any{
		"node_num":   report.NodeNum,
		"updated_at": time.Now(),
	}
	addStringUpdate(updates, "long_name", report.LongName)
	addStringUpdate(updates, "short_name", report.ShortName)
	addStringUpdate(updates, "hw_model", report.HWModel)
	addStringUpdate(updates, "role", report.Role)
	return s.db.Model(&NodeInfoRecord{}).Where("node_id = ?", report.NodeID).Updates(updates).Error
}

func nodeInfoUpdates(node *NodeInfoRecord) map[string]any {
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

func mapReportUpdates(report *MapReportRecord) map[string]any {
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

func (s *Store) InsertTextMessage(record map[string]any, clientInfo MQTTClientInfo) error {
	message, err := textMessageFromRecord(record, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(message).Error; err != nil {
		return fmt.Errorf("insert text_message from %s: %w", message.FromID, err)
	}
	return nil
}

func (s *Store) InsertPosition(record map[string]any, clientInfo MQTTClientInfo) error {
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

func (s *Store) upsertMapReportFromPosition(tx *gorm.DB, position *PositionRecord) error {
	report := &MapReportRecord{
		NodeID:            position.FromID,
		NodeNum:           position.FromNum,
		Latitude:          position.Latitude,
		Longitude:         position.Longitude,
		Altitude:          position.Altitude,
		PositionPrecision: position.PrecisionBits,
		ContentJSON:       position.ContentJSON,
	}

	var existing MapReportRecord
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
	return tx.Model(&MapReportRecord{}).Where("node_id = ?", position.FromID).Updates(updates).Error
}

func (s *Store) InsertTelemetry(record map[string]any, clientInfo MQTTClientInfo) error {
	telemetry, err := telemetryFromRecord(record, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(telemetry).Error; err != nil {
		return fmt.Errorf("insert telemetry from %s: %w", telemetry.FromID, err)
	}
	return nil
}

func (s *Store) InsertRouting(record map[string]any, clientInfo MQTTClientInfo) error {
	routing, err := routingFromRecord(record, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(routing).Error; err != nil {
		return fmt.Errorf("insert routing from %s: %w", routing.FromID, err)
	}
	return nil
}

func (s *Store) InsertTraceroute(record map[string]any, clientInfo MQTTClientInfo) error {
	traceroute, err := tracerouteFromRecord(record, clientInfo)
	if err != nil {
		return err
	}
	if err := s.db.Create(traceroute).Error; err != nil {
		return fmt.Errorf("insert traceroute from %s: %w", traceroute.FromID, err)
	}
	return nil
}

func nodeInfoFromRecord(record map[string]any) (*NodeInfoRecord, error) {
	recordType, ok := record["type"].(string)
	if !ok || recordType != "nodeinfo" {
		return nil, fmt.Errorf("record type %v is not nodeinfo", record["type"])
	}
	nodeID, nodeNum, contentJSON, err := nodeRecordBase(record, "nodeinfo")
	if err != nil {
		return nil, err
	}

	return &NodeInfoRecord{
		NodeID:      nodeID,
		NodeNum:     nodeNum,
		UserID:      NullableString(record["user_id"]),
		LongName:    NullableString(record["long_name"]),
		ShortName:   NullableString(record["short_name"]),
		HWModel:     NullableString(record["hw_model"]),
		Role:        NullableString(record["role"]),
		IsLicensed:  nullableBool(record["is_licensed"]),
		PublicKey:   NullableString(record["public_key"]),
		ContentJSON: contentJSON,
	}, nil
}

func mapReportFromRecord(record map[string]any) (*MapReportRecord, error) {
	recordType, ok := record["type"].(string)
	if !ok || recordType != "map_report" {
		return nil, fmt.Errorf("record type %v is not map_report", record["type"])
	}
	nodeID, nodeNum, contentJSON, err := nodeRecordBase(record, "map_report")
	if err != nil {
		return nil, err
	}

	return &MapReportRecord{
		NodeID:                 nodeID,
		NodeNum:                nodeNum,
		LongName:               NullableString(record["long_name"]),
		ShortName:              NullableString(record["short_name"]),
		HWModel:                NullableString(record["hw_model"]),
		Role:                   NullableString(record["role"]),
		FirmwareVersion:        NullableString(record["firmware_version"]),
		Region:                 NullableString(record["region"]),
		ModemPreset:            NullableString(record["modem_preset"]),
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

func textMessageFromRecord(record map[string]any, clientInfo MQTTClientInfo) (*TextMessageRecord, error) {
	recordType, ok := record["type"].(string)
	if !ok || recordType != "text_message" {
		return nil, fmt.Errorf("record type %v is not text_message", record["type"])
	}
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "text_message", clientInfo)
	if err != nil {
		return nil, err
	}
	return &TextMessageRecord{
		FromID:         common.FromID,
		FromNum:        common.FromNum,
		Text:           NullableString(record["text"]),
		PayloadHex:     NullableString(record["payload_hex"]),
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

func positionFromRecord(record map[string]any, clientInfo MQTTClientInfo) (*PositionRecord, error) {
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "position", clientInfo)
	if err != nil {
		return nil, err
	}
	return &PositionRecord{
		AppendPacketFields:        common,
		MQTTClientRecordFields:    clientFields,
		Latitude:                  nullableFloat64(record["latitude"]),
		Longitude:                 nullableFloat64(record["longitude"]),
		Altitude:                  nullableInt64(record["altitude"]),
		PositionTime:              nullableInt64(record["time"]),
		LocationSource:            NullableStringValue(record["location_source"]),
		AltitudeSource:            NullableStringValue(record["altitude_source"]),
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

func telemetryFromRecord(record map[string]any, clientInfo MQTTClientInfo) (*TelemetryRecord, error) {
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "telemetry", clientInfo)
	if err != nil {
		return nil, err
	}
	metricsJSON, err := nullableJSON(record["metrics"])
	if err != nil {
		return nil, fmt.Errorf("encode telemetry metrics_json: %w", err)
	}
	return &TelemetryRecord{
		AppendPacketFields:     common,
		MQTTClientRecordFields: clientFields,
		TelemetryTime:          nullableInt64(record["time"]),
		TelemetryType:          NullableString(record["telemetry_type"]),
		MetricsJSON:            metricsJSON,
	}, nil
}

func routingFromRecord(record map[string]any, clientInfo MQTTClientInfo) (*RoutingRecord, error) {
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "routing", clientInfo)
	if err != nil {
		return nil, err
	}
	return &RoutingRecord{AppendPacketFields: common, MQTTClientRecordFields: clientFields}, nil
}

func tracerouteFromRecord(record map[string]any, clientInfo MQTTClientInfo) (*TracerouteRecord, error) {
	common, clientFields, err := AppendPacketFieldsFromRecord(record, "traceroute", clientInfo)
	if err != nil {
		return nil, err
	}
	return &TracerouteRecord{AppendPacketFields: common, MQTTClientRecordFields: clientFields}, nil
}

func AppendPacketFieldsFromRecord(record map[string]any, wantType string, clientInfo MQTTClientInfo) (AppendPacketFields, MQTTClientRecordFields, error) {
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
			ChannelID:      NullableString(record["channel_id"]),
			GatewayID:      NullableString(record["gateway_id"]),
			PacketID:       nullableInt64(record["packet_id"]),
			PacketTo:       NullableString(record["packet_to"]),
			PacketToNum:    nullableInt64(record["packet_to_num"]),
			Portnum:        NullableString(record["portnum"]),
			PayloadLen:     nullableInt64(record["payload_len"]),
			PayloadVariant: NullableString(record["payload_variant"]),
			ViaMQTT:        nullableBool(record["via_mqtt"]),
			PKIEncrypted:   nullableBool(record["pki_encrypted"]),
			DecryptSuccess: nullableBool(record["decrypt_success"]),
			DecryptStatus:  NullableString(record["decrypt_status"]),
			ContentJSON:    string(contentJSON),
		}, MQTTClientRecordFields{
			MQTTClientID:   NullableString(clientInfo.ClientID),
			MQTTUsername:   NullableString(clientInfo.Username),
			MQTTListener:   NullableString(clientInfo.Listener),
			MQTTRemoteAddr: NullableString(clientInfo.RemoteAddr),
			MQTTRemoteHost: NullableString(clientInfo.RemoteHost),
			MQTTRemotePort: NullableString(clientInfo.RemotePort),
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

func NullableString(value any) *string {
	if value == nil {
		return nil
	}
	s, ok := value.(string)
	if !ok || s == "" {
		return nil
	}
	return &s
}

func NullableStringValue(value any) *string {
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
