package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"short-drama-cms/backend/internal/candidategeneration"
)

type GenerateCandidateSetInput struct {
	candidategeneration.Request
	BaseArtifactID    string
	ParentCandidateID string
	DerivedReason     string
}

type CandidateScore struct {
	TotalScore               float64         `json:"total_score"`
	Fidelity                 float64         `json:"fidelity"`
	Causality                float64         `json:"causality"`
	CharacterConsistency     float64         `json:"character_consistency"`
	Hook                     float64         `json:"hook"`
	Pacing                   float64         `json:"pacing"`
	Continuity               float64         `json:"continuity"`
	Filmability              float64         `json:"filmability"`
	EstimatedDurationSeconds int             `json:"estimated_duration_seconds"`
	ModificationRisk         float64         `json:"modification_risk"`
	RecommendationReasons    json.RawMessage `json:"recommendation_reasons"`
	DeductionReasons         json.RawMessage `json:"deduction_reasons"`
	Dimensions               json.RawMessage `json:"dimensions"`
	ReviewerProvider         string          `json:"reviewer_provider,omitempty"`
	ReviewerModel            string          `json:"reviewer_model,omitempty"`
}

type CandidateVersion struct {
	CandidateID          string          `json:"candidate_id"`
	CandidateSetID       string          `json:"candidate_set_id"`
	ParentCandidateID    *string         `json:"parent_candidate_id,omitempty"`
	ArtifactID           string          `json:"artifact_id"`
	Ordinal              int             `json:"ordinal"`
	Rank                 int             `json:"rank"`
	Label                string          `json:"label"`
	DifferenceDirection  string          `json:"difference_direction"`
	DerivedReason        *string         `json:"derived_reason,omitempty"`
	Content              json.RawMessage `json:"content"`
	StructuredDiff       json.RawMessage `json:"structured_diff"`
	ContentHash          string          `json:"content_hash"`
	Provider             string          `json:"provider,omitempty"`
	Model                string          `json:"model"`
	PromptVersion        string          `json:"prompt_version"`
	RandomSeed           int64           `json:"random_seed"`
	GenerationParameters json.RawMessage `json:"generation_parameters"`
	Score                CandidateScore  `json:"score"`
	IsFavorite           bool            `json:"is_favorite"`
	IsEliminated         bool            `json:"is_eliminated"`
	CreatedAt            time.Time       `json:"created_at"`
}

type CandidateSet struct {
	CandidateSetID       string             `json:"candidate_set_id"`
	ProjectID            string             `json:"project_id"`
	TargetType           string             `json:"target_type"`
	TargetID             string             `json:"target_id"`
	BaseArtifactID       *string            `json:"base_artifact_id,omitempty"`
	QualityScoreReportID string             `json:"quality_score_report_id"`
	CandidateCount       int                `json:"candidate_count"`
	ComponentTypes       json.RawMessage    `json:"component_types"`
	DifferenceDirections json.RawMessage    `json:"difference_directions"`
	MustPreserve         json.RawMessage    `json:"must_preserve"`
	AllowedChanges       json.RawMessage    `json:"allowed_changes"`
	Model                string             `json:"model"`
	GeneratorProvider    string             `json:"generator_provider,omitempty"`
	GeneratorModel       string             `json:"generator_model,omitempty"`
	ReviewerProvider     string             `json:"reviewer_provider,omitempty"`
	ReviewerModel        string             `json:"reviewer_model,omitempty"`
	BlindReview          bool               `json:"blind_review"`
	FrozenResolutionID   string             `json:"frozen_resolution_id"`
	FrozenContextHash    string             `json:"frozen_context_hash"`
	FrozenInputHash      string             `json:"frozen_input_hash"`
	FrozenInput          json.RawMessage    `json:"frozen_input"`
	PromptVersion        string             `json:"prompt_version"`
	RandomSeed           int64              `json:"random_seed"`
	GenerationParameters json.RawMessage    `json:"generation_parameters"`
	EstimatedCost        float64            `json:"estimated_cost"`
	Currency             string             `json:"currency"`
	GeneratorVersion     string             `json:"generator_version"`
	Candidates           []CandidateVersion `json:"candidates"`
	CreatedAt            time.Time          `json:"created_at"`
}

type CandidateDecisionInput struct {
	Decision  string `json:"decision"`
	Reason    string `json:"reason"`
	DecidedBy string `json:"decided_by"`
}

type CandidateDecision struct {
	CandidateDecisionID string    `json:"candidate_decision_id"`
	CandidateID         string    `json:"candidate_id"`
	Decision            string    `json:"decision"`
	Reason              string    `json:"reason"`
	DecidedBy           string    `json:"decided_by"`
	CreatedAt           time.Time `json:"created_at"`
}

type CandidateSelectionInput struct {
	CandidateID string `json:"candidate_id"`
	Confirmed   bool   `json:"confirmed"`
	ConfirmedBy string `json:"confirmed_by"`
}

type CandidateCompositionPartInput struct {
	ComponentKey string `json:"component_key"`
	CandidateID  string `json:"candidate_id"`
}

type CandidateCompositionInput struct {
	Parts       []CandidateCompositionPartInput `json:"parts"`
	Confirmed   bool                            `json:"confirmed"`
	ConfirmedBy string                          `json:"confirmed_by"`
}

