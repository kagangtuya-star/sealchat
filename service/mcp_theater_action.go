package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"sealchat/model"
)

// Only scheduling metadata reaches the browser. Saved action payloads remain on
// the server until an authorized step produces a local visual descriptor.
type TheaterExecutionPlan struct {
	ID       string                 `json:"id"`
	DelayMS  int                    `json:"delayMs"`
	Timing   TheaterExecutionTiming `json:"timing"`
	Children []TheaterExecutionPlan `json:"children,omitempty"`
	StepID   string                 `json:"stepId,omitempty"`
	Confirm  bool                   `json:"confirm,omitempty"`
}
type TheaterExecutionTiming struct {
	Mode    string `json:"mode"`
	DelayMS int    `json:"delayMs,omitempty"`
}
type theaterMCPActionLeaf struct {
	command TheaterActionCommand
	action  theaterStoredAction
	direct  bool
}
type TheaterMCPExecution struct {
	Plan   TheaterExecutionPlan
	leaves map[string]theaterMCPActionLeaf
}

func PrepareTheaterMCPExecution(ctx context.Context, a *MCPActor, s TheaterScope, sceneID, objectID, actionID, sequenceID string, revision int64) (*TheaterMCPExecution, error) {
	if _, _, err := requireTheaterPermission(a.User.ID, s.WorldID, s.ChannelID, TheaterPermissionActionTrigger); err != nil {
		return nil, err
	}
	snap, err := GetTheaterSnapshot(ctx, a.User.ID, s.WorldID, s.ChannelID, TheaterSnapshotOptions{})
	if err != nil {
		return nil, err
	}
	if snap.Revision != revision {
		return nil, newTheaterError(TheaterErrorRevisionConflict, "Theater revision 冲突", 409, map[string]any{"currentRevision": snap.Revision})
	}
	if snap.Snapshot.ActiveSceneID == nil || *snap.Snapshot.ActiveSceneID != sceneID {
		return nil, rendererError("renderer_scene_mismatch", "执行必须针对当前场景")
	}
	room, err := model.TheaterRoomFindByScope(s.WorldID, s.ChannelID)
	if err != nil {
		return nil, err
	}
	e := &TheaterMCPExecution{leaves: map[string]theaterMCPActionLeaf{}}
	count := 0
	var buildObject func(string, string, map[string]bool, int) (TheaterExecutionPlan, error)
	var buildAction func(theaterStoredAction, TheaterActionCommand, bool, map[string]bool, int) (TheaterExecutionPlan, error)
	node := func() TheaterExecutionPlan {
		count++
		return TheaterExecutionPlan{ID: fmt.Sprintf("node-%d", count), Timing: TheaterExecutionTiming{Mode: "after"}}
	}
	buildAction = func(action theaterStoredAction, command TheaterActionCommand, direct bool, chain map[string]bool, depth int) (TheaterExecutionPlan, error) {
		n := node()
		if count > 256 || depth > 8 {
			return n, theaterPayloadError("动作展开超过 256 节点或 8 层")
		}
		if action.Schedule != nil {
			n.DelayMS = action.Schedule.DelayMS
		}
		if action.Type == "object.trigger" {
			var p struct {
				ObjectID string `json:"objectId"`
			}
			if decodeStrictJSON(action.Payload, &p) != nil {
				return n, theaterPayloadError("object.trigger 无效")
			}
			child, err := buildObject(p.ObjectID, "", chain, depth+1)
			n.Children = []TheaterExecutionPlan{child}
			return n, err
		}
		if action.Type == "action.sequence" {
			var p theaterStoredSequencePayload
			if decodeStrictJSON(action.Payload, &p) != nil || p.Version != 1 {
				return n, theaterPayloadError("sequence 无效")
			}
			for _, step := range p.Steps {
				ref := command
				ref.StepID = step.ID
				child, err := buildAction(step.Action, ref, direct, chain, depth+1)
				if err != nil {
					return n, err
				}
				if decodeStrictJSON(step.Timing, &child.Timing) != nil {
					return n, theaterPayloadError("sequence timing 无效")
				}
				n.Children = append(n.Children, child)
			}
			return n, nil
		}
		if err := validateTheaterAtomicAction(action); err != nil {
			return n, err
		}
		if err := theaterMCPActionScopes(a, action); err != nil {
			return n, err
		}
		if action.Type == "scene.apply" && !CanSwitchTheaterScene(a.User.ID, s.WorldID, s.ChannelID) {
			return n, newTheaterError(TheaterErrorPermissionDenied, "缺少场景切换权限", 403, nil)
		}
		if action.Type == "clue.execute" {
			var p theaterClueExecutePayload
			_ = json.Unmarshal(action.Payload, &p)
			for _, entry := range p.Entries {
				child := node()
				child.StepID = child.ID
				child.Confirm = entry.Confirm != nil && *entry.Confirm
				ref := command
				ref.EntryID = entry.ID
				e.leaves[child.StepID] = theaterMCPActionLeaf{ref, action, direct}
				n.Children = append(n.Children, child)
			}
		} else {
			n.StepID = n.ID
			e.leaves[n.StepID] = theaterMCPActionLeaf{command, action, direct}
		}
		return n, nil
	}
	buildObject = func(id, selected string, chain map[string]bool, depth int) (TheaterExecutionPlan, error) {
		n := node()
		if depth > 8 || count > 256 || chain[id] {
			return n, theaterPayloadError("object.trigger 存在循环或展开超限")
		}
		accessible := false
		if _, ok := snap.Snapshot.PersistentObjects[id]; ok {
			accessible = true
		}
		for _, scene := range snap.Snapshot.Scenes {
			if _, ok := scene.Objects[id]; ok {
				accessible = true
			}
		}
		if !accessible {
			return n, newTheaterError(TheaterErrorNotFound, "动作对象不存在或不可见", 404, nil)
		}
		object, err := loadTheaterObject(model.GetDB(), room.ID, id)
		if err != nil {
			return n, err
		}
		if !object.Interactive || !isTheaterActionTargetKind(object.Kind) {
			return n, newTheaterError(TheaterErrorPermissionDenied, "对象未开放交互", 403, nil)
		}
		if err := validateTheaterActions(json.RawMessage(object.ActionsJSON)); err != nil {
			return n, err
		}
		var actions []theaterStoredAction
		_ = json.Unmarshal([]byte(object.ActionsJSON), &actions)
		var metadata map[string]any
		_ = json.Unmarshal([]byte(object.MetadataJSON), &metadata)
		next := map[string]bool{}
		for k, v := range chain {
			next[k] = v
		}
		next[id] = true
		found := selected == ""
		for _, action := range actions {
			if selected != "" && action.ID != selected {
				continue
			}
			found = true
			command := TheaterActionCommand{WorldID: s.WorldID, ChannelID: s.ChannelID, InputChannelID: s.InputChannelID, ObjectID: id, ActionID: action.ID}
			child, err := buildAction(action, command, false, next, depth+1)
			if err != nil {
				return n, err
			}
			if metadata["actionExecutionMode"] != "sequential" {
				child.Timing.Mode = "sync"
			}
			n.Children = append(n.Children, child)
		}
		if !found {
			return n, newTheaterError(TheaterErrorNotFound, "StageAction 不存在", 404, nil)
		}
		return n, nil
	}
	if sequenceID == "" {
		e.Plan, err = buildObject(objectID, actionID, map[string]bool{}, 0)
	} else {
		scene, ok := snap.Snapshot.Scenes[sceneID]
		if !ok {
			return nil, newTheaterError(TheaterErrorNotFound, "场景不可见", 404, nil)
		}
		var state struct {
			Sequences []struct {
				ID        string                      `json:"id"`
				Version   int                         `json:"version"`
				Enabled   bool                        `json:"enabled"`
				LoopCount int                         `json:"loopCount"`
				Steps     []theaterStoredSequenceStep `json:"steps"`
			} `json:"theaterSequences"`
		}
		if json.Unmarshal(scene.State, &state) != nil {
			return nil, theaterPayloadError("场景序列无效")
		}
		found := false
		e.Plan = node()
		for _, seq := range state.Sequences {
			if seq.ID != sequenceID {
				continue
			}
			found = true
			if seq.Version != 1 || !seq.Enabled || seq.LoopCount < 1 || seq.LoopCount > 100 || len(seq.Steps) == 0 || len(seq.Steps) > 32 {
				return nil, theaterPayloadError("场景序列无效或未启用")
			}
			for loop := 0; loop < seq.LoopCount; loop++ {
				iteration := node()
				for _, step := range seq.Steps {
					if step.Action.Type != "scene.apply" && step.Action.Type != "effect.play" && step.Action.Type != "object.trigger" {
						return nil, theaterPayloadError("场景序列动作类型不支持")
					}
					if step.Action.Type == "effect.play" || step.Action.Type == "object.trigger" {
						var target struct {
							EffectID string `json:"effectId"`
							ObjectID string `json:"objectId"`
						}
						_ = json.Unmarshal(step.Action.Payload, &target)
						id := target.ObjectID
						if step.Action.Type == "effect.play" {
							id = target.EffectID
						}
						object, targetErr := loadTheaterObject(model.GetDB(), room.ID, id)
						declared := ""
						if step.SceneID != nil {
							declared = *step.SceneID
						}
						if targetErr != nil || object.SceneID != declared {
							return nil, theaterPayloadError("序列目标不属于声明场景")
						}
					}
					child, buildErr := buildAction(step.Action, TheaterActionCommand{WorldID: s.WorldID, ChannelID: s.ChannelID, InputChannelID: s.InputChannelID}, true, map[string]bool{}, 0)
					if buildErr != nil {
						return nil, buildErr
					}
					if validateTheaterMCPTiming(step.Timing) != nil || decodeStrictJSON(step.Timing, &child.Timing) != nil {
						return nil, theaterPayloadError("sequence timing 无效")
					}
					iteration.Children = append(iteration.Children, child)
				}
				e.Plan.Children = append(e.Plan.Children, iteration)
			}
		}
		if !found {
			return nil, newTheaterError(TheaterErrorNotFound, "场景序列不存在", 404, nil)
		}
	}
	if err != nil {
		return nil, err
	}
	if count > 256 {
		return nil, theaterPayloadError("动作展开超过 256 节点")
	}
	if len(e.leaves) == 0 {
		return nil, theaterPayloadError("没有可执行步骤")
	}
	return e, nil
}

