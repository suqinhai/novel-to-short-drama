\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

-- Exercise effective mode against the complete deterministic Phase 5 fixture.
-- Everything in this file is rolled back so the historical fixtures remain
-- reusable by the older integration suites.
UPDATE drama.projects
SET input_resolution_mode='effective'
WHERE project_id='p_phase1_legacy';

INSERT INTO drama.artifacts(
  artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
  validity_status,is_current,idempotency_key,metadata
)
SELECT
  'artifact_phase18_ir','narrative_ir','p_phase1_legacy',ir_revision_id,
  revision_number,output_hash,'valid',true,'phase18:artifact:ir','{"acceptance":true}'
FROM drama.narrative_ir_revisions
WHERE ir_revision_id='ir_phase1_001';

INSERT INTO drama.candidate_hard_rule_results(
  candidate_hard_rule_result_id,candidate_selection_id,rule_name,passed,message
)
SELECT
  'hard_rule_phase18_1_'||rule_name,'selection_phase5_1',rule_name,true,'fixture pass'
FROM unnest(ARRAY[
  'causality','duration','character_state','foreshadowing','continuity'
]) rule_name;

INSERT INTO drama.artifact_current_bindings(
  artifact_current_binding_id,project_id,target_type,target_id,component_scope,current_artifact_id
) VALUES(
  'binding_phase18_candidate_1','p_phase1_legacy','episode','ep_phase1_legacy_001',
  'whole','artifact_phase5_selection'
);

INSERT INTO drama.character_performance_bibles(
  performance_bible_id,project_id,character_id,character_version,version,speech,acting,
  relational_voices,appearance,locked_fields,allowed_fields,change_reasons,source_refs,
  status,content_hash,created_by
) VALUES(
  'pb_phase18_zhou_v1','p_phase1_legacy','char_zhou','character-v1',1,
  '{"pace":"measured","pitch":"low"}','{"habit":"watches the key","taboo":"comic reaction"}',
  '{"char_lin":"restrained distrust"}','{"age":31,"posture":"guarded"}',
  '["speech.pitch","appearance.age"]','["acting.emotion"]',
  '{"v1":"resolver acceptance"}','{"source_span_ids":["span_legacy_full_ch_phase1_legacy_001"]}',
  'locked',repeat('2',64),'phase18-fixture'
);

INSERT INTO drama.character_visual_profiles(
  profile_id,project_id,character_id,version,canonical_name,gender,apparent_age,
  base_prompt,negative_prompt,source_story_bible_version,status,review_status,lock_status
) VALUES
  ('profile_phase18_lin','p_phase1_legacy','char_lin',1,'Lin','female','28',
   'realistic restrained investigator','identity drift',1,'ready','approved','locked'),
  ('profile_phase18_zhou','p_phase1_legacy','char_zhou',1,'Zhou','male','31',
   'realistic guarded witness','identity drift',1,'ready','approved','locked');

INSERT INTO drama.location_visual_profiles(
  profile_id,project_id,location_id,version,canonical_name,environment_type,
  base_prompt,negative_prompt,source_story_bible_version,status,review_status,lock_status
) VALUES(
  'profile_phase18_door','p_phase1_legacy','location_door',1,'Old house entrance','interior',
  'old wooden door under cold moonlight','modern furniture',1,'ready','approved','locked'
);

INSERT INTO drama.editing_template_bindings(
  editing_template_binding_id,project_id,episode_id,editing_template_version_id,version,
  override_config,is_current,change_reason,created_by
) VALUES(
  'template_binding_phase18','p_phase1_legacy',NULL,'etv_system_suspense_v1',1,
  '{}',true,'resolver acceptance binding','phase18-fixture'
);

