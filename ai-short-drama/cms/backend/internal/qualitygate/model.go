package qualitygate

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	SchemaVersion     = "cross-layer-quality-gate.v1"
	FindingSchema     = "quality-gate-finding.v1"
	ChangePlanSchema  = "quality-gate-change-plan.v1"
	BenchmarkSchema   = "quality-gate-benchmark.v1"
	DefaultRuleset    = "cross-layer-rules.v1"
	DefaultPrompt     = "cross-layer-review.v1"
	DetectorRule      = "rule"
	DetectorModel     = "model"
	SeverityInfo      = "info"
	SeverityWarning   = "warning"
	SeverityMajor     = "major"
	SeverityBlocking  = "blocking"
	FindingOpen       = "open"
	FindingResolved   = "resolved"
	FindingOverridden = "overridden"
)

type Stage string

const (
	StageSourceIR       Stage = "source_ir"
	StageAdaptationPlan Stage = "adaptation_plan"
	StageEpisodeOutline Stage = "episode_outline"
	StageScript         Stage = "script"
	StageStoryboard     Stage = "storyboard"
	StageMedia          Stage = "media"
	StageEditTimeline   Stage = "edit_timeline"
	StageMaster         Stage = "master"
)

var StageOrder = []Stage{
	StageSourceIR, StageAdaptationPlan, StageEpisodeOutline, StageScript,
	StageStoryboard, StageMedia, StageEditTimeline, StageMaster,
}

type Dimension string

const (
	DimensionSourceFidelity      Dimension = "source_fidelity"
	DimensionCharacterContinuity Dimension = "character_continuity"
	DimensionCausality           Dimension = "causality"
	DimensionForeshadowing       Dimension = "foreshadowing"
	DimensionHooks               Dimension = "hooks"
	DimensionInformationDensity  Dimension = "information_density"
	DimensionDialogueVisual      Dimension = "dialogue_visual_consistency"
	DimensionActionCoverage      Dimension = "action_coverage"
	DimensionAVIdentity          Dimension = "av_sync_identity"
	DimensionEditIntegrity       Dimension = "edit_integrity"
	DimensionConstraint          Dimension = "constraint_compliance"
)

var dimensions = map[Dimension]struct{}{
	DimensionSourceFidelity: {}, DimensionCharacterContinuity: {}, DimensionCausality: {},
	DimensionForeshadowing: {}, DimensionHooks: {}, DimensionInformationDensity: {},
	DimensionDialogueVisual: {}, DimensionActionCoverage: {}, DimensionAVIdentity: {},
	DimensionEditIntegrity: {}, DimensionConstraint: {},
}

type Locator struct {
	Stage        Stage  `json:"stage"`
	ArtifactID   string `json:"artifact_id"`
	EntityType   string `json:"entity_type"`
	EntityID     string `json:"entity_id"`
	FieldPath    string `json:"field_path,omitempty"`
	SourceSpanID string `json:"source_span_id,omitempty"`
	StartMS      *int64 `json:"start_ms,omitempty"`
	EndMS        *int64 `json:"end_ms,omitempty"`
	Frame        *int64 `json:"frame,omitempty"`
}

type Evidence struct {
	Locator  Locator `json:"locator"`
	Observed string  `json:"observed"`
	Expected string  `json:"expected,omitempty"`
	Quote    string  `json:"quote,omitempty"`
}

