<script setup>
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'
import {
  buildCandidateRequest, buildCompositionParts, componentLabels, filterCandidates,
  resolveTargetId, scoreLabels, targetComponents, targetLabels, validationRuleLabels,
} from '../services/candidateWorkbench'

const route = useRoute()
const projectId = computed(() => route.params.projectId)
const targets = ref({ project_id: '', arcs: [], episodes: [] })
const sets = ref([])
const activeSet = ref(null)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const selectionResult = ref(null)
const videoRefs = ref([])
const filters = reactive({ minimumScore: 0, favoriteOnly: false, showEliminated: false })
const composition = reactive({})
const timeComment = reactive({ candidate_id: '', timecode_ms: 0, comment_text: '' })
const form = reactive({
  target_type: 'episode', story_arc_id: '', episode_id: '', scene_id: '', shot_id: '',
  component_types: [...targetComponents.episode], candidate_count: 3,
  difference_directions: '强钩子\n紧凑节奏\n低成本可拍',
  must_preserve: '核心因果\n人物目标', allowed_changes: '对白\n场景顺序',
  random_seed: 42, temperature: 0, base_duration_seconds: 90,
  generator_provider: 'deterministic_mock', generator_model: 'deterministic-generator-v2',
  reviewer_provider: 'deterministic_mock', reviewer_model: 'deterministic-reviewer-v2', blind_review: true,
})

const candidates = computed(() => filterCandidates(activeSet.value?.candidates, filters))
const selectedEpisode = computed(() => targets.value.episodes.find((item) => item.episode_id === form.episode_id))
const scenes = computed(() => selectedEpisode.value?.scenes || [])
const selectedScene = computed(() => scenes.value.find((item) => item.scene_id === form.scene_id))
const shots = computed(() => selectedScene.value?.shots || [])
const needsScene = computed(() => ['scene', 'storyboard', 'image', 'video'].includes(form.target_type))
const needsShot = computed(() => ['storyboard', 'image', 'video'].includes(form.target_type))
const isImage = computed(() => activeSet.value?.target_type === 'image')
const isVideo = computed(() => activeSet.value?.target_type === 'video')
const generatorOptions = computed(() => {
  if (form.target_type === 'image') return [{ value: 'deterministic_mock', label: 'Deterministic Mock' }, { value: 'image_http', label: '真实图片 Provider' }]
  if (form.target_type === 'video') return [{ value: 'deterministic_mock', label: 'Deterministic Mock' }, { value: 'video_http', label: '真实视频 Provider' }]
  return [{ value: 'deterministic_mock', label: 'Deterministic Mock' }, { value: 'text_http', label: '真实文本 Provider' }]
})
const targetReady = computed(() => Boolean(resolveTargetId(form)))
const estimatedCost = computed(() => activeSet.value ? `${Number(activeSet.value.estimated_cost || 0).toFixed(4)} ${activeSet.value.currency}` : '—')

watch(() => form.target_type, (value) => {
  form.component_types = [...(targetComponents[value] || [])]
  form.generator_provider = 'deterministic_mock'
  form.generator_model = 'deterministic-generator-v2'
})

watch(() => form.episode_id, () => {
  form.scene_id = scenes.value[0]?.scene_id || ''
})
watch(() => form.scene_id, () => {
  form.shot_id = shots.value[0]?.shot_id || ''
})
watch(() => form.generator_provider, (value) => {
  form.generator_model = value === 'deterministic_mock' ? 'deterministic-generator-v2' : ''
})
watch(() => form.reviewer_provider, (value) => {
  form.reviewer_model = value === 'deterministic_mock' ? 'deterministic-reviewer-v2' : ''
})

function initializeTargets() {
  form.story_arc_id ||= targets.value.arcs?.[0]?.story_arc_revision_id || ''
  form.episode_id ||= targets.value.episodes?.[0]?.episode_id || ''
  form.scene_id ||= selectedEpisode.value?.scenes?.[0]?.scene_id || ''
  form.shot_id ||= selectedScene.value?.shots?.[0]?.shot_id || ''
}

function resetComposition() {
  Object.keys(composition).forEach((key) => delete composition[key])
  ;(activeSet.value?.component_types || []).forEach((key) => { composition[key] = '' })
}

