package scripteditor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidRequest = errors.New("invalid script AI request")
	ErrUnavailable    = errors.New("script AI provider unavailable")
)

var Operations = map[string]bool{
	"compress_dialogue":      true,
	"colloquialize":          true,
	"strengthen_conflict":    true,
	"strengthen_hook":        true,
	"convert":                true,
	"rewrite_preserve_facts": true,
}

var BlockTypes = map[string]bool{
	"action": true, "dialogue": true, "narration": true,
	"inner_monologue": true, "off_screen": true,
}

type Block struct {
	BlockID                string `json:"block_id"`
	BlockType              string `json:"block_type"`
	Text                   string `json:"text"`
	SpeakerName            string `json:"speaker_name,omitempty"`
	Emotion                string `json:"emotion,omitempty"`
	PerformanceInstruction string `json:"performance_instruction,omitempty"`
}

type Request struct {
	Operation    string          `json:"operation"`
	ConvertTo    string          `json:"convert_to,omitempty"`
	Instruction  string          `json:"instruction,omitempty"`
	Blocks       []Block         `json:"blocks"`
	Context      json.RawMessage `json:"context,omitempty"`
	MustPreserve []string        `json:"must_preserve"`
}

type Result struct {
	Blocks                   []Block          `json:"blocks"`
	Reason                   string           `json:"reason"`
	SourceEvidence           []SourceEvidence `json:"source_evidence"`
	EstimatedDurationDeltaMS int              `json:"estimated_duration_delta_ms"`
}

type SourceEvidence struct {
	SourceSpanID    string `json:"source_span_id,omitempty"`
	EventRevisionID string `json:"event_revision_id,omitempty"`
	Explanation     string `json:"explanation"`
}

type Rewriter interface {
	Rewrite(context.Context, Request) (Result, error)
}

type HTTPRewriter struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

func NewFromEnvironment() Rewriter {
	timeoutSeconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("MODEL_REQUEST_TIMEOUT_SECONDS")))
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	return &HTTPRewriter{
		endpoint: completionEndpoint(os.Getenv("LITELLM_BASE_URL")),
		apiKey:   strings.TrimSpace(os.Getenv("LITELLM_API_KEY")),
		model:    strings.TrimSpace(os.Getenv("SCRIPT_WRITING_MODEL")),
		client:   &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
	}
}

