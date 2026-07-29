<script setup>
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { ClipboardCheck, Clock3, CircleCheckBig, CircleX, Eye, Filter, RefreshCw, X, Webhook, ExternalLink, LoaderCircle, MessageSquareText, Search, ChevronLeft, ChevronRight, Layers3, GitCompareArrows } from 'lucide-vue-next'
import { api } from '../services/api'
import StatusBadge from '../components/StatusBadge.vue'
import EmptyState from '../components/EmptyState.vue'
import ReviewContentViewer from '../components/ReviewContentViewer.vue'
import { getDisplayValueLabel } from '../services/displayLabels'
import { getRegeneratedSuccessor, getRegenerationSourceLabel, getVisualRegenerationAction, isRegeneratedVisualReview, isVisualAssetReview, regenerationNeedsPrompt } from '../services/reviewRegeneration'
import { REVIEW_PAGE_SIZE, getAdjacentReview, getReviewTabCount, getReviewTaskTitle, groupReviewItems, reviewStatusTabs } from '../services/reviewWorkspace'
import { localEditLinkForReview } from '../services/localEditing'

const data = ref(null)
const overview = ref(null)
const loading = ref(true)
const error = ref('')
const result = ref(null)
const filters = reactive({ project_id: '', stage: '', status: 'pending' })
const searchQuery = ref('')
const appliedQuery = ref('')
const page = ref(1)
let searchTimer
let loadSequence = 0
const decision = reactive({
  open: false, item: null, operation: 'decision', regeneration_mode: 'replace',
  review_status: 'approved', review_comment: '', rejection_reason: '',
  revision_instruction: '', prompt_adjustment: '', selected_as_primary: true, lock_after_approval: true,
  provider_voice_id: '', next_after_submit: false,
})
const submitting = ref(false)
const preview = reactive({ open: false, item: null, content: null, loading: false, error: '' })

const items = computed(() => data.value?.items || [])
const summary = computed(() => overview.value?.summary || { total: 0, pending: 0, approved: 0, rejected: 0 })
const facets = computed(() => overview.value?.facets || data.value?.facets || { projects: [], stages: [] })
const groupedItems = computed(() => groupReviewItems(items.value))
const pageCount = computed(() => Math.max(1, Math.ceil((data.value?.total || 0) / REVIEW_PAGE_SIZE)))
const currentPreviewIndex = computed(() => items.value.findIndex((item) => item.review_id === preview.item?.review_id))
const previewPosition = computed(() => currentPreviewIndex.value < 0 ? '' : `${currentPreviewIndex.value + 1} / ${items.value.length}`)
const stageLabels = {
  story_bible: '故事圣经', season_outline: '分集大纲', episode_script: '单集剧本', storyboard: '分镜设计',
  visual_asset: '视觉资产', storyboard_image: '分镜图片', shot_video: '镜头视频', dialogue_audio: '对白音频',
  voice_profile: '声音档案', final_review: '成片终审', publication_metadata: '发布信息', final: '成片终审', publication: '发布审核',
}

async function load() {
  const sequence = ++loadSequence
  loading.value = true
  error.value = ''
  try {
    const [nextData, nextOverview] = await Promise.all([
      api.getReviews({ ...filters, q: appliedQuery.value, page: page.value, limit: REVIEW_PAGE_SIZE }),
      api.getReviews({ project_id: filters.project_id, stage: filters.stage, limit: 1 }),
    ])
    if (sequence !== loadSequence) return
    data.value = nextData
    overview.value = nextOverview
  }
  catch (err) { error.value = err.message }
  finally { if (sequence === loadSequence) loading.value = false }
}
watch(() => [filters.project_id, filters.stage, filters.status, appliedQuery.value, page.value], load)
watch(searchQuery, (value) => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    appliedQuery.value = value.trim()
    page.value = 1
  }, 250)
})
onMounted(load)
onUnmounted(() => clearTimeout(searchTimer))

