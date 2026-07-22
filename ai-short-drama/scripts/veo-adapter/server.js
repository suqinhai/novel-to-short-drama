'use strict'

const crypto = require('crypto')
const fs = require('fs')
const fsp = fs.promises
const http = require('http')
const path = require('path')
const { Readable } = require('stream')
const {
  appendGcsPrefix,
  assertAllowedImageUrl,
  buildVeoParameters,
  clampInteger,
  createServiceAccountJwt,
  decodeBase64Video,
  extractInteractionVideoOutputs,
  extractVideoOutputs,
  localVideoRef,
  modelFamily,
  normalizeAspectRatio,
  normalizeDuration,
  normalizeModel,
  normalizePipelineResolution,
  normalizeResolution,
  parseByteRange,
  parseGcsUri,
  parseLocalVideoRef,
  requestFingerprint,
  rewriteImageUrl,
  safeEqual,
  signMediaUrl,
  sniffImageMime,
  taskIdForIdempotency,
} = require('./lib')

const configuredOutputUri = String(process.env.VEO_GCS_OUTPUT_URI || '').trim()
const requestedOutputMode = String(process.env.VEO_OUTPUT_MODE || 'auto').trim().toLowerCase()
const outputMode = requestedOutputMode === 'auto'
  ? (configuredOutputUri ? 'gcs' : 'local')
  : requestedOutputMode

const config = {
  port: clampInteger(process.env.VEO_ADAPTER_PORT, 8091, 1, 65535),
  apiKey: String(process.env.VIDEO_API_KEY || process.env.VEO_ADAPTER_API_KEY || ''),
  credentialFile: String(process.env.VEO_SERVICE_ACCOUNT_FILE || '/run/secrets/veo-service-account.json'),
  credentialJson: String(process.env.VEO_SERVICE_ACCOUNT_JSON || ''),
  projectId: String(process.env.VEO_PROJECT_ID || '').trim(),
  location: String(process.env.VEO_LOCATION || 'us-central1').trim(),
  defaultModel: String(process.env.VEO_DEFAULT_MODEL || 'veo-3.1-fast-generate-001').trim(),
  outputMode,
  outputUri: configuredOutputUri,
  localOutputDir: String(process.env.VEO_LOCAL_OUTPUT_DIR || '/data/videos'),
  localVideoMaxBytes: clampInteger(process.env.VEO_LOCAL_VIDEO_MAX_MB, 128, 1, 512) * 1024 * 1024,
  taskDir: String(process.env.VEO_TASK_DIR || '/data/tasks'),
  publicBaseUrl: String(process.env.VEO_ADAPTER_PUBLIC_BASE_URL || '').replace(/\/+$/, ''),
  imageRewriteFrom: String(process.env.VEO_IMAGE_URL_REWRITE_FROM || '').replace(/\/+$/, ''),
  imageRewriteTo: String(process.env.VEO_IMAGE_URL_REWRITE_TO || '').replace(/\/+$/, ''),
  imageAllowedHosts: String(process.env.VEO_IMAGE_ALLOWED_HOSTS || 'media,localhost,127.0.0.1,host.docker.internal')
    .split(',').map((item) => item.trim()).filter(Boolean),
  imageMaxBytes: clampInteger(process.env.VEO_IMAGE_MAX_MB, 20, 1, 20) * 1024 * 1024,
  requestMaxBytes: clampInteger(process.env.VEO_REQUEST_MAX_BYTES, 1048576, 1024, 10485760),
  personGeneration: String(process.env.VEO_PERSON_GENERATION || 'allow_adult'),
  enhancePrompt: String(process.env.VEO_ENHANCE_PROMPT || 'true').toLowerCase() !== 'false',
  resizeMode: String(process.env.VEO_RESIZE_MODE || 'crop'),
  mediaUrlTtlSeconds: clampInteger(process.env.VEO_MEDIA_URL_TTL_SECONDS, 86400, 600, 604800),
  vertexApiOrigin: String(process.env.VEO_VERTEX_API_ORIGIN || '').replace(/\/+$/, ''),
}

class HttpError extends Error {
  constructor(statusCode, message, code = 'VEO_ADAPTER_ERROR') {
    super(message)
    this.statusCode = statusCode
    this.code = code
  }
}

class TaskStore {
  constructor(directory) {
    this.directory = directory
  }

  async init() {
    await fsp.mkdir(this.directory, { recursive: true, mode: 0o700 })
  }

