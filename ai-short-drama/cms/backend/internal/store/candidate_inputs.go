package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/candidategeneration"
)

type CandidateShotTarget struct {
	ShotID      string  `json:"shot_id"`
	ShotNumber  int     `json:"shot_number"`
	ShotOrder   int     `json:"shot_order"`
	Description string  `json:"description"`
	Duration    float64 `json:"duration_seconds"`
}

type CandidateSceneTarget struct {
	SceneID     string                `json:"scene_id"`
	SceneNumber int                   `json:"scene_number"`
	Label       string                `json:"label"`
	Shots       []CandidateShotTarget `json:"shots"`
}

type CandidateEpisodeTarget struct {
	EpisodeID     string                 `json:"episode_id"`
	EpisodeNumber int                    `json:"episode_number"`
	Title         string                 `json:"title"`
	Scenes        []CandidateSceneTarget `json:"scenes"`
}

type CandidateArcTarget struct {
	StoryArcRevisionID string `json:"story_arc_revision_id"`
	Title              string `json:"title"`
	Summary            string `json:"summary"`
}

type CandidateTargets struct {
	WorkID      string                   `json:"work_id"`
	WorkTitle   string                   `json:"work_title"`
	ProjectID   string                   `json:"project_id"`
	ProjectName string                   `json:"project_name"`
	Arcs        []CandidateArcTarget     `json:"arcs"`
	Episodes    []CandidateEpisodeTarget `json:"episodes"`
}

func (s *Store) ListCandidateTargets(ctx context.Context, projectID string) (CandidateTargets, error) {
	result := CandidateTargets{ProjectID: strings.TrimSpace(projectID), Arcs: []CandidateArcTarget{}, Episodes: []CandidateEpisodeTarget{}}
	err := s.pool.QueryRow(ctx, `SELECT project.novel_name,COALESCE(work.work_id,''),COALESCE(work.title,project.novel_name)
		FROM drama.projects project LEFT JOIN drama.project_source_bindings binding ON binding.project_id=project.project_id
		 AND binding.binding_role='primary' AND binding.is_current LEFT JOIN drama.source_works work ON work.work_id=binding.work_id
		WHERE project.project_id=$1`, result.ProjectID).Scan(&result.ProjectName, &result.WorkID, &result.WorkTitle)
	if errors.Is(err, pgx.ErrNoRows) {
		return CandidateTargets{}, ErrNotFound
	}
	if err != nil {
		return CandidateTargets{}, err
	}
	arcRows, err := s.pool.Query(ctx, `SELECT DISTINCT arc.story_arc_revision_id,arc.title,arc.summary
		FROM drama.story_arc_revisions arc
		JOIN drama.narrative_ir_revisions ir USING(ir_revision_id)
		JOIN drama.project_source_bindings binding
		  ON binding.work_id=ir.work_id AND binding.source_version_id=ir.source_version_id
		WHERE binding.project_id=$1 AND binding.binding_role='primary' AND binding.is_current
		  AND ir.status='published' AND ir.is_current
		ORDER BY arc.title,arc.story_arc_revision_id`, result.ProjectID)
	if err != nil {
		return CandidateTargets{}, err
	}
	for arcRows.Next() {
		var arc CandidateArcTarget
		if err := arcRows.Scan(&arc.StoryArcRevisionID, &arc.Title, &arc.Summary); err != nil {
			arcRows.Close()
			return CandidateTargets{}, err
		}
		result.Arcs = append(result.Arcs, arc)
	}
	if err := arcRows.Err(); err != nil {
		arcRows.Close()
		return CandidateTargets{}, err
	}
	arcRows.Close()

	episodeRows, err := s.pool.Query(ctx, `SELECT episode_id,episode_number,title
		FROM drama.episode_outlines WHERE project_id=$1 ORDER BY episode_number,episode_id`, result.ProjectID)
	if err != nil {
		return CandidateTargets{}, err
	}
	episodeIndex := map[string]int{}
	for episodeRows.Next() {
		var episode CandidateEpisodeTarget
		if err := episodeRows.Scan(&episode.EpisodeID, &episode.EpisodeNumber, &episode.Title); err != nil {
			episodeRows.Close()
			return CandidateTargets{}, err
		}
		episode.Scenes = []CandidateSceneTarget{}
		episodeIndex[episode.EpisodeID] = len(result.Episodes)
		result.Episodes = append(result.Episodes, episode)
	}
	if err := episodeRows.Err(); err != nil {
		episodeRows.Close()
		return CandidateTargets{}, err
	}
	episodeRows.Close()

	sceneRows, err := s.pool.Query(ctx, `WITH latest_scripts AS (
		SELECT DISTINCT ON(episode_id) script_id,episode_id FROM drama.episode_scripts
		WHERE project_id=$1 ORDER BY episode_id,version DESC,created_at DESC
	) SELECT scene.scene_id,scene.episode_id,scene.scene_number,
		concat_ws(' · ',NULLIF(scene.location_name,''),NULLIF(scene.time_of_day,''),NULLIF(scene.scene_purpose,''))
		FROM drama.script_scenes scene JOIN latest_scripts script USING(script_id,episode_id)
		ORDER BY scene.episode_id,scene.scene_number`, result.ProjectID)
	if err != nil {
		return CandidateTargets{}, err
	}
	sceneIndex := map[string][2]int{}
	for sceneRows.Next() {
		var scene CandidateSceneTarget
		var episodeID string
		if err := sceneRows.Scan(&scene.SceneID, &episodeID, &scene.SceneNumber, &scene.Label); err != nil {
			sceneRows.Close()
			return CandidateTargets{}, err
		}
		episodePosition, ok := episodeIndex[episodeID]
		if !ok {
			continue
		}
		scene.Shots = []CandidateShotTarget{}
		scenePosition := len(result.Episodes[episodePosition].Scenes)
		result.Episodes[episodePosition].Scenes = append(result.Episodes[episodePosition].Scenes, scene)
		sceneIndex[scene.SceneID] = [2]int{episodePosition, scenePosition}
	}
	if err := sceneRows.Err(); err != nil {
		sceneRows.Close()
		return CandidateTargets{}, err
	}
	sceneRows.Close()

	shotRows, err := s.pool.Query(ctx, `WITH latest_boards AS (
		SELECT DISTINCT ON(episode_id) storyboard_id,episode_id FROM drama.storyboards
		WHERE project_id=$1 ORDER BY episode_id,version DESC,created_at DESC
	) SELECT shot.shot_id,shot.scene_id,shot.shot_number,shot.shot_order,
		shot.action_description,shot.duration_seconds::float8
		FROM drama.storyboard_shots shot JOIN latest_boards board USING(storyboard_id,episode_id)
		ORDER BY shot.episode_id,shot.shot_order`, result.ProjectID)
	if err != nil {
		return CandidateTargets{}, err
	}
	defer shotRows.Close()
	for shotRows.Next() {
		var shot CandidateShotTarget
		var sceneID string
		if err := shotRows.Scan(&shot.ShotID, &sceneID, &shot.ShotNumber, &shot.ShotOrder, &shot.Description, &shot.Duration); err != nil {
			return CandidateTargets{}, err
		}
		position, ok := sceneIndex[sceneID]
		if ok {
			result.Episodes[position[0]].Scenes[position[1]].Shots = append(result.Episodes[position[0]].Scenes[position[1]].Shots, shot)
		}
	}
	return result, shotRows.Err()
}

