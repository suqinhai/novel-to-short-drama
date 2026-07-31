package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/localedit"
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
	Revision            int                   `json:"revision"`
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

type EpisodeContentChangePlanInput struct {
	ExpectedVersion int                  `json:"expected_version"`
	Outline         EpisodeOutlineUpdate `json:"outline"`
	Script          *EpisodeScriptUpdate `json:"script,omitempty"`
	MustPreserve    []string             `json:"must_preserve,omitempty"`
	Locks           []string             `json:"locks,omitempty"`
	RebuildTasks    []string             `json:"rebuild_tasks,omitempty"`
	RequestedBy     *string              `json:"requested_by,omitempty"`
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
		(EXISTS(SELECT 1 FROM drama.storyboards board
		  WHERE board.project_id=run.project_id AND board.episode_id=run.episode_id)
		 OR EXISTS(SELECT 1 FROM drama.shot_videos video
		  WHERE video.project_id=run.project_id AND video.episode_id=run.episode_id)
		 OR EXISTS(SELECT 1 FROM drama.dialogue_audio audio
		  WHERE audio.project_id=run.project_id AND audio.episode_id=run.episode_id)
		 OR EXISTS(SELECT 1 FROM drama.edit_timelines timeline
		  WHERE timeline.project_id=run.project_id AND timeline.episode_id=run.episode_id))
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
	result.Revision = result.Outline.Version

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
		err = nil
	} else if err != nil {
		return EpisodeContent{}, err
	} else {
		var script EpisodeScriptContent
		if err = json.Unmarshal(scriptJSON, &script); err != nil {
			return EpisodeContent{}, fmt.Errorf("decode episode script content: %w", err)
		}
		if script.Scenes == nil {
			script.Scenes = make([]EpisodeSceneContent, 0)
		}
		result.Script = &script
		if script.Version > result.Revision {
			result.Revision = script.Version
		}
	}

	var versioned json.RawMessage
	var version int
	err = s.pool.QueryRow(ctx, `SELECT version,content FROM drama.entity_versions
		WHERE project_id=$1 AND entity_type='episode_content' AND entity_id=$2 AND is_current`,
		result.ProjectID, result.EpisodeID).Scan(&version, &versioned)
	if err == nil {
		var snapshot struct {
			Outline EpisodeOutlineContent `json:"outline"`
			Script  *EpisodeScriptContent `json:"script"`
		}
		if err = json.Unmarshal(versioned, &snapshot); err != nil {
			return EpisodeContent{}, fmt.Errorf("decode versioned episode content: %w", err)
		}
		result.Outline, result.Script, result.Revision = snapshot.Outline, snapshot.Script, version
		if result.Script != nil && result.Script.Scenes == nil {
			result.Script.Scenes = make([]EpisodeSceneContent, 0)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return EpisodeContent{}, err
	}
	baseSnapshot, err := json.Marshal(map[string]any{
		"outline": result.Outline,
		"script":  result.Script,
	})
	if err != nil {
		return EpisodeContent{}, err
	}
	overlaid, err := overlayEpisodeSnapshot(
		ctx, s.pool, result.ProjectID, result.EpisodeID, baseSnapshot,
	)
	if err != nil {
		return EpisodeContent{}, err
	}
	var finalSnapshot struct {
		Outline EpisodeOutlineContent `json:"outline"`
		Script  *EpisodeScriptContent `json:"script"`
	}
	if err = json.Unmarshal(overlaid, &finalSnapshot); err != nil {
		return EpisodeContent{}, err
	}
	result.Outline, result.Script = finalSnapshot.Outline, finalSnapshot.Script
	return result, nil
}

