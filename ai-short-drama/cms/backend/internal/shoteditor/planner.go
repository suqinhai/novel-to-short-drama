package shoteditor

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

var ErrInvalidEdit = errors.New("invalid shot edit")

var coverageKinds = []struct{ Key, Label string }{
	{"establishing", "建立镜头"}, {"action", "动作镜头"}, {"reaction", "反应镜头"},
	{"shot_reverse", "正反打"}, {"insert_closeup", "插入特写"},
}

var editableFields = map[string]bool{
	"shot_size": true, "camera_angle": true, "composition": true, "camera_motion": true,
	"performance": true, "facial_expression": true, "action_description": true,
	"action_phase": true, "duration_seconds": true, "dialogue_ids": true,
	"head_state": true, "tail_state": true, "axis": true, "coverage_role": true,
	"coverage_group": true, "coverage_side": true, "character_ids": true,
	"location_id": true, "subtitle_text": true, "narration_text": true,
}

func Build(base []Shot, request Request) (Preview, error) {
	request.Operation = strings.ToLower(strings.TrimSpace(request.Operation))
	if len(base) == 0 && request.Operation != OperationRestore {
		return Preview{}, fmt.Errorf("%w: current shot sequence is empty", ErrInvalidEdit)
	}
	shots := CloneShots(base)
	preview := Preview{Operation: request.Operation, Conflicts: []Conflict{}, Coverage: []CoverageCheck{}, Handoffs: []Handoff{}, ChangedIDs: []string{}, RetiredIDs: []string{}, CreatedIDs: []string{}}
	var err error
	switch request.Operation {
	case OperationSplit:
		shots, preview.CreatedIDs, preview.RetiredIDs, err = split(shots, request)
	case OperationMerge:
		shots, preview.CreatedIDs, preview.RetiredIDs, err = merge(shots, request)
	case OperationReorder:
		shots, err = reorder(shots, request.OrderedShotIDs)
	case OperationUpdate:
		shots, err = update(shots, request.ShotID, request.Patch)
	case OperationRestore:
		if len(request.RestoreSnapshot) == 0 {
			err = fmt.Errorf("%w: restore source snapshot is empty", ErrInvalidEdit)
		} else {
			shots = CloneShots(request.RestoreSnapshot)
		}
	default:
		err = fmt.Errorf("%w: unsupported operation %q", ErrInvalidEdit, request.Operation)
	}
	if err != nil {
		return Preview{}, err
	}
	normalizeSequence(shots)
	preview.Shots = shots
	preview.ChangedIDs, preview.CreatedIDs, preview.RetiredIDs = diffIDs(base, shots, preview.CreatedIDs, preview.RetiredIDs)
	preview.Conflicts = append(preview.Conflicts, validateShotContent(shots, request.DialogueDurationsMS)...)
	preview.Conflicts = append(preview.Conflicts, ValidateContinuity(shots)...)
	preview.Coverage = CheckCoverage(shots, request.RequiredCoverage)
	for _, check := range preview.Coverage {
		if check.Required && !check.Passed {
			preview.Conflicts = append(preview.Conflicts, Conflict{
				Code: "COVERAGE_MISSING", Severity: "blocking", Message: check.Label + "缺失",
				Details: map[string]any{"scene_id": check.SceneID, "coverage_kind": check.Kind},
			})
		}
	}
	preview.Handoffs = BuildHandoffs(shots, preview.Conflicts)
	return preview, nil
}

func split(base []Shot, request Request) ([]Shot, []string, []string, error) {
	if len(request.Shots) < 2 || len(request.Shots) != len(request.NewShotIDs) {
		return nil, nil, nil, fmt.Errorf("%w: split requires at least two shot drafts and one new id per draft", ErrInvalidEdit)
	}
	index := shotIndex(base, request.ShotID)
	if index < 0 {
		return nil, nil, nil, fmt.Errorf("%w: split source shot was not found", ErrInvalidEdit)
	}
	source := base[index]
	parts := CloneShots(request.Shots)
	for i := range parts {
		inheritShot(&parts[i], source)
		parts[i].ShotID = request.NewShotIDs[i]
		parts[i].LineageRootShotID = source.LineageRootShotID
		if parts[i].LineageRootShotID == "" {
			parts[i].LineageRootShotID = source.ShotID
		}
		parts[i].GenerationVersion, parts[i].Version = 1, 1
	}
	totalDuration := 0.0
	dialoguePartition := []string{}
	for _, part := range parts {
		totalDuration += part.DurationSeconds
		dialoguePartition = append(dialoguePartition, part.DialogueIDs...)
	}
	if math.Abs(totalDuration-source.DurationSeconds) > .011 {
		return nil, nil, nil, fmt.Errorf("%w: split durations must equal the source duration", ErrInvalidEdit)
	}
	if !sameOrderedSet(dialoguePartition, source.DialogueIDs) || hasDuplicates(dialoguePartition) {
		return nil, nil, nil, fmt.Errorf("%w: split dialogues must form an exact, non-overlapping partition", ErrInvalidEdit)
	}
	if !reflect.DeepEqual(normalizeState(parts[0].HeadState), normalizeState(source.HeadState)) ||
		!reflect.DeepEqual(normalizeState(parts[len(parts)-1].TailState), normalizeState(source.TailState)) {
		return nil, nil, nil, fmt.Errorf("%w: split must preserve the source head and tail state", ErrInvalidEdit)
	}
	result := append([]Shot{}, base[:index]...)
	result = append(result, parts...)
	result = append(result, base[index+1:]...)
	return result, request.NewShotIDs, []string{source.ShotID}, nil
}

