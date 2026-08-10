package store

import (
	"encoding/json"
	"time"
)

type SourceWork struct {
	WorkID           string          `json:"work_id"`
	Title            string          `json:"title"`
	Author           *string         `json:"author"`
	Status           string          `json:"status"`
	ResourceRevision int             `json:"resource_revision"`
	Metadata         json.RawMessage `json:"metadata"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type SourceWorkList struct {
	Items []SourceWork
	Total int
}

type CreateSourceWorkInput struct {
	Title    string
	Author   *string
	Metadata json.RawMessage
}

type SourceVersion struct {
	SourceVersionID       string          `json:"source_version_id"`
	WorkID                string          `json:"work_id"`
	VersionNumber         int             `json:"version_number"`
	ParentSourceVersionID *string         `json:"parent_source_version_id"`
	Status                string          `json:"status"`
	VersionHash           string          `json:"version_hash"`
	NormalizationVersion  string          `json:"normalization_version"`
	ChapterCount          int             `json:"chapter_count"`
	TotalChars            int             `json:"total_chars"`
	ResourceRevision      int             `json:"resource_revision"`
	Metadata              json.RawMessage `json:"-"`
}

type CreateSourceVersionInput struct {
	ParentSourceVersionID *string
	NormalizationVersion  string
	Metadata              json.RawMessage
}

type ChapterInput struct {
	ClientItemKey string  `json:"client_item_key"`
	ChapterID     *string `json:"chapter_id"`
	Ordinal       int     `json:"ordinal"`
	Title         string  `json:"title"`
	Content       string  `json:"content"`
}

type ChapterRevision struct {
	ChapterID         string `json:"chapter_id"`
	ChapterRevisionID string `json:"chapter_revision_id"`
	Ordinal           int    `json:"ordinal"`
	RevisionNumber    int    `json:"revision_number"`
	Title             string `json:"title"`
	ContentHash       string `json:"content_hash"`
	CharCount         int    `json:"char_count"`
}

type VersionChapterContent struct {
	ChapterID         string `json:"chapter_id"`
	ChapterRevisionID string `json:"chapter_revision_id"`
	SourceVersionID   string `json:"source_version_id"`
	Ordinal           int    `json:"ordinal"`
	RevisionNumber    int    `json:"revision_number"`
	Title             string `json:"title"`
	Content           string `json:"content"`
	ContentHash       string `json:"content_hash"`
	CharCount         int    `json:"char_count"`
}

type ChapterRevisionHistoryItem struct {
	ChapterID         string    `json:"chapter_id"`
	ChapterRevisionID string    `json:"chapter_revision_id"`
	RevisionNumber    int       `json:"revision_number"`
	Title             string    `json:"title"`
	ContentHash       string    `json:"content_hash"`
	CharCount         int       `json:"char_count"`
	CreatedAt         time.Time `json:"created_at"`
}

type NarrativeIRRevisionSummary struct {
	IRRevisionID            string          `json:"ir_revision_id"`
	OperationID             string          `json:"operation_id"`
	OperationStatus         string          `json:"operation_status"`
	CheckpointStage         string          `json:"checkpoint_stage"`
	CompletedItems          int             `json:"completed_items"`
	TotalItems              int             `json:"total_items"`
	RetryCount              int             `json:"retry_count"`
	OperationErrorCode      *string         `json:"operation_error_code,omitempty"`
	OperationErrorMessage   *string         `json:"operation_error_message,omitempty"`
	OperationErrorRetryable *bool           `json:"operation_error_retryable,omitempty"`
	SourceVersionID         string          `json:"source_version_id"`
	RevisionNumber          int             `json:"revision_number"`
	Status                  string          `json:"status"`
	RevisionScope           string          `json:"revision_scope"`
	BaseIRRevisionID        *string         `json:"base_ir_revision_id,omitempty"`
	ExtractorVersion        string          `json:"extractor_version"`
	ChangedChapterIDs       json.RawMessage `json:"changed_chapter_ids"`
	ValidationSummary       json.RawMessage `json:"validation_summary"`
	CreatedAt               time.Time       `json:"created_at"`
	PublishedAt             *time.Time      `json:"published_at"`
}

type StoryArcSummary struct {
	StoryArcRevisionID string  `json:"story_arc_revision_id"`
	IRRevisionID       string  `json:"ir_revision_id"`
	ChapterID          string  `json:"chapter_id"`
	Title              string  `json:"title"`
	Summary            string  `json:"summary"`
	ArcType            string  `json:"arc_type"`
	Confidence         float64 `json:"confidence"`
}

type IRMergeProposalInput struct {
	BaseFullIRRevisionID    string `json:"base_full_ir_revision_id"`
	IncrementalIRRevisionID string `json:"incremental_ir_revision_id"`
	CreatedBy               string `json:"created_by,omitempty"`
}

type IRMergeProposalItem struct {
	IRMergeItemID             string          `json:"ir_merge_item_id"`
	ItemType                  string          `json:"item_type"`
	LogicalID                 string          `json:"logical_id"`
	ChangeType                string          `json:"change_type"`
	BeforeRevisionID          *string         `json:"before_revision_id"`
	AfterRevisionID           *string         `json:"after_revision_id"`
	BeforeValue               json.RawMessage `json:"before_value"`
	AfterValue                json.RawMessage `json:"after_value"`
	BeforeEvidence            json.RawMessage `json:"before_evidence"`
	AfterEvidence             json.RawMessage `json:"after_evidence"`
	SemanticFingerprint       *string         `json:"semantic_fingerprint,omitempty"`
	SemanticChanged           bool            `json:"semantic_changed"`
	SourceSpanChanged         bool            `json:"source_span_changed"`
	Confidence                float64         `json:"confidence"`
	ConflictCode              *string         `json:"conflict_code,omitempty"`
	ConflictMessage           *string         `json:"conflict_message,omitempty"`
	Resolution                *string         `json:"resolution,omitempty"`
	ResolvedValue             json.RawMessage `json:"resolved_value"`
	ResolutionStatus          string          `json:"resolution_status"`
	CanonicalizationRequired  bool            `json:"canonicalization_required"`
	CanonicalizationConfirmed bool            `json:"canonicalization_confirmed"`
	CanonicalizationDecision  *string         `json:"canonicalization_decision,omitempty"`
	CanonicalEntityID         *string         `json:"canonical_entity_id,omitempty"`
	ResolutionNote            *string         `json:"resolution_note,omitempty"`
	ResolvedBy                *string         `json:"resolved_by,omitempty"`
	ResolvedAt                *time.Time      `json:"resolved_at,omitempty"`
}

type IRMergeImpactArtifact struct {
	ArtifactID     string `json:"artifact_id"`
	ArtifactType   string `json:"artifact_type"`
	NativeEntityID string `json:"native_entity_id"`
	ProjectID      string `json:"project_id,omitempty"`
	Depth          int    `json:"propagation_depth"`
}

type IRMergeImpactPreview struct {
	SemanticChangeCount int                     `json:"semantic_change_count"`
	RelocationOnlyCount int                     `json:"relocation_only_count"`
	AffectedArtifacts   []IRMergeImpactArtifact `json:"affected_artifacts"`
	AutoRebuild         bool                    `json:"auto_rebuild"`
}

type IRMergeProposal struct {
	IRMergeProposalID         string                `json:"ir_merge_proposal_id"`
	WorkID                    string                `json:"work_id"`
	TargetSourceVersionID     string                `json:"target_source_version_id"`
	BaseFullIRRevisionID      string                `json:"base_full_ir_revision_id"`
	IncrementalIRRevisionID   string                `json:"incremental_ir_revision_id"`
	PublishedFullIRRevisionID *string               `json:"published_full_ir_revision_id,omitempty"`
	Status                    string                `json:"status"`
	ResourceRevision          int                   `json:"resource_revision"`
	Confidence                float64               `json:"confidence"`
	ConflictCount             int                   `json:"conflict_count"`
	UnresolvedCount           int                   `json:"unresolved_count"`
	ChangedChapterIDs         []string              `json:"changed_chapter_ids"`
	ImpactPreview             IRMergeImpactPreview  `json:"impact_preview"`
	Items                     []IRMergeProposalItem `json:"items"`
	CreatedBy                 *string               `json:"created_by,omitempty"`
	PublishedBy               *string               `json:"published_by,omitempty"`
	PublishedAt               *time.Time            `json:"published_at,omitempty"`
	CreatedAt                 time.Time             `json:"created_at"`
	UpdatedAt                 time.Time             `json:"updated_at"`
}

type IRMergeItemResolutionInput struct {
	Resolution                string          `json:"resolution"`
	ResolvedValue             json.RawMessage `json:"resolved_value,omitempty"`
	ResolutionNote            string          `json:"resolution_note,omitempty"`
	ResolvedBy                string          `json:"resolved_by,omitempty"`
	CanonicalizationConfirmed bool            `json:"canonicalization_confirmed,omitempty"`
	CanonicalizationDecision  string          `json:"canonicalization_decision,omitempty"`
	CanonicalEntityID         string          `json:"canonical_entity_id,omitempty"`
}

type PublishIRMergeInput struct {
	Confirmed   bool   `json:"confirmed"`
	PublishedBy string `json:"published_by,omitempty"`
}

type PublishIRMergeResult struct {
	IRMergeProposalID  string    `json:"ir_merge_proposal_id"`
	FullIRRevisionID   string    `json:"full_ir_revision_id"`
	SourceChangeSetID  string    `json:"source_change_set_id"`
	ImpactOperationIDs []string  `json:"impact_operation_ids"`
	Status             string    `json:"status"`
	PublishedAt        time.Time `json:"published_at"`
}

type ImportInput struct {
	Mode       string
	Text       string
	StorageRef string
	Items      []ChapterInput
}

type OperationCheckpoint struct {
	Stage          string  `json:"stage"`
	Cursor         *string `json:"cursor,omitempty"`
	CompletedItems *int    `json:"completed_items,omitempty"`
	TotalItems     *int    `json:"total_items,omitempty"`
}

type ResultReference struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

type OperationError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type Operation struct {
	OperationID    string              `json:"operation_id"`
	TraceID        string              `json:"trace_id"`
	OperationType  string              `json:"operation_type"`
	TargetType     string              `json:"target_type"`
	TargetID       string              `json:"target_id"`
	Status         string              `json:"status"`
	Checkpoint     OperationCheckpoint `json:"checkpoint"`
	RetryCount     int                 `json:"retry_count"`
	MaxRetries     int                 `json:"max_retries"`
	LeaseExpiresAt *time.Time          `json:"lease_expires_at,omitempty"`
	ResultRef      *ResultReference    `json:"result_ref"`
	Error          *OperationError     `json:"error"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	InputHash      string              `json:"-"`
}

