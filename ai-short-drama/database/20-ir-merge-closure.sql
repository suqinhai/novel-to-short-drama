BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '0';
SELECT pg_advisory_xact_lock(hashtext('drama:20-ir-merge-closure'));
SET search_path TO drama, public;

DO $$
DECLARE existing_checksum TEXT;
BEGIN
  IF to_regclass('drama.schema_migrations') IS NULL THEN
    RAISE EXCEPTION 'migration 06 must be applied before migration 20';
  END IF;
  SELECT checksum INTO existing_checksum FROM drama.schema_migrations WHERE version='20';
  IF existing_checksum IS NOT NULL AND existing_checksum <> 'incremental-full-ir-merge-v1-20260801' THEN
    RAISE EXCEPTION 'migration 20 checksum mismatch: %',existing_checksum;
  END IF;
END $$;

SELECT NOT EXISTS(SELECT 1 FROM drama.schema_migrations WHERE version='20') AS phase20_apply \gset

\if :phase20_apply

-- A proposal is mutable only while it is under review. Both input revisions and
-- every decision remain available after the resulting full snapshot is sealed.
CREATE TABLE drama.ir_merge_proposals (
  id BIGSERIAL PRIMARY KEY,
  ir_merge_proposal_id TEXT NOT NULL UNIQUE,
  work_id TEXT NOT NULL REFERENCES drama.source_works(work_id) ON DELETE RESTRICT,
  target_source_version_id TEXT NOT NULL,
  base_full_ir_revision_id TEXT NOT NULL REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE RESTRICT,
  incremental_ir_revision_id TEXT NOT NULL REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE RESTRICT,
  published_full_ir_revision_id TEXT UNIQUE,
  status TEXT NOT NULL DEFAULT 'draft'
    CHECK(status IN ('draft','ready','publishing','published','failed')),
  resource_revision INTEGER NOT NULL DEFAULT 1 CHECK(resource_revision > 0),
  confidence NUMERIC(5,4) NOT NULL DEFAULT 1 CHECK(confidence BETWEEN 0 AND 1),
  conflict_count INTEGER NOT NULL DEFAULT 0 CHECK(conflict_count >= 0),
  unresolved_count INTEGER NOT NULL DEFAULT 0 CHECK(unresolved_count >= 0),
  changed_chapter_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(changed_chapter_ids)='array'),
  impact_preview JSONB NOT NULL DEFAULT '{}'::jsonb,
  idempotency_key TEXT NOT NULL UNIQUE,
  publish_idempotency_key TEXT UNIQUE,
  created_by TEXT,
  published_by TEXT,
  published_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(base_full_ir_revision_id,incremental_ir_revision_id),
  UNIQUE(ir_merge_proposal_id,work_id,target_source_version_id),
  FOREIGN KEY(work_id,target_source_version_id)
    REFERENCES drama.source_versions(work_id,source_version_id) ON DELETE RESTRICT,
  CHECK((status='published' AND published_at IS NOT NULL AND published_full_ir_revision_id IS NOT NULL)
    OR status<>'published')
);

