package candidategeneration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	GeneratorVersion = "candidate-provider-v2"
	PromptVersion    = "multi-candidate-v2"
)

var (
	ErrProviderUnavailable = errors.New("candidate provider is unavailable")
	ErrProviderFailed      = errors.New("candidate provider failed")
	ErrInvalidProviderData = errors.New("candidate provider returned invalid data")
)

var AllowedTargetTypes = map[string]bool{
	"story_arc": true, "episode": true, "scene": true, "storyboard": true,
	"image": true, "video": true,
}

var AllowedComponents = map[string]bool{
	"episode_plan": true, "opening": true, "conflict": true, "climax": true, "ending_hook": true,
	"dialogue": true, "action": true, "narration": true,
	"composition": true, "shot_size": true, "camera_movement": true, "performance": true, "transition": true,
	"key_image": true, "video_shot": true,
}

type FrozenInput struct {
	SchemaVersion  string          `json:"schema_version"`
	ResolutionID   string          `json:"resolution_id"`
	ContextHash    string          `json:"context_hash"`
	ResolutionHash string          `json:"resolution_hash"`
	FrozenHash     string          `json:"frozen_hash"`
	Stage          string          `json:"stage"`
	EpisodeID      string          `json:"episode_id,omitempty"`
	Resolution     json.RawMessage `json:"resolution"`
	TargetContext  json.RawMessage `json:"target_context"`
}

type Request struct {
	TargetType           string   `json:"target_type"`
	TargetID             string   `json:"target_id"`
	ComponentTypes       []string `json:"component_types"`
	CandidateCount       int      `json:"candidate_count"`
	DifferenceDirections []string `json:"difference_directions"`
	MustPreserve         []string `json:"must_preserve"`
	AllowedChanges       []string `json:"allowed_changes"`
	// Model is retained only for decoding old records. New requests must name
	// both the generator and the independent reviewer explicitly.
	Model                string          `json:"model,omitempty"`
	GeneratorProvider    string          `json:"generator_provider,omitempty"`
	GeneratorModel       string          `json:"generator_model,omitempty"`
	ReviewerProvider     string          `json:"reviewer_provider,omitempty"`
	ReviewerModel        string          `json:"reviewer_model,omitempty"`
	BlindReview          bool            `json:"blind_review"`
	PromptVersion        string          `json:"prompt_version"`
	RandomSeed           int64           `json:"random_seed"`
	GenerationParameters json.RawMessage `json:"generation_parameters"`
	BaseContent          json.RawMessage `json:"base_content,omitempty"`
	BaseDurationSeconds  int             `json:"base_duration_seconds,omitempty"`
	FrozenInput          FrozenInput     `json:"frozen_input"`
}

