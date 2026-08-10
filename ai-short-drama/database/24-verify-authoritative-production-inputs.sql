\set ON_ERROR_STOP on
SET search_path TO drama,public;
DO $$
DECLARE default_value TEXT; sample JSONB;
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='24') THEN
    RAISE EXCEPTION 'migration 24 audit is missing';
  END IF;
  IF EXISTS(SELECT 1 FROM drama.projects WHERE input_resolution_mode<>'effective') THEN
    RAISE EXCEPTION 'legacy compatibility mode still exists';
  END IF;
  SELECT column_default INTO default_value FROM information_schema.columns
  WHERE table_schema='drama' AND table_name='candidate_sets' AND column_name='generator_provider';
  IF default_value IS NOT NULL THEN RAISE EXCEPTION 'generator provider still has a default'; END IF;
  IF EXISTS(SELECT 1 FROM information_schema.columns
    WHERE table_schema='drama'
      AND ((table_name='candidate_sets' AND column_name IN('generator_model','reviewer_provider','reviewer_model'))
        OR (table_name='candidates' AND column_name='provider')
        OR (table_name='candidate_scores' AND column_name IN('reviewer_provider','reviewer_model'))
        OR (table_name='incremental_rebuild_tasks' AND column_name='provider')
        OR (table_name='visual_qc_runs' AND column_name='provider'))
      AND column_default IS NOT NULL) THEN
    RAISE EXCEPTION 'a production provider/model column still has an implicit default';
  END IF;
  IF to_regclass('drama.candidate_execution_records') IS NULL
     OR to_regclass('drama.entity_version_bindings') IS NULL
     OR to_regclass('drama.candidate_selection_bindings') IS NULL THEN
    RAISE EXCEPTION 'authoritative audit/binding relations are missing';
  END IF;
  IF to_regclass('drama.uq_change_plans_preview_dedup') IS NULL THEN
    RAISE EXCEPTION 'change plan preview retry dedup index is missing';
  END IF;
  IF to_regprocedure('drama.resolve_effective_inputs(text,text,text)') IS NULL
     OR to_regprocedure('drama.resolve_production_snapshot(text,text,text)') IS NULL THEN
    RAISE EXCEPTION 'authoritative resolver functions are missing';
  END IF;
  sample := drama.effective_item('sample','episode_script','resolved','["id"]','[1]',
    repeat('a',64),'approved','{}','explicit verification','[]');
  IF NOT (sample->'provenance' ?& ARRAY[
    'source_type','source_id','version_id','binding_id','resolved_at','selection_reason'
  ]) THEN
    RAISE EXCEPTION 'resolver provenance is incomplete: %',sample->'provenance';
  END IF;
  sample := drama.effective_item('timeline','episode_script','stale','["timeline"]','[1]',
    repeat('b',64),'stale','{}','downstream output awaiting rebuild','[]');
  IF (sample->>'blocks')::boolean THEN
    RAISE EXCEPTION 'a stale optional downstream output blocks an upstream production stage';
  END IF;
  sample := drama.effective_item('candidate_selection','episode_script','needs_review','[]','[]',
    NULL,'unconfirmed','{}','candidate selection pending','[]');
  IF NOT (sample->>'blocks')::boolean THEN
    RAISE EXCEPTION 'an unconfirmed candidate no longer blocks downstream consumption';
  END IF;
  sample := drama.effective_item('narrative_ir','episode_script','stale','[]','[]',
    NULL,'stale','{}','required upstream is stale','[]');
  IF NOT (sample->>'blocks')::boolean THEN
    RAISE EXCEPTION 'a stale required upstream no longer blocks production';
  END IF;
  IF EXISTS(SELECT 1 FROM drama.effective_input_stage_requirements
    WHERE stage_key='post_production' AND input_kind='timeline' AND requirement<>'optional') THEN
    RAISE EXCEPTION 'post-production still has a circular timeline requirement';
  END IF;
  IF EXISTS(SELECT 1 FROM drama.effective_input_stage_requirements
    WHERE stage_key='visual_assets' AND input_kind IN('episode_plan','pacing_plan')
      AND requirement<>'optional') THEN
    RAISE EXCEPTION 'project-scoped visual assets still require episode-scoped inputs';
  END IF;
END $$;
SELECT 'phase24_verified' AS result;
