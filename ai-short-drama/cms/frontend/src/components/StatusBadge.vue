<script setup>
import { computed } from 'vue'
import { getStatusLabel, statusTones } from '../services/displayLabels'

const props = defineProps({ status: { type: String, default: '' } })
const tone = computed(() => statusTones[props.status] || 'neutral')
</script>

<template><span class="status-badge" :class="`status-${tone}`" :title="status || '未知'"><i></i>{{ getStatusLabel(status) }}</span></template>
