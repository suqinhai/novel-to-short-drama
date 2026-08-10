'use strict'

const fs = require('fs')
const path = require('path')
const assert = require('assert/strict')
const root = path.resolve(__dirname, '..')
const read = (file) => fs.readFileSync(path.join(root, file), 'utf8')

const migration = read('database/27-lightweight-nle.sql')
const store = read('cms/backend/internal/store/nle.go')
const handler = read('cms/backend/internal/httpapi/postproduction.go')
const component = read('cms/frontend/src/components/TimelineNLE.vue')
const service = read('cms/frontend/src/services/timelineNle.js')

for (const marker of [
  'edit_timelines_current_requires_approval', 'uq_nle_active_render_timeline',
  'trg_render_job_promote_timeline', "NEW.status='succeeded'", "NEW.status IN ('failed','timeout','cancelled')",
  "approval_state='render_failed'", "approval_state='approved'", 'parent_timeline_item_id',
]) assert(migration.includes(marker), `migration missing ${marker}`)

for (const marker of [
  'CreateNLEItemDraft', 'RestoreNLETimelineDraft', 'ConfirmNLETimelineRender',
  'LIMIT $4 OFFSET $5', "item.track_type='video'", 'nle-confirm:',
]) assert(store.includes(marker), `NLE store missing ${marker}`)

for (const marker of ['nle-timeline', 'restore-draft', '/render', 'createNLEItemDraft']) {
  assert(handler.includes(marker), `NLE HTTP API missing ${marker}`)
}

for (const marker of [
  '可播放多轨时间线', '确认并重编', 'J-cut', 'L-cut', '代理媒体生成中',
  '波形生成中', '限制在字幕安全区', 'visibleTimelineWindow',
]) assert(component.includes(marker), `NLE UI missing ${marker}`)

assert.deepEqual([...service.matchAll(/type: '([^']+)'/g)].slice(0, 7).map(match => match[1]),
  ['video', 'dialogue', 'narration', 'bgm', 'ambience', 'sound_effect', 'subtitle'])
console.log('PASS phase 27 lightweight NLE static acceptance')
