package candidategeneration

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDeterministicMockGeneratesAndRanksThreeCandidates(t *testing.T) {
	request := Request{
		TargetType: "episode", TargetID: "episode_1",
		ComponentTypes: []string{"opening", "conflict", "climax", "ending_hook"},
		CandidateCount: 3, DifferenceDirections: []string{"强钩子", "紧凑节奏", "低成本可拍"},
		MustPreserve: []string{"凶手身份", "主角受伤"}, AllowedChanges: []string{"对白", "场景顺序"},
		Model: "deterministic_mock", RandomSeed: 42,
	}
	first, err := Generate(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || !reflect.DeepEqual(first, second) {
		t.Fatalf("expected three deterministic candidates")
	}
	if first[0].Score.TotalScore < first[1].Score.TotalScore {
		t.Fatalf("candidates are not ranked")
	}
	for _, candidate := range first {
		if len(candidate.Score.RecommendationReasons) == 0 || len(candidate.Score.DeductionReasons) == 0 {
			t.Fatalf("candidate %d lacks explainable scoring", candidate.Ordinal)
		}
	}
}

func TestCompositionRunsEveryHardRule(t *testing.T) {
	content := map[string]any{
		"components": []any{map[string]any{"key": "opening"}, map[string]any{"key": "climax"}},
	}
	validation := ValidateComposition(content, 300)
	if len(validation.Results) != 5 || !validation.Passed {
		t.Fatalf("expected all hard rules to run and pass: %#v", validation)
	}
	expected := []string{"causality", "duration", "character_state", "foreshadowing", "continuity"}
	for i, result := range validation.Results {
		if result.Rule != expected[i] {
			t.Fatalf("hard rule %d = %s", i, result.Rule)
		}
	}
	content["continuity_break"] = true
	validation = ValidateComposition(content, 300)
	if validation.Passed || validation.Results[4].Passed {
		t.Fatal("continuity break must fail the composition")
	}
}

func TestStructuredDiffIsMachineReadable(t *testing.T) {
	request := Request{
		TargetType: "scene", TargetID: "scene_1", ComponentTypes: []string{"dialogue"},
		CandidateCount: 2, DifferenceDirections: []string{"自然对白"}, Model: "deterministic_mock",
		BaseContent: json.RawMessage(`{"schema_version":"old"}`),
	}
	candidates, err := Generate(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates[0].StructuredDiff) == 0 || candidates[0].StructuredDiff[0].Path == "" {
		t.Fatal("expected structured diff")
	}
}
