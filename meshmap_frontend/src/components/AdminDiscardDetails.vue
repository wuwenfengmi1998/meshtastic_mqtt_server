<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { clearDiscardDetails, deleteDiscardDetailsByIDs, getDiscardDetails } from '../api'
import type { DiscardDetails } from '../types'
import ConfirmDeleteModal from './ConfirmDeleteModal.vue'

const items = ref<DiscardDetails[]>([])
const loading = ref(false)
const error = ref('')
const page = ref(1)
const pageSize = 25

const selectedIds = ref<Set<number>>(new Set())
const pendingAction = ref<'batch' | 'clear' | null>(null)

const canPrev = () => page.value > 1
const canNext = () => items.value.length === pageSize

const allSelected = computed(
  () => items.value.length > 0 && items.value.every((item) => selectedIds.value.has(item.id)),
)
const someSelected = computed(() => selectedIds.value.size > 0 && !allSelected.value)
const selectedCount = computed(() => selectedIds.value.size)

const modalTitle = computed(() =>
  pendingAction.value === 'clear' ? '清空丢弃数据' : '批量删除丢弃数据',
)
const modalMessage = computed(() =>
  pendingAction.value === 'clear'
    ? '确定要清空全部丢弃数据吗？此操作不可撤销。'
    : `确定要删除选中的 ${selectedCount.value} 条丢弃数据吗？此操作不可撤销。`,
)
const modalConfirmText = computed(() =>
  pendingAction.value === 'clear' ? '清空全部' : '删除',
)

function toggleAll(checked: boolean) {
  if (checked) {
    items.value.forEach((item) => selectedIds.value.add(item.id))
  } else {
    selectedIds.value.clear()
  }
  selectedIds.value = new Set(selectedIds.value)
}

function toggleRow(id: number, checked: boolean) {
  if (checked) {
    selectedIds.value.add(id)
  } else {
    selectedIds.value.delete(id)
  }
  selectedIds.value = new Set(selectedIds.value)
}

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

async function refreshItems() {
  loading.value = true
  error.value = ''
  try {
    const response = await getDiscardDetails(pageSize, (page.value - 1) * pageSize)
    items.value = response.items
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

function changePage(nextPage: number) {
  page.value = Math.max(1, nextPage)
  selectedIds.value.clear()
  selectedIds.value = new Set(selectedIds.value)
  refreshItems()
}

function requestBatchDelete() {
  if (selectedCount.value === 0) return
  pendingAction.value = 'batch'
}

function requestClearAll() {
  pendingAction.value = 'clear'
}

function cancelDeleteModal() {
  pendingAction.value = null
}

async function confirmDeleteModal() {
  const action = pendingAction.value
  pendingAction.value = null
  if (!action) return
  try {
    if (action === 'clear') {
      await clearDiscardDetails()
    } else {
      await deleteDiscardDetailsByIDs([...selectedIds.value])
    }
    selectedIds.value.clear()
    selectedIds.value = new Set(selectedIds.value)
    page.value = 1
    await refreshItems()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(refreshItems)
</script>

<template>
  <section class="admin-dashboard">
    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Discard details</p>
          <h2>丢弃数据</h2>
        </div>
        <div style="display: flex; gap: 8px;">
          <button
            class="admin-button admin-button-danger"
            :disabled="loading || selectedCount === 0"
            @click="requestBatchDelete"
          >批量删除{{ selectedCount > 0 ? `（${selectedCount}）` : '' }}</button>
          <button
            class="admin-button admin-button-danger"
            :disabled="loading"
            @click="requestClearAll"
          >清空全部</button>
          <button class="admin-button" @click="refreshItems" :disabled="loading">{{ loading ? '刷新中...' : '刷新数据' }}</button>
        </div>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <div class="node-table-wrap table-fit-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th style="width: 40px;" class="cell-nowrap">
                <input
                  type="checkbox"
                  :checked="allSelected"
                  :indeterminate.prop="someSelected"
                  :disabled="items.length === 0"
                  @change="toggleAll(($event.target as HTMLInputElement).checked)"
                />
              </th>
              <th class="cell-nowrap">时间</th>
              <th>Topic</th>
              <th>Error</th>
              <th class="cell-nowrap">Payload Len</th>
              <th>Client ID</th>
              <th>Username</th>
              <th>Listener</th>
              <th>Remote Host</th>
              <th>Raw Base64</th>
              <th>Content JSON</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.id">
              <td class="cell-nowrap">
                <input
                  type="checkbox"
                  :checked="selectedIds.has(item.id)"
                  @change="toggleRow(item.id, ($event.target as HTMLInputElement).checked)"
                />
              </td>
              <td class="cell-nowrap">{{ formatTime(item.created_at) }}</td>
              <td>{{ item.topic || '-' }}</td>
              <td>{{ item.error || '-' }}</td>
              <td class="cell-nowrap">{{ item.payload_len }}</td>
              <td>{{ item.mqtt_client_id || '-' }}</td>
              <td>{{ item.mqtt_username || '-' }}</td>
              <td>{{ item.mqtt_listener || '-' }}</td>
              <td>{{ item.mqtt_remote_host || '-' }}</td>
              <td><pre class="discard-raw">{{ item.raw_base64 }}</pre></td>
              <td><pre class="discard-json">{{ item.content_json }}</pre></td>
            </tr>
          </tbody>
        </table>
        <div v-if="items.length === 0" class="empty">暂无丢弃数据</div>
      </div>

      <div class="pagination">
        <button :disabled="loading || !canPrev()" @click="changePage(page - 1)">上一页</button>
        <span>第 {{ page }} 页</span>
        <span>每页 {{ pageSize }} 条</span>
        <button :disabled="loading || !canNext()" @click="changePage(page + 1)">下一页</button>
      </div>
    </div>

    <ConfirmDeleteModal
      :open="pendingAction !== null"
      :title="modalTitle"
      :message="modalMessage"
      :confirm-text="modalConfirmText"
      @cancel="cancelDeleteModal"
      @confirm="confirmDeleteModal"
    />
  </section>
</template>
