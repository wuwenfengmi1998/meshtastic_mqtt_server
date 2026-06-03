import type { HealthStatus, ListResponse, MapReport, NodeInfo, PositionRecord, TextMessage } from './types'

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

export function getNodeInfo(limit = 500, offset = 0): Promise<ListResponse<NodeInfo>> {
  return getJSON<ListResponse<NodeInfo>>(`/api/nodeinfo?limit=${limit}&offset=${offset}`)
}

export function getMapReports(limit = 500, offset = 0): Promise<ListResponse<MapReport>> {
  return getJSON<ListResponse<MapReport>>(`/api/map-reports?limit=${limit}&offset=${offset}`)
}

export function getTextMessages(limit = 100, offset = 0): Promise<ListResponse<TextMessage>> {
  return getJSON<ListResponse<TextMessage>>(`/api/text-messages?limit=${limit}&offset=${offset}`)
}

export function getPositions(limit = 500): Promise<ListResponse<PositionRecord>> {
  return getJSON<ListResponse<PositionRecord>>(`/api/positions?limit=${limit}`)
}
