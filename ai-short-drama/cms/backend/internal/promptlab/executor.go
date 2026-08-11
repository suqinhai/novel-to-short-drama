package promptlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ExecutionRequest struct {
	Provider   string
	Model      string
	Parameters json.RawMessage
	Seed       *int64
	System     string
	User       string
}

type ExecutionResult struct {
	Output        json.RawMessage
	TokenUsage    json.RawMessage
	LatencyMS     int
	EstimatedCost float64
}

var providerEnvPattern = regexp.MustCompile(`[^A-Z0-9]+`)

// Execute invokes the configured provider. It has no mock or synthetic success
// fallback: unavailable providers and malformed responses are returned as real
// failures so an experiment cannot manufacture a completed result.
func Execute(ctx context.Context, input ExecutionRequest) (ExecutionResult, error) {
	provider := strings.TrimSpace(input.Provider)
	model := strings.TrimSpace(input.Model)
	if provider == "" || model == "" {
		return ExecutionResult{}, fmt.Errorf("provider and model are required")
	}
	endpoint, apiKey := providerConfiguration(provider)
	if endpoint == "" {
		return ExecutionResult{}, fmt.Errorf("provider %q endpoint is not configured", provider)
	}
	parameters := map[string]any{}
	if len(input.Parameters) > 0 && json.Unmarshal(input.Parameters, &parameters) != nil {
		return ExecutionResult{}, fmt.Errorf("provider parameters must be a JSON object")
	}
	payload := map[string]any{"model": model, "messages": []map[string]string{
		{"role": "system", "content": input.System},
		{"role": "user", "content": input.User},
	}}
	for key, value := range parameters {
		switch key {
		case "model", "messages":
			continue
		default:
			payload[key] = value
		}
	}
	if input.Seed != nil {
		payload["seed"] = *input.Seed
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ExecutionResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ExecutionResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	timeoutSeconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("PROMPT_LAB_REQUEST_TIMEOUT_SECONDS")))
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	started := time.Now()
	response, err := (&http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}).Do(request)
	latency := int(time.Since(started).Milliseconds())
	if err != nil {
		return ExecutionResult{LatencyMS: latency}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return ExecutionResult{LatencyMS: latency}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ExecutionResult{LatencyMS: latency}, fmt.Errorf("provider HTTP %d: %s", response.StatusCode, truncateResponse(responseBody, 500))
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if err = json.Unmarshal(responseBody, &decoded); err != nil {
		return ExecutionResult{LatencyMS: latency}, fmt.Errorf("decode provider response: %w", err)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return ExecutionResult{LatencyMS: latency}, fmt.Errorf("provider response has no output")
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(content, "```json"), "```"), "```"))
	output := json.RawMessage(content)
	if !json.Valid(output) {
		output, _ = json.Marshal(content)
	}
	usage := decoded.Usage
	if len(usage) == 0 || !json.Valid(usage) {
		usage = json.RawMessage(`{}`)
	}
	return ExecutionResult{Output: output, TokenUsage: usage, LatencyMS: latency}, nil
}

func providerConfiguration(provider string) (string, string) {
	key := providerEnvPattern.ReplaceAllString(strings.ToUpper(provider), "_")
	base := strings.TrimSpace(os.Getenv("PROMPT_LAB_PROVIDER_" + key + "_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("PROMPT_LAB_PROVIDER_" + key + "_API_KEY"))
	if strings.EqualFold(provider, "litellm") {
		if base == "" {
			base = strings.TrimSpace(os.Getenv("LITELLM_BASE_URL"))
		}
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("LITELLM_API_KEY"))
		}
	}
	return completionURL(base), apiKey
}

func completionURL(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(base, "/")
	}
	if strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1/chat/completions") {
		return strings.TrimRight(base, "/")
	}
	return strings.TrimRight(base, "/") + "/v1/chat/completions"
}

func truncateResponse(raw []byte, limit int) string {
	value := strings.TrimSpace(string(raw))
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
