<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, BookOpenCheck, Film, GitCompareArrows, LockKeyhole, RefreshCw, ShieldAlert, Sparkles, UsersRound } from 'lucide-vue-next'
import StatusBadge from '../components/StatusBadge.vue'
import {
  cloneBibleAsVersion, continuitySummary, handoffActionLabel, issueLocator,
  performanceContinuityApi, sortedIssues,
} from '../services/performanceContinuity.js'

const route = useRoute()
const tabs = [
  ['bibles', '角色表演圣经', UsersRound],
  ['ledger', '连续性时间线', GitCompareArrows],
  ['qc', '视觉 QC', ShieldAlert],
  ['handoffs', '首尾帧衔接', Film],
]
const activeTab = ref('bibles')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const bibles = ref([])
const ledger = ref([])
const issues = ref([])
const handoffs = ref([])
const selectedBibleId = ref('')
const changeReason = ref('')
const editor = reactive({ speech: '', acting: '', relational_voices: '', appearance: '', stage_states: '', locked_fields: '', allowed_fields: '' })
const selectedBible = computed(() => bibles.value.find((item) => item.performance_bible_id === selectedBibleId.value) || bibles.value[0])
const orderedIssues = computed(() => sortedIssues(issues.value))
const pretty = (value) => JSON.stringify(value ?? {}, null, 2)

function selectBible(bible) {
  selectedBibleId.value = bible.performance_bible_id
  for (const key of Object.keys(editor)) editor[key] = pretty(bible[key])
  changeReason.value = ''
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const id = route.params.projectId
    ;[bibles.value, ledger.value, issues.value, handoffs.value] = await Promise.all([
      performanceContinuityApi.listBibles(id), performanceContinuityApi.ledger(id),
      performanceContinuityApi.issues(id), performanceContinuityApi.handoffs(id),
    ])
    if (bibles.value.length) selectBible(bibles.value[0])
  } catch (err) { error.value = err.message }
  finally { loading.value = false }
}

async function saveVersion() {
  saving.value = true
  try {
    const edits = Object.fromEntries(Object.entries(editor).map(([key, value]) => [key, JSON.parse(value)]))
    const created = await performanceContinuityApi.createBibleVersion(
      route.params.projectId, cloneBibleAsVersion(selectedBible.value, edits, changeReason.value),
    )
    bibles.value.unshift(created)
    selectBible(created)
  } catch (err) { error.value = err.message }
  finally { saving.value = false }
}

async function lockSelected() {
  saving.value = true
  try {
    const updated = await performanceContinuityApi.lockBible(selectedBible.value.performance_bible_id)
    bibles.value = bibles.value.map((item) => item.performance_bible_id === updated.performance_bible_id ? updated : item)
    selectBible(updated)
  } catch (err) { error.value = err.message }
  finally { saving.value = false }
}

async function createRedo(issue) {
  saving.value = true
  try {
    const plan = await performanceContinuityApi.createRedo(issue.visual_qc_issue_id, 'cms-editor')
    issues.value = issues.value.map((item) => item.visual_qc_issue_id === issue.visual_qc_issue_id
      ? { ...item, status: 'planned', change_plan_id: plan.change_plan_id } : item)
  } catch (err) { error.value = err.message }
  finally { saving.value = false }
}

onMounted(load)
</script>

