<script setup>
import { computed, onMounted, ref } from 'vue'
import { Activity, AlertTriangle, BrainCircuit, CircleCheckBig, Clock3, Container, Database, ExternalLink, HardDrive, Layers3, RefreshCw, Server, ShieldCheck, Table2, TerminalSquare, Workflow, Wrench } from 'lucide-vue-next'
import { api } from '../services/api'
import EmptyState from '../components/EmptyState.vue'
import StatusBadge from '../components/StatusBadge.vue'

const data = ref(null)
const loading = ref(true)
const error = ref('')
const serviceIcons = { n8n: Workflow, postgres: Database, media: HardDrive, 'media-worker': Server, litellm: BrainCircuit }
const failedItems = computed(() => data.value?.failed_tasks?.items || [])

async function load() {
  loading.value = true
  error.value = ''
  try { data.value = await api.getDiagnostics() }
  catch (err) { error.value = err.message }
  finally { loading.value = false }
}

onMounted(load)
const formatTime = (value) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'
const overallTitle = computed(() => ({ healthy: '全部诊断项正常', degraded: '系统可运行，但存在需要处理的警告', unhealthy: '发现阻断性系统问题' }[data.value?.status] || '诊断结果未知'))
</script>

<template>
  <section class="view-stack diagnostic-view">
    <div class="hero-row"><div><h2>系统运行诊断</h2><p>检查 Docker 服务、n8n 工作流、凭证环境、受限节点和最近失败任务。</p></div><button class="button button-primary" :disabled="loading" @click="load"><RefreshCw :size="16" :class="{ spin: loading }" />立即诊断</button></div>
    <div v-if="error" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <div v-else-if="loading" class="diagnostic-loading"><span v-for="i in 5" :key="i"></span></div>
    <template v-else-if="data">
      <div class="health-summary" :class="data.status"><div class="health-icon"><Activity :size="25" /></div><div><span>OVERALL STATUS</span><h3>{{ overallTitle }}</h3><p>最近检查：{{ formatTime(data.checked_at) }} · 共 {{ data.summary.total }} 项检查</p></div><StatusBadge :status="data.status" /></div>

      <div class="metric-grid diagnostic-metrics">
        <article class="metric-card"><div class="metric-icon green"><CircleCheckBig :size="20" /></div><div><span>正常</span><strong>{{ data.summary.healthy }}</strong><small>无需处理</small></div></article>
        <article class="metric-card"><div class="metric-icon amber"><AlertTriangle :size="20" /></div><div><span>警告</span><strong>{{ data.summary.degraded }}</strong><small>建议尽快修复</small></div></article>
        <article class="metric-card"><div class="metric-icon red"><Activity :size="20" /></div><div><span>异常</span><strong>{{ data.summary.unhealthy }}</strong><small>可能阻断流程</small></div></article>
        <article class="metric-card"><div class="metric-icon blue"><Workflow :size="20" /></div><div><span>失败任务</span><strong>{{ data.failed_tasks.total }}</strong><small>展示最近 {{ failedItems.length }} 条</small></div></article>
      </div>

      <article class="panel padded diagnostic-section">
        <div class="section-title"><div><span>DOCKER HEALTH</span><h3>Docker 服务健康</h3></div><div class="section-icon"><Container :size="19" /></div></div>
        <div class="service-grid diagnostic-services">
          <article v-for="service in data.services" :key="service.name" class="service-card" :class="service.status">
            <div class="service-head"><div class="service-icon"><component :is="serviceIcons[service.name] || Server" :size="21" /></div><StatusBadge :status="service.status" /></div>
            <h3>{{ service.name }}</h3><p>{{ service.message }}</p>
            <div class="service-state"><span>container</span><code>{{ service.container_status }}</code><span>health</span><code>{{ service.health }}</code></div>
            <div v-if="service.suggestion" class="inline-suggestion"><Wrench :size="14" /><span>{{ service.suggestion }}</span></div>
            <div class="latency"><Clock3 :size="14" />检查耗时 <strong>{{ service.duration_ms }} ms</strong></div>
          </article>
        </div>
      </article>

      <div class="diagnostic-check-grid">
        <article class="panel padded diagnostic-check-card">
          <div class="diagnostic-check-head"><div class="check-icon"><Workflow :size="20" /></div><div><span>WORKFLOW ACTIVATION</span><h3>工作流启用状态</h3></div><StatusBadge :status="data.workflow_activation.status" /></div>
          <div class="check-value"><strong>{{ data.workflow_activation.active_count }}</strong><span>/ {{ data.workflow_activation.expected_count }} active</span></div>
          <p>{{ data.workflow_activation.message }}</p>
          <div v-if="data.workflow_activation.inactive.length || data.workflow_activation.missing.length" class="check-details"><code v-for="item in data.workflow_activation.inactive" :key="`inactive-${item.id}`">inactive · {{ item.name }}</code><code v-for="item in data.workflow_activation.missing" :key="`missing-${item.id}`">missing · {{ item.name }}</code></div>
        </article>

        <article class="panel padded diagnostic-check-card">
          <div class="diagnostic-check-head"><div class="check-icon"><ShieldCheck :size="20" /></div><div><span>CREDENTIAL ENV</span><h3>Postgres Credential</h3></div><StatusBadge :status="data.postgres_credential.status" /></div>
          <div class="credential-value"><code>POSTGRES_CREDENTIAL_ID</code><strong>{{ data.postgres_credential.configured ? '已配置' : data.postgres_credential.exists ? '存在但无效' : '缺失' }}</strong></div>
          <p>{{ data.postgres_credential.message }}</p>
        </article>

        <article class="panel padded diagnostic-check-card">
          <div class="diagnostic-check-head"><div class="check-icon warning"><TerminalSquare :size="20" /></div><div><span>NODE COMPATIBILITY</span><h3>executeCommand 扫描</h3></div><StatusBadge :status="data.execute_command.status" /></div>
          <div class="check-value"><strong>{{ data.execute_command.count }}</strong><span>个受限节点</span></div>
          <p>{{ data.execute_command.message }}</p>
          <div v-if="data.execute_command.nodes.length" class="execute-node-list"><div v-for="node in data.execute_command.nodes" :key="`${node.workflow_id}:${node.node_name}`"><code>{{ node.file }}</code><strong>{{ node.node_name }}</strong></div></div>
        </article>
      </div>

      <article v-if="data.recommendations.length" class="panel padded recommendation-panel">
        <div class="section-title"><div><span>ACTIONABLE GUIDANCE</span><h3>修复建议</h3></div><div class="section-icon warning"><Wrench :size="19" /></div></div>
        <div class="recommendation-list"><article v-for="(item, index) in data.recommendations" :key="`${item.title}-${index}`" :class="item.severity"><span>{{ index + 1 }}</span><div><div><strong>{{ item.title }}</strong><StatusBadge :status="item.severity" /></div><p>{{ item.description }}</p></div></article></div>
      </article>

      <article class="panel failed-task-panel">
        <div class="failed-task-head"><div><span>RECENT FAILURES</span><h3>最近 20 条失败 workflow_tasks</h3><p>{{ data.failed_tasks.message }}</p></div><StatusBadge :status="data.failed_tasks.status" /></div>
        <EmptyState v-if="failedItems.length === 0" title="最近没有失败任务" description="workflow_tasks 当前没有 failed 记录。" />
        <div v-else class="failed-task-list"><article v-for="item in failedItems" :key="item.task_id"><div class="failed-task-icon"><AlertTriangle :size="17" /></div><div class="failed-task-main"><div><strong>{{ item.workflow_stage }}</strong><code>{{ item.task_id }}</code></div><p>{{ item.error_message || '未记录错误信息' }}</p><span>{{ item.error_code || 'NO_ERROR_CODE' }} · {{ item.entity_type }} / {{ item.entity_id }}</span></div><div class="failed-task-project"><span>{{ item.novel_name }}</span><RouterLink :to="`/projects/${item.project_id}`"><ExternalLink :size="11" />{{ item.project_id }}</RouterLink></div><div class="failed-task-time"><strong>{{ formatTime(item.updated_at) }}</strong><span>重试 {{ item.retry_count }} / {{ item.max_retries }}</span></div></article></div>
      </article>

      <article class="panel padded">
        <div class="section-title"><div><span>DATABASE SNAPSHOT</span><h3>业务数据库快照</h3></div><div class="database-name"><Database :size="15" />{{ data.database.database || 'short_drama' }}</div></div>
        <div class="db-stats"><div><Table2 :size="18" /><span>业务表</span><strong>{{ data.database.schema_table_count }}</strong></div><div><Layers3 :size="18" /><span>生产项目</span><strong>{{ data.database.project_count }}</strong></div><div><Activity :size="18" /><span>活跃任务</span><strong>{{ data.database.active_tasks }}</strong></div><div><Clock3 :size="18" /><span>待审核</span><strong>{{ data.database.pending_reviews }}</strong></div></div>
        <p class="version-line">PostgreSQL {{ data.database.version }}</p>
      </article>
    </template>
  </section>
</template>
