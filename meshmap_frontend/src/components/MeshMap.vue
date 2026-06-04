<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import type { MapBoundsChangePayload, MapClusterNode, MapNode, MapRenderable } from '../types'

const props = withDefaults(defineProps<{
  items: MapRenderable[]
  selectedNodeId: string | null
  isAdmin: boolean
  autoFit?: boolean
  loading?: boolean
}>(), {
  autoFit: true,
  loading: false,
})

const emit = defineEmits<{
  'select-node': [nodeId: string]
  'clear-node': []
  'delete-node': [nodeId: string]
  'bounds-change': [payload: MapBoundsChangePayload]
}>()

const mapEl = ref<HTMLElement | null>(null)
const menuNodeId = ref<string | null>(null)
const menuX = ref(0)
const menuY = ref(0)
let map: L.Map | null = null
let markerLayer: L.LayerGroup | null = null
let hasFitBounds = false

onMounted(async () => {
  window.addEventListener('click', closeNodeMenu)
  window.addEventListener('keydown', handleKeydown)
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
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19,
    attribution: '&copy; OpenStreetMap contributors',
  }).addTo(map)
  map.on('click', () => {
    closeNodeMenu()
    emit('clear-node')
  })
  map.on('moveend', emitBoundsChange)
  markerLayer = L.layerGroup().addTo(map)
  renderMarkers(true)
  emitBoundsChange()
})

onBeforeUnmount(() => {
  window.removeEventListener('click', closeNodeMenu)
  window.removeEventListener('keydown', handleKeydown)
  map?.remove()
  map = null
  markerLayer = null
})

watch(
  () => [props.items, props.selectedNodeId] as const,
  () => renderMarkers(false),
  { deep: true },
)

function closeNodeMenu() {
  menuNodeId.value = null
}

function nodeDetailHref(nodeId: string): string {
  return `/detailed/${encodeURIComponent(nodeId)}`
}

function openNodeMenu(node: MapNode, event: L.LeafletMouseEvent) {
  L.DomEvent.stopPropagation(event)
  emit('select-node', node.node_id)
  menuNodeId.value = node.node_id
  menuX.value = event.originalEvent.clientX
  menuY.value = event.originalEvent.clientY
}

function deleteSelectedNode() {
  if (menuNodeId.value) {
    emit('delete-node', menuNodeId.value)
  }
  closeNodeMenu()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeNodeMenu()
  }
}

function emitBoundsChange() {
  if (!map) {
    return
  }
  const bounds = map.getBounds()
  emit('bounds-change', {
    bounds: {
      min_lat: clamp(bounds.getSouth(), -90, 90),
      max_lat: clamp(bounds.getNorth(), -90, 90),
      min_lng: normalizeLongitude(bounds.getWest()),
      max_lng: normalizeLongitude(bounds.getEast()),
    },
    zoom: map.getZoom(),
  })
}

function renderMarkers(forceFit: boolean) {
  if (!map || !markerLayer) {
    return
  }
  markerLayer.clearLayers()
  const bounds = L.latLngBounds([])

  for (const item of props.items) {
    if (item.type === 'cluster') {
      const marker = buildClusterMarker(item)
      marker.addTo(markerLayer)
      bounds.extend([item.latitude, item.longitude])
      continue
    }
    const node = item
    const selected = node.node_id === props.selectedNodeId
    const marker = L.marker([node.latitude, node.longitude], {
      icon: L.divIcon({
        className: `node-marker${selected ? ' selected' : ''}`,
        html: `<span style="--node-color: ${nodeColor(node.node_id)}">${escapeHTML(node.label || 'N')}</span>`,
        iconSize: [34, 22],
        iconAnchor: [17, 11],
      }),
      title: node.label,
    })
    marker.bindPopup(buildNodePopupHTML(node), { maxWidth: 320, className: 'node-detail-popup' })
    marker.on('click', (event) => {
      L.DomEvent.stopPropagation(event)
      closeNodeMenu()
      emit('select-node', node.node_id)
    })
    marker.on('contextmenu', (event) => openNodeMenu(node, event))
    marker.addTo(markerLayer)
    if (selected) {
      marker.openPopup()
    }
    bounds.extend([node.latitude, node.longitude])
  }

  if (props.autoFit && props.items.length > 0 && (forceFit || !hasFitBounds)) {
    map.fitBounds(bounds, { padding: [24, 24], maxZoom: 13 })
    hasFitBounds = true
  }
}

