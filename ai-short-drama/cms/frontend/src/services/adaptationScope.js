const modes = new Set(['chapters_only', 'arcs_only', 'union'])

export function selectCurrentFullIR(revisions = []) {
  return revisions
    .filter((item) => item.status === 'published' && item.revision_scope === 'full')
    .sort((left, right) => right.revision_number - left.revision_number)[0] || null
}

export function isScopeComplete(mode, chapterIds = [], storyArcRevisionIds = []) {
  if (mode === 'chapters_only') return chapterIds.length > 0
  if (mode === 'arcs_only') return storyArcRevisionIds.length > 0
  if (mode === 'union') return chapterIds.length > 0 || storyArcRevisionIds.length > 0
  return false
}

export function buildAdaptationScope(mode, chapterIds = [], storyArcRevisionIds = []) {
  if (!modes.has(mode)) throw new Error(`Unsupported adaptation scope mode: ${mode}`)
  return {
    mode,
    chapter_ids: mode === 'arcs_only' ? [] : [...chapterIds],
    story_arc_revision_ids: mode === 'chapters_only' ? [] : [...storyArcRevisionIds],
  }
}
