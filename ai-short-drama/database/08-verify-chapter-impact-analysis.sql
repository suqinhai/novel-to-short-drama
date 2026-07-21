\set ON_ERROR_STOP on

DO $$
DECLARE missing TEXT;
BEGIN
  SELECT string_agg(expected,', ') INTO missing
  FROM unnest(ARRAY[
    'regeneration_requests','regeneration_request_items'
  ]) expected
  WHERE to_regclass('drama.'||expected) IS NULL;
  IF missing IS NOT NULL THEN RAISE EXCEPTION 'missing impact tables: %',missing; END IF;

  IF EXISTS (
    SELECT 1 FROM (VALUES
      ('narrative_ir_revisions','revision_scope'),
      ('narrative_ir_revisions','base_ir_revision_id'),
      ('narrative_ir_revisions','changed_chapter_ids'),
      ('source_change_sets','from_ir_revision_id'),
      ('source_change_sets','to_ir_revision_id'),
      ('source_change_sets','changed_chapter_ids')
    ) required(table_name,column_name)
    WHERE NOT EXISTS (
      SELECT 1 FROM information_schema.columns c
      WHERE c.table_schema='drama' AND c.table_name=required.table_name AND c.column_name=required.column_name
    )
  ) THEN RAISE EXCEPTION 'missing impact analysis columns'; END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes WHERE schemaname='drama' AND indexname='idx_ir_incremental_base'
  ) THEN RAISE EXCEPTION 'missing incremental IR index'; END IF;

  IF EXISTS (
    SELECT 1 FROM drama.narrative_ir_revisions
    WHERE revision_scope='incremental'
      AND (base_ir_revision_id IS NULL OR jsonb_array_length(changed_chapter_ids)=0)
  ) THEN RAISE EXCEPTION 'invalid incremental IR contract rows'; END IF;
END $$;

SELECT 'chapter-impact-contract-ok' AS verification,
  (SELECT count(*) FROM drama.regeneration_requests) AS regeneration_requests,
  (SELECT count(*) FROM drama.source_change_sets WHERE to_ir_revision_id IS NOT NULL) AS analyzed_change_sets;
