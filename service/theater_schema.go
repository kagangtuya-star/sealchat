package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

const (
	theaterMaxSnapshotBytes = 4 << 20
	theaterMaxPayloadBytes  = 128 << 10
	theaterMaxScenes        = 200
	theaterMaxObjects       = 5000
	theaterMaxSceneObjects  = 2000
	theaterMaxBatchUpdates  = 200
	theaterMaxActions       = 32
)

type theaterSceneCreatePayload struct {
	SceneID string         `json:"sceneId"`
	Name    string         `json:"name"`
	Order   int64          `json:"order"`
	State   map[string]any `json:"state"`
}

type theaterSceneUpdatePayload struct {
	SceneID string         `json:"sceneId"`
	Fields  map[string]any `json:"fields"`
}

type theaterSceneDeletePayload struct {
	SceneID         string `json:"sceneId"`
	FallbackSceneID string `json:"fallbackSceneId"`
}

type theaterSceneApplyPayload struct {
	SceneID    string                    `json:"sceneId"`
	Transition *theaterTransitionPayload `json:"transition,omitempty"`
}

type theaterTransitionPayload struct {
	Type       string `json:"type"`
	DurationMS int64  `json:"durationMs"`
}

type theaterObjectInput struct {
	ID                  string          `json:"id"`
	ParentID            *string         `json:"parentId"`
	Kind                string          `json:"kind"`
	Name                string          `json:"name"`
	X                   float64         `json:"x"`
	Y                   float64         `json:"y"`
	Width               float64         `json:"width"`
	Height              float64         `json:"height"`
	Rotation            float64         `json:"rotation"`
	Scale               *float64        `json:"scale,omitempty"`
	ScaleX              *float64        `json:"scaleX,omitempty"`
	ScaleY              *float64        `json:"scaleY,omitempty"`
	Z                   float64         `json:"z"`
	OrderKey            string          `json:"orderKey"`
	Visible             *bool           `json:"visible"`
	Locked              bool            `json:"locked"`
	AspectRatioLocked   *bool           `json:"aspectRatioLocked"`
	Interactive         bool            `json:"interactive"`
	Editable            bool            `json:"editable"`
	OwnerUserID         *string         `json:"ownerUserId"`
	CharacterIdentityID *string         `json:"characterIdentityId"`
	Content             json.RawMessage `json:"content"`
	Actions             json.RawMessage `json:"actions"`
	Metadata            json.RawMessage `json:"metadata"`
}

type theaterObjectCreatePayload struct {
	SceneID *string            `json:"sceneId"`
	Object  theaterObjectInput `json:"object"`
}

type theaterObjectUpdatePayload struct {
	ObjectID string         `json:"objectId"`
	Fields   map[string]any `json:"fields"`
}

type theaterObjectBatchUpdatePayload struct {
	Updates []theaterObjectUpdatePayload `json:"updates"`
}

type theaterObjectDeletePayload struct {
	ObjectID string `json:"objectId"`
	Cascade  bool   `json:"cascade"`
}

type theaterObjectTogglePayload struct {
	ObjectID string `json:"objectId"`
	Visible  *bool  `json:"visible,omitempty"`
}

type theaterCharacterBindPayload struct {
	SceneID     *string            `json:"sceneId"`
	Object      theaterObjectInput `json:"object"`
	IdentityID  string             `json:"identityId"`
	OwnerUserID string             `json:"ownerUserId"`
}

type theaterResourceReferencePayload struct {
	ResourceID string          `json:"resourceId"`
	TargetType string          `json:"targetType"`
	TargetID   string          `json:"targetId"`
	Slot       string          `json:"slot"`
	Config     json.RawMessage `json:"config,omitempty"`
}

