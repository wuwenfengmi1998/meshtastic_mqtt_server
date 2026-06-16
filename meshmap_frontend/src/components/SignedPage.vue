<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getSignRecords } from '../api'
import type { SignRecord } from '../types'

const pageSize = 25
const records = ref<SignRecord[]>([])
const loading = ref(false)
const error = ref('')
const page = ref(1)
const total = ref(0)

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

function canPrev(): boolean {
  return page.value > 1
}

function canNext(): boolean {
  return page.value * pageSize < total.value || records.value.length === pageSize
}

async function loadRecords(nextPage = page.value) {
  loading.value = true
  error.value = ''
  try {
    const safePage = Math.max(1, nextPage)
    const response = await getSignRecords(pageSize, (safePage - 1) * pageSize)
    records.value = response.items
    total.value = response.total ?? response.offset + response.items.length
    page.value = safePage
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(() => loadRecords())
</script>

<template>
  <section class="admin-dashboard">
    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Signed</p>
          <h2>签到用户</h2>
        </div>
        <span class="counter">共 {{ total }} 条签到记录</span>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>节点 ID</th>
              <th>Long Name</th>
              <th>Short Name</th>
              <th>签到文本</th>
              <th>签到时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="record in records" :key="record.id">
              <td>{{ record.node_id }}</td>
              <td>{{ record.long_name || '-' }}</td>
              <td>{{ record.short_name || '-' }}</td>
              <td>{{ record.sign_text }}</td>
              <td>{{ formatTime(record.sign_time) }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="records.length === 0" class="empty">{{ loading ? '加载中...' : '暂无签到记录' }}</div>
      </div>

      <div class="pagination">
        <button class="admin-button" :disabled="!canPrev() || loading" @click="loadRecords(page - 1)">上一页</button>
        <span>第 {{ page }} 页</span>
        <button class="admin-button" :disabled="!canNext() || loading" @click="loadRecords(page + 1)">下一页</button>
      </div>
    </div>
  </section>
</template>
