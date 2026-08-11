package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const nlePageLimit = 500

type NLETimelineSummary struct {
	TimelineID          string          `json:"timeline_id"`
	ParentTimelineID    *string         `json:"parent_timeline_id,omitempty"`
	Version             int             `json:"version"`
	VersionReason       string          `json:"version_reason"`
	ApprovalState       string          `json:"approval_state"`
	Status              string          `json:"status"`
	IsCurrent           bool            `json:"is_current"`
	TargetDurationMS    int64           `json:"target_duration_ms"`
	FPS                 float64         `json:"fps"`
	Resolution          string          `json:"resolution"`
	SubtitleConfig      json.RawMessage `json:"subtitle_config"`
	ApprovedRenderJobID *string         `json:"approved_render_job_id,omitempty"`
	ApprovedAt          *time.Time      `json:"approved_at,omitempty"`
	CreatedAt           time.Time       `json:"created_at"`
}

type NLETimelineItem struct {
	TimelineItemID       string          `json:"timeline_item_id"`
	ParentTimelineItemID *string         `json:"parent_timeline_item_id,omitempty"`
	TimelineID           string          `json:"timeline_id"`
	TrackType            string          `json:"track_type"`
	TrackNumber          int             `json:"track_number"`
	SequenceNumber       int             `json:"sequence_number"`
	EntityType           string          `json:"entity_type"`
	EntityID             string          `json:"entity_id"`
	TimelineStartMS      int64           `json:"timeline_start_ms"`
	TimelineEndMS        int64           `json:"timeline_end_ms"`
	SourceInMS           int64           `json:"source_in_ms"`
	SourceOutMS          *int64          `json:"source_out_ms,omitempty"`
	DurationMS           int64           `json:"duration_ms"`
	Volume               float64         `json:"volume"`
	FadeInMS             int64           `json:"fade_in_ms"`
	FadeOutMS            int64           `json:"fade_out_ms"`
	TransformConfig      json.RawMessage `json:"transform_config"`
	EffectConfig         json.RawMessage `json:"effect_config"`
	ProxyURL             *string         `json:"proxy_url,omitempty"`
	WaveformURL          *string         `json:"waveform_url,omitempty"`
	SubtitleText         *string         `json:"subtitle_text,omitempty"`
	ProxyStatus          string          `json:"proxy_status"`
}

