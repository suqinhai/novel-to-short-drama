package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanWorkflowFilesFindsExecuteCommand(t *testing.T) {
	directory := t.TempDir()
	workflow := `{"id":"wf_test","name":"Test Workflow","nodes":[{"name":"Unsafe Command","type":"n8n-nodes-base.executeCommand"},{"name":"HTTP","type":"n8n-nodes-base.httpRequest"}]}`
	if err := os.WriteFile(filepath.Join(directory, "test.json"), []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, nodes, err := scanWorkflowFiles(directory)
	if err != nil {
		t.Fatalf("scan workflows: %v", err)
	}
	if len(expected) != 1 || expected[0].ID != "wf_test" {
		t.Fatalf("unexpected expected workflows: %+v", expected)
	}
	if len(nodes) != 1 || nodes[0].NodeName != "Unsafe Command" {
		t.Fatalf("unexpected executeCommand nodes: %+v", nodes)
	}
	check := evaluateUnsupportedNodes(nodes, nil)
	if check.Status != "degraded" || check.Count != 1 {
		t.Fatalf("unexpected node check: %+v", check)
	}
}

func TestScanWorkflowFilesRejectsMissingDirectory(t *testing.T) {
	_, _, err := scanWorkflowFiles(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("missing workflow directory should fail")
	}
}

func TestConfiguredValueRejectsPlaceholders(t *testing.T) {
	for _, value := range []string{"", "replace_me", "CHANGE_ME_NOW", "placeholder-id"} {
		if configuredValue(value) {
			t.Fatalf("placeholder %q should not be configured", value)
		}
	}
	if !configuredValue("credential-id-123") {
		t.Fatal("valid credential id should be configured")
	}
}
