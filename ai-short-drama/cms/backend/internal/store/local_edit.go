package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/localedit"
)

type ChangePlan struct {
	ChangePlanID   string               `json:"change_plan_id"`
	ProjectID      string               `json:"project_id"`
	Status         string               `json:"status"`
	Plan           localedit.Plan       `json:"plan"`
	Fingerprint    string               `json:"fingerprint"`
	Impacts        []ChangePlanImpact   `json:"impacts"`
	RebuildTasks   []IncrementalRebuild `json:"rebuild_tasks"`
	RequestedBy    *string              `json:"requested_by,omitempty"`
	ConfirmedBy    *string              `json:"confirmed_by,omitempty"`
	ConfirmedAt    *time.Time           `json:"confirmed_at,omitempty"`
	AppliedAt      *time.Time           `json:"applied_at,omitempty"`
	ErrorCode      *string              `json:"error_code,omitempty"`
	ErrorMessage   *string              `json:"error_message,omitempty"`
	ReviewMetadata json.RawMessage      `json:"review_metadata"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

type ChangePlanImpact struct {
	ArtifactID       string          `json:"artifact_id"`
	ArtifactType     string          `json:"artifact_type"`
	NativeEntityID   string          `json:"native_entity_id"`
	PropagationDepth int             `json:"propagation_depth"`
	BeforeStatus     string          `json:"before_status"`
	AfterStatus      string          `json:"after_status"`
	DependencyPath   json.RawMessage `json:"dependency_path"`
	RebuildAction    *string         `json:"rebuild_action,omitempty"`
}

type IncrementalRebuild struct {
	RebuildTaskID    string          `json:"rebuild_task_id"`
	Action           string          `json:"action"`
	TargetEntityType string          `json:"target_entity_type"`
	TargetEntityID   string          `json:"target_entity_id"`
	ArtifactID       *string         `json:"artifact_id,omitempty"`
	RangeStartMS     *int64          `json:"range_start_ms,omitempty"`
	RangeEndMS       *int64          `json:"range_end_ms,omitempty"`
	Status           string          `json:"status"`
	Provider         string          `json:"provider"`
	Input            json.RawMessage `json:"input"`
	Output           json.RawMessage `json:"output"`
	ErrorCode        *string         `json:"error_code,omitempty"`
	ErrorMessage     *string         `json:"error_message,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
}

