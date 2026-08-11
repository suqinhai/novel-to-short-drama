\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:31-step-8-10-p0-p1-closure'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='30') THEN
    RAISE EXCEPTION 'migration 30 must be applied before migration 31';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='31';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'step-8-10-p0-p1-closure-v1-20260811' THEN
    RAISE EXCEPTION 'migration 31 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='31') AS phase31_apply \gset

\if :phase31_apply

-- A render cannot bypass an already-established cross-layer gate. No gate is
-- permitted for legacy/in-progress episodes, but once one exists its open P0/P1
-- findings and required model review are authoritative for every write path.
CREATE OR REPLACE FUNCTION drama.guard_render_quality_gate()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  latest_run_id TEXT;
  latest_model_status TEXT;
  blocking_count INTEGER;
BEGIN
  SELECT gate_run_id,model_status INTO latest_run_id,latest_model_status
  FROM drama.quality_gate_runs
  WHERE project_id=NEW.project_id AND episode_id=NEW.episode_id
    AND status<>'superseded'
  ORDER BY created_at DESC,gate_run_id DESC LIMIT 1;

  IF latest_run_id IS NULL THEN RETURN NEW; END IF;

  SELECT count(*) INTO blocking_count FROM drama.quality_gate_findings
  WHERE gate_run_id=latest_run_id AND severity='blocking' AND status='open';
  IF blocking_count>0 OR latest_model_status='pending' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE=format('QUALITY_GATE_BLOCKED: run %s has %s open blocking findings and model status %s',
        latest_run_id,blocking_count,latest_model_status);
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER trg_render_jobs_quality_gate
BEFORE INSERT ON drama.render_jobs
FOR EACH ROW EXECUTE FUNCTION drama.guard_render_quality_gate();

-- Export selections are exact immutable release inputs, not merely records that
-- once happened to be approved. Every version must still be the effective
-- current version, and a selected master must point at the selected timeline.
CREATE OR REPLACE FUNCTION drama.guard_export_effective_chain()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  selected_timeline_id TEXT;
  master_timeline_id TEXT;
BEGIN
  selected_timeline_id:=NULLIF(NEW.selection->>'timeline_id','');
  IF selected_timeline_id IS NOT NULL AND NOT EXISTS(
    SELECT 1 FROM drama.edit_timelines timeline
    WHERE timeline.timeline_id=selected_timeline_id
      AND timeline.project_id=NEW.project_id AND timeline.episode_id=NEW.episode_id
      AND timeline.is_current AND timeline.approval_state IN ('approved','restored')
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='EXPORT_STALE_BLOCKED: timeline is not the current approved version';
  END IF;

  IF NULLIF(NEW.selection->>'master_id','') IS NOT NULL THEN
    SELECT master.timeline_id INTO master_timeline_id
    FROM drama.episode_masters master
    JOIN drama.edit_timelines timeline ON timeline.timeline_id=master.timeline_id
    WHERE master.master_id=NEW.selection->>'master_id'
      AND master.project_id=NEW.project_id AND master.episode_id=NEW.episode_id
      AND master.status='ready' AND master.is_current
      AND timeline.is_current AND timeline.approval_state IN ('approved','restored');
    IF master_timeline_id IS NULL THEN
      RAISE EXCEPTION USING ERRCODE='P0001',
        MESSAGE='EXPORT_STALE_BLOCKED: master is not current or does not reference the current approved timeline';
    END IF;
    IF selected_timeline_id IS NOT NULL AND master_timeline_id IS DISTINCT FROM selected_timeline_id THEN
      RAISE EXCEPTION USING ERRCODE='P0001',
        MESSAGE='EXPORT_VERSION_MISMATCH: master and timeline selections do not match';
    END IF;
  END IF;

  IF NULLIF(NEW.selection->>'source_version_id','') IS NOT NULL AND NOT EXISTS(
    SELECT 1 FROM drama.source_versions source_version
    JOIN drama.project_source_bindings binding USING(work_id,source_version_id)
    WHERE source_version.source_version_id=NEW.selection->>'source_version_id'
      AND source_version.status='published' AND source_version.is_current
      AND binding.project_id=NEW.project_id AND binding.binding_role='primary' AND binding.is_current
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='EXPORT_STALE_BLOCKED: source version is not the current published primary binding';
  END IF;

  IF NULLIF(NEW.selection->>'ir_revision_id','') IS NOT NULL AND NOT EXISTS(
    SELECT 1 FROM drama.narrative_ir_revisions ir
    WHERE ir.ir_revision_id=NEW.selection->>'ir_revision_id'
      AND ir.source_version_id=NEW.selection->>'source_version_id'
      AND ir.status='published' AND ir.is_current
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='EXPORT_STALE_BLOCKED: IR is not the current published revision for the selected source';
  END IF;

  IF NULLIF(NEW.selection->>'adaptation_spec_version_id','') IS NOT NULL AND NOT EXISTS(
    SELECT 1 FROM drama.adaptation_spec_versions version
    JOIN drama.adaptation_specs spec USING(adaptation_spec_id,project_id)
    WHERE version.adaptation_spec_version_id=NEW.selection->>'adaptation_spec_version_id'
      AND version.project_id=NEW.project_id
      AND version.source_version_id=NEW.selection->>'source_version_id'
      AND version.ir_revision_id=NEW.selection->>'ir_revision_id'
      AND version.status='active' AND spec.is_current
  ) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='EXPORT_STALE_BLOCKED: adaptation spec is not the current active version for selected Source/IR';
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER trg_professional_export_effective_chain
BEFORE INSERT ON drama.professional_export_jobs
FOR EACH ROW EXECUTE FUNCTION drama.guard_export_effective_chain();

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('31','step 8-10 authoritative QA, render gate and current-only export closure',
  'step-8-10-p0-p1-closure-v1-20260811');

\else
\echo 'migration 31 already applied with matching checksum; no-op'
\endif

COMMIT;
