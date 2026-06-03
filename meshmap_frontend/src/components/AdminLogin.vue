<script setup lang="ts">
import { ref } from 'vue'
import { adminLogin } from '../api'
import type { AdminUser } from '../types'

const emit = defineEmits<{
  login: [user: AdminUser]
}>()

const username = ref('admin')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submitLogin() {
  loading.value = true
  error.value = ''
  try {
    const response = await adminLogin(username.value, password.value)
    emit('login', response.user)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="admin-login panel">
    <div class="panel-header">
      <div>
        <p class="eyebrow">Admin</p>
        <h2>管理员登录</h2>
      </div>
    </div>

    <form class="admin-form" @submit.prevent="submitLogin">
      <label>
        <span>用户名</span>
        <input v-model="username" autocomplete="username" required />
      </label>
      <label>
        <span>密码</span>
        <input v-model="password" type="password" autocomplete="current-password" required />
      </label>
      <p v-if="error" class="error">{{ error }}</p>
      <button :disabled="loading" type="submit">{{ loading ? '登录中...' : '登录' }}</button>
    </form>
  </section>
</template>
