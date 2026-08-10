package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Store) StartCompilerRun(ctx context.Context, projectID, key string, input CompilerRunInput) (Operation, error) {
	planningConstraints := input.PlanningConstraints
	if len(planningConstraints) == 0 || string(planningConstraints) == "null" {
		planningConstraints = json.RawMessage(`{}`)
	}
	providerSuggestions := input.ProviderSuggestions
	if len(providerSuggestions) == 0 || string(providerSuggestions) == "null" {
		providerSuggestions = json.RawMessage(`[]`)
	}
	inputHash, err := hashJSON(map[string]any{
		"adaptation_spec_version_id": input.AdaptationSpecVersionID,
		"ir_revision_id":             input.IRRevisionID,
		"compiler_version":           input.CompilerVersion,
		"planning_constraints":       planningConstraints,
		"provider_suggestions":       providerSuggestions,
	})
	if err != nil {
		return Operation{}, err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)

	if replay, found, replayErr := getOperationByIdempotency(ctx, tx, key); replayErr != nil {
		return Operation{}, replayErr
	} else if found {
		var replayProject, replaySpec, replayIR, replayVersion, replayHash string
		err = tx.QueryRow(ctx, `SELECT project_id,adaptation_spec_version_id,ir_revision_id,compiler_version,input_hash
			FROM drama.compiler_runs WHERE operation_id=$1`, replay.OperationID).
			Scan(&replayProject, &replaySpec, &replayIR, &replayVersion, &replayHash)
		if err != nil || replay.OperationType != "adaptation_compile" || replay.TargetType != "project" ||
			replayProject != projectID || replaySpec != input.AdaptationSpecVersionID || replayIR != input.IRRevisionID ||
			replayVersion != input.CompilerVersion || replayHash != inputHash {
			return Operation{}, ErrConflict
		}
		return replay, nil
	}

	var workID, sourceVersionID, specIR, specStatus, irStatus, irScope, sourceStatus string
	err = tx.QueryRow(ctx, `SELECT spec.work_id,spec.source_version_id,spec.ir_revision_id,spec.status,ir.status,ir.revision_scope,source.status
		FROM drama.adaptation_spec_versions spec
		JOIN drama.narrative_ir_revisions ir ON ir.ir_revision_id=spec.ir_revision_id
		JOIN drama.source_versions source ON source.source_version_id=spec.source_version_id
		WHERE spec.adaptation_spec_version_id=$1 AND spec.project_id=$2 FOR SHARE OF spec,ir,source`,
		input.AdaptationSpecVersionID, projectID).
		Scan(&workID, &sourceVersionID, &specIR, &specStatus, &irStatus, &irScope, &sourceStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	if specStatus != "active" || specIR != input.IRRevisionID || irStatus != "published" || irScope != "full" || sourceStatus != "published" {
		return Operation{}, ErrConflict
	}

	operationID, err := newPublicID("op_")
	if err != nil {
		return Operation{}, err
	}
	traceID, err := newPublicID("tr_")
	if err != nil {
		return Operation{}, err
	}
	compilerRunID, err := newPublicID("compiler_")
	if err != nil {
		return Operation{}, err
	}
	pipeline := []string{
		"source_scope_resolution", "event_selection", "prerequisite_ordering",
		"event_compression_merge", "episode_allocation", "character_state_validation",
		"foreshadow_validation", "duration_validation", "reviewable_plan",
	}
	checkpoint := mustJSON(map[string]any{
		"adaptation_spec_version_id": input.AdaptationSpecVersionID,
		"ir_revision_id":             input.IRRevisionID,
		"compiler_version":           input.CompilerVersion,
		"planning_constraints":       planningConstraints,
		"provider_suggestions":       providerSuggestions,
		"pipeline":                   pipeline,
		"completed_items":            0,
		"total_items":                len(pipeline),
	})
	if _, err = tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,
		idempotency_key,input_hash,checkpoint_stage,checkpoint_data)
		VALUES($1,$2,'adaptation_compile','project',$3,'pending',$4,$5,'queued',$6)`,
		operationID, traceID, projectID, key, inputHash, checkpoint); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.compiler_runs(compiler_run_id,operation_id,project_id,work_id,source_version_id,
		adaptation_spec_version_id,ir_revision_id,compiler_version,status,input_hash,idempotency_key,checkpoint)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending',$9,$10,$11)`, compilerRunID, operationID, projectID, workID,
		sourceVersionID, input.AdaptationSpecVersionID, input.IRRevisionID, input.CompilerVersion, inputHash, key, checkpoint); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	operation, _, err := getOperationByIdempotency(ctx, tx, key)
	if err != nil {
		return Operation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	return operation, nil
}

