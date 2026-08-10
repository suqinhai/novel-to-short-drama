import assert from 'node:assert/strict'
import test from 'node:test'
import {
	batchReplaceScript, buildEpisodeContentPayload, calculateScriptMetrics, cloneEpisodeContent,
	copyScene, deleteDialogue, deleteScene, episodeContentChanged, insertAction, insertDialogue,
	insertScene, mergeSceneWithNext, moveDialogue, moveScene, splitScene, structuredScriptDiff,
	searchScript, validateStructuredScript,
} from '../src/services/episodeContent.js'

const fixture = {
  outline: {
    title: '第1集', logline: '危机出现', opening_hook: '门被撞开', story_goal: '找出真相',
    main_conflict: '双方对峙', climax: '身份揭晓', ending_hook: '电话响起',
    estimated_duration_seconds: 180, status: 'approved',
  },
  script: {
    script_id: 'script_1', title: '第1集', opening_hook: '门被撞开', climax: '身份揭晓',
    ending_hook: '电话响起', status: 'approved', scenes: [{
      scene_id: 'scene_1', scene_number: 1, location_name: '旧仓库', time_of_day: '夜',
      interior_exterior: '内景', scene_purpose: '建立危机',
		actions: ['主角冲向门口'], emotional_change: '警惕转为震惊',
		character_ids: ['character_1'], source_event_ids: ['event_1'],
		estimated_duration_seconds: 30, dialogues: [{
			dialogue_id: 'dialogue_1', sequence_number: 1, dialogue_type: 'dialogue', speaker_name: '主角',
        text: '谁在那里？', emotion: '警惕', performance_instruction: '压低声音',
        estimated_duration_ms: 1800, character_id: 'character_1',
      }],
    }],
  },
}

test('cloneEpisodeContent normalizes action strings without mutating source', () => {
  const cloned = cloneEpisodeContent(fixture)
	assert.deepEqual(cloned.script.scenes[0].actions, [{ action_id: 'action_scene_1_1', description: '主角冲向门口' }])
  cloned.script.scenes[0].actions[0].description = '修改'
  assert.equal(fixture.script.scenes[0].actions[0], '主角冲向门口')
})

test('cloneEpisodeContent accepts reactive proxy values', () => {
  const cloned = cloneEpisodeContent(new Proxy(fixture, {}))
  assert.equal(cloned.outline.title, fixture.outline.title)
	assert.deepEqual(cloned.script.scenes[0].actions, [{ action_id: 'action_scene_1_1', description: '主角冲向门口' }])
})

test('clone and payload preserve text-based action schemas', () => {
  const content = structuredClone(fixture)
  content.script.scenes[0].actions = [{ text: '主角冲向门口', adaptation_added: true }]
  const cloned = cloneEpisodeContent(content)
  assert.equal(cloned.script.scenes[0].actions[0].description, '主角冲向门口')
  cloned.script.scenes[0].actions[0].description = '主角停在门前'
	assert.deepEqual(buildEpisodeContentPayload(cloned).script.scenes[0].actions, [{
		action_id: 'action_scene_1_1',
		text: '主角停在门前',
    adaptation_added: true,
  }])
})

test('buildEpisodeContentPayload strips read-only fields', () => {
  const payload = buildEpisodeContentPayload(cloneEpisodeContent(fixture))
  assert.equal(payload.expected_version, 1)
  assert.equal(payload.outline.status, undefined)
  assert.equal(payload.script.status, undefined)
	assert.equal(payload.script.scenes[0].scene_number, 1)
	assert.equal(payload.script.scenes[0].dialogues[0].character_id, 'character_1')
	assert.deepEqual(payload.script.scenes[0].actions[0], { action_id: 'action_scene_1_1', description: '主角冲向门口' })
})

test('episodeContentChanged detects editable changes', () => {
  const draft = cloneEpisodeContent(fixture)
  assert.equal(episodeContentChanged(fixture, draft), false)
  draft.script.scenes[0].dialogues[0].text = '你是谁？'
  assert.equal(episodeContentChanged(fixture, draft), true)
})

test('buildEpisodeContentPayload sends the exact current revision for conflict detection', () => {
  const content = cloneEpisodeContent({ ...fixture, revision: 7 })
  assert.equal(buildEpisodeContentPayload(content).expected_version, 7)
})

test('scene structural operations keep scene_number unique and copy ids independently', () => {
	const draft = cloneEpisodeContent(fixture)
	const added = insertScene(draft, 0)
	assert.deepEqual(draft.script.scenes.map(scene => scene.scene_number), [1, 2])
	const copied = copyScene(draft, 'scene_1')
	assert.notEqual(copied.scene_id, 'scene_1')
	assert.notEqual(copied.dialogues[0].dialogue_id, 'dialogue_1')
	moveScene(draft, copied.scene_id, -1)
	assert.deepEqual(draft.script.scenes.map(scene => scene.scene_number), [1, 2, 3])
	assert.equal(splitScene(draft, 'scene_1', 1)?.scene_number > 0, true)
	assert.equal(mergeSceneWithNext(draft, 'scene_1'), true)
	assert.equal(deleteScene(draft, added.scene_id), true)
	assert.equal(new Set(draft.script.scenes.map(scene => scene.scene_number)).size, draft.script.scenes.length)
})

test('dialogues and actions can be inserted, deleted and reordered', () => {
	const draft = cloneEpisodeContent(fixture)
	const scene = draft.script.scenes[0]
	const second = insertDialogue(scene, 0, 'narration')
	assert.equal(moveDialogue(scene, second.dialogue_id, -1), true)
	assert.deepEqual(scene.dialogues.map(item => item.sequence_number), [1, 2])
	assert.equal(deleteDialogue(scene, second.dialogue_id), true)
	const action = insertAction(scene, 0)
	assert(action.action_id)
})

test('live metrics, diagnostics, batch replacement and version diff are structured', () => {
	const draft = cloneEpisodeContent(fixture)
	const metrics = calculateScriptMetrics(draft)
	assert(metrics.duration_ms > 0)
	assert(metrics.dialogue_duration_ms > 0)
	assert(metrics.action_ratio > 0)
	const matches = searchScript(draft, '主角')
	assert(matches.some(item => item.path === 'dialogue.dialogue_1.speaker_name'))
	assert.equal(matches.reduce((sum, item) => sum + item.count, 0), 2)
	assert.equal(batchReplaceScript(draft, '主角', '林夏'), 2)
	const issues = validateStructuredScript(draft, {
		characters: [{ character_id: 'character_2', name: '他人' }],
		events: [{ event_revision_id: 'event_1', location_name: '医院' }],
	})
	assert(issues.some(item => item.code === 'UNKNOWN_CHARACTER'))
	assert(issues.some(item => item.code === 'LOCATION_CONFLICT'))
	const diff = structuredScriptDiff(fixture, draft)
	assert(diff.some(item => item.path === 'dialogue.dialogue_1.speaker_name'))
})

test('version diff exposes dialogue moves and scene character presence', () => {
	const left = cloneEpisodeContent(fixture)
	const right = cloneEpisodeContent(fixture)
	const target = insertScene(right, 0)
	target.dialogues.push(right.script.scenes[0].dialogues.shift())
	right.script.scenes[0].character_ids = []
	const diff = structuredScriptDiff(left, right)
	assert(diff.some(item => item.path === 'dialogue.dialogue_1' && item.kind === 'move'))
	assert(diff.some(item => item.path === 'scene.scene_1.character_ids'))
})
