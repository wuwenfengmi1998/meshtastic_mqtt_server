<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { createAdminSignRecord, deleteAdminSignRecord, getAdminSignRecords, updateAdminSignRecord } from '../api'
import type { SignRecord, SignRecordPayload } from '../types'

const pageSize = 25
const records = ref<SignRecord[]>([])
const loading = ref(false)
const error = ref('')
const message = ref('')
const page = ref(1)
const total = ref(0)
const newNodeID = ref('')
const newLongName = ref('')
const newShortName = ref('')
const newSignText = ref('')
const newSignTime = ref(toDateTimeLocal(new Date().toISOString()))
const edits = ref<Record<number, SignRecordPayload>>({})

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

function toDateTimeLocal(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return ''
  }
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

function toRFC3339(value: string): string | undefined {
  if (!value) {
    return undefined
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    throw new Error('签到时间格式无效')
  }
  return date.toISOString()
}

function canPrev(): boolean {
  return page.value > 1
}

function canNext(): boolean {
  return page.value * pageSize < total.value || records.value.length === pageSize
}

function signPayload(nodeID: string, longName: string, shortName: string, signText: string, signTime: string): SignRecordPayload {
  if (!nodeID.trim()) {
    throw new Error('节点 ID 不能为空')
  }
  if (!signText.trim()) {
    throw new Error('签到文本不能为空')
  }
  return {
    node_id: nodeID.trim(),
    long_name: longName.trim(),
    short_name: shortName.trim(),
    sign_text: signText.trim(),
    sign_time: toRFC3339(signTime),
  }
}

async function refreshSigns(nextPage = page.value) {
  loading.value = true
  error.value = ''
  try {
    const safePage = Math.max(1, nextPage)
    const response = await getAdminSignRecords(pageSize, (safePage - 1) * pageSize)
    records.value = response.items
    total.value = response.total ?? response.offset + response.items.length
    page.value = safePage
    edits.value = {}
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function createSign() {
  error.value = ''
  message.value = ''
  let payload: SignRecordPayload
  try {
    payload = signPayload(newNodeID.value, newLongName.value, newShortName.value, newSignText.value, newSignTime.value)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    return
  }
  loading.value = true
  try {
    await createAdminSignRecord(payload)
    newNodeID.value = ''
    newLongName.value = ''
    newShortName.value = ''
    newSignText.value = ''
    newSignTime.value = toDateTimeLocal(new Date().toISOString())
    message.value = '签到记录已新增'
    await refreshSigns(1)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

function startEdit(record: SignRecord) {
  edits.value = {
    ...edits.value,
    [record.id]: {
      node_id: record.node_id,
      long_name: record.long_name || '',
      short_name: record.short_name || '',
      sign_text: record.sign_text,
      sign_time: toDateTimeLocal(record.sign_time),
    },
  }
}

function cancelEdit(id: number) {
  const next = { ...edits.value }
  delete next[id]
  edits.value = next
}

async function saveSign(record: SignRecord) {
  const edit = edits.value[record.id]
  if (!edit) {
    return
  }
  error.value = ''
  message.value = ''
  let payload: SignRecordPayload
  try {
    payload = signPayload(edit.node_id, edit.long_name, edit.short_name, edit.sign_text, edit.sign_time || '')
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
    return
  }
  loading.value = true
  try {
    await updateAdminSignRecord(record.id, payload)
    message.value = '签到记录已保存'
    await refreshSigns()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function removeSign(record: SignRecord) {
  if (!window.confirm(`确定要删除节点 ${record.node_id} 的签到记录吗？`)) {
    return
  }
  error.value = ''
  message.value = ''
  loading.value = true
  try {
    await deleteAdminSignRecord(record.id)
    message.value = '签到记录已删除'
    await refreshSigns(records.value.length === 1 ? page.value - 1 : page.value)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(() => refreshSigns())
</script>

<template>
  <section class="admin-dashboard">
    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Sign</p>
          <h2>签到管理</h2>
        </div>
        <button class="admin-button" :disabled="loading" @click="refreshSigns()">{{ loading ? '刷新中...' : '刷新签到' }}</button>
      </div>

      <form class="admin-form" @submit.prevent="createSign">
        <label>
          <span>节点 ID</span>
          <input v-model="newNodeID" autocomplete="off" placeholder="!1234abcd" />
        </label>
        <label>
          <span>Long Name</span>
          <input v-model="newLongName" autocomplete="off" placeholder="Long Name" />
        </label>
        <label>
          <span>Short Name</span>
          <input v-model="newShortName" autocomplete="off" placeholder="Short" />
        </label>
        <label>
          <span>签到时间</span>
          <input v-model="newSignTime" type="datetime-local" />
        </label>
        <label class="admin-form-wide">
          <span>签到文本</span>
          <input v-model="newSignText" autocomplete="off" placeholder="签到文本" />
        </label>
        <button class="admin-button" :disabled="loading" type="submit">新增签到</button>
      </form>
      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="message" class="success">{{ message }}</p>

      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>节点 ID</th>
              <th>Long Name</th>
              <th>Short Name</th>
              <th>签到文本</th>
              <th>签到时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="record in records" :key="record.id">
              <template v-if="edits[record.id]">
                <td>{{ record.id }}</td>
                <td><input v-model="edits[record.id].node_id" class="admin-table-input" /></td>
                <td><input v-model="edits[record.id].long_name" class="admin-table-input" /></td>
                <td><input v-model="edits[record.id].short_name" class="admin-table-input" /></td>
                <td><input v-model="edits[record.id].sign_text" class="admin-table-input" /></td>
                <td><input v-model="edits[record.id].sign_time" class="admin-table-input" type="datetime-local" /></td>
                <td>
                  <button class="admin-button" :disabled="loading" @click="saveSign(record)">保存</button>
                  <button class="admin-button" :disabled="loading" @click="cancelEdit(record.id)">取消</button>
                </td>
              </template>
              <template v-else>
                <td>{{ record.id }}</td>
                <td>{{ record.node_id }}</td>
                <td>{{ record.long_name || '-' }}</td>
                <td>{{ record.short_name || '-' }}</td>
                <td>{{ record.sign_text }}</td>
                <td>{{ formatTime(record.sign_time) }}</td>
                <td>
                  <button class="admin-button" :disabled="loading" @click="startEdit(record)">编辑</button>
                  <button class="admin-button danger" :disabled="loading" @click="removeSign(record)">删除</button>
                </td>
              </template>
            </tr>
          </tbody>
        </table>
        <div v-if="records.length === 0" class="empty">{{ loading ? '加载中...' : '暂无签到记录' }}</div>
      </div>

      <div class="pagination">
        <button class="admin-button" :disabled="!canPrev() || loading" @click="refreshSigns(page - 1)">上一页</button>
        <span>第 {{ page }} 页 / 共 {{ total }} 条</span>
        <button class="admin-button" :disabled="!canNext() || loading" @click="refreshSigns(page + 1)">下一页</button>
      </div>
    </div>
  </section>
</template>
