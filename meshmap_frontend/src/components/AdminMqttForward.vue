<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  createMQTTForwarder,
  createMQTTForwardTopic,
  deleteMQTTForwarder,
  deleteMQTTForwardTopic,
  getMQTTForwarders,
  getMQTTForwardStatus,
  getMQTTForwardTopics,
  restartMQTTForwarder,
  updateMQTTForwarder,
  updateMQTTForwardTopic,
} from '../api'
import type {
  MQTTForwarder,
  MQTTForwarderPayload,
  MQTTForwardRuntimeStatus,
  MQTTForwardTopic,
  MQTTForwardTopicPayload,
} from '../types'

const pageSize = 25
const topicPageSize = 100

const forwarders = ref<MQTTForwarder[]>([])
const topics = ref<Record<number, MQTTForwardTopic[]>>({})
const statuses = ref<Record<number, MQTTForwardRuntimeStatus>>({})
const edits = ref<Record<number, ForwarderEdit>>({})
const topicEdits = ref<Record<number, MQTTForwardTopicPayload>>({})
const expanded = ref<Record<number, boolean>>({})
const newTopics = ref<Record<number, MQTTForwardTopicPayload>>({})
const loading = ref(false)
const error = ref('')
const message = ref('')
const page = ref(1)
const total = ref(0)
let statusTimer: number | undefined

type ForwarderEdit = {
  name: string
  enabled: boolean
  source_host: string
  source_port: string
  source_username: string
  source_password: string
  source_password_clear: boolean
  source_client_id: string
  source_tls: boolean
  target_host: string
  target_port: string
  target_username: string
  target_password: string
  target_password_clear: boolean
  target_client_id: string
  target_tls: boolean
}

const newForwarder = ref<ForwarderEdit>({
  name: '',
  enabled: false,
  source_host: 'mqtt.mess.host',
  source_port: '1883',
  source_username: '',
  source_password: '',
  source_password_clear: false,
  source_client_id: '',
  source_tls: false,
  target_host: '127.0.0.1',
  target_port: '1883',
  target_username: '',
  target_password: '',
  target_password_clear: false,
  target_client_id: '',
  target_tls: false,
})

const canPrev = computed(() => page.value > 1)
const canNext = computed(() => page.value * pageSize < total.value || forwarders.value.length === pageSize)

function formatTime(value: string | null): string {
  return value ? new Date(value).toLocaleString() : '-'
}

function defaultTopic(): MQTTForwardTopicPayload {
  return { topic: 'msh/#', enabled: true, direction: 'source_to_target', source_prefix: '', target_prefix: '', qos: 0, retain: false }
}

function resetEdits(items: MQTTForwarder[]) {
  edits.value = Object.fromEntries(items.map((item) => [item.id, forwarderToEdit(item)]))
  for (const item of items) {
    if (!newTopics.value[item.id]) {
      newTopics.value[item.id] = defaultTopic()
    }
  }
}

function forwarderToEdit(item: MQTTForwarder): ForwarderEdit {
  return {
    name: item.name,
    enabled: item.enabled,
    source_host: item.source_host,
    source_port: String(item.source_port),
    source_username: item.source_username,
    source_password: '',
    source_password_clear: false,
    source_client_id: item.source_client_id,
    source_tls: item.source_tls,
    target_host: item.target_host,
    target_port: String(item.target_port),
    target_username: item.target_username,
    target_password: '',
    target_password_clear: false,
    target_client_id: item.target_client_id,
    target_tls: item.target_tls,
  }
}

function resetTopicEdits(forwarderId: number, items: MQTTForwardTopic[]) {
  topics.value = { ...topics.value, [forwarderId]: items }
  topicEdits.value = {
    ...topicEdits.value,
    ...Object.fromEntries(
      items.map((item) => [
        item.id,
        {
          topic: item.topic,
          enabled: item.enabled,
          direction: item.direction,
          source_prefix: item.source_prefix,
          target_prefix: item.target_prefix,
          qos: item.qos,
          retain: item.retain,
        },
      ]),
    ),
  }
}

function parsePort(value: string, label: string): number {
  const parsed = Number.parseInt(value.trim(), 10)
  if (!Number.isInteger(parsed) || parsed <= 0 || parsed > 65535) {
    throw new Error(`${label}必须是 1-65535`)
  }
  return parsed
}

