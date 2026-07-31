\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

-- One immutable diagnosis -> pacing -> quality -> candidate-selection chain is
-- persisted for the same episode consumed by the creative workbench.
INSERT INTO drama.operations(
  operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,
  input_hash,checkpoint_stage,result_type,result_id,completed_at
) VALUES
  ('operation_phase5_diagnostic','trace_phase5_diagnostic','spec_validation','project',
   'p_phase1_legacy','completed','phase5:operation:diagnostic',repeat('5',64),
   'finished','adaptation_diagnostic_report','diagnostic_phase5_v1',now()),
  ('operation_phase5_pacing','trace_phase5_pacing','adaptation_compile','project',
   'p_phase1_legacy','completed','phase5:operation:pacing',repeat('6',64),
   'finished','pacing_plan','pacing_phase5_v1',now()),
  ('operation_phase5_quality','trace_phase5_quality','invalidation_scan','artifact',
   'artifact_phase5_pacing','completed','phase5:operation:quality',repeat('7',64),
   'finished','quality_score_report','quality_phase5_v1',now());

INSERT INTO drama.artifacts(
  artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
  validity_status,is_current,idempotency_key,metadata
) VALUES
  ('artifact_phase5_diagnostic','adaptation_diagnostic_report','p_phase1_legacy',
   'diagnostic_phase5_v1',1,repeat('5',64),'valid',true,'phase5:artifact:diagnostic',
   '{"prompt_version":"diagnostic-prompt-v1","model_version":"deterministic-mock-v1"}'),
  ('artifact_phase5_pacing','pacing_plan','p_phase1_legacy',
   'pacing_phase5_v1',1,repeat('6',64),'valid',true,'phase5:artifact:pacing',
   '{"prompt_version":"pacing-prompt-v1","model_version":"deterministic-mock-v1"}'),
  ('artifact_phase5_beat','pacing_beat','p_phase1_legacy',
   'beat_phase5_hook',1,repeat('7',64),'valid',true,'phase5:artifact:beat',
   '{"source_span_id":"span_legacy_full_ch_phase1_legacy_001"}'),
  ('artifact_phase5_quality','quality_score_report','p_phase1_legacy',
   'quality_phase5_v1',1,repeat('8',64),'valid',true,'phase5:artifact:quality',
   '{"prompt_version":"quality-prompt-v1","model_version":"deterministic-mock-v1"}'),
  ('artifact_phase5_candidate_1','candidate_version','p_phase1_legacy',
   'candidate_phase5_1',1,repeat('9',64),'valid',true,'phase5:artifact:candidate:1','{}'),
  ('artifact_phase5_candidate_2','candidate_version','p_phase1_legacy',
   'candidate_phase5_2',1,repeat('a',64),'valid',true,'phase5:artifact:candidate:2','{}'),
  ('artifact_phase5_selection','candidate_selection','p_phase1_legacy',
   'selection_phase5_1',1,repeat('b',64),'valid',true,'phase5:artifact:selection','{}');

INSERT INTO drama.adaptation_diagnostic_reports(
  diagnostic_report_id,operation_id,artifact_id,project_id,source_version_id,ir_revision_id,
  adaptation_spec_version_id,version_number,status,analyzer_version,core_selling_points,
  target_audience,emotional_value,protagonist_curve,hook_recommendations,
  transformation_recommendations,unfilmable_passages,summary,content_hash
) VALUES(
  'diagnostic_phase5_v1','operation_phase5_diagnostic','artifact_phase5_diagnostic',
  'p_phase1_legacy','sv_legacy_novel_phase1_legacy','ir_phase1_001',
  'adaptation_spec_version_phase1_001',1,'completed','deterministic-diagnostic-v1',
  '["门后悬念","钥匙线索"]','{"platform":"vertical-short-drama","audience":"18-35"}',
  '["警觉","怀疑","威胁"]','{"char_lin":["克制","决断"]}',
  '{"opening":"门自动打开","ending":"脚步逼近"}',
  '[{"action":"frontload","target":"key clue"}]','[]',
  '{"mock_chain_stage":"adaptation_diagnosis"}',repeat('5',64)
);
INSERT INTO drama.adaptation_diagnostic_nodes(
  diagnostic_node_id,diagnostic_report_id,node_type,source_span_id,fact_revision_id,
  ordinal,title,description,intensity,production_complexity,recommended_action,evidence
) VALUES(
  'diagnostic_node_phase5_hook','diagnostic_phase5_v1','selling_point',
  'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
  1,'门后悬念','用自动开启的门在前三秒建立异常',0.92,0.20,'frontload',
  '{"source_span_id":"span_legacy_full_ch_phase1_legacy_001"}'
);

INSERT INTO drama.pacing_plan_versions(
  pacing_plan_id,operation_id,artifact_id,project_id,source_version_id,ir_revision_id,
  adaptation_spec_version_id,adaptation_plan_id,diagnostic_report_id,version_number,
  status,analyzer_version,total_duration_seconds,content_hash
) VALUES(
  'pacing_phase5_v1','operation_phase5_pacing','artifact_phase5_pacing','p_phase1_legacy',
  'sv_legacy_novel_phase1_legacy','ir_phase1_001','adaptation_spec_version_phase1_001',
  'adaptation_plan_phase1_001','diagnostic_phase5_v1',1,'published',
  'deterministic-pacing-v1',8,repeat('6',64)
);
INSERT INTO drama.pacing_episodes(
  pacing_episode_id,pacing_plan_id,adaptation_episode_plan_id,episode_number,title,
  conflict_intensity,emotional_intensity,information_reveal,hook_strength,
  estimated_duration_seconds
) VALUES(
  'pacing_episode_phase5_1','pacing_phase5_v1','adaptation_episode_plan_phase1_001',
  1,'门后的线索',0.78,0.72,0.65,0.91,8
);
INSERT INTO drama.pacing_beats(
  pacing_beat_id,pacing_plan_id,pacing_episode_id,beat_key,artifact_id,episode_number,
  beat_ordinal,title,summary,beat_type,source_span_id,fact_revision_id,event_revision_id,
  conflict_intensity,emotional_intensity,information_reveal,hook_strength,reversal_strength,
  dialogue_ratio,action_ratio,narration_ratio,estimated_duration_seconds,is_manual
) VALUES
  ('beat_phase5_hook','pacing_phase5_v1','pacing_episode_phase5_1','episode-1:opening',
   'artifact_phase5_beat',1,1,'门自动打开','林夏推门并察觉异常','opening_hook',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'event_revision_phase1_001',0.75,0.72,0.40,0.92,0.30,0.35,0.65,0,4,false),
  ('beat_phase5_turn','pacing_phase5_v1','pacing_episode_phase5_1','episode-1:turn',
   'artifact_phase5_beat',1,2,'钥匙反转','周野指出钥匙并听见脚步','ending_hook',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'event_revision_phase1_002',0.86,0.78,0.72,0.95,0.82,0.55,0.45,0,4,false);

