\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:28-cross-layer-quality-gate'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='27')
     OR to_regclass('drama.final_reviews') IS NULL THEN
    RAISE EXCEPTION 'migration 27 and final review schema must exist before migration 28';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='28';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'cross-layer-quality-gate-v1-20260810' THEN
    RAISE EXCEPTION 'migration 28 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='28') AS phase28_apply \gset

\if :phase28_apply

CREATE TABLE drama.quality_gate_runs (
  id BIGSERIAL PRIMARY KEY,
  gate_run_id TEXT NOT NULL UNIQUE,
  schema_version TEXT NOT NULL DEFAULT 'cross-layer-quality-gate.v1'
    CHECK(schema_version='cross-layer-quality-gate.v1'),
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  master_id TEXT REFERENCES drama.episode_masters(master_id) ON DELETE RESTRICT,
  ruleset_version TEXT NOT NULL CHECK(btrim(ruleset_version)<>''),
  rules_config JSONB NOT NULL CHECK(jsonb_typeof(rules_config)='object'),
  rules_config_hash TEXT NOT NULL CHECK(rules_config_hash ~ '^[0-9a-f]{64}$'),
  prompt_version TEXT,
  snapshot JSONB NOT NULL CHECK(jsonb_typeof(snapshot)='object'),
  snapshot_hash TEXT NOT NULL CHECK(snapshot_hash ~ '^[0-9a-f]{64}$'),
  rule_score NUMERIC(6,2) NOT NULL CHECK(rule_score BETWEEN 0 AND 100),
  rules_status TEXT NOT NULL DEFAULT 'completed' CHECK(rules_status IN ('processing','completed','failed')),
  model_review_required BOOLEAN NOT NULL DEFAULT true,
  model_status TEXT NOT NULL DEFAULT 'pending' CHECK(model_status IN ('not_required','pending','completed','failed')),
  status TEXT NOT NULL DEFAULT 'review_pending'
    CHECK(status IN ('review_pending','review_ready','approved','rejected','superseded')),
  created_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(project_id,episode_id,snapshot_hash,ruleset_version,rules_config_hash),
  CHECK((model_review_required AND model_status<>'not_required') OR
        (NOT model_review_required AND model_status='not_required'))
);
CREATE INDEX idx_quality_gate_runs_episode ON drama.quality_gate_runs(project_id,episode_id,created_at DESC);
CREATE INDEX idx_quality_gate_runs_master ON drama.quality_gate_runs(master_id,status);

CREATE TABLE drama.quality_gate_findings (
  id BIGSERIAL PRIMARY KEY,
  gate_run_id TEXT NOT NULL REFERENCES drama.quality_gate_runs(gate_run_id) ON DELETE CASCADE,
  finding_id TEXT NOT NULL,
  schema_version TEXT NOT NULL DEFAULT 'quality-gate-finding.v1'
    CHECK(schema_version='quality-gate-finding.v1'),
  detector_type TEXT NOT NULL CHECK(detector_type IN ('rule','model')),
  dimension TEXT NOT NULL CHECK(dimension IN (
    'source_fidelity','character_continuity','causality','foreshadowing','hooks',
    'information_density','dialogue_visual_consistency','action_coverage',
    'av_sync_identity','edit_integrity','constraint_compliance'
  )),
  code TEXT NOT NULL CHECK(btrim(code)<>''),
  severity TEXT NOT NULL CHECK(severity IN ('info','warning','major','blocking')),
  message TEXT NOT NULL CHECK(btrim(message)<>''),
  evidence JSONB NOT NULL CHECK(jsonb_typeof(evidence)='array' AND jsonb_array_length(evidence)>0),
  locators JSONB NOT NULL CHECK(jsonb_typeof(locators)='array' AND jsonb_array_length(locators)>0),
  recommendation TEXT NOT NULL CHECK(btrim(recommendation)<>''),
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','resolved','overridden')),
  detector_metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(detector_metadata)='object'),
  resolved_by TEXT,
  resolution_reason TEXT,
  resolved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(gate_run_id,finding_id),
  CHECK((status='open' AND resolved_at IS NULL) OR
        (status<>'open' AND resolved_at IS NOT NULL AND NULLIF(btrim(COALESCE(resolution_reason,'')),'') IS NOT NULL))
);
CREATE INDEX idx_quality_gate_findings_open ON drama.quality_gate_findings(gate_run_id,severity,status);
CREATE INDEX idx_quality_gate_findings_code ON drama.quality_gate_findings(code,dimension);

