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
  <section class="admin-dashboard">
    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">LLM</p>
          <h2>机器人 LLM 设置</h2>
        </div>
      </div>
      <p class="llm-section-desc">每个机器人可以独立启用或禁用 LLM 消息队列。启用后，该机器人收到的私聊消息将被加入队列。</p>

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

    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Queue</p>
          <h2>LLM 消息队列</h2>
        </div>
        <div class="admin-actions">
          <button class="admin-button" @click="() => loadMessages()" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
        </div>
      </div>

      <div class="llm-toolbar">
        <label class="llm-filter">
          <span>选择机器人</span>
          <select v-model="selectedBotId" @change="loadMessages(true)">
            <option :value="''">全部</option>
            <option v-for="bot in botNodes" :key="bot.id" :value="bot.id">
              {{ bot.long_name }} ({{ bot.node_id }})
            </option>
          </select>
        </label>

        <label class="llm-filter llm-filter-inline">
          <input type="checkbox" v-model="includeDeleted" @change="loadMessages(true)" />
          包含已删除
        </label>

        <button class="admin-button danger" @click="handleDeleteAllByBot" :disabled="!selectedBotId">
          删除该机器人所有消息
        </button>
      </div>

      <p v-if="error" class="error">{{ error }}</p>

      <div class="llm-stats">
        共 {{ total }} 条消息，当前显示 {{ displayFrom }} - {{ displayTo }}
      </div>

      <div v-if="loading" class="empty">加载中...</div>

      <div v-else class="node-table-wrap table-fit-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th class="cell-nowrap">ID</th>
              <th>机器人</th>
              <th class="cell-nowrap">类型</th>
              <th class="cell-nowrap">状态</th>
              <th>来自节点</th>
              <th>频道</th>
              <th>消息内容</th>
              <th class="cell-nowrap">接收时间</th>
              <th class="cell-nowrap">处理时间</th>
              <th class="cell-nowrap">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="msg in messages" :key="msg.id">
              <td class="cell-nowrap">{{ msg.id }}</td>
              <td>
                <div class="bot-info">
                  <div class="bot-long-name">{{ getBotName(msg.bot_id) }}</div>
                  <div class="bot-node-id" v-if="msg.bot_id !== 0">{{ msg.bot_node_id }}</div>
                </div>
              </td>
              <td class="cell-nowrap">
                <span class="type-badge" :class="getMessageType(msg) === '频道' ? 'channel' : 'direct'">
                  {{ getMessageType(msg) }}
                </span>
              </td>
              <td class="cell-nowrap">
                <span class="status-badge" :class="`status-${msg.status}`">
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
              <td class="cell-nowrap">{{ formatTime(msg.received_at) }}</td>
              <td class="cell-nowrap">{{ formatTime(msg.processed_at) }}</td>
              <td class="cell-nowrap">
                <button
                  v-if="!msg.deleted_at"
                  class="admin-button danger"
                  @click="handleDeleteMessage(msg.id)"
                >
                  删除
                </button>
                <span v-else class="muted-tag">已删除</span>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-if="messages.length === 0" class="empty">暂无消息</div>
      </div>

      <div class="pagination">
        <button @click="goPrev" :disabled="!hasPrev">上一页</button>
        <span>第 {{ Math.floor(offset / limit) + 1 }} 页</span>
        <button @click="goNext" :disabled="!hasMore">下一页</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.llm-section-desc {
  margin: 0;
  padding: 12px 16px 0;
  color: var(--color-muted);
  font-size: 13px;
  line-height: 1.6;
}

.bot-settings-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
  padding: 16px;
}

.bot-settings-card {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: 14px;
  background: var(--color-surface-soft);
  transition: border-color 0.16s ease, box-shadow 0.16s ease, transform 0.16s ease;
}

.bot-settings-card:hover {
  border-color: var(--color-primary);
  box-shadow: var(--shadow-sm);
  transform: translateY(-1px);
}

.bot-header {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--color-border);
}

.bot-header strong {
  color: var(--color-heading);
  font-size: 15px;
}

.bot-node-id {
  display: inline-block;
  width: fit-content;
  border-radius: var(--radius-sm);
  padding: 2px 8px;
  color: var(--color-muted);
  background: var(--color-surface-muted);
  font-family: var(--font-mono);
  font-size: 12px;
}

.bot-settings {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.setting-item {
  display: flex;
  align-items: center;
  gap: 8px;
  border-radius: var(--radius-sm);
  padding: 6px 8px;
  color: var(--color-text);
  font-size: 13px;
  cursor: pointer;
  transition: background-color 0.16s ease;
}

.setting-item:hover {
  background: var(--color-primary-soft);
}

.setting-item input[type='checkbox'] {
  cursor: pointer;
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary);
}

.saving-indicator {
  margin-top: 8px;
  color: var(--color-primary);
  font-size: 12px;
}

.llm-toolbar {
  display: flex;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 12px;
  padding: 14px 16px;
  border-bottom: 1px solid var(--color-border);
}

.llm-filter {
  display: grid;
  gap: 6px;
  color: var(--color-muted);
  font-size: 12px;
  font-weight: 700;
}

.llm-filter select {
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-sm);
  padding: 9px 12px;
  min-width: 220px;
  color: var(--color-heading);
  background: var(--color-surface);
  outline: none;
  cursor: pointer;
  transition: border-color 0.16s ease, box-shadow 0.16s ease;
}

.llm-filter select:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 20%, transparent);
}

.llm-filter-inline {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 9px;
  color: var(--color-text);
  font-size: 13px;
  cursor: pointer;
}

.llm-filter-inline input[type='checkbox'] {
  width: 16px;
  height: 16px;
  accent-color: var(--color-primary);
  cursor: pointer;
}

.llm-stats {
  padding: 12px 16px;
  border-bottom: 1px solid var(--color-border);
  color: var(--color-muted);
  font-size: 13px;
}

.type-badge,
.status-badge {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.type-badge.channel {
  color: color-mix(in srgb, var(--color-primary) 78%, var(--color-heading));
  background: var(--color-primary-soft);
}

.type-badge.direct {
  color: color-mix(in srgb, var(--color-accent) 78%, var(--color-heading));
  background: color-mix(in srgb, var(--color-accent) 14%, white);
}

.status-badge.status-pending {
  color: color-mix(in srgb, var(--color-warning) 72%, var(--color-heading));
  background: var(--color-warning-soft);
}

.status-badge.status-processing {
  color: color-mix(in srgb, var(--color-primary) 78%, var(--color-heading));
  background: var(--color-primary-soft);
}

.status-badge.status-processed {
  color: color-mix(in srgb, var(--color-success) 70%, var(--color-heading));
  background: var(--color-success-soft);
}

.status-badge.status-error {
  color: color-mix(in srgb, var(--color-danger) 76%, var(--color-heading));
  background: var(--color-danger-soft);
}

.bot-info,
.node-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.bot-long-name,
.node-name {
  color: var(--color-heading);
  font-weight: 700;
}

.node-id {
  display: inline-block;
  width: fit-content;
  border-radius: var(--radius-sm);
  padding: 1px 6px;
  color: var(--color-muted);
  background: var(--color-surface-muted);
  font-family: var(--font-mono);
  font-size: 11px;
}

.channel-cell {
  color: var(--color-muted);
  font-family: var(--font-mono);
  font-size: 12px;
}

.message-text {
  line-height: 1.5;
}

.muted-tag {
  display: inline-block;
  border-radius: var(--radius-sm);
  padding: 2px 8px;
  color: var(--color-muted);
  background: var(--color-surface-muted);
  font-size: 12px;
}
</style>
