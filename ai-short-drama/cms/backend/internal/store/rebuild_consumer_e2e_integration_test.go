package store

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"short-drama-cms/backend/internal/exportkit"
	"short-drama-cms/backend/internal/localedit"
	"short-drama-cms/backend/internal/qualitygate"
)

// TestRebuildConsumerDeliveryClosureIntegration deliberately depends on an
// external media worker pointed at this isolated database. The worker must
// claim the tasks itself; this test never edits rebuild/render task status.
func TestRebuildConsumerDeliveryClosureIntegration(t *testing.T) {
	databaseURL := os.Getenv("REBUILD_CLOSURE_DATABASE_URL")
	storageDirectory := os.Getenv("REBUILD_CLOSURE_STORAGE_DIR")
	if databaseURL == "" || storageDirectory == "" {
		t.Skip("REBUILD_CLOSURE_DATABASE_URL and REBUILD_CLOSURE_STORAGE_DIR are not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	const projectID, episodeID = "p_phase1_legacy", "ep_phase1_legacy_001"
	for name, durationMS := range map[string]int{
		"bgm-suspense.wav": 8000,
		"old-house.wav":    8000,
		"door-creak.wav":   1200,
	} {
		writeSilentPCMFixture(t, filepath.Join(storageDirectory, "sound", name), durationMS)
	}

	oldExportID := ""
	if err = database.pool.QueryRow(ctx, `SELECT export_id FROM drama.professional_export_jobs
		WHERE project_id=$1 AND status='ready' ORDER BY created_at LIMIT 1`, projectID).Scan(&oldExportID); errors.Is(err, pgx.ErrNoRows) {
		oldExportID = createBaselineExport(t, ctx, database, storageDirectory, projectID, episodeID)
	} else if err != nil {
		t.Fatal(err)
	}
	oldTimelineGate := ""
	if err = database.pool.QueryRow(ctx, `SELECT gate_run_id FROM drama.quality_gate_runs
		WHERE project_id=$1 AND target_timeline_id='timeline_phase5_v1'
		  AND status<>'superseded' ORDER BY created_at DESC LIMIT 1`, projectID).Scan(&oldTimelineGate); errors.Is(err, pgx.ErrNoRows) {
		baselineTimelineGate, gateErr := database.RunAuthoritativeTimelineQualityGate(ctx, projectID, episodeID,
			"timeline_phase5_v1", qualitygate.DefaultConfig(), false, "rebuild-closure-baseline")
		if gateErr != nil {
			t.Fatal(gateErr)
		}
		resolveBlockingFindings(t, ctx, database, projectID, episodeID, baselineTimelineGate)
		oldTimelineGate = baselineTimelineGate.GateRunID
	} else if err != nil {
		t.Fatal(err)
	}
	var unrelatedBefore string
	if err = database.pool.QueryRow(ctx, `SELECT content_hash||':'||validity_status||':'||is_current
		FROM drama.artifacts WHERE artifact_id='artifact_phase5_sound_bgm'`).Scan(&unrelatedBefore); err != nil {
		t.Fatal(err)
	}

	plan, err := localedit.Build(localedit.Request{
		Instruction: "change the approved adaptation plan and rebuild its produced episode",
		Target:      localedit.Target{EntityType: "adaptation_plan", EntityID: "adaptation_plan_phase1_001", Version: 1},
		Changes:     []localedit.Change{{Operation: "replace", Field: "strategy_label", Value: "rebuild closure v2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	change, err := database.CreateChangePlan(ctx, projectID, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Impacts) != 12 {
		t.Fatalf("impact analysis selected %d artifacts, want 11 rebuildable outputs plus the derived master", len(change.Impacts))
	}
	for _, impact := range change.Impacts {
		if impact.ArtifactType == "episode_master" && impact.PropagationDepth != 2 {
			t.Fatalf("master was not the only derived depth-2 impact: %#v", impact)
		}
	}
	change, err = database.ConfirmChangePlan(ctx, projectID, change.ChangePlanID, nil)
	if err == nil {
		change, err = database.ExecuteChangePlan(ctx, projectID, change.ChangePlanID)
	}
	if err != nil {
		t.Fatal(err)
	}
	if change.Status != "applied" || len(change.RebuildTasks) != 11 {
		t.Fatalf("change plan did not create the exact rebuild queue: %#v", change)
	}
	expected := map[string]int{"regenerate_voice": 2, "update_subtitle": 2, "regenerate_image": 2,
		"regenerate_video": 2, "update_continuity": 2, "recompose_timeline": 1}
	for _, task := range change.RebuildTasks {
		expected[task.Action]--
		if task.Status != "pending" || task.Provider != "local_conformance" || task.ArtifactID == nil {
			t.Fatalf("task was not a real provider-backed pending task: %#v", task)
		}
	}
	for action, remaining := range expected {
		if remaining != 0 {
			t.Fatalf("wrong target count for %s: delta=%d", action, remaining)
		}
	}

	waitForCount(t, ctx, database, 90*time.Second, `SELECT count(*) FROM drama.incremental_rebuild_tasks
		WHERE change_plan_id=$1 AND status='succeeded'`, []any{change.ChangePlanID}, 11, "rebuild tasks")
	var publishedCount, physicalCount int
	if err = database.pool.QueryRow(ctx, `SELECT
		count(*) FROM drama.rebuild_publications publication
		JOIN drama.incremental_rebuild_tasks task USING(rebuild_task_id)
		WHERE task.change_plan_id=$1`, change.ChangePlanID).
		Scan(&publishedCount); err != nil {
		t.Fatal(err)
	}
	if publishedCount != 11 {
		t.Fatalf("only %d/11 tasks atomically published successors", publishedCount)
	}
	rows, err := database.pool.Query(ctx, `SELECT output->'artifact'->>'storage_path',
		output->'artifact'->>'content_hash' FROM drama.incremental_rebuild_tasks
		WHERE change_plan_id=$1 ORDER BY rebuild_task_id`, change.ChangePlanID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var path, expectedHash string
		if err = rows.Scan(&path, &expectedHash); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		content, readErr := os.ReadFile(hostStoragePath(storageDirectory, path))
		if readErr != nil {
			rows.Close()
			t.Fatal(readErr)
		}
		hash := sha256.Sum256(content)
		if hex.EncodeToString(hash[:]) != expectedHash {
			rows.Close()
			t.Fatalf("physical output hash mismatch: %s", path)
		}
		physicalCount++
	}
	rows.Close()
	if err = rows.Err(); err != nil || physicalCount != 11 {
		t.Fatalf("physical provider outputs=%d err=%v", physicalCount, err)
	}

	var oldGateStatus, oldExportStatus, unrelatedAfter string
	if err = database.pool.QueryRow(ctx, `SELECT status FROM drama.quality_gate_runs WHERE gate_run_id=$1`, oldTimelineGate).Scan(&oldGateStatus); err != nil {
		t.Fatal(err)
	}
	if err = database.pool.QueryRow(ctx, `SELECT status FROM drama.professional_export_jobs WHERE export_id=$1`, oldExportID).Scan(&oldExportStatus); err != nil {
		t.Fatal(err)
	}
	if err = database.pool.QueryRow(ctx, `SELECT content_hash||':'||validity_status||':'||is_current
		FROM drama.artifacts WHERE artifact_id='artifact_phase5_sound_bgm'`).Scan(&unrelatedAfter); err != nil {
		t.Fatal(err)
	}
	if oldGateStatus != "superseded" || oldExportStatus != "stale" || unrelatedAfter != unrelatedBefore {
		t.Fatalf("invalidation scope wrong: gate=%s export=%s unrelated=%s/%s",
			oldGateStatus, oldExportStatus, unrelatedBefore, unrelatedAfter)
	}
	if downloadErr := database.ValidateProfessionalExportReady(ctx, projectID, oldExportID); downloadErr == nil ||
		!strings.Contains(downloadErr.Error(), "EXPORT_STALE_BLOCKED") {
		t.Fatalf("old export remained downloadable: %v", downloadErr)
	}

	resolution, err := database.ResolveEffectiveInputs(ctx, projectID, episodeID, "post_production")
	if err != nil || !strings.Contains(string(resolution), `"status": "ready"`) &&
		!strings.Contains(string(resolution), `"status":"ready"`) {
		t.Fatalf("resolver did not return the new current chain: %s err=%v", resolution, err)
	}
	var timelineID string
	if err = database.pool.QueryRow(ctx, `SELECT timeline_id FROM drama.edit_timelines
		WHERE project_id=$1 AND episode_id=$2 AND is_current`, projectID, episodeID).Scan(&timelineID); err != nil {
		t.Fatal(err)
	}
	if timelineID == "timeline_phase5_v1" {
		t.Fatal("resolver/native current still points to the old timeline")
	}
	run, err := database.RunAuthoritativeTimelineQualityGate(ctx, projectID, episodeID, timelineID,
		qualitygate.DefaultConfig(), false, "rebuild-closure")
	if err != nil {
		t.Fatal(err)
	}
	resolveBlockingFindings(t, ctx, database, projectID, episodeID, run)
	render, err := database.ConfirmNLETimelineRender(ctx, projectID, episodeID, timelineID, storageDirectory)
	if err != nil {
		t.Fatal(err)
	}
	waitForCount(t, ctx, database, 90*time.Second, `SELECT count(*) FROM drama.render_jobs
		WHERE render_job_id=$1 AND status='succeeded'`, []any{render.RenderJobID}, 1, "render")
	var masterID, masterPath, masterHash string
	if err = database.pool.QueryRow(ctx, `SELECT master_id,local_path,content_hash FROM drama.episode_masters
		WHERE render_job_id=$1 AND status='ready' AND is_current`, render.RenderJobID).
		Scan(&masterID, &masterPath, &masterHash); err != nil {
		t.Fatal(err)
	}
	masterBytes, err := os.ReadFile(hostStoragePath(storageDirectory, masterPath))
	if err != nil {
		t.Fatal(err)
	}
	actualMasterHash := sha256.Sum256(masterBytes)
	if hex.EncodeToString(actualMasterHash[:]) != masterHash || len(masterBytes) < 1024 {
		t.Fatal("new master file/hash is invalid")
	}
	masterGate, err := database.RunAuthoritativeQualityGate(ctx, projectID, episodeID, masterID,
		qualitygate.DefaultConfig(), false, "rebuild-closure-master")
	if err != nil {
		t.Fatal(err)
	}
	resolveBlockingFindings(t, ctx, database, projectID, episodeID, masterGate)
	if _, err = database.ApproveQualityGateMaster(ctx, projectID, episodeID, masterGate.GateRunID, "rebuild-closure"); err != nil {
		t.Fatal(err)
	}

	selection := ProfessionalExportSelection{EpisodeID: episodeID, BundleVersion: 33,
		ScriptID: "script_phase5_post", StoryboardID: "storyboard_phase5_post", TimelineID: timelineID,
		MasterID: masterID, StoryBibleID: "sb_phase1_legacy", SourceVersionID: "sv_legacy_novel_phase1_legacy",
		IRRevisionID: "ir_phase1_001", AdaptationSpecVersionID: "adaptation_spec_version_phase1_001"}
	exportJob, err := database.CreateProfessionalExport(ctx, projectID, CreateProfessionalExportInput{
		Formats: append([]string(nil), exportkit.Formats...), Selection: selection, RequestedBy: "rebuild-closure"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := database.BuildProfessionalExportSnapshot(ctx, exportJob)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(snapshot.Traceability), change.ChangePlanID) ||
		strings.Count(string(snapshot.Traceability), `"execution_mode":"local_conformance"`) != 11 {
		t.Fatalf("export traceability omitted rebuild provenance: %s", snapshot.Traceability)
	}
	packagePath := filepath.Join(storageDirectory, "exports", exportJob.ExportID+".zip")
	manifest, packageHash, err := exportkit.BuildPackage(packagePath, exportJob.Formats, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := database.CompleteProfessionalExport(ctx, projectID, exportJob.ExportID, packagePath, packageHash, manifest)
	if err != nil || ready.Status != "ready" {
		t.Fatalf("new export was not ready: %#v err=%v", ready, err)
	}
	archive, err := zip.OpenReader(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		reader, openErr := file.Open()
		if openErr != nil {
			archive.Close()
			t.Fatal(openErr)
		}
		_, openErr = io.Copy(io.Discard, reader)
		reader.Close()
		if openErr != nil {
			archive.Close()
			t.Fatal(openErr)
		}
	}
	archive.Close()
	if len(archive.File) < len(exportkit.Formats) || database.ValidateProfessionalExportReady(ctx, projectID, exportJob.ExportID) != nil {
		t.Fatal("new export archive did not round-trip as the current deliverable")
	}
	var sourcePlanCount, localConformanceCount, renderDependencyCount int
	if err = database.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM drama.change_plans WHERE change_plan_id=$1 AND status='applied'),
		(SELECT count(*) FROM drama.rebuild_publications WHERE provenance->>'execution_mode'='local_conformance'
		  AND rebuild_task_id IN(SELECT rebuild_task_id FROM drama.incremental_rebuild_tasks WHERE change_plan_id=$1)),
		(SELECT count(*) FROM drama.artifact_dependencies dependency
		  JOIN drama.artifacts master ON master.artifact_id=dependency.downstream_artifact_id
		  WHERE master.native_entity_id=$2)`, change.ChangePlanID, masterID).
		Scan(&sourcePlanCount, &localConformanceCount, &renderDependencyCount); err != nil {
		t.Fatal(err)
	}
	if sourcePlanCount != 1 || localConformanceCount != 11 || renderDependencyCount == 0 {
		t.Fatalf("provenance did not cross change/provider/render/export: plan=%d provider=%d render=%d",
			sourcePlanCount, localConformanceCount, renderDependencyCount)
	}
	t.Logf("REBUILD_CLOSURE_EVIDENCE change_plan=%s tasks=11 physical_outputs=%d publications=%d timeline=%s timeline_gate=%s render=%s master=%s master_gate=%s export=%s package=%s old_export=%s/%s old_gate=%s/%s provider=local_conformance",
		change.ChangePlanID, physicalCount, publishedCount, timelineID, run.GateRunID, render.RenderJobID,
		masterID, masterGate.GateRunID, exportJob.ExportID, packagePath, oldExportID, oldExportStatus,
		oldTimelineGate, oldGateStatus)
}

func waitForCount(t *testing.T, ctx context.Context, database *Store, timeout time.Duration,
	query string, args []any, want int, label string,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var count int
		if err := database.pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == want {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	var states []string
	rows, _ := database.pool.Query(ctx, `SELECT status||':'||COALESCE(error_code,'')||':'||COALESCE(error_message,'')
		FROM drama.incremental_rebuild_tasks ORDER BY created_at`)
	if rows != nil {
		for rows.Next() {
			var state string
			_ = rows.Scan(&state)
			states = append(states, state)
		}
		rows.Close()
	}
	sort.Strings(states)
	t.Fatalf("timed out waiting for %s=%d: %s", label, want, strings.Join(states, " | "))
}

func createBaselineExport(t *testing.T, ctx context.Context, database *Store, storageDirectory,
	projectID, episodeID string,
) string {
	t.Helper()
	gate, err := database.RunAuthoritativeQualityGate(ctx, projectID, episodeID,
		"master_phase5_v1", qualitygate.DefaultConfig(), false, "rebuild-closure-baseline-master")
	if err != nil {
		t.Fatal(err)
	}
	resolveBlockingFindings(t, ctx, database, projectID, episodeID, gate)
	if _, err = database.ApproveQualityGateMaster(ctx, projectID, episodeID, gate.GateRunID,
		"rebuild-closure-baseline"); err != nil {
		t.Fatal(err)
	}
	selection := ProfessionalExportSelection{EpisodeID: episodeID, BundleVersion: 32,
		ScriptID: "script_phase5_post", StoryboardID: "storyboard_phase5_post", TimelineID: "timeline_phase5_v1",
		MasterID: "master_phase5_v1", StoryBibleID: "sb_phase1_legacy",
		SourceVersionID: "sv_legacy_novel_phase1_legacy", IRRevisionID: "ir_phase1_001",
		AdaptationSpecVersionID: "adaptation_spec_version_phase1_001"}
	job, err := database.CreateProfessionalExport(ctx, projectID, CreateProfessionalExportInput{
		Formats: append([]string(nil), exportkit.Formats...), Selection: selection, RequestedBy: "rebuild-closure-baseline"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := database.BuildProfessionalExportSnapshot(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(storageDirectory, "baseline", job.ExportID+".zip")
	manifest, packageHash, err := exportkit.BuildPackage(packagePath, job.Formats, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.CompleteProfessionalExport(ctx, projectID, job.ExportID, packagePath, packageHash, manifest); err != nil {
		t.Fatal(err)
	}
	return job.ExportID
}

func resolveBlockingFindings(t *testing.T, ctx context.Context, database *Store,
	projectID, episodeID string, run QualityGateRecord,
) {
	t.Helper()
	for _, finding := range run.Findings {
		if finding.Severity != qualitygate.SeverityBlocking || finding.Status != qualitygate.FindingOpen {
			continue
		}
		if _, err := database.OverrideQualityGateFinding(ctx, projectID, episodeID, run.GateRunID,
			finding.FindingID, "isolated conformance output reviewed", "rebuild-closure"); err != nil {
			t.Fatal(err)
		}
	}
}

func hostStoragePath(storageDirectory, workerPath string) string {
	normalized := strings.ReplaceAll(workerPath, "\\", "/")
	if strings.HasPrefix(normalized, "/data/storage/") {
		return filepath.Join(storageDirectory, filepath.FromSlash(strings.TrimPrefix(normalized, "/data/storage/")))
	}
	return workerPath
}

func writeSilentPCMFixture(t *testing.T, path string, durationMS int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	const sampleRate, channels, bitsPerSample = 8000, 1, 16
	dataSize := uint32(durationMS * sampleRate / 1000 * channels * bitsPerSample / 8)
	values := []any{
		[]byte("RIFF"), uint32(36) + dataSize, []byte("WAVE"), []byte("fmt "), uint32(16),
		uint16(1), uint16(channels), uint32(sampleRate), uint32(sampleRate * channels * bitsPerSample / 8),
		uint16(channels * bitsPerSample / 8), uint16(bitsPerSample), []byte("data"), dataSize,
	}
	for _, value := range values {
		if err = binary.Write(file, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = io.CopyN(file, zeroReader{}, int64(dataSize)); err != nil {
		t.Fatal(err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 0
	}
	return len(buffer), nil
}
