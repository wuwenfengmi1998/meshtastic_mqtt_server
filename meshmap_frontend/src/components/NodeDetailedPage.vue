<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { createNodeBlockingRule, deleteNode, deleteTextMessage, getMapReportById, getNodeInfoById, getPositions, getTelemetry, getTextMessages } from '../api'
import type { MapReport, NodeInfo, PositionRecord, PublicMapTileSource, TelemetryRecord, TextMessage } from '../types'
import { fallbackMapSource, loadEnabledMapSources } from '../mapSource'
import ConfirmDeleteModal from './ConfirmDeleteModal.vue'
import NodeTrajectoryMap from './NodeTrajectoryMap.vue'

const props = defineProps<{
  nodeId: string
  isAdmin: boolean
}>()

const nodeInfo = ref<NodeInfo | null>(null)
const mapReport = ref<MapReport | null>(null)
const messages = ref<TextMessage[]>([])
const positions = ref<PositionRecord[]>([])
const telemetry = ref<TelemetryRecord[]>([])
const mapSources = ref<PublicMapTileSource[]>([fallbackMapSource])
const mapSource = ref<PublicMapTileSource>(fallbackMapSource)
const loading = ref(true)
const chatLoadingOlder = ref(false)
const chatHasMore = ref(true)
const telemetryLoading = ref(false)
const trajectoryLoading = ref(false)
const trajectoryError = ref('')
const trajectoryTruncated = ref(false)
const error = ref('')
const chatPageSize = 20
const telemetryPageSize = 25
const trajectoryPageSize = 500
const maxTrajectoryPoints = 5000
const telemetryPage = ref(1)
const trajectoryStartDate = ref(toDateInputValue())
const trajectoryEndDate = ref(toDateInputValue())
const chatHistoryRef = ref<HTMLElement | null>(null)
const scrollOverflowAllowance = 1
type GroupedTextMessage = TextMessage & { mergedCount: number; mergedMessages: TextMessage[] }
type PendingDeleteAction =
  | { kind: 'delete-message'; message: GroupedTextMessage }
  | { kind: 'delete-and-block-node'; message: GroupedTextMessage; nodeId: string; nodeNum: number | null }

const menuMessage = ref<GroupedTextMessage | null>(null)
const menuX = ref(0)
const menuY = ref(0)
const pendingDeleteAction = ref<PendingDeleteAction | null>(null)

const nodeTitle = computed(() => {
  return nodeInfo.value?.long_name || nodeInfo.value?.short_name || mapReport.value?.long_name || mapReport.value?.short_name || props.nodeId
})

const mergedNode = computed(() => {
  return {
    node_num: nodeInfo.value?.node_num ?? mapReport.value?.node_num ?? null,
    long_name: nodeInfo.value?.long_name || mapReport.value?.long_name || null,
    short_name: nodeInfo.value?.short_name || mapReport.value?.short_name || null,
    hw_model: nodeInfo.value?.hw_model || mapReport.value?.hw_model || null,
    role: nodeInfo.value?.role || mapReport.value?.role || null,
    updated_at: nodeInfo.value?.updated_at || mapReport.value?.updated_at || null,
  }
})

const deleteModalTitle = computed(() => {
  if (pendingDeleteAction.value?.kind === 'delete-and-block-node') {
    return '确认删除并屏蔽节点'
  }
  return '确认删除消息'
})

const deleteModalMessage = computed(() => {
  const action = pendingDeleteAction.value
  if (!action) {
    return ''
  }
  const count = deleteMessageCount(action.message)
  if (action.kind === 'delete-and-block-node') {
    return count > 1
      ? `确定要删除这组已合并的 ${count} 条聊天消息并屏蔽该节点吗？请输入屏蔽原因。`
      : '确定要删除这条聊天消息并屏蔽该节点吗？请输入屏蔽原因。'
  }
  return count > 1
    ? `确定要删除这组已合并的 ${count} 条聊天消息吗？此操作不可撤销。`
    : '确定要删除这条聊天消息吗？此操作不可撤销。'
})

const deleteModalConfirmText = computed(() => {
  return pendingDeleteAction.value?.kind === 'delete-and-block-node' ? '删除并屏蔽' : '删除'
})