func merge(base []Shot, request Request) ([]Shot, []string, []string, error) {
	if len(request.ShotIDs) < 2 || len(request.Shots) != 1 || len(request.NewShotIDs) != 1 {
		return nil, nil, nil, fmt.Errorf("%w: merge requires at least two sources and one result", ErrInvalidEdit)
	}
	firstIndex := shotIndex(base, request.ShotIDs[0])
	if firstIndex < 0 {
		return nil, nil, nil, fmt.Errorf("%w: merge source was not found", ErrInvalidEdit)
	}
	sources := make([]Shot, len(request.ShotIDs))
	for i, id := range request.ShotIDs {
		index := shotIndex(base, id)
		if index != firstIndex+i {
			return nil, nil, nil, fmt.Errorf("%w: merge sources must be adjacent and ordered", ErrInvalidEdit)
		}
		sources[i] = base[index]
	}
	left, right := sources[0], sources[len(sources)-1]
	for i, source := range sources {
		if source.SceneID != left.SceneID || source.StoryboardID != left.StoryboardID || source.LocationID != left.LocationID {
			return nil, nil, nil, fmt.Errorf("%w: merge sources must share storyboard, scene and location", ErrInvalidEdit)
		}
		if left.Axis != "" && source.Axis != "" && left.Axis != source.Axis {
			return nil, nil, nil, fmt.Errorf("%w: merge would cross the established axis", ErrInvalidEdit)
		}
		if i > 0 && len(boundaryConflicts(sources[i-1], source)) > 0 {
			return nil, nil, nil, fmt.Errorf("%w: source action/state boundary is not mergeable", ErrInvalidEdit)
		}
	}
	merged := request.Shots[0]
	inheritShot(&merged, left)
	merged.ShotID = request.NewShotIDs[0]
	merged.LineageRootShotID = left.LineageRootShotID
	if merged.LineageRootShotID == "" {
		merged.LineageRootShotID = left.ShotID
	}
	merged.GenerationVersion, merged.Version = 1, 1
	wantCharacters, wantDialogue := []string{}, []string{}
	totalDuration := 0.0
	for _, source := range sources {
		wantCharacters = append(wantCharacters, source.CharacterIDs...)
		wantDialogue = append(wantDialogue, source.DialogueIDs...)
		totalDuration += source.DurationSeconds
	}
	if !sameSet(merged.CharacterIDs, uniqueStrings(wantCharacters)) {
		return nil, nil, nil, fmt.Errorf("%w: merged character set must equal the union of all source shots", ErrInvalidEdit)
	}
	if !reflect.DeepEqual(merged.DialogueIDs, wantDialogue) {
		return nil, nil, nil, fmt.Errorf("%w: merged dialogues must preserve source order", ErrInvalidEdit)
	}
	if merged.DurationSeconds <= 0 || merged.DurationSeconds > totalDuration+.011 {
		return nil, nil, nil, fmt.Errorf("%w: merged duration must be positive and cannot exceed source duration", ErrInvalidEdit)
	}
	if !reflect.DeepEqual(normalizeState(merged.HeadState), normalizeState(left.HeadState)) ||
		!reflect.DeepEqual(normalizeState(merged.TailState), normalizeState(right.TailState)) {
		return nil, nil, nil, fmt.Errorf("%w: merge must preserve outer head and tail state", ErrInvalidEdit)
	}
	result := append([]Shot{}, base[:firstIndex]...)
	result = append(result, merged)
	result = append(result, base[firstIndex+len(sources):]...)
	return result, request.NewShotIDs, append([]string{}, request.ShotIDs...), nil
}