// CreateEpisodeContentChangePlan is the only write entry for the episode
// content modal. It turns the submitted draft into an immutable, reviewable
// field diff; no formal content is changed here.
func (s *Store) CreateEpisodeContentChangePlan(
	ctx context.Context,
	projectID, episodeRunID string,
	input EpisodeContentChangePlanInput,
	requestedBy *string,
) (ChangePlan, error) {
	if err := validateEpisodeContentChangePlanInput(input); err != nil {
		return ChangePlan{}, err
	}
	current, err := s.GetEpisodeContent(ctx, strings.TrimSpace(projectID), strings.TrimSpace(episodeRunID))
	if err != nil {
		return ChangePlan{}, err
	}
	if input.ExpectedVersion < 1 {
		return ChangePlan{}, fmt.Errorf("%w: expected_version is required", ErrInvalidEpisodeContent)
	}
	if input.ExpectedVersion != current.Revision {
		return ChangePlan{}, fmt.Errorf("%w: target version is %d, current is %d",
			ErrConflict, input.ExpectedVersion, current.Revision)
	}
	if !current.Editable {
		return ChangePlan{}, fmt.Errorf("%w: episode has active workflow tasks", ErrConflict)
	}

	changes, err := episodeContentChanges(current, input)
	if err != nil {
		return ChangePlan{}, err
	}
	request := localedit.Request{
		Instruction: fmt.Sprintf("版本化修改第 %d 集大纲与剧本内容", current.Outline.EpisodeNumber),
		Target: localedit.Target{
			EntityType: "episode_content", EntityID: current.EpisodeID, Version: current.Revision,
		},
		Changes: changes,
		MustPreserve: append([]string{
			"未出现在 diff 中的字段", "source_event_ids", "character_id", "source evidence",
		}, input.MustPreserve...),
		Locks: input.Locks,
	}
	if !current.HasDownstreamAssets {
		request.RebuildTasks = []string{}
	} else if input.RebuildTasks != nil {
		request.RebuildTasks = input.RebuildTasks
	}
	plan, err := localedit.Build(request)
	if err != nil {
		return ChangePlan{}, err
	}
	return s.CreateChangePlan(ctx, current.ProjectID, plan, requestedBy)
}

