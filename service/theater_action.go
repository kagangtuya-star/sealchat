package service

import (
	"context"
	"encoding/json"
	"strings"

	"sealchat/model"
)

type theaterStoredAction struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
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
	if !object.Visible || !object.Interactive || (object.Kind != "image" && object.Kind != "button") {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "对象未开放成员交互", 403, nil)
	}
	var actions []theaterStoredAction
	if err := json.Unmarshal([]byte(object.ActionsJSON), &actions); err != nil {
		return nil, theaterPayloadError("对象 actions 无效")
	}
	var selected *theaterStoredAction
	for index := range actions {
		if actions[index].ID == command.ActionID {
			selected = &actions[index]
			break
		}
	}
	if selected == nil {
		return nil, newTheaterError(TheaterErrorNotFound, "StageAction 不存在", 404, nil)
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
		result, err := ApplyTheaterMutation(ctx, actorID, TheaterMutationCommand{MutationID: mutationID, WorldID: command.WorldID, ChannelID: command.ChannelID, ExpectedRevision: command.ExpectedRevision, Type: TheaterMutationSceneApply, Payload: raw}, meta)
		if err != nil {
			return nil, err
		}
		return &TheaterActionResult{Kind: "mutation", Mutation: result}, nil
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
		result, err := ApplyTheaterMutation(ctx, actorID, TheaterMutationCommand{MutationID: mutationID, WorldID: command.WorldID, ChannelID: command.ChannelID, ExpectedRevision: command.ExpectedRevision, Type: TheaterMutationObjectToggle, Payload: raw}, meta)
		if err != nil {
			return nil, err
		}
		return &TheaterActionResult{Kind: "mutation", Mutation: result}, nil
	case "chat.insert":
		return &TheaterActionResult{Kind: "local", Descriptor: selected.Payload}, nil
	case "chat.send":
		chat, err := sendTheaterChat(ctx, actorID, command.WorldID, command.ChannelID, mutationID, selected.Payload)
		if err != nil {
			return nil, err
		}
		return &TheaterActionResult{Kind: "chat", Chat: chat}, nil
	default:
		return nil, newTheaterError(TheaterErrorMutationTypeUnsupported, "未知 StageAction", 400, map[string]any{"type": selected.Type})
	}
}
