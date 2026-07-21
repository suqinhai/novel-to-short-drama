<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { AlertTriangle, ArrowLeft, BookPlus, BrainCircuit, CheckCircle2, FileStack, FlaskConical, History, PencilLine, RefreshCw, Send, Upload } from 'lucide-vue-next'
import OperationTracker from '../components/OperationTracker.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { createIdempotencyKey, narrativeApi } from '../services/narrativeApi'

const route = useRoute()
const version = ref(null)
const work = ref(null)
const chapters = ref([])
const irRevisions = ref([])
const chapterRevisions = ref([])
const loading = ref(true)
const submitting = ref(false)
const error = ref('')
const notice = ref('')
const activeMode = ref('whole')
const operation = ref(null)
const historyChapterId = ref('')
const historyLoading = ref(false)
const historyError = ref('')
const irTestAcknowledged = ref(false)
const irRun = reactive({ extractor_version: 'cms-manual-test-v1', chapter_ids: [] })
const commandKeys = new Map()
const wholeText = ref('')
const single = reactive({ title: '', content: '', ordinal: 1 })
const batchText = ref('')
const revision = reactive({ chapter_id: '', title: '', content: '' })
const isDraft = computed(() => version.value?.status === 'draft')
const nextOrdinal = computed(() => Math.max(0, ...chapters.value.map((item) => item.ordinal || 0)) + 1)
const etag = computed(() => narrativeApi.getCachedVersionETag(route.params.versionId) || '—')
const batchPreview = computed(() => parseBatch(batchText.value))