func (r *HTTPRewriter) Rewrite(ctx context.Context, input Request) (Result, error) {
	if err := ValidateRequest(input); err != nil {
		return Result{}, err
	}
	if r == nil || r.endpoint == "" || r.model == "" {
		return Result{}, fmt.Errorf("%w: configure LITELLM_BASE_URL and SCRIPT_WRITING_MODEL", ErrUnavailable)
	}
	inputJSON, _ := json.Marshal(input)
	system := "你是结构化短剧剧本编辑器。只改写 blocks，不得新增、删除或改动 block_id；不得改写未提供内容。必须保持给定剧情事实、人物关系、时空和 must_preserve。只输出 JSON 对象 {\"blocks\":[...],\"reason\":\"...\",\"source_evidence\":[{\"source_span_id\":\"...\",\"event_revision_id\":\"...\",\"explanation\":\"...\"}],\"estimated_duration_delta_ms\":0}。证据 ID 必须来自 context。"
	user := "执行操作 " + input.Operation + "。"
	if input.ConvertTo != "" {
		user += " 所有块转换为 " + input.ConvertTo + "。"
	}
	if strings.TrimSpace(input.Instruction) != "" {
		user += " 补充要求：" + strings.TrimSpace(input.Instruction) + "。"
	}
	user += " 输入：" + string(inputJSON)
	payload := map[string]any{
		"model":           r.model,
		"messages":        []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0.2,
	}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if r.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+r.apiKey)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return Result{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Result{}, fmt.Errorf("script AI HTTP %d: %s", response.StatusCode, truncate(string(responseBody), 500))
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.Unmarshal(responseBody, &completion); err != nil || len(completion.Choices) == 0 {
		return Result{}, fmt.Errorf("decode script AI response: %w", err)
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var result Result
	if err = json.Unmarshal([]byte(strings.TrimSpace(content)), &result); err != nil {
		return Result{}, fmt.Errorf("decode structured script rewrite: %w", err)
	}
	if err = ValidateResult(input, result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func ValidateRequest(input Request) error {
	if !Operations[input.Operation] || len(input.Blocks) == 0 || len(input.Blocks) > 200 {
		return fmt.Errorf("%w: unsupported operation or empty selection", ErrInvalidRequest)
	}
	if input.Operation == "convert" && !BlockTypes[input.ConvertTo] {
		return fmt.Errorf("%w: convert_to is required", ErrInvalidRequest)
	}
	seen := map[string]bool{}
	for _, block := range input.Blocks {
		if strings.TrimSpace(block.BlockID) == "" || !BlockTypes[block.BlockType] ||
			strings.TrimSpace(block.Text) == "" || seen[block.BlockID] {
			return fmt.Errorf("%w: blocks require unique ids, valid types and text", ErrInvalidRequest)
		}
		seen[block.BlockID] = true
	}
	return nil
}

func ValidateResult(input Request, result Result) error {
	if len(result.Blocks) != len(input.Blocks) {
		return fmt.Errorf("%w: AI changed selection cardinality", ErrInvalidRequest)
	}
	expected := map[string]Block{}
	for _, block := range input.Blocks {
		expected[block.BlockID] = block
	}
	seen := map[string]bool{}
	for _, block := range result.Blocks {
		original, exists := expected[block.BlockID]
		if !exists || seen[block.BlockID] || strings.TrimSpace(block.Text) == "" || !BlockTypes[block.BlockType] {
			return fmt.Errorf("%w: AI returned an invalid block", ErrInvalidRequest)
		}
		if input.Operation == "convert" {
			if block.BlockType != input.ConvertTo {
				return fmt.Errorf("%w: AI ignored convert_to", ErrInvalidRequest)
			}
		} else if block.BlockType != original.BlockType {
			return fmt.Errorf("%w: non-conversion operation changed block type", ErrInvalidRequest)
		}
		seen[block.BlockID] = true
	}
	if strings.TrimSpace(result.Reason) == "" {
		return fmt.Errorf("%w: AI rewrite reason is required", ErrInvalidRequest)
	}
	if len(result.SourceEvidence) == 0 {
		return fmt.Errorf("%w: AI rewrite source evidence is required", ErrInvalidRequest)
	}
	knownSpanIDs, knownEventIDs, contextSupplied, err := evidenceIDsFromContext(input.Context)
	if err != nil {
		return fmt.Errorf("%w: invalid rewrite context: %v", ErrInvalidRequest, err)
	}
	for _, evidence := range result.SourceEvidence {
		spanID := strings.TrimSpace(evidence.SourceSpanID)
		eventID := strings.TrimSpace(evidence.EventRevisionID)
		if strings.TrimSpace(evidence.Explanation) == "" || (spanID == "" && eventID == "") {
			return fmt.Errorf("%w: AI rewrite evidence requires an id and explanation", ErrInvalidRequest)
		}
		if contextSupplied && ((spanID != "" && !knownSpanIDs[spanID]) ||
			(eventID != "" && !knownEventIDs[eventID])) {
			return fmt.Errorf("%w: AI rewrite evidence is not present in the supplied context", ErrInvalidRequest)
		}
	}
	return nil
}

func evidenceIDsFromContext(raw json.RawMessage) (map[string]bool, map[string]bool, bool, error) {
	spanIDs := map[string]bool{}
	eventIDs := map[string]bool{}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return spanIDs, eventIDs, false, nil
	}
	var reference struct {
		Events []struct {
			EventRevisionID string   `json:"event_revision_id"`
			SourceSpanIDs   []string `json:"source_span_ids"`
		} `json:"events"`
		SourceSpans []struct {
			SourceSpanID string `json:"source_span_id"`
		} `json:"source_spans"`
	}
	if err := json.Unmarshal(trimmed, &reference); err != nil {
		return nil, nil, true, err
	}
	for _, event := range reference.Events {
		if id := strings.TrimSpace(event.EventRevisionID); id != "" {
			eventIDs[id] = true
		}
		for _, sourceSpanID := range event.SourceSpanIDs {
			if id := strings.TrimSpace(sourceSpanID); id != "" {
				spanIDs[id] = true
			}
		}
	}
	for _, sourceSpan := range reference.SourceSpans {
		if id := strings.TrimSpace(sourceSpan.SourceSpanID); id != "" {
			spanIDs[id] = true
		}
	}
	return spanIDs, eventIDs, true, nil
}

func completionEndpoint(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return base
	}
	if strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/v1/chat/completions") {
		return strings.TrimRight(base, "/")
	}
	return strings.TrimRight(base, "/") + "/v1/chat/completions"
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
