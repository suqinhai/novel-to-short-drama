\set ON_ERROR_STOP on
SET search_path TO drama, public;

BEGIN;
INSERT INTO drama.source_versions(source_version_id,work_id,version_number,parent_source_version_id,status,is_current,
  version_hash,normalization_version,total_chars,chapter_count,resource_revision,idempotency_key,published_at)
VALUES('sv_phase4_revision','sw_legacy_novel_phase1_legacy',2,'sv_legacy_novel_phase1_legacy','draft',false,
  repeat('a',64),'utf8-nfc-lf-v1',26,2,1,'fixture:phase4:source-version',NULL);

INSERT INTO drama.chapter_revisions(chapter_revision_id,work_id,chapter_id,revision_number,title,content,
  content_hash,char_count,idempotency_key)
VALUES('cr_phase4_chapter_001','sw_legacy_novel_phase1_legacy','sch_legacy_ch_phase1_legacy_001',2,
  '第一章（修订）','林夏推开门，发现门后站着陌生人。',repeat('b',64),17,'fixture:phase4:chapter-revision');

INSERT INTO drama.source_version_chapters(version_chapter_id,work_id,source_version_id,chapter_id,chapter_revision_id,ordinal,idempotency_key)
SELECT 'svc_phase4_001','sw_legacy_novel_phase1_legacy','sv_phase4_revision','sch_legacy_ch_phase1_legacy_001',
  'cr_phase4_chapter_001',1,'fixture:phase4:membership:1'
UNION ALL
SELECT 'svc_phase4_002','sw_legacy_novel_phase1_legacy','sv_phase4_revision','sch_legacy_ch_phase1_legacy_002',
  'cr_legacy_ch_phase1_legacy_002',2,'fixture:phase4:membership:2';

UPDATE drama.source_versions SET status='superseded',is_current=false
WHERE source_version_id='sv_legacy_novel_phase1_legacy';
UPDATE drama.source_versions SET status='published',is_current=true,published_at=CURRENT_TIMESTAMP
WHERE source_version_id='sv_phase4_revision';

INSERT INTO drama.source_spans(source_span_id,work_id,source_version_id,chapter_id,chapter_revision_id,
  start_utf8_byte,end_utf8_byte,start_codepoint,end_codepoint,start_paragraph,end_paragraph,excerpt_hash,evidence_text,idempotency_key)
SELECT 'span_phase4_chapter_001','sw_legacy_novel_phase1_legacy','sv_phase4_revision','sch_legacy_ch_phase1_legacy_001',
  chapter_revision_id,0,octet_length(content),0,char_length(content),1,1,
  encode(digest(convert_to(content,'UTF8'),'sha256'),'hex'),content,'fixture:phase4:span'
FROM drama.chapter_revisions WHERE chapter_revision_id='cr_phase4_chapter_001';

INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash,checkpoint_stage)
VALUES('operation_phase4_ir','trace_phase4_ir','ir_extraction','ir_revision','ir_phase4_incremental','pending',
  'fixture:phase4:ir-operation',repeat('d',64),'ready_for_reconcile');
UPDATE drama.operations SET status='running',claim_token='11111111-1111-4111-8111-111111111111',lease_owner='fixture',
  lease_expires_at=CURRENT_TIMESTAMP+interval '10 minutes' WHERE operation_id='operation_phase4_ir';

INSERT INTO drama.narrative_ir_revisions(ir_revision_id,operation_id,work_id,source_version_id,revision_number,
  schema_version,extractor_version,status,is_current,input_hash,output_hash,idempotency_key,validation_summary,
  revision_scope,base_ir_revision_id,changed_chapter_ids)
VALUES('ir_phase4_incremental','operation_phase4_ir','sw_legacy_novel_phase1_legacy','sv_phase4_revision',1,
  'narrative-extraction.v1','fixture-v1','staging',false,repeat('d',64),repeat('e',64),
  'fixture:phase4:ir','{"schema_valid":true}','incremental','ir_phase1_001','["sch_legacy_ch_phase1_legacy_001"]');

