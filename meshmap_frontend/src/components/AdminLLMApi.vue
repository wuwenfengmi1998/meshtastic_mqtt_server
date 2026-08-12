<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  createLLMProvider,
  deleteLLMProvider,
  getLLMProviders,
  getLLMToolRouter,
  getLLMTopicConfig,
  getLLMPrimaryConfig,
  updateLLMProvider,
  updateLLMToolRouter,
  updateLLMTopicConfig,
  updateLLMPrimaryConfig,
  getAIServiceStatus,
  restartAIService,
} from '../api'
import type { LLMPlatformRouter, LLMTopicConfig, LLMProvider, LLMPrimaryConfig, AIServiceStatus } from '../types'

const loading = ref(false)
const error = ref('')
const success = ref('')
const showWarning = ref(false)

// AI Service Status
const aiStatus = ref<AIServiceStatus>({ running: false, enabled: false, provider_count: 0 })
const restarting = ref(false)

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

// Topic Config 相关 - 话题选择配置
const topicConfig = ref<LLMTopicConfig | null>(null)
const editingTopicConfig = ref(false)

const topicConfigForm = ref({
  enabled: false,
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

async function loadTopicConfig() {
  try {
    const response = await getLLMTopicConfig()
    topicConfig.value = response.item
  } catch (err) {
    // 如果不存在，使用默认值
    console.warn('Topic config not found, using defaults')
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
    let response: any
    if (isCreatingProvider.value) {
      response = await createLLMProvider(providerForm.value)
    } else if (editingProvider.value) {
      response = await updateLLMProvider(editingProvider.value.name, providerForm.value)
    }
    if (response.warning) {
      success.value = response.warning
      showWarning.value = true
    } else {
      success.value = isCreatingProvider.value ? '创建成功' : '更新成功'
      showWarning.value = false
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
    const response = await deleteLLMProvider(name)
    if (response.warning) {
      success.value = response.warning
      showWarning.value = true
    } else {
      success.value = '删除成功'
      showWarning.value = false
    }
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

function openEditTopicConfig() {
  if (topicConfig.value) {
    topicConfigForm.value = {
      enabled: topicConfig.value.enabled,
      openai_name: topicConfig.value.openai_name,
      timeout: topicConfig.value.timeout,
      max_tokens: topicConfig.value.max_tokens,
      system_prompt: topicConfig.value.system_prompt,
    }
  }
  editingTopicConfig.value = true
}

function closeTopicConfigForm() {
  editingTopicConfig.value = false
}

async function saveTopicConfig() {
  try {
    await updateLLMTopicConfig(topicConfigForm.value)
    success.value = '更新成功'
    clearSuccess()
    closeTopicConfigForm()
    await loadTopicConfig()
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

async function loadAIStatus() {
  try {
    aiStatus.value = await getAIServiceStatus()
  } catch (err) {
    console.warn('Failed to load AI service status', err)
  }
}

async function handleRestartAI() {
  if (!confirm('确定要重启 AI 服务吗？')) return
  restarting.value = true
  try {
    const result = await restartAIService()
    aiStatus.value = result
    success.value = result.message || 'AI 服务已重启'
    clearSuccess()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    restarting.value = false
  }
}

onMounted(() => {
  loadAIStatus()
  loadProviders()
  loadToolRouter()
  loadTopicConfig()
  loadPrimaryConfig()
})
</script>

<template>
  <section class="admin-dashboard">
    <div class="panel">
      <div class="ai-status-bar">
        <span class="status-indicator" :class="{ running: aiStatus.running, stopped: !aiStatus.running }"></span>
        <span class="status-text">
          <template v-if="aiStatus.running">AI 服务运行中，{{ aiStatus.provider_count }} 个提供商</template>
          <template v-else>{{ aiStatus.message || 'AI 服务未运行' }}</template>
        </span>
        <button
          class="admin-button"
          :disabled="restarting"
          @click="handleRestartAI"
        >
          {{ restarting ? '重启中...' : '重启 AI 服务' }}
        </button>
      </div>

      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="success" :class="showWarning ? 'warning' : 'success'">{{ success }}</p>
    </div>

    <!-- LLM Provider 列表 -->
    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Providers</p>
          <h2>AI 提供商配置</h2>
        </div>
        <button class="admin-button" @click="openCreateProvider">+ 添加配置</button>
      </div>
      <p class="section-desc">管理多个 LLM API 提供商配置，支持不同模型和服务。</p>

      <div class="section-body">
        <div v-if="loading" class="empty">加载中...</div>

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
                <button class="admin-button" @click="openEditProvider(provider)">编辑</button>
                <button class="admin-button danger" @click="confirmDeleteProvider(provider.name)">删除</button>
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

          <div v-if="providers.length === 0" class="empty">暂无配置，点击上方按钮添加第一个 AI 提供商配置。</div>
        </div>
      </div>
    </div>

    <!-- Tool Router 配置 -->
    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Tool Router</p>
          <h2>工具路由配置</h2>
        </div>
        <button v-if="!editingToolRouter" class="admin-button" @click="openEditToolRouter">编辑配置</button>
      </div>
      <p class="section-desc">配置 LLM 工具调用的路由设置，用于实现函数调用功能。</p>

      <div class="section-body">
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
            <button class="admin-button ghost" @click="closeToolRouterForm">取消</button>
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

        <div v-else class="empty">暂无工具路由配置，点击上方按钮进行配置。</div>
      </div>
    </div>

    <!-- Topic Config 配置 - 话题选择配置 -->
    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Topic Filter</p>
          <h2>话题选择配置</h2>
        </div>
        <button v-if="!editingTopicConfig" class="admin-button" @click="openEditTopicConfig">编辑配置</button>
      </div>
      <p class="section-desc">当工具路由未命中任何工具时，由话题选择判断是否回复。命中（输出 REPLY）才进入主回复，否则丢弃不回复。</p>

      <div class="section-body">
        <div v-if="editingTopicConfig" class="tool-router-form">
          <div class="form-group">
            <label>
              <input type="checkbox" v-model="topicConfigForm.enabled" />
              启用话题选择
            </label>
            <p class="form-hint">未启用时，未命中工具的消息一律进入主回复（不做过滤）</p>
          </div>

          <div class="form-row">
            <div class="form-group">
              <label>使用的 AI 配置</label>
              <select v-model="topicConfigForm.openai_name" class="form-input">
                <option value="">请选择</option>
                <option v-for="p in activeProviders" :key="p.name" :value="p.name">{{ p.name }}</option>
              </select>
              <p class="form-hint">选择用于话题判定的 AI 提供商配置，留空则使用主回复配置</p>
            </div>
          </div>

          <div class="form-row">
            <div class="form-group">
              <label>超时时间（秒）</label>
              <input type="number" v-model.number="topicConfigForm.timeout" class="form-input" min="1" />
            </div>
            <div class="form-group">
              <label>最大 Token 数</label>
              <input type="number" v-model.number="topicConfigForm.max_tokens" class="form-input" min="1" />
            </div>
          </div>

          <div class="form-group">
            <label>系统提示词</label>
            <textarea v-model="topicConfigForm.system_prompt" class="form-textarea" rows="6"></textarea>
            <p class="form-hint">要求模型对应当回复的消息输出 REPLY，否则输出 IGNORE</p>
          </div>

          <div class="form-actions">
            <button class="admin-button ghost" @click="closeTopicConfigForm">取消</button>
            <button class="admin-button" @click="saveTopicConfig">保存</button>
          </div>
        </div>

        <div v-else-if="topicConfig" class="tool-router-display">
          <div class="router-status">
            <span class="status-badge" :class="{ active: topicConfig.enabled, inactive: !topicConfig.enabled }">
              {{ topicConfig.enabled ? '已启用' : '已停用' }}
            </span>
          </div>
          <div class="router-details">
            <div class="detail-row">
              <span class="detail-label">使用的 AI 配置</span>
              <span class="detail-value">{{ topicConfig.openai_name || '未设置（使用主回复配置）' }}</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">超时时间</span>
              <span class="detail-value">{{ topicConfig.timeout }} 秒</span>
            </div>
            <div class="detail-row">
              <span class="detail-label">最大 Token 数</span>
              <span class="detail-value">{{ topicConfig.max_tokens }}</span>
            </div>
            <div class="detail-row full-width">
              <span class="detail-label">系统提示词</span>
              <pre class="detail-value system-prompt">{{ topicConfig.system_prompt }}</pre>
            </div>
          </div>
        </div>

        <div v-else class="empty">暂无话题选择配置，点击上方按钮进行配置。</div>
      </div>
    </div>

    <!-- Primary AI Config 配置 - 主 AI 回复配置 -->
    <div class="panel">
      <div class="panel-header">
        <div>
          <p class="eyebrow">Primary AI</p>
          <h2>主 AI 回复配置</h2>
        </div>
        <button v-if="!editingPrimaryConfig" class="admin-button" @click="openEditPrimaryConfig">编辑配置</button>
      </div>
      <p class="section-desc">配置机器人自动回复消息的核心 AI 设置。</p>

      <div class="section-body">
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
            <button class="admin-button ghost" @click="closePrimaryConfigForm">取消</button>
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

        <div v-else class="empty">暂无主 AI 回复配置，点击上方按钮进行配置。</div>
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
          <button class="admin-button ghost" @click="closeProviderForm">取消</button>
          <button class="admin-button" @click="saveProvider">保存</button>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.ai-status-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
}

.status-indicator {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-indicator.running {
  background: var(--color-success);
  box-shadow: 0 0 6px color-mix(in srgb, var(--color-success) 60%, transparent);
}

.status-indicator.stopped {
  background: var(--color-danger);
  box-shadow: 0 0 6px color-mix(in srgb, var(--color-danger) 50%, transparent);
}

.status-text {
  flex: 1;
  color: var(--color-text);
  font-size: 14px;
  font-weight: 700;
}

.section-desc {
  margin: 0;
  padding: 12px 16px 0;
  color: var(--color-muted);
  font-size: 13px;
  line-height: 1.6;
}

.section-body {
  padding: 16px;
}

.provider-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 12px;
}

.provider-card {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: 14px;
  background: var(--color-surface-soft);
  transition: border-color 0.16s ease, box-shadow 0.16s ease, transform 0.16s ease;
}

.provider-card:hover {
  border-color: var(--color-primary);
  box-shadow: var(--shadow-sm);
  transform: translateY(-1px);
}

.provider-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--color-border);
}

.provider-name {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.provider-name strong {
  color: var(--color-heading);
  font-size: 15px;
  overflow-wrap: anywhere;
}

.provider-actions {
  display: flex;
  gap: 8px;
}

.provider-details {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 12px;
}

.detail-row.full-width {
  flex-direction: column;
  align-items: flex-start;
}

.detail-label {
  flex-shrink: 0;
  color: var(--color-muted);
  font-size: 12px;
  font-weight: 700;
}

.detail-value {
  color: var(--color-heading);
  font-size: 13px;
  word-break: break-all;
  text-align: right;
}

.detail-value.masked {
  font-family: var(--font-mono);
  letter-spacing: 0.1em;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
}

.status-badge.active {
  color: color-mix(in srgb, var(--color-success) 70%, var(--color-heading));
  background: var(--color-success-soft);
}

.status-badge.inactive {
  color: color-mix(in srgb, var(--color-danger) 76%, var(--color-heading));
  background: var(--color-danger-soft);
}

.tool-router-form,
.tool-router-display {
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: 16px;
  background: var(--color-surface-soft);
}

.router-status {
  margin-bottom: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--color-border);
}

.router-details {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}

.router-details .detail-row {
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
}

.router-details .detail-row .detail-value {
  text-align: left;
}

.system-prompt {
  width: 100%;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: 10px 12px;
  color: var(--color-text);
  background: var(--color-surface);
  font-size: 13px;
  line-height: 1.6;
}

.form-group {
  margin-bottom: 14px;
}

.form-group label {
  display: block;
  margin-bottom: 6px;
  color: var(--color-text);
  font-size: 13px;
  font-weight: 700;
}

.form-group label input[type='checkbox'] {
  margin-right: 8px;
  width: auto;
  accent-color: var(--color-primary);
}

.form-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.form-input,
.form-textarea {
  width: 100%;
  box-sizing: border-box;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-sm);
  padding: 9px 12px;
  color: var(--color-heading);
  background: var(--color-surface);
  font-size: 14px;
  outline: none;
  transition: border-color 0.16s ease, box-shadow 0.16s ease;
}

.form-input:focus,
.form-textarea:focus {
  border-color: var(--color-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-primary) 20%, transparent);
}

.form-input:disabled {
  background: var(--color-surface-muted);
  cursor: not-allowed;
}

.form-textarea {
  resize: vertical;
  min-height: 100px;
  font-family: inherit;
}

.form-hint {
  margin: 6px 0 0;
  color: var(--color-muted);
  font-size: 12px;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--color-border);
}

.warning {
  margin: 0 16px 12px;
  border: 1px solid color-mix(in srgb, var(--color-warning) 36%, white);
  border-radius: var(--radius-md);
  padding: 10px 12px;
  color: color-mix(in srgb, var(--color-warning) 72%, var(--color-heading));
  background: var(--color-warning-soft);
}

/* 弹窗样式 */
.modal-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: rgba(36, 41, 39, 0.45);
}

.modal-content {
  width: 100%;
  max-width: 550px;
  max-height: 90vh;
  overflow-y: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  box-shadow: var(--shadow-floating);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 16px;
  border-bottom: 1px solid var(--color-border);
  background: color-mix(in srgb, var(--color-surface) 92%, var(--color-bg));
}

.modal-header h3 {
  margin: 0;
  color: var(--color-heading);
  font-size: 16px;
}

.modal-close {
  border: none;
  border-radius: var(--radius-sm);
  padding: 4px 8px;
  color: var(--color-muted);
  background: none;
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
  transition: background-color 0.16s ease, color 0.16s ease;
}

.modal-close:hover {
  color: var(--color-heading);
  background: var(--color-surface-muted);
}

.modal-body {
  padding: 16px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 16px 16px;
}

@media (max-width: 640px) {
  .form-row {
    grid-template-columns: 1fr;
  }

  .provider-grid {
    grid-template-columns: 1fr;
  }
}
</style>
