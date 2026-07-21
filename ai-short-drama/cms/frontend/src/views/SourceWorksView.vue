<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useDebounceFn } from '@vueuse/core'
import { ArrowUpRight, BookOpen, Plus, RefreshCw, Search, X } from 'lucide-vue-next'
import EmptyState from '../components/EmptyState.vue'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'

const works = ref([])
const page = reactive({ number: 1, limit: 50, total: 0 })
const search = ref('')
const loading = ref(true)
const error = ref('')
const creating = ref(false)
const showCreate = ref(false)
const form = reactive({ title: '', author: '' })
let pendingCreate = null
const totalPages = computed(() => Math.max(1, Math.ceil(page.total / page.limit)))

async function load(reset = false) {
  if (reset) page.number = 1
  loading.value = true
  error.value = ''
  try {
    const response = await narrativeApi.listWorks({ page: page.number, limit: page.limit, q: search.value.trim() })
    works.value = response.data
    Object.assign(page, response.page)
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function createWork() {
  if (!form.title.trim()) return
  creating.value = true
  error.value = ''
  try {
    const payload = {
      title: form.title.trim(),
      author: form.author.trim() || null,
      metadata: {},
    }
    const signature = JSON.stringify(payload)
    if (pendingCreate?.signature !== signature) pendingCreate = { signature, key: createIdempotencyKey('source-work') }
    await narrativeApi.createWork(payload, pendingCreate.key)
    pendingCreate = null
    Object.assign(form, { title: '', author: '' })
    showCreate.value = false
    await load(true)
  } catch (err) {
    error.value = err.message
  } finally {
    creating.value = false
  }
}

watch(search, useDebounceFn(() => load(true), 260))
onMounted(load)
</script>

<template>
  <section class="view-stack library-view">
    <div class="hero-row">
      <div><h2>原著资料库</h2><p>原著独立于制作项目管理，一部作品可拥有多个不可变发布版本。</p></div>
      <div class="hero-actions">
        <button class="button button-secondary" :disabled="loading" @click="load()"><RefreshCw :size="16" :class="{ spin: loading }" />刷新</button>
        <button class="button button-primary" @click="showCreate = true"><Plus :size="16" />新增作品</button>
      </div>
    </div>

    <form v-if="showCreate" class="panel padded inline-create" @submit.prevent="createWork">
      <div class="section-title"><div><span>SOURCE WORK</span><h3>创建原著作品</h3></div><button type="button" class="icon-button" @click="showCreate = false"><X :size="17" /></button></div>
      <div class="field-pair">
        <label class="field"><span>作品名 <i>*</i></span><input v-model="form.title" maxlength="1000" required /></label>
        <label class="field"><span>作者</span><input v-model="form.author" maxlength="500" /></label>
      </div>
      <div class="form-actions"><button class="button button-secondary" type="button" @click="showCreate = false">取消</button><button class="button button-primary" :disabled="creating || !form.title.trim()">{{ creating ? '创建中…' : '创建作品' }}</button></div>
    </form>

    <div class="panel">
      <div class="panel-toolbar"><div class="search-box"><Search :size="17" /><input v-model="search" placeholder="搜索作品名" /></div><span class="result-count">{{ page.total }} 部作品</span></div>
      <div v-if="error" class="error-banner">{{ error }} <button @click="load()">重试</button></div>
      <div v-if="loading" class="table-loading"><span v-for="i in 4" :key="i"></span></div>
      <EmptyState v-else-if="works.length === 0" title="资料库还是空的" description="先创建原著作品，再为作品创建和导入版本。" />
      <div v-else class="source-work-grid">
        <RouterLink v-for="work in works" :key="work.work_id" :to="`/library/${work.work_id}`" class="source-work-card">
          <span class="source-work-cover"><BookOpen :size="23" /></span>
          <div><strong>{{ work.title }}</strong><p>{{ work.author || '作者未填写' }}</p><code>{{ work.work_id }}</code></div>
          <div class="source-work-state"><i>{{ work.status }}</i><small>revision {{ work.resource_revision }}</small><ArrowUpRight :size="17" /></div>
        </RouterLink>
      </div>
      <div v-if="totalPages > 1" class="pager"><button :disabled="page.number <= 1" @click="page.number--; load()">上一页</button><span>{{ page.number }} / {{ totalPages }}</span><button :disabled="page.number >= totalPages" @click="page.number++; load()">下一页</button></div>
    </div>
  </section>
</template>
