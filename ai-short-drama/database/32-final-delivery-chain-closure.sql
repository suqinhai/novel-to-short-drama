\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:32-final-delivery-chain-closure'));
SET search_path TO drama,public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='31') THEN
    RAISE EXCEPTION 'migration 31 must be applied before migration 32';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='32';
  IF existing_checksum IS NOT NULL
     AND existing_checksum<>'final-delivery-chain-closure-v1-20260812' THEN
    RAISE EXCEPTION 'migration 32 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='32') AS phase32_apply \gset
\if :phase32_apply

ALTER TABLE drama.quality_gate_runs
  ADD COLUMN target_timeline_id TEXT REFERENCES drama.edit_timelines(timeline_id) ON DELETE RESTRICT;
UPDATE drama.quality_gate_runs run
SET target_timeline_id=COALESCE(NULLIF(run.snapshot->>'target_timeline_id',''),master.timeline_id)
FROM drama.episode_masters master
WHERE run.master_id=master.master_id AND run.target_timeline_id IS NULL;
CREATE INDEX idx_quality_gate_runs_timeline
  ON drama.quality_gate_runs(target_timeline_id,status,created_at DESC);

-- A finding's detector and its human disposition are separate facts. Keeping
-- this on the finding makes historical QA replay self-contained and prevents
-- a proposed (but unexecuted) plan from being presented as a completed fix.
ALTER TABLE drama.quality_gate_findings
  ADD COLUMN resolution_kind TEXT NOT NULL DEFAULT 'auto_detected'
    CHECK(resolution_kind IN('auto_detected','human_confirmed','resolved_by_rebuild','overridden')),
  ADD COLUMN human_confirmed_by TEXT,
  ADD COLUMN human_confirmation_reason TEXT,
  ADD COLUMN human_confirmed_at TIMESTAMPTZ,
  ADD COLUMN replacement_gate_run_id TEXT REFERENCES drama.quality_gate_runs(gate_run_id) ON DELETE RESTRICT;
UPDATE drama.quality_gate_findings
SET resolution_kind='overridden' WHERE status='overridden';
-- Legacy "resolved" rows only proved that a plan existed. Preserve their
-- human audit but reopen them until a changed replacement snapshot passes QA.
UPDATE drama.quality_gate_findings
SET status='open',resolution_kind='human_confirmed',
    human_confirmed_by=COALESCE(NULLIF(btrim(resolved_by),''),'migration-32'),
    human_confirmation_reason=COALESCE(NULLIF(btrim(resolution_reason),''),'legacy resolution requires replacement QA'),
    human_confirmed_at=COALESCE(resolved_at,now()),
    resolved_by=NULL,resolution_reason=NULL,resolved_at=NULL
WHERE status='resolved';
UPDATE drama.quality_gate_master_approvals approval SET status='revoked',revoked_at=now()
WHERE approval.status='active' AND EXISTS(SELECT 1 FROM drama.quality_gate_findings finding
  WHERE finding.gate_run_id=approval.gate_run_id AND finding.severity='blocking' AND finding.status='open');
UPDATE drama.quality_gate_runs run SET status='review_ready'
WHERE run.status='approved' AND EXISTS(SELECT 1 FROM drama.quality_gate_findings finding
  WHERE finding.gate_run_id=run.gate_run_id AND finding.severity='blocking' AND finding.status='open');
ALTER TABLE drama.quality_gate_findings
  ADD CONSTRAINT quality_gate_finding_disposition_check CHECK(
    (resolution_kind='auto_detected' AND human_confirmed_at IS NULL AND status='open') OR
    (resolution_kind='human_confirmed' AND status='open' AND human_confirmed_at IS NOT NULL
      AND NULLIF(btrim(COALESCE(human_confirmed_by,'')),'') IS NOT NULL
      AND NULLIF(btrim(COALESCE(human_confirmation_reason,'')),'') IS NOT NULL) OR
    (resolution_kind='resolved_by_rebuild' AND status='resolved' AND replacement_gate_run_id IS NOT NULL) OR
    (resolution_kind='overridden' AND status='overridden')
  );

