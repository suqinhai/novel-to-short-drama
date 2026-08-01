package candidategeneration

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func frozenFixture() FrozenInput {
	return FrozenInput{SchemaVersion: "candidate-frozen-input.v1", ResolutionID: "resolution_test",
		ContextHash:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ResolutionHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		FrozenHash:     "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		Stage:          "episode_script", EpisodeID: "episode_1",
		Resolution:    json.RawMessage(`{"schema_version":"effective-input-resolution.v1","resolution_id":"resolution_test","items":[{"kind":"narrative_ir","input_ids":["ir_1"],"content":{"event_count":3}}]}`),
		TargetContext: json.RawMessage(`{"source_kind":"script","episode":{"episode_id":"episode_1","opening_hook":"门外有人"}}`)}
}

func mockRequest() Request {
	return Request{TargetType: "episode", TargetID: "episode_1",
		ComponentTypes: []string{"opening", "conflict", "climax", "ending_hook"}, CandidateCount: 3,
		DifferenceDirections: []string{"强钩子", "紧凑节奏", "低成本可拍"},
		MustPreserve:         []string{"凶手身份", "主角受伤"}, AllowedChanges: []string{"对白", "场景顺序"},
		GeneratorProvider: "deterministic_mock", GeneratorModel: "deterministic-generator-v2",
		ReviewerProvider: "deterministic_mock", ReviewerModel: "deterministic-reviewer-v2",
		RandomSeed: 42, FrozenInput: frozenFixture(), GenerationParameters: json.RawMessage(`{"temperature":0}`)}
}

func TestDeterministicMockProducesThreeDistinctReplayableBodies(t *testing.T) {
	request := mockRequest()
	first, err := Generate(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || !reflect.DeepEqual(first, second) {
		t.Fatalf("expected three replayable candidates")
	}
	bodies := map[string]bool{}
	for _, candidate := range first {
		encoded, _ := json.Marshal(candidate.Content)
		bodies[string(encoded)] = true
		if err := ValidateScore(candidate.Score); err != nil {
			t.Fatalf("candidate %d lacks evidence-based scoring: %v", candidate.Ordinal, err)
		}
	}
	if len(bodies) != 3 {
		t.Fatalf("difference directions changed labels but not bodies: %#v", first)
	}
}

func TestGeneratorAndReviewerCannotBeSameModel(t *testing.T) {
	request := mockRequest()
	request.ReviewerModel = request.GeneratorModel
	if err := ValidateRequest(request); err == nil {
		t.Fatal("same provider/model must not generate and score its own candidate")
	}
}

func TestCompositionRunsEveryHardRuleWithEvidence(t *testing.T) {
	content := map[string]any{"components": []any{
		map[string]any{"key": "opening", "type": "opening", "content": "主角发现门锁被换。"},
		map[string]any{"key": "climax", "type": "climax", "content": "门后证据迫使嫌疑人承认撒谎。"},
	}}
	validation := ValidateComposition(content, 300)
	if len(validation.Results) != 5 || !validation.Passed {
		t.Fatalf("expected all hard rules to run and pass: %#v", validation)
	}
	expected := []string{"causality", "duration", "character_state", "foreshadowing", "continuity"}
	for index, result := range validation.Results {
		if result.Rule != expected[index] || len(result.Evidence) == 0 || result.Evidence[0].Path == "" {
			t.Fatalf("hard rule %d is not evidence-based: %#v", index, result)
		}
	}
	content["continuity_break"] = true
	validation = ValidateComposition(content, 300)
	if validation.Passed || validation.Results[4].Passed {
		t.Fatal("continuity break must fail the composition")
	}
}

type alwaysFailProvider struct{}

func (alwaysFailProvider) Name() string      { return "real_fail" }
func (alwaysFailProvider) MediaKind() string { return "text" }
func (alwaysFailProvider) Generate(context.Context, GenerationInput) (CandidateDraft, error) {
	return CandidateDraft{}, errors.New("upstream unavailable")
}

type labelOnlyProvider struct{}

func (labelOnlyProvider) Name() string      { return "label_only" }
func (labelOnlyProvider) MediaKind() string { return "text" }
func (labelOnlyProvider) Generate(_ context.Context, input GenerationInput) (CandidateDraft, error) {
	components := make([]Component, 0, len(input.Request.ComponentTypes))
	for _, componentType := range input.Request.ComponentTypes {
		components = append(components, Component{Key: componentType, Type: componentType, Title: componentType, Content: "正文完全相同"})
	}
	return CandidateDraft{Components: components, Content: map[string]any{
		"components": components, "difference_direction": input.DifferenceDirection,
	}}, nil
}

func TestRealProviderFailureNeverFallsBackToMock(t *testing.T) {
	request := mockRequest()
	request.GeneratorProvider, request.GeneratorModel = "real_fail", "real-model"
	registry := NewRegistry([]CandidateProvider{alwaysFailProvider{}, NewDeterministicMockProvider()}, []CandidateReviewer{NewDeterministicMockReviewer()})
	candidates, err := registry.GenerateAndReview(context.Background(), request)
	if len(candidates) != 0 || !errors.Is(err, ErrProviderFailed) {
		t.Fatalf("provider failure must be returned without fabricated candidates: candidates=%d err=%v", len(candidates), err)
	}
}

func TestChangingOnlyDirectionMetadataIsRejected(t *testing.T) {
	request := mockRequest()
	request.GeneratorProvider, request.GeneratorModel = "label_only", "real-model"
	registry := NewRegistry([]CandidateProvider{labelOnlyProvider{}}, []CandidateReviewer{NewDeterministicMockReviewer()})
	candidates, err := registry.GenerateAndReview(context.Background(), request)
	if len(candidates) != 0 || !errors.Is(err, ErrInvalidProviderData) {
		t.Fatalf("label-only differences must reject the whole set: candidates=%d err=%v", len(candidates), err)
	}
}
