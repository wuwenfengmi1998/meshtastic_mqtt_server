<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onBeforeUpdate, onMounted, onUpdated, ref, watch } from 'vue'
import { getBotConversations, getBotDirectMessages, getBotNodes, getNodeInfo, markBotDirectMessagesRead, sendBotMessage } from '../api'
import type { BotDirectConversation, BotDirectMessage, BotNode, NodeInfo } from '../types'

const chatPageSize = 30
const conversationPageSize = 100
const maxTextBytes = 200
const topThreshold = 8
const bottomThreshold = 40

const bots = ref<BotNode[]>([])
const targets = ref<NodeInfo[]>([])
const conversations = ref<BotDirectConversation[]>([])
const unreadTotal = ref(0)
const messages = ref<BotDirectMessage[]>([])
const selectedBotId = ref<number | null>(null)
const selectedPeerNum = ref<number | null>(null)
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
const selectedConversation = computed(() => conversations.value.find((item) => item.peer_node_num === selectedPeerNum.value) ?? null)
const selectedPeerNode = computed(() => {
  if (selectedPeerNum.value == null) return null
  return targets.value.find((node) => node.node_num === selectedPeerNum.value) ?? null
})
const directTextBytes = computed(() => new TextEncoder().encode(text.value).length)
const canSend = computed(() => !!selectedBot.value && selectedPeerNum.value != null && !!text.value.trim() && directTextBytes.value <= maxTextBytes && !sending.value)
const groupedMessages = computed(() => {
  // direction + packet_id + text 作为合并键，避免重复 publish 时把 inbound/outbound 误合并。
  const groups = new Map<string, BotDirectMessage & { mergedCount: number; mergedMessages: BotDirectMessage[] }>()
  for (const item of messages.value) {
    const key = `${item.direction}\n${item.packet_id ?? ''}\n${item.text ?? ''}`
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

watch(selectedBotId, async () => {
  selectedPeerNum.value = null
  conversations.value = []
  unreadTotal.value = 0
  resetChat()
  await reloadConversations()
})

watch(selectedPeerNum, () => {
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

function toChronological(items: BotDirectMessage[]) {
  return [...items].reverse()
}

function compareMessages(a: BotDirectMessage, b: BotDirectMessage) {
  const timeDiff = Date.parse(a.created_at) - Date.parse(b.created_at)
  return timeDiff !== 0 ? timeDiff : a.id - b.id
}

function mergeMessages(existing: BotDirectMessage[], incoming: BotDirectMessage[]) {
  const byId = new Map<number, BotDirectMessage>()
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
    } else {
      await reloadConversations()
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function reloadConversations() {
  if (!selectedBot.value) return
  try {
    const response = await getBotConversations(selectedBot.value.id, conversationPageSize, 0)
    let items = response.items
    // 用户通过“新建私聊”选中尚未有消息的节点时，本地有占位会话，但后端不会返回它。
    // 直接覆盖会让轮询自动取消选择，所以这里保留占位并 prepend 回去。
    if (selectedPeerNum.value != null && !items.some((c) => c.peer_node_num === selectedPeerNum.value)) {
      const localPlaceholder = conversations.value.find((c) => c.peer_node_num === selectedPeerNum.value)
      if (localPlaceholder) items = [localPlaceholder, ...items]
    }
    conversations.value = items
    unreadTotal.value = response.unread_total
    // 第一次进入时自动选中第一个会话，避免空白页面。
    if (selectedPeerNum.value == null && items.length > 0) {
      selectedPeerNum.value = items[0].peer_node_num
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function selectConversation(conv: BotDirectConversation) {
  if (selectedPeerNum.value === conv.peer_node_num) return
  selectedPeerNum.value = conv.peer_node_num
}

async function startConversationFromPicker(nodeNum: number) {
  if (!Number.isFinite(nodeNum) || nodeNum <= 0) return
  // 如果会话不在侧边栏，先做本地占位再从后端拉一次列表
  if (!conversations.value.some((c) => c.peer_node_num === nodeNum)) {
    const peer = targets.value.find((n) => n.node_num === nodeNum)
    if (peer) {
      conversations.value = [
        {
          bot_id: selectedBot.value?.id ?? 0,
          peer_node_id: peer.node_id,
          peer_node_num: peer.node_num,
          last_message_at: '',
          last_text: '',
          last_direction: '',
          unread_count: 0,
          total_count: 0,
        },
        ...conversations.value,
      ]
    }
  }
  selectedPeerNum.value = nodeNum
}

async function loadInitialMessages() {
  if (!selectedBot.value || selectedPeerNum.value == null) return
  loadingOlder.value = true
  try {
    const response = await getBotDirectMessages(selectedBot.value.id, selectedPeerNum.value, chatPageSize, 0)
    messages.value = toChronological(response.items)
    hasMore.value = response.items.length === chatPageSize
    initialized.value = true
    await nextTick()
    const el = panelRef.value
    if (el) el.scrollTop = el.scrollHeight
    await markCurrentConversationRead()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loadingOlder.value = false
  }
}

async function loadOlderMessages() {
  if (!selectedBot.value || selectedPeerNum.value == null || loadingOlder.value || !hasMore.value) return
  loadingOlder.value = true
  try {
    const response = await getBotDirectMessages(selectedBot.value.id, selectedPeerNum.value, chatPageSize, messages.value.length)
    messages.value = mergeMessages(messages.value, toChronological(response.items))
    hasMore.value = response.items.length === chatPageSize
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loadingOlder.value = false
  }
}

async function pollLatestMessages() {
  if (!selectedBot.value) return
  await reloadConversations()
  if (selectedPeerNum.value == null) return
  const response = await getBotDirectMessages(selectedBot.value.id, selectedPeerNum.value, chatPageSize, 0)
  const before = messages.value.length
  messages.value = mergeMessages(messages.value, toChronological(response.items))
  if (messages.value.length > before) {
    await markCurrentConversationRead()
  }
}

async function markCurrentConversationRead() {
  if (!selectedBot.value || selectedPeerNum.value == null) return
  const conv = selectedConversation.value
  if (conv && conv.unread_count === 0) return
  try {
    const result = await markBotDirectMessagesRead(selectedBot.value.id, selectedPeerNum.value)
    if (result.updated > 0) {
      // 本地立即清零，避免 polling 间隙仍然展示红点。
      conversations.value = conversations.value.map((c) =>
        c.peer_node_num === selectedPeerNum.value ? { ...c, unread_count: 0 } : c,
      )
      unreadTotal.value = Math.max(0, unreadTotal.value - result.updated)
    }
  } catch (err) {
    // 标记已读失败只打印不打断聊天体验
    console.warn('mark read failed', err)
  }
}

async function sendDirectMessage() {
  if (!selectedBot.value || selectedPeerNum.value == null) return
  const peer = selectedPeerNode.value
  const peerNodeId = peer ? peer.node_id : selectedConversation.value?.peer_node_id
  if (!peerNodeId) {
    error.value = '找不到目标节点 ID，请等待 NodeInfo 同步后再试'
    return
  }
  sending.value = true
  error.value = ''
  notice.value = ''
  try {
    const response = await sendBotMessage({ bot_id: selectedBot.value.id, message_type: 'direct', channel_id: 'PKI', to_node_id: peerNodeId, text: text.value })
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

function isOwn(item: BotDirectMessage) {
  return item.direction === 'outbound'
}

function senderName(item: BotDirectMessage) {
  if (item.direction === 'outbound') return selectedBot.value?.long_name || item.bot_node_id
  const peer = targets.value.find((n) => n.node_num === item.peer_node_num)
  return peer?.long_name || peer?.short_name || item.peer_node_id
}

function statusLabel(item: BotDirectMessage) {
  if (item.direction !== 'outbound') return ''
  if (item.status === 'failed') return `发送失败${item.error ? '：' + item.error : ''}`
  if (item.status === 'pending') return '发送中…'
  return ''
}

function conversationTitle(conv: BotDirectConversation) {
  const peer = targets.value.find((n) => n.node_num === conv.peer_node_num)
  return peer?.long_name || peer?.short_name || conv.peer_node_id
}

function previewText(conv: BotDirectConversation) {
  if (!conv.last_text) return '暂无消息'
  const prefix = conv.last_direction === 'outbound' ? '我: ' : ''
  const text = conv.last_text.replace(/\s+/g, ' ').trim()
  return prefix + (text.length > 32 ? text.slice(0, 32) + '…' : text)
}

function formatTime(value: string) {
  if (!value) return ''
  return new Date(value).toLocaleString()
}

function formatRelative(value: string) {
  if (!value) return ''
  const ts = Date.parse(value)
  if (!Number.isFinite(ts)) return ''
  const diff = Date.now() - ts
  if (diff < 60_000) return '刚刚'
  if (diff < 3600_000) return Math.floor(diff / 60_000) + ' 分钟前'
  if (diff < 86400_000) return Math.floor(diff / 3600_000) + ' 小时前'
  return new Date(ts).toLocaleDateString()
}

const peerNodeOptions = computed(() => {
  const seen = new Set(conversations.value.map((c) => c.peer_node_num))
  return targets.value.filter((node) => !seen.has(node.node_num))
})

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
        <h2>
          机器人私聊
          <span class="pki-badge" title="使用 X25519 + AES-CCM 与目标节点端到端加密">PKI 加密</span>
          <span v-if="unreadTotal > 0" class="header-unread-badge">{{ unreadTotal > 99 ? '99+' : unreadTotal }} 未读</span>
        </h2>
      </div>
      <div class="direct-actions">
        <a class="admin-button secondary" href="/admin/bot">返回频道聊天</a>
        <button class="admin-button" @click="refreshLists" :disabled="loading">{{ loading ? '刷新中...' : '刷新列表' }}</button>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="notice" class="success">{{ notice }}</p>

    <div class="direct-bot-picker">
      <label>机器人
        <select v-model="selectedBotId">
          <option :value="null">选择机器人</option>
          <option v-for="bot in bots" :key="bot.id" :value="bot.id">{{ bot.long_name }} · {{ bot.node_id }}</option>
        </select>
      </label>
      <label>新建私聊
        <select :value="''" @change="(event) => { startConversationFromPicker(Number((event.target as HTMLSelectElement).value)); (event.target as HTMLSelectElement).value = '' }">
          <option value="">选择目标节点开启会话</option>
          <option v-for="node in peerNodeOptions" :key="node.node_id" :value="node.node_num">{{ node.long_name || node.short_name || node.node_id }} · {{ node.node_id }}</option>
        </select>
      </label>
    </div>

    <div class="direct-layout">
      <aside class="conversation-list">
        <p v-if="conversations.length === 0" class="empty-state">还没有私聊会话。等设备发来消息或在上方“新建私聊”开启。</p>
        <button
          v-for="conv in conversations"
          :key="conv.peer_node_num"
          class="conversation-item"
          :class="{ active: conv.peer_node_num === selectedPeerNum }"
          @click="selectConversation(conv)"
        >
          <div class="conversation-row">
            <span class="conversation-title">{{ conversationTitle(conv) }}</span>
            <span class="conversation-time">{{ formatRelative(conv.last_message_at) }}</span>
          </div>
          <div class="conversation-row">
            <span class="conversation-preview">{{ previewText(conv) }}</span>
            <span v-if="conv.unread_count > 0" class="conversation-unread">{{ conv.unread_count > 99 ? '99+' : conv.unread_count }}</span>
          </div>
          <div class="conversation-meta">{{ conv.peer_node_id }} · 共 {{ conv.total_count }} 条</div>
        </button>
      </aside>

      <div class="direct-main">
        <p class="direct-hint">私聊固定走 PKI（channel_id = "PKI"），需要目标节点已上报 NodeInfo 公钥才能加密。</p>

        <div ref="panelRef" class="direct-chat-list" @scroll.passive="handleScroll">
          <div v-if="loadingOlder" class="chat-loading">正在加载更早消息...</div>
          <div v-else-if="!hasMore && messages.length > 0" class="chat-end">没有更多历史消息</div>
          <div v-if="groupedMessages.length === 0" class="empty-state">{{ selectedPeerNum == null ? '请选择左侧会话或新建私聊。' : '当前会话暂无消息。' }}</div>
          <div v-for="item in groupedMessages" :key="item.id" class="chat-bubble-row" :class="{ own: isOwn(item) }">
            <div class="chat-bubble">
              <div class="bubble-meta"><strong>{{ senderName(item) }}</strong><small>{{ formatTime(item.created_at) }}</small></div>
              <div class="bubble-text">{{ item.text || '[binary]' }} <span v-if="item.mergedCount > 1" class="message-merge-count">x{{ item.mergedCount }}</span></div>
              <div v-if="statusLabel(item)" class="bubble-status">{{ statusLabel(item) }}</div>
              <div class="bubble-topic">{{ item.topic }}</div>
            </div>
          </div>
        </div>

        <div class="direct-composer">
          <textarea v-model="text" rows="3" :placeholder="selectedPeerNum == null ? '先选择左侧会话再输入' : '输入私聊消息'" :disabled="selectedPeerNum == null"></textarea>
          <div class="send-actions">
            <span class="hint" :class="{ warn: directTextBytes > maxTextBytes }">{{ directTextBytes }} / {{ maxTextBytes }} bytes</span>
            <button class="admin-button" @click="sendDirectMessage" :disabled="!canSend">{{ sending ? '发送中...' : '发送私聊' }}</button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.direct-page { display: grid; gap: 12px; padding: 16px; }
.direct-header, .direct-actions, .send-actions { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
.direct-bot-picker { display: grid; grid-template-columns: repeat(2, minmax(200px, 1fr)); gap: 12px; }
.direct-hint { color: #475569; font-size: 12px; margin: 0 0 8px; }
.pki-badge { display: inline-flex; align-items: center; margin-left: 8px; border-radius: 999px; padding: 2px 10px; color: #1d4ed8; background: #dbeafe; font-size: 12px; font-weight: 700; vertical-align: middle; }
.header-unread-badge { display: inline-flex; align-items: center; margin-left: 8px; border-radius: 999px; padding: 2px 10px; color: #fff; background: #ef4444; font-size: 12px; font-weight: 800; vertical-align: middle; }
label { display: grid; gap: 5px; color: #334155; font-size: 13px; font-weight: 800; }
input, select, textarea { box-sizing: border-box; width: 100%; border: 1px solid #cbd5e1; border-radius: 10px; padding: 9px 11px; color: #0f172a; font: inherit; background: #fff; }
.direct-layout { display: grid; grid-template-columns: minmax(220px, 280px) 1fr; gap: 14px; align-items: start; }
.conversation-list { display: flex; flex-direction: column; gap: 6px; max-height: 620px; overflow: auto; padding: 6px; border: 1px solid #e2e8f0; border-radius: 14px; background: #fff; }
.conversation-item { all: unset; cursor: pointer; display: grid; gap: 4px; padding: 10px 12px; border-radius: 10px; border: 1px solid transparent; }
.conversation-item:hover { background: #f1f5f9; }
.conversation-item.active { background: #dbeafe; border-color: #93c5fd; }
.conversation-row { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.conversation-title { font-weight: 800; color: #0f172a; font-size: 14px; }
.conversation-time { color: #64748b; font-size: 11px; white-space: nowrap; }
.conversation-preview { flex: 1; color: #475569; font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.conversation-unread { display: inline-flex; align-items: center; justify-content: center; min-width: 20px; height: 20px; padding: 0 6px; border-radius: 999px; background: #ef4444; color: #fff; font-size: 11px; font-weight: 800; }
.conversation-meta { color: #94a3b8; font-size: 11px; }
.direct-main { display: grid; gap: 12px; }
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
.bubble-status { margin-top: 4px; color: #b91c1c; font-size: 11px; font-weight: 700; }
.bubble-topic { margin-top: 6px; font-size: 11px; word-break: break-all; }
.message-merge-count { display: inline-flex; margin-left: 6px; border-radius: 999px; padding: 1px 6px; color: #1d4ed8; background: #bfdbfe; font-size: 12px; font-weight: 800; }
.direct-composer { display: grid; gap: 10px; }
.admin-button.secondary { color: #334155; text-decoration: none; background: #e2e8f0; }
.empty-state { color: #64748b; padding: 16px; border: 1px dashed #cbd5e1; border-radius: 14px; text-align: center; background: #f8fafc; }
@media (max-width: 800px) {
  .direct-layout { grid-template-columns: 1fr; }
  .direct-bot-picker { grid-template-columns: 1fr; }
  .conversation-list { max-height: 320px; }
}
</style>
