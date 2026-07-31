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
	ChangePlanID string               `json:"change_plan_id"`
	ProjectID    string               `json:"project_id"`
	Status       string               `json:"status"`
	Plan         localedit.Plan       `json:"plan"`
	Fingerprint  string               `json:"fingerprint"`
	Impacts      []ChangePlanImpact   `json:"impacts"`
	RebuildTasks []IncrementalRebuild `json:"rebuild_tasks"`
	RequestedBy  *string              `json:"requested_by,omitempty"`
	ConfirmedBy  *string              `json:"confirmed_by,omitempty"`
	ConfirmedAt  *time.Time           `json:"confirmed_at,omitempty"`
	AppliedAt    *time.Time           `json:"applied_at,omitempty"`
	ErrorCode    *string              `json:"error_code,omitempty"`
	ErrorMessage *string              `json:"error_message,omitempty"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
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
	CreatedAt        time.Time       `json:"created_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
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
	if plan.Target.EntityType != "media" {
		targetContent, readErr := readEntitySnapshot(ctx, tx, plan.Target.EntityType, plan.Target.EntityID)
		if readErr != nil {
			return ChangePlan{}, readErr
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
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.change_plans(
		change_plan_id,project_id,status,user_intent,natural_language_instruction,
		target_entity_type,target_entity_id,target_version,must_preserve,allowed_fields,
		expected_changes,affected_upstream,affected_downstream,rebuild_decision,rebuild_tasks,
		risks,validation_rules,rollback_version,change_kind,semantic_change,locks,
		plan_fingerprint,requested_by)
		VALUES($1,$2,'validated',$3,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9::jsonb,
		$10::jsonb,$11::jsonb,$12::jsonb,$13::jsonb,$14::jsonb,$15::jsonb,$16,$17,$18,
		$19::jsonb,$20,$21)`,
		changePlanID, projectID, plan.UserIntent, plan.Target.EntityType, plan.Target.EntityID,
		plan.Target.Version, mustPreserve, allowedFieldsJSON, changesJSON, upstreamJSON,
		downstreamJSON, rebuildJSON, rebuildTasksJSON, risksJSON, rulesJSON,
		plan.RollbackVersion, plan.ChangeKind, plan.SemanticChange, locksJSON, fingerprint, requestedBy)
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
		if err = insertExactChangePlanImpacts(ctx, tx, changePlanID, projectID, plan.Target.EntityID, plan.ChangeKind); err != nil {
			return ChangePlan{}, err
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

func (s *Store) ExecuteChangePlan(ctx context.Context, projectID, planID string) (result ChangePlan, err error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ChangePlan{}, err
	}
	defer tx.Rollback(ctx)

	var status, entityType, entityID, changeKind, fingerprint string
	var targetVersion int
	var semantic bool
	var changesJSON, rebuildTasksJSON json.RawMessage
	err = tx.QueryRow(ctx, `SELECT status,target_entity_type,target_entity_id,target_version,
		expected_changes,rebuild_tasks,change_kind,semantic_change,plan_fingerprint
		FROM drama.change_plans WHERE project_id=$1 AND change_plan_id=$2 FOR UPDATE`,
		projectID, planID).Scan(&status, &entityType, &entityID, &targetVersion,
		&changesJSON, &rebuildTasksJSON, &changeKind, &semantic, &fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangePlan{}, ErrNotFound
	}
	if err != nil {
		return ChangePlan{}, err
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

	var changes []localedit.Change
	if err = json.Unmarshal(changesJSON, &changes); err != nil {
		return ChangePlan{}, err
	}
	if err = applyEntityChanges(ctx, tx, entityType, entityID, changes, fingerprint); err != nil {
		return ChangePlan{}, err
	}
	after, err := readEntitySnapshot(ctx, tx, entityType, entityID)
	if err != nil {
		return ChangePlan{}, err
	}
	beforeHash, _ := hashJSON(json.RawMessage(before))
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
	if entityType == "shot_video" {
		sourceType = "deterministic_mock"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.entity_versions(
		entity_version_id,project_id,entity_type,entity_id,version,parent_entity_version_id,
		change_plan_id,content,content_hash,semantic_hash,source_type,
		source_metadata,is_current)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,
		jsonb_build_object('change_kind',$12::text,'mock',($11::text='deterministic_mock')),true)`,
		newVersionID, projectID, entityType, entityID, currentVersion+1, currentVersionID,
		planID, after, afterHash, semanticHash, sourceType, changeKind); err != nil {
		return ChangePlan{}, err
	}

	if semantic {
		if _, err = tx.Exec(ctx, `UPDATE drama.artifacts artifact SET validity_status='stale'
			FROM drama.change_plan_impacts impact
			WHERE impact.change_plan_id=$1 AND artifact.artifact_id=impact.artifact_id
			  AND artifact.validity_status<>'stale'`, planID); err != nil {
			return ChangePlan{}, err
		}
	}
	if err = createDeterministicRebuildTasks(ctx, tx, planID, projectID, entityType, entityID, changes, rebuildTasksJSON, fingerprint); err != nil {
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
		VALUES($1,$2,'applied',jsonb_build_object('entity_version_id',$3::text,'version',$4::int))`,
		eventID, planID, newVersionID, currentVersion+1); err != nil {
		return ChangePlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ChangePlan{}, err
	}
	return s.GetChangePlan(ctx, projectID, planID)
}

func readEntitySnapshot(ctx context.Context, tx pgx.Tx, entityType, entityID string) (json.RawMessage, error) {
	queries := map[string]string{
		"dialogue": `SELECT to_jsonb(d)-'id'-'created_at'-'updated_at' FROM drama.dialogues d WHERE dialogue_id=$1 FOR UPDATE`,
		"scene":    `SELECT to_jsonb(s)-'id'-'created_at'-'updated_at' FROM drama.script_scenes s WHERE scene_id=$1 FOR UPDATE`,
		"shot":     `SELECT to_jsonb(s)-'id'-'created_at'-'updated_at' FROM drama.storyboard_shots s WHERE shot_id=$1 FOR UPDATE`,
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
	err := tx.QueryRow(ctx, query, entityID).Scan(&result)
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
	var query string
	switch entityType {
	case "dialogue":
		query = `SELECT 1 FROM drama.dialogues WHERE dialogue_id=$1`
	case "scene":
		query = `SELECT 1 FROM drama.script_scenes WHERE scene_id=$1`
	case "shot":
		query = `SELECT generation_version FROM drama.storyboard_shots WHERE shot_id=$1`
	case "shot_video":
		query = `SELECT generation_version FROM drama.shot_videos WHERE shot_video_id=$1`
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

func applyEntityChanges(ctx context.Context, tx pgx.Tx, entityType, entityID string, changes []localedit.Change, fingerprint string) error {
	if entityType == "shot_video" {
		return cloneDeterministicShotVideo(ctx, tx, entityID, changes, fingerprint)
	}
	for _, change := range changes {
		var err error
		switch entityType + "." + change.Field {
		case "dialogue.text", "dialogue.emotion", "dialogue.performance_instruction", "dialogue.speaker_name":
			column := map[string]string{"text": "text", "emotion": "emotion", "performance_instruction": "performance_instruction", "speaker_name": "speaker_name"}[change.Field]
			_, err = tx.Exec(ctx, `UPDATE drama.dialogues SET `+column+`=$2 WHERE dialogue_id=$1`, entityID, fmt.Sprint(change.Value))
		case "dialogue.estimated_duration_ms":
			_, err = tx.Exec(ctx, `UPDATE drama.dialogues SET estimated_duration_ms=$2 WHERE dialogue_id=$1`, entityID, intValue(change.Value, change.Delta))
		case "dialogue.requested_speed":
			// Speed belongs to generated audio; the dialogue snapshot remains factually unchanged.
			_, err = tx.Exec(ctx, `UPDATE drama.dialogues SET performance_instruction=
				concat_ws('; ',NULLIF(performance_instruction,''),'speed='||$2) WHERE dialogue_id=$1`,
				entityID, fmt.Sprint(change.Value))
		case "dialogue.production_mode":
			mode := fmt.Sprint(change.Value)
			if mode != "spoken" && mode != "narration" && mode != "action" {
				return fmt.Errorf("%w: unsupported production mode %s", localedit.ErrInvalidPlan, mode)
			}
			_, err = tx.Exec(ctx, `UPDATE drama.dialogues SET production_mode=$2,
				dialogue_type=CASE WHEN $2='narration' THEN 'narration'
					WHEN $2='spoken' AND dialogue_type='narration' THEN 'dialogue'
					ELSE dialogue_type END WHERE dialogue_id=$1`, entityID, mode)
			if err == nil && mode == "action" {
				_, err = tx.Exec(ctx, `UPDATE drama.script_scenes scene SET actions=actions||
					jsonb_build_array(jsonb_build_object(
						'type','dialogue_converted_action','source_dialogue_id',dialogue.dialogue_id,
						'description',dialogue.text))
					FROM drama.dialogues dialogue WHERE dialogue.dialogue_id=$1
					  AND scene.scene_id=dialogue.scene_id`, entityID)
			}
		case "scene.estimated_duration_seconds":
			if change.Operation == "adjust" {
				_, err = tx.Exec(ctx, `UPDATE drama.script_scenes SET estimated_duration_seconds=
					GREATEST(1,estimated_duration_seconds+$2) WHERE scene_id=$1`, entityID, intValue(change.Value, change.Delta))
			} else {
				_, err = tx.Exec(ctx, `UPDATE drama.script_scenes SET estimated_duration_seconds=$2 WHERE scene_id=$1`, entityID, intValue(change.Value, change.Delta))
			}
		case "scene.actions":
			if change.Operation == "replace" {
				payload, _ := json.Marshal(change.Value)
				_, err = tx.Exec(ctx, `UPDATE drama.script_scenes SET actions=$2::jsonb WHERE scene_id=$1`, entityID, payload)
			} else {
				payload, _ := json.Marshal([]any{change.Value})
				_, err = tx.Exec(ctx, `UPDATE drama.script_scenes SET actions=actions||$2::jsonb WHERE scene_id=$1`, entityID, payload)
			}
		case "scene.scene_number":
			var currentNumber int
			if err = tx.QueryRow(ctx, `SELECT scene_number FROM drama.script_scenes WHERE scene_id=$1`, entityID).Scan(&currentNumber); err == nil {
				targetNumber := intValue(change.Value, change.Delta)
				if change.Operation == "reorder" {
					targetNumber = currentNumber + targetNumber
				}
				err = reorderScene(ctx, tx, entityID, targetNumber)
			}
		case "scene.scene_purpose", "scene.emotional_change", "scene.location_name", "scene.time_of_day", "scene.interior_exterior":
			column := change.Field
			_, err = tx.Exec(ctx, `UPDATE drama.script_scenes SET `+column+`=$2 WHERE scene_id=$1`, entityID, fmt.Sprint(change.Value))
		case "shot.action_description", "shot.facial_expression", "shot.composition", "shot.shot_size", "shot.camera_angle", "shot.camera_motion":
			column := change.Field
			_, err = tx.Exec(ctx, `UPDATE drama.storyboard_shots SET `+column+`=$2 WHERE shot_id=$1`, entityID, fmt.Sprint(change.Value))
		case "shot.duration_seconds":
			_, err = tx.Exec(ctx, `UPDATE drama.storyboard_shots SET duration_seconds=$2 WHERE shot_id=$1`, entityID, floatValue(change.Value, change.Delta))
		default:
			return fmt.Errorf("%w: executor does not support %s.%s", localedit.ErrInvalidPlan, entityType, change.Field)
		}
		if err != nil {
			return err
		}
	}
	if entityType == "dialogue" {
		if err := refreshDialogueDerived(ctx, tx, entityID, fingerprint); err != nil {
			return err
		}
	}
	return nil
}

func reorderScene(ctx context.Context, tx pgx.Tx, sceneID string, targetNumber int) error {
	if targetNumber < 1 {
		return fmt.Errorf("%w: scene number must remain positive", localedit.ErrInvalidPlan)
	}
	var scriptID string
	var currentNumber, temporaryNumber int
	if err := tx.QueryRow(ctx, `SELECT script_id,scene_number,
		(SELECT max(scene_number)+1000 FROM drama.script_scenes sibling WHERE sibling.script_id=scene.script_id)
		FROM drama.script_scenes scene WHERE scene_id=$1 FOR UPDATE`,
		sceneID).Scan(&scriptID, &currentNumber, &temporaryNumber); err != nil {
		return err
	}
	if currentNumber == targetNumber {
		return nil
	}
	var displacedID string
	err := tx.QueryRow(ctx, `SELECT scene_id FROM drama.script_scenes
		WHERE script_id=$1 AND scene_number=$2 FOR UPDATE`, scriptID, targetNumber).Scan(&displacedID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if displacedID != "" {
		if _, err = tx.Exec(ctx, `UPDATE drama.script_scenes SET scene_number=$2 WHERE scene_id=$1`,
			displacedID, temporaryNumber); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.script_scenes SET scene_number=$2 WHERE scene_id=$1`,
		sceneID, targetNumber); err != nil {
		return err
	}
	if displacedID != "" {
		_, err = tx.Exec(ctx, `UPDATE drama.script_scenes SET scene_number=$2 WHERE scene_id=$1`,
			displacedID, currentNumber)
	}
	return err
}