func reorder(base []Shot, orderedIDs []string) ([]Shot, error) {
	if len(orderedIDs) != len(base) || !sameSet(orderedIDs, shotIDs(base)) {
		return nil, fmt.Errorf("%w: reorder must contain every current shot exactly once", ErrInvalidEdit)
	}
	byID := make(map[string]Shot, len(base))
	for _, shot := range base {
		byID[shot.ShotID] = shot
	}
	result := make([]Shot, 0, len(base))
	for _, id := range orderedIDs {
		result = append(result, byID[id])
	}
	return result, nil
}

func update(base []Shot, shotID string, patch map[string]any) ([]Shot, error) {
	index := shotIndex(base, shotID)
	if index < 0 || len(patch) == 0 {
		return nil, fmt.Errorf("%w: update requires a current shot and non-empty patch", ErrInvalidEdit)
	}
	raw, _ := json.Marshal(base[index])
	var node map[string]any
	_ = json.Unmarshal(raw, &node)
	for key, value := range patch {
		if !editableFields[key] {
			return nil, fmt.Errorf("%w: field %s is not editable", ErrInvalidEdit, key)
		}
		node[key] = value
	}
	raw, _ = json.Marshal(node)
	var next Shot
	if err := json.Unmarshal(raw, &next); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidEdit, err)
	}
	if next.ShotID != base[index].ShotID || next.SceneID != base[index].SceneID || next.StoryboardID != base[index].StoryboardID {
		return nil, fmt.Errorf("%w: update cannot change shot identity, storyboard or scene", ErrInvalidEdit)
	}
	next.Version = max(1, base[index].Version) + 1
	base[index] = next
	return base, nil
}

func validateShotContent(shots []Shot, dialogueDurations map[string]int64) []Conflict {
	result := make([]Conflict, 0)
	for _, shot := range shots {
		add := func(code, message string, details map[string]any) {
			result = append(result, Conflict{Code: code, Severity: "blocking", Message: message, ToShotID: shot.ShotID, Details: details})
		}
		if strings.TrimSpace(shot.ActionDescription) == "" {
			add("ACTION_REQUIRED", "镜头动作不能为空", nil)
		}
		if shot.DurationSeconds <= 0 {
			add("DURATION_INVALID", "镜头时长必须大于 0", nil)
		}
		var dialogueMS int64
		for _, id := range shot.DialogueIDs {
			dialogueMS += dialogueDurations[id]
		}
		if dialogueMS > 0 && shot.DurationSeconds*1000+1 < float64(dialogueMS) {
			add("DIALOGUE_EXCEEDS_SHOT", "对白时长超过镜头时长", map[string]any{"dialogue_ms": dialogueMS, "shot_ms": shot.DurationSeconds * 1000})
		}
		if shot.CoverageRole == "shot_reverse" && (shot.CoverageGroup == "" || (shot.CoverageSide != "a" && shot.CoverageSide != "b")) {
			add("SHOT_REVERSE_PAIR_INVALID", "正反打必须填写组名及 a/b 侧", nil)
		}
	}
	return result
}

func ValidateContinuity(shots []Shot) []Conflict {
	result := make([]Conflict, 0)
	for i := 0; i+1 < len(shots); i++ {
		result = append(result, boundaryConflicts(shots[i], shots[i+1])...)
	}
	return result
}

func boundaryConflicts(left, right Shot) []Conflict {
	result := make([]Conflict, 0)
	if left.SceneID == right.SceneID && left.Axis != "" && right.Axis != "" && left.Axis != right.Axis && right.CoverageRole != "establishing" {
		result = append(result, Conflict{Code: "AXIS_CROSS", Severity: "blocking", Message: "相邻镜头跨轴且没有建立镜头重置轴线", FromShotID: left.ShotID, ToShotID: right.ShotID})
	}
	keys := []string{"pose", "gaze", "motion_direction", "character_positions", "costume", "held_props"}
	for _, key := range keys {
		from, fromOK := left.TailState[key]
		to, toOK := right.HeadState[key]
		if fromOK && toOK && !reflect.DeepEqual(normalizeState(from), normalizeState(to)) {
			result = append(result, Conflict{Code: "CONTINUITY_STATE_MISMATCH", Severity: "blocking",
				Message: "相邻镜头首尾状态不连续: " + key, FromShotID: left.ShotID, ToShotID: right.ShotID,
				Details: map[string]any{"field": key, "tail": from, "head": to}})
		}
	}
	fromPhase, fromOK := left.ActionPhase["end"]
	toPhase, toOK := right.ActionPhase["start"]
	if fromOK && toOK && !reflect.DeepEqual(normalizeState(fromPhase), normalizeState(toPhase)) {
		result = append(result, Conflict{Code: "ACTION_PHASE_MISMATCH", Severity: "blocking", Message: "相邻镜头动作阶段无法接力", FromShotID: left.ShotID, ToShotID: right.ShotID})
	}
	return result
}

