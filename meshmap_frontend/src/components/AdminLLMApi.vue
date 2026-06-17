<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  createLLMProvider,
  deleteLLMProvider,
  getLLMProviders,
  getLLMToolRouter,
  getLLMPrimaryConfig,
  updateLLMProvider,
  updateLLMToolRouter,
  updateLLMPrimaryConfig,
} from '../api'
import type { LLMPlatformRouter, LLMProvider, LLMPrimaryConfig } from '../types'

const loading = ref(false)
const error = ref('')
const success = ref('')

// LLM Provider 相关
const providers = ref<LLMProvider[]>([])
const editingProvider = ref<LLMProvider | null>(null)
const showProviderForm = ref(false)
const isCreatingProvider = ref(false)

const providerForm = ref({
  name: '',
  active: true,
  api_key: '',
  base_url: 'https://ark.cn-beijing.volces.com/api/v3',
  model: '',
  timeout: 120,
  context_window_tokens: 262144,
})

// Tool Router 相关
const toolRouter = ref<LLMPlatformRouter | null>(null)
const editingToolRouter = ref(false)

const toolRouterForm = ref({
  enabled: true,
  openai_name: '',
  timeout: 30,
  max_tokens: 512,
  system_prompt: '',
})

// Primary AI Config 相关 - 主 AI 回复配置
const primaryConfig = ref<LLMPrimaryConfig | null>(null)
const editingPrimaryConfig = ref(false)

const primaryConfigForm = ref({
  enabled: false,
  provider_name: '',
  timeout: 120,
  max_tokens: 1024,
  system_prompt: '',
  enable_tool: false,
})

const activeProviders = computed(() => providers.value.filter((p) => p.active))

function clearSuccess() {
  setTimeout(() => {
    success.value = ''
  }, 3000)
}

