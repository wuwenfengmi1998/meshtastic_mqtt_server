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

export interface TextMessage {
  id: number
  from_id: string
  from_num: number
  text: string | null
  topic: string
  created_at: string
  mqtt_remote_host: string | null
  content_json: string
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

export interface MapNode {
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

export type NodeInfoById = Record<string, NodeInfo>
