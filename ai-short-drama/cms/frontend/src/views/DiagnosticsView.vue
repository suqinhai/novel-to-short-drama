<script setup>
import { onMounted, ref } from 'vue'
import { Activity, Database, Workflow, HardDrive, RefreshCw, Server, Clock3, Table2, Layers3 } from 'lucide-vue-next'
import { api } from '../services/api'
import StatusBadge from '../components/StatusBadge.vue'

const data = ref(null)
const loading = ref(true)
const error = ref('')
const icons = { PostgreSQL: Database, n8n: Workflow, 媒体服务: HardDrive }

async function load() {
  loading.value = true
  error.value = ''
  try { data.value = await api.getDiagnostics() }
  catch (err) { error.value = err.message }
  finally { loading.value = false }
}
onMounted(load)
const formatTime = (value) => new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value))
</script>

<template>
  <section class="view-stack">
    <div class="hero-row"><div><h2>系统运行状态</h2><p>检查 CMS 依赖的数据库、工作流与媒体服务连通性。</p></div><button class="button button-primary" :disabled="loading" @click="load"><RefreshCw :size="16" :class="{ spin: loading }" />立即诊断</button></div>
    <div v-if="error" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <div v-else-if="loading" class="diagnostic-loading"><span v-for="i in 3" :key="i"></span></div>
    <template v-else-if="data">
      <div class="health-summary" :class="data.status"><div class="health-icon"><Activity :size="25" /></div><div><span>OVERALL STATUS</span><h3>{{ data.status === 'healthy' ? '所有核心服务运行正常' : '部分服务需要关注' }}</h3><p>最近检查：{{ formatTime(data.checked_at) }}</p></div><StatusBadge :status="data.status" /></div>
      <div class="service-grid">
        <article v-for="component in data.components" :key="component.name" class="service-card">
          <div class="service-head"><div class="service-icon"><component :is="icons[component.name] || Server" :size="21" /></div><StatusBadge :status="component.status" /></div>
          <h3>{{ component.name }}</h3><p>{{ component.message }}</p><div class="latency"><Clock3 :size="14" />响应耗时 <strong>{{ component.latency_ms }} ms</strong></div>
        </article>
      </div>
      <article class="panel padded">
        <div class="section-title"><div><span>DATABASE SNAPSHOT</span><h3>业务数据库快照</h3></div><div class="database-name"><Database :size="15" />{{ data.database.database }}</div></div>
        <div class="db-stats"><div><Table2 :size="18" /><span>业务表</span><strong>{{ data.database.schema_table_count }}</strong></div><div><Layers3 :size="18" /><span>生产项目</span><strong>{{ data.database.project_count }}</strong></div><div><Activity :size="18" /><span>活跃任务</span><strong>{{ data.database.active_tasks }}</strong></div><div><Clock3 :size="18" /><span>待审核</span><strong>{{ data.database.pending_reviews }}</strong></div></div>
        <p class="version-line">PostgreSQL {{ data.database.version }}</p>
      </article>
    </template>
  </section>
</template>
