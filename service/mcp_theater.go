package service

import (
	"context"
	"encoding/json"
	"strings"

	"sealchat/model"
)

// TheaterScope keeps chat context separate from the durable Theater room.
type TheaterScope struct {
	WorldID        string `json:"worldId"`
	ScopeType      string `json:"scopeType"`
	ChannelID      string `json:"channelId"`
	InputChannelID string `json:"inputChannelId,omitempty"`
}

func NormalizeTheaterMCPScope(actorID string, s TheaterScope) (TheaterScope, error) {
	s.WorldID, s.ChannelID, s.InputChannelID = strings.TrimSpace(s.WorldID), strings.TrimSpace(s.ChannelID), strings.TrimSpace(s.InputChannelID)
	if s.ScopeType != "world" && s.ScopeType != "channel" {
		return s, theaterPayloadError("scopeType 必须为 world/channel")
	}
	if (s.ScopeType == "world" && s.ChannelID != "") || (s.ScopeType == "channel" && s.ChannelID == "") {
		return s, theaterPayloadError("world Theater 的 channelId 必须为空；channel Theater 必须指定 channelId")
	}
	if _, _, err := requireTheaterPermission(actorID, s.WorldID, s.ChannelID, TheaterPermissionView); err != nil {
		return s, err
	}
	if s.InputChannelID != "" {
		_, ch, err := resolveTheaterScope(s.WorldID, s.InputChannelID)
		if err != nil {
			return s, err
		}
		if ch == nil || !CanReadChannelByUserId(actorID, ch.ID) {
			return s, newTheaterError(TheaterErrorPermissionDenied, "无法访问 inputChannelId", 403, nil)
		}
	}
	return s, nil
}

// Local scene patches are merged at precisely the caller's revision. Native CAS
// remains the final authority; an intervening write causes a conflict, not a rebase.
// SessionID records an intent fingerprint so retries reuse the original merged
// payload even if the scene has changed since the successful first call.
func ApplyTheaterMCPMutation(ctx context.Context, actorID, credentialID string, command TheaterMutationCommand, statePatch map[string]any) (*TheaterMutationResult, error) {
	if command.Type == TheaterMutationDesignApply {
		_, normalized, err := decodeTheaterPayload(command.Type, command.Payload)
		if err != nil {
			return nil, err
		}
		command.Payload = normalized
	}
	intent, err := json.Marshal(struct {
		Command TheaterMutationCommand
		Patch   map[string]any
	}{command, statePatch})
	if err != nil {
		return nil, err
	}
	meta := TheaterRequestMeta{Source: "mcp", RequestID: command.MutationID, SessionID: "mcp:" + credentialID + ":" + theaterJSONHash(intent)}
	if command.MutationID == "" || len(command.MutationID) > 128 || command.ExpectedRevision < 0 {
		return nil, theaterPayloadError("mutationId/expectedRevision 无效")
	}
	if _, _, err := requireTheaterPermission(actorID, command.WorldID, command.ChannelID, TheaterPermissionView); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomFindByScope(command.WorldID, command.ChannelID)
	if err != nil {
		return nil, err
	}
	var existing *model.TheaterMutationModel
	if room != nil {
		existing, err = model.TheaterMutationFindByID(room.ID, command.MutationID)
		if err != nil {
			return nil, err
		}
	}
	if existing != nil {
		if existing.ActorUserID != actorID || existing.RequestSource != "mcp" || existing.SessionID != meta.SessionID {
			return nil, newTheaterError(TheaterErrorMutationIDReused, "mutationId 已用于不同请求；冲突后须使用新 ID", 409, nil)
		}
		command.Payload = json.RawMessage(existing.PayloadJSON)
		return ApplyTheaterMutation(ctx, actorID, command, meta)
	}
	if statePatch != nil && (command.Type == TheaterMutationObjectUpdate || command.Type == TheaterMutationObjectBatchUpdate) {
		command.Payload, err = mergeTheaterMCPObjectMetadata(ctx, actorID, command, statePatch)
		if err != nil {
			return nil, err
		}
		return ApplyTheaterMutation(ctx, actorID, command, meta)
	}
	if statePatch != nil {
		if command.Type != TheaterMutationSceneUpdate {
			return nil, theaterPayloadError("局部 state 仅支持 scene.update")
		}
		if _, _, err := requireTheaterPermission(actorID, command.WorldID, command.ChannelID, TheaterPermissionObjectEdit); err != nil {
			return nil, err
		}
		snapshot, err := GetTheaterSnapshot(ctx, actorID, command.WorldID, command.ChannelID, TheaterSnapshotOptions{})
		if err != nil {
			return nil, err
		}
		var payload theaterSceneUpdatePayload
		if err := decodeStrictJSON(command.Payload, &payload); err != nil {
			return nil, theaterPayloadError("scene patch 无效")
		}
		scene, ok := snapshot.Snapshot.Scenes[payload.SceneID]
		if !ok {
			return nil, newTheaterError(TheaterErrorNotFound, "场景不存在", 404, nil)
		}
		var state map[string]any
		if err := json.Unmarshal(scene.State, &state); err != nil {
			return nil, err
		}
		if state == nil {
			state = map[string]any{}
		}
		if snapshot.Revision != command.ExpectedRevision {
			// No patch is merged into a different revision. Supply a valid native
			// payload solely to record the CAS rejection and original intent.
			payload.Fields["state"] = state
			command.Payload, err = json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			return ApplyTheaterMutation(ctx, actorID, command, meta)
		}
		if _, embeds := statePatch["surfaceEmbeds"]; embeds && (snapshot.Snapshot.ActiveSceneID == nil || *snapshot.Snapshot.ActiveSceneID != payload.SceneID) {
			return nil, theaterPayloadError("surface_embed_set/clear 仅支持当前场景")
		}
		mergeTheaterMCPState(state, statePatch)
		payload.Fields["state"] = state
		command.Payload, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		// Applying the stale expected revision through native mutation records the
		// rejection and preserves its idempotent retry semantics.
	}
	return ApplyTheaterMutation(ctx, actorID, command, meta)
}

