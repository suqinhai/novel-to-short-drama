--
-- PostgreSQL database dump
--

-- Dumped from database version 16.4
-- Dumped by pg_dump version 16.4

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Data for Name: projects; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.projects VALUES (1, 'p_phase1_legacy', '旧数据升级样例', 12, 90, '写实', '9:16', '抖音', 'story_bible_approved', 'waiting_review', true, '{}', NULL, '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:58.97446+08', NULL, 'adaptation_spec_version_phase1_001', 'effective') ON CONFLICT DO NOTHING;


--
-- Data for Name: adaptation_specs; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.adaptation_specs VALUES (1, 'adaptation_spec_phase1_001', 'p_phase1_legacy', '旧项目 Phase 1 Spec', true, 1, 'fixture:spec:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.adaptation_specs VALUES (2, 'adaptation_spec_phase3_e2e', 'p_phase1_legacy', 'Phase 3 compiler E2E', false, 1, 'fixture:phase3:spec', '2026-08-17 12:14:59.388232+08', '2026-08-17 12:14:59.388232+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: operations; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.operations VALUES (1, 'operation_phase1_ir_001', 'trace_phase1_ir_001', 'ir_extraction', 'source_version', 'sv_legacy_novel_phase1_legacy', 'completed', 'fixture:operation:ir:001', '1111111111111111111111111111111111111111111111111111111111111111', 'completed', NULL, '{}', 'ir_revision', 'ir_phase1_001', NULL, NULL, NULL, NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.operations VALUES (2, 'operation_phase1_spec_001', 'trace_phase1_spec_001', 'spec_validation', 'adaptation_spec_version', 'adaptation_spec_version_phase1_001', 'completed', 'fixture:operation:spec:001', '8888888888888888888888888888888888888888888888888888888888888888', 'completed', NULL, '{}', 'adaptation_spec_version', 'adaptation_spec_version_phase1_001', NULL, NULL, NULL, NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.operations VALUES (3, 'operation_phase1_compile_001', 'trace_phase1_001', 'adaptation_compile', 'adaptation_spec_version', 'adaptation_spec_version_phase1_001', 'completed', 'fixture:operation:compile:001', '9999999999999999999999999999999999999999999999999999999999999999', 'completed', NULL, '{}', 'adaptation_plan', 'adaptation_plan_phase1_001', NULL, NULL, NULL, NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.operations VALUES (4, 'operation_phase1_invalidation_001', 'trace_phase1_002', 'invalidation_scan', 'artifact', 'artifact_phase1_fact_001', 'completed', 'fixture:operation:invalidation:001', 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff', 'completed', NULL, '{}', 'invalidation_task', 'invalidation_task_phase1_001', NULL, NULL, NULL, NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.operations VALUES (5, 'operation_phase3_spec_e2e', 'trace_phase3_spec_e2e', 'spec_validation', 'adaptation_spec_version', 'adaptation_spec_version_phase3_e2e', 'completed', 'fixture:phase3:spec-operation', '1111111111111111111111111111111111111111111111111111111111111111', 'finished', NULL, '{}', 'adaptation_spec_version', 'adaptation_spec_version_phase3_e2e', NULL, NULL, NULL, NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, '2026-08-17 12:14:59.393756+08', '2026-08-17 12:14:59.393756+08', '2026-08-17 12:14:59.393756+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.operations VALUES (6, 'operation_phase3_compile_e2e', 'trace_phase3_compile_e2e', 'adaptation_compile', 'project', 'p_phase1_legacy', 'pending', 'fixture:phase3:compile-operation', '3333333333333333333333333333333333333333333333333333333333333333', 'queued', NULL, '{}', NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, NULL, '2026-08-17 12:14:59.422747+08', '2026-08-17 12:14:59.422747+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.operations VALUES (7, 'operation_phase5_diagnostic', 'trace_phase5_diagnostic', 'spec_validation', 'project', 'p_phase1_legacy', 'completed', 'phase5:operation:diagnostic', '5555555555555555555555555555555555555555555555555555555555555555', 'finished', NULL, '{}', 'adaptation_diagnostic_report', 'diagnostic_phase5_v1', NULL, NULL, NULL, NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.operations VALUES (8, 'operation_phase5_pacing', 'trace_phase5_pacing', 'adaptation_compile', 'project', 'p_phase1_legacy', 'completed', 'phase5:operation:pacing', '6666666666666666666666666666666666666666666666666666666666666666', 'finished', NULL, '{}', 'pacing_plan', 'pacing_phase5_v1', NULL, NULL, NULL, NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.operations VALUES (9, 'operation_phase5_quality', 'trace_phase5_quality', 'invalidation_scan', 'artifact', 'artifact_phase5_pacing', 'completed', 'phase5:operation:quality', '7777777777777777777777777777777777777777777777777777777777777777', 'finished', NULL, '{}', 'quality_score_report', 'quality_phase5_v1', NULL, NULL, NULL, NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: source_works; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.source_works VALUES (1, 'sw_legacy_novel_phase1_legacy', '旧数据升级样例', NULL, 'active', 1, 'migration:06:source-work:novel_phase1_legacy', '{"legacy_novel_id": "novel_phase1_legacy", "migration_batch_id": "phase1-legacy-v1"}', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:47.991101+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: source_versions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.source_versions VALUES (1, 'sv_legacy_novel_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 1, NULL, 'published', true, '157964e9bc42fca342ab01816fcc7ac0f1e24601cf8a0f5c1b684e51bda5a1d2', 'legacy-clean-v1', 22, 2, 1, 'migration:06:source-version:novel_phase1_legacy', '{"source_type": "text", "legacy_novel_id": "novel_phase1_legacy"}', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:47.991101+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: narrative_ir_revisions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.narrative_ir_revisions VALUES (1, 'ir_phase1_001', 'operation_phase1_ir_001', 'ir_extraction', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 1, 'narrative-extraction.v1', 'fixture-v1', 'published', true, '1111111111111111111111111111111111111111111111111111111111111111', '2222222222222222222222222222222222222222222222222222222222222222', 'fixture:ir:001', '{}', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', 'full', NULL, '[]', NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: project_source_bindings; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.project_source_bindings VALUES (1, 'psb_legacy_novel_phase1_legacy', 'p_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'primary', true, 'migration:06:project-binding:novel_phase1_legacy', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:47.991101+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: adaptation_spec_versions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.adaptation_spec_versions VALUES (1, 'adaptation_spec_version_phase1_001', 'operation_phase1_spec_001', 'spec_validation', 'adaptation_spec_phase1_001', 'p_phase1_legacy', 'psb_legacy_novel_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 1, 'sv_legacy_novel_phase1_legacy', 'ir_phase1_001', 'active', '抖音', '{"age_band": "18-35"}', 12, 90, 'union', 'adaptation-rules-v1', '8888888888888888888888888888888888888888888888888888888888888888', 1, 'fixture:spec-version:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.adaptation_spec_versions VALUES (2, 'adaptation_spec_version_phase3_e2e', 'operation_phase3_spec_e2e', 'spec_validation', 'adaptation_spec_phase3_e2e', 'p_phase1_legacy', 'psb_legacy_novel_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 1, 'sv_legacy_novel_phase1_legacy', 'ir_phase1_001', 'active', 'fixture', '{}', 1, 90, 'chapters_only', 'adaptation-rules-v1', '2222222222222222222222222222222222222222222222222222222222222222', 1, 'fixture:phase3:spec-version', '2026-08-17 12:14:59.416707+08', '2026-08-17 12:14:59.39793+08', '2026-08-17 12:14:59.416707+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: artifact_types; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.artifact_types VALUES ('source_version', 'immutable source version', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('chapter_revision', 'immutable chapter revision', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('narrative_fact_revision', 'versioned narrative fact', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('story_arc_revision', 'versioned story arc', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('adaptation_spec_version', 'versioned adaptation specification', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('adaptation_plan', 'compiler adaptation plan', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('adaptation_episode_plan', 'compiler episode plan', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('season', 'legacy-compatible season', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('episode_outline', 'legacy-compatible episode outline', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('episode_script', 'episode script', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('script_scene', 'script scene', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('storyboard', 'storyboard', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('storyboard_shot', 'storyboard shot', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('generated_asset', 'generated visual asset', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('storyboard_image', 'storyboard image', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('shot_video', 'shot video', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('dialogue_audio', 'dialogue audio', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('edit_timeline', 'edit timeline', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('episode_master', 'episode master', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('qc_report', 'quality report', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('publication_metadata', 'publication metadata', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('adaptation_diagnostic_report', 'immutable explainable adaptation diagnosis', '2026-08-17 12:14:51.794204+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('pacing_plan', 'immutable season pacing plan', '2026-08-17 12:14:51.794204+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('pacing_beat', 'versioned editable drama beat', '2026-08-17 12:14:51.794204+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('quality_score_report', 'immutable explainable quality score', '2026-08-17 12:14:51.794204+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('candidate_version', 'immutable generated candidate version', '2026-08-17 12:14:52.318402+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('candidate_selection', 'approved candidate or composition snapshot', '2026-08-17 12:14:52.318402+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('performance_bible', 'versioned character performance contract', '2026-08-17 12:14:53.2021+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('continuity_ledger', 'scene and shot continuity state', '2026-08-17 12:14:53.2021+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('shot_handoff', 'adjacent shot boundary and action relay', '2026-08-17 12:14:53.2021+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('visual_qc_issue', 'frame-located cross-shot visual QC issue', '2026-08-17 12:14:53.2021+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('dialogue_timing', 'versioned speaker/audio/lip interval', '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('sound_asset', 'versioned licensed BGM, ambience or SFX asset', '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('sound_cue', 'versioned sound-to-event timecode cue', '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('editing_template', 'versioned genre editing strategy', '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('creative_workspace', 'versioned view over script, storyboard and timeline', '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('narrative_ir', 'published full Narrative IR revision', '2026-08-17 12:14:54.186635+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('visual_profile', 'approved locked visual profile', '2026-08-17 12:14:54.186635+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('editing_template_binding', 'current published editing template binding', '2026-08-17 12:14:54.186635+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_types VALUES ('subtitle_cue', 'versioned subtitle cue materialization', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: artifacts; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.artifacts VALUES (1, 'artifact_phase1_fact_001', 'narrative_fact_revision', NULL, 'fact_revision_phase1_event_001', 1, 'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd', 'valid', true, 'fixture:artifact:fact:001', '{}', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (2, 'artifact_phase1_episode_plan_001', 'adaptation_episode_plan', 'p_phase1_legacy', 'adaptation_episode_plan_phase1_001', 1, 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee', 'valid', true, 'fixture:artifact:episode-plan:001', '{}', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (3, 'artifact_phase5_diagnostic', 'adaptation_diagnostic_report', 'p_phase1_legacy', 'diagnostic_phase5_v1', 1, '5555555555555555555555555555555555555555555555555555555555555555', 'valid', true, 'phase5:artifact:diagnostic', '{"model_version": "deterministic-mock-v1", "prompt_version": "diagnostic-prompt-v1"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (4, 'artifact_phase5_pacing', 'pacing_plan', 'p_phase1_legacy', 'pacing_phase5_v1', 1, '6666666666666666666666666666666666666666666666666666666666666666', 'valid', true, 'phase5:artifact:pacing', '{"model_version": "deterministic-mock-v1", "prompt_version": "pacing-prompt-v1"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (5, 'artifact_phase5_beat', 'pacing_beat', 'p_phase1_legacy', 'beat_phase5_hook', 1, '7777777777777777777777777777777777777777777777777777777777777777', 'valid', true, 'phase5:artifact:beat', '{"source_span_id": "span_legacy_full_ch_phase1_legacy_001"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (6, 'artifact_phase5_quality', 'quality_score_report', 'p_phase1_legacy', 'quality_phase5_v1', 1, '8888888888888888888888888888888888888888888888888888888888888888', 'valid', true, 'phase5:artifact:quality', '{"model_version": "deterministic-mock-v1", "prompt_version": "quality-prompt-v1"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (7, 'artifact_phase5_candidate_1', 'candidate_version', 'p_phase1_legacy', 'candidate_phase5_1', 1, '9999999999999999999999999999999999999999999999999999999999999999', 'valid', true, 'phase5:artifact:candidate:1', '{}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (8, 'artifact_phase5_candidate_2', 'candidate_version', 'p_phase1_legacy', 'candidate_phase5_2', 1, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'valid', true, 'phase5:artifact:candidate:2', '{}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (9, 'artifact_phase5_selection', 'candidate_selection', 'p_phase1_legacy', 'selection_phase5_1', 1, 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 'valid', true, 'phase5:artifact:selection', '{}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (10, 'artifact_phase5_sound_bgm', 'sound_asset', 'p_phase1_legacy', 'sound_phase5_bgm', 1, '1111111111111111111111111111111111111111111111111111111111111111', 'valid', true, 'phase5:sound:bgm', '{"source_hint": "低频悬疑脉冲"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (11, 'artifact_phase5_sound_ambience', 'sound_asset', 'p_phase1_legacy', 'sound_phase5_ambience', 1, '2222222222222222222222222222222222222222222222222222222222222222', 'valid', true, 'phase5:sound:ambience', '{"source_hint": "旧宅夜间底噪"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (12, 'artifact_phase5_sound_door', 'sound_asset', 'p_phase1_legacy', 'sound_phase5_door', 1, '3333333333333333333333333333333333333333333333333333333333333333', 'valid', true, 'phase5:sound:door', '{"source_hint": "老木门吱呀"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (13, 'artifact_phase5_sound_bgm_noir', 'sound_asset', 'p_phase1_legacy', 'sound_phase5_bgm_noir', 1, '4444444444444444444444444444444444444444444444444444444444444444', 'valid', true, 'phase5:sound:bgm:noir', '{"style": "cinematic_noir"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (14, 'artifact_phase5_sound_ambience_noir', 'sound_asset', 'p_phase1_legacy', 'sound_phase5_ambience_noir', 1, '5555555555555555555555555555555555555555555555555555555555555555', 'valid', true, 'phase5:sound:ambience:noir', '{"style": "cinematic_noir"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (15, 'artifact_phase5_sound_door_noir', 'sound_asset', 'p_phase1_legacy', 'sound_phase5_door_noir', 1, '6666666666666666666666666666666666666666666666666666666666666666', 'valid', true, 'phase5:sound:door:noir', '{"style": "cinematic_noir"}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (16, 'artifact_phase5_dialogue', 'episode_script', 'p_phase1_legacy', 'dlg_phase5_1', 1, '3333333333333333333333333333333333333333333333333333333333333333', 'valid', true, 'phase5:artifact:dialogue', '{"mock": true}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (17, 'artifact_phase5_audio', 'dialogue_audio', 'p_phase1_legacy', 'audio_phase5_1', 1, 'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', 'valid', true, 'phase5:artifact:audio', '{"mock": true}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (18, 'artifact_phase5_timeline', 'edit_timeline', 'p_phase1_legacy', 'timeline_phase5_v1', 1, '4444444444444444444444444444444444444444444444444444444444444444', 'valid', true, 'phase5:artifact:timeline', '{"mock": true}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (19, 'artifact_phase5_master', 'episode_master', 'p_phase1_legacy', 'master_phase5_v1', 1, '2222222222222222222222222222222222222222222222222222222222222222', 'valid', true, 'phase5:artifact:master', '{"mock": true}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (20, 'artifact_native_fcf9b8ca498433b5aaa2d31b', 'dialogue_audio', 'p_phase1_legacy', 'audio_phase5_2', 1, 'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd', 'valid', true, 'native-current:dialogue_audio:audio_phase5_2', '{"backfill": "migration-33", "episode_id": "ep_phase1_legacy_001"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (21, 'artifact_native_6d6d22366918940d598488af', 'adaptation_plan', 'p_phase1_legacy', 'adaptation_plan_phase1_001', 1, 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 'valid', true, 'native-current:adaptation_plan:adaptation_plan_phase1_001', '{"backfill": "migration-33"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (22, 'artifact_native_212ac4a6efdba91edcd980ec', 'subtitle_cue', 'p_phase1_legacy', 'subtitle_phase5_1', 1, 'ff13734a0365592f182564f2db16f68b610e99b76295055212b69eb946d2ad5e', 'valid', true, 'native-current:subtitle_cue:subtitle_phase5_1', '{"backfill": "migration-33", "episode_id": "ep_phase1_legacy_001", "dialogue_id": "dlg_phase5_1"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (23, 'artifact_native_65ce2e40d8dcad4839ea7176', 'subtitle_cue', 'p_phase1_legacy', 'subtitle_phase5_2', 1, 'b1f47c104a010c7d1a310cd8f55dff5042b7ca7e598b6d240b1b6b9573c55305', 'valid', true, 'native-current:subtitle_cue:subtitle_phase5_2', '{"backfill": "migration-33", "episode_id": "ep_phase1_legacy_001", "dialogue_id": "dlg_phase5_2"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (24, 'artifact_native_c67aa91e1f70b831c47560b7', 'storyboard_image', 'p_phase1_legacy', 'image_phase5_1', 1, 'd8e437740ba359c0fc73d6fa01a5d771cbb1dba8e5b970af25c8fc6fab49ad29', 'valid', true, 'native-current:storyboard_image:image_phase5_1', '{"shot_id": "shot_phase5_1", "backfill": "migration-33", "episode_id": "ep_phase1_legacy_001"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (25, 'artifact_native_ef474f9300f10c156f1003b6', 'storyboard_image', 'p_phase1_legacy', 'image_phase5_2', 1, '5d93c2e98d7214c820fd1e02b9062a8451f0f954706f949c86bc44bf98128610', 'valid', true, 'native-current:storyboard_image:image_phase5_2', '{"shot_id": "shot_phase5_2", "backfill": "migration-33", "episode_id": "ep_phase1_legacy_001"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (26, 'artifact_native_5605aa4599b5618664d239f6', 'shot_video', 'p_phase1_legacy', 'video_phase5_1', 1, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'valid', true, 'native-current:shot_video:video_phase5_1', '{"shot_id": "shot_phase5_1", "backfill": "migration-33", "episode_id": "ep_phase1_legacy_001"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (27, 'artifact_native_5634fa5063adc38eaff65f5b', 'shot_video', 'p_phase1_legacy', 'video_phase5_2', 1, 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 'valid', true, 'native-current:shot_video:video_phase5_2', '{"shot_id": "shot_phase5_2", "backfill": "migration-33", "episode_id": "ep_phase1_legacy_001"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (28, 'artifact_native_0f135218e4e1f683d7d81ad2', 'continuity_ledger', 'p_phase1_legacy', 'continuity_phase5_1', 1, 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff', 'valid', true, 'native-current:continuity:continuity_phase5_1', '{"scope": "shot", "backfill": "migration-33", "episode_id": "ep_phase1_legacy_001"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifacts VALUES (29, 'artifact_native_3ce25afb554c868641221dd3', 'continuity_ledger', 'p_phase1_legacy', 'continuity_phase5_2', 1, '1111111111111111111111111111111111111111111111111111111111111111', 'valid', true, 'native-current:continuity:continuity_phase5_2', '{"scope": "shot", "backfill": "migration-33", "episode_id": "ep_phase1_legacy_001"}', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: adaptation_diagnostic_reports; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.adaptation_diagnostic_reports VALUES (1, 'diagnostic_phase5_v1', 'operation_phase5_diagnostic', 'artifact_phase5_diagnostic', 'p_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'ir_phase1_001', 'adaptation_spec_version_phase1_001', 1, 'completed', 'deterministic-diagnostic-v1', '["门后悬念", "钥匙线索"]', '{"audience": "18-35", "platform": "vertical-short-drama"}', '["警觉", "怀疑", "威胁"]', '{"char_lin": ["克制", "决断"]}', '{"ending": "脚步逼近", "opening": "门自动打开"}', '[{"action": "frontload", "target": "key clue"}]', '[]', '{"mock_chain_stage": "adaptation_diagnosis"}', '5555555555555555555555555555555555555555555555555555555555555555', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: source_chapters; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.source_chapters VALUES (1, 'sch_legacy_ch_phase1_legacy_001', 'sw_legacy_novel_phase1_legacy', 'legacy:ch_phase1_legacy_001', 'active', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.source_chapters VALUES (2, 'sch_legacy_ch_phase1_legacy_002', 'sw_legacy_novel_phase1_legacy', 'legacy:ch_phase1_legacy_002', 'active', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: chapter_revisions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.chapter_revisions VALUES (1, 'cr_legacy_ch_phase1_legacy_001', 'sw_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_001', 1, '第一章', '林夏推开门。\n门后没有人。', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 14, 'migration:06:chapter-revision:ch_phase1_legacy_001', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.chapter_revisions VALUES (2, 'cr_legacy_ch_phase1_legacy_002', 'sw_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_002', 1, '第二章', '手机亮起：线索🔑出现。', 'aa9504bbaf32e0919968b3b3478451005f0509fd4fdd0f3eae31d93570729430', 12, 'migration:06:chapter-revision:ch_phase1_legacy_002', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: narrative_facts; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.narrative_facts VALUES (1, 'fact_phase1_event_001', 'sw_legacy_novel_phase1_legacy', 'event', 'event:door-open', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.narrative_facts VALUES (2, 'fact_phase1_event_002', 'sw_legacy_novel_phase1_legacy', 'event', 'event:clue-appears', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.narrative_facts VALUES (3, 'fact_phase1_state_001', 'sw_legacy_novel_phase1_legacy', 'character_state', 'state:hero:suspicion', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.narrative_facts VALUES (4, 'fact_phase1_timeline_001', 'sw_legacy_novel_phase1_legacy', 'timeline', 'timeline:event-order', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.narrative_facts VALUES (5, 'fact_phase1_foreshadow_001', 'sw_legacy_novel_phase1_legacy', 'foreshadowing', 'foreshadow:key', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: source_version_chapters; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.source_version_chapters VALUES (1, 'svc_legacy_ch_phase1_legacy_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_001', 'cr_legacy_ch_phase1_legacy_001', 1, 'migration:06:version-chapter:ch_phase1_legacy_001', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.source_version_chapters VALUES (2, 'svc_legacy_ch_phase1_legacy_002', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_002', 'cr_legacy_ch_phase1_legacy_002', 2, 'migration:06:version-chapter:ch_phase1_legacy_002', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: source_spans; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.source_spans VALUES (1, 'span_legacy_full_ch_phase1_legacy_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_001', 'cr_legacy_ch_phase1_legacy_001', 0, 38, 0, 14, 1, 1, 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', NULL, 'utf8-codepoint-v1', 'migration:06:full-span:ch_phase1_legacy_001', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.source_spans VALUES (2, 'span_legacy_full_ch_phase1_legacy_002', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_002', 'cr_legacy_ch_phase1_legacy_002', 0, 34, 0, 11, 1, 1, 'aa9504bbaf32e0919968b3b3478451005f0509fd4fdd0f3eae31d93570729430', NULL, 'utf8-codepoint-v1', 'migration:06:full-span:ch_phase1_legacy_002', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: narrative_fact_revisions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.narrative_fact_revisions VALUES (1, 'fact_revision_phase1_event_001', 'fact_phase1_event_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_001', 'cr_legacy_ch_phase1_legacy_001', 'span_legacy_full_ch_phase1_legacy_001', '3333333333333333333333333333333333333333333333333333333333333333', 0.9800, '{"statement": "林夏推开门"}', 'valid', 'fixture:fact:event:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.narrative_fact_revisions VALUES (2, 'fact_revision_phase1_event_002', 'fact_phase1_event_002', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_002', 'cr_legacy_ch_phase1_legacy_002', 'span_legacy_full_ch_phase1_legacy_002', '4444444444444444444444444444444444444444444444444444444444444444', 0.9700, '{"statement": "钥匙线索出现"}', 'valid', 'fixture:fact:event:002', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.narrative_fact_revisions VALUES (3, 'fact_revision_phase1_state_001', 'fact_phase1_state_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_001', 'cr_legacy_ch_phase1_legacy_001', 'span_legacy_full_ch_phase1_legacy_001', '5555555555555555555555555555555555555555555555555555555555555555', 0.9500, '{"statement": "林夏从平静转为警觉"}', 'valid', 'fixture:fact:state:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.narrative_fact_revisions VALUES (4, 'fact_revision_phase1_timeline_001', 'fact_phase1_timeline_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_002', 'cr_legacy_ch_phase1_legacy_002', 'span_legacy_full_ch_phase1_legacy_002', '6666666666666666666666666666666666666666666666666666666666666666', 0.9400, '{"statement": "线索在开门之后出现"}', 'valid', 'fixture:fact:timeline:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.narrative_fact_revisions VALUES (5, 'fact_revision_phase1_foreshadow_001', 'fact_phase1_foreshadow_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_002', 'cr_legacy_ch_phase1_legacy_002', 'span_legacy_full_ch_phase1_legacy_002', '7777777777777777777777777777777777777777777777777777777777777777', 0.9300, '{"statement": "钥匙成为后续线索"}', 'valid', 'fixture:fact:foreshadow:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: story_arcs; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.story_arcs VALUES (1, 'story_arc_phase1_001', 'sw_legacy_novel_phase1_legacy', 'arc:investigation', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: story_arc_revisions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.story_arc_revisions VALUES (1, 'story_arc_revision_phase1_001', 'story_arc_phase1_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_001', 'cr_legacy_ch_phase1_legacy_001', 'span_legacy_full_ch_phase1_legacy_001', '调查开始', '林夏发现异常并获得钥匙线索', 'main', 0.9600, 'fixture:story-arc:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: adaptation_diagnostic_nodes; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.adaptation_diagnostic_nodes VALUES (1, 'diagnostic_node_phase5_hook', 'diagnostic_phase5_v1', 'selling_point', NULL, 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', NULL, 1, '门后悬念', '用自动开启的门在前三秒建立异常', 0.9200, 0.2000, 'frontload', '{"source_span_id": "span_legacy_full_ch_phase1_legacy_001"}', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: compiler_runs; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.compiler_runs VALUES (1, 'compiler_run_phase1_001', 'operation_phase1_compile_001', 'adaptation_compile', 'p_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'adaptation_spec_version_phase1_001', 'ir_phase1_001', 'fixture-compiler-v1', 'completed', '9999999999999999999999999999999999999999999999999999999999999999', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'fixture:compiler-run:001', '{}', NULL, NULL, 0, 3, NULL, NULL, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.compiler_runs VALUES (2, 'compiler_run_phase3_e2e', 'operation_phase3_compile_e2e', 'adaptation_compile', 'p_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'adaptation_spec_version_phase3_e2e', 'ir_phase1_001', 'constraint-e2e-v1', 'pending', '3333333333333333333333333333333333333333333333333333333333333333', NULL, 'fixture:phase3:compile-run', '{}', NULL, NULL, 0, 3, NULL, NULL, NULL, NULL, '2026-08-17 12:14:59.424613+08', '2026-08-17 12:14:59.424613+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: adaptation_plans; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.adaptation_plans VALUES (1, 'adaptation_plan_phase1_001', 'compiler_run_phase1_001', 'p_phase1_legacy', 'adaptation_spec_version_phase1_001', 1, 'approved', true, 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '{}', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', NULL, '整季方案', 'deterministic', '{}', '[]', NULL, 'fixture', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: adaptation_episode_plans; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.adaptation_episode_plans VALUES (1, 'adaptation_episode_plan_phase1_001', 'adaptation_plan_phase1_001', 1, '异常开端', '林夏发现异常并获得钥匙线索', 90, '门自动打开', '钥匙出现', '[]', '[]', '{}', 'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '[]', '[]', '[]', '[]', '[]', '门突然自动打开', '林夏必须确认门后的异常来源', '异常空间阻止林夏离开', '钥匙线索出现', '[{"emotion": 0.6, "position": 1}, {"emotion": 0.9, "position": 2}]', 0.55000, '[]', '[]') ON CONFLICT DO NOTHING;


--
-- Data for Name: adaptation_plan_validation_runs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: adaptation_rules; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.adaptation_rules VALUES (1, 'adaptation_rule_phase1_001', 'adaptation_spec_version_phase1_001', 'must_preserve', 'hard', 'event', 'event_revision_phase1_001', 100, '{}', '主线事件', 'fixture:rule:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.adaptation_rules VALUES (2, 'adaptation_rule_phase3_e2e', 'adaptation_spec_version_phase3_e2e', 'must_preserve', 'hard', 'event', 'event_revision_phase1_001', 100, '{}', 'E2E must preserve', 'fixture:phase3:rule', '2026-08-17 12:14:59.411449+08', '2026-08-17 12:14:59.411449+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: adaptation_scope_arcs; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.adaptation_scope_arcs VALUES (1, 'scope_arc_phase1_001', 'adaptation_spec_version_phase1_001', 'p_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'ir_phase1_001', 'story_arc_revision_phase1_001', 'include', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: adaptation_scope_chapters; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.adaptation_scope_chapters VALUES (1, 'scope_chapter_phase1_001', 'adaptation_spec_version_phase1_001', 'p_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'ir_phase1_001', 'sch_legacy_ch_phase1_legacy_001', 'include', NULL, NULL, '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.adaptation_scope_chapters VALUES (2, 'scope_chapter_phase3_e2e', 'adaptation_spec_version_phase3_e2e', 'p_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'ir_phase1_001', 'sch_legacy_ch_phase1_legacy_001', 'include', NULL, NULL, '2026-08-17 12:14:59.404909+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: artifact_current_bindings; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.artifact_current_bindings VALUES (1, 'binding_phase5_candidate', 'p_phase1_legacy', 'episode', 'ep_phase1_legacy_001', 'whole', 'artifact_phase5_selection', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: artifact_dependencies; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.artifact_dependencies VALUES (1, 'artifact_dependency_phase1_001', 'artifact_phase1_fact_001', 'artifact_phase1_episode_plan_001', 'semantic_event', '{}', 'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd', '["content_changed", "removed"]', 'fixture:dependency:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (2, 'dependency_phase5_dialogue_audio', 'artifact_phase5_dialogue', 'artifact_phase5_audio', 'dialogue_to_audio', '{"dialogue_id": "dlg_phase5_1"}', '3333333333333333333333333333333333333333333333333333333333333333', '["content_changed", "removed"]', 'phase5:dependency:dialogue-audio', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (3, 'dependency_phase5_audio_timeline', 'artifact_phase5_audio', 'artifact_phase5_timeline', 'audio_to_exact_timeline_interval', '{"end_ms": 2600, "start_ms": 800, "dialogue_id": "dlg_phase5_1"}', 'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', '["content_changed", "removed"]', 'phase5:dependency:audio-timeline', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (4, 'dependency_phase5_timeline_master', 'artifact_phase5_timeline', 'artifact_phase5_master', 'timeline_to_master', '{"episode_id": "ep_phase1_legacy_001"}', '4444444444444444444444444444444444444444444444444444444444444444', '["content_changed", "removed"]', 'phase5:dependency:timeline-master', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (5, 'ad_rebuild_fa8856e8102338fb0172ebe5', 'artifact_native_6d6d22366918940d598488af', 'artifact_native_0f135218e4e1f683d7d81ad2', 'adaptation_plan_to_continuity_ledger', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_native_0f135218e4e1f683d7d81ad2', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (6, 'ad_rebuild_6ef4962f8c82d3f5d2ede1bd', 'artifact_native_6d6d22366918940d598488af', 'artifact_native_212ac4a6efdba91edcd980ec', 'adaptation_plan_to_subtitle_cue', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_native_212ac4a6efdba91edcd980ec', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (7, 'ad_rebuild_30f3436d2feb8ec2fbc43275', 'artifact_native_6d6d22366918940d598488af', 'artifact_native_3ce25afb554c868641221dd3', 'adaptation_plan_to_continuity_ledger', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_native_3ce25afb554c868641221dd3', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (8, 'ad_rebuild_b1b134dd924cb4dd60b49e58', 'artifact_native_6d6d22366918940d598488af', 'artifact_native_5605aa4599b5618664d239f6', 'adaptation_plan_to_shot_video', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_native_5605aa4599b5618664d239f6', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (9, 'ad_rebuild_7345d53df0086f7e6c8f76fb', 'artifact_native_6d6d22366918940d598488af', 'artifact_native_5634fa5063adc38eaff65f5b', 'adaptation_plan_to_shot_video', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_native_5634fa5063adc38eaff65f5b', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (10, 'ad_rebuild_e29b5ce393838afc023353c6', 'artifact_native_6d6d22366918940d598488af', 'artifact_native_65ce2e40d8dcad4839ea7176', 'adaptation_plan_to_subtitle_cue', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_native_65ce2e40d8dcad4839ea7176', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (11, 'ad_rebuild_cc2dbcc652603374b3dae4d3', 'artifact_native_6d6d22366918940d598488af', 'artifact_native_c67aa91e1f70b831c47560b7', 'adaptation_plan_to_storyboard_image', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_native_c67aa91e1f70b831c47560b7', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (12, 'ad_rebuild_2d3803abb066f9ca2b177fd7', 'artifact_native_6d6d22366918940d598488af', 'artifact_native_ef474f9300f10c156f1003b6', 'adaptation_plan_to_storyboard_image', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_native_ef474f9300f10c156f1003b6', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (13, 'ad_rebuild_eb00e1554ff6c2b339330a30', 'artifact_native_6d6d22366918940d598488af', 'artifact_native_fcf9b8ca498433b5aaa2d31b', 'adaptation_plan_to_dialogue_audio', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_native_fcf9b8ca498433b5aaa2d31b', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (14, 'ad_rebuild_6ebda8aad9040fb6c5b311aa', 'artifact_native_6d6d22366918940d598488af', 'artifact_phase5_audio', 'adaptation_plan_to_dialogue_audio', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_phase5_audio', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_dependencies VALUES (15, 'ad_rebuild_e4b141e33f2352b447e76a35', 'artifact_native_6d6d22366918940d598488af', 'artifact_phase5_timeline', 'adaptation_plan_to_edit_timeline', '{}', 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', '["content_changed", "removed"]', 'rebuild-graph:artifact_native_6d6d22366918940d598488af:artifact_phase5_timeline', '2026-08-17 12:15:00.263984+08', '2026-08-17 12:15:00.263984+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: story_bibles; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.story_bibles VALUES (1, 'sb_phase1_legacy', 'p_phase1_legacy', 1, 'approved', '[]', '[]', '[]', '[]', '[]', '[]', '[]', '[]', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:47.991101+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: seasons; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.seasons VALUES (1, 'season_phase1_legacy', 'p_phase1_legacy', 'sb_phase1_legacy', 1, '旧版第一季', 12, 90, '旧链路兼容', 'approved', 1, '{}', '[]', '{}', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:47.991101+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: episode_outlines; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.episode_outlines VALUES (1, 'ep_phase1_legacy_001', 'season_phase1_legacy', 'p_phase1_legacy', 1, '门后的线索', '林夏发现异常。', '["ch_phase1_legacy_001"]', '[]', '门自动打开', '开始调查', '未知力量阻拦', '[]', '手机出现线索', '钥匙指向下一处', '[]', '[]', 90, '[]', '[]', 'approved', 1, '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:47.991101+08', NULL, NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: prompt_templates; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: prompt_versions; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: artifact_generation_provenance; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: generation_effective_input_claims; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: artifact_input_consumptions; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: character_performance_bibles; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.character_performance_bibles VALUES (1, 'pb_phase5_lin_v1', 'p_phase1_legacy', 'char_lin', 'character-v1', 1, 'performance-bible.v1', '{"pace": "calm", "pitch": "mid", "pauses": "short"}', '{"habit": "touches key", "taboo": "exaggerated panic"}', '{"char_zhou": "restrained distrust"}', '{"age": 28, "body": "upright", "hair": "low ponytail"}', '["speech.pitch", "appearance.age", "appearance.hair"]', '["acting.emotion"]', '{"v1": "phase5 mock"}', '{"source_span_ids": ["span_legacy_full_ch_phase1_legacy_001"], "fact_revision_ids": ["fact_revision_phase1_state_001"]}', 'locked', NULL, 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee', 'phase5-fixture', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.character_performance_bibles VALUES (2, 'pb_phase5_zhou_v1', 'p_phase1_legacy', 'char_zhou', 'character-v1', 1, 'performance-bible.v1', '{"pace": "measured", "pitch": "low", "pauses": "questioning"}', '{"habit": "checks exits", "taboo": "comic delivery"}', '{"char_lin": "restrained concern"}', '{"age": 31, "body": "steady", "hair": "short"}', '["speech.pitch", "appearance.age", "appearance.hair"]', '["acting.emotion"]', '{"v1": "phase5 mock"}', '{"source_span_ids": ["span_legacy_full_ch_phase1_legacy_001"], "fact_revision_ids": ["fact_revision_phase1_state_001"]}', 'locked', NULL, 'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd', 'phase5-fixture', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: artifact_performance_bible_refs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: change_plans; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: episode_scripts; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.episode_scripts VALUES (1, 'script_phase5_post', 'p_phase1_legacy', 'season_phase1_legacy', 'ep_phase1_legacy_001', 2, '门后的线索', '门自动打开', '[]', '林夏发现钥匙', '脚步声逼近', 8, 24, 1, '{}', '{}', 'approved', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, '{"char_lin": "pb_phase5_lin_v1", "char_zhou": "pb_phase5_zhou_v1"}') ON CONFLICT DO NOTHING;


--
-- Data for Name: script_scenes; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.script_scenes VALUES (1, 'scene_phase5_post', 'script_phase5_post', 'p_phase1_legacy', 'ep_phase1_legacy_001', 1, 'location_door', '旧宅门厅', '夜', '内', '["char_lin", "char_zhou"]', '发现线索并建立威胁', '[{"description": "林夏推开门"}]', '[]', '[]', '警觉转为决断', 8, '["event_revision_phase1_001", "event_revision_phase1_002"]', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: shot_sequence_versions; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: shot_edit_plans; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: storyboards; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.storyboards VALUES (1, 'storyboard_phase5_post', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'script_phase5_post', 1, 2, 8.00, 'approved', '{}', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', '{"char_lin": "pb_phase5_lin_v1", "char_zhou": "pb_phase5_zhou_v1"}') ON CONFLICT DO NOTHING;


--
-- Data for Name: storyboard_shots; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.storyboard_shots VALUES (1, 'shot_phase5_1', 'storyboard_phase5_post', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'scene_phase5_post', 1, 1, 4.00, 'medium', 'eye_level', 'slow_push', '林夏左侧，门右侧', '["char_lin"]', 'location_door', '林夏推门后凝视门锁', '克制警觉', '["dlg_phase5_1"]', '门不是风吹开的。', '', '冷月光', '压迫', '老木门吱呀', '低频悬疑脉冲', 'cut', '写实旧宅门厅', '林夏推门，动作连续', '畸形手', NULL, '{"axis": "lin-left"}', '{"source_event_ids": ["event_revision_phase1_001"]}', 'approved', 1, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', true, 'shot_phase5_1', NULL, '{}', '{}', '{}', '{}', '', '', '', '') ON CONFLICT DO NOTHING;
INSERT INTO drama.storyboard_shots VALUES (2, 'shot_phase5_2', 'storyboard_phase5_post', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'scene_phase5_post', 2, 2, 4.00, 'close', 'eye_level', 'static', '周野右侧反应', '["char_zhou"]', 'location_door', '周野看向林夏手中的钥匙', '怀疑', '["dlg_phase5_2"]', '钥匙在你手里？', '', '冷月光', '紧张', '远处脚步逼近', '低频悬疑脉冲', 'cut', '写实旧宅门厅', '周野反应特写', '脸部漂移', NULL, '{"axis": "zhou-right"}', '{"source_event_ids": ["event_revision_phase1_002"]}', 'approved', 1, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', true, 'shot_phase5_2', NULL, '{}', '{}', '{}', '{}', '', '', '', '') ON CONFLICT DO NOTHING;


--
-- Data for Name: continuity_ledger_entries; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.continuity_ledger_entries VALUES (1, 'continuity_phase5_1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 1, 'scene_phase5_post', 'shot_phase5_1', 'shot', 1, 'continuity-ledger.v1', '{"char_lin": {"prop": "none", "position": "left"}}', '{"char_lin": {"prop": "key", "position": "left"}}', NULL, 'valid', '[]', 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', true, NULL, 1, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.continuity_ledger_entries VALUES (2, 'continuity_phase5_2', 'p_phase1_legacy', 'ep_phase1_legacy_001', 1, 'scene_phase5_post', 'shot_phase5_2', 'shot', 2, 'continuity-ledger.v1', '{"char_lin": {"prop": "key", "position": "left"}}', '{"char_lin": {"prop": "key", "position": "left"}, "char_zhou": {"position": "right"}}', NULL, 'valid', '[]', '1111111111111111111111111111111111111111111111111111111111111111', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', true, NULL, 1, NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: entity_versions; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: shot_handoffs; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.shot_handoffs VALUES (1, 'handoff_phase5_1_2', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'shot_phase5_1', 'shot_phase5_2', 'shot-handoff.v1', '/frames/shot_phase5_1_tail.png', '/frames/shot_phase5_2_head.png', '{"char_lin": "holding_key"}', 'zhou looks left', 'left_to_right', 'door_open_complete', 'reaction_start', 'close', 'zhou right', 1, 'validated', '[]', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', true, NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: generation_context_reads; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: artifact_provenance_events; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_dialogue', 'artifact_phase5_dialogue', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'episode-script-prompt-v1', 'deterministic-mock-v1', NULL, '{"stage": "script"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_audio', 'artifact_phase5_audio', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'tts-prompt-v1', 'mock-tts-v1', NULL, '{"stage": "voice"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_timeline', 'artifact_phase5_timeline', 'mixed', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'editing-template:suspense:v1', 'media-worker-v1', NULL, '{"stage": "edit"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_master', 'artifact_phase5_master', 'rendered', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'render-manifest-v1', 'ffmpeg-6.1.2', NULL, '{"stage": "master"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_diagnostic', 'artifact_phase5_diagnostic', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'diagnostic-prompt-v1', 'deterministic-mock-v1', NULL, '{"stage": "adaptation_diagnosis"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_pacing', 'artifact_phase5_pacing', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'pacing-prompt-v1', 'deterministic-mock-v1', NULL, '{"stage": "pacing"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_beat', 'artifact_phase5_beat', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'pacing-prompt-v1', 'deterministic-mock-v1', NULL, '{"stage": "pacing_beat"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_quality', 'artifact_phase5_quality', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'quality-prompt-v1', 'deterministic-mock-v1', NULL, '{"stage": "quality_score"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_candidate_1', 'artifact_phase5_candidate_1', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'candidate-prompt-v1', 'deterministic-mock-v1', NULL, '{"stage": "candidate", "manual_edit_record": null}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_candidate_2', 'artifact_phase5_candidate_2', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'candidate-prompt-v1', 'deterministic-mock-v1', NULL, '{"stage": "candidate", "manual_edit_record": null}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_sound_bgm', 'artifact_phase5_sound_bgm', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'sound-prompt-v1', 'mock-audio-v1', NULL, '{"stage": "sound", "source_hint": "bgm_hint"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_sound_ambience', 'artifact_phase5_sound_ambience', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'sound-prompt-v1', 'mock-audio-v1', NULL, '{"stage": "sound", "source_hint": "atmosphere"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_sound_door', 'artifact_phase5_sound_door', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'sound-prompt-v1', 'mock-audio-v1', NULL, '{"stage": "sound", "source_hint": "sound_effect_hint"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_sound_bgm_noir', 'artifact_phase5_sound_bgm_noir', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'sound-prompt-v2', 'mock-audio-v1', NULL, '{"stage": "sound_style_alternative"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_sound_ambience_noir', 'artifact_phase5_sound_ambience_noir', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'sound-prompt-v2', 'mock-audio-v1', NULL, '{"stage": "sound_style_alternative"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_sound_door_noir', 'artifact_phase5_sound_door_noir', 'generated', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'sound-prompt-v2', 'mock-audio-v1', NULL, '{"stage": "sound_style_alternative"}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_provenance_events VALUES ('provenance_phase5_selection', 'artifact_phase5_selection', 'human_edit', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'adaptation_spec_version_phase1_001', NULL, NULL, 'candidate-selection-v1', 'human-confirmed', '{"actor": "phase5-fixture", "after": "candidate_phase5_1", "action": "candidate_confirmed", "before": null}', '{"stage": "candidate_selection", "manual_edit_record": {"actor": "phase5-fixture"}}', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: artifact_source_evidence; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.artifact_source_evidence VALUES (1, 'artifact_evidence_phase1_001', 'artifact_phase1_episode_plan_001', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'source', 'fixture:artifact-evidence:001', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.artifact_source_evidence VALUES (2, 'evidence_phase5_dialogue', 'artifact_phase5_dialogue', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'source', 'phase5:evidence:dialogue', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: asset_dependencies; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: pacing_plan_versions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.pacing_plan_versions VALUES (1, 'pacing_phase5_v1', NULL, 'operation_phase5_pacing', 'artifact_phase5_pacing', 'p_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'ir_phase1_001', 'adaptation_spec_version_phase1_001', 'adaptation_plan_phase1_001', 'diagnostic_phase5_v1', 1, 'published', 'deterministic-pacing-v1', 8, '6666666666666666666666666666666666666666666666666666666666666666', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: quality_score_reports; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.quality_score_reports VALUES (1, 'quality_phase5_v1', 'operation_phase5_quality', 'artifact_phase5_quality', 'p_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'ir_phase1_001', 'adaptation_spec_version_phase1_001', 'pacing_phase5_v1', 'diagnostic_phase5_v1', 'artifact_phase1_episode_plan_001', NULL, 1, 'episode', '{"episode_id": "ep_phase1_legacy_001", "episode_number": 1}', 'deterministic-quality-v1', 88.00, 'completed', '8888888888888888888888888888888888888888888888888888888888888888', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: candidate_sets; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.candidate_sets VALUES (1, 'candidate_set_phase5', 'p_phase1_legacy', 'episode', 'ep_phase1_legacy_001', 'artifact_phase1_episode_plan_001', 'quality_phase5_v1', 2, '["opening", "dialogue", "ending"]', '["suspense-forward", "performance-forward"]', '["source facts", "character state", "ending threat"]', '["dialogue density", "reaction timing"]', 'deterministic-mock-v1', 'candidate-prompt-v1', 20260730, '{"temperature": 0}', 0.000000, 'CNY', 'candidate-generator-v1', 'phase5:candidate:set', '9999999999999999999999999999999999999999999999999999999999999999', '2026-08-17 12:14:59.84683+08', 'deterministic_mock', 'deterministic-generator-v2', 'deterministic_mock', 'deterministic-reviewer-v2', true, 'legacy-unfrozen', '0000000000000000000000000000000000000000000000000000000000000000', '0000000000000000000000000000000000000000000000000000000000000000', '{}', '0000000000000000000000000000000000000000000000000000000000000000') ON CONFLICT DO NOTHING;


--
-- Data for Name: candidates; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.candidates VALUES (1, 'candidate_phase5_1', 'candidate_set_phase5', NULL, 'artifact_phase5_candidate_1', 1, '悬念前置版', 'suspense-forward', '前三秒强化异常', '{"ending": "脚步逼近", "opening": "门自动打开", "dialogue": "门不是风吹开的。"}', '[{"path": "/opening", "after": "门自动打开"}]', '9999999999999999999999999999999999999999999999999999999999999999', 'deterministic-mock-v1', 'candidate-prompt-v1', 20260730, '{"temperature": 0}', '2026-08-17 12:14:59.84683+08', 'deterministic_mock') ON CONFLICT DO NOTHING;
INSERT INTO drama.candidates VALUES (2, 'candidate_phase5_2', 'candidate_set_phase5', NULL, 'artifact_phase5_candidate_2', 2, '表演留白版', 'performance-forward', '增加角色反应停顿', '{"ending": "脚步逼近", "opening": "林夏停在门前", "dialogue": "门……是从里面开的。"}', '[{"path": "/dialogue", "after": "门……是从里面开的。"}]', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'deterministic-mock-v1', 'candidate-prompt-v1', 20260731, '{"temperature": 0}', '2026-08-17 12:14:59.84683+08', 'deterministic_mock') ON CONFLICT DO NOTHING;


--
-- Data for Name: candidate_selections; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.candidate_selections VALUES (1, 'selection_phase5_1', 'candidate_set_phase5', 'candidate_phase5_1', 'artifact_phase5_selection', 'candidate', '{"selected": "candidate_phase5_1"}', '{"duration": true, "causality": true, "continuity": true, "foreshadowing": true, "character_state": true}', 'phase5-fixture', 'phase5:selection:1', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: candidate_composition_parts; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: candidate_decisions; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: candidate_execution_records; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: candidate_hard_rule_results; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.candidate_hard_rule_results VALUES (1, 'hard_rule_phase5_causality', 'selection_phase5_1', 'causality', true, 'fixture pass', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.candidate_hard_rule_results VALUES (2, 'hard_rule_phase5_duration', 'selection_phase5_1', 'duration', true, 'fixture pass', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.candidate_hard_rule_results VALUES (3, 'hard_rule_phase5_character_state', 'selection_phase5_1', 'character_state', true, 'fixture pass', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.candidate_hard_rule_results VALUES (4, 'hard_rule_phase5_foreshadowing', 'selection_phase5_1', 'foreshadowing', true, 'fixture pass', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.candidate_hard_rule_results VALUES (5, 'hard_rule_phase5_continuity', 'selection_phase5_1', 'continuity', true, 'fixture pass', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: candidate_scores; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.candidate_scores VALUES (1, 'candidate_score_phase5_1', 'candidate_phase5_1', 'quality_phase5_v1', 92.00, 94.00, 96.00, 90.00, 91.00, 89.00, 8, 12.00, '["钩子强且忠实"]', '["对白略密"]', 'candidate-scorer-v1', '2026-08-17 12:14:59.84683+08', 0.00, 0.00, '[]', 'deterministic_mock', 'deterministic-reviewer-v2') ON CONFLICT DO NOTHING;
INSERT INTO drama.candidate_scores VALUES (2, 'candidate_score_phase5_2', 'candidate_phase5_2', 'quality_phase5_v1', 87.00, 91.00, 84.00, 86.00, 94.00, 88.00, 8, 10.00, '["表演空间充足"]', '["开场钩子稍弱"]', 'candidate-scorer-v1', '2026-08-17 12:14:59.84683+08', 0.00, 0.00, '[]', 'deterministic_mock', 'deterministic-reviewer-v2') ON CONFLICT DO NOTHING;


--
-- Data for Name: candidate_selection_bindings; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.candidate_selection_bindings VALUES (1, 'csb_a1588bd813fbda5c7d328942949af547', 'p_phase1_legacy', 'episode', 'ep_phase1_legacy_001', 'whole', 'artifact_phase5_selection', 'selection_phase5_1', true, '2026-08-17 12:14:59.84683+08', NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: candidate_timecode_comments; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: change_comments; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: change_plan_events; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: change_plan_impacts; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: character_visual_profiles; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.character_visual_profiles VALUES (1, 'profile_phase18_lin', 'p_phase1_legacy', 'char_lin', 1, 'Lin', 'female', '28', '', '', '{}', '', '', '', '', '', '', '[]', '', '[]', 'realistic restrained investigator', 'identity drift', 1, 'ready', 'approved', 'locked', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.character_visual_profiles VALUES (2, 'profile_phase18_zhou', 'p_phase1_legacy', 'char_zhou', 1, 'Zhou', 'male', '31', '', '', '{}', '', '', '', '', '', '', '[]', '', '[]', 'realistic guarded witness', 'identity drift', 1, 'ready', 'approved', 'locked', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: character_costumes; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: character_performance_stage_states; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: narrative_entities; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.narrative_entities VALUES (1, 'entity_phase1_hero', 'sw_legacy_novel_phase1_legacy', 'character', 'character:林夏', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: narrative_entity_revisions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.narrative_entity_revisions VALUES (1, 'entity_revision_phase1_hero', 'entity_phase1_hero', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'sch_legacy_ch_phase1_legacy_001', 'cr_legacy_ch_phase1_legacy_001', 'span_legacy_full_ch_phase1_legacy_001', '林夏', '{"identity": "记者"}', 0.9900, 'valid', 'fixture:entity-revision:hero', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: narrative_event_revisions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.narrative_event_revisions VALUES (1, 'event_revision_phase1_001', 'fact_revision_phase1_event_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'action', '林夏推开门', 1.0000, NULL, NULL, 0.8000, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.narrative_event_revisions VALUES (2, 'event_revision_phase1_002', 'fact_revision_phase1_event_002', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'discovery', '钥匙线索出现', 2.0000, NULL, NULL, 0.9000, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: character_state_changes; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.character_state_changes VALUES (1, 'state_change_phase1_001', 'fact_revision_phase1_state_001', 'entity_revision_phase1_hero', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'alertness', '{"value": "calm"}', '{"value": "alert"}', 'event_revision_phase1_001', 1.0000, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: compiler_checkpoints; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.compiler_checkpoints VALUES (1, 'compiler_checkpoint_phase1_001', 'compiler_run_phase1_001', 'constraint_validation', 'all', 'completed', '9999999999999999999999999999999999999999999999999999999999999999', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', '{}', 'fixture:compiler-checkpoint:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: compiler_diagnostics; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.compiler_diagnostics VALUES (1, 'compiler_diagnostic_phase1_001', 'compiler_run_phase1_001', 'info', 'ALL_RULES_SATISFIED', NULL, NULL, 'Fixture validation passed', '{}', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: editing_templates; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.editing_templates VALUES ('et_system_urban_power', 'urban_power', '都市爽剧', 'system', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.editing_templates VALUES ('et_system_emotion', 'emotion', '情感剧', 'system', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.editing_templates VALUES ('et_system_suspense', 'suspense', '悬疑剧', 'system', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.editing_templates VALUES ('et_system_comedy', 'comedy', '喜剧', 'system', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.editing_templates VALUES ('et_system_action', 'action', '动作剧', 'system', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: editing_template_versions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.editing_template_versions VALUES ('etv_system_urban_power_v1', 'et_system_urban_power', 'editing-template.v1', 1, NULL, '{"audio": {"bgm_density": 0.82, "sfx_density": 0.78}, "subtitle": {"style": "bold_high_contrast", "density": "high"}, "transitions": ["cut", "whip", "flash"], "beat_strategy": "payoff_on_beat", "fast_cut_ratio": 0.62, "pause_strategy": "short_before_payoff", "repeat_emphasis": "single_key_fact", "close_up_strategy": "identity_and_counterattack", "reaction_shot_ratio": 0.28, "average_shot_length_ms": 1800}', 'eff93316a4d160e516d0a270428f10d4a551d2159b4173f7f02dcf88580c64c9', 'published', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.editing_template_versions VALUES ('etv_system_emotion_v1', 'et_system_emotion', 'editing-template.v1', 1, NULL, '{"audio": {"bgm_density": 0.66, "sfx_density": 0.24}, "subtitle": {"style": "soft_clean", "density": "medium"}, "transitions": ["cut", "dissolve"], "beat_strategy": "melody_carries_turn", "fast_cut_ratio": 0.18, "pause_strategy": "breath_and_subtext", "repeat_emphasis": "avoid_mechanical_repeat", "close_up_strategy": "emotion_and_hands", "reaction_shot_ratio": 0.48, "average_shot_length_ms": 3600}', '8e52ddf505f1ddf68473a3f8be48f5927062c76ed55091d9f7c4fe44b6bdbce8', 'published', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.editing_template_versions VALUES ('etv_system_suspense_v1', 'et_system_suspense', 'editing-template.v1', 1, NULL, '{"audio": {"bgm_density": 0.74, "sfx_density": 0.63}, "subtitle": {"style": "condensed_minimal", "density": "low"}, "transitions": ["cut", "fade", "match_cut"], "beat_strategy": "unstable_then_stop", "fast_cut_ratio": 0.34, "pause_strategy": "hold_before_reveal", "repeat_emphasis": "reframe_clue", "close_up_strategy": "clue_and_microexpression", "reaction_shot_ratio": 0.36, "average_shot_length_ms": 2700}', 'bf7bfb560ff408a97128fd0696c03a0ec6b6063b807d29b628040443bb635cdd', 'published', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.editing_template_versions VALUES ('etv_system_comedy_v1', 'et_system_comedy', 'editing-template.v1', 1, NULL, '{"audio": {"bgm_density": 0.58, "sfx_density": 0.84}, "subtitle": {"style": "playful_emphasis", "density": "high"}, "transitions": ["cut", "snap_zoom"], "beat_strategy": "punchline_hit", "fast_cut_ratio": 0.48, "pause_strategy": "comic_timing", "repeat_emphasis": "rule_of_three", "close_up_strategy": "punchline_reaction", "reaction_shot_ratio": 0.55, "average_shot_length_ms": 2100}', '58533482a2e5bb40956a2a146d56546c5fce72ace1c9d9bba737b0130fe0ac35', 'published', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.editing_template_versions VALUES ('etv_system_action_v1', 'et_system_action', 'editing-template.v1', 1, NULL, '{"audio": {"bgm_density": 0.88, "sfx_density": 0.94}, "subtitle": {"style": "compact_safe_area", "density": "low"}, "transitions": ["cut", "whip", "impact_flash"], "beat_strategy": "phase_and_hit_on_beat", "fast_cut_ratio": 0.78, "pause_strategy": "direction_only", "repeat_emphasis": "single_speed_ramp", "close_up_strategy": "impact_and_prop", "reaction_shot_ratio": 0.18, "average_shot_length_ms": 1200}', 'fb067308f77a053b71817d2bb727dd3ccf562cbcea22855fe7058e9db2d29afe', 'published', NULL, '2026-08-17 12:14:53.645184+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: editing_template_bindings; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.editing_template_bindings VALUES ('template_binding_phase5', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'etv_system_suspense_v1', 1, NULL, '{}', true, 'phase5 fixture current binding', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: voice_profiles; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.voice_profiles VALUES (1, 'voice_phase5_lin', 'p_phase1_legacy', 'char_lin', 'character', 'deterministic_mock', 'mock-tts-v1', 'lin', 'zh-CN', 'unknown', NULL, '克制、清晰', 0.000, 1.000, 1.000, '[]', NULL, '', 1, 'ready', 'approved', 'locked', true, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.voice_profiles VALUES (2, 'voice_phase5_zhou', 'p_phase1_legacy', 'char_zhou', 'character', 'deterministic_mock', 'mock-tts-v1', 'zhou', 'zh-CN', 'unknown', NULL, '低沉、怀疑', 0.000, 1.000, 1.000, '[]', NULL, '', 1, 'ready', 'approved', 'locked', true, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: episode_audio_plans; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.episode_audio_plans VALUES (1, 'audio_plan_phase5', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'script_phase5_post', 1, NULL, '["audio_phase5_1", "audio_phase5_2"]', '[]', '[]', '[]', '{}', 8000, 'completed', 'approved', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: edit_timelines; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.edit_timelines VALUES (1, 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'script_phase5_post', 'storyboard_phase5_post', 'audio_plan_phase5', 1, '1080x1920', '9:16', 24.000, 'h264', 'aac', 48000, 8000, '{"video": 1, "dialogue": 1, "subtitle": 1}', '[{"type": "cut"}]', '{"style": "condensed_minimal"}', '{"bgm_ducking_enabled": true}', '{"model_version": "deterministic-mock-v1", "ir_revision_id": "ir_phase1_001", "prompt_version": "mock-prompt-v1", "source_version_id": "sv_legacy_novel_phase1_legacy", "adaptation_spec_version_id": "adaptation_spec_version_phase1_001"}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, 'etv_system_suspense_v1', 'mock_full_chain', 'approved', true, NULL, NULL, 'legacy') ON CONFLICT DO NOTHING;


--
-- Data for Name: creative_workspace_versions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.creative_workspace_versions VALUES ('workspace_phase5_v1', 'creative-workspace.v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 1, NULL, 'script_phase5_post', 'storyboard_phase5_post', 'pacing_phase5_v1', 'selection_phase5_1', 'timeline_phase5_v1', NULL, '{"model_version": "deterministic-mock-v1", "ir_revision_id": "ir_phase1_001", "prompt_version": "phase5-mock-v1", "source_version_id": "sv_legacy_novel_phase1_legacy", "adaptation_spec_version_id": "adaptation_spec_version_phase1_001"}', '{"char_lin": "pb_phase5_lin_v1", "char_zhou": "pb_phase5_zhou_v1"}', '["continuity_phase5_1", "continuity_phase5_2"]', '["quality_phase5_v1", "qc_phase5_v1"]', '{"active_tab": "script", "scene_order": ["scene_phase5_post"]}', 'approved', true, 'full mock chain assembled', 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: diagnostic_spec_proposals; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: dialogues; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.dialogues VALUES (1, 'dlg_phase5_1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'scene_phase5_post', 1, 'dialogue', 'char_lin', '林夏', '门不是风吹开的。', '克制', '短停顿后压低音量', 1800, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', 'spoken') ON CONFLICT DO NOTHING;
INSERT INTO drama.dialogues VALUES (2, 'dlg_phase5_2', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'scene_phase5_post', 2, 'dialogue', 'char_zhou', '周野', '钥匙在你手里？', '怀疑', '尾音上扬', 1500, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', 'spoken') ON CONFLICT DO NOTHING;


--
-- Data for Name: dialogue_audio; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.dialogue_audio VALUES (1, 'audio_phase5_1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'scene_phase5_post', 'dlg_phase5_1', 'char_lin', 'voice_phase5_lin', 1, 'dialogue', '门不是风吹开的。', '门不是风吹开的。', '克制', '短停顿后压低音量', 1.000, 'deterministic_mock', 'mock-tts-v1', NULL, NULL, '/dialogue-audio/audio_phase5_1.wav', '/waveforms/audio_phase5_1.json', 'wav', 48000, 1, NULL, 1800, NULL, NULL, NULL, 'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc', 'succeeded', 'passed', '{}', 'approved', NULL, NULL, NULL, true, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.dialogue_audio VALUES (2, 'audio_phase5_2', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'scene_phase5_post', 'dlg_phase5_2', 'char_zhou', 'voice_phase5_zhou', 1, 'dialogue', '钥匙在你手里？', '钥匙在你手里？', '怀疑', '尾音上扬', 1.000, 'deterministic_mock', 'mock-tts-v1', NULL, NULL, '/dialogue-audio/audio_phase5_2.wav', '/waveforms/audio_phase5_2.json', 'wav', 48000, 1, NULL, 1700, NULL, NULL, NULL, 'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd', 'succeeded', 'passed', '{}', 'approved', NULL, NULL, NULL, true, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: dialogue_timing_versions; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: dialogue_timing_issues; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: edit_timeline_items; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.edit_timeline_items VALUES (1, 'item_phase5_video_1', 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'video', 1, 1, 'shot', 'shot_phase5_1', '/shot-videos/video_phase5_1.mp4', NULL, 0, 4000, 0, 4000, 4000, 1.0000, 0, 0, '{}', '{}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.edit_timeline_items VALUES (2, 'item_phase5_video_2', 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'video', 1, 2, 'shot', 'shot_phase5_2', '/shot-videos/video_phase5_2.mp4', NULL, 4000, 8000, 0, 4000, 4000, 1.0000, 0, 0, '{}', '{}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.edit_timeline_items VALUES (3, 'item_phase5_dialogue_1', 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'dialogue', 1, 1, 'dialogue', 'dlg_phase5_1', '/dialogue-audio/audio_phase5_1.wav', NULL, 800, 2600, 0, 1800, 1800, 1.0000, 20, 40, '{}', '{}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.edit_timeline_items VALUES (4, 'item_phase5_dialogue_2', 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'dialogue', 1, 2, 'dialogue', 'dlg_phase5_2', '/dialogue-audio/audio_phase5_2.wav', NULL, 4300, 6000, 0, 1700, 1700, 1.0000, 20, 40, '{}', '{}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.edit_timeline_items VALUES (5, 'item_phase5_subtitle_1', 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'subtitle', 1, 1, 'dialogue', 'dlg_phase5_1', NULL, NULL, 800, 2600, 0, NULL, 1800, 1.0000, 0, 0, '{}', '{}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.edit_timeline_items VALUES (6, 'item_phase5_subtitle_2', 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'subtitle', 1, 2, 'dialogue', 'dlg_phase5_2', NULL, NULL, 4300, 6000, 0, NULL, 1700, 1.0000, 0, 0, '{}', '{}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.edit_timeline_items VALUES (7, 'item_phase5_bgm', 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'bgm', 1, 1, 'sound_cue', 'sound_cue_phase5_bgm', '/sound/bgm-suspense.wav', NULL, 0, 8000, 0, 8000, 8000, 0.4000, 300, 500, '{}', '{}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.edit_timeline_items VALUES (8, 'item_phase5_ambience', 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'ambience', 1, 1, 'sound_cue', 'sound_cue_phase5_ambience', '/sound/old-house.wav', NULL, 0, 8000, 0, 8000, 8000, 0.2500, 500, 500, '{}', '{}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.edit_timeline_items VALUES (9, 'item_phase5_door', 'timeline_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'sound_effect', 1, 1, 'sound_cue', 'sound_cue_phase5_door', '/sound/door-creak.wav', NULL, 200, 1400, 0, 1200, 1200, 0.8500, 20, 80, '{}', '{}', 'completed', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL, NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: effective_input_stage_requirements; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'narrative_ir', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'adaptation_spec', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'adaptation_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'episode_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'pacing_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'candidate_selection', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'visual_profiles', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'editing_template', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'timeline', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'narrative_ir', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'adaptation_spec', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'adaptation_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'episode_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'pacing_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'candidate_selection', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'performance_bible', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'continuity_ledger', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'visual_profiles', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'editing_template', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'timeline', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'narrative_ir', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'adaptation_spec', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'adaptation_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'candidate_selection', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'performance_bible', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'continuity_ledger', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'visual_profiles', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'editing_template', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'timeline', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'narrative_ir', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'adaptation_spec', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'adaptation_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'episode_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'performance_bible', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'continuity_ledger', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'episode_plan', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'pacing_plan', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'pacing_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'candidate_selection', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'performance_bible', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'continuity_ledger', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'visual_profiles', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'editing_template', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'timeline', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'narrative_ir', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'adaptation_spec', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'adaptation_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'episode_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'pacing_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'candidate_selection', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'performance_bible', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'continuity_ledger', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'visual_profiles', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'editing_template', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'timeline', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'narrative_ir', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'adaptation_spec', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'adaptation_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'episode_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'pacing_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'candidate_selection', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'performance_bible', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'continuity_ledger', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'visual_profiles', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'editing_template', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'timeline', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'narrative_ir', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'adaptation_spec', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'adaptation_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'episode_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'pacing_plan', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'candidate_selection', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'performance_bible', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'continuity_ledger', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'visual_profiles', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'editing_template', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'timeline', 'optional') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('episode_script', 'production_snapshot', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_design', 'production_snapshot', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('visual_assets', 'production_snapshot', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('storyboard_images', 'production_snapshot', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('image_to_video', 'production_snapshot', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('voice_audio', 'production_snapshot', 'required') ON CONFLICT DO NOTHING;
INSERT INTO drama.effective_input_stage_requirements VALUES ('post_production', 'production_snapshot', 'required') ON CONFLICT DO NOTHING;


--
-- Data for Name: entity_version_bindings; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: episode_event_assignments; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.episode_event_assignments VALUES (1, 'assignment_phase1_001', 'adaptation_episode_plan_phase1_001', 'event_revision_phase1_001', 1, 'preserve', NULL, '["adaptation_rule_phase1_001"]', 'fixture:assignment:001', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.episode_event_assignments VALUES (2, 'assignment_phase1_002', 'adaptation_episode_plan_phase1_001', 'event_revision_phase1_002', 2, 'preserve', NULL, '[]', 'fixture:assignment:002', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: render_jobs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: episode_masters; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.episode_masters VALUES (1, 'master_phase5_v1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'timeline_phase5_v1', NULL, 1, 'final', '/results/master_phase5_v1.mp4', NULL, NULL, NULL, false, 1080, 1920, '9:16', 24.000, 8000, NULL, 'h264', 'aac', 48000, -16.000, -1.000, '2222222222222222222222222222222222222222222222222222222222222222', 'ready', true, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: story_arc_runs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: episode_production_runs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: event_participants; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.event_participants VALUES (1, 'participant_phase1_001', 'event_revision_phase1_001', 'entity_revision_phase1_hero', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'actor', '{}', 'span_legacy_full_ch_phase1_legacy_001', 0.9900, 'fixture:participant:001', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.event_participants VALUES (2, 'participant_phase1_002', 'event_revision_phase1_002', 'entity_revision_phase1_hero', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'observer', '{}', 'span_legacy_full_ch_phase1_legacy_002', 0.9800, 'fixture:participant:002', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: event_relations; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.event_relations VALUES (1, 'event_relation_phase1_001', 'event_revision_phase1_001', 'event_revision_phase1_002', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'before', 'span_legacy_full_ch_phase1_legacy_002', 0.9500, 'fixture:event-relation:001', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: fact_evidence; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.fact_evidence VALUES (1, 'evidence_fact_revision_phase1_event_001', 'fact_revision_phase1_event_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'span_legacy_full_ch_phase1_legacy_001', 'primary', 0.9800, 'fixture:evidence:fact_revision_phase1_event_001', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.fact_evidence VALUES (2, 'evidence_fact_revision_phase1_event_002', 'fact_revision_phase1_event_002', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'span_legacy_full_ch_phase1_legacy_002', 'primary', 0.9700, 'fixture:evidence:fact_revision_phase1_event_002', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.fact_evidence VALUES (3, 'evidence_fact_revision_phase1_state_001', 'fact_revision_phase1_state_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'span_legacy_full_ch_phase1_legacy_001', 'primary', 0.9500, 'fixture:evidence:fact_revision_phase1_state_001', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.fact_evidence VALUES (4, 'evidence_fact_revision_phase1_timeline_001', 'fact_revision_phase1_timeline_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'span_legacy_full_ch_phase1_legacy_002', 'primary', 0.9400, 'fixture:evidence:fact_revision_phase1_timeline_001', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.fact_evidence VALUES (5, 'evidence_fact_revision_phase1_foreshadow_001', 'fact_revision_phase1_foreshadow_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'span_legacy_full_ch_phase1_legacy_002', 'primary', 0.9300, 'fixture:evidence:fact_revision_phase1_foreshadow_001', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: qc_jobs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: qc_reports; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.qc_reports VALUES (1, 'qc_phase5_v1', NULL, 'p_phase1_legacy', 'ep_phase1_legacy_001', 'master_phase5_v1', '{"av_sync": "passed", "loudness_lufs": -16}', '{"safe_area": "passed"}', '{"continuity": "warning"}', '{"license": "passed"}', 92.00, 'warning', '[]', '["shot_phase5_2 gaze"]', '["local redo"]', '["workbench:shot_phase5_2@4250"]', 'completed', 1, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: final_reviews; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: foreshadow_threads; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.foreshadow_threads VALUES (1, 'foreshadow_thread_phase1_001', 'sw_legacy_novel_phase1_legacy', 'thread:key', '钥匙线索', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: foreshadow_occurrences; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.foreshadow_occurrences VALUES (1, 'foreshadow_occurrence_phase1_001', 'foreshadow_thread_phase1_001', 'fact_revision_phase1_foreshadow_001', 'event_revision_phase1_002', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'planted', 2.0000, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: generated_assets; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: generation_usage; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: image_generation_tasks; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: source_change_sets; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: regeneration_requests; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: regeneration_request_items; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: incremental_rebuild_tasks; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: invalidation_tasks; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.invalidation_tasks VALUES (1, 'invalidation_task_phase1_001', 'operation_phase1_invalidation_001', 'invalidation_scan', 'p_phase1_legacy', NULL, 'artifact_phase1_fact_001', 'completed', 'manual', 'fixture:invalidation:001', '{}', NULL, NULL, 0, 3, NULL, NULL, NULL, '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: invalidation_impacts; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.invalidation_impacts VALUES (1, 'invalidation_impact_phase1_001', 'invalidation_task_phase1_001', 'artifact_phase1_episode_plan_001', 'valid', 'stale', 1, '{"reason": "fixture"}', '["artifact_dependency_phase1_001"]', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: ir_merge_proposals; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: ir_merge_proposal_items; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: novels; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.novels VALUES (1, 'novel_phase1_legacy', 'p_phase1_legacy', '旧数据升级样例', 'text', NULL, '/data/storage/novels/novel_phase1_legacy.txt', 'UTF-8', 22, 2, 'legacy-hash-v0', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:47.991101+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: legacy_source_bindings; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.legacy_source_bindings VALUES (1, 'lsb_novel_phase1_legacy', 'novel_phase1_legacy', 'p_phase1_legacy', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'phase1-legacy-v1', '2026-08-17 12:14:47.991101+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: location_visual_profiles; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.location_visual_profiles VALUES (1, 'profile_phase18_door', 'p_phase1_legacy', 'location_door', 1, 'Old house entrance', 'interior', '', '', '', '[]', '[]', '', '[]', '[]', '[]', '[]', 'old wooden door under cold moonlight', 'modern furniture', 1, 'ready', 'approved', 'locked', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: media_processing_jobs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: migration_audit; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.migration_audit VALUES (1, 'ma_06_novel_novel_phase1_legacy', '06', 'phase1-legacy-v1', 'backfill', 'legacy_novel', 'novel_phase1_legacy', '{"work_id": "sw_legacy_novel_phase1_legacy", "source_version_id": "sv_legacy_novel_phase1_legacy"}', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: narrative_entity_aliases; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.narrative_entity_aliases VALUES (1, 'alias_phase1_hero', 'entity_revision_phase1_hero', '小夏', 'name', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: narrative_entity_mentions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.narrative_entity_mentions VALUES (1, 'mention_phase1_hero', 'entity_revision_phase1_hero', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 'span_legacy_full_ch_phase1_legacy_001', '林夏', 0.9900, 'fixture:mention:hero', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: novel_chapters; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.novel_chapters VALUES (1, 'ch_phase1_legacy_001', 'novel_phase1_legacy', 'p_phase1_legacy', 1, '第一章', '林夏推开门。\n门后没有人。', 14, 'BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.novel_chapters VALUES (2, 'ch_phase1_legacy_002', 'novel_phase1_legacy', 'p_phase1_legacy', 2, '第二章', '手机亮起：线索🔑出现。', 12, 'md5:cccccccccccccccccccccccccccccccc', '2026-08-17 12:14:47.991101+08', '2026-08-17 12:14:48.310757+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: novel_chunks; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: pacing_episodes; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.pacing_episodes VALUES (1, 'pacing_episode_phase5_1', 'pacing_phase5_v1', 'adaptation_episode_plan_phase1_001', 1, '门后的线索', 0.7800, 0.7200, 0.6500, 0.9100, 8, '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: pacing_beats; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.pacing_beats VALUES (1, 'beat_phase5_hook', 'pacing_phase5_v1', 'pacing_episode_phase5_1', 'episode-1:opening', 'artifact_phase5_beat', 1, 1, '门自动打开', '林夏推门并察觉异常', 'opening_hook', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'event_revision_phase1_001', NULL, 0.7500, 0.7200, 0.4000, 0.9200, 0.3000, 0.3500, 0.6500, 0.0000, 4, false, '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.pacing_beats VALUES (2, 'beat_phase5_turn', 'pacing_phase5_v1', 'pacing_episode_phase5_1', 'episode-1:turn', 'artifact_phase5_beat', 1, 2, '钥匙反转', '周野指出钥匙并听见脚步', 'ending_hook', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', 'event_revision_phase1_002', NULL, 0.8600, 0.7800, 0.7200, 0.9500, 0.8200, 0.5500, 0.4500, 0.0000, 4, false, '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: pacing_issues; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: pacing_story_arcs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: quality_gate_runs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: quality_gate_master_approvals; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: professional_export_jobs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: prompt_test_suites; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: prompt_experiments; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: prompt_fixtures; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: prompt_blind_evaluations; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: prompt_experiment_variants; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: prompt_experiment_results; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: prompt_production_bindings; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: prop_visual_profiles; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: publication_metadata; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: publication_tasks; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: quality_gate_benchmark_runs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: quality_gate_findings; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: quality_gate_change_plans; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: quality_gate_overrides; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: quality_issue_edit_links; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.quality_issue_edit_links VALUES ('qel_phase5_visual', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'visual_qc', 'vqi_phase5', 'shot', 'shot_phase5_2', 4000, 4500, '/projects/p_phase1_legacy/episodes/ep_phase1_legacy_001/workbench?tab=storyboard&shot_id=shot_phase5_2', NULL, '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.quality_issue_edit_links VALUES ('qel_phase5_quality', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'quality', 'quality_issue_phase5_1', 'shot', 'shot_phase5_2', 4300, 5500, '/projects/p_phase1_legacy/episodes/ep_phase1_legacy_001/workbench?tab=sound&shot_id=shot_phase5_2', NULL, '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: quality_issues; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.quality_issues VALUES (1, 'quality_issue_phase5_1', 'quality_phase5_v1', '声画可执行性', 'warning', 1, 'beat_phase5_turn', 'artifact_phase5_beat', 'span_legacy_full_ch_phase1_legacy_001', 'fact_revision_phase1_event_001', '{"editor_tab": "sound", "timecode_ms": 4300}', '脚步声需与结尾反应镜头对齐', '结尾威胁音效尚需精确卡点', '将脚步 cue 对齐到 4300ms', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: quality_score_dimensions; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: rebuild_provider_executions; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: rebuild_publications; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: rebuild_task_events; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: regeneration_proposals; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: regeneration_proposal_items; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: review_tasks; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: shot_lineage; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: visual_styles; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.visual_styles VALUES (1, 'style_phase5_v1', 'p_phase1_legacy', 'Restrained suspense', 'project', 'Cold realistic suspense with stable character identity', 'cinematic realistic cold moonlight restrained performance', 'identity drift, distorted anatomy, modern furniture', '[]', '[]', '[]', '9:16', 1080, 1920, '{}', 1, 'approved', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: storyboard_images; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.storyboard_images VALUES (1, 'image_phase5_1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'storyboard_phase5_post', 'shot_phase5_1', 1, 1, NULL, '[]', '[]', NULL, '[]', '[]', '林夏推门写实分镜', '畸形手', 'deterministic_mock', 'mock-image-v1', 101, NULL, NULL, '/storyboard-images/image_phase5_1.png', 'succeeded', 'passed', '{}', 'approved', NULL, NULL, NULL, true, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.storyboard_images VALUES (2, 'image_phase5_2', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'storyboard_phase5_post', 'shot_phase5_2', 1, 1, NULL, '[]', '[]', NULL, '[]', '[]', '周野反应写实分镜', '脸部漂移', 'deterministic_mock', 'mock-image-v1', 102, NULL, NULL, '/storyboard-images/image_phase5_2.png', 'succeeded', 'passed', '{}', 'approved', NULL, NULL, NULL, true, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: shot_videos; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.shot_videos VALUES (1, 'video_phase5_1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'storyboard_phase5_post', 'shot_phase5_1', 'image_phase5_1', 1, 1, 'deterministic_mock', 'mock-video-v1', NULL, '林夏推门，动作连续', '', '/storyboard-images/image_phase5_1.png', '[]', '{}', NULL, 4.000, 4.000, '9:16', 1080, 1920, 24.000, 'h264', false, NULL, '/shot-videos/video_phase5_1.mp4', NULL, 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'succeeded', 'passed', '{}', 'approved', NULL, NULL, NULL, true, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL) ON CONFLICT DO NOTHING;
INSERT INTO drama.shot_videos VALUES (2, 'video_phase5_2', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'storyboard_phase5_post', 'shot_phase5_2', 'image_phase5_2', 1, 1, 'deterministic_mock', 'mock-video-v1', NULL, '周野反应特写', '', '/storyboard-images/image_phase5_2.png', '[]', '{}', NULL, 4.000, 4.000, '9:16', 1080, 1920, 24.000, 'h264', false, NULL, '/shot-videos/video_phase5_2.mp4', NULL, 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 'succeeded', 'passed', '{}', 'approved', NULL, NULL, NULL, true, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', NULL, NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: sound_assets; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.sound_assets VALUES ('sound_phase5_bgm', 'p_phase1_legacy', 'bgm', '低频悬疑脉冲', 'suspense_minimal', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_assets VALUES ('sound_phase5_ambience', 'p_phase1_legacy', 'ambience', '旧宅夜间底噪', 'suspense_minimal', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_assets VALUES ('sound_phase5_door', 'p_phase1_legacy', 'door', '老木门吱呀', 'suspense_minimal', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_assets VALUES ('sound_phase5_bgm_noir', 'p_phase1_legacy', 'bgm', '黑色电影低音弦乐', 'cinematic_noir', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_assets VALUES ('sound_phase5_ambience_noir', 'p_phase1_legacy', 'ambience', '黑色电影雨夜底噪', 'cinematic_noir', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_assets VALUES ('sound_phase5_door_noir', 'p_phase1_legacy', 'door', '黑色电影厚重门响', 'cinematic_noir', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: sound_asset_versions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.sound_asset_versions VALUES ('sound_version_phase5_bgm', 'sound_phase5_bgm', 'artifact_phase5_sound_bgm', 1, NULL, 'deterministic_mock', 'mock://bgm/suspense', '/sound/bgm-suspense.wav', 'deterministic_mock', 'mock-audio-v1', '["悬疑", "警觉"]', 96.000, 'D minor', 8000, '{"status": "cleared", "license_id": "mock-license-bgm-001", "usage_scope": "all-project-media"}', '{"prompt_version": "sound-prompt-v1"}', '1111111111111111111111111111111111111111111111111111111111111111', 'approved', true, 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_asset_versions VALUES ('sound_version_phase5_ambience', 'sound_phase5_ambience', 'artifact_phase5_sound_ambience', 1, NULL, 'deterministic_mock', 'mock://ambience/old-house', '/sound/old-house.wav', 'deterministic_mock', 'mock-audio-v1', '["夜", "压迫"]', NULL, NULL, 8000, '{"status": "cleared", "license_id": "mock-license-amb-001", "usage_scope": "all-project-media"}', '{"prompt_version": "sound-prompt-v1"}', '2222222222222222222222222222222222222222222222222222222222222222', 'approved', true, 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_asset_versions VALUES ('sound_version_phase5_door', 'sound_phase5_door', 'artifact_phase5_sound_door', 1, NULL, 'deterministic_mock', 'mock://sfx/door', '/sound/door-creak.wav', 'deterministic_mock', 'mock-audio-v1', '["异常", "陈旧"]', NULL, NULL, 1200, '{"status": "cleared", "license_id": "mock-license-sfx-001", "usage_scope": "all-project-media"}', '{"event_key": "shot_phase5_1:door-open"}', '3333333333333333333333333333333333333333333333333333333333333333', 'approved', true, 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_asset_versions VALUES ('sound_version_phase5_bgm_noir', 'sound_phase5_bgm_noir', 'artifact_phase5_sound_bgm_noir', 1, NULL, 'deterministic_mock', 'mock://bgm/noir', '/sound/bgm-noir.wav', 'deterministic_mock', 'mock-audio-v1', '["悬疑", "黑色电影"]', 92.000, 'C minor', 8000, '{"status": "cleared", "license_id": "mock-license-bgm-002", "usage_scope": "all-project-media"}', '{"prompt_version": "sound-prompt-v2"}', '4444444444444444444444444444444444444444444444444444444444444444', 'approved', true, 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_asset_versions VALUES ('sound_version_phase5_ambience_noir', 'sound_phase5_ambience_noir', 'artifact_phase5_sound_ambience_noir', 1, NULL, 'deterministic_mock', 'mock://ambience/noir-rain', '/sound/noir-rain.wav', 'deterministic_mock', 'mock-audio-v1', '["雨夜", "压迫"]', NULL, NULL, 8000, '{"status": "cleared", "license_id": "mock-license-amb-002", "usage_scope": "all-project-media"}', '{"prompt_version": "sound-prompt-v2"}', '5555555555555555555555555555555555555555555555555555555555555555', 'approved', true, 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_asset_versions VALUES ('sound_version_phase5_door_noir', 'sound_phase5_door_noir', 'artifact_phase5_sound_door_noir', 1, NULL, 'deterministic_mock', 'mock://sfx/noir-door', '/sound/noir-door.wav', 'deterministic_mock', 'mock-audio-v1', '["厚重", "威胁"]', NULL, NULL, 1200, '{"status": "cleared", "license_id": "mock-license-sfx-002", "usage_scope": "all-project-media"}', '{"event_key": "shot_phase5_1:door-open"}', '6666666666666666666666666666666666666666666666666666666666666666', 'approved', true, 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: sound_cue_versions; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.sound_cue_versions VALUES ('sound_cue_version_phase5_bgm', 'sound_cue_phase5_bgm', 'p_phase1_legacy', 'ep_phase1_legacy_001', NULL, NULL, 'sound_version_phase5_bgm', 'bgm', '低频悬疑脉冲', 'episode:emotion-curve', 1, 0, 8000, 0, 8000, -8.000, 300, 500, '{"bpm": 96, "align_to": ["beat_phase5_hook", "beat_phase5_turn"]}', '{"fade": "crossfade", "allow_key_change": true}', '{"ratio": 8, "enabled": true, "attack_ms": 20, "release_ms": 250, "threshold_db": -28}', 1, NULL, 'approved', true, 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_cue_versions VALUES ('sound_cue_version_phase5_ambience', 'sound_cue_phase5_ambience', 'p_phase1_legacy', 'ep_phase1_legacy_001', NULL, NULL, 'sound_version_phase5_ambience', 'ambience', '旧宅夜间底噪', 'episode:environment', 1, 0, 8000, 0, 8000, -14.000, 500, 500, '{}', '{"fade": "linear"}', '{}', 1, NULL, 'approved', true, 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.sound_cue_versions VALUES ('sound_cue_version_phase5_door', 'sound_cue_phase5_door', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'shot_phase5_1', NULL, 'sound_version_phase5_door', 'door', '老木门吱呀', 'shot_phase5_1:door-open', 1, 200, 1400, 0, 1200, -2.000, 20, 80, '{"align_to_ms": 200}', '{"cut_on_event": true}', '{}', 1, NULL, 'approved', true, 'phase5-fixture', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: sound_style_replacements; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: source_change_items; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: source_import_jobs; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: source_import_items; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: story_arc_events; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.story_arc_events VALUES (1, 'story_arc_event_phase1_001', 'story_arc_revision_phase1_001', 'event_revision_phase1_001', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 1, 'setup', 'fixture:arc-event:001', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;
INSERT INTO drama.story_arc_events VALUES (2, 'story_arc_event_phase1_002', 'story_arc_revision_phase1_001', 'event_revision_phase1_002', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', 2, 'progression', 'fixture:arc-event:002', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: subtitle_cues; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.subtitle_cues VALUES (1, 'subtitle_phase5_1', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'scene_phase5_post', 'shot_phase5_1', 'dlg_phase5_1', 'audio_phase5_1', 1, '林夏', '门不是风吹开的。', 800, 2600, 1800, '{"style": "condensed_minimal"}', 'approved', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', 1, NULL, true, 'approved') ON CONFLICT DO NOTHING;
INSERT INTO drama.subtitle_cues VALUES (2, 'subtitle_phase5_2', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'scene_phase5_post', 'shot_phase5_2', 'dlg_phase5_2', 'audio_phase5_2', 1, '周野', '钥匙在你手里？', 4300, 6000, 1700, '{"style": "condensed_minimal"}', 'approved', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', 1, NULL, true, 'approved') ON CONFLICT DO NOTHING;


--
-- Data for Name: timeline_facts; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.timeline_facts VALUES (1, 'timeline_phase1_001', 'fact_revision_phase1_timeline_001', NULL, 'event_revision_phase1_002', 'ir_phase1_001', 'sw_legacy_novel_phase1_legacy', 'sv_legacy_novel_phase1_legacy', '开门之后', '{"offset": "after", "relative_to": "event_revision_phase1_001"}', 2.0000, 'relative', '2026-08-17 12:14:58.97446+08', '2026-08-17 12:14:58.97446+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: tts_generation_tasks; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: video_generation_tasks; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: visual_qc_runs; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.visual_qc_runs VALUES (1, 'vqc_phase5', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'visual-qc-report.v1', 'phase5-full-chain', 'deterministic_mock', 'completed', 1, '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08', '2026-08-17 12:14:59.84683+08') ON CONFLICT DO NOTHING;


--
-- Data for Name: visual_qc_issues; Type: TABLE DATA; Schema: drama; Owner: -
--

INSERT INTO drama.visual_qc_issues VALUES (1, 'vqi_phase5', 'vqc_phase5', 'p_phase1_legacy', 'ep_phase1_legacy_001', 'scene_phase5_post', 'shot_phase5_2', 'gaze_error', 'major', 4250, 102, '{"actual": "front", "expected": "left"}', '只重建第二镜前 500ms 的视线动作', 'open', '2026-08-17 12:14:59.84683+08', NULL) ON CONFLICT DO NOTHING;


--
-- Data for Name: visual_qc_local_redo_plans; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: workflow_notifications; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Data for Name: workflow_tasks; Type: TABLE DATA; Schema: drama; Owner: -
--



--
-- Name: adaptation_diagnostic_nodes_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_diagnostic_nodes_id_seq', 1, true);


--
-- Name: adaptation_diagnostic_reports_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_diagnostic_reports_id_seq', 1, true);


--
-- Name: adaptation_episode_plans_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_episode_plans_id_seq', 1, true);


--
-- Name: adaptation_plan_validation_runs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_plan_validation_runs_id_seq', 1, false);


--
-- Name: adaptation_plans_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_plans_id_seq', 1, true);


--
-- Name: adaptation_rules_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_rules_id_seq', 2, true);


--
-- Name: adaptation_scope_arcs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_scope_arcs_id_seq', 1, true);


--
-- Name: adaptation_scope_chapters_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_scope_chapters_id_seq', 2, true);


--
-- Name: adaptation_spec_versions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_spec_versions_id_seq', 2, true);


--
-- Name: adaptation_specs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.adaptation_specs_id_seq', 2, true);


--
-- Name: artifact_current_bindings_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.artifact_current_bindings_id_seq', 1, true);


--
-- Name: artifact_dependencies_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.artifact_dependencies_id_seq', 15, true);


--
-- Name: artifact_generation_provenance_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.artifact_generation_provenance_id_seq', 1, false);


--
-- Name: artifact_performance_bible_refs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.artifact_performance_bible_refs_id_seq', 1, false);


--
-- Name: artifact_source_evidence_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.artifact_source_evidence_id_seq', 2, true);


--
-- Name: artifacts_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.artifacts_id_seq', 29, true);


--
-- Name: asset_dependencies_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.asset_dependencies_id_seq', 1, false);


--
-- Name: candidate_composition_parts_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidate_composition_parts_id_seq', 1, false);


--
-- Name: candidate_decisions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidate_decisions_id_seq', 1, false);


--
-- Name: candidate_execution_records_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidate_execution_records_id_seq', 1, false);


--
-- Name: candidate_hard_rule_results_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidate_hard_rule_results_id_seq', 5, true);


--
-- Name: candidate_scores_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidate_scores_id_seq', 2, true);


--
-- Name: candidate_selection_bindings_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidate_selection_bindings_id_seq', 1, true);


--
-- Name: candidate_selections_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidate_selections_id_seq', 1, true);


--
-- Name: candidate_sets_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidate_sets_id_seq', 1, true);


--
-- Name: candidate_timecode_comments_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidate_timecode_comments_id_seq', 1, false);


--
-- Name: candidates_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.candidates_id_seq', 2, true);


--
-- Name: change_comments_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.change_comments_id_seq', 1, false);


--
-- Name: change_plan_events_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.change_plan_events_id_seq', 1, false);


--
-- Name: change_plan_impacts_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.change_plan_impacts_id_seq', 1, false);


--
-- Name: change_plans_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.change_plans_id_seq', 1, false);


--
-- Name: chapter_revisions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.chapter_revisions_id_seq', 2, true);


--
-- Name: character_costumes_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.character_costumes_id_seq', 1, false);


--
-- Name: character_performance_bibles_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.character_performance_bibles_id_seq', 2, true);


--
-- Name: character_performance_stage_states_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.character_performance_stage_states_id_seq', 1, false);


--
-- Name: character_state_changes_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.character_state_changes_id_seq', 1, true);


--
-- Name: character_visual_profiles_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.character_visual_profiles_id_seq', 2, true);


--
-- Name: compiler_checkpoints_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.compiler_checkpoints_id_seq', 1, true);


--
-- Name: compiler_diagnostics_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.compiler_diagnostics_id_seq', 1, true);


--
-- Name: compiler_runs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.compiler_runs_id_seq', 2, true);


--
-- Name: continuity_ledger_entries_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.continuity_ledger_entries_id_seq', 2, true);


--
-- Name: diagnostic_spec_proposals_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.diagnostic_spec_proposals_id_seq', 1, false);


--
-- Name: dialogue_audio_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.dialogue_audio_id_seq', 2, true);


--
-- Name: dialogues_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.dialogues_id_seq', 2, true);


--
-- Name: edit_timeline_items_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.edit_timeline_items_id_seq', 9, true);


--
-- Name: edit_timelines_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.edit_timelines_id_seq', 1, true);


--
-- Name: entity_version_bindings_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.entity_version_bindings_id_seq', 1, false);


--
-- Name: entity_versions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.entity_versions_id_seq', 1, false);


--
-- Name: episode_audio_plans_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.episode_audio_plans_id_seq', 1, true);


--
-- Name: episode_event_assignments_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.episode_event_assignments_id_seq', 2, true);


--
-- Name: episode_masters_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.episode_masters_id_seq', 1, true);


--
-- Name: episode_outlines_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.episode_outlines_id_seq', 1, true);


--
-- Name: episode_production_runs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.episode_production_runs_id_seq', 1, false);


--
-- Name: episode_scripts_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.episode_scripts_id_seq', 1, true);


--
-- Name: event_participants_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.event_participants_id_seq', 2, true);


--
-- Name: event_relations_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.event_relations_id_seq', 1, true);


--
-- Name: fact_evidence_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.fact_evidence_id_seq', 5, true);


--
-- Name: final_reviews_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.final_reviews_id_seq', 1, false);


--
-- Name: foreshadow_occurrences_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.foreshadow_occurrences_id_seq', 1, true);


--
-- Name: foreshadow_threads_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.foreshadow_threads_id_seq', 1, true);


--
-- Name: generated_assets_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.generated_assets_id_seq', 1, false);


--
-- Name: generation_context_reads_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.generation_context_reads_id_seq', 1, false);


--
-- Name: generation_usage_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.generation_usage_id_seq', 1, false);


--
-- Name: image_generation_tasks_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.image_generation_tasks_id_seq', 1, false);


--
-- Name: incremental_rebuild_tasks_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.incremental_rebuild_tasks_id_seq', 1, false);


--
-- Name: invalidation_impacts_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.invalidation_impacts_id_seq', 1, true);


--
-- Name: invalidation_tasks_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.invalidation_tasks_id_seq', 1, true);


--
-- Name: ir_merge_proposal_items_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.ir_merge_proposal_items_id_seq', 1, false);


--
-- Name: ir_merge_proposals_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.ir_merge_proposals_id_seq', 1, false);


--
-- Name: legacy_source_bindings_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.legacy_source_bindings_id_seq', 1, true);


--
-- Name: location_visual_profiles_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.location_visual_profiles_id_seq', 1, true);


--
-- Name: media_processing_jobs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.media_processing_jobs_id_seq', 1, false);


--
-- Name: migration_audit_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.migration_audit_id_seq', 1, true);


--
-- Name: narrative_entities_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.narrative_entities_id_seq', 1, true);


--
-- Name: narrative_entity_aliases_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.narrative_entity_aliases_id_seq', 1, true);


--
-- Name: narrative_entity_mentions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.narrative_entity_mentions_id_seq', 1, true);


--
-- Name: narrative_entity_revisions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.narrative_entity_revisions_id_seq', 1, true);


--
-- Name: narrative_event_revisions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.narrative_event_revisions_id_seq', 2, true);


--
-- Name: narrative_fact_revisions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.narrative_fact_revisions_id_seq', 5, true);


--
-- Name: narrative_facts_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.narrative_facts_id_seq', 5, true);


--
-- Name: narrative_ir_revisions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.narrative_ir_revisions_id_seq', 1, true);


--
-- Name: novel_chapters_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.novel_chapters_id_seq', 2, true);


--
-- Name: novel_chunks_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.novel_chunks_id_seq', 1, false);


--
-- Name: novels_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.novels_id_seq', 1, true);


--
-- Name: operations_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.operations_id_seq', 9, true);


--
-- Name: pacing_beats_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.pacing_beats_id_seq', 2, true);


--
-- Name: pacing_episodes_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.pacing_episodes_id_seq', 1, true);


--
-- Name: pacing_issues_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.pacing_issues_id_seq', 1, false);


--
-- Name: pacing_plan_versions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.pacing_plan_versions_id_seq', 1, true);


--
-- Name: pacing_story_arcs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.pacing_story_arcs_id_seq', 1, false);


--
-- Name: professional_export_jobs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.professional_export_jobs_id_seq', 1, false);


--
-- Name: project_source_bindings_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.project_source_bindings_id_seq', 1, true);


--
-- Name: projects_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.projects_id_seq', 1, true);


--
-- Name: prompt_blind_evaluations_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prompt_blind_evaluations_id_seq', 1, false);


--
-- Name: prompt_experiment_results_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prompt_experiment_results_id_seq', 1, false);


--
-- Name: prompt_experiment_variants_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prompt_experiment_variants_id_seq', 1, false);


--
-- Name: prompt_experiments_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prompt_experiments_id_seq', 1, false);


--
-- Name: prompt_fixtures_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prompt_fixtures_id_seq', 1, false);


--
-- Name: prompt_production_bindings_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prompt_production_bindings_id_seq', 1, false);


--
-- Name: prompt_templates_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prompt_templates_id_seq', 1, false);


--
-- Name: prompt_test_suites_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prompt_test_suites_id_seq', 1, false);


--
-- Name: prompt_versions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prompt_versions_id_seq', 1, false);


--
-- Name: prop_visual_profiles_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.prop_visual_profiles_id_seq', 1, false);


--
-- Name: publication_metadata_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.publication_metadata_id_seq', 1, false);


--
-- Name: publication_tasks_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.publication_tasks_id_seq', 1, false);


--
-- Name: qc_jobs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.qc_jobs_id_seq', 1, false);


--
-- Name: qc_reports_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.qc_reports_id_seq', 1, true);


--
-- Name: quality_gate_benchmark_runs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.quality_gate_benchmark_runs_id_seq', 1, false);


--
-- Name: quality_gate_change_plans_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.quality_gate_change_plans_id_seq', 1, false);


--
-- Name: quality_gate_findings_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.quality_gate_findings_id_seq', 1, false);


--
-- Name: quality_gate_master_approvals_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.quality_gate_master_approvals_id_seq', 1, false);


--
-- Name: quality_gate_overrides_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.quality_gate_overrides_id_seq', 1, false);


--
-- Name: quality_gate_runs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.quality_gate_runs_id_seq', 1, false);


--
-- Name: quality_issues_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.quality_issues_id_seq', 1, true);


--
-- Name: quality_score_dimensions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.quality_score_dimensions_id_seq', 1, false);


--
-- Name: quality_score_reports_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.quality_score_reports_id_seq', 1, true);


--
-- Name: regeneration_proposal_items_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.regeneration_proposal_items_id_seq', 1, false);


--
-- Name: regeneration_proposals_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.regeneration_proposals_id_seq', 1, false);


--
-- Name: regeneration_request_items_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.regeneration_request_items_id_seq', 1, false);


--
-- Name: regeneration_requests_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.regeneration_requests_id_seq', 1, false);


--
-- Name: render_jobs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.render_jobs_id_seq', 1, false);


--
-- Name: review_tasks_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.review_tasks_id_seq', 1, false);


--
-- Name: script_scenes_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.script_scenes_id_seq', 1, true);


--
-- Name: seasons_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.seasons_id_seq', 1, true);


--
-- Name: shot_edit_plans_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.shot_edit_plans_id_seq', 1, false);


--
-- Name: shot_handoffs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.shot_handoffs_id_seq', 1, true);


--
-- Name: shot_lineage_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.shot_lineage_id_seq', 1, false);


--
-- Name: shot_sequence_versions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.shot_sequence_versions_id_seq', 1, false);


--
-- Name: shot_videos_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.shot_videos_id_seq', 2, true);


--
-- Name: source_change_items_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.source_change_items_id_seq', 1, false);


--
-- Name: source_change_sets_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.source_change_sets_id_seq', 1, false);


--
-- Name: source_chapters_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.source_chapters_id_seq', 2, true);


--
-- Name: source_import_items_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.source_import_items_id_seq', 1, false);


--
-- Name: source_import_jobs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.source_import_jobs_id_seq', 1, false);


--
-- Name: source_spans_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.source_spans_id_seq', 2, true);


--
-- Name: source_version_chapters_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.source_version_chapters_id_seq', 2, true);


--
-- Name: source_versions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.source_versions_id_seq', 1, true);


--
-- Name: source_works_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.source_works_id_seq', 1, true);


--
-- Name: story_arc_events_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.story_arc_events_id_seq', 2, true);


--
-- Name: story_arc_revisions_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.story_arc_revisions_id_seq', 1, true);


--
-- Name: story_arc_runs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.story_arc_runs_id_seq', 1, false);


--
-- Name: story_arcs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.story_arcs_id_seq', 1, true);


--
-- Name: story_bibles_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.story_bibles_id_seq', 1, true);


--
-- Name: storyboard_images_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.storyboard_images_id_seq', 2, true);


--
-- Name: storyboard_shots_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.storyboard_shots_id_seq', 2, true);


--
-- Name: storyboards_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.storyboards_id_seq', 1, true);


--
-- Name: subtitle_cues_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.subtitle_cues_id_seq', 2, true);


--
-- Name: timeline_facts_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.timeline_facts_id_seq', 1, true);


--
-- Name: tts_generation_tasks_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.tts_generation_tasks_id_seq', 1, false);


--
-- Name: video_generation_tasks_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.video_generation_tasks_id_seq', 1, false);


--
-- Name: visual_qc_issues_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.visual_qc_issues_id_seq', 1, true);


--
-- Name: visual_qc_local_redo_plans_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.visual_qc_local_redo_plans_id_seq', 1, false);


--
-- Name: visual_qc_runs_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.visual_qc_runs_id_seq', 1, true);


--
-- Name: visual_styles_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.visual_styles_id_seq', 1, true);


--
-- Name: voice_profiles_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.voice_profiles_id_seq', 2, true);


--
-- Name: workflow_notifications_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.workflow_notifications_id_seq', 1, false);


--
-- Name: workflow_tasks_id_seq; Type: SEQUENCE SET; Schema: drama; Owner: -
--

SELECT pg_catalog.setval('drama.workflow_tasks_id_seq', 1, false);


--
-- PostgreSQL database dump complete
--

