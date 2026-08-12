package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/shoteditor"
)

type ShotEditImpact struct {
	ChangedShotIDs []string         `json:"changed_shot_ids"`
	CreatedShotIDs []string         `json:"created_shot_ids"`
	RetiredShotIDs []string         `json:"retired_shot_ids"`
	StaleArtifacts []map[string]any `json:"stale_artifacts"`
	RebuildTasks   []map[string]any `json:"rebuild_tasks"`
}

type ShotEditPlan struct {
	ShotEditPlanID           string                     `json:"shot_edit_plan_id"`
	ProjectID                string                     `json:"project_id"`
	EpisodeID                string                     `json:"episode_id"`
	Operation                string                     `json:"operation"`
	Status                   string                     `json:"status"`
	BaseSequenceVersion      int                        `json:"base_sequence_version"`
	BaseSnapshotHash         string                     `json:"base_snapshot_hash"`
	Request                  shoteditor.Request         `json:"request"`
	BaseSnapshot             []shoteditor.Shot          `json:"base_snapshot"`
	ProposedSnapshot         []shoteditor.Shot          `json:"proposed_snapshot"`
	Impact                   ShotEditImpact             `json:"impact_preview"`
	CoverageReport           []shoteditor.CoverageCheck `json:"coverage_report"`
	ContinuityConflicts      []shoteditor.Conflict      `json:"continuity_conflicts"`
	HandoffPreview           []shoteditor.Handoff       `json:"handoff_preview"`
	Fingerprint              string                     `json:"fingerprint"`
	RequestedBy              *string                    `json:"requested_by,omitempty"`
	ConfirmedBy              *string                    `json:"confirmed_by,omitempty"`
	ConfirmedAt              *time.Time                 `json:"confirmed_at,omitempty"`
	AppliedSequenceVersionID *string                    `json:"applied_sequence_version_id,omitempty"`
	AppliedAt                *time.Time                 `json:"applied_at,omitempty"`
	ErrorCode                *string                    `json:"error_code,omitempty"`
	ErrorMessage             *string                    `json:"error_message,omitempty"`
	RebuildTasks             []IncrementalRebuild       `json:"rebuild_tasks"`
	CreatedAt                time.Time                  `json:"created_at"`
	UpdatedAt                time.Time                  `json:"updated_at"`
}

type ShotSequenceVersion struct {
	ShotSequenceVersionID string            `json:"shot_sequence_version_id"`
	Version               int               `json:"version"`
	ParentVersionID       *string           `json:"parent_shot_sequence_version_id,omitempty"`
	RestoredFromVersionID *string           `json:"restored_from_version_id,omitempty"`
	ShotEditPlanID        *string           `json:"shot_edit_plan_id,omitempty"`
	Snapshot              []shoteditor.Shot `json:"snapshot"`
	SnapshotHash          string            `json:"snapshot_hash"`
	IsCurrent             bool              `json:"is_current"`
	CreatedAt             time.Time         `json:"created_at"`
}

func (s *Store) CreateShotEditPlan(
	ctx context.Context, projectID, episodeID string, request shoteditor.Request,
) (ShotEditPlan, error) {
	resolverMetadata, err := s.freezeShotEditorInputs(ctx, projectID, episodeID)
	if err != nil {
		return ShotEditPlan{}, err
	}
	request.Metadata = resolverMetadata
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ShotEditPlan{}, err
	}
	defer tx.Rollback(ctx)
	base, version, _, err := readCurrentShotSequence(ctx, tx, projectID, episodeID, false)
	if err != nil {
		return ShotEditPlan{}, err
	}
	if request.BaseSequenceVersion > 0 && request.BaseSequenceVersion != version {
		return ShotEditPlan{}, fmt.Errorf("%w: shot sequence version is %d, current is %d", ErrConflict, request.BaseSequenceVersion, version)
	}
	request.BaseSequenceVersion = version
	resolvedShots, resolvedDialogueDurations, authoritative, resolveErr := resolvedShotEditorPayload(resolverMetadata)
	if resolveErr != nil {
		return ShotEditPlan{}, resolveErr
	}
	if authoritative {
		byID := shotMap(resolvedShots)
		authoritativeBase := make([]shoteditor.Shot, 0, len(base))
		for index, current := range base {
			resolved, exists := byID[current.ShotID]
			if !exists {
				return ShotEditPlan{}, fmt.Errorf("%w: current shot %s is absent from the Resolver snapshot",
					ErrConflict, current.ShotID)
			}
			resolved.ShotOrder, resolved.ShotNumber = index+1, index+1
			if resolved.Version == 0 {
				resolved.Version = max(1, resolved.GenerationVersion)
			}
			if !sameShotPayload(current, resolved) {
				return ShotEditPlan{}, fmt.Errorf("%w: shot %s differs from the Resolver snapshot",
					ErrConflict, current.ShotID)
			}
			authoritativeBase = append(authoritativeBase, resolved)
		}
		base = authoritativeBase
		request.DialogueDurationsMS = resolvedDialogueDurations
	} else {
		request.DialogueDurationsMS, err = readDialogueDurations(ctx, tx, episodeID)
		if err != nil {
			return ShotEditPlan{}, err
		}
	}
	if request.Operation == shoteditor.OperationRestore {
		request.RestoreSnapshot, err = readShotSequenceSnapshot(ctx, tx, projectID, episodeID, request.SourceSequenceVersionID)
		if err != nil {
			return ShotEditPlan{}, err
		}
	}
	if request.Operation == shoteditor.OperationSplit {
		request.NewShotIDs, err = publicIDs("shot_", len(request.Shots))
	} else if request.Operation == shoteditor.OperationMerge {
		request.NewShotIDs, err = publicIDs("shot_", 1)
	}
	if err != nil {
		return ShotEditPlan{}, err
	}
	preview, err := shoteditor.Build(base, request)
	if err != nil {
		return ShotEditPlan{}, err
	}
	baseHash, err := shotSequenceHash(base)
	if err != nil {
		return ShotEditPlan{}, err
	}
	impact, err := previewShotEditImpact(ctx, tx, projectID, episodeID, preview)
	if err != nil {
		return ShotEditPlan{}, err
	}
	planID, err := newPublicID("sep_")
	if err != nil {
		return ShotEditPlan{}, err
	}
	persistedRequest := request
	persistedRequest.DialogueDurationsMS, persistedRequest.RestoreSnapshot = nil, nil
	fingerprint, err := hashJSON(map[string]any{
		"project_id": projectID, "episode_id": episodeID, "base_hash": baseHash,
		"request": persistedRequest, "proposed": structuralShots(preview.Shots),
	})
	if err != nil {
		return ShotEditPlan{}, err
	}
	requestJSON, _ := json.Marshal(persistedRequest)
	baseJSON, _ := json.Marshal(base)
	proposedJSON, _ := json.Marshal(preview.Shots)
	impactJSON, _ := json.Marshal(impact)
	coverageJSON, _ := json.Marshal(preview.Coverage)
	conflictsJSON, _ := json.Marshal(preview.Conflicts)
	handoffJSON, _ := json.Marshal(preview.Handoffs)
	_, err = tx.Exec(ctx, `INSERT INTO drama.shot_edit_plans(
		shot_edit_plan_id,project_id,episode_id,operation,status,base_sequence_version,
		base_snapshot_hash,request,base_snapshot,proposed_snapshot,impact_preview,
		coverage_report,continuity_conflicts,handoff_preview,fingerprint,requested_by)
		VALUES($1,$2,$3,$4,'validated',$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb,
		$11::jsonb,$12::jsonb,$13::jsonb,$14,$15)`, planID, projectID, episodeID,
		request.Operation, version, baseHash, requestJSON, baseJSON, proposedJSON, impactJSON,
		coverageJSON, conflictsJSON, handoffJSON, fingerprint, request.RequestedBy)
	if err != nil {
		return ShotEditPlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ShotEditPlan{}, err
	}
	return s.GetShotEditPlan(ctx, projectID, episodeID, planID)
}