function parseBatch(text) {
  const items = []
  let current = null
  for (const line of text.replaceAll('\r\n', '\n').split('\n')) {
    const heading = line.match(/^#{1,6}\s+(.+?)\s*$/)
    if (heading) {
      if (current?.content.trim()) items.push(current)
      current = { title: heading[1].trim(), content: '' }
    } else if (current) {
      current.content += `${current.content ? '\n' : ''}${line}`
    }
  }
  if (current?.content.trim()) items.push(current)
  return items
}

function getCommandKey(kind, payload) {
  const signature = `${kind}:${JSON.stringify(payload)}`
  if (!commandKeys.has(signature)) commandKeys.set(signature, createIdempotencyKey(kind))
  return { signature, value: commandKeys.get(signature) }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [versionResponse, chapterResponse, irResponse] = await Promise.all([
      narrativeApi.getVersion(route.params.versionId), narrativeApi.listChapters(route.params.versionId), narrativeApi.listIRRevisions(route.params.versionId),
    ])
    version.value = versionResponse.data
    chapters.value = chapterResponse.data
    irRevisions.value = irResponse.data
    single.ordinal = nextOrdinal.value
    if (!work.value || work.value.work_id !== version.value.work_id) {
      work.value = (await narrativeApi.getWork(version.value.work_id)).data
    }
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function execute(kind, payload, callback, existingKey = null) {
  submitting.value = true
  error.value = ''
  notice.value = ''
  const key = existingKey || getCommandKey(kind, payload)
  try {
    const response = await callback(key.value)
    commandKeys.delete(key.signature)
    operation.value = response.data
    notice.value = `命令已接受，ETag 已更新为 ${narrativeApi.getCachedVersionETag(route.params.versionId) || '服务端返回值'}。`
  } catch (err) {
    error.value = err.isConflict ? `${err.message} 当前版本已变化，请刷新后检查再重试。` : err.message
  } finally {
    submitting.value = false
  }
}

async function submitWhole() {
  const payload = { mode: 'whole_book', text: wholeText.value.trim() }
  await execute('whole-book', payload, (key) => narrativeApi.startImport(route.params.versionId, payload, key))
}

async function submitSingle() {
  const chapter = { ordinal: Number(single.ordinal), title: single.title.trim(), content: single.content.trim() }
  const key = getCommandKey('single-chapter', chapter)
  const payload = { client_item_key: key.value, ...chapter }
  await execute('single-chapter', payload, (value) => narrativeApi.addChapter(route.params.versionId, payload, value), key)
}

async function submitBatch() {
  const sourceItems = batchPreview.value.map((item, index) => ({ ordinal: nextOrdinal.value + index,
    title: item.title, content: item.content.trim(),
  }))
  const key = getCommandKey('batch-chapters', sourceItems)
  const items = sourceItems.map((item, index) => ({ client_item_key: `${key.value}:${index + 1}`, ...item }))
  const payload = { items }
  await execute('batch-chapters', payload, (value) => narrativeApi.addChaptersBatch(route.params.versionId, payload, value), key)
}

async function submitRevision() {
  const payload = { title: revision.title.trim(), content: revision.content.trim() }
  await execute('chapter-revision', { chapter_id: revision.chapter_id, ...payload }, (key) => narrativeApi.reviseChapter(route.params.versionId, revision.chapter_id, payload, key))
}

async function publishVersion() {
  if (!window.confirm('发布后版本及其章节快照将被封存，不能继续修改。确定发布？')) return
  await execute('publish-version', {}, (key) => narrativeApi.publishVersion(route.params.versionId, key))
}

async function startTestIR() {
  if (version.value?.status !== 'published' || !irTestAcknowledged.value || !irRun.extractor_version.trim()) return
  const payload = {
    schema_version: 'narrative-extraction.v1',
    extractor_version: irRun.extractor_version.trim(),
    chapter_ids: [...irRun.chapter_ids],
  }
  const key = getCommandKey('manual-test-ir', payload)
  submitting.value = true
  error.value = ''
  notice.value = ''
  try {
    const response = await narrativeApi.startIRRun(route.params.versionId, payload, key.value)
    commandKeys.delete(key.signature)
    operation.value = response.data
    notice.value = '测试模式 Narrative IR 提取已排队；模型只会按工作流窗口读取章节片段。'
  } catch (err) {
    error.value = err.isConflict ? `${err.message} 请确认版本已发布、章节范围属于当前版本且没有冲突任务。` : err.message
  } finally {
    submitting.value = false
  }
}

async function showChapterHistory(chapterId) {
  historyChapterId.value = chapterId
  historyLoading.value = true
  historyError.value = ''
  try {
    chapterRevisions.value = (await narrativeApi.listChapterRevisions(chapterId)).data
  } catch (err) {
    chapterRevisions.value = []
    historyError.value = err.message
  } finally {
    historyLoading.value = false
  }
}

async function operationFinished() {
  await load()
  if (historyChapterId.value) await showChapterHistory(historyChapterId.value)
}

watch(() => revision.chapter_id, (chapterId) => {
  revision.title = chapters.value.find((item) => item.chapter_id === chapterId)?.title || ''
  revision.content = ''
})
onMounted(load)
</script>

<template>
  <section class="view-stack library-view">
    <RouterLink v-if="version" :to="`/library/${version.work_id}`" class="back-link"><ArrowLeft :size="16" />返回作品版本</RouterLink>
    <div v-if="loading" class="detail-skeleton"><span></span><span></span><span></span></div>
    <div v-else-if="error && !version" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <template v-else-if="version">
      <div class="detail-hero">
        <div class="detail-title"><div class="source-work-cover large"><FileStack :size="25" /></div><div><div class="title-line"><h2>{{ work?.title || version.work_id }} · v{{ version.version_number }}</h2><StatusBadge :status="version.status" /></div><p>{{ version.source_version_id }} · ETag {{ etag }}</p></div></div>
        <div class="detail-actions"><button class="button button-secondary" :disabled="loading" @click="load"><RefreshCw :size="16" />刷新版本</button><button v-if="isDraft" class="button button-primary" :disabled="submitting || !chapters.length" @click="publishVersion"><Send :size="16" />发布并封存</button></div>
      </div>

      <div v-if="error" class="error-banner large"><AlertTriangle :size="17" />{{ error }}<button @click="error = ''">关闭</button></div>
      <div v-if="notice" class="success-banner"><CheckCircle2 :size="17" />{{ notice }}</div>
      <OperationTracker :operation="operation" @terminal="operationFinished" />

      <div class="version-stats">
        <div><span>章节</span><strong>{{ version.chapter_count }}</strong></div><div><span>总字符</span><strong>{{ Number(version.total_chars).toLocaleString('zh-CN') }}</strong></div><div><span>规范化</span><strong>{{ version.normalization_version }}</strong></div><div><span>资源修订</span><strong>{{ version.resource_revision }}</strong></div>
      </div>

      <article v-if="isDraft" class="panel import-panel">
        <div class="production-data-head"><div><span>INGEST COMMANDS</span><h3>导入与修订</h3></div><p>每次写入均携带 Idempotency-Key 与当前 If-Match</p></div>
        <div class="import-tabs">
          <button :class="{ active: activeMode === 'whole' }" @click="activeMode = 'whole'"><Upload :size="16" />整本拆章</button>
          <button :class="{ active: activeMode === 'single' }" @click="activeMode = 'single'"><BookPlus :size="16" />逐章新增</button>
          <button :class="{ active: activeMode === 'batch' }" @click="activeMode = 'batch'"><FileStack :size="16" />批量导入</button>
          <button :class="{ active: activeMode === 'revision' }" @click="activeMode = 'revision'"><PencilLine :size="16" />章节修订</button>
        </div>

        <form v-if="activeMode === 'whole'" class="import-form" @submit.prevent="submitWhole">
          <div class="contract-notice">整本内容由后端分段处理；前端只提交一次导入命令，不会把正文发送给其他模型接口。</div>
          <label class="field"><span>整本小说正文 <i>*</i></span><textarea v-model="wholeText" rows="16" placeholder="粘贴完整正文；章节标题会由导入服务识别。" required></textarea><small>{{ wholeText.length.toLocaleString('zh-CN') }} 字符</small></label>
          <button class="button button-primary" :disabled="submitting || !wholeText.trim()">{{ submitting ? '提交中…' : '开始自动拆章' }}</button>
        </form>

        <form v-else-if="activeMode === 'single'" class="import-form" @submit.prevent="submitSingle">
          <div class="field-pair"><label class="field"><span>章节序号 <i>*</i></span><input v-model.number="single.ordinal" type="number" min="1" required /></label><label class="field"><span>章节标题 <i>*</i></span><input v-model="single.title" maxlength="1000" required /></label></div>
          <label class="field"><span>章节正文 <i>*</i></span><textarea v-model="single.content" rows="12" required></textarea><small>{{ single.content.length.toLocaleString('zh-CN') }} 字符</small></label>
          <button class="button button-primary" :disabled="submitting || !single.title.trim() || !single.content.trim()">{{ submitting ? '提交中…' : '新增章节' }}</button>
        </form>

        <form v-else-if="activeMode === 'batch'" class="import-form" @submit.prevent="submitBatch">
          <div class="contract-notice">使用 Markdown 标题分章，例如 <code># 第一章 雨夜</code>。每个标题直到下个标题之间为一章。</div>
          <label class="field"><span>批量章节 <i>*</i></span><textarea v-model="batchText" rows="16" placeholder="# 第一章&#10;正文……&#10;&#10;# 第二章&#10;正文……" required></textarea><small>识别到 {{ batchPreview.length }} 章，将从序号 {{ nextOrdinal }} 开始</small></label>
          <button class="button button-primary" :disabled="submitting || !batchPreview.length">{{ submitting ? '提交中…' : `导入 ${batchPreview.length} 章` }}</button>
        </form>

        <form v-else class="import-form" @submit.prevent="submitRevision">
          <div class="contract-notice warning">冻结契约不提供章节正文读取接口。修订时必须提交完整的新标题和正文，不能仅提交差异。</div>
          <label class="field"><span>选择章节 <i>*</i></span><select v-model="revision.chapter_id" required><option value="">请选择</option><option v-for="chapter in chapters" :key="chapter.chapter_id" :value="chapter.chapter_id">{{ chapter.ordinal }} · {{ chapter.title }}</option></select></label>
          <label class="field"><span>新标题 <i>*</i></span><input v-model="revision.title" maxlength="1000" required /></label>
          <label class="field"><span>完整修订正文 <i>*</i></span><textarea v-model="revision.content" rows="12" required></textarea><small>{{ revision.content.length.toLocaleString('zh-CN') }} 字符</small></label>
          <button class="button button-primary" :disabled="submitting || !revision.chapter_id || !revision.title.trim() || !revision.content.trim()">{{ submitting ? '提交中…' : '创建章节修订' }}</button>
        </form>
      </article>

      <article class="panel narrative-ir-panel">
        <div class="production-data-head"><div><span>NARRATIVE IR</span><h3>叙事中间层提取</h3></div><p>{{ irRevisions.length }} 个修订</p></div>
        <form v-if="version.status === 'published'" class="ir-test-form" @submit.prevent="startTestIR">
          <div class="contract-notice warning"><FlaskConical :size="17" /><div><strong>手工测试模式</strong><span>仅用于验证已发布版本的 IR 提取。空章节范围表示完整版本；非空范围仅处理明确选中的章节。请求不会额外发送 <code>test_mode</code> 字段。</span></div></div>
          <label class="field"><span>Extractor 版本 <i>*</i></span><input v-model="irRun.extractor_version" maxlength="200" required /></label>
          <div class="ir-chapter-scope"><strong>测试章节范围</strong><span>已选 {{ irRun.chapter_ids.length }} 章；不选则提取完整版本</span><div><label v-for="chapter in chapters" :key="chapter.chapter_id"><input v-model="irRun.chapter_ids" type="checkbox" :value="chapter.chapter_id" /><span>{{ chapter.ordinal }} · {{ chapter.title }}</span></label></div></div>
          <label class="test-ack"><input v-model="irTestAcknowledged" type="checkbox" /><span>我确认这是测试提取操作，会创建可审计的 IR operation。</span></label>
          <button class="button button-primary" :disabled="submitting || !irTestAcknowledged || !irRun.extractor_version.trim()"><BrainCircuit :size="16" />{{ submitting ? '提交中…' : '启动测试 IR' }}</button>
        </form>
        <div v-if="!irRevisions.length" class="compact-empty">当前版本还没有 Narrative IR 修订。</div>
        <div v-else class="ir-revision-list"><article v-for="item in irRevisions" :key="item.ir_revision_id"><b>IR r{{ item.revision_number }}</b><div><strong>{{ item.extractor_version }}</strong><code>{{ item.ir_revision_id }}</code><small>{{ item.revision_scope }}<template v-if="item.changed_chapter_ids?.length"> · {{ item.changed_chapter_ids.length }} 个变更章节</template></small></div><StatusBadge :status="item.status" /><time>{{ new Date(item.published_at || item.created_at).toLocaleString('zh-CN') }}</time></article></div>
      </article>

      <article class="panel chapter-list-panel">
        <div class="production-data-head"><div><span>ORDERED SNAPSHOT</span><h3>版本章节</h3></div><p>{{ chapters.length }} 章</p></div>
        <div v-if="!chapters.length" class="compact-empty">当前版本尚无章节。</div>
        <div v-else class="table-wrap"><table><thead><tr><th>序号</th><th>标题</th><th>修订</th><th>字符数</th><th>内容哈希</th><th>Chapter ID</th><th></th></tr></thead><tbody><tr v-for="chapter in chapters" :key="chapter.chapter_revision_id"><td><b>{{ chapter.ordinal }}</b></td><td>{{ chapter.title }}</td><td>r{{ chapter.revision_number }}</td><td>{{ Number(chapter.char_count).toLocaleString('zh-CN') }}</td><td><code class="hash-code">{{ chapter.content_hash }}</code></td><td><code>{{ chapter.chapter_id }}</code></td><td><button class="row-action" aria-label="查看修订历史" @click="showChapterHistory(chapter.chapter_id)"><History :size="16" /></button></td></tr></tbody></table></div>
      </article>

      <article v-if="historyChapterId" class="panel chapter-history-panel">
        <div class="production-data-head"><div><span>IMMUTABLE HISTORY</span><h3>章节修订历史</h3></div><code>{{ historyChapterId }}</code></div>
        <div v-if="historyLoading" class="compact-empty">正在读取修订历史……</div>
        <div v-else-if="historyError" class="error-banner">{{ historyError }} <button @click="showChapterHistory(historyChapterId)">重试</button></div>
        <div v-else class="revision-history-list"><article v-for="item in chapterRevisions" :key="item.chapter_revision_id"><b>r{{ item.revision_number }}</b><div><strong>{{ item.title }}</strong><code>{{ item.chapter_revision_id }}</code></div><span>{{ Number(item.char_count).toLocaleString('zh-CN') }} 字符</span><code class="hash-code">{{ item.content_hash }}</code><time>{{ new Date(item.created_at).toLocaleString('zh-CN') }}</time></article></div>
      </article>
    </template>
  </section>
</template>
