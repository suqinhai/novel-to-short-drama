<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { AlertTriangle, ArrowLeft, BookOpenCheck, CheckCircle2, CirclePlus, Layers3, Trash2 } from 'lucide-vue-next'
import OperationTracker from '../components/OperationTracker.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'

const route = useRoute()
const projectId = computed(() => route.params.projectId || '')
const specWritesEnabled = import.meta.env.VITE_ADAPTATION_SPEC_WRITES_ENABLED === 'true'
const works = ref([])
const versions = ref([])
const chapters = ref([])
const specs = ref([])
const loading = ref(true)
const submitting = ref(false)
const error = ref('')
const specsNotice = ref('')
const success = ref('')
const operation = ref(null)
let pendingSubmit = null
const selection = reactive({ work_id: '', source_version_id: '', chapter_ids: [] })
const form = reactive({ display_name: '', platform: '抖音', audience: '', audience_tags: '', target_episode_count: 24, episode_duration_seconds: 120 })
const rules = ref([
  { rule_type: 'must_preserve', enforcement: 'hard', target_type: 'chapter', target_id: '', priority: 100, text: '', rationale: '' },
  { rule_type: 'merge_allowed', enforcement: 'soft', target_type: 'free_text', target_id: null, priority: 50, text: '相邻且叙事功能相同的次要事件允许合并。', rationale: '' },
  { rule_type: 'must_not_change', enforcement: 'hard', target_type: 'free_text', target_id: null, priority: 100, text: '不得改变核心人物关系和关键因果链。', rationale: '' },
])
const publishedVersions = computed(() => versions.value.filter((item) => item.status === 'published'))
const allSelected = computed(() => chapters.value.length > 0 && selection.chapter_ids.length === chapters.value.length)
const canSubmit = computed(() => specWritesEnabled && selection.source_version_id && selection.chapter_ids.length && rules.value.length && (!rules.value.some(rule => rule.target_type === 'chapter' && !rule.target_id)) && (projectId.value || form.display_name.trim()))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const response = await narrativeApi.listWorks({ page: 1, limit: 200 })
    works.value = response.data
    if (projectId.value) {
      try {
        specs.value = (await narrativeApi.listAdaptationSpecs(projectId.value)).data
      } catch (err) {
        specsNotice.value = `暂时无法读取该项目的 Spec 历史：${err.message}`
      }
    }
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function loadVersions() {
  selection.source_version_id = ''
  selection.chapter_ids = []
  chapters.value = []
  versions.value = []
  if (!selection.work_id) return
  try {
    versions.value = (await narrativeApi.listVersions(selection.work_id)).data
  } catch (err) { error.value = err.message }
}

async function loadChapters() {
  selection.chapter_ids = []
  chapters.value = []
  if (!selection.source_version_id) return
  try {
    chapters.value = (await narrativeApi.listChapters(selection.source_version_id)).data
    selection.chapter_ids = chapters.value.map((item) => item.chapter_id)
    rules.value.filter((rule) => rule.target_type === 'chapter' && !rule.target_id).forEach((rule) => { rule.target_id = chapters.value[0]?.chapter_id || '' })
  } catch (err) { error.value = err.message }
}

function toggleAll() {
  selection.chapter_ids = allSelected.value ? [] : chapters.value.map((item) => item.chapter_id)
}

function addRule() {
  rules.value.push({ rule_type: 'must_preserve', enforcement: 'hard', target_type: 'free_text', target_id: null, priority: 50, text: '', rationale: '' })
}

function changeTarget(rule) {
  rule.target_id = rule.target_type === 'chapter' ? (selection.chapter_ids[0] || '') : null
}

