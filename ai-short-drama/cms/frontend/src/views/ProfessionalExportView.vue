<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Archive, CheckCircle2, Download, FileArchive, FileCheck2, LockKeyhole, RefreshCw } from 'lucide-vue-next'
import { api } from '../services/api'

const route = useRoute()
const projectId = computed(() => route.params.projectId)
const options = ref(null)
const exports = ref([])
const error = ref('')
const notice = ref('')
const loading = ref(false)
const form = reactive({ episode_id: '', bundle_version: 1, formats: [], script_id: '', storyboard_id: '', timeline_id: '', master_id: '', story_bible_id: '', source_version_id: '', ir_revision_id: '', adaptation_spec_version_id: '', requested_by: 'cms-exporter' })

const formatGroups = [
  { title: '编剧与策划', items: [['script_docx', '剧本 DOCX'], ['script_fountain', '剧本 Fountain'], ['episode_outline', '分集大纲']] },
  { title: '分镜与提示词', items: [['shot_list', '镜头表'], ['contact_sheet', '联系表'], ['prompt_package', '图片 / 视频提示词包']] },
  { title: '后期交换', items: [['subtitle_srt', 'SRT 字幕'], ['subtitle_ass', 'ASS 字幕'], ['timeline_edl', 'EDL 时间线'], ['timeline_xml', 'XML 时间线'], ['audio_stems', '分轨音频 stems']] },
  { title: '制作与审计', items: [['production_bibles', '角色 / 服装 / 地点 / 道具圣经'], ['traceability_report', 'Source / IR / Spec / 人工修改溯源报告']] },
]
const needsScript = computed(() => form.formats.some((item) => ['script_docx', 'script_fountain'].includes(item)))
const needsStoryboard = computed(() => form.formats.some((item) => ['shot_list', 'contact_sheet', 'prompt_package'].includes(item)))
const needsTimeline = computed(() => form.formats.some((item) => ['subtitle_srt', 'subtitle_ass', 'timeline_edl', 'timeline_xml', 'audio_stems'].includes(item)))
const needsBible = computed(() => form.formats.includes('production_bibles'))
const needsTrace = computed(() => form.formats.includes('traceability_report'))
const selectedEpisode = computed(() => options.value?.episodes.find((item) => item.id === form.episode_id))

function allowed(item, type) {
  const valid = {
    episode: ['approved', 'completed', 'scripting'], script: ['approved', 'completed', 'storyboarding'],
    storyboard: ['approved', 'completed'], timeline: ['approved', 'restored'], master: ['ready'],
    bible: ['approved'], source: ['published', 'superseded'], ir: ['published', 'superseded'], spec: ['active', 'superseded'],
  }
  return valid[type]?.includes(item.status)
}
function firstAllowed(items, type) { return items?.find((item) => allowed(item, type))?.id || '' }

async function loadBase() {
  loading.value = true; error.value = ''
  try {
    options.value = await api.getProfessionalExportOptions(projectId.value, form.episode_id)
    if (!form.episode_id) form.episode_id = firstAllowed(options.value.episodes, 'episode')
    exports.value = await api.getProfessionalExports(projectId.value, form.episode_id)
    form.bundle_version = Math.max(1, ...exports.value.map((item) => item.bundle_version + 1))
  } catch (err) { error.value = err.message }
  finally { loading.value = false }
}

async function loadEpisodeOptions() {
  if (!form.episode_id) return
  loading.value = true; error.value = ''
  try {
    options.value = await api.getProfessionalExportOptions(projectId.value, form.episode_id)
    Object.assign(form, {
      script_id: firstAllowed(options.value.scripts, 'script'), storyboard_id: firstAllowed(options.value.storyboards, 'storyboard'),
      timeline_id: firstAllowed(options.value.timelines, 'timeline'), master_id: firstAllowed(options.value.masters, 'master'),
      story_bible_id: firstAllowed(options.value.story_bibles, 'bible'), source_version_id: firstAllowed(options.value.source_versions, 'source'),
      ir_revision_id: firstAllowed(options.value.ir_revisions, 'ir'), adaptation_spec_version_id: firstAllowed(options.value.adaptation_specs, 'spec'),
    })
    exports.value = await api.getProfessionalExports(projectId.value, form.episode_id)
    form.bundle_version = Math.max(1, ...exports.value.map((item) => item.bundle_version + 1))
  } catch (err) { error.value = err.message }
  finally { loading.value = false }
}

watch(() => form.episode_id, loadEpisodeOptions)

