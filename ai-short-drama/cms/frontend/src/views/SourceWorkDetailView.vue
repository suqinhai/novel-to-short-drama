<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, ArrowUpRight, GitBranch, Plus, RefreshCw } from 'lucide-vue-next'
import StatusBadge from '../components/StatusBadge.vue'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'

const route = useRoute()
const router = useRouter()
const work = ref(null)
const versions = ref([])
const loading = ref(true)
const creating = ref(false)
const error = ref('')
const form = reactive({ normalization_version: 'unicode-nfc-v1', parent_source_version_id: '' })
let pendingCreate = null
const publishedVersions = computed(() => versions.value.filter((item) => item.status === 'published'))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [workResponse, versionsResponse] = await Promise.all([
      narrativeApi.getWork(route.params.workId), narrativeApi.listVersions(route.params.workId),
    ])
    work.value = workResponse.data
    versions.value = versionsResponse.data
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function createVersion() {
  creating.value = true
  error.value = ''
  try {
    const payload = {
      parent_source_version_id: form.parent_source_version_id || null,
      normalization_version: form.normalization_version.trim(),
      metadata: {},
    }
    const signature = JSON.stringify(payload)
    if (pendingCreate?.signature !== signature) pendingCreate = { signature, key: createIdempotencyKey('source-version') }
    const response = await narrativeApi.createVersion(route.params.workId, payload, pendingCreate.key)
    pendingCreate = null
    await router.push(`/library/versions/${response.data.source_version_id}`)
  } catch (err) {
    error.value = err.message
  } finally {
    creating.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="view-stack library-view">
    <RouterLink to="/library" class="back-link"><ArrowLeft :size="16" />返回原著资料库</RouterLink>
    <div v-if="error" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <div v-if="loading" class="detail-skeleton"><span></span><span></span><span></span></div>
    <template v-else-if="work">
      <div class="detail-hero">
        <div class="detail-title"><div class="source-work-cover large"><GitBranch :size="25" /></div><div><div class="title-line"><h2>{{ work.title }}</h2><StatusBadge :status="work.status" /></div><p>{{ work.author || '作者未填写' }} · {{ work.work_id }}</p></div></div>
        <button class="button button-secondary" @click="load"><RefreshCw :size="16" />刷新</button>
      </div>

      <div class="library-split">
        <article class="panel version-list-panel">
          <div class="production-data-head"><div><span>SOURCE VERSIONS</span><h3>作品版本</h3></div><p>{{ versions.length }} 个版本</p></div>
          <div v-if="versions.length" class="version-list">
            <RouterLink v-for="version in versions" :key="version.source_version_id" :to="`/library/versions/${version.source_version_id}`" class="version-row">
              <b>v{{ version.version_number }}</b><div><strong>{{ version.status }}</strong><code>{{ version.source_version_id }}</code><small>{{ version.chapter_count }} 章 · {{ Number(version.total_chars).toLocaleString('zh-CN') }} 字符</small></div><StatusBadge :status="version.status" /><ArrowUpRight :size="17" />
            </RouterLink>
          </div>
          <div v-else class="compact-empty">还没有版本，请在右侧创建第一个草稿版本。</div>
        </article>

        <form class="panel padded version-create-panel" @submit.prevent="createVersion">
          <div class="section-title"><div><span>NEW DRAFT</span><h3>创建草稿版本</h3></div><Plus :size="18" /></div>
          <label class="field"><span>规范化版本 <i>*</i></span><input v-model="form.normalization_version" maxlength="200" required /><small>用于保证导入文本哈希可重复。</small></label>
          <label class="field"><span>继承发布版本</span><select v-model="form.parent_source_version_id"><option value="">不继承，创建空版本</option><option v-for="item in publishedVersions" :key="item.source_version_id" :value="item.source_version_id">v{{ item.version_number }} · {{ item.source_version_id }}</option></select><small>仅列出已发布版本，作为新修订的父快照。</small></label>
          <button class="button button-primary full-button" :disabled="creating || !form.normalization_version.trim()">{{ creating ? '创建中…' : '创建并管理章节' }}</button>
        </form>
      </div>
    </template>
  </section>
</template>
