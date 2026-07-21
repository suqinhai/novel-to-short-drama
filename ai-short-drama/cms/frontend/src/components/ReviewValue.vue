<script setup>
import { computed } from 'vue'

const props = defineProps({
  value: { default: null },
  name: { type: String, default: '' },
  depth: { type: Number, default: 0 },
})

const labels = {
  characters: '人物设定', relationships: '人物关系', locations: '地点设定', world_rules: '世界规则', timeline: '故事时间线',
  key_events: '关键事件', foreshadowing: '伏笔', episodes: '分集大纲', opening_hook: '开场钩子', ending_hook: '结尾钩子',
  story_goal: '本集目标', main_conflict: '核心冲突', plot_points: '情节点', climax: '高潮', logline: '一句话梗概',
  scene_details: '场景正文', scenes: '场景', scene_number: '场次', scene_purpose: '场景目的', actions: '动作描述',
  dialogue_rows: '对白', dialogues: '对白', narration: '旁白', emotional_change: '情绪变化', speaker_name: '说话人', text: '台词',
  shots: '镜头列表', shot_number: '镜号', shot_order: '镜头顺序', shot_size: '景别', camera_angle: '机位角度',
  camera_motion: '运镜', composition: '构图', action_description: '画面动作', facial_expression: '表情', subtitle_text: '字幕',
  narration_text: '旁白', lighting: '灯光', atmosphere: '氛围', sound_effect_hint: '音效建议', bgm_hint: '配乐建议',
  transition_type: '转场', visual_prompt_base: '图片提示词', video_prompt_base: '视频提示词', final_prompt: '最终提示词',
  negative_prompt: '负面提示词', prompt: '生成提示词', video_prompt: '视频提示词', source_text: '原始对白', normalized_text: '配音文本',
  emotion: '情绪', performance_instruction: '表演指令', title: '标题', description: '简介', hashtags: '话题标签',
  title_candidates: '标题候选', cover_candidates: '封面候选', content_declaration: '内容声明', platform_options: '平台选项',
  master: '成片信息', qc_report: '质量检查报告', final_review: '终审记录', technical_report: '技术质检',
  subtitle_report: '字幕质检', content_report: '内容质检', compliance_report: '合规质检', blocking_issues: '阻断问题',
  warnings: '风险提示', recommended_actions: '修改建议', routing_decisions: '退回路径', quality_report: '质量报告',
  continuity_report: '连续性报告', continuity_in: '承接信息', continuity_out: '后续状态', personality: '性格',
  appearance: '外貌', identity: '身份', canonical_name: '姓名', aliases: '别名', evidence: '原文依据', quote: '原文摘录',
  participants: '参与人物', description: '描述', importance: '重要度', adaptation_strategy: '改编策略', generation_config: '生成配置',
  status: '产物状态', review_status: '审核状态', version: '版本', episode_number: '集数', estimated_duration_seconds: '预计时长（秒）',
  duration_seconds: '时长（秒）', actual_duration_seconds: '实际时长（秒）', actual_duration_ms: '实际时长（毫秒）',
  auto_qc_status: '自动质检状态', auto_qc_report: '自动质检报告', review_comment: '审核意见', rejection_reason: '拒绝原因',
  provider: '生成服务', model: '模型', aspect_ratio: '画幅', width: '宽度', height: '高度', fps: '帧率', codec: '编码',
  source_chapter_ids: '来源章节', source_chunk_ids: '来源文本块', source_event_ids: '来源事件', character_ids: '人物引用',
  location_name: '地点', time_of_day: '时间', interior_exterior: '内外景', overall_score: '综合得分', severity: '质检结论',
  platform: '发布平台', visibility: '可见范围', scheduled_at: '计划发布时间', lock_status: '锁定状态', speaking_style: '说话风格',
  apparent_age: '声音年龄', gender: '性别', language: '语言', pitch: '音高', speed: '语速', volume: '音量',
}

const hiddenKeys = new Set([
  'id', 'project_id', 'created_at', 'updated_at', 'provider_task_id', 'storage_url', 'original_url', 'local_path',
  'thumbnail_url', 'image_url', 'cover_url', 'sample_audio_url', 'content_hash', 'trace_id',
])

const isArray = computed(() => Array.isArray(props.value))
const isObject = computed(() => props.value !== null && typeof props.value === 'object' && !isArray.value)
const objectEntries = computed(() => isObject.value
  ? Object.entries(props.value).filter(([key, value]) => !hiddenKeys.has(key) && value !== null && value !== '')
  : [])
const arrayObjects = computed(() => isArray.value && props.value.some((item) => item !== null && typeof item === 'object'))

function label(key) {
  return labels[key] || key.replaceAll('_', ' ')
}

function primitive(value) {
  if (value === true) return '是'
  if (value === false) return '否'
  if (value === null || value === undefined || value === '') return '暂无'
  return String(value)
}

function itemTitle(item, index) {
  if (!item || typeof item !== 'object') return `第 ${index + 1} 项`
  return item.canonical_name || item.name || item.title || item.speaker_name ||
    (item.episode_number ? `第 ${item.episode_number} 集` : '') ||
    (item.scene_number ? `第 ${item.scene_number} 场` : '') ||
    (item.shot_number ? `镜头 ${item.shot_number}` : '') ||
    item.description || `第 ${index + 1} 项`
}
</script>

<template>
  <div class="review-value" :class="[`depth-${Math.min(depth, 3)}`, { 'is-array': isArray, 'is-object': isObject }]">
    <template v-if="isArray">
      <p v-if="value.length === 0" class="review-empty-value">暂无内容</p>
      <div v-else-if="!arrayObjects" class="review-chip-list">
        <span v-for="(item, index) in value" :key="index">{{ primitive(item) }}</span>
      </div>
      <div v-else class="review-object-list">
        <article v-for="(item, index) in value" :key="item?.id || item?.character_id || item?.scene_id || item?.shot_id || index" class="review-object-card">
          <header>{{ itemTitle(item, index) }}</header>
          <ReviewValue :value="item" :depth="depth + 1" />
        </article>
      </div>
    </template>
    <template v-else-if="isObject">
      <p v-if="objectEntries.length === 0" class="review-empty-value">暂无内容</p>
      <dl v-else class="review-field-list">
        <div v-for="([key, item]) in objectEntries" :key="key" :class="{ nested: item !== null && typeof item === 'object' }">
          <dt>{{ label(key) }}</dt>
          <dd><ReviewValue :name="key" :value="item" :depth="depth + 1" /></dd>
        </div>
      </dl>
    </template>
    <p v-else class="review-primitive">{{ primitive(value) }}</p>
  </div>
</template>
