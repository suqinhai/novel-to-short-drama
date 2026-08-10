<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import {
  Check, ChevronLeft, ChevronRight, Film, LoaderCircle, Magnet, Pause, Play,
  RotateCcw, Scissors, StepBack, StepForward, Volume2, ZoomIn, ZoomOut,
} from 'lucide-vue-next'
import { api } from '../services/api'
import {
  NLE_TRACKS, activeItemsAt, formatNLETimecode, gesturePatch, itemBoundaries,
  stepPlayhead, subtitlePreviewConfig, visibleTimelineWindow,
} from '../services/timelineNle'

const props = defineProps({
  projectId: { type: String, required: true },
  episodeId: { type: String, required: true },
  timelineVersions: { type: Array, default: () => [] },
})
const emit = defineEmits(['notice', 'error', 'versions-changed'])

const viewport = ref(null)
const video = ref(null)
const page = ref(null)
const loading = ref(true)
const loadingMore = ref(false)
const saving = ref(false)
const playing = ref(false)
const currentMS = ref(0)
const zoom = ref(60)
const snapping = ref(true)
const stepMode = ref('frame')
const selectedItemID = ref('')
const versionChoice = ref('')
const selection = reactive({ start: 0, end: 0, active: false })
const drag = reactive({ active: false, item: null, mode: '', startX: 0, preview: null })
const audioPlayers = new Map()
let animationFrame = 0
let clockOrigin = 0
let scrollTimer = 0
let pollingTimer = 0
let requestSerial = 0

const timeline = computed(() => page.value?.timeline || null)
const durationMS = computed(() => Math.max(1, Number(timeline.value?.target_duration_ms || 1)))
const fps = computed(() => Number(timeline.value?.fps || 25))
const currentTimelineID = computed(() => page.value?.current_timeline_id || '')
const items = computed(() => page.value?.items || [])
const timelineWidth = computed(() => Math.max(1050, durationMS.value / 1000 * zoom.value))
const lanes = computed(() => NLE_TRACKS.map(track => ({
  ...track, entries: items.value.filter(item => item.track_type === track.type),
})))
const selectedItem = computed(() => items.value.find(item => item.timeline_item_id === selectedItemID.value) || null)
const activeVideo = computed(() => activeItemsAt(items.value, currentMS.value, ['video'])[0] || null)
const activeSubtitle = computed(() => activeItemsAt(items.value, currentMS.value, ['subtitle'])[0] || null)
const subtitleConfig = computed(() => subtitlePreviewConfig(activeSubtitle.value, objectConfig(timeline.value?.subtitle_config)))
const canRender = computed(() => ['draft', 'render_failed'].includes(timeline.value?.approval_state)
  && !['pending', 'claimed', 'processing'].includes(page.value?.render_job?.status))
const renderStatus = computed(() => page.value?.render_job?.status || '')
const rulerTicks = computed(() => {
  const start = page.value?.window_start_ms || 0
  const end = page.value?.window_end_ms || Math.min(durationMS.value, 30000)
  const interval = zoom.value >= 130 ? 1000 : zoom.value >= 55 ? 5000 : 10000
  const first = Math.floor(start / interval) * interval
  const result = []
  for (let value = first; value <= Math.min(durationMS.value, end + interval); value += interval) result.push(value)
  return result
})

function objectConfig(value) {
  if (!value) return {}
  if (typeof value === 'object') return value
  try { return JSON.parse(value) } catch { return {} }
}

function displayItem(item) {
  return drag.active && drag.item?.timeline_item_id === item.timeline_item_id
    ? { ...item, ...(drag.preview || {}) } : item
}

function itemStyle(item) {
  const value = displayItem(item)
  return {
    left: `${Number(value.timeline_start_ms) / 1000 * zoom.value}px`,
    width: `${Math.max(8, (Number(value.timeline_end_ms) - Number(value.timeline_start_ms)) / 1000 * zoom.value)}px`,
  }
}

function timeStyle(value) {
  return { left: `${96 + Number(value) / 1000 * zoom.value}px` }
}

