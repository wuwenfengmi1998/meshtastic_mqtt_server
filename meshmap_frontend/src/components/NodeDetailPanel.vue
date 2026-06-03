<script setup lang="ts">
import type { MapNode, NodeInfoMap, PositionRecord, TextMessage } from '../types'

const props = defineProps<{
  node: NodeInfoMap | null
  mapNode: MapNode | null
  messages: TextMessage[]
  positions: PositionRecord[]
}>()

function nodeLabel(): string {
  return props.node?.long_name || props.node?.short_name || props.mapNode?.label || props.mapNode?.node_id || '未选择节点'
}

function formatTime(value: string | null | undefined): string {
  if (!value) {
    return '-'
  }
  return new Date(value).toLocaleString()
}
</script>

<template>
  <section class="node-detail-panel panel">
    <div class="panel-header">
      <div>
        <p class="eyebrow">Node detail</p>
        <h2>{{ nodeLabel() }}</h2>
      </div>
      <span v-if="mapNode" class="badge">{{ mapNode.source }}</span>
    </div>

    <div v-if="!node && !mapNode" class="empty">点击聊天消息或地图节点查看详情</div>
    <div v-else class="detail-grid">
      <div class="detail-main">
        <dl>
          <div><dt>Node ID</dt><dd>{{ node?.node_id || mapNode?.node_id }}</dd></div>
          <div><dt>Role</dt><dd>{{ node?.role || '-' }}</dd></div>
          <div><dt>Hardware</dt><dd>{{ node?.hw_model || '-' }}</dd></div>
          <div><dt>Latitude</dt><dd>{{ mapNode?.latitude ?? node?.latitude ?? '-' }}</dd></div>
          <div><dt>Longitude</dt><dd>{{ mapNode?.longitude ?? node?.longitude ?? '-' }}</dd></div>
          <div><dt>Altitude</dt><dd>{{ mapNode?.altitude ?? node?.altitude ?? '-' }}</dd></div>
          <div><dt>Updated</dt><dd>{{ formatTime(node?.updated_at || mapNode?.updated_at) }}</dd></div>
        </dl>
      </div>

      <div class="detail-side">
        <h3>最近消息</h3>
        <p v-if="messages.length === 0" class="muted">暂无消息</p>
        <ul v-else>
          <li v-for="message in messages.slice(0, 5)" :key="message.id">
            {{ message.text || '[binary]' }}
          </li>
        </ul>
      </div>

      <div class="detail-side">
        <h3>最近位置</h3>
        <p v-if="positions.length === 0" class="muted">暂无位置</p>
        <ul v-else>
          <li v-for="position in positions.slice(0, 5)" :key="position.id">
            {{ position.latitude ?? '-' }}, {{ position.longitude ?? '-' }} · {{ formatTime(position.created_at) }}
          </li>
        </ul>
      </div>
    </div>
  </section>
</template>
