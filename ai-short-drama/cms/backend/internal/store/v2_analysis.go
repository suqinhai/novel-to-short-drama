package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/adaptationanalysis"
)

type analysisBundle struct {
	input      adaptationanalysis.Input
	diagnostic adaptationanalysis.Diagnostic
	pacing     adaptationanalysis.PacingPlan
	quality    adaptationanalysis.QualityReport
}

func (s *Store) RunAdaptationAnalysis(ctx context.Context, projectID, key string) (Operation, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "adaptation-analysis:"+projectID); err != nil {
		return Operation{}, err
	}
	if replay, found, err := getOperationByIdempotency(ctx, tx, key); err != nil {
		return Operation{}, err
	} else if found {
		if replay.TargetType != "project" || replay.TargetID != projectID || replay.OperationType != "spec_validation" {
			return Operation{}, ErrConflict
		}
		return replay, nil
	}
	input, err := loadAnalysisInput(ctx, tx, projectID)
	if err != nil {
		return Operation{}, err
	}
	diagnostic, pacing, quality := adaptationanalysis.Analyze(input)
	bundle := analysisBundle{input: input, diagnostic: diagnostic, pacing: pacing, quality: quality}
	inputHash, err := hashJSON(map[string]any{
		"project_id": projectID, "source_version_id": input.SourceVersionID, "ir_revision_id": input.IRRevisionID,
		"adaptation_spec_version_id": input.AdaptationSpecVersion, "analyzer_version": adaptationanalysis.AnalyzerVersion,
	})
	if err != nil {
		return Operation{}, err
	}
	reportID, _ := newPublicID("diag_")
	operation, err := insertCompletedAnalysisOperation(ctx, tx, key, projectID, reportID, "adaptation_diagnostic_report", "diagnosis", inputHash)
	if err != nil {
		return Operation{}, err
	}
	diagnosticArtifactID, err := persistDiagnostic(ctx, tx, operation.OperationID, reportID, bundle)
	if err != nil {
		return Operation{}, err
	}
	pacingID, _ := newPublicID("pace_")
	pacingHash, _ := hashJSON(pacing)
	pacingOperation, err := insertCompletedAnalysisOperation(ctx, tx, key+":pacing", projectID, pacingID, "pacing_plan", "pacing", pacingHash)
	if err != nil {
		return Operation{}, err
	}
	pacingArtifactID, beatArtifacts, err := persistPacing(ctx, tx, pacingOperation.OperationID, pacingID, "", diagnosticArtifactID, bundle, nil)
	if err != nil {
		return Operation{}, err
	}
	scoreID, _ := newPublicID("score_")
	scoreHash, _ := hashJSON(quality)
	scoreOperation, err := insertCompletedAnalysisOperation(ctx, tx, key+":quality", projectID, scoreID, "quality_score_report", "quality_score", scoreHash)
	if err != nil {
		return Operation{}, err
	}
	if _, err := persistQuality(ctx, tx, scoreOperation.OperationID, scoreID, "", pacingID, reportID,
		diagnosticArtifactID, pacingArtifactID, beatArtifacts, bundle, "season", json.RawMessage(`{}`)); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	return operation, nil
}

func (s *Store) EditPacing(ctx context.Context, projectID, pacingPlanID, key string, input EditPacingInput) (Operation, error) {
	if len(input.Edits) == 0 || len(input.Edits) > 100 {
		return Operation{}, ErrConflict
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "pacing-edit:"+projectID); err != nil {
		return Operation{}, err
	}
	if replay, found, err := getOperationByIdempotency(ctx, tx, key); err != nil {
		return Operation{}, err
	} else if found {
		if replay.TargetType != "project" || replay.TargetID != projectID {
			return Operation{}, ErrConflict
		}
		return replay, nil
	}
	bundle, oldBeatArtifacts, diagnosticArtifactID, err := loadStoredBundle(ctx, tx, projectID, pacingPlanID)
	if err != nil {
		return Operation{}, err
	}
	edits := make([]adaptationanalysis.BeatEdit, 0, len(input.Edits))
	for _, edit := range input.Edits {
		edits = append(edits, adaptationanalysis.BeatEdit{
			BeatKey: edit.BeatKey, EpisodeNumber: edit.EpisodeNumber, Ordinal: edit.BeatOrdinal,
			EstimatedDuration: edit.EstimatedDurationSeconds,
		})
	}
	nextPacing, changed, err := adaptationanalysis.EditPacing(bundle.pacing, edits)
	if err != nil {
		return Operation{}, ErrConflict
	}
	bundle.pacing = nextPacing
	bundle.quality = adaptationanalysis.Rescore(bundle.input, bundle.diagnostic, nextPacing)
	pacingHash, _ := hashJSON(nextPacing)
	nextPacingID, _ := newPublicID("pace_")
	operation, err := insertCompletedAnalysisOperation(ctx, tx, key, projectID, nextPacingID, "pacing_plan", "pacing_edit", pacingHash)
	if err != nil {
		return Operation{}, err
	}
	pacingArtifactID, beatArtifacts, err := persistPacing(ctx, tx, operation.OperationID, nextPacingID, pacingPlanID,
		diagnosticArtifactID, bundle, oldBeatArtifacts)
	if err != nil {
		return Operation{}, err
	}
	if err := markChangedBeatDownstreamStale(ctx, tx, projectID, oldBeatArtifacts, changed, operation.OperationID); err != nil {
		return Operation{}, err
	}
	scoreID, _ := newPublicID("score_")
	scoreHash, _ := hashJSON(bundle.quality)
	scoreOperation, err := insertCompletedAnalysisOperation(ctx, tx, key+":quality", projectID, scoreID, "quality_score_report", "local_rescore", scoreHash)
	if err != nil {
		return Operation{}, err
	}
	diagnosticID := ""
	if err := tx.QueryRow(ctx, `SELECT diagnostic_report_id FROM drama.adaptation_diagnostic_reports
		WHERE project_id=$1 AND status='completed'`, projectID).Scan(&diagnosticID); err != nil {
		return Operation{}, err
	}
	if _, err := persistQuality(ctx, tx, scoreOperation.OperationID, scoreID, "", nextPacingID, diagnosticID,
		diagnosticArtifactID, pacingArtifactID, beatArtifacts, bundle, "season", json.RawMessage(`{"reason":"beat_edit"}`)); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	return operation, nil
}

