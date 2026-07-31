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
		first, applyErr := database.ApplyEditingTemplate(ctx, projectID, episodeID, ApplyEditingTemplateInput{
			EditingTemplateVersionID: "etv_system_urban_power_v1",
			Scope:                    "project", OverrideConfig: map[string]any{"fast_cut_ratio": .66},
			Reason: "project template acceptance",
		})
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		second, applyErr := database.ApplyEditingTemplate(ctx, projectID, episodeID, ApplyEditingTemplateInput{
			EditingTemplateVersionID: "etv_system_action_v1",
			Scope:                    "episode", OverrideConfig: map[string]any{"fast_cut_ratio": .8},
			Reason: "episode override acceptance",
		})
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		if first.Version != 2 || second.Version != 3 || second.ParentTimelineID == nil ||
			*second.ParentTimelineID != first.TimelineID {
			t.Fatalf("template switch did not create a linear timeline history: first=%#v second=%#v", first, second)
		}
		restored, restoreErr := database.RestoreTimelineVersion(ctx, projectID, episodeID, "timeline_phase5_v1", nil)
		if restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if restored.Version != 4 || restored.ParentTimelineID == nil ||
			*restored.ParentTimelineID != "timeline_phase5_v1" ||
			restored.ApprovalState != "restored" || !restored.IsCurrent {
			t.Fatalf("restore must clone a new current version: %#v", restored)
		}
		var originalCurrent bool
		if scanErr := database.pool.QueryRow(ctx, `SELECT is_current FROM drama.edit_timelines
			WHERE timeline_id='timeline_phase5_v1'`).Scan(&originalCurrent); scanErr != nil {
			t.Fatal(scanErr)
		}
		if originalCurrent {
			t.Fatal("restoring may not reactivate or overwrite the historical row")
		}
	})

	t.Run("whole episode sound style replacement preserves source cues and timeline", func(t *testing.T) {
		result, replaceErr := database.ReplaceEpisodeSoundStyle(ctx, projectID, episodeID, ReplaceSoundStyleInput{
			ToStyleGroup: "cinematic_noir", Reason: "sound style acceptance",
		})
		if replaceErr != nil {
			t.Fatal(replaceErr)
		}
		if result.Timeline.Version != 5 || result.FromStyleGroup != "suspense_minimal" ||
			result.ToStyleGroup != "cinematic_noir" || len(result.ReplacedCueVersionIDs) != 3 {
			t.Fatalf("unexpected sound style successor: %#v", result)
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
		if historical != 6 || currentNoir != 3 || originalTimeline != 1 {
			t.Fatalf("sound replacement overwrote history: cues=%d current_noir=%d original_timeline=%d",
				historical, currentNoir, originalTimeline)
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
		record, err = database.ConfirmChangePlan(ctx, projectID, record.ChangePlanID, nil)
		if err != nil {
			t.Fatal(err)
		}
		record, err = database.ExecuteChangePlan(ctx, projectID, record.ChangePlanID)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status != "applied" || len(record.RebuildTasks) != 3 {
			t.Fatalf("expected voice, subtitle and timeline rebuilds: %#v", record)
		}
		for _, task := range record.RebuildTasks {
			if task.RangeStartMS == nil || task.RangeEndMS == nil ||
				*task.RangeStartMS != 800 || *task.RangeEndMS != 2600 {
				t.Fatalf("rebuild escaped exact dialogue range: %#v", task)
			}
		}
		var originalText, currentText string
		var originalCurrent bool
		_ = database.pool.QueryRow(ctx, `SELECT text,is_current FROM drama.subtitle_cues
			WHERE subtitle_cue_id='subtitle_phase5_1'`).Scan(&originalText, &originalCurrent)
		_ = database.pool.QueryRow(ctx, `SELECT text FROM drama.subtitle_cues
			WHERE dialogue_id='dlg_phase5_1' AND is_current`).Scan(&currentText)
		if originalText != "门不是风吹开的。" || originalCurrent ||
			currentText != "门，是有人从里面打开的。" {
			t.Fatalf("subtitle version history was overwritten: old=%q/%v current=%q",
				originalText, originalCurrent, currentText)
		}
		var oldDuration, newDuration int64
		_ = database.pool.QueryRow(ctx, `SELECT duration_ms FROM drama.edit_timeline_items
			WHERE timeline_item_id='item_phase5_dialogue_1'`).Scan(&oldDuration)
		_ = database.pool.QueryRow(ctx, `SELECT item.duration_ms FROM drama.edit_timeline_items item
			JOIN drama.edit_timelines timeline USING(timeline_id)
			WHERE timeline.is_current AND item.entity_type='dialogue' AND item.entity_id='dlg_phase5_1'
			  AND item.track_type='dialogue'`).Scan(&newDuration)
		if oldDuration != 1800 || newDuration != 1800 {
			t.Fatalf("timeline history or exact successor changed unexpectedly: old=%d new=%d", oldDuration, newDuration)
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
}
