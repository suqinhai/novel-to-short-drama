const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api/v1'

async function request(path, options = {}) {
  const isFormData = typeof FormData !== 'undefined' && options.body instanceof FormData
  const response = await fetch(`${API_BASE}${path}`, {
    headers: { ...(isFormData ? {} : { 'Content-Type': 'application/json' }), ...options.headers },
    ...options,
  })
  const payload = await response.json().catch(() => ({}))
  if (!response.ok) {
    throw new Error(payload?.error?.message || `请求失败（${response.status}）`)
  }
  return payload.data
}

export const api = {
  getProjects(params = {}) {
    const query = new URLSearchParams(Object.entries(params).filter(([, value]) => value !== '' && value != null))
    return request(`/projects?${query}`)
  },
  getProject(projectId) {
    return request(`/projects/${encodeURIComponent(projectId)}`)
  },
  archiveProject(projectId) {
    return request(`/projects/${encodeURIComponent(projectId)}`, {
      method: 'DELETE', body: JSON.stringify({ confirm_project_id: projectId }),
    })
  },
  restoreProject(projectId) {
    return request(`/projects/${encodeURIComponent(projectId)}/restore`, { method: 'POST' })
  },
  createProject(payload) {
    return request('/projects', { method: 'POST', body: JSON.stringify(payload) })
  },
  runProjectAction(projectId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/actions`, { method: 'POST', body: JSON.stringify(payload) })
  },
  adoptRollingPlan(projectId, planId, payload = {}) {
    return request(`/projects/${encodeURIComponent(projectId)}/rolling-plans/${encodeURIComponent(planId)}/adopt`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  activateEpisodeRun(projectId, episodeRunId) {
    return request(`/projects/${encodeURIComponent(projectId)}/episode-runs/${encodeURIComponent(episodeRunId)}/activate`, {
      method: 'POST', body: JSON.stringify({}),
    })
  },
  getEpisodeContent(projectId, episodeRunId) {
    return request(`/projects/${encodeURIComponent(projectId)}/episode-runs/${encodeURIComponent(episodeRunId)}/content`)
  },
  updateEpisodeContent(projectId, episodeRunId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/episode-runs/${encodeURIComponent(episodeRunId)}/content`, {
      method: 'PATCH', body: JSON.stringify(payload),
    })
  },
  getReviews(params = {}) {
    const query = new URLSearchParams(Object.entries(params).filter(([, value]) => value !== '' && value != null))
    return request(`/reviews?${query}`)
  },
  getReviewContent(reviewId) {
    return request(`/reviews/${encodeURIComponent(reviewId)}/content`)
  },
  getMediaAssets(params = {}) {
    const query = new URLSearchParams(Object.entries(params).filter(([, value]) => value !== '' && value != null))
    return request(`/media-assets?${query}`)
  },
  regenerateMediaAsset(assetType, assetId, payload = {}) {
    return request(`/media-assets/${encodeURIComponent(assetType)}/${encodeURIComponent(assetId)}/regenerate`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  replaceMediaAsset(assetType, assetId, formData) {
    return request(`/media-assets/${encodeURIComponent(assetType)}/${encodeURIComponent(assetId)}/replacement`, {
      method: 'POST', body: formData,
    })
  },
  decideReview(reviewId, payload) {
    return request(`/reviews/${encodeURIComponent(reviewId)}/decision`, { method: 'POST', body: JSON.stringify(payload) })
  },
  regenerateReview(reviewId, payload) {
    return request(`/reviews/${encodeURIComponent(reviewId)}/regenerate`, { method: 'POST', body: JSON.stringify(payload) })
  },
  getDiagnostics() {
    return request('/diagnostics')
  },
  getAIConfig() {
    return request('/ai-config')
  },
  updateAIConfig(payload) {
    return request('/ai-config', { method: 'PUT', body: JSON.stringify(payload) })
  },
  getDataResetPreview() {
    return request('/data-reset')
  },
  resetAllData(payload) {
    return request('/data-reset', {
      method: 'POST',
      headers: { 'X-Data-Reset-Intent': 'permanent' },
      body: JSON.stringify(payload),
    })
  },
}
