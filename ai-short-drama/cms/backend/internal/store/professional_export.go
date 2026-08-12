package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/exportkit"
)

type VersionOption struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Label   string `json:"label"`
	Status  string `json:"status"`
}

type ProfessionalExportOptions struct {
	ProjectID       string          `json:"project_id"`
	ProjectName     string          `json:"project_name"`
	WorkID          string          `json:"work_id"`
	WorkTitle       string          `json:"work_title"`
	Episodes        []VersionOption `json:"episodes"`
	Scripts         []VersionOption `json:"scripts"`
	Storyboards     []VersionOption `json:"storyboards"`
	Timelines       []VersionOption `json:"timelines"`
	Masters         []VersionOption `json:"masters"`
	StoryBibles     []VersionOption `json:"story_bibles"`
	SourceVersions  []VersionOption `json:"source_versions"`
	IRRevisions     []VersionOption `json:"ir_revisions"`
	AdaptationSpecs []VersionOption `json:"adaptation_specs"`
	Formats         []string        `json:"formats"`
}

type ProfessionalExportSelection struct {
	EpisodeID               string `json:"episode_id"`
	BundleVersion           int    `json:"bundle_version"`
	ScriptID                string `json:"script_id,omitempty"`
	StoryboardID            string `json:"storyboard_id,omitempty"`
	TimelineID              string `json:"timeline_id,omitempty"`
	MasterID                string `json:"master_id,omitempty"`
	StoryBibleID            string `json:"story_bible_id,omitempty"`
	SourceVersionID         string `json:"source_version_id,omitempty"`
	IRRevisionID            string `json:"ir_revision_id,omitempty"`
	AdaptationSpecVersionID string `json:"adaptation_spec_version_id,omitempty"`
}

type CreateProfessionalExportInput struct {
	Formats     []string                    `json:"formats"`
	Selection   ProfessionalExportSelection `json:"selection"`
	RequestedBy string                      `json:"requested_by"`
}

type ProfessionalExportJob struct {
	ExportID                   string                      `json:"export_id"`
	ProjectID                  string                      `json:"project_id"`
	EpisodeID                  string                      `json:"episode_id"`
	BundleVersion              int                         `json:"bundle_version"`
	Formats                    []string                    `json:"formats"`
	Selection                  ProfessionalExportSelection `json:"selection"`
	SelectionHash              string                      `json:"selection_hash"`
	EffectiveInputResolutionID *string                     `json:"effective_input_resolution_id,omitempty"`
	EffectiveInputHash         *string                     `json:"effective_input_hash,omitempty"`
	GateApprovalID             *string                     `json:"gate_approval_id,omitempty"`
	Manifest                   json.RawMessage             `json:"manifest"`
	Status                     string                      `json:"status"`
	PackagePath                *string                     `json:"package_path,omitempty"`
	PackageHash                *string                     `json:"package_hash,omitempty"`
	ErrorMessage               *string                     `json:"error_message,omitempty"`
	RequestedBy                *string                     `json:"requested_by,omitempty"`
	CreatedAt                  time.Time                   `json:"created_at"`
	CompletedAt                *time.Time                  `json:"completed_at,omitempty"`
}

type LocalEditTarget struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Version    int    `json:"version"`
	Label      string `json:"label"`
	EpisodeID  string `json:"episode_id,omitempty"`
	SceneID    string `json:"scene_id,omitempty"`
	ShotID     string `json:"shot_id,omitempty"`
}

type CreationTargetContext struct {
	WorkID      string            `json:"work_id"`
	WorkTitle   string            `json:"work_title"`
	ProjectID   string            `json:"project_id"`
	ProjectName string            `json:"project_name"`
	Hierarchy   CandidateTargets  `json:"hierarchy"`
	EditTargets []LocalEditTarget `json:"edit_targets"`
}

