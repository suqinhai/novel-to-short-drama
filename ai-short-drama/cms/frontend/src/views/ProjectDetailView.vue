<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, RefreshCw, BookOpen, Clapperboard, Image, Video, ListChecks, Layers3, GitBranch, ClipboardCheck, FileText, BookMarked, ListVideo, ScrollText, PanelsTopLeft, CircleCheckBig, Webhook, Play, RotateCcw, LoaderCircle, AlertCircle, SlidersHorizontal, GitCompareArrows, Coins, Gauge, LockKeyhole, Eye } from 'lucide-vue-next'
import { api } from '../services/api'
import { getPipelineProgress, getPipelineStageIndex, getPipelineStageLabel, getStageUnitProgress, pipelineStages } from '../services/pipelineStage'
import { getDisplayValueLabel } from '../services/displayLabels'
import StatusBadge from '../components/StatusBadge.vue'
import DetailDataTable from '../components/DetailDataTable.vue'
import EpisodeContentModal from '../components/EpisodeContentModal.vue'

const route = useRoute()
const project = ref(null)
const loading = ref(true)
const error = ref('')
const activeDataTab = ref('workflow_tasks')
const createResult = ref(null)
const flowResult = ref(null)
const flowError = ref('')
const flowLoading = ref(false)
const retryingTaskId = ref('')
const runningEpisodeId = ref('')
const selectedEpisodeRun = ref(null)
const stages = pipelineStages
const currentIndex = computed(() => {
  if (!project.value) return -1
  return getPipelineStageIndex(project.value.current_stage, project.value.status)
})
const productionProgress = computed(() => {
  if (!project.value) return getPipelineProgress('', '')
  return getPipelineProgress(project.value.current_stage, project.value.status)
})
const stageUnitProgress = computed(() => getStageUnitProgress(project.value || {}))
const activeWorkflowTasks = computed(() => project.value?.workflow_tasks?.filter((item) => ['pending', 'running'].includes(item.status)).length || 0)
const episodeRuns = computed(() => project.value?.rolling_production?.episodes || [])
const rollingArcs = computed(() => project.value?.rolling_production?.arcs || [])
const isRollingProduction = computed(() => rollingArcs.value.length > 0)
const currentEpisodeRun = computed(() => episodeRuns.value.find((item) => ['active', 'waiting_review', 'paused', 'failed'].includes(item.status)))
const resumeDisabledReason = computed(() => {
  if (flowLoading.value) return '流程请求正在提交，请勿重复操作'
  if (activeWorkflowTasks.value > 0) return `当前有 ${activeWorkflowTasks.value} 个任务正在生产中，请等待任务结束`
  if (project.value?.counts?.pending_reviews > 0) return '当前有内容等待审核，请先完成审核'
  if (project.value?.status === 'completed') return '项目已经生产完成'
  if (project.value?.status === 'cancelled') return '回收站中的项目不能推进流程'
  return ''
})
const canResume = computed(() => !resumeDisabledReason.value)
const episodeRunDisplayStatus = (run) => (
  run?.status === 'waiting_review' && project.value?.counts?.pending_reviews === 0
    ? 'ready_to_continue'
    : run?.status
)
const canStartEpisode = (run) => {
  if (!run) return false
  if (flowLoading.value || activeWorkflowTasks.value > 0 || project.value?.counts?.pending_reviews > 0) return false
  if (['completed', 'cancelled'].includes(run.status)) return false
  if (currentEpisodeRun.value && currentEpisodeRun.value.episode_run_id !== run.episode_run_id) return false
  return !episodeRuns.value.some((item) => item.arc_run_id === run.arc_run_id &&
    item.episode_number < run.episode_number && item.status !== 'completed')
}
const episodeActionLabel = (run) => {
  if (runningEpisodeId.value === run.episode_run_id) return '提交中…'
  if (run.status === 'queued') return `生成第 ${run.episode_number} 集`
  if (run.status === 'failed') return '重试本集'
  return '继续本集'
}
const stepStatusLabel = (index) => {
  if (index < productionProgress.value.completedStages) return '已完成'
  if (index === productionProgress.value.currentIndex) {
    if (project.value?.status === 'failed') return '异常'
    if (project.value?.counts?.pending_reviews > 0) return '待审核'
    return '进行中'
  }
  return '待开始'
}