  taskPath(taskId) {
    if (!/^veo_[a-f0-9]{24}$/.test(taskId)) throw new HttpError(400, 'invalid task id', 'INVALID_TASK_ID')
    return path.join(this.directory, `${taskId}.json`)
  }

  async get(taskId) {
    try {
      return JSON.parse(await fsp.readFile(this.taskPath(taskId), 'utf8'))
    } catch (error) {
      if (error.code === 'ENOENT') return null
      throw error
    }
  }

  async save(task) {
    const target = this.taskPath(task.task_id)
    const temporary = `${target}.${process.pid}.${crypto.randomUUID()}.tmp`
    await fsp.writeFile(temporary, `${JSON.stringify(task, null, 2)}\n`, { mode: 0o600 })
    await fsp.rename(temporary, target)
  }
}

class GoogleServiceAccountAuth {
  constructor(credentialFile, credentialJson) {
    this.credentialFile = credentialFile
    this.credentialJson = credentialJson
    this.serviceAccount = null
    this.cachedToken = null
    this.tokenExpiresAt = 0
  }

  async load() {
    if (this.serviceAccount) return this.serviceAccount
    let raw = this.credentialJson
    if (!raw) {
      try {
        raw = await fsp.readFile(this.credentialFile, 'utf8')
      } catch (error) {
        throw new HttpError(503, `cannot read Google service account JSON: ${error.message}`, 'GOOGLE_CREDENTIAL_UNAVAILABLE')
      }
    }
    let parsed
    try {
      parsed = JSON.parse(raw)
    } catch {
      throw new HttpError(503, 'Google service account JSON is invalid', 'GOOGLE_CREDENTIAL_INVALID')
    }
    createServiceAccountJwt(parsed)
    this.serviceAccount = parsed
    return parsed
  }

  async projectId() {
    const serviceAccount = await this.load()
    return config.projectId || String(serviceAccount.project_id || '').trim()
  }

  async accessToken() {
    if (this.cachedToken && Date.now() < this.tokenExpiresAt - 300000) return this.cachedToken
    const serviceAccount = await this.load()
    const assertion = createServiceAccountJwt(serviceAccount)
    const body = new URLSearchParams({
      grant_type: 'urn:ietf:params:oauth:grant-type:jwt-bearer',
      assertion,
    })
    const response = await fetch('https://oauth2.googleapis.com/token', {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded' },
      body,
      signal: AbortSignal.timeout(15000),
    })
    const payload = await response.json().catch(() => ({}))
    if (!response.ok || !payload.access_token) {
      throw new HttpError(502, `Google OAuth token exchange failed (${response.status}): ${payload.error_description || payload.error || 'unknown error'}`, 'VEO_GOOGLE_AUTH_FAILED')
    }
    this.cachedToken = payload.access_token
    this.tokenExpiresAt = Date.now() + Number(payload.expires_in || 3600) * 1000
    return this.cachedToken
  }
}

const taskStore = new TaskStore(config.taskDir)
const googleAuth = new GoogleServiceAccountAuth(config.credentialFile, config.credentialJson)
const submissionLocks = new Map()
const pollLocks = new Map()

function sendJson(response, statusCode, payload) {
  const body = Buffer.from(JSON.stringify(payload))
  response.writeHead(statusCode, {
    'content-type': 'application/json; charset=utf-8',
    'content-length': body.length,
    'cache-control': 'no-store',
  })
  response.end(body)
}

async function readJson(request) {
  const chunks = []
  let size = 0
  for await (const chunk of request) {
    size += chunk.length
    if (size > config.requestMaxBytes) throw new HttpError(413, 'request body is too large', 'REQUEST_TOO_LARGE')
    chunks.push(chunk)
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString('utf8'))
  } catch {
    throw new HttpError(400, 'request body must be valid JSON', 'INVALID_JSON')
  }
}

function requireApiKey(request) {
  if (!config.apiKey) throw new HttpError(503, 'video adapter API key is not configured', 'VIDEO_ADAPTER_NOT_CONFIGURED')
  const authorization = String(request.headers.authorization || '')
  const supplied = authorization.toLowerCase().startsWith('bearer ') ? authorization.slice(7).trim() : ''
  if (!safeEqual(supplied, config.apiKey)) throw new HttpError(401, 'invalid adapter API key', 'UNAUTHORIZED')
}

