<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { createNodeBlockingRule, deleteNode, deleteTextMessage, getMapReportById, getNodeInfoById, getPositions, getTelemetry, getTextMessages } from '../api'
import type { MapReport, NodeInfo, PositionRecord, TelemetryRecord, TextMessage } from '../types'
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
const loading = ref(true)
const chatLoadingOlder = ref(false)
const chatHasMore = ref(true)
const error = ref('')
const chatPageSize = 20
const chatHistoryRef = ref<HTMLElement | null>(null)
const menuMessage = ref<TextMessage | null>(null)
const menuX = ref(0)
const menuY = ref(0)

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

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
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

async function optional<T>(request: Promise<T>): Promise<T | null> {
  try {
    return await request
  } catch {
    return null
  }
}

async function loadInitialMessages() {
  const response = await getTextMessages(chatPageSize, 0, props.nodeId)
  messages.value = toChronological(response.items)
  chatHasMore.value = response.items.length === chatPageSize
  await nextTick()
  const el = chatHistoryRef.value
  if (el) {
    el.scrollTop = el.scrollHeight
  }
}

async function loadOlderMessages() {
  if (chatLoadingOlder.value || !chatHasMore.value) {
    return
  }

  const el = chatHistoryRef.value
  const previousScrollHeight = el?.scrollHeight ?? 0
  const previousScrollTop = el?.scrollTop ?? 0
  chatLoadingOlder.value = true
  try {
    const response = await getTextMessages(chatPageSize, messages.value.length, props.nodeId)
    messages.value = mergeMessages(messages.value, toChronological(response.items))
    chatHasMore.value = response.items.length === chatPageSize
    await nextTick()
    if (el) {
      el.scrollTop = el.scrollHeight - previousScrollHeight + previousScrollTop
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    chatLoadingOlder.value = false
  }
}

function closeMessageMenu() {
  menuMessage.value = null
}

function openMessageMenu(message: TextMessage, event: MouseEvent) {
  if (!props.isAdmin) {
    return
  }
  menuMessage.value = message
  menuX.value = event.clientX
  menuY.value = event.clientY
}

async function deleteSelectedMessage() {
  if (!menuMessage.value) {
    return
  }
  const message = menuMessage.value
  closeMessageMenu()
  try {
    await deleteTextMessage(message.id)
    messages.value = messages.value.filter((item) => item.id !== message.id)
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

async function deleteAndBlockSelectedMessageNode() {
  if (!menuMessage.value) {
    return
  }
  const message = menuMessage.value
  const nodeId = message.from_id || props.nodeId
  const nodeNum = message.from_num ?? mergedNode.value.node_num ?? null
  closeMessageMenu()
  try {
    try {
      await deleteTextMessage(message.id)
    } catch (err) {
      if (!isMessageNotFoundError(err)) {
        throw err
      }
    }
    messages.value = messages.value.filter((item) => item.id !== message.id)

    try {
      await createNodeBlockingRule({
        node_id: nodeId,
        node_num: nodeNum,
        reason: '管理员右键删除并屏蔽节点',
        enabled: true,
      })
    } catch (err) {
      if (!isAlreadyBlockedError(err)) {
        throw err
      }
    }

    try {
      await deleteNode(nodeId)
    } catch (err) {
      if (!isNodeNotFoundError(err)) {
        throw err
      }
    }
    if (nodeId === props.nodeId) {
      nodeInfo.value = null
      mapReport.value = null
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
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

async function loadDetails() {
  loading.value = true
  error.value = ''
  try {
    const [nodeData, reportData, positionData, telemetryData] = await Promise.all([
      optional(getNodeInfoById(props.nodeId)),
      optional(getMapReportById(props.nodeId)),
      getPositions(500, 0, props.nodeId),
      getTelemetry(200, 0, props.nodeId),
    ])
    nodeInfo.value = nodeData
    mapReport.value = reportData
    positions.value = positionData.items
    telemetry.value = telemetryData.items
    await loadInitialMessages()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  window.addEventListener('click', closeMessageMenu)
  window.addEventListener('keydown', handleKeydown)
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
            <h2>历史聊天记录</h2>
          </div>
          <span class="badge">{{ messages.length }}</span>
        </div>
        <div ref="chatHistoryRef" class="detail-chat-history" @scroll.passive="handleChatScroll">
          <div v-if="chatLoadingOlder" class="chat-loading">正在加载更早消息...</div>
          <div v-else-if="!chatHasMore && messages.length > 0" class="chat-end">没有更多历史消息</div>
          <div v-if="messages.length === 0" class="empty">暂无聊天记录</div>
          <div
            v-for="message in messages"
            :key="message.id"
            class="detail-chat-item"
            @contextmenu.prevent.stop="openMessageMenu(message, $event)"
          >
            <span class="chat-meta">
              <strong>{{ formatTime(message.created_at) }}</strong>
              <small>{{ message.topic }}</small>
            </span>
            <span class="chat-text">{{ message.text || '[binary]' }}</span>
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
            <h2>地图轨迹</h2>
          </div>
          <span class="badge">{{ positions.length }}</span>
        </div>
        <NodeTrajectoryMap :positions="positions" />
      </div>
    </div>

    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Telemetry</p>
          <h2>遥测数据</h2>
        </div>
        <span class="badge">{{ telemetry.length }}</span>
      </div>
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
    </div>
  </section>
</template>
