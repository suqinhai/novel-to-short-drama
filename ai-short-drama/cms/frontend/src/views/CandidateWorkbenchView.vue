<script setup>
import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'
import {
  buildCandidateRequest, buildCompositionParts, componentLabels,
  filterCandidates, targetComponents, validationRuleLabels,
} from '../services/candidateWorkbench'

const route = useRoute()
const projectId = computed(() => route.params.projectId)
const sets = ref([])
const activeSet = ref(null)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const selectionResult = ref(null)
const videoRefs = ref([])
const imageMix = ref(50)
const filters = reactive({ minimumScore: 0, favoriteOnly: false, showEliminated: false })
const composition = reactive({})
const timeComment = reactive({ candidate_id: '', timecode_ms: 0, comment_text: '' })
const form = reactive({
  target_type: 'episode', target_id: '', component_types: [...targetComponents.episode],
  candidate_count: 3, difference_directions: '强钩子\n紧凑节奏\n低成本可拍',
  must_preserve: '核心因果\n人物目标', allowed_changes: '对白\n场景顺序',
  random_seed: 42, temperature: 0, base_duration_seconds: 90,
})

const candidates = computed(() => filterCandidates(activeSet.value?.candidates, filters))
const isImage = computed(() => activeSet.value?.target_type === 'image')
const isVideo = computed(() => activeSet.value?.target_type === 'video')
const estimatedCost = computed(() => activeSet.value ? `${activeSet.value.estimated_cost.toFixed(4)} ${activeSet.value.currency}` : '—')

watch(() => form.target_type, (value) => { form.component_types = [...(targetComponents[value] || [])] })

function resetComposition() {
  Object.keys(composition).forEach((key) => delete composition[key])
  activeSet.value?.component_types?.forEach((key) => { composition[key] = '' })
}

async function load(selectId = '') {
  error.value = ''
  try {
    sets.value = (await narrativeApi.listCandidateSets(projectId.value)).data || []
    const id = selectId || activeSet.value?.candidate_set_id || sets.value[0]?.candidate_set_id
    activeSet.value = id ? (await narrativeApi.getCandidateSet(projectId.value, id)).data : null
    resetComposition()
  } catch (e) { error.value = e.message }
}