type RebuildTaskStatusInput struct {
	Status       string          `json:"status"`
	Output       json.RawMessage `json:"output,omitempty"`
	ErrorCode    *string         `json:"error_code,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
}

type EntityVersion struct {
	EntityVersionID string          `json:"entity_version_id"`
	EntityType      string          `json:"entity_type"`
	EntityID        string          `json:"entity_id"`
	Version         int             `json:"version"`
	Content         json.RawMessage `json:"content"`
	ContentHash     string          `json:"content_hash"`
	SemanticHash    string          `json:"semantic_hash"`
	SourceType      string          `json:"source_type"`
	IsCurrent       bool            `json:"is_current"`
	CreatedAt       time.Time       `json:"created_at"`
}

type ChangeComment struct {
	CommentID       string    `json:"comment_id"`
	ProjectID       string    `json:"project_id"`
	EntityType      string    `json:"entity_type"`
	EntityID        string    `json:"entity_id"`
	EntityVersion   *int      `json:"entity_version,omitempty"`
	TimecodeStartMS *int64    `json:"timecode_start_ms,omitempty"`
	TimecodeEndMS   *int64    `json:"timecode_end_ms,omitempty"`
	Body            string    `json:"body"`
	Author          *string   `json:"author,omitempty"`
	Resolved        bool      `json:"resolved"`
	CreatedAt       time.Time `json:"created_at"`
}

type CreateChangeCommentInput struct {
	EntityType      string
	EntityID        string
	EntityVersion   *int
	TimecodeStartMS *int64
	TimecodeEndMS   *int64
	Body            string
	Author          *string
}

func (s *Store) CreateChangePlan(ctx context.Context, projectID string, plan localedit.Plan, requestedBy *string) (ChangePlan, error) {
	if err := localedit.Validate(plan); err != nil {
		return ChangePlan{}, err
	}
	changePlanID, err := newPublicID("cp_")
	if err != nil {
		return ChangePlan{}, err
	}
	eventID, err := newPublicID("cpe_")
	if err != nil {
		return ChangePlan{}, err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ChangePlan{}, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.projects WHERE project_id=$1)`, strings.TrimSpace(projectID)).Scan(&exists); err != nil {
		return ChangePlan{}, err
	}
	if !exists {
		return ChangePlan{}, ErrNotFound
	}
	targetHash := ""
	var targetContent json.RawMessage
	if plan.Target.EntityType != "media" {
		targetContent, err = readEntitySnapshot(ctx, tx, plan.Target.EntityType, plan.Target.EntityID)
		if err != nil {
			return ChangePlan{}, err
		}
		currentVersion, versionErr := readCurrentEntityVersion(
			ctx, tx, plan.Target.EntityType, plan.Target.EntityID,
		)
		if versionErr != nil {
			return ChangePlan{}, versionErr
		}
		if currentVersion != plan.Target.Version {
			return ChangePlan{}, fmt.Errorf("%w: target version is %d, current is %d",
				ErrConflict, plan.Target.Version, currentVersion)
		}
		var hashErr error
		targetHash, hashErr = hashJSON(json.RawMessage(targetContent))
		if hashErr != nil {
			return ChangePlan{}, hashErr
		}
		plan.ExpectedChanges, err = enrichChangesWithDiff(targetContent, plan.Target.EntityType, plan.ExpectedChanges)
		if err != nil {
			return ChangePlan{}, err
		}
		plan.ExpectedChanges, err = enrichChangeTimeRanges(
			ctx, tx, plan.Target.EntityType, plan.Target.EntityID, plan.ExpectedChanges,
		)
		if err != nil {
			return ChangePlan{}, err
		}
	}
	fingerprint := localedit.Fingerprint(plan)
	mustPreserve, _ := json.Marshal(plan.MustPreserve)
	allowedFieldsJSON, _ := json.Marshal(plan.AllowedFields)
	changesJSON, _ := json.Marshal(plan.ExpectedChanges)
	upstreamJSON, _ := json.Marshal(plan.Impact.Upstream)
	downstreamJSON, _ := json.Marshal(plan.Impact.Downstream)
	rebuildJSON, _ := json.Marshal(plan.Rebuild)
	rebuildTasksJSON, _ := json.Marshal(plan.Impact.RebuildTasks)
	risksJSON, _ := json.Marshal(plan.Risks)
	rulesJSON, _ := json.Marshal(plan.ValidationRules)
	locksJSON, _ := json.Marshal(plan.Locks)
	requestDedupKey := projectID + ":" + fingerprint + ":" + strconv.Itoa(plan.Target.Version)
	var persistedPlanID string
	err = tx.QueryRow(ctx, `INSERT INTO drama.change_plans(
		change_plan_id,project_id,status,user_intent,natural_language_instruction,
		target_entity_type,target_entity_id,target_version,must_preserve,allowed_fields,
		expected_changes,affected_upstream,affected_downstream,rebuild_decision,rebuild_tasks,
		risks,validation_rules,rollback_version,change_kind,semantic_change,locks,
		plan_fingerprint,requested_by,request_dedup_key)
		VALUES($1,$2,'validated',$3,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,
		$10::jsonb,$11::jsonb,$12::jsonb,$13::jsonb,$14::jsonb,$15::jsonb,$16,$17,$18,
		$19::jsonb,$20,$21,$22)
		ON CONFLICT(request_dedup_key) WHERE request_dedup_key IS NOT NULL DO NOTHING
		RETURNING change_plan_id`,
		changePlanID, projectID, plan.UserIntent, plan.Target.EntityType, plan.Target.EntityID,
		plan.Target.Version, mustPreserve, allowedFieldsJSON, changesJSON, upstreamJSON,
		downstreamJSON, rebuildJSON, rebuildTasksJSON, risksJSON, rulesJSON,
		plan.RollbackVersion, plan.ChangeKind, plan.SemanticChange, locksJSON, fingerprint, requestedBy,
		requestDedupKey).Scan(&persistedPlanID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `SELECT change_plan_id FROM drama.change_plans
			WHERE project_id=$1 AND plan_fingerprint=$2 AND target_version=$3`,
			projectID, fingerprint, plan.Target.Version).Scan(&persistedPlanID); err != nil {
			return ChangePlan{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return ChangePlan{}, err
		}
		return s.GetChangePlan(ctx, projectID, persistedPlanID)
	}
	if err != nil {
		return ChangePlan{}, err
	}
	if plan.Target.EntityType != "media" {
		if _, err = tx.Exec(ctx, `UPDATE drama.change_plans SET target_content_hash=$2
			WHERE change_plan_id=$1`, changePlanID, targetHash); err != nil {
			return ChangePlan{}, err
		}
	}
	if plan.SemanticChange {
		impactIDs := impactedNativeEntityIDs(plan)
		if plan.Target.EntityType == "timeline" {
			var timeline map[string]any
			if json.Unmarshal(targetContent, &timeline) == nil {
				impactIDs = append(impactIDs, strings.TrimSpace(fmt.Sprint(timeline["timeline_id"])))
			}
		}
		for _, nativeEntityID := range uniqueVersionedEntityIDs(impactIDs) {
			if err = insertExactChangePlanImpacts(ctx, tx, changePlanID, projectID, nativeEntityID, plan.ChangeKind); err != nil {
				return ChangePlan{}, err
			}
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.change_plan_events(change_plan_event_id,change_plan_id,event_type,actor)
		VALUES($1,$2,'created',$3)`, eventID, changePlanID, requestedBy); err != nil {
		return ChangePlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ChangePlan{}, err
	}
	return s.GetChangePlan(ctx, projectID, changePlanID)
}

func insertExactChangePlanImpacts(ctx context.Context, tx pgx.Tx, planID, projectID, nativeEntityID, changeKind string) error {
	_, err := tx.Exec(ctx, `WITH RECURSIVE walk AS (
		SELECT artifact.artifact_id,artifact.artifact_type,artifact.native_entity_id,
			artifact.validity_status,0 AS depth,ARRAY[artifact.artifact_id]::text[] AS path
		FROM drama.artifacts artifact
		WHERE artifact.project_id=$2 AND artifact.native_entity_id=$3 AND artifact.is_current
		UNION ALL
		SELECT downstream.artifact_id,downstream.artifact_type,downstream.native_entity_id,
			downstream.validity_status,walk.depth+1,walk.path||downstream.artifact_id
		FROM walk
		JOIN drama.artifact_dependencies dependency ON dependency.upstream_artifact_id=walk.artifact_id
		JOIN drama.artifacts downstream ON downstream.artifact_id=dependency.downstream_artifact_id
		WHERE dependency.invalidates_on ? $4
		  AND downstream.is_current
		  AND NOT downstream.artifact_id=ANY(walk.path)
	), collapsed AS (
		SELECT DISTINCT ON (artifact_id) artifact_id,artifact_type,native_entity_id,
			validity_status,depth,path FROM walk WHERE depth>0 ORDER BY artifact_id,depth
	)
	INSERT INTO drama.change_plan_impacts(
		change_plan_impact_id,change_plan_id,artifact_id,artifact_type,native_entity_id,
		propagation_depth,before_status,after_status,dependency_path,rebuild_action)
	SELECT 'cpi_'||md5($1||':'||artifact_id),$1,artifact_id,artifact_type,native_entity_id,
		depth,validity_status,'stale',to_jsonb(path),
		CASE artifact_type
		  WHEN 'dialogue_audio' THEN 'regenerate_voice'
		  WHEN 'storyboard_image' THEN 'regenerate_image'
		  WHEN 'shot_video' THEN 'regenerate_video'
		  WHEN 'edit_timeline' THEN 'recompose_timeline'
		  ELSE NULL
		END
	FROM collapsed
	ON CONFLICT(change_plan_id,artifact_id) DO NOTHING`, planID, projectID, nativeEntityID, changeKind)
	return err
}

func (s *Store) ConfirmChangePlan(ctx context.Context, projectID, planID string, actor *string) (ChangePlan, error) {
	eventID, err := newPublicID("cpe_")
	if err != nil {
		return ChangePlan{}, err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ChangePlan{}, err
	}
	defer tx.Rollback(ctx)
	var seasonPlanConfirmation bool
	err = tx.QueryRow(ctx, `SELECT target_entity_type='adaptation_plan'
		AND review_metadata ? 'preview_fingerprint'
		FROM drama.change_plans WHERE project_id=$1 AND change_plan_id=$2 FOR UPDATE`,
		projectID, planID).Scan(&seasonPlanConfirmation)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangePlan{}, ErrNotFound
	}
	if err != nil {
		return ChangePlan{}, err
	}
	if seasonPlanConfirmation {
		return ChangePlan{}, fmt.Errorf("%w: season change plans must use the atomic season confirmation endpoint", ErrConflict)
	}
	tag, err := tx.Exec(ctx, `UPDATE drama.change_plans SET status='confirmed',
		confirmed_by=$3,confirmed_at=now(),error_code=NULL,error_message=NULL
		WHERE project_id=$1 AND change_plan_id=$2 AND status='validated'`, projectID, planID, actor)
	if err != nil {
		return ChangePlan{}, err
	}
	if tag.RowsAffected() == 0 {
		var status string
		scanErr := tx.QueryRow(ctx, `SELECT status FROM drama.change_plans WHERE project_id=$1 AND change_plan_id=$2`, projectID, planID).Scan(&status)
		if errors.Is(scanErr, pgx.ErrNoRows) {
			return ChangePlan{}, ErrNotFound
		}
		if status == "confirmed" || status == "executing" || status == "applied" {
			if err = tx.Commit(ctx); err != nil {
				return ChangePlan{}, err
			}
			return s.GetChangePlan(ctx, projectID, planID)
		}
		return ChangePlan{}, fmt.Errorf("%w: plan is %s", ErrConflict, status)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.change_plan_events(change_plan_event_id,change_plan_id,event_type,actor)
		VALUES($1,$2,'confirmed',$3)`, eventID, planID, actor); err != nil {
		return ChangePlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ChangePlan{}, err
	}
	return s.GetChangePlan(ctx, projectID, planID)
}

