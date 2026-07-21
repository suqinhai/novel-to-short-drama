const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api/v1'

async function request(path, options = {}) {
  const response = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...options.headers },
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
  createProject(payload) {
    return request('/projects', { method: 'POST', body: JSON.stringify(payload) })
  },
  runProjectAction(projectId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/actions`, { method: 'POST', body: JSON.stringify(payload) })
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
  decideReview(reviewId, payload) {
    return request(`/reviews/${encodeURIComponent(reviewId)}/decision`, { method: 'POST', body: JSON.stringify(payload) })
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
}
