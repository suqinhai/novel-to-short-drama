\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:22-restore-effective-input-stage-requirements'));
SET search_path TO drama,public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS (SELECT 1 FROM drama.schema_migrations WHERE version='21') THEN
    RAISE EXCEPTION 'migration 21 must be applied before migration 22';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='22';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'restore-effective-input-stage-requirements-v1-20260804' THEN
    RAISE EXCEPTION 'migration 22 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='22') AS phase22_apply \gset

\if :phase22_apply

INSERT INTO drama.effective_input_stage_requirements(stage_key,input_kind,requirement)
SELECT stage_key,input_kind,
  CASE
    WHEN input_kind IN ('narrative_ir','adaptation_spec','adaptation_plan','episode_plan','pacing_plan')
      THEN 'required'
    WHEN input_kind IN ('performance_bible','continuity_ledger')
      AND stage_key IN ('episode_script','storyboard_design','storyboard_images','image_to_video','voice_audio','post_production')
      THEN 'required'
    WHEN input_kind='visual_profiles' AND stage_key IN ('storyboard_images','image_to_video')
      THEN 'required'
    WHEN input_kind IN ('editing_template','timeline') AND stage_key='post_production'
      THEN 'required'
    ELSE 'optional'
  END
FROM unnest(ARRAY[
  'episode_script','storyboard_design','visual_assets','storyboard_images',
  'image_to_video','voice_audio','post_production'
]) stage_key
CROSS JOIN unnest(ARRAY[
  'narrative_ir','adaptation_spec','adaptation_plan','episode_plan','pacing_plan',
  'candidate_selection','performance_bible','continuity_ledger','visual_profiles',
  'editing_template','timeline'
]) input_kind
ON CONFLICT(stage_key,input_kind) DO UPDATE
SET requirement=EXCLUDED.requirement;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES(
  '22',
  'restore-effective-input-stage-requirements-v1-20260804',
  'Restore effective-input stage requirements removed by business-data reset'
);

\else
\echo 'migration 22 already applied with matching checksum; no-op'
\endif

COMMIT;
