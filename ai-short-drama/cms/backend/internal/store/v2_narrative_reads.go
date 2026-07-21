package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ListChapterRevisions(ctx context.Context, chapterID string) ([]ChapterRevisionHistoryItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT chapter_id,chapter_revision_id,revision_number,title,content_hash,char_count,created_at
		FROM drama.chapter_revisions WHERE chapter_id=$1 ORDER BY revision_number DESC`, chapterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ChapterRevisionHistoryItem, 0)
	for rows.Next() {
		var item ChapterRevisionHistoryItem
		if err := rows.Scan(&item.ChapterID, &item.ChapterRevisionID, &item.RevisionNumber, &item.Title, &item.ContentHash, &item.CharCount, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.source_chapters WHERE chapter_id=$1)`, chapterID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return items, nil
}

func (s *Store) ListNarrativeIRRevisions(ctx context.Context, sourceVersionID string) ([]NarrativeIRRevisionSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT ir_revision_id,source_version_id,revision_number,status,revision_scope,extractor_version,
		changed_chapter_ids,validation_summary,created_at,published_at FROM drama.narrative_ir_revisions
		WHERE source_version_id=$1 ORDER BY revision_number DESC,created_at DESC`, sourceVersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]NarrativeIRRevisionSummary, 0)
	for rows.Next() {
		var item NarrativeIRRevisionSummary
		if err := rows.Scan(&item.IRRevisionID, &item.SourceVersionID, &item.RevisionNumber,
			&item.Status, &item.RevisionScope, &item.ExtractorVersion, &item.ChangedChapterIDs, &item.ValidationSummary, &item.CreatedAt, &item.PublishedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.source_versions WHERE source_version_id=$1)`, sourceVersionID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
	}
	return items, nil
}

func (s *Store) ListStoryArcs(ctx context.Context, irRevisionID string) ([]StoryArcSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT story_arc_revision_id,ir_revision_id,chapter_id,title,summary,arc_type,confidence
		FROM drama.story_arc_revisions WHERE ir_revision_id=$1 ORDER BY title,story_arc_revision_id`, irRevisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]StoryArcSummary, 0)
	for rows.Next() {
		var item StoryArcSummary
		if err := rows.Scan(&item.StoryArcRevisionID, &item.IRRevisionID, &item.ChapterID, &item.Title, &item.Summary, &item.ArcType, &item.Confidence); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		var status string
		err := s.pool.QueryRow(ctx, `SELECT status FROM drama.narrative_ir_revisions WHERE ir_revision_id=$1`, irRevisionID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}
