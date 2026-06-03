<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getAdminLoginLogs } from '../api'
import type { AdminLoginLog } from '../types'

const logs = ref<AdminLoginLog[]>([])
const loading = ref(false)
const error = ref('')
const page = ref(1)
const pageSize = 100

const canPrev = () => page.value > 1
const canNext = () => logs.value.length === pageSize

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

async function refreshLogs() {
  loading.value = true
  error.value = ''
  try {
    const response = await getAdminLoginLogs(pageSize, (page.value - 1) * pageSize)
    logs.value = response.items
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

function changePage(nextPage: number) {
  page.value = Math.max(1, nextPage)
  refreshLogs()
}

onMounted(refreshLogs)
</script>

<template>
  <section class="admin-dashboard">
    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Login logs</p>
          <h2>登录日志</h2>
        </div>
        <button class="admin-button" @click="refreshLogs" :disabled="loading">{{ loading ? '刷新中...' : '刷新日志' }}</button>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>时间</th>
              <th>用户名</th>
              <th>结果</th>
              <th>原因</th>
              <th>Remote Addr</th>
              <th>Remote Host</th>
              <th>User-Agent</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in logs" :key="log.id">
              <td>{{ formatTime(log.created_at) }}</td>
              <td>{{ log.username || '-' }}</td>
              <td>
                <span class="log-badge" :class="log.success ? 'log-success' : 'log-failure'">
                  {{ log.success ? '成功' : '失败' }}
                </span>
              </td>
              <td>{{ log.reason || '-' }}</td>
              <td>{{ log.remote_addr || '-' }}</td>
              <td>{{ log.remote_host || '-' }}</td>
              <td>{{ log.user_agent || '-' }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="logs.length === 0" class="empty">暂无登录日志</div>
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