async function fetchGoogleJson(url, options = {}) {
  const accessToken = await googleAuth.accessToken()
  const response = await fetch(url, {
    ...options,
    headers: {
      authorization: `Bearer ${accessToken}`,
      'content-type': 'application/json; charset=utf-8',
      ...(options.headers || {}),
    },
    signal: AbortSignal.timeout(120000),
  })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    const message = payload.error?.message || payload.message || `Vertex API returned HTTP ${response.status}`
    throw new HttpError(response.status === 429 ? 429 : 502, String(message).slice(0, 2000), payload.error?.status || 'VEO_VERTEX_ERROR')
  }
  return payload
}

function vertexApiOrigin(location = config.location) {
  if (config.vertexApiOrigin) return config.vertexApiOrigin
  return location === 'global'
    ? 'https://aiplatform.googleapis.com'
    : `https://${location}-aiplatform.googleapis.com`
}

async function uploadGcsObject(uri, data, mimeType) {
  const { bucket, object } = parseGcsUri(uri)
  const accessToken = await googleAuth.accessToken()
  const url = `https://storage.googleapis.com/upload/storage/v1/b/${encodeURIComponent(bucket)}/o?uploadType=media&name=${encodeURIComponent(object)}`
  const response = await fetch(url, {
    method: 'POST',
    headers: { authorization: `Bearer ${accessToken}`, 'content-type': mimeType },
    body: data,
    signal: AbortSignal.timeout(120000),
  })
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}))
    throw new HttpError(502, String(payload.error?.message || `GCS upload failed with HTTP ${response.status}`).slice(0, 2000), 'GOOGLE_GCS_UPLOAD_FAILED')
  }
}

async function readLimitedResponse(response, limit) {
  const declared = Number(response.headers.get('content-length') || 0)
  if (declared > limit) throw new HttpError(413, 'input image exceeds the configured size limit', 'VEO_IMAGE_TOO_LARGE')
  const reader = response.body.getReader()
  const chunks = []
  let size = 0
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    size += value.byteLength
    if (size > limit) {
      await reader.cancel()
      throw new HttpError(413, 'input image exceeds the configured size limit', 'VEO_IMAGE_TOO_LARGE')
    }
    chunks.push(Buffer.from(value))
  }
  return Buffer.concat(chunks)
}

async function downloadInputImage(rawUrl) {
  let current = rewriteImageUrl(rawUrl, config.imageRewriteFrom, config.imageRewriteTo)
  for (let redirects = 0; redirects <= 3; redirects += 1) {
    const parsed = assertAllowedImageUrl(current, config.imageAllowedHosts)
    const response = await fetch(parsed, {
      redirect: 'manual',
      signal: AbortSignal.timeout(30000),
      headers: { accept: 'image/png,image/jpeg' },
    })
    if ([301, 302, 303, 307, 308].includes(response.status)) {
      const location = response.headers.get('location')
      if (!location) throw new HttpError(502, 'image server returned an empty redirect', 'VEO_IMAGE_DOWNLOAD_FAILED')
      current = new URL(location, parsed).toString()
      continue
    }
    if (!response.ok || !response.body) {
      throw new HttpError(502, `input image download failed with HTTP ${response.status}`, 'VEO_IMAGE_DOWNLOAD_FAILED')
    }
    const data = await readLimitedResponse(response, config.imageMaxBytes)
    return { data, mimeType: sniffImageMime(data, response.headers.get('content-type') || '') }
  }
  throw new HttpError(502, 'input image redirected too many times', 'VEO_IMAGE_DOWNLOAD_FAILED')
}

