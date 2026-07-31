export const templateLabels = {
  urban_power: '都市爽剧',
  emotion: '情感剧',
  suspense: '悬疑剧',
  comedy: '喜剧',
  action: '动作剧',
}

export const soundTrackLabels = {
  bgm: 'BGM',
  ambience: '环境声',
  sound_effect: '音效',
  footstep: '脚步',
  door: '门响',
  fight: '打斗',
  dialogue: '对白',
  narration: '旁白',
  subtitle: '字幕',
  video: '画面',
}

const arrays = [
  'pacing_beats', 'candidates', 'scenes', 'dialogues', 'shots',
  'dialogue_timings', 'sound_cues', 'timeline_versions', 'timeline_items',
  'performance_bibles', 'continuity', 'quality_issues', 'visual_qc_issues',
  'dialogue_timing_issues', 'comments', 'workspace_versions', 'template_bindings',
]

export function normalizeWorkbench(value = {}) {
  const result = { ...value }
  for (const key of arrays) result[key] = Array.isArray(value[key]) ? value[key] : []
  result.scenes = [...result.scenes].sort((left, right) => left.scene_number - right.scene_number)
  result.dialogues = [...result.dialogues].sort((left, right) =>
    (left.scene_id === right.scene_id ? left.sequence_number - right.sequence_number
      : sceneOrder(result.scenes, left.scene_id) - sceneOrder(result.scenes, right.scene_id)))
  result.shots = [...result.shots].sort((left, right) => left.shot_order - right.shot_order)
  result.timeline_items = [...result.timeline_items].sort((left, right) =>
    left.timeline_start_ms - right.timeline_start_ms || left.sequence_number - right.sequence_number)
  return result
}

export function dialoguesForScene(workbench, sceneId) {
  return (workbench?.dialogues || []).filter((item) => item.scene_id === sceneId)
}

export function shotsForScene(workbench, sceneId) {
  return (workbench?.shots || []).filter((item) => item.scene_id === sceneId)
}

export function sceneDragPlan(scene, targetNumber) {
  return {
    instruction: `将场景换序到第 ${targetNumber} 场，保留人物、剧情事实与连续性`,
    target: { entity_type: 'scene', entity_id: scene.scene_id, version: Number(scene.version || 1) },
    allowed_fields: ['scene_number'],
    changes: [{ operation: 'reorder', field: 'scene_number', value: Number(targetNumber) }],
    must_preserve: ['剧情事实', '人物关系', '连续性输入输出'],
  }
}

export function dialogueEditPlan(dialogue, text) {
  return {
    instruction: `逐句修改对白 ${dialogue.dialogue_id}；只重建关联配音、字幕、镜头区间和剪辑区间`,
    target: { entity_type: 'dialogue', entity_id: dialogue.dialogue_id, version: Number(dialogue.version || 1) },
    allowed_fields: ['text'],
    changes: [{ operation: 'replace', field: 'text', value: String(text).trim() }],
    must_preserve: ['说话人', '人物关系', '场景事实'],
  }
}

export function dialogueConversionPlan(dialogue, mode) {
  const label = mode === 'narration' ? '旁白' : mode === 'action' ? '动作' : '对白'
  return {
    instruction: `将台词 ${dialogue.dialogue_id} 转为${label}，保留语义与来源证据`,
    target: { entity_type: 'dialogue', entity_id: dialogue.dialogue_id, version: Number(dialogue.version || 1) },
    allowed_fields: ['production_mode'],
    changes: [{ operation: 'replace', field: 'production_mode', value: mode }],
    must_preserve: ['台词语义', 'Source Span', 'IR Fact'],
  }
}

export function timingValidationItems(workbench) {
  const byDialogue = new Map((workbench?.dialogue_timings || []).map((item) => [item.dialogue_id, item]))
  return (workbench?.dialogues || []).flatMap((dialogue) => {
    const timing = byDialogue.get(dialogue.dialogue_id)
    if (!timing) return []
    return [{
      dialogue_id: dialogue.dialogue_id,
      dialogue_audio_id: timing.dialogue_audio_id,
      shot_id: timing.shot_id,
      speaker_character_id: timing.speaker_character_id || dialogue.character_id || '',
      speaker_name: timing.speaker_name || dialogue.speaker_name || '',
      turn_group: timing.turn_group || '',
      turn_index: Number(timing.turn_index || dialogue.sequence_number),
      start_ms: Number(timing.start_ms),
      end_ms: Number(timing.end_ms),
      audio_duration_ms: Number(timing.audio_duration_ms),
      target_lip_start_ms: Number(timing.target_lip_start_ms),
      target_lip_end_ms: Number(timing.target_lip_end_ms),
      visible_character_ids: timing.visible_character_ids || [],
      detected_speaker_id: timing.detected_speaker_id || '',
      detected_lip_start_ms: Number(timing.detected_lip_start_ms || timing.target_lip_start_ms),
      detected_lip_end_ms: Number(timing.detected_lip_end_ms || timing.target_lip_end_ms),
      confidence: Number(timing.confidence || 0),
      analyzer_version: timing.analyzer_version || 'cms-lipsync-v1',
    }]
  })
}

export function issueEditLink(projectId, issue) {
  if (issue?.editor_link?.editor_path) {
    return issue.editor_link.editor_path
  }
  const type = issue?.dialogue_id ? 'dialogue' : issue?.shot_id ? 'shot' : 'scene'
  const id = issue?.dialogue_id || issue?.shot_id || issue?.scene_id || issue?.entity_id || ''
  const version = issue?.entity_version || 1
  return `/projects/${encodeURIComponent(projectId)}/local-edit?entity_type=${type}&entity_id=${encodeURIComponent(id)}&version=${version}`
}

export function timelineLanes(items = []) {
  const groups = new Map()
  for (const item of items) {
    const key = item.track_type || 'other'
    if (!groups.has(key)) groups.set(key, [])
    groups.get(key).push(item)
  }
  return [...groups.entries()].map(([type, entries]) => ({
    type,
    label: soundTrackLabels[type] || type,
    entries: [...entries].sort((left, right) => left.timeline_start_ms - right.timeline_start_ms),
  }))
}

export function templateApplyPayload(templateVersionId, scope, overrideConfig = {}) {
  return {
    editing_template_version_id: templateVersionId,
    scope: scope === 'project' ? 'project' : 'episode',
    override_config: overrideConfig,
    reason: 'creative_workbench_template_switch',
  }
}

export function soundStyleReplacementPayload(toStyleGroup) {
  return {
    to_style_group: String(toStyleGroup || '').trim(),
    reason: 'creative_workbench_whole_episode_sound_style_replace',
    actor: 'creative-workbench',
  }
}

export function exactDialogueRebuildRange(timing) {
  if (!timing) return null
  return {
    start_ms: Number(timing.start_ms),
    end_ms: Number(timing.end_ms),
    artifacts: ['dialogue_audio', 'subtitle_cue', 'storyboard_shot_interval', 'edit_timeline_interval'],
  }
}

function sceneOrder(scenes, sceneId) {
  return scenes.find((item) => item.scene_id === sceneId)?.scene_number ?? Number.MAX_SAFE_INTEGER
}