type NLERenderJob struct {
	RenderJobID  string     `json:"render_job_id"`
	TimelineID   string     `json:"timeline_id"`
	Status       string     `json:"status"`
	Progress     float64    `json:"progress"`
	OutputURL    *string    `json:"output_url,omitempty"`
	ErrorCode    *string    `json:"error_code,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type NLETimelinePage struct {
	Timeline          NLETimelineSummary `json:"timeline"`
	CurrentTimelineID string             `json:"current_timeline_id"`
	Items             []NLETimelineItem  `json:"items"`
	Total             int                `json:"total"`
	HasMore           bool               `json:"has_more"`
	WindowStartMS     int64              `json:"window_start_ms"`
	WindowEndMS       int64              `json:"window_end_ms"`
	RenderJob         *NLERenderJob      `json:"render_job,omitempty"`
}

type NLETimelineItemPatch struct {
	BaseTimelineID  string         `json:"base_timeline_id"`
	TimelineStartMS *int64         `json:"timeline_start_ms,omitempty"`
	TimelineEndMS   *int64         `json:"timeline_end_ms,omitempty"`
	SourceInMS      *int64         `json:"source_in_ms,omitempty"`
	SourceOutMS     *int64         `json:"source_out_ms,omitempty"`
	Volume          *float64       `json:"volume,omitempty"`
	FadeInMS        *int64         `json:"fade_in_ms,omitempty"`
	FadeOutMS       *int64         `json:"fade_out_ms,omitempty"`
	TransformConfig map[string]any `json:"transform_config,omitempty"`
	EffectConfig    map[string]any `json:"effect_config,omitempty"`
	Reason          string         `json:"reason,omitempty"`
	Actor           *string        `json:"actor,omitempty"`
}

type NLEDraftResult struct {
	Timeline NLETimelineSummary `json:"timeline"`
	Item     *NLETimelineItem   `json:"item,omitempty"`
}

func (s *Store) GetNLETimelinePage(
	ctx context.Context, projectID, episodeID, timelineID string,
	startMS, endMS int64, limit, offset int,
) (NLETimelinePage, error) {
	if startMS < 0 || endMS <= startMS || offset < 0 {
		return NLETimelinePage{}, fmt.Errorf("%w: invalid millisecond window", ErrConflict)
	}
	if limit <= 0 || limit > nlePageLimit {
		limit = nlePageLimit
	}
	if timelineID == "" {
		err := s.pool.QueryRow(ctx, `SELECT timeline_id FROM drama.edit_timelines
			WHERE project_id=$1 AND episode_id=$2
			ORDER BY version DESC LIMIT 1`, projectID, episodeID).Scan(&timelineID)
		if errors.Is(err, pgx.ErrNoRows) {
			return NLETimelinePage{}, ErrNotFound
		}
		if err != nil {
			return NLETimelinePage{}, err
		}
	}

	result := NLETimelinePage{Items: make([]NLETimelineItem, 0), WindowStartMS: startMS, WindowEndMS: endMS}
	if err := s.scanNLETimeline(ctx, s.pool, projectID, episodeID, timelineID, &result.Timeline); err != nil {
		return NLETimelinePage{}, err
	}
	_ = s.pool.QueryRow(ctx, `SELECT timeline_id FROM drama.edit_timelines
		WHERE project_id=$1 AND episode_id=$2 AND is_current`, projectID, episodeID).Scan(&result.CurrentTimelineID)

	rows, err := s.pool.Query(ctx, `SELECT item.timeline_item_id,item.parent_timeline_item_id,item.timeline_id,
		item.track_type,item.track_number,item.sequence_number,item.entity_type,item.entity_id,
		item.timeline_start_ms,item.timeline_end_ms,item.source_in_ms,item.source_out_ms,item.duration_ms,
		item.volume,item.fade_in_ms,item.fade_out_ms,item.transform_config,item.effect_config,
		CASE WHEN item.track_type='video' THEN COALESCE(item.proxy_url,(
			SELECT job.output_url FROM drama.shot_videos video
			JOIN drama.media_processing_jobs job ON job.entity_id=video.shot_video_id
			WHERE video.shot_id=item.entity_id AND job.operation='transcode_video'
			  AND job.status='succeeded' AND NULLIF(btrim(COALESCE(job.output_url,'')),'') IS NOT NULL
			ORDER BY job.updated_at DESC LIMIT 1))
			ELSE COALESCE(item.proxy_url,item.source_url) END proxy_url,
		COALESCE(item.waveform_url,(
			SELECT audio.waveform_url FROM drama.dialogue_audio audio
			WHERE (audio.dialogue_audio_id=item.entity_id OR audio.dialogue_id=item.entity_id)
			  AND audio.is_current AND audio.waveform_url IS NOT NULL
			ORDER BY audio.generation_version DESC LIMIT 1)) waveform_url,
		CASE WHEN item.track_type='subtitle' THEN COALESCE(item.transform_config->>'text',(
			SELECT dialogue.text FROM drama.dialogues dialogue WHERE dialogue.dialogue_id=item.entity_id),item.entity_id)
			ELSE NULL END subtitle_text
		FROM drama.edit_timeline_items item
		WHERE item.timeline_id=$1 AND item.timeline_start_ms<$3 AND item.timeline_end_ms>$2
		ORDER BY item.track_type,item.track_number,item.timeline_start_ms,item.sequence_number
		LIMIT $4 OFFSET $5`, timelineID, startMS, endMS, limit+1, offset)
	if err != nil {
		return NLETimelinePage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item NLETimelineItem
		if err = rows.Scan(&item.TimelineItemID, &item.ParentTimelineItemID, &item.TimelineID,
			&item.TrackType, &item.TrackNumber, &item.SequenceNumber, &item.EntityType, &item.EntityID,
			&item.TimelineStartMS, &item.TimelineEndMS, &item.SourceInMS, &item.SourceOutMS, &item.DurationMS,
			&item.Volume, &item.FadeInMS, &item.FadeOutMS, &item.TransformConfig, &item.EffectConfig,
			&item.ProxyURL, &item.WaveformURL, &item.SubtitleText); err != nil {
			return NLETimelinePage{}, err
		}
		item.ProxyStatus = "ready"
		if item.TrackType == "video" && (item.ProxyURL == nil || strings.TrimSpace(*item.ProxyURL) == "") {
			item.ProxyStatus = "pending"
		}
		result.Items = append(result.Items, item)
	}
	if err = rows.Err(); err != nil {
		return NLETimelinePage{}, err
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.HasMore = true
	}
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM drama.edit_timeline_items
		WHERE timeline_id=$1 AND timeline_start_ms<$3 AND timeline_end_ms>$2`,
		timelineID, startMS, endMS).Scan(&result.Total); err != nil {
		return NLETimelinePage{}, err
	}
	var render NLERenderJob
	err = s.pool.QueryRow(ctx, `SELECT render_job_id,timeline_id,status,progress,output_url,error_code,error_message,
		created_at,completed_at FROM drama.render_jobs WHERE timeline_id=$1 ORDER BY created_at DESC LIMIT 1`,
		timelineID).Scan(&render.RenderJobID, &render.TimelineID, &render.Status, &render.Progress, &render.OutputURL,
		&render.ErrorCode, &render.ErrorMessage, &render.CreatedAt, &render.CompletedAt)
	if err == nil {
		result.RenderJob = &render
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return NLETimelinePage{}, err
	}
	return result, nil
}