func decodeTheaterPayload(mutationType string, raw json.RawMessage) (any, json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > theaterMaxPayloadBytes {
		return nil, nil, newTheaterError(TheaterErrorPayloadInvalid, "mutation payload 大小无效", 400, nil)
	}
	var target any
	switch mutationType {
	case TheaterMutationSceneCreate:
		target = &theaterSceneCreatePayload{}
	case TheaterMutationSceneUpdate:
		target = &theaterSceneUpdatePayload{}
	case TheaterMutationSceneDelete:
		target = &theaterSceneDeletePayload{}
	case TheaterMutationSceneApply:
		target = &theaterSceneApplyPayload{}
	case TheaterMutationObjectCreate:
		target = &theaterObjectCreatePayload{}
	case TheaterMutationObjectUpdate, TheaterMutationCharacterUpdate:
		target = &theaterObjectUpdatePayload{}
	case TheaterMutationObjectBatchUpdate:
		target = &theaterObjectBatchUpdatePayload{}
	case TheaterMutationObjectDelete:
		target = &theaterObjectDeletePayload{}
	case TheaterMutationObjectToggle:
		target = &theaterObjectTogglePayload{}
	case TheaterMutationCharacterBind:
		target = &theaterCharacterBindPayload{}
	case TheaterMutationResourceAttach, TheaterMutationResourceDetach:
		target = &theaterResourceReferencePayload{}
	default:
		return nil, nil, newTheaterError(TheaterErrorMutationTypeUnsupported, "不支持 mutation type", 400, map[string]any{"type": mutationType})
	}
	if err := decodeStrictJSON(raw, target); err != nil {
		return nil, nil, newTheaterError(TheaterErrorPayloadInvalid, err.Error(), 400, nil)
	}
	if err := validateDecodedTheaterPayload(mutationType, target); err != nil {
		return nil, nil, err
	}
	normalized, err := json.Marshal(target)
	if err != nil {
		return nil, nil, newTheaterError(TheaterErrorInternal, "规范化 mutation 失败", 500, nil)
	}
	return target, normalized, nil
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("JSON schema 无效: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("JSON 只能包含一个值")
	}
	return nil
}

func validateDecodedTheaterPayload(mutationType string, decoded any) error {
	switch payload := decoded.(type) {
	case *theaterSceneCreatePayload:
		if err := validateTheaterID(payload.SceneID, "sceneId"); err != nil {
			return err
		}
		if err := validateTheaterName(payload.Name); err != nil {
			return err
		}
		return validateSceneState(payload.State)
	case *theaterSceneUpdatePayload:
		if err := validateTheaterID(payload.SceneID, "sceneId"); err != nil {
			return err
		}
		return validateSceneFields(payload.Fields)
	case *theaterSceneDeletePayload:
		return validateTheaterID(payload.SceneID, "sceneId")
	case *theaterSceneApplyPayload:
		if err := validateTheaterID(payload.SceneID, "sceneId"); err != nil {
			return err
		}
		if payload.Transition != nil {
			if payload.Transition.Type != "none" && payload.Transition.Type != "crossfade" {
				return theaterPayloadError("transition.type 无效")
			}
			if payload.Transition.DurationMS < 0 || payload.Transition.DurationMS > 60000 {
				return theaterPayloadError("transition.durationMs 超限")
			}
		}
	case *theaterObjectCreatePayload:
		return validateObjectInput(&payload.Object)
	case *theaterObjectUpdatePayload:
		if err := validateTheaterID(payload.ObjectID, "objectId"); err != nil {
			return err
		}
		return validateObjectFields(payload.Fields, mutationType == TheaterMutationCharacterUpdate)
	case *theaterObjectBatchUpdatePayload:
		if len(payload.Updates) == 0 || len(payload.Updates) > theaterMaxBatchUpdates {
			return theaterPayloadError("updates 数量无效")
		}
		seen := make(map[string]bool, len(payload.Updates))
		for i := range payload.Updates {
			update := &payload.Updates[i]
			if err := validateTheaterID(update.ObjectID, "updates.objectId"); err != nil {
				return err
			}
			if seen[update.ObjectID] {
				return theaterPayloadError("updates 包含重复 objectId")
			}
			seen[update.ObjectID] = true
			if err := validateObjectFields(update.Fields, false); err != nil {
				return err
			}
		}
		return nil
	case *theaterObjectDeletePayload:
		return validateTheaterID(payload.ObjectID, "objectId")
	case *theaterObjectTogglePayload:
		return validateTheaterID(payload.ObjectID, "objectId")
	case *theaterCharacterBindPayload:
		if strings.TrimSpace(payload.IdentityID) == "" || strings.TrimSpace(payload.OwnerUserID) == "" {
			return theaterPayloadError("identityId 和 ownerUserId 必填")
		}
		payload.Object.Kind = "character"
		payload.Object.CharacterIdentityID = &payload.IdentityID
		payload.Object.OwnerUserID = &payload.OwnerUserID
		return validateObjectInput(&payload.Object)
	case *theaterResourceReferencePayload:
		if err := validateTheaterID(payload.ResourceID, "resourceId"); err != nil {
			return err
		}
		if payload.TargetType != "room" && payload.TargetType != "scene" && payload.TargetType != "object" {
			return theaterPayloadError("targetType 无效")
		}
		allowedSlots := map[string]bool{"background": true, "foreground": true, "image": true, "animatedImage": true, "video": true, "poster": true, "decoration": true}
		if !allowedSlots[payload.Slot] {
			return theaterPayloadError("slot 无效")
		}
		if payload.TargetType != "room" && strings.TrimSpace(payload.TargetID) == "" {
			return theaterPayloadError("targetId 必填")
		}
	}
	return nil
}

