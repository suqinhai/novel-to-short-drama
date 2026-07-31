package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/postproduction"
)

type CreativeWorkbench struct {
	ProjectID            string          `json:"project_id"`
	EpisodeID            string          `json:"episode_id"`
	Episode              json.RawMessage `json:"episode"`
	Diagnostic           json.RawMessage `json:"diagnostic"`
	PacingBeats          json.RawMessage `json:"pacing_beats"`
	Candidates           json.RawMessage `json:"candidates"`
	Scenes               json.RawMessage `json:"scenes"`
	Dialogues            json.RawMessage `json:"dialogues"`
	Shots                json.RawMessage `json:"shots"`
	DialogueTimings      json.RawMessage `json:"dialogue_timings"`
	SoundCues            json.RawMessage `json:"sound_cues"`
	TimelineVersions     json.RawMessage `json:"timeline_versions"`
	TimelineItems        json.RawMessage `json:"timeline_items"`
	PerformanceBibles    json.RawMessage `json:"performance_bibles"`
	Continuity           json.RawMessage `json:"continuity"`
	QualityIssues        json.RawMessage `json:"quality_issues"`
	VisualQCIssues       json.RawMessage `json:"visual_qc_issues"`
	DialogueTimingIssues json.RawMessage `json:"dialogue_timing_issues"`
	Comments             json.RawMessage `json:"comments"`
	WorkspaceVersions    json.RawMessage `json:"workspace_versions"`
	TemplateBindings     json.RawMessage `json:"template_bindings"`
	EffectiveInputs      json.RawMessage `json:"effective_inputs"`
}

type EditingTemplateRecord struct {
	EditingTemplateID        string          `json:"editing_template_id"`
	EditingTemplateVersionID string          `json:"editing_template_version_id"`
	TemplateKey              string          `json:"template_key"`
	Name                     string          `json:"name"`
	OwnerScope               string          `json:"owner_scope"`
	ProjectID                *string         `json:"project_id,omitempty"`
	Version                  int             `json:"version"`
	Config                   json.RawMessage `json:"config"`
	Status                   string          `json:"status"`
	CreatedAt                time.Time       `json:"created_at"`
}

type TimelineVersionRecord struct {
	TimelineID               string          `json:"timeline_id"`
	ParentTimelineID         *string         `json:"parent_timeline_id,omitempty"`
	ProjectID                string          `json:"project_id"`
	EpisodeID                string          `json:"episode_id"`
	Version                  int             `json:"version"`
	EditingTemplateVersionID *string         `json:"editing_template_version_id,omitempty"`
	VersionReason            string          `json:"version_reason"`
	ApprovalState            string          `json:"approval_state"`
	IsCurrent                bool            `json:"is_current"`
	Transitions              json.RawMessage `json:"transitions"`
	SubtitleConfig           json.RawMessage `json:"subtitle_config"`
	RenderConfig             json.RawMessage `json:"render_config"`
	CreatedAt                time.Time       `json:"created_at"`
}

type ApplyEditingTemplateInput struct {
	EditingTemplateVersionID string         `json:"editing_template_version_id"`
	Scope                    string         `json:"scope"`
	OverrideConfig           map[string]any `json:"override_config"`
	Actor                    *string        `json:"actor,omitempty"`
	Reason                   string         `json:"reason"`
}

type ReplaceSoundStyleInput struct {
	ToStyleGroup string  `json:"to_style_group"`
	Reason       string  `json:"reason,omitempty"`
	Actor        *string `json:"actor,omitempty"`
}

type SoundStyleReplacementResult struct {
	SoundStyleReplacementID string                `json:"sound_style_replacement_id"`
	FromStyleGroup          string                `json:"from_style_group"`
	ToStyleGroup            string                `json:"to_style_group"`
	ReplacedCueVersionIDs   []string              `json:"replaced_cue_version_ids"`
	Timeline                TimelineVersionRecord `json:"timeline"`
}