type nleTimelineScanner interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) scanNLETimeline(ctx context.Context, q nleTimelineScanner, projectID, episodeID, timelineID string, target *NLETimelineSummary) error {
	err := q.QueryRow(ctx, `SELECT timeline_id,parent_timeline_id,version,version_reason,approval_state,status,
		is_current,target_duration_ms,fps,resolution,subtitle_config,approved_render_job_id,approved_at,created_at
		FROM drama.edit_timelines WHERE project_id=$1 AND episode_id=$2 AND timeline_id=$3`,
		projectID, episodeID, timelineID).Scan(&target.TimelineID, &target.ParentTimelineID, &target.Version,
		&target.VersionReason, &target.ApprovalState, &target.Status, &target.IsCurrent, &target.TargetDurationMS,
		&target.FPS, &target.Resolution, &target.SubtitleConfig, &target.ApprovedRenderJobID, &target.ApprovedAt, &target.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Store) CreateNLEItemDraft(
	ctx context.Context, projectID, episodeID, timelineItemID string, patch NLETimelineItemPatch,
) (NLEDraftResult, error) {
	if strings.TrimSpace(patch.BaseTimelineID) == "" {
		return NLEDraftResult{}, fmt.Errorf("%w: base_timeline_id is required", ErrConflict)
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return NLEDraftResult{}, err
	}
	defer tx.Rollback(ctx)

	var state string
	var targetDurationMS int64
	if err = tx.QueryRow(ctx, `SELECT approval_state,target_duration_ms FROM drama.edit_timelines
		WHERE project_id=$1 AND episode_id=$2 AND timeline_id=$3 FOR UPDATE`,
		projectID, episodeID, patch.BaseTimelineID).Scan(&state, &targetDurationMS); errors.Is(err, pgx.ErrNoRows) {
		return NLEDraftResult{}, ErrNotFound
	}
	if err != nil {
		return NLEDraftResult{}, err
	}
	if state == "rendering" {
		return NLEDraftResult{}, fmt.Errorf("%w: rendering timeline is immutable", ErrConflict)
	}

	var old NLETimelineItem
	err = tx.QueryRow(ctx, `SELECT timeline_item_id,timeline_id,track_type,track_number,sequence_number,
		entity_type,entity_id,timeline_start_ms,timeline_end_ms,source_in_ms,source_out_ms,duration_ms,
		volume,fade_in_ms,fade_out_ms,transform_config,effect_config,proxy_url,waveform_url
		FROM drama.edit_timeline_items WHERE timeline_id=$1 AND timeline_item_id=$2 FOR UPDATE`,
		patch.BaseTimelineID, timelineItemID).Scan(&old.TimelineItemID, &old.TimelineID, &old.TrackType,
		&old.TrackNumber, &old.SequenceNumber, &old.EntityType, &old.EntityID, &old.TimelineStartMS,
		&old.TimelineEndMS, &old.SourceInMS, &old.SourceOutMS, &old.DurationMS, &old.Volume,
		&old.FadeInMS, &old.FadeOutMS, &old.TransformConfig, &old.EffectConfig, &old.ProxyURL, &old.WaveformURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return NLEDraftResult{}, ErrNotFound
	}
	if err != nil {
		return NLEDraftResult{}, err
	}

	start, end, sourceIn := old.TimelineStartMS, old.TimelineEndMS, old.SourceInMS
	sourceOut, volume, fadeIn, fadeOut := old.SourceOutMS, old.Volume, old.FadeInMS, old.FadeOutMS
	if patch.TimelineStartMS != nil {
		start = *patch.TimelineStartMS
	}
	if patch.TimelineEndMS != nil {
		end = *patch.TimelineEndMS
	}
	if patch.SourceInMS != nil {
		sourceIn = *patch.SourceInMS
	}
	if patch.SourceOutMS != nil {
		value := *patch.SourceOutMS
		sourceOut = &value
	}
	if patch.Volume != nil {
		volume = *patch.Volume
	}
	if patch.FadeInMS != nil {
		fadeIn = *patch.FadeInMS
	}
	if patch.FadeOutMS != nil {
		fadeOut = *patch.FadeOutMS
	}
	duration := end - start
	if start < 0 || duration <= 0 || sourceIn < 0 || volume < 0 || volume > 4 || fadeIn < 0 || fadeOut < 0 || fadeIn+fadeOut > duration {
		return NLEDraftResult{}, fmt.Errorf("%w: invalid millisecond trim, volume or fade values", ErrConflict)
	}
	if sourceOut != nil && *sourceOut <= sourceIn {
		return NLEDraftResult{}, fmt.Errorf("%w: source_out_ms must exceed source_in_ms", ErrConflict)
	}
	if end > targetDurationMS {
		return NLEDraftResult{}, fmt.Errorf("%w: timeline item exceeds target_duration_ms", ErrConflict)
	}
	transformJSON, effectJSON := json.RawMessage(nil), json.RawMessage(nil)
	if patch.TransformConfig != nil {
		transformJSON, _ = json.Marshal(patch.TransformConfig)
	}
	if patch.EffectConfig != nil {
		effectJSON, _ = json.Marshal(patch.EffectConfig)
	}
	reason := strings.TrimSpace(patch.Reason)
	if reason == "" {
		reason = "nle_item_edit"
	}
	draft, err := cloneTimelineVersion(ctx, tx, projectID, episodeID, patch.BaseTimelineID, "", "", reason, "draft", nil, nil)
	if err != nil {
		return NLEDraftResult{}, err
	}

	var newItemID string
	err = tx.QueryRow(ctx, `UPDATE drama.edit_timeline_items SET
		timeline_start_ms=$3,timeline_end_ms=$4,source_in_ms=$5,source_out_ms=$6,duration_ms=$4::bigint-$3::bigint,
		volume=$7,fade_in_ms=$8,fade_out_ms=$9,
		transform_config=transform_config||COALESCE($10::jsonb,'{}'::jsonb),
		effect_config=effect_config||COALESCE($11::jsonb,'{}'::jsonb),updated_at=CURRENT_TIMESTAMP
		WHERE timeline_id=$1 AND parent_timeline_item_id=$2 RETURNING timeline_item_id`,
		draft.TimelineID, timelineItemID, start, end, sourceIn, sourceOut, volume, fadeIn, fadeOut,
		nullableJSON(transformJSON), nullableJSON(effectJSON)).Scan(&newItemID)
	if err != nil {
		return NLEDraftResult{}, err
	}
	if err = validateNLETimeline(ctx, tx, draft.TimelineID, targetDurationMS); err != nil {
		return NLEDraftResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return NLEDraftResult{}, err
	}
	page, err := s.GetNLETimelinePage(ctx, projectID, episodeID, draft.TimelineID, 0, maxNLEInt64(end+1, 1), nlePageLimit, 0)
	if err != nil {
		return NLEDraftResult{}, err
	}
	result := NLEDraftResult{Timeline: page.Timeline}
	for index := range page.Items {
		if page.Items[index].TimelineItemID == newItemID {
			result.Item = &page.Items[index]
			break
		}
	}
	return result, nil
}

type nleValidationItem struct {
	id, trackType, entityID, sourcePath, sourceURL string
	trackNumber                                    int
	startMS, endMS, sourceInMS                     int64
	sourceOutMS                                    *int64
}

type nleQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func validateNLETimeline(ctx context.Context, queryer nleQueryer, timelineID string, targetDurationMS int64) error {
	rows, err := queryer.Query(ctx, `SELECT timeline_item_id,track_type,track_number,entity_id,
		timeline_start_ms,timeline_end_ms,source_in_ms,source_out_ms,COALESCE(source_path,''),COALESCE(source_url,'')
		FROM drama.edit_timeline_items WHERE timeline_id=$1`, timelineID)
	if err != nil {
		return err
	}
	items := []nleValidationItem{}
	for rows.Next() {
		var item nleValidationItem
		if err = rows.Scan(&item.id, &item.trackType, &item.trackNumber, &item.entityID,
			&item.startMS, &item.endMS, &item.sourceInMS, &item.sourceOutMS, &item.sourcePath, &item.sourceURL); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	byTrack := map[string][]nleValidationItem{}
	dialogueRanges := map[string][]nleValidationItem{}
	for _, item := range items {
		if item.startMS < 0 || item.endMS <= item.startMS || item.endMS > targetDurationMS {
			return fmt.Errorf("%w: timeline item %s is outside 0..target_duration_ms", ErrConflict, item.id)
		}
		key := fmt.Sprintf("%s:%d", item.trackType, item.trackNumber)
		byTrack[key] = append(byTrack[key], item)
		if item.trackType == "dialogue" || item.trackType == "narration" {
			dialogueRanges[item.entityID] = append(dialogueRanges[item.entityID], item)
		}
		if item.trackType == "subtitle" {
			continue
		}
		if strings.TrimSpace(item.sourcePath) == "" && strings.TrimSpace(item.sourceURL) == "" {
			return fmt.Errorf("%w: timeline item %s has an empty media reference", ErrConflict, item.id)
		}
		mediaDuration, known, stale, lookupErr := nleMediaDuration(ctx, queryer, item.trackType, item.entityID)
		if lookupErr != nil {
			return lookupErr
		}
		if stale {
			return fmt.Errorf("%w: timeline item %s references stale or unapproved media", ErrConflict, item.id)
		}
		if known {
			sourceEnd := item.sourceInMS + (item.endMS - item.startMS)
			if item.sourceOutMS != nil {
				sourceEnd = *item.sourceOutMS
			}
			if sourceEnd > mediaDuration {
				return fmt.Errorf("%w: timeline item %s source range exceeds media duration", ErrConflict, item.id)
			}
		}
	}
	for key, trackItems := range byTrack {
		sort.Slice(trackItems, func(i, j int) bool {
			if trackItems[i].startMS == trackItems[j].startMS {
				return trackItems[i].endMS < trackItems[j].endMS
			}
			return trackItems[i].startMS < trackItems[j].startMS
		})
		for index := 1; index < len(trackItems); index++ {
			if trackItems[index].startMS < trackItems[index-1].endMS {
				return fmt.Errorf("%w: illegal overlap on track %s between %s and %s", ErrConflict,
					key, trackItems[index-1].id, trackItems[index].id)
			}
		}
	}
	for _, item := range items {
		if item.trackType != "subtitle" {
			continue
		}
		dialogueID := item.entityID
		var resolvedDialogueID string
		err = queryer.QueryRow(ctx, `SELECT dialogue_id FROM drama.subtitle_cues
			WHERE subtitle_cue_id=$1`, item.entityID).Scan(&resolvedDialogueID)
		if err == nil {
			dialogueID = resolvedDialogueID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		contained := false
		for _, dialogue := range dialogueRanges[dialogueID] {
			if item.startMS >= dialogue.startMS && item.endMS <= dialogue.endMS {
				contained = true
				break
			}
		}
		if !contained {
			return fmt.Errorf("%w: subtitle item %s is outside its dialogue range", ErrConflict, item.id)
		}
	}
	return nil
}

func nleMediaDuration(ctx context.Context, queryer nleQueryer, trackType, entityID string) (duration int64, known, stale bool, err error) {
	var total int
	switch trackType {
	case "video":
		err = queryer.QueryRow(ctx, `SELECT count(*),COALESCE(max((actual_duration_seconds*1000)::bigint) FILTER(
			WHERE is_current AND status='succeeded' AND review_status='approved'),0)
			FROM drama.shot_videos WHERE shot_id=$1 OR shot_video_id=$1`, entityID).Scan(&total, &duration)
	case "dialogue", "narration":
		err = queryer.QueryRow(ctx, `SELECT count(*),COALESCE(max(actual_duration_ms) FILTER(
			WHERE is_current AND status='succeeded' AND review_status='approved'),0)
			FROM drama.dialogue_audio WHERE dialogue_id=$1 OR dialogue_audio_id=$1`, entityID).Scan(&total, &duration)
	case "bgm", "sound_effect", "ambience":
		err = queryer.QueryRow(ctx, `SELECT count(*),COALESCE(max(duration_ms) FILTER(
			WHERE is_current AND status='approved'),0)
			FROM drama.sound_asset_versions WHERE sound_asset_id=$1 OR sound_asset_version_id=$1`, entityID).Scan(&total, &duration)
	default:
		return 0, false, false, nil
	}
	if err != nil {
		return 0, false, false, err
	}
	if total == 0 {
		return 0, false, false, nil
	}
	if duration <= 0 {
		return 0, false, true, nil
	}
	return duration, true, false, nil
}

func maxNLEInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func (s *Store) RestoreNLETimelineDraft(ctx context.Context, projectID, episodeID, sourceTimelineID string, actor *string) (NLEDraftResult, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return NLEDraftResult{}, err
	}
	defer tx.Rollback(ctx)
	var ignored string
	err = tx.QueryRow(ctx, `SELECT timeline_id FROM drama.edit_timelines WHERE project_id=$1 AND episode_id=$2 AND timeline_id=$3 FOR UPDATE`, projectID, episodeID, sourceTimelineID).Scan(&ignored)
	if errors.Is(err, pgx.ErrNoRows) {
		return NLEDraftResult{}, ErrNotFound
	}
	if err != nil {
		return NLEDraftResult{}, err
	}
	draft, err := cloneTimelineVersion(ctx, tx, projectID, episodeID, sourceTimelineID, "", "", "timeline_restore", "restored", nil, nil)
	if err != nil {
		return NLEDraftResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return NLEDraftResult{}, err
	}
	var summary NLETimelineSummary
	if err = s.scanNLETimeline(ctx, s.pool, projectID, episodeID, draft.TimelineID, &summary); err != nil {
		return NLEDraftResult{}, err
	}
	return NLEDraftResult{Timeline: summary}, nil
}

func (s *Store) ConfirmNLETimelineRender(
	ctx context.Context, projectID, episodeID, timelineID string, storageDirectory ...string,
) (NLERenderJob, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return NLERenderJob{}, err
	}
	defer tx.Rollback(ctx)
	var version int
	var targetDurationMS int64
	var state string
	err = tx.QueryRow(ctx, `SELECT version,approval_state,target_duration_ms FROM drama.edit_timelines
		WHERE project_id=$1 AND episode_id=$2 AND timeline_id=$3 FOR UPDATE`, projectID, episodeID, timelineID).Scan(&version, &state, &targetDurationMS)
	if errors.Is(err, pgx.ErrNoRows) {
		return NLERenderJob{}, ErrNotFound
	}
	if err != nil {
		return NLERenderJob{}, err
	}
	if state != "draft" && state != "render_failed" {
		return NLERenderJob{}, fmt.Errorf("%w: only a draft timeline can be explicitly rendered", ErrConflict)
	}
	if err = ensureQualityGateAllowsRender(ctx, tx, projectID, episodeID); err != nil {
		return NLERenderJob{}, err
	}
	if err = validateNLETimeline(ctx, tx, timelineID, targetDurationMS); err != nil {
		return NLERenderJob{}, err
	}
	var existing NLERenderJob
	err = tx.QueryRow(ctx, `SELECT render_job_id,timeline_id,status,progress,output_url,error_code,error_message,created_at,completed_at
		FROM drama.render_jobs WHERE timeline_id=$1 AND status IN ('pending','claimed','processing') ORDER BY created_at DESC LIMIT 1`, timelineID).Scan(
		&existing.RenderJobID, &existing.TimelineID, &existing.Status, &existing.Progress, &existing.OutputURL, &existing.ErrorCode, &existing.ErrorMessage, &existing.CreatedAt, &existing.CompletedAt)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return NLERenderJob{}, err
	}
	renderID, err := newPublicID("rj_")
	if err != nil {
		return NLERenderJob{}, err
	}
	traceID, err := newPublicID("trace_")
	if err != nil {
		return NLERenderJob{}, err
	}
	manifestPath := fmt.Sprintf("/data/storage/results/nle/manifests/%s.json", renderID)
	outputPath := fmt.Sprintf("/data/storage/results/nle/renders/%s.mp4", renderID)
	artifacts := []string(nil)
	if len(storageDirectory) > 0 && strings.TrimSpace(storageDirectory[0]) != "" {
		artifacts, err = writeNLERenderArtifacts(ctx, tx, strings.TrimSpace(storageDirectory[0]), nleRenderArtifactInput{
			RenderJobID: renderID, ProjectID: projectID, EpisodeID: episodeID,
			TimelineID: timelineID, TimelineVersion: version, RenderType: "preview",
			ManifestPath: manifestPath, OutputPath: outputPath,
		})
		if err != nil {
			return NLERenderJob{}, err
		}
	}
	committed := false
	defer func() {
		if !committed {
			removeNLERenderArtifacts(artifacts)
		}
	}()
	_, err = tx.Exec(ctx, `INSERT INTO drama.render_jobs(render_job_id,idempotency_key,trace_id,project_id,episode_id,
		timeline_id,timeline_version,render_type,status,command_template_id,input_manifest_path,output_path,max_retries)
		VALUES($1,$2,$3,$4,$5,$6,$7,'preview','pending','nle-preview-v1',$8,$9,2)`, renderID, "nle-confirm:"+timelineID+":"+renderID, traceID, projectID, episodeID, timelineID, version, manifestPath, outputPath)
	if err != nil {
		return NLERenderJob{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE drama.edit_timelines SET approval_state='rendering',status='rendering',updated_at=CURRENT_TIMESTAMP WHERE timeline_id=$1`, timelineID)
	if err != nil {
		return NLERenderJob{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return NLERenderJob{}, err
	}
	committed = true
	return NLERenderJob{RenderJobID: renderID, TimelineID: timelineID, Status: "pending", Progress: 0, CreatedAt: time.Now()}, nil
}

func ensureQualityGateAllowsRender(ctx context.Context, tx pgx.Tx, projectID, episodeID string) error {
	var runID, modelStatus string
	err := tx.QueryRow(ctx, `SELECT gate_run_id,model_status FROM drama.quality_gate_runs
		WHERE project_id=$1 AND episode_id=$2 AND status<>'superseded'
		ORDER BY created_at DESC,gate_run_id DESC LIMIT 1`, projectID, episodeID).Scan(&runID, &modelStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var blockers int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM drama.quality_gate_findings
		WHERE gate_run_id=$1 AND severity='blocking' AND status='open'`, runID).Scan(&blockers); err != nil {
		return err
	}
	if blockers > 0 {
		return fmt.Errorf("%w: QUALITY_GATE_BLOCKED: %d blocking findings remain open in %s", ErrConflict, blockers, runID)
	}
	if modelStatus == "pending" {
		return fmt.Errorf("%w: QUALITY_GATE_BLOCKED: required model review is pending in %s", ErrConflict, runID)
	}
	return nil
}
