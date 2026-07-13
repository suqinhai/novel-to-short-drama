<script setup>
import { Inbox } from 'lucide-vue-next'
defineProps({ title: { type: String, default: '暂无数据' }, description: { type: String, default: '当前没有可展示的内容' } })
</script>

<template><div class="empty-state"><div><Inbox :size="26" /></div><h3>{{ title }}</h3><p>{{ description }}</p></div></template>
