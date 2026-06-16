<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { fallbackMapSource } from '../mapSource'
import type { PositionRecord, PublicMapTileSource } from '../types'

const props = withDefaults(defineProps<{
  positions: PositionRecord[]
  mapSource?: PublicMapTileSource
  mapSources?: PublicMapTileSource[]
}>(), {
  mapSource: () => fallbackMapSource,
  mapSources: () => [fallbackMapSource],
})

const emit = defineEmits<{
  'map-source-change': [sourceId: number]
}>()

const mapEl = ref<HTMLElement | null>(null)
let map: L.Map | null = null
let tileLayer: L.TileLayer | null = null
let layer: L.LayerGroup | null = null

function selectMapSource(sourceId: number) {
  emit('map-source-change', sourceId)
}

function applyTileLayer() {
  if (!map) {
    return
  }
  if (tileLayer) {
    tileLayer.remove()
  }
  tileLayer = L.tileLayer(props.mapSource.url_template, {
    maxZoom: props.mapSource.max_zoom || fallbackMapSource.max_zoom,
    attribution: props.mapSource.attribution || fallbackMapSource.attribution,
  }).addTo(map)
}

function pointStyle(isStart: boolean, isEnd: boolean): L.CircleMarkerOptions {
  if (isStart) {
    return { radius: 7, color: '#7f9183', fillColor: '#9aaa95', fillOpacity: 0.88, weight: 2 }
  }
  if (isEnd) {
    return { radius: 7, color: '#b4877f', fillColor: '#c59b93', fillOpacity: 0.88, weight: 2 }
  }
  return { radius: 3, color: '#7d8f9a', fillColor: '#9ab3c2', fillOpacity: 0.72, weight: 1 }
}

function buildPositionPopupHTML(position: PositionRecord, isStart: boolean, isEnd: boolean): string {
  const title = isStart ? '起点' : isEnd ? '终点' : '轨迹点'
  const latitude = position.latitude == null ? '-' : position.latitude.toFixed(6)
  const longitude = position.longitude == null ? '-' : position.longitude.toFixed(6)
  const altitude = position.altitude == null ? '-' : `${position.altitude} m`
  const time = new Date(position.created_at).toLocaleString()
  return `
    <div class="trajectory-popup">
      <strong>${escapeHTML(title)}</strong>
      <dl>
        <div><dt>纬度</dt><dd>${latitude}</dd></div>
        <div><dt>经度</dt><dd>${longitude}</dd></div>
        <div><dt>海拔</dt><dd>${altitude}</dd></div>
        <div><dt>时间</dt><dd>${escapeHTML(time)}</dd></div>
      </dl>
    </div>
  `
}

function escapeHTML(value: string): string {
  return value.replace(/[&<>'"]/g, (char) => {
    const entities: Record<string, string> = {
      '&': '&amp;',
      '<': '&lt;',
      '>': '&gt;',
      "'": '&#39;',
      '"': '&quot;',
    }
    return entities[char]
  })
}

function renderTrajectory() {
  if (!map || !layer) {
    return
  }
  const trajectoryLayer = layer
  trajectoryLayer.clearLayers()
  const positions = [...props.positions]
    .filter((position) => position.latitude != null && position.longitude != null)
    .reverse()
  const points = positions.map((position) => [position.latitude as number, position.longitude as number] as L.LatLngTuple)

  if (points.length === 0) {
    map.setView([0, 0], 2)
    return
  }

  if (points.length > 1) {
    L.polyline(points, { color: '#7d8f9a', weight: 4, opacity: 0.78 }).addTo(trajectoryLayer)
  }

  positions.forEach((position, index) => {
    const isStart = index === 0
    const isEnd = index === positions.length - 1
    L.circleMarker(points[index], pointStyle(isStart, isEnd))
      .bindPopup(buildPositionPopupHTML(position, isStart, isEnd), { maxWidth: 280, className: 'trajectory-point-popup' })
      .addTo(trajectoryLayer)
  })
  map.fitBounds(L.latLngBounds(points), { padding: [24, 24], maxZoom: 14 })
}

onMounted(async () => {
  await nextTick()
  if (!mapEl.value) {
    return
  }
  map = L.map(mapEl.value, {
    zoomControl: true,
    maxBounds: [
      [-85, -180],
      [85, 180],
    ],
    maxBoundsViscosity: 1.0,
    worldCopyJump: false,
  }).setView([0, 0], 2)
  map.attributionControl.setPrefix(false)
  applyTileLayer()
  layer = L.layerGroup().addTo(map)
  renderTrajectory()
})

onBeforeUnmount(() => {
  map?.remove()
  map = null
  tileLayer = null
  layer = null
})

watch(
  () => props.positions,
  () => renderTrajectory(),
  { deep: true },
)

watch(
  () => props.mapSource,
  () => applyTileLayer(),
  { deep: true },
)
</script>

<template>
  <div class="trajectory-map-shell">
    <div ref="mapEl" class="trajectory-map"></div>
    <div
      class="map-source-control"
      @click.stop
      @mousedown.stop
      @dblclick.stop
      @wheel.stop
    >
      <button class="map-source-icon" type="button" aria-label="切换地图图源">
        <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path stroke="none" d="M0 0h24v24H0z" fill="none" />
          <path d="M12 18.5l-3 -1.5l-6 3v-13l6 -3l6 3l6 -3v7.5" />
          <path d="M9 4v13" />
          <path d="M15 7v5.5" />
          <path d="M21.121 20.121a3 3 0 1 0 -4.242 0c.418 .419 1.125 1.045 2.121 1.879c1.051 -.89 1.759 -1.516 2.121 -1.879" />
          <path d="M19 18v.01" />
        </svg>
      </button>
      <div class="map-source-popover">
        <div class="map-source-drawer-header">
          <span>地图图源</span>
        </div>
        <div v-if="mapSources.length > 1" class="map-source-options">
          <button
            v-for="source in mapSources"
            :key="source.id"
            class="map-source-option"
            :class="{ active: source.id === mapSource.id }"
            type="button"
            @click="selectMapSource(source.id)"
          >
            <span class="map-source-option-name">{{ source.name }}</span>
            <span v-if="source.id === mapSource.id" class="map-source-option-check">当前</span>
          </button>
        </div>
        <span v-else class="map-source-control-pill">{{ mapSource.name }}</span>
      </div>
    </div>
  </div>
</template>