function normalizeGenerateRequest(body) {
  if (!body || typeof body !== 'object' || Array.isArray(body)) throw new HttpError(400, 'request must be an object', 'INVALID_REQUEST')
  const prompt = String(body.prompt || '').trim()
  const imageUrl = String(body.image_url || '').trim()
  if (!prompt) throw new HttpError(400, 'prompt is required', 'VEO_PROMPT_REQUIRED')
  if (!imageUrl) throw new HttpError(400, 'image_url is required', 'VEO_IMAGE_REQUIRED')
  if (prompt.length > 12000) throw new HttpError(400, 'prompt is too long', 'VEO_PROMPT_TOO_LONG')
  let model
  let aspectRatio
  let resolution
  try {
    model = normalizeModel(body.model, config.defaultModel)
    aspectRatio = normalizeAspectRatio(body.aspect_ratio)
    resolution = normalizeResolution(body.resolution, body.width, body.height)
  } catch (error) {
    throw new HttpError(400, error.message, 'VEO_REQUEST_INVALID')
  }
  if (modelFamily(model) === 'omni') resolution = '720p'
  const pipelineResolution = normalizePipelineResolution(modelFamily(model) === 'omni' ? '' : body.resolution, aspectRatio, resolution)
  const durationSeconds = normalizeDuration(body.duration_seconds, model)
  const fps = 24
  const negativePrompt = String(body.negative_prompt || '').trim().slice(0, 4000)
  const seedNumber = Number(body.seed)
  const seed = Number.isInteger(seedNumber) && seedNumber >= 0 && seedNumber <= 4294967295 ? seedNumber : undefined
  return {
    model, prompt, image_url: imageUrl, negative_prompt: negativePrompt,
    duration_seconds: durationSeconds, aspect_ratio: aspectRatio,
    resolution, pipeline_resolution: pipelineResolution, fps,
    expected_audio: body.expected_audio === true,
    ...(seed === undefined ? {} : { seed }),
  }
}

async function validateGoogleProject() {
  const projectId = await googleAuth.projectId()
  if (!projectId) throw new HttpError(503, 'Google project ID is not configured', 'GOOGLE_PROJECT_REQUIRED')
  if (!/^[-a-z0-9]{4,63}$/.test(projectId)) throw new HttpError(503, 'Google project ID is invalid', 'GOOGLE_PROJECT_INVALID')
  return projectId
}

async function materializeVideoOutputs(taskId, outputs, mode = config.outputMode) {
  if (mode === 'gcs') {
    return outputs.map((item) => item.uri).filter((uri) => String(uri).startsWith('gs://'))
  }
  if (mode !== 'local') {
    throw new HttpError(503, 'VEO_OUTPUT_MODE must be local or gcs', 'VEO_OUTPUT_MODE_INVALID')
  }
  const inline = outputs.filter((item) => item.data)
  if (!inline.length) return []
  const taskDirectory = path.join(config.localOutputDir, taskId)
  await fsp.mkdir(taskDirectory, { recursive: true, mode: 0o700 })
  const videos = []
  for (const [index, output] of inline.slice(0, 4).entries()) {
    let data
    try {
      data = decodeBase64Video(output.data, config.localVideoMaxBytes)
    } catch (error) {
      throw new HttpError(502, error.message, 'VIDEO_INLINE_OUTPUT_INVALID')
    }
    const target = path.join(taskDirectory, `${index}.mp4`)
    const temporary = `${target}.${process.pid}.${crypto.randomUUID()}.tmp`
    await fsp.writeFile(temporary, data, { mode: 0o600, flag: 'wx' })
    await fsp.rename(temporary, target)
    videos.push(localVideoRef(taskId, index))
  }
  return videos
}

async function submitVeoTask(taskId, normalized, requestHash) {
  const image = await downloadInputImage(normalized.image_url)
  const projectId = await validateGoogleProject()
  if (!/^[-a-z0-9]+$/.test(config.location)) throw new HttpError(503, 'VEO_LOCATION is invalid', 'VEO_LOCATION_INVALID')
  const storageUri = config.outputMode === 'gcs' ? appendGcsPrefix(config.outputUri, taskId) : ''
  const parameters = buildVeoParameters(normalized, {
    storageUri,
    personGeneration: config.personGeneration,
    enhancePrompt: config.enhancePrompt,
    resizeMode: config.resizeMode,
  })
  const url = `${vertexApiOrigin()}/v1/projects/${encodeURIComponent(projectId)}/locations/${encodeURIComponent(config.location)}/publishers/google/models/${encodeURIComponent(normalized.model)}:predictLongRunning`
  const operation = await fetchGoogleJson(url, {
    method: 'POST',
    body: JSON.stringify({
      instances: [{
        prompt: normalized.prompt,
        image: { bytesBase64Encoded: image.data.toString('base64'), mimeType: image.mimeType },
      }],
      parameters,
    }),
  })
  if (!operation.name) throw new HttpError(502, 'Vertex did not return an operation name', 'VEO_OPERATION_MISSING')
  const now = new Date().toISOString()
  const task = {
    task_id: taskId,
    provider: 'vertex-veo',
    status: 'processing',
    progress: 0,
    operation_name: String(operation.name),
    request_hash: requestHash,
    request: normalized,
    output_mode: config.outputMode,
    storage_uri: storageUri || null,
    videos: [],
    created_at: now,
    updated_at: now,
  }
  await taskStore.save(task)
  return task
}

