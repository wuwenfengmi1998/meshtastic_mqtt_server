<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onBeforeUpdate, onMounted, onUpdated, ref } from 'vue'
import type { NodeInfoById, TextMessage } from '../types'

const props = defineProps<{
  messages: TextMessage[]
  nodesById: NodeInfoById
  selectedNodeId: string | null
  loadingOlder: boolean
  hasMoreMessages: boolean
  isAdmin: boolean
}>()

type GroupedTextMessage = TextMessage & { mergedCount: number; mergedMessages: TextMessage[] }

const emit = defineEmits<{
  'select-node': [nodeId: string]
  'load-older': []
  'delete-message': [message: GroupedTextMessage]
  'delete-and-block-node': [payload: { nodeId: string; nodeNum: number | null; message: GroupedTextMessage }]
}>()

const panelRef = ref<HTMLElement | null>(null)
const menuMessage = ref<GroupedTextMessage | null>(null)
const menuX = ref(0)
const menuY = ref(0)
const topThreshold = 8
const bottomThreshold = 40
const scrollOverflowAllowance = 1

const groupedMessages = computed<GroupedTextMessage[]>(() => {
  const groups = new Map<string, GroupedTextMessage>()
  for (const message of props.messages) {
    const key = `${message.packet_id ?? ''}\n${message.text ?? ''}`
    const group = groups.get(key)
    if (group) {
      group.mergedCount += 1
      group.mergedMessages.push(message)
    } else {
      groups.set(key, { ...message, mergedCount: 1, mergedMessages: [message] })
    }
  }
  return Array.from(groups.values())
})

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

function nodeDetailHref(nodeId: string): string {
  return `/detailed/${encodeURIComponent(nodeId)}`
}

function openMessageMenu(message: GroupedTextMessage, event: MouseEvent) {
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

function deleteAndBlockSelectedNode() {
  if (menuMessage.value) {
    emit('delete-and-block-node', { nodeId: menuMessage.value.from_id, nodeNum: menuMessage.value.from_num ?? null, message: menuMessage.value })
  }
  closeMessageMenu()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeMessageMenu()
  }
}

function loadOlderFromCurrentScroll(el: HTMLElement) {
  if (
    props.loadingOlder ||
    !props.hasMoreMessages ||
    groupedMessages.value.length === 0 ||
    restoreScrollHeight != null
  ) {
    return
  }

  restoreScrollHeight = el.scrollHeight
  restoreScrollTop = el.scrollTop
  restoreMessageCount = groupedMessages.value.length
  emit('load-older')
}

function handleScroll() {
  closeMessageMenu()
  const el = panelRef.value
  if (!el || el.scrollTop > topThreshold) {
    return
  }
  loadOlderFromCurrentScroll(el)
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
    if (el.scrollHeight <= el.clientHeight + scrollOverflowAllowance) {
      loadOlderFromCurrentScroll(el)
    }
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
    if (groupedMessages.value.length > restoreMessageCount) {
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

  if (el.scrollHeight <= el.clientHeight + scrollOverflowAllowance) {
    loadOlderFromCurrentScroll(el)
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
      <span class="badge">{{ groupedMessages.length }}</span>
    </div>

    <div v-if="loadingOlder" class="chat-loading">正在加载更早消息...</div>
    <div v-else-if="!hasMoreMessages && messages.length > 0" class="chat-end">没有更多历史消息</div>
    <div v-if="messages.length === 0" class="empty">暂无聊天消息</div>
    <button
      v-for="message in groupedMessages"
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
      <span class="chat-text">
        {{ message.text || '[binary]' }}
        <span v-if="message.mergedCount > 1" class="message-merge-count">x{{ message.mergedCount }}</span>
      </span>
    </button>

    <div
      v-if="menuMessage"
      class="context-menu"
      :style="{ left: `${menuX}px`, top: `${menuY}px` }"
      @click.stop
    >
      <a :href="nodeDetailHref(menuMessage.from_id)">节点详细</a>
      <button v-if="isAdmin" class="danger" type="button" @click="deleteSelectedMessage">删除</button>
      <button v-if="isAdmin" class="danger" type="button" @click="deleteAndBlockSelectedNode">删除并屏蔽节点</button>
    </div>
  </aside>
</template>
