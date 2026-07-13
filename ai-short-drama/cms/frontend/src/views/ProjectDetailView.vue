<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, RefreshCw, BookOpen, Clapperboard, Image, Video, ListChecks, Layers3, GitBranch, ClipboardCheck, FileText, BookMarked, ListVideo, ScrollText, PanelsTopLeft } from 'lucide-vue-next'
import { api } from '../services/api'
import StatusBadge from '../components/StatusBadge.vue'
import DetailDataTable from '../components/DetailDataTable.vue'

const route = useRoute()
const project = ref(null)
const loading = ref(true)
const error = ref('')
const activeDataTab = ref('workflow_tasks')
const stages = [
  ['novel_import', '小说导入'], ['chunk_analysis', '文本拆解'], ['story_bible', '故事圣经'],
  ['episode_planning', '分集策划'], ['episode_script', '单集剧本'], ['storyboard', '分镜设计'],
  ['visual_assets', '视觉资产'], ['storyboard_images', '分镜图片'], ['shot_video', '镜头视频'],
  ['voice_audio', '语音音频'], ['edit_compose', '剪辑合成'], ['qc_review_publish', '质检发布'],
]
const currentIndex = computed(() => {
  if (!project.value) return -1
  const exact = stages.findIndex(([key]) => project.value.current_stage.includes(key))
  if (project.value.status === 'completed') return stages.length
  return exact
})

