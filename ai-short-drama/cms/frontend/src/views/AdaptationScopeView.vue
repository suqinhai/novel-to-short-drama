<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertTriangle, ArrowLeft, BookOpenCheck, CheckCircle2, CirclePlus, Layers3, LoaderCircle, Play, Trash2 } from 'lucide-vue-next'
import OperationTracker from '../components/OperationTracker.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'
import { buildAdaptationScope, isScopeComplete, selectCurrentFullIR } from '../services/adaptationScope'

const route = useRoute()
const router = useRouter()
const projectId = computed(() => route.params.projectId || '')
const works = ref([])
const versions = ref([])
const chapters = ref([])
const irRevisions = ref([])
const storyArcs = ref([])
const specs = ref([])
const loading = ref(true)
const submitting = ref(false)
const error = ref('')
const specsNotice = ref('')
const success = ref('')
const operation = ref(null)
const compilerOperation = ref(null)
const adaptationPlan = ref(null)
const compilingSpecId = ref('')
const compilerVersion = ref('constraint-compiler-v1')
const planLoading = ref(false)
let pendingSubmit = null
let pendingCompile = null
const selection = reactive({ work_id: '', source_version_id: '', ir_revision_id: '', scope_mode: 'chapters_only', chapter_ids: [], story_arc_revision_ids: [] })
const form = reactive({ display_name: '', platform: '抖音', audience: '', audience_tags: '', target_episode_count: 24, episode_duration_seconds: 120 })
const rules = ref([
  { rule_type: 'must_preserve', enforcement: 'hard', target_type: 'chapter', target_id: '', priority: 100, text: '', rationale: '' },
  { rule_type: 'merge_allowed', enforcement: 'soft', target_type: 'free_text', target_id: null, priority: 50, text: '相邻且叙事功能相同的次要事件允许合并。', rationale: '' },
  { rule_type: 'must_not_change', enforcement: 'hard', target_type: 'free_text', target_id: null, priority: 100, text: '不得改变核心人物关系和关键因果链。', rationale: '' },
])
const publishedVersions = computed(() => versions.value.filter((item) => item.status === 'published'))
const currentFullIR = computed(() => selectCurrentFullIR(irRevisions.value))
const allSelected = computed(() => chapters.value.length > 0 && selection.chapter_ids.length === chapters.value.length)
const allArcsSelected = computed(() => storyArcs.value.length > 0 && selection.story_arc_revision_ids.length === storyArcs.value.length)
const rulesComplete = computed(() => rules.value.length && rules.value.every((rule) => {
  if (rule.target_type === 'chapter') return selection.scope_mode !== 'arcs_only' && selection.chapter_ids.includes(rule.target_id)
  if (rule.target_type === 'story_arc') return selection.scope_mode !== 'chapters_only' && selection.story_arc_revision_ids.includes(rule.target_id)
  return rule.target_type !== 'free_text' || rule.text.trim()
}))
const canSubmit = computed(() => selection.source_version_id && selection.ir_revision_id &&
  isScopeComplete(selection.scope_mode, selection.chapter_ids, selection.story_arc_revision_ids) && rulesComplete.value &&
  (projectId.value || form.display_name.trim()))
const planValidationPassed = computed(() => adaptationPlan.value && Object.values(adaptationPlan.value.validation || {}).every(Boolean))

