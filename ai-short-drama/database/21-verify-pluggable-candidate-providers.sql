\set ON_ERROR_STOP on
SET search_path TO drama,public;

DO $$
DECLARE missing TEXT;
BEGIN
  SELECT string_agg(required.column_name,',') INTO missing
  FROM (VALUES
    ('candidate_sets','generator_provider'),('candidate_sets','reviewer_provider'),
    ('candidate_sets','frozen_input'),('candidate_sets','frozen_input_hash'),('candidate_sets','client_request_hash'),
    ('candidate_scores','causality'),('candidate_scores','character_consistency'),
    ('candidate_scores','dimensions'),('candidates','provider')
  ) required(table_name,column_name)
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.columns column_row
    WHERE column_row.table_schema='drama' AND column_row.table_name=required.table_name
      AND column_row.column_name=required.column_name
  );
  IF missing IS NOT NULL THEN
    RAISE EXCEPTION 'phase 21 missing columns: %',missing;
  END IF;
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='21') THEN
    RAISE EXCEPTION 'phase 21 migration audit is missing';
  END IF;
END $$;

SELECT 'phase21_verified' AS result;
