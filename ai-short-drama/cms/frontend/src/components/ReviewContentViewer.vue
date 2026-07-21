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
}
const label = (key) => keyLabels[key] || key.replaceAll('_', ' ')
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
      <section v-for="([key, value]) in rootEntries" :key="key" class="review-content-section">
        <h4>{{ label(key) }}</h4>
        <ReviewValue :name="key" :value="value" />
      </section>
    </div>
  </div>
</template>
