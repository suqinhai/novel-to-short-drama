<script setup>
import StatusBadge from './StatusBadge.vue'
import EmptyState from './EmptyState.vue'

defineProps({
  items: { type: Array, default: () => [] },
  columns: { type: Array, default: () => [] },
})

function cellValue(item, column) {
  const value = item[column.key]
  if (column.format) return column.format(value, item)
  return value === null || value === undefined || value === '' ? '—' : value
}
</script>

<template>
  <EmptyState v-if="items.length === 0" title="暂无记录" description="该项目在当前环节还没有生成数据。" />
  <div v-else class="detail-table-wrap">
    <table class="detail-data-table">
      <thead><tr><th v-for="column in columns" :key="column.key">{{ column.label }}</th></tr></thead>
      <tbody>
        <tr v-for="(item, index) in items" :key="item.task_id || item.review_id || item.novel_id || item.story_bible_id || item.episode_id || item.script_id || item.storyboard_id || index">
          <td v-for="column in columns" :key="column.key" :class="column.class">
            <StatusBadge v-if="column.type === 'status'" :status="item[column.key]" />
            <code v-else-if="column.type === 'id'" :title="String(cellValue(item, column))">{{ cellValue(item, column) }}</code>
            <span v-else :title="String(cellValue(item, column))">{{ cellValue(item, column) }}</span>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
