package performancecontinuity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

func ApplyBibleChanges(current PerformanceBible, changes map[string]any, reason string, ordinaryGeneration bool) (PerformanceBible, []Diagnostic) {
	diagnostics := make([]Diagnostic, 0)
	if strings.TrimSpace(reason) == "" {
		diagnostics = append(diagnostics, Diagnostic{Code: "CHANGE_REASON_REQUIRED", Severity: "blocking", Path: "change_reason", Message: "表演圣经变更必须记录理由", Suggestion: "填写可追溯的剧情或人工修订理由"})
		return current, diagnostics
	}
	locked := stringSet(current.LockedFields)
	allowed := stringSet(current.AllowedFields)
	raw, _ := json.Marshal(current)
	var document map[string]any
	_ = json.Unmarshal(raw, &document)
	for path, value := range changes {
		if locked[path] {
			code := "LOCKED_FIELD_CHANGE_REJECTED"
			message := "锁定字段不能被普通生成任务修改"
			if !ordinaryGeneration {
				code, message = "LOCKED_FIELD_REQUIRES_NEW_VERSION", "锁定字段只能通过显式新版本和解锁审批修改"
			}
			diagnostics = append(diagnostics, Diagnostic{Code: code, Severity: "blocking", Path: path, Message: message, Actual: value, Suggestion: "保留当前值，或由编辑创建解锁后的新版本"})
			continue
		}
		if !allowed[path] {
			diagnostics = append(diagnostics, Diagnostic{Code: "FIELD_NOT_ALLOWED", Severity: "blocking", Path: path, Message: "字段未列入允许变化范围", Actual: value, Suggestion: "在表演圣经中显式声明允许字段及变化理由"})
			continue
		}
		setPath(document, path, value)
	}
	if hasBlocking(diagnostics) {
		return current, diagnostics
	}
	updatedRaw, _ := json.Marshal(document)
	var updated PerformanceBible
	_ = json.Unmarshal(updatedRaw, &updated)
	updated.Version = current.Version + 1
	updated.Status = "draft"
	if updated.ChangeReasons == nil {
		updated.ChangeReasons = map[string]string{}
	}
	for path := range changes {
		updated.ChangeReasons[path] = reason
	}
	return updated, diagnostics
}

func DiagnoseTransition(previousOutput, nextInput State) []Diagnostic {
	var result []Diagnostic
	compare := func(path string, expected, actual any, code, suggestion string) {
		if !reflect.DeepEqual(expected, actual) {
			result = append(result, Diagnostic{Code: code, Severity: "blocking", Path: path, Message: "上一镜输出状态与下一镜输入状态矛盾", Expected: expected, Actual: actual, Suggestion: suggestion})
		}
	}
	compare("environment.location_id", previousOutput.Environment.LocationID, nextInput.Environment.LocationID, "LOCATION_DISCONTINUITY", "补充换场说明或恢复上一镜场景")
	compare("environment.time", previousOutput.Environment.Time, nextInput.Environment.Time, "TIME_DISCONTINUITY", "补充时间跳转或恢复连续时间")
	compare("environment.weather", previousOutput.Environment.Weather, nextInput.Environment.Weather, "WEATHER_DISCONTINUITY", "恢复天气或声明天气转场")
	compare("environment.lighting", previousOutput.Environment.Lighting, nextInput.Environment.Lighting, "LIGHTING_DISCONTINUITY", "统一光线方向和色温")
	compare("axis", previousOutput.Axis, nextInput.Axis, "AXIS_ERROR", "保持 180 度轴线，或插入中性镜头重建轴线")

	for id, previous := range previousOutput.Characters {
		next, ok := nextInput.Characters[id]
		if !ok {
			result = append(result, Diagnostic{Code: "CHARACTER_DISAPPEARED", Severity: "blocking", Path: "characters." + id, Message: "人物无离场动作却从下一镜消失", Expected: previous, Suggestion: "补充离场动作或恢复人物"})
			continue
		}
		compare("characters."+id+".costume", previous.Costume, next.Costume, "COSTUME_DISCONTINUITY", "恢复服装，或记录剧情阶段换装理由")
		compare("characters."+id+".hairstyle", previous.Hairstyle, next.Hairstyle, "HAIRSTYLE_DISCONTINUITY", "恢复发型约束")
		compare("characters."+id+".scars", previous.Scars, next.Scars, "SCAR_DISCONTINUITY", "恢复伤痕位置与状态")
		compare("characters."+id+".held_props", previous.HeldProps, next.HeldProps, "HELD_PROP_DISCONTINUITY", "补充放下/交接动作或恢复手持物")
		compare("characters."+id+".knows", previous.Knows, next.Knows, "KNOWLEDGE_REGRESSION", "人物已知信息不能无解释遗忘")
		compare("characters."+id+".does_not_know", previous.DoesNotKnow, next.DoesNotKnow, "KNOWLEDGE_CONTRADICTION", "同步尚未知信息")
		compare("characters."+id+".identity_ref", previous.IdentityRef, next.IdentityRef, "IDENTITY_REFERENCE_DRIFT", "绑定同一表演圣经与视觉档案版本")
	}
	for id, next := range nextInput.Characters {
		if _, ok := previousOutput.Characters[id]; !ok {
			result = append(result, Diagnostic{Code: "CHARACTER_APPEARED", Severity: "blocking", Path: "characters." + id, Message: "人物无入场动作却突然出现", Actual: next, Suggestion: "补充入场动作或从镜头中移除"})
		}
	}
	for id, previous := range previousOutput.Props {
		next, ok := nextInput.Props[id]
		if !ok || (previous.Visible && !next.Visible) {
			result = append(result, Diagnostic{Code: "PROP_DISAPPEARED", Severity: "blocking", Path: "props." + id, Message: "道具无动作说明却消失", Expected: previous, Actual: next, Suggestion: "补充拿走/遮挡/交接动作或恢复道具"})
			continue
		}
		compare("props."+id+".owner_character_id", previous.OwnerCharacterID, next.OwnerCharacterID, "PROP_OWNERSHIP_CONFLICT", "补充道具交接动作")
		compare("props."+id+".condition", previous.Condition, next.Condition, "PROP_DAMAGE_DISCONTINUITY", "恢复损坏状态或补充损坏动作")
	}
	return result
}

