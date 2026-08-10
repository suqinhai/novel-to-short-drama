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
const compareEvent = (left, right) => Number(left.chapter_ordinal || 0) - Number(right.chapter_ordinal || 0) ||
  Number(left.narrative_order) - Number(right.narrative_order) ||
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

const ruleTargetsEvent = (rule, event) => rule?.target_type !== 'free_text' && ruleMatchesEvent(rule, event);

function allocateByDuration(eventUnits, episodeCount, durationTarget) {
  if (!Number.isInteger(episodeCount) || episodeCount < 1 || eventUnits.length < episodeCount ||
      eventUnits.some((unit) => unit.estimated_seconds > durationTarget)) return null;

  const buckets = [];
  for (const unit of eventUnits) {
    const current = buckets[buckets.length - 1];
    const currentSeconds = current?.reduce((sum, item) => sum + item.estimated_seconds, 0) || 0;
    if (!current || currentSeconds + unit.estimated_seconds > durationTarget) buckets.push([unit]);
    else current.push(unit);
  }
  if (buckets.length > episodeCount) return null;

  while (buckets.length < episodeCount) {
    let splitBucketIndex = -1;
    let splitUnitIndex = -1;
    let bestDuration = -1;
    let bestDifference = Infinity;
    buckets.forEach((bucket, bucketIndex) => {
      if (bucket.length < 2) return;
      const total = bucket.reduce((sum, unit) => sum + unit.estimated_seconds, 0);
      let left = 0;
      for (let index = 1; index < bucket.length; index += 1) {
        left += bucket[index - 1].estimated_seconds;
        const difference = Math.abs(total - left * 2);
        if (total > bestDuration || (total === bestDuration && difference < bestDifference)) {
          splitBucketIndex = bucketIndex;
          splitUnitIndex = index;
          bestDuration = total;
          bestDifference = difference;
        }
      }
    });
    if (splitBucketIndex < 0) return null;
    const bucket = buckets[splitBucketIndex];
    buckets.splice(splitBucketIndex, 1, bucket.slice(0, splitUnitIndex), bucket.slice(splitUnitIndex));
  }
  return buckets;
}

function allocateByCount(eventUnits, episodeCount) {
  const buckets = [];
  let cursor = 0;
  for (let number = 1; number <= episodeCount && cursor < eventUnits.length; number += 1) {
    const remainingUnits = eventUnits.length - cursor;
    const remainingEpisodes = episodeCount - number + 1;
    const take = Math.ceil(remainingUnits / remainingEpisodes);
    buckets.push(eventUnits.slice(cursor, cursor + take));
    cursor += take;
  }
  return buckets;
}

function eventSignal(event, key, fallback = 0) {
  const explicit = Number(event?.[key]);
  if (Number.isFinite(explicit)) return Math.max(0, Math.min(1, explicit));
  return Math.max(0, Math.min(1, Number(fallback) || 0));
}

function unitSignals(unit) {
  const events = unit.events || [];
  const average = (key, fallback) => events.length
    ? events.reduce((sum, event) => sum + eventSignal(event, key, fallback(event)), 0) / events.length
    : 0;
  return {
    emotion: average('emotion_intensity', (event) => Number(event.importance ?? 0.5)),
    information: average('information_reveal', (event) => Number(event.importance ?? 0.5) * 0.7),
    hook: Math.max(0, ...events.map((event) => eventSignal(event, 'hook_strength',
      ['turning_point', 'climax', 'reversal', 'revelation'].includes(event.arc_role || event.event_type)
        ? 0.9 : Number(event.importance ?? 0.5)))),
    characterArc: Math.max(0, ...events.map((event) => eventSignal(event, 'character_arc_weight',
      Number(event.importance ?? 0.5) * (asArray(event.participant_entity_revision_ids).length ? 1 : 0.5)))),
    arcIDs: unique(events.flatMap((event) => asArray(event.story_arc_revision_ids))),
  };
}

