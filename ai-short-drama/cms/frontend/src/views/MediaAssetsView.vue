<script setup>
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import {
  AlertTriangle, ArrowRight, AudioLines, CheckCircle2, ChevronLeft, ChevronRight,
  CircleAlert, CirclePlay, Clock3, ExternalLink, Film, Filter, History, Hourglass,
  ImageOff, Images, Info, LayoutGrid, List, LoaderCircle, RefreshCw, RotateCcw, GitCompare,
  Search, Upload, X,
} from 'lucide-vue-next'
import { api } from '../services/api'
import {
  canRecoverMediaAsset,
  getMediaAssetSourceLabel,
  getMediaAssetSuccessorState,
  hasMediaAssetSuccessor,
  isMediaAssetRecoverable,
} from '../services/mediaAssetVersions'
import EmptyState from '../components/EmptyState.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { localEditLinkForMedia } from '../services/localEditing'

const data = ref(null)
const loading = ref(true)
const error = ref('')
const notice = ref('')
const searchQuery = ref('')
const viewMode = ref('grid')
const filters = reactive({
  project_id: '',
  type: '',
  media_kind: '',
  review_status: '',
  scope: 'current',
  q: '',
  sort: 'latest',
  page: 1,
  limit: 24,
})
const action = reactive({
  open: false, mode: '', item: null, prompt_adjustment: '', file: null,
  metadata: null, working: false, error: '',
})
const fileInput = ref(null)
let requestSequence = 0

const items = computed(() => data.value?.items || [])
const summary = computed(() => data.value?.summary || { total: 0, images: 0, videos: 0, audio: 0 })
const taskSummary = computed(() => data.value?.task_summary || {
  current: 0, pending: 0, attention: 0, processing: 0, history: 0,
})
const totalPages = computed(() => Math.max(1, Math.ceil((data.value?.total || 0) / filters.limit)))
const pageStart = computed(() => data.value?.total ? (filters.page - 1) * filters.limit + 1 : 0)
const pageEnd = computed(() => Math.min(filters.page * filters.limit, data.value?.total || 0))
const hasActiveFilters = computed(() => Boolean(
  filters.project_id || filters.type || filters.media_kind || filters.review_status || filters.q,
))

const typeLabels = {
  generated_assets: '生成资产', storyboard_images: '分镜图片', shot_videos: '镜头视频',
  dialogue_audio: '对白音频', episode_masters: '剧集成片',
}
const subtypeLabels = {
  character_front: '角色正面', character_side: '角色侧面', character_full_body: '角色全身',
  character_expression: '角色表情', costume_reference: '服装参考', location_reference: '场景参考',
  prop_reference: '道具参考', storyboard_frame: '分镜画面', shot_video: '镜头视频',
  dialogue: '对白', narration: '旁白', inner_monologue: '内心独白', off_screen: '画外音',
  preview: '预览成片', clean: '无字幕成片', subtitled: '字幕成片', final: '最终成片',
}
const taskTabs = computed(() => [
  { value: 'current', label: '当前资产', count: taskSummary.value.current, icon: CheckCircle2 },
  { value: 'pending', label: '待审核', count: taskSummary.value.pending, icon: Clock3 },
  { value: 'attention', label: '异常待处理', count: taskSummary.value.attention, icon: CircleAlert },
  { value: 'processing', label: '生成中', count: taskSummary.value.processing, icon: Hourglass },
  { value: 'history', label: '历史版本', count: taskSummary.value.history, icon: History },
])
const kindFilters = [
  { value: '', label: '全部媒体', icon: LayoutGrid },
  { value: 'image', label: '图片', icon: Images },
  { value: 'video', label: '视频', icon: Film },
  { value: 'audio', label: '音频', icon: AudioLines },
]

