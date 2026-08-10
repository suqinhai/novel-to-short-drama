function normalizeAction(action, sceneId, index) {
	if (typeof action === 'string') return { action_id: `action_${sceneId}_${index + 1}`, description: action }
	return {
		...action,
		action_id: action?.action_id || `action_${sceneId}_${index + 1}`,
		description: action?.description || action?.text || action?.visual || '',
	}
}

function buildActionPayload(action, sceneId, index) {
	if (typeof action === 'string') return { action_id: `action_${sceneId}_${index + 1}`, description: action }
	const { description = '', ...rest } = action || {}
	rest.action_id ||= `action_${sceneId}_${index + 1}`
  if (Object.hasOwn(rest, 'text')) return { ...rest, text: description }
  if (Object.hasOwn(rest, 'visual')) return { ...rest, visual: description }
  return { ...rest, description }
}

export function cloneEpisodeContent(content) {
  // API content only contains JSON values. Serializing first also unwraps Vue's
  // reactive Proxy, which structuredClone cannot clone directly in browsers.
  const cloned = JSON.parse(JSON.stringify(content))
	if (cloned.script?.scenes) {
		cloned.script.scenes = cloned.script.scenes.map((scene, sceneIndex) => ({
			...scene,
			scene_number: Number(scene.scene_number || sceneIndex + 1),
			character_ids: Array.isArray(scene.character_ids) ? [...scene.character_ids] : [],
			source_event_ids: Array.isArray(scene.source_event_ids) ? [...scene.source_event_ids] : [],
			actions: Array.isArray(scene.actions)
				? scene.actions.map((action, actionIndex) => normalizeAction(action, scene.scene_id, actionIndex))
				: [],
			dialogues: Array.isArray(scene.dialogues) ? scene.dialogues.map((dialogue, dialogueIndex) => ({
				...dialogue,
				sequence_number: Number(dialogue.sequence_number || dialogueIndex + 1),
			})) : [],
		}))
  }
  return cloned
}

