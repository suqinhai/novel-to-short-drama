<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ClipboardCheck, Clock3, CircleCheckBig, CircleX, Eye, Filter, RefreshCw, X, Webhook, ExternalLink, LoaderCircle, MessageSquareText } from 'lucide-vue-next'
import { api } from '../services/api'
import StatusBadge from '../components/StatusBadge.vue'
import EmptyState from '../components/EmptyState.vue'
import ReviewContentViewer from '../components/ReviewContentViewer.vue'

const data = ref(null)
const loading = ref(true)
const error = ref('')
const result = ref(null)
const filters = reactive({ project_id: '', stage: '', status: '' })
const decision = reactive({
  open: false, item: null, review_status: 'approved', review_comment: '', rejection_reason: '',
  revision_instruction: '', prompt_adjustment: '', selected_as_primary: true, lock_after_approval: true,
  provider_voice_id: '',
})
const submitting = ref(false)
const preview = reactive({ open: false, item: null, content: null, loading: false, error: '' })

const items = computed(() => data.value?.items || [])
const summary = computed(() => data.value?.summary || { total: 0, pending: 0, approved: 0, rejected: 0 })
const stageLabels = {
  story_bible: '故事圣经', season_outline: '分集大纲', episode_script: '单集剧本', storyboard: '分镜设计',
  visual_asset: '视觉资产', storyboard_image: '分镜图片', shot_video: '镜头视频', dialogue_audio: '对白音频',
  voice_profile: '声音档案', final_review: '成片终审', publication_metadata: '发布信息', final: '成片终审', publication: '发布审核',
}

async function load() {
  loading.value = true
  error.value = ''
  try { data.value = await api.getReviews({ ...filters, limit: 100 }) }
  catch (err) { error.value = err.message }
  finally { loading.value = false }
}
watch(() => [filters.project_id, filters.stage, filters.status], load)
onMounted(load)

function resetFilters() {
  filters.project_id = ''
  filters.stage = ''
  filters.status = ''
}

function webhookStage(item) {
  if (['story_bible', 'season_outline', 'episode_script', 'storyboard'].includes(item.stage)) return 'stage2'
  if (['visual_asset', 'storyboard_image'].includes(item.stage)) return 'stage3'
  if (['shot_video', 'dialogue_audio', 'voice_profile', 'video', 'audio'].includes(item.stage)) return 'stage4'
  return 'stage5'
}

function openDecision(item, status) {
  Object.assign(decision, {
    open: true, item, review_status: status, review_comment: '', rejection_reason: '', revision_instruction: '',
    prompt_adjustment: '', selected_as_primary: true, lock_after_approval: true,
    provider_voice_id: '',
  })
}

async function openPreview(item) {
  Object.assign(preview, { open: true, item, content: null, loading: true, error: '' })
  try { preview.content = await api.getReviewContent(item.review_id) }
  catch (err) { preview.error = err.message }
  finally { preview.loading = false }
}

function closePreview() {
  if (!submitting.value) Object.assign(preview, { open: false, item: null, content: null, loading: false, error: '' })
}

function decideFromPreview(status) {
  if (!preview.item) return
  openDecision(preview.item, status)
}

function closeDecision() {
  if (!submitting.value) decision.open = false
}

async function submitDecision() {
  if (!decision.item || submitting.value) return
  if (decision.review_status === 'rejected' && !decision.rejection_reason.trim()) return
  if (isVoiceProfile.value && decision.review_status === 'approved' && !decision.provider_voice_id.trim()) return
  submitting.value = true
  error.value = ''
  try {
    const response = await api.decideReview(decision.item.review_id, {
      review_status: decision.review_status,
      review_comment: decision.review_comment.trim(),
      rejection_reason: decision.rejection_reason.trim(),
      revision_instruction: decision.revision_instruction.trim(),
      prompt_adjustment: decision.prompt_adjustment.trim(),
      provider_voice_id: decision.provider_voice_id.trim(),
      selected_as_primary: decision.selected_as_primary,
      lock_after_approval: decision.lock_after_approval,
    })
    result.value = response
    decision.open = false
    Object.assign(preview, { open: false, item: null, content: null, loading: false, error: '' })
    await load()
  } catch (err) {
    error.value = err.message
  } finally {
    submitting.value = false
  }
}