type IRRunInput struct {
	SchemaVersion    string
	ExtractorVersion string
	ChapterIDs       []string
}

type CompilerRunInput struct {
	AdaptationSpecVersionID string
	IRRevisionID            string
	CompilerVersion         string
	PlanningConstraints     json.RawMessage
	ProviderSuggestions     json.RawMessage
}

type AdaptationSpecSummary struct {
	AdaptationSpecID        string  `json:"adaptation_spec_id"`
	AdaptationSpecVersionID string  `json:"adaptation_spec_version_id"`
	VersionNumber           int     `json:"version_number"`
	Status                  string  `json:"status"`
	SourceVersionID         string  `json:"source_version_id"`
	IRRevisionID            *string `json:"ir_revision_id"`
	ResourceRevision        int     `json:"resource_revision"`
}

type AdaptationScopeInput struct {
	Mode                string
	ChapterIDs          []string
	StoryArcRevisionIDs []string
}

type AdaptationRuleInput struct {
	RuleType    string
	Enforcement string
	TargetType  string
	TargetID    *string
	Priority    int
	Parameters  json.RawMessage
	Rationale   string
}

type AdaptationSpecInput struct {
	SchemaVersion          string
	SourceVersionID        string
	IRRevisionID           string
	Scope                  AdaptationScopeInput
	Platform               string
	AudienceProfile        json.RawMessage
	TargetEpisodeCount     int
	EpisodeDurationSeconds int
	Rules                  []AdaptationRuleInput
}

