package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateAdaptationProject(ctx context.Context, key string, input CreateAdaptationProjectInput) (Operation, error) {
	inputHash, err := hashJSON(input)
	if err != nil {
		return Operation{}, err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "adaptation-project:"+key); err != nil {
		return Operation{}, err
	}
	if replay, found, err := getOperationByIdempotency(ctx, tx, key); err != nil {
		return Operation{}, err
	} else if found {
		if replay.OperationType != "spec_validation" || replay.TargetType != "project" || replay.InputHash != inputHash {
			return Operation{}, ErrConflict
		}
		var linked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.adaptation_spec_versions
			WHERE operation_id=$1 AND project_id=$2 AND source_version_id=$3
			  AND ($4='' OR ir_revision_id=$4))`, replay.OperationID,
			replay.TargetID, input.AdaptationSpec.SourceVersionID, input.AdaptationSpec.IRRevisionID).Scan(&linked); err != nil || !linked {
			return Operation{}, ErrConflict
		}
		return replay, nil
	}
	workID, resolvedIR, err := resolveFrozenSpecInputs(ctx, tx, input.AdaptationSpec)
	if err != nil {
		return Operation{}, err
	}
	input.AdaptationSpec.IRRevisionID = resolvedIR
	specHash, err := hashJSON(input.AdaptationSpec)
	if err != nil {
		return Operation{}, err
	}
	projectID, err := newPublicID("prj_")
	if err != nil {
		return Operation{}, err
	}
	bindingID, err := newPublicID("psb_")
	if err != nil {
		return Operation{}, err
	}
	specID, err := newPublicID("as_")
	if err != nil {
		return Operation{}, err
	}
	specVersionID, err := newPublicID("asv_")
	if err != nil {
		return Operation{}, err
	}
	operationID, err := newPublicID("op_")
	if err != nil {
		return Operation{}, err
	}
	traceID, err := newPublicID("tr_")
	if err != nil {
		return Operation{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.projects(project_id,novel_name,target_episode_count,episode_duration_seconds,
		visual_style,aspect_ratio,target_platform,current_stage,status,test_mode,config,display_name)
		VALUES($1,$2,$3,$4,'narrative-ir','9:16',$5,'created','pending',false,$6,$2)`, projectID, input.DisplayName,
		input.AdaptationSpec.TargetEpisodeCount, input.AdaptationSpec.EpisodeDurationSeconds, input.AdaptationSpec.Platform,
		mustJSON(map[string]any{"contract_version": "2.0", "source_version_id": input.AdaptationSpec.SourceVersionID})); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.project_source_bindings(binding_id,project_id,work_id,source_version_id,binding_role,is_current,idempotency_key)
		VALUES($1,$2,$3,$4,'primary',true,$5)`, bindingID, projectID, workID, input.AdaptationSpec.SourceVersionID, key+":binding"); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_specs(adaptation_spec_id,project_id,display_name,is_current,idempotency_key)
		VALUES($1,$2,$3,true,$4)`, specID, projectID, input.DisplayName, key+":spec"); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	if err := insertActiveSpecVersion(ctx, tx, key, inputHash, specHash, operationID, traceID, "project", projectID,
		specID, specVersionID, bindingID, projectID, workID, 1, input.AdaptationSpec); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	return s.GetOperation(ctx, operationID)
}

