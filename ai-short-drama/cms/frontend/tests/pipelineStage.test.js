import assert from 'node:assert/strict'
import test from 'node:test'
import { getPipelineProgress, getPipelineStageIndex, getPipelineStageLabel, getStageUnitProgress, pipelineStages } from '../src/services/pipelineStage.js'

test('maps stage 4 video and audio states to their visible pipeline steps', () => {
  assert.equal(getPipelineStageIndex('video_processing'), 8)
  assert.equal(getPipelineStageIndex('shot_videos_approved'), 8)
  assert.equal(getPipelineStageIndex('voice_profile_review'), 9)
  assert.equal(getPipelineStageIndex('audio_processing'), 9)
  assert.equal(getPipelineStageIndex('stage_4_completed'), 9)
})

test('maps review, render and publish states across the full pipeline', () => {
  assert.equal(getPipelineStageIndex('adaptation_planning'), 3)
  assert.equal(getPipelineStageLabel('adaptation_planning'), '等待编译并采用改编计划')
  assert.equal(getPipelineStageIndex('season_outline_review'), 3)
  assert.equal(getPipelineStageIndex('storyboard_review'), 5)
  assert.equal(getPipelineStageIndex('storyboard_image_review'), 7)
  assert.equal(getPipelineStageIndex('preview_rendered'), 10)
  assert.equal(getPipelineStageIndex('waiting_final_review'), 11)
  assert.equal(getPipelineStageIndex('published'), pipelineStages.length)
  assert.equal(getPipelineStageIndex('unknown_stage'), -1)
})

test('reports completed production stages as a stable overall percentage', () => {
  assert.deepEqual(getPipelineProgress('episode_script', 'running'), {
    currentIndex: 4,
    completedStages: 4,
    remainingStages: 8,
    totalStages: pipelineStages.length,
    percentage: 33,
    remainingPercentage: 67,
    currentStageLabel: '单集剧本',
    nextPendingStageLabel: '单集剧本',
  })
  assert.equal(getPipelineProgress('published', 'completed').percentage, 100)
  assert.equal(getPipelineProgress('unknown_stage', 'running').percentage, 0)
  assert.equal(getPipelineProgress('storyboard_approved', 'stage_2_completed').completedStages, 6)
  assert.equal(getPipelineProgress('stage_4_completed', 'stage_4_completed').completedStages, 10)
})

test('uses readable labels for known, completed and unknown stages', () => {
  assert.equal(getPipelineStageLabel('storyboard_image_review'), '分镜图片审核')
  assert.equal(getPipelineStageLabel('edit_compose'), '剪辑合成')
  assert.equal(getPipelineStageLabel('review'), '故事圣经审核')
  assert.equal(getPipelineStageLabel('publishing'), '发布中')
  assert.equal(getPipelineStageLabel('published', 'completed'), '生产完成')
  assert.equal(getPipelineStageLabel('custom_stage'), '未识别阶段')
})

test('reports remaining stages and exact current-stage unit progress', () => {
  const pipeline = getPipelineProgress('chunk_analysis', 'running')
  assert.equal(pipeline.completedStages, 1)
  assert.equal(pipeline.remainingStages, 11)
  assert.equal(pipeline.nextPendingStageLabel, '文本拆解')

  assert.deepEqual(getStageUnitProgress({
    current_stage: 'chunk_analysis',
    status: 'running',
    chunk_count: 24,
    completed_chunk_count: 18,
  }), {
    completed: 18,
    total: 24,
    remaining: 6,
    percentage: 75,
    unit: '个文本分块',
  })
})
