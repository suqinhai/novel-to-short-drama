package store

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"short-drama-cms/backend/internal/localedit"
)

func TestLocalEditingFourScenariosIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE15_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE15_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suffix, _ := newPublicID("")
	id := func(prefix string) string { return prefix + suffix }
	projectID, storyBibleID, seasonID := id("p15_p_"), id("p15_b_"), id("p15_season_")
	episodeID, scriptID, sceneID := id("p15_ep_"), id("p15_script_"), id("p15_scene_")
	scene2ID, scene3ID := id("p15_scene2_"), id("p15_scene3_")
	dialogueID, storyboardID := id("p15_dialogue_"), id("p15_board_")
	shotID, otherShotID := id("p15_shot_"), id("p15_other_shot_")
	imageID, videoID := id("p15_image_"), id("p15_video_")

	defer func() {
		_, _ = database.writer.Exec(context.Background(), `DELETE FROM drama.projects WHERE project_id=$1`, projectID)
	}()
	setup := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO drama.projects(project_id,novel_name,target_episode_count,
			episode_duration_seconds,visual_style,aspect_ratio,target_platform,current_stage,status,test_mode)
			VALUES($1,'phase15-e2e',1,90,'mock','9:16','test','stage_5_completed','stage_5_completed',true)`, []any{projectID}},
		{`INSERT INTO drama.story_bibles(story_bible_id,project_id,version,status)
			VALUES($1,$2,1,'approved')`, []any{storyBibleID, projectID}},
		{`INSERT INTO drama.seasons(season_id,project_id,story_bible_id,title,target_episode_count,
			target_episode_duration_seconds,status,version)
			VALUES($1,$2,$3,'season',1,90,'approved',1)`, []any{seasonID, projectID, storyBibleID}},
		{`INSERT INTO drama.episode_outlines(episode_id,season_id,project_id,episode_number,title,
			estimated_duration_seconds,status,version)
			VALUES($1,$2,$3,1,'episode',90,'approved',1)`, []any{episodeID, seasonID, projectID}},
		{`INSERT INTO drama.episode_scripts(script_id,project_id,season_id,episode_id,version,title,
			estimated_duration_seconds,source_outline_version,status)
			VALUES($1,$2,$3,$4,1,'script',90,1,'approved')`, []any{scriptID, projectID, seasonID, episodeID}},
		{`INSERT INTO drama.script_scenes(scene_id,script_id,project_id,episode_id,scene_number,
			scene_purpose,actions,estimated_duration_seconds)
			VALUES($1,$2,$3,$4,1,'identity reveal','[]',50),
			      ($5,$2,$3,$4,2,'pursuit','[]',20),
			      ($6,$2,$3,$4,3,'aftermath','[]',20)`,
			[]any{sceneID, scriptID, projectID, episodeID, scene2ID, scene3ID}},
		{`INSERT INTO drama.dialogues(dialogue_id,project_id,episode_id,scene_id,sequence_number,
			dialogue_type,speaker_name,text,emotion,estimated_duration_ms)
			VALUES($1,$2,$3,$4,1,'dialogue','heroine','old line','tense',2000)`, []any{dialogueID, projectID, episodeID, sceneID}},
		{`INSERT INTO drama.storyboards(storyboard_id,project_id,episode_id,script_id,version,status)
			VALUES($1,$2,$3,$4,1,'approved')`, []any{storyboardID, projectID, episodeID, scriptID}},
		{`INSERT INTO drama.storyboard_shots(shot_id,storyboard_id,project_id,episode_id,scene_id,
			shot_number,shot_order,duration_seconds,shot_size,camera_angle,camera_motion,
			action_description,status,generation_version)
			VALUES($1,$2,$3,$4,$5,1,1,5,'medium','eye','static','old action','approved',1),
			      ($6,$2,$3,$4,$5,2,2,5,'medium','eye','static','unrelated','approved',1)`,
			[]any{shotID, storyboardID, projectID, episodeID, sceneID, otherShotID}},
		{`INSERT INTO drama.storyboard_images(storyboard_image_id,project_id,episode_id,storyboard_id,
			shot_id,generation_version,source_storyboard_version,final_prompt,provider,model,
			status,review_status,is_current)
			VALUES($1,$2,$3,$4,$5,1,1,'mock','deterministic_mock','mock','succeeded','approved',true)`,
			[]any{imageID, projectID, episodeID, storyboardID, shotID}},
		{`INSERT INTO drama.shot_videos(shot_video_id,project_id,episode_id,storyboard_id,shot_id,
			storyboard_image_id,source_image_generation_version,generation_version,provider,model,
			video_prompt,reference_image_url,requested_duration_seconds,actual_duration_seconds,
			aspect_ratio,status,auto_qc_status,review_status,is_current)
			VALUES($1,$2,$3,$4,$5,$6,1,1,'deterministic_mock','mock','old','mock://image',
			5,5,'9:16','succeeded','passed','approved',true)`,
			[]any{videoID, projectID, episodeID, storyboardID, shotID, imageID}},
	}
	for _, statement := range setup {
		if _, err = database.writer.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	insertArtifactGraph(t, ctx, database, projectID, dialogueID, sceneID, shotID, otherShotID, videoID, suffix)

	t.Run("unconfirmed plan is preview only", func(t *testing.T) {
		plan := mustPlan(t, localedit.Request{
			Instruction: "只改一句对白", Target: localedit.Target{EntityType: "dialogue", EntityID: dialogueID, Version: 1},
			Changes: []localedit.Change{{Operation: "replace", Field: "text", Value: "new line"}},
		})
		record, err := database.CreateChangePlan(ctx, projectID, plan, nil)
		if err != nil {
			t.Fatal(err)
		}
		var text string
		var versionCount int
		_ = database.pool.QueryRow(ctx, `SELECT text FROM drama.dialogues WHERE dialogue_id=$1`, dialogueID).Scan(&text)
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.entity_versions WHERE entity_type='dialogue' AND entity_id=$1`, dialogueID).Scan(&versionCount)
		if record.Status != "validated" || text != "old line" || versionCount != 0 {
			t.Fatalf("unconfirmed plan mutated formal data: status=%s text=%s versions=%d", record.Status, text, versionCount)
		}
	})

	scenarios := []struct {
		name      string
		request   localedit.Request
		wantTypes []string
	}{
		{"dialogue edit", localedit.Request{
			Instruction: "只改一句对白", Target: localedit.Target{EntityType: "dialogue", EntityID: dialogueID, Version: 1},
			Changes: []localedit.Change{{Operation: "replace", Field: "emotion", Value: "restrained"}},
		}, []string{"dialogue_audio", "edit_timeline"}},
		{"scene shorten", localedit.Request{
			Instruction: "把第2场缩短20秒，但保留身份揭露", Target: localedit.Target{EntityType: "scene", EntityID: sceneID, Version: 1},
			Changes:      []localedit.Change{{Operation: "adjust", Field: "estimated_duration_seconds", Delta: -20}},
			MustPreserve: []string{"身份揭露"},
		}, []string{"edit_timeline", "shot_video", "storyboard_image", "storyboard_shot"}},
		{"shot action edit", localedit.Request{
			Instruction: "保留人物和场景，只重做第6镜动作", Target: localedit.Target{EntityType: "shot", EntityID: shotID, Version: 1},
			Changes: []localedit.Change{{Operation: "replace", Field: "action_description", Value: "new action"}},
		}, []string{"edit_timeline", "shot_video", "storyboard_image"}},
		{"video segment redo", localedit.Request{
			Instruction: "只重做前2秒", Target: localedit.Target{EntityType: "shot_video", EntityID: videoID, Version: 1},
			Changes: []localedit.Change{{Operation: "regenerate_segment", Field: "segment", Value: "stronger", StartMS: pointer(int64(0)), EndMS: pointer(int64(2000))}},
		}, []string{"edit_timeline"}},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			plan := mustPlan(t, scenario.request)
			record, err := database.CreateChangePlan(ctx, projectID, plan, nil)
			if err != nil {
				t.Fatal(err)
			}
			gotTypes := make([]string, len(record.Impacts))
			for index, impact := range record.Impacts {
				gotTypes[index] = impact.ArtifactType
				if impact.NativeEntityID == otherShotID {
					t.Fatal("unrelated shot entered impact")
				}
			}
			sort.Strings(gotTypes)
			sort.Strings(scenario.wantTypes)
			if !equalStrings(gotTypes, scenario.wantTypes) {
				t.Fatalf("impact types=%v want=%v", gotTypes, scenario.wantTypes)
			}
			record, err = database.ConfirmChangePlan(ctx, projectID, record.ChangePlanID, nil)
			if err != nil || record.Status != "confirmed" {
				t.Fatalf("confirm: status=%s err=%v", record.Status, err)
			}
			record, err = database.ExecuteChangePlan(ctx, projectID, record.ChangePlanID)
			if err != nil || record.Status != "applied" {
				t.Fatalf("execute: status=%s err=%v", record.Status, err)
			}
			for _, task := range record.RebuildTasks {
				if task.Status != "pending" || task.Provider != "local_conformance" ||
					task.CompletedAt != nil {
					t.Fatalf("rebuild was falsely completed without execution: %+v", task)
				}
			}
		})
	}

	t.Run("rollback and reapply also require confirmed plans", func(t *testing.T) {
		versions, err := database.ListEntityVersions(ctx, projectID, "dialogue", dialogueID)
		if err != nil || len(versions) != 2 {
			t.Fatalf("dialogue versions: %+v err=%v", versions, err)
		}
		var originalID, editedID string
		for _, version := range versions {
			if version.Version == 1 {
				originalID = version.EntityVersionID
			}
			if version.Version == 2 {
				editedID = version.EntityVersionID
			}
		}
		rollback, err := database.CreateVersionRestorePlan(ctx, projectID, originalID, "rollback", nil, nil)
		if err != nil || rollback.Status != "validated" {
			t.Fatalf("rollback preview: %+v err=%v", rollback, err)
		}
		emotion := currentEntityString(t, ctx, database, "dialogue", dialogueID, "emotion")
		if emotion != "restrained" {
			t.Fatalf("rollback preview changed current version: %s", emotion)
		}
		rollback, err = database.ConfirmChangePlan(ctx, projectID, rollback.ChangePlanID, nil)
		if err == nil {
			rollback, err = database.ExecuteChangePlan(ctx, projectID, rollback.ChangePlanID)
		}
		if err != nil || rollback.Status != "applied" {
			t.Fatalf("rollback execute: %+v err=%v", rollback, err)
		}
		emotion = currentEntityString(t, ctx, database, "dialogue", dialogueID, "emotion")
		if emotion != "tense" {
			t.Fatalf("rollback did not restore v1: %s", emotion)
		}
		reapply, err := database.CreateVersionRestorePlan(ctx, projectID, editedID, "reapply", nil, nil)
		if err != nil || reapply.Status != "validated" {
			t.Fatalf("reapply preview: %+v err=%v", reapply, err)
		}
		reapply, err = database.ConfirmChangePlan(ctx, projectID, reapply.ChangePlanID, nil)
		if err == nil {
			reapply, err = database.ExecuteChangePlan(ctx, projectID, reapply.ChangePlanID)
		}
		emotion = currentEntityString(t, ctx, database, "dialogue", dialogueID, "emotion")
		if err != nil || reapply.Status != "applied" || emotion != "restrained" {
			t.Fatalf("reapply failed: status=%s emotion=%s err=%v", reapply.Status, emotion, err)
		}
		var formalEmotion string
		if err = database.pool.QueryRow(ctx, `SELECT emotion FROM drama.dialogues
			WHERE dialogue_id=$1`, dialogueID).Scan(&formalEmotion); err != nil {
			t.Fatal(err)
		}
		if formalEmotion != "tense" {
			t.Fatalf("rollback/reapply overwrote immutable formal row: %s", formalEmotion)
		}
	})

	t.Run("concurrent plan against superseded current returns conflict", func(t *testing.T) {
		version := currentEntityVersion(t, ctx, database, "dialogue", dialogueID)
		build := func(value string) ChangePlan {
			plan := mustPlan(t, localedit.Request{
				Instruction: "concurrent dialogue edit",
				Target: localedit.Target{
					EntityType: "dialogue", EntityID: dialogueID, Version: version,
				},
				Changes: []localedit.Change{{
					Operation: "replace", Field: "text", Value: value,
				}},
			})
			record, createErr := database.CreateChangePlan(ctx, projectID, plan, nil)
			if createErr != nil {
				t.Fatal(createErr)
			}
			record, createErr = database.ConfirmChangePlan(ctx, projectID, record.ChangePlanID, nil)
			if createErr != nil {
				t.Fatal(createErr)
			}
			return record
		}
		first, second := build("first concurrent line"), build("stale concurrent line")
		if _, err = database.ExecuteChangePlan(ctx, projectID, first.ChangePlanID); err != nil {
			t.Fatal(err)
		}
		if _, err = database.ExecuteChangePlan(ctx, projectID, second.ChangePlanID); !errors.Is(err, ErrConflict) {
			t.Fatalf("expected stale current conflict, got %v", err)
		}
		second, err = database.GetChangePlan(ctx, projectID, second.ChangePlanID)
		if err != nil || second.Status != "confirmed" {
			t.Fatalf("conflicted plan left a partial executing state: status=%s err=%v", second.Status, err)
		}
		if currentEntityString(t, ctx, database, "dialogue", dialogueID, "text") != "first concurrent line" {
			t.Fatal("conflicted plan changed the current dialogue")
		}
	})

	t.Run("scene reorder versions displaced scenes and only adjacent continuity", func(t *testing.T) {
		version := currentEntityVersion(t, ctx, database, "scene", sceneID)
		plan := mustPlan(t, localedit.Request{
			Instruction: "move the first scene to position three",
			Target: localedit.Target{
				EntityType: "scene", EntityID: sceneID, Version: version,
			},
			Changes: []localedit.Change{{
				Operation: "reorder", Field: "scene_number", Value: 3,
			}},
		})
		record, createErr := database.CreateChangePlan(ctx, projectID, plan, nil)
		if createErr != nil {
			t.Fatal(createErr)
		}
		record, createErr = database.ConfirmChangePlan(ctx, projectID, record.ChangePlanID, nil)
		if createErr == nil {
			record, createErr = database.ExecuteChangePlan(ctx, projectID, record.ChangePlanID)
		}
		if createErr != nil {
			t.Fatal(createErr)
		}
		if currentEntityString(t, ctx, database, "scene", sceneID, "scene_number") != "3" ||
			currentEntityString(t, ctx, database, "scene", scene2ID, "scene_number") != "1" ||
			currentEntityString(t, ctx, database, "scene", scene3ID, "scene_number") != "2" {
			t.Fatal("scene reorder did not create a coherent successor order")
		}
		continuityIDs := []string{}
		for _, task := range record.RebuildTasks {
			if task.Action == "update_continuity" {
				continuityIDs = append(continuityIDs, task.TargetEntityID)
			}
		}
		sort.Strings(continuityIDs)
		expectedIDs := []string{sceneID, scene2ID, scene3ID}
		sort.Strings(expectedIDs)
		if !equalStrings(continuityIDs, expectedIDs) {
			t.Fatalf("continuity rebuild scope=%v want=%v", continuityIDs, expectedIDs)
		}
		var nativeOrder []int
		rows, queryErr := database.pool.Query(ctx, `SELECT scene_number FROM drama.script_scenes
			WHERE scene_id=ANY($1) ORDER BY scene_number`, []string{sceneID, scene2ID, scene3ID})
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		for rows.Next() {
			var number int
			if queryErr = rows.Scan(&number); queryErr != nil {
				rows.Close()
				t.Fatal(queryErr)
			}
			nativeOrder = append(nativeOrder, number)
		}
		rows.Close()
		if !reflect.DeepEqual(nativeOrder, []int{1, 2, 3}) {
			t.Fatalf("scene reorder overwrote immutable formal rows: %v", nativeOrder)
		}
	})

	t.Run("failed execution leaves no partial current version", func(t *testing.T) {
		var currentVideoID string
		_ = database.pool.QueryRow(ctx, `SELECT shot_video_id FROM drama.shot_videos
			WHERE shot_id=$1 AND is_current ORDER BY generation_version DESC LIMIT 1`, shotID).Scan(&currentVideoID)
		version := currentEntityVersion(t, ctx, database, "shot_video", currentVideoID)
		var versionCountBefore int
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.entity_versions
			WHERE entity_type='shot_video' AND entity_id=$1`, currentVideoID).Scan(&versionCountBefore)
		plan := mustPlan(t, localedit.Request{
			Instruction: "重做超出时长的片段",
			Target:      localedit.Target{EntityType: "shot_video", EntityID: currentVideoID, Version: version},
			Changes:     []localedit.Change{{Operation: "regenerate_segment", Field: "segment", Value: "invalid", StartMS: pointer(int64(0)), EndMS: pointer(int64(999999))}},
		})
		record, err := database.CreateChangePlan(ctx, projectID, plan, nil)
		if err != nil {
			t.Fatal(err)
		}
		record, err = database.ConfirmChangePlan(ctx, projectID, record.ChangePlanID, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = database.ExecuteChangePlan(ctx, projectID, record.ChangePlanID)
		if !errors.Is(err, localedit.ErrInvalidPlan) {
			t.Fatalf("expected invalid segment, got %v", err)
		}
		var afterCurrent string
		var versionCountAfter int
		_ = database.pool.QueryRow(ctx, `SELECT shot_video_id FROM drama.shot_videos WHERE shot_id=$1 AND is_current`, shotID).Scan(&afterCurrent)
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.entity_versions
			WHERE entity_type='shot_video' AND entity_id=$1`, currentVideoID).Scan(&versionCountAfter)
		if afterCurrent != currentVideoID || versionCountAfter != versionCountBefore ||
			currentEntityVersion(t, ctx, database, "shot_video", currentVideoID) != version {
			t.Fatalf("failed transaction left partial current: current=%s versions=%d→%d",
				afterCurrent, versionCountBefore, versionCountAfter)
		}
	})
}

