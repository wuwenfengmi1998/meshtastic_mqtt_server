<script setup lang="ts">
import { nextTick, onBeforeUnmount, onBeforeUpdate, onMounted, onUpdated, ref } from 'vue'
import type { NodeInfoById, TextMessage } from '../types'

const props = defineProps<{
  messages: TextMessage[]
  nodesById: NodeInfoById
  selectedNodeId: string | null
  loadingOlder: boolean
  hasMoreMessages: boolean
  isAdmin: boolean
}>()

const emit = defineEmits<{
  'select-node': [nodeId: string]
  'load-older': []
  'delete-message': [message: TextMessage]
}>()

const panelRef = ref<HTMLElement | null>(null)
const menuMessage = ref<TextMessage | null>(null)
const menuX = ref(0)
const menuY = ref(0)
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

function closeMessageMenu() {
  menuMessage.value = null
}

function openMessageMenu(message: TextMessage, event: MouseEvent) {
  if (!props.isAdmin) {
    return
  }
  emit('select-node', message.from_id)
  menuMessage.value = message
  menuX.value = event.clientX
  menuY.value = event.clientY
}

function deleteSelectedMessage() {
  if (menuMessage.value) {
    emit('delete-message', menuMessage.value)
  }
  closeMessageMenu()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeMessageMenu()
  }
}

function handleScroll() {
  closeMessageMenu()
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
  window.addEventListener('click', closeMessageMenu)
  window.addEventListener('keydown', handleKeydown)
  await nextTick()
  const el = panelRef.value
  if (el) {
    el.scrollTop = el.scrollHeight
    didInitialScroll = true
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('click', closeMessageMenu)
  window.removeEventListener('keydown', handleKeydown)
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
      @contextmenu.prevent.stop="openMessageMenu(message, $event)"
    >
      <span class="chat-meta">
        <strong>{{ senderName(message) }}</strong>
        <small>{{ formatTime(message.created_at) }}</small>
      </span>
      <span class="chat-topic">{{ message.topic }}</span>
      <span class="chat-text">{{ message.text || '[binary]' }}</span>
    </button>

    <div
      v-if="menuMessage"
      class="context-menu"
      :style="{ left: `${menuX}px`, top: `${menuY}px` }"
      @click.stop
    >
      <button class="danger" type="button" @click="deleteSelectedMessage">删除</button>
    </div>
  </aside>
</template>
