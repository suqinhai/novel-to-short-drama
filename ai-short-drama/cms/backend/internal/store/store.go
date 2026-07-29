package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("record not found")
	ErrUnsafeArchive = errors.New("project cannot be archived safely")
	ErrNotArchived   = errors.New("project is not archived")
)

type Store struct {
	pool   *pgxpool.Pool
	writer *pgxpool.Pool
}

type BusinessDataSummary struct {
	TableCount int   `json:"table_count"`
	RowCount   int64 `json:"row_count"`
}

var preservedBusinessTables = map[string]struct{}{
	"artifact_types":    {},
	"migration_audit":   {},
	"schema_migrations": {},
}

type Project struct {
	ProjectID              string          `json:"project_id"`
	NovelName              string          `json:"novel_name"`
	TargetEpisodeCount     int             `json:"target_episode_count"`
	GeneratedEpisodeCount  int             `json:"generated_episode_count"`
	ChunkCount             int             `json:"chunk_count"`
	CompletedChunkCount    int             `json:"completed_chunk_count"`
	EpisodeDurationSeconds int             `json:"episode_duration_seconds"`
	VisualStyle            string          `json:"visual_style"`
	AspectRatio            string          `json:"aspect_ratio"`
	TargetPlatform         string          `json:"target_platform"`
	CurrentStage           string          `json:"current_stage"`
	Status                 string          `json:"status"`
	TestMode               bool            `json:"test_mode"`
	PendingReviews         int             `json:"pending_reviews"`
	FailedTasks            int             `json:"failed_tasks"`
	ActiveTasks            int             `json:"active_tasks"`
	CanArchive             bool            `json:"can_archive"`
	Config                 json.RawMessage `json:"config,omitempty"`
	ErrorMessage           *string         `json:"error_message,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

type ProjectCounts struct {
	Chapters        int `json:"chapters"`
	Chunks          int `json:"chunks"`
	CompletedChunks int `json:"completed_chunks"`
	Episodes        int `json:"episodes"`
	Scenes          int `json:"scenes"`
	Shots           int `json:"shots"`
	GeneratedImages int `json:"generated_images"`
	GeneratedVideos int `json:"generated_videos"`
	CompletedTasks  int `json:"completed_tasks"`
	PendingReviews  int `json:"pending_reviews"`
}

type ProjectDetail struct {
	Project
	Counts            ProjectCounts     `json:"counts"`
	WorkflowTasks     []WorkflowTask    `json:"workflow_tasks"`
	ReviewTasks       []ReviewTask      `json:"review_tasks"`
	Novels            []Novel           `json:"novels"`
	StoryBibles       []StoryBible      `json:"story_bibles"`
	Episodes          []Episode         `json:"episodes"`
	Scripts           []EpisodeScript   `json:"scripts"`
	Storyboards       []Storyboard      `json:"storyboards"`
	RollingProduction RollingProduction `json:"rolling_production"`
}

type WorkflowTask struct {
	TaskID            string     `json:"task_id"`
	WorkflowStage     string     `json:"workflow_stage"`
	Action            string     `json:"action"`
	EntityType        string     `json:"entity_type"`
	EntityID          string     `json:"entity_id"`
	GenerationVersion int        `json:"generation_version"`
	Status            string     `json:"status"`
	RetryCount        int        `json:"retry_count"`
	MaxRetries        int        `json:"max_retries"`
	ErrorCode         *string    `json:"error_code,omitempty"`
	ErrorMessage      *string    `json:"error_message,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type FailedWorkflowTask struct {
	TaskID            string    `json:"task_id"`
	ProjectID         string    `json:"project_id"`
	NovelName         string    `json:"novel_name"`
	WorkflowStage     string    `json:"workflow_stage"`
	Action            string    `json:"action"`
	EntityType        string    `json:"entity_type"`
	EntityID          string    `json:"entity_id"`
	GenerationVersion int       `json:"generation_version"`
	RetryCount        int       `json:"retry_count"`
	MaxRetries        int       `json:"max_retries"`
	ErrorCode         *string   `json:"error_code,omitempty"`
	ErrorMessage      *string   `json:"error_message,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type FailedWorkflowTaskResult struct {
	Items []FailedWorkflowTask `json:"items"`
	Total int                  `json:"total"`
}

type ReviewTask struct {
	ReviewID            string     `json:"review_id"`
	Stage               string     `json:"stage"`
	EntityType          string     `json:"entity_type"`
	EntityID            string     `json:"entity_id"`
	ReviewStatus        string     `json:"review_status"`
	ReviewComment       *string    `json:"review_comment,omitempty"`
	RejectionReason     *string    `json:"rejection_reason,omitempty"`
	RevisionInstruction *string    `json:"revision_instruction,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	ReviewedAt          *time.Time `json:"reviewed_at,omitempty"`
}

