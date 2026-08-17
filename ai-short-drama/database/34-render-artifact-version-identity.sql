\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:34-render-artifact-version-identity'));
SET search_path TO drama,public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='33') THEN
    RAISE EXCEPTION 'migration 33 must be applied before migration 34';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='34';
  IF existing_checksum IS NOT NULL
     AND existing_checksum NOT IN(
       'render-artifact-version-identity-v1-20260817',
       'render-artifact-version-identity-v2-20260817',
       'render-artifact-version-identity-v3-20260817'
     ) THEN
    RAISE EXCEPTION 'migration 34 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

-- Reassert this function on every migration replay. Migration 33 also
-- reasserts it for convergent legacy upgrades, so 34 must always be the final
-- authority even after an already-migrated database replays the full chain.
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
  SELECT artifact_id,content_hash INTO timeline_artifact_id,timeline_hash FROM drama.artifacts
  WHERE project_id=NEW.project_id AND artifact_type='edit_timeline' AND native_entity_id=NEW.timeline_id
  ORDER BY is_current DESC,revision_number DESC,created_at DESC LIMIT 1;
  IF timeline_artifact_id IS NULL THEN timeline_hash:=drama.timeline_content_hash(NEW.timeline_id); END IF;
  SELECT artifact_id INTO master_artifact_id FROM drama.artifacts
  WHERE project_id=NEW.project_id AND artifact_type='episode_master' AND native_entity_id=master.master_id
  ORDER BY is_current DESC,revision_number DESC,created_at DESC LIMIT 1;

  -- A content hash describes bytes, while artifact_id describes one versioned
  -- entity. Equal render bytes across generations therefore share
  -- content_hash but intentionally receive different artifact IDs.
  timeline_artifact_id:=COALESCE(timeline_artifact_id,'artifact_timeline_'||substr(encode(drama.digest(
    convert_to('edit_timeline:'||NEW.timeline_id,'UTF8'),'sha256'),'hex'),1,32));
  master_artifact_id:=COALESCE(master_artifact_id,'artifact_master_'||substr(encode(drama.digest(
    convert_to('episode_master:'||master.master_id,'UTF8'),'sha256'),'hex'),1,32));

  UPDATE drama.artifacts artifact SET is_current=false,
    validity_status=CASE WHEN validity_status='valid' THEN 'superseded' ELSE validity_status END,
    updated_at=now()
  WHERE artifact.project_id=NEW.project_id AND artifact.artifact_type IN('edit_timeline','episode_master')
    AND artifact.is_current AND artifact.artifact_id NOT IN(timeline_artifact_id,master_artifact_id)
    AND (artifact.metadata->>'episode_id'=NEW.episode_id
      OR artifact.native_entity_id IN(SELECT timeline_id FROM drama.edit_timelines WHERE episode_id=NEW.episode_id)
      OR artifact.native_entity_id IN(SELECT master_id FROM drama.episode_masters WHERE episode_id=NEW.episode_id));

  INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
    content_hash,validity_status,is_current,idempotency_key,metadata)
  VALUES(timeline_artifact_id,'edit_timeline',NEW.project_id,NEW.timeline_id,NEW.timeline_version,
    timeline_hash,'valid',true,'render:timeline:'||NEW.timeline_id,
    jsonb_build_object('episode_id',NEW.episode_id,'render_job_id',NEW.render_job_id,
      'effective_input_resolution_id',NEW.effective_input_resolution_id,'effective_input_hash',NEW.effective_input_hash))
  ON CONFLICT(artifact_id) DO UPDATE SET validity_status='valid',is_current=true,
    metadata=drama.artifacts.metadata||EXCLUDED.metadata,updated_at=now();
  INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
    content_hash,validity_status,is_current,idempotency_key,metadata)
  VALUES(master_artifact_id,'episode_master',NEW.project_id,master.master_id,master.generation_version,
    master.content_hash,'valid',true,'render:master:'||master.master_id,
    jsonb_build_object('episode_id',NEW.episode_id,'timeline_id',NEW.timeline_id,'render_job_id',NEW.render_job_id,
      'effective_input_resolution_id',NEW.effective_input_resolution_id,'effective_input_hash',NEW.effective_input_hash))
  ON CONFLICT(idempotency_key) DO UPDATE SET validity_status='valid',is_current=true,
    metadata=drama.artifacts.metadata||EXCLUDED.metadata,updated_at=now();

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
  ON CONFLICT DO NOTHING;

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
    ON CONFLICT DO NOTHING;
  END LOOP;
  PERFORM drama.refresh_project_delivery_projection(NEW.project_id);
  RETURN NEW;
