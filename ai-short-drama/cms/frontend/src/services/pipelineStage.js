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
  ['preparing_timeline', 'edit_compose'],
  ['edit_timeline', 'edit_compose'],
  ['rendering', 'edit_compose'],
  ['preview_rendered', 'edit_compose'],
  ['final_rendered', 'edit_compose'],
  ['waiting_qc', 'qc_review_publish'],
  ['qc_', 'qc_review_publish'],
  ['final_review', 'qc_review_publish'],
  ['waiting_final_review', 'qc_review_publish'],
  ['publication', 'qc_review_publish'],
  ['published', 'qc_review_publish'],
  ['stage_5', 'qc_review_publish'],
]

export function getPipelineStageIndex(currentStage, projectStatus = '') {
  if (projectStatus === 'completed' || currentStage === 'published') return pipelineStages.length
  const normalized = String(currentStage || '').toLowerCase().trim()
  const match = stageAliases.find(([alias]) => normalized.includes(alias))
  if (!match) return -1
  return pipelineStages.findIndex(([key]) => key === match[1])
}
