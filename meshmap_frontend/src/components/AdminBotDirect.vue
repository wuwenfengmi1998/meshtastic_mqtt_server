<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onBeforeUpdate, onMounted, onUpdated, ref, watch } from 'vue'
import { getBotDirectTextMessages, getBotNodes, getNodeInfo, sendBotMessage } from '../api'
import type { BotNode, NodeInfo, TextMessage } from '../types'

const chatPageSize = 30
const maxTextBytes = 200
const topThreshold = 8
const bottomThreshold = 40
// 私聊固定走 PKI，channel_id 与固件 ServiceEnvelope 保持一致
const directChannelId = 'PKI'

const bots = ref<BotNode[]>([])
const targets = ref<NodeInfo[]>([])
const messages = ref<TextMessage[]>([])
const selectedBotId = ref<number | null>(null)
const selectedTargetId = ref('')
const text = ref('')
const loading = ref(false)
const sending = ref(false)
const loadingOlder = ref(false)
const hasMore = ref(true)
const initialized = ref(false)
const error = ref('')
const notice = ref('')
const panelRef = ref<HTMLElement | null>(null)
let refreshTimer: number | undefined
let shouldStickToBottom = true
let didInitialScroll = false
let restoreScrollHeight: number | null = null
let restoreScrollTop = 0
let restoreMessageCount = 0

const selectedBot = computed(() => bots.value.find((item) => item.id === selectedBotId.value) ?? null)
const selectedTarget = computed(() => targets.value.find((item) => item.node_id === selectedTargetId.value) ?? null)
const directTextBytes = computed(() => new TextEncoder().encode(text.value).length)
const canSend = computed(() => !!selectedBot.value && !!selectedTarget.value && !!text.value.trim() && directTextBytes.value <= maxTextBytes && !sending.value)
const groupedMessages = computed(() => {
  const groups = new Map<string, TextMessage & { mergedCount: number; mergedMessages: TextMessage[] }>()
  for (const item of messages.value) {
    const key = `${item.packet_id ?? ''}\n${item.text ?? ''}`
    const group = groups.get(key)
    if (group) {
      group.mergedCount += 1
      group.mergedMessages.push(item)
    } else {
      groups.set(key, { ...item, mergedCount: 1, mergedMessages: [item] })
    }
  }
  return Array.from(groups.values())
})

watch(selectedBot, () => {
  resetChat()
  loadInitialMessages()
})

watch(selectedTargetId, () => {
  resetChat()
  loadInitialMessages()
})

function resetChat() {
  messages.value = []
  hasMore.value = true
  initialized.value = false
  didInitialScroll = false
  restoreScrollHeight = null
  restoreScrollTop = 0
  restoreMessageCount = 0
}

function toChronological(items: TextMessage[]) {
  return [...items].reverse()
}

function compareMessages(a: TextMessage, b: TextMessage) {
  const timeDiff = Date.parse(a.created_at) - Date.parse(b.created_at)
  return timeDiff !== 0 ? timeDiff : a.id - b.id
}

function mergeMessages(existing: TextMessage[], incoming: TextMessage[]) {
  const byId = new Map<number, TextMessage>()
  for (const item of existing) byId.set(item.id, item)
  for (const item of incoming) byId.set(item.id, item)
  return Array.from(byId.values()).sort(compareMessages)
}

function isNearBottom(el: HTMLElement) {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= bottomThreshold
}

function clearRestoreState() {
  restoreScrollHeight = null
  restoreScrollTop = 0
  restoreMessageCount = 0
}

