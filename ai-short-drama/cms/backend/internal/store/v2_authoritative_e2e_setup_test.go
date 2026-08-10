package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type authoritativeE2ELineage struct {
	BaseFullIRRevisionID    string
	IncrementalIRRevisionID string
	MergeProposalID         string
	MergedFullIRRevisionID  string
	SourceChangeSetID       string
	AdaptationSpecVersionID string
	AdaptationPlanID        string
	EpisodePlanID           string
	PacingPlanID            string
	SourceVersionID         string
	SourceBindingID         string
}

// prepareAuthoritativeE2ELineage makes the candidate/downstream acceptance use
// the full IR produced by a real reviewed merge. The lifecycle successors are
// immutable copies bound to that IR; no published input row is edited in place.
func prepareAuthoritativeE2ELineage(t *testing.T, ctx context.Context, database *Store, projectID, suffix string) authoritativeE2ELineage {
	t.Helper()
	var workID, sourceVersionID, baseIRID, baseIRHash string
	if err := database.pool.QueryRow(ctx, `SELECT binding.work_id,binding.source_version_id,
		ir.ir_revision_id,ir.output_hash
		FROM drama.project_source_bindings binding
		JOIN drama.narrative_ir_revisions ir ON ir.work_id=binding.work_id
		 AND ir.source_version_id=binding.source_version_id
		WHERE binding.project_id=$1 AND binding.binding_role='primary' AND binding.is_current
		 AND ir.revision_scope='full' AND ir.status='published' AND ir.is_current`, projectID).
		Scan(&workID, &sourceVersionID, &baseIRID, &baseIRHash); err != nil {
		t.Fatalf("load authoritative E2E base lineage: %v", err)
	}
	childSource, _, err := database.CreateSourceVersion(ctx, workID, "e2e:source-version:"+suffix,
		CreateSourceVersionInput{ParentSourceVersionID: &sourceVersionID,
			NormalizationVersion: "utf8-codepoint-v1", Metadata: []byte(`{"e2e":true}`)})
	if err != nil {
		t.Fatalf("create authoritative E2E child source: %v", err)
	}
	chapters, err := database.ListVersionChapters(ctx, childSource.SourceVersionID)
	if err != nil || len(chapters) == 0 {
		t.Fatalf("load merge source chapters: count=%d err=%v", len(chapters), err)
	}
	_, sourceRevision, err := database.ReviseChapter(ctx, childSource.SourceVersionID, chapters[0].ChapterID, 1,
		"e2e:chapter-revision:"+suffix, "第一章（修订）", "序🙂林夏推开门。终")
	if err != nil {
		t.Fatalf("revise authoritative E2E chapter: %v", err)
	}
	if _, _, err := database.PublishSourceVersion(ctx, childSource.SourceVersionID, sourceRevision,
		"e2e:source-publish:"+suffix); err != nil {
		t.Fatalf("publish authoritative E2E child source: %v", err)
	}
	chapters, err = database.ListVersionChapters(ctx, childSource.SourceVersionID)
	if err != nil {
		t.Fatalf("reload authoritative E2E child chapters: %v", err)
	}
	var incrementalID, incrementalOperationID string
	if err := database.pool.QueryRow(ctx, `SELECT ir_revision_id,operation_id
		FROM drama.narrative_ir_revisions WHERE source_version_id=$1 AND revision_scope='incremental'`,
		childSource.SourceVersionID).Scan(&incrementalID, &incrementalOperationID); err != nil {
		t.Fatalf("load authoritative E2E incremental IR shell: %v", err)
	}
	twinEntityID := "entity_e2e_twin_" + suffix
	if err := seedMergeTestIR(ctx, database, workID, childSource.SourceVersionID, incrementalID, chapters[:1],
		"entity_phase1_hero", twinEntityID, "fact_phase1_event_001", "", "", "", true); err != nil {
		t.Fatalf("seed authoritative E2E incremental IR: %v", err)
	}
	if err := publishTestIR(ctx, database, Operation{OperationID: incrementalOperationID}, incrementalID); err != nil {
		t.Fatalf("publish authoritative E2E incremental IR: %v", err)
	}

	proposal, created, err := database.CreateIRMergeProposal(ctx, "e2e:merge:proposal:"+suffix, IRMergeProposalInput{
		BaseFullIRRevisionID: baseIRID, IncrementalIRRevisionID: incrementalID, CreatedBy: "authoritative-e2e",
	})
	if err != nil || !created || proposal.UnresolvedCount == 0 {
		t.Fatalf("reviewed merge proposal missing conflict: created=%v proposal=%#v err=%v", created, proposal, err)
	}
	if _, err := database.PublishIRMergeProposal(ctx, proposal.IRMergeProposalID, "e2e:merge:blocked:"+suffix,
		PublishIRMergeInput{Confirmed: true}); !errors.Is(err, ErrIRMergeBlocked) {
		t.Fatalf("unresolved E2E merge was not blocked: %v", err)
	}
	var partialFullIRs int
	if err := database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.narrative_ir_revisions
		WHERE merge_proposal_id=$1`, proposal.IRMergeProposalID).Scan(&partialFullIRs); err != nil || partialFullIRs != 0 {
		t.Fatalf("blocked E2E merge left partial rows: count=%d err=%v", partialFullIRs, err)
	}
	for _, item := range proposal.Items {
		if item.ResolutionStatus == "resolved" {
			continue
		}
		resolution := IRMergeItemResolutionInput{Resolution: "accept_new", ResolvedBy: "authoritative-e2e"}
		if item.CanonicalizationRequired {
			resolution.CanonicalizationConfirmed = true
			resolution.CanonicalizationDecision = "distinct_entities"
		}
		if _, err := database.ResolveIRMergeItem(ctx, proposal.IRMergeProposalID, item.IRMergeItemID, resolution); err != nil {
			t.Fatalf("resolve authoritative E2E merge item %s: %v", item.IRMergeItemID, err)
		}
	}
	proposal, err = database.GetIRMergeProposal(ctx, proposal.IRMergeProposalID, "", "", "")
	if err != nil || proposal.Status != "ready" || proposal.UnresolvedCount != 0 {
		t.Fatalf("manually resolved E2E merge not ready: %#v err=%v", proposal, err)
	}
	merged, err := database.PublishIRMergeProposal(ctx, proposal.IRMergeProposalID, "e2e:merge:publish:"+suffix,
		PublishIRMergeInput{Confirmed: true, PublishedBy: "authoritative-e2e"})
	if err != nil {
		t.Fatalf("publish authoritative E2E reviewed merge: %v", err)
	}
	var baseStatus, incrementalStatus, baseHashAfter string
	var baseCurrent, incrementalCurrent bool
	if err := database.pool.QueryRow(ctx, `SELECT
		(SELECT status FROM drama.narrative_ir_revisions WHERE ir_revision_id=$1),
		(SELECT is_current FROM drama.narrative_ir_revisions WHERE ir_revision_id=$1),
		(SELECT output_hash FROM drama.narrative_ir_revisions WHERE ir_revision_id=$1),
		(SELECT status FROM drama.narrative_ir_revisions WHERE ir_revision_id=$2),
		(SELECT is_current FROM drama.narrative_ir_revisions WHERE ir_revision_id=$2)`, baseIRID, incrementalID).
		Scan(&baseStatus, &baseCurrent, &baseHashAfter, &incrementalStatus, &incrementalCurrent); err != nil {
		t.Fatalf("verify authoritative E2E input IR immutability: %v", err)
	}
	if baseStatus != "published" || !baseCurrent || baseHashAfter != baseIRHash || incrementalStatus != "published" || incrementalCurrent {
		t.Fatalf("merge mutated input IR history: base=%s/%v/%s incremental=%s/%v",
			baseStatus, baseCurrent, baseHashAfter, incrementalStatus, incrementalCurrent)
	}

	lineage := authoritativeE2ELineage{
		BaseFullIRRevisionID: baseIRID, IncrementalIRRevisionID: incrementalID,
		MergeProposalID: proposal.IRMergeProposalID, MergedFullIRRevisionID: merged.FullIRRevisionID,
		SourceChangeSetID: merged.SourceChangeSetID, SourceVersionID: childSource.SourceVersionID,
	}
	rebaseLifecycleInputsForMergedIR(t, ctx, database, projectID, workID, childSource.SourceVersionID, merged.FullIRRevisionID, suffix, &lineage)
	return lineage
}

func rebaseLifecycleInputsForMergedIR(t *testing.T, ctx context.Context, database *Store,
	projectID, workID, sourceVersionID, mergedIRID, suffix string, lineage *authoritativeE2ELineage) {
	t.Helper()
	tx, err := database.writer.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var oldSpecID, sourceBindingID string
	var oldSpecVersion int
	if err := tx.QueryRow(ctx, `SELECT version.adaptation_spec_version_id,
		(SELECT COALESCE(max(candidate.version_number),0) FROM drama.adaptation_spec_versions candidate
		 WHERE candidate.adaptation_spec_id=version.adaptation_spec_id),binding.binding_id
		FROM drama.adaptation_specs spec JOIN drama.adaptation_spec_versions version USING(adaptation_spec_id)
		JOIN drama.project_source_bindings binding ON binding.project_id=spec.project_id
		 AND binding.binding_role='primary' AND binding.is_current
		WHERE spec.project_id=$1 AND spec.is_current AND version.status='active' FOR UPDATE OF version,binding`, projectID).
		Scan(&oldSpecID, &oldSpecVersion, &sourceBindingID); err != nil {
		t.Fatal(err)
	}
	newSourceBindingID := "psb_e2e_" + suffix
	if _, err := tx.Exec(ctx, `UPDATE drama.project_source_bindings SET is_current=false
		WHERE binding_id=$1`, sourceBindingID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.project_source_bindings(binding_id,project_id,work_id,
		source_version_id,binding_role,is_current,idempotency_key)
		VALUES($1,$2,$3,$4,'primary',true,$5)`, newSourceBindingID, projectID, workID, sourceVersionID,
		"e2e:source-binding:"+suffix); err != nil {
		t.Fatal(err)
	}
	sourceBindingID = newSourceBindingID
	newSpecID := "asv_e2e_" + suffix
	specOperationID := "op_e2e_spec_" + suffix
	if _, err := tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,
		status,idempotency_key,input_hash,checkpoint_stage,result_type,result_id,completed_at)
		VALUES($1,$2,'spec_validation','adaptation_spec_version',$3,'completed',$4,$5,'finished',
		'adaptation_spec_version',$3,CURRENT_TIMESTAMP)`, specOperationID, "trace_e2e_spec_"+suffix,
		newSpecID, "e2e:spec:operation:"+suffix, hashText("e2e-spec:"+mergedIRID)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_spec_versions(
		adaptation_spec_version_id,operation_id,adaptation_spec_id,project_id,source_binding_id,work_id,
		version_number,source_version_id,ir_revision_id,status,platform,audience_profile,target_episode_count,
		episode_duration_seconds,scope_mode,ruleset_version,content_hash,idempotency_key)
		SELECT $1,$2,adaptation_spec_id,project_id,$3,work_id,$4,$5,$6,'draft',platform,
		audience_profile,target_episode_count,episode_duration_seconds,scope_mode,ruleset_version,$7,$8
		FROM drama.adaptation_spec_versions WHERE adaptation_spec_version_id=$9`, newSpecID, specOperationID,
		sourceBindingID, oldSpecVersion+1, sourceVersionID, mergedIRID, hashText("e2e-spec-content:"+mergedIRID),
		"e2e:spec:version:"+suffix, oldSpecID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_scope_chapters(scope_chapter_id,
		adaptation_spec_version_id,project_id,work_id,source_version_id,ir_revision_id,chapter_id,
		include_mode,ordinal_from,ordinal_to)
		SELECT $1||'_'||row_number() OVER(ORDER BY chapter_id),$2,project_id,work_id,$3,$4,
		chapter_id,include_mode,ordinal_from,ordinal_to FROM drama.adaptation_scope_chapters
		WHERE adaptation_spec_version_id=$5`, "scope_e2e_ch_"+suffix, newSpecID, sourceVersionID, mergedIRID, oldSpecID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_scope_arcs(scope_arc_id,adaptation_spec_version_id,
		project_id,work_id,source_version_id,ir_revision_id,story_arc_revision_id,include_mode)
		SELECT $1||'_'||row_number() OVER(ORDER BY old_scope.scope_arc_id),$2,old_scope.project_id,
		old_scope.work_id,$3,$4,new_arc.story_arc_revision_id,old_scope.include_mode
		FROM drama.adaptation_scope_arcs old_scope
		JOIN drama.story_arc_revisions old_arc ON old_arc.story_arc_revision_id=old_scope.story_arc_revision_id
		JOIN drama.story_arc_revisions new_arc ON new_arc.story_arc_id=old_arc.story_arc_id AND new_arc.ir_revision_id=$4
		WHERE old_scope.adaptation_spec_version_id=$5`, "scope_e2e_arc_"+suffix, newSpecID, sourceVersionID, mergedIRID, oldSpecID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_rules(adaptation_rule_id,adaptation_spec_version_id,
		rule_type,enforcement,target_type,target_id,priority,parameters,rationale,idempotency_key)
		SELECT $1||'_'||row_number() OVER(ORDER BY rule.adaptation_rule_id),$2,rule.rule_type,rule.enforcement,
		rule.target_type,CASE rule.target_type
		  WHEN 'event' THEN (SELECT new_event.event_revision_id
		    FROM drama.narrative_event_revisions old_event
		    JOIN drama.narrative_fact_revisions old_fact USING(fact_revision_id)
		    JOIN drama.narrative_fact_revisions new_fact ON new_fact.fact_id=old_fact.fact_id AND new_fact.ir_revision_id=$3
		    JOIN drama.narrative_event_revisions new_event ON new_event.fact_revision_id=new_fact.fact_revision_id
		    WHERE old_event.event_revision_id=rule.target_id)
		  WHEN 'entity' THEN (SELECT current_revision.entity_revision_id
		    FROM drama.narrative_entity_revisions old_revision
		    JOIN drama.narrative_entity_revisions current_revision
		      ON current_revision.entity_id=old_revision.entity_id AND current_revision.ir_revision_id=$3
		    WHERE old_revision.entity_revision_id=rule.target_id)
		  WHEN 'fact' THEN (SELECT current_revision.fact_revision_id
		    FROM drama.narrative_fact_revisions old_revision
		    JOIN drama.narrative_fact_revisions current_revision
		      ON current_revision.fact_id=old_revision.fact_id AND current_revision.ir_revision_id=$3
		    WHERE old_revision.fact_revision_id=rule.target_id)
		  WHEN 'story_arc' THEN (SELECT current_revision.story_arc_revision_id
		    FROM drama.story_arc_revisions old_revision
		    JOIN drama.story_arc_revisions current_revision
		      ON current_revision.story_arc_id=old_revision.story_arc_id AND current_revision.ir_revision_id=$3
		    WHERE old_revision.story_arc_revision_id=rule.target_id)
		  ELSE rule.target_id END,
		rule.priority,rule.parameters,rule.rationale,$4||'_'||row_number() OVER(ORDER BY rule.adaptation_rule_id)
		FROM drama.adaptation_rules rule WHERE rule.adaptation_spec_version_id=$5`, "rule_e2e_"+suffix, newSpecID,
		mergedIRID, "e2e:rule:"+suffix, oldSpecID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.adaptation_spec_versions SET status='superseded'
		WHERE adaptation_spec_version_id=$1`, oldSpecID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.adaptation_spec_versions SET status='active',activated_at=CURRENT_TIMESTAMP
		WHERE adaptation_spec_version_id=$1`, newSpecID); err != nil {
		t.Fatal(err)
	}

	compilerOperationID := "op_e2e_compile_" + suffix
	compilerRunID := "compiler_e2e_" + suffix
	if _, err := tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,
		status,idempotency_key,input_hash,checkpoint_stage,result_type,result_id,completed_at)
		VALUES($1,$2,'adaptation_compile','project',$3,'completed',$4,$5,'finished','adaptation_plan',$6,CURRENT_TIMESTAMP)`,
		compilerOperationID, "trace_e2e_compile_"+suffix, projectID, "e2e:compile:operation:"+suffix,
		hashText("e2e-compile:"+mergedIRID), "plan_e2e_"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.compiler_runs(compiler_run_id,operation_id,project_id,work_id,
		source_version_id,adaptation_spec_version_id,ir_revision_id,compiler_version,status,input_hash,output_hash,
		idempotency_key,started_at,completed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,'authoritative-e2e-v1','completed',$8,$9,$10,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		compilerRunID, compilerOperationID, projectID, workID, sourceVersionID, newSpecID, mergedIRID,
		hashText("e2e-compiler-input:"+mergedIRID), hashText("e2e-compiler-output:"+mergedIRID),
		"e2e:compiler:run:"+suffix); err != nil {
		t.Fatal(err)
	}
	var oldPlanID string
	var oldPlanVersion int
	if err := tx.QueryRow(ctx, `SELECT current_plan.adaptation_plan_id,
		(SELECT COALESCE(max(candidate.version_number),0) FROM drama.adaptation_plans candidate WHERE candidate.project_id=$1)
		FROM drama.adaptation_plans current_plan
		WHERE current_plan.project_id=$1 AND current_plan.status='approved' AND current_plan.is_current FOR UPDATE`, projectID).
		Scan(&oldPlanID, &oldPlanVersion); err != nil {
		t.Fatal(err)
	}
	newPlanID := "plan_e2e_" + suffix
	if _, err := tx.Exec(ctx, `UPDATE drama.adaptation_plans SET status='superseded',is_current=false
		WHERE adaptation_plan_id=$1`, oldPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_plans(adaptation_plan_id,compiler_run_id,project_id,
		adaptation_spec_version_id,version_number,status,is_current,content_hash,quality_report,
		approved_by,approved_at,validation_run_at)
		SELECT $1,$2,project_id,$3,$4,'approved',true,$5,quality_report,
			'authoritative-e2e-fixture',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
		FROM drama.adaptation_plans WHERE adaptation_plan_id=$6`, newPlanID, compilerRunID, newSpecID,
		oldPlanVersion+1, hashText("e2e-plan:"+mergedIRID), oldPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_episode_plans(adaptation_episode_plan_id,
		adaptation_plan_id,episode_number,title,logline,estimated_duration_seconds,opening_hook,ending_hook,
		continuity_in,continuity_out,validation_report,content_hash)
		SELECT $1||'_'||episode_number,$2,episode_number,title,logline,estimated_duration_seconds,opening_hook,
		ending_hook,continuity_in,continuity_out,validation_report,$3
		FROM drama.adaptation_episode_plans WHERE adaptation_plan_id=$4`, "aep_e2e_"+suffix, newPlanID,
		hashText("e2e-episode-plan:"+mergedIRID), oldPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,
		revision_number,content_hash,validity_status,is_current,idempotency_key,metadata)
		SELECT $1||'_'||episode_number,'adaptation_episode_plan',$2,adaptation_episode_plan_id,$3,$4,
		'valid',true,$5||'_'||episode_number,jsonb_build_object('ir_revision_id',$6::text)
		FROM drama.adaptation_episode_plans WHERE adaptation_plan_id=$7`, "artifact_e2e_episode_"+suffix,
		projectID, oldPlanVersion+1, hashText("e2e-episode-plan:"+mergedIRID),
		"e2e:artifact:episode:"+suffix, mergedIRID, newPlanID); err != nil {
		t.Fatal(err)
	}

	var oldPacingID, oldPacingArtifactID string
	var oldPacingVersion int
	if err := tx.QueryRow(ctx, `SELECT current_plan.pacing_plan_id,current_plan.artifact_id,
		(SELECT COALESCE(max(candidate.version_number),0) FROM drama.pacing_plan_versions candidate WHERE candidate.project_id=$1)
		FROM drama.pacing_plan_versions current_plan
		WHERE current_plan.project_id=$1 AND current_plan.status='published' FOR UPDATE`, projectID).
		Scan(&oldPacingID, &oldPacingArtifactID, &oldPacingVersion); err != nil {
		t.Fatal(err)
	}
	newPacingID := "pacing_e2e_" + suffix
	newPacingArtifactID := "artifact_e2e_pacing_" + suffix
	pacingOperationID := "op_e2e_pacing_" + suffix
	if _, err := tx.Exec(ctx, `UPDATE drama.pacing_plan_versions SET status='superseded' WHERE pacing_plan_id=$1`, oldPacingID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.artifacts SET validity_status='superseded',is_current=false
		WHERE artifact_id=$1`, oldPacingArtifactID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,
		status,idempotency_key,input_hash,checkpoint_stage,result_type,result_id,completed_at)
		VALUES($1,$2,'adaptation_compile','project',$3,'completed',$4,$5,'finished','pacing_plan',$6,CURRENT_TIMESTAMP)`,
		pacingOperationID, "trace_e2e_pacing_"+suffix, projectID, "e2e:pacing:operation:"+suffix,
		hashText("e2e-pacing:"+mergedIRID), newPacingID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,
		revision_number,content_hash,validity_status,is_current,idempotency_key,metadata)
		VALUES($1,'pacing_plan',$2,$3,$4,$5,'valid',true,$6,jsonb_build_object('ir_revision_id',$7::text))`,
		newPacingArtifactID, projectID, newPacingID, oldPacingVersion+1, hashText("e2e-pacing:"+mergedIRID),
		"e2e:artifact:pacing:"+suffix, mergedIRID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.pacing_plan_versions(pacing_plan_id,parent_pacing_plan_id,
		operation_id,artifact_id,project_id,source_version_id,ir_revision_id,adaptation_spec_version_id,
		adaptation_plan_id,diagnostic_report_id,version_number,status,analyzer_version,total_duration_seconds,content_hash)
		SELECT $1,pacing_plan_id,$2,$3,project_id,$4,$5,$6,$7,NULL,$8,'published',
		'authoritative-e2e-v1',total_duration_seconds,$9 FROM drama.pacing_plan_versions WHERE pacing_plan_id=$10`,
		newPacingID, pacingOperationID, newPacingArtifactID, sourceVersionID, mergedIRID, newSpecID, newPlanID,
		oldPacingVersion+1, hashText("e2e-pacing:"+mergedIRID), oldPacingID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.pacing_episodes(pacing_episode_id,pacing_plan_id,
		adaptation_episode_plan_id,episode_number,title,conflict_intensity,emotional_intensity,
		information_reveal,hook_strength,estimated_duration_seconds)
		SELECT $1||'_'||old_episode.episode_number,$2,new_episode.adaptation_episode_plan_id,
		old_episode.episode_number,old_episode.title,old_episode.conflict_intensity,old_episode.emotional_intensity,
		old_episode.information_reveal,old_episode.hook_strength,old_episode.estimated_duration_seconds
		FROM drama.pacing_episodes old_episode JOIN drama.adaptation_episode_plans new_episode
		 ON new_episode.adaptation_plan_id=$3 AND new_episode.episode_number=old_episode.episode_number
		WHERE old_episode.pacing_plan_id=$4`, "pace_ep_e2e_"+suffix, newPacingID, newPlanID, oldPacingID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,
		revision_number,content_hash,validity_status,is_current,idempotency_key,metadata)
		SELECT $1||'_'||beat.episode_number||'_'||beat.beat_ordinal,'pacing_beat',$2,
		$3||'_'||beat.episode_number||'_'||beat.beat_ordinal,$4,$5,'valid',true,
		$6||'_'||beat.episode_number||'_'||beat.beat_ordinal,jsonb_build_object('ir_revision_id',$7::text)
		FROM drama.pacing_beats beat WHERE beat.pacing_plan_id=$8`, "artifact_e2e_beat_"+suffix,
		projectID, "pace_beat_e2e_"+suffix, oldPacingVersion+1, hashText("e2e-pacing-beat:"+mergedIRID),
		"e2e:artifact:beat:"+suffix, mergedIRID, oldPacingID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.pacing_beats(pacing_beat_id,pacing_plan_id,pacing_episode_id,
		beat_key,artifact_id,episode_number,beat_ordinal,title,summary,beat_type,source_span_id,fact_revision_id,
		event_revision_id,story_arc_revision_id,conflict_intensity,emotional_intensity,information_reveal,
		hook_strength,reversal_strength,dialogue_ratio,action_ratio,narration_ratio,estimated_duration_seconds,is_manual)
		SELECT $1||'_'||beat.episode_number||'_'||beat.beat_ordinal,$2,
		$3||'_'||beat.episode_number,beat.beat_key,$4||'_'||beat.episode_number||'_'||beat.beat_ordinal,
		beat.episode_number,beat.beat_ordinal,beat.title,beat.summary,beat.beat_type,beat.source_span_id,
		COALESCE(new_fact.fact_revision_id,beat.fact_revision_id),COALESCE(new_event.event_revision_id,beat.event_revision_id),
		COALESCE(new_arc.story_arc_revision_id,beat.story_arc_revision_id),beat.conflict_intensity,
		beat.emotional_intensity,beat.information_reveal,beat.hook_strength,beat.reversal_strength,
		beat.dialogue_ratio,beat.action_ratio,beat.narration_ratio,beat.estimated_duration_seconds,beat.is_manual
		FROM drama.pacing_beats beat
		LEFT JOIN drama.narrative_fact_revisions old_fact ON old_fact.fact_revision_id=beat.fact_revision_id
		LEFT JOIN drama.narrative_fact_revisions new_fact ON new_fact.fact_id=old_fact.fact_id AND new_fact.ir_revision_id=$5
		LEFT JOIN drama.narrative_event_revisions old_event ON old_event.event_revision_id=beat.event_revision_id
		LEFT JOIN drama.narrative_fact_revisions old_event_fact ON old_event_fact.fact_revision_id=old_event.fact_revision_id
		LEFT JOIN drama.narrative_fact_revisions new_event_fact ON new_event_fact.fact_id=old_event_fact.fact_id AND new_event_fact.ir_revision_id=$5
		LEFT JOIN drama.narrative_event_revisions new_event ON new_event.fact_revision_id=new_event_fact.fact_revision_id
		LEFT JOIN drama.story_arc_revisions old_arc ON old_arc.story_arc_revision_id=beat.story_arc_revision_id
		LEFT JOIN drama.story_arc_revisions new_arc ON new_arc.story_arc_id=old_arc.story_arc_id AND new_arc.ir_revision_id=$5
		WHERE beat.pacing_plan_id=$6`, "pace_beat_e2e_"+suffix, newPacingID, "pace_ep_e2e_"+suffix,
		"artifact_e2e_beat_"+suffix, mergedIRID, oldPacingID); err != nil {
		t.Fatal(err)
	}

	if err := tx.Commit(ctx); err != nil {
		if errors.Is(err, pgx.ErrTxCommitRollback) {
			t.Fatalf("merged IR lifecycle successor transaction rolled back: %v", err)
		}
		t.Fatal(err)
	}
	lineage.AdaptationSpecVersionID = newSpecID
	lineage.AdaptationPlanID = newPlanID
	lineage.EpisodePlanID = "aep_e2e_" + suffix + "_1"
	lineage.PacingPlanID = newPacingID
	lineage.SourceBindingID = newSourceBindingID
}
