<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { deleteLLMMessage, deleteLLMMessagesByBot, getAdminRuntimeSettings, getBotNodes, getLLMMessages, updateAdminRuntimeSettings } from '../api'
import type { AdminRuntimeSettings, BotNode, LLMMessage, ListResponse } from '../types'

const loading = ref(false)
const savingSettings = ref(false)
const error = ref('')
const messages = ref<LLMMessage[]>([])
const botNodes = ref<BotNode[]>([])
const total = ref(0)
const limit = 50
const offset = ref(0)
const selectedBotId = ref<number | ''>('')
const includeDeleted = ref(false)
const settings = ref<AdminRuntimeSettings | null>(null)

const statusColors: Record<string, string> = {
  pending: 'background-color: #fff3cd;',
  processing: 'background-color: #cfe2ff;',
  processed: 'background-color: #d1e7dd;',
  error: 'background-color: #f8d7da;',
}

const statusLabels: Record<string, string> = {
  pending: '待处理',
  processing: '处理中',
  processed: '已处理',
  error: '错误',
}

async function loadSettings() {
  try {
    const response = await getAdminRuntimeSettings()
    settings.value = response.item
  } catch (err) {
    console.error('加载设置失败:', err)
  }
}

async function toggleQueueEnabled() {
  if (!settings.value || savingSettings.value) return
  savingSettings.value = true
  try {
    await updateAdminRuntimeSettings({
      allow_encrypted_forwarding: settings.value.allow_encrypted_forwarding,
      llm_queue_enabled: !settings.value.llm_queue_enabled,
      llm_include_channel_messages: settings.value.llm_include_channel_messages,
    })
    settings.value.llm_queue_enabled = !settings.value.llm_queue_enabled
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    savingSettings.value = false
  }
}

async function toggleIncludeChannel() {
  if (!settings.value || savingSettings.value) return
  savingSettings.value = true
  try {
    await updateAdminRuntimeSettings({
      allow_encrypted_forwarding: settings.value.allow_encrypted_forwarding,
      llm_queue_enabled: settings.value.llm_queue_enabled,
      llm_include_channel_messages: !settings.value.llm_include_channel_messages,
    })
    settings.value.llm_include_channel_messages = !settings.value.llm_include_channel_messages
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    savingSettings.value = false
  }
}

function getMessageType(msg: LLMMessage): string {
  return msg.bot_id === 0 ? '频道' : '私聊'
}

async function loadBotNodes() {
  try {
    const response = await getBotNodes(100, 0)
    botNodes.value = response.items
  } catch (err) {
    console.error('加载机器人列表失败:', err)
  }
}

