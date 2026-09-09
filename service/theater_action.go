package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"sealchat/model"
)

func theaterWorldClueActionError(err error) error {
	switch {
	case errors.Is(err, ErrWorldClueDenied):
		return newTheaterError(TheaterErrorPermissionDenied, "无权执行线索动作", 403, nil)
	case errors.Is(err, ErrWorldClueNotFound):
		return newTheaterError(TheaterErrorNotFound, "线索不存在或不可用", 404, nil)
	case errors.Is(err, ErrWorldClueInvalid):
		return newTheaterError(TheaterErrorPayloadInvalid, "线索动作配置已失效", 400, nil)
	case errors.Is(err, ErrWorldClueConflict):
		return newTheaterError(TheaterErrorRevisionConflict, "线索已被其他操作更新", 409, nil)
	default:
		return err
	}
}

type theaterStoredAction struct {
	ID       string                       `json:"id"`
	Type     string                       `json:"type"`
	Schedule *theaterStoredActionSchedule `json:"schedule,omitempty"`
	Payload  json.RawMessage              `json:"payload"`
}

type theaterStoredActionSchedule struct {
	LegacyDurationMS *int `json:"durationMs,omitempty"`
	DelayMS          int  `json:"delayMs"`
}

type theaterStoredSequenceStep struct {
	ID      string              `json:"id"`
	SceneID *string             `json:"sceneId"`
	Timing  json.RawMessage     `json:"timing"`
	Action  theaterStoredAction `json:"action"`
}

type theaterStoredSequencePayload struct {
	Version int                         `json:"version"`
	Name    string                      `json:"name"`
	Steps   []theaterStoredSequenceStep `json:"steps"`
}

func isTheaterActionTargetKind(kind string) bool {
	return kind == "drawing" || kind == "text" || kind == "image" || kind == "button"
}

