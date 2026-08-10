import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const read = (file) => fs.readFileSync(path.join(root, file), 'utf8')

test('prompt lab exposes all creative stages and separates approval from production promotion', () => {
  const view = read('src/views/PromptLabView.vue')
  for (const category of [
    'novel_analysis', 'narrative_ir', 'episode_planning', 'script', 'storyboard',
    'image', 'video', 'tts', 'qc',
  ]) assert.match(view, new RegExp(`'${category}'`))
  assert.match(view, /最终输入预览与 Token 估算/)
  assert.match(view, /选择冻结测试集/)
  assert.match(view, /人工盲评/)
  assert.match(view, /自动指标/)
  assert.match(view, /评分区仅使用匿名接口/)
  assert.doesNotMatch(view, /item\.blind_label \}\} · \{\{ item\.model/)
  assert.match(view, /approvePromptVersion/)
  assert.match(view, /promotePromptVersion/)
})

test('professional export UI covers every delivery family and exact version selection', () => {
  const view = read('src/views/ProfessionalExportView.vue')
  for (const format of [
    'script_docx', 'script_fountain', 'episode_outline', 'shot_list', 'contact_sheet',
    'subtitle_srt', 'subtitle_ass', 'timeline_edl', 'timeline_xml', 'audio_stems',
    'prompt_package', 'production_bibles', 'traceability_report',
  ]) assert.match(view, new RegExp(`'${format}'`))
  for (const label of ['作品', '项目', '单集版本', '剧本版本', '分镜版本', '剪辑时间线版本']) {
    assert.match(view, new RegExp(label))
  }
  assert.match(view, /禁止 current \/ draft 混用/)
})

test('candidate and local-edit targets are selected from hierarchy; technical IDs stay advanced', () => {
  const candidate = read('src/views/CandidateWorkbenchView.vue')
  const localEdit = read('src/views/LocalEditingWorkbenchView.vue')
  for (const view of [candidate, localEdit]) {
    for (const label of ['作品', '项目', '场', '镜']) assert.match(view, new RegExp(label))
    assert.match(view, /<summary>高级信息<\/summary>/)
    assert.doesNotMatch(view, /v-model="form\.entity_id"/)
    assert.doesNotMatch(view, /placeholder="[^"\n]*(技术 ID|实体 ID|target ID)/i)
  }
})