function selectStatus(status) {
  filters.status = status
  page.value = 1
}

function resetFilters() {
  filters.project_id = ''
  filters.stage = ''
  filters.status = 'pending'
  searchQuery.value = ''
  appliedQuery.value = ''
  page.value = 1
}

function webhookStage(item) {
  if (['story_bible', 'season_outline', 'episode_script', 'storyboard'].includes(item.stage)) return '剧本与分镜阶段'
  if (['visual_asset', 'storyboard_image'].includes(item.stage)) return '图片阶段'
  if (['shot_video', 'dialogue_audio', 'voice_profile', 'video', 'audio'].includes(item.stage)) return '视频与音频阶段'
  return '剪辑与发布阶段'
}

function openDecision(item, status, nextAfterSubmit = false) {
  Object.assign(decision, {
    open: true, item, operation: 'decision', regeneration_mode: 'replace',
    review_status: status, review_comment: '', rejection_reason: '', revision_instruction: '',
    prompt_adjustment: '', selected_as_primary: true, lock_after_approval: true,
    provider_voice_id: '', next_after_submit: nextAfterSubmit,
  })
}

function openRegeneration(item) {
  const action = getVisualRegenerationAction(item)
  if (!action) return
  Object.assign(decision, {
    open: true, item, operation: action.operation, regeneration_mode: action.mode,
    review_status: action.operation === 'reject_regenerate' ? 'rejected' : item.review_status,
    review_comment: '', rejection_reason: item.rejection_reason || '', revision_instruction: '',
    prompt_adjustment: '', selected_as_primary: false, lock_after_approval: false, provider_voice_id: '',
    next_after_submit: false,
  })
}

async function openPreview(item) {
  Object.assign(preview, { open: true, item, content: null, loading: true, error: '' })
  try { preview.content = await api.getReviewContent(item.review_id) }
  catch (err) { preview.error = err.message }
  finally { preview.loading = false }
}

function openSuccessor(item) {
  const successor = getRegeneratedSuccessor(item, items.value)
  if (successor) openPreview(successor)
}

function closePreview() {
  if (!submitting.value) Object.assign(preview, { open: false, item: null, content: null, loading: false, error: '' })
}

function decideFromPreview(status, nextAfterSubmit = false) {
  if (!preview.item) return
  openDecision(preview.item, status, nextAfterSubmit)
}

function openAdjacentPreview(direction) {
  const adjacent = getAdjacentReview(items.value, preview.item?.review_id, direction)
  if (adjacent) openPreview(adjacent)
}

function closeDecision() {
  if (!submitting.value) decision.open = false
}

async function submitDecision() {
  if (!decision.item || submitting.value) return
  if (decision.operation === 'decision' && decision.review_status === 'rejected' && !decision.rejection_reason.trim()) return
  if (decision.operation === 'reject_regenerate' && !decision.rejection_reason.trim()) return
  if (isRegeneration.value && regenerationNeedsPrompt(decision.regeneration_mode) && !decision.prompt_adjustment.trim() && !decision.revision_instruction.trim()) return
  if (decision.operation === 'decision' && isVoiceProfile.value && decision.review_status === 'approved' && !decision.provider_voice_id.trim()) return
  submitting.value = true
  error.value = ''
  const nextItem = decision.next_after_submit
    ? getAdjacentReview(items.value, decision.item.review_id, 1) || getAdjacentReview(items.value, decision.item.review_id, -1)
    : null
  try {
    const reviewPayload = {
      review_status: decision.review_status,
      review_comment: decision.review_comment.trim(),
      rejection_reason: decision.rejection_reason.trim(),
      revision_instruction: decision.revision_instruction.trim(),
      prompt_adjustment: decision.prompt_adjustment.trim(),
      provider_voice_id: decision.provider_voice_id.trim(),
      selected_as_primary: decision.selected_as_primary,
      lock_after_approval: decision.lock_after_approval,
    }
    let response
    if (decision.operation === 'reject_regenerate') {
      await api.decideReview(decision.item.review_id, { ...reviewPayload, review_status: 'rejected' })
      response = await api.regenerateReview(decision.item.review_id, {
        mode: 'replace',
        review_comment: reviewPayload.review_comment,
        rejection_reason: reviewPayload.rejection_reason,
        revision_instruction: reviewPayload.revision_instruction,
        prompt_adjustment: reviewPayload.prompt_adjustment,
      })
    } else if (decision.operation === 'regenerate') {
      response = await api.regenerateReview(decision.item.review_id, {
        mode: decision.regeneration_mode,
        review_comment: reviewPayload.review_comment,
        rejection_reason: reviewPayload.rejection_reason,
        revision_instruction: reviewPayload.revision_instruction,
        prompt_adjustment: reviewPayload.prompt_adjustment,
      })
    } else {
      response = await api.decideReview(decision.item.review_id, reviewPayload)
    }
    result.value = response
    decision.open = false
    if (!nextItem) Object.assign(preview, { open: false, item: null, content: null, loading: false, error: '' })
    await load()
    if (nextItem) await openPreview(nextItem)
  } catch (err) {
    error.value = err.message
  } finally {
    submitting.value = false
  }
}

