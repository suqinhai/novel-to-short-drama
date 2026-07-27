<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowLeft, BookOpenText, Clapperboard, Sparkles, Send, ShieldCheck, LoaderCircle } from 'lucide-vue-next'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'

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
    const productionIntent = {
      production_mode: 'rolling_episode',
      target_episode_count: Number(form.target_episode_count),
      episode_duration_seconds: Number(form.episode_duration_seconds),
      visual_style: form.visual_style.trim(),
      aspect_ratio: form.aspect_ratio,
      target_platform: form.target_platform,
      test_mode: Boolean(form.test_mode),
    }
    const workResponse = await narrativeApi.createWork({
      title: form.novel_name.trim(),
      author: null,
      metadata: productionIntent,
    }, createIdempotencyKey('rolling-source-work'))
    const versionResponse = await narrativeApi.createVersion(workResponse.data.work_id, {
      parent_source_version_id: null,
      normalization_version: 'unicode-nfc-v1',
      metadata: productionIntent,
    }, createIdempotencyKey('rolling-source-version'))
    const versionId = versionResponse.data.source_version_id
    await narrativeApi.getVersion(versionId)
    const importResponse = await narrativeApi.startImport(versionId, {
      mode: 'whole_book',
      text: form.novel_text.trim(),
    }, createIdempotencyKey('rolling-whole-book'))
    await router.push({
      name: 'source-version-detail',
      params: { versionId },
      query: { operation_id: importResponse.data.operation_id, rolling: '1' },
    })
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
    <div class="hero-row"><div><h2>导入小说，准备滚动生产</h2><p>整本小说只负责存档和拆章，不再自动启动 120 章分析或整季视频生成。</p></div></div>

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

        <div class="webhook-notice"><ShieldCheck :size="19" /><div><strong>安全导入模式</strong><span>导入后先选择 5–30 章故事弧，再逐集生产</span></div></div>
        <div v-if="error" class="error-banner large create-error">{{ error }}</div>
        <button class="button button-primary submit-project" type="submit" :disabled="!canSubmit">
          <LoaderCircle v-if="submitting" :size="17" class="spin" /><Send v-else :size="17" />
          {{ submitting ? '正在导入并拆章…' : '仅导入小说并拆章' }}
        </button>
        <p class="submit-hint"><Sparkles :size="13" />此操作不会调用文本分析模型，也不会生成图片或视频。</p>
      </aside>
    </form>
  </section>
</template>
