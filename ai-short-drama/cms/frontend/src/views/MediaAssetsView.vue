<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  AlertTriangle, AudioLines, CheckCircle2, CirclePlay, ExternalLink, Film, Filter,
  ImageOff, Images, Info, LoaderCircle, RefreshCw, RotateCcw, Upload, X,
} from 'lucide-vue-next'
import { api } from '../services/api'
import {
  canRecoverMediaAsset,
  getMediaAssetSuccessorState,
  hasMediaAssetSuccessor,
  isMediaAssetRecoverable,
} from '../services/mediaAssetVersions'
import EmptyState from '../components/EmptyState.vue'
import StatusBadge from '../components/StatusBadge.vue'

const data = ref(null)
const loading = ref(true)
const error = ref('')
const notice = ref('')
const filters = reactive({ project_id: '', type: '', review_status: '' })
const action = reactive({
  open: false, mode: '', item: null, prompt_adjustment: '', file: null,
  metadata: null, working: false, error: '',
})
const fileInput = ref(null)
let requestSequence = 0

const items = computed(() => data.value?.items || [])
const summary = computed(() => data.value?.summary || { total: 0, images: 0, videos: 0, audio: 0 })
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

async function load() {
  const sequence = ++requestSequence
  loading.value = true
  error.value = ''
  try {
    const response = await api.getMediaAssets({ ...filters, limit: 120 })
    if (sequence === requestSequence) data.value = response
  } catch (err) {
    if (sequence === requestSequence) error.value = err.message
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

watch(() => [filters.project_id, filters.type, filters.review_status], load)
onMounted(load)

function resetFilters() {
  Object.assign(filters, { project_id: '', type: '', review_status: '' })
}

const formatTime = (value) => value
  ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
  : '—'
const formatDuration = (value) => {
  if (!value) return ''
  const seconds = Math.round(value / 1000)
  return seconds >= 60 ? `${Math.floor(seconds / 60)}分${seconds % 60}秒` : `${seconds}秒`
}
const assetTitle = (item) => subtypeLabels[item.subtype] || item.subtype || typeLabels[item.asset_type] || item.asset_type
const isRecoverable = isMediaAssetRecoverable
const canRecover = canRecoverMediaAsset
const successorState = getMediaAssetSuccessorState
const acceptFor = (item) => ({
  image: 'image/jpeg,image/png,image/webp,image/gif',
  video: 'video/mp4,video/quicktime,video/webm',
  audio: 'audio/mpeg,audio/wav,audio/x-wav,audio/ogg,audio/mp4',
}[item?.media_kind] || '')
const kindName = (item) => ({ image: '图片', video: '视频', audio: '音频' }[item?.media_kind] || '媒体')
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
  <section class="view-stack">
    <div class="hero-row">
      <div><h2>媒体资产库</h2><p>统一浏览项目生成的图片、镜头视频、对白音频和剧集成片。</p></div>
      <button class="button button-secondary" :disabled="loading" @click="load"><RefreshCw :size="16" :class="{ spin: loading }" />刷新资产</button>
    </div>

    <div class="metric-grid media-metrics">
      <article class="metric-card"><div class="metric-icon blue"><Images :size="20" /></div><div><span>全部资产</span><strong>{{ summary.total }}</strong><small>当前筛选范围</small></div></article>
      <article class="metric-card"><div class="metric-icon green"><Images :size="20" /></div><div><span>图片</span><strong>{{ summary.images }}</strong><small>生成资产与分镜图片</small></div></article>
      <article class="metric-card"><div class="metric-icon amber"><Film :size="20" /></div><div><span>视频</span><strong>{{ summary.videos }}</strong><small>镜头视频与剧集成片</small></div></article>
      <article class="metric-card"><div class="metric-icon red"><AudioLines :size="20" /></div><div><span>音频</span><strong>{{ summary.audio }}</strong><small>对白与旁白音频</small></div></article>
    </div>

    <div v-if="notice" class="media-action-notice"><CheckCircle2 :size="18" /><span>{{ notice }}</span><button aria-label="关闭提示" @click="notice = ''"><X :size="15" /></button></div>

    <article class="panel media-library-panel">
      <div class="review-filterbar media-filterbar">
        <div class="filter-title"><Filter :size="16" />筛选</div>
        <select v-model="filters.project_id" class="select-control review-select" aria-label="按项目筛选">
          <option value="">全部项目</option>
          <option v-for="project in data?.facets?.projects || []" :key="project.project_id" :value="project.project_id">{{ project.novel_name }} · {{ project.project_id }}</option>
        </select>
        <select v-model="filters.type" class="select-control review-select" aria-label="按资产类型筛选">
          <option value="">全部类型</option>
          <option v-for="type in data?.facets?.types || []" :key="type" :value="type">{{ typeLabels[type] || type }}</option>
        </select>
        <select v-model="filters.review_status" class="select-control review-select" aria-label="按审核状态筛选">
          <option value="">全部审核状态</option>
          <option value="pending">待审核</option><option value="approved">已通过</option>
          <option value="rejected">已拒绝</option><option value="regenerating">重新生成</option>
        </select>
        <button class="clear-filters" @click="resetFilters">清除筛选</button><span class="result-count">{{ data?.total || 0 }} 条资产</span>
      </div>

      <div v-if="error" class="error-banner">{{ error }} <button @click="load">重新读取</button></div>
      <div v-if="loading" class="media-loading"><span v-for="i in 8" :key="i"></span></div>
      <EmptyState v-else-if="items.length === 0" title="没有匹配的媒体资产" description="当前项目或筛选条件下还没有可展示的媒体文件。" />
      <div v-else class="media-grid">
        <article v-for="item in items" :key="`${item.asset_type}:${item.asset_id}`" class="media-card" :class="{ 'media-card-error': canRecover(item), 'media-card-superseded': hasMediaAssetSuccessor(item) }">
          <div class="media-preview" :class="`kind-${item.media_kind}`">
            <img v-if="item.media_kind === 'image' && item.media_url" :src="item.preview_url || item.media_url" :alt="assetTitle(item)" loading="lazy" />
            <video v-else-if="item.media_kind === 'video' && item.media_url" :src="item.media_url" :poster="item.preview_url || undefined" controls preload="metadata">当前浏览器不支持视频播放。</video>
            <div v-else-if="item.media_kind === 'audio' && item.media_url" class="audio-player"><AudioLines :size="34" /><strong>{{ assetTitle(item) }}</strong><audio :src="item.media_url" controls preload="metadata">当前浏览器不支持音频播放。</audio></div>
            <div v-else-if="hasMediaAssetSuccessor(item)" class="media-missing superseded">
              <CheckCircle2 :size="28" />
              <span>{{ successorState(item).label }}</span>
              <small>{{ successorState(item).detail }}</small>
            </div>
            <div v-else class="media-missing" :class="{ error: isRecoverable(item) }">
              <AlertTriangle v-if="isRecoverable(item)" :size="28" />
              <ImageOff v-else :size="28" />
              <span>{{ isRecoverable(item) ? '媒体生成异常' : '媒体文件尚未就绪' }}</span>
              <small v-if="isRecoverable(item)">可以重新生成或上传文件替换</small>
            </div>
            <span class="media-kind-chip"><CirclePlay v-if="item.media_kind !== 'image'" :size="12" /><Images v-else :size="12" />{{ typeLabels[item.asset_type] || item.asset_type }}</span>
          </div>
          <div class="media-card-body">
            <div class="media-card-title"><div><span>{{ item.novel_name }}</span><h3>{{ assetTitle(item) }}</h3></div><StatusBadge :status="successorState(item)?.badgeStatus || item.status" /></div>
            <code class="media-asset-id" :title="item.asset_id">{{ item.asset_id }}</code>
            <div class="media-review-line"><span>审核状态</span><StatusBadge :status="item.review_status" /></div>
            <dl class="media-meta">
              <div><dt>项目</dt><dd>{{ item.project_id }}</dd></div>
              <div v-if="item.episode_id"><dt>剧集</dt><dd>{{ item.episode_id }}</dd></div>
              <div v-if="item.width && item.height"><dt>尺寸</dt><dd>{{ item.width }} × {{ item.height }}</dd></div>
              <div v-if="item.duration_ms"><dt>时长</dt><dd>{{ formatDuration(item.duration_ms) }}</dd></div>
              <div v-if="item.provider"><dt>模型</dt><dd>{{ item.provider }} · {{ item.model || '—' }}</dd></div>
              <div><dt>版本</dt><dd>v{{ item.generation_version || 1 }}{{ hasMediaAssetSuccessor(item) ? ' · 历史' : item.is_current ? ' · 当前' : '' }}</dd></div>
              <div v-if="hasMediaAssetSuccessor(item)"><dt>后继</dt><dd>v{{ item.successor_generation_version }} · {{ item.successor_asset_id }}</dd></div>
              <div><dt>更新</dt><dd>{{ formatTime(item.updated_at) }}</dd></div>
            </dl>
            <div v-if="canRecover(item)" class="media-recovery-actions">
              <button class="media-primary-action" @click="openAction('regenerate', item)"><RotateCcw :size="14" />重新生成</button>
              <button @click="openAction('replace', item)"><Upload :size="14" />上传替换</button>
              <button class="media-icon-action" title="查看异常详情" aria-label="查看异常详情" @click="openAction('details', item)"><Info :size="15" /></button>
            </div>
            <div class="media-card-actions">
              <RouterLink :to="`/projects/${item.project_id}`">查看项目</RouterLink>
              <a v-if="item.media_url" :href="item.media_url" target="_blank" rel="noreferrer"><ExternalLink :size="13" />打开原文件</a>
            </div>
          </div>
        </article>
      </div>
    </article>

    <div v-if="action.open" class="modal-backdrop media-action-backdrop" @click.self="closeAction">
      <section class="review-modal media-action-modal" role="dialog" aria-modal="true" :aria-label="action.mode === 'details' ? '异常详情' : action.mode === 'replace' ? '上传替换资产' : '重新生成资产'">
        <header class="media-action-head">
          <div>
            <span>{{ action.mode === 'details' ? '异常诊断' : '资产恢复' }}</span>
            <h3>{{ action.mode === 'details' ? '异常详情' : action.mode === 'replace' ? '上传替换资产' : '重新生成资产' }}</h3>
          </div>
          <button :disabled="action.working" aria-label="关闭" @click="closeAction"><X :size="18" /></button>
        </header>

        <template v-if="action.mode === 'details'">
          <div class="media-error-summary"><AlertTriangle :size="20" /><div><strong>{{ action.item.error_code || 'MEDIA_NOT_READY' }}</strong><p>{{ action.item.error_message || '生成任务已结束，但没有得到可访问的媒体文件。' }}</p></div></div>
          <dl class="media-error-details">
            <div><dt>资产 ID</dt><dd>{{ action.item.asset_id }}</dd></div>
            <div><dt>任务 ID</dt><dd>{{ action.item.task_id || '—' }}</dd></div>
            <div><dt>失败阶段</dt><dd>{{ typeLabels[action.item.asset_type] || action.item.asset_type }}</dd></div>
            <div><dt>重试次数</dt><dd>{{ action.item.retry_count || 0 }} / {{ action.item.max_retries || 3 }}</dd></div>
            <div><dt>发生时间</dt><dd>{{ formatTime(action.item.updated_at) }}</dd></div>
            <div><dt>当前版本</dt><dd>v{{ action.item.generation_version || 1 }}</dd></div>
          </dl>
          <div class="media-modal-actions">
            <button class="button button-secondary" @click="openAction('replace', action.item)"><Upload :size="15" />上传替换</button>
            <button class="button button-primary" @click="openAction('regenerate', action.item)"><RotateCcw :size="15" />重新生成</button>
          </div>
        </template>

        <template v-else-if="action.mode === 'regenerate'">
          <div class="media-version-notice"><Info :size="17" /><span>系统会创建 v{{ (action.item.generation_version || 1) + 1 }}；新版本成功前不会覆盖当前记录。</span></div>
          <label class="field media-action-field">
            <span>调整意见 <small>选填</small></span>
            <textarea v-model="action.prompt_adjustment" rows="4" placeholder="例如：保持人物与构图，只修复面部变形和画面闪烁"></textarea>
          </label>
          <div v-if="action.error" class="media-action-error"><AlertTriangle :size="16" />{{ action.error }}</div>
          <div class="media-modal-actions">
            <button class="button button-secondary" :disabled="action.working" @click="closeAction">取消</button>
            <button class="button button-primary" :disabled="action.working" @click="submitAction">
              <LoaderCircle v-if="action.working" :size="15" class="spin" /><RotateCcw v-else :size="15" />{{ action.working ? '正在提交' : '确认重新生成' }}
            </button>
          </div>
        </template>

        <template v-else>
          <div class="media-version-notice"><Info :size="17" /><span>上传内容将成为新版本，原异常版本会保留在历史记录中。</span></div>
          <input ref="fileInput" class="media-file-input" type="file" :accept="acceptFor(action.item)" @change="selectReplacement" />
          <button class="media-upload-zone" :class="{ selected: action.file }" :disabled="action.working" @click="fileInput?.click()">
            <CheckCircle2 v-if="action.file" :size="26" /><Upload v-else :size="26" />
            <strong>{{ action.file ? action.file.name : `选择${kindName(action.item)}文件` }}</strong>
            <span v-if="action.file">{{ (action.file.size / 1024 / 1024).toFixed(2) }} MB · {{ action.metadata?.width ? `${action.metadata.width} × ${action.metadata.height}` : '' }}{{ action.metadata?.duration_ms ? ` · ${formatDuration(action.metadata.duration_ms)}` : '' }}</span>
            <span v-else>{{ replacementHint }}</span>
          </button>
          <div v-if="action.error" class="media-action-error"><AlertTriangle :size="16" />{{ action.error }}</div>
          <div class="media-modal-actions">
            <button class="button button-secondary" :disabled="action.working" @click="closeAction">取消</button>
            <button class="button button-primary" :disabled="action.working || !action.file" @click="submitAction">
              <LoaderCircle v-if="action.working" :size="15" class="spin" /><Upload v-else :size="15" />{{ action.working ? '正在上传' : '上传并创建新版本' }}
            </button>
          </div>
        </template>
      </section>
    </div>
  </section>
</template>