END $$;

-- A current rebuild timeline remains the Resolver authority when its render
-- attempt fails. Keeping it current allows the audited render_failed retry
-- path to create a new render job; demoting it leaves the project with no
-- Resolver timeline and makes the documented retry path unreachable.
CREATE OR REPLACE FUNCTION drama.promote_timeline_after_render()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_episode TEXT;
BEGIN
  IF NEW.status IS NOT DISTINCT FROM OLD.status THEN RETURN NEW; END IF;

  SELECT episode_id INTO target_episode FROM drama.edit_timelines
  WHERE timeline_id=NEW.timeline_id FOR UPDATE;
  IF target_episode IS NULL THEN RETURN NEW; END IF;

  IF NEW.status='succeeded' THEN
    UPDATE drama.edit_timelines
       SET is_current=false,
           approval_state=CASE WHEN approval_state IN ('approved','restored') THEN 'superseded' ELSE approval_state END,
           updated_at=CURRENT_TIMESTAMP
     WHERE episode_id=target_episode AND is_current AND timeline_id<>NEW.timeline_id;

    UPDATE drama.edit_timelines
       SET is_current=true,approval_state='approved',status='completed',
           approved_render_job_id=NEW.render_job_id,approved_at=CURRENT_TIMESTAMP,
           updated_at=CURRENT_TIMESTAMP
     WHERE timeline_id=NEW.timeline_id AND approval_state='rendering';
  ELSIF NEW.status IN ('failed','timeout','cancelled') THEN
    UPDATE drama.edit_timelines
       SET approval_state='render_failed',status='failed',updated_at=CURRENT_TIMESTAMP
     WHERE timeline_id=NEW.timeline_id AND approval_state='rendering';
  END IF;
  RETURN NEW;
END $$;

-- Repair only the state produced by the old failure branch: the authoritative
-- artifact binding still names the render_failed timeline and the episode has
-- no native current timeline. No successful/current sibling is overwritten.
UPDATE drama.edit_timelines timeline
SET is_current=true,updated_at=CURRENT_TIMESTAMP
WHERE timeline.approval_state='render_failed'
  AND NOT EXISTS(SELECT 1 FROM drama.edit_timelines current_timeline
    WHERE current_timeline.episode_id=timeline.episode_id AND current_timeline.is_current)
  AND EXISTS(SELECT 1 FROM drama.artifact_current_bindings binding
    JOIN drama.artifacts artifact ON artifact.artifact_id=binding.current_artifact_id
    WHERE binding.project_id=timeline.project_id AND binding.target_type='episode'
      AND binding.target_id=timeline.episode_id AND binding.component_scope='edit_timeline'
      AND artifact.artifact_type='edit_timeline' AND artifact.native_entity_id=timeline.timeline_id
      AND artifact.is_current AND artifact.validity_status='valid');

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='34') AS phase34_apply \gset
\if :phase34_apply
INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('34','version-identity render artifacts with idempotent publication and retryable render failures',
  'render-artifact-version-identity-v3-20260817');
\else
UPDATE drama.schema_migrations
SET description='version-identity render artifacts with idempotent publication and retryable render failures',
  checksum='render-artifact-version-identity-v3-20260817'
WHERE version='34' AND checksum IN(
  'render-artifact-version-identity-v1-20260817',
  'render-artifact-version-identity-v2-20260817'
);
\echo 'migration 34 already applied with compatible checksum; function reasserted'
\endif
COMMIT;
\set ON_ERROR_STOP on