// Deterministic dynamic programming over contiguous Narrative IR units. The
// objective explicitly balances runtime, episode-end hook strength, emotional
// rise/fall, character-arc beats and information density. Provider suggestions
// may annotate the result later, but can never alter this hard-rule allocation.
function allocateWithNarrativeConstraints(eventUnits, episodeCount, durationTarget, constraints = {}) {
  if (!Number.isInteger(episodeCount) || episodeCount < 1 || eventUnits.length < episodeCount ||
      eventUnits.some((unit) => unit.estimated_seconds > durationTarget)) return null;
  const maxInfo = Number.isFinite(Number(constraints.information_reveal_per_episode_max))
    ? Number(constraints.information_reveal_per_episode_max) : 1;
  const minHook = Number.isFinite(Number(constraints.ending_hook_min)) ? Number(constraints.ending_hook_min) : 0;
  const prefixSeconds = [0];
  for (const unit of eventUnits) prefixSeconds.push(prefixSeconds[prefixSeconds.length - 1] + unit.estimated_seconds);
  const states = Array.from({length: episodeCount + 1}, () => Array(eventUnits.length + 1).fill(null));
  states[0][0] = {cost: 0, boundaries: []};
  for (let episode = 1; episode <= episodeCount; episode += 1) {
    for (let end = episode; end <= eventUnits.length; end += 1) {
      for (let start = episode - 1; start < end; start += 1) {
        const previous = states[episode - 1][start];
        if (!previous || eventUnits.length - end < episodeCount - episode) continue;
        const seconds = prefixSeconds[end] - prefixSeconds[start];
        if (seconds > durationTarget) continue;
        const units = eventUnits.slice(start, end);
        const signals = units.map(unitSignals);
        const info = signals.reduce((sum, item) => sum + item.information, 0) / Math.max(1, signals.length);
        const hook = signals[signals.length - 1]?.hook || 0;
        const emotions = signals.map((item) => item.emotion);
        const peak = Math.max(...emotions);
        const peakIndex = emotions.indexOf(peak);
        const idealPeak = Math.max(0, emotions.length - 2);
        const characterArc = Math.max(...signals.map((item) => item.characterArc));
        const arcCoverage = unique(signals.flatMap((item) => item.arcIDs)).length;
        const durationPenalty = Math.pow((durationTarget - seconds) / durationTarget, 2) * 100;
        const hookPenalty = Math.max(0, minHook - hook) * 160 - hook * 22;
        const informationPenalty = Math.max(0, info - maxInfo) * 140 + Math.abs(info - Math.min(maxInfo, 0.55)) * 9;
        const emotionPenalty = Math.abs(peakIndex - idealPeak) * 5 - peak * 8;
        const characterPenalty = characterArc ? -characterArc * 6 : 8;
        const arcPenalty = arcCoverage ? -Math.min(3, arcCoverage) * 2 : 5;
        const cost = previous.cost + durationPenalty + hookPenalty + informationPenalty + emotionPenalty + characterPenalty + arcPenalty;
        const current = states[episode][end];
        const boundaries = [...previous.boundaries, end];
        if (!current || cost < current.cost - 1e-9 ||
            (Math.abs(cost - current.cost) < 1e-9 && JSON.stringify(boundaries) < JSON.stringify(current.boundaries))) {
          states[episode][end] = {cost, boundaries};
        }
      }
    }
  }
  const best = states[episodeCount][eventUnits.length];
  if (!best) return null;
  const buckets = [];
  let start = 0;
  for (const end of best.boundaries) {
    buckets.push(eventUnits.slice(start, end));
    start = end;
  }
  return {buckets, score: Number(best.cost.toFixed(6)), strategy: 'narrative_constraint_dp'};
}

