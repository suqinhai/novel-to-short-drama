package httpapi

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPICandidateContractParses(t *testing.T) {
	raw, err := os.ReadFile("../../../../contracts/openapi/narrative-api.v2.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatalf("OpenAPI YAML is invalid: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("unexpected OpenAPI version %q", document.OpenAPI)
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
