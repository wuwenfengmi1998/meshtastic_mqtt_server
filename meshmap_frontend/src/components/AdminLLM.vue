<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { deleteLLMMessage, deleteLLMMessagesByBot, getBotNodes, getLLMMessages, updateBotNode } from '../api'
import type { BotNode, LLMMessage, ListResponse } from '../types'

const loading = ref(false)
const savingBotId = ref<number | null>(null)
const error = ref('')
const messages = ref<LLMMessage[]>([])
const botNodes = ref<BotNode[]>([])
const total = ref(0)
const limit = 50
const offset = ref(0)
const selectedBotId = ref<number | ''>('')
const includeDeleted = ref(false)

const statusColors: Record<string, string> = {
  pending: 'background: linear-gradient(135deg, #fef3c7 0%, #fde68a33 100%);',
  processing: 'background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe33 100%);',
  processed: 'background: linear-gradient(135deg, #dcfce7 0%, #bbf7d033 100%);',
  error: 'background: linear-gradient(135deg, #fee2e2 0%, #fecaca33 100%);',
}

const statusLabels: Record<string, string> = {
  pending: '待处理',
  processing: '处理中',
  processed: '已处理',
  error: '错误',
}

function getMessageType(msg: LLMMessage): string {
  return msg.channel_id ? '频道' : '私聊'
}

function getBotName(botId: number): string {
  const bot = botNodes.value.find(b => b.id === botId)
  return bot ? bot.long_name : '-'
}

async function loadBotNodes() {
  try {
    const response = await getBotNodes(100, 0)
    botNodes.value = response.items
  } catch (err) {
    console.error('加载机器人列表失败:', err)
  }
}

async function toggleBotLLMQueue(bot: BotNode) {
  savingBotId.value = bot.id
  try {
    await updateBotNode(bot.id, {
      long_name: bot.long_name,
      short_name: bot.short_name,
      enabled: bot.enabled,
      default_channel_id: bot.default_channel_id,
      topic_prefix: bot.topic_prefix,
      psk: bot.psk,
      nodeinfo_broadcast_enabled: bot.nodeinfo_broadcast_enabled,
      nodeinfo_broadcast_interval_seconds: bot.nodeinfo_broadcast_interval_seconds,
      llm_queue_enabled: !bot.llm_queue_enabled,
      llm_include_channel_messages: bot.llm_include_channel_messages,
    })
    bot.llm_queue_enabled = !bot.llm_queue_enabled
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    savingBotId.value = null
  }
}

async function toggleBotIncludeChannel(bot: BotNode) {
  savingBotId.value = bot.id
  try {
    await updateBotNode(bot.id, {
      long_name: bot.long_name,
      short_name: bot.short_name,
      enabled: bot.enabled,
      default_channel_id: bot.default_channel_id,
      topic_prefix: bot.topic_prefix,
      psk: bot.psk,
      nodeinfo_broadcast_enabled: bot.nodeinfo_broadcast_enabled,
      nodeinfo_broadcast_interval_seconds: bot.nodeinfo_broadcast_interval_seconds,
      llm_queue_enabled: bot.llm_queue_enabled,
      llm_include_channel_messages: !bot.llm_include_channel_messages,
    })
    bot.llm_include_channel_messages = !bot.llm_include_channel_messages
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    savingBotId.value = null
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
  loadBotNodes()
  loadMessages()
})
</script>

<template>
  <div class="admin-llm">
    <h2>LLM 消息队列</h2>

    <div class="admin-llm-section">
      <h3>机器人 LLM 设置</h3>
      <p class="section-desc">每个机器人可以独立启用或禁用 LLM 消息队列。启用后，该机器人收到的私聊消息将被加入队列。</p>

      <div class="bot-settings-grid">
        <div v-for="bot in botNodes" :key="bot.id" class="bot-settings-card">
          <div class="bot-header">
            <strong>{{ bot.long_name }}</strong>
            <span class="bot-node-id">{{ bot.node_id }}</span>
          </div>
          <div class="bot-settings">
            <label class="setting-item">
              <input
                type="checkbox"
                :checked="bot.llm_queue_enabled"
                @change="toggleBotLLMQueue(bot)"
                :disabled="savingBotId === bot.id"
              />
              <span>启用 LLM 队列</span>
            </label>
            <label class="setting-item">
              <input
                type="checkbox"
                :checked="bot.llm_include_channel_messages"
                @change="toggleBotIncludeChannel(bot)"
                :disabled="savingBotId === bot.id || !bot.llm_queue_enabled"
              />
              <span>包含频道消息</span>
            </label>
          </div>
          <div v-if="savingBotId === bot.id" class="saving-indicator">保存中...</div>
        </div>
      </div>
    </div>

    <div class="admin-llm-toolbar">
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

    <div v-if="loading" class="admin-loading">
      <div style="margin-bottom: 1rem;">🔄</div>
      加载中...
    </div>

    <div v-else class="admin-llm-table-wrapper">
      <table class="admin-llm-table">
      <thead>
        <tr>
          <th>ID</th>
          <th>机器人</th>
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
          <td class="bot-name-cell">
            <div class="bot-info">
              <div class="bot-long-name">{{ getBotName(msg.bot_id) }}</div>
              <div class="bot-node-id" v-if="msg.bot_id !== 0">{{ msg.bot_node_id }}</div>
            </div>
          </td>
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
          <td class="channel-cell">{{ msg.channel_id || '-' }}</td>
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
          <td colspan="10" class="empty-state">暂无消息</td>
        </tr>
      </tbody>
    </table>
    </div>

    <div class="admin-llm-pagination">
      <button class="admin-button admin-button-small" @click="goPrev" :disabled="!hasPrev">上一页</button>
      <span class="pagination-info">第 {{ Math.floor(offset / limit) + 1 }} 页</span>
      <button class="admin-button admin-button-small" @click="goNext" :disabled="!hasMore">下一页</button>
    </div>
  </div>