func (e *TheaterMCPExecution) StepNeedsAudio(s TheaterScope, stepID string) (bool, error) {
	leaf, ok := e.leaves[stepID]
	if !ok || leaf.action.Type != "effect.play" {
		return false, nil
	}
	var p theaterEffectPlayPayload
	_ = json.Unmarshal(leaf.action.Payload, &p)
	room, err := model.TheaterRoomFindByScope(s.WorldID, s.ChannelID)
	if err != nil {
		return false, err
	}
	if room == nil {
		return false, theaterPayloadError("房间不存在")
	}
	target, err := loadTheaterObject(model.GetDB(), room.ID, p.EffectID)
	if err != nil {
		return false, err
	}
	var raw map[string]any
	_ = json.Unmarshal([]byte(target.ContentJSON), &raw)
	fx, _ := raw["effect"].(map[string]any)
	return fx["audio"] != nil, nil
}
func theaterMCPActionScopes(a *MCPActor, action theaterStoredAction) error {
	if !a.Allows("theater:control") {
		return ErrMCPScopeDenied
	}
	switch action.Type {
	case "chat.send", "chat.insert", theaterActionChatRandomTable:
		if !a.Allows("theater:chat") {
			return ErrMCPScopeDenied
		}
	case "clue.execute":
		if !a.Allows("clue:write", "clue:publish") {
			return ErrMCPScopeDenied
		}
	}
	return nil
}
func (e *TheaterMCPExecution) StepIDs() []string {
	out := []string{}
	for id := range e.leaves {
		out = append(out, id)
	}
	return out
}
func (e *TheaterMCPExecution) Execute(ctx context.Context, a *MCPActor, s TheaterScope, executionID, stepID string, revision int64) (*TheaterActionResult, error) {
	leaf, ok := e.leaves[stepID]
	if !ok {
		return nil, theaterPayloadError("未知执行步骤")
	}
	if _, err := NormalizeTheaterMCPScope(a.User.ID, s); err != nil {
		return nil, err
	}
	if err := theaterMCPActionScopes(a, leaf.action); err != nil {
		return nil, err
	}
	if leaf.action.Type == "scene.apply" && !CanSwitchTheaterScene(a.User.ID, s.WorldID, s.ChannelID) {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "缺少场景切换权限", 403, nil)
	}
	room, err := model.TheaterRoomFindByScope(s.WorldID, s.ChannelID)
	if err != nil {
		return nil, err
	}
	if room == nil || room.Revision != revision {
		return nil, newTheaterError(TheaterErrorRevisionConflict, "执行 revision 冲突；不得自动重试", 409, map[string]any{"currentRevision": func() int64 {
			if room != nil {
				return room.Revision
			}
			return 0
		}()})
	}
	// A plan becomes invalid on edits. Re-resolve saved actions in the native
	// service, which prevents a browser from supplying a new business payload.
	command := leaf.command
	command.ActionRequestID = executionID + ":" + stepID
	command.ExpectedRevision = revision
	meta := TheaterRequestMeta{Source: "mcp", RequestID: command.ActionRequestID, SessionID: a.CredentialID}
	if leaf.action.Type == "effect.play" {
		var p theaterEffectPlayPayload
		_ = json.Unmarshal(leaf.action.Payload, &p)
		target, err := loadTheaterObject(model.GetDB(), room.ID, p.EffectID)
		if err != nil {
			return nil, err
		}
		// Audio is a separate capability, including audio played by visual effects.
		var raw map[string]any
		_ = json.Unmarshal([]byte(target.ContentJSON), &raw)
		if fx, ok := raw["effect"].(map[string]any); ok && fx["audio"] != nil {
			if !a.Allows("audio:write") {
				return nil, ErrMCPScopeDenied
			}
		}
	}
	if leaf.direct {
		switch leaf.action.Type {
		case "scene.apply":
			r, err := ApplyTheaterMutation(ctx, a.User.ID, TheaterMutationCommand{MutationID: command.ActionRequestID, WorldID: s.WorldID, ChannelID: s.ChannelID, ExpectedRevision: revision, Type: TheaterMutationSceneApply, Payload: leaf.action.Payload}, meta)
			return &TheaterActionResult{Kind: "mutation", Mutation: r}, err
		case "effect.play":
			var p theaterEffectPlayPayload
			_ = json.Unmarshal(leaf.action.Payload, &p)
			fx, err := theaterMCPEffect(a.User.ID, s, p.EffectID, command.ActionRequestID)
			return &TheaterActionResult{Kind: "effect", Effect: fx}, err
		default:
			return nil, theaterPayloadError("未支持的直接动作")
		}
	}
	if leaf.action.Type == "chat.insert" {
		if s.InputChannelID == "" || !CanReadChannelByUserId(a.User.ID, s.InputChannelID) {
			return nil, ErrWorldPermission
		}
	}
	return TriggerTheaterAction(ctx, a.User.ID, command, meta)
}
func theaterMCPEffect(actorID string, s TheaterScope, effectID, triggerID string) (*TheaterEffectActionResult, error) {
	if _, _, err := requireTheaterPermission(actorID, s.WorldID, s.ChannelID, TheaterPermissionActionTrigger); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomFindByScope(s.WorldID, s.ChannelID)
	if err != nil {
		return nil, err
	}
	target, err := loadTheaterObject(model.GetDB(), room.ID, strings.TrimSpace(effectID))
	if err != nil {
		return nil, err
	}
	if target.Kind != "effect" || !target.Visible || (target.SceneID != "" && target.SceneID != room.ActiveSceneID) {
		return nil, newTheaterError(TheaterErrorNotFound, "可播放特效不存在", 404, nil)
	}
	return &TheaterEffectActionResult{TriggerID: triggerID, EffectID: target.ID, SceneID: target.SceneID, RoomID: room.ID, Revision: room.Revision}, nil
}