INSERT INTO drama.narrative_entity_revisions(entity_revision_id,entity_id,ir_revision_id,work_id,source_version_id,
  chapter_id,primary_chapter_revision_id,primary_source_span_id,canonical_name,attributes,confidence,validation_status,idempotency_key)
VALUES('entity_revision_phase4_hero','entity_phase1_hero','ir_phase4_incremental','sw_legacy_novel_phase1_legacy',
  'sv_phase4_revision','sch_legacy_ch_phase1_legacy_001','cr_phase4_chapter_001','span_phase4_chapter_001',
  '林夏','{"identity":"记者"}',0.99,'valid','fixture:phase4:entity');

INSERT INTO drama.narrative_fact_revisions(fact_revision_id,fact_id,ir_revision_id,work_id,source_version_id,chapter_id,
  primary_chapter_revision_id,primary_source_span_id,canonical_fingerprint,confidence,payload,validation_status,idempotency_key)
VALUES
  ('fact_revision_phase4_event_001','fact_phase1_event_001','ir_phase4_incremental','sw_legacy_novel_phase1_legacy',
   'sv_phase4_revision','sch_legacy_ch_phase1_legacy_001','cr_phase4_chapter_001','span_phase4_chapter_001',repeat('1',64),0.98,
   '{"statement":"林夏推开门后遇见陌生人"}','valid','fixture:phase4:event-fact'),
  ('fact_revision_phase4_state_001','fact_phase1_state_001','ir_phase4_incremental','sw_legacy_novel_phase1_legacy',
   'sv_phase4_revision','sch_legacy_ch_phase1_legacy_001','cr_phase4_chapter_001','span_phase4_chapter_001',repeat('2',64),0.96,
   '{"statement":"林夏从平静转为恐惧"}','valid','fixture:phase4:state-fact');

INSERT INTO drama.fact_evidence(fact_evidence_id,fact_revision_id,ir_revision_id,work_id,source_version_id,source_span_id,
  evidence_role,confidence,idempotency_key)
VALUES
  ('evidence_phase4_event','fact_revision_phase4_event_001','ir_phase4_incremental','sw_legacy_novel_phase1_legacy',
   'sv_phase4_revision','span_phase4_chapter_001','primary',0.98,'fixture:phase4:event-evidence'),
  ('evidence_phase4_state','fact_revision_phase4_state_001','ir_phase4_incremental','sw_legacy_novel_phase1_legacy',
   'sv_phase4_revision','span_phase4_chapter_001','primary',0.96,'fixture:phase4:state-evidence');

INSERT INTO drama.narrative_event_revisions(event_revision_id,fact_revision_id,ir_revision_id,work_id,source_version_id,
  event_type,summary,narrative_order,importance)
VALUES('event_revision_phase4_001','fact_revision_phase4_event_001','ir_phase4_incremental',
  'sw_legacy_novel_phase1_legacy','sv_phase4_revision','encounter','林夏开门后遇见陌生人',1,0.9);

INSERT INTO drama.character_state_changes(state_change_id,fact_revision_id,character_entity_revision_id,state_dimension,
  ir_revision_id,work_id,source_version_id,before_state,after_state,trigger_event_revision_id,sequence_number)
VALUES('state_change_phase4_001','fact_revision_phase4_state_001','entity_revision_phase4_hero','alertness',
  'ir_phase4_incremental','sw_legacy_novel_phase1_legacy','sv_phase4_revision','{"value":"calm"}',
  '{"value":"afraid"}','event_revision_phase4_001',1);

INSERT INTO drama.story_arc_revisions(story_arc_revision_id,story_arc_id,ir_revision_id,work_id,source_version_id,
  chapter_id,primary_chapter_revision_id,primary_source_span_id,title,summary,arc_type,confidence,idempotency_key)
VALUES('story_arc_revision_phase4_001','story_arc_phase1_001','ir_phase4_incremental','sw_legacy_novel_phase1_legacy',
  'sv_phase4_revision','sch_legacy_ch_phase1_legacy_001','cr_phase4_chapter_001','span_phase4_chapter_001',
  '调查开始','林夏遭遇陌生人，调查被迫提前。','main',0.97,'fixture:phase4:arc');