func (s *Store) ListAdaptationSpecs(ctx context.Context, projectID string) ([]AdaptationSpecSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT v.adaptation_spec_id,v.adaptation_spec_version_id,v.version_number,v.status,
		v.source_version_id,v.ir_revision_id,v.resource_revision
		FROM drama.adaptation_spec_versions v WHERE v.project_id=$1 ORDER BY v.version_number DESC,v.created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AdaptationSpecSummary, 0)
	for rows.Next() {
		var item AdaptationSpecSummary
		if err := rows.Scan(&item.AdaptationSpecID, &item.AdaptationSpecVersionID, &item.VersionNumber, &item.Status,
			&item.SourceVersionID, &item.IRRevisionID, &item.ResourceRevision); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.projects WHERE project_id=$1)`, projectID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return items, nil
}

func (s *Store) CreateAdaptationSpecVersion(ctx context.Context, projectID, key string, input AdaptationSpecInput) (Operation, error) {
	inputHash, err := hashJSON(input)
	if err != nil {
		return Operation{}, err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "adaptation-spec:"+projectID+":"+key); err != nil {
		return Operation{}, err
	}
	if replay, found, err := getOperationByIdempotency(ctx, tx, key); err != nil {
		return Operation{}, err
	} else if found {
		if replay.OperationType != "spec_validation" || replay.TargetType != "adaptation_spec_version" || replay.InputHash != inputHash {
			return Operation{}, ErrConflict
		}
		var linkedProject, linkedSource, linkedIR string
		if err := tx.QueryRow(ctx, `SELECT project_id,source_version_id,ir_revision_id FROM drama.adaptation_spec_versions
			WHERE adaptation_spec_version_id=$1 AND operation_id=$2`, replay.TargetID, replay.OperationID).
			Scan(&linkedProject, &linkedSource, &linkedIR); err != nil || linkedProject != projectID ||
			linkedSource != input.SourceVersionID || (input.IRRevisionID != "" && linkedIR != input.IRRevisionID) {
			return Operation{}, ErrConflict
		}
		return replay, nil
	}
	workID, resolvedIR, err := resolveFrozenSpecInputs(ctx, tx, input)
	if err != nil {
		return Operation{}, err
	}
	input.IRRevisionID = resolvedIR
	specHash, err := hashJSON(input)
	if err != nil {
		return Operation{}, err
	}
	var displayName string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(display_name,novel_name) FROM drama.projects WHERE project_id=$1 FOR UPDATE`, projectID).Scan(&displayName); errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	} else if err != nil {
		return Operation{}, err
	}
	var currentWorkID string
	if err := tx.QueryRow(ctx, `SELECT work_id FROM drama.project_source_bindings
		WHERE project_id=$1 AND binding_role='primary' AND is_current FOR UPDATE`, projectID).Scan(&currentWorkID); errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrConflict
	} else if err != nil {
		return Operation{}, err
	} else if currentWorkID != workID {
		return Operation{}, ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.project_source_bindings SET is_current=false
		WHERE project_id=$1 AND binding_role='primary' AND is_current AND source_version_id<>$2`, projectID, input.SourceVersionID); err != nil {
		return Operation{}, err
	}
	var bindingID string
	err = tx.QueryRow(ctx, `SELECT binding_id FROM drama.project_source_bindings WHERE project_id=$1 AND source_version_id=$2 AND binding_role='primary'`,
		projectID, input.SourceVersionID).Scan(&bindingID)
	if errors.Is(err, pgx.ErrNoRows) {
		bindingID, err = newPublicID("psb_")
		if err != nil {
			return Operation{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drama.project_source_bindings(binding_id,project_id,work_id,source_version_id,binding_role,is_current,idempotency_key)
			VALUES($1,$2,$3,$4,'primary',true,$5)`, bindingID, projectID, workID, input.SourceVersionID, key+":binding"); err != nil {
			return Operation{}, mapPGConflict(err)
		}
	} else if err != nil {
		return Operation{}, err
	} else if _, err := tx.Exec(ctx, `UPDATE drama.project_source_bindings SET is_current=true WHERE binding_id=$1`, bindingID); err != nil {
		return Operation{}, err
	}
	var specID string
	var nextVersion int
	err = tx.QueryRow(ctx, `SELECT adaptation_spec_id FROM drama.adaptation_specs WHERE project_id=$1 AND is_current FOR UPDATE`, projectID).Scan(&specID)
	if errors.Is(err, pgx.ErrNoRows) {
		specID, err = newPublicID("as_")
		if err != nil {
			return Operation{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_specs(adaptation_spec_id,project_id,display_name,is_current,idempotency_key)
			VALUES($1,$2,$3,true,$4)`, specID, projectID, displayName, key+":spec"); err != nil {
			return Operation{}, mapPGConflict(err)
		}
	} else if err != nil {
		return Operation{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(version_number),0)+1 FROM drama.adaptation_spec_versions WHERE adaptation_spec_id=$1`, specID).Scan(&nextVersion); err != nil {
		return Operation{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.adaptation_spec_versions SET status='superseded'
		WHERE adaptation_spec_id=$1 AND status='active'`, specID); err != nil {
		return Operation{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.adaptation_specs SET resource_revision=resource_revision+1 WHERE adaptation_spec_id=$1`, specID); err != nil {
		return Operation{}, err
	}
	operationID, err := newPublicID("op_")
	if err != nil {
		return Operation{}, err
	}
	traceID, err := newPublicID("tr_")
	if err != nil {
		return Operation{}, err
	}
	specVersionID, err := newPublicID("asv_")
	if err != nil {
		return Operation{}, err
	}
	if err := insertActiveSpecVersion(ctx, tx, key, inputHash, specHash, operationID, traceID, "adaptation_spec_version", specVersionID,
		specID, specVersionID, bindingID, projectID, workID, nextVersion, input); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, mapPGConflict(err)
	}
	return s.GetOperation(ctx, operationID)
}

func resolveFrozenSpecInputs(ctx context.Context, tx pgx.Tx, input AdaptationSpecInput) (string, string, error) {
	var workID string
	err := tx.QueryRow(ctx, `SELECT work_id FROM drama.source_versions WHERE source_version_id=$1 AND status='published' AND published_at IS NOT NULL`,
		input.SourceVersionID).Scan(&workID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrConflict
	}
	if err != nil {
		return "", "", err
	}
	if input.IRRevisionID != "" {
		var validIR bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.narrative_ir_revisions
			WHERE ir_revision_id=$1 AND work_id=$2 AND source_version_id=$3 AND status='published' AND revision_scope='full')`,
			input.IRRevisionID, workID, input.SourceVersionID).Scan(&validIR); err != nil {
			return "", "", err
		}
		if !validIR {
			return "", "", ErrConflict
		}
		return workID, input.IRRevisionID, nil
	}
	var resolvedIDs []string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(array_agg(ir_revision_id ORDER BY revision_number DESC),'{}')
		FROM drama.narrative_ir_revisions WHERE work_id=$1 AND source_version_id=$2 AND status='published'
		  AND is_current AND revision_scope='full'`, workID, input.SourceVersionID).Scan(&resolvedIDs); err != nil {
		return "", "", err
	}
	if len(resolvedIDs) != 1 {
		return "", "", ErrConflict
	}
	return workID, resolvedIDs[0], nil
}