const deleteModalRequiresReason = computed(() => pendingDeleteAction.value?.kind === 'delete-and-block-node')

const groupedMessages = computed<GroupedTextMessage[]>(() => {
  const groups = new Map<string, GroupedTextMessage>()
  for (const message of messages.value) {
    const key = `${message.packet_id ?? ''}\n${message.text ?? ''}`
    const group = groups.get(key)
    if (group) {
      group.mergedCount += 1
      group.mergedMessages.push(message)
    } else {
      groups.set(key, { ...message, mergedCount: 1, mergedMessages: [message] })
    }
  }
  return Array.from(groups.values())
})

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

function toDateInputValue(date = new Date()): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function localDateRange(startDate: string, endDate: string): { since: string; until: string } | null {
  if (!startDate || !endDate) {
    trajectoryError.value = '请选择开始日期和结束日期'
    return null
  }
  const safeStartDate = startDate <= endDate ? startDate : endDate
  const safeEndDate = startDate <= endDate ? endDate : startDate
  trajectoryStartDate.value = safeStartDate
  trajectoryEndDate.value = safeEndDate
  const since = new Date(`${safeStartDate}T00:00:00.000`)
  const until = new Date(`${safeEndDate}T23:59:59.999`)
  return { since: since.toISOString(), until: until.toISOString() }
}

function metricEntries(value: string | null): Array<[string, unknown]> {
  if (!value) {
    return []
  }
  try {
    const parsed = JSON.parse(value) as Record<string, unknown>
    return Object.entries(parsed)
  } catch {
    return [['raw', value]]
  }
}

function metricLabel(key: string): string {
  const labels: Record<string, string> = {
    air_util_tx: '空口发送占用',
    battery_level: '电量',
    channel_utilization: '信道占用',
    uptime_seconds: '运行时长',
    voltage: '电压',
  }
  return labels[key] || key
}

function metricValue(key: string, value: unknown): string {
  if (typeof value !== 'number') {
    return String(value)
  }
  if (key === 'battery_level') {
    return `${value}%`
  }
  if (key === 'voltage') {
    return `${value.toFixed(2)} V`
  }
  if (key === 'air_util_tx' || key === 'channel_utilization') {
    return `${value.toFixed(2)}%`
  }
  if (key === 'uptime_seconds') {
    const hours = Math.floor(value / 3600)
    const minutes = Math.floor((value % 3600) / 60)
    const seconds = Math.floor(value % 60)
    return `${hours}h ${minutes}m ${seconds}s`
  }
  return Number.isInteger(value) ? String(value) : value.toFixed(2)
}

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

function messagesForDelete(message: GroupedTextMessage): TextMessage[] {
  return Array.from(new Map(message.mergedMessages.map((item) => [item.id, item])).values())
}

function deleteMessageCount(message: GroupedTextMessage): number {
  return messagesForDelete(message).length
}

async function optional<T>(request: Promise<T>): Promise<T | null> {
  try {
    return await request
  } catch {
    return null
  }
}

function canTelemetryPrev(): boolean {
  return telemetryPage.value > 1
}

function canTelemetryNext(): boolean {
  return telemetry.value.length === telemetryPageSize
}

async function loadTelemetryPage() {
  telemetryLoading.value = true
  try {
    const response = await getTelemetry(telemetryPageSize, (telemetryPage.value - 1) * telemetryPageSize, props.nodeId)
    telemetry.value = response.items
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    telemetryLoading.value = false
  }
}

function changeTelemetryPage(nextPage: number) {
  telemetryPage.value = Math.max(1, nextPage)
  loadTelemetryPage()
}

async function loadTrajectoryRange() {
  const range = localDateRange(trajectoryStartDate.value, trajectoryEndDate.value)
  if (!range) {
    return
  }

  trajectoryLoading.value = true
  trajectoryError.value = ''
  trajectoryTruncated.value = false
  positions.value = []
  try {
    const items: PositionRecord[] = []
    for (let offset = 0; offset < maxTrajectoryPoints; offset += trajectoryPageSize) {
      const response = await getPositions(trajectoryPageSize, offset, {
        nodeId: props.nodeId,
        since: range.since,
        until: range.until,
      })
      items.push(...response.items)
      if (response.items.length < trajectoryPageSize) {
        break
      }
      if (items.length >= maxTrajectoryPoints) {
        trajectoryTruncated.value = true
        break
      }
    }
    positions.value = items.slice(0, maxTrajectoryPoints)
  } catch (err) {
    trajectoryError.value = err instanceof Error ? err.message : String(err)
  } finally {
    trajectoryLoading.value = false
  }
}