async function loadProviders() {
  loading.value = true
  error.value = ''
  try {
    const response = await getLLMProviders(true)
    providers.value = response.items
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function loadToolRouter() {
  try {
    const response = await getLLMToolRouter()
    toolRouter.value = response.item
  } catch (err) {
    // 如果不存在，使用默认值
    console.warn('Tool router config not found, using defaults')
  }
}

async function loadPrimaryConfig() {
  try {
    const response = await getLLMPrimaryConfig()
    primaryConfig.value = response.item
  } catch (err) {
    // 如果不存在，使用默认值
    console.warn('Primary AI config not found, using defaults')
  }
}

function openCreateProvider() {
  isCreatingProvider.value = true
  providerForm.value = {
    name: '',
    active: true,
    api_key: '',
    base_url: 'https://ark.cn-beijing.volces.com/api/v3',
    model: '',
    timeout: 120,
    context_window_tokens: 262144,
  }
  showProviderForm.value = true
  error.value = ''
}

function openEditProvider(provider: LLMProvider) {
  isCreatingProvider.value = false
  providerForm.value = {
    name: provider.name,
    active: provider.active,
    api_key: provider.api_key || '',
    base_url: provider.base_url,
    model: provider.model,
    timeout: provider.timeout,
    context_window_tokens: provider.context_window_tokens,
  }
  editingProvider.value = provider
  showProviderForm.value = true
  error.value = ''
}

function closeProviderForm() {
  showProviderForm.value = false
  editingProvider.value = null
}

async function saveProvider() {
  if (!providerForm.value.name.trim()) {
    error.value = '请输入配置名称'
    return
  }
  if (!providerForm.value.base_url.trim()) {
    error.value = '请输入 API 地址'
    return
  }

  try {
    if (isCreatingProvider.value) {
      await createLLMProvider(providerForm.value)
      success.value = '创建成功'
    } else if (editingProvider.value) {
      await updateLLMProvider(editingProvider.value.name, providerForm.value)
      success.value = '更新成功'
    }
    clearSuccess()
    closeProviderForm()
    await loadProviders()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function confirmDeleteProvider(name: string) {
  if (!confirm(`确定要删除配置 "${name}" 吗？此操作不可撤销。`)) {
    return
  }
  try {
    await deleteLLMProvider(name)
    success.value = '删除成功'
    clearSuccess()
    await loadProviders()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

function openEditToolRouter() {
  if (toolRouter.value) {
    toolRouterForm.value = {
      enabled: toolRouter.value.enabled,
      openai_name: toolRouter.value.openai_name,
      timeout: toolRouter.value.timeout,
      max_tokens: toolRouter.value.max_tokens,
      system_prompt: toolRouter.value.system_prompt,
    }
  }
  editingToolRouter.value = true
}

function closeToolRouterForm() {
  editingToolRouter.value = false
}

async function saveToolRouter() {
  try {
    await updateLLMToolRouter(toolRouterForm.value)
    success.value = '更新成功'
    clearSuccess()
    closeToolRouterForm()
    await loadToolRouter()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

function openEditPrimaryConfig() {
  if (primaryConfig.value) {
    primaryConfigForm.value = {
      enabled: primaryConfig.value.enabled,
      provider_name: primaryConfig.value.provider_name,
      timeout: primaryConfig.value.timeout,
      max_tokens: primaryConfig.value.max_tokens,
      system_prompt: primaryConfig.value.system_prompt,
      enable_tool: primaryConfig.value.enable_tool,
    }
  }
  editingPrimaryConfig.value = true
}

function closePrimaryConfigForm() {
  editingPrimaryConfig.value = false
}

async function savePrimaryConfig() {
  try {
    await updateLLMPrimaryConfig(primaryConfigForm.value)
    success.value = '更新成功'
    clearSuccess()
    closePrimaryConfigForm()
    await loadPrimaryConfig()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(() => {
  loadProviders()
  loadToolRouter()
  loadPrimaryConfig()
})
</script>

<template>
  <div class="admin-llm-api">
    <h2>LLM API 配置管理</h2>

    <p v-if="error" class="error">{{ error }}</p>
    <p v-if="success" class="success">{{ success }}</p>

    <!-- LLM Provider 列表 -->
    <div class="admin-section">
      <div class="section-header">
        <div>
          <h3>AI 提供商配置</h3>
          <p class="section-desc">管理多个 LLM API 提供商配置，支持不同模型和服务。</p>
        </div>
        <button class="admin-button" @click="openCreateProvider">+ 添加配置</button>
      </div>

      <div v-if="loading" class="admin-loading">加载中...</div>

      <div v-else class="provider-grid">
        <div v-for="provider in providers" :key="provider.name" class="provider-card">
          <div class="provider-header">
            <div class="provider-name">
              <span class="status-badge" :class="{ active: provider.active, inactive: !provider.active }">
                {{ provider.active ? '启用' : '停用' }}
              </span>
              <strong>{{ provider.name }}</strong>
            </div>
            <div class="provider-actions">
              <button class="admin-button admin-button-small" @click="openEditProvider(provider)">编辑</button>
              <button class="admin-button admin-button-small admin-button-danger" @click="confirmDeleteProvider(provider.name)">删除</button>
            </div>
          </div>
          <div class="provider-details">
            <div class="detail-row">
              <span class="detail-label">API 地址</span>
              <span class="detail-value">{{ provider.base_url }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">模型</span>
              <span class="detail-value">{{ provider.model || '-' }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">超时</span>
              <span class="detail-value">{{ provider.timeout }} 秒</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">上下文窗口</span>
              <span class="detail-value">{{ provider.context_window_tokens.toLocaleString() }} tokens</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">API Key</span>
              <span class="detail-value masked">{{ provider.api_key ? '••••••••••' : '-' }}</span>
            </div>
          </div>
        </div>

        <div v-if="providers.length === 0" class="empty-state">
          <p>暂无配置，点击上方按钮添加第一个 AI 提供商配置。</p>
        </div>
      </div>
    </div>

    <!-- Tool Router 配置 -->
    <div class="admin-section">
      <div class="section-header">
        <div>
          <h3>工具路由配置</h3>
          <p class="section-desc">配置 LLM 工具调用的路由设置，用于实现函数调用功能。</p>
        </div>
        <button v-if="!editingToolRouter" class="admin-button" @click="openEditToolRouter">编辑配置</button>
      </div>

      <div v-if="editingToolRouter" class="tool-router-form">
        <div class="form-group">
          <label>
            <input type="checkbox" v-model="toolRouterForm.enabled" />
            启用工具路由
          </label>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label>使用的 AI 配置</label>
            <select v-model="toolRouterForm.openai_name" class="form-input">
              <option value="">请选择</option>
              <option v-for="p in activeProviders" :key="p.name" :value="p.name">{{ p.name }}</option>
            </select>
            <p class="form-hint">选择用于工具调用的 AI 提供商配置</p>
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label>超时时间（秒）</label>
            <input type="number" v-model.number="toolRouterForm.timeout" class="form-input" min="1" />
          </div>
          <div class="form-group">
            <label>最大 Token 数</label>
            <input type="number" v-model.number="toolRouterForm.max_tokens" class="form-input" min="1" />
          </div>
        </div>

        <div class="form-group">
          <label>系统提示词</label>
          <textarea v-model="toolRouterForm.system_prompt" class="form-textarea" rows="6"></textarea>
          <p class="form-hint">用于指导模型如何使用工具的系统提示词</p>
        </div>

        <div class="form-actions">
          <button class="admin-button admin-button-secondary" @click="closeToolRouterForm">取消</button>
          <button class="admin-button" @click="saveToolRouter">保存</button>
        </div>
      </div>

      <div v-else-if="toolRouter" class="tool-router-display">
        <div class="router-status">
          <span class="status-badge" :class="{ active: toolRouter.enabled, inactive: !toolRouter.enabled }">
            {{ toolRouter.enabled ? '已启用' : '已停用' }}
          </span>
        </div>
        <div class="router-details">
          <div class="detail-row">
            <span class="detail-label">使用的 AI 配置</span>
            <span class="detail-value">{{ toolRouter.openai_name || '未设置' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">超时时间</span>
            <span class="detail-value">{{ toolRouter.timeout }} 秒</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">最大 Token 数</span>
            <span class="detail-value">{{ toolRouter.max_tokens }}</span>
          </div>
          <div class="detail-row full-width">
            <span class="detail-label">系统提示词</span>
            <pre class="detail-value system-prompt">{{ toolRouter.system_prompt }}</pre>
          </div>
        </div>
      </div>

      <div v-else class="empty-state">
        <p>暂无工具路由配置，点击上方按钮进行配置。</p>
      </div>
    </div>

    <!-- Primary AI Config 配置 - 主 AI 回复配置 -->
    <div class="admin-section">
      <div class="section-header">
        <div>
          <h3>主 AI 回复配置</h3>
          <p class="section-desc">配置机器人自动回复消息的核心 AI 设置。</p>
        </div>
        <button v-if="!editingPrimaryConfig" class="admin-button" @click="openEditPrimaryConfig">编辑配置</button>
      </div>

      <div v-if="editingPrimaryConfig" class="tool-router-form">
        <div class="form-group">
          <label>
            <input type="checkbox" v-model="primaryConfigForm.enabled" />
            启用 AI 自动回复
          </label>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label>使用的 AI 配置</label>
            <select v-model="primaryConfigForm.provider_name" class="form-input">
              <option value="">请选择</option>
              <option v-for="p in activeProviders" :key="p.name" :value="p.name">{{ p.name }}</option>
            </select>
            <p class="form-hint">选择用于自动回复消息的 AI 提供商配置</p>
          </div>
          <div class="form-group">
            <label>是否启用工具调用</label>
            <select v-model="primaryConfigForm.enable_tool" class="form-input">
              <option :value="false">不启用</option>
              <option :value="true">启用</option>
            </select>
            <p class="form-hint">选择是否让 AI 在回复中调用工具（如计算器等）</p>
          </div>
        </div>

        <div class="form-row">
          <div class="form-group">
            <label>超时时间（秒）</label>
            <input type="number" v-model.number="primaryConfigForm.timeout" class="form-input" min="1" />
          </div>
          <div class="form-group">
            <label>最大 Token 数</label>
            <input type="number" v-model.number="primaryConfigForm.max_tokens" class="form-input" min="1" />
          </div>
        </div>

        <div class="form-group">
          <label>系统提示词</label>
          <textarea v-model="primaryConfigForm.system_prompt" class="form-textarea" rows="6"></textarea>
          <p class="form-hint">用于指导 AI 如何回复用户消息的系统提示词</p>
        </div>

        <div class="form-actions">
          <button class="admin-button admin-button-secondary" @click="closePrimaryConfigForm">取消</button>
          <button class="admin-button" @click="savePrimaryConfig">保存</button>
        </div>
      </div>

      <div v-else-if="primaryConfig" class="tool-router-display">
        <div class="router-status">
          <span class="status-badge" :class="{ active: primaryConfig.enabled, inactive: !primaryConfig.enabled }">
            {{ primaryConfig.enabled ? '已启用' : '已停用' }}
          </span>
        </div>
        <div class="router-details">
          <div class="detail-row">
            <span class="detail-label">使用的 AI 配置</span>
            <span class="detail-value">{{ primaryConfig.provider_name || '未设置' }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">超时时间</span>
            <span class="detail-value">{{ primaryConfig.timeout }} 秒</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">最大 Token 数</span>
            <span class="detail-value">{{ primaryConfig.max_tokens }}</span>
          </div>
          <div class="detail-row">
            <span class="detail-label">工具调用</span>
            <span class="detail-value">{{ primaryConfig.enable_tool ? '已启用' : '未启用' }}</span>
          </div>
          <div class="detail-row full-width">
            <span class="detail-label">系统提示词</span>
            <pre class="detail-value system-prompt">{{ primaryConfig.system_prompt }}</pre>
          </div>
        </div>
      </div>

      <div v-else class="empty-state">
        <p>暂无主 AI 回复配置，点击上方按钮进行配置。</p>
      </div>
    </div>

    <!-- Provider 表单弹窗 -->
    <div v-if="showProviderForm" class="modal-overlay" @click.self="closeProviderForm">
      <div class="modal-content">
        <div class="modal-header">
          <h3>{{ isCreatingProvider ? '添加 AI 提供商配置' : '编辑 AI 提供商配置' }}</h3>
          <button class="modal-close" @click="closeProviderForm">×</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label>配置名称 *</label>
            <input
              type="text"
              v-model="providerForm.name"
              class="form-input"
              :disabled="!isCreatingProvider"
              placeholder="例如：volcengine-ark"
            />
            <p class="form-hint">唯一标识此配置的名称，创建后不可修改</p>
          </div>

          <div class="form-group">
            <label>
              <input type="checkbox" v-model="providerForm.active" />
              启用此配置
            </label>
          </div>

          <div class="form-group">
            <label>API 地址 *</label>
            <input type="text" v-model="providerForm.base_url" class="form-input" placeholder="https://api.example.com/v1" />
          </div>

          <div class="form-group">
            <label>API Key *</label>
            <input type="password" v-model="providerForm.api_key" class="form-input" placeholder="sk-..." />
          </div>

          <div class="form-group">
            <label>模型名称</label>
            <input type="text" v-model="providerForm.model" class="form-input" placeholder="例如：doubao-pro-32k" />
          </div>

          <div class="form-row">
            <div class="form-group">
              <label>超时时间（秒）</label>
              <input type="number" v-model.number="providerForm.timeout" class="form-input" min="1" />
            </div>
            <div class="form-group">
              <label>上下文窗口（tokens）</label>
              <input type="number" v-model.number="providerForm.context_window_tokens" class="form-input" min="1" />
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="admin-button admin-button-secondary" @click="closeProviderForm">取消</button>
          <button class="admin-button" @click="saveProvider">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.admin-llm-api {
  padding: 1.5rem;
  max-width: 100%;
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
  min-height: 100vh;
}

.admin-llm-api h2 {
  margin: 0 0 2rem;
  font-size: 1.75rem;
  font-weight: 700;
  color: #1e293b;
  letter-spacing: -0.02em;
}

.admin-section {
  background: white;
  padding: 1.75rem;
  border-radius: 16px;
  margin-bottom: 2rem;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05), 0 4px 6px rgba(0, 0, 0, 0.03);
  border: 1px solid #e2e8f0;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 1.5rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid #e2e8f0;
}

.section-header h3 {
  margin: 0 0 0.5rem;
  font-size: 1.25rem;
  font-weight: 600;
  color: #334155;
}

.section-desc {
  margin: 0;
  color: #64748b;
  font-size: 0.9rem;
}

.provider-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(380px, 1fr));
  gap: 1.25rem;
}

.provider-card {
  background: linear-gradient(135deg, #fafbfc 0%, #f8fafc 100%);
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 1.25rem;
  transition: all 0.2s ease;
}

.provider-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 20px rgba(0, 0, 0, 0.08);
  border-color: #cbd5e1;
}

.provider-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
  padding-bottom: 0.75rem;
  border-bottom: 1px solid #e2e8f0;
}

.provider-name {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.provider-name strong {
  font-size: 1.05rem;
  color: #1e293b;
  font-weight: 600;
}

.provider-actions {
  display: flex;
  gap: 0.5rem;
}

.provider-details {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}

.detail-row.full-width {
  flex-direction: column;
  align-items: flex-start;
}

.detail-label {
  font-size: 0.85rem;
  color: #64748b;
  font-weight: 500;
  white-space: nowrap;
}

.detail-value {
  font-size: 0.9rem;
  color: #334155;
  word-break: break-all;
  text-align: right;
}

.detail-value.masked {
  font-family: monospace;
  letter-spacing: 0.1em;
}

.status-badge {
  display: inline-block;
  padding: 0.25rem 0.6rem;
  border-radius: 20px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.status-badge.active {
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  color: #166534;
  border: 1px solid #86efac;
}

.status-badge.inactive {
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  color: #991b1b;
  border: 1px solid #fca5a5;
}

.tool-router-form,
.tool-router-display {
  padding: 1.25rem;
  background: #f8fafc;
  border-radius: 12px;
  border: 1px solid #e2e8f0;
}

.router-status {
  margin-bottom: 1rem;
  padding-bottom: 1rem;
  border-bottom: 1px solid #e2e8f0;
}

.router-details {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1rem;
}

.router-details .detail-row {
  flex-direction: column;
  align-items: flex-start;
  gap: 0.25rem;
}

.router-details .detail-row .detail-value {
  text-align: left;
}

.system-prompt {
  width: 100%;
  white-space: pre-wrap;
  word-break: break-word;
  background: white;
  padding: 0.75rem;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  font-size: 0.85rem;
  line-height: 1.6;
  margin: 0;
}

.form-group {
  margin-bottom: 1.25rem;
}

.form-group label {
  display: block;
  margin-bottom: 0.5rem;
  font-weight: 500;
  color: #334155;
  font-size: 0.9rem;
}

.form-group label input[type='checkbox'] {
  margin-right: 0.5rem;
  width: auto;
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 1rem;
}

.form-input,
.form-textarea,
.form-select {
  width: 100%;
  padding: 0.75rem 1rem;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  font-size: 0.9rem;
  color: #334155;
  background: white;
  transition: all 0.15s ease;
  box-sizing: border-box;
}

.form-input:focus,
.form-textarea:focus,
.form-select:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.form-input:disabled {
  background: #f1f5f9;
  cursor: not-allowed;
}

.form-textarea {
  resize: vertical;
  min-height: 100px;
  font-family: inherit;
}

.form-hint {
  margin: 0.5rem 0 0;
  font-size: 0.8rem;
  color: #64748b;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px solid #e2e8f0;
}

.error {
  color: #991b1b;
  padding: 1rem 1.25rem;
  background: linear-gradient(135deg, #fee2e2 0%, #fecaca 100%);
  border-radius: 10px;
  margin-bottom: 1.25rem;
  border: 1px solid #fca5a5;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.success {
  color: #166534;
  padding: 1rem 1.25rem;
  background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
  border-radius: 10px;
  margin-bottom: 1.25rem;
  border: 1px solid #86efac;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.admin-loading {
  padding: 3rem;
  text-align: center;
  color: #64748b;
  background: #f8fafc;
  border-radius: 12px;
  font-size: 1rem;
}

.empty-state {
  padding: 3rem 2rem;
  text-align: center;
  color: #64748b;
  background: #f8fafc;
  border-radius: 12px;
  border: 2px dashed #e2e8f0;
}

.empty-state p {
  margin: 0;
}

/* 弹窗样式 */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 1rem;
}

.modal-content {
  background: white;
  border-radius: 16px;
  width: 100%;
  max-width: 550px;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1.5rem 1.5rem 1rem;
  border-bottom: 1px solid #e2e8f0;
}

.modal-header h3 {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 600;
  color: #1e293b;
}

.modal-close {
  background: none;
  border: none;
  font-size: 1.5rem;
  cursor: pointer;
  color: #64748b;
  padding: 0.25rem 0.5rem;
  line-height: 1;
  border-radius: 6px;
  transition: all 0.15s ease;
}

.modal-close:hover {
  background: #f1f5f9;
  color: #1e293b;
}

.modal-body {
  padding: 1.5rem;
}

.modal-footer {
  padding: 1rem 1.5rem 1.5rem;
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
}

.admin-button {
  padding: 0.6rem 1.25rem;
  border: none;
  border-radius: 8px;
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);
  color: white;
  box-shadow: 0 2px 4px rgba(59, 130, 246, 0.2);
}

.admin-button:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.3);
}

.admin-button:active:not(:disabled) {
  transform: translateY(0);
}

.admin-button-small {
  padding: 0.4rem 0.75rem;
  font-size: 0.8rem;
}

.admin-button-secondary {
  background: linear-gradient(135deg, #64748b 0%, #475569 100%);
  box-shadow: 0 2px 4px rgba(100, 116, 139, 0.2);
}

.admin-button-secondary:hover:not(:disabled) {
  box-shadow: 0 4px 12px rgba(100, 116, 139, 0.3);
}

.admin-button-danger {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
  box-shadow: 0 2px 4px rgba(239, 68, 68, 0.2);
}

.admin-button-danger:hover:not(:disabled) {
  box-shadow: 0 4px 12px rgba(239, 68, 68, 0.3);
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  transform: none !important;
}
</style>