DO $$
DECLARE result JSONB;
BEGIN
  result := drama.resolve_effective_inputs(
    'p_phase1_legacy','ep_phase1_legacy_001','09'
  );
  IF result->>'status'<>'ready' OR NOT (result->>'ready')::boolean THEN
    RAISE EXCEPTION 'expected ready stage 09 resolution, got %',result;
  END IF;
  IF jsonb_array_length(result->'items')<>11 THEN
    RAISE EXCEPTION 'resolver must return all 11 authoritative input kinds';
  END IF;
  IF EXISTS(
    SELECT 1 FROM jsonb_array_elements(result->'items') item
    WHERE item->>'requirement' NOT IN ('required','optional')
       OR item->>'state' NOT IN ('resolved','missing','stale','needs_review','blocked')
  ) THEN
    RAISE EXCEPTION 'resolver returned an unsupported requirement/state';
  END IF;
  IF (SELECT item->>'state' FROM jsonb_array_elements(result->'items') item
      WHERE item->>'kind'='candidate_selection')<>'resolved' THEN
    RAISE EXCEPTION 'confirmed candidate binding was not resolved';
  END IF;
END $$;

-- Candidate selection is an explicit pointer. Switching it must change the
-- downstream context; stale or unconfirmed selections can never resolve.
CREATE TEMP TABLE phase18_context_probe(
  probe_key TEXT PRIMARY KEY,
  context_hash TEXT NOT NULL,
  item_hash TEXT NOT NULL
) ON COMMIT DROP;

INSERT INTO phase18_context_probe
SELECT 'candidate_1',resolution->>'context_hash',item->>'content_hash'
FROM (SELECT drama.resolve_effective_inputs(
  'p_phase1_legacy','ep_phase1_legacy_001','06'
) resolution) resolved
CROSS JOIN LATERAL jsonb_array_elements(resolution->'items') item
WHERE item->>'kind'='candidate_selection';

INSERT INTO drama.artifacts(
  artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
  validity_status,is_current,idempotency_key,metadata
) VALUES(
  'artifact_phase18_selection_2','candidate_selection','p_phase1_legacy',
  'selection_phase18_2',1,repeat('3',64),'valid',true,
  'phase18:artifact:selection:2','{"acceptance":true}'
);
INSERT INTO drama.candidate_selections(
  candidate_selection_id,candidate_set_id,selected_candidate_id,artifact_id,selection_type,
  content,validation_summary,confirmed_by,idempotency_key
) VALUES(
  'selection_phase18_2','candidate_set_phase5','candidate_phase5_2',
  'artifact_phase18_selection_2','candidate',
  '{"selected":"candidate_phase5_2","opening":"performance pause"}',
  '{"causality":true,"duration":true,"character_state":true,"foreshadowing":true,"continuity":true}',
  'phase18-fixture','phase18:selection:2'
);
INSERT INTO drama.candidate_hard_rule_results(
  candidate_hard_rule_result_id,candidate_selection_id,rule_name,passed,message
)
SELECT
  'hard_rule_phase18_2_'||rule_name,'selection_phase18_2',rule_name,true,'fixture pass'
FROM unnest(ARRAY[
  'causality','duration','character_state','foreshadowing','continuity'
]) rule_name;

UPDATE drama.artifact_current_bindings
SET current_artifact_id='artifact_phase18_selection_2'
WHERE artifact_current_binding_id='binding_phase18_candidate_1';

INSERT INTO phase18_context_probe
SELECT 'candidate_2',resolution->>'context_hash',item->>'content_hash'
FROM (SELECT drama.resolve_effective_inputs(
  'p_phase1_legacy','ep_phase1_legacy_001','06'
) resolution) resolved
CROSS JOIN LATERAL jsonb_array_elements(resolution->'items') item
WHERE item->>'kind'='candidate_selection';

DO $$
BEGIN
  IF (SELECT context_hash FROM phase18_context_probe WHERE probe_key='candidate_1')
     =(SELECT context_hash FROM phase18_context_probe WHERE probe_key='candidate_2') THEN
    RAISE EXCEPTION 'candidate switch did not change generation context';
  END IF;
  IF (SELECT item_hash FROM phase18_context_probe WHERE probe_key='candidate_1')
     =(SELECT item_hash FROM phase18_context_probe WHERE probe_key='candidate_2') THEN
    RAISE EXCEPTION 'candidate switch did not change candidate input hash';
  END IF;
