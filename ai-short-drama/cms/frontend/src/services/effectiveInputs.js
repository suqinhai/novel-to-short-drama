export const resolverStages = [
  ['05', '剧本'],
  ['06', '分镜'],
  ['07', '视觉档案'],
  ['08', '分镜图片'],
  ['09', '视频'],
  ['10', 'TTS'],
  ['17', '后期工作台'],
]

export const effectiveInputKinds = {
  narrative_ir: 'Narrative IR',
  adaptation_spec: 'Adaptation Spec',
  adaptation_plan: '改编计划',
  episode_plan: '单集计划',
  pacing_plan: '节奏计划',
  candidate_selection: '候选选择/组合',
  performance_bible: '表演圣经',
  continuity_ledger: '连续性账本',
  visual_profiles: '视觉档案',
  editing_template: '剪辑模板绑定',
  timeline: '当前时间线',
}

export function effectiveInputStateLabel(state) {
  return ({
    resolved: '有效',
    missing: '缺失',
    stale: '已过期',
    needs_review: '待确认',
    blocked: '阻断',
  })[state] || state || '未知'
}

export function summarizeEffectiveInputs(resolution = {}) {
  const items = Array.isArray(resolution.items) ? resolution.items : []
  const compatibilityMode = resolution.mode === 'legacy'
  const ready = resolution.status === 'ready'
  return {
    ready,
    executable: ready || compatibilityMode,
    compatibilityMode,
    required: items.filter(item => item.requirement === 'required').length,
    optional: items.filter(item => item.requirement === 'optional').length,
    resolved: items.filter(item => item.state === 'resolved').length,
    blocked: items.filter(item => item.blocks).length,
    missing: items.filter(item => item.state === 'missing').map(item => item.kind),
  }
}

export function defaultResolverStage(currentStage = '') {
  const value = String(currentStage).toLowerCase()
  if (value.includes('storyboard_image') || value.includes('stage_3')) return '08'
  if (value.includes('video') || value.includes('shot_')) return '09'
  if (value.includes('voice') || value.includes('audio') || value.includes('tts')) return '10'
  if (value.includes('timeline') || value.includes('render') || value.includes('stage_4')) return '17'
  if (value.includes('storyboard')) return '06'
  if (value.includes('script')) return '05'
  if (value.includes('visual')) return '07'
  return '05'
}
