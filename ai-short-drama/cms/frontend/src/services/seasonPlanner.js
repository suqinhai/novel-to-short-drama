const modes = new Set(['preserve', 'merge', 'split', 'omit', 'original', 'transform'])

const clone = (value) => JSON.parse(JSON.stringify(value))
const unique = (items) => [...new Set((items || []).filter(Boolean))]
const cardId = (prefix = 'card') => `${prefix}_${crypto.randomUUID()}`

function assignmentCard(assignment) {
  return {
    card_id: `card_${assignment.event_revision_id}`,
    presentation_mode: assignment.usage_mode === 'reference' ? 'preserve' : assignment.usage_mode,
    source_event_ids: [assignment.event_revision_id],
    summary: assignment.summary || assignment.event_revision_id,
    rationale: '',
    importance: Number(assignment.importance || 0),
    source_chapter_ids: assignment.chapter_id ? [assignment.chapter_id] : [],
    chapter_title: assignment.chapter_title || assignment.chapter_id || '',
    participants: assignment.participants || [],
    character_states: assignment.character_states || [],
    foreshadowing: assignment.foreshadowing || [],
    merge_group_id: assignment.merge_group_id || '',
  }
}

export function buildSeasonDraft(plan = {}) {
  const snapshot = plan.workbench_snapshot
  if (snapshot?.schema_version === 'season-plan-draft.v1') return clone(snapshot)
  const episodes = (plan.episodes || []).map((episode) => {
    const cards = []
    const assignments = episode.event_assignments || []
    const merged = new Map()
    for (const assignment of assignments) {
      if (assignment.usage_mode === 'merge' && assignment.merge_group_id) {
        const key = assignment.merge_group_id
        if (!merged.has(key)) merged.set(key, [])
        merged.get(key).push(assignment)
      } else cards.push(assignmentCard(assignment))
    }
    for (const [mergeGroupId, items] of merged) {
      const audit = (episode.merged_content || []).find((item) => item.merge_group_id === mergeGroupId)
      cards.push({
        card_id: `card_${mergeGroupId}`, presentation_mode: 'merge', merge_group_id: mergeGroupId,
        source_event_ids: items.map((item) => item.event_revision_id),
        summary: audit?.description || items.map((item) => item.summary).join(' / '),
        rationale: audit?.description || '', importance: Math.max(...items.map((item) => Number(item.importance || 0))),
        source_chapter_ids: unique(items.map((item) => item.chapter_id)),
        chapter_title: unique(items.map((item) => item.chapter_title || item.chapter_id)).join(' / '),
        participants: items.flatMap((item) => item.participants || []),
        character_states: items.flatMap((item) => item.character_states || []),
        foreshadowing: items.flatMap((item) => item.foreshadowing || []),
      })
    }
    for (const addition of episode.added_adaptation_content || []) cards.push({
      card_id: addition.content_id || cardId('original'), presentation_mode: 'original', source_event_ids: [],
      summary: addition.description, rationale: addition.reason, importance: 0.5, source_chapter_ids: [],
      participants: [], character_states: [], foreshadowing: [],
    })
    return {
      episode_number: episode.episode_number,
      title: episode.title || `第 ${episode.episode_number} 集`, logline: episode.logline || '',
      three_second_opening: episode.three_second_opening || episode.opening_hook || '',
      first_thirty_seconds_goal: episode.first_thirty_seconds_goal || episode.logline || '',
      core_conflict: episode.core_conflict || episode.logline || '', climax: episode.climax || episode.ending_hook || '',
      ending_hook: episode.ending_hook || '', emotion_curve: episode.emotion_curve?.length ? episode.emotion_curve : [{ position: 1, emotion: 0.5 }],
      information_reveal_amount: Number(episode.information_reveal_amount ?? 0.5),
      estimated_duration_seconds: Number(episode.estimated_duration_seconds || 1),
      continuity_in: episode.continuity_in || [], continuity_out: episode.continuity_out || [], events: cards,
    }
  })
  return {
    schema_version: 'season-plan-draft.v1', plan_name: plan.plan_name || `整季方案 v${plan.version_number || 1}`,
    strategy_label: plan.strategy_label || 'narrative_constraint_dp', episodes, omitted_events: [],
    creative_suggestions: plan.creative_suggestions || [],
  }
}