type CreateAdaptationProjectInput struct {
	DisplayName    string
	AdaptationSpec AdaptationSpecInput
}

type ImpactChange struct {
	SourceChangeItemID string          `json:"source_change_item_id"`
	ChangeType         string          `json:"change_type"`
	BeforeEntityID     *string         `json:"before_entity_id"`
	AfterEntityID      *string         `json:"after_entity_id"`
	Details            json.RawMessage `json:"details"`
}

type ArtifactImpact struct {
	ArtifactID       string          `json:"artifact_id"`
	ArtifactType     string          `json:"artifact_type"`
	NativeEntityID   string          `json:"native_entity_id"`
	RevisionNumber   int             `json:"revision_number"`
	BeforeStatus     string          `json:"before_status"`
	AfterStatus      string          `json:"after_status"`
	ReviewStatus     *string         `json:"review_status"`
	PropagationDepth int             `json:"propagation_depth"`
	Reason           json.RawMessage `json:"reason"`
}

type ProjectImpact struct {
	SourceChangeSetID      string           `json:"source_change_set_id"`
	FromSourceVersionID    string           `json:"from_source_version_id"`
	ToSourceVersionID      string           `json:"to_source_version_id"`
	FromIRRevisionID       *string          `json:"from_ir_revision_id"`
	ToIRRevisionID         *string          `json:"to_ir_revision_id"`
	Status                 string           `json:"status"`
	ChangedChapterIDs      []string         `json:"changed_chapter_ids"`
	ChangedEvents          []ImpactChange   `json:"changed_events"`
	ChangedCharacterStates []ImpactChange   `json:"changed_character_states"`
	AffectedStoryArcs      []ImpactChange   `json:"affected_story_arcs"`
	AffectedArtifacts      []ArtifactImpact `json:"affected_artifacts"`
	NeedsReview            []string         `json:"needs_review"`
}

type RegenerationRequestInput struct {
	Strategy    string
	ArtifactIDs []string
	RequestedBy *string
}

type RegenerationRequest struct {
	RegenerationRequestID string    `json:"regeneration_request_id"`
	SourceChangeSetID     string    `json:"source_change_set_id"`
	ProjectID             string    `json:"project_id"`
	Strategy              string    `json:"strategy"`
	Status                string    `json:"status"`
	ArtifactIDs           []string  `json:"artifact_ids"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type PacingBeatEditInput struct {
	BeatKey                  string `json:"beat_key"`
	EpisodeNumber            *int   `json:"episode_number,omitempty"`
	BeatOrdinal              *int   `json:"beat_ordinal,omitempty"`
	EstimatedDurationSeconds *int   `json:"estimated_duration_seconds,omitempty"`
}

type EditPacingInput struct {
	Edits []PacingBeatEditInput `json:"edits"`
}

type QualityRescoreInput struct {
	Scope         string          `json:"scope"`
	ScopeSelector json.RawMessage `json:"scope_selector"`
}
