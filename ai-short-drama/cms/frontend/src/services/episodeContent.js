function normalizeAction(action) {
  if (typeof action === 'string') return { description: action }
  return {
    ...action,
    description: action?.description || action?.text || action?.visual || '',
  }
}

function buildActionPayload(action) {
  if (typeof action === 'string') return { description: action }
  const { description = '', ...rest } = action || {}
  if (Object.hasOwn(rest, 'text')) return { ...rest, text: description }
  if (Object.hasOwn(rest, 'visual')) return { ...rest, visual: description }
  return { ...rest, description }
}

export function cloneEpisodeContent(content) {
  // API content only contains JSON values. Serializing first also unwraps Vue's
  // reactive Proxy, which structuredClone cannot clone directly in browsers.
  const cloned = JSON.parse(JSON.stringify(content))
  if (cloned.script?.scenes) {
    cloned.script.scenes = cloned.script.scenes.map((scene) => ({
      ...scene,
      actions: Array.isArray(scene.actions)
        ? scene.actions.map(normalizeAction)
        : [],
      dialogues: Array.isArray(scene.dialogues) ? scene.dialogues.map((dialogue) => ({ ...dialogue })) : [],
    }))
  }
  return cloned
}

export function buildEpisodeContentPayload(content) {
  const outline = content.outline || {}
  const payload = {
    outline: {
      title: outline.title || '',
      logline: outline.logline || '',
      opening_hook: outline.opening_hook || '',
      story_goal: outline.story_goal || '',
      main_conflict: outline.main_conflict || '',
      climax: outline.climax || '',
      ending_hook: outline.ending_hook || '',
      estimated_duration_seconds: Number(outline.estimated_duration_seconds || 0),
    },
  }
  if (!content.script) return payload
  payload.script = {
    script_id: content.script.script_id,
    title: content.script.title || '',
    opening_hook: content.script.opening_hook || '',
    climax: content.script.climax || '',
    ending_hook: content.script.ending_hook || '',
    scenes: (content.script.scenes || []).map((scene) => ({
      scene_id: scene.scene_id,
      location_name: scene.location_name || '',
      time_of_day: scene.time_of_day || '',
      interior_exterior: scene.interior_exterior || '',
      scene_purpose: scene.scene_purpose || '',
      actions: Array.isArray(scene.actions)
        ? scene.actions.map(buildActionPayload)
        : [],
      emotional_change: scene.emotional_change || '',
      estimated_duration_seconds: Number(scene.estimated_duration_seconds || 0),
      dialogues: (scene.dialogues || []).map((dialogue) => ({
        dialogue_id: dialogue.dialogue_id,
        dialogue_type: dialogue.dialogue_type || 'dialogue',
        speaker_name: dialogue.speaker_name || '',
        text: dialogue.text || '',
        emotion: dialogue.emotion || '',
        performance_instruction: dialogue.performance_instruction || '',
        estimated_duration_ms: Number(dialogue.estimated_duration_ms || 0),
      })),
    })),
  }
  return payload
}

export function episodeContentChanged(original, draft) {
  return JSON.stringify(buildEpisodeContentPayload(original)) !== JSON.stringify(buildEpisodeContentPayload(draft))
}
