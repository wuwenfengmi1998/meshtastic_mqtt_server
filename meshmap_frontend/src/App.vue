<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { getAdminMe, getHealth, getMapReports, getNodeInfo, getPositions, getTextMessages } from './api'
import AdminDashboard from './components/AdminDashboard.vue'
import AdminLogin from './components/AdminLogin.vue'
import ChatPanel from './components/ChatPanel.vue'
import MeshMap from './components/MeshMap.vue'
import NodeListPanel from './components/NodeListPanel.vue'
import type { AdminUser, HealthStatus, MapNode, MapReport, NodeInfo, NodeInfoById, PositionRecord, TextMessage } from './types'

const isAdminPage = window.location.pathname === '/admin'
const adminUser = ref<AdminUser | null>(null)
const adminChecking = ref(false)

const loading = ref(true)
const nodePageLoading = ref(false)
const error = ref('')
const selectedNodeId = ref<string | null>(null)
const health = ref<HealthStatus | null>(null)
const nodeInfoSource = ref<NodeInfo[]>([])
const mapReportSource = ref<MapReport[]>([])
const pagedNodeInfo = ref<NodeInfo[]>([])
const nodePage = ref(1)
const nodePageSize = 25
const nodeTotal = ref(0)
const messages = ref<TextMessage[]>([])
const chatPageSize = 20
const chatLoadingOlder = ref(false)
const chatHasMore = ref(true)
const chatInitialized = ref(false)
const positions = ref<PositionRecord[]>([])
let refreshTimer: number | undefined

const nodesById = computed<NodeInfoById>(() => {
  const map = new Map<string, NodeInfo>()
  for (const node of nodeInfoSource.value) {
    map.set(node.node_id, node)
  }
  for (const node of pagedNodeInfo.value) {
    map.set(node.node_id, node)
  }
  return Object.fromEntries(map)
})

const mapNodes = computed<MapNode[]>(() => {
  return mapReportSource.value
    .filter((report) => report.latitude != null && report.longitude != null)
    .map((report) => {
      const nodeinfo = nodesById.value[report.node_id] ?? null
      return {
        node_id: report.node_id,
        label: report.short_name || report.long_name || nodeinfo?.short_name || nodeinfo?.long_name || report.node_id,
        latitude: report.latitude as number,
        longitude: report.longitude as number,
        altitude: report.altitude,
        source: 'map_report',
        updated_at: report.updated_at,
        nodeinfo,
        map_report: report,
        latest_position: null,
      }
    })
})

function toChronological(items: TextMessage[]): TextMessage[] {
  return [...items].reverse()
}

function compareMessages(a: TextMessage, b: TextMessage): number {
  const timeDiff = Date.parse(a.created_at) - Date.parse(b.created_at)
  return timeDiff !== 0 ? timeDiff : a.id - b.id
}

function mergeMessages(existing: TextMessage[], incoming: TextMessage[]): TextMessage[] {
  const byId = new Map<number, TextMessage>()
  for (const message of existing) {
    byId.set(message.id, message)
  }
  for (const message of incoming) {
    byId.set(message.id, message)
  }
  return Array.from(byId.values()).sort(compareMessages)
}

async function loadInitialChatMessages() {
  const response = await getTextMessages(chatPageSize, 0)
  messages.value = toChronological(response.items)
  chatHasMore.value = response.items.length === chatPageSize
  chatInitialized.value = true
}

async function loadOlderMessages() {
  if (chatLoadingOlder.value || !chatHasMore.value) {
    return
  }

  chatLoadingOlder.value = true
  try {
    const response = await getTextMessages(chatPageSize, messages.value.length)
    messages.value = mergeMessages(messages.value, toChronological(response.items))
    chatHasMore.value = response.items.length === chatPageSize
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    chatLoadingOlder.value = false
  }
}

async function pollLatestMessages() {
  const response = await getTextMessages(chatPageSize, 0)
  messages.value = mergeMessages(messages.value, toChronological(response.items))
}