func (s *Store) GetProfessionalExportOptions(ctx context.Context, projectID, episodeID string) (ProfessionalExportOptions, error) {
	result := ProfessionalExportOptions{ProjectID: projectID, Formats: append([]string(nil), exportkit.Formats...), Episodes: []VersionOption{}, Scripts: []VersionOption{}, Storyboards: []VersionOption{}, Timelines: []VersionOption{}, Masters: []VersionOption{}, StoryBibles: []VersionOption{}, SourceVersions: []VersionOption{}, IRRevisions: []VersionOption{}, AdaptationSpecs: []VersionOption{}}
	err := s.pool.QueryRow(ctx, `SELECT project.novel_name,COALESCE(work.work_id,''),COALESCE(work.title,project.novel_name)
		FROM drama.projects project LEFT JOIN drama.project_source_bindings binding
		 ON binding.project_id=project.project_id AND binding.binding_role='primary' AND binding.is_current
		LEFT JOIN drama.source_works work ON work.work_id=binding.work_id WHERE project.project_id=$1`, projectID).Scan(&result.ProjectName, &result.WorkID, &result.WorkTitle)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProfessionalExportOptions{}, ErrNotFound
	}
	if err != nil {
		return ProfessionalExportOptions{}, err
	}
	result.Episodes, err = s.queryVersionOptions(ctx, `SELECT episode_id,version,'第 '||episode_number||' 集 · '||title,status FROM drama.episode_outlines WHERE project_id=$1 ORDER BY episode_number,version DESC`, projectID)
	if err != nil {
		return result, err
	}
	if episodeID != "" {
		result.Scripts, err = s.queryVersionOptions(ctx, `SELECT script_id,version,title,status FROM drama.episode_scripts WHERE project_id=$1 AND episode_id=$2 ORDER BY version DESC`, projectID, episodeID)
		if err != nil {
			return result, err
		}
		result.Storyboards, err = s.queryVersionOptions(ctx, `SELECT storyboard_id,version,'分镜 v'||version,status FROM drama.storyboards WHERE project_id=$1 AND episode_id=$2 ORDER BY version DESC`, projectID, episodeID)
		if err != nil {
			return result, err
		}
		result.Timelines, err = s.queryVersionOptions(ctx, `SELECT timeline_id,version,'时间线 v'||version,approval_state FROM drama.edit_timelines WHERE project_id=$1 AND episode_id=$2 ORDER BY version DESC`, projectID, episodeID)
		if err != nil {
			return result, err
		}
		result.Masters, err = s.queryVersionOptions(ctx, `SELECT master_id,generation_version,master_type||' v'||generation_version,status FROM drama.episode_masters WHERE project_id=$1 AND episode_id=$2 ORDER BY generation_version DESC`, projectID, episodeID)
		if err != nil {
			return result, err
		}
	}
	result.StoryBibles, err = s.queryVersionOptions(ctx, `SELECT story_bible_id,version,'制作圣经 v'||version,status FROM drama.story_bibles WHERE project_id=$1 ORDER BY version DESC`, projectID)
	if err != nil {
		return result, err
	}
	result.SourceVersions, err = s.queryVersionOptions(ctx, `SELECT version.source_version_id,version.version_number,'原著版本 v'||version.version_number,version.status
		FROM drama.project_source_bindings binding JOIN drama.source_versions version USING(work_id,source_version_id)
		WHERE binding.project_id=$1 ORDER BY binding.is_current DESC,version.version_number DESC`, projectID)
	if err != nil {
		return result, err
	}
	result.IRRevisions, err = s.queryVersionOptions(ctx, `SELECT ir.ir_revision_id,ir.revision_number,'IR v'||ir.revision_number,ir.status
		FROM drama.project_source_bindings binding JOIN drama.narrative_ir_revisions ir USING(work_id,source_version_id)
		WHERE binding.project_id=$1 ORDER BY ir.revision_number DESC`, projectID)
	if err != nil {
		return result, err
	}
	result.AdaptationSpecs, err = s.queryVersionOptions(ctx, `SELECT adaptation_spec_version_id,version_number,'改编 Spec v'||version_number,status
		FROM drama.adaptation_spec_versions WHERE project_id=$1 ORDER BY version_number DESC`, projectID)
	return result, err
}

