package performancecontinuity

import (
	"encoding/json"
	"errors"
)

const (
	PerformanceBibleSchema = "performance-bible.v1"
	ContinuityLedgerSchema = "continuity-ledger.v1"
	VisualQCSchema         = "visual-qc-report.v1"
	ShotHandoffSchema      = "shot-handoff.v1"
)

var ErrInvalidInput = errors.New("invalid performance or continuity input")

type SpeechProfile struct {
	RateWPM       int      `json:"rate_wpm"`
	Pitch         string   `json:"pitch"`
	PauseHabits   []string `json:"pause_habits"`
	Catchphrases  []string `json:"catchphrases"`
	VoiceIdentity string   `json:"voice_identity"`
}

type ActingProfile struct {
	EmotionStyle   map[string]string `json:"emotion_style"`
	BodyHabits     []string          `json:"body_habits"`
	ProhibitedActs []string          `json:"prohibited_acts"`
}

type AppearanceProfile struct {
	FaceShape   string `json:"face_shape"`
	Hairstyle   string `json:"hairstyle"`
	ApparentAge string `json:"apparent_age"`
	BodyType    string `json:"body_type"`
	Posture     string `json:"posture"`
}

type StageState struct {
	StageKey      string            `json:"stage_key"`
	EpisodeFrom   int               `json:"episode_from"`
	EpisodeTo     *int              `json:"episode_to,omitempty"`
	Costume       string            `json:"costume"`
	Scars         []string          `json:"scars"`
	Props         []string          `json:"props"`
	Psychology    string            `json:"psychology"`
	Relationships map[string]string `json:"relationships"`
}

type PerformanceBible struct {
	SchemaVersion    string                     `json:"schema_version"`
	BibleID          string                     `json:"bible_id"`
	ProjectID        string                     `json:"project_id"`
	CharacterID      string                     `json:"character_id"`
	CharacterVersion string                     `json:"character_version"`
	Version          int                        `json:"version"`
	Status           string                     `json:"status"`
	Speech           SpeechProfile              `json:"speech"`
	Acting           ActingProfile              `json:"acting"`
	RelationalVoices map[string]SpeechProfile   `json:"relational_voices"`
	Appearance       AppearanceProfile          `json:"appearance"`
	StageStates      []StageState               `json:"stage_states"`
	LockedFields     []string                   `json:"locked_fields"`
	AllowedFields    []string                   `json:"allowed_fields"`
	ChangeReasons    map[string]string          `json:"change_reasons"`
	SourceRefs       map[string]string          `json:"source_refs"`
	Metadata         map[string]json.RawMessage `json:"metadata,omitempty"`
}

type CharacterState struct {
	Position      string            `json:"position"`
	ScreenX       float64           `json:"screen_x"`
	Facing        string            `json:"facing"`
	GazeTarget    string            `json:"gaze_target"`
	Posture       string            `json:"posture"`
	Costume       string            `json:"costume"`
	Hairstyle     string            `json:"hairstyle"`
	Scars         []string          `json:"scars"`
	HeldProps     []string          `json:"held_props"`
	Knows         []string          `json:"knows"`
	DoesNotKnow   []string          `json:"does_not_know"`
	Relationships map[string]string `json:"relationships"`
	Emotion       string            `json:"emotion"`
	PhysicalState string            `json:"physical_state"`
	IdentityRef   string            `json:"identity_ref"`
}

type PropState struct {
	OwnerCharacterID string `json:"owner_character_id"`
	Position         string `json:"position"`
	Condition        string `json:"condition"`
	Visible          bool   `json:"visible"`
}

type EnvironmentState struct {
	LocationID string `json:"location_id"`
	Time       string `json:"time"`
	Weather    string `json:"weather"`
	Lighting   string `json:"lighting"`
}

type State struct {
	Characters  map[string]CharacterState `json:"characters"`
	Props       map[string]PropState      `json:"props"`
	Environment EnvironmentState          `json:"environment"`
	Axis        string                    `json:"axis"`
}

type LedgerEntry struct {
	SchemaVersion string       `json:"schema_version"`
	EntryID       string       `json:"entry_id"`
	ProjectID     string       `json:"project_id"`
	EpisodeID     string       `json:"episode_id"`
	EpisodeNumber int          `json:"episode_number"`
	SceneID       string       `json:"scene_id"`
	ShotID        string       `json:"shot_id"`
	ShotOrder     int          `json:"shot_order"`
	InputState    State        `json:"input_state"`
	OutputState   State        `json:"output_state"`
	InheritedFrom string       `json:"inherited_from,omitempty"`
	Diagnostics   []Diagnostic `json:"diagnostics,omitempty"`
}