export function moveEventCard(draft, cardIdValue, toEpisodeIndex, toIndex) {
  const next = clone(draft)
  let moved
  for (const episode of next.episodes) {
    const index = episode.events.findIndex((card) => card.card_id === cardIdValue)
    if (index >= 0) moved = episode.events.splice(index, 1)[0]
  }
  const omittedIndex = next.omitted_events.findIndex((card) => card.card_id === cardIdValue)
  if (!moved && omittedIndex >= 0) moved = next.omitted_events.splice(omittedIndex, 1)[0]
  if (!moved || !next.episodes[toEpisodeIndex]) return next
  moved.presentation_mode = moved.presentation_mode === 'omit' ? 'preserve' : moved.presentation_mode
  next.episodes[toEpisodeIndex].events.splice(Math.max(0, Math.min(toIndex, next.episodes[toEpisodeIndex].events.length)), 0, moved)
  return renumberEpisodes(next)
}

export function applyEventOperation(draft, operation, selection = [], options = {}) {
  if (!['merge', 'split', 'omit', 'original', 'transform'].includes(operation)) throw new Error(`Unsupported operation: ${operation}`)
  const next = clone(draft)
  const selected = new Set(selection)
  const locations = []
  next.episodes.forEach((episode, episodeIndex) => episode.events.forEach((card, index) => {
    if (selected.has(card.card_id)) locations.push({ episode, episodeIndex, index, card })
  }))
  if (operation === 'original') {
    const episode = next.episodes[Number(options.episode_index || 0)]
    if (!episode || !String(options.summary || '').trim() || !String(options.rationale || '').trim()) return next
    episode.events.push({ card_id: cardId('original'), presentation_mode: 'original', source_event_ids: [],
      summary: String(options.summary).trim(), rationale: String(options.rationale).trim(), importance: Number(options.importance || 0.5),
      source_chapter_ids: [], participants: [], character_states: [], foreshadowing: [] })
    return next
  }
  if (!locations.length) return next
  if (operation === 'merge') {
    if (locations.length < 2 || locations.some((item) => item.episodeIndex !== locations[0].episodeIndex)) return next
    const first = locations[0]
    const sourceCards = locations.map((item) => item.card)
    first.episode.events = first.episode.events.filter((card) => !selected.has(card.card_id))
    first.episode.events.splice(Math.min(...locations.map((item) => item.index)), 0, {
      card_id: cardId('merge'), presentation_mode: 'merge', merge_group_id: cardId('merge_group'),
      source_event_ids: unique(sourceCards.flatMap((card) => card.source_event_ids)),
      summary: String(options.summary || sourceCards.map((card) => card.summary).join(' / ')),
      rationale: String(options.rationale || '整合为一个短剧节拍'),
      importance: Math.max(...sourceCards.map((card) => Number(card.importance || 0))),
      source_chapter_ids: unique(sourceCards.flatMap((card) => card.source_chapter_ids)),
      participants: sourceCards.flatMap((card) => card.participants || []),
      character_states: sourceCards.flatMap((card) => card.character_states || []),
      foreshadowing: sourceCards.flatMap((card) => card.foreshadowing || []),
    })
  } else if (operation === 'split') {
    const { episode, index, card } = locations[0]
    episode.events.splice(index, 1,
      { ...card, card_id: cardId('split'), presentation_mode: 'split', split_label: 'A', summary: String(options.first_summary || `${card.summary}（上）`) },
      { ...card, card_id: cardId('split'), presentation_mode: 'split', split_label: 'B', summary: String(options.second_summary || `${card.summary}（下）`) })
  } else if (operation === 'omit') {
    for (const item of locations.sort((a, b) => b.index - a.index)) {
      item.episode.events.splice(item.index, 1)
      next.omitted_events.push({ ...item.card, presentation_mode: 'omit', rationale: String(options.rationale || item.card.rationale || '节奏压缩') })
    }
  } else if (operation === 'transform') {
    for (const item of locations) {
      item.card.presentation_mode = 'transform'
      item.card.rationale = String(options.rationale || item.card.rationale || '为适配短剧媒介进行变形')
    }
  }
  return renumberEpisodes(next)
}

export function renumberEpisodes(draft) {
  draft.episodes.forEach((episode, index) => { episode.episode_number = index + 1 })
  return draft
}

function matchesRule(rule, card) {
  if (rule.target_type === 'free_text') return true
  if (rule.target_type === 'event') return card.source_event_ids.includes(rule.target_id)
  if (rule.target_type === 'chapter') return card.source_chapter_ids.includes(rule.target_id)
  if (rule.target_type === 'entity') return (card.participants || []).some((item) => (item.entity_revision_id || item) === rule.target_id)
  return false
}

