package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"short-drama-cms/backend/internal/qualitygate"
)

func TestCrossLayerGatePersistenceAndApprovalIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE28_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE28_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	seedQualityGateIntegration(t, ctx, database)
	defer database.writer.Exec(ctx, `DELETE FROM drama.projects WHERE project_id='qg_store_project'`)

	artifacts := make([]qualitygate.Artifact, 0, len(qualitygate.StageOrder))
	for _, stage := range qualitygate.StageOrder {
		artifact := qualitygate.Artifact{Stage: stage, ArtifactID: string(stage) + "_store", Version: 1, DurationMS: 10000}
		if stage == qualitygate.StageEditTimeline {
			artifact.Timeline = []qualitygate.TimelineItem{
				{TimelineItemID: "video_store", TrackType: "video", EntityType: "shot", EntityID: "shot_store", StartMS: 0, EndMS: 10000},
				{TimelineItemID: "audio_store", TrackType: "audio", EntityType: "audio", EntityID: "audio_store", StartMS: 0, EndMS: 10000},
			}
		}
		artifacts = append(artifacts, artifact)
	}
	artifacts[0].Facts = []qualitygate.Fact{{Key: "killer", Value: "uncle", Critical: true,
		SourceSpanID: "span_store", RequiredStages: []qualitygate.Stage{qualitygate.StageAdaptationPlan}}}
	artifacts[1].Facts = []qualitygate.Fact{{Key: "killer", Value: "butler"}}
	snapshot := qualitygate.Snapshot{SchemaVersion: qualitygate.SchemaVersion, ProjectID: "qg_store_project",
		EpisodeID: "qg_store_episode", MasterID: "qg_store_master", DurationMS: 10000, Artifacts: artifacts}
	run, err := qualitygate.EvaluateRules(snapshot, qualitygate.DefaultConfig(), false)
	if err != nil {
		t.Fatal(err)
	}
	record, err := database.SaveQualityGateRuleRun(ctx, snapshot, run, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Findings) != 1 || record.Findings[0].Code != "CRITICAL_FACT_CHANGED" {
		t.Fatalf("unexpected persisted findings: %#v", record.Findings)
	}
	if _, err = database.ApproveQualityGateMaster(ctx, snapshot.ProjectID, snapshot.EpisodeID, run.GateRunID, "tester"); !errors.Is(err, ErrConflict) {
		t.Fatalf("open blocker must prevent approval: %v", err)
	}
	if _, err = database.ResolveQualityGateFinding(ctx, snapshot.ProjectID, snapshot.EpisodeID, run.GateRunID,
		record.Findings[0].FindingID, "", "fixed", "tester"); !errors.Is(err, ErrValidation) {
		t.Fatalf("resolution without a local change plan must fail: %v", err)
	}
	plan, err := database.CreateQualityGateChangePlan(ctx, snapshot.ProjectID, snapshot.EpisodeID, run.GateRunID, record.Findings[0].FindingID, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if plan.DirectMutationAllowed || !plan.RequiresConfirmation {
		t.Fatalf("unsafe change plan: %#v", plan)
	}
	confirmed, err := database.ConfirmQualityGateFinding(ctx, snapshot.ProjectID, snapshot.EpisodeID, run.GateRunID,
		record.Findings[0].FindingID, "reviewed source evidence", "tester")
	if err != nil || confirmed.Findings[0].ResolutionKind != qualitygate.DispositionHumanConfirmed || confirmed.Findings[0].Status != qualitygate.FindingOpen {
		t.Fatalf("human confirmation must remain an open, auditable finding: %#v err=%v", confirmed.Findings, err)
	}
	if _, err = database.ResolveQualityGateFinding(ctx, snapshot.ProjectID, snapshot.EpisodeID, run.GateRunID,
		record.Findings[0].FindingID, run.GateRunID, "not rebuilt", "tester"); !errors.Is(err, ErrConflict) {
		t.Fatalf("same QA snapshot must not prove a rebuild: %v", err)
	}
	replacementSnapshot := snapshot
	replacementSnapshot.Artifacts = append([]qualitygate.Artifact(nil), snapshot.Artifacts...)
	replacementSnapshot.Artifacts[1].Facts = []qualitygate.Fact{{Key: "killer", Value: "uncle"}}
	replacementRun, err := qualitygate.EvaluateRules(replacementSnapshot, qualitygate.DefaultConfig(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(replacementRun.Findings) != 0 {
		t.Fatalf("replacement QA must prove the issue disappeared: %#v", replacementRun.Findings)
	}
	if _, err = database.SaveQualityGateRuleRun(ctx, replacementSnapshot, replacementRun, "tester"); err != nil {
		t.Fatal(err)
	}
	resolved, err := database.ResolveQualityGateFinding(ctx, snapshot.ProjectID, snapshot.EpisodeID, run.GateRunID,
		record.Findings[0].FindingID, replacementRun.GateRunID, "rebuilt and checked on replacement snapshot", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Findings[0].ResolutionKind != qualitygate.DispositionResolvedByRebuild ||
		resolved.Findings[0].ReplacementGateRunID != replacementRun.GateRunID {
		t.Fatalf("rebuild resolution provenance is incomplete: %#v", resolved.Findings[0])
	}
	approval, err := database.ApproveQualityGateMaster(ctx, snapshot.ProjectID, snapshot.EpisodeID, replacementRun.GateRunID, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if approval.MasterID != snapshot.MasterID || approval.Status != "active" {
		t.Fatalf("unexpected approval: %#v", approval)
	}
	if _, err = database.writer.Exec(ctx, `INSERT INTO drama.final_reviews(final_review_id,project_id,episode_id,master_id,
		qc_report_id,review_status,reviewed_by,reviewed_at) VALUES('qg_store_review','qg_store_project','qg_store_episode',
		'qg_store_master','qg_store_qc','approved','tester',now())`); err != nil {
		t.Fatalf("database gate rejected valid approval: %v", err)
	}
}

func seedQualityGateIntegration(t *testing.T, ctx context.Context, database *Store) {
	t.Helper()
	_, _ = database.writer.Exec(ctx, `DELETE FROM drama.projects WHERE project_id='qg_store_project'`)
	tx, err := database.writer.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	statements := []string{
		`SET LOCAL session_replication_role=replica`,
		`INSERT INTO drama.projects(project_id,novel_name,target_episode_count,episode_duration_seconds,visual_style,aspect_ratio,target_platform)
		 VALUES('qg_store_project','fixture',1,10,'fixture','9:16','test')`,
		`INSERT INTO drama.episode_outlines(episode_id,season_id,project_id,episode_number,title,estimated_duration_seconds,version)
		 VALUES('qg_store_episode','fake_season','qg_store_project',1,'fixture',10,1)`,
		`INSERT INTO drama.edit_timelines(timeline_id,project_id,episode_id,script_id,storyboard_id,audio_plan_id,version,
		 resolution,aspect_ratio,fps,video_codec,audio_codec,sample_rate,target_duration_ms,status,approval_state,is_current)
		 VALUES('qg_store_timeline','qg_store_project','qg_store_episode','fake_script','fake_storyboard','fake_audio',1,
		 '1080x1920','9:16',25,'h264','aac',48000,10000,'completed','approved',true)`,
		`INSERT INTO drama.episode_masters(master_id,project_id,episode_id,timeline_id,generation_version,master_type,local_path,status,duration_ms)
		 VALUES('qg_store_master','qg_store_project','qg_store_episode','qg_store_timeline',1,'final','/tmp/qg-store.mp4','ready',10000)`,
		`INSERT INTO drama.qc_reports(qc_report_id,project_id,episode_id,master_id,overall_score,severity,status,version)
		 VALUES('qg_store_qc','qg_store_project','qg_store_episode','qg_store_master',100,'passed','completed',1)`,
		`SET LOCAL session_replication_role=origin`,
	}
	for _, statement := range statements {
		if _, err = tx.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