-- Close legacy rows that carried a template/selection id on native content but
-- never published the corresponding Resolver pointer. These are deterministic
-- pointer repairs; no generated content is invented.
INSERT INTO drama.editing_template_bindings(editing_template_binding_id,project_id,episode_id,
  editing_template_version_id,version,override_config,is_current,change_reason,created_by)
SELECT 'etb_'||substr(encode(drama.digest(convert_to(timeline.project_id||timeline.episode_id||
    timeline.editing_template_version_id,'UTF8'),'sha256'),'hex'),1,24),
  timeline.project_id,timeline.episode_id,timeline.editing_template_version_id,1,'{}'::jsonb,true,
  'migration 32 repairs explicit current timeline template pointer','migration-32'
FROM drama.edit_timelines timeline
JOIN drama.editing_template_versions versioned
  ON versioned.editing_template_version_id=timeline.editing_template_version_id AND versioned.status='published'
WHERE timeline.is_current AND timeline.editing_template_version_id IS NOT NULL
  AND NOT EXISTS(SELECT 1 FROM drama.editing_template_bindings binding
    WHERE binding.project_id=timeline.project_id AND binding.episode_id=timeline.episode_id AND binding.is_current)
ON CONFLICT(editing_template_binding_id) DO NOTHING;

INSERT INTO drama.candidate_hard_rule_results(candidate_hard_rule_result_id,candidate_selection_id,
  rule_name,passed,message)
SELECT 'chr_'||substr(encode(drama.digest(convert_to(selection.candidate_selection_id||rule_name,'UTF8'),'sha256'),'hex'),1,24),
  selection.candidate_selection_id,rule_name,true,'backfilled from confirmed validation_summary'
FROM drama.candidate_selections selection
CROSS JOIN unnest(ARRAY['causality','duration','character_state','foreshadowing','continuity']) rule_name
WHERE NULLIF(btrim(selection.confirmed_by),'') IS NOT NULL
  AND COALESCE((selection.validation_summary->>rule_name)::boolean,false)
ON CONFLICT(candidate_selection_id,rule_name) DO NOTHING;

INSERT INTO drama.artifact_current_bindings(artifact_current_binding_id,project_id,target_type,target_id,
  component_scope,current_artifact_id)
SELECT 'acb_'||substr(encode(drama.digest(convert_to(set_row.project_id||set_row.target_type||set_row.target_id||
    selection.artifact_id,'UTF8'),'sha256'),'hex'),1,24),set_row.project_id,set_row.target_type,set_row.target_id,
  'whole',selection.artifact_id
FROM drama.candidate_selections selection
JOIN drama.candidate_sets set_row USING(candidate_set_id)
JOIN drama.artifacts artifact ON artifact.artifact_id=selection.artifact_id
WHERE NULLIF(btrim(selection.confirmed_by),'') IS NOT NULL AND artifact.validity_status='valid' AND artifact.is_current
ON CONFLICT(project_id,target_type,target_id,component_scope) DO NOTHING;

-- Migration 24 attached candidate-only history to the now general artifact
-- current registry. Non-candidate pointers (timeline/master below) must not be
-- rejected merely because they do not have a candidate_selection row.
CREATE OR REPLACE FUNCTION drama.capture_candidate_binding_version()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE selection_id TEXT; new_binding_id TEXT;
BEGIN
  SELECT candidate_selection_id INTO selection_id
  FROM drama.candidate_selections WHERE artifact_id=NEW.current_artifact_id;
  IF selection_id IS NULL THEN RETURN NEW; END IF;
  UPDATE drama.candidate_selection_bindings
  SET is_current=false,superseded_at=CURRENT_TIMESTAMP
  WHERE project_id=NEW.project_id AND target_type=NEW.target_type
    AND target_id=NEW.target_id AND component_scope=NEW.component_scope AND is_current;
  new_binding_id := 'csb_'||substr(encode(drama.digest(convert_to(
    NEW.project_id||':'||NEW.target_type||':'||NEW.target_id||':'||
    NEW.component_scope||':'||NEW.current_artifact_id,'UTF8'),'sha256'),'hex'),1,32);
  INSERT INTO drama.candidate_selection_bindings(
    binding_id,project_id,target_type,target_id,component_scope,artifact_id,
    candidate_selection_id,is_current
  ) VALUES(new_binding_id,NEW.project_id,NEW.target_type,NEW.target_id,
    NEW.component_scope,NEW.current_artifact_id,selection_id,true)
  ON CONFLICT(binding_id) DO UPDATE SET
    is_current=true,superseded_at=NULL,bound_at=CURRENT_TIMESTAMP;
  RETURN NEW;
