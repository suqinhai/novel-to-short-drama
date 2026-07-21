package store

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
)

func (s *Store) GetProjectImpact(ctx context.Context, projectID, toSourceVersionID string) (ProjectImpact, string, error) {
	var result ProjectImpact
	var changedChapters []byte
	err := s.pool.QueryRow(ctx, `SELECT change_set.source_change_set_id,change_set.from_source_version_id,
		change_set.to_source_version_id,change_set.from_ir_revision_id,change_set.to_ir_revision_id,
		change_set.status,change_set.changed_chapter_ids
		FROM drama.source_change_sets change_set
		WHERE change_set.to_source_version_id=$2
		  AND EXISTS(SELECT 1 FROM drama.invalidation_tasks task
		    WHERE task.source_change_set_id=change_set.source_change_set_id AND task.project_id=$1)
		ORDER BY change_set.created_at DESC LIMIT 1`, projectID, toSourceVersionID).
		Scan(&result.SourceChangeSetID, &result.FromSourceVersionID, &result.ToSourceVersionID,
			&result.FromIRRevisionID, &result.ToIRRevisionID, &result.Status, &changedChapters)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, "", ErrNotFound
	}
	if err != nil {
		return result, "", err
	}
	if err := json.Unmarshal(changedChapters, &result.ChangedChapterIDs); err != nil {
		return result, "", err
	}
	result.ChangedEvents = []ImpactChange{}
	result.ChangedCharacterStates = []ImpactChange{}
	result.AffectedStoryArcs = []ImpactChange{}
	rows, err := s.pool.Query(ctx, `SELECT source_change_item_id,change_type,before_entity_id,after_entity_id,details
		FROM drama.source_change_items WHERE source_change_set_id=$1
		ORDER BY entity_type,source_change_item_id`, result.SourceChangeSetID)
	if err != nil {
		return result, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var item ImpactChange
		var details []byte
		if err := rows.Scan(&item.SourceChangeItemID, &item.ChangeType, &item.BeforeEntityID, &item.AfterEntityID, &details); err != nil {
			return result, "", err
		}
		item.Details = json.RawMessage(details)
		var discriminator struct {
			Subtype string `json:"subtype"`
		}
		_ = json.Unmarshal(details, &discriminator)
		switch discriminator.Subtype {
		case "event":
			result.ChangedEvents = append(result.ChangedEvents, item)
		case "character_state":
			result.ChangedCharacterStates = append(result.ChangedCharacterStates, item)
		case "story_arc":
			result.AffectedStoryArcs = append(result.AffectedStoryArcs, item)
		}
	}
	if err := rows.Err(); err != nil {
		return result, "", err
	}

	result.AffectedArtifacts = []ArtifactImpact{}
	result.NeedsReview = []string{}
	artifactRows, err := s.pool.Query(ctx, `SELECT artifact.artifact_id,artifact.artifact_type,artifact.native_entity_id,
		artifact.revision_number,impact.before_status,impact.after_status,impact.propagation_depth,impact.reason,
		CASE artifact.artifact_type
		  WHEN 'adaptation_plan' THEN (SELECT status FROM drama.adaptation_plans WHERE adaptation_plan_id=artifact.native_entity_id)
		  WHEN 'adaptation_episode_plan' THEN (SELECT plan.status FROM drama.adaptation_episode_plans episode
		    JOIN drama.adaptation_plans plan USING(adaptation_plan_id) WHERE episode.adaptation_episode_plan_id=artifact.native_entity_id)
		  WHEN 'episode_outline' THEN (SELECT status FROM drama.episode_outlines WHERE episode_id=artifact.native_entity_id)
		  WHEN 'episode_script' THEN (SELECT status FROM drama.episode_scripts WHERE script_id=artifact.native_entity_id)
		  ELSE NULL
		END AS review_status
		FROM drama.invalidation_tasks task
		JOIN drama.invalidation_impacts impact USING(invalidation_task_id)
		JOIN drama.artifacts artifact USING(artifact_id)
		WHERE task.source_change_set_id=$1 AND task.project_id=$2
		ORDER BY impact.propagation_depth,artifact.artifact_type,artifact.artifact_id`, result.SourceChangeSetID, projectID)
	if err != nil {
		return result, "", err
	}
	defer artifactRows.Close()
	for artifactRows.Next() {
		var item ArtifactImpact
		var reason []byte
		if err := artifactRows.Scan(&item.ArtifactID, &item.ArtifactType, &item.NativeEntityID, &item.RevisionNumber,
			&item.BeforeStatus, &item.AfterStatus, &item.PropagationDepth, &reason, &item.ReviewStatus); err != nil {
			return result, "", err
		}
		item.Reason = json.RawMessage(reason)
		result.AffectedArtifacts = append(result.AffectedArtifacts, item)
		if item.ReviewStatus != nil && (*item.ReviewStatus == "approved" || *item.ReviewStatus == "waiting_review") {
			result.NeedsReview = append(result.NeedsReview, item.ArtifactID)
		}
	}
	if err := artifactRows.Err(); err != nil {
		return result, "", err
	}
	traceID, _ := newPublicID("tr_")
	return result, traceID, nil
}

