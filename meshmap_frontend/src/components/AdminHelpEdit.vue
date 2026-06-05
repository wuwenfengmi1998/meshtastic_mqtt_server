<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getAdminHelpContent, previewAdminHelpContent, saveAdminHelpContent } from '../api'
import type { HelpContent } from '../types'

const markdown = ref('')
const previewHtml = ref('')
const latest = ref<HelpContent | null>(null)
const loading = ref(false)
const previewing = ref(false)
const saving = ref(false)
const error = ref('')
const message = ref('')
let previewTimer: number | undefined

function formatTime(value: string | null): string {
  return value ? new Date(value).toLocaleString() : '默认内容'
}

async function loadHelpContent() {
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    const response = await getAdminHelpContent()
    latest.value = response.item
    markdown.value = response.item.markdown
    previewHtml.value = response.item.html
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function previewHelpContent() {
  previewing.value = true
  error.value = ''
  try {
    const response = await previewAdminHelpContent(markdown.value)
    previewHtml.value = response.html
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    previewing.value = false
  }
}

function schedulePreview() {
  if (previewTimer !== undefined) {
    window.clearTimeout(previewTimer)
  }
  previewTimer = window.setTimeout(() => {
    previewHelpContent()
  }, 400)
}

async function saveHelpContent() {
  error.value = ''
  message.value = ''
  if (!markdown.value.trim()) {
    error.value = '帮助内容不能为空'
    return
  }
  saving.value = true
  try {
    const response = await saveAdminHelpContent(markdown.value)
    latest.value = response.item
    markdown.value = response.item.markdown
    previewHtml.value = response.item.html
    message.value = `帮助内容已保存为版本 #${response.item.id}`
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}

onMounted(loadHelpContent)
</script>

<template>
  <section class="admin-dashboard">
    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Help</p>
          <h2>帮助编辑</h2>
        </div>
        <div class="admin-actions">
          <button class="admin-button" @click="loadHelpContent" :disabled="loading || saving">{{ loading ? '加载中...' : '重新加载' }}</button>
          <button class="admin-button" @click="saveHelpContent" :disabled="loading || saving">{{ saving ? '保存中...' : '保存新版本' }}</button>
        </div>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="message" class="success">{{ message }}</p>
      <p class="help-version muted">
        当前版本：{{ latest?.id ? `#${latest.id}` : '默认内容' }} · 创建人：{{ latest?.created_by || '-' }} · 时间：{{ formatTime(latest?.created_at ?? null) }}
      </p>

      <div class="help-editor-grid">
        <label class="help-editor-pane">
          <span>Markdown 内容</span>
          <textarea v-model="markdown" @input="schedulePreview" placeholder="请输入帮助内容 Markdown"></textarea>
        </label>
        <div class="help-editor-pane help-preview-pane">
          <div class="help-preview-header">
            <span>预览</span>
            <button class="admin-button" @click="previewHelpContent" :disabled="previewing">{{ previewing ? '预览中...' : '刷新预览' }}</button>
          </div>
          <div class="markdown-body help-preview" v-html="previewHtml"></div>
        </div>
      </div>
    </div>
  </section>
</template>
