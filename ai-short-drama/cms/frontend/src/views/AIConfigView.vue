<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  AlertTriangle, Bot, Boxes, Building2, Cable, ChevronDown, CircleGauge, Container,
  FileCog, Globe2, Image, KeyRound, Mic2, Network, RefreshCw, Rocket, RotateCcw,
  Save, Settings2, ShieldCheck, SlidersHorizontal, Video, Waypoints,
} from 'lucide-vue-next'
import { api } from '../services/api'

const data = ref(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const result = ref(null)
const activeTab = ref('overview')
const activeModelSection = ref('text')
const restartExpanded = ref(false)
const drafts = reactive({})
const baseline = reactive({})
const secretDrafts = reactive({})

const sourceLabels = { native: '原生直连', custom: '自定义接口', gateway: '统一网关' }
const sourceIcons = { native: Building2, custom: Cable, gateway: Network }
const optionLabels = {
  'gemini-omni-flash-preview': 'Gemini Omni Flash（推荐）',
  'veo-3.1-generate-001': 'Veo 3.1',
  'veo-3.1-fast-generate-001': 'Veo 3.1 Fast（推荐批量生产）',
  'mock-image-to-video': 'Mock 测试模型',
  'gemini-3.1-flash-tts-preview': 'Gemini 3.1 Flash TTS Preview（推荐）',
  'gemini-2.5-flash-preview-tts': 'Gemini 2.5 Flash Preview TTS',
  'gemini-2.5-pro-preview-tts': 'Gemini 2.5 Pro Preview TTS',
  'gemini-2.5-flash-tts': 'Vertex Gemini 2.5 Flash TTS',
  'gemini-2.5-pro-tts': 'Vertex Gemini 2.5 Pro TTS',
  'gemini-2.5-flash-lite-preview-tts': 'Vertex Gemini 2.5 Flash-Lite TTS Preview',
  'chirp-3-hd': 'Chirp 3 HD',
}
const defaultPlan = {
  AI_CONNECTION_MODE: 'hybrid', TEXT_API_SOURCE: 'gateway', IMAGE_API_SOURCE: 'native',
  VIDEO_API_SOURCE: 'native', TTS_API_SOURCE: 'native', VIDEO_USE_GENERATED_AUDIO: 'false',
  VEO_LOCATION: 'us-central1', VEO_OUTPUT_MODE: 'local', TTS_VERTEX_LOCATION: 'global',
}
const googleVideoModels = [
  { id: 'gemini-omni-flash-preview', title: 'Gemini Omni Flash', badge: '推荐', description: '3–10 秒、720p，速度快、角色一致性好；当前为 Preview。' },
  { id: 'veo-3.1-fast-generate-001', title: 'Veo 3.1 Fast', badge: '批量', description: '适合批量镜头生产，支持 4/6/8 秒与 720p/1080p。' },
  { id: 'veo-3.1-generate-001', title: 'Veo 3.1', badge: '质量', description: '适合重点镜头，生成速度与成本通常高于 Fast。' },
]
const googleSpeechModels = [
  { provider: 'google_gemini_speech', model: 'gemini-3.1-flash-tts-preview', title: 'Gemini Speech', badge: '可控', description: '原生 Gemini TTS，支持用自然语言控制语气、节奏和表现；当前为 Preview。', baseUrl: 'https://generativelanguage.googleapis.com', voice: 'Kore' },
  { provider: 'google_vertex_gemini_speech', model: 'gemini-3.1-flash-tts-preview', title: 'Vertex AI Gemini TTS', badge: '生产', description: '通过 Google Cloud 服务账号调用 Vertex AI，支持项目结算、IAM 与区域配置。', baseUrl: 'http://veo-adapter:8091', voice: 'Kore' },
  { provider: 'google_chirp3_hd', model: 'chirp-3-hd', title: 'Chirp 3 HD', badge: '稳定', description: 'Google Cloud 高保真语音，普通话使用 cmn-CN-Chirp3-HD-* 完整声线 ID。', baseUrl: 'https://texttospeech.googleapis.com', voice: 'cmn-CN-Chirp3-HD-Kore' },
]
const geminiSpeechModelOptions = ['gemini-3.1-flash-tts-preview', 'gemini-2.5-flash-preview-tts', 'gemini-2.5-pro-preview-tts']
const vertexSpeechModelOptions = ['gemini-3.1-flash-tts-preview', 'gemini-2.5-flash-tts', 'gemini-2.5-pro-tts', 'gemini-2.5-flash-lite-preview-tts']
const googleVideoFieldKeys = new Set([
  'VIDEO_PROVIDER', 'VIDEO_MODEL', 'VIDEO_USE_GENERATED_AUDIO',
  'VEO_OUTPUT_MODE', 'VEO_PROJECT_ID', 'VEO_LOCATION', 'VEO_GCS_OUTPUT_URI',
])
const googleAudioFieldKeys = new Set([
  'TTS_PROVIDER', 'TTS_MODEL', 'DEFAULT_NARRATOR_VOICE_ID',
  'TTS_VERTEX_PROJECT_ID', 'TTS_VERTEX_LOCATION',
])
const connectionModes = [
  {
    id: 'hybrid', title: '混合路由', icon: Waypoints, recommended: true,
    description: '文本走统一网关，图片、视频和语音走原生授权接口，兼顾统一管理与媒体协议兼容。',
    sources: { TEXT_API_SOURCE: 'gateway', IMAGE_API_SOURCE: 'native', VIDEO_API_SOURCE: 'native', TTS_API_SOURCE: 'native' },
  },
  {
    id: 'native', title: '全部原生直连', icon: Building2,
    description: '每类能力直接连接供应商接口，链路最短；供应商协议必须已有对应适配器。',
    sources: { TEXT_API_SOURCE: 'native', IMAGE_API_SOURCE: 'native', VIDEO_API_SOURCE: 'native', TTS_API_SOURCE: 'native' },
  },
  {
    id: 'custom', title: '全部自定义接口', icon: Cable,
    description: '分别填写文本、图片、视频和 TTS 的自定义地址，适合已有内部代理或供应商聚合层。',
    sources: { TEXT_API_SOURCE: 'custom', IMAGE_API_SOURCE: 'custom', VIDEO_API_SOURCE: 'custom', TTS_API_SOURCE: 'custom' },
  },
  {
    id: 'gateway', title: '兼容网关优先', icon: Network,
    description: '文本和图片使用 OpenAI 兼容网关；视频与 TTS 保留自定义适配协议，避免错误复用同一地址。',
    sources: { TEXT_API_SOURCE: 'gateway', IMAGE_API_SOURCE: 'gateway', VIDEO_API_SOURCE: 'custom', TTS_API_SOURCE: 'custom' },
  },
]
const capabilities = [
  {
    id: 'text', title: '文本生成', icon: Bot, sourceKey: 'TEXT_API_SOURCE', baseKey: 'LITELLM_BASE_URL', secretKey: 'LITELLM_API_KEY',
    endpoint: 'POST {Base URL}/v1/chat/completions', note: '用于小说分析、故事圣经、剧本、分镜和文本质检。Base URL 不要重复填写 /v1。',
  },
  {
    id: 'image', title: '图片生成', icon: Image, sourceKey: 'IMAGE_API_SOURCE', baseKey: 'IMAGE_API_BASE_URL', secretKey: 'IMAGE_API_KEY',
    endpoint: 'POST {Base URL}/v1/images/generations', note: 'OpenAI 兼容同步接口需返回图片 URL；异步接口请选择 generic_async_image。',
  },
  {
    id: 'video', title: '视频生成', icon: Video, sourceKey: 'VIDEO_API_SOURCE', baseKey: 'VIDEO_API_BASE_URL', secretKey: 'VIDEO_API_KEY',
    endpoint: 'POST /generate · GET /tasks/{id}', note: '当前使用异步视频适配协议，不等同于普通 OpenAI 文本网关；原生供应商需要相应适配器。',
  },
  {
    id: 'tts', title: '语音合成', icon: Mic2, sourceKey: 'TTS_API_SOURCE', baseKey: 'TTS_API_BASE_URL', secretKey: 'TTS_API_KEY',
    endpoint: 'Google 原生接口 · 或 POST {Base URL}/tts', note: 'Google 模型由内置适配器安全解码并落盘；自定义同步接口仍须返回可下载的音频 URL。',
  },
]
const routedFieldKeys = new Set([
  'AI_CONNECTION_MODE', ...capabilities.flatMap((item) => [item.sourceKey, item.baseKey]),
])
const routedSecretKeys = new Set(capabilities.map((item) => item.secretKey))
const operationalFieldKeys = new Set(['MOCK_MODE', 'PUBLISH_PROVIDER', 'ALLOW_REAL_PUBLISH'])
const configTabs = [
  { id: 'overview', label: '概览', description: '状态与摘要', icon: CircleGauge },
  { id: 'connections', label: '接口与密钥', description: '路由与凭据', icon: Network },
  { id: 'models', label: '模型配置', description: '四类生成能力', icon: SlidersHorizontal },
  { id: 'operations', label: '运行与发布', description: '环境与上线', icon: Rocket },
  { id: 'advanced', label: '高级设置', description: '其他敏感项', icon: Settings2 },
]
const modelSections = [
  { id: 'text', label: '文本', icon: Bot },
  { id: 'image', label: '图片', icon: Image },
  { id: 'video', label: '视频', icon: Video },
  { id: 'tts', label: '语音', icon: Mic2 },
]
const capabilityModelKeys = {
  text: 'TEXT_ANALYSIS_MODEL',
  image: 'IMAGE_MODEL',
  video: 'VIDEO_MODEL',
  tts: 'TTS_MODEL',
}

const fieldsByKey = computed(() => Object.fromEntries((data.value?.fields || []).map((field) => [field.key, field])))
const secretsByKey = computed(() => Object.fromEntries((data.value?.secrets || []).map((secret) => [secret.key, secret])))
const categories = computed(() => {
  const groups = []
  for (const field of data.value?.fields || []) {
    if (routedFieldKeys.has(field.key) || googleVideoFieldKeys.has(field.key) || googleAudioFieldKeys.has(field.key)) continue
    let group = groups.find((item) => item.name === field.category)
    if (!group) {
      group = { name: field.category, fields: [] }
      groups.push(group)
    }
    group.fields.push(field)
  }
  return groups
})
const textModelCategories = computed(() => categories.value.map((group) => ({
  ...group,
  fields: group.fields.filter((field) => !operationalFieldKeys.has(field.key) && !field.key.startsWith('IMAGE_')),
})).filter((group) => group.fields.length))
const imageModelCategories = computed(() => categories.value.map((group) => ({
  ...group,
  fields: group.fields.filter((field) => !operationalFieldKeys.has(field.key) && field.key.startsWith('IMAGE_')),
})).filter((group) => group.fields.length))
const operationsCategories = computed(() => categories.value.map((group) => ({
  ...group,
  fields: group.fields.filter((field) => operationalFieldKeys.has(field.key)),
})).filter((group) => group.fields.length))
const advancedSecrets = computed(() => (data.value?.secrets || []).filter((secret) => !routedSecretKeys.has(secret.key) && secret.key !== 'VEO_SERVICE_ACCOUNT_JSON'))
const googleCredential = computed(() => secretsByKey.value.VEO_SERVICE_ACCOUNT_JSON)
const currentMode = computed(() => connectionModes.find((mode) => mode.id === drafts.AI_CONNECTION_MODE) || connectionModes[0])
const changedValues = computed(() => Object.fromEntries(
  Object.keys(drafts).filter((key) => drafts[key] !== baseline[key]).map((key) => [key, drafts[key]]),
))
const pendingSecrets = computed(() => Object.fromEntries(
  Object.entries(secretDrafts).filter(([, value]) => value !== ''),
))
const changeCount = computed(() => Object.keys(changedValues.value).length + Object.keys(pendingSecrets.value).length)
const tabChangeCounts = computed(() => {
  const changedKeys = new Set(Object.keys(changedValues.value))
  const changedSecretKeys = new Set(Object.keys(pendingSecrets.value))
  const modelFieldKeys = new Set([
    ...googleVideoFieldKeys,
    ...googleAudioFieldKeys,
    ...textModelCategories.value.flatMap((group) => group.fields.map((field) => field.key)),
    ...imageModelCategories.value.flatMap((group) => group.fields.map((field) => field.key)),
  ])
  return {
    connections: [...routedFieldKeys].filter((key) => changedKeys.has(key)).length
      + [...routedSecretKeys, 'VEO_SERVICE_ACCOUNT_JSON'].filter((key) => changedSecretKeys.has(key)).length,
    models: [...modelFieldKeys].filter((key) => changedKeys.has(key)).length,
    operations: [...operationalFieldKeys].filter((key) => changedKeys.has(key)).length,
    advanced: advancedSecrets.value.filter((secret) => changedSecretKeys.has(secret.key)).length,
  }
})

function hydrate(response) {
  data.value = response
  for (const key of Object.keys(drafts)) delete drafts[key]
  for (const key of Object.keys(baseline)) delete baseline[key]
  for (const key of Object.keys(secretDrafts)) delete secretDrafts[key]
  for (const field of response.fields || []) {
    let value = field.has_managed_override ? field.managed_value : field.current_value
    if (!value && defaultPlan[field.key]) value = defaultPlan[field.key]
    drafts[field.key] = value
    baseline[field.key] = value
  }
  for (const secret of response.secrets || []) secretDrafts[secret.key] = ''
  if (googleVideoModels.some((model) => model.id === drafts.VIDEO_MODEL)) {
    if (!drafts.VIDEO_API_BASE_URL) drafts.VIDEO_API_BASE_URL = 'http://veo-adapter:8091'
    if (drafts.VIDEO_PROVIDER !== 'generic_async_video') drafts.VIDEO_PROVIDER = 'generic_async_video'
    if (drafts.VIDEO_API_SOURCE !== 'native') drafts.VIDEO_API_SOURCE = 'native'
  }
  const googleSpeech = googleSpeechModels.find((item) => item.provider === drafts.TTS_PROVIDER)
  if (googleSpeech) {
    if (!drafts.TTS_API_BASE_URL) drafts.TTS_API_BASE_URL = googleSpeech.baseUrl
    if (!drafts.TTS_MODEL) drafts.TTS_MODEL = googleSpeech.model
    if (!drafts.DEFAULT_NARRATOR_VOICE_ID) drafts.DEFAULT_NARRATOR_VOICE_ID = googleSpeech.voice
    if (drafts.TTS_API_SOURCE !== 'native') drafts.TTS_API_SOURCE = 'native'
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try { hydrate(await api.getAIConfig()) }
  catch (err) { error.value = err.message }
  finally { loading.value = false }
}

function applyMode(mode) {
  drafts.AI_CONNECTION_MODE = mode.id
  for (const [key, value] of Object.entries(mode.sources)) drafts[key] = value
}

function selectGoogleVideoModel(model) {
  drafts.VIDEO_API_SOURCE = 'native'
  drafts.VIDEO_PROVIDER = 'generic_async_video'
  drafts.VIDEO_MODEL = model
  drafts.VIDEO_API_BASE_URL = 'http://veo-adapter:8091'
}

function selectGoogleSpeechModel(item) {
  drafts.TTS_API_SOURCE = 'native'
  drafts.TTS_PROVIDER = item.provider
  drafts.TTS_MODEL = item.model
  drafts.TTS_API_BASE_URL = item.baseUrl
  drafts.DEFAULT_NARRATOR_VOICE_ID = item.voice
}

function openModelSection(section) {
  activeTab.value = 'models'
  activeModelSection.value = section
}

function capabilityModelValue(capability) {
  return drafts[capabilityModelKeys[capability.id]] || '未配置'
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

const displayCurrent = (field) => field?.current_value || '未配置'
const sourceIcon = (source) => sourceIcons[source] || Globe2
onMounted(load)
</script>

<template>
  <section class="view-stack ai-config-view">
    <div class="hero-row">
      <div><h2>AI 接口与模型配置</h2><p>选择原生接口、自定义接口或统一网关，并按能力配置安全独立的访问地址与密钥。</p></div>
      <div class="hero-actions ai-config-actions"><button class="button button-secondary" :disabled="loading || saving" @click="load"><RefreshCw :size="16" :class="{ spin: loading }" />重新读取</button><button class="button button-primary" :disabled="!changeCount || saving" @click="save"><Save :size="16" />{{ saving ? '安全写入中…' : `保存配置${changeCount ? ` (${changeCount})` : ''}` }}</button></div>
    </div>

    <div class="config-notice managed-config-notice" :class="{ pending: data?.pending_restart }"><ShieldCheck :size="20" /><div><strong>{{ data?.pending_restart ? '存在待生效配置' : '密钥保护已启用' }}</strong><p>密钥只允许覆盖写入，页面和接口永不返回明文；不同能力可使用不同供应商和 Token。</p></div><span>{{ data?.managed_file || 'cms-managed.env' }}</span></div>

    <div v-if="result" class="config-save-result"><AlertTriangle :size="20" /><div><strong>{{ result.message }}</strong><span>普通 restart 不会重新加载环境变量，需要重建相关服务后才会生效。</span><button type="button" class="restart-command-toggle" :class="{ expanded: restartExpanded }" @click="restartExpanded = !restartExpanded"><ChevronDown :size="15" />{{ restartExpanded ? '收起重启命令' : '查看重启命令' }}</button><code v-if="restartExpanded">{{ result.restart_command }}</code></div></div>
    <div v-else-if="data?.pending_restart" class="config-save-result pending"><AlertTriangle :size="20" /><div><strong>CMS 托管文件与当前容器环境不同</strong><span>需要重建 n8n 与 Google 视频适配器后才会使用待生效值。</span><button type="button" class="restart-command-toggle" :class="{ expanded: restartExpanded }" @click="restartExpanded = !restartExpanded"><ChevronDown :size="15" />{{ restartExpanded ? '收起重启命令' : '查看重启命令' }}</button><code v-if="restartExpanded">{{ data.restart_command }}</code></div></div>

    <div v-if="error" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <div v-if="loading" class="config-loading"><span></span><span></span></div>
    <template v-else-if="data">
      <nav class="ai-config-tabs" role="tablist" aria-label="AI 配置分类">
        <button v-for="tab in configTabs" :id="`ai-config-tab-${tab.id}`" :key="tab.id" type="button" role="tab" :aria-selected="activeTab === tab.id" :aria-controls="`ai-config-panel-${tab.id}`" :class="{ active: activeTab === tab.id }" @click="activeTab = tab.id">
          <component :is="tab.icon" :size="18" />
          <span><strong>{{ tab.label }}</strong><small>{{ tab.description }}</small></span>
          <i v-if="tabChangeCounts[tab.id]">{{ tabChangeCounts[tab.id] }}</i>
        </button>
      </nav>

      <div v-show="activeTab === 'overview'" id="ai-config-panel-overview" class="ai-config-tab-panel" role="tabpanel" aria-labelledby="ai-config-tab-overview">
        <article class="panel config-source-panel">
          <div><Container :size="19" /><span>配置来源</span><strong>{{ data.source }}</strong><code>{{ data.container_name }}</code></div>
          <div><FileCog :size="19" /><span>容器状态</span><strong>{{ data.container_status }}</strong><code>{{ data.managed_file_exists ? '托管文件已创建' : '尚无托管文件' }}</code></div>
          <div><Video :size="19" /><span>Google 视频适配器</span><strong>{{ data.video_adapter_status }}</strong><code>{{ data.video_adapter_name }}</code></div>
          <div><ShieldCheck :size="19" /><span>密钥响应</span><strong>{{ data.secrets_exposed ? '风险：已暴露' : '已脱敏' }}</strong><code>boolean status only</code></div>
        </article>

        <article class="panel padded config-overview-panel">
          <div class="section-title"><div><span>CONFIGURATION OVERVIEW</span><h3>当前配置摘要</h3></div><div class="section-icon"><CircleGauge :size="19" /></div></div>
          <div class="overview-strategy">
            <span class="plan-icon"><component :is="currentMode.icon" :size="20" /></span>
            <div><span>当前接入方案</span><strong>{{ currentMode.title }}</strong><p>{{ currentMode.description }}</p></div>
            <button type="button" class="button button-secondary" @click="activeTab = 'connections'">调整接口与密钥</button>
          </div>
          <div class="overview-capability-grid">
            <button v-for="capability in capabilities" :key="capability.id" type="button" class="overview-capability-card" @click="openModelSection(capability.id)">
              <span><component :is="capability.icon" :size="19" /></span>
              <div><strong>{{ capability.title }}</strong><small>{{ sourceLabels[drafts[capability.sourceKey]] || '未配置' }}</small><code>{{ capabilityModelValue(capability) }}</code></div>
              <i>配置模型</i>
            </button>
          </div>
        </article>
      </div>

      <div v-show="activeTab === 'connections'" id="ai-config-panel-connections" class="ai-config-tab-panel" role="tabpanel" aria-labelledby="ai-config-tab-connections">
        <article class="panel padded connection-plan-panel">
        <div class="section-title"><div><span>CONNECTION STRATEGY</span><h3>选择接入方案</h3></div><div class="section-icon"><Boxes :size="19" /></div></div>
        <p class="plan-intro">方案只负责组织路由，不会覆盖已填写的 URL、模型或密钥。选择后仍可逐项调整。</p>
        <div class="connection-plan-grid">
          <button v-for="mode in connectionModes" :key="mode.id" type="button" class="connection-plan-card" :class="{ active: drafts.AI_CONNECTION_MODE === mode.id }" @click="applyMode(mode)">
            <span class="plan-icon"><component :is="mode.icon" :size="20" /></span>
            <span class="plan-copy"><strong>{{ mode.title }}<i v-if="mode.recommended">推荐</i></strong><small>{{ mode.description }}</small></span>
            <span class="plan-radio"><i></i></span>
          </button>
        </div>
        </article>

        <article class="panel padded capability-routing-panel">
        <div class="section-title"><div><span>CAPABILITY ROUTING</span><h3>按能力配置接口</h3></div><div class="section-icon"><Network :size="19" /></div></div>
        <div class="capability-route-grid">
          <section v-for="capability in capabilities" :key="capability.id" class="capability-route-card">
            <div class="capability-route-head"><span><component :is="capability.icon" :size="19" /></span><div><strong>{{ capability.title }}</strong><small>{{ capability.endpoint }}</small></div></div>
            <label><span>接口来源</span><div class="source-select-wrap"><component :is="sourceIcon(drafts[capability.sourceKey])" :size="15" /><select v-model="drafts[capability.sourceKey]" class="select-control"><option value="native">{{ sourceLabels.native }}</option><option value="custom">{{ sourceLabels.custom }}</option><option value="gateway">{{ sourceLabels.gateway }}</option></select></div></label>
            <label><span>Base URL <i v-if="fieldsByKey[capability.baseKey]?.has_managed_override">待重启覆盖</i></span><input v-model="drafts[capability.baseKey]" type="url" placeholder="https://api.example.com" spellcheck="false" /><small>当前容器：{{ displayCurrent(fieldsByKey[capability.baseKey]) }}</small></label>
            <label><span>{{ capability.id === 'video' ? '内部适配器 Key' : 'API Key' }} <i v-if="secretsByKey[capability.secretKey]?.managed_override_configured">托管文件已填写</i></span><input v-model="secretDrafts[capability.secretKey]" type="password" autocomplete="new-password" :placeholder="secretsByKey[capability.secretKey]?.configured ? '已配置；留空不修改' : capability.id === 'video' ? '填写一个长随机内部访问令牌' : '输入新的 API Key'" spellcheck="false" /><small :class="{ configured: secretsByKey[capability.secretKey]?.configured }">当前容器：{{ secretsByKey[capability.secretKey]?.configured ? '已配置' : '未配置' }}</small></label>
            <p><AlertTriangle :size="13" />{{ capability.note }}</p>
          </section>
        </div>
        </article>

        <article class="panel padded shared-credential-panel">
          <div class="section-title"><div><span>SHARED GOOGLE CREDENTIAL</span><h3>Google 共用服务账号</h3></div><div class="section-icon"><KeyRound :size="19" /></div></div>
          <p class="plan-intro">Veo 与 Vertex AI Gemini TTS 共用此服务账号。凭据只写入权限受限的托管配置，页面与接口永不返回私钥内容。</p>
          <label class="google-credential-field">
            <div><strong>Google 服务账号 JSON</strong><span :class="{ configured: googleCredential?.configured || googleCredential?.managed_override_configured }">{{ googleCredential?.managed_override_configured ? '托管文件已填写，等待重启' : googleCredential?.configured ? '当前已配置' : '尚未配置' }}</span></div>
            <code>VEO_SERVICE_ACCOUNT_JSON</code>
            <textarea v-model="secretDrafts.VEO_SERVICE_ACCOUNT_JSON" rows="8" autocomplete="off" :placeholder="googleCredential?.configured || googleCredential?.managed_override_configured ? '已安全保存；留空不修改。需要更换时粘贴新的完整 JSON。' : '粘贴从 Google Cloud 下载的完整服务账号 JSON'" spellcheck="false"></textarea>
            <small><ShieldCheck :size="14" />JSON 只写入权限受限的托管配置，页面与接口永不返回私钥内容。</small>
          </label>
        </article>
      </div>

      <div v-show="activeTab === 'models'" id="ai-config-panel-models" class="ai-config-tab-panel" role="tabpanel" aria-labelledby="ai-config-tab-models">
        <nav class="model-section-tabs" aria-label="模型能力分类">
          <button v-for="section in modelSections" :key="section.id" type="button" :class="{ active: activeModelSection === section.id }" @click="activeModelSection = section.id"><component :is="section.icon" :size="17" />{{ section.label }}</button>
        </nav>

      <article v-show="activeModelSection === 'video'" class="panel padded google-video-panel">
        <div class="section-title"><div><span>GOOGLE VIDEO</span><h3>Google 视频模型</h3></div><div class="section-icon"><Video :size="19" /></div></div>
        <p class="plan-intro">Veo 3.1 与 Gemini Omni 共用同一个安全适配器和服务账号。点击模型卡即可切换，工作流代码无需修改。</p>
        <div class="google-model-grid">
          <button v-for="model in googleVideoModels" :key="model.id" type="button" class="google-model-card" :class="{ active: drafts.VIDEO_MODEL === model.id }" @click="selectGoogleVideoModel(model.id)">
            <span><Video :size="19" /></span><div><strong>{{ model.title }}<i>{{ model.badge }}</i></strong><code>{{ model.id }}</code><small>{{ model.description }}</small></div><b></b>
          </button>
        </div>
        <div class="google-config-grid">
          <label class="config-edit-field"><span class="config-field-head"><strong>视频输出存储</strong><i v-if="fieldsByKey.VEO_OUTPUT_MODE?.has_managed_override">待重启覆盖</i></span><code>VEO_OUTPUT_MODE</code><select v-model="drafts.VEO_OUTPUT_MODE" class="select-control"><option value="auto">自动（无 GCS 地址时使用本地）</option><option value="local">本地存储（无需 GCS）</option><option value="gcs">Google Cloud Storage</option></select><small>本地模式将生成结果写入适配器的持久化 Docker 卷，再由工作流下载到媒体库。</small></label>
          <label class="config-edit-field"><span class="config-field-head"><strong>Google Cloud Project ID</strong><i v-if="fieldsByKey.VEO_PROJECT_ID?.has_managed_override">待重启覆盖</i></span><code>VEO_PROJECT_ID</code><input v-model="drafts.VEO_PROJECT_ID" type="text" placeholder="可留空，从服务账号自动读取" spellcheck="false" /><small>Google Cloud 项目 ID，不是项目名称。</small></label>
          <label class="config-edit-field"><span class="config-field-head"><strong>Veo 区域</strong><i v-if="fieldsByKey.VEO_LOCATION?.has_managed_override">待重启覆盖</i></span><code>VEO_LOCATION</code><input v-model="drafts.VEO_LOCATION" type="text" placeholder="us-central1" spellcheck="false" /><small>仅用于 Veo；Omni 会自动使用 global。</small></label>
          <label v-if="drafts.VEO_OUTPUT_MODE === 'gcs' || (drafts.VEO_OUTPUT_MODE === 'auto' && drafts.VEO_GCS_OUTPUT_URI)" class="config-edit-field"><span class="config-field-head"><strong>Cloud Storage 输出目录</strong><i v-if="fieldsByKey.VEO_GCS_OUTPUT_URI?.has_managed_override">待重启覆盖</i></span><code>VEO_GCS_OUTPUT_URI</code><input v-model="drafts.VEO_GCS_OUTPUT_URI" type="text" placeholder="gs://bucket/short-drama" spellcheck="false" /><small>服务账号需要对此目录拥有对象创建和读取权限。</small></label>
          <label class="config-edit-field"><span class="config-field-head"><strong>模型原生音频</strong></span><code>VIDEO_USE_GENERATED_AUDIO</code><select v-model="drafts.VIDEO_USE_GENERATED_AUDIO" class="select-control"><option value="false">关闭（使用系统配音，推荐）</option><option value="true">保留模型生成的音频</option></select><small>关闭后标准化阶段会移除 Omni/Veo 自带音轨，避免双重配音。</small></label>
        </div>
      </article>

      <article v-show="activeModelSection === 'tts'" class="panel padded google-audio-panel">
        <div class="section-title"><div><span>GOOGLE SPEECH</span><h3>Google 语音模型</h3></div><div class="section-icon"><Mic2 :size="19" /></div></div>
        <p class="plan-intro">支持 Gemini Developer API、Vertex AI Gemini TTS 与 Chirp 3 HD。Vertex AI 路由使用与 Veo 共用的服务账号完成 OAuth，不会把私钥放入工作流执行数据。</p>
        <div class="google-model-grid google-speech-model-grid">
          <button v-for="item in googleSpeechModels" :key="item.provider" type="button" class="google-model-card" :class="{ active: drafts.TTS_PROVIDER === item.provider }" @click="selectGoogleSpeechModel(item)">
            <span><Mic2 :size="19" /></span><div><strong>{{ item.title }}<i>{{ item.badge }}</i></strong><code>{{ item.model }}</code><small>{{ item.description }}</small></div><b></b>
          </button>
        </div>
        <div class="google-config-grid">
          <label v-if="['google_gemini_speech', 'google_vertex_gemini_speech'].includes(drafts.TTS_PROVIDER)" class="config-edit-field">
            <span class="config-field-head"><strong>{{ drafts.TTS_PROVIDER === 'google_vertex_gemini_speech' ? 'Vertex AI Gemini TTS 模型' : 'Gemini Speech 模型' }}</strong><i v-if="fieldsByKey.TTS_MODEL?.has_managed_override">待重启覆盖</i></span>
            <code>TTS_MODEL</code>
            <select v-model="drafts.TTS_MODEL" class="select-control"><option v-for="model in drafts.TTS_PROVIDER === 'google_vertex_gemini_speech' ? vertexSpeechModelOptions : geminiSpeechModelOptions" :key="model" :value="model">{{ optionLabels[model] }}</option></select>
            <small>{{ drafts.TTS_PROVIDER === 'google_vertex_gemini_speech' ? '3.1 Flash TTS 使用 global；2.5 系列可按合规与容量需求选择区域。' : '使用 Gemini API Key 和 Interactions API；Preview 模型频率限制通常更严格。' }}</small>
          </label>
          <label v-else-if="drafts.TTS_PROVIDER === 'google_chirp3_hd'" class="config-edit-field">
            <span class="config-field-head"><strong>Chirp 模型</strong><i v-if="fieldsByKey.TTS_MODEL?.has_managed_override">待重启覆盖</i></span>
            <code>TTS_MODEL</code><input v-model="drafts.TTS_MODEL" type="text" readonly /><small>Chirp 3 HD 的具体语言和音色由完整声线 ID 决定。</small>
          </label>
          <label v-if="drafts.TTS_PROVIDER === 'google_vertex_gemini_speech'" class="config-edit-field">
            <span class="config-field-head"><strong>Vertex AI Project ID</strong><i v-if="fieldsByKey.TTS_VERTEX_PROJECT_ID?.has_managed_override">待重启覆盖</i></span>
            <code>TTS_VERTEX_PROJECT_ID</code><input v-model="drafts.TTS_VERTEX_PROJECT_ID" type="text" placeholder="可留空，从服务账号自动读取" spellcheck="false" />
            <small>填写 Google Cloud 项目 ID，不是项目显示名称。</small>
          </label>
          <label v-if="drafts.TTS_PROVIDER === 'google_vertex_gemini_speech'" class="config-edit-field">
            <span class="config-field-head"><strong>Vertex AI TTS 区域</strong><i v-if="fieldsByKey.TTS_VERTEX_LOCATION?.has_managed_override">待重启覆盖</i></span>
            <code>TTS_VERTEX_LOCATION</code><select v-model="drafts.TTS_VERTEX_LOCATION" class="select-control"><option v-for="location in fieldsByKey.TTS_VERTEX_LOCATION?.options || ['global']" :key="location" :value="location">{{ location }}</option></select>
            <small>Gemini 3.1 Flash TTS 请选择 global。</small>
          </label>
          <label v-if="['google_gemini_speech', 'google_vertex_gemini_speech', 'google_chirp3_hd'].includes(drafts.TTS_PROVIDER)" class="config-edit-field">
            <span class="config-field-head"><strong>默认旁白声线</strong><i v-if="fieldsByKey.DEFAULT_NARRATOR_VOICE_ID?.has_managed_override">待重启覆盖</i></span>
            <code>DEFAULT_NARRATOR_VOICE_ID</code><input v-model="drafts.DEFAULT_NARRATOR_VOICE_ID" type="text" :placeholder="drafts.TTS_PROVIDER === 'google_chirp3_hd' ? 'cmn-CN-Chirp3-HD-Kore' : 'Kore'" spellcheck="false" />
            <small>角色声线仍可在声音档案审核时分别填写；Gemini 使用短声线名，Chirp 使用完整 locale-model-voice ID。</small>
          </label>
        </div>
        <div v-if="drafts.TTS_PROVIDER === 'google_vertex_gemini_speech'" class="config-notice managed-config-notice"><ShieldCheck :size="18" /><div><strong>Vertex AI 服务账号</strong><p>与 Google 视频模型共用 VEO_SERVICE_ACCOUNT_JSON；服务账号至少需要 Vertex AI User（aiplatform.endpoints.predict）权限，并在项目中启用 Vertex AI API 与结算。</p></div><span>{{ googleCredential?.configured || googleCredential?.managed_override_configured ? '当前已配置' : '尚未配置' }}</span></div>
        <div v-else class="config-notice managed-config-notice"><ShieldCheck :size="18" /><div><strong>共用语音 API Key</strong><p>Gemini Developer API 与 Chirp 3 HD 使用上方语音合成卡片的 TTS_API_KEY；密钥只通过请求头发送。</p></div><span>{{ secretsByKey.TTS_API_KEY?.configured ? '当前已配置' : '尚未配置' }}</span></div>
      </article>

      <article v-for="group in textModelCategories" v-show="activeModelSection === 'text'" :key="`text-${group.name}`" class="panel padded ai-config-group">
        <div class="section-title"><div><span>MODELS & PROVIDERS</span><h3>{{ group.name }}</h3></div><div class="section-icon"><Bot :size="19" /></div></div>
        <div class="config-field-grid">
          <label v-for="field in group.fields" :key="field.key" class="config-edit-field" :class="{ dangerous: field.key === 'ALLOW_REAL_PUBLISH' }">
            <span class="config-field-head"><strong>{{ field.label }}</strong><i v-if="field.has_managed_override">待重启覆盖</i></span>
            <code>{{ field.key }}</code>
            <select v-if="field.kind === 'boolean'" v-model="drafts[field.key]" class="select-control"><option value="true">true</option><option value="false">false</option></select>
            <select v-else-if="field.kind === 'select'" v-model="drafts[field.key]" class="select-control"><option v-for="option in field.options" :key="option" :value="option">{{ option }}</option></select>
            <template v-else>
              <input v-model="drafts[field.key]" :type="field.kind === 'url' ? 'url' : 'text'" :list="field.kind === 'suggest' ? `options-${field.key}` : undefined" :placeholder="field.allow_empty ? '留空表示禁用' : '请输入配置值'" spellcheck="false" />
              <datalist v-if="field.kind === 'suggest'" :id="`options-${field.key}`"><option v-for="option in field.options" :key="option" :value="option" :label="optionLabels[option] || option" /></datalist>
            </template>
            <small>{{ field.description || '当前容器：' }}<b v-if="!field.description">{{ displayCurrent(field) }}</b></small>
          </label>
        </div>
      </article>

      <article v-for="group in imageModelCategories" v-show="activeModelSection === 'image'" :key="`image-${group.name}`" class="panel padded ai-config-group">
        <div class="section-title"><div><span>IMAGE MODELS & PROVIDERS</span><h3>{{ group.name }}</h3></div><div class="section-icon"><Image :size="19" /></div></div>
        <div class="config-field-grid">
          <label v-for="field in group.fields" :key="field.key" class="config-edit-field">
            <span class="config-field-head"><strong>{{ field.label }}</strong><i v-if="field.has_managed_override">待重启覆盖</i></span>
            <code>{{ field.key }}</code>
            <select v-if="field.kind === 'boolean'" v-model="drafts[field.key]" class="select-control"><option value="true">true</option><option value="false">false</option></select>
            <select v-else-if="field.kind === 'select'" v-model="drafts[field.key]" class="select-control"><option v-for="option in field.options" :key="option" :value="option">{{ option }}</option></select>
            <template v-else>
              <input v-model="drafts[field.key]" :type="field.kind === 'url' ? 'url' : 'text'" :list="field.kind === 'suggest' ? `options-${field.key}` : undefined" :placeholder="field.allow_empty ? '留空表示禁用' : '请输入配置值'" spellcheck="false" />
              <datalist v-if="field.kind === 'suggest'" :id="`options-${field.key}`"><option v-for="option in field.options" :key="option" :value="option" :label="optionLabels[option] || option" /></datalist>
            </template>
            <small>{{ field.description || '当前容器：' }}<b v-if="!field.description">{{ displayCurrent(field) }}</b></small>
          </label>
        </div>
      </article>
      </div>

      <div v-show="activeTab === 'operations'" id="ai-config-panel-operations" class="ai-config-tab-panel" role="tabpanel" aria-labelledby="ai-config-tab-operations">
        <article v-for="group in operationsCategories" :key="group.name" class="panel padded ai-config-group">
          <div class="section-title"><div><span>RUNTIME & PUBLISHING</span><h3>{{ group.name }}</h3></div><div class="section-icon"><Rocket :size="19" /></div></div>
          <div class="config-field-grid">
            <label v-for="field in group.fields" :key="field.key" class="config-edit-field" :class="{ dangerous: field.key === 'ALLOW_REAL_PUBLISH' }">
              <span class="config-field-head"><strong>{{ field.label }}</strong><i v-if="field.has_managed_override">待重启覆盖</i></span>
              <code>{{ field.key }}</code>
              <select v-if="field.kind === 'boolean'" v-model="drafts[field.key]" class="select-control"><option value="true">true</option><option value="false">false</option></select>
              <select v-else-if="field.kind === 'select'" v-model="drafts[field.key]" class="select-control"><option v-for="option in field.options" :key="option" :value="option">{{ option }}</option></select>
              <template v-else>
                <input v-model="drafts[field.key]" :type="field.kind === 'url' ? 'url' : 'text'" :placeholder="field.allow_empty ? '留空表示禁用' : '请输入配置值'" spellcheck="false" />
              </template>
              <small>{{ field.description || '当前容器：' }}<b v-if="!field.description">{{ displayCurrent(field) }}</b></small>
            </label>
          </div>
        </article>
      </div>

      <div v-show="activeTab === 'advanced'" id="ai-config-panel-advanced" class="ai-config-tab-panel" role="tabpanel" aria-labelledby="ai-config-tab-advanced">
      <article v-if="advancedSecrets.length" class="panel padded secret-config-panel">
        <div class="section-title"><div><span>ADVANCED CREDENTIALS</span><h3>其他敏感配置</h3></div><div class="section-icon"><KeyRound :size="19" /></div></div>
        <p class="secret-intro">这些密钥用于 LiteLLM 上游或发布接口。输入框始终为空，填写后只覆盖托管配置。</p>
        <div class="secret-field-grid">
          <label v-for="secret in advancedSecrets" :key="secret.key" class="secret-edit-field">
            <div><strong>{{ secret.label }}</strong><span :class="{ configured: secret.configured }">当前容器：{{ secret.configured ? '已配置' : '未配置' }}</span><span v-if="secret.managed_override_configured" class="pending-secret">托管文件：已填写，待重启</span></div>
            <code>{{ secret.key }}</code>
            <input v-model="secretDrafts[secret.key]" type="password" autocomplete="new-password" placeholder="留空不修改；输入新值将覆盖" spellcheck="false" />
          </label>
        </div>
      </article>
      </div>

      <div class="config-footer-actions"><span><ShieldCheck :size="15" />保存内容仅限白名单配置；密钥不会进入前端响应。</span><button class="button button-secondary" :disabled="!changeCount || saving" @click="resetDrafts"><RotateCcw :size="15" />放弃修改</button><button class="button button-primary" :disabled="!changeCount || saving" @click="save"><Save :size="15" />保存到 cms-managed.env</button></div>
    </template>
  </section>
</template>

<style scoped>
.restart-command-toggle {
  width: max-content;
  margin-top: 9px;
  padding: 5px 0;
  display: flex;
  align-items: center;
  gap: 5px;
  border: 0;
  color: #87632e;
  background: transparent;
  font: inherit;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.restart-command-toggle svg { transition: transform .18s; }
.restart-command-toggle.expanded svg { transform: rotate(180deg); }
.ai-config-tabs {
  position: sticky;
  top: 94px;
  z-index: 8;
  padding: 7px;
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 5px;
  border: 1px solid #dce2ec;
  border-radius: 12px;
  background: rgba(255,255,255,.95);
  box-shadow: 0 8px 24px rgba(31,43,68,.08);
  backdrop-filter: blur(12px);
}
.ai-config-tabs button {
  min-width: 0;
  min-height: 54px;
  padding: 8px 10px;
  display: flex;
  align-items: center;
  gap: 9px;
  border: 1px solid transparent;
  border-radius: 8px;
  color: #758197;
  background: transparent;
  text-align: left;
  cursor: pointer;
  transition: .16s;
}
.ai-config-tabs button:hover { color: #5066b3; background: #f5f7fc; }
.ai-config-tabs button.active {
  border-color: #d6def7;
  color: #405dbb;
  background: #edf2ff;
  box-shadow: 0 1px 3px rgba(41,59,106,.06);
}
.ai-config-tabs button > svg { flex: 0 0 auto; }
.ai-config-tabs button > span { min-width: 0; display: grid; gap: 2px; }
.ai-config-tabs button strong { font-size: 13px; }
.ai-config-tabs button small {
  overflow: hidden;
  color: #9aa3b2;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ai-config-tabs button.active small { color: #7181b7; }
.ai-config-tabs button > i {
  min-width: 20px;
  height: 20px;
  margin-left: auto;
  padding: 0 6px;
  display: grid;
  place-items: center;
  border-radius: 10px;
  color: #fff;
  background: #6079d6;
  font-size: 11px;
  font-style: normal;
  font-weight: 700;
}
.ai-config-tab-panel { display: grid; gap: 16px; }
.config-source-panel { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.config-overview-panel { display: grid; gap: 16px; }
.overview-strategy {
  padding: 15px;
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr) auto;
  align-items: center;
  gap: 13px;
  border: 1px solid #dce4f5;
  border-radius: 10px;
  background: linear-gradient(135deg,#f6f8ff,#fff);
}
.overview-strategy > div > span,
.overview-strategy > div > strong,
.overview-strategy > div > p { display: block; }
.overview-strategy > div > span { color: #8d98aa; font-size: 12px; }
.overview-strategy > div > strong { margin-top: 3px; color: #34425d; font-size: 15px; }
.overview-strategy > div > p { margin: 5px 0 0; color: #778397; font-size: 13px; line-height: 1.55; }
.overview-capability-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
}
.overview-capability-card {
  min-width: 0;
  padding: 14px;
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr);
  gap: 10px;
  border: 1px solid #e3e7ee;
  border-radius: 10px;
  color: #354057;
  background: #fbfcfe;
  text-align: left;
  cursor: pointer;
  transition: .16s;
}
.overview-capability-card:hover {
  border-color: #bac8f2;
  background: #f7f9ff;
  transform: translateY(-1px);
}
.overview-capability-card > span {
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  border-radius: 9px;
  color: #536bc0;
  background: #edf1ff;
}
.overview-capability-card > div { min-width: 0; }
.overview-capability-card strong,
.overview-capability-card small,
.overview-capability-card code { display: block; }
.overview-capability-card strong { font-size: 13px; }
.overview-capability-card small { margin-top: 3px; color: #7383a3; font-size: 12px; }
.overview-capability-card code {
  margin-top: 7px;
  overflow: hidden;
  color: #7d899c;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.overview-capability-card > i {
  grid-column: 2;
  width: max-content;
  color: #5670c4;
  font-size: 12px;
  font-style: normal;
  font-weight: 600;
}
.model-section-tabs {
  padding: 5px;
  display: flex;
  gap: 5px;
  border: 1px solid #dfe4ed;
  border-radius: 10px;
  background: #f7f8fb;
}
.model-section-tabs button {
  min-width: 105px;
  height: 40px;
  padding: 0 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  border: 1px solid transparent;
  border-radius: 7px;
  color: #758197;
  background: transparent;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.model-section-tabs button:hover { color: #536bbc; background: #fff; }
.model-section-tabs button.active {
  border-color: #d7def3;
  color: #455fba;
  background: #fff;
  box-shadow: 0 2px 8px rgba(42,57,91,.07);
}
@media (max-width: 1100px) {
  .ai-config-tabs button small { display: none; }
  .overview-capability-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .config-source-panel { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .config-source-panel > div:nth-child(2) { border-right: 0; }
  .config-source-panel > div:nth-child(-n+2) { border-bottom: 1px solid #eaedf2; }
}
@media (max-width: 780px) {
  .ai-config-tabs {
    top: 80px;
    display: flex;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .ai-config-tabs::-webkit-scrollbar { display: none; }
  .ai-config-tabs button { min-width: 142px; }
  .ai-config-tabs button small { display: block; }
  .overview-strategy { grid-template-columns: 40px minmax(0, 1fr); }
  .overview-strategy .button { grid-column: 1 / -1; width: 100%; justify-content: center; }
  .model-section-tabs { overflow-x: auto; }
  .model-section-tabs button { flex: 1; }
}
@media (max-width: 600px) {
  .overview-capability-grid,
  .config-source-panel { grid-template-columns: 1fr; }
  .config-source-panel > div {
    border-right: 0 !important;
    border-bottom: 1px solid #eaedf2;
  }
  .config-source-panel > div:last-child { border-bottom: 0; }
  .model-section-tabs button { min-width: 88px; }
  .ai-config-tabs { margin-right: -4px; margin-left: -4px; }
}
</style>