func episodeContentChanges(
	current EpisodeContent, input EpisodeContentChangePlanInput,
) ([]localedit.Change, error) {
	changes := make([]localedit.Change, 0)
	add := func(path string, before, after any) {
		if !reflect.DeepEqual(normalizeJSONValue(before), normalizeJSONValue(after)) {
			changes = append(changes, localedit.Change{
				Operation: "replace", Field: path, Value: after,
			})
		}
	}
	outline := input.Outline
	add("outline.title", current.Outline.Title, strings.TrimSpace(outline.Title))
	add("outline.logline", current.Outline.Logline, strings.TrimSpace(outline.Logline))
	add("outline.opening_hook", current.Outline.OpeningHook, strings.TrimSpace(outline.OpeningHook))
	add("outline.story_goal", current.Outline.StoryGoal, strings.TrimSpace(outline.StoryGoal))
	add("outline.main_conflict", current.Outline.MainConflict, strings.TrimSpace(outline.MainConflict))
	add("outline.climax", current.Outline.Climax, strings.TrimSpace(outline.Climax))
	add("outline.ending_hook", current.Outline.EndingHook, strings.TrimSpace(outline.EndingHook))
	add("outline.estimated_duration_seconds", current.Outline.EstimatedDurationSeconds, outline.EstimatedDurationSeconds)

	if input.Script == nil {
		if current.Script != nil {
			return nil, fmt.Errorf("%w: an existing script cannot be omitted", ErrConflict)
		}
		return changes, nil
	}
	if current.Script == nil || strings.TrimSpace(input.Script.ScriptID) != current.Script.ScriptID {
		return nil, fmt.Errorf("%w: script version changed, reload before preview", ErrConflict)
	}
	script := input.Script
	add("script.title", current.Script.Title, strings.TrimSpace(script.Title))
	add("script.opening_hook", current.Script.OpeningHook, strings.TrimSpace(script.OpeningHook))
	add("script.climax", current.Script.Climax, strings.TrimSpace(script.Climax))
	add("script.ending_hook", current.Script.EndingHook, strings.TrimSpace(script.EndingHook))

	currentScenes := make(map[string]EpisodeSceneContent, len(current.Script.Scenes))
	for _, scene := range current.Script.Scenes {
		currentScenes[scene.SceneID] = scene
	}
	if len(currentScenes) != len(script.Scenes) {
		return nil, fmt.Errorf("%w: scene set changed, reload before preview", ErrConflict)
	}
	for _, nextScene := range script.Scenes {
		beforeScene, ok := currentScenes[nextScene.SceneID]
		if !ok {
			return nil, fmt.Errorf("%w: unknown scene %s", ErrConflict, nextScene.SceneID)
		}
		prefix := "scene." + nextScene.SceneID + "."
		add(prefix+"location_name", beforeScene.LocationName, strings.TrimSpace(nextScene.LocationName))
		add(prefix+"time_of_day", beforeScene.TimeOfDay, strings.TrimSpace(nextScene.TimeOfDay))
		add(prefix+"interior_exterior", beforeScene.InteriorExterior, strings.TrimSpace(nextScene.InteriorExterior))
		add(prefix+"scene_purpose", beforeScene.ScenePurpose, strings.TrimSpace(nextScene.ScenePurpose))
		add(prefix+"emotional_change", beforeScene.EmotionalChange, strings.TrimSpace(nextScene.EmotionalChange))
		add(prefix+"estimated_duration_seconds", beforeScene.EstimatedDurationSeconds, nextScene.EstimatedDurationSeconds)
		var beforeActions, afterActions any
		if err := json.Unmarshal(beforeScene.Actions, &beforeActions); err != nil {
			return nil, fmt.Errorf("decode current scene actions: %w", err)
		}
		if err := json.Unmarshal(nextScene.Actions, &afterActions); err != nil {
			return nil, fmt.Errorf("%w: scene actions must be valid JSON", ErrInvalidEpisodeContent)
		}
		add(prefix+"actions", beforeActions, afterActions)

		currentDialogues := make(map[string]EpisodeDialogueContent, len(beforeScene.Dialogues))
		for _, dialogue := range beforeScene.Dialogues {
			currentDialogues[dialogue.DialogueID] = dialogue
		}
		if len(currentDialogues) != len(nextScene.Dialogues) {
			return nil, fmt.Errorf("%w: dialogue set changed for scene %s", ErrConflict, nextScene.SceneID)
		}
		for _, nextDialogue := range nextScene.Dialogues {
			beforeDialogue, exists := currentDialogues[nextDialogue.DialogueID]
			if !exists {
				return nil, fmt.Errorf("%w: unknown dialogue %s", ErrConflict, nextDialogue.DialogueID)
			}
			dialoguePrefix := "dialogue." + nextDialogue.DialogueID + "."
			add(dialoguePrefix+"dialogue_type", beforeDialogue.DialogueType, nextDialogue.DialogueType)
			add(dialoguePrefix+"speaker_name", beforeDialogue.SpeakerName, strings.TrimSpace(nextDialogue.SpeakerName))
			add(dialoguePrefix+"text", beforeDialogue.Text, strings.TrimSpace(nextDialogue.Text))
			add(dialoguePrefix+"emotion", beforeDialogue.Emotion, strings.TrimSpace(nextDialogue.Emotion))
			add(dialoguePrefix+"performance_instruction", beforeDialogue.PerformanceInstruction,
				strings.TrimSpace(nextDialogue.PerformanceInstruction))
			add(dialoguePrefix+"estimated_duration_ms", beforeDialogue.EstimatedDurationMS,
				nextDialogue.EstimatedDurationMS)
		}
	}
	return changes, nil
}

func validateEpisodeContentChangePlanInput(input EpisodeContentChangePlanInput) error {
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
