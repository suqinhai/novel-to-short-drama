package effectiveinput

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidRequest = errors.New("invalid effective input request")
	ErrNotFound       = errors.New("effective input scope not found")
)

type Item struct {
	Kind         string          `json:"kind"`
	Requirement  string          `json:"requirement"`
	State        string          `json:"state"`
	InputID      *string         `json:"input_id,omitempty"`
	InputIDs     []string        `json:"input_ids"`
	Versions     json.RawMessage `json:"versions"`
	ContentHash  *string         `json:"content_hash,omitempty"`
	SourceStatus string          `json:"source_status"`
	Content      json.RawMessage `json:"content"`
	ArtifactIDs  []string        `json:"artifact_ids"`
	Reason       *string         `json:"reason,omitempty"`
	Blocks       bool            `json:"blocks"`
}

type Diagnostic struct {
	Kind   string `json:"kind"`
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type Resolution struct {
	SchemaVersion   string          `json:"schema_version"`
	ResolverVersion string          `json:"resolver_version"`
	ResolutionID    string          `json:"resolution_id"`
	ProjectID       string          `json:"project_id"`
	EpisodeID       *string         `json:"episode_id"`
	Stage           string          `json:"stage"`
	Mode            string          `json:"mode"`
	Status          string          `json:"status"`
	Ready           bool            `json:"ready"`
	ContextHash     string          `json:"context_hash"`
	ResolutionHash  string          `json:"resolution_hash"`
	Items           []Item          `json:"items"`
	Context         json.RawMessage `json:"context"`
	Missing         []string        `json:"missing"`
	Blockers        []Diagnostic    `json:"blockers"`
}

type Repository interface {
	ResolveEffectiveInputs(context.Context, string, string, string) (json.RawMessage, error)
}

type Resolver struct {
	repository Repository
}

func New(repository Repository) *Resolver {
	return &Resolver{repository: repository}
}

func (r *Resolver) Resolve(ctx context.Context, projectID, episodeID, stage string) (Resolution, error) {
	projectID = strings.TrimSpace(projectID)
	episodeID = strings.TrimSpace(episodeID)
	stage = strings.TrimSpace(stage)
	if r == nil || r.repository == nil {
		return Resolution{}, fmt.Errorf("%w: resolver repository is unavailable", ErrInvalidRequest)
	}
	if projectID == "" || stage == "" {
		return Resolution{}, fmt.Errorf("%w: project_id and stage are required", ErrInvalidRequest)
	}
	raw, err := r.repository.ResolveEffectiveInputs(ctx, projectID, episodeID, stage)
	if err != nil {
		return Resolution{}, err
	}
	var result Resolution
	if err := json.Unmarshal(raw, &result); err != nil {
		return Resolution{}, fmt.Errorf("decode effective input resolution: %w", err)
	}
	if err := validateResolution(result); err != nil {
		return Resolution{}, err
	}
	return result, nil
}

func validateResolution(result Resolution) error {
	if result.SchemaVersion != "effective-input-resolution.v1" ||
		result.ResolverVersion == "" || result.ResolutionID == "" ||
		result.ProjectID == "" || result.Stage == "" ||
		result.ContextHash == "" || result.ResolutionHash == "" {
		return fmt.Errorf("invalid effective input resolution envelope")
	}
	if result.Status != "ready" && result.Status != "needs_review" && result.Status != "blocked" {
		return fmt.Errorf("invalid effective input resolution status %q", result.Status)
	}
	for _, item := range result.Items {
		if item.Requirement != "required" && item.Requirement != "optional" {
			return fmt.Errorf("invalid requirement %q for %s", item.Requirement, item.Kind)
		}
		switch item.State {
		case "resolved", "missing", "stale", "needs_review", "blocked":
		default:
			return fmt.Errorf("invalid state %q for %s", item.State, item.Kind)
		}
		if item.State == "resolved" && (len(item.InputIDs) == 0 || item.ContentHash == nil) {
			return fmt.Errorf("resolved input %s is missing identity/hash", item.Kind)
		}
	}
	return nil
}
