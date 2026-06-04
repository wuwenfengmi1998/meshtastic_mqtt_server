<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { NodeInfo } from '../types'

const props = defineProps<{
  nodes: NodeInfo[]
  selectedNodeId: string | null
  page: number
  pageSize: number
  total: number
  loading: boolean
  isAdmin: boolean
}>()

const emit = defineEmits<{
  'select-node': [nodeId: string]
  'page-change': [page: number]
  'delete-node': [nodeId: string]
  'delete-and-block-node': [payload: { nodeId: string; nodeNum: number | null }]
}>()

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))
const canPrev = computed(() => props.page > 1)
const canNext = computed(() => props.page < totalPages.value)
const menuNode = ref<NodeInfo | null>(null)
const menuX = ref(0)
const menuY = ref(0)

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

function closeNodeMenu() {
  menuNode.value = null
}

function nodeDetailHref(nodeId: string): string {
  return `/detailed/${encodeURIComponent(nodeId)}`
}

function openNodeMenu(node: NodeInfo, event: MouseEvent) {
  emit('select-node', node.node_id)
  menuNode.value = node
  menuX.value = event.clientX
  menuY.value = event.clientY
}

function deleteSelectedNode() {
  if (menuNode.value) {
    emit('delete-node', menuNode.value.node_id)
  }
  closeNodeMenu()
}

function deleteAndBlockSelectedNode() {
  if (menuNode.value) {
    emit('delete-and-block-node', { nodeId: menuNode.value.node_id, nodeNum: menuNode.value.node_num ?? null })
  }
  closeNodeMenu()
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeNodeMenu()
  }
}

onMounted(() => {
  window.addEventListener('click', closeNodeMenu)
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('click', closeNodeMenu)
  window.removeEventListener('keydown', handleKeydown)
})
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

    <div class="node-table-wrap" @scroll="closeNodeMenu">
      <table class="node-table">
        <thead>
          <tr>
            <th>Node ID</th>
            <th>Long Name</th>
            <th>Short Name</th>
            <th>硬件</th>
            <th>角色</th>
            <th>Public Key</th>
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
            @contextmenu.prevent.stop="openNodeMenu(node, $event)"
          >
            <td>{{ node.node_id }}</td>
            <td>{{ node.long_name || '-' }}</td>
            <td>{{ node.short_name || '-' }}</td>
            <td>{{ node.hw_model || '-' }}</td>
            <td>{{ node.role || '-' }}</td>
            <td>{{ node.public_key || '-' }}</td>
            <td>{{ formatTime(node.updated_at) }}</td>
          </tr>
        </tbody>
      </table>
      <div v-if="nodes.length === 0" class="empty">暂无节点数据</div>
    </div>

    <div
      v-if="menuNode"
      class="context-menu"
      :style="{ left: `${menuX}px`, top: `${menuY}px` }"
      @click.stop
    >
      <a :href="nodeDetailHref(menuNode.node_id)">节点详细</a>
      <button v-if="isAdmin" class="danger" type="button" @click="deleteSelectedNode">删除</button>
      <button v-if="isAdmin" class="danger" type="button" @click="deleteAndBlockSelectedNode">删除并屏蔽节点</button>
    </div>

    <div class="pagination">
      <button :disabled="loading || !canPrev" @click="emit('page-change', page - 1)">上一页</button>
      <span>第 {{ page }} / {{ totalPages }} 页</span>
      <span>每页 {{ pageSize }} 条</span>
      <button :disabled="loading || !canNext" @click="emit('page-change', page + 1)">下一页</button>
    </div>
  </section>
</template>
