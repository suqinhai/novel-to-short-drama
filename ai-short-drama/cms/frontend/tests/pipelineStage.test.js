import assert from 'node:assert/strict'
import test from 'node:test'
import { getPipelineStageIndex, pipelineStages } from '../src/services/pipelineStage.js'

test('maps stage 4 video and audio states to their visible pipeline steps', () => {
  assert.equal(getPipelineStageIndex('video_processing'), 8)
  assert.equal(getPipelineStageIndex('shot_videos_approved'), 8)
  assert.equal(getPipelineStageIndex('voice_profile_review'), 9)
  assert.equal(getPipelineStageIndex('audio_processing'), 9)
  assert.equal(getPipelineStageIndex('stage_4_completed'), 9)
})

test('maps review, render and publish states across the full pipeline', () => {
  assert.equal(getPipelineStageIndex('season_outline_review'), 3)
  assert.equal(getPipelineStageIndex('storyboard_review'), 5)
  assert.equal(getPipelineStageIndex('storyboard_image_review'), 7)
  assert.equal(getPipelineStageIndex('preview_rendered'), 10)
  assert.equal(getPipelineStageIndex('waiting_final_review'), 11)
  assert.equal(getPipelineStageIndex('published'), pipelineStages.length)
  assert.equal(getPipelineStageIndex('unknown_stage'), -1)
})
