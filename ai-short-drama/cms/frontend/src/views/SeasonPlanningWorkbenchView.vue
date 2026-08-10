<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { AlertTriangle, ArrowLeft, CheckCircle2, CopyPlus, GitCompareArrows, GripVertical, Layers3, LoaderCircle, Plus, RefreshCw, Save, Scissors, Sparkles, Trash2, WandSparkles } from 'lucide-vue-next'
import { api } from '../services/api'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'
import { applyEventOperation, buildSeasonDraft, compareSeasonPlans, moveEventCard, seasonCurves, validateDraftLocally } from '../services/seasonPlanner'

const route = useRoute()
const projectId = computed(() => route.params.projectId)
const loading = ref(true)
const saving = ref(false)
const approving = ref(false)
const queueing = ref(false)
const generating = ref(false)
const error = ref('')
const success = ref('')
const plans = ref([])
const plan = ref(null)
const draft = ref(null)
const activeTab = ref('episodes')
const selectedCards = ref([])
const draggedCard = ref('')
const serverValidation = ref(null)
const compareIds = ref([])
const comparedPlans = ref([])
const originalSummary = ref('')
const originalRationale = ref('')
const originalEpisode = ref(0)
const strategy = ref('hook_first')
let validationTimer

const localValidation = computed(() => draft.value ? validateDraftLocally(draft.value, plan.value?.rules || [],
  Number(plan.value?.target_episode_duration_seconds || Infinity)) : { passed: false, diagnostics: [], rule_violations: { hard: [], soft: [] } })
const validation = computed(() => serverValidation.value || localValidation.value)
const diagnostics = computed(() => validation.value?.diagnostics || [])
const hardViolations = computed(() => diagnostics.value.filter((item) => item.severity === 'blocking' || item.details?.rule_enforcement === 'hard'))
const softViolations = computed(() => diagnostics.value.filter((item) => item.severity === 'warning' || item.details?.rule_enforcement === 'soft'))
const curves = computed(() => draft.value ? seasonCurves(draft.value) : { emotion: [], information_reveal: [], duration: [] })
const comparison = computed(() => compareSeasonPlans(comparedPlans.value))
const canApprove = computed(() => plan.value?.status === 'waiting_review' && validation.value?.passed)
const canQueue = computed(() => plan.value?.status === 'approved')

const characters = computed(() => {
  const result = new Map()
  for (const episode of draft.value?.episodes || []) for (const card of episode.events) for (const person of card.participants || []) {
    const id = person.entity_revision_id || person
    if (!result.has(id)) result.set(id, { id, name: person.name || id, episodes: [], states: [] })
    const item = result.get(id)
    if (!item.episodes.includes(episode.episode_number)) item.episodes.push(episode.episode_number)
    item.states.push(...(card.character_states || []).filter((state) => state.character_entity_revision_id === id))
  }
  return [...result.values()]
})

const relationships = computed(() => {
  const pairs = new Map()
  for (const episode of draft.value?.episodes || []) for (const card of episode.events) {
    const people = uniqueBy((card.participants || []).map((person) => ({ id: person.entity_revision_id || person, name: person.name || person })), 'id')
    for (let left = 0; left < people.length; left += 1) for (let right = left + 1; right < people.length; right += 1) {
      const key = [people[left].id, people[right].id].sort().join('::')
      if (!pairs.has(key)) pairs.set(key, { key, left: people[left], right: people[right], episodes: [] })
      if (!pairs.get(key).episodes.includes(episode.episode_number)) pairs.get(key).episodes.push(episode.episode_number)
    }
  }
  return [...pairs.values()]
})

const foreshadows = computed(() => {
  const threads = new Map()
  for (const episode of draft.value?.episodes || []) for (const card of episode.events) for (const item of card.foreshadowing || []) {
    if (!threads.has(item.foreshadow_thread_id)) threads.set(item.foreshadow_thread_id, { id: item.foreshadow_thread_id, title: item.title || item.foreshadow_thread_id, occurrences: [] })
    threads.get(item.foreshadow_thread_id).occurrences.push({ episode: episode.episode_number, stage: item.lifecycle_stage, card: card.summary })
  }
  return [...threads.values()]
})

function uniqueBy(items, key) {
  return [...new Map(items.map((item) => [item[key], item])).values()]
}

function curvePoints(values, width = 760, height = 150) {
  if (!values.length) return ''
  const max = Math.max(1, ...values)
  return values.map((value, index) => `${values.length === 1 ? width / 2 : index * width / (values.length - 1)},${height - Number(value) / max * (height - 18) - 9}`).join(' ')
}