async function loadWindow(timelineID = timeline.value?.timeline_id || '', forcedWindow = null) {
  const serial = ++requestSerial
  const window = forcedWindow || visibleTimelineWindow(
    viewport.value?.scrollLeft || 0,
    Math.max(1, (viewport.value?.clientWidth || 1200) - 96),
    zoom.value, durationMS.value || 30000,
  )
  try {
    const result = await api.getNLETimeline(props.projectId, props.episodeId, {
      timeline_id: timelineID || undefined, start_ms: window.start_ms,
      end_ms: Math.max(window.start_ms + 1, window.end_ms), limit: 500,
    })
    if (serial !== requestSerial) return
    page.value = result
    versionChoice.value = result.timeline.timeline_id
    if (selectedItemID.value && !result.items.some(item => item.timeline_item_id === selectedItemID.value)) {
      selectedItemID.value = ''
    }
    manageRenderPolling()
  } catch (error) {
    emit('error', error.message)
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (!page.value?.has_more || loadingMore.value) return
  const expectedTimelineID = timeline.value.timeline_id
  const expectedStartMS = page.value.window_start_ms
  const expectedEndMS = page.value.window_end_ms
  const offset = items.value.length
  loadingMore.value = true
  try {
    const result = await api.getNLETimeline(props.projectId, props.episodeId, {
      timeline_id: expectedTimelineID, start_ms: expectedStartMS,
      end_ms: expectedEndMS, limit: 500, offset,
    })
    if (timeline.value?.timeline_id !== expectedTimelineID ||
      page.value?.window_start_ms !== expectedStartMS || page.value?.window_end_ms !== expectedEndMS) return
    const seen = new Set(items.value.map(item => item.timeline_item_id))
    page.value = {
      ...result,
      items: [...items.value, ...result.items.filter(item => !seen.has(item.timeline_item_id))],
    }
  } catch (error) {
    emit('error', error.message)
  } finally {
    loadingMore.value = false
  }
}

function scheduleWindowLoad() {
  clearTimeout(scrollTimer)
  scrollTimer = setTimeout(() => loadWindow(), 120)
}

function ensureTimeVisible(timeMS) {
  if (!viewport.value) return
  const left = timeMS / 1000 * zoom.value
  const visibleLeft = viewport.value.scrollLeft
  const visibleRight = visibleLeft + viewport.value.clientWidth - 96
  if (left < visibleLeft + 80 || left > visibleRight - 80) {
    viewport.value.scrollLeft = Math.max(0, left - (viewport.value.clientWidth - 96) / 2)
    scheduleWindowLoad()
  }
}

function setPlayhead(value, keepVisible = true) {
  currentMS.value = Math.min(durationMS.value, Math.max(0, Math.round(Number(value) || 0)))
  if (playing.value) clockOrigin = performance.now() - currentMS.value
  syncMedia()
  if (keepVisible) ensureTimeVisible(currentMS.value)
}

function togglePlayback() {
  if (playing.value) { pausePlayback(); return }
  if (currentMS.value >= durationMS.value) currentMS.value = 0
  playing.value = true
  clockOrigin = performance.now() - currentMS.value
  tick()
}

function pausePlayback() {
  playing.value = false
  cancelAnimationFrame(animationFrame)
  video.value?.pause()
  for (const player of audioPlayers.values()) player.pause()
}

function tick() {
  if (!playing.value) return
  currentMS.value = Math.min(durationMS.value, Math.round(performance.now() - clockOrigin))
  syncMedia()
  if (currentMS.value >= durationMS.value) { pausePlayback(); return }
  ensureTimeVisible(currentMS.value)
  animationFrame = requestAnimationFrame(tick)
}

function localMediaSeconds(item) {
  return Math.max(0, (Number(item.source_in_ms || 0) + currentMS.value - Number(item.timeline_start_ms)) / 1000)
}

function syncVideo() {
  const element = video.value
  const item = activeVideo.value
  if (!element || !item?.proxy_url) return
  const target = localMediaSeconds(item)
  if (Number.isFinite(element.duration) && Math.abs(element.currentTime - target) > .12) element.currentTime = target
  if (playing.value && element.paused) element.play().catch(() => {})
  if (!playing.value && !element.paused) element.pause()
}

function envelopeVolume(item) {
  const elapsed = currentMS.value - Number(item.timeline_start_ms)
  const remaining = Number(item.timeline_end_ms) - currentMS.value
  const fadeIn = Number(item.fade_in_ms || 0)
  const fadeOut = Number(item.fade_out_ms || 0)
  const envelope = Math.min(fadeIn ? elapsed / fadeIn : 1, fadeOut ? remaining / fadeOut : 1, 1)
  return Math.max(0, Math.min(1, Number(item.volume ?? 1) * Math.max(0, envelope)))
}

function syncAudio() {
  if (typeof Audio === 'undefined') return
  const active = activeItemsAt(items.value, currentMS.value, ['dialogue', 'narration', 'bgm', 'ambience', 'sound_effect'])
    .filter(item => item.proxy_url)
  const activeIDs = new Set(active.map(item => item.timeline_item_id))
  for (const [id, player] of audioPlayers) {
    if (!activeIDs.has(id)) { player.pause(); audioPlayers.delete(id) }
  }
  for (const item of active) {
    let player = audioPlayers.get(item.timeline_item_id)
    if (!player) {
      player = new Audio(item.proxy_url)
      player.preload = 'metadata'
      audioPlayers.set(item.timeline_item_id, player)
    }
    const target = localMediaSeconds(item)
    if (Math.abs(player.currentTime - target) > .16) player.currentTime = target
    player.volume = envelopeVolume(item)
    if (playing.value && player.paused) player.play().catch(() => {})
    if (!playing.value && !player.paused) player.pause()
  }
}

function syncMedia() { syncVideo(); syncAudio() }

watch(() => activeVideo.value?.timeline_item_id, () => nextTick(syncVideo))

function step(direction) {
  pausePlayback()
  setPlayhead(stepPlayhead(currentMS.value, direction, stepMode.value, fps.value, durationMS.value))
}

function selectClip(item) {
  selectedItemID.value = item.timeline_item_id
  if (currentMS.value < Number(item.timeline_start_ms) || currentMS.value >= Number(item.timeline_end_ms)) {
    setPlayhead(item.timeline_start_ms)
  }
}

function beginGesture(event, item, mode) {
  if (saving.value || timeline.value?.approval_state === 'rendering') return
  event.currentTarget.setPointerCapture?.(event.pointerId)
  Object.assign(drag, { active: true, item, mode, startX: event.clientX, preview: {} })
}

function pointerMove(event) {
  if (!drag.active) return
  const delta = (event.clientX - drag.startX) / zoom.value * 1000
  drag.preview = gesturePatch(drag.item, drag.mode, delta,
    snapping.value ? itemBoundaries(items.value, drag.item.timeline_item_id) : [], snapping.value ? 90 : 0)
}

async function pointerUp() {
  if (!drag.active) return
  const item = drag.item
  const changes = drag.preview || {}
  Object.assign(drag, { active: false, item: null, mode: '', preview: null })
  if (Object.keys(changes).length) await savePatch(item, changes, `timeline_${drag.mode || 'gesture'}`)
}

function timelineTimeFromPointer(event) {
  const content = event.currentTarget.closest('.nle-timeline-content') || event.currentTarget
  const rect = content.getBoundingClientRect()
  return Math.min(durationMS.value, Math.max(0, Math.round((event.clientX - rect.left - 96) / zoom.value * 1000)))
}

function beginSelection(event) {
  if (event.target.closest('.nle-clip')) return
  const value = timelineTimeFromPointer(event)
  Object.assign(selection, { start: value, end: value, active: true })
  setPlayhead(value, false)
}

function selectionMove(event) {
  if (!selection.active) return
  selection.end = timelineTimeFromPointer(event)
}

function endSelection() { selection.active = false }

async function savePatch(item, changes, reason) {
  if (!item || saving.value) return
  saving.value = true
  try {
    const result = await api.editNLETimelineItem(props.projectId, props.episodeId,
      timeline.value.timeline_id, item.timeline_item_id, {
        base_timeline_id: timeline.value.timeline_id, ...changes, reason, actor: 'creative-workbench-nle',
      })
    selectedItemID.value = result.item?.timeline_item_id || ''
    emit('notice', `编辑已写入草稿时间线 v${result.timeline.version}；尚未创建渲染任务。`)
    await loadWindow(result.timeline.timeline_id)
    emit('versions-changed')
  } catch (error) {
    emit('error', error.message)
  } finally {
    saving.value = false
  }
}

function commitNumeric(field, event) {
  const item = selectedItem.value
  if (!item) return
  const value = Number(event.target.value)
  if (!Number.isFinite(value)) return
  if (field === 'timeline_start_ms') {
    const delta = Math.round(value) - Number(item.timeline_start_ms)
    savePatch(item, { timeline_start_ms: Math.round(value), timeline_end_ms: Number(item.timeline_end_ms) + delta }, 'move_clip')
  } else savePatch(item, { [field]: field === 'volume' ? value : Math.round(value) }, `set_${field}`)
}

function applyJCut() {
  const item = selectedItem.value
  if (!item) return
  const amount = Math.min(100, Number(item.timeline_start_ms), Number(item.source_in_ms || 0))
  if (!amount) return
  savePatch(item, {
    timeline_start_ms: Number(item.timeline_start_ms) - amount,
    source_in_ms: Number(item.source_in_ms) - amount,
  }, 'j_cut')
}

function applyLCut() {
  const item = selectedItem.value
  if (!item) return
  const patch = { timeline_end_ms: Number(item.timeline_end_ms) + 100 }
  if (item.source_out_ms != null) patch.source_out_ms = Number(item.source_out_ms) + 100
  savePatch(item, patch, 'l_cut')
}

function updateSubtitleTransform(patch, reason) {
  if (selectedItem.value?.track_type !== 'subtitle') return
  savePatch(selectedItem.value, { transform_config: patch }, reason)
}

function updateSubtitleStyle(style) {
  if (selectedItem.value?.track_type !== 'subtitle') return
  const presets = {
    clean: { subtitle_style: 'clean', color: '#ffffff', background: 'rgba(0,0,0,.64)', font_weight: 700 },
    outline: { subtitle_style: 'outline', color: '#ffffff', background: 'rgba(0,0,0,.18)', font_weight: 800 },
    emphasis: { subtitle_style: 'emphasis', color: '#ffe56b', background: 'rgba(18,22,35,.78)', font_weight: 900 },
  }
  savePatch(selectedItem.value, { effect_config: presets[style] || presets.clean }, 'subtitle_style')
}

async function confirmRender() {
  if (!canRender.value || saving.value) return
  saving.value = true
  try {
    const job = await api.confirmNLETimelineRender(props.projectId, props.episodeId, timeline.value.timeline_id)
    emit('notice', `已显式确认重编，渲染任务 ${job.render_job_id} 已创建；成功前 current 保持 v${props.timelineVersions.find(item => item.timeline_id === currentTimelineID.value)?.version || '旧版'}。`)
    await loadWindow(timeline.value.timeline_id)
    emit('versions-changed')
  } catch (error) { emit('error', error.message) } finally { saving.value = false }
}

async function restoreVersion() {
  if (!versionChoice.value || saving.value) return
  const source = props.timelineVersions.find(item => item.timeline_id === versionChoice.value)
  if (!source || source.timeline_id === timeline.value?.timeline_id) return
  saving.value = true
  try {
    const result = await api.restoreNLETimelineDraft(props.projectId, props.episodeId, source.timeline_id, { actor: 'creative-workbench-nle' })
    emit('notice', `已从 v${source.version} 创建恢复草稿 v${result.timeline.version}；旧 current 未改变，仍需确认并重编。`)
    await loadWindow(result.timeline.timeline_id)
    emit('versions-changed')
  } catch (error) { emit('error', error.message) } finally { saving.value = false }
}

function manageRenderPolling() {
  const active = ['pending', 'claimed', 'processing'].includes(renderStatus.value)
  if (active && !pollingTimer) pollingTimer = setInterval(() => loadWindow(), 2500)
  if (!active && pollingTimer) {
    clearInterval(pollingTimer); pollingTimer = 0
    if (renderStatus.value) emit('versions-changed')
  }
}

watch(zoom, async (next, previous) => {
  if (!viewport.value) return
  const centerMS = ((viewport.value.scrollLeft + viewport.value.clientWidth / 2 - 96) / previous) * 1000
  await nextTick()
  viewport.value.scrollLeft = Math.max(0, centerMS / 1000 * next - viewport.value.clientWidth / 2 + 96)
  scheduleWindowLoad()
})

onMounted(() => {
  window.addEventListener('pointermove', pointerMove)
  window.addEventListener('pointerup', pointerUp)
  loadWindow('', { start_ms: 0, end_ms: 30000 })
})

onBeforeUnmount(() => {
  pausePlayback()
  window.removeEventListener('pointermove', pointerMove)
  window.removeEventListener('pointerup', pointerUp)
  clearTimeout(scrollTimer)
  clearInterval(pollingTimer)
  for (const player of audioPlayers.values()) player.pause()
  audioPlayers.clear()
})
</script>

<template>
  <article class="nle-shell">
    <header class="nle-head">
      <div><span>LIGHTWEIGHT NLE · MILLISECONDS</span><h3>可播放多轨时间线</h3></div>
      <div class="nle-version-state" v-if="timeline">
        <b :class="timeline.approval_state">v{{ timeline.version }} · {{ timeline.approval_state }}</b>
        <small>current: {{ currentTimelineID === timeline.timeline_id ? '本版本' : currentTimelineID || '—' }}</small>
      </div>
      <button class="render-button" :disabled="!canRender || saving" @click="confirmRender">
        <LoaderCircle v-if="saving" :size="14" class="spin" /><Film v-else :size="14" />确认并重编
      </button>
    </header>

    <div v-if="loading" class="nle-loading"><LoaderCircle :size="18" class="spin" />正在按可视窗口加载代理媒体…</div>
    <template v-else-if="timeline">
      <div class="nle-preview-grid">
        <div class="nle-monitor">
          <video v-if="activeVideo?.proxy_url" ref="video" :key="activeVideo.timeline_item_id" :src="activeVideo.proxy_url" muted playsinline preload="metadata" @loadedmetadata="syncVideo" />
          <div v-else class="proxy-placeholder"><Film :size="34" /><strong>{{ activeVideo ? '代理媒体生成中' : '此时间点没有视频片段' }}</strong><small>播放器不会回退加载高清源文件</small></div>
          <div class="safe-area safe-action"></div><div class="safe-area safe-title"></div>
          <div v-if="activeSubtitle" class="subtitle-preview" :class="subtitleConfig.style" :style="{
            left:`${subtitleConfig.x}%`,top:`${subtitleConfig.y}%`,fontSize:`${subtitleConfig.fontSize}px`,
            color:subtitleConfig.color,background:subtitleConfig.background,fontWeight:subtitleConfig.fontWeight,
          }">{{ subtitleConfig.text }}</div>
          <span class="proxy-badge">PROXY</span>
        </div>
        <aside class="nle-inspector">
          <h4>片段参数</h4>
          <template v-if="selectedItem">
            <strong>{{ NLE_TRACKS.find(track => track.type === selectedItem.track_type)?.label }} · {{ selectedItem.entity_id }}</strong>
            <div class="inspector-fields">
              <label>位置 ms<input :value="selectedItem.timeline_start_ms" type="number" min="0" step="1" :disabled="saving" @change="commitNumeric('timeline_start_ms',$event)" /></label>
              <label>出点 ms<input :value="selectedItem.timeline_end_ms" type="number" min="1" step="1" :disabled="saving" @change="commitNumeric('timeline_end_ms',$event)" /></label>
              <label>素材入点<input :value="selectedItem.source_in_ms" type="number" min="0" step="1" :disabled="saving" @change="commitNumeric('source_in_ms',$event)" /></label>
              <label>素材出点<input :value="selectedItem.source_out_ms ?? ''" type="number" min="1" step="1" :disabled="saving || selectedItem.source_out_ms == null" @change="commitNumeric('source_out_ms',$event)" /></label>
              <label>音量<input :value="selectedItem.volume" type="number" min="0" max="2" step="0.05" :disabled="saving" @change="commitNumeric('volume',$event)" /></label>
              <label>淡入 ms<input :value="selectedItem.fade_in_ms" type="number" min="0" step="1" :disabled="saving" @change="commitNumeric('fade_in_ms',$event)" /></label>
              <label>淡出 ms<input :value="selectedItem.fade_out_ms" type="number" min="0" step="1" :disabled="saving" @change="commitNumeric('fade_out_ms',$event)" /></label>
            </div>
            <div v-if="NLE_TRACKS.find(track => track.type === selectedItem.track_type)?.kind === 'audio'" class="cut-buttons"><button :disabled="saving" @click="applyJCut">J-cut −100ms</button><button :disabled="saving" @click="applyLCut">L-cut +100ms</button></div>
            <div v-if="selectedItem.track_type === 'subtitle'" class="subtitle-controls">
              <label>垂直位置<input :value="objectConfig(selectedItem.transform_config).position_y_pct ?? 84" type="range" min="10" max="92" @change="updateSubtitleTransform({position_y_pct:Number($event.target.value)},'subtitle_position')" /></label>
              <label>水平位置<input :value="objectConfig(selectedItem.transform_config).position_x_pct ?? 50" type="range" min="8" max="92" @change="updateSubtitleTransform({position_x_pct:Number($event.target.value)},'subtitle_position')" /></label>
              <label>字号<input :value="objectConfig(selectedItem.transform_config).font_size_px ?? 28" type="number" min="12" max="72" @change="updateSubtitleTransform({font_size_px:Number($event.target.value)},'subtitle_font_size')" /></label>
              <label><input :checked="objectConfig(selectedItem.transform_config).safe_area_enabled !== false" type="checkbox" @change="updateSubtitleTransform({safe_area_enabled:$event.target.checked},'subtitle_safe_area')" />限制在字幕安全区</label>
              <select :value="objectConfig(selectedItem.effect_config).subtitle_style || 'clean'" @change="updateSubtitleStyle($event.target.value)"><option value="clean">清爽底条</option><option value="outline">描边</option><option value="emphasis">重点强调</option></select>
            </div>
          </template>
          <p v-else>选择片段后可调整入出点、位置、音量、淡入淡出及字幕样式。</p>
        </aside>
      </div>

      <div class="nle-transport">
        <button title="后退" @click="step(-1)"><StepBack :size="15" /></button>
        <button class="play-button" :title="playing?'暂停':'播放'" @click="togglePlayback"><Pause v-if="playing" :size="17" /><Play v-else :size="17" /></button>
        <button title="前进" @click="step(1)"><StepForward :size="15" /></button>
        <select v-model="stepMode" aria-label="定位步长"><option value="frame">逐帧（{{ Math.round(1000/fps) }}ms）</option><option value="100ms">逐 100ms</option></select>
        <strong>{{ formatNLETimecode(currentMS) }}</strong><span>/ {{ formatNLETimecode(durationMS) }}</span>
        <input class="scrub-range" :value="currentMS" type="range" min="0" :max="durationMS" step="1" @input="setPlayhead($event.target.value)" />
      </div>

      <div class="nle-tools">
        <button :class="{active:snapping}" @click="snapping=!snapping"><Magnet :size="14" />吸附 {{ snapping?'开':'关' }}</button>
        <ZoomOut :size="14" /><input v-model.number="zoom" aria-label="时间线缩放" type="range" min="18" max="220" step="2" /><ZoomIn :size="14" /><code>{{ zoom }} px/s</code>
        <span v-if="selection.start !== selection.end">选区 {{ formatNLETimecode(Math.min(selection.start,selection.end)) }}–{{ formatNLETimecode(Math.max(selection.start,selection.end)) }}</span>
        <div class="version-restore"><select v-model="versionChoice"><option v-for="version in timelineVersions" :key="version.timeline_id" :value="version.timeline_id">v{{ version.version }} · {{ version.approval_state }}{{ version.is_current?' · current':'' }}</option></select><button :disabled="saving || versionChoice === timeline.timeline_id" @click="restoreVersion"><RotateCcw :size="13" />恢复为新草稿</button></div>
      </div>

      <div ref="viewport" class="nle-timeline-viewport" @scroll="scheduleWindowLoad">
        <div class="nle-timeline-content" :style="{width:`${timelineWidth + 96}px`}" @pointerdown="beginSelection" @pointermove="selectionMove" @pointerup="endSelection">
          <div class="nle-ruler"><b>轨道 / 时间</b><div class="ruler-canvas"><span v-for="tick in rulerTicks" :key="tick" :style="{left:`${tick/1000*zoom}px`}">{{ formatNLETimecode(tick) }}</span></div></div>
          <div v-for="lane in lanes" :key="lane.type" class="nle-lane">
            <b><component :is="lane.kind==='video'?Film:lane.kind==='audio'?Volume2:Scissors" :size="13" />{{ lane.label }}</b>
            <div class="lane-canvas">
              <div v-for="item in lane.entries" :key="item.timeline_item_id" class="nle-clip" :class="[lane.type,{selected:item.timeline_item_id===selectedItemID,pending:item.proxy_status==='pending'}]" :style="itemStyle(item)" @pointerdown.stop="beginGesture($event,item,'move')" @click.stop="selectClip(item)">
                <i class="trim-handle left" @pointerdown.stop="beginGesture($event,item,'trim-start')"></i>
                <img v-if="lane.kind==='audio' && item.waveform_url" :src="item.waveform_url" loading="lazy" alt="音频波形" />
                <span><strong>{{ item.subtitle_text || item.entity_id }}</strong><small v-if="lane.kind==='audio' && !item.waveform_url">波形生成中</small><small v-if="lane.kind==='video' && item.proxy_status==='pending'">代理生成中</small></span>
                <i class="trim-handle right" @pointerdown.stop="beginGesture($event,item,'trim-end')"></i>
              </div>
            </div>
          </div>
          <div v-if="selection.start !== selection.end" class="range-selection" :style="{left:`${96+Math.min(selection.start,selection.end)/1000*zoom}px`,width:`${Math.abs(selection.end-selection.start)/1000*zoom}px`}"></div>
          <div class="playhead" :style="timeStyle(currentMS)"><i></i></div>
        </div>
      </div>

      <footer class="nle-status">
        <span><Check :size="13" />可视窗口 {{ formatNLETimecode(page.window_start_ms) }}–{{ formatNLETimecode(page.window_end_ms) }} · 已载入 {{ items.length }}/{{ page.total }} 个片段</span>
        <button v-if="page.has_more" class="load-page-button" :disabled="loadingMore" @click="loadMore"><LoaderCircle v-if="loadingMore" :size="12" class="spin" />加载当前窗口下一页</button>
        <span v-if="renderStatus" :class="renderStatus">渲染 {{ renderStatus }}<template v-if="page.render_job"> · {{ Math.round(page.render_job.progress) }}%</template><template v-if="page.render_job?.error_message"> · {{ page.render_job.error_message }}</template></span>
        <span v-if="timeline.approval_state==='render_failed'" class="failed">渲染失败，旧 current 未被替换</span>
      </footer>
    </template>
  </article>
