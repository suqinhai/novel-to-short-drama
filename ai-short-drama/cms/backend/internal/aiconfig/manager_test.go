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