type Diagnostic struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Path       string `json:"path"`
	Message    string `json:"message"`
	Expected   any    `json:"expected,omitempty"`
	Actual     any    `json:"actual,omitempty"`
	Suggestion string `json:"suggestion"`
}

type FrameLocator struct {
	EpisodeID  string `json:"episode_id"`
	SceneID    string `json:"scene_id"`
	ShotID     string `json:"shot_id"`
	TimecodeMS int64  `json:"timecode_ms"`
	Frame      int    `json:"frame"`
}

type FrameObservation struct {
	Locator         FrameLocator        `json:"locator"`
	CharacterIDs    []string            `json:"character_ids"`
	IdentityScores  map[string]float64  `json:"identity_scores"`
	Ages            map[string]string   `json:"ages"`
	Hairstyles      map[string]string   `json:"hairstyles"`
	Costumes        map[string]string   `json:"costumes"`
	Scars           map[string][]string `json:"scars"`
	Props           map[string]bool     `json:"props"`
	Positions       map[string]float64  `json:"positions"`
	GazeDirections  map[string]string   `json:"gaze_directions"`
	MotionDirection string              `json:"motion_direction"`
	Axis            string              `json:"axis"`
	BackgroundID    string              `json:"background_id"`
	Defects         []string            `json:"defects"`
	SubtitleBoxes   []SubtitleBox       `json:"subtitle_boxes"`
	FlickerScore    float64             `json:"flicker_score"`
	MeltScore       float64             `json:"melt_score"`
	Pose            map[string]string   `json:"pose"`
	ShotSize        string              `json:"shot_size"`
	Composition     string              `json:"composition"`
}

type SubtitleBox struct {
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	OverlapsFace bool    `json:"overlaps_face"`
}

type QCIssue struct {
	IssueID        string       `json:"issue_id"`
	Category       string       `json:"category"`
	Severity       string       `json:"severity"`
	Locator        FrameLocator `json:"locator"`
	Evidence       string       `json:"evidence"`
	Recommendation string       `json:"recommendation"`
	Status         string       `json:"status"`
	LocalRedo      LocalRedo    `json:"local_redo"`
}

type LocalRedo struct {
	EntityType    string   `json:"entity_type"`
	EntityID      string   `json:"entity_id"`
	AllowedFields []string `json:"allowed_fields"`
	StartMS       int64    `json:"start_ms"`
	EndMS         int64    `json:"end_ms"`
}

type ShotHandoff struct {
	SchemaVersion         string            `json:"schema_version"`
	HandoffID             string            `json:"handoff_id"`
	ProjectID             string            `json:"project_id"`
	EpisodeID             string            `json:"episode_id"`
	FromShotID            string            `json:"from_shot_id"`
	ToShotID              string            `json:"to_shot_id"`
	TargetTailFrameRef    string            `json:"target_tail_frame_ref"`
	ReferenceHeadFrameRef string            `json:"reference_head_frame_ref"`
	PoseConstraints       map[string]string `json:"pose_constraints"`
	GazeConstraint        string            `json:"gaze_constraint"`
	MotionDirection       string            `json:"motion_direction"`
	FromActionPhase       string            `json:"from_action_phase"`
	ToActionPhase         string            `json:"to_action_phase"`
	ShotSizeConstraint    string            `json:"shot_size_constraint"`
	CompositionConstraint string            `json:"composition_constraint"`
	Version               int               `json:"version"`
}

type GenerationRequest struct {
	ArtifactType         string            `json:"artifact_type"`
	ProjectID            string            `json:"project_id"`
	EpisodeID            string            `json:"episode_id"`
	SceneID              string            `json:"scene_id"`
	ShotID               string            `json:"shot_id"`
	CharacterIDs         []string          `json:"character_ids"`
	PerformanceBibleRefs map[string]string `json:"performance_bible_refs"`
	Ledger               *LedgerEntry      `json:"ledger"`
	Handoff              *ShotHandoff      `json:"handoff,omitempty"`
	BasePrompt           string            `json:"base_prompt"`
}

type GenerationContext struct {
	Allowed              bool              `json:"allowed"`
	PerformanceBibleRefs map[string]string `json:"performance_bible_refs"`
	ContinuityEntryID    string            `json:"continuity_entry_id"`
	HandoffID            string            `json:"handoff_id,omitempty"`
	ResolvedPrompt       string            `json:"resolved_prompt,omitempty"`
	Diagnostics          []Diagnostic      `json:"diagnostics"`
}
