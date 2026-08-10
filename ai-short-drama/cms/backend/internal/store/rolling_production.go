package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type AdoptRollingPlanInput struct {
	Title         string   `json:"title"`
	MaxVideoBatch int      `json:"max_video_batch"`
	TokenBudget   *int64   `json:"token_budget,omitempty"`
	CostBudget    *float64 `json:"cost_budget,omitempty"`
	Currency      string   `json:"currency"`
}

type StoryArcRun struct {
	ArcRunID             string          `json:"arc_run_id"`
	ProjectID            string          `json:"project_id"`
	AdaptationPlanID     *string         `json:"adaptation_plan_id,omitempty"`
	Title                string          `json:"title"`
	SourceChapterIDs     json.RawMessage `json:"source_chapter_ids"`
	FirstChapterOrdinal  *int            `json:"first_chapter_ordinal,omitempty"`
	LastChapterOrdinal   *int            `json:"last_chapter_ordinal,omitempty"`
	PlannedEpisodeCount  int             `json:"planned_episode_count"`
	CurrentEpisodeNumber int             `json:"current_episode_number"`
	Status               string          `json:"status"`
	TokenBudget          *int64          `json:"token_budget,omitempty"`
	CostBudget           *float64        `json:"cost_budget,omitempty"`
	Currency             string          `json:"currency"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
	CompletedAt          *time.Time      `json:"completed_at,omitempty"`
}

type EpisodeProductionRun struct {
	EpisodeRunID            string          `json:"episode_run_id"`
	ArcRunID                string          `json:"arc_run_id"`
	ProjectID               string          `json:"project_id"`
	EpisodeID               string          `json:"episode_id"`
	AdaptationEpisodePlanID *string         `json:"adaptation_episode_plan_id,omitempty"`
	EpisodeNumber           int             `json:"episode_number"`
	Title                   string          `json:"title"`
	SourceChapterIDs        json.RawMessage `json:"source_chapter_ids"`
	CurrentStage            string          `json:"current_stage"`
	Status                  string          `json:"status"`
	GenerationVersion       int             `json:"generation_version"`
	MaxVideoBatch           int             `json:"max_video_batch"`
	TokenBudget             *int64          `json:"token_budget,omitempty"`
	CostBudget              *float64        `json:"cost_budget,omitempty"`
	TokenSpent              int64           `json:"token_spent"`
	CostSpent               float64         `json:"cost_spent"`
	Currency                string          `json:"currency"`
	ContinuityIn            json.RawMessage `json:"continuity_in"`
	ContinuityOut           json.RawMessage `json:"continuity_out"`
	LastErrorCode           *string         `json:"last_error_code,omitempty"`
	LastErrorMessage        *string         `json:"last_error_message,omitempty"`
	StartedAt               *time.Time      `json:"started_at,omitempty"`
	CompletedAt             *time.Time      `json:"completed_at,omitempty"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

type RollingProduction struct {
	Arcs     []StoryArcRun          `json:"arcs"`
	Episodes []EpisodeProductionRun `json:"episodes"`
}

type adaptationEpisodeSeed struct {
	PlanID        string
	EpisodeNumber int
	Title         string
	Logline       string
	Duration      int
	OpeningHook   string
	EndingHook    string
	ContinuityIn  json.RawMessage
	ContinuityOut json.RawMessage
	ChapterIDs    json.RawMessage
}

// AdoptAdaptationPlan creates the compatibility records used by the existing
// episode workflows, but leaves every episode queued. Nothing is generated
// until one episode is explicitly activated by the operator.
func (s *Store) AdoptAdaptationPlan(ctx context.Context, projectID, adaptationPlanID string, input AdoptRollingPlanInput) (RollingProduction, error) {
	projectID = strings.TrimSpace(projectID)
	adaptationPlanID = strings.TrimSpace(adaptationPlanID)
	if projectID == "" || adaptationPlanID == "" {
		return RollingProduction{}, ErrNotFound
	}
	if input.MaxVideoBatch == 0 {
		input.MaxVideoBatch = 5
	}
	if input.MaxVideoBatch < 1 || input.MaxVideoBatch > 20 {
		return RollingProduction{}, fmt.Errorf("%w: max_video_batch must be between 1 and 20", ErrConflict)
	}
	input.Currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	if input.Currency == "" {
		input.Currency = "CNY"
	}

	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return RollingProduction{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, projectID); err != nil {
		return RollingProduction{}, err
	}

	var existingArcID string
	err = tx.QueryRow(ctx, `SELECT arc_run_id FROM drama.story_arc_runs
		WHERE project_id=$1 AND adaptation_plan_id=$2`, projectID, adaptationPlanID).Scan(&existingArcID)
	if err == nil {
		var isCurrent bool
		if err = tx.QueryRow(ctx, `SELECT status='approved' AND is_current
			FROM drama.adaptation_plans
			WHERE project_id=$1 AND adaptation_plan_id=$2`, projectID, adaptationPlanID).Scan(&isCurrent); err != nil {
			return RollingProduction{}, err
		}
		// Older adoption code published only the native plan rows. Repair that
		// projection on an idempotent replay, but never revive a genuinely stale
		// or failed artifact and never displace a newer current plan.
		if isCurrent {
			if err = publishAdaptationPlanArtifacts(ctx, tx, projectID, adaptationPlanID); err != nil {
				return RollingProduction{}, err
			}
		}
		if err = tx.Commit(ctx); err != nil {
			return RollingProduction{}, err
		}
		return s.GetRollingProduction(ctx, projectID)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RollingProduction{}, err
	}

	var (
		planStatus, compilerRunID, specVersionID, irRevisionID string
		workID, sourceVersionID, projectName                   string
		approvedAt                                             *time.Time
	)
	err = tx.QueryRow(ctx, `SELECT plan.status,plan.compiler_run_id,plan.adaptation_spec_version_id,
		compiler.ir_revision_id,compiler.work_id,compiler.source_version_id,project.novel_name,plan.approved_at
		FROM drama.adaptation_plans plan
		JOIN drama.compiler_runs compiler ON compiler.compiler_run_id=plan.compiler_run_id
		JOIN drama.projects project ON project.project_id=plan.project_id
		WHERE plan.project_id=$1 AND plan.adaptation_plan_id=$2
		FOR UPDATE OF plan`, projectID, adaptationPlanID).Scan(
		&planStatus, &compilerRunID, &specVersionID, &irRevisionID,
		&workID, &sourceVersionID, &projectName, &approvedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RollingProduction{}, ErrNotFound
	}
	if err != nil {
		return RollingProduction{}, err
	}
	if planStatus != "approved" || approvedAt == nil {
		return RollingProduction{}, fmt.Errorf("%w: adaptation plan must pass approval validation before queue creation", ErrConflict)
	}

	rows, err := tx.Query(ctx, `SELECT episode.adaptation_episode_plan_id,episode.episode_number,
		episode.title,episode.logline,episode.estimated_duration_seconds,episode.opening_hook,
		episode.ending_hook,episode.continuity_in,episode.continuity_out,
		COALESCE((
			SELECT jsonb_agg(chapter_id ORDER BY first_sequence)
			FROM (
				SELECT fact.chapter_id,min(assignment.sequence_number) AS first_sequence
				FROM drama.episode_event_assignments assignment
				JOIN drama.narrative_event_revisions event
				  ON event.event_revision_id=assignment.event_revision_id
				JOIN drama.narrative_fact_revisions fact
				  ON fact.fact_revision_id=event.fact_revision_id
				WHERE assignment.adaptation_episode_plan_id=episode.adaptation_episode_plan_id
				GROUP BY fact.chapter_id
			) selected_chapters
		),'[]'::jsonb)
		FROM drama.adaptation_episode_plans episode
		WHERE episode.adaptation_plan_id=$1
		ORDER BY episode.episode_number`, adaptationPlanID)
	if err != nil {
		return RollingProduction{}, err
	}
	seeds := make([]adaptationEpisodeSeed, 0)
	for rows.Next() {
		var seed adaptationEpisodeSeed
		if err = rows.Scan(&seed.PlanID, &seed.EpisodeNumber, &seed.Title, &seed.Logline,
			&seed.Duration, &seed.OpeningHook, &seed.EndingHook, &seed.ContinuityIn,
			&seed.ContinuityOut, &seed.ChapterIDs); err != nil {
			rows.Close()
			return RollingProduction{}, err
		}
		seeds = append(seeds, seed)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return RollingProduction{}, err
	}
	if len(seeds) == 0 {
		return RollingProduction{}, fmt.Errorf("%w: adaptation plan has no episodes", ErrConflict)
	}
	if len(seeds) > 12 {
		return RollingProduction{}, fmt.Errorf("%w: a rolling story arc may contain at most 12 episodes", ErrConflict)
	}

	bibleID, err := newPublicID("bible_roll_")
	if err != nil {
		return RollingProduction{}, err
	}
	var bibleVersion int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM drama.story_bibles
		WHERE project_id=$1`, projectID).Scan(&bibleVersion); err != nil {
		return RollingProduction{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.story_bibles(
		story_bible_id,project_id,version,status,characters,relationships,locations,
		world_rules,timeline,key_events,foreshadowing,source_chunk_ids)
	VALUES(
		$1,$2,$3,'approved',
		COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'character_id',entity.entity_id,'name',revision.canonical_name,
			'canonical_name',revision.canonical_name,'attributes',revision.attributes,
			'confidence',revision.confidence) ORDER BY revision.canonical_name)
			FROM drama.narrative_entity_revisions revision
			JOIN drama.narrative_entities entity ON entity.entity_id=revision.entity_id
			WHERE revision.ir_revision_id=$4 AND entity.entity_type='character'
			  AND revision.validation_status<>'invalid'),'[]'::jsonb),
		'[]'::jsonb,
		COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'location_id',entity.entity_id,'name',revision.canonical_name,
			'canonical_name',revision.canonical_name,'attributes',revision.attributes,
			'confidence',revision.confidence) ORDER BY revision.canonical_name)
			FROM drama.narrative_entity_revisions revision
			JOIN drama.narrative_entities entity ON entity.entity_id=revision.entity_id
			WHERE revision.ir_revision_id=$4 AND entity.entity_type='location'
			  AND revision.validation_status<>'invalid'),'[]'::jsonb),
		'[]'::jsonb,
		COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'event_id',event.event_revision_id,'summary',event.summary,
			'event_type',event.event_type,'narrative_order',event.narrative_order)
			ORDER BY event.narrative_order)
			FROM drama.narrative_event_revisions event
			WHERE event.ir_revision_id=$4),'[]'::jsonb),
		COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'event_id',event.event_revision_id,'summary',event.summary,
			'event_type',event.event_type,'importance',event.importance)
			ORDER BY event.narrative_order)
			FROM drama.narrative_event_revisions event
			WHERE event.ir_revision_id=$4),'[]'::jsonb),
		'[]'::jsonb,'[]'::jsonb)`,
		bibleID, projectID, bibleVersion, irRevisionID)
	if err != nil {
		return RollingProduction{}, err
	}

	// A rolling project has no legacy story-bible authoring pass. Seed one
	// locked, source-derived performance contract for every character selected
	// by the approved plan so the first script can consume effective inputs
	// without a circular dependency on script scenes that do not exist yet.
	_, err = tx.Exec(ctx, `WITH selected_characters AS (
		SELECT DISTINCT entity.entity_id character_id,revision.entity_revision_id,
			revision.canonical_name,revision.attributes
		FROM drama.adaptation_episode_plans episode
		JOIN drama.episode_event_assignments assignment
		  ON assignment.adaptation_episode_plan_id=episode.adaptation_episode_plan_id
		JOIN drama.event_participants participant
		  ON participant.event_revision_id=assignment.event_revision_id
		JOIN drama.narrative_entity_revisions revision
		  ON revision.entity_revision_id=participant.entity_revision_id
		JOIN drama.narrative_entities entity ON entity.entity_id=revision.entity_id
		WHERE episode.adaptation_plan_id=$2 AND entity.entity_type='character'
		  AND revision.validation_status<>'invalid'
	), versioned AS (
		SELECT selected.*,
			COALESCE((SELECT max(existing.version)+1
			  FROM drama.character_performance_bibles existing
			  WHERE existing.project_id=$1 AND existing.character_id=selected.character_id
			    AND existing.character_version=selected.entity_revision_id),1) next_version
		FROM selected_characters selected
	)
	INSERT INTO drama.character_performance_bibles(
		performance_bible_id,project_id,character_id,character_version,version,
		speech,acting,relational_voices,appearance,locked_fields,allowed_fields,
		change_reasons,source_refs,status,content_hash,created_by)
	SELECT 'pb_roll_'||substr(encode(drama.digest(convert_to(
		$1||':'||character_id||':'||entity_revision_id||':'||next_version::text,'UTF8'),'sha256'),'hex'),1,20),
		$1,character_id,entity_revision_id,next_version,
		jsonb_build_object('style','source-derived','canonical_name',canonical_name),
		jsonb_build_object('baseline','source-derived','attributes',attributes),
		'{}'::jsonb,
		jsonb_build_object('canonical_name',canonical_name,'attributes',attributes),
		'["identity","speech","acting","appearance"]'::jsonb,
		'["emotion","performance_instruction"]'::jsonb,
		jsonb_build_object('initial','Derived from the frozen Narrative IR during rolling-plan adoption'),
		jsonb_build_object('ir_revision_id',$3::text,'adaptation_plan_id',$2,
		  'entity_revision_id',entity_revision_id),
		'locked',encode(drama.digest(convert_to(jsonb_build_object(
		  'character_id',character_id,'character_version',entity_revision_id,
		  'canonical_name',canonical_name,'attributes',attributes)::text,'UTF8'),'sha256'),'hex'),
		'rolling-production-bootstrap'
	FROM versioned
	WHERE NOT EXISTS(
		SELECT 1 FROM drama.character_performance_bibles locked
		WHERE locked.project_id=$1 AND locked.character_id=versioned.character_id
		  AND locked.status='locked'
	)`, projectID, adaptationPlanID, irRevisionID)
	if err != nil {
		return RollingProduction{}, err
	}

	seasonID, err := newPublicID("season_roll_")
	if err != nil {
		return RollingProduction{}, err
	}
	var seasonNumber int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(season_number),0)+1 FROM drama.seasons
		WHERE project_id=$1`, projectID).Scan(&seasonNumber); err != nil {
		return RollingProduction{}, err
	}
	arcTitle := strings.TrimSpace(input.Title)
	if arcTitle == "" {
		arcTitle = projectName + "｜滚动生产"
	}

	allChapterIDs := uniqueChapterIDs(seeds)
	if len(allChapterIDs) > 30 {
		return RollingProduction{}, fmt.Errorf("%w: a rolling story arc may contain at most 30 source chapters", ErrConflict)
	}
	allChapterJSON, err := json.Marshal(allChapterIDs)
	if err != nil {
		return RollingProduction{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.seasons(
		season_id,project_id,story_bible_id,season_number,title,target_episode_count,
		target_episode_duration_seconds,adaptation_strategy,status,version,generation_config,
		source_chapter_ids,quality_report,adaptation_spec_version_id,compiler_run_id,adaptation_plan_id)
	SELECT $1,$2,$3,$4,$5,$6,spec.episode_duration_seconds,
		'rolling_episode','approved',1,
		jsonb_build_object('production_mode','rolling_episode','max_video_batch',$7::integer),
		$8::jsonb,'{}'::jsonb,$9,$10,$11
	FROM drama.adaptation_spec_versions spec
	WHERE spec.adaptation_spec_version_id=$9`,
		seasonID, projectID, bibleID, seasonNumber, arcTitle, len(seeds),
		input.MaxVideoBatch, allChapterJSON, specVersionID, compilerRunID, adaptationPlanID)
	if err != nil {
		return RollingProduction{}, err
	}

	arcRunID, err := newPublicID("arc_run_")
	if err != nil {
		return RollingProduction{}, err
	}
	var firstOrdinal, lastOrdinal *int
	err = tx.QueryRow(ctx, `SELECT min(ordinal),max(ordinal)
		FROM drama.source_version_chapters
		WHERE work_id=$1 AND source_version_id=$2 AND chapter_id=ANY($3::text[])`,
		workID, sourceVersionID, allChapterIDs).Scan(&firstOrdinal, &lastOrdinal)
	if err != nil {
		return RollingProduction{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO drama.story_arc_runs(
		arc_run_id,project_id,adaptation_plan_id,title,source_chapter_ids,
		first_chapter_ordinal,last_chapter_ordinal,planned_episode_count,status,
		token_budget,cost_budget,currency)
	VALUES($1,$2,$3,$4,$5::jsonb,$6,$7,$8,'ready',$9,$10,$11)`,
		arcRunID, projectID, adaptationPlanID, arcTitle, allChapterJSON,
		firstOrdinal, lastOrdinal, len(seeds), input.TokenBudget, input.CostBudget, input.Currency)
	if err != nil {
		return RollingProduction{}, err
	}

	for _, seed := range seeds {
		episodeID, idErr := newPublicID("episode_roll_")
		if idErr != nil {
			return RollingProduction{}, idErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO drama.episode_outlines(
			episode_id,season_id,project_id,episode_number,title,logline,
			source_chapter_ids,source_chunk_ids,opening_hook,story_goal,main_conflict,
			plot_points,climax,ending_hook,character_ids,location_ids,
			estimated_duration_seconds,continuity_in,continuity_out,status,version,
			adaptation_episode_plan_id,source_ir_revision_id)
		VALUES(
			$1,$2,$3,$4,$5,$6,$7::jsonb,'[]'::jsonb,$8,$6,$6,
			COALESCE((SELECT jsonb_agg(jsonb_build_object(
				'event_id',event.event_revision_id,'description',event.summary,
				'sequence_number',assignment.sequence_number,'usage_mode',assignment.usage_mode)
				ORDER BY assignment.sequence_number)
				FROM drama.episode_event_assignments assignment
				JOIN drama.narrative_event_revisions event
				  ON event.event_revision_id=assignment.event_revision_id
				WHERE assignment.adaptation_episode_plan_id=$9),'[]'::jsonb),
			COALESCE((SELECT event.summary
				FROM drama.episode_event_assignments assignment
				JOIN drama.narrative_event_revisions event
				  ON event.event_revision_id=assignment.event_revision_id
				WHERE assignment.adaptation_episode_plan_id=$9
				ORDER BY assignment.sequence_number DESC LIMIT 1),''),
			$10,
			COALESCE((SELECT jsonb_agg(entity_id ORDER BY entity_id)
				FROM (SELECT DISTINCT entity.entity_id
					FROM drama.episode_event_assignments assignment
					JOIN drama.event_participants participant
					  ON participant.event_revision_id=assignment.event_revision_id
					JOIN drama.narrative_entity_revisions entity
					  ON entity.entity_revision_id=participant.entity_revision_id
					WHERE assignment.adaptation_episode_plan_id=$9) characters),'[]'::jsonb),
			COALESCE((SELECT jsonb_agg(entity_id ORDER BY entity_id)
				FROM (SELECT DISTINCT location.entity_id
					FROM drama.episode_event_assignments assignment
					JOIN drama.narrative_event_revisions event
					  ON event.event_revision_id=assignment.event_revision_id
					JOIN drama.narrative_entity_revisions location
					  ON location.entity_revision_id=event.location_entity_revision_id
					WHERE assignment.adaptation_episode_plan_id=$9) locations),'[]'::jsonb),
			$11,$12::jsonb,$13::jsonb,'approved',1,$9,$14)`,
			episodeID, seasonID, projectID, seed.EpisodeNumber, seed.Title, seed.Logline,
			seed.ChapterIDs, seed.OpeningHook, seed.PlanID, seed.EndingHook, seed.Duration,
			seed.ContinuityIn, seed.ContinuityOut, irRevisionID)
		if err != nil {
			return RollingProduction{}, err
		}
		continuityID, idErr := newPublicID("continuity_roll_")
		if idErr != nil {
			return RollingProduction{}, idErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO drama.continuity_ledger_entries(
			continuity_entry_id,project_id,episode_id,episode_number,scope,sequence_number,
			input_state,output_state,validation_status,diagnostics,state_hash)
		VALUES($1,$2,$3,$4,'episode',0,
			jsonb_build_object('constraints',$5::jsonb,'source','adaptation_episode_plan'),
			jsonb_build_object('constraints',$6::jsonb,'source','adaptation_episode_plan'),
			'valid','[]'::jsonb,
			encode(drama.digest(convert_to(jsonb_build_object(
			  'input',$5::jsonb,'output',$6::jsonb)::text,'UTF8'),'sha256'),'hex'))`,
			continuityID, projectID, episodeID, seed.EpisodeNumber, seed.ContinuityIn, seed.ContinuityOut)
		if err != nil {
			return RollingProduction{}, err
		}
		episodeRunID, idErr := newPublicID("episode_run_")
		if idErr != nil {
			return RollingProduction{}, idErr
		}
		_, err = tx.Exec(ctx, `INSERT INTO drama.episode_production_runs(
			episode_run_id,arc_run_id,project_id,episode_id,adaptation_episode_plan_id,
			episode_number,title,source_chapter_ids,current_stage,status,generation_version,
			max_video_batch,token_budget,cost_budget,currency,continuity_in)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8::jsonb,'season_outline_approved','queued',1,
			$9,$10,$11,$12,$13::jsonb)`,
			episodeRunID, arcRunID, projectID, episodeID, seed.PlanID, seed.EpisodeNumber,
			seed.Title, seed.ChapterIDs, input.MaxVideoBatch, input.TokenBudget,
			input.CostBudget, input.Currency, seed.ContinuityIn)
		if err != nil {
			return RollingProduction{}, err
		}
	}

	if _, err = tx.Exec(ctx, `UPDATE drama.projects
		SET target_episode_count=$2,current_stage='waiting_next_episode',
			status='waiting_next_episode',error_message=NULL,
			config=COALESCE(config,'{}'::jsonb) || jsonb_build_object(
				'production_mode','rolling_episode','max_video_batch',$3::integer,
				'active_arc_run_id',$4::text)
		WHERE project_id=$1`, projectID, len(seeds), input.MaxVideoBatch, arcRunID); err != nil {
		return RollingProduction{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return RollingProduction{}, err
	}
	return s.GetRollingProduction(ctx, projectID)
}

func publishAdaptationPlanArtifacts(
	ctx context.Context,
	tx pgx.Tx,
	projectID string,
	adaptationPlanID string,
) error {
	if _, err := tx.Exec(ctx, `UPDATE drama.artifacts artifact
		SET is_current=CASE
				WHEN artifact.native_entity_id=$2
					AND artifact.validity_status IN ('valid','needs_review') THEN true
				ELSE false
			END,
			validity_status=CASE
				WHEN artifact.native_entity_id=$2 AND artifact.validity_status='needs_review' THEN 'valid'
				WHEN artifact.native_entity_id<>$2
					AND artifact.validity_status IN ('valid','needs_review','rebuilding') THEN 'superseded'
				ELSE artifact.validity_status
			END,
			updated_at=CURRENT_TIMESTAMP
		WHERE artifact.project_id=$1 AND artifact.artifact_type='adaptation_plan'`,
		projectID, adaptationPlanID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE drama.artifacts artifact
		SET is_current=CASE
				WHEN selected.adaptation_episode_plan_id IS NOT NULL
					AND artifact.validity_status IN ('valid','needs_review') THEN true
				ELSE false
			END,
			validity_status=CASE
				WHEN selected.adaptation_episode_plan_id IS NOT NULL
					AND artifact.validity_status='needs_review' THEN 'valid'
				WHEN selected.adaptation_episode_plan_id IS NULL
					AND artifact.validity_status IN ('valid','needs_review','rebuilding') THEN 'superseded'
				ELSE artifact.validity_status
			END,
			updated_at=CURRENT_TIMESTAMP
		FROM (SELECT episode.adaptation_episode_plan_id
			FROM drama.adaptation_episode_plans episode
			WHERE episode.adaptation_plan_id=$2) selected
		RIGHT JOIN drama.artifacts candidate
			ON candidate.native_entity_id=selected.adaptation_episode_plan_id
		WHERE artifact.id=candidate.id
			AND artifact.project_id=$1
			AND artifact.artifact_type='adaptation_episode_plan'`, projectID, adaptationPlanID)
	return err
}

