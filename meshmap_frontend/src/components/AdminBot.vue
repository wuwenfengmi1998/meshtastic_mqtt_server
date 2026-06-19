<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onBeforeUpdate, onMounted, onUpdated, ref, watch } from 'vue'
import { broadcastBotNodeInfo, createBotNode, deleteBotNode, getBotNodes, getNodeInfo, getTextMessages, regenerateBotNodeKeys, sendBotMessage, updateBotNode } from '../api'
import type { BotMessageType, BotNode, BotNodePayload, NodeInfo, TextMessage } from '../types'

const botPageSize = 100
const chatPageSize = 30
const maxTextBytes = 200
const topThreshold = 8
const bottomThreshold = 40
const scrollOverflowAllowance = 1

const bots = ref<BotNode[]>([])
const chatMessages = ref<TextMessage[]>([])
const targets = ref<NodeInfo[]>([])
const selectedBotId = ref<number | null>(null)
const loading = ref(false)
const chatLoadingOlder = ref(false)
const chatHasMore = ref(true)
const chatInitialized = ref(false)
const saving = ref(false)
const sending = ref(false)
const broadcastingNodeInfo = ref(false)
const regeneratingKeys = ref(false)
const error = ref('')
const message = ref('')
const targetQuery = ref('')
const chatPanelRef = ref<HTMLElement | null>(null)

const newBot = ref<{ node_num: string | number | null; long_name: string; short_name: string; default_channel_id: string; topic_prefix: string; psk: string; nodeinfo_broadcast_enabled: boolean; nodeinfo_broadcast_interval_seconds: string | number; enabled: boolean }>({ node_num: '', long_name: '', short_name: '', default_channel_id: 'LongFast', topic_prefix: 'msh/CN', psk: 'AQ==', nodeinfo_broadcast_enabled: true, nodeinfo_broadcast_interval_seconds: '3600', enabled: true })
const edits = ref<Record<number, { node_num: string | number | null; long_name: string; short_name: string; default_channel_id: string; topic_prefix: string; psk: string; nodeinfo_broadcast_enabled: boolean; nodeinfo_broadcast_interval_seconds: string | number; enabled: boolean }>>({})
const sendForm = ref<{ message_type: BotMessageType; channel_id: string; to_node_id: string; text: string }>({ message_type: 'channel', channel_id: 'LongFast', to_node_id: '', text: '' })