func refreshDialogueDerived(ctx context.Context, tx pgx.Tx, dialogueID, fingerprint string) error {
	var productionMode string
	if err := tx.QueryRow(ctx, `SELECT production_mode FROM drama.dialogues WHERE dialogue_id=$1`,
		dialogueID).Scan(&productionMode); err != nil {
		return err
	}
	if productionMode == "action" {
		if _, err := tx.Exec(ctx, `UPDATE drama.dialogue_audio SET is_current=false
			WHERE dialogue_id=$1 AND is_current`, dialogueID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE drama.subtitle_cues SET is_current=false,
			approval_state=CASE WHEN approval_state='approved' THEN 'superseded' ELSE approval_state END
			WHERE dialogue_id=$1 AND is_current`, dialogueID); err != nil {
			return err
		}
		return cloneDialogueEditTimelines(ctx, tx, dialogueID, "", fingerprint, true)
	}
	newAudioID := "da_mock_" + fingerprint[:20]
	if _, err := tx.Exec(ctx, `UPDATE drama.dialogue_audio SET is_current=false
		WHERE dialogue_id=$1 AND is_current`, dialogueID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO drama.dialogue_audio(
		dialogue_audio_id,project_id,episode_id,scene_id,dialogue_id,character_id,voice_profile_id,
		generation_version,dialogue_type,source_text,normalized_text,emotion,
		performance_instruction,requested_speed,provider,model,storage_url,actual_duration_ms,content_hash,
		status,auto_qc_status,auto_qc_report,review_status,is_current)
		SELECT $2,source.project_id,source.episode_id,source.scene_id,source.dialogue_id,
		source.character_id,source.voice_profile_id,
		(SELECT COALESCE(max(v.generation_version),0)+1 FROM drama.dialogue_audio v WHERE v.dialogue_id=source.dialogue_id),
		dialogue.dialogue_type,dialogue.text,dialogue.text,dialogue.emotion,
		dialogue.performance_instruction,source.requested_speed,'deterministic_mock','local-tts-v1',
		'/data/storage/dialogue-audio/'||$2::text||'.wav',dialogue.estimated_duration_ms,$3,'succeeded','passed',
		jsonb_build_object('deterministic_mock',true,'source_audio_id',source.dialogue_audio_id),
		'pending',true
		FROM drama.dialogue_audio source
		JOIN drama.dialogues dialogue ON dialogue.dialogue_id=source.dialogue_id
		WHERE source.dialogue_id=$1
		ORDER BY source.generation_version DESC LIMIT 1`,
		dialogueID, newAudioID, fingerprint)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.subtitle_cues SET
		is_current=false,
		approval_state=CASE WHEN approval_state='approved' THEN 'superseded' ELSE approval_state END
		WHERE dialogue_id=$1 AND is_current`, dialogueID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.subtitle_cues(
		subtitle_cue_id,project_id,episode_id,scene_id,shot_id,dialogue_id,dialogue_audio_id,
		sequence_number,speaker_name,text,start_ms,end_ms,duration_ms,style_config,status,
		cue_version,parent_subtitle_cue_id,is_current,approval_state)
		SELECT 'sc_mock_'||substr(encode(digest(source.subtitle_cue_id||':'||$2,'sha256'),'hex'),1,20),
		source.project_id,source.episode_id,source.scene_id,source.shot_id,source.dialogue_id,
		$2,source.sequence_number,dialogue.speaker_name,dialogue.text,source.start_ms,
		source.start_ms+dialogue.estimated_duration_ms,dialogue.estimated_duration_ms,
		source.style_config,'draft',source.cue_version+1,source.subtitle_cue_id,true,'draft'
		FROM (
		  SELECT DISTINCT ON(sequence_number) * FROM drama.subtitle_cues
		  WHERE dialogue_id=$1 ORDER BY sequence_number,cue_version DESC,created_at DESC
		) source
		JOIN drama.dialogues dialogue USING(dialogue_id)`,
		dialogueID, newAudioID); err != nil {
		return err
	}
	return cloneDialogueEditTimelines(ctx, tx, dialogueID, newAudioID, fingerprint, false)
}

