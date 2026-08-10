\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path TO drama,public;

-- The fixture deliberately disables referential triggers only while creating a
-- minimal isolated graph. CHECK/UNIQUE constraints remain active. The actual
-- final-review gate is exercised after origin mode is restored.
SET LOCAL session_replication_role = replica;
INSERT INTO drama.projects(project_id,novel_name,target_episode_count,episode_duration_seconds,visual_style,aspect_ratio,target_platform)
VALUES('qg_accept_project','quality gate fixture',1,10,'fixture','9:16','test');
INSERT INTO drama.episode_outlines(episode_id,season_id,project_id,episode_number,title,estimated_duration_seconds,version)
VALUES('qg_accept_episode','fixture_season','qg_accept_project',1,'fixture episode',10,1);
INSERT INTO drama.edit_timelines(timeline_id,project_id,episode_id,script_id,storyboard_id,audio_plan_id,version,
  resolution,aspect_ratio,fps,video_codec,audio_codec,sample_rate,target_duration_ms,status,approval_state,is_current)
VALUES('qg_accept_timeline','qg_accept_project','qg_accept_episode','fixture_script','fixture_storyboard','fixture_audio',1,
  '1080x1920','9:16',25,'h264','aac',48000,10000,'completed','approved',true);
INSERT INTO drama.episode_masters(master_id,project_id,episode_id,timeline_id,generation_version,master_type,local_path,status,duration_ms)
VALUES('qg_accept_master','qg_accept_project','qg_accept_episode','qg_accept_timeline',1,'final','/tmp/qg-accept.mp4','ready',10000);
INSERT INTO drama.qc_reports(qc_report_id,project_id,episode_id,master_id,overall_score,severity,status,version)
VALUES('qg_accept_qc','qg_accept_project','qg_accept_episode','qg_accept_master',100,'passed','completed',1);
SET LOCAL session_replication_role = origin;

DO $$
DECLARE was_blocked BOOLEAN := false;
BEGIN
  BEGIN
    INSERT INTO drama.final_reviews(final_review_id,project_id,episode_id,master_id,qc_report_id,review_status,reviewed_by,reviewed_at)
    VALUES('qg_accept_review_blocked','qg_accept_project','qg_accept_episode','qg_accept_master','qg_accept_qc','approved','fixture',now());
  EXCEPTION WHEN SQLSTATE 'P0001' THEN
    IF SQLERRM LIKE 'QUALITY_GATE_BLOCKED:%' THEN was_blocked := true; ELSE RAISE; END IF;
  END;
  IF NOT was_blocked THEN RAISE EXCEPTION 'approved final review bypassed cross-layer gate'; END IF;
END $$;

INSERT INTO drama.quality_gate_runs(gate_run_id,project_id,episode_id,master_id,ruleset_version,rules_config,
  rules_config_hash,snapshot,snapshot_hash,rule_score,rules_status,model_review_required,model_status,status)
VALUES('qgr_accept','qg_accept_project','qg_accept_episode','qg_accept_master','cross-layer-rules.v1','{}',repeat('a',64),
  '{"schema_version":"cross-layer-quality-gate.v1"}',repeat('b',64),100,'completed',false,'not_required','approved');
INSERT INTO drama.quality_gate_master_approvals(gate_approval_id,gate_run_id,project_id,episode_id,master_id,approved_by)
VALUES('qga_accept','qgr_accept','qg_accept_project','qg_accept_episode','qg_accept_master','fixture');
INSERT INTO drama.final_reviews(final_review_id,project_id,episode_id,master_id,qc_report_id,review_status,reviewed_by,reviewed_at)
VALUES('qg_accept_review_allowed','qg_accept_project','qg_accept_episode','qg_accept_master','qg_accept_qc','approved','fixture',now());

DO $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM drama.final_reviews WHERE final_review_id='qg_accept_review_allowed') THEN
    RAISE EXCEPTION 'valid quality-gated final review was not saved';
  END IF;
END $$;

ROLLBACK;
