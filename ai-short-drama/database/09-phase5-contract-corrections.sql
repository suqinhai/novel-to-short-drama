BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(hashtext('drama:09-phase5-contract-corrections'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='09';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'phase5-contract-corrections-v4-20260721' THEN
    RAISE EXCEPTION 'migration 09 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='09') AS phase5_apply \gset

\if :phase5_apply

-- Phase 4 added three identity fields after the original IR seal was written.
-- Replace only the guard function; the existing trigger remains attached.
CREATE OR REPLACE FUNCTION drama.guard_published_ir_revision()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' AND OLD.published_at IS NOT NULL THEN
    RAISE EXCEPTION 'published IR revision % is immutable',OLD.ir_revision_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.published_at IS NOT NULL AND (
    NEW.operation_id IS DISTINCT FROM OLD.operation_id OR
    NEW.work_id IS DISTINCT FROM OLD.work_id OR
    NEW.source_version_id IS DISTINCT FROM OLD.source_version_id OR
    NEW.revision_number IS DISTINCT FROM OLD.revision_number OR
    NEW.schema_version IS DISTINCT FROM OLD.schema_version OR
    NEW.extractor_version IS DISTINCT FROM OLD.extractor_version OR
    NEW.input_hash IS DISTINCT FROM OLD.input_hash OR
    NEW.output_hash IS DISTINCT FROM OLD.output_hash OR
    NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key OR
    NEW.validation_summary IS DISTINCT FROM OLD.validation_summary OR
    NEW.revision_scope IS DISTINCT FROM OLD.revision_scope OR
    NEW.base_ir_revision_id IS DISTINCT FROM OLD.base_ir_revision_id OR
    NEW.changed_chapter_ids IS DISTINCT FROM OLD.changed_chapter_ids OR
    NEW.published_at IS DISTINCT FROM OLD.published_at
  ) THEN
    RAISE EXCEPTION 'published IR revision % content is immutable',OLD.ir_revision_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.published_at IS NOT NULL
     AND NEW.status IS DISTINCT FROM OLD.status
     AND NOT (OLD.status='published' AND NEW.status='superseded') THEN
    RAISE EXCEPTION 'invalid sealed IR state transition % -> %',OLD.status,NEW.status;
  END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

-- Published IR must always pass through staging/validation.  This also makes
-- the impact enqueue trigger impossible to bypass with a direct INSERT.
CREATE OR REPLACE FUNCTION drama.guard_incremental_ir_publish()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE base_source_version TEXT;
BEGIN
  IF TG_OP='INSERT' AND NEW.status='published' THEN
    RAISE EXCEPTION 'IR revisions must be inserted as staging and published by transition';
  END IF;
  IF NEW.revision_scope='incremental' THEN
    SELECT source_version_id INTO base_source_version
    FROM drama.narrative_ir_revisions
    WHERE ir_revision_id=NEW.base_ir_revision_id AND status='published' AND revision_scope='full';
    IF base_source_version IS NULL OR NOT EXISTS(
      SELECT 1 FROM drama.source_versions
      WHERE source_version_id=NEW.source_version_id AND parent_source_version_id=base_source_version
    ) THEN
      RAISE EXCEPTION 'incremental IR requires the published full IR of its parent source version';
    END IF;
    IF EXISTS(
      SELECT chapter_id FROM jsonb_array_elements_text(NEW.changed_chapter_ids) chapter_id
      GROUP BY chapter_id HAVING count(*)>1
    ) OR EXISTS(
      SELECT 1 FROM jsonb_array_elements_text(NEW.changed_chapter_ids) requested(chapter_id)
      LEFT JOIN drama.source_version_chapters current_chapter
        ON current_chapter.source_version_id=NEW.source_version_id AND current_chapter.chapter_id=requested.chapter_id
      LEFT JOIN drama.source_version_chapters parent_chapter
        ON parent_chapter.source_version_id=base_source_version AND parent_chapter.chapter_id=requested.chapter_id
      WHERE (current_chapter.chapter_id IS NULL AND parent_chapter.chapter_id IS NULL)
         OR current_chapter.chapter_revision_id IS NOT DISTINCT FROM parent_chapter.chapter_revision_id
    ) THEN
      RAISE EXCEPTION 'incremental IR chapters must be unique changed, added or removed chapters';
    END IF;
    IF NEW.status='published' THEN NEW.is_current:=false; END IF;
  END IF;
  RETURN NEW;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_ir_incremental_published_source
  ON drama.narrative_ir_revisions(source_version_id)
  WHERE revision_scope='incremental' AND status='published';
ALTER TABLE drama.narrative_ir_revisions ENABLE TRIGGER trg_incremental_ir_publish_guard;
ALTER TABLE drama.narrative_ir_revisions ENABLE TRIGGER trg_enqueue_incremental_impact;

-- A source span is evidence, not an approximate locator. Validate new writes
-- against the immutable chapter revision without rewriting legacy rows.
CREATE OR REPLACE FUNCTION drama.validate_source_span_bounds()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE chapter_content TEXT; actual_excerpt TEXT; actual_start_byte INTEGER; actual_end_byte INTEGER;
BEGIN
  SELECT revision.content INTO chapter_content
  FROM drama.chapter_revisions revision
  JOIN drama.source_version_chapters membership
    ON membership.work_id=NEW.work_id AND membership.source_version_id=NEW.source_version_id
      AND membership.chapter_id=NEW.chapter_id AND membership.chapter_revision_id=NEW.chapter_revision_id
  WHERE revision.work_id=NEW.work_id AND revision.chapter_id=NEW.chapter_id
    AND revision.chapter_revision_id=NEW.chapter_revision_id;
  IF chapter_content IS NULL THEN RAISE EXCEPTION 'source span chapter revision is not in the source version'; END IF;
  IF NEW.end_codepoint>char_length(chapter_content) OR NEW.end_utf8_byte>octet_length(chapter_content) THEN
    RAISE EXCEPTION 'source span is outside chapter content';
  END IF;
  actual_excerpt:=substring(chapter_content FROM NEW.start_codepoint+1 FOR NEW.end_codepoint-NEW.start_codepoint);
  actual_start_byte:=octet_length(substring(chapter_content FROM 1 FOR NEW.start_codepoint));
  actual_end_byte:=actual_start_byte+octet_length(actual_excerpt);
  IF actual_start_byte<>NEW.start_utf8_byte OR actual_end_byte<>NEW.end_utf8_byte OR
     encode(digest(convert_to(actual_excerpt,'UTF8'),'sha256'),'hex')<>NEW.excerpt_hash OR
     (NEW.evidence_text IS NOT NULL AND NEW.evidence_text<>actual_excerpt) THEN
    RAISE EXCEPTION 'source span byte/codepoint/hash/evidence mismatch';
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_source_span_bounds ON drama.source_spans;
CREATE TRIGGER trg_source_span_bounds BEFORE INSERT OR UPDATE ON drama.source_spans
  FOR EACH ROW EXECUTE FUNCTION drama.validate_source_span_bounds();

-- Provider envelopes and request/response bodies are never part of canonical
-- source or IR documents. NOT VALID preserves compatible legacy rows while
-- enforcing the rule for every new write.
DO $$
DECLARE item RECORD; constraint_name TEXT;
BEGIN
  FOR item IN SELECT * FROM (VALUES
    ('source_works','metadata'),('source_versions','metadata'),
    ('narrative_ir_revisions','validation_summary'),('narrative_entity_revisions','attributes'),
    ('narrative_fact_revisions','payload'),('event_participants','participation_state'),
    ('character_state_changes','before_state'),('character_state_changes','after_state'),
    ('timeline_facts','normalized_time')
  ) AS fields(table_name,column_name)
  LOOP
    constraint_name:='no_provider_payload_'||item.table_name||'_'||item.column_name;
    IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass('drama.'||item.table_name) AND conname=constraint_name) THEN
      EXECUTE format('ALTER TABLE drama.%I ADD CONSTRAINT %I CHECK (NOT drama.jsonb_has_forbidden_provider_payload(%I)) NOT VALID',
        item.table_name,constraint_name,item.column_name);
    END IF;
  END LOOP;
END $$;

-- Once a compiler plan is exposed for review, its normalized episode/event
-- audit rows are immutable. Review may change only status/current selection.
CREATE OR REPLACE FUNCTION drama.guard_reviewable_plan()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' AND NOT EXISTS(SELECT 1 FROM drama.projects WHERE project_id=OLD.project_id) THEN RETURN OLD; END IF;
  IF TG_OP='DELETE' AND OLD.status IN ('waiting_review','approved','rejected') THEN
    RAISE EXCEPTION 'reviewable adaptation plan % is immutable',OLD.adaptation_plan_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.status IN ('waiting_review','approved','rejected') AND (
    NEW.compiler_run_id IS DISTINCT FROM OLD.compiler_run_id OR NEW.project_id IS DISTINCT FROM OLD.project_id OR
    NEW.adaptation_spec_version_id IS DISTINCT FROM OLD.adaptation_spec_version_id OR
    NEW.version_number IS DISTINCT FROM OLD.version_number OR NEW.content_hash IS DISTINCT FROM OLD.content_hash OR
    NEW.quality_report IS DISTINCT FROM OLD.quality_report
  ) THEN RAISE EXCEPTION 'reviewable adaptation plan % content is immutable',OLD.adaptation_plan_id; END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_reviewable_plan_child()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE episode_id TEXT; plan_status TEXT; owner_project_id TEXT;
BEGIN
  episode_id:=CASE WHEN TG_TABLE_NAME='adaptation_episode_plans'
    THEN (CASE WHEN TG_OP='DELETE' THEN OLD.adaptation_episode_plan_id ELSE NEW.adaptation_episode_plan_id END)
    ELSE (CASE WHEN TG_OP='DELETE' THEN OLD.adaptation_episode_plan_id ELSE NEW.adaptation_episode_plan_id END) END;
  SELECT plan.status,plan.project_id INTO plan_status,owner_project_id FROM drama.adaptation_episode_plans episode
    JOIN drama.adaptation_plans plan ON plan.adaptation_plan_id=episode.adaptation_plan_id
    WHERE episode.adaptation_episode_plan_id=episode_id;
  IF owner_project_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM drama.projects WHERE project_id=owner_project_id) THEN
    RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
  END IF;
  IF plan_status IN ('waiting_review','approved','rejected') THEN
    RAISE EXCEPTION 'reviewable plan child % is immutable',episode_id;
  END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE OR REPLACE FUNCTION drama.validate_episode_audit_snapshot()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE episode_id TEXT; saved_events JSONB; assigned_events JSONB; saved_chapters JSONB;