type ReviewCenterItem struct {
	ReviewID              string          `json:"review_id"`
	ProjectID             string          `json:"project_id"`
	NovelName             string          `json:"novel_name"`
	Stage                 string          `json:"stage"`
	EntityType            string          `json:"entity_type"`
	EntityID              string          `json:"entity_id"`
	ReviewStatus          string          `json:"review_status"`
	ReviewComment         *string         `json:"review_comment,omitempty"`
	RejectionReason       *string         `json:"rejection_reason,omitempty"`
	RevisionInstruction   *string         `json:"revision_instruction,omitempty"`
	Metadata              json.RawMessage `json:"metadata"`
	RegeneratedByReviewID *string         `json:"regenerated_by_review_id,omitempty"`
	RegeneratedByEntityID *string         `json:"regenerated_by_entity_id,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	ReviewedAt            *time.Time      `json:"reviewed_at,omitempty"`
}

type ReviewProjectOption struct {
	ProjectID string `json:"project_id"`
	NovelName string `json:"novel_name"`
}

type ReviewSummary struct {
	Total    int `json:"total"`
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Rejected int `json:"rejected"`
}

type ReviewFacets struct {
	Projects []ReviewProjectOption `json:"projects"`
	Stages   []string              `json:"stages"`
	Statuses []string              `json:"statuses"`
}

type ReviewListResult struct {
	Items   []ReviewCenterItem `json:"items"`
	Total   int                `json:"total"`
	Page    int                `json:"page"`
	Limit   int                `json:"limit"`
	Summary ReviewSummary      `json:"summary"`
	Facets  ReviewFacets       `json:"facets"`
}

type ReviewMedia struct {
	Kind        string  `json:"kind"`
	Label       string  `json:"label"`
	OriginalURL *string `json:"original_url,omitempty"`
	StorageURL  *string `json:"storage_url,omitempty"`
	PreviewURL  *string `json:"preview_url,omitempty"`
	MediaURL    *string `json:"media_url,omitempty"`
}

type ReviewContent struct {
	ReviewID     string          `json:"review_id"`
	ProjectID    string          `json:"project_id"`
	NovelName    string          `json:"novel_name"`
	Stage        string          `json:"stage"`
	EntityType   string          `json:"entity_type"`
	EntityID     string          `json:"entity_id"`
	ReviewStatus string          `json:"review_status"`
	ArtifactType string          `json:"artifact_type"`
	Metadata     json.RawMessage `json:"metadata"`
	Artifact     json.RawMessage `json:"artifact"`
	Media        []ReviewMedia   `json:"media"`
	TestMode     bool            `json:"test_mode"`
}

type MediaAsset struct {
	AssetID           string    `json:"asset_id"`
	AssetType         string    `json:"asset_type"`
	ProjectID         string    `json:"project_id"`
	NovelName         string    `json:"novel_name"`
	EpisodeID         *string   `json:"episode_id,omitempty"`
	EntityType        string    `json:"entity_type"`
	EntityID          string    `json:"entity_id"`
	Subtype           string    `json:"subtype"`
	MediaKind         string    `json:"media_kind"`
	Status            string    `json:"status"`
	ReviewStatus      string    `json:"review_status"`
	OriginalURL       *string   `json:"original_url,omitempty"`
	StorageURL        *string   `json:"storage_url,omitempty"`
	ThumbnailURL      *string   `json:"thumbnail_url,omitempty"`
	MediaURL          *string   `json:"media_url,omitempty"`
	PreviewURL        *string   `json:"preview_url,omitempty"`
	Width             *int      `json:"width,omitempty"`
	Height            *int      `json:"height,omitempty"`
	DurationMS        *int64    `json:"duration_ms,omitempty"`
	Provider          *string   `json:"provider,omitempty"`
	Model             *string   `json:"model,omitempty"`
	GenerationVersion int       `json:"generation_version"`
	IsCurrent         bool      `json:"is_current"`
	ErrorCode         *string   `json:"error_code,omitempty"`
	ErrorMessage      *string   `json:"error_message,omitempty"`
	TaskID            *string   `json:"task_id,omitempty"`
	RetryCount        int       `json:"retry_count"`
	MaxRetries        int       `json:"max_retries"`
	TestMode          bool      `json:"test_mode"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type MediaAssetReplacement struct {
	SourceAssetType string
	SourceAssetID   string
	AssetID         string
	StorageURL      string
	ContentHash     string
	Width           *int
	Height          *int
	DurationMS      *int64
}

type MediaAssetSummary struct {
	Total  int `json:"total"`
	Images int `json:"images"`
	Videos int `json:"videos"`
	Audio  int `json:"audio"`
}

type MediaAssetFacets struct {
	Projects []ReviewProjectOption `json:"projects"`
	Types    []string              `json:"types"`
	Statuses []string              `json:"statuses"`
}

type MediaAssetListResult struct {
	Items   []MediaAsset      `json:"items"`
	Total   int               `json:"total"`
	Page    int               `json:"page"`
	Limit   int               `json:"limit"`
	Summary MediaAssetSummary `json:"summary"`
	Facets  MediaAssetFacets  `json:"facets"`
}

type ReviewContext struct {
	ReviewID     string          `json:"review_id"`
	ProjectID    string          `json:"project_id"`
	Stage        string          `json:"stage"`
	EntityType   string          `json:"entity_type"`
	EntityID     string          `json:"entity_id"`
	ReviewStatus string          `json:"review_status"`
	Metadata     json.RawMessage `json:"metadata"`
	EpisodeID    *string         `json:"episode_id,omitempty"`
	TestMode     bool            `json:"test_mode"`
}

type FailedTaskContext struct {
	TaskID            string          `json:"task_id"`
	WorkflowStage     string          `json:"workflow_stage"`
	EntityType        string          `json:"entity_type"`
	EntityID          string          `json:"entity_id"`
	GenerationVersion int             `json:"generation_version"`
	Status            string          `json:"status"`
	InputData         json.RawMessage `json:"input_data"`
}

type FlowActionContext struct {
	ProjectID              string             `json:"project_id"`
	NovelName              string             `json:"novel_name"`
	TargetEpisodeCount     int                `json:"target_episode_count"`
	EpisodeDurationSeconds int                `json:"episode_duration_seconds"`
	VisualStyle            string             `json:"visual_style"`
	AspectRatio            string             `json:"aspect_ratio"`
	TargetPlatform         string             `json:"target_platform"`
	CurrentStage           string             `json:"current_stage"`
	Status                 string             `json:"status"`
	TestMode               bool               `json:"test_mode"`
	ActiveTasks            int                `json:"active_tasks"`
	PendingReviews         int                `json:"pending_reviews"`
	EpisodeID              *string            `json:"episode_id,omitempty"`
	OriginalInput          json.RawMessage    `json:"original_input"`
	Task                   *FailedTaskContext `json:"task,omitempty"`
}

type Novel struct {
	NovelID      string    `json:"novel_id"`
	Name         string    `json:"name"`
	SourceType   string    `json:"source_type"`
	Encoding     string    `json:"encoding"`
	TotalChars   int       `json:"total_chars"`
	ChapterCount int       `json:"chapter_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type StoryBible struct {
	StoryBibleID   string    `json:"story_bible_id"`
	Version        int       `json:"version"`
	Status         string    `json:"status"`
	CharacterCount int       `json:"character_count"`
	LocationCount  int       `json:"location_count"`
	KeyEventCount  int       `json:"key_event_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Episode struct {
	EpisodeID                string    `json:"episode_id"`
	EpisodeNumber            int       `json:"episode_number"`
	Title                    string    `json:"title"`
	Logline                  string    `json:"logline"`
	EstimatedDurationSeconds int       `json:"estimated_duration_seconds"`
	Status                   string    `json:"status"`
	Version                  int       `json:"version"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type EpisodeScript struct {
	ScriptID                 string    `json:"script_id"`
	EpisodeID                string    `json:"episode_id"`
	Version                  int       `json:"version"`
	Title                    string    `json:"title"`
	EstimatedDurationSeconds int       `json:"estimated_duration_seconds"`
	DialogueCharCount        int       `json:"dialogue_char_count"`
	SceneCount               int       `json:"scene_count"`
	Status                   string    `json:"status"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type Storyboard struct {
	StoryboardID             string    `json:"storyboard_id"`
	EpisodeID                string    `json:"episode_id"`
	ScriptID                 string    `json:"script_id"`
	Version                  int       `json:"version"`
	TotalShots               int       `json:"total_shots"`
	EstimatedDurationSeconds int       `json:"estimated_duration_seconds"`
	Status                   string    `json:"status"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type ListResult struct {
	Items []Project `json:"items"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
}

type ProjectArchiveResult struct {
	ProjectID string    `json:"project_id"`
	Status    string    `json:"status"`
	ChangedAt time.Time `json:"changed_at"`
}

const mediaAssetsCTE = `WITH media_assets AS (
	SELECT 'generated_assets'::text asset_type,ga.asset_id,ga.project_id,p.novel_name,
		NULL::text episode_id,ga.entity_type,ga.entity_id,ga.asset_type subtype,'image'::text media_kind,
		ga.status,ga.review_status,ga.original_url,ga.storage_url,ga.thumbnail_url,ga.width,ga.height,
		NULL::bigint duration_ms,ga.provider,ga.model,ga.generation_version,
		ga.selected_as_primary is_current,ga.error_code,ga.error_message,
		(SELECT t.task_id FROM drama.image_generation_tasks t WHERE t.asset_id=ga.asset_id ORDER BY t.created_at DESC LIMIT 1) task_id,
		ga.retry_count,COALESCE((SELECT t.max_retries FROM drama.image_generation_tasks t WHERE t.asset_id=ga.asset_id ORDER BY t.created_at DESC LIMIT 1),3) max_retries,
		p.test_mode,ga.created_at,ga.updated_at
	FROM drama.generated_assets ga JOIN drama.projects p ON p.project_id=ga.project_id
	UNION ALL
	SELECT 'storyboard_images',si.storyboard_image_id,si.project_id,p.novel_name,
		si.episode_id,'shot',si.shot_id,'storyboard_frame','image',si.status,si.review_status,
		si.image_url,si.storage_url,NULL::text,NULL::int,NULL::int,NULL::bigint,si.provider,si.model,
		si.generation_version,si.is_current,
		(SELECT t.error_code FROM drama.image_generation_tasks t WHERE t.project_id=si.project_id AND t.shot_id=si.shot_id AND t.generation_version=si.generation_version ORDER BY t.created_at DESC LIMIT 1),
		(SELECT t.error_message FROM drama.image_generation_tasks t WHERE t.project_id=si.project_id AND t.shot_id=si.shot_id AND t.generation_version=si.generation_version ORDER BY t.created_at DESC LIMIT 1),
		(SELECT t.task_id FROM drama.image_generation_tasks t WHERE t.project_id=si.project_id AND t.shot_id=si.shot_id AND t.generation_version=si.generation_version ORDER BY t.created_at DESC LIMIT 1),
		COALESCE((SELECT t.retry_count FROM drama.image_generation_tasks t WHERE t.project_id=si.project_id AND t.shot_id=si.shot_id AND t.generation_version=si.generation_version ORDER BY t.created_at DESC LIMIT 1),0),
		COALESCE((SELECT t.max_retries FROM drama.image_generation_tasks t WHERE t.project_id=si.project_id AND t.shot_id=si.shot_id AND t.generation_version=si.generation_version ORDER BY t.created_at DESC LIMIT 1),3),
		p.test_mode,si.created_at,si.updated_at
	FROM drama.storyboard_images si JOIN drama.projects p ON p.project_id=si.project_id
	UNION ALL
	SELECT 'shot_videos',sv.shot_video_id,sv.project_id,p.novel_name,
		sv.episode_id,'shot',sv.shot_id,'shot_video','video',sv.status,sv.review_status,
		sv.original_url,sv.storage_url,sv.thumbnail_url,sv.width,sv.height,
		CASE WHEN sv.actual_duration_seconds IS NULL THEN NULL ELSE round(sv.actual_duration_seconds*1000)::bigint END,
		sv.provider,sv.model,sv.generation_version,sv.is_current,
		(SELECT t.error_code FROM drama.video_generation_tasks t WHERE t.project_id=sv.project_id AND t.shot_id=sv.shot_id AND t.generation_version=sv.generation_version ORDER BY t.created_at DESC LIMIT 1),
		(SELECT t.error_message FROM drama.video_generation_tasks t WHERE t.project_id=sv.project_id AND t.shot_id=sv.shot_id AND t.generation_version=sv.generation_version ORDER BY t.created_at DESC LIMIT 1),
		(SELECT t.task_id FROM drama.video_generation_tasks t WHERE t.project_id=sv.project_id AND t.shot_id=sv.shot_id AND t.generation_version=sv.generation_version ORDER BY t.created_at DESC LIMIT 1),
		COALESCE((SELECT t.retry_count FROM drama.video_generation_tasks t WHERE t.project_id=sv.project_id AND t.shot_id=sv.shot_id AND t.generation_version=sv.generation_version ORDER BY t.created_at DESC LIMIT 1),0),
		COALESCE((SELECT t.max_retries FROM drama.video_generation_tasks t WHERE t.project_id=sv.project_id AND t.shot_id=sv.shot_id AND t.generation_version=sv.generation_version ORDER BY t.created_at DESC LIMIT 1),3),
		p.test_mode,sv.created_at,sv.updated_at
	FROM drama.shot_videos sv JOIN drama.projects p ON p.project_id=sv.project_id
	UNION ALL
	SELECT 'dialogue_audio',da.dialogue_audio_id,da.project_id,p.novel_name,
		da.episode_id,'dialogue',da.dialogue_id,da.dialogue_type,'audio',da.status,da.review_status,
		da.original_url,da.storage_url,da.waveform_url,NULL::int,NULL::int,da.actual_duration_ms::bigint,
		da.provider,da.model,da.generation_version,da.is_current,
		(SELECT t.error_code FROM drama.tts_generation_tasks t WHERE t.project_id=da.project_id AND t.dialogue_id=da.dialogue_id AND t.generation_version=da.generation_version ORDER BY t.created_at DESC LIMIT 1),
		(SELECT t.error_message FROM drama.tts_generation_tasks t WHERE t.project_id=da.project_id AND t.dialogue_id=da.dialogue_id AND t.generation_version=da.generation_version ORDER BY t.created_at DESC LIMIT 1),
		(SELECT t.task_id FROM drama.tts_generation_tasks t WHERE t.project_id=da.project_id AND t.dialogue_id=da.dialogue_id AND t.generation_version=da.generation_version ORDER BY t.created_at DESC LIMIT 1),
		COALESCE((SELECT t.retry_count FROM drama.tts_generation_tasks t WHERE t.project_id=da.project_id AND t.dialogue_id=da.dialogue_id AND t.generation_version=da.generation_version ORDER BY t.created_at DESC LIMIT 1),0),
		COALESCE((SELECT t.max_retries FROM drama.tts_generation_tasks t WHERE t.project_id=da.project_id AND t.dialogue_id=da.dialogue_id AND t.generation_version=da.generation_version ORDER BY t.created_at DESC LIMIT 1),3),
		p.test_mode,da.created_at,da.updated_at
	FROM drama.dialogue_audio da JOIN drama.projects p ON p.project_id=da.project_id
	UNION ALL
	SELECT 'episode_masters',em.master_id,em.project_id,p.novel_name,
		em.episode_id,'episode',em.episode_id,em.master_type,'video',em.status,
		COALESCE(fr.review_status,'pending'),NULL::text,COALESCE(NULLIF(em.storage_url,''),em.local_path),em.thumbnail_url,
		em.width,em.height,em.duration_ms,NULL::text,NULL::text,em.generation_version,em.is_current,
		rj.error_code,rj.error_message,rj.render_job_id,COALESCE(rj.retry_count,0),COALESCE(rj.max_retries,3),
		p.test_mode,em.created_at,em.updated_at
	FROM drama.episode_masters em JOIN drama.projects p ON p.project_id=em.project_id
	LEFT JOIN LATERAL (
		SELECT review_status FROM drama.final_reviews f WHERE f.master_id=em.master_id
		ORDER BY f.reviewed_at DESC NULLS LAST,f.created_at DESC LIMIT 1
	) fr ON true
	LEFT JOIN drama.render_jobs rj ON rj.render_job_id=em.render_job_id
) `

func New(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	config.MaxConns = 8
	config.MinConns = 1
	config.MaxConnLifetime = 30 * time.Minute
	config.ConnConfig.RuntimeParams["search_path"] = "drama,public"
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(checkCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to short_drama: %w", err)
	}
	writerConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("parse writer database configuration: %w", err)
	}
	writerConfig.MaxConns = 4
	writerConfig.MinConns = 1
	writerConfig.MaxConnLifetime = 30 * time.Minute
	writerConfig.ConnConfig.RuntimeParams["search_path"] = "drama,public"
	writer, err := pgxpool.NewWithConfig(ctx, writerConfig)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create writer database pool: %w", err)
	}
	writerCheckCtx, writerCheckCancel := context.WithTimeout(ctx, 5*time.Second)
	defer writerCheckCancel()
	if err := writer.Ping(writerCheckCtx); err != nil {
		writer.Close()
		pool.Close()
		return nil, fmt.Errorf("connect writer to short_drama: %w", err)
	}
	return &Store{pool: pool, writer: writer}, nil
}

