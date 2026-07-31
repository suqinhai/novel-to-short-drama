<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  Activity, ArrowLeft, AudioLines, CheckCircle2, ChevronDown, Clock3, Film,
  GitCompareArrows, GripVertical, History, Layers3, MessageSquareText, Music2,
  Pause, Play, RefreshCw, Scissors, ShieldAlert, Sparkles, Split, Subtitles,
  TimerReset, Undo2, UsersRound, Volume2, WandSparkles,
} from 'lucide-vue-next'
import { api } from '../services/api'
import {
  dialogueConversionPlan, dialogueEditPlan, dialoguesForScene, exactDialogueRebuildRange,
  issueEditLink, normalizeWorkbench, sceneDragPlan, shotsForScene, soundTrackLabels,
  soundStyleReplacementPayload, templateApplyPayload, templateLabels, timelineLanes,
  timingValidationItems,
} from '../services/creativeWorkbench'

const route = useRoute()
const projectId = computed(() => route.params.projectId)
const episodeId = computed(() => route.params.episodeId)
const activeTab = ref('script')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const workspace = ref(normalizeWorkbench())
const templates = ref([])
const selectedSceneId = ref('')
const selectedTemplateId = ref('')
const templateScope = ref('episode')
const dialogueDrafts = reactive({})
const draggedSceneId = ref('')
const timingReport = ref(null)
const pendingPlan = ref(null)
const selectedTimelineId = ref('')
const soundStyleTarget = ref('cinematic_noir')
const commentTargetKey = ref('')
const commentTimecodeMS = ref('')
const commentBody = ref('')

const tabs = [
  ['script', '剧本与节拍', Layers3],
  ['storyboard', '分镜故事板', Film],
  ['sound', '口型与声音', AudioLines],
  ['timeline', '剪辑时间线', Scissors],
  ['quality', '连续性与 QC', ShieldAlert],
  ['versions', '候选与版本', History],
]
const selectedScene = computed(() => workspace.value.scenes.find(item => item.scene_id === selectedSceneId.value) || workspace.value.scenes[0])
const sceneDialogues = computed(() => dialoguesForScene(workspace.value, selectedScene.value?.scene_id))
const sceneShots = computed(() => shotsForScene(workspace.value, selectedScene.value?.scene_id))
const lanes = computed(() => timelineLanes(workspace.value.timeline_items))
const maxTimelineMS = computed(() => Math.max(1, ...workspace.value.timeline_items.map(item => Number(item.timeline_end_ms || 0))))
const allIssues = computed(() => [
  ...workspace.value.dialogue_timing_issues.map(item => ({ ...item, issue_kind: 'dialogue_timing' })),
  ...workspace.value.visual_qc_issues.map(item => ({ ...item, issue_kind: 'visual_qc' })),
  ...workspace.value.quality_issues.map(item => ({ ...item, issue_kind: 'quality' })),
])
const currentTimeline = computed(() => workspace.value.timeline_versions.find(item => item.is_current))
const selectedTemplate = computed(() => templates.value.find(item => item.editing_template_version_id === selectedTemplateId.value))
const commentTargets = computed(() => {
  const scene = selectedScene.value
  if (!scene) return []
  return [
    { key: `scene:${scene.scene_id}`, type: 'scene', id: scene.scene_id, version: Number(scene.version || 1), label: `场景 ${scene.scene_number}` },
    ...sceneDialogues.value.map(item => ({ key: `dialogue:${item.dialogue_id}`, type: 'dialogue', id: item.dialogue_id, version: Number(item.version || 1), label: `台词 · ${item.speaker_name}` })),
    ...sceneShots.value.map(item => ({ key: `shot:${item.shot_id}`, type: 'shot', id: item.shot_id, version: Number(item.generation_version || 1), label: `镜头 ${item.shot_number}` })),
  ]
})

function templateKey(item) {
  return item.template_key || item.template?.template_key || ''
}

function milliseconds(value) {
  const total = Number(value || 0)
  const minutes = Math.floor(total / 60000)
  const seconds = ((total % 60000) / 1000).toFixed(1)
  return `${minutes}:${seconds.padStart(4, '0')}`
}

function score(value) {
  return Math.round(Number(value || 0))
}

