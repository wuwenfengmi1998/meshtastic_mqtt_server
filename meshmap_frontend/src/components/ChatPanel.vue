<script setup lang="ts">
import type { NodeInfoById, TextMessage } from '../types'

const props = defineProps<{
  messages: TextMessage[]
  nodesById: NodeInfoById
  selectedNodeId: string | null
}>()

const emit = defineEmits<{
  'select-node': [nodeId: string]
}>()

function senderName(message: TextMessage): string {
  const node = props.nodesById[message.from_id]
  return node?.long_name || node?.short_name || message.from_id
}

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}
</script>

<template>
  <aside class="chat-panel panel">
    <div class="panel-header">
      <div>
        <p class="eyebrow">Chat</p>
        <h2>聊天信息</h2>
      </div>
      <span class="badge">{{ messages.length }}</span>
    </div>

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
      <span class="chat-host">{{ message.mqtt_remote_host || message.topic }}</span>
    </button>
  </aside>
</template>