INSERT INTO drama.quality_score_reports(
  quality_score_report_id,operation_id,artifact_id,project_id,source_version_id,ir_revision_id,
  adaptation_spec_version_id,pacing_plan_id,diagnostic_report_id,target_artifact_id,
  version_number,scope,scope_selector,analyzer_version,total_score,status,content_hash
) VALUES(
  'quality_phase5_v1','operation_phase5_quality','artifact_phase5_quality','p_phase1_legacy',
  'sv_legacy_novel_phase1_legacy','ir_phase1_001','adaptation_spec_version_phase1_001',
  'pacing_phase5_v1','diagnostic_phase5_v1','artifact_phase1_episode_plan_001',
  1,'episode','{"episode_id":"ep_phase1_legacy_001","episode_number":1}',
  'deterministic-quality-v1',88,'completed',repeat('8',64)
);
INSERT INTO drama.quality_issues(
  quality_issue_id,quality_score_report_id,dimension,severity,episode_number,pacing_beat_id,
  artifact_id,source_span_id,fact_revision_id,location,evidence,message,suggestion
) VALUES(
  'quality_issue_phase5_1','quality_phase5_v1','声画可执行性','warning',1,
  'beat_phase5_turn','artifact_phase5_beat','span_legacy_full_ch_phase1_legacy_001',
  'fact_revision_phase1_event_001','{"timecode_ms":4300,"editor_tab":"sound"}',
  '脚步声需与结尾反应镜头对齐','结尾威胁音效尚需精确卡点','将脚步 cue 对齐到 4300ms'
);

INSERT INTO drama.candidate_sets(
  candidate_set_id,project_id,target_type,target_id,base_artifact_id,quality_score_report_id,
  candidate_count,component_types,difference_directions,must_preserve,allowed_changes,
  model,prompt_version,random_seed,generation_parameters,estimated_cost,currency,
  generator_version,idempotency_key,request_hash
) VALUES(
  'candidate_set_phase5','p_phase1_legacy','episode','ep_phase1_legacy_001',
  'artifact_phase1_episode_plan_001','quality_phase5_v1',2,
  '["opening","dialogue","ending"]','["suspense-forward","performance-forward"]',
  '["source facts","character state","ending threat"]','["dialogue density","reaction timing"]',
  'deterministic-mock-v1','candidate-prompt-v1',20260730,'{"temperature":0}',0,'CNY',
  'candidate-generator-v1','phase5:candidate:set',repeat('9',64)
);
INSERT INTO drama.candidates(
  candidate_id,candidate_set_id,artifact_id,ordinal,label,difference_direction,derived_reason,
  content,structured_diff,content_hash,model,prompt_version,random_seed,generation_parameters
) VALUES
  ('candidate_phase5_1','candidate_set_phase5','artifact_phase5_candidate_1',1,
   '悬念前置版','suspense-forward','前三秒强化异常',
   '{"opening":"门自动打开","dialogue":"门不是风吹开的。","ending":"脚步逼近"}',
   '[{"path":"/opening","after":"门自动打开"}]',repeat('9',64),
   'deterministic-mock-v1','candidate-prompt-v1',20260730,'{"temperature":0}'),
  ('candidate_phase5_2','candidate_set_phase5','artifact_phase5_candidate_2',2,
   '表演留白版','performance-forward','增加角色反应停顿',
   '{"opening":"林夏停在门前","dialogue":"门……是从里面开的。","ending":"脚步逼近"}',
   '[{"path":"/dialogue","after":"门……是从里面开的。"}]',repeat('a',64),
   'deterministic-mock-v1','candidate-prompt-v1',20260731,'{"temperature":0}');
INSERT INTO drama.candidate_scores(
  candidate_score_id,candidate_id,source_quality_score_report_id,total_score,fidelity,hook,
  pacing,continuity,filmability,estimated_duration_seconds,modification_risk,
  recommendation_reasons,deduction_reasons,scorer_version
) VALUES
  ('candidate_score_phase5_1','candidate_phase5_1','quality_phase5_v1',92,94,96,90,91,89,8,12,
   '["钩子强且忠实"]','["对白略密"]','candidate-scorer-v1'),
  ('candidate_score_phase5_2','candidate_phase5_2','quality_phase5_v1',87,91,84,86,94,88,8,10,
   '["表演空间充足"]','["开场钩子稍弱"]','candidate-scorer-v1');
INSERT INTO drama.candidate_selections(
  candidate_selection_id,candidate_set_id,selected_candidate_id,artifact_id,selection_type,
  content,validation_summary,confirmed_by,idempotency_key
) VALUES(
  'selection_phase5_1','candidate_set_phase5','candidate_phase5_1','artifact_phase5_selection',
  'candidate','{"selected":"candidate_phase5_1"}',
  '{"causality":true,"duration":true,"character_state":true,"foreshadowing":true,"continuity":true}',
  'phase5-fixture','phase5:selection:1'
);