BEGIN
  episode_id:=CASE WHEN TG_TABLE_NAME='adaptation_episode_plans'
    THEN (CASE WHEN TG_OP='DELETE' THEN OLD.adaptation_episode_plan_id ELSE NEW.adaptation_episode_plan_id END)
    ELSE (CASE WHEN TG_OP='DELETE' THEN OLD.adaptation_episode_plan_id ELSE NEW.adaptation_episode_plan_id END) END;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  SELECT episode.source_event_ids,episode.source_chapter_ids INTO saved_events,saved_chapters
    FROM drama.adaptation_episode_plans episode WHERE episode.adaptation_episode_plan_id=episode_id;
  IF saved_events IS NULL THEN RETURN NEW; END IF;
  -- Additive compatibility: pre-contract writers omitted both audit arrays and
  -- rely on the normalized assignment rows. New compiler writes non-empty
  -- snapshots and is checked strictly below.
  IF saved_events='[]'::jsonb AND saved_chapters='[]'::jsonb THEN RETURN NEW; END IF;
  SELECT COALESCE(jsonb_agg(assignment.event_revision_id ORDER BY assignment.sequence_number),'[]'::jsonb)
    INTO assigned_events FROM drama.episode_event_assignments assignment WHERE assignment.adaptation_episode_plan_id=episode_id;
  IF saved_events<>assigned_events THEN RAISE EXCEPTION 'episode source_event_ids do not match normalized assignments'; END IF;
  IF EXISTS(SELECT 1 FROM jsonb_array_elements_text(saved_chapters) saved(chapter_id)
      WHERE NOT EXISTS(SELECT 1 FROM drama.episode_event_assignments assignment
        JOIN drama.narrative_event_revisions event USING(event_revision_id)
        JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
        WHERE assignment.adaptation_episode_plan_id=episode_id AND fact.chapter_id=saved.chapter_id)) OR
     EXISTS(SELECT 1 FROM drama.episode_event_assignments assignment
        JOIN drama.narrative_event_revisions event USING(event_revision_id)
        JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
        WHERE assignment.adaptation_episode_plan_id=episode_id AND NOT saved_chapters ? fact.chapter_id) THEN
    RAISE EXCEPTION 'episode source_chapter_ids do not match assigned event provenance';
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_reviewable_plan_immutable ON drama.adaptation_plans;
CREATE TRIGGER trg_reviewable_plan_immutable BEFORE UPDATE OR DELETE ON drama.adaptation_plans
  FOR EACH ROW EXECUTE FUNCTION drama.guard_reviewable_plan();
