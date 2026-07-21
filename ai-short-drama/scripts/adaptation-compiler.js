'use strict';

const crypto = require('crypto');

const PIPELINE = [
  'source_scope_resolution',
  'event_selection',
  'prerequisite_ordering',
  'event_compression_merge',
  'episode_allocation',
  'character_state_validation',
  'foreshadow_validation',
  'duration_validation',
  'reviewable_plan',
];

const stable = (value) => {
  if (Array.isArray(value)) return value.map(stable);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.keys(value).sort().map((key) => [key, stable(value[key])]));
  }
  return value;
};
const digest = (value) => crypto.createHash('sha256').update(JSON.stringify(stable(value))).digest('hex');
const makeID = (prefix, value) => `${prefix}${digest(value).slice(0, 24)}`;
const unique = (items) => [...new Set(items.filter(Boolean))];
const asArray = (value) => Array.isArray(value) ? value : [];
const compareEvent = (left, right) => Number(left.narrative_order) - Number(right.narrative_order) ||
  String(left.event_revision_id).localeCompare(String(right.event_revision_id));
const diagnostic = (severity, code, message, entityType = null, entityID = null, details = {}) => ({
  severity, code, message, entity_type: entityType, entity_id: entityID, details,
});

function ruleMatchesEvent(rule, event) {
  if (!rule) return false;
  if (rule.target_type === 'free_text') return true;
  if (rule.target_type === 'event') return rule.target_id === event.event_revision_id;
  if (rule.target_type === 'fact') return rule.target_id === event.fact_revision_id;
  if (rule.target_type === 'chapter') return rule.target_id === event.chapter_id;
  if (rule.target_type === 'story_arc') return asArray(event.story_arc_revision_ids).includes(rule.target_id);
  if (rule.target_type === 'entity') return asArray(event.participant_entity_revision_ids).includes(rule.target_id);
  if (rule.target_type === 'attribute') {
    const owner = rule.parameters?.owner_id;
    return owner === event.event_revision_id || owner === event.fact_revision_id || owner === event.chapter_id ||
      asArray(event.story_arc_revision_ids).includes(owner) || asArray(event.participant_entity_revision_ids).includes(owner);
  }
  return false;
}