func (s *Store) Close() {
	s.pool.Close()
	s.writer.Close()
}

func (s *Store) BusinessDataSummary(ctx context.Context) (BusinessDataSummary, error) {
	tables, err := s.businessDataTables(ctx)
	if err != nil {
		return BusinessDataSummary{}, err
	}
	summary := BusinessDataSummary{TableCount: len(tables)}
	if len(tables) == 0 {
		return summary, nil
	}
	counts := make([]string, 0, len(tables))
	for _, table := range tables {
		counts = append(counts, "SELECT count(*)::bigint AS count FROM "+pgx.Identifier{"drama", table}.Sanitize())
	}
	query := "SELECT COALESCE(sum(count),0)::bigint FROM (" + strings.Join(counts, " UNION ALL ") + ") business_counts"
	if err := s.writer.QueryRow(ctx, query).Scan(&summary.RowCount); err != nil {
		return BusinessDataSummary{}, fmt.Errorf("count business data: %w", err)
	}
	return summary, nil
}

func (s *Store) ResetBusinessData(ctx context.Context) (BusinessDataSummary, error) {
	summary, err := s.BusinessDataSummary(ctx)
	if err != nil {
		return BusinessDataSummary{}, err
	}
	tables, err := s.businessDataTables(ctx)
	if err != nil {
		return BusinessDataSummary{}, err
	}
	if len(tables) == 0 {
		return summary, nil
	}
	identifiers := make([]string, 0, len(tables))
	for _, table := range tables {
		identifiers = append(identifiers, pgx.Identifier{"drama", table}.Sanitize())
	}
	if _, err := s.writer.Exec(ctx, "TRUNCATE TABLE "+strings.Join(identifiers, ", ")+" RESTART IDENTITY CASCADE"); err != nil {
		return BusinessDataSummary{}, fmt.Errorf("truncate business data: %w", err)
	}
	return summary, nil
}