function makeSpec() {
  return {
    schema_version: 'adaptation-spec.v1',
    source_version_id: selection.source_version_id,
    scope: { mode: 'chapters_only', chapter_ids: [...selection.chapter_ids], story_arc_revision_ids: [] },
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
  if (!specWritesEnabled) {
    error.value = 'Adaptation Spec 写入尚未启用；当前页面仅用于配置和核对范围。'
    return
  }
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

watch(() => selection.work_id, loadVersions)
watch(() => selection.source_version_id, loadChapters)
watch(() => selection.chapter_ids, () => {
  const selected = new Set(selection.chapter_ids)
  rules.value.filter((rule) => rule.target_type === 'chapter' && !selected.has(rule.target_id)).forEach((rule) => { rule.target_id = selection.chapter_ids[0] || '' })
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
      <OperationTracker :operation="operation" />

      <div class="contract-notice warning arc-unavailable"><AlertTriangle :size="17" /><div><strong>当前采用仅章节范围模式</strong><span>冻结的 v2 契约没有故事弧读取接口，也没有 IR revision 列表接口，无法安全选择故事弧。此页固定提交 <code>chapters_only</code>，不会推测 ID 或调用未定义端点。</span></div></div>
      <div v-if="!specWritesEnabled" class="contract-notice warning"><AlertTriangle :size="17" /><div><strong>Adaptation Spec 写入未启用</strong><span>当前后端只启用了 Source Library 和 IR 启动接口。页面可用于配置范围，但提交按钮保持禁用；待 spec-validation worker 上线后，通过 <code>VITE_ADAPTATION_SPEC_WRITES_ENABLED=true</code> 显式开启。</span></div></div>

      <form class="adaptation-form" @submit.prevent="submit">
        <article class="panel padded">
          <div class="section-title"><div><span>SOURCE SNAPSHOT</span><h3>1. 来源与章节范围</h3></div><BookOpenCheck :size="19" /></div>
          <label v-if="!projectId" class="field"><span>改编项目名 <i>*</i></span><input v-model="form.display_name" maxlength="1000" required /></label>
          <div class="field-pair">
            <label class="field"><span>原著作品 <i>*</i></span><select v-model="selection.work_id" required><option value="">请选择作品</option><option v-for="item in works" :key="item.work_id" :value="item.work_id">{{ item.title }}{{ item.author ? ` · ${item.author}` : '' }}</option></select></label>
            <label class="field"><span>发布版本 <i>*</i></span><select v-model="selection.source_version_id" :disabled="!selection.work_id" required><option value="">请选择已发布版本</option><option v-for="item in publishedVersions" :key="item.source_version_id" :value="item.source_version_id">v{{ item.version_number }} · {{ item.chapter_count }} 章</option></select><small v-if="selection.work_id && !publishedVersions.length">该作品暂无已发布版本。</small></label>
          </div>
          <div v-if="chapters.length" class="chapter-picker">
            <div class="chapter-picker-head"><div><strong>选择章节</strong><span>已选 {{ selection.chapter_ids.length }} / {{ chapters.length }}</span></div><button type="button" class="button button-secondary" @click="toggleAll">{{ allSelected ? '取消全选' : '选择全部' }}</button></div>
            <label v-for="chapter in chapters" :key="chapter.chapter_id" class="chapter-option"><input v-model="selection.chapter_ids" type="checkbox" :value="chapter.chapter_id" /><span><b>{{ chapter.ordinal }}</b><strong>{{ chapter.title }}</strong><small>r{{ chapter.revision_number }} · {{ Number(chapter.char_count).toLocaleString('zh-CN') }} 字符</small></span></label>
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
                <label class="field"><span>目标类型</span><select v-model="rule.target_type" @change="changeTarget(rule)"><option value="chapter">章节</option><option value="free_text">自由文本</option></select></label>
                <label class="field"><span>优先级</span><input v-model.number="rule.priority" type="number" min="0" /></label>
              </div>
              <label v-if="rule.target_type === 'chapter'" class="field"><span>目标章节 <i>*</i></span><select v-model="rule.target_id" required><option value="">请选择已纳入范围的章节</option><option v-for="chapter in chapters.filter(item => selection.chapter_ids.includes(item.chapter_id))" :key="chapter.chapter_id" :value="chapter.chapter_id">{{ chapter.ordinal }} · {{ chapter.title }}</option></select></label>
              <label v-else class="field"><span>规则内容 <i>*</i></span><textarea v-model="rule.text" rows="3" required></textarea></label>
              <label class="field"><span>理由</span><input v-model="rule.rationale" maxlength="4000" placeholder="可选，说明业务理由" /></label>
            </div>
            <button type="button" class="rule-delete" :disabled="rules.length === 1" aria-label="删除规则" @click="rules.splice(index, 1)"><Trash2 :size="17" /></button>
          </div>
        </article>

        <div class="adaptation-submit"><span>将按 <code>adaptation-spec.v1</code> 提交；服务端仍会执行 JSON Schema 和业务校验。</span><button class="button button-primary" :disabled="submitting || !canSubmit">{{ submitting ? '提交中…' : projectId ? '提交改编规格' : '创建改编项目' }}</button></div>
      </form>

      <article v-if="projectId" class="panel spec-history">
        <div class="production-data-head"><div><span>SPEC HISTORY</span><h3>已有规格版本</h3></div><p>{{ specs.length }} 个版本</p></div>
        <div v-if="specsNotice" class="contract-notice warning">{{ specsNotice }}</div>
        <div v-else-if="!specs.length" class="compact-empty">该项目还没有 Adaptation Spec。</div>
        <div v-else class="version-list"><div v-for="item in specs" :key="item.adaptation_spec_version_id" class="version-row"><b>v{{ item.version_number }}</b><div><strong>{{ item.adaptation_spec_version_id }}</strong><code>{{ item.source_version_id }}</code></div><StatusBadge :status="item.status" /></div></div>
      </article>
    </template>
  </section>
</template>