const isVisualAsset = computed(() => decision.item?.stage === 'visual_asset')
const isVoiceProfile = computed(() => decision.item?.stage === 'voice_profile')
const supportsPromptAdjustment = computed(() => ['visual_asset', 'storyboard_image', 'shot_video', 'dialogue_audio'].includes(decision.item?.stage))
const formatTime = (value) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'
</script>

<template>
  <section class="view-stack">
    <div class="hero-row"><div><h2>内容审核中心</h2><p>集中处理剧本、视觉、音视频和发布环节的人工审核任务。</p></div><button class="button button-secondary" :disabled="loading" @click="load"><RefreshCw :size="16" :class="{ spin: loading }" />刷新任务</button></div>

    <div class="metric-grid review-metrics">
      <article class="metric-card"><div class="metric-icon blue"><ClipboardCheck :size="20" /></div><div><span>全部任务</span><strong>{{ summary.total }}</strong><small>当前筛选范围</small></div></article>
      <article class="metric-card"><div class="metric-icon amber"><Clock3 :size="20" /></div><div><span>待审核</span><strong>{{ summary.pending }}</strong><small>等待人工决策</small></div></article>
      <article class="metric-card"><div class="metric-icon green"><CircleCheckBig :size="20" /></div><div><span>已通过</span><strong>{{ summary.approved }}</strong><small>approved</small></div></article>
      <article class="metric-card"><div class="metric-icon red"><CircleX :size="20" /></div><div><span>已拒绝</span><strong>{{ summary.rejected }}</strong><small>rejected</small></div></article>
    </div>

    <div v-if="result" class="review-result-banner"><Webhook :size="19" /><div><strong>n8n 审核请求已返回</strong><span>{{ result.review_id }} · {{ result.webhook_stage }}</span></div><code>{{ JSON.stringify(result.n8n_response) }}</code><button aria-label="关闭返回结果" @click="result = null"><X :size="16" /></button></div>

    <article class="panel review-panel">
      <div class="review-filterbar">
        <div class="filter-title"><Filter :size="16" />筛选</div>
        <select v-model="filters.project_id" class="select-control review-select"><option value="">全部项目</option><option v-for="project in data?.facets?.projects || []" :key="project.project_id" :value="project.project_id">{{ project.novel_name }} · {{ project.project_id }}</option></select>
        <select v-model="filters.stage" class="select-control review-select"><option value="">全部阶段</option><option v-for="stage in data?.facets?.stages || []" :key="stage" :value="stage">{{ stageLabels[stage] || stage }}</option></select>
        <select v-model="filters.status" class="select-control review-select"><option value="">全部状态</option><option value="pending">待审核</option><option value="approved">已通过</option><option value="rejected">已拒绝</option><option value="cancelled">已取消</option></select>
        <button class="clear-filters" @click="resetFilters">清除筛选</button><span class="result-count">{{ data?.total || 0 }} 条记录</span>
      </div>
      <div v-if="error" class="error-banner">{{ error }} <button @click="load">重新读取</button></div>
      <div v-if="loading" class="table-loading"><span v-for="i in 5" :key="i"></span></div>
      <EmptyState v-else-if="items.length === 0" title="没有匹配的审核任务" description="请调整项目、阶段或状态筛选条件。" />
      <div v-else class="review-list">
        <article v-for="item in items" :key="item.review_id" class="review-row">
          <div class="review-stage-icon"><ClipboardCheck :size="18" /></div>
          <div class="review-main"><div class="review-title"><strong>{{ stageLabels[item.stage] || item.stage }}</strong><StatusBadge :status="item.review_status" /><span>{{ webhookStage(item) }}</span></div><p>{{ item.novel_name }} <RouterLink :to="`/projects/${item.project_id}`"><ExternalLink :size="11" />{{ item.project_id }}</RouterLink></p><div class="review-entity"><code>{{ item.entity_type }}</code><span>{{ item.entity_id }}</span></div></div>
          <div class="review-history"><span>创建时间</span><strong>{{ formatTime(item.created_at) }}</strong><small v-if="item.reviewed_at">审核于 {{ formatTime(item.reviewed_at) }}</small></div>
          <div class="review-actions"><button class="review-open-button" @click="openPreview(item)"><Eye :size="15" />查看内容</button></div>
        </article>
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
          <span v-if="preview.item?.review_status !== 'pending'">该任务已经完成审核，当前为只读查看。</span>
          <template v-else>
            <button class="button button-danger" :disabled="preview.loading || !!preview.error" @click="decideFromPreview('rejected')"><CircleX :size="16" />退回修改</button>
            <button class="button button-primary" :disabled="preview.loading || !!preview.error" @click="decideFromPreview('approved')"><CircleCheckBig :size="16" />通过审核</button>
          </template>
        </footer>
      </aside>
    </div>

    <div v-if="decision.open" class="modal-backdrop" @click.self="closeDecision">
      <div class="review-modal" role="dialog" aria-modal="true" :aria-label="decision.review_status === 'approved' ? '通过审核' : '拒绝审核'">
        <div class="modal-head"><div><span>N8N {{ decision.item ? webhookStage(decision.item) : '' }} REVIEW</span><h3>{{ decision.review_status === 'approved' ? '通过审核' : '拒绝审核' }}</h3></div><button aria-label="关闭审核窗口" @click="closeDecision"><X :size="18" /></button></div>
        <div class="decision-target"><strong>{{ stageLabels[decision.item?.stage] || decision.item?.stage }}</strong><code>{{ decision.item?.review_id }}</code><span>{{ decision.item?.entity_id }}</span></div>
        <label class="field"><span>审核意见</span><textarea v-model="decision.review_comment" rows="3" placeholder="可选：记录本次审核意见"></textarea></label>
        <label v-if="decision.review_status === 'rejected'" class="field"><span>拒绝原因 <i>*</i></span><textarea v-model="decision.rejection_reason" rows="3" placeholder="请说明需要修改的问题" required></textarea></label>
        <label v-if="decision.review_status === 'rejected'" class="field"><span>修改指令</span><textarea v-model="decision.revision_instruction" rows="2" placeholder="可选：给后续重做流程的具体指令"></textarea></label>
        <label v-if="supportsPromptAdjustment" class="field"><span>Prompt 调整</span><textarea v-model="decision.prompt_adjustment" rows="2" placeholder="可选：用于视觉或音视频重新生成"></textarea></label>
        <label v-if="isVoiceProfile && decision.review_status === 'approved'" class="field"><span>供应商音色 ID <i>*</i></span><input v-model="decision.provider_voice_id" type="text" placeholder="例如：Kore、Aoede、Puck" required /></label>
        <div v-if="(isVisualAsset || isVoiceProfile) && decision.review_status === 'approved'" class="decision-options"><label v-if="isVisualAsset"><input v-model="decision.selected_as_primary" type="checkbox" />设为主资产</label><label><input v-model="decision.lock_after_approval" type="checkbox" />批准后锁定</label></div>
        <div class="modal-notice"><Webhook :size="16" /><span>此操作将调用 n8n，不会由 CMS 直接更新 review_tasks。</span></div>
        <div class="modal-actions"><button class="button button-secondary" :disabled="submitting" @click="closeDecision">取消</button><button class="button" :class="decision.review_status === 'approved' ? 'button-primary' : 'button-danger'" :disabled="submitting || (decision.review_status === 'rejected' && !decision.rejection_reason.trim()) || (isVoiceProfile && decision.review_status === 'approved' && !decision.provider_voice_id.trim())" @click="submitDecision"><LoaderCircle v-if="submitting" :size="16" class="spin" /><MessageSquareText v-else :size="16" />确认{{ decision.review_status === 'approved' ? '通过' : '拒绝' }}</button></div>
      </div>
    </div>
  </section>
</template>