async function loadPlans(preferredId = '') {
  plans.value = (await narrativeApi.listSeasonPlans(projectId.value)).data || []
  const target = preferredId || plan.value?.adaptation_plan_id || plans.value[0]?.adaptation_plan_id
  if (target) await loadPlan(target)
}

async function loadPlan(planId) {
  loading.value = true
  error.value = ''
  try {
    plan.value = (await narrativeApi.getAdaptationPlan(planId)).data
    draft.value = buildSeasonDraft(plan.value)
    serverValidation.value = plan.value.latest_validation?.validator_version ? plan.value.latest_validation : null
    selectedCards.value = []
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

function changed() {
  serverValidation.value = null
  clearTimeout(validationTimer)
  validationTimer = setTimeout(validateWithServer, 280)
}

async function validateWithServer() {
  if (!plan.value?.adaptation_plan_id || !draft.value) return
  try {
    serverValidation.value = (await narrativeApi.validateSeasonPlan(plan.value.adaptation_plan_id, draft.value)).data
  } catch (err) {
    error.value = `校验失败：${err.message}`
  }
}

function toggleCard(cardId) {
  selectedCards.value = selectedCards.value.includes(cardId)
    ? selectedCards.value.filter((id) => id !== cardId) : [...selectedCards.value, cardId]
}

function runOperation(operation, options = {}) {
  draft.value = applyEventOperation(draft.value, operation, selectedCards.value, options)
  selectedCards.value = []
  changed()
}

function addOriginal() {
  draft.value = applyEventOperation(draft.value, 'original', [], { episode_index: originalEpisode.value,
    summary: originalSummary.value, rationale: originalRationale.value })
  originalSummary.value = ''
  originalRationale.value = ''
  changed()
}

function dropAt(episodeIndex, cardIndex) {
  if (!draggedCard.value) return
  draft.value = moveEventCard(draft.value, draggedCard.value, episodeIndex, cardIndex)
  draggedCard.value = ''
  changed()
}

async function saveVersion() {
  if (!draft.value || saving.value) return
  saving.value = true
  error.value = ''
  try {
    const response = await narrativeApi.createSeasonPlanVersion(plan.value.adaptation_plan_id, draft.value, createIdempotencyKey('season-plan-version'))
    success.value = '已保存为新的 adaptation plan version；原计划保持不变。'
    await loadPlans(response.data.adaptation_plan_id)
  } catch (err) { error.value = err.message } finally { saving.value = false }
}

async function approve() {
  if (!plan.value || approving.value) return
  approving.value = true
  error.value = ''
  try {
    const response = await narrativeApi.approveSeasonPlan(plan.value.adaptation_plan_id)
    serverValidation.value = response.data.validation
    success.value = '批准完成。校验已记录，尚未建立生产队列。'
    await loadPlans(plan.value.adaptation_plan_id)
  } catch (err) {
    if (err.data?.validation) serverValidation.value = err.data.validation
    error.value = err.message
  } finally { approving.value = false }
}

async function createQueue() {
  if (!canQueue.value || queueing.value) return
  queueing.value = true
  error.value = ''
  try {
    await api.adoptRollingPlan(projectId.value, plan.value.adaptation_plan_id, { max_video_batch: 5, currency: 'CNY' })
    success.value = '已从批准版本建立滚动单集生产队列。'
  } catch (err) { error.value = err.message } finally { queueing.value = false }
}

async function generateAlternative() {
  if (generating.value) return
  generating.value = true
  error.value = ''
  try {
    const specs = (await narrativeApi.listAdaptationSpecs(projectId.value)).data || []
    const current = specs.find((item) => item.status === 'active' && item.ir_revision_id)
    if (!current) throw new Error('没有可用的 active adaptation spec。')
    const constraintProfiles = {
      hook_first: { ending_hook_min: 0.75 }, emotion_wave: { minimum_emotion_range: 0.35 },
      character_arc: { character_arc_min_beats: 1 }, information_control: { information_reveal_per_episode_max: 0.55 },
    }
    const operation = (await narrativeApi.startCompilerRun(projectId.value, {
      adaptation_spec_version_id: current.adaptation_spec_version_id, ir_revision_id: current.ir_revision_id,
      compiler_version: `constraint-v2:${strategy.value}:${Date.now()}`,
      planning_constraints: constraintProfiles[strategy.value],
    }, createIdempotencyKey('season-alternative'))).data
    for (let attempt = 0; attempt < 60; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 1000))
      const state = (await narrativeApi.getOperation(operation.operation_id)).data
      if (['failed', 'cancelled', 'partially_failed'].includes(state.status)) throw new Error(state.error?.message || '方案生成失败')
      if (state.result_ref?.resource_type === 'adaptation_plan') {
        await loadPlans(state.result_ref.resource_id)
        success.value = `已生成 ${strategy.value} 整季方案。`
        return
      }
    }
    throw new Error('方案仍在生成，可稍后刷新方案列表。')
  } catch (err) { error.value = err.message } finally { generating.value = false }
}

