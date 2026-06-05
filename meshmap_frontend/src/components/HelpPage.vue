<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getHelpContent } from '../api'

const loading = ref(false)
const error = ref('')
const html = ref('')

async function loadHelpContent() {
  loading.value = true
  error.value = ''
  try {
    const response = await getHelpContent()
    html.value = response.item.html
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(loadHelpContent)
</script>

<template>
  <section class="help-page">
    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Help</p>
          <h2>如何连接 MQTT</h2>
        </div>
      </div>

      <div class="help-content">
        <p v-if="loading" class="muted">正在加载帮助内容...</p>
        <p v-else-if="error" class="error">{{ error }}</p>
        <div v-else class="markdown-body" v-html="html"></div>
      </div>
    </div>
  </section>
</template>
