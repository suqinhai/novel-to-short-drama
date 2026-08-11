package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/candidategeneration"
	"short-drama-cms/backend/internal/promptlab"
)

func (s *Store) applyProductionCandidatePrompt(ctx context.Context, request *candidategeneration.Request) error {
	promptKey := strings.TrimSpace(request.PromptKey)
	if promptKey == "" {
		promptKey = "production.candidate." + request.TargetType
	}
	var versionID, systemTemplate, userTemplate string
	var schema, defaults, modelDefaults json.RawMessage
	err := s.pool.QueryRow(ctx, `SELECT version.prompt_version_id,version.system_template,version.user_template,
		version.variable_schema,version.default_variables,version.model_defaults
		FROM drama.prompt_templates template
		JOIN drama.prompt_production_bindings binding
		  ON binding.prompt_template_id=template.prompt_template_id AND binding.is_current
		JOIN drama.prompt_versions version ON version.prompt_version_id=binding.prompt_version_id
		WHERE template.prompt_key=$1 AND version.status='approved'`, promptKey).Scan(
		&versionID, &systemTemplate, &userTemplate, &schema, &defaults, &modelDefaults)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	variables, err := json.Marshal(map[string]any{
		"target_type": request.TargetType, "target_id": request.TargetID,
		"component_types": request.ComponentTypes, "candidate_count": request.CandidateCount,
		"difference_directions": request.DifferenceDirections, "must_preserve": request.MustPreserve,
		"allowed_changes": request.AllowedChanges, "base_duration_seconds": request.BaseDurationSeconds,
		"frozen_input": request.FrozenInput,
	})
	if err != nil {
		return err
	}
	preview, err := promptlab.Render(systemTemplate, userTemplate, schema, defaults, variables)
	if err != nil {
		return fmt.Errorf("%w: active production prompt %s cannot render: %v", ErrValidation, versionID, err)
	}
	request.PromptKey = promptKey
	request.PromptVersion = versionID
	request.ProductionPrompt = preview.FinalInput

	settings := map[string]any{}
	if len(modelDefaults) > 0 && json.Unmarshal(modelDefaults, &settings) == nil {
		if provider, ok := settings["provider"].(string); ok && strings.TrimSpace(provider) != "" {
			request.GeneratorProvider = strings.TrimSpace(provider)
		}
		if model, ok := settings["model"].(string); ok && strings.TrimSpace(model) != "" {
			request.GeneratorModel = strings.TrimSpace(model)
		}
		parameters := map[string]any{}
		_ = json.Unmarshal(request.GenerationParameters, &parameters)
		for key, value := range settings {
			if key != "provider" && key != "model" {
				if _, exists := parameters[key]; !exists {
					parameters[key] = value
				}
			}
		}
		request.GenerationParameters, _ = json.Marshal(parameters)
	}
	return nil
}
