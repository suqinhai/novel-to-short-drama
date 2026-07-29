\set ON_ERROR_STOP on
SET search_path TO drama, public;

DO $$
DECLARE missing_tables TEXT[];
DECLARE immutable_trigger_count INTEGER;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM drama.schema_migrations
    WHERE version='14' AND checksum='multi-candidate-selection-v1-20260729'
  ) THEN
    RAISE EXCEPTION 'migration 14 ledger row/checksum missing';
  END IF;

  SELECT array_agg(required_name ORDER BY required_name) INTO missing_tables
  FROM unnest(ARRAY[
    'candidate_sets','candidates','candidate_scores','candidate_decisions','candidate_selections',
    'candidate_composition_parts','candidate_hard_rule_results','artifact_current_bindings',
    'candidate_timecode_comments'
  ]) required(required_name)
  WHERE to_regclass('drama.'||required.required_name) IS NULL;
  IF missing_tables IS NOT NULL THEN
    RAISE EXCEPTION 'migration 14 tables missing: %',missing_tables;
  END IF;

  SELECT count(*) INTO immutable_trigger_count
  FROM pg_trigger
  WHERE NOT tgisinternal
    AND tgrelid IN (
      'drama.candidate_sets'::regclass,'drama.candidates'::regclass,
      'drama.candidate_scores'::regclass,'drama.candidate_selections'::regclass,
      'drama.candidate_composition_parts'::regclass,'drama.candidate_hard_rule_results'::regclass
    )
    AND tgname LIKE 'trg_%_immutable';
  IF immutable_trigger_count<>6 THEN
    RAISE EXCEPTION 'expected 6 immutable candidate triggers, got %',immutable_trigger_count;
  END IF;

  IF EXISTS (
    SELECT 1 FROM drama.candidates candidate
    JOIN drama.artifacts artifact USING(artifact_id)
    WHERE artifact.is_current
  ) THEN
    RAISE EXCEPTION 'unselected candidate artifact entered current state';
  END IF;

  IF EXISTS (
    SELECT 1 FROM drama.artifact_current_bindings binding
    JOIN drama.artifacts artifact ON artifact.artifact_id=binding.current_artifact_id
    WHERE artifact.artifact_type<>'candidate_selection' OR NOT artifact.is_current
  ) THEN
    RAISE EXCEPTION 'current binding points to a non-selection or historical artifact';
  END IF;
END $$;

SELECT 'PASS' AS result,
       (SELECT count(*) FROM drama.candidate_sets) AS candidate_set_count,
       (SELECT count(*) FROM drama.candidates) AS candidate_count,
       (SELECT count(*) FROM drama.candidate_selections) AS selection_count;
