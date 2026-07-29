const busyStatuses = new Set(['pending', 'submitting', 'generating', 'processing', 'rendering'])
const successfulStatuses = new Set(['succeeded', 'ready', 'completed', 'approved'])
const failedStatuses = new Set(['failed', 'timeout', 'cancelled'])

export function isMediaAssetRecoverable(item) {
  return failedStatuses.has(item?.status)
    || (!item?.media_url && !busyStatuses.has(item?.status))
}

export function hasMediaAssetSuccessor(item) {
  return Boolean(item?.successor_asset_id)
}

export function canRecoverMediaAsset(item) {
  return isMediaAssetRecoverable(item) && !hasMediaAssetSuccessor(item)
}

export function getMediaAssetSourceLabel(item) {
  if (!item?.predecessor_asset_id) return ''
  const version = Number(item.generation_version)
  const versionLabel = Number.isInteger(version) && version > 0 ? ` · 版本 v${version}` : ''
  const operation = item.provider === 'manual_upload' ? '上传替换' : '重新生成'
  return `由 ${item.predecessor_asset_id} ${operation}${versionLabel}`
}

export function getMediaAssetSuccessorState(item) {
  if (!hasMediaAssetSuccessor(item)) return null
  const version = item.successor_generation_version || '?'
  if (successfulStatuses.has(item.successor_status)) {
    const uploaded = item.successor_provider === 'manual_upload'
    return {
      badgeStatus: uploaded ? 'replaced' : 'regenerated',
      label: uploaded ? '已上传替换' : '已重新生成',
      detail: `后继版本 v${version} 已可用`,
    }
  }
  if (failedStatuses.has(item.successor_status)) {
    return {
      badgeStatus: 'superseded',
      label: '已有后继版本',
      detail: `后继版本 v${version} 生成异常，请在该版本继续处理`,
    }
  }
  return {
    badgeStatus: 'regenerating',
    label: '重新生成中',
    detail: `后继版本 v${version} 正在处理`,
  }
}
