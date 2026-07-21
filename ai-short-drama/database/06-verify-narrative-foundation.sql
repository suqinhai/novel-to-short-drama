\set ON_ERROR_STOP on
SET search_path TO drama, public;

DO $$
DECLARE
  missing_tables TEXT[];
  missing_columns TEXT[];
  bad_count BIGINT;
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM drama.schema_migrations
    WHERE version='06' AND checksum='phase1-contract-v2-20260721'
  ) THEN
    RAISE EXCEPTION 'migration 06 ledger row/checksum missing';
  END IF;

  SELECT array_agg(required_name ORDER BY required_name) INTO missing_tables
  FROM unnest(ARRAY[
    'migration_audit','source_works','source_versions','source_chapters','chapter_revisions','source_version_chapters',
    'source_spans','operations','source_import_jobs','source_import_items','project_source_bindings',
    'legacy_source_bindings','narrative_ir_revisions','narrative_entities',
    'narrative_entity_revisions','narrative_facts','narrative_fact_revisions','fact_evidence',
    'narrative_event_revisions','event_participants','event_relations','character_state_changes',
    'timeline_facts','foreshadow_threads','foreshadow_occurrences','story_arcs','story_arc_revisions',
    'story_arc_events','adaptation_specs','adaptation_spec_versions','adaptation_scope_chapters',
    'adaptation_scope_arcs','adaptation_rules','compiler_runs','compiler_checkpoints',
    'compiler_diagnostics','adaptation_plans','adaptation_episode_plans','episode_event_assignments',
    'artifact_types','artifacts','artifact_dependencies','artifact_source_evidence','source_change_sets',
    'source_change_items','invalidation_tasks','invalidation_impacts'
  ]) AS required(required_name)
  WHERE to_regclass('drama.'||required_name) IS NULL;
  IF missing_tables IS NOT NULL THEN
    RAISE EXCEPTION 'required Phase 1 tables missing: %',missing_tables;
  END IF;

  SELECT array_agg(v.table_name||'.'||v.column_name ORDER BY v.table_name,v.column_name)
  INTO missing_columns
  FROM (VALUES
    ('operations','claim_token'),('operations','claim_request_id'),('operations','heartbeat_at'),
    ('source_spans','chapter_revision_id'),('story_arc_revisions','primary_source_span_id'),
    ('adaptation_spec_versions','source_binding_id'),('compiler_runs','source_version_id'),
    ('artifact_dependencies','observed_upstream_hash'),('invalidation_tasks','operation_id')
  ) AS v(table_name,column_name)
  WHERE NOT EXISTS (
    SELECT 1 FROM information_schema.columns c
    WHERE c.table_schema='drama' AND c.table_name=v.table_name AND c.column_name=v.column_name
  );
  IF missing_columns IS NOT NULL THEN
    RAISE EXCEPTION 'required Phase 1 columns missing: %',missing_columns;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.novels n
  LEFT JOIN drama.legacy_source_bindings l ON l.legacy_novel_id=n.novel_id
  LEFT JOIN drama.source_works w ON w.work_id=l.work_id
  LEFT JOIN drama.source_versions sv ON sv.source_version_id=l.source_version_id
  LEFT JOIN drama.project_source_bindings pb
    ON pb.project_id=n.project_id AND pb.source_version_id=l.source_version_id
  WHERE l.legacy_novel_id IS NULL OR w.work_id IS NULL OR sv.source_version_id IS NULL OR pb.binding_id IS NULL;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'legacy novel backfill incomplete: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.novel_chapters c
  LEFT JOIN drama.source_chapters sc ON sc.chapter_id='sch_legacy_'||c.chapter_id
  LEFT JOIN drama.chapter_revisions cr ON cr.chapter_revision_id='cr_legacy_'||c.chapter_id
  LEFT JOIN drama.source_version_chapters svc ON svc.version_chapter_id='svc_legacy_'||c.chapter_id
  WHERE sc.chapter_id IS NULL OR cr.chapter_revision_id IS NULL OR svc.version_chapter_id IS NULL
     OR cr.content_hash <> CASE WHEN lower(c.content_hash) ~ '^[0-9a-f]{64}$' THEN lower(c.content_hash)
                               ELSE encode(digest(convert_to(c.content,'UTF8'),'sha256'),'hex') END
     OR svc.ordinal <> c.chapter_number;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'legacy chapter backfill incomplete or hash/order mismatch: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.novel_chapters c
  LEFT JOIN drama.source_spans s ON s.source_span_id='span_legacy_full_'||c.chapter_id
  WHERE char_length(c.content)>0 AND (
    s.source_span_id IS NULL OR s.start_codepoint<>0 OR s.end_codepoint<>char_length(c.content)
    OR s.start_utf8_byte<>0 OR s.end_utf8_byte<>octet_length(c.content)
    OR s.excerpt_hash<>CASE WHEN lower(c.content_hash) ~ '^[0-9a-f]{64}$' THEN lower(c.content_hash)
                            ELSE encode(digest(convert_to(c.content,'UTF8'),'sha256'),'hex') END
  );
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'legacy full-chapter span mismatch: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM (
    SELECT project_id,count(*)
    FROM drama.project_source_bindings
    WHERE binding_role='primary' AND is_current
    GROUP BY project_id HAVING count(*)>1
  ) duplicates;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'multiple current primary source bindings: % project(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.source_versions child
  JOIN drama.source_versions parent ON parent.source_version_id=child.parent_source_version_id
  WHERE child.work_id<>parent.work_id;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'source version parent crosses works: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.narrative_fact_revisions fr
  LEFT JOIN drama.source_spans s
    ON s.source_span_id=fr.primary_source_span_id
   AND s.work_id=fr.work_id
   AND s.source_version_id=fr.source_version_id
   AND s.chapter_id=fr.chapter_id
   AND s.chapter_revision_id=fr.primary_chapter_revision_id
  WHERE s.source_span_id IS NULL;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'narrative facts without exact source provenance: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.narrative_entity_revisions er
  LEFT JOIN drama.source_spans s
    ON s.source_span_id=er.primary_source_span_id
   AND s.work_id=er.work_id
   AND s.source_version_id=er.source_version_id
   AND s.chapter_id=er.chapter_id
   AND s.chapter_revision_id=er.primary_chapter_revision_id
  WHERE s.source_span_id IS NULL;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'narrative entities without exact source provenance: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.narrative_event_revisions e
  JOIN drama.narrative_fact_revisions fr ON fr.fact_revision_id=e.fact_revision_id
  JOIN drama.narrative_facts f ON f.fact_id=fr.fact_id
  WHERE f.fact_kind<>'event';
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'event revisions attached to non-event facts: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.narrative_event_revisions ev
  JOIN drama.narrative_entity_revisions er ON er.entity_revision_id=ev.location_entity_revision_id
  JOIN drama.narrative_entities e ON e.entity_id=er.entity_id
  WHERE e.entity_type<>'location';
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'event location references a non-location entity: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.character_state_changes c
  JOIN drama.narrative_fact_revisions fr ON fr.fact_revision_id=c.fact_revision_id
  JOIN drama.narrative_facts f ON f.fact_id=fr.fact_id
  JOIN drama.narrative_entity_revisions er ON er.entity_revision_id=c.character_entity_revision_id
  JOIN drama.narrative_entities e ON e.entity_id=er.entity_id
  WHERE f.fact_kind<>'character_state' OR e.entity_type<>'character';
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'character state typed-reference mismatch: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.timeline_facts t
  JOIN drama.narrative_fact_revisions fr ON fr.fact_revision_id=t.fact_revision_id
  JOIN drama.narrative_facts f ON f.fact_id=fr.fact_id
  WHERE f.fact_kind<>'timeline';
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'timeline rows attached to non-timeline facts: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.fact_evidence fe
  JOIN drama.narrative_fact_revisions fr ON fr.fact_revision_id=fe.fact_revision_id
  JOIN drama.source_spans s ON s.source_span_id=fe.source_span_id
  WHERE s.work_id<>fr.work_id OR s.source_version_id<>fr.source_version_id;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'fact supporting evidence crosses source versions: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.event_participants ep
  JOIN drama.narrative_event_revisions ev ON ev.event_revision_id=ep.event_revision_id
  JOIN drama.narrative_fact_revisions efr ON efr.fact_revision_id=ev.fact_revision_id
  JOIN drama.narrative_entity_revisions er ON er.entity_revision_id=ep.entity_revision_id
  JOIN drama.source_spans s ON s.source_span_id=ep.source_span_id
  WHERE er.ir_revision_id<>efr.ir_revision_id
     OR s.source_version_id<>efr.source_version_id;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'event participant crosses IR/source revisions: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.event_relations rel
  JOIN drama.narrative_event_revisions f ON f.event_revision_id=rel.from_event_revision_id
  JOIN drama.narrative_fact_revisions ffr ON ffr.fact_revision_id=f.fact_revision_id
  JOIN drama.narrative_event_revisions t ON t.event_revision_id=rel.to_event_revision_id
  JOIN drama.narrative_fact_revisions tfr ON tfr.fact_revision_id=t.fact_revision_id
  JOIN drama.source_spans s ON s.source_span_id=rel.source_span_id
  WHERE ffr.ir_revision_id<>tfr.ir_revision_id
     OR s.source_version_id<>ffr.source_version_id;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'event relation crosses IR/source revisions: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.story_arc_events sae
  JOIN drama.story_arc_revisions sar ON sar.story_arc_revision_id=sae.story_arc_revision_id
  JOIN drama.narrative_event_revisions ev ON ev.event_revision_id=sae.event_revision_id
  JOIN drama.narrative_fact_revisions fr ON fr.fact_revision_id=ev.fact_revision_id
  WHERE sar.ir_revision_id<>fr.ir_revision_id;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'story arc event crosses IR revisions: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.adaptation_scope_chapters scp
  JOIN drama.adaptation_spec_versions sp ON sp.adaptation_spec_version_id=scp.adaptation_spec_version_id
  JOIN drama.source_versions sv ON sv.source_version_id=sp.source_version_id
  JOIN drama.source_chapters c ON c.chapter_id=scp.chapter_id
  WHERE c.work_id<>sv.work_id;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'adaptation chapter scope crosses source works: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.adaptation_scope_arcs sap
  JOIN drama.adaptation_spec_versions sp ON sp.adaptation_spec_version_id=sap.adaptation_spec_version_id
  JOIN drama.source_versions sv ON sv.source_version_id=sp.source_version_id
  JOIN drama.story_arc_revisions sar ON sar.story_arc_revision_id=sap.story_arc_revision_id
  JOIN drama.story_arcs sa ON sa.story_arc_id=sar.story_arc_id
  WHERE sa.work_id<>sv.work_id;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'adaptation story-arc scope crosses source works: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.adaptation_spec_versions sp
  JOIN drama.adaptation_specs s ON s.adaptation_spec_id=sp.adaptation_spec_id
  LEFT JOIN drama.project_source_bindings pb
    ON pb.project_id=s.project_id AND pb.source_version_id=sp.source_version_id
  WHERE pb.binding_id IS NULL;
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'adaptation spec source is not a project binding: % row(s)',bad_count;
  END IF;

  SELECT count(*) INTO bad_count
  FROM drama.compiler_runs cr
  JOIN drama.adaptation_spec_versions sp ON sp.adaptation_spec_version_id=cr.adaptation_spec_version_id
  JOIN drama.adaptation_specs s ON s.adaptation_spec_id=sp.adaptation_spec_id
  JOIN drama.narrative_ir_revisions ir ON ir.ir_revision_id=cr.ir_revision_id
  WHERE s.project_id<>cr.project_id OR sp.source_version_id<>ir.source_version_id
     OR (sp.ir_revision_id IS NOT NULL AND sp.ir_revision_id<>cr.ir_revision_id);
  IF bad_count <> 0 THEN
    RAISE EXCEPTION 'compiler run input contract mismatch: % row(s)',bad_count;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname='drama' AND indexname='uq_project_primary_source_current'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname='drama' AND indexname='idx_operations_claim'
  ) THEN
    RAISE EXCEPTION 'required Phase 1 indexes missing';
  END IF;

  IF to_regprocedure('drama.claim_operation(text,text,text[],integer)') IS NULL
     OR to_regprocedure('drama.heartbeat_operation(text,uuid,integer)') IS NULL
     OR to_regprocedure('drama.assert_operation_claim(text,uuid)') IS NULL
     OR to_regprocedure('drama.checkpoint_operation(text,uuid,text,text,text,jsonb)') IS NULL
     OR to_regprocedure('drama.finish_operation(text,uuid,text,text,text,text,text,boolean)') IS NULL THEN
    RAISE EXCEPTION 'required operation lease functions missing';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.source_versions'::regclass AND tgname='trg_source_versions_immutable'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.novel_chapters'::regclass AND tgname='trg_novel_chapters_updated'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.source_version_chapters'::regclass AND tgname='trg_source_version_chapters_immutable'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.artifacts'::regclass AND tgname='trg_artifacts_revision_immutable'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.adaptation_rules'::regclass AND tgname='trg_adaptation_rule_target'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.adaptation_spec_versions'::regclass AND tgname='trg_adaptation_spec_activation'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.narrative_ir_revisions'::regclass AND tgname='trg_ir_source_snapshot'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.compiler_runs'::regclass AND tgname='trg_compiler_frozen_inputs'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.narrative_ir_revisions'::regclass AND tgname='trg_narrative_ir_revisions_immutable'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.adaptation_spec_versions'::regclass AND tgname='trg_adaptation_spec_version_immutable'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.episode_event_assignments'::regclass AND tgname='trg_episode_event_assignment_input'
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgrelid='drama.adaptation_episode_plans'::regclass AND tgname='trg_episode_plan_parent_immutable'
  ) THEN
    RAISE EXCEPTION 'required Phase 1 triggers missing';
  END IF;
END $$;

SELECT 'phase1_verification' AS check_name,'PASS' AS result;
SELECT 'source_works' AS entity,count(*) AS row_count FROM drama.source_works
UNION ALL SELECT 'source_versions',count(*) FROM drama.source_versions
UNION ALL SELECT 'source_chapters',count(*) FROM drama.source_chapters
UNION ALL SELECT 'chapter_revisions',count(*) FROM drama.chapter_revisions
UNION ALL SELECT 'source_spans',count(*) FROM drama.source_spans
UNION ALL SELECT 'operations',count(*) FROM drama.operations
UNION ALL SELECT 'narrative_fact_revisions',count(*) FROM drama.narrative_fact_revisions
UNION ALL SELECT 'adaptation_spec_versions',count(*) FROM drama.adaptation_spec_versions
UNION ALL SELECT 'artifacts',count(*) FROM drama.artifacts
UNION ALL SELECT 'invalidation_tasks',count(*) FROM drama.invalidation_tasks
ORDER BY entity;