function forwarderPayload(edit: ForwarderEdit, includePasswords: boolean): MQTTForwarderPayload {
  if (!edit.name.trim()) {
    throw new Error('名称不能为空')
  }
  if (!edit.source_host.trim()) {
    throw new Error('源 Host 不能为空')
  }
  if (!edit.target_host.trim()) {
    throw new Error('目标 Host 不能为空')
  }
  const payload: MQTTForwarderPayload = {
    name: edit.name.trim(),
    enabled: edit.enabled,
    source_host: edit.source_host.trim(),
    source_port: parsePort(edit.source_port, '源端口'),
    source_username: edit.source_username.trim(),
    source_client_id: edit.source_client_id.trim(),
    source_tls: edit.source_tls,
    target_host: edit.target_host.trim(),
    target_port: parsePort(edit.target_port, '目标端口'),
    target_username: edit.target_username.trim(),
    target_client_id: edit.target_client_id.trim(),
    target_tls: edit.target_tls,
  }
  if (includePasswords || edit.source_password.trim()) {
    payload.source_password = edit.source_password
  }
  if (edit.source_password_clear) {
    payload.source_password_clear = true
  }
  if (includePasswords || edit.target_password.trim()) {
    payload.target_password = edit.target_password
  }
  if (edit.target_password_clear) {
    payload.target_password_clear = true
  }
  return payload
}

function topicPayload(edit: MQTTForwardTopicPayload): MQTTForwardTopicPayload {
  if (!edit.topic.trim()) {
    throw new Error('TOPIC 不能为空')
  }
  return {
    topic: edit.topic.trim(),
    enabled: edit.enabled,
    direction: edit.direction,
    source_prefix: edit.source_prefix.trim(),
    target_prefix: edit.target_prefix.trim(),
    qos: Number(edit.qos),
    retain: edit.retain,
  }
}