CREATE TABLE drama.quality_gate_overrides (
  id BIGSERIAL PRIMARY KEY,
  override_id TEXT NOT NULL UNIQUE,
  gate_run_id TEXT NOT NULL,
  finding_id TEXT NOT NULL,
  reason TEXT NOT NULL CHECK(btrim(reason)<>''),
  accepted_by TEXT NOT NULL CHECK(btrim(accepted_by)<>''),
  status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  revoked_at TIMESTAMPTZ,
  FOREIGN KEY(gate_run_id,finding_id)
    REFERENCES drama.quality_gate_findings(gate_run_id,finding_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uq_quality_gate_active_override
  ON drama.quality_gate_overrides(gate_run_id,finding_id) WHERE status='active';

CREATE TABLE drama.quality_gate_change_plans (
  id BIGSERIAL PRIMARY KEY,
  change_plan_id TEXT NOT NULL UNIQUE,
  gate_run_id TEXT NOT NULL,
  finding_id TEXT NOT NULL,
  schema_version TEXT NOT NULL DEFAULT 'quality-gate-change-plan.v1'
    CHECK(schema_version='quality-gate-change-plan.v1'),
  plan JSONB NOT NULL CHECK(jsonb_typeof(plan)='object'),
  status TEXT NOT NULL DEFAULT 'proposed' CHECK(status IN ('proposed','confirmed','cancelled','executed')),
  requested_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(gate_run_id,finding_id)
    REFERENCES drama.quality_gate_findings(gate_run_id,finding_id) ON DELETE RESTRICT,
  UNIQUE(gate_run_id,finding_id)
);

CREATE TABLE drama.quality_gate_master_approvals (
  id BIGSERIAL PRIMARY KEY,
  gate_approval_id TEXT NOT NULL UNIQUE,
  gate_run_id TEXT NOT NULL UNIQUE REFERENCES drama.quality_gate_runs(gate_run_id) ON DELETE RESTRICT,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  episode_id TEXT NOT NULL REFERENCES drama.episode_outlines(episode_id) ON DELETE CASCADE,
  master_id TEXT NOT NULL REFERENCES drama.episode_masters(master_id) ON DELETE RESTRICT,
  approved_by TEXT NOT NULL CHECK(btrim(approved_by)<>''),
  status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked')),
  approved_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  revoked_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_quality_gate_active_master_approval
  ON drama.quality_gate_master_approvals(master_id) WHERE status='active';

CREATE TABLE drama.quality_gate_benchmark_runs (
  id BIGSERIAL PRIMARY KEY,
  benchmark_run_id TEXT NOT NULL UNIQUE,
  suite_id TEXT NOT NULL,
  suite_version INTEGER NOT NULL CHECK(suite_version>0),
  ruleset_version TEXT,
  prompt_version TEXT,
  provider TEXT,
  model TEXT,
  score JSONB NOT NULL CHECK(jsonb_typeof(score)='object'),
  passed BOOLEAN NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_quality_gate_benchmark_version
  ON drama.quality_gate_benchmark_runs(suite_id,suite_version,created_at DESC);

CREATE OR REPLACE FUNCTION drama.enforce_cross_layer_master_gate()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.review_status='approved' AND (
    TG_OP='INSERT' OR OLD.review_status IS DISTINCT FROM NEW.review_status
  ) AND NOT EXISTS (
    SELECT 1 FROM drama.quality_gate_master_approvals approval
    JOIN drama.quality_gate_runs run USING(gate_run_id)
    WHERE approval.master_id=NEW.master_id AND approval.project_id=NEW.project_id
      AND approval.episode_id=NEW.episode_id AND approval.status='active'
      AND run.status='approved' AND run.rules_status='completed'
      AND run.model_status IN ('completed','not_required')
      AND NOT EXISTS (
        SELECT 1 FROM drama.quality_gate_findings finding
        WHERE finding.gate_run_id=run.gate_run_id
          AND finding.severity='blocking' AND finding.status='open'
      )
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='QUALITY_GATE_BLOCKED: master has no valid cross-layer quality approval';
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER trg_final_review_cross_layer_gate
BEFORE INSERT OR UPDATE OF review_status ON drama.final_reviews
FOR EACH ROW EXECUTE FUNCTION drama.enforce_cross_layer_master_gate();

CREATE TRIGGER trg_quality_gate_runs_updated
BEFORE UPDATE ON drama.quality_gate_runs FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
CREATE TRIGGER trg_quality_gate_findings_updated
BEFORE UPDATE ON drama.quality_gate_findings FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
CREATE TRIGGER trg_quality_gate_change_plans_updated
BEFORE UPDATE ON drama.quality_gate_change_plans FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('28','evidence-backed source-to-master cross-layer quality gate','cross-layer-quality-gate-v1-20260810');

\else
\echo 'migration 28 already applied with matching checksum; no-op'
\endif

COMMIT;
