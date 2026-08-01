<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { AlertTriangle, ArrowLeft, CheckCircle2, GitMerge, RefreshCw, ShieldCheck } from 'lucide-vue-next'
import StatusBadge from '../components/StatusBadge.vue'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'

const route = useRoute()
const proposal = ref(null)
const loading = ref(true)
const submitting = ref(false)
const savingItemId = ref('')
const error = ref('')
const notice = ref('')
const published = ref(null)
const filters = reactive({ item_type: '', change_type: '', resolution_status: '' })
const decisions = reactive({})

const typeLabels = {
  entity: '实体', fact: '事实', event: '事件', relation: '关系 / 因果', state: '人物状态',
  foreshadow: '伏笔', story_arc: '故事弧',
}
const changeLabels = {
  added: '新增', modified: '修改', deleted: '删除', relocated: '仅证据迁移', unchanged: '未变', conflict: '冲突',
}
const resolutionLabels = {
  accept_new: '接受新值', keep_old: '保留旧值', merge: '合并', manual_edit: '标记 / 完成人工编辑', delete_invalid: '删除无效事实',
}
const visibleItems = computed(() => (proposal.value?.items || []).filter((item) =>
  (!filters.item_type || item.item_type === filters.item_type) &&
  (!filters.change_type || item.change_type === filters.change_type) &&
  (!filters.resolution_status || item.resolution_status === filters.resolution_status)))
const canPublish = computed(() => proposal.value?.status === 'ready' && proposal.value?.unresolved_count === 0)

function pretty(value) {
  if (value == null) return '—'
  try { return JSON.stringify(value, null, 2) } catch { return String(value) }
}

function evidenceText(value) {
  if (!value) return '无原文证据'
  return value.evidence_text || '证据正文未缓存'
}

function evidenceLocator(value) {
  if (!value) return ''
  return `${value.chapter_id || ''} · byte ${value.start_utf8_byte ?? '—'}–${value.end_utf8_byte ?? '—'} · codepoint ${value.start_codepoint ?? '—'}–${value.end_codepoint ?? '—'}`
}

