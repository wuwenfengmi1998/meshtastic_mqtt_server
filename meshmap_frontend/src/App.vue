<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { adminLogout, createNodeBlockingRule, deleteNode, deleteTextMessage, getAdminMe, getHealth, getMapReportViewport, getNodeInfo, getPositions, getTextMessages } from './api'
import AdminBlockingManagement from './components/AdminBlockingManagement.vue'
import AdminBot from './components/AdminBot.vue'
import AdminBotDirect from './components/AdminBotDirect.vue'
import AdminDashboard from './components/AdminDashboard.vue'
import AdminDiscardDetails from './components/AdminDiscardDetails.vue'
import AdminHelpEdit from './components/AdminHelpEdit.vue'
import AdminLogin from './components/AdminLogin.vue'
import AdminLoginLogs from './components/AdminLoginLogs.vue'
import AdminMapSource from './components/AdminMapSource.vue'
import AdminMqttForward from './components/AdminMqttForward.vue'
import AdminSignManagement from './components/AdminSignManagement.vue'
import AdminUsers from './components/AdminUsers.vue'
import ChatPanel from './components/ChatPanel.vue'
import ConfirmDeleteModal from './components/ConfirmDeleteModal.vue'
import HelpPage from './components/HelpPage.vue'
import MeshMap from './components/MeshMap.vue'
import NodeDetailedPage from './components/NodeDetailedPage.vue'
import NodeListPanel from './components/NodeListPanel.vue'
import SignedPage from './components/SignedPage.vue'
import { fallbackMapSource, loadEnabledMapSources } from './mapSource'
import type { AdminUser, HealthStatus, MapBoundsChangePayload, MapBoundsQuery, MapRenderable, MapViewportItem, MapViewportPoint, NodeInfo, NodeInfoById, PositionRecord, PublicMapTileSource, TextMessage } from './types'

const currentPath = window.location.pathname
const adminPath = currentPath
const isAdminPage = adminPath.startsWith('/admin')
const isMqttForwardAdminPage = adminPath === '/admin/mqtt_forward' || adminPath === '/admin/mqtt_forward/'
const isBotAdminPage = adminPath === '/admin/bot' || adminPath === '/admin/bot/'
const isBotDirectAdminPage = adminPath === '/admin/bot/direct' || adminPath === '/admin/bot/direct/'
const isSignAdminPage = adminPath === '/admin/sign' || adminPath === '/admin/sign/'
const detailMatch = currentPath.match(/^\/detailed\/(.+)$/)
const detailedNodeId = detailMatch ? decodeURIComponent(detailMatch[1]) : ''
const isDetailedPage = !!detailedNodeId
const isHelpPage = currentPath === '/help'
const isSignedPage = currentPath === '/signed'
const adminUser = ref<AdminUser | null>(null)
const adminChecking = ref(false)

const loading = ref(true)
const nodePageLoading = ref(false)
const error = ref('')
const selectedNodeId = ref<string | null>(null)
const health = ref<HealthStatus | null>(null)
const nodeInfoSource = ref<NodeInfo[]>([])
const mapViewportItems = ref<MapViewportItem[]>([])
const selectedMapPoint = ref<MapViewportPoint | null>(null)
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
const mapSources = ref<PublicMapTileSource[]>([fallbackMapSource])
const mapSource = ref<PublicMapTileSource>(fallbackMapSource)
const pendingDeleteAction = ref<PendingDeleteAction | null>(null)
type DeletableTextMessage = TextMessage & { mergedCount?: number; mergedMessages?: TextMessage[] }
type NodeActionRequest = { nodeId: string; nodeNum: number | null; message?: DeletableTextMessage }
type NodeActionPayload = NodeActionRequest & { reason: string }
type PendingDeleteAction =
  | { kind: 'delete-message'; message: DeletableTextMessage }
  | { kind: 'delete-node'; nodeId: string }
  | ({ kind: 'delete-and-block-node' } & NodeActionRequest)
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
  const items = mapViewportItems.value
  const selectedItem = selectedMapPoint.value
  const renderItems = selectedItem && selectedItem.type === 'point' && !items.some((item) => item.type === 'point' && item.node_id === selectedItem.node_id)
    ? [...items, selectedItem]
    : items

  return renderItems
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

const deleteModalTitle = computed(() => {
  const action = pendingDeleteAction.value
  if (!action) {
    return ''
  }
  if (action.kind === 'delete-message') {
    return '确认删除消息'
  }
  if (action.kind === 'delete-node') {
    return '确认删除节点'
  }
  return '确认删除并屏蔽节点'
})

