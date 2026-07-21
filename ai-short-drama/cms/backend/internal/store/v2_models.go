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
