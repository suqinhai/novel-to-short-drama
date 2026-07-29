export const targetComponents = {
  story_arc: ['episode_plan'],
  episode: ['opening', 'conflict', 'climax', 'ending_hook'],
  scene: ['dialogue', 'action', 'narration'],
  storyboard: ['composition', 'shot_size', 'camera_movement', 'performance', 'transition'],
  image: ['key_image'],
  video: ['video_shot'],
}

export const componentLabels = {
  episode_plan: '分集方案', opening: '开场', conflict: '冲突推进', climax: '高潮', ending_hook: '结尾钩子',
  dialogue: '对白', action: '动作', narration: '旁白', composition: '构图', shot_size: '景别',
  camera_movement: '运镜', performance: '表演', transition: '转场', key_image: '关键图片', video_shot: '视频镜头',
}

export function splitLines(value) {
  return String(value || '').split(/\r?\n|[,，]/).map((item) => item.trim()).filter(Boolean)
}

export function buildCandidateRequest(form) {
  return {
    target_type: form.target_type,
    target_id: form.target_id.trim(),
    component_types: [...form.component_types],
    candidate_count: Number(form.candidate_count),
    difference_directions: splitLines(form.difference_directions),
    must_preserve: splitLines(form.must_preserve),
    allowed_changes: splitLines(form.allowed_changes),
    model: 'deterministic_mock',
    prompt_version: 'multi-candidate-v1',
    random_seed: Number(form.random_seed) || 0,
    generation_parameters: { temperature: Number(form.temperature), comparison_mode: 'structured' },
    base_duration_seconds: Number(form.base_duration_seconds) || 90,
  }
}

export function filterCandidates(candidates, filters) {
  return [...(candidates || [])]
    .filter((item) => filters.showEliminated || !item.is_eliminated)
    .filter((item) => !filters.favoriteOnly || item.is_favorite)
    .filter((item) => Number(item.score?.total_score || 0) >= Number(filters.minimumScore || 0))
    .sort((a, b) => Number(b.score?.total_score || 0) - Number(a.score?.total_score || 0) || a.ordinal - b.ordinal)
}

export function buildCompositionParts(componentTypes, selections) {
  return componentTypes
    .filter((componentKey) => selections[componentKey])
    .map((componentKey) => ({ component_key: componentKey, candidate_id: selections[componentKey] }))
}

export function validationRuleLabels(summary) {
  const labels = {
    causality: '因果', duration: '时长', character_state: '人物状态',
    foreshadowing: '伏笔', continuity: '连续性',
  }
  return (summary?.results || []).map((item) => ({ ...item, label: labels[item.rule] || item.rule }))
}
