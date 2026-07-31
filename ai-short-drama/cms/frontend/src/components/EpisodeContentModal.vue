<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import {
  AlertCircle, AlertTriangle, CheckCircle2, Clock3, Eye, FileText, GitCompareArrows,
  LoaderCircle, MapPin, MessageSquareText, Pencil, Play, Save, ScrollText,
  ShieldCheck, X,
} from 'lucide-vue-next'
import { api } from '../services/api'
import { buildEpisodeContentPayload, cloneEpisodeContent, episodeContentChanged } from '../services/episodeContent'
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

const changed = computed(() => content.value && draft.value && episodeContentChanged(content.value, draft.value))
const current = computed(() => editing.value ? draft.value : content.value)
const script = computed(() => current.value?.script || null)
const planDiff = computed(() => pendingPlan.value?.plan?.expected_changes || [])

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
    savedNotice.value = '修改计划已生成；正式内容尚未改变。'
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
            <article><b>重建范围 / 选择</b><span v-for="item in pendingPlan.plan.impact.rebuild_tasks" :key="item"><input type="checkbox" checked disabled />{{ item }} · 执行后 pending</span><span v-if="!pendingPlan.plan.impact.rebuild_tasks.length">无需重建，可保存并确认</span></article>
          </div>
          <div class="episode-plan-diff">
            <div class="episode-plan-diff-head"><b>字段</b><b>修改前</b><b>修改后</b></div>
            <div v-for="row in planDiff" :key="row.field"><code>{{ row.field }}</code><span>{{ formatDiffValue(row.before) }}</span><span>{{ formatDiffValue(row.after) }}</span></div>
          </div>
          <div class="episode-plan-risks"><b>风险</b><span v-for="risk in pendingPlan.plan.risks" :key="risk"><AlertTriangle :size="12" />{{ risk }}</span></div>
          <footer>
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

          <section v-else class="episode-script-content">
            <div v-if="!script" class="episode-script-empty">
              <ScrollText :size="28" /><h4>本集剧本尚未生成</h4><p>当前仍可在“分集大纲”中查看和修改标题、冲突、钩子等内容。</p>
            </div>
            <template v-else>
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

              <div class="episode-scene-list">
                <article v-for="scene in script.scenes" :key="scene.scene_id" class="episode-scene-card">
                  <header>
                    <b>场景 {{ scene.scene_number }}</b>
                    <span><MapPin :size="13" />{{ scene.location_name || '未指定地点' }}</span>
                    <span><Clock3 :size="13" />{{ scene.time_of_day || '未指定' }} · {{ scene.interior_exterior || '未指定' }} · {{ formatDuration(scene.estimated_duration_seconds) }}</span>
                  </header>

                  <div v-if="editing" class="episode-scene-editor">
                    <div class="episode-edit-grid compact">
                      <label class="episode-field"><span>地点</span><input v-model="scene.location_name" /></label>
                      <label class="episode-field"><span>时间</span><input v-model="scene.time_of_day" /></label>
                      <label class="episode-field"><span>内/外景</span><input v-model="scene.interior_exterior" /></label>
                      <label class="episode-field"><span>时长（秒）</span><input v-model.number="scene.estimated_duration_seconds" type="number" min="1" max="1800" /></label>
                      <label class="episode-field full"><span>场景目的</span><textarea v-model="scene.scene_purpose" rows="2" /></label>
                      <label class="episode-field full"><span>情绪变化</span><textarea v-model="scene.emotional_change" rows="2" /></label>
                    </div>
                    <div v-if="scene.actions.length" class="episode-action-editor">
                      <strong>动作</strong>
                      <label v-for="(action, actionIndex) in scene.actions" :key="actionIndex">
                        <span>{{ actionIndex + 1 }}</span><textarea v-model="action.description" rows="2" />
                      </label>
                    </div>
                  </div>
                  <div v-else class="episode-scene-copy">
                    <div><span>场景目的</span><p>{{ scene.scene_purpose || '—' }}</p></div>
                    <div><span>情绪变化</span><p>{{ scene.emotional_change || '—' }}</p></div>
                    <div v-if="scene.actions?.length" class="episode-action-list"><span>动作</span><ol><li v-for="(action, actionIndex) in scene.actions" :key="actionIndex">{{ actionDescription(action) }}</li></ol></div>
                  </div>

                  <div class="episode-dialogue-list">
                    <div class="episode-dialogue-heading"><MessageSquareText :size="15" /><strong>对白与旁白</strong><span>{{ scene.dialogues.length }} 条</span></div>
                    <article v-for="dialogue in scene.dialogues" :key="dialogue.dialogue_id" class="episode-dialogue-row">
                      <template v-if="editing">
                        <div class="dialogue-edit-meta">
                          <label class="episode-field"><span>类型</span><select v-model="dialogue.dialogue_type"><option value="dialogue">对白</option><option value="narration">旁白</option><option value="inner_monologue">内心独白</option><option value="off_screen">画外音</option></select></label>
                          <label class="episode-field"><span>说话人</span><input v-model="dialogue.speaker_name" /></label>
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
