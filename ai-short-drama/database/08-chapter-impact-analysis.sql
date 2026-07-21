BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(hashtext('drama:08-chapter-impact-analysis'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL THEN
    RAISE EXCEPTION 'migration 06 must be applied before migration 08';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='08';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'chapter-impact-analysis-v1-20260721' THEN
    RAISE EXCEPTION 'migration 08 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='08') AS phase4_apply \gset

\if :phase4_apply

-- Additive contract for chapter-scoped IR candidates and precise downstream
-- invalidation.  Published source/IR rows remain immutable and no legacy table
-- or column is removed.
ALTER TABLE drama.narrative_ir_revisions
  ADD COLUMN IF NOT EXISTS revision_scope TEXT NOT NULL DEFAULT 'full',
  ADD COLUMN IF NOT EXISTS base_ir_revision_id TEXT,
  ADD COLUMN IF NOT EXISTS changed_chapter_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.narrative_ir_revisions'::regclass
      AND conname='narrative_ir_revisions_scope_check'
  ) THEN
    ALTER TABLE drama.narrative_ir_revisions
      ADD CONSTRAINT narrative_ir_revisions_scope_check
      CHECK (
        (revision_scope='full' AND base_ir_revision_id IS NULL) OR
        (revision_scope='incremental' AND base_ir_revision_id IS NOT NULL
          AND jsonb_typeof(changed_chapter_ids)='array'
          AND jsonb_array_length(changed_chapter_ids)>0)
      ) NOT VALID;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.narrative_ir_revisions'::regclass
      AND conname='narrative_ir_revisions_base_fk'
  ) THEN
    ALTER TABLE drama.narrative_ir_revisions
      ADD CONSTRAINT narrative_ir_revisions_base_fk
      FOREIGN KEY(base_ir_revision_id)
      REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE RESTRICT NOT VALID;
  END IF;
END $$;

ALTER TABLE drama.source_change_sets
  ADD COLUMN IF NOT EXISTS from_ir_revision_id TEXT,
  ADD COLUMN IF NOT EXISTS to_ir_revision_id TEXT,
  ADD COLUMN IF NOT EXISTS changed_chapter_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.source_change_sets'::regclass
      AND conname='source_change_sets_from_ir_fk'
  ) THEN
    ALTER TABLE drama.source_change_sets
      ADD CONSTRAINT source_change_sets_from_ir_fk FOREIGN KEY(from_ir_revision_id)
      REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE RESTRICT NOT VALID;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.source_change_sets'::regclass
      AND conname='source_change_sets_to_ir_fk'
  ) THEN
    ALTER TABLE drama.source_change_sets
      ADD CONSTRAINT source_change_sets_to_ir_fk FOREIGN KEY(to_ir_revision_id)
      REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE RESTRICT NOT VALID;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='drama.source_change_sets'::regclass
      AND conname='source_change_sets_changed_chapters_check'
  ) THEN
    ALTER TABLE drama.source_change_sets
      ADD CONSTRAINT source_change_sets_changed_chapters_check
      CHECK (jsonb_typeof(changed_chapter_ids)='array') NOT VALID;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS drama.regeneration_requests (
  id BIGSERIAL PRIMARY KEY,
  regeneration_request_id TEXT NOT NULL UNIQUE,
  source_change_set_id TEXT NOT NULL REFERENCES drama.source_change_sets(source_change_set_id) ON DELETE RESTRICT,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  strategy TEXT NOT NULL DEFAULT 'selective' CHECK(strategy IN ('selective','full_recompile')),
  status TEXT NOT NULL DEFAULT 'queued'
    CHECK(status IN ('queued','running','completed','failed','cancelled')),
  requested_by TEXT,
  idempotency_key TEXT NOT NULL UNIQUE,
  request_summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code TEXT,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(regeneration_request_id,project_id,source_change_set_id),
  CHECK (NOT drama.jsonb_has_forbidden_provider_payload(request_summary))
);

