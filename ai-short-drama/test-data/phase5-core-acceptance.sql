\set ON_ERROR_STOP on
SET search_path TO drama,public;
BEGIN;

INSERT INTO source_works(work_id,title,idempotency_key) VALUES('work_phase5_core','Phase 5 core','phase5:core:work');
INSERT INTO source_versions(source_version_id,work_id,version_number,status,is_current,version_hash,normalization_version,
  total_chars,chapter_count,idempotency_key,published_at)
VALUES
 ('sv_phase5_core_v1','work_phase5_core',1,'draft',false,repeat('1',64),'phase5-v1',4,1,'phase5:core:v1',NULL),
 ('sv_phase5_core_v2','work_phase5_core',2,'draft',false,repeat('2',64),'phase5-v1',4,1,'phase5:core:v2',NULL);
INSERT INTO source_chapters(chapter_id,work_id,canonical_key) VALUES('chapter_phase5_core','work_phase5_core','chapter:1');
INSERT INTO chapter_revisions(chapter_revision_id,work_id,chapter_id,revision_number,title,content,content_hash,char_count,idempotency_key)
VALUES('chapter_revision_phase5_core','work_phase5_core','chapter_phase5_core',1,'第一章','甲乙丙丁',
  encode(digest(convert_to('甲乙丙丁','UTF8'),'sha256'),'hex'),4,'phase5:core:chapter-revision');
INSERT INTO source_version_chapters(version_chapter_id,work_id,source_version_id,chapter_id,chapter_revision_id,ordinal,idempotency_key)
VALUES
 ('svc_phase5_core_v1','work_phase5_core','sv_phase5_core_v1','chapter_phase5_core','chapter_revision_phase5_core',1,'phase5:core:svc:v1'),
 ('svc_phase5_core_v2','work_phase5_core','sv_phase5_core_v2','chapter_phase5_core','chapter_revision_phase5_core',1,'phase5:core:svc:v2');
UPDATE source_versions SET status='published',published_at=now(),is_current=(source_version_id='sv_phase5_core_v2')
WHERE source_version_id IN ('sv_phase5_core_v1','sv_phase5_core_v2');

INSERT INTO source_spans(source_span_id,work_id,source_version_id,chapter_id,chapter_revision_id,start_utf8_byte,end_utf8_byte,
  start_codepoint,end_codepoint,excerpt_hash,evidence_text,idempotency_key)
VALUES('span_phase5_core','work_phase5_core','sv_phase5_core_v2','chapter_phase5_core','chapter_revision_phase5_core',0,6,0,2,
  encode(digest(convert_to('甲乙','UTF8'),'sha256'),'hex'),'甲乙','phase5:core:span');

DO $$ BEGIN
  BEGIN
    INSERT INTO source_spans(source_span_id,work_id,source_version_id,chapter_id,chapter_revision_id,start_utf8_byte,end_utf8_byte,
      start_codepoint,end_codepoint,excerpt_hash,evidence_text,idempotency_key)
    VALUES('span_phase5_bad','work_phase5_core','sv_phase5_core_v2','chapter_phase5_core','chapter_revision_phase5_core',0,99,0,2,
      repeat('0',64),'错误','phase5:core:span:bad');
    RAISE EXCEPTION 'invalid source span was accepted';
  EXCEPTION WHEN OTHERS THEN
    IF SQLERRM='invalid source span was accepted' THEN RAISE; END IF;
  END;
END $$;

INSERT INTO projects(project_id,novel_name,target_episode_count,episode_duration_seconds,visual_style,aspect_ratio,target_platform,test_mode,display_name)
VALUES('project_phase5_a','Phase5 A',12,90,'mock','9:16','mock',true,'Phase5 A'),
      ('project_phase5_b','Phase5 B',24,120,'mock','9:16','mock',true,'Phase5 B');
INSERT INTO project_source_bindings(binding_id,project_id,work_id,source_version_id,binding_role,is_current,idempotency_key)
VALUES('binding_phase5_a','project_phase5_a','work_phase5_core','sv_phase5_core_v1','primary',true,'phase5:core:binding:a'),
      ('binding_phase5_b','project_phase5_b','work_phase5_core','sv_phase5_core_v2','primary',true,'phase5:core:binding:b');
DELETE FROM projects WHERE project_id='project_phase5_a';
DO $$ BEGIN
  IF NOT EXISTS(SELECT 1 FROM source_works WHERE work_id='work_phase5_core') OR
     NOT EXISTS(SELECT 1 FROM project_source_bindings WHERE project_id='project_phase5_b') THEN
    RAISE EXCEPTION 'project deletion crossed the source/project isolation boundary';
  END IF;
END $$;

INSERT INTO operations(operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash,
  checkpoint_stage,claim_token,claim_request_id,lease_owner,lease_expires_at,retry_count,max_retries,started_at)
VALUES('operation_phase5_expired','trace_phase5_expired','source_import','source_version','sv_phase5_core_v2','running',
  'phase5:core:expired',repeat('9',64),'window','11111111-1111-4111-8111-111111111111','phase5-expired-claim-1','phase5-test',
  now()-interval '1 minute',0,1,now()-interval '2 minutes');
SELECT * FROM recover_expired_operations(10);
DO $$ BEGIN IF (SELECT status FROM operations WHERE operation_id='operation_phase5_expired')<>'pending' THEN
  RAISE EXCEPTION 'expired operation was not requeued'; END IF; END $$;
UPDATE operations SET status='running',claim_token='22222222-2222-4222-8222-222222222222',claim_request_id='phase5-expired-claim-2',
  lease_owner='phase5-test',lease_expires_at=now()-interval '1 minute' WHERE operation_id='operation_phase5_expired';
SELECT * FROM recover_expired_operations(10);
DO $$ BEGIN IF (SELECT status FROM operations WHERE operation_id='operation_phase5_expired')<>'failed' THEN
  RAISE EXCEPTION 'retry-exhausted operation was not terminated'; END IF; END $$;

DO $$ DECLARE bad_count INTEGER; BEGIN
  SELECT count(*) INTO bad_count FROM artifact_dependencies dependency
    LEFT JOIN artifacts upstream ON upstream.artifact_id=dependency.upstream_artifact_id
    LEFT JOIN artifacts downstream ON downstream.artifact_id=dependency.downstream_artifact_id
    WHERE upstream.artifact_id IS NULL OR downstream.artifact_id IS NULL;
  IF bad_count<>0 THEN RAISE EXCEPTION 'orphan artifact dependencies: %',bad_count; END IF;
END $$;

EXPLAIN (ANALYZE,BUFFERS,FORMAT TEXT)
SELECT membership.chapter_id,membership.ordinal,revision.title,revision.content_hash
FROM source_version_chapters membership JOIN chapter_revisions revision USING(chapter_revision_id)
WHERE membership.source_version_id='sv_phase5_core_v2' ORDER BY membership.ordinal LIMIT 100;

ROLLBACK;
SELECT 'PASS Phase 5 core relations, exact source span, isolation, recovery and query plan' AS result;
