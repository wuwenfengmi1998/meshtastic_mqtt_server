<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { createBotNode, deleteBotNode, getBotMessages, getBotNodes, getNodeInfo, sendBotMessage, updateBotNode } from '../api'
import type { BotMessage, BotMessageStatus, BotMessageType, BotNode, BotNodePayload, NodeInfo } from '../types'

const botPageSize = 100
const messagePageSize = 100
const maxTextBytes = 200

const bots = ref<BotNode[]>([])
const messages = ref<BotMessage[]>([])
const targets = ref<NodeInfo[]>([])
const selectedBotId = ref<number | null>(null)
const loading = ref(false)
const messageLoading = ref(false)
const saving = ref(false)
const sending = ref(false)
const error = ref('')
const message = ref('')
const targetQuery = ref('')

const newBot = ref({ node_num: '', long_name: '', short_name: '', default_channel_id: 'LongFast', enabled: true })
const edits = ref<Record<number, { node_num: string; long_name: string; short_name: string; default_channel_id: string; topic_prefix: string; enabled: boolean }>>({})
const sendForm = ref<{ message_type: BotMessageType; channel_id: string; to_node_id: string; text: string }>({ message_type: 'channel', channel_id: 'LongFast', to_node_id: '', text: '' })

const selectedBot = computed(() => bots.value.find((bot) => bot.id === selectedBotId.value) ?? null)
const enabledBots = computed(() => bots.value.filter((bot) => bot.enabled).length)
const sendTextBytes = computed(() => new TextEncoder().encode(sendForm.value.text).length)
const isTextTooLong = computed(() => sendTextBytes.value > maxTextBytes)
const recentMessages = computed(() => [...messages.value].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at)))
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
  if (sendForm.value.message_type === 'direct' && !sendForm.value.to_node_id) return false
  return true
})

watch(selectedBot, (bot) => {
  if (bot) {
    sendForm.value.channel_id = bot.default_channel_id
  }
})

function botPayload(form: { node_num: string; long_name: string; short_name: string; default_channel_id: string; topic_prefix?: string; enabled: boolean }): BotNodePayload {
  const nodeNumText = form.node_num.trim()
  return {
    node_num: nodeNumText ? Number(nodeNumText) : null,
    long_name: form.long_name.trim(),
    short_name: form.short_name.trim(),
    default_channel_id: form.default_channel_id.trim(),
    topic_prefix: form.topic_prefix?.trim() || 'msh/2/e',
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
    enabled: bot.enabled,
  }]))
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

async function refreshMessages() {
  if (!selectedBotId.value) {
    messages.value = []
    return
  }
  messageLoading.value = true
  try {
    const response = await getBotMessages(selectedBotId.value, messagePageSize, 0)
    messages.value = response.items
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    messageLoading.value = false
  }
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
  refreshMessages()
}

async function createBot() {
  saving.value = true
  error.value = ''
  message.value = ''
  try {
    await createBotNode(botPayload({ ...newBot.value, topic_prefix: 'msh/2/e' }))
    newBot.value = { node_num: '', long_name: '', short_name: '', default_channel_id: 'LongFast', enabled: true }
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
      messages.value = []
    }
    await refreshBots()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
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
      channel_id: sendForm.value.channel_id || selectedBot.value.default_channel_id,
      to_node_id: sendForm.value.message_type === 'direct' ? sendForm.value.to_node_id : undefined,
      text: sendForm.value.text,
    })
    if (response.error) {
      error.value = response.error
    } else {
      message.value = '消息已发送'
      sendForm.value.text = ''
    }
    await refreshMessages()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    sending.value = false
  }
}

function formatTime(value: string | null) {
  return value ? new Date(value).toLocaleString() : '-'
}

function statusText(status: BotMessageStatus) {
  return status === 'published' ? '已发送' : status === 'failed' ? '失败' : '等待中'
}

function targetLabel(item: BotMessage) {
  if (item.message_type === 'channel') return '频道广播'
  return item.to_node_id ? `私聊 ${item.to_node_id}` : '定向消息'
}