const selectedBot = computed(() => bots.value.find((bot) => bot.id === selectedBotId.value) ?? null)
const enabledBots = computed(() => bots.value.filter((bot) => bot.enabled).length)
const currentChannelID = computed(() => sendForm.value.channel_id.trim() || selectedBot.value?.default_channel_id || '')
const sendTextBytes = computed(() => new TextEncoder().encode(sendForm.value.text).length)
const isTextTooLong = computed(() => sendTextBytes.value > maxTextBytes)
const nodesById = computed(() => {
  const map = new Map<string, NodeInfo>()
  for (const node of targets.value) {
    map.set(node.node_id, node)
  }
  return Object.fromEntries(map)
})
const groupedChatMessages = computed(() => {
  const groups = new Map<string, TextMessage & { mergedCount: number; mergedMessages: TextMessage[] }>()
  for (const item of chatMessages.value) {
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
const targetOptions = computed(() => {
  const query = targetQuery.value.trim().toLowerCase()
  return targets.value
    .filter((node) => node.node_id !== selectedBot.value?.node_id)
    .filter((node) => {
      if (!query) return true
      return [node.node_id, node.long_name, node.short_name, String(node.node_num)]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(query))
    })
    .slice(0, 80)
})
const canSend = computed(() => {
  if (!selectedBot.value || sending.value || isTextTooLong.value || !sendForm.value.text.trim()) return false
  if (!currentChannelID.value) return false
  if (sendForm.value.message_type === 'direct' && !sendForm.value.to_node_id) return false
  return true
})

let shouldStickToBottom = true
let didInitialScroll = false
let restoreScrollHeight: number | null = null
let restoreScrollTop = 0
let restoreMessageCount = 0
let chatRefreshTimer: number | undefined

watch(selectedBot, (bot) => {
  if (bot) {
    sendForm.value.channel_id = bot.default_channel_id
    resetChatState()
    loadInitialChatMessages()
  }
})

watch(currentChannelID, () => {
  if (selectedBot.value) {
    resetChatState()
    loadInitialChatMessages()
  }
})

function botPayload(form: { node_num: string | number | null; long_name: string; short_name: string; default_channel_id: string; topic_prefix?: string; psk?: string; nodeinfo_broadcast_enabled?: boolean; nodeinfo_broadcast_interval_seconds?: string | number; enabled: boolean }): BotNodePayload {
  // <input type="number"> 的 v-model 会把绑定值转成 number，而 number 上没有 trim，
  // 直接调用会抛 "node_num.trim is not a function" 让保存失败。统一转成 string 再 trim。
  const nodeNumText = form.node_num == null ? '' : String(form.node_num).trim()
  const interval = Number(form.nodeinfo_broadcast_interval_seconds || 3600)
  return {
    node_num: nodeNumText ? Number(nodeNumText) : null,
    long_name: form.long_name.trim(),
    short_name: form.short_name.trim(),
    default_channel_id: form.default_channel_id.trim(),
    topic_prefix: form.topic_prefix?.trim() || 'msh/CN',
    psk: form.psk?.trim() || 'AQ==',
    nodeinfo_broadcast_enabled: form.nodeinfo_broadcast_enabled ?? true,
    nodeinfo_broadcast_interval_seconds: Number.isFinite(interval) && interval > 0 ? interval : 3600,
    enabled: form.enabled,
  }
}

function resetEdits() {
  edits.value = Object.fromEntries(bots.value.map((bot) => [bot.id, {
    node_num: String(bot.node_num),
    long_name: bot.long_name,
    short_name: bot.short_name,
    default_channel_id: bot.default_channel_id,
    topic_prefix: bot.topic_prefix,
    psk: bot.psk || 'AQ==',
    nodeinfo_broadcast_enabled: bot.nodeinfo_broadcast_enabled,
    nodeinfo_broadcast_interval_seconds: String(bot.nodeinfo_broadcast_interval_seconds || 3600),
    enabled: bot.enabled,
  }]))
}

function resetChatState() {
  chatMessages.value = []
  chatHasMore.value = true
  chatInitialized.value = false
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

async function refreshBots() {
  loading.value = true
  error.value = ''
  try {
    const response = await getBotNodes(botPageSize, 0)
    bots.value = response.items
    resetEdits()
    if (!selectedBotId.value && bots.value.length > 0) {
      selectBot(bots.value[0])
    }
    if (selectedBotId.value && !bots.value.some((bot) => bot.id === selectedBotId.value)) {
      selectedBotId.value = bots.value[0]?.id ?? null
      if (bots.value[0]) selectBot(bots.value[0])
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function loadInitialChatMessages() {
  if (!currentChannelID.value) return
  chatLoadingOlder.value = true
  try {
    const response = await getTextMessages(chatPageSize, 0, { channelId: currentChannelID.value })
    chatMessages.value = toChronological(response.items)
    chatHasMore.value = response.items.length === chatPageSize
    chatInitialized.value = true
    await nextTick()
    const el = chatPanelRef.value
    if (el) el.scrollTop = el.scrollHeight
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    chatLoadingOlder.value = false
  }
}

async function loadOlderChatMessages() {
  if (chatLoadingOlder.value || !chatHasMore.value || !currentChannelID.value) return
  chatLoadingOlder.value = true
  try {
    const response = await getTextMessages(chatPageSize, chatMessages.value.length, { channelId: currentChannelID.value })
    chatMessages.value = mergeMessages(chatMessages.value, toChronological(response.items))
    chatHasMore.value = response.items.length === chatPageSize
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    chatLoadingOlder.value = false
  }
}

async function pollLatestChatMessages() {
  if (!currentChannelID.value) return
  const response = await getTextMessages(chatPageSize, 0, { channelId: currentChannelID.value })
  chatMessages.value = mergeMessages(chatMessages.value, toChronological(response.items))
}

async function refreshTargets() {
  try {
    const response = await getNodeInfo(500, 0)
    targets.value = response.items
  } catch {
    targets.value = []
  }
}

function selectBot(bot: BotNode) {
  selectedBotId.value = bot.id
  sendForm.value.channel_id = bot.default_channel_id
}

function applyBotUpdate(bot: BotNode) {
  const idx = bots.value.findIndex((item) => item.id === bot.id)
  if (idx >= 0) {
    bots.value.splice(idx, 1, bot)
  }
  resetEdits()
  selectedBotId.value = bot.id
}

async function createBot() {
  saving.value = true
  error.value = ''
  message.value = ''
  try {
    await createBotNode(botPayload(newBot.value))
    newBot.value = { node_num: '', long_name: '', short_name: '', default_channel_id: 'LongFast', topic_prefix: 'msh/CN', psk: 'AQ==', nodeinfo_broadcast_enabled: true, nodeinfo_broadcast_interval_seconds: '3600', enabled: true }
    message.value = '机器人已创建'
    await refreshBots()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}

async function saveBot(bot: BotNode) {
  const edit = edits.value[bot.id]
  if (!edit) return
  saving.value = true
  error.value = ''
  message.value = ''
  try {
    await updateBotNode(bot.id, botPayload(edit))
    message.value = '机器人已保存'
    await refreshBots()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}

async function removeBot(bot: BotNode) {
  if (!window.confirm(`确定删除机器人 ${bot.long_name} (${bot.node_id}) 吗？`)) return
  saving.value = true
  error.value = ''
  try {
    await deleteBotNode(bot.id)
    if (selectedBotId.value === bot.id) {
      selectedBotId.value = null
      resetChatState()
    }
    await refreshBots()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}

async function broadcastNodeInfoNow() {
  if (!selectedBot.value) {
    error.value = '请先选择机器人'
    return
  }
  broadcastingNodeInfo.value = true
  error.value = ''
  message.value = ''
  try {
    const response = await broadcastBotNodeInfo(selectedBot.value.id)
    applyBotUpdate(response.item)
    message.value = 'NodeInfo 已广播'
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    broadcastingNodeInfo.value = false
  }
}

async function regenerateKeys() {
  if (!selectedBot.value) {
    error.value = '请先选择机器人'
    return
  }
  if (!window.confirm('确定要重新生成该机器人的密钥吗？重新生成后，旧密钥将不能再用于 PKI 私聊。')) return
  regeneratingKeys.value = true
  error.value = ''
  message.value = ''
  try {
    const response = await regenerateBotNodeKeys(selectedBot.value.id)
    applyBotUpdate(response.item)
    message.value = '机器人密钥已重新生成'
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    regeneratingKeys.value = false
  }
}

async function sendMessage() {
  if (!selectedBot.value) {
    error.value = '请先选择机器人'
    return
  }
  if (isTextTooLong.value) {
    error.value = `消息过长，最多 ${maxTextBytes} bytes`
    return
  }
  sending.value = true
  error.value = ''
  message.value = ''
  try {
    const response = await sendBotMessage({
      bot_id: selectedBot.value.id,
      message_type: sendForm.value.message_type,
      channel_id: currentChannelID.value,
      to_node_id: sendForm.value.message_type === 'direct' ? sendForm.value.to_node_id : undefined,
      text: sendForm.value.text,
    })
    if (response.error) {
      error.value = response.error
    } else {
      message.value = '消息已发送'
      sendForm.value.text = ''
    }
    await pollLatestChatMessages()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    sending.value = false
  }
}

function handleChatScroll() {
  const el = chatPanelRef.value
  if (!el || el.scrollTop > topThreshold) return
  if (restoreScrollHeight == null) {
    restoreScrollHeight = el.scrollHeight
    restoreScrollTop = el.scrollTop
    restoreMessageCount = groupedChatMessages.value.length
  }
  loadOlderChatMessages()
}

function senderName(item: TextMessage) {
  if (item.from_id === selectedBot.value?.node_id) return selectedBot.value.long_name
  const node = nodesById.value[item.from_id]
  return node?.long_name || node?.short_name || item.from_id
}

function isOwnMessage(item: TextMessage) {
  return item.from_id === selectedBot.value?.node_id
}

function formatTime(value: string | null) {
  return value ? new Date(value).toLocaleString() : '-'
}

onBeforeUpdate(() => {
  const el = chatPanelRef.value
  if (el) shouldStickToBottom = isNearBottom(el)
})

onUpdated(() => {
  const el = chatPanelRef.value
  if (!el) return
  if (restoreScrollHeight != null) {
    if (groupedChatMessages.value.length > restoreMessageCount) {
      el.scrollTop = el.scrollHeight - restoreScrollHeight + restoreScrollTop
      clearRestoreState()
      return
    }
    if (!chatLoadingOlder.value) clearRestoreState()
  }
  if (!didInitialScroll || shouldStickToBottom) {
    el.scrollTop = el.scrollHeight
    didInitialScroll = true
  }
  if (chatInitialized.value && el.scrollHeight <= el.clientHeight + scrollOverflowAllowance) {
    handleChatScroll()
  }
})

onMounted(() => {
  refreshBots()
  refreshTargets()
  chatRefreshTimer = window.setInterval(() => {
    if (selectedBot.value && currentChannelID.value) {
      pollLatestChatMessages()
    }
  }, 5000)
})

onBeforeUnmount(() => {
  if (chatRefreshTimer !== undefined) {
    window.clearInterval(chatRefreshTimer)
  }
})
</script>

<template>
  <section class="admin-bot-page">
    <div class="panel bot-hero">
      <div>
        <p class="eyebrow">Meshtastic Bot</p>
        <h2>机器人节点</h2>
        <p class="hint">当前阶段使用频道 PSK 发送频道消息和定向消息；PKI 端到端私聊将在后续实现。</p>
      </div>
      <div class="bot-hero-actions">
        <span class="stat-chip">总数 {{ bots.length }}</span>
        <span class="stat-chip ok">启用 {{ enabledBots }}</span>
        <button class="admin-button" @click="refreshBots" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
      </div>
    </div>

    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="message" class="success">{{ message }}</p>

    <div class="bot-layout">
      <aside class="panel bot-sidebar">
        <div class="section-title">
          <div>
            <p class="eyebrow">Create</p>
            <h3>新建机器人</h3>
          </div>
        </div>
        <div class="bot-form compact-form">
          <label>节点号 <small>留空自动生成</small><input v-model="newBot.node_num" type="number" placeholder="305419896" /></label>
          <label>长名称<input v-model="newBot.long_name" placeholder="MQTT Bot" /></label>
          <label>短名称<input v-model="newBot.short_name" placeholder="BOT" /></label>
          <label>默认频道<input v-model="newBot.default_channel_id" placeholder="LongFast" /></label>
          <label>MQTT 根地址 <small>最终发布到 根地址/2/e/频道/节点</small><input v-model="newBot.topic_prefix" placeholder="msh/CN" /></label>
          <label>频道密钥 PSK <small>默认 AQ==</small><input v-model="newBot.psk" placeholder="AQ==" /></label>
          <label>NodeInfo 间隔秒数<input v-model="newBot.nodeinfo_broadcast_interval_seconds" type="number" min="60" /></label>
          <label class="inline"><input v-model="newBot.nodeinfo_broadcast_enabled" type="checkbox" /> 定期广播 NodeInfo</label>
          <label class="inline"><input v-model="newBot.enabled" type="checkbox" /> 启用</label>
          <button class="admin-button full" @click="createBot" :disabled="saving">创建机器人</button>
        </div>

        <div class="section-title list-title">
          <div>
            <p class="eyebrow">Nodes</p>
            <h3>机器人列表</h3>
          </div>
        </div>
        <div v-if="bots.length === 0" class="empty-state">暂无机器人</div>
        <div class="bot-list">
          <article v-for="bot in bots" :key="bot.id" class="bot-card" :class="{ selected: selectedBotId === bot.id, disabled: !bot.enabled }">
            <button class="bot-select" @click="selectBot(bot)">
              <span class="avatar">{{ bot.short_name.slice(0, 2).toUpperCase() }}</span>
              <span class="bot-main">
                <strong>{{ bot.long_name }}</strong>
                <small>{{ bot.node_id }} · {{ bot.default_channel_id }}</small>
              </span>
              <span class="state-dot" :class="{ ok: bot.enabled }"></span>
            </button>
            <details class="bot-details">
              <summary>编辑节点</summary>
              <div v-if="edits[bot.id]" class="bot-edit compact-form">
                <label>节点号<input v-model="edits[bot.id].node_num" type="number" /></label>
                <label>长名称<input v-model="edits[bot.id].long_name" /></label>
                <label>短名称<input v-model="edits[bot.id].short_name" /></label>
                <label>默认频道<input v-model="edits[bot.id].default_channel_id" /></label>
                <label>MQTT 根地址<input v-model="edits[bot.id].topic_prefix" placeholder="msh/CN" /></label>
                <label>频道密钥 PSK<input v-model="edits[bot.id].psk" placeholder="AQ==" /></label>
                <label>NodeInfo 间隔秒数<input v-model="edits[bot.id].nodeinfo_broadcast_interval_seconds" type="number" min="60" /></label>
                <label class="inline"><input v-model="edits[bot.id].nodeinfo_broadcast_enabled" type="checkbox" /> 定期广播 NodeInfo</label>
                <label class="inline"><input v-model="edits[bot.id].enabled" type="checkbox" /> 启用</label>
                <div class="row-actions">
                  <button class="admin-button" @click="saveBot(bot)" :disabled="saving">保存</button>
                  <button class="admin-button danger" @click="removeBot(bot)" :disabled="saving">删除</button>
                </div>
              </div>
            </details>
          </article>
        </div>
      </aside>

      <main class="bot-main-panel">
        <template v-if="selectedBot">
          <section class="panel selected-summary">
            <div>
              <p class="eyebrow">Selected Bot</p>
              <h2>{{ selectedBot.long_name }} <small>{{ selectedBot.short_name }}</small></h2>
            </div>
            <div class="summary-actions">
              <button class="admin-button secondary" @click="regenerateKeys" :disabled="regeneratingKeys">
                {{ regeneratingKeys ? '生成中...' : '重新生成密钥' }}
              </button>
              <button class="admin-button secondary" @click="broadcastNodeInfoNow" :disabled="broadcastingNodeInfo || !selectedBot.enabled">
                {{ broadcastingNodeInfo ? '广播中...' : '立即广播 NodeInfo' }}
              </button>
            </div>
            <div class="summary-grid">
              <span><strong>{{ selectedBot.node_id }}</strong><small>Node ID</small></span>
              <span><strong>{{ selectedBot.node_num }}</strong><small>Node Num</small></span>
              <span><strong>{{ selectedBot.default_channel_id }}</strong><small>默认频道</small></span>
              <span><strong>{{ selectedBot.topic_prefix || 'msh/CN' }}</strong><small>MQTT 根地址</small></span>
              <span><strong>{{ selectedBot.psk || 'AQ==' }}</strong><small>频道 PSK</small></span>
              <span><strong>{{ selectedBot.private_key_set ? '已生成' : '未生成' }}</strong><small>机器人密钥</small></span>
              <span class="public-key"><strong>{{ selectedBot.public_key || '-' }}</strong><small>Public Key</small></span>
              <span><strong>{{ selectedBot.nodeinfo_broadcast_enabled ? `${selectedBot.nodeinfo_broadcast_interval_seconds}s` : '关闭' }}</strong><small>NodeInfo 广播</small></span>
              <span><strong>{{ selectedBot.enabled ? '启用' : '停用' }}</strong><small>状态</small></span>
            </div>
          </section>

          <section class="panel bot-chat-panel">
            <div class="bot-chat-header">
              <div>
                <p class="eyebrow">Channel Chat</p>
                <h3>{{ currentChannelID || '未选择频道' }}</h3>
              </div>
              <button class="admin-button secondary" @click="pollLatestChatMessages" :disabled="chatLoadingOlder">刷新聊天</button>
            </div>

            <div ref="chatPanelRef" class="bot-chat-list" @scroll.passive="handleChatScroll">
              <div v-if="chatLoadingOlder" class="chat-loading">正在加载更早消息...</div>
              <div v-else-if="!chatHasMore && chatMessages.length > 0" class="chat-end">没有更多历史消息</div>
              <div v-if="groupedChatMessages.length === 0" class="empty-state">当前频道暂无聊天消息</div>
              <div v-for="item in groupedChatMessages" :key="item.id" class="chat-bubble-row" :class="{ own: isOwnMessage(item) }">
                <div class="chat-bubble">
                  <div class="bubble-meta">
                    <strong>{{ senderName(item) }}</strong>
                    <small>{{ formatTime(item.created_at) }}</small>
                  </div>
                  <div class="bubble-text">
                    {{ item.text || '[binary]' }}
                    <span v-if="item.mergedCount > 1" class="message-merge-count">x{{ item.mergedCount }}</span>
                  </div>
                  <div class="bubble-topic">{{ item.topic }}</div>
                </div>
              </div>
            </div>

            <div class="bot-chat-composer">
              <!-- <div class="segmented-control">
                <button :class="{ active: sendForm.message_type === 'channel' }" @click="sendForm.message_type = 'channel'">频道广播</button>
                <a class="direct-chat-link" href="/admin/bot/direct">打开私聊窗口</a>
              </div> -->
              <div class="composer-grid">
                <label>频道 ID<input v-model="sendForm.channel_id" /></label>
                <label v-if="sendForm.message_type === 'direct'">搜索目标<input v-model="targetQuery" placeholder="节点名 / !nodeid / node_num" /></label>
                <label v-if="sendForm.message_type === 'direct'" class="wide">目标节点
                  <select v-model="sendForm.to_node_id">
                    <option value="">选择目标节点</option>
                    <option v-for="node in targetOptions" :key="node.node_id" :value="node.node_id">
                      {{ node.long_name || node.short_name || node.node_id }} · {{ node.node_id }} · {{ node.node_num }}
                    </option>
                  </select>
                </label>
                <label class="wide">消息内容
                  <textarea v-model="sendForm.text" rows="3" placeholder="输入要发送的文本，Enter 换行。"></textarea>
                </label>
              </div>
              <div class="send-actions">
                <span class="hint" :class="{ warn: isTextTooLong }">{{ sendTextBytes }} / {{ maxTextBytes }} bytes</span>
                <button class="admin-button send-button" @click="sendMessage" :disabled="!canSend">{{ sending ? '发送中...' : '发送消息' }}</button>
              </div>
            </div>
          </section>
        </template>
        <div v-else class="panel empty-state large">请选择或创建一个机器人。</div>
      </main>
    </div>
  </section>
</template>

<style scoped>
.admin-bot-page { display: grid; gap: 12px; }
.bot-hero, .selected-summary { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 16px; padding: 16px; }
.bot-hero-actions, .row-actions, .send-actions, .section-title, .bot-chat-header { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.hint { color: #64748b; font-size: 13px; }
.hint.warn { color: #b91c1c; font-weight: 800; }
.bot-layout { display: grid; grid-template-columns: minmax(300px, 380px) minmax(0, 1fr); gap: 12px; align-items: start; }
.bot-sidebar, .bot-main-panel { display: grid; gap: 12px; }
.bot-sidebar { padding: 14px; }
.compact-form { display: grid; gap: 10px; }
.bot-form { border-bottom: 1px solid #e2e8f0; padding-bottom: 14px; }
.list-title { margin-top: 2px; }
label { display: grid; gap: 5px; color: #334155; font-size: 13px; font-weight: 800; }
label small { color: #64748b; font-weight: 600; }
label.inline { display: flex; align-items: center; gap: 8px; }
input, select, textarea { box-sizing: border-box; width: 100%; border: 1px solid #cbd5e1; border-radius: 10px; padding: 9px 11px; color: #0f172a; font: inherit; background: #fff; }
textarea { resize: vertical; line-height: 1.45; }
input:focus, select:focus, textarea:focus { outline: 2px solid #bfdbfe; border-color: #2563eb; }
.full { width: 100%; }
.bot-list { display: grid; gap: 10px; }
.bot-card { border: 1px solid #e2e8f0; border-radius: 14px; padding: 10px; background: #f8fafc; transition: border-color .15s, box-shadow .15s, background .15s; }
.bot-card.selected { border-color: #2563eb; background: #eff6ff; box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.12); }
.bot-card.disabled { opacity: 0.72; }
.bot-select { display: grid; grid-template-columns: 42px 1fr auto; align-items: center; gap: 10px; width: 100%; border: 0; padding: 0; color: inherit; text-align: left; background: transparent; }
.avatar { display: grid; place-items: center; width: 42px; height: 42px; border-radius: 12px; color: #1d4ed8; font-weight: 900; background: #dbeafe; }
.bot-main { display: grid; gap: 2px; min-width: 0; }
.bot-main strong, .bot-main small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.bot-main small { color: #64748b; }
.state-dot { width: 10px; height: 10px; border-radius: 999px; background: #cbd5e1; }
.state-dot.ok { background: #22c55e; }
.bot-details { margin-top: 10px; }
.bot-details summary { color: #2563eb; font-size: 13px; font-weight: 800; cursor: pointer; }
.bot-edit { margin-top: 10px; }
.selected-summary small { color: #64748b; font-size: 14px; }
.summary-grid { display: grid; grid-template-columns: repeat(4, minmax(110px, 1fr)); gap: 8px; flex: 1 1 100%; min-width: min(620px, 100%); }
.summary-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.summary-grid span, .stat-chip { display: grid; gap: 3px; border-radius: 12px; padding: 10px 12px; background: #f8fafc; }
.summary-grid strong { color: #0f172a; }
.summary-grid .public-key { grid-column: span 2; }
.summary-grid .public-key strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; }
.summary-grid small { color: #64748b; font-size: 12px; }
.stat-chip { display: inline-flex; align-items: center; color: #334155; font-size: 13px; font-weight: 800; background: #e2e8f0; }
.stat-chip.ok { color: #166534; background: #dcfce7; }
.bot-chat-panel { display: grid; grid-template-rows: auto minmax(320px, 1fr) auto; min-height: 680px; overflow: hidden; }
.bot-chat-header { padding: 14px 16px; border-bottom: 1px solid #e2e8f0; }
.bot-chat-list { min-height: 0; max-height: 520px; overflow: auto; display: flex; flex-direction: column; gap: 10px; padding: 14px; background: linear-gradient(180deg, #f8fafc 0%, #eef4ff 100%); }
.chat-loading, .chat-end { align-self: center; border-radius: 999px; padding: 6px 10px; color: #64748b; font-size: 12px; background: #e2e8f0; }
.chat-bubble-row { display: flex; justify-content: flex-start; }
.chat-bubble-row.own { justify-content: flex-end; }
.chat-bubble { max-width: min(680px, 78%); border: 1px solid #e2e8f0; border-radius: 16px 16px 16px 4px; padding: 10px 12px; background: #fff; box-shadow: 0 4px 16px rgba(15, 23, 42, 0.06); }
.chat-bubble-row.own .chat-bubble { border-color: #bfdbfe; border-radius: 16px 16px 4px 16px; background: #dbeafe; }
.bubble-meta { display: flex; align-items: center; justify-content: space-between; gap: 12px; color: #334155; font-size: 12px; }
.bubble-meta small, .bubble-topic { color: #64748b; }
.bubble-text { margin-top: 6px; color: #0f172a; line-height: 1.45; white-space: pre-wrap; word-break: break-word; }
.bubble-topic { margin-top: 6px; font-size: 11px; word-break: break-all; }
.message-merge-count { display: inline-flex; margin-left: 6px; border-radius: 999px; padding: 1px 6px; color: #1d4ed8; background: #bfdbfe; font-size: 12px; font-weight: 800; }
.bot-chat-composer { display: grid; gap: 12px; border-top: 1px solid #e2e8f0; padding: 14px 16px; background: #fff; }
.segmented-control { display: inline-flex; width: fit-content; border: 1px solid #cbd5e1; border-radius: 999px; padding: 3px; background: #f8fafc; }
.segmented-control button, .segmented-control .direct-chat-link { border: 0; border-radius: 999px; padding: 8px 13px; color: #475569; font-weight: 800; text-decoration: none; background: transparent; }
.segmented-control button.active { color: #fff; background: #2563eb; }
.composer-grid { display: grid; grid-template-columns: repeat(2, minmax(220px, 1fr)); gap: 12px; }
.composer-grid .wide { grid-column: 1 / -1; }
.send-button { min-width: 120px; }
.admin-button.secondary { color: #334155; background: #e2e8f0; }
.admin-button.danger { background: #dc2626; }
.empty-state { color: #64748b; padding: 16px; border: 1px dashed #cbd5e1; border-radius: 14px; text-align: center; background: #f8fafc; }
.empty-state.large { min-height: 260px; display: grid; place-items: center; }
@media (max-width: 1100px) {
  .bot-layout { grid-template-columns: 1fr; }
  .summary-grid { grid-template-columns: repeat(2, minmax(120px, 1fr)); }
}
@media (max-width: 700px) {
  .bot-hero, .selected-summary { align-items: stretch; flex-direction: column; }
  .composer-grid, .summary-grid { grid-template-columns: 1fr; }
  .bot-hero-actions { justify-content: flex-start; flex-wrap: wrap; }
  .chat-bubble { max-width: 92%; }
}
</style>