<template>
  <section class="view-stack phase4">
    <div class="head">
      <RouterLink :to="`/projects/${route.params.projectId}`" class="back-link"><ArrowLeft :size="16" />返回项目</RouterLink>
      <button class="button button-secondary" :disabled="loading" @click="load"><RefreshCw :size="16" />刷新状态</button>
    </div>
    <div v-if="error" class="error-banner large">{{ error }}</div>
    <div class="tabs">
      <button v-for="[key,label,icon] in tabs" :key="key" :class="{ active: activeTab === key }" @click="activeTab = key">
        <component :is="icon" :size="17" />{{ label }}
      </button>
    </div>
    <div v-if="loading" class="panel padded">正在读取第四阶段状态…</div>

    <div v-else-if="activeTab === 'bibles'" class="split">
      <aside class="panel bible-list">
        <button v-for="bible in bibles" :key="bible.performance_bible_id" :class="{ active: selectedBible?.performance_bible_id === bible.performance_bible_id }" @click="selectBible(bible)">
          <div><strong>{{ bible.character_id }}</strong><StatusBadge :status="bible.status" /></div>
          <span>{{ bible.character_version }} · v{{ bible.version }}</span>
        </button>
        <p v-if="!bibles.length">尚无角色表演圣经。</p>
      </aside>
      <article v-if="selectedBible" class="panel padded">
        <div class="head">
          <div><small>PERFORMANCE CONTRACT</small><h3>{{ selectedBible.character_id }} · {{ selectedBible.character_version }} · v{{ selectedBible.version }}</h3></div>
          <button class="button button-secondary" :disabled="saving || selectedBible.status === 'locked'" @click="lockSelected"><LockKeyhole :size="16" />锁定版本</button>
        </div>
        <div class="lock-note"><LockKeyhole :size="15" />锁定字段 {{ selectedBible.locked_fields?.length || 0 }} 个；普通生成任务不能修改。</div>
        <div class="edit-grid">
          <label>语速、音高、停顿、口头禅<textarea v-model="editor.speech" :disabled="selectedBible.status === 'locked'" /></label>
          <label>情绪、肢体习惯与禁区<textarea v-model="editor.acting" :disabled="selectedBible.status === 'locked'" /></label>
          <label>关系语气差异<textarea v-model="editor.relational_voices" :disabled="selectedBible.status === 'locked'" /></label>
          <label>声音/脸型/发型/年龄感/体态<textarea v-model="editor.appearance" :disabled="selectedBible.status === 'locked'" /></label>
          <label>剧情阶段服装/伤痕/道具/心理/关系<textarea v-model="editor.stage_states" :disabled="selectedBible.status === 'locked'" /></label>
          <label>锁定字段<textarea v-model="editor.locked_fields" :disabled="selectedBible.status === 'locked'" /></label>
          <label>允许变化字段<textarea v-model="editor.allowed_fields" :disabled="selectedBible.status === 'locked'" /></label>
        </div>
        <div class="save-row">
          <input v-model="changeReason" :disabled="selectedBible.status === 'locked'" placeholder="填写新版本变化理由" />
          <button class="button button-primary" :disabled="saving || selectedBible.status === 'locked' || !changeReason.trim()" @click="saveVersion"><BookOpenCheck :size="16" />保存为新版本</button>
        </div>
      </article>
    </div>

    <article v-else-if="activeTab === 'ledger'" class="panel padded">
      <div class="section-title"><div><span>CONTINUITY TIMELINE</span><h3>场 / 镜输入与输出状态</h3></div></div>
      <div v-for="entry in ledger" :key="entry.continuity_entry_id" class="ledger-card" :class="{ conflict: entry.validation_status === 'conflict' }">
        <div class="head"><strong>第 {{ entry.episode_number }} 集 · {{ entry.scene_id || '集级' }} · {{ entry.shot_id || entry.scope }}</strong><StatusBadge :status="entry.validation_status" /></div>
        <p>{{ continuitySummary(entry).environment.location_id || '未标场景' }} · {{ continuitySummary(entry).environment.time || '未标时间' }} · {{ continuitySummary(entry).environment.weather || '未标天气' }} · 轴线 {{ continuitySummary(entry).axis }}</p>
        <div class="chips"><span v-for="character in continuitySummary(entry).characters" :key="character.id">{{ character.id }}：{{ character.position }} / {{ character.facing }} / {{ character.costume }} / {{ character.held }} / {{ character.emotion }}</span></div>
        <details><summary>输入 / 输出 JSON</summary><div class="state-pair"><pre>{{ pretty(entry.input_state) }}</pre><pre>{{ pretty(entry.output_state) }}</pre></div></details>
      </div>
      <p v-if="!ledger.length">尚无连续性状态；生成门禁会返回可解释诊断。</p>
    </article>

    <article v-else-if="activeTab === 'qc'" class="panel padded">
      <div class="section-title"><div><span>FRAME-LOCATED VISUAL QC</span><h3>跨镜头视觉问题（{{ orderedIssues.length }}）</h3></div></div>
      <div v-for="issue in orderedIssues" :key="issue.visual_qc_issue_id" class="issue" :class="issue.severity">
        <i></i><div><div class="issue-title"><strong>{{ issue.category }}</strong><StatusBadge :status="issue.severity" /><StatusBadge :status="issue.status" /></div><code>{{ issueLocator(issue) }}</code><p>{{ issue.recommendation }}</p><details><summary>证据</summary><pre>{{ pretty(issue.evidence) }}</pre></details></div>
        <button class="button button-secondary" :disabled="saving || issue.status !== 'open'" @click="createRedo(issue)"><Sparkles :size="15" />{{ issue.status === 'planned' ? '已创建计划' : '创建局部修改计划' }}</button>
      </div>
      <p v-if="!orderedIssues.length">当前没有未解决的跨镜头视觉问题。</p>
    </article>

    <article v-else class="panel padded">
      <div class="section-title"><div><span>TAIL / HEAD COMPARISON</span><h3>相邻镜头首尾帧与动作接力</h3></div></div>
      <div class="handoff-grid">
        <div v-for="handoff in handoffs" :key="handoff.shot_handoff_id" class="handoff">
          <div class="head"><strong>{{ handoff.from_shot_id }} → {{ handoff.to_shot_id }}</strong><StatusBadge :status="handoff.status" /></div>
          <div class="frames">
            <figure><div><img v-if="handoff.target_tail_frame_ref" :src="handoff.target_tail_frame_ref" alt="目标尾帧" /><span v-else>目标尾帧</span></div><figcaption>上一镜输出</figcaption></figure>
            <figure><div><img v-if="handoff.reference_head_frame_ref" :src="handoff.reference_head_frame_ref" alt="参考首帧" /><span v-else>参考首帧</span></div><figcaption>下一镜输入</figcaption></figure>
          </div>
          <b>{{ handoffActionLabel(handoff) }}</b>
          <p>方向 {{ handoff.motion_direction || '—' }} · 视线 {{ handoff.gaze_constraint || '—' }} · 景别 {{ handoff.shot_size_constraint || '—' }}</p>
        </div>
      </div>
      <p v-if="!handoffs.length">尚无衔接记录；视频生成门禁会阻断。</p>
    </article>
  </section>