END $$;

ALTER TABLE drama.render_jobs
  ADD COLUMN effective_input_resolution_id TEXT,
  ADD COLUMN effective_input_hash TEXT CHECK(effective_input_hash IS NULL OR effective_input_hash ~ '^[0-9a-f]{64}$'),
  ADD COLUMN quality_gate_run_id TEXT REFERENCES drama.quality_gate_runs(gate_run_id) ON DELETE RESTRICT;

ALTER TABLE drama.professional_export_jobs DROP CONSTRAINT professional_export_jobs_status_check;
ALTER TABLE drama.professional_export_jobs
  ADD CONSTRAINT professional_export_jobs_status_check
    CHECK(status IN('queued','building','ready','failed','stale')),
  ADD COLUMN effective_input_resolution_id TEXT,
  ADD COLUMN effective_input_hash TEXT CHECK(effective_input_hash IS NULL OR effective_input_hash ~ '^[0-9a-f]{64}$'),
  ADD COLUMN gate_approval_id TEXT REFERENCES drama.quality_gate_master_approvals(gate_approval_id) ON DELETE RESTRICT,
  ADD COLUMN invalidated_at TIMESTAMPTZ,
  ADD COLUMN invalidation_reason TEXT;

CREATE OR REPLACE FUNCTION drama.timeline_content_hash(target_timeline_id TEXT)
RETURNS TEXT LANGUAGE sql STABLE AS $$
  SELECT encode(drama.digest(convert_to(jsonb_build_object(
    'timeline',to_jsonb(timeline)-'id'-'created_at'-'updated_at',
    'items',COALESCE((SELECT jsonb_agg(to_jsonb(item)-'id'-'created_at'-'updated_at'
      ORDER BY item.track_type,item.track_number,item.sequence_number)
      FROM drama.edit_timeline_items item WHERE item.timeline_id=timeline.timeline_id),'[]'::jsonb)
  )::text,'UTF8'),'sha256'),'hex')
  FROM drama.edit_timelines timeline WHERE timeline.timeline_id=target_timeline_id
$$;