DROP TRIGGER IF EXISTS trg_reviewable_episode_immutable ON drama.adaptation_episode_plans;
CREATE TRIGGER trg_reviewable_episode_immutable BEFORE UPDATE OR DELETE ON drama.adaptation_episode_plans
  FOR EACH ROW EXECUTE FUNCTION drama.guard_reviewable_plan_child();
DROP TRIGGER IF EXISTS trg_reviewable_assignment_immutable ON drama.episode_event_assignments;
CREATE TRIGGER trg_reviewable_assignment_immutable BEFORE UPDATE OR DELETE ON drama.episode_event_assignments
  FOR EACH ROW EXECUTE FUNCTION drama.guard_reviewable_plan_child();
DROP TRIGGER IF EXISTS trg_episode_audit_snapshot_episode ON drama.adaptation_episode_plans;
CREATE CONSTRAINT TRIGGER trg_episode_audit_snapshot_episode AFTER INSERT OR UPDATE ON drama.adaptation_episode_plans
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION drama.validate_episode_audit_snapshot();
DROP TRIGGER IF EXISTS trg_episode_audit_snapshot_assignment ON drama.episode_event_assignments;
CREATE CONSTRAINT TRIGGER trg_episode_audit_snapshot_assignment AFTER INSERT OR UPDATE OR DELETE ON drama.episode_event_assignments
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION drama.validate_episode_audit_snapshot();