async function submitOmniTask(taskId, normalized, requestHash) {
  const image = await downloadInputImage(normalized.image_url)
  const projectId = await validateGoogleProject()
  const storageUri = config.outputMode === 'gcs' ? appendGcsPrefix(config.outputUri, taskId) : ''
  let imageInput
  let responseFormat
  if (storageUri) {
    const extension = image.mimeType === 'image/png' ? 'png' : 'jpg'
    const inputUri = `${storageUri}input.${extension}`
    const outputUri = `${storageUri}output/`
    await uploadGcsObject(inputUri, image.data, image.mimeType)
    imageInput = { type: 'image', uri: inputUri, mime_type: image.mimeType }
    responseFormat = {
      type: 'video', delivery: 'uri', gcs_uri: outputUri,
      aspect_ratio: normalized.aspect_ratio,
      duration: `${normalized.duration_seconds}s`,
    }
  } else {
    imageInput = { type: 'image', data: image.data.toString('base64'), mime_type: image.mimeType }
    responseFormat = {
      type: 'video', delivery: 'inline',
      aspect_ratio: normalized.aspect_ratio,
      duration: `${normalized.duration_seconds}s`,
    }
  }
  const prompt = normalized.negative_prompt
    ? `${normalized.prompt}\n\nAvoid these visual defects: ${normalized.negative_prompt}`
    : normalized.prompt
  const url = `${vertexApiOrigin('global')}/v1beta1/projects/${encodeURIComponent(projectId)}/locations/global/interactions`
  const interaction = await fetchGoogleJson(url, {
    method: 'POST',
    body: JSON.stringify({
      model: normalized.model,
      background: true,
      input: [
        imageInput,
        { type: 'text', text: prompt },
      ],
      response_format: [responseFormat],
      generation_config: { video_config: { task: 'image_to_video' } },
    }),
  })
  if (!interaction.id) throw new HttpError(502, 'Vertex did not return an interaction ID', 'OMNI_INTERACTION_MISSING')
  const now = new Date().toISOString()
  const outputs = extractInteractionVideoOutputs(interaction)
  const videos = await materializeVideoOutputs(taskId, outputs)
  const completed = String(interaction.status || '').toLowerCase() === 'completed' && videos.length > 0
  const task = {
    task_id: taskId,
    provider: 'vertex-omni',
    status: completed ? 'succeeded' : 'processing',
    progress: completed ? 100 : 0,
    interaction_id: String(interaction.id),
    request_hash: requestHash,
    request: normalized,
    output_mode: config.outputMode,
    storage_uri: storageUri || null,
    videos,
    created_at: now,
    updated_at: now,
    ...(completed ? { completed_at: now } : {}),
  }
  await taskStore.save(task)
  return task
}

async function submitVertexTask(taskId, normalized, requestHash) {
  return modelFamily(normalized.model) === 'omni'
    ? submitOmniTask(taskId, normalized, requestHash)
    : submitVeoTask(taskId, normalized, requestHash)
}

function withLock(map, key, work) {
  if (map.has(key)) return map.get(key)
  const promise = Promise.resolve().then(work).finally(() => map.delete(key))
  map.set(key, promise)
  return promise
}

function requestBaseUrl(request) {
  if (config.publicBaseUrl) return config.publicBaseUrl
  const protocol = String(request.headers['x-forwarded-proto'] || 'http').split(',')[0].trim()
  return `${protocol}://${request.headers.host}`
}

function mediaUrlFor(request, taskId, index) {
  const expiresAt = Math.floor(Date.now() / 1000) + config.mediaUrlTtlSeconds
  const signature = signMediaUrl(config.apiKey, taskId, index, expiresAt)
  return `${requestBaseUrl(request)}/media/${taskId}/${index}?expires=${expiresAt}&signature=${encodeURIComponent(signature)}`
}

