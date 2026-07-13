<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ArrowLeft, RefreshCw, BookOpen, Clapperboard, Image, Video, ListChecks, Layers3 } from 'lucide-vue-next'
import { api } from '../services/api'
import StatusBadge from '../components/StatusBadge.vue'

const route = useRoute()
const project = ref(null)
const loading = ref(true)
const error = ref('')
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
    </template>
  </section>
</template>
