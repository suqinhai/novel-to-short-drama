'use strict'

const assert = require('assert/strict')
const { generateKeyPairSync } = require('crypto')
const test = require('node:test')
const {
  appendGcsPrefix,
  assertAllowedImageUrl,
  createServiceAccountJwt,
  decodeBase64Video,
  extractInteractionVideoOutputs,
  extractInteractionVideoUris,
  extractVideoOutputs,
  extractVideoUris,
  localVideoRef,
  normalizeDuration,
  normalizePipelineResolution,
  normalizeResolution,
  parseByteRange,
  parseGcsUri,
  parseLocalVideoRef,
  rewriteImageUrl,
  safeEqual,
  signMediaUrl,
  sniffImageMime,
  taskIdForIdempotency,
} = require('../lib')

test('maps arbitrary shot durations to Veo-supported values', () => {
  assert.equal(normalizeDuration(2), 4)
  assert.equal(normalizeDuration(5), 6)
  assert.equal(normalizeDuration(7), 8)
  assert.equal(normalizeDuration(30), 8)
})

test('clamps Omni durations to whole seconds from 3 through 10', () => {
  assert.equal(normalizeDuration(2, 'gemini-omni-flash-preview'), 3)
  assert.equal(normalizeDuration(5.6, 'gemini-omni-flash-preview'), 6)
  assert.equal(normalizeDuration(30, 'gemini-omni-flash-preview'), 10)
})

test('normalizes pipeline and Vertex resolutions', () => {
  assert.equal(normalizeResolution('1080x1920'), '1080p')
  assert.equal(normalizeResolution('720x1280'), '720p')
  assert.equal(normalizePipelineResolution('', '9:16', '1080p'), '1080x1920')
  assert.equal(normalizePipelineResolution('', '16:9', '720p'), '1280x720')
})

test('rewrites and restricts image URLs', () => {
  const rewritten = rewriteImageUrl('http://localhost:8088/storyboard/a.png', 'http://localhost:8088', 'http://media')
  assert.equal(rewritten, 'http://media/storyboard/a.png')
  assert.equal(assertAllowedImageUrl(rewritten, ['media']).hostname, 'media')
  assert.throws(() => assertAllowedImageUrl('http://169.254.169.254/latest', ['media']), /not allowed/)
})

test('validates image magic bytes', () => {
  const png = Buffer.from('89504e470d0a1a0a00000000', 'hex')
  const jpeg = Buffer.from('ffd8ffe000000000', 'hex')
  assert.equal(sniffImageMime(png, 'image/png'), 'image/png')
  assert.equal(sniffImageMime(jpeg, 'image/jpeg'), 'image/jpeg')
  assert.throws(() => sniffImageMime(Buffer.from('not an image')), /valid PNG or JPEG/)
})

test('creates stable provider task IDs and signed media URLs', () => {
  assert.equal(taskIdForIdempotency('same'), taskIdForIdempotency('same'))
  assert.match(taskIdForIdempotency('same'), /^veo_[a-f0-9]{24}$/)
  const signature = signMediaUrl('secret', 'veo_0123456789abcdef01234567', 0, 123)
  assert.ok(safeEqual(signature, signMediaUrl('secret', 'veo_0123456789abcdef01234567', 0, 123)))
  assert.equal(safeEqual(signature, `${signature}x`), false)
})

test('handles GCS task prefixes and response variants', () => {
  assert.equal(appendGcsPrefix('gs://bucket-name/veo/', 'veo_abc'), 'gs://bucket-name/veo/veo_abc/')
  assert.deepEqual(parseGcsUri('gs://bucket-name/folder/video.mp4'), { bucket: 'bucket-name', object: 'folder/video.mp4' })
  assert.deepEqual(extractVideoUris({ response: { videos: [{ gcsUri: 'gs://bucket-name/a.mp4' }] } }), ['gs://bucket-name/a.mp4'])
  assert.deepEqual(extractVideoUris({ response: { generatedVideos: [{ video: { uri: 'gs://bucket-name/b.mp4' } }] } }), ['gs://bucket-name/b.mp4'])
  assert.deepEqual(extractInteractionVideoUris({ steps: [{ type: 'model_output', content: [{ type: 'video', uri: 'gs://bucket-name/omni.mp4' }] }] }), ['gs://bucket-name/omni.mp4'])
})

test('extracts inline Veo and Omni video payloads', () => {
  const veo = extractVideoOutputs({ response: { generatedVideos: [{ video: { bytesBase64Encoded: 'AAAA', mimeType: 'video/mp4' } }] } })
  assert.deepEqual(veo, [{ uri: '', data: 'AAAA', mime_type: 'video/mp4' }])
  const omni = extractInteractionVideoOutputs({
    steps: [{ type: 'model_output', content: [{ type: 'video', data: 'BBBB', mime_type: 'video/mp4' }] }],
  })
  assert.deepEqual(omni, [{ uri: '', data: 'BBBB', mime_type: 'video/mp4' }])
})

test('validates local MP4 payloads and references', () => {
  const mp4 = Buffer.from('000000186674797069736f6d0000000069736f6d', 'hex')
  assert.deepEqual(decodeBase64Video(mp4.toString('base64'), 1024), mp4)
  assert.throws(() => decodeBase64Video(Buffer.from('not-video').toString('base64'), 1024), /valid MP4/)
  assert.throws(() => decodeBase64Video(mp4.toString('base64'), 8), /size limit/)
  const taskId = 'veo_0123456789abcdef01234567'
  const reference = localVideoRef(taskId, 2)
  assert.equal(reference, `local://${taskId}/2.mp4`)
  assert.deepEqual(parseLocalVideoRef(reference), { taskId, index: 2 })
})

test('normalizes HTTP byte ranges for locally served videos', () => {
  assert.equal(parseByteRange('', 100), null)
  assert.deepEqual(parseByteRange('bytes=10-19', 100), { start: 10, end: 19 })
  assert.deepEqual(parseByteRange('bytes=90-', 100), { start: 90, end: 99 })
  assert.deepEqual(parseByteRange('bytes=-10', 100), { start: 90, end: 99 })
  assert.throws(() => parseByteRange('bytes=100-101', 100), /invalid byte range/)
})

test('creates a signed service-account JWT', () => {
  const { privateKey } = generateKeyPairSync('rsa', { modulusLength: 2048 })
  const jwt = createServiceAccountJwt({
    type: 'service_account',
    client_email: 'veo@example.iam.gserviceaccount.com',
    private_key: privateKey.export({ format: 'pem', type: 'pkcs8' }).toString(),
  }, 1000)
  assert.equal(jwt.split('.').length, 3)
})