func CheckCoverage(shots []Shot, required []string) []CoverageCheck {
	requiredSet := stringSet(required)
	byScene := map[string][]Shot{}
	for _, shot := range shots {
		byScene[shot.SceneID] = append(byScene[shot.SceneID], shot)
	}
	scenes := make([]string, 0, len(byScene))
	for scene := range byScene {
		scenes = append(scenes, scene)
	}
	sort.Strings(scenes)
	result := make([]CoverageCheck, 0, len(scenes)*len(coverageKinds))
	for _, scene := range scenes {
		items := byScene[scene]
		for _, kind := range coverageKinds {
			ids := []string{}
			if kind.Key == "shot_reverse" {
				groups := map[string]map[string][]string{}
				for _, shot := range items {
					if shot.CoverageRole != kind.Key || shot.CoverageGroup == "" {
						continue
					}
					if groups[shot.CoverageGroup] == nil {
						groups[shot.CoverageGroup] = map[string][]string{}
					}
					groups[shot.CoverageGroup][shot.CoverageSide] = append(groups[shot.CoverageGroup][shot.CoverageSide], shot.ShotID)
				}
				for _, sides := range groups {
					if len(sides["a"]) > 0 && len(sides["b"]) > 0 {
						ids = append(ids, sides["a"]...)
						ids = append(ids, sides["b"]...)
					}
				}
			} else {
				for _, shot := range items {
					if shot.CoverageRole == kind.Key {
						ids = append(ids, shot.ShotID)
					}
				}
			}
			result = append(result, CoverageCheck{SceneID: scene, Kind: kind.Key, Label: kind.Label,
				Passed: len(ids) > 0, ShotIDs: ids, Required: requiredSet[kind.Key]})
		}
	}
	return result
}

func BuildHandoffs(shots []Shot, conflicts []Conflict) []Handoff {
	result := make([]Handoff, 0, max(0, len(shots)-1))
	for i := 0; i+1 < len(shots); i++ {
		left, right := shots[i], shots[i+1]
		diagnostics := []Conflict{}
		for _, conflict := range conflicts {
			if conflict.FromShotID == left.ShotID && conflict.ToShotID == right.ShotID {
				diagnostics = append(diagnostics, conflict)
			}
		}
		status := "ready"
		if len(diagnostics) > 0 {
			status = "conflict"
		}
		result = append(result, Handoff{FromShotID: left.ShotID, ToShotID: right.ShotID,
			TargetTailFrameRef: left.TailFrameRef, ReferenceHeadFrame: right.HeadFrameRef,
			FromActionPhase: stringValue(left.ActionPhase["end"]), ToActionPhase: stringValue(right.ActionPhase["start"]),
			MotionDirection: stringValue(right.HeadState["motion_direction"]), GazeConstraint: stringValue(right.HeadState["gaze"]),
			ShotSizeConstraint: left.ShotSize + "→" + right.ShotSize, CompositionConstraint: right.Composition,
			PoseConstraints: map[string]any{"from": left.TailState["pose"], "to": right.HeadState["pose"]}, Status: status, Diagnostics: diagnostics})
	}
	return result
}

func normalizeSequence(shots []Shot) {
	for i := range shots {
		shots[i].ShotOrder, shots[i].ShotNumber = i+1, i+1
		if shots[i].CharacterIDs == nil {
			shots[i].CharacterIDs = []string{}
		}
		if shots[i].DialogueIDs == nil {
			shots[i].DialogueIDs = []string{}
		}
		if shots[i].HeadState == nil {
			shots[i].HeadState = map[string]any{}
		}
		if shots[i].TailState == nil {
			shots[i].TailState = map[string]any{}
		}
		if shots[i].Performance == nil {
			shots[i].Performance = map[string]any{}
		}
		if shots[i].ActionPhase == nil {
			shots[i].ActionPhase = map[string]any{}
		}
		if shots[i].ContinuityNotes == nil {
			shots[i].ContinuityNotes = map[string]any{}
		}
		if shots[i].SourceSceneData == nil {
			shots[i].SourceSceneData = map[string]any{}
		}
	}
}