async function loadMessages(resetOffset = false) {
  if (resetOffset) {
    offset.value = 0
  }
  loading.value = true
  error.value = ''
  try {
    const botId = selectedBotId.value === '' ? undefined : selectedBotId.value
    const response: ListResponse<LLMMessage> = await getLLMMessages(limit, offset.value, botId, includeDeleted.value)
    messages.value = response.items
    total.value = response.total ?? response.items.length
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function handleDeleteMessage(id: number) {
  if (!confirm('确定要删除这条消息吗？')) {
    return
  }
  try {
    await deleteLLMMessage(id)
    await loadMessages()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function handleDeleteAllByBot() {
  const botId = selectedBotId.value === '' ? undefined : selectedBotId.value
  if (!botId) {
    alert('请先选择机器人')
    return
  }
  if (!confirm(`确定要删除该机器人的所有队列消息吗？`)) {
    return
  }
  try {
    await deleteLLMMessagesByBot(botId)
    await loadMessages()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

function formatTime(timeStr: string | null): string {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

const displayFrom = computed(() => offset.value + 1)
const displayTo = computed(() => Math.min(offset.value + limit, total.value))
const hasMore = computed(() => offset.value + limit < total.value)
const hasPrev = computed(() => offset.value > 0)

function goPrev() {
  if (hasPrev.value) {
    offset.value = Math.max(0, offset.value - limit)
    loadMessages()
  }
}

function goNext() {
  if (hasMore.value) {
    offset.value += limit
    loadMessages()
  }
}

onMounted(() => {
  loadSettings()
  loadBotNodes()
  loadMessages()
})
</script>

<template>
  <div class="admin-llm">
    <h2>LLM 消息队列</h2>

    <div class="admin-llm-toolbar">
      <div class="admin-llm-filter">
        <label>
          <input type="checkbox" :checked="settings?.llm_queue_enabled ?? false" @change="toggleQueueEnabled" :disabled="savingSettings" />
          启用 LLM 消息队列
        </label>
      </div>

      <div class="admin-llm-filter">
        <label>
          <input type="checkbox" :checked="settings?.llm_include_channel_messages ?? false" @change="toggleIncludeChannel" :disabled="savingSettings || !settings?.llm_queue_enabled" />
          包含频道消息
        </label>
      </div>

      <div class="admin-llm-filter">
        <label>选择机器人：</label>
        <select v-model="selectedBotId" @change="loadMessages(true)">
          <option :value="''">全部</option>
          <option v-for="bot in botNodes" :key="bot.id" :value="bot.id">
            {{ bot.long_name }} ({{ bot.node_id }})
          </option>
        </select>
      </div>

      <div class="admin-llm-filter">
        <label>
          <input type="checkbox" v-model="includeDeleted" @change="loadMessages(true)" />
          包含已删除
        </label>
      </div>

      <button class="admin-button admin-button-danger" @click="handleDeleteAllByBot" :disabled="!selectedBotId">
        删除该机器人所有消息
      </button>

      <button class="admin-button" @click="() => loadMessages()">刷新</button>
    </div>

    <p v-if="error" class="error">{{ error }}</p>

    <div class="admin-llm-stats">
      共 {{ total }} 条消息，当前显示 {{ displayFrom }} - {{ displayTo }}
    </div>

    <div v-if="loading" class="admin-loading">加载中...</div>

    <table v-else class="admin-llm-table">
      <thead>
        <tr>
          <th>ID</th>
          <th>类型</th>
          <th>状态</th>
          <th>来自节点</th>
          <th>频道</th>
          <th>消息内容</th>
          <th>接收时间</th>
          <th>处理时间</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="msg in messages" :key="msg.id" :style="statusColors[msg.status]">
          <td>{{ msg.id }}</td>
          <td>
            <span class="type-badge" :class="getMessageType(msg) === '频道' ? 'channel' : 'direct'">
              {{ getMessageType(msg) }}
            </span>
          </td>
          <td>
            <span class="status-badge" :style="statusColors[msg.status]">
              {{ statusLabels[msg.status] || msg.status }}
            </span>
          </td>
          <td>
            <div class="node-info">
              <div class="node-name">{{ msg.long_name || msg.short_name || '-' }}</div>
              <div class="node-id">{{ msg.from_node_id }}</div>
            </div>
          </td>
          <td class="channel-cell">{{ msg.channel_id || (msg.bot_id === 0 ? '默认频道' : '-') }}</td>
          <td class="message-text">{{ msg.text }}</td>
          <td>{{ formatTime(msg.received_at) }}</td>
          <td>{{ formatTime(msg.processed_at) }}</td>
          <td>
            <button class="admin-button admin-button-small admin-button-danger" @click="handleDeleteMessage(msg.id)">
              删除
            </button>
          </td>
        </tr>
        <tr v-if="messages.length === 0">
          <td colspan="9" class="empty-state">暂无消息</td>
        </tr>
      </tbody>
    </table>

    <div class="admin-llm-pagination">
      <button class="admin-button admin-button-small" @click="goPrev" :disabled="!hasPrev">上一页</button>
      <span class="pagination-info">第 {{ Math.floor(offset / limit) + 1 }} 页</span>
      <button class="admin-button admin-button-small" @click="goNext" :disabled="!hasMore">下一页</button>
    </div>
  </div>
</template>

<style scoped>
.admin-llm {
  padding: 1rem;
  max-width: 100%;
}

.admin-llm h2 {
  margin: 0 0 1.5rem;
  font-size: 1.5rem;
}

.admin-llm-toolbar {
  display: flex;
  gap: 1rem;
  align-items: center;
  margin-bottom: 1rem;
  flex-wrap: wrap;
}

.admin-llm-filter {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.admin-llm-filter label {
  font-size: 0.9rem;
  color: #666;
}

.admin-llm-filter select {
  padding: 0.5rem;
  border: 1px solid #ddd;
  border-radius: 4px;
  min-width: 200px;
}

.admin-llm-stats {
  padding: 0.75rem;
  background: #f8f9fa;
  border-radius: 4px;
  margin-bottom: 1rem;
  font-size: 0.9rem;
  color: #666;
}

.admin-loading {
  padding: 2rem;
  text-align: center;
  color: #666;
}

.type-badge {
  display: inline-block;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  font-size: 0.8rem;
  font-weight: 500;
}

.type-badge.channel {
  background-color: #e2e8f0;
  color: #475569;
}

.type-badge.direct {
  background-color: #fce7f3;
  color: #be185d;
}

.channel-cell {
  font-family: monospace;
  font-size: 0.8rem;
  color: #64748b;
  max-width: 100px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.admin-llm-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9rem;
}

.admin-llm-table th,
.admin-llm-table td {
  padding: 0.75rem;
  text-align: left;
  border-bottom: 1px solid #eee;
}

.admin-llm-table th {
  background: #f8f9fa;
  font-weight: 600;
  position: sticky;
  top: 0;
}

.admin-llm-table tr:hover {
  background: rgba(0, 0, 0, 0.02);
}

.status-badge {
  display: inline-block;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  font-size: 0.8rem;
  font-weight: 500;
}

.node-info {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.node-name {
  font-weight: 500;
}

.node-id {
  font-size: 0.8rem;
  color: #666;
  font-family: monospace;
}

.message-text {
  max-width: 400px;
  word-break: break-word;
  line-height: 1.4;
}

.empty-state {
  text-align: center;
  padding: 3rem !important;
  color: #999;
}

.admin-llm-pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  margin-top: 1.5rem;
  padding-top: 1rem;
  border-top: 1px solid #eee;
}

.pagination-info {
  color: #666;
  font-size: 0.9rem;
}

.error {
  color: #dc3545;
  padding: 0.75rem;
  background: #f8d7da;
  border-radius: 4px;
  margin-bottom: 1rem;
}

.admin-button-small {
  padding: 0.25rem 0.5rem;
  font-size: 0.8rem;
}

.admin-button-danger {
  background: #dc3545;
  color: white;
}

.admin-button-danger:hover:not(:disabled) {
  background: #c82333;
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
