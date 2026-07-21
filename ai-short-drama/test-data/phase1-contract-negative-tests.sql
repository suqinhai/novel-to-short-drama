\set ON_ERROR_STOP on
BEGIN;
SET search_path TO drama, public;

-- A version remains sealed after published -> superseded.
UPDATE drama.source_versions
SET status='superseded'
WHERE source_version_id='sv_legacy_novel_phase1_legacy';
DO $$
BEGIN
  BEGIN
    UPDATE drama.source_versions
    SET version_hash=repeat('0',64)
    WHERE source_version_id='sv_legacy_novel_phase1_legacy';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='sealed source version mutation was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

-- Activated specs cannot be removed by deleting an immediate parent.  Only the
-- legacy-compatible project deletion path at the end of this test may cascade.
DO $$
BEGIN
  BEGIN
    DELETE FROM drama.adaptation_specs
    WHERE adaptation_spec_id='adaptation_spec_phase1_001';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='active spec was deleted through its spec parent';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
  BEGIN
    DELETE FROM drama.project_source_bindings
    WHERE binding_id='psb_legacy_novel_phase1_legacy';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='active spec was deleted through its source binding parent';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
  BEGIN
    DELETE FROM drama.operations
    WHERE operation_id='operation_phase1_spec_001';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='active spec was deleted through its operation parent';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

DO $$
BEGIN
  BEGIN
    UPDATE drama.operations
    SET checkpoint_data='{"provider":{"response_body":"forbidden"}}'::jsonb
    WHERE operation_id='operation_phase1_compile_001';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='nested provider payload was accepted';
  EXCEPTION WHEN check_violation THEN NULL;
  END;
END $$;

DO $$
BEGIN
  BEGIN
    UPDATE drama.narrative_event_revisions SET summary='mutated'
    WHERE event_revision_id='event_revision_phase1_001';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='published IR child mutation was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
  BEGIN
    UPDATE drama.adaptation_spec_versions SET platform='mutated'
    WHERE adaptation_spec_version_id='adaptation_spec_version_phase1_001';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='active spec mutation was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
  BEGIN
    UPDATE drama.adaptation_rules SET rationale='mutated'
    WHERE adaptation_rule_id='adaptation_rule_phase1_001';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='active spec rule mutation was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

-- New chapter membership cannot be attached to an already sealed version.
INSERT INTO drama.source_chapters(chapter_id,work_id,canonical_key)
VALUES('sch_phase1_negative','sw_legacy_novel_phase1_legacy','negative:membership');
INSERT INTO drama.chapter_revisions(
  chapter_revision_id,work_id,chapter_id,revision_number,title,content,content_hash,
  char_count,idempotency_key
) VALUES(
  'cr_phase1_negative','sw_legacy_novel_phase1_legacy','sch_phase1_negative',1,
  'negative','x',repeat('0',64),1,'negative:chapter-revision'
);
DO $$
BEGIN
  BEGIN
    INSERT INTO drama.source_version_chapters(
      version_chapter_id,work_id,source_version_id,chapter_id,chapter_revision_id,
      ordinal,idempotency_key
    ) VALUES(
      'svc_phase1_negative','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
      'sch_phase1_negative','cr_phase1_negative',99,'negative:membership'
    );
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='sealed source membership insert was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

-- Evidence locators are append-only after publication: extraction may add one,
-- but it cannot edit or delete it later.
INSERT INTO drama.source_spans(
  source_span_id,work_id,source_version_id,chapter_id,chapter_revision_id,
  start_utf8_byte,end_utf8_byte,start_codepoint,end_codepoint,excerpt_hash,
  idempotency_key
) VALUES(
  'span_phase1_append_only','sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
  'sch_legacy_ch_phase1_legacy_001','cr_legacy_ch_phase1_legacy_001',0,3,0,1,
  repeat('0',64),'negative:append-only-span'
);
DO $$
BEGIN
  BEGIN
    UPDATE drama.source_spans SET end_codepoint=2
    WHERE source_span_id='span_phase1_append_only';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='sealed source span update was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