</template>

<style scoped>
.admin-llm {
  padding: 1.5rem;
  max-width: 100%;
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
  min-height: 100vh;
}

.admin-llm h2 {
  margin: 0 0 2rem;
  font-size: 1.75rem;
  font-weight: 700;
  color: #1e293b;
  letter-spacing: -0.02em;
}

.admin-llm-section {
  background: white;
  padding: 1.75rem;
  border-radius: 16px;
  margin-bottom: 2rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05), 0 4px 6px rgba(0, 0, 0, 0.03);
  border: 1px solid #e2e8f0;
}

.admin-llm-section h3 {
  margin: 0 0 0.75rem;
  font-size: 1.25rem;
  font-weight: 600;
  color: #334155;
}

.section-desc {
  margin: 0 0 1.5rem;
  color: #64748b;
  font-size: 0.95rem;
  line-height: 1.5;
}

.bot-settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 1.25rem;
}

.bot-settings-card {
  background: linear-gradient(135deg, #fafbfc 0%, #f8fafc 100%);
  padding: 1.25rem;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
  transition: all 0.2s ease;
  position: relative;
  overflow: hidden;
}

.bot-settings-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: linear-gradient(90deg, #3b82f6, #8b5cf6);
  opacity: 0;
  transition: opacity 0.2s ease;
}

.bot-settings-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.08);
  border-color: #cbd5e1;
}

.bot-settings-card:hover::before {
  opacity: 1;
}

.bot-header {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin-bottom: 1rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid #e2e8f0;
}

.bot-header strong {
  font-size: 1.05rem;
  color: #1e293b;
  font-weight: 600;
}

.bot-node-id {
  font-size: 0.8rem;
  color: #64748b;
  font-family: 'SF Mono', 'Monaco', 'Inconsolata', monospace;
  background: #f1f5f9;
  padding: 0.2rem 0.5rem;
  border-radius: 4px;
  display: inline-block;
  width: fit-content;
}

.bot-settings {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.setting-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  cursor: pointer;
  font-size: 0.9rem;
  color: #475569;
  padding: 0.5rem;
  border-radius: 8px;
  transition: background-color 0.15s ease;
}

.setting-item:hover {
  background: #f1f5f9;
}

.setting-item input[type='checkbox'] {
  cursor: pointer;
  width: 18px;
  height: 18px;
  accent-color: #3b82f6;
}