func (s *Store) GetCreativeWorkbench(ctx context.Context, projectID, episodeID string) (CreativeWorkbench, error) {
	var result CreativeWorkbench
	result.ProjectID, result.EpisodeID = projectID, episodeID
	err := s.pool.QueryRow(ctx, `SELECT to_jsonb(episode)-'id'
		FROM drama.episode_outlines episode WHERE project_id=$1 AND episode_id=$2`,
		projectID, episodeID).Scan(&result.Episode)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreativeWorkbench{}, ErrNotFound
	}
	if err != nil {
		return CreativeWorkbench{}, err
	}
	queries := []struct {
		target *json.RawMessage
		sql    string
	}{
		{&result.Diagnostic, `SELECT COALESCE((
			SELECT to_jsonb(report)-'id' FROM drama.adaptation_diagnostic_reports report
			WHERE report.project_id=$1 AND report.status='completed' ORDER BY report.version_number DESC LIMIT 1
		),'{}'::jsonb) WHERE $2::text IS NOT NULL`},
		{&result.PacingBeats, `SELECT COALESCE(jsonb_agg(to_jsonb(beat)-'id' ORDER BY beat.episode_number,beat.beat_ordinal),'[]')
			FROM drama.pacing_beats beat JOIN drama.pacing_plan_versions plan USING(pacing_plan_id)
			WHERE plan.project_id=$1 AND beat.episode_number=(SELECT episode_number FROM drama.episode_outlines WHERE episode_id=$2)
			  AND plan.status='published'`},
		{&result.Candidates, `SELECT COALESCE(jsonb_agg((to_jsonb(candidate)-'id')||
				jsonb_build_object('score',to_jsonb(score)-'id','total_score',score.total_score)
				ORDER BY candidate.ordinal),'[]')
			FROM drama.candidates candidate JOIN drama.candidate_sets candidate_set USING(candidate_set_id)
			LEFT JOIN drama.candidate_scores score USING(candidate_id)
			WHERE candidate_set.project_id=$1
			  AND candidate_set.target_type='episode' AND candidate_set.target_id=$2`},
		{&result.Scenes, `SELECT COALESCE(jsonb_agg(to_jsonb(scene)-'id' ORDER BY scene.scene_number),'[]')
			FROM drama.script_scenes scene WHERE scene.project_id=$1 AND scene.episode_id=$2`},
		{&result.Dialogues, `SELECT COALESCE(jsonb_agg(to_jsonb(dialogue)-'id' ORDER BY scene.scene_number,dialogue.sequence_number),'[]')
			FROM drama.dialogues dialogue JOIN drama.script_scenes scene USING(scene_id)
			WHERE dialogue.project_id=$1 AND dialogue.episode_id=$2`},
		{&result.Shots, `SELECT COALESCE(jsonb_agg((to_jsonb(shot)-'id')||jsonb_build_object(
				'thumbnail_url',(SELECT image.storage_url FROM drama.storyboard_images image
					WHERE image.shot_id=shot.shot_id AND image.is_current ORDER BY image.generation_version DESC LIMIT 1))
				ORDER BY shot.shot_order),'[]')
			FROM drama.storyboard_shots shot WHERE shot.project_id=$1 AND shot.episode_id=$2`},
		{&result.DialogueTimings, `SELECT COALESCE(jsonb_agg(to_jsonb(timing)-'id' ORDER BY timing.start_ms),'[]')
			FROM drama.dialogue_timing_versions timing WHERE timing.project_id=$1 AND timing.episode_id=$2 AND timing.is_current`},
		{&result.SoundCues, `SELECT COALESCE(jsonb_agg((to_jsonb(cue)-'id')||jsonb_build_object(
				'asset',to_jsonb(asset_version)-'id','asset_name',asset.name)
				ORDER BY cue.start_ms,cue.sequence_number),'[]')
			FROM drama.sound_cue_versions cue
			JOIN drama.sound_asset_versions asset_version USING(sound_asset_version_id)
			JOIN drama.sound_assets asset USING(sound_asset_id)
			WHERE cue.project_id=$1 AND cue.episode_id=$2 AND cue.is_current`},
		{&result.TimelineVersions, `SELECT COALESCE(jsonb_agg(to_jsonb(timeline)-'id' ORDER BY timeline.version DESC),'[]')
			FROM drama.edit_timelines timeline WHERE timeline.project_id=$1 AND timeline.episode_id=$2`},
		{&result.TimelineItems, `SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id' ORDER BY item.track_type,item.track_number,item.sequence_number),'[]')
			FROM drama.edit_timeline_items item JOIN drama.edit_timelines timeline USING(timeline_id)
			WHERE item.project_id=$1 AND item.episode_id=$2 AND timeline.is_current`},
		{&result.PerformanceBibles, `SELECT COALESCE(jsonb_agg(to_jsonb(bible)-'id' ORDER BY bible.character_id,bible.version DESC),'[]')
			FROM drama.character_performance_bibles bible WHERE bible.project_id=$1 AND bible.status IN('approved','locked')
			  AND $2::text IS NOT NULL`},
		{&result.Continuity, `SELECT COALESCE(jsonb_agg(to_jsonb(entry)-'id' ORDER BY entry.sequence_number),'[]')
			FROM drama.continuity_ledger_entries entry WHERE entry.project_id=$1 AND entry.episode_id=$2`},
		{&result.QualityIssues, `SELECT COALESCE(jsonb_agg((to_jsonb(issue)-'id')||jsonb_build_object(
				'editor_link',to_jsonb(link)-'id') ORDER BY issue.severity DESC,issue.episode_number),'[]')
			FROM drama.quality_issues issue JOIN drama.quality_score_reports report USING(quality_score_report_id)
			LEFT JOIN drama.quality_issue_edit_links link ON link.issue_kind='quality' AND link.issue_id=issue.quality_issue_id
			WHERE report.project_id=$1 AND (issue.episode_number IS NULL OR issue.episode_number=
				(SELECT episode_number FROM drama.episode_outlines WHERE episode_id=$2))`},
		{&result.VisualQCIssues, `SELECT COALESCE(jsonb_agg((to_jsonb(issue)-'id')||jsonb_build_object(
				'editor_link',to_jsonb(link)-'id') ORDER BY issue.timecode_ms),'[]')
			FROM drama.visual_qc_issues issue
			LEFT JOIN drama.quality_issue_edit_links link ON link.issue_kind='visual_qc' AND link.issue_id=issue.visual_qc_issue_id
			WHERE issue.project_id=$1 AND issue.episode_id=$2`},
		{&result.DialogueTimingIssues, `SELECT COALESCE(jsonb_agg((to_jsonb(issue)-'id')||jsonb_build_object(
				'editor_link',to_jsonb(link)-'id') ORDER BY issue.start_ms),'[]')
			FROM drama.dialogue_timing_issues issue
			LEFT JOIN drama.quality_issue_edit_links link ON link.issue_kind='dialogue_timing' AND link.issue_id=issue.dialogue_timing_issue_id
			WHERE issue.project_id=$1 AND issue.episode_id=$2`},
		{&result.Comments, `SELECT COALESCE(jsonb_agg(to_jsonb(comment)-'id' ORDER BY comment.created_at),'[]')
			FROM drama.change_comments comment WHERE comment.project_id=$1 AND (
			  comment.entity_id IN(SELECT dialogue_id FROM drama.dialogues WHERE episode_id=$2)
			  OR comment.entity_id IN(SELECT shot_id FROM drama.storyboard_shots WHERE episode_id=$2)
			  OR comment.entity_id IN(SELECT scene_id FROM drama.script_scenes WHERE episode_id=$2))`},
		{&result.WorkspaceVersions, `SELECT COALESCE(jsonb_agg(to_jsonb(workspace)-'id' ORDER BY workspace.version DESC),'[]')
			FROM drama.creative_workspace_versions workspace WHERE workspace.project_id=$1 AND workspace.episode_id=$2`},
		{&result.TemplateBindings, `SELECT COALESCE(jsonb_agg((to_jsonb(binding)-'id')||jsonb_build_object(
				'template',to_jsonb(template)-'id','template_version',to_jsonb(template_version)-'id')
				ORDER BY binding.version DESC),'[]')
			FROM drama.editing_template_bindings binding
			JOIN drama.editing_template_versions template_version USING(editing_template_version_id)
			JOIN drama.editing_templates template USING(editing_template_id)
			WHERE binding.project_id=$1 AND (binding.episode_id IS NULL OR binding.episode_id=$2)`},
		{&result.EffectiveInputs, `SELECT drama.resolve_effective_inputs($1,$2,'17')`},
	}
	for _, query := range queries {
		if err = s.pool.QueryRow(ctx, query.sql, projectID, episodeID).Scan(query.target); err != nil {
			return CreativeWorkbench{}, err
		}
	}
	if err = s.overlayCreativeWorkbenchVersions(ctx, &result); err != nil {
		return CreativeWorkbench{}, err
	}
	return result, nil
}

