import type {
  AdminLoginLogsResponse,
  AdminLoginResponse,
  AdminManagedUserResponse,
  AdminMqttStatus,
  AdminRuntimeSettingsPayload,
  AdminRuntimeSettingsResponse,
  AdminUsersResponse,
  BlockingRuleResponse,
  DiscardDetails,
  ForbiddenWordBlockingRule,
  ForbiddenWordBlockingRulePayload,
  HealthStatus,
  HelpContentResponse,
  HelpPreviewResponse,
  IPBlockingRule,
  IPBlockingRulePayload,
  ListResponse,
  MapBoundsQuery,
  MapReport,
  MapTileSource,
  MapTileSourcePayload,
  MapTileSourceResponse,
  MapViewportResponse,
  MQTTForwarder,
  MQTTForwarderPayload,
  MQTTForwardMutationResponse,
  MQTTForwardStatusResponse,
  MQTTForwardTopic,
  MQTTForwardTopicPayload,
  NodeBlockingRule,
  NodeBlockingRulePayload,
  NodeInfo,
  PositionRecord,
  PublicMapTileSourceResponse,
  TelemetryRecord,
  TextMessage,
} from './types'

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, { credentials: 'same-origin', ...init })
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`
    try {
      const data = (await response.json()) as { error?: string }
      if (data.error) {
        message = data.error
      }
    } catch {
      // Keep the HTTP status message when the response is not JSON.
    }
    throw new Error(message)
  }
  return response.json() as Promise<T>
}

function listPath(path: string, limit: number, offset: number, nodeId = ''): string {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (nodeId) {
    params.set('node_id', nodeId)
  }
  return `${path}?${params.toString()}`
}

function getJSON<T>(path: string): Promise<T> {
  return requestJSON<T>(path)
}

function postJSON<T>(path: string, body?: unknown): Promise<T> {
  return requestJSON<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body == null ? undefined : JSON.stringify(body),
  })
}

function putJSON<T>(path: string, body?: unknown): Promise<T> {
  return requestJSON<T>(path, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: body == null ? undefined : JSON.stringify(body),
  })
}

function deleteJSON<T>(path: string): Promise<T> {
  return requestJSON<T>(path, { method: 'DELETE' })
}

export function getHealth(): Promise<HealthStatus> {
  return getJSON<HealthStatus>('/api/health')
}

export function getHelpContent(): Promise<HelpContentResponse> {
  return getJSON<HelpContentResponse>('/api/help')
}

export function getNodeInfo(limit = 500, offset = 0): Promise<ListResponse<NodeInfo>> {
  return getJSON<ListResponse<NodeInfo>>(listPath('/api/nodeinfo', limit, offset))
}

export function getNodeInfoById(nodeId: string): Promise<NodeInfo> {
  return getJSON<NodeInfo>(`/api/nodeinfo/${encodeURIComponent(nodeId)}`)
}

export function getMapReports(limit = 500, offset = 0, bounds?: MapBoundsQuery): Promise<ListResponse<MapReport>> {
  const params = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  if (bounds) {
    params.set('min_lat', String(bounds.min_lat))
    params.set('max_lat', String(bounds.max_lat))
    params.set('min_lng', String(bounds.min_lng))
    params.set('max_lng', String(bounds.max_lng))
  }
  return getJSON<ListResponse<MapReport>>(`/api/map-reports?${params.toString()}`)
}

export function getMapReportById(nodeId: string): Promise<MapReport> {
  return getJSON<MapReport>(`/api/map-reports/${encodeURIComponent(nodeId)}`)
}

export function getMapReportViewport(bounds: MapBoundsQuery, zoom: number, limit = 1000): Promise<MapViewportResponse> {
  const params = new URLSearchParams({
    min_lat: String(bounds.min_lat),
    max_lat: String(bounds.max_lat),
    min_lng: String(bounds.min_lng),
    max_lng: String(bounds.max_lng),
    zoom: String(zoom),
    limit: String(limit),
  })
  return getJSON<MapViewportResponse>(`/api/map-reports/viewport?${params.toString()}`)
}

export function getDefaultMapSource(): Promise<PublicMapTileSourceResponse> {
  return getJSON<PublicMapTileSourceResponse>('/api/map-source/default')
}

export function getTextMessages(limit = 100, offset = 0, nodeId = ''): Promise<ListResponse<TextMessage>> {
  return getJSON<ListResponse<TextMessage>>(listPath('/api/text-messages', limit, offset, nodeId))
}

export function deleteTextMessage(id: number): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/text-messages/${id}`)
}

