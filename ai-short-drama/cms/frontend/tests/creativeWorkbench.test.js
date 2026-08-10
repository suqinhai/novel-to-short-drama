import test from 'node:test'
import assert from 'node:assert/strict'
import {
  dialogueConversionPlan, dialogueEditPlan, exactDialogueRebuildRange, normalizeWorkbench,
  reorderedShotIds, sceneDragPlan, shotMergeRequest, shotReorderRequest, shotSplitRequest,
  shotUpdateRequest, soundStyleReplacementPayload, templateApplyPayload, timelineLanes, timingValidationItems,
  timelineRestoreChangePlan, timelineSoundStyleChangePlan, timelineTemplateChangePlan,
} from '../src/services/creativeWorkbench.js'

test('统一工作台按现有实体 ID 组织场景、对白和镜头，不复制数据模型', () => {
  const result = normalizeWorkbench({
    scenes: [{ scene_id: 's2', scene_number: 2 }, { scene_id: 's1', scene_number: 1 }],
    dialogues: [
      { dialogue_id: 'd2', scene_id: 's2', sequence_number: 1 },
      { dialogue_id: 'd1', scene_id: 's1', sequence_number: 1 },
    ],
    shots: [{ shot_id: 'x2', shot_order: 2 }, { shot_id: 'x1', shot_order: 1 }],
  })
  assert.deepEqual(result.scenes.map(item => item.scene_id), ['s1', 's2'])
  assert.deepEqual(result.dialogues.map(item => item.dialogue_id), ['d1', 'd2'])
  assert.deepEqual(result.shots.map(item => item.shot_id), ['x1', 'x2'])
})

test('对白编辑与对白转旁白/动作都生成结构化局部修改计划', () => {
  const dialogue = { dialogue_id: 'dlg_1', version: 3 }
  assert.deepEqual(dialogueEditPlan(dialogue, '新台词').changes, [
    { operation: 'replace', field: 'text', value: '新台词' },
  ])
  assert.equal(dialogueConversionPlan(dialogue, 'narration').changes[0].value, 'narration')
  assert.equal(dialogueConversionPlan(dialogue, 'action').changes[0].value, 'action')
})

test('场景拖拽只计划 scene_number 并锁定剧情事实', () => {
  const plan = sceneDragPlan({ scene_id: 'scene_2', version: 1 }, 1)
  assert.deepEqual(plan.allowed_fields, ['scene_number'])
  assert.equal(plan.changes[0].value, 1)
  assert(plan.must_preserve.includes('剧情事实'))
})

test('镜头拖拽、拆分、合并和字段修改都生成 shot sequence 预览请求', () => {
  const first = {
    shot_id: 'shot_a', shot_order: 1, duration_seconds: 4, action_description: '抬手',
    character_ids: ['alice'], dialogue_ids: ['d1'], head_state: { pose: 'down' }, tail_state: { pose: 'middle' },
    action_phase: { start: 'start', end: 'middle' }, shot_size: 'wide', camera_angle: 'eye',
  }
  const second = {
    shot_id: 'shot_b', shot_order: 2, duration_seconds: 3, action_description: '落手',
    character_ids: ['bob'], dialogue_ids: ['d2'], head_state: { pose: 'middle' }, tail_state: { pose: 'end' },
    action_phase: { start: 'middle', end: 'end' }, shot_size: 'medium', camera_angle: 'eye',
  }
  const workspace = { shot_sequence_version: 4, shots: [first, second] }
  assert.deepEqual(reorderedShotIds(workspace.shots, 'shot_b', 'shot_a'), ['shot_b', 'shot_a'])
  assert.equal(shotReorderRequest(workspace, 'shot_b', 'shot_a').base_sequence_version, 4)
  assert.deepEqual(shotUpdateRequest(workspace, first, { shot_size: 'close_up' }).patch, { shot_size: 'close_up' })
  const split = shotSplitRequest(workspace, first, {
    first_action: '抬手上半', second_action: '抬手下半', first_duration: 2, second_duration: 2,
    first_dialogue_ids: [], second_dialogue_ids: ['d1'], bridge_state: { pose: 'bridge' }, bridge_phase: 'bridge',
  })
  assert.equal(split.operation, 'split')
  assert.deepEqual(split.shots.map(item => item.duration_seconds), [2, 2])
  assert.deepEqual(split.shots[0].tail_state, split.shots[1].head_state)
  const merge = shotMergeRequest(workspace, first, second)
  assert.equal(merge.operation, 'merge')
  assert.deepEqual(merge.shots[0].character_ids, ['alice', 'bob'])
  assert.deepEqual(merge.shots[0].dialogue_ids, ['d1', 'd2'])
})

test('对白精确重建范围不扩散到整集', () => {
  assert.deepEqual(exactDialogueRebuildRange({ start_ms: 1200, end_ms: 3400 }), {
    start_ms: 1200,
    end_ms: 3400,
    artifacts: ['dialogue_audio', 'subtitle_cue', 'storyboard_shot_interval', 'edit_timeline_interval'],
  })
})

test('模板切换按项目/单集覆盖并形成新的应用请求', () => {
  assert.deepEqual(templateApplyPayload('etv_action_v1', 'episode', { fast_cut_ratio: .8 }), {
    editing_template_version_id: 'etv_action_v1',
    scope: 'episode',
    override_config: { fast_cut_ratio: .8 },
    reason: 'creative_workbench_template_switch',
  })
})

test('整集声音风格替换创建版本化请求并保留旧音轨', () => {
  assert.deepEqual(soundStyleReplacementPayload(' cinematic_noir '), {
    to_style_group: 'cinematic_noir',
    reason: 'creative_workbench_whole_episode_sound_style_replace',
    actor: 'creative-workbench',
  })
})

test('口型校验复用已记录的对白 timing，时间线展示所有声画轨', () => {
  const workspace = {
    dialogues: [{ dialogue_id: 'd1', sequence_number: 1, speaker_name: '林夏' }],
    dialogue_timings: [{
      dialogue_id: 'd1', dialogue_audio_id: 'a1', shot_id: 'x1',
      start_ms: 100, end_ms: 900, audio_duration_ms: 800,
      target_lip_start_ms: 100, target_lip_end_ms: 900,
    }],
  }
  assert.equal(timingValidationItems(workspace)[0].dialogue_audio_id, 'a1')
  assert.deepEqual(timelineLanes([
    { track_type: 'bgm', timeline_start_ms: 0 },
    { track_type: 'dialogue', timeline_start_ms: 100 },
  ]).map(item => item.label), ['BGM', '对白'])
})

test('template, sound style and restore all produce timeline change plans', () => {
  const current = { episode_id: 'episode-1', timeline_id: 'timeline-3', version: 3 }
  const template = timelineTemplateChangePlan(current, 'template-v2', 'episode', { pace: 'fast' })
  assert.equal(template.target.entity_type, 'timeline')
  assert.equal(template.target.entity_id, 'episode-1')
  assert.equal(template.target.version, 3)
  assert.deepEqual(template.changes.map(change => change.field), [
    'editing_template_version_id', 'template_scope', 'override_config',
  ])

  const sound = timelineSoundStyleChangePlan(current, ' cinematic_noir ')
  assert.equal(sound.changes[0].field, 'sound_style_group')
  assert.equal(sound.changes[0].value, 'cinematic_noir')

  const restore = timelineRestoreChangePlan(current, { timeline_id: 'timeline-1', version: 1 })
  assert.equal(restore.changes[0].field, 'restore_source_timeline_id')
  assert.equal(restore.changes[0].value, 'timeline-1')
})