function providerPayload(request, task) {
  if (task.status === 'failed') {
    return {
      status: 'failed', progress: 0, provider_task_id: task.task_id,
      error: { code: task.error_code || 'GOOGLE_VIDEO_FAILED', message: task.error_message || 'Google video generation failed' },
    }
  }
  if (task.status !== 'succeeded') {
    return { status: 'processing', progress: Number(task.progress || 0), provider_task_id: task.task_id }
  }
  const videos = task.videos.map((_, index) => ({
    url: mediaUrlFor(request, task.task_id, index),
    duration_seconds: task.request.duration_seconds,
    aspect_ratio: task.request.aspect_ratio,
    resolution: task.request.pipeline_resolution,
    fps: task.request.fps,
  }))
  return {
    status: 'succeeded', progress: 100, provider_task_id: task.task_id,
    video_url: videos[0]?.url || null, videos,
    usage: { video_count: videos.length, duration_seconds: task.request.duration_seconds },
  }
}

async function handleGenerate(request, response) {
  requireApiKey(request)
  if (request.method !== 'POST') throw new HttpError(405, 'method not allowed', 'METHOD_NOT_ALLOWED')
  const normalized = normalizeGenerateRequest(await readJson(request))
  const idempotencyKey = String(request.headers['idempotency-key'] || crypto.randomUUID()).trim()
  if (idempotencyKey.length > 512) throw new HttpError(400, 'Idempotency-Key is too long', 'INVALID_IDEMPOTENCY_KEY')
  const taskId = taskIdForIdempotency(idempotencyKey)
  const requestHash = requestFingerprint(normalized)
  let task = await taskStore.get(taskId)
  if (task && task.request_hash !== requestHash) throw new HttpError(409, 'Idempotency-Key is already bound to a different request', 'IDEMPOTENCY_CONFLICT')
  if (!task) {
    task = await withLock(submissionLocks, taskId, async () => {
      const existing = await taskStore.get(taskId)
      if (existing) return existing
      return submitVertexTask(taskId, normalized, requestHash)
    })
  }
  if (task.request_hash !== requestHash) throw new HttpError(409, 'Idempotency-Key is already bound to a different request', 'IDEMPOTENCY_CONFLICT')
  sendJson(response, 200, providerPayload(request, task))
}

async function refreshTask(task) {
  if (task.status !== 'processing') return task
  if (task.provider === 'vertex-omni') return refreshOmniTask(task)
  const projectId = await googleAuth.projectId()
  const model = task.request.model
  const url = `${vertexApiOrigin()}/v1/projects/${encodeURIComponent(projectId)}/locations/${encodeURIComponent(config.location)}/publishers/google/models/${encodeURIComponent(model)}:fetchPredictOperation`
  const operation = await fetchGoogleJson(url, {
    method: 'POST',
    body: JSON.stringify({ operationName: task.operation_name }),
  })
  const now = new Date().toISOString()
  if (!operation.done) {
    const next = { ...task, progress: Math.max(5, Number(operation.metadata?.progressPercent || task.progress || 0)), updated_at: now }
    await taskStore.save(next)
    return next
  }
  if (operation.error) {
    const failed = {
      ...task, status: 'failed', progress: 0,
      error_code: String(operation.error.status || operation.error.code || 'VEO_VERTEX_FAILED'),
      error_message: String(operation.error.message || 'Vertex Veo operation failed').slice(0, 2000),
      updated_at: now,
    }
    await taskStore.save(failed)
    return failed
  }
  const taskOutputMode = task.output_mode || config.outputMode
  const videos = await materializeVideoOutputs(task.task_id, extractVideoOutputs(operation), taskOutputMode)
  if (!videos.length) {
    const failed = {
      ...task, status: 'failed', progress: 0,
      error_code: 'VEO_RESULT_MISSING',
      error_message: `Vertex completed without a usable ${taskOutputMode === 'local' ? 'inline video' : 'GCS video URI'}`,
      updated_at: now,
    }
    await taskStore.save(failed)
    return failed
  }
  const completed = { ...task, status: 'succeeded', progress: 100, videos, updated_at: now, completed_at: now }
  await taskStore.save(completed)
  return completed
}

