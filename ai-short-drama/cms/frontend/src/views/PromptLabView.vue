<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Beaker, Check, ClipboardCheck, Eye, FlaskConical, GitBranch, Play, Plus, Rocket, ShieldCheck } from 'lucide-vue-next'
import { api } from '../services/api'

const categories = [
  ['novel_analysis', '小说分析'], ['narrative_ir', '叙事 IR'], ['episode_planning', '分集'],
  ['script', '剧本'], ['storyboard', '分镜'], ['image', '图片'], ['video', '视频'],
  ['tts', 'TTS'], ['qc', 'QC'],
]
const category = ref('novel_analysis')
const templates = ref([])
const fixtures = ref([])
const suites = ref([])
const experiments = ref([])
const activeTemplateId = ref('')
const activeExperiment = ref(null)
const blindExperiment = ref(null)
const preview = ref(null)
const loading = ref(false)
const error = ref('')
const notice = ref('')
const actor = ref('cms-admin')

const templateForm = reactive({ prompt_key: '', display_name: '', description: '' })
const versionForm = reactive({
  system_template: '', user_template: '', change_note: '',
  variable_schema: '{\n  "type": "object",\n  "required": [],\n  "properties": {}\n}',
  default_variables: '{}', model_defaults: '{\n  "temperature": 0.7\n}',
})
const previewVariables = ref('{}')
const fixtureForm = reactive({ fixture_key: '', display_name: '', variables: '{}', expected_output: '{}' })
const suiteForm = reactive({ display_name: '', fixture_ids: [], metric_config: '{"json_valid": true, "expected_overlap": true}' })
const experimentForm = reactive({
  display_name: '', prompt_test_suite_id: '', blind_review: true,
  variants: [newVariant(), newVariant()],
})
const resultForm = reactive({ prompt_experiment_variant_id: '', prompt_fixture_id: '', output: '{}', latency_ms: '', estimated_cost: 0 })
const blindForm = reactive({ prompt_fixture_id: '', blind_label: '', reviewer: '', score: 80, rubric_scores: '{}', comment: '' })

const activeTemplate = computed(() => templates.value.find((item) => item.prompt_template_id === activeTemplateId.value))
const promptVersions = computed(() => templates.value.flatMap((item) => item.versions.map((version) => ({ ...version, template_name: item.display_name }))))
const selectedVersion = computed(() => activeTemplate.value?.versions?.[0] || null)
const suiteFixtures = computed(() => {
  const suite = suites.value.find((item) => item.prompt_test_suite_id === experimentForm.prompt_test_suite_id)
  return fixtures.value.filter((item) => suite?.fixture_ids.includes(item.prompt_fixture_id))
})
const activeSuiteFixtures = computed(() => {
  const suite = suites.value.find((item) => item.prompt_test_suite_id === activeExperiment.value?.prompt_test_suite_id)
  return fixtures.value.filter((item) => suite?.fixture_ids.includes(item.prompt_fixture_id))
})
const categoryName = computed(() => categories.find(([key]) => key === category.value)?.[1] || category.value)

function newVariant() { return { prompt_version_id: '', provider: 'litellm', model: '', parameters: '{"temperature": 0.7}', seed: 42 } }
function parseJSON(value, label) {
  try { return JSON.parse(value || '{}') }
  catch { throw new Error(`${label} 不是有效 JSON`) }
}
function resetMessages() { error.value = ''; notice.value = '' }

async function load() {
  loading.value = true
  resetMessages()
  try {
    ;[templates.value, fixtures.value, suites.value, experiments.value] = await Promise.all([
      api.getPromptTemplates(category.value), api.getPromptFixtures(category.value),
      api.getPromptTestSuites(category.value), api.getPromptExperiments(category.value),
    ])
    if (!templates.value.some((item) => item.prompt_template_id === activeTemplateId.value)) activeTemplateId.value = templates.value[0]?.prompt_template_id || ''
    if (!experimentForm.prompt_test_suite_id) experimentForm.prompt_test_suite_id = suites.value[0]?.prompt_test_suite_id || ''
  } catch (err) { error.value = err.message }
  finally { loading.value = false }
}

