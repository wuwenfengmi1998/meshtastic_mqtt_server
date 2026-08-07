<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getBackendVersion } from '../api'
import { FRONTEND_VERSION } from '../version'

const backendVersion = ref('-')
const commitVersion = ref('-')

onMounted(async () => {
  try {
    const res = await getBackendVersion()
    backendVersion.value = res.version
    commitVersion.value = res.commit
  } catch {
    backendVersion.value = '-'
    commitVersion.value = '-'
  }
})
</script>

<template>
  <footer class="app-footer">
    <span>前端 v{{ FRONTEND_VERSION }}</span>
    <span class="app-footer-sep">·</span>
    <span>后端 v{{ backendVersion }}</span>
    <span class="app-footer-sep">·</span>
    <span>提交 {{ commitVersion }}</span>
  </footer>
</template>
