<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  AlertTriangle, Bot, Boxes, Building2, Cable, Container, FileCog, Globe2,
  Image, KeyRound, Mic2, Network, RefreshCw, RotateCcw, Save, ShieldCheck, Video, Waypoints,
} from 'lucide-vue-next'
import { api } from '../services/api'

const data = ref(null)
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const result = ref(null)
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
}
const defaultPlan = {
  AI_CONNECTION_MODE: 'hybrid', TEXT_API_SOURCE: 'gateway', IMAGE_API_SOURCE: 'native',
  VIDEO_API_SOURCE: 'native', TTS_API_SOURCE: 'native', VIDEO_USE_GENERATED_AUDIO: 'false',
  VEO_LOCATION: 'us-central1', VEO_OUTPUT_MODE: 'local',
}
const googleVideoModels = [
  { id: 'gemini-omni-flash-preview', title: 'Gemini Omni Flash', badge: '推荐', description: '3–10 秒、720p，速度快、角色一致性好；当前为 Preview。' },
  { id: 'veo-3.1-fast-generate-001', title: 'Veo 3.1 Fast', badge: '批量', description: '适合批量镜头生产，支持 4/6/8 秒与 720p/1080p。' },
  { id: 'veo-3.1-generate-001', title: 'Veo 3.1', badge: '质量', description: '适合重点镜头，生成速度与成本通常高于 Fast。' },
]
const googleVideoFieldKeys = new Set([
  'VIDEO_PROVIDER', 'VIDEO_MODEL', 'VIDEO_USE_GENERATED_AUDIO',
  'VEO_OUTPUT_MODE', 'VEO_PROJECT_ID', 'VEO_LOCATION', 'VEO_GCS_OUTPUT_URI',
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
    endpoint: 'POST {Base URL}/tts', note: '同步接口必须返回可下载的音频 URL，不能只返回 Base64 或二进制响应。',
  },
]
const routedFieldKeys = new Set([
  'AI_CONNECTION_MODE', ...capabilities.flatMap((item) => [item.sourceKey, item.baseKey]),
])
const routedSecretKeys = new Set(capabilities.map((item) => item.secretKey))

const fieldsByKey = computed(() => Object.fromEntries((data.value?.fields || []).map((field) => [field.key, field])))
const secretsByKey = computed(() => Object.fromEntries((data.value?.secrets || []).map((secret) => [secret.key, secret])))
const categories = computed(() => {
  const groups = []
  for (const field of data.value?.fields || []) {
    if (routedFieldKeys.has(field.key) || googleVideoFieldKeys.has(field.key)) continue
    let group = groups.find((item) => item.name === field.category)
    if (!group) {
      group = { name: field.category, fields: [] }
      groups.push(group)
    }
    group.fields.push(field)
  }
  return groups
})
const advancedSecrets = computed(() => (data.value?.secrets || []).filter((secret) => !routedSecretKeys.has(secret.key) && secret.key !== 'VEO_SERVICE_ACCOUNT_JSON'))
const googleCredential = computed(() => secretsByKey.value.VEO_SERVICE_ACCOUNT_JSON)
const changedValues = computed(() => Object.fromEntries(
  Object.keys(drafts).filter((key) => drafts[key] !== baseline[key]).map((key) => [key, drafts[key]]),
))
const pendingSecrets = computed(() => Object.fromEntries(
  Object.entries(secretDrafts).filter(([, value]) => value !== ''),
))
const changeCount = computed(() => Object.keys(changedValues.value).length + Object.keys(pendingSecrets.value).length)

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

    <div v-if="result" class="config-save-result"><AlertTriangle :size="20" /><div><strong>{{ result.message }}</strong><span>普通 restart 不会重新加载环境变量，请执行一次下方命令：</span><code>{{ result.restart_command }}</code></div></div>
    <div v-else-if="data?.pending_restart" class="config-save-result pending"><AlertTriangle :size="20" /><div><strong>CMS 托管文件与当前容器环境不同</strong><span>需要重建 n8n 与 Google 视频适配器后才会使用待生效值。</span><code>{{ data.restart_command }}</code></div></div>

    <div v-if="error" class="error-banner large">{{ error }} <button @click="load">重试</button></div>
    <div v-if="loading" class="config-loading"><span></span><span></span></div>
    <template v-else-if="data">
      <article class="panel config-source-panel">
        <div><Container :size="19" /><span>配置来源</span><strong>{{ data.source }}</strong><code>{{ data.container_name }}</code></div>
        <div><FileCog :size="19" /><span>容器状态</span><strong>{{ data.container_status }}</strong><code>{{ data.managed_file_exists ? '托管文件已创建' : '尚无托管文件' }}</code></div>
        <div><Video :size="19" /><span>Google 视频适配器</span><strong>{{ data.video_adapter_status }}</strong><code>{{ data.video_adapter_name }}</code></div>
        <div><ShieldCheck :size="19" /><span>密钥响应</span><strong>{{ data.secrets_exposed ? '风险：已暴露' : '已脱敏' }}</strong><code>boolean status only</code></div>
      </article>

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

      <article class="panel padded google-video-panel">
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
        <label class="google-credential-field">
          <div><strong>Google 服务账号 JSON</strong><span :class="{ configured: googleCredential?.configured || googleCredential?.managed_override_configured }">{{ googleCredential?.managed_override_configured ? '托管文件已填写，等待重启' : googleCredential?.configured ? '当前已配置' : '尚未配置' }}</span></div>
          <code>VEO_SERVICE_ACCOUNT_JSON</code>
          <textarea v-model="secretDrafts.VEO_SERVICE_ACCOUNT_JSON" rows="8" autocomplete="off" :placeholder="googleCredential?.configured || googleCredential?.managed_override_configured ? '已安全保存；留空不修改。需要更换时粘贴新的完整 JSON。' : '粘贴从 Google Cloud 下载的完整服务账号 JSON'" spellcheck="false"></textarea>
          <small><ShieldCheck :size="14" />JSON 只写入权限受限的托管配置，页面与接口永不返回私钥内容。</small>
        </label>
      </article>

      <article v-for="group in categories" :key="group.name" class="panel padded ai-config-group">
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

      <div class="config-footer-actions"><span><ShieldCheck :size="15" />保存内容仅限白名单配置；密钥不会进入前端响应。</span><button class="button button-secondary" :disabled="!changeCount || saving" @click="resetDrafts"><RotateCcw :size="15" />放弃修改</button><button class="button button-primary" :disabled="!changeCount || saving" @click="save"><Save :size="15" />保存到 cms-managed.env</button></div>
    </template>
  </section>
</template>