type CandidateSelection struct {
	CandidateSelectionID string          `json:"candidate_selection_id"`
	CandidateSetID       string          `json:"candidate_set_id"`
	SelectedCandidateID  *string         `json:"selected_candidate_id,omitempty"`
	ArtifactID           string          `json:"artifact_id"`
	SelectionType        string          `json:"selection_type"`
	Content              json.RawMessage `json:"content"`
	ValidationSummary    json.RawMessage `json:"validation_summary"`
	ConfirmedBy          string          `json:"confirmed_by"`
	CreatedAt            time.Time       `json:"created_at"`
}

type TimecodeCommentInput struct {
	TimecodeMS  int64  `json:"timecode_ms"`
	CommentText string `json:"comment_text"`
	Author      string `json:"author"`
}

type TimecodeComment struct {
	CandidateTimecodeCommentID string    `json:"candidate_timecode_comment_id"`
	CandidateID                string    `json:"candidate_id"`
	TimecodeMS                 int64     `json:"timecode_ms"`
	CommentText                string    `json:"comment_text"`
	Author                     string    `json:"author"`
	CreatedAt                  time.Time `json:"created_at"`
}

func (s *Store) GenerateCandidateSet(ctx context.Context, projectID, key string, input GenerateCandidateSetInput) (CandidateSet, bool, error) {
	candidategeneration.NormalizeRequest(&input.Request)
	if err := candidategeneration.ValidateRequest(input.Request); err != nil {
		return CandidateSet{}, false, ErrConflict
	}
	clientRequestHash, err := hashJSON(input)
	if err != nil {
		return CandidateSet{}, false, err
	}
	var idempotentID, idempotentClientHash string
	err = s.pool.QueryRow(ctx, `SELECT candidate_set_id,client_request_hash FROM drama.candidate_sets
		WHERE idempotency_key=$1`, key).Scan(&idempotentID, &idempotentClientHash)
	if err == nil {
		if idempotentClientHash != clientRequestHash {
			return CandidateSet{}, false, ErrConflict
		}
		result, replayErr := s.GetCandidateSet(ctx, projectID, idempotentID)
		return result, false, replayErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CandidateSet{}, false, err
	}
	frozen, err := s.freezeCandidateInputs(ctx, projectID, input.Request)
	if err != nil {
		return CandidateSet{}, false, err
	}
	input.FrozenInput = frozen
	if len(input.BaseContent) == 0 {
		input.BaseContent = frozen.TargetContext
	}
	requestHash, err := hashJSON(input)
	if err != nil {
		return CandidateSet{}, false, err
	}
	// A frozen input + request + seed is a replay identity, independent of the
	// HTTP idempotency key. Returning the persisted set avoids a second real call.
	var replayID, replayHash string
	err = s.pool.QueryRow(ctx, `SELECT candidate_set_id,request_hash FROM drama.candidate_sets
		WHERE idempotency_key=$1 OR (project_id=$2 AND request_hash=$3)
		ORDER BY (idempotency_key=$1) DESC,created_at DESC LIMIT 1`, key, projectID, requestHash).Scan(&replayID, &replayHash)
	if err == nil {
		if replayHash != requestHash {
			return CandidateSet{}, false, ErrConflict
		}
		result, replayErr := s.GetCandidateSet(ctx, projectID, replayID)
		return result, false, replayErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CandidateSet{}, false, err
	}
	engine := s.candidateEngine
	if engine == nil {
		engine = candidategeneration.NewRegistryFromEnvironment()
	}
	generated, executions, err := engine.GenerateAndReviewAudited(ctx, input.Request)
	if err != nil {
		if auditErr := persistCandidateExecutions(ctx, s.writer, projectID, "", requestHash, key, executions, nil); auditErr != nil {
			return CandidateSet{}, false, fmt.Errorf("persist failed candidate audit: %w (provider error: %v)", auditErr, err)
		}
		return CandidateSet{}, false, fmt.Errorf("%w: %v", ErrCandidateProvider, err)
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return CandidateSet{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "candidate-set:"+projectID+":"+input.TargetType+":"+input.TargetID); err != nil {
		return CandidateSet{}, false, err
	}
	var existingID, existingHash string
	err = tx.QueryRow(ctx, `SELECT candidate_set_id,request_hash FROM drama.candidate_sets
		WHERE idempotency_key=$1 OR (project_id=$2 AND request_hash=$3)
		ORDER BY (idempotency_key=$1) DESC,created_at DESC LIMIT 1`, key, projectID, requestHash).
		Scan(&existingID, &existingHash)
	if err == nil {
		if existingHash != requestHash {
			return CandidateSet{}, false, ErrConflict
		}
		result, err := getCandidateSetTx(ctx, tx, projectID, existingID)
		return result, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CandidateSet{}, false, err
	}
	var qualityReportID, qualityArtifactID string
	if err := tx.QueryRow(ctx, `SELECT quality_score_report_id,artifact_id
		FROM drama.quality_score_reports WHERE project_id=$1 AND status='completed'
		ORDER BY created_at DESC LIMIT 1`, projectID).
		Scan(&qualityReportID, &qualityArtifactID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CandidateSet{}, false, ErrNotFound
		}
		return CandidateSet{}, false, err
	}
	if input.BaseArtifactID != "" {
		var belongs bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.artifacts
			WHERE artifact_id=$1 AND project_id=$2)`, input.BaseArtifactID, projectID).Scan(&belongs); err != nil || !belongs {
			if err != nil {
				return CandidateSet{}, false, err
			}
			return CandidateSet{}, false, ErrNotFound
		}
	}
	if input.ParentCandidateID != "" {
		var parentBelongs bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM drama.candidates candidate JOIN drama.candidate_sets set USING(candidate_set_id)
			WHERE candidate.candidate_id=$1 AND set.project_id=$2
		)`, input.ParentCandidateID, projectID).Scan(&parentBelongs); err != nil || !parentBelongs {
			if err != nil {
				return CandidateSet{}, false, err
			}
			return CandidateSet{}, false, ErrNotFound
		}
	}
	candidateSetID, _ := newPublicID("candset_")
	candidateIDsByOrdinal := make(map[int]string, len(generated))
	estimatedCost := estimateCandidateCost(input.TargetType, input.CandidateCount, len(input.ComponentTypes))
	if _, err := tx.Exec(ctx, `INSERT INTO drama.candidate_sets(
		candidate_set_id,project_id,target_type,target_id,base_artifact_id,quality_score_report_id,
		candidate_count,component_types,difference_directions,must_preserve,allowed_changes,model,
		prompt_version,random_seed,generation_parameters,estimated_cost,generator_version,idempotency_key,request_hash,
		generator_provider,generator_model,reviewer_provider,reviewer_model,blind_review,
		frozen_resolution_id,frozen_context_hash,frozen_input_hash,frozen_input,client_request_hash)
		VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,
		$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)`,
		candidateSetID, projectID, input.TargetType, input.TargetID, input.BaseArtifactID, qualityReportID,
		input.CandidateCount, mustJSON(input.ComponentTypes), mustJSON(input.DifferenceDirections),
		mustJSON(input.MustPreserve), mustJSON(input.AllowedChanges), input.GeneratorModel,
		defaultCandidatePrompt(input.PromptVersion), input.RandomSeed, defaultJSON(input.GenerationParameters),
		estimatedCost, candidategeneration.GeneratorVersion, key, requestHash,
		input.GeneratorProvider, input.GeneratorModel, input.ReviewerProvider, input.ReviewerModel, input.BlindReview,
		frozen.ResolutionID, frozen.ContextHash, frozen.FrozenHash, mustJSON(frozen), clientRequestHash); err != nil {
		return CandidateSet{}, false, mapPGConflict(err)
	}
	for index, candidate := range generated {
		candidateID, _ := newPublicID("cand_")
		candidateIDsByOrdinal[candidate.Ordinal] = candidateID
		artifactID, _ := newPublicID("artifact_")
		contentHash, _ := hashJSON(candidate.Content)
		if _, err := tx.Exec(ctx, `INSERT INTO drama.artifacts(
			artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
			validity_status,is_current,idempotency_key,metadata)
			VALUES($1,'candidate_version',$2,$3,1,$4,'needs_review',false,$5,$6)`,
			artifactID, projectID, candidateID, contentHash, key+":artifact:"+fmt.Sprint(index+1),
			mustJSON(map[string]any{"candidate_set_id": candidateSetID, "downstream_eligible": false})); err != nil {
			return CandidateSet{}, false, mapPGConflict(err)
		}
		parentID := input.ParentCandidateID
		if _, err := tx.Exec(ctx, `INSERT INTO drama.candidates(
			candidate_id,candidate_set_id,parent_candidate_id,artifact_id,ordinal,label,difference_direction,
			derived_reason,content,structured_diff,content_hash,model,prompt_version,random_seed,generation_parameters,provider)
			VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13,$14,$15,$16)`,
			candidateID, candidateSetID, parentID, artifactID, candidate.Ordinal, candidate.Label,
			candidate.DifferenceDirection, input.DerivedReason, mustJSON(candidate.Content),
			mustJSON(candidate.StructuredDiff), contentHash, candidate.Model, candidate.PromptVersion,
			candidate.RandomSeed, candidate.GenerationParameters, candidate.Provider); err != nil {
			return CandidateSet{}, false, mapPGConflict(err)
		}
		scoreID, _ := newPublicID("candscore_")
		score := candidate.Score
		if _, err := tx.Exec(ctx, `INSERT INTO drama.candidate_scores(
			candidate_score_id,candidate_id,source_quality_score_report_id,total_score,fidelity,hook,pacing,
			continuity,filmability,estimated_duration_seconds,modification_risk,recommendation_reasons,
			deduction_reasons,scorer_version,causality,character_consistency,dimensions,reviewer_provider,reviewer_model)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
			scoreID, candidateID, qualityReportID, score.TotalScore, score.Fidelity, score.Hook, score.Pacing,
			score.Continuity, score.Filmability, score.EstimatedDurationSeconds, score.ModificationRisk,
			mustJSON(score.RecommendationReasons), mustJSON(score.DeductionReasons),
			adaptationScoreVersion(), score.Causality, score.CharacterConsistency, mustJSON(score.Dimensions),
			score.ReviewerProvider, score.ReviewerModel); err != nil {
			return CandidateSet{}, false, mapPGConflict(err)
		}
		if err := createDependency(ctx, tx, qualityArtifactID, artifactID, "candidate_quality_baseline",
			key+":quality:"+fmt.Sprint(index+1)); err != nil {
			return CandidateSet{}, false, err
		}
		for dependencyIndex, upstream := range frozenArtifactIDs(frozen.Resolution) {
			if err := createDependency(ctx, tx, upstream, artifactID, "candidate_frozen_effective_input",
				fmt.Sprintf("%s:frozen:%d:%d", key, index+1, dependencyIndex+1)); err != nil {
				return CandidateSet{}, false, err
			}
		}
		if input.BaseArtifactID != "" {
			if err := createDependency(ctx, tx, input.BaseArtifactID, artifactID, "candidate_derived_from",
				key+":base:"+fmt.Sprint(index+1)); err != nil {
				return CandidateSet{}, false, err
			}
		}
	}
	if err := persistCandidateExecutions(ctx, tx, projectID, candidateSetID, requestHash, key,
		executions, candidateIDsByOrdinal); err != nil {
		return CandidateSet{}, false, err
	}
	result, err := getCandidateSetTx(ctx, tx, projectID, candidateSetID)
	if err != nil {
		return CandidateSet{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CandidateSet{}, false, mapPGConflict(err)
	}
	return result, true, nil
}

type candidateExecutionWriter interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func persistCandidateExecutions(
	ctx context.Context, writer candidateExecutionWriter, projectID, candidateSetID,
	requestHash, requestKey string, executions []candidategeneration.ExecutionRecord,
	candidateIDs map[int]string,
) error {
	for _, execution := range executions {
		candidateID := ""
		if candidateIDs != nil {
			candidateID = candidateIDs[execution.Ordinal]
		}
		shortType := "gen"
		if execution.ExecutionType == "evaluation" {
			shortType = "eval"
		}
		executionID := fmt.Sprintf("cex_%s_%s_%d_%d", requestHash[:16], shortType,
			execution.Ordinal, execution.Attempt)
		idempotencyKey := fmt.Sprintf("%s:execution:%s:%d:%d", requestKey,
			execution.ExecutionType, execution.Ordinal, execution.Attempt)
		_, err := writer.Exec(ctx, `INSERT INTO drama.candidate_execution_records(
			candidate_execution_id,project_id,candidate_set_id,candidate_id,request_hash,
			execution_type,ordinal,status,started_at,completed_at,provider,model,
			failure_reason,retry_count,attempt,blind,idempotency_key)
			VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12,
			NULLIF($13,''),$14,$15,$16,$17)
			ON CONFLICT(candidate_execution_id) DO UPDATE SET
			status=EXCLUDED.status,completed_at=EXCLUDED.completed_at,
			failure_reason=EXCLUDED.failure_reason
			WHERE drama.candidate_execution_records.request_hash=EXCLUDED.request_hash
			  AND drama.candidate_execution_records.execution_type=EXCLUDED.execution_type
			  AND drama.candidate_execution_records.ordinal=EXCLUDED.ordinal
			  AND drama.candidate_execution_records.attempt=EXCLUDED.attempt`,
			executionID, projectID, candidateSetID, candidateID, requestHash,
			execution.ExecutionType, execution.Ordinal, execution.Status,
			execution.StartedAt, execution.CompletedAt, execution.Provider, execution.Model,
			execution.FailureReason, execution.RetryCount, execution.Attempt,
			execution.Blind, idempotencyKey)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListCandidateSets(ctx context.Context, projectID string) ([]CandidateSet, error) {
	rows, err := s.pool.Query(ctx, `SELECT candidate_set_id FROM drama.candidate_sets
		WHERE project_id=$1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	result := make([]CandidateSet, 0, len(ids))
	for _, id := range ids {
		item, err := s.GetCandidateSet(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) GetCandidateSet(ctx context.Context, projectID, candidateSetID string) (CandidateSet, error) {
	return getCandidateSetQuery(ctx, s.pool, projectID, candidateSetID)
}

func (s *Store) RecordCandidateDecision(ctx context.Context, candidateID, key string, input CandidateDecisionInput) (CandidateDecision, bool, error) {
	if !matchesCandidateDecision(input.Decision) {
		return CandidateDecision{}, false, ErrConflict
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return CandidateDecision{}, false, err
	}
	defer tx.Rollback(ctx)
	var result CandidateDecision
	err = tx.QueryRow(ctx, `SELECT candidate_decision_id,candidate_id,decision,COALESCE(reason,''),
		COALESCE(decided_by,''),created_at FROM drama.candidate_decisions WHERE idempotency_key=$1`, key).
		Scan(&result.CandidateDecisionID, &result.CandidateID, &result.Decision, &result.Reason, &result.DecidedBy, &result.CreatedAt)
	if err == nil {
		if result.CandidateID != candidateID || result.Decision != input.Decision {
			return CandidateDecision{}, false, ErrConflict
		}
		return result, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CandidateDecision{}, false, err
	}
	id, _ := newPublicID("canddecision_")
	err = tx.QueryRow(ctx, `INSERT INTO drama.candidate_decisions(
		candidate_decision_id,candidate_id,decision,reason,decided_by,idempotency_key)
		SELECT $1,candidate_id,$2,NULLIF($3,''),NULLIF($4,''),$5 FROM drama.candidates WHERE candidate_id=$6
		RETURNING candidate_decision_id,candidate_id,decision,COALESCE(reason,''),COALESCE(decided_by,''),created_at`,
		id, input.Decision, input.Reason, input.DecidedBy, key, candidateID).
		Scan(&result.CandidateDecisionID, &result.CandidateID, &result.Decision, &result.Reason, &result.DecidedBy, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CandidateDecision{}, false, ErrNotFound
	}
	if err != nil {
		return CandidateDecision{}, false, mapPGConflict(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CandidateDecision{}, false, mapPGConflict(err)
	}
	return result, true, nil
}

func (s *Store) SelectCandidate(ctx context.Context, projectID, candidateSetID, key string, input CandidateSelectionInput) (CandidateSelection, bool, error) {
	if !input.Confirmed || strings.TrimSpace(input.ConfirmedBy) == "" {
		return CandidateSelection{}, false, ErrConflict
	}
	return s.createSelection(ctx, projectID, candidateSetID, key, "candidate", input.CandidateID, nil, input.ConfirmedBy)
}

func (s *Store) ComposeCandidates(ctx context.Context, projectID, candidateSetID, key string, input CandidateCompositionInput) (CandidateSelection, bool, error) {
	if !input.Confirmed || strings.TrimSpace(input.ConfirmedBy) == "" || len(input.Parts) < 2 || len(input.Parts) > 20 {
		return CandidateSelection{}, false, ErrConflict
	}
	seen := map[string]bool{}
	for _, part := range input.Parts {
		if part.ComponentKey == "" || part.CandidateID == "" || seen[part.ComponentKey] {
			return CandidateSelection{}, false, ErrConflict
		}
		seen[part.ComponentKey] = true
	}
	return s.createSelection(ctx, projectID, candidateSetID, key, "composition", "", input.Parts, input.ConfirmedBy)
}

func (s *Store) createSelection(ctx context.Context, projectID, candidateSetID, key, selectionType, candidateID string,
	parts []CandidateCompositionPartInput, confirmedBy string) (CandidateSelection, bool, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return CandidateSelection{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "candidate-selection:"+projectID+":"+candidateSetID); err != nil {
		return CandidateSelection{}, false, err
	}
	var replay CandidateSelection
	err = scanSelection(tx.QueryRow(ctx, selectionQuery+` WHERE selection.idempotency_key=$1`, key), &replay)
	if err == nil {
		if replay.CandidateSetID != candidateSetID || replay.SelectionType != selectionType {
			return CandidateSelection{}, false, ErrConflict
		}
		if selectionType == "candidate" {
			if replay.SelectedCandidateID == nil || *replay.SelectedCandidateID != candidateID {
				return CandidateSelection{}, false, ErrConflict
			}
		} else {
			rows, queryErr := tx.Query(ctx, `SELECT component_key,source_candidate_id
				FROM drama.candidate_composition_parts WHERE candidate_selection_id=$1
				ORDER BY component_key`, replay.CandidateSelectionID)
			if queryErr != nil {
				return CandidateSelection{}, false, queryErr
			}
			existing := []CandidateCompositionPartInput{}
			for rows.Next() {
				var part CandidateCompositionPartInput
				if queryErr = rows.Scan(&part.ComponentKey, &part.CandidateID); queryErr != nil {
					rows.Close()
					return CandidateSelection{}, false, queryErr
				}
				existing = append(existing, part)
			}
			rows.Close()
			expected := append([]CandidateCompositionPartInput(nil), parts...)
			sort.Slice(expected, func(i, j int) bool { return expected[i].ComponentKey < expected[j].ComponentKey })
			if len(existing) != len(expected) {
				return CandidateSelection{}, false, ErrConflict
			}
			for i := range existing {
				if existing[i] != expected[i] {
					return CandidateSelection{}, false, ErrConflict
				}
			}
		}
		return replay, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CandidateSelection{}, false, err
	}
	var targetType, targetID string
	var baseDuration int
	if err := tx.QueryRow(ctx, `SELECT target_type,target_id,COALESCE((
		SELECT max(estimated_duration_seconds) FROM drama.candidate_scores score
		JOIN drama.candidates candidate USING(candidate_id) WHERE candidate.candidate_set_id=set.candidate_set_id
	),0) FROM drama.candidate_sets set WHERE candidate_set_id=$1 AND project_id=$2`,
		candidateSetID, projectID).Scan(&targetType, &targetID, &baseDuration); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CandidateSelection{}, false, ErrNotFound
		}
		return CandidateSelection{}, false, err
	}
	var content map[string]any
	sourceArtifacts := []string{}
	if selectionType == "candidate" {
		var raw json.RawMessage
		var artifactID string
		if err := tx.QueryRow(ctx, `SELECT content,artifact_id FROM drama.candidates
			WHERE candidate_id=$1 AND candidate_set_id=$2`, candidateID, candidateSetID).Scan(&raw, &artifactID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return CandidateSelection{}, false, ErrNotFound
			}
			return CandidateSelection{}, false, err
		}
		if json.Unmarshal(raw, &content) != nil {
			return CandidateSelection{}, false, ErrConflict
		}
		sourceArtifacts = append(sourceArtifacts, artifactID)
	} else {
		candidates := map[string]candidategeneration.Candidate{}
		selected := map[string]string{}
		for _, part := range parts {
			var raw json.RawMessage
			var artifactID, setID string
			if err := tx.QueryRow(ctx, `SELECT content,artifact_id,candidate_set_id FROM drama.candidates WHERE candidate_id=$1`,
				part.CandidateID).Scan(&raw, &artifactID, &setID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return CandidateSelection{}, false, ErrNotFound
				}
				return CandidateSelection{}, false, err
			}
			if setID != candidateSetID {
				return CandidateSelection{}, false, ErrConflict
			}
			var candidate candidategeneration.Candidate
			var body struct {
				Components []candidategeneration.Component `json:"components"`
			}
			if json.Unmarshal(raw, &body) != nil {
				return CandidateSelection{}, false, ErrConflict
			}
			candidate.Components = body.Components
			candidates[part.CandidateID] = candidate
			selected[part.ComponentKey] = part.CandidateID
			sourceArtifacts = append(sourceArtifacts, artifactID)
		}
		var validation candidategeneration.Validation
		content, validation = candidategeneration.Compose(candidates, selected, baseDuration)
		if !validation.Passed {
			return CandidateSelection{}, false, ErrConflict
		}
	}
	validation := candidategeneration.ValidateComposition(content, baseDuration)
	if !validation.Passed {
		return CandidateSelection{}, false, ErrConflict
	}
	selectionID, _ := newPublicID("selection_")
	artifactID, _ := newPublicID("artifact_")
	contentHash, _ := hashJSON(content)
	var revision int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision_number),0)+1 FROM drama.artifacts
		WHERE project_id=$1 AND artifact_type='candidate_selection' AND native_entity_id=$2`, projectID, targetID).Scan(&revision); err != nil {
		return CandidateSelection{}, false, err
	}
	var previousArtifactID string
	_ = tx.QueryRow(ctx, `SELECT current_artifact_id FROM drama.artifact_current_bindings
		WHERE project_id=$1 AND target_type=$2 AND target_id=$3 AND component_scope='whole'`,
		projectID, targetType, targetID).Scan(&previousArtifactID)
	if previousArtifactID != "" {
		if _, err := tx.Exec(ctx, `UPDATE drama.artifacts SET is_current=false,validity_status='superseded'
			WHERE artifact_id=$1`, previousArtifactID); err != nil {
			return CandidateSelection{}, false, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.artifacts(
		artifact_id,artifact_type,project_id,native_entity_id,revision_number,content_hash,
		validity_status,is_current,idempotency_key,metadata)
		VALUES($1,'candidate_selection',$2,$3,$4,$5,'valid',true,$6,$7)`,
		artifactID, projectID, targetID, revision, contentHash, key+":artifact",
		mustJSON(map[string]any{"candidate_set_id": candidateSetID, "selection_type": selectionType, "downstream_eligible": true})); err != nil {
		return CandidateSelection{}, false, mapPGConflict(err)
	}
	validationRaw := mustJSON(validation)
	var selected any
	if candidateID != "" {
		selected = candidateID
	}
	if _, err := tx.Exec(ctx, `INSERT INTO drama.candidate_selections(
		candidate_selection_id,candidate_set_id,selected_candidate_id,artifact_id,selection_type,
		content,validation_summary,confirmed_by,idempotency_key)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9)`,
		selectionID, candidateSetID, selected, artifactID, selectionType, mustJSON(content),
		validationRaw, confirmedBy, key); err != nil {
		return CandidateSelection{}, false, mapPGConflict(err)
	}
	if selectionType == "composition" {
		for index, part := range parts {
			partID, _ := newPublicID("compositionpart_")
			if _, err := tx.Exec(ctx, `INSERT INTO drama.candidate_composition_parts(
				candidate_composition_part_id,candidate_selection_id,component_key,source_candidate_id,ordinal)
				VALUES($1,$2,$3,$4,$5)`, partID, selectionID, part.ComponentKey, part.CandidateID, index+1); err != nil {
				return CandidateSelection{}, false, mapPGConflict(err)
			}
		}
	}
	for _, result := range validation.Results {
		resultID, _ := newPublicID("hardrule_")
		if _, err := tx.Exec(ctx, `INSERT INTO drama.candidate_hard_rule_results(
			candidate_hard_rule_result_id,candidate_selection_id,rule_name,passed,message)
			VALUES($1,$2,$3,$4,$5)`, resultID, selectionID, result.Rule, result.Passed, result.Message); err != nil {
			return CandidateSelection{}, false, mapPGConflict(err)
		}
	}
	for index, upstream := range uniqueStrings(sourceArtifacts) {
		if err := createDependency(ctx, tx, upstream, artifactID, "candidate_selected_component",
			key+":source:"+fmt.Sprint(index+1)); err != nil {
			return CandidateSelection{}, false, err
		}
	}
	bindingID, _ := newPublicID("currentbinding_")
	if _, err := tx.Exec(ctx, `INSERT INTO drama.artifact_current_bindings(
		artifact_current_binding_id,project_id,target_type,target_id,component_scope,current_artifact_id)
		VALUES($1,$2,$3,$4,'whole',$5)
		ON CONFLICT(project_id,target_type,target_id,component_scope)
		DO UPDATE SET current_artifact_id=EXCLUDED.current_artifact_id,selected_at=CURRENT_TIMESTAMP`,
		bindingID, projectID, targetType, targetID, artifactID); err != nil {
		return CandidateSelection{}, false, mapPGConflict(err)
	}
	// A story-arc decision is a project-wide upstream decision. Alias the same
	// confirmed artifact to every episode so the existing Effective Input
	// Resolver exposes it to subsequent script/storyboard generations.
	if targetType == "story_arc" {
		episodeRows, queryErr := tx.Query(ctx, `SELECT episode_id FROM drama.episode_outlines
			WHERE project_id=$1 ORDER BY episode_number,episode_id`, projectID)
		if queryErr != nil {
			return CandidateSelection{}, false, queryErr
		}
		for episodeRows.Next() {
			var episodeID string
			if queryErr = episodeRows.Scan(&episodeID); queryErr != nil {
				episodeRows.Close()
				return CandidateSelection{}, false, queryErr
			}
			aliasID, _ := newPublicID("currentbinding_")
			if _, queryErr = tx.Exec(ctx, `INSERT INTO drama.artifact_current_bindings(
				artifact_current_binding_id,project_id,target_type,target_id,component_scope,current_artifact_id)
				VALUES($1,$2,'story_arc',$3,'whole',$4)
				ON CONFLICT(project_id,target_type,target_id,component_scope)
				DO UPDATE SET current_artifact_id=EXCLUDED.current_artifact_id,selected_at=CURRENT_TIMESTAMP`,
				aliasID, projectID, episodeID, artifactID); queryErr != nil {
				episodeRows.Close()
				return CandidateSelection{}, false, mapPGConflict(queryErr)
			}
		}
		if queryErr = episodeRows.Err(); queryErr != nil {
			episodeRows.Close()
			return CandidateSelection{}, false, queryErr
		}
		episodeRows.Close()
	}
	err = scanSelection(tx.QueryRow(ctx, selectionQuery+` WHERE selection.candidate_selection_id=$1`, selectionID), &replay)
	if err != nil {
		return CandidateSelection{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CandidateSelection{}, false, mapPGConflict(err)
	}
	return replay, true, nil
}