function initializeDecision(item) {
  if (decisions[item.ir_merge_item_id]) return
  decisions[item.ir_merge_item_id] = {
    resolution: item.resolution || '',
    resolvedText: item.resolved_value && typeof item.resolved_value === 'object'
      ? JSON.stringify(item.resolved_value, null, 2) : '',
    note: item.resolution_note || '',
    canonicalization_confirmed: item.canonicalization_confirmed || false,
    canonicalization_decision: item.canonicalization_decision || '',
    canonical_entity_id: item.canonical_entity_id || '',
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    proposal.value = (await narrativeApi.getIRMergeProposal(route.params.proposalId)).data
    for (const item of proposal.value.items) initializeDecision(item)
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function saveDecision(item) {
  const draft = decisions[item.ir_merge_item_id]
  if (!draft?.resolution) return
  let resolvedValue
  if (draft.resolvedText.trim()) {
    try { resolvedValue = JSON.parse(draft.resolvedText) } catch {
      error.value = '合并 / 人工编辑结果必须是有效 JSON。'
      return
    }
  }
  savingItemId.value = item.ir_merge_item_id
  error.value = ''
  notice.value = ''
  try {
    await narrativeApi.resolveIRMergeItem(proposal.value.ir_merge_proposal_id, item.ir_merge_item_id, {
      resolution: draft.resolution,
      ...(resolvedValue === undefined ? {} : { resolved_value: resolvedValue }),
      resolution_note: draft.note,
      canonicalization_confirmed: draft.canonicalization_confirmed,
      canonicalization_decision: draft.canonicalization_decision,
      canonical_entity_id: draft.canonical_entity_id,
    })
    proposal.value = (await narrativeApi.getIRMergeProposal(route.params.proposalId)).data
    notice.value = `已保存 ${typeLabels[item.item_type]} ${item.logical_id} 的裁决。`
  } catch (err) {
    error.value = err.message
  } finally {
    savingItemId.value = ''
  }
}

async function publishProposal() {
  if (!canPublish.value || !window.confirm('确认后将原子创建并发布新的不可变 full IR，所有原 IR 均保留。继续？')) return
  submitting.value = true
  error.value = ''
  notice.value = ''
  try {
    published.value = (await narrativeApi.publishIRMergeProposal(proposal.value.ir_merge_proposal_id, {
      confirmed: true,
    }, createIdempotencyKey(`publish-ir-merge-${proposal.value.ir_merge_proposal_id}`))).data
    await load()
    notice.value = `新 full IR ${published.value.full_ir_revision_id} 已发布；影响扫描已排队，未自动重建任何下游。`
  } catch (err) {
    error.value = err.message
  } finally {
    submitting.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="view-stack ir-merge-view">
    <RouterLink :to="proposal ? `/library/versions/${proposal.target_source_version_id}` : '/library'" class="back-link"><ArrowLeft :size="16" />返回 Source Version</RouterLink>
    <div v-if="loading && !proposal" class="detail-skeleton"><span></span><span></span><span></span></div>
    <div v-else-if="error && !proposal" class="error-banner large"><AlertTriangle :size="17" />{{ error }}<button @click="load">重试</button></div>
    <template v-else-if="proposal">
      <div class="detail-hero">
        <div class="detail-title"><div class="source-work-cover large"><GitMerge :size="25" /></div><div><div class="title-line"><h2>Incremental IR → Full IR 审核合并</h2><StatusBadge :status="proposal.status" /></div><p>{{ proposal.base_full_ir_revision_id }} → {{ proposal.incremental_ir_revision_id }}</p></div></div>
        <div class="detail-actions"><button class="button button-secondary" :disabled="loading" @click="load"><RefreshCw :size="16" />刷新</button><button class="button button-primary" :disabled="submitting || !canPublish" @click="publishProposal"><ShieldCheck :size="16" />{{ submitting ? '发布中…' : '确认发布 full IR' }}</button></div>
      </div>

      <div v-if="error" class="error-banner large"><AlertTriangle :size="17" />{{ error }}<button @click="error = ''">关闭</button></div>
      <div v-if="notice" class="success-banner"><CheckCircle2 :size="17" />{{ notice }}</div>
      <div class="version-stats">
        <div><span>变更章节</span><strong>{{ proposal.changed_chapter_ids.length }}</strong></div>
        <div><span>差异项</span><strong>{{ proposal.items.length }}</strong></div>
        <div><span>冲突</span><strong>{{ proposal.conflict_count }}</strong></div>
        <div><span>未处理</span><strong>{{ proposal.unresolved_count }}</strong></div>
      </div>

      <article v-if="proposal.unresolved_count" class="contract-notice warning impact-warning"><AlertTriangle :size="18" />尚有 {{ proposal.unresolved_count }} 项未完成；同名人物或别名冲突必须明确选择“同一实体 / 不同实体”后才能发布。</article>
      <article v-if="published" class="panel padded published-result"><h3>已发布不可变 full IR</h3><code>{{ published.full_ir_revision_id }}</code><p>Source change set：<code>{{ published.source_change_set_id }}</code>。已生成影响扫描与可选择的 regeneration proposal，不会自动重建。</p></article>

      <article class="panel padded merge-filter-panel">
        <div class="section-title"><div><span>VERSION COMPARISON</span><h3>实体与叙事差异</h3></div><p>置信度 {{ Math.round(proposal.confidence * 100) }}%</p></div>
        <div class="merge-filters">
          <label class="field"><span>类型</span><select v-model="filters.item_type"><option value="">全部</option><option v-for="(label, key) in typeLabels" :key="key" :value="key">{{ label }}</option></select></label>
          <label class="field"><span>变化</span><select v-model="filters.change_type"><option value="">全部</option><option v-for="(label, key) in changeLabels" :key="key" :value="key">{{ label }}</option></select></label>
          <label class="field"><span>处理状态</span><select v-model="filters.resolution_status"><option value="">全部</option><option value="unresolved">未处理</option><option value="needs_manual_edit">需人工编辑</option><option value="resolved">已处理</option></select></label>
        </div>
      </article>

      <article v-for="item in visibleItems" :key="item.ir_merge_item_id" class="panel padded merge-item" :class="{ conflicted: item.conflict_code }">
        <header><div><span>{{ typeLabels[item.item_type] }} · {{ changeLabels[item.change_type] }}</span><h3>{{ item.logical_id }}</h3></div><div class="merge-item-status"><StatusBadge :status="item.resolution_status" /><b>{{ Math.round(item.confidence * 100) }}%</b></div></header>
        <div v-if="item.conflict_message" class="contract-notice warning"><AlertTriangle :size="16" />{{ item.conflict_message }} <code>{{ item.conflict_code }}</code></div>
        <div class="value-comparison">
          <section><h4>Base full IR</h4><pre>{{ pretty(item.before_value) }}</pre></section>
          <section><h4>Incremental IR</h4><pre>{{ pretty(item.after_value) }}</pre></section>
        </div>
        <div class="evidence-comparison">
          <section><h4>旧原文证据</h4><small>{{ evidenceLocator(item.before_evidence) }}</small><blockquote>{{ evidenceText(item.before_evidence) }}</blockquote></section>
          <section><h4>新原文证据</h4><small>{{ evidenceLocator(item.after_evidence) }}</small><blockquote>{{ evidenceText(item.after_evidence) }}</blockquote></section>
        </div>
        <div v-if="item.source_span_changed && !item.semantic_changed" class="relocation-note"><CheckCircle2 :size="16" />原文位置改变但语义指纹未变：仅记录 relocation，不触发语义失效。</div>
        <form class="merge-resolution" @submit.prevent="saveDecision(item)">
          <label class="field"><span>处理方式</span><select v-model="decisions[item.ir_merge_item_id].resolution" required><option value="">请选择</option><option v-for="(label, key) in resolutionLabels" :key="key" :value="key">{{ label }}</option></select></label>
          <template v-if="['merge', 'manual_edit'].includes(decisions[item.ir_merge_item_id].resolution)">
            <label class="field full"><span>合并 / 人工编辑后的 JSON</span><textarea v-model="decisions[item.ir_merge_item_id].resolvedText" rows="8" placeholder="留空会保持“需人工编辑”，填写有效 JSON 后才视为已解决。"></textarea></label>
          </template>
          <template v-if="item.canonicalization_required">
            <label class="test-ack full"><input v-model="decisions[item.ir_merge_item_id].canonicalization_confirmed" type="checkbox" /><span>我已人工核对人物身份与别名</span></label>
            <label class="field"><span>Canonicalization</span><select v-model="decisions[item.ir_merge_item_id].canonicalization_decision"><option value="">请选择</option><option value="same_entity">同一实体</option><option value="distinct_entities">不同实体</option></select></label>
            <label v-if="decisions[item.ir_merge_item_id].canonicalization_decision === 'same_entity'" class="field"><span>Canonical Entity ID</span><input v-model="decisions[item.ir_merge_item_id].canonical_entity_id" required /></label>
          </template>
          <label class="field full"><span>审核备注</span><input v-model="decisions[item.ir_merge_item_id].note" /></label>
          <button class="button button-secondary" :disabled="savingItemId === item.ir_merge_item_id">{{ savingItemId === item.ir_merge_item_id ? '保存中…' : '保存本项裁决' }}</button>
        </form>
      </article>

      <article class="panel padded impact-preview">
        <div class="section-title"><div><span>IMPACT PREVIEW</span><h3>发布后影响预览</h3></div><p>语义变化 {{ proposal.impact_preview.semantic_change_count }} · 仅 relocation {{ proposal.impact_preview.relocation_only_count }}</p></div>
        <p>只命中显式事实 / 事件引用及其依赖链。默认不选择重建项；正式扫描完成后生成 regeneration proposal。</p>
        <div v-if="!proposal.impact_preview.affected_artifacts.length" class="compact-empty">当前没有下游产物命中这些语义变化。</div>
        <div v-else class="preview-artifacts"><div v-for="artifact in proposal.impact_preview.affected_artifacts" :key="artifact.artifact_id"><b>{{ artifact.artifact_type }}</b><code>{{ artifact.native_entity_id }}</code><small>依赖深度 {{ artifact.propagation_depth }}</small></div></div>
      </article>
    </template>
  </section>
</template>

<style scoped>
.impact-warning{display:flex;gap:10px;align-items:center}.published-result code{word-break:break-all}.merge-filter-panel{display:grid;gap:14px}.merge-filters{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:14px}.merge-item{display:grid;gap:16px}.merge-item.conflicted{border-color:#d99b45}.merge-item>header{display:flex;justify-content:space-between;gap:16px}.merge-item>header span{color:var(--muted);font-size:12px}.merge-item>header h3{margin:4px 0 0;word-break:break-all}.merge-item-status{display:flex;gap:10px;align-items:center}.value-comparison,.evidence-comparison{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}.value-comparison section,.evidence-comparison section{border:1px solid var(--line);border-radius:12px;padding:14px;min-width:0}.value-comparison h4,.evidence-comparison h4{margin:0 0 8px}.value-comparison pre{white-space:pre-wrap;word-break:break-word;max-height:360px;overflow:auto;font-size:12px}.evidence-comparison small{color:var(--muted);word-break:break-all}.evidence-comparison blockquote{margin:10px 0 0;white-space:pre-wrap}.relocation-note{display:flex;align-items:center;gap:8px;color:#317a50;background:#edf9f1;border-radius:10px;padding:10px 12px}.merge-resolution{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px;border-top:1px solid var(--line);padding-top:16px}.merge-resolution .full{grid-column:1/-1}.preview-artifacts{display:grid;gap:8px}.preview-artifacts>div{display:grid;grid-template-columns:180px 1fr auto;gap:12px;border-top:1px solid var(--line);padding:10px 0}.preview-artifacts small{color:var(--muted)}@media(max-width:850px){.merge-filters,.value-comparison,.evidence-comparison,.merge-resolution{grid-template-columns:1fr}.merge-resolution .full{grid-column:auto}.preview-artifacts>div{grid-template-columns:1fr}}
</style>