func (s *Store) ListEditingTemplates(ctx context.Context, projectID string) ([]EditingTemplateRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT template.editing_template_id,version.editing_template_version_id,
		template.template_key,template.name,template.owner_scope,template.project_id,
		version.version,version.config,version.status,version.created_at
		FROM drama.editing_templates template JOIN drama.editing_template_versions version USING(editing_template_id)
		WHERE template.owner_scope='system' OR template.project_id=$1
		ORDER BY template.owner_scope,template.name,version.version DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]EditingTemplateRecord, 0)
	for rows.Next() {
		var item EditingTemplateRecord
		if err = rows.Scan(&item.EditingTemplateID, &item.EditingTemplateVersionID,
			&item.TemplateKey, &item.Name, &item.OwnerScope, &item.ProjectID,
			&item.Version, &item.Config, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ApplyEditingTemplate(
	ctx context.Context, projectID, episodeID string, input ApplyEditingTemplateInput,
) (TimelineVersionRecord, error) {
	return TimelineVersionRecord{}, fmt.Errorf(
		"%w: direct template mutation is disabled; create a timeline change plan", ErrConflict,
	)
}

func (s *Store) ListTimelineVersions(ctx context.Context, projectID, episodeID string) ([]TimelineVersionRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT timeline_id,parent_timeline_id,project_id,episode_id,version,
		editing_template_version_id,version_reason,approval_state,is_current,
		transitions,subtitle_config,render_config,created_at
		FROM drama.edit_timelines WHERE project_id=$1 AND episode_id=$2 ORDER BY version DESC`,
		projectID, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TimelineVersionRecord, 0)
	for rows.Next() {
		var item TimelineVersionRecord
		if err = rows.Scan(&item.TimelineID, &item.ParentTimelineID, &item.ProjectID,
			&item.EpisodeID, &item.Version, &item.EditingTemplateVersionID,
			&item.VersionReason, &item.ApprovalState, &item.IsCurrent, &item.Transitions,
			&item.SubtitleConfig, &item.RenderConfig, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) RestoreTimelineVersion(
	ctx context.Context, projectID, episodeID, sourceTimelineID string, actor *string,
) (TimelineVersionRecord, error) {
	return TimelineVersionRecord{}, fmt.Errorf(
		"%w: direct timeline restore is disabled; create a timeline change plan", ErrConflict,
	)
}

func (s *Store) ReplaceEpisodeSoundStyle(
	ctx context.Context, projectID, episodeID string, input ReplaceSoundStyleInput,
) (SoundStyleReplacementResult, error) {
	return SoundStyleReplacementResult{}, fmt.Errorf(
		"%w: direct sound style mutation is disabled; create a timeline change plan", ErrConflict,
	)
}

func cloneTimelineVersion(
	ctx context.Context, tx pgx.Tx, projectID, episodeID, sourceTimelineID, bindingID,
	templateVersionID, reason, approvalState string, templateConfig, overrideConfig json.RawMessage,
) (TimelineVersionRecord, error) {
	var sourceID string
	if sourceTimelineID != "" {
		sourceID = sourceTimelineID
	} else {
		err := tx.QueryRow(ctx, `SELECT timeline_id FROM drama.edit_timelines
			WHERE project_id=$1 AND episode_id=$2 AND is_current FOR UPDATE`,
			projectID, episodeID).Scan(&sourceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return TimelineVersionRecord{}, ErrNotFound
		}
		if err != nil {
			return TimelineVersionRecord{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE drama.edit_timelines
		SET is_current=false,approval_state=CASE WHEN approval_state='approved' THEN 'superseded' ELSE approval_state END
		WHERE project_id=$1 AND episode_id=$2 AND is_current`, projectID, episodeID); err != nil {
		return TimelineVersionRecord{}, err
	}
	timelineID, err := newPublicID("etl_")
	if err != nil {
		return TimelineVersionRecord{}, err
	}
	if approvalState == "" {
		approvalState = "draft"
	}
	tag, err := tx.Exec(ctx, `INSERT INTO drama.edit_timelines(
		timeline_id,project_id,episode_id,script_id,storyboard_id,audio_plan_id,version,
		resolution,aspect_ratio,fps,video_codec,audio_codec,sample_rate,target_duration_ms,
		tracks,transitions,subtitle_config,render_config,source_versions,status,
		parent_timeline_id,editing_template_binding_id,editing_template_version_id,
		version_reason,approval_state,is_current)
		SELECT $4,project_id,episode_id,script_id,storyboard_id,audio_plan_id,
		(SELECT COALESCE(max(version),0)+1 FROM drama.edit_timelines WHERE episode_id=$2),
		resolution,aspect_ratio,fps,video_codec,audio_codec,sample_rate,target_duration_ms,
		tracks,
		CASE WHEN $9::jsonb IS NULL THEN transitions ELSE COALESCE($9::jsonb->'transitions',transitions) END,
		subtitle_config||COALESCE($9::jsonb->'subtitle','{}')||COALESCE($10::jsonb->'subtitle','{}'),
		render_config||COALESCE($9::jsonb,'{}')||COALESCE($10::jsonb,'{}'),
		source_versions,'draft',$3,
		COALESCE(NULLIF($5,''),editing_template_binding_id),
		COALESCE(NULLIF($6,''),editing_template_version_id),$7,$8,true
		FROM drama.edit_timelines WHERE project_id=$1 AND episode_id=$2 AND timeline_id=$3`,
		projectID, episodeID, sourceID, timelineID, bindingID, templateVersionID,
		reason, approvalState, nullableJSON(templateConfig), nullableJSON(overrideConfig))
	if err != nil {
		return TimelineVersionRecord{}, err
	}
	if tag.RowsAffected() != 1 {
		return TimelineVersionRecord{}, ErrNotFound
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.edit_timeline_items(
		timeline_item_id,timeline_id,project_id,episode_id,track_type,track_number,
		sequence_number,entity_type,entity_id,source_url,source_path,timeline_start_ms,
		timeline_end_ms,source_in_ms,source_out_ms,duration_ms,volume,fade_in_ms,fade_out_ms,
		transform_config,effect_config,status)
		SELECT 'eti_'||substr(encode(digest(timeline_item_id||':'||$2,'sha256'),'hex'),1,24),
		$2,project_id,episode_id,track_type,track_number,sequence_number,entity_type,entity_id,
		source_url,source_path,timeline_start_ms,timeline_end_ms,source_in_ms,source_out_ms,
		duration_ms,volume,fade_in_ms,fade_out_ms,transform_config,
		effect_config||CASE WHEN $3='' THEN '{}'::jsonb
			ELSE jsonb_build_object('editing_template_version_id',$3::text) END,status
		FROM drama.edit_timeline_items WHERE timeline_id=$1`,
		sourceID, timelineID, templateVersionID)
	if err != nil {
		return TimelineVersionRecord{}, err
	}
	var result TimelineVersionRecord
	err = tx.QueryRow(ctx, `SELECT timeline_id,parent_timeline_id,project_id,episode_id,version,
		editing_template_version_id,version_reason,approval_state,is_current,
		transitions,subtitle_config,render_config,created_at
		FROM drama.edit_timelines WHERE timeline_id=$1`, timelineID).Scan(
		&result.TimelineID, &result.ParentTimelineID, &result.ProjectID, &result.EpisodeID,
		&result.Version, &result.EditingTemplateVersionID, &result.VersionReason,
		&result.ApprovalState, &result.IsCurrent, &result.Transitions,
		&result.SubtitleConfig, &result.RenderConfig, &result.CreatedAt)
	return result, err
}

func (s *Store) SaveDialogueTimingValidation(
	ctx context.Context, projectID, episodeID string, items []postproduction.DialogueTiming,
	report postproduction.TimingReport, actor *string,
) error {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	issuesByDialogue := make(map[string][]postproduction.TimingIssue)
	for _, issue := range report.Issues {
		issuesByDialogue[issue.DialogueID] = append(issuesByDialogue[issue.DialogueID], issue)
	}
	for _, item := range items {
		timingID, idErr := newPublicID("dtv_")
		if idErr != nil {
			return idErr
		}
		var parentID *string
		var version int
		err = tx.QueryRow(ctx, `SELECT dialogue_timing_version_id,version
			FROM drama.dialogue_timing_versions WHERE dialogue_id=$1 AND is_current FOR UPDATE`,
			item.DialogueID).Scan(&parentID, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			parentID, version, err = nil, 0, nil
		}
		if err != nil {
			return err
		}
		if parentID != nil {
			if _, err = tx.Exec(ctx, `UPDATE drama.dialogue_timing_versions
				SET is_current=false,status=CASE WHEN status='approved' THEN 'superseded' ELSE status END
				WHERE dialogue_timing_version_id=$1`, *parentID); err != nil {
				return err
			}
		}
		itemIssues := issuesByDialogue[item.DialogueID]
		issueCodes, _ := json.Marshal(issueCodeList(itemIssues))
		content, _ := json.Marshal(item)
		hash := sha256.Sum256(content)
		status := "aligned"
		if len(itemIssues) > 0 {
			status = "warning"
		}
		analyzerVersion := item.AnalyzerVersion
		if analyzerVersion == "" {
			analyzerVersion = "deterministic-lipsync-v1"
		}
		tag, insertErr := tx.Exec(ctx, `INSERT INTO drama.dialogue_timing_versions(
			dialogue_timing_version_id,project_id,episode_id,scene_id,shot_id,dialogue_id,
			dialogue_audio_id,speaker_character_id,speaker_name,turn_group,turn_index,start_ms,end_ms,
			audio_duration_ms,target_lip_start_ms,target_lip_end_ms,visible_character_ids,
			detected_speaker_id,detected_lip_start_ms,detected_lip_end_ms,lip_offset_ms,
			confidence,issue_codes,version,parent_timing_version_id,status,is_current,analyzer_version,
			content_hash,created_by)
			SELECT $1,$2,$3,dialogue.scene_id,$4,dialogue.dialogue_id,
			COALESCE(NULLIF($5,''),(SELECT audio.dialogue_audio_id FROM drama.dialogue_audio audio
				WHERE audio.dialogue_id=dialogue.dialogue_id AND audio.is_current
				ORDER BY audio.generation_version DESC LIMIT 1)),
			COALESCE(NULLIF($6,''),dialogue.character_id),COALESCE(NULLIF($7,''),dialogue.speaker_name),
			$8,$9,$10,$11,$12,$13,$14,$15::jsonb,NULLIF($16,''),
			$17,$18,$19,$20,$21::jsonb,$22,$23,$24,true,$25,$26,$27
			FROM drama.dialogues dialogue
			WHERE dialogue.project_id=$2 AND dialogue.episode_id=$3 AND dialogue.dialogue_id=$28`,
			timingID, projectID, episodeID, item.ShotID, item.DialogueAudioID,
			item.SpeakerCharacterID, item.SpeakerName, item.TurnGroup, item.TurnIndex,
			item.StartMS, item.EndMS, item.AudioDurationMS, item.TargetLipStartMS,
			item.TargetLipEndMS, mustJSONRawPP(item.VisibleCharacterIDs), item.DetectedSpeakerID,
			nullablePositive(item.DetectedLipStartMS), nullablePositive(item.DetectedLipEndMS),
			maxLipOffset(item), nullableConfidence(item.Confidence), issueCodes,
			version+1, parentID, status, analyzerVersion, hex.EncodeToString(hash[:]), actor,
			item.DialogueID)
		if insertErr != nil {
			return insertErr
		}
		if tag.RowsAffected() != 1 {
			return ErrNotFound
		}
		for _, issue := range itemIssues {
			issueID, idErr := newPublicID("dti_")
			if idErr != nil {
				return idErr
			}
			dialogueSuggestions := report.Suggestions[item.DialogueID]
			if dialogueSuggestions == nil {
				dialogueSuggestions = []postproduction.DurationSuggestion{}
			}
			suggestions, _ := json.Marshal(dialogueSuggestions)
			if _, err = tx.Exec(ctx, `INSERT INTO drama.dialogue_timing_issues(
				dialogue_timing_issue_id,dialogue_timing_version_id,project_id,episode_id,
				issue_code,severity,start_ms,end_ms,offset_ms,message,suggestions)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb)`,
				issueID, timingID, projectID, episodeID, issue.Code, issue.Severity,
				issue.StartMS, issue.EndMS, issue.OffsetMS, issue.Message, suggestions); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func mustJSONRawPP(value any) json.RawMessage {
	result, _ := json.Marshal(value)
	return result
}

func nullablePositive(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableConfidence(value float64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func issueCodeList(values []postproduction.TimingIssue) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Code)
	}
	return result
}

func maxLipOffset(item postproduction.DialogueTiming) int64 {
	start := item.DetectedLipStartMS - item.TargetLipStartMS
	end := item.DetectedLipEndMS - item.TargetLipEndMS
	if start < 0 {
		start = -start
	}
	if end < 0 {
		end = -end
	}
	if start > end {
		return start
	}
	return end
}
