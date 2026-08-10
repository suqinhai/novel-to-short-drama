package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"short-drama-cms/backend/internal/shoteditor"
)

func TestAtomicMultiShotEditorIntegration(t *testing.T) {
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
	projectID, storyBibleID, seasonID := id("p26_p_"), id("p26_bible_"), id("p26_season_")
	episodeID, scriptID, sceneID, boardID := id("p26_ep_"), id("p26_script_"), id("p26_scene_"), id("p26_board_")
	shotA, shotB, imageID := id("p26_shot_a_"), id("p26_shot_b_"), id("p26_image_")
	defer func() {
		_, _ = database.writer.Exec(context.Background(), `DELETE FROM drama.projects WHERE project_id=$1`, projectID)
	}()
	setup := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO drama.projects(project_id,novel_name,target_episode_count,episode_duration_seconds,visual_style,aspect_ratio,target_platform,current_stage,status,test_mode) VALUES($1,'shot-editor',1,90,'mock','9:16','test','stage_5_completed','stage_5_completed',true)`, []any{projectID}},
		{`INSERT INTO drama.story_bibles(story_bible_id,project_id,version,status) VALUES($1,$2,1,'approved')`, []any{storyBibleID, projectID}},
		{`INSERT INTO drama.seasons(season_id,project_id,story_bible_id,title,target_episode_count,target_episode_duration_seconds,status,version) VALUES($1,$2,$3,'season',1,90,'approved',1)`, []any{seasonID, projectID, storyBibleID}},
		{`INSERT INTO drama.episode_outlines(episode_id,season_id,project_id,episode_number,title,estimated_duration_seconds,status,version) VALUES($1,$2,$3,1,'episode',90,'approved',1)`, []any{episodeID, seasonID, projectID}},
		{`INSERT INTO drama.episode_scripts(script_id,project_id,season_id,episode_id,version,title,estimated_duration_seconds,source_outline_version,status) VALUES($1,$2,$3,$4,1,'script',90,1,'approved')`, []any{scriptID, projectID, seasonID, episodeID}},
		{`INSERT INTO drama.script_scenes(scene_id,script_id,project_id,episode_id,scene_number,scene_purpose,actions,estimated_duration_seconds) VALUES($1,$2,$3,$4,1,'letter reveal','[]',10)`, []any{sceneID, scriptID, projectID, episodeID}},
		{`INSERT INTO drama.storyboards(storyboard_id,project_id,episode_id,script_id,version,status,total_shots,estimated_duration_seconds) VALUES($1,$2,$3,$4,1,'approved',2,7)`, []any{boardID, projectID, episodeID, scriptID}},
		{`INSERT INTO drama.storyboard_shots(shot_id,storyboard_id,project_id,episode_id,scene_id,shot_number,shot_order,duration_seconds,shot_size,camera_angle,camera_motion,composition,character_ids,location_id,action_description,dialogue_ids,status,generation_version,head_state,tail_state,action_phase,axis,coverage_role) VALUES
		 ($1,$2,$3,$4,$5,1,1,4,'wide','eye','static','two shot','["alice","bob"]','room','Alice raises letter','[]','approved',1,'{"pose":"letter down","gaze":"bob"}','{"pose":"letter half raised","gaze":"bob"}','{"start":"raise/start","end":"raise/middle"}','axis_1','establishing'),
		 ($6,$2,$3,$4,$5,2,2,3,'medium','eye','push','alice single','["alice","bob"]','room','Alice presents letter','[]','approved',1,'{"pose":"letter half raised","gaze":"bob"}','{"pose":"letter raised","gaze":"bob"}','{"start":"raise/middle","end":"raise/end"}','axis_1','reaction')`, []any{shotA, boardID, projectID, episodeID, sceneID, shotB}},
		{`INSERT INTO drama.storyboard_images(storyboard_image_id,project_id,episode_id,storyboard_id,shot_id,generation_version,source_storyboard_version,final_prompt,provider,model,status,review_status,is_current,storage_url) VALUES($1,$2,$3,$4,$5,1,1,'legacy','mock','mock','succeeded','approved',true,'mock://legacy-shot-a')`, []any{imageID, projectID, episodeID, boardID, shotA}},
	}
	for _, statement := range setup {
		if _, err = database.writer.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("unconfirmed preview writes no formal shot data", func(t *testing.T) {
		plan, err := database.CreateShotEditPlan(ctx, projectID, episodeID, shoteditor.Request{Operation: shoteditor.OperationUpdate, ShotID: shotA, Patch: map[string]any{"shot_size": "close_up"}})
		if err != nil {
			t.Fatal(err)
		}
		var size string
		var versionCount, sequenceCount int
		_ = database.pool.QueryRow(ctx, `SELECT shot_size FROM drama.storyboard_shots WHERE shot_id=$1`, shotA).Scan(&size)
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.entity_versions WHERE entity_type='shot' AND entity_id=$1`, shotA).Scan(&versionCount)
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.shot_sequence_versions WHERE episode_id=$1`, episodeID).Scan(&sequenceCount)
		if plan.Status != "validated" || size != "wide" || versionCount != 0 || sequenceCount != 0 {
			t.Fatalf("preview mutated formal rows: status=%s size=%s versions=%d sequences=%d", plan.Status, size, versionCount, sequenceCount)
		}
	})

	t.Run("continuity conflict blocks confirmation", func(t *testing.T) {
		plan, err := database.CreateShotEditPlan(ctx, projectID, episodeID, shoteditor.Request{Operation: shoteditor.OperationReorder, OrderedShotIDs: []string{shotB, shotA}})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.ContinuityConflicts) == 0 {
			t.Fatal("reorder preview did not expose head/tail conflict")
		}
		if _, err = database.ConfirmShotEditPlan(ctx, projectID, episodeID, plan.ShotEditPlanID, nil); !errors.Is(err, ErrConflict) {
			t.Fatalf("conflicting plan confirmation should fail: %v", err)
		}
	})

	t.Run("concurrent multi-shot plans serialize without lost update", func(t *testing.T) {
		planA, err := database.CreateShotEditPlan(ctx, projectID, episodeID, shoteditor.Request{Operation: shoteditor.OperationUpdate, ShotID: shotA, Patch: map[string]any{"duration_seconds": 4.2}})
		if err != nil {
			t.Fatal(err)
		}
		planB, err := database.CreateShotEditPlan(ctx, projectID, episodeID, shoteditor.Request{Operation: shoteditor.OperationUpdate, ShotID: shotB, Patch: map[string]any{"camera_angle": "low"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = database.ConfirmShotEditPlan(ctx, projectID, episodeID, planA.ShotEditPlanID, nil); err != nil {
			t.Fatal(err)
		}
		if _, err = database.ConfirmShotEditPlan(ctx, projectID, episodeID, planB.ShotEditPlanID, nil); err != nil {
			t.Fatal(err)
		}
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for _, planID := range []string{planA.ShotEditPlanID, planB.ShotEditPlanID} {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				_, executeErr := database.ExecuteShotEditPlan(context.Background(), projectID, episodeID, id)
				errs <- executeErr
			}(planID)
		}
		wg.Wait()
		close(errs)
		successes, conflicts := 0, 0
		for executeErr := range errs {
			if executeErr == nil {
				successes++
			} else if errors.Is(executeErr, ErrConflict) {
				conflicts++
			} else {
				t.Errorf("unexpected concurrent error: %v", executeErr)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("concurrent result success=%d conflict=%d", successes, conflicts)
		}
		versions, err := database.ListShotSequenceVersions(ctx, projectID, episodeID)
		if err != nil {
			t.Fatal(err)
		}
		if len(versions) != 2 || !versions[0].IsCurrent || versions[0].Version != 2 {
			t.Fatalf("concurrent writes created incoherent sequence versions: %#v", versions)
		}
	})

	t.Run("version restore creates a successor and does not reuse legacy media", func(t *testing.T) {
		update, err := database.CreateShotEditPlan(ctx, projectID, episodeID, shoteditor.Request{Operation: shoteditor.OperationUpdate, ShotID: shotA, Patch: map[string]any{"composition": "forced close composition"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = database.ConfirmShotEditPlan(ctx, projectID, episodeID, update.ShotEditPlanID, nil); err != nil {
			t.Fatal(err)
		}
		if _, err = database.ExecuteShotEditPlan(ctx, projectID, episodeID, update.ShotEditPlanID); err != nil {
			t.Fatal(err)
		}
		versions, err := database.ListShotSequenceVersions(ctx, projectID, episodeID)
		if err != nil {
			t.Fatal(err)
		}
		var sourceID string
		for _, version := range versions {
			if version.Version == 1 {
				sourceID = version.ShotSequenceVersionID
			}
		}
		plan, err := database.CreateShotEditPlan(ctx, projectID, episodeID, shoteditor.Request{Operation: shoteditor.OperationRestore, SourceSequenceVersionID: sourceID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = database.ConfirmShotEditPlan(ctx, projectID, episodeID, plan.ShotEditPlanID, nil); err != nil {
			t.Fatal(err)
		}
		applied, err := database.ExecuteShotEditPlan(ctx, projectID, episodeID, plan.ShotEditPlanID)
		if err != nil {
			t.Fatal(err)
		}
		if applied.AppliedSequenceVersionID == nil {
			t.Fatal("restore did not produce a successor sequence")
		}
		tx, err := database.writer.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		shots, currentVersion, _, readErr := readCurrentShotSequence(ctx, tx, projectID, episodeID, false)
		_ = tx.Rollback(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if currentVersion != 4 {
			t.Fatalf("restore should create v4, got v%d", currentVersion)
		}
		for _, shot := range shots {
			if shot.ShotID == shotA && shot.ThumbnailURL != "" {
				t.Fatalf("legacy media leaked onto restored successor: %s", shot.ThumbnailURL)
			}
		}
	})

	t.Run("mid-transaction rebuild failure rolls back shots lineage continuity handoffs and current", func(t *testing.T) {
		tx, err := database.writer.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		current, currentVersion, _, readErr := readCurrentShotSequence(ctx, tx, projectID, episodeID, false)
		_ = tx.Rollback(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		source := current[0]
		first, second := source, source
		first.DurationSeconds, second.DurationSeconds = source.DurationSeconds/2, source.DurationSeconds-source.DurationSeconds/2
		first.ActionDescription, second.ActionDescription = "Alice grips letter", "Alice lifts letter"
		bridge := map[string]any{"pose": "letter quarter raised", "gaze": "bob"}
		first.TailState, second.HeadState = bridge, bridge
		first.ActionPhase = map[string]any{"start": "raise/start", "end": "raise/bridge"}
		second.ActionPhase = map[string]any{"start": "raise/bridge", "end": "raise/middle"}
		plan, err := database.CreateShotEditPlan(ctx, projectID, episodeID, shoteditor.Request{Operation: shoteditor.OperationSplit, BaseSequenceVersion: currentVersion, ShotID: source.ShotID, Shots: []shoteditor.Shot{first, second}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = database.ConfirmShotEditPlan(ctx, projectID, episodeID, plan.ShotEditPlanID, nil); err != nil {
			t.Fatal(err)
		}
		if !safeSQLIdentifier(projectID) {
			t.Fatal("unsafe generated project id")
		}
		triggerName := "trg_test_fail_" + strings.TrimPrefix(suffix, "_")
		functionName := "test_fail_" + strings.TrimPrefix(suffix, "_")
		ddl := fmt.Sprintf(`CREATE FUNCTION drama.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected rebuild failure'; END $$; CREATE TRIGGER %s BEFORE INSERT ON drama.incremental_rebuild_tasks FOR EACH ROW WHEN (NEW.project_id='%s') EXECUTE FUNCTION drama.%s()`, functionName, triggerName, projectID, functionName)
		if _, err = database.writer.Exec(ctx, ddl); err != nil {
			t.Fatal(err)
		}
		defer database.writer.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON drama.incremental_rebuild_tasks; DROP FUNCTION IF EXISTS drama.%s()`, triggerName, functionName))
		if _, err = database.ExecuteShotEditPlan(ctx, projectID, episodeID, plan.ShotEditPlanID); err == nil {
			t.Fatal("injected failure should abort execution")
		}
		var newShots, lineage, tasks int
		newIDs := []string{plan.ProposedSnapshot[0].ShotID, plan.ProposedSnapshot[1].ShotID}
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.storyboard_shots WHERE shot_id=ANY($1)`, newIDs).Scan(&newShots)
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.shot_lineage WHERE shot_edit_plan_id=$1`, plan.ShotEditPlanID).Scan(&lineage)
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.incremental_rebuild_tasks WHERE shot_edit_plan_id=$1`, plan.ShotEditPlanID).Scan(&tasks)
		tx, err = database.writer.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		after, afterVersion, _, readErr := readCurrentShotSequence(ctx, tx, projectID, episodeID, false)
		_ = tx.Rollback(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if newShots != 0 || lineage != 0 || tasks != 0 || afterVersion != currentVersion || len(after) != len(current) {
			t.Fatalf("failure leaked formal writes: shots=%d lineage=%d tasks=%d version=%d", newShots, lineage, tasks, afterVersion)
		}
		failed, err := database.GetShotEditPlan(ctx, projectID, episodeID, plan.ShotEditPlanID)
		if err != nil {
			t.Fatal(err)
		}
		if failed.Status != "failed" {
			t.Fatalf("failed plan status=%s", failed.Status)
		}
	})
}

func safeSQLIdentifier(value string) bool {
	for _, r := range value {
		if !(r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return value != ""
}