func (s *Store) businessDataTables(ctx context.Context) ([]string, error) {
	rows, err := s.writer.Query(ctx, `
		SELECT tablename
		FROM pg_tables
		WHERE schemaname='drama'
		ORDER BY tablename`)
	if err != nil {
		return nil, fmt.Errorf("list business tables: %w", err)
	}
	defer rows.Close()
	tables := make([]string, 0, 96)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, fmt.Errorf("scan business table: %w", err)
		}
		if _, preserved := preservedBusinessTables[table]; !preserved {
			tables = append(tables, table)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list business tables: %w", err)
	}
	return tables, nil
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) RecentFailedWorkflowTasks(ctx context.Context, limit int) (FailedWorkflowTaskResult, error) {
	result := FailedWorkflowTaskResult{Items: make([]FailedWorkflowTask, 0)}
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM drama.workflow_tasks WHERE status='failed'`).Scan(&result.Total); err != nil {
		return FailedWorkflowTaskResult{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT w.task_id,w.project_id,p.novel_name,w.workflow_stage,w.action,
		w.entity_type,w.entity_id,w.generation_version,w.retry_count,w.max_retries,w.error_code,
		w.error_message,w.updated_at
		FROM drama.workflow_tasks w JOIN drama.projects p ON p.project_id=w.project_id
		WHERE w.status='failed' ORDER BY w.updated_at DESC LIMIT $1`, limit)
	if err != nil {
		return FailedWorkflowTaskResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item FailedWorkflowTask
		if err := rows.Scan(&item.TaskID, &item.ProjectID, &item.NovelName, &item.WorkflowStage,
			&item.Action, &item.EntityType, &item.EntityID, &item.GenerationVersion, &item.RetryCount,
			&item.MaxRetries, &item.ErrorCode, &item.ErrorMessage, &item.UpdatedAt); err != nil {
			return FailedWorkflowTaskResult{}, err
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func (s *Store) ListProjects(ctx context.Context, query, status string, page, limit int) (ListResult, error) {
	query = strings.TrimSpace(query)
	status = strings.TrimSpace(status)
	where := `WHERE ($1 = '' OR p.novel_name ILIKE '%' || $1 || '%' OR p.project_id ILIKE '%' || $1 || '%')
		AND (($2 = '' AND p.status <> 'cancelled') OR ($2 <> '' AND p.status = $2))`

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM drama.projects p `+where, query, status).Scan(&total); err != nil {
		return ListResult{}, err
	}

	rows, err := s.pool.Query(ctx, `
        SELECT p.project_id, p.novel_name, p.target_episode_count,
		  (SELECT COUNT(DISTINCT e.episode_id) FROM drama.episode_outlines e WHERE e.project_id = p.project_id),
		  (SELECT COUNT(*) FROM drama.novel_chunks c WHERE c.project_id = p.project_id),
		  (SELECT COUNT(*) FROM drama.novel_chunks c WHERE c.project_id = p.project_id AND c.analysis_status = 'completed'),
          p.episode_duration_seconds, p.visual_style, p.aspect_ratio, p.target_platform,
          p.current_stage, p.status, p.test_mode,
          (SELECT COUNT(*) FROM drama.review_tasks r WHERE r.project_id = p.project_id AND r.review_status = 'pending'),
          (SELECT COUNT(*) FROM drama.workflow_tasks w WHERE w.project_id = p.project_id AND w.status = 'failed'),
		  (SELECT COUNT(*) FROM drama.workflow_tasks w WHERE w.project_id = p.project_id AND w.status IN ('pending','running')),
          p.error_message, p.created_at, p.updated_at
        FROM drama.projects p `+where+`
        ORDER BY p.updated_at DESC
        LIMIT $3 OFFSET $4`, query, status, limit, (page-1)*limit)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	items := make([]Project, 0)
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ProjectID, &project.NovelName, &project.TargetEpisodeCount,
			&project.GeneratedEpisodeCount, &project.ChunkCount, &project.CompletedChunkCount,
			&project.EpisodeDurationSeconds, &project.VisualStyle,
			&project.AspectRatio, &project.TargetPlatform, &project.CurrentStage, &project.Status,
			&project.TestMode, &project.PendingReviews, &project.FailedTasks, &project.ActiveTasks, &project.ErrorMessage,
			&project.CreatedAt, &project.UpdatedAt); err != nil {
			return ListResult{}, err
		}
		project.CanArchive = canArchiveProject(project.Status, project.FailedTasks, project.ActiveTasks, project.PendingReviews, 0)
		items = append(items, project)
	}
	return ListResult{Items: items, Total: total, Page: page, Limit: limit}, rows.Err()
}

func canArchiveProject(status string, failedTasks, activeTasks, pendingReviews, finalizedOutputs int) bool {
	if activeTasks > 0 || pendingReviews > 0 || finalizedOutputs > 0 || failedTasks == 0 {
		return false
	}
	return status == "failed" || status == "running" || status == "pending"
}

func (s *Store) ArchiveProject(ctx context.Context, projectID string) (ProjectArchiveResult, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ProjectArchiveResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status, currentStage string
	var config json.RawMessage
	var failedTasks, activeTasks, pendingReviews, finalizedOutputs int
	err = tx.QueryRow(ctx, `SELECT p.status,p.current_stage,COALESCE(p.config,'{}'::jsonb),
		(SELECT COUNT(*) FROM drama.workflow_tasks w WHERE w.project_id=p.project_id AND w.status='failed'),
		(SELECT COUNT(*) FROM drama.workflow_tasks w WHERE w.project_id=p.project_id AND w.status IN ('pending','running')),
		(SELECT COUNT(*) FROM drama.review_tasks r WHERE r.project_id=p.project_id AND r.review_status='pending'),
		(SELECT COUNT(*) FROM drama.episode_masters m WHERE m.project_id=p.project_id AND m.master_type='final' AND m.status='ready')
		FROM drama.projects p WHERE p.project_id=$1 FOR UPDATE`, projectID).Scan(
		&status, &currentStage, &config, &failedTasks, &activeTasks, &pendingReviews, &finalizedOutputs,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectArchiveResult{}, ErrNotFound
	}
	if err != nil {
		return ProjectArchiveResult{}, err
	}
	if status == "cancelled" {
		return ProjectArchiveResult{ProjectID: projectID, Status: status, ChangedAt: time.Now()}, nil
	}
	if !canArchiveProject(status, failedTasks, activeTasks, pendingReviews, finalizedOutputs) {
		return ProjectArchiveResult{}, ErrUnsafeArchive
	}

	configMap := rawJSONMap(config)
	changedAt := time.Now().UTC()
	configMap["cms_archive"] = map[string]any{
		"archived_at": changedAt.Format(time.RFC3339Nano), "previous_status": status,
		"previous_stage": currentStage, "reason": "user_deleted_failed_project",
	}
	updatedConfig, err := json.Marshal(configMap)
	if err != nil {
		return ProjectArchiveResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.projects SET status='cancelled',config=$2::jsonb WHERE project_id=$1`, projectID, updatedConfig); err != nil {
		return ProjectArchiveResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ProjectArchiveResult{}, err
	}
	return ProjectArchiveResult{ProjectID: projectID, Status: "cancelled", ChangedAt: changedAt}, nil
}

func (s *Store) RestoreProject(ctx context.Context, projectID string) (ProjectArchiveResult, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return ProjectArchiveResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var config json.RawMessage
	err = tx.QueryRow(ctx, `SELECT status,COALESCE(config,'{}'::jsonb) FROM drama.projects WHERE project_id=$1 FOR UPDATE`, projectID).Scan(&status, &config)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectArchiveResult{}, ErrNotFound
	}
	if err != nil {
		return ProjectArchiveResult{}, err
	}
	if status != "cancelled" {
		return ProjectArchiveResult{}, ErrNotArchived
	}

	configMap := rawJSONMap(config)
	archive, _ := configMap["cms_archive"].(map[string]any)
	if archive == nil {
		archive = make(map[string]any)
	}
	changedAt := time.Now().UTC()
	archive["restored_at"] = changedAt.Format(time.RFC3339Nano)
	configMap["cms_archive"] = archive
	updatedConfig, err := json.Marshal(configMap)
	if err != nil {
		return ProjectArchiveResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.projects SET status='failed',config=$2::jsonb WHERE project_id=$1`, projectID, updatedConfig); err != nil {
		return ProjectArchiveResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ProjectArchiveResult{}, err
	}
	return ProjectArchiveResult{ProjectID: projectID, Status: "failed", ChangedAt: changedAt}, nil
}

func (s *Store) ListMediaAssets(ctx context.Context, projectID, assetType, reviewStatus string, page, limit int) (MediaAssetListResult, error) {
	projectID = strings.TrimSpace(projectID)
	assetType = strings.TrimSpace(assetType)
	reviewStatus = strings.TrimSpace(reviewStatus)
	where := `WHERE ($1='' OR project_id=$1)
		AND ($2='' OR asset_type=$2)
		AND ($3='' OR review_status=$3)`

	result := MediaAssetListResult{Page: page, Limit: limit}
	if err := s.pool.QueryRow(ctx, mediaAssetsCTE+`SELECT COUNT(*),
		COUNT(*) FILTER (WHERE media_kind='image'),
		COUNT(*) FILTER (WHERE media_kind='video'),
		COUNT(*) FILTER (WHERE media_kind='audio')
		FROM media_assets `+where, projectID, assetType, reviewStatus).Scan(
		&result.Total, &result.Summary.Images, &result.Summary.Videos, &result.Summary.Audio,
	); err != nil {
		return MediaAssetListResult{}, err
	}
	result.Summary.Total = result.Total

	rows, err := s.pool.Query(ctx, mediaAssetsCTE+`SELECT asset_id,asset_type,project_id,novel_name,
		episode_id,entity_type,entity_id,subtype,media_kind,status,review_status,original_url,storage_url,
		thumbnail_url,width,height,duration_ms,provider,model,generation_version,is_current,
		error_code,error_message,task_id,retry_count,max_retries,test_mode,created_at,updated_at
		FROM media_assets `+where+`
		ORDER BY updated_at DESC,asset_id
		LIMIT $4 OFFSET $5`, projectID, assetType, reviewStatus, limit, (page-1)*limit)
	if err != nil {
		return MediaAssetListResult{}, err
	}
	defer rows.Close()
	result.Items = make([]MediaAsset, 0)
	for rows.Next() {
		var item MediaAsset
		if err := rows.Scan(&item.AssetID, &item.AssetType, &item.ProjectID, &item.NovelName,
			&item.EpisodeID, &item.EntityType, &item.EntityID, &item.Subtype, &item.MediaKind,
			&item.Status, &item.ReviewStatus, &item.OriginalURL, &item.StorageURL, &item.ThumbnailURL,
			&item.Width, &item.Height, &item.DurationMS, &item.Provider, &item.Model,
			&item.GenerationVersion, &item.IsCurrent, &item.ErrorCode, &item.ErrorMessage,
			&item.TaskID, &item.RetryCount, &item.MaxRetries, &item.TestMode,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return MediaAssetListResult{}, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return MediaAssetListResult{}, err
	}

	projectRows, err := s.pool.Query(ctx, mediaAssetsCTE+`SELECT DISTINCT project_id,novel_name
		FROM media_assets ORDER BY novel_name,project_id`)
	if err != nil {
		return MediaAssetListResult{}, err
	}
	result.Facets.Projects = make([]ReviewProjectOption, 0)
	for projectRows.Next() {
		var option ReviewProjectOption
		if err := projectRows.Scan(&option.ProjectID, &option.NovelName); err != nil {
			projectRows.Close()
			return MediaAssetListResult{}, err
		}
		result.Facets.Projects = append(result.Facets.Projects, option)
	}
	projectRows.Close()
	if err := projectRows.Err(); err != nil {
		return MediaAssetListResult{}, err
	}
	result.Facets.Types = []string{"generated_assets", "storyboard_images", "shot_videos", "dialogue_audio", "episode_masters"}
	result.Facets.Statuses = []string{"pending", "approved", "rejected", "regenerating"}
	return result, nil
}

func (s *Store) GetMediaAsset(ctx context.Context, assetType, assetID string) (MediaAsset, error) {
	var item MediaAsset
	err := s.pool.QueryRow(ctx, mediaAssetsCTE+`SELECT asset_id,asset_type,project_id,novel_name,
		episode_id,entity_type,entity_id,subtype,media_kind,status,review_status,original_url,storage_url,
		thumbnail_url,width,height,duration_ms,provider,model,generation_version,is_current,
		error_code,error_message,task_id,retry_count,max_retries,test_mode,created_at,updated_at
		FROM media_assets WHERE asset_type=$1 AND asset_id=$2`, assetType, assetID).Scan(
		&item.AssetID, &item.AssetType, &item.ProjectID, &item.NovelName,
		&item.EpisodeID, &item.EntityType, &item.EntityID, &item.Subtype, &item.MediaKind,
		&item.Status, &item.ReviewStatus, &item.OriginalURL, &item.StorageURL, &item.ThumbnailURL,
		&item.Width, &item.Height, &item.DurationMS, &item.Provider, &item.Model,
		&item.GenerationVersion, &item.IsCurrent, &item.ErrorCode, &item.ErrorMessage,
		&item.TaskID, &item.RetryCount, &item.MaxRetries, &item.TestMode,
		&item.CreatedAt, &item.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaAsset{}, ErrNotFound
	}
	return item, err
}

func (s *Store) NextMediaAssetGenerationVersion(ctx context.Context, assetType, assetID string) (int, error) {
	queries := map[string]string{
		"generated_assets": `SELECT COALESCE(MAX(candidate.generation_version),0)+1
			FROM drama.generated_assets source
			JOIN drama.generated_assets candidate ON candidate.project_id=source.project_id
				AND candidate.entity_type=source.entity_type AND candidate.entity_id=source.entity_id
				AND candidate.asset_type=source.asset_type
			WHERE source.asset_id=$1`,
		"storyboard_images": `SELECT COALESCE(MAX(candidate.generation_version),0)+1
			FROM drama.storyboard_images source
			JOIN drama.storyboard_images candidate ON candidate.shot_id=source.shot_id
			WHERE source.storyboard_image_id=$1`,
		"shot_videos": `SELECT COALESCE(MAX(candidate.generation_version),0)+1
			FROM drama.shot_videos source
			JOIN drama.shot_videos candidate ON candidate.shot_id=source.shot_id
			WHERE source.shot_video_id=$1`,
		"dialogue_audio": `SELECT COALESCE(MAX(candidate.generation_version),0)+1
			FROM drama.dialogue_audio source
			JOIN drama.dialogue_audio candidate ON candidate.dialogue_id=source.dialogue_id
			WHERE source.dialogue_audio_id=$1`,
		"episode_masters": `SELECT COALESCE(MAX(candidate.generation_version),0)+1
			FROM drama.episode_masters source
			JOIN drama.episode_masters candidate ON candidate.episode_id=source.episode_id
				AND candidate.master_type=source.master_type
			WHERE source.master_id=$1`,
	}
	query, ok := queries[assetType]
	if !ok {
		return 0, fmt.Errorf("unsupported media asset type %q", assetType)
	}
	var version int
	if err := s.pool.QueryRow(ctx, query, assetID).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if version <= 1 {
		if _, err := s.GetMediaAsset(ctx, assetType, assetID); err != nil {
			return 0, err
		}
	}
	return version, nil
}

func (s *Store) ReplaceMediaAsset(ctx context.Context, replacement MediaAssetReplacement) (MediaAsset, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return MediaAsset{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var commandTag pgconn.CommandTag
	switch replacement.SourceAssetType {
	case "generated_assets":
		var selected bool
		if err = tx.QueryRow(ctx, `SELECT selected_as_primary FROM drama.generated_assets WHERE asset_id=$1 FOR UPDATE`,
			replacement.SourceAssetID).Scan(&selected); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return MediaAsset{}, ErrNotFound
			}
			return MediaAsset{}, err
		}
		if selected {
			if _, err = tx.Exec(ctx, `UPDATE drama.generated_assets SET selected_as_primary=false
				WHERE profile_id=(SELECT profile_id FROM drama.generated_assets WHERE asset_id=$1)
				  AND asset_type=(SELECT asset_type FROM drama.generated_assets WHERE asset_id=$1)`,
				replacement.SourceAssetID); err != nil {
				return MediaAsset{}, err
			}
		}
		commandTag, err = tx.Exec(ctx, `INSERT INTO drama.generated_assets(
			asset_id,project_id,asset_type,entity_type,entity_id,profile_id,generation_version,
			provider,model,prompt,negative_prompt,request_parameters,reference_asset_ids,
			reference_image_urls,status,storage_url,width,height,content_hash,review_status,
			selected_as_primary)
			SELECT $2,source.project_id,source.asset_type,source.entity_type,source.entity_id,source.profile_id,
				(SELECT COALESCE(MAX(v.generation_version),0)+1 FROM drama.generated_assets v
				 WHERE v.project_id=source.project_id AND v.entity_type=source.entity_type
				   AND v.entity_id=source.entity_id AND v.asset_type=source.asset_type),
				'manual_upload','user_upload',source.prompt,source.negative_prompt,
				source.request_parameters||jsonb_build_object('replacement_for',source.asset_id),
				source.reference_asset_ids,source.reference_image_urls,'succeeded',$3,$4,$5,$6,
				'pending',$7
			FROM drama.generated_assets source WHERE source.asset_id=$1`,
			replacement.SourceAssetID, replacement.AssetID, replacement.StorageURL,
			replacement.Width, replacement.Height, replacement.ContentHash, selected)
	case "storyboard_images":
		commandTag, err = tx.Exec(ctx, `INSERT INTO drama.storyboard_images(
			storyboard_image_id,project_id,episode_id,storyboard_id,shot_id,generation_version,
			source_storyboard_version,visual_style_id,character_profile_ids,costume_ids,
			location_profile_id,prop_ids,reference_asset_ids,final_prompt,negative_prompt,
			provider,model,image_asset_id,storage_url,status,auto_qc_status,auto_qc_report,
			review_status,is_current)
			SELECT $2,source.project_id,source.episode_id,source.storyboard_id,source.shot_id,
				(SELECT COALESCE(MAX(v.generation_version),0)+1 FROM drama.storyboard_images v WHERE v.shot_id=source.shot_id),
				source.source_storyboard_version,source.visual_style_id,source.character_profile_ids,
				source.costume_ids,source.location_profile_id,source.prop_ids,source.reference_asset_ids,
				source.final_prompt,source.negative_prompt,'manual_upload','user_upload',
				source.image_asset_id,$3,'succeeded','pending',
				jsonb_build_object('source','manual_upload','replacement_for',source.storyboard_image_id),
				'pending',true
			FROM drama.storyboard_images source WHERE source.storyboard_image_id=$1`,
			replacement.SourceAssetID, replacement.AssetID, replacement.StorageURL)
	case "shot_videos":
		commandTag, err = tx.Exec(ctx, `INSERT INTO drama.shot_videos(
			shot_video_id,project_id,episode_id,storyboard_id,shot_id,storyboard_image_id,
			source_image_generation_version,generation_version,provider,model,video_prompt,
			negative_prompt,reference_image_url,reference_asset_ids,request_parameters,seed,
			requested_duration_seconds,actual_duration_seconds,aspect_ratio,width,height,fps,
			has_audio,storage_url,content_hash,status,auto_qc_status,auto_qc_report,
			review_status,is_current)
			SELECT $2,source.project_id,source.episode_id,source.storyboard_id,source.shot_id,
				source.storyboard_image_id,source.source_image_generation_version,
				(SELECT COALESCE(MAX(v.generation_version),0)+1 FROM drama.shot_videos v WHERE v.shot_id=source.shot_id),
				'manual_upload','user_upload',source.video_prompt,source.negative_prompt,
				source.reference_image_url,source.reference_asset_ids,
				source.request_parameters||jsonb_build_object('replacement_for',source.shot_video_id),
				source.seed,COALESCE(($6::bigint/1000.0),source.requested_duration_seconds),
				COALESCE(($6::bigint/1000.0),source.actual_duration_seconds),
				source.aspect_ratio,COALESCE($4,source.width),COALESCE($5,source.height),source.fps,
				source.has_audio,$3,$7,'succeeded','pending',
				jsonb_build_object('source','manual_upload','replacement_for',source.shot_video_id),
				'pending',true
			FROM drama.shot_videos source WHERE source.shot_video_id=$1`,
			replacement.SourceAssetID, replacement.AssetID, replacement.StorageURL,
			replacement.Width, replacement.Height, replacement.DurationMS, replacement.ContentHash)
	case "dialogue_audio":
		commandTag, err = tx.Exec(ctx, `INSERT INTO drama.dialogue_audio(
			dialogue_audio_id,project_id,episode_id,scene_id,dialogue_id,character_id,
			voice_profile_id,generation_version,dialogue_type,source_text,normalized_text,
			emotion,performance_instruction,requested_speed,provider,model,storage_url,
			actual_duration_ms,content_hash,status,auto_qc_status,auto_qc_report,
			review_status,is_current)
			SELECT $2,source.project_id,source.episode_id,source.scene_id,source.dialogue_id,
				source.character_id,source.voice_profile_id,
				(SELECT COALESCE(MAX(v.generation_version),0)+1 FROM drama.dialogue_audio v WHERE v.dialogue_id=source.dialogue_id),
				source.dialogue_type,source.source_text,source.normalized_text,source.emotion,
				source.performance_instruction,source.requested_speed,'manual_upload','user_upload',
				$3,$4,$5,'succeeded','pending',
				jsonb_build_object('source','manual_upload','replacement_for',source.dialogue_audio_id),
				'pending',true
			FROM drama.dialogue_audio source WHERE source.dialogue_audio_id=$1`,
			replacement.SourceAssetID, replacement.AssetID, replacement.StorageURL,
			replacement.DurationMS, replacement.ContentHash)
	case "episode_masters":
		commandTag, err = tx.Exec(ctx, `INSERT INTO drama.episode_masters(
			master_id,project_id,episode_id,timeline_id,generation_version,master_type,
			storage_url,subtitle_url,subtitle_burned,width,height,aspect_ratio,fps,duration_ms,
			file_size_bytes,video_codec,audio_codec,sample_rate,content_hash,status,is_current)
			SELECT $2,source.project_id,source.episode_id,source.timeline_id,
				(SELECT COALESCE(MAX(v.generation_version),0)+1 FROM drama.episode_masters v
				 WHERE v.episode_id=source.episode_id AND v.master_type=source.master_type),
				source.master_type,$3,source.subtitle_url,source.subtitle_burned,
				COALESCE($4,source.width),COALESCE($5,source.height),source.aspect_ratio,source.fps,
				COALESCE($6,source.duration_ms),source.file_size_bytes,source.video_codec,
				source.audio_codec,source.sample_rate,$7,'ready',true
			FROM drama.episode_masters source WHERE source.master_id=$1`,
			replacement.SourceAssetID, replacement.AssetID, replacement.StorageURL,
			replacement.Width, replacement.Height, replacement.DurationMS, replacement.ContentHash)
	default:
		return MediaAsset{}, fmt.Errorf("unsupported media asset type %q", replacement.SourceAssetType)
	}
	if err != nil {
		return MediaAsset{}, err
	}
	if commandTag.RowsAffected() != 1 {
		return MediaAsset{}, ErrNotFound
	}
	if err = tx.Commit(ctx); err != nil {
		return MediaAsset{}, err
	}
	return s.GetMediaAsset(ctx, replacement.SourceAssetType, replacement.AssetID)
}

func (s *Store) GetProject(ctx context.Context, projectID string) (ProjectDetail, error) {
	var detail ProjectDetail
	err := s.pool.QueryRow(ctx, `
        SELECT p.project_id, p.novel_name, p.target_episode_count,
		  (SELECT COUNT(DISTINCT e.episode_id) FROM drama.episode_outlines e WHERE e.project_id = p.project_id),
          p.episode_duration_seconds, p.visual_style, p.aspect_ratio, p.target_platform,
          p.current_stage, p.status, p.test_mode,
          (SELECT COUNT(*) FROM drama.review_tasks r WHERE r.project_id = p.project_id AND r.review_status = 'pending'),
          (SELECT COUNT(*) FROM drama.workflow_tasks w WHERE w.project_id = p.project_id AND w.status = 'failed'),
          p.config, p.error_message, p.created_at, p.updated_at
        FROM drama.projects p WHERE p.project_id = $1`, projectID).Scan(
		&detail.ProjectID, &detail.NovelName, &detail.TargetEpisodeCount, &detail.GeneratedEpisodeCount,
		&detail.EpisodeDurationSeconds, &detail.VisualStyle, &detail.AspectRatio, &detail.TargetPlatform,
		&detail.CurrentStage, &detail.Status, &detail.TestMode, &detail.PendingReviews, &detail.FailedTasks,
		&detail.Config, &detail.ErrorMessage, &detail.CreatedAt, &detail.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectDetail{}, ErrNotFound
	}
	if err != nil {
		return ProjectDetail{}, err
	}

	err = s.pool.QueryRow(ctx, `
      SELECT
        (SELECT COUNT(*) FROM drama.novel_chapters WHERE project_id = $1),
        (SELECT COUNT(*) FROM drama.novel_chunks WHERE project_id = $1),
        (SELECT COUNT(*) FROM drama.novel_chunks WHERE project_id = $1 AND analysis_status = 'completed'),
		(SELECT COUNT(DISTINCT episode_id) FROM drama.episode_outlines WHERE project_id = $1),
		(SELECT COUNT(*) FROM drama.script_scenes WHERE project_id = $1),
		(SELECT COUNT(*) FROM drama.storyboard_shots WHERE project_id = $1),
        (SELECT COUNT(*) FROM drama.storyboard_images WHERE project_id = $1),
        (SELECT COUNT(*) FROM drama.shot_videos WHERE project_id = $1),
        (SELECT COUNT(*) FROM drama.workflow_tasks WHERE project_id = $1 AND status = 'completed'),
        (SELECT COUNT(*) FROM drama.review_tasks WHERE project_id = $1 AND review_status = 'pending')`, projectID).Scan(
		&detail.Counts.Chapters, &detail.Counts.Chunks, &detail.Counts.CompletedChunks,
		&detail.Counts.Episodes, &detail.Counts.Scenes,
		&detail.Counts.Shots, &detail.Counts.GeneratedImages, &detail.Counts.GeneratedVideos,
		&detail.Counts.CompletedTasks, &detail.Counts.PendingReviews,
	)
	if err != nil {
		return ProjectDetail{}, err
	}
	detail.ChunkCount = detail.Counts.Chunks
	detail.CompletedChunkCount = detail.Counts.CompletedChunks
	if detail.WorkflowTasks, err = s.workflowTasks(ctx, projectID); err != nil {
		return ProjectDetail{}, err
	}
	if detail.ReviewTasks, err = s.reviewTasks(ctx, projectID); err != nil {
		return ProjectDetail{}, err
	}
	if detail.Novels, err = s.novels(ctx, projectID); err != nil {
		return ProjectDetail{}, err
	}
	if detail.StoryBibles, err = s.storyBibles(ctx, projectID); err != nil {
		return ProjectDetail{}, err
	}
	if detail.Episodes, err = s.episodes(ctx, projectID); err != nil {
		return ProjectDetail{}, err
	}
	if detail.Scripts, err = s.scripts(ctx, projectID); err != nil {
		return ProjectDetail{}, err
	}
	if detail.Storyboards, err = s.storyboards(ctx, projectID); err != nil {
		return ProjectDetail{}, err
	}
	if detail.RollingProduction, err = s.GetRollingProduction(ctx, projectID); err != nil {
		return ProjectDetail{}, err
	}
	return detail, nil
}

func (s *Store) workflowTasks(ctx context.Context, projectID string) ([]WorkflowTask, error) {
	rows, err := s.pool.Query(ctx, `SELECT task_id, workflow_stage, action, entity_type, entity_id,
		generation_version, status, retry_count, max_retries, error_code, error_message,
		started_at, completed_at, created_at, updated_at
		FROM drama.workflow_tasks WHERE project_id = $1 ORDER BY updated_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]WorkflowTask, 0)
	for rows.Next() {
		var item WorkflowTask
		if err := rows.Scan(&item.TaskID, &item.WorkflowStage, &item.Action, &item.EntityType, &item.EntityID,
			&item.GenerationVersion, &item.Status, &item.RetryCount, &item.MaxRetries, &item.ErrorCode,
			&item.ErrorMessage, &item.StartedAt, &item.CompletedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) reviewTasks(ctx context.Context, projectID string) ([]ReviewTask, error) {
	rows, err := s.pool.Query(ctx, `SELECT review_id, stage, entity_type, entity_id, review_status,
		review_comment, rejection_reason, revision_instruction, created_at, reviewed_at
		FROM drama.review_tasks WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ReviewTask, 0)
	for rows.Next() {
		var item ReviewTask
		if err := rows.Scan(&item.ReviewID, &item.Stage, &item.EntityType, &item.EntityID, &item.ReviewStatus,
			&item.ReviewComment, &item.RejectionReason, &item.RevisionInstruction, &item.CreatedAt, &item.ReviewedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListReviews(ctx context.Context, projectID, stage, status string, page, limit int) (ReviewListResult, error) {
	projectID = strings.TrimSpace(projectID)
	stage = strings.TrimSpace(stage)
	status = strings.TrimSpace(status)
	where := `WHERE ($1 = '' OR r.project_id = $1)
		AND ($2 = '' OR r.stage = $2)
		AND ($3 = '' OR r.review_status = $3)`

	var result ReviewListResult
	result.Page, result.Limit = page, limit
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM drama.review_tasks r `+where, projectID, stage, status).Scan(&result.Total); err != nil {
		return ReviewListResult{}, err
	}
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*),
		COUNT(*) FILTER (WHERE r.review_status='pending'),
		COUNT(*) FILTER (WHERE r.review_status='approved'),
		COUNT(*) FILTER (WHERE r.review_status='rejected')
		FROM drama.review_tasks r `+where, projectID, stage, status).Scan(
		&result.Summary.Total, &result.Summary.Pending, &result.Summary.Approved, &result.Summary.Rejected,
	); err != nil {
		return ReviewListResult{}, err
	}

	rows, err := s.pool.Query(ctx, `SELECT r.review_id, r.project_id, p.novel_name, r.stage, r.entity_type,
		r.entity_id, r.review_status, r.review_comment, r.rejection_reason, r.revision_instruction,
		COALESCE(r.metadata, '{}'::jsonb),
		(SELECT successor.review_id FROM drama.review_tasks successor
		 WHERE r.stage='visual_asset' AND r.entity_type='generated_asset'
		   AND successor.stage='visual_asset' AND successor.entity_type='generated_asset'
		   AND successor.metadata->>'regenerated_from_asset_id'=r.entity_id
		 ORDER BY successor.created_at DESC LIMIT 1),
		(SELECT successor.entity_id FROM drama.review_tasks successor
		 WHERE r.stage='visual_asset' AND r.entity_type='generated_asset'
		   AND successor.stage='visual_asset' AND successor.entity_type='generated_asset'
		   AND successor.metadata->>'regenerated_from_asset_id'=r.entity_id
		 ORDER BY successor.created_at DESC LIMIT 1),
		r.created_at, r.reviewed_at
		FROM drama.review_tasks r JOIN drama.projects p ON p.project_id=r.project_id `+where+`
		ORDER BY CASE r.review_status WHEN 'pending' THEN 0 ELSE 1 END, r.created_at DESC
		LIMIT $4 OFFSET $5`, projectID, stage, status, limit, (page-1)*limit)
	if err != nil {
		return ReviewListResult{}, err
	}
	defer rows.Close()
	result.Items = make([]ReviewCenterItem, 0)
	for rows.Next() {
		var item ReviewCenterItem
		if err := rows.Scan(&item.ReviewID, &item.ProjectID, &item.NovelName, &item.Stage, &item.EntityType,
			&item.EntityID, &item.ReviewStatus, &item.ReviewComment, &item.RejectionReason,
			&item.RevisionInstruction, &item.Metadata, &item.RegeneratedByReviewID,
			&item.RegeneratedByEntityID, &item.CreatedAt, &item.ReviewedAt); err != nil {
			return ReviewListResult{}, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return ReviewListResult{}, err
	}

	projectRows, err := s.pool.Query(ctx, `SELECT DISTINCT p.project_id,p.novel_name
		FROM drama.review_tasks r JOIN drama.projects p ON p.project_id=r.project_id ORDER BY p.novel_name,p.project_id`)
	if err != nil {
		return ReviewListResult{}, err
	}
	result.Facets.Projects = make([]ReviewProjectOption, 0)
	for projectRows.Next() {
		var option ReviewProjectOption
		if err := projectRows.Scan(&option.ProjectID, &option.NovelName); err != nil {
			projectRows.Close()
			return ReviewListResult{}, err
		}
		result.Facets.Projects = append(result.Facets.Projects, option)
	}
	projectRows.Close()
	if err := projectRows.Err(); err != nil {
		return ReviewListResult{}, err
	}

	stageRows, err := s.pool.Query(ctx, `SELECT DISTINCT stage FROM drama.review_tasks ORDER BY stage`)
	if err != nil {
		return ReviewListResult{}, err
	}
	result.Facets.Stages = make([]string, 0)
	for stageRows.Next() {
		var item string
		if err := stageRows.Scan(&item); err != nil {
			stageRows.Close()
			return ReviewListResult{}, err
		}
		result.Facets.Stages = append(result.Facets.Stages, item)
	}
	stageRows.Close()
	if err := stageRows.Err(); err != nil {
		return ReviewListResult{}, err
	}
	result.Facets.Statuses = []string{"pending", "approved", "rejected", "cancelled"}
	return result, nil
}

func (s *Store) GetReviewContent(ctx context.Context, reviewID string) (ReviewContent, error) {
	var result ReviewContent
	err := s.pool.QueryRow(ctx, `SELECT r.review_id,r.project_id,p.novel_name,r.stage,r.entity_type,
		r.entity_id,r.review_status,COALESCE(r.metadata,'{}'::jsonb),p.test_mode
		FROM drama.review_tasks r JOIN drama.projects p ON p.project_id=r.project_id
		WHERE r.review_id=$1`, reviewID).Scan(
		&result.ReviewID, &result.ProjectID, &result.NovelName, &result.Stage, &result.EntityType,
		&result.EntityID, &result.ReviewStatus, &result.Metadata, &result.TestMode,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewContent{}, ErrNotFound
	}
	if err != nil {
		return ReviewContent{}, err
	}

	result.ArtifactType = reviewArtifactType(result.Stage, result.EntityType)
	result.Media = make([]ReviewMedia, 0)
	var artifact json.RawMessage
	switch result.ArtifactType {
	case "story_bible":
		err = s.pool.QueryRow(ctx, `SELECT to_jsonb(b)-'id'
			FROM drama.story_bibles b WHERE b.project_id=$1 AND b.story_bible_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact)
	case "season_outline":
		err = s.pool.QueryRow(ctx, `SELECT (to_jsonb(s)-'id') || jsonb_build_object(
			'episodes',COALESCE((SELECT jsonb_agg(to_jsonb(e)-'id' ORDER BY e.episode_number,e.version DESC)
				FROM drama.episode_outlines e WHERE e.project_id=s.project_id AND e.season_id=s.season_id),'[]'::jsonb))
			FROM drama.seasons s WHERE s.project_id=$1 AND s.season_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact)
	case "episode_script":
		err = s.pool.QueryRow(ctx, `SELECT (to_jsonb(s)-'id') || jsonb_build_object(
			'scene_details',COALESCE((SELECT jsonb_agg(
				(to_jsonb(sc)-'id') || jsonb_build_object('dialogue_rows',COALESCE(
					(SELECT jsonb_agg(to_jsonb(d)-'id' ORDER BY d.sequence_number)
					 FROM drama.dialogues d WHERE d.scene_id=sc.scene_id),sc.dialogues,'[]'::jsonb))
				ORDER BY sc.scene_number) FROM drama.script_scenes sc WHERE sc.script_id=s.script_id),'[]'::jsonb))
			FROM drama.episode_scripts s WHERE s.project_id=$1 AND s.script_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact)
	case "storyboard":
		err = s.pool.QueryRow(ctx, `SELECT (to_jsonb(b)-'id') || jsonb_build_object(
			'shots',COALESCE((SELECT jsonb_agg(to_jsonb(sh)-'id' ORDER BY sh.shot_order,sh.shot_number)
				FROM drama.storyboard_shots sh WHERE sh.storyboard_id=b.storyboard_id),'[]'::jsonb))
			FROM drama.storyboards b WHERE b.project_id=$1 AND b.storyboard_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact)
	case "visual_asset":
		var media ReviewMedia
		media.Kind, media.Label = "image", "视觉资产"
		err = s.pool.QueryRow(ctx, `SELECT to_jsonb(a)-'id',a.original_url,a.storage_url,a.thumbnail_url
			FROM drama.generated_assets a WHERE a.project_id=$1 AND a.asset_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact, &media.OriginalURL, &media.StorageURL, &media.PreviewURL)
		if err == nil {
			result.Media = append(result.Media, media)
		}
	case "storyboard_image":
		var media ReviewMedia
		media.Kind, media.Label = "image", "分镜图片"
		err = s.pool.QueryRow(ctx, `SELECT to_jsonb(i)-'id',i.image_url,i.storage_url,NULL::text
			FROM drama.storyboard_images i WHERE i.project_id=$1 AND i.storyboard_image_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact, &media.OriginalURL, &media.StorageURL, &media.PreviewURL)
		if err == nil {
			result.Media = append(result.Media, media)
		}
	case "shot_video":
		var media ReviewMedia
		media.Kind, media.Label = "video", "镜头视频"
		err = s.pool.QueryRow(ctx, `SELECT to_jsonb(v)-'id',v.original_url,v.storage_url,v.thumbnail_url
			FROM drama.shot_videos v WHERE v.project_id=$1 AND v.shot_video_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact, &media.OriginalURL, &media.StorageURL, &media.PreviewURL)
		if err == nil {
			result.Media = append(result.Media, media)
		}
	case "dialogue_audio":
		var media ReviewMedia
		media.Kind, media.Label = "audio", "对白音频"
		err = s.pool.QueryRow(ctx, `SELECT to_jsonb(a)-'id',a.original_url,a.storage_url,a.waveform_url
			FROM drama.dialogue_audio a WHERE a.project_id=$1 AND a.dialogue_audio_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact, &media.OriginalURL, &media.StorageURL, &media.PreviewURL)
		if err == nil {
			result.Media = append(result.Media, media)
		}
	case "voice_profile":
		var media ReviewMedia
		media.Kind, media.Label = "audio", "声线样音"
		err = s.pool.QueryRow(ctx, `SELECT to_jsonb(v)-'id',v.sample_audio_url,NULL::text,NULL::text
			FROM drama.voice_profiles v WHERE v.project_id=$1 AND v.voice_profile_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact, &media.OriginalURL, &media.StorageURL, &media.PreviewURL)
		if err == nil && media.OriginalURL != nil {
			result.Media = append(result.Media, media)
		}
	case "final_review":
		metadata := rawJSONMap(result.Metadata)
		masterID := firstNonEmpty(metadataString(metadata, "master_id"), result.EntityID)
		qcReportID := firstNonEmpty(metadataString(metadata, "qc_report_id"), result.EntityID)
		var media ReviewMedia
		media.Kind, media.Label = "video", "最终成片"
		err = s.pool.QueryRow(ctx, `SELECT jsonb_build_object(
			'master',to_jsonb(m)-'id',
			'qc_report',COALESCE((SELECT to_jsonb(q)-'id' FROM drama.qc_reports q
				WHERE q.project_id=m.project_id AND (q.qc_report_id=$3 OR q.master_id=m.master_id)
				ORDER BY q.version DESC LIMIT 1),'{}'::jsonb),
			'final_review',COALESCE((SELECT to_jsonb(f)-'id' FROM drama.final_reviews f
				WHERE f.project_id=m.project_id AND f.master_id=m.master_id ORDER BY f.created_at DESC LIMIT 1),'{}'::jsonb)),
			NULL::text,COALESCE(NULLIF(m.storage_url,''),m.local_path),m.thumbnail_url
			FROM drama.episode_masters m WHERE m.project_id=$1 AND (m.master_id=$2 OR m.master_id=(
				SELECT q.master_id FROM drama.qc_reports q WHERE q.qc_report_id=$3 LIMIT 1))
			ORDER BY m.generation_version DESC LIMIT 1`,
			result.ProjectID, masterID, qcReportID).Scan(&artifact, &media.OriginalURL, &media.StorageURL, &media.PreviewURL)
		if err == nil {
			result.Media = append(result.Media, media)
		}
	case "publication_metadata":
		var media ReviewMedia
		media.Kind, media.Label = "image", "发布封面"
		err = s.pool.QueryRow(ctx, `SELECT (to_jsonb(p)-'id') || jsonb_build_object(
			'master',COALESCE((SELECT to_jsonb(m)-'id' FROM drama.episode_masters m WHERE m.master_id=p.master_id),'{}'::jsonb)),
			p.cover_url,NULL::text,p.cover_url
			FROM drama.publication_metadata p WHERE p.project_id=$1 AND p.metadata_id=$2`,
			result.ProjectID, result.EntityID).Scan(&artifact, &media.OriginalURL, &media.StorageURL, &media.PreviewURL)
		if err == nil && media.OriginalURL != nil {
			result.Media = append(result.Media, media)
		}
	default:
		return ReviewContent{}, ErrNotFound
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewContent{}, ErrNotFound
	}
	if err != nil {
		return ReviewContent{}, err
	}
	result.Artifact = artifact
	return result, nil
}

