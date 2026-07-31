<script setup>
import { computed, ref, watch } from 'vue'
import { AlertTriangle, CheckCircle2, RefreshCw, ShieldCheck } from 'lucide-vue-next'
import { api } from '../services/api'
import {
  defaultResolverStage, effectiveInputKinds, effectiveInputStateLabel,
  resolverStages, summarizeEffectiveInputs,
} from '../services/effectiveInputs'

const props = defineProps({
  projectId: { type: String, required: true },
  episodeId: { type: String, default: '' },
  currentStage: { type: String, default: '' },
})

const stage = ref(defaultResolverStage(props.currentStage))
const resolution = ref(null)
const loading = ref(false)
const error = ref('')
const summary = computed(() => summarizeEffectiveInputs(resolution.value || {}))
const shortHash = value => value ? `${value.slice(0, 12)}…${value.slice(-8)}` : '—'

async function load() {
  if (!props.projectId) return
  loading.value = true
  error.value = ''
  try {
    resolution.value = await api.getEffectiveInputs(props.projectId, props.episodeId, stage.value)
  } catch (err) {
    error.value = err.message
    resolution.value = null
  } finally {
    loading.value = false
  }
}

watch(() => [props.projectId, props.episodeId, stage.value], load, { immediate: true })
</script>

<template>
  <article class="panel padded effective-input-panel">
    <div class="resolver-head">
      <div>
        <span>EFFECTIVE INPUT RESOLVER</span>
        <h3><ShieldCheck :size="19" />权威输入诊断</h3>
      </div>
      <div class="resolver-actions">
        <select v-model="stage" aria-label="解析阶段">
          <option v-for="[value, label] in resolverStages" :key="value" :value="value">{{ value }} · {{ label }}</option>
        </select>
        <button class="button button-secondary" :disabled="loading" @click="load">
          <RefreshCw :size="15" :class="{ spin: loading }" />刷新
        </button>
      </div>
    </div>

    <div v-if="error" class="resolver-error"><AlertTriangle :size="16" />{{ error }}</div>
    <template v-else-if="resolution">
      <div class="resolver-summary" :class="{ blocked: !summary.ready }">
        <CheckCircle2 v-if="summary.ready" :size="18" />
        <AlertTriangle v-else :size="18" />
        <strong>{{ summary.ready ? '输入完整，可执行' : `执行被阻断（${summary.blocked} 项）` }}</strong>
        <span>模式 {{ resolution.mode }} · required {{ summary.required }} · optional {{ summary.optional }} · 已解析 {{ summary.resolved }}</span>
        <code title="上下文哈希">{{ shortHash(resolution.context_hash) }}</code>
      </div>
      <div v-if="resolution.blockers?.length" class="resolver-blockers">
        <div v-for="item in resolution.blockers" :key="`${item.kind}:${item.reason}`">
          <b>{{ effectiveInputKinds[item.kind] || item.kind }}</b>
          <span>{{ effectiveInputStateLabel(item.state) }}</span>
          <code>{{ item.reason }}</code>
        </div>
      </div>
      <div class="resolver-table-wrap">
        <table>
          <thead><tr><th>输入</th><th>要求</th><th>状态</th><th>ID / 版本</th><th>内容哈希</th></tr></thead>
          <tbody>
            <tr v-for="item in resolution.items" :key="item.kind" :class="{ danger: item.blocks }">
              <td><strong>{{ effectiveInputKinds[item.kind] || item.kind }}</strong></td>
              <td>{{ item.requirement }}</td>
              <td>{{ effectiveInputStateLabel(item.state) }}<small v-if="item.reason">{{ item.reason }}</small></td>
              <td><code>{{ item.input_ids?.join(', ') || '—' }}</code><small>{{ JSON.stringify(item.versions || []) }}</small></td>
              <td><code>{{ shortHash(item.content_hash) }}</code></td>
            </tr>
          </tbody>
        </table>
      </div>
      <small class="resolver-audit">resolution {{ resolution.resolution_id }} · audit {{ shortHash(resolution.resolution_hash) }}</small>
    </template>
  </article>
</template>

<style scoped>
.effective-input-panel { display: grid; gap: 14px; }
.resolver-head,.resolver-actions,.resolver-summary { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.resolver-head span { color: var(--muted); font-size: 11px; letter-spacing: .12em; }
.resolver-head h3 { display: flex; align-items: center; gap: 7px; margin: 3px 0 0; }
.resolver-actions select { min-width: 150px; padding: 9px 10px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); color: inherit; }
.resolver-summary { justify-content: flex-start; padding: 11px 13px; border-radius: 9px; color: #166534; background: #f0fdf4; }
.resolver-summary.blocked { color: #991b1b; background: #fef2f2; }
.resolver-summary span { color: inherit; opacity: .82; }
.resolver-summary code { margin-left: auto; }
.resolver-error,.resolver-blockers div { display: flex; align-items: center; gap: 9px; padding: 9px 11px; color: #991b1b; background: #fef2f2; border-radius: 8px; }
.resolver-blockers { display: grid; gap: 6px; }
.resolver-blockers code { margin-left: auto; }
.resolver-table-wrap { overflow: auto; }
table { width: 100%; border-collapse: collapse; font-size: 13px; }
th,td { padding: 10px; text-align: left; border-bottom: 1px solid var(--border); vertical-align: top; }
td small { display: block; margin-top: 4px; color: var(--muted); }
tr.danger td { background: #fffafa; }
.resolver-audit { color: var(--muted); }
</style>
