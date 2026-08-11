package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestSeasonWorkbenchVersionApprovalAndQueueGateIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE25_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE25_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	draft := SeasonPlanDraft{
		SchemaVersion: "season-plan-draft.v1", PlanName: "整季对抗测试版", StrategyLabel: "causal-first",
		Episodes: []SeasonEpisodeDraft{{
			EpisodeNumber: 1, Title: "异常开端重排版", Logline: "林夏发现异常并获得钥匙线索",
			ThreeSecondOpening: "门在无人触碰时自动打开", FirstThirtySecondsGoal: "林夏确认异常来源并寻找出口",
			CoreConflict: "林夏想离开，但异常空间锁住出口", Climax: "林夏拿到能打开下一扇门的钥匙",
			EndingHook: "钥匙背面刻着她的名字", EmotionCurve: json.RawMessage(`[{"position":1,"emotion":0.55},{"position":2,"emotion":0.95}]`),
			InformationRevealAmount: 0.55, EstimatedDurationSeconds: 90,
			Events: []SeasonEventCard{
				{CardID: "card_phase25_1", PresentationMode: "preserve", SourceEventIDs: []string{"event_revision_phase1_001"}, Summary: "门自动打开"},
				{CardID: "card_phase25_2", PresentationMode: "preserve", SourceEventIDs: []string{"event_revision_phase1_002"}, Summary: "钥匙出现"},
			},
		}}, OmittedEvents: []SeasonEventCard{}, CreativeSuggestions: json.RawMessage(`[]`),
	}

	reversed := draft
	reversed.Episodes = append([]SeasonEpisodeDraft(nil), draft.Episodes...)
	reversed.Episodes[0].Events = []SeasonEventCard{draft.Episodes[0].Events[1], draft.Episodes[0].Events[0]}
	adversarial, err := database.ValidateSeasonPlanDraft(ctx, "adaptation_plan_phase1_001", reversed)
	if err != nil {
		t.Fatal(err)
	}
	if adversarial.Passed || !hasSeasonDiagnostic(adversarial.Diagnostics, "CAUSAL_ORDER_VIOLATION") {
		t.Fatalf("reversed causal chain was not blocked: %#v", adversarial)
	}

	beforeHash := ""
	if err = database.pool.QueryRow(ctx, `SELECT content_hash FROM drama.adaptation_plans
		WHERE adaptation_plan_id='adaptation_plan_phase1_001'`).Scan(&beforeHash); err != nil {
		t.Fatal(err)
	}
	preview, err := database.PreviewSeasonPlanChange(ctx, "adaptation_plan_phase1_001", draft)
	if err != nil || preview.ChangePlanID == "" || preview.Status != "validated" ||
		preview.PreviewFingerprint == "" || len(preview.Diff) == 0 {
		t.Fatalf("change-plan preview was not created: %#v err=%v", preview, err)
	}
	if _, err = database.ConfirmChangePlan(ctx, "p_phase1_legacy", preview.ChangePlanID, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("season change plan bypassed its atomic confirmation endpoint: %v", err)
	}
	draft.PreviewFingerprint = preview.PreviewFingerprint
	planRaw, _, err := database.CreateSeasonPlanVersion(ctx, "adaptation_plan_phase1_001", "phase25-save-version", draft)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		AdaptationPlanID string `json:"adaptation_plan_id"`
		VersionNumber    int    `json:"version_number"`
		Status           string `json:"status"`
	}
	if err = json.Unmarshal(planRaw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.AdaptationPlanID == "" || saved.VersionNumber <= 1 || saved.Status != "waiting_review" {
		t.Fatalf("save did not create an immutable successor: %#v", saved)
	}
	replayRaw, _, err := database.CreateSeasonPlanVersion(ctx, "adaptation_plan_phase1_001", "phase25-save-version", draft)
	if err != nil {
		t.Fatalf("repeated season change confirmation was not idempotent: %v", err)
	}
	var replay struct {
		AdaptationPlanID string `json:"adaptation_plan_id"`
	}
	if err = json.Unmarshal(replayRaw, &replay); err != nil || replay.AdaptationPlanID != saved.AdaptationPlanID {
		t.Fatalf("repeated confirmation created a different successor: %#v err=%v", replay, err)
	}
	var changePlanStatus, resultPlanID string
	if err = database.pool.QueryRow(ctx, `SELECT status,review_metadata->>'result_adaptation_plan_id'
		FROM drama.change_plans WHERE change_plan_id=$1`, preview.ChangePlanID).
		Scan(&changePlanStatus, &resultPlanID); err != nil || changePlanStatus != "applied" || resultPlanID != saved.AdaptationPlanID {
		t.Fatalf("season change plan audit did not close atomically: status=%s result=%s err=%v",
			changePlanStatus, resultPlanID, err)
	}
	if _, err = database.PreviewSeasonPlanChange(ctx, "adaptation_plan_phase1_001", draft); !errors.Is(err, ErrConflict) {
		t.Fatalf("superseded base version was not rejected: %v", err)
	}
	var afterHash string
	if err = database.pool.QueryRow(ctx, `SELECT content_hash FROM drama.adaptation_plans
		WHERE adaptation_plan_id='adaptation_plan_phase1_001'`).Scan(&afterHash); err != nil || afterHash != beforeHash {
		t.Fatalf("approved base plan content changed: before=%s after=%s err=%v", beforeHash, afterHash, err)
	}

	var queueCountBefore int
	if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.story_arc_runs WHERE project_id='p_phase1_legacy'`).Scan(&queueCountBefore); err != nil {
		t.Fatal(err)
	}
	if _, err = database.AdoptAdaptationPlan(ctx, "p_phase1_legacy", saved.AdaptationPlanID, AdoptRollingPlanInput{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("waiting-review plan bypassed approval gate: %v", err)
	}
	var queueCountAfterRejected int
	_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.story_arc_runs WHERE project_id='p_phase1_legacy'`).Scan(&queueCountAfterRejected)
	if queueCountAfterRejected != queueCountBefore {
		t.Fatalf("rejected queue attempt mutated queue: before=%d after=%d", queueCountBefore, queueCountAfterRejected)
	}

	approval, err := database.ApproveSeasonPlan(ctx, saved.AdaptationPlanID, "phase25-reviewer")
	if err != nil || approval.Status != "approved" || approval.QueueCreated || !approval.Validation.Passed {
		t.Fatalf("approval validation failed or created queue early: %#v err=%v", approval, err)
	}
	var publishedArtifacts int
	if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.artifacts
		WHERE project_id='p_phase1_legacy' AND is_current AND validity_status='valid'
		  AND ((artifact_type='adaptation_plan' AND native_entity_id=$1)
		    OR (artifact_type='adaptation_episode_plan' AND metadata->>'adaptation_plan_id'=$1))`,
		saved.AdaptationPlanID).Scan(&publishedArtifacts); err != nil || publishedArtifacts < 2 {
		t.Fatalf("approved workbench plan was not published into the resolver artifact graph: count=%d err=%v", publishedArtifacts, err)
	}
	resolution, err := database.ResolveEffectiveInputs(ctx, "p_phase1_legacy", "ep_phase1_legacy_001", "episode_script")
	if err != nil || !bytes.Contains(resolution, []byte(saved.AdaptationPlanID)) {
		t.Fatalf("effective input resolver did not consume approved workbench plan: %s err=%v", resolution, err)
	}
	var queueCountAfterApproval int
	_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.story_arc_runs WHERE project_id='p_phase1_legacy'`).Scan(&queueCountAfterApproval)
	if queueCountAfterApproval != queueCountBefore {
		t.Fatalf("approval created production queue: before=%d after=%d", queueCountBefore, queueCountAfterApproval)
	}

	rolling, err := database.AdoptAdaptationPlan(ctx, "p_phase1_legacy", saved.AdaptationPlanID,
		AdoptRollingPlanInput{MaxVideoBatch: 3, Currency: "CNY"})
	if err != nil {
		t.Fatalf("approved plan did not establish queue: %#v err=%v", rolling, err)
	}
	queueFound := false
	for _, arc := range rolling.Arcs {
		if arc.AdaptationPlanID != nil && *arc.AdaptationPlanID == saved.AdaptationPlanID {
			queueFound = true
			break
		}
	}
	if !queueFound {
		t.Fatalf("approved plan queue was not created: %#v", rolling)
	}
	var validationRuns int
	if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.adaptation_plan_validation_runs
		WHERE adaptation_plan_id=$1 AND validation_scope='approval' AND passed`, saved.AdaptationPlanID).Scan(&validationRuns); err != nil || validationRuns != 1 {
		t.Fatalf("approval validation was not audited: count=%d err=%v", validationRuns, err)
	}
}

func hasSeasonDiagnostic(items []SeasonDiagnostic, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}
