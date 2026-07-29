export const REVIEW_PAGE_SIZE = 20

export const reviewStatusTabs = [
  { value: 'pending', label: '待我审核' },
  { value: 'processed', label: '已处理' },
  { value: '', label: '全部记录' },
]

export function groupReviewItems(items = []) {
  const groups = new Map()
  for (const item of items) {
    const key = `${item.project_id}::${item.stage}`
    if (!groups.has(key)) {
      groups.set(key, {
        key,
        projectId: item.project_id,
        projectName: item.novel_name,
        stage: item.stage,
        items: [],
        pendingCount: 0,
      })
    }
    const group = groups.get(key)
    group.items.push(item)
    if (item.review_status === 'pending') group.pendingCount += 1
  }
  return [...groups.values()]
}

export function getReviewTabCount(tab, summary = {}) {
  if (tab === 'pending') return summary.pending || 0
  if (tab === 'processed') return (summary.approved || 0) + (summary.rejected || 0)
  return summary.total || 0
}

export function getReviewTaskTitle(item, stageLabel = '审核任务') {
  const entityLabels = {
    dialogue_audio: '对白音频',
    voice_profile: '角色声音',
    shot_video: '镜头视频',
    storyboard_frame: '分镜图片',
    generated_asset: '视觉资产',
    episode_script: '单集剧本',
    storyboard: '分镜设计',
  }
  const entityLabel = entityLabels[item?.entity_type] || stageLabel
  return `${entityLabel}任务`
}

export function getAdjacentReview(items = [], reviewId, direction) {
  const index = items.findIndex((item) => item.review_id === reviewId)
  if (index < 0) return null
  return items[index + direction] || null
}
