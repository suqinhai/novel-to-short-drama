<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { CheckCircle2, CircleAlert, LoaderCircle, RefreshCw } from 'lucide-vue-next'
import { narrativeApi } from '../services/narrativeApi'

const props = defineProps({ operation: { type: Object, default: null } })
const emit = defineEmits(['terminal'])
const current = ref(props.operation)
const error = ref('')
const polling = ref(false)
let timer = 0
const terminalStatuses = new Set(['completed', 'partially_failed', 'failed', 'cancelled', 'needs_review'])
const terminal = computed(() => current.value && terminalStatuses.has(current.value.status))
const successful = computed(() => current.value?.status === 'completed')

function stop() {
  window.clearTimeout(timer)
  timer = 0
  polling.value = false
}

async function refresh() {
  if (!current.value?.operation_id) return
  polling.value = true
  error.value = ''
  try {
    const response = await narrativeApi.getOperation(current.value.operation_id)
    current.value = response.data
    if (terminal.value) {
      stop()
      emit('terminal', current.value)
    } else {
      timer = window.setTimeout(refresh, 2000)
    }
  } catch (err) {
    error.value = err.message
    polling.value = false
  }
}

watch(() => props.operation, (value) => {
  stop()
  current.value = value
  if (value?.operation_id && !terminalStatuses.has(value.status)) refresh()
}, { immediate: true })
onBeforeUnmount(stop)
</script>

<template>
  <article v-if="current" class="operation-card" :class="{ success: successful, failed: terminal && !successful }">
    <div class="operation-icon">
      <CheckCircle2 v-if="successful" :size="20" />
      <CircleAlert v-else-if="terminal" :size="20" />
      <LoaderCircle v-else :size="20" class="spin" />
    </div>
    <div class="operation-main">
      <span>异步操作 · {{ current.operation_type }}</span>
      <strong>{{ current.status }}</strong>
      <code>{{ current.operation_id }}</code>
      <p v-if="current.checkpoint?.stage">{{ current.checkpoint.stage }} · {{ current.checkpoint.completed_items || 0 }} / {{ current.checkpoint.total_items || '—' }}</p>
      <p v-if="current.error">{{ current.error.message }}</p>
      <p v-if="error" class="operation-error">状态刷新失败：{{ error }}</p>
    </div>
    <button v-if="error" class="button button-secondary" @click="refresh"><RefreshCw :size="15" />重试查询</button>
  </article>
</template>
