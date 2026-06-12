<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getAdminMqttStatus, getAdminRuntimeSettings, updateAdminRuntimeSettings } from '../api'
import type { AdminMqttStatus, AdminRuntimeSettings } from '../types'

const status = ref<AdminMqttStatus | null>(null)
const runtimeSettings = ref<AdminRuntimeSettings | null>(null)
const loading = ref(false)
const settingsLoading = ref(false)
const error = ref('')
const settingsError = ref('')
const settingsMessage = ref('')
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

async function refreshRuntimeSettings() {
  settingsLoading.value = true
  settingsError.value = ''
  try {
    const response = await getAdminRuntimeSettings()
    runtimeSettings.value = response.item
  } catch (err) {
    settingsError.value = err instanceof Error ? err.message : String(err)
  } finally {
    settingsLoading.value = false
  }
}

async function saveEncryptedForwarding(value: boolean) {
  if (!runtimeSettings.value) {
    return
  }
  const previous = runtimeSettings.value.allow_encrypted_forwarding
  runtimeSettings.value.allow_encrypted_forwarding = value
  settingsLoading.value = true
  settingsError.value = ''
  settingsMessage.value = ''
  try {
    const response = await updateAdminRuntimeSettings({ allow_encrypted_forwarding: value })
    runtimeSettings.value = response.item
    settingsMessage.value = '设置已保存'
  } catch (err) {
    runtimeSettings.value.allow_encrypted_forwarding = previous
    settingsError.value = err instanceof Error ? err.message : String(err)
  } finally {
    settingsLoading.value = false
  }
}

onMounted(() => {
  refreshStatus()
  refreshRuntimeSettings()
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

    <div class="panel admin-status-panel mqtt-control-panel">
      <div class="panel-header control-header">
        <div class="control-title">
          <div>
            <p class="eyebrow">MQTT Forwarding</p>
            <h2>MQTT 转发控制</h2>
          </div>
        </div>
        <span class="control-badge" :class="{ active: runtimeSettings?.allow_encrypted_forwarding }">
          {{ runtimeSettings?.allow_encrypted_forwarding ? '加密包放行' : '默认拦截' }}
        </span>
      </div>
      <div class="control-body">
        <div class="control-copy">
          <h3>加密转发</h3>
          <p>
            控制 Broker 在无法解密 Meshtastic 加密包时是否仍允许转发。关闭时保持当前行为：无法解密的加密包会被丢弃并记录到丢弃详情。
          </p>
        </div>
        <div v-if="!runtimeSettings" class="empty control-empty">正在加载转发设置...</div>
        <label v-else class="switch-card" :class="{ enabled: runtimeSettings.allow_encrypted_forwarding, saving: settingsLoading }">
          <span class="switch-text">
            <strong>允许无法解密的加密包继续转发</strong>
            <small>{{ runtimeSettings.allow_encrypted_forwarding ? '已开启，原始 payload 将继续转发' : '已关闭，无法解密时会拒绝转发' }}</small>
          </span>
          <input
            type="checkbox"
            :checked="runtimeSettings.allow_encrypted_forwarding"
            :disabled="settingsLoading"
            @change="saveEncryptedForwarding(($event.target as HTMLInputElement).checked)"
          />
          <span class="switch-toggle" aria-hidden="true"></span>
        </label>
      </div>
      <p v-if="settingsError" class="error">{{ settingsError }}</p>
      <p v-if="settingsMessage" class="success">{{ settingsMessage }}</p>
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

<style scoped>
.mqtt-control-panel {
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  border: 1px solid var(--color-border);
  background: linear-gradient(135deg, var(--color-surface) 0%, var(--color-surface-soft) 100%);
}

.control-header {
  position: relative;
  align-items: flex-start;
}

.control-title {
  display: flex;
  align-items: center;
  gap: 0.85rem;
}

.control-badge {
  display: inline-flex;
  align-items: center;
  border: 1px solid var(--color-border);
  border-radius: 999px;
  padding: 6px 12px;
  color: var(--color-muted);
  font-size: 12px;
  font-weight: 800;
  background: color-mix(in srgb, var(--color-surface) 84%, transparent);
}

.control-badge.active {
  border-color: color-mix(in srgb, var(--color-success) 36%, white);
  color: color-mix(in srgb, var(--color-success) 72%, var(--color-heading));
  background: var(--color-success-soft);
}

.control-body {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 0.85fr);
  gap: 1rem;
  align-items: stretch;
}

.control-copy,
.switch-card {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--color-surface) 90%, transparent);
  box-shadow: var(--shadow-sm);
}

.control-copy {
  padding: 1rem;
}

.control-copy h3 {
  margin: 0 0 0.45rem;
  color: var(--color-heading);
  font-size: 18px;
}

.control-copy p {
  margin: 0;
  color: var(--color-muted);
  line-height: 1.7;
}

.control-empty {
  align-self: center;
}

.switch-card {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  min-height: 108px;
  padding: 1rem;
  color: var(--color-text);
  cursor: pointer;
  transition: transform 0.16s ease, border-color 0.16s ease, box-shadow 0.16s ease, background-color 0.16s ease;
}

.switch-card:hover {
  transform: translateY(-1px);
  border-color: var(--color-primary);
  box-shadow: var(--shadow-md);
}

.switch-card.enabled {
  border-color: color-mix(in srgb, var(--color-success) 42%, white);
  background: var(--color-success-soft);
}

.switch-card.saving {
  cursor: wait;
  opacity: 0.76;
}

.switch-card input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}

.switch-text {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.switch-text strong {
  color: var(--color-heading);
  font-size: 15px;
}

.switch-text small {
  color: var(--color-muted);
  font-size: 12px;
  line-height: 1.45;
}

.switch-toggle {
  position: relative;
  flex: 0 0 auto;
  width: 54px;
  height: 30px;
  border-radius: 999px;
  background: var(--color-border-strong);
  box-shadow: inset 0 2px 4px rgba(47, 52, 50, 0.12);
  transition: background-color 0.16s ease;
}

.switch-toggle::after {
  content: '';
  position: absolute;
  top: 4px;
  left: 4px;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  background: #fff;
  box-shadow: 0 4px 10px rgba(47, 52, 50, 0.18);
  transition: transform 0.16s ease;
}

.switch-card.enabled .switch-toggle {
  background: var(--color-success);
}

.switch-card.enabled .switch-toggle::after {
  transform: translateX(24px);
}

@media (max-width: 820px) {
  .control-body {
    grid-template-columns: 1fr;
  }

  .control-header {
    gap: 0.75rem;
  }
}
</style>
