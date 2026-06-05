<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { createAdminMapSource, deleteAdminMapSource, getAdminMapSources, setDefaultAdminMapSource, updateAdminMapSource } from '../api'
import type { MapTileSource, MapTileSourcePayload } from '../types'

const items = ref<MapTileSource[]>([])
const loading = ref(false)
const error = ref('')
const message = ref('')
const page = ref(1)
const pageSize = 25

const newSource = ref<MapTileSourcePayload>({
  name: '',
  url_template: 'https://tile.openstreetmap.jp/{z}/{x}/{y}.png',
  attribution: '&copy; OpenStreetMap contributors',
  max_zoom: 19,
  enabled: true,
  is_default: false,
})

const canPrev = () => page.value > 1
const canNext = () => items.value.length === pageSize
const enabledCount = computed(() => items.value.filter((item) => item.enabled).length)
const defaultSource = computed(() => items.value.find((item) => item.is_default) ?? null)

function editableCopy(item: MapTileSource): MapTileSourcePayload {
  return {
    name: item.name,
    url_template: item.url_template,
    attribution: item.attribution,
    max_zoom: item.max_zoom,
    enabled: item.enabled,
    is_default: item.is_default,
  }
}

const drafts = ref<Record<number, MapTileSourcePayload>>({})

function resetNewSource() {
  newSource.value = {
    name: '',
    url_template: '',
    attribution: '&copy; OpenStreetMap contributors',
    max_zoom: 19,
    enabled: true,
    is_default: false,
  }
}

function validatePayload(payload: MapTileSourcePayload): string {
  if (!payload.name.trim()) {
    return '请输入图源名称'
  }
  const url = payload.url_template.trim()
  if (!url) {
    return '请输入图源 URL 模板'
  }
  for (const placeholder of ['{z}', '{x}', '{y}']) {
    if (!url.includes(placeholder)) {
      return `URL 模板必须包含 ${placeholder}`
    }
  }
  if (!Number.isInteger(payload.max_zoom) || payload.max_zoom < 1 || payload.max_zoom > 30) {
    return '最大缩放级别必须是 1 到 30 之间的整数'
  }
  if (payload.is_default && !payload.enabled) {
    return '默认图源必须启用'
  }
  return ''
}