func (s *Store) freezeCandidateInputs(ctx context.Context, projectID string, request candidategeneration.Request) (candidategeneration.FrozenInput, error) {
	episodeID, err := s.candidateTargetEpisode(ctx, projectID, request.TargetType, request.TargetID)
	if err != nil {
		return candidategeneration.FrozenInput{}, err
	}
	stage := candidateResolutionStage(request.TargetType)
	resolution, err := s.ResolveEffectiveInputs(ctx, projectID, episodeID, stage)
	if err != nil {
		return candidategeneration.FrozenInput{}, err
	}
	var envelope struct {
		ResolutionID   string          `json:"resolution_id"`
		ContextHash    string          `json:"context_hash"`
		ResolutionHash string          `json:"resolution_hash"`
		Status         string          `json:"status"`
		Ready          bool            `json:"ready"`
		Blockers       json.RawMessage `json:"blockers"`
		Context        struct {
			ProductionSnapshot json.RawMessage `json:"production_snapshot"`
		} `json:"context"`
	}
	if err := json.Unmarshal(resolution, &envelope); err != nil || envelope.ResolutionID == "" || envelope.ContextHash == "" {
		return candidategeneration.FrozenInput{}, fmt.Errorf("invalid effective input resolution")
	}
	// A stale or unconfirmed candidate selection must block every production
	// consumer, but it cannot lock the candidate regeneration entry point that
	// repairs that exact state. No other Resolver blocker is bypassed here.
	candidateRemediation := false
	if !envelope.Ready || envelope.Status != "ready" {
		var blockers []struct {
			Kind  string `json:"kind"`
			State string `json:"state"`
		}
		if json.Unmarshal(envelope.Blockers, &blockers) == nil && len(blockers) > 0 {
			candidateRemediation = true
			for _, blocker := range blockers {
				if blocker.Kind != "candidate_selection" || (blocker.State != "stale" && blocker.State != "needs_review") {
					candidateRemediation = false
					break
				}
			}
		}
	}
	if ((!envelope.Ready || envelope.Status != "ready") && !candidateRemediation) || len(envelope.Context.ProductionSnapshot) == 0 {
		return candidategeneration.FrozenInput{}, fmt.Errorf("%w: effective input resolver status=%s ready=%t blockers=%s", ErrConflict, envelope.Status, envelope.Ready, envelope.Blockers)
	}
	var snapshotEnvelope struct {
		State     string          `json:"state"`
		VersionID string          `json:"version_id"`
		BindingID string          `json:"binding_id"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(envelope.Context.ProductionSnapshot, &snapshotEnvelope); err != nil ||
		snapshotEnvelope.State != "resolved" || snapshotEnvelope.VersionID == "" ||
		snapshotEnvelope.BindingID == "" || len(snapshotEnvelope.Payload) == 0 {
		return candidategeneration.FrozenInput{}, fmt.Errorf("%w: production snapshot is not a resolved immutable binding", ErrConflict)
	}
	targetContext, err := json.Marshal(map[string]any{
		"source_kind":         "effective_input_snapshot",
		"target_type":         request.TargetType,
		"target_id":           request.TargetID,
		"resolution_id":       envelope.ResolutionID,
		"production_snapshot": json.RawMessage(envelope.Context.ProductionSnapshot),
	})
	if err != nil {
		return candidategeneration.FrozenInput{}, err
	}
	frozen := candidategeneration.FrozenInput{SchemaVersion: "candidate-frozen-input.v1", ResolutionID: envelope.ResolutionID,
		ContextHash: envelope.ContextHash, ResolutionHash: envelope.ResolutionHash, Stage: stage, EpisodeID: episodeID,
		Resolution: resolution, TargetContext: targetContext}
	hash, err := hashJSON(map[string]any{"resolution": json.RawMessage(resolution), "target_context": json.RawMessage(targetContext),
		"stage": stage, "episode_id": episodeID})
	if err != nil {
		return candidategeneration.FrozenInput{}, err
	}
	frozen.FrozenHash = hash
	return frozen, nil
}

func candidateResolutionStage(targetType string) string {
	switch targetType {
	case "story_arc", "episode":
		return "episode_script"
	case "scene":
		return "storyboard_design"
	case "storyboard", "image":
		return "storyboard_images"
	case "video":
		return "image_to_video"
	default:
		return "episode_script"
	}
}

// candidateTargetEpisode only resolves target identity. All production content is
// subsequently obtained from the Effective Input Resolver's immutable snapshot.
func (s *Store) candidateTargetEpisode(ctx context.Context, projectID, targetType, targetID string) (string, error) {
	var episodeID string
	var err error
	switch targetType {
	case "story_arc":
		err = s.pool.QueryRow(ctx, `SELECT ''
			FROM drama.story_arc_revisions arc
			JOIN drama.narrative_ir_revisions ir USING(ir_revision_id)
			JOIN drama.project_source_bindings binding
			  ON binding.work_id=ir.work_id AND binding.source_version_id=ir.source_version_id
			WHERE arc.story_arc_revision_id=$1 AND binding.project_id=$2 AND binding.is_current`, targetID, projectID).Scan(&episodeID)
	case "episode":
		err = s.pool.QueryRow(ctx, `SELECT outline.episode_id
			FROM drama.episode_outlines outline WHERE outline.episode_id=$1 AND outline.project_id=$2`, targetID, projectID).Scan(&episodeID)
	case "scene":
		err = s.pool.QueryRow(ctx, `SELECT scene.episode_id
			FROM drama.script_scenes scene WHERE scene.scene_id=$1 AND scene.project_id=$2`, targetID, projectID).Scan(&episodeID)
	case "storyboard":
		// The UI selects a shot for storyboard refinement. A storyboard id remains accepted for API compatibility.
		err = s.pool.QueryRow(ctx, `SELECT board.episode_id
			FROM drama.storyboards board LEFT JOIN drama.storyboard_shots shot
			  ON shot.storyboard_id=board.storyboard_id AND shot.shot_id=$1
			WHERE board.project_id=$2 AND (board.storyboard_id=$1 OR shot.shot_id=$1)
			ORDER BY board.version DESC LIMIT 1`, targetID, projectID).Scan(&episodeID)
	case "image", "video":
		err = s.pool.QueryRow(ctx, `SELECT shot.episode_id
			FROM drama.storyboard_shots shot WHERE shot.shot_id=$1 AND shot.project_id=$2`, targetID, projectID).Scan(&episodeID)
	default:
		return "", ErrConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return episodeID, nil
}

func frozenArtifactIDs(resolution json.RawMessage) []string {
	var envelope struct {
		Items []struct {
			ArtifactIDs []string `json:"artifact_ids"`
		} `json:"items"`
	}
	if json.Unmarshal(resolution, &envelope) != nil {
		return nil
	}
	items := []string{}
	for _, input := range envelope.Items {
		items = append(items, input.ArtifactIDs...)
	}
	return uniqueStrings(items)
}