</template>

<style scoped>
.nle-shell{overflow:hidden;border:1px solid #dce2eb!important;background:#0d1422!important;color:#dce6f7}.nle-head{background:#141e30;border-color:#27334a!important}.nle-head h3{color:#f3f6fb}.nle-head>div:first-child span{color:#7f95b7}.nle-version-state{margin-left:auto;display:grid;text-align:right}.nle-version-state b{font-size:11px}.nle-version-state b.draft{color:#f0c76e}.nle-version-state b.rendering{color:#7eb9ff}.nle-version-state b.render_failed{color:#ff8989}.nle-version-state b.approved{color:#7be0b3}.nle-version-state small{color:#8290a6;font-size:9px}.render-button{height:32px;display:flex;align-items:center;gap:5px;border:0;border-radius:6px;padding:0 10px;color:#fff;background:#496dde;font-size:10px}.render-button:disabled{opacity:.38}.nle-loading{height:180px;display:flex;place-items:center;justify-content:center;gap:8px;color:#9cabc1}.nle-preview-grid{display:grid;grid-template-columns:minmax(0,1fr) 265px;gap:1px;background:#27334a}.nle-monitor{position:relative;aspect-ratio:16/9;max-height:385px;display:grid;place-items:center;overflow:hidden;background:#03070e}.nle-monitor video{width:100%;height:100%;object-fit:contain}.proxy-placeholder{display:grid;justify-items:center;gap:7px;color:#7e8ba0}.proxy-placeholder strong{font-size:12px}.proxy-placeholder small{font-size:9px}.proxy-badge{position:absolute;right:9px;top:9px;padding:3px 5px;border:1px solid #7c8da7;border-radius:4px;color:#b9c5d8;background:#101827cc;font-size:8px}.safe-area{position:absolute;pointer-events:none;border:1px dashed #f1c96c66}.safe-action{inset:5%}.safe-title{inset:10%}.subtitle-preview{position:absolute;max-width:76%;transform:translate(-50%,-50%);border-radius:4px;padding:4px 10px;text-align:center;line-height:1.3;text-shadow:0 1px 2px #000;white-space:pre-wrap}.subtitle-preview.outline{background:transparent!important;text-shadow:-1px -1px #000,1px -1px #000,-1px 1px #000,1px 1px #000}.subtitle-preview.emphasis{letter-spacing:.04em}.nle-inspector{padding:12px;background:#111a29}.nle-inspector h4{margin:0 0 8px;font-size:11px}.nle-inspector>strong{display:block;overflow:hidden;color:#b9c9e2;font-size:10px;text-overflow:ellipsis;white-space:nowrap}.nle-inspector>p{color:#77869e;font-size:10px;line-height:1.6}.inspector-fields{display:grid;grid-template-columns:1fr 1fr;gap:6px;margin-top:9px}.inspector-fields label,.subtitle-controls label{display:grid;gap:3px;color:#8190a7;font-size:8px}.inspector-fields input,.subtitle-controls input,.subtitle-controls select{min-width:0;border:1px solid #334159;border-radius:4px;padding:5px;color:#dce6f7;background:#182336;font:9px inherit}.cut-buttons{display:flex;gap:5px;margin-top:7px}.cut-buttons button,.version-restore button{border:1px solid #3a4a66;border-radius:5px;padding:5px;color:#b9c9e3;background:#1a2840;font-size:9px}.subtitle-controls{display:grid;gap:6px;margin-top:8px}.nle-transport{display:flex;align-items:center;gap:5px;padding:8px 10px;border-top:1px solid #2b3548;border-bottom:1px solid #2b3548;background:#111a28}.nle-transport button{width:28px;height:28px;display:grid;place-items:center;border:1px solid #35415a;border-radius:5px;color:#bdc8da;background:#1b2639}.nle-transport .play-button{color:#fff;background:#486ad6}.nle-transport select{border:1px solid #35415a;border-radius:5px;padding:5px;color:#c2cde0;background:#182236;font-size:9px}.nle-transport strong{margin-left:6px;color:#fff;font:12px ui-monospace,monospace}.nle-transport>span{color:#77869b;font:10px ui-monospace,monospace}.scrub-range{min-width:90px;flex:1;accent-color:#6f8cff}.nle-tools{min-height:38px;display:flex;align-items:center;gap:6px;padding:5px 10px;color:#91a0b5;background:#151f30;font-size:9px}.nle-tools>button{display:flex;align-items:center;gap:4px;border:1px solid #33415a;border-radius:5px;padding:5px 7px;color:#9cacc4;background:#1a2639}.nle-tools>button.active{color:#91a9ff;border-color:#5573d3;background:#1d2c4e}.nle-tools>input{width:120px;accent-color:#6f8cff}.nle-tools code{color:#8392aa}.nle-tools>span{margin-left:8px;color:#e1bd6a}.version-restore{margin-left:auto;display:flex;gap:4px}.version-restore select{max-width:165px;border:1px solid #33415a;border-radius:5px;color:#afbdd1;background:#182235;font-size:9px}.nle-timeline-viewport{max-height:390px;overflow:auto;background:#0d1420;scrollbar-color:#3b4860 #131d2c}.nle-timeline-content{position:relative;min-width:100%;user-select:none}.nle-ruler,.nle-lane{display:grid;grid-template-columns:96px 1fr}.nle-ruler{position:sticky;top:0;z-index:8;height:30px;border-bottom:1px solid #334057;background:#121b2a}.nle-ruler>b,.nle-lane>b{position:sticky;left:0;z-index:6;display:flex;align-items:center;gap:5px;padding-left:9px;border-right:1px solid #303c52;color:#8e9db3;background:#151f2f;font-size:9px}.ruler-canvas,.lane-canvas{position:relative}.ruler-canvas span{position:absolute;top:8px;color:#77879e;font:8px ui-monospace,monospace;transform:translateX(3px)}.ruler-canvas span:before{content:'';position:absolute;left:-3px;top:13px;height:7px;border-left:1px solid #40506b}.nle-lane{height:48px;border-bottom:1px solid #202b3d}.lane-canvas{background:repeating-linear-gradient(90deg,#101827 0,#101827 calc(1s),#111b2b calc(1s));background-size:calc(v-bind(zoom) * 1px) 100%}.nle-clip{position:absolute;top:5px;bottom:5px;display:flex;align-items:center;overflow:hidden;border:1px solid #5572c8;border-radius:5px;color:#dbe5f6;background:#334f9a;cursor:grab}.nle-clip.dialogue{border-color:#3a9b78;background:#276f58}.nle-clip.narration{border-color:#59a1a6;background:#347176}.nle-clip.bgm{border-color:#8c66be;background:#65418e}.nle-clip.ambience{border-color:#a37652;background:#76523a}.nle-clip.sound_effect{border-color:#c27c47;background:#8a522c}.nle-clip.subtitle{border-color:#818fa4;background:#48576c}.nle-clip.selected{outline:2px solid #eef4ff;outline-offset:1px;z-index:3}.nle-clip.pending{background:repeating-linear-gradient(135deg,#35435c,#35435c 6px,#2b364b 6px,#2b364b 12px)}.nle-clip img{position:absolute;width:100%;height:100%;object-fit:cover;opacity:.35}.nle-clip>span{position:relative;z-index:1;min-width:0;padding:0 8px}.nle-clip strong,.nle-clip small{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.nle-clip strong{font-size:8px}.nle-clip small{color:#d4def0aa;font-size:7px}.trim-handle{position:absolute;z-index:3;top:0;bottom:0;width:6px;cursor:ew-resize}.trim-handle.left{left:0}.trim-handle.right{right:0}.trim-handle:hover{background:#fff8}.playhead{position:absolute;z-index:7;top:0;bottom:0;width:1px;background:#ff6b65;pointer-events:none}.playhead i{position:absolute;left:-4px;top:0;width:9px;height:9px;clip-path:polygon(0 0,100% 0,50% 100%);background:#ff6b65}.range-selection{position:absolute;z-index:2;top:30px;bottom:0;border:1px solid #e4c26388;background:#e4c2631f;pointer-events:none}.nle-status{min-height:31px;display:flex;align-items:center;gap:14px;padding:5px 10px;color:#78879e;background:#111a28;font-size:8px}.nle-status span{display:flex;align-items:center;gap:4px}.nle-status .succeeded{color:#6fdaae}.nle-status .failed,.nle-status .timeout{color:#ff8c8c}.spin{animation:spin 1s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}@media(max-width:900px){.nle-preview-grid{grid-template-columns:1fr}.nle-inspector{max-height:240px;overflow:auto}.nle-tools{flex-wrap:wrap}.version-restore{margin-left:0;width:100%}.nle-status{align-items:flex-start;flex-direction:column}}
.load-page-button{display:flex;align-items:center;gap:4px;border:1px solid #3a4962;border-radius:4px;padding:4px 6px;color:#aebbd0;background:#172337;font-size:8px}.load-page-button:disabled{opacity:.5}
</style>
