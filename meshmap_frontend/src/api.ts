import type { HealthStatus, ListResponse, NodeInfoMap, PositionRecord, TextMessage } from './types'

async function getJSON<T>(path: string): Promise<T> {
  const response = await fetch(path)
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`)
  }
  return response.json() as Promise<T>
}

export function getHealth(): Promise<HealthStatus> {
  return getJSON<HealthStatus>('/api/health')
}

export function getNodes(limit = 500, offset = 0): Promise<ListResponse<NodeInfoMap>> {
  return getJSON<ListResponse<NodeInfoMap>>(`/api/nodes?limit=${limit}&offset=${offset}`)
}

export function getTextMessages(limit = 100): Promise<ListResponse<TextMessage>> {
  return getJSON<ListResponse<TextMessage>>(`/api/text-messages?limit=${limit}`)
}

export function getPositions(limit = 500): Promise<ListResponse<PositionRecord>> {
  return getJSON<ListResponse<PositionRecord>>(`/api/positions?limit=${limit}`)
}