export function buildEpisodeContentPayload(content) {
  const outline = content.outline || {}
  const payload = {
    expected_version: Number(content.revision || content.script?.version || outline.version || 1),
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
			scene_number: Number(scene.scene_number || 0),
			location_id: scene.location_id || null,
			location_name: scene.location_name || '',
      time_of_day: scene.time_of_day || '',
			interior_exterior: scene.interior_exterior || '',
			character_ids: Array.isArray(scene.character_ids) ? scene.character_ids : [],
			scene_purpose: scene.scene_purpose || '',
			actions: Array.isArray(scene.actions)
				? scene.actions.map((action, actionIndex) => buildActionPayload(action, scene.scene_id, actionIndex))
        : [],
			emotional_change: scene.emotional_change || '',
			estimated_duration_seconds: Number(scene.estimated_duration_seconds || 0),
			source_event_ids: Array.isArray(scene.source_event_ids) ? scene.source_event_ids : [],
			dialogues: (scene.dialogues || []).map((dialogue) => ({
				dialogue_id: dialogue.dialogue_id,
				sequence_number: Number(dialogue.sequence_number || 0),
				dialogue_type: dialogue.dialogue_type || 'dialogue',
				character_id: dialogue.character_id || null,
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

let fallbackId = 0

export function newEditorId(prefix) {
	const uuid = globalThis.crypto?.randomUUID?.()
	if (uuid) return `${prefix}_${uuid.replaceAll('-', '')}`
	fallbackId += 1
	return `${prefix}_${Date.now().toString(36)}_${fallbackId.toString(36)}`
}

export function normalizeScriptOrder(content) {
	const scenes = content?.script?.scenes || []
	scenes.forEach((scene, sceneIndex) => {
		scene.scene_number = sceneIndex + 1
		;(scene.dialogues || []).forEach((dialogue, dialogueIndex) => {
			dialogue.sequence_number = dialogueIndex + 1
		})
		;(scene.actions || []).forEach((action, actionIndex) => {
			action.action_id ||= `action_${scene.scene_id}_${actionIndex + 1}`
		})
	})
	return content
}

function blankScene(number, source = {}) {
	const sceneId = newEditorId('scene')
	return {
		scene_id: sceneId,
		scene_number: number,
		location_id: source.location_id || null,
		location_name: source.location_name || '',
		time_of_day: source.time_of_day || '',
		interior_exterior: source.interior_exterior || '',
		character_ids: [...(source.character_ids || [])],
		scene_purpose: '', actions: [], emotional_change: '',
		estimated_duration_seconds: 1,
		source_event_ids: [...(source.source_event_ids || [])],
		dialogues: [],
	}
}

export function insertScene(content, afterIndex = -1) {
	const scenes = content.script.scenes
	const index = Math.max(0, Math.min(scenes.length, Number(afterIndex) + 1))
	const source = scenes[Math.max(0, Math.min(scenes.length - 1, Number(afterIndex)))] || {}
	const scene = blankScene(index + 1, source)
	scenes.splice(index, 0, scene)
	normalizeScriptOrder(content)
	return scene
}

export function deleteScene(content, sceneId) {
	const scenes = content.script.scenes
	const index = scenes.findIndex(scene => scene.scene_id === sceneId)
	if (index < 0) return false
	scenes.splice(index, 1)
	normalizeScriptOrder(content)
	return true
}

export function copyScene(content, sceneId) {
	const scenes = content.script.scenes
	const index = scenes.findIndex(scene => scene.scene_id === sceneId)
	if (index < 0) return null
	const scene = JSON.parse(JSON.stringify(scenes[index]))
	scene.scene_id = newEditorId('scene')
	scene.actions = (scene.actions || []).map(action => ({ ...action, action_id: newEditorId('action') }))
	scene.dialogues = (scene.dialogues || []).map(dialogue => ({ ...dialogue, dialogue_id: newEditorId('dialogue') }))
	scenes.splice(index + 1, 0, scene)
	normalizeScriptOrder(content)
	return scene
}

export function splitScene(content, sceneId, dialogueIndex) {
	const scenes = content.script.scenes
	const index = scenes.findIndex(scene => scene.scene_id === sceneId)
	if (index < 0) return null
	const source = scenes[index]
	const splitAt = Math.max(0, Math.min(source.dialogues.length, Number(dialogueIndex)))
	const scene = blankScene(index + 2, source)
	scene.scene_purpose = source.scene_purpose ? `${source.scene_purpose}（续）` : ''
	scene.emotional_change = source.emotional_change
	scene.dialogues = source.dialogues.splice(splitAt)
	scene.estimated_duration_seconds = Math.max(1, Math.round(Number(source.estimated_duration_seconds || 2) / 2))
	source.estimated_duration_seconds = Math.max(1, Number(source.estimated_duration_seconds || 2) - scene.estimated_duration_seconds)
	scenes.splice(index + 1, 0, scene)
	normalizeScriptOrder(content)
	return scene
}

export function mergeSceneWithNext(content, sceneId) {
	const scenes = content.script.scenes
	const index = scenes.findIndex(scene => scene.scene_id === sceneId)
	if (index < 0 || index >= scenes.length - 1) return false
	const target = scenes[index]
	const next = scenes[index + 1]
	target.actions.push(...next.actions)
	target.dialogues.push(...next.dialogues)
	target.scene_purpose = [target.scene_purpose, next.scene_purpose].filter(Boolean).join('；')
	target.emotional_change = [target.emotional_change, next.emotional_change].filter(Boolean).join(' → ')
	target.estimated_duration_seconds = Number(target.estimated_duration_seconds || 0) + Number(next.estimated_duration_seconds || 0)
	target.character_ids = [...new Set([...(target.character_ids || []), ...(next.character_ids || [])])]
	target.source_event_ids = [...new Set([...(target.source_event_ids || []), ...(next.source_event_ids || [])])]
	scenes.splice(index + 1, 1)
	normalizeScriptOrder(content)
	return true
}

export function moveScene(content, sceneId, delta) {
	const scenes = content.script.scenes
	const index = scenes.findIndex(scene => scene.scene_id === sceneId)
	const target = index + Number(delta)
	if (index < 0 || target < 0 || target >= scenes.length) return false
	const [scene] = scenes.splice(index, 1)
	scenes.splice(target, 0, scene)
	normalizeScriptOrder(content)
	return true
}

export function insertDialogue(scene, afterIndex = -1, type = 'dialogue') {
	const dialogue = {
		dialogue_id: newEditorId('dialogue'), sequence_number: 1,
		dialogue_type: type, character_id: null, speaker_name: '', text: '新内容',
		emotion: '', performance_instruction: '', estimated_duration_ms: 500,
	}
	const index = Math.max(0, Math.min(scene.dialogues.length, Number(afterIndex) + 1))
	scene.dialogues.splice(index, 0, dialogue)
	scene.dialogues.forEach((item, itemIndex) => { item.sequence_number = itemIndex + 1 })
	return dialogue
}

export function deleteDialogue(scene, dialogueId) {
	const index = scene.dialogues.findIndex(item => item.dialogue_id === dialogueId)
	if (index < 0) return false
	scene.dialogues.splice(index, 1)
	scene.dialogues.forEach((item, itemIndex) => { item.sequence_number = itemIndex + 1 })
	return true
}

export function moveDialogue(scene, dialogueId, delta) {
	const index = scene.dialogues.findIndex(item => item.dialogue_id === dialogueId)
	const target = index + Number(delta)
	if (index < 0 || target < 0 || target >= scene.dialogues.length) return false
	const [dialogue] = scene.dialogues.splice(index, 1)
	scene.dialogues.splice(target, 0, dialogue)
	scene.dialogues.forEach((item, itemIndex) => { item.sequence_number = itemIndex + 1 })
	return true
}

export function insertAction(scene, afterIndex = -1) {
	const action = { action_id: newEditorId('action'), description: '新动作' }
	const index = Math.max(0, Math.min(scene.actions.length, Number(afterIndex) + 1))
	scene.actions.splice(index, 0, action)
	return action
}

export function deleteAction(scene, actionId) {
	const index = scene.actions.findIndex(item => item.action_id === actionId)
	if (index < 0) return false
	scene.actions.splice(index, 1)
	return true
}

export function moveAction(scene, actionId, delta) {
	const index = scene.actions.findIndex(item => item.action_id === actionId)
	const target = index + Number(delta)
	if (index < 0 || target < 0 || target >= scene.actions.length) return false
	const [action] = scene.actions.splice(index, 1)
	scene.actions.splice(target, 0, action)
	return true
}

export function estimateDialogueDurationMS(text) {
	const value = String(text || '').trim()
	if (!value) return 0
	const punctuationPauses = (value.match(/[，。！？；,.!?;]/g) || []).length * 110
	return Math.max(500, Math.round([...value].length * 220 + punctuationPauses))
}

export function calculateScriptMetrics(content) {
	const scenes = (content?.script?.scenes || []).map((scene) => {
		const dialogueDurationMS = (scene.dialogues || []).reduce((sum, dialogue) =>
			sum + estimateDialogueDurationMS(dialogue.text), 0)
		const actionDurationMS = (scene.actions || []).reduce((sum, action) =>
			sum + Math.max(700, [...String(action.description || '')].length * 170), 0)
		const durationMS = dialogueDurationMS + actionDurationMS
		return {
			scene_id: scene.scene_id,
			dialogue_duration_ms: dialogueDurationMS,
			action_duration_ms: actionDurationMS,
			duration_ms: durationMS,
			action_ratio: durationMS ? actionDurationMS / durationMS : 0,
		}
	})
	return {
		scenes,
		duration_ms: scenes.reduce((sum, scene) => sum + scene.duration_ms, 0),
		dialogue_duration_ms: scenes.reduce((sum, scene) => sum + scene.dialogue_duration_ms, 0),
		action_duration_ms: scenes.reduce((sum, scene) => sum + scene.action_duration_ms, 0),
		action_ratio: scenes.reduce((sum, scene) => sum + scene.duration_ms, 0)
			? scenes.reduce((sum, scene) => sum + scene.action_duration_ms, 0) / scenes.reduce((sum, scene) => sum + scene.duration_ms, 0)
			: 0,
	}
}

export function validateStructuredScript(content, referenceContext = {}) {
	const issues = []
	const characters = referenceContext.characters || []
	const characterIDs = new Set(characters.map(item => item.character_id))
	const characterNames = new Set(characters.map(item => item.name).filter(Boolean))
	const events = new Map((referenceContext.events || []).map(item => [item.event_revision_id, item]))
	const repeated = new Map()
	for (const scene of content?.script?.scenes || []) {
		const present = new Set(scene.character_ids || [])
		if (!String(scene.scene_purpose || '').trim() || !String(scene.emotional_change || '').trim()) {
			issues.push({ code: 'MOTIVATION_MISSING', severity: 'warning', scene_id: scene.scene_id, message: '场景目的或情绪动机缺失。' })
		}
		const sourceLocations = [...new Set((scene.source_event_ids || []).map(id => events.get(id)?.location_name).filter(Boolean))]
		if (scene.location_name && sourceLocations.length && !sourceLocations.some(location =>
			scene.location_name.includes(location) || location.includes(scene.location_name))) {
			issues.push({ code: 'LOCATION_CONFLICT', severity: 'warning', scene_id: scene.scene_id, message: `地点“${scene.location_name}”与原著事件地点“${sourceLocations.join(' / ')}”冲突。` })
		}
		for (const dialogue of scene.dialogues || []) {
			const speakingType = dialogue.dialogue_type === 'dialogue' || dialogue.dialogue_type === 'inner_monologue'
			if (speakingType && dialogue.character_id && !present.has(dialogue.character_id)) {
				issues.push({ code: 'CHARACTER_NOT_PRESENT', severity: 'blocking', scene_id: scene.scene_id, dialogue_id: dialogue.dialogue_id, message: `${dialogue.speaker_name || dialogue.character_id} 不在场却说话。` })
			}
			if (dialogue.dialogue_type !== 'narration' && characterIDs.size && ((dialogue.character_id && !characterIDs.has(dialogue.character_id)) ||
				(!dialogue.character_id && dialogue.speaker_name && characterNames.size && !characterNames.has(dialogue.speaker_name)))) {
				issues.push({ code: 'UNKNOWN_CHARACTER', severity: 'blocking', scene_id: scene.scene_id, dialogue_id: dialogue.dialogue_id, message: `未知角色：${dialogue.speaker_name || dialogue.character_id}` })
			}
			const normalized = normalizeRepeatedText(dialogue.text)
			if (normalized) repeated.set(normalized, [...(repeated.get(normalized) || []), { scene_id: scene.scene_id, dialogue_id: dialogue.dialogue_id }])
		}
		for (const action of scene.actions || []) {
			const normalized = normalizeRepeatedText(action.description)
			if (normalized) repeated.set(normalized, [...(repeated.get(normalized) || []), { scene_id: scene.scene_id, action_id: action.action_id }])
		}
	}
	for (const [text, locations] of repeated) {
		if (locations.length > 1) issues.push({ code: 'REPEATED_INFORMATION', severity: 'warning', message: `信息重复 ${locations.length} 次：${text.slice(0, 24)}`, locations })
	}
	return issues
}

function normalizeRepeatedText(value) {
	const normalized = String(value || '').toLocaleLowerCase().replace(/[\s，。！？；、,.!?;:'"“”‘’]/g, '')
	return normalized.length >= 4 ? normalized : ''
}

export function batchReplaceScript(content, search, replacement, options = {}) {
	const needle = String(search || '')
	if (!needle) return 0
	const flags = options.caseSensitive ? 'g' : 'gi'
	const pattern = new RegExp(needle.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), flags)
	let count = 0
	const replace = value => String(value || '').replace(pattern, match => { count += 1; return replacement })
	for (const scene of content?.script?.scenes || []) {
		scene.location_name = replace(scene.location_name)
		scene.scene_purpose = replace(scene.scene_purpose)
		scene.emotional_change = replace(scene.emotional_change)
		for (const action of scene.actions || []) action.description = replace(action.description)
		for (const dialogue of scene.dialogues || []) {
			dialogue.speaker_name = replace(dialogue.speaker_name)
			dialogue.text = replace(dialogue.text)
			dialogue.performance_instruction = replace(dialogue.performance_instruction)
		}
	}
	return count
}

export function searchScript(content, query, options = {}) {
	const needle = String(query || '')
	if (!needle) return []
	const normalizedNeedle = options.caseSensitive ? needle : needle.toLocaleLowerCase()
	const results = []
	const inspect = (path, value, location = {}) => {
		const text = String(value || '')
		const haystack = options.caseSensitive ? text : text.toLocaleLowerCase()
		let from = 0
		const offsets = []
		while (from <= haystack.length - normalizedNeedle.length && results.length < 200) {
			const index = haystack.indexOf(normalizedNeedle, from)
			if (index < 0) break
			offsets.push(index)
			from = index + Math.max(1, normalizedNeedle.length)
		}
		if (offsets.length) results.push({ path, text, offsets, count: offsets.length, ...location })
	}
	inspect('script.title', content?.script?.title)
	inspect('script.opening_hook', content?.script?.opening_hook)
	inspect('script.climax', content?.script?.climax)
	inspect('script.ending_hook', content?.script?.ending_hook)
	for (const scene of content?.script?.scenes || []) {
		const sceneLocation = { scene_id: scene.scene_id, scene_number: scene.scene_number }
		for (const field of ['location_name', 'scene_purpose', 'emotional_change']) {
			inspect(`scene.${scene.scene_id}.${field}`, scene[field], sceneLocation)
		}
		for (const action of scene.actions || []) {
			inspect(`action.${action.action_id}.description`, action.description, {
				...sceneLocation, action_id: action.action_id,
			})
		}
		for (const dialogue of scene.dialogues || []) {
			for (const field of ['speaker_name', 'text', 'performance_instruction']) {
				inspect(`dialogue.${dialogue.dialogue_id}.${field}`, dialogue[field], {
					...sceneLocation, dialogue_id: dialogue.dialogue_id,
				})
			}
		}
	}
	return results
}

export function structuredScriptDiff(left, right) {
	const leftPayload = buildEpisodeContentPayload(left)
	const rightPayload = buildEpisodeContentPayload(right)
	const rows = []
	const add = (path, before, after, kind = 'replace') => {
		if (JSON.stringify(before) !== JSON.stringify(after)) rows.push({ path, before, after, kind })
	}
	for (const field of ['title', 'opening_hook', 'climax', 'ending_hook']) add(`script.${field}`, leftPayload.script?.[field], rightPayload.script?.[field])
	const leftScenes = new Map((leftPayload.script?.scenes || []).map(scene => [scene.scene_id, scene]))
	const rightScenes = new Map((rightPayload.script?.scenes || []).map(scene => [scene.scene_id, scene]))
	for (const [id, scene] of leftScenes) if (!rightScenes.has(id)) add(`scene.${id}`, scene, null, 'remove')
	for (const [id, scene] of rightScenes) if (!leftScenes.has(id)) add(`scene.${id}`, null, scene, 'insert')
	for (const [id, before] of leftScenes) {
		const after = rightScenes.get(id)
		if (!after) continue
		for (const field of ['scene_number', 'location_name', 'time_of_day', 'interior_exterior', 'character_ids', 'scene_purpose', 'actions', 'emotional_change', 'estimated_duration_seconds']) add(`scene.${id}.${field}`, before[field], after[field], field === 'scene_number' ? 'reorder' : 'replace')
	}
	const dialogues = scenes => new Map([...scenes.values()].flatMap(scene => scene.dialogues.map(dialogue => [
		dialogue.dialogue_id, { ...dialogue, scene_id: scene.scene_id },
	])))
	const leftDialogues = dialogues(leftScenes)
	const rightDialogues = dialogues(rightScenes)
	for (const [id, dialogue] of leftDialogues) if (!rightDialogues.has(id)) add(`dialogue.${id}`, dialogue, null, 'remove')
	for (const [id, dialogue] of rightDialogues) if (!leftDialogues.has(id)) add(`dialogue.${id}`, null, dialogue, 'insert')
	for (const [id, before] of leftDialogues) {
		const after = rightDialogues.get(id)
		if (!after) continue
		if (before.scene_id !== after.scene_id) add(`dialogue.${id}`, before, after, 'move')
		for (const field of ['sequence_number', 'dialogue_type', 'speaker_name', 'text', 'emotion', 'performance_instruction', 'estimated_duration_ms']) add(`dialogue.${id}.${field}`, before[field], after[field], field === 'sequence_number' ? 'reorder' : 'replace')
	}
	return rows
}