INSERT INTO drama.episode_scripts(
  script_id,project_id,season_id,episode_id,version,title,opening_hook,scenes,
  climax,ending_hook,estimated_duration_seconds,dialogue_char_count,
  source_outline_version,status,performance_bible_refs
) VALUES(
  'script_phase5_post','p_phase1_legacy','season_phase1_legacy','ep_phase1_legacy_001',2,
  '门后的线索','门自动打开','[]','林夏发现钥匙','脚步声逼近',8,24,1,'completed',
  '{"char_lin":"pb_phase5_lin_v1"}'
);

INSERT INTO drama.script_scenes(
  scene_id,script_id,project_id,episode_id,scene_number,location_id,location_name,
  time_of_day,interior_exterior,character_ids,scene_purpose,actions,dialogues,narration,
  emotional_change,estimated_duration_seconds,source_event_ids
) VALUES(
  'scene_phase5_post','script_phase5_post','p_phase1_legacy','ep_phase1_legacy_001',1,
  'location_door','旧宅门厅','夜','内','["char_lin","char_zhou"]',
  '发现线索并建立威胁','[{"description":"林夏推开门"}]','[]','[]',
  '警觉转为决断',8,'["event_revision_phase1_001","event_revision_phase1_002"]'
);

INSERT INTO drama.dialogues(
  dialogue_id,project_id,episode_id,scene_id,sequence_number,dialogue_type,
  character_id,speaker_name,text,emotion,performance_instruction,estimated_duration_ms,production_mode
) VALUES
  ('dlg_phase5_1','p_phase1_legacy','ep_phase1_legacy_001','scene_phase5_post',1,'dialogue',
   'char_lin','林夏','门不是风吹开的。','克制','短停顿后压低音量',1800,'spoken'),
  ('dlg_phase5_2','p_phase1_legacy','ep_phase1_legacy_001','scene_phase5_post',2,'dialogue',
   'char_zhou','周野','钥匙在你手里？','怀疑','尾音上扬',1500,'spoken');

INSERT INTO drama.storyboards(
  storyboard_id,project_id,episode_id,script_id,version,total_shots,
  estimated_duration_seconds,status,performance_bible_refs
) VALUES(
  'storyboard_phase5_post','p_phase1_legacy','ep_phase1_legacy_001','script_phase5_post',
  1,2,8,'approved','{"char_lin":"pb_phase5_lin_v1"}'
);

INSERT INTO drama.storyboard_shots(
  shot_id,storyboard_id,project_id,episode_id,scene_id,shot_number,shot_order,
  duration_seconds,shot_size,camera_angle,camera_motion,composition,character_ids,
  location_id,action_description,facial_expression,dialogue_ids,subtitle_text,
  narration_text,lighting,atmosphere,sound_effect_hint,bgm_hint,transition_type,
  visual_prompt_base,video_prompt_base,negative_prompt_base,continuity_notes,
  source_scene_data,status,generation_version
) VALUES
  ('shot_phase5_1','storyboard_phase5_post','p_phase1_legacy','ep_phase1_legacy_001',
   'scene_phase5_post',1,1,4,'medium','eye_level','slow_push','林夏左侧，门右侧',
   '["char_lin"]','location_door','林夏推门后凝视门锁','克制警觉','["dlg_phase5_1"]',
   '门不是风吹开的。','','冷月光','压迫','老木门吱呀','低频悬疑脉冲','cut',
   '写实旧宅门厅','林夏推门，动作连续','畸形手','{"axis":"lin-left"}',
   '{"source_event_ids":["event_revision_phase1_001"]}','approved',1),
  ('shot_phase5_2','storyboard_phase5_post','p_phase1_legacy','ep_phase1_legacy_001',
   'scene_phase5_post',2,2,4,'close','eye_level','static','周野右侧反应',
   '["char_zhou"]','location_door','周野看向林夏手中的钥匙','怀疑','["dlg_phase5_2"]',
   '钥匙在你手里？','','冷月光','紧张','远处脚步逼近','低频悬疑脉冲','cut',
   '写实旧宅门厅','周野反应特写','脸部漂移','{"axis":"zhou-right"}',
   '{"source_event_ids":["event_revision_phase1_002"]}','approved',1);

-- Storyboard hints are represented as formal, licensed and versioned sound
-- assets. A complete alternate style group is available for whole-episode
-- replacement without modifying these approved source versions.
INSERT INTO drama.artifacts(
  artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
  validity_status,is_current,idempotency_key,metadata
) VALUES
  ('artifact_phase5_sound_bgm','sound_asset','p_phase1_legacy','sound_phase5_bgm',1,
   repeat('1',64),'valid',true,'phase5:sound:bgm','{"source_hint":"低频悬疑脉冲"}'),
  ('artifact_phase5_sound_ambience','sound_asset','p_phase1_legacy','sound_phase5_ambience',1,
   repeat('2',64),'valid',true,'phase5:sound:ambience','{"source_hint":"旧宅夜间底噪"}'),
  ('artifact_phase5_sound_door','sound_asset','p_phase1_legacy','sound_phase5_door',1,
   repeat('3',64),'valid',true,'phase5:sound:door','{"source_hint":"老木门吱呀"}'),
  ('artifact_phase5_sound_bgm_noir','sound_asset','p_phase1_legacy','sound_phase5_bgm_noir',1,
   repeat('4',64),'valid',true,'phase5:sound:bgm:noir','{"style":"cinematic_noir"}'),
  ('artifact_phase5_sound_ambience_noir','sound_asset','p_phase1_legacy','sound_phase5_ambience_noir',1,
   repeat('5',64),'valid',true,'phase5:sound:ambience:noir','{"style":"cinematic_noir"}'),
  ('artifact_phase5_sound_door_noir','sound_asset','p_phase1_legacy','sound_phase5_door_noir',1,
   repeat('6',64),'valid',true,'phase5:sound:door:noir','{"style":"cinematic_noir"}');
