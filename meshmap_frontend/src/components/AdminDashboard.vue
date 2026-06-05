<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getAdminMqttStatus } from '../api'
import type { AdminMqttStatus } from '../types'

const status = ref<AdminMqttStatus | null>(null)
const loading = ref(false)
const error = ref('')
let timer: number | undefined

function formatUptime(seconds: number): string {
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60
  return `${hours}h ${minutes}m ${secs}s`
}

async function refreshStatus() {
  loading.value = true
  error.value = ''
  try {
    status.value = await getAdminMqttStatus()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  refreshStatus()
  timer = window.setInterval(refreshStatus, 5000)
})

onBeforeUnmount(() => {
  if (timer !== undefined) {
    window.clearInterval(timer)
  }
})
</script>

<template>
  <section class="admin-dashboard">
    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Admin</p>
          <h2>MQTT 服务状态</h2>
        </div>
        <div class="admin-actions">
          <button @click="refreshStatus" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
        </div>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <div v-if="!status" class="empty">正在加载 MQTT 状态...</div>
      <div v-else class="admin-status-grid">
        <div><span>运行状态</span><strong>{{ status.running ? '运行中' : '未运行' }}</strong></div>
        <div><span>监听地址</span><strong>{{ status.address || '-' }}</strong></div>
        <div><span>TLS</span><strong>{{ status.tls ? '启用' : '未启用' }}</strong></div>
        <div><span>Uptime</span><strong>{{ formatUptime(status.uptime || 0) }}</strong></div>
        <div><span>当前连接</span><strong>{{ status.clients_connected }}</strong></div>
        <div><span>订阅数</span><strong>{{ status.subscriptions }}</strong></div>
        <div><span>转发消息</span><strong>{{ status.messages_sent }}</strong></div>
        <div><span>数据库队列</span><strong>{{ status.db_write_queue_length }}</strong></div>
        <a class="status-card-link" href="/admin/discard_details"><span>丢弃消息</span><strong>{{ status.messages_dropped }}</strong></a>
        <div><span>收到包</span><strong>{{ status.packets_received }}</strong></div>
        <div><span>发送包</span><strong>{{ status.packets_sent }}</strong></div>
      </div>
    </div>

    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Clients</p>
          <h2>MQTT 客户端</h2>
        </div>
        <span class="badge">{{ status?.clients?.length ?? 0 }}</span>
      </div>
      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>Client ID</th>
              <th>Username</th>
              <th>Listener</th>
              <th>Remote Addr</th>
              <th>Remote Host</th>
              <th>Remote Port</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="client in status?.clients || []" :key="client.client_id">
              <td>{{ client.client_id || '-' }}</td>
              <td>{{ client.username || '-' }}</td>
              <td>{{ client.listener || '-' }}</td>
              <td>{{ client.remote_addr || '-' }}</td>
              <td>{{ client.remote_host || '-' }}</td>
              <td>{{ client.remote_port || '-' }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="!status?.clients?.length" class="empty">暂无客户端连接</div>
      </div>
    </div>
  </section>
</template>
