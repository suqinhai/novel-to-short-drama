<script setup>
import { computed } from 'vue'
import { AlertTriangle, FlaskConical, Image as ImageIcon, Music2, Video } from 'lucide-vue-next'
import ReviewValue from './ReviewValue.vue'
import StatusBadge from './StatusBadge.vue'

const props = defineProps({ content: { type: Object, required: true } })

const artifactLabels = {
  story_bible: '故事圣经', season_outline: '整季分集大纲', episode_script: '单集剧本', storyboard: '分镜设计',
  visual_asset: '视觉资产', storyboard_image: '分镜图片', shot_video: '镜头视频', dialogue_audio: '对白音频',
  voice_profile: '声音档案', final_review: '成片终审', publication_metadata: '发布信息',
}
const title = computed(() => props.content.artifact?.title || props.content.artifact?.canonical_name || artifactLabels[props.content.artifact_type] || '审核内容')
const rootEntries = computed(() => Object.entries(props.content.artifact || {}).filter(([key]) => ![
  'id', 'project_id', 'created_at', 'updated_at', 'storage_url', 'original_url', 'local_path', 'thumbnail_url',
  'image_url', 'cover_url', 'sample_audio_url', 'content_hash', 'provider_task_id', 'trace_id',
].includes(key)))
const keyLabels = {
  characters: '人物设定', relationships: '人物关系', locations: '地点设定', world_rules: '世界规则', timeline: '故事时间线',
  key_events: '关键事件', foreshadowing: '伏笔', episodes: '分集大纲', scene_details: '剧本场景', scenes: '场景原稿',
  shots: '分镜镜头', master: '最终成片', qc_report: '质量检查报告', final_review: '人工终审记录',
  source_text: '原始台词', normalized_text: '规范台词', emotion: '情绪要求', performance_instruction: '表演指令',
  actual_duration_ms: '实际时长', requested_speed: '语速', auto_qc_status: '自动质检结论', auto_qc_report: '自动质检详情',
  generation_version: '生成版本', review_comment: '审核意见', rejection_reason: '拒绝原因', prompt_adjustment: '生成调整',
  model: '生成模型', format: '文件格式', status: '产物状态', bitrate: '码率', channels: '声道数',
  provider: '服务提供方', sample_rate: '采样率', peak_db: '峰值', loudness_lufs: '响度', silence_ratio: '静音比例',
}
const label = (key) => keyLabels[key] || key.replaceAll('_', ' ')
const focusKeys = new Set(['source_text', 'normalized_text', 'emotion', 'performance_instruction', 'actual_duration_ms', 'requested_speed', 'auto_qc_status'])
const explicitTechnicalKeys = new Set(['model', 'format', 'status', 'bitrate', 'channels', 'provider', 'sample_rate', 'waveform_url', 'dialogue_type', 'is_current', 'peak_db', 'loudness_lufs', 'silence_ratio'])
const isTechnicalKey = (key) => explicitTechnicalKeys.has(key) || key.endsWith('_id') || key.endsWith('_url')
const mainEntries = computed(() => rootEntries.value.filter(([key]) => !focusKeys.has(key) && !isTechnicalKey(key)))
const technicalEntries = computed(() => rootEntries.value.filter(([key]) => isTechnicalKey(key)))
const focus = computed(() => {
  const artifact = props.content.artifact || {}
  return {
    sourceText: artifact.source_text || artifact.normalized_text || '',
    normalizedText: artifact.source_text && artifact.normalized_text !== artifact.source_text ? artifact.normalized_text : '',
    emotion: artifact.emotion || '',
    instruction: artifact.performance_instruction || '',
    duration: Number.isFinite(Number(artifact.actual_duration_ms)) ? `${(Number(artifact.actual_duration_ms) / 1000).toFixed(2)} 秒` : '',
    speed: artifact.requested_speed ? `${artifact.requested_speed}×` : '',
    qcStatus: artifact.auto_qc_status || artifact.qc_status || '',
  }
})
const hasFocus = computed(() => Object.values(focus.value).some(Boolean))
const qcPassed = computed(() => ['passed', 'approved', 'succeeded'].includes(String(focus.value.qcStatus).toLowerCase()))
</script>

<template>
  <div class="review-content-viewer">
    <div v-if="content.test_mode" class="review-test-warning">
      <FlaskConical :size="18" /><div><strong>测试模式产物</strong><span>内容可能包含 Mock 占位数据，请勿按正式成片标准直接通过。</span></div>
    </div>

    <div class="review-artifact-heading">
      <div><span>{{ artifactLabels[content.artifact_type] || content.artifact_type }}</span><h3>{{ title }}</h3></div>
      <StatusBadge :status="content.review_status" />
    </div>

    <section v-if="hasFocus" class="review-focus-card">
      <header><span>审核重点</span><strong>先确认内容与表现，再查看技术参数</strong></header>
      <blockquote v-if="focus.sourceText">“{{ focus.sourceText }}”</blockquote>
      <p v-if="focus.normalizedText" class="review-normalized-text">规范文本：{{ focus.normalizedText }}</p>
      <div class="review-focus-chips">
        <span v-if="focus.emotion"><b>情绪</b>{{ focus.emotion }}</span>
        <span v-if="focus.duration"><b>时长</b>{{ focus.duration }}</span>
        <span v-if="focus.speed"><b>语速</b>{{ focus.speed }}</span>
        <span v-if="focus.qcStatus" :class="{ passed: qcPassed, warning: !qcPassed }"><b>自动质检</b>{{ qcPassed ? '通过' : focus.qcStatus }}</span>
      </div>
      <div v-if="focus.instruction" class="review-performance-note"><span>表演指令</span><p>{{ focus.instruction }}</p></div>
    </section>

    <div v-if="content.media?.length" class="review-media-grid">
      <article v-for="(media, index) in content.media" :key="index" class="review-media-card">
        <div class="review-media-label">
          <ImageIcon v-if="media.kind === 'image'" :size="17" /><Video v-else-if="media.kind === 'video'" :size="17" /><Music2 v-else :size="17" />
          <strong>{{ media.label }}</strong>
        </div>
        <img v-if="media.kind === 'image' && media.media_url" :src="media.preview_url || media.media_url" :alt="media.label" />
        <video v-else-if="media.kind === 'video' && media.media_url" :src="media.media_url" :poster="media.preview_url || undefined" controls preload="metadata">当前浏览器不支持视频播放。</video>
        <audio v-else-if="media.kind === 'audio' && media.media_url" :src="media.media_url" controls preload="metadata">当前浏览器不支持音频播放。</audio>
        <div v-else class="review-media-missing"><AlertTriangle :size="18" />媒体文件暂不可访问</div>
      </article>
    </div>

    <div class="review-content-sections">
      <section v-for="([key, value]) in mainEntries" :key="key" class="review-content-section">
        <h4>{{ label(key) }}</h4>
        <ReviewValue :name="key" :value="value" />
      </section>
    </div>

    <details v-if="technicalEntries.length" class="review-technical-details">
      <summary>技术详情 <span>{{ technicalEntries.length }} 项</span></summary>
      <div>
        <section v-for="([key, value]) in technicalEntries" :key="key" class="review-content-section">
          <h4>{{ label(key) }}</h4>
          <ReviewValue :name="key" :value="value" />
        </section>
      </div>
    </details>
  </div>
</template>