const formatShortDate = (value) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'
const formatDuration = (value) => `${value || 0} 秒`
const formatProductionStage = (value) => {
  const generalLabel = getDisplayValueLabel(value)
  return generalLabel === '其他' ? getPipelineStageLabel(value) : generalLabel
}
const productionTabs = computed(() => {
  if (!project.value) return []
  return [
    { key: 'workflow_tasks', label: '工作流任务', icon: GitBranch, items: project.value.workflow_tasks, columns: [
      { key: 'task_id', label: '任务 ID', type: 'id' }, { key: 'workflow_stage', label: '阶段', format: formatProductionStage }, { key: 'action', label: '动作', format: getDisplayValueLabel },
      { key: 'status', label: '状态', type: 'status' }, { key: 'generation_version', label: '版本', format: (v) => `v${v}` },
      { key: 'error_message', label: '错误信息', class: 'wide-cell' }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
      { key: 'retry_action', label: '操作', type: 'action', visible: (item) => item.status === 'failed', disabled: (item) => flowLoading.value || activeWorkflowTasks.value > 0 || retryingTaskId.value === item.task_id, labelFor: (item) => retryingTaskId.value === item.task_id ? '重试中…' : '重试' },
    ] },
    { key: 'review_tasks', label: '审核任务', icon: ClipboardCheck, items: project.value.review_tasks, columns: [
      { key: 'review_id', label: '审核 ID', type: 'id' }, { key: 'stage', label: '阶段', format: formatProductionStage }, { key: 'entity_type', label: '对象类型', format: getDisplayValueLabel },
      { key: 'review_status', label: '审核状态', type: 'status' }, { key: 'review_comment', label: '审核意见', class: 'wide-cell' },
      { key: 'created_at', label: '创建时间', format: formatShortDate }, { key: 'reviewed_at', label: '审核时间', format: formatShortDate },
    ] },
    { key: 'novels', label: '小说', icon: FileText, items: project.value.novels, columns: [
      { key: 'novel_id', label: '小说 ID', type: 'id' }, { key: 'name', label: '小说名' }, { key: 'source_type', label: '来源', format: getDisplayValueLabel },
      { key: 'encoding', label: '编码' }, { key: 'total_chars', label: '总字数', format: (v) => Number(v).toLocaleString('zh-CN') },
      { key: 'chapter_count', label: '章节数' }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
    { key: 'story_bibles', label: '故事圣经', icon: BookMarked, items: project.value.story_bibles, columns: [
      { key: 'story_bible_id', label: '故事圣经 ID', type: 'id' }, { key: 'version', label: '版本', format: (v) => `v${v}` },
      { key: 'status', label: '状态', type: 'status' }, { key: 'character_count', label: '角色' }, { key: 'location_count', label: '地点' },
      { key: 'key_event_count', label: '关键事件' }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
    { key: 'episodes', label: '分集', icon: ListVideo, items: project.value.episodes, columns: [
      { key: 'episode_number', label: '集数', format: (v) => `第 ${v} 集` }, { key: 'episode_id', label: '剧集 ID', type: 'id' },
      { key: 'title', label: '标题' }, { key: 'status', label: '状态', type: 'status' }, { key: 'version', label: '版本', format: (v) => `v${v}` },
      { key: 'estimated_duration_seconds', label: '预计时长', format: formatDuration }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
    { key: 'scripts', label: '剧本', icon: ScrollText, items: project.value.scripts, columns: [
      { key: 'script_id', label: '剧本 ID', type: 'id' }, { key: 'episode_id', label: '剧集 ID', type: 'id' }, { key: 'title', label: '标题' },
      { key: 'status', label: '状态', type: 'status' }, { key: 'version', label: '版本', format: (v) => `v${v}` }, { key: 'scene_count', label: '场景数' },
      { key: 'dialogue_char_count', label: '对白字数' }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
    { key: 'storyboards', label: '分镜', icon: PanelsTopLeft, items: project.value.storyboards, columns: [
      { key: 'storyboard_id', label: '分镜 ID', type: 'id' }, { key: 'episode_id', label: '剧集 ID', type: 'id' },
      { key: 'status', label: '状态', type: 'status' }, { key: 'version', label: '版本', format: (v) => `v${v}` },
      { key: 'total_shots', label: '镜头数' }, { key: 'estimated_duration_seconds', label: '预计时长', format: formatDuration },
      { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
  ]
})
const activeTab = computed(() => productionTabs.value.find((tab) => tab.key === activeDataTab.value) || productionTabs.value[0])

async function runFlowAction(action, taskId = '', episodeRunId = '') {
  if (flowLoading.value || (action === 'resume' && (episodeRunId ? !canStartEpisode(episodeRuns.value.find((item) => item.episode_run_id === episodeRunId)) : !canResume.value))) return
  flowLoading.value = true
  retryingTaskId.value = taskId
  runningEpisodeId.value = episodeRunId
  flowError.value = ''
  flowResult.value = null
  try {
    const response = await api.runProjectAction(project.value.project_id, {
      action,
      task_id: taskId || undefined,
      episode_run_id: episodeRunId || undefined,
    })
    flowResult.value = response
    project.value = response.project
  } catch (err) {
    flowError.value = err.message
  } finally {
    flowLoading.value = false
    retryingTaskId.value = ''
    runningEpisodeId.value = ''
  }
}

function handleTableAction({ item, column }) {
  if (column.key === 'retry_action' && item.status === 'failed') {
    runFlowAction('retry', item.task_id, currentEpisodeRun.value?.episode_run_id || '')
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try { project.value = await api.getProject(route.params.projectId) }
  catch (err) { error.value = err.message }
  finally { loading.value = false }
}

function openEpisodeContent(run) {
  selectedEpisodeRun.value = run
}

async function handleEpisodeContentSaved() {
  await load()
  if (selectedEpisodeRun.value) {
    selectedEpisodeRun.value = episodeRuns.value.find(
      (run) => run.episode_run_id === selectedEpisodeRun.value.episode_run_id,
    ) || selectedEpisodeRun.value
  }
}
onMounted(() => {
  if (route.query.created === '1') {
    try {
      const stored = sessionStorage.getItem(`cms:create-result:${route.params.projectId}`)
      if (stored) createResult.value = JSON.parse(stored)
    } catch { /* 无可用缓存时只展示项目详情 */ }
  }
  load()
})

const formatDate = (value) => new Intl.DateTimeFormat('zh-CN', { dateStyle: 'long', timeStyle: 'short' }).format(new Date(value))
const createResultText = computed(() => JSON.stringify(createResult.value, null, 2))
</script>

<template>
  <section class="view-stack">
    <RouterLink to="/projects" class="back-link"><ArrowLeft :size="16" />返回项目列表</RouterLink>
    <div v-if="loading" class="detail-skeleton"><span></span><span></span><span></span></div>
    <div v-else-if="error" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <template v-else-if="project">
      <div class="detail-hero">
        <div class="detail-title"><div class="project-cover large">{{ project.novel_name.slice(0, 1) }}</div><div><div class="title-line"><h2>{{ project.novel_name }}</h2><StatusBadge :status="project.status" /></div><p>{{ project.project_id }} · 创建于 {{ formatDate(project.created_at) }}</p></div></div>
        <div class="detail-actions"><RouterLink v-if="project.episodes?.[0]?.episode_id" class="button button-primary" :to="`/projects/${project.project_id}/episodes/${project.episodes[0].episode_id}/workbench`"><Clapperboard :size="16" />统一创作工作台</RouterLink><RouterLink class="button button-secondary" :to="`/projects/${project.project_id}/performance-continuity`"><GitCompareArrows :size="16" />表演与连续性</RouterLink><RouterLink class="button button-secondary" :to="`/projects/${project.project_id}/local-edit`"><GitCompareArrows :size="16" />局部精修</RouterLink><RouterLink class="button button-secondary" :to="`/projects/${project.project_id}/adaptation-diagnostics`"><Gauge :size="16" />改编诊断</RouterLink><RouterLink class="button button-secondary" :to="`/projects/${project.project_id}/candidates`"><GitCompareArrows :size="16" />候选工作台</RouterLink><RouterLink class="button button-secondary" :to="`/projects/${project.project_id}/impact`"><GitCompareArrows :size="16" />修订影响</RouterLink><RouterLink class="button button-secondary" :to="`/projects/${project.project_id}/adaptation-scope`"><SlidersHorizontal :size="16" />改编范围</RouterLink><button class="button button-secondary" :disabled="loading" @click="load"><RefreshCw :size="16" />刷新详情</button><button v-if="!isRollingProduction" class="button button-primary" :disabled="!canResume" :title="resumeDisabledReason || '继续推进到下一生产节点'" @click="runFlowAction('resume')"><LoaderCircle v-if="flowLoading && !retryingTaskId" :size="16" class="spin" /><Play v-else :size="16" />{{ flowLoading && !retryingTaskId ? '推进中…' : activeWorkflowTasks ? '生产执行中' : '继续生产' }}</button></div>
      </div>

      <article v-if="createResult" class="creation-result-card">
        <div class="creation-result-head"><div class="creation-success-icon"><CircleCheckBig :size="21" /></div><div><span>N8N WEBHOOK RESPONSE</span><h3>项目已提交，返回结果如下</h3></div><Webhook :size="20" /></div>
        <pre>{{ createResultText }}</pre>
      </article>

      <div v-if="flowError" class="error-banner large flow-error"><AlertCircle :size="17" />{{ flowError }}<button @click="flowError = ''">关闭</button></div>
      <article v-if="flowResult" class="flow-result-card" :class="{ failed: flowResult.n8n_response?.success === false }">
        <div class="flow-result-head"><div class="flow-result-icon"><RotateCcw :size="20" /></div><div><span>自动化流程 · {{ getDisplayValueLabel(flowResult.action) }}</span><h3>流程调用已返回</h3></div><div class="latest-state"><span>最新状态</span><strong>{{ getPipelineStageLabel(flowResult.project.current_stage, flowResult.project.status) }}</strong><StatusBadge :status="flowResult.project.status" /></div></div>
        <pre>{{ JSON.stringify(flowResult.n8n_response, null, 2) }}</pre>
      </article>

      <article v-if="isRollingProduction" class="panel padded rolling-production-panel">
        <div class="section-title">
          <div><span>ROLLING EPISODE PRODUCTION</span><h3>单集滚动生产队列</h3></div>
          <div class="rolling-guard"><LockKeyhole :size="16" />同时只允许生产 1 集</div>
        </div>
        <div class="rolling-summary">
          <div><strong>{{ rollingArcs[0]?.title }}</strong><span>故事弧章节 {{ rollingArcs[0]?.first_chapter_ordinal || '—' }}–{{ rollingArcs[0]?.last_chapter_ordinal || '—' }}</span></div>
          <div><b>{{ episodeRuns.filter(item => item.status === 'completed').length }}</b><span>/ {{ episodeRuns.length }} 集已完成</span></div>
        </div>
        <div class="episode-run-list">
          <article v-for="run in episodeRuns" :key="run.episode_run_id" class="episode-run-card" :class="{ current: currentEpisodeRun?.episode_run_id === run.episode_run_id, completed: run.status === 'completed' }">
            <div class="episode-run-number">{{ run.status === 'completed' ? '✓' : run.episode_number }}</div>
            <div class="episode-run-copy">
              <div><strong>第 {{ run.episode_number }} 集 · {{ run.title }}</strong><StatusBadge :status="episodeRunDisplayStatus(run)" /></div>
              <p>当前节点：{{ getPipelineStageLabel(run.current_stage, run.status === 'completed' ? 'completed' : '') }}</p>
              <small><Gauge :size="13" />视频每批最多 {{ run.max_video_batch }} 个镜头 <Coins :size="13" />Token {{ Number(run.token_spent || 0).toLocaleString('zh-CN') }}<template v-if="run.token_budget"> / {{ Number(run.token_budget).toLocaleString('zh-CN') }}</template> · 费用 ¥{{ Number(run.cost_spent || 0).toFixed(2) }}<template v-if="run.cost_budget"> / ¥{{ Number(run.cost_budget).toFixed(2) }}</template></small>
            </div>
            <div class="episode-run-actions">
              <RouterLink class="button button-secondary episode-content-button" :to="`/projects/${project.project_id}/episodes/${run.episode_id}/workbench`"><Clapperboard :size="15" />创作工作台</RouterLink>
              <button class="button button-secondary episode-content-button" @click="openEpisodeContent(run)"><Eye :size="15" />查看内容</button>
              <button v-if="run.status !== 'completed' && run.status !== 'cancelled'" class="button button-primary" :disabled="!canStartEpisode(run)" @click="runFlowAction('resume', '', run.episode_run_id)">
                <LoaderCircle v-if="runningEpisodeId === run.episode_run_id" :size="16" class="spin" /><Play v-else :size="16" />{{ episodeActionLabel(run) }}
              </button>
              <span v-else class="episode-run-done">本集已完成</span>
            </div>
          </article>
        </div>
        <div class="rolling-next-hint">下一集只有在上一集完成质检与发布后才会解锁。你可以先检查本集剧本、分镜和样片，再决定是否继续。</div>
      </article>

      <div class="detail-grid">
        <div class="main-column">
          <article class="panel padded">
            <div class="section-title pipeline-title"><div><span>PRODUCTION PIPELINE</span><h3>生产流程</h3></div><div class="pipeline-current"><span>当前：{{ getPipelineStageLabel(project.current_stage, project.status) }}</span><strong>{{ productionProgress.percentage }}%</strong></div></div>
            <div class="pipeline-progress-summary">
              <div><span>总体生产进度</span><strong>已完成 {{ productionProgress.completedStages }} / {{ productionProgress.totalStages }} 个阶段 · 还剩 {{ productionProgress.remainingStages }} 个</strong></div>
              <div class="pipeline-progress-track" role="progressbar" aria-label="总体生产进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="productionProgress.percentage"><i :style="{ width: `${productionProgress.percentage}%` }"></i></div>
              <small>下一个待完成阶段：{{ productionProgress.nextPendingStageLabel }}</small>
              <div v-if="stageUnitProgress" class="stage-unit-progress">
                <div><span>{{ productionProgress.currentStageLabel }}明细</span><strong>已完成 {{ stageUnitProgress.completed }} / {{ stageUnitProgress.total }} {{ stageUnitProgress.unit }}，还剩 {{ stageUnitProgress.remaining }} {{ stageUnitProgress.unit }}</strong></div>
                <div class="pipeline-progress-track" role="progressbar" :aria-label="`${productionProgress.currentStageLabel}进度`" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="stageUnitProgress.percentage"><i :style="{ width: `${stageUnitProgress.percentage}%` }"></i></div>
              </div>
            </div>
            <div class="pipeline">
              <div v-for="(stage, index) in stages" :key="stage[0]" class="pipeline-step" :class="{ done: index < productionProgress.completedStages, current: index === currentIndex && index >= productionProgress.completedStages }">
                <i>{{ index < productionProgress.completedStages ? '✓' : index + 1 }}</i><span>{{ stage[1] }}</span><small>{{ stepStatusLabel(index) }}</small>
              </div>
            </div>
          </article>

          <article class="panel padded">
            <div class="section-title"><div><span>CONTENT INVENTORY</span><h3>内容资产</h3></div></div>
            <div class="asset-grid">
              <div><BookOpen :size="20" /><span>原文章节</span><strong>{{ project.counts.chapters }}</strong></div>
              <div><Layers3 :size="20" /><span>文本拆解（完成 / 全部）</span><strong>{{ project.counts.completed_chunks }} / {{ project.counts.chunks }}</strong></div>
              <div><Clapperboard :size="20" /><span>剧集 / 场景</span><strong>{{ project.counts.episodes }} / {{ project.counts.scenes }}</strong></div>
              <div><ListChecks :size="20" /><span>分镜镜头</span><strong>{{ project.counts.shots }}</strong></div>
              <div><Image :size="20" /><span>分镜图片</span><strong>{{ project.counts.generated_images }}</strong></div>
              <div><Video :size="20" /><span>镜头视频</span><strong>{{ project.counts.generated_videos }}</strong></div>
            </div>
          </article>
        </div>

        <aside class="side-column">
          <article class="panel padded metadata-card">
            <div class="section-title"><div><span>PROJECT PROFILE</span><h3>项目参数</h3></div></div>
            <dl><div><dt>视觉风格</dt><dd>{{ project.visual_style }}</dd></div><div><dt>画面比例</dt><dd>{{ project.aspect_ratio }}</dd></div><div><dt>目标平台</dt><dd>{{ project.target_platform }}</dd></div><div><dt>单集时长</dt><dd>{{ project.episode_duration_seconds }} 秒</dd></div><div><dt>目标集数</dt><dd>{{ project.target_episode_count }} 集</dd></div><div><dt>运行模式</dt><dd>{{ project.test_mode ? '测试模式' : '正式模式' }}</dd></div></dl>
          </article>
          <article class="attention-card"><span>待办概览</span><strong>{{ project.counts.pending_reviews }}</strong><p>项内容等待人工审核</p><div><b>{{ project.counts.completed_tasks }}</b> 个工作流任务已完成</div></article>
        </aside>
      </div>

      <article class="panel production-data-panel">
        <div class="production-data-head">
          <div><span>READ-ONLY DATABASE VIEW</span><h3>项目生产数据</h3></div>
          <p>数据直接读取自 <code>drama</code> schema</p>
        </div>
        <div class="data-tabs">
          <button v-for="tab in productionTabs" :key="tab.key" :class="{ active: activeDataTab === tab.key }" @click="activeDataTab = tab.key">
            <component :is="tab.icon" :size="15" />{{ tab.label }}<i>{{ tab.items?.length || 0 }}</i>
          </button>
        </div>
        <DetailDataTable v-if="activeTab" :items="activeTab.items || []" :columns="activeTab.columns" @row-action="handleTableAction" />
      </article>

      <EpisodeContentModal
        v-if="selectedEpisodeRun"
        :project-id="project.project_id"
        :episode-run="selectedEpisodeRun"
        @close="selectedEpisodeRun = null"
        @saved="handleEpisodeContentSaved"
      />
    </template>
  </section>
</template>
