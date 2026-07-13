<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { AlertTriangle, Bot, Container, FileCog, KeyRound, RefreshCw, RotateCcw, Save, ShieldCheck } from 'lucide-vue-next'
import { api } from '../services/api'

const data = ref(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const result = ref(null)
const drafts = reactive({})
const baseline = reactive({})
const secretDrafts = reactive({})

const categories = computed(() => {
  const groups = []
  for (const field of data.value?.fields || []) {
    let group = groups.find((item) => item.name === field.category)
    if (!group) {
      group = { name: field.category, fields: [] }
      groups.push(group)
    }
    group.fields.push(field)
  }
  return groups
})
const changedValues = computed(() => Object.fromEntries(
  Object.keys(drafts).filter((key) => drafts[key] !== baseline[key]).map((key) => [key, drafts[key]]),
))
const pendingSecrets = computed(() => Object.fromEntries(
  Object.entries(secretDrafts).filter(([, value]) => value !== ''),
))
const changeCount = computed(() => Object.keys(changedValues.value).length + Object.keys(pendingSecrets.value).length)

function hydrate(response) {
  data.value = response
  for (const field of response.fields || []) {
    const value = field.has_managed_override ? field.managed_value : field.current_value
    drafts[field.key] = value
    baseline[field.key] = value
  }
  for (const secret of response.secrets || []) secretDrafts[secret.key] = ''
}

async function load() {
  loading.value = true
  error.value = ''
  try { hydrate(await api.getAIConfig()) }
  catch (err) { error.value = err.message }
  finally { loading.value = false }
}

function resetDrafts() {
  for (const key of Object.keys(baseline)) drafts[key] = baseline[key]
  for (const key of Object.keys(secretDrafts)) secretDrafts[key] = ''
}

async function save() {
  if (!changeCount.value || saving.value) return
  saving.value = true
  error.value = ''
  result.value = null
  try {
    result.value = await api.updateAIConfig({ values: changedValues.value, secrets: pendingSecrets.value })
    for (const key of Object.keys(secretDrafts)) secretDrafts[key] = ''
    await load()
  } catch (err) {
    error.value = err.message
  } finally {
    saving.value = false
  }
}

const displayCurrent = (field) => field.current_value || '未配置'
onMounted(load)
</script>

<template>
  <section class="view-stack ai-config-view">
    <div class="hero-row">
      <div><h2>AI 配置管理</h2><p>读取 n8n 容器当前环境，并安全管理下次重建时生效的模型与 Provider 覆盖配置。</p></div>
      <div class="hero-actions ai-config-actions"><button class="button button-secondary" :disabled="loading || saving" @click="load"><RefreshCw :size="16" :class="{ spin: loading }" />重新读取</button><button class="button button-primary" :disabled="!changeCount || saving" @click="save"><Save :size="16" />{{ saving ? '安全写入中…' : `保存配置${changeCount ? ` (${changeCount})` : ''}` }}</button></div>
    </div>

    <div class="config-notice managed-config-notice" :class="{ pending: data?.pending_restart }"><ShieldCheck :size="20" /><div><strong>{{ data?.pending_restart ? '存在待生效配置' : '密钥保护已启用' }}</strong><p>API Key 只允许覆盖写入，页面和接口永不返回明文；配置不会写入数据库或请求日志。</p></div><span>{{ data?.managed_file || 'cms-managed.env' }}</span></div>

    <div v-if="result" class="config-save-result"><AlertTriangle :size="20" /><div><strong>{{ result.message }}</strong><span>普通 restart 不会重新加载环境变量，请使用以下命令重建 n8n：</span><code>{{ result.restart_command }}</code></div></div>
    <div v-else-if="data?.pending_restart" class="config-save-result pending"><AlertTriangle :size="20" /><div><strong>CMS 托管文件与当前容器环境不同</strong><span>需要重建 n8n 容器后才会使用待生效值。</span><code>{{ data.restart_command }}</code></div></div>

    <div v-if="error" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <div v-if="loading" class="config-loading"><span></span><span></span></div>
    <template v-else-if="data">
      <article class="panel config-source-panel">
        <div><Container :size="19" /><span>配置来源</span><strong>{{ data.source }}</strong><code>{{ data.container_name }}</code></div>
        <div><FileCog :size="19" /><span>容器状态</span><strong>{{ data.container_status }}</strong><code>{{ data.managed_file_exists ? '托管文件已创建' : '尚无托管文件' }}</code></div>
        <div><ShieldCheck :size="19" /><span>密钥响应</span><strong>{{ data.secrets_exposed ? '风险：已暴露' : '已脱敏' }}</strong><code>boolean status only</code></div>
      </article>

      <article v-for="group in categories" :key="group.name" class="panel padded ai-config-group">
        <div class="section-title"><div><span>CMS MANAGED ENV</span><h3>{{ group.name }}</h3></div><div class="section-icon"><Bot :size="19" /></div></div>
        <div class="config-field-grid">
          <label v-for="field in group.fields" :key="field.key" class="config-edit-field" :class="{ dangerous: field.key === 'ALLOW_REAL_PUBLISH' }">
            <span class="config-field-head"><strong>{{ field.label }}</strong><i v-if="field.has_managed_override">待重启覆盖</i></span>
            <code>{{ field.key }}</code>
            <select v-if="field.kind === 'boolean'" v-model="drafts[field.key]" class="select-control"><option value="true">true</option><option value="false">false</option></select>
            <input v-else v-model="drafts[field.key]" :type="field.kind === 'url' ? 'url' : 'text'" :placeholder="field.allow_empty ? '留空表示禁用' : '请输入配置值'" spellcheck="false" />
            <small>当前容器：<b>{{ displayCurrent(field) }}</b></small>
          </label>
        </div>
      </article>

      <article class="panel padded secret-config-panel">
        <div class="section-title"><div><span>WRITE ONLY CREDENTIALS</span><h3>敏感 API Key</h3></div><div class="section-icon"><KeyRound :size="19" /></div></div>
        <p class="secret-intro">输入框始终为空。填写后只会覆盖写入 CMS 托管 env 文件，保存响应及再次读取均不会包含密钥内容。</p>
        <div class="secret-field-grid">
          <label v-for="secret in data.secrets" :key="secret.key" class="secret-edit-field">
            <div><strong>{{ secret.label }}</strong><span :class="{ configured: secret.configured }">当前容器：{{ secret.configured ? '已配置' : '未配置' }}</span><span v-if="secret.managed_override_configured" class="pending-secret">托管文件：已填写，待重启</span></div>
            <code>{{ secret.key }}</code>
            <input v-model="secretDrafts[secret.key]" type="password" autocomplete="new-password" placeholder="留空不修改；输入新值将覆盖" spellcheck="false" />
          </label>
        </div>
      </article>

      <div class="config-footer-actions"><span><ShieldCheck :size="15" />保存内容仅限白名单配置；密钥不会进入前端响应。</span><button class="button button-secondary" :disabled="!changeCount || saving" @click="resetDrafts"><RotateCcw :size="15" />放弃修改</button><button class="button button-primary" :disabled="!changeCount || saving" @click="save"><Save :size="15" />保存到 cms-managed.env</button></div>
    </template>
  </section>
</template>
