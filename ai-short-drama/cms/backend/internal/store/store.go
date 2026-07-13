package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("record not found")

type Store struct {
	pool *pgxpool.Pool
}

type Project struct {
	ProjectID              string          `json:"project_id"`
	NovelName              string          `json:"novel_name"`
	TargetEpisodeCount     int             `json:"target_episode_count"`
	GeneratedEpisodeCount  int             `json:"generated_episode_count"`
	EpisodeDurationSeconds int             `json:"episode_duration_seconds"`
	VisualStyle            string          `json:"visual_style"`
	AspectRatio            string          `json:"aspect_ratio"`
	TargetPlatform         string          `json:"target_platform"`
	CurrentStage           string          `json:"current_stage"`
	Status                 string          `json:"status"`
	TestMode               bool            `json:"test_mode"`
	PendingReviews         int             `json:"pending_reviews"`
	FailedTasks            int             `json:"failed_tasks"`
	Config                 json.RawMessage `json:"config,omitempty"`
	ErrorMessage           *string         `json:"error_message,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

type ProjectCounts struct {
	Chapters        int `json:"chapters"`
	Chunks          int `json:"chunks"`
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
	Counts        ProjectCounts   `json:"counts"`
	WorkflowTasks []WorkflowTask  `json:"workflow_tasks"`
	ReviewTasks   []ReviewTask    `json:"review_tasks"`
	Novels        []Novel         `json:"novels"`
	StoryBibles   []StoryBible    `json:"story_bibles"`
	Episodes      []Episode       `json:"episodes"`
	Scripts       []EpisodeScript `json:"scripts"`
	Storyboards   []Storyboard    `json:"storyboards"`
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
	ReviewID            string          `json:"review_id"`
	ProjectID           string          `json:"project_id"`
	NovelName           string          `json:"novel_name"`
	Stage               string          `json:"stage"`
	EntityType          string          `json:"entity_type"`
	EntityID            string          `json:"entity_id"`
	ReviewStatus        string          `json:"review_status"`
	ReviewComment       *string         `json:"review_comment,omitempty"`
	RejectionReason     *string         `json:"rejection_reason,omitempty"`
	RevisionInstruction *string         `json:"revision_instruction,omitempty"`
	Metadata            json.RawMessage `json:"metadata"`
	CreatedAt           time.Time       `json:"created_at"`
	ReviewedAt          *time.Time      `json:"reviewed_at,omitempty"`
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

type MediaAsset struct {
	AssetID      string    `json:"asset_id"`
	AssetType    string    `json:"asset_type"`
	ProjectID    string    `json:"project_id"`
	NovelName    string    `json:"novel_name"`
	EpisodeID    *string   `json:"episode_id,omitempty"`
	EntityType   string    `json:"entity_type"`
	EntityID     string    `json:"entity_id"`
	Subtype      string    `json:"subtype"`
	MediaKind    string    `json:"media_kind"`
	Status       string    `json:"status"`
	ReviewStatus string    `json:"review_status"`
	OriginalURL  *string   `json:"original_url,omitempty"`
	StorageURL   *string   `json:"storage_url,omitempty"`
	ThumbnailURL *string   `json:"thumbnail_url,omitempty"`
	MediaURL     *string   `json:"media_url,omitempty"`
	PreviewURL   *string   `json:"preview_url,omitempty"`
	Width        *int      `json:"width,omitempty"`
	Height       *int      `json:"height,omitempty"`
	DurationMS   *int64    `json:"duration_ms,omitempty"`
	Provider     *string   `json:"provider,omitempty"`
	Model        *string   `json:"model,omitempty"`
	IsCurrent    bool      `json:"is_current"`
	ErrorMessage *string   `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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

const mediaAssetsCTE = `WITH media_assets AS (
	SELECT 'generated_assets'::text asset_type,ga.asset_id,ga.project_id,p.novel_name,
		NULL::text episode_id,ga.entity_type,ga.entity_id,ga.asset_type subtype,'image'::text media_kind,
		ga.status,ga.review_status,ga.original_url,ga.storage_url,ga.thumbnail_url,ga.width,ga.height,
		NULL::bigint duration_ms,ga.provider,ga.model,true is_current,ga.error_message,ga.created_at,ga.updated_at
	FROM drama.generated_assets ga JOIN drama.projects p ON p.project_id=ga.project_id
	UNION ALL
	SELECT 'storyboard_images',si.storyboard_image_id,si.project_id,p.novel_name,
		si.episode_id,'shot',si.shot_id,'storyboard_frame','image',si.status,si.review_status,
		si.image_url,si.storage_url,NULL::text,NULL::int,NULL::int,NULL::bigint,si.provider,si.model,
		si.is_current,NULL::text,si.created_at,si.updated_at
	FROM drama.storyboard_images si JOIN drama.projects p ON p.project_id=si.project_id
	UNION ALL
	SELECT 'shot_videos',sv.shot_video_id,sv.project_id,p.novel_name,
		sv.episode_id,'shot',sv.shot_id,'shot_video','video',sv.status,sv.review_status,
		sv.original_url,sv.storage_url,sv.thumbnail_url,sv.width,sv.height,
		CASE WHEN sv.actual_duration_seconds IS NULL THEN NULL ELSE round(sv.actual_duration_seconds*1000)::bigint END,
		sv.provider,sv.model,sv.is_current,NULL::text,sv.created_at,sv.updated_at
	FROM drama.shot_videos sv JOIN drama.projects p ON p.project_id=sv.project_id
	UNION ALL
	SELECT 'dialogue_audio',da.dialogue_audio_id,da.project_id,p.novel_name,
		da.episode_id,'dialogue',da.dialogue_id,da.dialogue_type,'audio',da.status,da.review_status,
		da.original_url,da.storage_url,da.waveform_url,NULL::int,NULL::int,da.actual_duration_ms::bigint,
		da.provider,da.model,da.is_current,NULL::text,da.created_at,da.updated_at
	FROM drama.dialogue_audio da JOIN drama.projects p ON p.project_id=da.project_id
	UNION ALL
	SELECT 'episode_masters',em.master_id,em.project_id,p.novel_name,
		em.episode_id,'episode',em.episode_id,em.master_type,'video',em.status,
		COALESCE(fr.review_status,'pending'),NULL::text,COALESCE(NULLIF(em.storage_url,''),em.local_path),em.thumbnail_url,
		em.width,em.height,em.duration_ms, NULL::text,NULL::text,em.is_current,NULL::text,em.created_at,em.updated_at
	FROM drama.episode_masters em JOIN drama.projects p ON p.project_id=em.project_id
	LEFT JOIN LATERAL (
		SELECT review_status FROM drama.final_reviews f WHERE f.master_id=em.master_id
		ORDER BY f.reviewed_at DESC NULLS LAST,f.created_at DESC LIMIT 1
	) fr ON true
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
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

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
        AND ($2 = '' OR p.status = $2)`

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM drama.projects p `+where, query, status).Scan(&total); err != nil {
		return ListResult{}, err
	}

	rows, err := s.pool.Query(ctx, `
        SELECT p.project_id, p.novel_name, p.target_episode_count,
		  (SELECT COUNT(DISTINCT e.episode_id) FROM drama.episode_outlines e WHERE e.project_id = p.project_id),
          p.episode_duration_seconds, p.visual_style, p.aspect_ratio, p.target_platform,
          p.current_stage, p.status, p.test_mode,
          (SELECT COUNT(*) FROM drama.review_tasks r WHERE r.project_id = p.project_id AND r.review_status = 'pending'),
          (SELECT COUNT(*) FROM drama.workflow_tasks w WHERE w.project_id = p.project_id AND w.status = 'failed'),
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
			&project.GeneratedEpisodeCount, &project.EpisodeDurationSeconds, &project.VisualStyle,
			&project.AspectRatio, &project.TargetPlatform, &project.CurrentStage, &project.Status,
			&project.TestMode, &project.PendingReviews, &project.FailedTasks, &project.ErrorMessage,
			&project.CreatedAt, &project.UpdatedAt); err != nil {
			return ListResult{}, err
		}
		items = append(items, project)
	}
	return ListResult{Items: items, Total: total, Page: page, Limit: limit}, rows.Err()
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
		thumbnail_url,width,height,duration_ms,provider,model,is_current,error_message,created_at,updated_at
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
			&item.Width, &item.Height, &item.DurationMS, &item.Provider, &item.Model, &item.IsCurrent,
			&item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt); err != nil {
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
		(SELECT COUNT(DISTINCT episode_id) FROM drama.episode_outlines WHERE project_id = $1),
		(SELECT COUNT(*) FROM drama.script_scenes WHERE project_id = $1),
		(SELECT COUNT(*) FROM drama.storyboard_shots WHERE project_id = $1),
        (SELECT COUNT(*) FROM drama.storyboard_images WHERE project_id = $1),
        (SELECT COUNT(*) FROM drama.shot_videos WHERE project_id = $1),
        (SELECT COUNT(*) FROM drama.workflow_tasks WHERE project_id = $1 AND status = 'completed'),
        (SELECT COUNT(*) FROM drama.review_tasks WHERE project_id = $1 AND review_status = 'pending')`, projectID).Scan(
		&detail.Counts.Chapters, &detail.Counts.Chunks, &detail.Counts.Episodes, &detail.Counts.Scenes,
		&detail.Counts.Shots, &detail.Counts.GeneratedImages, &detail.Counts.GeneratedVideos,
		&detail.Counts.CompletedTasks, &detail.Counts.PendingReviews,
	)
	if err != nil {
		return ProjectDetail{}, err
	}
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
		COALESCE(r.metadata, '{}'::jsonb), r.created_at, r.reviewed_at
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
			&item.RevisionInstruction, &item.Metadata, &item.CreatedAt, &item.ReviewedAt); err != nil {
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

func (s *Store) GetFlowActionContext(ctx context.Context, projectID, taskID string) (FlowActionContext, error) {
	var action FlowActionContext
	err := s.pool.QueryRow(ctx, `SELECT p.project_id,p.novel_name,p.target_episode_count,
		p.episode_duration_seconds,p.visual_style,p.aspect_ratio,p.target_platform,p.current_stage,
		p.status,p.test_mode,
		(SELECT episode_id FROM drama.episode_outlines e WHERE e.project_id=p.project_id
		 ORDER BY CASE e.status WHEN 'approved' THEN 0 ELSE 1 END,e.episode_number,e.version DESC LIMIT 1),
		COALESCE((SELECT input_data FROM drama.workflow_tasks w WHERE w.project_id=p.project_id
		 AND w.workflow_stage='orchestrator' ORDER BY w.created_at DESC LIMIT 1),'{}'::jsonb)
		FROM drama.projects p WHERE p.project_id=$1`, projectID).Scan(
		&action.ProjectID, &action.NovelName, &action.TargetEpisodeCount, &action.EpisodeDurationSeconds,
		&action.VisualStyle, &action.AspectRatio, &action.TargetPlatform, &action.CurrentStage,
		&action.Status, &action.TestMode, &action.EpisodeID, &action.OriginalInput,
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
