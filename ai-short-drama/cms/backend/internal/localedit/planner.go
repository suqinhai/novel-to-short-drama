package localedit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const SchemaVersion = "change-plan.v1"

var ErrInvalidPlan = errors.New("invalid change plan")

type Target struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Version    int    `json:"version"`
}

type Change struct {
	Operation string `json:"operation"`
	Field     string `json:"field,omitempty"`
	Value     any    `json:"value,omitempty"`
	Delta     any    `json:"delta,omitempty"`
	StartMS   *int64 `json:"start_ms,omitempty"`
	EndMS     *int64 `json:"end_ms,omitempty"`
	Before    any    `json:"before,omitempty"`
	After     any    `json:"after,omitempty"`
}

type RebuildDecision struct {
	Voice      bool `json:"voice"`
	Image      bool `json:"image"`
	Video      bool `json:"video"`
	Edit       bool `json:"edit"`
	Subtitle   bool `json:"subtitle"`
	Continuity bool `json:"continuity"`
}

type Impact struct {
	Upstream     []string `json:"upstream"`
	Downstream   []string `json:"downstream"`
	RebuildTasks []string `json:"rebuild_tasks"`
}

type Plan struct {
	SchemaVersion   string          `json:"schema_version"`
	UserIntent      string          `json:"user_intent"`
	Target          Target          `json:"target"`
	MustPreserve    []string        `json:"must_preserve"`
	AllowedFields   []string        `json:"allowed_fields"`
	ExpectedChanges []Change        `json:"expected_changes"`
	Impact          Impact          `json:"impact"`
	Rebuild         RebuildDecision `json:"rebuild"`
	Risks           []string        `json:"risks"`
	ValidationRules []string        `json:"validation_rules"`
	RollbackVersion int             `json:"rollback_version"`
	ChangeKind      string          `json:"change_kind"`
	SemanticChange  bool            `json:"semantic_change"`
	Locks           []string        `json:"locks"`
}

type Request struct {
	Instruction    string   `json:"instruction"`
	Target         Target   `json:"target"`
	MustPreserve   []string `json:"must_preserve,omitempty"`
	AllowedFields  []string `json:"allowed_fields,omitempty"`
	Changes        []Change `json:"changes,omitempty"`
	ChangeKind     string   `json:"change_kind,omitempty"`
	SemanticChange *bool    `json:"semantic_change,omitempty"`
	RebuildTasks   []string `json:"rebuild_tasks,omitempty"`
	Locks          []string `json:"locks,omitempty"`
}

var allowedFields = map[string]map[string]bool{
	"dialogue": {
		"text": true, "emotion": true, "performance_instruction": true,
		"estimated_duration_ms": true, "requested_speed": true, "production_mode": true,
		"dialogue_type": true, "speaker_name": true, "sequence_number": true,
	},
	"scene": {
		"scene_purpose": true, "actions": true, "emotional_change": true,
		"estimated_duration_seconds": true, "scene_number": true,
		"location_name": true, "time_of_day": true, "interior_exterior": true,
		"character_ids": true,
	},
	"shot": {
		"action_description": true, "facial_expression": true, "composition": true,
		"shot_size": true, "camera_angle": true, "camera_motion": true,
		"duration_seconds": true, "shot_order": true,
	},
	"shot_video": {
		"segment": true, "video_prompt": true, "action_description": true,
	},
	"media": {
		"source_url": true, "storage_url": true, "content_hash": true,
	},
	"outline": {
		"title": true, "logline": true, "opening_hook": true, "story_goal": true,
		"main_conflict": true, "climax": true, "ending_hook": true,
		"estimated_duration_seconds": true,
	},
	"script": {
		"title": true, "opening_hook": true, "climax": true, "ending_hook": true,
	},
	"adaptation_spec": {
		"platform": true, "audience_profile": true, "target_episode_count": true,
		"episode_duration_seconds": true, "scope_mode": true, "chapter_ids": true,
		"story_arc_revision_ids": true, "rules": true,
	},
	"adaptation_plan": {
		"quality_report": true, "plan_name": true, "strategy_label": true,
		"episodes": true, "omitted_events": true, "creative_suggestions": true,
	},
	"pacing": {
		"total_duration_seconds": true, "story_arcs": true, "episodes": true, "beats": true,
	},
	"performance_bible": {
		"speech": true, "acting": true, "relational_voices": true, "appearance": true,
		"locked_fields": true, "allowed_fields": true, "change_reasons": true, "source_refs": true,
	},
	"continuity": {
		"input_state": true, "output_state": true, "validation_status": true, "diagnostics": true,
	},
	"episode_content": {"*": true},
	"timeline": {
		"editing_template_version_id": true, "template_scope": true,
		"override_config": true, "sound_style_group": true,
		"restore_source_timeline_id": true, "items": true,
	},
	"timeline_item": {
		"timeline_start_ms": true, "timeline_end_ms": true, "source_in_ms": true,
		"source_out_ms": true, "duration_ms": true, "source_url": true,
	},
}

