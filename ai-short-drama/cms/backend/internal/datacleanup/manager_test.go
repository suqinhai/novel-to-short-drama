package datacleanup

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestClearN8NHistoryPassesConfiguredConnectionDirectlyToPsql(t *testing.T) {
	manager := New(nil, Config{
		PostgresContainer: "postgres-test",
		PostgresUser:      "n8n-user",
		N8NDatabase:       "n8n-database",
	})
	var command string
	var arguments []string
	manager.run = func(_ context.Context, name string, args ...string) ([]byte, error) {
		command = name
		arguments = append([]string(nil), args...)
		return nil, nil
	}

	if err := manager.clearN8NHistory(context.Background()); err != nil {
		t.Fatal(err)
	}

	if command != "docker" {
		t.Fatalf("command = %q, want docker", command)
	}
	wantPrefix := []string{
		"exec", "postgres-test", "psql", "-X", "-v", "ON_ERROR_STOP=1",
		"-U", "n8n-user", "-d", "n8n-database", "-c",
	}
	if len(arguments) != len(wantPrefix)+1 || !slices.Equal(arguments[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("unexpected arguments: %#v", arguments)
	}
	if strings.Contains(arguments[len(arguments)-1], "$POSTGRES_") {
		t.Fatalf("SQL command still depends on container environment: %q", arguments[len(arguments)-1])
	}
}

func TestClearStoragePreservesInfrastructureFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "storage")
	for _, directory := range []string{"renders/project/episode", "novels", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"healthz":                   "ok\n",
		"renders/.gitkeep":          "\n",
		"renders/project/video.mp4": "video-data",
		"novels/source.txt":         "novel-data",
		"logs/worker.json":          "log-data",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := clearStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	if summary.FileCount != 3 || summary.TotalSize != int64(len("video-data")+len("novel-data")+len("log-data")) {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	for _, name := range []string{"healthz", "renders/.gitkeep"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			t.Fatalf("preserved file %s is missing: %v", name, err)
		}
	}
	for _, name := range []string{"renders/project/video.mp4", "novels/source.txt", "logs/worker.json"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Fatalf("data file %s still exists", name)
		}
	}
}

func TestSafeStorageRootRejectsBroadDirectory(t *testing.T) {
	if _, err := safeStorageRoot(t.TempDir()); err == nil {
		t.Fatal("directory not named storage must be rejected")
	}
}