func inheritShot(target *Shot, source Shot) {
	target.StoryboardID, target.ProjectID, target.EpisodeID = source.StoryboardID, source.ProjectID, source.EpisodeID
	if target.SceneID == "" {
		target.SceneID = source.SceneID
	}
	if target.LocationID == "" {
		target.LocationID = source.LocationID
	}
	if target.ShotSize == "" {
		target.ShotSize = source.ShotSize
	}
	if target.CameraAngle == "" {
		target.CameraAngle = source.CameraAngle
	}
	if target.CameraMotion == "" {
		target.CameraMotion = source.CameraMotion
	}
	if target.Composition == "" {
		target.Composition = source.Composition
	}
	if target.CharacterIDs == nil {
		target.CharacterIDs = copyStrings(source.CharacterIDs)
	}
	if target.HeadState == nil {
		target.HeadState = cloneMap(source.HeadState)
	}
	if target.TailState == nil {
		target.TailState = cloneMap(source.TailState)
	}
	if target.Performance == nil {
		target.Performance = cloneMap(source.Performance)
	}
	if target.ActionPhase == nil {
		target.ActionPhase = cloneMap(source.ActionPhase)
	}
	if target.Axis == "" {
		target.Axis = source.Axis
	}
	if target.Status == "" {
		target.Status = "draft"
	}
	if target.TransitionType == "" {
		target.TransitionType = source.TransitionType
	}
	if target.Lighting == "" {
		target.Lighting = source.Lighting
	}
	if target.Atmosphere == "" {
		target.Atmosphere = source.Atmosphere
	}
	target.ContinuityNotes = cloneMap(source.ContinuityNotes)
	target.SourceSceneData = cloneMap(source.SourceSceneData)
}

func diffIDs(base, next []Shot, created, retired []string) ([]string, []string, []string) {
	before, after := map[string]Shot{}, map[string]Shot{}
	for _, shot := range base {
		before[shot.ShotID] = shot
	}
	for _, shot := range next {
		after[shot.ShotID] = shot
	}
	for id := range after {
		if _, ok := before[id]; !ok {
			created = append(created, id)
		}
	}
	for id := range before {
		if _, ok := after[id]; !ok {
			retired = append(retired, id)
		}
	}
	changed := append([]string{}, created...)
	for id, shot := range after {
		if old, ok := before[id]; ok && !sameShotContent(old, shot) {
			changed = append(changed, id)
		}
	}
	return uniqueStrings(changed), uniqueStrings(created), uniqueStrings(retired)
}

func sameShotContent(a, b Shot) bool {
	aa, bb := []Shot{a}, []Shot{b}
	normalizeSequence(aa)
	normalizeSequence(bb)
	a, b = aa[0], bb[0]
	a.ThumbnailURL = ""
	b.ThumbnailURL = ""
	a.HeadFrameRef = ""
	b.HeadFrameRef = ""
	a.TailFrameRef = ""
	b.TailFrameRef = ""
	a.Version = 0
	b.Version = 0
	a.ShotOrder, a.ShotNumber = 0, 0
	b.ShotOrder, b.ShotNumber = 0, 0
	return reflect.DeepEqual(a, b)
}
func normalizeState(v any) any {
	raw, _ := json.Marshal(v)
	var result any
	_ = json.Unmarshal(raw, &result)
	return result
}
func cloneMap(v map[string]any) map[string]any {
	raw, _ := json.Marshal(v)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	if result == nil {
		result = map[string]any{}
	}
	return result
}
func shotIndex(shots []Shot, id string) int {
	for i := range shots {
		if shots[i].ShotID == id {
			return i
		}
	}
	return -1
}
func shotIDs(shots []Shot) []string {
	result := make([]string, len(shots))
	for i := range shots {
		result[i] = shots[i].ShotID
	}
	return result
}
func copyStrings(v []string) []string { return append([]string{}, v...) }
func uniqueStrings(v []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, x := range v {
		if x != "" && !seen[x] {
			seen[x] = true
			result = append(result, x)
		}
	}
	return result
}
func stringSet(v []string) map[string]bool {
	result := map[string]bool{}
	for _, x := range v {
		result[x] = true
	}
	return result
}
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string{}, a...)
	bb := append([]string{}, b...)
	sort.Strings(aa)
	sort.Strings(bb)
	return reflect.DeepEqual(aa, bb)
}
func sameOrderedSet(a, b []string) bool { return reflect.DeepEqual(a, b) }
func hasDuplicates(v []string) bool     { return len(uniqueStrings(v)) != len(v) }
func stringValue(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
