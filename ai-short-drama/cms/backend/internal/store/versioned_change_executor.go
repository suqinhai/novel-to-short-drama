package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"short-drama-cms/backend/internal/localedit"
)

type versionedRowsQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type pendingRebuildTarget struct {
	EntityType string
	EntityID   string
	StartMS    *int64
	EndMS      *int64
}

func impactedNativeEntityIDs(plan localedit.Plan) []string {
	if plan.Target.EntityType != "episode_content" {
		return []string{plan.Target.EntityID}
	}
	ids := []string{}
	broadEpisodeChange := false
	for _, change := range plan.ExpectedChanges {
		parts := strings.Split(change.Field, ".")
		if len(parts) >= 2 && (parts[0] == "scene" || parts[0] == "dialogue") {
			ids = append(ids, parts[1])
		} else {
			broadEpisodeChange = true
		}
	}
	if broadEpisodeChange || len(ids) == 0 {
		ids = append(ids, plan.Target.EntityID)
	}
	return uniqueVersionedEntityIDs(ids)
}

func enrichChangesWithDiff(
	snapshot json.RawMessage, entityType string, changes []localedit.Change,
) ([]localedit.Change, error) {
	var content map[string]any
	if err := json.Unmarshal(snapshot, &content); err != nil {
		return nil, fmt.Errorf("decode target snapshot: %w", err)
	}
	result := make([]localedit.Change, len(changes))
	for index, change := range changes {
		before, ok := lookupVersionedField(content, entityType, change.Field)
		if change.Operation == "insert" {
			before, ok = nil, true
		}
		if !ok && (entityType == "timeline" || change.Operation == "regenerate_segment") {
			before, ok = nil, true
		}
		if !ok {
			return nil, fmt.Errorf("%w: field %s is absent from the current version",
				localedit.ErrInvalidPlan, change.Field)
		}
		after, err := changedValue(before, change)
		if err != nil {
			return nil, err
		}
		change.Before, change.After = before, after
		result[index] = change
	}
	return result, nil
}

func enrichChangeTimeRanges(
	ctx context.Context, tx pgx.Tx, entityType, entityID string, changes []localedit.Change,
) ([]localedit.Change, error) {
	result := append([]localedit.Change(nil), changes...)
	for index := range result {
		if result[index].StartMS != nil && result[index].EndMS != nil {
			continue
		}
		dialogueID := ""
		switch entityType {
		case "dialogue":
			dialogueID = entityID
		case "episode_content":
			parts := strings.Split(result[index].Field, ".")
			if len(parts) >= 2 && parts[0] == "dialogue" {
				dialogueID = parts[1]
			}
		}
		if dialogueID == "" {
			continue
		}
		var startMS, endMS int64
		err := tx.QueryRow(ctx, `SELECT min(start_ms),max(end_ms)
			FROM drama.subtitle_cues
			WHERE dialogue_id=$1 AND is_current
			HAVING count(*)>0`, dialogueID).Scan(&startMS, &endMS)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if endMS > startMS {
			result[index].StartMS, result[index].EndMS = &startMS, &endMS
		}
	}
	return result, nil
}

func materializeVersionedChange(
	ctx context.Context, tx pgx.Tx, projectID, entityType, entityID string,
	before json.RawMessage, changes []localedit.Change, fingerprint string,
) (json.RawMessage, error) {
	if entityType == "timeline" {
		return materializeTimelineChange(ctx, tx, projectID, entityID, changes, fingerprint)
	}
	var content map[string]any
	if err := json.Unmarshal(before, &content); err != nil {
		return nil, err
	}
	for _, change := range changes {
		if entityType == "shot_video" && change.Operation == "regenerate_segment" &&
			change.EndMS != nil {
			durationSeconds, _ := numericValue(content["actual_duration_seconds"])
			if durationSeconds <= 0 {
				durationSeconds, _ = numericValue(content["requested_duration_seconds"])
			}
			if *change.EndMS > int64(durationSeconds*1000) {
				return nil, fmt.Errorf("%w: video segment exceeds source duration",
					localedit.ErrInvalidPlan)
			}
		}
		current, ok := lookupVersionedField(content, entityType, change.Field)
		if change.Operation == "insert" && !ok {
			current, ok = nil, true
		}
		if !ok && change.Operation == "regenerate_segment" {
			current, ok = nil, true
		}
		if !ok {
			return nil, fmt.Errorf("%w: field %s is absent from current", localedit.ErrInvalidPlan, change.Field)
		}
		if change.Before != nil &&
			!reflect.DeepEqual(normalizeJSONValue(current), normalizeJSONValue(change.Before)) {
			return nil, fmt.Errorf("%w: planned source value for %s no longer matches current",
				ErrConflict, change.Field)
		}
		next, err := changedValue(current, change)
		if err != nil {
			return nil, err
		}
		if change.After != nil && !reflect.DeepEqual(normalizeJSONValue(next), normalizeJSONValue(change.After)) {
			return nil, fmt.Errorf("%w: planned diff for %s no longer matches current",
				ErrConflict, change.Field)
		}
		if !setVersionedField(content, entityType, change.Field, next) {
			return nil, fmt.Errorf("%w: cannot apply field %s", localedit.ErrInvalidPlan, change.Field)
		}
	}
	if entityType == "episode_content" {
		if err := validateMaterializedEpisodeStructure(content); err != nil {
			return nil, err
		}
	}
	return json.Marshal(content)
}