const formatShortDate = (value) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'
const formatDuration = (value) => `${value || 0} 秒`
const productionTabs = computed(() => {
  if (!project.value) return []
  return [
    { key: 'workflow_tasks', label: '工作流任务', icon: GitBranch, items: project.value.workflow_tasks, columns: [
      { key: 'task_id', label: 'Task ID', type: 'id' }, { key: 'workflow_stage', label: '阶段' }, { key: 'action', label: '动作' },
      { key: 'status', label: '状态', type: 'status' }, { key: 'generation_version', label: '版本', format: (v) => `v${v}` },
      { key: 'error_message', label: '错误信息', class: 'wide-cell' }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
    { key: 'review_tasks', label: '审核任务', icon: ClipboardCheck, items: project.value.review_tasks, columns: [
      { key: 'review_id', label: 'Review ID', type: 'id' }, { key: 'stage', label: '阶段' }, { key: 'entity_type', label: '对象类型' },
      { key: 'review_status', label: '审核状态', type: 'status' }, { key: 'review_comment', label: '审核意见', class: 'wide-cell' },
      { key: 'created_at', label: '创建时间', format: formatShortDate }, { key: 'reviewed_at', label: '审核时间', format: formatShortDate },
    ] },
    { key: 'novels', label: '小说', icon: FileText, items: project.value.novels, columns: [
      { key: 'novel_id', label: 'Novel ID', type: 'id' }, { key: 'name', label: '小说名' }, { key: 'source_type', label: '来源' },
      { key: 'encoding', label: '编码' }, { key: 'total_chars', label: '总字数', format: (v) => Number(v).toLocaleString('zh-CN') },
      { key: 'chapter_count', label: '章节数' }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
    { key: 'story_bibles', label: '故事圣经', icon: BookMarked, items: project.value.story_bibles, columns: [
      { key: 'story_bible_id', label: 'Story Bible ID', type: 'id' }, { key: 'version', label: '版本', format: (v) => `v${v}` },
      { key: 'status', label: '状态', type: 'status' }, { key: 'character_count', label: '角色' }, { key: 'location_count', label: '地点' },
      { key: 'key_event_count', label: '关键事件' }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
    { key: 'episodes', label: '分集', icon: ListVideo, items: project.value.episodes, columns: [
      { key: 'episode_number', label: '集数', format: (v) => `第 ${v} 集` }, { key: 'episode_id', label: 'Episode ID', type: 'id' },
      { key: 'title', label: '标题' }, { key: 'status', label: '状态', type: 'status' }, { key: 'version', label: '版本', format: (v) => `v${v}` },
      { key: 'estimated_duration_seconds', label: '预计时长', format: formatDuration }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
    { key: 'scripts', label: '剧本', icon: ScrollText, items: project.value.scripts, columns: [
      { key: 'script_id', label: 'Script ID', type: 'id' }, { key: 'episode_id', label: 'Episode ID', type: 'id' }, { key: 'title', label: '标题' },
      { key: 'status', label: '状态', type: 'status' }, { key: 'version', label: '版本', format: (v) => `v${v}` }, { key: 'scene_count', label: '场景数' },
      { key: 'dialogue_char_count', label: '对白字数' }, { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
    { key: 'storyboards', label: '分镜', icon: PanelsTopLeft, items: project.value.storyboards, columns: [
      { key: 'storyboard_id', label: 'Storyboard ID', type: 'id' }, { key: 'episode_id', label: 'Episode ID', type: 'id' },
      { key: 'status', label: '状态', type: 'status' }, { key: 'version', label: '版本', format: (v) => `v${v}` },
      { key: 'total_shots', label: '镜头数' }, { key: 'estimated_duration_seconds', label: '预计时长', format: formatDuration },
      { key: 'updated_at', label: '更新时间', format: formatShortDate },
    ] },
  ]
})
const activeTab = computed(() => productionTabs.value.find((tab) => tab.key === activeDataTab.value) || productionTabs.value[0])

async function load() {
  loading.value = true
  error.value = ''
  try { project.value = await api.getProject(route.params.projectId) }
  catch (err) { error.value = err.message }
  finally { loading.value = false }
}
onMounted(load)

const formatDate = (value) => new Intl.DateTimeFormat('zh-CN', { dateStyle: 'long', timeStyle: 'short' }).format(new Date(value))
</script>

<template>
  <section class="view-stack">
    <RouterLink to="/projects" class="back-link"><ArrowLeft :size="16" />返回项目列表</RouterLink>
    <div v-if="loading" class="detail-skeleton"><span></span><span></span><span></span></div>
    <div v-else-if="error" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <template v-else-if="project">
      <div class="detail-hero">
        <div class="detail-title"><div class="project-cover large">{{ project.novel_name.slice(0, 1) }}</div><div><div class="title-line"><h2>{{ project.novel_name }}</h2><StatusBadge :status="project.status" /></div><p>{{ project.project_id }} · 创建于 {{ formatDate(project.created_at) }}</p></div></div>
        <button class="button button-secondary" @click="load"><RefreshCw :size="16" />刷新详情</button>
      </div>

      <div class="detail-grid">
        <div class="main-column">
          <article class="panel padded">
            <div class="section-title"><div><span>PRODUCTION PIPELINE</span><h3>生产流程</h3></div><strong>{{ project.current_stage.replaceAll('_', ' ') }}</strong></div>
            <div class="pipeline">
              <div v-for="(stage, index) in stages" :key="stage[0]" class="pipeline-step" :class="{ done: index < currentIndex, current: index === currentIndex }">
                <i>{{ index < currentIndex ? '✓' : index + 1 }}</i><span>{{ stage[1] }}</span>
              </div>
            </div>
          </article>

          <article class="panel padded">
            <div class="section-title"><div><span>CONTENT INVENTORY</span><h3>内容资产</h3></div></div>
            <div class="asset-grid">
              <div><BookOpen :size="20" /><span>原文章节</span><strong>{{ project.counts.chapters }}</strong></div>
              <div><Layers3 :size="20" /><span>文本分块</span><strong>{{ project.counts.chunks }}</strong></div>
              <div><Clapperboard :size="20" /><span>剧集 / 场景</span><strong>{{ project.counts.episodes }} / {{ project.counts.scenes }}</strong></div>
              <div><ListChecks :size="20" /><span>分镜镜头</span><strong>{{ project.counts.shots }}</strong></div>
              <div><Image :size="20" /><span>分镜图片</span><strong>{{ project.counts.generated_images }}</strong></div>
              <div><Video :size="20" /><span>镜头视频</span><strong>{{ project.counts.generated_videos }}</strong></div>
            </div>
          </article>
        </div>

        <aside class="side-column">
          <article class="panel padded metadata-card">
            <div class="section-title"><div><span>PROJECT PROFILE</span><h3>项目参数</h3></div></div>
            <dl><div><dt>视觉风格</dt><dd>{{ project.visual_style }}</dd></div><div><dt>画面比例</dt><dd>{{ project.aspect_ratio }}</dd></div><div><dt>目标平台</dt><dd>{{ project.target_platform }}</dd></div><div><dt>单集时长</dt><dd>{{ project.episode_duration_seconds }} 秒</dd></div><div><dt>目标集数</dt><dd>{{ project.target_episode_count }} 集</dd></div><div><dt>运行模式</dt><dd>{{ project.test_mode ? '测试模式' : '正式模式' }}</dd></div></dl>
          </article>
          <article class="attention-card"><span>待办概览</span><strong>{{ project.counts.pending_reviews }}</strong><p>项内容等待人工审核</p><div><b>{{ project.counts.completed_tasks }}</b> 个工作流任务已完成</div></article>
        </aside>
      </div>

      <article class="panel production-data-panel">
        <div class="production-data-head">
          <div><span>READ-ONLY DATABASE VIEW</span><h3>项目生产数据</h3></div>
          <p>数据直接读取自 <code>drama</code> schema</p>
        </div>
        <div class="data-tabs">
          <button v-for="tab in productionTabs" :key="tab.key" :class="{ active: activeDataTab === tab.key }" @click="activeDataTab = tab.key">
            <component :is="tab.icon" :size="15" />{{ tab.label }}<i>{{ tab.items?.length || 0 }}</i>
          </button>
        </div>
        <DetailDataTable v-if="activeTab" :items="activeTab.items || []" :columns="activeTab.columns" />
      </article>
    </template>
  </section>
</template>