func reviewArtifactType(stage, entityType string) string {
	stage = strings.ToLower(strings.TrimSpace(stage))
	entityType = strings.ToLower(strings.TrimSpace(entityType))
	switch {
	case matchesValue(stage, entityType, "story_bible"):
		return "story_bible"
	case matchesValue(stage, entityType, "season_outline", "season"):
		return "season_outline"
	case matchesValue(stage, entityType, "episode_script", "script"):
		return "episode_script"
	case matchesValue(stage, entityType, "storyboard"):
		return "storyboard"
	case matchesValue(stage, entityType, "visual_asset", "generated_asset"):
		return "visual_asset"
	case matchesValue(stage, entityType, "storyboard_image"):
		return "storyboard_image"
	case matchesValue(stage, entityType, "shot_video", "video"):
		return "shot_video"
	case matchesValue(stage, entityType, "dialogue_audio", "audio"):
		return "dialogue_audio"
	case matchesValue(stage, entityType, "voice_profile"):
		return "voice_profile"
	case matchesValue(stage, entityType, "final", "final_review", "qc_report"):
		return "final_review"
	case matchesValue(stage, entityType, "publication", "publication_metadata"):
		return "publication_metadata"
	default:
		return ""
	}
}

func matchesValue(stage, entityType string, values ...string) bool {
	for _, value := range values {
		if stage == value || entityType == value {
			return true
		}
	}
	return false
}