CREATE TABLE drama.ir_merge_proposal_items (
  id BIGSERIAL PRIMARY KEY,
  ir_merge_item_id TEXT NOT NULL UNIQUE,
  ir_merge_proposal_id TEXT NOT NULL REFERENCES drama.ir_merge_proposals(ir_merge_proposal_id) ON DELETE RESTRICT,
  item_type TEXT NOT NULL
    CHECK(item_type IN ('entity','fact','event','relation','state','foreshadow','story_arc')),
  logical_id TEXT NOT NULL,
  change_type TEXT NOT NULL
    CHECK(change_type IN ('added','modified','deleted','relocated','unchanged','conflict')),
  before_revision_id TEXT,
  after_revision_id TEXT,
  before_value JSONB,
  after_value JSONB,
  before_evidence JSONB,
  after_evidence JSONB,
  semantic_fingerprint TEXT CHECK(semantic_fingerprint IS NULL OR semantic_fingerprint ~ '^[0-9a-f]{64}$'),
  semantic_changed BOOLEAN NOT NULL DEFAULT true,
  source_span_changed BOOLEAN NOT NULL DEFAULT false,
  confidence NUMERIC(5,4) NOT NULL DEFAULT 1 CHECK(confidence BETWEEN 0 AND 1),
  conflict_code TEXT,
  conflict_message TEXT,
  resolution TEXT CHECK(resolution IS NULL OR resolution IN
    ('accept_new','keep_old','merge','manual_edit','delete_invalid')),
  resolved_value JSONB,
  resolution_status TEXT NOT NULL DEFAULT 'unresolved'
    CHECK(resolution_status IN ('unresolved','resolved','needs_manual_edit')),
  canonicalization_required BOOLEAN NOT NULL DEFAULT false,
  canonicalization_confirmed BOOLEAN NOT NULL DEFAULT false,
  canonicalization_decision TEXT
    CHECK(canonicalization_decision IS NULL OR canonicalization_decision IN ('same_entity','distinct_entities')),
  canonical_entity_id TEXT,
  resolution_note TEXT,
  resolved_by TEXT,
  resolved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(ir_merge_proposal_id,item_type,logical_id),
  CHECK(NOT canonicalization_confirmed OR canonicalization_decision IS NOT NULL),
  CHECK(canonicalization_decision<>'same_entity' OR canonical_entity_id IS NOT NULL)
);

