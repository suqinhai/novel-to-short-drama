package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"short-drama-cms/backend/internal/candidategeneration"
)

func TestPhase13MockE2EAndSelectiveBeatInvalidation(t *testing.T) {
	databaseURL := os.Getenv("PHASE2_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PHASE2_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	suffix, _ := newPublicID("")
	work, _, err := database.CreateSourceWork(ctx, "phase13-work-"+suffix,
		CreateSourceWorkInput{Title: "林晚账册疑案", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	version, _, err := database.CreateSourceVersion(ctx, work.WorkID, "phase13-version-"+suffix,
		CreateSourceVersionInput{NormalizationVersion: "phase13-test-v1", Metadata: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, revision, err := database.ApplyImport(ctx, version.SourceVersionID, 1, "phase13-import-"+suffix,
		ImportInput{Mode: "batch_chapters", Items: []ChapterInput{
			{ClientItemKey: "c1", Ordinal: 1, Title: "暗箭", Content: "林晚被追杀，她却发现父亲留下的账册，幕后身份即将揭开。"},
			{ClientItemKey: "c2", Ordinal: 2, Title: "反击", Content: "众人嘲讽她无能。林晚拿出密信当众打脸，对手突然承认背叛。"},
			{ClientItemKey: "c3", Ordinal: 3, Title: "门外", Content: "她要救出弟弟，代价是失去信任。门外敲响三声，真正主谋出现。"},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = database.PublishSourceVersion(ctx, version.SourceVersionID, revision, "phase13-publish-"+suffix); err != nil {
		t.Fatal(err)
	}
	chapters, err := database.ListVersionChapters(ctx, version.SourceVersionID)
	if err != nil || len(chapters) != 3 {
		t.Fatalf("chapters=%d err=%v", len(chapters), err)
	}
	chapterIDs := []string{chapters[0].ChapterID, chapters[1].ChapterID, chapters[2].ChapterID}
	if _, err = database.writer.Exec(ctx, `INSERT INTO drama.source_spans(
		source_span_id,work_id,source_version_id,chapter_id,chapter_revision_id,
		start_utf8_byte,end_utf8_byte,start_codepoint,end_codepoint,start_paragraph,end_paragraph,
		excerpt_hash,evidence_text,idempotency_key)
		SELECT 'span_phase13_'||$2||'_'||membership.ordinal,version.work_id,membership.source_version_id,
			membership.chapter_id,membership.chapter_revision_id,0,octet_length(revision.content),
			0,char_length(revision.content),1,1,
			encode(digest(convert_to(revision.content,'UTF8'),'sha256'),'hex'),revision.content,
			'phase13:span:'||$2||':'||membership.ordinal
		FROM drama.source_version_chapters membership
		JOIN drama.source_versions version USING(source_version_id)
		JOIN drama.chapter_revisions revision USING(chapter_revision_id)
		WHERE membership.source_version_id=$1`, version.SourceVersionID, suffix); err != nil {
		t.Fatal(err)
	}
	irOperation, err := database.StartIRRun(ctx, version.SourceVersionID, "phase13-ir-"+suffix,
		IRRunInput{SchemaVersion: "narrative-extraction.v1", ExtractorVersion: "deterministic-mock-v1", ChapterIDs: chapterIDs})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.writer.Exec(ctx, `UPDATE drama.narrative_ir_revisions SET status='published',is_current=true,
		output_hash=$2,published_at=CURRENT_TIMESTAMP WHERE ir_revision_id=$1`,
		irOperation.TargetID, hashText("phase13-ir-"+suffix)); err != nil {
		t.Fatal(err)
	}
	spec := AdaptationSpecInput{
		SchemaVersion: "adaptation-spec.v1", SourceVersionID: version.SourceVersionID, IRRevisionID: irOperation.TargetID,
		Scope: AdaptationScopeInput{Mode: "chapters_only", ChapterIDs: chapterIDs}, Platform: "test",
		AudienceProfile:    json.RawMessage(`{"description":"成长悬疑用户","tags":["逆袭"]}`),
		TargetEpisodeCount: 3, EpisodeDurationSeconds: 90,
		Rules: []AdaptationRuleInput{{RuleType: "must_preserve", Enforcement: "hard", TargetType: "free_text",
			Priority: 80, Parameters: json.RawMessage(`{"instruction":"保留账册与身份揭露"}`)}},
	}
	projectOperation, err := database.CreateAdaptationProject(ctx, "phase13-project-"+suffix,
		CreateAdaptationProjectInput{DisplayName: "Phase 13 Mock E2E", AdaptationSpec: spec})
	if err != nil {
		t.Fatal(err)
	}
	projectID := projectOperation.TargetID
	storyBibleID := "story-bible-" + suffix
	seasonID := "season-" + suffix
	episodeID := "episode-" + suffix
	if _, err = database.writer.Exec(ctx, `INSERT INTO drama.story_bibles(
		story_bible_id,project_id,version,status) VALUES($1,$2,1,'approved')`, storyBibleID, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.writer.Exec(ctx, `INSERT INTO drama.seasons(
		season_id,project_id,story_bible_id,season_number,title,target_episode_count,
		target_episode_duration_seconds,status,version)
		VALUES($1,$2,$3,1,'第一季',3,90,'approved',1)`, seasonID, projectID, storyBibleID); err != nil {
		t.Fatal(err)
	}
	if _, err = database.writer.Exec(ctx, `INSERT INTO drama.episode_outlines(
		episode_id,season_id,project_id,episode_number,title,opening_hook,story_goal,
		main_conflict,climax,ending_hook,estimated_duration_seconds,status,version)
		VALUES($1,$2,$3,1,'账册疑案','追杀中发现账册','查明父亲死亡真相',
		'证据与亲人安危冲突','密信揭穿背叛','真正主谋在门外现身',90,'approved',1)`, episodeID, seasonID, projectID); err != nil {
		t.Fatal(err)
	}
	analysisKey := "phase13-analysis-" + suffix
	analysis, err := database.RunAdaptationAnalysis(ctx, projectID, analysisKey)
	if err != nil || analysis.Status != "completed" {
		t.Fatalf("analysis=%#v err=%v", analysis, err)
	}
	replay, err := database.RunAdaptationAnalysis(ctx, projectID, analysisKey)
	if err != nil || replay.OperationID != analysis.OperationID {
		t.Fatalf("analysis replay=%#v err=%v", replay, err)
	}
	diagnosticJSON, _, err := database.GetLatestDiagnostic(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostic struct {
		ID    string `json:"diagnostic_report_id"`
		Nodes []any  `json:"nodes"`
	}
	if err := json.Unmarshal(diagnosticJSON, &diagnostic); err != nil || diagnostic.ID == "" || len(diagnostic.Nodes) == 0 {
		t.Fatalf("diagnostic=%s err=%v", diagnosticJSON, err)
	}
	pacingJSON, _, err := database.GetLatestPacing(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	var pacing struct {
		ID    string `json:"pacing_plan_id"`
		Beats []struct {
			Key      string `json:"beat_key"`
			Duration int    `json:"estimated_duration_seconds"`
		} `json:"beats"`
	}
	if err := json.Unmarshal(pacingJSON, &pacing); err != nil || len(pacing.Beats) < 2 {
		t.Fatalf("pacing=%s err=%v", pacingJSON, err)
	}
	scoreJSON, _, err := database.GetLatestQualityScore(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	var score struct {
		Dimensions []struct {
			Name     string `json:"dimension"`
			Evidence []any  `json:"evidence"`
			Issues   []any  `json:"issues"`
		} `json:"dimensions"`
	}
	if err := json.Unmarshal(scoreJSON, &score); err != nil || len(score.Dimensions) != 10 {
		t.Fatalf("score dimensions=%d err=%v payload=%s", len(score.Dimensions), err, scoreJSON)
	}
	for _, dimension := range score.Dimensions {
		if len(dimension.Evidence) == 0 || len(dimension.Issues) == 0 {
			t.Fatalf("dimension %s lacks explainability: %#v", dimension.Name, dimension)
		}
	}

	candidateInput := GenerateCandidateSetInput{Request: candidategeneration.Request{
		TargetType: "episode", TargetID: episodeID,
		ComponentTypes: []string{"opening", "conflict", "climax", "ending_hook"},
		CandidateCount: 3, DifferenceDirections: []string{"强钩子", "紧凑节奏", "低成本可拍"},
		MustPreserve: []string{"主角目标", "真相"}, AllowedChanges: []string{"对白", "场景顺序"},
		Model: "deterministic_mock", RandomSeed: 42, BaseDurationSeconds: 90,
	}}
	candidateKey := "phase14-candidates-" + suffix
	candidateSet, created, err := database.GenerateCandidateSet(ctx, projectID, candidateKey, candidateInput)
	if err != nil || !created || len(candidateSet.Candidates) != 3 {
		t.Fatalf("candidate set=%#v created=%v err=%v", candidateSet, created, err)
	}
	replayedSet, replayCreated, err := database.GenerateCandidateSet(ctx, projectID, candidateKey, candidateInput)
	if err != nil || replayCreated || replayedSet.CandidateSetID != candidateSet.CandidateSetID {
		t.Fatalf("candidate replay=%#v created=%v err=%v", replayedSet, replayCreated, err)
	}
	firstSelection, created, err := database.SelectCandidate(ctx, projectID, candidateSet.CandidateSetID,
		"phase14-select-a-"+suffix, CandidateSelectionInput{
			CandidateID: candidateSet.Candidates[0].CandidateID, Confirmed: true, ConfirmedBy: "phase14-e2e",
		})
	if err != nil || !created {
		t.Fatalf("first selection=%#v created=%v err=%v", firstSelection, created, err)
	}
	composed, created, err := database.ComposeCandidates(ctx, projectID, candidateSet.CandidateSetID,
		"phase14-compose-"+suffix, CandidateCompositionInput{Confirmed: true, ConfirmedBy: "phase14-e2e",
			Parts: []CandidateCompositionPartInput{
				{ComponentKey: "opening", CandidateID: candidateSet.Candidates[0].CandidateID},
				{ComponentKey: "climax", CandidateID: candidateSet.Candidates[1].CandidateID},
				{ComponentKey: "ending_hook", CandidateID: candidateSet.Candidates[2].CandidateID},
			}})
	if err != nil || !created {
		t.Fatalf("composition=%#v created=%v err=%v", composed, created, err)
	}
	var hardRuleCount, passedRuleCount int
	if err := database.writer.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE passed)
		FROM drama.candidate_hard_rule_results WHERE candidate_selection_id=$1`, composed.CandidateSelectionID).
		Scan(&hardRuleCount, &passedRuleCount); err != nil || hardRuleCount != 5 || passedRuleCount != 5 {
		t.Fatalf("hard rules=%d passed=%d err=%v", hardRuleCount, passedRuleCount, err)
	}
	var firstCurrent, composedCurrent bool
	if err := database.writer.QueryRow(ctx, `SELECT is_current FROM drama.artifacts WHERE artifact_id=$1`,
		firstSelection.ArtifactID).Scan(&firstCurrent); err != nil {
		t.Fatal(err)
	}
	if err := database.writer.QueryRow(ctx, `SELECT is_current FROM drama.artifacts WHERE artifact_id=$1`,
		composed.ArtifactID).Scan(&composedCurrent); err != nil {
		t.Fatal(err)
	}
	if firstCurrent || !composedCurrent {
		t.Fatalf("selection must preserve old artifact as history: old=%v new=%v", firstCurrent, composedCurrent)
	}
	var downstreamCurrent, candidateCurrent int
	if err := database.writer.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM drama.artifact_current_bindings WHERE current_artifact_id=$1),
		(SELECT count(*) FROM drama.artifacts artifact JOIN drama.candidates candidate USING(artifact_id)
		 WHERE candidate.candidate_set_id=$2 AND artifact.is_current)`,
		composed.ArtifactID, candidateSet.CandidateSetID).Scan(&downstreamCurrent, &candidateCurrent); err != nil {
		t.Fatal(err)
	}
	if downstreamCurrent != 1 || candidateCurrent != 0 {
		t.Fatalf("only selected artifact may enter downstream: binding=%d candidate_current=%d", downstreamCurrent, candidateCurrent)
	}

	var beat1Artifact, beat2Artifact string
	if err := database.writer.QueryRow(ctx, `SELECT artifact_id FROM drama.pacing_beats
		WHERE pacing_plan_id=$1 AND beat_key=$2`, pacing.ID, pacing.Beats[0].Key).Scan(&beat1Artifact); err != nil {
		t.Fatal(err)
	}
	if err := database.writer.QueryRow(ctx, `SELECT artifact_id FROM drama.pacing_beats
		WHERE pacing_plan_id=$1 AND beat_key=$2`, pacing.ID, pacing.Beats[1].Key).Scan(&beat2Artifact); err != nil {
		t.Fatal(err)
	}
	tx, err := database.writer.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	related, _, err := createArtifactRevision(ctx, tx, projectID, "adaptation_episode_plan", "related-"+suffix, hashText("related"))
	if err == nil {
		var unrelated string
		unrelated, _, err = createArtifactRevision(ctx, tx, projectID, "adaptation_episode_plan", "unrelated-"+suffix, hashText("unrelated"))
		if err == nil {
			err = createDependency(ctx, tx, beat1Artifact, related, "pacing_input", "phase13-related:"+suffix)
		}
		if err == nil {
			err = createDependency(ctx, tx, beat2Artifact, unrelated, "pacing_input", "phase13-unrelated:"+suffix)
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		edit := EditPacingInput{Edits: []PacingBeatEditInput{{
			BeatKey: pacing.Beats[0].Key, EstimatedDurationSeconds: intPointer(pacing.Beats[0].Duration + 7),
		}}}
		if _, err = database.EditPacing(ctx, projectID, pacing.ID, "phase13-edit-"+suffix, edit); err != nil {
			t.Fatal(err)
		}
		var relatedStatus, unrelatedStatus string
		if err = database.writer.QueryRow(ctx, `SELECT validity_status FROM drama.artifacts WHERE artifact_id=$1`, related).Scan(&relatedStatus); err != nil {
			t.Fatal(err)
		}
		if err = database.writer.QueryRow(ctx, `SELECT validity_status FROM drama.artifacts WHERE artifact_id=$1`, unrelated).Scan(&unrelatedStatus); err != nil {
			t.Fatal(err)
		}
		if relatedStatus != "stale" || unrelatedStatus != "valid" {
			t.Fatalf("selective invalidation related=%s unrelated=%s", relatedStatus, unrelatedStatus)
		}
	}

	spec.TargetEpisodeCount = 4
	if _, err = database.CreateAdaptationSpecVersion(ctx, projectID, "phase13-confirm-spec-"+suffix, spec); err != nil {
		t.Fatalf("confirmed spec from diagnosis: %v", err)
	}
}

func intPointer(value int) *int { return &value }