func (s *Store) SetChangePlanReviewMetadata(
	ctx context.Context, projectID, planID string, metadata json.RawMessage,
) (ChangePlan, error) {
	var value map[string]any
	if len(metadata) == 0 || json.Unmarshal(metadata, &value) != nil {
		return ChangePlan{}, fmt.Errorf("%w: review metadata must be a JSON object", localedit.ErrInvalidPlan)
	}
	tag, err := s.writer.Exec(ctx, `UPDATE drama.change_plans SET review_metadata=$3::jsonb,updated_at=now()
		WHERE project_id=$1 AND change_plan_id=$2 AND status='validated'`, projectID, planID, metadata)
	if err != nil {
		return ChangePlan{}, err
	}
	if tag.RowsAffected() == 0 {
		return ChangePlan{}, fmt.Errorf("%w: review metadata can only be attached to a validated plan", ErrConflict)
	}
	return s.GetChangePlan(ctx, projectID, planID)
}

func (s *Store) RejectChangePlan(
	ctx context.Context, projectID, planID string, actor *string, reason string,
) (ChangePlan, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ChangePlan{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM drama.change_plans
		WHERE project_id=$1 AND change_plan_id=$2 FOR UPDATE`, projectID, planID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangePlan{}, ErrNotFound
	}
	if err != nil {
		return ChangePlan{}, err
	}
	if status == "cancelled" {
		if err = tx.Commit(ctx); err != nil {
			return ChangePlan{}, err
		}
		return s.GetChangePlan(ctx, projectID, planID)
	}
	if status != "validated" {
		return ChangePlan{}, fmt.Errorf("%w: only an unconfirmed candidate can be rejected", ErrConflict)
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.change_plans SET status='cancelled',updated_at=now()
		WHERE change_plan_id=$1`, planID); err != nil {
		return ChangePlan{}, err
	}
	eventID, err := newPublicID("cpe_")
	if err != nil {
		return ChangePlan{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.change_plan_events(
		change_plan_event_id,change_plan_id,event_type,actor,details)
		VALUES($1,$2,'cancelled',$3,jsonb_build_object('reason',$4::text))`,
		eventID, planID, actor, strings.TrimSpace(reason)); err != nil {
		return ChangePlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ChangePlan{}, err
	}
	return s.GetChangePlan(ctx, projectID, planID)
}

func (s *Store) ExecuteChangePlan(ctx context.Context, projectID, planID string) (result ChangePlan, err error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ChangePlan{}, err
	}
	defer tx.Rollback(ctx)

	var status, entityType, entityID, changeKind, fingerprint, userIntent string
	var targetContentHash *string
	var targetVersion int
	var semantic, seasonPlanConfirmation bool
	var changesJSON, rebuildTasksJSON json.RawMessage
	err = tx.QueryRow(ctx, `SELECT status,target_entity_type,target_entity_id,target_version,
		expected_changes,rebuild_tasks,change_kind,semantic_change,plan_fingerprint,target_content_hash
		,user_intent,(target_entity_type='adaptation_plan' AND review_metadata ? 'preview_fingerprint')
		FROM drama.change_plans WHERE project_id=$1 AND change_plan_id=$2 FOR UPDATE`,
		projectID, planID).Scan(&status, &entityType, &entityID, &targetVersion,
		&changesJSON, &rebuildTasksJSON, &changeKind, &semantic, &fingerprint,
		&targetContentHash, &userIntent, &seasonPlanConfirmation)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangePlan{}, ErrNotFound
	}
	if err != nil {
		return ChangePlan{}, err
	}
	if seasonPlanConfirmation {
		return ChangePlan{}, fmt.Errorf("%w: season change plans must use the atomic season confirmation endpoint", ErrConflict)
	}
	if status == "applied" {
		if err = tx.Commit(ctx); err != nil {
			return ChangePlan{}, err
		}
		return s.GetChangePlan(ctx, projectID, planID)
	}
	if status != "confirmed" {
		return ChangePlan{}, fmt.Errorf("%w: plan must be confirmed", ErrConflict)
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.change_plans SET status='executing' WHERE change_plan_id=$1`, planID); err != nil {
		return ChangePlan{}, err
	}

	before, err := readEntitySnapshot(ctx, tx, entityType, entityID)
	if err != nil {
		return ChangePlan{}, err
	}
	var currentVersionID *string
	var currentVersion int
	err = tx.QueryRow(ctx, `SELECT entity_version_id,version FROM drama.entity_versions
		WHERE entity_type=$1 AND entity_id=$2 AND is_current FOR UPDATE`, entityType, entityID).Scan(&currentVersionID, &currentVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		currentVersion, err = readNativeEntityVersion(ctx, tx, entityType, entityID)
		currentVersionID = nil
	} else if err != nil {
		return ChangePlan{}, err
	}
	if err != nil {
		return ChangePlan{}, err
	}
	if currentVersion != targetVersion {
		return ChangePlan{}, fmt.Errorf("%w: target version is %d, current is %d", ErrConflict, targetVersion, currentVersion)
	}
	beforeHash, hashErr := hashJSON(json.RawMessage(before))
	if hashErr != nil {
		return ChangePlan{}, hashErr
	}
	if targetContentHash != nil && *targetContentHash != beforeHash {
		return ChangePlan{}, fmt.Errorf("%w: target content changed after preview", ErrConflict)
	}

	var changes []localedit.Change
	if err = json.Unmarshal(changesJSON, &changes); err != nil {
		return ChangePlan{}, err
	}
	after, err := materializeVersionedChange(
		ctx, tx, projectID, entityType, entityID, before, changes, fingerprint,
	)
	if err != nil {
		return ChangePlan{}, err
	}
	afterHash, _ := hashJSON(json.RawMessage(after))
	if currentVersionID == nil {
		originalVersionID, idErr := newPublicID("ev_")
		if idErr != nil {
			return ChangePlan{}, idErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO drama.entity_versions(
			entity_version_id,project_id,entity_type,entity_id,version,content,content_hash,
			semantic_hash,source_type,is_current)
			VALUES($1,$2,$3,$4,$5,$6::jsonb,$7,$7,'generated',false)`,
			originalVersionID, projectID, entityType, entityID, currentVersion, before, beforeHash); err != nil {
			return ChangePlan{}, err
		}
		currentVersionID = &originalVersionID
	} else {
		if _, err = tx.Exec(ctx, `UPDATE drama.entity_versions SET is_current=false WHERE entity_version_id=$1`, *currentVersionID); err != nil {
			return ChangePlan{}, err
		}
	}
	newVersionID, err := newPublicID("ev_")
	if err != nil {
		return ChangePlan{}, err
	}
	semanticHash := afterHash
	if !semantic {
		semanticHash = beforeHash
	}
	sourceType := "local_edit"
	eventType := "applied"
	if strings.HasPrefix(userIntent, "撤销到") {
		sourceType, eventType = "rollback", "rolled_back"
	} else if strings.HasPrefix(userIntent, "重新应用") {
		eventType = "reapplied"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.entity_versions(
		entity_version_id,project_id,entity_type,entity_id,version,parent_entity_version_id,
		change_plan_id,content,content_hash,semantic_hash,source_type,
		source_metadata,is_current)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,
		jsonb_build_object('change_kind',$12::text),true)`,
		newVersionID, projectID, entityType, entityID, currentVersion+1, currentVersionID,
		planID, after, afterHash, semanticHash, sourceType, changeKind); err != nil {
		return ChangePlan{}, err
	}
	if entityType == "adaptation_plan" {
		if err = publishVersionedAdaptationPlanArtifact(
			ctx, tx, projectID, planID, entityID, currentVersion+1, afterHash,
		); err != nil {
			return ChangePlan{}, err
		}
	}
	if entityType == "scene" {
		if err = versionAdjacentScenes(
			ctx, tx, projectID, planID, entityID, changes,
		); err != nil {
			return ChangePlan{}, err
		}
	}

	if semantic {
		if _, err = tx.Exec(ctx, `UPDATE drama.artifacts artifact SET validity_status='stale'
			FROM drama.change_plan_impacts impact
			WHERE impact.change_plan_id=$1 AND artifact.artifact_id=impact.artifact_id
			  AND artifact.validity_status<>'stale'`, planID); err != nil {
			return ChangePlan{}, err
		}
	}
	if err = createPendingRebuildTasks(ctx, tx, planID, projectID, entityType, entityID, changes, rebuildTasksJSON, fingerprint); err != nil {
		return ChangePlan{}, err
	}
	eventID, err := newPublicID("cpe_")
	if err != nil {
		return ChangePlan{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.change_plans SET status='applied',applied_at=now() WHERE change_plan_id=$1`, planID); err != nil {
		return ChangePlan{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.change_plan_events(change_plan_event_id,change_plan_id,event_type,details)
		VALUES($1,$2,$3,jsonb_build_object('entity_version_id',$4::text,'version',$5::int))`,
		eventID, planID, eventType, newVersionID, currentVersion+1); err != nil {
		return ChangePlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ChangePlan{}, err
	}
	return s.GetChangePlan(ctx, projectID, planID)
}

func publishVersionedAdaptationPlanArtifact(
	ctx context.Context, tx pgx.Tx, projectID, changePlanID, entityID string,
	version int, contentHash string,
) error {
	var predecessorID string
	err := tx.QueryRow(ctx, `SELECT artifact_id FROM drama.artifacts
		WHERE project_id=$1 AND artifact_type='adaptation_plan'
		  AND native_entity_id=$2 AND is_current FOR UPDATE`, projectID, entityID).
		Scan(&predecessorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	successorID := "artifact_ev_" + contentHash[:24]
	if _, err = tx.Exec(ctx, `UPDATE drama.artifacts SET is_current=false,
		validity_status='superseded',updated_at=now() WHERE artifact_id=$1`, predecessorID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.artifacts(artifact_id,artifact_type,project_id,
		native_entity_id,revision_number,content_hash,validity_status,is_current,idempotency_key,metadata)
		VALUES($1,'adaptation_plan',$2,$3,$4,$5,'valid',true,$6,
		  jsonb_build_object('change_plan_id',$7::text,'predecessor_artifact_id',$8::text))`,
		successorID, projectID, entityID, version, contentHash,
		"change-plan:artifact:"+changePlanID, changePlanID, predecessorID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.artifact_current_bindings(
		artifact_current_binding_id,project_id,target_type,target_id,component_scope,current_artifact_id)
		VALUES('acb_ev_'||substr(encode(drama.digest(convert_to($1||':'||$2,'UTF8'),'sha256'),'hex'),1,24),
		  $1,'adaptation_plan',$2,'whole',$3)
		ON CONFLICT(project_id,target_type,target_id,component_scope) DO UPDATE SET
		  current_artifact_id=EXCLUDED.current_artifact_id,selected_at=now()`,
		projectID, entityID, successorID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.artifact_dependencies(artifact_dependency_id,
		upstream_artifact_id,downstream_artifact_id,dependency_type,dependency_selector,
		observed_upstream_hash,invalidates_on,idempotency_key)
	SELECT 'ad_ev_'||substr(encode(drama.digest(convert_to($1||':'||dependency.downstream_artifact_id,'UTF8'),'sha256'),'hex'),1,24),
	  $1,dependency.downstream_artifact_id,dependency.dependency_type,
	  dependency.dependency_selector||jsonb_build_object('source_change_plan_id',$3::text),
	  $4,dependency.invalidates_on,'change-plan:dependency:'||$1||':'||dependency.downstream_artifact_id
	FROM drama.artifact_dependencies dependency WHERE dependency.upstream_artifact_id=$2
	ON CONFLICT(idempotency_key) DO NOTHING`, successorID, predecessorID, changePlanID, contentHash)
	return err
}

func readEntitySnapshot(ctx context.Context, tx pgx.Tx, entityType, entityID string) (json.RawMessage, error) {
	var versioned json.RawMessage
	err := tx.QueryRow(ctx, `SELECT content FROM drama.entity_versions
		WHERE entity_type=$1 AND entity_id=$2 AND is_current FOR UPDATE`,
		entityType, entityID).Scan(&versioned)
	if err == nil {
		if entityType == "episode_content" {
			var projectID string
			if readErr := tx.QueryRow(ctx, `SELECT project_id FROM drama.episode_outlines
				WHERE episode_id=$1`, entityID).Scan(&projectID); readErr != nil {
				return nil, readErr
			}
			return overlayEpisodeSnapshot(ctx, tx, projectID, entityID, versioned)
		}
		return versioned, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if inherited, ok, inheritedErr := readInheritedEpisodeSnapshot(
		ctx, tx, entityType, entityID,
	); inheritedErr != nil {
		return nil, inheritedErr
	} else if ok {
		return inherited, nil
	}
	if entityType == "episode_content" {
		native, readErr := readNativeEpisodeContentSnapshot(ctx, tx, entityID)
		if readErr != nil {
			return nil, readErr
		}
		var projectID string
		if readErr = tx.QueryRow(ctx, `SELECT project_id FROM drama.episode_outlines
			WHERE episode_id=$1`, entityID).Scan(&projectID); readErr != nil {
			return nil, readErr
		}
		return overlayEpisodeSnapshot(ctx, tx, projectID, entityID, native)
	}
	queries := map[string]string{
		"dialogue": `SELECT to_jsonb(d)-'id'-'created_at'-'updated_at' FROM drama.dialogues d WHERE dialogue_id=$1 FOR UPDATE`,
		"scene":    `SELECT to_jsonb(s)-'id'-'created_at'-'updated_at' FROM drama.script_scenes s WHERE scene_id=$1 FOR UPDATE`,
		"shot":     `SELECT to_jsonb(s)-'id'-'created_at'-'updated_at' FROM drama.storyboard_shots s WHERE shot_id=$1 FOR UPDATE`,
		"outline":  `SELECT to_jsonb(o)-'id'-'created_at'-'updated_at' FROM drama.episode_outlines o WHERE episode_id=$1 FOR UPDATE`,
		"script":   `SELECT to_jsonb(s)-'id'-'created_at'-'updated_at' FROM drama.episode_scripts s WHERE script_id=$1 FOR UPDATE`,
		"adaptation_spec": `SELECT (to_jsonb(s)-'id'-'created_at'-'updated_at')||jsonb_build_object(
			  'chapter_ids',COALESCE((SELECT jsonb_agg(scope.chapter_id ORDER BY scope.ordinal_from NULLS LAST,scope.chapter_id)
			    FROM drama.adaptation_scope_chapters scope WHERE scope.adaptation_spec_version_id=s.adaptation_spec_version_id
			      AND scope.include_mode='include'),'[]'::jsonb),
			  'story_arc_revision_ids',COALESCE((SELECT jsonb_agg(scope.story_arc_revision_id ORDER BY scope.story_arc_revision_id)
			    FROM drama.adaptation_scope_arcs scope WHERE scope.adaptation_spec_version_id=s.adaptation_spec_version_id
			      AND scope.include_mode='include'),'[]'::jsonb),
			  'rules',COALESCE((SELECT jsonb_agg(to_jsonb(rule)-'id'-'created_at'-'updated_at' ORDER BY rule.priority DESC,rule.adaptation_rule_id)
			    FROM drama.adaptation_rules rule WHERE rule.adaptation_spec_version_id=s.adaptation_spec_version_id),'[]'::jsonb)
			) FROM drama.adaptation_spec_versions s WHERE adaptation_spec_version_id=$1 FOR UPDATE`,
		"adaptation_plan": seasonPlanDraftSnapshotSQL,
		"pacing": `SELECT (to_jsonb(p)-'id'-'created_at')||jsonb_build_object(
			  'story_arcs',COALESCE((SELECT jsonb_agg(to_jsonb(item)-'id'-'created_at' ORDER BY item.ordinal)
			    FROM drama.pacing_story_arcs item WHERE item.pacing_plan_id=p.pacing_plan_id),'[]'::jsonb),
			  'episodes',COALESCE((SELECT jsonb_agg(to_jsonb(item)-'id'-'created_at' ORDER BY item.episode_number)
			    FROM drama.pacing_episodes item WHERE item.pacing_plan_id=p.pacing_plan_id),'[]'::jsonb),
			  'beats',COALESCE((SELECT jsonb_agg(to_jsonb(item)-'id'-'created_at' ORDER BY item.episode_number,item.beat_ordinal)
			    FROM drama.pacing_beats item WHERE item.pacing_plan_id=p.pacing_plan_id),'[]'::jsonb)
			) FROM drama.pacing_plan_versions p WHERE pacing_plan_id=$1 FOR UPDATE`,
		"performance_bible": `SELECT to_jsonb(p)-'id'-'created_at'-'updated_at'
			FROM drama.character_performance_bibles p WHERE performance_bible_id=$1 FOR UPDATE`,
		"continuity": `SELECT to_jsonb(c)-'id'-'created_at'-'updated_at'
			FROM drama.continuity_ledger_entries c WHERE continuity_entry_id=$1 FOR UPDATE`,
		"timeline": `SELECT to_jsonb(t)-'id'-'created_at'-'updated_at' FROM drama.edit_timelines t
			WHERE t.episode_id=$1 ORDER BY t.version DESC LIMIT 1 FOR UPDATE`,
		"timeline_item": `SELECT to_jsonb(i)-'id'-'created_at'-'updated_at'
			FROM drama.edit_timeline_items i WHERE timeline_item_id=$1 FOR UPDATE`,
		"shot_video": `SELECT to_jsonb(v)-'id'-'created_at'-'updated_at'
			FROM drama.shot_videos v
			WHERE v.shot_id=(SELECT source.shot_id FROM drama.shot_videos source WHERE source.shot_video_id=$1)
			  AND v.is_current
			ORDER BY v.generation_version DESC LIMIT 1 FOR UPDATE`,
	}
	query, ok := queries[entityType]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported entity type %s", localedit.ErrInvalidPlan, entityType)
	}
	var result json.RawMessage
	err = tx.QueryRow(ctx, query, entityID).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return result, err
}

func readCurrentEntityVersion(ctx context.Context, tx pgx.Tx, entityType, entityID string) (int, error) {
	var version int
	err := tx.QueryRow(ctx, `SELECT version FROM drama.entity_versions
		WHERE entity_type=$1 AND entity_id=$2 AND is_current FOR UPDATE`,
		entityType, entityID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return readNativeEntityVersion(ctx, tx, entityType, entityID)
	}
	return version, err
}

func readNativeEntityVersion(ctx context.Context, tx pgx.Tx, entityType, entityID string) (int, error) {
	if inheritedVersion, ok, err := readInheritedEpisodeVersion(
		ctx, tx, entityType, entityID,
	); err != nil {
		return 0, err
	} else if ok {
		return inheritedVersion, nil
	}
	var query string
	switch entityType {
	case "dialogue":
		query = `SELECT 1 FROM drama.dialogues WHERE dialogue_id=$1`
	case "scene":
		query = `SELECT 1 FROM drama.script_scenes WHERE scene_id=$1`
	case "shot":
		query = `SELECT generation_version FROM drama.storyboard_shots WHERE shot_id=$1`
	case "shot_video":
		query = `SELECT current.generation_version FROM drama.shot_videos source
			JOIN drama.shot_videos current ON current.shot_id=source.shot_id AND current.is_current
			WHERE source.shot_video_id=$1 ORDER BY current.generation_version DESC LIMIT 1`
	case "outline":
		query = `SELECT version FROM drama.episode_outlines WHERE episode_id=$1`
	case "script":
		query = `SELECT version FROM drama.episode_scripts WHERE script_id=$1`
	case "adaptation_spec":
		query = `SELECT version_number FROM drama.adaptation_spec_versions WHERE adaptation_spec_version_id=$1`
	case "adaptation_plan":
		query = `SELECT version_number FROM drama.adaptation_plans WHERE adaptation_plan_id=$1`
	case "pacing":
		query = `SELECT version_number FROM drama.pacing_plan_versions WHERE pacing_plan_id=$1`
	case "performance_bible":
		query = `SELECT version FROM drama.character_performance_bibles WHERE performance_bible_id=$1`
	case "continuity":
		query = `SELECT 1 FROM drama.continuity_ledger_entries WHERE continuity_entry_id=$1`
	case "episode_content":
		query = `SELECT GREATEST(outline.version,COALESCE((
			SELECT max(script.version) FROM drama.episode_scripts script
			WHERE script.episode_id=outline.episode_id),outline.version))
			FROM drama.episode_outlines outline WHERE outline.episode_id=$1`
	case "timeline":
		query = `SELECT version FROM drama.edit_timelines
			WHERE episode_id=$1 ORDER BY version DESC LIMIT 1`
	case "timeline_item":
		query = `SELECT timeline.version FROM drama.edit_timeline_items item
			JOIN drama.edit_timelines timeline USING(timeline_id) WHERE item.timeline_item_id=$1`
	default:
		return 0, fmt.Errorf("%w: unsupported entity type %s", localedit.ErrInvalidPlan, entityType)
	}
	var version int
	err := tx.QueryRow(ctx, query, entityID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	return version, err
}

func createPendingRebuildTasks(
	ctx context.Context, tx pgx.Tx, planID, projectID, entityType, entityID string,
	changes []localedit.Change, rebuildTasksJSON json.RawMessage, fingerprint string,
) error {
	var actions []string
	if err := json.Unmarshal(rebuildTasksJSON, &actions); err != nil {
		return err
	}
	var startMS, endMS *int64
	for _, change := range changes {
		if change.StartMS != nil {
			startMS = change.StartMS
		}
		if change.EndMS != nil {
			endMS = change.EndMS
		}
	}
	if entityType == "dialogue" && (startMS == nil || endMS == nil) {
		var derivedStart, derivedEnd int64
		rangeErr := tx.QueryRow(ctx, `SELECT start_ms,end_ms FROM drama.subtitle_cues
			WHERE dialogue_id=$1 AND is_current ORDER BY sequence_number LIMIT 1`,
			entityID).Scan(&derivedStart, &derivedEnd)
		if rangeErr == nil {
			startMS, endMS = &derivedStart, &derivedEnd
		} else if !errors.Is(rangeErr, pgx.ErrNoRows) {
			return rangeErr
		}
	}
	taskNumber := 0
	for _, action := range actions {
		targets, err := resolvePendingRebuildTargets(
			ctx, tx, projectID, action, entityType, entityID, changes, startMS, endMS,
		)
		if err != nil {
			return err
		}
		for _, target := range targets {
			taskNumber++
			taskID := "irt_" + fingerprint[:16] + "_" + strconv.Itoa(taskNumber)
			_, err = tx.Exec(ctx, `INSERT INTO drama.incremental_rebuild_tasks(
				rebuild_task_id,change_plan_id,project_id,action,target_entity_type,target_entity_id,
				range_start_ms,range_end_ms,status,provider,input,output)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,'pending','workflow',
				jsonb_build_object(
				  'plan_fingerprint',$9::text,'requires_real_execution',true,
				  'source_entity_type',$10::text,'source_entity_id',$11::text,
				  'source_entity_version_id',(SELECT entity_version_id
				    FROM drama.entity_versions
				    WHERE entity_type=$10::text AND entity_id=$11::text AND is_current),
				  'source_version',(SELECT version
				    FROM drama.entity_versions
				    WHERE entity_type=$10::text AND entity_id=$11::text AND is_current)
				),
				'{}'::jsonb)`,
				taskID, planID, projectID, action, target.EntityType, target.EntityID,
				target.StartMS, target.EndMS, fingerprint, entityType, entityID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) GetChangePlan(ctx context.Context, projectID, planID string) (ChangePlan, error) {
	var item ChangePlan
	var mustPreserve, allowedFieldsJSON, changesJSON, upstreamJSON, downstreamJSON json.RawMessage
	var rebuildJSON, rebuildTasksJSON, risksJSON, rulesJSON, locksJSON json.RawMessage
	var target localedit.Target
	var changeKind string
	var semantic bool
	err := s.pool.QueryRow(ctx, `SELECT change_plan_id,project_id,status,user_intent,
		target_entity_type,target_entity_id,target_version,must_preserve,allowed_fields,
		expected_changes,affected_upstream,affected_downstream,rebuild_decision,rebuild_tasks,
		risks,validation_rules,rollback_version,change_kind,semantic_change,locks,plan_fingerprint,
		requested_by,confirmed_by,confirmed_at,applied_at,error_code,error_message,review_metadata,created_at,updated_at
		FROM drama.change_plans WHERE project_id=$1 AND change_plan_id=$2`,
		projectID, planID).Scan(&item.ChangePlanID, &item.ProjectID, &item.Status, &item.Plan.UserIntent,
		&target.EntityType, &target.EntityID, &target.Version, &mustPreserve, &allowedFieldsJSON,
		&changesJSON, &upstreamJSON, &downstreamJSON, &rebuildJSON, &rebuildTasksJSON,
		&risksJSON, &rulesJSON, &item.Plan.RollbackVersion, &changeKind, &semantic, &locksJSON,
		&item.Fingerprint, &item.RequestedBy, &item.ConfirmedBy, &item.ConfirmedAt, &item.AppliedAt,
		&item.ErrorCode, &item.ErrorMessage, &item.ReviewMetadata, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangePlan{}, ErrNotFound
	}
	if err != nil {
		return ChangePlan{}, err
	}
	item.Plan.SchemaVersion = localedit.SchemaVersion
	item.Plan.Target, item.Plan.ChangeKind, item.Plan.SemanticChange = target, changeKind, semantic
	_ = json.Unmarshal(mustPreserve, &item.Plan.MustPreserve)
	_ = json.Unmarshal(allowedFieldsJSON, &item.Plan.AllowedFields)
	_ = json.Unmarshal(changesJSON, &item.Plan.ExpectedChanges)
	_ = json.Unmarshal(upstreamJSON, &item.Plan.Impact.Upstream)
	_ = json.Unmarshal(downstreamJSON, &item.Plan.Impact.Downstream)
	_ = json.Unmarshal(rebuildJSON, &item.Plan.Rebuild)
	_ = json.Unmarshal(rebuildTasksJSON, &item.Plan.Impact.RebuildTasks)
	_ = json.Unmarshal(risksJSON, &item.Plan.Risks)
	_ = json.Unmarshal(rulesJSON, &item.Plan.ValidationRules)
	_ = json.Unmarshal(locksJSON, &item.Plan.Locks)
	item.Impacts, err = s.listChangePlanImpacts(ctx, planID)
	if err != nil {
		return ChangePlan{}, err
	}
	item.RebuildTasks, err = s.listRebuildTasks(ctx, planID)
	return item, err
}

func (s *Store) ListChangePlans(ctx context.Context, projectID string) ([]ChangePlan, error) {
	rows, err := s.pool.Query(ctx, `SELECT change_plan_id FROM drama.change_plans
		WHERE project_id=$1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	result := make([]ChangePlan, 0, len(ids))
	for _, id := range ids {
		item, readErr := s.GetChangePlan(ctx, projectID, id)
		if readErr != nil {
			return nil, readErr
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListEntityVersions(ctx context.Context, projectID, entityType, entityID string) ([]EntityVersion, error) {
	rows, err := s.pool.Query(ctx, `SELECT entity_version_id,entity_type,entity_id,version,content,
		content_hash,semantic_hash,source_type,is_current,created_at
		FROM drama.entity_versions WHERE project_id=$1 AND entity_type=$2 AND entity_id=$3
		ORDER BY version DESC`, projectID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]EntityVersion, 0)
	for rows.Next() {
		var item EntityVersion
		if err = rows.Scan(&item.EntityVersionID, &item.EntityType, &item.EntityID, &item.Version,
			&item.Content, &item.ContentHash, &item.SemanticHash, &item.SourceType,
			&item.IsCurrent, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CreateVersionRestorePlan(
	ctx context.Context, projectID, entityVersionID, mode string, paths []string, requestedBy *string,
) (ChangePlan, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "rollback" && mode != "reapply" {
		return ChangePlan{}, fmt.Errorf("%w: mode must be rollback or reapply", localedit.ErrInvalidPlan)
	}
	var entityType, entityID string
	var sourceVersion, currentVersion int
	var sourceContent, currentContent json.RawMessage
	err := s.pool.QueryRow(ctx, `SELECT source.entity_type,source.entity_id,source.version,source.content,
		current.version,current.content
		FROM drama.entity_versions source
		JOIN drama.entity_versions current ON current.project_id=source.project_id
		  AND current.entity_type=source.entity_type AND current.entity_id=source.entity_id
		  AND current.is_current
		WHERE source.project_id=$1 AND source.entity_version_id=$2`,
		projectID, entityVersionID).Scan(&entityType, &entityID, &sourceVersion, &sourceContent,
		&currentVersion, &currentContent)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangePlan{}, ErrNotFound
	}
	if err != nil {
		return ChangePlan{}, err
	}
	if sourceVersion == currentVersion {
		return ChangePlan{}, fmt.Errorf("%w: selected version is already current", ErrConflict)
	}
	changes, err := restoreChanges(entityType, sourceContent, currentContent)
	if err != nil {
		return ChangePlan{}, err
	}
	if len(changes) == 0 {
		return ChangePlan{}, fmt.Errorf("%w: selected version has no supported field differences", ErrConflict)
	}
	if len(paths) > 0 {
		changes = selectRestoreChanges(entityType, changes, paths, sourceContent, currentContent)
		if len(changes) == 0 {
			return ChangePlan{}, fmt.Errorf("%w: selected paths have no restorable differences", ErrConflict)
		}
	}
	verb := "撤销到"
	if mode == "reapply" {
		verb = "重新应用"
	}
	plan, err := localedit.Build(localedit.Request{
		Instruction: fmt.Sprintf("%s历史版本 v%d", verb, sourceVersion),
		Target: localedit.Target{
			EntityType: entityType, EntityID: entityID, Version: currentVersion,
		},
		Changes: changes,
		MustPreserve: []string{
			"未包含在历史版本 diff 中的字段",
		},
	})
	if err != nil {
		return ChangePlan{}, err
	}
	return s.CreateChangePlan(ctx, projectID, plan, requestedBy)
}

func selectRestoreChanges(
	entityType string, changes []localedit.Change, paths []string,
	sourceJSON, currentJSON json.RawMessage,
) []localedit.Change {
	selected := map[string]bool{}
	selectedSceneIDs := map[string]bool{}
	for _, path := range paths {
		if path = strings.TrimSpace(path); path == "" {
			continue
		}
		selected[path] = true
		parts := strings.Split(path, ".")
		if entityType == "episode_content" && len(parts) == 2 && parts[0] == "scene" {
			selectedSceneIDs[parts[1]] = true
		}
	}
	dialogueScenes := map[string]map[string]bool{}
	if len(selectedSceneIDs) > 0 {
		for _, raw := range []json.RawMessage{sourceJSON, currentJSON} {
			var content map[string]any
			if json.Unmarshal(raw, &content) != nil {
				continue
			}
			script, _ := content["script"].(map[string]any)
			for _, sceneItem := range anySlice(script["scenes"]) {
				scene, _ := sceneItem.(map[string]any)
				sceneID := fmt.Sprint(scene["scene_id"])
				for _, dialogueItem := range anySlice(scene["dialogues"]) {
					dialogue, _ := dialogueItem.(map[string]any)
					dialogueID := fmt.Sprint(dialogue["dialogue_id"])
					if dialogueScenes[dialogueID] == nil {
						dialogueScenes[dialogueID] = map[string]bool{}
					}
					dialogueScenes[dialogueID][sceneID] = true
				}
			}
		}
	}
	filtered := make([]localedit.Change, 0, len(changes))
	for _, change := range changes {
		include := selected[change.Field]
		parts := strings.Split(change.Field, ".")
		if !include && len(parts) >= 2 && parts[0] == "dialogue" {
			for sceneID := range dialogueScenes[parts[1]] {
				if selectedSceneIDs[sceneID] {
					include = true
					break
				}
			}
		}
		if include {
			filtered = append(filtered, change)
		}
	}
	return filtered
}

func restoreChanges(entityType string, sourceJSON, currentJSON json.RawMessage) ([]localedit.Change, error) {
	var source, current map[string]any
	if err := json.Unmarshal(sourceJSON, &source); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(currentJSON, &current); err != nil {
		return nil, err
	}
	if entityType == "episode_content" {
		return restoreEpisodeContentChanges(source, current)
	}
	if entityType == "timeline" {
		sourceTimelineID := strings.TrimSpace(fmt.Sprint(source["timeline_id"]))
		if sourceTimelineID == "" {
			return nil, fmt.Errorf("%w: historical timeline has no timeline_id", localedit.ErrInvalidPlan)
		}
		return []localedit.Change{{
			Operation: "replace", Field: "restore_source_timeline_id", Value: sourceTimelineID,
		}}, nil
	}
	fields := map[string][]string{
		"outline": {
			"title", "logline", "opening_hook", "story_goal", "main_conflict",
			"climax", "ending_hook", "estimated_duration_seconds",
		},
		"script": {"title", "opening_hook", "climax", "ending_hook"},
		"dialogue": {
			"dialogue_type", "speaker_name", "text", "emotion", "performance_instruction",
			"estimated_duration_ms", "production_mode",
		},
		"scene": {
			"scene_purpose", "actions", "emotional_change", "estimated_duration_seconds",
			"location_name", "time_of_day", "interior_exterior", "scene_number", "character_ids",
		},
		"shot": {
			"action_description", "facial_expression", "composition", "shot_size",
			"camera_angle", "camera_motion", "duration_seconds", "shot_order",
		},
		"adaptation_spec":   {"platform", "audience_profile", "target_episode_count", "episode_duration_seconds", "scope_mode", "chapter_ids", "story_arc_revision_ids", "rules"},
		"adaptation_plan":   {"quality_report"},
		"pacing":            {"total_duration_seconds", "story_arcs", "episodes", "beats"},
		"performance_bible": {"speech", "acting", "relational_voices", "appearance", "locked_fields", "allowed_fields", "change_reasons", "source_refs"},
		"continuity":        {"input_state", "output_state", "validation_status", "diagnostics"},
	}
	if fields[entityType] == nil {
		return nil, fmt.Errorf("%w: version restore is not supported for %s", localedit.ErrInvalidPlan, entityType)
	}
	changes := make([]localedit.Change, 0)
	for _, field := range fields[entityType] {
		sourceValue, sourceExists := source[field]
		if !sourceExists {
			continue
		}
		sourceRaw, _ := json.Marshal(sourceValue)
		currentRaw, _ := json.Marshal(current[field])
		if string(sourceRaw) != string(currentRaw) {
			changes = append(changes, localedit.Change{
				Operation: "replace", Field: field, Value: sourceValue,
			})
		}
	}
	return changes, nil
}

func (s *Store) AddChangeComment(ctx context.Context, projectID string, input CreateChangeCommentInput) (ChangeComment, error) {
	if strings.TrimSpace(input.Body) == "" || strings.TrimSpace(input.EntityID) == "" {
		return ChangeComment{}, fmt.Errorf("%w: comment body and entity are required", localedit.ErrInvalidPlan)
	}
	if input.TimecodeEndMS != nil && (input.TimecodeStartMS == nil || *input.TimecodeEndMS <= *input.TimecodeStartMS) {
		return ChangeComment{}, fmt.Errorf("%w: invalid timecode range", localedit.ErrInvalidPlan)
	}
	id, err := newPublicID("cc_")
	if err != nil {
		return ChangeComment{}, err
	}
	var item ChangeComment
	err = s.writer.QueryRow(ctx, `INSERT INTO drama.change_comments(
		comment_id,project_id,entity_type,entity_id,entity_version,timecode_start_ms,
		timecode_end_ms,body,author) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING comment_id,project_id,entity_type,entity_id,entity_version,timecode_start_ms,
		timecode_end_ms,body,author,resolved,created_at`,
		id, projectID, input.EntityType, input.EntityID, input.EntityVersion,
		input.TimecodeStartMS, input.TimecodeEndMS, strings.TrimSpace(input.Body), input.Author).Scan(
		&item.CommentID, &item.ProjectID, &item.EntityType, &item.EntityID, &item.EntityVersion,
		&item.TimecodeStartMS, &item.TimecodeEndMS, &item.Body, &item.Author, &item.Resolved, &item.CreatedAt)
	return item, err
}

func (s *Store) ListChangeComments(ctx context.Context, projectID, entityType, entityID string) ([]ChangeComment, error) {
	rows, err := s.pool.Query(ctx, `SELECT comment_id,project_id,entity_type,entity_id,entity_version,
		timecode_start_ms,timecode_end_ms,body,author,resolved,created_at
		FROM drama.change_comments WHERE project_id=$1 AND ($2='' OR entity_type=$2)
		AND ($3='' OR entity_id=$3) ORDER BY created_at`, projectID, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ChangeComment, 0)
	for rows.Next() {
		var item ChangeComment
		if err = rows.Scan(&item.CommentID, &item.ProjectID, &item.EntityType, &item.EntityID,
			&item.EntityVersion, &item.TimecodeStartMS, &item.TimecodeEndMS, &item.Body,
			&item.Author, &item.Resolved, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) listChangePlanImpacts(ctx context.Context, planID string) ([]ChangePlanImpact, error) {
	rows, err := s.pool.Query(ctx, `SELECT artifact_id,artifact_type,native_entity_id,
		propagation_depth,before_status,after_status,dependency_path,rebuild_action
		FROM drama.change_plan_impacts WHERE change_plan_id=$1 ORDER BY propagation_depth,artifact_id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ChangePlanImpact, 0)
	for rows.Next() {
		var item ChangePlanImpact
		if err = rows.Scan(&item.ArtifactID, &item.ArtifactType, &item.NativeEntityID,
			&item.PropagationDepth, &item.BeforeStatus, &item.AfterStatus,
			&item.DependencyPath, &item.RebuildAction); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) listRebuildTasks(ctx context.Context, planID string) ([]IncrementalRebuild, error) {
	rows, err := s.pool.Query(ctx, `SELECT rebuild_task_id,action,target_entity_type,target_entity_id,
		artifact_id,range_start_ms,range_end_ms,status,provider,input,output,error_code,error_message,
		created_at,completed_at
		FROM drama.incremental_rebuild_tasks WHERE change_plan_id=$1 ORDER BY created_at,rebuild_task_id`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]IncrementalRebuild, 0)
	for rows.Next() {
		var item IncrementalRebuild
		if err = rows.Scan(&item.RebuildTaskID, &item.Action, &item.TargetEntityType,
			&item.TargetEntityID, &item.ArtifactID, &item.RangeStartMS, &item.RangeEndMS,
			&item.Status, &item.Provider, &item.Input, &item.Output, &item.ErrorCode,
			&item.ErrorMessage, &item.CreatedAt,
			&item.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) UpdateRebuildTaskStatus(
	ctx context.Context, projectID, planID, taskID string, input RebuildTaskStatusInput,
) (IncrementalRebuild, error) {
	status := strings.ToLower(strings.TrimSpace(input.Status))
	if status != "cancelled" {
		return IncrementalRebuild{}, fmt.Errorf("%w: rebuild execution state is worker-owned; this endpoint only accepts cancelled", localedit.ErrInvalidPlan)
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return IncrementalRebuild{}, err
	}
	defer tx.Rollback(ctx)
	var task IncrementalRebuild
	err = tx.QueryRow(ctx, `UPDATE drama.incremental_rebuild_tasks task SET
		status='cancelled',error_code='REBUILD_CANCELLED_BY_USER',error_message=$4,
		completed_at=now(),claim_token=NULL,lease_owner=NULL,lease_expires_at=NULL,heartbeat_at=NULL,updated_at=now()
		FROM drama.change_plans plan
		WHERE task.change_plan_id=plan.change_plan_id AND plan.project_id=$1
		  AND task.change_plan_id=$2 AND task.rebuild_task_id=$3
		  AND task.status IN('pending','claimed','running','retry_wait')
		RETURNING task.rebuild_task_id,task.action,task.target_entity_type,task.target_entity_id,
		  task.artifact_id,task.range_start_ms,task.range_end_ms,task.status,task.provider,
		  task.input,task.output,task.error_code,task.error_message,task.created_at,task.completed_at`,
		projectID, planID, taskID, input.ErrorMessage).Scan(
		&task.RebuildTaskID, &task.Action, &task.TargetEntityType, &task.TargetEntityID,
		&task.ArtifactID, &task.RangeStartMS, &task.RangeEndMS, &task.Status, &task.Provider,
		&task.Input, &task.Output, &task.ErrorCode, &task.ErrorMessage, &task.CreatedAt,
		&task.CompletedAt,
	)
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

func intValue(value, delta any) int {
	if value == nil {
		value = delta
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	default:
		n, _ := strconv.Atoi(fmt.Sprint(value))
		return n
	}
}

func floatValue(value, delta any) float64 {
	if value == nil {
		value = delta
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	case json.Number:
		n, _ := typed.Float64()
		return n
	default:
		n, _ := strconv.ParseFloat(fmt.Sprint(value), 64)
		return n
	}
}
