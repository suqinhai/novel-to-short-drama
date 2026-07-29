<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { CheckCircle2, CircleAlert, LoaderCircle, RefreshCw } from 'lucide-vue-next'
import { narrativeApi } from '../services/narrativeApi'
import { createTerminalNotifier, isFailedOperation, isReviewOperation, isTerminalOperation } from '../services/operationTerminal'
import { getDisplayValueLabel, getStatusLabel } from '../services/displayLabels'

const props = defineProps({
  operation: { type: Object, default: null },
  reviewActionLabel: { type: String, default: '' },
})
const emit = defineEmits(['terminal', 'review'])
const current = ref(props.operation)
const error = ref('')
const polling = ref(false)
let timer = 0
const terminal = computed(() => isTerminalOperation(current.value))
const successful = computed(() => current.value?.status === 'completed')
const reviewable = computed(() => isReviewOperation(current.value))
const failed = computed(() => isFailedOperation(current.value))
const hasProgressCounts = computed(() => Number.isInteger(current.value?.checkpoint?.completed_items) &&
  Number.isInteger(current.value?.checkpoint?.total_items))
const notifyTerminal = createTerminalNotifier((operation) => emit('terminal', operation))

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
      notifyTerminal(current.value)
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
  if (isTerminalOperation(value)) notifyTerminal(value)
  else if (value?.operation_id) refresh()
}, { immediate: true })
onBeforeUnmount(stop)
</script>

<template>
  <article v-if="current" class="operation-card" :class="{ success: successful, review: reviewable, failed }">
    <div class="operation-icon">
      <CheckCircle2 v-if="successful" :size="20" />
      <CircleAlert v-else-if="terminal" :size="20" />
      <LoaderCircle v-else :size="20" class="spin" />
    </div>
    <div class="operation-main">
      <span>异步操作 · {{ getDisplayValueLabel(current.operation_type) }}</span>
      <strong>{{ getStatusLabel(current.status) }}</strong>
      <code>{{ current.operation_id }}</code>
      <p v-if="current.checkpoint?.stage">
        {{ getDisplayValueLabel(current.checkpoint.stage) }}
        <template v-if="hasProgressCounts"> · {{ current.checkpoint.completed_items }} / {{ current.checkpoint.total_items }}</template>
      </p>
      <p v-if="current.error">{{ current.error.message }}</p>
      <p v-if="error" class="operation-error">状态刷新失败：{{ error }}</p>
    </div>
    <button v-if="error" class="button button-secondary" @click="refresh"><RefreshCw :size="15" />重试查询</button>
    <button v-else-if="reviewable && reviewActionLabel" class="button button-primary" @click="emit('review')">{{ reviewActionLabel }}</button>
  </article>
</template>
