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

	"short-drama-cms/backend/internal/promptlab"
)

type PromptTemplate struct {
	PromptTemplateID          string          `json:"prompt_template_id"`
	Category                  string          `json:"category"`
	PromptKey                 string          `json:"prompt_key"`
	DisplayName               string          `json:"display_name"`
	Description               string          `json:"description"`
	ProductionPromptVersionID *string         `json:"production_prompt_version_id,omitempty"`
	Versions                  []PromptVersion `json:"versions"`
	CreatedAt                 time.Time       `json:"created_at"`
	UpdatedAt                 time.Time       `json:"updated_at"`
}

type PromptVersion struct {
	PromptVersionID  string          `json:"prompt_version_id"`
	PromptTemplateID string          `json:"prompt_template_id"`
	Version          int             `json:"version"`
	SystemTemplate   string          `json:"system_template"`
	UserTemplate     string          `json:"user_template"`
	VariableSchema   json.RawMessage `json:"variable_schema"`
	DefaultVariables json.RawMessage `json:"default_variables"`
	ModelDefaults    json.RawMessage `json:"model_defaults"`
	ChangeNote       string          `json:"change_note"`
	ContentHash      string          `json:"content_hash"`
	Status           string          `json:"status"`
	IsProduction     bool            `json:"is_production"`
	CreatedBy        *string         `json:"created_by,omitempty"`
	ApprovedBy       *string         `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time      `json:"approved_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

type CreatePromptTemplateInput struct {
	Category    string `json:"category"`
	PromptKey   string `json:"prompt_key"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	CreatedBy   string `json:"created_by"`
}

type CreatePromptVersionInput struct {
	SystemTemplate   string          `json:"system_template"`
	UserTemplate     string          `json:"user_template"`
	VariableSchema   json.RawMessage `json:"variable_schema"`
	DefaultVariables json.RawMessage `json:"default_variables"`
	ModelDefaults    json.RawMessage `json:"model_defaults"`
	ChangeNote       string          `json:"change_note"`
	CreatedBy        string          `json:"created_by"`
}

type PromptFixture struct {
	PromptFixtureID string          `json:"prompt_fixture_id"`
	Category        string          `json:"category"`
	FixtureKey      string          `json:"fixture_key"`
	Version         int             `json:"version"`
	DisplayName     string          `json:"display_name"`
	Variables       json.RawMessage `json:"variables"`
	ExpectedOutput  json.RawMessage `json:"expected_output,omitempty"`
	InputHash       string          `json:"input_hash"`
	Frozen          bool            `json:"frozen"`
	CreatedAt       time.Time       `json:"created_at"`
}

type CreatePromptFixtureInput struct {
	Category       string          `json:"category"`
	FixtureKey     string          `json:"fixture_key"`
	DisplayName    string          `json:"display_name"`
	Variables      json.RawMessage `json:"variables"`
	ExpectedOutput json.RawMessage `json:"expected_output"`
	CreatedBy      string          `json:"created_by"`
}

type PromptTestSuite struct {
	PromptTestSuiteID string          `json:"prompt_test_suite_id"`
	Category          string          `json:"category"`
	DisplayName       string          `json:"display_name"`
	Version           int             `json:"version"`
	FixtureIDs        []string        `json:"fixture_ids"`
	MetricConfig      json.RawMessage `json:"metric_config"`
	SuiteHash         string          `json:"suite_hash"`
	Frozen            bool            `json:"frozen"`
	CreatedAt         time.Time       `json:"created_at"`
}

type CreatePromptTestSuiteInput struct {
	Category     string          `json:"category"`
	DisplayName  string          `json:"display_name"`
	FixtureIDs   []string        `json:"fixture_ids"`
	MetricConfig json.RawMessage `json:"metric_config"`
	CreatedBy    string          `json:"created_by"`
}

type PromptExperimentVariantInput struct {
	PromptVersionID string          `json:"prompt_version_id"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model"`
	Parameters      json.RawMessage `json:"parameters"`
	Seed            *int64          `json:"seed,omitempty"`
}

type CreatePromptExperimentInput struct {
	Category          string                         `json:"category"`
	DisplayName       string                         `json:"display_name"`
	PromptTestSuiteID string                         `json:"prompt_test_suite_id"`
	BlindReview       bool                           `json:"blind_review"`
	Variants          []PromptExperimentVariantInput `json:"variants"`
	CreatedBy         string                         `json:"created_by"`
}

type PromptExperimentVariant struct {
	PromptExperimentVariantID string          `json:"prompt_experiment_variant_id"`
	PromptVersionID           string          `json:"prompt_version_id,omitempty"`
	Provider                  string          `json:"provider,omitempty"`
	Model                     string          `json:"model,omitempty"`
	Parameters                json.RawMessage `json:"parameters,omitempty"`
	Seed                      *int64          `json:"seed,omitempty"`
	BlindLabel                string          `json:"blind_label"`
}

type PromptExperimentResult struct {
	PromptExperimentResultID  string          `json:"prompt_experiment_result_id"`
	PromptExperimentVariantID string          `json:"prompt_experiment_variant_id"`
	PromptFixtureID           string          `json:"prompt_fixture_id"`
	BlindLabel                string          `json:"blind_label"`
	RenderedInput             string          `json:"rendered_input"`
	RenderedInputHash         string          `json:"rendered_input_hash"`
	Output                    json.RawMessage `json:"output"`
	OutputHash                string          `json:"output_hash"`
	TokenEstimate             int             `json:"token_estimate"`
	TokenUsage                json.RawMessage `json:"token_usage"`
	LatencyMS                 *int            `json:"latency_ms,omitempty"`
	EstimatedCost             float64         `json:"estimated_cost"`
	AutomaticMetrics          json.RawMessage `json:"automatic_metrics"`
	Status                    string          `json:"status"`
	ErrorMessage              *string         `json:"error_message,omitempty"`
}

type PromptBlindEvaluation struct {
	PromptBlindEvaluationID string          `json:"prompt_blind_evaluation_id"`
	PromptFixtureID         string          `json:"prompt_fixture_id"`
	BlindLabel              string          `json:"blind_label"`
	Reviewer                string          `json:"reviewer"`
	Score                   float64         `json:"score"`
	RubricScores            json.RawMessage `json:"rubric_scores"`
	Comment                 string          `json:"comment"`
	CreatedAt               time.Time       `json:"created_at"`
}

type PromptExperiment struct {
	PromptExperimentID string                    `json:"prompt_experiment_id"`
	Category           string                    `json:"category"`
	DisplayName        string                    `json:"display_name"`
	PromptTestSuiteID  string                    `json:"prompt_test_suite_id"`
	SuiteHash          string                    `json:"suite_hash"`
	BlindReview        bool                      `json:"blind_review"`
	Status             string                    `json:"status"`
	Variants           []PromptExperimentVariant `json:"variants"`
	Results            []PromptExperimentResult  `json:"results"`
	Evaluations        []PromptBlindEvaluation   `json:"evaluations"`
	CreatedAt          time.Time                 `json:"created_at"`
}

type SavePromptExperimentResultInput struct {
	PromptExperimentVariantID string          `json:"prompt_experiment_variant_id"`
	PromptFixtureID           string          `json:"prompt_fixture_id"`
	RenderedInput             string          `json:"rendered_input"`
	RenderedInputHash         string          `json:"rendered_input_hash"`
	Output                    json.RawMessage `json:"output"`
	TokenEstimate             int             `json:"token_estimate"`
	TokenUsage                json.RawMessage `json:"token_usage"`
	LatencyMS                 *int            `json:"latency_ms"`
	EstimatedCost             float64         `json:"estimated_cost"`
	AutomaticMetrics          json.RawMessage `json:"automatic_metrics"`
	Status                    string          `json:"status"`
	ErrorMessage              string          `json:"error_message"`
}

type SaveBlindEvaluationInput struct {
	PromptFixtureID string          `json:"prompt_fixture_id"`
	BlindLabel      string          `json:"blind_label"`
	Reviewer        string          `json:"reviewer"`
	Score           float64         `json:"score"`
	RubricScores    json.RawMessage `json:"rubric_scores"`
	Comment         string          `json:"comment"`
}

type ArtifactProvenance struct {
	GenerationProvenanceID string          `json:"generation_provenance_id"`
	ProjectID              string          `json:"project_id"`
	EpisodeID              *string         `json:"episode_id,omitempty"`
	ArtifactType           string          `json:"artifact_type"`
	ArtifactID             string          `json:"artifact_id"`
	ArtifactVersion        int             `json:"artifact_version"`
	PromptVersionID        string          `json:"prompt_version_id"`
	Provider               string          `json:"provider"`
	Model                  string          `json:"model"`
	Parameters             json.RawMessage `json:"parameters"`
	Seed                   *int64          `json:"seed,omitempty"`
	InputArtifactHash      string          `json:"input_artifact_hash"`
	OutputArtifactHash     string          `json:"output_artifact_hash"`
	SourceArtifacts        json.RawMessage `json:"source_artifacts"`
	CreatedAt              time.Time       `json:"created_at"`
}

func (s *Store) ListPromptTemplates(ctx context.Context, category string) ([]PromptTemplate, error) {
	query := `SELECT template.prompt_template_id,template.category,template.prompt_key,template.display_name,
		template.description,current_binding.prompt_version_id,template.created_at,template.updated_at
		FROM drama.prompt_templates template
		LEFT JOIN drama.prompt_production_bindings current_binding
		  ON current_binding.prompt_template_id=template.prompt_template_id AND current_binding.is_current
		WHERE ($1='' OR template.category=$1) ORDER BY template.category,template.display_name`
	rows, err := s.pool.Query(ctx, query, strings.TrimSpace(category))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PromptTemplate{}
	for rows.Next() {
		var item PromptTemplate
		if err = rows.Scan(&item.PromptTemplateID, &item.Category, &item.PromptKey, &item.DisplayName,
			&item.Description, &item.ProductionPromptVersionID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Versions, err = s.ListPromptVersions(ctx, item.PromptTemplateID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreatePromptTemplate(ctx context.Context, input CreatePromptTemplateInput) (PromptTemplate, error) {
	input.Category, input.PromptKey, input.DisplayName = strings.TrimSpace(input.Category), strings.TrimSpace(input.PromptKey), strings.TrimSpace(input.DisplayName)
	if !promptlab.ValidCategory(input.Category) || input.PromptKey == "" || input.DisplayName == "" {
		return PromptTemplate{}, fmt.Errorf("%w: valid category, prompt_key and display_name are required", ErrValidation)
	}
	id, err := newPublicID("pt_")
	if err != nil {
		return PromptTemplate{}, err
	}
	_, err = s.writer.Exec(ctx, `INSERT INTO drama.prompt_templates(prompt_template_id,category,prompt_key,display_name,description,created_by)
		VALUES($1,$2,$3,$4,$5,NULLIF($6,''))`, id, input.Category, input.PromptKey, input.DisplayName,
		strings.TrimSpace(input.Description), strings.TrimSpace(input.CreatedBy))
	if err != nil {
		return PromptTemplate{}, err
	}
	items, err := s.ListPromptTemplates(ctx, input.Category)
	if err != nil {
		return PromptTemplate{}, err
	}
	for _, item := range items {
		if item.PromptTemplateID == id {
			return item, nil
		}
	}
	return PromptTemplate{}, ErrNotFound
}

func (s *Store) ListPromptVersions(ctx context.Context, templateID string) ([]PromptVersion, error) {
	rows, err := s.pool.Query(ctx, `SELECT version.prompt_version_id,version.prompt_template_id,version.version,
		version.system_template,version.user_template,version.variable_schema,version.default_variables,
		version.model_defaults,version.change_note,version.content_hash,version.status,
		EXISTS(SELECT 1 FROM drama.prompt_production_bindings binding WHERE binding.prompt_version_id=version.prompt_version_id AND binding.is_current),
		version.created_by,version.approved_by,version.approved_at,version.created_at
		FROM drama.prompt_versions version WHERE version.prompt_template_id=$1 ORDER BY version.version DESC`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PromptVersion{}
	for rows.Next() {
		var item PromptVersion
		if err = rows.Scan(&item.PromptVersionID, &item.PromptTemplateID, &item.Version, &item.SystemTemplate,
			&item.UserTemplate, &item.VariableSchema, &item.DefaultVariables, &item.ModelDefaults,
			&item.ChangeNote, &item.ContentHash, &item.Status, &item.IsProduction, &item.CreatedBy,
			&item.ApprovedBy, &item.ApprovedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetPromptVersion(ctx context.Context, versionID string) (PromptVersion, error) {
	var item PromptVersion
	err := s.pool.QueryRow(ctx, `SELECT version.prompt_version_id,version.prompt_template_id,version.version,
		version.system_template,version.user_template,version.variable_schema,version.default_variables,
		version.model_defaults,version.change_note,version.content_hash,version.status,
		EXISTS(SELECT 1 FROM drama.prompt_production_bindings binding WHERE binding.prompt_version_id=version.prompt_version_id AND binding.is_current),
		version.created_by,version.approved_by,version.approved_at,version.created_at
		FROM drama.prompt_versions version WHERE version.prompt_version_id=$1`, versionID).Scan(
		&item.PromptVersionID, &item.PromptTemplateID, &item.Version, &item.SystemTemplate,
		&item.UserTemplate, &item.VariableSchema, &item.DefaultVariables, &item.ModelDefaults,
		&item.ChangeNote, &item.ContentHash, &item.Status, &item.IsProduction, &item.CreatedBy,
		&item.ApprovedBy, &item.ApprovedAt, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PromptVersion{}, ErrNotFound
	}
	return item, err
}

func (s *Store) CreatePromptVersion(ctx context.Context, templateID string, input CreatePromptVersionInput) (PromptVersion, error) {
	input.UserTemplate, input.ChangeNote = strings.TrimSpace(input.UserTemplate), strings.TrimSpace(input.ChangeNote)
	if input.UserTemplate == "" || input.ChangeNote == "" {
		return PromptVersion{}, fmt.Errorf("%w: user_template and change_note are required", ErrValidation)
	}
	schema, err := objectJSON(input.VariableSchema)
	if err != nil {
		return PromptVersion{}, fmt.Errorf("%w: variable_schema %v", ErrValidation, err)
	}
	defaults, err := objectJSON(input.DefaultVariables)
	if err != nil {
		return PromptVersion{}, fmt.Errorf("%w: default_variables %v", ErrValidation, err)
	}
	modelDefaults, err := objectJSON(input.ModelDefaults)
	if err != nil {
		return PromptVersion{}, fmt.Errorf("%w: model_defaults %v", ErrValidation, err)
	}
	hash, err := promptlab.ContentHash(input.SystemTemplate, input.UserTemplate, schema, defaults, modelDefaults)
	if err != nil {
		return PromptVersion{}, err
	}
	id, err := newPublicID("pv_")
	if err != nil {
		return PromptVersion{}, err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return PromptVersion{}, err
	}
	defer tx.Rollback(ctx)
	var version int
	err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM drama.prompt_versions WHERE prompt_template_id=$1`, templateID).Scan(&version)
	if err != nil {
		return PromptVersion{}, err
	}
	command, err := tx.Exec(ctx, `INSERT INTO drama.prompt_versions(prompt_version_id,prompt_template_id,version,
		system_template,user_template,variable_schema,default_variables,model_defaults,change_note,content_hash,created_by)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')
		WHERE EXISTS(SELECT 1 FROM drama.prompt_templates WHERE prompt_template_id=$2)`, id, templateID, version,
		input.SystemTemplate, input.UserTemplate, schema, defaults, modelDefaults, input.ChangeNote, hash, strings.TrimSpace(input.CreatedBy))
	if err != nil {
		return PromptVersion{}, err
	}
	if command.RowsAffected() == 0 {
		return PromptVersion{}, ErrNotFound
	}
	if err = tx.Commit(ctx); err != nil {
		return PromptVersion{}, err
	}
	return s.GetPromptVersion(ctx, id)
}

func (s *Store) ApprovePromptVersion(ctx context.Context, versionID, actor string) (PromptVersion, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return PromptVersion{}, fmt.Errorf("%w: approver is required", ErrValidation)
	}
	command, err := s.writer.Exec(ctx, `UPDATE drama.prompt_versions SET status='approved',approved_by=$2,approved_at=CURRENT_TIMESTAMP
		WHERE prompt_version_id=$1 AND status='draft'`, versionID, actor)
	if err != nil {
		return PromptVersion{}, err
	}
	if command.RowsAffected() == 0 {
		return PromptVersion{}, fmt.Errorf("%w: only a draft version can be approved", ErrConflict)
	}
	return s.GetPromptVersion(ctx, versionID)
}

func (s *Store) PromotePromptVersion(ctx context.Context, versionID, actor string) (PromptVersion, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return PromptVersion{}, fmt.Errorf("%w: promoter is required", ErrValidation)
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return PromptVersion{}, err
	}
	defer tx.Rollback(ctx)
	var templateID, status string
	err = tx.QueryRow(ctx, `SELECT prompt_template_id,status FROM drama.prompt_versions WHERE prompt_version_id=$1 FOR UPDATE`, versionID).Scan(&templateID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return PromptVersion{}, ErrNotFound
	}
	if err != nil {
		return PromptVersion{}, err
	}
	if status != "approved" {
		return PromptVersion{}, fmt.Errorf("%w: prompt version must be explicitly approved", ErrConflict)
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.prompt_production_bindings SET is_current=false,superseded_at=CURRENT_TIMESTAMP
		WHERE prompt_template_id=$1 AND is_current`, templateID); err != nil {
		return PromptVersion{}, err
	}
	bindingID, err := newPublicID("ppb_")
	if err != nil {
		return PromptVersion{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.prompt_production_bindings(prompt_binding_id,prompt_template_id,prompt_version_id,promoted_by)
		VALUES($1,$2,$3,$4)`, bindingID, templateID, versionID, actor); err != nil {
		return PromptVersion{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return PromptVersion{}, err
	}
	return s.GetPromptVersion(ctx, versionID)
}

func (s *Store) CreatePromptFixture(ctx context.Context, input CreatePromptFixtureInput) (PromptFixture, error) {
	input.Category, input.FixtureKey, input.DisplayName = strings.TrimSpace(input.Category), strings.TrimSpace(input.FixtureKey), strings.TrimSpace(input.DisplayName)
	if !promptlab.ValidCategory(input.Category) || input.FixtureKey == "" || input.DisplayName == "" {
		return PromptFixture{}, fmt.Errorf("%w: category, fixture_key and display_name are required", ErrValidation)
	}
	variables, err := objectJSON(input.Variables)
	if err != nil {
		return PromptFixture{}, fmt.Errorf("%w: variables %v", ErrValidation, err)
	}
	if len(input.ExpectedOutput) == 0 {
		input.ExpectedOutput = json.RawMessage(`{}`)
	}
	if !json.Valid(input.ExpectedOutput) {
		return PromptFixture{}, fmt.Errorf("%w: expected_output must be valid JSON", ErrValidation)
	}
	raw, _ := json.Marshal([]json.RawMessage{variables, input.ExpectedOutput})
	hash := sha256.Sum256(raw)
	id, err := newPublicID("pfx_")
	if err != nil {
		return PromptFixture{}, err
	}
	var item PromptFixture
	err = s.writer.QueryRow(ctx, `INSERT INTO drama.prompt_fixtures(prompt_fixture_id,category,fixture_key,version,
		display_name,variables,expected_output,input_hash,created_by)
		VALUES($1,$2,$3,(SELECT COALESCE(MAX(version),0)+1 FROM drama.prompt_fixtures WHERE category=$2 AND fixture_key=$3),
		$4,$5,$6,$7,NULLIF($8,''))
		RETURNING prompt_fixture_id,category,fixture_key,version,display_name,variables,expected_output,input_hash,frozen,created_at`,
		id, input.Category, input.FixtureKey, input.DisplayName, variables, input.ExpectedOutput,
		hex.EncodeToString(hash[:]), strings.TrimSpace(input.CreatedBy)).Scan(&item.PromptFixtureID, &item.Category,
		&item.FixtureKey, &item.Version, &item.DisplayName, &item.Variables, &item.ExpectedOutput,
		&item.InputHash, &item.Frozen, &item.CreatedAt)
	return item, err
}

func (s *Store) ListPromptFixtures(ctx context.Context, category string) ([]PromptFixture, error) {
	rows, err := s.pool.Query(ctx, `SELECT prompt_fixture_id,category,fixture_key,version,display_name,variables,
		expected_output,input_hash,frozen,created_at FROM drama.prompt_fixtures
		WHERE ($1='' OR category=$1) ORDER BY category,fixture_key,version DESC`, strings.TrimSpace(category))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PromptFixture{}
	for rows.Next() {
		var item PromptFixture
		if err = rows.Scan(&item.PromptFixtureID, &item.Category, &item.FixtureKey, &item.Version,
			&item.DisplayName, &item.Variables, &item.ExpectedOutput, &item.InputHash, &item.Frozen, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreatePromptTestSuite(ctx context.Context, input CreatePromptTestSuiteInput) (PromptTestSuite, error) {
	input.Category, input.DisplayName = strings.TrimSpace(input.Category), strings.TrimSpace(input.DisplayName)
	if !promptlab.ValidCategory(input.Category) || input.DisplayName == "" || len(input.FixtureIDs) == 0 {
		return PromptTestSuite{}, fmt.Errorf("%w: category, display_name and fixture_ids are required", ErrValidation)
	}
	metrics, err := objectJSON(input.MetricConfig)
	if err != nil {
		return PromptTestSuite{}, fmt.Errorf("%w: metric_config %v", ErrValidation, err)
	}
	fixtureJSON, _ := json.Marshal(input.FixtureIDs)
	var count int
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM drama.prompt_fixtures WHERE category=$1 AND prompt_fixture_id=ANY($2)`, input.Category, input.FixtureIDs).Scan(&count)
	if err != nil {
		return PromptTestSuite{}, err
	}
	if count != len(input.FixtureIDs) {
		return PromptTestSuite{}, fmt.Errorf("%w: every fixture must exist in the suite category", ErrValidation)
	}
	raw, _ := json.Marshal([]json.RawMessage{fixtureJSON, metrics})
	hash := sha256.Sum256(raw)
	id, err := newPublicID("pts_")
	if err != nil {
		return PromptTestSuite{}, err
	}
	var item PromptTestSuite
	var idsJSON []byte
	err = s.writer.QueryRow(ctx, `INSERT INTO drama.prompt_test_suites(prompt_test_suite_id,category,display_name,version,
		fixture_ids,metric_config,suite_hash,created_by)
		VALUES($1,$2,$3,(SELECT COALESCE(MAX(version),0)+1 FROM drama.prompt_test_suites WHERE category=$2 AND display_name=$3),
		$4,$5,$6,NULLIF($7,'')) RETURNING prompt_test_suite_id,category,display_name,version,fixture_ids,metric_config,suite_hash,frozen,created_at`,
		id, input.Category, input.DisplayName, fixtureJSON, metrics, hex.EncodeToString(hash[:]), strings.TrimSpace(input.CreatedBy)).Scan(
		&item.PromptTestSuiteID, &item.Category, &item.DisplayName, &item.Version, &idsJSON, &item.MetricConfig,
		&item.SuiteHash, &item.Frozen, &item.CreatedAt)
	if err == nil {
		err = json.Unmarshal(idsJSON, &item.FixtureIDs)
	}
	return item, err
}

func (s *Store) ListPromptTestSuites(ctx context.Context, category string) ([]PromptTestSuite, error) {
	rows, err := s.pool.Query(ctx, `SELECT prompt_test_suite_id,category,display_name,version,fixture_ids,metric_config,
		suite_hash,frozen,created_at FROM drama.prompt_test_suites WHERE ($1='' OR category=$1)
		ORDER BY category,display_name,version DESC`, strings.TrimSpace(category))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PromptTestSuite{}
	for rows.Next() {
		var item PromptTestSuite
		var idsJSON []byte
		if err = rows.Scan(&item.PromptTestSuiteID, &item.Category, &item.DisplayName, &item.Version,
			&idsJSON, &item.MetricConfig, &item.SuiteHash, &item.Frozen, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(idsJSON, &item.FixtureIDs); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreatePromptExperiment(ctx context.Context, input CreatePromptExperimentInput) (PromptExperiment, error) {
	input.Category, input.DisplayName = strings.TrimSpace(input.Category), strings.TrimSpace(input.DisplayName)
	if !promptlab.ValidCategory(input.Category) || input.DisplayName == "" || len(input.Variants) < 2 {
		return PromptExperiment{}, fmt.Errorf("%w: a category, name and at least two variants are required", ErrValidation)
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return PromptExperiment{}, err
	}
	defer tx.Rollback(ctx)
	var suiteCategory, suiteHash string
	err = tx.QueryRow(ctx, `SELECT category,suite_hash FROM drama.prompt_test_suites WHERE prompt_test_suite_id=$1`, input.PromptTestSuiteID).Scan(&suiteCategory, &suiteHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return PromptExperiment{}, ErrNotFound
	}
	if err != nil {
		return PromptExperiment{}, err
	}
	if suiteCategory != input.Category {
		return PromptExperiment{}, fmt.Errorf("%w: suite category mismatch", ErrValidation)
	}
	experimentID, err := newPublicID("pex_")
	if err != nil {
		return PromptExperiment{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.prompt_experiments(prompt_experiment_id,category,display_name,
		prompt_test_suite_id,suite_hash,blind_review,status,created_by) VALUES($1,$2,$3,$4,$5,$6,'draft',NULLIF($7,''))`,
		experimentID, input.Category, input.DisplayName, input.PromptTestSuiteID, suiteHash, input.BlindReview,
		strings.TrimSpace(input.CreatedBy)); err != nil {
		return PromptExperiment{}, err
	}
	for index, variant := range input.Variants {
		variant.Provider, variant.Model = strings.TrimSpace(variant.Provider), strings.TrimSpace(variant.Model)
		if variant.Provider == "" || variant.Model == "" {
			return PromptExperiment{}, fmt.Errorf("%w: provider and model are required", ErrValidation)
		}
		parameters, parseErr := objectJSON(variant.Parameters)
		if parseErr != nil {
			return PromptExperiment{}, fmt.Errorf("%w: variant parameters %v", ErrValidation, parseErr)
		}
		var versionCategory string
		if err = tx.QueryRow(ctx, `SELECT template.category FROM drama.prompt_versions version
			JOIN drama.prompt_templates template USING(prompt_template_id) WHERE version.prompt_version_id=$1`, variant.PromptVersionID).Scan(&versionCategory); err != nil {
			return PromptExperiment{}, err
		}
		if versionCategory != input.Category {
			return PromptExperiment{}, fmt.Errorf("%w: prompt version category mismatch", ErrValidation)
		}
		variantID, idErr := newPublicID("pev_")
		if idErr != nil {
			return PromptExperiment{}, idErr
		}
		label := "方案 " + alphaLabel(index)
		if _, err = tx.Exec(ctx, `INSERT INTO drama.prompt_experiment_variants(prompt_experiment_variant_id,
			prompt_experiment_id,prompt_version_id,provider,model,parameters,seed,blind_label)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, variantID, experimentID, variant.PromptVersionID,
			variant.Provider, variant.Model, parameters, variant.Seed, label); err != nil {
			return PromptExperiment{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return PromptExperiment{}, err
	}
	return s.GetPromptExperiment(ctx, experimentID, false)
}

func (s *Store) ListPromptExperiments(ctx context.Context, category string) ([]PromptExperiment, error) {
	rows, err := s.pool.Query(ctx, `SELECT prompt_experiment_id FROM drama.prompt_experiments
		WHERE ($1='' OR category=$1) ORDER BY created_at DESC`, strings.TrimSpace(category))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	items := make([]PromptExperiment, 0, len(ids))
	for _, id := range ids {
		item, getErr := s.GetPromptExperiment(ctx, id, false)
		if getErr != nil {
			return nil, getErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) GetPromptExperiment(ctx context.Context, experimentID string, blind bool) (PromptExperiment, error) {
	var item PromptExperiment
	err := s.pool.QueryRow(ctx, `SELECT prompt_experiment_id,category,display_name,prompt_test_suite_id,suite_hash,
		blind_review,status,created_at FROM drama.prompt_experiments WHERE prompt_experiment_id=$1`, experimentID).Scan(
		&item.PromptExperimentID, &item.Category, &item.DisplayName, &item.PromptTestSuiteID, &item.SuiteHash,
		&item.BlindReview, &item.Status, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PromptExperiment{}, ErrNotFound
	}
	if err != nil {
		return PromptExperiment{}, err
	}
	variantRows, err := s.pool.Query(ctx, `SELECT prompt_experiment_variant_id,prompt_version_id,provider,model,parameters,seed,blind_label
		FROM drama.prompt_experiment_variants WHERE prompt_experiment_id=$1 ORDER BY blind_label`, experimentID)
	if err != nil {
		return PromptExperiment{}, err
	}
	item.Variants = []PromptExperimentVariant{}
	for variantRows.Next() {
		var value PromptExperimentVariant
		if err = variantRows.Scan(&value.PromptExperimentVariantID, &value.PromptVersionID,
			&value.Provider, &value.Model, &value.Parameters, &value.Seed, &value.BlindLabel); err != nil {
			variantRows.Close()
			return PromptExperiment{}, err
		}
		if blind {
			value.PromptVersionID = ""
			value.Provider = ""
			value.Model = ""
			value.Parameters = nil
			value.Seed = nil
		}
		item.Variants = append(item.Variants, value)
	}
	variantRows.Close()
	resultRows, err := s.pool.Query(ctx, `SELECT result.prompt_experiment_result_id,result.prompt_experiment_variant_id,
		result.prompt_fixture_id,variant.blind_label,result.rendered_input,result.rendered_input_hash,result.output,result.output_hash,
		result.token_estimate,result.token_usage,result.latency_ms,result.estimated_cost::float8,result.automatic_metrics,result.status,
		result.error_message
		FROM drama.prompt_experiment_results result JOIN drama.prompt_experiment_variants variant USING(prompt_experiment_variant_id)
		WHERE result.prompt_experiment_id=$1 ORDER BY result.prompt_fixture_id,variant.blind_label`, experimentID)
	if err != nil {
		return PromptExperiment{}, err
	}
	item.Results = []PromptExperimentResult{}
	for resultRows.Next() {
		var value PromptExperimentResult
		if err = resultRows.Scan(&value.PromptExperimentResultID, &value.PromptExperimentVariantID,
			&value.PromptFixtureID, &value.BlindLabel, &value.RenderedInput, &value.RenderedInputHash, &value.Output, &value.OutputHash,
			&value.TokenEstimate, &value.TokenUsage, &value.LatencyMS, &value.EstimatedCost, &value.AutomaticMetrics, &value.Status,
			&value.ErrorMessage); err != nil {
			resultRows.Close()
			return PromptExperiment{}, err
		}
		if blind {
			value.PromptExperimentVariantID = ""
			value.RenderedInput = ""
			value.RenderedInputHash = ""
		}
		item.Results = append(item.Results, value)
	}
	resultRows.Close()
	evalRows, err := s.pool.Query(ctx, `SELECT prompt_blind_evaluation_id,prompt_fixture_id,blind_label,reviewer,score::float8,
		rubric_scores,comment,created_at FROM drama.prompt_blind_evaluations WHERE prompt_experiment_id=$1 ORDER BY created_at`, experimentID)
	if err != nil {
		return PromptExperiment{}, err
	}
	defer evalRows.Close()
	item.Evaluations = []PromptBlindEvaluation{}
	for evalRows.Next() {
		var value PromptBlindEvaluation
		if err = evalRows.Scan(&value.PromptBlindEvaluationID, &value.PromptFixtureID, &value.BlindLabel,
			&value.Reviewer, &value.Score, &value.RubricScores, &value.Comment, &value.CreatedAt); err != nil {
			return PromptExperiment{}, err
		}
		item.Evaluations = append(item.Evaluations, value)
	}
	return item, evalRows.Err()
}

func (s *Store) SavePromptExperimentResult(ctx context.Context, experimentID string, input SavePromptExperimentResultInput) (PromptExperiment, error) {
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		input.Status = "completed"
	}
	if input.Status != "completed" && input.Status != "failed" {
		return PromptExperiment{}, fmt.Errorf("%w: result status must be completed or failed", ErrValidation)
	}
	if input.Status == "completed" && (strings.TrimSpace(input.RenderedInput) == "" || len(input.Output) == 0 || !json.Valid(input.Output)) {
		return PromptExperiment{}, fmt.Errorf("%w: completed result requires rendered_input and valid JSON output", ErrValidation)
	}
	if input.Status == "failed" && strings.TrimSpace(input.ErrorMessage) == "" {
		return PromptExperiment{}, fmt.Errorf("%w: failed result requires error_message", ErrValidation)
	}
	if len(input.Output) == 0 || !json.Valid(input.Output) {
		input.Output = json.RawMessage(`{}`)
	}
	if input.RenderedInputHash == "" {
		sum := sha256.Sum256([]byte(input.RenderedInput))
		input.RenderedInputHash = hex.EncodeToString(sum[:])
	}
	if len(input.TokenUsage) == 0 {
		input.TokenUsage = json.RawMessage(`{}`)
	}
	if len(input.AutomaticMetrics) == 0 {
		input.AutomaticMetrics = json.RawMessage(`{}`)
	}
	outputHash := sha256.Sum256(input.Output)
	resultID, err := newPublicID("per_")
	if err != nil {
		return PromptExperiment{}, err
	}
	command, err := s.writer.Exec(ctx, `INSERT INTO drama.prompt_experiment_results(prompt_experiment_result_id,prompt_experiment_id,
		prompt_experiment_variant_id,prompt_fixture_id,rendered_input,rendered_input_hash,output,output_hash,token_estimate,
		token_usage,latency_ms,estimated_cost,automatic_metrics,status,error_message)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULLIF($15,'')
		WHERE EXISTS(SELECT 1 FROM drama.prompt_experiment_variants WHERE prompt_experiment_variant_id=$3 AND prompt_experiment_id=$2)
		AND EXISTS(SELECT 1 FROM drama.prompt_test_suites suite JOIN drama.prompt_experiments experiment USING(prompt_test_suite_id)
			WHERE experiment.prompt_experiment_id=$2 AND suite.fixture_ids ? $4)
		ON CONFLICT(prompt_experiment_variant_id,prompt_fixture_id) DO NOTHING`, resultID, experimentID, input.PromptExperimentVariantID,
		input.PromptFixtureID, input.RenderedInput, input.RenderedInputHash, input.Output, hex.EncodeToString(outputHash[:]), input.TokenEstimate,
		input.TokenUsage, input.LatencyMS, input.EstimatedCost, input.AutomaticMetrics, input.Status, strings.TrimSpace(input.ErrorMessage))
	if err != nil {
		return PromptExperiment{}, err
	}
	if command.RowsAffected() == 0 {
		return PromptExperiment{}, fmt.Errorf("%w: result does not belong to the frozen experiment matrix", ErrConflict)
	}
	return s.GetPromptExperiment(ctx, experimentID, false)
}

func (s *Store) BeginPromptExperimentRun(ctx context.Context, experimentID string) (PromptExperiment, []PromptFixture, error) {
	command, err := s.writer.Exec(ctx, `UPDATE drama.prompt_experiments experiment SET status='running'
		WHERE prompt_experiment_id=$1 AND status IN ('draft','evaluation')
		AND NOT EXISTS(SELECT 1 FROM drama.prompt_experiment_results result
			WHERE result.prompt_experiment_id=experiment.prompt_experiment_id)`, experimentID)
	if err != nil {
		return PromptExperiment{}, nil, err
	}
	if command.RowsAffected() == 0 {
		return PromptExperiment{}, nil, fmt.Errorf("%w: experiment is already running or has immutable results", ErrConflict)
	}
	experiment, err := s.GetPromptExperiment(ctx, experimentID, false)
	if err != nil {
		return PromptExperiment{}, nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT fixture.prompt_fixture_id,fixture.category,fixture.fixture_key,fixture.version,
		fixture.display_name,fixture.variables,fixture.expected_output,fixture.input_hash,fixture.frozen,fixture.created_at
		FROM drama.prompt_experiments experiment
		JOIN drama.prompt_test_suites suite USING(prompt_test_suite_id)
		JOIN drama.prompt_fixtures fixture ON suite.fixture_ids ? fixture.prompt_fixture_id
		WHERE experiment.prompt_experiment_id=$1 ORDER BY fixture.prompt_fixture_id`, experimentID)
	if err != nil {
		return PromptExperiment{}, nil, err
	}
	defer rows.Close()
	fixtures := []PromptFixture{}
	for rows.Next() {
		var item PromptFixture
		if err = rows.Scan(&item.PromptFixtureID, &item.Category, &item.FixtureKey, &item.Version,
			&item.DisplayName, &item.Variables, &item.ExpectedOutput, &item.InputHash, &item.Frozen, &item.CreatedAt); err != nil {
			return PromptExperiment{}, nil, err
		}
		fixtures = append(fixtures, item)
	}
	if err = rows.Err(); err != nil {
		return PromptExperiment{}, nil, err
	}
	return experiment, fixtures, nil
}

func (s *Store) FinishPromptExperimentRun(ctx context.Context, experimentID string) (PromptExperiment, error) {
	command, err := s.writer.Exec(ctx, `UPDATE drama.prompt_experiments experiment
		SET status=CASE WHEN blind_review THEN 'evaluation' ELSE 'completed' END,completed_at=CURRENT_TIMESTAMP
		WHERE prompt_experiment_id=$1 AND status='running'
		AND (SELECT count(*) FROM drama.prompt_experiment_results result
			WHERE result.prompt_experiment_id=experiment.prompt_experiment_id)=
			(SELECT jsonb_array_length(suite.fixture_ids)*count(variant.*)
			 FROM drama.prompt_test_suites suite
			 JOIN drama.prompt_experiment_variants variant ON variant.prompt_experiment_id=experiment.prompt_experiment_id
			 WHERE suite.prompt_test_suite_id=experiment.prompt_test_suite_id GROUP BY suite.fixture_ids)`, experimentID)
	if err != nil {
		return PromptExperiment{}, err
	}
	if command.RowsAffected() == 0 {
		return PromptExperiment{}, fmt.Errorf("%w: experiment result matrix is incomplete", ErrConflict)
	}
	return s.GetPromptExperiment(ctx, experimentID, false)
}

func (s *Store) SavePromptBlindEvaluation(ctx context.Context, experimentID string, input SaveBlindEvaluationInput) (PromptExperiment, error) {
	input.Reviewer, input.BlindLabel = strings.TrimSpace(input.Reviewer), strings.TrimSpace(input.BlindLabel)
	if input.Reviewer == "" || input.BlindLabel == "" || input.Score < 0 || input.Score > 100 {
		return PromptExperiment{}, fmt.Errorf("%w: blind label, reviewer and score 0-100 are required", ErrValidation)
	}
	rubric, err := objectJSON(input.RubricScores)
	if err != nil {
		return PromptExperiment{}, fmt.Errorf("%w: rubric_scores %v", ErrValidation, err)
	}
	id, err := newPublicID("pbe_")
	if err != nil {
		return PromptExperiment{}, err
	}
	command, err := s.writer.Exec(ctx, `INSERT INTO drama.prompt_blind_evaluations(prompt_blind_evaluation_id,prompt_experiment_id,
		prompt_fixture_id,blind_label,reviewer,score,rubric_scores,comment)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8 WHERE EXISTS(SELECT 1 FROM drama.prompt_experiment_variants
		 WHERE prompt_experiment_id=$2 AND blind_label=$4)
		AND EXISTS(SELECT 1 FROM drama.prompt_experiment_results result JOIN drama.prompt_experiment_variants variant
		 USING(prompt_experiment_variant_id) WHERE result.prompt_experiment_id=$2 AND result.prompt_fixture_id=$3
		 AND variant.blind_label=$4 AND result.status='completed')`,
		id, experimentID, input.PromptFixtureID, input.BlindLabel, input.Reviewer, input.Score, rubric, strings.TrimSpace(input.Comment))
	if err != nil {
		return PromptExperiment{}, err
	}
	if command.RowsAffected() == 0 {
		return PromptExperiment{}, fmt.Errorf("%w: blind evaluation target has no completed result", ErrConflict)
	}
	return s.GetPromptExperiment(ctx, experimentID, true)
}

func (s *Store) RecordArtifactProvenance(ctx context.Context, input ArtifactProvenance) (ArtifactProvenance, error) {
	if strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.ArtifactType) == "" || strings.TrimSpace(input.ArtifactID) == "" || input.ArtifactVersion < 1 ||
		strings.TrimSpace(input.PromptVersionID) == "" || strings.TrimSpace(input.Provider) == "" || strings.TrimSpace(input.Model) == "" ||
		input.Seed == nil || !isSHA256(input.InputArtifactHash) || !isSHA256(input.OutputArtifactHash) {
		return ArtifactProvenance{}, fmt.Errorf("%w: complete prompt/model/parameter/seed and artifact hashes are required", ErrValidation)
	}
	parameters, err := objectJSON(input.Parameters)
	if err != nil {
		return ArtifactProvenance{}, fmt.Errorf("%w: parameters %v", ErrValidation, err)
	}
	if len(input.SourceArtifacts) == 0 {
		input.SourceArtifacts = json.RawMessage(`[]`)
	}
	var sources []any
	if err = json.Unmarshal(input.SourceArtifacts, &sources); err != nil {
		return ArtifactProvenance{}, fmt.Errorf("%w: source_artifacts must be an array", ErrValidation)
	}
	id, err := newPublicID("agp_")
	if err != nil {
		return ArtifactProvenance{}, err
	}
	err = s.writer.QueryRow(ctx, `INSERT INTO drama.artifact_generation_provenance(generation_provenance_id,project_id,episode_id,
		artifact_type,artifact_id,artifact_version,prompt_version_id,provider,model,parameters,seed,input_artifact_hash,
		output_artifact_hash,source_artifacts) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING generation_provenance_id,project_id,episode_id,artifact_type,artifact_id,artifact_version,prompt_version_id,
		provider,model,parameters,seed,input_artifact_hash,output_artifact_hash,source_artifacts,created_at`, id, input.ProjectID, input.EpisodeID,
		input.ArtifactType, input.ArtifactID, input.ArtifactVersion, input.PromptVersionID, input.Provider, input.Model, parameters, input.Seed,
		input.InputArtifactHash, input.OutputArtifactHash, input.SourceArtifacts).Scan(&input.GenerationProvenanceID, &input.ProjectID, &input.EpisodeID,
		&input.ArtifactType, &input.ArtifactID, &input.ArtifactVersion, &input.PromptVersionID, &input.Provider, &input.Model, &input.Parameters, &input.Seed,
		&input.InputArtifactHash, &input.OutputArtifactHash, &input.SourceArtifacts, &input.CreatedAt)
	return input, err
}

func (s *Store) ListArtifactProvenance(ctx context.Context, projectID, episodeID string) ([]ArtifactProvenance, error) {
	rows, err := s.pool.Query(ctx, `SELECT generation_provenance_id,project_id,episode_id,artifact_type,artifact_id,artifact_version,
		prompt_version_id,provider,model,parameters,seed,input_artifact_hash,output_artifact_hash,source_artifacts,created_at
		FROM drama.artifact_generation_provenance WHERE project_id=$1 AND ($2='' OR episode_id=$2) ORDER BY created_at DESC`, projectID, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ArtifactProvenance{}
	for rows.Next() {
		var item ArtifactProvenance
		if err = rows.Scan(&item.GenerationProvenanceID, &item.ProjectID, &item.EpisodeID, &item.ArtifactType, &item.ArtifactID,
			&item.ArtifactVersion, &item.PromptVersionID, &item.Provider, &item.Model, &item.Parameters, &item.Seed, &item.InputArtifactHash, &item.OutputArtifactHash,
			&item.SourceArtifacts, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func objectJSON(value json.RawMessage) (json.RawMessage, error) {
	if len(value) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var object map[string]any
	if err := json.Unmarshal(value, &object); err != nil {
		return nil, err
	}
	return value, nil
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func alphaLabel(index int) string {
	index++
	result := ""
	for index > 0 {
		index--
		result = string(rune('A'+index%26)) + result
		index /= 26
	}
	return result
}