END $$;

UPDATE drama.artifacts
SET validity_status='stale'
WHERE artifact_id='artifact_phase18_selection_2';
DO $$
DECLARE result JSONB;
BEGIN
  result := drama.resolve_effective_inputs('p_phase1_legacy','ep_phase1_legacy_001','06');
  IF (SELECT item->>'state' FROM jsonb_array_elements(result->'items') item
      WHERE item->>'kind'='candidate_selection')<>'stale'
     OR result->>'status'<>'blocked' THEN
    RAISE EXCEPTION 'stale candidate was accepted: %',result;
  END IF;
END $$;
UPDATE drama.artifacts
SET validity_status='valid'
WHERE artifact_id='artifact_phase18_selection_2';

DELETE FROM drama.artifact_current_bindings
WHERE artifact_current_binding_id='binding_phase18_candidate_1';
DO $$
DECLARE result JSONB;
BEGIN
  result := drama.resolve_effective_inputs('p_phase1_legacy','ep_phase1_legacy_001','06');
  IF (SELECT item->>'state' FROM jsonb_array_elements(result->'items') item
      WHERE item->>'kind'='candidate_selection')<>'needs_review'
     OR result->>'status'<>'needs_review' THEN
    RAISE EXCEPTION 'unconfirmed candidate set was silently accepted: %',result;
  END IF;
END $$;
INSERT INTO drama.artifact_current_bindings(
  artifact_current_binding_id,project_id,target_type,target_id,component_scope,current_artifact_id
) VALUES(
  'binding_phase18_candidate_1','p_phase1_legacy','episode','ep_phase1_legacy_001',
  'whole','artifact_phase18_selection_2'
);

-- Image/video/TTS stages must hard-stop when the performance contract is not
-- complete or when continuity has a conflict.
INSERT INTO drama.storyboard_shots(
  shot_id,storyboard_id,project_id,episode_id,scene_id,shot_number,shot_order,
  duration_seconds,shot_size,camera_angle,camera_motion,composition,character_ids,
  location_id,action_description,facial_expression,dialogue_ids,subtitle_text,
  narration_text,lighting,atmosphere,sound_effect_hint,bgm_hint,transition_type,
  visual_prompt_base,video_prompt_base,negative_prompt_base,continuity_notes,
  source_scene_data,status,generation_version
) VALUES(
  'shot_phase18_missing_bible','storyboard_phase5_post','p_phase1_legacy',
  'ep_phase1_legacy_001','scene_phase5_post',3,3,1,'close','eye_level','static',
  'missing character check','["char_without_locked_bible"]','location_door',
  'acceptance probe','neutral','[]','','','','','','','cut','','','','{}','{}','approved',1
);
DO $$
DECLARE stage_name TEXT; result JSONB;
BEGIN
  FOREACH stage_name IN ARRAY ARRAY['08','09','10'] LOOP
    result := drama.resolve_effective_inputs(
      'p_phase1_legacy','ep_phase1_legacy_001',stage_name
    );
    IF (SELECT item->>'state' FROM jsonb_array_elements(result->'items') item
        WHERE item->>'kind'='performance_bible')<>'blocked'
       OR result->>'status'<>'blocked' THEN
      RAISE EXCEPTION 'stage % did not block missing locked bible: %',stage_name,result;
    END IF;
  END LOOP;
END $$;
DELETE FROM drama.storyboard_shots WHERE shot_id='shot_phase18_missing_bible';

