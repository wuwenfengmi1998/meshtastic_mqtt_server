export interface ListResponse<T> {
  items: T[]
  limit: number
  offset: number
}

export interface HealthStatus {
  status: string
  database: string
}

export interface NodeInfoMap {
  node_id: string
  node_num: number
  latest_type: string
  long_name: string | null
  short_name: string | null
  hw_model: string | null
  role: string | null
  latitude: number | null
  longitude: number | null
  altitude: number | null
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
