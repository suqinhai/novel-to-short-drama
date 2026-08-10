package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	pc "short-drama-cms/backend/internal/performancecontinuity"
)

func TestPerformanceContinuityPhase4Integration(t *testing.T) {
	databaseURL := os.Getenv("PHASE4_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE4_DATABASE_URL is not set")
	}
	t.Setenv("MOCK_MODE", "true")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suffix, _ := newPublicID("")
	id := func(prefix string) string { return prefix + suffix }
	projectID, bibleID, seasonID := id("p4_p_"), id("p4_story_"), id("p4_season_")
	episode1, episode2 := id("p4_ep1_"), id("p4_ep2_")
	scriptID, sceneID, boardID := id("p4_script_"), id("p4_scene_"), id("p4_board_")
	shot1, shot2 := id("p4_shot1_"), id("p4_shot2_")

	defer func() {
		_, _ = database.writer.Exec(context.Background(), `DELETE FROM drama.projects WHERE project_id=$1`, projectID)
	}()
	setup := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO drama.projects(project_id,novel_name,target_episode_count,
			episode_duration_seconds,visual_style,aspect_ratio,target_platform,current_stage,status,test_mode)
			VALUES($1,'phase4-e2e',2,90,'mock','9:16','test','storyboard_approved','running',true)`, []any{projectID}},
		{`INSERT INTO drama.story_bibles(story_bible_id,project_id,version,status)
			VALUES($1,$2,1,'approved')`, []any{bibleID, projectID}},
		{`INSERT INTO drama.seasons(season_id,project_id,story_bible_id,title,target_episode_count,
			target_episode_duration_seconds,status,version)
			VALUES($1,$2,$3,'season',2,90,'approved',1)`, []any{seasonID, projectID, bibleID}},
		{`INSERT INTO drama.episode_outlines(episode_id,season_id,project_id,episode_number,title,
			estimated_duration_seconds,status,version)
			VALUES($1,$3,$4,1,'ep1',90,'approved',1),($2,$3,$4,2,'ep2',90,'approved',1)`,
			[]any{episode1, episode2, seasonID, projectID}},
		{`INSERT INTO drama.episode_scripts(script_id,project_id,season_id,episode_id,version,title,
			estimated_duration_seconds,source_outline_version,status)
			VALUES($1,$2,$3,$4,1,'script',90,1,'approved')`, []any{scriptID, projectID, seasonID, episode1}},
		{`INSERT INTO drama.script_scenes(scene_id,script_id,project_id,episode_id,scene_number,
			character_ids,scene_purpose,actions,estimated_duration_seconds)
			VALUES($1,$2,$3,$4,1,'["char_lin"]','test','[]',20)`, []any{sceneID, scriptID, projectID, episode1}},
		{`INSERT INTO drama.storyboards(storyboard_id,project_id,episode_id,script_id,version,status)
			VALUES($1,$2,$3,$4,1,'approved')`, []any{boardID, projectID, episode1, scriptID}},
		{`INSERT INTO drama.storyboard_shots(shot_id,storyboard_id,project_id,episode_id,scene_id,
			shot_number,shot_order,duration_seconds,shot_size,camera_angle,camera_motion,composition,
			character_ids,action_description,status,generation_version)
			VALUES($1,$3,$4,$5,$6,1,1,2,'medium','eye','static','left','["char_lin"]','start slap','approved',1),
			      ($2,$3,$4,$5,$6,2,2,2,'close','eye','static','right','["char_lin"]','complete slap','approved',1)`,
			[]any{shot1, shot2, boardID, projectID, episode1, sceneID}},
	}
	for _, statement := range setup {
		if _, err = database.writer.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("locked bible rejects in-place generated mutation", func(t *testing.T) {
		record, createErr := database.CreatePerformanceBibleVersion(ctx, projectID, CreatePerformanceBibleInput{
			CharacterID: "char_lin", CharacterVersion: "adult",
			Speech:           json.RawMessage(`{"rate_wpm":150,"pitch":"mid","voice_identity":"voice_lin","pause_habits":[],"catchphrases":[]}`),
			Acting:           json.RawMessage(`{"emotion_style":{},"body_habits":[],"prohibited_acts":[]}`),
			RelationalVoices: json.RawMessage(`{}`),
			Appearance:       json.RawMessage(`{"face_shape":"oval","hairstyle":"low ponytail","apparent_age":"28","body_type":"slim","posture":"upright"}`),
			LockedFields:     json.RawMessage(`["appearance.face_shape"]`),
			AllowedFields:    json.RawMessage(`["appearance.hairstyle"]`),
			ChangeReasons:    json.RawMessage(`{}`), SourceRefs: json.RawMessage(`{"story_bible":"v1"}`),
			StageStates: []pc.StageState{{
				StageKey: "injured", EpisodeFrom: 2, Costume: "blue_coat_torn",
				Scars: []string{"right_brow"}, Props: []string{"letter"},
				Psychology: "guarded", Relationships: map[string]string{"char_zhou": "distrust"},
			}},
			ChangeReason: "initial fixture",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, createErr = database.LockPerformanceBible(ctx, record.PerformanceBibleID); createErr != nil {
			t.Fatal(createErr)
		}
		items, listErr := database.ListPerformanceBibles(ctx, projectID)
		if listErr != nil || len(items) != 1 || string(items[0].StageStates) == "[]" {
			t.Fatalf("performance stage state was not persisted: %+v err=%v", items, listErr)
		}
		if _, createErr = database.writer.Exec(ctx, `UPDATE drama.character_performance_bibles
			SET appearance=appearance||'{"face_shape":"round"}' WHERE performance_bible_id=$1`,
			record.PerformanceBibleID); createErr == nil {
			t.Fatal("locked performance bible accepted an in-place mutation")
		}
	})

	t.Run("cross-episode state inherits exact output", func(t *testing.T) {
		state := `{"characters":{"char_lin":{"costume":"blue_coat"}},"props":{},"environment":{"location_id":"room"},"axis":"A"}`
		entryID := id("p4_ledger_")
		hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		if _, err = database.writer.Exec(ctx, `INSERT INTO drama.continuity_ledger_entries(
			continuity_entry_id,project_id,episode_id,episode_number,scope,sequence_number,input_state,
			output_state,validation_status,state_hash)VALUES($1,$2,$3,1,'episode',0,$4,$4,'valid',$5)`,
			entryID, projectID, episode1, state, hash); err != nil {
			t.Fatal(err)
		}
		var inheritedID string
		if err = database.writer.QueryRow(ctx, `SELECT drama.inherit_episode_continuity($1,$2,2)`,
			entryID, episode2).Scan(&inheritedID); err != nil {
			t.Fatal(err)
		}
		var inheritedFrom string
		var input json.RawMessage
		if err = database.pool.QueryRow(ctx, `SELECT inherited_from_entry_id,input_state
			FROM drama.continuity_ledger_entries WHERE continuity_entry_id=$1`, inheritedID).
			Scan(&inheritedFrom, &input); err != nil || inheritedFrom != entryID ||
			!json.Valid(input) {
			t.Fatalf("cross-episode inheritance failed: from=%s input=%s err=%v", inheritedFrom, input, err)
		}
	})

	t.Run("shot change dirties only adjacent handoff", func(t *testing.T) {
		handoffID := id("p4_handoff_")
		if _, err = database.writer.Exec(ctx, `INSERT INTO drama.shot_handoffs(
			shot_handoff_id,project_id,episode_id,from_shot_id,to_shot_id,from_action_phase,
			to_action_phase,version)VALUES($1,$2,$3,$4,$5,'start:slap','complete:slap',1)`,
			handoffID, projectID, episode1, shot1, shot2); err != nil {
			t.Fatal(err)
		}
		if _, err = database.writer.Exec(ctx, `UPDATE drama.storyboard_shots
			SET action_description='start slap faster' WHERE shot_id=$1`, shot1); err != nil {
			t.Fatal(err)
		}
		var status string
		if err = database.pool.QueryRow(ctx, `SELECT status FROM drama.shot_handoffs
			WHERE shot_handoff_id=$1`, handoffID).Scan(&status); err != nil || status != "dirty" {
			t.Fatalf("adjacent handoff was not dirtied: %s %v", status, err)
		}
	})

	t.Run("frame issue creates a confirmed-gated local redo plan", func(t *testing.T) {
		issues := pc.RunVisualQC([]pc.FrameObservation{{
			Locator:        pc.FrameLocator{EpisodeID: episode1, SceneID: sceneID, ShotID: shot2, TimecodeMS: 40, Frame: 1},
			IdentityScores: map[string]float64{"char_lin": .61},
		}})
		if len(issues) != 1 {
			t.Fatalf("fixture issue count=%d", len(issues))
		}
		if _, err = database.SaveVisualQCFixtureRun(ctx, projectID, episode1, "phase4-db-fixture", issues); err != nil {
			t.Fatal(err)
		}
		plan, planErr := database.CreateVisualQCRedoPlan(ctx, issues[0].IssueID, nil)
		if planErr != nil || plan.Status != "validated" || plan.Plan.Target.EntityID != shot2 {
			t.Fatalf("local redo plan failed: %+v err=%v", plan, planErr)
		}
		var status string
		if err = database.pool.QueryRow(ctx, `SELECT status FROM drama.visual_qc_issues
			WHERE visual_qc_issue_id=$1`, issues[0].IssueID).Scan(&status); err != nil || status != "planned" {
			t.Fatalf("QC issue was not linked to plan: %s %v", status, err)
		}
	})
}
