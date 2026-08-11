package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/effectiveinput"
	"short-drama-cms/backend/internal/qualitygate"
)

type qualityGateProductionSnapshot struct {
	SchemaVersion string          `json:"schema_version"`
	ContentHash   string          `json:"content_hash"`
	Payload       json.RawMessage `json:"payload"`
}

// RunAuthoritativeQualityGate deliberately accepts identities only. All reviewed
// artifact ids, versions, media bindings and timeline ranges are rebuilt from the
// Effective Input Resolver and the database in this request.
func (s *Store) RunAuthoritativeQualityGate(
	ctx context.Context, projectID, episodeID, masterID string, config qualitygate.Config,
	modelReviewRequired bool, actor string,
) (QualityGateRecord, error) {
	snapshot, err := s.BuildAuthoritativeQualityGateSnapshot(ctx, projectID, episodeID, masterID)
	if err != nil {
		return QualityGateRecord{}, err
	}
	run, err := qualitygate.EvaluateRules(snapshot, config, modelReviewRequired)
	if err != nil {
		return QualityGateRecord{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return s.SaveQualityGateRuleRun(ctx, snapshot, run, actor)
}

func (s *Store) BuildAuthoritativeQualityGateSnapshot(
	ctx context.Context, projectID, episodeID, masterID string,
) (qualitygate.Snapshot, error) {
	projectID, episodeID, masterID = strings.TrimSpace(projectID), strings.TrimSpace(episodeID), strings.TrimSpace(masterID)
	if projectID == "" || episodeID == "" || masterID == "" {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: project_id, episode_id and master_id are required", ErrValidation)
	}

	var resolution effectiveinput.Resolution
	raw, err := s.ResolveEffectiveInputs(ctx, projectID, episodeID, "post_production")
	if err != nil {
		return qualitygate.Snapshot{}, err
	}
	if err = json.Unmarshal(raw, &resolution); err != nil {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: decode effective input resolution: %v", ErrValidation, err)
	}
	if resolution.ResolverVersion == "" || resolution.ResolutionHash == "" {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: effective input resolution is incomplete", ErrConflict)
	}

	productionItem, ok := effectiveItemByKind(resolution.Items, "production_snapshot")
	if !ok || productionItem.State != "resolved" || len(productionItem.Content) == 0 {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: authoritative production snapshot is unavailable", ErrConflict)
	}
	var production qualityGateProductionSnapshot
	if err = json.Unmarshal(productionItem.Content, &production); err != nil || production.SchemaVersion != "production-input-snapshot.v1" {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: authoritative production snapshot is invalid", ErrConflict)
	}
	var payload map[string]json.RawMessage
	if err = json.Unmarshal(production.Payload, &payload); err != nil {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: production snapshot payload is invalid", ErrConflict)
	}

	var timelineID, masterStatus, approvalState string
	var timelineVersion, masterVersion int
	var durationMS int64
	var timelineCurrent, masterCurrent bool
	err = s.pool.QueryRow(ctx, `SELECT master.timeline_id,master.generation_version,master.duration_ms,master.status,
		master.is_current,timeline.version,timeline.approval_state,timeline.is_current
		FROM drama.episode_masters master JOIN drama.edit_timelines timeline ON timeline.timeline_id=master.timeline_id
		WHERE master.project_id=$1 AND master.episode_id=$2 AND master.master_id=$3`,
		projectID, episodeID, masterID).Scan(&timelineID, &masterVersion, &durationMS, &masterStatus,
		&masterCurrent, &timelineVersion, &approvalState, &timelineCurrent)
	if errors.Is(err, pgx.ErrNoRows) {
		return qualitygate.Snapshot{}, ErrNotFound
	}
	if err != nil {
		return qualitygate.Snapshot{}, err
	}
	if masterStatus != "ready" || !masterCurrent || !timelineCurrent || (approvalState != "approved" && approvalState != "restored") {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: master must reference the current approved timeline", ErrConflict)
	}
	resolvedTimelineID := rawStringField(payload["timeline"], "timeline_id")
	if resolvedTimelineID == "" || resolvedTimelineID != timelineID {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: master timeline does not match the Effective Input Resolver", ErrConflict)
	}
	if err = validateNLETimeline(ctx, s.pool, timelineID, durationMS); err != nil {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: authoritative timeline is invalid: %v", ErrConflict, err)
	}

	artifacts := make([]qualitygate.Artifact, 0, len(qualitygate.StageOrder))
	artifacts = append(artifacts,
		artifactFromEffectiveItem(qualitygate.StageSourceIR, resolution.Items, "narrative_ir"),
		artifactFromEffectiveItem(qualitygate.StageAdaptationPlan, resolution.Items, "adaptation_plan"),
		artifactFromPayload(qualitygate.StageEpisodeOutline, payload["outline"], "episode_id", episodeID),
		artifactFromPayload(qualitygate.StageScript, payload["script"], "script_id", ""),
		artifactFromPayload(qualitygate.StageStoryboard, payload["storyboard"], "storyboard_id", ""),
	)
	mediaID := "media:" + production.ContentHash
	if production.ContentHash == "" {
		mediaID = "media:" + resolution.ResolutionHash
	}
	artifacts = append(artifacts, qualitygate.Artifact{Stage: qualitygate.StageMedia, ArtifactID: mediaID, Version: masterVersion})

	timelineArtifact := qualitygate.Artifact{Stage: qualitygate.StageEditTimeline, ArtifactID: timelineID,
		Version: timelineVersion, DurationMS: durationMS, Timeline: []qualitygate.TimelineItem{}}
	rows, err := s.pool.Query(ctx, `SELECT timeline_item_id,track_type,entity_type,entity_id,timeline_start_ms,timeline_end_ms,
		COALESCE(source_path,''),COALESCE(source_url,'') FROM drama.edit_timeline_items
		WHERE timeline_id=$1 ORDER BY track_type,track_number,sequence_number`, timelineID)
	if err != nil {
		return qualitygate.Snapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item qualitygate.TimelineItem
		var sourcePath, sourceURL string
		if err = rows.Scan(&item.TimelineItemID, &item.TrackType, &item.EntityType, &item.EntityID,
			&item.StartMS, &item.EndMS, &sourcePath, &sourceURL); err != nil {
			return qualitygate.Snapshot{}, err
		}
		if item.TrackType != "subtitle" && strings.TrimSpace(sourcePath) == "" && strings.TrimSpace(sourceURL) == "" {
			return qualitygate.Snapshot{}, fmt.Errorf("%w: timeline item %s has no media reference", ErrConflict, item.TimelineItemID)
		}
		timelineArtifact.Timeline = append(timelineArtifact.Timeline, item)
	}
	if err = rows.Err(); err != nil {
		return qualitygate.Snapshot{}, err
	}
	artifacts = append(artifacts, timelineArtifact,
		qualitygate.Artifact{Stage: qualitygate.StageMaster, ArtifactID: masterID, Version: masterVersion, DurationMS: durationMS})

	for index := range artifacts {
		if artifacts[index].ArtifactID == "" || artifacts[index].Version < 1 {
			return qualitygate.Snapshot{}, fmt.Errorf("%w: Resolver did not provide authoritative %s identity/version", ErrConflict, artifacts[index].Stage)
		}
	}
	snapshot := qualitygate.Snapshot{SchemaVersion: qualitygate.SchemaVersion, ProjectID: projectID,
		EpisodeID: episodeID, MasterID: masterID, DurationMS: durationMS, Artifacts: artifacts}
	if err = snapshot.Validate(); err != nil {
		return qualitygate.Snapshot{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return snapshot, nil
}

func effectiveItemByKind(items []effectiveinput.Item, kind string) (effectiveinput.Item, bool) {
	for _, item := range items {
		if item.Kind == kind {
			return item, true
		}
	}
	return effectiveinput.Item{}, false
}

func artifactFromEffectiveItem(stage qualitygate.Stage, items []effectiveinput.Item, kind string) qualitygate.Artifact {
	item, _ := effectiveItemByKind(items, kind)
	id := ""
	if item.InputID != nil {
		id = strings.TrimSpace(*item.InputID)
	}
	if id == "" && len(item.InputIDs) > 0 {
		id = strings.TrimSpace(item.InputIDs[0])
	}
	return qualitygate.Artifact{Stage: stage, ArtifactID: id, Version: firstVersion(item.Versions)}
}

func artifactFromPayload(stage qualitygate.Stage, raw json.RawMessage, idKey, fallback string) qualitygate.Artifact {
	id := rawStringField(raw, idKey)
	if id == "" {
		id = fallback
	}
	return qualitygate.Artifact{Stage: stage, ArtifactID: id, Version: rawVersion(raw)}
}

func rawStringField(raw json.RawMessage, key string) string {
	var value map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	result, _ := value[key].(string)
	return strings.TrimSpace(result)
}

func rawVersion(raw json.RawMessage) int {
	var value map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return 1
	}
	for _, key := range []string{"version", "generation_version", "revision_number", "source_outline_version"} {
		if number, ok := value[key].(float64); ok && number >= 1 {
			return int(number)
		}
	}
	return 1
}

func firstVersion(raw json.RawMessage) int {
	var values []any
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil || len(values) == 0 {
		return 1
	}
	switch value := values[0].(type) {
	case float64:
		if value >= 1 {
			return int(value)
		}
	case map[string]any:
		for _, key := range []string{"version", "plan_version", "generation_version"} {
			if number, ok := value[key].(float64); ok && number >= 1 {
				return int(number)
			}
		}
	}
	return 1
}
