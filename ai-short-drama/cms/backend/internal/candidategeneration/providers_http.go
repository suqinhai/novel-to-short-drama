package candidategeneration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type httpProviderConfig struct {
	name, kind, endpoint, apiKey, defaultModel string
	client                                     *http.Client
}

type httpCandidateProvider struct{ config httpProviderConfig }

func (p *httpCandidateProvider) Name() string      { return p.config.name }
func (p *httpCandidateProvider) MediaKind() string { return p.config.kind }

func (p *httpCandidateProvider) Generate(ctx context.Context, input GenerationInput) (CandidateDraft, error) {
	if strings.TrimSpace(p.config.endpoint) == "" {
		return CandidateDraft{}, fmt.Errorf("endpoint is not configured")
	}
	model := input.Request.GeneratorModel
	if model == "" {
		model = p.config.defaultModel
	}
	if model == "" {
		return CandidateDraft{}, fmt.Errorf("model is not configured")
	}
	prompt := generationPrompt(input)
	if p.config.kind == "text" {
		payload := map[string]any{"model": model, "seed": input.Seed, "messages": []map[string]string{
			{"role": "system", "content": "你是短剧候选生成器。只输出 JSON，不得评价自己的结果。"},
			{"role": "user", "content": prompt},
		}, "response_format": map[string]string{"type": "json_object"}}
		copyGenerationParameters(payload, input.Request.GenerationParameters)
		var response struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := p.post(ctx, p.config.endpoint, payload, &response); err != nil {
			return CandidateDraft{}, err
		}
		if len(response.Choices) == 0 {
			return CandidateDraft{}, fmt.Errorf("text response has no choices")
		}
		var draft CandidateDraft
		if err := decodeJSONObject(response.Choices[0].Message.Content, &draft); err != nil {
			return CandidateDraft{}, fmt.Errorf("decode text candidate: %w", err)
		}
		if draft.Content == nil {
			draft.Content = map[string]any{}
		}
		draft.Content["components"] = draft.Components
		draft.Content["difference_direction"] = input.DifferenceDirection
		draft.Content["frozen_input_hash"] = input.Request.FrozenInput.FrozenHash
		return draft, nil
	}
	payload := map[string]any{"model": model, "prompt": prompt, "seed": input.Seed, "target_type": input.Request.TargetType,
		"target_id": input.Request.TargetID, "difference_direction": input.DifferenceDirection}
	copyGenerationParameters(payload, input.Request.GenerationParameters)
	var response map[string]any
	if err := p.post(ctx, p.config.endpoint, payload, &response); err != nil {
		return CandidateDraft{}, err
	}
	mediaURL := firstMediaURL(response)
	if mediaURL == "" {
		return CandidateDraft{}, fmt.Errorf("media response has no output URL")
	}
	componentType := "key_image"
	if p.config.kind == "video" {
		componentType = "video_shot"
	}
	component := Component{Key: componentType, Type: componentType, Title: map[string]string{"image": "关键图片", "video": "视频镜头"}[p.config.kind], Content: prompt}
	content := map[string]any{"schema_version": "candidate-content.v2", "target_type": input.Request.TargetType,
		"target_id": input.Request.TargetID, "difference_direction": input.DifferenceDirection,
		"difference_manifest": map[string]any{"direction": input.DifferenceDirection, "provider_output": true},
		"components":          []Component{component}, "media": map[string]any{"kind": p.config.kind, "preview_url": mediaURL, "provider_uri": mediaURL},
		"frozen_input_hash": input.Request.FrozenInput.FrozenHash}
	return CandidateDraft{Components: []Component{component}, Content: content}, nil
}

func (p *httpCandidateProvider) post(ctx context.Context, endpoint string, payload any, destination any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if p.config.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+p.config.apiKey)
	}
	response, err := p.config.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", response.StatusCode, truncate(string(responseBody), 500))
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return fmt.Errorf("decode provider response: %w", err)
	}
	return nil
}

type httpCandidateReviewer struct{ provider *httpCandidateProvider }

func (r *httpCandidateReviewer) Name() string { return r.provider.Name() }
func (r *httpCandidateReviewer) Review(ctx context.Context, input ReviewInput) (Score, error) {
	requestCopy := input.Request
	if input.HideGenerator {
		requestCopy.GeneratorProvider, requestCopy.GeneratorModel, requestCopy.Model = "", "", ""
	}
	promptPayload := map[string]any{"frozen_input": requestCopy.FrozenInput, "target_type": requestCopy.TargetType,
		"target_id": requestCopy.TargetID, "difference_direction": input.DifferenceDirection, "candidate": input.Candidate}
	prompt := "独立评审以下短剧候选。逐项给出 0-100 分、可定位证据与扣分位置；九个 dimension 必须是 fidelity、causality、character_consistency、hook、pacing、filmability、continuity、estimated_duration、modification_risk。estimated_duration_seconds 必须大于 0。每项 evidence 和 deductions.location 必须含 source_kind、source_id、path、reason。只输出与 Score schema 对应的 JSON。\n" + contentJSON(promptPayload)
	payload := map[string]any{"model": input.Request.ReviewerModel, "messages": []map[string]string{
		{"role": "system", "content": "你是独立短剧评审，不参与候选生成。所有判断必须引用冻结输入或候选 JSON 路径。"},
		{"role": "user", "content": prompt},
	}, "response_format": map[string]string{"type": "json_object"}, "temperature": 0}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := r.provider.post(ctx, r.provider.config.endpoint, payload, &response); err != nil {
		return Score{}, err
	}
	if len(response.Choices) == 0 {
		return Score{}, fmt.Errorf("review response has no choices")
	}
	var score Score
	if err := decodeJSONObject(response.Choices[0].Message.Content, &score); err != nil {
		return Score{}, fmt.Errorf("decode review: %w", err)
	}
	return score, nil
}

