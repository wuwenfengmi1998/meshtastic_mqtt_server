<script setup lang="ts">
import { computed } from 'vue'
import type { NodeInfo } from '../types'

const props = defineProps<{
  nodes: NodeInfo[]
  selectedNodeId: string | null
  page: number
  pageSize: number
  total: number
  loading: boolean
}>()

const emit = defineEmits<{
  'select-node': [nodeId: string]
  'page-change': [page: number]
}>()

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const canPrev = computed(() => props.page > 1)
const canNext = computed(() => props.page < totalPages.value)

function nodeName(node: NodeInfo): string {
  return node.long_name || node.short_name || node.node_id
}

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}
</script>

<template>
  <section class="node-list-panel panel">
    <div class="panel-header">
      <div>
        <p class="eyebrow">NodeInfo</p>
        <h2>节点列表</h2>
      </div>
      <span class="badge">共 {{ total }} 条</span>
    </div>

    <div class="node-table-wrap">
      <table class="node-table">
        <thead>
          <tr>
            <th>节点</th>
            <th>Node ID</th>
            <th>User ID</th>
            <th>角色</th>
            <th>硬件</th>
            <th>更新时间</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="node in nodes"
            :key="node.node_id"
            class="node-row"
            :class="{ selected: selectedNodeId === node.node_id }"
            @click="emit('select-node', node.node_id)"
          >
            <td>{{ nodeName(node) }}</td>
            <td>{{ node.node_id }}</td>
            <td>{{ node.user_id || '-' }}</td>
            <td>{{ node.role || '-' }}</td>
            <td>{{ node.hw_model || '-' }}</td>
            <td>{{ formatTime(node.updated_at) }}</td>
          </tr>
        </tbody>
      </table>
      <div v-if="nodes.length === 0" class="empty">暂无节点数据</div>
    </div>

    <div class="pagination">
      <button :disabled="loading || !canPrev" @click="emit('page-change', page - 1)">上一页</button>
      <span>第 {{ page }} / {{ totalPages }} 页</span>
      <span>每页 {{ pageSize }} 条</span>
      <button :disabled="loading || !canNext" @click="emit('page-change', page + 1)">下一页</button>
    </div>
  </section>
</template>
