package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestEpisodeContentStructuralPlanIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE15_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE15_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suffix, _ := newPublicID("")
	id := func(prefix string) string { return prefix + suffix }
	projectID, storyBibleID, seasonID := id("p6_project_"), id("p6_bible_"), id("p6_season_")
	episodeID, scriptID, sceneID := id("p6_episode_"), id("p6_script_"), id("p6_scene_")
	arcID, runID, storyboardID := id("p6_arc_"), id("p6_run_"), id("p6_board_")
	defer func() {
		_, _ = database.writer.Exec(context.Background(), `DELETE FROM drama.projects WHERE project_id=$1`, projectID)
	}()
	setup := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO drama.projects(project_id,novel_name,target_episode_count,episode_duration_seconds,visual_style,aspect_ratio,target_platform,current_stage,status,test_mode) VALUES($1,'episode-editor',1,90,'test','9:16','test','episode_script_approved','waiting_review',true)`, []any{projectID}},
		{`INSERT INTO drama.story_bibles(story_bible_id,project_id,version,status) VALUES($1,$2,1,'approved')`, []any{storyBibleID, projectID}},
		{`INSERT INTO drama.seasons(season_id,project_id,story_bible_id,title,target_episode_count,target_episode_duration_seconds,status,version) VALUES($1,$2,$3,'season',1,90,'approved',1)`, []any{seasonID, projectID, storyBibleID}},
		{`INSERT INTO drama.episode_outlines(episode_id,season_id,project_id,episode_number,title,estimated_duration_seconds,status,version) VALUES($1,$2,$3,1,'episode',90,'approved',1)`, []any{episodeID, seasonID, projectID}},
		{`INSERT INTO drama.episode_scripts(script_id,project_id,season_id,episode_id,version,title,estimated_duration_seconds,source_outline_version,status) VALUES($1,$2,$3,$4,1,'script',90,1,'approved')`, []any{scriptID, projectID, seasonID, episodeID}},
		{`INSERT INTO drama.script_scenes(scene_id,script_id,project_id,episode_id,scene_number,scene_purpose,actions,estimated_duration_seconds) VALUES($1,$2,$3,$4,1,'opening','[]',45)`, []any{sceneID, scriptID, projectID, episodeID}},
		{`INSERT INTO drama.story_arc_runs(arc_run_id,project_id,title,planned_episode_count,status) VALUES($1,$2,'arc',1,'active')`, []any{arcID, projectID}},
		{`INSERT INTO drama.episode_production_runs(episode_run_id,arc_run_id,project_id,episode_id,episode_number,title,current_stage,status) VALUES($1,$2,$3,$4,1,'episode','episode_script_approved','waiting_review')`, []any{runID, arcID, projectID, episodeID}},
		{`INSERT INTO drama.storyboards(storyboard_id,project_id,episode_id,script_id,version,status,total_shots,estimated_duration_seconds) VALUES($1,$2,$3,$4,1,'approved',0,0)`, []any{storyboardID, projectID, episodeID, scriptID}},
	}
	for _, statement := range setup {
		if _, err = database.writer.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	current, err := database.GetEpisodeContent(ctx, projectID, runID)
	if err != nil {
		t.Fatal(err)
	}
	var outline EpisodeOutlineUpdate
	raw, _ := json.Marshal(current.Outline)
	_ = json.Unmarshal(raw, &outline)
	var script EpisodeScriptUpdate
	raw, _ = json.Marshal(current.Script)
	_ = json.Unmarshal(raw, &script)
	newSceneID := id("p6_new_scene_")
	newScene := EpisodeSceneUpdate{SceneID: newSceneID, SceneNumber: 2, LocationName: "new room",
		TimeOfDay: "night", InteriorExterior: "interior", CharacterIDs: json.RawMessage(`[]`),
		ScenePurpose: "turn", Actions: json.RawMessage(`[{"action_id":"action-new","description":"door opens"}]`),
		EstimatedDurationSeconds: 45, SourceEventIDs: json.RawMessage(`[]`), Dialogues: []EpisodeDialogueUpdate{}}
	script.Scenes = append(script.Scenes, newScene)
	invalid := script
	invalid.Scenes = append([]EpisodeSceneUpdate(nil), script.Scenes...)
	invalid.Scenes[len(invalid.Scenes)-1].CharacterIDs = json.RawMessage(`["character-does-not-exist"]`)
	_, err = database.CreateEpisodeContentChangePlan(ctx, projectID, runID,
		EpisodeContentChangePlanInput{ExpectedVersion: current.Revision, Outline: outline, Script: &invalid}, nil)
	if !errors.Is(err, ErrInvalidEpisodeContent) {
		t.Fatalf("unknown character reference was accepted: %v", err)
	}
	rejectedOutline := outline
	rejectedOutline.Title += " rejected candidate"
	rejected, err := database.CreateEpisodeContentChangePlan(ctx, projectID, runID,
		EpisodeContentChangePlanInput{ExpectedVersion: current.Revision, Outline: rejectedOutline, Script: &script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reviewMetadata := json.RawMessage(`{"original_blocks":[{"block_id":"outline-title","text":"original"}],"new_blocks":[{"block_id":"outline-title","text":"candidate"}],"reason":"candidate review","source_evidence":[{"source_span_id":"span-1","explanation":"fixture"}],"estimated_duration_delta_ms":0}`)
	rejected, err = database.SetChangePlanReviewMetadata(ctx, projectID, rejected.ChangePlanID, reviewMetadata)
	if err != nil || len(rejected.ReviewMetadata) == 0 {
		t.Fatalf("AI review metadata was not persisted: %#v err=%v", rejected, err)
	}
	rejected, err = database.RejectChangePlan(ctx, projectID, rejected.ChangePlanID, nil, "not accepted")
	if err != nil || rejected.Status != "cancelled" {
		t.Fatalf("AI candidate was not rejected: %#v err=%v", rejected, err)
	}
	rejected, err = database.RejectChangePlan(ctx, projectID, rejected.ChangePlanID, nil, "idempotent replay")
	if err != nil || rejected.Status != "cancelled" {
		t.Fatalf("AI candidate rejection was not idempotent: %#v err=%v", rejected, err)
	}
	afterReject, err := database.GetEpisodeContent(ctx, projectID, runID)
	if err != nil || afterReject.Revision != current.Revision || afterReject.Outline.Title != current.Outline.Title {
		t.Fatalf("rejected AI candidate mutated current content: %#v err=%v", afterReject, err)
	}
	plan, err := database.CreateEpisodeContentChangePlan(ctx, projectID, runID,
		EpisodeContentChangePlanInput{ExpectedVersion: current.Revision, Outline: outline, Script: &script}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = database.ConfirmChangePlan(ctx, projectID, plan.ChangePlanID, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = database.ExecuteChangePlan(ctx, projectID, plan.ChangePlanID)
	if err != nil || plan.Status != "applied" {
		t.Fatalf("add-scene change plan did not execute: %#v err=%v", plan, err)
	}
	refreshed, err := database.GetEpisodeContent(ctx, projectID, runID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Revision != current.Revision+1 || refreshed.Script == nil || len(refreshed.Script.Scenes) != 2 ||
		refreshed.Script.Scenes[1].SceneID != newSceneID {
		t.Fatalf("new scene is not present in current immutable episode content: %#v", refreshed)
	}
}
