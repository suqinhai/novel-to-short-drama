package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrInvalidEpisodeContent = errors.New("invalid episode content")

type EpisodeOutlineContent struct {
	EpisodeID                string    `json:"episode_id"`
	EpisodeNumber            int       `json:"episode_number"`
	Title                    string    `json:"title"`
	Logline                  string    `json:"logline"`
	OpeningHook              string    `json:"opening_hook"`
	StoryGoal                string    `json:"story_goal"`
	MainConflict             string    `json:"main_conflict"`
	Climax                   string    `json:"climax"`
	EndingHook               string    `json:"ending_hook"`
	EstimatedDurationSeconds int       `json:"estimated_duration_seconds"`
	Status                   string    `json:"status"`
	Version                  int       `json:"version"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type EpisodeDialogueContent struct {
	DialogueID             string    `json:"dialogue_id"`
	SequenceNumber         int       `json:"sequence_number"`
	DialogueType           string    `json:"dialogue_type"`
	CharacterID            *string   `json:"character_id,omitempty"`
	SpeakerName            string    `json:"speaker_name"`
	Text                   string    `json:"text"`
	Emotion                string    `json:"emotion"`
	PerformanceInstruction string    `json:"performance_instruction"`
	EstimatedDurationMS    int       `json:"estimated_duration_ms"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type EpisodeSceneContent struct {
	SceneID                  string                   `json:"scene_id"`
	SceneNumber              int                      `json:"scene_number"`
	LocationID               *string                  `json:"location_id,omitempty"`
	LocationName             string                   `json:"location_name"`
	TimeOfDay                string                   `json:"time_of_day"`
	InteriorExterior         string                   `json:"interior_exterior"`
	CharacterIDs             json.RawMessage          `json:"character_ids"`
	ScenePurpose             string                   `json:"scene_purpose"`
	Actions                  json.RawMessage          `json:"actions"`
	EmotionalChange          string                   `json:"emotional_change"`
	EstimatedDurationSeconds int                      `json:"estimated_duration_seconds"`
	SourceEventIDs           json.RawMessage          `json:"source_event_ids"`
	Dialogues                []EpisodeDialogueContent `json:"dialogues"`
	UpdatedAt                time.Time                `json:"updated_at"`
}

type EpisodeScriptContent struct {
	ScriptID                 string                `json:"script_id"`
	Version                  int                   `json:"version"`
	Title                    string                `json:"title"`
	OpeningHook              string                `json:"opening_hook"`
	Climax                   string                `json:"climax"`
	EndingHook               string                `json:"ending_hook"`
	EstimatedDurationSeconds int                   `json:"estimated_duration_seconds"`
	DialogueCharCount        int                   `json:"dialogue_char_count"`
	ContinuityReport         json.RawMessage       `json:"continuity_report"`
	QualityReport            json.RawMessage       `json:"quality_report"`
	Status                   string                `json:"status"`
	Scenes                   []EpisodeSceneContent `json:"scenes"`
	UpdatedAt                time.Time             `json:"updated_at"`
}

type EpisodeContent struct {
	ProjectID           string                `json:"project_id"`
	EpisodeRunID        string                `json:"episode_run_id"`
	EpisodeID           string                `json:"episode_id"`
	RunStatus           string                `json:"run_status"`
	CurrentStage        string                `json:"current_stage"`
	Editable            bool                  `json:"editable"`
	ReadOnlyReason      string                `json:"read_only_reason,omitempty"`
	HasDownstreamAssets bool                  `json:"has_downstream_assets"`
	Outline             EpisodeOutlineContent `json:"outline"`
	Script              *EpisodeScriptContent `json:"script,omitempty"`
}

type EpisodeOutlineUpdate struct {
	Title                    string `json:"title"`
	Logline                  string `json:"logline"`
	OpeningHook              string `json:"opening_hook"`
	StoryGoal                string `json:"story_goal"`
	MainConflict             string `json:"main_conflict"`
	Climax                   string `json:"climax"`
	EndingHook               string `json:"ending_hook"`
	EstimatedDurationSeconds int    `json:"estimated_duration_seconds"`
}

type EpisodeDialogueUpdate struct {
	DialogueID             string `json:"dialogue_id"`
	DialogueType           string `json:"dialogue_type"`
	SpeakerName            string `json:"speaker_name"`
	Text                   string `json:"text"`
	Emotion                string `json:"emotion"`
	PerformanceInstruction string `json:"performance_instruction"`
	EstimatedDurationMS    int    `json:"estimated_duration_ms"`
}

type EpisodeSceneUpdate struct {
	SceneID                  string                  `json:"scene_id"`
	LocationName             string                  `json:"location_name"`
	TimeOfDay                string                  `json:"time_of_day"`
	InteriorExterior         string                  `json:"interior_exterior"`
	ScenePurpose             string                  `json:"scene_purpose"`
	Actions                  json.RawMessage         `json:"actions"`
	EmotionalChange          string                  `json:"emotional_change"`
	EstimatedDurationSeconds int                     `json:"estimated_duration_seconds"`
	Dialogues                []EpisodeDialogueUpdate `json:"dialogues"`
}

type EpisodeScriptUpdate struct {
	ScriptID    string               `json:"script_id"`
	Title       string               `json:"title"`
	OpeningHook string               `json:"opening_hook"`
	Climax      string               `json:"climax"`
	EndingHook  string               `json:"ending_hook"`
	Scenes      []EpisodeSceneUpdate `json:"scenes"`
}

type UpdateEpisodeContentInput struct {
	Outline EpisodeOutlineUpdate `json:"outline"`
	Script  *EpisodeScriptUpdate `json:"script,omitempty"`
}

func (s *Store) GetEpisodeContent(ctx context.Context, projectID, episodeRunID string) (EpisodeContent, error) {
	var result EpisodeContent
	var activeTasks int
	err := s.pool.QueryRow(ctx, `SELECT run.project_id,run.episode_run_id,run.episode_id,
		run.status,run.current_stage,
		outline.episode_id,outline.episode_number,outline.title,outline.logline,
		outline.opening_hook,outline.story_goal,outline.main_conflict,outline.climax,
		outline.ending_hook,outline.estimated_duration_seconds,outline.status,
		outline.version,outline.updated_at,
		(SELECT count(*) FROM drama.workflow_tasks task
		 WHERE task.project_id=run.project_id AND task.status IN ('pending','running')
		   AND (task.entity_id=run.episode_id OR task.input_data->>'episode_id'=run.episode_id)),
		EXISTS(SELECT 1 FROM drama.storyboards board
		 WHERE board.project_id=run.project_id AND board.episode_id=run.episode_id)
		FROM drama.episode_production_runs run
		JOIN drama.episode_outlines outline
		  ON outline.project_id=run.project_id AND outline.episode_id=run.episode_id
		WHERE run.project_id=$1 AND run.episode_run_id=$2`,
		strings.TrimSpace(projectID), strings.TrimSpace(episodeRunID)).Scan(
		&result.ProjectID, &result.EpisodeRunID, &result.EpisodeID,
		&result.RunStatus, &result.CurrentStage,
		&result.Outline.EpisodeID, &result.Outline.EpisodeNumber, &result.Outline.Title,
		&result.Outline.Logline, &result.Outline.OpeningHook, &result.Outline.StoryGoal,
		&result.Outline.MainConflict, &result.Outline.Climax, &result.Outline.EndingHook,
		&result.Outline.EstimatedDurationSeconds, &result.Outline.Status,
		&result.Outline.Version, &result.Outline.UpdatedAt,
		&activeTasks, &result.HasDownstreamAssets,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return EpisodeContent{}, ErrNotFound
	}
	if err != nil {
		return EpisodeContent{}, err
	}
	result.Editable = activeTasks == 0
	if !result.Editable {
		result.ReadOnlyReason = "本集有生产任务正在执行，请等待任务结束后再修改"
	}

	var scriptJSON json.RawMessage
	err = s.pool.QueryRow(ctx, `SELECT jsonb_build_object(
		'script_id',script.script_id,'version',script.version,'title',script.title,
		'opening_hook',script.opening_hook,'climax',script.climax,'ending_hook',script.ending_hook,
		'estimated_duration_seconds',script.estimated_duration_seconds,
		'dialogue_char_count',script.dialogue_char_count,
		'continuity_report',script.continuity_report,'quality_report',script.quality_report,
		'status',script.status,'updated_at',script.updated_at,
		'scenes',COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'scene_id',scene.scene_id,'scene_number',scene.scene_number,
			'location_id',scene.location_id,'location_name',scene.location_name,
			'time_of_day',scene.time_of_day,'interior_exterior',scene.interior_exterior,
			'character_ids',scene.character_ids,'scene_purpose',scene.scene_purpose,
			'actions',scene.actions,'emotional_change',scene.emotional_change,
			'estimated_duration_seconds',scene.estimated_duration_seconds,
			'source_event_ids',scene.source_event_ids,'updated_at',scene.updated_at,
			'dialogues',COALESCE((SELECT jsonb_agg(jsonb_build_object(
				'dialogue_id',dialogue.dialogue_id,'sequence_number',dialogue.sequence_number,
				'dialogue_type',dialogue.dialogue_type,'character_id',dialogue.character_id,
				'speaker_name',dialogue.speaker_name,'text',dialogue.text,
				'emotion',dialogue.emotion,
				'performance_instruction',dialogue.performance_instruction,
				'estimated_duration_ms',dialogue.estimated_duration_ms,
				'updated_at',dialogue.updated_at) ORDER BY dialogue.sequence_number)
				FROM drama.dialogues dialogue WHERE dialogue.scene_id=scene.scene_id),'[]'::jsonb))
			ORDER BY scene.scene_number)
			FROM drama.script_scenes scene WHERE scene.script_id=script.script_id),'[]'::jsonb))
		FROM drama.episode_scripts script
		WHERE script.project_id=$1 AND script.episode_id=$2
		ORDER BY script.version DESC LIMIT 1`, result.ProjectID, result.EpisodeID).Scan(&scriptJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return EpisodeContent{}, err
	}
	var script EpisodeScriptContent
	if err = json.Unmarshal(scriptJSON, &script); err != nil {
		return EpisodeContent{}, fmt.Errorf("decode episode script content: %w", err)
	}
	if script.Scenes == nil {
		script.Scenes = make([]EpisodeSceneContent, 0)
	}
	result.Script = &script
	return result, nil
}

func (s *Store) UpdateEpisodeContent(
	ctx context.Context,
	projectID, episodeRunID string,
	input UpdateEpisodeContentInput,
) (EpisodeContent, error) {
	if err := validateEpisodeContentUpdate(input); err != nil {
		return EpisodeContent{}, err
	}
	projectID = strings.TrimSpace(projectID)
	episodeRunID = strings.TrimSpace(episodeRunID)
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return EpisodeContent{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, projectID); err != nil {
		return EpisodeContent{}, err
	}

	var episodeID string
	err = tx.QueryRow(ctx, `SELECT run.episode_id
		FROM drama.episode_production_runs run
		JOIN drama.episode_outlines outline
		  ON outline.project_id=run.project_id AND outline.episode_id=run.episode_id
		WHERE run.project_id=$1 AND run.episode_run_id=$2
		FOR UPDATE OF run,outline`, projectID, episodeRunID).Scan(&episodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return EpisodeContent{}, ErrNotFound
	}
	if err != nil {
		return EpisodeContent{}, err
	}
	var activeTasks int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM drama.workflow_tasks task
		WHERE task.project_id=$1 AND task.status IN ('pending','running')
		  AND (task.entity_id=$2 OR task.input_data->>'episode_id'=$2)`,
		projectID, episodeID).Scan(&activeTasks); err != nil {
		return EpisodeContent{}, err
	}
	if activeTasks > 0 {
		return EpisodeContent{}, fmt.Errorf("%w: episode has active workflow tasks", ErrConflict)
	}

	outline := input.Outline
	if _, err = tx.Exec(ctx, `UPDATE drama.episode_outlines SET
		title=$3,logline=$4,opening_hook=$5,story_goal=$6,main_conflict=$7,
		climax=$8,ending_hook=$9,estimated_duration_seconds=$10,updated_at=now()
		WHERE project_id=$1 AND episode_id=$2`,
		projectID, episodeID, strings.TrimSpace(outline.Title), strings.TrimSpace(outline.Logline),
		strings.TrimSpace(outline.OpeningHook), strings.TrimSpace(outline.StoryGoal),
		strings.TrimSpace(outline.MainConflict), strings.TrimSpace(outline.Climax),
		strings.TrimSpace(outline.EndingHook), outline.EstimatedDurationSeconds); err != nil {
		return EpisodeContent{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.episode_production_runs
		SET title=$3,updated_at=now() WHERE project_id=$1 AND episode_id=$2`,
		projectID, episodeID, strings.TrimSpace(outline.Title)); err != nil {
		return EpisodeContent{}, err
	}

	if input.Script != nil {
		if err = updateEpisodeScriptContent(ctx, tx, projectID, episodeID, *input.Script); err != nil {
			return EpisodeContent{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return EpisodeContent{}, err
	}
	return s.GetEpisodeContent(ctx, projectID, episodeRunID)
}

func updateEpisodeScriptContent(
	ctx context.Context,
	tx pgx.Tx,
	projectID, episodeID string,
	script EpisodeScriptUpdate,
) error {
	var currentScriptID string
	err := tx.QueryRow(ctx, `SELECT script_id FROM drama.episode_scripts
		WHERE project_id=$1 AND episode_id=$2
		ORDER BY version DESC LIMIT 1 FOR UPDATE`,
		projectID, episodeID).Scan(&currentScriptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: script does not exist", ErrConflict)
	}
	if err != nil {
		return err
	}
	if currentScriptID != strings.TrimSpace(script.ScriptID) {
		return fmt.Errorf("%w: script version changed, reload before saving", ErrConflict)
	}

	sceneRows, err := tx.Query(ctx, `SELECT scene_id FROM drama.script_scenes
		WHERE script_id=$1 ORDER BY scene_number FOR UPDATE`, currentScriptID)
	if err != nil {
		return err
	}
	sceneIDs := make(map[string]struct{})
	for sceneRows.Next() {
		var sceneID string
		if err = sceneRows.Scan(&sceneID); err != nil {
			sceneRows.Close()
			return err
		}
		sceneIDs[sceneID] = struct{}{}
	}
	sceneRows.Close()
	if err = sceneRows.Err(); err != nil {
		return err
	}
	if len(sceneIDs) != len(script.Scenes) {
		return fmt.Errorf("%w: scene set changed, reload before saving", ErrConflict)
	}

	dialogueRows, err := tx.Query(ctx, `SELECT dialogue_id,scene_id FROM drama.dialogues
		WHERE episode_id=$1 ORDER BY scene_id,sequence_number FOR UPDATE`, episodeID)
	if err != nil {
		return err
	}
	dialogueIDs := make(map[string]map[string]struct{})
	for dialogueRows.Next() {
		var dialogueID, sceneID string
		if err = dialogueRows.Scan(&dialogueID, &sceneID); err != nil {
			dialogueRows.Close()
			return err
		}
		if dialogueIDs[sceneID] == nil {
			dialogueIDs[sceneID] = make(map[string]struct{})
		}
		dialogueIDs[sceneID][dialogueID] = struct{}{}
	}
	dialogueRows.Close()
	if err = dialogueRows.Err(); err != nil {
		return err
	}

	for _, scene := range script.Scenes {
		if _, ok := sceneIDs[scene.SceneID]; !ok {
			return fmt.Errorf("%w: unknown scene %s", ErrConflict, scene.SceneID)
		}
		if len(dialogueIDs[scene.SceneID]) != len(scene.Dialogues) {
			return fmt.Errorf("%w: dialogue set changed for scene %s", ErrConflict, scene.SceneID)
		}
		if _, err = tx.Exec(ctx, `UPDATE drama.script_scenes SET
			location_name=$2,time_of_day=$3,interior_exterior=$4,scene_purpose=$5,
			actions=$6::jsonb,emotional_change=$7,estimated_duration_seconds=$8,
			updated_at=now() WHERE scene_id=$1`,
			scene.SceneID, strings.TrimSpace(scene.LocationName), strings.TrimSpace(scene.TimeOfDay),
			strings.TrimSpace(scene.InteriorExterior), strings.TrimSpace(scene.ScenePurpose),
			string(scene.Actions), strings.TrimSpace(scene.EmotionalChange),
			scene.EstimatedDurationSeconds); err != nil {
			return err
		}
		for _, dialogue := range scene.Dialogues {
			if _, ok := dialogueIDs[scene.SceneID][dialogue.DialogueID]; !ok {
				return fmt.Errorf("%w: unknown dialogue %s", ErrConflict, dialogue.DialogueID)
			}
			if _, err = tx.Exec(ctx, `UPDATE drama.dialogues SET dialogue_type=$2,
				speaker_name=$3,text=$4,emotion=$5,performance_instruction=$6,
				estimated_duration_ms=$7,updated_at=now()
				WHERE dialogue_id=$1 AND scene_id=$8`,
				dialogue.DialogueID, dialogue.DialogueType, strings.TrimSpace(dialogue.SpeakerName),
				strings.TrimSpace(dialogue.Text), strings.TrimSpace(dialogue.Emotion),
				strings.TrimSpace(dialogue.PerformanceInstruction),
				dialogue.EstimatedDurationMS, scene.SceneID); err != nil {
				return err
			}
		}
	}

	if _, err = tx.Exec(ctx, `UPDATE drama.script_scenes scene SET
		dialogues=COALESCE((SELECT jsonb_agg(to_jsonb(dialogue)
			-'id'-'project_id'-'episode_id'-'scene_id'-'created_at'-'updated_at'
			ORDER BY dialogue.sequence_number)
			FROM drama.dialogues dialogue
			WHERE dialogue.scene_id=scene.scene_id AND dialogue.dialogue_type<>'narration'),'[]'::jsonb),
		narration=COALESCE((SELECT jsonb_agg(to_jsonb(dialogue)
			-'id'-'project_id'-'episode_id'-'scene_id'-'created_at'-'updated_at'
			ORDER BY dialogue.sequence_number)
			FROM drama.dialogues dialogue
			WHERE dialogue.scene_id=scene.scene_id AND dialogue.dialogue_type='narration'),'[]'::jsonb)
		WHERE scene.script_id=$1`, currentScriptID); err != nil {
		return err
	}

	var dialogueCharacters, durationSeconds int
	if err = tx.QueryRow(ctx, `SELECT
		COALESCE((SELECT sum(length(dialogue.text))
			FROM drama.dialogues dialogue
			JOIN drama.script_scenes scene ON scene.scene_id=dialogue.scene_id
			WHERE dialogue.episode_id=$1 AND scene.script_id=$2),0)::int,
		COALESCE((SELECT sum(estimated_duration_seconds) FROM drama.script_scenes WHERE script_id=$2),0)::int`,
		episodeID, currentScriptID).Scan(&dialogueCharacters, &durationSeconds); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.episode_scripts script SET
		title=$2,opening_hook=$3,climax=$4,ending_hook=$5,
		estimated_duration_seconds=$6,dialogue_char_count=$7,
		scenes=COALESCE((SELECT jsonb_agg(to_jsonb(scene)
			-'id'-'script_id'-'project_id'-'episode_id'-'created_at'-'updated_at'
			ORDER BY scene.scene_number)
			FROM drama.script_scenes scene WHERE scene.script_id=script.script_id),'[]'::jsonb),
		updated_at=now()
		WHERE script.script_id=$1`,
		currentScriptID, strings.TrimSpace(script.Title), strings.TrimSpace(script.OpeningHook),
		strings.TrimSpace(script.Climax), strings.TrimSpace(script.EndingHook),
		durationSeconds, dialogueCharacters); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE drama.review_tasks SET
		metadata=COALESCE(metadata,'{}'::jsonb) || jsonb_build_object(
			'manually_edited',true,'manually_edited_at',now())
		WHERE project_id=$1 AND entity_id=$2`, projectID, currentScriptID)
	return err
}

func validateEpisodeContentUpdate(input UpdateEpisodeContentInput) error {
	outline := input.Outline
	if strings.TrimSpace(outline.Title) == "" || len([]rune(outline.Title)) > 400 {
		return fmt.Errorf("%w: outline title is required and must be at most 400 characters", ErrInvalidEpisodeContent)
	}
	if outline.EstimatedDurationSeconds < 1 || outline.EstimatedDurationSeconds > 3600 {
		return fmt.Errorf("%w: outline duration must be between 1 and 3600 seconds", ErrInvalidEpisodeContent)
	}
	for name, value := range map[string]string{
		"logline": outline.Logline, "opening_hook": outline.OpeningHook,
		"story_goal": outline.StoryGoal, "main_conflict": outline.MainConflict,
		"climax": outline.Climax, "ending_hook": outline.EndingHook,
	} {
		if len([]rune(value)) > 20000 {
			return fmt.Errorf("%w: %s is too long", ErrInvalidEpisodeContent, name)
		}
	}
	if input.Script == nil {
		return nil
	}
	script := input.Script
	if strings.TrimSpace(script.ScriptID) == "" || strings.TrimSpace(script.Title) == "" {
		return fmt.Errorf("%w: script id and title are required", ErrInvalidEpisodeContent)
	}
	if len(script.Scenes) == 0 {
		return fmt.Errorf("%w: script must contain at least one scene", ErrInvalidEpisodeContent)
	}
	seenScenes := make(map[string]struct{}, len(script.Scenes))
	allowedDialogueTypes := map[string]bool{
		"dialogue": true, "narration": true, "inner_monologue": true, "off_screen": true,
	}
	for _, scene := range script.Scenes {
		if strings.TrimSpace(scene.SceneID) == "" {
			return fmt.Errorf("%w: scene id is required", ErrInvalidEpisodeContent)
		}
		if _, exists := seenScenes[scene.SceneID]; exists {
			return fmt.Errorf("%w: duplicate scene id", ErrInvalidEpisodeContent)
		}
		seenScenes[scene.SceneID] = struct{}{}
		if scene.EstimatedDurationSeconds < 1 || scene.EstimatedDurationSeconds > 1800 {
			return fmt.Errorf("%w: scene duration must be between 1 and 1800 seconds", ErrInvalidEpisodeContent)
		}
		if len(scene.Actions) > 100000 {
			return fmt.Errorf("%w: scene actions are too large", ErrInvalidEpisodeContent)
		}
		var actions []any
		if len(scene.Actions) == 0 || json.Unmarshal(scene.Actions, &actions) != nil {
			return fmt.Errorf("%w: scene actions must be a JSON array", ErrInvalidEpisodeContent)
		}
		seenDialogues := make(map[string]struct{}, len(scene.Dialogues))
		for _, dialogue := range scene.Dialogues {
			if strings.TrimSpace(dialogue.DialogueID) == "" || strings.TrimSpace(dialogue.Text) == "" {
				return fmt.Errorf("%w: dialogue id and text are required", ErrInvalidEpisodeContent)
			}
			if _, exists := seenDialogues[dialogue.DialogueID]; exists {
				return fmt.Errorf("%w: duplicate dialogue id", ErrInvalidEpisodeContent)
			}
			seenDialogues[dialogue.DialogueID] = struct{}{}
			if !allowedDialogueTypes[dialogue.DialogueType] {
				return fmt.Errorf("%w: unsupported dialogue type", ErrInvalidEpisodeContent)
			}
			if dialogue.EstimatedDurationMS < 1 || dialogue.EstimatedDurationMS > 600000 {
				return fmt.Errorf("%w: dialogue duration must be between 1 and 600000 milliseconds", ErrInvalidEpisodeContent)
			}
			if len([]rune(dialogue.Text)) > 5000 {
				return fmt.Errorf("%w: dialogue text is too long", ErrInvalidEpisodeContent)
			}
		}
	}
	return nil
}