func rawJSONMap(raw json.RawMessage) map[string]any {
	value := make(map[string]any)
	_ = json.Unmarshal(raw, &value)
	return value
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Store) GetReviewContext(ctx context.Context, reviewID string) (ReviewContext, error) {
	var review ReviewContext
	err := s.pool.QueryRow(ctx, `SELECT r.review_id,r.project_id,r.stage,r.entity_type,r.entity_id,
		r.review_status,COALESCE(r.metadata,'{}'::jsonb),p.test_mode,
		COALESCE(NULLIF(r.metadata->>'episode_id',''),
			(SELECT episode_id FROM drama.shot_videos WHERE shot_video_id=r.entity_id),
			(SELECT episode_id FROM drama.dialogue_audio WHERE dialogue_audio_id=r.entity_id),
			(SELECT episode_id FROM drama.final_reviews WHERE final_review_id=r.entity_id),
			(SELECT episode_id FROM drama.publication_metadata WHERE metadata_id=r.entity_id))
		FROM drama.review_tasks r JOIN drama.projects p ON p.project_id=r.project_id
		WHERE r.review_id=$1`, reviewID).Scan(&review.ReviewID, &review.ProjectID, &review.Stage,
		&review.EntityType, &review.EntityID, &review.ReviewStatus, &review.Metadata, &review.TestMode, &review.EpisodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewContext{}, ErrNotFound
	}
	return review, err
}