INSERT INTO drama.sound_assets(sound_asset_id,project_id,asset_type,name,style_group) VALUES
  ('sound_phase5_bgm','p_phase1_legacy','bgm','低频悬疑脉冲','suspense_minimal'),
  ('sound_phase5_ambience','p_phase1_legacy','ambience','旧宅夜间底噪','suspense_minimal'),
  ('sound_phase5_door','p_phase1_legacy','door','老木门吱呀','suspense_minimal'),
  ('sound_phase5_bgm_noir','p_phase1_legacy','bgm','黑色电影低音弦乐','cinematic_noir'),
  ('sound_phase5_ambience_noir','p_phase1_legacy','ambience','黑色电影雨夜底噪','cinematic_noir'),
  ('sound_phase5_door_noir','p_phase1_legacy','door','黑色电影厚重门响','cinematic_noir');
INSERT INTO drama.sound_asset_versions(
  sound_asset_version_id,sound_asset_id,artifact_id,version,source_kind,source_uri,
  storage_uri,provider,model_version,mood,bpm,musical_key,duration_ms,license,
  metadata,content_hash,status,is_current,created_by
) VALUES
  ('sound_version_phase5_bgm','sound_phase5_bgm','artifact_phase5_sound_bgm',1,
   'deterministic_mock','mock://bgm/suspense','/sound/bgm-suspense.wav',
   'deterministic_mock','mock-audio-v1','["悬疑","警觉"]',96,'D minor',8000,
   '{"status":"cleared","usage_scope":"all-project-media","license_id":"mock-license-bgm-001"}',
   '{"prompt_version":"sound-prompt-v1"}',repeat('1',64),'approved',true,'phase5-fixture'),
  ('sound_version_phase5_ambience','sound_phase5_ambience','artifact_phase5_sound_ambience',1,
   'deterministic_mock','mock://ambience/old-house','/sound/old-house.wav',
   'deterministic_mock','mock-audio-v1','["夜","压迫"]',NULL,NULL,8000,
   '{"status":"cleared","usage_scope":"all-project-media","license_id":"mock-license-amb-001"}',
   '{"prompt_version":"sound-prompt-v1"}',repeat('2',64),'approved',true,'phase5-fixture'),
  ('sound_version_phase5_door','sound_phase5_door','artifact_phase5_sound_door',1,
   'deterministic_mock','mock://sfx/door','/sound/door-creak.wav',
   'deterministic_mock','mock-audio-v1','["异常","陈旧"]',NULL,NULL,1200,
   '{"status":"cleared","usage_scope":"all-project-media","license_id":"mock-license-sfx-001"}',
   '{"event_key":"shot_phase5_1:door-open"}',repeat('3',64),'approved',true,'phase5-fixture'),
  ('sound_version_phase5_bgm_noir','sound_phase5_bgm_noir','artifact_phase5_sound_bgm_noir',1,
   'deterministic_mock','mock://bgm/noir','/sound/bgm-noir.wav',
   'deterministic_mock','mock-audio-v1','["悬疑","黑色电影"]',92,'C minor',8000,
   '{"status":"cleared","usage_scope":"all-project-media","license_id":"mock-license-bgm-002"}',
   '{"prompt_version":"sound-prompt-v2"}',repeat('4',64),'approved',true,'phase5-fixture'),
  ('sound_version_phase5_ambience_noir','sound_phase5_ambience_noir','artifact_phase5_sound_ambience_noir',1,
   'deterministic_mock','mock://ambience/noir-rain','/sound/noir-rain.wav',
   'deterministic_mock','mock-audio-v1','["雨夜","压迫"]',NULL,NULL,8000,
   '{"status":"cleared","usage_scope":"all-project-media","license_id":"mock-license-amb-002"}',
   '{"prompt_version":"sound-prompt-v2"}',repeat('5',64),'approved',true,'phase5-fixture'),
  ('sound_version_phase5_door_noir','sound_phase5_door_noir','artifact_phase5_sound_door_noir',1,
   'deterministic_mock','mock://sfx/noir-door','/sound/noir-door.wav',
   'deterministic_mock','mock-audio-v1','["厚重","威胁"]',NULL,NULL,1200,
   '{"status":"cleared","usage_scope":"all-project-media","license_id":"mock-license-sfx-002"}',
   '{"event_key":"shot_phase5_1:door-open"}',repeat('6',64),'approved',true,'phase5-fixture');
INSERT INTO drama.sound_cue_versions(
  sound_cue_version_id,sound_cue_id,project_id,episode_id,shot_id,sound_asset_version_id,
  cue_type,source_hint,event_key,sequence_number,start_ms,end_ms,source_in_ms,source_out_ms,
  gain_db,fade_in_ms,fade_out_ms,beat_sync,transition_config,ducking_config,version,
  status,is_current,created_by
) VALUES
  ('sound_cue_version_phase5_bgm','sound_cue_phase5_bgm','p_phase1_legacy',
   'ep_phase1_legacy_001',NULL,'sound_version_phase5_bgm','bgm','低频悬疑脉冲',
   'episode:emotion-curve',1,0,8000,0,8000,-8,300,500,
   '{"bpm":96,"align_to":["beat_phase5_hook","beat_phase5_turn"]}',
   '{"fade":"crossfade","allow_key_change":true}',
   '{"enabled":true,"threshold_db":-28,"ratio":8,"attack_ms":20,"release_ms":250}',
   1,'approved',true,'phase5-fixture'),
  ('sound_cue_version_phase5_ambience','sound_cue_phase5_ambience','p_phase1_legacy',
   'ep_phase1_legacy_001',NULL,'sound_version_phase5_ambience','ambience','旧宅夜间底噪',
   'episode:environment',1,0,8000,0,8000,-14,500,500,'{}','{"fade":"linear"}','{}',
   1,'approved',true,'phase5-fixture'),
  ('sound_cue_version_phase5_door','sound_cue_phase5_door','p_phase1_legacy',
   'ep_phase1_legacy_001','shot_phase5_1','sound_version_phase5_door','door','老木门吱呀',
   'shot_phase5_1:door-open',1,200,1400,0,1200,-2,20,80,
   '{"align_to_ms":200}','{"cut_on_event":true}','{}',1,'approved',true,'phase5-fixture');