func (s *Store) RescoreQuality(ctx context.Context, projectID, key string, input QualityRescoreInput) (Operation, error) {
	if input.Scope == "" {
		input.Scope = "season"
	}
	if input.Scope != "season" && input.Scope != "episode" && input.Scope != "beat" && input.Scope != "artifact" {
		return Operation{}, ErrConflict
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	if replay, found, err := getOperationByIdempotency(ctx, tx, key); err != nil {
		return Operation{}, err
	} else if found {
		if replay.TargetID != projectID {
			return Operation{}, ErrConflict
		}
		return replay, nil
	}
	var pacingPlanID string
	err = tx.QueryRow(ctx, `SELECT pacing_plan_id FROM drama.pacing_plan_versions
		WHERE project_id=$1 AND status='published'`, projectID).Scan(&pacingPlanID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	bundle, beatArtifacts, diagnosticArtifactID, err := loadStoredBundle(ctx, tx, projectID, pacingPlanID)
	if err != nil {
		return Operation{}, err
	}
	bundle.quality = adaptationanalysis.Rescore(bundle.input, bundle.diagnostic, bundle.pacing)
	scoreID, _ := newPublicID("score_")
	scoreHash, _ := hashJSON(map[string]any{"report": bundle.quality, "scope": input.Scope, "selector": input.ScopeSelector})
	operation, err := insertCompletedAnalysisOperation(ctx, tx, key, projectID, scoreID, "quality_score_report", "local_rescore", scoreHash)
	if err != nil {
		return Operation{}, err
	}
	var diagnosticID, pacingArtifactID string
	if err := tx.QueryRow(ctx, `SELECT d.diagnostic_report_id,p.artifact_id
		FROM drama.adaptation_diagnostic_reports d CROSS JOIN drama.pacing_plan_versions p
		WHERE d.project_id=$1 AND d.status='completed' AND p.pacing_plan_id=$2`, projectID, pacingPlanID).
		Scan(&diagnosticID, &pacingArtifactID); err != nil {
		return Operation{}, err
	}
	if _, err := persistQuality(ctx, tx, operation.OperationID, scoreID, "", pacingPlanID, diagnosticID,
		diagnosticArtifactID, pacingArtifactID, beatArtifacts, bundle, input.Scope, input.ScopeSelector); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	return operation, nil
}

func (s *Store) GetLatestDiagnostic(ctx context.Context, projectID string) (json.RawMessage, string, error) {
	var payload json.RawMessage
	var traceID string
	err := s.pool.QueryRow(ctx, `SELECT operation.trace_id,jsonb_build_object(
		'diagnostic_report_id',report.diagnostic_report_id,'version_number',report.version_number,
		'status',report.status,'source_version_id',report.source_version_id,'ir_revision_id',report.ir_revision_id,
		'adaptation_spec_version_id',report.adaptation_spec_version_id,'analyzer_version',report.analyzer_version,
		'core_selling_points',report.core_selling_points,'target_audience',report.target_audience,
		'emotional_value',report.emotional_value,'protagonist_curve',report.protagonist_curve,
		'hook_recommendations',report.hook_recommendations,
		'transformation_recommendations',report.transformation_recommendations,
		'unfilmable_passages',report.unfilmable_passages,'summary',report.summary,
		'nodes',COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'diagnostic_node_id',node.diagnostic_node_id,'node_type',node.node_type,'ordinal',node.ordinal,
			'title',node.title,'description',node.description,'intensity',node.intensity,
			'production_complexity',node.production_complexity,'recommended_action',node.recommended_action,
			'chapter_id',node.chapter_id,'source_span_id',node.source_span_id,'fact_revision_id',node.fact_revision_id,
			'story_arc_revision_id',node.story_arc_revision_id,'evidence',node.evidence
		) ORDER BY node.node_type,node.ordinal) FROM drama.adaptation_diagnostic_nodes node
		WHERE node.diagnostic_report_id=report.diagnostic_report_id),'[]'::jsonb)
	) FROM drama.adaptation_diagnostic_reports report
	JOIN drama.operations operation USING(operation_id)
	WHERE report.project_id=$1 AND report.status='completed'`, projectID).Scan(&traceID, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return payload, traceID, err
}

func (s *Store) GetLatestPacing(ctx context.Context, projectID string) (json.RawMessage, string, error) {
	var payload json.RawMessage
	var traceID string
	err := s.pool.QueryRow(ctx, `SELECT operation.trace_id,jsonb_build_object(
		'pacing_plan_id',plan.pacing_plan_id,'parent_pacing_plan_id',plan.parent_pacing_plan_id,
		'version_number',plan.version_number,'status',plan.status,'analyzer_version',plan.analyzer_version,
		'total_duration_seconds',plan.total_duration_seconds,
		'story_arcs',COALESCE((SELECT jsonb_agg(to_jsonb(arc)-'id'-'created_at'-'pacing_plan_id' ORDER BY arc.ordinal)
			FROM drama.pacing_story_arcs arc WHERE arc.pacing_plan_id=plan.pacing_plan_id),'[]'::jsonb),
		'episodes',COALESCE((SELECT jsonb_agg(to_jsonb(episode)-'id'-'created_at'-'pacing_plan_id' ORDER BY episode.episode_number)
			FROM drama.pacing_episodes episode WHERE episode.pacing_plan_id=plan.pacing_plan_id),'[]'::jsonb),
		'beats',COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'pacing_beat_id',beat.pacing_beat_id,'beat_key',beat.beat_key,'episode_number',beat.episode_number,
			'beat_ordinal',beat.beat_ordinal,'title',beat.title,'summary',beat.summary,'beat_type',beat.beat_type,
			'source_span_id',beat.source_span_id,'fact_revision_id',beat.fact_revision_id,
			'event_revision_id',beat.event_revision_id,'story_arc_revision_id',beat.story_arc_revision_id,
			'conflict_intensity',beat.conflict_intensity,'emotional_intensity',beat.emotional_intensity,
			'information_reveal',beat.information_reveal,'hook_strength',beat.hook_strength,
			'reversal_strength',beat.reversal_strength,'dialogue_ratio',beat.dialogue_ratio,
			'action_ratio',beat.action_ratio,'narration_ratio',beat.narration_ratio,
			'estimated_duration_seconds',beat.estimated_duration_seconds,'is_manual',beat.is_manual
		) ORDER BY beat.episode_number,beat.beat_ordinal) FROM drama.pacing_beats beat
			WHERE beat.pacing_plan_id=plan.pacing_plan_id),'[]'::jsonb),
		'issues',COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'pacing_issue_id',issue.pacing_issue_id,'pacing_beat_id',issue.pacing_beat_id,
			'episode_number',issue.episode_number,'issue_code',issue.issue_code,'severity',issue.severity,
			'location',issue.location,'message',issue.message,'suggestion',issue.suggestion
		) ORDER BY issue.id) FROM drama.pacing_issues issue WHERE issue.pacing_plan_id=plan.pacing_plan_id),'[]'::jsonb)
	) FROM drama.pacing_plan_versions plan JOIN drama.operations operation USING(operation_id)
	WHERE plan.project_id=$1 AND plan.status='published'`, projectID).Scan(&traceID, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return payload, traceID, err
}

func (s *Store) GetLatestQualityScore(ctx context.Context, projectID string) (json.RawMessage, string, error) {
	var payload json.RawMessage
	var traceID string
	err := s.pool.QueryRow(ctx, `SELECT operation.trace_id,jsonb_build_object(
		'quality_score_report_id',report.quality_score_report_id,'version_number',report.version_number,
		'status',report.status,'scope',report.scope,'scope_selector',report.scope_selector,
		'analyzer_version',report.analyzer_version,'total_score',report.total_score,
		'dimensions',COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'quality_score_dimension_id',dimension.quality_score_dimension_id,'dimension',dimension.dimension,
			'score',dimension.score,'weight',dimension.weight,'evidence',dimension.evidence,
			'issue_count',dimension.issue_count,
			'issues',COALESCE((SELECT jsonb_agg(jsonb_build_object(
				'quality_issue_id',issue.quality_issue_id,'severity',issue.severity,
				'episode_number',issue.episode_number,'pacing_beat_id',issue.pacing_beat_id,
				'artifact_id',issue.artifact_id,'source_span_id',issue.source_span_id,
				'fact_revision_id',issue.fact_revision_id,'location',issue.location,
				'evidence',issue.evidence,'message',issue.message,'suggestion',issue.suggestion
			) ORDER BY issue.id) FROM drama.quality_issues issue
			WHERE issue.quality_score_report_id=report.quality_score_report_id
			  AND issue.dimension=dimension.dimension),'[]'::jsonb)
		) ORDER BY dimension.id) FROM drama.quality_score_dimensions dimension
		WHERE dimension.quality_score_report_id=report.quality_score_report_id),'[]'::jsonb)
	) FROM drama.quality_score_reports report JOIN drama.operations operation USING(operation_id)
	WHERE report.project_id=$1 AND report.status='completed'`, projectID).Scan(&traceID, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return payload, traceID, err
}

func loadAnalysisInput(ctx context.Context, tx pgx.Tx, projectID string) (adaptationanalysis.Input, error) {
	var input adaptationanalysis.Input
	input.ProjectID = projectID
	var adaptationPlanID *string
	err := tx.QueryRow(ctx, `SELECT binding.source_version_id,ir.ir_revision_id,
		COALESCE(spec.adaptation_spec_version_id,''),COALESCE(spec.target_episode_count,project.target_episode_count),
		COALESCE(spec.episode_duration_seconds,project.episode_duration_seconds),plan.adaptation_plan_id
		FROM drama.projects project
		JOIN drama.project_source_bindings binding ON binding.project_id=project.project_id
		  AND binding.binding_role='primary' AND binding.is_current
		JOIN drama.narrative_ir_revisions ir ON ir.source_version_id=binding.source_version_id
		  AND ir.status='published' AND ir.revision_scope='full' AND ir.is_current
		LEFT JOIN drama.adaptation_spec_versions spec ON spec.adaptation_spec_version_id=project.current_adaptation_spec_version_id
		LEFT JOIN LATERAL(SELECT adaptation_plan_id FROM drama.adaptation_plans
		  WHERE project_id=project.project_id AND is_current ORDER BY version_number DESC LIMIT 1) plan ON true
		WHERE project.project_id=$1`, projectID).Scan(&input.SourceVersionID, &input.IRRevisionID,
		&input.AdaptationSpecVersion, &input.TargetEpisodeCount, &input.EpisodeDuration, &adaptationPlanID)
	if errors.Is(err, pgx.ErrNoRows) {
		return input, ErrNotFound
	}
	if err != nil {
		return input, err
	}
	rows, err := tx.Query(ctx, `SELECT membership.chapter_id,membership.ordinal,revision.title,revision.content,
		COALESCE(span.source_span_id,''),revision.chapter_revision_id
		FROM drama.source_version_chapters membership
		JOIN drama.chapter_revisions revision USING(chapter_revision_id)
		LEFT JOIN LATERAL(SELECT source_span_id FROM drama.source_spans span
		  WHERE span.source_version_id=membership.source_version_id AND span.chapter_id=membership.chapter_id
		  ORDER BY (span.end_codepoint-span.start_codepoint) DESC LIMIT 1) span ON true
		WHERE membership.source_version_id=$1 ORDER BY membership.ordinal`, input.SourceVersionID)
	if err != nil {
		return input, err
	}
	for rows.Next() {
		var chapter adaptationanalysis.Chapter
		if err := rows.Scan(&chapter.ID, &chapter.Ordinal, &chapter.Title, &chapter.Content, &chapter.SpanID, &chapter.Revision); err != nil {
			rows.Close()
			return input, err
		}
		input.Chapters = append(input.Chapters, chapter)
	}
	rows.Close()
	eventRows, err := tx.Query(ctx, `SELECT event.event_revision_id,event.fact_revision_id,fact.chapter_id,
		fact.primary_source_span_id,COALESCE(arc.story_arc_revision_id,''),event.summary,event.event_type,
		event.importance::float8,event.narrative_order::float8
		FROM drama.narrative_event_revisions event
		JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
		LEFT JOIN drama.story_arc_events arc_event ON arc_event.event_revision_id=event.event_revision_id
		LEFT JOIN drama.story_arc_revisions arc ON arc.story_arc_revision_id=arc_event.story_arc_revision_id
		WHERE event.ir_revision_id=$1 ORDER BY event.narrative_order,event.event_revision_id`, input.IRRevisionID)
	if err != nil {
		return input, err
	}
	for eventRows.Next() {
		var event adaptationanalysis.Event
		if err := eventRows.Scan(&event.EventRevisionID, &event.FactRevisionID, &event.ChapterID,
			&event.SourceSpanID, &event.StoryArcRevisionID, &event.Summary, &event.EventType,
			&event.Importance, &event.NarrativeOrder); err != nil {
			eventRows.Close()
			return input, err
		}
		input.Events = append(input.Events, event)
	}
	eventRows.Close()
	arcRows, err := tx.Query(ctx, `SELECT story_arc_revision_id,title,summary,arc_type
		FROM drama.story_arc_revisions WHERE ir_revision_id=$1 ORDER BY created_at,story_arc_revision_id`, input.IRRevisionID)
	if err != nil {
		return input, err
	}
	for arcRows.Next() {
		var arc adaptationanalysis.StoryArc
		if err := arcRows.Scan(&arc.StoryArcRevisionID, &arc.Title, &arc.Summary, &arc.ArcType); err != nil {
			arcRows.Close()
			return input, err
		}
		input.StoryArcs = append(input.StoryArcs, arc)
	}
	arcRows.Close()
	return input, nil
}

func insertCompletedAnalysisOperation(ctx context.Context, tx pgx.Tx, key, projectID, resultID, resultType, kind, inputHash string) (Operation, error) {
	operationID, _ := newPublicID("op_")
	traceID, _ := newPublicID("tr_")
	checkpoint := mustJSON(map[string]any{"stage": "finished", "analysis_kind": kind, "analyzer_version": adaptationanalysis.AnalyzerVersion})
	_, err := tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,
		idempotency_key,input_hash,checkpoint_stage,checkpoint_data,result_type,result_id,completed_at)
		VALUES($1,$2,'spec_validation','project',$3,'completed',$4,$5,'finished',$6,$7,$8,CURRENT_TIMESTAMP)`,
		operationID, traceID, projectID, key, inputHash, checkpoint, resultType, resultID)
	if err != nil {
		return Operation{}, mapPGConflict(err)
	}
	return scanOperation(tx.QueryRow(ctx, operationSelect+` WHERE operation_id=$1`, operationID))
}

func persistDiagnostic(ctx context.Context, tx pgx.Tx, operationID, reportID string, bundle analysisBundle) (string, error) {
	if _, err := tx.Exec(ctx, `UPDATE drama.adaptation_diagnostic_reports SET status='superseded'
		WHERE project_id=$1 AND status='completed'`, bundle.input.ProjectID); err != nil {
		return "", err
	}
	contentHash, _ := hashJSON(bundle.diagnostic)
	artifactID, _, err := createArtifactRevision(ctx, tx, bundle.input.ProjectID, "adaptation_diagnostic_report", reportID, contentHash)
	if err != nil {
		return "", err
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version_number),0)+1 FROM drama.adaptation_diagnostic_reports
		WHERE project_id=$1`, bundle.input.ProjectID).Scan(&version); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_diagnostic_reports(
		diagnostic_report_id,operation_id,artifact_id,project_id,source_version_id,ir_revision_id,
		adaptation_spec_version_id,version_number,analyzer_version,core_selling_points,target_audience,
		emotional_value,protagonist_curve,hook_recommendations,transformation_recommendations,
		unfilmable_passages,summary,content_hash)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		reportID, operationID, artifactID, bundle.input.ProjectID, bundle.input.SourceVersionID,
		bundle.input.IRRevisionID, bundle.input.AdaptationSpecVersion, version, bundle.diagnostic.AnalyzerVersion,
		mustJSON(bundle.diagnostic.CoreSellingPoints), mustJSON(bundle.diagnostic.TargetAudience),
		mustJSON(bundle.diagnostic.EmotionalValue), mustJSON(bundle.diagnostic.ProtagonistCurve),
		mustJSON(bundle.diagnostic.HookRecommendations), mustJSON(bundle.diagnostic.TransformationRecommendations),
		mustJSON(bundle.diagnostic.UnfilmablePassages), mustJSON(bundle.diagnostic.Summary), contentHash); err != nil {
		return "", mapPGConflict(err)
	}
	for index, node := range bundle.diagnostic.Nodes {
		nodeID, _ := newPublicID("diagnode_")
		action := nullableString(node.RecommendedAction)
		if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_diagnostic_nodes(
			diagnostic_node_id,diagnostic_report_id,node_type,chapter_id,source_span_id,fact_revision_id,
			story_arc_revision_id,ordinal,title,description,intensity,production_complexity,recommended_action,evidence)
			VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14)`,
			nodeID, reportID, node.NodeType, node.Evidence.ChapterID, node.Evidence.SourceSpanID,
			node.Evidence.FactRevisionID, node.Evidence.StoryArcRevisionID, node.Ordinal, node.Title,
			node.Description, node.Intensity, node.ProductionComplexity, action, mustJSON(node)); err != nil {
			return "", mapPGConflict(err)
		}
		if node.Evidence.SourceSpanID != "" || node.Evidence.FactRevisionID != "" {
			evidenceID, _ := newPublicID("ase_")
			if _, err := tx.Exec(ctx, `INSERT INTO drama.artifact_source_evidence(
				artifact_source_evidence_id,artifact_id,source_span_id,fact_revision_id,evidence_role,idempotency_key)
				VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),'source',$5)`,
				evidenceID, artifactID, node.Evidence.SourceSpanID, node.Evidence.FactRevisionID,
				fmt.Sprintf("analysis:%s:evidence:%d", reportID, index)); err != nil {
				return "", mapPGConflict(err)
			}
		}
	}
	upstreams, err := ensureAnalysisUpstreamArtifacts(ctx, tx, bundle.input)
	if err != nil {
		return "", err
	}
	for index, upstreamID := range upstreams {
		if err := createDependency(ctx, tx, upstreamID, artifactID, "diagnosis_input", fmt.Sprintf("analysis:%s:input:%d", reportID, index)); err != nil {
			return "", err
		}
	}
	return artifactID, nil
}

func persistPacing(ctx context.Context, tx pgx.Tx, operationID, pacingID, parentID, diagnosticArtifactID string,
	bundle analysisBundle, reusable map[string]string) (string, map[string]string, error) {
	if _, err := tx.Exec(ctx, `UPDATE drama.pacing_plan_versions SET status='superseded'
		WHERE project_id=$1 AND status='published'`, bundle.input.ProjectID); err != nil {
		return "", nil, err
	}
	contentHash, _ := hashJSON(bundle.pacing)
	artifactID, _, err := createArtifactRevision(ctx, tx, bundle.input.ProjectID, "pacing_plan", pacingID, contentHash)
	if err != nil {
		return "", nil, err
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version_number),0)+1 FROM drama.pacing_plan_versions
		WHERE project_id=$1`, bundle.input.ProjectID).Scan(&version); err != nil {
		return "", nil, err
	}
	var diagnosticID string
	if err := tx.QueryRow(ctx, `SELECT diagnostic_report_id FROM drama.adaptation_diagnostic_reports
		WHERE project_id=$1 AND status='completed'`, bundle.input.ProjectID).Scan(&diagnosticID); err != nil {
		return "", nil, err
	}
	var adaptationPlanID *string
	_ = tx.QueryRow(ctx, `SELECT adaptation_plan_id FROM drama.adaptation_plans
		WHERE project_id=$1 AND is_current ORDER BY version_number DESC LIMIT 1`, bundle.input.ProjectID).Scan(&adaptationPlanID)
	if _, err := tx.Exec(ctx, `INSERT INTO drama.pacing_plan_versions(
		pacing_plan_id,parent_pacing_plan_id,operation_id,artifact_id,project_id,source_version_id,
		ir_revision_id,adaptation_spec_version_id,adaptation_plan_id,diagnostic_report_id,version_number,
		analyzer_version,total_duration_seconds,content_hash)
		VALUES($1,NULLIF($2,''),$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13,$14)`,
		pacingID, parentID, operationID, artifactID, bundle.input.ProjectID, bundle.input.SourceVersionID,
		bundle.input.IRRevisionID, bundle.input.AdaptationSpecVersion, adaptationPlanID, diagnosticID, version,
		bundle.pacing.AnalyzerVersion, bundle.pacing.TotalDuration, contentHash); err != nil {
		return "", nil, mapPGConflict(err)
	}
	for _, arc := range bundle.pacing.Arcs {
		arcID, _ := newPublicID("pacearc_")
		if _, err := tx.Exec(ctx, `INSERT INTO drama.pacing_story_arcs(
			pacing_story_arc_id,pacing_plan_id,story_arc_revision_id,ordinal,title,conflict_intensity,
			emotional_intensity,information_reveal,estimated_duration_seconds)
			VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9)`, arcID, pacingID, arc.StoryArcRevisionID,
			arc.Ordinal, arc.Title, arc.ConflictIntensity, arc.EmotionalIntensity, arc.InformationReveal,
			arc.EstimatedDuration); err != nil {
			return "", nil, mapPGConflict(err)
		}
	}
	episodeIDs := map[int]string{}
	adaptationEpisodeIDs := map[int]string{}
	if adaptationPlanID != nil {
		rows, err := tx.Query(ctx, `SELECT episode_number,adaptation_episode_plan_id FROM drama.adaptation_episode_plans
			WHERE adaptation_plan_id=$1`, *adaptationPlanID)
		if err != nil {
			return "", nil, err
		}
		for rows.Next() {
			var number int
			var id string
			if err := rows.Scan(&number, &id); err != nil {
				rows.Close()
				return "", nil, err
			}
			adaptationEpisodeIDs[number] = id
		}
		rows.Close()
	}
	for _, episode := range bundle.pacing.Episodes {
		episodeID, _ := newPublicID("paceep_")
		episodeIDs[episode.EpisodeNumber] = episodeID
		if _, err := tx.Exec(ctx, `INSERT INTO drama.pacing_episodes(
			pacing_episode_id,pacing_plan_id,adaptation_episode_plan_id,episode_number,title,
			conflict_intensity,emotional_intensity,information_reveal,hook_strength,estimated_duration_seconds)
			VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10)`, episodeID, pacingID,
			adaptationEpisodeIDs[episode.EpisodeNumber], episode.EpisodeNumber, episode.Title,
			episode.ConflictIntensity, episode.EmotionalIntensity, episode.InformationReveal,
			episode.HookStrength, episode.EstimatedDuration); err != nil {
			return "", nil, mapPGConflict(err)
		}
	}
	beatArtifacts := map[string]string{}
	beatIDs := map[string]string{}
	for _, beat := range bundle.pacing.Beats {
		beatHash, _ := hashJSON(beat)
		beatArtifactID := ""
		if reusable != nil && reusable[beat.Key] != "" {
			var priorHash string
			if err := tx.QueryRow(ctx, `SELECT content_hash FROM drama.artifacts WHERE artifact_id=$1`,
				reusable[beat.Key]).Scan(&priorHash); err != nil {
				return "", nil, err
			}
			if priorHash == beatHash {
				beatArtifactID = reusable[beat.Key]
			}
		}
		if beatArtifactID == "" {
			beatArtifactID, _, err = createArtifactRevision(ctx, tx, bundle.input.ProjectID, "pacing_beat", beat.Key, beatHash)
			if err != nil {
				return "", nil, err
			}
		}
		beatArtifacts[beat.Key] = beatArtifactID
		beatID, _ := newPublicID("beat_")
		beatIDs[beat.Key] = beatID
		if _, err := tx.Exec(ctx, `INSERT INTO drama.pacing_beats(
			pacing_beat_id,pacing_plan_id,pacing_episode_id,beat_key,artifact_id,episode_number,beat_ordinal,
			title,summary,beat_type,source_span_id,fact_revision_id,event_revision_id,story_arc_revision_id,
			conflict_intensity,emotional_intensity,information_reveal,hook_strength,reversal_strength,
			dialogue_ratio,action_ratio,narration_ratio,estimated_duration_seconds,is_manual)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),
			NULLIF($14,''),$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)`,
			beatID, pacingID, episodeIDs[beat.EpisodeNumber], beat.Key, beatArtifactID, beat.EpisodeNumber,
			beat.Ordinal, beat.Title, beat.Summary, beat.Type, beat.Evidence.SourceSpanID, beat.Evidence.FactRevisionID,
			eventIDFromBeat(bundle.input.Events, beat.Key), beat.Evidence.StoryArcRevisionID, beat.ConflictIntensity,
			beat.EmotionalIntensity, beat.InformationReveal, beat.HookStrength, beat.ReversalStrength,
			beat.DialogueRatio, beat.ActionRatio, beat.NarrationRatio, beat.EstimatedDuration, beat.Manual); err != nil {
			return "", nil, mapPGConflict(err)
		}
		if err := createDependency(ctx, tx, diagnosticArtifactID, beatArtifactID, "diagnosis_to_beat",
			"analysis:"+pacingID+":beat:"+beat.Key); err != nil {
			return "", nil, err
		}
		if adaptationEpisodeID := adaptationEpisodeIDs[beat.EpisodeNumber]; adaptationEpisodeID != "" {
			episodeArtifactID, err := ensureNativeArtifact(ctx, tx, bundle.input.ProjectID, "adaptation_episode_plan", adaptationEpisodeID)
			if err != nil {
				return "", nil, err
			}
			if err := createDependency(ctx, tx, beatArtifactID, episodeArtifactID, "pacing_controls_episode",
				"analysis:"+pacingID+":episode:"+fmt.Sprint(beat.EpisodeNumber)+":"+beat.Key); err != nil {
				return "", nil, err
			}
		}
		if err := createDependency(ctx, tx, beatArtifactID, artifactID, "beat_member",
			"analysis:"+pacingID+":member:"+beat.Key); err != nil {
			return "", nil, err
		}
	}
	for _, issue := range bundle.pacing.Issues {
		issueID, _ := newPublicID("paceissue_")
		if _, err := tx.Exec(ctx, `INSERT INTO drama.pacing_issues(
			pacing_issue_id,pacing_plan_id,pacing_beat_id,episode_number,issue_code,severity,
			location,message,suggestion) VALUES($1,$2,NULLIF($3,''),NULLIF($4,0),$5,$6,$7,$8,$9)`,
			issueID, pacingID, beatIDs[issue.BeatKey], issue.EpisodeNumber, issue.Code, issue.Severity,
			mustJSON(issue.Location), issue.Message, issue.Suggestion); err != nil {
			return "", nil, mapPGConflict(err)
		}
	}
	if err := createDependency(ctx, tx, diagnosticArtifactID, artifactID, "diagnosis_to_pacing",
		"analysis:"+pacingID+":diagnosis"); err != nil {
		return "", nil, err
	}
	return artifactID, beatArtifacts, nil
}

