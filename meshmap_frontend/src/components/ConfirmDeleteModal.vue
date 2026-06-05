<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  open: boolean
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  requireReason?: boolean
  reasonLabel?: string
  reasonPlaceholder?: string
}>(), {
  confirmText: '确认',
  cancelText: '取消',
  requireReason: false,
  reasonLabel: '屏蔽原因',
  reasonPlaceholder: '请输入屏蔽原因',
})

const emit = defineEmits<{
  cancel: []
  confirm: [payload: { reason?: string }]
}>()

const reason = ref('')
const reasonInputRef = ref<HTMLTextAreaElement | null>(null)
const trimmedReason = computed(() => reason.value.trim())
const confirmDisabled = computed(() => props.requireReason && !trimmedReason.value)

function cancel() {
  emit('cancel')
}

function confirm() {
  if (confirmDisabled.value) {
    return
  }
  emit('confirm', props.requireReason ? { reason: trimmedReason.value } : {})
}

function handleKeydown(event: KeyboardEvent) {
  if (!props.open) {
    return
  }
  if (event.key === 'Escape') {
    cancel()
  }
}

watch(
  () => props.open,
  async (open) => {
    reason.value = ''
    if (open && props.requireReason) {
      await nextTick()
      reasonInputRef.value?.focus()
    }
  },
)

watch(
  () => props.open,
  (open) => {
    if (open) {
      window.addEventListener('keydown', handleKeydown)
    } else {
      window.removeEventListener('keydown', handleKeydown)
    }
  },
)

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div v-if="open" class="modal-backdrop" @click.self="cancel">
    <section class="confirm-modal" role="dialog" aria-modal="true" :aria-label="title">
      <div class="confirm-modal-header">
        <div>
          <p class="eyebrow">Confirm</p>
          <h2>{{ title }}</h2>
        </div>
        <button class="confirm-modal-close" type="button" aria-label="关闭" @click="cancel">×</button>
      </div>

      <div class="confirm-modal-body">
        <p>{{ message }}</p>
        <label v-if="requireReason" class="confirm-modal-reason">
          <span>{{ reasonLabel }}</span>
          <textarea
            ref="reasonInputRef"
            v-model="reason"
            rows="3"
            :placeholder="reasonPlaceholder"
          ></textarea>
        </label>
      </div>

      <div class="confirm-modal-actions">
        <button class="confirm-modal-secondary" type="button" @click="cancel">{{ cancelText }}</button>
        <button class="confirm-modal-danger" type="button" :disabled="confirmDisabled" @click="confirm">
          {{ confirmText }}
        </button>
      </div>
    </section>
  </div>
</template>