func InheritEpisodeState(previous LedgerEntry, nextEpisodeID string, nextEpisodeNumber int) LedgerEntry {
	return LedgerEntry{
		SchemaVersion: ContinuityLedgerSchema,
		EntryID:       stableID("ledger", previous.ProjectID, nextEpisodeID, "input"),
		ProjectID:     previous.ProjectID,
		EpisodeID:     nextEpisodeID,
		EpisodeNumber: nextEpisodeNumber,
		InputState:    cloneState(previous.OutputState),
		OutputState:   cloneState(previous.OutputState),
		InheritedFrom: previous.EntryID,
	}
}

func PrepareGeneration(req GenerationRequest) GenerationContext {
	context := GenerationContext{Allowed: false, PerformanceBibleRefs: req.PerformanceBibleRefs, Diagnostics: []Diagnostic{}}
	if req.Ledger == nil {
		context.Diagnostics = append(context.Diagnostics, Diagnostic{Code: "CONTINUITY_LEDGER_REQUIRED", Severity: "blocking", Path: "ledger", Message: "生成前必须读取连续性账本", Suggestion: "先创建或读取当前场/镜的输入状态"})
	} else {
		context.ContinuityEntryID = req.Ledger.EntryID
		for _, diagnostic := range req.Ledger.Diagnostics {
			if diagnostic.Severity == "blocking" || diagnostic.Severity == "critical" {
				context.Diagnostics = append(context.Diagnostics, diagnostic)
			}
		}
	}
	for _, characterID := range req.CharacterIDs {
		if strings.TrimSpace(req.PerformanceBibleRefs[characterID]) == "" {
			context.Diagnostics = append(context.Diagnostics, Diagnostic{Code: "PERFORMANCE_BIBLE_REF_REQUIRED", Severity: "blocking", Path: "performance_bible_refs." + characterID, Message: "剧本、分镜、图像、视频和 TTS 必须引用明确的表演圣经版本", Suggestion: "绑定 character/version 的锁定表演圣经"})
		}
	}
	if req.ArtifactType == "video" && req.Handoff == nil {
		context.Diagnostics = append(context.Diagnostics, Diagnostic{Code: "SHOT_HANDOFF_REQUIRED", Severity: "blocking", Path: "handoff", Message: "视频生成必须读取相邻镜头首尾帧与动作接力约束", Suggestion: "创建当前镜头的相邻衔接记录"})
	}
	if req.Handoff != nil {
		context.HandoffID = req.Handoff.HandoffID
	}
	if hasBlocking(context.Diagnostics) {
		return context
	}
	promptParts := []string{strings.TrimSpace(req.BasePrompt), "连续性账本=" + context.ContinuityEntryID}
	for _, id := range sortedKeys(req.PerformanceBibleRefs) {
		promptParts = append(promptParts, "角色表演="+id+"@"+req.PerformanceBibleRefs[id])
	}
	if req.Handoff != nil {
		promptParts = append(promptParts,
			"动作接力="+req.Handoff.FromActionPhase+"→"+req.Handoff.ToActionPhase,
			"运动方向="+req.Handoff.MotionDirection,
			"视线="+req.Handoff.GazeConstraint,
			"构图="+req.Handoff.CompositionConstraint,
		)
	}
	context.Allowed = true
	context.ResolvedPrompt = strings.Join(promptParts, "；")
	return context
}

