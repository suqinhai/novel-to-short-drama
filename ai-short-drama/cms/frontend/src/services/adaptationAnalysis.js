export const severityLabel = (severity) => ({
  critical: '阻断', major: '严重', warning: '中等', info: '已复核',
  high: '严重', medium: '中等', low: '提示',
}[severity] || severity || '提示')

export function curvePoints(episodes = [], field, width = 600, height = 150) {
  if (!episodes.length) return ''
  const step = episodes.length === 1 ? 0 : width / (episodes.length - 1)
  return episodes.map((episode, index) => {
    const value = Math.max(0, Math.min(1, Number(episode[field]) || 0))
    return `${Math.round(index * step)},${Math.round(height - value * height)}`
  }).join(' ')
}

export function normalizeBeatEdits(beats = []) {
  return beats.map((beat) => ({
    beat_key: beat.beat_key,
    episode_number: Number(beat.episode_number),
    beat_ordinal: Number(beat.beat_ordinal),
    estimated_duration_seconds: Number(beat.estimated_duration_seconds),
  }))
}

export function buildSpecFromDiagnostic(diagnostic, pacing, current = {}) {
  const audience = diagnostic?.target_audience || {}
  const chapterIds = [...new Set((diagnostic?.nodes || []).map((node) => node.chapter_id).filter(Boolean))]
  return {
    schema_version: 'adaptation-spec.v1',
    source_version_id: diagnostic?.source_version_id,
    ir_revision_id: diagnostic?.ir_revision_id,
    scope: { mode: 'chapters_only', chapter_ids: chapterIds, story_arc_revision_ids: [] },
    platform: current.platform || '抖音',
    audience_profile: {
      description: current.audience || audience.primary || audience.segment || '短剧观众',
      tags: current.audience_tags || diagnostic?.emotional_value || [],
    },
    target_episode_count: current.target_episode_count || Math.max(1, pacing?.episodes?.length || 1),
    episode_duration_seconds: current.episode_duration_seconds ||
      Math.max(1, Math.round((pacing?.total_duration_seconds || 120) / Math.max(1, pacing?.episodes?.length || 1))),
    rules: [
      {
        rule_type: 'must_preserve', enforcement: 'hard', target_type: 'free_text',
        target_id: null, priority: 80,
        parameters: { instruction: (diagnostic?.core_selling_points || []).join('；') },
        rationale: `来自诊断报告 ${diagnostic?.diagnostic_report_id || ''}，经用户确认`,
      },
    ],
  }
}
