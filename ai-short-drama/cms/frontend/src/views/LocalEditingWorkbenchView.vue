<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  AlertTriangle, ArrowRight, CheckCircle2, ClipboardList, GitCompareArrows,
  History, LoaderCircle, MessageSquareText, Play, RefreshCw, ShieldCheck,
} from 'lucide-vue-next'
import { api } from '../services/api'
import {
  buildChangePlanRequest, changeStatusLabels, entityOptions, planDiffRows, rebuildLabels,
} from '../services/localEditing'

const route = useRoute()
const projectId = computed(() => route.params.projectId)
const form = reactive({
  instruction: '', entity_type: route.query.entity_type || 'dialogue',
  entity_id: route.query.entity_id || '', version: Number(route.query.version || 1),
  requested_by: '',
})
const plan = ref(null)
const history = ref([])
const versions = ref([])
const comments = ref([])
const comment = reactive({ body: '', timecode_start_ms: '', timecode_end_ms: '', author: '' })
const loading = ref(false)
const error = ref('')
const notice = ref('')

const diffRows = computed(() => planDiffRows(plan.value?.plan))
const rebuilds = computed(() => rebuildLabels(plan.value?.plan))
const canConfirm = computed(() => plan.value?.status === 'validated')
const canExecute = computed(() => plan.value?.status === 'confirmed')

async function loadHistory() {
  history.value = await api.getChangePlans(projectId.value)
}

async function loadBindings() {
  if (!form.entity_id) {
    versions.value = []
    comments.value = []
    return
  }
  ;[versions.value, comments.value] = await Promise.all([
    api.getEntityVersions(projectId.value, form.entity_type, form.entity_id),
    api.getChangeComments(projectId.value, form.entity_type, form.entity_id),
  ])
}

onMounted(async () => {
  try {
    await Promise.all([loadHistory(), loadBindings()])
  } catch (err) {
    error.value = err.message
  }
})
watch(() => [form.entity_type, form.entity_id], loadBindings)