CREATE OR REPLACE FUNCTION drama.delivery_effective_input_hash(resolution JSONB)
RETURNS TEXT LANGUAGE sql IMMUTABLE AS $$
  SELECT encode(drama.digest(convert_to(jsonb_build_object(
    'items',COALESCE((SELECT jsonb_agg(jsonb_build_object(
      'kind',item->>'kind','state',item->>'state','input_ids',item->'input_ids',
      'versions',item->'versions','content_hash',item->>'content_hash','artifact_ids',item->'artifact_ids')
      ORDER BY item->>'kind') FROM jsonb_array_elements(COALESCE(resolution->'items','[]'::jsonb)) item
      WHERE item->>'kind'<>'production_snapshot'),'[]'::jsonb),
    'production_payload',(COALESCE(resolution#>'{context,production_snapshot,payload}','{}'::jsonb)-'project')
  )::text,'UTF8'),'sha256'),'hex')
$$;

-- A render is released only by a QA preflight for the exact immutable timeline
-- and the exact current Effective Input Resolver result. Direct SQL writes use
-- the same guard, so the API cannot be bypassed.
CREATE OR REPLACE FUNCTION drama.guard_render_quality_gate()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  resolution JSONB;
  gate RECORD;
  live_timeline_hash TEXT;
  blocking_count INTEGER;
  pending_rebuild_count INTEGER;
BEGIN
  resolution:=drama.resolve_effective_inputs(NEW.project_id,NEW.episode_id,'post_production');
  IF COALESCE(resolution->>'status','blocked')<>'ready'
     OR NOT COALESCE((resolution->>'ready')::boolean,false) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='EFFECTIVE_INPUTS_BLOCKED: post-production Resolver must be ready before render';
  END IF;
  live_timeline_hash:=drama.timeline_content_hash(NEW.timeline_id);
  SELECT run.gate_run_id,run.model_status,run.snapshot INTO gate
  FROM drama.quality_gate_runs run
  WHERE run.project_id=NEW.project_id AND run.episode_id=NEW.episode_id
    AND run.target_timeline_id=NEW.timeline_id AND run.master_id IS NULL
    AND run.status IN('review_ready','approved')
    AND run.snapshot->>'target_timeline_hash'=live_timeline_hash
    AND run.snapshot->>'effective_input_hash'=drama.delivery_effective_input_hash(resolution)
  ORDER BY run.created_at DESC,run.gate_run_id DESC LIMIT 1;
  IF gate.gate_run_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='QUALITY_GATE_TARGET_MISMATCH: exact target timeline preflight is required';
  END IF;
  SELECT count(*) INTO blocking_count FROM drama.quality_gate_findings finding
  WHERE finding.gate_run_id=gate.gate_run_id
    AND finding.severity='blocking' AND finding.status='open';
  IF blocking_count>0 OR gate.model_status='pending' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE=format('QUALITY_GATE_BLOCKED: run %s has %s open blockers and model status %s',
        gate.gate_run_id,blocking_count,gate.model_status);
  END IF;
  SELECT count(*) INTO pending_rebuild_count
  FROM drama.incremental_rebuild_tasks task
  WHERE task.project_id=NEW.project_id AND task.status IN('pending','running')
    AND (task.target_entity_id=NEW.timeline_id OR task.artifact_id IN(
      SELECT jsonb_array_elements_text(COALESCE(gate.snapshot->'resolver_artifact_ids','[]'::jsonb))
    ));
  IF pending_rebuild_count>0 THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE=format('REBUILD_PENDING: %s required rebuild tasks are unfinished',pending_rebuild_count);
  END IF;
  NEW.effective_input_resolution_id:=resolution->>'resolution_id';
  NEW.effective_input_hash:=drama.delivery_effective_input_hash(resolution);
  NEW.quality_gate_run_id:=gate.gate_run_id;
  RETURN NEW;
END $$;

-- A professional package is a release of one resolver chain, not a bag of
-- individually valid ids. It also requires active QA approval for that exact
-- master/timeline/resolution snapshot.
CREATE OR REPLACE FUNCTION drama.guard_export_effective_chain()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  resolution JSONB;
  payload JSONB;
  selected_master RECORD;
  approval RECORD;
  expected TEXT;
BEGIN
  IF TG_OP='UPDATE' AND NEW.status NOT IN('building','ready') THEN RETURN NEW; END IF;
  resolution:=drama.resolve_effective_inputs(NEW.project_id,NEW.episode_id,'post_production');
  IF COALESCE(resolution->>'status','blocked')<>'ready'
     OR NOT COALESCE((resolution->>'ready')::boolean,false) THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='EXPORT_EFFECTIVE_INPUTS_BLOCKED: post-production Resolver must be ready';
  END IF;
  payload:=resolution#>'{context,production_snapshot,payload}';
  IF NULLIF(NEW.selection->>'master_id','') IS NULL
     OR NULLIF(NEW.selection->>'timeline_id','') IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='EXPORT_RELEASE_TARGET_REQUIRED: master_id and timeline_id are required';
  END IF;
  SELECT master.master_id,master.timeline_id,master.status,master.is_current,
    timeline.version timeline_version,timeline.is_current timeline_current,timeline.approval_state
  INTO selected_master
  FROM drama.episode_masters master JOIN drama.edit_timelines timeline USING(timeline_id)
  WHERE master.master_id=NEW.selection->>'master_id'
    AND master.project_id=NEW.project_id AND master.episode_id=NEW.episode_id;
  IF selected_master.master_id IS NULL OR selected_master.status<>'ready' OR NOT selected_master.is_current
     OR NOT selected_master.timeline_current OR selected_master.approval_state NOT IN('approved','restored')
     OR selected_master.timeline_id IS DISTINCT FROM NEW.selection->>'timeline_id' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='EXPORT_VERSION_MISMATCH: master and timeline must be the same current ready chain';
  END IF;

  SELECT item->'input_ids'->>0 INTO expected FROM jsonb_array_elements(resolution->'items') item
    WHERE item->>'kind'='narrative_ir';
  IF expected IS NOT NULL THEN
    SELECT source_version_id INTO expected FROM drama.narrative_ir_revisions WHERE ir_revision_id=expected;
  END IF;
  IF NULLIF(NEW.selection->>'source_version_id','') IS DISTINCT FROM NULLIF(expected,'') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_VERSION_MISMATCH: source is not Resolver current';
  END IF;
  SELECT item->'input_ids'->>0 INTO expected FROM jsonb_array_elements(resolution->'items') item
    WHERE item->>'kind'='narrative_ir';
  IF NULLIF(NEW.selection->>'ir_revision_id','') IS DISTINCT FROM NULLIF(expected,'') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_VERSION_MISMATCH: IR is not Resolver current';
  END IF;
  SELECT item->'input_ids'->>0 INTO expected FROM jsonb_array_elements(resolution->'items') item
    WHERE item->>'kind'='adaptation_spec';
  IF NULLIF(NEW.selection->>'adaptation_spec_version_id','') IS DISTINCT FROM NULLIF(expected,'') THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_VERSION_MISMATCH: adaptation spec is not Resolver current';
  END IF;
  IF NULLIF(NEW.selection->>'script_id','') IS NOT NULL
     AND NEW.selection->>'script_id' IS DISTINCT FROM payload#>>'{script,script_id}' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_VERSION_MISMATCH: script is not Resolver current';
  END IF;
  IF NULLIF(NEW.selection->>'storyboard_id','') IS NOT NULL
     AND NEW.selection->>'storyboard_id' IS DISTINCT FROM payload#>>'{storyboard,storyboard_id}' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_VERSION_MISMATCH: storyboard is not Resolver current';
  END IF;
  IF NEW.selection->>'timeline_id' IS DISTINCT FROM payload#>>'{timeline,timeline_id}' THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='EXPORT_VERSION_MISMATCH: timeline is not Resolver current';
  END IF;

  SELECT gate_approval_id,run.gate_run_id,run.snapshot INTO approval
  FROM drama.quality_gate_master_approvals approved
  JOIN drama.quality_gate_runs run USING(gate_run_id)
  WHERE approved.master_id=selected_master.master_id AND approved.project_id=NEW.project_id
    AND approved.episode_id=NEW.episode_id AND approved.status='active'
    AND run.status='approved' AND run.master_id=selected_master.master_id
    AND run.target_timeline_id=selected_master.timeline_id
    AND run.snapshot->>'effective_input_hash'=drama.delivery_effective_input_hash(resolution)
    AND run.snapshot->>'target_timeline_hash'=drama.timeline_content_hash(selected_master.timeline_id)
    AND NOT EXISTS(SELECT 1 FROM drama.quality_gate_findings finding
      WHERE finding.gate_run_id=run.gate_run_id AND finding.severity='blocking' AND finding.status='open')
  ORDER BY approved.approved_at DESC LIMIT 1;
  IF approval.gate_approval_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',
      MESSAGE='EXPORT_QA_BLOCKED: exact current master has no active quality approval';
  END IF;
  NEW.effective_input_resolution_id:=resolution->>'resolution_id';
  NEW.effective_input_hash:=drama.delivery_effective_input_hash(resolution);
  NEW.gate_approval_id:=approval.gate_approval_id;
  RETURN NEW;
END $$;

DROP TRIGGER trg_professional_export_effective_chain ON drama.professional_export_jobs;
CREATE TRIGGER trg_professional_export_effective_chain
BEFORE INSERT OR UPDATE OF status ON drama.professional_export_jobs
FOR EACH ROW EXECUTE FUNCTION drama.guard_export_effective_chain();

CREATE OR REPLACE FUNCTION drama.refresh_project_delivery_projection(target_project_id TEXT)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE projected_stage TEXT; projected_status TEXT;
BEGIN
  IF EXISTS(SELECT 1 FROM drama.render_jobs job WHERE job.project_id=target_project_id
      AND job.status IN('pending','claimed','processing')) THEN
    projected_stage:='rendering'; projected_status:='rendering';
  ELSIF EXISTS(SELECT 1 FROM drama.professional_export_jobs job WHERE job.project_id=target_project_id
      AND job.status='ready') THEN
    projected_stage:='stage_5_completed'; projected_status:='stage_5_completed';
  ELSIF EXISTS(SELECT 1 FROM drama.quality_gate_master_approvals approval
      WHERE approval.project_id=target_project_id AND approval.status='active') THEN
    projected_stage:='qc_completed'; projected_status:='qc_completed';
  ELSIF EXISTS(SELECT 1 FROM drama.episode_masters master JOIN drama.artifacts artifact
      ON artifact.native_entity_id=master.master_id AND artifact.project_id=master.project_id
      WHERE master.project_id=target_project_id AND master.status='ready' AND master.is_current
        AND artifact.artifact_type='episode_master' AND artifact.validity_status='valid' AND artifact.is_current) THEN
    projected_stage:='preview_rendered'; projected_status:='preview_rendered';
  ELSIF EXISTS(SELECT 1 FROM drama.edit_timelines timeline JOIN drama.artifacts artifact
      ON artifact.native_entity_id=timeline.timeline_id AND artifact.project_id=timeline.project_id
      WHERE timeline.project_id=target_project_id AND timeline.is_current
        AND timeline.approval_state IN('approved','restored')
        AND artifact.artifact_type='edit_timeline' AND artifact.validity_status='valid' AND artifact.is_current) THEN
    projected_stage:='edit_timeline_ready'; projected_status:='edit_timeline_ready';
  ELSE
    projected_stage:='waiting_media'; projected_status:='waiting_media';
  END IF;
  UPDATE drama.projects SET current_stage=projected_stage,status=projected_status,updated_at=now()
  WHERE project_id=target_project_id;
END $$;

-- Invalidation revokes only deliveries whose frozen selection/snapshot cites
-- the artifact. Old packages remain traceable but can no longer be downloaded.
CREATE OR REPLACE FUNCTION drama.invalidate_quality_and_exports_on_artifact_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.validity_status='valid' AND NEW.is_current THEN RETURN NEW; END IF;
  IF NEW.validity_status IS NOT DISTINCT FROM OLD.validity_status
     AND NEW.is_current IS NOT DISTINCT FROM OLD.is_current THEN RETURN NEW; END IF;
  UPDATE drama.quality_gate_master_approvals approval
  SET status='revoked',revoked_at=now()
  FROM drama.quality_gate_runs run
  WHERE approval.gate_run_id=run.gate_run_id AND approval.status='active'
    AND run.project_id=NEW.project_id
    AND (run.snapshot::text LIKE '%'||NEW.artifact_id||'%'
      OR run.snapshot::text LIKE '%'||NEW.native_entity_id||'%');
  UPDATE drama.quality_gate_runs run SET status='superseded'
  WHERE run.project_id=NEW.project_id AND run.status<>'superseded'
    AND (run.snapshot::text LIKE '%'||NEW.artifact_id||'%'
      OR run.snapshot::text LIKE '%'||NEW.native_entity_id||'%');
  UPDATE drama.professional_export_jobs job
  SET status='stale',invalidated_at=now(),
    invalidation_reason='artifact '||NEW.artifact_id||' is '||NEW.validity_status,
    error_message='export chain invalidated by upstream artifact change'
  WHERE job.project_id=NEW.project_id AND job.status IN('building','ready')
    AND (job.selection::text LIKE '%'||NEW.artifact_id||'%'
      OR job.selection::text LIKE '%'||NEW.native_entity_id||'%'
      OR job.effective_input_hash IN(
        SELECT run.snapshot->>'effective_input_hash' FROM drama.quality_gate_runs run
        WHERE run.project_id=NEW.project_id
          AND (run.snapshot::text LIKE '%'||NEW.artifact_id||'%'
            OR run.snapshot::text LIKE '%'||NEW.native_entity_id||'%')));
  PERFORM drama.refresh_project_delivery_projection(NEW.project_id);
  RETURN NEW;
END $$;
CREATE TRIGGER trg_artifact_invalidate_delivery
AFTER UPDATE OF validity_status,is_current ON drama.artifacts
FOR EACH ROW EXECUTE FUNCTION drama.invalidate_quality_and_exports_on_artifact_change();

-- Successful native renders atomically publish artifact successors and current
-- bindings. A failed render never executes this function and leaves old current
-- bindings usable.
CREATE OR REPLACE FUNCTION drama.publish_render_artifact_successors()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  master RECORD;
  timeline_artifact_id TEXT;
  master_artifact_id TEXT;
  timeline_hash TEXT;
  upstream_id TEXT;
  upstream_hash TEXT;
BEGIN
  IF NEW.status<>'succeeded' OR OLD.status='succeeded' THEN RETURN NEW; END IF;
  SELECT * INTO master FROM drama.episode_masters
  WHERE render_job_id=NEW.render_job_id AND project_id=NEW.project_id AND episode_id=NEW.episode_id
    AND timeline_id=NEW.timeline_id AND status='ready' AND is_current
  ORDER BY updated_at DESC LIMIT 1;
  IF master.master_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='RENDER_ARTIFACT_MISSING: ready current master is required';
  END IF;
  timeline_hash:=drama.timeline_content_hash(NEW.timeline_id);
  timeline_artifact_id:='artifact_timeline_'||substr(timeline_hash,1,24);
  master_artifact_id:='artifact_master_'||substr(master.content_hash,1,24);

  UPDATE drama.artifacts artifact SET is_current=false,
    validity_status=CASE WHEN validity_status='valid' THEN 'superseded' ELSE validity_status END,
    updated_at=now()
  WHERE artifact.project_id=NEW.project_id AND artifact.artifact_type IN('edit_timeline','episode_master')
    AND artifact.is_current AND artifact.native_entity_id NOT IN(NEW.timeline_id,master.master_id)
    AND (artifact.metadata->>'episode_id'=NEW.episode_id
      OR artifact.native_entity_id IN(SELECT timeline_id FROM drama.edit_timelines WHERE episode_id=NEW.episode_id)
      OR artifact.native_entity_id IN(SELECT master_id FROM drama.episode_masters WHERE episode_id=NEW.episode_id));

  INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
    content_hash,validity_status,is_current,idempotency_key,metadata)
  VALUES(timeline_artifact_id,'edit_timeline',NEW.project_id,NEW.timeline_id,NEW.timeline_version,
    timeline_hash,'valid',true,'render:timeline:'||NEW.timeline_id,
    jsonb_build_object('episode_id',NEW.episode_id,'render_job_id',NEW.render_job_id,
      'effective_input_resolution_id',NEW.effective_input_resolution_id,'effective_input_hash',NEW.effective_input_hash))
  ON CONFLICT(idempotency_key) DO UPDATE SET validity_status='valid',is_current=true,updated_at=now();
  INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
    content_hash,validity_status,is_current,idempotency_key,metadata)
  VALUES(master_artifact_id,'episode_master',NEW.project_id,master.master_id,master.generation_version,
    master.content_hash,'valid',true,'render:master:'||master.master_id,
    jsonb_build_object('episode_id',NEW.episode_id,'timeline_id',NEW.timeline_id,'render_job_id',NEW.render_job_id,
      'effective_input_resolution_id',NEW.effective_input_resolution_id,'effective_input_hash',NEW.effective_input_hash))
  ON CONFLICT(idempotency_key) DO UPDATE SET validity_status='valid',is_current=true,updated_at=now();

  INSERT INTO drama.artifact_current_bindings(artifact_current_binding_id,project_id,target_type,target_id,component_scope,current_artifact_id)
  VALUES('acb_'||substr(encode(drama.digest(convert_to(NEW.project_id||NEW.episode_id||'edit_timeline','UTF8'),'sha256'),'hex'),1,24),
    NEW.project_id,'episode',NEW.episode_id,'edit_timeline',timeline_artifact_id)
  ON CONFLICT(project_id,target_type,target_id,component_scope) DO UPDATE
    SET current_artifact_id=EXCLUDED.current_artifact_id,selected_at=now();
  INSERT INTO drama.artifact_current_bindings(artifact_current_binding_id,project_id,target_type,target_id,component_scope,current_artifact_id)
  VALUES('acb_'||substr(encode(drama.digest(convert_to(NEW.project_id||NEW.episode_id||'episode_master','UTF8'),'sha256'),'hex'),1,24),
    NEW.project_id,'episode',NEW.episode_id,'episode_master',master_artifact_id)
  ON CONFLICT(project_id,target_type,target_id,component_scope) DO UPDATE
    SET current_artifact_id=EXCLUDED.current_artifact_id,selected_at=now();
  INSERT INTO drama.artifact_dependencies(artifact_dependency_id,upstream_artifact_id,downstream_artifact_id,
    dependency_type,dependency_selector,observed_upstream_hash,idempotency_key)
  VALUES('ad_'||substr(encode(drama.digest(convert_to(timeline_artifact_id||master_artifact_id,'UTF8'),'sha256'),'hex'),1,24),
    timeline_artifact_id,master_artifact_id,'timeline_to_master',jsonb_build_object('episode_id',NEW.episode_id),
    timeline_hash,'render:dependency:'||timeline_artifact_id||':'||master_artifact_id)
  ON CONFLICT(idempotency_key) DO NOTHING;

  FOR upstream_id IN SELECT jsonb_array_elements_text(COALESCE((SELECT snapshot->'resolver_artifact_ids'
      FROM drama.quality_gate_runs WHERE gate_run_id=NEW.quality_gate_run_id),'[]'::jsonb)) LOOP
    SELECT content_hash INTO upstream_hash FROM drama.artifacts WHERE artifact_id=upstream_id;
    IF upstream_hash IS NULL THEN CONTINUE; END IF;
    INSERT INTO drama.artifact_dependencies(artifact_dependency_id,upstream_artifact_id,downstream_artifact_id,
      dependency_type,dependency_selector,observed_upstream_hash,idempotency_key)
    VALUES('ad_'||substr(encode(drama.digest(convert_to(upstream_id||timeline_artifact_id,'UTF8'),'sha256'),'hex'),1,24),
      upstream_id,timeline_artifact_id,'effective_input_to_timeline',
      jsonb_build_object('resolution_id',NEW.effective_input_resolution_id),upstream_hash,
      'render:input:'||upstream_id||':'||timeline_artifact_id)
    ON CONFLICT(idempotency_key) DO NOTHING;
  END LOOP;
  PERFORM drama.refresh_project_delivery_projection(NEW.project_id);
  RETURN NEW;