function applyTodayTrajectory() {
  const today = toDateInputValue()
  trajectoryStartDate.value = today
  trajectoryEndDate.value = today
  loadTrajectoryRange()
}

async function loadInitialMessages() {
  const response = await getTextMessages(chatPageSize, 0, props.nodeId)
  messages.value = toChronological(response.items)
  chatHasMore.value = response.items.length === chatPageSize
  await nextTick()
  const el = chatHistoryRef.value
  if (el) {
    el.scrollTop = el.scrollHeight
    await loadMoreUntilScrollable(el)
  }
}

async function loadOlderMessages() {
  const el = chatHistoryRef.value
  await loadOlderMessagesFromCurrentScroll(el)
}

async function loadOlderMessagesFromCurrentScroll(el: HTMLElement | null) {
  if (chatLoadingOlder.value || !chatHasMore.value) {
    return
  }

  const previousScrollHeight = el?.scrollHeight ?? 0
  const previousScrollTop = el?.scrollTop ?? 0
  const previousGroupedMessageCount = groupedMessages.value.length
  chatLoadingOlder.value = true
  try {
    const response = await getTextMessages(chatPageSize, messages.value.length, props.nodeId)
    messages.value = mergeMessages(messages.value, toChronological(response.items))
    chatHasMore.value = response.items.length === chatPageSize
    await nextTick()
    if (el && groupedMessages.value.length > previousGroupedMessageCount) {
      el.scrollTop = el.scrollHeight - previousScrollHeight + previousScrollTop
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    chatLoadingOlder.value = false
  }
}

async function loadMoreUntilScrollable(el: HTMLElement) {
  while (chatHasMore.value && el.scrollHeight <= el.clientHeight + scrollOverflowAllowance) {
    const previousGroupedMessageCount = groupedMessages.value.length
    await loadOlderMessagesFromCurrentScroll(el)
    if (groupedMessages.value.length <= previousGroupedMessageCount) {
      break
    }
  }
}

function closeMessageMenu() {
  menuMessage.value = null
}

function openMessageMenu(message: GroupedTextMessage, event: MouseEvent) {
  if (!props.isAdmin) {
    return
  }
  menuMessage.value = message
  menuX.value = event.clientX
  menuY.value = event.clientY
}

function deleteSelectedMessage() {
  if (!menuMessage.value) {
    return
  }
  pendingDeleteAction.value = { kind: 'delete-message', message: menuMessage.value }
  closeMessageMenu()
}

async function performDeleteMessage(message: GroupedTextMessage) {
  try {
    await deleteMessagesFromLocalState(message)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
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

async function deleteMessagesFromLocalState(message: GroupedTextMessage) {
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

function deleteAndBlockSelectedMessageNode() {
  if (!menuMessage.value) {
    return
  }
  const message = menuMessage.value
  pendingDeleteAction.value = {
    kind: 'delete-and-block-node',
    message,
    nodeId: message.from_id || props.nodeId,
    nodeNum: message.from_num ?? mergedNode.value.node_num ?? null,
  }
  closeMessageMenu()
}

async function performDeleteAndBlockMessageNode(payload: { message: GroupedTextMessage; nodeId: string; nodeNum: number | null; reason: string }) {
  try {
    await deleteMessagesFromLocalState(payload.message)

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
    if (payload.nodeId === props.nodeId) {
      nodeInfo.value = null
      mapReport.value = null
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function confirmDeleteModal(payload: { reason?: string }) {
  const action = pendingDeleteAction.value
  pendingDeleteAction.value = null
  if (!action) {
    return
  }

  if (action.kind === 'delete-message') {
    await performDeleteMessage(action.message)
    return
  }

  const reason = payload.reason?.trim()
  if (!reason) {
    return
  }
  await performDeleteAndBlockMessageNode({
    message: action.message,
    nodeId: action.nodeId,
    nodeNum: action.nodeNum,
    reason,
  })
}

function cancelDeleteModal() {
  pendingDeleteAction.value = null
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeMessageMenu()
  }
}

function handleChatScroll() {
  closeMessageMenu()
  const el = chatHistoryRef.value
  if (!el || el.scrollTop > 8) {
    return
  }
  loadOlderMessages()
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

async function loadDetails() {
  loading.value = true
  error.value = ''
  trajectoryError.value = ''
  telemetryPage.value = 1
  try {
    const [nodeData, reportData] = await Promise.all([
      optional(getNodeInfoById(props.nodeId)),
      optional(getMapReportById(props.nodeId)),
    ])
    nodeInfo.value = nodeData
    mapReport.value = reportData
    await Promise.all([
      loadTrajectoryRange(),
      loadTelemetryPage(),
      loadInitialMessages(),
    ])
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  window.addEventListener('click', closeMessageMenu)
  window.addEventListener('keydown', handleKeydown)
  loadMapSource()
  loadDetails()
})

onBeforeUnmount(() => {
  window.removeEventListener('click', closeMessageMenu)
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <section class="detail-page">
    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Node detail</p>
          <h2>{{ nodeTitle }}</h2>
        </div>
        <span class="badge">{{ nodeId }}</span>
      </div>
      <p v-if="error" class="error">{{ error }}</p>
      <div v-if="loading" class="empty">正在加载节点详情...</div>
      <div v-else class="detail-summary-grid">
        <div><span>Node ID</span><strong>{{ nodeId }}</strong></div>
        <div><span>Node Num</span><strong>{{ mergedNode.node_num ?? '-' }}</strong></div>
        <div><span>Long Name</span><strong>{{ mergedNode.long_name || '-' }}</strong></div>
        <div><span>Short Name</span><strong>{{ mergedNode.short_name || '-' }}</strong></div>
        <div><span>硬件</span><strong>{{ mergedNode.hw_model || '-' }}</strong></div>
        <div><span>角色</span><strong>{{ mergedNode.role || '-' }}</strong></div>
        <div><span>User ID</span><strong>{{ nodeInfo?.user_id || '-' }}</strong></div>
        <div><span>授权</span><strong>{{ nodeInfo?.is_licensed ?? '-' }}</strong></div>
        <div><span>固件版本</span><strong>{{ mapReport?.firmware_version || '-' }}</strong></div>
        <div><span>区域</span><strong>{{ mapReport?.region || '-' }}</strong></div>
        <div><span>调制预设</span><strong>{{ mapReport?.modem_preset || '-' }}</strong></div>
        <div><span>最新坐标</span><strong>{{ mapReport?.latitude ?? '-' }}, {{ mapReport?.longitude ?? '-' }}</strong></div>
        <div><span>海拔</span><strong>{{ mapReport?.altitude ?? '-' }}</strong></div>
        <div><span>位置精度</span><strong>{{ mapReport?.position_precision ?? '-' }}</strong></div>
        <div><span>在线节点</span><strong>{{ mapReport?.num_online_local_nodes ?? '-' }}</strong></div>
        <div><span>上报位置</span><strong>{{ mapReport?.has_opted_report_location ?? '-' }}</strong></div>
        <div><span>更新时间</span><strong>{{ mergedNode.updated_at ? formatTime(mergedNode.updated_at) : '-' }}</strong></div>
      </div>
    </div>

    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Public Key</p>
          <h2>节点公钥</h2>
        </div>
      </div>
      <pre class="public-key-block">{{ nodeInfo?.public_key || '-' }}</pre>
    </div>

    <div class="detail-section-grid">
      <div class="panel detail-chat-panel">
        <div class="panel-header">
          <div>
            <p class="eyebrow">Chat</p>
            <h2>历史聊天记录：{{ nodeTitle }}</h2>
          </div>
          <span class="badge">{{ groupedMessages.length }}</span>
        </div>
        <div ref="chatHistoryRef" class="detail-chat-history" @scroll.passive="handleChatScroll">
          <div v-if="chatLoadingOlder" class="chat-loading">正在加载更早消息...</div>
          <div v-else-if="!chatHasMore && messages.length > 0" class="chat-end">没有更多历史消息</div>
          <div v-if="messages.length === 0" class="empty">暂无聊天记录</div>
          <div
            v-for="message in groupedMessages"
            :key="message.id"
            class="detail-chat-item"
            @contextmenu.prevent.stop="openMessageMenu(message, $event)"
          >
            <span class="chat-meta">
              <strong>{{ formatTime(message.created_at) }}</strong>
              <small>{{ message.topic }}</small>
            </span>
            <span class="chat-text">
              {{ message.text || '[binary]' }}
              <span v-if="message.mergedCount > 1" class="message-merge-count">x{{ message.mergedCount }}</span>
            </span>
          </div>
        </div>
        <div
          v-if="menuMessage"
          class="context-menu"
          :style="{ left: `${menuX}px`, top: `${menuY}px` }"
          @click.stop
        >
          <button class="danger" type="button" @click="deleteSelectedMessage">删除</button>
          <button class="danger" type="button" @click="deleteAndBlockSelectedMessageNode">删除并屏蔽节点</button>
        </div>
      </div>

      <div class="panel">
        <div class="panel-header">
          <div>
            <p class="eyebrow">Trajectory</p>
            <h2>地图轨迹：{{ nodeTitle }}</h2>
          </div>
          <span class="badge">{{ positions.length }}</span>
        </div>
        <div class="trajectory-toolbar">
          <label class="trajectory-date-field">
            <span>开始日期</span>
            <input v-model="trajectoryStartDate" type="date" :disabled="trajectoryLoading" />
          </label>
          <label class="trajectory-date-field">
            <span>结束日期</span>
            <input v-model="trajectoryEndDate" type="date" :disabled="trajectoryLoading" />
          </label>
          <button type="button" :disabled="trajectoryLoading" @click="loadTrajectoryRange">
            {{ trajectoryLoading ? '查询中...' : '查询轨迹' }}
          </button>
          <button type="button" :disabled="trajectoryLoading" @click="applyTodayTrajectory">今天</button>
        </div>
        <p v-if="trajectoryError" class="error trajectory-status">{{ trajectoryError }}</p>
        <p v-else-if="trajectoryTruncated" class="trajectory-status">轨迹点较多，仅显示前 {{ maxTrajectoryPoints }} 条，请缩小日期范围。</p>
        <p v-else-if="trajectoryLoading" class="trajectory-status">正在加载轨迹...</p>
        <NodeTrajectoryMap
          :positions="positions"
          :map-source="mapSource"
          :map-sources="mapSources"
          @map-source-change="selectMapSource"
        />
      </div>
    </div>

    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Telemetry</p>
          <h2>遥测数据：{{ nodeTitle }}</h2>
        </div>
        <span class="badge">本页 {{ telemetry.length }}</span>
      </div>
      <div v-if="telemetryLoading" class="admin-loading">正在加载遥测数据...</div>
      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>时间</th>
              <th>类型</th>
              <th>指标</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in telemetry" :key="item.id">
              <td>{{ formatTime(item.created_at) }}</td>
              <td>{{ item.telemetry_type || '-' }}</td>
              <td>
                <div class="metrics-grid">
                  <div v-for="[key, value] in metricEntries(item.metrics_json)" :key="key" class="metric-chip">
                    <span>{{ metricLabel(key) }}</span>
                    <strong>{{ metricValue(key, value) }}</strong>
                  </div>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-if="telemetry.length === 0" class="empty">暂无遥测数据</div>
      </div>
      <div class="pagination">
        <button :disabled="telemetryLoading || !canTelemetryPrev()" @click="changeTelemetryPage(telemetryPage - 1)">上一页</button>
        <span>第 {{ telemetryPage }} 页</span>
        <span>每页 {{ telemetryPageSize }} 条</span>
        <button :disabled="telemetryLoading || !canTelemetryNext()" @click="changeTelemetryPage(telemetryPage + 1)">下一页</button>
      </div>
    </div>

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
  </section>
</template>
