BEGIN;

-- Operational rollback only: stop automatic enqueue/publish guarding while
-- retaining every source, IR, impact and user decision row for audit/recovery.
ALTER TABLE drama.narrative_ir_revisions DISABLE TRIGGER trg_enqueue_incremental_impact;
ALTER TABLE drama.narrative_ir_revisions DISABLE TRIGGER trg_incremental_ir_publish_guard;

COMMENT ON TABLE drama.regeneration_requests IS
  'Phase 4 operationally rolled back: retained read-only for audit. Re-enable the named triggers before accepting new requests.';

COMMIT;

-- Restore without changing the migration ledger:
-- ALTER TABLE drama.narrative_ir_revisions ENABLE TRIGGER trg_incremental_ir_publish_guard;
-- ALTER TABLE drama.narrative_ir_revisions ENABLE TRIGGER trg_enqueue_incremental_impact;
