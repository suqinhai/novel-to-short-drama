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

	// Compiler output starts as reviewable and non-current. Adoption must
	// publish both the native plan and its artifact projection atomically.
	if _, err = database.writer.Exec(ctx, `INSERT INTO drama.artifacts(
		artifact_id,artifact_type,project_id,native_entity_id,revision_number,
		content_hash,validity_status,is_current,idempotency_key,metadata
	)
	SELECT 'artifact_rolling_plan','adaptation_plan',project_id,adaptation_plan_id,
		version_number,content_hash,'needs_review',false,'fixture:rolling:plan','{}'::jsonb
	FROM drama.adaptation_plans WHERE adaptation_plan_id='adaptation_plan_phase1_001'
	ON CONFLICT(idempotency_key) DO UPDATE
	SET validity_status='needs_review',is_current=false`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.writer.Exec(ctx, `UPDATE drama.artifacts
		SET validity_status='needs_review',is_current=false
		WHERE artifact_type='adaptation_episode_plan'
			AND native_entity_id='adaptation_episode_plan_phase1_001'`); err != nil {
		t.Fatal(err)
	}

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
	var publishedArtifacts int
	if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.artifacts
		WHERE project_id='p_phase1_legacy' AND validity_status='valid' AND is_current
			AND ((artifact_type='adaptation_plan' AND native_entity_id='adaptation_plan_phase1_001')
				OR (artifact_type='adaptation_episode_plan'
					AND native_entity_id='adaptation_episode_plan_phase1_001'))`).Scan(&publishedArtifacts); err != nil {
		t.Fatal(err)
	}
	if publishedArtifacts != 2 {
		t.Fatalf("adoption did not publish plan artifact projection: got %d current artifacts", publishedArtifacts)
	}
	var continuityEntries, charactersWithoutBible int
	if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.continuity_ledger_entries
		WHERE project_id='p_phase1_legacy' AND validation_status='valid'`).Scan(&continuityEntries); err != nil {
		t.Fatal(err)
	}
	if continuityEntries != len(rolling.Episodes) {
		t.Fatalf("expected one valid continuity seed per episode, got %d", continuityEntries)
	}
	if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT DISTINCT jsonb_array_elements_text(outline.character_ids) character_id
		FROM drama.episode_outlines outline WHERE outline.project_id='p_phase1_legacy'
	) required WHERE NOT EXISTS(
		SELECT 1 FROM drama.character_performance_bibles bible
		WHERE bible.project_id='p_phase1_legacy' AND bible.character_id=required.character_id
		  AND bible.status='locked'
	)`).Scan(&charactersWithoutBible); err != nil {
		t.Fatal(err)
	}
	if charactersWithoutBible != 0 {
		t.Fatalf("%d episode characters do not have a locked performance bible", charactersWithoutBible)
	}

	activated, err := database.ActivateEpisodeProductionRun(ctx, "p_phase1_legacy", run.EpisodeRunID)
	if err != nil {
		t.Fatal(err)
	}
	if activated.Status != "active" || activated.CurrentStage != "season_outline_approved" {
		t.Fatalf("unexpected active episode: %#v", activated)
	}
}
