import test from 'node:test'
import assert from 'node:assert/strict'
import {
  dialogueConversionPlan, dialogueEditPlan, exactDialogueRebuildRange, normalizeWorkbench,
  sceneDragPlan, soundStyleReplacementPayload, templateApplyPayload, timelineLanes, timingValidationItems,
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