func insertArtifactGraph(t *testing.T, ctx context.Context, database *Store, projectID, dialogueID, sceneID, shotID, otherShotID, videoID, suffix string) {
	t.Helper()
	hash := strings.Repeat("a", 64)
	rows := []struct{ id, kind, native string }{
		{"ar_d_" + suffix, "episode_script", dialogueID},
		{"ar_audio_" + suffix, "dialogue_audio", dialogueID},
		{"ar_dt_d_" + suffix, "edit_timeline", dialogueID},
		{"ar_s_" + suffix, "script_scene", sceneID},
		{"ar_sh_" + suffix, "storyboard_shot", shotID},
		{"ar_img_" + suffix, "storyboard_image", shotID},
		{"ar_vid_" + suffix, "shot_video", shotID},
		{"ar_dt_sh_" + suffix, "edit_timeline", shotID},
		{"ar_other_" + suffix, "storyboard_shot", otherShotID},
		{"ar_vroot_" + suffix, "shot_video", videoID},
		{"ar_dt_v_" + suffix, "edit_timeline", videoID},
	}
	for _, row := range rows {
		_, err := database.writer.Exec(ctx, `INSERT INTO drama.artifacts(
			artifact_id,artifact_type,project_id,native_entity_id,content_hash,idempotency_key)
			VALUES($1,$2,$3,$4,$5,$1)`, row.id, row.kind, projectID, row.native, hash)
		if err != nil {
			t.Fatal(err)
		}
	}
	edges := [][2]string{
		{"ar_d_", "ar_audio_"}, {"ar_d_", "ar_dt_d_"},
		{"ar_s_", "ar_sh_"}, {"ar_sh_", "ar_img_"}, {"ar_img_", "ar_vid_"}, {"ar_vid_", "ar_dt_sh_"},
		{"ar_vroot_", "ar_dt_v_"},
	}
	for index, edge := range edges {
		_, err := database.writer.Exec(ctx, `INSERT INTO drama.artifact_dependencies(
			artifact_dependency_id,upstream_artifact_id,downstream_artifact_id,dependency_type,
			observed_upstream_hash,invalidates_on,idempotency_key)
			VALUES($1,$2,$3,'local_edit_test',$4,'["content_changed","removed"]',$1)`,
			"ard_"+suffix+"_"+string(rune('a'+index)), edge[0]+suffix, edge[1]+suffix, hash)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func mustPlan(t *testing.T, request localedit.Request) localedit.Plan {
	t.Helper()
	plan, err := localedit.Build(request)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func pointer[T any](value T) *T { return &value }

func currentEntityVersion(
	t *testing.T, ctx context.Context, database *Store, entityType, entityID string,
) int {
	t.Helper()
	var version int
	if err := database.pool.QueryRow(ctx, `SELECT version FROM drama.entity_versions
		WHERE entity_type=$1 AND entity_id=$2 AND is_current`, entityType, entityID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func currentEntityString(
	t *testing.T, ctx context.Context, database *Store, entityType, entityID, field string,
) string {
	t.Helper()
	var value string
	if err := database.pool.QueryRow(ctx, `SELECT content->>$3 FROM drama.entity_versions
		WHERE entity_type=$1 AND entity_id=$2 AND is_current`,
		entityType, entityID, field).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
