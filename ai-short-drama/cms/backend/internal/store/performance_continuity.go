package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/localedit"
	pc "short-drama-cms/backend/internal/performancecontinuity"
)

type PerformanceBibleRecord struct {
	PerformanceBibleID string          `json:"performance_bible_id"`
	ProjectID          string          `json:"project_id"`
	CharacterID        string          `json:"character_id"`
	CharacterVersion   string          `json:"character_version"`
	Version            int             `json:"version"`
	Speech             json.RawMessage `json:"speech"`
	Acting             json.RawMessage `json:"acting"`
	RelationalVoices   json.RawMessage `json:"relational_voices"`
	Appearance         json.RawMessage `json:"appearance"`
	StageStates        json.RawMessage `json:"stage_states"`
	LockedFields       json.RawMessage `json:"locked_fields"`
	AllowedFields      json.RawMessage `json:"allowed_fields"`
	ChangeReasons      json.RawMessage `json:"change_reasons"`
	SourceRefs         json.RawMessage `json:"source_refs"`
	Status             string          `json:"status"`
	ContentHash        string          `json:"content_hash"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type CreatePerformanceBibleInput struct {
	CharacterID      string          `json:"character_id"`
	CharacterVersion string          `json:"character_version"`
	Speech           json.RawMessage `json:"speech"`
	Acting           json.RawMessage `json:"acting"`
	RelationalVoices json.RawMessage `json:"relational_voices"`
	Appearance       json.RawMessage `json:"appearance"`
	StageStates      []pc.StageState `json:"stage_states"`
	LockedFields     json.RawMessage `json:"locked_fields"`
	AllowedFields    json.RawMessage `json:"allowed_fields"`
	ChangeReasons    json.RawMessage `json:"change_reasons"`
	SourceRefs       json.RawMessage `json:"source_refs"`
	ParentID         *string         `json:"parent_performance_bible_id,omitempty"`
	ChangeReason     string          `json:"change_reason"`
	CreatedBy        *string         `json:"created_by,omitempty"`
}

type ContinuityRecord struct {
	ContinuityEntryID string          `json:"continuity_entry_id"`
	EpisodeID         string          `json:"episode_id"`
	EpisodeNumber     int             `json:"episode_number"`
	SceneID           *string         `json:"scene_id,omitempty"`
	ShotID            *string         `json:"shot_id,omitempty"`
	Scope             string          `json:"scope"`
	SequenceNumber    int             `json:"sequence_number"`
	InputState        json.RawMessage `json:"input_state"`
	OutputState       json.RawMessage `json:"output_state"`
	InheritedFrom     *string         `json:"inherited_from_entry_id,omitempty"`
	ValidationStatus  string          `json:"validation_status"`
	Diagnostics       json.RawMessage `json:"diagnostics"`
}

type VisualQCIssueRecord struct {
	VisualQCIssueID string          `json:"visual_qc_issue_id"`
	EpisodeID       string          `json:"episode_id"`
	SceneID         string          `json:"scene_id"`
	ShotID          string          `json:"shot_id"`
	Category        string          `json:"category"`
	Severity        string          `json:"severity"`
	TimecodeMS      int64           `json:"timecode_ms"`
	FrameNumber     int             `json:"frame_number"`
	Evidence        json.RawMessage `json:"evidence"`
	Recommendation  string          `json:"recommendation"`
	Status          string          `json:"status"`
}

type ShotHandoffRecord struct {
	ShotHandoffID         string          `json:"shot_handoff_id"`
	EpisodeID             string          `json:"episode_id"`
	FromShotID            string          `json:"from_shot_id"`
	ToShotID              string          `json:"to_shot_id"`
	TargetTailFrameRef    *string         `json:"target_tail_frame_ref,omitempty"`
	ReferenceHeadFrameRef *string         `json:"reference_head_frame_ref,omitempty"`
	PoseConstraints       json.RawMessage `json:"pose_constraints"`
	GazeConstraint        string          `json:"gaze_constraint"`
	MotionDirection       string          `json:"motion_direction"`
	FromActionPhase       string          `json:"from_action_phase"`
	ToActionPhase         string          `json:"to_action_phase"`
	ShotSizeConstraint    string          `json:"shot_size_constraint"`
	CompositionConstraint string          `json:"composition_constraint"`
	Version               int             `json:"version"`
	Status                string          `json:"status"`
	Diagnostics           json.RawMessage `json:"diagnostics"`
}

func (s *Store) ListPerformanceBibles(ctx context.Context, projectID string) ([]PerformanceBibleRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT performance_bible_id,project_id,character_id,character_version,
		version,speech,acting,relational_voices,appearance,
		(SELECT COALESCE(jsonb_agg(jsonb_build_object(
		  'stage_key',stage.stage_key,'episode_from',stage.episode_from,'episode_to',stage.episode_to,
		  'costume',stage.costume_state->>'costume','scars',stage.scars,'props',stage.props,
		  'psychology',stage.psychology,'relationships',stage.relationships
		) ORDER BY stage.episode_from),'[]'::jsonb)
		 FROM drama.character_performance_stage_states stage
		 WHERE stage.performance_bible_id=bible.performance_bible_id) stage_states,
		locked_fields,allowed_fields,change_reasons,
		source_refs,status,content_hash,created_at,updated_at
		FROM drama.character_performance_bibles bible WHERE bible.project_id=$1
		ORDER BY character_id,character_version,version DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PerformanceBibleRecord, 0)
	for rows.Next() {
		var item PerformanceBibleRecord
		if err = rows.Scan(&item.PerformanceBibleID, &item.ProjectID, &item.CharacterID, &item.CharacterVersion,
			&item.Version, &item.Speech, &item.Acting, &item.RelationalVoices, &item.Appearance, &item.StageStates,
			&item.LockedFields, &item.AllowedFields, &item.ChangeReasons, &item.SourceRefs,
			&item.Status, &item.ContentHash, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CreatePerformanceBibleVersion(ctx context.Context, projectID string, input CreatePerformanceBibleInput) (PerformanceBibleRecord, error) {
	input.CharacterID = strings.TrimSpace(input.CharacterID)
	input.CharacterVersion = strings.TrimSpace(input.CharacterVersion)
	input.ChangeReason = strings.TrimSpace(input.ChangeReason)
	if input.CharacterID == "" || input.CharacterVersion == "" || input.ChangeReason == "" {
		return PerformanceBibleRecord{}, fmt.Errorf("%w: character, version and change reason are required", pc.ErrInvalidInput)
	}
	for _, value := range []*json.RawMessage{&input.Speech, &input.Acting, &input.RelationalVoices, &input.Appearance, &input.LockedFields, &input.AllowedFields, &input.ChangeReasons, &input.SourceRefs} {
		if len(*value) == 0 {
			*value = json.RawMessage(`{}`)
		}
	}
	if string(input.LockedFields) == "{}" {
		input.LockedFields = json.RawMessage(`[]`)
	}
	if string(input.AllowedFields) == "{}" {
		input.AllowedFields = json.RawMessage(`[]`)
	}
	canonical, _ := json.Marshal(input)
	hash := sha256.Sum256(append([]byte(projectID+":"), canonical...))
	contentHash := hex.EncodeToString(hash[:])
	var version int
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1
		FROM drama.character_performance_bibles
		WHERE project_id=$1 AND character_id=$2 AND character_version=$3`,
		projectID, input.CharacterID, input.CharacterVersion).Scan(&version); err != nil {
		return PerformanceBibleRecord{}, err
	}
	id := "pb_" + contentHash[:20]
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return PerformanceBibleRecord{}, err
	}
	defer tx.Rollback(ctx)
	var result PerformanceBibleRecord
	err = tx.QueryRow(ctx, `INSERT INTO drama.character_performance_bibles(
		performance_bible_id,project_id,character_id,character_version,version,speech,acting,
		relational_voices,appearance,locked_fields,allowed_fields,change_reasons,source_refs,status,
		parent_performance_bible_id,content_hash,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,
			COALESCE($12::jsonb,'{}'::jsonb)||jsonb_build_object('_version',$13::text),
			$14,'draft',$15,$16,$17)
		RETURNING performance_bible_id,project_id,character_id,character_version,version,speech,acting,
		relational_voices,appearance,locked_fields,allowed_fields,change_reasons,source_refs,status,
		content_hash,created_at,updated_at`,
		id, projectID, input.CharacterID, input.CharacterVersion, version, input.Speech, input.Acting,
		input.RelationalVoices, input.Appearance, input.LockedFields, input.AllowedFields,
		input.ChangeReasons, input.ChangeReason, input.SourceRefs, input.ParentID, contentHash, input.CreatedBy,
	).Scan(&result.PerformanceBibleID, &result.ProjectID, &result.CharacterID, &result.CharacterVersion,
		&result.Version, &result.Speech, &result.Acting, &result.RelationalVoices, &result.Appearance,
		&result.LockedFields, &result.AllowedFields, &result.ChangeReasons, &result.SourceRefs,
		&result.Status, &result.ContentHash, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return PerformanceBibleRecord{}, err
	}
	for index, stage := range input.StageStates {
		if strings.TrimSpace(stage.StageKey) == "" || strings.TrimSpace(stage.Psychology) == "" {
			return PerformanceBibleRecord{}, fmt.Errorf("%w: stage key and psychology are required", pc.ErrInvalidInput)
		}
		scars, _ := json.Marshal(stage.Scars)
		props, _ := json.Marshal(stage.Props)
		relationships, _ := json.Marshal(stage.Relationships)
		stageHash := sha256.Sum256([]byte(id + ":" + stage.StageKey))
		stageID := "pbs_" + hex.EncodeToString(stageHash[:])[:20]
		if _, err = tx.Exec(ctx, `INSERT INTO drama.character_performance_stage_states(
			performance_stage_state_id,performance_bible_id,stage_key,episode_from,episode_to,
			costume_state,scars,props,psychology,relationships,change_reason)
			VALUES($1,$2,$3,$4,$5,jsonb_build_object('costume',$6::text),$7,$8,$9,$10,$11)`,
			stageID, id, stage.StageKey, stage.EpisodeFrom, stage.EpisodeTo, stage.Costume,
			scars, props, stage.Psychology, relationships,
			fmt.Sprintf("%s [stage %d]", input.ChangeReason, index+1)); err != nil {
			return PerformanceBibleRecord{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return PerformanceBibleRecord{}, err
	}
	result.StageStates, _ = json.Marshal(input.StageStates)
	return result, nil
}

func (s *Store) LockPerformanceBible(ctx context.Context, performanceBibleID string) (PerformanceBibleRecord, error) {
	var projectID string
	err := s.pool.QueryRow(ctx, `SELECT project_id FROM drama.character_performance_bibles
		WHERE performance_bible_id=$1`, performanceBibleID).Scan(&projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return PerformanceBibleRecord{}, ErrNotFound
	}
	if err != nil {
		return PerformanceBibleRecord{}, err
	}
	tag, err := s.writer.Exec(ctx, `UPDATE drama.character_performance_bibles SET status='locked'
		WHERE performance_bible_id=$1 AND status IN ('draft','approved')`, performanceBibleID)
	if err != nil {
		return PerformanceBibleRecord{}, err
	}
	if tag.RowsAffected() == 0 {
		return PerformanceBibleRecord{}, ErrConflict
	}
	items, err := s.ListPerformanceBibles(ctx, projectID)
	if err != nil {
		return PerformanceBibleRecord{}, err
	}
	for _, item := range items {
		if item.PerformanceBibleID == performanceBibleID {
			return item, nil
		}
	}
	return PerformanceBibleRecord{}, ErrNotFound
}

func (s *Store) ListContinuityLedger(ctx context.Context, projectID, episodeID string) ([]ContinuityRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT continuity_entry_id,episode_id,episode_number,scene_id,shot_id,
		scope,sequence_number,input_state,output_state,inherited_from_entry_id,validation_status,diagnostics
		FROM drama.continuity_ledger_entries
		WHERE project_id=$1 AND ($2='' OR episode_id=$2)
		ORDER BY episode_number,sequence_number`, projectID, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ContinuityRecord, 0)
	for rows.Next() {
		var item ContinuityRecord
		if err = rows.Scan(&item.ContinuityEntryID, &item.EpisodeID, &item.EpisodeNumber,
			&item.SceneID, &item.ShotID, &item.Scope, &item.SequenceNumber, &item.InputState,
			&item.OutputState, &item.InheritedFrom, &item.ValidationStatus, &item.Diagnostics); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListVisualQCIssues(ctx context.Context, projectID, episodeID, severity string) ([]VisualQCIssueRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT visual_qc_issue_id,episode_id,scene_id,shot_id,category,severity,
		timecode_ms,frame_number,evidence,recommendation,status
		FROM drama.visual_qc_issues WHERE project_id=$1
		  AND ($2='' OR episode_id=$2) AND ($3='' OR severity=$3)
		ORDER BY CASE severity WHEN 'blocking' THEN 0 WHEN 'critical' THEN 1 WHEN 'major' THEN 2 ELSE 3 END,
		  episode_id,scene_id,shot_id,timecode_ms,frame_number`, projectID, episodeID, severity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]VisualQCIssueRecord, 0)
	for rows.Next() {
		var item VisualQCIssueRecord
		if err = rows.Scan(&item.VisualQCIssueID, &item.EpisodeID, &item.SceneID, &item.ShotID,
			&item.Category, &item.Severity, &item.TimecodeMS, &item.FrameNumber, &item.Evidence,
			&item.Recommendation, &item.Status); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListShotHandoffs(ctx context.Context, projectID, episodeID string) ([]ShotHandoffRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT shot_handoff_id,episode_id,from_shot_id,to_shot_id,
		target_tail_frame_ref,reference_head_frame_ref,pose_constraints,gaze_constraint,motion_direction,
		from_action_phase,to_action_phase,shot_size_constraint,composition_constraint,version,status,diagnostics
		FROM drama.shot_handoffs WHERE project_id=$1 AND ($2='' OR episode_id=$2)
		ORDER BY episode_id,from_shot_id,to_shot_id,version DESC`, projectID, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ShotHandoffRecord, 0)
	for rows.Next() {
		var item ShotHandoffRecord
		if err = rows.Scan(&item.ShotHandoffID, &item.EpisodeID, &item.FromShotID, &item.ToShotID,
			&item.TargetTailFrameRef, &item.ReferenceHeadFrameRef, &item.PoseConstraints,
			&item.GazeConstraint, &item.MotionDirection, &item.FromActionPhase, &item.ToActionPhase,
			&item.ShotSizeConstraint, &item.CompositionConstraint, &item.Version, &item.Status,
			&item.Diagnostics); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SaveVisualQCFixtureRun(
	ctx context.Context, projectID, episodeID, fixtureID string, issues []pc.QCIssue,
) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("MOCK_MODE")), "true") {
		return "", fmt.Errorf("%w: visual QC fixture requires MOCK_MODE=true", ErrConflict)
	}
	sum := sha256.Sum256([]byte(projectID + ":" + episodeID + ":" + fixtureID))
	runID := "vqcr_" + hex.EncodeToString(sum[:])[:20]
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO drama.visual_qc_runs(
		visual_qc_run_id,project_id,episode_id,fixture_id,provider,status,issue_count,started_at,completed_at)
		VALUES($1,$2,$3,$4,'deterministic_mock','completed',$5,now(),now())
		ON CONFLICT(visual_qc_run_id) DO UPDATE SET issue_count=excluded.issue_count,
		  status='completed',completed_at=now()`,
		runID, projectID, episodeID, fixtureID, len(issues)); err != nil {
		return "", err
	}
	for _, issue := range issues {
		evidence, _ := json.Marshal(map[string]any{
			"summary": issue.Evidence, "local_redo": issue.LocalRedo, "fixture_id": fixtureID,
		})
		if _, err = tx.Exec(ctx, `INSERT INTO drama.visual_qc_issues(
			visual_qc_issue_id,visual_qc_run_id,project_id,episode_id,scene_id,shot_id,category,
			severity,timecode_ms,frame_number,evidence,recommendation,status)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'open')
			ON CONFLICT(visual_qc_issue_id) DO UPDATE SET evidence=excluded.evidence,
			  recommendation=excluded.recommendation`,
			issue.IssueID, runID, projectID, episodeID, issue.Locator.SceneID, issue.Locator.ShotID,
			issue.Category, issue.Severity, issue.Locator.TimecodeMS, issue.Locator.Frame,
			evidence, issue.Recommendation); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return runID, nil
}

func (s *Store) CreateVisualQCRedoPlan(ctx context.Context, issueID string, requestedBy *string) (ChangePlan, error) {
	var projectID, shotID, recommendation string
	var timecodeMS int64
	var version int
	err := s.pool.QueryRow(ctx, `SELECT issue.project_id,issue.shot_id,issue.recommendation,issue.timecode_ms,
		shot.generation_version FROM drama.visual_qc_issues issue
		JOIN drama.storyboard_shots shot ON shot.shot_id=issue.shot_id
		WHERE issue.visual_qc_issue_id=$1 AND issue.status='open'`, issueID).
		Scan(&projectID, &shotID, &recommendation, &timecodeMS, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangePlan{}, ErrNotFound
	}
	if err != nil {
		return ChangePlan{}, err
	}
	plan, err := localedit.Build(localedit.Request{
		Instruction:   "Resolve visual QC issue " + issueID + ": " + recommendation,
		Target:        localedit.Target{EntityType: "shot", EntityID: shotID, Version: version},
		AllowedFields: []string{"action_description"},
		Changes: []localedit.Change{{
			Operation: "regenerate", Field: "action_description",
			Value: recommendation + fmt.Sprintf(" [frame range %d-%dms]", maxInt64(0, timecodeMS-250), timecodeMS+250),
		}},
	})
	if err != nil {
		return ChangePlan{}, err
	}
	result, err := s.CreateChangePlan(ctx, projectID, plan, requestedBy)
	if err != nil {
		return ChangePlan{}, err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ChangePlan{}, err
	}
	defer tx.Rollback(ctx)
	linkID := "vqrp_" + issueID
	if _, err = tx.Exec(ctx, `INSERT INTO drama.visual_qc_local_redo_plans(
		visual_qc_local_redo_plan_id,visual_qc_issue_id,change_plan_id,range_start_ms,range_end_ms,adjacency_scope)
		VALUES($1,$2,$3,$4,$5,(SELECT COALESCE(jsonb_agg(shot_handoff_id),'[]'::jsonb)
		  FROM drama.shot_handoffs WHERE from_shot_id=$6 OR to_shot_id=$6))`,
		linkID, issueID, result.ChangePlanID, maxInt64(0, timecodeMS-250), timecodeMS+250, shotID); err != nil {
		return ChangePlan{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.visual_qc_issues SET status='planned'
		WHERE visual_qc_issue_id=$1`, issueID); err != nil {
		return ChangePlan{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ChangePlan{}, err
	}
	return result, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