UPDATE drama.continuity_ledger_entries
SET validation_status='conflict',diagnostics='[{"code":"AXIS_BREAK"}]'
WHERE continuity_entry_id='continuity_phase5_2';
DO $$
DECLARE stage_name TEXT; result JSONB;
BEGIN
  FOREACH stage_name IN ARRAY ARRAY['08','09','10'] LOOP
    result := drama.resolve_effective_inputs(
      'p_phase1_legacy','ep_phase1_legacy_001',stage_name
    );
    IF (SELECT item->>'state' FROM jsonb_array_elements(result->'items') item
        WHERE item->>'kind'='continuity_ledger')<>'blocked'
       OR NOT (result->'blockers' @> '[{"kind":"continuity_ledger","reason":"CONTINUITY_CONFLICT"}]') THEN
      RAISE EXCEPTION 'stage % did not block continuity conflict: %',stage_name,result;
    END IF;
  END LOOP;
END $$;
UPDATE drama.continuity_ledger_entries
SET validation_status='valid',diagnostics='[]'
WHERE continuity_entry_id='continuity_phase5_2';

-- Add a second episode and then publish a new pacing version whose episode 1
-- semantics change while episode 2 remains byte-for-byte equivalent.
INSERT INTO drama.episode_outlines(
  episode_id,season_id,project_id,episode_number,title,logline,source_chapter_ids,
  source_chunk_ids,opening_hook,story_goal,main_conflict,plot_points,climax,
  ending_hook,character_ids,location_ids,estimated_duration_seconds,
  continuity_in,continuity_out,status,version
) VALUES(
  'ep_phase18_002','season_phase1_legacy','p_phase1_legacy',2,'Second clue',
  'The unchanged control episode.','["ch_phase1_legacy_001"]','[]','A second knock',
  'Trace the witness','Time pressure','[]','The witness speaks','A new threat',
  '[]','[]',9,'[]','[]','approved',1
);
INSERT INTO drama.adaptation_episode_plans(
  adaptation_episode_plan_id,adaptation_plan_id,episode_number,title,logline,
  estimated_duration_seconds,opening_hook,ending_hook,content_hash
) VALUES(
  'adaptation_episode_plan_phase18_002','adaptation_plan_phase1_001',2,
  'Second clue','The unchanged control episode.',9,'A second knock','A new threat',
  repeat('4',64)
);
INSERT INTO drama.artifacts(
  artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
  validity_status,is_current,idempotency_key,metadata
) VALUES
  ('artifact_phase18_episode_plan_2','adaptation_episode_plan','p_phase1_legacy',
   'adaptation_episode_plan_phase18_002',1,repeat('4',64),'valid',true,
   'phase18:artifact:episode-plan:2','{}'),
  ('artifact_phase18_beat_ep2_v1','pacing_beat','p_phase1_legacy',
   'beat_phase18_ep2_v1',1,repeat('5',64),'valid',true,'phase18:artifact:beat:ep2:v1','{}');
INSERT INTO drama.pacing_episodes(
  pacing_episode_id,pacing_plan_id,adaptation_episode_plan_id,episode_number,title,
  conflict_intensity,emotional_intensity,information_reveal,hook_strength,
  estimated_duration_seconds
) VALUES(
  'pacing_episode_phase18_2_v1','pacing_phase5_v1',
  'adaptation_episode_plan_phase18_002',2,'Second clue',0.61,0.58,0.52,0.73,9
);
INSERT INTO drama.pacing_beats(
  pacing_beat_id,pacing_plan_id,pacing_episode_id,beat_key,artifact_id,episode_number,
  beat_ordinal,title,summary,beat_type,source_span_id,fact_revision_id,event_revision_id,
  conflict_intensity,emotional_intensity,information_reveal,hook_strength,reversal_strength,
  dialogue_ratio,action_ratio,narration_ratio,estimated_duration_seconds,is_manual
) VALUES(
  'beat_phase18_ep2_v1','pacing_phase5_v1','pacing_episode_phase18_2_v1',
  'episode-2:opening','artifact_phase18_beat_ep2_v1',2,1,'Second knock',
  'The witness hears an unchanged second knock.','opening_hook',
  'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
  'event_revision_phase1_001',0.61,0.58,0.52,0.73,0.20,0.30,0.60,0.10,9,false
);