async function refreshOmniTask(task) {
  const projectId = await validateGoogleProject()
  const url = `${vertexApiOrigin('global')}/v1beta1/projects/${encodeURIComponent(projectId)}/locations/global/interactions/${encodeURIComponent(task.interaction_id)}`
  const interaction = await fetchGoogleJson(url, { method: 'GET' })
  const now = new Date().toISOString()
  const status = String(interaction.status || 'in_progress').toLowerCase()
  if (!['completed', 'failed', 'cancelled'].includes(status)) {
    const next = { ...task, progress: Math.max(5, Number(task.progress || 0)), updated_at: now }
    await taskStore.save(next)
    return next
  }
  if (status !== 'completed') {
    const failed = {
      ...task, status: 'failed', progress: 0,
      error_code: String(interaction.error?.code || `OMNI_${status.toUpperCase()}`),
      error_message: String(interaction.error?.message || `Gemini Omni interaction ${status}`).slice(0, 2000),
      updated_at: now,
    }
    await taskStore.save(failed)
    return failed
  }
  const taskOutputMode = task.output_mode || config.outputMode
  const videos = await materializeVideoOutputs(task.task_id, extractInteractionVideoOutputs(interaction), taskOutputMode)
  if (!videos.length) {
    const failed = {
      ...task, status: 'failed', progress: 0,
      error_code: 'OMNI_RESULT_MISSING',
      error_message: `Gemini Omni completed without a usable ${taskOutputMode === 'local' ? 'inline video' : 'GCS video URI'}`,
      updated_at: now,
    }
    await taskStore.save(failed)
    return failed
  }
  const completed = { ...task, status: 'succeeded', progress: 100, videos, updated_at: now, completed_at: now }
  await taskStore.save(completed)
  return completed
}

async function handleTask(request, response, taskId) {
  requireApiKey(request)
  if (request.method !== 'GET') throw new HttpError(405, 'method not allowed', 'METHOD_NOT_ALLOWED')
  let task = await taskStore.get(taskId)
  if (!task) throw new HttpError(404, 'task not found', 'TASK_NOT_FOUND')
  task = await withLock(pollLocks, taskId, async () => refreshTask(await taskStore.get(taskId)))
  sendJson(response, 200, providerPayload(request, task))
}

async function serveLocalVideo(request, response, taskId, index, reference) {
  let parsed
  try {
    parsed = parseLocalVideoRef(reference)
  } catch {
    throw new HttpError(404, 'video not found', 'VIDEO_NOT_FOUND')
  }
  if (parsed.taskId !== taskId || parsed.index !== index) {
    throw new HttpError(404, 'video not found', 'VIDEO_NOT_FOUND')
  }
  const target = path.join(config.localOutputDir, taskId, `${index}.mp4`)
  let stat
  try {
    stat = await fsp.stat(target)
  } catch (error) {
    if (error.code === 'ENOENT') throw new HttpError(404, 'video not found', 'VIDEO_NOT_FOUND')
    throw error
  }
  if (!stat.isFile()) throw new HttpError(404, 'video not found', 'VIDEO_NOT_FOUND')
  let range
  try {
    range = parseByteRange(request.headers.range, stat.size)
  } catch {
    response.writeHead(416, { 'content-range': `bytes */${stat.size}`, 'accept-ranges': 'bytes' })
    return response.end()
  }
  const start = range?.start ?? 0
  const end = range?.end ?? Math.max(0, stat.size - 1)
  const statusCode = range ? 206 : 200
  const headers = {
    'content-type': 'video/mp4',
    'content-length': Math.max(0, end - start + 1),
    'cache-control': 'private, max-age=300',
    'accept-ranges': 'bytes',
    'last-modified': stat.mtime.toUTCString(),
    ...(range ? { 'content-range': `bytes ${start}-${end}/${stat.size}` } : {}),
  }
  response.writeHead(statusCode, headers)
  if (request.method === 'HEAD' || stat.size === 0) return response.end()
  fs.createReadStream(target, { start, end }).pipe(response)
}

