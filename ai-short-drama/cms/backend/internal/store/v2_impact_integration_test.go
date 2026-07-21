package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestPublishedChapterRevisionQueuesOnlyChangedChapterIR(t *testing.T) {
	databaseURL := os.Getenv("PHASE4_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE4_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suffix, _ := newPublicID("")
	work, _, err := database.CreateSourceWork(ctx, "phase4-auto-work-"+suffix,
		CreateSourceWorkInput{Title: "Phase 4 automatic impact", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	parent, _, err := database.CreateSourceVersion(ctx, work.WorkID, "phase4-auto-parent-"+suffix,
		CreateSourceVersionInput{NormalizationVersion: "phase4-test-v1", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, parentRevision, err := database.ApplyImport(ctx, parent.SourceVersionID, 1, "phase4-auto-import-"+suffix,
		ImportInput{Mode: "batch_chapters", Items: []ChapterInput{
			{ClientItemKey: "one", Ordinal: 1, Title: "One", Content: "Original one"},
			{ClientItemKey: "two", Ordinal: 2, Title: "Two", Content: "Original two"},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if _, parentRevision, err = database.PublishSourceVersion(ctx, parent.SourceVersionID, parentRevision, "phase4-auto-publish-parent-"+suffix); err != nil {
		t.Fatal(err)
	}
	fullIR, err := database.StartIRRun(ctx, parent.SourceVersionID, "phase4-auto-full-ir-"+suffix,
		IRRunInput{SchemaVersion: "narrative-extraction.v1", ExtractorVersion: "phase4-auto-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.writer.Exec(ctx, `UPDATE drama.narrative_ir_revisions SET status='published',is_current=true,
		output_hash=repeat('1',64),published_at=CURRENT_TIMESTAMP WHERE ir_revision_id=$1`, fullIR.TargetID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.writer.Exec(ctx, `UPDATE drama.operations SET status='completed',checkpoint_stage='finished',
		result_type='ir_revision',result_id=$2,completed_at=CURRENT_TIMESTAMP WHERE operation_id=$1`, fullIR.OperationID, fullIR.TargetID); err != nil {
		t.Fatal(err)
	}

	child, _, err := database.CreateSourceVersion(ctx, work.WorkID, "phase4-auto-child-"+suffix,
		CreateSourceVersionInput{ParentSourceVersionID: &parent.SourceVersionID, NormalizationVersion: "phase4-test-v1", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	chapters, err := database.ListVersionChapters(ctx, child.SourceVersionID)
	if err != nil || len(chapters) != 2 {
		t.Fatalf("child chapters: %#v err=%v", chapters, err)
	}
	_, childRevision, err := database.ReviseChapter(ctx, child.SourceVersionID, chapters[0].ChapterID, 1,
		"phase4-auto-revise-"+suffix, "One revised", "Revised one only")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.PublishSourceVersion(ctx, child.SourceVersionID, childRevision, "phase4-auto-publish-child-"+suffix); err != nil {
		t.Fatal(err)
	}
	var scope, baseIR, operationStatus string
	var changed []byte
	if err := database.pool.QueryRow(ctx, `SELECT ir.revision_scope,ir.base_ir_revision_id,ir.changed_chapter_ids,operation.status
		FROM drama.narrative_ir_revisions ir JOIN drama.operations operation USING(operation_id)
		WHERE ir.source_version_id=$1`, child.SourceVersionID).Scan(&scope, &baseIR, &changed, &operationStatus); err != nil {
		t.Fatal(err)
	}
	var changedIDs []string
	if err := json.Unmarshal(changed, &changedIDs); err != nil {
		t.Fatal(err)
	}
	if scope != "incremental" || baseIR != fullIR.TargetID || operationStatus != "pending" ||
		len(changedIDs) != 1 || changedIDs[0] != chapters[0].ChapterID {
		t.Fatalf("automatic incremental IR mismatch: scope=%s base=%s status=%s chapters=%v", scope, baseIR, operationStatus, changedIDs)
	}
	_ = parentRevision
}

func TestChapterImpactReadAndDecisionIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE4_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE4_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	impact, traceID, err := database.GetProjectImpact(ctx, "p_phase1_legacy", "sv_phase4_revision")
	if err != nil {
		t.Fatal(err)
	}
	if traceID == "" || impact.Status != "needs_review" || len(impact.ChangedEvents) != 1 ||
		len(impact.ChangedCharacterStates) != 1 || len(impact.AffectedStoryArcs) != 1 || len(impact.AffectedArtifacts) < 4 {
		t.Fatalf("unexpected impact report: trace=%q impact=%#v", traceID, impact)
	}
	artifactIDs := []string{impact.AffectedArtifacts[0].ArtifactID, impact.AffectedArtifacts[1].ArtifactID}
	key := "phase4-regeneration-integration-" + time.Now().UTC().Format("20060102150405.000000000")
	request, created, err := database.CreateRegenerationRequest(ctx, "p_phase1_legacy", impact.SourceChangeSetID, key,
		RegenerationRequestInput{Strategy: "selective", ArtifactIDs: artifactIDs})
	if err != nil || !created || request.Status != "queued" || len(request.ArtifactIDs) != 2 {
		t.Fatalf("create regeneration request: created=%v request=%#v err=%v", created, request, err)
	}
	replay, created, err := database.CreateRegenerationRequest(ctx, "p_phase1_legacy", impact.SourceChangeSetID, key,
		RegenerationRequestInput{Strategy: "selective", ArtifactIDs: artifactIDs})
	if err != nil || created || replay.RegenerationRequestID != request.RegenerationRequestID {
		t.Fatalf("regeneration idempotency replay failed: created=%v request=%#v err=%v", created, replay, err)
	}
}
