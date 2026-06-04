<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { adminLogout, createNodeBlockingRule, deleteNode, deleteTextMessage, getAdminMe, getHealth, getMapReportViewport, getNodeInfo, getPositions, getTextMessages } from './api'
import AdminBlockingManagement from './components/AdminBlockingManagement.vue'
import AdminDashboard from './components/AdminDashboard.vue'
import AdminDiscardDetails from './components/AdminDiscardDetails.vue'
import AdminLogin from './components/AdminLogin.vue'
import AdminLoginLogs from './components/AdminLoginLogs.vue'
import AdminUsers from './components/AdminUsers.vue'
import ChatPanel from './components/ChatPanel.vue'
import HelpPage from './components/HelpPage.vue'
import MeshMap from './components/MeshMap.vue'
import NodeDetailedPage from './components/NodeDetailedPage.vue'
import NodeListPanel from './components/NodeListPanel.vue'
import type { AdminUser, HealthStatus, MapBoundsChangePayload, MapBoundsQuery, MapRenderable, MapViewportItem, NodeInfo, NodeInfoById, PositionRecord, TextMessage } from './types'

const currentPath = window.location.pathname
const adminPath = currentPath
const isAdminPage = adminPath.startsWith('/admin')
const detailMatch = currentPath.match(/^\/detailed\/(.+)$/)
const detailedNodeId = detailMatch ? decodeURIComponent(detailMatch[1]) : ''
const isDetailedPage = !!detailedNodeId
const isHelpPage = currentPath === '/help'
const adminUser = ref<AdminUser | null>(null)
const adminChecking = ref(false)

const loading = ref(true)
const nodePageLoading = ref(false)
const error = ref('')
const selectedNodeId = ref<string | null>(null)
const health = ref<HealthStatus | null>(null)
const nodeInfoSource = ref<NodeInfo[]>([])
const mapViewportItems = ref<MapViewportItem[]>([])
const mapViewportMode = ref<'points' | 'clusters'>('points')
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
const currentMapBounds = ref<MapBoundsQuery | null>(null)
const currentMapZoom = ref(2)
const mapReportsLoading = ref(false)
const mapReportTotal = ref(0)
type NodeActionPayload = { nodeId: string; nodeNum: number | null; message?: TextMessage }
let refreshTimer: number | undefined
let mapBoundsTimer: number | undefined
let mapReportRequestSeq = 0

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

