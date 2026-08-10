package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"short-drama-cms/backend/internal/scripteditor"
	"short-drama-cms/backend/internal/store"
)

type scriptAISelection struct {
	SceneIDs    []string `json:"scene_ids,omitempty"`
	DialogueIDs []string `json:"dialogue_ids,omitempty"`
	ActionIDs   []string `json:"action_ids,omitempty"`
}

type episodeContentAIChangePlanRequest struct {
	Draft       store.EpisodeContentChangePlanInput `json:"draft"`
	Selection   scriptAISelection                   `json:"selection"`
	Operation   string                              `json:"operation"`
	ConvertTo   string                              `json:"convert_to,omitempty"`
	Instruction string                              `json:"instruction,omitempty"`
}

func (h *Handler) getEpisodeRunContent(c *gin.Context) {
	result, err := h.store.GetEpisodeContent(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeRunID"),
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "EPISODE_CONTENT_NOT_FOUND", "单集内容不存在")
	case err != nil:
		respondError(c, http.StatusInternalServerError, "EPISODE_CONTENT_READ_FAILED", "单集内容读取失败："+err.Error())
	default:
		c.JSON(http.StatusOK, gin.H{"data": result})
	}
}

func (h *Handler) createEpisodeContentChangePlan(c *gin.Context) {
	var input store.EpisodeContentChangePlanInput
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_EPISODE_CONTENT", "单集内容格式无效："+err.Error())
		return
	}
	result, err := h.store.CreateEpisodeContentChangePlan(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeRunID"), input, input.RequestedBy,
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "EPISODE_CONTENT_NOT_FOUND", "单集内容不存在")
	case errors.Is(err, store.ErrInvalidEpisodeContent):
		respondError(c, http.StatusBadRequest, "INVALID_EPISODE_CONTENT", err.Error())
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "EPISODE_CONTENT_CONFLICT", err.Error())
	case err != nil:
		respondError(c, http.StatusInternalServerError, "EPISODE_CHANGE_PLAN_CREATE_FAILED", "修改计划创建失败："+err.Error())
	default:
		c.JSON(http.StatusCreated, gin.H{"data": result})
	}
}

func (h *Handler) createEpisodeContentAIChangePlan(c *gin.Context) {
	if h.scriptRewriter == nil {
		respondError(c, http.StatusServiceUnavailable, "SCRIPT_AI_UNAVAILABLE", "剧本 AI 服务不可用")
		return
	}
	var input episodeContentAIChangePlanRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_SCRIPT_AI_REQUEST", "AI 修改请求格式无效："+err.Error())
		return
	}
	current, err := h.store.GetEpisodeContent(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeRunID"),
	)
	if err != nil {
		respondError(c, http.StatusConflict, "EPISODE_CONTENT_CONFLICT", "读取当前剧本失败："+err.Error())
		return
	}
	blocks, err := selectedScriptBlocks(input.Draft, input.Selection)
	if err != nil {
		respondError(c, http.StatusUnprocessableEntity, "INVALID_SCRIPT_SELECTION", err.Error())
		return
	}
	preserve := append([]string{"原著事件", "Source Span", "人物关系", "因果与时空"}, input.Draft.MustPreserve...)
	result, err := h.scriptRewriter.Rewrite(c.Request.Context(), scripteditor.Request{
		Operation: input.Operation, ConvertTo: input.ConvertTo,
		Instruction: input.Instruction, Blocks: blocks,
		Context: current.ReferenceContext, MustPreserve: preserve,
	})
	if err != nil {
		status := http.StatusBadGateway
		code := "SCRIPT_AI_REWRITE_FAILED"
		if errors.Is(err, scripteditor.ErrInvalidRequest) {
			status, code = http.StatusUnprocessableEntity, "INVALID_SCRIPT_AI_RESULT"
		} else if errors.Is(err, scripteditor.ErrUnavailable) {
			status, code = http.StatusServiceUnavailable, "SCRIPT_AI_UNAVAILABLE"
		}
		respondError(c, status, code, err.Error())
		return
	}
	if err = applyScriptAIBlocks(&input.Draft, result.Blocks); err != nil {
		respondError(c, http.StatusUnprocessableEntity, "INVALID_SCRIPT_AI_RESULT", err.Error())
		return
	}
	input.Draft.Instruction = "AI " + input.Operation + "：仅修改当前选中范围"
	input.Draft.MustPreserve = preserve
	resultPlan, err := h.store.CreateEpisodeContentChangePlan(
		c.Request.Context(), c.Param("projectID"), c.Param("episodeRunID"),
		input.Draft, input.Draft.RequestedBy,
	)
	if err != nil {
		handleEpisodeContentPlanError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": resultPlan})
}

func handleEpisodeContentPlanError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		respondError(c, http.StatusNotFound, "EPISODE_CONTENT_NOT_FOUND", "单集内容不存在")
	case errors.Is(err, store.ErrInvalidEpisodeContent):
		respondError(c, http.StatusBadRequest, "INVALID_EPISODE_CONTENT", err.Error())
	case errors.Is(err, store.ErrConflict):
		respondError(c, http.StatusConflict, "EPISODE_CONTENT_CONFLICT", err.Error())
	default:
		respondError(c, http.StatusInternalServerError, "EPISODE_CHANGE_PLAN_CREATE_FAILED", err.Error())
	}
}