func persistQuality(ctx context.Context, tx pgx.Tx, operationID, scoreID, parentID, pacingID, diagnosticID,
	diagnosticArtifactID, pacingArtifactID string, beatArtifacts map[string]string, bundle analysisBundle,
	scope string, selector json.RawMessage) (string, error) {
	if _, err := tx.Exec(ctx, `UPDATE drama.quality_score_reports SET status='superseded'
		WHERE project_id=$1 AND status='completed'`, bundle.input.ProjectID); err != nil {
		return "", err
	}
	contentHash, _ := hashJSON(map[string]any{"report": bundle.quality, "scope": scope, "selector": selector})
	artifactID, _, err := createArtifactRevision(ctx, tx, bundle.input.ProjectID, "quality_score_report", scoreID, contentHash)
	if err != nil {
		return "", err
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version_number),0)+1 FROM drama.quality_score_reports
		WHERE project_id=$1`, bundle.input.ProjectID).Scan(&version); err != nil {
		return "", err
	}
	if len(selector) == 0 {
		selector = json.RawMessage(`{}`)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.quality_score_reports(
		quality_score_report_id,operation_id,artifact_id,project_id,source_version_id,ir_revision_id,
		adaptation_spec_version_id,pacing_plan_id,diagnostic_report_id,parent_quality_score_report_id,
		version_number,scope,scope_selector,analyzer_version,total_score,content_hash)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,NULLIF($10,''),$11,$12,$13,$14,$15,$16)`,
		scoreID, operationID, artifactID, bundle.input.ProjectID, bundle.input.SourceVersionID,
		bundle.input.IRRevisionID, bundle.input.AdaptationSpecVersion, pacingID, diagnosticID, parentID,
		version, scope, selector, bundle.quality.AnalyzerVersion, bundle.quality.TotalScore, contentHash); err != nil {
		return "", mapPGConflict(err)
	}
	for dimensionIndex, dimension := range bundle.quality.Dimensions {
		dimensionID, _ := newPublicID("scoredim_")
		if _, err := tx.Exec(ctx, `INSERT INTO drama.quality_score_dimensions(
			quality_score_dimension_id,quality_score_report_id,dimension,score,weight,evidence,issue_count)
			VALUES($1,$2,$3,$4,$5,$6,$7)`, dimensionID, scoreID, dimension.Dimension, dimension.Score,
			dimension.Weight, mustJSON(dimension.Evidence), len(dimension.Issues)); err != nil {
			return "", mapPGConflict(err)
		}
		for issueIndex, issue := range dimension.Issues {
			issueID, _ := newPublicID("scoreissue_")
			var pacingBeatID *string
			if issue.BeatKey != "" {
				var id string
				if err := tx.QueryRow(ctx, `SELECT pacing_beat_id FROM drama.pacing_beats
					WHERE pacing_plan_id=$1 AND beat_key=$2`, pacingID, issue.BeatKey).Scan(&id); err == nil {
					pacingBeatID = &id
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO drama.quality_issues(
				quality_issue_id,quality_score_report_id,dimension,severity,episode_number,pacing_beat_id,
				artifact_id,source_span_id,fact_revision_id,location,evidence,message,suggestion)
				VALUES($1,$2,$3,$4,NULLIF($5,0),$6,$7,NULLIF($8,''),NULLIF($9,''),$10,$11,$12,$13)`,
				issueID, scoreID, dimension.Dimension, issue.Severity, issue.EpisodeNumber, pacingBeatID,
				nullableArtifact(beatArtifacts[issue.BeatKey]), issue.Evidence.SourceSpanID, issue.Evidence.FactRevisionID,
				mustJSON(issue.Location), issue.Evidence.Excerpt, issue.Message, issue.Suggestion); err != nil {
				return "", mapPGConflict(err)
			}
			_ = issueIndex
		}
		_ = dimensionIndex
	}
	for index, upstream := range append([]string{diagnosticArtifactID, pacingArtifactID}, sortedMapValues(beatArtifacts)...) {
		if upstream == "" {
			continue
		}
		if err := createDependency(ctx, tx, upstream, artifactID, "quality_input",
			fmt.Sprintf("analysis:%s:quality-input:%d", scoreID, index)); err != nil {
			return "", err
		}
	}
	return artifactID, nil
}

func loadStoredBundle(ctx context.Context, tx pgx.Tx, projectID, pacingPlanID string) (analysisBundle, map[string]string, string, error) {
	input, err := loadAnalysisInput(ctx, tx, projectID)
	if err != nil {
		return analysisBundle{}, nil, "", err
	}
	diagnostic, _, _ := adaptationanalysis.Analyze(input)
	var planProject string
	err = tx.QueryRow(ctx, `SELECT project_id
		FROM drama.pacing_plan_versions WHERE pacing_plan_id=$1`, pacingPlanID).Scan(&planProject)
	if err != nil {
		// The target validation below uses a dedicated query; avoid leaking row details.
		var exists bool
		_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.pacing_plan_versions WHERE pacing_plan_id=$1)`, pacingPlanID).Scan(&exists)
		if !exists {
			return analysisBundle{}, nil, "", ErrNotFound
		}
	}
	if planProject != "" && planProject != projectID {
		return analysisBundle{}, nil, "", ErrNotFound
	}
	var pacing adaptationanalysis.PacingPlan
	pacing.AnalyzerVersion = adaptationanalysis.AnalyzerVersion
	rows, err := tx.Query(ctx, `SELECT beat.beat_key,beat.episode_number,beat.beat_ordinal,beat.title,beat.summary,
		beat.beat_type,COALESCE(fact.chapter_id,''),COALESCE(beat.source_span_id,''),COALESCE(beat.fact_revision_id,''),
		COALESCE(beat.story_arc_revision_id,''),beat.conflict_intensity::float8,beat.emotional_intensity::float8,
		beat.information_reveal::float8,beat.hook_strength::float8,beat.reversal_strength::float8,
		beat.dialogue_ratio::float8,beat.action_ratio::float8,beat.narration_ratio::float8,
		beat.estimated_duration_seconds,beat.is_manual,beat.artifact_id
		FROM drama.pacing_beats beat LEFT JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
		WHERE beat.pacing_plan_id=$1 ORDER BY beat.episode_number,beat.beat_ordinal`, pacingPlanID)
	if err != nil {
		return analysisBundle{}, nil, "", err
	}
	beatArtifacts := map[string]string{}
	for rows.Next() {
		var beat adaptationanalysis.Beat
		var artifactID string
		if err := rows.Scan(&beat.Key, &beat.EpisodeNumber, &beat.Ordinal, &beat.Title, &beat.Summary, &beat.Type,
			&beat.Evidence.ChapterID, &beat.Evidence.SourceSpanID, &beat.Evidence.FactRevisionID,
			&beat.Evidence.StoryArcRevisionID, &beat.ConflictIntensity, &beat.EmotionalIntensity,
			&beat.InformationReveal, &beat.HookStrength, &beat.ReversalStrength, &beat.DialogueRatio,
			&beat.ActionRatio, &beat.NarrationRatio, &beat.EstimatedDuration, &beat.Manual, &artifactID); err != nil {
			rows.Close()
			return analysisBundle{}, nil, "", err
		}
		beat.Evidence.Excerpt = beat.Summary
		pacing.Beats = append(pacing.Beats, beat)
		beatArtifacts[beat.Key] = artifactID
	}
	rows.Close()
	// Rebuild derived curves and issues from the immutable beat snapshot.
	pacing, _, err = adaptationanalysis.EditPacing(pacing, nil)
	if err != nil {
		return analysisBundle{}, nil, "", err
	}
	var diagnosticArtifactID string
	if err := tx.QueryRow(ctx, `SELECT artifact_id FROM drama.adaptation_diagnostic_reports
		WHERE project_id=$1 AND status='completed'`, projectID).Scan(&diagnosticArtifactID); err != nil {
		return analysisBundle{}, nil, "", err
	}
	return analysisBundle{input: input, diagnostic: diagnostic, pacing: pacing}, beatArtifacts, diagnosticArtifactID, nil
}

func createArtifactRevision(ctx context.Context, tx pgx.Tx, projectID, artifactType, nativeID, contentHash string) (string, int, error) {
	var revision int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision_number),0)+1 FROM drama.artifacts
		WHERE project_id=$1 AND artifact_type=$2 AND native_entity_id=$3`, projectID, artifactType, nativeID).Scan(&revision); err != nil {
		return "", 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.artifacts SET is_current=false,validity_status='superseded'
		WHERE project_id=$1 AND artifact_type=$2 AND native_entity_id=$3 AND is_current`,
		projectID, artifactType, nativeID); err != nil {
		return "", 0, err
	}
	artifactID, _ := newPublicID("artifact_")
	if _, err := tx.Exec(ctx, `INSERT INTO drama.artifacts(
		artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
		validity_status,is_current,idempotency_key,metadata)
		VALUES($1,$2,$3,$4,$5,$6,'valid',true,$7,$8)`, artifactID, artifactType, projectID,
		nativeID, revision, contentHash, fmt.Sprintf("analysis:%s:%s:%d", artifactType, nativeID, revision),
		mustJSON(map[string]any{"analyzer_version": adaptationanalysis.AnalyzerVersion})); err != nil {
		return "", 0, mapPGConflict(err)
	}
	return artifactID, revision, nil
}