func (s *Store) GetShotEditPlan(ctx context.Context, projectID, episodeID, planID string) (ShotEditPlan, error) {
	var result ShotEditPlan
	var requestJSON, baseJSON, proposedJSON, impactJSON, coverageJSON, conflictsJSON, handoffJSON json.RawMessage
	err := s.pool.QueryRow(ctx, `SELECT shot_edit_plan_id,project_id,episode_id,operation,status,
		base_sequence_version,base_snapshot_hash,request,base_snapshot,proposed_snapshot,
		impact_preview,coverage_report,continuity_conflicts,handoff_preview,fingerprint,
		requested_by,confirmed_by,confirmed_at,applied_sequence_version_id,applied_at,
		error_code,error_message,created_at,updated_at
		FROM drama.shot_edit_plans WHERE project_id=$1 AND episode_id=$2 AND shot_edit_plan_id=$3`,
		projectID, episodeID, planID).Scan(&result.ShotEditPlanID, &result.ProjectID, &result.EpisodeID,
		&result.Operation, &result.Status, &result.BaseSequenceVersion, &result.BaseSnapshotHash,
		&requestJSON, &baseJSON, &proposedJSON, &impactJSON, &coverageJSON, &conflictsJSON,
		&handoffJSON, &result.Fingerprint, &result.RequestedBy, &result.ConfirmedBy,
		&result.ConfirmedAt, &result.AppliedSequenceVersionID, &result.AppliedAt,
		&result.ErrorCode, &result.ErrorMessage, &result.CreatedAt, &result.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShotEditPlan{}, ErrNotFound
	}
	if err != nil {
		return ShotEditPlan{}, err
	}
	for _, target := range []struct {
		raw   json.RawMessage
		value any
	}{
		{requestJSON, &result.Request}, {baseJSON, &result.BaseSnapshot}, {proposedJSON, &result.ProposedSnapshot},
		{impactJSON, &result.Impact}, {coverageJSON, &result.CoverageReport}, {conflictsJSON, &result.ContinuityConflicts},
		{handoffJSON, &result.HandoffPreview},
	} {
		if err = json.Unmarshal(target.raw, target.value); err != nil {
			return ShotEditPlan{}, err
		}
	}
	result.RebuildTasks, err = s.listShotEditRebuildTasks(ctx, planID)
	return result, err
}

func (s *Store) ConfirmShotEditPlan(ctx context.Context, projectID, episodeID, planID string, actor *string) (ShotEditPlan, error) {
	tag, err := s.writer.Exec(ctx, `UPDATE drama.shot_edit_plans SET status='confirmed',confirmed_by=$4,
		confirmed_at=now(),updated_at=now(),error_code=NULL,error_message=NULL
		WHERE project_id=$1 AND episode_id=$2 AND shot_edit_plan_id=$3 AND status='validated'
		AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(continuity_conflicts) item
		  WHERE item->>'severity'='blocking')`, projectID, episodeID, planID, actor)
	if err != nil {
		return ShotEditPlan{}, err
	}
	if tag.RowsAffected() == 0 {
		plan, readErr := s.GetShotEditPlan(ctx, projectID, episodeID, planID)
		if readErr != nil {
			return ShotEditPlan{}, readErr
		}
		if plan.Status == "confirmed" || plan.Status == "applied" {
			return plan, nil
		}
		if len(plan.ContinuityConflicts) > 0 {
			return ShotEditPlan{}, fmt.Errorf("%w: preview contains blocking continuity or coverage conflicts", ErrConflict)
		}
		return ShotEditPlan{}, fmt.Errorf("%w: shot edit plan is %s", ErrConflict, plan.Status)
	}
	return s.GetShotEditPlan(ctx, projectID, episodeID, planID)
}

func (s *Store) ExecuteShotEditPlan(ctx context.Context, projectID, episodeID, planID string) (ShotEditPlan, error) {
	result, err := s.executeShotEditPlan(ctx, projectID, episodeID, planID)
	if err == nil {
		return result, nil
	}
	code := "SHOT_EDIT_EXECUTION_FAILED"
	if errors.Is(err, ErrConflict) {
		code = "SHOT_SEQUENCE_CONFLICT"
	}
	message := err.Error()
	_, _ = s.writer.Exec(context.Background(), `UPDATE drama.shot_edit_plans SET status='failed',
		error_code=$4,error_message=$5,updated_at=now()
		WHERE project_id=$1 AND episode_id=$2 AND shot_edit_plan_id=$3 AND status<>'applied'`,
		projectID, episodeID, planID, code, message)
	return ShotEditPlan{}, err
}