INSERT INTO phase18_context_probe
SELECT 'pacing_ep1_v1',resolution->>'context_hash',item->>'content_hash'
FROM (SELECT drama.resolve_effective_inputs(
  'p_phase1_legacy','ep_phase1_legacy_001','05'
) resolution) resolved
CROSS JOIN LATERAL jsonb_array_elements(resolution->'items') item
WHERE item->>'kind'='pacing_plan';
INSERT INTO phase18_context_probe
SELECT 'pacing_ep2_v1',resolution->>'context_hash',item->>'content_hash'
FROM (SELECT drama.resolve_effective_inputs(
  'p_phase1_legacy','ep_phase18_002','05'
) resolution) resolved
CROSS JOIN LATERAL jsonb_array_elements(resolution->'items') item
WHERE item->>'kind'='pacing_plan';

UPDATE drama.pacing_plan_versions
SET status='superseded'
WHERE pacing_plan_id='pacing_phase5_v1';
UPDATE drama.artifacts
SET is_current=false,validity_status='superseded'
WHERE artifact_id='artifact_phase5_pacing';
INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,
  input_hash,checkpoint_stage,result_type,result_id,completed_at
) VALUES(
  'operation_phase18_pacing_v2','trace_phase18_pacing_v2','adaptation_compile',
  'project','p_phase1_legacy','completed','phase18:operation:pacing:v2',
  repeat('6',64),'finished','pacing_plan','pacing_phase18_v2',now()
);
INSERT INTO drama.artifacts(
  artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
  validity_status,is_current,idempotency_key,metadata
) VALUES
  ('artifact_phase18_pacing_v2','pacing_plan','p_phase1_legacy','pacing_phase18_v2',
   2,repeat('6',64),'valid',true,'phase18:artifact:pacing:v2','{}'),
  ('artifact_phase18_beat_ep1a_v2','pacing_beat','p_phase1_legacy','beat_phase18_ep1a_v2',
   2,repeat('7',64),'valid',true,'phase18:artifact:beat:ep1a:v2','{}'),
  ('artifact_phase18_beat_ep1b_v2','pacing_beat','p_phase1_legacy','beat_phase18_ep1b_v2',
   2,repeat('8',64),'valid',true,'phase18:artifact:beat:ep1b:v2','{}'),
  ('artifact_phase18_beat_ep2_v2','pacing_beat','p_phase1_legacy','beat_phase18_ep2_v2',
   2,repeat('9',64),'valid',true,'phase18:artifact:beat:ep2:v2','{}');
INSERT INTO drama.pacing_plan_versions(
  pacing_plan_id,parent_pacing_plan_id,operation_id,artifact_id,project_id,
  source_version_id,ir_revision_id,adaptation_spec_version_id,adaptation_plan_id,
  diagnostic_report_id,version_number,status,analyzer_version,total_duration_seconds,
  content_hash
) VALUES(
  'pacing_phase18_v2','pacing_phase5_v1','operation_phase18_pacing_v2',
  'artifact_phase18_pacing_v2','p_phase1_legacy','sv_legacy_novel_phase1_legacy',
  'ir_phase1_001','adaptation_spec_version_phase1_001','adaptation_plan_phase1_001',
  'diagnostic_phase5_v1',2,'published','deterministic-pacing-v2',17,repeat('6',64)
);
INSERT INTO drama.pacing_episodes(
  pacing_episode_id,pacing_plan_id,adaptation_episode_plan_id,episode_number,title,
  conflict_intensity,emotional_intensity,information_reveal,hook_strength,
  estimated_duration_seconds
) VALUES
  ('pacing_episode_phase18_1_v2','pacing_phase18_v2',
   'adaptation_episode_plan_phase1_001',1,'Changed first episode',0.91,0.80,0.70,0.97,8),
  ('pacing_episode_phase18_2_v2','pacing_phase18_v2',
   'adaptation_episode_plan_phase18_002',2,'Second clue',0.61,0.58,0.52,0.73,9);
