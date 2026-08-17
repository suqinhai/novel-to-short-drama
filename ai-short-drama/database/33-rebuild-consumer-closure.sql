\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout = '5s';
SELECT pg_advisory_xact_lock(hashtext('drama:33-rebuild-consumer-closure'));
SET search_path TO drama,public;

-- Reassert the final-delivery projection on upgrades where migration 32 was
-- previously recorded before its remediation body was expanded.
CREATE OR REPLACE FUNCTION drama.refresh_project_delivery_projection(target_project_id TEXT)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE projected_stage TEXT; projected_status TEXT;
BEGIN
  IF EXISTS(SELECT 1 FROM drama.professional_export_jobs job WHERE job.project_id=target_project_id
      AND job.status='ready') THEN
    projected_stage:='stage_5_completed'; projected_status:='stage_5_completed';
  ELSIF EXISTS(SELECT 1 FROM drama.quality_gate_master_approvals approval
      JOIN drama.quality_gate_runs run USING(gate_run_id)
      WHERE run.project_id=target_project_id AND approval.status='active') THEN
    projected_stage:='qc_completed'; projected_status:='qc_completed';
  ELSIF EXISTS(SELECT 1 FROM drama.episode_masters master WHERE master.project_id=target_project_id
      AND master.status='ready' AND master.is_current) THEN
    projected_stage:='preview_rendered'; projected_status:='preview_rendered';
  ELSIF EXISTS(SELECT 1 FROM drama.render_jobs job WHERE job.project_id=target_project_id
      AND job.status IN('pending','claimed','processing')) THEN
    projected_stage:='rendering'; projected_status:='rendering';
  ELSIF EXISTS(SELECT 1 FROM drama.edit_timelines timeline WHERE timeline.project_id=target_project_id
      AND timeline.is_current) THEN
    projected_stage:='edit_timeline_ready'; projected_status:='edit_timeline_ready';
  ELSE
    projected_stage:='waiting_media'; projected_status:='waiting_media';
  END IF;
  UPDATE drama.projects SET current_stage=projected_stage,status=projected_status,updated_at=now()
  WHERE project_id=target_project_id;
END $$;

-- A rebuild publication already owns the formal current artifact for the new
-- timeline. Render publication must enrich/reuse it instead of attempting to
-- create a second current artifact for the same native timeline.
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
  -- Artifact identity tracks the versioned native entity, not its bytes. Two
  -- independently published versions may legitimately have identical output
  -- hashes and must still retain separate lineage/current-binding records.
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

-- Some upgraded installations recorded migration 32 before its final QA
-- disposition columns were present. These columns are part of the existing
-- QA contract consumed after rebuild, so make that remediation convergent.
ALTER TABLE drama.quality_gate_findings
  ADD COLUMN IF NOT EXISTS resolution_kind TEXT NOT NULL DEFAULT 'auto_detected'
    CHECK(resolution_kind IN('auto_detected','human_confirmed','resolved_by_rebuild','overridden')),
  ADD COLUMN IF NOT EXISTS human_confirmed_by TEXT,
  ADD COLUMN IF NOT EXISTS human_confirmation_reason TEXT,
  ADD COLUMN IF NOT EXISTS human_confirmed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS replacement_gate_run_id TEXT
    REFERENCES drama.quality_gate_runs(gate_run_id) ON DELETE RESTRICT;
UPDATE drama.quality_gate_findings SET resolution_kind='overridden'
WHERE status='overridden' AND resolution_kind<>'overridden';

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL
     OR NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='32') THEN
    RAISE EXCEPTION 'migration 32 must be applied before migration 33';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='33';
  IF existing_checksum IS NOT NULL
     AND existing_checksum <> 'rebuild-consumer-closure-v1-20260812' THEN
    RAISE EXCEPTION 'migration 33 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='33') AS phase33_apply \gset
\if :phase33_apply

INSERT INTO drama.artifact_types(artifact_type,description) VALUES
  ('subtitle_cue','versioned subtitle cue materialization'),
  ('continuity_ledger','versioned continuity ledger materialization')
ON CONFLICT(artifact_type) DO NOTHING;

-- Rebuild successors are general artifacts, not candidate selections. Reassert
-- the general binding behavior for installations upgraded from candidate-only
-- migration 24.
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

-- Register every current native rebuild target in the formal artifact graph.
-- Existing formal artifacts win; this only closes legacy-native gaps.
INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
  content_hash,validity_status,is_current,idempotency_key,metadata)
