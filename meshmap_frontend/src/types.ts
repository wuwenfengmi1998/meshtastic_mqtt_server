export interface ListResponse<T> {
  items: T[]
  limit: number
  offset: number
  total?: number
}

export interface HealthStatus {
  status: string
  database: string
}

export interface HelpContent {
  id: number | null
  markdown: string
  html: string
  created_by: string
  created_at: string | null
}

export interface HelpContentResponse {
  item: HelpContent
}

export interface HelpContentPayload {
  markdown: string
}

export interface HelpPreviewResponse {
  html: string
}

export interface NodeInfo {
  node_id: string
  node_num: number
  user_id: string | null
  long_name: string | null
  short_name: string | null
  hw_model: string | null
  role: string | null
  is_licensed: boolean | null
  public_key: string | null
  updated_at: string
  content_json: string
}

export interface MapReport {
  node_id: string
  node_num: number
  long_name: string | null
  short_name: string | null
  hw_model: string | null
  role: string | null
  firmware_version: string | null
  region: string | null
  modem_preset: string | null
  latitude: number | null
  longitude: number | null
  altitude: number | null
  position_precision: number | null
  num_online_local_nodes: number | null
  has_opted_report_location: boolean | null
  updated_at: string
  content_json: string
}

export interface MapBoundsQuery {
  min_lat: number
  max_lat: number
  min_lng: number
  max_lng: number
}

export interface MapBoundsChangePayload {
  bounds: MapBoundsQuery
  zoom: number
}

export interface PublicMapTileSource {
  id: number
  name: string
  url_template: string
  attribution: string
  max_zoom: number
}

export interface MapTileSource extends PublicMapTileSource {
  enabled: boolean
  is_default: boolean
  proxy_enabled: boolean
  created_at: string
  updated_at: string
}

export interface MapTileSourcePayload {
  name: string
  url_template: string
  attribution: string
  max_zoom: number
  enabled: boolean
  is_default: boolean
  proxy_enabled: boolean
}

export interface MapTileSourceResponse {
  item: MapTileSource
}

export interface PublicMapTileSourceResponse {
  item: PublicMapTileSource
}

export interface PublicMapTileSourcesResponse {
  items: PublicMapTileSource[]
}

export interface MapViewportPoint extends MapReport {
  type: 'point'
}

export interface MapViewportCluster {
  type: 'cluster'
  cluster_id: string
  latitude: number
  longitude: number
  count: number
}

export type MapViewportItem = MapViewportPoint | MapViewportCluster

export interface MapViewportResponse {
  mode: 'points' | 'clusters'
  items: MapViewportItem[]
  total: number
  limit: number
  zoom: number
}

export interface TextMessage {
  id: number
  from_id: string
  from_num: number
  packet_id: number | null
  text: string | null
  topic: string
  channel_id: string | null
  created_at: string
  mqtt_remote_host: string | null
  content_json: string
}

export interface SignRecord {
  id: number
  node_id: string
  long_name: string | null
  short_name: string | null
  sign_text: string
  sign_time: string
}

export interface SignDayCount {
  date: string
  count: number
}

export interface SignRecordPayload {
  node_id: string
  long_name: string
  short_name: string
  sign_text: string
  sign_time?: string
}

// 机器人 PKI 私聊（bot_direct_messages 表）。direction 区分本地 bot 视角的进出方向。
export interface BotDirectMessage {
  id: number
  bot_id: number
  bot_node_id: string
  bot_node_num: number
  peer_node_id: string
  peer_node_num: number
  direction: 'inbound' | 'outbound'
  topic: string
  packet_id: number
  text: string
  payload_len: number
  pki_encrypted: boolean
  want_ack: boolean
  gateway_id: string | null
  status: string
  error: string
  bot_message_id: number | null
  created_by: string | null
  published_at: string | null
  received_at: string | null
  read_at: string | null
  created_at: string
}

// 会话摘要：每个 (bot, peer) 一条，给侧边栏使用。
export interface BotDirectConversation {
  bot_id: number
  peer_node_id: string
  peer_node_num: number
  last_message_at: string
  last_text: string
  last_direction: 'inbound' | 'outbound' | string
  unread_count: number
  total_count: number
}

export interface BotDirectConversationsResponse {
  items: BotDirectConversation[]
  limit: number
  offset: number
  unread_total: number
}

export interface PositionRecord {
  id: number
  from_id: string
  from_num: number
  latitude: number | null
  longitude: number | null
  altitude: number | null
  created_at: string
  content_json: string
}

export interface TelemetryRecord {
  id: number
  from_id: string
  from_num: number
  telemetry_type: string | null
  metrics_json: string | null
  created_at: string
  content_json: string
}

export interface MapNode {
  type: 'node'
  node_id: string
  label: string
  latitude: number
  longitude: number
  altitude: number | null
  source: 'map_report' | 'position'
  updated_at: string
  nodeinfo: NodeInfo | null
  map_report: MapReport | null
  latest_position: PositionRecord | null
}

export interface MapClusterNode {
  type: 'cluster'
  cluster_id: string
  latitude: number
  longitude: number
  count: number
}

export type MapRenderable = MapNode | MapClusterNode

export type NodeInfoById = Record<string, NodeInfo>

export interface AdminUser {
  username: string
  role: string
}

export interface AdminLoginResponse {
  user: AdminUser
}

export interface AdminManagedUser {
  id: number
  username: string
  role: string
  created_at: string
  updated_at: string
}

export interface AdminUsersResponse {
  items: AdminManagedUser[]
}

export interface AdminManagedUserResponse {
  user: AdminManagedUser
}