watch(category, async () => {
  activeTemplateId.value = ''
  activeExperiment.value = null
  blindExperiment.value = null
  Object.assign(experimentForm, { display_name: '', prompt_test_suite_id: '', variants: [newVariant(), newVariant()] })
  await load()
})

async function createTemplate() {
  resetMessages()
  try {
    const created = await api.createPromptTemplate({ ...templateForm, category: category.value, created_by: actor.value })
    Object.assign(templateForm, { prompt_key: '', display_name: '', description: '' })
    await load(); activeTemplateId.value = created.prompt_template_id; notice.value = 'Prompt 已建立；请创建第一个不可变版本。'
  } catch (err) { error.value = err.message }
}

async function createVersion() {
  resetMessages()
  try {
    await api.createPromptVersion(activeTemplateId.value, {
      system_template: versionForm.system_template, user_template: versionForm.user_template,
      variable_schema: parseJSON(versionForm.variable_schema, '变量 Schema'),
      default_variables: parseJSON(versionForm.default_variables, '默认变量'),
      model_defaults: parseJSON(versionForm.model_defaults, '模型参数'), change_note: versionForm.change_note,
      created_by: actor.value,
    })
    versionForm.change_note = ''; await load(); notice.value = '新版本已保存；旧版本没有被覆盖。'
  } catch (err) { error.value = err.message }
}

async function previewVersion(version = selectedVersion.value) {
  resetMessages()
  try { preview.value = await api.previewPromptVersion(version.prompt_version_id, parseJSON(previewVariables.value, '预览变量')) }
  catch (err) { error.value = err.message }
}

async function approve(version) {
  if (!window.confirm(`批准 v${version.version}？批准只代表可用，仍需单独晋升 production。`)) return
  try { await api.approvePromptVersion(version.prompt_version_id, actor.value); await load(); notice.value = `v${version.version} 已明确批准。` }
  catch (err) { error.value = err.message }
}

async function promote(version) {
  if (!window.confirm(`将 v${version.version} 设为 production current？当前生产版本会保留在历史中。`)) return
  try { await api.promotePromptVersion(version.prompt_version_id, actor.value); await load(); notice.value = `v${version.version} 已成为 production current。` }
  catch (err) { error.value = err.message }
}

async function createFixture() {
  resetMessages()
  try {
    await api.createPromptFixture({ category: category.value, fixture_key: fixtureForm.fixture_key,
      display_name: fixtureForm.display_name, variables: parseJSON(fixtureForm.variables, 'fixture 变量'),
      expected_output: parseJSON(fixtureForm.expected_output, '期望输出'), created_by: actor.value })
    Object.assign(fixtureForm, { fixture_key: '', display_name: '', variables: '{}', expected_output: '{}' })
    await load(); notice.value = '冻结 fixture 已创建。'
  } catch (err) { error.value = err.message }
}

async function createSuite() {
  resetMessages()
  try {
    await api.createPromptTestSuite({ category: category.value, display_name: suiteForm.display_name,
      fixture_ids: suiteForm.fixture_ids, metric_config: parseJSON(suiteForm.metric_config, '指标配置'), created_by: actor.value })
    Object.assign(suiteForm, { display_name: '', fixture_ids: [] }); await load(); notice.value = '测试集快照已冻结。'
  } catch (err) { error.value = err.message }
}

async function createExperiment() {
  resetMessages()
  try {
    const created = await api.createPromptExperiment({ category: category.value, display_name: experimentForm.display_name,
      prompt_test_suite_id: experimentForm.prompt_test_suite_id, blind_review: experimentForm.blind_review,
      variants: experimentForm.variants.map((item) => ({ ...item, parameters: parseJSON(item.parameters, '实验参数'), seed: Number(item.seed) })), created_by: actor.value })
    await load(); await openExperiment(created.prompt_experiment_id); notice.value = '比较矩阵已冻结，盲评界面只显示方案代号。'
  } catch (err) { error.value = err.message }
}

async function openExperiment(id) {
  try {
    ;[activeExperiment.value, blindExperiment.value] = await Promise.all([api.getPromptExperiment(id), api.getPromptExperiment(id, true)])
    resultForm.prompt_experiment_variant_id = activeExperiment.value.variants[0]?.prompt_experiment_variant_id || ''
    resultForm.prompt_fixture_id = activeSuiteFixtures.value[0]?.prompt_fixture_id || ''
    blindForm.blind_label = blindExperiment.value.variants[0]?.blind_label || ''
    blindForm.prompt_fixture_id = resultForm.prompt_fixture_id
  } catch (err) { error.value = err.message }
}