// mergeTheaterMCPObjectMetadata applies patch["objectMetadata"][objectId] keys
// onto each target object's metadata read at the caller's revision, preserving
// every other metadata key. A stale revision keeps the current metadata so the
// native CAS records the conflict instead of rebasing the patch.
func mergeTheaterMCPObjectMetadata(ctx context.Context, actorID string, command TheaterMutationCommand, patch map[string]any) (json.RawMessage, error) {
	objectPatches, ok := patch["objectMetadata"].(map[string]any)
	if !ok || len(patch) != 1 || len(objectPatches) == 0 {
		return nil, theaterPayloadError("对象 metadata 局部更新无效")
	}
	if _, _, err := requireTheaterPermission(actorID, command.WorldID, command.ChannelID, TheaterPermissionObjectEdit); err != nil {
		return nil, err
	}
	snapshot, err := GetTheaterSnapshot(ctx, actorID, command.WorldID, command.ChannelID, TheaterSnapshotOptions{})
	if err != nil {
		return nil, err
	}
	var updates []theaterObjectUpdatePayload
	var batch theaterObjectBatchUpdatePayload
	if command.Type == TheaterMutationObjectUpdate {
		var single theaterObjectUpdatePayload
		if err := decodeStrictJSON(command.Payload, &single); err != nil {
			return nil, theaterPayloadError("object patch 无效")
		}
		updates = []theaterObjectUpdatePayload{single}
	} else {
		if err := decodeStrictJSON(command.Payload, &batch); err != nil {
			return nil, theaterPayloadError("object patch 无效")
		}
		updates = batch.Updates
	}
	applied := map[string]bool{}
	for index := range updates {
		changes, ok := objectPatches[updates[index].ObjectID].(map[string]any)
		if !ok {
			continue
		}
		object, found := snapshot.Snapshot.PersistentObjects[updates[index].ObjectID]
		for _, scene := range snapshot.Snapshot.Scenes {
			if found {
				break
			}
			object, found = scene.Objects[updates[index].ObjectID]
		}
		if !found {
			return nil, newTheaterError(TheaterErrorNotFound, "对象不存在", 404, nil)
		}
		metadata := map[string]any{}
		if len(object.Metadata) > 0 {
			if err := json.Unmarshal(object.Metadata, &metadata); err != nil || metadata == nil {
				metadata = map[string]any{}
			}
		}
		if snapshot.Revision == command.ExpectedRevision {
			for key, value := range changes {
				metadata[key] = value
			}
		}
		if updates[index].Fields == nil {
			updates[index].Fields = map[string]any{}
		}
		updates[index].Fields["metadata"] = metadata
		applied[updates[index].ObjectID] = true
	}
	if len(applied) != len(objectPatches) {
		return nil, theaterPayloadError("对象 metadata 局部更新目标无效")
	}
	if command.Type == TheaterMutationObjectUpdate {
		return json.Marshal(updates[0])
	}
	batch.Updates = updates
	return json.Marshal(batch)
}

