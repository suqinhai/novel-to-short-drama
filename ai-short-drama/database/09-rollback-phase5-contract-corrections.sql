-- Non-destructive operational rollback for migration 09.
-- This intentionally keeps all data, constraints, functions, indexes and the
-- schema_migrations ledger. Re-enable the named triggers to restore bridging.
BEGIN;
SET LOCAL lock_timeout='5s';
SET search_path TO drama,public;

ALTER TABLE drama.novels DISABLE TRIGGER trg_prepare_legacy_source;
ALTER TABLE drama.novel_chapters DISABLE TRIGGER trg_mirror_legacy_chapter;
ALTER TABLE drama.workflow_tasks DISABLE TRIGGER trg_seal_completed_legacy_import;

COMMENT ON TABLE drama.legacy_source_bindings IS
  'Phase 5 live legacy mirroring is operationally disabled; existing mappings and all business data are preserved.';
COMMIT;

-- Restore commands:
-- ALTER TABLE drama.novels ENABLE TRIGGER trg_prepare_legacy_source;
-- ALTER TABLE drama.novel_chapters ENABLE TRIGGER trg_mirror_legacy_chapter;
-- ALTER TABLE drama.workflow_tasks ENABLE TRIGGER trg_seal_completed_legacy_import;