function compile(input) {
  const run = input?.run || {};
  const spec = input?.spec || {};
  const events = asArray(input?.events).map((event) => ({...event})).sort(compareEvent);
  const rules = asArray(input?.rules).map((rule) => ({...rule, parameters: rule.parameters || {}}));
  const diagnostics = [];
  const stages = [];
  const stage = (name, data) => stages.push({stage: name, status: 'completed', data});
  const block = (code, message, entityType, entityID, details) => diagnostics.push(
    diagnostic('blocking', code, message, entityType, entityID, details),
  );

  if (!run.compiler_run_id || !spec.source_version_id || run.source_version_id !== spec.source_version_id ||
      run.ir_revision_id !== spec.ir_revision_id || spec.status !== 'active' || input.ir_status !== 'published') {
    block('FROZEN_INPUT_MISMATCH', 'Compiler inputs are not one active spec, one published IR and one source version.');
  }

  const includeChapters = new Set(asArray(input.scope_chapters).filter((item) => item.include_mode === 'include').map((item) => item.chapter_id));
  const excludeChapters = new Set(asArray(input.scope_chapters).filter((item) => item.include_mode === 'exclude').map((item) => item.chapter_id));
  const includeArcs = new Set(asArray(input.scope_arcs).filter((item) => item.include_mode === 'include').map((item) => item.story_arc_revision_id));
  const excludeArcs = new Set(asArray(input.scope_arcs).filter((item) => item.include_mode === 'exclude').map((item) => item.story_arc_revision_id));
  const inChapterScope = (event) => includeChapters.has(event.chapter_id);
  const inArcScope = (event) => asArray(event.story_arc_revision_ids).some((id) => includeArcs.has(id));
  const excluded = (event) => excludeChapters.has(event.chapter_id) || asArray(event.story_arc_revision_ids).some((id) => excludeArcs.has(id));
  const inScope = (event) => {
    if (excluded(event)) return false;
    if (spec.scope_mode === 'chapters_only') return inChapterScope(event);
    if (spec.scope_mode === 'arcs_only') return inArcScope(event);
    if (spec.scope_mode === 'intersection') return inChapterScope(event) && inArcScope(event);
    return inChapterScope(event) || inArcScope(event);
  };
  const scoped = events.filter(inScope);
  if (!scoped.length) block('EMPTY_SOURCE_SCOPE', 'The resolved chapter/story-arc scope contains no Narrative IR events.');
  stage('source_scope_resolution', {
    scope_mode: spec.scope_mode,
    include_chapter_ids: [...includeChapters].sort(),
    include_story_arc_revision_ids: [...includeArcs].sort(),
    candidate_event_count: scoped.length,
  });

  const selected = scoped.slice();
  const selectedIDs = new Set(selected.map((event) => event.event_revision_id));
  for (const rule of rules.filter((item) => item.enforcement === 'hard' && item.rule_type === 'must_preserve')) {
    let satisfied = false;
    if (rule.target_type === 'event') satisfied = selectedIDs.has(rule.target_id);
    else if (rule.target_type === 'chapter') satisfied = selected.some((event) => event.chapter_id === rule.target_id);
    else if (rule.target_type === 'story_arc') satisfied = selected.some((event) => asArray(event.story_arc_revision_ids).includes(rule.target_id));
    else if (rule.target_type === 'fact') satisfied = selected.some((event) => event.fact_revision_id === rule.target_id);
    else if (rule.target_type === 'entity') satisfied = selected.some((event) => asArray(event.participant_entity_revision_ids).includes(rule.target_id));
    else satisfied = selected.some((event) => ruleMatchesEvent(rule, event));
    if (!satisfied) block('MUST_PRESERVE_OUTSIDE_SCOPE', 'A hard must_preserve target is absent from the resolved source scope.', rule.target_type, rule.target_id, {rule_id: rule.adaptation_rule_id});
  }
  stage('event_selection', {selected_event_ids: selected.map((event) => event.event_revision_id)});

  const byID = new Map(selected.map((event) => [event.event_revision_id, event]));
  const outgoing = new Map(selected.map((event) => [event.event_revision_id, new Set()]));
  const indegree = new Map(selected.map((event) => [event.event_revision_id, 0]));
  for (const relation of asArray(input.relations)) {
    let from = relation.from_event_revision_id;
    let to = relation.to_event_revision_id;
    if (relation.relation_type === 'after') [from, to] = [to, from];
    if (!['before', 'after', 'causes', 'enables'].includes(relation.relation_type) || !byID.has(from) || !byID.has(to)) continue;
    if (!outgoing.get(from).has(to)) {
      outgoing.get(from).add(to);
      indegree.set(to, indegree.get(to) + 1);
    }
  }
  const ready = selected.filter((event) => indegree.get(event.event_revision_id) === 0).sort(compareEvent);
  const ordered = [];
  while (ready.length) {
    const event = ready.shift();
    ordered.push(event);
    for (const nextID of [...outgoing.get(event.event_revision_id)].sort()) {
      indegree.set(nextID, indegree.get(nextID) - 1);
      if (indegree.get(nextID) === 0) {
        ready.push(byID.get(nextID));
        ready.sort(compareEvent);
      }
    }
  }
  if (ordered.length !== selected.length) {
    block('PREREQUISITE_CYCLE', 'Causal or prerequisite relations contain a cycle.', 'ir_revision', run.ir_revision_id);
    for (const event of selected) if (!ordered.includes(event)) ordered.push(event);
  }
  const reorderedIDs = ordered.filter((event, index) => event.event_revision_id !== selected[index]?.event_revision_id).map((event) => event.event_revision_id);
  stage('prerequisite_ordering', {ordered_event_ids: ordered.map((event) => event.event_revision_id), reordered_event_ids: reorderedIDs});

  const episodeCount = Number(spec.target_episode_count || 0);
  const durationTarget = Number(spec.episode_duration_seconds || 0);
  if (!Number.isInteger(episodeCount) || episodeCount < 1 || !Number.isInteger(durationTarget) || durationTarget < 1) {
    block('INVALID_TARGET_FORMAT', 'Episode count and duration must be positive integers.');
  }
  const eventUnits = ordered.map((event) => ({
    events: [event],
    estimated_seconds: Math.max(12, Math.round(18 + Number(event.importance ?? 0.5) * 42)),
    merge_group_id: null,
    merge_rule_ids: [],
  }));
  const totalCapacity = Math.max(0, episodeCount * durationTarget);
  let totalSeconds = eventUnits.reduce((sum, unit) => sum + unit.estimated_seconds, 0);
  for (let index = 0; totalSeconds > totalCapacity && index < eventUnits.length - 1 && eventUnits.length > episodeCount; index += 1) {
    const left = eventUnits[index];
    const right = eventUnits[index + 1];
    if (left.events.length > 1 || right.events.length > 1) continue;
    const leftEvent = left.events[0];
    const rightEvent = right.events[0];
    const authorizers = rules.filter((rule) => rule.rule_type === 'merge_allowed' && ruleMatchesEvent(rule, leftEvent) && ruleMatchesEvent(rule, rightEvent));
    const immutable = rules.some((rule) => rule.rule_type === 'must_not_change' && (ruleMatchesEvent(rule, leftEvent) || ruleMatchesEvent(rule, rightEvent)));
    if (!authorizers.length || immutable) continue;
    const mergedSeconds = Math.max(15, Math.round((left.estimated_seconds + right.estimated_seconds) * 0.72));
    const group = {
      events: [leftEvent, rightEvent], estimated_seconds: mergedSeconds,
      merge_group_id: makeID('merge_', [run.compiler_run_id, leftEvent.event_revision_id, rightEvent.event_revision_id]),
      merge_rule_ids: unique(authorizers.map((rule) => rule.adaptation_rule_id)).sort(),
    };
    eventUnits.splice(index, 2, group);
    totalSeconds = eventUnits.reduce((sum, unit) => sum + unit.estimated_seconds, 0);
  }
  if (totalSeconds > totalCapacity) block('DURATION_CAPACITY_EXCEEDED', 'Selected source events cannot fit the requested season duration under authorized merge rules.', 'adaptation_spec_version', run.adaptation_spec_version_id, {estimated_seconds: totalSeconds, capacity_seconds: totalCapacity});
  if (eventUnits.length < episodeCount) block('TOO_FEW_EVENT_UNITS', 'The source scope has fewer independently allocatable event units than target episodes.', 'adaptation_spec_version', run.adaptation_spec_version_id, {event_units: eventUnits.length, target_episode_count: episodeCount});
  stage('event_compression_merge', {event_unit_count: eventUnits.length, estimated_seconds: totalSeconds, merge_groups: eventUnits.filter((unit) => unit.merge_group_id).map((unit) => ({merge_group_id: unit.merge_group_id, source_event_ids: unit.events.map((event) => event.event_revision_id), rule_ids: unit.merge_rule_ids}))});

  const buckets = [];
  let cursor = 0;
  for (let number = 1; number <= episodeCount && cursor < eventUnits.length; number += 1) {
    const remainingUnits = eventUnits.length - cursor;
    const remainingEpisodes = episodeCount - number + 1;
    const take = Math.ceil(remainingUnits / remainingEpisodes);
    buckets.push(eventUnits.slice(cursor, cursor + take));
    cursor += take;
  }
  stage('episode_allocation', {episode_count: buckets.length, event_counts: buckets.map((bucket) => bucket.reduce((sum, unit) => sum + unit.events.length, 0))});

  const eventPosition = new Map();
  buckets.forEach((bucket, episodeIndex) => bucket.forEach((unit) => unit.events.forEach((event, eventIndex) => {
    eventPosition.set(event.event_revision_id, [episodeIndex, eventIndex]);
  })));
  let characterStateValid = true;
  const stateGroups = new Map();
  for (const change of asArray(input.state_changes).filter((item) => eventPosition.has(item.trigger_event_revision_id))) {
    const key = `${change.character_entity_revision_id}\u0000${change.state_dimension}`;
    if (!stateGroups.has(key)) stateGroups.set(key, []);
    stateGroups.get(key).push(change);
  }
  for (const changes of stateGroups.values()) {
    changes.sort((a, b) => Number(a.sequence_number) - Number(b.sequence_number));
    for (let index = 1; index < changes.length; index += 1) {
      if (JSON.stringify(stable(changes[index - 1].after_state)) !== JSON.stringify(stable(changes[index].before_state))) {
        characterStateValid = false;
        block('CHARACTER_STATE_DISCONTINUITY', 'Character state transitions do not join across selected events.', 'state_change', changes[index].state_change_id, {previous_state_change_id: changes[index - 1].state_change_id});
      }
    }
  }
  stage('character_state_validation', {valid: characterStateValid, checked_state_change_count: [...stateGroups.values()].reduce((sum, items) => sum + items.length, 0)});

  let foreshadowValid = true;
  const threadGroups = new Map();
  for (const occurrence of asArray(input.foreshadow_occurrences).filter((item) => eventPosition.has(item.event_revision_id))) {
    if (!threadGroups.has(occurrence.foreshadow_thread_id)) threadGroups.set(occurrence.foreshadow_thread_id, []);
    threadGroups.get(occurrence.foreshadow_thread_id).push(occurrence);
  }
  for (const [threadID, occurrences] of threadGroups) {
    occurrences.sort((a, b) => Number(a.occurrence_order) - Number(b.occurrence_order));
    const firstPlant = occurrences.findIndex((item) => item.lifecycle_stage === 'planted');
    const firstResolution = occurrences.findIndex((item) => ['partially_resolved', 'resolved'].includes(item.lifecycle_stage));
    if (firstResolution >= 0 && (firstPlant < 0 || firstResolution < firstPlant)) {
      foreshadowValid = false;
      block('FORESHADOW_RESOLUTION_WITHOUT_PLANT', 'A selected foreshadow resolution appears before its planted occurrence.', 'foreshadow_thread', threadID);
    } else if (firstPlant >= 0 && firstResolution < 0) {
      diagnostics.push(diagnostic('warning', 'FORESHADOW_OPEN_AT_SCOPE_END', 'A planted foreshadow thread remains open at the selected scope boundary.', 'foreshadow_thread', threadID));
    }
  }
  stage('foreshadow_validation', {valid: foreshadowValid, checked_thread_count: threadGroups.size});

  let durationValid = buckets.length === episodeCount;
  const episodeDurations = buckets.map((bucket) => bucket.reduce((sum, unit) => sum + unit.estimated_seconds, 0));
  episodeDurations.forEach((seconds, index) => {
    if (seconds > durationTarget) {
      durationValid = false;
      block('EPISODE_DURATION_EXCEEDED', 'An episode exceeds the requested duration.', 'episode', String(index + 1), {estimated_seconds: seconds, target_seconds: durationTarget});
    } else if (seconds < Math.max(1, Math.floor(durationTarget * 0.3))) {
      diagnostics.push(diagnostic('warning', 'EPISODE_DURATION_UNDER_TARGET', 'An episode uses less than 30% of the target duration.', 'episode', String(index + 1), {estimated_seconds: seconds, target_seconds: durationTarget}));
    }
  });
  stage('duration_validation', {valid: durationValid, target_seconds: durationTarget, episode_estimated_seconds: episodeDurations});

  const episodes = buckets.map((bucket, episodeIndex) => {
    const episodeEvents = bucket.flatMap((unit) => unit.events);
    const sourceEventIDs = episodeEvents.map((event) => event.event_revision_id);
    const sourceChapterIDs = unique(episodeEvents.map((event) => event.chapter_id));
    const mergedContent = bucket.filter((unit) => unit.merge_group_id).map((unit) => ({
      merge_group_id: unit.merge_group_id,
      source_event_ids: unit.events.map((event) => event.event_revision_id),
      description: `按规则合并呈现事件：${unit.events.map((event) => event.summary).join(' / ')}`.slice(0, 4000),
      rule_ids: unit.merge_rule_ids,
    }));
    const added = [];
    const deviations = [];
    for (const event of episodeEvents) {
      const transforms = rules.filter((rule) => rule.rule_type === 'transform_required' && ruleMatchesEvent(rule, event));
      for (const rule of transforms) {
        const description = String(rule.parameters?.added_content_description || '').trim();
        if (description) added.push({
          content_id: makeID('added_', [run.compiler_run_id, event.event_revision_id, rule.adaptation_rule_id]),
          description: description.slice(0, 4000), reason: String(rule.rationale || 'transform_required rule').slice(0, 4000),
          rule_ids: [rule.adaptation_rule_id],
        });
        deviations.push({
          deviation_id: makeID('deviation_', ['transform', run.compiler_run_id, event.event_revision_id, rule.adaptation_rule_id]),
          kind: description ? 'addition' : 'transform',
          description: String(rule.rationale || `按规则转换事件：${event.summary}`).slice(0, 4000),
          source_event_ids: [event.event_revision_id], rule_ids: [rule.adaptation_rule_id],
        });
      }
    }
    for (const merge of mergedContent) deviations.push({
      deviation_id: makeID('deviation_', ['merge', run.compiler_run_id, merge.merge_group_id]), kind: 'merge',
      description: merge.description, source_event_ids: merge.source_event_ids, rule_ids: merge.rule_ids,
    });
    for (const eventID of reorderedIDs.filter((id) => sourceEventIDs.includes(id))) deviations.push({
      deviation_id: makeID('deviation_', ['reorder', run.compiler_run_id, eventID]), kind: 'reorder',
      description: '为满足明确的前置、时间或因果关系调整呈现顺序。', source_event_ids: [eventID], rule_ids: [],
    });
    const assignments = [];
    let sequence = 0;
    for (const unit of bucket) for (const event of unit.events) {
      sequence += 1;
      const transformRules = rules.filter((rule) => rule.rule_type === 'transform_required' && ruleMatchesEvent(rule, event));
      const preserveRules = rules.filter((rule) => ['must_preserve', 'must_not_change'].includes(rule.rule_type) && ruleMatchesEvent(rule, event));
      assignments.push({
        event_revision_id: event.event_revision_id, sequence_number: sequence,
        usage_mode: transformRules.length ? 'transform' : unit.merge_group_id ? 'merge' : 'preserve',
        merge_group_id: unit.merge_group_id,
        rule_ids: unique([...unit.merge_rule_ids, ...transformRules.map((rule) => rule.adaptation_rule_id), ...preserveRules.map((rule) => rule.adaptation_rule_id)]).sort(),
      });
    }
    const first = episodeEvents[0];
    const last = episodeEvents[episodeEvents.length - 1];
    return {
      episode_number: episodeIndex + 1,
      title: `第${episodeIndex + 1}集｜${String(first?.summary || '待审核事件').slice(0, 120)}`,
      logline: episodeEvents.map((event) => event.summary).join('；').slice(0, 4000),
      estimated_duration_seconds: episodeDurations[episodeIndex],
      opening_hook: String(first?.summary || '').slice(0, 4000),
      ending_hook: String(last?.summary || '').slice(0, 4000),
      continuity_in: episodeIndex ? [`承接第${episodeIndex}集的事件状态`] : [],
      continuity_out: episodeIndex + 1 < buckets.length ? [`进入第${episodeIndex + 2}集的前置状态`] : [],
      source_event_ids: sourceEventIDs,
      source_chapter_ids: sourceChapterIDs,
      added_adaptation_content: added,
      merged_content: mergedContent,
      deviation_notes: deviations,
      event_assignments: assignments,
    };
  });

  const eventReferencesValid = episodes.every((episode) =>
    episode.source_event_ids.length === episode.event_assignments.length &&
    episode.source_event_ids.every((id, index) => id === episode.event_assignments[index].event_revision_id) &&
    unique(episode.source_chapter_ids).length === episode.source_chapter_ids.length);
  if (!eventReferencesValid) block('EPISODE_SOURCE_AUDIT_MISMATCH', 'Episode source audit arrays disagree with normalized assignments.');
  const hardRulesSatisfied = !diagnostics.some((item) => item.severity === 'blocking');
  const validation = {
    hard_rules_satisfied: hardRulesSatisfied,
    event_references_valid: eventReferencesValid,
    timeline_valid: ordered.length === selected.length,
    causality_valid: ordered.length === selected.length,
    foreshadowing_valid: foreshadowValid,
    duration_valid: durationValid,
  };
  const plan = {schema_version: 'compiler-plan.v2', compiler_run_id: run.compiler_run_id, episodes, diagnostics, validation};
  stage('reviewable_plan', {episode_count: episodes.length, output_hash: digest(plan), blocking_diagnostic_count: diagnostics.filter((item) => item.severity === 'blocking').length});
  const publishable = Object.values(validation).every(Boolean) && episodes.length === episodeCount && episodes.length > 0;
  return {plan, stages, publishable, output_hash: digest(plan), pipeline: PIPELINE};
}

module.exports = {PIPELINE, compile, digest};