CREATE TABLE IF NOT EXISTS drama.regeneration_request_items (
  id BIGSERIAL PRIMARY KEY,
  regeneration_request_item_id TEXT NOT NULL UNIQUE,
  regeneration_request_id TEXT NOT NULL REFERENCES drama.regeneration_requests(regeneration_request_id) ON DELETE CASCADE,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  requested_action TEXT NOT NULL DEFAULT 'regenerate' CHECK(requested_action='regenerate'),
  status TEXT NOT NULL DEFAULT 'queued'
    CHECK(status IN ('queued','running','completed','failed','cancelled','skipped')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(regeneration_request_id,artifact_id)
);

CREATE INDEX IF NOT EXISTS idx_ir_incremental_base
  ON drama.narrative_ir_revisions(base_ir_revision_id,status,created_at)
  WHERE revision_scope='incremental';
CREATE INDEX IF NOT EXISTS idx_source_change_sets_project_lookup
  ON drama.source_change_sets(to_source_version_id,status,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_source_change_items_change_lookup
  ON drama.source_change_items(source_change_set_id,entity_type,change_type);
CREATE INDEX IF NOT EXISTS idx_invalidation_impacts_artifact
  ON drama.invalidation_impacts(artifact_id,invalidation_task_id);
CREATE INDEX IF NOT EXISTS idx_regeneration_requests_project
  ON drama.regeneration_requests(project_id,status,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_regeneration_request_items_status
  ON drama.regeneration_request_items(regeneration_request_id,status,artifact_id);

DROP TRIGGER IF EXISTS trg_regeneration_requests_updated_at ON drama.regeneration_requests;
CREATE TRIGGER trg_regeneration_requests_updated_at
  BEFORE UPDATE ON drama.regeneration_requests
  FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
DROP TRIGGER IF EXISTS trg_regeneration_request_items_updated_at ON drama.regeneration_request_items;
CREATE TRIGGER trg_regeneration_request_items_updated_at
  BEFORE UPDATE ON drama.regeneration_request_items
  FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();

COMMENT ON COLUMN drama.narrative_ir_revisions.revision_scope IS
  'full is a complete source snapshot; incremental is a chapter-scoped candidate compared with base_ir_revision_id and is never promoted to current automatically.';
COMMENT ON TABLE drama.regeneration_requests IS
  'User-authored regeneration decisions. Creating a request never deletes or overwrites reviewed artifacts.';

CREATE OR REPLACE FUNCTION drama.guard_incremental_ir_publish()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE base_source_version TEXT;
BEGIN
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
      WHERE current_chapter.chapter_id IS NULL
        OR current_chapter.chapter_revision_id IS NOT DISTINCT FROM parent_chapter.chapter_revision_id
    ) THEN
      RAISE EXCEPTION 'incremental IR chapters must be unique changed chapters in the child source version';
    END IF;
    IF NEW.status='published' THEN
      IF TG_OP='UPDATE' THEN
        IF OLD.status IS DISTINCT FROM 'published' AND EXISTS(
          SELECT 1 FROM drama.narrative_ir_revisions prior
          WHERE prior.source_version_id=NEW.source_version_id AND prior.revision_scope='incremental'
            AND prior.status='published' AND prior.ir_revision_id<>NEW.ir_revision_id
        ) THEN
          RAISE EXCEPTION 'source version % already has a published incremental IR candidate',NEW.source_version_id;
        END IF;
      END IF;
      NEW.is_current:=false;
    END IF;
  END IF;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_incremental_ir_publish_guard ON drama.narrative_ir_revisions;
CREATE TRIGGER trg_incremental_ir_publish_guard
  BEFORE INSERT OR UPDATE ON drama.narrative_ir_revisions
  FOR EACH ROW EXECUTE FUNCTION drama.guard_incremental_ir_publish();

CREATE OR REPLACE FUNCTION drama.enqueue_incremental_impact()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  base_source_version TEXT;
  change_set_id TEXT;
BEGIN
  IF NEW.revision_scope<>'incremental' OR NEW.status<>'published'
     OR OLD.status='published' THEN RETURN NEW; END IF;
  SELECT source_version_id INTO base_source_version
  FROM drama.narrative_ir_revisions WHERE ir_revision_id=NEW.base_ir_revision_id;
  change_set_id:='chg_'||substr(encode(drama.digest(
    base_source_version||'|'||NEW.source_version_id,'sha256'),'hex'),1,32);

  INSERT INTO drama.source_change_sets(source_change_set_id,work_id,from_source_version_id,to_source_version_id,
    from_ir_revision_id,to_ir_revision_id,changed_chapter_ids,status,idempotency_key,summary)
  VALUES(change_set_id,NEW.work_id,base_source_version,NEW.source_version_id,NEW.base_ir_revision_id,
    NEW.ir_revision_id,NEW.changed_chapter_ids,'pending','chapter-impact:'||base_source_version||':'||NEW.source_version_id,
    jsonb_build_object('state','queued','candidate_ir_revision_id',NEW.ir_revision_id))
  ON CONFLICT(from_source_version_id,to_source_version_id) DO UPDATE SET
    from_ir_revision_id=EXCLUDED.from_ir_revision_id,to_ir_revision_id=EXCLUDED.to_ir_revision_id,
    changed_chapter_ids=EXCLUDED.changed_chapter_ids,updated_at=CURRENT_TIMESTAMP;

  INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,
    idempotency_key,input_hash,checkpoint_stage,checkpoint_data)
  SELECT 'op_'||substr(encode(drama.digest(binding.project_id||'|'||change_set_id,'sha256'),'hex'),1,32),
    'tr_'||substr(encode(drama.digest('trace|'||binding.project_id||'|'||change_set_id,'sha256'),'hex'),1,32),
    'invalidation_scan','project',binding.project_id,'pending','chapter-impact-scan:'||binding.project_id||':'||change_set_id,
    encode(drama.digest(NEW.base_ir_revision_id||'|'||NEW.ir_revision_id||'|'||NEW.changed_chapter_ids::text,'sha256'),'hex'),
    'queued',jsonb_build_object('source_change_set_id',change_set_id,'changed_chapter_ids',NEW.changed_chapter_ids)
  FROM drama.project_source_bindings binding
  WHERE binding.work_id=NEW.work_id AND binding.is_current
    AND binding.source_version_id IN (base_source_version,NEW.source_version_id)
  ON CONFLICT(idempotency_key) DO NOTHING;

  INSERT INTO drama.invalidation_tasks(invalidation_task_id,operation_id,project_id,source_change_set_id,
    status,reason_type,idempotency_key,checkpoint)
  SELECT 'inv_'||substr(encode(drama.digest(operation.operation_id,'sha256'),'hex'),1,32),operation.operation_id,
    operation.target_id,change_set_id,'pending','source_changed','chapter-impact-task:'||operation.target_id||':'||change_set_id,
    jsonb_build_object('stage','queued','changed_chapter_ids',NEW.changed_chapter_ids)
  FROM drama.operations operation
  WHERE operation.operation_type='invalidation_scan'
    AND operation.idempotency_key LIKE 'chapter-impact-scan:%:'||change_set_id
  ON CONFLICT(idempotency_key) DO NOTHING;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_enqueue_incremental_impact ON drama.narrative_ir_revisions;
CREATE TRIGGER trg_enqueue_incremental_impact
  AFTER UPDATE OF status ON drama.narrative_ir_revisions
  FOR EACH ROW EXECUTE FUNCTION drama.enqueue_incremental_impact();

CREATE OR REPLACE FUNCTION drama.analyze_chapter_impact(p_operation_id TEXT,p_claim_token UUID)
RETURNS JSONB LANGUAGE plpgsql AS $$
DECLARE
  task_row drama.invalidation_tasks%ROWTYPE;
  change_row drama.source_change_sets%ROWTYPE;
  impact_count INTEGER;
BEGIN
  PERFORM drama.assert_operation_claim(p_operation_id,p_claim_token);
  SELECT * INTO task_row FROM drama.invalidation_tasks WHERE operation_id=p_operation_id FOR UPDATE;
  IF task_row.invalidation_task_id IS NULL THEN RAISE EXCEPTION 'invalidation task not found'; END IF;
  SELECT * INTO change_row FROM drama.source_change_sets
    WHERE source_change_set_id=task_row.source_change_set_id FOR UPDATE;
  UPDATE drama.invalidation_tasks SET status='running',started_at=COALESCE(started_at,CURRENT_TIMESTAMP),
    checkpoint=jsonb_build_object('stage','comparing_ir','changed_chapter_ids',change_row.changed_chapter_ids)
    WHERE invalidation_task_id=task_row.invalidation_task_id;
  UPDATE drama.source_change_sets SET status='running' WHERE source_change_set_id=change_row.source_change_set_id;

  -- Compare logical event identities and canonical fingerprints only inside the
  -- revised chapters. Exact source spans remain attached to each fact revision.
  INSERT INTO drama.source_change_items(source_change_item_id,source_change_set_id,entity_type,change_type,
    before_entity_id,after_entity_id,semantic_fingerprint,details)
  SELECT 'sci_'||substr(encode(drama.digest(change_row.source_change_set_id||'|event|'||COALESCE(old_fact.fact_id,new_fact.fact_id),'sha256'),'hex'),1,32),
    change_row.source_change_set_id,'fact',
    CASE WHEN old_fact.fact_revision_id IS NULL THEN 'added' WHEN new_fact.fact_revision_id IS NULL THEN 'removed' ELSE 'changed' END,
    old_fact.fact_revision_id,new_fact.fact_revision_id,COALESCE(new_fact.canonical_fingerprint,old_fact.canonical_fingerprint),
    jsonb_build_object('subtype','event','logical_fact_id',COALESCE(old_fact.fact_id,new_fact.fact_id),
      'before_event_revision_id',old_event.event_revision_id,'after_event_revision_id',new_event.event_revision_id,
      'chapter_id',COALESCE(new_fact.chapter_id,old_fact.chapter_id))
  FROM (SELECT fact.* FROM drama.narrative_fact_revisions fact
    JOIN drama.narrative_event_revisions event USING(fact_revision_id)
    WHERE fact.ir_revision_id=change_row.from_ir_revision_id
      AND change_row.changed_chapter_ids ? fact.chapter_id) old_fact
  FULL JOIN (SELECT fact.* FROM drama.narrative_fact_revisions fact
    JOIN drama.narrative_event_revisions event USING(fact_revision_id)
    WHERE fact.ir_revision_id=change_row.to_ir_revision_id) new_fact USING(fact_id)
  LEFT JOIN drama.narrative_event_revisions old_event ON old_event.fact_revision_id=old_fact.fact_revision_id
  LEFT JOIN drama.narrative_event_revisions new_event ON new_event.fact_revision_id=new_fact.fact_revision_id
  WHERE old_fact.fact_revision_id IS NULL OR new_fact.fact_revision_id IS NULL
    OR old_fact.canonical_fingerprint<>new_fact.canonical_fingerprint
  ON CONFLICT(source_change_item_id) DO NOTHING;

  INSERT INTO drama.source_change_items(source_change_item_id,source_change_set_id,entity_type,change_type,
    before_entity_id,after_entity_id,semantic_fingerprint,details)
  SELECT 'sci_'||substr(encode(drama.digest(change_row.source_change_set_id||'|state|'||COALESCE(old_fact.fact_id,new_fact.fact_id),'sha256'),'hex'),1,32),
    change_row.source_change_set_id,'fact',
    CASE WHEN old_state.state_change_id IS NULL THEN 'added' WHEN new_state.state_change_id IS NULL THEN 'removed' ELSE 'changed' END,
    old_state.state_change_id,new_state.state_change_id,COALESCE(new_fact.canonical_fingerprint,old_fact.canonical_fingerprint),
    jsonb_build_object('subtype','character_state','logical_fact_id',COALESCE(old_fact.fact_id,new_fact.fact_id),
      'character_entity_id',COALESCE(new_entity.entity_id,old_entity.entity_id),
      'state_dimension',COALESCE(new_state.state_dimension,old_state.state_dimension),
      'before_state',old_state.after_state,'after_state',new_state.after_state,
      'chapter_id',COALESCE(new_fact.chapter_id,old_fact.chapter_id))
  FROM (SELECT fact.* FROM drama.narrative_fact_revisions fact
    JOIN drama.character_state_changes state USING(fact_revision_id)
    WHERE fact.ir_revision_id=change_row.from_ir_revision_id
      AND change_row.changed_chapter_ids ? fact.chapter_id) old_fact
  FULL JOIN (SELECT fact.* FROM drama.narrative_fact_revisions fact
    JOIN drama.character_state_changes state USING(fact_revision_id)
    WHERE fact.ir_revision_id=change_row.to_ir_revision_id) new_fact USING(fact_id)
  LEFT JOIN drama.character_state_changes old_state ON old_state.fact_revision_id=old_fact.fact_revision_id
  LEFT JOIN drama.character_state_changes new_state ON new_state.fact_revision_id=new_fact.fact_revision_id
  LEFT JOIN drama.narrative_entity_revisions old_entity ON old_entity.entity_revision_id=old_state.character_entity_revision_id
  LEFT JOIN drama.narrative_entity_revisions new_entity ON new_entity.entity_revision_id=new_state.character_entity_revision_id
  WHERE old_state.state_change_id IS NULL OR new_state.state_change_id IS NULL
    OR old_fact.canonical_fingerprint<>new_fact.canonical_fingerprint
    OR old_state.before_state<>new_state.before_state OR old_state.after_state<>new_state.after_state
  ON CONFLICT(source_change_item_id) DO NOTHING;

  -- A story arc is affected when one of its old events changed or when its
  -- chapter-scoped candidate revision changed. The immutable arc row itself is
  -- never updated.
  INSERT INTO drama.source_change_items(source_change_item_id,source_change_set_id,entity_type,change_type,
    before_entity_id,after_entity_id,details)
  SELECT 'sci_'||substr(encode(drama.digest(change_row.source_change_set_id||'|arc|'||old_arc.story_arc_id,'sha256'),'hex'),1,32),
    change_row.source_change_set_id,'story_arc',CASE WHEN new_arc.story_arc_revision_id IS NULL THEN 'removed' ELSE 'changed' END,
    old_arc.story_arc_revision_id,new_arc.story_arc_revision_id,
    jsonb_build_object('subtype','story_arc','logical_story_arc_id',old_arc.story_arc_id,'title',old_arc.title)
  FROM drama.story_arc_revisions old_arc
  LEFT JOIN drama.story_arc_revisions new_arc ON new_arc.ir_revision_id=change_row.to_ir_revision_id
    AND new_arc.story_arc_id=old_arc.story_arc_id
  WHERE old_arc.ir_revision_id=change_row.from_ir_revision_id AND (
    change_row.changed_chapter_ids ? old_arc.chapter_id OR EXISTS(
      SELECT 1 FROM drama.story_arc_events arc_event
      JOIN drama.narrative_event_revisions event USING(event_revision_id)
      JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
      WHERE arc_event.story_arc_revision_id=old_arc.story_arc_revision_id
        AND change_row.changed_chapter_ids ? fact.chapter_id))
  ON CONFLICT(source_change_item_id) DO NOTHING;

  INSERT INTO drama.source_change_items(source_change_item_id,source_change_set_id,entity_type,change_type,
    before_entity_id,after_entity_id,details)
  SELECT 'sci_'||substr(encode(drama.digest(change_row.source_change_set_id||'|arc|'||new_arc.story_arc_id,'sha256'),'hex'),1,32),
    change_row.source_change_set_id,'story_arc','added',NULL,new_arc.story_arc_revision_id,
    jsonb_build_object('subtype','story_arc','logical_story_arc_id',new_arc.story_arc_id,'title',new_arc.title)
  FROM drama.story_arc_revisions new_arc
  WHERE new_arc.ir_revision_id=change_row.to_ir_revision_id AND NOT EXISTS(
    SELECT 1 FROM drama.story_arc_revisions old_arc
    WHERE old_arc.ir_revision_id=change_row.from_ir_revision_id AND old_arc.story_arc_id=new_arc.story_arc_id)
  ON CONFLICT(source_change_item_id) DO NOTHING;

  -- Materialize lineage identities for legacy reviewed outputs without changing
  -- their domain review status or content.
  INSERT INTO drama.artifacts(artifact_id,artifact_type,native_entity_id,revision_number,content_hash,
    validity_status,is_current,idempotency_key,metadata)
  SELECT 'art_'||substr(encode(drama.digest('story-arc|'||arc.story_arc_revision_id,'sha256'),'hex'),1,32),
    'story_arc_revision',arc.story_arc_revision_id,ir.revision_number,
    encode(drama.digest(arc.title||'|'||arc.summary||'|'||arc.arc_type,'sha256'),'hex'),'valid',false,
    'impact-story-arc:'||arc.story_arc_revision_id,jsonb_build_object('story_arc_id',arc.story_arc_id)
  FROM drama.source_change_items item JOIN drama.story_arc_revisions arc
    ON arc.story_arc_revision_id=item.before_entity_id
  JOIN drama.narrative_ir_revisions ir USING(ir_revision_id)
  WHERE item.source_change_set_id=change_row.source_change_set_id AND item.details->>'subtype'='story_arc'
  ON CONFLICT DO NOTHING;

  INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
    validity_status,is_current,idempotency_key,metadata)
  SELECT 'art_'||substr(encode(drama.digest('outline|'||outline.episode_id,'sha256'),'hex'),1,32),
    'episode_outline',outline.project_id,outline.episode_id,outline.version,
    encode(drama.digest(to_jsonb(outline)::text,'sha256'),'hex'),'valid',true,'impact-outline:'||outline.episode_id,
    jsonb_build_object('review_status',outline.status)
  FROM drama.episode_outlines outline
  WHERE outline.project_id=task_row.project_id AND EXISTS(
    SELECT 1 FROM jsonb_array_elements_text(outline.source_chapter_ids) chapter_id
    WHERE change_row.changed_chapter_ids ? chapter_id)
  ON CONFLICT DO NOTHING;

  INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
    validity_status,is_current,idempotency_key,metadata)
  SELECT 'art_'||substr(encode(drama.digest('script|'||script.script_id,'sha256'),'hex'),1,32),
    'episode_script',script.project_id,script.script_id,script.version,
    encode(drama.digest(to_jsonb(script)::text,'sha256'),'hex'),'valid',true,'impact-script:'||script.script_id,
    jsonb_build_object('review_status',script.status,'episode_id',script.episode_id)
  FROM drama.episode_scripts script JOIN drama.episode_outlines outline USING(episode_id)
  WHERE script.project_id=task_row.project_id AND (
    EXISTS(SELECT 1 FROM jsonb_array_elements_text(outline.source_chapter_ids) chapter_id
      WHERE change_row.changed_chapter_ids ? chapter_id)
    OR EXISTS(SELECT 1 FROM drama.script_scenes scene
      CROSS JOIN LATERAL jsonb_array_elements_text(scene.source_event_ids) source_event_id
      JOIN drama.narrative_event_revisions event ON event.event_revision_id=source_event_id
      JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
      WHERE scene.script_id=script.script_id AND fact.ir_revision_id=change_row.from_ir_revision_id
        AND change_row.changed_chapter_ids ? fact.chapter_id))
  ON CONFLICT DO NOTHING;

  -- Direct selectors use exact logical facts, event assignments and chapter
  -- provenance. Recursive propagation follows only declared dependencies.
  WITH RECURSIVE changed_old_facts AS (
    SELECT before_entity_id fact_revision_id FROM drama.source_change_items
    WHERE source_change_set_id=change_row.source_change_set_id AND details->>'subtype' IN ('event','character_state')
      AND before_entity_id IS NOT NULL
  ), changed_old_events AS (
    SELECT event.event_revision_id FROM changed_old_facts JOIN drama.narrative_event_revisions event USING(fact_revision_id)
  ), direct AS (
    SELECT artifact.artifact_id,0 depth,jsonb_build_array(artifact.artifact_id) path
    FROM drama.artifacts artifact WHERE (artifact.project_id IS NULL OR artifact.project_id=task_row.project_id) AND (
      EXISTS(SELECT 1 FROM drama.artifact_source_evidence evidence JOIN changed_old_facts USING(fact_revision_id)
        WHERE evidence.artifact_id=artifact.artifact_id)
      OR (artifact.artifact_type='story_arc_revision' AND EXISTS(SELECT 1 FROM drama.source_change_items item
        WHERE item.source_change_set_id=change_row.source_change_set_id AND item.before_entity_id=artifact.native_entity_id
          AND item.details->>'subtype'='story_arc'))
      OR (artifact.artifact_type='adaptation_episode_plan' AND EXISTS(
        SELECT 1 FROM drama.adaptation_episode_plans episode
        WHERE episode.adaptation_episode_plan_id=artifact.native_entity_id AND (
          EXISTS(SELECT 1 FROM drama.episode_event_assignments assignment JOIN changed_old_events USING(event_revision_id)
            WHERE assignment.adaptation_episode_plan_id=episode.adaptation_episode_plan_id)
          OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(episode.source_chapter_ids) chapter_id
            WHERE change_row.changed_chapter_ids ? chapter_id))))
      OR (artifact.artifact_type='adaptation_plan' AND EXISTS(
        SELECT 1 FROM drama.adaptation_episode_plans episode WHERE episode.adaptation_plan_id=artifact.native_entity_id AND (
          EXISTS(SELECT 1 FROM drama.episode_event_assignments assignment JOIN changed_old_events USING(event_revision_id)
            WHERE assignment.adaptation_episode_plan_id=episode.adaptation_episode_plan_id)
          OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(episode.source_chapter_ids) chapter_id
            WHERE change_row.changed_chapter_ids ? chapter_id))))
      OR (artifact.artifact_type='episode_outline' AND EXISTS(SELECT 1 FROM drama.episode_outlines outline
        CROSS JOIN LATERAL jsonb_array_elements_text(outline.source_chapter_ids) chapter_id
        WHERE outline.episode_id=artifact.native_entity_id AND change_row.changed_chapter_ids ? chapter_id))
      OR (artifact.artifact_type='episode_script' AND EXISTS(SELECT 1 FROM drama.episode_scripts script
        JOIN drama.episode_outlines outline USING(episode_id)
        WHERE script.script_id=artifact.native_entity_id AND EXISTS(
          SELECT 1 FROM jsonb_array_elements_text(outline.source_chapter_ids) chapter_id
          WHERE change_row.changed_chapter_ids ? chapter_id)))
    )
  ), affected AS (
    SELECT * FROM direct
    UNION ALL
    SELECT downstream.artifact_id,affected.depth+1,affected.path||to_jsonb(downstream.artifact_id)
    FROM affected JOIN drama.artifact_dependencies dependency ON dependency.upstream_artifact_id=affected.artifact_id
    JOIN drama.artifacts downstream ON downstream.artifact_id=dependency.downstream_artifact_id
    WHERE affected.depth<30 AND NOT affected.path ? downstream.artifact_id
      AND (downstream.project_id IS NULL OR downstream.project_id=task_row.project_id)
  ), collapsed AS (
    SELECT artifact_id,min(depth) depth FROM affected GROUP BY artifact_id
  ), impacts AS (
    INSERT INTO drama.invalidation_impacts(invalidation_impact_id,invalidation_task_id,artifact_id,before_status,
      after_status,propagation_depth,reason,dependency_path)
    SELECT 'impi_'||substr(encode(drama.digest(task_row.invalidation_task_id||'|'||artifact.artifact_id,'sha256'),'hex'),1,32),
      task_row.invalidation_task_id,artifact.artifact_id,artifact.validity_status,'stale',collapsed.depth,
      jsonb_build_object('source_change_set_id',change_row.source_change_set_id,'selector','exact_lineage'),
      jsonb_build_array(artifact.artifact_id)
    FROM collapsed JOIN drama.artifacts artifact USING(artifact_id)
    ON CONFLICT(invalidation_task_id,artifact_id) DO NOTHING RETURNING artifact_id
  )
  UPDATE drama.artifacts artifact SET validity_status='stale'
  WHERE artifact.artifact_id IN (SELECT artifact_id FROM collapsed) AND artifact.validity_status<>'stale';

  SELECT count(*) INTO impact_count FROM drama.invalidation_impacts
    WHERE invalidation_task_id=task_row.invalidation_task_id;
  UPDATE drama.invalidation_tasks SET status='needs_review',completed_at=CURRENT_TIMESTAMP,
    checkpoint=jsonb_build_object('stage','finished','affected_artifact_count',impact_count,
      'preserved_reviewed_artifacts',true) WHERE invalidation_task_id=task_row.invalidation_task_id;
  UPDATE drama.source_change_sets SET status='needs_review',summary=jsonb_build_object(
    'changed_chapter_ids',change_row.changed_chapter_ids,
    'changed_event_count',(SELECT count(*) FROM drama.source_change_items WHERE source_change_set_id=change_row.source_change_set_id AND details->>'subtype'='event'),
    'changed_character_state_count',(SELECT count(*) FROM drama.source_change_items WHERE source_change_set_id=change_row.source_change_set_id AND details->>'subtype'='character_state'),
    'affected_story_arc_count',(SELECT count(*) FROM drama.source_change_items WHERE source_change_set_id=change_row.source_change_set_id AND details->>'subtype'='story_arc'),
    'affected_artifact_count',impact_count,'reviewed_artifacts_preserved',true)
    WHERE source_change_set_id=change_row.source_change_set_id;
  PERFORM drama.finish_operation(p_operation_id,p_claim_token,'needs_review','invalidation_task',task_row.invalidation_task_id);
  RETURN jsonb_build_object('source_change_set_id',change_row.source_change_set_id,
    'invalidation_task_id',task_row.invalidation_task_id,'affected_artifact_count',impact_count,'status','needs_review');
END $$;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES('08','chapter-impact-analysis-v1-20260721','Chapter-scoped Narrative IR impact analysis and explicit regeneration decisions')
ON CONFLICT(version) DO NOTHING;

\else
\echo 'migration 08 already applied with matching checksum; no-op'
\endif

COMMIT;