function allocateWithBalancedMerges(eventUnits, episodeCount, durationTarget, rules, compilerRunID) {
  if (!Number.isInteger(episodeCount) || episodeCount < 1 || eventUnits.length < episodeCount) return null;

  const segment = (start, end) => {
    const units = eventUnits.slice(start, end);
    const events = units.flatMap((unit) => unit.events);
    if (units.length === 1) return {...units[0], events};

    const authorizers = rules.filter((rule) => rule.rule_type === 'merge_allowed' &&
      events.every((event) => ruleMatchesEvent(rule, event)));
    const immutable = rules.some((rule) => rule.rule_type === 'must_not_change' && rule.enforcement === 'hard' &&
      events.some((event) => ruleTargetsEvent(rule, event)));
    if (!authorizers.length || immutable) return null;

    const estimatedSeconds = units.slice(1).reduce((seconds, unit) =>
      Math.max(15, Math.round((seconds + unit.estimated_seconds) * 0.72)), units[0].estimated_seconds);
    if (estimatedSeconds > durationTarget) return null;
    return {
      events,
      estimated_seconds: estimatedSeconds,
      merge_group_id: makeID('merge_', [compilerRunID, ...events.map((event) => event.event_revision_id)]),
      merge_rule_ids: unique(authorizers.map((rule) => rule.adaptation_rule_id)).sort(),
    };
  };

  const states = Array.from({length: episodeCount + 1}, () => Array(eventUnits.length + 1).fill(null));
  states[0][0] = {cost: 0, groups: []};
  for (let groupCount = 1; groupCount <= episodeCount; groupCount += 1) {
    for (let end = groupCount; end <= eventUnits.length; end += 1) {
      for (let start = groupCount - 1; start < end; start += 1) {
        const previous = states[groupCount - 1][start];
        if (!previous || eventUnits.length - end < episodeCount - groupCount) continue;
        const unit = segment(start, end);
        if (!unit || unit.estimated_seconds > durationTarget) continue;
        const underfill = durationTarget - unit.estimated_seconds;
        const cost = previous.cost + underfill * underfill;
        const current = states[groupCount][end];
        if (!current || cost < current.cost) states[groupCount][end] = {cost, groups: [...previous.groups, unit]};
      }
    }
  }
  const best = states[episodeCount][eventUnits.length];
  return best ? {eventUnits: best.groups, buckets: best.groups.map((unit) => [unit])} : null;
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
      run.ir_revision_id !== spec.ir_revision_id || spec.status !== 'active' || input.ir_status !== 'published' ||
      input.ir_scope !== 'full') {
    block('FROZEN_INPUT_MISMATCH', 'Compiler inputs are not one active spec, one published full IR and one source version.');
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
  const planningConstraints = spec.planning_constraints || input.planning_constraints || {};
  if (!Number.isInteger(episodeCount) || episodeCount < 1 || !Number.isInteger(durationTarget) || durationTarget < 1) {
    block('INVALID_TARGET_FORMAT', 'Episode count and duration must be positive integers.');
  }
  let eventUnits = ordered.map((event) => ({
    events: [event],
    estimated_seconds: Math.max(12, Math.round(18 + Number(event.importance ?? 0.5) * 42)),
    merge_group_id: null,
    merge_rule_ids: [],
  }));
  const totalCapacity = Math.max(0, episodeCount * durationTarget);
  let totalSeconds = eventUnits.reduce((sum, unit) => sum + unit.estimated_seconds, 0);
  let narrativeAllocation = allocateWithNarrativeConstraints(
    eventUnits, episodeCount, durationTarget, planningConstraints,
  );
  let buckets = narrativeAllocation?.buckets || null;
  if (!buckets) {
    const balanced = allocateWithBalancedMerges(eventUnits, episodeCount, durationTarget, rules, run.compiler_run_id);
    if (balanced) {
      eventUnits = balanced.eventUnits;
      narrativeAllocation = allocateWithNarrativeConstraints(
        eventUnits, episodeCount, durationTarget, planningConstraints,
      );
      buckets = narrativeAllocation?.buckets || balanced.buckets;
      totalSeconds = eventUnits.reduce((sum, unit) => sum + unit.estimated_seconds, 0);
    }
  }
  while (!buckets && eventUnits.length > episodeCount) {
    const candidates = [];
    for (let index = 0; index < eventUnits.length - 1; index += 1) {
      const left = eventUnits[index];
      const right = eventUnits[index + 1];
      const eventsToMerge = [...left.events, ...right.events];
      const authorizers = rules.filter((rule) => rule.rule_type === 'merge_allowed' &&
        eventsToMerge.every((event) => ruleMatchesEvent(rule, event)));
      const immutable = rules.some((rule) => rule.rule_type === 'must_not_change' && rule.enforcement === 'hard' &&
        eventsToMerge.some((event) => ruleTargetsEvent(rule, event)));
      const mergedSeconds = Math.max(15, Math.round((left.estimated_seconds + right.estimated_seconds) * 0.72));
      if (!authorizers.length || immutable || mergedSeconds > durationTarget) continue;
      candidates.push({index, left, right, eventsToMerge, authorizers, mergedSeconds});
    }
    if (!candidates.length) break;
    candidates.sort((left, right) => left.eventsToMerge.length - right.eventsToMerge.length ||
      left.mergedSeconds - right.mergedSeconds || left.index - right.index);
    const candidate = candidates[0];
    const group = {
      events: candidate.eventsToMerge,
      estimated_seconds: candidate.mergedSeconds,
      merge_group_id: makeID('merge_', [run.compiler_run_id, ...candidate.eventsToMerge.map((event) => event.event_revision_id)]),
      merge_rule_ids: unique([
        ...candidate.left.merge_rule_ids, ...candidate.right.merge_rule_ids,
        ...candidate.authorizers.map((rule) => rule.adaptation_rule_id),
      ]).sort(),
    };
    eventUnits.splice(candidate.index, 2, group);
    totalSeconds = eventUnits.reduce((sum, unit) => sum + unit.estimated_seconds, 0);
    narrativeAllocation = allocateWithNarrativeConstraints(
      eventUnits, episodeCount, durationTarget, planningConstraints,
    );
    buckets = narrativeAllocation?.buckets || null;
  }
  if (totalSeconds > totalCapacity) block('DURATION_CAPACITY_EXCEEDED', `所选事件预计需要 ${totalSeconds} 秒，超过目标总容量 ${totalCapacity} 秒；请增加集数或单集时长、缩小范围，或补充明确的允许合并/省略规则。`, 'adaptation_spec_version', run.adaptation_spec_version_id, {estimated_seconds: totalSeconds, capacity_seconds: totalCapacity});
  if (eventUnits.length < episodeCount) block('TOO_FEW_EVENT_UNITS', `可独立分配的事件单元只有 ${eventUnits.length} 个，少于目标 ${episodeCount} 集。`, 'adaptation_spec_version', run.adaptation_spec_version_id, {event_units: eventUnits.length, target_episode_count: episodeCount});
  stage('event_compression_merge', {event_unit_count: eventUnits.length, estimated_seconds: totalSeconds, merge_groups: eventUnits.filter((unit) => unit.merge_group_id).map((unit) => ({merge_group_id: unit.merge_group_id, source_event_ids: unit.events.map((event) => event.event_revision_id), rule_ids: unit.merge_rule_ids}))});

  buckets ||= allocateByCount(eventUnits, episodeCount);
  stage('episode_allocation', {
    episode_count: buckets.length,
    strategy: narrativeAllocation?.strategy || 'deterministic_fallback',
    objective_score: narrativeAllocation?.score ?? null,
    constraints: planningConstraints,
    event_counts: buckets.map((bucket) => bucket.reduce((sum, unit) => sum + unit.events.length, 0)),
  });

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
      block('EPISODE_DURATION_EXCEEDED', `第 ${index + 1} 集预计 ${seconds} 秒，超过单集目标 ${durationTarget} 秒。`, 'episode', String(index + 1), {estimated_seconds: seconds, target_seconds: durationTarget});
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
      const preserveRules = rules.filter((rule) => ['must_preserve', 'must_not_change'].includes(rule.rule_type) &&
        ruleTargetsEvent(rule, event));
      assignments.push({
        event_revision_id: event.event_revision_id, sequence_number: sequence,
        usage_mode: transformRules.length ? 'transform' : unit.merge_group_id ? 'merge' : 'preserve',
        merge_group_id: unit.merge_group_id,
        rule_ids: unique([...unit.merge_rule_ids, ...transformRules.map((rule) => rule.adaptation_rule_id), ...preserveRules.map((rule) => rule.adaptation_rule_id)]).sort(),
      });
    }
    const first = episodeEvents[0];
    const last = episodeEvents[episodeEvents.length - 1];
    const signals = episodeEvents.map((event, eventIndex) => ({
      position: eventIndex + 1,
      event_revision_id: event.event_revision_id,
      emotion: Number(eventSignal(event, 'emotion_intensity', Number(event.importance ?? 0.5)).toFixed(3)),
    }));
    const climaxEvent = episodeEvents.reduce((best, event) => {
      const score = eventSignal(event, 'emotion_intensity', Number(event.importance ?? 0.5)) +
        (['climax', 'turning_point', 'reversal'].includes(event.arc_role || event.event_type) ? 0.4 : 0);
      return !best || score >= best.score ? {event, score} : best;
    }, null)?.event;
    const revealAmount = Number((episodeEvents.reduce((sum, event) => sum +
      eventSignal(event, 'information_reveal', Number(event.importance ?? 0.5) * 0.7), 0) /
      Math.max(1, episodeEvents.length)).toFixed(3));
    const participantIDs = unique(episodeEvents.flatMap((event) => asArray(event.participant_entity_revision_ids)));
    const storyArcIDs = unique(episodeEvents.flatMap((event) => asArray(event.story_arc_revision_ids)));
    return {
      episode_number: episodeIndex + 1,
      title: `第${episodeIndex + 1}集｜${String(first?.summary || '待审核事件').slice(0, 120)}`,
      logline: episodeEvents.map((event) => event.summary).join('；').slice(0, 4000),
      estimated_duration_seconds: episodeDurations[episodeIndex],
      opening_hook: String(first?.summary || '').slice(0, 4000),
      three_second_opening: String(first?.summary || '').slice(0, 4000),
      first_thirty_seconds_goal: episodeEvents.slice(0, Math.max(1, Math.ceil(episodeEvents.length / 3)))
        .map((event) => event.summary).join('；').slice(0, 4000),
      core_conflict: String(episodeEvents.reduce((best, event) =>
        Number(event.importance ?? 0) >= Number(best?.importance ?? -1) ? event : best, null)?.summary || '').slice(0, 4000),
      climax: String(climaxEvent?.summary || last?.summary || '').slice(0, 4000),
      ending_hook: String(last?.summary || '').slice(0, 4000),
      emotion_curve: signals,
      information_reveal_amount: revealAmount,
      character_arc_entity_ids: participantIDs,
      story_arc_revision_ids: storyArcIDs,
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
    pacing_valid: true,
    hook_valid: true,
    emotion_valid: true,
    character_arc_valid: true,
    information_reveal_valid: true,
  };
  for (const episode of episodes) {
    const episodeID = String(episode.episode_number);
    const endEvent = events.find((event) => event.event_revision_id === episode.source_event_ids.at(-1));
    const hook = eventSignal(endEvent, 'hook_strength', Number(endEvent?.importance ?? 0.5));
    const minimumHook = Number(planningConstraints.ending_hook_min ?? 0);
    if (minimumHook > 0 && hook < minimumHook) {
      block('ENDING_HOOK_BELOW_MINIMUM', 'Episode ending hook is below the configured minimum.', 'episode', episodeID,
        {actual: hook, minimum: minimumHook});
      validation.hook_valid = false;
    }
    const maximumReveal = Number(planningConstraints.information_reveal_per_episode_max ?? 1);
    if (episode.information_reveal_amount > maximumReveal) {
      block('INFORMATION_REVEAL_EXCEEDED', 'Episode information reveal exceeds the configured maximum.', 'episode', episodeID,
        {actual: episode.information_reveal_amount, maximum: maximumReveal});
      validation.information_reveal_valid = false;
    }
    if (episode.emotion_curve.length > 1 && Math.max(...episode.emotion_curve.map((item) => item.emotion)) -
        Math.min(...episode.emotion_curve.map((item) => item.emotion)) < Number(planningConstraints.minimum_emotion_range ?? 0)) {
      block('EMOTION_CURVE_TOO_FLAT', 'Episode emotion range is below the configured minimum.', 'episode', episodeID);
      validation.emotion_valid = false;
    }
    if (Number(planningConstraints.character_arc_min_beats ?? 0) > episode.character_arc_entity_ids.length) {
      block('CHARACTER_ARC_BEATS_INSUFFICIENT', 'Episode lacks the configured minimum character-arc coverage.', 'episode', episodeID);
      validation.character_arc_valid = false;
    }
  }
  validation.hard_rules_satisfied = !diagnostics.some((item) => item.severity === 'blocking');
  const creativeSuggestions = asArray(input.provider_suggestions).map((suggestion, index) => ({
    suggestion_id: String(suggestion.suggestion_id || makeID('suggestion_', [run.compiler_run_id, index, suggestion.summary || ''])),
    provider: String(suggestion.provider || 'unknown').slice(0, 200),
    scope: String(suggestion.scope || 'season').slice(0, 200),
    summary: String(suggestion.summary || '').slice(0, 4000),
    rationale: String(suggestion.rationale || '').slice(0, 4000),
  })).filter((suggestion) => suggestion.summary);
  const plan = {
    schema_version: 'compiler-plan.v2', compiler_run_id: run.compiler_run_id, episodes, diagnostics, validation,
    season_curves: {
      emotion: episodes.map((episode) => Math.max(...episode.emotion_curve.map((item) => item.emotion))),
      information_reveal: episodes.map((episode) => episode.information_reveal_amount),
      duration: episodes.map((episode) => episode.estimated_duration_seconds),
    },
    creative_suggestions: creativeSuggestions,
  };
  stage('reviewable_plan', {episode_count: episodes.length, output_hash: digest(plan), blocking_diagnostic_count: diagnostics.filter((item) => item.severity === 'blocking').length});
  const publishable = Object.values(validation).every(Boolean) && episodes.length === episodeCount && episodes.length > 0;
  return {plan, stages, publishable, output_hash: digest(plan), pipeline: PIPELINE};
}

module.exports = {PIPELINE, compile, digest};
