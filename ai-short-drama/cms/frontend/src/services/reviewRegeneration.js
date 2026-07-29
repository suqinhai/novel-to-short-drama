export function isVisualAssetReview(item) {
  return item?.stage === 'visual_asset' && item?.entity_type === 'generated_asset'
}

export function isRegeneratedVisualReview(item) {
  return isVisualAssetReview(item)
    && item?.review_status === 'rejected'
    && Boolean(item?.regenerated_by_review_id)
}

export function getRegenerationSourceLabel(item) {
  if (!isVisualAssetReview(item) || !item?.regenerated_from_asset_id) return ''
  const version = Number(item.generation_version)
  const versionLabel = Number.isInteger(version) && version > 0 ? ` · 版本 v${version}` : ''
  return `由 ${item.regenerated_from_asset_id} 重新生成${versionLabel}`
}

export function getRegeneratedSuccessor(item, loadedItems = []) {
  if (!isRegeneratedVisualReview(item)) return null
  const loadedSuccessor = loadedItems.find(candidate => candidate.review_id === item.regenerated_by_review_id)
  if (loadedSuccessor) return loadedSuccessor
  return {
    ...item,
    review_id: item.regenerated_by_review_id,
    entity_id: item.regenerated_by_entity_id,
    review_status: 'pending',
    reviewed_at: null,
    regenerated_from_asset_id: item.entity_id,
    generation_version: item.regenerated_by_generation_version,
    regenerated_by_review_id: null,
    regenerated_by_entity_id: null,
    regenerated_by_generation_version: null,
  }
}

export function getVisualRegenerationAction(item) {
  if (!isVisualAssetReview(item)) return null
  if (item.review_status === 'pending') {
    return { operation: 'reject_regenerate', mode: 'replace', label: '退回重做' }
  }
  if (item.review_status === 'rejected') {
    if (isRegeneratedVisualReview(item)) return null
    return { operation: 'regenerate', mode: 'replace', label: '按意见重新生成' }
  }
  if (item.review_status === 'approved') {
    return { operation: 'regenerate', mode: 'variant', label: '生成新变体' }
  }
  return null
}

export function regenerationNeedsPrompt(mode) {
  return mode === 'variant'
}
