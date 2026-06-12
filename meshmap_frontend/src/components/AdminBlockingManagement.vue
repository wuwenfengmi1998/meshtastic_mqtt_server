<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  createForbiddenWordBlockingRule,
  createIPBlockingRule,
  createNodeBlockingRule,
  deleteForbiddenWordBlockingRule,
  deleteIPBlockingRule,
  deleteNodeBlockingRule,
  getForbiddenWordBlockingRules,
  getIPBlockingRules,
  getNodeBlockingRules,
  updateForbiddenWordBlockingRule,
  updateIPBlockingRule,
  updateNodeBlockingRule,
} from '../api'
import type {
  ForbiddenWordBlockingRule,
  ForbiddenWordBlockingRulePayload,
  IPBlockingRule,
  IPBlockingRulePayload,
  NodeBlockingRule,
  NodeBlockingRulePayload,
} from '../types'

const pageSize = 25

const nodeRules = ref<NodeBlockingRule[]>([])
const nodeLoading = ref(false)
const nodeError = ref('')
const nodeMessage = ref('')
const nodePage = ref(1)
const nodeTotal = ref(0)
const newNodeId = ref('')
const newNodeNum = ref('')
const newNodeReason = ref('')
const newNodeEnabled = ref(true)
const nodeEdits = ref<Record<number, { node_id: string; node_num: string; reason: string; enabled: boolean }>>({})

const ipRules = ref<IPBlockingRule[]>([])
const ipLoading = ref(false)
const ipError = ref('')
const ipMessage = ref('')
const ipPage = ref(1)
const ipTotal = ref(0)
const newIPValue = ref('')
const newIPReason = ref('')
const newIPEnabled = ref(true)
const ipEdits = ref<Record<number, IPBlockingRulePayload>>({})

const wordRules = ref<ForbiddenWordBlockingRule[]>([])
const wordLoading = ref(false)
const wordError = ref('')
const wordMessage = ref('')
const wordPage = ref(1)
const wordTotal = ref(0)
const newWord = ref('')
const newWordMatchType = ref('contains')
const newWordCaseSensitive = ref(false)
const newWordReason = ref('')
const newWordEnabled = ref(true)
const wordEdits = ref<Record<number, ForbiddenWordBlockingRulePayload>>({})

function formatTime(value: string): string {
  return new Date(value).toLocaleString()
}

function canPrev(page: number): boolean {
  return page > 1
}

function canNext(page: number, total: number, count: number): boolean {
  return page * pageSize < total || count === pageSize
}

function parseOptionalInt(value: string): number | null {
  const trimmed = value.trim()
  if (!trimmed) {
    return null
  }
  const parsed = Number.parseInt(trimmed, 10)
  if (!Number.isFinite(parsed) || String(parsed) !== trimmed) {
    throw new Error('节点数字 ID 必须是整数')
  }
  return parsed
}

function nodePayload(nodeId: string, nodeNum: string, reason: string, enabled: boolean): NodeBlockingRulePayload {
  if (!nodeId.trim()) {
    throw new Error('节点 ID 不能为空')
  }
  return {
    node_id: nodeId.trim(),
    node_num: parseOptionalInt(nodeNum),
    reason: reason.trim(),
    enabled,
  }
}

function ipPayload(ipValue: string, reason: string, enabled: boolean): IPBlockingRulePayload {
  if (!ipValue.trim()) {
    throw new Error('IP 或 CIDR 不能为空')
  }
  return { ip_value: ipValue.trim(), reason: reason.trim(), enabled }
}

function wordPayload(word: string, matchType: string, caseSensitive: boolean, reason: string, enabled: boolean): ForbiddenWordBlockingRulePayload {
  if (!word.trim()) {
    throw new Error('违禁词不能为空')
  }
  return {
    word: word.trim(),
    match_type: matchType || 'contains',
    case_sensitive: caseSensitive,
    reason: reason.trim(),
    enabled,
  }
}

function resetNodeEdits(items: NodeBlockingRule[]) {
  nodeEdits.value = Object.fromEntries(
    items.map((item) => [
      item.id,
      {
        node_id: item.node_id,
        node_num: item.node_num == null ? '' : String(item.node_num),
        reason: item.reason,
        enabled: item.enabled,
      },
    ]),
  )
}