CREATE TABLE drama.regeneration_proposals (
  id BIGSERIAL PRIMARY KEY,
  regeneration_proposal_id TEXT NOT NULL UNIQUE,
  source_change_set_id TEXT NOT NULL REFERENCES drama.source_change_sets(source_change_set_id) ON DELETE RESTRICT,
  project_id TEXT NOT NULL REFERENCES drama.projects(project_id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'proposed' CHECK(status IN ('proposed','accepted','dismissed')),
  summary JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(source_change_set_id,project_id)
);

CREATE TABLE drama.regeneration_proposal_items (
  id BIGSERIAL PRIMARY KEY,
  regeneration_proposal_item_id TEXT NOT NULL UNIQUE,
  regeneration_proposal_id TEXT NOT NULL REFERENCES drama.regeneration_proposals(regeneration_proposal_id) ON DELETE CASCADE,
  artifact_id TEXT NOT NULL REFERENCES drama.artifacts(artifact_id) ON DELETE RESTRICT,
  selected BOOLEAN NOT NULL DEFAULT false,
  recommended_action TEXT NOT NULL DEFAULT 'regenerate' CHECK(recommended_action='regenerate'),
  reason JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(regeneration_proposal_id,artifact_id)
);

ALTER TABLE drama.narrative_ir_revisions
  ADD COLUMN merge_proposal_id TEXT REFERENCES drama.ir_merge_proposals(ir_merge_proposal_id) ON DELETE RESTRICT;
ALTER TABLE drama.ir_merge_proposals
  ADD CONSTRAINT ir_merge_proposals_published_ir_fk FOREIGN KEY(published_full_ir_revision_id)
  REFERENCES drama.narrative_ir_revisions(ir_revision_id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX uq_narrative_ir_merge_proposal
  ON drama.narrative_ir_revisions(merge_proposal_id) WHERE merge_proposal_id IS NOT NULL;

-- The merge is a first-class operation, while retaining the existing composite
-- operation foreign key used by Narrative IR revisions.
ALTER TABLE drama.operations DROP CONSTRAINT IF EXISTS operations_operation_type_check;
ALTER TABLE drama.operations ADD CONSTRAINT operations_operation_type_check CHECK(operation_type IN (
  'source_import','ir_extraction','ir_merge','spec_validation','adaptation_compile','invalidation_scan'));
ALTER TABLE drama.narrative_ir_revisions DROP CONSTRAINT IF EXISTS narrative_ir_revisions_operation_type_check;
ALTER TABLE drama.narrative_ir_revisions ADD CONSTRAINT narrative_ir_revisions_operation_type_check
  CHECK(operation_type IN ('ir_extraction','ir_merge'));

-- More than one audited scan can exist for a source pair (the early incremental
-- preview and the authoritative published-full-IR scan).
DO $$ DECLARE constraint_name TEXT; BEGIN
  SELECT conname INTO constraint_name FROM pg_constraint
  WHERE conrelid='drama.source_change_sets'::regclass AND contype='u'
    AND pg_get_constraintdef(oid)='UNIQUE (from_source_version_id, to_source_version_id)';
  IF constraint_name IS NOT NULL THEN
    EXECUTE format('ALTER TABLE drama.source_change_sets DROP CONSTRAINT %I',constraint_name);
  END IF;
END $$;
CREATE UNIQUE INDEX uq_source_change_sets_ir_transition
  ON drama.source_change_sets(from_source_version_id,to_source_version_id,
    COALESCE(from_ir_revision_id,''),COALESCE(to_ir_revision_id,''));

CREATE INDEX idx_ir_merge_proposals_target
  ON drama.ir_merge_proposals(target_source_version_id,status,created_at DESC);
CREATE INDEX idx_ir_merge_items_review
  ON drama.ir_merge_proposal_items(ir_merge_proposal_id,item_type,resolution_status,change_type);
CREATE INDEX idx_regeneration_proposals_project
  ON drama.regeneration_proposals(project_id,status,created_at DESC);

DO $$ DECLARE constraint_name TEXT; BEGIN
  SELECT conname INTO constraint_name FROM pg_constraint
  WHERE conrelid='drama.foreshadow_occurrences'::regclass AND contype='u'
    AND pg_get_constraintdef(oid)='UNIQUE (foreshadow_thread_id, occurrence_order)';
  IF constraint_name IS NOT NULL THEN
    EXECUTE format('ALTER TABLE drama.foreshadow_occurrences DROP CONSTRAINT %I',constraint_name);
  END IF;
END $$;
ALTER TABLE drama.foreshadow_occurrences
  ADD CONSTRAINT foreshadow_occurrences_thread_order_ir_key
  UNIQUE(foreshadow_thread_id,occurrence_order,ir_revision_id);

CREATE TRIGGER trg_ir_merge_proposals_updated_at
  BEFORE UPDATE ON drama.ir_merge_proposals
  FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
CREATE TRIGGER trg_ir_merge_items_updated_at
  BEFORE UPDATE ON drama.ir_merge_proposal_items
  FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();
CREATE TRIGGER trg_regeneration_proposals_updated_at
  BEFORE UPDATE ON drama.regeneration_proposals
  FOR EACH ROW EXECUTE FUNCTION drama.set_updated_at();

CREATE FUNCTION drama.guard_sealed_ir_merge()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE proposal_status TEXT;
BEGIN
  IF TG_TABLE_NAME='ir_merge_proposals' THEN
    IF OLD.status='published' AND (TG_OP='DELETE' OR NEW IS DISTINCT FROM OLD) THEN
      RAISE EXCEPTION 'published IR merge proposal % is immutable',OLD.ir_merge_proposal_id;
    END IF;
  ELSE
    SELECT status INTO proposal_status FROM drama.ir_merge_proposals
    WHERE ir_merge_proposal_id=COALESCE(NEW.ir_merge_proposal_id,OLD.ir_merge_proposal_id);
    IF proposal_status='published' THEN
      RAISE EXCEPTION 'items of a published IR merge proposal are immutable';
    END IF;
  END IF;
  RETURN CASE WHEN TG_OP='DELETE' THEN OLD ELSE NEW END;
END $$;

CREATE TRIGGER trg_ir_merge_proposals_immutable
  BEFORE UPDATE OR DELETE ON drama.ir_merge_proposals
  FOR EACH ROW EXECUTE FUNCTION drama.guard_sealed_ir_merge();
CREATE TRIGGER trg_ir_merge_items_immutable
  BEFORE UPDATE OR DELETE ON drama.ir_merge_proposal_items
  FOR EACH ROW EXECUTE FUNCTION drama.guard_sealed_ir_merge();

CREATE OR REPLACE FUNCTION drama.validate_compiler_frozen_inputs()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='UPDATE'
     AND NEW.project_id IS NOT DISTINCT FROM OLD.project_id
     AND NEW.work_id IS NOT DISTINCT FROM OLD.work_id
     AND NEW.source_version_id IS NOT DISTINCT FROM OLD.source_version_id
     AND NEW.adaptation_spec_version_id IS NOT DISTINCT FROM OLD.adaptation_spec_version_id
     AND NEW.ir_revision_id IS NOT DISTINCT FROM OLD.ir_revision_id THEN
    RETURN NEW;
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM drama.adaptation_spec_versions sp
    JOIN drama.narrative_ir_revisions ir ON ir.ir_revision_id=sp.ir_revision_id
    WHERE sp.adaptation_spec_version_id=NEW.adaptation_spec_version_id
      AND sp.project_id=NEW.project_id AND sp.work_id=NEW.work_id
      AND sp.source_version_id=NEW.source_version_id AND sp.ir_revision_id=NEW.ir_revision_id
      AND sp.status='active' AND ir.status='published' AND ir.revision_scope='full'
  ) THEN
    RAISE EXCEPTION 'compiler run requires matching active spec and published full IR inputs';
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION drama.guard_published_ir_revision()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' AND OLD.published_at IS NOT NULL THEN
    RAISE EXCEPTION 'published IR revision % is immutable',OLD.ir_revision_id;
  END IF;
  IF TG_OP='UPDATE' AND OLD.published_at IS NOT NULL AND (
    NEW.operation_id IS DISTINCT FROM OLD.operation_id OR
    NEW.operation_type IS DISTINCT FROM OLD.operation_type OR
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
    NEW.merge_proposal_id IS DISTINCT FROM OLD.merge_proposal_id OR
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

-- Refresh proposal counters in the same transaction as every conflict decision.
CREATE FUNCTION drama.refresh_ir_merge_proposal_counts()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE proposal_id TEXT:=COALESCE(NEW.ir_merge_proposal_id,OLD.ir_merge_proposal_id);
BEGIN
  UPDATE drama.ir_merge_proposals proposal SET
    conflict_count=(SELECT count(*) FROM drama.ir_merge_proposal_items item
      WHERE item.ir_merge_proposal_id=proposal_id AND item.conflict_code IS NOT NULL),
    unresolved_count=(SELECT count(*) FROM drama.ir_merge_proposal_items item
      WHERE item.ir_merge_proposal_id=proposal_id AND (
        item.resolution_status<>'resolved' OR
        (item.canonicalization_required AND NOT item.canonicalization_confirmed))),
    confidence=COALESCE((SELECT avg(item.confidence) FROM drama.ir_merge_proposal_items item
      WHERE item.ir_merge_proposal_id=proposal_id),1),
    status=CASE WHEN NOT EXISTS(SELECT 1 FROM drama.ir_merge_proposal_items item
      WHERE item.ir_merge_proposal_id=proposal_id AND (
        item.resolution_status<>'resolved' OR
        (item.canonicalization_required AND NOT item.canonicalization_confirmed)))
      THEN 'ready' ELSE 'draft' END,
    resource_revision=resource_revision+1
  WHERE proposal.ir_merge_proposal_id=proposal_id AND proposal.status IN ('draft','ready');
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;

CREATE CONSTRAINT TRIGGER trg_refresh_ir_merge_proposal_counts
  AFTER INSERT OR UPDATE OR DELETE ON drama.ir_merge_proposal_items
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION drama.refresh_ir_merge_proposal_counts();

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
    base_source_version||'|'||NEW.source_version_id||'|'||NEW.ir_revision_id,'sha256'),'hex'),1,32);

  INSERT INTO drama.source_change_sets(source_change_set_id,work_id,from_source_version_id,to_source_version_id,
    from_ir_revision_id,to_ir_revision_id,changed_chapter_ids,status,idempotency_key,summary)
  VALUES(change_set_id,NEW.work_id,base_source_version,NEW.source_version_id,NEW.base_ir_revision_id,
    NEW.ir_revision_id,NEW.changed_chapter_ids,'pending','chapter-impact:'||NEW.ir_revision_id,
    jsonb_build_object('state','queued','candidate_ir_revision_id',NEW.ir_revision_id,'authoritative_full_ir',false))
  ON CONFLICT(idempotency_key) DO UPDATE SET
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

