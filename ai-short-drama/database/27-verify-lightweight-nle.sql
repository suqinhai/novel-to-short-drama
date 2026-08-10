\set ON_ERROR_STOP on
SET search_path TO drama, public;

DO $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.schema_migrations
    WHERE version='27' AND checksum='lightweight-nle-v1-20260810') THEN
    RAISE EXCEPTION 'migration 27 ledger entry is missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_trigger
    WHERE tgname='trg_render_job_promote_timeline' AND NOT tgisinternal) THEN
    RAISE EXCEPTION 'render-gated current promotion trigger is missing';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM pg_indexes
    WHERE schemaname='drama' AND indexname='idx_edit_timeline_items_window') THEN
    RAISE EXCEPTION 'timeline window index is missing';
  END IF;
END $$;
