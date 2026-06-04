import type {
  AdminLoginLogsResponse,
  AdminLoginResponse,
  AdminManagedUserResponse,
  AdminMqttStatus,
  AdminUsersResponse,
  BlockingRuleResponse,
  DiscardDetails,
  ForbiddenWordBlockingRule,
  ForbiddenWordBlockingRulePayload,
  HealthStatus,
  IPBlockingRule,
  IPBlockingRulePayload,
  ListResponse,
  MapReport,
  NodeBlockingRule,
  NodeBlockingRulePayload,
  NodeInfo,
  PositionRecord,
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

export function getNodeInfo(limit = 500, offset = 0): Promise<ListResponse<NodeInfo>> {
  return getJSON<ListResponse<NodeInfo>>(listPath('/api/nodeinfo', limit, offset))
}

export function getNodeInfoById(nodeId: string): Promise<NodeInfo> {
  return getJSON<NodeInfo>(`/api/nodeinfo/${encodeURIComponent(nodeId)}`)
}

export function getMapReports(limit = 500, offset = 0): Promise<ListResponse<MapReport>> {
  return getJSON<ListResponse<MapReport>>(listPath('/api/map-reports', limit, offset))
}

export function getMapReportById(nodeId: string): Promise<MapReport> {
  return getJSON<MapReport>(`/api/map-reports/${encodeURIComponent(nodeId)}`)
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