func (s *Store) executeShotEditPlan(ctx context.Context, projectID, episodeID, planID string) (ShotEditPlan, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ShotEditPlan{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	var baseVersion int
	var baseHash, operation, fingerprint string
	var requestJSON, proposedJSON, handoffJSON json.RawMessage
	err = tx.QueryRow(ctx, `SELECT status,base_sequence_version,base_snapshot_hash,operation,
		fingerprint,request,proposed_snapshot,handoff_preview FROM drama.shot_edit_plans
		WHERE project_id=$1 AND episode_id=$2 AND shot_edit_plan_id=$3 FOR UPDATE`,
		projectID, episodeID, planID).Scan(&status, &baseVersion, &baseHash, &operation, &fingerprint, &requestJSON, &proposedJSON, &handoffJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return ShotEditPlan{}, ErrNotFound
	}
	if err != nil {
		return ShotEditPlan{}, err
	}
	if status == "applied" {
		_ = tx.Commit(ctx)
		return s.GetShotEditPlan(ctx, projectID, episodeID, planID)
	}
	if status != "confirmed" {
		return ShotEditPlan{}, fmt.Errorf("%w: shot edit plan must be confirmed", ErrConflict)
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('shot-sequence:'||$1::text))`, episodeID); err != nil {
		return ShotEditPlan{}, err
	}
	current, currentVersion, currentSequenceID, err := readCurrentShotSequence(ctx, tx, projectID, episodeID, true)
	if err != nil {
		return ShotEditPlan{}, err
	}
	currentHash, err := shotSequenceHash(current)
	if err != nil {
		return ShotEditPlan{}, err
	}
	if currentVersion != baseVersion || currentHash != baseHash {
		return ShotEditPlan{}, fmt.Errorf("%w: shot sequence changed after preview", ErrConflict)
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.shot_edit_plans SET status='executing',updated_at=now() WHERE shot_edit_plan_id=$1`, planID); err != nil {
		return ShotEditPlan{}, err
	}
	var request shoteditor.Request
	var proposed []shoteditor.Shot
	var handoffs []shoteditor.Handoff
	if err = json.Unmarshal(requestJSON, &request); err != nil {
		return ShotEditPlan{}, err
	}
	if err = s.verifyShotEditorInputs(ctx, projectID, episodeID, request.Metadata); err != nil {
		return ShotEditPlan{}, err
	}
	if err = json.Unmarshal(proposedJSON, &proposed); err != nil {
		return ShotEditPlan{}, err
	}
	if err = json.Unmarshal(handoffJSON, &handoffs); err != nil {
		return ShotEditPlan{}, err
	}

	sequenceID, err := newPublicID("ssv_")
	if err != nil {
		return ShotEditPlan{}, err
	}
	if currentSequenceID == nil {
		baseSequenceID, idErr := newPublicID("ssv_")
		if idErr != nil {
			return ShotEditPlan{}, idErr
		}
		baseJSON, _ := json.Marshal(current)
		_, err = tx.Exec(ctx, `INSERT INTO drama.shot_sequence_versions(shot_sequence_version_id,project_id,
			episode_id,version,snapshot,snapshot_hash,is_current) VALUES($1,$2,$3,$4,$5::jsonb,$6,false)`,
			baseSequenceID, projectID, episodeID, currentVersion, baseJSON, currentHash)
		if err != nil {
			return ShotEditPlan{}, err
		}
		currentSequenceID = &baseSequenceID
	}

	baseByID := shotMap(current)
	proposedByID := shotMap(proposed)
	changedIDs, createdIDs, retiredIDs := shotSequenceDiff(current, proposed)
	newEntityVersions := map[string]string{}
	newArtifacts := map[string]string{}
	for i := range proposed {
		shot := &proposed[i]
		old, wasCurrent := baseByID[shot.ShotID]
		exists, existErr := nativeShotExists(ctx, tx, shot.ShotID)
		if existErr != nil {
			return ShotEditPlan{}, existErr
		}
		if !exists {
			if err = insertNativeShot(ctx, tx, *shot, false); err != nil {
				return ShotEditPlan{}, err
			}
		}
		if !wasCurrent || !sameShotPayload(old, *shot) {
			if wasCurrent {
				if err = ensureShotHistoryVersion(ctx, tx, projectID, old); err != nil {
					return ShotEditPlan{}, err
				}
			}
			entityVersionID, version, versionErr := insertShotSuccessorVersion(ctx, tx, projectID, planID, *shot, exists)
			if versionErr != nil {
				return ShotEditPlan{}, versionErr
			}
			shot.Version = version
			newEntityVersions[shot.ShotID] = entityVersionID
		}
	}
	for _, id := range retiredIDs {
		if err = ensureShotHistoryVersion(ctx, tx, projectID, baseByID[id]); err != nil {
			return ShotEditPlan{}, err
		}
	}

	storyboardArtifacts := map[string]string{}
	for _, shot := range proposed {
		if storyboardArtifacts[shot.StoryboardID] == "" {
			storyboardArtifacts[shot.StoryboardID], err = ensureStoryboardArtifact(ctx, tx, projectID, shot.StoryboardID)
			if err != nil {
				return ShotEditPlan{}, err
			}
		}
		if versionID := newEntityVersions[shot.ShotID]; versionID != "" {
			artifactID, artifactErr := insertShotArtifact(ctx, tx, projectID, planID, shot, versionID, false)
			if artifactErr != nil {
				return ShotEditPlan{}, artifactErr
			}
			newArtifacts[shot.ShotID] = artifactID
			if err = insertArtifactDependency(ctx, tx, storyboardArtifacts[shot.StoryboardID], artifactID, "storyboard_contains", planID+":"+shot.ShotID); err != nil {
				return ShotEditPlan{}, err
			}
		}
	}
	if err = insertShotLineageAndDependencies(ctx, tx, projectID, planID, operation, request, createdIDs, current, proposed, newArtifacts); err != nil {
		return ShotEditPlan{}, err
	}

	restoredFrom := any(nil)
	if operation == shoteditor.OperationRestore {
		restoredFrom = request.SourceSequenceVersionID
	}
	proposedHash, err := shotSequenceHash(proposed)
	if err != nil {
		return ShotEditPlan{}, err
	}
	proposedJSON, _ = json.Marshal(proposed)
	_, err = tx.Exec(ctx, `INSERT INTO drama.shot_sequence_versions(shot_sequence_version_id,project_id,episode_id,
		version,parent_shot_sequence_version_id,restored_from_version_id,shot_edit_plan_id,snapshot,snapshot_hash,is_current)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,false)`, sequenceID, projectID, episodeID, currentVersion+1,
		currentSequenceID, restoredFrom, planID, proposedJSON, proposedHash)
	if err != nil {
		return ShotEditPlan{}, err
	}
	continuityIDs, err := insertShotContinuity(ctx, tx, projectID, episodeID, sequenceID, proposed)
	if err != nil {
		return ShotEditPlan{}, err
	}
	handoffIDs, err := insertShotHandoffs(ctx, tx, projectID, episodeID, sequenceID, handoffs)
	if err != nil {
		return ShotEditPlan{}, err
	}
	if err = markShotArtifactsStale(ctx, tx, projectID, append(changedIDs, retiredIDs...)); err != nil {
		return ShotEditPlan{}, err
	}
	if err = insertShotRebuildTasks(ctx, tx, projectID, episodeID, planID, fingerprint, operation, changedIDs, createdIDs, newEntityVersions, sequenceID); err != nil {
		return ShotEditPlan{}, err
	}
	if err = switchCurrentShotSequence(ctx, tx, projectID, episodeID, planID, sequenceID, currentSequenceID,
		proposed, retiredIDs, newEntityVersions, newArtifacts, continuityIDs, handoffIDs); err != nil {
		return ShotEditPlan{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.shot_edit_plans SET status='applied',applied_sequence_version_id=$2,
		applied_at=now(),updated_at=now() WHERE shot_edit_plan_id=$1`, planID, sequenceID); err != nil {
		return ShotEditPlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ShotEditPlan{}, err
	}
	_ = proposedByID
	return s.GetShotEditPlan(ctx, projectID, episodeID, planID)
}

func (s *Store) ListShotSequenceVersions(ctx context.Context, projectID, episodeID string) ([]ShotSequenceVersion, error) {
	rows, err := s.pool.Query(ctx, `SELECT shot_sequence_version_id,version,parent_shot_sequence_version_id,
		restored_from_version_id,shot_edit_plan_id,snapshot,snapshot_hash,is_current,created_at
		FROM drama.shot_sequence_versions WHERE project_id=$1 AND episode_id=$2 ORDER BY version DESC`, projectID, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ShotSequenceVersion{}
	for rows.Next() {
		var item ShotSequenceVersion
		var snapshot json.RawMessage
		if err = rows.Scan(&item.ShotSequenceVersionID, &item.Version, &item.ParentVersionID, &item.RestoredFromVersionID, &item.ShotEditPlanID, &snapshot, &item.SnapshotHash, &item.IsCurrent, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(snapshot, &item.Snapshot); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func readCurrentShotSequence(ctx context.Context, tx pgx.Tx, projectID, episodeID string, lock bool) ([]shoteditor.Shot, int, *string, error) {
	sequenceLock := ""
	if lock {
		sequenceLock = " FOR UPDATE"
	}
	var sequenceSnapshot json.RawMessage
	var sequenceVersion int
	var sequenceIDValue string
	err := tx.QueryRow(ctx, `SELECT snapshot,version,shot_sequence_version_id
		FROM drama.shot_sequence_versions WHERE project_id=$1 AND episode_id=$2 AND is_current`+sequenceLock,
		projectID, episodeID).Scan(&sequenceSnapshot, &sequenceVersion, &sequenceIDValue)
	if err == nil {
		var shots []shoteditor.Shot
		if err = json.Unmarshal(sequenceSnapshot, &shots); err != nil {
			return nil, 0, nil, err
		}
		if len(shots) == 0 {
			return nil, 0, nil, ErrNotFound
		}
		// Media/frame references are projections of the currently bound shot
		// entity version, never durable sequence content. Rehydrate them against
		// that binding so restore cannot leak legacy media into a successor.
		for i := range shots {
			shots[i].ThumbnailURL, shots[i].HeadFrameRef, shots[i].TailFrameRef = "", "", ""
			var versionID *string
			_ = tx.QueryRow(ctx, `SELECT entity_version_id FROM drama.entity_versions
				WHERE entity_type='shot' AND entity_id=$1 AND is_current`, shots[i].ShotID).Scan(&versionID)
			_ = tx.QueryRow(ctx, `SELECT storage_url FROM drama.storyboard_images
				WHERE shot_id=$1 AND is_current AND (($2::text IS NULL AND shot_entity_version_id IS NULL)
				  OR shot_entity_version_id=$2) ORDER BY generation_version DESC LIMIT 1`,
				shots[i].ShotID, versionID).Scan(&shots[i].ThumbnailURL)
			_ = tx.QueryRow(ctx, `SELECT reference_head_frame_ref FROM drama.shot_handoffs
				WHERE to_shot_id=$1 AND is_current ORDER BY version DESC LIMIT 1`, shots[i].ShotID).Scan(&shots[i].HeadFrameRef)
			_ = tx.QueryRow(ctx, `SELECT target_tail_frame_ref FROM drama.shot_handoffs
				WHERE from_shot_id=$1 AND is_current ORDER BY version DESC LIMIT 1`, shots[i].ShotID).Scan(&shots[i].TailFrameRef)
		}
		return shots, sequenceVersion, &sequenceIDValue, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil, err
	}
	lockSQL := ""
	if lock {
		lockSQL = " FOR UPDATE OF shot"
	}
	rows, err := tx.Query(ctx, `SELECT ((to_jsonb(shot)-'id'-'created_at'-'updated_at'-'is_current'-'retired_by_shot_edit_plan_id')
		||COALESCE(version.content,'{}'::jsonb)||jsonb_build_object(
		'version',COALESCE(version.version,shot.generation_version),
		'thumbnail_url',COALESCE((SELECT image.storage_url FROM drama.storyboard_images image
		  WHERE image.shot_id=shot.shot_id AND image.is_current
		    AND ((version.entity_version_id IS NULL AND image.shot_entity_version_id IS NULL)
		      OR image.shot_entity_version_id=version.entity_version_id)
		  ORDER BY image.generation_version DESC LIMIT 1),''),
		'head_frame_ref',COALESCE((SELECT handoff.reference_head_frame_ref FROM drama.shot_handoffs handoff
		  WHERE handoff.to_shot_id=shot.shot_id AND handoff.is_current ORDER BY handoff.version DESC LIMIT 1),''),
		'tail_frame_ref',COALESCE((SELECT handoff.target_tail_frame_ref FROM drama.shot_handoffs handoff
		  WHERE handoff.from_shot_id=shot.shot_id AND handoff.is_current ORDER BY handoff.version DESC LIMIT 1),'')))
		FROM drama.storyboard_shots shot LEFT JOIN LATERAL(SELECT entity_version_id,version,content
		  FROM drama.entity_versions WHERE entity_type='shot' AND entity_id=shot.shot_id AND is_current) version ON true
		WHERE shot.project_id=$1 AND shot.episode_id=$2 AND shot.is_current ORDER BY shot.shot_order`+lockSQL, projectID, episodeID)
	if err != nil {
		return nil, 0, nil, err
	}
	defer rows.Close()
	shots := []shoteditor.Shot{}
	for rows.Next() {
		var raw json.RawMessage
		if err = rows.Scan(&raw); err != nil {
			return nil, 0, nil, err
		}
		var shot shoteditor.Shot
		if err = json.Unmarshal(raw, &shot); err != nil {
			return nil, 0, nil, err
		}
		shots = append(shots, shot)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, nil, err
	}
	if len(shots) == 0 {
		return nil, 0, nil, ErrNotFound
	}
	var version int
	var sequenceID *string
	err = tx.QueryRow(ctx, `SELECT version,shot_sequence_version_id FROM drama.shot_sequence_versions
		WHERE project_id=$1 AND episode_id=$2 AND is_current`+func() string {
		if lock {
			return " FOR UPDATE"
		}
		return ""
	}(), projectID, episodeID).Scan(&version, &sequenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		version = 1
		sequenceID = nil
		err = nil
	}
	return shots, version, sequenceID, err
}

func readShotSequenceSnapshot(ctx context.Context, tx pgx.Tx, projectID, episodeID, versionID string) ([]shoteditor.Shot, error) {
	var raw json.RawMessage
	err := tx.QueryRow(ctx, `SELECT snapshot FROM drama.shot_sequence_versions
		WHERE project_id=$1 AND episode_id=$2 AND shot_sequence_version_id=$3`, projectID, episodeID, versionID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var result []shoteditor.Shot
	err = json.Unmarshal(raw, &result)
	return result, err
}

type shotEditorResolverBypassKey struct{}

// withShotEditorResolverBypass is package-private so only isolated store tests
// whose fixture predates the Resolver graph can opt out; HTTP callers cannot.
func withShotEditorResolverBypass(ctx context.Context) context.Context {
	return context.WithValue(ctx, shotEditorResolverBypassKey{}, true)
}

func (s *Store) freezeShotEditorInputs(
	ctx context.Context, projectID, episodeID string,
) (map[string]interface{}, error) {
	if bypass, _ := ctx.Value(shotEditorResolverBypassKey{}).(bool); bypass {
		return map[string]interface{}{"resolver_bypass": "isolated_store_integration_test"}, nil
	}
	raw, err := s.ResolveEffectiveInputs(ctx, projectID, episodeID, "storyboard_design")
	if err != nil {
		return nil, err
	}
	var envelope struct {
		ResolutionID   string          `json:"resolution_id"`
		ContextHash    string          `json:"context_hash"`
		ResolutionHash string          `json:"resolution_hash"`
		Status         string          `json:"status"`
		Ready          bool            `json:"ready"`
		Blockers       json.RawMessage `json:"blockers"`
		Context        struct {
			ProductionSnapshot json.RawMessage `json:"production_snapshot"`
		} `json:"context"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil || envelope.ResolutionID == "" ||
		envelope.ContextHash == "" || envelope.ResolutionHash == "" {
		return nil, fmt.Errorf("%w: invalid storyboard effective input resolution", ErrConflict)
	}
	if !envelope.Ready || envelope.Status != "ready" {
		return nil, fmt.Errorf("%w: storyboard effective inputs status=%s ready=%t blockers=%s",
			ErrConflict, envelope.Status, envelope.Ready, envelope.Blockers)
	}
	var snapshot struct {
		State     string `json:"state"`
		VersionID string `json:"version_id"`
		BindingID string `json:"binding_id"`
	}
	if json.Unmarshal(envelope.Context.ProductionSnapshot, &snapshot) != nil ||
		snapshot.State != "resolved" || snapshot.VersionID == "" || snapshot.BindingID == "" {
		return nil, fmt.Errorf("%w: storyboard production snapshot is not an immutable resolved binding", ErrConflict)
	}
	return map[string]interface{}{
		"source_kind": "effective_input_snapshot", "stage": "storyboard_design",
		"resolution_id": envelope.ResolutionID, "context_hash": envelope.ContextHash,
		"resolution_hash":     envelope.ResolutionHash,
		"production_snapshot": json.RawMessage(envelope.Context.ProductionSnapshot),
	}, nil
}

func (s *Store) verifyShotEditorInputs(
	ctx context.Context, projectID, episodeID string, frozen map[string]interface{},
) error {
	if fmt.Sprint(frozen["resolver_bypass"]) == "isolated_store_integration_test" {
		return nil
	}
	if bypass, _ := ctx.Value(shotEditorResolverBypassKey{}).(bool); bypass {
		return nil
	}
	current, err := s.freezeShotEditorInputs(ctx, projectID, episodeID)
	if err != nil {
		return err
	}
	for _, key := range []string{"resolution_hash", "context_hash"} {
		if strings.TrimSpace(fmt.Sprint(frozen[key])) == "" || fmt.Sprint(frozen[key]) != fmt.Sprint(current[key]) {
			return fmt.Errorf("%w: effective storyboard inputs changed after preview", ErrConflict)
		}
	}
	return nil
}

func resolvedShotEditorPayload(
	metadata map[string]interface{},
) ([]shoteditor.Shot, map[string]int64, bool, error) {
	if fmt.Sprint(metadata["resolver_bypass"]) == "isolated_store_integration_test" {
		return nil, nil, false, nil
	}
	value, exists := metadata["production_snapshot"]
	if !exists {
		return nil, nil, false, fmt.Errorf("%w: frozen Resolver snapshot is missing", ErrConflict)
	}
	raw, err := json.Marshal(value)
	if direct, ok := value.(json.RawMessage); ok {
		raw = direct
	}
	if err != nil {
		return nil, nil, false, err
	}
	var snapshot struct {
		State   string `json:"state"`
		Payload struct {
			Shots     []shoteditor.Shot `json:"shots"`
			Dialogues []struct {
				DialogueID          string `json:"dialogue_id"`
				EstimatedDurationMS int64  `json:"estimated_duration_ms"`
			} `json:"dialogues"`
		} `json:"payload"`
	}
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		return nil, nil, false, fmt.Errorf("%w: decode frozen Resolver snapshot: %v", ErrConflict, err)
	}
	if snapshot.State != "resolved" || len(snapshot.Payload.Shots) == 0 {
		return nil, nil, false, fmt.Errorf("%w: frozen Resolver snapshot has no resolved shots", ErrConflict)
	}
	durations := make(map[string]int64, len(snapshot.Payload.Dialogues))
	for _, dialogue := range snapshot.Payload.Dialogues {
		if id := strings.TrimSpace(dialogue.DialogueID); id != "" && dialogue.EstimatedDurationMS > 0 {
			durations[id] = dialogue.EstimatedDurationMS
		}
	}
	return snapshot.Payload.Shots, durations, true, nil
}

func readDialogueDurations(ctx context.Context, tx pgx.Tx, episodeID string) (map[string]int64, error) {
	rows, err := tx.Query(ctx, `SELECT dialogue_id,estimated_duration_ms FROM drama.dialogues WHERE episode_id=$1`, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var id string
		var ms int64
		if err = rows.Scan(&id, &ms); err != nil {
			return nil, err
		}
		result[id] = ms
	}
	return result, rows.Err()
}

func previewShotEditImpact(ctx context.Context, tx pgx.Tx, projectID, episodeID string, preview shoteditor.Preview) (ShotEditImpact, error) {
	impact := ShotEditImpact{ChangedShotIDs: preview.ChangedIDs, CreatedShotIDs: preview.CreatedIDs, RetiredShotIDs: preview.RetiredIDs, StaleArtifacts: []map[string]any{}, RebuildTasks: []map[string]any{}}
	ids := uniqueVersionedEntityIDs(append(append([]string{}, preview.ChangedIDs...), preview.RetiredIDs...))
	if len(ids) > 0 {
		rows, err := tx.Query(ctx, `WITH RECURSIVE walk AS(SELECT artifact_id,artifact_type,native_entity_id,0 depth
		FROM drama.artifacts WHERE project_id=$1 AND native_entity_id=ANY($2) AND is_current UNION
		SELECT child.artifact_id,child.artifact_type,child.native_entity_id,walk.depth+1 FROM walk
		JOIN drama.artifact_dependencies dependency ON dependency.upstream_artifact_id=walk.artifact_id
		JOIN drama.artifacts child ON child.artifact_id=dependency.downstream_artifact_id WHERE child.is_current)
		SELECT DISTINCT artifact_id,artifact_type,native_entity_id,depth FROM walk ORDER BY depth,artifact_type,artifact_id`, projectID, ids)
		if err != nil {
			return impact, err
		}
		for rows.Next() {
			var id, kind, native string
			var depth int
			if err = rows.Scan(&id, &kind, &native, &depth); err != nil {
				rows.Close()
				return impact, err
			}
			impact.StaleArtifacts = append(impact.StaleArtifacts, map[string]any{"artifact_id": id, "artifact_type": kind, "native_entity_id": native, "depth": depth, "after_status": "stale"})
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return impact, err
		}
		rows.Close()
	}
	actions := shotRebuildActions(preview.Operation)
	for _, action := range actions {
		targets := []string{episodeID}
		if action == "regenerate_image" || action == "regenerate_video" {
			targets = preview.ChangedIDs
			if preview.Operation == shoteditor.OperationSplit || preview.Operation == shoteditor.OperationMerge {
				targets = preview.CreatedIDs
			}
		}
		for _, id := range targets {
			impact.RebuildTasks = append(impact.RebuildTasks, map[string]any{"action": action, "target_entity_id": id, "status": "pending", "requires_real_execution": true})
		}
	}
	return impact, nil
}

func publicIDs(prefix string, count int) ([]string, error) {
	result := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id, err := newPublicID(prefix)
		if err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, nil
}
func structuralShots(shots []shoteditor.Shot) []shoteditor.Shot {
	result := shoteditor.CloneShots(shots)
	for i := range result {
		result[i].ThumbnailURL = ""
		result[i].HeadFrameRef = ""
		result[i].TailFrameRef = ""
	}
	return result
}
func shotSequenceHash(shots []shoteditor.Shot) (string, error) {
	return hashJSON(structuralShots(shots))
}
func shotMap(shots []shoteditor.Shot) map[string]shoteditor.Shot {
	result := map[string]shoteditor.Shot{}
	for _, shot := range shots {
		result[shot.ShotID] = shot
	}
	return result
}
func sameStructuralShot(a, b shoteditor.Shot) bool {
	ah, _ := shotSequenceHash([]shoteditor.Shot{a})
	bh, _ := shotSequenceHash([]shoteditor.Shot{b})
	return ah == bh
}
func sameShotPayload(a, b shoteditor.Shot) bool {
	normalize := func(shot *shoteditor.Shot) {
		if shot.CharacterIDs == nil {
			shot.CharacterIDs = []string{}
		}
		if shot.DialogueIDs == nil {
			shot.DialogueIDs = []string{}
		}
		if shot.HeadState == nil {
			shot.HeadState = map[string]any{}
		}
		if shot.TailState == nil {
			shot.TailState = map[string]any{}
		}
		if shot.Performance == nil {
			shot.Performance = map[string]any{}
		}
		if shot.ActionPhase == nil {
			shot.ActionPhase = map[string]any{}
		}
		if shot.ContinuityNotes == nil {
			shot.ContinuityNotes = map[string]any{}
		}
		if shot.SourceSceneData == nil {
			shot.SourceSceneData = map[string]any{}
		}
	}
	normalize(&a)
	normalize(&b)
	a.ShotOrder, a.ShotNumber = 0, 0
	b.ShotOrder, b.ShotNumber = 0, 0
	return sameStructuralShot(a, b)
}
func shotSequenceDiff(base, next []shoteditor.Shot) (changed, created, retired []string) {
	before, after := shotMap(base), shotMap(next)
	for id, shot := range after {
		old, ok := before[id]
		if !ok {
			created = append(created, id)
			changed = append(changed, id)
		} else if !sameShotPayload(old, shot) {
			changed = append(changed, id)
		}
	}
	for id := range before {
		if _, ok := after[id]; !ok {
			retired = append(retired, id)
		}
	}
	sort.Strings(changed)
	sort.Strings(created)
	sort.Strings(retired)
	return
}
func nativeShotExists(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.storyboard_shots WHERE shot_id=$1)`, id).Scan(&exists)
	return exists, err
}

func insertNativeShot(ctx context.Context, tx pgx.Tx, shot shoteditor.Shot, current bool) error {
	characters, _ := json.Marshal(shot.CharacterIDs)
	dialogues, _ := json.Marshal(shot.DialogueIDs)
	continuity, _ := json.Marshal(shot.ContinuityNotes)
	source, _ := json.Marshal(shot.SourceSceneData)
	head, _ := json.Marshal(shot.HeadState)
	tail, _ := json.Marshal(shot.TailState)
	performance, _ := json.Marshal(shot.Performance)
	phase, _ := json.Marshal(shot.ActionPhase)
	_, err := tx.Exec(ctx, `INSERT INTO drama.storyboard_shots(shot_id,storyboard_id,project_id,episode_id,scene_id,
		shot_number,shot_order,duration_seconds,shot_size,camera_angle,camera_motion,composition,character_ids,
		location_id,action_description,facial_expression,dialogue_ids,subtitle_text,narration_text,lighting,
		atmosphere,sound_effect_hint,bgm_hint,transition_type,visual_prompt_base,video_prompt_base,
		negative_prompt_base,continuity_notes,source_scene_data,status,generation_version,is_current,
		lineage_root_shot_id,head_state,tail_state,performance,action_phase,axis,coverage_role,coverage_group,coverage_side)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14,$15,$16,$17::jsonb,$18,$19,$20,
		$21,$22,$23,$24,$25,$26,$27,$28::jsonb,$29::jsonb,$30,$31,$32,$33,$34::jsonb,$35::jsonb,
		$36::jsonb,$37::jsonb,$38,$39,$40,$41)`, shot.ShotID, shot.StoryboardID, shot.ProjectID, shot.EpisodeID, shot.SceneID,
		shot.ShotNumber, shot.ShotOrder, shot.DurationSeconds, shot.ShotSize, shot.CameraAngle, shot.CameraMotion,
		shot.Composition, characters, shotNullableString(shot.LocationID), shot.ActionDescription, shot.FacialExpression,
		dialogues, shot.SubtitleText, shot.NarrationText, shot.Lighting, shot.Atmosphere, shot.SoundEffectHint, shot.BGMHint,
		defaultString(shot.TransitionType, "cut"), shot.VisualPromptBase, shot.VideoPromptBase, shot.NegativePromptBase,
		continuity, source, defaultString(shot.Status, "draft"), max(1, shot.GenerationVersion), current,
		defaultString(shot.LineageRootShotID, shot.ShotID), head, tail, performance, phase, shot.Axis, shot.CoverageRole,
		shot.CoverageGroup, shot.CoverageSide)
	return err
}

func shotVersionContent(shot shoteditor.Shot) json.RawMessage {
	shot.ThumbnailURL = ""
	shot.HeadFrameRef = ""
	shot.TailFrameRef = ""
	raw, _ := json.Marshal(shot)
	return raw
}
func insertShotSuccessorVersion(ctx context.Context, tx pgx.Tx, projectID, planID string, shot shoteditor.Shot, nativeExists bool) (string, int, error) {
	var parentID *string
	var maxVersion int
	err := tx.QueryRow(ctx, `SELECT entity_version_id,version FROM drama.entity_versions WHERE entity_type='shot' AND entity_id=$1 ORDER BY version DESC LIMIT 1 FOR UPDATE`, shot.ShotID).Scan(&parentID, &maxVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
		maxVersion = 0
		if nativeExists {
			if err = ensureShotHistoryVersion(ctx, tx, projectID, shot); err != nil {
				return "", 0, err
			}
			err = tx.QueryRow(ctx, `SELECT entity_version_id,version FROM drama.entity_versions WHERE entity_type='shot' AND entity_id=$1 ORDER BY version DESC LIMIT 1`, shot.ShotID).Scan(&parentID, &maxVersion)
		}
	}
	if err != nil {
		return "", 0, err
	}
	version := maxVersion + 1
	if !nativeExists {
		version = 1
		parentID = nil
	}
	id, err := newPublicID("ev_")
	if err != nil {
		return "", 0, err
	}
	content := shotVersionContent(shot)
	hash, err := hashJSON(content)
	if err != nil {
		return "", 0, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.entity_versions(entity_version_id,project_id,entity_type,entity_id,
		version,parent_entity_version_id,content,content_hash,semantic_hash,source_type,source_metadata,is_current)
		VALUES($1,$2,'shot',$3,$4,$5,$6::jsonb,$7,$7,'local_edit',jsonb_build_object('shot_edit_plan_id',$8::text),false)`, id, projectID, shot.ShotID, version, parentID, content, hash, planID)
	return id, version, err
}