func validateTheaterID(value, field string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return theaterPayloadError(field + " 无效")
	}
	return nil
}

func validateTheaterName(value string) error {
	length := len([]rune(strings.TrimSpace(value)))
	if length < 1 || length > 512 {
		return theaterPayloadError("name 长度无效")
	}
	return nil
}

func validateSceneState(state map[string]any) error {
	allowed := map[string]bool{"background": true, "foreground": true, "surfaceStyles": true, "fieldWidth": true, "fieldHeight": true, "grid": true, "transition": true, "resources": true}
	for key, value := range state {
		if !allowed[key] {
			return theaterPayloadError("scene state 包含禁止字段: " + key)
		}
		if err := rejectUnsafeTheaterJSON(value); err != nil {
			return err
		}
	}
	if styles, ok := state["surfaceStyles"]; ok {
		if err := validateTheaterSurfaceStyles(styles); err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(state)
	if len(raw) > 64<<10 {
		return theaterPayloadError("scene state 超过 64 KiB")
	}
	return nil
}

func validateTheaterSurfaceStyles(value any) error {
	styles, ok := value.(map[string]any)
	if !ok {
		return theaterPayloadError("surfaceStyles 无效")
	}
	if len(styles) != 2 {
		return theaterPayloadError("surfaceStyles 图层无效")
	}
	for _, target := range []string{"background", "foreground"} {
		style, ok := styles[target].(map[string]any)
		if !ok {
			return theaterPayloadError("surfaceStyles." + target + " 无效")
		}
		allowed := map[string]bool{"brightness": true, "blurPx": true, "opacity": true, "fit": true, "overlay": true}
		for key := range style {
			if !allowed[key] {
				return theaterPayloadError("surfaceStyles." + target + " 包含禁止字段: " + key)
			}
		}
		for name, bounds := range map[string][2]float64{"brightness": {0, 2}, "blurPx": {0, 40}, "opacity": {0, 1}} {
			number, valid := theaterNumericValue(style[name])
			if !valid || math.IsNaN(number) || math.IsInf(number, 0) || number < bounds[0] || number > bounds[1] {
				return theaterPayloadError("surfaceStyles." + target + "." + name + " 无效")
			}
		}
		fit, ok := style["fit"].(string)
		if !ok || !map[string]bool{"fill": true, "cover": true, "contain": true, "tile": true, "center": true}[fit] {
			return theaterPayloadError("surfaceStyles." + target + ".fit 无效")
		}
		overlay, ok := style["overlay"].(map[string]any)
		if !ok || len(overlay) != 3 {
			return theaterPayloadError("surfaceStyles." + target + ".overlay 无效")
		}
		if _, ok := overlay["enabled"].(bool); !ok {
			return theaterPayloadError("surfaceStyles." + target + ".overlay.enabled 无效")
		}
		color, ok := overlay["color"].(string)
		if !ok || strings.TrimSpace(color) == "" || len(color) > 64 {
			return theaterPayloadError("surfaceStyles." + target + ".overlay.color 无效")
		}
		opacity, valid := theaterNumericValue(overlay["opacity"])
		if !valid || math.IsNaN(opacity) || math.IsInf(opacity, 0) || opacity < 0 || opacity > 1 {
			return theaterPayloadError("surfaceStyles." + target + ".overlay.opacity 无效")
		}
	}
	return nil
}

func validateSceneFields(fields map[string]any) error {
	if len(fields) == 0 {
		return theaterPayloadError("fields 不能为空")
	}
	allowed := map[string]bool{"name": true, "order": true, "locked": true, "state": true}
	for key := range fields {
		if !allowed[key] {
			return theaterPayloadError("scene fields 包含禁止字段: " + key)
		}
	}
	if name, ok := fields["name"].(string); ok {
		if err := validateTheaterName(name); err != nil {
			return err
		}
	}
	if state, ok := fields["state"].(map[string]any); ok {
		return validateSceneState(state)
	}
	return rejectUnsafeTheaterJSON(fields)
}

func validateObjectInput(object *theaterObjectInput) error {
	if err := validateTheaterID(object.ID, "object.id"); err != nil {
		return err
	}
	allowedKinds := map[string]bool{"group": true, "drawing": true, "text": true, "image": true, "button": true, "character": true, "video": true}
	if !allowedKinds[object.Kind] {
		return theaterPayloadError("object.kind 无效")
	}
	for _, value := range []float64{object.X, object.Y, object.Width, object.Height, object.Rotation, object.Z} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return theaterPayloadError("object transform 必须为有限数")
		}
	}
	for name, scale := range map[string]*float64{"scale": object.Scale, "scaleX": object.ScaleX, "scaleY": object.ScaleY} {
		if scale != nil && (math.IsNaN(*scale) || math.IsInf(*scale, 0) || *scale < 0.01 || *scale > 100) {
			return theaterPayloadError("object " + name + " 无效")
		}
	}
	if object.Width < 0 || object.Height < 0 || object.Width > 1000000 || object.Height > 1000000 {
		return theaterPayloadError("object 尺寸无效")
	}
	if len(object.Name) > 512 || len(object.OrderKey) > 128 {
		return theaterPayloadError("object 字符串超限")
	}
	if len(object.Content)+len(object.Actions)+len(object.Metadata) > 64<<10 {
		return theaterPayloadError("object JSON 超过 64 KiB")
	}
	for _, raw := range []json.RawMessage{object.Content, object.Metadata} {
		if len(raw) > 0 {
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return theaterPayloadError("object JSON 无效")
			}
			if err := rejectUnsafeTheaterJSON(value); err != nil {
				return err
			}
		}
	}
	if len(object.Actions) > 0 {
		if err := validateTheaterActions(object.Actions); err != nil {
			return err
		}
	}
	return nil
}