-- Enforce the work/version identity of both sides of an impact comparison.
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='drama.source_change_sets'::regclass AND conname='source_change_sets_from_ir_source_fk') THEN
    ALTER TABLE drama.source_change_sets ADD CONSTRAINT source_change_sets_from_ir_source_fk
      FOREIGN KEY(from_ir_revision_id,work_id,from_source_version_id)
      REFERENCES drama.narrative_ir_revisions(ir_revision_id,work_id,source_version_id)
      ON DELETE RESTRICT NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='drama.source_change_sets'::regclass AND conname='source_change_sets_to_ir_source_fk') THEN
    ALTER TABLE drama.source_change_sets ADD CONSTRAINT source_change_sets_to_ir_source_fk
      FOREIGN KEY(to_ir_revision_id,work_id,to_source_version_id)
      REFERENCES drama.narrative_ir_revisions(ir_revision_id,work_id,source_version_id)
      ON DELETE RESTRICT NOT VALID;
  END IF;
END $$;
ALTER TABLE drama.source_change_sets VALIDATE CONSTRAINT source_change_sets_from_ir_source_fk;
ALTER TABLE drama.source_change_sets VALIDATE CONSTRAINT source_change_sets_to_ir_source_fk;

CREATE UNIQUE INDEX IF NOT EXISTS uq_invalidation_task_project_change
  ON drama.invalidation_tasks(project_id,source_change_set_id)
  WHERE project_id IS NOT NULL AND source_change_set_id IS NOT NULL;