const deleteModalMessage = computed(() => {
  const action = pendingDeleteAction.value
  if (!action) {
    return ''
  }
  if (action.kind === 'delete-message') {
    const count = deleteMessageCount(action.message)
    return count > 1
      ? `确定要删除这组已合并的 ${count} 条聊天消息吗？此操作不可撤销。`
      : '确定要删除这条聊天消息吗？此操作不可撤销。'
  }
  if (action.kind === 'delete-node') {
    return '确定要删除这个节点吗？此操作不可撤销。'
  }
  if (!action.message) {
    return '确定要删除并屏蔽这个节点吗？请输入屏蔽原因。'
  }
  const count = deleteMessageCount(action.message)
  return count > 1
    ? `确定要删除这组已合并的 ${count} 条聊天消息并屏蔽该节点吗？请输入屏蔽原因。`
    : '确定要删除这条聊天消息并屏蔽该节点吗？请输入屏蔽原因。'
})

const deleteModalConfirmText = computed(() => {
  return pendingDeleteAction.value?.kind === 'delete-and-block-node' ? '删除并屏蔽' : '删除'
})

const deleteModalRequiresReason = computed(() => pendingDeleteAction.value?.kind === 'delete-and-block-node')

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

function messagesForDelete(message: DeletableTextMessage): TextMessage[] {
  const items = message.mergedMessages?.length ? message.mergedMessages : [message]
  return Array.from(new Map(items.map((item) => [item.id, item])).values())
}

function deleteMessageCount(message: DeletableTextMessage): number {
  return messagesForDelete(message).length
}

function isSameJSON(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right)
}

function isMapViewportPoint(item: MapViewportItem): item is MapViewportPoint {
  return item.type === 'point'
}

function selectNode(nodeId: string) {
  selectedNodeId.value = nodeId
  const item = mapViewportItems.value.find((item): item is MapViewportPoint => isMapViewportPoint(item) && item.node_id === nodeId)
  selectedMapPoint.value = item ?? (selectedMapPoint.value?.node_id === nodeId ? selectedMapPoint.value : null)
}