func TriggerTheaterAction(ctx context.Context, actorID string, command TheaterActionCommand, meta TheaterRequestMeta) (*TheaterActionResult, error) {
	if _, _, err := requireTheaterPermission(actorID, command.WorldID, command.ChannelID, TheaterPermissionActionTrigger); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(command.WorldID, command.ChannelID, actorID)
	if err != nil {
		return nil, err
	}
	object, err := loadTheaterObject(model.GetDB(), room.ID, command.ObjectID)
	if err != nil {
		return nil, err
	}
	// Visibility controls hit testing in the client. Do not use it as an
	// execution precondition: an action may hide its own source before later
	// actions from the same click (for example, chat-driven effects) run.
	if !object.Interactive || !isTheaterActionTargetKind(object.Kind) {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "对象未开放成员交互", 403, nil)
	}
	if err := validateTheaterActions(json.RawMessage(object.ActionsJSON)); err != nil {
		return nil, err
	}
	var actions []theaterStoredAction
	if err := json.Unmarshal([]byte(object.ActionsJSON), &actions); err != nil {
		return nil, theaterPayloadError("对象 actions 无效")
	}
	var selected *theaterStoredAction
	var selectedSceneID *string
	for index := range actions {
		if actions[index].ID == command.ActionID {
			selected = &actions[index]
			break
		}
	}
	if selected == nil {
		return nil, newTheaterError(TheaterErrorNotFound, "StageAction 不存在", 404, nil)
	}
	if selected.Type == "action.sequence" {
		stepID := strings.TrimSpace(command.StepID)
		if stepID == "" {
			return nil, theaterPayloadError("action.sequence 缺少 stepId")
		}
		var sequence theaterStoredSequencePayload
		if err := decodeStrictJSON(selected.Payload, &sequence); err != nil || sequence.Version != 1 {
			return nil, theaterPayloadError("action.sequence payload 无效")
		}
		selected = nil
		for index := range sequence.Steps {
			if sequence.Steps[index].ID == stepID {
				selected = &sequence.Steps[index].Action
				selectedSceneID = sequence.Steps[index].SceneID
				break
			}
		}
		if selected == nil {
			return nil, newTheaterError(TheaterErrorNotFound, "StageAction step 不存在", 404, nil)
		}
	} else if strings.TrimSpace(command.StepID) != "" {
		return nil, theaterPayloadError("普通 StageAction 不能包含 stepId")
	}
	if selected.Type != "clue.execute" && strings.TrimSpace(command.EntryID) != "" {
		return nil, theaterPayloadError("普通 StageAction 不能包含 entryId")
	}
	mutationID := strings.TrimSpace(command.ActionRequestID)
	if mutationID == "" {
		return nil, theaterPayloadError("actionRequestId 必填")
	}
	switch selected.Type {
	case TheaterMutationSceneApply:
		var payload theaterSceneApplyPayload
		if err := decodeStrictJSON(selected.Payload, &payload); err != nil {
			return nil, theaterPayloadError(err.Error())
		}
		raw, _ := json.Marshal(payload)
		result, err := applyTheaterActionMutation(ctx, actorID, TheaterMutationCommand{MutationID: mutationID, WorldID: command.WorldID, ChannelID: command.ChannelID, ExpectedRevision: command.ExpectedRevision, Type: TheaterMutationSceneApply, Payload: raw}, meta)
		if err != nil {
			return nil, err
		}
		return &TheaterActionResult{Kind: "mutation", Mutation: result}, nil
	case "effect.play":
		var payload theaterEffectPlayPayload
		if err := decodeStrictJSON(selected.Payload, &payload); err != nil {
			return nil, theaterPayloadError("effect.play action payload 无效")
		}
		target, err := loadTheaterObject(model.GetDB(), room.ID, strings.TrimSpace(payload.EffectID))
		if err != nil {
			return nil, err
		}
		if target.Kind != "effect" || (target.SceneID != "" && target.SceneID != room.ActiveSceneID) {
			return nil, newTheaterError(TheaterErrorNotFound, "可播放特效不存在", 404, nil)
		}
		if selectedSceneID != nil && strings.TrimSpace(*selectedSceneID) != "" && target.SceneID != "" && target.SceneID != strings.TrimSpace(*selectedSceneID) {
			return nil, newTheaterError(TheaterErrorNotFound, "特效不属于动作声明场景", 404, nil)
		}
		if !target.Visible {
			return nil, newTheaterError(TheaterErrorNotFound, "特效未启用", 404, nil)
		}
		return &TheaterActionResult{Kind: "effect", Effect: &TheaterEffectActionResult{
			TriggerID: mutationID, EffectID: target.ID, SceneID: target.SceneID, RoomID: room.ID, Revision: room.Revision,
		}}, nil
	case TheaterMutationObjectToggle:
		var payload theaterObjectTogglePayload
		if len(selected.Payload) > 0 {
			if err := decodeStrictJSON(selected.Payload, &payload); err != nil {
				return nil, theaterPayloadError(err.Error())
			}
		}
		if strings.TrimSpace(payload.ObjectID) == "" {
			return nil, theaterPayloadError("object.toggle action 缺少 objectId")
		}
		raw, _ := json.Marshal(payload)
		result, err := applyTheaterActionMutation(ctx, actorID, TheaterMutationCommand{MutationID: mutationID, WorldID: command.WorldID, ChannelID: command.ChannelID, ExpectedRevision: command.ExpectedRevision, Type: TheaterMutationObjectToggle, Payload: raw}, meta)
		if err != nil {
			return nil, err
		}
		return &TheaterActionResult{Kind: "mutation", Mutation: result}, nil
	case "chat.insert":
		return &TheaterActionResult{Kind: "local", Descriptor: selected.Payload}, nil
	case "chat.send":
		inputChannelID := strings.TrimSpace(command.InputChannelID)
		if inputChannelID == "" {
			inputChannelID = strings.TrimSpace(command.ChannelID)
		}
		chat, err := sendTheaterChat(ctx, actorID, command.WorldID, inputChannelID, mutationID, selected.Payload)
		if err != nil {
			return nil, err
		}
		return &TheaterActionResult{Kind: "chat", Chat: chat}, nil
	case theaterActionChatRandomTable:
		inputChannelID := strings.TrimSpace(command.InputChannelID)
		if inputChannelID == "" {
			inputChannelID = strings.TrimSpace(command.ChannelID)
		}
		chat, err := sendTheaterRandomTable(ctx, actorID, command.WorldID, inputChannelID, mutationID, selected.Payload)
		if err != nil {
			return nil, err
		}
		return &TheaterActionResult{Kind: "chat", Chat: chat}, nil
	case "clue.execute":
		entryID := strings.TrimSpace(command.EntryID)
		if entryID == "" || len(entryID) > 128 {
			return nil, theaterPayloadError("clue.execute 缺少 entryId")
		}
		var payload theaterClueExecutePayload
		if err := decodeStrictJSON(selected.Payload, &payload); err != nil || payload.Version != 1 {
			return nil, theaterPayloadError("clue.execute action payload 无效")
		}
		var entry *theaterClueActionEntry
		for index := range payload.Entries {
			if payload.Entries[index].ID == entryID {
				entry = &payload.Entries[index]
				break
			}
		}
		if entry == nil {
			return nil, newTheaterError(TheaterErrorNotFound, "clue.execute entry 不存在", 404, nil)
		}
		targets := make([]WorldClueTheaterTarget, 0, len(entry.Targets))
		for _, target := range entry.Targets {
			targets = append(targets, WorldClueTheaterTarget{UserID: target.UserID, Access: target.Access})
		}
		clue, err := WorldClueExecuteTheaterEntry(command.WorldID, entry.ClueID, actorID, targets, entry.Present != nil && *entry.Present)
		if err != nil {
			return nil, theaterWorldClueActionError(err)
		}
		return &TheaterActionResult{Kind: "clue", Clue: &TheaterClueActionResult{ClueID: clue.ClueID, PublishSeq: clue.PublishSeq, RecipientIDs: clue.RecipientIDs, Revision: clue.Revision, Status: clue.Status}}, nil
	default:
		return nil, newTheaterError(TheaterErrorMutationTypeUnsupported, "未知 StageAction", 400, map[string]any{"type": selected.Type})
	}
}

