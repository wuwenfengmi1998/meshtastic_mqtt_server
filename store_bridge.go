package main

// 桥接到 internal/store —— 让根目录其余文件无须修改即可继续使用旧的小写类型/函数名。
// 当各领域包逐步迁出根目录后，可以删除这些别名。

import (
	storepkg "meshtastic_mqtt_server/internal/store"
)

// ---- 类型别名 ----

type (
	store                       = storepkg.Store
	mqttClientInfo              = storepkg.MQTTClientInfo
	dbWriteQueue                = storepkg.WriteQueue
	listOptions                 = storepkg.ListOptions
	mapReportViewportOptions    = storepkg.MapReportViewportOptions
	mapReportViewportResult     = storepkg.MapReportViewportResult
	mapReportClusterRecord      = storepkg.MapReportClusterRecord
	userRecord                  = storepkg.UserRecord
	loginLogRecord              = storepkg.LoginLogRecord
	helpContentRecord           = storepkg.HelpContentRecord
	runtimeSettingRecord        = storepkg.RuntimeSettingRecord
	mapTileSourceRecord         = storepkg.MapTileSourceRecord
	mapTileSourceInput          = storepkg.MapTileSourceInput
	discardDetailsRecord        = storepkg.DiscardDetailsRecord
	signRecord                  = storepkg.SignRecord
	nodeBlockingRecord          = storepkg.NodeBlockingRecord
	ipBlockingRecord            = storepkg.IPBlockingRecord
	forbiddenWordBlockingRecord = storepkg.ForbiddenWordBlockingRecord
	mqttForwarderRecord         = storepkg.MQTTForwarderRecord
	mqttForwarderInput          = storepkg.MQTTForwarderInput
	mqttForwardTopicRecord      = storepkg.MQTTForwardTopicRecord
	mqttForwardTopicInput       = storepkg.MQTTForwardTopicInput
	botNodeRecord               = storepkg.BotNodeRecord
	botNodeInput                = storepkg.BotNodeInput
	botMessageRecord            = storepkg.BotMessageRecord
	botDirectMessageRecord      = storepkg.BotDirectMessageRecord
	llmMessageQueueRecord       = storepkg.LLMMessageQueueRecord
	nodeInfoRecord              = storepkg.NodeInfoRecord
	mapReportRecord             = storepkg.MapReportRecord
	textMessageRecord           = storepkg.TextMessageRecord
	llmProviderRecord           = storepkg.LLMProviderRecord
	llmToolRouterRecord         = storepkg.LLMToolRouterRecord
	llmPrimaryConfigRecord      = storepkg.LLMPrimaryConfigRecord
	positionRecord              = storepkg.PositionRecord
	telemetryRecord             = storepkg.TelemetryRecord
	routingRecord               = storepkg.RoutingRecord
	tracerouteRecord            = storepkg.TracerouteRecord
)

// AppendPacketFields / MQTTClientRecordFields 已经是导出名，不需要别名。
// 直接用包限定名供 root 文件调用：
type (
	AppendPacketFields     = storepkg.AppendPacketFields
	MQTTClientRecordFields = storepkg.MQTTClientRecordFields
)

// 其它额外导出类型的别名（旧的小写形式仍被根目录文件直接使用）。
type (
	runtimeSettingsSnapshot     = storepkg.RuntimeSettingsSnapshot
	mqttForwarderConfig         = storepkg.MQTTForwarderConfig
	botDirectMessageListOptions = storepkg.BotDirectMessageListOptions
	botMessageListOptions       = storepkg.BotMessageListOptions
	botDirectConversation       = storepkg.BotDirectConversation
	signDayCount                = storepkg.SignDayCount
)

