package aiconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerSaveUsesWhitelistAndPreservesSecretWithoutReturningIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cms-managed.env")
	if err := os.WriteFile(path, []byte("POSTGRES_PASSWORD=must-not-survive\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(path, "unused")
	result, err := manager.Save(
		map[string]string{"MOCK_MODE": "false", "IMAGE_MODEL": "image-model-v2"},
		map[string]string{"IMAGE_API_KEY": "test-$-secret"},
	)
	if err != nil {
		t.Fatalf("save managed config: %v", err)
	}
	if result.SavedFieldCount != 2 || result.SavedSecretCount != 1 {
		t.Fatalf("unexpected save counts: %+v", result)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "POSTGRES_PASSWORD") {
		t.Fatal("non-whitelisted key was retained")
	}
	values, exists, err := readEnvFile(path)
	if err != nil || !exists {
		t.Fatalf("read managed config: exists=%v err=%v", exists, err)
	}
	if values["MOCK_MODE"] != "false" || values["IMAGE_MODEL"] != "image-model-v2" || values["IMAGE_API_KEY"] != "test-$-secret" {
		t.Fatal("managed values did not round-trip")
	}
}

func TestManagerSaveRejectsUnknownAndInjectedValues(t *testing.T) {
	manager := New(filepath.Join(t.TempDir(), "cms-managed.env"), "unused")
	tests := []struct {
		values  map[string]string
		secrets map[string]string
	}{
		{values: map[string]string{"POSTGRES_PASSWORD": "forbidden"}},
		{values: map[string]string{"MOCK_MODE": "yes"}},
		{values: map[string]string{"AI_CONNECTION_MODE": "unsupported"}},
		{values: map[string]string{"TEXT_API_SOURCE": "subscription"}},
		{values: map[string]string{"IMAGE_MODEL": "model\nINJECTED=value"}},
		{secrets: map[string]string{"IMAGE_API_KEY": ""}},
		{secrets: map[string]string{"UNKNOWN_API_KEY": "secret"}},
	}
	for _, test := range tests {
		if _, err := manager.Save(test.values, test.secrets); err != ErrInvalidInput {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	}
}

func TestManagerSaveAcceptsConnectionPlanAndProviderSelections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cms-managed.env")
	manager := New(path, "unused")
	_, err := manager.Save(map[string]string{
		"AI_CONNECTION_MODE": "hybrid",
		"TEXT_API_SOURCE":    "gateway",
		"IMAGE_API_SOURCE":   "native",
		"VIDEO_API_SOURCE":   "custom",
		"TTS_API_SOURCE":     "native",
		"IMAGE_PROVIDER":     "generic_openai_images",
		"VIDEO_PROVIDER":     "generic_async_video",
		"VEO_OUTPUT_MODE":    "local",
		"TTS_PROVIDER":       "generic_sync_tts",
	}, nil)
	if err != nil {
		t.Fatalf("save connection plan: %v", err)
	}
	values, _, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["AI_CONNECTION_MODE"] != "hybrid" || values["TEXT_API_SOURCE"] != "gateway" {
		t.Fatalf("connection plan did not round-trip: %+v", values)
	}
}

func TestManagerSaveAcceptsRecommendedAndCustomVideoModels(t *testing.T) {
	for _, model := range []string{
		"gemini-omni-flash-preview",
		"veo-3.1-generate-001",
		"veo-3.1-fast-generate-001",
		"compatible-gateway-video-model",
	} {
		path := filepath.Join(t.TempDir(), "cms-managed.env")
		manager := New(path, "unused")
		if _, err := manager.Save(map[string]string{"VIDEO_MODEL": model}, nil); err != nil {
			t.Fatalf("save video model %q: %v", model, err)
		}
		values, _, err := readEnvFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if values["VIDEO_MODEL"] != model {
			t.Fatalf("video model did not round-trip: got %q want %q", values["VIDEO_MODEL"], model)
		}
	}
}

func TestSecretConfiguredRejectsPlaceholders(t *testing.T) {
	for _, value := range []string{"", "replace_me", "CHANGE_ME_NOW", "your_api_key_here"} {
		if secretConfigured(value) {
			t.Fatalf("placeholder %q should not be configured", value)
		}
	}
	if !secretConfigured("configured-token") {
		t.Fatal("non-placeholder token should be configured")
	}
}

func TestManagerAcceptsAndCanonicalizesServiceAccountJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cms-managed.env")
	manager := New(path, "unused")
	credential := `{
  "type": "service_account",
  "project_id": "example-project",
  "client_email": "video@example-project.iam.gserviceaccount.com",
  "private_key": "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n"
}`
	if _, err := manager.Save(nil, map[string]string{"VEO_SERVICE_ACCOUNT_JSON": credential}); err != nil {
		t.Fatalf("save service account JSON: %v", err)
	}
	values, _, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(values["VEO_SERVICE_ACCOUNT_JSON"], "\n  ") || !strings.Contains(values["VEO_SERVICE_ACCOUNT_JSON"], `"project_id":"example-project"`) {
		t.Fatalf("credential was not canonicalized: %q", values["VEO_SERVICE_ACCOUNT_JSON"])
	}
}

func TestManagerRejectsInvalidServiceAccountJSONAndGCSURI(t *testing.T) {
	manager := New(filepath.Join(t.TempDir(), "cms-managed.env"), "unused")
	if _, err := manager.Save(nil, map[string]string{"VEO_SERVICE_ACCOUNT_JSON": `{"type":"service_account"}`}); err != ErrInvalidInput {
		t.Fatalf("expected invalid service account JSON to be rejected, got %v", err)
	}
	if _, err := manager.Save(map[string]string{"VEO_GCS_OUTPUT_URI": "https://example.com/bucket"}, nil); err != ErrInvalidInput {
		t.Fatalf("expected non-GCS URI to be rejected, got %v", err)
	}
	if _, err := manager.Save(map[string]string{"VEO_OUTPUT_MODE": "filesystem"}, nil); err != ErrInvalidInput {
		t.Fatalf("expected invalid output mode to be rejected, got %v", err)
	}
	if _, err := manager.Save(map[string]string{"VEO_OUTPUT_MODE": "local", "VEO_GCS_OUTPUT_URI": ""}, nil); err != nil {
		t.Fatalf("expected local mode with empty GCS URI to be accepted, got %v", err)
	}
}