// TriggerTheaterActionBatch applies independent visibility toggles from one
// click as one theater mutation. This gives every client one final snapshot
// instead of visibly replaying each toggle after the previous revision.
func TriggerTheaterActionBatch(ctx context.Context, actorID string, command TheaterActionBatchCommand, meta TheaterRequestMeta) (*TheaterActionResult, error) {
	if _, _, err := requireTheaterPermission(actorID, command.WorldID, command.ChannelID, TheaterPermissionActionTrigger); err != nil {
		return nil, err
	}
	command.ActionRequestID = strings.TrimSpace(command.ActionRequestID)
	if command.ActionRequestID == "" || len(command.ActionRequestID) > 128 {
		return nil, theaterPayloadError("actionRequestId 无效")
	}
	if len(command.ActionIDs) < 2 || len(command.ActionIDs) > 32 {
		return nil, theaterPayloadError("actionIds 数量无效")
	}
	room, err := model.TheaterRoomCreateIfMissing(command.WorldID, command.ChannelID, actorID)
	if err != nil {
		return nil, err
	}
	object, err := loadTheaterObject(model.GetDB(), room.ID, command.ObjectID)
	if err != nil {
		return nil, err
	}
	if !object.Interactive || !isTheaterActionTargetKind(object.Kind) {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "对象未开放成员交互", 403, nil)
	}
	if err := validateTheaterActions(json.RawMessage(object.ActionsJSON)); err != nil {
		return nil, err
	}
	var actions []theaterStoredAction
	if err := json.Unmarshal([]byte(object.ActionsJSON), &actions); err != nil {
		return nil, theaterPayloadError("对象 actions 无效")
	}
	actionByID := make(map[string]theaterStoredAction, len(actions))
	for _, action := range actions {
		actionByID[action.ID] = action
	}
	seenActions := make(map[string]struct{}, len(command.ActionIDs))
	visibleByObjectID := map[string]bool{}
	order := make([]string, 0, len(command.ActionIDs))
	for _, actionID := range command.ActionIDs {
		actionID = strings.TrimSpace(actionID)
		if actionID == "" {
			return nil, theaterPayloadError("actionId 无效")
		}
		if _, exists := seenActions[actionID]; exists {
			return nil, theaterPayloadError("actionIds 不能重复")
		}
		seenActions[actionID] = struct{}{}
		action, exists := actionByID[actionID]
		if !exists || action.Type != TheaterMutationObjectToggle {
			return nil, theaterPayloadError("批量动作仅支持 object.toggle")
		}
		var payload theaterObjectTogglePayload
		if err := decodeStrictJSON(action.Payload, &payload); err != nil || strings.TrimSpace(payload.ObjectID) == "" {
			return nil, theaterPayloadError("object.toggle action 无效")
		}
		targetID := strings.TrimSpace(payload.ObjectID)
		if _, loaded := visibleByObjectID[targetID]; !loaded {
			target, err := loadTheaterObject(model.GetDB(), room.ID, targetID)
			if err != nil {
				return nil, err
			}
			visibleByObjectID[targetID] = target.Visible
			order = append(order, targetID)
		}
		visibleByObjectID[targetID] = !visibleByObjectID[targetID]
	}
	updates := make([]theaterObjectUpdatePayload, 0, len(order))
	for _, objectID := range order {
		updates = append(updates, theaterObjectUpdatePayload{ObjectID: objectID, Fields: map[string]any{"visible": visibleByObjectID[objectID]}})
	}
	payload, err := json.Marshal(theaterObjectBatchUpdatePayload{Updates: updates})
	if err != nil {
		return nil, err
	}
	mutation, err := applyTheaterActionMutation(ctx, actorID, TheaterMutationCommand{
		MutationID: command.ActionRequestID, WorldID: command.WorldID, ChannelID: command.ChannelID,
		ExpectedRevision: command.ExpectedRevision, Type: TheaterMutationObjectBatchUpdate, Payload: payload,
	}, meta)
	if err != nil {
		return nil, err
	}
	return &TheaterActionResult{Kind: "mutation", Mutation: mutation}, nil
}
