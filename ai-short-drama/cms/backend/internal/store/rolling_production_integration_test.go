package store

import (
	"context"
	"os"
	"testing"
	"time"
)

// The rolling test uses the same frozen phase-1 compiler fixture as the
// compiler integration test, plus migration 12.
func TestRollingProductionAdoptionAndActivationIntegration(t *testing.T) {
	databaseURL := os.Getenv("ROLLING_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ROLLING_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	rolling, err := database.AdoptAdaptationPlan(ctx, "p_phase1_legacy", "adaptation_plan_phase1_001",
		AdoptRollingPlanInput{MaxVideoBatch: 3, Currency: "CNY"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rolling.Arcs) != 1 || len(rolling.Episodes) != 1 {
		t.Fatalf("unexpected rolling queue: %#v", rolling)
	}
	run := rolling.Episodes[0]
	if run.Status != "queued" || run.MaxVideoBatch != 3 || run.EpisodeID == "" {
		t.Fatalf("unexpected queued episode: %#v", run)
	}

	activated, err := database.ActivateEpisodeProductionRun(ctx, "p_phase1_legacy", run.EpisodeRunID)
	if err != nil {
		t.Fatal(err)
	}
	if activated.Status != "active" || activated.CurrentStage != "season_outline_approved" {
		t.Fatalf("unexpected active episode: %#v", activated)
	}
}