function clearSelectedNode() {
  selectedNodeId.value = null
  selectedMapPoint.value = null
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
    if (!isSameJSON(mapViewportItems.value, response.items)) {
      mapViewportItems.value = response.items
    }
    const selectedItem = selectedNodeId.value
      ? response.items.find((item): item is MapViewportPoint => isMapViewportPoint(item) && item.node_id === selectedNodeId.value)
      : null
    if (selectedItem) {
      selectedMapPoint.value = selectedItem
    }
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

async function loadMapSource() {
  const sources = await loadEnabledMapSources()
  mapSources.value = sources
  mapSource.value = sources[0] ?? fallbackMapSource
}

function selectMapSource(sourceId: number) {
  const source = mapSources.value.find((item) => item.id === sourceId)
  if (source) {
    mapSource.value = source
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

function requestDeleteMessage(message: DeletableTextMessage) {
  pendingDeleteAction.value = { kind: 'delete-message', message }
}

function requestDeleteNode(nodeId: string) {
  pendingDeleteAction.value = { kind: 'delete-node', nodeId }
}

function requestDeleteAndBlockNode(payload: NodeActionRequest) {
  pendingDeleteAction.value = { kind: 'delete-and-block-node', ...payload }
}

function cancelDeleteModal() {
  pendingDeleteAction.value = null
}

async function confirmDeleteModal(payload: { reason?: string }) {
  const action = pendingDeleteAction.value
  pendingDeleteAction.value = null
  if (!action) {
    return
  }

  if (action.kind === 'delete-message') {
    await deleteMessage(action.message)
    return
  }

  if (action.kind === 'delete-node') {
    await deleteNodeById(action.nodeId)
    return
  }

  const reason = payload.reason?.trim()
  if (!reason) {
    return
  }
  await deleteAndBlockNode({
    nodeId: action.nodeId,
    nodeNum: action.nodeNum,
    message: action.message,
    reason,
  })
}

async function deleteMessage(message: DeletableTextMessage) {
  try {
    await deleteMessagesFromLocalState(message)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function removeNodeFromLocalState(nodeId: string) {
  nodeInfoSource.value = nodeInfoSource.value.filter((node) => node.node_id !== nodeId)
  pagedNodeInfo.value = pagedNodeInfo.value.filter((node) => node.node_id !== nodeId)
  mapViewportItems.value = mapViewportItems.value.filter((item) => item.type === 'cluster' || item.node_id !== nodeId)
  if (selectedNodeId.value === nodeId) {
    clearSelectedNode()
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

async function deleteMessagesFromLocalState(message: DeletableTextMessage) {
  const items = messagesForDelete(message)
  const removableIds = new Set<number>()
  const errors: string[] = []

  await Promise.all(items.map(async (item) => {
    try {
      await deleteTextMessage(item.id)
      removableIds.add(item.id)
    } catch (err) {
      if (isMessageNotFoundError(err)) {
        removableIds.add(item.id)
        return
      }
      errors.push(err instanceof Error ? err.message : String(err))
    }
  }))

  if (removableIds.size > 0) {
    messages.value = messages.value.filter((item) => !removableIds.has(item.id))
  }
  if (errors.length > 0) {
    throw new Error(`部分消息删除失败（${errors.length}/${items.length}）：${errors[0]}`)
  }
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
      await deleteMessagesFromLocalState(payload.message)
    }

    try {
      await createNodeBlockingRule({
        node_id: payload.nodeId,
        node_num: payload.nodeNum,
        reason: payload.reason,
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
  if (isDetailedPage || isHelpPage || isSignedPage) {
    return
  }
  loadMapSource()
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
            <a href="/admin/mqtt_forward/" :class="{ active: isMqttForwardAdminPage }">MQTT转发</a>
            <a href="/admin/bot" :class="{ active: isBotAdminPage }">机器人</a>
            <a href="/admin/bot/direct" :class="{ active: isBotDirectAdminPage }">机器人私聊</a>
            <a href="/admin/sign" :class="{ active: isSignAdminPage }">签到管理</a>
            <a href="/admin/map_source" :class="{ active: adminPath === '/admin/map_source' }">地图图源</a>
            <a href="/admin/help_edit" :class="{ active: adminPath === '/admin/help_edit' }">帮助编辑</a>
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
          
          <span class="counter">节点 {{ nodeTotal }} · 已加载消息 {{ messages.length }} · 坐标 {{ mapItems.length }} / {{ mapReportTotal }}{{ mapViewportMode === 'clusters' ? ' · 已聚合' : '' }}{{ mapReportsLoading ? ' · 坐标加载中...' : '' }}</span>
          <a class="topbar-link" href="/signed">签到列表</a>
          <a class="topbar-link" href="/help">使用帮助</a>
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
        <AdminMqttForward v-else-if="isMqttForwardAdminPage" />
        <AdminBot v-else-if="isBotAdminPage" />
        <AdminBotDirect v-else-if="isBotDirectAdminPage" />
        <AdminSignManagement v-else-if="isSignAdminPage" />
        <AdminMapSource v-else-if="adminPath === '/admin/map_source'" />
        <AdminHelpEdit v-else-if="adminPath === '/admin/help_edit'" />
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

    <template v-else-if="isSignedPage">
      <SignedPage />
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
          @select-node="selectNode"
          @load-older="loadOlderMessages"
          @delete-message="requestDeleteMessage"
          @delete-and-block-node="requestDeleteAndBlockNode"
        />
        <MeshMap
          :items="mapItems"
          :selected-node-id="selectedNodeId"
          :is-admin="!!adminUser"
          :auto-fit="false"
          :loading="mapReportsLoading"
          :map-source="mapSource"
          :map-sources="mapSources"
          @map-source-change="selectMapSource"
          @bounds-change="handleMapBoundsChange"
          @select-node="selectNode"
          @clear-node="clearSelectedNode"
          @delete-node="requestDeleteNode"
          @delete-and-block-node="requestDeleteAndBlockNode"
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
        @select-node="selectNode"
        @page-change="loadNodePage"
        @delete-node="requestDeleteNode"
        @delete-and-block-node="requestDeleteAndBlockNode"
      />
    </template>

    <ConfirmDeleteModal
      :open="!!pendingDeleteAction"
      :title="deleteModalTitle"
      :message="deleteModalMessage"
      :confirm-text="deleteModalConfirmText"
      :require-reason="deleteModalRequiresReason"
      reason-label="屏蔽原因"
      reason-placeholder="请输入屏蔽原因"
      @cancel="cancelDeleteModal"
      @confirm="confirmDeleteModal"
    />
  </main>
</template>