var errBlockingAlreadyExists = storepkg.ErrBlockingAlreadyExists
var errBotNodeAlreadyExists = storepkg.ErrBotNodeAlreadyExists
var (
	errMapTileSourceAlreadyExists        = storepkg.ErrMapTileSourceAlreadyExists
	errMapTileSourceCannotDeleteDefault  = storepkg.ErrMapTileSourceCannotDeleteDefault
	errMapTileSourceCannotDisableDefault = storepkg.ErrMapTileSourceCannotDisableDefault
	errMapTileSourceDefaultMustBeEnabled = storepkg.ErrMapTileSourceDefaultMustBeEnabled
)

func mapTileSourceHash(urlTemplate string) string {
	return storepkg.MapTileSourceHash(urlTemplate)
}

const (
	forbiddenWordMatchContains             = storepkg.ForbiddenWordMatchContains
	runtimeSettingAllowEncryptedForwarding = storepkg.RuntimeSettingAllowEncryptedForwarding
	runtimeSettingLLMQueueEnabled          = storepkg.RuntimeSettingLLMQueueEnabled
	runtimeSettingLLMQueueIncludeChannel   = storepkg.RuntimeSettingLLMQueueIncludeChannel

	botDefaultPSK                     = storepkg.BotDefaultPSK
	botMessageTypeChannel             = storepkg.BotMessageTypeChannel
	botMessageTypeDirect              = storepkg.BotMessageTypeDirect
	botMessageStatusPending           = storepkg.BotMessageStatusPending
	botMessageStatusPublished         = storepkg.BotMessageStatusPublished
	botMessageStatusFailed            = storepkg.BotMessageStatusFailed
	botDirectMessageDirectionInbound  = storepkg.BotDirectMessageDirectionInbound
	botDirectMessageDirectionOutbound = storepkg.BotDirectMessageDirectionOutbound
	botDefaultTopicPrefix             = storepkg.BotDefaultTopicPrefix

	mqttForwardDirectionSourceToTarget = storepkg.MQTTForwardDirectionSourceToTarget
	mqttForwardDirectionBidirectional  = storepkg.MQTTForwardDirectionBidirectional
)

const botDefaultNodeInfoBroadcastSeconds = storepkg.BotDefaultNodeInfoBroadcastSeconds

func validateBotNodeNum(n int64) error { return storepkg.ValidateBotNodeNum(n) }

var errUserAlreadyExists = storepkg.ErrUserAlreadyExists

var (
	errMQTTForwarderAlreadyExists    = storepkg.ErrMQTTForwarderAlreadyExists
	errMQTTForwardTopicAlreadyExists = storepkg.ErrMQTTForwardTopicAlreadyExists
)

func nullableString(v any) *string      { return storepkg.NullableString(v) }
func nullableStringValue(v any) *string { return storepkg.NullableStringValue(v) }
func decodeBotPublicKey(row botNodeRecord) ([]byte, error) {
	return storepkg.DecodeBotPublicKey(row)
}

// LLM 消息队列状态字符串常量。
const (
	llmMessageStatusPending    = storepkg.LLMMessageStatusPending
	llmMessageStatusProcessing = storepkg.LLMMessageStatusProcessing
	llmMessageStatusProcessed  = storepkg.LLMMessageStatusProcessed
	llmMessageStatusError      = storepkg.LLMMessageStatusError
)

const defaultHelpMarkdown = storepkg.DefaultHelpMarkdown

// 旧名 llmMessageDTO 现在通过 store 提供。
func llmMessageDTO(row llmMessageQueueRecord) map[string]any {
	return storepkg.LLMMessageDTO(row)
}

// ---- 工厂函数包装 ----

func openStore(cfg databaseConfig) (*store, error) { return storepkg.OpenStore(cfg) }
func normalizeListOptions(o listOptions) listOptions {
	return storepkg.NormalizeListOptions(o)
}
func normalizeMapReportViewportOptions(o mapReportViewportOptions) mapReportViewportOptions {
	return storepkg.NormalizeMapReportViewportOptions(o)
}

// newDBWriteQueue 在新包里更名，提供旧名字避免改 main.go。
func newDBWriteQueue(s *store) *dbWriteQueue { return storepkg.NewWriteQueue(s) }