async function load(selectId = '') {
  error.value = ''
  try {
    const [targetResponse, setResponse] = await Promise.all([
      narrativeApi.listCandidateTargets(projectId.value), narrativeApi.listCandidateSets(projectId.value),
    ])
    targets.value = targetResponse.data || { project_id: projectId.value, arcs: [], episodes: [] }
    initializeTargets()
    sets.value = setResponse.data || []
    const id = selectId || activeSet.value?.candidate_set_id || sets.value[0]?.candidate_set_id
    activeSet.value = id ? (await narrativeApi.getCandidateSet(projectId.value, id)).data : null
    resetComposition()
  } catch (exception) {
    error.value = exception.message
  }
}

async function generate() {
  if (!targetReady.value) {
    error.value = '请先通过项目→集→场→镜选择完整目标。'
    return
  }
  busy.value = true
  error.value = ''
  try {
    const response = await narrativeApi.generateCandidateSet(
      projectId.value, buildCandidateRequest(form), createIdempotencyKey('candidate-generate'),
    )
    await load(response.data.candidate_set_id)
    notice.value = `已生成并独立评审 ${response.data.candidate_count} 个候选；冻结输入 ${response.data.frozen_input_hash?.slice(0, 12) || '已记录'}。`
  } catch (exception) {
    error.value = exception.message
  } finally {
    busy.value = false
  }
}

async function changeSet(event) {
  activeSet.value = (await narrativeApi.getCandidateSet(projectId.value, event.target.value)).data
  resetComposition()
}

async function decide(candidate, decision) {
  const label = { favorite: '收藏', unfavorite: '取消收藏', eliminate: '淘汰', restore: '恢复' }[decision]
  if (!window.confirm(`确认${label} ${candidate.label}？候选正文不会被修改。`)) return
  busy.value = true
  try {
    await narrativeApi.recordCandidateDecision(candidate.candidate_id, { decision, reason: `CMS ${label}`, decided_by: 'cms-user' }, createIdempotencyKey('candidate-decision'))
    await load(activeSet.value.candidate_set_id)
  } catch (exception) {
    error.value = exception.message
  } finally {
    busy.value = false
  }
}

async function selectCandidate(candidate) {
  if (!window.confirm(`确认选择 ${candidate.label}？只有这个确认结果会成为下游 effective input。`)) return
  busy.value = true
  try {
    const response = await narrativeApi.selectCandidate(projectId.value, activeSet.value.candidate_set_id, {
      candidate_id: candidate.candidate_id, confirmed: true, confirmed_by: 'cms-user',
    }, createIdempotencyKey('candidate-select'))
    selectionResult.value = response.data
    notice.value = `${candidate.label} 已成为下游 current artifact；其他候选仍不可进入下游。`
  } catch (exception) {
    error.value = exception.message
  } finally {
    busy.value = false
  }
}

async function compose() {
  const parts = buildCompositionParts(activeSet.value.component_types, composition)
  if (parts.length < 2 || new Set(parts.map((item) => item.candidate_id)).size < 2) {
    error.value = '组合至少需要两个组件，并且来自两个不同候选。'
    return
  }
  if (!window.confirm(`确认组合 ${parts.length} 个组件？系统将重新执行全部五项硬规则。`)) return
  busy.value = true
  try {
    const response = await narrativeApi.composeCandidates(projectId.value, activeSet.value.candidate_set_id, {
      parts, confirmed: true, confirmed_by: 'cms-user',
    }, createIdempotencyKey('candidate-compose'))
    selectionResult.value = response.data
    notice.value = '组合已通过因果、时长、人物状态、伏笔与连续性硬规则，并成为下游 effective input。'
  } catch (exception) {
    error.value = exception.message
  } finally {
    busy.value = false
  }
}

function syncVideos(event) {
  for (const video of videoRefs.value) {
    if (video && video !== event.target && Math.abs(video.currentTime - event.target.currentTime) > 0.15) video.currentTime = event.target.currentTime
  }
}

async function playTogether() {
  await nextTick()
  await Promise.allSettled(videoRefs.value.filter(Boolean).map((video) => video.play()))
}

async function addTimeComment() {
  if (!timeComment.candidate_id || !timeComment.comment_text.trim()) return
  await narrativeApi.addCandidateTimecodeComment(timeComment.candidate_id, {
    timecode_ms: Number(timeComment.timecode_ms), comment_text: timeComment.comment_text.trim(), author: 'cms-user',
  }, createIdempotencyKey('timecode-comment'))
  timeComment.comment_text = ''
  notice.value = '时间码评论已保存。'
}

onMounted(load)
</script>