export interface DiscardDetails {
  id: number
  topic: string
  error: string
  payload_len: number
  raw_base64: string
  mqtt_client_id: string | null
  mqtt_username: string | null
  mqtt_listener: string | null
  mqtt_remote_addr: string | null
  mqtt_remote_host: string | null
  mqtt_remote_port: string | null
  created_at: string
  content_json: string
}

export interface AdminLoginLog {
  id: number
  username: string
  user_id: number | null
  success: boolean
  reason: string
  remote_addr: string
  remote_host: string
  user_agent: string
  created_at: string
}

export interface AdminLoginLogsResponse {
  items: AdminLoginLog[]
  limit: number
  offset: number
}

export interface AdminMqttClient {
  client_id: string
  username: string
  listener: string
  remote_addr: string
  remote_host: string
  remote_port: string
}

export interface AdminRuntimeSettings {
  allow_encrypted_forwarding: boolean
}

export interface AdminRuntimeSettingsPayload {
  allow_encrypted_forwarding: boolean
}

export interface AdminRuntimeSettingsResponse {
  item: AdminRuntimeSettings
}

export interface AdminMqttStatus {
  running: boolean
  address: string
  tls: boolean
  version: string
  started: number
  uptime: number
  bytes_received: number
  bytes_sent: number
  clients_connected: number
  clients_disconnected: number
  clients_maximum: number
  clients_total: number
  messages_received: number
  messages_sent: number
  messages_dropped: number
  db_write_queue_length: number
  retained: number
  inflight: number
  inflight_dropped: number
  subscriptions: number
  packets_received: number
  packets_sent: number
  clients: AdminMqttClient[]
}

export interface NodeBlockingRule {
  id: number
  node_id: string
  node_num: number | null
  reason: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface NodeBlockingRulePayload {
  node_id: string
  node_num: number | null
  reason: string
  enabled: boolean
}

export interface IPBlockingRule {
  id: number
  ip_value: string
  reason: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface IPBlockingRulePayload {
  ip_value: string
  reason: string
  enabled: boolean
}

export interface ForbiddenWordBlockingRule {
  id: number
  word: string
  match_type: string
  case_sensitive: boolean
  reason: string
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface ForbiddenWordBlockingRulePayload {
  word: string
  match_type: string
  case_sensitive: boolean
  reason: string
  enabled: boolean
}

export interface BlockingRuleResponse<T> {
  item: T
}

export type MQTTForwardDirection = 'source_to_target' | 'bidirectional'

export interface MQTTForwarder {
  id: number
  name: string
  enabled: boolean
  source_host: string
  source_port: number
  source_username: string
  source_password_set: boolean
  source_client_id: string
  source_tls: boolean
  target_host: string
  target_port: number
  target_username: string
  target_password_set: boolean
  target_client_id: string
  target_tls: boolean
  created_at: string
  updated_at: string
}

export interface MQTTForwarderPayload {
  name: string
  enabled: boolean
  source_host: string
  source_port: number
  source_username: string
  source_password?: string
  source_password_clear?: boolean
  source_client_id: string
  source_tls: boolean
  target_host: string
  target_port: number
  target_username: string
  target_password?: string
  target_password_clear?: boolean
  target_client_id: string
  target_tls: boolean
}

export interface MQTTForwardTopic {
  id: number
  forwarder_id: number
  topic: string
  enabled: boolean
  direction: MQTTForwardDirection
  source_prefix: string
  target_prefix: string
  qos: number
  retain: boolean
  created_at: string
  updated_at: string
}

export interface MQTTForwardTopicPayload {
  topic: string
  enabled: boolean
  direction: MQTTForwardDirection
  source_prefix: string
  target_prefix: string
  qos: number
  retain: boolean
}

export interface MQTTForwardRuntimeStatus {
  forwarder_id: number
  running: boolean
  source_connected: boolean
  target_connected: boolean
  last_error: string
  started_at: string | null
  messages_forwarded: number
  messages_dropped: number
}

export interface MQTTForwardMutationResponse<T> {
  item: T
}

export interface MQTTForwardStatusResponse {
  items: MQTTForwardRuntimeStatus[]
}

export type BotMessageType = 'channel' | 'direct'
export type BotMessageStatus = 'pending' | 'published' | 'failed'

export interface BotNode {
  id: number
  node_id: string
  node_num: number
  long_name: string
  short_name: string
  enabled: boolean
  default_channel_id: string
  topic_prefix: string
  psk: string
  public_key: string
  private_key_set: boolean
  nodeinfo_broadcast_enabled: boolean
  nodeinfo_broadcast_interval_seconds: number
  last_nodeinfo_broadcast_at: string | null
  created_at: string
  updated_at: string
}

export interface BotNodePayload {
  node_num?: number | null
  long_name: string
  short_name: string
  enabled: boolean
  default_channel_id: string
  topic_prefix?: string
  psk?: string
  nodeinfo_broadcast_enabled?: boolean
  nodeinfo_broadcast_interval_seconds?: number
}

export interface BotNodeMutationResponse {
  item: BotNode
}

export interface BotMessage {
  id: number
  bot_id: number
  bot_node_id: string
  bot_node_num: number
  message_type: BotMessageType
  channel_id: string
  to_node_id: string | null
  to_node_num: number | null
  topic: string
  packet_id: number
  text: string
  payload_len: number
  encrypted: boolean
  status: BotMessageStatus
  error: string
  published_at: string | null
  created_by: string
  created_at: string
}

export interface BotSendMessagePayload {
  bot_id: number
  message_type: BotMessageType
  channel_id: string
  to_node_id?: string
  to_node_num?: number | null
  text: string
}

export interface BotMessageMutationResponse {
  item: BotMessage
  error?: string
}