async function generate() {
  busy.value = true
  error.value = ''
  try {
    const response = await narrativeApi.generateCandidateSet(projectId.value, buildCandidateRequest(form), createIdempotencyKey('candidate-generate'))
    await load(response.data.candidate_set_id)
    notice.value = `已生成 ${response.data.candidate_count} 个不可变候选，并完成自动评分排序。`
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

async function changeSet(event) {
  activeSet.value = (await narrativeApi.getCandidateSet(projectId.value, event.target.value)).data
  resetComposition()
}

async function decide(candidate, decision) {
  const action = decision === 'eliminate' ? '淘汰' : decision === 'favorite' ? '收藏' : decision === 'restore' ? '恢复' : '取消收藏'
  if (!window.confirm(`确认${action} ${candidate.label}？候选正文不会被修改。`)) return
  busy.value = true
  try {
    await narrativeApi.recordCandidateDecision(candidate.candidate_id, { decision, reason: `CMS ${action}`, decided_by: 'cms-user' }, createIdempotencyKey('candidate-decision'))
    await load(activeSet.value.candidate_set_id)
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

async function selectCandidate(candidate) {
  if (!window.confirm(`确认选择 ${candidate.label} 成为 current？旧 current 会保留为历史版本。`)) return
  busy.value = true
  try {
    const response = await narrativeApi.selectCandidate(projectId.value, activeSet.value.candidate_set_id, {
      candidate_id: candidate.candidate_id, confirmed: true, confirmed_by: 'cms-user',
    }, createIdempotencyKey('candidate-select'))
    selectionResult.value = response.data
    notice.value = `${candidate.label} 已复制为新的 current artifact；候选本身仍保持不可变。`
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

async function compose() {
  const parts = buildCompositionParts(activeSet.value.component_types, composition)
  if (parts.length < 2) {
    error.value = '至少选择两个来自不同候选的组件。'
    return
  }
  if (!window.confirm(`确认组合 ${parts.length} 个组件？系统将重新运行全部五项硬规则。`)) return
  busy.value = true
  try {
    const response = await narrativeApi.composeCandidates(projectId.value, activeSet.value.candidate_set_id, {
      parts, confirmed: true, confirmed_by: 'cms-user',
    }, createIdempotencyKey('candidate-compose'))
    selectionResult.value = response.data
    notice.value = '组合已创建为新的 artifact，全部硬规则已重新校验。'
  } catch (e) { error.value = e.message } finally { busy.value = false }
}

function syncVideos(event) {
  for (const video of videoRefs.value) {
    if (video && video !== event.target && Math.abs(video.currentTime - event.target.currentTime) > .15) {
      video.currentTime = event.target.currentTime
    }
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
      <div><RouterLink :to="`/projects/${projectId}`">← 返回项目</RouterLink><h2>候选工作台</h2><p>生成、评分、并排对比、人工择优与跨候选组合</p></div>
      <button class="button button-secondary" :disabled="busy" @click="load()">刷新</button>
    </header>
    <p v-if="error" class="error-banner">{{ error }}</p><p v-if="notice" class="notice">{{ notice }}</p>

    <form class="panel generation-form" @submit.prevent="generate">
      <div class="panel-title"><div><h3>生成候选</h3><p>本阶段使用确定性 Mock，不产生商业结算。</p></div><button class="button button-primary" :disabled="busy">{{ busy ? '处理中…' : '生成并自动评分' }}</button></div>
      <div class="form-grid">
        <label><span>目标类型</span><select v-model="form.target_type"><option v-for="(_, key) in targetComponents" :key="key" :value="key">{{ key }}</option></select></label>
        <label><span>目标 ID</span><input v-model="form.target_id" required placeholder="episode_xxx / scene_xxx" /></label>
        <label><span>候选数量</span><input v-model.number="form.candidate_count" type="number" min="2" max="12" required /></label>
        <label><span>随机种子</span><input v-model.number="form.random_seed" type="number" /></label>
        <label><span>基准时长（秒）</span><input v-model.number="form.base_duration_seconds" type="number" min="1" /></label>
        <label><span>成本估算</span><input value="按目标 × 组件 × 候选数估算" disabled /></label>
      </div>
      <div class="component-options"><label v-for="item in targetComponents[form.target_type]" :key="item"><input v-model="form.component_types" type="checkbox" :value="item" />{{ componentLabels[item] }}</label></div>
      <div class="text-grid">
        <label><span>差异方向（逐行）</span><textarea v-model="form.difference_directions" required /></label>
        <label><span>必须保持</span><textarea v-model="form.must_preserve" /></label>
        <label><span>允许变化</span><textarea v-model="form.allowed_changes" /></label>
      </div>
    </form>

    <section v-if="sets.length" class="panel toolbar">
      <label>候选批次 <select :value="activeSet?.candidate_set_id" @change="changeSet"><option v-for="item in sets" :key="item.candidate_set_id" :value="item.candidate_set_id">{{ item.target_type }} · {{ item.target_id }} · {{ item.candidate_count }} 个</option></select></label>
      <label>最低分 <input v-model.number="filters.minimumScore" type="number" min="0" max="100" /></label>
      <label><input v-model="filters.favoriteOnly" type="checkbox" />仅收藏</label>
      <label><input v-model="filters.showEliminated" type="checkbox" />显示淘汰</label>
      <strong>成本估算 {{ estimatedCost }}</strong>
    </section>

    <section v-if="activeSet" class="comparison-grid">
      <article v-for="candidate in candidates" :key="candidate.candidate_id" class="candidate-card" :class="{ eliminated: candidate.is_eliminated }">
        <header><div><span>#{{ candidate.rank }} · {{ candidate.difference_direction }}</span><h3>{{ candidate.label }}</h3></div><strong>{{ candidate.score.total_score }}</strong></header>
        <div class="score-row"><span>忠实 {{ candidate.score.fidelity }}</span><span>钩子 {{ candidate.score.hook }}</span><span>节奏 {{ candidate.score.pacing }}</span><span>连续 {{ candidate.score.continuity }}</span><span>可拍 {{ candidate.score.filmability }}</span><span>风险 {{ candidate.score.modification_risk }}</span></div>
        <p class="duration">预计 {{ candidate.score.estimated_duration_seconds }} 秒 · seed {{ candidate.random_seed }} · {{ candidate.model }} / {{ candidate.prompt_version }}</p>
        <div v-if="isImage && candidate.content.media?.preview_url" class="image-preview"><img :src="candidate.content.media.preview_url" :alt="candidate.label" /></div>
        <video v-if="isVideo && candidate.content.media?.preview_url" :ref="(el) => { if (el && !videoRefs.includes(el)) videoRefs.push(el) }" controls :src="candidate.content.media.preview_url" @timeupdate="syncVideos" />
        <section v-for="component in candidate.content.components" :key="component.key" class="component"><b>{{ componentLabels[component.type] || component.title }}</b><p>{{ component.content }}</p></section>
        <details><summary>结构化 diff</summary><pre>{{ JSON.stringify(candidate.structured_diff, null, 2) }}</pre></details>
        <div class="reasons"><p><b>推荐理由</b> {{ candidate.score.recommendation_reasons.join('；') }}</p><p><b>扣分原因</b> {{ candidate.score.deduction_reasons.join('；') }}</p></div>
        <footer><button @click="decide(candidate, candidate.is_favorite ? 'unfavorite' : 'favorite')">{{ candidate.is_favorite ? '取消收藏' : '收藏' }}</button><button @click="decide(candidate, candidate.is_eliminated ? 'restore' : 'eliminate')">{{ candidate.is_eliminated ? '恢复' : '淘汰' }}</button><button class="button button-primary" @click="selectCandidate(candidate)">选择为 current</button></footer>
      </article>
    </section>

    <section v-if="isImage && candidates.length >= 2" class="panel image-compare">
      <h3>图片滑块 / 并排对比</h3><input v-model.number="imageMix" type="range" min="0" max="100" /><p>候选 A {{ imageMix }}% · 候选 B {{ 100-imageMix }}%</p>
    </section>
    <section v-if="isVideo" class="panel">
      <div class="panel-title"><h3>视频同步播放与时间码评论</h3><button @click="playTogether">同步播放</button></div>
      <form class="time-comment" @submit.prevent="addTimeComment"><select v-model="timeComment.candidate_id" required><option value="">选择候选</option><option v-for="item in candidates" :key="item.candidate_id" :value="item.candidate_id">{{ item.label }}</option></select><input v-model.number="timeComment.timecode_ms" type="number" min="0" placeholder="时间码 ms" /><input v-model="timeComment.comment_text" required placeholder="评论" /><button>保存</button></form>
    </section>

    <section v-if="activeSet && activeSet.component_types.length > 1" class="panel composition">
      <div class="panel-title"><div><h3>跨候选组合</h3><p>例如候选 A 的开场 + 候选 B 的高潮 + 候选 C 的结尾。</p></div><button class="button button-primary" :disabled="busy" @click="compose">确认组合并校验</button></div>
      <div class="composition-grid"><label v-for="component in activeSet.component_types" :key="component"><span>{{ componentLabels[component] }}</span><select v-model="composition[component]"><option value="">不选择</option><option v-for="candidate in activeSet.candidates" :key="candidate.candidate_id" :value="candidate.candidate_id">{{ candidate.label }} · {{ candidate.score.total_score }}</option></select></label></div>
    </section>

    <section v-if="selectionResult" class="panel validation">
      <h3>新 artifact：{{ selectionResult.artifact_id }}</h3>
      <p>旧版本未覆盖；只有这个已确认版本进入下游 current 绑定。</p>
      <div><span v-for="rule in validationRuleLabels(selectionResult.validation_summary)" :key="rule.rule" :class="{ pass: rule.passed }">{{ rule.label }} {{ rule.passed ? '通过' : '失败' }}</span></div>
    </section>
  </section>
</template>

<style scoped>
.candidate-page{display:grid;gap:18px}.page-head,.panel-title,.toolbar,.candidate-card header,.candidate-card footer{display:flex;justify-content:space-between;align-items:center;gap:12px}.panel,.candidate-card{background:var(--surface,#fff);border:1px solid var(--border,#dce2e8);border-radius:12px;padding:18px}.form-grid,.text-grid,.composition-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(210px,1fr));gap:12px}.generation-form label,.composition label{display:grid;gap:6px}.generation-form input,.generation-form select,.generation-form textarea,.toolbar input,.toolbar select,.composition select{padding:9px;border:1px solid #ccd4dd;border-radius:7px}.text-grid textarea{min-height:95px}.component-options,.score-row,.candidate-card footer,.validation div{display:flex;flex-wrap:wrap;gap:8px;margin:12px 0}.component-options label,.score-row span,.validation span{background:#edf2f7;border-radius:999px;padding:5px 9px}.comparison-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(340px,1fr));gap:14px}.candidate-card header>strong{font-size:30px;color:#315cbb}.candidate-card.eliminated{opacity:.55}.component{border-top:1px solid #e7ebef;padding-top:10px}.component p,.duration,.reasons{color:#5d6875}.candidate-card pre{max-height:230px;overflow:auto;white-space:pre-wrap}.candidate-card video,.image-preview img{width:100%;border-radius:8px}.time-comment{display:grid;grid-template-columns:180px 130px 1fr auto;gap:8px}.validation .pass{background:#daf2e2;color:#23653a}.notice{background:#e8f6ed;padding:12px;border-radius:8px}@media(max-width:720px){.page-head,.panel-title,.toolbar{align-items:stretch;display:grid}.time-comment{grid-template-columns:1fr}.comparison-grid{grid-template-columns:1fr}}
</style>
