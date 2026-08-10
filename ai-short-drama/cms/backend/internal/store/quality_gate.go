package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/qualitygate"
)

type QualityGateRecord struct {
	GateRunID           string                `json:"gate_run_id"`
	SchemaVersion       string                `json:"schema_version"`
	ProjectID           string                `json:"project_id"`
	EpisodeID           string                `json:"episode_id"`
	MasterID            *string               `json:"master_id,omitempty"`
	RulesetVersion      string                `json:"ruleset_version"`
	RulesConfig         qualitygate.Config    `json:"rules_config"`
	PromptVersion       *string               `json:"prompt_version,omitempty"`
	RuleScore           float64               `json:"rule_score"`
	RulesStatus         string                `json:"rules_status"`
	ModelReviewRequired bool                  `json:"model_review_required"`
	ModelStatus         string                `json:"model_status"`
	Status              string                `json:"status"`
	Findings            []qualitygate.Finding `json:"findings"`
	Approval            *QualityGateApproval  `json:"approval,omitempty"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
}

type QualityGateApproval struct {
	GateApprovalID string    `json:"gate_approval_id"`
	GateRunID      string    `json:"gate_run_id"`
	MasterID       string    `json:"master_id"`
	ApprovedBy     string    `json:"approved_by"`
	Status         string    `json:"status"`
	ApprovedAt     time.Time `json:"approved_at"`
}

type QualityGateOverride struct {
	OverrideID string    `json:"override_id"`
	GateRunID  string    `json:"gate_run_id"`
	FindingID  string    `json:"finding_id"`
	Reason     string    `json:"reason"`
	AcceptedBy string    `json:"accepted_by"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Store) SaveQualityGateRuleRun(ctx context.Context, snapshot qualitygate.Snapshot, run qualitygate.Run, actor string) (QualityGateRecord, error) {
	if err := snapshot.Validate(); err != nil {
		return QualityGateRecord{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if run.ProjectID != snapshot.ProjectID || run.EpisodeID != snapshot.EpisodeID || run.GateRunID == "" {
		return QualityGateRecord{}, fmt.Errorf("%w: rule result does not match snapshot", ErrValidation)
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return QualityGateRecord{}, err
	}
	hash := sha256.Sum256(snapshotJSON)
	snapshotHash := hex.EncodeToString(hash[:])
	rulesConfigJSON, err := json.Marshal(run.RulesConfig)
	if err != nil {
		return QualityGateRecord{}, err
	}
	rulesConfigHashBytes := sha256.Sum256(rulesConfigJSON)
	rulesConfigHash := hex.EncodeToString(rulesConfigHashBytes[:])
	modelStatus, status := "pending", "review_pending"
	if !run.ModelReviewRequired {
		modelStatus, status = "not_required", "review_ready"
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return QualityGateRecord{}, err
	}
	defer tx.Rollback(ctx)
	var episodeExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.episode_outlines WHERE project_id=$1 AND episode_id=$2)`,
		snapshot.ProjectID, snapshot.EpisodeID).Scan(&episodeExists); err != nil {
		return QualityGateRecord{}, err
	}
	if !episodeExists {
		return QualityGateRecord{}, fmt.Errorf("%w: episode does not belong to project", ErrValidation)
	}
	if snapshot.MasterID != "" {
		var masterExists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.episode_masters
			WHERE master_id=$1 AND project_id=$2 AND episode_id=$3)`, snapshot.MasterID,
			snapshot.ProjectID, snapshot.EpisodeID).Scan(&masterExists); err != nil {
			return QualityGateRecord{}, err
		}
		if !masterExists {
			return QualityGateRecord{}, fmt.Errorf("%w: master does not belong to project and episode", ErrValidation)
		}
	}
	command, err := tx.Exec(ctx, `INSERT INTO drama.quality_gate_runs(
		gate_run_id,project_id,episode_id,master_id,ruleset_version,rules_config,rules_config_hash,snapshot,snapshot_hash,
		rule_score,rules_status,model_review_required,model_status,status,created_by)
		VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,'completed',$11,$12,$13,NULLIF($14,''))
		ON CONFLICT(gate_run_id) DO NOTHING`, run.GateRunID, snapshot.ProjectID, snapshot.EpisodeID,
		snapshot.MasterID, run.RulesetVersion, rulesConfigJSON, rulesConfigHash, snapshotJSON, snapshotHash, run.RuleScore,
		run.ModelReviewRequired, modelStatus, status, strings.TrimSpace(actor))
	if err != nil {
		return QualityGateRecord{}, err
	}
	if command.RowsAffected() > 0 {
		if snapshot.MasterID != "" {
			if _, err = tx.Exec(ctx, `UPDATE drama.quality_gate_master_approvals approval SET status='revoked',revoked_at=now()
				FROM drama.quality_gate_runs old_run WHERE approval.gate_run_id=old_run.gate_run_id
				AND approval.master_id=$1 AND approval.status='active' AND old_run.gate_run_id<>$2`, snapshot.MasterID, run.GateRunID); err != nil {
				return QualityGateRecord{}, err
			}
			if _, err = tx.Exec(ctx, `UPDATE drama.quality_gate_runs SET status='superseded'
				WHERE master_id=$1 AND gate_run_id<>$2 AND status IN('review_pending','review_ready','approved')`, snapshot.MasterID, run.GateRunID); err != nil {
				return QualityGateRecord{}, err
			}
		}
		for _, finding := range run.Findings {
			if err = qualitygate.ValidateFinding(finding); err != nil {
				return QualityGateRecord{}, fmt.Errorf("%w: %v", ErrValidation, err)
			}
			evidence, _ := json.Marshal(finding.Evidence)
			locators, _ := json.Marshal(finding.Locators)
			metadata := finding.Metadata
			if len(metadata) == 0 {
				metadata = json.RawMessage(`{}`)
			}
			if _, err = tx.Exec(ctx, `INSERT INTO drama.quality_gate_findings(
				gate_run_id,finding_id,detector_type,dimension,code,severity,message,evidence,locators,recommendation,status,detector_metadata)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'open',$11)`, run.GateRunID, finding.FindingID,
				finding.DetectorType, finding.Dimension, finding.Code, finding.Severity, finding.Message,
				evidence, locators, finding.Recommendation, metadata); err != nil {
				return QualityGateRecord{}, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return QualityGateRecord{}, err
	}
	return s.GetQualityGateRun(ctx, snapshot.ProjectID, snapshot.EpisodeID, run.GateRunID)
}

func (s *Store) SaveQualityGateModelReview(ctx context.Context, projectID, episodeID, runID string, review qualitygate.ModelReview) (QualityGateRecord, error) {
	for index := range review.Findings {
		review.Findings[index].DetectorType = qualitygate.DetectorModel
		if review.Findings[index].Status == "" {
			review.Findings[index].Status = qualitygate.FindingOpen
		}
	}
	if err := qualitygate.ValidateModelReview(review); err != nil {
		return QualityGateRecord{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return QualityGateRecord{}, err
	}
	defer tx.Rollback(ctx)
	var status, modelStatus string
	var snapshotJSON []byte
	err = tx.QueryRow(ctx, `SELECT status,model_status,snapshot FROM drama.quality_gate_runs
		WHERE gate_run_id=$1 AND project_id=$2 AND episode_id=$3 FOR UPDATE`, runID, projectID, episodeID).Scan(&status, &modelStatus, &snapshotJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return QualityGateRecord{}, ErrNotFound
	}
	if err != nil {
		return QualityGateRecord{}, err
	}
	if status == "approved" || status == "superseded" || modelStatus != "pending" {
		return QualityGateRecord{}, fmt.Errorf("%w: model review is not pending", ErrConflict)
	}
	var snapshot qualitygate.Snapshot
	if err = json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return QualityGateRecord{}, err
	}
	if err = qualitygate.ValidateModelReviewAgainstSnapshot(review, snapshot); err != nil {
		return QualityGateRecord{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	for _, finding := range review.Findings {
		evidence, _ := json.Marshal(finding.Evidence)
		locators, _ := json.Marshal(finding.Locators)
		metadata, _ := json.Marshal(map[string]any{"provider": review.Provider, "model": review.Model,
			"prompt_version": review.PromptVersion, "review_metadata": json.RawMessage(defaultJSONObject(finding.Metadata))})
		if _, err = tx.Exec(ctx, `INSERT INTO drama.quality_gate_findings(
			gate_run_id,finding_id,detector_type,dimension,code,severity,message,evidence,locators,recommendation,status,detector_metadata)
			VALUES($1,$2,'model',$3,$4,$5,$6,$7,$8,$9,'open',$10)`, runID, finding.FindingID,
			finding.Dimension, finding.Code, finding.Severity, finding.Message, evidence, locators,
			finding.Recommendation, metadata); err != nil {
			return QualityGateRecord{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.quality_gate_runs SET model_status='completed',prompt_version=$2,status='review_ready'
		WHERE gate_run_id=$1`, runID, review.PromptVersion); err != nil {
		return QualityGateRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return QualityGateRecord{}, err
	}
	return s.GetQualityGateRun(ctx, projectID, episodeID, runID)
}

func (s *Store) GetQualityGateRun(ctx context.Context, projectID, episodeID, runID string) (QualityGateRecord, error) {
	var result QualityGateRecord
	err := s.pool.QueryRow(ctx, `SELECT gate_run_id,schema_version,project_id,episode_id,master_id,
		ruleset_version,rules_config,prompt_version,rule_score,rules_status,model_review_required,model_status,status,created_at,updated_at
		FROM drama.quality_gate_runs WHERE gate_run_id=$1 AND project_id=$2 AND episode_id=$3`, runID, projectID, episodeID).Scan(
		&result.GateRunID, &result.SchemaVersion, &result.ProjectID, &result.EpisodeID, &result.MasterID,
		&result.RulesetVersion, &result.RulesConfig, &result.PromptVersion, &result.RuleScore, &result.RulesStatus,
		&result.ModelReviewRequired, &result.ModelStatus, &result.Status, &result.CreatedAt, &result.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return QualityGateRecord{}, ErrNotFound
	}
	if err != nil {
		return QualityGateRecord{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT finding_id,schema_version,detector_type,dimension,code,severity,
		message,evidence,locators,recommendation,status,detector_metadata
		FROM drama.quality_gate_findings WHERE gate_run_id=$1
		ORDER BY CASE severity WHEN 'blocking' THEN 0 WHEN 'major' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END,created_at,finding_id`, runID)
	if err != nil {
		return QualityGateRecord{}, err
	}
	defer rows.Close()
	result.Findings = []qualitygate.Finding{}
	for rows.Next() {
		var finding qualitygate.Finding
		var evidenceJSON, locatorsJSON []byte
		if err = rows.Scan(&finding.FindingID, &finding.SchemaVersion, &finding.DetectorType, &finding.Dimension,
			&finding.Code, &finding.Severity, &finding.Message, &evidenceJSON, &locatorsJSON,
			&finding.Recommendation, &finding.Status, &finding.Metadata); err != nil {
			return QualityGateRecord{}, err
		}
		if err = json.Unmarshal(evidenceJSON, &finding.Evidence); err != nil {
			return QualityGateRecord{}, err
		}
		if err = json.Unmarshal(locatorsJSON, &finding.Locators); err != nil {
			return QualityGateRecord{}, err
		}
		result.Findings = append(result.Findings, finding)
	}
	if err = rows.Err(); err != nil {
		return QualityGateRecord{}, err
	}
	var approval QualityGateApproval
	err = s.pool.QueryRow(ctx, `SELECT gate_approval_id,gate_run_id,master_id,approved_by,status,approved_at
		FROM drama.quality_gate_master_approvals WHERE gate_run_id=$1 ORDER BY approved_at DESC LIMIT 1`, runID).Scan(
		&approval.GateApprovalID, &approval.GateRunID, &approval.MasterID, &approval.ApprovedBy, &approval.Status, &approval.ApprovedAt)
	if err == nil {
		result.Approval = &approval
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return QualityGateRecord{}, err
	}
	return result, nil
}

func (s *Store) OverrideQualityGateFinding(ctx context.Context, projectID, episodeID, runID, findingID, reason, actor string) (QualityGateOverride, error) {
	reason, actor = strings.TrimSpace(reason), strings.TrimSpace(actor)
	if reason == "" || actor == "" {
		return QualityGateOverride{}, fmt.Errorf("%w: override reason and actor are required", ErrValidation)
	}
	digest := sha256.Sum256([]byte(runID + ":" + findingID + ":" + reason + ":" + actor))
	overrideID := "qgo_" + hex.EncodeToString(digest[:])[:24]
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return QualityGateOverride{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT finding.status FROM drama.quality_gate_findings finding
		JOIN drama.quality_gate_runs run USING(gate_run_id)
		WHERE finding.gate_run_id=$1 AND finding.finding_id=$2 AND run.project_id=$3 AND run.episode_id=$4 FOR UPDATE OF finding`,
		runID, findingID, projectID, episodeID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return QualityGateOverride{}, ErrNotFound
	}
	if err != nil {
		return QualityGateOverride{}, err
	}
	if status != qualitygate.FindingOpen {
		return QualityGateOverride{}, fmt.Errorf("%w: only open findings can be overridden", ErrConflict)
	}
	var result QualityGateOverride
	err = tx.QueryRow(ctx, `INSERT INTO drama.quality_gate_overrides(override_id,gate_run_id,finding_id,reason,accepted_by)
		VALUES($1,$2,$3,$4,$5) RETURNING override_id,gate_run_id,finding_id,reason,accepted_by,status,created_at`,
		overrideID, runID, findingID, reason, actor).Scan(&result.OverrideID, &result.GateRunID, &result.FindingID,
		&result.Reason, &result.AcceptedBy, &result.Status, &result.CreatedAt)
	if err != nil {
		return QualityGateOverride{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.quality_gate_findings SET status='overridden',resolved_by=$3,
		resolution_reason=$4,resolved_at=now() WHERE gate_run_id=$1 AND finding_id=$2`, runID, findingID, actor, reason); err != nil {
		return QualityGateOverride{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return QualityGateOverride{}, err
	}
	return result, nil
}

func (s *Store) ResolveQualityGateFinding(ctx context.Context, projectID, episodeID, runID, findingID, reason, actor string) (QualityGateRecord, error) {
	reason, actor = strings.TrimSpace(reason), strings.TrimSpace(actor)
	if reason == "" || actor == "" {
		return QualityGateRecord{}, fmt.Errorf("%w: resolution reason and actor are required", ErrValidation)
	}
	command, err := s.writer.Exec(ctx, `UPDATE drama.quality_gate_findings finding SET status='resolved',resolved_by=$5,
		resolution_reason=$6,resolved_at=now() FROM drama.quality_gate_runs run
		WHERE finding.gate_run_id=$1 AND finding.finding_id=$2 AND run.gate_run_id=finding.gate_run_id
		AND run.project_id=$3 AND run.episode_id=$4 AND finding.status='open' AND run.status<>'approved'
		AND EXISTS(SELECT 1 FROM drama.quality_gate_change_plans plan
		  WHERE plan.gate_run_id=finding.gate_run_id AND plan.finding_id=finding.finding_id
		    AND plan.status IN('proposed','confirmed','executed'))`,
		runID, findingID, projectID, episodeID, actor, reason)
	if err != nil {
		return QualityGateRecord{}, err
	}
	if command.RowsAffected() == 0 {
		return QualityGateRecord{}, fmt.Errorf("%w: finding is missing, closed, gate is approved, or local change plan is absent", ErrConflict)
	}
	return s.GetQualityGateRun(ctx, projectID, episodeID, runID)
}

func (s *Store) CreateQualityGateChangePlan(ctx context.Context, projectID, episodeID, runID, findingID, actor string) (qualitygate.ChangePlan, error) {
	finding, err := s.getQualityGateFinding(ctx, projectID, episodeID, runID, findingID)
	if err != nil {
		return qualitygate.ChangePlan{}, err
	}
	plan, err := qualitygate.BuildLocalChangePlan(finding)
	if err != nil {
		return qualitygate.ChangePlan{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	planJSON, _ := json.Marshal(plan)
	_, err = s.writer.Exec(ctx, `INSERT INTO drama.quality_gate_change_plans(change_plan_id,gate_run_id,finding_id,plan,requested_by)
		VALUES($1,$2,$3,$4,NULLIF($5,'')) ON CONFLICT(gate_run_id,finding_id) DO NOTHING`,
		plan.ChangePlanID, runID, findingID, planJSON, strings.TrimSpace(actor))
	return plan, err
}

func (s *Store) ApproveQualityGateMaster(ctx context.Context, projectID, episodeID, runID, actor string) (QualityGateApproval, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return QualityGateApproval{}, fmt.Errorf("%w: approving actor is required", ErrValidation)
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return QualityGateApproval{}, err
	}
	defer tx.Rollback(ctx)
	var masterID, rulesStatus, modelStatus, status string
	err = tx.QueryRow(ctx, `SELECT COALESCE(master_id,''),rules_status,model_status,status FROM drama.quality_gate_runs
		WHERE gate_run_id=$1 AND project_id=$2 AND episode_id=$3 FOR UPDATE`, runID, projectID, episodeID).Scan(&masterID, &rulesStatus, &modelStatus, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return QualityGateApproval{}, ErrNotFound
	}
	if err != nil {
		return QualityGateApproval{}, err
	}
	if masterID == "" || rulesStatus != "completed" || (modelStatus != "completed" && modelStatus != "not_required") || status != "review_ready" {
		return QualityGateApproval{}, fmt.Errorf("%w: rule/model reviews are incomplete or run is not approvable", ErrConflict)
	}
	var blockers int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM drama.quality_gate_findings
		WHERE gate_run_id=$1 AND severity='blocking' AND status='open'`, runID).Scan(&blockers); err != nil {
		return QualityGateApproval{}, err
	}
	if blockers > 0 {
		return QualityGateApproval{}, fmt.Errorf("%w: %d blocking findings remain open", ErrConflict, blockers)
	}
	digest := sha256.Sum256([]byte(runID + ":" + masterID))
	approvalID := "qga_" + hex.EncodeToString(digest[:])[:24]
	var result QualityGateApproval
	err = tx.QueryRow(ctx, `INSERT INTO drama.quality_gate_master_approvals(
		gate_approval_id,gate_run_id,project_id,episode_id,master_id,approved_by)
		VALUES($1,$2,$3,$4,$5,$6)
		RETURNING gate_approval_id,gate_run_id,master_id,approved_by,status,approved_at`,
		approvalID, runID, projectID, episodeID, masterID, actor).Scan(&result.GateApprovalID,
		&result.GateRunID, &result.MasterID, &result.ApprovedBy, &result.Status, &result.ApprovedAt)
	if err != nil {
		return QualityGateApproval{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.quality_gate_runs SET status='approved' WHERE gate_run_id=$1`, runID); err != nil {
		return QualityGateApproval{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return QualityGateApproval{}, err
	}
	return result, nil
}

func (s *Store) getQualityGateFinding(ctx context.Context, projectID, episodeID, runID, findingID string) (qualitygate.Finding, error) {
	var finding qualitygate.Finding
	var evidenceJSON, locatorsJSON []byte
	err := s.pool.QueryRow(ctx, `SELECT finding.finding_id,finding.schema_version,finding.detector_type,finding.dimension,
		finding.code,finding.severity,finding.message,finding.evidence,finding.locators,finding.recommendation,finding.status,finding.detector_metadata
		FROM drama.quality_gate_findings finding JOIN drama.quality_gate_runs run USING(gate_run_id)
		WHERE finding.gate_run_id=$1 AND finding.finding_id=$2 AND run.project_id=$3 AND run.episode_id=$4`,
		runID, findingID, projectID, episodeID).Scan(&finding.FindingID, &finding.SchemaVersion, &finding.DetectorType,
		&finding.Dimension, &finding.Code, &finding.Severity, &finding.Message, &evidenceJSON, &locatorsJSON,
		&finding.Recommendation, &finding.Status, &finding.Metadata)
	if errors.Is(err, pgx.ErrNoRows) {
		return qualitygate.Finding{}, ErrNotFound
	}
	if err != nil {
		return qualitygate.Finding{}, err
	}
	if err = json.Unmarshal(evidenceJSON, &finding.Evidence); err != nil {
		return qualitygate.Finding{}, err
	}
	if err = json.Unmarshal(locatorsJSON, &finding.Locators); err != nil {
		return qualitygate.Finding{}, err
	}
	return finding, nil
}

func defaultJSONObject(value json.RawMessage) []byte {
	if len(value) == 0 || !json.Valid(value) {
		return []byte(`{}`)
	}
	return value
}