func ensureShotHistoryVersion(ctx context.Context, tx pgx.Tx, projectID string, shot shoteditor.Shot) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.entity_versions WHERE entity_type='shot' AND entity_id=$1)`, shot.ShotID).Scan(&exists); err != nil || exists {
		return err
	}
	id, err := newPublicID("ev_")
	if err != nil {
		return err
	}
	version := max(1, shot.Version)
	content := shotVersionContent(shot)
	hash, err := hashJSON(content)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.entity_versions(entity_version_id,project_id,entity_type,entity_id,version,
		content,content_hash,semantic_hash,source_type,is_current) VALUES($1,$2,'shot',$3,$4,$5::jsonb,$6,$6,'generated',false)`, id, projectID, shot.ShotID, version, content, hash)
	return err
}

func ensureStoryboardArtifact(ctx context.Context, tx pgx.Tx, projectID, storyboardID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT artifact_id FROM drama.artifacts WHERE project_id=$1 AND artifact_type='storyboard' AND native_entity_id=$2 AND is_current`, projectID, storyboardID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	id, err = newPublicID("art_")
	if err != nil {
		return "", err
	}
	hash, err := hashJSON(map[string]any{"storyboard_id": storyboardID})
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,validity_status,is_current,idempotency_key) VALUES($1,'storyboard',$2,$3,COALESCE((SELECT version FROM drama.storyboards WHERE storyboard_id=$3),1),$4,'valid',true,$5)`, id, projectID, storyboardID, hash, "shot-editor-storyboard:"+storyboardID)
	return id, err
}

