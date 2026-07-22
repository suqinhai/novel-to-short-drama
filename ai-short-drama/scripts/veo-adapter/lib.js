'use strict'

const crypto = require('crypto')

const SUPPORTED_MODELS = new Set([
  'gemini-omni-flash-preview',
  'veo-3.1-generate-001',
  'veo-3.1-fast-generate-001',
])

function clampInteger(value, fallback, min, max) {
  const parsed = Number.parseInt(value, 10)
  if (!Number.isFinite(parsed)) return fallback
  return Math.min(max, Math.max(min, parsed))
}

function normalizeModel(value, fallback = 'veo-3.1-fast-generate-001') {
  const model = String(value || fallback).trim()
  if (!SUPPORTED_MODELS.has(model)) {
    throw new Error(`unsupported Google video model: ${model}`)
  }
  return model
}

function modelFamily(model) {
  return String(model || '').startsWith('gemini-omni-') ? 'omni' : 'veo'
}

function normalizeDuration(value, model = 'veo-3.1-fast-generate-001') {
  const requested = Number(value)
  if (modelFamily(model) === 'omni') {
    if (!Number.isFinite(requested)) return 6
    return Math.min(10, Math.max(3, Math.round(requested)))
  }
  if (!Number.isFinite(requested)) return 6
  return [4, 6, 8].reduce((best, candidate) => (
    Math.abs(candidate - requested) <= Math.abs(best - requested) ? candidate : best
  ), 6)
}

function normalizeAspectRatio(value) {
  const aspect = String(value || '9:16').trim()
  if (!['9:16', '16:9'].includes(aspect)) {
    throw new Error('aspect_ratio must be 9:16 or 16:9')
  }
  return aspect
}

function normalizeResolution(value, width, height) {
  const raw = String(value || '').trim().toLowerCase()
  if (raw === '720p' || raw === '1080p') return raw
  const match = raw.match(/^(\d+)x(\d+)$/)
  const largest = match
    ? Math.max(Number(match[1]), Number(match[2]))
    : Math.max(Number(width || 0), Number(height || 0))
  return largest > 0 && largest <= 1280 ? '720p' : '1080p'
}

function normalizePipelineResolution(value, aspectRatio, vertexResolution) {
  const raw = String(value || '').trim().toLowerCase()
  if (/^\d+x\d+$/.test(raw)) return raw
  const landscape = aspectRatio === '16:9'
  if (vertexResolution === '720p') return landscape ? '1280x720' : '720x1280'
  return landscape ? '1920x1080' : '1080x1920'
}

function parseGcsUri(value) {
  const uri = String(value || '').trim()
  if (!uri.startsWith('gs://')) throw new Error('invalid GCS URI')
  const slash = uri.indexOf('/', 5)
  if (slash < 0) throw new Error('GCS URI must include an object path')
  const bucket = uri.slice(5, slash)
  const object = uri.slice(slash + 1)
  if (!/^[a-z0-9][a-z0-9._-]{1,220}[a-z0-9]$/.test(bucket) || !object) {
    throw new Error('invalid GCS URI')
  }
  return { bucket, object }
}

function appendGcsPrefix(base, taskId) {
  const normalized = String(base || '').trim().replace(/\/+$/, '')
  if (!/^gs:\/\/[a-z0-9][a-z0-9._-]+(?:\/.*)?$/i.test(normalized)) {
    throw new Error('VEO_GCS_OUTPUT_URI must be a gs:// bucket or folder')
  }
  return `${normalized}/${taskId}/`
}

function base64Url(value) {
  return Buffer.from(value).toString('base64url')
}

function createServiceAccountJwt(serviceAccount, nowSeconds = Math.floor(Date.now() / 1000)) {
  if (!serviceAccount || serviceAccount.type !== 'service_account') {
    throw new Error('credential must be a Google service_account JSON')
  }
  if (!serviceAccount.client_email || !serviceAccount.private_key) {
    throw new Error('service account JSON is missing client_email or private_key')
  }
  const header = {
    alg: 'RS256',
    typ: 'JWT',
    ...(serviceAccount.private_key_id ? { kid: serviceAccount.private_key_id } : {}),
  }
  const payload = {
    iss: serviceAccount.client_email,
    scope: 'https://www.googleapis.com/auth/cloud-platform',
    aud: 'https://oauth2.googleapis.com/token',
    iat: nowSeconds,
    exp: nowSeconds + 3600,
  }
  const unsigned = `${base64Url(JSON.stringify(header))}.${base64Url(JSON.stringify(payload))}`
  const signature = crypto.sign('RSA-SHA256', Buffer.from(unsigned), serviceAccount.private_key)
  return `${unsigned}.${signature.toString('base64url')}`
}

