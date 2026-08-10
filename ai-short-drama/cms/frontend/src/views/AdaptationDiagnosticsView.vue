<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../services/api'
import { narrativeApi, createIdempotencyKey } from '../services/narrativeApi'
import { buildSpecFromDiagnostic, curvePoints, severityLabel } from '../services/adaptationAnalysis'

const route = useRoute()
const projectId = computed(() => route.params.projectId)
const diagnostic = ref(null)
const pacing = ref(null)
const quality = ref(null)
const beats = ref([])
const busy = ref(false)
const error = ref('')
const notice = ref('')
const specDraft = ref(null)
const showSpecConfirm = ref(false)
const specPlan = ref(null)
const pacingPlan = ref(null)

const curve = (field) => curvePoints(pacing.value?.episodes || [], field)

async function load() {
  error.value = ''
  const settled = await Promise.allSettled([
    narrativeApi.getLatestDiagnostic(projectId.value),
    narrativeApi.getLatestPacing(projectId.value),
    narrativeApi.getLatestQualityScore(projectId.value),
  ])
  diagnostic.value = settled[0].status === 'fulfilled' ? settled[0].value.data : null
  pacing.value = settled[1].status === 'fulfilled' ? settled[1].value.data : null
  quality.value = settled[2].status === 'fulfilled' ? settled[2].value.data : null
  beats.value = structuredClone(pacing.value?.beats || [])
  if (!diagnostic.value && settled.every((item) => item.status === 'rejected')) {
    error.value = '尚无诊断报告，请先运行确定性分析。'
  }
}

