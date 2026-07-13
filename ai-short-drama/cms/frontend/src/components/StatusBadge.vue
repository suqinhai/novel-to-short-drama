<script setup>
import { computed } from 'vue'

const props = defineProps({ status: { type: String, default: '' } })
const labels = {
  pending: '待处理', running: '生产中', completed: '已完成', failed: '异常',
  waiting_review: '待审核', cancelled: '已取消', healthy: '正常', unhealthy: '异常', degraded: '需关注',
  approved: '已通过', rejected: '已拒绝', draft: '草稿', pending_review: '待审核', skipped: '已跳过',
}
const tone = computed(() => ({
  completed: 'success', healthy: 'success', running: 'info', waiting_review: 'warning',
  pending: 'neutral', failed: 'danger', unhealthy: 'danger', degraded: 'warning', cancelled: 'neutral',
  approved: 'success', rejected: 'danger', draft: 'neutral', pending_review: 'warning', skipped: 'neutral',
}[props.status] || 'neutral'))
</script>

<template><span class="status-badge" :class="`status-${tone}`"><i></i>{{ labels[status] || status || '未知' }}</span></template>
