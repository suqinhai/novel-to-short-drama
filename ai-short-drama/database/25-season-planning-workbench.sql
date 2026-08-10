BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(hashtext('drama:25-season-planning-workbench'));

CREATE SCHEMA IF NOT EXISTS drama;
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL THEN
    RAISE EXCEPTION 'migration 06 must be applied before migration 25';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='25';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'season-planning-workbench-v1-20260810' THEN
    RAISE EXCEPTION 'migration 25 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='25') AS phase25_apply \gset

\if :phase25_apply

-- A compiler run freezes source inputs, but editors may derive many immutable
-- whole-season alternatives from that same run.
ALTER TABLE drama.adaptation_plans DROP CONSTRAINT IF EXISTS adaptation_plans_compiler_run_id_key;
ALTER TABLE drama.adaptation_plans
  ADD COLUMN IF NOT EXISTS parent_adaptation_plan_id TEXT,
  ADD COLUMN IF NOT EXISTS plan_name TEXT NOT NULL DEFAULT '整季方案',
  ADD COLUMN IF NOT EXISTS strategy_label TEXT NOT NULL DEFAULT 'deterministic',
  ADD COLUMN IF NOT EXISTS workbench_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS creative_suggestions JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS save_idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS approved_by TEXT,
  ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS validation_run_at TIMESTAMPTZ;

ALTER TABLE drama.adaptation_plans
  ADD CONSTRAINT adaptation_plans_parent_fk
    FOREIGN KEY(parent_adaptation_plan_id) REFERENCES drama.adaptation_plans(adaptation_plan_id) ON DELETE RESTRICT,
  ADD CONSTRAINT adaptation_plans_workbench_json_check CHECK(
    jsonb_typeof(workbench_snapshot)='object' AND jsonb_typeof(creative_suggestions)='array' AND
    NOT drama.jsonb_has_forbidden_provider_payload(workbench_snapshot) AND
    NOT drama.jsonb_has_forbidden_provider_payload(creative_suggestions)),
  ADD CONSTRAINT adaptation_plans_approval_audit_check CHECK(
    (status='approved' AND approved_at IS NOT NULL AND validation_run_at IS NOT NULL) OR status<>'approved') NOT VALID;

CREATE UNIQUE INDEX IF NOT EXISTS uq_adaptation_plan_save_idempotency
  ON drama.adaptation_plans(save_idempotency_key) WHERE save_idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_adaptation_plan_parent ON drama.adaptation_plans(parent_adaptation_plan_id);

ALTER TABLE drama.adaptation_episode_plans
  ADD COLUMN IF NOT EXISTS three_second_opening TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS first_thirty_seconds_goal TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS core_conflict TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS climax TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS emotion_curve JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS information_reveal_amount NUMERIC(6,5) NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS character_arc_entity_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS story_arc_revision_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE drama.adaptation_episode_plans
  ADD CONSTRAINT adaptation_episode_plans_season_fields_check CHECK(
    jsonb_typeof(emotion_curve)='array' AND jsonb_typeof(character_arc_entity_ids)='array' AND
    jsonb_typeof(story_arc_revision_ids)='array' AND information_reveal_amount BETWEEN 0 AND 1 AND
    NOT drama.jsonb_has_forbidden_provider_payload(emotion_curve));

ALTER TABLE drama.episode_event_assignments DROP CONSTRAINT IF EXISTS episode_event_assignments_usage_mode_check;
ALTER TABLE drama.episode_event_assignments ADD CONSTRAINT episode_event_assignments_usage_mode_check
  CHECK(usage_mode IN ('preserve','merge','split','transform','reference'));