INSERT INTO drama.storyboard_images(
  storyboard_image_id,project_id,episode_id,storyboard_id,shot_id,generation_version,
  source_storyboard_version,final_prompt,negative_prompt,provider,model,seed,
  storage_url,status,auto_qc_status,review_status,is_current
) VALUES
  ('image_phase5_1','p_phase1_legacy','ep_phase1_legacy_001','storyboard_phase5_post','shot_phase5_1',
   1,1,'林夏推门写实分镜','畸形手','deterministic_mock','mock-image-v1',101,
   '/storyboard-images/image_phase5_1.png','succeeded','passed','approved',true),
  ('image_phase5_2','p_phase1_legacy','ep_phase1_legacy_001','storyboard_phase5_post','shot_phase5_2',
   1,1,'周野反应写实分镜','脸部漂移','deterministic_mock','mock-image-v1',102,
   '/storyboard-images/image_phase5_2.png','succeeded','passed','approved',true);

INSERT INTO drama.shot_videos(
  shot_video_id,project_id,episode_id,storyboard_id,shot_id,storyboard_image_id,
  source_image_generation_version,generation_version,provider,model,video_prompt,
  reference_image_url,requested_duration_seconds,actual_duration_seconds,aspect_ratio,
  width,height,fps,codec,storage_url,content_hash,status,auto_qc_status,review_status,is_current
) VALUES
  ('video_phase5_1','p_phase1_legacy','ep_phase1_legacy_001','storyboard_phase5_post','shot_phase5_1',
   'image_phase5_1',1,1,'deterministic_mock','mock-video-v1','林夏推门，动作连续',
   '/storyboard-images/image_phase5_1.png',4,4,'9:16',1080,1920,24,'h264',
   '/shot-videos/video_phase5_1.mp4',repeat('a',64),'succeeded','passed','approved',true),
  ('video_phase5_2','p_phase1_legacy','ep_phase1_legacy_001','storyboard_phase5_post','shot_phase5_2',
   'image_phase5_2',1,1,'deterministic_mock','mock-video-v1','周野反应特写',
   '/storyboard-images/image_phase5_2.png',4,4,'9:16',1080,1920,24,'h264',
   '/shot-videos/video_phase5_2.mp4',repeat('b',64),'succeeded','passed','approved',true);

INSERT INTO drama.voice_profiles(
  voice_profile_id,project_id,character_id,voice_role,provider,model,provider_voice_id,
  speaking_style,version,status,review_status,lock_status,is_default
) VALUES
  ('voice_phase5_lin','p_phase1_legacy','char_lin','character','deterministic_mock','mock-tts-v1',
   'lin','克制、清晰',1,'ready','approved','locked',true),
  ('voice_phase5_zhou','p_phase1_legacy','char_zhou','character','deterministic_mock','mock-tts-v1',
   'zhou','低沉、怀疑',1,'ready','approved','locked',true);

INSERT INTO drama.dialogue_audio(
  dialogue_audio_id,project_id,episode_id,scene_id,dialogue_id,character_id,voice_profile_id,
  generation_version,dialogue_type,source_text,normalized_text,emotion,performance_instruction,
  requested_speed,provider,model,storage_url,waveform_url,format,sample_rate,channels,
  actual_duration_ms,content_hash,status,auto_qc_status,review_status,is_current
) VALUES
  ('audio_phase5_1','p_phase1_legacy','ep_phase1_legacy_001','scene_phase5_post','dlg_phase5_1',
   'char_lin','voice_phase5_lin',1,'dialogue','门不是风吹开的。','门不是风吹开的。','克制',
   '短停顿后压低音量',1,'deterministic_mock','mock-tts-v1','/dialogue-audio/audio_phase5_1.wav',
   '/waveforms/audio_phase5_1.json','wav',48000,1,1800,repeat('c',64),'succeeded','passed','approved',true),
  ('audio_phase5_2','p_phase1_legacy','ep_phase1_legacy_001','scene_phase5_post','dlg_phase5_2',
   'char_zhou','voice_phase5_zhou',1,'dialogue','钥匙在你手里？','钥匙在你手里？','怀疑',
   '尾音上扬',1,'deterministic_mock','mock-tts-v1','/dialogue-audio/audio_phase5_2.wav',
   '/waveforms/audio_phase5_2.json','wav',48000,1,1700,repeat('d',64),'succeeded','passed','approved',true);

INSERT INTO drama.subtitle_cues(
  subtitle_cue_id,project_id,episode_id,scene_id,shot_id,dialogue_id,dialogue_audio_id,
  sequence_number,speaker_name,text,start_ms,end_ms,duration_ms,style_config,status,
  cue_version,is_current,approval_state
) VALUES
  ('subtitle_phase5_1','p_phase1_legacy','ep_phase1_legacy_001','scene_phase5_post','shot_phase5_1',
   'dlg_phase5_1','audio_phase5_1',1,'林夏','门不是风吹开的。',800,2600,1800,
   '{"style":"condensed_minimal"}','approved',1,true,'approved'),
  ('subtitle_phase5_2','p_phase1_legacy','ep_phase1_legacy_001','scene_phase5_post','shot_phase5_2',
   'dlg_phase5_2','audio_phase5_2',1,'周野','钥匙在你手里？',4300,6000,1700,
   '{"style":"condensed_minimal"}','approved',1,true,'approved');

