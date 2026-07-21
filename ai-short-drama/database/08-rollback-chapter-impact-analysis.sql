BEGIN;

-- Operational rollback only: stop automatic enqueue/publish guarding while
-- retaining every source, IR, impact and user decision row for audit/recovery.
DROP TRIGGER IF EXISTS trg_enqueue_incremental_impact ON drama.narrative_ir_revisions;
DROP TRIGGER IF EXISTS trg_incremental_ir_publish_guard ON drama.narrative_ir_revisions;

COMMENT ON TABLE drama.regeneration_requests IS
  'Phase 4 operationally rolled back: retained read-only for audit. Reapply migration 08 before accepting new requests.';

COMMIT;