async function refreshForwarders(targetPage = page.value) {
  loading.value = true
  error.value = ''
  try {
    const safePage = Math.max(1, targetPage)
    const response = await getMQTTForwarders(pageSize, (safePage - 1) * pageSize)
    forwarders.value = response.items
    total.value = response.total ?? response.offset + response.items.length
    page.value = safePage
    resetEdits(response.items)
    await refreshStatus()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function refreshStatus() {
  try {
    const response = await getMQTTForwardStatus()
    statuses.value = Object.fromEntries(response.items.map((item) => [item.forwarder_id, item]))
  } catch {
    // Keep the page usable if status polling fails; CRUD calls will surface errors.
  }
}

async function createForwarder() {
  error.value = ''
  message.value = ''
  let payload: MQTTForwarderPayload
  try {
    payload = forwarderPayload(newForwarder.value, true)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    return
  }
  loading.value = true
  try {
    await createMQTTForwarder(payload)
    newForwarder.value.name = ''
    newForwarder.value.source_password = ''
    newForwarder.value.target_password = ''
    message.value = 'MQTT 转发线程已新增'
    await refreshForwarders(1)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function saveForwarder(item: MQTTForwarder) {
  error.value = ''
  message.value = ''
  const edit = edits.value[item.id]
  if (!edit) return
  let payload: MQTTForwarderPayload
  try {
    payload = forwarderPayload(edit, false)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    return
  }
  loading.value = true
  try {
    await updateMQTTForwarder(item.id, payload)
    message.value = 'MQTT 转发线程已保存'
    await refreshForwarders()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function removeForwarder(item: MQTTForwarder) {
  if (!window.confirm(`确定删除 MQTT 转发线程「${item.name}」吗？`)) return
  error.value = ''
  message.value = ''
  loading.value = true
  try {
    await deleteMQTTForwarder(item.id)
    message.value = 'MQTT 转发线程已删除'
    await refreshForwarders(forwarders.value.length === 1 ? page.value - 1 : page.value)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function restartForwarder(item: MQTTForwarder) {
  error.value = ''
  message.value = ''
  try {
    await restartMQTTForwarder(item.id)
    message.value = 'MQTT 转发线程已重启'
    await refreshStatus()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function toggleTopics(item: MQTTForwarder) {
  expanded.value = { ...expanded.value, [item.id]: !expanded.value[item.id] }
  if (expanded.value[item.id] && !topics.value[item.id]) {
    await refreshTopics(item.id)
  }
}

async function refreshTopics(forwarderId: number) {
  const response = await getMQTTForwardTopics(forwarderId, topicPageSize, 0)
  resetTopicEdits(forwarderId, response.items)
}

async function createTopic(forwarderId: number) {
  error.value = ''
  message.value = ''
  let payload: MQTTForwardTopicPayload
  try {
    payload = topicPayload(newTopics.value[forwarderId] ?? defaultTopic())
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    return
  }
  try {
    await createMQTTForwardTopic(forwarderId, payload)
    newTopics.value = { ...newTopics.value, [forwarderId]: defaultTopic() }
    message.value = 'TOPIC 已新增'
    await refreshTopics(forwarderId)
    await refreshStatus()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function saveTopic(topic: MQTTForwardTopic) {
  error.value = ''
  message.value = ''
  const edit = topicEdits.value[topic.id]
  if (!edit) return
  try {
    await updateMQTTForwardTopic(topic.id, topicPayload(edit))
    message.value = 'TOPIC 已保存'
    await refreshTopics(topic.forwarder_id)
    await refreshStatus()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function removeTopic(topic: MQTTForwardTopic) {
  if (!window.confirm(`确定删除 TOPIC「${topic.topic}」吗？`)) return
  error.value = ''
  message.value = ''
  try {
    await deleteMQTTForwardTopic(topic.id)
    message.value = 'TOPIC 已删除'
    await refreshTopics(topic.forwarder_id)
    await refreshStatus()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

function statusText(item: MQTTForwarder): string {
  const status = statuses.value[item.id]
  if (!item.enabled) return '已禁用'
  if (!status) return '未运行'
  return status.source_connected && status.target_connected ? '已连接' : '连接中/异常'
}

onMounted(() => {
  refreshForwarders()
  statusTimer = window.setInterval(refreshStatus, 5000)
})

onBeforeUnmount(() => {
  if (statusTimer !== undefined) {
    window.clearInterval(statusTimer)
  }
})
</script>

<template>
  <section class="mqtt-forward-page">
    <div class="mqtt-hero panel">
      <div>
        <p class="eyebrow">MQTT Forward</p>
        <h2>MQTT 转发管理</h2>
        <p class="muted">统一管理源 Broker、目标 Broker 和每个 TOPIC 的转发方向。保存配置后后端会自动重启对应线程。</p>
      </div>
      <div class="hero-stats">
        <div>
          <strong>{{ total }}</strong>
          <span>线程配置</span>
        </div>
        <div>
          <strong>{{ Object.keys(statuses).length }}</strong>
          <span>运行中</span>
        </div>
      </div>
    </div>

    <div class="panel form-panel">
      <div class="panel-heading compact">
        <div>
          <p class="eyebrow">Create</p>
          <h2>新增转发线程</h2>
        </div>
        <label class="switch-card">
          <input v-model="newForwarder.enabled" type="checkbox" />
          <span>创建后启用</span>
        </label>
      </div>
      <form class="forward-form" @submit.prevent="createForwarder">
        <label class="field span-2">名称<input v-model="newForwarder.name" placeholder="例如：Meshtastic CN" /></label>
        <fieldset class="broker-card source-card">
          <legend>源 Broker</legend>
          <label class="field span-2">Host<input v-model="newForwarder.source_host" /></label>
          <label class="field small">Port<input v-model="newForwarder.source_port" /></label>
          <label class="field">用户名<input v-model="newForwarder.source_username" /></label>
          <label class="field">密码<input v-model="newForwarder.source_password" type="password" /></label>
          <label class="field span-2">Client ID<input v-model="newForwarder.source_client_id" placeholder="留空自动生成" /></label>
          <label class="switch-card"><input v-model="newForwarder.source_tls" type="checkbox" /> <span>TLS</span></label>
        </fieldset>
        <fieldset class="broker-card target-card">
          <legend>目标 Broker</legend>
          <label class="field span-2">Host<input v-model="newForwarder.target_host" /></label>
          <label class="field small">Port<input v-model="newForwarder.target_port" /></label>
          <label class="field">用户名<input v-model="newForwarder.target_username" /></label>
          <label class="field">密码<input v-model="newForwarder.target_password" type="password" /></label>
          <label class="field span-2">Client ID<input v-model="newForwarder.target_client_id" placeholder="留空自动生成" /></label>
          <label class="switch-card"><input v-model="newForwarder.target_tls" type="checkbox" /> <span>TLS</span></label>
        </fieldset>
        <div class="form-actions">
          <button class="admin-button" type="submit" :disabled="loading">新增转发线程</button>
        </div>
      </form>
      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="message" class="success">{{ message }}</p>
    </div>

    <div class="panel list-panel">
      <div class="panel-heading">
        <div>
          <p class="eyebrow">Forwarders</p>
          <h2>转发线程</h2>
        </div>
        <button class="admin-button ghost" @click="refreshForwarders()" :disabled="loading">刷新</button>
      </div>

      <div v-if="!forwarders.length" class="empty-state">暂无 MQTT 转发线程，先在上方创建一个配置。</div>

      <article v-for="item in forwarders" :key="item.id" class="forwarder-card">
        <header class="forwarder-title">
          <div>
            <h3>{{ item.name }}</h3>
            <p class="endpoint-line">{{ item.source_host }}:{{ item.source_port }} → {{ item.target_host }}:{{ item.target_port }}</p>
          </div>
          <span
            class="status-pill"
            :class="{
              ok: item.enabled && statuses[item.id]?.source_connected && statuses[item.id]?.target_connected,
              disabled: !item.enabled,
              warn: item.enabled && (!statuses[item.id]?.source_connected || !statuses[item.id]?.target_connected),
            }"
          >
            {{ statusText(item) }}
          </span>
        </header>

        <div class="runtime-grid">
          <div><span>源连接</span><strong>{{ statuses[item.id]?.source_connected ? '已连接' : '未连接' }}</strong></div>
          <div><span>目标连接</span><strong>{{ statuses[item.id]?.target_connected ? '已连接' : '未连接' }}</strong></div>
          <div><span>已转发</span><strong>{{ statuses[item.id]?.messages_forwarded ?? 0 }}</strong></div>
          <div><span>已丢弃</span><strong>{{ statuses[item.id]?.messages_dropped ?? 0 }}</strong></div>
          <div class="span-2"><span>启动时间</span><strong>{{ formatTime(statuses[item.id]?.started_at ?? null) }}</strong></div>
        </div>
        <p v-if="statuses[item.id]?.last_error" class="inline-error">{{ statuses[item.id]?.last_error }}</p>

        <div v-if="edits[item.id]" class="edit-shell">
          <div class="edit-section main-section">
            <label class="field">名称<input v-model="edits[item.id].name" /></label>
            <label class="switch-card"><input v-model="edits[item.id].enabled" type="checkbox" /> <span>启用线程</span></label>
          </div>
          <div class="edit-section source-card">
            <h4>源 Broker</h4>
            <label class="field span-2">Host<input v-model="edits[item.id].source_host" /></label>
            <label class="field small">Port<input v-model="edits[item.id].source_port" /></label>
            <label class="field">用户名<input v-model="edits[item.id].source_username" /></label>
            <label class="field">密码<input v-model="edits[item.id].source_password" type="password" :placeholder="item.source_password_set ? '留空保持原密码' : ''" /></label>
            <label class="field span-2">Client ID<input v-model="edits[item.id].source_client_id" /></label>
            <label class="switch-card"><input v-model="edits[item.id].source_password_clear" type="checkbox" /> <span>清空源密码</span></label>
            <label class="switch-card"><input v-model="edits[item.id].source_tls" type="checkbox" /> <span>源 TLS</span></label>
          </div>
          <div class="edit-section target-card">
            <h4>目标 Broker</h4>
            <label class="field span-2">Host<input v-model="edits[item.id].target_host" /></label>
            <label class="field small">Port<input v-model="edits[item.id].target_port" /></label>
            <label class="field">用户名<input v-model="edits[item.id].target_username" /></label>
            <label class="field">密码<input v-model="edits[item.id].target_password" type="password" :placeholder="item.target_password_set ? '留空保持原密码' : ''" /></label>
            <label class="field span-2">Client ID<input v-model="edits[item.id].target_client_id" /></label>
            <label class="switch-card"><input v-model="edits[item.id].target_password_clear" type="checkbox" /> <span>清空目标密码</span></label>
            <label class="switch-card"><input v-model="edits[item.id].target_tls" type="checkbox" /> <span>目标 TLS</span></label>
          </div>
        </div>

        <div class="actions">
          <button class="admin-button" @click="saveForwarder(item)" :disabled="loading">保存并重启</button>
          <button class="admin-button ghost" @click="restartForwarder(item)">仅重启</button>
          <button class="admin-button danger" @click="removeForwarder(item)" :disabled="loading">删除</button>
          <button class="admin-button secondary" @click="toggleTopics(item)">{{ expanded[item.id] ? '收起 TOPICS' : '管理 TOPICS' }}</button>
        </div>

        <div v-if="expanded[item.id]" class="topics-box">
          <div class="topics-heading">
            <div>
              <p class="eyebrow">Topics</p>
              <h4>订阅规则</h4>
            </div>
            <span class="badge">{{ topics[item.id]?.length ?? 0 }} 条</span>
          </div>
          <form v-if="newTopics[item.id]" class="topic-row new-topic" @submit.prevent="createTopic(item.id)">
            <input v-model="newTopics[item.id].topic" placeholder="msh/#" />
            <label class="mini-check"><input v-model="newTopics[item.id].enabled" type="checkbox" /> 启用</label>
            <select v-model="newTopics[item.id].direction">
              <option value="source_to_target">单向：源 → 目标</option>
              <option value="bidirectional">双向</option>
            </select>
            <input v-model="newTopics[item.id].source_prefix" placeholder="源前缀" />
            <input v-model="newTopics[item.id].target_prefix" placeholder="目标前缀" />
            <select v-model.number="newTopics[item.id].qos">
              <option :value="0">QoS 0</option>
              <option :value="1">QoS 1</option>
              <option :value="2">QoS 2</option>
            </select>
            <label class="mini-check"><input v-model="newTopics[item.id].retain" type="checkbox" /> Retain</label>
            <button class="admin-button" type="submit">新增</button>
          </form>
          <div v-for="topic in topics[item.id] ?? []" :key="topic.id" class="topic-row">
            <input v-model="topicEdits[topic.id].topic" />
            <label class="mini-check"><input v-model="topicEdits[topic.id].enabled" type="checkbox" /> 启用</label>
            <select v-model="topicEdits[topic.id].direction">
              <option value="source_to_target">单向：源 → 目标</option>
              <option value="bidirectional">双向</option>
            </select>
            <input v-model="topicEdits[topic.id].source_prefix" placeholder="源前缀" />
            <input v-model="topicEdits[topic.id].target_prefix" placeholder="目标前缀" />
            <select v-model.number="topicEdits[topic.id].qos">
              <option :value="0">QoS 0</option>
              <option :value="1">QoS 1</option>
              <option :value="2">QoS 2</option>
            </select>
            <label class="mini-check"><input v-model="topicEdits[topic.id].retain" type="checkbox" /> Retain</label>
            <button class="admin-button ghost" @click="saveTopic(topic)">保存</button>
            <button class="admin-button danger" @click="removeTopic(topic)">删除</button>
          </div>
        </div>
      </article>

      <div class="pagination">
        <button class="admin-button ghost" :disabled="!canPrev || loading" @click="refreshForwarders(page - 1)">上一页</button>
        <span>第 {{ page }} 页 · 共 {{ total }} 条</span>
        <button class="admin-button ghost" :disabled="!canNext || loading" @click="refreshForwarders(page + 1)">下一页</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.mqtt-forward-page {
  width: min(1440px, 100%);
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.mqtt-forward-page :deep(input),
.mqtt-forward-page :deep(select) {
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

.mqtt-forward-page :deep(input:focus),
.mqtt-forward-page :deep(select:focus) {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 20%, transparent);
}

.mqtt-hero,
.form-panel,
.list-panel {
  padding: 18px;
}

.mqtt-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  background: linear-gradient(135deg, var(--color-surface) 0%, var(--color-surface-soft) 100%);
}

.mqtt-hero h2 {
  font-size: 24px;
}

.hero-stats {
  display: grid;
  grid-template-columns: repeat(2, minmax(110px, 1fr));
  gap: 0.75rem;
}

.hero-stats div {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: 12px 16px;
  text-align: center;
  background: color-mix(in srgb, var(--color-surface) 84%, transparent);
}

.hero-stats strong {
  display: block;
  color: color-mix(in srgb, var(--color-primary) 72%, var(--color-heading));
  font-size: 24px;
}

.hero-stats span,
.endpoint-line,
.runtime-grid span {
  color: var(--color-muted);
  font-size: 13px;
}

.panel-heading,
.forwarder-title,
.actions,
.pagination,
.topics-heading {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  flex-wrap: wrap;
}

.panel-heading,
.forwarder-title,
.topics-heading {
  justify-content: space-between;
}

.panel-heading.compact {
  margin-bottom: 1rem;
}

.forward-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1rem;
}

.field {
  display: grid;
  gap: 6px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 700;
}

.span-2 {
  grid-column: span 2;
}

.broker-card,
.edit-section,
.forwarder-card,
.topics-box {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
}

.broker-card {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.75rem;
  margin: 0;
  padding: 1rem;
}

.broker-card legend {
  padding: 0 8px;
  color: var(--color-text);
  font-weight: 800;
}

.source-card {
  background: linear-gradient(180deg, var(--color-surface-soft) 0%, var(--color-surface) 100%);
}

.target-card {
  background: linear-gradient(180deg, var(--color-success-soft) 0%, var(--color-surface) 100%);
}

.form-actions {
  grid-column: 1 / -1;
  display: flex;
  justify-content: flex-end;
}

.switch-card,
.mini-check {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: 9px 11px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 700;
  background: var(--color-surface-soft);
}

.switch-card input,
.mini-check input {
  width: auto;
}

.forwarder-card {
  padding: 1rem;
  margin-top: 1rem;
  box-shadow: inset 4px 0 0 var(--color-primary-soft);
}

.forwarder-title h3 {
  color: var(--color-heading);
  font-size: 18px;
}

.status-pill {
  border-radius: 999px;
  padding: 7px 12px;
  color: color-mix(in srgb, var(--color-warning) 72%, var(--color-heading));
  background: var(--color-warning-soft);
}

.status-pill.ok {
  color: color-mix(in srgb, var(--color-success) 72%, var(--color-heading));
  background: var(--color-success-soft);
}

.status-pill.warn {
  color: color-mix(in srgb, var(--color-warning) 72%, var(--color-heading));
  background: var(--color-warning-soft);
}

.status-pill.disabled {
  color: var(--color-muted);
  background: var(--color-surface-muted);
}

.runtime-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 0.75rem;
  margin: 1rem 0;
}

.runtime-grid div {
  border-radius: var(--radius-md);
  padding: 10px 12px;
  background: var(--color-surface-soft);
}

.runtime-grid strong {
  display: block;
  margin-top: 3px;
  color: var(--color-heading);
}

.inline-error {
  border: 1px solid color-mix(in srgb, var(--color-danger) 36%, white);
  border-radius: var(--radius-md);
  padding: 10px 12px;
  color: color-mix(in srgb, var(--color-danger) 74%, var(--color-heading));
  background: var(--color-danger-soft);
  word-break: break-word;
}

.edit-shell {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
  margin: 1rem 0;
}

.edit-section {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.75rem;
  padding: 1rem;
}

.edit-section h4 {
  grid-column: 1 / -1;
  margin: 0;
}

.main-section {
  grid-column: 1 / -1;
  grid-template-columns: minmax(240px, 1fr) auto;
  align-items: end;
}

.actions {
  justify-content: flex-end;
  margin-top: 0.75rem;
}

.topics-box {
  margin-top: 1rem;
  padding: 1rem;
  background: var(--color-surface-soft);
}

.topic-row {
  display: grid;
  grid-template-columns: minmax(180px, 1.6fr) minmax(90px, 0.7fr) minmax(150px, 1fr) repeat(2, minmax(120px, 1fr)) minmax(90px, 0.7fr) minmax(90px, 0.7fr) auto auto;
  gap: 0.5rem;
  align-items: center;
  border-top: 1px solid var(--color-border);
  padding-top: 0.75rem;
  margin-top: 0.75rem;
}

.topic-row.new-topic {
  border: 1px dashed color-mix(in srgb, var(--color-primary) 54%, white);
  border-radius: var(--radius-md);
  padding: 0.75rem;
  background: var(--color-primary-soft);
}

.empty-state {
  border: 1px dashed var(--color-border-strong);
  border-radius: var(--radius-md);
  padding: 24px;
  color: var(--color-muted);
  text-align: center;
  background: var(--color-surface-soft);
}

.pagination {
  justify-content: center;
  margin-top: 1rem;
}

@media (max-width: 1100px) {
  .forward-form,
  .edit-shell {
    grid-template-columns: 1fr;
  }

  .broker-card,
  .edit-section,
  .topic-row {
    grid-template-columns: 1fr 1fr;
  }

  .span-2,
  .main-section {
    grid-column: auto;
  }
}

@media (max-width: 700px) {
  .mqtt-hero,
  .panel-heading,
  .forwarder-title {
    align-items: stretch;
    flex-direction: column;
  }

  .hero-stats,
  .broker-card,
  .edit-section,
  .topic-row,
  .main-section {
    grid-template-columns: 1fr;
  }

  .span-2 {
    grid-column: auto;
  }
}
</style>