func RunVisualQC(frames []FrameObservation) []QCIssue {
	if len(frames) == 0 {
		return []QCIssue{}
	}
	var issues []QCIssue
	add := func(frame FrameObservation, category, severity, evidence, recommendation string) {
		sum := sha256.Sum256([]byte(category + frame.Locator.ShotID + fmt.Sprint(frame.Locator.Frame) + evidence))
		issues = append(issues, QCIssue{
			IssueID: "vqi_" + hex.EncodeToString(sum[:])[:20], Category: category, Severity: severity,
			Locator: frame.Locator, Evidence: evidence, Recommendation: recommendation, Status: "open",
			LocalRedo: LocalRedo{EntityType: "shot", EntityID: frame.Locator.ShotID, AllowedFields: []string{"action_description", "composition", "camera_angle", "camera_motion"}, StartMS: max64(0, frame.Locator.TimecodeMS-250), EndMS: frame.Locator.TimecodeMS + 250},
		})
	}
	first := frames[0]
	for index, frame := range frames {
		for characterID, score := range frame.IdentityScores {
			if score < .82 {
				add(frame, "identity_drift", "critical", fmt.Sprintf("%s identity similarity %.2f < 0.82", characterID, score), "绑定锁定角色脸部参考并局部重做该帧范围")
			}
		}
		for _, defect := range frame.Defects {
			switch defect {
			case "extra_hand", "extra_finger":
				add(frame, "limb_deformation", "critical", defect, "加强手部负面提示并重做定位帧")
			case "face_deformation":
				add(frame, "face_deformation", "critical", defect, "使用角色身份参考重做面部")
			}
		}
		if frame.FlickerScore > .25 {
			add(frame, "video_flicker", "major", fmt.Sprintf("flicker score %.2f", frame.FlickerScore), "锁定种子、参考帧和光照后局部重做")
		}
		if frame.MeltScore > .2 {
			add(frame, "background_melt", "critical", fmt.Sprintf("background melt score %.2f", frame.MeltScore), "使用固定背景参考与低运动强度重做")
		}
		for _, box := range frame.SubtitleBoxes {
			if box.OverlapsFace {
				add(frame, "subtitle_over_face", "major", "subtitle box overlaps detected face", "移动字幕到下方安全区")
			}
			if box.X < .05 || box.Y < .05 || box.X+box.Width > .95 || box.Y+box.Height > .95 {
				add(frame, "subtitle_outside_safe_area", "major", fmt.Sprintf("subtitle box %.2f,%.2f %.2fx%.2f", box.X, box.Y, box.Width, box.Height), "将字幕限制在 5% 安全区内")
			}
		}
		if index == 0 {
			continue
		}
		previous := frames[index-1]
		compareMapChanges(previous, frame, add)
		for _, id := range previous.CharacterIDs {
			if !containsString(frame.CharacterIDs, id) {
				add(frame, "object_disappeared", "critical", id+" disappeared without an exit action", "恢复人物或补充离场动作")
			}
		}
		for _, id := range frame.CharacterIDs {
			if !containsString(previous.CharacterIDs, id) {
				add(frame, "object_appeared", "critical", id+" appeared without an entrance action", "移除人物或补充入场动作")
			}
		}
		if previous.Axis != "" && frame.Axis != "" && previous.Axis != frame.Axis {
			add(frame, "axis_error", "critical", previous.Axis+" → "+frame.Axis, "恢复 180 度轴线或插入中性镜头")
		}
		if previous.MotionDirection != "" && frame.MotionDirection != "" && previous.MotionDirection != frame.MotionDirection {
			add(frame, "motion_direction_error", "major", previous.MotionDirection+" → "+frame.MotionDirection, "保持运动方向或展示明确转向动作")
		}
		for id, x := range previous.Positions {
			if nextX, ok := frame.Positions[id]; ok && ((x < .5 && nextX > .5) || (x > .5 && nextX < .5)) {
				add(frame, "screen_position_jump", "major", fmt.Sprintf("%s x %.2f → %.2f", id, x, nextX), "保持人物屏幕左右位置或使用中性镜头")
			}
		}
		for id, gaze := range previous.GazeDirections {
			if next, ok := frame.GazeDirections[id]; ok && gaze != next {
				add(frame, "gaze_error", "major", id+" gaze "+gaze+" → "+next, "保持视线目标，或补充明确的转头/移视动作")
			}
		}
		if !posesConnect(previous.Pose, frame.Pose) {
			add(frame, "action_discontinuity", "critical", fmt.Sprintf("tail pose %v cannot connect to head pose %v", previous.Pose, frame.Pose), "重做相邻镜头边界，保存“开始动作→完成动作”的接力")
			add(frame, "handoff_failure", "critical", "target tail frame cannot naturally reach reference head frame", "重新生成目标尾帧/参考首帧并重算相邻动作接力")
		}
	}
	_ = first
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Locator.ShotID == issues[j].Locator.ShotID {
			return issues[i].Locator.Frame < issues[j].Locator.Frame
		}
		return issues[i].Locator.ShotID < issues[j].Locator.ShotID
	})
	return issues
}