func (s *Store) CreateRegenerationRequest(ctx context.Context, projectID, changeSetID, key string, input RegenerationRequestInput) (RegenerationRequest, bool, error) {
	artifactIDs := append([]string(nil), input.ArtifactIDs...)
	sort.Strings(artifactIDs)
	input.ArtifactIDs = artifactIDs
	inputHash, err := hashJSON(input)
	if err != nil {
		return RegenerationRequest{}, false, err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return RegenerationRequest{}, false, err
	}
	defer tx.Rollback(ctx)

	if existing, found, err := getRegenerationRequestByKey(ctx, tx, key); err != nil {
		return RegenerationRequest{}, false, err
	} else if found {
		var summary struct {
			InputHash string `json:"input_hash"`
		}
		_ = json.Unmarshal(existing.summary, &summary)
		if existing.request.ProjectID != projectID || existing.request.SourceChangeSetID != changeSetID || summary.InputHash != inputHash {
			return RegenerationRequest{}, false, ErrConflict
		}
		return existing.request, false, nil
	}

	var changeStatus string
	if err := tx.QueryRow(ctx, `SELECT change_set.status FROM drama.source_change_sets change_set
		WHERE change_set.source_change_set_id=$1 AND EXISTS(
		  SELECT 1 FROM drama.invalidation_tasks task WHERE task.source_change_set_id=change_set.source_change_set_id AND task.project_id=$2
		) FOR UPDATE`, changeSetID, projectID).Scan(&changeStatus); errors.Is(err, pgx.ErrNoRows) {
		return RegenerationRequest{}, false, ErrNotFound
	} else if err != nil {
		return RegenerationRequest{}, false, err
	}
	if changeStatus != "needs_review" && changeStatus != "completed" {
		return RegenerationRequest{}, false, ErrConflict
	}
	var affectedCount int
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT impact.artifact_id)
		FROM drama.invalidation_tasks task JOIN drama.invalidation_impacts impact USING(invalidation_task_id)
		JOIN drama.artifacts artifact USING(artifact_id)
		WHERE task.source_change_set_id=$1 AND task.project_id=$2
		  AND impact.artifact_id=ANY($3) AND artifact.validity_status='stale'`, changeSetID, projectID, artifactIDs).Scan(&affectedCount); err != nil {
		return RegenerationRequest{}, false, err
	}
	if affectedCount != len(artifactIDs) {
		return RegenerationRequest{}, false, ErrConflict
	}
	requestID, _ := newPublicID("regen_")
	if _, err := tx.Exec(ctx, `INSERT INTO drama.regeneration_requests(regeneration_request_id,source_change_set_id,project_id,
		strategy,status,requested_by,idempotency_key,request_summary)
		VALUES($1,$2,$3,$4,'queued',$5,$6,$7)`, requestID, changeSetID, projectID, input.Strategy,
		input.RequestedBy, key, mustJSON(map[string]any{"input_hash": inputHash, "selected_artifact_count": len(artifactIDs)})); err != nil {
		return RegenerationRequest{}, false, mapPGConflict(err)
	}
	for _, artifactID := range artifactIDs {
		itemID, _ := newPublicID("regeni_")
		if _, err := tx.Exec(ctx, `INSERT INTO drama.regeneration_request_items(regeneration_request_item_id,
			regeneration_request_id,artifact_id) VALUES($1,$2,$3)`, itemID, requestID, artifactID); err != nil {
			return RegenerationRequest{}, false, mapPGConflict(err)
		}
	}
	loaded, _, err := getRegenerationRequestByKey(ctx, tx, key)
	if err != nil {
		return RegenerationRequest{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RegenerationRequest{}, false, err
	}
	return loaded.request, true, nil
}

type regenerationRequestRow struct {
	request RegenerationRequest
	summary []byte
}

func getRegenerationRequestByKey(ctx context.Context, tx pgx.Tx, key string) (regenerationRequestRow, bool, error) {
	var row regenerationRequestRow
	var artifactIDs []string
	err := tx.QueryRow(ctx, `SELECT request.regeneration_request_id,request.source_change_set_id,request.project_id,
		request.strategy,request.status,request.request_summary,request.created_at,request.updated_at,
		COALESCE((SELECT array_agg(item.artifact_id ORDER BY item.artifact_id)
		  FROM drama.regeneration_request_items item
		  WHERE item.regeneration_request_id=request.regeneration_request_id),'{}')
		FROM drama.regeneration_requests request WHERE request.idempotency_key=$1`, key).
		Scan(&row.request.RegenerationRequestID, &row.request.SourceChangeSetID, &row.request.ProjectID,
			&row.request.Strategy, &row.request.Status, &row.summary, &row.request.CreatedAt, &row.request.UpdatedAt, &artifactIDs)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, nil
	}
	row.request.ArtifactIDs = artifactIDs
	return row, err == nil, err
}
