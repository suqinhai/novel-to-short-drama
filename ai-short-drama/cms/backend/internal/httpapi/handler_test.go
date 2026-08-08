package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/config"
	"short-drama-cms/backend/internal/datacleanup"
)

type fakeDataCleaner struct {
	resetCalls int
}

func (f *fakeDataCleaner) Preview(context.Context) (datacleanup.Preview, error) {
	return datacleanup.Preview{
		ConfirmationPhrase: datacleanup.ConfirmationPhrase,
		AIConfigFileExists: true,
		Destructive:        true,
	}, nil
}

func (f *fakeDataCleaner) Reset(context.Context) (datacleanup.Result, error) {
	f.resetCalls++
	return datacleanup.Result{DeletedBusinessRows: 42, AIConfigPreserved: true}, nil
}

func TestCORSAllowsFrozenV2MutationHeaders(t *testing.T) {
	handler := &Handler{config: config.Config{AllowedOrigins: []string{"http://localhost:5173"}}}
	router := gin.New()
	router.Use(handler.cors())
	router.OPTIONS("/api/v2/source-versions/sv_test/chapters/ch_test", func(c *gin.Context) {})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v2/source-versions/sv_test/chapters/ch_test", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	request.Header.Set("Access-Control-Request-Headers", "Idempotency-Key,If-Match,X-Trace-ID")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected preflight status %d", recorder.Code)
	}
	for _, expected := range []string{"Idempotency-Key", "If-Match", "X-Trace-ID", "X-Data-Reset-Intent"} {
		if !strings.Contains(recorder.Header().Get("Access-Control-Allow-Headers"), expected) {
			t.Fatalf("missing CORS header %s: %s", expected, recorder.Header().Get("Access-Control-Allow-Headers"))
		}
	}
	if !strings.Contains(recorder.Header().Get("Access-Control-Allow-Methods"), http.MethodPatch) {
		t.Fatalf("PATCH is not allowed: %s", recorder.Header().Get("Access-Control-Allow-Methods"))
	}
}

func TestEffectiveInputReadOnlyRouteValidatesStageAndAvailability(t *testing.T) {
	router := New(nil, config.Config{}).Router()

	missingStage := httptest.NewRecorder()
	router.ServeHTTP(missingStage, httptest.NewRequest(
		http.MethodGet, "/api/v1/projects/p1/episodes/ep1/effective-inputs", nil,
	))
	if missingStage.Code != http.StatusBadRequest ||
		!strings.Contains(missingStage.Body.String(), "EFFECTIVE_INPUT_STAGE_REQUIRED") {
		t.Fatalf("unexpected missing-stage response: %d %s",
			missingStage.Code, missingStage.Body.String())
	}

	unavailable := httptest.NewRecorder()
	router.ServeHTTP(unavailable, httptest.NewRequest(
		http.MethodGet, "/api/v1/projects/p1/episodes/ep1/effective-inputs?stage=09", nil,
	))
	if unavailable.Code != http.StatusServiceUnavailable ||
		!strings.Contains(unavailable.Body.String(), "EFFECTIVE_INPUT_RESOLVER_UNAVAILABLE") {
		t.Fatalf("unexpected unavailable response: %d %s",
			unavailable.Code, unavailable.Body.String())
	}
}

func TestDirectFormalContentMutationRoutesAreUnavailable(t *testing.T) {
	router := New(nil, config.Config{}).Router()

	directEpisodePatch := httptest.NewRecorder()
	router.ServeHTTP(directEpisodePatch, httptest.NewRequest(
		http.MethodPatch, "/api/v1/projects/p1/episode-runs/run1/content",
		strings.NewReader(`{"outline":{"title":"overwrite"}}`),
	))
	if directEpisodePatch.Code != http.StatusNotFound {
		t.Fatalf("legacy direct episode PATCH is still routed: %d %s",
			directEpisodePatch.Code, directEpisodePatch.Body.String())
	}

	for _, endpoint := range []string{
		"/api/v1/projects/p1/episodes/ep1/editing-template",
		"/api/v1/projects/p1/episodes/ep1/sound-style",
		"/api/v1/projects/p1/episodes/ep1/timeline-versions/timeline1/restore",
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(
			http.MethodPost, endpoint, strings.NewReader(`{}`),
		))
		if recorder.Code != http.StatusGone ||
			!strings.Contains(recorder.Body.String(), "DIRECT_TIMELINE_MUTATION_DISABLED") {
			t.Fatalf("direct timeline mutation is not explicitly disabled at %s: %d %s",
				endpoint, recorder.Code, recorder.Body.String())
		}
	}
}