INSERT INTO drama.pacing_beats(
  pacing_beat_id,pacing_plan_id,pacing_episode_id,beat_key,artifact_id,episode_number,
  beat_ordinal,title,summary,beat_type,source_span_id,fact_revision_id,event_revision_id,
  conflict_intensity,emotional_intensity,information_reveal,hook_strength,reversal_strength,
  dialogue_ratio,action_ratio,narration_ratio,estimated_duration_seconds,is_manual
) VALUES
  ('beat_phase18_ep1a_v2','pacing_phase18_v2','pacing_episode_phase18_1_v2',
   'episode-1:opening','artifact_phase18_beat_ep1a_v2',1,1,'Door opens faster',
   'The changed opening compresses the first reveal.','opening_hook',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'event_revision_phase1_001',0.90,0.80,0.50,0.97,0.40,0.30,0.70,0,4,false),
  ('beat_phase18_ep1b_v2','pacing_phase18_v2','pacing_episode_phase18_1_v2',
   'episode-1:turn','artifact_phase18_beat_ep1b_v2',1,2,'Key reversal',
   'The first episode reversal now lands earlier.','ending_hook',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'event_revision_phase1_002',0.90,0.82,0.75,0.98,0.86,0.55,0.45,0,4,false),
  ('beat_phase18_ep2_v2','pacing_phase18_v2','pacing_episode_phase18_2_v2',
   'episode-2:opening','artifact_phase18_beat_ep2_v2',2,1,'Second knock',
   'The witness hears an unchanged second knock.','opening_hook',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'event_revision_phase1_001',0.61,0.58,0.52,0.73,0.20,0.30,0.60,0.10,9,false
);

INSERT INTO phase18_context_probe
SELECT 'pacing_ep1_v2',resolution->>'context_hash',item->>'content_hash'
FROM (SELECT drama.resolve_effective_inputs(
  'p_phase1_legacy','ep_phase1_legacy_001','05'
) resolution) resolved
CROSS JOIN LATERAL jsonb_array_elements(resolution->'items') item
WHERE item->>'kind'='pacing_plan';
INSERT INTO phase18_context_probe
SELECT 'pacing_ep2_v2',resolution->>'context_hash',item->>'content_hash'
FROM (SELECT drama.resolve_effective_inputs(
  'p_phase1_legacy','ep_phase18_002','05'
) resolution) resolved
CROSS JOIN LATERAL jsonb_array_elements(resolution->'items') item
WHERE item->>'kind'='pacing_plan';

DO $$
BEGIN
  IF (SELECT context_hash FROM phase18_context_probe WHERE probe_key='pacing_ep1_v1')
     =(SELECT context_hash FROM phase18_context_probe WHERE probe_key='pacing_ep1_v2') THEN
    RAISE EXCEPTION 'changed episode pacing did not change episode 1 generation context';
  END IF;
  IF (SELECT item_hash FROM phase18_context_probe WHERE probe_key='pacing_ep2_v1')
     <>(SELECT item_hash FROM phase18_context_probe WHERE probe_key='pacing_ep2_v2') THEN
    RAISE EXCEPTION 'unchanged episode 2 pacing hash changed across plan versions';
  END IF;
  IF (SELECT context_hash FROM phase18_context_probe WHERE probe_key='pacing_ep2_v1')
     <>(SELECT context_hash FROM phase18_context_probe WHERE probe_key='pacing_ep2_v2') THEN
    RAISE EXCEPTION 'unrelated episode 2 generation context changed with episode 1 pacing';
  END IF;
END $$;

