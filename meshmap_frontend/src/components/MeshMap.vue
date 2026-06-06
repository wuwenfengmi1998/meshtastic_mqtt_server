<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import L from 'leaflet'
import 'leaflet/dist/leaflet.css'
import { fallbackMapSource } from '../mapSource'
import type { MapBoundsChangePayload, MapClusterNode, MapNode, MapRenderable, PublicMapTileSource } from '../types'

const props = withDefaults(defineProps<{
  items: MapRenderable[]
  selectedNodeId: string | null
  isAdmin: boolean
  autoFit?: boolean
  loading?: boolean
  mapSource?: PublicMapTileSource
  mapSources?: PublicMapTileSource[]
}>(), {
  autoFit: true,
  loading: false,
  mapSource: () => fallbackMapSource,
  mapSources: () => [fallbackMapSource],
})

const emit = defineEmits<{
  'select-node': [nodeId: string]
  'clear-node': []
  'delete-node': [nodeId: string]
  'delete-and-block-node': [payload: { nodeId: string; nodeNum: number | null }]
  'bounds-change': [payload: MapBoundsChangePayload]
  'map-source-change': [sourceId: number]
}>()

const mapEl = ref<HTMLElement | null>(null)
const menuNode = ref<MapNode | null>(null)
const menuX = ref(0)
const menuY = ref(0)
const lastRaisedNodeId = ref<string | null>(null)
let map: L.Map | null = null
let tileLayer: L.TileLayer | null = null
let markerLayer: L.LayerGroup | null = null
const markersByKey = new Map<string, L.Marker>()
const overlapShuffleOrders = new Map<string, string[]>()
const shuffledSelectedNodeIds = new Set<string>()
let hasFitBounds = false

const minMapZoom = 3
const defaultMapCenter: L.LatLngExpression = [35.8617, 104.1954]
const defaultMapZoom = 4
const worldBounds = L.latLngBounds(
  [-85.05112878, -180],
  [85.05112878, 180],
)

onMounted(async () => {
  window.addEventListener('click', closeNodeMenu)
  window.addEventListener('keydown', handleKeydown)
  await nextTick()
  if (!mapEl.value) {
    return
  }
  map = L.map(mapEl.value, {
    zoomControl: true,
    minZoom: minMapZoom,
    maxBounds: worldBounds,
    maxBoundsViscosity: 1.0,
    worldCopyJump: false,
  }).setView(defaultMapCenter, defaultMapZoom)
  map.attributionControl.setPrefix(false)
  applyTileLayer()
  map.on('click', () => {
    closeNodeMenu()
    overlapShuffleOrders.clear()
    shuffledSelectedNodeIds.clear()
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
  tileLayer = null
  markerLayer = null
  markersByKey.clear()
  overlapShuffleOrders.clear()
  shuffledSelectedNodeIds.clear()
})

watch(
  () => [props.items, props.selectedNodeId] as const,
  () => renderMarkers(false),
  { deep: true },
)

watch(
  () => props.mapSource,
  () => applyTileLayer(),
  { deep: true },
)

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
    minZoom: minMapZoom,
    maxZoom: props.mapSource.max_zoom || fallbackMapSource.max_zoom,
    noWrap: true,
    bounds: worldBounds,
    attribution: props.mapSource.attribution || fallbackMapSource.attribution,
  }).addTo(map)
}

function closeNodeMenu() {
  menuNode.value = null
}

function nodeDetailHref(nodeId: string): string {
  return `/detailed/${encodeURIComponent(nodeId)}`
}

function openNodeMenu(node: MapNode, event: L.LeafletMouseEvent) {
  L.DomEvent.stopPropagation(event)
  lastRaisedNodeId.value = node.node_id
  emit('select-node', node.node_id)
  menuNode.value = node
  menuX.value = event.originalEvent.clientX
  menuY.value = event.originalEvent.clientY
}

function deleteSelectedNode() {
  if (menuNode.value) {
    emit('delete-node', menuNode.value.node_id)
  }
  closeNodeMenu()
}

