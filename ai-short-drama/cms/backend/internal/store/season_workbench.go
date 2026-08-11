package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/localedit"
)

type SeasonEventCard struct {
	CardID           string          `json:"card_id"`
	PresentationMode string          `json:"presentation_mode"`
	SourceEventIDs   []string        `json:"source_event_ids"`
	Summary          string          `json:"summary"`
	Rationale        string          `json:"rationale,omitempty"`
	SplitLabel       string          `json:"split_label,omitempty"`
	MergeGroupID     string          `json:"merge_group_id,omitempty"`
	Importance       float64         `json:"importance,omitempty"`
	SourceChapterIDs []string        `json:"source_chapter_ids,omitempty"`
	ChapterTitle     string          `json:"chapter_title,omitempty"`
	Participants     json.RawMessage `json:"participants,omitempty"`
	CharacterStates  json.RawMessage `json:"character_states,omitempty"`
	Foreshadowing    json.RawMessage `json:"foreshadowing,omitempty"`
}

type SeasonEpisodeDraft struct {
	EpisodeNumber            int               `json:"episode_number"`
	Title                    string            `json:"title"`
	Logline                  string            `json:"logline"`
	ThreeSecondOpening       string            `json:"three_second_opening"`
	FirstThirtySecondsGoal   string            `json:"first_thirty_seconds_goal"`
	CoreConflict             string            `json:"core_conflict"`
	Climax                   string            `json:"climax"`
	EndingHook               string            `json:"ending_hook"`
	EmotionCurve             json.RawMessage   `json:"emotion_curve"`
	InformationRevealAmount  float64           `json:"information_reveal_amount"`
	EstimatedDurationSeconds int               `json:"estimated_duration_seconds"`
	ContinuityIn             []string          `json:"continuity_in,omitempty"`
	ContinuityOut            []string          `json:"continuity_out,omitempty"`
	Events                   []SeasonEventCard `json:"events"`
}

type SeasonPlanDraft struct {
	SchemaVersion       string               `json:"schema_version"`
	PlanName            string               `json:"plan_name"`
	StrategyLabel       string               `json:"strategy_label"`
	Episodes            []SeasonEpisodeDraft `json:"episodes"`
	OmittedEvents       []SeasonEventCard    `json:"omitted_events"`
	CreativeSuggestions json.RawMessage      `json:"creative_suggestions,omitempty"`
	PreviewFingerprint  string               `json:"preview_fingerprint,omitempty"`
}

type SeasonPlanChangePreview struct {
	ChangePlanID         string                 `json:"change_plan_id"`
	Status               string                 `json:"status"`
	BaseAdaptationPlanID string                 `json:"base_adaptation_plan_id"`
	BaseVersion          int                    `json:"base_version"`
	BaseContentHash      string                 `json:"base_content_hash"`
	PreviewFingerprint   string                 `json:"preview_fingerprint"`
	Diff                 []map[string]any       `json:"diff"`
	StaleScope           []map[string]any       `json:"stale_scope"`
	Validation           SeasonValidationResult `json:"validation"`
}

const seasonPlanDraftSnapshotSQL = `SELECT jsonb_build_object(
	'schema_version',COALESCE(NULLIF(plan.workbench_snapshot->>'schema_version',''),'season-plan-draft.v1'),
	'plan_name',plan.plan_name,'strategy_label',plan.strategy_label,
	'episodes',CASE WHEN jsonb_typeof(plan.workbench_snapshot->'episodes')='array'
		AND jsonb_array_length(plan.workbench_snapshot->'episodes')>0
		THEN plan.workbench_snapshot->'episodes'
		ELSE COALESCE((SELECT jsonb_agg(jsonb_build_object(
			'episode_number',episode.episode_number,'title',episode.title,'logline',episode.logline,
			'three_second_opening',episode.three_second_opening,
			'first_thirty_seconds_goal',episode.first_thirty_seconds_goal,
			'core_conflict',episode.core_conflict,'climax',episode.climax,'ending_hook',episode.ending_hook,
			'emotion_curve',episode.emotion_curve,'information_reveal_amount',episode.information_reveal_amount,
			'estimated_duration_seconds',episode.estimated_duration_seconds,
			'continuity_in',episode.continuity_in,'continuity_out',episode.continuity_out,
			'events',COALESCE((SELECT jsonb_agg(jsonb_build_object(
				'card_id','card_'||assignment.episode_event_assignment_id,
				'presentation_mode',assignment.usage_mode,
				'source_event_ids',jsonb_build_array(assignment.event_revision_id),
				'summary',event.summary,'rationale','',
				'merge_group_id',COALESCE(assignment.merge_group_id,''),'importance',event.importance)
				ORDER BY assignment.sequence_number)
				FROM drama.episode_event_assignments assignment
				JOIN drama.narrative_event_revisions event USING(event_revision_id)
				WHERE assignment.adaptation_episode_plan_id=episode.adaptation_episode_plan_id),'[]'::jsonb)
		) ORDER BY episode.episode_number)
		FROM drama.adaptation_episode_plans episode
		WHERE episode.adaptation_plan_id=plan.adaptation_plan_id),'[]'::jsonb) END,
	'omitted_events',COALESCE(plan.workbench_snapshot->'omitted_events','[]'::jsonb),
	'creative_suggestions',COALESCE(plan.creative_suggestions,'[]'::jsonb)
) FROM drama.adaptation_plans plan WHERE plan.adaptation_plan_id=$1`

type SeasonDiagnostic struct {
	Severity   string         `json:"severity"`
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	EntityType *string        `json:"entity_type"`
	EntityID   *string        `json:"entity_id"`
	Details    map[string]any `json:"details"`
}

type SeasonValidationResult struct {
	ValidatorVersion string                        `json:"validator_version"`
	Passed           bool                          `json:"passed"`
	Checks           map[string]bool               `json:"checks"`
	Diagnostics      []SeasonDiagnostic            `json:"diagnostics"`
	RuleViolations   map[string][]SeasonDiagnostic `json:"rule_violations"`
}