function taskIdForIdempotency(value) {
  const input = String(value || crypto.randomUUID())
  return `veo_${crypto.createHash('sha256').update(input).digest('hex').slice(0, 24)}`
}

function requestFingerprint(value) {
  return crypto.createHash('sha256').update(JSON.stringify(value)).digest('hex')
}

function rewriteImageUrl(value, fromPrefix, toPrefix) {
  const raw = String(value || '').trim()
  const from = String(fromPrefix || '').replace(/\/+$/, '')
  const to = String(toPrefix || '').replace(/\/+$/, '')
  if (from && to && (raw === from || raw.startsWith(`${from}/`))) {
    return `${to}${raw.slice(from.length)}`
  }
  return raw
}

function assertAllowedImageUrl(value, allowedHosts) {
  let parsed
  try {
    parsed = new URL(value)
  } catch {
    throw new Error('image_url must be an absolute HTTP(S) URL')
  }
  if (!['http:', 'https:'].includes(parsed.protocol) || parsed.username || parsed.password) {
    throw new Error('image_url must be a safe HTTP(S) URL')
  }
  const allowed = new Set((allowedHosts || []).map((item) => String(item).trim().toLowerCase()).filter(Boolean))
  if (allowed.size && !allowed.has(parsed.hostname.toLowerCase())) {
    throw new Error(`image_url host is not allowed: ${parsed.hostname}`)
  }
  return parsed
}

function sniffImageMime(buffer, contentType = '') {
  const declared = String(contentType).split(';')[0].trim().toLowerCase()
  const isPng = buffer.length >= 8 && buffer.subarray(0, 8).equals(Buffer.from('89504e470d0a1a0a', 'hex'))
  const isJpeg = buffer.length >= 3 && buffer[0] === 0xff && buffer[1] === 0xd8 && buffer[2] === 0xff
  if (isPng && (!declared || declared === 'image/png' || declared === 'application/octet-stream')) return 'image/png'
  if (isJpeg && (!declared || ['image/jpeg', 'image/jpg', 'application/octet-stream'].includes(declared))) return 'image/jpeg'
  throw new Error('Veo input image must be a valid PNG or JPEG')
}

function safeEqual(left, right) {
  const a = Buffer.from(String(left || ''))
  const b = Buffer.from(String(right || ''))
  return a.length === b.length && crypto.timingSafeEqual(a, b)
}

function signMediaUrl(secret, taskId, index, expiresAt) {
  return crypto.createHmac('sha256', secret)
    .update(`${taskId}:${index}:${expiresAt}`)
    .digest('base64url')
}

function extractVideoUris(operation) {
  const response = operation && operation.response && typeof operation.response === 'object'
    ? operation.response
    : {}
  const candidates = []
  if (Array.isArray(response.videos)) candidates.push(...response.videos)
  if (Array.isArray(response.generatedVideos)) candidates.push(...response.generatedVideos)
  return candidates.map((entry) => {
    if (typeof entry === 'string') return entry
    if (!entry || typeof entry !== 'object') return ''
    return entry.gcsUri || entry.uri || entry.video?.gcsUri || entry.video?.uri || ''
  }).filter((uri) => String(uri).startsWith('gs://'))
}

function extractInteractionVideoUris(interaction) {
  const steps = Array.isArray(interaction?.steps) ? interaction.steps : []
  const uris = []
  for (const step of steps) {
    if (!step || step.type !== 'model_output' || !Array.isArray(step.content)) continue
    for (const item of step.content) {
      if (item?.type === 'video' && String(item.uri || '').startsWith('gs://')) uris.push(String(item.uri))
    }
  }
  return uris
}

module.exports = {
  SUPPORTED_MODELS,
  appendGcsPrefix,
  assertAllowedImageUrl,
  clampInteger,
  createServiceAccountJwt,
  extractInteractionVideoUris,
  extractVideoUris,
  modelFamily,
  normalizeAspectRatio,
  normalizeDuration,
  normalizeModel,
  normalizePipelineResolution,
  normalizeResolution,
  parseGcsUri,
  requestFingerprint,
  rewriteImageUrl,
  safeEqual,
  signMediaUrl,
  sniffImageMime,
  taskIdForIdempotency,
}