func validateObjectFields(fields map[string]any, characterOnly bool) error {
	if len(fields) == 0 {
		return theaterPayloadError("fields 不能为空")
	}
	allowed := map[string]bool{"parentId": true, "name": true, "x": true, "y": true, "width": true, "height": true, "rotation": true, "scale": true, "scaleX": true, "scaleY": true, "z": true, "orderKey": true, "visible": true, "locked": true, "aspectRatioLocked": true, "interactive": true, "editable": true, "content": true, "actions": true, "metadata": true}
	if characterOnly {
		allowed = map[string]bool{"x": true, "y": true, "width": true, "height": true, "rotation": true, "z": true, "orderKey": true, "visible": true, "locked": true, "content": true, "metadata": true}
	}
	for key := range fields {
		if !allowed[key] {
			return theaterPayloadError("object fields 包含禁止字段: " + key)
		}
	}
	for _, name := range []string{"scale", "scaleX", "scaleY"} {
		if value, ok := fields[name]; ok {
			scale, valid := theaterNumericValue(value)
			if !valid || math.IsNaN(scale) || math.IsInf(scale, 0) || scale < 0.01 || scale > 100 {
				return theaterPayloadError("object " + name + " 无效")
			}
		}
	}
	if actions, ok := fields["actions"]; ok {
		raw, err := json.Marshal(actions)
		if err != nil {
			return theaterPayloadError("object actions 无效")
		}
		if err := validateTheaterActions(raw); err != nil {
			return err
		}
	}
	return rejectUnsafeTheaterJSON(fields)
}

