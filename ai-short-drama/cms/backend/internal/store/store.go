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