type Finding struct {
	SchemaVersion  string          `json:"schema_version"`
	FindingID      string          `json:"finding_id"`
	DetectorType   string          `json:"detector_type"`
	Dimension      Dimension       `json:"dimension"`
	Code           string          `json:"code"`
	Severity       string          `json:"severity"`
	Message        string          `json:"message"`
	Evidence       []Evidence      `json:"evidence"`
	Locators       []Locator       `json:"locators"`
	Recommendation string          `json:"recommendation"`
	Status         string          `json:"status"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

type Snapshot struct {
	SchemaVersion string     `json:"schema_version"`
	ProjectID     string     `json:"project_id"`
	EpisodeID     string     `json:"episode_id"`
	MasterID      string     `json:"master_id,omitempty"`
	DurationMS    int64      `json:"duration_ms,omitempty"`
	Artifacts     []Artifact `json:"artifacts"`
}

type Artifact struct {
	Stage       Stage                  `json:"stage"`
	ArtifactID  string                 `json:"artifact_id"`
	Version     int                    `json:"version"`
	DurationMS  int64                  `json:"duration_ms,omitempty"`
	Facts       []Fact                 `json:"facts,omitempty"`
	Characters  []CharacterObservation `json:"characters,omitempty"`
	Events      []Event                `json:"events,omitempty"`
	Foreshadows []ForeshadowOccurrence `json:"foreshadows,omitempty"`
	Hooks       []Hook                 `json:"hooks,omitempty"`
	Reveals     []Reveal               `json:"reveals,omitempty"`
	Actions     []Action               `json:"actions,omitempty"`
	AVBindings  []AVBinding            `json:"av_bindings,omitempty"`
	Timeline    []TimelineItem         `json:"timeline,omitempty"`
	Signals     []MediaSignal          `json:"signals,omitempty"`
	Constraints []ConstraintCheck      `json:"constraints,omitempty"`
}

type Fact struct {
	Key            string  `json:"key"`
	Value          string  `json:"value"`
	Critical       bool    `json:"critical"`
	SourceSpanID   string  `json:"source_span_id,omitempty"`
	Quote          string  `json:"quote,omitempty"`
	RequiredStages []Stage `json:"required_stages,omitempty"`
}

type CharacterObservation struct {
	CharacterID    string            `json:"character_id"`
	Goal           string            `json:"goal,omitempty"`
	Motivation     string            `json:"motivation,omitempty"`
	Relationships  map[string]string `json:"relationships,omitempty"`
	State          map[string]string `json:"state,omitempty"`
	ChangeEventIDs []string          `json:"change_event_ids,omitempty"`
	Evidence       string            `json:"evidence,omitempty"`
}

type Event struct {
	EventID     string   `json:"event_id"`
	Order       int      `json:"order"`
	TimeMS      *int64   `json:"time_ms,omitempty"`
	CauseIDs    []string `json:"cause_ids,omitempty"`
	Description string   `json:"description,omitempty"`
}

type ForeshadowOccurrence struct {
	ThreadID       string  `json:"thread_id"`
	Kind           string  `json:"kind"` // planted, reinforced, revealed, resolved
	TimeMS         *int64  `json:"time_ms,omitempty"`
	SourceSpanID   string  `json:"source_span_id,omitempty"`
	RequiredStages []Stage `json:"required_stages,omitempty"`
	Evidence       string  `json:"evidence,omitempty"`
}

type Hook struct {
	HookID         string  `json:"hook_id"`
	Kind           string  `json:"kind"` // opening_3s, first_30s, ending
	TimeMS         int64   `json:"time_ms"`
	Content        string  `json:"content,omitempty"`
	SourceSpanID   string  `json:"source_span_id,omitempty"`
	RequiredStages []Stage `json:"required_stages,omitempty"`
}

type Reveal struct {
	RevealID string `json:"reveal_id"`
	Key      string `json:"key"`
	TimeMS   int64  `json:"time_ms"`
	Content  string `json:"content,omitempty"`
}

type Action struct {
	ActionID        string   `json:"action_id"`
	Description     string   `json:"description,omitempty"`
	Required        bool     `json:"required"`
	CoversActionIDs []string `json:"covers_action_ids,omitempty"`
	TimeMS          *int64   `json:"time_ms,omitempty"`
}

type AVBinding struct {
	BindingID           string            `json:"binding_id"`
	DialogueID          string            `json:"dialogue_id,omitempty"`
	ShotID              string            `json:"shot_id,omitempty"`
	StartMS             int64             `json:"start_ms"`
	EndMS               int64             `json:"end_ms"`
	SpeakerCharacterID  string            `json:"speaker_character_id,omitempty"`
	SubtitleCharacterID string            `json:"subtitle_character_id,omitempty"`
	LipCharacterID      string            `json:"lip_character_id,omitempty"`
	VisibleCharacterIDs []string          `json:"visible_character_ids,omitempty"`
	DialogueText        string            `json:"dialogue_text,omitempty"`
	SubtitleText        string            `json:"subtitle_text,omitempty"`
	SpokenAssertions    map[string]string `json:"spoken_assertions,omitempty"`
	VisualAssertions    map[string]string `json:"visual_assertions,omitempty"`
}

type TimelineItem struct {
	TimelineItemID string `json:"timeline_item_id"`
	TrackType      string `json:"track_type"` // video, audio, subtitle
	EntityType     string `json:"entity_type"`
	EntityID       string `json:"entity_id"`
	StartMS        int64  `json:"start_ms"`
	EndMS          int64  `json:"end_ms"`
}

type MediaSignal struct {
	SignalID string `json:"signal_id"`
	Kind     string `json:"kind"` // black, silence, duplicate, out_of_bounds
	StartMS  int64  `json:"start_ms"`
	EndMS    int64  `json:"end_ms"`
	Evidence string `json:"evidence,omitempty"`
}

type ConstraintCheck struct {
	ConstraintID   string `json:"constraint_id"`
	Kind           string `json:"kind"` // template, performance_bible, continuity
	ReferenceID    string `json:"reference_id"`
	Compliant      bool   `json:"compliant"`
	Severity       string `json:"severity,omitempty"`
	Observed       string `json:"observed"`
	Expected       string `json:"expected"`
	Recommendation string `json:"recommendation,omitempty"`
}

type Run struct {
	SchemaVersion       string    `json:"schema_version"`
	GateRunID           string    `json:"gate_run_id"`
	RulesetVersion      string    `json:"ruleset_version"`
	RulesConfig         Config    `json:"rules_config"`
	ProjectID           string    `json:"project_id"`
	EpisodeID           string    `json:"episode_id"`
	MasterID            string    `json:"master_id,omitempty"`
	ModelReviewRequired bool      `json:"model_review_required"`
	Findings            []Finding `json:"findings"`
	BlockingCount       int       `json:"blocking_count"`
	RuleScore           float64   `json:"rule_score"`
}

type ModelReview struct {
	SchemaVersion string    `json:"schema_version"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	PromptVersion string    `json:"prompt_version"`
	Findings      []Finding `json:"findings"`
}