const mapItems = computed<MapRenderable[]>(() => {
  return mapViewportItems.value
    .filter((item) => item.type === 'cluster' || (item.latitude != null && item.longitude != null))
    .map((item) => {
      if (item.type === 'cluster') {
        return {
          type: 'cluster',
          cluster_id: item.cluster_id,
          latitude: item.latitude,
          longitude: item.longitude,
          count: item.count,
        }
      }
      const nodeinfo = nodesById.value[item.node_id] ?? null
      return {
        type: 'node',
        node_id: item.node_id,
        label: item.short_name || item.long_name || nodeinfo?.short_name || nodeinfo?.long_name || item.node_id,
        latitude: item.latitude as number,
        longitude: item.longitude as number,
        altitude: item.altitude,
        source: 'map_report',
        updated_at: item.updated_at,
        nodeinfo,
        map_report: item,
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

async function loadMapReportsForBounds(bounds: MapBoundsQuery, zoom: number, showLoading = true) {
  const requestSeq = ++mapReportRequestSeq
  if (showLoading) {
    mapReportsLoading.value = true
  }
  try {
    const response = await getMapReportViewport(bounds, zoom)
    if (requestSeq !== mapReportRequestSeq) {
      return
    }
    mapViewportItems.value = response.items
    mapViewportMode.value = response.mode
    mapReportTotal.value = response.total
  } catch (err) {
    if (requestSeq === mapReportRequestSeq) {
      error.value = err instanceof Error ? err.message : String(err)
    }
  } finally {
    if (requestSeq === mapReportRequestSeq && showLoading) {
      mapReportsLoading.value = false
    }
  }
}

function handleMapBoundsChange(payload: MapBoundsChangePayload) {
  currentMapBounds.value = payload.bounds
  currentMapZoom.value = payload.zoom
  if (mapBoundsTimer !== undefined) {
    window.clearTimeout(mapBoundsTimer)
  }
  mapBoundsTimer = window.setTimeout(() => {
    loadMapReportsForBounds(payload.bounds, payload.zoom)
  }, 250)
}

async function refresh(showLoading = true) {
  if (showLoading) {
    loading.value = true
  }
  error.value = ''
  try {
    const [healthData, nodeInfoData, positionData] = await Promise.all([
      getHealth(),
      getNodeInfo(500, 0),
      getPositions(500),
    ])
    health.value = healthData
    nodeInfoSource.value = nodeInfoData.items
    positions.value = positionData.items
    await Promise.all([
      currentMapBounds.value ? loadMapReportsForBounds(currentMapBounds.value, currentMapZoom.value, false) : Promise.resolve(),
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

async function logoutAdmin() {
  try {
    await adminLogout()
  } finally {
    adminUser.value = null
  }
}

async function deleteMessage(message: TextMessage) {
  try {
    await deleteTextMessage(message.id)
    messages.value = messages.value.filter((item) => item.id !== message.id)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function removeNodeFromLocalState(nodeId: string) {
  nodeInfoSource.value = nodeInfoSource.value.filter((node) => node.node_id !== nodeId)
  pagedNodeInfo.value = pagedNodeInfo.value.filter((node) => node.node_id !== nodeId)
  mapViewportItems.value = mapViewportItems.value.filter((item) => item.type === 'cluster' || item.node_id !== nodeId)
  if (selectedNodeId.value === nodeId) {
    selectedNodeId.value = null
  }
  await loadNodePage(nodePage.value, false)
}

function isAlreadyBlockedError(err: unknown): boolean {
  return err instanceof Error && err.message === 'blocking rule already exists'
}

function isNodeNotFoundError(err: unknown): boolean {
  return err instanceof Error && err.message === 'node not found'
}

function isMessageNotFoundError(err: unknown): boolean {
  return err instanceof Error && err.message === 'message not found'
}

async function deleteNodeById(nodeId: string) {
  try {
    await deleteNode(nodeId)
    await removeNodeFromLocalState(nodeId)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function deleteAndBlockNode(payload: NodeActionPayload) {
  try {
    if (payload.message) {
      try {
        await deleteTextMessage(payload.message.id)
      } catch (err) {
        if (!isMessageNotFoundError(err)) {
          throw err
        }
      }
      messages.value = messages.value.filter((item) => item.id !== payload.message?.id)
    }

    try {
      await createNodeBlockingRule({
        node_id: payload.nodeId,
        node_num: payload.nodeNum,
        reason: '管理员右键删除并屏蔽节点',
        enabled: true,
      })
    } catch (err) {
      if (!isAlreadyBlockedError(err)) {
        throw err
      }
    }

    try {
      await deleteNode(payload.nodeId)
    } catch (err) {
      if (!isNodeNotFoundError(err)) {
        throw err
      }
    }
    await removeNodeFromLocalState(payload.nodeId)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(() => {
  if (isAdminPage) {
    checkAdminSession()
    return
  }
  checkAdminSession()
  if (isDetailedPage || isHelpPage) {
    return
  }
  refresh()
  refreshTimer = window.setInterval(() => refresh(false), 5000)
})

onBeforeUnmount(() => {
  if (refreshTimer !== undefined) {
    window.clearInterval(refreshTimer)
  }
  if (mapBoundsTimer !== undefined) {
    window.clearTimeout(mapBoundsTimer)
  }
})
</script>

<template>
  <main class="app-shell">
    <header class="topbar">
      <div>
        <p class="eyebrow">Meshtastic MQTT Server</p>
        <h1 v-if="isDetailedPage">节点详情</h1>
        <h1 v-else-if="isHelpPage">使用帮助</h1>
        <h1 v-else>{{ isAdminPage ? 'Admin' : 'MeshMap' }}</h1>
      </div>
      <div class="topbar-actions">
        <template v-if="isAdminPage">
          <nav v-if="adminUser" class="admin-nav">
            <a href="/admin" :class="{ active: adminPath === '/admin' }">服务状态</a>
            <a href="/admin/users" :class="{ active: adminPath === '/admin/users' }">用户管理</a>
            <a href="/admin/blocking_management" :class="{ active: adminPath === '/admin/blocking_management' }">屏蔽管理</a>
            <a href="/admin/log/login" :class="{ active: adminPath === '/admin/log/login' }">登录日志</a>
            <a href="/admin/discard_details" :class="{ active: adminPath === '/admin/discard_details' }">丢弃数据</a>
          </nav>
          <a class="topbar-link" href="/">返回地图</a>
        </template>
        <template v-else-if="isDetailedPage">
          <span class="counter">{{ detailedNodeId }}</span>
          <a class="topbar-link" href="/">返回地图</a>
          <a class="topbar-link" href="/admin">管理</a>
        </template>
        <template v-else-if="isHelpPage">
          <a class="topbar-link" href="/">返回地图</a>
          <a class="topbar-link" href="/admin">管理</a>
        </template>
        <template v-else>
          <a class="topbar-link" href="/help">使用帮助</a>
          <span class="counter">节点 {{ nodeTotal }} · 已加载消息 {{ messages.length }} · 坐标 {{ mapItems.length }} / {{ mapReportTotal }}{{ mapViewportMode === 'clusters' ? ' · 已聚合' : '' }}{{ mapReportsLoading ? ' · 坐标加载中...' : '' }}</span>
          <a class="topbar-link" href="/admin">管理</a>
          <button @click="() => refresh()" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
        </template>
      </div>
    </header>

    <template v-if="isAdminPage">
      <div v-if="adminChecking" class="panel admin-loading">正在检查登录状态...</div>
      <template v-else-if="adminUser">
        <div class="panel admin-session-card">
          <div>
            <p class="eyebrow">Session</p>
            <h2>当前登录：{{ adminUser.username }}</h2>
          </div>
          <button class="admin-button" @click="logoutAdmin">退出登录</button>
        </div>
        <AdminUsers v-if="adminPath === '/admin/users'" :user="adminUser" />
        <AdminBlockingManagement v-else-if="adminPath === '/admin/blocking_management'" />
        <AdminLoginLogs v-else-if="adminPath === '/admin/log/login'" />
        <AdminDiscardDetails v-else-if="adminPath === '/admin/discard_details'" />
        <AdminDashboard v-else />
      </template>
      <AdminLogin v-else @login="adminUser = $event" />
    </template>

    <template v-else-if="isDetailedPage">
      <NodeDetailedPage :node-id="detailedNodeId" :is-admin="!!adminUser" />
    </template>

    <template v-else-if="isHelpPage">
      <HelpPage />
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
          :is-admin="!!adminUser"
          @select-node="selectedNodeId = $event"
          @load-older="loadOlderMessages"
          @delete-message="deleteMessage"
          @delete-and-block-node="deleteAndBlockNode"
        />
        <MeshMap
          :items="mapItems"
          :selected-node-id="selectedNodeId"
          :is-admin="!!adminUser"
          :auto-fit="false"
          :loading="mapReportsLoading"
          @bounds-change="handleMapBoundsChange"
          @select-node="selectedNodeId = $event"
          @clear-node="selectedNodeId = null"
          @delete-node="deleteNodeById"
          @delete-and-block-node="deleteAndBlockNode"
        />
      </section>

      <NodeListPanel
        :nodes="pagedNodeInfo"
        :selected-node-id="selectedNodeId"
        :page="nodePage"
        :page-size="nodePageSize"
        :total="nodeTotal"
        :loading="nodePageLoading || loading"
        :is-admin="!!adminUser"
        @select-node="selectedNodeId = $event"
        @page-change="loadNodePage"
        @delete-node="deleteNodeById"
        @delete-and-block-node="deleteAndBlockNode"
      />
    </template>
  </main>
</template>