function resetIPEdits(items: IPBlockingRule[]) {
  ipEdits.value = Object.fromEntries(items.map((item) => [item.id, { ip_value: item.ip_value, reason: item.reason, enabled: item.enabled }]))
}

function resetWordEdits(items: ForbiddenWordBlockingRule[]) {
  wordEdits.value = Object.fromEntries(
    items.map((item) => [
      item.id,
      {
        word: item.word,
        match_type: item.match_type,
        case_sensitive: item.case_sensitive,
        reason: item.reason,
        enabled: item.enabled,
      },
    ]),
  )
}

async function refreshNodeRules(page = nodePage.value) {
  nodeLoading.value = true
  nodeError.value = ''
  try {
    const safePage = Math.max(1, page)
    const response = await getNodeBlockingRules(pageSize, (safePage - 1) * pageSize)
    nodeRules.value = response.items
    nodeTotal.value = response.total ?? response.offset + response.items.length
    nodePage.value = safePage
    resetNodeEdits(response.items)
  } catch (err) {
    nodeError.value = err instanceof Error ? err.message : String(err)
  } finally {
    nodeLoading.value = false
  }
}

async function createNodeRule() {
  nodeError.value = ''
  nodeMessage.value = ''
  let payload: NodeBlockingRulePayload
  try {
    payload = nodePayload(newNodeId.value, newNodeNum.value, newNodeReason.value, newNodeEnabled.value)
  } catch (err) {
    nodeError.value = err instanceof Error ? err.message : String(err)
    return
  }
  nodeLoading.value = true
  try {
    await createNodeBlockingRule(payload)
    newNodeId.value = ''
    newNodeNum.value = ''
    newNodeReason.value = ''
    newNodeEnabled.value = true
    nodeMessage.value = '节点屏蔽规则已新增'
    await refreshNodeRules(1)
  } catch (err) {
    nodeError.value = err instanceof Error ? err.message : String(err)
  } finally {
    nodeLoading.value = false
  }
}

async function saveNodeRule(rule: NodeBlockingRule) {
  nodeError.value = ''
  nodeMessage.value = ''
  const edit = nodeEdits.value[rule.id]
  if (!edit) {
    return
  }
  let payload: NodeBlockingRulePayload
  try {
    payload = nodePayload(edit.node_id, edit.node_num, edit.reason, edit.enabled)
  } catch (err) {
    nodeError.value = err instanceof Error ? err.message : String(err)
    return
  }
  nodeLoading.value = true
  try {
    await updateNodeBlockingRule(rule.id, payload)
    nodeMessage.value = '节点屏蔽规则已保存'
    await refreshNodeRules()
  } catch (err) {
    nodeError.value = err instanceof Error ? err.message : String(err)
  } finally {
    nodeLoading.value = false
  }
}

async function removeNodeRule(rule: NodeBlockingRule) {
  nodeError.value = ''
  nodeMessage.value = ''
  nodeLoading.value = true
  try {
    await deleteNodeBlockingRule(rule.id)
    nodeMessage.value = '节点屏蔽规则已删除'
    await refreshNodeRules(nodeRules.value.length === 1 ? nodePage.value - 1 : nodePage.value)
  } catch (err) {
    nodeError.value = err instanceof Error ? err.message : String(err)
  } finally {
    nodeLoading.value = false
  }
}

async function refreshIPRules(page = ipPage.value) {
  ipLoading.value = true
  ipError.value = ''
  try {
    const safePage = Math.max(1, page)
    const response = await getIPBlockingRules(pageSize, (safePage - 1) * pageSize)
    ipRules.value = response.items
    ipTotal.value = response.total ?? response.offset + response.items.length
    ipPage.value = safePage
    resetIPEdits(response.items)
  } catch (err) {
    ipError.value = err instanceof Error ? err.message : String(err)
  } finally {
    ipLoading.value = false
  }
}

async function createIPRule() {
  ipError.value = ''
  ipMessage.value = ''
  let payload: IPBlockingRulePayload
  try {
    payload = ipPayload(newIPValue.value, newIPReason.value, newIPEnabled.value)
  } catch (err) {
    ipError.value = err instanceof Error ? err.message : String(err)
    return
  }
  ipLoading.value = true
  try {
    await createIPBlockingRule(payload)
    newIPValue.value = ''
    newIPReason.value = ''
    newIPEnabled.value = true
    ipMessage.value = 'IP 屏蔽规则已新增'
    await refreshIPRules(1)
  } catch (err) {
    ipError.value = err instanceof Error ? err.message : String(err)
  } finally {
    ipLoading.value = false
  }
}