async function load() {
  const sequence = ++requestSequence
  loading.value = true
  error.value = ''
  try {
    const response = await api.getMediaAssets({ ...filters })
    if (sequence === requestSequence) data.value = response
  } catch (err) {
    if (sequence === requestSequence) error.value = err.message
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

watch(
  () => [
    filters.project_id, filters.type, filters.media_kind, filters.review_status,
    filters.scope, filters.q, filters.sort, filters.limit,
  ],
  () => {
    if (filters.page !== 1) filters.page = 1
    else load()
  },
)
watch(() => filters.page, load)
onMounted(load)

function setScope(scope) {
  filters.review_status = ''
  filters.scope = scope
}

function applySearch() {
  filters.q = searchQuery.value.trim()
}

function resetFilters() {
  searchQuery.value = ''
  Object.assign(filters, {
    project_id: '', type: '', media_kind: '', review_status: '',
    scope: 'current', q: '', sort: 'latest', page: 1,
  })
}

function changePage(page) {
  filters.page = Math.min(totalPages.value, Math.max(1, page))
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

const formatTime = value => value
  ? new Intl.DateTimeFormat('zh-CN', {
      month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
    }).format(new Date(value))
  : '—'
const formatDuration = (value) => {
  if (!value) return ''
  const seconds = Math.round(value / 1000)
  return seconds >= 60 ? `${Math.floor(seconds / 60)}分${seconds % 60}秒` : `${seconds}秒`
}
const compactID = value => value && value.length > 18 ? `…${value.slice(-8)}` : value
const assetTitle = item => subtypeLabels[item.subtype] || item.subtype || typeLabels[item.asset_type] || item.asset_type
const entityLabel = (item) => {
  const prefix = { shot: '镜头', dialogue: '对白', episode: '剧集' }[item.entity_type] || '资产'
  return `${prefix} ${compactID(item.entity_id)}`
}
const primaryFact = (item) => {
  if (item.duration_ms) return formatDuration(item.duration_ms)
  if (item.width && item.height) return `${item.width} × ${item.height}`
  return typeLabels[item.asset_type] || item.media_kind
}
const isRecoverable = isMediaAssetRecoverable
const canRecover = canRecoverMediaAsset
const successorState = getMediaAssetSuccessorState
const sourceLabel = getMediaAssetSourceLabel
const acceptFor = item => ({
  image: 'image/jpeg,image/png,image/webp,image/gif',
  video: 'video/mp4,video/quicktime,video/webm',
  audio: 'audio/mpeg,audio/wav,audio/x-wav,audio/ogg,audio/mp4',
}[item?.media_kind] || '')
const kindName = item => ({ image: '图片', video: '视频', audio: '音频' }[item?.media_kind] || '媒体')
const replacementHint = computed(() => {
  if (!action.item) return ''
  if (action.item.media_kind === 'image') return '支持 JPG、PNG、WebP、GIF，将校验图片尺寸。'
  if (action.item.media_kind === 'video') return '支持 MP4、MOV、WebM，将校验画面尺寸与时长。'
  return '支持 MP3、WAV、OGG、M4A，将校验音频时长。'
})

function openAction(mode, item) {
  if (mode !== 'details' && !canRecover(item)) return
  Object.assign(action, {
    open: true, mode, item, prompt_adjustment: '', file: null,
    metadata: null, working: false, error: '',
  })
}

function closeAction() {
  if (!action.working) action.open = false
}

async function viewSuccessor(item) {
  if (!item?.successor_asset_id) return
  searchQuery.value = item.successor_asset_id
  Object.assign(filters, {
    project_id: item.project_id,
    type: item.asset_type,
    media_kind: '',
    review_status: '',
    scope: 'current',
    q: item.successor_asset_id,
    page: 1,
  })
  action.open = false
  await load()
  await nextTick()
  const target = document.getElementById(`media-asset-${item.successor_asset_id}`)
  target?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  target?.focus({ preventScroll: true })
}

async function readMediaMetadata(file, kind) {
  const url = URL.createObjectURL(file)
  try {
    if (kind === 'image') {
      const image = new Image()
      image.src = url
      await image.decode()
      return { width: image.naturalWidth, height: image.naturalHeight }
    }
    const media = document.createElement(kind === 'video' ? 'video' : 'audio')
    media.preload = 'metadata'
    media.src = url
    await new Promise((resolve, reject) => {
      media.onloadedmetadata = resolve
      media.onerror = () => reject(new Error(`无法读取${kind === 'video' ? '视频' : '音频'}信息`))
    })
    const metadata = { duration_ms: Math.round(media.duration * 1000) }
    if (kind === 'video') Object.assign(metadata, { width: media.videoWidth, height: media.videoHeight })
    return metadata
  } finally {
    URL.revokeObjectURL(url)
  }
}

async function selectReplacement(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  action.error = ''
  if (file.size > 512 * 1024 * 1024) {
    action.error = '文件不能超过 512MB。'
    return
  }
  try {
    const metadata = await readMediaMetadata(file, action.item.media_kind)
    if (!metadata.width && action.item.media_kind !== 'audio') throw new Error('无法读取媒体尺寸')
    if (!metadata.duration_ms && action.item.media_kind !== 'image') throw new Error('无法读取媒体时长')
    action.file = file
    action.metadata = metadata
  } catch (err) {
    action.error = `${err.message}，请确认文件格式正确。`
  }
}

async function submitAction() {
  if (!action.item || action.working) return
  action.working = true
  action.error = ''
  try {
    if (action.mode === 'regenerate') {
      const result = await api.regenerateMediaAsset(action.item.asset_type, action.item.asset_id, {
        prompt_adjustment: action.prompt_adjustment.trim(),
      })
      notice.value = `已提交重新生成，新版本 v${result.generation_version} 完成前仍保留当前资产。`
    } else {
      if (!action.file || !action.metadata) {
        action.error = `请先选择${kindName(action.item)}文件。`
        return
      }
      const form = new FormData()
      form.append('file', action.file)
      for (const [key, value] of Object.entries(action.metadata)) form.append(key, String(value))
      const result = await api.replaceMediaAsset(action.item.asset_type, action.item.asset_id, form)
      notice.value = `替换文件已保存为新版本 v${result.asset.generation_version}，原版本仍可追溯。`
    }
    action.open = false
    await load()
  } catch (err) {
    action.error = err.message
  } finally {
    action.working = false
  }
}
</script>

<template>
  <section class="view-stack media-browser-view">
    <div class="hero-row media-browser-hero">
      <div>
        <h2>媒体资产库</h2>
        <p>优先处理当前版本和异常资产，历史版本集中保留在独立视图中。</p>
      </div>
      <button class="button button-secondary" :disabled="loading" @click="load">
        <RefreshCw :size="16" :class="{ spin: loading }" />刷新资产
      </button>
    </div>

    <nav class="media-task-tabs" aria-label="资产任务视图">
      <button
        v-for="tab in taskTabs"
        :key="tab.value"
        :class="{ active: filters.scope === tab.value, attention: tab.value === 'attention' && tab.count }"
        @click="setScope(tab.value)"
      >
        <component :is="tab.icon" :size="16" />
        <span>{{ tab.label }}</span>
        <b>{{ tab.count }}</b>
      </button>
    </nav>

    <div v-if="notice" class="media-action-notice">
      <CheckCircle2 :size="18" /><span>{{ notice }}</span>
      <button aria-label="关闭提示" @click="notice = ''"><X :size="15" /></button>
    </div>

    <article class="panel media-browser-panel">
      <header class="media-browser-toolbar">
        <form class="media-search" role="search" @submit.prevent="applySearch">
          <Search :size="16" />
          <input v-model="searchQuery" aria-label="搜索媒体资产" placeholder="搜索资产、剧集、镜头或对白 ID" />
          <button type="submit">搜索</button>
        </form>
        <div class="media-view-toggle" aria-label="切换展示方式">
          <button :class="{ active: viewMode === 'grid' }" title="网格视图" @click="viewMode = 'grid'">
            <LayoutGrid :size="16" />
          </button>
          <button :class="{ active: viewMode === 'list' }" title="列表视图" @click="viewMode = 'list'">
            <List :size="16" />
          </button>
        </div>
      </header>

      <div class="media-kindbar">
        <button
          v-for="kind in kindFilters"
          :key="kind.value"
          :class="{ active: filters.media_kind === kind.value }"
          @click="filters.media_kind = kind.value"
        >
          <component :is="kind.icon" :size="15" />{{ kind.label }}
        </button>
        <span>{{ data?.total || 0 }} 条结果</span>
      </div>

      <div class="media-filterbar">
        <div class="filter-title"><Filter :size="15" />精确筛选</div>
        <select v-model="filters.project_id" class="select-control" aria-label="按项目筛选">
          <option value="">全部项目</option>
          <option v-for="project in data?.facets?.projects || []" :key="project.project_id" :value="project.project_id">
            {{ project.novel_name }}
          </option>
        </select>
        <select v-model="filters.type" class="select-control" aria-label="按资产类型筛选">
          <option value="">全部资产类型</option>
          <option v-for="type in data?.facets?.types || []" :key="type" :value="type">
            {{ typeLabels[type] || type }}
          </option>
        </select>
        <select v-model="filters.review_status" class="select-control" aria-label="按审核状态筛选">
          <option value="">全部审核状态</option>
          <option value="pending">待审核</option>
          <option value="approved">已通过</option>
          <option value="rejected">已拒绝</option>
          <option value="regenerating">重新生成</option>
        </select>
        <select v-model="filters.sort" class="select-control media-sort" aria-label="资产排序">
          <option value="latest">最近更新</option>
          <option value="oldest">最早更新</option>
          <option value="type">按类型排列</option>
        </select>
        <button v-if="hasActiveFilters" class="clear-filters" @click="resetFilters">清除筛选</button>
      </div>

      <div v-if="error" class="error-banner">{{ error }} <button @click="load">重新读取</button></div>
      <div v-if="loading" class="media-browser-loading">
        <span v-for="i in 8" :key="i"></span>
      </div>
      <EmptyState
        v-else-if="items.length === 0"
        title="没有匹配的媒体资产"
        description="尝试切换任务视图、媒体类型或清除搜索条件。"
      />
      <div v-else class="media-results" :class="`media-results-${viewMode}`">
        <article
          v-for="item in items"
          :id="`media-asset-${item.asset_id}`"
          :key="`${item.asset_type}:${item.asset_id}`"
          class="media-browser-card"
          :class="[
            `media-card-${item.media_kind}`,
            { error: canRecover(item), history: hasMediaAssetSuccessor(item) },
          ]"
          tabindex="-1"
        >
          <div class="media-browser-preview" :class="`kind-${item.media_kind}`">
            <img
              v-if="item.media_kind === 'image' && item.media_url"
              :src="item.preview_url || item.media_url"
              :alt="assetTitle(item)"
              loading="lazy"
            />
            <video
              v-else-if="item.media_kind === 'video' && item.media_url"
              :src="item.media_url"
              :poster="item.preview_url || undefined"
              controls
              preload="metadata"
            >当前浏览器不支持视频播放。</video>
            <div v-else-if="item.media_kind === 'audio' && item.media_url" class="media-audio-player">
              <div><AudioLines :size="24" /><strong>{{ assetTitle(item) }}</strong></div>
              <audio :src="item.media_url" controls preload="metadata">当前浏览器不支持音频播放。</audio>
            </div>
            <div v-else-if="hasMediaAssetSuccessor(item)" class="media-browser-missing history">
              <History :size="25" /><span>{{ successorState(item).label }}</span>
              <small>{{ successorState(item).detail }}</small>
            </div>
            <div v-else class="media-browser-missing" :class="{ error: isRecoverable(item) }">
              <AlertTriangle v-if="isRecoverable(item)" :size="25" />
              <ImageOff v-else :size="25" />
              <span>{{ isRecoverable(item) ? '媒体生成异常' : '媒体文件尚未就绪' }}</span>
            </div>
            <span class="media-kind-chip">
              <CirclePlay v-if="item.media_kind !== 'image'" :size="12" />
              <Images v-else :size="12" />
              {{ typeLabels[item.asset_type] || item.asset_type }}
            </span>
          </div>

          <div class="media-browser-card-body">
            <div class="media-browser-title">
              <div>
                <span>{{ item.novel_name }}</span>
                <h3>{{ assetTitle(item) }}</h3>
              </div>
              <StatusBadge :status="successorState(item)?.badgeStatus || item.status" />
            </div>
            <div class="media-card-facts">
              <span>{{ entityLabel(item) }}</span>
              <span>{{ primaryFact(item) }}</span>
              <span>v{{ item.generation_version || 1 }}</span>
              <span>{{ formatTime(item.updated_at) }}</span>
            </div>
            <div class="media-card-review">
              <span>审核</span><StatusBadge :status="item.review_status" />
            </div>
            <div v-if="canRecover(item)" class="media-inline-warning">
              <AlertTriangle :size="14" />
              <span>{{ item.error_message || '需要重新生成或上传文件替换' }}</span>
            </div>
            <button v-if="hasMediaAssetSuccessor(item)" class="media-successor-link" @click="viewSuccessor(item)">
              <span>{{ successorState(item).detail }}</span><ArrowRight :size="13" /><strong>查看新版本</strong>
            </button>
            <div class="media-browser-actions">
              <button @click="openAction('details', item)"><Info :size="14" />详情</button>
              <RouterLink :to="localEditLinkForMedia(item)"><GitCompare :size="14" />局部修改</RouterLink>
              <button v-if="canRecover(item)" class="primary" @click="openAction('regenerate', item)">
                <RotateCcw :size="14" />重新生成
              </button>
              <RouterLink :to="`/projects/${item.project_id}`">查看项目</RouterLink>
              <a v-if="item.media_url" :href="item.media_url" target="_blank" rel="noreferrer" title="打开原文件">
                <ExternalLink :size="14" />
              </a>
            </div>
          </div>
        </article>
      </div>

      <footer v-if="!loading && data?.total" class="media-pagination">
        <span>第 {{ pageStart }}–{{ pageEnd }} 条，共 {{ data.total }} 条</span>
        <label>
          每页
          <select v-model.number="filters.limit" class="select-control" aria-label="每页显示数量">
            <option :value="24">24</option>
            <option :value="48">48</option>
          </select>
        </label>
        <button :disabled="filters.page <= 1" aria-label="上一页" @click="changePage(filters.page - 1)">
          <ChevronLeft :size="16" />
        </button>
        <b>{{ filters.page }} / {{ totalPages }}</b>
        <button :disabled="filters.page >= totalPages" aria-label="下一页" @click="changePage(filters.page + 1)">
          <ChevronRight :size="16" />
        </button>
      </footer>
    </article>

    <div v-if="action.open && action.mode === 'details'" class="media-detail-backdrop" @click.self="closeAction">
      <aside class="media-detail-drawer" role="dialog" aria-modal="true" aria-label="资产详情">
        <header>
          <div><span>ASSET DETAILS</span><h3>{{ assetTitle(action.item) }}</h3><p>{{ action.item.asset_id }}</p></div>
          <button aria-label="关闭详情" @click="closeAction"><X :size="18" /></button>
        </header>
        <div class="media-detail-body">
          <div class="media-detail-preview" :class="`kind-${action.item.media_kind}`">
            <img
              v-if="action.item.media_kind === 'image' && action.item.media_url"
              :src="action.item.media_url"
              :alt="assetTitle(action.item)"
            />
            <video
              v-else-if="action.item.media_kind === 'video' && action.item.media_url"
              :src="action.item.media_url"
              controls
              preload="metadata"
            />
            <audio
              v-else-if="action.item.media_kind === 'audio' && action.item.media_url"
              :src="action.item.media_url"
              controls
              preload="metadata"
            />
            <div v-else class="media-browser-missing" :class="{ error: isRecoverable(action.item) }">
              <AlertTriangle v-if="isRecoverable(action.item)" :size="28" /><ImageOff v-else :size="28" />
              <span>{{ isRecoverable(action.item) ? '媒体生成异常' : '没有可预览文件' }}</span>
            </div>
          </div>

          <div class="media-detail-status">
            <StatusBadge :status="successorState(action.item)?.badgeStatus || action.item.status" />
            <StatusBadge :status="action.item.review_status" />
            <span>{{ typeLabels[action.item.asset_type] }}</span>
            <span>v{{ action.item.generation_version || 1 }}</span>
          </div>

          <div v-if="isRecoverable(action.item)" class="media-error-summary">
            <AlertTriangle :size="20" />
            <div><strong>{{ action.item.error_code || 'MEDIA_NOT_READY' }}</strong><p>{{ action.item.error_message || '生成任务已结束，但没有得到可访问的媒体文件。' }}</p></div>
          </div>

          <section class="media-detail-section">
            <h4>所属信息</h4>
            <dl class="media-detail-list">
              <div><dt>项目</dt><dd>{{ action.item.novel_name }}</dd></div>
              <div><dt>项目 ID</dt><dd>{{ action.item.project_id }}</dd></div>
              <div v-if="action.item.episode_id"><dt>剧集 ID</dt><dd>{{ action.item.episode_id }}</dd></div>
              <div><dt>{{ entityLabel(action.item).split(' ')[0] }} ID</dt><dd>{{ action.item.entity_id }}</dd></div>
              <div><dt>资产 ID</dt><dd>{{ action.item.asset_id }}</dd></div>
            </dl>
          </section>

          <section class="media-detail-section">
            <h4>媒体与生成信息</h4>
            <dl class="media-detail-list">
              <div v-if="action.item.width && action.item.height"><dt>尺寸</dt><dd>{{ action.item.width }} × {{ action.item.height }}</dd></div>
              <div v-if="action.item.duration_ms"><dt>时长</dt><dd>{{ formatDuration(action.item.duration_ms) }}</dd></div>
              <div><dt>生成服务</dt><dd>{{ action.item.provider || '—' }}</dd></div>
              <div><dt>模型</dt><dd>{{ action.item.model || '—' }}</dd></div>
              <div><dt>更新时间</dt><dd>{{ formatTime(action.item.updated_at) }}</dd></div>
              <div v-if="action.item.task_id"><dt>任务 ID</dt><dd>{{ action.item.task_id }}</dd></div>
            </dl>
          </section>

          <section class="media-detail-section">
            <h4>版本记录</h4>
            <div class="media-version-timeline">
              <div v-if="action.item.predecessor_asset_id">
                <i></i><span>上一个版本</span><code>{{ action.item.predecessor_asset_id }}</code>
              </div>
              <div class="current"><i></i><span>当前查看 · v{{ action.item.generation_version || 1 }}</span><code>{{ sourceLabel(action.item) || action.item.asset_id }}</code></div>
              <button v-if="hasMediaAssetSuccessor(action.item)" @click="viewSuccessor(action.item)">
                <i></i><span>{{ successorState(action.item).label }} · v{{ action.item.successor_generation_version }}</span>
                <code>{{ action.item.successor_asset_id }}</code>
              </button>
            </div>
          </section>
        </div>
        <footer>
          <RouterLink class="button button-secondary" :to="`/projects/${action.item.project_id}`">查看项目</RouterLink>
          <a v-if="action.item.media_url" class="button button-secondary" :href="action.item.media_url" target="_blank" rel="noreferrer">
            <ExternalLink :size="14" />原文件
          </a>
          <button v-if="canRecover(action.item)" class="button button-secondary" @click="openAction('replace', action.item)">
            <Upload :size="15" />上传替换
          </button>
          <button v-if="canRecover(action.item)" class="button button-primary" @click="openAction('regenerate', action.item)">
            <RotateCcw :size="15" />重新生成
          </button>
        </footer>
      </aside>
    </div>

    <div v-if="action.open && action.mode !== 'details'" class="modal-backdrop media-action-backdrop" @click.self="closeAction">
      <section class="review-modal media-action-modal" role="dialog" aria-modal="true" :aria-label="action.mode === 'replace' ? '上传替换资产' : '重新生成资产'">
        <header class="media-action-head">
          <div><span>资产恢复</span><h3>{{ action.mode === 'replace' ? '上传替换资产' : '重新生成资产' }}</h3></div>
          <button :disabled="action.working" aria-label="关闭" @click="closeAction"><X :size="18" /></button>
        </header>

        <template v-if="action.mode === 'regenerate'">
          <div class="media-version-notice"><Info :size="17" /><span>系统会创建 v{{ (action.item.generation_version || 1) + 1 }}；新版本成功前不会覆盖当前记录。</span></div>
          <label class="field media-action-field">
            <span>调整意见 <small>选填</small></span>
            <textarea v-model="action.prompt_adjustment" rows="4" placeholder="例如：保持人物与构图，只修复面部变形和画面闪烁"></textarea>
          </label>
          <div v-if="action.error" class="media-action-error"><AlertTriangle :size="16" />{{ action.error }}</div>
          <div class="media-modal-actions">
            <button class="button button-secondary" :disabled="action.working" @click="closeAction">取消</button>
            <button class="button button-primary" :disabled="action.working" @click="submitAction">
              <LoaderCircle v-if="action.working" :size="15" class="spin" /><RotateCcw v-else :size="15" />
              {{ action.working ? '正在提交' : '确认重新生成' }}
            </button>
          </div>
        </template>

        <template v-else>
          <div class="media-version-notice"><Info :size="17" /><span>上传内容将成为新版本，原异常版本会保留在历史记录中。</span></div>
          <input ref="fileInput" class="media-file-input" type="file" :accept="acceptFor(action.item)" @change="selectReplacement" />
          <button class="media-upload-zone" :class="{ selected: action.file }" :disabled="action.working" @click="fileInput?.click()">
            <CheckCircle2 v-if="action.file" :size="26" /><Upload v-else :size="26" />
            <strong>{{ action.file ? action.file.name : `选择${kindName(action.item)}文件` }}</strong>
            <span v-if="action.file">
              {{ (action.file.size / 1024 / 1024).toFixed(2) }} MB
              {{ action.metadata?.width ? ` · ${action.metadata.width} × ${action.metadata.height}` : '' }}
              {{ action.metadata?.duration_ms ? ` · ${formatDuration(action.metadata.duration_ms)}` : '' }}
            </span>
            <span v-else>{{ replacementHint }}</span>
          </button>
          <div v-if="action.error" class="media-action-error"><AlertTriangle :size="16" />{{ action.error }}</div>
          <div class="media-modal-actions">
            <button class="button button-secondary" :disabled="action.working" @click="closeAction">取消</button>
            <button class="button button-primary" :disabled="action.working || !action.file" @click="submitAction">
              <LoaderCircle v-if="action.working" :size="15" class="spin" /><Upload v-else :size="15" />
              {{ action.working ? '正在上传' : '上传并创建新版本' }}
            </button>
          </div>
        </template>
      </section>
    </div>
  </section>
</template>
