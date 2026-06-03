<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { getHealth, getNodes, getPositions, getTextMessages } from './api'
import ChatPanel from './components/ChatPanel.vue'
import MeshMap from './components/MeshMap.vue'
import NodeListPanel from './components/NodeListPanel.vue'
import type { HealthStatus, MapNode, NodeInfoById, NodeInfoMap, PositionRecord, TextMessage } from './types'

const loading = ref(true)
const nodePageLoading = ref(false)
const error = ref('')
const selectedNodeId = ref<string | null>(null)
const health = ref<HealthStatus | null>(null)
const mapNodeSource = ref<NodeInfoMap[]>([])
const pagedNodes = ref<NodeInfoMap[]>([])
const nodePage = ref(1)
const nodePageSize = 25
const nodeTotal = ref(0)
const messages = ref<TextMessage[]>([])
const positions = ref<PositionRecord[]>([])

const nodesById = computed<NodeInfoById>(() => {
  const map = new Map<string, NodeInfoMap>()
  for (const node of mapNodeSource.value) {
    map.set(node.node_id, node)
  }
  for (const node of pagedNodes.value) {
    map.set(node.node_id, node)
  }
  return Object.fromEntries(map)
})

const mapNodes = computed<MapNode[]>(() => {
  return mapNodeSource.value
    .filter((node) => node.latitude != null && node.longitude != null)
    .map((node) => ({
      node_id: node.node_id,
      label: node.short_name || node.node_id,
      latitude: node.latitude as number,
      longitude: node.longitude as number,
      altitude: node.altitude,
      source: 'node',
      updated_at: node.updated_at,
      node,
      latest_position: null,
    }))
})

async function loadNodePage(page: number) {
  nodePageLoading.value = true
  try {
    const safePage = Math.max(1, page)
    const response = await getNodes(nodePageSize, (safePage - 1) * nodePageSize)
    pagedNodes.value = response.items
    nodeTotal.value = response.total ?? response.offset + response.items.length
    nodePage.value = safePage
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    nodePageLoading.value = false
  }
}

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const [healthData, mapNodeData, messageData, positionData] = await Promise.all([
      getHealth(),
      getNodes(500, 0),
      getTextMessages(100),
      getPositions(500),
    ])
    health.value = healthData
    mapNodeSource.value = mapNodeData.items
    messages.value = messageData.items
    positions.value = positionData.items
    await loadNodePage(nodePage.value)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <main class="app-shell">
    <header class="topbar">
      <div>
        <p class="eyebrow">Meshtastic MQTT Server</p>
        <h1>MeshMap</h1>
      </div>
      <div class="topbar-actions">
        <span class="status-pill" :class="{ ok: health?.status === 'ok' }">
          {{ health?.status ?? 'unknown' }} / db {{ health?.database ?? 'unknown' }}
        </span>
        <span class="counter">节点 {{ nodeTotal }} · 消息 {{ messages.length }} · 坐标 {{ mapNodes.length }}</span>
        <button @click="refresh" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
      </div>
    </header>

    <p v-if="error" class="error">{{ error }}</p>

    <section class="workspace">
      <ChatPanel
        :messages="messages"
        :nodes-by-id="nodesById"
        :selected-node-id="selectedNodeId"
        @select-node="selectedNodeId = $event"
      />
      <MeshMap
        :nodes="mapNodes"
        :selected-node-id="selectedNodeId"
        @select-node="selectedNodeId = $event"
        @clear-node="selectedNodeId = null"
      />
    </section>

    <NodeListPanel
      :nodes="pagedNodes"
      :selected-node-id="selectedNodeId"
      :page="nodePage"
      :page-size="nodePageSize"
      :total="nodeTotal"
      :loading="nodePageLoading || loading"
      @select-node="selectedNodeId = $event"
      @page-change="loadNodePage"
    />
  </main>
</template>
