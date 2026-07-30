import { api } from './api.js'

export const performanceContinuityApi = {
  listBibles: (projectId) => api.getPerformanceBibles(projectId),
  createBibleVersion: (projectId, payload) => api.createPerformanceBibleVersion(projectId, payload),
  lockBible: (performanceBibleId) => api.lockPerformanceBible(performanceBibleId),
  ledger: (projectId, episodeId = '') => api.getContinuityLedger(projectId, episodeId),
  issues: (projectId, filters = {}) => api.getVisualQCIssues(projectId, filters),
  createRedo: (issueId, requestedBy) => api.createVisualQCRedo(issueId, { requested_by: requestedBy || undefined }),
  handoffs: (projectId, episodeId = '') => api.getShotHandoffs(projectId, episodeId),
}

export function cloneBibleAsVersion(bible, edits = {}, changeReason = '') {
  const value = (key, fallback) => edits[key] ?? bible[key] ?? fallback
  return {
    character_id: bible.character_id,
    character_version: bible.character_version,
    speech: value('speech', {}),
    acting: value('acting', {}),
    relational_voices: value('relational_voices', {}),
    appearance: value('appearance', {}),
    stage_states: value('stage_states', []),
    locked_fields: value('locked_fields', []),
    allowed_fields: value('allowed_fields', []),
    change_reasons: value('change_reasons', {}),
    source_refs: value('source_refs', {}),
    parent_performance_bible_id: bible.performance_bible_id,
    change_reason: String(changeReason).trim(),
  }
}

export function fieldLockState(bible, fieldPath) {
  const locked = Array.isArray(bible?.locked_fields) && bible.locked_fields.includes(fieldPath)
  const allowed = Array.isArray(bible?.allowed_fields) && bible.allowed_fields.includes(fieldPath)
  return { locked, allowed, editable: !locked && allowed }
}

export function continuitySummary(entry) {
  const state = entry?.output_state || {}
  return {
    environment: state.environment || {},
    axis: state.axis || '—',
    characters: Object.entries(state.characters || {}).map(([id, value]) => ({
      id,
      position: value.position || '—',
      facing: value.facing || '—',
      costume: value.costume || '—',
      held: (value.held_props || []).join('、') || '无',
      emotion: value.emotion || '—',
    })),
    propCount: Object.keys(state.props || {}).length,
  }
}

export function issueLocator(issue) {
  const seconds = Number(issue.timecode_ms || 0) / 1000
  return `${issue.episode_id} / ${issue.scene_id} / ${issue.shot_id} · ${seconds.toFixed(3)}s · F${issue.frame_number}`
}

export function severityRank(value) {
  return ({ blocking: 0, critical: 1, major: 2, minor: 3 })[value] ?? 4
}

export function sortedIssues(issues) {
  return [...(issues || [])].sort((a, b) => (
    severityRank(a.severity) - severityRank(b.severity)
    || String(a.episode_id).localeCompare(String(b.episode_id))
    || String(a.shot_id).localeCompare(String(b.shot_id))
    || Number(a.timecode_ms) - Number(b.timecode_ms)
    || Number(a.frame_number) - Number(b.frame_number)
  ))
}

export function handoffActionLabel(handoff) {
  return `${handoff.from_action_phase || '尾帧姿态'} → ${handoff.to_action_phase || '首帧承接'}`
}