INSERT INTO drama.episode_audio_plans(
  audio_plan_id,project_id,episode_id,script_id,version,dialogue_audio_ids,
  bgm_cues,sound_effect_cues,ambience_cues,estimated_duration_ms,status,review_status
) VALUES(
  'audio_plan_phase5','p_phase1_legacy','ep_phase1_legacy_001','script_phase5_post',1,
  '["audio_phase5_1","audio_phase5_2"]','[]','[]','[]',8000,'completed','approved'
);

INSERT INTO drama.edit_timelines(
  timeline_id,project_id,episode_id,script_id,storyboard_id,audio_plan_id,version,
  resolution,aspect_ratio,fps,video_codec,audio_codec,sample_rate,target_duration_ms,
  tracks,transitions,subtitle_config,render_config,source_versions,status,
  editing_template_version_id,version_reason,approval_state,is_current
) VALUES(
  'timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','script_phase5_post',
  'storyboard_phase5_post','audio_plan_phase5',1,'1080x1920','9:16',24,'h264','aac',48000,8000,
  '{"video":1,"dialogue":1,"subtitle":1}','[{"type":"cut"}]',
  '{"style":"condensed_minimal"}','{"bgm_ducking_enabled":true}',
  '{"source_version_id":"sv_legacy_novel_phase1_legacy","ir_revision_id":"ir_phase1_001","adaptation_spec_version_id":"adaptation_spec_version_phase1_001","prompt_version":"mock-prompt-v1","model_version":"deterministic-mock-v1"}',
  'completed','etv_system_suspense_v1','mock_full_chain','approved',true
);

INSERT INTO drama.edit_timeline_items(
  timeline_item_id,timeline_id,project_id,episode_id,track_type,track_number,sequence_number,
  entity_type,entity_id,source_url,timeline_start_ms,timeline_end_ms,source_in_ms,
  source_out_ms,duration_ms,volume,fade_in_ms,fade_out_ms,status
) VALUES
  ('item_phase5_video_1','timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','video',1,1,
   'shot','shot_phase5_1','/shot-videos/video_phase5_1.mp4',0,4000,0,4000,4000,1,0,0,'completed'),
  ('item_phase5_video_2','timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','video',1,2,
   'shot','shot_phase5_2','/shot-videos/video_phase5_2.mp4',4000,8000,0,4000,4000,1,0,0,'completed'),
  ('item_phase5_dialogue_1','timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','dialogue',1,1,
   'dialogue','dlg_phase5_1','/dialogue-audio/audio_phase5_1.wav',800,2600,0,1800,1800,1,20,40,'completed'),
  ('item_phase5_dialogue_2','timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','dialogue',1,2,
   'dialogue','dlg_phase5_2','/dialogue-audio/audio_phase5_2.wav',4300,6000,0,1700,1700,1,20,40,'completed'),
  ('item_phase5_subtitle_1','timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','subtitle',1,1,
   'dialogue','dlg_phase5_1',NULL,800,2600,0,NULL,1800,1,0,0,'completed'),
  ('item_phase5_subtitle_2','timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','subtitle',1,2,
   'dialogue','dlg_phase5_2',NULL,4300,6000,0,NULL,1700,1,0,0,'completed'),
  ('item_phase5_bgm','timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','bgm',1,1,
   'sound_cue','sound_cue_phase5_bgm','/sound/bgm-suspense.wav',0,8000,0,8000,8000,.40,300,500,'completed'),
  ('item_phase5_ambience','timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','ambience',1,1,
   'sound_cue','sound_cue_phase5_ambience','/sound/old-house.wav',0,8000,0,8000,8000,.25,500,500,'completed'),
  ('item_phase5_door','timeline_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','sound_effect',1,1,
   'sound_cue','sound_cue_phase5_door','/sound/door-creak.wav',200,1400,0,1200,1200,.85,20,80,'completed');

INSERT INTO drama.character_performance_bibles(
  performance_bible_id,project_id,character_id,character_version,version,speech,acting,
  relational_voices,appearance,locked_fields,allowed_fields,change_reasons,source_refs,
  status,content_hash,created_by
) VALUES(
  'pb_phase5_lin_v1','p_phase1_legacy','char_lin','character-v1',1,
  '{"pace":"calm","pitch":"mid","pauses":"short"}','{"habit":"touches key","taboo":"exaggerated panic"}',
  '{"char_zhou":"restrained distrust"}','{"age":28,"hair":"low ponytail","body":"upright"}',
  '["speech.pitch","appearance.age","appearance.hair"]','["acting.emotion"]',
  '{"v1":"phase5 mock"}','{"source_span_ids":["span_legacy_full_ch_phase1_legacy_001"],"fact_revision_ids":["fact_revision_phase1_state_001"]}',
  'locked',repeat('e',64),'phase5-fixture'
);

INSERT INTO drama.continuity_ledger_entries(
  continuity_entry_id,project_id,episode_id,episode_number,scene_id,shot_id,scope,
  sequence_number,input_state,output_state,validation_status,diagnostics,state_hash
) VALUES
  ('continuity_phase5_1','p_phase1_legacy','ep_phase1_legacy_001',1,'scene_phase5_post',
   'shot_phase5_1','shot',1,'{"char_lin":{"position":"left","prop":"none"}}',
   '{"char_lin":{"position":"left","prop":"key"}}','valid','[]',repeat('f',64)),
  ('continuity_phase5_2','p_phase1_legacy','ep_phase1_legacy_001',1,'scene_phase5_post',
   'shot_phase5_2','shot',2,'{"char_lin":{"position":"left","prop":"key"}}',
   '{"char_lin":{"position":"left","prop":"key"},"char_zhou":{"position":"right"}}',
   'valid','[]',repeat('1',64));