func changedValue(before any, change localedit.Change) (any, error) {
	switch change.Operation {
	case "replace", "regenerate", "manual_replace", "insert":
		return change.Value, nil
	case "remove":
		return nil, nil
	case "adjust":
		beforeNumber, ok := numericValue(before)
		if !ok {
			if change.Value != nil {
				return change.Value, nil
			}
			return nil, fmt.Errorf("%w: field %s is not numeric", localedit.ErrInvalidPlan, change.Field)
		}
		delta, ok := numericValue(change.Delta)
		if !ok {
			delta, ok = numericValue(change.Value)
		}
		if !ok {
			return nil, fmt.Errorf("%w: numeric delta is required for %s", localedit.ErrInvalidPlan, change.Field)
		}
		next := beforeNumber + delta
		if strings.Contains(change.Field, "duration") && next <= 0 {
			return nil, fmt.Errorf("%w: duration must remain positive", localedit.ErrInvalidPlan)
		}
		return next, nil
	case "reorder":
		if change.Value != nil {
			return change.Value, nil
		}
		beforeNumber, beforeOK := numericValue(before)
		delta, deltaOK := numericValue(change.Delta)
		if !beforeOK || !deltaOK {
			return nil, fmt.Errorf("%w: reorder requires a target or delta", localedit.ErrInvalidPlan)
		}
		return beforeNumber + delta, nil
	case "regenerate_segment":
		return map[string]any{
			"instruction": change.Value, "start_ms": change.StartMS, "end_ms": change.EndMS,
			"rebuild_status": "pending",
		}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported operation %s", localedit.ErrInvalidPlan, change.Operation)
	}
}

func lookupVersionedField(content map[string]any, entityType, field string) (any, bool) {
	if entityType != "episode_content" {
		value, ok := content[field]
		return value, ok
	}
	parts := strings.Split(field, ".")
	switch {
	case len(parts) == 2 && (parts[0] == "outline" || parts[0] == "script"):
		node, ok := content[parts[0]].(map[string]any)
		if !ok {
			return nil, false
		}
		value, exists := node[parts[1]]
		return value, exists
	case len(parts) == 3 && parts[0] == "scene":
		scene := findScene(content, parts[1])
		if scene == nil {
			return nil, false
		}
		value, exists := scene[parts[2]]
		return value, exists
	case len(parts) == 3 && parts[0] == "dialogue":
		dialogue := findDialogue(content, parts[1])
		if dialogue == nil {
			return nil, false
		}
		value, exists := dialogue[parts[2]]
		return value, exists
	case len(parts) == 2 && parts[0] == "scene":
		scene := findScene(content, parts[1])
		return scene, scene != nil
	case len(parts) == 2 && parts[0] == "dialogue":
		dialogue := findDialogue(content, parts[1])
		return dialogue, dialogue != nil
	default:
		return nil, false
	}
}

func setVersionedField(content map[string]any, entityType, field string, value any) bool {
	if entityType != "episode_content" {
		content[field] = value
		return true
	}
	parts := strings.Split(field, ".")
	switch {
	case len(parts) == 2 && (parts[0] == "outline" || parts[0] == "script"):
		node, ok := content[parts[0]].(map[string]any)
		if !ok {
			return false
		}
		node[parts[1]] = value
		return true
	case len(parts) == 3 && parts[0] == "scene":
		scene := findScene(content, parts[1])
		if scene == nil {
			return false
		}
		scene[parts[2]] = value
		return true
	case len(parts) == 3 && parts[0] == "dialogue":
		dialogue := findDialogue(content, parts[1])
		if dialogue == nil {
			return false
		}
		dialogue[parts[2]] = value
		return true
	case len(parts) == 2 && parts[0] == "scene":
		return setEpisodeScene(content, parts[1], value)
	case len(parts) == 2 && parts[0] == "dialogue":
		return setEpisodeDialogue(content, parts[1], value)
	default:
		return false
	}
}

func setEpisodeScene(content map[string]any, sceneID string, value any) bool {
	script, _ := content["script"].(map[string]any)
	if script == nil {
		return false
	}
	scenes, _ := script["scenes"].([]any)
	index := -1
	for i, item := range scenes {
		scene, _ := item.(map[string]any)
		if fmt.Sprint(scene["scene_id"]) == sceneID {
			index = i
			break
		}
	}
	if value == nil {
		if index < 0 {
			return false
		}
		script["scenes"] = append(scenes[:index], scenes[index+1:]...)
		return true
	}
	next, ok := value.(map[string]any)
	if !ok || fmt.Sprint(next["scene_id"]) != sceneID || index >= 0 {
		return false
	}
	if _, exists := next["dialogues"]; !exists {
		next["dialogues"] = []any{}
	}
	script["scenes"] = append(scenes, next)
	return true
}

func setEpisodeDialogue(content map[string]any, dialogueID string, value any) bool {
	script, _ := content["script"].(map[string]any)
	scenes, _ := script["scenes"].([]any)
	foundScene, foundIndex := -1, -1
	for sceneIndex, sceneItem := range scenes {
		scene, _ := sceneItem.(map[string]any)
		dialogues, _ := scene["dialogues"].([]any)
		for dialogueIndex, dialogueItem := range dialogues {
			dialogue, _ := dialogueItem.(map[string]any)
			if fmt.Sprint(dialogue["dialogue_id"]) == dialogueID {
				foundScene, foundIndex = sceneIndex, dialogueIndex
				break
			}
		}
	}
	if value == nil {
		if foundScene < 0 {
			return false
		}
		scene, _ := scenes[foundScene].(map[string]any)
		dialogues, _ := scene["dialogues"].([]any)
		scene["dialogues"] = append(dialogues[:foundIndex], dialogues[foundIndex+1:]...)
		return true
	}
	next, ok := value.(map[string]any)
	if !ok || fmt.Sprint(next["dialogue_id"]) != dialogueID || foundScene >= 0 {
		return false
	}
	sceneID := strings.TrimSpace(fmt.Sprint(next["scene_id"]))
	target := findScene(content, sceneID)
	if target == nil {
		return false
	}
	dialogues, _ := target["dialogues"].([]any)
	target["dialogues"] = append(dialogues, next)
	return true
}