CREATE OR REPLACE FUNCTION drama.validate_regeneration_request_scope()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS(
    SELECT 1 FROM drama.invalidation_tasks task
    WHERE task.project_id=NEW.project_id AND task.source_change_set_id=NEW.source_change_set_id
  ) THEN
    RAISE EXCEPTION 'source change set is not assigned to project %',NEW.project_id;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.validate_regeneration_item_scope()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS(
    SELECT 1 FROM drama.regeneration_requests request
    JOIN drama.invalidation_tasks task
      ON task.project_id=request.project_id AND task.source_change_set_id=request.source_change_set_id
    JOIN drama.invalidation_impacts impact
      ON impact.invalidation_task_id=task.invalidation_task_id AND impact.artifact_id=NEW.artifact_id
    JOIN drama.artifacts artifact ON artifact.artifact_id=NEW.artifact_id
    WHERE request.regeneration_request_id=NEW.regeneration_request_id
      AND artifact.project_id=request.project_id
  ) THEN
    RAISE EXCEPTION 'artifact is outside the requested project impact';
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_regeneration_request_scope ON drama.regeneration_requests;
CREATE TRIGGER trg_regeneration_request_scope
  BEFORE INSERT OR UPDATE OF project_id,source_change_set_id ON drama.regeneration_requests
  FOR EACH ROW EXECUTE FUNCTION drama.validate_regeneration_request_scope();
DROP TRIGGER IF EXISTS trg_regeneration_item_scope ON drama.regeneration_request_items;
CREATE TRIGGER trg_regeneration_item_scope
  BEFORE INSERT OR UPDATE OF regeneration_request_id,artifact_id ON drama.regeneration_request_items
  FOR EACH ROW EXECUTE FUNCTION drama.validate_regeneration_item_scope();

-- Explicit, bounded recovery for expired canonical operations.  A scheduler
-- may call this repeatedly; it never steals a live lease.
CREATE OR REPLACE FUNCTION drama.recover_expired_operations(p_limit INTEGER DEFAULT 100)
RETURNS TABLE(operation_id TEXT,status TEXT,retry_count INTEGER) LANGUAGE plpgsql AS $$
BEGIN
  IF p_limit<1 OR p_limit>1000 THEN RAISE EXCEPTION 'recovery limit must be 1..1000'; END IF;
  RETURN QUERY
  WITH expired AS (
    SELECT operation.id FROM drama.operations operation
    WHERE operation.status IN ('running','validating') AND operation.lease_expires_at<CURRENT_TIMESTAMP
    ORDER BY operation.lease_expires_at FOR UPDATE SKIP LOCKED LIMIT p_limit
  )
  UPDATE drama.operations operation SET
    status=CASE WHEN operation.retry_count<operation.max_retries THEN 'pending' ELSE 'failed' END,
    retry_count=operation.retry_count+1,
    claim_token=NULL,claim_request_id=NULL,lease_owner=NULL,lease_expires_at=NULL,
    error_code=CASE WHEN operation.retry_count<operation.max_retries THEN NULL ELSE 'RETRY_EXHAUSTED' END,
    error_message=CASE WHEN operation.retry_count<operation.max_retries THEN NULL ELSE 'operation lease expired and retry budget was exhausted' END,
    error_retryable=CASE WHEN operation.retry_count<operation.max_retries THEN NULL ELSE false END,
    completed_at=CASE WHEN operation.retry_count<operation.max_retries THEN NULL ELSE CURRENT_TIMESTAMP END,
    checkpoint_stage=CASE WHEN operation.retry_count<operation.max_retries THEN 'retry_queued' ELSE 'finished' END
  FROM expired WHERE operation.id=expired.id
  RETURNING operation.operation_id,operation.status,operation.retry_count;
END $$;