async function saveIPRule(rule: IPBlockingRule) {
  ipError.value = ''
  ipMessage.value = ''
  const edit = ipEdits.value[rule.id]
  if (!edit) {
    return
  }
  let payload: IPBlockingRulePayload
  try {
    payload = ipPayload(edit.ip_value, edit.reason, edit.enabled)
  } catch (err) {
    ipError.value = err instanceof Error ? err.message : String(err)
    return
  }
  ipLoading.value = true
  try {
    await updateIPBlockingRule(rule.id, payload)
    ipMessage.value = 'IP 屏蔽规则已保存'
    await refreshIPRules()
  } catch (err) {
    ipError.value = err instanceof Error ? err.message : String(err)
  } finally {
    ipLoading.value = false
  }
}

async function removeIPRule(rule: IPBlockingRule) {
  ipError.value = ''
  ipMessage.value = ''
  ipLoading.value = true
  try {
    await deleteIPBlockingRule(rule.id)
    ipMessage.value = 'IP 屏蔽规则已删除'
    await refreshIPRules(ipRules.value.length === 1 ? ipPage.value - 1 : ipPage.value)
  } catch (err) {
    ipError.value = err instanceof Error ? err.message : String(err)
  } finally {
    ipLoading.value = false
  }
}

async function refreshWordRules(page = wordPage.value) {
  wordLoading.value = true
  wordError.value = ''
  try {
    const safePage = Math.max(1, page)
    const response = await getForbiddenWordBlockingRules(pageSize, (safePage - 1) * pageSize)
    wordRules.value = response.items
    wordTotal.value = response.total ?? response.offset + response.items.length
    wordPage.value = safePage
    resetWordEdits(response.items)
  } catch (err) {
    wordError.value = err instanceof Error ? err.message : String(err)
  } finally {
    wordLoading.value = false
  }
}

async function createWordRule() {
  wordError.value = ''
  wordMessage.value = ''
  let payload: ForbiddenWordBlockingRulePayload
  try {
    payload = wordPayload(newWord.value, newWordMatchType.value, newWordCaseSensitive.value, newWordReason.value, newWordEnabled.value)
  } catch (err) {
    wordError.value = err instanceof Error ? err.message : String(err)
    return
  }
  wordLoading.value = true
  try {
    await createForbiddenWordBlockingRule(payload)
    newWord.value = ''
    newWordMatchType.value = 'contains'
    newWordCaseSensitive.value = false
    newWordReason.value = ''
    newWordEnabled.value = true
    wordMessage.value = '违禁词屏蔽规则已新增'
    await refreshWordRules(1)
  } catch (err) {
    wordError.value = err instanceof Error ? err.message : String(err)
  } finally {
    wordLoading.value = false
  }
}

async function saveWordRule(rule: ForbiddenWordBlockingRule) {
  wordError.value = ''
  wordMessage.value = ''
  const edit = wordEdits.value[rule.id]
  if (!edit) {
    return
  }
  let payload: ForbiddenWordBlockingRulePayload
  try {
    payload = wordPayload(edit.word, edit.match_type, edit.case_sensitive, edit.reason, edit.enabled)
  } catch (err) {
    wordError.value = err instanceof Error ? err.message : String(err)
    return
  }
  wordLoading.value = true
  try {
    await updateForbiddenWordBlockingRule(rule.id, payload)
    wordMessage.value = '违禁词屏蔽规则已保存'
    await refreshWordRules()
  } catch (err) {
    wordError.value = err instanceof Error ? err.message : String(err)
  } finally {
    wordLoading.value = false
  }
}

async function removeWordRule(rule: ForbiddenWordBlockingRule) {
  wordError.value = ''
  wordMessage.value = ''
  wordLoading.value = true
  try {
    await deleteForbiddenWordBlockingRule(rule.id)
    wordMessage.value = '违禁词屏蔽规则已删除'
    await refreshWordRules(wordRules.value.length === 1 ? wordPage.value - 1 : wordPage.value)
  } catch (err) {
    wordError.value = err instanceof Error ? err.message : String(err)
  } finally {
    wordLoading.value = false
  }
}

