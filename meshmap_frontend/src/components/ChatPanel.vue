<script setup lang="ts">
import { nextTick, onBeforeUpdate, onMounted, onUpdated, ref } from 'vue'
import type { NodeInfoById, TextMessage } from '../types'

const props = defineProps<{
  messages: TextMessage[]
  nodesById: NodeInfoById
  selectedNodeId: string | null
  loadingOlder: boolean
  hasMoreMessages: boolean
}>()

const emit = defineEmits<{
  'select-node': [nodeId: string]
  'load-older': []
}>()

const panelRef = ref<HTMLElement | null>(null)
const topThreshold = 8
const bottomThreshold = 40

let didInitialScroll = false
let shouldStickToBottom = true
let restoreScrollHeight: number | null = null
let restoreScrollTop = 0
let restoreMessageCount = 0

function senderName(message: TextMessage): string {
  const node = props.nodesById[message.from_id]
  return node?.long_name || node?.short_name || message.from_id
}

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

function isNearBottom(el: HTMLElement): boolean {
  return el.scrollHeight - el.scrollTop - el.clientHeight <= bottomThreshold
}

function clearRestoreState() {
  restoreScrollHeight = null
  restoreScrollTop = 0
  restoreMessageCount = 0
}

function handleScroll() {
  const el = panelRef.value
  if (
    !el ||
    props.loadingOlder ||
    !props.hasMoreMessages ||
    props.messages.length === 0 ||
    restoreScrollHeight != null
  ) {
    return
  }

  if (el.scrollTop <= topThreshold) {
    restoreScrollHeight = el.scrollHeight
    restoreScrollTop = el.scrollTop
    restoreMessageCount = props.messages.length
    emit('load-older')
  }
}

onBeforeUpdate(() => {
  const el = panelRef.value
  if (el) {
    shouldStickToBottom = isNearBottom(el)
  }
})

onMounted(async () => {
  await nextTick()
  const el = panelRef.value
  if (el) {
    el.scrollTop = el.scrollHeight
    didInitialScroll = true
  }
})

onUpdated(() => {
  const el = panelRef.value
  if (!el) {
    return
  }

  if (restoreScrollHeight != null) {
    if (props.messages.length > restoreMessageCount) {
      el.scrollTop = el.scrollHeight - restoreScrollHeight + restoreScrollTop
      clearRestoreState()
      return
    }
    if (!props.loadingOlder) {
      clearRestoreState()
    }
  }

  if (!didInitialScroll || shouldStickToBottom) {
    el.scrollTop = el.scrollHeight
    didInitialScroll = true
  }
})
</script>

<template>
  <aside ref="panelRef" class="chat-panel panel" @scroll.passive="handleScroll">
    <div class="panel-header">
      <div>
        <p class="eyebrow">Chat</p>
        <h2>聊天信息</h2>
      </div>
      <span class="badge">{{ messages.length }}</span>
    </div>

    <div v-if="loadingOlder" class="chat-loading">正在加载更早消息...</div>
    <div v-else-if="!hasMoreMessages && messages.length > 0" class="chat-end">没有更多历史消息</div>
    <div v-if="messages.length === 0" class="empty">暂无聊天消息</div>
    <button
      v-for="message in messages"
      :key="message.id"
      class="chat-item"
      :class="{ selected: selectedNodeId === message.from_id }"
      type="button"
      @click="emit('select-node', message.from_id)"
    >
      <span class="chat-meta">
        <strong>{{ senderName(message) }}</strong>
        <small>{{ formatTime(message.created_at) }}</small>
      </span>
      <span class="chat-text">{{ message.text || '[binary]' }}</span>
    </button>
  </aside>
</template>