func ensureAnalysisUpstreamArtifacts(ctx context.Context, tx pgx.Tx, input adaptationanalysis.Input) ([]string, error) {
	ids := []string{}
	sourceID, err := ensureNativeArtifact(ctx, tx, "", "source_version", input.SourceVersionID)
	if err != nil {
		return nil, err
	}
	ids = append(ids, sourceID)
	if input.AdaptationSpecVersion != "" {
		specID, err := ensureNativeArtifact(ctx, tx, input.ProjectID, "adaptation_spec_version", input.AdaptationSpecVersion)
		if err != nil {
			return nil, err
		}
		ids = append(ids, specID)
	}
	for _, event := range input.Events {
		factID, err := ensureNativeArtifact(ctx, tx, "", "narrative_fact_revision", event.FactRevisionID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, factID)
	}
	for _, arc := range input.StoryArcs {
		arcID, err := ensureNativeArtifact(ctx, tx, "", "story_arc_revision", arc.StoryArcRevisionID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, arcID)
	}
	sort.Strings(ids)
	return uniqueStrings(ids), nil
}

func ensureNativeArtifact(ctx context.Context, tx pgx.Tx, projectID, artifactType, nativeID string) (string, error) {
	var artifactID string
	err := tx.QueryRow(ctx, `SELECT artifact_id FROM drama.artifacts
		WHERE project_id IS NOT DISTINCT FROM NULLIF($1,'') AND artifact_type=$2 AND native_entity_id=$3 AND is_current
		ORDER BY revision_number DESC LIMIT 1`, projectID, artifactType, nativeID).Scan(&artifactID)
	if err == nil {
		return artifactID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	contentHash := hashString(artifactType + "|" + nativeID)
	switch artifactType {
	case "source_version":
		_ = tx.QueryRow(ctx, `SELECT version_hash FROM drama.source_versions WHERE source_version_id=$1`, nativeID).Scan(&contentHash)
	case "narrative_fact_revision":
		_ = tx.QueryRow(ctx, `SELECT canonical_fingerprint FROM drama.narrative_fact_revisions WHERE fact_revision_id=$1`, nativeID).Scan(&contentHash)
	case "adaptation_episode_plan":
		_ = tx.QueryRow(ctx, `SELECT content_hash FROM drama.adaptation_episode_plans WHERE adaptation_episode_plan_id=$1`, nativeID).Scan(&contentHash)
	case "adaptation_spec_version":
		_ = tx.QueryRow(ctx, `SELECT content_hash FROM drama.adaptation_spec_versions WHERE adaptation_spec_version_id=$1`, nativeID).Scan(&contentHash)
	}
	artifactID, _ = newPublicID("artifact_")
	_, err = tx.Exec(ctx, `INSERT INTO drama.artifacts(
		artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
		validity_status,is_current,idempotency_key,metadata)
		VALUES($1,$2,NULLIF($3,''),$4,1,$5,'valid',true,$6,'{}'::jsonb)`,
		artifactID, artifactType, projectID, nativeID, contentHash, "analysis:upstream:"+artifactType+":"+nativeID)
	return artifactID, mapPGConflict(err)
}

func createDependency(ctx context.Context, tx pgx.Tx, upstreamID, downstreamID, dependencyType, key string) error {
	var hash string
	if err := tx.QueryRow(ctx, `SELECT content_hash FROM drama.artifacts WHERE artifact_id=$1`, upstreamID).Scan(&hash); err != nil {
		return err
	}
	dependencyID, _ := newPublicID("adep_")
	_, err := tx.Exec(ctx, `INSERT INTO drama.artifact_dependencies(
		artifact_dependency_id,upstream_artifact_id,downstream_artifact_id,dependency_type,
		dependency_selector,observed_upstream_hash,invalidates_on,idempotency_key)
		VALUES($1,$2,$3,$4,'{}'::jsonb,$5,'["content_changed","removed","dependency_changed"]'::jsonb,$6)
		ON CONFLICT(upstream_artifact_id,downstream_artifact_id,dependency_type) DO NOTHING`,
		dependencyID, upstreamID, downstreamID, dependencyType, hash, key)
	return mapPGConflict(err)
}

func markChangedBeatDownstreamStale(ctx context.Context, tx pgx.Tx, projectID string, oldArtifacts map[string]string,
	changed []string, operationID string) error {
	roots := []string{}
	for _, key := range changed {
		if oldArtifacts[key] != "" {
			roots = append(roots, oldArtifacts[key])
		}
	}
	if len(roots) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE impacted(artifact_id,depth,path) AS (
		SELECT dependency.downstream_artifact_id,1,ARRAY[dependency.upstream_artifact_id,dependency.downstream_artifact_id]
		FROM drama.artifact_dependencies dependency
		WHERE dependency.upstream_artifact_id=ANY($1)
		UNION ALL
		SELECT dependency.downstream_artifact_id,impacted.depth+1,impacted.path||dependency.downstream_artifact_id
		FROM impacted JOIN drama.artifact_dependencies dependency
		  ON dependency.upstream_artifact_id=impacted.artifact_id
		WHERE impacted.depth<32 AND NOT dependency.downstream_artifact_id=ANY(impacted.path)
	)
	SELECT artifact_id,min(depth) FROM impacted GROUP BY artifact_id`, roots)
	if err != nil {
		return err
	}
	type impact struct {
		id    string
		depth int
	}
	impacts := []impact{}
	for rows.Next() {
		var item impact
		if err := rows.Scan(&item.id, &item.depth); err != nil {
			rows.Close()
			return err
		}
		impacts = append(impacts, item)
	}
	rows.Close()
	if len(impacts) == 0 {
		return nil
	}
	invalidationOperationID, _ := newPublicID("op_")
	traceID, _ := newPublicID("tr_")
	taskID, _ := newPublicID("inv_")
	rootHash := hashString(fmt.Sprint(roots))
	if _, err := tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,
		idempotency_key,input_hash,checkpoint_stage,checkpoint_data,result_type,result_id,completed_at)
		VALUES($1,$2,'invalidation_scan','project',$3,'completed',$4,$5,'finished',$6,'invalidation_task',$7,CURRENT_TIMESTAMP)`,
		invalidationOperationID, traceID, projectID, "pacing-edit-invalidation:"+operationID, rootHash,
		mustJSON(map[string]any{"analysis_kind": "pacing_edit", "changed_beat_artifact_ids": roots}),
		taskID); err != nil {
		return mapPGConflict(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.invalidation_tasks(
		invalidation_task_id,operation_id,project_id,root_artifact_id,status,reason_type,
		idempotency_key,checkpoint,completed_at)
		VALUES($1,$2,$3,$4,'completed','dependency_changed',$5,$6,CURRENT_TIMESTAMP)`,
		taskID, invalidationOperationID, projectID, roots[0], "pacing-edit-task:"+operationID,
		mustJSON(map[string]any{"changed_beat_artifact_ids": roots, "affected_count": len(impacts)})); err != nil {
		return mapPGConflict(err)
	}
	for _, item := range impacts {
		var before string
		if err := tx.QueryRow(ctx, `SELECT validity_status FROM drama.artifacts WHERE artifact_id=$1 FOR UPDATE`, item.id).Scan(&before); err != nil {
			return err
		}
		if before == "valid" || before == "needs_review" {
			if _, err := tx.Exec(ctx, `UPDATE drama.artifacts SET validity_status='stale' WHERE artifact_id=$1`, item.id); err != nil {
				return err
			}
		}
		impactID, _ := newPublicID("impact_")
		if _, err := tx.Exec(ctx, `INSERT INTO drama.invalidation_impacts(
			invalidation_impact_id,invalidation_task_id,artifact_id,before_status,after_status,
			propagation_depth,reason,dependency_path)
			VALUES($1,$2,$3,$4,'stale',$5,$6,'[]'::jsonb) ON CONFLICT(invalidation_task_id,artifact_id) DO NOTHING`,
			impactID, taskID, item.id, before, item.depth,
			mustJSON(map[string]any{"reason": "pacing_beat_changed", "root_beat_artifact_ids": roots})); err != nil {
			return mapPGConflict(err)
		}
	}
	return nil
}

func eventIDFromBeat(events []adaptationanalysis.Event, beatKey string) string {
	for _, event := range events {
		if "beat."+event.EventRevisionID == beatKey {
			return event.EventRevisionID
		}
	}
	return ""
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableArtifact(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func sortedMapValues(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	result := []string{values[0]}
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
