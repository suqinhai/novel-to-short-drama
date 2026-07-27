export const pipelineStages = [
  ['novel_import', '小说导入'],
  ['chunk_analysis', '文本拆解'],
  ['story_bible', '故事圣经'],
  ['episode_planning', '分集策划'],
  ['episode_script', '单集剧本'],
  ['storyboard', '分镜设计'],
  ['visual_assets', '视觉资产'],
  ['storyboard_images', '分镜图片'],
  ['shot_video', '镜头视频'],
  ['voice_audio', '语音音频'],
  ['edit_compose', '剪辑合成'],
  ['qc_review_publish', '质检发布'],
]

const stageAliases = [
  ['created', 'novel_import'],
  ['novel_import', 'novel_import'],
  ['chunk_analysis', 'chunk_analysis'],
  ['story_bible', 'story_bible'],
  ['season_outline', 'episode_planning'],
  ['episode_planning', 'episode_planning'],
  ['episode_script', 'episode_script'],
  ['storyboard_images', 'storyboard_images'],
  ['storyboard_image', 'storyboard_images'],
  ['storyboard', 'storyboard'],
  ['visual_asset', 'visual_assets'],
  ['stage_2_completed', 'storyboard'],
  ['stage_3', 'storyboard_images'],
  ['image_to_video', 'shot_video'],
  ['video_', 'shot_video'],
  ['shot_video', 'shot_video'],
  ['voice_', 'voice_audio'],
  ['tts_', 'voice_audio'],
  ['dialogue_audio', 'voice_audio'],
  ['audio_', 'voice_audio'],
  ['stage_4', 'voice_audio'],
  ['edit_compose', 'edit_compose'],
  ['preparing_timeline', 'edit_compose'],
  ['waiting_media', 'edit_compose'],
  ['edit_timeline', 'edit_compose'],
  ['rendering', 'edit_compose'],
  ['preview_rendered', 'edit_compose'],
  ['final_rendered', 'edit_compose'],
  ['waiting_qc', 'qc_review_publish'],
  ['qc_', 'qc_review_publish'],
  ['final_review', 'qc_review_publish'],
  ['waiting_final_review', 'qc_review_publish'],
  ['publication', 'qc_review_publish'],
  ['publishing', 'qc_review_publish'],
  ['published', 'qc_review_publish'],
  ['stage_5', 'qc_review_publish'],
  ['review', 'story_bible'],
]

const completedStageCheckpoints = new Set([
  'story_bible_approved',
  'season_outline_approved',
  'episode_script_approved',
  'storyboard_approved',
  'stage_2_completed',
  'visual_assets_locked',
  'storyboard_images_approved',
  'stage_3_completed',
  'shot_videos_approved',
  'audio_ready',
  'audio_plan_completed',
  'stage_4_completed',
  'final_rendered',
])

export function getPipelineStageIndex(currentStage, projectStatus = '') {
  if (projectStatus === 'completed' || currentStage === 'published') return pipelineStages.length
  const normalized = String(currentStage || '').toLowerCase().trim()
  const match = stageAliases.find(([alias]) => normalized.includes(alias))
  if (!match) return -1
  return pipelineStages.findIndex(([key]) => key === match[1])
}

export function getPipelineStageLabel(currentStage, projectStatus = '') {
  const index = getPipelineStageIndex(currentStage, projectStatus)
  if (index === pipelineStages.length) return '生产完成'
  if (index >= 0) return pipelineStages[index][1]
  return String(currentStage || '').replaceAll('_', ' ').trim() || '尚未开始'
}

export function getPipelineProgress(currentStage, projectStatus = '') {
  const currentIndex = getPipelineStageIndex(currentStage, projectStatus)
  const totalStages = pipelineStages.length
  const normalizedStage = String(currentStage || '').toLowerCase().trim()
  const checkpointOffset = completedStageCheckpoints.has(normalizedStage) ? 1 : 0
  const completedStages = currentIndex < 0 ? 0 : Math.min(currentIndex + checkpointOffset, totalStages)

  return {
    currentIndex,
    completedStages,
    totalStages,
    percentage: Math.round((completedStages / totalStages) * 100),
    currentStageLabel: getPipelineStageLabel(currentStage, projectStatus),
  }
}
