package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
	"unicode/utf8"
)

func TestIRMergePublishesAtomicFullSnapshotAndPreservesUTF8Spans(t *testing.T) {
	databaseURL := os.Getenv("PHASE20_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE20_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suffix, _ := newPublicID("")
	work, _, err := database.CreateSourceWork(ctx, "phase20-work-"+suffix,
		CreateSourceWorkInput{Title: "IR merge 中文🙂", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	baseSource, _, err := database.CreateSourceVersion(ctx, work.WorkID, "phase20-base-source-"+suffix,
		CreateSourceVersionInput{NormalizationVersion: "utf8-codepoint-v1", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, revision, err := database.ApplyImport(ctx, baseSource.SourceVersionID, 1, "phase20-import-"+suffix,
		ImportInput{Mode: "batch_chapters", Items: []ChapterInput{
			{ClientItemKey: "one", Ordinal: 1, Title: "第一章", Content: "林夏看见门。🙂"},
			{ClientItemKey: "two", Ordinal: 2, Title: "第二章", Content: "秘密未被引用。"},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if _, revision, err = database.PublishSourceVersion(ctx, baseSource.SourceVersionID, revision, "phase20-publish-base-"+suffix); err != nil {
		t.Fatal(err)
	}
	baseChapters, err := database.ListVersionChapters(ctx, baseSource.SourceVersionID)
	if err != nil {
		t.Fatal(err)
	}
	baseRun, err := database.StartIRRun(ctx, baseSource.SourceVersionID, "phase20-base-ir-"+suffix,
		IRRunInput{SchemaVersion: "narrative-extraction.v1", ExtractorVersion: "phase20-test"})
	if err != nil {
		t.Fatal(err)
	}
	entityID := "entity_" + suffix
	twinEntityID := "entity_twin_" + suffix
	eventFactID := "fact_event_" + suffix
	unreferencedFactID := "fact_unused_" + suffix
	changedUnreferencedFactID := "fact_changed_unused_" + suffix
	relocatedFactID := "fact_relocated_" + suffix
	if err := seedMergeTestIR(ctx, database, work.WorkID, baseSource.SourceVersionID, baseRun.TargetID,
		baseChapters, entityID, "", eventFactID, unreferencedFactID, changedUnreferencedFactID, relocatedFactID, false); err != nil {
		t.Fatal(err)
	}
	if err := publishTestIR(ctx, database, baseRun, baseRun.TargetID); err != nil {
		t.Fatal(err)
	}
	projectID := "project_" + suffix
	changedArtifactID := "art_changed_" + suffix
	downstreamArtifactID := "art_downstream_" + suffix
	unrelatedArtifactID := "art_unrelated_" + suffix
	relocatedArtifactID := "art_relocated_" + suffix
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.projects(project_id,novel_name,target_episode_count,
		episode_duration_seconds,visual_style,aspect_ratio,target_platform,test_mode)
		VALUES($1,'IR merge impact',2,90,'test','9:16','test',true)`, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.project_source_bindings(binding_id,project_id,work_id,
		source_version_id,binding_role,is_current,idempotency_key) VALUES($1,$2,$3,$4,'primary',true,$5)`,
		"binding_"+suffix, projectID, work.WorkID, baseSource.SourceVersionID, "seed:binding:"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,
		revision_number,content_hash,validity_status,is_current,idempotency_key) VALUES
		($1,'episode_outline',$4,'episode_changed',1,$5,'valid',true,$8),
		($2,'episode_script',$4,'script_downstream',1,$6,'valid',true,$9),
		($3,'episode_outline',$4,'episode_unrelated',1,$7,'valid',true,$10)`, changedArtifactID, downstreamArtifactID,
		unrelatedArtifactID, projectID, hashText("changed"), hashText("downstream"), hashText("unrelated"),
		"seed:artifact:changed:"+suffix, "seed:artifact:downstream:"+suffix, "seed:artifact:unrelated:"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,
		revision_number,content_hash,validity_status,is_current,idempotency_key)
		VALUES($1,'episode_outline',$2,'episode_relocated',1,$3,'valid',true,$4)`, relocatedArtifactID, projectID,
		hashText("relocated"), "seed:artifact:relocated:"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.artifact_source_evidence(artifact_source_evidence_id,
		artifact_id,fact_revision_id,evidence_role,idempotency_key) VALUES
		($1,$3,$5,'source',$7),($2,$4,$6,'source',$8)`, "ase_changed_"+suffix, "ase_unrelated_"+suffix,
		changedArtifactID, unrelatedArtifactID, "fr_seed_"+baseRun.TargetID, "fr_unused_"+baseRun.TargetID,
		"seed:ase:changed:"+suffix, "seed:ase:unrelated:"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.artifact_source_evidence(artifact_source_evidence_id,
		artifact_id,fact_revision_id,evidence_role,idempotency_key) VALUES($1,$2,$3,'source',$4)`,
		"ase_relocated_"+suffix, relocatedArtifactID, "fr_relocated_"+baseRun.TargetID,
		"seed:ase:relocated:"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.artifact_dependencies(artifact_dependency_id,
		upstream_artifact_id,downstream_artifact_id,dependency_type,observed_upstream_hash,idempotency_key)
		VALUES($1,$2,$3,'content',$4,$5)`, "adep_"+suffix, changedArtifactID, downstreamArtifactID,
		hashText("changed"), "seed:dependency:"+suffix); err != nil {
		t.Fatal(err)
	}

	childSource, _, err := database.CreateSourceVersion(ctx, work.WorkID, "phase20-child-source-"+suffix,
		CreateSourceVersionInput{ParentSourceVersionID: &baseSource.SourceVersionID,
			NormalizationVersion: "utf8-codepoint-v1", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	childChapters, err := database.ListVersionChapters(ctx, childSource.SourceVersionID)
	if err != nil {
		t.Fatal(err)
	}
	_, childRevision, err := database.ReviseChapter(ctx, childSource.SourceVersionID, childChapters[0].ChapterID, 1,
		"phase20-revise-"+suffix, "第一章（修订）", "序🙂林夏推开门。终")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = database.PublishSourceVersion(ctx, childSource.SourceVersionID, childRevision, "phase20-publish-child-"+suffix); err != nil {
		t.Fatal(err)
	}
	childChapters, err = database.ListVersionChapters(ctx, childSource.SourceVersionID)
	if err != nil {
		t.Fatal(err)
	}
	var incrementalID, incrementalOperationID string
	if err := database.pool.QueryRow(ctx, `SELECT ir_revision_id,operation_id FROM drama.narrative_ir_revisions
		WHERE source_version_id=$1 AND revision_scope='incremental'`, childSource.SourceVersionID).
		Scan(&incrementalID, &incrementalOperationID); err != nil {
		t.Fatal(err)
	}
	incrementalRun := Operation{OperationID: incrementalOperationID}
	if err := seedMergeTestIR(ctx, database, work.WorkID, childSource.SourceVersionID, incrementalID,
		childChapters[:1], entityID, twinEntityID, eventFactID, "", changedUnreferencedFactID, relocatedFactID, true); err != nil {
		t.Fatal(err)
	}
	if err := publishTestIR(ctx, database, incrementalRun, incrementalID); err != nil {
		t.Fatal(err)
	}

	proposal, created, err := database.CreateIRMergeProposal(ctx, "phase20-proposal-"+suffix, IRMergeProposalInput{
		BaseFullIRRevisionID: baseRun.TargetID, IncrementalIRRevisionID: incrementalID, CreatedBy: "tester",
	})
	if err != nil || !created {
		t.Fatalf("create proposal: created=%v err=%v proposal=%#v", created, err, proposal)
	}
	if proposal.UnresolvedCount != 1 || proposal.Status != "draft" {
		t.Fatalf("same-name conflict must block proposal: %#v", proposal)
	}
	if proposal.ImpactPreview.RelocationOnlyCount == 0 {
		t.Fatalf("proposal did not retain source-only relocation: %#v", proposal.ImpactPreview)
	}
	if _, err := database.PublishIRMergeProposal(ctx, proposal.IRMergeProposalID, "phase20-blocked-publish-"+suffix,
		PublishIRMergeInput{Confirmed: true}); !errors.Is(err, ErrIRMergeBlocked) {
		t.Fatalf("unresolved same-name conflict should block publish, got %v", err)
	}
	var partialCount int
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.narrative_ir_revisions WHERE merge_proposal_id=$1`,
		proposal.IRMergeProposalID).Scan(&partialCount); err != nil || partialCount != 0 {
		t.Fatalf("failed merge left a partial full IR: count=%d err=%v", partialCount, err)
	}
	var conflictItem IRMergeProposalItem
	for _, item := range proposal.Items {
		if item.CanonicalizationRequired && item.LogicalID == twinEntityID {
			conflictItem = item
			break
		}
	}
	if conflictItem.IRMergeItemID == "" {
		t.Fatal("same-name entity conflict item not found")
	}
	if _, err := database.ResolveIRMergeItem(ctx, proposal.IRMergeProposalID, conflictItem.IRMergeItemID,
		IRMergeItemResolutionInput{Resolution: "accept_new", CanonicalizationConfirmed: true,
			CanonicalizationDecision: "distinct_entities", ResolvedBy: "tester"}); err != nil {
		t.Fatal(err)
	}
	proposal, err = database.GetIRMergeProposal(ctx, proposal.IRMergeProposalID, "", "", "")
	if err != nil || proposal.Status != "ready" || proposal.UnresolvedCount != 0 {
		t.Fatalf("resolved proposal not ready: %#v err=%v", proposal, err)
	}
	var automaticallyResolved int
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.ir_merge_proposal_items
		WHERE ir_merge_proposal_id=$1 AND resolution_status='resolved'
		  AND NOT canonicalization_required`, proposal.IRMergeProposalID).Scan(&automaticallyResolved); err != nil || automaticallyResolved == 0 {
		t.Fatalf("non-conflicting changes were not automatically mergeable: count=%d err=%v", automaticallyResolved, err)
	}

	// A proposal is bound to the exact current full IR used by its preview.
	if _, err := database.writer.Exec(ctx, `UPDATE drama.narrative_ir_revisions SET is_current=false
		WHERE ir_revision_id=$1`, baseRun.TargetID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.PublishIRMergeProposal(ctx, proposal.IRMergeProposalID,
		"phase20-stale-base-"+suffix, PublishIRMergeInput{Confirmed: true}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale full IR base must reject publication, got %v", err)
	}
	if _, err := database.writer.Exec(ctx, `UPDATE drama.narrative_ir_revisions SET is_current=true
		WHERE ir_revision_id=$1`, baseRun.TargetID); err != nil {
		t.Fatal(err)
	}

	// Force a failure after the publication transaction has begun. Every staging
	// row and operation must roll back, leaving the old current full IR intact.
	if _, err := database.writer.Exec(ctx, `CREATE OR REPLACE FUNCTION drama.phase20_forced_merge_failure()
		RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
		  IF NEW.revision_scope='full' AND NEW.idempotency_key LIKE 'phase20-fault-%' THEN
		    RAISE EXCEPTION 'forced merge publication failure';
		  END IF;
		  RETURN NEW;
		END $$;
		DROP TRIGGER IF EXISTS trg_phase20_forced_merge_failure ON drama.narrative_ir_revisions;
		CREATE TRIGGER trg_phase20_forced_merge_failure BEFORE INSERT ON drama.narrative_ir_revisions
		FOR EACH ROW EXECUTE FUNCTION drama.phase20_forced_merge_failure()`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.PublishIRMergeProposal(ctx, proposal.IRMergeProposalID,
		"phase20-fault-"+suffix, PublishIRMergeInput{Confirmed: true}); err == nil {
		t.Fatal("forced merge publication failure unexpectedly succeeded")
	}
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.narrative_ir_revisions
		WHERE merge_proposal_id=$1`, proposal.IRMergeProposalID).Scan(&partialCount); err != nil || partialCount != 0 {
		t.Fatalf("transaction failure left partial full IR rows: count=%d err=%v", partialCount, err)
	}
	var proposalStatus string
	if err := database.pool.QueryRow(ctx, `SELECT status FROM drama.ir_merge_proposals
		WHERE ir_merge_proposal_id=$1`, proposal.IRMergeProposalID).Scan(&proposalStatus); err != nil || proposalStatus != "ready" {
		t.Fatalf("transaction failure changed proposal state: status=%s err=%v", proposalStatus, err)
	}
	if _, err := database.writer.Exec(ctx, `DROP TRIGGER trg_phase20_forced_merge_failure ON drama.narrative_ir_revisions;
		DROP FUNCTION drama.phase20_forced_merge_failure()`); err != nil {
		t.Fatal(err)
	}
	result, err := database.PublishIRMergeProposal(ctx, proposal.IRMergeProposalID, "phase20-publish-merge-"+suffix,
		PublishIRMergeInput{Confirmed: true, PublishedBy: "tester"})
	if err != nil {
		t.Fatal(err)
	}
	var scope, publishedStatus string
	var factCount, entityCount int
	if err := database.pool.QueryRow(ctx, `SELECT ir.revision_scope,ir.status,
		(SELECT count(*) FROM drama.narrative_fact_revisions fact WHERE fact.ir_revision_id=ir.ir_revision_id),
		(SELECT count(*) FROM drama.narrative_entity_revisions entity WHERE entity.ir_revision_id=ir.ir_revision_id)
		FROM drama.narrative_ir_revisions ir WHERE ir.ir_revision_id=$1`, result.FullIRRevisionID).
		Scan(&scope, &publishedStatus, &factCount, &entityCount); err != nil {
		t.Fatal(err)
	}
	if scope != "full" || publishedStatus != "published" || factCount != 4 || entityCount != 2 {
		t.Fatalf("invalid full snapshot scope=%s status=%s facts=%d entities=%d", scope, publishedStatus, factCount, entityCount)
	}
	var preservedInputs int
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.narrative_ir_revisions
		WHERE ir_revision_id=ANY($1) AND status='published'`, []string{baseRun.TargetID, incrementalID}).Scan(&preservedInputs); err != nil || preservedInputs != 2 {
		t.Fatalf("input IR history was not preserved: count=%d err=%v", preservedInputs, err)
	}
	var startByte, endByte, startCodepoint, endCodepoint int
	var evidence string
	if err := database.pool.QueryRow(ctx, `SELECT span.start_utf8_byte,span.end_utf8_byte,span.start_codepoint,
		span.end_codepoint,span.evidence_text FROM drama.narrative_entity_revisions entity
		JOIN drama.source_spans span ON span.source_span_id=entity.primary_source_span_id
		WHERE entity.ir_revision_id=$1 AND entity.entity_id=$2`, result.FullIRRevisionID, entityID).
		Scan(&startByte, &endByte, &startCodepoint, &endCodepoint, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence != "林夏" || endByte-startByte != len("林夏") || endCodepoint-startCodepoint != utf8.RuneCountInString("林夏") {
		t.Fatalf("UTF-8/codepoint span mismatch: byte=%d..%d codepoint=%d..%d evidence=%q",
			startByte, endByte, startCodepoint, endCodepoint, evidence)
	}
	if result.SourceChangeSetID == "" {
		t.Fatal("authoritative source change set was not created")
	}
	if len(result.ImpactOperationIDs) != 1 {
		t.Fatalf("expected one authoritative project impact operation, got %v", result.ImpactOperationIDs)
	}
	claimToken := "22222222-2222-4222-8222-222222222222"
	if _, err := database.writer.Exec(ctx, `UPDATE drama.operations SET status='running',claim_token=$2::uuid,
		lease_owner='phase20-test',lease_expires_at=CURRENT_TIMESTAMP+interval '5 minutes'
		WHERE operation_id=$1`, result.ImpactOperationIDs[0], claimToken); err != nil {
		t.Fatal(err)
	}
	var impactResult []byte
	if err := database.writer.QueryRow(ctx, `SELECT drama.analyze_chapter_impact($1,$2::uuid)`,
		result.ImpactOperationIDs[0], claimToken).Scan(&impactResult); err != nil {
		t.Fatal(err)
	}
	var changedStatus, downstreamStatus, unrelatedStatus, relocatedStatus string
	if err := database.pool.QueryRow(ctx, `SELECT
		(SELECT validity_status FROM drama.artifacts WHERE artifact_id=$1),
		(SELECT validity_status FROM drama.artifacts WHERE artifact_id=$2),
		(SELECT validity_status FROM drama.artifacts WHERE artifact_id=$3),
		(SELECT validity_status FROM drama.artifacts WHERE artifact_id=$4)`, changedArtifactID, downstreamArtifactID,
		unrelatedArtifactID, relocatedArtifactID).Scan(&changedStatus, &downstreamStatus, &unrelatedStatus, &relocatedStatus); err != nil {
		t.Fatal(err)
	}
	if changedStatus != "stale" || downstreamStatus != "stale" || unrelatedStatus != "valid" || relocatedStatus != "valid" {
		t.Fatalf("impact was not exact: changed=%s downstream=%s unrelated=%s relocated=%s result=%s",
			changedStatus, downstreamStatus, unrelatedStatus, relocatedStatus, impactResult)
	}
	var regenerationProposalCount, regenerationRequestCount int
	if err := database.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM drama.regeneration_proposals WHERE source_change_set_id=$1),
		(SELECT count(*) FROM drama.regeneration_requests WHERE source_change_set_id=$1)`, result.SourceChangeSetID).
		Scan(&regenerationProposalCount, &regenerationRequestCount); err != nil {
		t.Fatal(err)
	}
	if regenerationProposalCount != 1 || regenerationRequestCount != 0 {
		t.Fatalf("impact must create only a selectable regeneration proposal: proposals=%d requests=%d",
			regenerationProposalCount, regenerationRequestCount)
	}
	t.Logf("IR merge evidence work=%s base_full_ir=%s incremental_ir=%s proposal=%s merged_full_ir=%s source_change_set=%s impacts=%v stale=%s/%s unrelated=%s relocation_only=%s rebuild_proposals=%d automatic_rebuilds=%d",
		work.WorkID, baseRun.TargetID, incrementalID, proposal.IRMergeProposalID, result.FullIRRevisionID,
		result.SourceChangeSetID, result.ImpactOperationIDs, changedStatus, downstreamStatus,
		unrelatedStatus, relocatedStatus, regenerationProposalCount, regenerationRequestCount)
}

func seedMergeTestIR(ctx context.Context, database *Store, workID, sourceVersionID, irID string,
	chapters []ChapterRevision, entityID, twinEntityID, eventFactID, unusedFactID, changedUnusedFactID, relocatedFactID string, revised bool) error {
	if len(chapters) == 0 {
		return errors.New("chapters are required")
	}
	var chapterContent string
	if err := database.pool.QueryRow(ctx, `SELECT revision.content
		FROM drama.source_version_chapters membership
		JOIN drama.chapter_revisions revision
		  ON revision.chapter_revision_id=membership.chapter_revision_id
		WHERE membership.source_version_id=$1 AND membership.chapter_id=$2
		  AND membership.chapter_revision_id=$3`, sourceVersionID, chapters[0].ChapterID,
		chapters[0].ChapterRevisionID).Scan(&chapterContent); err != nil {
		return err
	}
	entityEvidence := "林夏"
	entityByte := len([]byte(chapterContent[:stringsIndex(chapterContent, entityEvidence)]))
	entityCodepoint := utf8.RuneCountInString(chapterContent[:stringsIndex(chapterContent, entityEvidence)])
	entitySpanID := "span_entity_" + irID
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.source_spans(source_span_id,work_id,source_version_id,
		chapter_id,chapter_revision_id,start_utf8_byte,end_utf8_byte,start_codepoint,end_codepoint,start_paragraph,
		end_paragraph,excerpt_hash,evidence_text,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,1,1,$10,$11,$12)`,
		entitySpanID, workID, sourceVersionID, chapters[0].ChapterID, chapters[0].ChapterRevisionID,
		entityByte, entityByte+len(entityEvidence), entityCodepoint, entityCodepoint+utf8.RuneCountInString(entityEvidence),
		hashText(entityEvidence), entityEvidence, "seed:"+entitySpanID); err != nil {
		return fmt.Errorf("insert entity source span content=%q byte=%d:%d codepoint=%d:%d: %w", chapterContent,
			entityByte, entityByte+len(entityEvidence), entityCodepoint,
			entityCodepoint+utf8.RuneCountInString(entityEvidence), err)
	}
	eventEvidence := chapterContent
	eventSpanID := "span_event_" + irID
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.source_spans(source_span_id,work_id,source_version_id,
		chapter_id,chapter_revision_id,start_utf8_byte,end_utf8_byte,start_codepoint,end_codepoint,start_paragraph,
		end_paragraph,excerpt_hash,evidence_text,idempotency_key) VALUES($1,$2,$3,$4,$5,0,$6,0,$7,1,1,$8,$9,$10)`,
		eventSpanID, workID, sourceVersionID, chapters[0].ChapterID, chapters[0].ChapterRevisionID,
		len(eventEvidence), utf8.RuneCountInString(eventEvidence), hashText(eventEvidence), eventEvidence, "seed:"+eventSpanID); err != nil {
		return fmt.Errorf("insert event source span content=%q bytes=%d codepoints=%d: %w", chapterContent,
			len(eventEvidence), utf8.RuneCountInString(eventEvidence), err)
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_entities(entity_id,work_id,entity_type,stable_key)
		VALUES($1,$2,'character',$3) ON CONFLICT(entity_id) DO NOTHING`, entityID, workID, "entity:"+entityID); err != nil {
		return err
	}
	entityRevisionID := "er_seed_" + irID
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_entity_revisions(entity_revision_id,entity_id,
		ir_revision_id,work_id,source_version_id,chapter_id,primary_chapter_revision_id,primary_source_span_id,
		canonical_name,attributes,confidence,validation_status,idempotency_key)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'林夏','{}',0.99,'valid',$9)`, entityRevisionID, entityID, irID,
		workID, sourceVersionID, chapters[0].ChapterID, chapters[0].ChapterRevisionID, entitySpanID, "seed:"+entityRevisionID); err != nil {
		return err
	}
	if twinEntityID != "" {
		if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_entities(entity_id,work_id,entity_type,stable_key)
			VALUES($1,$2,'character',$3)`, twinEntityID, workID, "entity:"+twinEntityID); err != nil {
			return err
		}
		twinRevisionID := "er_twin_" + irID
		if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_entity_revisions(entity_revision_id,entity_id,
			ir_revision_id,work_id,source_version_id,chapter_id,primary_chapter_revision_id,primary_source_span_id,
			canonical_name,attributes,confidence,validation_status,idempotency_key)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,'林夏','{"role":"同名人物"}',0.98,'valid',$9)`, twinRevisionID, twinEntityID,
			irID, workID, sourceVersionID, chapters[0].ChapterID, chapters[0].ChapterRevisionID, entitySpanID, "seed:"+twinRevisionID); err != nil {
			return err
		}
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_facts(fact_id,work_id,fact_kind,stable_key)
		VALUES($1,$2,'event',$3) ON CONFLICT(fact_id) DO NOTHING`, eventFactID, workID, "fact:"+eventFactID); err != nil {
		return err
	}
	eventFactRevisionID := "fr_seed_" + irID
	payload := map[string]any{"statement": "林夏看见门"}
	summary := "林夏看见门"
	if revised {
		payload["statement"] = "林夏推开门"
		summary = "林夏推开门"
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_fact_revisions(fact_revision_id,fact_id,
		ir_revision_id,work_id,source_version_id,chapter_id,primary_chapter_revision_id,primary_source_span_id,
		canonical_fingerprint,confidence,payload,validation_status,idempotency_key)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,0.99,$10,'valid',$11)`, eventFactRevisionID, eventFactID, irID,
		workID, sourceVersionID, chapters[0].ChapterID, chapters[0].ChapterRevisionID, eventSpanID,
		hashText(fmt.Sprint(payload["statement"])), mustJSON(payload), "seed:"+eventFactRevisionID); err != nil {
		return err
	}
	if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_event_revisions(event_revision_id,fact_revision_id,
		ir_revision_id,work_id,source_version_id,event_type,summary,narrative_order,importance)
		VALUES($1,$2,$3,$4,$5,'discovery',$6,1,0.9)`, "ev_seed_"+irID, eventFactRevisionID, irID, workID,
		sourceVersionID, summary); err != nil {
		return err
	}
	if changedUnusedFactID != "" {
		if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_facts(fact_id,work_id,fact_kind,stable_key)
			VALUES($1,$2,'world_rule',$3) ON CONFLICT(fact_id) DO NOTHING`, changedUnusedFactID, workID,
			"fact:"+changedUnusedFactID); err != nil {
			return err
		}
		statement := "door state: closed"
		if revised {
			statement = "door state: open"
		}
		changedUnusedRevisionID := "fr_changed_unused_" + irID
		if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_fact_revisions(fact_revision_id,fact_id,
			ir_revision_id,work_id,source_version_id,chapter_id,primary_chapter_revision_id,primary_source_span_id,
			canonical_fingerprint,confidence,payload,validation_status,idempotency_key)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,0.99,jsonb_build_object('statement',$10::text),'valid',$11)`,
			changedUnusedRevisionID, changedUnusedFactID, irID, workID, sourceVersionID, chapters[0].ChapterID,
			chapters[0].ChapterRevisionID, eventSpanID, hashText(statement), statement, "seed:"+changedUnusedRevisionID); err != nil {
			return err
		}
	}
	if relocatedFactID != "" {
		if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_facts(fact_id,work_id,fact_kind,stable_key)
			VALUES($1,$2,'world_rule',$3) ON CONFLICT(fact_id) DO NOTHING`, relocatedFactID, workID,
			"fact:"+relocatedFactID); err != nil {
			return err
		}
		relocatedRevisionID := "fr_relocated_" + irID
		if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_fact_revisions(fact_revision_id,fact_id,
			ir_revision_id,work_id,source_version_id,chapter_id,primary_chapter_revision_id,primary_source_span_id,
			canonical_fingerprint,confidence,payload,validation_status,idempotency_key)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,0.99,'{"statement":"protagonist present"}','valid',$10)`,
			relocatedRevisionID, relocatedFactID, irID, workID, sourceVersionID, chapters[0].ChapterID,
			chapters[0].ChapterRevisionID, entitySpanID, hashText("protagonist present"), "seed:"+relocatedRevisionID); err != nil {
			return err
		}
	}
	if unusedFactID != "" && len(chapters) > 1 {
		unusedEvidence := "秘密未被引用。"
		unusedSpanID := "span_unused_" + irID
		if _, err := database.writer.Exec(ctx, `INSERT INTO drama.source_spans(source_span_id,work_id,source_version_id,
			chapter_id,chapter_revision_id,start_utf8_byte,end_utf8_byte,start_codepoint,end_codepoint,start_paragraph,
			end_paragraph,excerpt_hash,evidence_text,idempotency_key) VALUES($1,$2,$3,$4,$5,0,$6,0,$7,1,1,$8,$9,$10)`,
			unusedSpanID, workID, sourceVersionID, chapters[1].ChapterID, chapters[1].ChapterRevisionID,
			len(unusedEvidence), utf8.RuneCountInString(unusedEvidence), hashText(unusedEvidence), unusedEvidence, "seed:"+unusedSpanID); err != nil {
			return err
		}
		if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_facts(fact_id,work_id,fact_kind,stable_key)
			VALUES($1,$2,'world_rule',$3) ON CONFLICT(fact_id) DO NOTHING`, unusedFactID, workID, "fact:"+unusedFactID); err != nil {
			return err
		}
		if _, err := database.writer.Exec(ctx, `INSERT INTO drama.narrative_fact_revisions(fact_revision_id,fact_id,
			ir_revision_id,work_id,source_version_id,chapter_id,primary_chapter_revision_id,primary_source_span_id,
			canonical_fingerprint,confidence,payload,validation_status,idempotency_key)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,0.99,'{"statement":"秘密"}','valid',$10)`, "fr_unused_"+irID,
			unusedFactID, irID, workID, sourceVersionID, chapters[1].ChapterID, chapters[1].ChapterRevisionID,
			unusedSpanID, hashText("秘密"), "seed:fr-unused:"+irID); err != nil {
			return err
		}
	}
	return nil
}

func publishTestIR(ctx context.Context, database *Store, operation Operation, irID string) error {
	if _, err := database.writer.Exec(ctx, `UPDATE drama.narrative_ir_revisions SET status='published',is_current=true,
		output_hash=$2,published_at=CURRENT_TIMESTAMP WHERE ir_revision_id=$1`, irID, hashText("output:"+irID)); err != nil {
		return err
	}
	_, err := database.writer.Exec(ctx, `UPDATE drama.operations SET status='completed',checkpoint_stage='finished',
		result_type='ir_revision',result_id=$2,completed_at=CURRENT_TIMESTAMP WHERE operation_id=$1`, operation.OperationID, irID)
	return err
}

func stringsIndex(value, needle string) int {
	for index := 0; index+len(needle) <= len(value); index++ {
		if value[index:index+len(needle)] == needle {
			return index
		}
	}
	return -1
}