SELECT 'artifact_native_'||substr(encode(drama.digest(convert_to('dialogue_audio:'||audio.dialogue_audio_id,'UTF8'),'sha256'),'hex'),1,24),
  'dialogue_audio',audio.project_id,audio.dialogue_audio_id,audio.generation_version,
  CASE WHEN COALESCE(audio.content_hash,'') ~ '^[0-9a-f]{64}$' THEN audio.content_hash
    ELSE encode(drama.digest(convert_to((to_jsonb(audio)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex') END,
  'valid',true,'native-current:dialogue_audio:'||audio.dialogue_audio_id,
  jsonb_build_object('episode_id',audio.episode_id,'backfill','migration-33')
FROM drama.dialogue_audio audio WHERE audio.is_current AND NOT EXISTS(
  SELECT 1 FROM drama.artifacts artifact WHERE artifact.artifact_type='dialogue_audio'
    AND artifact.native_entity_id=audio.dialogue_audio_id)
ON CONFLICT(idempotency_key) DO NOTHING;

-- A reviewed plan participates in impact analysis only when it has a formal
-- immutable artifact identity. Legacy projects may have only the native row.
INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
  content_hash,validity_status,is_current,idempotency_key,metadata)
SELECT 'artifact_native_'||substr(encode(drama.digest(convert_to('adaptation_plan:'||plan.adaptation_plan_id,'UTF8'),'sha256'),'hex'),1,24),
  'adaptation_plan',plan.project_id,plan.adaptation_plan_id,plan.version_number,plan.content_hash,
  'valid',true,'native-current:adaptation_plan:'||plan.adaptation_plan_id,
  jsonb_build_object('backfill','migration-33')
FROM drama.adaptation_plans plan
WHERE plan.is_current AND NOT EXISTS(SELECT 1 FROM drama.artifacts artifact
  WHERE artifact.artifact_type='adaptation_plan' AND artifact.native_entity_id=plan.adaptation_plan_id)
ON CONFLICT(idempotency_key) DO NOTHING;

INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
  content_hash,validity_status,is_current,idempotency_key,metadata)
SELECT 'artifact_native_'||substr(encode(drama.digest(convert_to('subtitle_cue:'||cue.subtitle_cue_id,'UTF8'),'sha256'),'hex'),1,24),
  'subtitle_cue',cue.project_id,cue.subtitle_cue_id,cue.cue_version,
  encode(drama.digest(convert_to((to_jsonb(cue)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex'),
  'valid',true,'native-current:subtitle_cue:'||cue.subtitle_cue_id,
  jsonb_build_object('episode_id',cue.episode_id,'dialogue_id',cue.dialogue_id,'backfill','migration-33')
FROM drama.subtitle_cues cue WHERE cue.is_current AND NOT EXISTS(
  SELECT 1 FROM drama.artifacts artifact WHERE artifact.artifact_type='subtitle_cue'
    AND artifact.native_entity_id=cue.subtitle_cue_id)
ON CONFLICT(idempotency_key) DO NOTHING;

INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
  content_hash,validity_status,is_current,idempotency_key,metadata)
SELECT 'artifact_native_'||substr(encode(drama.digest(convert_to('storyboard_image:'||image.storyboard_image_id,'UTF8'),'sha256'),'hex'),1,24),
  'storyboard_image',image.project_id,image.storyboard_image_id,image.generation_version,
  encode(drama.digest(convert_to((to_jsonb(image)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex'),
  'valid',true,'native-current:storyboard_image:'||image.storyboard_image_id,
  jsonb_build_object('episode_id',image.episode_id,'shot_id',image.shot_id,'backfill','migration-33')
FROM drama.storyboard_images image WHERE image.is_current AND NOT EXISTS(
  SELECT 1 FROM drama.artifacts artifact WHERE artifact.artifact_type='storyboard_image'
    AND artifact.native_entity_id=image.storyboard_image_id)
ON CONFLICT(idempotency_key) DO NOTHING;

INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
  content_hash,validity_status,is_current,idempotency_key,metadata)
SELECT 'artifact_native_'||substr(encode(drama.digest(convert_to('shot_video:'||video.shot_video_id,'UTF8'),'sha256'),'hex'),1,24),
  'shot_video',video.project_id,video.shot_video_id,video.generation_version,
  CASE WHEN COALESCE(video.content_hash,'') ~ '^[0-9a-f]{64}$' THEN video.content_hash
    ELSE encode(drama.digest(convert_to((to_jsonb(video)-'id'-'created_at'-'updated_at')::text,'UTF8'),'sha256'),'hex') END,
  'valid',true,'native-current:shot_video:'||video.shot_video_id,
  jsonb_build_object('episode_id',video.episode_id,'shot_id',video.shot_id,'backfill','migration-33')
FROM drama.shot_videos video WHERE video.is_current AND NOT EXISTS(
  SELECT 1 FROM drama.artifacts artifact WHERE artifact.artifact_type='shot_video'
    AND artifact.native_entity_id=video.shot_video_id)
ON CONFLICT(idempotency_key) DO NOTHING;

ALTER TABLE drama.edit_timelines DROP CONSTRAINT IF EXISTS edit_timelines_edit_origin_check;
ALTER TABLE drama.edit_timelines ADD CONSTRAINT edit_timelines_edit_origin_check CHECK(edit_origin IN(
  'legacy','nle_edit','timeline_restore','template_change','sound_change','rebuild_consumer'
));
ALTER TABLE drama.edit_timelines DROP CONSTRAINT IF EXISTS edit_timelines_current_requires_approval;
ALTER TABLE drama.edit_timelines ADD CONSTRAINT edit_timelines_current_requires_approval CHECK(
  NOT is_current OR approval_state IN('approved','restored')
  OR (edit_origin='rebuild_consumer' AND approval_state IN('draft','rendering','render_failed'))
);

-- Subtitle and continuity rows were originally unique snapshots. Rebuilds need
-- immutable successors plus one current pointer, just like the media tables.
ALTER TABLE drama.subtitle_cues
  DROP CONSTRAINT IF EXISTS subtitle_cues_dialogue_audio_id_sequence_number_key;
ALTER TABLE drama.subtitle_cues
  ADD CONSTRAINT subtitle_cues_dialogue_sequence_version_key
    UNIQUE(dialogue_id,sequence_number,cue_version);

ALTER TABLE drama.continuity_ledger_entries
  DROP CONSTRAINT IF EXISTS continuity_ledger_entries_project_id_episode_id_scope_se_key;
ALTER TABLE drama.continuity_ledger_entries
  ADD COLUMN continuity_version INTEGER NOT NULL DEFAULT 1 CHECK(continuity_version>0),
  ADD COLUMN parent_continuity_entry_id TEXT
    REFERENCES drama.continuity_ledger_entries(continuity_entry_id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX continuity_ledger_scope_version_key
  ON drama.continuity_ledger_entries(project_id,episode_id,scope,sequence_number,continuity_version)
  WHERE is_current;
CREATE UNIQUE INDEX uq_continuity_ledger_current
  ON drama.continuity_ledger_entries(project_id,episode_id,scope,sequence_number)
  WHERE is_current;

INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
  content_hash,validity_status,is_current,idempotency_key,metadata)
SELECT 'artifact_native_'||substr(encode(drama.digest(convert_to('continuity:'||entry.continuity_entry_id,'UTF8'),'sha256'),'hex'),1,24),
  'continuity_ledger',entry.project_id,entry.continuity_entry_id,entry.continuity_version,entry.state_hash,
  'valid',true,'native-current:continuity:'||entry.continuity_entry_id,
  jsonb_build_object('episode_id',entry.episode_id,'scope',entry.scope,'backfill','migration-33')
FROM drama.continuity_ledger_entries entry WHERE entry.is_current AND NOT EXISTS(
  SELECT 1 FROM drama.artifacts artifact WHERE artifact.artifact_type='continuity_ledger'
    AND artifact.native_entity_id=entry.continuity_entry_id)
ON CONFLICT(idempotency_key) DO NOTHING;

INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,
  content_hash,validity_status,is_current,idempotency_key,metadata)
SELECT 'artifact_native_'||substr(encode(drama.digest(convert_to('edit_timeline:'||timeline.timeline_id,'UTF8'),'sha256'),'hex'),1,24),
  'edit_timeline',timeline.project_id,timeline.timeline_id,timeline.version,
  drama.timeline_content_hash(timeline.timeline_id),'valid',true,
  'native-current:edit_timeline:'||timeline.timeline_id,
  jsonb_build_object('episode_id',timeline.episode_id,'backfill','migration-33')
FROM drama.edit_timelines timeline WHERE timeline.is_current AND NOT EXISTS(
  SELECT 1 FROM drama.artifacts artifact WHERE artifact.artifact_type='edit_timeline'
    AND artifact.native_entity_id=timeline.timeline_id)
ON CONFLICT(idempotency_key) DO NOTHING;

-- The versioned change planner calculates exact impacts by walking declared
-- artifact dependencies. Connect a current adaptation plan only to the six
-- concrete rebuildable outputs of its already-produced episodes. Sound,
-- pacing and unrelated episodes are deliberately absent from this graph.
WITH rebuild_outputs AS (
  SELECT audio.project_id,audio.episode_id,artifact.artifact_id,artifact.artifact_type
  FROM drama.dialogue_audio audio JOIN drama.artifacts artifact
    ON artifact.native_entity_id=audio.dialogue_audio_id
  WHERE audio.is_current AND artifact.is_current AND artifact.artifact_type='dialogue_audio'
  UNION ALL
  SELECT cue.project_id,cue.episode_id,artifact.artifact_id,artifact.artifact_type
  FROM drama.subtitle_cues cue JOIN drama.artifacts artifact
    ON artifact.native_entity_id=cue.subtitle_cue_id
  WHERE cue.is_current AND artifact.is_current AND artifact.artifact_type='subtitle_cue'
  UNION ALL
  SELECT image.project_id,image.episode_id,artifact.artifact_id,artifact.artifact_type
  FROM drama.storyboard_images image JOIN drama.artifacts artifact
    ON artifact.native_entity_id=image.storyboard_image_id
  WHERE image.is_current AND artifact.is_current AND artifact.artifact_type='storyboard_image'
  UNION ALL
  SELECT video.project_id,video.episode_id,artifact.artifact_id,artifact.artifact_type
  FROM drama.shot_videos video JOIN drama.artifacts artifact
    ON artifact.native_entity_id=video.shot_video_id
  WHERE video.is_current AND artifact.is_current AND artifact.artifact_type='shot_video'
  UNION ALL
  SELECT continuity.project_id,continuity.episode_id,artifact.artifact_id,artifact.artifact_type
  FROM drama.continuity_ledger_entries continuity JOIN drama.artifacts artifact
    ON artifact.native_entity_id=continuity.continuity_entry_id
  WHERE continuity.is_current AND artifact.is_current AND artifact.artifact_type='continuity_ledger'
  UNION ALL
  SELECT timeline.project_id,timeline.episode_id,artifact.artifact_id,artifact.artifact_type
  FROM drama.edit_timelines timeline JOIN drama.artifacts artifact
    ON artifact.native_entity_id=timeline.timeline_id
  WHERE timeline.is_current AND artifact.is_current AND artifact.artifact_type='edit_timeline'
), exact_edges AS (
  SELECT DISTINCT plan_artifact.artifact_id upstream_artifact_id,
    output.artifact_id downstream_artifact_id,output.artifact_type,
    plan_artifact.content_hash observed_upstream_hash
  FROM drama.adaptation_plans plan
  JOIN drama.artifacts plan_artifact ON plan_artifact.artifact_type='adaptation_plan'
    AND plan_artifact.native_entity_id=plan.adaptation_plan_id AND plan_artifact.is_current
  JOIN drama.adaptation_episode_plans episode_plan
    ON episode_plan.adaptation_plan_id=plan.adaptation_plan_id
  JOIN drama.episode_outlines outline ON outline.project_id=plan.project_id
    AND outline.episode_number=episode_plan.episode_number
  JOIN rebuild_outputs output ON output.project_id=outline.project_id
    AND output.episode_id=outline.episode_id
  WHERE plan.is_current
)
INSERT INTO drama.artifact_dependencies(artifact_dependency_id,upstream_artifact_id,
  downstream_artifact_id,dependency_type,dependency_selector,observed_upstream_hash,
  invalidates_on,idempotency_key)
SELECT 'ad_rebuild_'||substr(encode(drama.digest(convert_to(
    edge.upstream_artifact_id||':'||edge.downstream_artifact_id,'UTF8'),'sha256'),'hex'),1,24),
  edge.upstream_artifact_id,edge.downstream_artifact_id,
  'adaptation_plan_to_'||edge.artifact_type,'{}'::jsonb,edge.observed_upstream_hash,
  '["content_changed","removed"]'::jsonb,
  'rebuild-graph:'||edge.upstream_artifact_id||':'||edge.downstream_artifact_id
FROM exact_edges edge
ON CONFLICT(idempotency_key) DO NOTHING;

ALTER TABLE drama.incremental_rebuild_tasks
  ADD COLUMN claim_token UUID,
  ADD COLUMN lease_owner TEXT,
  ADD COLUMN lease_expires_at TIMESTAMPTZ,
  ADD COLUMN heartbeat_at TIMESTAMPTZ,
  ADD COLUMN attempt_count INTEGER NOT NULL DEFAULT 0 CHECK(attempt_count>=0),
  ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 3 CHECK(max_attempts BETWEEN 1 AND 20),
  ADD COLUMN next_attempt_at TIMESTAMPTZ,
  ADD COLUMN started_at TIMESTAMPTZ,
  ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  ADD COLUMN provider_execution_id TEXT,
  ADD COLUMN successor_artifact_id TEXT REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  ADD COLUMN output_validated_at TIMESTAMPTZ,
  ADD COLUMN regeneration_request_item_id TEXT
    REFERENCES drama.regeneration_request_items(regeneration_request_item_id) ON DELETE CASCADE;

DO $$
DECLARE item RECORD;
BEGIN
  FOR item IN SELECT conname FROM pg_constraint
    WHERE conrelid='drama.incremental_rebuild_tasks'::regclass AND contype='c'
      AND (pg_get_constraintdef(oid) ILIKE '%status%'
        OR pg_get_constraintdef(oid) ILIKE '%completed_at%'
        OR conname='incremental_rebuild_tasks_one_plan_check')
  LOOP
    EXECUTE format('ALTER TABLE drama.incremental_rebuild_tasks DROP CONSTRAINT %I',item.conname);
  END LOOP;
END $$;

ALTER TABLE drama.incremental_rebuild_tasks
  ADD CONSTRAINT incremental_rebuild_tasks_status_check CHECK(status IN(
    'pending','claimed','running','retry_wait','succeeded','failed','cancelled'
  )),
  ADD CONSTRAINT incremental_rebuild_tasks_terminal_time_check CHECK(
    status NOT IN('succeeded','failed','cancelled') OR completed_at IS NOT NULL
  ),
  ADD CONSTRAINT incremental_rebuild_tasks_claim_check CHECK(
    (status IN('claimed','running') AND claim_token IS NOT NULL
      AND NULLIF(btrim(lease_owner),'') IS NOT NULL AND lease_expires_at IS NOT NULL)
    OR (status NOT IN('claimed','running'))
  ),
  ADD CONSTRAINT incremental_rebuild_tasks_success_check CHECK(
    status<>'succeeded' OR (successor_artifact_id IS NOT NULL
      AND output_validated_at IS NOT NULL AND jsonb_typeof(output)='object'
      AND output->>'schema_version'='rebuild-provider-output.v1')
  ),
  ADD CONSTRAINT incremental_rebuild_tasks_one_plan_check CHECK(
    num_nonnulls(change_plan_id,shot_edit_plan_id,regeneration_request_item_id)=1
  );

CREATE UNIQUE INDEX uq_incremental_rebuild_provider_execution
  ON drama.incremental_rebuild_tasks(provider_execution_id)
  WHERE provider_execution_id IS NOT NULL;
CREATE INDEX idx_incremental_rebuild_claimable
  ON drama.incremental_rebuild_tasks(status,next_attempt_at,lease_expires_at,created_at,rebuild_task_id)
  WHERE status IN('pending','claimed','running','retry_wait');
CREATE UNIQUE INDEX uq_incremental_rebuild_regeneration_item
  ON drama.incremental_rebuild_tasks(regeneration_request_item_id)
  WHERE regeneration_request_item_id IS NOT NULL;

CREATE TABLE drama.rebuild_task_events(
  rebuild_task_event_id TEXT PRIMARY KEY,
  rebuild_task_id TEXT NOT NULL REFERENCES drama.incremental_rebuild_tasks(rebuild_task_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL CHECK(event_type IN(
    'created','claimed','lease_recovered','running','heartbeat','provider_called',
    'output_validated','published','retry_scheduled','failed','cancelled','duplicate_callback'
  )),
  attempt INTEGER NOT NULL CHECK(attempt>=0),
  worker_id TEXT,
  claim_token UUID,
  details JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(details)='object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_rebuild_task_events_task
  ON drama.rebuild_task_events(rebuild_task_id,created_at,rebuild_task_event_id);

CREATE TABLE drama.rebuild_provider_executions(
  rebuild_provider_execution_id TEXT PRIMARY KEY,
  rebuild_task_id TEXT NOT NULL REFERENCES drama.incremental_rebuild_tasks(rebuild_task_id) ON DELETE CASCADE,
  attempt INTEGER NOT NULL CHECK(attempt>0),
  provider TEXT NOT NULL CHECK(NULLIF(btrim(provider),'') IS NOT NULL),
  action TEXT NOT NULL,
  request_hash TEXT NOT NULL CHECK(request_hash ~ '^[0-9a-f]{64}$'),
  status TEXT NOT NULL CHECK(status IN(
    'running','succeeded','failed','timed_out','invalid_output'
  )),
  output JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(output)='object'),
  error_code TEXT,
  error_message TEXT,
  started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMPTZ,
  UNIQUE(rebuild_task_id,attempt),
  CHECK(status='running' OR completed_at IS NOT NULL),
  CHECK(status NOT IN('failed','timed_out','invalid_output') OR NULLIF(btrim(error_code),'') IS NOT NULL)
);

CREATE TABLE drama.rebuild_publications(
  rebuild_publication_id TEXT PRIMARY KEY,
  rebuild_task_id TEXT NOT NULL UNIQUE
    REFERENCES drama.incremental_rebuild_tasks(rebuild_task_id) ON DELETE RESTRICT,
  predecessor_artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  successor_artifact_id TEXT NOT NULL UNIQUE REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  predecessor_native_entity_id TEXT NOT NULL,
  successor_native_entity_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  provider_execution_id TEXT NOT NULL UNIQUE,
  output_hash TEXT NOT NULL CHECK(output_hash ~ '^[0-9a-f]{64}$'),
  provenance JSONB NOT NULL CHECK(jsonb_typeof(provenance)='object'),
  published_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE OR REPLACE FUNCTION drama.record_rebuild_event(
  target_task_id TEXT,target_event TEXT,target_attempt INTEGER,target_worker TEXT,
  target_claim UUID,target_details JSONB DEFAULT '{}'::jsonb
) RETURNS VOID LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO drama.rebuild_task_events(
    rebuild_task_event_id,rebuild_task_id,event_type,attempt,worker_id,claim_token,details
  ) VALUES(
    'rte_'||substr(encode(drama.digest(convert_to(target_task_id||':'||target_event||':'||
      target_attempt::text||':'||COALESCE(target_worker,'')||':'||COALESCE(target_claim::text,'')||':'||
      clock_timestamp()::text,'UTF8'),'sha256'),'hex'),1,32),
    target_task_id,target_event,target_attempt,target_worker,target_claim,COALESCE(target_details,'{}'::jsonb)
  );
END $$;

CREATE OR REPLACE FUNCTION drama.claim_incremental_rebuild_task(
  target_worker_id TEXT,target_lease_seconds INTEGER DEFAULT 60
) RETURNS SETOF drama.incremental_rebuild_tasks LANGUAGE plpgsql AS $$
DECLARE selected RECORD; claimed drama.incremental_rebuild_tasks%ROWTYPE; recovered BOOLEAN;
BEGIN
  IF NULLIF(btrim(target_worker_id),'') IS NULL OR length(target_worker_id)>128 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='REBUILD_WORKER_ID_INVALID';
  END IF;
  IF target_lease_seconds<5 OR target_lease_seconds>3600 THEN
    RAISE EXCEPTION USING ERRCODE='22023',MESSAGE='REBUILD_LEASE_INVALID';
  END IF;

  UPDATE drama.incremental_rebuild_tasks task SET status='failed',completed_at=now(),
    error_code='REBUILD_MAX_ATTEMPTS_EXHAUSTED',
    error_message=COALESCE(NULLIF(task.error_message,''),'lease expired after maximum attempts'),
    claim_token=NULL,lease_owner=NULL,lease_expires_at=NULL,heartbeat_at=NULL,updated_at=now()
  WHERE task.status IN('claimed','running') AND task.lease_expires_at<=now()
    AND task.attempt_count>=task.max_attempts;

  SELECT task.id,task.status IN('claimed','running') AS recovered
  INTO selected
  FROM drama.incremental_rebuild_tasks task
  WHERE ((task.status IN('pending','retry_wait') AND COALESCE(task.next_attempt_at,'-infinity')<=now())
      OR (task.status IN('claimed','running') AND task.lease_expires_at<=now()))
    AND task.attempt_count<task.max_attempts
  ORDER BY CASE WHEN task.status IN('claimed','running') THEN 0 ELSE 1 END,
    CASE task.action WHEN 'regenerate_voice' THEN 1 WHEN 'update_subtitle' THEN 2
      WHEN 'regenerate_image' THEN 3 WHEN 'regenerate_video' THEN 4
      WHEN 'update_continuity' THEN 5 WHEN 'recompose_timeline' THEN 6 ELSE 99 END,
    task.created_at,task.rebuild_task_id
  FOR UPDATE SKIP LOCKED LIMIT 1;
  IF selected.id IS NULL THEN RETURN; END IF;
  recovered:=selected.recovered;

  UPDATE drama.incremental_rebuild_tasks task SET
    status='claimed',claim_token=gen_random_uuid(),lease_owner=target_worker_id,
    lease_expires_at=now()+make_interval(secs=>target_lease_seconds),heartbeat_at=now(),
    attempt_count=task.attempt_count+1,next_attempt_at=NULL,
    started_at=COALESCE(task.started_at,now()),completed_at=NULL,updated_at=now()
  WHERE task.id=selected.id RETURNING task.* INTO claimed;
  PERFORM drama.record_rebuild_event(claimed.rebuild_task_id,
    CASE WHEN recovered THEN 'lease_recovered' ELSE 'claimed' END,
    claimed.attempt_count,target_worker_id,claimed.claim_token,
    jsonb_build_object('lease_expires_at',claimed.lease_expires_at));
  RETURN NEXT claimed;
END $$;

CREATE OR REPLACE FUNCTION drama.start_incremental_rebuild_task(
  target_task_id TEXT,target_claim UUID,target_lease_seconds INTEGER DEFAULT 60
) RETURNS drama.incremental_rebuild_tasks LANGUAGE plpgsql AS $$
DECLARE result drama.incremental_rebuild_tasks%ROWTYPE;
BEGIN
  UPDATE drama.incremental_rebuild_tasks task SET status='running',heartbeat_at=now(),
    lease_expires_at=now()+make_interval(secs=>target_lease_seconds),updated_at=now()
  WHERE task.rebuild_task_id=target_task_id AND task.status='claimed'
    AND task.claim_token=target_claim AND task.lease_expires_at>now()
  RETURNING task.* INTO result;
  IF result.rebuild_task_id IS NULL THEN
    RAISE EXCEPTION USING ERRCODE='P0001',MESSAGE='REBUILD_CLAIM_LOST';
  END IF;
  PERFORM drama.record_rebuild_event(result.rebuild_task_id,'running',result.attempt_count,
    result.lease_owner,result.claim_token,'{}'::jsonb);
  RETURN result;
END $$;

CREATE OR REPLACE FUNCTION drama.heartbeat_incremental_rebuild_task(
  target_task_id TEXT,target_claim UUID,target_lease_seconds INTEGER DEFAULT 60
) RETURNS BOOLEAN LANGUAGE plpgsql AS $$
DECLARE touched INTEGER;
BEGIN
  UPDATE drama.incremental_rebuild_tasks task SET heartbeat_at=now(),
    lease_expires_at=now()+make_interval(secs=>target_lease_seconds),updated_at=now()
  WHERE task.rebuild_task_id=target_task_id AND task.status='running'
    AND task.claim_token=target_claim AND task.lease_expires_at>now();
  GET DIAGNOSTICS touched=ROW_COUNT;
  RETURN touched=1;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_rebuild_task_state()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status IN('succeeded','failed','cancelled') THEN
    IF (to_jsonb(NEW)-'updated_at') IS DISTINCT FROM (to_jsonb(OLD)-'updated_at') THEN
      RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='REBUILD_TERMINAL_IMMUTABLE';
    END IF;
    RETURN NEW;
  END IF;
  IF NEW.status IS DISTINCT FROM OLD.status AND NOT (
    (OLD.status='pending' AND NEW.status IN('claimed','cancelled')) OR
    (OLD.status='retry_wait' AND NEW.status IN('claimed','cancelled')) OR
    (OLD.status='claimed' AND NEW.status IN('running','retry_wait','failed','cancelled')) OR
    (OLD.status='running' AND NEW.status IN('claimed','retry_wait','succeeded','failed','cancelled'))
  ) THEN
    RAISE EXCEPTION USING ERRCODE='23514',
      MESSAGE=format('REBUILD_STATE_TRANSITION_INVALID: %s -> %s',OLD.status,NEW.status);
  END IF;
  IF NEW.status='succeeded' AND NOT EXISTS(
    SELECT 1 FROM drama.rebuild_publications publication
    WHERE publication.rebuild_task_id=NEW.rebuild_task_id
      AND publication.successor_artifact_id=NEW.successor_artifact_id
      AND publication.provider_execution_id=NEW.provider_execution_id
  ) THEN
    RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='REBUILD_PUBLICATION_REQUIRED';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_guard_rebuild_task_state
BEFORE UPDATE ON drama.incremental_rebuild_tasks
FOR EACH ROW EXECUTE FUNCTION drama.guard_rebuild_task_state();

-- Resolve old producers onto an explicit provider. Test projects opt in to the
-- conformance provider; production projects must configure an action route.
CREATE OR REPLACE FUNCTION drama.prepare_incremental_rebuild_task()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE project_test_mode BOOLEAN; configured_provider TEXT; resolved_artifact TEXT;
	resolved_episode TEXT;
BEGIN
  SELECT test_mode,config#>>ARRAY['rebuild_providers',NEW.action,'provider']
    INTO project_test_mode,configured_provider
  FROM drama.projects WHERE project_id=NEW.project_id;
  IF NEW.provider='workflow' OR NULLIF(btrim(NEW.provider),'') IS NULL THEN
    NEW.provider:=CASE WHEN project_test_mode THEN 'local_conformance'
      ELSE COALESCE(NULLIF(btrim(configured_provider),''),'unconfigured') END;
  END IF;

  IF NEW.artifact_id IS NULL THEN
    IF NEW.action='recompose_timeline' THEN
      SELECT artifact.artifact_id INTO resolved_artifact FROM drama.artifacts artifact
      WHERE artifact.project_id=NEW.project_id AND artifact.artifact_type='edit_timeline'
        AND artifact.native_entity_id=NEW.target_entity_id AND artifact.is_current
      ORDER BY artifact.updated_at DESC LIMIT 1;
	  IF resolved_artifact IS NULL THEN
		resolved_episode:=CASE WHEN NEW.target_entity_type='shot_sequence' THEN NEW.target_entity_id ELSE NULL END;
		IF resolved_episode IS NULL THEN
		  SELECT video.episode_id INTO resolved_episode FROM drama.shot_videos video
		  WHERE video.shot_video_id=NEW.target_entity_id OR video.shot_id=NEW.target_entity_id
		  ORDER BY video.is_current DESC,video.generation_version DESC LIMIT 1;
		END IF;
		SELECT artifact.artifact_id INTO resolved_artifact FROM drama.edit_timelines timeline
		JOIN drama.artifacts artifact ON artifact.native_entity_id=timeline.timeline_id
		WHERE timeline.project_id=NEW.project_id AND timeline.episode_id=resolved_episode
		  AND timeline.is_current AND artifact.artifact_type='edit_timeline' AND artifact.is_current
		ORDER BY timeline.version DESC,artifact.updated_at DESC LIMIT 1;
	  END IF;
    ELSIF NEW.action='regenerate_voice' THEN
      SELECT artifact.artifact_id INTO resolved_artifact
      FROM drama.artifacts artifact
      JOIN drama.dialogue_audio audio ON audio.dialogue_audio_id=artifact.native_entity_id
      WHERE artifact.project_id=NEW.project_id AND artifact.artifact_type='dialogue_audio'
        AND audio.dialogue_id=NEW.target_entity_id AND artifact.is_current AND audio.is_current
      ORDER BY artifact.updated_at DESC LIMIT 1;
    ELSIF NEW.action='update_subtitle' THEN
      SELECT artifact.artifact_id INTO resolved_artifact
      FROM drama.artifacts artifact
      JOIN drama.subtitle_cues cue ON cue.subtitle_cue_id=artifact.native_entity_id
      WHERE artifact.project_id=NEW.project_id AND artifact.artifact_type='subtitle_cue'
        AND cue.dialogue_id=NEW.target_entity_id AND artifact.is_current AND cue.is_current
      ORDER BY cue.sequence_number,artifact.updated_at DESC LIMIT 1;
    ELSIF NEW.action='regenerate_image' THEN
      SELECT artifact.artifact_id INTO resolved_artifact
      FROM drama.artifacts artifact JOIN drama.storyboard_images image
        ON image.storyboard_image_id=artifact.native_entity_id
      WHERE artifact.project_id=NEW.project_id AND artifact.artifact_type='storyboard_image'
        AND image.shot_id=NEW.target_entity_id AND artifact.is_current AND image.is_current
      ORDER BY artifact.updated_at DESC LIMIT 1;
    ELSIF NEW.action='regenerate_video' THEN
      SELECT artifact.artifact_id INTO resolved_artifact
      FROM drama.artifacts artifact JOIN drama.shot_videos video
        ON video.shot_video_id=artifact.native_entity_id
      WHERE artifact.project_id=NEW.project_id AND artifact.artifact_type='shot_video'
        AND video.shot_id=NEW.target_entity_id AND artifact.is_current AND video.is_current
      ORDER BY artifact.updated_at DESC LIMIT 1;
    ELSIF NEW.action='update_continuity' THEN
      SELECT artifact.artifact_id INTO resolved_artifact FROM drama.artifacts artifact
      JOIN drama.continuity_ledger_entries continuity ON continuity.continuity_entry_id=artifact.native_entity_id
      WHERE artifact.project_id=NEW.project_id AND artifact.artifact_type='continuity_ledger'
        AND artifact.is_current AND continuity.is_current
		AND (artifact.native_entity_id=NEW.target_entity_id
		  OR (NEW.target_entity_type='scene_continuity' AND continuity.scene_id=NEW.target_entity_id)
		  OR (NEW.target_entity_type='shot_sequence' AND continuity.episode_id=NEW.target_entity_id))
      ORDER BY (artifact.native_entity_id=NEW.target_entity_id) DESC,
		(continuity.scope='episode') DESC,continuity.sequence_number,artifact.updated_at DESC LIMIT 1;
    END IF;
    NEW.artifact_id:=resolved_artifact;
  END IF;
  IF NEW.artifact_id IS NOT NULL THEN
    SELECT artifact.content_hash INTO resolved_artifact FROM drama.artifacts artifact
    WHERE artifact.artifact_id=NEW.artifact_id;
    NEW.input:=COALESCE(NEW.input,'{}'::jsonb)||jsonb_build_object(
      'schema_version',COALESCE(NEW.input->>'schema_version','rebuild-task-input.v1'),
      'predecessor_artifact_id',NEW.artifact_id,
      'predecessor_content_hash',resolved_artifact
    );
  END IF;
  NEW.updated_at:=now();
  RETURN NEW;
END $$;
CREATE TRIGGER trg_prepare_incremental_rebuild_task
BEFORE INSERT ON drama.incremental_rebuild_tasks
FOR EACH ROW EXECUTE FUNCTION drama.prepare_incremental_rebuild_task();

UPDATE drama.incremental_rebuild_tasks task SET provider=CASE
  WHEN project.test_mode THEN 'local_conformance'
  ELSE COALESCE(NULLIF(project.config#>>ARRAY['rebuild_providers',task.action,'provider'],''),'unconfigured') END
FROM drama.projects project
WHERE project.project_id=task.project_id AND task.provider='workflow'
  AND task.status IN('pending','retry_wait');

CREATE OR REPLACE FUNCTION drama.enqueue_regeneration_rebuild_task()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE artifact_row RECORD; request_row RECORD; mapped_action TEXT; mapped_target_type TEXT;
  mapped_provider TEXT; target_version_id TEXT; task_id TEXT;
BEGIN
  SELECT artifact.*,project.test_mode,project.config INTO artifact_row
  FROM drama.artifacts artifact JOIN drama.projects project USING(project_id)
  WHERE artifact.artifact_id=NEW.artifact_id;
  SELECT request.* INTO request_row FROM drama.regeneration_requests request
  WHERE request.regeneration_request_id=NEW.regeneration_request_id;
  mapped_action:=CASE artifact_row.artifact_type
    WHEN 'dialogue_audio' THEN 'regenerate_voice'
    WHEN 'subtitle_cue' THEN 'update_subtitle'
    WHEN 'storyboard_image' THEN 'regenerate_image'
    WHEN 'shot_video' THEN 'regenerate_video'
    WHEN 'edit_timeline' THEN 'recompose_timeline'
    WHEN 'continuity_ledger' THEN 'update_continuity'
    ELSE NULL END;
  IF mapped_action IS NULL THEN RETURN NEW; END IF;
  mapped_target_type:=CASE mapped_action
    WHEN 'regenerate_voice' THEN 'dialogue'
    WHEN 'update_subtitle' THEN 'dialogue'
    WHEN 'regenerate_image' THEN 'storyboard_shot'
    WHEN 'regenerate_video' THEN 'storyboard_shot'
    WHEN 'recompose_timeline' THEN 'edit_timeline'
    WHEN 'update_continuity' THEN 'continuity_ledger_entry' END;
  mapped_provider:=CASE WHEN artifact_row.test_mode THEN 'local_conformance'
    ELSE COALESCE(NULLIF(artifact_row.config#>>ARRAY['rebuild_providers',mapped_action,'provider'],''),'unconfigured') END;
  IF mapped_action='regenerate_voice' THEN
    SELECT dialogue_id INTO artifact_row.native_entity_id FROM drama.dialogue_audio
      WHERE dialogue_audio_id=artifact_row.native_entity_id;
  ELSIF mapped_action='update_subtitle' THEN
    SELECT dialogue_id INTO artifact_row.native_entity_id FROM drama.subtitle_cues
      WHERE subtitle_cue_id=artifact_row.native_entity_id;
  ELSIF mapped_action IN('regenerate_image','regenerate_video') THEN
    IF mapped_action='regenerate_image' THEN
      SELECT shot_id INTO artifact_row.native_entity_id FROM drama.storyboard_images
        WHERE storyboard_image_id=artifact_row.native_entity_id;
    ELSE
      SELECT shot_id INTO artifact_row.native_entity_id FROM drama.shot_videos
        WHERE shot_video_id=artifact_row.native_entity_id;
    END IF;
  END IF;
  task_id:='rebuild_'||substr(encode(drama.digest(convert_to(
    NEW.regeneration_request_item_id||':'||NEW.artifact_id||':'||mapped_action,'UTF8'),'sha256'),'hex'),1,32);
  INSERT INTO drama.incremental_rebuild_tasks(
    rebuild_task_id,change_plan_id,shot_edit_plan_id,regeneration_request_item_id,project_id,
    action,target_entity_type,target_entity_id,target_entity_version_id,artifact_id,status,provider,input,output
  ) VALUES(task_id,NULL,NULL,NEW.regeneration_request_item_id,artifact_row.project_id,
    mapped_action,mapped_target_type,artifact_row.native_entity_id,target_version_id,NEW.artifact_id,
    'pending',mapped_provider,jsonb_build_object(
      'schema_version','rebuild-task-input.v1','requires_real_execution',true,
      'source_change_set_id',request_row.source_change_set_id,
      'from_source_version_id',(SELECT from_source_version_id FROM drama.source_change_sets WHERE source_change_set_id=request_row.source_change_set_id),
      'to_source_version_id',(SELECT to_source_version_id FROM drama.source_change_sets WHERE source_change_set_id=request_row.source_change_set_id),
      'predecessor_artifact_id',NEW.artifact_id,
      'predecessor_content_hash',artifact_row.content_hash,
      'requested_by',request_row.requested_by,
      'provenance',jsonb_build_object('kind','impact_regeneration','regeneration_request_id',NEW.regeneration_request_id)
    ),'{}'::jsonb)
  ON CONFLICT(rebuild_task_id) DO NOTHING;
  PERFORM drama.record_rebuild_event(task_id,'created',0,NULL,NULL,
    jsonb_build_object('regeneration_request_item_id',NEW.regeneration_request_item_id));
  RETURN NEW;
END $$;
CREATE TRIGGER trg_enqueue_regeneration_rebuild_task
AFTER INSERT ON drama.regeneration_request_items
FOR EACH ROW EXECUTE FUNCTION drama.enqueue_regeneration_rebuild_task();

INSERT INTO drama.schema_migrations(version,description,checksum)
VALUES('33','leased generic rebuild consumer, strict provider output and atomic successor publication',
  'rebuild-consumer-closure-v1-20260812');
\else
\echo 'migration 33 already applied with matching checksum; no-op'
\endif
COMMIT;
\set ON_ERROR_STOP on
