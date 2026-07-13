<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { AudioLines, CirclePlay, ExternalLink, Film, Filter, ImageOff, Images, RefreshCw } from 'lucide-vue-next'
import { api } from '../services/api'
import EmptyState from '../components/EmptyState.vue'
import StatusBadge from '../components/StatusBadge.vue'

const data = ref(null)
const loading = ref(true)
const error = ref('')
const filters = reactive({ project_id: '', type: '', review_status: '' })
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
        <article v-for="item in items" :key="`${item.asset_type}:${item.asset_id}`" class="media-card">
          <div class="media-preview" :class="`kind-${item.media_kind}`">
            <img v-if="item.media_kind === 'image' && item.media_url" :src="item.preview_url || item.media_url" :alt="assetTitle(item)" loading="lazy" />
            <video v-else-if="item.media_kind === 'video' && item.media_url" :src="item.media_url" :poster="item.preview_url || undefined" controls preload="metadata">当前浏览器不支持视频播放。</video>
            <div v-else-if="item.media_kind === 'audio' && item.media_url" class="audio-player"><AudioLines :size="34" /><strong>{{ assetTitle(item) }}</strong><audio :src="item.media_url" controls preload="metadata">当前浏览器不支持音频播放。</audio></div>
            <div v-else class="media-missing"><ImageOff :size="28" /><span>媒体文件尚未就绪</span></div>
            <span class="media-kind-chip"><CirclePlay v-if="item.media_kind !== 'image'" :size="12" /><Images v-else :size="12" />{{ typeLabels[item.asset_type] || item.asset_type }}</span>
          </div>
          <div class="media-card-body">
            <div class="media-card-title"><div><span>{{ item.novel_name }}</span><h3>{{ assetTitle(item) }}</h3></div><StatusBadge :status="item.status" /></div>
            <code class="media-asset-id" :title="item.asset_id">{{ item.asset_id }}</code>
            <div class="media-review-line"><span>审核状态</span><StatusBadge :status="item.review_status" /></div>
            <dl class="media-meta">
              <div><dt>项目</dt><dd>{{ item.project_id }}</dd></div>
              <div v-if="item.episode_id"><dt>剧集</dt><dd>{{ item.episode_id }}</dd></div>
              <div v-if="item.width && item.height"><dt>尺寸</dt><dd>{{ item.width }} × {{ item.height }}</dd></div>
              <div v-if="item.duration_ms"><dt>时长</dt><dd>{{ formatDuration(item.duration_ms) }}</dd></div>
              <div v-if="item.provider"><dt>模型</dt><dd>{{ item.provider }} · {{ item.model || '—' }}</dd></div>
              <div><dt>更新</dt><dd>{{ formatTime(item.updated_at) }}</dd></div>
            </dl>
            <div class="media-card-actions">
              <RouterLink :to="`/projects/${item.project_id}`">查看项目</RouterLink>
              <a v-if="item.media_url" :href="item.media_url" target="_blank" rel="noreferrer"><ExternalLink :size="13" />打开原文件</a>
            </div>
          </div>
        </article>
      </div>
    </article>
  </section>
</template>
