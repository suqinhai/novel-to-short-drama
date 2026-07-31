export const entityOptions = [
  { value: 'outline', label: '大纲' },
  { value: 'script', label: '剧本' },
  { value: 'episode_content', label: '单集内容' },
  { value: 'dialogue', label: '对白' },
  { value: 'scene', label: '场景' },
  { value: 'shot', label: '镜头' },
  { value: 'shot_video', label: '视频片段' },
  { value: 'timeline', label: '时间线' },
  { value: 'timeline_item', label: '时间线片段' },
  { value: 'media', label: '媒体替换' },
]

export const changeStatusLabels = {
  validated: '待确认', confirmed: '已确认', executing: '执行中',
  applied: '已应用', failed: '失败', cancelled: '已取消',
}

export function buildChangePlanRequest(form) {
  return {
    instruction: String(form.instruction || '').trim(),
    target: {
      entity_type: form.entity_type,
      entity_id: String(form.entity_id || '').trim(),
      version: Number(form.version || 1),
    },
    requested_by: String(form.requested_by || '').trim() || undefined,
  }
}

export function planDiffRows(plan) {
  return (plan?.expected_changes || []).map((change) => ({
    field: change.field,
    operation: change.operation,
    before: change.before ?? (change.operation === 'adjust' ? '当前值' : '当前版本'),
    after: change.after ?? change.delta ?? change.value ?? `${change.start_ms}–${change.end_ms}ms`,
  }))
}

export function rebuildLabels(plan) {
  const decisions = plan?.rebuild || {}
  return [
    ['voice', '重新配音'], ['subtitle', '更新字幕'], ['image', '重新出图'],
    ['video', '重新生成视频'], ['edit', '重新剪辑'], ['continuity', '更新相邻连续性'],
  ].filter(([key]) => decisions[key]).map(([, label]) => label)
}

export function localEditLinkForReview(item) {
  const type = item?.stage === 'storyboard' ? 'shot'
    : item?.stage === 'shot_video' ? 'shot_video'
      : item?.stage === 'episode_script' ? 'dialogue' : 'media'
  return {
    path: `/projects/${item.project_id}/local-edit`,
    query: { entity_type: type, entity_id: item.entity_id, version: item.generation_version || 1 },
  }
}

export function localEditLinkForMedia(item) {
  let type = 'media'
  let id = item.asset_id
  if (item.asset_type === 'shot_videos') type = 'shot_video'
  else if (item.entity_type === 'shot') { type = 'shot'; id = item.entity_id }
  else if (item.entity_type === 'dialogue') { type = 'dialogue'; id = item.entity_id }
  return {
    path: `/projects/${item.project_id}/local-edit`,
    query: { entity_type: type, entity_id: id, version: item.generation_version || 1 },
  }
}
