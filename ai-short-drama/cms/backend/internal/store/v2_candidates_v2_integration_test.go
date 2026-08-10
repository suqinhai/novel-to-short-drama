package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"short-drama-cms/backend/internal/candidategeneration"
	"short-drama-cms/backend/internal/localedit"
)

func TestPluggableCandidateFrozenReplayAndDownstreamConsumption(t *testing.T) {
	databaseURL := os.Getenv("PHASE21_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE21_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suffix, _ := newPublicID("")
	const projectID = "p_phase1_legacy"
	const episodeID = "ep_phase1_legacy_001"
	const dialogueID = "dlg_phase5_1"
	lineage := prepareAuthoritativeE2ELineage(t, ctx, database, projectID, suffix)
	var nativeText, nativeHash, unrelatedStatusBefore string
	if err := database.pool.QueryRow(ctx, `SELECT text,encode(digest(convert_to(to_jsonb(dialogue)::text,'UTF8'),'sha256'),'hex')
		FROM drama.dialogues dialogue WHERE dialogue_id=$1`, dialogueID).Scan(&nativeText, &nativeHash); err != nil {
		t.Fatal(err)
	}
	if err := database.pool.QueryRow(ctx, `SELECT validity_status FROM drama.artifacts
		WHERE artifact_id='artifact_phase5_sound_bgm'`).Scan(&unrelatedStatusBefore); err != nil {
		t.Fatal(err)
	}
	var baseVersion int
	if err := database.pool.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM drama.entity_versions
		WHERE entity_type='dialogue' AND entity_id=$1 AND is_current),1)`, dialogueID).Scan(&baseVersion); err != nil {
		t.Fatal(err)
	}
	newDialogue := "authoritative-candidate-input-" + suffix
	plan, err := localedit.Build(localedit.Request{
		Instruction: "version the dialogue before candidate generation",
		Target:      localedit.Target{EntityType: "dialogue", EntityID: dialogueID, Version: baseVersion},
		Changes:     []localedit.Change{{Operation: "replace", Field: "text", Value: newDialogue}},
	})
	if err != nil {
		t.Fatal(err)
	}
	change, err := database.CreateChangePlan(ctx, projectID, plan, nil)
	if err != nil || change.Status != "validated" || len(change.Plan.ExpectedChanges) != 1 || len(change.Impacts) == 0 {
		t.Fatalf("change plan preview missing diff/impact: %#v err=%v", change, err)
	}
	replayedPlan, err := database.CreateChangePlan(ctx, projectID, plan, nil)
	if err != nil || replayedPlan.ChangePlanID != change.ChangePlanID {
		t.Fatalf("change plan retry was not idempotent: %s/%s err=%v", change.ChangePlanID, replayedPlan.ChangePlanID, err)
	}
	change, err = database.ConfirmChangePlan(ctx, projectID, change.ChangePlanID, nil)
	if err != nil || change.Status != "confirmed" {
		t.Fatalf("confirm change plan: %#v err=%v", change, err)
	}
	confirmedReplay, err := database.ConfirmChangePlan(ctx, projectID, change.ChangePlanID, nil)
	if err != nil || confirmedReplay.ChangePlanID != change.ChangePlanID || confirmedReplay.Status != "confirmed" {
		t.Fatalf("confirm retry was not idempotent: %#v err=%v", confirmedReplay, err)
	}
	change, err = database.ExecuteChangePlan(ctx, projectID, change.ChangePlanID)
	if err != nil || change.Status != "applied" || len(change.RebuildTasks) == 0 {
		t.Fatalf("execute change plan: %#v err=%v", change, err)
	}
	appliedReplay, err := database.ExecuteChangePlan(ctx, projectID, change.ChangePlanID)
	if err != nil || appliedReplay.ChangePlanID != change.ChangePlanID || appliedReplay.Status != "applied" ||
		len(appliedReplay.RebuildTasks) != len(change.RebuildTasks) {
		t.Fatalf("execute retry duplicated or changed the plan: %#v err=%v", appliedReplay, err)
	}
	for _, rebuild := range change.RebuildTasks {
		if rebuild.Status != "pending" || rebuild.Provider != "workflow" {
			t.Fatalf("rebuild was not pending for a real worker: %#v", rebuild)
		}
	}
	var currentEntityVersionID, currentBindingID, currentText, currentHash, nativeTextAfter, nativeHashAfter, unrelatedStatusAfter string
	var currentVersion int
	if err := database.pool.QueryRow(ctx, `SELECT versioned.entity_version_id,binding.binding_id,
		versioned.version,versioned.content->>'text',versioned.content_hash
		FROM drama.entity_versions versioned JOIN drama.entity_version_bindings binding
		  ON binding.entity_version_id=versioned.entity_version_id AND binding.is_current
		WHERE versioned.entity_type='dialogue' AND versioned.entity_id=$1 AND versioned.is_current`, dialogueID).
		Scan(&currentEntityVersionID, &currentBindingID, &currentVersion, &currentText, &currentHash); err != nil {
		t.Fatal(err)
	}
	if err := database.pool.QueryRow(ctx, `SELECT text,encode(digest(convert_to(to_jsonb(dialogue)::text,'UTF8'),'sha256'),'hex')
		FROM drama.dialogues dialogue WHERE dialogue_id=$1`, dialogueID).Scan(&nativeTextAfter, &nativeHashAfter); err != nil {
		t.Fatal(err)
	}
	if err := database.pool.QueryRow(ctx, `SELECT validity_status FROM drama.artifacts
		WHERE artifact_id='artifact_phase5_sound_bgm'`).Scan(&unrelatedStatusAfter); err != nil {
		t.Fatal(err)
	}
	if currentVersion != baseVersion+1 || currentText != newDialogue || currentHash == nativeHash ||
		nativeTextAfter != nativeText || nativeHashAfter != nativeHash || unrelatedStatusAfter != unrelatedStatusBefore {
		t.Fatalf("immutable switch mismatch version=%d text=%q native=%q/%q hash=%s/%s unrelated=%s/%s",
			currentVersion, currentText, nativeText, nativeTextAfter, nativeHash, currentHash,
			unrelatedStatusBefore, unrelatedStatusAfter)
	}
	stalePlan, buildErr := localedit.Build(localedit.Request{
		Instruction: "stale base must fail",
		Target:      localedit.Target{EntityType: "dialogue", EntityID: dialogueID, Version: baseVersion},
		Changes:     []localedit.Change{{Operation: "replace", Field: "emotion", Value: "stale"}},
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	if _, err := database.CreateChangePlan(ctx, projectID, stalePlan, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale-base preview must be rejected, got %v", err)
	}
	preCandidateResolution, err := database.ResolveEffectiveInputs(ctx, projectID, episodeID, "episode_script")
	if err != nil {
		t.Fatal(err)
	}
	var preCandidateEnvelope struct {
		Ready    bool `json:"ready"`
		Blockers []struct {
			Kind  string `json:"kind"`
			State string `json:"state"`
		} `json:"blockers"`
	}
	if err := json.Unmarshal(preCandidateResolution, &preCandidateEnvelope); err != nil || preCandidateEnvelope.Ready || len(preCandidateEnvelope.Blockers) != 1 ||
		preCandidateEnvelope.Blockers[0].Kind != "candidate_selection" ||
		(preCandidateEnvelope.Blockers[0].State != "stale" && preCandidateEnvelope.Blockers[0].State != "needs_review") {
		t.Fatalf("superseded candidate was not the sole downstream blocker before regeneration: %s err=%v", preCandidateResolution, err)
	}
	request := GenerateCandidateSetInput{Request: candidategeneration.Request{
		TargetType: "episode", TargetID: "ep_phase1_legacy_001",
		ComponentTypes: []string{"opening", "conflict", "climax", "ending_hook"}, CandidateCount: 3,
		DifferenceDirections: []string{"强钩子", "紧凑节奏", "低成本可拍"},
		MustPreserve:         []string{"开门事件", "钥匙线索"}, AllowedChanges: []string{"对白", "镜头节奏"},
		GeneratorProvider: "deterministic_mock", GeneratorModel: "deterministic-generator-v2",
		ReviewerProvider: "deterministic_mock", ReviewerModel: "deterministic-reviewer-v2", BlindReview: true,
		RandomSeed: 20260801, BaseDurationSeconds: 90, GenerationParameters: json.RawMessage(`{"temperature":0}`),
	}}
	set, created, err := database.GenerateCandidateSet(ctx, projectID, "phase21-generate-"+suffix, request)
	if err != nil || !created || len(set.Candidates) != 3 {
		t.Fatalf("candidate set created=%v count=%d err=%v", created, len(set.Candidates), err)
	}
	if set.GeneratorProvider != request.GeneratorProvider || set.GeneratorModel != request.GeneratorModel ||
		set.ReviewerProvider != request.ReviewerProvider || set.ReviewerModel != request.ReviewerModel || !set.BlindReview {
		t.Fatalf("candidate provider/reviewer audit was lost from the persisted response: %#v", set)
	}
	hashes := map[string]bool{}
	for _, candidate := range set.Candidates {
		hashes[candidate.ContentHash] = true
		var dimensions []candidategeneration.DimensionScore
		if err := json.Unmarshal(candidate.Score.Dimensions, &dimensions); err != nil || len(dimensions) != 9 {
			t.Fatalf("candidate %s dimensions=%d err=%v", candidate.CandidateID, len(dimensions), err)
		}
		for _, dimension := range dimensions {
			if len(dimension.Evidence) == 0 || dimension.Evidence[0].SourceID == "" || dimension.Evidence[0].Path == "" ||
				len(dimension.Deductions) == 0 || dimension.Deductions[0].Location.Path == "" {
				t.Fatalf("dimension %s is not locatable: %#v", dimension.Dimension, dimension)
			}
		}
	}
	if len(hashes) != 3 {
		t.Fatalf("three difference directions did not produce three bodies: %#v", hashes)
	}
	var frozen candidategeneration.FrozenInput
	if err := json.Unmarshal(set.FrozenInput, &frozen); err != nil || frozen.ResolutionID != set.FrozenResolutionID ||
		!strings.Contains(string(frozen.TargetContext), newDialogue) ||
		!strings.Contains(string(frozen.TargetContext), currentEntityVersionID) ||
		!strings.Contains(string(frozen.TargetContext), currentBindingID) ||
		!strings.Contains(string(frozen.TargetContext), change.ChangePlanID) ||
		!strings.Contains(string(frozen.Resolution), lineage.MergedFullIRRevisionID) ||
		!strings.Contains(string(frozen.Resolution), lineage.AdaptationSpecVersionID) ||
		!strings.Contains(string(frozen.Resolution), lineage.AdaptationPlanID) ||
		!strings.Contains(string(frozen.Resolution), lineage.PacingPlanID) {
		t.Fatalf("provider input did not use current Resolver snapshot: %s err=%v", set.FrozenInput, err)
	}
	var generatedExecutions, evaluatedExecutions, successfulExecutions, blindEvaluations int
	if err := database.pool.QueryRow(ctx, `SELECT
		count(*) FILTER(WHERE execution_type='generation'),
		count(*) FILTER(WHERE execution_type='evaluation'),
		count(*) FILTER(WHERE status='succeeded' AND completed_at IS NOT NULL),
		count(*) FILTER(WHERE execution_type='evaluation' AND blind)
		FROM drama.candidate_execution_records WHERE candidate_set_id=$1`, set.CandidateSetID).
		Scan(&generatedExecutions, &evaluatedExecutions, &successfulExecutions, &blindEvaluations); err != nil {
		t.Fatal(err)
	}
	if generatedExecutions != 3 || evaluatedExecutions != 3 || successfulExecutions != 6 || blindEvaluations != 3 {
		t.Fatalf("independent execution audit incomplete gen=%d eval=%d success=%d blind=%d",
			generatedExecutions, evaluatedExecutions, successfulExecutions, blindEvaluations)
	}
	if _, _, err := database.SelectCandidate(ctx, projectID, set.CandidateSetID,
		"phase21-unconfirmed-"+suffix, CandidateSelectionInput{CandidateID: set.Candidates[0].CandidateID, Confirmed: false}); !errors.Is(err, ErrConflict) {
		t.Fatalf("unconfirmed candidate entered selection path: %v", err)
	}
	replay, replayCreated, err := database.GenerateCandidateSet(ctx, projectID, "phase21-replay-other-key-"+suffix, request)
	if err != nil || replayCreated || replay.CandidateSetID != set.CandidateSetID || replay.FrozenInputHash != set.FrozenInputHash {
		t.Fatalf("frozen replay set=%s/%s created=%v err=%v", set.CandidateSetID, replay.CandidateSetID, replayCreated, err)
	}
	selected, selectedCreated, err := database.SelectCandidate(ctx, projectID, set.CandidateSetID,
		"phase21-select-"+suffix, CandidateSelectionInput{CandidateID: set.Candidates[0].CandidateID, Confirmed: true, ConfirmedBy: "phase21-test"})
	if err != nil || !selectedCreated {
		t.Fatalf("selection created=%v err=%v", selectedCreated, err)
	}
	selectionReplay, selectionReplayCreated, err := database.SelectCandidate(ctx, projectID, set.CandidateSetID,
		"phase21-select-"+suffix, CandidateSelectionInput{CandidateID: set.Candidates[0].CandidateID, Confirmed: true, ConfirmedBy: "phase21-test"})
	if err != nil || selectionReplayCreated || selectionReplay.CandidateSelectionID != selected.CandidateSelectionID {
		t.Fatalf("selection retry was not idempotent: %#v created=%v err=%v", selectionReplay, selectionReplayCreated, err)
	}
	finalSelection, finalCreated, err := database.SelectCandidate(ctx, projectID, set.CandidateSetID,
		"phase21-switch-"+suffix, CandidateSelectionInput{CandidateID: set.Candidates[1].CandidateID, Confirmed: true, ConfirmedBy: "phase21-test"})
	if err != nil || !finalCreated || finalSelection.CandidateSelectionID == selected.CandidateSelectionID {
		t.Fatalf("candidate switch failed: %#v created=%v err=%v", finalSelection, finalCreated, err)
	}
	var finalSelectionBindingID string
	if err := database.pool.QueryRow(ctx, `SELECT binding_id FROM drama.candidate_selection_bindings
		WHERE project_id=$1 AND target_type='episode' AND target_id=$2 AND is_current`, projectID, episodeID).
		Scan(&finalSelectionBindingID); err != nil {
		t.Fatal(err)
	}
	var raw json.RawMessage
	traceID := "phase21-next-storyboard-" + suffix
	err = database.writer.QueryRow(ctx, `SELECT drama.claim_effective_inputs($1,$2,$3,$4,$5)`,
		projectID, episodeID, "06", traceID, 1).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	var claim struct {
		Allowed bool `json:"allowed"`
		Items   []struct {
			Kind     string   `json:"kind"`
			InputIDs []string `json:"input_ids"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &claim); err != nil || !claim.Allowed {
		t.Fatalf("next generation was not allowed to consume the selection: %s err=%v", raw, err)
	}
	found := false
	for _, item := range claim.Items {
		if item.Kind == "candidate_selection" && len(item.InputIDs) == 1 && item.InputIDs[0] == finalSelection.CandidateSelectionID {
			found = true
		}
	}
	if !found {
		t.Fatalf("next generation did not consume selection %s: %s", finalSelection.CandidateSelectionID, raw)
	}
	if !strings.Contains(string(raw), newDialogue) || !strings.Contains(string(raw), currentEntityVersionID) ||
		!strings.Contains(string(raw), currentBindingID) || !strings.Contains(string(raw), change.ChangePlanID) ||
		!strings.Contains(string(raw), finalSelectionBindingID) || !strings.Contains(string(raw), set.PromptVersion) ||
		!strings.Contains(string(raw), set.GeneratorProvider) || !strings.Contains(string(raw), set.ReviewerProvider) ||
		!strings.Contains(string(raw), lineage.SourceBindingID) {
		t.Fatalf("downstream claim lost version/candidate/provider provenance: %s", raw)
	}
	taskID := "phase21-storyboard-task-" + suffix
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.workflow_tasks(
		task_id,trace_id,project_id,workflow_stage,action,entity_type,entity_id,generation_version,
		idempotency_key,status,input_data,output_data,completed_at)
		VALUES($1,$2,$3,'storyboard_design','run','episode',$4,1,$5,'completed','{}'::jsonb,
		jsonb_build_object('data_ref',jsonb_build_object('entity_id','storyboard_phase5_post')),CURRENT_TIMESTAMP)`,
		taskID, traceID, projectID, episodeID, "phase21:storyboard-task:"+suffix); err != nil {
		t.Fatal(err)
	}
	var recorded json.RawMessage
	if err := database.writer.QueryRow(ctx, `SELECT drama.record_effective_input_outputs($1,$2)`, traceID, "06").Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	var provenance json.RawMessage
	if err := database.pool.QueryRow(ctx, `SELECT event.details FROM drama.artifact_provenance_events event
		JOIN drama.artifacts artifact USING(artifact_id)
		WHERE artifact.project_id=$1 AND artifact.artifact_type='storyboard'
		  AND artifact.native_entity_id='storyboard_phase5_post' AND event.details->>'trace_id'=$2`, projectID, traceID).
		Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{lineage.MergedFullIRRevisionID, lineage.AdaptationSpecVersionID,
		lineage.AdaptationPlanID, lineage.PacingPlanID, lineage.SourceBindingID, change.ChangePlanID, currentEntityVersionID,
		currentBindingID, finalSelection.CandidateSelectionID, finalSelectionBindingID, set.PromptVersion,
		set.GeneratorProvider, set.GeneratorModel, set.ReviewerProvider, set.ReviewerModel, newDialogue} {
		if !strings.Contains(string(provenance), required) {
			t.Fatalf("final generation provenance missing %q: %s (record=%s)", required, provenance, recorded)
		}
	}
	var currentCandidates, candidateBindings int
	if err := database.writer.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM drama.artifacts artifact JOIN drama.candidates candidate USING(artifact_id)
		 WHERE candidate.candidate_set_id=$1 AND artifact.is_current),
		(SELECT count(*) FROM drama.artifact_current_bindings binding JOIN drama.candidates candidate
		 ON candidate.artifact_id=binding.current_artifact_id WHERE candidate.candidate_set_id=$1)`, set.CandidateSetID).
		Scan(&currentCandidates, &candidateBindings); err != nil {
		t.Fatal(err)
	}
	if currentCandidates != 0 || candidateBindings != 0 {
		t.Fatalf("unselected candidate leaked downstream: current=%d bindings=%d", currentCandidates, candidateBindings)
	}
	t.Logf("E2E evidence project=%s source_version=%s source_binding=%s base_full_ir=%s incremental_ir=%s merge_proposal=%s merged_full_ir=%s source_change_set=%s spec=%s plan=%s pacing=%s change_plan=%s entity_version=%s version=%d entity_binding=%s old_hash=%s new_hash=%s impacts=%d pending_rebuilds=%d candidate_set=%s final_candidate=%s candidate_selection=%s candidate_binding=%s prompt=%s generator=%s/%s reviewer=%s/%s final_provenance_bytes=%d",
		projectID, lineage.SourceVersionID, lineage.SourceBindingID, lineage.BaseFullIRRevisionID, lineage.IncrementalIRRevisionID, lineage.MergeProposalID,
		lineage.MergedFullIRRevisionID, lineage.SourceChangeSetID, lineage.AdaptationSpecVersionID,
		lineage.AdaptationPlanID, lineage.PacingPlanID, change.ChangePlanID, currentEntityVersionID, currentVersion, currentBindingID,
		nativeHash, currentHash, len(change.Impacts), len(change.RebuildTasks), set.CandidateSetID,
		set.Candidates[1].CandidateID, finalSelection.CandidateSelectionID, finalSelectionBindingID,
		set.PromptVersion, set.GeneratorProvider, set.GeneratorModel, set.ReviewerProvider, set.ReviewerModel, len(provenance))
}