type Component struct {
	Key     string `json:"key"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type Evidence struct {
	SourceKind string `json:"source_kind"`
	SourceID   string `json:"source_id"`
	Path       string `json:"path"`
	Quote      string `json:"quote,omitempty"`
	Reason     string `json:"reason"`
}

type Deduction struct {
	Dimension string   `json:"dimension"`
	Penalty   float64  `json:"penalty"`
	Reason    string   `json:"reason"`
	Location  Evidence `json:"location"`
}

type DimensionScore struct {
	Dimension  string      `json:"dimension"`
	Score      float64     `json:"score"`
	Evidence   []Evidence  `json:"evidence"`
	Deductions []Deduction `json:"deductions"`
}

type Score struct {
	TotalScore               float64          `json:"total_score"`
	Fidelity                 float64          `json:"fidelity"`
	Causality                float64          `json:"causality"`
	CharacterConsistency     float64          `json:"character_consistency"`
	Hook                     float64          `json:"hook"`
	Pacing                   float64          `json:"pacing"`
	Filmability              float64          `json:"filmability"`
	Continuity               float64          `json:"continuity"`
	EstimatedDurationSeconds int              `json:"estimated_duration_seconds"`
	ModificationRisk         float64          `json:"modification_risk"`
	Dimensions               []DimensionScore `json:"dimensions"`
	RecommendationReasons    []string         `json:"recommendation_reasons"`
	DeductionReasons         []string         `json:"deduction_reasons"`
	ReviewerProvider         string           `json:"reviewer_provider"`
	ReviewerModel            string           `json:"reviewer_model"`
}

type Candidate struct {
	Ordinal              int             `json:"ordinal"`
	Label                string          `json:"label"`
	DifferenceDirection  string          `json:"difference_direction"`
	Components           []Component     `json:"components"`
	Content              map[string]any  `json:"content"`
	Score                Score           `json:"score"`
	Provider             string          `json:"provider"`
	Model                string          `json:"model"`
	PromptVersion        string          `json:"prompt_version"`
	RandomSeed           int64           `json:"random_seed"`
	GenerationParameters json.RawMessage `json:"generation_parameters"`
	StructuredDiff       []DiffEntry     `json:"structured_diff"`
}

type CandidateDraft struct {
	Components []Component    `json:"components"`
	Content    map[string]any `json:"content"`
}

type GenerationInput struct {
	Request             Request
	Ordinal             int
	DifferenceDirection string
	Seed                int64
}

type ReviewInput struct {
	Request             Request
	Ordinal             int
	DifferenceDirection string
	Candidate           CandidateDraft
	HideGenerator       bool
}

// ExecutionRecord is an append-only audit fact emitted by the orchestration
// layer. It records generation and evaluation independently, including failed
// and invalid provider responses for which no candidate may be persisted.
type ExecutionRecord struct {
	ExecutionType string    `json:"execution_type"`
	Ordinal       int       `json:"ordinal"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	FailureReason string    `json:"failure_reason,omitempty"`
	RetryCount    int       `json:"retry_count"`
	Attempt       int       `json:"attempt"`
	Blind         bool      `json:"blind"`
}

// CandidateProvider is implemented independently by text, image and video generators.
// A provider must either return a real candidate or an error; fallback is an orchestration decision
// and this package deliberately never substitutes deterministic_mock.
type CandidateProvider interface {
	Name() string
	MediaKind() string
	Generate(context.Context, GenerationInput) (CandidateDraft, error)
}

// CandidateReviewer is separate from CandidateProvider so generation credentials/models never
// implicitly become scoring credentials/models.
type CandidateReviewer interface {
	Name() string
	Review(context.Context, ReviewInput) (Score, error)
}

type Registry struct {
	providers map[string]CandidateProvider
	reviewers map[string]CandidateReviewer
}

func NewRegistry(providers []CandidateProvider, reviewers []CandidateReviewer) *Registry {
	result := &Registry{providers: map[string]CandidateProvider{}, reviewers: map[string]CandidateReviewer{}}
	for _, provider := range providers {
		_ = result.RegisterProvider(provider)
	}
	for _, reviewer := range reviewers {
		_ = result.RegisterReviewer(reviewer)
	}
	return result
}

func (r *Registry) RegisterProvider(provider CandidateProvider) error {
	if r == nil || provider == nil || strings.TrimSpace(provider.Name()) == "" || strings.TrimSpace(provider.MediaKind()) == "" {
		return fmt.Errorf("invalid candidate provider")
	}
	if r.providers == nil {
		r.providers = map[string]CandidateProvider{}
	}
	r.providers[provider.Name()] = provider
	return nil
}

func (r *Registry) RegisterReviewer(reviewer CandidateReviewer) error {
	if r == nil || reviewer == nil || strings.TrimSpace(reviewer.Name()) == "" {
		return fmt.Errorf("invalid candidate reviewer")
	}
	if r.reviewers == nil {
		r.reviewers = map[string]CandidateReviewer{}
	}
	r.reviewers[reviewer.Name()] = reviewer
	return nil
}

func (r *Registry) GenerateAndReview(ctx context.Context, request Request) ([]Candidate, error) {
	candidates, _, err := r.GenerateAndReviewAudited(ctx, request)
	return candidates, err
}