func selectedScriptBlocks(
	draft store.EpisodeContentChangePlanInput, selection scriptAISelection,
) ([]scripteditor.Block, error) {
	if draft.Script == nil {
		return nil, fmt.Errorf("script is required")
	}
	scenes := stringSet(selection.SceneIDs)
	dialogues := stringSet(selection.DialogueIDs)
	actions := stringSet(selection.ActionIDs)
	result := make([]scripteditor.Block, 0)
	for _, scene := range draft.Script.Scenes {
		wholeScene := scenes[scene.SceneID]
		var sceneActions []map[string]any
		_ = json.Unmarshal(scene.Actions, &sceneActions)
		for _, action := range sceneActions {
			id := strings.TrimSpace(fmt.Sprint(action["action_id"]))
			if id == "" || (!wholeScene && !actions[id]) {
				continue
			}
			result = append(result, scripteditor.Block{
				BlockID: id, BlockType: "action", Text: actionText(action),
			})
		}
		for _, dialogue := range scene.Dialogues {
			if !wholeScene && !dialogues[dialogue.DialogueID] {
				continue
			}
			result = append(result, scripteditor.Block{
				BlockID: dialogue.DialogueID, BlockType: dialogue.DialogueType,
				Text: dialogue.Text, SpeakerName: dialogue.SpeakerName,
				Emotion: dialogue.Emotion, PerformanceInstruction: dialogue.PerformanceInstruction,
			})
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("当前选中范围没有可供 AI 修改的动作、对白或旁白")
	}
	return result, nil
}

func applyScriptAIBlocks(draft *store.EpisodeContentChangePlanInput, blocks []scripteditor.Block) error {
	if draft.Script == nil {
		return fmt.Errorf("script is required")
	}
	byID := make(map[string]scripteditor.Block, len(blocks))
	for _, block := range blocks {
		byID[block.BlockID] = block
	}
	for sceneIndex := range draft.Script.Scenes {
		scene := &draft.Script.Scenes[sceneIndex]
		var actions []map[string]any
		if err := json.Unmarshal(scene.Actions, &actions); err != nil {
			return fmt.Errorf("decode actions: %w", err)
		}
		nextActions := make([]map[string]any, 0, len(actions))
		convertedDialogues := make([]store.EpisodeDialogueUpdate, 0)
		for _, action := range actions {
			id := strings.TrimSpace(fmt.Sprint(action["action_id"]))
			block, selected := byID[id]
			if !selected {
				nextActions = append(nextActions, action)
				continue
			}
			if block.BlockType == "action" {
				setActionText(action, block.Text)
				nextActions = append(nextActions, action)
				continue
			}
			convertedDialogues = append(convertedDialogues, store.EpisodeDialogueUpdate{
				DialogueID: id, DialogueType: block.BlockType, SpeakerName: block.SpeakerName,
				Text: strings.TrimSpace(block.Text), Emotion: block.Emotion,
				PerformanceInstruction: block.PerformanceInstruction,
				EstimatedDurationMS:    estimateDialogueMS(block.Text),
			})
		}
		nextDialogues := make([]store.EpisodeDialogueUpdate, 0, len(scene.Dialogues)+len(convertedDialogues))
		for _, dialogue := range scene.Dialogues {
			block, selected := byID[dialogue.DialogueID]
			if !selected {
				nextDialogues = append(nextDialogues, dialogue)
				continue
			}
			if block.BlockType == "action" {
				nextActions = append(nextActions, map[string]any{
					"action_id": dialogue.DialogueID, "description": strings.TrimSpace(block.Text),
					"source_dialogue_id": dialogue.DialogueID,
				})
				continue
			}
			dialogue.DialogueType = block.BlockType
			dialogue.Text = strings.TrimSpace(block.Text)
			dialogue.SpeakerName = strings.TrimSpace(block.SpeakerName)
			dialogue.Emotion = strings.TrimSpace(block.Emotion)
			dialogue.PerformanceInstruction = strings.TrimSpace(block.PerformanceInstruction)
			dialogue.EstimatedDurationMS = estimateDialogueMS(block.Text)
			nextDialogues = append(nextDialogues, dialogue)
		}
		nextDialogues = append(nextDialogues, convertedDialogues...)
		for index := range nextDialogues {
			nextDialogues[index].SequenceNumber = index + 1
		}
		scene.Dialogues = nextDialogues
		scene.Actions, _ = json.Marshal(nextActions)
	}
	return nil
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = true
		}
	}
	return result
}

func actionText(action map[string]any) string {
	for _, key := range []string{"description", "text", "visual"} {
		if value := strings.TrimSpace(fmt.Sprint(action[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return "未填写动作"
}

func setActionText(action map[string]any, text string) {
	for _, key := range []string{"description", "text", "visual"} {
		if _, exists := action[key]; exists {
			action[key] = strings.TrimSpace(text)
			return
		}
	}
	action["description"] = strings.TrimSpace(text)
}

func estimateDialogueMS(text string) int {
	value := len([]rune(strings.TrimSpace(text))) * 240
	if value < 500 {
		return 500
	}
	if value > 600000 {
		return 600000
	}
	return value
}