func validateMaterializedEpisodeStructure(content map[string]any) error {
	script, _ := content["script"].(map[string]any)
	if script == nil {
		return nil
	}
	scenes, _ := script["scenes"].([]any)
	sceneNumbers := map[int]bool{}
	dialogueIDs := map[string]bool{}
	for _, sceneItem := range scenes {
		scene, ok := sceneItem.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(scene["scene_id"])) == "" {
			return fmt.Errorf("%w: invalid scene in materialized script", ErrInvalidEpisodeContent)
		}
		numberValue, ok := numericValue(scene["scene_number"])
		number := int(numberValue)
		if !ok || number < 1 || sceneNumbers[number] || numberValue != float64(number) {
			return fmt.Errorf("%w: scene_number must be a unique positive integer", ErrInvalidEpisodeContent)
		}
		sceneNumbers[number] = true
		sequences := map[int]bool{}
		dialogues, _ := scene["dialogues"].([]any)
		for _, dialogueItem := range dialogues {
			dialogue, ok := dialogueItem.(map[string]any)
			id := strings.TrimSpace(fmt.Sprint(dialogue["dialogue_id"]))
			sequenceValue, sequenceOK := numericValue(dialogue["sequence_number"])
			sequence := int(sequenceValue)
			if !ok || id == "" || dialogueIDs[id] || !sequenceOK || sequence < 1 ||
				sequences[sequence] || sequenceValue != float64(sequence) {
				return fmt.Errorf("%w: dialogue ids and sequence numbers must be unique", ErrInvalidEpisodeContent)
			}
			dialogueIDs[id], sequences[sequence] = true, true
		}
	}
	return nil
}

func findScene(content map[string]any, sceneID string) map[string]any {
	script, _ := content["script"].(map[string]any)
	scenes, _ := script["scenes"].([]any)
	for _, item := range scenes {
		scene, _ := item.(map[string]any)
		if fmt.Sprint(scene["scene_id"]) == sceneID {
			return scene
		}
	}
	return nil
}

func findDialogue(content map[string]any, dialogueID string) map[string]any {
	script, _ := content["script"].(map[string]any)
	scenes, _ := script["scenes"].([]any)
	for _, sceneItem := range scenes {
		scene, _ := sceneItem.(map[string]any)
		dialogues, _ := scene["dialogues"].([]any)
		for _, dialogueItem := range dialogues {
			dialogue, _ := dialogueItem.(map[string]any)
			if fmt.Sprint(dialogue["dialogue_id"]) == dialogueID {
				return dialogue
			}
		}
	}
	return nil
}

func restoreEpisodeContentChanges(
	source, current map[string]any,
) ([]localedit.Change, error) {
	paths := []string{
		"outline.title", "outline.logline", "outline.opening_hook", "outline.story_goal",
		"outline.main_conflict", "outline.climax", "outline.ending_hook",
		"outline.estimated_duration_seconds", "script.title", "script.opening_hook",
		"script.climax", "script.ending_hook",
	}
	sourceScript, _ := source["script"].(map[string]any)
	currentScript, _ := current["script"].(map[string]any)
	sourceScenes, _ := sourceScript["scenes"].([]any)
	currentScenes, _ := currentScript["scenes"].([]any)
	sourceSceneByID, currentSceneByID := map[string]map[string]any{}, map[string]map[string]any{}
	sourceDialogueByID, currentDialogueByID := map[string]map[string]any{}, map[string]map[string]any{}
	for _, sceneItem := range sourceScenes {
		scene, _ := sceneItem.(map[string]any)
		sceneID := fmt.Sprint(scene["scene_id"])
		sourceSceneByID[sceneID] = scene
		for _, dialogueItem := range anySlice(scene["dialogues"]) {
			dialogue, _ := dialogueItem.(map[string]any)
			copy := cloneJSONMap(dialogue)
			copy["scene_id"] = sceneID
			sourceDialogueByID[fmt.Sprint(dialogue["dialogue_id"])] = copy
		}
	}
	for _, sceneItem := range currentScenes {
		scene, _ := sceneItem.(map[string]any)
		sceneID := fmt.Sprint(scene["scene_id"])
		currentSceneByID[sceneID] = scene
		for _, dialogueItem := range anySlice(scene["dialogues"]) {
			dialogue, _ := dialogueItem.(map[string]any)
			copy := cloneJSONMap(dialogue)
			copy["scene_id"] = sceneID
			currentDialogueByID[fmt.Sprint(dialogue["dialogue_id"])] = copy
		}
	}
	changes := make([]localedit.Change, 0)
	for _, id := range sortedJSONMapKeys(currentDialogueByID) {
		dialogue := currentDialogueByID[id]
		sourceDialogue := sourceDialogueByID[id]
		if sourceDialogue == nil || sourceDialogue["scene_id"] != dialogue["scene_id"] {
			changes = append(changes, localedit.Change{Operation: "remove", Field: "dialogue." + id})
		}
	}
	for _, id := range sortedJSONMapKeys(currentSceneByID) {
		if sourceSceneByID[id] == nil {
			changes = append(changes, localedit.Change{Operation: "remove", Field: "scene." + id})
		}
	}
	for _, id := range sortedJSONMapKeys(sourceSceneByID) {
		scene := sourceSceneByID[id]
		if currentSceneByID[id] == nil {
			value := cloneJSONMap(scene)
			value["dialogues"] = []any{}
			changes = append(changes, localedit.Change{Operation: "insert", Field: "scene." + id, Value: value})
		}
	}
	for _, id := range sortedJSONMapKeys(sourceDialogueByID) {
		dialogue := sourceDialogueByID[id]
		currentDialogue := currentDialogueByID[id]
		if currentDialogue == nil || currentDialogue["scene_id"] != dialogue["scene_id"] {
			changes = append(changes, localedit.Change{Operation: "insert", Field: "dialogue." + id, Value: dialogue})
		}
	}
	for _, sceneItem := range sourceScenes {
		scene, _ := sceneItem.(map[string]any)
		sceneID := fmt.Sprint(scene["scene_id"])
		if sceneID == "" || currentSceneByID[sceneID] == nil {
			continue
		}
		for _, field := range []string{
			"scene_number", "location_name", "time_of_day", "interior_exterior",
			"character_ids", "scene_purpose", "actions", "emotional_change", "estimated_duration_seconds",
		} {
			paths = append(paths, "scene."+sceneID+"."+field)
		}
		dialogues, _ := scene["dialogues"].([]any)
		for _, dialogueItem := range dialogues {
			dialogue, _ := dialogueItem.(map[string]any)
			dialogueID := fmt.Sprint(dialogue["dialogue_id"])
			currentDialogue := currentDialogueByID[dialogueID]
			if dialogueID == "" || currentDialogue == nil ||
				currentDialogue["scene_id"] != sceneID {
				continue
			}
			for _, field := range []string{
				"sequence_number", "dialogue_type", "speaker_name", "text", "emotion",
				"performance_instruction", "estimated_duration_ms",
			} {
				paths = append(paths, "dialogue."+dialogueID+"."+field)
			}
		}
	}
	for _, path := range paths {
		sourceValue, sourceOK := lookupVersionedField(source, "episode_content", path)
		currentValue, currentOK := lookupVersionedField(current, "episode_content", path)
		if sourceOK && currentOK &&
			!reflect.DeepEqual(normalizeJSONValue(sourceValue), normalizeJSONValue(currentValue)) {
			changes = append(changes, localedit.Change{
				Operation: "replace", Field: path, Value: sourceValue,
			})
		}
	}
	return changes, nil
}