<template>
  <section class="candidate-page">
    <header class="page-head">
      <div><RouterLink :to="`/projects/${projectId}`">← 返回项目</RouterLink><h2>候选工作台</h2><p>可插拔真实生成、独立证据化评审与显式下游确认</p></div>
      <button class="button button-secondary" :disabled="busy" @click="load()">刷新</button>
    </header>
    <p v-if="error" class="error-banner">{{ error }}</p><p v-if="notice" class="notice">{{ notice }}</p>

    <form class="panel generation-form" @submit.prevent="generate">
      <div class="panel-title"><div><h3>生成候选</h3><p>生成失败会原样报错，不会自动回退到 Mock。</p></div><button class="button button-primary" :disabled="busy || !targetReady">{{ busy ? '处理中…' : '生成并独立评审' }}</button></div>

      <div class="target-path">
        <label><span>项目</span><select disabled><option>{{ targets.project_id || projectId }}</option></select></label>
        <label><span>候选类型</span><select v-model="form.target_type"><option v-for="(label, key) in targetLabels" :key="key" :value="key">{{ label }}</option></select></label>
        <label v-if="form.target_type === 'story_arc'"><span>故事弧</span><select v-model="form.story_arc_id" required><option value="">请选择故事弧</option><option v-for="arc in targets.arcs" :key="arc.story_arc_revision_id" :value="arc.story_arc_revision_id">{{ arc.title }}</option></select></label>
        <template v-else>
          <label><span>集</span><select v-model="form.episode_id" required><option value="">请选择集</option><option v-for="episode in targets.episodes" :key="episode.episode_id" :value="episode.episode_id">第 {{ episode.episode_number }} 集 · {{ episode.title }}</option></select></label>
          <label v-if="needsScene"><span>场</span><select v-model="form.scene_id" required><option value="">请选择场</option><option v-for="scene in scenes" :key="scene.scene_id" :value="scene.scene_id">场 {{ scene.scene_number }} · {{ scene.label || scene.scene_id }}</option></select></label>
          <label v-if="needsShot"><span>镜</span><select v-model="form.shot_id" required><option value="">请选择镜头</option><option v-for="shot in shots" :key="shot.shot_id" :value="shot.shot_id">镜 {{ shot.shot_order }} · {{ shot.description || shot.shot_id }}</option></select></label>
        </template>
      </div>

      <div class="provider-grid">
        <label><span>生成 Provider</span><select v-model="form.generator_provider"><option v-for="option in generatorOptions" :key="option.value" :value="option.value">{{ option.label }}</option></select></label>
        <label><span>生成模型</span><input v-model="form.generator_model" required :readonly="form.generator_provider === 'deterministic_mock'" placeholder="填写真实模型 ID" /></label>
        <label><span>评审 Provider</span><select v-model="form.reviewer_provider"><option value="deterministic_mock">Deterministic Reviewer</option><option value="reviewer_http">真实独立 Reviewer</option></select></label>
        <label><span>评分模型</span><input v-model="form.reviewer_model" required :readonly="form.reviewer_provider === 'deterministic_mock'" placeholder="必须与生成模型分离" /></label>
        <label class="blind-toggle"><input v-model="form.blind_review" type="checkbox" /><span>盲评：比较时隐藏模型与供应商</span></label>
      </div>

      <div class="form-grid">
        <label><span>候选数量</span><input v-model.number="form.candidate_count" type="number" min="2" max="12" required /></label>
        <label><span>随机种子</span><input v-model.number="form.random_seed" type="number" /></label>
        <label><span>基准时长（秒）</span><input v-model.number="form.base_duration_seconds" type="number" min="1" /></label>
        <label><span>温度</span><input v-model.number="form.temperature" type="number" min="0" max="2" step="0.1" /></label>
      </div>
      <div class="component-options"><label v-for="item in targetComponents[form.target_type]" :key="item"><input v-model="form.component_types" type="checkbox" :value="item" />{{ componentLabels[item] }}</label></div>
      <div class="text-grid">
        <label><span>差异方向（逐行）</span><textarea v-model="form.difference_directions" required /></label>
        <label><span>必须保持</span><textarea v-model="form.must_preserve" /></label>
        <label><span>允许变化</span><textarea v-model="form.allowed_changes" /></label>
      </div>
    </form>

    <section v-if="sets.length" class="panel toolbar">
      <label>候选批次 <select :value="activeSet?.candidate_set_id" @change="changeSet"><option v-for="item in sets" :key="item.candidate_set_id" :value="item.candidate_set_id">{{ targetLabels[item.target_type] || item.target_type }} · {{ item.target_id }} · {{ item.candidate_count }} 个</option></select></label>
      <label>最低分 <input v-model.number="filters.minimumScore" type="number" min="0" max="100" /></label>
      <label><input v-model="filters.favoriteOnly" type="checkbox" />仅收藏</label>
      <label><input v-model="filters.showEliminated" type="checkbox" />显示淘汰</label>
      <strong>成本估算 {{ estimatedCost }}</strong>
    </section>

    <section v-if="activeSet" class="set-provenance panel">
      <span>冻结输入 <code>{{ activeSet.frozen_input_hash }}</code></span>
      <span>Resolver <code>{{ activeSet.frozen_resolution_id }}</code></span>
      <span v-if="activeSet.blind_review">盲评开启：供应商和模型已隐藏</span>
      <span v-else>{{ activeSet.generator_provider }} / {{ activeSet.generator_model }} → {{ activeSet.reviewer_provider }} / {{ activeSet.reviewer_model }}</span>
    </section>

    <section v-if="activeSet" class="comparison-grid">
      <article v-for="candidate in candidates" :key="candidate.candidate_id" class="candidate-card" :class="{ eliminated: candidate.is_eliminated }">
        <header><div><span>#{{ candidate.rank }} · {{ candidate.difference_direction }}</span><h3>{{ candidate.label }}</h3></div><strong>{{ candidate.score.total_score }}</strong></header>
        <div class="score-row">
          <span>忠实 {{ candidate.score.fidelity }}</span><span>因果 {{ candidate.score.causality }}</span><span>人物 {{ candidate.score.character_consistency }}</span>
          <span>钩子 {{ candidate.score.hook }}</span><span>节奏 {{ candidate.score.pacing }}</span><span>可拍 {{ candidate.score.filmability }}</span>
          <span>连续 {{ candidate.score.continuity }}</span><span>风险 {{ candidate.score.modification_risk }}</span>
        </div>
        <p class="duration">预计 {{ candidate.score.estimated_duration_seconds }} 秒 · seed {{ candidate.random_seed }}<template v-if="candidate.model"> · {{ candidate.provider }} / {{ candidate.model }}</template></p>
        <div v-if="isImage && candidate.content.media?.preview_url" class="image-preview"><img :src="candidate.content.media.preview_url" :alt="candidate.label" /></div>
        <video v-if="isVideo && candidate.content.media?.preview_url" :ref="(element) => { if (element && !videoRefs.includes(element)) videoRefs.push(element) }" controls :src="candidate.content.media.preview_url" @timeupdate="syncVideos" />
        <section v-for="component in candidate.content.components" :key="component.key" class="component"><b>{{ componentLabels[component.type] || component.title }}</b><p>{{ component.content }}</p></section>
        <details><summary>结构化 diff</summary><pre>{{ JSON.stringify(candidate.structured_diff, null, 2) }}</pre></details>
        <details class="evidence"><summary>九维评分证据与扣分位置</summary>
          <section v-for="dimension in candidate.score.dimensions" :key="dimension.dimension">
            <h4>{{ scoreLabels[dimension.dimension] || dimension.dimension }} · {{ dimension.dimension === 'estimated_duration' ? `${candidate.score.estimated_duration_seconds} 秒` : dimension.score }}</h4>
            <p v-for="(item, index) in dimension.evidence" :key="`e-${index}`"><b>证据</b> <code>{{ item.source_kind }}/{{ item.source_id }}{{ item.path }}</code> — {{ item.reason }}<q v-if="item.quote">{{ item.quote }}</q></p>
            <p v-for="(item, index) in dimension.deductions" :key="`d-${index}`"><b>扣 {{ item.penalty }}</b> {{ item.reason }} <code>{{ item.location.source_kind }}/{{ item.location.source_id }}{{ item.location.path }}</code></p>
          </section>
        </details>
        <div class="reasons"><p><b>推荐理由</b> {{ candidate.score.recommendation_reasons.join('；') }}</p><p><b>扣分摘要</b> {{ candidate.score.deduction_reasons.join('；') }}</p></div>
        <footer><button @click="decide(candidate, candidate.is_favorite ? 'unfavorite' : 'favorite')">{{ candidate.is_favorite ? '取消收藏' : '收藏' }}</button><button @click="decide(candidate, candidate.is_eliminated ? 'restore' : 'eliminate')">{{ candidate.is_eliminated ? '恢复' : '淘汰' }}</button><button class="button button-primary" @click="selectCandidate(candidate)">确认进入下游</button></footer>
      </article>
    </section>

    <section v-if="isVideo" class="panel"><div class="panel-title"><h3>视频同步播放与时间码评论</h3><button @click="playTogether">同步播放</button></div><form class="time-comment" @submit.prevent="addTimeComment"><select v-model="timeComment.candidate_id" required><option value="">选择候选</option><option v-for="item in candidates" :key="item.candidate_id" :value="item.candidate_id">{{ item.label }}</option></select><input v-model.number="timeComment.timecode_ms" type="number" min="0" placeholder="时间码 ms" /><input v-model="timeComment.comment_text" required placeholder="评论" /><button>保存</button></form></section>

    <section v-if="activeSet && activeSet.component_types.length > 1" class="panel composition">
      <div class="panel-title"><div><h3>跨候选组合</h3><p>组合后重新执行因果、时长、人物状态、伏笔与连续性硬规则。</p></div><button class="button button-primary" :disabled="busy" @click="compose">确认组合并校验</button></div>
      <div class="composition-grid"><label v-for="component in activeSet.component_types" :key="component"><span>{{ componentLabels[component] }}</span><select v-model="composition[component]"><option value="">不选择</option><option v-for="candidate in activeSet.candidates" :key="candidate.candidate_id" :value="candidate.candidate_id">{{ candidate.label }} · {{ candidate.score.total_score }}</option></select></label></div>
    </section>

    <section v-if="selectionResult" class="panel validation">
      <h3>下游 artifact：{{ selectionResult.artifact_id }}</h3><p>只有这个显式确认的版本进入 current binding；其他候选永远不会被 Resolver 返回给下游。</p>
      <div><span v-for="rule in validationRuleLabels(selectionResult.validation_summary)" :key="rule.rule" :class="{ pass: rule.passed }">{{ rule.label }} {{ rule.passed ? '通过' : '失败' }}</span></div>
    </section>
  </section>
