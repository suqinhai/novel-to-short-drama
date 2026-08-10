package shoteditor

import "encoding/json"

const (
	OperationSplit   = "split"
	OperationMerge   = "merge"
	OperationReorder = "reorder"
	OperationUpdate  = "update"
	OperationRestore = "restore"
)

type Shot struct {
	ShotID             string         `json:"shot_id"`
	StoryboardID       string         `json:"storyboard_id"`
	ProjectID          string         `json:"project_id"`
	EpisodeID          string         `json:"episode_id"`
	SceneID            string         `json:"scene_id"`
	ShotNumber         int            `json:"shot_number"`
	ShotOrder          int            `json:"shot_order"`
	DurationSeconds    float64        `json:"duration_seconds"`
	ShotSize           string         `json:"shot_size"`
	CameraAngle        string         `json:"camera_angle"`
	CameraMotion       string         `json:"camera_motion"`
	Composition        string         `json:"composition"`
	CharacterIDs       []string       `json:"character_ids"`
	LocationID         string         `json:"location_id"`
	ActionDescription  string         `json:"action_description"`
	FacialExpression   string         `json:"facial_expression"`
	DialogueIDs        []string       `json:"dialogue_ids"`
	SubtitleText       string         `json:"subtitle_text"`
	NarrationText      string         `json:"narration_text"`
	Lighting           string         `json:"lighting"`
	Atmosphere         string         `json:"atmosphere"`
	SoundEffectHint    string         `json:"sound_effect_hint"`
	BGMHint            string         `json:"bgm_hint"`
	TransitionType     string         `json:"transition_type"`
	VisualPromptBase   string         `json:"visual_prompt_base"`
	VideoPromptBase    string         `json:"video_prompt_base"`
	NegativePromptBase string         `json:"negative_prompt_base"`
	ContinuityNotes    map[string]any `json:"continuity_notes"`
	SourceSceneData    map[string]any `json:"source_scene_data"`
	Status             string         `json:"status"`
	GenerationVersion  int            `json:"generation_version"`
	Version            int            `json:"version"`
	LineageRootShotID  string         `json:"lineage_root_shot_id"`
	HeadState          map[string]any `json:"head_state"`
	TailState          map[string]any `json:"tail_state"`
	Performance        map[string]any `json:"performance"`
	ActionPhase        map[string]any `json:"action_phase"`
	Axis               string         `json:"axis"`
	CoverageRole       string         `json:"coverage_role"`
	CoverageGroup      string         `json:"coverage_group"`
	CoverageSide       string         `json:"coverage_side"`
	ThumbnailURL       string         `json:"thumbnail_url,omitempty"`
	HeadFrameRef       string         `json:"head_frame_ref,omitempty"`
	TailFrameRef       string         `json:"tail_frame_ref,omitempty"`
}

type Request struct {
	Operation               string                 `json:"operation"`
	BaseSequenceVersion     int                    `json:"base_sequence_version,omitempty"`
	ShotID                  string                 `json:"shot_id,omitempty"`
	ShotIDs                 []string               `json:"shot_ids,omitempty"`
	OrderedShotIDs          []string               `json:"ordered_shot_ids,omitempty"`
	Shots                   []Shot                 `json:"shots,omitempty"`
	Patch                   map[string]any         `json:"patch,omitempty"`
	SourceSequenceVersionID string                 `json:"source_sequence_version_id,omitempty"`
	RequiredCoverage        []string               `json:"required_coverage,omitempty"`
	RequestedBy             *string                `json:"requested_by,omitempty"`
	NewShotIDs              []string               `json:"-"`
	RestoreSnapshot         []Shot                 `json:"-"`
	DialogueDurationsMS     map[string]int64       `json:"-"`
	Metadata                map[string]interface{} `json:"metadata,omitempty"`
}

type Conflict struct {
	Code       string         `json:"code"`
	Severity   string         `json:"severity"`
	Message    string         `json:"message"`
	FromShotID string         `json:"from_shot_id,omitempty"`
	ToShotID   string         `json:"to_shot_id,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

type CoverageCheck struct {
	SceneID  string   `json:"scene_id"`
	Kind     string   `json:"kind"`
	Label    string   `json:"label"`
	Passed   bool     `json:"passed"`
	ShotIDs  []string `json:"shot_ids"`
	Required bool     `json:"required"`
}

type Handoff struct {
	FromShotID            string         `json:"from_shot_id"`
	ToShotID              string         `json:"to_shot_id"`
	TargetTailFrameRef    string         `json:"target_tail_frame_ref,omitempty"`
	ReferenceHeadFrame    string         `json:"reference_head_frame_ref,omitempty"`
	FromActionPhase       string         `json:"from_action_phase,omitempty"`
	ToActionPhase         string         `json:"to_action_phase,omitempty"`
	MotionDirection       string         `json:"motion_direction,omitempty"`
	GazeConstraint        string         `json:"gaze_constraint,omitempty"`
	ShotSizeConstraint    string         `json:"shot_size_constraint,omitempty"`
	CompositionConstraint string         `json:"composition_constraint,omitempty"`
	PoseConstraints       map[string]any `json:"pose_constraints"`
	Status                string         `json:"status"`
	Diagnostics           []Conflict     `json:"diagnostics"`
}

type Preview struct {
	Operation  string          `json:"operation"`
	Shots      []Shot          `json:"shots"`
	Conflicts  []Conflict      `json:"continuity_conflicts"`
	Coverage   []CoverageCheck `json:"coverage_report"`
	Handoffs   []Handoff       `json:"handoff_preview"`
	ChangedIDs []string        `json:"changed_shot_ids"`
	RetiredIDs []string        `json:"retired_shot_ids"`
	CreatedIDs []string        `json:"created_shot_ids"`
}

func CloneShots(value []Shot) []Shot {
	raw, _ := json.Marshal(value)
	var result []Shot
	_ = json.Unmarshal(raw, &result)
	return result
}