func anySlice(value any) []any {
	result, _ := value.([]any)
	return result
}

func cloneJSONMap(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	result := map[string]any{}
	_ = json.Unmarshal(raw, &result)
	return result
}

func sortedJSONMapKeys(values map[string]map[string]any) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func readNativeEpisodeContentSnapshot(
	ctx context.Context, tx pgx.Tx, episodeID string,
) (json.RawMessage, error) {
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT episode_id FROM drama.episode_outlines
		WHERE episode_id=$1 FOR UPDATE`, episodeID).Scan(&lockedID); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	var result json.RawMessage
	err := tx.QueryRow(ctx, `SELECT jsonb_build_object(
		'outline',to_jsonb(outline)-'id'-'created_at'-'updated_at',
		'script',(SELECT (to_jsonb(script)-'id'-'created_at'-'updated_at')||
			jsonb_build_object('scenes',COALESCE((SELECT jsonb_agg(
				(to_jsonb(scene)-'id'-'created_at'-'updated_at')||
				jsonb_build_object('dialogues',COALESCE((SELECT jsonb_agg(
					to_jsonb(dialogue)-'id'-'created_at'-'updated_at'
					ORDER BY dialogue.sequence_number)
					FROM drama.dialogues dialogue WHERE dialogue.scene_id=scene.scene_id),'[]'::jsonb))
				ORDER BY scene.scene_number)
				FROM drama.script_scenes scene WHERE scene.script_id=script.script_id),'[]'::jsonb))
			FROM drama.episode_scripts script WHERE script.episode_id=outline.episode_id
			ORDER BY script.version DESC LIMIT 1))
		FROM drama.episode_outlines outline WHERE outline.episode_id=$1`,
		episodeID).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return result, err
}

func overlayEpisodeSnapshot(
	ctx context.Context, querier versionedRowsQuerier, projectID, episodeID string,
	content json.RawMessage,
) (json.RawMessage, error) {
	var snapshot map[string]any
	if err := json.Unmarshal(content, &snapshot); err != nil {
		return nil, err
	}
	outline, _ := snapshot["outline"].(map[string]any)
	script, _ := snapshot["script"].(map[string]any)
	scriptID := fmt.Sprint(script["script_id"])
	sceneIDs, dialogueIDs := map[string]bool{}, map[string]bool{}
	if scenes, ok := script["scenes"].([]any); ok {
		for _, sceneItem := range scenes {
			scene, _ := sceneItem.(map[string]any)
			sceneIDs[fmt.Sprint(scene["scene_id"])] = true
			if dialogues, exists := scene["dialogues"].([]any); exists {
				for _, dialogueItem := range dialogues {
					dialogue, _ := dialogueItem.(map[string]any)
					dialogueIDs[fmt.Sprint(dialogue["dialogue_id"])] = true
				}
			}
		}
	}
	rows, err := querier.Query(ctx, `SELECT entity_type,entity_id,version,content
		FROM drama.entity_versions
		WHERE project_id=$1 AND is_current
		  AND entity_type IN('outline','script','scene','dialogue')`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var entityType, entityID string
		var entityVersion int
		var entityContent json.RawMessage
		if err = rows.Scan(&entityType, &entityID, &entityVersion, &entityContent); err != nil {
			return nil, err
		}
		var node map[string]any
		if err = json.Unmarshal(entityContent, &node); err != nil {
			return nil, err
		}
		node["version"] = entityVersion
		switch {
		case entityType == "outline" && entityID == episodeID:
			outline, snapshot["outline"] = node, node
		case entityType == "script" && entityID == scriptID:
			if _, exists := node["scenes"]; !exists {
				node["scenes"] = script["scenes"]
			}
			script, snapshot["script"] = node, node
		case entityType == "scene" && sceneIDs[entityID]:
			if target := findScene(snapshot, entityID); target != nil {
				dialogues := target["dialogues"]
				for key := range target {
					delete(target, key)
				}
				for key, value := range node {
					target[key] = value
				}
				if _, exists := target["dialogues"]; !exists {
					target["dialogues"] = dialogues
				}
			}
		case entityType == "dialogue" && dialogueIDs[entityID]:
			if target := findDialogue(snapshot, entityID); target != nil {
				for key := range target {
					delete(target, key)
				}
				for key, value := range node {
					target[key] = value
				}
			}
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	_ = outline
	return json.Marshal(snapshot)
}

func (s *Store) overlayCreativeWorkbenchVersions(
	ctx context.Context, workbench *CreativeWorkbench,
) error {
	var episodeSnapshot json.RawMessage
	var episodeVersion int
	err := s.pool.QueryRow(ctx, `SELECT version,content FROM drama.entity_versions
		WHERE project_id=$1 AND entity_type='episode_content' AND entity_id=$2 AND is_current`,
		workbench.ProjectID, workbench.EpisodeID).Scan(&episodeVersion, &episodeSnapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		var scenes, dialogues []map[string]any
		if json.Unmarshal(workbench.Scenes, &scenes) != nil || json.Unmarshal(workbench.Dialogues, &dialogues) != nil {
			return fmt.Errorf("decode creative workbench content")
		}
		byScene := make(map[string][]map[string]any)
		for _, dialogue := range dialogues {
			sceneID := fmt.Sprint(dialogue["scene_id"])
			byScene[sceneID] = append(byScene[sceneID], dialogue)
		}
		sceneItems := make([]any, 0, len(scenes))
		for _, scene := range scenes {
			scene["dialogues"] = byScene[fmt.Sprint(scene["scene_id"])]
			sceneItems = append(sceneItems, scene)
		}
		var outline map[string]any
		if err = json.Unmarshal(workbench.Episode, &outline); err != nil {
			return err
		}
		episodeSnapshot, err = json.Marshal(map[string]any{
			"outline": outline,
			"script":  map[string]any{"scenes": sceneItems},
		})
		episodeVersion = 1
	} else if err != nil {
		return err
	}
	episodeSnapshot, err = overlayEpisodeSnapshot(
		ctx, s.pool, workbench.ProjectID, workbench.EpisodeID, episodeSnapshot,
	)
	if err != nil {
		return err
	}
	var episode map[string]any
	if err = json.Unmarshal(episodeSnapshot, &episode); err != nil {
		return err
	}
	if outline, ok := episode["outline"]; ok {
		workbench.Episode, _ = json.Marshal(outline)
	}
	script, _ := episode["script"].(map[string]any)
	sceneItems, _ := script["scenes"].([]any)
	flatDialogues := make([]any, 0)
	for _, sceneItem := range sceneItems {
		scene, _ := sceneItem.(map[string]any)
		if _, exists := scene["version"]; !exists {
			scene["version"] = episodeVersion
		}
		dialogues, _ := scene["dialogues"].([]any)
		for _, dialogueItem := range dialogues {
			dialogue, _ := dialogueItem.(map[string]any)
			if _, exists := dialogue["version"]; !exists {
				dialogue["version"] = episodeVersion
			}
			flatDialogues = append(flatDialogues, dialogue)
		}
		delete(scene, "dialogues")
	}
	workbench.Scenes, _ = json.Marshal(sceneItems)
	workbench.Dialogues, _ = json.Marshal(flatDialogues)

	var shots []map[string]any
	if err = json.Unmarshal(workbench.Shots, &shots); err != nil {
		return err
	}
	shotByID := make(map[string]map[string]any, len(shots))
	for _, shot := range shots {
		shotByID[fmt.Sprint(shot["shot_id"])] = shot
	}
	rows, err := s.pool.Query(ctx, `SELECT entity_id,version,content FROM drama.entity_versions
		WHERE project_id=$1 AND entity_type='shot' AND is_current`, workbench.ProjectID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var version int
		var content json.RawMessage
		if err = rows.Scan(&id, &version, &content); err != nil {
			rows.Close()
			return err
		}
		target := shotByID[id]
		if target == nil {
			continue
		}
		var current map[string]any
		if err = json.Unmarshal(content, &current); err != nil {
			rows.Close()
			return err
		}
		thumbnail := target["thumbnail_url"]
		for key := range target {
			delete(target, key)
		}
		for key, value := range current {
			target[key] = value
		}
		target["version"], target["thumbnail_url"] = version, thumbnail
	}
	rows.Close()
	workbench.Shots, _ = json.Marshal(shots)
	return rows.Err()
}

func readInheritedEpisodeSnapshot(
	ctx context.Context, tx pgx.Tx, entityType, entityID string,
) (json.RawMessage, bool, error) {
	episodeID, ok, err := episodeIDForVersionedEntity(ctx, tx, entityType, entityID)
	if err != nil || !ok {
		return nil, false, err
	}
	var content json.RawMessage
	err = tx.QueryRow(ctx, `SELECT content FROM drama.entity_versions
		WHERE entity_type='episode_content' AND entity_id=$1 AND is_current FOR UPDATE`,
		episodeID).Scan(&content)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var snapshot map[string]any
	if err = json.Unmarshal(content, &snapshot); err != nil {
		return nil, false, err
	}
	var value any
	switch entityType {
	case "outline":
		value = snapshot["outline"]
	case "script":
		value = snapshot["script"]
	case "scene":
		value = findScene(snapshot, entityID)
	case "dialogue":
		value = findDialogue(snapshot, entityID)
	default:
		return nil, false, nil
	}
	if value == nil {
		return nil, false, nil
	}
	result, err := json.Marshal(value)
	return result, true, err
}

func readInheritedEpisodeVersion(
	ctx context.Context, tx pgx.Tx, entityType, entityID string,
) (int, bool, error) {
	episodeID, ok, err := episodeIDForVersionedEntity(ctx, tx, entityType, entityID)
	if err != nil || !ok {
		return 0, false, err
	}
	var version int
	err = tx.QueryRow(ctx, `SELECT version FROM drama.entity_versions
		WHERE entity_type='episode_content' AND entity_id=$1 AND is_current FOR UPDATE`,
		episodeID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return version, err == nil, err
}

func episodeIDForVersionedEntity(
	ctx context.Context, tx pgx.Tx, entityType, entityID string,
) (string, bool, error) {
	var query string
	switch entityType {
	case "outline":
		return entityID, true, nil
	case "script":
		query = `SELECT episode_id FROM drama.episode_scripts WHERE script_id=$1`
	case "scene":
		query = `SELECT episode_id FROM drama.script_scenes WHERE scene_id=$1`
	case "dialogue":
		query = `SELECT episode_id FROM drama.dialogues WHERE dialogue_id=$1`
	default:
		return "", false, nil
	}
	var episodeID string
	err := tx.QueryRow(ctx, query, entityID).Scan(&episodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrNotFound
	}
	return episodeID, err == nil, err
}

func materializeTimelineChange(
	ctx context.Context, tx pgx.Tx, projectID, episodeID string,
	changes []localedit.Change, fingerprint string,
) (json.RawMessage, error) {
	var templateVersionID, scope, restoreSourceID string
	var templateConfig, overrideConfig json.RawMessage
	renderOverride := map[string]any{}
	for _, change := range changes {
		switch change.Field {
		case "editing_template_version_id":
			templateVersionID = strings.TrimSpace(fmt.Sprint(change.Value))
		case "template_scope":
			scope = strings.TrimSpace(fmt.Sprint(change.Value))
		case "restore_source_timeline_id":
			restoreSourceID = strings.TrimSpace(fmt.Sprint(change.Value))
		case "override_config":
			overrideConfig, _ = json.Marshal(change.Value)
		case "sound_style_group":
			renderOverride["sound_style_group"] = strings.TrimSpace(fmt.Sprint(change.Value))
		case "items":
			renderOverride["timeline_item_patch"] = change.Value
		}
	}
	if scope == "" {
		scope = "episode"
	}
	if templateVersionID != "" {
		if err := tx.QueryRow(ctx, `SELECT config FROM drama.editing_template_versions
			WHERE editing_template_version_id=$1 AND status='published'`,
			templateVersionID).Scan(&templateConfig); errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		} else if err != nil {
			return nil, err
		}
	}
	if len(renderOverride) > 0 {
		overrideConfig, _ = json.Marshal(renderOverride)
	}
	var bindingID string
	if templateVersionID != "" {
		var oldBindingID *string
		bindingEpisodeID := any(episodeID)
		if scope == "project" {
			bindingEpisodeID = nil
		}
		_ = tx.QueryRow(ctx, `SELECT editing_template_binding_id
			FROM drama.editing_template_bindings
			WHERE project_id=$1 AND episode_id IS NOT DISTINCT FROM $2::text AND is_current
			FOR UPDATE`, projectID, bindingEpisodeID).Scan(&oldBindingID)
		newBindingID, err := newPublicID("etb_")
		if err != nil {
			return nil, err
		}
		if oldBindingID != nil {
			if _, err = tx.Exec(ctx, `UPDATE drama.editing_template_bindings SET is_current=false
				WHERE editing_template_binding_id=$1`, *oldBindingID); err != nil {
				return nil, err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO drama.editing_template_bindings(
			editing_template_binding_id,project_id,episode_id,editing_template_version_id,
			version,parent_binding_id,override_config,is_current,change_reason)
			VALUES($1,$2,$3,$4,COALESCE((SELECT max(version)+1
				FROM drama.editing_template_bindings
				WHERE project_id=$2 AND episode_id IS NOT DISTINCT FROM $3::text),1),
				$5,COALESCE($6::jsonb,'{}'::jsonb),true,$7)`,
			newBindingID, projectID, bindingEpisodeID, templateVersionID, oldBindingID,
			nullableJSON(overrideConfig), "change_plan:"+fingerprint); err != nil {
			return nil, err
		}
		bindingID = newBindingID
	}
	timeline, err := cloneTimelineVersion(
		ctx, tx, projectID, episodeID, restoreSourceID, bindingID, templateVersionID,
		"change_plan:"+fingerprint, "draft", templateConfig, overrideConfig,
	)
	if err != nil {
		return nil, err
	}
	var result json.RawMessage
	err = tx.QueryRow(ctx, `SELECT to_jsonb(t)-'id'-'created_at'-'updated_at'
		FROM drama.edit_timelines t WHERE timeline_id=$1`, timeline.TimelineID).Scan(&result)
	return result, err
}