func (s *Store) AddCandidateTimecodeComment(ctx context.Context, candidateID, key string, input TimecodeCommentInput) (TimecodeComment, bool, error) {
	if input.TimecodeMS < 0 || input.CommentText == "" {
		return TimecodeComment{}, false, ErrConflict
	}
	var result TimecodeComment
	err := s.writer.QueryRow(ctx, `INSERT INTO drama.candidate_timecode_comments(
		candidate_timecode_comment_id,candidate_id,timecode_ms,comment_text,author,idempotency_key)
		SELECT $1,candidate_id,$2,$3,NULLIF($4,''),$5 FROM drama.candidates WHERE candidate_id=$6
		ON CONFLICT(idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key
		RETURNING candidate_timecode_comment_id,candidate_id,timecode_ms,comment_text,COALESCE(author,''),created_at`,
		mustPublicID("timecomment_"), input.TimecodeMS, input.CommentText, input.Author, key, candidateID).
		Scan(&result.CandidateTimecodeCommentID, &result.CandidateID, &result.TimecodeMS,
			&result.CommentText, &result.Author, &result.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TimecodeComment{}, false, ErrNotFound
	}
	if err != nil {
		return TimecodeComment{}, false, mapPGConflict(err)
	}
	// PostgreSQL cannot distinguish this no-op upsert without another round trip;
	// API semantics remain idempotent and the stable comment id proves replay.
	return result, true, nil
}