func RecalculateAdjacentHandoffs(shots []string, changedShotID string, existing []ShotHandoff) []ShotHandoff {
	index := -1
	for i, id := range shots {
		if id == changedShotID {
			index = i
			break
		}
	}
	if index < 0 {
		return existing
	}
	affected := map[string]bool{}
	if index > 0 {
		affected[shots[index-1]+"→"+shots[index]] = true
	}
	if index+1 < len(shots) {
		affected[shots[index]+"→"+shots[index+1]] = true
	}
	result := make([]ShotHandoff, 0, len(existing)+2)
	for _, handoff := range existing {
		key := handoff.FromShotID + "→" + handoff.ToShotID
		if !affected[key] {
			result = append(result, handoff)
		}
	}
	for key := range affected {
		parts := strings.Split(key, "→")
		result = append(result, ShotHandoff{
			SchemaVersion: ShotHandoffSchema, HandoffID: stableID("handoff", parts[0], parts[1]),
			FromShotID: parts[0], ToShotID: parts[1], Version: 1,
			FromActionPhase: "动作进行中", ToActionPhase: "承接并完成动作",
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].FromShotID < result[j].FromShotID })
	return result
}

func compareMapChanges(previous, current FrameObservation, add func(FrameObservation, string, string, string, string)) {
	for id, age := range previous.Ages {
		if next, ok := current.Ages[id]; ok && age != next {
			add(current, "age_drift", "major", id+" "+age+" → "+next, "恢复表演圣经的年龄感约束")
		}
	}
	for id, hair := range previous.Hairstyles {
		if next, ok := current.Hairstyles[id]; ok && hair != next {
			add(current, "hairstyle_change", "major", id+" "+hair+" → "+next, "绑定同一发型参考")
		}
	}
	for id, costume := range previous.Costumes {
		if next, ok := current.Costumes[id]; ok && costume != next {
			add(current, "costume_change", "critical", id+" "+costume+" → "+next, "恢复账本服装或记录换装理由")
		}
	}
	for id, scars := range previous.Scars {
		if next, ok := current.Scars[id]; ok && !reflect.DeepEqual(scars, next) {
			add(current, "scar_change", "major", fmt.Sprintf("%s %v → %v", id, scars, next), "恢复伤痕位置与状态")
		}
	}
	for id, visible := range previous.Props {
		if visible && !current.Props[id] {
			add(current, "prop_disappeared", "critical", id+" disappeared", "恢复道具或补充拿走/遮挡动作")
		}
	}
	if previous.BackgroundID != "" && current.BackgroundID != "" && previous.BackgroundID != current.BackgroundID {
		add(current, "background_change", "major", previous.BackgroundID+" → "+current.BackgroundID, "绑定同一场景参考")
	}
}

func posesConnect(previous, current map[string]string) bool {
	for id, tail := range previous {
		head, ok := current[id]
		if !ok {
			continue
		}
		if strings.HasPrefix(tail, "start:") {
			return head == "complete:"+strings.TrimPrefix(tail, "start:")
		}
		if tail != head {
			return false
		}
	}
	return true
}

func cloneState(state State) State {
	raw, _ := json.Marshal(state)
	var result State
	_ = json.Unmarshal(raw, &result)
	return result
}

func setPath(document map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := document
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func hasBlocking(values []Diagnostic) bool {
	for _, value := range values {
		if value.Severity == "blocking" {
			return true
		}
	}
	return false
}

func stableID(prefix string, values ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(values, ":")))
	return prefix + "_" + hex.EncodeToString(sum[:])[:20]
}

func sortedKeys(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
