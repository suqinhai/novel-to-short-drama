package promptlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecuteCallsConfiguredProviderAndPreservesUsage(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":\"真实输出\"}"}}],"usage":{"total_tokens":12}}`))
	}))
	defer server.Close()
	t.Setenv("PROMPT_LAB_PROVIDER_LOCAL_TEST_BASE_URL", server.URL)
	seed := int64(17)
	result, err := Execute(context.Background(), ExecutionRequest{Provider: "local-test", Model: "model-a",
		Parameters: json.RawMessage(`{"temperature":0.2}`), Seed: &seed, System: "系统", User: "用户"})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Output) != `{"answer":"真实输出"}` || !strings.Contains(string(result.TokenUsage), `"total_tokens":12`) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if received["model"] != "model-a" || received["seed"].(float64) != 17 || received["temperature"].(float64) != 0.2 {
		t.Fatalf("request settings were not sent: %#v", received)
	}
}

func TestExecuteReturnsProviderFailureWithoutSyntheticOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()
	t.Setenv("PROMPT_LAB_PROVIDER_FAILURE_TEST_BASE_URL", server.URL)
	result, err := Execute(context.Background(), ExecutionRequest{Provider: "failure-test", Model: "model-a"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("expected real provider failure, got result=%+v err=%v", result, err)
	}
	if len(result.Output) != 0 {
		t.Fatalf("failure must not fabricate output: %s", result.Output)
	}
}