func (snapshot Snapshot) Validate() error {
	if snapshot.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %s", SchemaVersion)
	}
	if strings.TrimSpace(snapshot.ProjectID) == "" || strings.TrimSpace(snapshot.EpisodeID) == "" {
		return errors.New("project_id and episode_id are required")
	}
	if len(snapshot.Artifacts) == 0 {
		return errors.New("at least one artifact is required")
	}
	if strings.TrimSpace(snapshot.MasterID) != "" && snapshot.DurationMS <= 0 {
		return errors.New("duration_ms must be positive when master_id is present")
	}
	seen := map[Stage]bool{}
	artifactIDs := map[string]bool{}
	for _, artifact := range snapshot.Artifacts {
		if !validStage(artifact.Stage) || strings.TrimSpace(artifact.ArtifactID) == "" || artifact.Version < 1 {
			return fmt.Errorf("invalid artifact stage, id, or version")
		}
		if seen[artifact.Stage] {
			return fmt.Errorf("stage %s occurs more than once", artifact.Stage)
		}
		if artifactIDs[artifact.ArtifactID] {
			return fmt.Errorf("artifact_id %s occurs more than once", artifact.ArtifactID)
		}
		seen[artifact.Stage] = true
		artifactIDs[artifact.ArtifactID] = true
		for _, binding := range artifact.AVBindings {
			if binding.EndMS <= binding.StartMS {
				return fmt.Errorf("AV binding %s has an invalid time range", binding.BindingID)
			}
		}
		for _, item := range artifact.Timeline {
			if item.EndMS <= item.StartMS {
				return fmt.Errorf("timeline item %s has an invalid time range", item.TimelineItemID)
			}
		}
	}
	return nil
}

func ValidateModelReview(review ModelReview) error {
	if review.SchemaVersion != SchemaVersion || strings.TrimSpace(review.Provider) == "" ||
		strings.TrimSpace(review.Model) == "" || strings.TrimSpace(review.PromptVersion) == "" {
		return errors.New("model review schema, provider, model and prompt_version are required")
	}
	seen := map[string]bool{}
	for index := range review.Findings {
		review.Findings[index].DetectorType = DetectorModel
		if seen[review.Findings[index].FindingID] {
			return fmt.Errorf("model finding_id %s occurs more than once", review.Findings[index].FindingID)
		}
		seen[review.Findings[index].FindingID] = true
		if err := ValidateFinding(review.Findings[index]); err != nil {
			return fmt.Errorf("model finding %d: %w", index, err)
		}
	}
	return nil
}