async function loadSpecs() {
  if (!projectId.value) return
  specsNotice.value = ''
  try {
    specs.value = (await narrativeApi.listAdaptationSpecs(projectId.value)).data
  } catch (err) {
    specsNotice.value = `暂时无法读取该项目的 Spec 历史：${err.message}`
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const response = await narrativeApi.listWorks({ page: 1, limit: 200 })
    works.value = response.data
    await loadSpecs()
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function loadVersions() {
  selection.source_version_id = ''
  selection.ir_revision_id = ''
  selection.chapter_ids = []
  selection.story_arc_revision_ids = []
  chapters.value = []
  irRevisions.value = []
  storyArcs.value = []
  versions.value = []
  if (!selection.work_id) return
  try {
    versions.value = (await narrativeApi.listVersions(selection.work_id)).data
  } catch (err) { error.value = err.message }
}

async function loadChapters() {
  selection.chapter_ids = []
  selection.story_arc_revision_ids = []
  selection.ir_revision_id = ''
  chapters.value = []
  irRevisions.value = []
  storyArcs.value = []
  if (!selection.source_version_id) return
  try {
    const [chapterResponse, irResponse] = await Promise.all([
      narrativeApi.listChapters(selection.source_version_id), narrativeApi.listIRRevisions(selection.source_version_id),
    ])
    chapters.value = chapterResponse.data
    irRevisions.value = irResponse.data
    selection.chapter_ids = chapters.value.map((item) => item.chapter_id)
    selection.ir_revision_id = currentFullIR.value?.ir_revision_id || ''
    if (selection.ir_revision_id) {
      storyArcs.value = (await narrativeApi.listStoryArcs(selection.ir_revision_id)).data
    }
    rules.value.filter((rule) => rule.target_type === 'chapter' && !rule.target_id).forEach((rule) => { rule.target_id = chapters.value[0]?.chapter_id || '' })
  } catch (err) { error.value = err.message }
}

function toggleAll() {
  selection.chapter_ids = allSelected.value ? [] : chapters.value.map((item) => item.chapter_id)
}

function toggleAllArcs() {
  selection.story_arc_revision_ids = allArcsSelected.value ? [] : storyArcs.value.map((item) => item.story_arc_revision_id)
}

function changeScopeMode() {
  for (const rule of rules.value) {
    if (selection.scope_mode === 'arcs_only' && rule.target_type === 'chapter') {
      rule.target_type = 'story_arc'
      rule.target_id = selection.story_arc_revision_ids[0] || storyArcs.value[0]?.story_arc_revision_id || ''
    } else if (selection.scope_mode === 'chapters_only' && rule.target_type === 'story_arc') {
      rule.target_type = 'chapter'
      rule.target_id = selection.chapter_ids[0] || ''
    }
  }
}

function addRule() {
  rules.value.push({ rule_type: 'must_preserve', enforcement: 'hard', target_type: 'free_text', target_id: null, priority: 50, text: '', rationale: '' })
}

function changeTarget(rule) {
  if (rule.target_type === 'chapter') rule.target_id = selection.chapter_ids[0] || ''
  else if (rule.target_type === 'story_arc') rule.target_id = selection.story_arc_revision_ids[0] || ''
  else rule.target_id = null
}

function makeSpec() {
  return {
    schema_version: 'adaptation-spec.v1',
    source_version_id: selection.source_version_id,
    ir_revision_id: selection.ir_revision_id,
    scope: buildAdaptationScope(selection.scope_mode, selection.chapter_ids, selection.story_arc_revision_ids),
    platform: form.platform.trim(),
    audience_profile: {
      description: form.audience.trim(),
      tags: form.audience_tags.split(/[,，]/).map((item) => item.trim()).filter(Boolean),
    },
    target_episode_count: Number(form.target_episode_count),
    episode_duration_seconds: Number(form.episode_duration_seconds),
    rules: rules.value.map((rule) => ({
      rule_type: rule.rule_type,
      enforcement: rule.enforcement,
      target_type: rule.target_type,
      target_id: rule.target_type === 'free_text' ? null : rule.target_id,
      priority: Number(rule.priority),
      parameters: rule.target_type === 'free_text' ? { instruction: rule.text.trim() } : {},
      rationale: rule.rationale.trim(),
    })),
  }
}

async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  error.value = ''
  success.value = ''
  const spec = makeSpec()
  try {
    const payload = projectId.value ? spec : { display_name: form.display_name.trim(), adaptation_spec: spec }
    const signature = JSON.stringify(payload)
    if (pendingSubmit?.signature !== signature) pendingSubmit = { signature, key: createIdempotencyKey(projectId.value ? 'adaptation-spec' : 'adaptation-project') }
    const response = projectId.value
      ? await narrativeApi.createAdaptationSpec(projectId.value, payload, pendingSubmit.key)
      : await narrativeApi.createAdaptationProject(payload, pendingSubmit.key)
    pendingSubmit = null
    operation.value = response.data
    success.value = projectId.value ? '改编规格命令已提交。' : '改编项目创建命令已提交。'
  } catch (err) {
    error.value = err.isConflict ? `${err.message} 请检查是否重复提交或项目状态已变化。` : err.message
  } finally {
    submitting.value = false
  }
}

async function handleSpecTerminal(terminalOperation) {
  if (terminalOperation.status === 'completed') {
    success.value = '改编规格校验完成，正在刷新可编译版本。'
    if (!projectId.value && terminalOperation.target_type === 'project' && terminalOperation.target_id) {
      await router.replace(`/projects/${terminalOperation.target_id}/adaptation-scope`)
    }
  }
  await loadSpecs()
}

async function compileSpec(spec) {
  if (!projectId.value || spec.status !== 'active' || !spec.ir_revision_id || !compilerVersion.value.trim()) return
  error.value = ''
  success.value = ''
  adaptationPlan.value = null
  compilingSpecId.value = spec.adaptation_spec_version_id
  const payload = {
    adaptation_spec_version_id: spec.adaptation_spec_version_id,
    ir_revision_id: spec.ir_revision_id,
    compiler_version: compilerVersion.value.trim(),
  }
  const signature = JSON.stringify(payload)
  if (pendingCompile?.signature !== signature) pendingCompile = { signature, key: createIdempotencyKey('adaptation-compile') }
  try {
    const response = await narrativeApi.startCompilerRun(projectId.value, payload, pendingCompile.key)
    pendingCompile = null
    compilerOperation.value = response.data
    success.value = '改编编译任务已提交。'
  } catch (err) {
    error.value = err.isConflict ? `${err.message} 请刷新 Spec 状态后重试。` : err.message
  } finally {
    compilingSpecId.value = ''
  }
}

async function handleCompilerTerminal(terminalOperation) {
  if (terminalOperation.status !== 'completed' || terminalOperation.result_ref?.resource_type !== 'adaptation_plan') return
  planLoading.value = true
  error.value = ''
  try {
    adaptationPlan.value = (await narrativeApi.getAdaptationPlan(terminalOperation.result_ref.resource_id)).data
    success.value = '改编计划已生成，可查看分集与约束诊断。'
  } catch (err) {
    error.value = `编译已完成，但计划读取失败：${err.message}`
  } finally {
    planLoading.value = false
  }
}

watch(() => selection.work_id, loadVersions)
watch(() => selection.source_version_id, loadChapters)
watch(() => selection.chapter_ids, () => {
  const selected = new Set(selection.chapter_ids)
  rules.value.filter((rule) => rule.target_type === 'chapter' && !selected.has(rule.target_id)).forEach((rule) => { rule.target_id = selection.chapter_ids[0] || '' })
}, { deep: true })
watch(() => selection.story_arc_revision_ids, () => {
  const selected = new Set(selection.story_arc_revision_ids)
  rules.value.filter((rule) => rule.target_type === 'story_arc' && !selected.has(rule.target_id)).forEach((rule) => { rule.target_id = selection.story_arc_revision_ids[0] || '' })
}, { deep: true })
onMounted(load)
</script>

<template>
  <section class="view-stack adaptation-view">
    <RouterLink :to="projectId ? `/projects/${projectId}` : '/projects'" class="back-link"><ArrowLeft :size="16" />{{ projectId ? '返回项目详情' : '返回项目列表' }}</RouterLink>
    <div class="hero-row"><div><h2>{{ projectId ? '配置改编范围' : '从原著创建改编项目' }}</h2><p>选择已发布的原著版本与章节范围，并形成可验证的 Adaptation Spec。</p></div></div>

    <div v-if="loading" class="detail-skeleton"><span></span><span></span><span></span></div>
    <template v-else>
      <div v-if="error" class="error-banner large"><AlertTriangle :size="17" />{{ error }}<button @click="error = ''">关闭</button></div>
      <div v-if="success" class="success-banner"><CheckCircle2 :size="17" />{{ success }}</div>
      <OperationTracker :operation="operation" @terminal="handleSpecTerminal" />
      <OperationTracker :operation="compilerOperation" @terminal="handleCompilerTerminal" />

      <div class="contract-notice arc-unavailable"><BookOpenCheck :size="17" /><div><strong>范围绑定已发布的完整 Narrative IR</strong><span>章节和故事弧都来自同一个 source version 与当前 published/full IR，不推测 ID，也不以向量相似度代替来源。</span></div></div>

      <form class="adaptation-form" @submit.prevent="submit">
        <article class="panel padded">
          <div class="section-title"><div><span>SOURCE SNAPSHOT</span><h3>1. 来源与章节范围</h3></div><BookOpenCheck :size="19" /></div>
          <label v-if="!projectId" class="field"><span>改编项目名 <i>*</i></span><input v-model="form.display_name" maxlength="1000" required /></label>
          <div class="field-pair">
            <label class="field"><span>原著作品 <i>*</i></span><select v-model="selection.work_id" required><option value="">请选择作品</option><option v-for="item in works" :key="item.work_id" :value="item.work_id">{{ item.title }}{{ item.author ? ` · ${item.author}` : '' }}</option></select></label>
            <label class="field"><span>发布版本 <i>*</i></span><select v-model="selection.source_version_id" :disabled="!selection.work_id" required><option value="">请选择已发布版本</option><option v-for="item in publishedVersions" :key="item.source_version_id" :value="item.source_version_id">v{{ item.version_number }} · {{ item.chapter_count }} 章</option></select><small v-if="selection.work_id && !publishedVersions.length">该作品暂无已发布版本。</small></label>
          </div>
          <div v-if="selection.source_version_id && !currentFullIR" class="contract-notice warning">该版本还没有 published/full Narrative IR，请先在版本页面完成测试提取与审核发布。</div>
          <div v-else-if="currentFullIR" class="current-ir-summary"><div><span>当前完整 IR</span><strong>IR r{{ currentFullIR.revision_number }} · {{ currentFullIR.extractor_version }}</strong><code>{{ currentFullIR.ir_revision_id }}</code></div><StatusBadge :status="currentFullIR.status" /></div>
          <label class="field scope-mode-field"><span>改编范围模式 <i>*</i></span><select v-model="selection.scope_mode" :disabled="!currentFullIR" @change="changeScopeMode"><option value="chapters_only">仅章节</option><option value="arcs_only" :disabled="!storyArcs.length">仅故事弧</option><option value="union">章节与故事弧并集</option></select><small>范围中的章节和故事弧会按冻结契约分别提交。</small></label>
          <div v-if="chapters.length && selection.scope_mode !== 'arcs_only'" class="chapter-picker">
            <div class="chapter-picker-head"><div><strong>选择章节</strong><span>已选 {{ selection.chapter_ids.length }} / {{ chapters.length }}</span></div><button type="button" class="button button-secondary" @click="toggleAll">{{ allSelected ? '取消全选' : '选择全部' }}</button></div>
            <label v-for="chapter in chapters" :key="chapter.chapter_id" class="chapter-option"><input v-model="selection.chapter_ids" type="checkbox" :value="chapter.chapter_id" /><span><b>{{ chapter.ordinal }}</b><strong>{{ chapter.title }}</strong><small>r{{ chapter.revision_number }} · {{ Number(chapter.char_count).toLocaleString('zh-CN') }} 字符</small></span></label>
          </div>
          <div v-if="currentFullIR && selection.scope_mode !== 'chapters_only'" class="story-arc-picker">
            <div class="chapter-picker-head"><div><strong>选择故事弧</strong><span>已选 {{ selection.story_arc_revision_ids.length }} / {{ storyArcs.length }}</span></div><button v-if="storyArcs.length" type="button" class="button button-secondary" @click="toggleAllArcs">{{ allArcsSelected ? '取消全选' : '选择全部' }}</button></div>
            <div v-if="!storyArcs.length" class="compact-empty">当前完整 IR 没有可选故事弧。</div>
            <label v-for="arc in storyArcs" :key="arc.story_arc_revision_id" class="story-arc-option"><input v-model="selection.story_arc_revision_ids" type="checkbox" :value="arc.story_arc_revision_id" /><span><strong>{{ arc.title }}</strong><small>{{ arc.arc_type }} · 置信度 {{ Math.round(arc.confidence * 100) }}% · 来源章节 {{ arc.chapter_id }}</small><p v-if="arc.summary">{{ arc.summary }}</p></span></label>
          </div>
        </article>

        <article class="panel padded">
          <div class="section-title"><div><span>DELIVERY PROFILE</span><h3>2. 平台、受众与体量</h3></div><Layers3 :size="19" /></div>
          <div class="field-pair"><label class="field"><span>平台 <i>*</i></span><input v-model="form.platform" maxlength="200" required /></label><label class="field"><span>目标受众</span><input v-model="form.audience" placeholder="例如：18–35 岁都市女性" /></label></div>
          <label class="field"><span>受众标签</span><input v-model="form.audience_tags" placeholder="爽剧，情感，都市（逗号分隔）" /></label>
          <div class="field-pair"><label class="field"><span>目标集数 <i>*</i></span><input v-model.number="form.target_episode_count" type="number" min="1" max="1000" required /></label><label class="field"><span>单集时长（秒） <i>*</i></span><input v-model.number="form.episode_duration_seconds" type="number" min="1" max="7200" required /></label></div>
        </article>

        <article class="panel padded rules-panel">
          <div class="section-title"><div><span>ADAPTATION RULES</span><h3>3. 改编约束</h3></div><button type="button" class="button button-secondary" @click="addRule"><CirclePlus :size="16" />新增规则</button></div>
          <div v-for="(rule, index) in rules" :key="index" class="rule-card">
            <div class="rule-index">{{ index + 1 }}</div>
            <div class="rule-fields">
              <div class="rule-grid">
                <label class="field"><span>规则</span><select v-model="rule.rule_type"><option value="must_preserve">必须保留</option><option value="merge_allowed">允许合并</option><option value="must_not_change">禁止修改</option><option value="omit_allowed">允许省略</option><option value="transform_required">必须转换</option></select></label>
                <label class="field"><span>强度</span><select v-model="rule.enforcement"><option value="hard">硬约束</option><option value="soft">软约束</option></select></label>
                <label class="field"><span>目标类型</span><select v-model="rule.target_type" @change="changeTarget(rule)"><option value="chapter" :disabled="selection.scope_mode === 'arcs_only'">章节</option><option value="story_arc" :disabled="selection.scope_mode === 'chapters_only'">故事弧</option><option value="free_text">自由文本</option></select></label>
                <label class="field"><span>优先级</span><input v-model.number="rule.priority" type="number" min="0" /></label>
              </div>
              <label v-if="rule.target_type === 'chapter'" class="field"><span>目标章节 <i>*</i></span><select v-model="rule.target_id" required><option value="">请选择已纳入范围的章节</option><option v-for="chapter in chapters.filter(item => selection.chapter_ids.includes(item.chapter_id))" :key="chapter.chapter_id" :value="chapter.chapter_id">{{ chapter.ordinal }} · {{ chapter.title }}</option></select></label>
              <label v-else-if="rule.target_type === 'story_arc'" class="field"><span>目标故事弧 <i>*</i></span><select v-model="rule.target_id" required><option value="">请选择已纳入范围的故事弧</option><option v-for="arc in storyArcs.filter(item => selection.story_arc_revision_ids.includes(item.story_arc_revision_id))" :key="arc.story_arc_revision_id" :value="arc.story_arc_revision_id">{{ arc.title }} · {{ arc.arc_type }}</option></select></label>
              <label v-else class="field"><span>规则内容 <i>*</i></span><textarea v-model="rule.text" rows="3" required></textarea></label>
              <label class="field"><span>理由</span><input v-model="rule.rationale" maxlength="4000" placeholder="可选，说明业务理由" /></label>
            </div>
            <button type="button" class="rule-delete" :disabled="rules.length === 1" aria-label="删除规则" @click="rules.splice(index, 1)"><Trash2 :size="17" /></button>
          </div>
        </article>

        <div class="adaptation-submit"><span>将按 <code>adaptation-spec.v1</code> 提交；服务端仍会执行 JSON Schema 和业务校验。</span><button class="button button-primary" :disabled="submitting || !canSubmit">{{ submitting ? '提交中…' : projectId ? '提交改编规格' : '创建改编项目' }}</button></div>
      </form>

      <article v-if="projectId" class="panel spec-history">
        <div class="production-data-head"><div><span>SPEC HISTORY</span><h3>已有规格版本</h3></div><label class="compiler-version-field"><span>编译器版本</span><input v-model="compilerVersion" maxlength="200" /></label></div>
        <div v-if="specsNotice" class="contract-notice warning">{{ specsNotice }}</div>
        <div v-else-if="!specs.length" class="compact-empty">该项目还没有 Adaptation Spec。</div>
        <div v-else class="version-list"><div v-for="item in specs" :key="item.adaptation_spec_version_id" class="version-row"><b>v{{ item.version_number }}</b><div><strong>{{ item.adaptation_spec_version_id }}</strong><code>{{ item.source_version_id }}<template v-if="item.ir_revision_id"> · {{ item.ir_revision_id }}</template></code></div><StatusBadge :status="item.status" /><button v-if="item.status === 'active' && item.ir_revision_id" class="button button-primary" :disabled="compilingSpecId === item.adaptation_spec_version_id || !compilerVersion.trim()" @click="compileSpec(item)"><LoaderCircle v-if="compilingSpecId === item.adaptation_spec_version_id" :size="15" class="spin" /><Play v-else :size="15" />{{ compilingSpecId === item.adaptation_spec_version_id ? '提交中…' : '编译计划' }}</button><small v-else-if="item.status === 'active'" class="spec-unavailable">缺少 IR revision</small></div></div>
      </article>

      <article v-if="planLoading || adaptationPlan" class="panel padded compiler-plan-panel">
        <div class="section-title"><div><span>REVIEWABLE ADAPTATION PLAN</span><h3>改编编译结果</h3></div><StatusBadge v-if="adaptationPlan" :status="planValidationPassed ? 'completed' : 'needs_review'" /></div>
        <div v-if="planLoading" class="compact-empty"><LoaderCircle :size="18" class="spin" />正在读取编译计划……</div>
        <template v-else-if="adaptationPlan">
          <div class="version-stats"><div><span>分集</span><strong>{{ adaptationPlan.episodes?.length || 0 }}</strong></div><div><span>诊断</span><strong>{{ adaptationPlan.diagnostics?.length || 0 }}</strong></div><div><span>硬规则</span><strong>{{ adaptationPlan.validation?.hard_rules_satisfied ? '通过' : '未通过' }}</strong></div><div><span>时间线 / 因果</span><strong>{{ adaptationPlan.validation?.timeline_valid && adaptationPlan.validation?.causality_valid ? '通过' : '检查失败' }}</strong></div></div>
          <div v-if="adaptationPlan.diagnostics?.length" class="compiler-diagnostics"><article v-for="(item, index) in adaptationPlan.diagnostics" :key="`${item.code}-${index}`" :class="item.severity"><strong>{{ item.severity }} · {{ item.code }}</strong><p>{{ item.message }}</p></article></div>
          <div class="compiler-episodes"><article v-for="episode in adaptationPlan.episodes || []" :key="episode.episode_number"><b>第 {{ episode.episode_number }} 集</b><div><strong>{{ episode.title }}</strong><p>{{ episode.logline }}</p><small>{{ episode.estimated_duration_seconds }} 秒 · {{ episode.source_event_ids.length }} 个原著事件 · {{ episode.source_chapter_ids.length }} 章</small></div></article></div>
        </template>
      </article>
    </template>
  </section>
</template>