func ensureShotArtifact(ctx context.Context, tx pgx.Tx, projectID string, shot shoteditor.Shot) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT artifact_id FROM drama.artifacts WHERE project_id=$1 AND artifact_type='storyboard_shot' AND native_entity_id=$2 AND is_current`, projectID, shot.ShotID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	versionID := ""
	_ = tx.QueryRow(ctx, `SELECT entity_version_id FROM drama.entity_versions WHERE entity_type='shot' AND entity_id=$1 ORDER BY version DESC LIMIT 1`, shot.ShotID).Scan(&versionID)
	return insertShotArtifact(ctx, tx, projectID, "history", shot, versionID, true)
}

func insertShotArtifact(ctx context.Context, tx pgx.Tx, projectID, planID string, shot shoteditor.Shot, entityVersionID string, current bool) (string, error) {
	id, err := newPublicID("art_")
	if err != nil {
		return "", err
	}
	content := shotVersionContent(shot)
	hash, err := hashJSON(content)
	if err != nil {
		return "", err
	}
	revision := max(1, shot.Version)
	_, err = tx.Exec(ctx, `INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,validity_status,is_current,idempotency_key,metadata) VALUES($1,'storyboard_shot',$2,$3,$4,$5,$6,$7,$8,jsonb_build_object('entity_version_id',$9::text,'shot_edit_plan_id',$10::text))`, id, projectID, shot.ShotID, revision, hash, func() string {
		if current {
			return "valid"
		}
		return "needs_review"
	}(), current, "shot-editor-artifact:"+planID+":"+shot.ShotID+":"+fmt.Sprint(revision), shotNullableString(entityVersionID), planID)
	return id, err
}

func insertArtifactDependency(ctx context.Context, tx pgx.Tx, upstream, downstream, kind, key string) error {
	var hash string
	if err := tx.QueryRow(ctx, `SELECT content_hash FROM drama.artifacts WHERE artifact_id=$1`, upstream).Scan(&hash); err != nil {
		return err
	}
	id, err := newPublicID("ad_")
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.artifact_dependencies(artifact_dependency_id,upstream_artifact_id,downstream_artifact_id,dependency_type,dependency_selector,observed_upstream_hash,idempotency_key) VALUES($1,$2,$3,$4,'{}'::jsonb,$5,$6) ON CONFLICT(idempotency_key) DO NOTHING`, id, upstream, downstream, kind, hash, "shot-editor-dependency:"+key)
	return err
}