func getCandidateSetTx(ctx context.Context, tx pgx.Tx, projectID, candidateSetID string) (CandidateSet, error) {
	return getCandidateSetQuery(ctx, tx, projectID, candidateSetID)
}

type candidateQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func getCandidateSetQuery(ctx context.Context, queryer candidateQueryer, projectID, candidateSetID string) (CandidateSet, error) {
	var result CandidateSet
	err := queryer.QueryRow(ctx, `SELECT candidate_set_id,project_id,target_type,target_id,base_artifact_id,
		quality_score_report_id,candidate_count,component_types,difference_directions,must_preserve,
		allowed_changes,model,prompt_version,random_seed,generation_parameters,estimated_cost::float8,
		currency,generator_version,created_at,generator_provider,generator_model,reviewer_provider,
		reviewer_model,blind_review,frozen_resolution_id,frozen_context_hash,frozen_input_hash,frozen_input
		FROM drama.candidate_sets WHERE candidate_set_id=$1 AND project_id=$2`, candidateSetID, projectID).
		Scan(&result.CandidateSetID, &result.ProjectID, &result.TargetType, &result.TargetID, &result.BaseArtifactID,
			&result.QualityScoreReportID, &result.CandidateCount, &result.ComponentTypes,
			&result.DifferenceDirections, &result.MustPreserve, &result.AllowedChanges, &result.Model,
			&result.PromptVersion, &result.RandomSeed, &result.GenerationParameters, &result.EstimatedCost,
			&result.Currency, &result.GeneratorVersion, &result.CreatedAt, &result.GeneratorProvider,
			&result.GeneratorModel, &result.ReviewerProvider, &result.ReviewerModel, &result.BlindReview,
			&result.FrozenResolutionID, &result.FrozenContextHash, &result.FrozenInputHash, &result.FrozenInput)
	if errors.Is(err, pgx.ErrNoRows) {
		return CandidateSet{}, ErrNotFound
	}
	if err != nil {
		return CandidateSet{}, err
	}
	rows, err := queryer.Query(ctx, `WITH latest_preference AS (
		SELECT DISTINCT ON(candidate_id) candidate_id,decision FROM drama.candidate_decisions
		WHERE decision IN ('favorite','unfavorite')
		ORDER BY candidate_id,created_at DESC,id DESC
	), latest_elimination AS (
		SELECT DISTINCT ON(candidate_id) candidate_id,decision FROM drama.candidate_decisions
		WHERE decision IN ('eliminate','restore')
		ORDER BY candidate_id,created_at DESC,id DESC
	)
	SELECT candidate.candidate_id,candidate.candidate_set_id,candidate.parent_candidate_id,candidate.artifact_id,
		candidate.ordinal,candidate.label,candidate.difference_direction,candidate.derived_reason,candidate.content,
		candidate.structured_diff,candidate.content_hash,candidate.model,candidate.prompt_version,candidate.random_seed,
		candidate.generation_parameters,candidate.provider,score.total_score::float8,score.fidelity::float8,
		score.causality::float8,score.character_consistency::float8,score.hook::float8,
		score.pacing::float8,score.continuity::float8,score.filmability::float8,
		score.estimated_duration_seconds,score.modification_risk::float8,score.recommendation_reasons,
		score.deduction_reasons,score.dimensions,score.reviewer_provider,score.reviewer_model,
		COALESCE(preference.decision='favorite',false),
		COALESCE(elimination.decision='eliminate',false),candidate.created_at
	FROM drama.candidates candidate JOIN drama.candidate_scores score USING(candidate_id)
	LEFT JOIN latest_preference preference USING(candidate_id)
	LEFT JOIN latest_elimination elimination USING(candidate_id)
	WHERE candidate.candidate_set_id=$1
	ORDER BY score.total_score DESC,candidate.ordinal`, candidateSetID)
	if err != nil {
		return CandidateSet{}, err
	}
	defer rows.Close()
	rank := 0
	for rows.Next() {
		rank++
		var candidate CandidateVersion
		candidate.Rank = rank
		if err := rows.Scan(&candidate.CandidateID, &candidate.CandidateSetID, &candidate.ParentCandidateID,
			&candidate.ArtifactID, &candidate.Ordinal, &candidate.Label, &candidate.DifferenceDirection,
			&candidate.DerivedReason, &candidate.Content, &candidate.StructuredDiff, &candidate.ContentHash,
			&candidate.Model, &candidate.PromptVersion, &candidate.RandomSeed, &candidate.GenerationParameters,
			&candidate.Provider, &candidate.Score.TotalScore, &candidate.Score.Fidelity,
			&candidate.Score.Causality, &candidate.Score.CharacterConsistency, &candidate.Score.Hook,
			&candidate.Score.Pacing, &candidate.Score.Continuity, &candidate.Score.Filmability,
			&candidate.Score.EstimatedDurationSeconds, &candidate.Score.ModificationRisk,
			&candidate.Score.RecommendationReasons, &candidate.Score.DeductionReasons, &candidate.Score.Dimensions,
			&candidate.Score.ReviewerProvider, &candidate.Score.ReviewerModel,
			&candidate.IsFavorite, &candidate.IsEliminated, &candidate.CreatedAt); err != nil {
			return CandidateSet{}, err
		}
		result.Candidates = append(result.Candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return CandidateSet{}, err
	}
	return result, nil
}

const selectionQuery = `SELECT selection.candidate_selection_id,selection.candidate_set_id,
	selection.selected_candidate_id,selection.artifact_id,selection.selection_type,selection.content,
	selection.validation_summary,COALESCE(selection.confirmed_by,''),selection.created_at
	FROM drama.candidate_selections selection`

func scanSelection(row pgx.Row, result *CandidateSelection) error {
	return row.Scan(&result.CandidateSelectionID, &result.CandidateSetID, &result.SelectedCandidateID,
		&result.ArtifactID, &result.SelectionType, &result.Content, &result.ValidationSummary,
		&result.ConfirmedBy, &result.CreatedAt)
}

func matchesCandidateDecision(value string) bool {
	return value == "favorite" || value == "unfavorite" || value == "eliminate" || value == "restore"
}

func estimateCandidateCost(targetType string, count, components int) float64 {
	unit := .006
	if targetType == "image" {
		unit = .12
	} else if targetType == "video" {
		unit = .85
	}
	return float64(count*components) * unit
}

func defaultCandidatePrompt(value string) string {
	if value == "" {
		return candidategeneration.PromptVersion
	}
	return value
}

func adaptationScoreVersion() string { return "phase1-quality-compatible-v1" }

func applyPhase1QualityBaseline(candidates []candidategeneration.Candidate, dimensions map[string]float64) {
	blend := func(candidate, baseline float64) float64 {
		if baseline <= 0 {
			return candidate
		}
		return math.Round((candidate*.6+baseline*.4)*100) / 100
	}
	filmabilityBaseline := dimensions["视觉可执行性"]
	if soundVisual := dimensions["声画可执行性"]; filmabilityBaseline > 0 && soundVisual > 0 {
		filmabilityBaseline = (filmabilityBaseline + soundVisual) / 2
	} else if filmabilityBaseline == 0 {
		filmabilityBaseline = soundVisual
	}
	for index := range candidates {
		score := &candidates[index].Score
		score.Fidelity = blend(score.Fidelity, dimensions["原著忠实度"])
		score.Hook = blend(score.Hook, dimensions["钩子强度"])
		score.Pacing = blend(score.Pacing, dimensions["节奏密度"])
		score.Continuity = blend(score.Continuity, dimensions["连续性"])
		score.Filmability = blend(score.Filmability, filmabilityBaseline)
		score.TotalScore = math.Round((score.Fidelity*.25+score.Hook*.18+score.Pacing*.18+
			score.Continuity*.16+score.Filmability*.15+(100-score.ModificationRisk)*.08)*100) / 100
		score.RecommendationReasons = append([]string{"已按第一阶段质量维度校准"}, score.RecommendationReasons...)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score.TotalScore == candidates[j].Score.TotalScore {
			return candidates[i].Ordinal < candidates[j].Ordinal
		}
		return candidates[i].Score.TotalScore > candidates[j].Score.TotalScore
	})
}

func defaultJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}

func mustPublicID(prefix string) string {
	id, _ := newPublicID(prefix)
	return id
}

func sortedCandidateIDs(parts []CandidateCompositionPartInput) []string {
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, part.CandidateID)
	}
	sort.Strings(result)
	return result
}