</template>

<style scoped>
.head,.tabs,.save-row,.issue-title{display:flex;align-items:center;justify-content:space-between;gap:12px}.tabs{justify-content:flex-start;border-bottom:1px solid #e5e7eb}.tabs button{display:flex;align-items:center;gap:7px;padding:12px 16px;border:0;background:none;cursor:pointer;color:#64748b}.tabs button.active{color:#6d28d9;border-bottom:2px solid #7c3aed}.split{display:grid;grid-template-columns:260px 1fr;gap:18px}.bible-list{padding:8px}.bible-list button{width:100%;padding:12px;border:0;border-radius:9px;background:none;text-align:left}.bible-list button.active{background:#f3e8ff}.bible-list button div{display:flex;justify-content:space-between}.bible-list span{font-size:12px;color:#64748b}.lock-note{display:flex;gap:7px;padding:10px;background:#fffbeb;border:1px solid #fde68a;border-radius:8px}.edit-grid{display:grid;grid-template-columns:1fr 1fr;gap:12px;margin:16px 0}.edit-grid label{display:grid;gap:6px;font-size:13px;font-weight:600}.edit-grid textarea{min-height:145px;padding:9px;border:1px solid #dbe2ea;border-radius:8px;font:12px/1.45 ui-monospace,monospace}.save-row input{flex:1;padding:10px;border:1px solid #dbe2ea;border-radius:8px}.ledger-card,.issue,.handoff{border:1px solid #e5e7eb;border-radius:9px;padding:13px;margin-bottom:10px}.ledger-card.conflict{border-color:#dc2626}.chips{display:flex;flex-wrap:wrap;gap:6px}.chips span{padding:5px 8px;background:#f8fafc;border-radius:6px;font-size:12px}.state-pair,.frames{display:grid;grid-template-columns:1fr 1fr;gap:8px}.state-pair pre,.issue pre{max-height:240px;overflow:auto;background:#0f172a;color:#e2e8f0;padding:9px;border-radius:7px;font-size:11px}.issue{display:grid;grid-template-columns:7px 1fr auto;gap:12px}.issue>i{border-radius:4px;background:#94a3b8}.issue.critical>i,.issue.blocking>i{background:#dc2626}.issue.major>i{background:#f59e0b}.issue code{font-size:12px;color:#475569}.handoff-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:12px}.frames figure{margin:10px 0}.frames figure>div{aspect-ratio:9/16;background:#0f172a;color:#94a3b8;border-radius:7px;display:grid;place-items:center}.frames img{width:100%;height:100%;object-fit:cover;border-radius:7px}.frames figcaption{text-align:center;font-size:12px;color:#64748b}.handoff b{display:block;padding:8px;background:#f5f3ff;color:#6d28d9;border-radius:7px}@media(max-width:900px){.split,.edit-grid,.state-pair{grid-template-columns:1fr}}
</style>
