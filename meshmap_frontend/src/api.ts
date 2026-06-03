import type {
  AdminLoginLogsResponse,
  AdminLoginResponse,
  AdminManagedUserResponse,
  AdminMqttStatus,
  AdminUsersResponse,
  HealthStatus,
  ListResponse,
  MapReport,
  NodeInfo,
  PositionRecord,
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
  return getJSON<ListResponse<NodeInfo>>(`/api/nodeinfo?limit=${limit}&offset=${offset}`)
}

export function getMapReports(limit = 500, offset = 0): Promise<ListResponse<MapReport>> {
  return getJSON<ListResponse<MapReport>>(`/api/map-reports?limit=${limit}&offset=${offset}`)
}

export function getTextMessages(limit = 100, offset = 0): Promise<ListResponse<TextMessage>> {
  return getJSON<ListResponse<TextMessage>>(`/api/text-messages?limit=${limit}&offset=${offset}`)
}

export function deleteTextMessage(id: number): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/text-messages/${id}`)
}

export function deleteNode(nodeId: string): Promise<{ status: string }> {
  return deleteJSON<{ status: string }>(`/api/admin/nodes/${encodeURIComponent(nodeId)}`)
}

export function getPositions(limit = 500): Promise<ListResponse<PositionRecord>> {
  return getJSON<ListResponse<PositionRecord>>(`/api/positions?limit=${limit}`)
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