-- The claim consumed by a generation is persisted with exact IDs/hashes and
-- becomes both dependency and provenance on the produced artifact.
INSERT INTO drama.video_generation_tasks(
  task_id,idempotency_key,trace_id,project_id,episode_id,shot_id,
  storyboard_image_id,generation_version,provider,model,status,progress
) VALUES(
  'video_task_phase18_provenance','phase18:video-task:provenance',
  'trace_phase18_generation','p_phase1_legacy','ep_phase1_legacy_001',
  'shot_phase5_1','image_phase5_1',1,'deterministic_mock','mock-video-v1',
  'succeeded',100
);
SELECT drama.claim_effective_inputs(
  'p_phase1_legacy','ep_phase1_legacy_001','09','trace_phase18_generation',1
);
SELECT drama.record_effective_input_outputs('trace_phase18_generation','09');

DO $$
DECLARE target_artifact_id TEXT;
BEGIN
  SELECT artifact_id INTO target_artifact_id
  FROM drama.artifacts
  WHERE project_id='p_phase1_legacy' AND artifact_type='shot_video'
    AND native_entity_id='video_phase5_1' AND revision_number=1;
  IF target_artifact_id IS NULL THEN
    RAISE EXCEPTION 'generation output artifact was not linked';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM drama.artifact_input_consumptions
    WHERE artifact_id=target_artifact_id
      AND input_kind='candidate_selection'
      AND input_id='selection_phase18_2'
      AND observed_input_hash=(SELECT item_hash FROM phase18_context_probe
        WHERE probe_key='candidate_2')
  ) THEN
    RAISE EXCEPTION 'actual candidate ID/hash was not recorded as consumed';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM drama.artifact_input_consumptions
    WHERE artifact_id=target_artifact_id
      AND input_kind='pacing_plan'
      AND input_id='pacing_phase18_v2'
  ) THEN
    RAISE EXCEPTION 'actual pacing plan was not recorded as consumed';
  END IF;
  IF NOT EXISTS(
    SELECT 1 FROM drama.artifact_dependencies
    WHERE downstream_artifact_id=target_artifact_id AND dependency_type='effective_input'
  ) OR NOT EXISTS(
    SELECT 1 FROM drama.artifact_provenance_events
    WHERE artifact_id=target_artifact_id
      AND details->>'context_hash'=(SELECT context_hash
        FROM drama.generation_effective_input_claims
        WHERE trace_id='trace_phase18_generation' AND stage_key='image_to_video')
  ) THEN
    RAISE EXCEPTION 'effective input dependency/provenance was not attached';
  END IF;
END $$;

-- A published IR may only transition to superseded. The resolver must report
-- that explicit lifecycle as stale rather than guessing another revision.
UPDATE drama.artifacts
SET validity_status='stale'
WHERE artifact_id='artifact_phase18_ir';
DO $$
DECLARE result JSONB;
BEGIN
  result := drama.resolve_effective_inputs(
    'p_phase1_legacy','ep_phase1_legacy_001','05'
  );
  IF (SELECT item->>'state' FROM jsonb_array_elements(result->'items') item
      WHERE item->>'kind'='narrative_ir')<>'stale' THEN
    RAISE EXCEPTION 'stale current IR artifact was resolved: %',result;
  END IF;
END $$;
UPDATE drama.artifacts
SET validity_status='valid'
WHERE artifact_id='artifact_phase18_ir';

UPDATE drama.narrative_ir_revisions
SET status='superseded',is_current=false
WHERE ir_revision_id='ir_phase1_001';
DO $$
DECLARE result JSONB;
BEGIN
  result := drama.resolve_effective_inputs(
    'p_phase1_legacy','ep_phase1_legacy_001','05'
  );
  IF (SELECT item->>'state' FROM jsonb_array_elements(result->'items') item
      WHERE item->>'kind'='narrative_ir')<>'stale'
     OR result->>'status'<>'blocked' THEN
    RAISE EXCEPTION 'superseded IR was not blocked as stale: %',result;
  END IF;
END $$;

ROLLBACK;
\echo 'PASS phase18 effective input resolver acceptance'
