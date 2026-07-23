package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/config"
)

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
	for _, expected := range []string{"Idempotency-Key", "If-Match", "X-Trace-ID"} {
		if !strings.Contains(recorder.Header().Get("Access-Control-Allow-Headers"), expected) {
			t.Fatalf("missing CORS header %s: %s", expected, recorder.Header().Get("Access-Control-Allow-Headers"))
		}
	}
	if !strings.Contains(recorder.Header().Get("Access-Control-Allow-Methods"), http.MethodPatch) {
		t.Fatalf("PATCH is not allowed: %s", recorder.Header().Get("Access-Control-Allow-Methods"))
	}
}

func TestCreateProjectForwardsToN8N(t *testing.T) {
	var received createProjectRequest
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode webhook request: %v", err)
		}
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

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if received.NovelText != "第一章 测试正文" || received.NovelName != "测试小说" {
		t.Fatalf("unexpected forwarded request: %+v", received)
	}
	var response struct {
		Data struct {
			ProjectID string `json:"project_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode CMS response: %v", err)
	}
	if response.Data.ProjectID != "p_test_001" {
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