func (r *Registry) GenerateAndReviewAudited(ctx context.Context, request Request) ([]Candidate, []ExecutionRecord, error) {
	normalizeRequest(&request)
	if err := ValidateRequest(request); err != nil {
		return nil, nil, err
	}
	provider := r.providers[request.GeneratorProvider]
	if provider == nil || (provider.MediaKind() != "any" && provider.MediaKind() != targetMediaKind(request.TargetType)) {
		now := time.Now().UTC()
		reason := fmt.Sprintf("generator %q does not support %s", request.GeneratorProvider, request.TargetType)
		return nil, []ExecutionRecord{{ExecutionType: "generation", Ordinal: 1, Status: "failed",
				StartedAt: now, CompletedAt: now, Provider: request.GeneratorProvider,
				Model: request.GeneratorModel, FailureReason: reason, Attempt: 1}},
			fmt.Errorf("%w: %s", ErrProviderUnavailable, reason)
	}
	reviewer := r.reviewers[request.ReviewerProvider]
	if reviewer == nil {
		now := time.Now().UTC()
		reason := fmt.Sprintf("reviewer %q is unavailable", request.ReviewerProvider)
		return nil, []ExecutionRecord{{ExecutionType: "evaluation", Ordinal: 1, Status: "failed",
				StartedAt: now, CompletedAt: now, Provider: request.ReviewerProvider,
				Model: request.ReviewerModel, FailureReason: reason, Attempt: 1, Blind: request.BlindReview}},
			fmt.Errorf("%w: %s", ErrProviderUnavailable, reason)
	}
	candidates := make([]Candidate, 0, request.CandidateCount)
	executions := make([]ExecutionRecord, 0, request.CandidateCount*2)
	contentHashes := map[string]bool{}
	for index := 0; index < request.CandidateCount; index++ {
		direction := request.DifferenceDirections[index%len(request.DifferenceDirections)]
		seed := request.RandomSeed + int64(index)
		generation := ExecutionRecord{ExecutionType: "generation", Ordinal: index + 1, Status: "running",
			StartedAt: time.Now().UTC(), Provider: provider.Name(), Model: request.GeneratorModel, Attempt: 1}
		draft, err := provider.Generate(ctx, GenerationInput{Request: request, Ordinal: index + 1, DifferenceDirection: direction, Seed: seed})
		if err != nil {
			generation.Status, generation.CompletedAt, generation.FailureReason = "failed", time.Now().UTC(), err.Error()
			executions = append(executions, generation)
			return nil, executions, fmt.Errorf("%w: generator %s candidate %d: %v", ErrProviderFailed, provider.Name(), index+1, err)
		}
		if err := validateDraft(request, draft); err != nil {
			generation.Status, generation.CompletedAt, generation.FailureReason = "invalid", time.Now().UTC(), err.Error()
			executions = append(executions, generation)
			return nil, executions, fmt.Errorf("%w: candidate %d: %v", ErrInvalidProviderData, index+1, err)
		}
		substantive := map[string]any{"components": draft.Components}
		if media, ok := draft.Content["media"]; ok {
			substantive["media"] = media
		}
		canonical, _ := json.Marshal(substantive)
		digest := sha256.Sum256(canonical)
		hash := hex.EncodeToString(digest[:])
		if contentHashes[hash] {
			generation.Status, generation.CompletedAt, generation.FailureReason = "invalid", time.Now().UTC(), "duplicate substantive content"
			executions = append(executions, generation)
			return nil, executions, fmt.Errorf("%w: difference direction %q produced duplicate content", ErrInvalidProviderData, direction)
		}
		contentHashes[hash] = true
		generation.Status, generation.CompletedAt = "succeeded", time.Now().UTC()
		executions = append(executions, generation)
		evaluation := ExecutionRecord{ExecutionType: "evaluation", Ordinal: index + 1, Status: "running",
			StartedAt: time.Now().UTC(), Provider: reviewer.Name(), Model: request.ReviewerModel, Attempt: 1, Blind: request.BlindReview}
		score, err := reviewer.Review(ctx, ReviewInput{Request: request, Ordinal: index + 1,
			DifferenceDirection: direction, Candidate: draft, HideGenerator: request.BlindReview})
		if err != nil {
			evaluation.Status, evaluation.CompletedAt, evaluation.FailureReason = "failed", time.Now().UTC(), err.Error()
			executions = append(executions, evaluation)
			return nil, executions, fmt.Errorf("%w: reviewer %s candidate %d: %v", ErrProviderFailed, reviewer.Name(), index+1, err)
		}
		score.ReviewerProvider, score.ReviewerModel = reviewer.Name(), request.ReviewerModel
		if err := ValidateScore(score); err != nil {
			evaluation.Status, evaluation.CompletedAt, evaluation.FailureReason = "invalid", time.Now().UTC(), err.Error()
			executions = append(executions, evaluation)
			return nil, executions, fmt.Errorf("%w: candidate %d score: %v", ErrInvalidProviderData, index+1, err)
		}
		evaluation.Status, evaluation.CompletedAt = "succeeded", time.Now().UTC()
		executions = append(executions, evaluation)
		candidates = append(candidates, Candidate{
			Ordinal: index + 1, Label: fmt.Sprintf("候选 %c", 'A'+rune(index)), DifferenceDirection: direction,
			Components: draft.Components, Content: draft.Content, Score: score, Provider: provider.Name(),
			Model: request.GeneratorModel, PromptVersion: request.PromptVersion, RandomSeed: seed,
			GenerationParameters: request.GenerationParameters, StructuredDiff: structuredDiff(request.BaseContent, draft.Content),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score.TotalScore == candidates[j].Score.TotalScore {
			return candidates[i].Ordinal < candidates[j].Ordinal
		}
		return candidates[i].Score.TotalScore > candidates[j].Score.TotalScore
	})
	return candidates, executions, nil
}

func normalizeRequest(request *Request) {
	if request.PromptVersion == "" {
		request.PromptVersion = PromptVersion
	}
	if len(request.GenerationParameters) == 0 {
		request.GenerationParameters = json.RawMessage(`{}`)
	}
}

func NormalizeRequest(request *Request) { normalizeRequest(request) }

func ValidateRequest(request Request) error {
	normalizeRequest(&request)
	if !AllowedTargetTypes[request.TargetType] {
		return fmt.Errorf("unsupported target_type")
	}
	if strings.TrimSpace(request.TargetID) == "" {
		return fmt.Errorf("target_id is required")
	}
	if request.CandidateCount < 2 || request.CandidateCount > 12 {
		return fmt.Errorf("candidate_count must be between 2 and 12")
	}
	if len(request.ComponentTypes) == 0 || len(request.ComponentTypes) > 20 {
		return fmt.Errorf("component_types must contain 1 to 20 items")
	}
	seen := map[string]bool{}
	for _, item := range request.ComponentTypes {
		if !AllowedComponents[item] || seen[item] {
			return fmt.Errorf("unsupported or duplicate component_type %q", item)
		}
		seen[item] = true
	}
	if len(request.DifferenceDirections) == 0 {
		return fmt.Errorf("difference_directions is required")
	}
	for _, direction := range request.DifferenceDirections {
		if strings.TrimSpace(direction) == "" {
			return fmt.Errorf("difference direction cannot be empty")
		}
	}
	if request.GeneratorProvider == "" || request.GeneratorModel == "" || request.ReviewerProvider == "" || request.ReviewerModel == "" {
		return fmt.Errorf("generator and reviewer provider/model are required")
	}
	if request.GeneratorProvider == request.ReviewerProvider && request.GeneratorModel == request.ReviewerModel {
		return fmt.Errorf("generator and reviewer must not be the same model")
	}
	var parameters map[string]any
	if json.Unmarshal(request.GenerationParameters, &parameters) != nil || parameters == nil {
		return fmt.Errorf("generation_parameters must be an object")
	}
	if request.FrozenInput.SchemaVersion != "" {
		if request.FrozenInput.SchemaVersion != "candidate-frozen-input.v1" || request.FrozenInput.ContextHash == "" ||
			request.FrozenInput.FrozenHash == "" || len(request.FrozenInput.Resolution) == 0 || len(request.FrozenInput.TargetContext) == 0 {
			return fmt.Errorf("frozen_input is incomplete")
		}
	}
	return nil
}

func ValidateScore(score Score) error {
	values := []float64{score.TotalScore, score.Fidelity, score.Causality, score.CharacterConsistency,
		score.Hook, score.Pacing, score.Filmability, score.Continuity, score.ModificationRisk}
	for _, value := range values {
		if value < 0 || value > 100 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("score outside 0..100")
		}
	}
	if score.EstimatedDurationSeconds < 1 {
		return fmt.Errorf("estimated duration is required")
	}
	required := map[string]bool{"fidelity": false, "causality": false, "character_consistency": false,
		"hook": false, "pacing": false, "filmability": false, "continuity": false,
		"estimated_duration": false, "modification_risk": false}
	for _, dimension := range score.Dimensions {
		if _, ok := required[dimension.Dimension]; !ok || required[dimension.Dimension] || len(dimension.Evidence) == 0 {
			return fmt.Errorf("missing, duplicate, or unknown evidence for %q", dimension.Dimension)
		}
		for _, evidence := range dimension.Evidence {
			if evidence.SourceKind == "" || evidence.SourceID == "" || evidence.Path == "" || evidence.Reason == "" {
				return fmt.Errorf("dimension %s has non-locatable evidence", dimension.Dimension)
			}
		}
		for _, deduction := range dimension.Deductions {
			if deduction.Penalty <= 0 || deduction.Reason == "" || deduction.Location.SourceID == "" || deduction.Location.Path == "" {
				return fmt.Errorf("dimension %s has non-locatable deduction", dimension.Dimension)
			}
		}
		required[dimension.Dimension] = true
	}
	for dimension, present := range required {
		if !present {
			return fmt.Errorf("dimension %s lacks evidence", dimension)
		}
	}
	if len(score.RecommendationReasons) == 0 || len(score.DeductionReasons) == 0 {
		return fmt.Errorf("recommendation and deduction summaries are required")
	}
	return nil
}