CREATE TABLE drama.adaptation_plan_validation_runs (
  id BIGSERIAL PRIMARY KEY,
  validation_run_id TEXT NOT NULL UNIQUE,
  adaptation_plan_id TEXT NOT NULL REFERENCES drama.adaptation_plans(adaptation_plan_id) ON DELETE RESTRICT,
  validation_scope TEXT NOT NULL DEFAULT 'approval' CHECK(validation_scope IN('operation','save','approval')),
  passed BOOLEAN NOT NULL,
  validator_version TEXT NOT NULL,
  checks JSONB NOT NULL DEFAULT '{}'::jsonb,
  diagnostics JSONB NOT NULL DEFAULT '[]'::jsonb,
  input_hash TEXT NOT NULL CHECK(input_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CHECK(jsonb_typeof(checks)='object' AND jsonb_typeof(diagnostics)='array' AND
    NOT drama.jsonb_has_forbidden_provider_payload(checks) AND
    NOT drama.jsonb_has_forbidden_provider_payload(diagnostics))
);
CREATE INDEX idx_adaptation_plan_validation_latest
  ON drama.adaptation_plan_validation_runs(adaptation_plan_id,created_at DESC);

-- Backfill audit timestamps for legacy approved plans without changing their
-- immutable content fields.
UPDATE drama.adaptation_plans
SET approved_at=COALESCE(approved_at,updated_at),validation_run_at=COALESCE(validation_run_at,updated_at),
    approved_by=COALESCE(approved_by,'legacy-adoption')
WHERE status='approved';

ALTER TABLE drama.adaptation_plans VALIDATE CONSTRAINT adaptation_plans_approval_audit_check;

CREATE OR REPLACE FUNCTION drama.guard_reviewable_plan()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' AND NOT EXISTS(SELECT 1 FROM drama.projects WHERE project_id=OLD.project_id) THEN RETURN OLD; END IF;
  IF TG_OP='DELETE' AND OLD.status IN ('waiting_review','approved','rejected') THEN
    RAISE EXCEPTION 'reviewable adaptation plan % is immutable',OLD.adaptation_plan_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.status IN ('waiting_review','approved','rejected') AND (
    NEW.compiler_run_id IS DISTINCT FROM OLD.compiler_run_id OR NEW.project_id IS DISTINCT FROM OLD.project_id OR
    NEW.adaptation_spec_version_id IS DISTINCT FROM OLD.adaptation_spec_version_id OR
    NEW.version_number IS DISTINCT FROM OLD.version_number OR NEW.content_hash IS DISTINCT FROM OLD.content_hash OR
    NEW.quality_report IS DISTINCT FROM OLD.quality_report OR NEW.parent_adaptation_plan_id IS DISTINCT FROM OLD.parent_adaptation_plan_id OR
    NEW.plan_name IS DISTINCT FROM OLD.plan_name OR NEW.strategy_label IS DISTINCT FROM OLD.strategy_label OR
    NEW.workbench_snapshot IS DISTINCT FROM OLD.workbench_snapshot OR NEW.creative_suggestions IS DISTINCT FROM OLD.creative_suggestions
  ) THEN RAISE EXCEPTION 'reviewable adaptation plan % content is immutable',OLD.adaptation_plan_id; END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE OR REPLACE FUNCTION drama.validate_adaptation_plan_for_approval(target_plan_id TEXT)
RETURNS JSONB LANGUAGE plpgsql STABLE AS $$
DECLARE result JSONB;
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.adaptation_plans WHERE adaptation_plan_id=target_plan_id) THEN
    RAISE EXCEPTION 'adaptation plan % does not exist',target_plan_id;
  END IF;
  WITH plan_context AS (
    SELECT plan.*,run.ir_revision_id,spec.episode_duration_seconds,spec.scope_mode
    FROM drama.adaptation_plans plan
    JOIN drama.compiler_runs run ON run.compiler_run_id=plan.compiler_run_id
    JOIN drama.adaptation_spec_versions spec ON spec.adaptation_spec_version_id=plan.adaptation_spec_version_id
    WHERE plan.adaptation_plan_id=target_plan_id
  ), positions AS (
    SELECT assignment.event_revision_id,episode.episode_number,assignment.sequence_number,
      assignment.usage_mode,assignment.rule_trace,
      episode.episode_number::bigint*1000000+assignment.sequence_number AS position
    FROM drama.adaptation_episode_plans episode
    JOIN drama.episode_event_assignments assignment USING(adaptation_episode_plan_id)
    WHERE episode.adaptation_plan_id=target_plan_id
  ), scoped_events AS (
    SELECT event.event_revision_id,event.fact_revision_id,fact.chapter_id
    FROM plan_context context
    JOIN drama.narrative_event_revisions event ON event.ir_revision_id=context.ir_revision_id
    JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
    WHERE NOT EXISTS(SELECT 1 FROM drama.adaptation_scope_chapters excluded
      WHERE excluded.adaptation_spec_version_id=context.adaptation_spec_version_id
        AND excluded.include_mode='exclude' AND excluded.chapter_id=fact.chapter_id)
      AND NOT EXISTS(SELECT 1 FROM drama.adaptation_scope_arcs excluded
        JOIN drama.story_arc_events arc USING(story_arc_revision_id)
        WHERE excluded.adaptation_spec_version_id=context.adaptation_spec_version_id
          AND excluded.include_mode='exclude' AND arc.event_revision_id=event.event_revision_id)
      AND CASE context.scope_mode
        WHEN 'chapters_only' THEN EXISTS(SELECT 1 FROM drama.adaptation_scope_chapters included
          WHERE included.adaptation_spec_version_id=context.adaptation_spec_version_id
            AND included.include_mode='include' AND included.chapter_id=fact.chapter_id)
        WHEN 'arcs_only' THEN EXISTS(SELECT 1 FROM drama.adaptation_scope_arcs included
          JOIN drama.story_arc_events arc USING(story_arc_revision_id)
          WHERE included.adaptation_spec_version_id=context.adaptation_spec_version_id
            AND included.include_mode='include' AND arc.event_revision_id=event.event_revision_id)
        WHEN 'intersection' THEN EXISTS(SELECT 1 FROM drama.adaptation_scope_chapters included
          WHERE included.adaptation_spec_version_id=context.adaptation_spec_version_id
            AND included.include_mode='include' AND included.chapter_id=fact.chapter_id)
          AND EXISTS(SELECT 1 FROM drama.adaptation_scope_arcs included JOIN drama.story_arc_events arc USING(story_arc_revision_id)
            WHERE included.adaptation_spec_version_id=context.adaptation_spec_version_id
              AND included.include_mode='include' AND arc.event_revision_id=event.event_revision_id)
        ELSE EXISTS(SELECT 1 FROM drama.adaptation_scope_chapters included
          WHERE included.adaptation_spec_version_id=context.adaptation_spec_version_id
            AND included.include_mode='include' AND included.chapter_id=fact.chapter_id)
          OR EXISTS(SELECT 1 FROM drama.adaptation_scope_arcs included JOIN drama.story_arc_events arc USING(story_arc_revision_id)
            WHERE included.adaptation_spec_version_id=context.adaptation_spec_version_id
              AND included.include_mode='include' AND arc.event_revision_id=event.event_revision_id)
      END
  ), diagnostics AS (
    SELECT 'blocking' severity,'PLAN_HAS_NO_EPISODES' code,'整季方案至少需要一集。' message,
      NULL::text entity_type,NULL::text entity_id,'{}'::jsonb details
    WHERE NOT EXISTS(SELECT 1 FROM drama.adaptation_episode_plans WHERE adaptation_plan_id=target_plan_id)
    UNION ALL
    SELECT 'blocking','EPISODE_STRUCTURE_INCOMPLETE','批准前必须填写开场3秒、前30秒目标、核心冲突、高潮、结尾钩子和情绪曲线。',
      'episode',episode.episode_number::text,jsonb_build_object('episode_number',episode.episode_number)
    FROM drama.adaptation_episode_plans episode WHERE episode.adaptation_plan_id=target_plan_id AND
      (btrim(episode.three_second_opening)='' OR btrim(episode.first_thirty_seconds_goal)='' OR
       btrim(episode.core_conflict)='' OR btrim(episode.climax)='' OR btrim(episode.ending_hook)='' OR
       jsonb_array_length(episode.emotion_curve)=0)
    UNION ALL
    SELECT 'blocking','EPISODE_DURATION_EXCEEDED','预计时长超过改编规格上限。','episode',episode.episode_number::text,
      jsonb_build_object('actual_seconds',episode.estimated_duration_seconds,'maximum_seconds',context.episode_duration_seconds)
    FROM drama.adaptation_episode_plans episode CROSS JOIN plan_context context
    WHERE episode.adaptation_plan_id=target_plan_id AND episode.estimated_duration_seconds>context.episode_duration_seconds
    UNION ALL
    SELECT 'blocking','CAUSAL_ORDER_VIOLATION','事件呈现顺序违反因果或前置关系。','event_relation',relation.event_relation_id,
      jsonb_build_object('from_event_revision_id',relation.from_event_revision_id,'to_event_revision_id',relation.to_event_revision_id)
    FROM drama.event_relations relation CROSS JOIN plan_context context
    JOIN positions source ON source.event_revision_id=CASE WHEN relation.relation_type='after' THEN relation.to_event_revision_id ELSE relation.from_event_revision_id END
    JOIN positions target ON target.event_revision_id=CASE WHEN relation.relation_type='after' THEN relation.from_event_revision_id ELSE relation.to_event_revision_id END
    WHERE relation.ir_revision_id=context.ir_revision_id AND relation.relation_type IN('before','after','causes','enables')
      AND source.position>=target.position
    UNION ALL
    SELECT 'blocking','FORESHADOW_RESOLUTION_WITHOUT_PLANT','伏笔回收早于埋设。','foreshadow_thread',resolved.foreshadow_thread_id,
      jsonb_build_object('resolution_event_revision_id',resolved.event_revision_id)
    FROM drama.foreshadow_occurrences resolved CROSS JOIN plan_context context
    JOIN positions resolution_position ON resolution_position.event_revision_id=resolved.event_revision_id
    WHERE resolved.ir_revision_id=context.ir_revision_id AND resolved.lifecycle_stage IN('partially_resolved','resolved')
      AND NOT EXISTS(SELECT 1 FROM drama.foreshadow_occurrences planted JOIN positions plant_position
        ON plant_position.event_revision_id=planted.event_revision_id
        WHERE planted.foreshadow_thread_id=resolved.foreshadow_thread_id AND planted.lifecycle_stage='planted'
          AND plant_position.position<resolution_position.position)
    UNION ALL
    SELECT 'blocking','CHARACTER_STATE_ORDER_VIOLATION','人物状态变化顺序与 Narrative IR 不一致。','state_change',later.state_change_id,
      jsonb_build_object('previous_state_change_id',earlier.state_change_id)
    FROM drama.character_state_changes earlier
    JOIN drama.character_state_changes later ON later.ir_revision_id=earlier.ir_revision_id
      AND later.character_entity_revision_id=earlier.character_entity_revision_id
      AND later.state_dimension=earlier.state_dimension AND later.sequence_number>earlier.sequence_number
    CROSS JOIN plan_context context
    JOIN positions earlier_position ON earlier_position.event_revision_id=earlier.trigger_event_revision_id
    JOIN positions later_position ON later_position.event_revision_id=later.trigger_event_revision_id
    WHERE earlier.ir_revision_id=context.ir_revision_id AND earlier_position.position>=later_position.position
      AND NOT EXISTS(SELECT 1 FROM drama.character_state_changes middle
        WHERE middle.ir_revision_id=earlier.ir_revision_id AND middle.character_entity_revision_id=earlier.character_entity_revision_id
          AND middle.state_dimension=earlier.state_dimension AND middle.sequence_number>earlier.sequence_number
          AND middle.sequence_number<later.sequence_number)
    UNION ALL
    SELECT CASE WHEN rule.enforcement='hard' THEN 'blocking' ELSE 'warning' END,
      'MUST_PRESERVE_VIOLATION','规则要求保留的事件在方案中缺失。','adaptation_rule',rule.adaptation_rule_id,
      jsonb_build_object('rule_enforcement',rule.enforcement,'event_revision_id',event.event_revision_id)
    FROM scoped_events event CROSS JOIN plan_context context
    JOIN drama.adaptation_rules rule ON rule.adaptation_spec_version_id=context.adaptation_spec_version_id
      AND rule.rule_type='must_preserve' AND (
        (rule.target_type='event' AND rule.target_id=event.event_revision_id) OR
        (rule.target_type='fact' AND rule.target_id=event.fact_revision_id) OR
        (rule.target_type='chapter' AND rule.target_id=event.chapter_id) OR
        (rule.target_type='story_arc' AND EXISTS(SELECT 1 FROM drama.story_arc_events arc
          WHERE arc.event_revision_id=event.event_revision_id AND arc.story_arc_revision_id=rule.target_id)) OR
        (rule.target_type='entity' AND EXISTS(SELECT 1 FROM drama.event_participants participant
          WHERE participant.event_revision_id=event.event_revision_id AND participant.entity_revision_id=rule.target_id)))
    WHERE NOT EXISTS(SELECT 1 FROM positions WHERE positions.event_revision_id=event.event_revision_id)
    UNION ALL
    SELECT 'blocking','OMISSION_NOT_AUTHORIZED','省略 Narrative IR 事件必须有明确的允许省略规则。','event',event.event_revision_id,'{}'::jsonb
    FROM scoped_events event CROSS JOIN plan_context context
    WHERE NOT EXISTS(SELECT 1 FROM positions WHERE positions.event_revision_id=event.event_revision_id)
      AND NOT EXISTS(SELECT 1 FROM drama.adaptation_rules rule
        WHERE rule.adaptation_spec_version_id=context.adaptation_spec_version_id AND rule.rule_type='omit_allowed' AND (
          rule.target_type='free_text' OR (rule.target_type='event' AND rule.target_id=event.event_revision_id) OR
          (rule.target_type='fact' AND rule.target_id=event.fact_revision_id) OR
          (rule.target_type='chapter' AND rule.target_id=event.chapter_id)))
    UNION ALL
    SELECT CASE WHEN rule.enforcement='hard' THEN 'blocking' ELSE 'warning' END,
      'MUST_NOT_CHANGE_VIOLATION','受保护事件只能原样保留。','adaptation_rule',rule.adaptation_rule_id,
      jsonb_build_object('rule_enforcement',rule.enforcement,'event_revision_id',position.event_revision_id,'usage_mode',position.usage_mode)
    FROM positions position CROSS JOIN plan_context context
    JOIN scoped_events event ON event.event_revision_id=position.event_revision_id
    JOIN drama.adaptation_rules rule ON rule.adaptation_spec_version_id=context.adaptation_spec_version_id
      AND rule.rule_type='must_not_change' AND rule.target_type<>'free_text' AND (
        (rule.target_type='event' AND rule.target_id=event.event_revision_id) OR
        (rule.target_type='fact' AND rule.target_id=event.fact_revision_id) OR
        (rule.target_type='chapter' AND rule.target_id=event.chapter_id))
    WHERE position.usage_mode<>'preserve'
    UNION ALL
    SELECT 'blocking','MERGE_NOT_AUTHORIZED','合并呈现缺少 merge_allowed 规则。','event',position.event_revision_id,'{}'::jsonb
    FROM positions position CROSS JOIN plan_context context
    WHERE position.usage_mode='merge' AND NOT EXISTS(SELECT 1 FROM drama.adaptation_rules rule
      WHERE rule.adaptation_spec_version_id=context.adaptation_spec_version_id AND rule.rule_type='merge_allowed'
        AND (rule.target_type='free_text' OR rule.adaptation_rule_id IN(SELECT jsonb_array_elements_text(position.rule_trace))))
    UNION ALL
    SELECT CASE WHEN rule.enforcement='hard' THEN 'blocking' ELSE 'warning' END,
      'TRANSFORM_REQUIRED_VIOLATION','事件必须按规则执行变形改编。','adaptation_rule',rule.adaptation_rule_id,
      jsonb_build_object('rule_enforcement',rule.enforcement,'event_revision_id',event.event_revision_id)
    FROM scoped_events event CROSS JOIN plan_context context
    JOIN drama.adaptation_rules rule ON rule.adaptation_spec_version_id=context.adaptation_spec_version_id
      AND rule.rule_type='transform_required' AND (
        rule.target_type='free_text' OR (rule.target_type='event' AND rule.target_id=event.event_revision_id) OR
        (rule.target_type='fact' AND rule.target_id=event.fact_revision_id) OR
        (rule.target_type='chapter' AND rule.target_id=event.chapter_id))
    WHERE EXISTS(SELECT 1 FROM positions WHERE positions.event_revision_id=event.event_revision_id)
      AND NOT EXISTS(SELECT 1 FROM positions WHERE positions.event_revision_id=event.event_revision_id AND positions.usage_mode='transform')
    UNION ALL
    SELECT 'blocking','INVALID_ORIGINAL_ADDITION','原创补充必须填写内容和理由。','event_card',card->>'card_id','{}'::jsonb
    FROM plan_context context
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(context.workbench_snapshot->'episodes','[]'::jsonb)) episode
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(episode->'events','[]'::jsonb)) card
    WHERE card->>'presentation_mode'='original' AND
      (btrim(COALESCE(card->>'summary',''))='' OR btrim(COALESCE(card->>'rationale',''))='' OR jsonb_array_length(COALESCE(card->'source_event_ids','[]'::jsonb))<>0)
    UNION ALL
    SELECT 'blocking','DUPLICATE_EVENT_PRESENTATION','同一事件仅可通过拆分呈现重复出现。','event',source.event_revision_id,
      jsonb_build_object('occurrence_count',count(*))
    FROM plan_context context
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(context.workbench_snapshot->'episodes','[]'::jsonb)) episode
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(episode->'events','[]'::jsonb)) card
    CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(card->'source_event_ids','[]'::jsonb)) source(event_revision_id)
    GROUP BY source.event_revision_id HAVING count(*)>1 AND bool_or(card->>'presentation_mode'<>'split')
  ), packed AS (
    SELECT COALESCE(jsonb_agg(jsonb_build_object('severity',severity,'code',code,'message',message,
      'entity_type',entity_type,'entity_id',entity_id,'details',details) ORDER BY code,entity_id),'[]'::jsonb) items,
      count(*) FILTER(WHERE severity='blocking')=0 passed
    FROM diagnostics
  )
  SELECT jsonb_build_object('validator_version','season-workbench-v1','passed',packed.passed,
    'checks',jsonb_build_object(
	  'structure',NOT (packed.items @> '[{"code":"EPISODE_STRUCTURE_INCOMPLETE"}]'::jsonb OR
	    packed.items @> '[{"code":"INVALID_ORIGINAL_ADDITION"}]'::jsonb OR
	    packed.items @> '[{"code":"DUPLICATE_EVENT_PRESENTATION"}]'::jsonb),
      'causality',NOT packed.items @> '[{"code":"CAUSAL_ORDER_VIOLATION"}]'::jsonb,
      'character_state',NOT packed.items @> '[{"code":"CHARACTER_STATE_ORDER_VIOLATION"}]'::jsonb,
      'foreshadowing',NOT packed.items @> '[{"code":"FORESHADOW_RESOLUTION_WITHOUT_PLANT"}]'::jsonb,
      'duration',NOT packed.items @> '[{"code":"EPISODE_DURATION_EXCEEDED"}]'::jsonb,
      'rules',NOT (packed.items @> '[{"code":"MUST_PRESERVE_VIOLATION"}]'::jsonb OR
        packed.items @> '[{"code":"MUST_NOT_CHANGE_VIOLATION"}]'::jsonb OR
        packed.items @> '[{"code":"OMISSION_NOT_AUTHORIZED"}]'::jsonb OR
        packed.items @> '[{"code":"MERGE_NOT_AUTHORIZED"}]'::jsonb OR
		packed.items @> '[{"code":"TRANSFORM_REQUIRED_VIOLATION"}]'::jsonb)),
    'diagnostics',packed.items) INTO result FROM packed;
  RETURN result;
END $$;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES('25','season-planning-workbench-v1-20260810','Versioned whole-season planning workbench and approval validation')
ON CONFLICT(version) DO NOTHING;

\else
\echo 'migration 25 already applied with matching checksum; no-op'
\endif

COMMIT;