INSERT INTO drama.shot_handoffs(
  shot_handoff_id,project_id,episode_id,from_shot_id,to_shot_id,target_tail_frame_ref,
  reference_head_frame_ref,pose_constraints,gaze_constraint,motion_direction,
  from_action_phase,to_action_phase,shot_size_constraint,composition_constraint,version,status
) VALUES(
  'handoff_phase5_1_2','p_phase1_legacy','ep_phase1_legacy_001','shot_phase5_1','shot_phase5_2',
  '/frames/shot_phase5_1_tail.png','/frames/shot_phase5_2_head.png',
  '{"char_lin":"holding_key"}','zhou looks left','left_to_right','door_open_complete',
  'reaction_start','close','zhou right',1,'validated'
);

INSERT INTO drama.visual_qc_runs(
  visual_qc_run_id,project_id,episode_id,fixture_id,provider,status,issue_count,started_at,completed_at
) VALUES(
  'vqc_phase5','p_phase1_legacy','ep_phase1_legacy_001','phase5-full-chain',
  'deterministic_mock','completed',1,now(),now()
);
INSERT INTO drama.visual_qc_issues(
  visual_qc_issue_id,visual_qc_run_id,project_id,episode_id,scene_id,shot_id,
  category,severity,timecode_ms,frame_number,evidence,recommendation,status
) VALUES(
  'vqi_phase5','vqc_phase5','p_phase1_legacy','ep_phase1_legacy_001','scene_phase5_post',
  'shot_phase5_2','gaze_error','major',4250,102,'{"expected":"left","actual":"front"}',
  '只重建第二镜前 500ms 的视线动作','open'
);
INSERT INTO drama.quality_issue_edit_links(
  quality_issue_edit_link_id,project_id,episode_id,issue_kind,issue_id,entity_type,
  entity_id,timecode_start_ms,timecode_end_ms,editor_path
) VALUES(
  'qel_phase5_visual','p_phase1_legacy','ep_phase1_legacy_001','visual_qc','vqi_phase5',
  'shot','shot_phase5_2',4000,4500,
  '/projects/p_phase1_legacy/episodes/ep_phase1_legacy_001/workbench?tab=storyboard&shot_id=shot_phase5_2'
),(
  'qel_phase5_quality','p_phase1_legacy','ep_phase1_legacy_001','quality','quality_issue_phase5_1',
  'shot','shot_phase5_2',4300,5500,
  '/projects/p_phase1_legacy/episodes/ep_phase1_legacy_001/workbench?tab=sound&shot_id=shot_phase5_2'
);

INSERT INTO drama.episode_masters(
  master_id,project_id,episode_id,timeline_id,generation_version,master_type,storage_url,
  width,height,aspect_ratio,fps,duration_ms,video_codec,audio_codec,sample_rate,
  loudness_lufs,peak_db,content_hash,status,is_current
) VALUES(
  'master_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','timeline_phase5_v1',1,
  'final','/results/master_phase5_v1.mp4',1080,1920,'9:16',24,8000,'h264','aac',48000,
  -16,-1,repeat('2',64),'ready',true
);
INSERT INTO drama.qc_reports(
  qc_report_id,project_id,episode_id,master_id,technical_report,subtitle_report,
  content_report,compliance_report,overall_score,severity,blocking_issues,warnings,
  recommended_actions,routing_decisions,status,version
) VALUES(
  'qc_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001','master_phase5_v1',
  '{"av_sync":"passed","loudness_lufs":-16}','{"safe_area":"passed"}',
  '{"continuity":"warning"}','{"license":"passed"}',92,'warning','[]',
  '["shot_phase5_2 gaze"]','["local redo"]','["workbench:shot_phase5_2@4250"]','completed',1
);

INSERT INTO drama.artifacts(
  artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
  validity_status,is_current,idempotency_key,metadata
) VALUES
  ('artifact_phase5_dialogue','episode_script','p_phase1_legacy','dlg_phase5_1',1,repeat('3',64),'valid',true,'phase5:artifact:dialogue','{"mock":true}'),
  ('artifact_phase5_audio','dialogue_audio','p_phase1_legacy','audio_phase5_1',1,repeat('c',64),'valid',true,'phase5:artifact:audio','{"mock":true}'),
  ('artifact_phase5_timeline','edit_timeline','p_phase1_legacy','timeline_phase5_v1',1,repeat('4',64),'valid',true,'phase5:artifact:timeline','{"mock":true}'),
  ('artifact_phase5_master','episode_master','p_phase1_legacy','master_phase5_v1',1,repeat('2',64),'valid',true,'phase5:artifact:master','{"mock":true}');

INSERT INTO drama.artifact_dependencies(
  artifact_dependency_id,upstream_artifact_id,downstream_artifact_id,dependency_type,
  dependency_selector,observed_upstream_hash,invalidates_on,idempotency_key
) VALUES
  ('dependency_phase5_dialogue_audio','artifact_phase5_dialogue','artifact_phase5_audio',
   'dialogue_to_audio','{"dialogue_id":"dlg_phase5_1"}',repeat('3',64),
   '["content_changed","removed"]','phase5:dependency:dialogue-audio'),
  ('dependency_phase5_audio_timeline','artifact_phase5_audio','artifact_phase5_timeline',
   'audio_to_exact_timeline_interval','{"dialogue_id":"dlg_phase5_1","start_ms":800,"end_ms":2600}',
   repeat('c',64),'["content_changed","removed"]','phase5:dependency:audio-timeline'),
  ('dependency_phase5_timeline_master','artifact_phase5_timeline','artifact_phase5_master',
   'timeline_to_master','{"episode_id":"ep_phase1_legacy_001"}',repeat('4',64),
   '["content_changed","removed"]','phase5:dependency:timeline-master');