</template>

<style scoped>
.candidate-page{display:grid;gap:18px}.page-head,.panel-title,.toolbar,.candidate-card header,.candidate-card footer{display:flex;justify-content:space-between;align-items:center;gap:12px}.panel,.candidate-card{background:var(--surface,#fff);border:1px solid var(--border,#dce2e8);border-radius:12px;padding:18px}.target-path,.provider-grid,.form-grid,.text-grid,.composition-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(210px,1fr));gap:12px;margin-top:14px}.target-path{padding:14px;border-radius:10px;background:#f5f7fb}.generation-form label,.composition label{display:grid;gap:6px}.generation-form input,.generation-form select,.generation-form textarea,.toolbar input,.toolbar select,.composition select,.time-comment input,.time-comment select{padding:9px;border:1px solid #ccd4dd;border-radius:7px}.text-grid textarea{min-height:95px}.blind-toggle{align-content:center;grid-template-columns:auto 1fr!important}.component-options,.score-row,.candidate-card footer,.validation div,.set-provenance{display:flex;flex-wrap:wrap;gap:8px;margin:12px 0}.component-options label,.score-row span,.validation span,.set-provenance span{background:#edf2f7;border-radius:999px;padding:5px 9px}.comparison-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(390px,1fr));gap:14px}.candidate-card header>strong{font-size:30px;color:#315cbb}.candidate-card.eliminated{opacity:.55}.component,.evidence section{border-top:1px solid #e7ebef;padding-top:10px}.component p,.duration,.reasons,.evidence p{color:#5d6875}.candidate-card pre{max-height:230px;overflow:auto;white-space:pre-wrap}.candidate-card video,.image-preview img{width:100%;border-radius:8px}.evidence code,.set-provenance code{overflow-wrap:anywhere;color:#415a96}.evidence q{display:block;margin-top:4px;color:#68758a}.time-comment{display:grid;grid-template-columns:180px 130px 1fr auto;gap:8px}.validation .pass{background:#daf2e2;color:#23653a}.notice{background:#e8f6ed;padding:12px;border-radius:8px}@media(max-width:720px){.page-head,.panel-title,.toolbar{align-items:stretch;display:grid}.time-comment{grid-template-columns:1fr}.comparison-grid{grid-template-columns:1fr}}
</style>
