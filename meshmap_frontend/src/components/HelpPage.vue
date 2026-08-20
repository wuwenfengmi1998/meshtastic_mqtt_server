<script setup lang="ts">
import { onMounted, ref } from 'vue'
import DOMPurify from 'dompurify'
import { getHelpContent } from '../api'

const loading = ref(false)
const error = ref('')
const html = ref('')

async function loadHelpContent() {
  loading.value = true
  error.value = ''
  try {
    const response = await getHelpContent()
    // 服务端已用 bluemonday 消毒,这里再用 DOMPurify 做客户端纵深防御。
    html.value = DOMPurify.sanitize(response.item.html)
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