async function updateComparison() {
  comparedPlans.value = await Promise.all(compareIds.value.map(async (id) => (await narrativeApi.getAdaptationPlan(id)).data))
  activeTab.value = 'compare'
}

onMounted(async () => {
  try { await loadPlans(String(route.query.plan || '')) } catch (err) { error.value = err.message; loading.value = false }
})
</script>

<template>
  <section class="season-workbench">
    <header class="season-toolbar">
      <RouterLink class="button button-secondary" :to="`/projects/${projectId}/adaptation-scope`"><ArrowLeft :size="15" />改编范围</RouterLink>
      <div class="season-plan-picker">
        <span>整季方案</span>
        <select :value="plan?.adaptation_plan_id" @change="loadPlan($event.target.value)">
          <option v-for="item in plans" :key="item.adaptation_plan_id" :value="item.adaptation_plan_id">v{{ item.version_number }} · {{ item.plan_name }} · {{ item.status }}</option>
        </select>
      </div>
      <select v-model="strategy" class="season-strategy"><option value="hook_first">钩子优先</option><option value="emotion_wave">情绪波浪</option><option value="character_arc">人物弧优先</option><option value="information_control">信息控制</option></select>
      <button class="button button-secondary" :disabled="generating" @click="generateAlternative"><LoaderCircle v-if="generating" :size="15" class="spin" /><Sparkles v-else :size="15" />生成新方案</button>
      <button class="button button-secondary" :disabled="saving || !draft" @click="saveVersion"><LoaderCircle v-if="saving" :size="15" class="spin" /><Save v-else :size="15" />保存新版本</button>
      <button class="button button-primary" :disabled="approving || !canApprove" @click="approve"><LoaderCircle v-if="approving" :size="15" class="spin" /><CheckCircle2 v-else :size="15" />重新校验并批准</button>
      <button class="button button-secondary" :disabled="queueing || !canQueue" @click="createQueue"><Layers3 :size="15" />建立单集队列</button>
    </header>

    <div v-if="error" class="season-message error"><AlertTriangle :size="16" />{{ error }}</div>
    <div v-if="success" class="season-message success"><CheckCircle2 :size="16" />{{ success }}</div>
    <div v-if="loading" class="season-loading"><LoaderCircle :size="24" class="spin" />加载整季工作台…</div>

    <template v-else-if="draft">
      <nav class="season-tabs">
        <button v-for="tab in [{id:'episodes',label:'分集编排'},{id:'season',label:'整季曲线'},{id:'characters',label:'人物弧'},{id:'relationships',label:'关系弧'},{id:'foreshadow',label:'伏笔生命周期'},{id:'compare',label:'方案比较'}]" :key="tab.id" :class="{ active: activeTab === tab.id }" @click="activeTab = tab.id">{{ tab.label }}</button>
      </nav>

      <div v-if="activeTab === 'episodes'" class="season-layout">
        <main class="season-board-wrap">
          <div class="season-operations">
            <strong>已选 {{ selectedCards.length }} 个事件</strong>
            <button :disabled="selectedCards.length < 2" @click="runOperation('merge')"><CopyPlus :size="14" />合并事件</button>
            <button :disabled="selectedCards.length !== 1" @click="runOperation('split')"><Scissors :size="14" />拆分呈现</button>
            <button :disabled="!selectedCards.length" @click="runOperation('omit')"><Trash2 :size="14" />允许省略</button>
            <button :disabled="!selectedCards.length" @click="runOperation('transform')"><WandSparkles :size="14" />变形改编</button>
          </div>
          <div class="season-board">
            <article v-for="(episode, episodeIndex) in draft.episodes" :key="episode.episode_number" class="season-episode" @dragover.prevent @drop="dropAt(episodeIndex, episode.events.length)">
              <header><b>EP {{ String(episode.episode_number).padStart(2, '0') }}</b><input v-model="episode.title" @input="changed" /></header>
              <div class="season-episode-brief">
                <label><span>开场 3 秒</span><textarea v-model="episode.three_second_opening" rows="2" @input="changed"></textarea></label>
                <label><span>前 30 秒目标</span><textarea v-model="episode.first_thirty_seconds_goal" rows="2" @input="changed"></textarea></label>
                <label><span>核心冲突</span><textarea v-model="episode.core_conflict" rows="2" @input="changed"></textarea></label>
                <label><span>高潮</span><textarea v-model="episode.climax" rows="2" @input="changed"></textarea></label>
                <label><span>结尾钩子</span><textarea v-model="episode.ending_hook" rows="2" @input="changed"></textarea></label>
                <div class="season-inline-fields"><label><span>信息揭示</span><input v-model.number="episode.information_reveal_amount" type="number" min="0" max="1" step="0.05" @input="changed" /></label><label><span>预计时长</span><input v-model.number="episode.estimated_duration_seconds" type="number" min="1" @input="changed" /></label></div>
              </div>
              <div class="episode-emotion-mini"><span>情绪曲线</span><svg viewBox="0 0 220 54" preserveAspectRatio="none"><polyline :points="curvePoints(episode.emotion_curve.map(point => Number(point.emotion ?? point)),220,54)" /></svg></div>
              <div class="event-stack">
                <div v-if="!episode.events.length" class="event-drop-empty">拖入 Narrative IR 事件</div>
                <article v-for="(card, cardIndex) in episode.events" :key="card.card_id" class="narrative-event-card" :class="[card.presentation_mode,{selected:selectedCards.includes(card.card_id)}]" draggable="true" @dragstart="draggedCard = card.card_id" @dragover.prevent @drop.stop="dropAt(episodeIndex, cardIndex)" @click="toggleCard(card.card_id)">
                  <GripVertical :size="15" class="event-grip" /><div class="event-main"><div class="event-tags"><b>{{ card.presentation_mode }}</b><span v-for="chapter in card.source_chapter_ids" :key="chapter">{{ card.chapter_title || chapter }}</span><i>重要度 {{ Number(card.importance || 0).toFixed(2) }}</i></div><strong>{{ card.summary }}</strong>
                    <div class="event-audit"><span v-if="card.participants?.length">人物 {{ card.participants.map(item => item.name || item).join('、') }}</span><span v-if="card.character_states?.length">状态 {{ card.character_states.map(item => item.state_dimension).join('、') }}</span><span v-if="card.foreshadowing?.length">伏笔 {{ card.foreshadowing.map(item => `${item.title || item.foreshadow_thread_id}:${item.lifecycle_stage}`).join('、') }}</span></div>
                  </div>
                </article>
              </div>
            </article>
          </div>
          <section class="season-omitted"><h3>明确省略区 <span>{{ draft.omitted_events.length }}</span></h3><div><article v-for="card in draft.omitted_events" :key="card.card_id" class="narrative-event-card omit" draggable="true" @dragstart="draggedCard = card.card_id"><strong>{{ card.summary }}</strong><small>{{ card.rationale }}</small></article><p v-if="!draft.omitted_events.length">没有省略事件</p></div></section>
        </main>

        <aside class="season-rules-panel">
          <header><div><span>LIVE VALIDATION</span><h3>规则与校验</h3></div><button @click="validateWithServer"><RefreshCw :size="14" /></button></header>
          <div class="validation-summary" :class="{ passed: validation?.passed }"><strong>{{ validation?.passed ? '可批准' : '存在阻断' }}</strong><span>{{ hardViolations.length }} hard · {{ softViolations.length }} soft</span></div>
          <section><h4>Hard rules</h4><article v-for="(item,index) in hardViolations" :key="`hard-${index}`" class="rule-violation hard"><b>{{ item.code }}</b><p>{{ item.message }}</p></article><p v-if="!hardViolations.length" class="no-violations">无 hard rule 违反</p></section>
          <section><h4>Soft rules</h4><article v-for="(item,index) in softViolations" :key="`soft-${index}`" class="rule-violation soft"><b>{{ item.code }}</b><p>{{ item.message }}</p></article><p v-if="!softViolations.length" class="no-violations">无 soft rule 提醒</p></section>
          <section class="original-add"><h4><Plus :size="14" />原创补充</h4><select v-model.number="originalEpisode"><option v-for="(episode,index) in draft.episodes" :key="index" :value="index">第 {{ index + 1 }} 集</option></select><textarea v-model="originalSummary" rows="3" placeholder="原创内容"></textarea><textarea v-model="originalRationale" rows="2" placeholder="补充理由（必填）"></textarea><button class="button button-secondary" @click="addOriginal"><Plus :size="14" />加入本集</button></section>
        </aside>
      </div>

      <section v-else-if="activeTab === 'season'" class="season-insight-panel">
        <header><span>SEASON SHAPE</span><h2>整季节奏、情绪与信息揭示</h2></header>
        <div class="curve-card"><h3>情绪曲线</h3><svg viewBox="0 0 760 150" preserveAspectRatio="none"><polyline class="emotion" :points="curvePoints(curves.emotion)" /></svg><div class="curve-labels"><span v-for="(_,index) in curves.emotion" :key="index">EP{{ index+1 }}</span></div></div>
        <div class="curve-card"><h3>信息揭示量</h3><svg viewBox="0 0 760 150" preserveAspectRatio="none"><polyline class="info" :points="curvePoints(curves.information_reveal)" /></svg><div class="curve-labels"><span v-for="(_,index) in curves.information_reveal" :key="index">EP{{ index+1 }}</span></div></div>
        <div class="duration-bars"><article v-for="(value,index) in curves.duration" :key="index"><span>EP{{ index+1 }}</span><i :style="{height:`${Math.max(8,value/Math.max(...curves.duration)*120)}px`}"></i><b>{{ value }}s</b></article></div>
      </section>

      <section v-else-if="activeTab === 'characters'" class="season-insight-panel"><header><span>CHARACTER ARCS</span><h2>人物状态与出场弧</h2></header><div class="arc-grid"><article v-for="item in characters" :key="item.id"><h3>{{ item.name }}</h3><p>覆盖集数：{{ item.episodes.map(value=>`EP${value}`).join(' → ') }}</p><div v-for="state in item.states" :key="state.state_change_id"><b>{{ state.state_dimension }}</b><code>{{ state.before_state }} → {{ state.after_state }}</code></div></article><p v-if="!characters.length">当前事件未标注人物。</p></div></section>
      <section v-else-if="activeTab === 'relationships'" class="season-insight-panel"><header><span>RELATIONSHIP ARCS</span><h2>人物关系共现弧</h2></header><div class="arc-grid"><article v-for="item in relationships" :key="item.key"><h3>{{ item.left.name }} ↔ {{ item.right.name }}</h3><p>关系推进：{{ item.episodes.map(value=>`EP${value}`).join(' → ') }}</p></article><p v-if="!relationships.length">需要至少两名人物共同参与事件。</p></div></section>
      <section v-else-if="activeTab === 'foreshadow'" class="season-insight-panel"><header><span>FORESHADOW LIFECYCLE</span><h2>伏笔生命周期</h2></header><div class="foreshadow-lanes"><article v-for="thread in foreshadows" :key="thread.id"><h3>{{ thread.title }}</h3><div><span v-for="item in thread.occurrences" :key="`${item.episode}-${item.stage}`"><b>EP{{ item.episode }}</b>{{ item.stage }}</span></div></article><p v-if="!foreshadows.length">当前方案没有伏笔标注。</p></div></section>

      <section v-else class="season-insight-panel"><header><span>ALTERNATIVES</span><h2>多个整季方案比较</h2></header><div class="compare-picker"><label v-for="item in plans" :key="item.adaptation_plan_id"><input v-model="compareIds" type="checkbox" :value="item.adaptation_plan_id" :disabled="compareIds.length >= 4 && !compareIds.includes(item.adaptation_plan_id)" />v{{ item.version_number }} {{ item.plan_name }}</label><button class="button button-primary" :disabled="compareIds.length < 2" @click="updateComparison"><GitCompareArrows :size="15" />比较已选方案</button></div><div class="comparison-table"><article v-for="item in comparison" :key="item.adaptation_plan_id"><h3>{{ item.plan_name }}</h3><dl><div><dt>分集</dt><dd>{{ item.episode_count }}</dd></div><div><dt>总时长</dt><dd>{{ item.total_duration_seconds }}s</dd></div><div><dt>平均情绪峰值</dt><dd>{{ item.average_emotion.toFixed(2) }}</dd></div><div><dt>平均信息揭示</dt><dd>{{ item.average_information_reveal.toFixed(2) }}</dd></div><div><dt>阻断</dt><dd>{{ item.blocking_violations }}</dd></div></dl></article></div></section>
    </template>
  </section>
</template>
