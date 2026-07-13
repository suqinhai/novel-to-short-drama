package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"short-drama-cms/backend/internal/config"
)

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