async function refreshItems() {
  loading.value = true
  error.value = ''
  try {
    const response = await getAdminMapSources(pageSize, (page.value - 1) * pageSize)
    items.value = response.items
    drafts.value = Object.fromEntries(response.items.map((item) => [item.id, editableCopy(item)]))
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

function changePage(nextPage: number) {
  page.value = Math.max(1, nextPage)
  refreshItems()
}

async function createSource() {
  const validation = validatePayload(newSource.value)
  if (validation) {
    error.value = validation
    return
  }
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    await createAdminMapSource({ ...newSource.value })
    message.value = '图源已添加'
    resetNewSource()
    page.value = 1
    await refreshItems()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function saveSource(item: MapTileSource) {
  const draft = drafts.value[item.id]
  if (!draft) {
    return
  }
  const validation = validatePayload(draft)
  if (validation) {
    error.value = validation
    return
  }
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    await updateAdminMapSource(item.id, { ...draft })
    message.value = '图源已保存'
    await refreshItems()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function setDefaultSource(item: MapTileSource) {
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    await setDefaultAdminMapSource(item.id)
    message.value = '默认图源已更新'
    await refreshItems()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function removeSource(item: MapTileSource) {
  if (!window.confirm(`确定要删除图源「${item.name}」吗？`)) {
    return
  }
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    await deleteAdminMapSource(item.id)
    message.value = '图源已删除'
    if (items.value.length === 1 && page.value > 1) {
      page.value -= 1
    }
    await refreshItems()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(refreshItems)
</script>

<template>
  <section class="map-source-page">
    <div class="map-source-hero panel">
      <div class="hero-copy">
        <p class="eyebrow">Map source</p>
        <h2>地图图源</h2>
        <p class="muted">集中维护 Leaflet 瓦片图源。URL 模板必须包含 <code>{z}</code>、<code>{x}</code>、<code>{y}</code>。</p>
      </div>
      <div class="hero-stats">
        <div>
          <strong>{{ items.length }}</strong>
          <span>当前图源</span>
        </div>
        <div>
          <strong>{{ enabledCount }}</strong>
          <span>已启用</span>
        </div>
        <div>
          <strong>{{ defaultSource?.name || '-' }}</strong>
          <span>默认图源</span>
        </div>
      </div>
    </div>

    <div class="panel map-source-create-panel">
      <div class="panel-heading compact">
        <div>
          <p class="eyebrow">Create</p>
          <h2>新增图源</h2>
        </div>
        <button class="admin-button ghost" type="button" @click="refreshItems" :disabled="loading">{{ loading ? '刷新中...' : '刷新数据' }}</button>
      </div>

      <form class="map-source-form" @submit.prevent="createSource">
        <label class="field">名称<input v-model="newSource.name" placeholder="OpenStreetMap Japan" /></label>
        <label class="field url-field">URL 模板<input v-model="newSource.url_template" placeholder="https://tile.example.com/{z}/{x}/{y}.png" /></label>
        <label class="field attribution-field">Attribution<input v-model="newSource.attribution" placeholder="&copy; OpenStreetMap contributors" /></label>
        <label class="field zoom-field">最大缩放<input v-model.number="newSource.max_zoom" type="number" min="1" max="30" /></label>
        <label class="switch-card"><input v-model="newSource.enabled" type="checkbox" /> <span>启用</span></label>
        <label class="switch-card"><input v-model="newSource.is_default" type="checkbox" /> <span>设为默认</span></label>
        <div class="form-actions">
          <button class="admin-button" type="submit" :disabled="loading">添加图源</button>
        </div>
      </form>
      <p class="template-tip">示例：<code>https://tile.openstreetmap.jp/{z}/{x}/{y}.png</code></p>
      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="message" class="success">{{ message }}</p>
    </div>

    <div class="panel map-source-list-panel">
      <div class="panel-heading">
        <div>
          <p class="eyebrow">Sources</p>
          <h2>图源列表</h2>
        </div>
        <span class="badge">{{ items.length }} 条</span>
      </div>

      <div v-if="items.length === 0" class="empty-state">暂无地图图源，先在上方添加一个配置。</div>

      <article v-for="item in items" :key="item.id" class="map-source-card" :class="{ default: item.is_default, disabled: !item.enabled }">
        <header class="source-card-title">
          <div>
            <div class="source-title-row">
              <h3>{{ item.name }}</h3>
              <span v-if="item.is_default" class="status-pill ok">默认</span>
              <span v-else-if="item.enabled" class="status-pill">启用</span>
              <span v-else class="status-pill disabled">停用</span>
            </div>
            <p class="source-url">{{ item.url_template }}</p>
          </div>
          <button v-if="!item.is_default" class="admin-button ghost" :disabled="loading || !item.enabled" @click="setDefaultSource(item)">设为默认</button>
        </header>

        <div v-if="drafts[item.id]" class="source-edit-grid">
          <label class="field">名称<input v-model="drafts[item.id].name" /></label>
          <label class="field url-field">URL 模板<input v-model="drafts[item.id].url_template" /></label>
          <label class="field attribution-field">Attribution<input v-model="drafts[item.id].attribution" /></label>
          <label class="field zoom-field">最大缩放<input v-model.number="drafts[item.id].max_zoom" type="number" min="1" max="30" /></label>
          <label class="switch-card"><input v-model="drafts[item.id].enabled" type="checkbox" :disabled="item.is_default" /> <span>启用图源</span></label>
        </div>

        <div class="source-meta">
          <div><span>ID</span><strong>{{ item.id }}</strong></div>
          <div><span>最大缩放</span><strong>{{ item.max_zoom }}</strong></div>
          <div><span>Attribution</span><strong>{{ item.attribution || '-' }}</strong></div>
        </div>

        <div class="actions">
          <button class="admin-button" :disabled="loading" @click="saveSource(item)">保存</button>
          <button class="admin-button danger" :disabled="loading || item.is_default" @click="removeSource(item)">删除</button>
        </div>
      </article>

      <div class="pagination">
        <button :disabled="loading || !canPrev()" @click="changePage(page - 1)">上一页</button>
        <span>第 {{ page }} 页</span>
        <span>每页 {{ pageSize }} 条</span>
        <button :disabled="loading || !canNext()" @click="changePage(page + 1)">下一页</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.map-source-page {
  display: grid;
  gap: 12px;
}

.map-source-page :deep(input) {
  width: 100%;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-sm);
  padding: 9px 11px;
  color: var(--color-heading);
  font: inherit;
  background: var(--color-surface);
  outline: none;
  transition: border-color 0.16s ease, box-shadow 0.16s ease;
}

.map-source-page :deep(input:focus) {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 20%, transparent);
}

.map-source-page :deep(input[type='checkbox']) {
  width: auto;
}

.map-source-hero,
.map-source-create-panel,
.map-source-list-panel {
  padding: 18px;
}

.map-source-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  background: linear-gradient(135deg, var(--color-surface) 0%, var(--color-surface-soft) 100%);
}

.hero-copy {
  min-width: 260px;
}

.hero-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(120px, 1fr));
  gap: 0.75rem;
}

.hero-stats div {
  min-width: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: 12px 16px;
  text-align: center;
  background: color-mix(in srgb, var(--color-surface) 84%, transparent);
}

.hero-stats strong {
  display: block;
  overflow: hidden;
  color: color-mix(in srgb, var(--color-primary) 72%, var(--color-heading));
  font-size: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hero-stats span,
.source-meta span,
.template-tip,
.source-url {
  color: var(--color-muted);
  font-size: 13px;
}

.panel-heading,
.source-card-title,
.source-title-row,
.actions {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  flex-wrap: wrap;
}

.panel-heading,
.source-card-title {
  justify-content: space-between;
}

.panel-heading.compact {
  margin-bottom: 1rem;
}

.map-source-form,
.source-edit-grid {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) minmax(320px, 2fr) minmax(220px, 1.4fr) minmax(100px, 0.5fr) auto auto;
  gap: 0.75rem;
  align-items: end;
}

.field {
  display: grid;
  gap: 6px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 700;
}

.url-field {
  min-width: 320px;
}

.zoom-field {
  min-width: 96px;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
}

.switch-card {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 39px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: 9px 11px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 700;
  background: var(--color-surface-soft);
}

.template-tip {
  margin: 12px 0 0;
}

.map-source-card {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: 1rem;
  margin-top: 1rem;
  background: var(--color-surface);
  box-shadow: inset 4px 0 0 var(--color-primary-soft);
}

.map-source-card.default {
  box-shadow: inset 4px 0 0 var(--color-success);
}

.map-source-card.disabled {
  background: var(--color-surface-soft);
  box-shadow: inset 4px 0 0 var(--color-border-strong);
}

.source-title-row h3 {
  margin: 0;
  color: var(--color-heading);
  font-size: 18px;
}

.source-url {
  max-width: 860px;
  margin: 6px 0 0;
  overflow-wrap: anywhere;
  font-family: var(--font-mono);
}

.status-pill {
  border-radius: 999px;
  padding: 7px 12px;
  color: color-mix(in srgb, var(--color-primary) 72%, var(--color-heading));
  font-size: 13px;
  font-weight: 800;
  background: var(--color-primary-soft);
}

.status-pill.ok {
  color: color-mix(in srgb, var(--color-success) 72%, var(--color-heading));
  background: var(--color-success-soft);
}

.status-pill.disabled {
  color: var(--color-muted);
  background: var(--color-surface-muted);
}

.source-edit-grid {
  grid-template-columns: minmax(180px, 1fr) minmax(320px, 2fr) minmax(220px, 1.4fr) minmax(100px, 0.5fr) auto;
  margin-top: 1rem;
}

.source-meta {
  display: grid;
  grid-template-columns: minmax(70px, 0.4fr) minmax(100px, 0.5fr) minmax(220px, 2fr);
  gap: 0.75rem;
  margin: 1rem 0;
}

.source-meta div {
  min-width: 0;
  border-radius: var(--radius-md);
  padding: 10px 12px;
  background: var(--color-surface-soft);
}

.source-meta strong {
  display: block;
  margin-top: 3px;
  overflow-wrap: anywhere;
  color: var(--color-heading);
}

.actions {
  justify-content: flex-end;
}

.empty-state {
  border: 1px dashed var(--color-border-strong);
  border-radius: var(--radius-md);
  padding: 24px;
  color: var(--color-muted);
  text-align: center;
  background: var(--color-surface-soft);
}

@media (max-width: 1100px) {
  .map-source-hero,
  .panel-heading,
  .source-card-title {
    align-items: stretch;
    flex-direction: column;
  }

  .hero-stats,
  .map-source-form,
  .source-edit-grid,
  .source-meta {
    grid-template-columns: 1fr 1fr;
  }

  .url-field,
  .attribution-field {
    grid-column: 1 / -1;
    min-width: 0;
  }
}

@media (max-width: 700px) {
  .hero-stats,
  .map-source-form,
  .source-edit-grid,
  .source-meta {
    grid-template-columns: 1fr;
  }
}
</style>
