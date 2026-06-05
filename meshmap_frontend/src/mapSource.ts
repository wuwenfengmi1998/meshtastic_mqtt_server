import { getDefaultMapSource } from './api'
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