async function submitResult() {
  try {
    activeExperiment.value = await api.submitPromptExperimentResult(activeExperiment.value.prompt_experiment_id, {
      prompt_experiment_variant_id: resultForm.prompt_experiment_variant_id,
      prompt_fixture_id: resultForm.prompt_fixture_id, output: parseJSON(resultForm.output, '模型输出'),
      latency_ms: resultForm.latency_ms === '' ? null : Number(resultForm.latency_ms), estimated_cost: Number(resultForm.estimated_cost || 0), token_usage: {},
    })
    blindExperiment.value = await api.getPromptExperiment(activeExperiment.value.prompt_experiment_id, true)
    notice.value = '结果已记录，自动指标已由服务端计算。'
  } catch (err) { error.value = err.message }
}

async function submitBlindEvaluation() {
  try {
    blindExperiment.value = await api.submitPromptBlindEvaluation(activeExperiment.value.prompt_experiment_id, {
      ...blindForm, score: Number(blindForm.score), rubric_scores: parseJSON(blindForm.rubric_scores, '评分细项'),
    })
    activeExperiment.value = await api.getPromptExperiment(activeExperiment.value.prompt_experiment_id)
    notice.value = '盲评已保存；评分记录不暴露模型身份。'
  } catch (err) { error.value = err.message }
}

onMounted(load)
</script>