function issueTime(issue) {
  return issue.timecode_ms ?? issue.start_ms ?? issue.location?.timecode_ms ?? 0
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [payload, templateRows] = await Promise.all([
      api.getCreativeWorkbench(projectId.value, episodeId.value),
      api.getEditingTemplates(projectId.value),
    ])
    workspace.value = normalizeWorkbench(payload)
    templates.value = templateRows || []
    selectedSceneId.value ||= workspace.value.scenes[0]?.scene_id || ''
    const binding = workspace.value.template_bindings.find(item => item.is_current && item.episode_id === episodeId.value)
      || workspace.value.template_bindings.find(item => item.is_current && !item.episode_id)
    selectedTemplateId.value = binding?.editing_template_version_id
      || currentTimeline.value?.editing_template_version_id
      || templates.value[0]?.editing_template_version_id
      || ''
    selectedTimelineId.value = currentTimeline.value?.timeline_id || ''
    commentTargetKey.value ||= commentTargets.value[0]?.key || ''
    for (const item of workspace.value.dialogues) dialogueDrafts[item.dialogue_id] = item.text
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function createPlan(payload) {
  saving.value = true
  error.value = ''
  try {
    pendingPlan.value = await api.createChangePlan(projectId.value, payload)
    notice.value = `已创建局部修改计划 ${pendingPlan.value.change_plan_id}；确认前不会写入正式版本。`
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

async function saveDialogue(dialogue) {
  const nextText = String(dialogueDrafts[dialogue.dialogue_id] || '').trim()
  if (!nextText || nextText === dialogue.text) return
  await createPlan(dialogueEditPlan(dialogue, nextText))
}

async function convertDialogue(dialogue, mode) {
  await createPlan(dialogueConversionPlan(dialogue, mode))
}

async function dropScene(target) {
  const source = workspace.value.scenes.find(item => item.scene_id === draggedSceneId.value)
  draggedSceneId.value = ''
  if (!source || source.scene_id === target.scene_id) return
  await createPlan(sceneDragPlan(source, target.scene_number))
}

async function retimeShot(shot, deltaSeconds) {
  const next = Math.max(.4, Number(shot.duration_seconds) + deltaSeconds)
  await createPlan({
    instruction: `将镜头 ${shot.shot_id} 调整为 ${next.toFixed(1)} 秒，只重建该镜头与剪辑区间`,
    target: { entity_type: 'shot', entity_id: shot.shot_id, version: Number(shot.generation_version || 1) },
    allowed_fields: ['duration_seconds'],
    changes: [{ operation: 'replace', field: 'duration_seconds', value: next }],
    must_preserve: ['人物身份', '构图意图', '动作连续性'],
  })
}

async function markShotStructure(shot, operation) {
  const label = operation === 'split' ? '拆分' : operation === 'merge' ? '与下一镜合并' : '换序'
  await createPlan({
    instruction: `${label}镜头 ${shot.shot_id}；保持连续性账本与相邻首尾帧约束，生成结构化局部修改`,
    target: { entity_type: 'shot', entity_id: shot.shot_id, version: Number(shot.generation_version || 1) },
    allowed_fields: ['action_description'],
    changes: [{
      operation: 'regenerate', field: 'action_description',
      value: `${shot.action_description} [workbench:${operation}]`,
    }],
    must_preserve: ['人物状态', '动作接力', '镜头轴线'],
  })
}

async function validateTimings() {
  const items = timingValidationItems(workspace.value)
  if (!items.length) {
    error.value = '当前集尚无对白口型时间记录。'
    return
  }
  saving.value = true
  try {
    const result = await api.validateDialogueTimings(projectId.value, episodeId.value, {
      items, tolerance_ms: 120, persist: true, actor: 'creative-workbench',
    })
    timingReport.value = result.report
    notice.value = `口型与对白轮次校验完成：${result.report.issues.length} 个问题。`
    await load()
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

async function applyTemplate() {
  if (!selectedTemplateId.value) return
  saving.value = true
  error.value = ''
  try {
    const timeline = await api.applyEditingTemplate(
      projectId.value, episodeId.value,
      templateApplyPayload(selectedTemplateId.value, templateScope.value),
    )
    selectedTimelineId.value = timeline.timeline_id
    notice.value = `已应用${selectedTemplate.value?.name || '剪辑模板'}并创建时间线 v${timeline.version}，旧版本仍可查看和恢复。`
    await load()
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

async function restoreTimeline(item) {
  saving.value = true
  try {
    const restored = await api.restoreTimelineVersion(projectId.value, episodeId.value, item.timeline_id, {
      actor: 'creative-workbench',
    })
    notice.value = `已从 v${item.version} 创建恢复版本 v${restored.version}；历史记录未被覆盖。`
    await load()
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

async function replaceSoundStyle() {
  if (!soundStyleTarget.value.trim()) return
  saving.value = true
  error.value = ''
  try {
    const result = await api.replaceEpisodeSoundStyle(
      projectId.value, episodeId.value, soundStyleReplacementPayload(soundStyleTarget.value),
    )
    notice.value = `整集声音风格已替换为 ${result.to_style_group}，并创建时间线 v${result.timeline.version}；原声音资产、cue 与时间线保留为历史版本。`
    await load()
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

async function submitComment() {
  const target = commentTargets.value.find(item => item.key === commentTargetKey.value)
  const body = commentBody.value.trim()
  if (!target || !body) return
  const payload = {
    entity_type: target.type, entity_id: target.id, entity_version: target.version,
    body, author: 'creative-workbench',
  }
  if (commentTimecodeMS.value !== '') {
    const start = Math.max(0, Number(commentTimecodeMS.value))
    payload.timecode_start_ms = start
    payload.timecode_end_ms = start + 1
  }
  saving.value = true
  try {
    await api.createChangeComment(projectId.value, payload)
    commentBody.value = ''
    notice.value = `评论已绑定到 ${target.label}${commentTimecodeMS.value === '' ? '' : ` @ ${commentTimecodeMS.value}ms`}。`
    await load()
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

function openPlan() {
  if (!pendingPlan.value) return
  route.meta
  window.location.assign(`/projects/${projectId.value}/local-edit?entity_type=${pendingPlan.value.plan.target.entity_type}&entity_id=${encodeURIComponent(pendingPlan.value.plan.target.entity_id)}&version=${pendingPlan.value.plan.target.version}`)
}

onMounted(load)
</script>

<template>
  <section class="workbench-shell">
    <header class="workbench-header">
      <div>
        <RouterLink :to="`/projects/${projectId}`" class="back-link"><ArrowLeft :size="15" />返回项目</RouterLink>
        <div class="workbench-title"><div><span>UNIFIED CREATIVE DESK</span><h2>{{ workspace.episode?.title || '统一剧本分镜创作工作台' }}</h2></div><b>EP {{ workspace.episode?.episode_number || '—' }}</b></div>
        <p>诊断、节奏、候选、剧本、表演、连续性、声画与 QC 共用同一版本链。</p>
      </div>
      <div class="header-actions">
        <div class="template-control">
          <label><span>剪辑策略</span><select v-model="selectedTemplateId"><option v-for="item in templates" :key="item.editing_template_version_id" :value="item.editing_template_version_id">{{ templateLabels[templateKey(item)] || item.name }} · v{{ item.version }}</option></select></label>
          <label><span>覆盖范围</span><select v-model="templateScope"><option value="episode">本集覆盖</option><option value="project">项目默认</option></select></label>
          <button :disabled="saving || !selectedTemplateId" @click="applyTemplate"><WandSparkles :size="15" />应用并新建时间线</button>
        </div>
        <button class="refresh-button" :disabled="loading" title="刷新" @click="load"><RefreshCw :size="17" /></button>
      </div>
    </header>

    <div v-if="notice" class="workbench-notice"><CheckCircle2 :size="16" />{{ notice }}<button v-if="pendingPlan" @click="openPlan">查看并确认</button></div>
    <div v-if="error" class="error-banner">{{ error }}</div>
    <div v-if="loading" class="panel padded">正在装配统一工作台数据…</div>

    <template v-else>
      <div class="workbench-metrics">
        <article><Activity :size="18" /><span>改编质量</span><strong>{{ score(workspace.diagnostic?.total_score) || '—' }}</strong></article>
        <article><Layers3 :size="18" /><span>场 / 镜</span><strong>{{ workspace.scenes.length }} / {{ workspace.shots.length }}</strong></article>
        <article><AudioLines :size="18" /><span>对白 / 声音提示</span><strong>{{ workspace.dialogues.length }} / {{ workspace.sound_cues.length }}</strong></article>
        <article><ShieldAlert :size="18" /><span>待处理问题</span><strong>{{ allIssues.length }}</strong></article>
        <article><History :size="18" /><span>当前时间线</span><strong>v{{ currentTimeline?.version || '—' }}</strong></article>
      </div>

      <nav class="workbench-tabs">
        <button v-for="[key,label,icon] in tabs" :key="key" :class="{ active: activeTab === key }" @click="activeTab = key"><component :is="icon" :size="16" />{{ label }}</button>
      </nav>

      <div class="workbench-grid">
        <aside class="scene-rail">
          <div class="rail-head"><span>SCENE ORDER</span><strong>场景卡片</strong><small>拖拽换序</small></div>
          <button
            v-for="scene in workspace.scenes" :key="scene.scene_id" draggable="true"
            :class="{ active: selectedScene?.scene_id === scene.scene_id }"
            @dragstart="draggedSceneId = scene.scene_id" @dragover.prevent @drop="dropScene(scene)"
            @click="selectedSceneId = scene.scene_id"
          >
            <GripVertical :size="15" /><b>{{ String(scene.scene_number).padStart(2, '0') }}</b>
            <span><strong>{{ scene.location_name || scene.scene_purpose || scene.scene_id }}</strong><small>{{ scene.interior_exterior }} · {{ scene.time_of_day }} · {{ scene.estimated_duration_seconds }}s</small></span>
          </button>
          <p v-if="!workspace.scenes.length">暂无场景。</p>
        </aside>

        <main class="creative-canvas">
          <section v-if="activeTab === 'script'" class="canvas-stack">
            <article class="pacing-panel">
              <header><div><span>PACING BEATS</span><h3>剧情节拍时间轴</h3></div><small>{{ workspace.pacing_beats.length }} 个节拍</small></header>
              <div class="beat-line">
                <div v-for="beat in workspace.pacing_beats" :key="beat.pacing_beat_id" :style="{ flex: Math.max(1, beat.duration_seconds || 1) }" :title="beat.beat_key">
                  <i :style="{ height: `${18 + Number(beat.intensity || 0) * 34}px` }"></i><span>{{ beat.beat_key }}</span>
                </div>
                <p v-if="!workspace.pacing_beats.length">尚无已发布节拍计划</p>
              </div>
            </article>

            <article class="dialogue-panel">
              <header><div><span>LINE EDITOR</span><h3>{{ selectedScene?.scene_purpose || '逐句对白' }}</h3></div><b>{{ sceneDialogues.length }} 句</b></header>
              <div v-for="dialogue in sceneDialogues" :key="dialogue.dialogue_id" class="dialogue-row">
                <div class="speaker"><UsersRound :size="15" /><strong>{{ dialogue.speaker_name || '旁白' }}</strong><small>{{ dialogue.emotion || '自然' }}</small></div>
                <textarea v-model="dialogueDrafts[dialogue.dialogue_id]" :aria-label="`${dialogue.speaker_name}对白`" />
                <div class="dialogue-meta"><span><Clock3 :size="13" />{{ dialogue.estimated_duration_ms }}ms</span><code>{{ dialogue.dialogue_id }}</code></div>
                <div class="line-actions">
                  <button :disabled="saving || dialogueDrafts[dialogue.dialogue_id] === dialogue.text" @click="saveDialogue(dialogue)">建立精确修改</button>
                  <details><summary>转换<ChevronDown :size="13" /></summary><div><button @click="convertDialogue(dialogue, 'narration')">转为旁白</button><button @click="convertDialogue(dialogue, 'action')">转为动作</button><button @click="convertDialogue(dialogue, 'spoken')">转为对白</button></div></details>
                </div>
              </div>
              <p v-if="!sceneDialogues.length" class="empty-row">当前场景没有对白。</p>
            </article>
          </section>

          <section v-else-if="activeTab === 'storyboard'" class="canvas-stack">
            <article class="storyboard-panel">
              <header><div><span>SHOT STORYBOARD</span><h3>分镜缩略图故事板</h3></div><small>拆分、合并、换序、调时长均生成局部计划</small></header>
              <div class="shot-strip">
                <article v-for="(shot,index) in sceneShots" :key="shot.shot_id" class="shot-card">
                  <div class="shot-frame"><img v-if="shot.thumbnail_url" :src="shot.thumbnail_url" :alt="`镜头 ${shot.shot_number}`" /><span v-else><Film :size="28" />SHOT {{ shot.shot_number }}</span><b>{{ Number(shot.duration_seconds).toFixed(1) }}s</b></div>
                  <div class="shot-copy"><strong>{{ shot.shot_size }} · {{ shot.camera_motion }}</strong><p>{{ shot.action_description }}</p><code>{{ shot.shot_id }}</code></div>
                  <div class="shot-tools">
                    <button title="拆分镜头" @click="markShotStructure(shot, 'split')"><Split :size="14" />拆分</button>
                    <button :disabled="index === sceneShots.length - 1" title="与下一镜合并" @click="markShotStructure(shot, 'merge')"><Layers3 :size="14" />合并</button>
                    <button title="缩短 0.5 秒" @click="retimeShot(shot, -.5)"><TimerReset :size="14" />−0.5s</button>
                    <button title="延长 0.5 秒" @click="retimeShot(shot, .5)"><Clock3 :size="14" />+0.5s</button>
                  </div>
                </article>
              </div>
            </article>
          </section>

          <section v-else-if="activeTab === 'sound'" class="canvas-stack sound-layout">
            <article class="sound-panel">
              <header><div><span>DIALOGUE / LIP SYNC</span><h3>对白时间与口型校验</h3></div><button class="primary-mini" :disabled="saving" @click="validateTimings"><Sparkles :size="14" />运行校验</button></header>
              <div v-for="timing in workspace.dialogue_timings" :key="timing.dialogue_timing_version_id" class="timing-row">
                <div><strong>{{ timing.speaker_name }}</strong><code>{{ timing.dialogue_id }}</code></div>
                <span>{{ milliseconds(timing.start_ms) }} → {{ milliseconds(timing.end_ms) }}</span>
                <span>音频 {{ timing.audio_duration_ms }}ms</span>
                <span>口型 {{ timing.target_lip_start_ms }}–{{ timing.target_lip_end_ms }}ms</span>
                <b :class="timing.status">{{ timing.status }}</b>
              </div>
              <p v-if="!workspace.dialogue_timings.length" class="empty-row">配音完成后将在这里建立逐句口型时间版本。</p>
              <div v-if="timingReport" class="timing-report"><strong>本次校验</strong><span>{{ timingReport.passed ? '通过' : `${timingReport.issues.length} 个问题` }}</span></div>
            </article>

            <article class="sound-panel">
              <header><div><span>FORMAL SOUND TASKS</span><h3>BGM / SFX / 环境声</h3></div><div class="sound-style-control"><input v-model="soundStyleTarget" aria-label="目标声音风格" placeholder="cinematic_noir" /><button class="primary-mini" :disabled="saving || !soundStyleTarget.trim()" @click="replaceSoundStyle"><Music2 :size="14" />整集替换并新建版本</button></div></header>
              <div v-for="cue in workspace.sound_cues" :key="cue.sound_cue_version_id" class="cue-row">
                <i :class="cue.cue_type"><Volume2 :size="15" /></i>
                <span><strong>{{ soundTrackLabels[cue.cue_type] || cue.cue_type }} · {{ cue.asset_name }}</strong><small>{{ cue.source_hint || cue.event_key || '正式声音任务' }}</small></span>
                <code>{{ milliseconds(cue.start_ms) }}–{{ milliseconds(cue.end_ms) }}</code>
                <b v-if="cue.ducking_config && Object.keys(cue.ducking_config).length">DUCK</b>
              </div>
              <p v-if="!workspace.sound_cues.length" class="empty-row">分镜中的 bgm_hint / sound_effect_hint 尚未转为正式任务。</p>
            </article>
          </section>

          <section v-else-if="activeTab === 'timeline'" class="canvas-stack">
            <article class="timeline-panel">
              <header><div><span>MULTI-TRACK TIMELINE</span><h3>图片、视频、音频和字幕时间线</h3></div><div class="transport"><button><Pause :size="14" /></button><button><Play :size="14" /></button><strong>{{ milliseconds(maxTimelineMS) }}</strong></div></header>
              <div class="timeline-ruler"><span v-for="tick in 6" :key="tick" :style="{ left: `${(tick-1)*20}%` }">{{ milliseconds(maxTimelineMS*(tick-1)/5) }}</span></div>
              <div v-for="lane in lanes" :key="lane.type" class="timeline-lane">
                <b>{{ lane.label }}</b>
                <div><span v-for="item in lane.entries" :key="item.timeline_item_id" :class="lane.type" :style="{ left: `${item.timeline_start_ms/maxTimelineMS*100}%`, width: `${Math.max(1,(item.timeline_end_ms-item.timeline_start_ms)/maxTimelineMS*100)}%` }" :title="item.entity_id">{{ item.entity_id }}</span></div>
              </div>
              <p v-if="!lanes.length" class="empty-row">当前时间线尚无可展示轨道。</p>
            </article>
          </section>

          <section v-else-if="activeTab === 'quality'" class="canvas-stack">
            <article class="quality-panel">
              <header><div><span>EDIT-LOCATED QUALITY</span><h3>角色状态、连续性与质量评分</h3></div><b>{{ allIssues.length }} issues</b></header>
              <div class="quality-columns">
                <section><h4>表演圣经</h4><div v-for="bible in workspace.performance_bibles" :key="bible.performance_bible_id" class="state-card"><strong>{{ bible.character_id }} · v{{ bible.version }}</strong><span>{{ bible.status }}</span></div><p v-if="!workspace.performance_bibles.length">暂无锁定表演圣经</p></section>
                <section><h4>连续性账本</h4><div v-for="entry in workspace.continuity" :key="entry.continuity_entry_id" class="state-card"><strong>{{ entry.scope }} · {{ entry.shot_id || entry.scene_id || '集级' }}</strong><span>{{ entry.validation_status }}</span></div><p v-if="!workspace.continuity.length">暂无连续性记录</p></section>
              </div>
              <div class="issue-table">
                <article v-for="issue in allIssues" :key="issue.dialogue_timing_issue_id || issue.visual_qc_issue_id || issue.quality_issue_id">
                  <i :class="issue.severity"></i><span><strong>{{ issue.issue_code || issue.category || issue.dimension }}</strong><small>{{ issue.message || issue.recommendation || issue.suggestion }}</small></span><code>{{ milliseconds(issueTime(issue)) }}</code>
                  <RouterLink :to="issueEditLink(projectId, issue)">跳转编辑</RouterLink>
                </article>
              </div>
            </article>
          </section>

          <section v-else class="canvas-stack">
            <article class="version-panel">
              <header><div><span>CANDIDATE & VERSION DIFF</span><h3>候选、时间线版本与恢复</h3></div><GitCompareArrows :size="20" /></header>
              <div class="version-columns">
                <section><h4>候选版本</h4><article v-for="candidate in workspace.candidates" :key="candidate.candidate_id"><b>#{{ candidate.ordinal }}</b><span><strong>{{ candidate.label }}</strong><small>{{ candidate.difference_direction }}</small></span><em>{{ candidate.total_score || '—' }}</em></article><p v-if="!workspace.candidates.length">暂无候选</p></section>
                <section><h4>剪辑时间线版本</h4><article v-for="timeline in workspace.timeline_versions" :key="timeline.timeline_id" :class="{ current: timeline.is_current }"><b>v{{ timeline.version }}</b><span><strong>{{ timeline.version_reason }}</strong><small>{{ timeline.approval_state }} · {{ timeline.editing_template_version_id || '未绑定模板' }}</small></span><em v-if="timeline.is_current">CURRENT</em><button v-else :disabled="saving" @click="restoreTimeline(timeline)"><Undo2 :size="13" />恢复为新版本</button></article><p v-if="!workspace.timeline_versions.length">暂无剪辑时间线版本</p></section>
              </div>
            </article>
          </section>
        </main>

        <aside class="inspector-rail">
          <div class="rail-head"><span>INSPECTOR</span><strong>选中场景</strong></div>
          <dl v-if="selectedScene">
            <div><dt>场景</dt><dd>{{ selectedScene.scene_number }} · {{ selectedScene.location_name }}</dd></div>
            <div><dt>人物</dt><dd>{{ selectedScene.character_ids?.join('、') || '—' }}</dd></div>
            <div><dt>情绪变化</dt><dd>{{ selectedScene.emotional_change || '—' }}</dd></div>
            <div><dt>来源事件</dt><dd>{{ selectedScene.source_event_ids?.length || 0 }} 个</dd></div>
          </dl>
          <section><h4>精确重建预览</h4><div v-for="dialogue in sceneDialogues" :key="dialogue.dialogue_id" class="rebuild-card"><strong>{{ dialogue.speaker_name }}</strong><template v-if="workspace.dialogue_timings.find(item => item.dialogue_id === dialogue.dialogue_id)"><span>{{ milliseconds(exactDialogueRebuildRange(workspace.dialogue_timings.find(item => item.dialogue_id === dialogue.dialogue_id)).start_ms) }}–{{ milliseconds(exactDialogueRebuildRange(workspace.dialogue_timings.find(item => item.dialogue_id === dialogue.dialogue_id)).end_ms) }}</span><small>配音 · 字幕 · 镜头区间 · 剪辑区间</small></template><span v-else>待建立 timing</span></div></section>
          <section><h4>绑定评论</h4><form class="comment-form" @submit.prevent="submitComment"><select v-model="commentTargetKey"><option v-for="target in commentTargets" :key="target.key" :value="target.key">{{ target.label }}</option></select><input v-model="commentTimecodeMS" type="number" min="0" placeholder="时间码 ms（可选）" /><textarea v-model="commentBody" placeholder="评论内容" /><button :disabled="saving || !commentBody.trim()">绑定评论</button></form><div v-for="comment in workspace.comments.slice(0,6)" :key="comment.comment_id" class="comment-card"><MessageSquareText :size="13" /><span><strong>{{ comment.body }}</strong><small>{{ comment.entity_type }} · {{ comment.entity_id }}<template v-if="comment.timecode_start_ms !== null"> · {{ comment.timecode_start_ms }}ms</template></small></span></div><p v-if="!workspace.comments.length">暂无评论</p></section>
        </aside>
      </div>
    </template>
  </section>
</template>

<style scoped>
.workbench-shell{display:grid;gap:14px;color:#172033}.workbench-header{display:flex;align-items:flex-end;justify-content:space-between;gap:20px;padding:20px 22px;border:1px solid #e2e7ef;border-radius:14px;background:linear-gradient(135deg,#fff 55%,#f0f4ff)}.workbench-title{display:flex;align-items:center;gap:12px;margin-top:8px}.workbench-title span,.rail-head span,.creative-canvas header span{color:#6f7f99;font-size:10px;letter-spacing:.14em}.workbench-title h2{margin:2px 0 0;font-size:24px}.workbench-title>b{padding:7px 9px;border-radius:7px;color:#fff;background:#17223b;font-size:12px}.workbench-header p{margin:7px 0 0;color:#788497;font-size:13px}.header-actions{display:flex;align-items:flex-end;gap:8px}.template-control{display:flex;align-items:flex-end;gap:6px;padding:9px;border:1px solid #dbe2ef;border-radius:10px;background:#fff}.template-control label{display:grid;gap:4px}.template-control label span{color:#8a95a5;font-size:10px}.template-control select{height:32px;border:0;border-radius:6px;color:#3c485d;background:#f5f7fb}.template-control button,.primary-mini{height:32px;display:flex;align-items:center;gap:5px;border:0;border-radius:7px;padding:0 10px;color:#fff;background:#536fd1;cursor:pointer}.refresh-button{width:38px;height:38px;border:1px solid #dbe2ea;border-radius:9px;background:#fff}.workbench-notice{display:flex;align-items:center;gap:8px;padding:10px 13px;border:1px solid #bfe6d7;border-radius:9px;color:#1c7357;background:#effaf5;font-size:13px}.workbench-notice button{margin-left:auto;border:0;color:#365dc0;background:none;font-weight:600;cursor:pointer}.workbench-metrics{display:grid;grid-template-columns:repeat(5,1fr);gap:1px;overflow:hidden;border:1px solid #e1e5ec;border-radius:11px;background:#e3e7ed}.workbench-metrics article{display:grid;grid-template-columns:25px 1fr auto;align-items:center;padding:12px 14px;background:#fff}.workbench-metrics svg{color:#6d80b7}.workbench-metrics span{color:#8b95a5;font-size:11px}.workbench-metrics strong{font-size:16px}.workbench-tabs{display:flex;gap:4px;padding:5px;border:1px solid #e2e6ed;border-radius:10px;background:#fff}.workbench-tabs button{height:36px;display:flex;align-items:center;gap:6px;border:0;border-radius:7px;padding:0 13px;color:#6f7b8d;background:none;cursor:pointer}.workbench-tabs button.active{color:#fff;background:#1c2942}.workbench-grid{display:grid;grid-template-columns:210px minmax(0,1fr) 245px;gap:12px;align-items:start}.scene-rail,.creative-canvas,.inspector-rail{border:1px solid #e1e6ed;border-radius:12px;background:#fff}.scene-rail,.inspector-rail{position:sticky;top:88px;overflow:hidden}.rail-head{padding:13px 14px;border-bottom:1px solid #ebedf1}.rail-head strong,.rail-head small{display:block}.rail-head strong{margin-top:2px}.rail-head small{margin-top:3px;color:#9aa3b0;font-size:11px}.scene-rail>button{width:100%;display:grid;grid-template-columns:15px 26px 1fr;align-items:center;gap:6px;padding:11px 10px;border:0;border-bottom:1px solid #eef0f3;color:#59667b;background:#fff;text-align:left;cursor:grab}.scene-rail>button.active{color:#2f4fa9;background:#f0f4ff;box-shadow:inset 3px 0 #5e78d4}.scene-rail>button>b{font-size:12px}.scene-rail>button span{min-width:0}.scene-rail>button span strong,.scene-rail>button span small{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.scene-rail>button span strong{font-size:12px}.scene-rail>button span small{margin-top:4px;color:#99a1ae;font-size:10px}.creative-canvas{min-width:0;overflow:hidden}.canvas-stack{display:grid;gap:12px;padding:14px;background:#f8f9fb}.creative-canvas article{border:1px solid #e3e7ed;border-radius:10px;background:#fff}.creative-canvas article>header{display:flex;align-items:center;justify-content:space-between;padding:12px 14px;border-bottom:1px solid #eceef2}.creative-canvas header h3{margin:2px 0 0;font-size:14px}.creative-canvas header small{color:#929baa;font-size:11px}.beat-line{height:95px;display:flex;align-items:flex-end;gap:2px;padding:12px 14px}.beat-line>div{min-width:26px;display:grid;justify-items:center;gap:5px}.beat-line i{width:100%;max-width:48px;border-radius:4px 4px 1px 1px;background:linear-gradient(#6c83d9,#aebced)}.beat-line span{width:100%;overflow:hidden;text-overflow:ellipsis;color:#7e899b;font-size:9px;text-align:center;white-space:nowrap}.dialogue-row{display:grid;grid-template-columns:95px minmax(0,1fr) 105px 104px;gap:10px;align-items:center;padding:11px 13px;border-bottom:1px solid #edf0f3}.speaker{display:grid;grid-template-columns:18px 1fr;align-items:center}.speaker small{grid-column:2;color:#909aa9;font-size:10px}.dialogue-row textarea{min-height:54px;resize:vertical;border:1px solid transparent;border-radius:7px;padding:8px;color:#273349;background:#f8f9fb;font:13px/1.55 inherit}.dialogue-row textarea:focus{outline:none;border-color:#b9c7ec;background:#fff}.dialogue-meta{display:grid;gap:4px;color:#7b8799;font-size:10px}.dialogue-meta span{display:flex;align-items:center;gap:4px}.dialogue-meta code{overflow:hidden;text-overflow:ellipsis}.line-actions{display:grid;gap:4px}.line-actions>button,.line-actions summary,.line-actions details div button,.shot-tools button{display:flex;align-items:center;justify-content:center;gap:3px;border:1px solid #dce2eb;border-radius:6px;padding:5px;color:#657289;background:#fff;font-size:10px;cursor:pointer}.line-actions details{position:relative}.line-actions summary{list-style:none}.line-actions details div{position:absolute;right:0;z-index:4;width:95px;padding:4px;border:1px solid #dce2ea;border-radius:7px;background:#fff;box-shadow:0 6px 18px #20304a1f}.line-actions details div button{width:100%;border:0}.shot-strip{display:grid;grid-template-columns:repeat(auto-fill,minmax(190px,1fr));gap:9px;padding:12px}.shot-card{overflow:hidden!important}.shot-frame{position:relative;aspect-ratio:16/10;display:grid;place-items:center;color:#748198;background:#162033}.shot-frame img{width:100%;height:100%;object-fit:cover}.shot-frame span{display:grid;justify-items:center;gap:5px;font-size:10px}.shot-frame>b{position:absolute;right:6px;bottom:6px;padding:3px 5px;border-radius:4px;color:#fff;background:#08101dcc;font-size:10px}.shot-copy{padding:9px}.shot-copy strong{font-size:11px}.shot-copy p{height:34px;overflow:hidden;margin:5px 0;color:#778295;font-size:10px;line-height:1.5}.shot-copy code{color:#a0a7b2;font-size:9px}.shot-tools{display:grid;grid-template-columns:1fr 1fr;gap:4px;padding:0 8px 8px}.shot-tools button{padding:5px 2px}.sound-layout{grid-template-columns:1fr}.timing-row{display:grid;grid-template-columns:1fr 110px 95px 145px 65px;gap:8px;align-items:center;padding:9px 12px;border-bottom:1px solid #edf0f3;font-size:10px}.timing-row>div strong,.timing-row>div code{display:block}.timing-row>div code{margin-top:3px;color:#939cab}.timing-row>b{padding:4px;border-radius:5px;color:#86652b;background:#fff4de;text-align:center}.timing-row>b.aligned{color:#277258;background:#e9f8f2}.timing-report{display:flex;justify-content:space-between;padding:10px 12px;color:#465676;background:#eef3ff}.cue-row{display:grid;grid-template-columns:30px 1fr 100px auto;gap:8px;align-items:center;padding:9px 12px;border-bottom:1px solid #edf0f3}.cue-row>i{width:28px;height:28px;display:grid;place-items:center;border-radius:7px;color:#566fc3;background:#edf2ff}.cue-row span strong,.cue-row span small{display:block}.cue-row span strong{font-size:11px}.cue-row span small{margin-top:3px;color:#8994a5;font-size:10px}.cue-row code{font-size:10px}.cue-row>b{color:#a25c2e;font-size:9px}.transport{display:flex;align-items:center;gap:4px}.transport button{width:27px;height:27px;border:1px solid #dce1ea;border-radius:6px;background:#fff}.transport strong{font-size:11px}.timeline-ruler{position:relative;height:26px;margin-left:92px;border-bottom:1px solid #e8ebf0}.timeline-ruler span{position:absolute;top:8px;color:#a0a8b4;font-size:9px}.timeline-lane{display:grid;grid-template-columns:88px 1fr;min-height:40px;border-bottom:1px solid #edf0f3}.timeline-lane>b{padding:12px;color:#69768a;font-size:10px}.timeline-lane>div{position:relative;margin:4px 8px 4px 0;background:repeating-linear-gradient(90deg,#f8f9fb 0,#f8f9fb 19.8%,#edf0f4 20%)}.timeline-lane span{position:absolute;top:4px;bottom:4px;min-width:6px;overflow:hidden;border-radius:4px;padding:5px;color:#fff;background:#5f78c6;font-size:8px;white-space:nowrap}.timeline-lane span.dialogue{background:#41a07a}.timeline-lane span.bgm{background:#815fb3}.timeline-lane span.sound_effect,.timeline-lane span.ambience{background:#c67d4a}.timeline-lane span.subtitle{background:#526071}.quality-columns,.version-columns{display:grid;grid-template-columns:1fr 1fr;gap:10px;padding:12px}.quality-columns section,.version-columns section{border:1px solid #e5e8ed;border-radius:8px;padding:10px}.quality-columns h4,.version-columns h4,.inspector-rail h4{margin:0 0 8px;font-size:11px}.state-card{display:flex;justify-content:space-between;padding:7px;border-radius:6px;background:#f8f9fb;font-size:10px}.issue-table{padding:0 12px 12px}.issue-table article{display:grid;grid-template-columns:5px 1fr 70px 68px;gap:8px;align-items:center;padding:8px;border-radius:0;border-width:0 0 1px}.issue-table i{height:28px;border-radius:3px;background:#e0a53d}.issue-table i.critical,.issue-table i.blocking{background:#cf4e52}.issue-table span strong,.issue-table span small{display:block}.issue-table span strong{font-size:10px}.issue-table span small{margin-top:3px;color:#8a94a4;font-size:9px}.issue-table code,.issue-table a{font-size:9px}.version-columns section>article{display:grid;grid-template-columns:35px 1fr auto;gap:7px;align-items:center;margin-bottom:5px;padding:8px;background:#fafbfc}.version-columns section>article.current{background:#edf7f3}.version-columns article span strong,.version-columns article span small{display:block}.version-columns article span strong{font-size:10px}.version-columns article span small{margin-top:3px;color:#8c96a5;font-size:9px}.version-columns article em{color:#338164;font-size:8px}.version-columns article button{display:flex;align-items:center;gap:3px;border:0;color:#4864b5;background:none;font-size:9px}.inspector-rail dl{margin:0}.inspector-rail dl div{padding:10px 12px;border-bottom:1px solid #edf0f3}.inspector-rail dt{color:#98a1ae;font-size:9px}.inspector-rail dd{margin:4px 0 0;font-size:11px}.inspector-rail>section{padding:12px;border-top:1px solid #e8ebef}.rebuild-card{padding:7px;margin-bottom:5px;border-radius:6px;background:#f7f9fc}.rebuild-card strong,.rebuild-card span,.rebuild-card small{display:block}.rebuild-card strong{font-size:10px}.rebuild-card span,.rebuild-card small{margin-top:3px;color:#8490a3;font-size:9px}.comment-card{display:flex;gap:6px;padding:7px;border-bottom:1px solid #edf0f3}.comment-card span strong,.comment-card span small{display:block}.comment-card span strong{font-size:10px}.comment-card span small{margin-top:3px;color:#929baa;font-size:9px}.empty-row{padding:20px;color:#97a0ad;text-align:center;font-size:11px}@media(max-width:1180px){.workbench-grid{grid-template-columns:190px minmax(0,1fr)}.inspector-rail{position:static;grid-column:1/-1}.workbench-metrics{grid-template-columns:repeat(3,1fr)}}@media(max-width:850px){.workbench-header{align-items:flex-start;flex-direction:column}.header-actions,.template-control{width:100%;flex-wrap:wrap}.workbench-grid{grid-template-columns:1fr}.scene-rail{position:static;display:grid;grid-template-columns:repeat(2,1fr)}.scene-rail .rail-head{grid-column:1/-1}.workbench-tabs{overflow-x:auto}.workbench-tabs button{white-space:nowrap}.dialogue-row{grid-template-columns:80px 1fr}.dialogue-meta,.line-actions{grid-column:2}.workbench-metrics{grid-template-columns:1fr 1fr}.timing-row{grid-template-columns:1fr 1fr}}
.sound-style-control{display:flex;align-items:center;gap:6px}.sound-style-control input{height:30px;width:145px;border:1px solid #dce2eb;border-radius:6px;padding:0 8px;color:#46546b;background:#fff;font:11px inherit}.comment-form{display:grid;gap:5px;margin-bottom:8px}.comment-form select,.comment-form input,.comment-form textarea{border:1px solid #dfe4ec;border-radius:6px;padding:6px;color:#4d596c;background:#fff;font:10px inherit}.comment-form textarea{min-height:48px;resize:vertical}.comment-form button{height:28px;border:0;border-radius:6px;color:#fff;background:#536fd1;font-size:10px}
</style>