function deleteAndBlockSelectedNode() {
  if (menuNode.value) {
    emit('delete-and-block-node', {
      nodeId: menuNode.value.node_id,
      nodeNum: menuNode.value.map_report?.node_num ?? menuNode.value.nodeinfo?.node_num ?? null,
    })
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
  const bounds = L.latLngBounds([])
  const visibleMarkerKeys = new Set<string>()
  const overlapGroups = buildOverlapGroups(props.items)

  for (const item of props.items) {
    const markerKey = mapMarkerKey(item)
    visibleMarkerKeys.add(markerKey)

    if (item.type === 'cluster') {
      const existingMarker = markersByKey.get(markerKey)
      if (!existingMarker) {
        const marker = buildClusterMarker(item)
        marker.addTo(markerLayer)
        markersByKey.set(markerKey, marker)
      }
      bounds.extend([item.latitude, item.longitude])
      continue
    }

    const node = item
    const rawSelected = node.node_id === props.selectedNodeId
    const shuffledSelected = rawSelected && shuffledSelectedNodeIds.has(node.node_id)
    const selected = rawSelected && !shuffledSelected
    const overlapGroupKey = nodeOverlapGroupKey(node, overlapGroups)
    const overlapGroup = overlapGroupKey ? overlapGroups.get(overlapGroupKey) : undefined
    const overlapIndex = overlapGroup ? nodeOverlapIndex(node, overlapGroup) : 0
    const raised = selected || node.node_id === lastRaisedNodeId.value
    const zIndexOffset = raised ? 1000 : overlapIndex
    const nodeIcon = L.divIcon({
      className: `node-marker${selected ? ' selected' : ''}`,
      html: `<span style="--node-color: ${nodeColor(node.node_id)}">${escapeHTML(node.label || 'N')}</span>`,
      iconSize: [34, 22],
      iconAnchor: [17, 11],
    })
    let marker = markersByKey.get(markerKey)

    if (!marker) {
      marker = L.marker([node.latitude, node.longitude], {
        icon: nodeIcon,
        title: node.label,
        zIndexOffset,
      })
      marker.bindPopup(buildNodePopupHTML(node), { maxWidth: 320, className: 'node-detail-popup' })
      marker.addTo(markerLayer)
      markersByKey.set(markerKey, marker)
    } else {
      marker.setLatLng([node.latitude, node.longitude])
      marker.setIcon(nodeIcon)
      marker.setZIndexOffset(zIndexOffset)
      marker.options.title = node.label
      marker.getElement()?.setAttribute('title', node.label)
      const popup = marker.getPopup()
      if (popup) {
        popup.setContent(buildNodePopupHTML(node))
      } else {
        marker.bindPopup(buildNodePopupHTML(node), { maxWidth: 320, className: 'node-detail-popup' })
      }
    }

    marker.off('click')
    marker.off('contextmenu')
    marker.on('click', (event) => {
      L.DomEvent.stopPropagation(event)
      closeNodeMenu()
      if (node.node_id === props.selectedNodeId) {
        if (moveSelectedNodeBehindOverlap(node, overlapGroups)) {
          shuffledSelectedNodeIds.add(node.node_id)
          marker?.closePopup()
          emit('clear-node')
          renderMarkers(false)
        }
        return
      }
      shuffledSelectedNodeIds.clear()
      lastRaisedNodeId.value = node.node_id
      emit('select-node', node.node_id)
    })
    marker.on('contextmenu', (event) => openNodeMenu(node, event))

    if (selected && !marker.getPopup()?.isOpen()) {
      marker.openPopup()
    }
    bounds.extend([node.latitude, node.longitude])
  }

  for (const [markerKey, marker] of markersByKey) {
    if (!visibleMarkerKeys.has(markerKey)) {
      markerLayer.removeLayer(marker)
      markersByKey.delete(markerKey)
    }
  }

  if (props.autoFit && props.items.length > 0 && (forceFit || !hasFitBounds)) {
    map.fitBounds(bounds, { padding: [24, 24], maxZoom: 13 })
    hasFitBounds = true
  }
}

function mapMarkerKey(item: MapRenderable): string {
  if (item.type === 'cluster') {
    return `cluster:${item.latitude}:${item.longitude}:${item.count}`
  }
  return `node:${item.node_id}`
}

function buildOverlapGroups(items: MapRenderable[]): Map<string, string[]> {
  const groups = new Map<string, string[]>()
  if (!map) {
    return groups
  }

  const capsules = items
    .filter((item): item is MapNode => item.type !== 'cluster')
    .map((node) => ({ node, bounds: nodeCapsuleBounds(node) }))
  const visited = new Set<string>()

  for (const capsule of capsules) {
    if (visited.has(capsule.node.node_id)) {
      continue
    }

    const stack = [capsule]
    const group: string[] = []
    visited.add(capsule.node.node_id)

    while (stack.length > 0) {
      const current = stack.pop()
      if (!current) {
        continue
      }
      group.push(current.node.node_id)

      for (const candidate of capsules) {
        if (visited.has(candidate.node.node_id)) {
          continue
        }
        if (capsuleBoundsOverlap(current.bounds, candidate.bounds)) {
          visited.add(candidate.node.node_id)
          stack.push(candidate)
        }
      }
    }

    if (group.length >= 2) {
      const key = overlapGroupKey(group)
      const existingOrder = overlapShuffleOrders.get(key) ?? []
      const activeIds = new Set(group)
      const ordered = existingOrder.filter((nodeId) => activeIds.has(nodeId))
      for (const nodeId of group) {
        if (!ordered.includes(nodeId)) {
          ordered.push(nodeId)
        }
      }
      overlapShuffleOrders.set(key, ordered)
      groups.set(key, ordered)
    }
  }

  for (const key of overlapShuffleOrders.keys()) {
    if (!groups.has(key)) {
      overlapShuffleOrders.delete(key)
    }
  }

  return groups
}

function nodeOverlapGroupKey(node: MapNode, overlapGroups: Map<string, string[]>): string | null {
  for (const [key, nodeIds] of overlapGroups) {
    if (nodeIds.includes(node.node_id)) {
      return key
    }
  }
  return null
}

function nodeOverlapIndex(node: MapNode, group: string[]): number {
  const index = group.indexOf(node.node_id)
  return index === -1 ? 0 : index
}

function moveSelectedNodeBehindOverlap(node: MapNode, overlapGroups: Map<string, string[]>): boolean {
  const groupKey = nodeOverlapGroupKey(node, overlapGroups)
  if (!groupKey) {
    return false
  }
  const group = overlapGroups.get(groupKey)
  if (!group || group.length < 2) {
    return false
  }

  const nextOrder = [node.node_id, ...group.filter((nodeId) => nodeId !== node.node_id)]
  overlapShuffleOrders.set(groupKey, nextOrder)
  lastRaisedNodeId.value = null
  return true
}

function overlapGroupKey(nodeIds: string[]): string {
  return [...nodeIds].sort().join('|')
}

function nodeCapsuleBounds(node: MapNode): { left: number; right: number; top: number; bottom: number } {
  const point = map!.latLngToLayerPoint([node.latitude, node.longitude])
  const width = nodeCapsuleWidth(node)
  const height = 22
  return {
    left: point.x - width / 2,
    right: point.x + width / 2,
    top: point.y - height / 2,
    bottom: point.y + height / 2,
  }
}

function nodeCapsuleWidth(node: MapNode): number {
  const label = node.label || 'N'
  return Math.max(34, Math.ceil(label.length * 6 + 10))
}

function capsuleBoundsOverlap(
  left: { left: number; right: number; top: number; bottom: number },
  right: { left: number; right: number; top: number; bottom: number },
): boolean {
  return left.left <= right.right && left.right >= right.left && left.top <= right.bottom && left.bottom >= right.top
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
      map.setView([cluster.latitude, cluster.longitude], Math.max(minMapZoom, Math.min(map.getZoom() + 2, map.getMaxZoom())))
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
    [42, 68],
    [92, 136],
    [188, 218],
    [330, 354],
  ]
  const range = hueRanges[hash % hueRanges.length]
  const hue = range[0] + (hash % (range[1] - range[0]))
  const saturation = 24 + (hash % 14)
  const lightness = 42 + (hash % 12)
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
    <!-- <div v-if="loading" class="map-empty">正在加载当前区域坐标...</div>
    <div v-else-if="items.length === 0" class="map-empty">暂无可显示坐标的节点</div> -->
    <div
      v-if="menuNode"
      class="context-menu"
      :style="{ left: `${menuX}px`, top: `${menuY}px` }"
      @click.stop
    >
      <a :href="nodeDetailHref(menuNode.node_id)">节点详细</a>
      <button v-if="isAdmin" class="danger" type="button" @click="deleteSelectedNode">删除</button>
      <button v-if="isAdmin" class="danger" type="button" @click="deleteAndBlockSelectedNode">删除并屏蔽节点</button>
    </div>
  </section>
</template>
