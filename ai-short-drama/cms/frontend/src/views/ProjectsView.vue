<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useDebounceFn } from '@vueuse/core'
import { ArrowUpRight, Search, RefreshCw, Layers3, Clock3, CircleCheckBig, AlertTriangle, Plus } from 'lucide-vue-next'
import { api } from '../services/api'
import StatusBadge from '../components/StatusBadge.vue'
import EmptyState from '../components/EmptyState.vue'

const projects = ref([])
const total = ref(0)
const loading = ref(true)
const error = ref('')
const search = ref('')
const status = ref('')

const summary = computed(() => ({
  all: total.value,
  active: projects.value.filter((item) => ['running', 'pending'].includes(item.status)).length,
  review: projects.value.reduce((sum, item) => sum + item.pending_reviews, 0),
  failed: projects.value.filter((item) => item.status === 'failed' || item.failed_tasks > 0).length,
}))

async function loadProjects() {
  loading.value = true
  error.value = ''
  try {
    const data = await api.getProjects({ q: search.value, status: status.value, limit: 50 })
    projects.value = data.items
    total.value = data.total
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

watch([search, status], useDebounceFn(loadProjects, 260))
onMounted(loadProjects)

const formatTime = (value) => new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
const progress = (item) => Math.min(100, Math.round((item.generated_episode_count / Math.max(item.target_episode_count, 1)) * 100))
</script>

<template>
  <section class="view-stack">
    <div class="hero-row">
      <div><h2>生产项目</h2><p>追踪从小说拆解到成片发布的完整生产进度。</p></div>
      <div class="hero-actions"><button class="button button-secondary" :disabled="loading" @click="loadProjects"><RefreshCw :size="16" :class="{ spin: loading }" />刷新数据</button><RouterLink to="/projects/new" class="button button-primary"><Plus :size="16" />新建项目</RouterLink></div>
    </div>

    <div class="metric-grid">
      <article class="metric-card"><div class="metric-icon blue"><Layers3 :size="20" /></div><div><span>全部项目</span><strong>{{ summary.all }}</strong><small>数据库累计项目</small></div></article>
      <article class="metric-card"><div class="metric-icon violet"><Clock3 :size="20" /></div><div><span>生产进行中</span><strong>{{ summary.active }}</strong><small>等待或正在执行</small></div></article>
      <article class="metric-card"><div class="metric-icon green"><CircleCheckBig :size="20" /></div><div><span>待人工审核</span><strong>{{ summary.review }}</strong><small>当前审核任务</small></div></article>
      <article class="metric-card"><div class="metric-icon amber"><AlertTriangle :size="20" /></div><div><span>需要关注</span><strong>{{ summary.failed }}</strong><small>项目或任务异常</small></div></article>
    </div>

    <div class="panel">
      <div class="panel-toolbar">
        <div class="search-box"><Search :size="17" /><input v-model="search" placeholder="搜索项目名称或项目 ID" /></div>
        <select v-model="status" class="select-control"><option value="">全部状态</option><option value="running">生产中</option><option value="waiting_review">待审核</option><option value="completed">已完成</option><option value="failed">异常</option></select>
        <span class="result-count">{{ total }} 个项目</span>
      </div>

      <div v-if="error" class="error-banner">{{ error }} <button @click="loadProjects">重试</button></div>
      <div v-if="loading" class="table-loading"><span v-for="i in 4" :key="i"></span></div>
      <EmptyState v-else-if="projects.length === 0" title="没有匹配的项目" description="请调整搜索条件，或通过现有 n8n 项目入口创建项目。" />
      <div v-else class="table-wrap">
        <table>
          <thead><tr><th>项目 / Project ID</th><th>状态</th><th>当前阶段</th><th>集数进度</th><th>错误信息</th><th>更新时间</th><th></th></tr></thead>
          <tbody>
            <tr v-for="item in projects" :key="item.project_id">
              <td><div class="project-cell"><div class="project-cover">{{ item.novel_name.slice(0, 1) }}</div><div><strong>{{ item.novel_name }}</strong><span>{{ item.project_id }}</span></div></div></td>
              <td><StatusBadge :status="item.status" /></td>
              <td><span class="stage-text">{{ item.current_stage.replaceAll('_', ' ') }}</span><small v-if="item.pending_reviews">{{ item.pending_reviews }} 项待审核</small></td>
              <td><div class="progress-label"><span>{{ item.generated_episode_count }} / {{ item.target_episode_count }} 集</span><b>{{ progress(item) }}%</b></div><div class="progress-track"><i :style="{ width: `${progress(item)}%` }"></i></div></td>
              <td><span v-if="item.error_message" class="error-message-cell" :title="item.error_message">{{ item.error_message }}</span><span v-else class="no-error">—</span></td>
              <td><span class="date-text">{{ formatTime(item.updated_at) }}</span></td>
              <td><RouterLink class="row-action" :to="`/projects/${item.project_id}`" aria-label="查看详情"><ArrowUpRight :size="17" /></RouterLink></td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
