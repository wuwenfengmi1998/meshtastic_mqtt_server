<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { adminLogout, createAdminUser, getAdminMqttStatus, getAdminUsers, updateAdminUserPassword } from '../api'
import type { AdminManagedUser, AdminMqttStatus, AdminUser } from '../types'

const props = defineProps<{
  user: AdminUser
}>()

const emit = defineEmits<{
  logout: []
}>()

const status = ref<AdminMqttStatus | null>(null)
const users = ref<AdminManagedUser[]>([])
const loading = ref(false)
const usersLoading = ref(false)
const error = ref('')
const userError = ref('')
const userMessage = ref('')
const newUsername = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const passwordEdits = ref<Record<number, string>>({})
const passwordSaving = ref<Record<number, boolean>>({})
let timer: number | undefined

function formatUptime(seconds: number): string {
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60
  return `${hours}h ${minutes}m ${secs}s`
}

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
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

async function refreshUsers() {
  usersLoading.value = true
  userError.value = ''
  try {
    const response = await getAdminUsers()
    users.value = response.items
  } catch (err) {
    userError.value = err instanceof Error ? err.message : String(err)
  } finally {
    usersLoading.value = false
  }
}

async function createUser() {
  userError.value = ''
  userMessage.value = ''
  if (!newUsername.value.trim()) {
    userError.value = '用户名不能为空'
    return
  }
  if (!newPassword.value) {
    userError.value = '密码不能为空'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    userError.value = '两次输入的密码不一致'
    return
  }

  usersLoading.value = true
  try {
    await createAdminUser(newUsername.value.trim(), newPassword.value)
    newUsername.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    userMessage.value = '用户已创建'
    await refreshUsers()
  } catch (err) {
    userError.value = err instanceof Error ? err.message : String(err)
  } finally {
    usersLoading.value = false
  }
}

async function updatePassword(user: AdminManagedUser) {
  const password = passwordEdits.value[user.id] || ''
  userError.value = ''
  userMessage.value = ''
  if (!password) {
    userError.value = '新密码不能为空'
    return
  }

  passwordSaving.value = { ...passwordSaving.value, [user.id]: true }
  try {
    await updateAdminUserPassword(user.id, password)
    passwordEdits.value = { ...passwordEdits.value, [user.id]: '' }
    userMessage.value = `${user.username} 的密码已修改`
    await refreshUsers()
  } catch (err) {
    userError.value = err instanceof Error ? err.message : String(err)
  } finally {
    passwordSaving.value = { ...passwordSaving.value, [user.id]: false }
  }
}

async function logout() {
  try {
    await adminLogout()
  } finally {
    emit('logout')
  }
}

onMounted(() => {
  refreshStatus()
  refreshUsers()
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
          <span class="badge">{{ props.user.username }}</span>
          <button @click="refreshStatus" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
          <button @click="logout">退出</button>
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
        <div><span>收到消息</span><strong>{{ status.messages_received }}</strong></div>
        <div><span>发送消息</span><strong>{{ status.messages_sent }}</strong></div>
        <div><span>收到包</span><strong>{{ status.packets_received }}</strong></div>
        <div><span>发送包</span><strong>{{ status.packets_sent }}</strong></div>
      </div>
    </div>

    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Users</p>
          <h2>用户管理</h2>
        </div>
        <button class="admin-button" @click="refreshUsers" :disabled="usersLoading">{{ usersLoading ? '刷新中...' : '刷新用户' }}</button>
      </div>

      <form class="admin-form admin-user-form" @submit.prevent="createUser">
        <label>
          <span>用户名</span>
          <input v-model="newUsername" autocomplete="off" placeholder="new-admin" />
        </label>
        <label>
          <span>密码</span>
          <input v-model="newPassword" type="password" autocomplete="new-password" />
        </label>
        <label>
          <span>确认密码</span>
          <input v-model="confirmPassword" type="password" autocomplete="new-password" />
        </label>
        <button class="admin-button" :disabled="usersLoading" type="submit">新增用户</button>
      </form>
      <p v-if="userError" class="error">{{ userError }}</p>
      <p v-if="userMessage" class="success">{{ userMessage }}</p>

      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>用户名</th>
              <th>角色</th>
              <th>创建时间</th>
              <th>更新时间</th>
              <th>新密码</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in users" :key="user.id">
              <td>{{ user.id }}</td>
              <td>{{ user.username }} <span v-if="user.username === props.user.username" class="badge">当前</span></td>
              <td>{{ user.role }}</td>
              <td>{{ formatTime(user.created_at) }}</td>
              <td>{{ formatTime(user.updated_at) }}</td>
              <td>
                <input
                  v-model="passwordEdits[user.id]"
                  class="admin-table-input"
                  type="password"
                  autocomplete="new-password"
                  placeholder="输入新密码"
                />
              </td>
              <td>
                <button class="admin-button" :disabled="passwordSaving[user.id]" @click="updatePassword(user)">
                  {{ passwordSaving[user.id] ? '保存中...' : '修改密码' }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-if="users.length === 0" class="empty">暂无用户</div>
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