INSERT INTO drama.artifact_source_evidence(
  artifact_source_evidence_id,artifact_id,source_span_id,fact_revision_id,evidence_role,idempotency_key
) VALUES(
  'evidence_phase5_dialogue','artifact_phase5_dialogue',
  'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001','source',
  'phase5:evidence:dialogue'
);

INSERT INTO drama.artifact_provenance_events(
  artifact_provenance_event_id,artifact_id,event_type,source_span_id,fact_revision_id,
  adaptation_spec_version_id,prompt_version,model_version,details,actor
) VALUES
  ('provenance_phase5_dialogue','artifact_phase5_dialogue','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','episode-script-prompt-v1','deterministic-mock-v1',
   '{"stage":"script"}','phase5-fixture'),
  ('provenance_phase5_audio','artifact_phase5_audio','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','tts-prompt-v1','mock-tts-v1',
   '{"stage":"voice"}','phase5-fixture'),
  ('provenance_phase5_timeline','artifact_phase5_timeline','mixed',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','editing-template:suspense:v1','media-worker-v1',
   '{"stage":"edit"}','phase5-fixture'),
  ('provenance_phase5_master','artifact_phase5_master','rendered',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','render-manifest-v1','ffmpeg-6.1.2',
   '{"stage":"master"}','phase5-fixture'),
  ('provenance_phase5_diagnostic','artifact_phase5_diagnostic','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','diagnostic-prompt-v1','deterministic-mock-v1',
   '{"stage":"adaptation_diagnosis"}','phase5-fixture'),
  ('provenance_phase5_pacing','artifact_phase5_pacing','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','pacing-prompt-v1','deterministic-mock-v1',
   '{"stage":"pacing"}','phase5-fixture'),
  ('provenance_phase5_beat','artifact_phase5_beat','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','pacing-prompt-v1','deterministic-mock-v1',
   '{"stage":"pacing_beat"}','phase5-fixture'),
  ('provenance_phase5_quality','artifact_phase5_quality','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','quality-prompt-v1','deterministic-mock-v1',
   '{"stage":"quality_score"}','phase5-fixture'),
  ('provenance_phase5_candidate_1','artifact_phase5_candidate_1','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','candidate-prompt-v1','deterministic-mock-v1',
   '{"stage":"candidate","manual_edit_record":null}','phase5-fixture'),
  ('provenance_phase5_candidate_2','artifact_phase5_candidate_2','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','candidate-prompt-v1','deterministic-mock-v1',
   '{"stage":"candidate","manual_edit_record":null}','phase5-fixture'),
  ('provenance_phase5_selection','artifact_phase5_selection','human_edit',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','candidate-selection-v1','human-confirmed',
   '{"stage":"candidate_selection","manual_edit_record":{"actor":"phase5-fixture"}}','phase5-fixture'),
  ('provenance_phase5_sound_bgm','artifact_phase5_sound_bgm','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','sound-prompt-v1','mock-audio-v1',
   '{"stage":"sound","source_hint":"bgm_hint"}','phase5-fixture'),
  ('provenance_phase5_sound_ambience','artifact_phase5_sound_ambience','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','sound-prompt-v1','mock-audio-v1',
   '{"stage":"sound","source_hint":"atmosphere"}','phase5-fixture'),
  ('provenance_phase5_sound_door','artifact_phase5_sound_door','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','sound-prompt-v1','mock-audio-v1',
   '{"stage":"sound","source_hint":"sound_effect_hint"}','phase5-fixture'),
  ('provenance_phase5_sound_bgm_noir','artifact_phase5_sound_bgm_noir','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','sound-prompt-v2','mock-audio-v1',
   '{"stage":"sound_style_alternative"}','phase5-fixture'),
  ('provenance_phase5_sound_ambience_noir','artifact_phase5_sound_ambience_noir','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','sound-prompt-v2','mock-audio-v1',
   '{"stage":"sound_style_alternative"}','phase5-fixture'),
  ('provenance_phase5_sound_door_noir','artifact_phase5_sound_door_noir','generated',
   'span_legacy_full_ch_phase1_legacy_001','fact_revision_phase1_event_001',
   'adaptation_spec_version_phase1_001','sound-prompt-v2','mock-audio-v1',
   '{"stage":"sound_style_alternative"}','phase5-fixture');
UPDATE drama.artifact_provenance_events
SET manual_edit_record='{"actor":"phase5-fixture","action":"candidate_confirmed","before":null,"after":"candidate_phase5_1"}'
WHERE artifact_provenance_event_id='provenance_phase5_selection';

INSERT INTO drama.creative_workspace_versions(
  creative_workspace_version_id,project_id,episode_id,version,script_id,storyboard_id,
  pacing_plan_id,candidate_selection_id,timeline_id,source_versions,performance_bible_refs,continuity_entry_ids,
  quality_report_ids,layout,status,is_current,change_reason,created_by
) VALUES(
  'workspace_phase5_v1','p_phase1_legacy','ep_phase1_legacy_001',1,'script_phase5_post',
  'storyboard_phase5_post','pacing_phase5_v1','selection_phase5_1','timeline_phase5_v1',
  '{"source_version_id":"sv_legacy_novel_phase1_legacy","ir_revision_id":"ir_phase1_001","adaptation_spec_version_id":"adaptation_spec_version_phase1_001","prompt_version":"phase5-mock-v1","model_version":"deterministic-mock-v1"}',
  '{"char_lin":"pb_phase5_lin_v1"}','["continuity_phase5_1","continuity_phase5_2"]',
  '["quality_phase5_v1","qc_phase5_v1"]','{"scene_order":["scene_phase5_post"],"active_tab":"script"}',
  'approved',true,'full mock chain assembled','phase5-fixture'
);

COMMIT;
