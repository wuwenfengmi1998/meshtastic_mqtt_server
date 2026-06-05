import { getDefaultMapSource, getEnabledMapSources } from './api'
import type { PublicMapTileSource } from './types'

export const fallbackMapSource: PublicMapTileSource = {
  id: 0,
  name: 'OpenStreetMap Japan',
  url_template: 'https://tile.openstreetmap.jp/{z}/{x}/{y}.png',
  attribution: '&copy; OpenStreetMap contributors',
  max_zoom: 19,
}

export async function loadDefaultMapSource(): Promise<PublicMapTileSource> {
  try {
    const response = await getDefaultMapSource()
    return response.item
  } catch {
    return fallbackMapSource
  }
}

export async function loadEnabledMapSources(): Promise<PublicMapTileSource[]> {
  try {
    const response = await getEnabledMapSources()
    return response.items.length > 0 ? response.items : [fallbackMapSource]
  } catch {
    return [fallbackMapSource]
  }
}