DO $$
BEGIN
  BEGIN
    UPDATE drama.artifacts SET content_hash=repeat('0',64)
    WHERE artifact_id='artifact_phase1_episode_plan_001';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='artifact revision mutation was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

DO $$
BEGIN
  BEGIN
    INSERT INTO drama.adaptation_rules(
      adaptation_rule_id,adaptation_spec_version_id,rule_type,enforcement,target_type,
      target_id,priority,parameters,idempotency_key
    ) VALUES(
      'adaptation_rule_phase1_cross_source','adaptation_spec_version_phase1_001',
      'must_preserve','hard','event','event_revision_missing',100,'{}','negative:rule-target'
    );
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='out-of-scope adaptation rule target was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash
) VALUES(
  'operation_phase1_invalid_spec','trace_phase1_invalid_spec','spec_validation',
  'adaptation_spec_version','adaptation_spec_version_phase1_invalid','pending',
  'negative:invalid-spec-operation',repeat('0',64)
);
INSERT INTO drama.adaptation_spec_versions(
  adaptation_spec_version_id,operation_id,adaptation_spec_id,project_id,source_binding_id,
  work_id,version_number,source_version_id,ir_revision_id,status,platform,audience_profile,
  target_episode_count,episode_duration_seconds,scope_mode,content_hash,idempotency_key
) VALUES(
  'adaptation_spec_version_phase1_invalid','operation_phase1_invalid_spec',
  'adaptation_spec_phase1_001','p_phase1_legacy','psb_legacy_novel_phase1_legacy',
  'sw_legacy_novel_phase1_legacy',2,'sv_legacy_novel_phase1_legacy','ir_phase1_001',
  'draft','test','{}',1,60,'union',repeat('0',64),'negative:invalid-spec-version'
);
DO $$
BEGIN
  BEGIN
    UPDATE drama.adaptation_spec_versions SET status='active',activated_at=now()
    WHERE adaptation_spec_version_id='adaptation_spec_version_phase1_invalid';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='empty adaptation spec was activated';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,
  input_hash,checkpoint_stage,result_type,result_id,completed_at
) VALUES(
  'operation_phase1_ir_cross','trace_phase1_ir_cross','ir_extraction','source_version',
  'sv_legacy_novel_phase1_legacy','completed','negative:operation:ir-cross',repeat('0',64),
  'completed','ir_revision','ir_phase1_cross',now()
);
INSERT INTO drama.narrative_ir_revisions(
  ir_revision_id,operation_id,work_id,source_version_id,revision_number,schema_version,
  extractor_version,status,is_current,input_hash,output_hash,idempotency_key
) VALUES(
  'ir_phase1_cross','operation_phase1_ir_cross','sw_legacy_novel_phase1_legacy',
  'sv_legacy_novel_phase1_legacy',2,'narrative-extraction.v1','negative-v1','staging',false,
  repeat('0',64),repeat('1',64),'negative:ir-cross'
);
INSERT INTO drama.narrative_facts(fact_id,work_id,fact_kind,stable_key)
VALUES('fact_phase1_cross','sw_legacy_novel_phase1_legacy','event','negative:event-cross');
INSERT INTO drama.narrative_fact_revisions(
  fact_revision_id,fact_id,ir_revision_id,work_id,source_version_id,chapter_id,
  primary_chapter_revision_id,primary_source_span_id,canonical_fingerprint,
  confidence,validation_status,idempotency_key
) VALUES(
  'fact_revision_phase1_cross','fact_phase1_cross','ir_phase1_cross',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
  'sch_legacy_ch_phase1_legacy_001','cr_legacy_ch_phase1_legacy_001',
  'span_legacy_full_ch_phase1_legacy_001',repeat('0',64),1,'valid','negative:fact-cross'
);
INSERT INTO drama.narrative_event_revisions(
  event_revision_id,fact_revision_id,ir_revision_id,work_id,source_version_id,
  event_type,summary,narrative_order
) VALUES(
  'event_revision_phase1_cross','fact_revision_phase1_cross','ir_phase1_cross',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','negative','cross IR event',99
);
UPDATE drama.narrative_ir_revisions SET status='published',published_at=now()
WHERE ir_revision_id='ir_phase1_cross';