.saving-indicator {
  margin-top: 0.75rem;
  font-size: 0.8rem;
  color: #3b82f6;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.saving-indicator::before {
  content: '';
  width: 12px;
  height: 12px;
  border: 2px solid #3b82f6;
  border-top-color: transparent;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.admin-llm-toolbar {
  display: flex;
  gap: 1rem;
  align-items: center;
  margin-bottom: 1.5rem;
  flex-wrap: wrap;
  padding: 1.25rem;
  background: white;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  border: 1px solid #e2e8f0;
}

.admin-llm-filter {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.admin-llm-filter label {
  font-size: 0.9rem;
  color: #64748b;
  font-weight: 500;
}

.admin-llm-filter select {
  padding: 0.6rem 1rem;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  min-width: 220px;
  font-size: 0.9rem;
  color: #334155;
  background: white;
  cursor: pointer;
  transition: all 0.15s ease;
}

.admin-llm-filter select:hover {
  border-color: #94a3b8;
}

.admin-llm-filter select:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.admin-llm-filter input[type='checkbox'] {
  width: 16px;
  height: 16px;
  accent-color: #3b82f6;
  cursor: pointer;
}

.admin-llm-stats {
  padding: 1rem 1.25rem;
  background: linear-gradient(135deg, #eff6ff 0%, #dbeafe 100%);
  border-radius: 10px;
  margin-bottom: 1.25rem;
  font-size: 0.9rem;
  color: #1e40af;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  border: 1px solid #bfdbfe;
}

.admin-loading {
  padding: 3rem;
  text-align: center;
  color: #64748b;
  background: white;
  border-radius: 12px;
  font-size: 1rem;
}

.type-badge {
  display: inline-block;
  padding: 0.35rem 0.75rem;
  border-radius: 20px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.type-badge.channel {
  background: linear-gradient(135deg, #e0e7ff 0%, #c7d2fe 100%);
  color: #4338ca;
  border: 1px solid #a5b4fc;
}

.type-badge.direct {
  background: linear-gradient(135deg, #fce7f3 0%, #fbcfe8 100%);
  color: #be185d;
  border: 1px solid #f9a8d4;
}

.channel-cell {
  font-family: 'SF Mono', 'Monaco', 'Inconsolata', monospace;
  font-size: 0.8rem;
  color: #64748b;
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  background: #f8fafc;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  border: 1px solid #e2e8f0;
}

.bot-name-cell {
  min-width: 160px;
}

.bot-info {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.bot-long-name {
  font-weight: 600;
  color: #1e293b;
}

.node-info {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.node-name {
  font-weight: 600;
  color: #1e293b;
}

.node-id {
  font-size: 0.8rem;
  color: #64748b;
  font-family: 'SF Mono', 'Monaco', 'Inconsolata', monospace;
  background: #f1f5f9;
  padding: 0.2rem 0.5rem;
  border-radius: 4px;
  display: inline-block;
  width: fit-content;
}

.admin-llm-table-wrapper {
  overflow-x: auto;
  background: white;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  border: 1px solid #e2e8f0;
}

.admin-llm-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9rem;
  min-width: 1000px;
}

.admin-llm-table th,
.admin-llm-table td {
  padding: 1rem;
  text-align: left;
  border-bottom: 1px solid #f1f5f9;
}

.admin-llm-table th {
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
  font-weight: 600;
  color: #475569;
  position: sticky;
  top: 0;
  font-size: 0.85rem;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  border-bottom: 2px solid #e2e8f0;
}

.admin-llm-table tbody tr {
  transition: background-color 0.15s ease;
}

.admin-llm-table tbody tr:hover {
  background: #f8fafc;
}

.status-badge {
  display: inline-block;
  padding: 0.4rem 0.8rem;
  border-radius: 20px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.02em;
  border: 1px solid transparent;
}

.status-badge[style*='#fff3cd'] {
  background: linear-gradient(135deg, #fef3c7 0%, #fde68a 100%) !important;
  color: #92400e;
  border-color: #fcd34d;
}

.status-badge[style*='#cfe2ff'] {
  background: linear-gradient(135deg, #dbeafe 0%, #bfdbfe 100%) !important;
  color: #1e40af;
  border-color: #93c5fd;
}

.status-badge[style*='#d1e7dd'] {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%) !important;
  color: #166534;
  border-color: #86efac;
}

.status-badge[style*='#f8d7da'] {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%) !important;
  color: #991b1b;
  border-color: #fca5a5;
}

.message-text {
  max-width: 450px;
  word-break: break-word;
  line-height: 1.5;
  color: #334155;
  font-size: 0.875rem;
}

.empty-state {
  text-align: center;
  padding: 4rem 2rem !important;
  color: #94a3b8;
  font-size: 1rem;
}

.empty-state::before {
  content: '📭';
  display: block;
  font-size: 3rem;
  margin-bottom: 1rem;
}

.admin-llm-pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  margin-top: 1.5rem;
  padding: 1.25rem;
  background: white;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  border: 1px solid #e2e8f0;
}

.pagination-info {
  color: #64748b;
  font-size: 0.9rem;
  font-weight: 500;
  padding: 0.5rem 1rem;
  background: #f8fafc;
  border-radius: 8px;
  min-width: 80px;
  text-align: center;
}

.error {
  color: #991b1b;
  padding: 1rem 1.25rem;
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  border-radius: 10px;
  margin-bottom: 1.25rem;
  border: 1px solid #fca5a5;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.error::before {
  content: '⚠️';
}

.admin-button {
  padding: 0.6rem 1.25rem;
  border: none;
  border-radius: 8px;
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);
  color: white;
  box-shadow: 0 2px 4px rgba(59, 130, 246, 0.2);
}

.admin-button:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.3);
}

.admin-button:active:not(:disabled) {
  transform: translateY(0);
}

.admin-button-small {
  padding: 0.4rem 0.75rem;
  font-size: 0.8rem;
}

.admin-button-danger {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  box-shadow: 0 2px 4px rgba(239, 68, 68, 0.2);
}

.admin-button-danger:hover:not(:disabled) {
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.3);
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  transform: none !important;
}
</style>
