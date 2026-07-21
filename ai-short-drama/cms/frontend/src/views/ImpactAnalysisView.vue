<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertTriangle, ArrowLeft, CheckCircle2, GitCompareArrows, RefreshCw } from 'lucide-vue-next'
import StatusBadge from '../components/StatusBadge.vue'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'

const route = useRoute()
const router = useRouter()
const targetVersionId = ref(String(route.query.to_source_version_id || ''))
const impact = ref(null)
const loading = ref(false)
const submitting = ref(false)
const error = ref('')
const notice = ref('')
const selectedArtifactIds = ref([])
const strategy = ref('selective')
const decisionKey = ref('')
const approvedAffected = computed(() => impact.value?.affected_artifacts.filter((item) =>
  item.review_status === 'approved' || item.review_status === 'waiting_review') || [])

async function loadImpact() {
  if (!targetVersionId.value.trim()) return
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    const response = await narrativeApi.getProjectImpact(route.params.projectId, targetVersionId.value.trim())
    impact.value = response.data
    selectedArtifactIds.value = []
    decisionKey.value = ''
    await router.replace({ query: { to_source_version_id: targetVersionId.value.trim() } })
  } catch (err) {
    impact.value = null
    error.value = err.status === 404
      ? '尚未找到该版本的影响报告。增量 IR 和失效扫描可能仍在运行。'
      : err.message
  } finally {
    loading.value = false
  }
}

async function requestRegeneration() {
  if (!impact.value || !selectedArtifactIds.value.length) return
  submitting.value = true
  error.value = ''
  notice.value = ''
  if (!decisionKey.value) decisionKey.value = createIdempotencyKey('impact-regeneration')
  try {
    const response = await narrativeApi.createRegenerationRequest(route.params.projectId, impact.value.source_change_set_id, {
      strategy: strategy.value,
      artifact_ids: selectedArtifactIds.value,
    }, decisionKey.value)
    notice.value = `已记录再生成决定 ${response.data.regeneration_request_id}。系统不会覆盖现有审核产物。`
    decisionKey.value = ''
  } catch (err) {
    error.value = err.message
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  if (targetVersionId.value) loadImpact()
})
</script>

<template>
  <section class="view-stack impact-view">
    <RouterLink :to="`/projects/${route.params.projectId}`" class="back-link"><ArrowLeft :size="16" />返回项目</RouterLink>
    <div class="detail-hero">
      <div class="detail-title"><div class="source-work-cover large"><GitCompareArrows :size="25" /></div><div><div class="title-line"><h2>章节修订影响分析</h2><StatusBadge v-if="impact" :status="impact.status" /></div><p>只展示显式来源与依赖命中的产物，不以向量相似度推断影响。</p></div></div>
    </div>

    <form class="panel padded impact-query" @submit.prevent="loadImpact">
      <label class="field"><span>修订后的 Source Version ID</span><input v-model="targetVersionId" required placeholder="sv_..." /></label>
      <button class="button button-primary" :disabled="loading || !targetVersionId.trim()"><RefreshCw :size="16" />{{ loading ? '分析加载中…' : '查看影响范围' }}</button>
    </form>
    <div v-if="error" class="error-banner large"><AlertTriangle :size="17" />{{ error }}</div>
    <div v-if="notice" class="success-banner"><CheckCircle2 :size="17" />{{ notice }}</div>

    <template v-if="impact">
      <div class="version-stats">
        <div><span>修订章节</span><strong>{{ impact.changed_chapter_ids.length }}</strong></div>
        <div><span>事件变化</span><strong>{{ impact.changed_events.length }}</strong></div>
        <div><span>人物状态变化</span><strong>{{ impact.changed_character_states.length }}</strong></div>
        <div><span>受影响产物</span><strong>{{ impact.affected_artifacts.length }}</strong></div>
      </div>

      <article v-if="approvedAffected.length" class="contract-notice warning impact-warning">
        <AlertTriangle :size="18" />{{ approvedAffected.length }} 个已审核或待审核产物受到影响。它们只被标记 stale，内容和审核状态均已保留。
      </article>

      <article class="panel padded">
        <div class="section-title"><div><span>NARRATIVE IR DIFF</span><h3>事实变化</h3></div></div>
        <div class="impact-groups">
          <section><h4>事件</h4><p v-if="!impact.changed_events.length">无语义变化</p><ul><li v-for="item in impact.changed_events" :key="item.source_change_item_id"><StatusBadge :status="item.change_type" /><code>{{ item.details.logical_fact_id }}</code><span>{{ item.details.chapter_id }}</span></li></ul></section>
          <section><h4>人物状态</h4><p v-if="!impact.changed_character_states.length">无状态变化</p><ul><li v-for="item in impact.changed_character_states" :key="item.source_change_item_id"><StatusBadge :status="item.change_type" /><code>{{ item.details.character_entity_id }}</code><span>{{ item.details.state_dimension }}</span></li></ul></section>
          <section><h4>故事弧</h4><p v-if="!impact.affected_story_arcs.length">无故事弧受影响</p><ul><li v-for="item in impact.affected_story_arcs" :key="item.source_change_item_id"><StatusBadge :status="item.change_type" /><span>{{ item.details.title || item.details.logical_story_arc_id }}</span></li></ul></section>
        </div>
      </article>

      <form class="panel padded" @submit.prevent="requestRegeneration">
        <div class="section-title"><div><span>EXPLICIT REGENERATION DECISION</span><h3>选择要重新生成的产物</h3></div><select v-model="strategy"><option value="selective">选择性再生成</option><option value="full_recompile">从改编计划重新编译</option></select></div>
        <p class="decision-help">默认不选择任何产物。提交只创建可审计的再生成请求，不会立即删除、覆盖或改写现有内容。</p>
        <div v-if="!impact.affected_artifacts.length" class="compact-empty">没有命中显式依赖的派生产物。</div>
        <label v-for="artifact in impact.affected_artifacts" :key="artifact.artifact_id" class="artifact-choice">
          <input v-model="selectedArtifactIds" type="checkbox" :value="artifact.artifact_id" />
          <span><b>{{ artifact.artifact_type }}</b><code>{{ artifact.native_entity_id }}</code></span>
          <small>深度 {{ artifact.propagation_depth }} · {{ artifact.review_status || '未审核' }} · {{ artifact.before_status }} → stale</small>
        </label>
        <button class="button button-primary" :disabled="submitting || !selectedArtifactIds.length">{{ submitting ? '记录中…' : `确认重新生成 ${selectedArtifactIds.length} 项` }}</button>
      </form>
    </template>
  </section>
</template>

<style scoped>
.impact-query{display:flex;gap:16px;align-items:end}.impact-query .field{flex:1}.impact-warning{display:flex;gap:10px;align-items:center}.impact-groups{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px}.impact-groups section{border:1px solid var(--line);border-radius:12px;padding:16px}.impact-groups h4{margin:0 0 12px}.impact-groups ul{list-style:none;margin:0;padding:0;display:grid;gap:8px}.impact-groups li{display:flex;gap:8px;align-items:center;flex-wrap:wrap}.artifact-choice{display:grid;grid-template-columns:auto 1fr auto;gap:12px;align-items:center;border-top:1px solid var(--line);padding:14px 4px}.artifact-choice span{display:flex;gap:10px;align-items:center}.artifact-choice small{color:var(--muted)}.decision-help{color:var(--muted);margin-bottom:12px}@media(max-width:900px){.impact-groups{grid-template-columns:1fr}.artifact-choice{grid-template-columns:auto 1fr}.artifact-choice small{grid-column:2}.impact-query{align-items:stretch;flex-direction:column}}
</style>