func (s *Store) NextVisualAssetGenerationVersion(ctx context.Context, projectID, profileID, assetType string) (int, error) {
	var version int
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(MAX(generation_version),0)+1
		FROM (
			SELECT generation_version
			FROM drama.generated_assets
			WHERE project_id=$1 AND profile_id=$2 AND asset_type=$3
			UNION ALL
			SELECT generation_version
			FROM drama.image_generation_tasks
			WHERE project_id=$1
			  AND COALESCE(NULLIF(request_payload->>'profile_id',''),
			              NULLIF(request_payload#>>'{payload,profile_id}',''))=$2
			  AND COALESCE(NULLIF(request_payload->>'asset_type',''),
			              NULLIF(request_payload#>>'{payload,asset_type}',''))=$3
		) versions`,
		projectID, profileID, assetType,
	).Scan(&version)
	return version, err
}

func (s *Store) HasSuccessfulVisualAssetRegeneration(ctx context.Context, sourceAssetID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM drama.review_tasks successor
		WHERE successor.stage='visual_asset'
		  AND successor.entity_type='generated_asset'
		  AND successor.metadata->>'regenerated_from_asset_id'=$1
	)`, sourceAssetID).Scan(&exists)
	return exists, err
}

func (s *Store) GetFlowActionContext(ctx context.Context, projectID, taskID string) (FlowActionContext, error) {
	var action FlowActionContext
	err := s.pool.QueryRow(ctx, `SELECT p.project_id,p.novel_name,p.target_episode_count,
		p.episode_duration_seconds,p.visual_style,p.aspect_ratio,p.target_platform,p.current_stage,
		p.status,p.test_mode,
		(SELECT COUNT(*) FROM drama.workflow_tasks w WHERE w.project_id=p.project_id AND w.status IN ('pending','running')),
		(SELECT COUNT(*) FROM drama.review_tasks r WHERE r.project_id=p.project_id AND r.review_status='pending'),
		(SELECT episode_id FROM drama.episode_outlines e WHERE e.project_id=p.project_id
		 ORDER BY CASE e.status WHEN 'approved' THEN 0 ELSE 1 END,e.episode_number,e.version DESC LIMIT 1),
		COALESCE((SELECT input_data FROM drama.workflow_tasks w WHERE w.project_id=p.project_id
		 AND w.workflow_stage='orchestrator' ORDER BY w.created_at DESC LIMIT 1),'{}'::jsonb)
		FROM drama.projects p WHERE p.project_id=$1`, projectID).Scan(
		&action.ProjectID, &action.NovelName, &action.TargetEpisodeCount, &action.EpisodeDurationSeconds,
		&action.VisualStyle, &action.AspectRatio, &action.TargetPlatform, &action.CurrentStage,
		&action.Status, &action.TestMode, &action.ActiveTasks, &action.PendingReviews,
		&action.EpisodeID, &action.OriginalInput,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return FlowActionContext{}, ErrNotFound
	}
	if err != nil {
		return FlowActionContext{}, err
	}
	if strings.TrimSpace(taskID) == "" {
		return action, nil
	}
	var task FailedTaskContext
	err = s.pool.QueryRow(ctx, `SELECT task_id,workflow_stage,entity_type,entity_id,generation_version,status,
		COALESCE(input_data,'{}'::jsonb) FROM drama.workflow_tasks WHERE project_id=$1 AND task_id=$2`, projectID, taskID).Scan(
		&task.TaskID, &task.WorkflowStage, &task.EntityType, &task.EntityID,
		&task.GenerationVersion, &task.Status, &task.InputData,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return FlowActionContext{}, ErrNotFound
	}
	if err != nil {
		return FlowActionContext{}, err
	}
	action.Task = &task
	return action, nil
}

func (s *Store) novels(ctx context.Context, projectID string) ([]Novel, error) {
	rows, err := s.pool.Query(ctx, `SELECT novel_id, name, source_type, encoding, total_chars, chapter_count, created_at, updated_at
		FROM drama.novels WHERE project_id = $1 ORDER BY updated_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Novel, 0)
	for rows.Next() {
		var item Novel
		if err := rows.Scan(&item.NovelID, &item.Name, &item.SourceType, &item.Encoding, &item.TotalChars,
			&item.ChapterCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) storyBibles(ctx context.Context, projectID string) ([]StoryBible, error) {
	rows, err := s.pool.Query(ctx, `SELECT story_bible_id, version, status,
		COALESCE(jsonb_array_length(characters), 0), COALESCE(jsonb_array_length(locations), 0),
		COALESCE(jsonb_array_length(key_events), 0), created_at, updated_at
		FROM drama.story_bibles WHERE project_id = $1 ORDER BY version DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]StoryBible, 0)
	for rows.Next() {
		var item StoryBible
		if err := rows.Scan(&item.StoryBibleID, &item.Version, &item.Status, &item.CharacterCount,
			&item.LocationCount, &item.KeyEventCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) episodes(ctx context.Context, projectID string) ([]Episode, error) {
	rows, err := s.pool.Query(ctx, `SELECT episode_id, episode_number, title, logline,
		estimated_duration_seconds, status, version, created_at, updated_at
		FROM drama.episode_outlines WHERE project_id = $1 ORDER BY episode_number, version DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Episode, 0)
	for rows.Next() {
		var item Episode
		if err := rows.Scan(&item.EpisodeID, &item.EpisodeNumber, &item.Title, &item.Logline,
			&item.EstimatedDurationSeconds, &item.Status, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) scripts(ctx context.Context, projectID string) ([]EpisodeScript, error) {
	rows, err := s.pool.Query(ctx, `SELECT s.script_id, s.episode_id, s.version, s.title,
		s.estimated_duration_seconds, s.dialogue_char_count,
		(SELECT COUNT(*) FROM drama.script_scenes sc WHERE sc.script_id = s.script_id),
		s.status, s.created_at, s.updated_at
		FROM drama.episode_scripts s WHERE s.project_id = $1 ORDER BY s.updated_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EpisodeScript, 0)
	for rows.Next() {
		var item EpisodeScript
		if err := rows.Scan(&item.ScriptID, &item.EpisodeID, &item.Version, &item.Title,
			&item.EstimatedDurationSeconds, &item.DialogueCharCount, &item.SceneCount,
			&item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) storyboards(ctx context.Context, projectID string) ([]Storyboard, error) {
	rows, err := s.pool.Query(ctx, `SELECT storyboard_id, episode_id, script_id, version, total_shots,
		estimated_duration_seconds, status, created_at, updated_at
		FROM drama.storyboards WHERE project_id = $1 ORDER BY updated_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Storyboard, 0)
	for rows.Next() {
		var item Storyboard
		if err := rows.Scan(&item.StoryboardID, &item.EpisodeID, &item.ScriptID, &item.Version,
			&item.TotalShots, &item.EstimatedDurationSeconds, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type DatabaseStats struct {
	Version          string `json:"version"`
	Database         string `json:"database"`
	SchemaTableCount int    `json:"schema_table_count"`
	ProjectCount     int    `json:"project_count"`
	ActiveTasks      int    `json:"active_tasks"`
	PendingReviews   int    `json:"pending_reviews"`
}

func (s *Store) DatabaseStats(ctx context.Context) (DatabaseStats, error) {
	var stats DatabaseStats
	err := s.pool.QueryRow(ctx, `
      SELECT current_database(), current_setting('server_version'),
        (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'drama'),
        (SELECT COUNT(*) FROM drama.projects),
        (SELECT COUNT(*) FROM drama.workflow_tasks WHERE status IN ('pending','running')),
        (SELECT COUNT(*) FROM drama.review_tasks WHERE review_status = 'pending')`).Scan(
		&stats.Database, &stats.Version, &stats.SchemaTableCount, &stats.ProjectCount, &stats.ActiveTasks, &stats.PendingReviews,
	)
	return stats, err
}