func (s *Store) GetAdaptationPlan(ctx context.Context, adaptationPlanID string) (json.RawMessage, string, error) {
	var plan json.RawMessage
	var traceID string
	err := s.pool.QueryRow(ctx, `SELECT operation.trace_id,jsonb_build_object(
		'schema_version','compiler-plan.v2','adaptation_plan_id',plan.adaptation_plan_id,
		'project_id',plan.project_id,'status',plan.status,'compiler_run_id',run.compiler_run_id,
		'target_episode_duration_seconds',spec.episode_duration_seconds,
		'version_number',plan.version_number,'parent_adaptation_plan_id',plan.parent_adaptation_plan_id,
		'plan_name',plan.plan_name,'strategy_label',plan.strategy_label,'is_current',plan.is_current,
		'approved_by',plan.approved_by,'approved_at',plan.approved_at,'validation_run_at',plan.validation_run_at,
		'workbench_snapshot',plan.workbench_snapshot,'creative_suggestions',plan.creative_suggestions,
		'episodes',COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'episode_number',episode.episode_number,'title',episode.title,'logline',episode.logline,
			'estimated_duration_seconds',episode.estimated_duration_seconds,'opening_hook',episode.opening_hook,
			'three_second_opening',episode.three_second_opening,
			'first_thirty_seconds_goal',episode.first_thirty_seconds_goal,'core_conflict',episode.core_conflict,
			'climax',episode.climax,'emotion_curve',episode.emotion_curve,
			'information_reveal_amount',episode.information_reveal_amount,
			'character_arc_entity_ids',episode.character_arc_entity_ids,'story_arc_revision_ids',episode.story_arc_revision_ids,
			'ending_hook',episode.ending_hook,'continuity_in',episode.continuity_in,'continuity_out',episode.continuity_out,
			'source_event_ids',CASE WHEN jsonb_array_length(episode.source_event_ids)=0 THEN
				COALESCE((SELECT jsonb_agg(assignment.event_revision_id ORDER BY assignment.sequence_number)
					FROM drama.episode_event_assignments assignment
					WHERE assignment.adaptation_episode_plan_id=episode.adaptation_episode_plan_id),'[]'::jsonb)
				ELSE episode.source_event_ids END,
			'source_chapter_ids',CASE WHEN jsonb_array_length(episode.source_chapter_ids)=0 THEN
				COALESCE((SELECT jsonb_agg(provenance.chapter_id ORDER BY provenance.first_sequence)
					FROM (SELECT fact.chapter_id,min(assignment.sequence_number) first_sequence
						FROM drama.episode_event_assignments assignment
						JOIN drama.narrative_event_revisions event USING(event_revision_id)
						JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
						WHERE assignment.adaptation_episode_plan_id=episode.adaptation_episode_plan_id
						GROUP BY fact.chapter_id) provenance),'[]'::jsonb)
				ELSE episode.source_chapter_ids END,
			'added_adaptation_content',episode.added_adaptation_content,'merged_content',episode.merged_content,
			'deviation_notes',episode.deviation_notes,
			'event_assignments',COALESCE((SELECT jsonb_agg(jsonb_build_object(
				'event_revision_id',assignment.event_revision_id,'sequence_number',assignment.sequence_number,
				'usage_mode',assignment.usage_mode,'merge_group_id',assignment.merge_group_id,
				'rule_ids',assignment.rule_trace,'summary',event.summary,'importance',event.importance,
				'event_type',event.event_type,'chapter_id',fact.chapter_id,'chapter_title',chapter_revision.title,
				'participants',COALESCE((SELECT jsonb_agg(jsonb_build_object('entity_revision_id',entity.entity_revision_id,
					'name',entity.canonical_name,'role',participant.participant_role) ORDER BY entity.canonical_name)
					FROM drama.event_participants participant JOIN drama.narrative_entity_revisions entity
					ON entity.entity_revision_id=participant.entity_revision_id
					WHERE participant.event_revision_id=assignment.event_revision_id),'[]'::jsonb),
				'character_states',COALESCE((SELECT jsonb_agg(jsonb_build_object('state_change_id',state.state_change_id,
					'character_entity_revision_id',state.character_entity_revision_id,'state_dimension',state.state_dimension,
					'before_state',state.before_state,'after_state',state.after_state) ORDER BY state.sequence_number)
					FROM drama.character_state_changes state WHERE state.trigger_event_revision_id=assignment.event_revision_id),'[]'::jsonb),
				'foreshadowing',COALESCE((SELECT jsonb_agg(jsonb_build_object('foreshadow_thread_id',occurrence.foreshadow_thread_id,
					'title',thread.title,'lifecycle_stage',occurrence.lifecycle_stage,'occurrence_order',occurrence.occurrence_order)
					ORDER BY occurrence.occurrence_order) FROM drama.foreshadow_occurrences occurrence
					JOIN drama.foreshadow_threads thread USING(foreshadow_thread_id)
					WHERE occurrence.event_revision_id=assignment.event_revision_id),'[]'::jsonb)
				) ORDER BY assignment.sequence_number)
				FROM drama.episode_event_assignments assignment
				JOIN drama.narrative_event_revisions event USING(event_revision_id)
				JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
				JOIN drama.source_version_chapters version_chapter ON version_chapter.source_version_id=run.source_version_id
					AND version_chapter.chapter_id=fact.chapter_id
				JOIN drama.chapter_revisions chapter_revision ON chapter_revision.chapter_revision_id=version_chapter.chapter_revision_id
				WHERE assignment.adaptation_episode_plan_id=episode.adaptation_episode_plan_id),'[]'::jsonb)
		) ORDER BY episode.episode_number) FROM drama.adaptation_episode_plans episode
		WHERE episode.adaptation_plan_id=plan.adaptation_plan_id),'[]'::jsonb),
		'diagnostics',COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'severity',diagnostic.severity,'code',diagnostic.diagnostic_code,'message',diagnostic.message,
			'entity_type',diagnostic.entity_type,'entity_id',diagnostic.entity_id,'details',diagnostic.details)
			ORDER BY diagnostic.id) FROM drama.compiler_diagnostics diagnostic
			WHERE diagnostic.compiler_run_id=run.compiler_run_id),'[]'::jsonb),
		'validation',COALESCE(plan.quality_report->'validation','{}'::jsonb),
		'latest_validation',COALESCE((SELECT jsonb_build_object('validator_version',validation.validator_version,
			'passed',validation.passed,'checks',validation.checks,'diagnostics',validation.diagnostics)
			FROM drama.adaptation_plan_validation_runs validation WHERE validation.adaptation_plan_id=plan.adaptation_plan_id
			ORDER BY validation.created_at DESC LIMIT 1),plan.quality_report->'workbench_validation','null'::jsonb),
		'season_curves',jsonb_build_object(
			'emotion',COALESCE((SELECT jsonb_agg(COALESCE((SELECT max((point->>'emotion')::numeric)
				FROM jsonb_array_elements(episode.emotion_curve) point),0) ORDER BY episode.episode_number)
				FROM drama.adaptation_episode_plans episode WHERE episode.adaptation_plan_id=plan.adaptation_plan_id),'[]'::jsonb),
			'information_reveal',COALESCE((SELECT jsonb_agg(episode.information_reveal_amount ORDER BY episode.episode_number)
				FROM drama.adaptation_episode_plans episode WHERE episode.adaptation_plan_id=plan.adaptation_plan_id),'[]'::jsonb),
			'duration',COALESCE((SELECT jsonb_agg(episode.estimated_duration_seconds ORDER BY episode.episode_number)
				FROM drama.adaptation_episode_plans episode WHERE episode.adaptation_plan_id=plan.adaptation_plan_id),'[]'::jsonb)),
		'rules',COALESCE((SELECT jsonb_agg(jsonb_build_object('adaptation_rule_id',rule.adaptation_rule_id,
			'rule_type',rule.rule_type,'enforcement',rule.enforcement,'target_type',rule.target_type,
			'target_id',rule.target_id,'priority',rule.priority,'parameters',rule.parameters,'rationale',rule.rationale)
			ORDER BY rule.priority DESC,rule.adaptation_rule_id) FROM drama.adaptation_rules rule
			WHERE rule.adaptation_spec_version_id=plan.adaptation_spec_version_id),'[]'::jsonb)
	) FROM drama.adaptation_plans plan
	JOIN drama.compiler_runs run ON run.compiler_run_id=plan.compiler_run_id
	JOIN drama.adaptation_spec_versions spec ON spec.adaptation_spec_version_id=plan.adaptation_spec_version_id
	JOIN drama.operations operation ON operation.operation_id=run.operation_id
	WHERE plan.adaptation_plan_id=$1`, adaptationPlanID).Scan(&traceID, &plan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return plan, traceID, err
}

func (s *Store) GetLatestAdaptationPlan(ctx context.Context, projectID string) (json.RawMessage, string, error) {
	var adaptationPlanID string
	err := s.pool.QueryRow(ctx, `SELECT adaptation_plan_id
		FROM drama.adaptation_plans
		WHERE project_id=$1
		ORDER BY created_at DESC,id DESC
		LIMIT 1`, projectID).Scan(&adaptationPlanID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return s.GetAdaptationPlan(ctx, adaptationPlanID)
}
