package store

import (
	"context"
	"os"
	"testing"
	"time"

	"short-drama-cms/backend/internal/effectiveinput"
)

func TestEffectiveInputResolverIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE18_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE18_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var claimsBefore, claimsAfter int
	if err := database.pool.QueryRow(ctx,
		`SELECT count(*) FROM drama.generation_effective_input_claims`).Scan(&claimsBefore); err != nil {
		t.Fatal(err)
	}
	result, err := effectiveinput.New(database).Resolve(
		ctx, "p_phase1_legacy", "ep_phase1_legacy_001", "09",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != "effective-input-resolution.v1" ||
		result.ResolverVersion != "effective-input-resolver.v1" ||
		len(result.Items) != 11 || result.ContextHash == "" || result.ResolutionHash == "" {
		t.Fatalf("incomplete resolution: %+v", result)
	}
	if result.Mode != "legacy" {
		t.Fatalf("historical project did not retain legacy compatibility mode: %q", result.Mode)
	}
	if err := database.pool.QueryRow(ctx,
		`SELECT count(*) FROM drama.generation_effective_input_claims`).Scan(&claimsAfter); err != nil {
		t.Fatal(err)
	}
	if claimsAfter != claimsBefore {
		t.Fatalf("read-only resolution wrote generation claims: before=%d after=%d",
			claimsBefore, claimsAfter)
	}
}
