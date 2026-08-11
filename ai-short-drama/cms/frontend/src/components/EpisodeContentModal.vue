<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
	AlertCircle, AlertTriangle, ArrowDown, ArrowUp, Bot, CheckCircle2, Clock3, Copy,
	Eye, FileText, GitCompareArrows, History, LoaderCircle, MapPin, MessageSquareText,
	Pencil, Play, Plus, RotateCcw, Save, Scissors, Search, ScrollText, ShieldCheck,
	Trash2, X,
} from 'lucide-vue-next'
import { api } from '../services/api'
import {
	batchReplaceScript, buildEpisodeContentPayload, calculateScriptMetrics, cloneEpisodeContent,
	copyScene, deleteAction, deleteDialogue, deleteScene, episodeContentChanged, insertAction,
	insertDialogue, insertScene, mergeSceneWithNext, moveAction, moveDialogue, moveScene,
	searchScript, splitScene, structuredScriptDiff, validateStructuredScript,
} from '../services/episodeContent'
import StatusBadge from './StatusBadge.vue'

const props = defineProps({
  projectId: { type: String, required: true },
  episodeRun: { type: Object, required: true },
})
const emit = defineEmits(['close', 'saved'])

const content = ref(null)
const draft = ref(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const editing = ref(false)
const activeTab = ref('outline')
const savedNotice = ref('')
const pendingPlan = ref(null)
const pendingPlanOrigin = ref('draft')
const selectedRebuildTasks = ref([])
const selectedBlocks = ref([])
const aiOperation = ref('compress_dialogue')
const aiConvertTo = ref('dialogue')
const aiInstruction = ref('')
const searchText = ref('')
const replacementText = ref('')
const replaceNotice = ref('')
const versions = ref([])
const leftVersionId = ref('')
const rightVersionId = ref('')
const selectedRestorePaths = ref([])

const changed = computed(() => content.value && draft.value && episodeContentChanged(content.value, draft.value))
const current = computed(() => editing.value ? draft.value : content.value)
const script = computed(() => current.value?.script || null)
const planDiff = computed(() => pendingPlan.value?.plan?.expected_changes || [])
const rebuildSelectionChanged = computed(() => {
	if (pendingPlanOrigin.value !== 'draft') return false
  const planned = pendingPlan.value?.plan?.impact?.rebuild_tasks || []
  return planned.length !== selectedRebuildTasks.value.length ||
    planned.some(item => !selectedRebuildTasks.value.includes(item))
})
const referenceContext = computed(() => content.value?.reference_context || {})
const metrics = computed(() => calculateScriptMetrics(current.value))
const diagnostics = computed(() => validateStructuredScript(current.value, referenceContext.value))
const searchMatches = computed(() => searchScript(current.value, searchText.value))
const leftVersion = computed(() => versions.value.find(item => item.entity_version_id === leftVersionId.value))
const rightVersion = computed(() => versions.value.find(item => item.entity_version_id === rightVersionId.value))
const versionDiff = computed(() => leftVersion.value && rightVersion.value
	? structuredScriptDiff(leftVersion.value.content, rightVersion.value.content) : [])

function formatDuration(seconds) {
  const value = Number(seconds || 0)
  if (value < 60) return `${value} 秒`
  return `${Math.floor(value / 60)} 分 ${value % 60} 秒`
}

function actionDescription(action) {
  if (typeof action === 'string') return action
  return action?.description || action?.text || action?.visual || '未填写动作描述'
}

function formatDiffValue(value) {
  if (value == null) return '—'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

async function loadContent() {
  loading.value = true
  error.value = ''
  try {
		content.value = await api.getEpisodeContent(props.projectId, props.episodeRun.episode_run_id)
		draft.value = cloneEpisodeContent(content.value)
		versions.value = await api.getEntityVersions(props.projectId, 'episode_content', content.value.episode_id)
		rightVersionId.value = versions.value.find(item => item.is_current)?.entity_version_id || versions.value[0]?.entity_version_id || ''
		leftVersionId.value = versions.value.find(item => !item.is_current)?.entity_version_id || versions.value[1]?.entity_version_id || rightVersionId.value
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

function beginEdit() {
  if (!content.value?.editable) return
  draft.value = cloneEpisodeContent(content.value)
  editing.value = true
  savedNotice.value = ''
}

function cancelEdit() {
  draft.value = cloneEpisodeContent(content.value)
  editing.value = false
  error.value = ''
	pendingPlan.value = null
	pendingPlanOrigin.value = 'draft'
	selectedRebuildTasks.value = []
	selectedBlocks.value = []
}

function requestClose() {
  if (saving.value) return
  if (editing.value && changed.value && !window.confirm('尚有未保存的修改，确定关闭吗？')) return
  emit('close')
}

async function saveContent() {
  if (!changed.value || saving.value) return
  saving.value = true
  error.value = ''
  savedNotice.value = ''
  try {
    pendingPlan.value = await api.createEpisodeContentChangePlan(
      props.projectId,
      props.episodeRun.episode_run_id,
      {
        ...buildEpisodeContentPayload(draft.value),
        must_preserve: ['未修改字段', '来源证据', '人物与场景标识'],
        locks: ['character'],
      },
    )
    pendingPlanOrigin.value = 'draft'
    selectedRebuildTasks.value = [...(pendingPlan.value.plan?.impact?.rebuild_tasks || [])]
    savedNotice.value = '修改计划已生成；正式内容尚未改变。'
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

function sceneMetric(sceneId) {
	return metrics.value.scenes.find(item => item.scene_id === sceneId) || {}
}

function selectValue(type, id) {
	return `${type}:${id}`
}

function addSceneAfter(index) {
	const scene = insertScene(draft.value, index)
	selectedBlocks.value = [selectValue('scene', scene.scene_id)]
}

function removeScene(scene) {
	if (window.confirm(`删除场景 ${scene.scene_number}？正式剧本会在 change plan 执行后才改变。`)) deleteScene(draft.value, scene.scene_id)
}

function duplicateScene(scene) {
	copyScene(draft.value, scene.scene_id)
}

function splitSceneAt(scene, dialogueIndex) {
	splitScene(draft.value, scene.scene_id, dialogueIndex)
}

function mergeScene(scene) {
	mergeSceneWithNext(draft.value, scene.scene_id)
}

function addDialogue(scene, type = 'dialogue') {
	const dialogue = insertDialogue(scene, scene.dialogues.length - 1, type)
	selectedBlocks.value = [selectValue('dialogue', dialogue.dialogue_id)]
}

function addAction(scene) {
	const action = insertAction(scene, scene.actions.length - 1)
	selectedBlocks.value = [selectValue('action', action.action_id)]
}

function syncDialogueCharacter(dialogue) {
	const character = referenceContext.value.characters?.find(item => item.character_id === dialogue.character_id)
	if (character) dialogue.speaker_name = character.name
}

async function runAI() {
	if (!selectedBlocks.value.length || saving.value) {
		error.value = '请先勾选场景、动作或台词范围。'
		return
	}
	saving.value = true
	error.value = ''
	try {
		const selection = { scene_ids: [], dialogue_ids: [], action_ids: [] }
		for (const value of selectedBlocks.value) {
			const [type, ...rest] = value.split(':')
			const id = rest.join(':')
			if (type === 'scene') selection.scene_ids.push(id)
			if (type === 'dialogue') selection.dialogue_ids.push(id)
			if (type === 'action') selection.action_ids.push(id)
		}
		pendingPlan.value = await api.createEpisodeContentAIChangePlan(
			props.projectId, props.episodeRun.episode_run_id, {
				draft: {
					...buildEpisodeContentPayload(draft.value),
					must_preserve: ['原著事件', 'Source Span', '人物状态', 'must_preserve'],
					locks: ['character'], requested_by: 'structured-script-editor',
				},
				selection, operation: aiOperation.value,
				convert_to: aiOperation.value === 'convert' ? aiConvertTo.value : '',
				instruction: aiInstruction.value,
			},
		)
		pendingPlanOrigin.value = 'ai'
		selectedRebuildTasks.value = [...(pendingPlan.value.plan?.impact?.rebuild_tasks || [])]
		savedNotice.value = 'AI 已返回结构化 diff；正式剧本尚未改变。'
	} catch (err) {
		error.value = err.message
	} finally {
		saving.value = false
	}
}

function replaceAll() {
	const count = batchReplaceScript(draft.value, searchText.value, replacementText.value)
	replaceNotice.value = count ? `已在草稿替换 ${count} 处；保存后进入 change plan。` : '没有找到匹配内容。'
}

async function restoreVersion() {
	if (!leftVersion.value || !selectedRestorePaths.value.length || saving.value) return
	saving.value = true
	error.value = ''
	try {
		pendingPlan.value = await api.createVersionRestorePlan(
			props.projectId, leftVersion.value.entity_version_id,
			{ mode: 'rollback', paths: selectedRestorePaths.value, requested_by: 'structured-script-editor' },
		)
		pendingPlanOrigin.value = 'restore'
		selectedRebuildTasks.value = [...(pendingPlan.value.plan?.impact?.rebuild_tasks || [])]
		savedNotice.value = '局部恢复计划已生成；确认与执行前 current 版本不变。'
	} catch (err) {
		error.value = err.message
	} finally {
		saving.value = false
	}
}

async function confirmPlan(executeImmediately = false) {
  if (pendingPlan.value?.status !== 'validated' || saving.value) return
  saving.value = true
  error.value = ''
  try {
    if (rebuildSelectionChanged.value) {
      pendingPlan.value = await api.createEpisodeContentChangePlan(
        props.projectId,
        props.episodeRun.episode_run_id,
        {
          ...buildEpisodeContentPayload(draft.value),
          must_preserve: ['未修改字段', '来源证据', '人物与场景标识'],
          locks: ['character'],
          rebuild_tasks: [...selectedRebuildTasks.value],
        },
      )
    }
    pendingPlan.value = await api.confirmChangePlan(
      props.projectId, pendingPlan.value.change_plan_id, { actor: 'episode-content-modal' },
    )
    savedNotice.value = '计划已确认；正式内容仍未改变。'
    if (executeImmediately) await executePlan(true)
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

async function rejectPlan() {
	if (pendingPlan.value?.status !== 'validated' || saving.value) return
	saving.value = true
	error.value = ''
	try {
		await api.rejectChangePlan(props.projectId, pendingPlan.value.change_plan_id, {
			actor: 'episode-content-modal', reason: 'candidate rejected in review',
		})
		pendingPlan.value = null
		selectedRebuildTasks.value = []
		savedNotice.value = '候选已拒绝；current 剧本未发生变化。'
	} catch (err) {
		error.value = err.message
	} finally {
		saving.value = false
	}
}

async function executePlan(nested = false) {
  if (pendingPlan.value?.status !== 'confirmed') return
  if (!nested) saving.value = true
  error.value = ''
  try {
    pendingPlan.value = await api.executeChangePlan(
      props.projectId, pendingPlan.value.change_plan_id,
    )
    await loadContent()
    draft.value = cloneEpisodeContent(content.value)
    editing.value = false
    savedNotice.value = '已创建新版本并切换 current；重建任务已进入 pending。'
    emit('saved', content.value)
		pendingPlan.value = null
		pendingPlanOrigin.value = 'draft'
		selectedRebuildTasks.value = []
  } catch (err) {
    error.value = err.message
  } finally {
    if (!nested) saving.value = false
  }
}

function handleKeydown(event) {
  if (event.key === 'Escape') requestClose()
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  loadContent()
})
onUnmounted(() => window.removeEventListener('keydown', handleKeydown))
</script>

<template>
  <div class="modal-backdrop episode-content-backdrop" @click.self="requestClose">
    <article class="review-modal episode-content-modal" role="dialog" aria-modal="true" aria-labelledby="episode-content-title">
      <header class="episode-content-head">
        <div>
          <span>EPISODE CONTENT</span>
          <h3 id="episode-content-title">第 {{ episodeRun.episode_number }} 集内容</h3>
          <p>{{ episodeRun.title }}</p>
        </div>
        <div class="episode-content-head-actions">
          <button
            v-if="content && !editing"
            class="button button-secondary"
            :disabled="!content.editable"
            :title="content.read_only_reason || '修改本集内容'"
            @click="beginEdit"
          ><Pencil :size="15" />修改内容</button>
          <button class="episode-modal-close" title="关闭" @click="requestClose"><X :size="18" /></button>
        </div>
      </header>

      <div v-if="loading" class="episode-content-loading">
        <LoaderCircle :size="24" class="spin" /><span>正在读取本集内容…</span>
      </div>
      <div v-else-if="error && !content" class="error-banner episode-content-error">
        <AlertCircle :size="17" />{{ error }}<button @click="loadContent">重试</button>
      </div>

      <template v-else-if="content">
        <div class="episode-content-meta">
          <StatusBadge :status="content.run_status" />
          <span><FileText :size="14" />大纲 v{{ content.outline.version }}</span>
          <span v-if="content.script"><ScrollText :size="14" />剧本 v{{ content.script.version }}</span>
          <span><Clock3 :size="14" />{{ formatDuration(content.outline.estimated_duration_seconds) }}</span>
        </div>

        <div v-if="!content.editable" class="episode-content-warning">
          <AlertTriangle :size="17" /><div><strong>当前只读</strong><p>{{ content.read_only_reason }}</p></div>
        </div>
        <div v-if="content.has_downstream_assets" class="episode-content-warning downstream">
          <AlertTriangle :size="17" /><div><strong>已有下游内容</strong><p>确认前会列出精确受影响 artifact、时间区间与重建任务；未受影响内容不会失效。</p></div>
        </div>
        <div v-if="savedNotice" class="episode-save-notice">{{ savedNotice }}</div>
        <div v-if="error" class="error-banner episode-content-error"><AlertCircle :size="17" />{{ error }}</div>

        <section v-if="pendingPlan" class="episode-change-plan">
          <header><GitCompareArrows :size="18" /><div><strong>修改计划预览</strong><small>{{ pendingPlan.change_plan_id }} · {{ pendingPlan.status }}</small></div></header>
          <div class="episode-plan-summary">
            <article><b>must_preserve</b><span v-for="item in pendingPlan.plan.must_preserve" :key="item"><ShieldCheck :size="12" />{{ item }}</span></article>
            <article><b>锁定项</b><span v-for="item in pendingPlan.plan.locks" :key="item">锁定 {{ item }}</span><span v-if="!pendingPlan.plan.locks.length">无</span></article>
            <article><b>影响 artifact</b><span v-for="item in pendingPlan.impacts" :key="item.artifact_id">{{ item.artifact_type }} · {{ item.native_entity_id }}</span><span v-for="item in pendingPlan.plan.impact.downstream" :key="`planned:${item}`">{{ item }} · 计划范围</span><span v-if="!pendingPlan.impacts.length && !pendingPlan.plan.impact.downstream.length">无现存下游 artifact</span></article>
            <article><b>重建范围 / 选择</b><label v-for="item in pendingPlan.plan.impact.rebuild_tasks" :key="item"><input v-model="selectedRebuildTasks" type="checkbox" :value="item" :disabled="pendingPlan.status !== 'validated' || saving || pendingPlanOrigin !== 'draft'" />{{ item }} · 选中后执行状态为 pending</label><span v-if="pendingPlanOrigin !== 'draft'">AI 与恢复计划锁定其精确重建范围。</span><span v-if="rebuildSelectionChanged">未选任务对应 artifact 将保持 stale，不会被伪标记为已完成。</span><span v-if="!pendingPlan.plan.impact.rebuild_tasks.length">无需重建，可保存并确认</span></article>
          </div>
          <div class="episode-plan-diff">
            <div class="episode-plan-diff-head"><b>字段</b><b>修改前</b><b>修改后</b></div>
            <div v-for="row in planDiff" :key="row.field"><code>{{ row.field }}<small v-if="row.start_ms != null">重建 {{ row.start_ms }}–{{ row.end_ms }}ms</small></code><span>{{ formatDiffValue(row.before) }}</span><span>{{ formatDiffValue(row.after) }}</span></div>
          </div>
          <div v-if="pendingPlan.review_metadata?.candidate_type === 'script_ai_rewrite'" class="episode-plan-summary">
            <article><b>AI 改写理由</b><span>{{ pendingPlan.review_metadata.reason }}</span></article>
            <article><b>来源证据</b><span v-for="item in pendingPlan.review_metadata.source_evidence" :key="`${item.source_span_id}:${item.event_revision_id}`">{{ item.source_span_id || item.event_revision_id }} · {{ item.explanation }}</span></article>
            <article><b>预计时长变化</b><span>{{ pendingPlan.review_metadata.estimated_duration_delta_ms }} ms</span></article>
          </div>
          <div class="episode-plan-risks"><b>风险</b><span v-for="risk in pendingPlan.plan.risks" :key="risk"><AlertTriangle :size="12" />{{ risk }}</span></div>
          <footer>
            <button v-if="pendingPlan.status === 'validated'" class="button button-secondary" :disabled="saving" @click="rejectPlan">拒绝候选</button>
            <button class="button button-secondary" :disabled="saving" @click="pendingPlan = null">返回编辑</button>
            <button v-if="pendingPlan.status === 'validated' && !content.has_downstream_assets" class="button button-primary" :disabled="saving" @click="confirmPlan(true)"><Save :size="15" />保存并确认</button>
            <button v-else-if="pendingPlan.status === 'validated'" class="button button-primary" :disabled="saving" @click="confirmPlan(false)"><ShieldCheck :size="15" />确认影响与重建</button>
            <button v-else-if="pendingPlan.status === 'confirmed'" class="button button-primary" :disabled="saving" @click="executePlan(false)"><Play :size="15" />执行并创建新版本</button>
          </footer>
        </section>

        <template v-else>
		<nav class="episode-content-tabs" aria-label="单集内容类型">
			<button :class="{ active: activeTab === 'outline' }" @click="activeTab = 'outline'"><FileText :size="15" />分集大纲</button>
			<button :class="{ active: activeTab === 'script' }" @click="activeTab = 'script'"><ScrollText :size="15" />单集剧本<i>{{ content.script ? content.script.scenes.length : 0 }}</i></button>
			<button :class="{ active: activeTab === 'versions' }" @click="activeTab = 'versions'"><History :size="15" />版本比较<i>{{ versions.length }}</i></button>
		</nav>

        <main class="episode-content-body">
          <section v-if="activeTab === 'outline'" class="episode-outline-content">
            <template v-if="editing">
              <div class="episode-edit-grid">
                <label class="episode-field full"><span>标题</span><input v-model="draft.outline.title" maxlength="400" /></label>
                <label class="episode-field"><span>预计时长（秒）</span><input v-model.number="draft.outline.estimated_duration_seconds" type="number" min="1" max="3600" /></label>
                <label class="episode-field full"><span>一句话梗概</span><textarea v-model="draft.outline.logline" rows="3" /></label>
                <label class="episode-field full"><span>开场钩子</span><textarea v-model="draft.outline.opening_hook" rows="3" /></label>
                <label class="episode-field full"><span>本集目标</span><textarea v-model="draft.outline.story_goal" rows="3" /></label>
                <label class="episode-field full"><span>核心冲突</span><textarea v-model="draft.outline.main_conflict" rows="3" /></label>
                <label class="episode-field full"><span>高潮</span><textarea v-model="draft.outline.climax" rows="3" /></label>
                <label class="episode-field full"><span>结尾钩子</span><textarea v-model="draft.outline.ending_hook" rows="3" /></label>
              </div>
            </template>
            <template v-else>
              <div class="episode-readable-title"><span>第 {{ content.outline.episode_number }} 集</span><h4>{{ content.outline.title }}</h4></div>
              <div class="episode-readable-grid">
                <article class="wide"><span>一句话梗概</span><p>{{ content.outline.logline || '—' }}</p></article>
                <article><span>开场钩子</span><p>{{ content.outline.opening_hook || '—' }}</p></article>
                <article><span>本集目标</span><p>{{ content.outline.story_goal || '—' }}</p></article>
                <article class="wide emphasis"><span>核心冲突</span><p>{{ content.outline.main_conflict || '—' }}</p></article>
                <article><span>高潮</span><p>{{ content.outline.climax || '—' }}</p></article>
                <article><span>结尾钩子</span><p>{{ content.outline.ending_hook || '—' }}</p></article>
              </div>
            </template>
          </section>

		  <section v-else-if="activeTab === 'script'" class="episode-script-content">
            <div v-if="!script" class="episode-script-empty">
              <ScrollText :size="28" /><h4>本集剧本尚未生成</h4><p>当前仍可在“分集大纲”中查看和修改标题、冲突、钩子等内容。</p>
            </div>
			<template v-else>
			  <div class="script-live-metrics">
				<span><Clock3 :size="14" />整集 {{ formatDuration(Math.round(metrics.duration_ms / 1000)) }}</span>
				<span>对白 {{ formatDuration(Math.round(metrics.dialogue_duration_ms / 1000)) }}</span>
				<span>动作比例 {{ Math.round(metrics.action_ratio * 100) }}%</span>
				<span :class="{ danger: diagnostics.some(item => item.severity === 'blocking') }">{{ diagnostics.length }} 个结构问题</span>
			  </div>
			  <div v-if="editing" class="script-editor-toolbar">
				<div class="script-ai-tools">
				  <Bot :size="17" />
				  <select v-model="aiOperation" aria-label="AI 操作">
					<option value="compress_dialogue">压缩对白</option><option value="colloquialize">口语化</option>
					<option value="strengthen_conflict">加强冲突</option><option value="strengthen_hook">增强钩子</option>
					<option value="convert">转对白 / 动作 / 旁白</option><option value="rewrite_preserve_facts">保持剧情事实改写</option>
				  </select>
				  <select v-if="aiOperation === 'convert'" v-model="aiConvertTo" aria-label="转换类型">
					<option value="dialogue">对白</option><option value="action">动作</option><option value="narration">旁白</option>
					<option value="inner_monologue">内心独白</option><option value="off_screen">画外音</option>
				  </select>
				  <input v-model="aiInstruction" placeholder="补充要求（可选）" />
				  <button class="button button-primary" :disabled="saving || !selectedBlocks.length" @click="runAI">生成结构化 diff（{{ selectedBlocks.length }}）</button>
				</div>
				<div class="script-search-tools">
				  <Search :size="16" /><input v-model="searchText" placeholder="人物称谓或专有名词" /><input v-model="replacementText" placeholder="替换为" />
				  <button class="button button-secondary" :disabled="!searchText" @click="replaceAll">批量替换</button><small>{{ searchText ? `找到 ${searchMatches.reduce((sum, item) => sum + item.count, 0)} 处` : '' }} {{ replaceNotice }}</small>
				  <div v-if="searchText && searchMatches.length" class="script-search-results"><code v-for="item in searchMatches.slice(0, 12)" :key="item.path">{{ item.path }} · {{ item.count }} 处 · {{ item.text }}</code><small v-if="searchMatches.length > 12">另有 {{ searchMatches.length - 12 }} 个字段命中</small></div>
				</div>
			  </div>
			  <div v-if="editing" class="episode-script-edit-head">
                <label class="episode-field full"><span>剧本标题</span><input v-model="draft.script.title" maxlength="400" /></label>
                <label class="episode-field"><span>开场钩子</span><textarea v-model="draft.script.opening_hook" rows="2" /></label>
                <label class="episode-field"><span>高潮</span><textarea v-model="draft.script.climax" rows="2" /></label>
                <label class="episode-field full"><span>结尾钩子</span><textarea v-model="draft.script.ending_hook" rows="2" /></label>
              </div>
              <div v-else class="episode-script-summary">
                <div><span>剧本标题</span><strong>{{ script.title }}</strong></div>
                <div><span>场景 / 对白字数</span><strong>{{ script.scenes.length }} / {{ script.dialogue_char_count }}</strong></div>
                <div><span>开场钩子</span><p>{{ script.opening_hook || '—' }}</p></div>
                <div><span>高潮</span><p>{{ script.climax || '—' }}</p></div>
                <div><span>结尾钩子</span><p>{{ script.ending_hook || '—' }}</p></div>
              </div>

			  <div class="structured-editor-layout">
			  <div class="episode-scene-list">
				<button v-if="editing" class="scene-add-button" @click="addSceneAfter(-1)"><Plus :size="15" />在开头新增场景</button>
				<article v-for="scene in script.scenes" :key="scene.scene_id" class="episode-scene-card">
				  <header>
					<label v-if="editing" class="block-selector" title="选择整个场景作为 AI 范围"><input v-model="selectedBlocks" type="checkbox" :value="selectValue('scene', scene.scene_id)" /></label>
					<b>场景 {{ scene.scene_number }}</b>
					<span><MapPin :size="13" />{{ scene.location_name || '未指定地点' }}</span>
					<span><Clock3 :size="13" />{{ scene.time_of_day || '未指定' }} · {{ scene.interior_exterior || '未指定' }} · 实时 {{ formatDuration(Math.round((sceneMetric(scene.scene_id).duration_ms || 0) / 1000)) }} · 对白 {{ formatDuration(Math.round((sceneMetric(scene.scene_id).dialogue_duration_ms || 0) / 1000)) }} · 动作 {{ Math.round((sceneMetric(scene.scene_id).action_ratio || 0) * 100) }}%</span>
					<div v-if="editing" class="scene-structure-actions">
					  <button title="上移" @click="moveScene(draft, scene.scene_id, -1)"><ArrowUp :size="13" /></button><button title="下移" @click="moveScene(draft, scene.scene_id, 1)"><ArrowDown :size="13" /></button>
					  <button title="复制场景" @click="duplicateScene(scene)"><Copy :size="13" /></button><button title="拆分场景" @click="splitSceneAt(scene, Math.ceil(scene.dialogues.length / 2))"><Scissors :size="13" /></button>
					  <button title="与下一场合并" @click="mergeScene(scene)"><GitCompareArrows :size="13" /></button><button title="删除场景" @click="removeScene(scene)"><Trash2 :size="13" /></button>
					  <button title="在后面新增场景" @click="addSceneAfter(scene.scene_number - 1)"><Plus :size="13" /></button>
					</div>
				  </header>

                  <div v-if="editing" class="episode-scene-editor">
                    <div class="episode-edit-grid compact">
                      <label class="episode-field"><span>地点</span><input v-model="scene.location_name" /></label>
                      <label class="episode-field"><span>时间</span><input v-model="scene.time_of_day" /></label>
                      <label class="episode-field"><span>内/外景</span><input v-model="scene.interior_exterior" /></label>
                      <label class="episode-field"><span>时长（秒）</span><input v-model.number="scene.estimated_duration_seconds" type="number" min="1" max="1800" /></label>
					  <label class="episode-field full"><span>场景目的</span><textarea v-model="scene.scene_purpose" rows="2" /></label>
					  <label class="episode-field full"><span>情绪变化</span><textarea v-model="scene.emotional_change" rows="2" /></label>
					  <div class="scene-character-picker full"><span>在场人物</span><label v-for="character in referenceContext.characters" :key="character.character_id"><input v-model="scene.character_ids" type="checkbox" :value="character.character_id" />{{ character.name }}</label><small v-if="!referenceContext.characters?.length">暂无可绑定的人物资料。</small></div>
					</div>
					<div class="episode-action-editor">
					  <strong>动作 <button type="button" @click="addAction(scene)"><Plus :size="12" />新增</button></strong>
					  <label v-for="(action, actionIndex) in scene.actions" :key="actionIndex">
						<input v-model="selectedBlocks" type="checkbox" :value="selectValue('action', action.action_id)" title="选择为 AI 范围" />
						<span>{{ actionIndex + 1 }}</span><textarea v-model="action.description" rows="2" />
						<i><button type="button" @click="moveAction(scene, action.action_id, -1)"><ArrowUp :size="12" /></button><button type="button" @click="moveAction(scene, action.action_id, 1)"><ArrowDown :size="12" /></button><button type="button" @click="deleteAction(scene, action.action_id)"><Trash2 :size="12" /></button></i>
					  </label>
                    </div>
                  </div>
                  <div v-else class="episode-scene-copy">
                    <div><span>场景目的</span><p>{{ scene.scene_purpose || '—' }}</p></div>
                    <div><span>情绪变化</span><p>{{ scene.emotional_change || '—' }}</p></div>
                    <div v-if="scene.actions?.length" class="episode-action-list"><span>动作</span><ol><li v-for="(action, actionIndex) in scene.actions" :key="actionIndex">{{ actionDescription(action) }}</li></ol></div>
                  </div>

				  <div class="episode-dialogue-list">
					<div class="episode-dialogue-heading"><MessageSquareText :size="15" /><strong>对白与旁白</strong><span>{{ scene.dialogues.length }} 条</span><div v-if="editing"><button @click="addDialogue(scene, 'dialogue')"><Plus :size="12" />对白</button><button @click="addDialogue(scene, 'narration')">旁白</button><button @click="addDialogue(scene, 'inner_monologue')">内心</button><button @click="addDialogue(scene, 'off_screen')">画外音</button></div></div>
					<article v-for="dialogue in scene.dialogues" :key="dialogue.dialogue_id" class="episode-dialogue-row">
					  <template v-if="editing">
						<div class="dialogue-structure-actions"><label><input v-model="selectedBlocks" type="checkbox" :value="selectValue('dialogue', dialogue.dialogue_id)" />AI 选区</label><button @click="moveDialogue(scene, dialogue.dialogue_id, -1)"><ArrowUp :size="12" /></button><button @click="moveDialogue(scene, dialogue.dialogue_id, 1)"><ArrowDown :size="12" /></button><button @click="deleteDialogue(scene, dialogue.dialogue_id)"><Trash2 :size="12" /></button></div>
						<div class="dialogue-edit-meta">
						  <label class="episode-field"><span>类型</span><select v-model="dialogue.dialogue_type"><option value="dialogue">对白</option><option value="narration">旁白</option><option value="inner_monologue">内心独白</option><option value="off_screen">画外音</option></select></label>
						  <label class="episode-field"><span>说话人</span><select v-model="dialogue.character_id" @change="syncDialogueCharacter(dialogue)"><option :value="null">未绑定角色</option><option v-for="character in referenceContext.characters" :key="character.character_id" :value="character.character_id">{{ character.name }}</option></select><input v-model="dialogue.speaker_name" /></label>
                          <label class="episode-field"><span>情绪</span><input v-model="dialogue.emotion" /></label>
                          <label class="episode-field"><span>时长（毫秒）</span><input v-model.number="dialogue.estimated_duration_ms" type="number" min="1" max="600000" /></label>
                        </div>
                        <label class="episode-field"><span>台词</span><textarea v-model="dialogue.text" rows="3" /></label>
                        <label class="episode-field"><span>表演提示</span><input v-model="dialogue.performance_instruction" /></label>
                      </template>
                      <template v-else>
                        <div class="dialogue-speaker"><b>{{ dialogue.speaker_name || '旁白' }}</b><span>{{ dialogue.emotion || dialogue.dialogue_type }}</span></div>
                        <p>{{ dialogue.text }}</p>
                        <small v-if="dialogue.performance_instruction">{{ dialogue.performance_instruction }}</small>
                      </template>
                    </article>
                    <p v-if="!scene.dialogues.length" class="episode-dialogue-empty">本场景暂无对白。</p>
                  </div>
                </article>
			  </div>
			  <aside class="script-reference-sidebar">
				<section><header><AlertTriangle :size="15" /><strong>实时校验</strong><b>{{ diagnostics.length }}</b></header><p v-if="!diagnostics.length" class="reference-empty">未发现结构问题。</p><button v-for="issue in diagnostics" :key="`${issue.code}:${issue.scene_id}:${issue.dialogue_id}:${issue.action_id}`" :class="issue.severity"><code>{{ issue.code }}</code><span>{{ issue.message }}</span></button></section>
				<section><header><FileText :size="15" /><strong>原著事件</strong><b>{{ referenceContext.events?.length || 0 }}</b></header><article v-for="event in referenceContext.events" :key="event.event_revision_id"><code>{{ event.event_revision_id }}</code><p>{{ event.summary }}</p><small>{{ event.location_name || '地点未标注' }} · {{ event.participants?.map(item => item.name).join('、') || '无人物' }}</small></article><p v-if="!referenceContext.events?.length" class="reference-empty">当前场景未引用原著事件。</p></section>
				<section><header><MapPin :size="15" /><strong>Source Span</strong><b>{{ referenceContext.source_spans?.length || 0 }}</b></header><article v-for="span in referenceContext.source_spans" :key="span.source_span_id"><code>{{ span.source_span_id }}</code><p>{{ span.evidence_text || '无证据摘录' }}</p><small>{{ span.chapter_id }} · {{ span.start_codepoint }}–{{ span.end_codepoint }}</small></article></section>
				<section><header><MessageSquareText :size="15" /><strong>人物状态</strong><b>{{ referenceContext.character_states?.length || 0 }}</b></header><article v-for="state in referenceContext.character_states" :key="`${state.character_id}:${state.state_dimension}:${state.trigger_event_revision_id}`"><strong>{{ state.name }} · {{ state.state_dimension }}</strong><p>{{ JSON.stringify(state.before_state) }} → {{ JSON.stringify(state.after_state) }}</p></article></section>
				<section><header><ShieldCheck :size="15" /><strong>must_preserve</strong><b>{{ referenceContext.must_preserve?.length || 0 }}</b></header><article v-for="rule in referenceContext.must_preserve" :key="rule.adaptation_rule_id"><code>{{ rule.enforcement }} · {{ rule.target_type }}</code><p>{{ rule.rationale || rule.target_id || JSON.stringify(rule.parameters) }}</p></article></section>
			  </aside>
			  </div>
			</template>
		  </section>

		  <section v-else class="episode-version-compare">
			<header><div><span>VERSIONED SCRIPT</span><h4>版本并排比较与局部恢复</h4><p>恢复会创建 change plan 和新版本，不覆盖历史版本及其下游产物。</p></div></header>
			<div v-if="versions.length < 2" class="episode-script-empty"><History :size="28" /><h4>尚无可比较的历史版本</h4><p>首次执行修改后，原版本与 successor 会同时保留在这里。</p></div>
			<template v-else>
			  <div class="version-pickers"><label>恢复来源<select v-model="leftVersionId"><option v-for="item in versions" :key="item.entity_version_id" :value="item.entity_version_id">v{{ item.version }} · {{ item.source_type }}{{ item.is_current ? '（当前）' : '' }}</option></select></label><GitCompareArrows :size="20" /><label>比较目标<select v-model="rightVersionId"><option v-for="item in versions" :key="item.entity_version_id" :value="item.entity_version_id">v{{ item.version }} · {{ item.source_type }}{{ item.is_current ? '（当前）' : '' }}</option></select></label></div>
			  <div class="version-diff-table"><div class="head"><b>恢复</b><b>路径</b><b>v{{ leftVersion?.version }}</b><b>v{{ rightVersion?.version }}</b></div><label v-for="row in versionDiff" :key="row.path"><input v-model="selectedRestorePaths" type="checkbox" :value="row.path" /><code>{{ row.kind }} · {{ row.path }}</code><pre>{{ formatDiffValue(row.before) }}</pre><pre>{{ formatDiffValue(row.after) }}</pre></label></div>
			  <button class="button button-primary version-restore-button" :disabled="!selectedRestorePaths.length || saving || leftVersion?.is_current" @click="restoreVersion"><RotateCcw :size="15" />生成所选 {{ selectedRestorePaths.length }} 项的局部恢复计划</button>
			</template>
		  </section>
		</main>

        <footer v-if="editing" class="episode-content-footer">
          <span>{{ changed ? '有未保存的修改' : '尚未修改内容' }}</span>
          <button class="button button-secondary" :disabled="saving" @click="cancelEdit">取消</button>
          <button class="button button-primary" :disabled="!changed || saving" @click="saveContent">
            <LoaderCircle v-if="saving" :size="16" class="spin" /><Save v-else :size="16" />{{ saving ? '保存中…' : '保存修改' }}
          </button>
        </footer>
        <footer v-else class="episode-content-footer view-footer">
          <span><Eye :size="14" />当前为查看模式</span>
          <button class="button button-secondary" @click="requestClose">关闭</button>
        </footer>
        </template>
      </template>
    </article>
  </div>
</template>