function buildClusterMarker(cluster: MapClusterNode): L.Marker {
  const size = clusterIconSize(cluster.count)
  const marker = L.marker([cluster.latitude, cluster.longitude], {
    icon: L.divIcon({
      className: `cluster-marker ${clusterClass(cluster.count)}`,
      html: `<span>${formatCount(cluster.count)}</span>`,
      iconSize: [size, size],
      iconAnchor: [size / 2, size / 2],
    }),
    title: `${cluster.count} 个坐标`,
  })
  marker.bindPopup(buildClusterPopupHTML(cluster), { maxWidth: 260, className: 'node-detail-popup' })
  marker.on('click', () => {
    closeNodeMenu()
    if (map) {
      map.setView([cluster.latitude, cluster.longitude], Math.min(map.getZoom() + 2, map.getMaxZoom()))
    }
  })
  return marker
}

function buildNodePopupHTML(node: MapNode): string {
  const info = node.nodeinfo
  const report = node.map_report
  return `
    <div class="node-popup">
      <strong>${escapeHTML(node.node_id)}</strong>
      <dl>
        <div><dt>长名称</dt><dd>${escapeHTML(report?.long_name || info?.long_name || '-')}</dd></div>
        <div><dt>短名称</dt><dd>${escapeHTML(report?.short_name || info?.short_name || '-')}</dd></div>
        <div><dt>硬件型号</dt><dd>${escapeHTML(report?.hw_model || info?.hw_model || '-')}</dd></div>
        <div><dt>角色</dt><dd>${escapeHTML(report?.role || info?.role || '-')}</dd></div>
        <div><dt>固件版本</dt><dd>${escapeHTML(report?.firmware_version || '-')}</dd></div>
        <div><dt>区域</dt><dd>${escapeHTML(report?.region || '-')}</dd></div>
        <div><dt>调制预设</dt><dd>${escapeHTML(report?.modem_preset || '-')}</dd></div>
        <div><dt>海拔</dt><dd>${node.altitude ?? '-'}</dd></div>
        <div><dt>经度</dt><dd>${node.longitude.toFixed(5)}</dd></div>
        <div><dt>纬度</dt><dd>${node.latitude.toFixed(5)}</dd></div>
        <div><dt>位置精度</dt><dd>${report?.position_precision ?? '-'}</dd></div>
        <div><dt>在线节点</dt><dd>${report?.num_online_local_nodes ?? '-'}</dd></div>
      </dl>
    </div>
  `
}

function buildClusterPopupHTML(cluster: MapClusterNode): string {
  return `
    <div class="node-popup">
      <strong>聚合坐标</strong>
      <dl>
        <div><dt>数量</dt><dd>${cluster.count}</dd></div>
        <div><dt>经度</dt><dd>${cluster.longitude.toFixed(5)}</dd></div>
        <div><dt>纬度</dt><dd>${cluster.latitude.toFixed(5)}</dd></div>
      </dl>
    </div>
  `
}

function clusterIconSize(count: number): number {
  if (count >= 1000) {
    return 58
  }
  if (count >= 100) {
    return 50
  }
  if (count >= 10) {
    return 42
  }
  return 34
}

function clusterClass(count: number): string {
  if (count >= 1000) {
    return 'cluster-large'
  }
  if (count >= 100) {
    return 'cluster-medium'
  }
  return 'cluster-small'
}

function formatCount(count: number): string {
  return count >= 1000 ? `${Math.round(count / 100) / 10}k` : String(count)
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

function normalizeLongitude(value: number): number {
  let normalized = value
  while (normalized < -180) {
    normalized += 360
  }
  while (normalized > 180) {
    normalized -= 360
  }
  return normalized
}

function nodeColor(nodeId: string): string {
  let hash = 0
  for (let index = 0; index < nodeId.length; index += 1) {
    hash = (hash * 31 + nodeId.charCodeAt(index)) >>> 0
  }

  const hueRanges = [
    [35, 75],
    [95, 165],
    [185, 250],
    [265, 315],
  ]
  const range = hueRanges[hash % hueRanges.length]
  const hue = range[0] + (hash % (range[1] - range[0]))
  const saturation = 68 + (hash % 18)
  const lightness = 32 + (hash % 10)
  return `hsl(${hue} ${saturation}% ${lightness}%)`
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
</script>

<template>
  <section class="map-panel panel">
    <div ref="mapEl" class="map-container"></div>
    <div v-if="loading" class="map-empty">正在加载当前区域坐标...</div>
    <div v-else-if="items.length === 0" class="map-empty">暂无可显示坐标的节点</div>
    <div
      v-if="menuNodeId"
      class="context-menu"
      :style="{ left: `${menuX}px`, top: `${menuY}px` }"
      @click.stop
    >
      <a :href="nodeDetailHref(menuNodeId)">节点详细</a>
      <button v-if="isAdmin" class="danger" type="button" @click="deleteSelectedNode">删除</button>
    </div>
  </section>
</template>
