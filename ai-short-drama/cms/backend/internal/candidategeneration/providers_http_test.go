package candidategeneration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPProvidersAreIndependentBlindAndAudited(t *testing.T) {
	request := mockRequest()
	request.CandidateCount = 2
	request.GeneratorProvider, request.GeneratorModel = "text_http", "generator-model"
	request.ReviewerProvider, request.ReviewerModel = "reviewer_http", "reviewer-model"
	request.BlindReview = true
	var generationOrdinal atomic.Int32
	var reviewerSawGenerator atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, incoming *http.Request) {
		body := make(map[string]any)
		_ = json.NewDecoder(incoming.Body).Decode(&body)
		if strings.Contains(incoming.URL.Path, "review") {
			raw, _ := json.Marshal(body)
			if strings.Contains(string(raw), request.GeneratorProvider) || strings.Contains(string(raw), request.GeneratorModel) {
				reviewerSawGenerator.Store(true)
			}
			draft, _ := NewDeterministicMockProvider().Generate(context.Background(), GenerationInput{
				Request: request, Ordinal: 1, DifferenceDirection: request.DifferenceDirections[0], Seed: request.RandomSeed,
			})
			score, _ := NewDeterministicMockReviewer().Review(context.Background(), ReviewInput{
				Request: request, Ordinal: 1, DifferenceDirection: request.DifferenceDirections[0], Candidate: draft,
			})
			writeHTTPChoice(writer, score)
			return
		}
		ordinal := int(generationOrdinal.Add(1))
		components := make([]Component, 0, len(request.ComponentTypes))
		for _, componentType := range request.ComponentTypes {
			components = append(components, Component{Key: componentType, Type: componentType,
				Title: componentType, Content: fmt.Sprintf("provider body %d %s", ordinal, componentType)})
		}
		writeHTTPChoice(writer, CandidateDraft{Components: components, Content: map[string]any{
			"schema_version": "candidate-content.v2", "variant": ordinal,
		}})
	}))
	defer server.Close()
	client := server.Client()
	provider := &httpCandidateProvider{config: httpProviderConfig{name: "text_http", kind: "text",
		endpoint: server.URL + "/generate", client: client}}
	reviewerHTTP := &httpCandidateProvider{config: httpProviderConfig{name: "reviewer_http", kind: "text",
		endpoint: server.URL + "/review", client: client}}
	registry := NewRegistry([]CandidateProvider{provider}, []CandidateReviewer{
		&httpCandidateReviewer{provider: reviewerHTTP},
	})
	candidates, executions, err := registry.GenerateAndReviewAudited(context.Background(), request)
	if err != nil || len(candidates) != 2 || len(executions) != 4 {
		t.Fatalf("independent provider run failed: candidates=%d executions=%d err=%v", len(candidates), len(executions), err)
	}
	if reviewerSawGenerator.Load() {
		t.Fatal("blind reviewer received generator provider/model")
	}
	for _, execution := range executions {
		if execution.Status != "succeeded" || execution.StartedAt.IsZero() || execution.CompletedAt.IsZero() {
			t.Fatalf("execution audit is incomplete: %+v", execution)
		}
		if execution.ExecutionType == "evaluation" && (!execution.Blind || execution.Provider != "reviewer_http") {
			t.Fatalf("evaluation was not independent and blind: %+v", execution)
		}
	}
}

func TestEnvironmentRegistryOnlyEnablesDeterministicMockExplicitly(t *testing.T) {
	t.Setenv("CANDIDATE_ENABLE_DETERMINISTIC_MOCK", "false")
	registry := NewRegistryFromEnvironment()
	if registry.providers["deterministic_mock"] != nil || registry.reviewers["deterministic_mock"] != nil {
		t.Fatal("production registry registered deterministic mock")
	}
	t.Setenv("CANDIDATE_ENABLE_DETERMINISTIC_MOCK", "true")
	registry = NewRegistryFromEnvironment()
	if registry.providers["deterministic_mock"] == nil || registry.reviewers["deterministic_mock"] == nil {
		t.Fatal("explicit test environment did not register deterministic mock")
	}
}

func TestHTTPProviderInvalidFailureTimeoutAndReviewerFailure(t *testing.T) {
	cases := []struct {
		name       string
		generation http.HandlerFunc
		review     http.HandlerFunc
		wantStatus string
		wantType   string
	}{
		{name: "invalid generator output", wantStatus: "invalid", wantType: "generation",
			generation: func(w http.ResponseWriter, _ *http.Request) { writeHTTPChoice(w, map[string]any{}) }},
		{name: "generator failure", wantStatus: "failed", wantType: "generation",
			generation: func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "provider failed", http.StatusBadGateway) }},
		{name: "generator timeout", wantStatus: "failed", wantType: "generation",
			generation: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(50 * time.Millisecond)
				writeHTTPChoice(w, map[string]any{})
			}},
		{name: "reviewer failure", wantStatus: "failed", wantType: "evaluation",
			generation: validHTTPGeneration, review: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "review failed", http.StatusServiceUnavailable)
			}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "review") {
					if test.review != nil {
						test.review(w, r)
						return
					}
					http.Error(w, "review must not run", http.StatusInternalServerError)
					return
				}
				test.generation(w, r)
			}))
			defer server.Close()
			request := mockRequest()
			request.CandidateCount = 2
			request.GeneratorProvider, request.GeneratorModel = "text_http", "generator-model"
			request.ReviewerProvider, request.ReviewerModel = "reviewer_http", "reviewer-model"
			client := server.Client()
			if strings.Contains(test.name, "timeout") {
				client.Timeout = 10 * time.Millisecond
			}
			provider := &httpCandidateProvider{config: httpProviderConfig{name: "text_http", kind: "text", endpoint: server.URL + "/generate", client: client}}
			reviewerProvider := &httpCandidateProvider{config: httpProviderConfig{name: "reviewer_http", kind: "text", endpoint: server.URL + "/review", client: client}}
			registry := NewRegistry([]CandidateProvider{provider}, []CandidateReviewer{&httpCandidateReviewer{provider: reviewerProvider}})
			candidates, executions, err := registry.GenerateAndReviewAudited(context.Background(), request)
			if len(candidates) != 0 || err == nil || (!errors.Is(err, ErrProviderFailed) && !errors.Is(err, ErrInvalidProviderData)) {
				t.Fatalf("failure produced success data: candidates=%d executions=%+v err=%v", len(candidates), executions, err)
			}
			last := executions[len(executions)-1]
			if last.Status != test.wantStatus || last.ExecutionType != test.wantType || last.FailureReason == "" {
				t.Fatalf("failure audit mismatch: %+v", last)
			}
		})
	}
}

func validHTTPGeneration(w http.ResponseWriter, _ *http.Request) {
	request := mockRequest()
	components := make([]Component, 0, len(request.ComponentTypes))
	for _, componentType := range request.ComponentTypes {
		components = append(components, Component{Key: componentType, Type: componentType,
			Title: componentType, Content: "valid " + componentType})
	}
	writeHTTPChoice(w, CandidateDraft{Components: components, Content: map[string]any{"variant": "valid"}})
}

func writeHTTPChoice(w http.ResponseWriter, value any) {
	content, _ := json.Marshal(value)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
		"message": map[string]any{"content": string(content)},
	}}})
}
