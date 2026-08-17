package store

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"short-drama-cms/backend/internal/candidategeneration"
	"short-drama-cms/backend/internal/exportkit"
	"short-drama-cms/backend/internal/qualitygate"
)

func TestStep810P0P1ClosureIntegration(t *testing.T) {
	databaseURL := os.Getenv("PHASE31_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE31_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	const projectID, episodeID = "p_phase1_legacy", "ep_phase1_legacy_001"

	t.Run("authoritative QA snapshot binds current resolver timeline and master", func(t *testing.T) {
		snapshot, buildErr := database.BuildAuthoritativeQualityGateSnapshot(ctx, projectID, episodeID, "master_phase5_v1")
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		if snapshot.MasterID != "master_phase5_v1" || len(snapshot.Artifacts) != len(qualitygate.StageOrder) {
			t.Fatalf("authoritative snapshot incomplete: %#v", snapshot)
		}
		for _, artifact := range snapshot.Artifacts {
			if artifact.ArtifactID == "stale-media-ghost" {
				t.Fatal("client-forged artifact entered authoritative snapshot")
			}
			if artifact.Stage == qualitygate.StageEditTimeline && artifact.ArtifactID != "timeline_phase5_v1" {
				t.Fatalf("snapshot timeline is not the current master timeline: %#v", artifact)
			}
		}
		if _, updateErr := database.writer.Exec(ctx, `UPDATE drama.shot_videos SET is_current=false WHERE shot_video_id='video_phase5_1'`); updateErr != nil {
			t.Fatal(updateErr)
		}
		_, staleErr := database.BuildAuthoritativeQualityGateSnapshot(ctx, projectID, episodeID, "master_phase5_v1")
		if _, restoreErr := database.writer.Exec(ctx, `UPDATE drama.shot_videos SET is_current=true WHERE shot_video_id='video_phase5_1'`); restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if staleErr == nil || (!strings.Contains(staleErr.Error(), "stale or unapproved media") &&
			!strings.Contains(staleErr.Error(), "authoritative production snapshot is unavailable") &&
			!strings.Contains(staleErr.Error(), "EFFECTIVE_INPUTS_BLOCKED")) {
			t.Fatalf("stale timeline media entered authoritative QA snapshot: %v", staleErr)
		}
		if _, updateErr := database.writer.Exec(ctx, `UPDATE drama.episode_masters SET is_current=false WHERE master_id='master_phase5_v1'`); updateErr != nil {
			t.Fatal(updateErr)
		}
		_, nonCurrentErr := database.BuildAuthoritativeQualityGateSnapshot(ctx, projectID, episodeID, "master_phase5_v1")
		if _, restoreErr := database.writer.Exec(ctx, `UPDATE drama.episode_masters SET is_current=true WHERE master_id='master_phase5_v1'`); restoreErr != nil {
			t.Fatal(restoreErr)
		}
		if nonCurrentErr == nil || !strings.Contains(nonCurrentErr.Error(), "current approved timeline") {
			t.Fatalf("non-current master entered authoritative QA snapshot: %v", nonCurrentErr)
		}
	})

	t.Run("active Prompt binding changes new production input and rollback preserves history", func(t *testing.T) {
		template, createErr := database.CreatePromptTemplate(ctx, CreatePromptTemplateInput{
			Category: "script", PromptKey: "production.candidate.episode", DisplayName: "验收生产提示词", CreatedBy: "acceptance-owner",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		createVersion := func(marker, note string) PromptVersion {
			version, versionErr := database.CreatePromptVersion(ctx, template.PromptTemplateID, CreatePromptVersionInput{
				SystemTemplate:   "系统 " + marker,
				UserTemplate:     "目标 {{target_id}} " + marker,
				VariableSchema:   json.RawMessage(`{"type":"object","required":["target_id"],"properties":{"target_id":{"type":"string"}}}`),
				DefaultVariables: json.RawMessage(`{}`),
				ModelDefaults:    json.RawMessage(`{"provider":"acceptance-provider","model":"acceptance-model","temperature":0.25}`),
				ChangeNote:       note,
				CreatedBy:        "acceptance-owner",
			})
			if versionErr != nil {
				t.Fatal(versionErr)
			}
			if _, versionErr = database.ApprovePromptVersion(ctx, version.PromptVersionID, "acceptance-approver"); versionErr != nil {
				t.Fatal(versionErr)
			}
			if _, versionErr = database.PromotePromptVersion(ctx, version.PromptVersionID, "acceptance-promoter"); versionErr != nil {
				t.Fatal(versionErr)
			}
			return version
		}
		v1 := createVersion("PROMPT-V1", "创建 v1")
		historical := candidategeneration.Request{
			TargetType: "episode", TargetID: episodeID, GenerationParameters: json.RawMessage(`{}`),
		}
		if applyErr := database.applyProductionCandidatePrompt(ctx, &historical); applyErr != nil {
			t.Fatal(applyErr)
		}
		if historical.PromptVersion != v1.PromptVersionID || !strings.Contains(historical.ProductionPrompt, "PROMPT-V1") ||
			historical.GeneratorProvider != "acceptance-provider" || historical.GeneratorModel != "acceptance-model" {
			t.Fatalf("v1 binding did not enter production input: %#v", historical)
		}

		v2 := createVersion("PROMPT-V2", "创建 v2")
		fresh := candidategeneration.Request{
			TargetType: "episode", TargetID: episodeID, GenerationParameters: json.RawMessage(`{}`),
		}
		if applyErr := database.applyProductionCandidatePrompt(ctx, &fresh); applyErr != nil {
			t.Fatal(applyErr)
		}
		if fresh.PromptVersion != v2.PromptVersionID || !strings.Contains(fresh.ProductionPrompt, "PROMPT-V2") ||
			fresh.ProductionPrompt == historical.ProductionPrompt || historical.PromptVersion != v1.PromptVersionID {
			t.Fatalf("active switch was not isolated from historical input: historical=%#v fresh=%#v", historical, fresh)
		}

		if _, rollbackErr := database.PromotePromptVersion(ctx, v1.PromptVersionID, "acceptance-rollback"); rollbackErr != nil {
			t.Fatal(rollbackErr)
		}
		rolledBack := candidategeneration.Request{
			TargetType: "episode", TargetID: episodeID, GenerationParameters: json.RawMessage(`{}`),
		}
		if applyErr := database.applyProductionCandidatePrompt(ctx, &rolledBack); applyErr != nil {
			t.Fatal(applyErr)
		}
		if rolledBack.PromptVersion != v1.PromptVersionID || !strings.Contains(rolledBack.ProductionPrompt, "PROMPT-V1") {
			t.Fatalf("rollback did not restore v1: %#v", rolledBack)
		}
	})

	var legalDraftID string
	t.Run("NLE rejects bounds overlap subtitle and media duration but permits a real J-cut", func(t *testing.T) {
		var before int
		if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.edit_timelines WHERE project_id=$1 AND episode_id=$2`,
			projectID, episodeID).Scan(&before); err != nil {
			t.Fatal(err)
		}
		start, end := int64(9000), int64(13000)
		_, editErr := database.CreateNLEItemDraft(ctx, projectID, episodeID, "item_phase5_video_1",
			NLETimelineItemPatch{BaseTimelineID: "timeline_phase5_v1", TimelineStartMS: &start, TimelineEndMS: &end, Reason: "out-of-bounds"})
		if !errors.Is(editErr, ErrConflict) {
			t.Fatalf("out-of-bounds edit was accepted: %v", editErr)
		}
		start, end = 2000, 6000
		_, editErr = database.CreateNLEItemDraft(ctx, projectID, episodeID, "item_phase5_video_1",
			NLETimelineItemPatch{BaseTimelineID: "timeline_phase5_v1", TimelineStartMS: &start, TimelineEndMS: &end, Reason: "overlap"})
		if !errors.Is(editErr, ErrConflict) || !strings.Contains(editErr.Error(), "illegal overlap") {
			t.Fatalf("same-track overlap was accepted: %v", editErr)
		}
		sourceOut := int64(999999)
		_, editErr = database.CreateNLEItemDraft(ctx, projectID, episodeID, "item_phase5_video_1",
			NLETimelineItemPatch{BaseTimelineID: "timeline_phase5_v1", SourceOutMS: &sourceOut, Reason: "media-overrun"})
		if !errors.Is(editErr, ErrConflict) || !strings.Contains(editErr.Error(), "media duration") {
			t.Fatalf("source range beyond approved media was accepted: %v", editErr)
		}
		start, end = 0, 3000
		_, editErr = database.CreateNLEItemDraft(ctx, projectID, episodeID, "item_phase5_subtitle_1",
			NLETimelineItemPatch{BaseTimelineID: "timeline_phase5_v1", TimelineStartMS: &start, TimelineEndMS: &end, Reason: "subtitle-overrun"})
		if !errors.Is(editErr, ErrConflict) || !strings.Contains(editErr.Error(), "dialogue range") {
			t.Fatalf("subtitle outside dialogue was accepted: %v", editErr)
		}
		var afterInvalid int
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.edit_timelines WHERE project_id=$1 AND episode_id=$2`,
			projectID, episodeID).Scan(&afterInvalid)
		if afterInvalid != before {
			t.Fatalf("rejected edits left successor timelines: before=%d after=%d", before, afterInvalid)
		}

		end = 5500
		subtitleDraft, editErr := database.CreateNLEItemDraft(ctx, projectID, episodeID, "item_phase5_subtitle_2",
			NLETimelineItemPatch{BaseTimelineID: "timeline_phase5_v1", TimelineEndMS: &end, Reason: "prepare-j-cut"})
		if editErr != nil {
			t.Fatal(editErr)
		}
		dialogueItemID := ""
		subtitlePage, pageErr := database.GetNLETimelinePage(ctx, projectID, episodeID,
			subtitleDraft.Timeline.TimelineID, 0, 8000, 100, 0)
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		for _, item := range subtitlePage.Items {
			if item.TrackType == "dialogue" && item.EntityID == "dlg_phase5_2" {
				dialogueItemID = item.TimelineItemID
				break
			}
		}
		if dialogueItemID == "" {
			t.Fatal("dialogue successor item missing from subtitle draft")
		}
		start, end = 3900, 5600
		dialogueDraft, editErr := database.CreateNLEItemDraft(ctx, projectID, episodeID, dialogueItemID,
			NLETimelineItemPatch{BaseTimelineID: subtitleDraft.Timeline.TimelineID, TimelineStartMS: &start,
				TimelineEndMS: &end, Reason: "dialogue-j-cut"})
		if editErr != nil {
			t.Fatal(editErr)
		}
		legalDraftID = dialogueDraft.Timeline.TimelineID
		page, pageErr := database.GetNLETimelinePage(ctx, projectID, episodeID, legalDraftID, 3800, 5700, 100, 0)
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		found := false
		for _, item := range page.Items {
			if item.TrackType == "dialogue" && item.EntityID == "dlg_phase5_2" && item.TimelineStartMS == 3900 {
				found = true
			}
		}
		if !found {
			t.Fatalf("persisted J-cut is missing after reload: %#v", page.Items)
		}
	})

	t.Run("all professional formats build and round-trip from current versions", func(t *testing.T) {
		masterGate, gateErr := database.RunAuthoritativeQualityGate(ctx, projectID, episodeID,
			"master_phase5_v1", qualitygate.DefaultConfig(), false, "acceptance-export-gate")
		if gateErr != nil {
			t.Fatal(gateErr)
		}
		for _, finding := range masterGate.Findings {
			if finding.Severity == qualitygate.SeverityBlocking && finding.Status == qualitygate.FindingOpen {
				if _, gateErr = database.OverrideQualityGateFinding(ctx, projectID, episodeID, masterGate.GateRunID,
					finding.FindingID, "acceptance fixture risk reviewed for export", "acceptance-owner"); gateErr != nil {
					t.Fatal(gateErr)
				}
			}
		}
		if _, gateErr = database.ApproveQualityGateMaster(ctx, projectID, episodeID, masterGate.GateRunID, "acceptance-owner"); gateErr != nil {
			t.Fatal(gateErr)
		}
		selection := ProfessionalExportSelection{EpisodeID: episodeID, BundleVersion: 31,
			ScriptID: "script_phase5_post", StoryboardID: "storyboard_phase5_post", TimelineID: "timeline_phase5_v1",
			MasterID: "master_phase5_v1", StoryBibleID: "sb_phase1_legacy",
			SourceVersionID: "sv_legacy_novel_phase1_legacy", IRRevisionID: "ir_phase1_001",
			AdaptationSpecVersionID: "adaptation_spec_version_phase1_001"}
		staleSelection := selection
		staleSelection.BundleVersion = 32
		staleSelection.TimelineID = legalDraftID
		if _, staleErr := database.CreateProfessionalExport(ctx, projectID, CreateProfessionalExportInput{
			Formats: append([]string(nil), exportkit.Formats...), Selection: staleSelection, RequestedBy: "acceptance"}); staleErr == nil ||
			(!strings.Contains(staleErr.Error(), "EXPORT_STALE_BLOCKED") && !strings.Contains(staleErr.Error(), "EXPORT_VERSION_MISMATCH")) {
			t.Fatalf("stale timeline/master selection was accepted: %v", staleErr)
		}
		job, createErr := database.CreateProfessionalExport(ctx, projectID, CreateProfessionalExportInput{
			Formats: append([]string(nil), exportkit.Formats...), Selection: selection, RequestedBy: "acceptance"})
		if createErr != nil {
			t.Fatal(createErr)
		}
		snapshot, snapshotErr := database.BuildProfessionalExportSnapshot(ctx, job)
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		path := filepath.Join(t.TempDir(), "all-formats.zip")
		manifest, packageHash, buildErr := exportkit.BuildPackage(path, job.Formats, snapshot)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		if len(manifest.Files) < len(exportkit.Formats) || len(packageHash) != 64 {
			t.Fatalf("export manifest incomplete: files=%d hash=%s", len(manifest.Files), packageHash)
		}
		files := readExportArchive(t, path)
		assertExportRoundTrips(t, files)
		ready, completeErr := database.CompleteProfessionalExport(ctx, projectID, job.ExportID, path, packageHash, manifest)
		if completeErr != nil || ready.Status != "ready" {
			t.Fatalf("export did not become ready: %#v err=%v", ready, completeErr)
		}
		if _, invalidateErr := database.writer.Exec(ctx, `UPDATE drama.artifacts
			SET validity_status='stale',is_current=false WHERE artifact_id='artifact_phase5_timeline'`); invalidateErr != nil {
			t.Fatal(invalidateErr)
		}
		defer func() {
			_, _ = database.writer.Exec(ctx, `UPDATE drama.artifacts
				SET validity_status='valid',is_current=true WHERE artifact_id='artifact_phase5_timeline'`)
		}()
		var exportStatus, approvalStatus string
		if queryErr := database.pool.QueryRow(ctx, `SELECT job.status,approval.status
			FROM drama.professional_export_jobs job
			JOIN drama.quality_gate_master_approvals approval ON approval.gate_approval_id=job.gate_approval_id
			WHERE job.export_id=$1`, job.ExportID).Scan(&exportStatus, &approvalStatus); queryErr != nil {
			t.Fatal(queryErr)
		}
		if exportStatus != "stale" || approvalStatus != "revoked" {
			t.Fatalf("artifact invalidation did not revoke delivery: export=%s approval=%s", exportStatus, approvalStatus)
		}
		var projectedStage string
		if queryErr := database.pool.QueryRow(ctx, `SELECT current_stage FROM drama.projects WHERE project_id=$1`, projectID).Scan(&projectedStage); queryErr != nil {
			t.Fatal(queryErr)
		}
		if projectedStage == "stage_5_completed" || projectedStage == "qc_completed" {
			t.Fatalf("project delivery projection remained falsely completed after invalidation: %s", projectedStage)
		}
		if downloadErr := database.ValidateProfessionalExportReady(ctx, projectID, job.ExportID); downloadErr == nil ||
			!strings.Contains(downloadErr.Error(), "EXPORT_STALE_BLOCKED") {
			t.Fatalf("invalidated export remained downloadable: %v", downloadErr)
		}
	})

	t.Run("blocking QA prevents API and direct DB render until audited override", func(t *testing.T) {
		snapshot, buildErr := database.BuildAuthoritativeQualityGateSnapshot(ctx, projectID, episodeID, "master_phase5_v1")
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		snapshot.Artifacts[0].Facts = []qualitygate.Fact{{Key: "identity", Value: "林夏", Critical: true,
			RequiredStages: []qualitygate.Stage{qualitygate.StageAdaptationPlan}}}
		snapshot.Artifacts[1].Facts = []qualitygate.Fact{{Key: "identity", Value: "陌生人"}}
		run, evaluateErr := qualitygate.EvaluateRules(snapshot, qualitygate.DefaultConfig(), false)
		if evaluateErr != nil {
			t.Fatal(evaluateErr)
		}
		record, saveErr := database.SaveQualityGateRuleRun(ctx, snapshot, run, "acceptance")
		if saveErr != nil || len(record.Findings) == 0 || record.Findings[0].Severity != qualitygate.SeverityBlocking {
			t.Fatalf("blocking gate was not persisted: %#v err=%v", record, saveErr)
		}
		var jobsBefore int
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.render_jobs WHERE timeline_id=$1`, legalDraftID).Scan(&jobsBefore)
		if _, renderErr := database.ConfirmNLETimelineRender(ctx, projectID, episodeID, legalDraftID); !errors.Is(renderErr, ErrConflict) ||
			!strings.Contains(renderErr.Error(), "QUALITY_GATE_TARGET_MISMATCH") {
			t.Fatalf("old-master gate released a new timeline: %v", renderErr)
		}
		_, directErr := database.writer.Exec(ctx, `INSERT INTO drama.render_jobs(render_job_id,idempotency_key,trace_id,
			project_id,episode_id,timeline_id,timeline_version,render_type,status,command_template_id,input_manifest_path,output_path)
			VALUES('rj_phase31_bypass','phase31-bypass','trace-phase31-bypass',$1,$2,$3,3,'preview','pending','test','/tmp/input','/tmp/output')`,
			projectID, episodeID, legalDraftID)
		if directErr == nil || !strings.Contains(directErr.Error(), "QUALITY_GATE_TARGET_MISMATCH") {
			t.Fatalf("direct render insert accepted an old-master gate: %v", directErr)
		}
		targetSnapshot, targetErr := database.BuildAuthoritativeTimelineQualityGateSnapshot(ctx, projectID, episodeID, legalDraftID)
		if targetErr != nil {
			t.Fatal(targetErr)
		}
		targetSnapshot.Artifacts[0].Facts = snapshot.Artifacts[0].Facts
		targetSnapshot.Artifacts[1].Facts = snapshot.Artifacts[1].Facts
		targetRun, targetErr := qualitygate.EvaluateRules(targetSnapshot, qualitygate.DefaultConfig(), false)
		if targetErr != nil {
			t.Fatal(targetErr)
		}
		targetRecord, targetErr := database.SaveQualityGateRuleRun(ctx, targetSnapshot, targetRun, "acceptance-target")
		if targetErr != nil {
			t.Fatal(targetErr)
		}
		if _, renderErr := database.ConfirmNLETimelineRender(ctx, projectID, episodeID, legalDraftID); !errors.Is(renderErr, ErrConflict) ||
			!strings.Contains(renderErr.Error(), "QUALITY_GATE_BLOCKED") {
			t.Fatalf("target blocking gate did not block render: %v", renderErr)
		}
		var jobsAfter int
		_ = database.pool.QueryRow(ctx, `SELECT count(*) FROM drama.render_jobs WHERE timeline_id=$1`, legalDraftID).Scan(&jobsAfter)
		if jobsAfter != jobsBefore {
			t.Fatalf("blocked render left a task: before=%d after=%d", jobsBefore, jobsAfter)
		}
		for _, finding := range targetRecord.Findings {
			if finding.Severity != qualitygate.SeverityBlocking || finding.Status != qualitygate.FindingOpen {
				continue
			}
			if _, overrideErr := database.OverrideQualityGateFinding(ctx, projectID, episodeID, targetRecord.GateRunID,
				finding.FindingID, "负责人接受本次已定位风险", "acceptance-owner"); overrideErr != nil {
				t.Fatal(overrideErr)
			}
		}
		job, renderErr := database.ConfirmNLETimelineRender(ctx, projectID, episodeID, legalDraftID)
		if renderErr != nil || job.Status != "pending" {
			t.Fatalf("audited override did not release render: %#v err=%v", job, renderErr)
		}
		masterID := "master_phase32_" + strings.TrimPrefix(job.RenderJobID, "rj_")
		if _, persistErr := database.writer.Exec(ctx, `INSERT INTO drama.episode_masters(
			master_id,project_id,episode_id,timeline_id,render_job_id,generation_version,master_type,
			storage_url,width,height,aspect_ratio,fps,duration_ms,video_codec,audio_codec,sample_rate,
			content_hash,status,is_current) VALUES($1,$2,$3,$4,$5,1,'preview',$6,1080,1920,'9:16',24,8000,
			'h264','aac',48000,$7,'ready',true)`, masterID, projectID, episodeID, legalDraftID,
			job.RenderJobID, "/results/"+masterID+".mp4", strings.Repeat("a", 64)); persistErr != nil {
			t.Fatal(persistErr)
		}
		if _, persistErr := database.writer.Exec(ctx, `UPDATE drama.render_jobs
			SET status='succeeded',progress=100,output_url=$2,completed_at=now(),updated_at=now()
			WHERE render_job_id=$1`, job.RenderJobID, "/results/"+masterID+".mp4"); persistErr != nil {
			t.Fatal(persistErr)
		}
		var currentTimeline, currentMaster string
		if queryErr := database.pool.QueryRow(ctx, `SELECT
			(SELECT artifact.native_entity_id FROM drama.artifact_current_bindings binding
			 JOIN drama.artifacts artifact ON artifact.artifact_id=binding.current_artifact_id
			 WHERE binding.project_id=$1 AND binding.target_type='episode' AND binding.target_id=$2
			   AND binding.component_scope='edit_timeline'),
			(SELECT artifact.native_entity_id FROM drama.artifact_current_bindings binding
			 JOIN drama.artifacts artifact ON artifact.artifact_id=binding.current_artifact_id
			 WHERE binding.project_id=$1 AND binding.target_type='episode' AND binding.target_id=$2
			   AND binding.component_scope='episode_master')`, projectID, episodeID).Scan(&currentTimeline, &currentMaster); queryErr != nil {
			t.Fatal(queryErr)
		}
		if currentTimeline != legalDraftID || currentMaster != masterID {
			t.Fatalf("successful render did not publish current artifacts: timeline=%s master=%s", currentTimeline, currentMaster)
		}

		// A later generation can be byte-identical (for example, deterministic
		// FFmpeg input after an upstream metadata-only change). It must publish a
		// distinct version artifact instead of colliding through a hash-based
		// artifact ID and leaving a stale artifact bound as current.
		var firstMasterArtifactID string
		if queryErr := database.pool.QueryRow(ctx, `SELECT artifact_id FROM drama.artifacts
			WHERE artifact_type='episode_master' AND native_entity_id=$1`, masterID).
			Scan(&firstMasterArtifactID); queryErr != nil {
			t.Fatal(queryErr)
		}
		restored, restoreErr := database.RestoreNLETimelineDraft(ctx, projectID, episodeID, legalDraftID, nil)
		if restoreErr != nil || restored.Timeline.ParentTimelineID == nil ||
			*restored.Timeline.ParentTimelineID != legalDraftID || restored.Timeline.IsCurrent {
			t.Fatalf("identical-content successor draft was not created: %#v err=%v", restored, restoreErr)
		}
		secondTimelineID := restored.Timeline.TimelineID
		sameHashGate, gateErr := database.RunAuthoritativeTimelineQualityGate(ctx, projectID, episodeID,
			secondTimelineID, qualitygate.DefaultConfig(), false, "acceptance-identical-master")
		if gateErr != nil {
			t.Fatal(gateErr)
		}
		resolveBlockingFindings(t, ctx, database, projectID, episodeID, sameHashGate)
		secondJob, secondRenderErr := database.ConfirmNLETimelineRender(ctx, projectID, episodeID, secondTimelineID)
		if secondRenderErr != nil || secondJob.Status != "pending" {
			t.Fatalf("identical-content successor render was not released: %#v err=%v", secondJob, secondRenderErr)
		}
		secondRenderID := secondJob.RenderJobID
		var nextGeneration int
		if queryErr := database.pool.QueryRow(ctx, `SELECT COALESCE(max(generation_version),0)+1
			FROM drama.episode_masters WHERE episode_id=$1`, episodeID).Scan(&nextGeneration); queryErr != nil {
			t.Fatal(queryErr)
		}
		if _, persistErr := database.writer.Exec(ctx, `UPDATE drama.episode_masters SET is_current=false
			WHERE project_id=$1 AND episode_id=$2 AND master_type='preview' AND is_current`, projectID, episodeID); persistErr != nil {
			t.Fatal(persistErr)
		}
		const secondMasterID = "master_phase34_identical"
		if _, persistErr := database.writer.Exec(ctx, `INSERT INTO drama.episode_masters(
			master_id,project_id,episode_id,timeline_id,render_job_id,generation_version,master_type,
			storage_url,width,height,aspect_ratio,fps,duration_ms,video_codec,audio_codec,sample_rate,
			content_hash,status,is_current) VALUES($1,$2,$3,$4,$5,$6,'preview',$7,1080,1920,'9:16',24,8000,
			'h264','aac',48000,$8,'ready',true)`, secondMasterID, projectID, episodeID, secondTimelineID,
			secondRenderID, nextGeneration, "/results/master_phase34_identical.mp4", strings.Repeat("a", 64)); persistErr != nil {
			t.Fatal(persistErr)
		}
		if _, persistErr := database.writer.Exec(ctx, `UPDATE drama.render_jobs
			SET status='succeeded',progress=100,output_url=$2,completed_at=now(),updated_at=now()
			WHERE render_job_id=$1`, secondRenderID, "/results/master_phase34_identical.mp4"); persistErr != nil {
			t.Fatalf("identical-content successor publication failed: %v", persistErr)
		}
		var secondMasterArtifactID, firstValidity, secondValidity, currentMasterArtifactID, renderStatus string
		var firstCurrent, secondCurrent bool
		if queryErr := database.pool.QueryRow(ctx, `SELECT artifact_id,validity_status,is_current
			FROM drama.artifacts WHERE artifact_type='episode_master' AND native_entity_id=$1`, secondMasterID).
			Scan(&secondMasterArtifactID, &secondValidity, &secondCurrent); queryErr != nil {
			t.Fatal(queryErr)
		}
		if queryErr := database.pool.QueryRow(ctx, `SELECT validity_status,is_current
			FROM drama.artifacts WHERE artifact_id=$1`, firstMasterArtifactID).Scan(&firstValidity, &firstCurrent); queryErr != nil {
			t.Fatal(queryErr)
		}
		if queryErr := database.pool.QueryRow(ctx, `SELECT binding.current_artifact_id
			FROM drama.artifact_current_bindings binding
			WHERE binding.project_id=$1 AND binding.target_type='episode' AND binding.target_id=$2
			  AND binding.component_scope='episode_master'`, projectID, episodeID).
			Scan(&currentMasterArtifactID); queryErr != nil {
			t.Fatal(queryErr)
		}
		if queryErr := database.pool.QueryRow(ctx, `SELECT status FROM drama.render_jobs WHERE render_job_id=$1`,
			secondRenderID).Scan(&renderStatus); queryErr != nil {
			t.Fatal(queryErr)
		}
		if secondMasterArtifactID == firstMasterArtifactID || secondValidity != "valid" || !secondCurrent ||
			firstValidity != "superseded" || firstCurrent || currentMasterArtifactID != secondMasterArtifactID ||
			renderStatus != "succeeded" {
			t.Fatalf("identical-content successor did not switch atomically: first=%s/%s/%t second=%s/%s/%t binding=%s render=%s",
				firstMasterArtifactID, firstValidity, firstCurrent, secondMasterArtifactID, secondValidity,
				secondCurrent, currentMasterArtifactID, renderStatus)
		}
	})
}

func readExportArchive(t *testing.T, path string) map[string][]byte {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	files := map[string][]byte{}
	for _, file := range archive.File {
		reader, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		content, readErr := io.ReadAll(reader)
		reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(content) == 0 {
			t.Fatalf("export file is empty: %s", file.Name)
		}
		files[file.Name] = content
	}
	return files
}

func assertExportRoundTrips(t *testing.T, files map[string][]byte) {
	t.Helper()
	requireContaining := func(suffix string) (string, []byte) {
		for name, content := range files {
			if strings.HasSuffix(name, suffix) {
				return name, content
			}
		}
		t.Fatalf("missing export suffix %s; files=%v", suffix, exportFileNames(files))
		return "", nil
	}
	_, docx := requireContaining(".docx")
	inner, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil || len(inner.File) < 3 {
		t.Fatalf("DOCX cannot be reopened: parts=%d err=%v", len(inner.File), err)
	}
	_, fountain := requireContaining(".fountain")
	if !strings.Contains(string(fountain), "INT.") && !strings.Contains(string(fountain), "EXT.") &&
		!strings.Contains(string(fountain), "内.") && !strings.Contains(string(fountain), "外.") {
		t.Fatalf("Fountain has no scene heading: %s", fountain)
	}
	for _, suffix := range []string{"镜头表.csv", "镜头提示词.csv"} {
		_, content := requireContaining(suffix)
		content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
		rows, csvErr := csv.NewReader(bytes.NewReader(content)).ReadAll()
		if csvErr != nil || len(rows) < 2 {
			t.Fatalf("CSV cannot be parsed or lacks data: %s rows=%d err=%v", suffix, len(rows), csvErr)
		}
	}
	_, srt := requireContaining(".srt")
	if !strings.Contains(string(srt), " --> ") {
		t.Fatalf("SRT has no timecode: %s", srt)
	}
	_, ass := requireContaining(".ass")
	if !strings.Contains(string(ass), "[Events]") || !strings.Contains(string(ass), "Dialogue:") {
		t.Fatalf("ASS parser markers missing: %s", ass)
	}
	_, edl := requireContaining(".edl")
	if !strings.Contains(string(edl), "TITLE:") || !strings.Contains(string(edl), "FROM CLIP NAME") {
		t.Fatalf("EDL has no events: %s", edl)
	}
	_, timelineXML := requireContaining("时间线.xml")
	var parsedXML struct {
		XMLName xml.Name
	}
	if err = xml.Unmarshal(timelineXML, &parsedXML); err != nil {
		t.Fatalf("timeline XML cannot be parsed: %v", err)
	}
	for name, content := range files {
		if strings.HasSuffix(name, ".json") {
			var value any
			if json.Unmarshal(content, &value) != nil {
				t.Fatalf("JSON cannot be parsed: %s", name)
			}
		}
		if strings.HasSuffix(name, ".m3u8") && !strings.HasPrefix(string(content), "#EXTM3U") {
			t.Fatalf("stem playlist cannot be parsed: %s", name)
		}
	}
	if _, ok := files["manifest.json"]; !ok {
		t.Fatal("provenance manifest missing")
	}
}

func exportFileNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	return names
}
