\set ON_ERROR_STOP on
SET search_path TO drama, public;

DO $$
DECLARE missing_columns TEXT[]; bad_count BIGINT;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM drama.schema_migrations
    WHERE version='07' AND checksum='adaptation-compiler-audit-v1-20260721'
  ) THEN
    RAISE EXCEPTION 'migration 07 ledger row/checksum missing';
  END IF;

  SELECT array_agg(required_name ORDER BY required_name) INTO missing_columns
  FROM unnest(ARRAY[
    'source_event_ids','source_chapter_ids','added_adaptation_content','merged_content','deviation_notes'
  ]) required(required_name)
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema='drama' AND table_name='adaptation_episode_plans'
      AND column_name=required.required_name
  );
  IF missing_columns IS NOT NULL THEN
    RAISE EXCEPTION 'migration 07 columns missing: %',missing_columns;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.adaptation_episode_plans ep
  WHERE jsonb_typeof(ep.source_event_ids)<>'array'
     OR jsonb_typeof(ep.source_chapter_ids)<>'array'
     OR jsonb_typeof(ep.added_adaptation_content)<>'array'
     OR jsonb_typeof(ep.merged_content)<>'array'
     OR jsonb_typeof(ep.deviation_notes)<>'array';
  IF bad_count<>0 THEN
    RAISE EXCEPTION 'episode audit fields contain non-array values: %',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.adaptation_episode_plans ep
  WHERE EXISTS (
    SELECT 1 FROM jsonb_array_elements_text(ep.source_event_ids) event_id
    WHERE NOT EXISTS (
      SELECT 1 FROM drama.episode_event_assignments assignment
      WHERE assignment.adaptation_episode_plan_id=ep.adaptation_episode_plan_id
        AND assignment.event_revision_id=event_id
    )
  ) OR EXISTS (
    SELECT 1 FROM drama.episode_event_assignments assignment
    WHERE assignment.adaptation_episode_plan_id=ep.adaptation_episode_plan_id
      AND NOT ep.source_event_ids ? assignment.event_revision_id
  );
  IF bad_count<>0 THEN
    RAISE EXCEPTION 'episode source_event_ids disagree with normalized assignments: %',bad_count;
  END IF;
END $$;

SELECT 'PASS' AS result,
       (SELECT count(*) FROM drama.adaptation_episode_plans) AS episode_plan_count,
       (SELECT count(*) FROM drama.compiler_checkpoints) AS compiler_checkpoint_count;