func resolvePendingRebuildTargets(
	ctx context.Context, tx pgx.Tx, action, entityType, entityID string,
	changes []localedit.Change, fallbackStart, fallbackEnd *int64,
) ([]pendingRebuildTarget, error) {
	targets := []pendingRebuildTarget{{
		EntityType: entityType, EntityID: entityID, StartMS: fallbackStart, EndMS: fallbackEnd,
	}}
	if entityType == "episode_content" {
		dialogueIDs, sceneIDs := []string{}, []string{}
		broadEpisodeChange := false
		for _, change := range changes {
			parts := strings.Split(change.Field, ".")
			if len(parts) >= 2 && parts[0] == "dialogue" {
				dialogueIDs = append(dialogueIDs, parts[1])
			}
			if len(parts) >= 2 && parts[0] == "scene" {
				sceneIDs = append(sceneIDs, parts[1])
			}
			if len(parts) < 2 || (parts[0] != "scene" && parts[0] != "dialogue") {
				broadEpisodeChange = true
			}
		}
		dialogueIDs, sceneIDs = uniqueVersionedEntityIDs(dialogueIDs), uniqueVersionedEntityIDs(sceneIDs)
		if !broadEpisodeChange && (len(dialogueIDs) > 0 || len(sceneIDs) > 0) {
			targets = nil
			if action != "update_continuity" && action != "regenerate_image" {
				for _, id := range dialogueIDs {
					resolved, err := resolvePendingRebuildTargets(
						ctx, tx, action, "dialogue", id, changes, fallbackStart, fallbackEnd,
					)
					if err != nil {
						return nil, err
					}
					targets = append(targets, resolved...)
				}
			}
			if action != "regenerate_voice" && action != "update_subtitle" {
				for _, id := range sceneIDs {
					resolved, err := resolvePendingRebuildTargets(
						ctx, tx, action, "scene", id, changes, fallbackStart, fallbackEnd,
					)
					if err != nil {
						return nil, err
					}
					targets = append(targets, resolved...)
				}
			}
			return dedupePendingTargets(targets), nil
		}
	}
	switch entityType {
	case "dialogue":
		startMS, endMS := matchingChangeRange(changes, entityType, entityID)
		if startMS == nil || endMS == nil {
			startMS, endMS = fallbackStart, fallbackEnd
		}
		if startMS == nil || endMS == nil {
			var start, end int64
			err := tx.QueryRow(ctx, `SELECT min(start_ms),max(end_ms) FROM drama.subtitle_cues
				WHERE dialogue_id=$1 AND is_current`, entityID).Scan(&start, &end)
			if err == nil && end > start {
				startMS, endMS = &start, &end
			}
		}
		switch action {
		case "regenerate_voice", "update_subtitle":
			return []pendingRebuildTarget{{
				EntityType: "dialogue", EntityID: entityID, StartMS: startMS, EndMS: endMS,
			}}, nil
		case "regenerate_video":
			rows, err := tx.Query(ctx, `SELECT DISTINCT shot.shot_id
				FROM drama.storyboard_shots shot
				LEFT JOIN drama.subtitle_cues cue ON cue.shot_id=shot.shot_id
				  AND cue.dialogue_id=$1 AND cue.is_current
				WHERE shot.dialogue_ids ? $1 OR cue.subtitle_cue_id IS NOT NULL
				ORDER BY shot.shot_id`, entityID)
			if err != nil {
				return nil, err
			}
			result := []pendingRebuildTarget{}
			for rows.Next() {
				var shotID string
				if err = rows.Scan(&shotID); err != nil {
					rows.Close()
					return nil, err
				}
				result = append(result, pendingRebuildTarget{
					EntityType: "storyboard_shot_interval", EntityID: shotID,
					StartMS: startMS, EndMS: endMS,
				})
			}
			rows.Close()
			if len(result) == 0 {
				result = append(result, pendingRebuildTarget{
					EntityType: "storyboard_shot_interval", EntityID: entityID,
					StartMS: startMS, EndMS: endMS,
				})
			}
			return result, rows.Err()
		case "recompose_timeline":
			var timelineID string
			err := tx.QueryRow(ctx, `SELECT timeline.timeline_id
				FROM drama.edit_timelines timeline
				JOIN drama.dialogues dialogue ON dialogue.episode_id=timeline.episode_id
				WHERE dialogue.dialogue_id=$1 AND timeline.is_current
				ORDER BY timeline.version DESC LIMIT 1`, entityID).Scan(&timelineID)
			if errors.Is(err, pgx.ErrNoRows) {
				timelineID = entityID
			} else if err != nil {
				return nil, err
			}
			return []pendingRebuildTarget{{
				EntityType: "edit_timeline_interval", EntityID: timelineID,
				StartMS: startMS, EndMS: endMS,
			}}, nil
		}
	case "scene":
		switch action {
		case "update_continuity":
			sourceNumber, targetNumber, reordered := sceneReorderRange(changes, entityID)
			if !reordered {
				source, sourceErr := readEntitySnapshot(ctx, tx, "scene", entityID)
				if sourceErr != nil {
					return nil, sourceErr
				}
				var sourceScene map[string]any
				if sourceErr = json.Unmarshal(source, &sourceScene); sourceErr != nil {
					return nil, sourceErr
				}
				sourceNumber, _ = numericValue(sourceScene["scene_number"])
			}
			var scriptID string
			err := tx.QueryRow(ctx, `SELECT script_id FROM drama.script_scenes
				WHERE scene_id=$1`, entityID).Scan(&scriptID)
			if errors.Is(err, pgx.ErrNoRows) {
				return []pendingRebuildTarget{{
					EntityType: "scene_continuity", EntityID: entityID,
				}}, nil
			}
			if err != nil {
				return nil, err
			}
			rows, err := tx.Query(ctx, `SELECT scene_id FROM drama.script_scenes
				WHERE script_id=$1 ORDER BY scene_number`, scriptID)
			if err != nil {
				return nil, err
			}
			sceneIDs := []string{}
			for rows.Next() {
				var sceneID string
				if err = rows.Scan(&sceneID); err != nil {
					rows.Close()
					return nil, err
				}
				sceneIDs = append(sceneIDs, sceneID)
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				return nil, err
			}
			result := []pendingRebuildTarget{}
			for _, sceneID := range sceneIDs {
				snapshot, snapshotErr := readEntitySnapshot(ctx, tx, "scene", sceneID)
				if snapshotErr != nil {
					return nil, snapshotErr
				}
				var scene map[string]any
				if snapshotErr = json.Unmarshal(snapshot, &scene); snapshotErr != nil {
					return nil, snapshotErr
				}
				number, ok := numericValue(scene["scene_number"])
				if !ok {
					continue
				}
				if reordered {
					nearSource := number >= sourceNumber-1 && number <= sourceNumber+1
					nearTarget := number >= targetNumber-1 && number <= targetNumber+1
					if !nearSource && !nearTarget {
						continue
					}
				} else if sceneID != entityID &&
					(number < sourceNumber-1 || number > sourceNumber+1) {
					continue
				}
				result = append(result, pendingRebuildTarget{
					EntityType: "scene_continuity", EntityID: sceneID,
				})
			}
			return result, nil
		case "regenerate_video":
			rows, err := tx.Query(ctx, `SELECT shot_id FROM drama.storyboard_shots
				WHERE scene_id=$1 ORDER BY shot_order`, entityID)
			if err != nil {
				return nil, err
			}
			result := []pendingRebuildTarget{}
			for rows.Next() {
				var shotID string
				if err = rows.Scan(&shotID); err != nil {
					rows.Close()
					return nil, err
				}
				result = append(result, pendingRebuildTarget{
					EntityType: "storyboard_shot_interval", EntityID: shotID,
				})
			}
			rows.Close()
			return result, rows.Err()
		case "recompose_timeline":
			var timelineID string
			err := tx.QueryRow(ctx, `SELECT timeline.timeline_id
				FROM drama.edit_timelines timeline
				JOIN drama.script_scenes scene ON scene.episode_id=timeline.episode_id
				WHERE scene.scene_id=$1 AND timeline.is_current
				ORDER BY timeline.version DESC LIMIT 1`, entityID).Scan(&timelineID)
			if errors.Is(err, pgx.ErrNoRows) {
				timelineID = entityID
			} else if err != nil {
				return nil, err
			}
			return []pendingRebuildTarget{{
				EntityType: "edit_timeline_interval", EntityID: timelineID,
			}}, nil
		}
	case "timeline":
		if action == "recompose_timeline" {
			var timelineID string
			err := tx.QueryRow(ctx, `SELECT timeline_id FROM drama.edit_timelines
				WHERE episode_id=$1 AND is_current ORDER BY version DESC LIMIT 1`, entityID).Scan(&timelineID)
			if err != nil {
				return nil, err
			}
			return []pendingRebuildTarget{{
				EntityType: "edit_timeline", EntityID: timelineID,
			}}, nil
		}
	}
	return targets, nil
}