export function deleteNode(nodeId: string): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/nodes/${encodeURIComponent(nodeId)}`)
}

export function getPositions(limit = 500, offset = 0, nodeId = ''): Promise<ListResponse<PositionRecord>> {
  return getJSON<ListResponse<PositionRecord>>(listPath('/api/positions', limit, offset, nodeId))
}

export function getDiscardDetails(limit = 100, offset = 0): Promise<ListResponse<DiscardDetails>> {
  return getJSON<ListResponse<DiscardDetails>>(listPath('/api/discard-details', limit, offset))
}

export function getTelemetry(limit = 500, offset = 0, nodeId = ''): Promise<ListResponse<TelemetryRecord>> {
  return getJSON<ListResponse<TelemetryRecord>>(listPath('/api/telemetry', limit, offset, nodeId))
}

export function adminLogin(username: string, password: string): Promise<AdminLoginResponse> {
  return postJSON<AdminLoginResponse>('/api/admin/login', { username, password })
}

export function adminLogout(): Promise<{ status: string }> {
  return postJSON<{ status: string }>('/api/admin/logout')
}

export function getAdminMe(): Promise<AdminLoginResponse> {
  return getJSON<AdminLoginResponse>('/api/admin/me')
}

export function getAdminMqttStatus(): Promise<AdminMqttStatus> {
  return getJSON<AdminMqttStatus>('/api/admin/mqtt/status')
}

export function getAdminRuntimeSettings(): Promise<AdminRuntimeSettingsResponse> {
  return getJSON<AdminRuntimeSettingsResponse>('/api/admin/runtime-settings')
}

export function updateAdminRuntimeSettings(payload: AdminRuntimeSettingsPayload): Promise<AdminRuntimeSettingsResponse> {
  return putJSON<AdminRuntimeSettingsResponse>('/api/admin/runtime-settings', payload)
}

export function getAdminHelpContent(): Promise<HelpContentResponse> {
  return getJSON<HelpContentResponse>('/api/admin/help')
}

export function saveAdminHelpContent(markdown: string): Promise<HelpContentResponse> {
  return postJSON<HelpContentResponse>('/api/admin/help', { markdown })
}

export function previewAdminHelpContent(markdown: string): Promise<HelpPreviewResponse> {
  return postJSON<HelpPreviewResponse>('/api/admin/help/preview', { markdown })
}

export function getAdminUsers(): Promise<AdminUsersResponse> {
  return getJSON<AdminUsersResponse>('/api/admin/users')
}

export function createAdminUser(username: string, password: string): Promise<AdminManagedUserResponse> {
  return postJSON<AdminManagedUserResponse>('/api/admin/users', { username, password })
}

export function updateAdminUserPassword(id: number, password: string): Promise<AdminManagedUserResponse> {
  return putJSON<AdminManagedUserResponse>(`/api/admin/users/${id}/password`, { password })
}

export function getAdminLoginLogs(limit = 100, offset = 0): Promise<AdminLoginLogsResponse> {
  return getJSON<AdminLoginLogsResponse>(`/api/admin/log/login?limit=${limit}&offset=${offset}`)
}

export function getAdminMapSources(limit = 100, offset = 0): Promise<ListResponse<MapTileSource>> {
  return getJSON<ListResponse<MapTileSource>>(listPath('/api/admin/map-source', limit, offset))
}

export function createAdminMapSource(payload: MapTileSourcePayload): Promise<MapTileSourceResponse> {
  return postJSON<MapTileSourceResponse>('/api/admin/map-source', payload)
}

export function updateAdminMapSource(id: number, payload: MapTileSourcePayload): Promise<MapTileSourceResponse> {
  return putJSON<MapTileSourceResponse>(`/api/admin/map-source/${id}`, payload)
}

export function deleteAdminMapSource(id: number): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/map-source/${id}`)
}

export function setDefaultAdminMapSource(id: number): Promise<MapTileSourceResponse> {
  return postJSON<MapTileSourceResponse>(`/api/admin/map-source/${id}/default`)
}

export function getNodeBlockingRules(limit = 100, offset = 0): Promise<ListResponse<NodeBlockingRule>> {
  return getJSON<ListResponse<NodeBlockingRule>>(listPath('/api/admin/blocking/nodes', limit, offset))
}

export function createNodeBlockingRule(payload: NodeBlockingRulePayload): Promise<BlockingRuleResponse<NodeBlockingRule>> {
  return postJSON<BlockingRuleResponse<NodeBlockingRule>>('/api/admin/blocking/nodes', payload)
}