async function handleMedia(request, response, taskId, indexText, url) {
  if (request.method !== 'GET' && request.method !== 'HEAD') throw new HttpError(405, 'method not allowed', 'METHOD_NOT_ALLOWED')
  const index = Number(indexText)
  const expiresAt = Number(url.searchParams.get('expires'))
  const supplied = url.searchParams.get('signature') || ''
  if (!Number.isInteger(expiresAt) || expiresAt < Math.floor(Date.now() / 1000)) {
    throw new HttpError(403, 'media URL has expired', 'MEDIA_URL_EXPIRED')
  }
  if (!safeEqual(supplied, signMediaUrl(config.apiKey, taskId, index, expiresAt))) {
    throw new HttpError(403, 'invalid media URL signature', 'MEDIA_URL_INVALID')
  }
  const task = await taskStore.get(taskId)
  if (!task || task.status !== 'succeeded' || !task.videos[index]) throw new HttpError(404, 'video not found', 'VIDEO_NOT_FOUND')
  const reference = task.videos[index]
  if (String(reference).startsWith('local://')) {
    return serveLocalVideo(request, response, taskId, index, reference)
  }
  const { bucket, object } = parseGcsUri(reference)
  const accessToken = await googleAuth.accessToken()
  const headers = { authorization: `Bearer ${accessToken}` }
  if (request.headers.range) headers.range = request.headers.range
  const upstream = await fetch(`https://storage.googleapis.com/storage/v1/b/${encodeURIComponent(bucket)}/o/${encodeURIComponent(object)}?alt=media`, {
    headers,
    redirect: 'follow',
    signal: AbortSignal.timeout(120000),
  })
  if (!upstream.ok && upstream.status !== 206) throw new HttpError(502, `GCS video download failed with HTTP ${upstream.status}`, 'VIDEO_DOWNLOAD_FAILED')
  const outputHeaders = {
    'content-type': upstream.headers.get('content-type') || 'video/mp4',
    'cache-control': 'private, max-age=300',
    'accept-ranges': upstream.headers.get('accept-ranges') || 'bytes',
  }
  for (const name of ['content-length', 'content-range', 'etag', 'last-modified']) {
    const value = upstream.headers.get(name)
    if (value) outputHeaders[name] = value
  }
  response.writeHead(upstream.status, outputHeaders)
  if (request.method === 'HEAD' || !upstream.body) return response.end()
  Readable.fromWeb(upstream.body).pipe(response)
}

async function route(request, response) {
  const url = new URL(request.url, `http://${request.headers.host || 'localhost'}`)
  if (url.pathname === '/health') {
    let credentialReadable = false
    try { await googleAuth.load(); credentialReadable = true } catch { credentialReadable = false }
    return sendJson(response, 200, {
      status: 'ok', service: 'google-video-adapter',
      configured: Boolean(config.apiKey && credentialReadable && (
        config.outputMode === 'local' || (config.outputMode === 'gcs' && config.outputUri)
      )),
      output_mode: config.outputMode,
      gcs_output_configured: Boolean(config.outputUri),
      veo_location: config.location, omni_location: 'global', default_model: config.defaultModel,
      supported_models: ['gemini-omni-flash-preview', 'veo-3.1-generate-001', 'veo-3.1-fast-generate-001'],
      credential_readable: credentialReadable,
    })
  }
  if (url.pathname === '/generate') return handleGenerate(request, response)
  const taskMatch = url.pathname.match(/^\/tasks\/(veo_[a-f0-9]{24})$/)
  if (taskMatch) return handleTask(request, response, taskMatch[1])
  const mediaMatch = url.pathname.match(/^\/media\/(veo_[a-f0-9]{24})\/(\d+)$/)
  if (mediaMatch) return handleMedia(request, response, mediaMatch[1], mediaMatch[2], url)
  throw new HttpError(404, 'not found', 'NOT_FOUND')
}

async function main() {
  await taskStore.init()
  await fsp.mkdir(config.localOutputDir, { recursive: true, mode: 0o700 })
  if (!['local', 'gcs'].includes(config.outputMode)) {
    throw new Error('VEO_OUTPUT_MODE must be local, gcs, or auto')
  }
  normalizeModel(config.defaultModel)
  const server = http.createServer((request, response) => {
    route(request, response).catch((error) => {
      const statusCode = Number(error.statusCode || 500)
      const message = statusCode >= 500 && !(error instanceof HttpError) ? 'internal adapter error' : String(error.message || 'unknown error')
      if (!response.headersSent) sendJson(response, statusCode, { status: 'failed', error: { code: error.code || 'VEO_ADAPTER_ERROR', message } })
      else response.destroy(error)
      console.error(JSON.stringify({ level: 'error', code: error.code || 'VEO_ADAPTER_ERROR', status: statusCode, message: String(error.message || error).slice(0, 2000) }))
    })
  })
  server.requestTimeout = 130000
  server.headersTimeout = 15000
  server.listen(config.port, '0.0.0.0', () => {
    console.log(JSON.stringify({ level: 'info', message: 'Google video adapter listening', port: config.port, location: config.location, model: config.defaultModel }))
  })
}

if (require.main === module) {
  main().catch((error) => {
    console.error(error)
    process.exit(1)
  })
}

module.exports = { config, normalizeGenerateRequest, providerPayload }
