package httpapi

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAllOpenAPIContractsParse(t *testing.T) {
	var document struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	files, err := filepath.Glob("../../../../contracts/openapi/*.yaml")
	if err != nil || len(files) == 0 {
		t.Fatalf("list OpenAPI contracts: files=%d err=%v", len(files), err)
	}
	for _, file := range files {
		raw, readErr := os.ReadFile(file)
		if readErr != nil {
			t.Fatal(readErr)
		}
		document = struct {
			OpenAPI string                    `yaml:"openapi"`
			Paths   map[string]map[string]any `yaml:"paths"`
		}{}
		if err := yaml.Unmarshal(raw, &document); err != nil {
			t.Fatalf("OpenAPI YAML %s is invalid: %v", file, err)
		}
		if document.OpenAPI != "3.1.0" || len(document.Paths) == 0 {
			t.Fatalf("invalid OpenAPI root %s: version=%q paths=%d", file, document.OpenAPI, len(document.Paths))
		}
	}
	raw, err := os.ReadFile("../../../../contracts/openapi/narrative-api.v2.yaml")
	if err != nil {
		t.Fatal(err)
	}
	document = struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}{}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/adaptation-projects/{project_id}/candidate-sets",
		"/adaptation-projects/{project_id}/candidate-sets/{candidate_set_id}/selections",
		"/adaptation-projects/{project_id}/candidate-sets/{candidate_set_id}/compositions",
		"/candidates/{candidate_id}/decisions",
		"/candidates/{candidate_id}/timecode-comments",
	} {
		if _, ok := document.Paths[path]; !ok {
			t.Errorf("candidate contract path missing: %s", path)
		}
	}
}