func theaterNumericValue(value any) (float64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}

func validateTheaterActions(raw json.RawMessage) error {
	var actions []theaterStoredAction
	if err := decodeStrictJSON(raw, &actions); err != nil || len(actions) > theaterMaxActions {
		return theaterPayloadError("object actions 无效")
	}
	seen := map[string]struct{}{}
	for _, action := range actions {
		if err := validateTheaterID(action.ID, "action.id"); err != nil {
			return err
		}
		if _, ok := seen[action.ID]; ok {
			return theaterPayloadError("action.id 重复")
		}
		seen[action.ID] = struct{}{}
		switch action.Type {
		case TheaterMutationSceneApply:
			var payload theaterSceneApplyPayload
			if err := decodeStrictJSON(action.Payload, &payload); err != nil || strings.TrimSpace(payload.SceneID) == "" {
				return theaterPayloadError("scene.apply action payload 无效")
			}
		case TheaterMutationObjectToggle:
			var payload theaterObjectTogglePayload
			if len(action.Payload) > 0 {
				if err := decodeStrictJSON(action.Payload, &payload); err != nil {
					return theaterPayloadError("object.toggle action payload 无效")
				}
			}
		case "chat.send":
			var payload theaterChatSendPayload
			if err := decodeStrictJSON(action.Payload, &payload); err != nil {
				return theaterPayloadError("chat.send action payload 无效")
			}
			if _, err := normalizeTheaterChatSendPayload(payload); err != nil {
				return err
			}
		case "chat.insert":
			var payload any
			if err := json.Unmarshal(action.Payload, &payload); err != nil {
				return theaterPayloadError("chat.insert action payload 无效")
			}
			if err := rejectUnsafeTheaterJSON(payload); err != nil {
				return err
			}
		default:
			return theaterPayloadError("action.type 无效")
		}
	}
	return nil
}

func rejectUnsafeTheaterJSON(value any) error {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			lower := strings.ToLower(key)
			if lower == "camera" || lower == "selectedobjectid" || lower == "script" || lower == "code" || lower == "resolvedappearance" || lower == "chatmessages" {
				return theaterPayloadError("包含禁止字段: " + key)
			}
			if err := rejectUnsafeTheaterJSON(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range current {
			if err := rejectUnsafeTheaterJSON(child); err != nil {
				return err
			}
		}
	case float64:
		if math.IsNaN(current) || math.IsInf(current, 0) {
			return theaterPayloadError("JSON 数字必须有限")
		}
	case string:
		lower := strings.ToLower(strings.TrimSpace(current))
		if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "file:") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "blob:") {
			return theaterPayloadError("禁止不可信 URL 协议")
		}
	}
	return nil
}

func theaterPayloadError(message string) *TheaterError {
	return newTheaterError(TheaterErrorPayloadInvalid, message, 400, nil)
}

func theaterJSONHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func canonicalTheaterJSON(value any) ([]byte, string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	return raw, theaterJSONHash(raw), nil
}