const isVisualAsset = computed(() => decision.item?.stage === 'visual_asset')
const isVoiceProfile = computed(() => decision.item?.stage === 'voice_profile')
const isRegeneration = computed(() => decision.operation === 'regenerate' || decision.operation === 'reject_regenerate')
const modalTitle = computed(() => {
  if (decision.operation === 'reject_regenerate') return '退回并重新生成'
  if (decision.operation === 'regenerate') return decision.regeneration_mode === 'variant' ? '生成新变体' : '按意见重新生成'
  return decision.review_status === 'approved' ? '通过审核' : '拒绝审核'
})
const submitLabel = computed(() => {
  if (submitting.value && isRegeneration.value) return '图片生成中（预计 1–3 分钟）'
  if (decision.operation === 'decision' && decision.review_status === 'approved' && decision.next_after_submit) return '确认通过并下一条'
  return isRegeneration.value ? '确认重新生成' : `确认${decision.review_status === 'approved' ? '通过' : '拒绝'}`
})
const submitDisabled = computed(() => submitting.value
  || ((decision.operation === 'decision' && decision.review_status === 'rejected') || decision.operation === 'reject_regenerate') && !decision.rejection_reason.trim()
  || (isRegeneration.value && regenerationNeedsPrompt(decision.regeneration_mode) && !decision.prompt_adjustment.trim() && !decision.revision_instruction.trim())
  || (decision.operation === 'decision' && isVoiceProfile.value && decision.review_status === 'approved' && !decision.provider_voice_id.trim()))
const supportsPromptAdjustment = computed(() => ['visual_asset', 'storyboard_image', 'shot_video', 'dialogue_audio'].includes(decision.item?.stage))
const formatTime = (value) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'
const statusTabCount = (status) => getReviewTabCount(status, summary.value)
const taskTitle = (item) => getReviewTaskTitle(item, stageLabels[item.stage] || '审核')
</script>

