package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"short-drama-cms/backend/internal/localedit"
	"short-drama-cms/backend/internal/postproduction"
)

func TestPhase5PostProductionMockChainIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE5_POST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE5_POST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	const projectID = "p_phase1_legacy"
	const episodeID = "ep_phase1_legacy_001"

	t.Run("unified workbench reads prior stages by shared ids", func(t *testing.T) {
		result, getErr := database.GetCreativeWorkbench(ctx, projectID, episodeID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if len(result.Scenes) < 10 || len(result.Dialogues) < 10 || len(result.Shots) < 10 ||
			len(result.PerformanceBibles) < 10 || len(result.Continuity) < 10 ||
			len(result.VisualQCIssues) < 10 {
			t.Fatalf("workbench omitted upstream stage data: %#v", result)
		}
		var diagnostic map[string]any
		var pacing, candidates, sounds []map[string]any
		if json.Unmarshal(result.Diagnostic, &diagnostic) != nil ||
			json.Unmarshal(result.PacingBeats, &pacing) != nil ||
			json.Unmarshal(result.Candidates, &candidates) != nil ||
			json.Unmarshal(result.SoundCues, &sounds) != nil ||
			diagnostic["diagnostic_report_id"] != "diagnostic_phase5_v1" ||
			len(pacing) != 2 || len(candidates) != 2 || len(sounds) != 3 {
			t.Fatalf("complete diagnosis -> pacing -> candidates -> sound chain missing: diagnostic=%s pacing=%s candidates=%s sounds=%s",
				result.Diagnostic, result.PacingBeats, result.Candidates, result.SoundCues)
		}
	})

	t.Run("template switches and restore always create successors", func(t *testing.T) {
		firstPlan := executeIntegrationChangePlan(t, ctx, database, projectID, localedit.Request{
			Instruction: "switch project editing template through a confirmed change plan",
			Target: localedit.Target{
				EntityType: "timeline", EntityID: episodeID, Version: 1,
			},
			Changes: []localedit.Change{
				{Operation: "replace", Field: "editing_template_version_id", Value: "etv_system_urban_power_v1"},
				{Operation: "replace", Field: "template_scope", Value: "project"},
				{Operation: "replace", Field: "override_config", Value: map[string]any{"fast_cut_ratio": .66}},
			},
		})
		first := latestTimeline(t, ctx, database, projectID, episodeID)
		if firstPlan.Status != "applied" || len(firstPlan.RebuildTasks) != 1 ||
			firstPlan.RebuildTasks[0].Status != "pending" {
			t.Fatalf("template change was not queued through the version executor: %#v", firstPlan)
		}
		executeIntegrationChangePlan(t, ctx, database, projectID, localedit.Request{
			Instruction: "switch episode editing template through a confirmed change plan",
			Target: localedit.Target{
				EntityType: "timeline", EntityID: episodeID, Version: 2,
			},
			Changes: []localedit.Change{
				{Operation: "replace", Field: "editing_template_version_id", Value: "etv_system_action_v1"},
				{Operation: "replace", Field: "template_scope", Value: "episode"},
				{Operation: "replace", Field: "override_config", Value: map[string]any{"fast_cut_ratio": .8}},
			},
		})
		second := latestTimeline(t, ctx, database, projectID, episodeID)
		if first.Version != 2 || second.Version != 3 || second.ParentTimelineID == nil ||
			*second.ParentTimelineID != first.TimelineID {
			t.Fatalf("template switch did not create a linear timeline history: first=%#v second=%#v", first, second)
		}
		executeIntegrationChangePlan(t, ctx, database, projectID, localedit.Request{
			Instruction: "restore historical timeline as a new successor",
			Target: localedit.Target{
				EntityType: "timeline", EntityID: episodeID, Version: 3,
			},
			Changes: []localedit.Change{{
				Operation: "replace", Field: "restore_source_timeline_id", Value: "timeline_phase5_v1",
			}},
		})
		restored := latestTimeline(t, ctx, database, projectID, episodeID)
		if restored.Version != 4 || restored.ParentTimelineID == nil ||
			*restored.ParentTimelineID != "timeline_phase5_v1" ||
			restored.ApprovalState != "draft" || restored.IsCurrent {
			t.Fatalf("restore must clone a new non-current draft version: %#v", restored)
		}
		var originalCurrent bool
		if scanErr := database.pool.QueryRow(ctx, `SELECT is_current FROM drama.edit_timelines
			WHERE timeline_id='timeline_phase5_v1'`).Scan(&originalCurrent); scanErr != nil {
			t.Fatal(scanErr)
		}
		if !originalCurrent {
			t.Fatal("restoring a draft must preserve the last approved current row")
		}
	})

	t.Run("whole episode sound style replacement preserves source cues and timeline", func(t *testing.T) {
		result := executeIntegrationChangePlan(t, ctx, database, projectID, localedit.Request{
			Instruction: "replace episode sound style through a confirmed change plan",
			Target: localedit.Target{
				EntityType: "timeline", EntityID: episodeID, Version: 4,
			},
			Changes: []localedit.Change{{
				Operation: "replace", Field: "sound_style_group", Value: "cinematic_noir",
			}},
		})
		timeline := latestTimeline(t, ctx, database, projectID, episodeID)
		if timeline.Version != 5 || len(result.RebuildTasks) != 1 ||
			result.RebuildTasks[0].Status != "pending" {
			t.Fatalf("unexpected sound style successor plan: timeline=%#v plan=%#v", timeline, result)
		}
		var requestedStyle string
		if err = database.pool.QueryRow(ctx, `SELECT render_config->>'sound_style_group'
			FROM drama.edit_timelines WHERE timeline_id=$1`, timeline.TimelineID).Scan(&requestedStyle); err != nil {
			t.Fatal(err)
		}
		var historical, currentNoir, originalTimeline int
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.sound_cue_versions
			WHERE episode_id=$1`, episodeID).Scan(&historical)
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.sound_cue_versions cue
			JOIN drama.sound_asset_versions version USING(sound_asset_version_id)
			JOIN drama.sound_assets asset USING(sound_asset_id)
			WHERE cue.episode_id=$1 AND cue.is_current AND asset.style_group='cinematic_noir'`,
			episodeID).Scan(&currentNoir)
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.edit_timelines
			WHERE timeline_id='timeline_phase5_v1'`).Scan(&originalTimeline)
		if requestedStyle != "cinematic_noir" || historical != 3 || currentNoir != 0 ||
			originalTimeline != 1 {
			t.Fatalf("pending sound rebuild mutated old media: style=%q cues=%d current_noir=%d original_timeline=%d",
				requestedStyle, historical, currentNoir, originalTimeline)
		}
	})

	t.Run("speaker lip and turn validation persists exact editor located issues", func(t *testing.T) {
		items := []postproduction.DialogueTiming{
			{
				DialogueID: "dlg_phase5_1", DialogueAudioID: "audio_phase5_1",
				ShotID: "shot_phase5_1", SpeakerCharacterID: "char_lin", SpeakerName: "林夏",
				TurnGroup: "door_exchange", TurnIndex: 1, StartMS: 800, EndMS: 2600,
				AudioDurationMS: 2050, TargetLipStartMS: 800, TargetLipEndMS: 2500,
				VisibleCharacterIDs: []string{"char_lin"}, DetectedSpeakerID: "char_lin",
				DetectedLipStartMS: 980, DetectedLipEndMS: 2700, Confidence: .92,
			},
			{
				DialogueID: "dlg_phase5_2", DialogueAudioID: "audio_phase5_2",
				ShotID: "shot_phase5_2", SpeakerCharacterID: "char_zhou", SpeakerName: "周野",
				TurnGroup: "door_exchange", TurnIndex: 2, StartMS: 2450, EndMS: 4150,
				AudioDurationMS: 1700, TargetLipStartMS: 2450, TargetLipEndMS: 4150,
				VisibleCharacterIDs: []string{"char_zhou"}, DetectedSpeakerID: "char_zhou",
				DetectedLipStartMS: 2450, DetectedLipEndMS: 4150, Confidence: .98,
			},
		}
		report, validateErr := postproduction.ValidateDialogueTimings(items, 120)
		if validateErr != nil {
			t.Fatal(validateErr)
		}
		if report.Passed || len(report.Issues) < 3 {
			t.Fatalf("expected overrun, drift and turn issues: %#v", report)
		}
		if saveErr := database.SaveDialogueTimingValidation(ctx, projectID, episodeID, items, report, nil); saveErr != nil {
			t.Fatal(saveErr)
		}
		var timingCount, issueCount int
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.dialogue_timing_versions
			WHERE episode_id=$1 AND is_current`, episodeID).Scan(&timingCount)
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.dialogue_timing_issues
			WHERE episode_id=$1`, episodeID).Scan(&issueCount)
		if timingCount != 2 || issueCount < 3 {
			t.Fatalf("timing persistence incomplete: timings=%d issues=%d", timingCount, issueCount)
		}
	})

	t.Run("dialogue edit rebuilds exact range and preserves approved subtitle timeline history", func(t *testing.T) {
		plan, buildErr := localedit.Build(localedit.Request{
			Instruction: "修改这一句对白，只重建关联字幕、配音、镜头和剪辑区间",
			Target:      localedit.Target{EntityType: "dialogue", EntityID: "dlg_phase5_1", Version: 1},
			Changes:     []localedit.Change{{Operation: "replace", Field: "text", Value: "门，是有人从里面打开的。"}},
		})
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		record, createErr := database.CreateChangePlan(ctx, projectID, plan, nil)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if len(record.Plan.ExpectedChanges) != 1 ||
			record.Plan.ExpectedChanges[0].StartMS == nil ||
			record.Plan.ExpectedChanges[0].EndMS == nil ||
			*record.Plan.ExpectedChanges[0].StartMS != 800 ||
			*record.Plan.ExpectedChanges[0].EndMS != 2600 {
			t.Fatalf("preview omitted the exact rebuild interval: %#v", record.Plan.ExpectedChanges)
		}
		record, err = database.ConfirmChangePlan(ctx, projectID, record.ChangePlanID, nil)
		if err != nil {
			t.Fatal(err)
		}
		record, err = database.ExecuteChangePlan(ctx, projectID, record.ChangePlanID)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status != "applied" || len(record.RebuildTasks) != 4 {
			t.Fatalf("expected voice, subtitle, shot interval and timeline interval rebuilds: %#v", record)
		}
		targetTypes := map[string]bool{}
		for _, task := range record.RebuildTasks {
			if task.Status != "pending" || task.Provider != "workflow" || task.CompletedAt != nil {
				t.Fatalf("media rebuild was falsely marked complete: %#v", task)
			}
			if task.RangeStartMS == nil || task.RangeEndMS == nil ||
				*task.RangeStartMS != 800 || *task.RangeEndMS != 2600 {
				t.Fatalf("rebuild escaped exact dialogue range: %#v", task)
			}
			targetTypes[task.TargetEntityType] = true
		}
		for _, expected := range []string{
			"dialogue", "storyboard_shot_interval", "edit_timeline_interval",
		} {
			if !targetTypes[expected] {
				t.Fatalf("missing exact dialogue rebuild target %s: %#v", expected, record.RebuildTasks)
			}
		}
		var originalText string
		var originalCurrent bool
		_ = database.pool.QueryRow(ctx, `SELECT text,is_current FROM drama.subtitle_cues
			WHERE subtitle_cue_id='subtitle_phase5_1'`).Scan(&originalText, &originalCurrent)
		if originalText != "门不是风吹开的。" || !originalCurrent {
			t.Fatalf("pending rebuild changed approved subtitle media: old=%q current=%v",
				originalText, originalCurrent)
		}
		if currentEntityString(t, ctx, database, "dialogue", "dlg_phase5_1", "text") !=
			"门，是有人从里面打开的。" {
			t.Fatal("dialogue successor did not become current")
		}
		var oldDuration int64
		_ = database.pool.QueryRow(ctx, `SELECT duration_ms FROM drama.edit_timeline_items
			WHERE timeline_item_id='item_phase5_dialogue_1'`).Scan(&oldDuration)
		if oldDuration != 1800 || latestTimeline(t, ctx, database, projectID, episodeID).Version != 5 {
			t.Fatalf("pending dialogue rebuild changed old timeline: old=%d", oldDuration)
		}
	})

	t.Run("every final mock artifact remains traceable to source fact spec prompt model and manual history", func(t *testing.T) {
		var traced int
		err = database.pool.QueryRow(ctx, `SELECT count(DISTINCT artifact.artifact_id)
			FROM drama.artifacts artifact
			JOIN drama.artifact_source_evidence evidence USING(artifact_id)
			JOIN drama.artifact_provenance_events provenance USING(artifact_id)
			WHERE artifact.artifact_id IN(
			  'artifact_phase5_dialogue','artifact_phase5_audio',
			  'artifact_phase5_timeline','artifact_phase5_master')
			  AND evidence.source_span_id IS NOT NULL AND evidence.fact_revision_id IS NOT NULL
			  AND provenance.adaptation_spec_version_id IS NOT NULL
			  AND provenance.prompt_version IS NOT NULL AND provenance.model_version IS NOT NULL`).Scan(&traced)
		if err != nil {
			t.Fatal(err)
		}
		// Only the dialogue artifact carries direct source evidence; downstream
		// provenance is reached through explicit artifact_dependencies.
		if traced != 1 {
			t.Fatalf("root artifact provenance missing: traced=%d", traced)
		}
		var downstream int
		_ = database.pool.QueryRow(ctx, `WITH RECURSIVE lineage AS(
			SELECT artifact_id FROM drama.artifacts WHERE artifact_id='artifact_phase5_dialogue'
			UNION ALL
			SELECT dependency.downstream_artifact_id FROM lineage
			JOIN drama.artifact_dependencies dependency ON dependency.upstream_artifact_id=lineage.artifact_id
		) SELECT count(*) FROM lineage`).Scan(&downstream)
		if downstream != 4 {
			t.Fatalf("expected dialogue→audio→timeline→master lineage, got %d nodes", downstream)
		}
	})

	t.Run("NLE drafts never replace current until render succeeds and failed render rolls back", func(t *testing.T) {
		base := latestTimeline(t, ctx, database, projectID, episodeID)
		var itemID, approvedBefore string
		if err = database.pool.QueryRow(ctx, `SELECT timeline_item_id FROM drama.edit_timeline_items
			WHERE timeline_id=$1 AND track_type='dialogue' ORDER BY sequence_number LIMIT 1`,
			base.TimelineID).Scan(&itemID); err != nil {
			t.Fatal(err)
		}
		if err = database.pool.QueryRow(ctx, `SELECT timeline_id FROM drama.edit_timelines
			WHERE episode_id=$1 AND is_current`, episodeID).Scan(&approvedBefore); err != nil {
			t.Fatal(err)
		}
		volume := .75
		draft, draftErr := database.CreateNLEItemDraft(ctx, projectID, episodeID, itemID, NLETimelineItemPatch{
			BaseTimelineID: base.TimelineID, Volume: &volume, Reason: "acceptance_volume_edit",
		})
		if draftErr != nil {
			t.Fatal(draftErr)
		}
		if draft.Timeline.IsCurrent || draft.Timeline.ApprovalState != "draft" || draft.Item == nil {
			t.Fatalf("edit must create a non-current item-linked draft: %#v", draft)
		}
		job, renderErr := database.ConfirmNLETimelineRender(ctx, projectID, episodeID, draft.Timeline.TimelineID)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		if _, err = database.writer.Exec(ctx, `UPDATE drama.render_jobs
			SET status='failed',completed_at=now(),error_code='ACCEPTANCE_FAILURE',error_message='fixture failure'
			WHERE render_job_id=$1`, job.RenderJobID); err != nil {
			t.Fatal(err)
		}
		var currentAfterFailure, failedState string
		_ = database.pool.QueryRow(ctx, `SELECT timeline_id FROM drama.edit_timelines
			WHERE episode_id=$1 AND is_current`, episodeID).Scan(&currentAfterFailure)
		_ = database.pool.QueryRow(ctx, `SELECT approval_state FROM drama.edit_timelines
			WHERE timeline_id=$1`, draft.Timeline.TimelineID).Scan(&failedState)
		if currentAfterFailure != approvedBefore || failedState != "render_failed" {
			t.Fatalf("failed render replaced approved current: before=%s after=%s draft=%s",
				approvedBefore, currentAfterFailure, failedState)
		}

		restored, restoreErr := database.RestoreNLETimelineDraft(ctx, projectID, episodeID, "timeline_phase5_v1", nil)
		if restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if restored.Timeline.IsCurrent || restored.Timeline.ParentTimelineID == nil ||
			*restored.Timeline.ParentTimelineID != "timeline_phase5_v1" {
			t.Fatalf("restore must create a successor draft: %#v", restored)
		}
		successJob, successErr := database.ConfirmNLETimelineRender(ctx, projectID, episodeID, restored.Timeline.TimelineID)
		if successErr != nil {
			t.Fatal(successErr)
		}
		if _, err = database.writer.Exec(ctx, `UPDATE drama.render_jobs
			SET status='succeeded',progress=100,completed_at=now(),output_url='/results/acceptance.mp4'
			WHERE render_job_id=$1`, successJob.RenderJobID); err != nil {
			t.Fatal(err)
		}
		var promotedState string
		var promotedCurrent bool
		_ = database.pool.QueryRow(ctx, `SELECT approval_state,is_current FROM drama.edit_timelines
			WHERE timeline_id=$1`, restored.Timeline.TimelineID).Scan(&promotedState, &promotedCurrent)
		if promotedState != "approved" || !promotedCurrent {
			t.Fatalf("successful render did not atomically approve current: state=%s current=%v",
				promotedState, promotedCurrent)
		}
	})
}

func executeIntegrationChangePlan(
	t *testing.T, ctx context.Context, database *Store, projectID string, request localedit.Request,
) ChangePlan {
	t.Helper()
	plan, err := localedit.Build(request)
	if err != nil {
		t.Fatal(err)
	}
	record, err := database.CreateChangePlan(ctx, projectID, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	record, err = database.ConfirmChangePlan(ctx, projectID, record.ChangePlanID, nil)
	if err == nil {
		record, err = database.ExecuteChangePlan(ctx, projectID, record.ChangePlanID)
	}
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func latestTimeline(
	t *testing.T, ctx context.Context, database *Store, projectID, episodeID string,
) TimelineVersionRecord {
	t.Helper()
	versions, err := database.ListTimelineVersions(ctx, projectID, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) > 0 {
		return versions[0]
	}
	t.Fatal("timeline not found")
	return TimelineVersionRecord{}
}
