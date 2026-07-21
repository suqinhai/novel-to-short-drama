package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestAdaptationProjectAndSpecIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE2_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE2_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suffix, err := newPublicID("")
	if err != nil {
		t.Fatal(err)
	}
	work, _, err := database.CreateSourceWork(ctx, "adaptation-test-work-"+suffix,
		CreateSourceWorkInput{Title: "Adaptation integration", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	version, _, err := database.CreateSourceVersion(ctx, work.WorkID, "adaptation-test-version-"+suffix,
		CreateSourceVersionInput{NormalizationVersion: "test-v1", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, revision, err := database.ApplyImport(ctx, version.SourceVersionID, 1, "adaptation-test-import-"+suffix,
		ImportInput{Mode: "single_chapter", Items: []ChapterInput{{ClientItemKey: "c1", Ordinal: 1, Title: "Chapter", Content: "Content"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = database.PublishSourceVersion(ctx, version.SourceVersionID, revision, "adaptation-test-publish-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	chapters, err := database.ListVersionChapters(ctx, version.SourceVersionID)
	if err != nil || len(chapters) != 1 {
		t.Fatalf("chapters=%#v err=%v", chapters, err)
	}
	irOperation, err := database.StartIRRun(ctx, version.SourceVersionID, "adaptation-test-ir-"+suffix,
		IRRunInput{SchemaVersion: "narrative-extraction.v1", ExtractorVersion: "test-v1", ChapterIDs: []string{chapters[0].ChapterID}})
	if err != nil {
		t.Fatal(err)
	}
	outputHash := hashText("empty-published-ir")
	if _, err := database.writer.Exec(ctx, `UPDATE drama.narrative_ir_revisions SET status='published',is_current=true,
		output_hash=$2,published_at=CURRENT_TIMESTAMP WHERE ir_revision_id=$1`, irOperation.TargetID, outputHash); err != nil {
		t.Fatal(err)
	}
	spec := AdaptationSpecInput{SchemaVersion: "adaptation-spec.v1", SourceVersionID: version.SourceVersionID,
		Scope:    AdaptationScopeInput{Mode: "chapters_only", ChapterIDs: []string{chapters[0].ChapterID}},
		Platform: "test-platform", AudienceProfile: json.RawMessage(`{"age":"adult"}`), TargetEpisodeCount: 8,
		EpisodeDurationSeconds: 120, Rules: []AdaptationRuleInput{{RuleType: "must_preserve", Enforcement: "hard",
			TargetType: "chapter", TargetID: &chapters[0].ChapterID, Priority: 100, Parameters: json.RawMessage(`{}`)}},
	}
	projectKey := "adaptation-test-project-" + suffix
	operation, err := database.CreateAdaptationProject(ctx, projectKey,
		CreateAdaptationProjectInput{DisplayName: "Adaptation project", AdaptationSpec: spec})
	if err != nil || operation.Status != "completed" || operation.TargetType != "project" || operation.ResultRef == nil {
		t.Fatalf("create project operation=%#v err=%v", operation, err)
	}
	replay, err := database.CreateAdaptationProject(ctx, projectKey,
		CreateAdaptationProjectInput{DisplayName: "Adaptation project", AdaptationSpec: spec})
	if err != nil || replay.OperationID != operation.OperationID {
		t.Fatalf("project replay=%#v err=%v", replay, err)
	}
	summaries, err := database.ListAdaptationSpecs(ctx, operation.TargetID)
	if err != nil || len(summaries) != 1 || summaries[0].Status != "active" || summaries[0].IRRevisionID == nil ||
		*summaries[0].IRRevisionID != irOperation.TargetID {
		t.Fatalf("initial specs=%#v err=%v", summaries, err)
	}
	// The second version exercises the explicit frozen-IR path after the first
	// version proved that omission resolves the sole current published full IR.
	spec.IRRevisionID = irOperation.TargetID
	spec.TargetEpisodeCount = 10
	specKey := "adaptation-test-spec-v2-" + suffix
	specOperation, err := database.CreateAdaptationSpecVersion(ctx, operation.TargetID, specKey, spec)
	if err != nil || specOperation.Status != "completed" || specOperation.TargetType != "adaptation_spec_version" {
		t.Fatalf("new spec operation=%#v err=%v", specOperation, err)
	}
	replayedSpec, err := database.CreateAdaptationSpecVersion(ctx, operation.TargetID, specKey, spec)
	if err != nil || replayedSpec.OperationID != specOperation.OperationID {
		t.Fatalf("spec replay=%#v err=%v", replayedSpec, err)
	}
	summaries, err = database.ListAdaptationSpecs(ctx, operation.TargetID)
	if err != nil || len(summaries) != 2 || summaries[0].Status != "active" || summaries[1].Status != "superseded" {
		t.Fatalf("versioned specs=%#v err=%v", summaries, err)
	}
}