END $$;
CREATE TRIGGER trg_render_publish_artifacts
AFTER UPDATE OF status ON drama.render_jobs
FOR EACH ROW EXECUTE FUNCTION drama.publish_render_artifact_successors();

CREATE OR REPLACE FUNCTION drama.project_delivery_projection()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM drama.refresh_project_delivery_projection(NEW.project_id);
  RETURN NEW;
END $$;
CREATE TRIGGER trg_render_project_projection_insert
AFTER INSERT ON drama.render_jobs FOR EACH ROW EXECUTE FUNCTION drama.project_delivery_projection();
CREATE TRIGGER trg_render_project_projection_update
AFTER UPDATE OF status ON drama.render_jobs FOR EACH ROW EXECUTE FUNCTION drama.project_delivery_projection();
CREATE TRIGGER trg_qa_project_projection
AFTER INSERT OR UPDATE OF status ON drama.quality_gate_master_approvals
FOR EACH ROW EXECUTE FUNCTION drama.project_delivery_projection();
CREATE TRIGGER trg_export_project_projection
AFTER UPDATE OF status ON drama.professional_export_jobs
FOR EACH ROW EXECUTE FUNCTION drama.project_delivery_projection();

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('32','target-bound render QA, Resolver-locked export, invalidation and artifact-current closure',
  'final-delivery-chain-closure-v1-20260812');
\else
\echo 'migration 32 already applied with matching checksum; no-op'
\endif
COMMIT;