func ValidateModelReviewAgainstSnapshot(review ModelReview, snapshot Snapshot) error {
	if err := ValidateModelReview(review); err != nil {
		return err
	}
	artifacts := map[string]Stage{}
	for _, artifact := range snapshot.Artifacts {
		artifacts[artifact.ArtifactID] = artifact.Stage
	}
	for index, finding := range review.Findings {
		if err := validateFindingArtifactGrounding(finding, artifacts); err != nil {
			return fmt.Errorf("model finding %d: %w", index, err)
		}
	}
	return nil
}

func ValidateFindingAgainstSnapshot(finding Finding, snapshot Snapshot) error {
	if err := ValidateFinding(finding); err != nil {
		return err
	}
	artifacts := map[string]Stage{}
	for _, artifact := range snapshot.Artifacts {
		artifacts[artifact.ArtifactID] = artifact.Stage
	}
	return validateFindingArtifactGrounding(finding, artifacts)
}

func validateFindingArtifactGrounding(finding Finding, artifacts map[string]Stage) error {
	for _, locator := range append(append([]Locator(nil), finding.Locators...), evidenceLocators(finding.Evidence)...) {
		stage, exists := artifacts[locator.ArtifactID]
		if !exists || stage != locator.Stage {
			return fmt.Errorf("cites artifact %s outside the reviewed snapshot", locator.ArtifactID)
		}
	}
	return nil
}

func evidenceLocators(evidence []Evidence) []Locator {
	result := make([]Locator, 0, len(evidence))
	for _, item := range evidence {
		result = append(result, item.Locator)
	}
	return result
}

func ValidateFinding(finding Finding) error {
	if finding.SchemaVersion != FindingSchema {
		return fmt.Errorf("schema_version must be %s", FindingSchema)
	}
	if finding.DetectorType != DetectorRule && finding.DetectorType != DetectorModel {
		return errors.New("detector_type must be rule or model")
	}
	if _, ok := dimensions[finding.Dimension]; !ok {
		return errors.New("unknown dimension")
	}
	if strings.TrimSpace(finding.FindingID) == "" || !validSeverity(finding.Severity) || strings.TrimSpace(finding.Code) == "" ||
		strings.TrimSpace(finding.Message) == "" || strings.TrimSpace(finding.Recommendation) == "" {
		return errors.New("finding_id, code, severity, message and recommendation are required")
	}
	if len(finding.Evidence) == 0 || len(finding.Locators) == 0 {
		return errors.New("at least one evidence item and locator are required")
	}
	for _, evidence := range finding.Evidence {
		if strings.TrimSpace(evidence.Observed) == "" || validateLocator(evidence.Locator) != nil {
			return errors.New("every evidence item requires an observed value and valid locator")
		}
	}
	for _, locator := range finding.Locators {
		if err := validateLocator(locator); err != nil {
			return err
		}
	}
	if finding.Status != FindingOpen && finding.Status != FindingResolved && finding.Status != FindingOverridden {
		return errors.New("invalid finding status")
	}
	if len(finding.Metadata) > 0 {
		var metadata map[string]any
		if err := json.Unmarshal(finding.Metadata, &metadata); err != nil || metadata == nil {
			return errors.New("metadata must be a JSON object")
		}
	}
	return nil
}

func validateLocator(locator Locator) error {
	if !validStage(locator.Stage) || strings.TrimSpace(locator.ArtifactID) == "" ||
		strings.TrimSpace(locator.EntityType) == "" || strings.TrimSpace(locator.EntityID) == "" {
		return errors.New("locator requires stage, artifact_id, entity_type and entity_id")
	}
	if locator.StartMS != nil && locator.EndMS != nil && *locator.EndMS <= *locator.StartMS {
		return errors.New("locator end_ms must be greater than start_ms")
	}
	return nil
}

func validStage(stage Stage) bool {
	for _, candidate := range StageOrder {
		if stage == candidate {
			return true
		}
	}
	return false
}

func validSeverity(severity string) bool {
	return severity == SeverityInfo || severity == SeverityWarning ||
		severity == SeverityMajor || severity == SeverityBlocking
}

func sortFindings(findings []Finding) {
	weight := map[string]int{SeverityBlocking: 0, SeverityMajor: 1, SeverityWarning: 2, SeverityInfo: 3}
	sort.SliceStable(findings, func(i, j int) bool {
		if weight[findings[i].Severity] != weight[findings[j].Severity] {
			return weight[findings[i].Severity] < weight[findings[j].Severity]
		}
		if findings[i].Dimension != findings[j].Dimension {
			return findings[i].Dimension < findings[j].Dimension
		}
		return findings[i].FindingID < findings[j].FindingID
	})
}