func mergeTheaterMCPState(state, patch map[string]any) {
	for k, v := range patch {
		if k == "surfaceEmbeds" || k == "surfaceStyles" || k == "grid" {
			target, _ := state[k].(map[string]any)
			if target == nil {
				target = map[string]any{}
			}
			if k == "surfaceEmbeds" {
				for _, name := range []string{"background", "foreground"} {
					if _, ok := target[name]; !ok {
						target[name] = nil
					}
				}
			} else if k == "surfaceStyles" {
				// Match StageStore's normalization for a scene with no styles yet.
				for _, name := range []string{"background", "foreground"} {
					if _, ok := target[name]; !ok {
						opacity, blur := 1.0, 0.0
						if name == "background" {
							opacity, blur = 0.9, 10
						}
						fit := "cover"
						if v, ok := state["fieldObjectFit"].(string); ok {
							fit = v
						}
						target[name] = map[string]any{"brightness": 1, "blurPx": blur, "opacity": opacity, "zoom": 1, "fit": fit, "overlay": map[string]any{"enabled": false, "color": "#000000", "opacity": 0.4}}
					}
				}
			}
			if changes, ok := v.(map[string]any); ok {
				for name, value := range changes {
					target[name] = value
				}
				state[k] = target
				continue
			}
		}
		state[k] = v
	}
}

func TheaterMCPLimits() map[string]any {
	return map[string]any{"snapshotBytes": theaterMaxSnapshotBytes, "payloadBytes": theaterMaxPayloadBytes, "sceneStateBytes": 64 << 10, "objectJSONBytes": theaterMaxObjectJSONBytes, "effectWebHTMLBytes": theaterEffectWebHTMLMaxBytes, "effectWebPayloadBytes": theaterMaxWebEffectPayloadBytes, "scenes": theaterMaxScenes, "objects": theaterMaxObjects, "sceneObjects": theaterMaxSceneObjects, "batchUpdates": theaterMaxBatchUpdates, "actions": theaterMaxActions,
		"designPlanSteps": theaterMaxDesignSteps, "designPayloadBytes": theaterMaxPayloadBytes,
		"layoutModes":     map[string]any{"align": []string{"left", "center_x", "right", "top", "center_y", "bottom"}, "distribute": []string{"horizontal", "vertical"}, "grid": "objectIds order; same scope and parent; rotated AABB"},
		"duplicateLimits": map[string]int{"scenesPerRequest": 1, "rootsPerRequest": 1, "subtreeObjects": theaterMaxSceneObjects},
		"sceneSequences":  64, "sequenceSteps": 32, "sequenceTriggers": 32, "sequenceLoopCount": [2]int{1, 100}, "sequenceDelayMs": [2]int{0, 60000},
		"objectPositionRotationZ": "finite JSON numbers; no additional native range", "objectNameCharacters": 512, "objectOrderKeyBytes": 128, "cameraXY": [2]int{-1_000_000, 1_000_000}, "cameraZoom": [2]float64{0.01, 100},
		"worldUnitPx": 24, "anchor": "center", "objectSize": [2]float64{0, 1_000_000}, "objectScale": [2]float64{0.01, 100}, "iframeScale": [2]float64{0.25, 5}, "effectDesignSize": [2]int{1920, 1080}, "effectX": [2]int{-1920, 1920}, "effectY": [2]int{-1080, 1080}, "captureMaxEdge": 1600, "captureMaxBytes": 2 << 20}
}

