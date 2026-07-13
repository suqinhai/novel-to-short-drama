<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, BookOpenText, Clapperboard, Sparkles, Send, ShieldCheck, LoaderCircle } from 'lucide-vue-next'
import { api } from '../services/api'

const router = useRouter()
const submitting = ref(false)
const error = ref('')
const form = reactive({
  novel_text: '',
  novel_name: '',
  target_episode_count: 12,
  episode_duration_seconds: 90,
  visual_style: '东方悬疑写实',
  aspect_ratio: '9:16',
  target_platform: '抖音',
  test_mode: true,
})

const characterCount = computed(() => form.novel_text.length.toLocaleString('zh-CN'))
const canSubmit = computed(() => !submitting.value && form.novel_text.trim() && form.novel_name.trim() && form.visual_style.trim() && form.target_episode_count > 0 && form.episode_duration_seconds > 0)

async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  error.value = ''
  try {
    const result = await api.createProject({
      ...form,
      novel_text: form.novel_text.trim(),
      novel_name: form.novel_name.trim(),
      visual_style: form.visual_style.trim(),
      target_episode_count: Number(form.target_episode_count),
      episode_duration_seconds: Number(form.episode_duration_seconds),
    })
    try {
      sessionStorage.setItem(`cms:create-result:${result.project_id}`, JSON.stringify(result.n8n_response))
    } catch { /* 浏览器禁用 sessionStorage 时仍可正常跳转 */ }
    await router.push({ name: 'project-detail', params: { projectId: result.project_id }, query: { created: '1' } })
  } catch (err) {
    error.value = err.message
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section class="view-stack new-project-view">
    <RouterLink to="/projects" class="back-link"><ArrowLeft :size="16" />返回项目列表</RouterLink>
    <div class="hero-row"><div><h2>创建短剧项目</h2><p>粘贴小说正文并配置生产规格，提交后由 n8n 启动现有工作流。</p></div></div>

    <form class="create-layout" @submit.prevent="submit">
      <div class="create-main">
        <article class="panel padded form-section">
          <div class="section-title"><div><span>SOURCE MATERIAL</span><h3>小说内容</h3></div><div class="section-icon"><BookOpenText :size="19" /></div></div>
          <label class="field"><span>小说名 <i>*</i></span><input v-model="form.novel_name" maxlength="200" placeholder="例如：雨夜归人" required /></label>
          <label class="field"><span>小说正文 <i>*</i></span><textarea v-model="form.novel_text" rows="18" placeholder="在这里粘贴完整小说正文……" required></textarea><small>{{ characterCount }} 字符 · 最大请求正文 20 MB</small></label>
        </article>
      </div>

      <aside class="create-side">
        <article class="panel padded form-section">
          <div class="section-title"><div><span>PRODUCTION PROFILE</span><h3>生产参数</h3></div><div class="section-icon"><Clapperboard :size="19" /></div></div>
          <div class="field-pair">
            <label class="field"><span>目标集数 <i>*</i></span><input v-model.number="form.target_episode_count" type="number" min="1" max="1000" required /></label>
            <label class="field"><span>单集时长（秒） <i>*</i></span><input v-model.number="form.episode_duration_seconds" type="number" min="1" max="7200" required /></label>
          </div>
          <label class="field"><span>视觉风格 <i>*</i></span><input v-model="form.visual_style" list="visual-styles" maxlength="200" required /><datalist id="visual-styles"><option value="东方悬疑写实"/><option value="都市电影感"/><option value="古风唯美"/><option value="国漫风格"/></datalist></label>
          <div class="field-pair">
            <label class="field"><span>画幅</span><select v-model="form.aspect_ratio"><option value="9:16">9:16 竖屏</option><option value="16:9">16:9 横屏</option><option value="1:1">1:1 方形</option><option value="4:3">4:3</option></select></label>
            <label class="field"><span>目标平台</span><select v-model="form.target_platform"><option value="抖音">抖音</option><option value="快手">快手</option><option value="视频号">视频号</option><option value="B站">B站</option><option value="小红书">小红书</option></select></label>
          </div>
          <label class="switch-field"><div><span>测试模式</span><small>使用当前工作流的测试范围与 Mock 配置</small></div><input v-model="form.test_mode" type="checkbox" /><i></i></label>
        </article>

        <div class="webhook-notice"><ShieldCheck :size="19" /><div><strong>通过 n8n 创建</strong><span>CMS 不直接写入 PostgreSQL</span></div></div>
        <div v-if="error" class="error-banner large create-error">{{ error }}</div>
        <button class="button button-primary submit-project" type="submit" :disabled="!canSubmit">
          <LoaderCircle v-if="submitting" :size="17" class="spin" /><Send v-else :size="17" />
          {{ submitting ? '正在提交并执行工作流…' : '提交并创建项目' }}
        </button>
        <p class="submit-hint"><Sparkles :size="13" />n8n 完成当前同步流程后页面会自动跳转。</p>
      </aside>
    </form>
  </section>
</template>