func NewRegistryFromEnvironment() *Registry {
	timeoutSeconds, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("MODEL_REQUEST_TIMEOUT_SECONDS")))
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	client := &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	textEndpoint := completionEndpoint(envFirst("CANDIDATE_TEXT_API_BASE_URL", "LITELLM_BASE_URL"))
	textKey := envFirst("CANDIDATE_TEXT_API_KEY", "LITELLM_API_KEY")
	imageEndpoint := generationEndpoint(envFirst("CANDIDATE_IMAGE_API_BASE_URL", "IMAGE_API_BASE_URL"), "/v1/images/generations")
	videoEndpoint := generationEndpoint(envFirst("CANDIDATE_VIDEO_API_BASE_URL", "VIDEO_API_BASE_URL"), "/v1/videos/generations")
	textProvider := &httpCandidateProvider{config: httpProviderConfig{name: "text_http", kind: "text", endpoint: textEndpoint,
		apiKey: textKey, defaultModel: envFirst("CANDIDATE_TEXT_MODEL", "SCRIPT_WRITING_MODEL"), client: client}}
	imageProvider := &httpCandidateProvider{config: httpProviderConfig{name: "image_http", kind: "image", endpoint: imageEndpoint,
		apiKey: envFirst("CANDIDATE_IMAGE_API_KEY", "IMAGE_API_KEY"), defaultModel: envFirst("CANDIDATE_IMAGE_MODEL", "IMAGE_MODEL"), client: client}}
	videoProvider := &httpCandidateProvider{config: httpProviderConfig{name: "video_http", kind: "video", endpoint: videoEndpoint,
		apiKey: envFirst("CANDIDATE_VIDEO_API_KEY", "VIDEO_API_KEY"), defaultModel: envFirst("CANDIDATE_VIDEO_MODEL", "VIDEO_MODEL"), client: client}}
	// Review credentials, endpoint and model are intentionally not inherited
	// from the generation gateway. Production must configure an independent
	// reviewer explicitly; an omitted reviewer fails closed.
	reviewerProvider := &httpCandidateProvider{config: httpProviderConfig{name: "reviewer_http", kind: "text", endpoint: completionEndpoint(os.Getenv("CANDIDATE_REVIEW_API_BASE_URL")),
		apiKey: strings.TrimSpace(os.Getenv("CANDIDATE_REVIEW_API_KEY")), defaultModel: strings.TrimSpace(os.Getenv("CANDIDATE_REVIEW_MODEL")), client: client}}
	providers := []CandidateProvider{textProvider, imageProvider, videoProvider}
	reviewers := []CandidateReviewer{&httpCandidateReviewer{provider: reviewerProvider}}
	// The deterministic provider is a test fixture, never a production fallback.
	// Tests that need it must opt in explicitly in their environment.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CANDIDATE_ENABLE_DETERMINISTIC_MOCK")), "true") {
		providers = append(providers, NewDeterministicMockProvider())
		reviewers = append(reviewers, NewDeterministicMockReviewer())
	}
	return NewRegistry(providers, reviewers)
}

func generationPrompt(input GenerationInput) string {
	payload := map[string]any{"target_type": input.Request.TargetType, "target_id": input.Request.TargetID,
		"component_types": input.Request.ComponentTypes, "difference_direction": input.DifferenceDirection,
		"must_preserve": input.Request.MustPreserve, "allowed_changes": input.Request.AllowedChanges,
		"seed": input.Seed, "frozen_input": input.Request.FrozenInput}
	if strings.TrimSpace(input.Request.ProductionPrompt) != "" {
		return strings.TrimSpace(input.Request.ProductionPrompt) +
			"\n\nRuntime frozen inputs (authoritative; do not invent replacements):\n" + contentJSON(payload)
	}
	return "基于且仅基于 frozen_input 生成一个短剧候选。difference_direction 必须改变正文的结构、节奏或视听执行，不得只写标签。返回 JSON：{components:[{key,type,title,content}],content:{...}}。\n" + contentJSON(payload)
}

func copyGenerationParameters(payload map[string]any, raw json.RawMessage) {
	var parameters map[string]any
	if json.Unmarshal(raw, &parameters) != nil {
		return
	}
	for key, value := range parameters {
		switch key {
		case "temperature", "top_p", "max_tokens", "size", "duration", "aspect_ratio", "resolution":
			payload[key] = value
		}
	}
}

func decodeJSONObject(content string, destination any) error {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return json.Unmarshal([]byte(strings.TrimSpace(content)), destination)
}

func firstMediaURL(response map[string]any) string {
	for _, key := range []string{"url", "uri", "output_url", "video_url", "image_url"} {
		if value, ok := response[key].(string); ok && value != "" {
			return value
		}
	}
	for _, key := range []string{"data", "output", "videos", "images"} {
		items, ok := response[key].([]any)
		if !ok || len(items) == 0 {
			continue
		}
		if item, ok := items[0].(map[string]any); ok {
			if value := firstMediaURL(item); value != "" {
				return value
			}
		}
	}
	return ""
}

func completionEndpoint(base string) string { return generationEndpoint(base, "/v1/chat/completions") }
func generationEndpoint(base, suffix string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return base
	}
	if strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), strings.TrimRight(suffix, "/")) {
		return strings.TrimRight(base, "/")
	}
	return strings.TrimRight(base, "/") + suffix
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