export function validateDraftLocally(draft, rules = [], durationTarget = Infinity) {
  const diagnostics = []
  const add = (severity, code, message, card, rule) => diagnostics.push({ severity, code, message,
    entity_type: rule ? 'adaptation_rule' : card ? 'event_card' : 'adaptation_plan',
    entity_id: rule?.adaptation_rule_id || card?.card_id || null,
    details: { rule_enforcement: rule?.enforcement, source_event_ids: card?.source_event_ids || [] } })
  for (const episode of draft.episodes || []) {
    if (!episode.three_second_opening || !episode.first_thirty_seconds_goal || !episode.core_conflict || !episode.climax || !episode.ending_hook || !(episode.emotion_curve || []).length) add('blocking', 'EPISODE_STRUCTURE_INCOMPLETE', '本集结构字段尚未填写完整。')
    if (Number(episode.estimated_duration_seconds) > durationTarget) add('blocking', 'EPISODE_DURATION_EXCEEDED', '预计时长超过目标。')
    for (const card of episode.events || []) {
      if (!modes.has(card.presentation_mode)) add('blocking', 'INVALID_PRESENTATION_MODE', '未知呈现方式。', card)
      if (card.presentation_mode === 'merge' && !rules.some((rule) => rule.rule_type === 'merge_allowed' && matchesRule(rule, card))) add('blocking', 'MERGE_NOT_AUTHORIZED', '合并呈现缺少授权规则。', card)
      for (const rule of rules.filter((item) => matchesRule(item, card))) {
        const severity = rule.enforcement === 'hard' ? 'blocking' : 'warning'
        if (rule.rule_type === 'must_not_change' && card.presentation_mode !== 'preserve') add(severity, 'MUST_NOT_CHANGE_VIOLATION', '受保护事件不能改变。', card, rule)
        if (rule.rule_type === 'transform_required' && card.presentation_mode !== 'transform') add(severity, 'TRANSFORM_REQUIRED_VIOLATION', '事件必须变形改编。', card, rule)
      }
    }
  }
  for (const card of draft.omitted_events || []) {
    const matching = rules.filter((rule) => matchesRule(rule, card))
    if (!matching.some((rule) => rule.rule_type === 'omit_allowed')) add('blocking', 'OMISSION_NOT_AUTHORIZED', '省略事件缺少授权规则。', card)
    for (const rule of matching.filter((rule) => rule.rule_type === 'must_preserve')) add(rule.enforcement === 'hard' ? 'blocking' : 'warning', 'MUST_PRESERVE_VIOLATION', '规则要求保留该事件。', card, rule)
  }
  return {
    passed: !diagnostics.some((item) => item.severity === 'blocking'), diagnostics,
    rule_violations: { hard: diagnostics.filter((item) => item.details.rule_enforcement === 'hard' || item.severity === 'blocking'),
      soft: diagnostics.filter((item) => item.details.rule_enforcement === 'soft') },
  }
}

export function seasonCurves(draft) {
  return {
    emotion: (draft.episodes || []).map((episode) => Math.max(0, ...(episode.emotion_curve || []).map((point) => Number(point.emotion ?? point) || 0))),
    information_reveal: (draft.episodes || []).map((episode) => Number(episode.information_reveal_amount || 0)),
    duration: (draft.episodes || []).map((episode) => Number(episode.estimated_duration_seconds || 0)),
  }
}

export function compareSeasonPlans(plans) {
  return (plans || []).map((plan) => {
    const draft = plan.schema_version === 'season-plan-draft.v1' ? plan : buildSeasonDraft(plan)
    const curves = seasonCurves(draft)
    return { adaptation_plan_id: plan.adaptation_plan_id, plan_name: plan.plan_name || draft.plan_name,
      episode_count: draft.episodes.length, total_duration_seconds: curves.duration.reduce((sum, value) => sum + value, 0),
      average_emotion: curves.emotion.reduce((sum, value) => sum + value, 0) / Math.max(1, curves.emotion.length),
      average_information_reveal: curves.information_reveal.reduce((sum, value) => sum + value, 0) / Math.max(1, curves.information_reveal.length),
      blocking_violations: (plan.validation?.diagnostics || plan.diagnostics || []).filter((item) => item.severity === 'blocking').length }
  })
}