async function createExport() {
  loading.value = true; error.value = ''; notice.value = ''
  try {
    const selection = { episode_id: form.episode_id, bundle_version: Number(form.bundle_version) }
    for (const key of ['script_id', 'storyboard_id', 'timeline_id', 'master_id', 'story_bible_id', 'source_version_id', 'ir_revision_id', 'adaptation_spec_version_id']) if (form[key]) selection[key] = form[key]
    const created = await api.createProfessionalExport(projectId.value, { formats: form.formats, selection, requested_by: form.requested_by })
    notice.value = `导出包 v${created.bundle_version} 已冻结并生成；后续内容变化不会污染这个包。`
    exports.value = await api.getProfessionalExports(projectId.value, form.episode_id)
    form.bundle_version = Math.max(...exports.value.map((item) => item.bundle_version + 1))
  } catch (err) { error.value = err.message }
  finally { loading.value = false }
}

onMounted(loadBase)
</script>

<template>
  <section class="export-view view-stack">
    <div class="export-hero"><div><span>VERSION-LOCKED DELIVERY</span><h2>专业导出中心</h2><p>每个导出包只绑定一个作品、项目、单集与一组明确版本；禁止 current / draft 混用。</p></div><RouterLink class="button button-secondary" :to="`/projects/${projectId}`">返回项目</RouterLink></div>
    <div v-if="notice" class="success-banner"><CheckCircle2 :size="17" />{{ notice }}</div><div v-if="error" class="error-banner">{{ error }}</div>

    <form v-if="options" class="panel export-form" @submit.prevent="createExport">
      <div class="panel-head"><div><span>TARGET PATH</span><h3>作品 → 项目 → 集 → 精确版本</h3></div><LockKeyhole :size="21" /></div>
      <div class="target-chain"><label><span>作品</span><select><option>{{ options.work_title }}</option></select></label><label><span>项目</span><select><option>{{ options.project_name }}</option></select></label><label><span>单集版本</span><select v-model="form.episode_id" required><option value="">请选择已批准单集</option><option v-for="item in options.episodes" :key="item.id" :value="item.id" :disabled="!allowed(item, 'episode')">{{ item.label }} · v{{ item.version }} · {{ item.status }}</option></select></label><label><span>导出包版本</span><input v-model.number="form.bundle_version" type="number" min="1" required /></label></div>
      <p class="scope-hint"><FileCheck2 :size="15" />当前选择：{{ options.work_title }} / {{ options.project_name }} / {{ selectedEpisode?.label || '未选择单集' }} / 导出包 v{{ form.bundle_version }}</p>

      <section class="format-section"><h4>选择交付格式</h4><div class="format-groups"><article v-for="group in formatGroups" :key="group.title"><b>{{ group.title }}</b><label v-for="[key, label] in group.items" :key="key"><input v-model="form.formats" type="checkbox" :value="key" />{{ label }}</label></article></div></section>

      <section class="version-section"><h4>内容版本绑定</h4><div class="version-grid">
        <label v-if="needsScript"><span>剧本版本</span><select v-model="form.script_id" required><option v-for="item in options.scripts" :key="item.id" :value="item.id" :disabled="!allowed(item, 'script')">{{ item.label }} · v{{ item.version }} · {{ item.status }}</option></select></label>
        <label v-if="needsStoryboard"><span>分镜版本</span><select v-model="form.storyboard_id" required><option v-for="item in options.storyboards" :key="item.id" :value="item.id" :disabled="!allowed(item, 'storyboard')">{{ item.label }} · {{ item.status }}</option></select></label>
        <label v-if="needsTimeline"><span>剪辑时间线版本</span><select v-model="form.timeline_id" required><option v-for="item in options.timelines" :key="item.id" :value="item.id" :disabled="!allowed(item, 'timeline')">{{ item.label }} · {{ item.status }}</option></select></label>
        <label v-if="needsBible"><span>制作圣经版本</span><select v-model="form.story_bible_id" required><option v-for="item in options.story_bibles" :key="item.id" :value="item.id" :disabled="!allowed(item, 'bible')">{{ item.label }} · {{ item.status }}</option></select></label>
        <template v-if="needsTrace"><label><span>Source 版本</span><select v-model="form.source_version_id" required><option v-for="item in options.source_versions" :key="item.id" :value="item.id" :disabled="!allowed(item, 'source')">{{ item.label }} · {{ item.status }}</option></select></label><label><span>IR 版本</span><select v-model="form.ir_revision_id" required><option v-for="item in options.ir_revisions" :key="item.id" :value="item.id" :disabled="!allowed(item, 'ir')">{{ item.label }} · {{ item.status }}</option></select></label><label><span>Spec 版本</span><select v-model="form.adaptation_spec_version_id" required><option v-for="item in options.adaptation_specs" :key="item.id" :value="item.id" :disabled="!allowed(item, 'spec')">{{ item.label }} · {{ item.status }}</option></select></label></template>
      </div><div class="guard-note"><LockKeyhole :size="16" /><span>列表中的草稿会显示但不可选择。数据库还会复核项目归属、单集归属与批准状态。</span></div></section>
      <label class="requester"><span>导出操作人</span><input v-model="form.requested_by" /></label>
      <button class="button button-primary" :disabled="loading || !form.formats.length || !form.episode_id"><FileArchive :size="16" />{{ loading ? '正在构建…' : '冻结快照并生成导出包' }}</button>
    </form>

    <article class="panel export-history"><div class="panel-head"><div><span>DELIVERY HISTORY</span><h3>导出历史</h3></div><button :disabled="loading" @click="loadEpisodeOptions"><RefreshCw :size="15" /></button></div><div v-if="!exports.length" class="compact-empty">当前单集还没有导出包。</div><div v-for="item in exports" :key="item.export_id" class="export-row"><div class="export-icon"><Archive :size="19" /></div><div><strong>导出包 v{{ item.bundle_version }}</strong><span>{{ item.formats.length }} 种格式 · {{ new Date(item.created_at).toLocaleString('zh-CN') }}</span><small>状态：{{ item.status }}</small></div><a v-if="item.status === 'ready'" class="button button-secondary" :href="api.professionalExportDownloadUrl(projectId, item.export_id)"><Download :size="15" />下载 ZIP</a><details><summary>高级信息</summary><dl><div><dt>Export ID</dt><dd><code>{{ item.export_id }}</code></dd></div><div><dt>Episode ID</dt><dd><code>{{ item.episode_id }}</code></dd></div><div><dt>Selection hash</dt><dd><code>{{ item.selection_hash }}</code></dd></div><div><dt>Package hash</dt><dd><code>{{ item.package_hash }}</code></dd></div></dl></details></div></article>
  </section>
