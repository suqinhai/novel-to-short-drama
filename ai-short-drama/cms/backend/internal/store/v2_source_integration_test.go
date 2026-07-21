package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSourceV2LifecycleIntegration(t *testing.T) {
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
	workKey := "phase2-test-work-" + suffix
	work, created, err := database.CreateSourceWork(ctx, workKey, CreateSourceWorkInput{Title: "Phase 2 test", Metadata: json.RawMessage(`{}`)})
	if err != nil || !created {
		t.Fatalf("create work: created=%v err=%v", created, err)
	}
	replayedWork, created, err := database.CreateSourceWork(ctx, workKey, CreateSourceWorkInput{Title: "Phase 2 test", Metadata: json.RawMessage(`{}`)})
	if err != nil || created || replayedWork.WorkID != work.WorkID {
		t.Fatalf("work replay: created=%v work=%#v err=%v", created, replayedWork, err)
	}
	versionKey := "phase2-test-version-" + suffix
	version, _, err := database.CreateSourceVersion(ctx, work.WorkID, versionKey, CreateSourceVersionInput{
		NormalizationVersion: "phase2-test-v1", Metadata: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	chapterKey := "phase2-test-import-" + suffix
	importInput := ImportInput{Mode: "batch_chapters", Items: []ChapterInput{
		{ClientItemKey: "c1", Ordinal: 1, Title: "第一章 开端", Content: "第一章正文"},
		{ClientItemKey: "c2", Ordinal: 2, Title: "第二章 相遇", Content: "第二章正文"},
		{ClientItemKey: "c3", Ordinal: 3, Title: "第三章 转折", Content: "第三章正文"},
		{ClientItemKey: "c4", Ordinal: 4, Title: "第四章 危机", Content: "第四章正文"},
		{ClientItemKey: "c5", Ordinal: 5, Title: "第五章 抉择", Content: "第五章正文"},
	}}
	operation, revision, err := database.ApplyImport(ctx, version.SourceVersionID, 1, chapterKey, importInput)
	if err != nil || operation.Status != "completed" || revision != 2 {
		t.Fatalf("apply import: op=%#v revision=%d err=%v", operation, revision, err)
	}
	replayed, replayRevision, err := database.ApplyImport(ctx, version.SourceVersionID, 1, chapterKey, importInput)
	if err != nil || replayed.OperationID != operation.OperationID || replayRevision != 2 {
		t.Fatalf("import replay: op=%#v revision=%d err=%v", replayed, replayRevision, err)
	}
	chapters, err := database.ListVersionChapters(ctx, version.SourceVersionID)
	if err != nil || len(chapters) != 5 {
		t.Fatalf("list chapters: %#v err=%v", chapters, err)
	}
	for index, chapter := range chapters {
		if chapter.Ordinal != index+1 {
			t.Fatalf("chapter order mismatch at %d: %#v", index, chapter)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(importInput.Items[index].Content)))
		if chapter.ContentHash != digest {
			t.Fatalf("chapter hash mismatch at %d: %s", index, chapter.ContentHash)
		}
		var membershipVersion string
		if err := database.pool.QueryRow(ctx, `SELECT source_version_id FROM drama.source_version_chapters
			WHERE source_version_id=$1 AND chapter_id=$2`, version.SourceVersionID, chapter.ChapterID).Scan(&membershipVersion); err != nil || membershipVersion != version.SourceVersionID {
			t.Fatalf("chapter source version mismatch at %d: %s err=%v", index, membershipVersion, err)
		}
	}
	unchangedRevisionIDs := make([]string, len(chapters)-1)
	for index := 1; index < len(chapters); index++ {
		unchangedRevisionIDs[index-1] = chapters[index].ChapterRevisionID
	}
	reviseKey := "phase2-test-revise-" + suffix
	_, revision, err = database.ReviseChapter(ctx, version.SourceVersionID, chapters[0].ChapterID, 2, reviseKey, "第一章（修订）", "修订内容")
	if err != nil || revision != 3 {
		t.Fatalf("revise chapter: revision=%d err=%v", revision, err)
	}
	chaptersAfterRevision, err := database.ListVersionChapters(ctx, version.SourceVersionID)
	if err != nil || len(chaptersAfterRevision) != 5 {
		t.Fatalf("list revised chapters: %#v err=%v", chaptersAfterRevision, err)
	}
	for index := 1; index < len(chaptersAfterRevision); index++ {
		if chaptersAfterRevision[index].ChapterRevisionID != unchangedRevisionIDs[index-1] {
			t.Fatalf("unchanged chapter %d was duplicated", index+1)
		}
	}
	publishKey := "phase2-test-publish-" + suffix
	_, revision, err = database.PublishSourceVersion(ctx, version.SourceVersionID, 3, publishKey)
	if err != nil || revision != 4 {
		t.Fatalf("publish: revision=%d err=%v", revision, err)
	}
	_, _, err = database.ApplyImport(ctx, version.SourceVersionID, 4, "phase2-test-after-publish-"+suffix, ImportInput{
		Mode: "single_chapter", Items: []ChapterInput{{ClientItemKey: "c3", Ordinal: 3, Title: "第三章", Content: "不可写"}},
	})
	if !errors.Is(err, ErrImmutable) {
		t.Fatalf("published version mutation should be rejected, got %v", err)
	}
	irOperation, err := database.StartIRRun(ctx, version.SourceVersionID, "phase2-test-ir-"+suffix, IRRunInput{
		SchemaVersion: "narrative-extraction.v1", ExtractorVersion: "phase2-test", ChapterIDs: []string{chapters[0].ChapterID},
	})
	if err != nil || irOperation.Status != "pending" || irOperation.OperationType != "ir_extraction" ||
		irOperation.TargetType != "ir_revision" || irOperation.TargetID == version.SourceVersionID {
		t.Fatalf("IR operation must remain honestly pending: %#v err=%v", irOperation, err)
	}
	var irSourceVersionID, irStatus, irOperationID string
	if err := database.pool.QueryRow(ctx, `SELECT source_version_id,status,operation_id FROM drama.narrative_ir_revisions WHERE ir_revision_id=$1`, irOperation.TargetID).
		Scan(&irSourceVersionID, &irStatus, &irOperationID); err != nil {
		t.Fatalf("staging IR revision missing: %v", err)
	}
	if irSourceVersionID != version.SourceVersionID || irStatus != "staging" || irOperationID != irOperation.OperationID {
		t.Fatalf("operation/IR linkage mismatch: source=%s status=%s operation=%s", irSourceVersionID, irStatus, irOperationID)
	}
	replayedIR, err := database.StartIRRun(ctx, version.SourceVersionID, "phase2-test-ir-"+suffix, IRRunInput{
		SchemaVersion: "narrative-extraction.v1", ExtractorVersion: "phase2-test", ChapterIDs: []string{chapters[0].ChapterID},
	})
	if err != nil || replayedIR.OperationID != irOperation.OperationID || replayedIR.TargetID != irOperation.TargetID {
		t.Fatalf("IR run replay did not preserve operation and staging IR: %#v err=%v", replayedIR, err)
	}
}

func TestSourceV2ThousandChapterBatchIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE2_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE2_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	work, _, err := database.CreateSourceWork(ctx, "phase5-scale-work-"+suffix, CreateSourceWorkInput{Title: "Phase 5 scale", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	version, _, err := database.CreateSourceVersion(ctx, work.WorkID, "phase5-scale-version-"+suffix, CreateSourceVersionInput{NormalizationVersion: "phase5-scale-v1", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	revision := version.ResourceRevision
	for batch := 0; batch < 10; batch++ {
		items := make([]ChapterInput, 0, 100)
		for offset := 0; offset < 100; offset++ {
			ordinal := batch*100 + offset + 1
			items = append(items, ChapterInput{ClientItemKey: fmt.Sprintf("chapter-%04d", ordinal), Ordinal: ordinal, Title: fmt.Sprintf("第%d章", ordinal), Content: fmt.Sprintf("第%d章的 Mock 正文。", ordinal)})
		}
		operation, nextRevision, err := database.ApplyImport(ctx, version.SourceVersionID, revision, fmt.Sprintf("phase5-scale-%s-%02d", suffix, batch), ImportInput{Mode: "batch_chapters", Items: items})
		if err != nil || operation.Status != "completed" {
			t.Fatalf("batch %d: operation=%#v err=%v", batch, operation, err)
		}
		revision = nextRevision
	}
	chapters, err := database.ListVersionChapters(ctx, version.SourceVersionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1000 || chapters[0].Ordinal != 1 || chapters[999].Ordinal != 1000 {
		t.Fatalf("unexpected 1000 chapter snapshot: count=%d", len(chapters))
	}
}