async function generatePlan() {
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    plan.value = await api.createChangePlan(projectId.value, buildChangePlanRequest(form))
    notice.value = '修改计划已通过 Schema 和业务规则验证；尚未写入正式业务数据。'
    await loadHistory()
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function confirmPlan() {
  loading.value = true
  error.value = ''
  try {
    plan.value = await api.confirmChangePlan(projectId.value, plan.value.change_plan_id, {
      actor: form.requested_by || undefined,
    })
    notice.value = '计划已确认。仍需单独点击“执行修改”才会写入。'
    await loadHistory()
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function executePlan() {
  loading.value = true
  error.value = ''
  try {
    plan.value = await api.executeChangePlan(projectId.value, plan.value.change_plan_id)
    notice.value = '局部修改已原子应用，旧版本保留；精确重建任务已进入 pending，等待真实 worker 执行。'
    form.version = plan.value.plan.target.version + 1
    await Promise.all([loadHistory(), loadBindings()])
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function addComment() {
  if (!comment.body.trim()) return
  loading.value = true
  error.value = ''
  try {
    await api.createChangeComment(projectId.value, {
      entity_type: form.entity_type, entity_id: form.entity_id, entity_version: Number(form.version),
      timecode_start_ms: comment.timecode_start_ms === '' ? undefined : Number(comment.timecode_start_ms),
      timecode_end_ms: comment.timecode_end_ms === '' ? undefined : Number(comment.timecode_end_ms),
      body: comment.body.trim(), author: comment.author.trim() || undefined,
    })
    Object.assign(comment, { body: '', timecode_start_ms: '', timecode_end_ms: '' })
    await loadBindings()
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

function selectHistory(item) {
  plan.value = item
  form.entity_type = item.plan.target.entity_type
  form.entity_id = item.plan.target.entity_id
  form.version = item.plan.target.version
}

async function restoreVersion(item) {
  loading.value = true
  error.value = ''
  try {
    plan.value = await api.createVersionRestorePlan(projectId.value, item.entity_version_id, {
      mode: item.version < Math.max(...versions.value.map(version => version.version)) ? 'rollback' : 'reapply',
      requested_by: form.requested_by || undefined,
    })
    notice.value = `已为历史版本 v${item.version} 生成修改计划；确认前不会覆盖 current。`
    await loadHistory()
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="view-stack local-edit-view">
    <div class="hero-row">
      <div><h2>局部精修工作台</h2><p>先生成可审计的修改计划，确认后再原子执行；不会整集重做。</p></div>
      <RouterLink class="button button-secondary" :to="`/projects/${projectId}`">返回项目</RouterLink>
    </div>

    <div v-if="notice" class="success-banner"><CheckCircle2 :size="17" />{{ notice }}</div>
    <div v-if="error" class="error-banner">{{ error }}</div>

    <div class="local-edit-layout">
      <article class="panel local-edit-compose">
        <div class="panel-head"><div><span>NATURAL LANGUAGE ASSISTANT</span><h3>描述要修改的局部内容</h3></div><ShieldCheck :size="22" /></div>
        <form class="local-edit-form" @submit.prevent="generatePlan">
          <div class="local-target-grid">
            <label class="field"><span>目标类型</span><select v-model="form.entity_type"><option v-for="item in entityOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label>
            <label class="field"><span>实体 ID</span><input v-model="form.entity_id" required placeholder="dialogue / scene / shot / video ID" /></label>
            <label class="field"><span>目标版本</span><input v-model.number="form.version" type="number" min="1" required /></label>
          </div>
          <label class="field"><span>自然语言修改指令</span><textarea v-model="form.instruction" rows="5" required placeholder="例如：把第2场缩短20秒，但保留身份揭露。"></textarea></label>
          <label class="field"><span>操作人 <small>可选</small></span><input v-model="form.requested_by" placeholder="姓名或账号" /></label>
          <button class="button button-primary" :disabled="loading || !form.entity_id || !form.instruction.trim()">
            <LoaderCircle v-if="loading" :size="16" class="spin" /><ClipboardList v-else :size="16" />生成修改计划
          </button>
        </form>
      </article>

      <aside class="panel local-edit-history">
        <div class="panel-head"><div><span>CHANGE HISTORY</span><h3>修改历史</h3></div><button :disabled="loading" @click="loadHistory"><RefreshCw :size="15" /></button></div>
        <div v-if="!history.length" class="compact-empty">尚无修改计划</div>
        <button v-for="item in history" :key="item.change_plan_id" class="local-history-row" :class="{ active: item.change_plan_id === plan?.change_plan_id }" @click="selectHistory(item)">
          <History :size="16" /><span><strong>{{ item.plan.user_intent }}</strong><small>{{ item.plan.target.entity_type }} · v{{ item.plan.target.version }}</small></span><b>{{ changeStatusLabels[item.status] || item.status }}</b>
        </button>
      </aside>
    </div>

    <article v-if="plan" class="panel change-plan-preview">
      <div class="panel-head"><div><span>CHANGE PLAN · {{ plan.change_plan_id }}</span><h3>确认前预览</h3></div><strong class="plan-status">{{ changeStatusLabels[plan.status] || plan.status }}</strong></div>
      <div class="change-plan-grid">
        <section><h4>用户意图与保持项</h4><p>{{ plan.plan.user_intent }}</p><div class="plan-chips"><span v-for="item in plan.plan.must_preserve" :key="item"><ShieldCheck :size="13" />{{ item }}</span><span v-for="item in plan.plan.locks" :key="item">锁定 {{ item }}</span></div></section>
        <section><h4>目标与回滚</h4><dl><div><dt>目标</dt><dd>{{ plan.plan.target.entity_type }} / {{ plan.plan.target.entity_id }}</dd></div><div><dt>目标版本</dt><dd>v{{ plan.plan.target.version }}</dd></div><div><dt>回滚版本</dt><dd>v{{ plan.plan.rollback_version }}</dd></div><div><dt>语义变化</dt><dd>{{ plan.plan.semantic_change ? '是' : '否' }}</dd></div></dl></section>
      </div>

      <section class="plan-section">
        <h4><GitCompareArrows :size="16" />字段 diff</h4>
        <table><thead><tr><th>字段</th><th>操作</th><th>修改前</th><th>修改后</th></tr></thead><tbody><tr v-for="row in diffRows" :key="row.field"><td><code>{{ row.field }}</code></td><td>{{ row.operation }}</td><td>{{ row.before }}</td><td class="diff-after">{{ typeof row.after === 'object' ? JSON.stringify(row.after) : row.after }}</td></tr></tbody></table>
      </section>

      <div class="change-plan-grid">
        <section><h4>精确影响范围</h4><p v-if="!plan.impacts.length">artifact graph 暂无实体命中；以下计划范围仍会生成 pending 重建任务。</p><ol><li v-for="impact in plan.impacts" :key="impact.artifact_id"><code>{{ impact.artifact_type }}</code> {{ impact.native_entity_id }} <small>深度 {{ impact.propagation_depth }}</small></li><li v-for="item in plan.plan.impact.downstream" :key="`planned:${item}`"><code>{{ item }}</code> <small>计划范围</small></li></ol></section>
        <section><h4>将执行的重建</h4><div class="plan-chips"><span v-for="item in rebuilds" :key="item">{{ item }}</span><span v-if="!rebuilds.length">不触发重建</span></div><p v-for="risk in plan.plan.risks" :key="risk" class="plan-risk"><AlertTriangle :size="13" />{{ risk }}</p></section>
      </div>
      <div class="plan-validation"><ShieldCheck :size="18" /><div><strong>验证规则</strong><span v-for="rule in plan.plan.validation_rules" :key="rule">{{ rule }}</span></div></div>
      <footer class="plan-actions">
        <span v-if="canConfirm">此时正式业务数据仍未改变。</span>
        <span v-else-if="canExecute">已确认，等待执行。</span>
        <span v-else-if="plan.status === 'applied'">修改已应用；{{ plan.rebuild_tasks.length }} 个增量任务已排队，未执行前不会显示 succeeded。</span>
        <button v-if="canConfirm" class="button button-primary" :disabled="loading" @click="confirmPlan"><ShieldCheck :size="16" />确认计划</button>
        <button v-if="canExecute" class="button button-primary" :disabled="loading" @click="executePlan"><Play :size="16" />执行修改</button>
      </footer>
      <section v-if="plan.rebuild_tasks.length" class="plan-section">
        <h4>重建任务状态</h4>
        <ol><li v-for="task in plan.rebuild_tasks" :key="task.rebuild_task_id"><code>{{ task.action }}</code> · {{ task.target_entity_type }} / {{ task.target_entity_id }} · <strong>{{ task.status }}</strong><small v-if="task.range_start_ms != null"> · {{ task.range_start_ms }}–{{ task.range_end_ms }}ms</small></li></ol>
      </section>
    </article>

    <div class="local-edit-layout">
      <article class="panel local-version-panel">
        <div class="panel-head"><div><span>ROLLBACK SOURCES</span><h3>实体版本</h3></div><History :size="19" /></div>
        <div v-if="!versions.length" class="compact-empty">执行首次修改后将建立可回滚版本链</div>
        <div v-for="item in versions" :key="item.entity_version_id" class="version-record"><b>v{{ item.version }}</b><span>{{ item.source_type }}</span><code>{{ item.content_hash.slice(0, 12) }}</code><strong v-if="item.is_current">current</strong><button v-else :disabled="loading" @click="restoreVersion(item)">生成回滚/重应用计划</button></div>
      </article>
      <article class="panel local-comment-panel">
        <div class="panel-head"><div><span>BOUND COMMENTS</span><h3>实体 / 时间码评论</h3></div><MessageSquareText :size="19" /></div>
        <form @submit.prevent="addComment">
          <label class="field"><span>评论</span><textarea v-model="comment.body" rows="2" :disabled="!form.entity_id" placeholder="绑定到当前台词、镜头或视频时间段"></textarea></label>
          <div><input v-model="comment.timecode_start_ms" type="number" min="0" placeholder="开始 ms" /><ArrowRight :size="14" /><input v-model="comment.timecode_end_ms" type="number" min="1" placeholder="结束 ms" /><button class="button button-secondary" :disabled="loading || !comment.body.trim() || !form.entity_id">添加</button></div>
        </form>
        <div v-for="item in comments" :key="item.comment_id" class="comment-record"><MessageSquareText :size="14" /><span><strong>{{ item.body }}</strong><small v-if="item.timecode_start_ms != null">{{ item.timecode_start_ms }}–{{ item.timecode_end_ms }}ms</small></span></div>
      </article>
    </div>
  </section>
</template>