func Build(req Request) (Plan, error) {
	req.Instruction = strings.TrimSpace(req.Instruction)
	req.Target.EntityType = normalizeEntityType(req.Target.EntityType)
	req.Target.EntityID = strings.TrimSpace(req.Target.EntityID)
	if req.Instruction == "" || req.Target.EntityType == "" || req.Target.EntityID == "" || req.Target.Version < 1 {
		return Plan{}, fmt.Errorf("%w: instruction and versioned target are required", ErrInvalidPlan)
	}
	if allowedFields[req.Target.EntityType] == nil {
		return Plan{}, fmt.Errorf("%w: unsupported target type %s", ErrInvalidPlan, req.Target.EntityType)
	}
	if req.Target.EntityType == "shot" && isStructuralShotInstruction(req.Instruction) {
		return Plan{}, fmt.Errorf("%w: split, merge and reorder must use the atomic shot-sequence editor", ErrInvalidPlan)
	}

	changes, inferredPreserve, locks, inferredKind := interpret(req)
	locks = append(locks, req.Locks...)
	if len(req.Changes) > 0 {
		changes = req.Changes
	}
	if len(changes) == 0 {
		return Plan{}, fmt.Errorf("%w: instruction did not produce an executable change", ErrInvalidPlan)
	}

	fields := append([]string(nil), req.AllowedFields...)
	if len(fields) == 0 {
		for _, change := range changes {
			if change.Field != "" && !contains(fields, change.Field) {
				fields = append(fields, change.Field)
			}
		}
	}
	for _, field := range fields {
		if !isAllowedField(req.Target.EntityType, field) {
			return Plan{}, fmt.Errorf("%w: field %s cannot change on %s", ErrInvalidPlan, field, req.Target.EntityType)
		}
	}
	for _, change := range changes {
		if err := validateChange(req.Target.EntityType, change, fields); err != nil {
			return Plan{}, err
		}
	}

	semantic := true
	if req.SemanticChange != nil {
		semantic = *req.SemanticChange
	}
	kind := strings.TrimSpace(req.ChangeKind)
	if kind == "" {
		kind = inferredKind
	}
	if kind == "" {
		kind = "content_changed"
	}
	if kind == "format_changed" || kind == "source_relocated" {
		semantic = false
	}

	preserve := unique(append(append([]string(nil), req.MustPreserve...), inferredPreserve...))
	rebuild, impact := decideImpact(req.Target.EntityType, fields, changes, semantic)
	if req.RebuildTasks != nil {
		selected := unique(req.RebuildTasks)
		for _, action := range selected {
			if !contains(impact.RebuildTasks, action) {
				return Plan{}, fmt.Errorf("%w: rebuild action %s is outside the calculated impact", ErrInvalidPlan, action)
			}
		}
		impact.RebuildTasks = selected
		rebuild = decisionForActions(selected)
	}
	plan := Plan{
		SchemaVersion: SchemaVersion, UserIntent: req.Instruction, Target: req.Target,
		MustPreserve: preserve, AllowedFields: unique(fields), ExpectedChanges: changes,
		Impact: impact, Rebuild: rebuild, Risks: risksFor(req.Target.EntityType, changes),
		ValidationRules: validationFor(req.Target.EntityType, preserve, locks),
		RollbackVersion: req.Target.Version, ChangeKind: kind, SemanticChange: semantic,
		Locks: unique(locks),
	}
	if err := Validate(plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func Validate(plan Plan) error {
	if plan.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: unsupported schema version", ErrInvalidPlan)
	}
	if plan.UserIntent == "" || plan.Target.EntityID == "" || plan.Target.Version < 1 {
		return fmt.Errorf("%w: incomplete intent or target", ErrInvalidPlan)
	}
	if len(plan.AllowedFields) == 0 || len(plan.ExpectedChanges) == 0 || len(plan.ValidationRules) == 0 {
		return fmt.Errorf("%w: fields, changes and validation rules are required", ErrInvalidPlan)
	}
	for _, change := range plan.ExpectedChanges {
		if err := validateChange(plan.Target.EntityType, change, plan.AllowedFields); err != nil {
			return err
		}
	}
	return nil
}

func Fingerprint(plan Plan) string {
	data, _ := json.Marshal(plan)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func interpret(req Request) ([]Change, []string, []string, string) {
	text := req.Instruction
	var changes []Change
	var preserve, locks []string
	kind := "content_changed"

	if strings.Contains(text, "不要改变剧情") || strings.Contains(text, "不改变剧情") {
		preserve = append(preserve, "剧情事实", "人物关系", "因果顺序")
	}
	if strings.Contains(text, "身份揭露") && strings.Contains(text, "保留") {
		preserve = append(preserve, "身份揭露")
	}
	if strings.Contains(text, "不要闭合伏笔") || strings.Contains(text, "不闭合伏笔") {
		preserve = append(preserve, "伏笔保持开放")
	}
	if strings.Contains(text, "保留人物") || strings.Contains(text, "锁定人物") {
		locks = append(locks, "character")
	}
	if strings.Contains(text, "保留场景") || strings.Contains(text, "锁定场景") ||
		(strings.Contains(text, "保留人物") && strings.Contains(text, "场景")) {
		locks = append(locks, "location")
	}
	if strings.Contains(text, "锁定服装") || strings.Contains(text, "保留服装") {
		locks = append(locks, "costume")
	}
	if strings.Contains(text, "锁定构图") || strings.Contains(text, "保留构图") {
		locks = append(locks, "composition")
	}

	switch req.Target.EntityType {
	case "dialogue":
		if strings.Contains(text, "克制") {
			changes = append(changes, Change{Operation: "replace", Field: "emotion", Value: "克制"})
		}
		if strings.Contains(text, "语速") {
			speed := 0.9
			if strings.Contains(text, "更快") || strings.Contains(text, "加快") {
				speed = 1.15
			}
			changes = append(changes, Change{Operation: "replace", Field: "requested_speed", Value: speed})
		}
		if strings.Contains(text, "转为旁白") || strings.Contains(text, "转换为旁白") {
			changes = append(changes, Change{Operation: "replace", Field: "production_mode", Value: "narration"})
		}
		if strings.Contains(text, "转为动作") || strings.Contains(text, "转换为动作") {
			changes = append(changes, Change{Operation: "replace", Field: "production_mode", Value: "action"})
		}
	case "scene":
		if match := regexp.MustCompile(`缩短\s*(\d+)\s*秒`).FindStringSubmatch(text); len(match) == 2 {
			n, _ := strconv.Atoi(match[1])
			changes = append(changes, Change{Operation: "adjust", Field: "estimated_duration_seconds", Delta: -n})
		}
		if strings.Contains(text, "减少旁白") {
			changes = append(changes, Change{Operation: "adjust", Field: "actions", Value: map[string]any{"reduce_narration": true}})
		}
		if strings.Contains(text, "冲突") && (strings.Contains(text, "强化") || strings.Contains(text, "不够强")) {
			changes = append(changes, Change{Operation: "adjust", Field: "actions", Value: map[string]any{"strengthen_conflict": true, "window_ms": 10000}})
		}
		if strings.Contains(text, "提前") {
			changes = append(changes, Change{Operation: "reorder", Field: "scene_number", Delta: -1})
		}
	case "shot":
		if strings.Contains(text, "动作") {
			changes = append(changes, Change{Operation: "regenerate", Field: "action_description", Value: strings.TrimSpace(text)})
		}
	case "shot_video":
		start, end := int64(0), int64(0)
		if match := regexp.MustCompile(`前\s*(\d+)\s*秒`).FindStringSubmatch(text); len(match) == 2 {
			seconds, _ := strconv.ParseInt(match[1], 10, 64)
			end = seconds * 1000
		}
		if end == 0 {
			end = 10000
		}
		changes = append(changes, Change{
			Operation: "regenerate_segment", Field: "segment", Value: strings.TrimSpace(text),
			StartMS: &start, EndMS: &end,
		})
	}
	return changes, preserve, locks, kind
}

func validateChange(entityType string, change Change, allowed []string) error {
	if change.Operation == "" {
		return fmt.Errorf("%w: change operation is required", ErrInvalidPlan)
	}
	if change.Field != "" && (!isAllowedField(entityType, change.Field) || !contains(allowed, change.Field)) {
		return fmt.Errorf("%w: change field %s is not allowed", ErrInvalidPlan, change.Field)
	}
	switch change.Operation {
	case "replace", "adjust", "reorder", "regenerate", "manual_replace", "insert", "remove":
	case "regenerate_segment":
		if change.StartMS == nil || change.EndMS == nil || *change.StartMS < 0 || *change.EndMS <= *change.StartMS {
			return fmt.Errorf("%w: a valid video time range is required", ErrInvalidPlan)
		}
	default:
		return fmt.Errorf("%w: unsupported operation %s", ErrInvalidPlan, change.Operation)
	}
	return nil
}

func decideImpact(entityType string, fields []string, changes []Change, semantic bool) (RebuildDecision, Impact) {
	if !semantic {
		return RebuildDecision{}, Impact{Upstream: []string{}, Downstream: []string{}, RebuildTasks: []string{}}
	}
	var rebuild RebuildDecision
	var downstream []string
	switch entityType {
	case "dialogue":
		rebuild.Subtitle, rebuild.Video, rebuild.Edit = true, true, true
		downstream = append(downstream, "subtitle_cue", "storyboard_shot_interval", "edit_timeline_interval")
		if contains(fields, "text") || contains(fields, "emotion") || contains(fields, "performance_instruction") || contains(fields, "requested_speed") {
			rebuild.Voice = true
			downstream = append(downstream, "dialogue_audio")
		}
	case "scene":
		rebuild.Video, rebuild.Edit, rebuild.Continuity = true, true, true
		downstream = append(downstream, "adjacent_continuity", "storyboard_shot_interval", "edit_timeline_interval")
	case "shot":
		rebuild.Image, rebuild.Video, rebuild.Edit = true, true, true
		downstream = append(downstream, "storyboard_image", "shot_video", "edit_timeline_item")
	case "shot_video":
		rebuild.Video, rebuild.Edit = true, true
		downstream = append(downstream, "shot_video_segment", "edit_timeline_item")
	case "media":
		rebuild.Edit = true
		downstream = append(downstream, "edit_timeline_item")
	case "outline", "script", "adaptation_spec", "adaptation_plan", "pacing", "performance_bible", "continuity":
		rebuild.Voice, rebuild.Subtitle, rebuild.Image = true, true, true
		rebuild.Video, rebuild.Edit, rebuild.Continuity = true, true, true
		downstream = append(downstream, "episode_script", "storyboard", "dialogue_audio",
			"subtitle_cue", "storyboard_image", "shot_video", "edit_timeline")
	case "episode_content":
		for _, field := range fields {
			switch {
			case strings.HasPrefix(field, "dialogue."):
				rebuild.Voice, rebuild.Subtitle, rebuild.Video, rebuild.Edit = true, true, true, true
				downstream = append(downstream, "dialogue_audio", "subtitle_cue",
					"storyboard_shot_interval", "edit_timeline_interval")
			case strings.HasPrefix(field, "scene."):
				rebuild.Video, rebuild.Edit, rebuild.Continuity = true, true, true
				downstream = append(downstream, "adjacent_continuity",
					"storyboard_shot_interval", "edit_timeline_interval")
			default:
				rebuild.Voice, rebuild.Subtitle, rebuild.Image = true, true, true
				rebuild.Video, rebuild.Edit, rebuild.Continuity = true, true, true
				downstream = append(downstream, "episode_script", "storyboard", "dialogue_audio",
					"subtitle_cue", "storyboard_image", "shot_video", "edit_timeline")
			}
		}
	case "timeline", "timeline_item":
		rebuild.Edit = true
		downstream = append(downstream, "edit_timeline", "episode_master")
	}
	tasks := make([]string, 0, 5)
	if rebuild.Voice {
		tasks = append(tasks, "regenerate_voice")
	}
	if rebuild.Subtitle {
		tasks = append(tasks, "update_subtitle")
	}
	if rebuild.Image {
		tasks = append(tasks, "regenerate_image")
	}
	if rebuild.Video {
		tasks = append(tasks, "regenerate_video")
	}
	if rebuild.Edit {
		tasks = append(tasks, "recompose_timeline")
	}
	if rebuild.Continuity {
		tasks = append(tasks, "update_continuity")
	}
	return rebuild, Impact{Upstream: []string{}, Downstream: unique(downstream), RebuildTasks: tasks}
}

func decisionForActions(actions []string) RebuildDecision {
	var result RebuildDecision
	for _, action := range actions {
		switch action {
		case "regenerate_voice":
			result.Voice = true
		case "update_subtitle":
			result.Subtitle = true
		case "regenerate_image":
			result.Image = true
		case "regenerate_video":
			result.Video = true
		case "recompose_timeline":
			result.Edit = true
		case "update_continuity":
			result.Continuity = true
		}
	}
	return result
}

func isAllowedField(entityType, field string) bool {
	fields := allowedFields[entityType]
	if fields == nil {
		return false
	}
	if fields[field] {
		return true
	}
	if !fields["*"] || entityType != "episode_content" {
		return false
	}
	parts := strings.Split(field, ".")
	if len(parts) == 2 && parts[0] == "outline" {
		return allowedFields["outline"][parts[1]]
	}
	if len(parts) == 2 && parts[0] == "script" {
		return allowedFields["script"][parts[1]]
	}
	if len(parts) == 3 && parts[0] == "scene" {
		return strings.TrimSpace(parts[1]) != "" && allowedFields["scene"][parts[2]]
	}
	if len(parts) == 3 && parts[0] == "dialogue" {
		return strings.TrimSpace(parts[1]) != "" && (allowedFields["dialogue"][parts[2]] ||
			parts[2] == "dialogue_type" || parts[2] == "speaker_name")
	}
	if len(parts) == 2 && (parts[0] == "scene" || parts[0] == "dialogue") {
		return strings.TrimSpace(parts[1]) != ""
	}
	return false
}

func validationFor(entityType string, preserve, locks []string) []string {
	rules := []string{
		"目标版本仍为 current",
		"所有变更字段均在 allowed_fields 中",
		"正式数据仅在计划 confirmed 后写入",
		"写入、失效与 current 切换必须在同一事务完成",
	}
	if len(preserve) > 0 {
		rules = append(rules, "must_preserve 事实的规范化指纹不得变化")
	}
	if len(locks) > 0 {
		rules = append(rules, "锁定元素的引用与内容哈希不得变化")
	}
	if entityType == "shot_video" {
		rules = append(rules, "重建时间段必须位于源视频时长内")
	}
	return rules
}

func risksFor(entityType string, changes []Change) []string {
	risks := []string{"目标可能已被其他修改 supersede"}
	if entityType == "dialogue" {
		risks = append(risks, "配音时长变化可能造成字幕或剪辑漂移")
	}
	if entityType == "scene" {
		risks = append(risks, "场景缩短或换序可能破坏节奏与连续性")
	}
	if entityType == "shot" || entityType == "shot_video" {
		risks = append(risks, "局部重建可能出现人物或运动连续性偏差")
	}
	return risks
}

func normalizeEntityType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "line", "dialog":
		return "dialogue"
	case "script_scene":
		return "scene"
	case "storyboard_shot":
		return "shot"
	case "video":
		return "shot_video"
	case "episode", "episode_revision":
		return "episode_content"
	case "edit_timeline":
		return "timeline"
	case "action":
		return "scene"
	case "adaptation_spec_version":
		return "adaptation_spec"
	case "adaptation_plan_version":
		return "adaptation_plan"
	case "pacing_plan", "pacing_plan_version":
		return "pacing"
	case "character_performance_bible":
		return "performance_bible"
	case "continuity_entry":
		return "continuity"
	case "post_production_config":
		return "timeline"
	default:
		return value
	}
}

func isStructuralShotInstruction(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, marker := range []string{"拆分镜头", "拆成两镜", "合并镜头", "合并相邻", "镜头换序", "镜头重排", "split shot", "merge shot", "reorder shot"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func unique(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