// The native scene state is extensible. MCP only accepts the existing frontend
// sequencer contract, rather than relying on that extensibility for validation.
func ValidateTheaterMCPSequences(raw json.RawMessage) error {
	var sequences []struct {
		Version   int    `json:"version"`
		ID        string `json:"id"`
		Name      string `json:"name"`
		Enabled   bool   `json:"enabled"`
		LoopCount int    `json:"loopCount"`
		Triggers  []struct {
			ID              string   `json:"id"`
			Type            string   `json:"type"`
			Threshold       int      `json:"threshold"`
			Every           int      `json:"every"`
			CooldownMS      int      `json:"cooldownMs"`
			Keywords        []string `json:"keywords,omitempty"`
			TargetActorName *string  `json:"targetActorName,omitempty"`
			ObjectID        string   `json:"objectId,omitempty"`
		} `json:"triggers"`
		Steps []theaterStoredSequenceStep `json:"steps"`
	}
	if decodeStrictJSON(raw, &sequences) != nil || sequences == nil || len(sequences) > 64 {
		return theaterPayloadError("sequences 无效或超过 64 项")
	}
	seen := map[string]bool{}
	for _, seq := range sequences {
		if validateTheaterID(seq.ID, "sequence.id") != nil || seen[seq.ID] || seq.Version != 1 || len([]rune(seq.Name)) > 128 || seq.LoopCount < 1 || seq.LoopCount > 100 || len(seq.Triggers) > 32 || len(seq.Steps) > 32 {
			return theaterPayloadError("sequence 字段无效或超限")
		}
		seen[seq.ID] = true
		triggerIDs := map[string]bool{}
		for _, trigger := range seq.Triggers {
			if validateTheaterID(trigger.ID, "trigger.id") != nil || triggerIDs[trigger.ID] || trigger.Threshold < 1 || trigger.Threshold > 65535 || trigger.Every < 1 || trigger.Every > 65535 || trigger.CooldownMS < 0 || trigger.CooldownMS > 300000 {
				return theaterPayloadError("sequence trigger 无效")
			}
			triggerIDs[trigger.ID] = true
			switch trigger.Type {
			case "message":
				if trigger.ObjectID != "" || len(trigger.Keywords) > 32 || (trigger.TargetActorName != nil && len([]rune(*trigger.TargetActorName)) > 512) {
					return theaterPayloadError("message trigger 无效")
				}
				for _, keyword := range trigger.Keywords {
					if len([]rune(keyword)) > 128 {
						return theaterPayloadError("trigger keyword 超限")
					}
				}
			case "component.click":
				if validateTheaterID(trigger.ObjectID, "trigger.objectId") != nil || len(trigger.Keywords) != 0 || trigger.TargetActorName != nil {
					return theaterPayloadError("component.click trigger 无效")
				}
			default:
				return theaterPayloadError("sequence trigger type 无效")
			}
		}
		stepIDs := map[string]bool{}
		for _, step := range seq.Steps {
			if validateTheaterID(step.ID, "step.id") != nil || stepIDs[step.ID] || step.Action.ID != "" || step.Action.Schedule != nil {
				return theaterPayloadError("sequence step 无效")
			}
			stepIDs[step.ID] = true
			if step.SceneID != nil && *step.SceneID != "" && validateTheaterID(*step.SceneID, "step.sceneId") != nil {
				return theaterPayloadError("sequence step sceneId 无效")
			}
			if err := validateTheaterMCPTiming(step.Timing); err != nil {
				return err
			}
			switch step.Action.Type {
			case "scene.apply", "effect.play":
				if err := validateTheaterAtomicAction(step.Action); err != nil {
					return err
				}
			case "object.trigger":
				var payload struct {
					ObjectID string `json:"objectId"`
				}
				if decodeStrictJSON(step.Action.Payload, &payload) != nil || validateTheaterID(payload.ObjectID, "object.trigger.objectId") != nil {
					return theaterPayloadError("object.trigger 无效")
				}
			default:
				return theaterPayloadError("场景序列动作类型不支持")
			}
		}
	}
	return nil
}

func validateTheaterMCPTiming(raw json.RawMessage) error {
	var timing struct {
		Mode    string `json:"mode"`
		DelayMS *int   `json:"delayMs,omitempty"`
	}
	if decodeStrictJSON(raw, &timing) != nil {
		return theaterPayloadError("sequence timing 无效")
	}
	switch timing.Mode {
	case "after", "sync":
		if timing.DelayMS != nil {
			return theaterPayloadError("after/sync 不接受 delayMs")
		}
	case "delay":
		if timing.DelayMS == nil || *timing.DelayMS < 0 || *timing.DelayMS > 60000 {
			return theaterPayloadError("sequence delayMs 范围 0..60000")
		}
	default:
		return theaterPayloadError("sequence timing.mode 无效")
	}
	return nil
}
