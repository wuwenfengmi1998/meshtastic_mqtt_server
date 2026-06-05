<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { fallbackMapSource } from '../mapSource'
import type { PositionRecord, PublicMapTileSource } from '../types'

const props = withDefaults(defineProps<{
  positions: PositionRecord[]
  mapSource?: PublicMapTileSource
}>(), {
  mapSource: () => fallbackMapSource,
})

const mapEl = ref<HTMLElement | null>(null)
let map: L.Map | null = null
let tileLayer: L.TileLayer | null = null
let layer: L.LayerGroup | null = null

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

function renderTrajectory() {
  if (!map || !layer) {
    return
  }
  layer.clearLayers()
  const points = [...props.positions]
    .filter((position) => position.latitude != null && position.longitude != null)
    .reverse()
    .map((position) => [position.latitude as number, position.longitude as number] as L.LatLngTuple)

  if (points.length === 0) {
    map.setView([0, 0], 2)
    return
  }

  if (points.length > 1) {
    L.polyline(points, { color: '#2563eb', weight: 4, opacity: 0.8 }).addTo(layer)
  }
  L.circleMarker(points[0], { radius: 6, color: '#16a34a', fillColor: '#22c55e', fillOpacity: 0.9 }).bindPopup('起点').addTo(layer)
  L.circleMarker(points[points.length - 1], { radius: 6, color: '#dc2626', fillColor: '#ef4444', fillOpacity: 0.9 }).bindPopup('终点').addTo(layer)
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
  <div ref="mapEl" class="trajectory-map"></div>
</template>
