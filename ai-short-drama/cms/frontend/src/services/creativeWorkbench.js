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

export function reorderedShotIds(shots, sourceId, targetId) {
  const result = [...(shots || [])].sort((left, right) => left.shot_order - right.shot_order)
  const sourceIndex = result.findIndex(item => item.shot_id === sourceId)
  const targetIndex = result.findIndex(item => item.shot_id === targetId)
  if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return result.map(item => item.shot_id)
  const [source] = result.splice(sourceIndex, 1)
  result.splice(result.findIndex(item => item.shot_id === targetId), 0, source)
  return result.map(item => item.shot_id)
}

export function shotReorderRequest(workbench, sourceId, targetId) {
  return {
    operation: 'reorder',
    base_sequence_version: Number(workbench?.shot_sequence_version || 1),
    ordered_shot_ids: reorderedShotIds(workbench?.shots, sourceId, targetId),
    requested_by: 'creative-workbench',
  }
}

export function shotUpdateRequest(workbench, shot, patch) {
  return {
    operation: 'update', base_sequence_version: Number(workbench?.shot_sequence_version || 1),
    shot_id: shot.shot_id, patch, requested_by: 'creative-workbench',
  }
}

export function shotSplitRequest(workbench, shot, form) {
  const first = structuredClone(shot)
  const second = structuredClone(shot)
  const bridge = form.bridge_state || {}
  Object.assign(first, {
    action_description: String(form.first_action || '').trim(), duration_seconds: Number(form.first_duration),
    dialogue_ids: form.first_dialogue_ids || [], tail_state: bridge,
    action_phase: { ...(shot.action_phase || {}), end: String(form.bridge_phase || 'bridge') },
    coverage_role: form.first_coverage_role || shot.coverage_role || '',
  })
  Object.assign(second, {
    action_description: String(form.second_action || '').trim(), duration_seconds: Number(form.second_duration),
    dialogue_ids: form.second_dialogue_ids || [], head_state: bridge,
    action_phase: { ...(shot.action_phase || {}), start: String(form.bridge_phase || 'bridge') },
    coverage_role: form.second_coverage_role || shot.coverage_role || '',
  })
  return {
    operation: 'split', base_sequence_version: Number(workbench?.shot_sequence_version || 1),
    shot_id: shot.shot_id, shots: [first, second], requested_by: 'creative-workbench',
  }
}

export function shotMergeRequest(workbench, left, right, form = {}) {
  const merged = structuredClone(left)
  Object.assign(merged, {
    action_description: String(form.action_description || `${left.action_description}；${right.action_description}`).trim(),
    duration_seconds: Number(form.duration_seconds ?? (Number(left.duration_seconds) + Number(right.duration_seconds))),
    shot_size: form.shot_size || left.shot_size, camera_angle: form.camera_angle || left.camera_angle,
    composition: form.composition || left.composition, camera_motion: form.camera_motion || left.camera_motion,
    character_ids: [...new Set([...(left.character_ids || []), ...(right.character_ids || [])])],
    dialogue_ids: [...(left.dialogue_ids || []), ...(right.dialogue_ids || [])],
    head_state: left.head_state || {}, tail_state: right.tail_state || {},
    action_phase: { start: left.action_phase?.start || '', end: right.action_phase?.end || '' },
    coverage_role: form.coverage_role || left.coverage_role || '',
  })
  return {
    operation: 'merge', base_sequence_version: Number(workbench?.shot_sequence_version || 1),
    shot_ids: [left.shot_id, right.shot_id], shots: [merged], requested_by: 'creative-workbench',
  }
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

export function timelineTemplateChangePlan(timeline, templateVersionId, scope, overrideConfig = {}) {
  return {
    instruction: `切换剪辑模板并创建 successor 时间线；旧时间线保持可查看`,
    target: {
      entity_type: 'timeline',
      entity_id: timeline.episode_id,
      version: Number(timeline.version || 1),
    },
    allowed_fields: ['editing_template_version_id', 'template_scope', 'override_config'],
    changes: [
      { operation: 'replace', field: 'editing_template_version_id', value: templateVersionId },
      { operation: 'replace', field: 'template_scope', value: scope === 'project' ? 'project' : 'episode' },
      { operation: 'replace', field: 'override_config', value: overrideConfig },
    ],
    must_preserve: ['原时间线', '原媒体资产', '已批准版本'],
    locks: ['character', 'location', 'composition'],
  }
}

export function timelineRestoreChangePlan(current, source) {
  return {
    instruction: `从历史时间线 v${source.version} 创建新的恢复版本，不覆盖历史`,
    target: {
      entity_type: 'timeline',
      entity_id: current.episode_id,
      version: Number(current.version || 1),
    },
    allowed_fields: ['restore_source_timeline_id'],
    changes: [{
      operation: 'replace', field: 'restore_source_timeline_id', value: source.timeline_id,
    }],
    must_preserve: ['所有历史时间线', '原媒体资产'],
  }
}

export function timelineSoundStyleChangePlan(timeline, styleGroup) {
  return {
    instruction: `将整集声音风格改为 ${String(styleGroup || '').trim()}，只创建待重建的 successor 时间线`,
    target: {
      entity_type: 'timeline',
      entity_id: timeline.episode_id,
      version: Number(timeline.version || 1),
    },
    allowed_fields: ['sound_style_group'],
    changes: [{
      operation: 'replace', field: 'sound_style_group', value: String(styleGroup || '').trim(),
    }],
    must_preserve: ['原声音资产', '原 cue 版本', '原时间线'],
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
