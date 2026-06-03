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

export function getNodes(): Promise<ListResponse<NodeInfoMap>> {
  return getJSON<ListResponse<NodeInfoMap>>('/api/nodes?limit=100')
}

export function getTextMessages(): Promise<ListResponse<TextMessage>> {
  return getJSON<ListResponse<TextMessage>>('/api/text-messages?limit=20')
}

export function getPositions(): Promise<ListResponse<PositionRecord>> {
  return getJSON<ListResponse<PositionRecord>>('/api/positions?limit=200')
}