type SeasonPlanSummary struct {
	AdaptationPlanID       string     `json:"adaptation_plan_id"`
	ParentAdaptationPlanID *string    `json:"parent_adaptation_plan_id,omitempty"`
	VersionNumber          int        `json:"version_number"`
	PlanName               string     `json:"plan_name"`
	StrategyLabel          string     `json:"strategy_label"`
	Status                 string     `json:"status"`
	IsCurrent              bool       `json:"is_current"`
	EpisodeCount           int        `json:"episode_count"`
	TotalDurationSeconds   int        `json:"total_duration_seconds"`
	BlockingViolations     int        `json:"blocking_violations"`
	WarningViolations      int        `json:"warning_violations"`
	ApprovedAt             *time.Time `json:"approved_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
}

type SeasonApprovalResult struct {
	AdaptationPlanID string                 `json:"adaptation_plan_id"`
	Status           string                 `json:"status"`
	QueueCreated     bool                   `json:"queue_created"`
	Validation       SeasonValidationResult `json:"validation"`
}

type seasonRule struct {
	ID, RuleType, Enforcement, TargetType string
	TargetID                              *string
	Parameters                            json.RawMessage
}

type seasonEventMeta struct {
	EventID, FactID, ChapterID string
	ArcIDs, ParticipantIDs     []string
}

type seasonRelation struct{ ID, From, To, Kind string }
type seasonStateChange struct {
	ID, CharacterID, Dimension, EventID string
	Sequence                            float64
	Before, After                       json.RawMessage
}
type seasonForeshadow struct {
	ThreadID, EventID, Stage string
	Order                    float64
}
type seasonValidationContext struct {
	ProjectID, IRRevisionID, SpecVersionID, ScopeMode string
	DurationTarget                                    int
	Events                                            map[string]seasonEventMeta
	ScopeChapters, ExcludedChapters                   map[string]bool
	ScopeArcs, ExcludedArcs                           map[string]bool
	Rules                                             []seasonRule
	Relations                                         []seasonRelation
	StateChanges                                      []seasonStateChange
	Foreshadows                                       []seasonForeshadow
}

func stringPointer(value string) *string { return &value }

func (s *Store) loadSeasonValidationContext(ctx context.Context, adaptationPlanID string) (seasonValidationContext, error) {
	result := seasonValidationContext{Events: map[string]seasonEventMeta{}, ScopeChapters: map[string]bool{},
		ExcludedChapters: map[string]bool{}, ScopeArcs: map[string]bool{}, ExcludedArcs: map[string]bool{}}
	err := s.pool.QueryRow(ctx, `SELECT plan.project_id,run.ir_revision_id,plan.adaptation_spec_version_id,
		spec.scope_mode,spec.episode_duration_seconds FROM drama.adaptation_plans plan
		JOIN drama.compiler_runs run ON run.compiler_run_id=plan.compiler_run_id
		JOIN drama.adaptation_spec_versions spec ON spec.adaptation_spec_version_id=plan.adaptation_spec_version_id
		WHERE plan.adaptation_plan_id=$1`, adaptationPlanID).Scan(&result.ProjectID, &result.IRRevisionID,
		&result.SpecVersionID, &result.ScopeMode, &result.DurationTarget)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}

	rows, err := s.pool.Query(ctx, `SELECT event.event_revision_id,event.fact_revision_id,fact.chapter_id,
		COALESCE((SELECT array_agg(arc.story_arc_revision_id ORDER BY arc.story_arc_revision_id)
			FROM drama.story_arc_events arc WHERE arc.event_revision_id=event.event_revision_id),'{}'::text[]),
		COALESCE((SELECT array_agg(DISTINCT participant.entity_revision_id ORDER BY participant.entity_revision_id)
			FROM drama.event_participants participant WHERE participant.event_revision_id=event.event_revision_id),'{}'::text[])
		FROM drama.narrative_event_revisions event JOIN drama.narrative_fact_revisions fact USING(fact_revision_id)
		WHERE event.ir_revision_id=$1`, result.IRRevisionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var event seasonEventMeta
		if err = rows.Scan(&event.EventID, &event.FactID, &event.ChapterID, &event.ArcIDs, &event.ParticipantIDs); err != nil {
			rows.Close()
			return result, err
		}
		result.Events[event.EventID] = event
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}

	rows, err = s.pool.Query(ctx, `SELECT chapter_id,include_mode FROM drama.adaptation_scope_chapters WHERE adaptation_spec_version_id=$1`, result.SpecVersionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var id, mode string
		if err = rows.Scan(&id, &mode); err != nil {
			rows.Close()
			return result, err
		}
		if mode == "include" {
			result.ScopeChapters[id] = true
		} else {
			result.ExcludedChapters[id] = true
		}
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT story_arc_revision_id,include_mode FROM drama.adaptation_scope_arcs WHERE adaptation_spec_version_id=$1`, result.SpecVersionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var id, mode string
		if err = rows.Scan(&id, &mode); err != nil {
			rows.Close()
			return result, err
		}
		if mode == "include" {
			result.ScopeArcs[id] = true
		} else {
			result.ExcludedArcs[id] = true
		}
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, `SELECT adaptation_rule_id,rule_type,enforcement,target_type,target_id,parameters
		FROM drama.adaptation_rules WHERE adaptation_spec_version_id=$1 ORDER BY priority DESC,adaptation_rule_id`, result.SpecVersionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var rule seasonRule
		if err = rows.Scan(&rule.ID, &rule.RuleType, &rule.Enforcement, &rule.TargetType, &rule.TargetID, &rule.Parameters); err != nil {
			rows.Close()
			return result, err
		}
		result.Rules = append(result.Rules, rule)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT event_relation_id,from_event_revision_id,to_event_revision_id,relation_type
		FROM drama.event_relations WHERE ir_revision_id=$1 AND relation_type=ANY($2::text[])`, result.IRRevisionID, []string{"before", "after", "causes", "enables"})
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var relation seasonRelation
		if err = rows.Scan(&relation.ID, &relation.From, &relation.To, &relation.Kind); err != nil {
			rows.Close()
			return result, err
		}
		result.Relations = append(result.Relations, relation)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT state_change_id,character_entity_revision_id,state_dimension,
		COALESCE(trigger_event_revision_id,''),sequence_number,before_state,after_state
		FROM drama.character_state_changes WHERE ir_revision_id=$1 ORDER BY character_entity_revision_id,state_dimension,sequence_number`, result.IRRevisionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var state seasonStateChange
		if err = rows.Scan(&state.ID, &state.CharacterID, &state.Dimension, &state.EventID, &state.Sequence, &state.Before, &state.After); err != nil {
			rows.Close()
			return result, err
		}
		result.StateChanges = append(result.StateChanges, state)
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT foreshadow_thread_id,COALESCE(event_revision_id,''),lifecycle_stage,occurrence_order
		FROM drama.foreshadow_occurrences WHERE ir_revision_id=$1 ORDER BY foreshadow_thread_id,occurrence_order`, result.IRRevisionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item seasonForeshadow
		if err = rows.Scan(&item.ThreadID, &item.EventID, &item.Stage, &item.Order); err != nil {
			rows.Close()
			return result, err
		}
		result.Foreshadows = append(result.Foreshadows, item)
	}
	rows.Close()
	return result, rows.Err()
}

func eventInSeasonScope(event seasonEventMeta, context seasonValidationContext) bool {
	if context.ExcludedChapters[event.ChapterID] {
		return false
	}
	chapter := context.ScopeChapters[event.ChapterID]
	arc, excludedArc := false, false
	for _, id := range event.ArcIDs {
		arc = arc || context.ScopeArcs[id]
		excludedArc = excludedArc || context.ExcludedArcs[id]
	}
	if excludedArc {
		return false
	}
	switch context.ScopeMode {
	case "chapters_only":
		return chapter
	case "arcs_only":
		return arc
	case "intersection":
		return chapter && arc
	default:
		return chapter || arc
	}
}

func ruleMatchesSeasonEvent(rule seasonRule, event seasonEventMeta) bool {
	if rule.TargetType == "free_text" {
		return true
	}
	if rule.TargetID == nil {
		return false
	}
	target := *rule.TargetID
	switch rule.TargetType {
	case "event":
		return target == event.EventID
	case "fact":
		return target == event.FactID
	case "chapter":
		return target == event.ChapterID
	case "story_arc":
		for _, id := range event.ArcIDs {
			if id == target {
				return true
			}
		}
	case "entity":
		for _, id := range event.ParticipantIDs {
			if id == target {
				return true
			}
		}
	case "attribute":
		var parameters map[string]any
		_ = json.Unmarshal(rule.Parameters, &parameters)
		return fmt.Sprint(parameters["owner_id"]) == event.EventID
	}
	return false
}

func ValidateSeasonDraft(draft SeasonPlanDraft, context seasonValidationContext) SeasonValidationResult {
	result := SeasonValidationResult{ValidatorVersion: "season-workbench-go-v1", Checks: map[string]bool{
		"structure": true, "causality": true, "character_state": true, "foreshadowing": true, "duration": true, "rules": true,
	}, Diagnostics: []SeasonDiagnostic{}, RuleViolations: map[string][]SeasonDiagnostic{"hard": {}, "soft": {}}}
	add := func(severity, code, message, entityType, entityID string, details map[string]any) {
		item := SeasonDiagnostic{Severity: severity, Code: code, Message: message, Details: details}
		if entityType != "" {
			item.EntityType = stringPointer(entityType)
		}
		if entityID != "" {
			item.EntityID = stringPointer(entityID)
		}
		result.Diagnostics = append(result.Diagnostics, item)
		if enforcement, ok := details["rule_enforcement"].(string); ok {
			result.RuleViolations[enforcement] = append(result.RuleViolations[enforcement], item)
			result.Checks["rules"] = result.Checks["rules"] && severity != "blocking"
		}
	}
	if draft.SchemaVersion != "season-plan-draft.v1" || len(draft.Episodes) == 0 {
		result.Checks["structure"] = false
		add("blocking", "INVALID_SEASON_STRUCTURE", "整季方案结构无效或没有分集。", "adaptation_plan", "", map[string]any{})
	}
	positions := map[string]int{}
	modes := map[string][]string{}
	episodeSeen := map[int]bool{}
	cursor := 0
	for index, episode := range draft.Episodes {
		if episode.EpisodeNumber != index+1 || episodeSeen[episode.EpisodeNumber] {
			result.Checks["structure"] = false
			add("blocking", "EPISODE_NUMBER_GAP", "分集编号必须从 1 连续递增。", "episode", fmt.Sprint(episode.EpisodeNumber), map[string]any{})
		}
		episodeSeen[episode.EpisodeNumber] = true
		if strings.TrimSpace(episode.ThreeSecondOpening) == "" || strings.TrimSpace(episode.FirstThirtySecondsGoal) == "" || strings.TrimSpace(episode.CoreConflict) == "" || strings.TrimSpace(episode.Climax) == "" || strings.TrimSpace(episode.EndingHook) == "" || len(episode.EmotionCurve) == 0 || string(episode.EmotionCurve) == "[]" {
			result.Checks["structure"] = false
			add("blocking", "EPISODE_STRUCTURE_INCOMPLETE", "必须填写开场3秒、前30秒目标、核心冲突、高潮、结尾钩子和情绪曲线。", "episode", fmt.Sprint(episode.EpisodeNumber), map[string]any{})
		}
		if episode.EstimatedDurationSeconds < 1 || episode.EstimatedDurationSeconds > context.DurationTarget {
			result.Checks["duration"] = false
			add("blocking", "EPISODE_DURATION_EXCEEDED", "预计时长超出改编规格。", "episode", fmt.Sprint(episode.EpisodeNumber), map[string]any{"actual_seconds": episode.EstimatedDurationSeconds, "maximum_seconds": context.DurationTarget})
		}
		if episode.InformationRevealAmount < 0 || episode.InformationRevealAmount > 1 {
			result.Checks["structure"] = false
			add("blocking", "INVALID_INFORMATION_REVEAL", "信息揭示量必须在 0 到 1 之间。", "episode", fmt.Sprint(episode.EpisodeNumber), map[string]any{})
		}
		for _, card := range episode.Events {
			cursor++
			if card.PresentationMode == "original" {
				if len(card.SourceEventIDs) > 0 || strings.TrimSpace(card.Summary) == "" || strings.TrimSpace(card.Rationale) == "" {
					result.Checks["structure"] = false
					add("blocking", "INVALID_ORIGINAL_ADDITION", "原创补充必须无原著事件引用，并填写内容和理由。", "event_card", card.CardID, map[string]any{})
				}
				continue
			}
			if card.PresentationMode == "merge" && len(card.SourceEventIDs) < 2 {
				result.Checks["structure"] = false
				add("blocking", "INVALID_MERGE", "合并呈现至少需要两个原著事件。", "event_card", card.CardID, map[string]any{})
			}
			for _, eventID := range card.SourceEventIDs {
				event, ok := context.Events[eventID]
				if !ok || !eventInSeasonScope(event, context) {
					result.Checks["structure"] = false
					add("blocking", "EVENT_OUTSIDE_FROZEN_SCOPE", "事件不属于冻结的 Narrative IR 范围。", "event", eventID, map[string]any{})
					continue
				}
				if _, exists := positions[eventID]; !exists {
					positions[eventID] = cursor
				}
				modes[eventID] = append(modes[eventID], card.PresentationMode)
			}
		}
	}
	for _, card := range draft.OmittedEvents {
		for _, eventID := range card.SourceEventIDs {
			modes[eventID] = append(modes[eventID], "omit")
		}
	}
	for eventID, event := range context.Events {
		if !eventInSeasonScope(event, context) {
			continue
		}
		present := positions[eventID] > 0
		eventModes := modes[eventID]
		presentModes := make([]string, 0, len(eventModes))
		for _, mode := range eventModes {
			if mode != "omit" {
				presentModes = append(presentModes, mode)
			}
		}
		if len(presentModes) > 1 {
			allSplit := true
			for _, mode := range presentModes {
				allSplit = allSplit && mode == "split"
			}
			if !allSplit {
				result.Checks["structure"] = false
				add("blocking", "DUPLICATE_EVENT_PRESENTATION", "同一事件仅可通过拆分呈现重复出现。", "event", eventID, map[string]any{})
			}
		}
		if !present {
			authorized := false
			for _, rule := range context.Rules {
				if rule.RuleType == "omit_allowed" && ruleMatchesSeasonEvent(rule, event) {
					authorized = true
				}
			}
			if !authorized {
				add("blocking", "OMISSION_NOT_AUTHORIZED", "省略事件必须有明确的允许省略规则。", "event", eventID, map[string]any{"rule_enforcement": "hard"})
			}
		}
		for _, rule := range context.Rules {
			if !ruleMatchesSeasonEvent(rule, event) {
				continue
			}
			severity := "warning"
			if rule.Enforcement == "hard" {
				severity = "blocking"
			}
			violated := false
			code := ""
			message := ""
			switch rule.RuleType {
			case "must_preserve":
				violated = !present
				code = "MUST_PRESERVE_VIOLATION"
				message = "规则要求保留的事件在方案中缺失。"
			case "must_not_change":
				for _, mode := range eventModes {
					violated = violated || !map[string]bool{"preserve": true, "omit": false}[mode]
				}
				code = "MUST_NOT_CHANGE_VIOLATION"
				message = "受保护事件不能合并、拆分或变形。"
			case "transform_required":
				violated = present
				for _, mode := range eventModes {
					if mode == "transform" {
						violated = false
					}
				}
				code = "TRANSFORM_REQUIRED_VIOLATION"
				message = "事件必须按规则执行变形改编。"
			}
			if violated {
				add(severity, code, message, "adaptation_rule", rule.ID, map[string]any{"rule_enforcement": rule.Enforcement, "event_revision_id": eventID})
			}
		}
		for _, mode := range eventModes {
			if mode == "merge" {
				authorized := false
				for _, rule := range context.Rules {
					if rule.RuleType == "merge_allowed" && ruleMatchesSeasonEvent(rule, event) {
						authorized = true
					}
				}
				if !authorized {
					add("blocking", "MERGE_NOT_AUTHORIZED", "合并呈现缺少 merge_allowed 规则。", "event", eventID, map[string]any{"rule_enforcement": "hard"})
				}
			}
		}
	}
	for _, relation := range context.Relations {
		from, to := relation.From, relation.To
		if relation.Kind == "after" {
			from, to = to, from
		}
		if positions[from] > 0 && positions[to] > 0 && positions[from] >= positions[to] {
			result.Checks["causality"] = false
			add("blocking", "CAUSAL_ORDER_VIOLATION", "事件呈现顺序违反因果或前置关系。", "event_relation", relation.ID, map[string]any{"from_event_revision_id": from, "to_event_revision_id": to})
		}
	}
	stateGroups := map[string][]seasonStateChange{}
	for _, state := range context.StateChanges {
		if positions[state.EventID] > 0 {
			key := state.CharacterID + "\x00" + state.Dimension
			stateGroups[key] = append(stateGroups[key], state)
		}
	}
	for _, states := range stateGroups {
		sort.Slice(states, func(i, j int) bool { return states[i].Sequence < states[j].Sequence })
		for index := 1; index < len(states); index++ {
			previous, current := states[index-1], states[index]
			if positions[previous.EventID] >= positions[current.EventID] || string(previous.After) != string(current.Before) {
				result.Checks["character_state"] = false
				add("blocking", "CHARACTER_STATE_DISCONTINUITY", "人物状态变化顺序或前后状态不连续。", "state_change", current.ID, map[string]any{"previous_state_change_id": previous.ID})
			}
		}
	}
	threadGroups := map[string][]seasonForeshadow{}
	for _, item := range context.Foreshadows {
		if positions[item.EventID] > 0 {
			threadGroups[item.ThreadID] = append(threadGroups[item.ThreadID], item)
		}
	}
	for thread, items := range threadGroups {
		sort.Slice(items, func(i, j int) bool { return positions[items[i].EventID] < positions[items[j].EventID] })
		planted := false
		for _, item := range items {
			if item.Stage == "planted" {
				planted = true
			}
			if (item.Stage == "partially_resolved" || item.Stage == "resolved") && !planted {
				result.Checks["foreshadowing"] = false
				add("blocking", "FORESHADOW_RESOLUTION_WITHOUT_PLANT", "伏笔回收早于埋设。", "foreshadow_thread", thread, map[string]any{})
			}
		}
	}
	result.Passed = true
	for _, item := range result.Diagnostics {
		if item.Severity == "blocking" {
			result.Passed = false
			break
		}
	}
	return result
}

func (s *Store) ValidateSeasonPlanDraft(ctx context.Context, adaptationPlanID string, draft SeasonPlanDraft) (SeasonValidationResult, error) {
	contextValue, err := s.loadSeasonValidationContext(ctx, adaptationPlanID)
	if err != nil {
		return SeasonValidationResult{}, err
	}
	return ValidateSeasonDraft(draft, contextValue), nil
}

func (s *Store) ListSeasonPlans(ctx context.Context, projectID string) ([]SeasonPlanSummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT plan.adaptation_plan_id,plan.parent_adaptation_plan_id,plan.version_number,
		plan.plan_name,plan.strategy_label,plan.status,plan.is_current,count(episode.id),
		COALESCE(sum(episode.estimated_duration_seconds),0),
		COALESCE((SELECT count(*) FROM jsonb_array_elements(COALESCE(plan.quality_report->'diagnostics','[]'::jsonb)) item WHERE item->>'severity'='blocking'),0),
		COALESCE((SELECT count(*) FROM jsonb_array_elements(COALESCE(plan.quality_report->'diagnostics','[]'::jsonb)) item WHERE item->>'severity'='warning'),0),
		plan.approved_at,plan.created_at FROM drama.adaptation_plans plan
		LEFT JOIN drama.adaptation_episode_plans episode USING(adaptation_plan_id)
		WHERE plan.project_id=$1 GROUP BY plan.id ORDER BY plan.version_number DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SeasonPlanSummary{}
	for rows.Next() {
		var item SeasonPlanSummary
		if err = rows.Scan(&item.AdaptationPlanID, &item.ParentAdaptationPlanID, &item.VersionNumber, &item.PlanName, &item.StrategyLabel, &item.Status, &item.IsCurrent, &item.EpisodeCount, &item.TotalDurationSeconds, &item.BlockingViolations, &item.WarningViolations, &item.ApprovedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CreateSeasonPlanVersion(ctx context.Context, basePlanID, key string, draft SeasonPlanDraft) (json.RawMessage, string, error) {
	key = strings.TrimSpace(key)
	if key != "" {
		var replayID string
		err := s.pool.QueryRow(ctx, `SELECT adaptation_plan_id FROM drama.adaptation_plans
			WHERE save_idempotency_key=$1`, key).Scan(&replayID)
		if err == nil {
			return s.GetAdaptationPlan(ctx, replayID)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, "", err
		}
	}
	previewFingerprint := strings.TrimSpace(draft.PreviewFingerprint)
	if previewFingerprint == "" {
		return nil, "", fmt.Errorf("%w: a persisted season change plan must be confirmed", ErrConflict)
	}
	validation, err := s.ValidateSeasonPlanDraft(ctx, basePlanID, draft)
	if err != nil {
		return nil, "", err
	}
	draft.PreviewFingerprint = ""
	snapshot, err := json.Marshal(draft)
	if err != nil {
		return nil, "", err
	}
	contentHash, err := hashJSON(draft)
	if err != nil {
		return nil, "", err
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)
	var projectID, compilerRunID, specVersionID, baseHash string
	var baseVersion int
	err = tx.QueryRow(ctx, `SELECT project_id,compiler_run_id,adaptation_spec_version_id,version_number,content_hash
		FROM drama.adaptation_plans WHERE adaptation_plan_id=$1 FOR SHARE`, basePlanID).
		Scan(&projectID, &compilerRunID, &specVersionID, &baseVersion, &baseHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, projectID); err != nil {
		return nil, "", err
	}
	var latestVersion int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(version_number),0) FROM drama.adaptation_plans WHERE project_id=$1`, projectID).Scan(&latestVersion); err != nil {
		return nil, "", err
	}
	if baseVersion != latestVersion {
		return nil, "", fmt.Errorf("%w: adaptation plan base version %d is stale; current latest version is %d", ErrConflict, baseVersion, latestVersion)
	}
	var changePlanID, changePlanStatus string
	var reviewMetadata json.RawMessage
	err = tx.QueryRow(ctx, `SELECT change_plan_id,status,review_metadata
		FROM drama.change_plans
		WHERE project_id=$1 AND target_entity_type='adaptation_plan'
		  AND target_entity_id=$2 AND target_version=$3
		  AND review_metadata->>'preview_fingerprint'=$4
		ORDER BY created_at DESC LIMIT 1 FOR UPDATE`,
		projectID, basePlanID, baseVersion, previewFingerprint).
		Scan(&changePlanID, &changePlanStatus, &reviewMetadata)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", fmt.Errorf("%w: season change plan was not found; preview again", ErrConflict)
	}
	if err != nil {
		return nil, "", err
	}
	if changePlanStatus != "validated" {
		return nil, "", fmt.Errorf("%w: season change plan is %s", ErrConflict, changePlanStatus)
	}
	var review struct {
		BaseContentHash string          `json:"base_content_hash"`
		SeasonDraft     SeasonPlanDraft `json:"season_draft"`
	}
	if err = json.Unmarshal(reviewMetadata, &review); err != nil || review.BaseContentHash != baseHash {
		return nil, "", fmt.Errorf("%w: season change plan provenance no longer matches its base", ErrConflict)
	}
	review.SeasonDraft.PreviewFingerprint = ""
	if !reflect.DeepEqual(normalizeJSONValue(review.SeasonDraft), normalizeJSONValue(draft)) {
		return nil, "", fmt.Errorf("%w: submitted season draft differs from the reviewed change plan", ErrConflict)
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.change_plans
		SET status='executing',confirmed_by='season-workbench',confirmed_at=now()
		WHERE change_plan_id=$1`, changePlanID); err != nil {
		return nil, "", err
	}
	confirmedEventID, err := newPublicID("cpe_")
	if err != nil {
		return nil, "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.change_plan_events(
		change_plan_event_id,change_plan_id,event_type,actor,details)
		VALUES($1,$2,'confirmed','season-workbench',jsonb_build_object('preview_fingerprint',$3::text))`,
		confirmedEventID, changePlanID, previewFingerprint); err != nil {
		return nil, "", err
	}
	planID, err := newPublicID("ap_")
	if err != nil {
		return nil, "", err
	}
	version := latestVersion + 1
	creative := draft.CreativeSuggestions
	if len(creative) == 0 {
		creative = json.RawMessage(`[]`)
	}
	validationJSON, _ := json.Marshal(validation)
	compilerValidation := map[string]bool{
		"hard_rules_satisfied": validation.Passed, "event_references_valid": validation.Checks["structure"],
		"timeline_valid": validation.Checks["causality"], "causality_valid": validation.Checks["causality"],
		"foreshadowing_valid": validation.Checks["foreshadowing"], "duration_valid": validation.Checks["duration"],
		"pacing_valid": validation.Checks["duration"], "hook_valid": validation.Checks["structure"],
		"emotion_valid": validation.Checks["structure"], "character_arc_valid": validation.Checks["character_state"],
		"information_reveal_valid": validation.Checks["structure"],
	}
	compilerValidationJSON, _ := json.Marshal(compilerValidation)
	_, err = tx.Exec(ctx, `INSERT INTO drama.adaptation_plans(adaptation_plan_id,compiler_run_id,project_id,adaptation_spec_version_id,
		version_number,status,is_current,content_hash,quality_report,parent_adaptation_plan_id,plan_name,strategy_label,
		workbench_snapshot,creative_suggestions,save_idempotency_key)
		VALUES($1,$2,$3,$4,$5,'draft',false,$6,jsonb_build_object('validation',$7::jsonb,'diagnostics',$8::jsonb->'diagnostics','workbench_validation',$8::jsonb,'change_plan_id',$15::text),$9,$10,$11,$12::jsonb,$13::jsonb,$14)`,
		planID, compilerRunID, projectID, specVersionID, version, contentHash, compilerValidationJSON, validationJSON, basePlanID,
		strings.TrimSpace(draft.PlanName), strings.TrimSpace(draft.StrategyLabel), snapshot, creative, key, changePlanID)
	if err != nil {
		return nil, "", mapPGConflict(err)
	}
	validationContext, err := s.loadSeasonValidationContext(ctx, basePlanID)
	if err != nil {
		return nil, "", err
	}
	for _, episode := range draft.Episodes {
		episodeID, idErr := newPublicID("aep_")
		if idErr != nil {
			return nil, "", idErr
		}
		eventIDs := []string{}
		chapters := []string{}
		chapterSeen := map[string]bool{}
		eventSeen := map[string]bool{}
		added := []map[string]any{}
		merged := []map[string]any{}
		deviations := []map[string]any{}
		characterIDs := []string{}
		arcIDs := []string{}
		for _, card := range episode.Events {
			if card.PresentationMode == "original" {
				added = append(added, map[string]any{"content_id": card.CardID, "description": card.Summary, "reason": card.Rationale, "rule_ids": []string{"workbench_explicit_original"}})
				deviations = append(deviations, map[string]any{"deviation_id": "deviation_" + card.CardID, "kind": "addition", "description": card.Rationale, "source_event_ids": []string{}, "rule_ids": []string{"workbench_explicit_original"}})
			}
			if card.PresentationMode == "merge" {
				mergeRules := matchingRuleIDs(validationContext, card.SourceEventIDs, "merge_allowed")
				if len(mergeRules) == 0 {
					mergeRules = []string{"workbench_unresolved_merge_rule"}
				}
				merged = append(merged, map[string]any{"merge_group_id": card.MergeGroupID, "source_event_ids": card.SourceEventIDs, "description": card.Summary, "rule_ids": mergeRules})
			}
			if card.PresentationMode == "split" || card.PresentationMode == "transform" {
				deviations = append(deviations, map[string]any{"deviation_id": "deviation_" + card.CardID, "kind": "transform", "description": card.Rationale, "source_event_ids": card.SourceEventIDs, "rule_ids": matchingRuleIDs(validationContext, card.SourceEventIDs, "transform_required")})
			}
			for _, eventID := range card.SourceEventIDs {
				if eventSeen[eventID] {
					continue
				}
				eventSeen[eventID] = true
				eventIDs = append(eventIDs, eventID)
				meta := validationContext.Events[eventID]
				if !chapterSeen[meta.ChapterID] {
					chapterSeen[meta.ChapterID] = true
					chapters = append(chapters, meta.ChapterID)
				}
				characterIDs = append(characterIDs, meta.ParticipantIDs...)
				arcIDs = append(arcIDs, meta.ArcIDs...)
			}
		}
		sourceEvents, _ := json.Marshal(eventIDs)
		sourceChapters, _ := json.Marshal(chapters)
		addedJSON, _ := json.Marshal(added)
		mergedJSON, _ := json.Marshal(merged)
		deviationJSON, _ := json.Marshal(deviations)
		continuityInValues, continuityOutValues := episode.ContinuityIn, episode.ContinuityOut
		if continuityInValues == nil {
			continuityInValues = []string{}
		}
		if continuityOutValues == nil {
			continuityOutValues = []string{}
		}
		continuityIn, _ := json.Marshal(continuityInValues)
		continuityOut, _ := json.Marshal(continuityOutValues)
		characterJSON, _ := json.Marshal(seasonUniqueStrings(characterIDs))
		arcJSON, _ := json.Marshal(seasonUniqueStrings(arcIDs))
		episodeHash, _ := hashJSON(episode)
		_, err = tx.Exec(ctx, `INSERT INTO drama.adaptation_episode_plans(adaptation_episode_plan_id,adaptation_plan_id,episode_number,title,logline,
			estimated_duration_seconds,opening_hook,ending_hook,continuity_in,continuity_out,validation_report,content_hash,
			source_event_ids,source_chapter_ids,added_adaptation_content,merged_content,deviation_notes,three_second_opening,
			first_thirty_seconds_goal,core_conflict,climax,emotion_curve,information_reveal_amount,character_arc_entity_ids,story_arc_revision_ids)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11::jsonb,$12,$13::jsonb,$14::jsonb,$15::jsonb,$16::jsonb,$17::jsonb,
			$18,$19,$20,$21,$22::jsonb,$23,$24::jsonb,$25::jsonb)`, episodeID, planID, episode.EpisodeNumber, episode.Title, episode.Logline,
			episode.EstimatedDurationSeconds, episode.ThreeSecondOpening, episode.EndingHook, continuityIn, continuityOut, validationJSON, episodeHash,
			sourceEvents, sourceChapters, addedJSON, mergedJSON, deviationJSON, episode.ThreeSecondOpening, episode.FirstThirtySecondsGoal,
			episode.CoreConflict, episode.Climax, episode.EmotionCurve, episode.InformationRevealAmount, characterJSON, arcJSON)
		if err != nil {
			return nil, "", err
		}
		sequence := 0
		for _, card := range episode.Events {
			for _, eventID := range card.SourceEventIDs {
				if !eventSeen[eventID] {
					continue
				}
				eventSeen[eventID] = false
				sequence++
				usage := "preserve"
				if card.PresentationMode == "merge" {
					usage = "merge"
				} else if card.PresentationMode == "transform" {
					usage = "transform"
				} else if card.PresentationMode == "split" {
					usage = "split"
				}
				assignmentID, _ := newPublicID("eea_")
				ruleIDs := matchingRuleIDs(validationContext, []string{eventID}, map[string]string{"merge": "merge_allowed", "transform": "transform_required"}[card.PresentationMode])
				ruleJSON, _ := json.Marshal(ruleIDs)
				var mergeID any = nil
				if usage == "merge" {
					mergeID = card.MergeGroupID
				}
				_, err = tx.Exec(ctx, `INSERT INTO drama.episode_event_assignments(episode_event_assignment_id,adaptation_episode_plan_id,event_revision_id,sequence_number,usage_mode,merge_group_id,rule_trace,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8)`, assignmentID, episodeID, eventID, sequence, usage, mergeID, ruleJSON, "season-workbench:"+planID+":"+fmt.Sprint(episode.EpisodeNumber)+":"+eventID)
				if err != nil {
					return nil, "", err
				}
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.adaptation_plans SET status='waiting_review',updated_at=CURRENT_TIMESTAMP WHERE adaptation_plan_id=$1`, planID); err != nil {
		return nil, "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.change_plans
		SET status='applied',applied_at=now(),
			review_metadata=review_metadata||jsonb_build_object('result_adaptation_plan_id',$2::text)
		WHERE change_plan_id=$1`, changePlanID, planID); err != nil {
		return nil, "", err
	}
	appliedEventID, err := newPublicID("cpe_")
	if err != nil {
		return nil, "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO drama.change_plan_events(
		change_plan_event_id,change_plan_id,event_type,actor,details)
		VALUES($1,$2,'applied','season-workbench',jsonb_build_object('adaptation_plan_id',$3::text))`,
		appliedEventID, changePlanID, planID); err != nil {
		return nil, "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, "", err
	}
	return s.GetAdaptationPlan(ctx, planID)
}

func (s *Store) PreviewSeasonPlanChange(
	ctx context.Context, basePlanID string, draft SeasonPlanDraft,
) (SeasonPlanChangePreview, error) {
	validation, err := s.ValidateSeasonPlanDraft(ctx, basePlanID, draft)
	if err != nil {
		return SeasonPlanChangePreview{}, err
	}
	var projectID, baseHash string
	var baseVersion, latestVersion int
	var baseSnapshot json.RawMessage
	err = s.pool.QueryRow(ctx, `SELECT base.project_id,base.version_number,base.content_hash,
		(SELECT max(version_number) FROM drama.adaptation_plans
		WHERE project_id=base.project_id)
		FROM drama.adaptation_plans base WHERE base.adaptation_plan_id=$1`, basePlanID).
		Scan(&projectID, &baseVersion, &baseHash, &latestVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return SeasonPlanChangePreview{}, ErrNotFound
	}
	if err != nil {
		return SeasonPlanChangePreview{}, err
	}
	if baseVersion != latestVersion {
		return SeasonPlanChangePreview{}, fmt.Errorf("%w: adaptation plan base version %d is stale; current latest version is %d", ErrConflict, baseVersion, latestVersion)
	}
	if err = s.pool.QueryRow(ctx, seasonPlanDraftSnapshotSQL, basePlanID).Scan(&baseSnapshot); err != nil {
		return SeasonPlanChangePreview{}, err
	}
	draft.PreviewFingerprint = ""
	fingerprint, err := hashJSON(map[string]any{
		"base_adaptation_plan_id": basePlanID, "base_version": baseVersion,
		"base_content_hash": baseHash, "draft": draft,
	})
	if err != nil {
		return SeasonPlanChangePreview{}, err
	}
	var before map[string]any
	var after map[string]any
	_ = json.Unmarshal(baseSnapshot, &before)
	afterRaw, _ := json.Marshal(draft)
	_ = json.Unmarshal(afterRaw, &after)
	diff := []map[string]any{}
	for _, field := range []string{"plan_name", "strategy_label", "episodes", "omitted_events", "creative_suggestions"} {
		if !reflect.DeepEqual(normalizeJSONValue(before[field]), normalizeJSONValue(after[field])) {
			diff = append(diff, map[string]any{"field": field, "before": before[field], "after": after[field]})
		}
	}
	stale := []map[string]any{}
	rows, queryErr := s.pool.Query(ctx, `WITH RECURSIVE walk AS (
		SELECT artifact.artifact_id,artifact.artifact_type,artifact.native_entity_id,0 depth
		FROM drama.artifacts artifact WHERE artifact.project_id=$1
		  AND artifact.artifact_type='adaptation_plan' AND artifact.native_entity_id=$2 AND artifact.is_current
		UNION ALL
		SELECT child.artifact_id,child.artifact_type,child.native_entity_id,walk.depth+1
		FROM walk JOIN drama.artifact_dependencies dependency ON dependency.upstream_artifact_id=walk.artifact_id
		JOIN drama.artifacts child ON child.artifact_id=dependency.downstream_artifact_id AND child.is_current)
		SELECT DISTINCT artifact_id,artifact_type,native_entity_id,depth FROM walk WHERE depth>0
		ORDER BY depth,artifact_type,artifact_id`, projectID, basePlanID)
	if queryErr != nil {
		return SeasonPlanChangePreview{}, queryErr
	}
	for rows.Next() {
		var id, kind, native string
		var depth int
		if err = rows.Scan(&id, &kind, &native, &depth); err != nil {
			rows.Close()
			return SeasonPlanChangePreview{}, err
		}
		stale = append(stale, map[string]any{"artifact_id": id, "artifact_type": kind,
			"native_entity_id": native, "depth": depth, "after_status": "stale"})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return SeasonPlanChangePreview{}, err
	}
	changes := make([]localedit.Change, 0, len(diff))
	allowedFields := make([]string, 0, len(diff))
	for _, item := range diff {
		field := fmt.Sprint(item["field"])
		changes = append(changes, localedit.Change{Operation: "replace", Field: field, Value: item["after"]})
		allowedFields = append(allowedFields, field)
	}
	plan, err := localedit.Build(localedit.Request{
		Instruction: "确认整季策划变更并创建不可变 adaptation plan successor",
		Target: localedit.Target{EntityType: "adaptation_plan", EntityID: basePlanID,
			Version: baseVersion},
		Changes: changes, AllowedFields: allowedFields,
		MustPreserve: []string{"source_event_ids", "causal_order", "provenance", "approved current until confirmation"},
		Locks:        []string{"source_event_ids", "causal_order", "provenance"},
	})
	if err != nil {
		return SeasonPlanChangePreview{}, err
	}
	changePlan, err := s.CreateChangePlan(ctx, projectID, plan, nil)
	if err != nil {
		return SeasonPlanChangePreview{}, err
	}
	reviewMetadata, _ := json.Marshal(map[string]any{
		"preview_fingerprint": fingerprint, "base_content_hash": baseHash,
		"season_draft": draft, "validation": validation, "stale_scope": stale,
	})
	changePlan, err = s.SetChangePlanReviewMetadata(ctx, projectID, changePlan.ChangePlanID, reviewMetadata)
	if err != nil {
		return SeasonPlanChangePreview{}, err
	}
	return SeasonPlanChangePreview{ChangePlanID: changePlan.ChangePlanID, Status: changePlan.Status,
		BaseAdaptationPlanID: basePlanID, BaseVersion: baseVersion,
		BaseContentHash: baseHash, PreviewFingerprint: fingerprint, Diff: diff,
		StaleScope: stale, Validation: validation}, nil
}

func seasonUniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
func matchingRuleIDs(context seasonValidationContext, eventIDs []string, ruleType string) []string {
	if ruleType == "" {
		return []string{}
	}
	result := []string{}
	for _, rule := range context.Rules {
		if rule.RuleType != ruleType {
			continue
		}
		matches := true
		for _, id := range eventIDs {
			matches = matches && ruleMatchesSeasonEvent(rule, context.Events[id])
		}
		if matches {
			result = append(result, rule.ID)
		}
	}
	return seasonUniqueStrings(result)
}

func (s *Store) ApproveSeasonPlan(ctx context.Context, adaptationPlanID, approvedBy string) (SeasonApprovalResult, error) {
	var raw json.RawMessage
	err := s.pool.QueryRow(ctx, `SELECT drama.validate_adaptation_plan_for_approval($1)`, adaptationPlanID).Scan(&raw)
	if err != nil {
		return SeasonApprovalResult{}, err
	}
	var validation SeasonValidationResult
	if err = json.Unmarshal(raw, &validation); err != nil {
		return SeasonApprovalResult{}, err
	}
	result := SeasonApprovalResult{AdaptationPlanID: adaptationPlanID, Status: "waiting_review", QueueCreated: false, Validation: validation}
	if !validation.Passed {
		return result, ErrValidation
	}
	tx, err := s.writer.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var projectID, status, contentHash string
	err = tx.QueryRow(ctx, `SELECT project_id,status,content_hash FROM drama.adaptation_plans WHERE adaptation_plan_id=$1 FOR UPDATE`, adaptationPlanID).Scan(&projectID, &status, &contentHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if status == "approved" {
		result.Status = "approved"
		_ = tx.Commit(ctx)
		return result, nil
	}
	if status != "waiting_review" {
		return result, ErrConflict
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, projectID); err != nil {
		return result, err
	}
	runID, _ := newPublicID("apvr_")
	checks, _ := json.Marshal(validation.Checks)
	diagnostics, _ := json.Marshal(validation.Diagnostics)
	_, err = tx.Exec(ctx, `INSERT INTO drama.adaptation_plan_validation_runs(validation_run_id,adaptation_plan_id,validation_scope,passed,validator_version,checks,diagnostics,input_hash) VALUES($1,$2,'approval',true,$3,$4::jsonb,$5::jsonb,$6)`, runID, adaptationPlanID, validation.ValidatorVersion, checks, diagnostics, contentHash)
	if err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.adaptation_plans SET status='superseded',is_current=false,updated_at=CURRENT_TIMESTAMP WHERE project_id=$1 AND status='approved' AND adaptation_plan_id<>$2`, projectID, adaptationPlanID); err != nil {
		return result, err
	}
	if _, err = tx.Exec(ctx, `UPDATE drama.adaptation_plans SET status='approved',is_current=true,approved_by=$2,approved_at=CURRENT_TIMESTAMP,validation_run_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE adaptation_plan_id=$1`, adaptationPlanID, strings.TrimSpace(approvedBy)); err != nil {
		return result, err
	}
	if err = publishAdaptationPlanArtifacts(ctx, tx, projectID, adaptationPlanID); err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	result.Status = "approved"
	return result, nil
}
