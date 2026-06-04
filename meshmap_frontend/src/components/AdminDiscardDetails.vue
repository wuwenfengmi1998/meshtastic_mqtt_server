<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getDiscardDetails } from '../api'
import type { DiscardDetails } from '../types'

const items = ref<DiscardDetails[]>([])
const loading = ref(false)
const error = ref('')
const page = ref(1)
const pageSize = 25

const canPrev = () => page.value > 1
const canNext = () => items.value.length === pageSize

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
  refreshItems()
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
        <button class="admin-button" @click="refreshItems" :disabled="loading">{{ loading ? '刷新中...' : '刷新数据' }}</button>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>时间</th>
              <th>Topic</th>
              <th>Error</th>
              <th>Payload Len</th>
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
              <td>{{ formatTime(item.created_at) }}</td>
              <td>{{ item.topic || '-' }}</td>
              <td>{{ item.error || '-' }}</td>
              <td>{{ item.payload_len }}</td>
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
  </section>
</template>
