import assert from 'node:assert/strict'
import test from 'node:test'
import { getDisplayValueLabel, getStatusLabel } from '../src/services/displayLabels.js'

test('translates production statuses to Chinese and hides unknown raw codes', () => {
  assert.equal(getStatusLabel('running'), '生产中')
  assert.equal(getStatusLabel('validating'), '校验中')
  assert.equal(getStatusLabel('superseded'), '已被替代')
  assert.equal(getStatusLabel('custom_status'), '未知状态')
})

test('labels a reviewed video batch as ready to continue', () => {
  assert.equal(getStatusLabel('waiting_shot_video_review'), '镜头视频分批待继续')
  assert.equal(getStatusLabel('ready_to_continue'), '待继续')
})

test('translates workflow actions and technical display values', () => {
  assert.equal(getDisplayValueLabel('resume'), '继续流程')
  assert.equal(getDisplayValueLabel('ir_extraction'), '叙事信息提取')
  assert.equal(getDisplayValueLabel('incremental'), '增量版本')
})
