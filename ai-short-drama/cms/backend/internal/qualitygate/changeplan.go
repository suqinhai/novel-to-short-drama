package qualitygate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

type ChangeTarget struct {
	Stage      Stage  `json:"stage"`
	ArtifactID string `json:"artifact_id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	FieldPath  string `json:"field_path,omitempty"`
	StartMS    *int64 `json:"start_ms,omitempty"`
	EndMS      *int64 `json:"end_ms,omitempty"`
}

type ChangeOperation struct {
	Operation     string `json:"operation"`
	FieldPath     string `json:"field_path,omitempty"`
	Instruction   string `json:"instruction"`
	StartMS       *int64 `json:"start_ms,omitempty"`
	EndMS         *int64 `json:"end_ms,omitempty"`
	ExpectedValue string `json:"expected_value,omitempty"`
}

type ChangePlan struct {
	SchemaVersion         string            `json:"schema_version"`
	ChangePlanID          string            `json:"change_plan_id"`
	FindingID             string            `json:"finding_id"`
	Target                ChangeTarget      `json:"target"`
	Operations            []ChangeOperation `json:"operations"`
	MustPreserve          []string          `json:"must_preserve"`
	ValidationCodes       []string          `json:"validation_codes"`
	DownstreamRebuild     []Stage           `json:"downstream_rebuild"`
	RequiresConfirmation  bool              `json:"requires_confirmation"`
	DirectMutationAllowed bool              `json:"direct_mutation_allowed"`
}

func BuildLocalChangePlan(finding Finding) (ChangePlan, error) {
	if err := ValidateFinding(finding); err != nil {
		return ChangePlan{}, err
	}
	if len(finding.Locators) == 0 {
		return ChangePlan{}, errors.New("finding has no local target")
	}
	targetLocator := finding.Locators[len(finding.Locators)-1]
	target := ChangeTarget{Stage: targetLocator.Stage, ArtifactID: targetLocator.ArtifactID,
		EntityType: targetLocator.EntityType, EntityID: targetLocator.EntityID,
		FieldPath: targetLocator.FieldPath, StartMS: targetLocator.StartMS, EndMS: targetLocator.EndMS}
	operation := ChangeOperation{Operation: operationFor(finding.Dimension), FieldPath: target.FieldPath,
		Instruction: finding.Recommendation, StartMS: target.StartMS, EndMS: target.EndMS}
	for _, evidence := range finding.Evidence {
		if evidence.Expected != "" && evidence.Expected != "preserved downstream" {
			operation.ExpectedValue = evidence.Expected
			break
		}
	}
	preserve := make([]string, 0)
	seen := map[string]bool{}
	for _, evidence := range finding.Evidence {
		for _, value := range []string{evidence.Locator.SourceSpanID, evidence.Expected} {
			value = strings.TrimSpace(value)
			if value != "" && !seen[value] {
				seen[value] = true
				preserve = append(preserve, value)
			}
		}
	}
	digest := sha256.Sum256([]byte(finding.FindingID + ":" + target.ArtifactID + ":" + target.EntityID))
	return ChangePlan{SchemaVersion: ChangePlanSchema, ChangePlanID: "qgcp_" + hex.EncodeToString(digest[:])[:24],
		FindingID: finding.FindingID, Target: target, Operations: []ChangeOperation{operation},
		MustPreserve: preserve, ValidationCodes: []string{finding.Code},
		DownstreamRebuild: downstreamStages(target.Stage), RequiresConfirmation: true,
		DirectMutationAllowed: false}, nil
}

func operationFor(dimension Dimension) string {
	switch dimension {
	case DimensionEditIntegrity, DimensionAVIdentity:
		return "regenerate_segment"
	case DimensionActionCoverage:
		return "add_coverage"
	case DimensionInformationDensity, DimensionCausality, DimensionForeshadowing, DimensionHooks:
		return "adjust"
	default:
		return "replace"
	}
}

func downstreamStages(stage Stage) []Stage {
	for index, candidate := range StageOrder {
		if candidate == stage && index+1 < len(StageOrder) {
			return append([]Stage(nil), StageOrder[index+1:]...)
		}
	}
	return []Stage{}
}