export function updateNodeBlockingRule(id: number, payload: NodeBlockingRulePayload): Promise<BlockingRuleResponse<NodeBlockingRule>> {
  return putJSON<BlockingRuleResponse<NodeBlockingRule>>(`/api/admin/blocking/nodes/${id}`, payload)
}

export function deleteNodeBlockingRule(id: number): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/blocking/nodes/${id}`)
}

export function getIPBlockingRules(limit = 100, offset = 0): Promise<ListResponse<IPBlockingRule>> {
  return getJSON<ListResponse<IPBlockingRule>>(listPath('/api/admin/blocking/ips', limit, offset))
}

export function createIPBlockingRule(payload: IPBlockingRulePayload): Promise<BlockingRuleResponse<IPBlockingRule>> {
  return postJSON<BlockingRuleResponse<IPBlockingRule>>('/api/admin/blocking/ips', payload)
}

export function updateIPBlockingRule(id: number, payload: IPBlockingRulePayload): Promise<BlockingRuleResponse<IPBlockingRule>> {
  return putJSON<BlockingRuleResponse<IPBlockingRule>>(`/api/admin/blocking/ips/${id}`, payload)
}

export function deleteIPBlockingRule(id: number): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/blocking/ips/${id}`)
}

export function getForbiddenWordBlockingRules(limit = 100, offset = 0): Promise<ListResponse<ForbiddenWordBlockingRule>> {
  return getJSON<ListResponse<ForbiddenWordBlockingRule>>(listPath('/api/admin/blocking/words', limit, offset))
}

export function createForbiddenWordBlockingRule(payload: ForbiddenWordBlockingRulePayload): Promise<BlockingRuleResponse<ForbiddenWordBlockingRule>> {
  return postJSON<BlockingRuleResponse<ForbiddenWordBlockingRule>>('/api/admin/blocking/words', payload)
}

export function updateForbiddenWordBlockingRule(id: number, payload: ForbiddenWordBlockingRulePayload): Promise<BlockingRuleResponse<ForbiddenWordBlockingRule>> {
  return putJSON<BlockingRuleResponse<ForbiddenWordBlockingRule>>(`/api/admin/blocking/words/${id}`, payload)
}

export function deleteForbiddenWordBlockingRule(id: number): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/blocking/words/${id}`)
}

export function getMQTTForwarders(limit = 100, offset = 0): Promise<ListResponse<MQTTForwarder>> {
  return getJSON<ListResponse<MQTTForwarder>>(listPath('/api/admin/mqtt-forward/forwarders', limit, offset))
}

export function createMQTTForwarder(payload: MQTTForwarderPayload): Promise<MQTTForwardMutationResponse<MQTTForwarder>> {
  return postJSON<MQTTForwardMutationResponse<MQTTForwarder>>('/api/admin/mqtt-forward/forwarders', payload)
}

export function updateMQTTForwarder(id: number, payload: MQTTForwarderPayload): Promise<MQTTForwardMutationResponse<MQTTForwarder>> {
  return putJSON<MQTTForwardMutationResponse<MQTTForwarder>>(`/api/admin/mqtt-forward/forwarders/${id}`, payload)
}

export function deleteMQTTForwarder(id: number): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/mqtt-forward/forwarders/${id}`)
}

export function restartMQTTForwarder(id: number): Promise<{ status: string }> {
  return postJSON<{ status: string }>(`/api/admin/mqtt-forward/forwarders/${id}/restart`)
}

export function getMQTTForwardTopics(forwarderId: number, limit = 100, offset = 0): Promise<ListResponse<MQTTForwardTopic>> {
  return getJSON<ListResponse<MQTTForwardTopic>>(listPath(`/api/admin/mqtt-forward/forwarders/${forwarderId}/topics`, limit, offset))
}

export function createMQTTForwardTopic(forwarderId: number, payload: MQTTForwardTopicPayload): Promise<MQTTForwardMutationResponse<MQTTForwardTopic>> {
  return postJSON<MQTTForwardMutationResponse<MQTTForwardTopic>>(`/api/admin/mqtt-forward/forwarders/${forwarderId}/topics`, payload)
}

export function updateMQTTForwardTopic(id: number, payload: MQTTForwardTopicPayload): Promise<MQTTForwardMutationResponse<MQTTForwardTopic>> {
  return putJSON<MQTTForwardMutationResponse<MQTTForwardTopic>>(`/api/admin/mqtt-forward/topics/${id}`, payload)
}

export function deleteMQTTForwardTopic(id: number): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/mqtt-forward/topics/${id}`)
}

export function getMQTTForwardStatus(): Promise<MQTTForwardStatusResponse> {
  return getJSON<MQTTForwardStatusResponse>('/api/admin/mqtt-forward/status')
}