onMounted(() => {
  refreshNodeRules()
  refreshIPRules()
  refreshWordRules()
})
</script>

<template>
  <section class="admin-dashboard">
    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Blocking</p>
          <h2>屏蔽管理</h2>
        </div>
      </div>
      <p class="empty">管理节点、IP/CIDR、违禁词三类屏蔽规则。</p>
    </div>

    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Forbidden Words</p>
          <h2>违禁词屏蔽</h2>
        </div>
        <button class="admin-button" @click="refreshWordRules()" :disabled="wordLoading">{{ wordLoading ? '刷新中...' : '刷新' }}</button>
      </div>

      <form class="admin-form admin-user-form" @submit.prevent="createWordRule">
        <label>
          <span>违禁词</span>
          <input v-model="newWord" autocomplete="off" placeholder="spam" />
        </label>
        <label>
          <span>匹配类型</span>
          <select v-model="newWordMatchType" class="admin-table-input">
            <option value="contains">包含</option>
          </select>
        </label>
        <label>
          <span>区分大小写</span>
          <input v-model="newWordCaseSensitive" type="checkbox" />
        </label>
        <label>
          <span>原因</span>
          <input v-model="newWordReason" autocomplete="off" placeholder="policy" />
        </label>
        <label>
          <span>启用</span>
          <input v-model="newWordEnabled" type="checkbox" />
        </label>
        <button class="admin-button" :disabled="wordLoading" type="submit">新增违禁词规则</button>
      </form>
      <p v-if="wordError" class="error">{{ wordError }}</p>
      <p v-if="wordMessage" class="success">{{ wordMessage }}</p>

      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>违禁词</th>
              <th>匹配类型</th>
              <th>区分大小写</th>
              <th>原因</th>
              <th>启用</th>
              <th>创建时间</th>
              <th>更新时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="rule in wordRules" :key="rule.id">
              <td>{{ rule.id }}</td>
              <td><input v-model="wordEdits[rule.id].word" class="admin-table-input" /></td>
              <td>
                <select v-model="wordEdits[rule.id].match_type" class="admin-table-input">
                  <option value="contains">包含</option>
                </select>
              </td>
              <td><input v-model="wordEdits[rule.id].case_sensitive" type="checkbox" /></td>
              <td><input v-model="wordEdits[rule.id].reason" class="admin-table-input" /></td>
              <td><input v-model="wordEdits[rule.id].enabled" type="checkbox" /></td>
              <td>{{ formatTime(rule.created_at) }}</td>
              <td>{{ formatTime(rule.updated_at) }}</td>
              <td>
                <button class="admin-button" :disabled="wordLoading" @click="saveWordRule(rule)">保存</button>
                <button class="admin-button" :disabled="wordLoading" @click="removeWordRule(rule)">删除</button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-if="wordRules.length === 0" class="empty">暂无违禁词屏蔽规则</div>
      </div>
      <div class="pagination">
        <button class="admin-button" :disabled="wordLoading || !canPrev(wordPage)" @click="refreshWordRules(wordPage - 1)">上一页</button>
        <span>第 {{ wordPage }} 页 · 共 {{ wordTotal }} 条</span>
        <button class="admin-button" :disabled="wordLoading || !canNext(wordPage, wordTotal, wordRules.length)" @click="refreshWordRules(wordPage + 1)">下一页</button>
      </div>
    </div>
    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Nodes</p>
          <h2>节点屏蔽</h2>
        </div>
        <button class="admin-button" @click="refreshNodeRules()" :disabled="nodeLoading">{{ nodeLoading ? '刷新中...' : '刷新' }}</button>
      </div>

      <form class="admin-form admin-user-form" @submit.prevent="createNodeRule">
        <label>
          <span>节点 ID</span>
          <input v-model="newNodeId" autocomplete="off" placeholder="!12345678" />
        </label>
        <label>
          <span>节点数字 ID</span>
          <input v-model="newNodeNum" autocomplete="off" placeholder="可选" />
        </label>
        <label>
          <span>原因</span>
          <input v-model="newNodeReason" autocomplete="off" placeholder="spam / abuse" />
        </label>
        <label>
          <span>启用</span>
          <input v-model="newNodeEnabled" type="checkbox" />
        </label>
        <button class="admin-button" :disabled="nodeLoading" type="submit">新增节点规则</button>
      </form>
      <p v-if="nodeError" class="error">{{ nodeError }}</p>
      <p v-if="nodeMessage" class="success">{{ nodeMessage }}</p>

      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>节点 ID</th>
              <th>数字 ID</th>
              <th>原因</th>
              <th>启用</th>
              <th>创建时间</th>
              <th>更新时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="rule in nodeRules" :key="rule.id">
              <td>{{ rule.id }}</td>
              <td><input v-model="nodeEdits[rule.id].node_id" class="admin-table-input" /></td>
              <td><input v-model="nodeEdits[rule.id].node_num" class="admin-table-input" placeholder="可选" /></td>
              <td><input v-model="nodeEdits[rule.id].reason" class="admin-table-input" /></td>
              <td><input v-model="nodeEdits[rule.id].enabled" type="checkbox" /></td>
              <td>{{ formatTime(rule.created_at) }}</td>
              <td>{{ formatTime(rule.updated_at) }}</td>
              <td>
                <button class="admin-button" :disabled="nodeLoading" @click="saveNodeRule(rule)">保存</button>
                <button class="admin-button" :disabled="nodeLoading" @click="removeNodeRule(rule)">删除</button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-if="nodeRules.length === 0" class="empty">暂无节点屏蔽规则</div>
      </div>
      <div class="pagination">
        <button class="admin-button" :disabled="nodeLoading || !canPrev(nodePage)" @click="refreshNodeRules(nodePage - 1)">上一页</button>
        <span>第 {{ nodePage }} 页 · 共 {{ nodeTotal }} 条</span>
        <button class="admin-button" :disabled="nodeLoading || !canNext(nodePage, nodeTotal, nodeRules.length)" @click="refreshNodeRules(nodePage + 1)">下一页</button>
      </div>
    </div>

    <div class="panel admin-status-panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">IP / CIDR</p>
          <h2>IP 屏蔽</h2>
        </div>
        <button class="admin-button" @click="refreshIPRules()" :disabled="ipLoading">{{ ipLoading ? '刷新中...' : '刷新' }}</button>
      </div>

      <form class="admin-form admin-user-form" @submit.prevent="createIPRule">
        <label>
          <span>IP 或 CIDR</span>
          <input v-model="newIPValue" autocomplete="off" placeholder="127.0.0.1 或 192.168.1.0/24" />
        </label>
        <label>
          <span>原因</span>
          <input v-model="newIPReason" autocomplete="off" placeholder="abuse" />
        </label>
        <label>
          <span>启用</span>
          <input v-model="newIPEnabled" type="checkbox" />
        </label>
        <button class="admin-button" :disabled="ipLoading" type="submit">新增 IP 规则</button>
      </form>
      <p v-if="ipError" class="error">{{ ipError }}</p>
      <p v-if="ipMessage" class="success">{{ ipMessage }}</p>

      <div class="node-table-wrap">
        <table class="node-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>IP/CIDR</th>
              <th>原因</th>
              <th>启用</th>
              <th>创建时间</th>
              <th>更新时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="rule in ipRules" :key="rule.id">
              <td>{{ rule.id }}</td>
              <td><input v-model="ipEdits[rule.id].ip_value" class="admin-table-input" /></td>
              <td><input v-model="ipEdits[rule.id].reason" class="admin-table-input" /></td>
              <td><input v-model="ipEdits[rule.id].enabled" type="checkbox" /></td>
              <td>{{ formatTime(rule.created_at) }}</td>
              <td>{{ formatTime(rule.updated_at) }}</td>
              <td>
                <button class="admin-button" :disabled="ipLoading" @click="saveIPRule(rule)">保存</button>
                <button class="admin-button" :disabled="ipLoading" @click="removeIPRule(rule)">删除</button>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-if="ipRules.length === 0" class="empty">暂无 IP 屏蔽规则</div>
      </div>
      <div class="pagination">
        <button class="admin-button" :disabled="ipLoading || !canPrev(ipPage)" @click="refreshIPRules(ipPage - 1)">上一页</button>
        <span>第 {{ ipPage }} 页 · 共 {{ ipTotal }} 条</span>
        <button class="admin-button" :disabled="ipLoading || !canNext(ipPage, ipTotal, ipRules.length)" @click="refreshIPRules(ipPage + 1)">下一页</button>
      </div>
    </div>

  </section>
</template>