-- Build a second, valid compiler input on the other IR so reparenting the
-- already-assigned episode plan would create an actual cross-IR state.
INSERT INTO drama.adaptation_specs(
  adaptation_spec_id,project_id,display_name,is_current,idempotency_key
) VALUES(
  'adaptation_spec_phase1_cross','p_phase1_legacy','Negative cross-IR spec',false,
  'negative:spec-cross'
);
INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash
) VALUES(
  'operation_phase1_spec_cross','trace_phase1_spec_cross','spec_validation',
  'adaptation_spec_version','adaptation_spec_version_phase1_cross','pending',
  'negative:operation:spec-cross',repeat('2',64)
);
INSERT INTO drama.adaptation_spec_versions(
  adaptation_spec_version_id,operation_id,adaptation_spec_id,project_id,source_binding_id,
  work_id,version_number,source_version_id,ir_revision_id,status,platform,audience_profile,
  target_episode_count,episode_duration_seconds,scope_mode,content_hash,idempotency_key
) VALUES(
  'adaptation_spec_version_phase1_cross','operation_phase1_spec_cross',
  'adaptation_spec_phase1_cross','p_phase1_legacy','psb_legacy_novel_phase1_legacy',
  'sw_legacy_novel_phase1_legacy',1,'sv_legacy_novel_phase1_legacy','ir_phase1_cross',
  'draft','test','{}',1,60,'chapters_only',repeat('2',64),'negative:spec-version-cross'
);
INSERT INTO drama.adaptation_scope_chapters(
  scope_chapter_id,adaptation_spec_version_id,project_id,work_id,source_version_id,
  ir_revision_id,chapter_id,include_mode
) VALUES(
  'scope_chapter_phase1_cross','adaptation_spec_version_phase1_cross','p_phase1_legacy',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy','ir_phase1_cross',
  'sch_legacy_ch_phase1_legacy_001','include'
);
INSERT INTO drama.adaptation_rules(
  adaptation_rule_id,adaptation_spec_version_id,rule_type,enforcement,target_type,
  target_id,priority,parameters,rationale,idempotency_key
) VALUES(
  'adaptation_rule_phase1_cross','adaptation_spec_version_phase1_cross','must_preserve',
  'hard','chapter','sch_legacy_ch_phase1_legacy_001',100,'{}','negative cross IR plan',
  'negative:rule-cross'
);
UPDATE drama.adaptation_spec_versions SET status='active',activated_at=now()
WHERE adaptation_spec_version_id='adaptation_spec_version_phase1_cross';
INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash
) VALUES(
  'operation_phase1_compile_cross','trace_phase1_compile_cross','adaptation_compile',
  'adaptation_spec_version','adaptation_spec_version_phase1_cross','pending',
  'negative:operation:compile-cross',repeat('3',64)
);
INSERT INTO drama.compiler_runs(
  compiler_run_id,operation_id,project_id,work_id,source_version_id,
  adaptation_spec_version_id,ir_revision_id,compiler_version,status,input_hash,idempotency_key
) VALUES(
  'compiler_run_phase1_cross','operation_phase1_compile_cross','p_phase1_legacy',
  'sw_legacy_novel_phase1_legacy','sv_legacy_novel_phase1_legacy',
  'adaptation_spec_version_phase1_cross','ir_phase1_cross','negative-v1','pending',
  repeat('3',64),'negative:compiler-run-cross'
);
INSERT INTO drama.adaptation_plans(
  adaptation_plan_id,compiler_run_id,project_id,adaptation_spec_version_id,
  version_number,status,is_current,content_hash
) VALUES(
  'adaptation_plan_phase1_cross','compiler_run_phase1_cross','p_phase1_legacy',
  'adaptation_spec_version_phase1_cross',2,'draft',false,repeat('4',64)
);
DO $$
BEGIN
  BEGIN
    UPDATE drama.adaptation_episode_plans
    SET adaptation_plan_id='adaptation_plan_phase1_cross'
    WHERE adaptation_episode_plan_id='adaptation_episode_plan_phase1_001';
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='assigned episode plan was reparented across IR inputs';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