func insertActiveSpecVersion(ctx context.Context, tx pgx.Tx, key, inputHash, specHash, operationID, traceID, targetType, targetID,
	specID, specVersionID, bindingID, projectID, workID string, versionNumber int, input AdaptationSpecInput) error {
	if _, err := tx.Exec(ctx, `INSERT INTO drama.operations(operation_id,trace_id,operation_type,target_type,target_id,status,idempotency_key,input_hash,
		checkpoint_stage,checkpoint_data,result_type,result_id,completed_at)
		VALUES($1,$2,'spec_validation',$3,$4,'completed',$5,$6,'finished',$7,'adaptation_spec_version',$8,CURRENT_TIMESTAMP)`,
		operationID, traceID, targetType, targetID, key, inputHash,
		mustJSON(map[string]any{"stage": "finished", "validation": "passed"}), specVersionID); err != nil {
		return mapPGConflict(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_spec_versions(adaptation_spec_version_id,operation_id,adaptation_spec_id,project_id,
		source_binding_id,work_id,version_number,source_version_id,ir_revision_id,status,platform,audience_profile,target_episode_count,
		episode_duration_seconds,scope_mode,content_hash,idempotency_key)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'draft',$10,$11,$12,$13,$14,$15,$16)`, specVersionID, operationID, specID, projectID,
		bindingID, workID, versionNumber, input.SourceVersionID, input.IRRevisionID, input.Platform, input.AudienceProfile,
		input.TargetEpisodeCount, input.EpisodeDurationSeconds, input.Scope.Mode, specHash, key); err != nil {
		return mapPGConflict(err)
	}
	for _, chapterID := range input.Scope.ChapterIDs {
		scopeID, err := newPublicID("asc_")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_scope_chapters(scope_chapter_id,adaptation_spec_version_id,project_id,work_id,
			source_version_id,ir_revision_id,chapter_id,include_mode)
			VALUES($1,$2,$3,$4,$5,$6,$7,'include')`, scopeID, specVersionID, projectID, workID,
			input.SourceVersionID, input.IRRevisionID, chapterID); err != nil {
			return mapPGConflict(err)
		}
	}
	for _, arcID := range input.Scope.StoryArcRevisionIDs {
		scopeID, err := newPublicID("asa_")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_scope_arcs(scope_arc_id,adaptation_spec_version_id,project_id,work_id,
			source_version_id,ir_revision_id,story_arc_revision_id,include_mode) VALUES($1,$2,$3,$4,$5,$6,$7,'include')`, scopeID,
			specVersionID, projectID, workID, input.SourceVersionID, input.IRRevisionID, arcID); err != nil {
			return mapPGConflict(err)
		}
	}
	for index, rule := range input.Rules {
		ruleID, err := newPublicID("ar_")
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drama.adaptation_rules(adaptation_rule_id,adaptation_spec_version_id,rule_type,enforcement,
			target_type,target_id,priority,parameters,rationale,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ruleID,
			specVersionID, rule.RuleType, rule.Enforcement, rule.TargetType, rule.TargetID, rule.Priority, rule.Parameters, rule.Rationale,
			key+":rule:"+fmt.Sprintf("%d", index+1)); err != nil {
			return mapPGConflict(err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.adaptation_spec_versions SET status='active',activated_at=CURRENT_TIMESTAMP
		WHERE adaptation_spec_version_id=$1`, specVersionID); err != nil {
		return mapPGConflict(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.projects SET current_adaptation_spec_version_id=$2 WHERE project_id=$1`, projectID, specVersionID); err != nil {
		return err
	}
	return nil
}
