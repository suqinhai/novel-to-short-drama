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
	ProjectID string                   `json:"project_id"`
	Arcs      []CandidateArcTarget     `json:"arcs"`
	Episodes  []CandidateEpisodeTarget `json:"episodes"`
}

func (s *Store) ListCandidateTargets(ctx context.Context, projectID string) (CandidateTargets, error) {
	result := CandidateTargets{ProjectID: strings.TrimSpace(projectID), Arcs: []CandidateArcTarget{}, Episodes: []CandidateEpisodeTarget{}}
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM drama.projects WHERE project_id=$1)`, result.ProjectID).Scan(&exists); err != nil {
		return CandidateTargets{}, err
	}
	if !exists {
		return CandidateTargets{}, ErrNotFound
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
	episodeID, targetContext, err := s.candidateTargetContext(ctx, projectID, request.TargetType, request.TargetID)
	if err != nil {
		return candidategeneration.FrozenInput{}, err
	}
	stage := candidateResolutionStage(request.TargetType)
	resolution, err := s.ResolveEffectiveInputs(ctx, projectID, episodeID, stage)
	if err != nil {
		return candidategeneration.FrozenInput{}, err
	}
	var envelope struct {
		ResolutionID   string `json:"resolution_id"`
		ContextHash    string `json:"context_hash"`
		ResolutionHash string `json:"resolution_hash"`
	}
	if err := json.Unmarshal(resolution, &envelope); err != nil || envelope.ResolutionID == "" || envelope.ContextHash == "" {
		return candidategeneration.FrozenInput{}, fmt.Errorf("invalid effective input resolution")
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

func (s *Store) candidateTargetContext(ctx context.Context, projectID, targetType, targetID string) (string, json.RawMessage, error) {
	var episodeID string
	var raw json.RawMessage
	var err error
	switch targetType {
	case "story_arc":
		err = s.pool.QueryRow(ctx, `SELECT '',jsonb_build_object(
			'source_kind','narrative_ir','story_arc_revision_id',arc.story_arc_revision_id,
			'ir_revision_id',arc.ir_revision_id,'chapter_id',arc.chapter_id,'title',arc.title,
			'summary',arc.summary,'arc_type',arc.arc_type,'source_span_id',arc.primary_source_span_id)
			FROM drama.story_arc_revisions arc
			JOIN drama.narrative_ir_revisions ir USING(ir_revision_id)
			JOIN drama.project_source_bindings binding
			  ON binding.work_id=ir.work_id AND binding.source_version_id=ir.source_version_id
			WHERE arc.story_arc_revision_id=$1 AND binding.project_id=$2 AND binding.is_current`, targetID, projectID).Scan(&episodeID, &raw)
	case "episode":
		err = s.pool.QueryRow(ctx, `SELECT outline.episode_id,jsonb_build_object(
			'source_kind','script','episode',to_jsonb(outline),
			'script',COALESCE((SELECT to_jsonb(script) FROM drama.episode_scripts script
			  WHERE script.project_id=outline.project_id AND script.episode_id=outline.episode_id ORDER BY version DESC LIMIT 1),'null'::jsonb))
			FROM drama.episode_outlines outline WHERE outline.episode_id=$1 AND outline.project_id=$2`, targetID, projectID).Scan(&episodeID, &raw)
	case "scene":
		err = s.pool.QueryRow(ctx, `SELECT scene.episode_id,jsonb_build_object(
			'source_kind','script','script_id',scene.script_id,'scene',to_jsonb(scene),
			'dialogues',COALESCE((SELECT jsonb_agg(to_jsonb(dialogue) ORDER BY sequence_number)
			  FROM drama.dialogues dialogue WHERE dialogue.scene_id=scene.scene_id),'[]'::jsonb))
			FROM drama.script_scenes scene WHERE scene.scene_id=$1 AND scene.project_id=$2`, targetID, projectID).Scan(&episodeID, &raw)
	case "storyboard":
		// The UI selects a shot for storyboard refinement. A storyboard id remains accepted for API compatibility.
		err = s.pool.QueryRow(ctx, `SELECT board.episode_id,jsonb_build_object(
			'source_kind','storyboard','storyboard_id',board.storyboard_id,
			'target',CASE WHEN shot.shot_id IS NULL THEN to_jsonb(board) ELSE to_jsonb(shot) END)
			FROM drama.storyboards board LEFT JOIN drama.storyboard_shots shot
			  ON shot.storyboard_id=board.storyboard_id AND shot.shot_id=$1
			WHERE board.project_id=$2 AND (board.storyboard_id=$1 OR shot.shot_id=$1)
			ORDER BY board.version DESC LIMIT 1`, targetID, projectID).Scan(&episodeID, &raw)
	case "image", "video":
		err = s.pool.QueryRow(ctx, `SELECT shot.episode_id,jsonb_build_object(
			'source_kind','storyboard','storyboard_id',shot.storyboard_id,'shot',to_jsonb(shot),
			'current_images',COALESCE((SELECT jsonb_agg(to_jsonb(image) ORDER BY image.generation_version DESC)
			  FROM drama.storyboard_images image WHERE image.shot_id=shot.shot_id AND image.is_current),'[]'::jsonb))
			FROM drama.storyboard_shots shot WHERE shot.shot_id=$1 AND shot.project_id=$2`, targetID, projectID).Scan(&episodeID, &raw)
	default:
		return "", nil, ErrConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	if err != nil {
		return "", nil, err
	}
	return episodeID, raw, nil
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
