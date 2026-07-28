import { getDisplayValueLabel } from './displayLabels.js'

export function getIRRevisionDisplayStatus(revision) {
  const operationStatus = String(revision?.operation_status || '').toLowerCase()
  if (operationStatus && operationStatus !== 'completed') return operationStatus
  return revision?.status || operationStatus
}

export function getIRScopeSummary(revision) {
  const chapterCount = Array.isArray(revision?.changed_chapter_ids)
    ? revision.changed_chapter_ids.length
    : 0
  if (revision?.revision_scope === 'incremental') {
    return chapterCount ? `增量修订 · ${chapterCount} 个变更章节` : '增量修订'
  }
  return chapterCount ? `本次提取 ${chapterCount} 章` : '完整提取'
}

export function getIRProgressSummary(revision) {
  const status = String(revision?.operation_status || '').toLowerCase()
  const retryCount = Number(revision?.retry_count || 0)
  const stage = getDisplayValueLabel(revision?.checkpoint_stage)
  if (status === 'failed' && revision?.operation_error_message) {
    return revision.operation_error_message
  }
  if (!['pending', 'running', 'validating'].includes(status)) return ''
  const retry = retryCount > 0 ? `第 ${retryCount} 次重试 · ` : ''
  const completed = Number(revision?.completed_items)
  const total = Number(revision?.total_items)
  if (Number.isInteger(completed) && completed >= 0 && Number.isInteger(total) && total > 0) {
    return `${retry}已完成 ${Math.min(completed, total)} / ${total}`
  }
  return `${retry}${stage}`
}