<template>
  <section class="prompt-lab view-stack">
    <header class="lab-hero">
      <div><span>PROMPT & MODEL LAB</span><h2>Prompt / 模型实验室</h2><p>不可变版本、冻结测试集、盲评与 production 审批在同一条审计链上。</p></div>
      <label>操作人<input v-model="actor" /></label>
    </header>

    <nav class="category-tabs"><button v-for="[key, label] in categories" :key="key" :class="{ active: key === category }" @click="category = key">{{ label }}</button></nav>
    <div v-if="notice" class="success-banner"><Check :size="16" />{{ notice }}</div>
    <div v-if="error" class="error-banner">{{ error }}</div>

    <div class="lab-grid">
      <aside class="panel template-list">
        <div class="panel-title"><div><small>{{ categoryName }}</small><h3>Prompt 清单</h3></div><GitBranch :size="20" /></div>
        <button v-for="item in templates" :key="item.prompt_template_id" :class="{ active: activeTemplateId === item.prompt_template_id }" @click="activeTemplateId = item.prompt_template_id">
          <span><b>{{ item.display_name }}</b><small>{{ item.prompt_key }}</small></span><i v-if="item.production_prompt_version_id">PROD</i>
        </button>
        <form class="compact-form" @submit.prevent="createTemplate"><h4>新增 Prompt</h4><input v-model="templateForm.display_name" required placeholder="显示名称" /><input v-model="templateForm.prompt_key" required pattern="[a-z0-9_.-]+" placeholder="prompt.key" /><input v-model="templateForm.description" placeholder="用途说明" /><button class="button button-secondary"><Plus :size="14" />创建</button></form>
      </aside>

      <main class="panel version-editor">
        <div v-if="!activeTemplate" class="compact-empty">先创建或选择 Prompt。</div>
        <template v-else>
          <div class="panel-title"><div><small>IMMUTABLE VERSIONS</small><h3>{{ activeTemplate.display_name }}</h3></div><ShieldCheck :size="21" /></div>
          <div class="version-strip">
            <article v-for="version in activeTemplate.versions" :key="version.prompt_version_id" :class="{ production: version.is_production }">
              <header><b>v{{ version.version }}</b><span>{{ version.status }}</span><i v-if="version.is_production">production current</i></header>
              <p>{{ version.change_note }}</p><small>{{ version.content_hash.slice(0, 12) }}…</small>
              <footer><button @click="previewVersion(version)"><Eye :size="13" />预览</button><button v-if="version.status === 'draft'" @click="approve(version)"><ClipboardCheck :size="13" />批准</button><button v-if="version.status === 'approved' && !version.is_production" @click="promote(version)"><Rocket :size="13" />晋升</button></footer>
            </article>
          </div>
          <form class="version-form" @submit.prevent="createVersion">
            <label><span>System 模板</span><textarea v-model="versionForm.system_template" rows="5" placeholder="可使用 {{variable}}" /></label>
            <label><span>User 模板</span><textarea v-model="versionForm.user_template" rows="7" required placeholder="写作任务与结构约束" /></label>
            <div class="json-grid"><label><span>变量 JSON Schema</span><textarea v-model="versionForm.variable_schema" rows="8" /></label><label><span>默认变量</span><textarea v-model="versionForm.default_variables" rows="8" /></label><label><span>模型默认参数</span><textarea v-model="versionForm.model_defaults" rows="8" /></label></div>
            <label><span>版本说明</span><input v-model="versionForm.change_note" required placeholder="说明本版本为什么变化" /></label>
            <button class="button button-primary"><GitBranch :size="15" />另存为新版本</button>
          </form>
          <section class="preview-box"><div class="panel-title"><h4>最终输入预览与 Token 估算</h4><button @click="previewVersion()"><Play :size="14" />渲染</button></div><textarea v-model="previewVariables" rows="4" aria-label="预览变量 JSON" /><template v-if="preview"><div class="preview-meta"><b>约 {{ preview.token_estimate }} tokens</b><span>输入哈希 {{ preview.input_hash }}</span></div><pre>{{ preview.final_input }}</pre></template></section>
        </template>
      </main>
    </div>

    <div class="lab-grid test-grid">
      <article class="panel"><div class="panel-title"><div><small>FROZEN FIXTURES</small><h3>测试 fixture</h3></div><Beaker :size="20" /></div><form class="compact-form" @submit.prevent="createFixture"><input v-model="fixtureForm.display_name" required placeholder="fixture 名称" /><input v-model="fixtureForm.fixture_key" required placeholder="fixture.key" /><label>变量 JSON<textarea v-model="fixtureForm.variables" rows="5" /></label><label>期望输出 JSON<textarea v-model="fixtureForm.expected_output" rows="5" /></label><button class="button button-secondary">创建冻结版本</button></form><ul class="fixture-list"><li v-for="item in fixtures" :key="item.prompt_fixture_id"><span><b>{{ item.display_name }}</b><small>{{ item.fixture_key }} · v{{ item.version }}</small></span><i>🔒</i></li></ul></article>
      <article class="panel"><div class="panel-title"><div><small>TEST SUITE</small><h3>同一测试集</h3></div><FlaskConical :size="20" /></div><form class="compact-form" @submit.prevent="createSuite"><input v-model="suiteForm.display_name" required placeholder="测试集名称" /><div class="check-list"><label v-for="item in fixtures" :key="item.prompt_fixture_id"><input v-model="suiteForm.fixture_ids" type="checkbox" :value="item.prompt_fixture_id" />{{ item.display_name }} v{{ item.version }}</label></div><textarea v-model="suiteForm.metric_config" rows="4" /><button class="button button-secondary" :disabled="!suiteForm.fixture_ids.length">冻结测试集</button></form><ul class="fixture-list"><li v-for="item in suites" :key="item.prompt_test_suite_id"><span><b>{{ item.display_name }}</b><small>v{{ item.version }} · {{ item.fixture_ids.length }} fixtures</small></span><i>🔒</i></li></ul></article>
    </div>

    <article class="panel experiment-panel">
      <div class="panel-title"><div><small>MULTI-PROMPT / MULTI-MODEL</small><h3>比较实验</h3></div><FlaskConical :size="21" /></div>
      <form class="experiment-form" @submit.prevent="createExperiment"><div class="experiment-head"><input v-model="experimentForm.display_name" required placeholder="实验名称" /><select v-model="experimentForm.prompt_test_suite_id" required><option value="">选择冻结测试集</option><option v-for="item in suites" :key="item.prompt_test_suite_id" :value="item.prompt_test_suite_id">{{ item.display_name }} v{{ item.version }}</option></select><label><input v-model="experimentForm.blind_review" type="checkbox" />人工盲评</label></div><div class="variant-row" v-for="(variant, index) in experimentForm.variants" :key="index"><b>方案 {{ String.fromCharCode(65 + index) }}</b><select v-model="variant.prompt_version_id" required><option value="">Prompt 版本</option><option v-for="version in promptVersions" :key="version.prompt_version_id" :value="version.prompt_version_id">{{ version.template_name }} · v{{ version.version }} · {{ version.status }}</option></select><input v-model="variant.provider" required placeholder="Provider" /><input v-model="variant.model" required placeholder="模型" /><input v-model="variant.parameters" required placeholder="参数 JSON" /><input v-model.number="variant.seed" type="number" /></div><footer><button type="button" @click="experimentForm.variants.push(newVariant())"><Plus :size="14" />增加方案</button><button class="button button-primary" :disabled="!suites.length">建立比较矩阵</button></footer></form>
      <div class="experiment-history"><button v-for="item in experiments" :key="item.prompt_experiment_id" @click="openExperiment(item.prompt_experiment_id)"><b>{{ item.display_name }}</b><span>{{ item.status }} · {{ item.variants.length }} 方案</span></button></div>
    </article>

    <article v-if="activeExperiment" class="panel result-panel">
      <div class="panel-title"><div><small>AUTO METRICS + HUMAN BLIND REVIEW</small><h3>{{ activeExperiment.display_name }}</h3></div><span>{{ activeExperiment.suite_hash.slice(0, 12) }}…</span></div>
      <div class="result-entry"><form @submit.prevent="submitResult"><h4>记录模型测试结果</h4><select v-model="resultForm.prompt_experiment_variant_id"><option v-for="item in activeExperiment.variants" :key="item.prompt_experiment_variant_id" :value="item.prompt_experiment_variant_id">{{ item.blind_label }}</option></select><select v-model="resultForm.prompt_fixture_id"><option v-for="item in activeSuiteFixtures" :key="item.prompt_fixture_id" :value="item.prompt_fixture_id">{{ item.display_name }}</option></select><textarea v-model="resultForm.output" rows="6" placeholder="模型输出 JSON" /><input v-model.number="resultForm.latency_ms" type="number" placeholder="延迟 ms" /><button class="button button-secondary">计算并保存自动指标</button></form><form @submit.prevent="submitBlindEvaluation"><h4>人工盲评</h4><small>评分区仅使用匿名接口返回的方案代号与输出。</small><select v-model="blindForm.blind_label"><option v-for="item in blindExperiment?.variants" :key="item.blind_label" :value="item.blind_label">{{ item.blind_label }}</option></select><select v-model="blindForm.prompt_fixture_id"><option v-for="item in activeSuiteFixtures" :key="item.prompt_fixture_id" :value="item.prompt_fixture_id">{{ item.display_name }}</option></select><input v-model="blindForm.reviewer" required placeholder="评审人" /><input v-model.number="blindForm.score" type="number" min="0" max="100" /><textarea v-model="blindForm.comment" rows="3" placeholder="评语" /><button class="button button-secondary">提交盲评</button></form></div>
      <div class="matrix-wrap"><table><thead><tr><th>fixture</th><th>盲评方案</th><th>输出</th><th>自动指标</th><th>人工分</th></tr></thead><tbody><tr v-for="item in blindExperiment?.results" :key="`${item.prompt_fixture_id}:${item.blind_label}`"><td>{{ activeSuiteFixtures.find(f => f.prompt_fixture_id === item.prompt_fixture_id)?.display_name }}</td><td><b>{{ item.blind_label }}</b></td><td><pre>{{ JSON.stringify(item.output, null, 2) }}</pre></td><td><span v-for="(value, key) in item.automatic_metrics" :key="key">{{ key }}: {{ typeof value === 'number' ? value.toFixed(2) : value }}</span></td><td>{{ blindExperiment.evaluations.filter(e => e.prompt_fixture_id === item.prompt_fixture_id && e.blind_label === item.blind_label).map(e => e.score).join(' / ') || '待评' }}</td></tr></tbody></table></div>
    </article>
  </section>