</template>

<style scoped>
.export-view{gap:18px}.export-hero,.panel-head,.export-row{display:flex;justify-content:space-between;align-items:center;gap:14px}.export-hero{padding:22px;border-radius:14px;background:linear-gradient(120deg,#182845,#385682);color:#fff}.export-hero span,.panel-head span{font-size:11px;letter-spacing:.12em}.panel{background:#fff;border:1px solid #dce2ea;border-radius:13px;padding:18px}.export-form{display:grid;gap:18px}.target-chain,.version-grid{display:grid;grid-template-columns:repeat(4,minmax(170px,1fr));gap:10px}.target-chain label,.version-grid label,.requester{display:grid;gap:5px}.target-chain select,.target-chain input,.version-grid select,.requester input{padding:9px;border:1px solid #ccd5e0;border-radius:7px}.scope-hint,.guard-note{display:flex;align-items:center;gap:7px;background:#edf4ff;color:#294f8a;padding:10px;border-radius:8px}.format-section,.version-section{border-top:1px solid #e3e8ef;padding-top:14px}.format-groups{display:grid;grid-template-columns:repeat(4,1fr);gap:10px}.format-groups article{background:#f7f9fc;border-radius:9px;padding:11px;display:grid;gap:8px}.format-groups label{display:flex;gap:6px}.export-history{display:grid;gap:8px}.export-row{padding:12px;border-top:1px solid #e3e8ef;display:grid;grid-template-columns:auto 1fr auto minmax(180px,auto)}.export-row>div:nth-child(2){display:grid}.export-row span,.export-row small{color:#687487}.export-icon{width:38px;height:38px;border-radius:9px;background:#edf3ff;display:grid;place-items:center;color:#315da9}.export-row details dl{font-size:11px}.export-row details div{display:grid;grid-template-columns:90px 1fr}.export-row code{overflow-wrap:anywhere}@media(max-width:900px){.target-chain,.version-grid,.format-groups{grid-template-columns:1fr 1fr}.export-row{grid-template-columns:auto 1fr}.export-row details{grid-column:1/-1}}@media(max-width:600px){.target-chain,.version-grid,.format-groups{grid-template-columns:1fr}}
</style>