-- Keep the old whole-book workflow compatible after Phase 1.  New legacy
-- imports are mirrored while draft, then sealed when the legacy task commits.
CREATE OR REPLACE FUNCTION drama.prepare_legacy_source()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE source_role TEXT;
BEGIN
  INSERT INTO drama.source_works(work_id,title,status,idempotency_key,metadata,created_at,updated_at)
  VALUES('sw_legacy_'||NEW.novel_id,NEW.name,'active','legacy-live:work:'||NEW.novel_id,
    jsonb_build_object('legacy_novel_id',NEW.novel_id,'bridge_version','phase5-v1'),NEW.created_at,NEW.updated_at)
  ON CONFLICT(work_id) DO NOTHING;
  INSERT INTO drama.source_versions(source_version_id,work_id,version_number,status,is_current,version_hash,
    normalization_version,total_chars,chapter_count,idempotency_key,metadata,created_at,updated_at)
  VALUES('sv_legacy_'||NEW.novel_id,'sw_legacy_'||NEW.novel_id,1,'draft',false,
    encode(digest(convert_to(NEW.novel_id||':'||NEW.content_hash,'UTF8'),'sha256'),'hex'),'legacy-clean-v1',
    0,0,'legacy-live:version:'||NEW.novel_id,jsonb_build_object('legacy_novel_id',NEW.novel_id),NEW.created_at,NEW.updated_at)
  ON CONFLICT(source_version_id) DO NOTHING;
  source_role:=CASE WHEN EXISTS(SELECT 1 FROM drama.project_source_bindings
    WHERE project_id=NEW.project_id AND binding_role='primary' AND is_current) THEN 'supplemental' ELSE 'primary' END;
  INSERT INTO drama.project_source_bindings(binding_id,project_id,work_id,source_version_id,binding_role,is_current,idempotency_key)
  VALUES('psb_legacy_'||NEW.novel_id,NEW.project_id,'sw_legacy_'||NEW.novel_id,'sv_legacy_'||NEW.novel_id,
    source_role,true,'legacy-live:binding:'||NEW.novel_id) ON CONFLICT(binding_id) DO NOTHING;
  INSERT INTO drama.legacy_source_bindings(legacy_binding_id,legacy_novel_id,project_id,work_id,source_version_id,migration_batch_id)
  VALUES('lsb_'||NEW.novel_id,NEW.novel_id,NEW.project_id,'sw_legacy_'||NEW.novel_id,'sv_legacy_'||NEW.novel_id,'phase5-live-v1')
  ON CONFLICT(legacy_novel_id) DO NOTHING;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.mirror_legacy_chapter()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE chapter_hash TEXT;