-- Authoritative impact scan. Merge publication pre-populates source_change_items
-- from the reviewed proposal, so relocation-only rows are retained for audit but
-- excluded from invalidation. A fallback diff keeps legacy incremental scans
-- operational and scopes both sides to the changed chapters.
CREATE OR REPLACE FUNCTION drama.analyze_chapter_impact(p_operation_id TEXT,p_claim_token UUID)
RETURNS JSONB LANGUAGE plpgsql AS $$
DECLARE
  task_row drama.invalidation_tasks%ROWTYPE;
  change_row drama.source_change_sets%ROWTYPE;
  impact_count INTEGER;
  proposal_id TEXT;
BEGIN
  PERFORM drama.assert_operation_claim(p_operation_id,p_claim_token);
  SELECT * INTO task_row FROM drama.invalidation_tasks WHERE operation_id=p_operation_id FOR UPDATE;
  IF task_row.invalidation_task_id IS NULL THEN RAISE EXCEPTION 'invalidation task not found'; END IF;
  SELECT * INTO change_row FROM drama.source_change_sets
    WHERE source_change_set_id=task_row.source_change_set_id FOR UPDATE;
  UPDATE drama.invalidation_tasks SET status='running',started_at=COALESCE(started_at,CURRENT_TIMESTAMP),
    checkpoint=jsonb_build_object('stage','comparing_published_full_ir','changed_chapter_ids',change_row.changed_chapter_ids)
    WHERE invalidation_task_id=task_row.invalidation_task_id;
  UPDATE drama.source_change_sets SET status='running' WHERE source_change_set_id=change_row.source_change_set_id;

  IF NOT EXISTS(SELECT 1 FROM drama.source_change_items WHERE source_change_set_id=change_row.source_change_set_id) THEN
    INSERT INTO drama.source_change_items(source_change_item_id,source_change_set_id,entity_type,change_type,
      before_entity_id,after_entity_id,semantic_fingerprint,details)
    SELECT 'sci_'||substr(encode(drama.digest(change_row.source_change_set_id||'|fact|'||COALESCE(old_fact.fact_id,new_fact.fact_id),'sha256'),'hex'),1,32),
      change_row.source_change_set_id,'fact',
      CASE WHEN old_fact.fact_revision_id IS NULL THEN 'added'
        WHEN new_fact.fact_revision_id IS NULL THEN 'removed'
        WHEN old_fact.canonical_fingerprint=new_fact.canonical_fingerprint THEN 'relocated' ELSE 'changed' END,
      old_fact.fact_revision_id,new_fact.fact_revision_id,
      COALESCE(new_fact.canonical_fingerprint,old_fact.canonical_fingerprint),
      jsonb_build_object('subtype',COALESCE(new_fact.fact_kind,old_fact.fact_kind),
        'logical_fact_id',COALESCE(old_fact.fact_id,new_fact.fact_id),
        'chapter_id',COALESCE(new_fact.chapter_id,old_fact.chapter_id),
        'semantic_changed',old_fact.fact_revision_id IS NULL OR new_fact.fact_revision_id IS NULL
          OR old_fact.canonical_fingerprint<>new_fact.canonical_fingerprint,
        'source_span_changed',old_fact.primary_source_span_id IS DISTINCT FROM new_fact.primary_source_span_id,
        'before_event_revision_id',old_event.event_revision_id,
        'after_event_revision_id',new_event.event_revision_id)
    FROM (SELECT fact.*,logical.fact_kind FROM drama.narrative_fact_revisions fact
      JOIN drama.narrative_facts logical USING(fact_id)
      WHERE fact.ir_revision_id=change_row.from_ir_revision_id
        AND change_row.changed_chapter_ids ? fact.chapter_id) old_fact
    FULL JOIN (SELECT fact.*,logical.fact_kind FROM drama.narrative_fact_revisions fact
      JOIN drama.narrative_facts logical USING(fact_id)
      WHERE fact.ir_revision_id=change_row.to_ir_revision_id
        AND change_row.changed_chapter_ids ? fact.chapter_id) new_fact USING(fact_id)
    LEFT JOIN drama.narrative_event_revisions old_event ON old_event.fact_revision_id=old_fact.fact_revision_id
    LEFT JOIN drama.narrative_event_revisions new_event ON new_event.fact_revision_id=new_fact.fact_revision_id
    WHERE old_fact.fact_revision_id IS NULL OR new_fact.fact_revision_id IS NULL
      OR old_fact.canonical_fingerprint<>new_fact.canonical_fingerprint
      OR old_fact.primary_source_span_id<>new_fact.primary_source_span_id
    ON CONFLICT(source_change_item_id) DO NOTHING;

    INSERT INTO drama.source_change_items(source_change_item_id,source_change_set_id,entity_type,change_type,
      before_entity_id,after_entity_id,details)
    SELECT 'sci_'||substr(encode(drama.digest(change_row.source_change_set_id||'|arc|'||COALESCE(old_arc.story_arc_id,new_arc.story_arc_id),'sha256'),'hex'),1,32),
      change_row.source_change_set_id,'story_arc',
      CASE WHEN old_arc.story_arc_revision_id IS NULL THEN 'added'
        WHEN new_arc.story_arc_revision_id IS NULL THEN 'removed'
        WHEN old_arc.title=new_arc.title AND old_arc.summary=new_arc.summary AND old_arc.arc_type=new_arc.arc_type THEN 'relocated'
        ELSE 'changed' END,
      old_arc.story_arc_revision_id,new_arc.story_arc_revision_id,
      jsonb_build_object('subtype','story_arc','logical_story_arc_id',COALESCE(old_arc.story_arc_id,new_arc.story_arc_id),
        'chapter_id',COALESCE(new_arc.chapter_id,old_arc.chapter_id),
        'semantic_changed',old_arc.story_arc_revision_id IS NULL OR new_arc.story_arc_revision_id IS NULL
          OR old_arc.title<>new_arc.title OR old_arc.summary<>new_arc.summary OR old_arc.arc_type<>new_arc.arc_type,
        'source_span_changed',old_arc.primary_source_span_id IS DISTINCT FROM new_arc.primary_source_span_id)
    FROM (SELECT arc.* FROM drama.story_arc_revisions arc
      WHERE arc.ir_revision_id=change_row.from_ir_revision_id
        AND change_row.changed_chapter_ids ? arc.chapter_id) old_arc
    FULL JOIN (SELECT arc.* FROM drama.story_arc_revisions arc
      WHERE arc.ir_revision_id=change_row.to_ir_revision_id
        AND change_row.changed_chapter_ids ? arc.chapter_id) new_arc USING(story_arc_id)
    WHERE old_arc.story_arc_revision_id IS NULL OR new_arc.story_arc_revision_id IS NULL
      OR old_arc.title<>new_arc.title OR old_arc.summary<>new_arc.summary OR old_arc.arc_type<>new_arc.arc_type
      OR old_arc.primary_source_span_id<>new_arc.primary_source_span_id
    ON CONFLICT(source_change_item_id) DO NOTHING;
  END IF;

  -- Register the immutable base arc revision in lineage without rebuilding it.
  INSERT INTO drama.artifacts(artifact_id,artifact_type,native_entity_id,revision_number,content_hash,
    validity_status,is_current,idempotency_key,metadata)
  SELECT 'art_'||substr(encode(drama.digest('story-arc|'||arc.story_arc_revision_id,'sha256'),'hex'),1,32),
    'story_arc_revision',arc.story_arc_revision_id,ir.revision_number,
    encode(drama.digest(arc.title||'|'||arc.summary||'|'||arc.arc_type,'sha256'),'hex'),'valid',false,
    'impact-story-arc:'||arc.story_arc_revision_id,jsonb_build_object('story_arc_id',arc.story_arc_id)
  FROM drama.source_change_items item JOIN drama.story_arc_revisions arc
    ON arc.story_arc_revision_id=item.before_entity_id
  JOIN drama.narrative_ir_revisions ir USING(ir_revision_id)
  WHERE item.source_change_set_id=change_row.source_change_set_id AND item.entity_type='story_arc'
  ON CONFLICT DO NOTHING;

  WITH RECURSIVE semantic_items AS (
    SELECT * FROM drama.source_change_items item
    WHERE item.source_change_set_id=change_row.source_change_set_id
      AND COALESCE((item.details->>'semantic_changed')::boolean,item.change_type<>'relocated')
  ), changed_old_facts AS (
    SELECT before_entity_id fact_revision_id FROM semantic_items
    WHERE entity_type='fact' AND before_entity_id IS NOT NULL
  ), changed_old_events AS (
    SELECT COALESCE(item.details->>'before_event_revision_id',event.event_revision_id) event_revision_id
    FROM semantic_items item
    LEFT JOIN drama.narrative_event_revisions event ON event.fact_revision_id=item.before_entity_id
    WHERE item.details->>'subtype' IN ('event','character_state','foreshadowing')
      AND COALESCE(item.details->>'before_event_revision_id',event.event_revision_id) IS NOT NULL
    UNION
    SELECT event.event_revision_id
    FROM semantic_items item
    CROSS JOIN LATERAL (VALUES(item.details->'before_value'->>'from_fact_id'),
      (item.details->'before_value'->>'to_fact_id')) endpoint(fact_id)
    JOIN drama.narrative_fact_revisions fact ON fact.fact_id=endpoint.fact_id
      AND fact.ir_revision_id=change_row.from_ir_revision_id
    JOIN drama.narrative_event_revisions event USING(fact_revision_id)
    WHERE item.details->>'subtype'='relation' AND endpoint.fact_id IS NOT NULL
  ), direct AS (
    SELECT artifact.artifact_id,0 depth,jsonb_build_array(artifact.artifact_id) path
    FROM drama.artifacts artifact
    WHERE (artifact.project_id IS NULL OR artifact.project_id=task_row.project_id) AND (
      EXISTS(SELECT 1 FROM drama.artifact_source_evidence evidence
        JOIN changed_old_facts USING(fact_revision_id) WHERE evidence.artifact_id=artifact.artifact_id)
      OR EXISTS(SELECT 1 FROM drama.adaptation_episode_plans episode
        JOIN drama.episode_event_assignments assignment USING(adaptation_episode_plan_id)
        JOIN changed_old_events USING(event_revision_id)
        WHERE artifact.native_entity_id IN (episode.adaptation_episode_plan_id,episode.adaptation_plan_id))
      OR EXISTS(SELECT 1 FROM semantic_items item
        WHERE item.entity_type='story_arc' AND item.before_entity_id=artifact.native_entity_id)
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
      jsonb_build_object('source_change_set_id',change_row.source_change_set_id,'selector','reviewed_semantic_lineage'),
      jsonb_build_array(artifact.artifact_id)
    FROM collapsed JOIN drama.artifacts artifact USING(artifact_id)
    ON CONFLICT(invalidation_task_id,artifact_id) DO NOTHING RETURNING artifact_id
  )
  UPDATE drama.artifacts artifact SET validity_status='stale'
  WHERE artifact.artifact_id IN (SELECT artifact_id FROM collapsed) AND artifact.validity_status<>'stale';

  SELECT count(*) INTO impact_count FROM drama.invalidation_impacts
    WHERE invalidation_task_id=task_row.invalidation_task_id;

  proposal_id:='regenp_'||substr(encode(drama.digest(task_row.project_id||'|'||change_row.source_change_set_id,'sha256'),'hex'),1,32);
  INSERT INTO drama.regeneration_proposals(regeneration_proposal_id,source_change_set_id,project_id,summary)
  VALUES(proposal_id,change_row.source_change_set_id,task_row.project_id,
    jsonb_build_object('affected_artifact_count',impact_count,'auto_rebuild',false,'selection_required',true))
  ON CONFLICT(source_change_set_id,project_id) DO NOTHING;
  INSERT INTO drama.regeneration_proposal_items(regeneration_proposal_item_id,regeneration_proposal_id,artifact_id,reason)
  SELECT 'regenpi_'||substr(encode(drama.digest(proposal_id||'|'||impact.artifact_id,'sha256'),'hex'),1,32),
    proposal_id,impact.artifact_id,impact.reason
  FROM drama.invalidation_impacts impact
  WHERE impact.invalidation_task_id=task_row.invalidation_task_id
  ON CONFLICT(regeneration_proposal_id,artifact_id) DO NOTHING;

  UPDATE drama.invalidation_tasks SET status='needs_review',completed_at=CURRENT_TIMESTAMP,
    checkpoint=jsonb_build_object('stage','finished','affected_artifact_count',impact_count,
      'regeneration_proposal_id',proposal_id,'auto_rebuild',false)
    WHERE invalidation_task_id=task_row.invalidation_task_id;
  UPDATE drama.source_change_sets SET status='needs_review',summary=jsonb_build_object(
    'changed_chapter_ids',change_row.changed_chapter_ids,
    'semantic_change_count',(SELECT count(*) FROM drama.source_change_items item
      WHERE item.source_change_set_id=change_row.source_change_set_id
        AND COALESCE((item.details->>'semantic_changed')::boolean,item.change_type<>'relocated')),
    'relocation_only_count',(SELECT count(*) FROM drama.source_change_items item
      WHERE item.source_change_set_id=change_row.source_change_set_id
        AND NOT COALESCE((item.details->>'semantic_changed')::boolean,item.change_type<>'relocated')),
    'affected_artifact_count',impact_count,'reviewed_artifacts_preserved',true,
    'regeneration_proposal_id',proposal_id,'auto_rebuild',false)
    WHERE source_change_set_id=change_row.source_change_set_id;
  PERFORM drama.finish_operation(p_operation_id,p_claim_token,'needs_review','invalidation_task',task_row.invalidation_task_id);
  RETURN jsonb_build_object('source_change_set_id',change_row.source_change_set_id,
    'invalidation_task_id',task_row.invalidation_task_id,'affected_artifact_count',impact_count,
    'regeneration_proposal_id',proposal_id,'auto_rebuild',false,'status','needs_review');
END $$;

INSERT INTO drama.schema_migrations(version,checksum,description)
VALUES('20','incremental-full-ir-merge-v1-20260801',
  'Reviewed Incremental IR to immutable published Full IR merge closure')
ON CONFLICT(version) DO NOTHING;

\else
\echo 'migration 20 already applied with matching checksum; no-op'
\endif

COMMIT;