// cloneDialogueEditTimelines materializes a new current timeline per affected
// episode. Approved/history timelines and their items are never rewritten.
func cloneDialogueEditTimelines(
	ctx context.Context, tx pgx.Tx, dialogueID, dialogueAudioID, fingerprint string, removeSpokenItems bool,
) error {
	rows, err := tx.Query(ctx, `SELECT timeline.timeline_id
		FROM drama.edit_timelines timeline
		WHERE timeline.is_current AND EXISTS(
			SELECT 1 FROM drama.edit_timeline_items item
			WHERE item.timeline_id=timeline.timeline_id
			  AND item.entity_type='dialogue' AND item.entity_id=$1
		)
		ORDER BY timeline.timeline_id FOR UPDATE OF timeline`, dialogueID)
	if err != nil {
		return err
	}
	var sourceTimelineIDs []string
	for rows.Next() {
		var sourceID string
		if err = rows.Scan(&sourceID); err != nil {
			rows.Close()
			return err
		}
		sourceTimelineIDs = append(sourceTimelineIDs, sourceID)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for index, sourceID := range sourceTimelineIDs {
		newTimelineID := fmt.Sprintf("etl_mock_%s_%d", fingerprint[:16], index+1)
		if _, err = tx.Exec(ctx, `UPDATE drama.edit_timelines SET
			is_current=false,
			approval_state=CASE WHEN approval_state='approved' THEN 'superseded' ELSE approval_state END
			WHERE timeline_id=$1`, sourceID); err != nil {
			return err
		}
		tag, insertErr := tx.Exec(ctx, `INSERT INTO drama.edit_timelines(
			timeline_id,project_id,episode_id,script_id,storyboard_id,audio_plan_id,version,
			resolution,aspect_ratio,fps,video_codec,audio_codec,sample_rate,target_duration_ms,
			tracks,transitions,subtitle_config,render_config,source_versions,status,
			parent_timeline_id,editing_template_binding_id,editing_template_version_id,
			version_reason,approval_state,is_current)
			SELECT $2,project_id,episode_id,script_id,storyboard_id,audio_plan_id,
			(SELECT max(version)+1 FROM drama.edit_timelines sibling WHERE sibling.episode_id=source.episode_id),
			resolution,aspect_ratio,fps,video_codec,audio_codec,sample_rate,target_duration_ms,
			tracks,transitions,subtitle_config,
			render_config||jsonb_build_object('incremental_dialogue_id',$3::text),
			source_versions||jsonb_build_object('dialogue_audio_id',$4::text),
			'draft',timeline_id,editing_template_binding_id,editing_template_version_id,
			'dialogue_edit:'||$3::text,'draft',true
			FROM drama.edit_timelines source WHERE timeline_id=$1`,
			sourceID, newTimelineID, dialogueID, dialogueAudioID)
		if insertErr != nil {
			return insertErr
		}
		if tag.RowsAffected() != 1 {
			return ErrNotFound
		}
		if _, err = tx.Exec(ctx, `INSERT INTO drama.edit_timeline_items(
			timeline_item_id,timeline_id,project_id,episode_id,track_type,track_number,
			sequence_number,entity_type,entity_id,source_url,source_path,timeline_start_ms,
			timeline_end_ms,source_in_ms,source_out_ms,duration_ms,volume,fade_in_ms,fade_out_ms,
			transform_config,effect_config,status)
			SELECT 'eti_'||substr(encode(digest(item.timeline_item_id||':'||$2,'sha256'),'hex'),1,24),
			$2,item.project_id,item.episode_id,item.track_type,item.track_number,item.sequence_number,
			item.entity_type,item.entity_id,
			CASE WHEN item.entity_type='dialogue' AND item.entity_id=$3
				  AND item.track_type IN('dialogue','narration')
				THEN (SELECT storage_url FROM drama.dialogue_audio WHERE dialogue_audio_id=$4)
				ELSE item.source_url END,
			item.source_path,item.timeline_start_ms,
			CASE WHEN item.entity_type='dialogue' AND item.entity_id=$3
				THEN item.timeline_start_ms+dialogue.estimated_duration_ms ELSE item.timeline_end_ms END,
			item.source_in_ms,
			CASE WHEN item.entity_type='dialogue' AND item.entity_id=$3
				  AND item.track_type IN('dialogue','narration')
				THEN item.source_in_ms+dialogue.estimated_duration_ms ELSE item.source_out_ms END,
			CASE WHEN item.entity_type='dialogue' AND item.entity_id=$3
				THEN dialogue.estimated_duration_ms ELSE item.duration_ms END,
			item.volume,
			CASE WHEN item.entity_type='dialogue' AND item.entity_id=$3
				THEN LEAST(item.fade_in_ms,dialogue.estimated_duration_ms) ELSE item.fade_in_ms END,
			CASE WHEN item.entity_type='dialogue' AND item.entity_id=$3
				THEN LEAST(item.fade_out_ms,GREATEST(0,dialogue.estimated_duration_ms-
					LEAST(item.fade_in_ms,dialogue.estimated_duration_ms)))
				ELSE item.fade_out_ms END,
			item.transform_config,
			CASE WHEN item.entity_type='dialogue' AND item.entity_id=$3
				THEN item.effect_config||jsonb_build_object('incremental_rebuild',true,'dialogue_audio_id',$4::text)
				ELSE item.effect_config END,
			CASE WHEN item.entity_type='dialogue' AND item.entity_id=$3 THEN 'pending' ELSE item.status END
			FROM drama.edit_timeline_items item
			LEFT JOIN drama.dialogues dialogue ON dialogue.dialogue_id=$3
			WHERE item.timeline_id=$1
			  AND NOT ($5::boolean AND item.entity_type='dialogue' AND item.entity_id=$3
			    AND item.track_type IN('dialogue','narration','subtitle'))`,
			sourceID, newTimelineID, dialogueID, dialogueAudioID, removeSpokenItems); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE drama.edit_timelines timeline SET target_duration_ms=(
			SELECT max(item.timeline_end_ms) FROM drama.edit_timeline_items item WHERE item.timeline_id=timeline.timeline_id
			) WHERE timeline_id=$1`, newTimelineID); err != nil {
			return err
		}
	}
	return nil
}

func cloneDeterministicShotVideo(ctx context.Context, tx pgx.Tx, sourceID string, changes []localedit.Change, fingerprint string) error {
	newID := "sv_mock_" + fingerprint[:20]
	var startMS, endMS int64
	for _, change := range changes {
		if change.Operation == "regenerate_segment" && change.StartMS != nil && change.EndMS != nil {
			startMS, endMS = *change.StartMS, *change.EndMS
		}
	}
	var durationMS int64
	if err := tx.QueryRow(ctx, `SELECT (COALESCE(actual_duration_seconds,requested_duration_seconds)*1000)::bigint
		FROM drama.shot_videos WHERE shot_video_id=$1`, sourceID).Scan(&durationMS); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if endMS > durationMS {
		return fmt.Errorf("%w: video segment exceeds source duration", localedit.ErrInvalidPlan)
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.shot_videos SET is_current=false WHERE shot_video_id=$1`, sourceID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO drama.shot_videos(
		shot_video_id,project_id,episode_id,storyboard_id,shot_id,storyboard_image_id,
		source_image_generation_version,generation_version,provider,model,video_prompt,
		negative_prompt,reference_image_url,reference_asset_ids,request_parameters,seed,
		requested_duration_seconds,actual_duration_seconds,aspect_ratio,width,height,fps,codec,
		has_audio,storage_url,content_hash,status,auto_qc_status,auto_qc_report,review_status,
		prompt_adjustment,is_current)
		SELECT $2::text,project_id,episode_id,storyboard_id,shot_id,storyboard_image_id,
		source_image_generation_version,
		(SELECT COALESCE(max(v.generation_version),0)+1 FROM drama.shot_videos v WHERE v.shot_id=source.shot_id),
		'deterministic_mock','local-segment-v1',video_prompt,negative_prompt,reference_image_url,
		reference_asset_ids,request_parameters||jsonb_build_object('segment_start_ms',$3::bigint,'segment_end_ms',$4::bigint),
		seed,requested_duration_seconds,actual_duration_seconds,aspect_ratio,width,height,fps,codec,
		has_audio,'/data/storage/shot-videos/'||$2::text||'.mp4',$5,'succeeded','passed',
		jsonb_build_object('deterministic_mock',true,'source_video_id',$1::text),'pending',
		'local segment rebuild',true
		FROM drama.shot_videos source WHERE shot_video_id=$1::text`,
		sourceID, newID, startMS, endMS, fingerprint)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func createDeterministicRebuildTasks(
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
	for index, action := range actions {
		taskID := "irt_" + fingerprint[:16] + "_" + strconv.Itoa(index+1)
		_, err := tx.Exec(ctx, `INSERT INTO drama.incremental_rebuild_tasks(
			rebuild_task_id,change_plan_id,project_id,action,target_entity_type,target_entity_id,
			range_start_ms,range_end_ms,status,provider,input,output,completed_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,'completed','deterministic_mock',
			jsonb_build_object('plan_fingerprint',$9::text),
			jsonb_build_object('mock',true,'result_key',$1||':result'),now())`,
			taskID, planID, projectID, action, entityType, entityID, startMS, endMS, fingerprint)
		if err != nil {
			return err
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
		requested_by,confirmed_by,confirmed_at,applied_at,error_code,error_message,created_at,updated_at
		FROM drama.change_plans WHERE project_id=$1 AND change_plan_id=$2`,
		projectID, planID).Scan(&item.ChangePlanID, &item.ProjectID, &item.Status, &item.Plan.UserIntent,
		&target.EntityType, &target.EntityID, &target.Version, &mustPreserve, &allowedFieldsJSON,
		&changesJSON, &upstreamJSON, &downstreamJSON, &rebuildJSON, &rebuildTasksJSON,
		&risksJSON, &rulesJSON, &item.Plan.RollbackVersion, &changeKind, &semantic, &locksJSON,
		&item.Fingerprint, &item.RequestedBy, &item.ConfirmedBy, &item.ConfirmedAt, &item.AppliedAt,
		&item.ErrorCode, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt)
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
	ctx context.Context, projectID, entityVersionID, mode string, requestedBy *string,
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

func restoreChanges(entityType string, sourceJSON, currentJSON json.RawMessage) ([]localedit.Change, error) {
	var source, current map[string]any
	if err := json.Unmarshal(sourceJSON, &source); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(currentJSON, &current); err != nil {
		return nil, err
	}
	fields := map[string][]string{
		"dialogue": {"text", "emotion", "performance_instruction", "estimated_duration_ms"},
		"scene": {
			"scene_purpose", "actions", "emotional_change", "estimated_duration_seconds",
			"location_name", "time_of_day", "interior_exterior",
		},
		"shot": {
			"action_description", "facial_expression", "composition", "shot_size",
			"camera_angle", "camera_motion", "duration_seconds",
		},
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
		artifact_id,range_start_ms,range_end_ms,status,provider,input,output,created_at,completed_at
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
			&item.Status, &item.Provider, &item.Input, &item.Output, &item.CreatedAt,
			&item.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
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