BEGIN
  chapter_hash:=CASE WHEN lower(NEW.content_hash)~'^[0-9a-f]{64}$' THEN lower(NEW.content_hash)
    ELSE encode(digest(convert_to(NEW.content,'UTF8'),'sha256'),'hex') END;
  INSERT INTO drama.source_chapters(chapter_id,work_id,canonical_key,status,created_at,updated_at)
  VALUES('sch_legacy_'||NEW.chapter_id,'sw_legacy_'||NEW.novel_id,'legacy:'||NEW.chapter_id,'active',NEW.created_at,NEW.created_at)
  ON CONFLICT(chapter_id) DO NOTHING;
  INSERT INTO drama.chapter_revisions(chapter_revision_id,work_id,chapter_id,revision_number,title,content,content_hash,
    char_count,idempotency_key,created_at,updated_at)
  VALUES('cr_legacy_'||NEW.chapter_id,'sw_legacy_'||NEW.novel_id,'sch_legacy_'||NEW.chapter_id,1,NEW.title,NEW.content,
    chapter_hash,NEW.char_count,'legacy-live:chapter-revision:'||NEW.chapter_id,NEW.created_at,NEW.created_at)
  ON CONFLICT(chapter_revision_id) DO NOTHING;
  INSERT INTO drama.source_version_chapters(version_chapter_id,work_id,source_version_id,chapter_id,chapter_revision_id,
    ordinal,idempotency_key,created_at,updated_at)
  VALUES('svc_legacy_'||NEW.chapter_id,'sw_legacy_'||NEW.novel_id,'sv_legacy_'||NEW.novel_id,
    'sch_legacy_'||NEW.chapter_id,'cr_legacy_'||NEW.chapter_id,NEW.chapter_number,
    'legacy-live:membership:'||NEW.chapter_id,NEW.created_at,NEW.created_at)
  ON CONFLICT(version_chapter_id) DO NOTHING;
  IF octet_length(NEW.content)>0 AND char_length(NEW.content)>0 THEN
    INSERT INTO drama.source_spans(source_span_id,work_id,source_version_id,chapter_id,chapter_revision_id,
      start_utf8_byte,end_utf8_byte,start_codepoint,end_codepoint,start_paragraph,end_paragraph,excerpt_hash,
      locator_version,idempotency_key,created_at,updated_at)
    VALUES('span_legacy_full_'||NEW.chapter_id,'sw_legacy_'||NEW.novel_id,'sv_legacy_'||NEW.novel_id,
      'sch_legacy_'||NEW.chapter_id,'cr_legacy_'||NEW.chapter_id,0,octet_length(NEW.content),0,char_length(NEW.content),
      1,GREATEST(1,1+length(NEW.content)-length(replace(NEW.content,E'\n',''))),chapter_hash,'utf8-codepoint-v1',
      'legacy-live:span:'||NEW.chapter_id,NEW.created_at,NEW.created_at)
    ON CONFLICT(source_span_id) DO NOTHING;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.seal_completed_legacy_import()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE legacy_novel_id TEXT;
BEGIN
  IF NEW.workflow_stage<>'novel_import' OR NEW.status<>'completed' OR OLD.status='completed' THEN RETURN NEW; END IF;
  legacy_novel_id:=COALESCE(NEW.output_data#>>'{data_ref,entity_id}',NEW.entity_id);
  IF legacy_novel_id IS NULL OR legacy_novel_id='' OR NOT EXISTS(SELECT 1 FROM drama.novels WHERE novel_id=legacy_novel_id) THEN
    RETURN NEW;
  END IF;
  UPDATE drama.source_versions version SET
    total_chars=stats.total_chars,chapter_count=stats.chapter_count,version_hash=stats.version_hash,
    status='published',is_current=true,published_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP
  FROM (
    SELECT count(*)::integer chapter_count,COALESCE(sum(char_count),0)::integer total_chars,
      encode(digest(convert_to(COALESCE(string_agg(content,E'\n' ORDER BY chapter_number),legacy_novel_id),'UTF8'),'sha256'),'hex') version_hash
    FROM drama.novel_chapters WHERE novel_id=legacy_novel_id
  ) stats WHERE version.source_version_id='sv_legacy_'||legacy_novel_id AND version.status='draft';
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_prepare_legacy_source ON drama.novels;
CREATE TRIGGER trg_prepare_legacy_source AFTER INSERT ON drama.novels
  FOR EACH ROW EXECUTE FUNCTION drama.prepare_legacy_source();
DROP TRIGGER IF EXISTS trg_mirror_legacy_chapter ON drama.novel_chapters;
CREATE TRIGGER trg_mirror_legacy_chapter AFTER INSERT ON drama.novel_chapters
  FOR EACH ROW EXECUTE FUNCTION drama.mirror_legacy_chapter();
DROP TRIGGER IF EXISTS trg_seal_completed_legacy_import ON drama.workflow_tasks;
CREATE TRIGGER trg_seal_completed_legacy_import AFTER UPDATE OF status ON drama.workflow_tasks
  FOR EACH ROW EXECUTE FUNCTION drama.seal_completed_legacy_import();

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES('09','phase5-contract-corrections-v4-20260721','Phase 5 contract sealing, exact provenance, impact isolation, lease recovery and live legacy bridge')
ON CONFLICT(version) DO NOTHING;

\else
\echo 'migration 09 already applied with matching checksum; no-op'
\endif

COMMIT;
