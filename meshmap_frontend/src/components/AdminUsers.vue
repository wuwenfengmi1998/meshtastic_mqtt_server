<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { createAdminUser, getAdminUsers, updateAdminUserPassword } from '../api'
import type { AdminManagedUser, AdminUser } from '../types'

const props = defineProps<{
  user: AdminUser
}>()

const users = ref<AdminManagedUser[]>([])
const usersLoading = ref(false)
const userError = ref('')
const userMessage = ref('')
const newUsername = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const passwordEdits = ref<Record<number, string>>({})
const passwordSaving = ref<Record<number, boolean>>({})

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
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

onMounted(refreshUsers)
</script>

<template>
  <section class="admin-dashboard">
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
            <tr v-for="managedUser in users" :key="managedUser.id">
              <td>{{ managedUser.id }}</td>
              <td>{{ managedUser.username }} <span v-if="managedUser.username === props.user.username" class="badge">当前</span></td>
              <td>{{ managedUser.role }}</td>
              <td>{{ formatTime(managedUser.created_at) }}</td>
              <td>{{ formatTime(managedUser.updated_at) }}</td>
              <td>
                <input
                  v-model="passwordEdits[managedUser.id]"
                  class="admin-table-input"
                  type="password"
                  autocomplete="new-password"
                  placeholder="输入新密码"
                />
              </td>
              <td>
                <button class="admin-button" :disabled="passwordSaving[managedUser.id]" @click="updatePassword(managedUser)">
                  {{ passwordSaving[managedUser.id] ? '保存中...' : '修改密码' }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-if="users.length === 0" class="empty">暂无用户</div>
      </div>
    </div>
  </section>
</template>
