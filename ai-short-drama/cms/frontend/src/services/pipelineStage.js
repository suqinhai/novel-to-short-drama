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
  ['adaptation_planning', 'episode_planning'],
  ['created', 'novel_import'],
  ['novel_import', 'novel_import'],
  ['chunk_analysis', 'chunk_analysis'],
  ['story_bible', 'story_bible'],
  ['season_outline', 'episode_planning'],
  ['waiting_next_episode', 'episode_planning'],
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

const exactStageLabels = {
  adaptation_planning: '等待编译并采用改编计划',
  waiting_next_episode: '等待启动下一集',
  created: '项目已创建',
  novel_import: '小说导入',
  chunk_analysis: '文本拆解',
  story_bible: '故事圣经生成',
  review: '故事圣经审核',
  story_bible_approved: '故事圣经已通过',
  season_outline_review: '分集策划审核',
  season_outline_approved: '分集策划已通过',
  episode_script_review: '单集剧本审核',
  episode_script_approved: '单集剧本已通过',
  storyboard_review: '分镜设计审核',
  storyboard_approved: '分镜设计已通过',
  stage_2_completed: '剧本与分镜阶段已完成',
  visual_assets: '视觉资产生成',
  visual_asset_review: '视觉资产审核',
  visual_assets_locked: '视觉资产已锁定',
  storyboard_images: '分镜图片生成',
  storyboard_image_review: '分镜图片审核',
  storyboard_images_approved: '分镜图片已通过',
  stage_3_completed: '图片阶段已完成',
  stage_3_failed: '图片阶段异常',
  image_to_video: '镜头视频生成',
  video_tasks_submitted: '视频任务已提交',
  video_processing: '镜头视频处理中',
  shot_videos_generated: '镜头视频已生成',
  shot_video_review: '镜头视频审核',
  shot_videos_approved: '镜头视频已通过',
  voice_audio: '语音音频生成',
  voice_profiles_created: '角色音色已创建',
  voice_profile_review: '角色音色审核',
  voice_profiles_locked: '角色音色已锁定',
  tts_processing: '语音合成处理中',
  dialogue_audio_generated: '对白音频已生成',
  audio_processing: '音频处理中',
  audio_review: '音频审核',
  audio_ready: '音频已就绪',
  audio_plan_completed: '音频阶段已完成',
  stage_4_completed: '视频与音频阶段已完成',
  stage_4_failed: '视频与音频阶段异常',
  preparing_timeline: '正在准备剪辑时间线',
  edit_timeline_ready: '剪辑时间线已就绪',
  rendering: '成片渲染中',
  preview_rendered: '预览片已生成',
  final_rendered: '最终成片已生成',
  waiting_qc: '等待质量检查',
  qc_completed: '质量检查已完成',
  waiting_final_review: '等待最终审核',
  final_review_approved: '最终审核已通过',
  preparing_publication: '正在准备发布',
  waiting_publication_metadata_review: '等待发布信息审核',
  publication_metadata_approved: '发布信息已通过',
  publication_submitted: '发布任务已提交',
  publishing: '发布中',
  published: '已发布',
  stage_5_completed: '质检发布阶段已完成',
  stage_5_failed: '质检发布阶段异常',
}

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
  const normalized = String(currentStage || '').toLowerCase().trim()
  if (exactStageLabels[normalized]) return exactStageLabels[normalized]
  if (index >= 0) return pipelineStages[index][1]
  return normalized ? '未识别阶段' : '尚未开始'
}

export function getPipelineProgress(currentStage, projectStatus = '') {
  const currentIndex = getPipelineStageIndex(currentStage, projectStatus)
  const totalStages = pipelineStages.length
  const normalizedStage = String(currentStage || '').toLowerCase().trim()
  const checkpointOffset = completedStageCheckpoints.has(normalizedStage) ? 1 : 0
  const completedStages = currentIndex < 0 ? 0 : Math.min(currentIndex + checkpointOffset, totalStages)
  const remainingStages = totalStages - completedStages
  const nextPendingStage = pipelineStages[completedStages]

  return {
    currentIndex,
    completedStages,
    remainingStages,
    totalStages,
    percentage: Math.round((completedStages / totalStages) * 100),
    remainingPercentage: Math.round((remainingStages / totalStages) * 100),
    currentStageLabel: getPipelineStageLabel(currentStage, projectStatus),
    nextPendingStageLabel: nextPendingStage?.[1] || '全部完成',
  }
}

export function getStageUnitProgress(project = {}) {
  const stageIndex = getPipelineStageIndex(project.current_stage, project.status)
  let completed = 0
  let total = 0
  let unit = ''

  if (stageIndex === 1) {
    completed = Number(project.completed_chunk_count ?? project.counts?.completed_chunks ?? 0)
    total = Number(project.chunk_count ?? project.counts?.chunks ?? 0)
    unit = '个文本分块'
  } else if (stageIndex === 3) {
    completed = Number(project.generated_episode_count ?? project.counts?.episodes ?? 0)
    total = Number(project.target_episode_count ?? 0)
    unit = '集'
  } else if (stageIndex === 4) {
    completed = Number(project.scripts?.length ?? 0)
    total = Number(project.target_episode_count ?? 0)
    unit = '集剧本'
  } else if (stageIndex === 5) {
    completed = Number(project.storyboards?.length ?? 0)
    total = Number(project.target_episode_count ?? 0)
    unit = '集分镜'
  } else if (stageIndex === 7) {
    completed = Number(project.counts?.generated_images ?? 0)
    total = Number(project.counts?.shots ?? 0)
    unit = '张分镜图片'
  } else if (stageIndex === 8) {
    completed = Number(project.counts?.generated_videos ?? 0)
    total = Number(project.counts?.shots ?? 0)
    unit = '个镜头视频'
  }

  if (total <= 0) return null
  const safeCompleted = Math.min(Math.max(completed, 0), total)
  return {
    completed: safeCompleted,
    total,
    remaining: total - safeCompleted,
    percentage: Math.round((safeCompleted / total) * 100),
    unit,
  }
}