onMounted(() => {
  refreshBots()
  refreshTargets()
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
                <label>Topic 前缀<input v-model="edits[bot.id].topic_prefix" /></label>
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
            <div class="summary-grid">
              <span><strong>{{ selectedBot.node_id }}</strong><small>Node ID</small></span>
              <span><strong>{{ selectedBot.node_num }}</strong><small>Node Num</small></span>
              <span><strong>{{ selectedBot.default_channel_id }}</strong><small>默认频道</small></span>
              <span><strong>{{ selectedBot.enabled ? '启用' : '停用' }}</strong><small>状态</small></span>
            </div>
          </section>

          <section class="panel send-panel">
            <div class="section-title">
              <div>
                <p class="eyebrow">Compose</p>
                <h3>发送消息</h3>
              </div>
            </div>
            <div class="segmented-control">
              <button :class="{ active: sendForm.message_type === 'channel' }" @click="sendForm.message_type = 'channel'">频道广播</button>
              <button :class="{ active: sendForm.message_type === 'direct' }" @click="sendForm.message_type = 'direct'">定向消息</button>
            </div>
            <div class="send-grid">
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
                <textarea v-model="sendForm.text" rows="4" placeholder="输入要发送的文本，真实设备是否接受定向消息取决于固件兼容性。"></textarea>
              </label>
            </div>
            <div class="send-actions">
              <span class="hint" :class="{ warn: isTextTooLong }">{{ sendTextBytes }} / {{ maxTextBytes }} bytes</span>
              <button class="admin-button send-button" @click="sendMessage" :disabled="!canSend">{{ sending ? '发送中...' : '发送消息' }}</button>
            </div>
          </section>

          <section class="panel history-panel">
            <div class="history-header">
              <div>
                <p class="eyebrow">History</p>
                <h3>发送历史</h3>
              </div>
              <button class="admin-button secondary" @click="refreshMessages" :disabled="messageLoading">{{ messageLoading ? '刷新中...' : '刷新历史' }}</button>
            </div>
            <div class="message-list">
              <div v-if="recentMessages.length === 0" class="empty-state">暂无发送记录</div>
              <article v-for="item in recentMessages" :key="item.id" class="message-card" :class="item.status">
                <div class="message-head">
                  <div>
                    <span class="message-target">{{ targetLabel(item) }}</span>
                    <span class="message-time">{{ formatTime(item.created_at) }}</span>
                  </div>
                  <span class="status-badge" :class="item.status">{{ statusText(item.status) }}</span>
                </div>
                <p class="message-text">{{ item.text }}</p>
                <div class="message-meta">
                  <span>{{ item.channel_id }}</span>
                  <span>#{{ item.packet_id }}</span>
                  <span>{{ item.encrypted ? 'AES-CTR' : '明文' }}</span>
                </div>
                <p v-if="item.error" class="message-error">{{ item.error }}</p>
              </article>
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
.bot-hero, .selected-summary { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 16px; }
.bot-hero-actions, .row-actions, .history-header, .send-actions, .section-title { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
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
.summary-grid { display: grid; grid-template-columns: repeat(4, minmax(110px, 1fr)); gap: 8px; min-width: min(620px, 100%); }
.summary-grid span, .stat-chip { display: grid; gap: 3px; border-radius: 12px; padding: 10px 12px; background: #f8fafc; }
.summary-grid strong { color: #0f172a; }
.summary-grid small { color: #64748b; font-size: 12px; }
.stat-chip { display: inline-flex; align-items: center; color: #334155; font-size: 13px; font-weight: 800; background: #e2e8f0; }
.stat-chip.ok { color: #166534; background: #dcfce7; }
.send-panel, .history-panel { padding: 16px; display: grid; gap: 14px; }
.segmented-control { display: inline-flex; width: fit-content; border: 1px solid #cbd5e1; border-radius: 999px; padding: 3px; background: #f8fafc; }
.segmented-control button { border: 0; border-radius: 999px; padding: 8px 13px; color: #475569; font-weight: 800; background: transparent; }
.segmented-control button.active { color: #fff; background: #2563eb; }
.send-grid { display: grid; grid-template-columns: repeat(2, minmax(220px, 1fr)); gap: 12px; }
.send-grid .wide { grid-column: 1 / -1; }
.send-button { min-width: 120px; }
.admin-button.secondary { color: #334155; background: #e2e8f0; }
.admin-button.danger { background: #dc2626; }
.message-list { display: grid; gap: 10px; max-height: 520px; overflow: auto; padding-right: 4px; }
.message-card { border: 1px solid #e2e8f0; border-radius: 14px; padding: 12px; background: #fff; }
.message-card.published { border-color: #bbf7d0; }
.message-card.failed { border-color: #fecaca; background: #fff7f7; }
.message-head { display: flex; justify-content: space-between; gap: 10px; }
.message-target { display: block; color: #0f172a; font-weight: 900; }
.message-time, .message-meta { color: #64748b; font-size: 12px; }
.message-text { margin: 10px 0; color: #0f172a; line-height: 1.45; white-space: pre-wrap; word-break: break-word; }
.message-meta { display: flex; flex-wrap: wrap; gap: 8px; }
.status-badge { border-radius: 999px; padding: 4px 8px; font-size: 12px; font-weight: 900; white-space: nowrap; }
.status-badge.published { color: #166534; background: #dcfce7; }
.status-badge.failed { color: #991b1b; background: #fee2e2; }
.status-badge.pending { color: #92400e; background: #fef3c7; }
.message-error { margin: 10px 0 0; border-radius: 10px; padding: 8px 10px; color: #991b1b; background: #fee2e2; }
.empty-state { color: #64748b; padding: 16px; border: 1px dashed #cbd5e1; border-radius: 14px; text-align: center; background: #f8fafc; }
.empty-state.large { min-height: 260px; display: grid; place-items: center; }
@media (max-width: 1100px) {
  .bot-layout { grid-template-columns: 1fr; }
  .summary-grid { grid-template-columns: repeat(2, minmax(120px, 1fr)); }
}
@media (max-width: 700px) {
  .bot-hero, .selected-summary { align-items: stretch; flex-direction: column; }
  .send-grid, .summary-grid { grid-template-columns: 1fr; }
  .bot-hero-actions { justify-content: flex-start; flex-wrap: wrap; }
}
</style>