func TestCreateProjectForwardsToN8N(t *testing.T) {
	receivedCh := make(chan createProjectWorkflowRequest, 1)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		var received createProjectWorkflowRequest
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode webhook request: %v", err)
		}
		receivedCh <- received
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"project_id":"p_test_001","status":"waiting_review"}`))
	}))
	defer webhook.Close()

	handler := New(nil, config.Config{N8NProjectURL: webhook.URL, WebhookTimeout: time.Second})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{
		"novel_text":"第一章 测试正文", "novel_name":"测试小说", "target_episode_count":12,
		"episode_duration_seconds":90, "visual_style":"写实", "aspect_ratio":"9:16",
		"target_platform":"抖音", "test_mode":true
	}`))
	request.Header.Set("Content-Type", "application/json")
	handler.Router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var received createProjectWorkflowRequest
	select {
	case received = <-receivedCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for n8n dispatch")
	}
	if received.NovelText != "第一章 测试正文" || received.NovelName != "测试小说" {
		t.Fatalf("unexpected forwarded request: %+v", received)
	}
	if received.Action != "run" || received.ProjectID == "" {
		t.Fatalf("missing asynchronous workflow identity: %+v", received)
	}
	var response struct {
		Data struct {
			ProjectID string `json:"project_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode CMS response: %v", err)
	}
	if response.Data.ProjectID != received.ProjectID {
		t.Fatalf("unexpected project id %q", response.Data.ProjectID)
	}
}

func TestCreateProjectRejectsMissingNovelText(t *testing.T) {
	handler := New(nil, config.Config{WebhookTimeout: time.Second})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{
		"novel_name":"测试小说", "target_episode_count":12, "episode_duration_seconds":90,
		"visual_style":"写实", "aspect_ratio":"9:16", "target_platform":"抖音"
	}`))
	request.Header.Set("Content-Type", "application/json")
	handler.Router().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestReviewWebhookRoute(t *testing.T) {
	tests := []struct {
		stage, entity, wantWebhook, wantTarget string
	}{
		{"story_bible", "story_bible", "stage2", ""},
		{"season_outline", "season", "stage2", ""},
		{"visual_asset", "generated_asset", "stage3", ""},
		{"storyboard_image", "storyboard_image", "stage3", ""},
		{"shot_video", "shot_video", "stage4", "image_to_video"},
		{"dialogue_audio", "dialogue_audio", "stage4", "voice_audio"},
		{"voice_profile", "voice_profile", "stage4", "voice_audio"},
		{"final_review", "final_review", "stage5", ""},
		{"publication_metadata", "publication_metadata", "stage5", ""},
	}
	for _, test := range tests {
		webhook, target, ok := reviewWebhookRoute(test.stage, test.entity)
		if !ok || webhook != test.wantWebhook || target != test.wantTarget {
			t.Fatalf("route %s/%s = %s/%s/%v, want %s/%s", test.stage, test.entity, webhook, target, ok, test.wantWebhook, test.wantTarget)
		}
	}
	if _, _, ok := reviewWebhookRoute("unknown", "unknown"); ok {
		t.Fatal("unknown review route should be rejected")
	}
}

func TestNormalizeVisualAssetRegenerationMode(t *testing.T) {
	tests := []struct {
		mode, reviewStatus, want string
		ok                       bool
	}{
		{"", "approved", "variant", true},
		{"", "rejected", "replace", true},
		{" VARIANT ", "approved", "variant", true},
		{"replace", "rejected", "replace", true},
		{"overwrite", "rejected", "overwrite", false},
	}
	for _, test := range tests {
		got, ok := normalizeVisualAssetRegenerationMode(test.mode, test.reviewStatus)
		if got != test.want || ok != test.ok {
			t.Fatalf("normalizeVisualAssetRegenerationMode(%q,%q) = %q/%v, want %q/%v",
				test.mode, test.reviewStatus, got, ok, test.want, test.ok)
		}
	}
}

func TestN8NReturnedFailure(t *testing.T) {
	failed, message := n8nReturnedFailure(map[string]any{
		"success": false,
		"status":  "failed",
		"error": map[string]any{
			"code":    "VOICE_NOT_SUPPORTED",
			"message": "voice profile is missing, unapproved, or has no provider voice",
		},
	})
	if !failed {
		t.Fatal("success=false response should be treated as failed")
	}
	if !strings.Contains(message, "VOICE_NOT_SUPPORTED") {
		t.Fatalf("unexpected failure message %q", message)
	}

	failed, _ = n8nReturnedFailure(map[string]any{"success": true})
	if failed {
		t.Fatal("success=true response should not be treated as failed")
	}
}