func (s *Store) queryVersionOptions(ctx context.Context, query string, args ...any) ([]VersionOption, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []VersionOption{}
	for rows.Next() {
		var item VersionOption
		if err = rows.Scan(&item.ID, &item.Version, &item.Label, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetCreationTargetContext(ctx context.Context, projectID string) (CreationTargetContext, error) {
	var result CreationTargetContext
	result.ProjectID = projectID
	result.EditTargets = []LocalEditTarget{}
	err := s.pool.QueryRow(ctx, `SELECT project.novel_name,COALESCE(work.work_id,''),COALESCE(work.title,project.novel_name)
		FROM drama.projects project LEFT JOIN drama.project_source_bindings binding ON binding.project_id=project.project_id
		 AND binding.binding_role='primary' AND binding.is_current LEFT JOIN drama.source_works work ON work.work_id=binding.work_id
		WHERE project.project_id=$1`, projectID).Scan(&result.ProjectName, &result.WorkID, &result.WorkTitle)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	result.Hierarchy, err = s.ListCandidateTargets(ctx, projectID)
	if err != nil {
		return result, err
	}
	rows, err := s.pool.Query(ctx, `WITH native_targets AS (
		SELECT 'outline'::text entity_type,episode_id entity_id,version,'第 '||episode_number||' 集大纲' label,episode_id,''::text scene_id,''::text shot_id FROM drama.episode_outlines WHERE project_id=$1
		UNION ALL SELECT 'episode_content',episode_id,COALESCE((SELECT max(version) FROM drama.entity_versions ev WHERE ev.entity_type='episode_content' AND ev.entity_id=episode.episode_id),1),'第 '||episode_number||' 集完整内容',episode_id,'','' FROM drama.episode_outlines episode WHERE project_id=$1
		UNION ALL SELECT 'script',script_id,version,title,episode_id,'','' FROM drama.episode_scripts WHERE project_id=$1
		UNION ALL SELECT 'scene',scene_id,COALESCE((SELECT max(version) FROM drama.entity_versions ev WHERE ev.entity_type='scene' AND ev.entity_id=scene.scene_id),1),'场 '||scene_number||' · '||COALESCE(NULLIF(location_name,''),NULLIF(scene_purpose,''),'未命名'),episode_id,scene_id,'' FROM drama.script_scenes scene WHERE project_id=$1
		UNION ALL SELECT 'dialogue',dialogue_id,COALESCE((SELECT max(version) FROM drama.entity_versions ev WHERE ev.entity_type='dialogue' AND ev.entity_id=dialogue.dialogue_id),1),COALESCE(NULLIF(speaker_name,''),'旁白')||' · '||left(text,36),episode_id,scene_id,'' FROM drama.dialogues dialogue WHERE project_id=$1
		UNION ALL SELECT 'shot',shot_id,COALESCE((SELECT max(version) FROM drama.entity_versions ev WHERE ev.entity_type='shot' AND ev.entity_id=shot.shot_id),generation_version),'镜 '||shot_order||' · '||left(action_description,36),episode_id,scene_id,shot_id FROM drama.storyboard_shots shot WHERE project_id=$1
		UNION ALL SELECT 'shot_video',video.shot_video_id,video.generation_version,'镜头视频 v'||video.generation_version,video.episode_id,shot.scene_id,video.shot_id FROM drama.shot_videos video JOIN drama.storyboard_shots shot USING(shot_id) WHERE video.project_id=$1
		UNION ALL SELECT 'timeline',timeline_id,version,'时间线 v'||version,episode_id,'','' FROM drama.edit_timelines WHERE project_id=$1
		UNION ALL SELECT 'timeline_item',item.timeline_item_id,timeline.version,item.track_type||' · '||item.sequence_number,timeline.episode_id,'','' FROM drama.edit_timeline_items item JOIN drama.edit_timelines timeline USING(timeline_id) WHERE item.project_id=$1
		UNION ALL SELECT 'media',image.storyboard_image_id,image.generation_version,'分镜图片 v'||image.generation_version,image.episode_id,shot.scene_id,image.shot_id FROM drama.storyboard_images image JOIN drama.storyboard_shots shot USING(shot_id) WHERE image.project_id=$1
		UNION ALL SELECT 'media',video.shot_video_id,video.generation_version,'镜头视频 v'||video.generation_version,video.episode_id,shot.scene_id,video.shot_id FROM drama.shot_videos video JOIN drama.storyboard_shots shot USING(shot_id) WHERE video.project_id=$1
	)
	SELECT entity_type,entity_id,version,label,episode_id,scene_id,shot_id FROM native_targets ORDER BY episode_id,scene_id,shot_id,entity_type,label`, projectID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item LocalEditTarget
		if err = rows.Scan(&item.EntityType, &item.EntityID, &item.Version, &item.Label, &item.EpisodeID, &item.SceneID, &item.ShotID); err != nil {
			return result, err
		}
		result.EditTargets = append(result.EditTargets, item)
	}
	return result, rows.Err()
}

func validateExportInput(input CreateProfessionalExportInput) error {
	if input.Selection.EpisodeID == "" || input.Selection.BundleVersion < 1 || len(input.Formats) == 0 {
		return fmt.Errorf("%w: explicit episode_id, bundle_version and formats are required", ErrValidation)
	}
	if strings.TrimSpace(input.Selection.MasterID) == "" || strings.TrimSpace(input.Selection.TimelineID) == "" {
		return fmt.Errorf("%w: master_id and timeline_id are required for a professional release", ErrValidation)
	}
	seen := map[string]bool{}
	for _, format := range input.Formats {
		if !exportkit.ValidFormat(format) {
			return fmt.Errorf("%w: unsupported export format %q", ErrValidation, format)
		}
		if seen[format] {
			return fmt.Errorf("%w: duplicate export format %q", ErrValidation, format)
		}
		seen[format] = true
	}
	require := func(value, name string, formats ...string) error {
		for _, format := range formats {
			if seen[format] && value == "" {
				return fmt.Errorf("%w: %s is required for %s", ErrValidation, name, format)
			}
		}
		return nil
	}
	if err := require(input.Selection.ScriptID, "script_id", exportkit.ScriptDOCX, exportkit.ScriptFountain); err != nil {
		return err
	}
	if err := require(input.Selection.StoryboardID, "storyboard_id", exportkit.ShotList, exportkit.ContactSheet, exportkit.PromptPackage); err != nil {
		return err
	}
	if err := require(input.Selection.TimelineID, "timeline_id", exportkit.SubtitleSRT, exportkit.SubtitleASS, exportkit.TimelineEDL, exportkit.TimelineXML, exportkit.AudioStems); err != nil {
		return err
	}
	if err := require(input.Selection.StoryBibleID, "story_bible_id", exportkit.ProductionBibles); err != nil {
		return err
	}
	if seen[exportkit.Traceability] && (input.Selection.SourceVersionID == "" || input.Selection.IRRevisionID == "" || input.Selection.AdaptationSpecVersionID == "") {
		return fmt.Errorf("%w: traceability export requires exact source_version_id, ir_revision_id and adaptation_spec_version_id", ErrValidation)
	}
	return nil
}

func (s *Store) CreateProfessionalExport(ctx context.Context, projectID string, input CreateProfessionalExportInput) (ProfessionalExportJob, error) {
	if err := validateExportInput(input); err != nil {
		return ProfessionalExportJob{}, err
	}
	sort.Strings(input.Formats)
	selectionJSON, _ := json.Marshal(input.Selection)
	formatJSON, _ := json.Marshal(input.Formats)
	sum := sha256.Sum256(selectionJSON)
	selectionHash := hex.EncodeToString(sum[:])
	exportID, err := newPublicID("exp_")
	if err != nil {
		return ProfessionalExportJob{}, err
	}
	_, err = s.writer.Exec(ctx, `INSERT INTO drama.professional_export_jobs(export_id,project_id,episode_id,bundle_version,formats,selection,selection_hash,status,requested_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,'building',NULLIF($8,''))`, exportID, projectID, input.Selection.EpisodeID, input.Selection.BundleVersion, formatJSON, selectionJSON, selectionHash, strings.TrimSpace(input.RequestedBy))
	if err != nil {
		return ProfessionalExportJob{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return s.GetProfessionalExport(ctx, projectID, exportID)
}

func (s *Store) GetProfessionalExport(ctx context.Context, projectID, exportID string) (ProfessionalExportJob, error) {
	var item ProfessionalExportJob
	var formatsJSON, selectionJSON []byte
	err := s.pool.QueryRow(ctx, `SELECT export_id,project_id,episode_id,bundle_version,formats,selection,selection_hash,
		effective_input_resolution_id,effective_input_hash,gate_approval_id,manifest,
		status,package_path,package_hash,error_message,requested_by,created_at,completed_at FROM drama.professional_export_jobs
		WHERE export_id=$1 AND project_id=$2`, exportID, projectID).Scan(&item.ExportID, &item.ProjectID, &item.EpisodeID, &item.BundleVersion,
		&formatsJSON, &selectionJSON, &item.SelectionHash, &item.EffectiveInputResolutionID, &item.EffectiveInputHash,
		&item.GateApprovalID, &item.Manifest, &item.Status, &item.PackagePath, &item.PackageHash, &item.ErrorMessage,
		&item.RequestedBy, &item.CreatedAt, &item.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrNotFound
	}
	if err != nil {
		return item, err
	}
	if err = json.Unmarshal(formatsJSON, &item.Formats); err != nil {
		return item, err
	}
	err = json.Unmarshal(selectionJSON, &item.Selection)
	return item, err
}

func (s *Store) ListProfessionalExports(ctx context.Context, projectID, episodeID string) ([]ProfessionalExportJob, error) {
	rows, err := s.pool.Query(ctx, `SELECT export_id FROM drama.professional_export_jobs WHERE project_id=$1 AND ($2='' OR episode_id=$2) ORDER BY created_at DESC`, projectID, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	items := make([]ProfessionalExportJob, 0, len(ids))
	for _, id := range ids {
		item, getErr := s.GetProfessionalExport(ctx, projectID, id)
		if getErr != nil {
			return nil, getErr
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) CompleteProfessionalExport(ctx context.Context, projectID, exportID, packagePath, packageHash string, manifest exportkit.Manifest) (ProfessionalExportJob, error) {
	manifestJSON, _ := json.Marshal(manifest)
	command, err := s.writer.Exec(ctx, `UPDATE drama.professional_export_jobs SET status='ready',package_path=$3,package_hash=$4,manifest=$5,completed_at=CURRENT_TIMESTAMP,error_message=NULL WHERE export_id=$1 AND project_id=$2 AND status='building'`, exportID, projectID, packagePath, packageHash, manifestJSON)
	if err != nil {
		return ProfessionalExportJob{}, err
	}
	if command.RowsAffected() == 0 {
		return ProfessionalExportJob{}, fmt.Errorf("%w: export is not building", ErrConflict)
	}
	return s.GetProfessionalExport(ctx, projectID, exportID)
}

func (s *Store) ValidateProfessionalExportReady(ctx context.Context, projectID, exportID string) error {
	var valid bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM drama.professional_export_jobs job
		JOIN drama.quality_gate_master_approvals approval ON approval.gate_approval_id=job.gate_approval_id
		JOIN drama.quality_gate_runs run ON run.gate_run_id=approval.gate_run_id
		JOIN drama.episode_masters master ON master.master_id=job.selection->>'master_id'
		JOIN drama.edit_timelines timeline ON timeline.timeline_id=job.selection->>'timeline_id'
		WHERE job.export_id=$1 AND job.project_id=$2 AND job.status='ready'
		  AND approval.status='active' AND run.status='approved'
		  AND master.status='ready' AND master.is_current AND timeline.is_current
		  AND master.timeline_id=timeline.timeline_id
		  AND job.effective_input_hash=drama.delivery_effective_input_hash(
		    drama.resolve_effective_inputs(job.project_id,job.episode_id,'post_production'))
		  AND run.snapshot->>'target_timeline_hash'=drama.timeline_content_hash(timeline.timeline_id)
	)`, exportID, projectID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: EXPORT_STALE_BLOCKED: package no longer matches current Resolver/QA chain", ErrConflict)
	}
	return nil
}
func (s *Store) FailProfessionalExport(ctx context.Context, projectID, exportID string, buildErr error) {
	_, _ = s.writer.Exec(ctx, `UPDATE drama.professional_export_jobs SET status='failed',error_message=$3,completed_at=CURRENT_TIMESTAMP WHERE export_id=$1 AND project_id=$2 AND status='building'`, exportID, projectID, buildErr.Error())
}

func (s *Store) BuildProfessionalExportSnapshot(ctx context.Context, job ProfessionalExportJob) (exportkit.Snapshot, error) {
	selection := job.Selection
	snapshot := exportkit.Snapshot{ExportID: job.ExportID, ProjectID: job.ProjectID, EpisodeID: job.EpisodeID, BundleVersion: job.BundleVersion, SelectionHash: job.SelectionHash, CreatedAt: job.CreatedAt}
	snapshot.Selection, _ = json.Marshal(selection)
	err := s.pool.QueryRow(ctx, `SELECT project.novel_name,COALESCE(work.work_id,''),COALESCE(work.title,project.novel_name),
		episode.episode_number,episode.title,to_jsonb(episode)-'id' FROM drama.projects project JOIN drama.episode_outlines episode ON episode.project_id=project.project_id
		LEFT JOIN drama.project_source_bindings binding ON binding.project_id=project.project_id AND binding.binding_role='primary' AND binding.is_current
		LEFT JOIN drama.source_works work ON work.work_id=binding.work_id WHERE project.project_id=$1 AND episode.episode_id=$2`, job.ProjectID, job.EpisodeID).Scan(&snapshot.ProjectName, &snapshot.WorkID, &snapshot.WorkTitle, &snapshot.EpisodeNumber, &snapshot.EpisodeTitle, &snapshot.Outline)
	if err != nil {
		return snapshot, err
	}
	if selection.ScriptID != "" {
		if err = s.loadExportScript(ctx, selection.ScriptID, &snapshot); err != nil {
			return snapshot, err
		}
	}
	if selection.StoryboardID != "" {
		if err = s.loadExportStoryboard(ctx, selection.StoryboardID, &snapshot); err != nil {
			return snapshot, err
		}
	}
	if selection.TimelineID != "" {
		if err = s.loadExportTimeline(ctx, selection.TimelineID, &snapshot); err != nil {
			return snapshot, err
		}
	}
	if selection.StoryBibleID != "" {
		if snapshot.Bibles, err = s.loadExportBibles(ctx, job.ProjectID, selection.StoryBibleID); err != nil {
			return snapshot, err
		}
	} else {
		snapshot.Bibles = json.RawMessage(`{}`)
	}
	if requestedExportFormat(job.Formats, exportkit.PromptPackage) {
		if snapshot.PromptPackage, err = s.loadPromptPackage(ctx, job.ProjectID, job.EpisodeID, selection.StoryboardID); err != nil {
			return snapshot, err
		}
	} else {
		snapshot.PromptPackage = json.RawMessage(`{}`)
	}
	if requestedExportFormat(job.Formats, exportkit.Traceability) {
		if snapshot.Traceability, err = s.loadTraceability(ctx, job, selection); err != nil {
			return snapshot, err
		}
	} else {
		snapshot.Traceability = json.RawMessage(`{}`)
	}
	return snapshot, nil
}

func (s *Store) loadExportScript(ctx context.Context, scriptID string, snapshot *exportkit.Snapshot) error {
	err := s.pool.QueryRow(ctx, `SELECT script_id,version,title FROM drama.episode_scripts WHERE script_id=$1`, scriptID).Scan(&snapshot.ScriptID, &snapshot.ScriptVersion, &snapshot.ScriptTitle)
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `SELECT scene_id,scene_number,location_name,time_of_day,interior_exterior,scene_purpose,actions FROM drama.script_scenes WHERE script_id=$1 ORDER BY scene_number`, scriptID)
	if err != nil {
		return err
	}
	snapshot.Scenes = []exportkit.Scene{}
	sceneIndex := map[string]int{}
	for rows.Next() {
		var scene exportkit.Scene
		if err = rows.Scan(&scene.SceneID, &scene.SceneNumber, &scene.Location, &scene.TimeOfDay, &scene.InteriorExterior, &scene.Purpose, &scene.Actions); err != nil {
			rows.Close()
			return err
		}
		scene.Dialogues = []exportkit.Dialogue{}
		sceneIndex[scene.SceneID] = len(snapshot.Scenes)
		snapshot.Scenes = append(snapshot.Scenes, scene)
	}
	rows.Close()
	dialogueRows, err := s.pool.Query(ctx, `SELECT dialogue_id,scene_id,sequence_number,dialogue_type,speaker_name,text,emotion,estimated_duration_ms FROM drama.dialogues WHERE scene_id IN(SELECT scene_id FROM drama.script_scenes WHERE script_id=$1) ORDER BY scene_id,sequence_number`, scriptID)
	if err != nil {
		return err
	}
	defer dialogueRows.Close()
	for dialogueRows.Next() {
		var dialogue exportkit.Dialogue
		if err = dialogueRows.Scan(&dialogue.DialogueID, &dialogue.SceneID, &dialogue.Sequence, &dialogue.Type, &dialogue.Speaker, &dialogue.Text, &dialogue.Emotion, &dialogue.DurationMS); err != nil {
			return err
		}
		if index, ok := sceneIndex[dialogue.SceneID]; ok {
			snapshot.Scenes[index].Dialogues = append(snapshot.Scenes[index].Dialogues, dialogue)
		}
	}
	return dialogueRows.Err()
}

func (s *Store) loadExportStoryboard(ctx context.Context, storyboardID string, snapshot *exportkit.Snapshot) error {
	if err := s.pool.QueryRow(ctx, `SELECT storyboard_id,version FROM drama.storyboards WHERE storyboard_id=$1`, storyboardID).Scan(&snapshot.StoryboardID, &snapshot.StoryboardVersion); err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `SELECT shot.shot_id,shot.scene_id,shot.shot_order,shot.duration_seconds::float8,shot.shot_size,
		shot.camera_angle,shot.camera_motion,shot.action_description,shot.subtitle_text,shot.visual_prompt_base,shot.video_prompt_base,
		shot.negative_prompt_base,COALESCE(image.storage_url,image.image_url,''),''::text
		FROM drama.storyboard_shots shot LEFT JOIN LATERAL(SELECT storage_url,image_url FROM drama.storyboard_images
		 WHERE shot_id=shot.shot_id AND is_current ORDER BY generation_version DESC LIMIT 1) image ON true
		WHERE shot.storyboard_id=$1 ORDER BY shot.shot_order`, storyboardID)
	if err != nil {
		return err
	}
	defer rows.Close()
	snapshot.Shots = []exportkit.Shot{}
	for rows.Next() {
		var shot exportkit.Shot
		if err = rows.Scan(&shot.ShotID, &shot.SceneID, &shot.ShotOrder, &shot.DurationSeconds, &shot.ShotSize, &shot.CameraAngle, &shot.CameraMotion, &shot.Action, &shot.Subtitle, &shot.VisualPrompt, &shot.VideoPrompt, &shot.NegativePrompt, &shot.ImageURL, &shot.ThumbnailURL); err != nil {
			return err
		}
		snapshot.Shots = append(snapshot.Shots, shot)
	}
	return rows.Err()
}

func (s *Store) loadExportTimeline(ctx context.Context, timelineID string, snapshot *exportkit.Snapshot) error {
	if err := s.pool.QueryRow(ctx, `SELECT timeline_id,version,fps::float8 FROM drama.edit_timelines WHERE timeline_id=$1`, timelineID).Scan(&snapshot.TimelineID, &snapshot.TimelineVersion, &snapshot.FPS); err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `SELECT timeline_item_id,track_type,track_number,sequence_number,entity_id,COALESCE(source_url,''),
		COALESCE(source_path,''),timeline_start_ms,timeline_end_ms,source_in_ms,source_out_ms,volume::float8 FROM drama.edit_timeline_items
		WHERE timeline_id=$1 ORDER BY track_type,track_number,sequence_number`, timelineID)
	if err != nil {
		return err
	}
	defer rows.Close()
	snapshot.TimelineItems = []exportkit.TimelineItem{}
	for rows.Next() {
		var item exportkit.TimelineItem
		if err = rows.Scan(&item.ItemID, &item.TrackType, &item.TrackNumber, &item.Sequence, &item.EntityID, &item.SourceURL, &item.SourcePath, &item.StartMS, &item.EndMS, &item.SourceInMS, &item.SourceOutMS, &item.Volume); err != nil {
			return err
		}
		snapshot.TimelineItems = append(snapshot.TimelineItems, item)
	}
	return rows.Err()
}

func (s *Store) loadExportBibles(ctx context.Context, projectID, storyBibleID string) (json.RawMessage, error) {
	var story, characters, costumes, locations, props json.RawMessage
	err := s.pool.QueryRow(ctx, `SELECT to_jsonb(story_bible_row)-'id' FROM drama.story_bibles story_bible_row WHERE story_bible_id=$1 AND project_id=$2`, storyBibleID, projectID).Scan(&story)
	if err != nil {
		return nil, err
	}
	queries := []struct {
		target *json.RawMessage
		query  string
	}{{&characters, `SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id' ORDER BY version DESC),'[]') FROM drama.character_visual_profiles item WHERE project_id=$1 AND review_status='approved'`}, {&costumes, `SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id' ORDER BY version DESC),'[]') FROM drama.character_costumes item WHERE project_id=$1 AND review_status='approved'`}, {&locations, `SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id' ORDER BY version DESC),'[]') FROM drama.location_visual_profiles item WHERE project_id=$1 AND review_status='approved'`}, {&props, `SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id' ORDER BY version DESC),'[]') FROM drama.prop_visual_profiles item WHERE project_id=$1 AND review_status='approved'`}}
	for _, query := range queries {
		if err = s.pool.QueryRow(ctx, query.query, projectID).Scan(query.target); err != nil {
			return nil, err
		}
	}
	return json.Marshal(map[string]json.RawMessage{"story": story, "characters": characters, "costumes": costumes, "locations": locations, "props": props})
}

func (s *Store) loadPromptPackage(ctx context.Context, projectID, episodeID, storyboardID string) (json.RawMessage, error) {
	var provenance json.RawMessage
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id' ORDER BY created_at),'[]') FROM drama.artifact_generation_provenance item WHERE project_id=$1 AND episode_id=$2`, projectID, episodeID).Scan(&provenance); err != nil {
		return nil, err
	}
	var shots json.RawMessage
	if storyboardID != "" {
		if err := s.pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('shot_id',shot_id,'shot_order',shot_order,'visual_prompt',visual_prompt_base,'video_prompt',video_prompt_base,'negative_prompt',negative_prompt_base) ORDER BY shot_order),'[]') FROM drama.storyboard_shots WHERE storyboard_id=$1`, storyboardID).Scan(&shots); err != nil {
			return nil, err
		}
	} else {
		shots = json.RawMessage(`[]`)
	}
	return json.Marshal(map[string]json.RawMessage{"shots": shots, "generation_provenance": provenance})
}

func (s *Store) loadTraceability(ctx context.Context, job ProfessionalExportJob, selection ProfessionalExportSelection) (json.RawMessage, error) {
	values := map[string]any{"project_id": job.ProjectID, "episode_id": job.EpisodeID, "selection": selection, "selection_hash": job.SelectionHash}
	queries := []struct {
		key, query string
		args       []any
	}{{"source", `SELECT COALESCE(jsonb_agg(to_jsonb(source_row)-'id'),'[]') FROM drama.source_versions source_row WHERE source_version_id=$1`, []any{selection.SourceVersionID}}, {"ir", `SELECT COALESCE(jsonb_agg(to_jsonb(ir)-'id'),'[]') FROM drama.narrative_ir_revisions ir WHERE ir_revision_id=$1`, []any{selection.IRRevisionID}}, {"spec", `SELECT COALESCE(jsonb_agg(to_jsonb(spec)-'id'),'[]') FROM drama.adaptation_spec_versions spec WHERE adaptation_spec_version_id=$1`, []any{selection.AdaptationSpecVersionID}}, {"manual_changes", `SELECT COALESCE(jsonb_agg(to_jsonb(entity_version_row)-'id' ORDER BY created_at),'[]') FROM drama.entity_versions entity_version_row WHERE project_id=$1 AND source_type IN('manual_upload','local_edit','rollback')`, []any{job.ProjectID}}, {"change_plans", `SELECT COALESCE(jsonb_agg(to_jsonb(plan)-'id' ORDER BY created_at),'[]') FROM drama.change_plans plan WHERE project_id=$1 AND status='applied'`, []any{job.ProjectID}}, {"generation_provenance", `SELECT COALESCE(jsonb_agg(to_jsonb(item)-'id' ORDER BY created_at),'[]') FROM drama.artifact_generation_provenance item WHERE project_id=$1 AND episode_id=$2`, []any{job.ProjectID, job.EpisodeID}}}
	for _, query := range queries {
		var raw json.RawMessage
		if err := s.pool.QueryRow(ctx, query.query, query.args...).Scan(&raw); err != nil {
			return nil, err
		}
		values[query.key] = raw
	}
	return json.Marshal(values)
}

func requestedExportFormat(formats []string, requested string) bool {
	for _, format := range formats {
		if format == requested {
			return true
		}
	}
	return false
}

func ExportPackagePath(storageDirectory string, job ProfessionalExportJob) string {
	return filepath.Join(storageDirectory, "exports", job.ProjectID, job.EpisodeID, fmt.Sprintf("v%d", job.BundleVersion), job.ExportID+".zip")
}
