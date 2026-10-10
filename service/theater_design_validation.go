package service

import (
	"encoding/json"
	"strings"
)

func validateTheaterFieldState(state map[string]any) error {
	for _, key := range []string{"fieldWidth", "fieldHeight"} {
		if value, ok := state[key]; ok {
			n, valid := theaterNumericValue(value)
			if !valid || !theaterFinite(n) || n < 1 || n > 1_000_000 {
				return theaterPayloadError(key + " 范围 1..1000000")
			}
		}
	}
	if value, exists := state["grid"]; exists && value != nil {
		grid, ok := value.(map[string]any)
		if !ok {
			return theaterPayloadError("grid 无效")
		}
		for _, key := range []string{"display", "onTop", "align"} {
			if v, ok := grid[key]; ok {
				if _, valid := v.(bool); !valid {
					return theaterPayloadError("grid." + key + " 无效")
				}
			}
		}
		if v, ok := grid["size"]; ok {
			n, valid := theaterNumericValue(v)
			if !valid || !theaterFinite(n) || n < 0.01 || n > 1_000_000 {
				return theaterPayloadError("grid.size 范围 0.01..1000000")
			}
		}
		if v, ok := grid["objectFit"]; ok {
			fit, valid := v.(string)
			if !valid || !theaterContains([]string{"cover", "contain", "fill"}, fit) {
				return theaterPayloadError("grid.objectFit 无效")
			}
		}
		if v, ok := grid["backgroundColor"]; ok {
			color, valid := v.(string)
			if !valid || strings.TrimSpace(color) == "" || len(color) > 128 {
				return theaterPayloadError("grid.backgroundColor 无效")
			}
		}
	}
	return nil
}

func validateTheaterDesignReferences(snapshot TheaterSharedSnapshot, objects map[string]TheaterObjectSnapshot) error {
	check := func(_ map[string]any, field, id string) error {
		if id == "" {
			return nil
		}
		switch field {
		case "sceneId":
			if _, ok := snapshot.Scenes[id]; !ok {
				return newTheaterError(TheaterErrorPayloadInvalid, "scene reference 不存在: "+id, 400, map[string]any{"referenceId": id})
			}
		case "objectId":
			if _, ok := objects[id]; !ok {
				return newTheaterError(TheaterErrorPayloadInvalid, "object reference 不存在: "+id, 400, map[string]any{"referenceId": id})
			}
		case "effectId":
			if o, ok := objects[id]; !ok || o.Kind != "effect" {
				return newTheaterError(TheaterErrorPayloadInvalid, "effect reference 不存在或类型无效: "+id, 400, map[string]any{"referenceId": id})
			}
		}
		return nil
	}
	for _, id := range theaterCloneSortedIDs(objects) {
		o := objects[id]
		var actions []any
		_ = json.Unmarshal(o.Actions, &actions)
		if err := walkTheaterCloneActions(actions, func(_ map[string]any) {}, check); err != nil {
			native := err.(*TheaterError)
			native.Details["objectId"] = o.ID
			return native
		}
	}
	for _, scene := range snapshot.Scenes {
		state := map[string]any{}
		_ = json.Unmarshal(scene.State, &state)
		sequences, _ := state["theaterSequences"].([]any)
		for _, raw := range sequences {
			sequence, _ := raw.(map[string]any)
			if err := walkTheaterCloneSequence(sequence, func(_ map[string]any) {}, check); err != nil {
				native := err.(*TheaterError)
				native.Details["sceneId"] = scene.ID
				return native
			}
		}
	}
	return nil
}

// Only schema-owned actions/steps/triggers are traversed. User text, overlay
// registry effectId, media params, identity and resource data are not references.
func walkTheaterCloneActions(actions []any, onID func(map[string]any), reference func(map[string]any, string, string) error) error {
	for _, raw := range actions {
		action, _ := raw.(map[string]any)
		if err := walkTheaterCloneAction(action, onID, reference); err != nil {
			return err
		}
	}
	return nil
}
func walkTheaterCloneAction(action map[string]any, onID func(map[string]any), reference func(map[string]any, string, string) error) error {
	if action == nil {
		return nil
	}
	onID(action)
	payload, _ := action["payload"].(map[string]any)
	if action["type"] == "action.sequence" {
		return walkTheaterCloneSequence(payload, onID, reference)
	}
	field := ""
	switch action["type"] {
	case "scene.apply":
		field = "sceneId"
	case "object.toggle", "object.trigger":
		field = "objectId"
	case "effect.play":
		field = "effectId"
	}
	if field != "" {
		if id, ok := payload[field].(string); ok {
			return reference(payload, field, id)
		}
	}
	return nil
}
func walkTheaterCloneSequence(sequence map[string]any, onID func(map[string]any), reference func(map[string]any, string, string) error) error {
	if sequence == nil {
		return nil
	}
	onID(sequence)
	triggers, _ := sequence["triggers"].([]any)
	for _, raw := range triggers {
		trigger, _ := raw.(map[string]any)
		onID(trigger)
		if id, ok := trigger["objectId"].(string); ok {
			if err := reference(trigger, "objectId", id); err != nil {
				return err
			}
		}
	}
	steps, _ := sequence["steps"].([]any)
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		onID(step)
		if id, ok := step["sceneId"].(string); ok {
			if err := reference(step, "sceneId", id); err != nil {
				return err
			}
		}
		action, _ := step["action"].(map[string]any)
		if err := walkTheaterCloneAction(action, onID, reference); err != nil {
			return err
		}
	}
	return nil
}