<template>
  <section class="view-stack">
    <div class="hero-row"><div><h2>内容审核中心</h2><p>优先处理当前待办，按项目和生产阶段连续完成审核。</p></div><button class="button button-secondary" :disabled="loading" @click="load"><RefreshCw :size="16" :class="{ spin: loading }" />刷新任务</button></div>

    <div class="metric-grid review-metrics">
      <button class="metric-card review-metric-action primary" type="button" @click="selectStatus('pending')"><div class="metric-icon amber"><Clock3 :size="20" /></div><div><span>待我审核</span><strong>{{ summary.pending }}</strong><small>需要立即处理</small></div></button>
      <button class="metric-card review-metric-action" type="button" @click="selectStatus('processed')"><div class="metric-icon green"><CircleCheckBig :size="20" /></div><div><span>已处理</span><strong>{{ summary.approved + summary.rejected }}</strong><small>通过 {{ summary.approved }} · 拒绝 {{ summary.rejected }}</small></div></button>
      <button class="metric-card review-metric-action" type="button" @click="selectStatus('')"><div class="metric-icon blue"><ClipboardCheck :size="20" /></div><div><span>全部记录</span><strong>{{ summary.total }}</strong><small>当前项目与阶段</small></div></button>
    </div>

    <div v-if="result" class="review-result-banner"><Webhook :size="19" /><div><strong>n8n 审核请求已返回</strong><span>{{ result.review_id }} · {{ result.webhook_stage }}</span></div><code>{{ JSON.stringify(result.n8n_response) }}</code><button aria-label="关闭返回结果" @click="result = null"><X :size="16" /></button></div>

    <article class="panel review-panel">
      <div class="review-status-tabs" role="tablist" aria-label="审核任务状态">
        <button v-for="tab in reviewStatusTabs" :key="tab.value || 'all'" type="button" role="tab" :aria-selected="filters.status === tab.value" :class="{ active: filters.status === tab.value }" @click="selectStatus(tab.value)">
          {{ tab.label }}<span>{{ statusTabCount(tab.value) }}</span>
        </button>
      </div>
      <div class="review-filterbar">
        <label class="review-search"><Search :size="15" /><input v-model="searchQuery" type="search" placeholder="搜索项目或任务编号" aria-label="搜索审核任务" /></label>
        <div class="filter-title"><Filter :size="16" />筛选</div>
        <select v-model="filters.project_id" class="select-control review-select" @change="page = 1"><option value="">全部项目</option><option v-for="project in facets.projects || []" :key="project.project_id" :value="project.project_id">{{ project.novel_name }}</option></select>
        <select v-model="filters.stage" class="select-control review-select" @change="page = 1"><option value="">全部阶段</option><option v-for="stage in facets.stages || []" :key="stage" :value="stage">{{ stageLabels[stage] || stage }}</option></select>
        <button class="clear-filters" @click="resetFilters">清除筛选</button><span class="result-count">{{ data?.total || 0 }} 条记录</span>
      </div>
      <div v-if="error" class="error-banner">{{ error }} <button @click="load">重新读取</button></div>
      <div v-if="loading" class="table-loading"><span v-for="i in 5" :key="i"></span></div>
      <EmptyState v-else-if="items.length === 0" title="没有匹配的审核任务" description="请调整项目、阶段或搜索条件。" />
      <div v-else class="review-groups">
        <section v-for="group in groupedItems" :key="group.key" class="review-group">
          <header class="review-group-head">
            <div class="review-stage-icon"><Layers3 :size="18" /></div>
            <div><span>{{ stageLabels[group.stage] || '其他审核阶段' }}</span><h3>{{ group.projectName }}</h3></div>
            <RouterLink :to="`/projects/${group.projectId}`"><ExternalLink :size="13" />查看项目</RouterLink>
            <strong>{{ group.pendingCount ? `${group.pendingCount} 条待审核` : `${group.items.length} 条记录` }}</strong>
          </header>
          <div class="review-list">
            <article v-for="item in group.items" :key="item.review_id" class="review-row" :class="{ pending: item.review_status === 'pending' }">
              <div class="review-main">
                <div class="review-title"><strong>{{ taskTitle(item) }}</strong><StatusBadge :status="item.review_status" /><span v-if="isRegeneratedVisualReview(item)">已重新生成</span></div>
                <p>{{ getDisplayValueLabel(item.entity_type) }} · {{ webhookStage(item) }}</p>
                <div v-if="getRegenerationSourceLabel(item)" class="review-lineage"><RefreshCw :size="13" /><span>{{ getRegenerationSourceLabel(item) }}</span></div>
                <details class="review-technical-id"><summary>技术编号</summary><code>{{ item.entity_id }}</code></details>
              </div>
              <div class="review-history"><span>{{ item.review_status === 'pending' ? '等待自' : '创建于' }}</span><strong>{{ formatTime(item.created_at) }}</strong><small v-if="item.reviewed_at">审核于 {{ formatTime(item.reviewed_at) }}</small></div>
              <div class="review-actions"><button class="review-open-button" @click="openPreview(item)"><Eye :size="15" />{{ item.review_status === 'pending' ? '开始审核' : '查看记录' }}</button><button v-if="isRegeneratedVisualReview(item)" class="review-successor-button" @click="openSuccessor(item)"><RefreshCw :size="15" />查看新版本</button></div>
            </article>
          </div>
        </section>
        <footer v-if="pageCount > 1" class="review-pagination">
          <span>第 {{ page }} / {{ pageCount }} 页</span>
          <button type="button" :disabled="page <= 1 || loading" aria-label="上一页" @click="page -= 1"><ChevronLeft :size="17" />上一页</button>
          <button type="button" :disabled="page >= pageCount || loading" aria-label="下一页" @click="page += 1">下一页<ChevronRight :size="17" /></button>
        </footer>
      </div>
    </article>

    <div v-if="preview.open" class="review-drawer-backdrop" @click.self="closePreview">
      <aside class="review-drawer" role="dialog" aria-modal="true" aria-label="审核内容详情">
        <header class="review-drawer-head">
          <div><span>CONTENT REVIEW</span><h2>{{ stageLabels[preview.item?.stage] || preview.item?.stage }}</h2><p>{{ preview.item?.novel_name }} · {{ preview.item?.entity_id }}</p></div>
          <button aria-label="关闭审核内容" @click="closePreview"><X :size="20" /></button>
        </header>
        <div class="review-drawer-body">
          <div v-if="preview.loading" class="review-content-loading"><LoaderCircle :size="24" class="spin" /><strong>正在读取生成内容…</strong><span>系统正在按审核对象加载实际产物，而不是任务 ID。</span></div>
          <div v-else-if="preview.error" class="review-content-error"><CircleX :size="22" /><strong>内容读取失败</strong><span>{{ preview.error }}</span><button class="button button-secondary" @click="openPreview(preview.item)"><RefreshCw :size="15" />重新读取</button></div>
          <ReviewContentViewer v-else-if="preview.content" :content="preview.content" />
        </div>
        <footer class="review-drawer-actions">
          <span v-if="isRegeneratedVisualReview(preview.item)">已重新生成后继版本。</span>
          <span v-else-if="preview.item?.review_status !== 'pending'">该任务已经完成审核。</span>
          <span v-else>{{ previewPosition }} · 完成后可直接进入下一条</span>
          <button class="button button-secondary review-nav-button" :disabled="preview.loading || currentPreviewIndex <= 0" @click="openAdjacentPreview(-1)"><ChevronLeft :size="16" />上一条</button>
          <button class="button button-secondary review-nav-button" :disabled="preview.loading || currentPreviewIndex < 0 || currentPreviewIndex >= items.length - 1" @click="openAdjacentPreview(1)">下一条<ChevronRight :size="16" /></button>
          <RouterLink v-if="preview.item" class="button button-secondary" :to="localEditLinkForReview(preview.item)"><GitCompareArrows :size="16" />局部修改</RouterLink>
          <button v-if="preview.item?.review_status === 'pending'" class="button button-danger" :disabled="preview.loading || !!preview.error" @click="decideFromPreview('rejected')"><CircleX :size="16" />拒绝</button>
          <button v-if="getVisualRegenerationAction(preview.item)" class="button button-secondary" :disabled="preview.loading || !!preview.error" @click="openRegeneration(preview.item)"><RefreshCw :size="16" />{{ getVisualRegenerationAction(preview.item).label }}</button>
          <button v-if="isRegeneratedVisualReview(preview.item)" class="button button-primary" :disabled="preview.loading" @click="openSuccessor(preview.item)"><RefreshCw :size="16" />查看新版本</button>
          <button v-if="preview.item?.review_status === 'pending'" class="button button-primary" :disabled="preview.loading || !!preview.error" @click="decideFromPreview('approved', true)"><CircleCheckBig :size="16" />通过并下一条</button>
        </footer>
      </aside>
    </div>

    <div v-if="decision.open" class="modal-backdrop" @click.self="closeDecision">
      <div class="review-modal" role="dialog" aria-modal="true" :aria-label="modalTitle">
        <div class="modal-head"><div><span>{{ decision.item ? webhookStage(decision.item) : '' }}{{ isRegeneration ? '生成' : '审核' }}</span><h3>{{ modalTitle }}</h3></div><button aria-label="关闭审核窗口" @click="closeDecision"><X :size="18" /></button></div>
        <div class="decision-target"><strong>{{ stageLabels[decision.item?.stage] || decision.item?.stage }}</strong><code>{{ decision.item?.review_id }}</code><span>{{ decision.item?.entity_id }}</span></div>
        <label class="field"><span>审核意见</span><textarea v-model="decision.review_comment" rows="3" placeholder="可选：记录本次审核意见"></textarea></label>
        <label v-if="(decision.operation === 'decision' && decision.review_status === 'rejected') || decision.operation === 'reject_regenerate' || (isRegeneration && decision.regeneration_mode === 'replace')" class="field"><span>拒绝原因 <i v-if="decision.operation === 'reject_regenerate' || decision.operation === 'decision'">*</i></span><textarea v-model="decision.rejection_reason" rows="3" placeholder="请说明需要修改的问题"></textarea></label>
        <label v-if="decision.review_status === 'rejected' || isRegeneration" class="field"><span>修改指令</span><textarea v-model="decision.revision_instruction" rows="2" placeholder="可选：给重新生成流程的具体指令"></textarea></label>
        <label v-if="supportsPromptAdjustment && (decision.review_status === 'rejected' || isRegeneration)" class="field"><span>Prompt 调整 <i v-if="isRegeneration && decision.regeneration_mode === 'variant'">*</i></span><textarea v-model="decision.prompt_adjustment" rows="2" placeholder="描述新图片需要保持和改变的内容"></textarea></label>
        <label v-if="decision.operation === 'decision' && isVoiceProfile && decision.review_status === 'approved'" class="field"><span>供应商音色 ID <i>*</i></span><input v-model="decision.provider_voice_id" type="text" placeholder="例如：Kore、Aoede、Puck" required /></label>
        <div v-if="decision.operation === 'decision' && (isVisualAsset || isVoiceProfile) && decision.review_status === 'approved'" class="decision-options"><label v-if="isVisualAsset"><input v-model="decision.selected_as_primary" type="checkbox" />设为主资产</label><label><input v-model="decision.lock_after_approval" type="checkbox" />批准后锁定</label></div>
        <div v-if="isRegeneration && decision.regeneration_mode === 'variant'" class="modal-notice regeneration"><RefreshCw :size="16" /><span>当前已通过的主图会继续生效；新变体生成后仍需单独审核，不会自动重做已有分镜或视频。</span></div>
        <div v-else class="modal-notice"><Webhook :size="16" /><span>{{ submitting && isRegeneration ? '图片模型正在生成并保存新版本，请保持页面打开；完成或失败后会自动给出结果。' : isRegeneration ? '系统会创建新版本和新的待审核记录，保留当前图片作为历史。' : '此操作将调用 n8n，不会由 CMS 直接更新 review_tasks。' }}</span></div>
        <div class="modal-actions"><button class="button button-secondary" :disabled="submitting" @click="closeDecision">取消</button><button class="button" :class="decision.operation === 'decision' && decision.review_status === 'rejected' ? 'button-danger' : 'button-primary'" :disabled="submitDisabled" @click="submitDecision"><LoaderCircle v-if="submitting" :size="16" class="spin" /><MessageSquareText v-else :size="16" />{{ submitLabel }}</button></div>
      </div>
    </div>
  </section>
</template>
