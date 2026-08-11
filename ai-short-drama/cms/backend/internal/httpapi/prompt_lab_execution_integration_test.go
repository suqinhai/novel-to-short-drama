package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"short-drama-cms/backend/internal/config"
	"short-drama-cms/backend/internal/store"
)

func TestPromptLabServerExecutionAndFailurePersistenceIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE31_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE31_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := store.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		model, _ := payload["model"].(string)
		messages, _ := payload["messages"].([]any)
		if len(messages) != 2 {
			t.Fatalf("provider did not receive rendered system/user inputs: %#v", payload)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"choices": []any{map[string]any{
			"message": map[string]any{"content": fmt.Sprintf(`{"model":%q,"result":"真实结果"}`, model)},
		}}, "usage": map[string]any{"total_tokens": 9}})
	}))
	defer provider.Close()
	t.Setenv("PROMPT_LAB_PROVIDER_LOCAL_TEST_BASE_URL", provider.URL)
	failureProvider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "provider unavailable", http.StatusBadGateway)
	}))
	defer failureProvider.Close()
	t.Setenv("PROMPT_LAB_PROVIDER_FAILURE_TEST_BASE_URL", failureProvider.URL)

	suffix := fmt.Sprint(time.Now().UnixNano())
	template, err := database.CreatePromptTemplate(ctx, store.CreatePromptTemplateInput{
		Category: "script", PromptKey: "acceptance.prompt." + suffix, DisplayName: "验收 Prompt", CreatedBy: "acceptance",
	})
	if err != nil {
		t.Fatal(err)
	}
	version, err := database.CreatePromptVersion(ctx, template.PromptTemplateID, store.CreatePromptVersionInput{
		SystemTemplate: "系统 {{name}}", UserTemplate: "输出 {{name}}", ChangeNote: "真实执行验收", CreatedBy: "acceptance",
		VariableSchema:   json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`),
		DefaultVariables: json.RawMessage(`{}`), ModelDefaults: json.RawMessage(`{"temperature":0.1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := database.CreatePromptFixture(ctx, store.CreatePromptFixtureInput{Category: "script",
		FixtureKey: "acceptance.fixture." + suffix, DisplayName: "中文换行 fixture",
		Variables: json.RawMessage(`{"name":"林夏\n第二行"}`), ExpectedOutput: json.RawMessage(`{"result":"真实结果"}`), CreatedBy: "acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	suite, err := database.CreatePromptTestSuite(ctx, store.CreatePromptTestSuiteInput{Category: "script",
		DisplayName: "验收测试集 " + suffix, FixtureIDs: []string{fixture.PromptFixtureID}, MetricConfig: json.RawMessage(`{}`), CreatedBy: "acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	createExperiment := func(providerName string) store.PromptExperiment {
		experiment, createErr := database.CreatePromptExperiment(ctx, store.CreatePromptExperimentInput{Category: "script",
			DisplayName: "服务端执行 " + providerName + suffix, PromptTestSuiteID: suite.PromptTestSuiteID, BlindReview: true,
			Variants: []store.PromptExperimentVariantInput{
				{PromptVersionID: version.PromptVersionID, Provider: providerName, Model: "model-a", Parameters: json.RawMessage(`{"temperature":0.2}`)},
				{PromptVersionID: version.PromptVersionID, Provider: providerName, Model: "model-b", Parameters: json.RawMessage(`{"temperature":0.3}`)},
			}, CreatedBy: "acceptance"})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return experiment
	}

	router := New(database, config.Config{}).Router()
	forgedGate := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/p_phase1_legacy/episodes/ep_phase1_legacy_001/quality-gates/rule-runs",
		bytes.NewBufferString(`{"master_id":"master_phase5_v1","snapshot":{"artifacts":[{"artifact_id":"forged"}]}}`))
	forgedGate.Header.Set("Content-Type", "application/json")
	forgedGateResponse := httptest.NewRecorder()
	router.ServeHTTP(forgedGateResponse, forgedGate)
	if forgedGateResponse.Code != http.StatusBadRequest || !strings.Contains(forgedGateResponse.Body.String(), "unknown field") {
		t.Fatalf("client-supplied QA snapshot was not rejected: %d %s", forgedGateResponse.Code, forgedGateResponse.Body.String())
	}

	run := func(experiment store.PromptExperiment) store.PromptExperiment {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/prompt-lab/experiments/"+experiment.PromptExperimentID+"/run", nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusCreated {
			t.Fatalf("run returned %d: %s", response.Code, response.Body.String())
		}
		result, getErr := database.GetPromptExperiment(ctx, experiment.PromptExperimentID, false)
		if getErr != nil {
			t.Fatal(getErr)
		}
		return result
	}

	succeeded := run(createExperiment("local-test"))
	if len(succeeded.Results) != 2 {
		t.Fatalf("expected complete 2x1 matrix, got %#v", succeeded.Results)
	}
	for _, result := range succeeded.Results {
		if result.Status != "completed" || result.ErrorMessage != nil || result.LatencyMS == nil ||
			!strings.Contains(result.RenderedInput, "林夏\n第二行") || !strings.Contains(string(result.TokenUsage), "total_tokens") {
			t.Fatalf("real execution evidence incomplete: %#v", result)
		}
	}
	manual := httptest.NewRequest(http.MethodPost, "/api/v1/prompt-lab/experiments/"+succeeded.PromptExperimentID+"/results",
		bytes.NewBufferString(`{"output":{"fake":true}}`))
	manual.Header.Set("Content-Type", "application/json")
	manualResponse := httptest.NewRecorder()
	router.ServeHTTP(manualResponse, manual)
	if manualResponse.Code != http.StatusGone {
		t.Fatalf("manual fake-result endpoint was not disabled: %d %s", manualResponse.Code, manualResponse.Body.String())
	}

	failed := run(createExperiment("failure-test"))
	for _, result := range failed.Results {
		if result.Status != "failed" || result.ErrorMessage == nil || !strings.Contains(*result.ErrorMessage, "HTTP 502") || string(result.Output) != `{}` {
			t.Fatalf("provider failure became fake success: %#v", result)
		}
	}
}
