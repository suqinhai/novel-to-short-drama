const API_BASE = import.meta.env?.VITE_API_BASE_URL || '/api/v1'

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
  getEffectiveInputs(projectId, episodeId, stage) {
    const scope = episodeId
      ? `/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}`
      : `/projects/${encodeURIComponent(projectId)}`
    return request(`${scope}/effective-inputs?${new URLSearchParams({ stage })}`)
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
	createEpisodeContentChangePlan(projectId, episodeRunId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/episode-runs/${encodeURIComponent(episodeRunId)}/content/change-plan`, {
      method: 'POST', body: JSON.stringify(payload),
    })
	},
	createEpisodeContentAIChangePlan(projectId, episodeRunId, payload) {
		return request(`/projects/${encodeURIComponent(projectId)}/episode-runs/${encodeURIComponent(episodeRunId)}/content/ai-change-plan`, {
			method: 'POST', body: JSON.stringify(payload),
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
  getChangePlans(projectId) {
    return request(`/projects/${encodeURIComponent(projectId)}/change-plans`)
  },
  createChangePlan(projectId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/change-plans`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  confirmChangePlan(projectId, changePlanId, payload = {}) {
    return request(`/projects/${encodeURIComponent(projectId)}/change-plans/${encodeURIComponent(changePlanId)}/confirm`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  rejectChangePlan(projectId, changePlanId, payload = {}) {
    return request(`/projects/${encodeURIComponent(projectId)}/change-plans/${encodeURIComponent(changePlanId)}/reject`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  executeChangePlan(projectId, changePlanId) {
    return request(`/projects/${encodeURIComponent(projectId)}/change-plans/${encodeURIComponent(changePlanId)}/execute`, {
      method: 'POST', body: JSON.stringify({}),
    })
  },
  getEntityVersions(projectId, entityType, entityId) {
    const query = new URLSearchParams({ entity_type: entityType, entity_id: entityId })
    return request(`/projects/${encodeURIComponent(projectId)}/entity-versions?${query}`)
  },
  createVersionRestorePlan(projectId, entityVersionId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/entity-versions/${encodeURIComponent(entityVersionId)}/change-plan`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  getChangeComments(projectId, entityType = '', entityId = '') {
    const query = new URLSearchParams({ entity_type: entityType, entity_id: entityId })
    return request(`/projects/${encodeURIComponent(projectId)}/change-comments?${query}`)
  },
  createChangeComment(projectId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/change-comments`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  getCreativeWorkbench(projectId, episodeId) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/creative-workbench`)
  },
  createShotEditPlan(projectId, episodeId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/shot-edit-plans`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  getShotEditPlan(projectId, episodeId, planId) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/shot-edit-plans/${encodeURIComponent(planId)}`)
  },
  confirmShotEditPlan(projectId, episodeId, planId, payload = {}) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/shot-edit-plans/${encodeURIComponent(planId)}/confirm`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  executeShotEditPlan(projectId, episodeId, planId) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/shot-edit-plans/${encodeURIComponent(planId)}/execute`, {
      method: 'POST', body: JSON.stringify({}),
    })
  },
  getShotSequenceVersions(projectId, episodeId) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/shot-sequence-versions`)
  },
  getEditingTemplates(projectId) {
    return request(`/projects/${encodeURIComponent(projectId)}/editing-templates`)
  },
  validateDialogueTimings(projectId, episodeId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/dialogue-timings/validate`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  getTimelineVersions(projectId, episodeId) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/timeline-versions`)
  },
  getNLETimeline(projectId, episodeId, params = {}) {
    const query = new URLSearchParams(Object.entries(params).filter(([, value]) => value !== '' && value != null))
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/nle-timeline?${query}`)
  },
  editNLETimelineItem(projectId, episodeId, timelineId, itemId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/timeline-versions/${encodeURIComponent(timelineId)}/items/${encodeURIComponent(itemId)}`, {
      method: 'PATCH', body: JSON.stringify(payload),
    })
  },
  restoreNLETimelineDraft(projectId, episodeId, timelineId, payload = {}) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/timeline-versions/${encodeURIComponent(timelineId)}/restore-draft`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  confirmNLETimelineRender(projectId, episodeId, timelineId) {
    return request(`/projects/${encodeURIComponent(projectId)}/episodes/${encodeURIComponent(episodeId)}/timeline-versions/${encodeURIComponent(timelineId)}/render`, {
      method: 'POST', body: JSON.stringify({}),
    })
  },
  getPerformanceBibles(projectId) {
    return request(`/projects/${encodeURIComponent(projectId)}/performance-bibles`)
  },
  createPerformanceBibleVersion(projectId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/performance-bibles`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  lockPerformanceBible(performanceBibleId) {
    return request(`/performance-bibles/${encodeURIComponent(performanceBibleId)}/lock`, {
      method: 'POST', body: JSON.stringify({}),
    })
  },
  getContinuityLedger(projectId, episodeId = '') {
    return request(`/projects/${encodeURIComponent(projectId)}/continuity-ledger?${new URLSearchParams({ episode_id: episodeId })}`)
  },
  prepareGenerationContext(projectId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/generation-context/prepare`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  getVisualQCIssues(projectId, filters = {}) {
    const query = new URLSearchParams(Object.entries(filters).filter(([, value]) => value !== '' && value != null))
    return request(`/projects/${encodeURIComponent(projectId)}/visual-qc/issues?${query}`)
  },
  runVisualQCFixture(projectId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/visual-qc/run-fixture`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  createVisualQCRedo(issueId, payload = {}) {
    return request(`/visual-qc/issues/${encodeURIComponent(issueId)}/create-local-redo`, {
      method: 'POST', body: JSON.stringify(payload),
    })
  },
  getShotHandoffs(projectId, episodeId = '') {
    return request(`/projects/${encodeURIComponent(projectId)}/shot-handoffs?${new URLSearchParams({ episode_id: episodeId })}`)
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
  getCreationTargets(projectId) {
    return request(`/projects/${encodeURIComponent(projectId)}/creation-targets`)
  },
  getPromptTemplates(category = '') {
    return request(`/prompt-lab/templates?${new URLSearchParams({ category })}`)
  },
  createPromptTemplate(payload) {
    return request('/prompt-lab/templates', { method: 'POST', body: JSON.stringify(payload) })
  },
  createPromptVersion(templateId, payload) {
    return request(`/prompt-lab/templates/${encodeURIComponent(templateId)}/versions`, { method: 'POST', body: JSON.stringify(payload) })
  },
  previewPromptVersion(versionId, variables) {
    return request(`/prompt-lab/versions/${encodeURIComponent(versionId)}/preview`, { method: 'POST', body: JSON.stringify({ variables }) })
  },
  approvePromptVersion(versionId, actor) {
    return request(`/prompt-lab/versions/${encodeURIComponent(versionId)}/approve`, { method: 'POST', body: JSON.stringify({ actor }) })
  },
  promotePromptVersion(versionId, actor) {
    return request(`/prompt-lab/versions/${encodeURIComponent(versionId)}/promote`, { method: 'POST', body: JSON.stringify({ actor }) })
  },
  getPromptFixtures(category = '') {
    return request(`/prompt-lab/fixtures?${new URLSearchParams({ category })}`)
  },
  createPromptFixture(payload) {
    return request('/prompt-lab/fixtures', { method: 'POST', body: JSON.stringify(payload) })
  },
  getPromptTestSuites(category = '') {
    return request(`/prompt-lab/test-suites?${new URLSearchParams({ category })}`)
  },
  createPromptTestSuite(payload) {
    return request('/prompt-lab/test-suites', { method: 'POST', body: JSON.stringify(payload) })
  },
  getPromptExperiments(category = '') {
    return request(`/prompt-lab/experiments?${new URLSearchParams({ category })}`)
  },
  createPromptExperiment(payload) {
    return request('/prompt-lab/experiments', { method: 'POST', body: JSON.stringify(payload) })
  },
  getPromptExperiment(experimentId, blind = false) {
    return request(`/prompt-lab/experiments/${encodeURIComponent(experimentId)}${blind ? '/blind' : ''}`)
  },
  runPromptExperiment(experimentId) {
    return request(`/prompt-lab/experiments/${encodeURIComponent(experimentId)}/run`, { method: 'POST' })
  },
  submitPromptBlindEvaluation(experimentId, payload) {
    return request(`/prompt-lab/experiments/${encodeURIComponent(experimentId)}/blind-evaluations`, { method: 'POST', body: JSON.stringify(payload) })
  },
  getGenerationProvenance(projectId, episodeId = '') {
    return request(`/projects/${encodeURIComponent(projectId)}/generation-provenance?${new URLSearchParams({ episode_id: episodeId })}`)
  },
  getProfessionalExportOptions(projectId, episodeId = '') {
    return request(`/projects/${encodeURIComponent(projectId)}/export-options?${new URLSearchParams({ episode_id: episodeId })}`)
  },
  getProfessionalExports(projectId, episodeId = '') {
    return request(`/projects/${encodeURIComponent(projectId)}/professional-exports?${new URLSearchParams({ episode_id: episodeId })}`)
  },
  createProfessionalExport(projectId, payload) {
    return request(`/projects/${encodeURIComponent(projectId)}/professional-exports`, { method: 'POST', body: JSON.stringify(payload) })
  },
  professionalExportDownloadUrl(projectId, exportId) {
    return `${API_BASE}/projects/${encodeURIComponent(projectId)}/professional-exports/${encodeURIComponent(exportId)}/download`
  },
}