async function refreshLists() {
  loading.value = true
  error.value = ''
  try {
    const [botResponse, nodeResponse] = await Promise.all([getBotNodes(100, 0), getNodeInfo(500, 0)])
    bots.value = botResponse.items
    targets.value = nodeResponse.items
    if (!selectedBotId.value && bots.value.length > 0) {
      selectedBotId.value = bots.value[0].id
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function loadInitialMessages() {
  if (!selectedBot.value || !selectedTarget.value) return
  loadingOlder.value = true
  try {
    const response = await getBotDirectTextMessages(selectedBot.value.id, selectedTarget.value.node_num, chatPageSize, 0, directChannelId)
    messages.value = toChronological(response.items)
    hasMore.value = response.items.length === chatPageSize
    initialized.value = true
    await nextTick()
    const el = panelRef.value
    if (el) el.scrollTop = el.scrollHeight
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loadingOlder.value = false
  }
}

async function loadOlderMessages() {
  if (!selectedBot.value || !selectedTarget.value || loadingOlder.value || !hasMore.value) return
  loadingOlder.value = true
  try {
    const response = await getBotDirectTextMessages(selectedBot.value.id, selectedTarget.value.node_num, chatPageSize, messages.value.length, directChannelId)
    messages.value = mergeMessages(messages.value, toChronological(response.items))
    hasMore.value = response.items.length === chatPageSize
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loadingOlder.value = false
  }
}

async function pollLatestMessages() {
  if (!selectedBot.value || !selectedTarget.value) return
  const response = await getBotDirectTextMessages(selectedBot.value.id, selectedTarget.value.node_num, chatPageSize, 0, directChannelId)
  messages.value = mergeMessages(messages.value, toChronological(response.items))
}

async function sendDirectMessage() {
  if (!selectedBot.value || !selectedTarget.value) return
  sending.value = true
  error.value = ''
  notice.value = ''
  try {
    const response = await sendBotMessage({ bot_id: selectedBot.value.id, message_type: 'direct', channel_id: directChannelId, to_node_id: selectedTarget.value.node_id, text: text.value })
    if (response.error) {
      error.value = response.error
    } else {
      text.value = ''
      notice.value = '消息已发送'
    }
    await pollLatestMessages()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    sending.value = false
  }
}

function handleScroll() {
  const el = panelRef.value
  if (!el || el.scrollTop > topThreshold) return
  if (restoreScrollHeight == null) {
    restoreScrollHeight = el.scrollHeight
    restoreScrollTop = el.scrollTop
    restoreMessageCount = groupedMessages.value.length
  }
  loadOlderMessages()
}

function isOwn(item: TextMessage) {
  return item.from_id === selectedBot.value?.node_id
}

function senderName(item: TextMessage) {
  if (item.from_id === selectedBot.value?.node_id) return selectedBot.value?.long_name || item.from_id
  if (item.from_id === selectedTarget.value?.node_id) return selectedTarget.value?.long_name || selectedTarget.value?.short_name || item.from_id
  return item.from_id
}

function formatTime(value: string) {
  return new Date(value).toLocaleString()
}

onBeforeUpdate(() => {
  const el = panelRef.value
  if (el) shouldStickToBottom = isNearBottom(el)
})

onUpdated(() => {
  const el = panelRef.value
  if (!el) return
  if (restoreScrollHeight != null) {
    if (groupedMessages.value.length > restoreMessageCount) {
      el.scrollTop = el.scrollHeight - restoreScrollHeight + restoreScrollTop
      clearRestoreState()
      return
    }
    if (!loadingOlder.value) clearRestoreState()
  }
  if (!didInitialScroll || shouldStickToBottom) {
    el.scrollTop = el.scrollHeight
    didInitialScroll = true
  }
})

onMounted(() => {
  refreshLists()
  refreshTimer = window.setInterval(() => pollLatestMessages(), 5000)
})

onBeforeUnmount(() => {
  if (refreshTimer !== undefined) window.clearInterval(refreshTimer)
})
</script>

<template>
  <section class="panel direct-page">
    <div class="direct-header">
      <div>
        <p class="eyebrow">Direct Bot Chat</p>
        <h2>机器人私聊 <span class="pki-badge" title="使用 X25519 + AES-CCM 与目标节点端到端加密">PKI 加密</span></h2>
      </div>
      <div class="direct-actions">
        <a class="admin-button secondary" href="/admin/bot">返回频道聊天</a>
        <button class="admin-button" @click="refreshLists" :disabled="loading">{{ loading ? '刷新中...' : '刷新列表' }}</button>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="notice" class="success">{{ notice }}</p>

    <div class="direct-selectors">
      <label>机器人
        <select v-model="selectedBotId">
          <option :value="null">选择机器人</option>
          <option v-for="bot in bots" :key="bot.id" :value="bot.id">{{ bot.long_name }} · {{ bot.node_id }}</option>
        </select>
      </label>
      <label>目标节点
        <select v-model="selectedTargetId">
          <option value="">选择目标节点</option>
          <option v-for="node in targets" :key="node.node_id" :value="node.node_id">{{ node.long_name || node.short_name || node.node_id }} · {{ node.node_id }}</option>
        </select>
      </label>
    </div>

    <p class="direct-hint">私聊固定走 PKI（channel_id = "PKI"），需要目标节点已上报 NodeInfo 公钥才能加密。</p>

    <div ref="panelRef" class="direct-chat-list" @scroll.passive="handleScroll">
      <div v-if="loadingOlder" class="chat-loading">正在加载更早消息...</div>
      <div v-else-if="!hasMore && messages.length > 0" class="chat-end">没有更多历史消息</div>
      <div v-if="groupedMessages.length === 0" class="empty-state">请选择机器人和目标节点，或当前会话暂无消息。</div>
      <div v-for="item in groupedMessages" :key="item.id" class="chat-bubble-row" :class="{ own: isOwn(item) }">
        <div class="chat-bubble">
          <div class="bubble-meta"><strong>{{ senderName(item) }}</strong><small>{{ formatTime(item.created_at) }}</small></div>
          <div class="bubble-text">{{ item.text || '[binary]' }} <span v-if="item.mergedCount > 1" class="message-merge-count">x{{ item.mergedCount }}</span></div>
          <div class="bubble-topic">{{ item.topic }}</div>
        </div>
      </div>
    </div>

    <div class="direct-composer">
      <textarea v-model="text" rows="3" placeholder="输入私聊消息"></textarea>
      <div class="send-actions">
        <span class="hint" :class="{ warn: directTextBytes > maxTextBytes }">{{ directTextBytes }} / {{ maxTextBytes }} bytes</span>
        <button class="admin-button" @click="sendDirectMessage" :disabled="!canSend">{{ sending ? '发送中...' : '发送私聊' }}</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.direct-page { display: grid; gap: 12px; padding: 16px; }
.direct-header, .direct-actions, .send-actions { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
.direct-selectors { display: grid; grid-template-columns: repeat(2, minmax(180px, 1fr)); gap: 12px; }
.direct-hint { color: #475569; font-size: 12px; margin: 0; }
.pki-badge { display: inline-flex; align-items: center; margin-left: 8px; border-radius: 999px; padding: 2px 10px; color: #1d4ed8; background: #dbeafe; font-size: 12px; font-weight: 700; vertical-align: middle; }
label { display: grid; gap: 5px; color: #334155; font-size: 13px; font-weight: 800; }
input, select, textarea { box-sizing: border-box; width: 100%; border: 1px solid #cbd5e1; border-radius: 10px; padding: 9px 11px; color: #0f172a; font: inherit; background: #fff; }
.direct-chat-list { min-height: 420px; max-height: 560px; overflow: auto; display: flex; flex-direction: column; gap: 10px; border: 1px solid #e2e8f0; border-radius: 14px; padding: 14px; background: linear-gradient(180deg, #f8fafc 0%, #eef4ff 100%); }
.chat-loading, .chat-end { align-self: center; border-radius: 999px; padding: 6px 10px; color: #64748b; font-size: 12px; background: #e2e8f0; }
.chat-bubble-row { display: flex; justify-content: flex-start; }
.chat-bubble-row.own { justify-content: flex-end; }
.chat-bubble { max-width: min(680px, 78%); border: 1px solid #e2e8f0; border-radius: 16px 16px 16px 4px; padding: 10px 12px; background: #fff; box-shadow: 0 4px 16px rgba(15, 23, 42, 0.06); }
.chat-bubble-row.own .chat-bubble { border-color: #bfdbfe; border-radius: 16px 16px 4px 16px; background: #dbeafe; }
.bubble-meta { display: flex; align-items: center; justify-content: space-between; gap: 12px; color: #334155; font-size: 12px; }
.bubble-meta small, .bubble-topic, .hint { color: #64748b; }
.hint.warn { color: #b91c1c; font-weight: 800; }
.bubble-text { margin-top: 6px; color: #0f172a; line-height: 1.45; white-space: pre-wrap; word-break: break-word; }
.bubble-topic { margin-top: 6px; font-size: 11px; word-break: break-all; }
.message-merge-count { display: inline-flex; margin-left: 6px; border-radius: 999px; padding: 1px 6px; color: #1d4ed8; background: #bfdbfe; font-size: 12px; font-weight: 800; }
.direct-composer { display: grid; gap: 10px; }
.admin-button.secondary { color: #334155; text-decoration: none; background: #e2e8f0; }
.empty-state { color: #64748b; padding: 16px; border: 1px dashed #cbd5e1; border-radius: 14px; text-align: center; background: #f8fafc; }
@media (max-width: 800px) { .direct-selectors { grid-template-columns: 1fr; } }
</style>
