<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getHealth, getNodes, getPositions, getTextMessages } from './api'
import type { HealthStatus, NodeInfoMap, PositionRecord, TextMessage } from './types'

const loading = ref(true)
const error = ref('')
const health = ref<HealthStatus | null>(null)
const nodes = ref<NodeInfoMap[]>([])
const messages = ref<TextMessage[]>([])
const positions = ref<PositionRecord[]>([])

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const [healthData, nodeData, messageData, positionData] = await Promise.all([
      getHealth(),
      getNodes(),
      getTextMessages(),
      getPositions(),
    ])
    health.value = healthData
    nodes.value = nodeData.items
    messages.value = messageData.items
    positions.value = positionData.items
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <main class="page">
    <header class="header">
      <div>
        <p class="eyebrow">Meshtastic MQTT Server</p>
        <h1>MeshMap Dashboard</h1>
      </div>
      <button @click="refresh" :disabled="loading">{{ loading ? '刷新中...' : '刷新' }}</button>
    </header>

    <section class="status" :class="{ ok: health?.status === 'ok' }">
      <strong>服务状态</strong>
      <span>{{ health?.status ?? 'unknown' }}</span>
      <span>database: {{ health?.database ?? 'unknown' }}</span>
    </section>

    <p v-if="error" class="error">{{ error }}</p>

    <section class="grid">
      <article class="card wide">
        <h2>节点</h2>
        <table>
          <thead>
            <tr>
              <th>Node</th>
              <th>Name</th>
              <th>Role</th>
              <th>HW</th>
              <th>Lat</th>
              <th>Lon</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="node in nodes" :key="node.node_id">
              <td>{{ node.node_id }}</td>
              <td>{{ node.long_name || node.short_name || '-' }}</td>
              <td>{{ node.role || '-' }}</td>
              <td>{{ node.hw_model || '-' }}</td>
              <td>{{ node.latitude ?? '-' }}</td>
              <td>{{ node.longitude ?? '-' }}</td>
            </tr>
          </tbody>
        </table>
      </article>

      <article class="card">
        <h2>最近聊天</h2>
        <ul class="list">
          <li v-for="msg in messages" :key="msg.id">
            <strong>{{ msg.from_id }}</strong>
            <span>{{ msg.text || '[binary]' }}</span>
            <small>{{ msg.mqtt_remote_host || '-' }}</small>
          </li>
        </ul>
      </article>

      <article class="card">
        <h2>最近位置</h2>
        <ul class="list">
          <li v-for="pos in positions" :key="pos.id">
            <strong>{{ pos.from_id }}</strong>
            <span>{{ pos.latitude ?? '-' }}, {{ pos.longitude ?? '-' }}</span>
            <small>alt {{ pos.altitude ?? '-' }}</small>
          </li>
        </ul>
      </article>
    </section>
  </main>
</template>