INSERT INTO drama.story_arc_events(story_arc_event_id,story_arc_revision_id,event_revision_id,ir_revision_id,work_id,
  source_version_id,event_ordinal,arc_role,idempotency_key)
VALUES('story_arc_event_phase4_001','story_arc_revision_phase4_001','event_revision_phase4_001','ir_phase4_incremental',
  'sw_legacy_novel_phase1_legacy','sv_phase4_revision',1,'setup','fixture:phase4:arc-event');

UPDATE drama.episode_outlines SET source_chapter_ids='["sch_legacy_ch_phase1_legacy_001"]'
WHERE episode_id='ep_phase1_legacy_001';
INSERT INTO drama.episode_scripts(script_id,project_id,season_id,episode_id,version,title,estimated_duration_seconds,
  source_outline_version,status)
VALUES('script_phase4_approved','p_phase1_legacy','season_phase1_legacy','ep_phase1_legacy_001',1,
  '门后的线索',90,1,'approved');

UPDATE drama.narrative_ir_revisions SET status='published',published_at=CURRENT_TIMESTAMP,is_current=true
WHERE ir_revision_id='ir_phase4_incremental';
SELECT * FROM drama.finish_operation('operation_phase4_ir','11111111-1111-4111-8111-111111111111',
  'completed','ir_revision','ir_phase4_incremental');
COMMIT;

DO $$
DECLARE claimed drama.operations%ROWTYPE;
DECLARE result JSONB;
BEGIN
  SELECT * INTO claimed FROM drama.claim_operation('phase4-test-worker','phase4-test-claim',
    ARRAY['invalidation_scan']::text[],300);
  IF claimed.operation_id IS NULL THEN RAISE EXCEPTION 'impact operation was not enqueued'; END IF;
  SELECT drama.analyze_chapter_impact(claimed.operation_id,claimed.claim_token) INTO result;
  IF result->>'status'<>'needs_review' THEN RAISE EXCEPTION 'unexpected impact result: %',result; END IF;
END $$;

DO $$
DECLARE missing TEXT;
BEGIN
  IF (SELECT is_current FROM drama.narrative_ir_revisions WHERE ir_revision_id='ir_phase4_incremental') THEN
    RAISE EXCEPTION 'incremental IR became current';
  END IF;
  IF NOT (SELECT is_current FROM drama.narrative_ir_revisions WHERE ir_revision_id='ir_phase1_001') THEN
    RAISE EXCEPTION 'base full IR lost current status';
  END IF;
  IF (SELECT count(*) FROM drama.source_change_items WHERE details->>'subtype'='event')<>1 THEN
    RAISE EXCEPTION 'event diff count mismatch';
  END IF;
  IF (SELECT count(*) FROM drama.source_change_items WHERE details->>'subtype'='character_state')<>1 THEN
    RAISE EXCEPTION 'character state diff count mismatch';
  END IF;
  SELECT string_agg(required_type,',') INTO missing
  FROM unnest(ARRAY['story_arc_revision','adaptation_episode_plan','episode_outline','episode_script']) required_type
  WHERE NOT EXISTS(SELECT 1 FROM drama.artifacts WHERE artifact_type=required_type AND validity_status='stale');
  IF missing IS NOT NULL THEN RAISE EXCEPTION 'missing stale artifact types: %',missing; END IF;
  IF (SELECT status FROM drama.episode_outlines WHERE episode_id='ep_phase1_legacy_001')<>'approved'
    OR (SELECT status FROM drama.episode_scripts WHERE script_id='script_phase4_approved')<>'approved'
    OR (SELECT status FROM drama.adaptation_plans WHERE adaptation_plan_id='adaptation_plan_phase1_001')<>'approved' THEN
    RAISE EXCEPTION 'reviewed domain artifacts were overwritten';
  END IF;
END $$;

SELECT 'PASS' result,
  (SELECT count(*) FROM drama.source_change_items) changed_items,
  (SELECT count(*) FROM drama.artifacts WHERE validity_status='stale') stale_artifacts,
  (SELECT status FROM drama.source_change_sets WHERE to_source_version_id='sv_phase4_revision') impact_status;