func matchingChangeRange(
	changes []localedit.Change, entityType, entityID string,
) (*int64, *int64) {
	for _, change := range changes {
		matches := entityType != "dialogue"
		if entityType == "dialogue" {
			parts := strings.Split(change.Field, ".")
			matches = len(parts) < 2 || parts[0] != "dialogue" || parts[1] == entityID
		}
		if matches && change.StartMS != nil && change.EndMS != nil {
			return change.StartMS, change.EndMS
		}
	}
	return nil, nil
}

func sceneReorderRange(changes []localedit.Change, entityID string) (float64, float64, bool) {
	for _, change := range changes {
		field := change.Field
		parts := strings.Split(field, ".")
		if len(parts) == 3 && parts[0] == "scene" {
			if parts[1] != entityID {
				continue
			}
			field = parts[2]
		}
		if field != "scene_number" || change.Operation != "reorder" {
			continue
		}
		before, beforeOK := numericValue(change.Before)
		afterValue := change.After
		if afterValue == nil {
			afterValue = change.Value
		}
		after, afterOK := numericValue(afterValue)
		if beforeOK && afterOK {
			return before, after, true
		}
	}
	return 0, 0, false
}

func versionAdjacentScenes(
	ctx context.Context, tx pgx.Tx, projectID, planID, sceneID string,
	changes []localedit.Change,
) error {
	var sourceNumber, targetNumber float64
	hasReorder := false
	for _, change := range changes {
		if change.Field == "scene_number" && change.Operation == "reorder" {
			value := change.After
			if value == nil {
				value = change.Value
			}
			targetNumber, hasReorder = numericValue(value)
			sourceNumber, _ = numericValue(change.Before)
			break
		}
	}
	if !hasReorder || targetNumber < 1 {
		return nil
	}
	var scriptID string
	if err := tx.QueryRow(ctx, `SELECT script_id FROM drama.script_scenes
		WHERE scene_id=$1`, sceneID).Scan(&scriptID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT scene_id FROM drama.script_scenes
		WHERE script_id=$1 AND scene_id<>$2 ORDER BY scene_number FOR UPDATE`,
		scriptID, sceneID)
	if err != nil {
		return err
	}
	siblingIDs := []string{}
	for rows.Next() {
		var siblingID string
		if err = rows.Scan(&siblingID); err != nil {
			rows.Close()
			return err
		}
		siblingIDs = append(siblingIDs, siblingID)
	}
	rows.Close()
	for _, siblingID := range siblingIDs {
		before, readErr := readEntitySnapshot(ctx, tx, "scene", siblingID)
		if readErr != nil {
			return readErr
		}
		var content map[string]any
		if readErr = json.Unmarshal(before, &content); readErr != nil {
			return readErr
		}
		number, ok := numericValue(content["scene_number"])
		delta := float64(0)
		if targetNumber < sourceNumber && number >= targetNumber && number < sourceNumber {
			delta = 1
		}
		if targetNumber > sourceNumber && number <= targetNumber && number > sourceNumber {
			delta = -1
		}
		if !ok || delta == 0 {
			continue
		}
		var currentVersionID *string
		var currentVersion int
		readErr = tx.QueryRow(ctx, `SELECT entity_version_id,version
			FROM drama.entity_versions
			WHERE entity_type='scene' AND entity_id=$1 AND is_current FOR UPDATE`,
			siblingID).Scan(&currentVersionID, &currentVersion)
		if errors.Is(readErr, pgx.ErrNoRows) {
			currentVersion, readErr = readNativeEntityVersion(ctx, tx, "scene", siblingID)
			currentVersionID = nil
		}
		if readErr != nil {
			return readErr
		}
		beforeHash, _ := hashJSON(before)
		if currentVersionID == nil {
			originalID, idErr := newPublicID("ev_")
			if idErr != nil {
				return idErr
			}
			if _, readErr = tx.Exec(ctx, `INSERT INTO drama.entity_versions(
				entity_version_id,project_id,entity_type,entity_id,version,content,
				content_hash,semantic_hash,source_type,is_current)
				VALUES($1,$2,'scene',$3,$4,$5::jsonb,$6,$6,'generated',false)`,
				originalID, projectID, siblingID, currentVersion, before, beforeHash); readErr != nil {
				return readErr
			}
			currentVersionID = &originalID
		} else if _, readErr = tx.Exec(ctx, `UPDATE drama.entity_versions SET is_current=false
			WHERE entity_version_id=$1`, *currentVersionID); readErr != nil {
			return readErr
		}
		content["scene_number"] = number + delta
		after, _ := json.Marshal(content)
		afterHash, _ := hashJSON(after)
		successorID, idErr := newPublicID("ev_")
		if idErr != nil {
			return idErr
		}
		if _, readErr = tx.Exec(ctx, `INSERT INTO drama.entity_versions(
			entity_version_id,project_id,entity_type,entity_id,version,parent_entity_version_id,
			change_plan_id,content,content_hash,semantic_hash,source_type,source_metadata,is_current)
			VALUES($1,$2,'scene',$3,$4,$5,$6,$7::jsonb,$8,$8,'local_edit',
				jsonb_build_object('adjacent_reorder_of',$9::text),true)`,
			successorID, projectID, siblingID, currentVersion+1, currentVersionID,
			planID, after, afterHash, sceneID); readErr != nil {
			return readErr
		}
	}
	return nil
}

func dedupePendingTargets(values []pendingRebuildTarget) []pendingRebuildTarget {
	result := make([]pendingRebuildTarget, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		key := value.EntityType + ":" + value.EntityID + ":" +
			fmt.Sprint(value.StartMS) + ":" + fmt.Sprint(value.EndMS)
		if value.EntityID == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		result, err := typed.Float64()
		return result, err == nil
	case string:
		result, err := strconv.ParseFloat(typed, 64)
		return result, err == nil
	default:
		return 0, false
	}
}

func normalizeJSONValue(value any) any {
	raw, _ := json.Marshal(value)
	var normalized any
	_ = json.Unmarshal(raw, &normalized)
	return normalized
}

func uniqueVersionedEntityIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