async function loadNodePage(page: number, showLoading = true) {
  if (showLoading) {
    nodePageLoading.value = true
  }
  try {
    const safePage = Math.max(1, page)
    const response = await getNodeInfo(nodePageSize, (safePage - 1) * nodePageSize)
    pagedNodeInfo.value = response.items
    nodeTotal.value = response.total ?? response.offset + response.items.length
    nodePage.value = safePage
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (showLoading) {
      nodePageLoading.value = false
    }
  }
}

async function refresh(showLoading = true) {
  if (showLoading) {
    loading.value = true
  }
  error.value = ''
  try {
    const [healthData, nodeInfoData, mapReportData, positionData] = await Promise.all([
      getHealth(),
      getNodeInfo(500, 0),
      getMapReports(500, 0),
      getPositions(500),
    ])
    health.value = healthData
    nodeInfoSource.value = nodeInfoData.items
    mapReportSource.value = mapReportData.items
    positions.value = positionData.items
    await Promise.all([
      loadNodePage(nodePage.value, showLoading),
      chatInitialized.value ? pollLatestMessages() : loadInitialChatMessages(),
    ])
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    if (showLoading) {
      loading.value = false
    }
  }
}

async function checkAdminSession() {
  adminChecking.value = true
  try {
    const response = await getAdminMe()
    adminUser.value = response.user
  } catch {
    adminUser.value = null
  } finally {
    adminChecking.value = false
  }
}

onMounted(() => {
  if (isAdminPage) {
    checkAdminSession()
    return
  }
  refresh()
  refreshTimer = window.setInterval(() => refresh(false), 5000)
})

onBeforeUnmount(() => {
  if (refreshTimer !== undefined) {
    window.clearInterval(refreshTimer)
  }
})
</script>

<template>
  <main class="app-shell">
    <header class="topbar">
      <div>
        <p class="eyebrow">Meshtastic MQTT Server</p>
        <h1>{{ isAdminPage ? 'Admin' : 'MeshMap' }}</h1>
      </div>
      <div class="topbar-actions">
        <template v-if="isAdminPage">
          <a class="topbar-link" href="/">返回地图</a>
        </template>
        <template v-else>
          <span class="status-pill" :class="{ ok: health?.status === 'ok' }">
            {{ health?.status ?? 'unknown' }} / db {{ health?.database ?? 'unknown' }}
          </span>
          <span class="counter">节点 {{ nodeTotal }} · 已加载消息 {{ messages.length }} · 坐标 {{ mapNodes.length }}</span>
          <a class="topbar-link" href="/admin">管理</a>
          <button @click="() => refresh()" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
        </template>
      </div>
    </header>

    <template v-if="isAdminPage">
      <div v-if="adminChecking" class="panel admin-loading">正在检查登录状态...</div>
      <AdminDashboard v-else-if="adminUser" :user="adminUser" @logout="adminUser = null" />
      <AdminLogin v-else @login="adminUser = $event" />
    </template>

    <template v-else>
      <p v-if="error" class="error">{{ error }}</p>

      <section class="workspace">
        <ChatPanel
          :messages="messages"
          :nodes-by-id="nodesById"
          :selected-node-id="selectedNodeId"
          :loading-older="chatLoadingOlder"
          :has-more-messages="chatHasMore"
          @select-node="selectedNodeId = $event"
          @load-older="loadOlderMessages"
        />
        <MeshMap
          :nodes="mapNodes"
          :selected-node-id="selectedNodeId"
          @select-node="selectedNodeId = $event"
          @clear-node="selectedNodeId = null"
        />
      </section>

      <NodeListPanel
        :nodes="pagedNodeInfo"
        :selected-node-id="selectedNodeId"
        :page="nodePage"
        :page-size="nodePageSize"
        :total="nodeTotal"
        :loading="nodePageLoading || loading"
        @select-node="selectedNodeId = $event"
        @page-change="loadNodePage"
      />
    </template>
  </main>
</template>
