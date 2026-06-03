<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import type { MapNode } from '../types'

const props = defineProps<{
  nodes: MapNode[]
  selectedNodeId: string | null
  isAdmin: boolean
}>()

const emit = defineEmits<{
  'select-node': [nodeId: string]
  'clear-node': []
  'delete-node': [nodeId: string]
}>()

const mapEl = ref<HTMLElement | null>(null)
const menuNodeId = ref<string | null>(null)
const menuX = ref(0)
const menuY = ref(0)
let map: L.Map | null = null
let markerLayer: L.LayerGroup | null = null
let hasFitBounds = false

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
  L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
    maxZoom: 19,
    attribution: '&copy; OpenStreetMap contributors',
  }).addTo(map)
  map.on('click', () => {
    closeNodeMenu()
    emit('clear-node')
  })
  markerLayer = L.layerGroup().addTo(map)
  renderMarkers(true)
})

onBeforeUnmount(() => {
  map?.remove()
  map = null
  markerLayer = null
})

watch(
  () => [props.nodes, props.selectedNodeId] as const,
  () => renderMarkers(false),
  { deep: true },
)

function renderMarkers(forceFit: boolean) {
  if (!map || !markerLayer) {
    return
  }
  markerLayer.clearLayers()
  const bounds = L.latLngBounds([])

  for (const node of props.nodes) {
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
      emit('select-node', node.node_id)
    })
    marker.addTo(markerLayer)
    if (selected) {
      marker.openPopup()
    }
    bounds.extend([node.latitude, node.longitude])
  }

  if (props.nodes.length > 0 && (forceFit || !hasFitBounds)) {
    map.fitBounds(bounds, { padding: [24, 24], maxZoom: 13 })
    hasFitBounds = true
  }
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
    <div v-if="nodes.length === 0" class="map-empty">暂无可显示坐标的节点</div>
  </section>
</template>