DO $$
BEGIN
  BEGIN
    INSERT INTO drama.episode_event_assignments(
      episode_event_assignment_id,adaptation_episode_plan_id,event_revision_id,
      sequence_number,usage_mode,idempotency_key
    ) VALUES(
      'assignment_phase1_cross','adaptation_episode_plan_phase1_001',
      'event_revision_phase1_cross',3,'preserve','negative:assignment-cross'
    );
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='cross-IR episode event assignment was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN NULL;
  END;
END $$;

-- Claim/heartbeat/takeover is token-fenced and retry-bounded.
INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,
  input_hash,max_retries
) VALUES(
  'operation_phase1_lease_test','trace_phase1_lease_test','source_import','source_version',
  'sv_legacy_novel_phase1_legacy','pending','negative:lease-operation',repeat('0',64),1
);
DO $$
DECLARE first_claim UUID; second_claim UUID; row_count INTEGER;
BEGIN
  SELECT claim_token INTO first_claim
  FROM drama.claim_operation('worker-a','claim-request-a',ARRAY['source_import'],60);
  IF first_claim IS NULL THEN RAISE EXCEPTION 'initial operation claim failed'; END IF;

  SELECT count(*) INTO row_count
  FROM drama.heartbeat_operation(
    'operation_phase1_lease_test','00000000-0000-4000-8000-000000000000'::uuid,60
  );
  IF row_count<>0 THEN RAISE EXCEPTION 'stale claim token heartbeat was accepted'; END IF;

  UPDATE drama.operations SET lease_expires_at=CURRENT_TIMESTAMP-interval '1 second'
  WHERE operation_id='operation_phase1_lease_test';
  SELECT claim_token INTO second_claim
  FROM drama.claim_operation('worker-b','claim-request-b',ARRAY['source_import'],60);
  IF second_claim IS NULL OR second_claim=first_claim THEN
    RAISE EXCEPTION 'expired operation lease was not safely taken over';
  END IF;
  IF (SELECT retry_count FROM drama.operations WHERE operation_id='operation_phase1_lease_test')<>1 THEN
    RAISE EXCEPTION 'operation takeover did not increment retry_count';
  END IF;
  BEGIN
    PERFORM drama.finish_operation(
      'operation_phase1_lease_test',first_claim,'completed','source_version',
      'sv_legacy_novel_phase1_legacy',NULL,NULL,NULL
    );
    RAISE EXCEPTION USING ERRCODE='P0002',MESSAGE='stale worker completed an operation after takeover';
  EXCEPTION WHEN SQLSTATE '55000' THEN NULL;
  END;
  PERFORM drama.checkpoint_operation(
    'operation_phase1_lease_test',second_claim,'validating','contract-validation',NULL,'{}'
  );
  PERFORM drama.finish_operation(
    'operation_phase1_lease_test',second_claim,'completed','source_version',
    'sv_legacy_novel_phase1_legacy',NULL,NULL,NULL
  );
END $$;

-- Legacy project deletion remains possible; source-library rows are retained.
DELETE FROM drama.projects WHERE project_id='p_phase1_legacy';
DO $$
BEGIN
  IF EXISTS(SELECT 1 FROM drama.projects WHERE project_id='p_phase1_legacy') THEN
    RAISE EXCEPTION 'legacy project cascade deletion failed';
  END IF;
  IF NOT EXISTS(SELECT 1 FROM drama.source_works WHERE work_id='sw_legacy_novel_phase1_legacy') THEN
    RAISE EXCEPTION 'project deletion incorrectly removed source library work';
  END IF;
END $$;

ROLLBACK;
SELECT 'phase1_negative_contract_tests' AS check_name,'PASS' AS result;