func insertShotLineageAndDependencies(ctx context.Context, tx pgx.Tx, projectID, planID, operation string, request shoteditor.Request, createdIDs []string, base, proposed []shoteditor.Shot, newArtifacts map[string]string) error {
	relations := [][2]string{}
	relation := ""
	if operation == shoteditor.OperationSplit {
		relation = "split_into"
		for _, target := range createdIDs {
			relations = append(relations, [2]string{request.ShotID, target})
		}
	}
	if operation == shoteditor.OperationMerge {
		relation = "merged_into"
		if len(createdIDs) != 1 {
			return fmt.Errorf("%w: merge successor identity is missing", shoteditor.ErrInvalidEdit)
		}
		for _, source := range request.ShotIDs {
			relations = append(relations, [2]string{source, createdIDs[0]})
		}
	}
	baseMap := shotMap(base)
	for _, pair := range relations {
		lineageID, err := newPublicID("sl_")
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO drama.shot_lineage(shot_lineage_id,shot_edit_plan_id,source_shot_id,target_shot_id,relation) VALUES($1,$2,$3,$4,$5)`, lineageID, planID, pair[0], pair[1], relation); err != nil {
			return err
		}
		sourceArtifact, err := ensureShotArtifact(ctx, tx, projectID, baseMap[pair[0]])
		if err != nil {
			return err
		}
		if targetArtifact := newArtifacts[pair[1]]; targetArtifact != "" {
			if err = insertArtifactDependency(ctx, tx, sourceArtifact, targetArtifact, relation, planID+":"+pair[0]+":"+pair[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertShotContinuity(ctx context.Context, tx pgx.Tx, projectID, episodeID, sequenceID string, shots []shoteditor.Shot) ([]string, error) {
	var episodeNumber int
	if err := tx.QueryRow(ctx, `SELECT episode_number FROM drama.episode_outlines WHERE episode_id=$1`, episodeID).Scan(&episodeNumber); err != nil {
		return nil, err
	}
	ids := []string{}
	for _, shot := range shots {
		id, err := newPublicID("cle_")
		if err != nil {
			return nil, err
		}
		input, _ := json.Marshal(shot.HeadState)
		output, _ := json.Marshal(shot.TailState)
		stateHash, err := hashJSON(map[string]any{"input": shot.HeadState, "output": shot.TailState})
		if err != nil {
			return nil, err
		}
		var inherited *string
		_ = tx.QueryRow(ctx, `SELECT continuity_entry_id FROM drama.continuity_ledger_entries WHERE project_id=$1 AND episode_id=$2 AND shot_id=$3 AND is_current ORDER BY updated_at DESC LIMIT 1`, projectID, episodeID, shot.ShotID).Scan(&inherited)
		_, err = tx.Exec(ctx, `INSERT INTO drama.continuity_ledger_entries(continuity_entry_id,project_id,episode_id,episode_number,scene_id,shot_id,scope,sequence_number,input_state,output_state,inherited_from_entry_id,validation_status,diagnostics,state_hash,is_current,shot_sequence_version_id,continuity_version) VALUES($1,$2,$3,$4,$5,$6,'shot',$7,$8::jsonb,$9::jsonb,$10,'valid','[]'::jsonb,$11,false,$12,(SELECT COALESCE(max(continuity_version),0)+1 FROM drama.continuity_ledger_entries WHERE project_id=$2 AND episode_id=$3 AND scope='shot' AND sequence_number=$7))`, id, projectID, episodeID, episodeNumber, shot.SceneID, shot.ShotID, shot.ShotOrder, input, output, inherited, stateHash, sequenceID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func insertShotHandoffs(ctx context.Context, tx pgx.Tx, projectID, episodeID, sequenceID string, handoffs []shoteditor.Handoff) ([]string, error) {
	ids := []string{}
	for _, handoff := range handoffs {
		id, err := newPublicID("sh_")
		if err != nil {
			return nil, err
		}
		pose, _ := json.Marshal(handoff.PoseConstraints)
		diagnostics, _ := json.Marshal(handoff.Diagnostics)
		var version int
		if err = tx.QueryRow(ctx, `SELECT COALESCE(max(version)+1,1) FROM drama.shot_handoffs WHERE from_shot_id=$1 AND to_shot_id=$2`, handoff.FromShotID, handoff.ToShotID).Scan(&version); err != nil {
			return nil, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO drama.shot_handoffs(shot_handoff_id,project_id,episode_id,from_shot_id,to_shot_id,target_tail_frame_ref,reference_head_frame_ref,pose_constraints,gaze_constraint,motion_direction,from_action_phase,to_action_phase,shot_size_constraint,composition_constraint,version,status,diagnostics,is_current,shot_sequence_version_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,false,$18)`, id, projectID, episodeID, handoff.FromShotID, handoff.ToShotID, shotNullableString(handoff.TargetTailFrameRef), shotNullableString(handoff.ReferenceHeadFrame), pose, handoff.GazeConstraint, handoff.MotionDirection, handoff.FromActionPhase, handoff.ToActionPhase, handoff.ShotSizeConstraint, handoff.CompositionConstraint, version, handoff.Status, diagnostics, sequenceID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func markShotArtifactsStale(ctx context.Context, tx pgx.Tx, projectID string, shotIDs []string) error {
	shotIDs = uniqueVersionedEntityIDs(shotIDs)
	if len(shotIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `WITH RECURSIVE walk AS(SELECT artifact_id FROM drama.artifacts WHERE project_id=$1 AND native_entity_id=ANY($2) AND is_current UNION SELECT child.artifact_id FROM walk JOIN drama.artifact_dependencies dependency ON dependency.upstream_artifact_id=walk.artifact_id JOIN drama.artifacts child ON child.artifact_id=dependency.downstream_artifact_id WHERE child.is_current) UPDATE drama.artifacts SET validity_status='stale' WHERE artifact_id IN(SELECT artifact_id FROM walk)`, projectID, shotIDs)
	return err
}

func shotRebuildActions(operation string) []string {
	switch operation {
	case shoteditor.OperationReorder:
		return []string{"update_continuity", "recompose_timeline"}
	default:
		return []string{"regenerate_image", "regenerate_video", "update_continuity", "recompose_timeline"}
	}
}
func insertShotRebuildTasks(ctx context.Context, tx pgx.Tx, projectID, episodeID, planID, fingerprint, operation string, changedIDs, createdIDs []string, versionIDs map[string]string, sequenceID string) error {
	number := 0
	for _, action := range shotRebuildActions(operation) {
		targets := []string{episodeID}
		targetType := "shot_sequence"
		if action == "regenerate_image" || action == "regenerate_video" {
			targets = changedIDs
			if operation == shoteditor.OperationSplit || operation == shoteditor.OperationMerge {
				targets = createdIDs
			}
			targetType = "shot"
		}
		for _, target := range uniqueVersionedEntityIDs(targets) {
			number++
			taskID := "sirt_" + fingerprint[:16] + "_" + fmt.Sprint(number)
			versionID := any(nil)
			if targetType == "shot" {
				versionID = shotNullableString(versionIDs[target])
			}
			_, err := tx.Exec(ctx, `INSERT INTO drama.incremental_rebuild_tasks(rebuild_task_id,change_plan_id,shot_edit_plan_id,project_id,action,target_entity_type,target_entity_id,target_entity_version_id,status,provider,input,output) VALUES($1,NULL,$2,$3,$4,$5,$6,$7,'pending','workflow',jsonb_build_object('plan_fingerprint',$8::text,'requires_real_execution',true,'shot_sequence_version_id',$9::text,'shot_entity_version_id',$7::text),'{}'::jsonb)`, taskID, planID, projectID, action, targetType, target, versionID, fingerprint, sequenceID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func switchCurrentShotSequence(ctx context.Context, tx pgx.Tx, projectID, episodeID, planID, sequenceID string, currentSequenceID *string, proposed []shoteditor.Shot, retiredIDs []string, newVersions, newArtifacts map[string]string, continuityIDs, handoffIDs []string) error {
	if _, err := tx.Exec(ctx, `SELECT drama.materialize_shot_sequence_projection($1,$2,$3,$4)`,
		projectID, episodeID, planID, sequenceID); err != nil {
		return err
	}
	versionIDs := mapValues(newVersions)
	affected := append(mapKeys(newVersions), retiredIDs...)
	if len(affected) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE drama.entity_versions SET is_current=false WHERE entity_type='shot' AND entity_id=ANY($1) AND is_current`, uniqueVersionedEntityIDs(affected)); err != nil {
			return err
		}
	}
	if len(versionIDs) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE drama.entity_versions SET is_current=true WHERE entity_version_id=ANY($1)`, versionIDs); err != nil {
			return err
		}
	}
	artifactIDs := mapValues(newArtifacts)
	if len(newArtifacts) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE drama.artifacts SET is_current=false WHERE artifact_type='storyboard_shot' AND native_entity_id=ANY($1) AND is_current`, mapKeys(newArtifacts)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE drama.artifacts SET is_current=true WHERE artifact_id=ANY($1)`, artifactIDs); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.continuity_ledger_entries SET is_current=false,validation_status=CASE WHEN validation_status='superseded' THEN validation_status ELSE 'superseded' END WHERE project_id=$1 AND episode_id=$2 AND scope='shot' AND is_current`, projectID, episodeID); err != nil {
		return err
	}
	if len(continuityIDs) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE drama.continuity_ledger_entries SET is_current=true WHERE continuity_entry_id=ANY($1)`, continuityIDs); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.shot_handoffs SET is_current=false WHERE project_id=$1 AND episode_id=$2 AND is_current`, projectID, episodeID); err != nil {
		return err
	}
	if len(handoffIDs) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE drama.shot_handoffs SET is_current=true WHERE shot_handoff_id=ANY($1)`, handoffIDs); err != nil {
			return err
		}
	}
	if currentSequenceID != nil {
		if _, err := tx.Exec(ctx, `UPDATE drama.shot_sequence_versions SET is_current=false WHERE shot_sequence_version_id=$1`, *currentSequenceID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.shot_sequence_versions SET is_current=true WHERE shot_sequence_version_id=$1`, sequenceID); err != nil {
		return err
	}
	return nil
}

func (s *Store) listShotEditRebuildTasks(ctx context.Context, planID string) ([]IncrementalRebuild, error) {
	rows, err := s.pool.Query(ctx, `SELECT rebuild_task_id,action,target_entity_type,target_entity_id,artifact_id,range_start_ms,range_end_ms,status,provider,input,output,error_code,error_message,created_at,completed_at FROM drama.incremental_rebuild_tasks WHERE shot_edit_plan_id=$1 ORDER BY created_at,rebuild_task_id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []IncrementalRebuild{}
	for rows.Next() {
		var item IncrementalRebuild
		if err = rows.Scan(&item.RebuildTaskID, &item.Action, &item.TargetEntityType, &item.TargetEntityID, &item.ArtifactID, &item.RangeStartMS, &item.RangeEndMS, &item.Status, &item.Provider, &item.Input, &item.Output, &item.ErrorCode, &item.ErrorMessage, &item.CreatedAt, &item.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) UpdateShotEditRebuildTaskStatus(
	ctx context.Context, projectID, episodeID, planID, taskID string, input RebuildTaskStatusInput,
) (IncrementalRebuild, error) {
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status != "cancelled" {
		return IncrementalRebuild{}, fmt.Errorf("%w: rebuild execution state is worker-owned; this endpoint only accepts cancelled", shoteditor.ErrInvalidEdit)
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return IncrementalRebuild{}, err
	}
	defer tx.Rollback(ctx)
	var task IncrementalRebuild
	err = tx.QueryRow(ctx, `UPDATE drama.incremental_rebuild_tasks task SET
		status='cancelled',error_code='REBUILD_CANCELLED_BY_USER',error_message=$5,
		completed_at=now(),claim_token=NULL,lease_owner=NULL,lease_expires_at=NULL,heartbeat_at=NULL,updated_at=now()
		FROM drama.shot_edit_plans plan
		WHERE task.shot_edit_plan_id=plan.shot_edit_plan_id AND plan.project_id=$1 AND plan.episode_id=$2
		  AND task.shot_edit_plan_id=$3 AND task.rebuild_task_id=$4
		  AND task.status IN('pending','claimed','running','retry_wait')
		RETURNING task.rebuild_task_id,task.action,task.target_entity_type,task.target_entity_id,
		  task.artifact_id,task.range_start_ms,task.range_end_ms,task.status,task.provider,
		  task.input,task.output,task.error_code,task.error_message,task.created_at,task.completed_at`,
		projectID, episodeID, planID, taskID, input.ErrorMessage).Scan(
		&task.RebuildTaskID, &task.Action, &task.TargetEntityType, &task.TargetEntityID, &task.ArtifactID,
		&task.RangeStartMS, &task.RangeEndMS, &task.Status, &task.Provider, &task.Input, &task.Output,
		&task.ErrorCode, &task.ErrorMessage, &task.CreatedAt, &task.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return IncrementalRebuild{}, ErrConflict
	}
	if err != nil {
		return IncrementalRebuild{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return IncrementalRebuild{}, err
	}
	return task, nil
}

func shotNullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func mapKeys(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
func mapValues(values map[string]string) []string {
	keys := mapKeys(values)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		if values[key] != "" {
			result = append(result, values[key])
		}
	}
	return result
}