func TestProjectFlowRoute(t *testing.T) {
	tests := []struct {
		stage, wantWebhook, wantTarget string
	}{
		{"novel_import", "projects", ""},
		{"story_bible_approved", "stage2", ""},
		{"storyboard_approved", "stage3", ""},
		{"visual_asset_review", "stage3", ""},
		{"storyboard_images_approved", "stage4", ""},
		{"video_processing", "stage4", ""},
		{"shot_videos_approved", "stage4", ""},
		{"audio_processing", "stage4", ""},
		{"stage_4_completed", "stage5", ""},
		{"waiting_final_review", "stage5", ""},
	}
	for _, test := range tests {
		webhook, target, ok := projectFlowRoute(test.stage)
		if !ok || webhook != test.wantWebhook || target != test.wantTarget {
			t.Fatalf("route %s = %s/%s/%v, want %s/%s", test.stage, webhook, target, ok, test.wantWebhook, test.wantTarget)
		}
	}
	if _, _, ok := projectFlowRoute("unknown_stage"); ok {
		t.Fatal("unknown project stage should be rejected")
	}
}

func TestVersionedAdaptationProjectDetection(t *testing.T) {
	if !isVersionedAdaptationProject(json.RawMessage(`{
		"contract_version":"2.0","source_version_id":"sv_test"
	}`)) {
		t.Fatal("v2 source-bound project was not detected")
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"contract_version":"2.0"}`),
		json.RawMessage(`{"source_version_id":"sv_test"}`),
		json.RawMessage(`not-json`),
	} {
		if isVersionedAdaptationProject(raw) {
			t.Fatalf("legacy or invalid config was detected as v2: %s", raw)
		}
	}
}

func TestResolvePublicMediaURL(t *testing.T) {
	value := func(input string) *string { return &input }
	tests := []struct {
		name       string
		candidate  *string
		publicBase string
		want       string
	}{
		{"container storage path", value("/data/storage/provider-responses/frame.svg"), "https://media.example.com", "https://media.example.com/provider-responses/frame.svg"},
		{"windows storage path", value(`D:\storage\shot-videos\shot.mp4`), "https://media.example.com/", "https://media.example.com/shot-videos/shot.mp4"},
		{"local media url", value("http://localhost:8088/dialogue-audio/line.wav"), "https://media.example.com", "https://media.example.com/dialogue-audio/line.wav"},
		{"external provider url", value("https://provider.example.net/output.mp4"), "https://media.example.com", "https://provider.example.net/output.mp4"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolvePublicMediaURL(test.publicBase, test.candidate)
			if got == nil || *got != test.want {
				t.Fatalf("resolvePublicMediaURL() = %v, want %q", got, test.want)
			}
		})
	}
	if got := resolvePublicMediaURL("https://media.example.com", nil); got != nil {
		t.Fatalf("nil media candidate should remain nil, got %q", *got)
	}
}

func TestUpdateAIConfigDoesNotEchoSecrets(t *testing.T) {
	handler := New(nil, config.Config{ManagedEnvFile: filepath.Join(t.TempDir(), "cms-managed.env")})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/ai-config", strings.NewReader(`{
		"values":{"MOCK_MODE":"false","IMAGE_MODEL":"image-model-v2"},
		"secrets":{"IMAGE_API_KEY":"test-secret-must-not-be-returned"}
	}`))
	request.Header.Set("Content-Type", "application/json")
	handler.Router().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "test-secret-must-not-be-returned") {
		t.Fatal("AI configuration response exposed a secret")
	}
	if !strings.Contains(recorder.Body.String(), `"saved_secret_count":1`) || !strings.Contains(recorder.Body.String(), `"restart_required":true`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestDataResetRequiresIntentAndExactConfirmation(t *testing.T) {
	cleaner := &fakeDataCleaner{}
	handler := &Handler{dataCleaner: cleaner}
	router := handler.Router()

	for name, test := range map[string]struct {
		intent string
		body   string
	}{
		"missing intent": {"", `{"confirmation":"永久删除全部数据","preserve_ai_config":true}`},
		"wrong phrase":   {"permanent", `{"confirmation":"删除数据","preserve_ai_config":true}`},
		"AI not kept":    {"permanent", `{"confirmation":"永久删除全部数据","preserve_ai_config":false}`},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/data-reset", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.intent != "" {
				request.Header.Set("X-Data-Reset-Intent", test.intent)
			}
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if cleaner.resetCalls != 0 {
		t.Fatalf("unsafe request invoked reset %d times", cleaner.resetCalls)
	}
}

func TestDataResetWithExactConfirmation(t *testing.T) {
	cleaner := &fakeDataCleaner{}
	handler := &Handler{dataCleaner: cleaner}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-reset", strings.NewReader(`{
		"confirmation":"永久删除全部数据",
		"preserve_ai_config":true
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Data-Reset-Intent", "permanent")
	handler.Router().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if cleaner.resetCalls != 1 || !strings.Contains(recorder.Body.String(), `"ai_config_preserved":true`) {
		t.Fatalf("unexpected reset response: %s", recorder.Body.String())
	}
}