async function runAnalysis() {
  busy.value = true
  error.value = ''
  try {
    await narrativeApi.runAdaptationAnalysis(projectId.value, createIdempotencyKey('diagnosis'))
    await load()
    notice.value = '诊断、节奏计划和质量评分已生成。'
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

async function saveBeats() {
  busy.value = true
  try {
    pacingPlan.value = await api.createChangePlan(projectId.value, {
      instruction: '更新节奏 beats；先预览精确影响，确认后创建不可变 successor 版本',
      target: { entity_type: 'pacing', entity_id: pacing.value.pacing_plan_id, version: pacing.value.version_number },
      changes: [{ operation: 'replace', field: 'beats', value: beats.value }],
    })
    notice.value = '节奏差异、影响和重建范围预览已生成；正式内容尚未修改。'
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

async function applyPacingPlan() {
  if (!pacingPlan.value) return
  busy.value = true
  try {
    const id = pacingPlan.value.change_plan_id
    await api.confirmChangePlan(projectId.value, id, { actor: 'cms-user' })
    await api.executeChangePlan(projectId.value, id)
    pacingPlan.value = null
    await load()
    notice.value = '节奏 successor 版本已原子切换；精确 stale 与 pending rebuild 已记录。'
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

async function rescore() {
  busy.value = true
  try {
    await narrativeApi.rescoreQuality(projectId.value, 'season', {}, createIdempotencyKey('quality-score'))
    await load()
    notice.value = '已完成局部可解释评分。'
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

function prepareSpec() {
  specDraft.value = buildSpecFromDiagnostic(diagnostic.value, pacing.value)
  specPlan.value = null
  showSpecConfirm.value = true
}

async function confirmSpec() {
  busy.value = true
  try {
    if (!specPlan.value) {
      const specs = (await narrativeApi.listAdaptationSpecs(projectId.value)).data
      const current = specs.find((item) => item.status === 'active') || specs[0]
      if (!current) throw new Error('没有可作为修改基线的 Adaptation Spec。')
      const spec = specDraft.value
      specPlan.value = await api.createChangePlan(projectId.value, {
        instruction: '应用诊断建议到 Adaptation Spec；先预览差异和影响，确认后创建不可变 successor 版本',
        target: { entity_type: 'adaptation_spec', entity_id: current.adaptation_spec_version_id, version: current.version_number },
        must_preserve: ['source_version_id', 'ir_revision_id', 'source_binding_id'],
        changes: [
          { operation: 'replace', field: 'platform', value: spec.platform },
          { operation: 'replace', field: 'audience_profile', value: spec.audience_profile },
          { operation: 'replace', field: 'target_episode_count', value: spec.target_episode_count },
          { operation: 'replace', field: 'episode_duration_seconds', value: spec.episode_duration_seconds },
          { operation: 'replace', field: 'scope_mode', value: spec.scope.mode },
          { operation: 'replace', field: 'chapter_ids', value: spec.scope.chapter_ids },
          { operation: 'replace', field: 'story_arc_revision_ids', value: spec.scope.story_arc_revision_ids },
          { operation: 'replace', field: 'rules', value: spec.rules },
        ],
      })
      notice.value = 'Spec 差异、影响与重建范围预览已生成；请再次确认后应用。'
      return
    }
    const id = specPlan.value.change_plan_id
    await api.confirmChangePlan(projectId.value, id, { actor: 'cms-user' })
    await api.executeChangePlan(projectId.value, id)
    showSpecConfirm.value = false
    specPlan.value = null
    notice.value = 'Adaptation Spec successor 版本已原子切换。'
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

onMounted(load)
</script>

<template>
  <section class="analysis-page">
    <header class="page-head">
      <div><RouterLink :to="`/projects/${projectId}`">← 返回项目</RouterLink><h2>改编诊断</h2><p>确定性 Mock · 版本化证据 · 局部失效</p></div>
      <div class="actions"><button class="button button-secondary" :disabled="busy" @click="load">刷新</button><button class="button button-primary" :disabled="busy" @click="runAnalysis">{{ busy ? '处理中…' : '重新分析' }}</button></div>
    </header>
    <p v-if="error" class="error-banner">{{ error }}</p><p v-if="notice" class="notice">{{ notice }}</p>

    <template v-if="diagnostic">
      <div class="cards">
        <article><h3>原著核心卖点</h3><ul><li v-for="item in diagnostic.core_selling_points" :key="item">{{ item }}</li></ul></article>
        <article><h3>目标受众与情绪价值</h3><p>{{ diagnostic.target_audience }}</p><div class="chips"><span v-for="item in diagnostic.emotional_value" :key="item">{{ item }}</span></div></article>
        <article><h3>主角曲线</h3><dl><template v-for="(value, key) in diagnostic.protagonist_curve" :key="key"><dt>{{ key }}</dt><dd>{{ value }}</dd></template></dl></article>
        <article><h3>开篇与结尾钩子</h3><dl><template v-for="(value, key) in diagnostic.hook_recommendations" :key="key"><dt>{{ key }}</dt><dd>{{ value }}</dd></template></dl></article>
      </div>
      <section class="panel"><div class="panel-title"><h3>改编动作与不可直接影视化内容</h3><button class="button button-secondary" @click="prepareSpec">生成 Spec 草稿</button></div>
        <div class="split"><ul><li v-for="(item, i) in diagnostic.transformation_recommendations" :key="i"><strong>{{ item.action }}</strong> {{ item.reason || item.description }}</li></ul><ul><li v-for="(item, i) in diagnostic.unfilmable_passages" :key="i">{{ item.type }}：{{ item.excerpt }}（{{ item.suggestion }}）</li></ul></div>
      </section>
    </template>

    <section v-if="pacing" class="panel">
      <div class="panel-title"><div><h3>整季 / 单集节奏曲线</h3><p>冲突、情绪、信息揭示与钩子均归一化为 0–1。</p></div><button class="button button-primary" :disabled="busy" @click="saveBeats">保存为新节奏版本</button></div>
      <div class="chart">
        <svg viewBox="0 0 600 150" preserveAspectRatio="none"><polyline :points="curve('conflict_intensity')" class="conflict"/><polyline :points="curve('emotional_intensity')" class="emotion"/><polyline :points="curve('information_reveal')" class="reveal"/><polyline :points="curve('hook_strength')" class="hook"/></svg>
        <div class="legend"><span>冲突</span><span>情绪</span><span>信息</span><span>钩子</span></div>
      </div>
      <div class="issue-list"><p v-for="issue in pacing.issues" :key="issue.pacing_issue_id"><b :class="`sev-${issue.severity}`">{{ severityLabel(issue.severity) }}</b> 第 {{ issue.episode_number || '—' }} 集：{{ issue.message }} <em>{{ issue.suggestion }}</em></p></div>
      <div class="beat-table"><div class="beat-row head"><span>节拍</span><span>集</span><span>序</span><span>秒</span><span>强度</span></div><div v-for="beat in beats" :key="beat.beat_key" class="beat-row"><span><b>{{ beat.title }}</b><small>{{ beat.summary }}</small></span><input v-model.number="beat.episode_number" type="number" min="1"><input v-model.number="beat.beat_ordinal" type="number" min="1"><input v-model.number="beat.estimated_duration_seconds" type="number" min="1"><span>{{ beat.conflict_intensity }} / {{ beat.emotional_intensity }} / {{ beat.hook_strength }}</span></div></div>
    </section>

    <div v-if="pacingPlan" class="modal"><div><h3>确认节奏修改计划？</h3><p>以下为差异、影响和 pending rebuild 预览；确认前正式内容保持不变。</p><pre>{{ JSON.stringify({ changes: pacingPlan.plan?.expected_changes, impacts: pacingPlan.impacts, rebuild_tasks: pacingPlan.plan?.rebuild_tasks, risks: pacingPlan.plan?.risks }, null, 2) }}</pre><div class="actions"><button @click="pacingPlan=null">取消</button><button class="button button-primary" :disabled="busy" @click="applyPacingPlan">确认并应用</button></div></div></div>

    <section v-if="quality" class="panel">
      <div class="panel-title"><div><h3>可解释质量评分 · {{ quality.total_score }}</h3><p>每项均展示证据、位置、严重度与修改建议。</p></div><button class="button button-secondary" :disabled="busy" @click="rescore">局部重新评分</button></div>
      <div class="score-grid"><article v-for="dimension in quality.dimensions" :key="dimension.dimension"><header><h4>{{ dimension.dimension }}</h4><strong>{{ dimension.score }}</strong></header><p v-for="issue in dimension.issues" :key="issue.quality_issue_id"><b :class="`sev-${issue.severity}`">{{ severityLabel(issue.severity) }}</b> {{ issue.message }}<small>位置：{{ issue.location }} · 建议：{{ issue.suggestion }}</small></p><details><summary>查看证据</summary><pre>{{ dimension.evidence }}</pre></details></article></div>
    </section>

    <div v-if="showSpecConfirm" class="modal"><div><h3>确认创建 Adaptation Spec 新版本？</h3><p>{{ specPlan ? '以下为 change plan 差异与影响预览；再次确认才会原子切换 current。' : '诊断只生成草稿；先创建差异与影响预览，不会立即修改正式内容。' }}</p><pre>{{ JSON.stringify(specPlan ? { changes: specPlan.plan?.expected_changes, impacts: specPlan.impacts, rebuild_tasks: specPlan.plan?.rebuild_tasks, risks: specPlan.plan?.risks } : specDraft, null, 2) }}</pre><div class="actions"><button @click="showSpecConfirm=false;specPlan=null">取消</button><button class="button button-primary" :disabled="busy" @click="confirmSpec">{{ specPlan ? '确认并应用' : '生成修改预览' }}</button></div></div></div>
  </section>
</template>

<style scoped>
.analysis-page{display:grid;gap:20px}.page-head,.panel-title,.actions{display:flex;justify-content:space-between;align-items:center;gap:12px}.page-head h2{margin:8px 0 2px}.cards,.score-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:14px}.cards article,.panel,.score-grid article{background:var(--surface,#fff);border:1px solid var(--border,#dbe1e8);border-radius:12px;padding:18px}.chips{display:flex;flex-wrap:wrap;gap:7px}.chips span{background:#eef2ff;border-radius:999px;padding:4px 9px}.split{display:grid;grid-template-columns:1fr 1fr;gap:18px}.chart svg{width:100%;height:190px;background:repeating-linear-gradient(#fff,#fff 37px,#eef1f5 38px)}polyline{fill:none;stroke-width:4}.conflict{stroke:#e25555}.emotion{stroke:#9257d6}.reveal{stroke:#3293c8}.hook{stroke:#e79526}.legend{display:flex;gap:18px}.beat-table{overflow:auto}.beat-row{display:grid;grid-template-columns:minmax(280px,3fr) 60px 60px 80px 180px;gap:8px;align-items:center;border-top:1px solid #e6e9ee;padding:9px}.beat-row small,.score-grid small{display:block;color:#677383}.beat-row input{width:100%}.score-grid article header{display:flex;justify-content:space-between}.score-grid article header strong{font-size:26px}.score-grid pre{white-space:pre-wrap}.issue-list em{color:#677383}.sev-critical,.sev-high{color:#bd3030}.sev-medium{color:#b56a00}.sev-low{color:#397258}.notice{background:#eaf8ef;padding:12px;border-radius:8px}.modal{position:fixed;inset:0;background:#0008;display:grid;place-items:center;z-index:20}.modal>div{background:#fff;border-radius:12px;padding:24px;width:min(650px,90vw)}.modal pre{max-height:45vh;overflow:auto}@media(max-width:760px){.page-head,.panel-title,.split{display:grid;grid-template-columns:1fr}.beat-row{grid-template-columns:200px 55px 55px 70px 150px}}
</style>