</template>

<style scoped>
.prompt-lab{gap:18px}.lab-hero,.panel-title,.experiment-head,.experiment-form footer{display:flex;align-items:center;justify-content:space-between;gap:12px}.lab-hero{padding:18px 22px;border-radius:14px;background:linear-gradient(120deg,#17243e,#263f6b);color:#fff}.lab-hero span,.panel-title small{letter-spacing:.12em;font-size:11px}.lab-hero p{opacity:.75}.lab-hero label{display:grid;gap:5px}.lab-hero input{padding:8px;border-radius:7px;border:1px solid #ffffff44;background:#ffffff12;color:#fff}.category-tabs{display:flex;gap:7px;overflow:auto}.category-tabs button{white-space:nowrap;border:1px solid #d7deea;background:#fff;border-radius:999px;padding:8px 14px}.category-tabs button.active{background:#2f5fb9;color:#fff;border-color:#2f5fb9}.lab-grid{display:grid;grid-template-columns:280px 1fr;gap:16px}.test-grid{grid-template-columns:1fr 1fr}.panel{background:#fff;border:1px solid #dce2eb;border-radius:13px;padding:18px}.template-list>button{width:100%;display:flex;justify-content:space-between;text-align:left;border:0;background:#f6f8fb;padding:10px;margin:7px 0;border-radius:8px}.template-list>button.active{outline:2px solid #4772c7}.template-list button span,.fixture-list span{display:grid}.template-list i,.version-strip i{font-size:10px;color:#23653a}.compact-form,.version-form{display:grid;gap:9px;margin-top:16px}.compact-form input,.compact-form textarea,.compact-form select,.version-form input,.version-form textarea,.preview-box textarea,.experiment-form input,.experiment-form select,.result-entry input,.result-entry select,.result-entry textarea{padding:9px;border:1px solid #cbd4df;border-radius:7px}.compact-form label,.version-form label{display:grid;gap:5px}.version-strip{display:flex;gap:9px;overflow:auto;padding:12px 2px}.version-strip article{min-width:190px;border:1px solid #dde3ec;border-radius:9px;padding:10px}.version-strip article.production{border-color:#3d9b62;background:#f1fbf5}.version-strip header,.version-strip footer{display:flex;gap:7px;align-items:center;flex-wrap:wrap}.version-strip header span{font-size:11px;background:#eef2f7;padding:3px 6px;border-radius:9px}.version-strip footer button{border:0;background:#edf2fa;padding:5px;border-radius:5px;display:flex;gap:3px}.json-grid{display:grid;grid-template-columns:2fr 1fr 1fr;gap:10px}.preview-box{margin-top:16px;border-top:1px solid #e2e7ee;padding-top:14px}.preview-meta{display:flex;justify-content:space-between;gap:10px;margin:9px 0;font-size:12px;color:#526176}.preview-box pre,.matrix-wrap pre{white-space:pre-wrap;max-height:280px;overflow:auto;background:#172033;color:#edf4ff;padding:12px;border-radius:8px}.fixture-list{list-style:none;padding:0}.fixture-list li{display:flex;justify-content:space-between;padding:9px 0;border-top:1px solid #edf0f4}.check-list{display:grid;grid-template-columns:repeat(2,1fr);gap:6px}.experiment-form{display:grid;gap:9px}.variant-row{display:grid;grid-template-columns:80px 1.4fr .8fr 1fr 1.2fr 90px;gap:7px;align-items:center;background:#f6f8fb;padding:9px;border-radius:8px}.experiment-history{display:flex;gap:8px;overflow:auto;margin-top:14px}.experiment-history button{display:grid;text-align:left;min-width:190px;border:1px solid #dbe2ec;background:#fff;padding:9px;border-radius:8px}.result-entry{display:grid;grid-template-columns:1fr 1fr;gap:14px}.result-entry form{display:grid;gap:7px;background:#f7f9fc;padding:12px;border-radius:9px}.matrix-wrap{overflow:auto;margin-top:15px}.matrix-wrap table{width:100%;border-collapse:collapse}.matrix-wrap th,.matrix-wrap td{border-bottom:1px solid #e1e6ee;padding:9px;text-align:left;vertical-align:top}.matrix-wrap td span{display:block;font-size:12px}.matrix-wrap pre{max-width:360px;max-height:130px;margin:0}@media(max-width:900px){.lab-grid,.test-grid,.result-entry{grid-template-columns:1fr}.json-grid{grid-template-columns:1fr}.variant-row{grid-template-columns:1fr 1fr}.lab-hero{align-items:flex-start;display:grid}}
</style>
