package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// The Phase 3 integration harness applies test-data/phase1-contract-seed.sql
// before setting PHASE3_DATABASE_URL. That fixture supplies one frozen
// source/IR/spec tuple without making this test mutate public contract rows.
func TestCompilerRunLifecycleIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE3_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE3_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	suffix, err := newPublicID("")
	if err != nil {
		t.Fatal(err)
	}
	key := "phase3-compiler-integration-" + suffix
	input := CompilerRunInput{
		AdaptationSpecVersionID: "adaptation_spec_version_phase1_001",
		IRRevisionID:            "ir_phase1_001",
		CompilerVersion:         "constraint-integration-" + suffix,
	}
	operation, err := database.StartCompilerRun(ctx, "p_phase1_legacy", key, input)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != "pending" || operation.OperationType != "adaptation_compile" || operation.TargetType != "project" {
		t.Fatalf("unexpected compiler operation: %#v", operation)
	}
	replay, err := database.StartCompilerRun(ctx, "p_phase1_legacy", key, input)
	if err != nil || replay.OperationID != operation.OperationID {
		t.Fatalf("compiler replay mismatch: %#v err=%v", replay, err)
	}

	plan, traceID, err := database.GetAdaptationPlan(ctx, "adaptation_plan_phase1_001")
	if err != nil || len(plan) == 0 || traceID == "" {
		t.Fatalf("reviewable plan read failed: trace=%q plan=%s err=%v", traceID, string(plan), err)
	}
	var decoded struct {
		SchemaVersion string `json:"schema_version"`
		Episodes      []struct {
			SourceEventIDs         []string `json:"source_event_ids"`
			SourceChapterIDs       []string `json:"source_chapter_ids"`
			AddedAdaptationContent []any    `json:"added_adaptation_content"`
			MergedContent          []any    `json:"merged_content"`
			DeviationNotes         []any    `json:"deviation_notes"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(plan, &decoded); err != nil || decoded.SchemaVersion != "compiler-plan.v2" ||
		len(decoded.Episodes) != 1 || len(decoded.Episodes[0].SourceEventIDs) == 0 || len(decoded.Episodes[0].SourceChapterIDs) == 0 ||
		decoded.Episodes[0].AddedAdaptationContent == nil || decoded.Episodes[0].MergedContent == nil || decoded.Episodes[0].DeviationNotes == nil {
		t.Fatalf("reviewable plan v2 audit fields missing: %#v err=%v", decoded, err)
	}
}
