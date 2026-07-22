package store

import "testing"

func TestCanArchiveProject(t *testing.T) {
	tests := []struct {
		name                                      string
		status                                    string
		failed, active, reviews, finalizedOutputs int
		want                                      bool
	}{
		{"failed project", "failed", 1, 0, 0, 0, true},
		{"stale running project", "running", 2, 0, 0, 0, true},
		{"active task", "running", 1, 1, 0, 0, false},
		{"pending review", "failed", 1, 0, 1, 0, false},
		{"final master", "failed", 1, 0, 0, 1, false},
		{"healthy project", "running", 0, 0, 0, 0, false},
		{"completed project", "completed", 1, 0, 0, 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := canArchiveProject(test.status, test.failed, test.active, test.reviews, test.finalizedOutputs)
			if got != test.want {
				t.Fatalf("canArchiveProject() = %v, want %v", got, test.want)
			}
		})
	}
}
