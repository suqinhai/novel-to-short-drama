export function isVisualAssetReview(item) {
  return item?.stage === 'visual_asset' && item?.entity_type === 'generated_asset'
}

export function getVisualRegenerationAction(item) {
  if (!isVisualAssetReview(item)) return null
  if (item.review_status === 'pending') {
    return { operation: 'reject_regenerate', mode: 'replace', label: '退回重做' }
  }
  if (item.review_status === 'rejected') {
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