func uniqueChapterIDs(seeds []adaptationEpisodeSeed) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, seed := range seeds {
		var ids []string
		_ = json.Unmarshal(seed.ChapterIDs, &ids)
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Store) GetRollingProduction(ctx context.Context, projectID string) (RollingProduction, error) {
	result := RollingProduction{
		Arcs:     make([]StoryArcRun, 0),
		Episodes: make([]EpisodeProductionRun, 0),
	}
	rows, err := s.pool.Query(ctx, `SELECT arc_run_id,project_id,adaptation_plan_id,title,
		source_chapter_ids,first_chapter_ordinal,last_chapter_ordinal,planned_episode_count,
		current_episode_number,status,token_budget,cost_budget,currency,created_at,updated_at,completed_at
		FROM drama.story_arc_runs WHERE project_id=$1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item StoryArcRun
		if err = rows.Scan(&item.ArcRunID, &item.ProjectID, &item.AdaptationPlanID, &item.Title,
			&item.SourceChapterIDs, &item.FirstChapterOrdinal, &item.LastChapterOrdinal,
			&item.PlannedEpisodeCount, &item.CurrentEpisodeNumber, &item.Status,
			&item.TokenBudget, &item.CostBudget, &item.Currency, &item.CreatedAt,
			&item.UpdatedAt, &item.CompletedAt); err != nil {
			rows.Close()
			return result, err
		}
		result.Arcs = append(result.Arcs, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	rows, err = s.pool.Query(ctx, episodeProductionRunSelect+`
		WHERE run.project_id=$1 ORDER BY run.created_at DESC,run.episode_number`, projectID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		item, scanErr := scanEpisodeProductionRun(rows)
		if scanErr != nil {
			rows.Close()
			return result, scanErr
		}
		result.Episodes = append(result.Episodes, item)
	}
	rows.Close()
	return result, rows.Err()
}

const episodeProductionRunSelect = `SELECT run.episode_run_id,run.arc_run_id,run.project_id,
	run.episode_id,run.adaptation_episode_plan_id,run.episode_number,run.title,
	run.source_chapter_ids,run.current_stage,run.status,run.generation_version,
	run.max_video_batch,run.token_budget,run.cost_budget,run.token_spent,run.cost_spent,
	run.currency,run.continuity_in,run.continuity_out,run.last_error_code,
	run.last_error_message,run.started_at,run.completed_at,run.created_at,run.updated_at
	FROM drama.episode_production_runs run`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEpisodeProductionRun(row rowScanner) (EpisodeProductionRun, error) {
	var item EpisodeProductionRun
	err := row.Scan(&item.EpisodeRunID, &item.ArcRunID, &item.ProjectID, &item.EpisodeID,
		&item.AdaptationEpisodePlanID, &item.EpisodeNumber, &item.Title, &item.SourceChapterIDs,
		&item.CurrentStage, &item.Status, &item.GenerationVersion, &item.MaxVideoBatch,
		&item.TokenBudget, &item.CostBudget, &item.TokenSpent, &item.CostSpent,
		&item.Currency, &item.ContinuityIn, &item.ContinuityOut, &item.LastErrorCode,
		&item.LastErrorMessage, &item.StartedAt, &item.CompletedAt, &item.CreatedAt,
		&item.UpdatedAt)
	return item, err
}

func (s *Store) GetEpisodeProductionRun(ctx context.Context, projectID, episodeRunID string) (EpisodeProductionRun, error) {
	item, err := scanEpisodeProductionRun(s.pool.QueryRow(ctx, episodeProductionRunSelect+`
		WHERE run.project_id=$1 AND run.episode_run_id=$2`, projectID, episodeRunID))
	if errors.Is(err, pgx.ErrNoRows) {
		return EpisodeProductionRun{}, ErrNotFound
	}
	return item, err
}

func (s *Store) GetEpisodeProductionRunByEpisodeID(ctx context.Context, projectID, episodeID string) (EpisodeProductionRun, error) {
	item, err := scanEpisodeProductionRun(s.pool.QueryRow(ctx, episodeProductionRunSelect+`
		WHERE run.project_id=$1 AND run.episode_id=$2
		ORDER BY run.created_at DESC LIMIT 1`, projectID, episodeID))
	if errors.Is(err, pgx.ErrNoRows) {
		return EpisodeProductionRun{}, ErrNotFound
	}
	return item, err
}

func (s *Store) ActivateEpisodeProductionRun(ctx context.Context, projectID, episodeRunID string) (EpisodeProductionRun, error) {
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return EpisodeProductionRun{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, projectID); err != nil {
		return EpisodeProductionRun{}, err
	}
	if _, err = tx.Exec(ctx, `SELECT drama.refresh_episode_production_usage($1)`, episodeRunID); err != nil {
		return EpisodeProductionRun{}, err
	}
	var status, stage, arcRunID string
	var episodeNumber int
	var tokenBudget *int64
	var costBudget *float64
	var tokenSpent int64
	var costSpent float64
	err = tx.QueryRow(ctx, `SELECT status,current_stage,arc_run_id,episode_number,
		token_budget,cost_budget,token_spent,cost_spent
		FROM drama.episode_production_runs
		WHERE project_id=$1 AND episode_run_id=$2 FOR UPDATE`,
		projectID, episodeRunID).Scan(&status, &stage, &arcRunID, &episodeNumber,
		&tokenBudget, &costBudget, &tokenSpent, &costSpent)
	if errors.Is(err, pgx.ErrNoRows) {
		return EpisodeProductionRun{}, ErrNotFound
	}
	if err != nil {
		return EpisodeProductionRun{}, err
	}
	if status == "completed" || status == "cancelled" {
		return EpisodeProductionRun{}, fmt.Errorf("%w: episode run is %s", ErrConflict, status)
	}
	if tokenBudget != nil && tokenSpent >= *tokenBudget {
		return EpisodeProductionRun{}, fmt.Errorf("%w: episode token budget is exhausted", ErrConflict)
	}
	if costBudget != nil && costSpent >= *costBudget {
		return EpisodeProductionRun{}, fmt.Errorf("%w: episode cost budget is exhausted", ErrConflict)
	}
	if status == "queued" {
		var blocking int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM drama.episode_production_runs
			WHERE project_id=$1 AND (
				status IN ('active','waiting_review','paused') OR
				(arc_run_id=$2 AND episode_number<$3 AND status<>'completed')
			)`, projectID, arcRunID, episodeNumber).Scan(&blocking); err != nil {
			return EpisodeProductionRun{}, err
		}
		if blocking > 0 {
			return EpisodeProductionRun{}, fmt.Errorf("%w: finish the current or previous episode first", ErrConflict)
		}
		if _, err = tx.Exec(ctx, `UPDATE drama.episode_production_runs
			SET status='active',started_at=COALESCE(started_at,CURRENT_TIMESTAMP),
				last_error_code=NULL,last_error_message=NULL
			WHERE project_id=$1 AND episode_run_id=$2`, projectID, episodeRunID); err != nil {
			return EpisodeProductionRun{}, err
		}
	} else {
		var projectStage string
		var pendingReviews int
		if err = tx.QueryRow(ctx, `SELECT project.current_stage,
			(SELECT count(*) FROM drama.review_tasks review
			 WHERE review.project_id=project.project_id AND review.review_status='pending')
			FROM drama.projects project WHERE project.project_id=$1`,
			projectID).Scan(&projectStage, &pendingReviews); err != nil {
			return EpisodeProductionRun{}, err
		}
		if pendingReviews > 0 {
			return EpisodeProductionRun{}, fmt.Errorf("%w: finish pending reviews first", ErrConflict)
		}
		if projectStage != "" && projectStage != "waiting_next_episode" {
			stage = projectStage
		}
		if _, err = tx.Exec(ctx, `UPDATE drama.episode_production_runs
			SET status='active',current_stage=$3,
				last_error_code=NULL,last_error_message=NULL
			WHERE project_id=$1 AND episode_run_id=$2`,
			projectID, episodeRunID, stage); err != nil {
			return EpisodeProductionRun{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.story_arc_runs
		SET status='active',current_episode_number=$2
		WHERE project_id=$1 AND arc_run_id=$3`, projectID, episodeNumber, arcRunID); err != nil {
		return EpisodeProductionRun{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.projects
		SET current_stage=$2,status='running',error_message=NULL
		WHERE project_id=$1`, projectID, stage); err != nil {
		return EpisodeProductionRun{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return EpisodeProductionRun{}, err
	}
	return s.GetEpisodeProductionRun(ctx, projectID, episodeRunID)
}

func (s *Store) SyncEpisodeProductionRun(ctx context.Context, projectID, episodeRunID, stage, projectStatus string) error {
	stage = strings.TrimSpace(stage)
	projectStatus = strings.TrimSpace(projectStatus)
	if stage == "" {
		return nil
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, projectID); err != nil {
		return err
	}
	var arcRunID string
	err = tx.QueryRow(ctx, `SELECT arc_run_id FROM drama.episode_production_runs
		WHERE project_id=$1 AND episode_run_id=$2 FOR UPDATE`, projectID, episodeRunID).Scan(&arcRunID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	isComplete := stage == "published" || stage == "stage_5_completed"
	runStatus := "active"
	if strings.Contains(projectStatus, "review") || strings.HasPrefix(stage, "waiting_") {
		runStatus = "waiting_review"
	}
	if strings.Contains(projectStatus, "failed") || strings.HasSuffix(stage, "_failed") {
		runStatus = "failed"
	}
	if isComplete {
		runStatus = "completed"
	}
	_, err = tx.Exec(ctx, `UPDATE drama.episode_production_runs
		SET current_stage=$3,status=$4,
			completed_at=CASE WHEN $4='completed' THEN CURRENT_TIMESTAMP ELSE completed_at END
		WHERE project_id=$1 AND episode_run_id=$2`, projectID, episodeRunID, stage, runStatus)
	if err != nil {
		return err
	}
	if isComplete {
		var remainingInArc int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM drama.episode_production_runs
			WHERE arc_run_id=$1 AND status NOT IN ('completed','cancelled')`, arcRunID).Scan(&remainingInArc); err != nil {
			return err
		}
		if remainingInArc == 0 {
			if _, err = tx.Exec(ctx, `UPDATE drama.story_arc_runs
				SET status='completed',completed_at=CURRENT_TIMESTAMP WHERE arc_run_id=$1`, arcRunID); err != nil {
				return err
			}
		}
		var remainingInProject int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM drama.episode_production_runs
			WHERE project_id=$1 AND status NOT IN ('completed','cancelled')`,
			projectID).Scan(&remainingInProject); err != nil {
			return err
		}
		if remainingInProject == 0 {
			_, err = tx.Exec(ctx, `UPDATE drama.projects SET current_stage='waiting_next_episode',
				status='completed' WHERE project_id=$1`, projectID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE drama.projects SET current_stage='waiting_next_episode',
				status='waiting_next_episode' WHERE project_id=$1`, projectID)
		}
		if err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `SELECT drama.refresh_episode_production_usage($1)`, episodeRunID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
