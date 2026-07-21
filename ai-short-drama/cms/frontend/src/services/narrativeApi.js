const API_BASE = import.meta.env.VITE_NARRATIVE_API_BASE_URL || '/api/v2'
const ETAG_PREFIX = 'cms:narrative-etag:'

export class NarrativeApiError extends Error {
  constructor(message, { status = 0, code = 'REQUEST_FAILED', details = [], traceId = '' } = {}) {
    super(message)
    this.name = 'NarrativeApiError'
    this.status = status
    this.code = code
    this.details = details
    this.traceId = traceId
    this.isConflict = status === 409 || status === 412
  }
}

function resourceKey(resourceType, resourceId) {
  return `${ETAG_PREFIX}${resourceType}:${resourceId}`
}

function rememberETag(resourceType, resourceId, etag) {
  if (!etag || !resourceId) return
  sessionStorage.setItem(resourceKey(resourceType, resourceId), etag)
}

function cachedETag(resourceType, resourceId) {
  return sessionStorage.getItem(resourceKey(resourceType, resourceId)) || ''
}

function revisionETag(revision) {
  return Number.isInteger(revision) && revision > 0 ? `"${revision}"` : ''
}

export function createIdempotencyKey(scope = 'cms') {
  return `${scope}:${crypto.randomUUID()}`
}

async function request(path, { resource, ...options } = {}) {
  const headers = new Headers(options.headers || {})
  if (options.body != null && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const response = await fetch(`${API_BASE}${path}`, { credentials: 'same-origin', ...options, headers })
  const payload = await response.json().catch(() => ({}))
  const etag = response.headers.get('ETag') || ''

  if (!response.ok) {
    const apiError = payload?.error || {}
    throw new NarrativeApiError(apiError.message || `请求失败（${response.status}）`, {
      status: response.status,
      code: apiError.code,
      details: apiError.details,
      traceId: payload?.trace_id,
    })
  }

  if (resource) {
    rememberETag(resource.type, resource.id, etag || revisionETag(payload?.data?.resource_revision))
  }
  return {
    data: payload.data,
    page: payload.page,
    traceId: payload.trace_id,
    contractVersion: payload.contract_version,
    etag,
  }
}

function command(path, { method = 'POST', body, idempotencyKey, versionResource } = {}) {
  const headers = { 'Idempotency-Key': idempotencyKey }
  if (versionResource) {
    const etag = cachedETag('source-version', versionResource)
    if (!etag) {
      throw new NarrativeApiError('缺少版本 ETag，请刷新版本后重试。', { status: 412, code: 'ETAG_REQUIRED' })
    }
    headers['If-Match'] = etag
  }
  return request(path, {
    method,
    headers,
    body: body == null ? undefined : JSON.stringify(body),
    resource: versionResource ? { type: 'source-version', id: versionResource } : undefined,
  })
}

const id = encodeURIComponent

export const narrativeApi = {
  listWorks(params = {}) {
    const query = new URLSearchParams(Object.entries(params).filter(([, value]) => value !== '' && value != null))
    return request(`/source-works?${query}`)
  },
  getWork(workId) {
    return request(`/source-works/${id(workId)}`)
  },
  createWork(payload, idempotencyKey) {
    return command('/source-works', { body: payload, idempotencyKey })
  },
  listVersions(workId) {
    return request(`/source-works/${id(workId)}/versions`)
  },
  createVersion(workId, payload, idempotencyKey) {
    return command(`/source-works/${id(workId)}/versions`, { body: payload, idempotencyKey })
  },
  getVersion(sourceVersionId) {
    return request(`/source-versions/${id(sourceVersionId)}`, {
      resource: { type: 'source-version', id: sourceVersionId },
    })
  },
  listChapters(sourceVersionId) {
    return request(`/source-versions/${id(sourceVersionId)}/chapters`)
  },
  startImport(sourceVersionId, payload, idempotencyKey) {
    return command(`/source-versions/${id(sourceVersionId)}/imports`, {
      body: payload, idempotencyKey, versionResource: sourceVersionId,
    })
  },
  addChapter(sourceVersionId, payload, idempotencyKey) {
    return command(`/source-versions/${id(sourceVersionId)}/chapters`, {
      body: payload, idempotencyKey, versionResource: sourceVersionId,
    })
  },
  addChaptersBatch(sourceVersionId, payload, idempotencyKey) {
    return command(`/source-versions/${id(sourceVersionId)}/chapters:batch`, {
      body: payload, idempotencyKey, versionResource: sourceVersionId,
    })
  },
  reviseChapter(sourceVersionId, chapterId, payload, idempotencyKey) {
    return command(`/source-versions/${id(sourceVersionId)}/chapters/${id(chapterId)}`, {
      method: 'PATCH', body: payload, idempotencyKey, versionResource: sourceVersionId,
    })
  },
  publishVersion(sourceVersionId, idempotencyKey) {
    return command(`/source-versions/${id(sourceVersionId)}:publish`, {
      idempotencyKey, versionResource: sourceVersionId,
    })
  },
  getOperation(operationId) {
    return request(`/operations/${id(operationId)}`)
  },
  getProjectImpact(projectId, toSourceVersionId) {
    const query = new URLSearchParams({ to_source_version_id: toSourceVersionId })
    return request(`/adaptation-projects/${id(projectId)}/impact?${query}`)
  },
  createRegenerationRequest(projectId, changeSetId, payload, idempotencyKey) {
    return command(`/adaptation-projects/${id(projectId)}/impact/${id(changeSetId)}/regeneration-requests`, {
      body: payload, idempotencyKey,
    })
  },
  createAdaptationProject(payload, idempotencyKey) {
    return command('/adaptation-projects', { body: payload, idempotencyKey })
  },
  listAdaptationSpecs(projectId) {
    return request(`/adaptation-projects/${id(projectId)}/specs`)
  },
  createAdaptationSpec(projectId, payload, idempotencyKey) {
    return command(`/adaptation-projects/${id(projectId)}/specs`, { body: payload, idempotencyKey })
  },
  getCachedVersionETag(sourceVersionId) {
    return cachedETag('source-version', sourceVersionId)
  },
}