func validateDraft(request Request, draft CandidateDraft) error {
	if len(draft.Components) == 0 || draft.Content == nil {
		return fmt.Errorf("components and content are required")
	}
	wanted := map[string]bool{}
	for _, component := range request.ComponentTypes {
		wanted[component] = true
	}
	seen := map[string]bool{}
	for _, component := range draft.Components {
		if !wanted[component.Type] || seen[component.Type] || strings.TrimSpace(component.Content) == "" {
			return fmt.Errorf("component %q is missing, duplicate, or empty", component.Type)
		}
		seen[component.Type] = true
	}
	for component := range wanted {
		if !seen[component] {
			return fmt.Errorf("component %q is missing", component)
		}
	}
	draft.Content["components"] = draft.Components
	return nil
}

func targetMediaKind(targetType string) string {
	if targetType == "image" {
		return "image"
	}
	if targetType == "video" {
		return "video"
	}
	return "text"
}

type DiffEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

func structuredDiff(base json.RawMessage, after map[string]any) []DiffEntry {
	if len(base) == 0 {
		return []DiffEntry{{Path: "/", Kind: "add", After: after}}
	}
	var before map[string]any
	if json.Unmarshal(base, &before) != nil {
		return []DiffEntry{{Path: "/", Kind: "replace", Before: string(base), After: after}}
	}
	result := []DiffEntry{}
	for key, value := range after {
		old, ok := before[key]
		oldJSON, _ := json.Marshal(old)
		newJSON, _ := json.Marshal(value)
		if !ok {
			result = append(result, DiffEntry{Path: "/" + key, Kind: "add", After: value})
		} else if string(oldJSON) != string(newJSON) {
			result = append(result, DiffEntry{Path: "/" + key, Kind: "replace", Before: old, After: value})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func clampScore(value float64) float64 { return math.Max(0, math.Min(100, value)) }
func round(value float64) float64      { return math.Round(value*100) / 100 }
