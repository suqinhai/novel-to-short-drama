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
	Counts ProjectCounts `json:"counts"`
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
	return detail, err
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
