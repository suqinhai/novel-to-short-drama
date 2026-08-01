\set ON_ERROR_STOP on
SET search_path TO drama, public;

DO $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='20'
    AND checksum='incremental-full-ir-merge-v1-20260801') THEN
    RAISE EXCEPTION 'migration 20 is missing or has an unexpected checksum';
  END IF;
  IF to_regclass('drama.ir_merge_proposals') IS NULL
    OR to_regclass('drama.ir_merge_proposal_items') IS NULL
    OR to_regclass('drama.regeneration_proposals') IS NULL
    OR to_regclass('drama.regeneration_proposal_items') IS NULL THEN
    RAISE EXCEPTION 'IR merge or regeneration proposal tables are missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='drama.narrative_ir_revisions'::regclass
    AND attname='merge_proposal_id' AND NOT attisdropped) THEN
    RAISE EXCEPTION 'full IR merge provenance column is missing';
  END IF;
  IF NOT pg_get_functiondef('drama.validate_compiler_frozen_inputs()'::regprocedure) LIKE '%revision_scope=''full''%' THEN
    RAISE EXCEPTION 'database compiler gate does not require a published full IR';
  END IF;
  IF NOT pg_get_functiondef('drama.analyze_chapter_impact(text,uuid)'::regprocedure) LIKE '%semantic_changed%'
    OR NOT pg_get_functiondef('drama.analyze_chapter_impact(text,uuid)'::regprocedure) LIKE '%regeneration_proposals%' THEN
    RAISE EXCEPTION 'semantic-only impact or regeneration proposal logic is missing';
  END IF;
  IF EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='drama.source_change_sets'::regclass AND contype='u'
    AND pg_get_constraintdef(oid)='UNIQUE (from_source_version_id, to_source_version_id)') THEN
    RAISE EXCEPTION 'source-pair uniqueness still prevents authoritative full-IR rescans';
  END IF;
END $$;

SELECT 'PASS' result,
  (SELECT count(*) FROM information_schema.tables WHERE table_schema='drama'
    AND table_name IN ('ir_merge_proposals','ir_merge_proposal_items','regeneration_proposals','regeneration_proposal_items')) tables,
  (SELECT count(*) FROM pg_trigger WHERE tgrelid IN ('drama.ir_merge_proposals'::regclass,
    'drama.ir_merge_proposal_items'::regclass) AND NOT tgisinternal) audit_triggers;
