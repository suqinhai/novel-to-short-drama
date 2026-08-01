package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"short-drama-cms/backend/internal/candidategeneration"
)

func TestPluggableCandidateFrozenReplayAndDownstreamConsumption(t *testing.T) {
	databaseURL := os.Getenv("PHASE21_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE21_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suffix, _ := newPublicID("")
	request := GenerateCandidateSetInput{Request: candidategeneration.Request{
		TargetType: "episode", TargetID: "ep_phase1_legacy_001",
		ComponentTypes: []string{"opening", "conflict", "climax", "ending_hook"}, CandidateCount: 3,
		DifferenceDirections: []string{"强钩子", "紧凑节奏", "低成本可拍"},
		MustPreserve:         []string{"开门事件", "钥匙线索"}, AllowedChanges: []string{"对白", "镜头节奏"},
		GeneratorProvider: "deterministic_mock", GeneratorModel: "deterministic-generator-v2",
		ReviewerProvider: "deterministic_mock", ReviewerModel: "deterministic-reviewer-v2", BlindReview: true,
		RandomSeed: 20260801, BaseDurationSeconds: 90, GenerationParameters: json.RawMessage(`{"temperature":0}`),
	}}
	set, created, err := database.GenerateCandidateSet(ctx, "p_phase1_legacy", "phase21-generate-"+suffix, request)
	if err != nil || !created || len(set.Candidates) != 3 {
		t.Fatalf("candidate set created=%v count=%d err=%v", created, len(set.Candidates), err)
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
	replay, replayCreated, err := database.GenerateCandidateSet(ctx, "p_phase1_legacy", "phase21-replay-other-key-"+suffix, request)
	if err != nil || replayCreated || replay.CandidateSetID != set.CandidateSetID || replay.FrozenInputHash != set.FrozenInputHash {
		t.Fatalf("frozen replay set=%s/%s created=%v err=%v", set.CandidateSetID, replay.CandidateSetID, replayCreated, err)
	}
	selected, selectedCreated, err := database.SelectCandidate(ctx, "p_phase1_legacy", set.CandidateSetID,
		"phase21-select-"+suffix, CandidateSelectionInput{CandidateID: set.Candidates[0].CandidateID, Confirmed: true, ConfirmedBy: "phase21-test"})
	if err != nil || !selectedCreated {
		t.Fatalf("selection created=%v err=%v", selectedCreated, err)
	}
	var raw json.RawMessage
	err = database.writer.QueryRow(ctx, `SELECT drama.claim_effective_inputs($1,$2,$3,$4,$5)`,
		"p_phase1_legacy", "ep_phase1_legacy_001", "06", "phase21-next-storyboard-"+suffix, 1).Scan(&raw)
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
		if item.Kind == "candidate_selection" && len(item.InputIDs) == 1 && item.InputIDs[0] == selected.CandidateSelectionID {
			found = true
		}
	}
	if !found {
		t.Fatalf("next generation did not consume selection %s: %s", selected.CandidateSelectionID, raw)
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
}
