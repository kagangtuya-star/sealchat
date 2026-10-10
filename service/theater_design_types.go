package service

import (
	"encoding/json"
	"strings"

	"sealchat/utils"
)

const theaterMaxDesignSteps = 64

// The wire union is decoded strictly by kind before any planning. RawMessage
// retains a short top-level tool schema; it is never a native payload passthrough.
type TheaterDesignPlan struct {
	InputChannelID string            `json:"inputChannelId,omitempty"`
	Steps          []json.RawMessage `json:"steps"`
	steps          []any
	admin          bool
	actorID        string
	config         utils.AppConfig
}

type TheaterDesignKind struct {
	Kind string `json:"kind"`
}

type TheaterDesignSceneCreate struct {
	TheaterDesignKind
	SceneID string                `json:"sceneId"`
	Fields  TheaterMCPSceneFields `json:"fields"`
}
type TheaterDesignSceneUpdate struct {
	TheaterDesignKind
	SceneID string                `json:"sceneId"`
	Fields  TheaterMCPSceneFields `json:"fields"`
}
type TheaterDesignSceneDelete struct {
	TheaterDesignKind
	SceneID         string `json:"sceneId"`
	FallbackSceneID string `json:"fallbackSceneId,omitempty"`
}
type TheaterDesignSceneReorder struct {
	TheaterDesignKind
	SceneIDs []string `json:"sceneIds"`
}
type TheaterDesignSceneFolders struct {
	TheaterDesignKind
	Folders []TheaterSceneFolder `json:"folders"`
}

type TheaterMCPFieldFields struct {
	BackgroundColor *string  `json:"backgroundColor,omitempty"`
	FieldWidth      *float64 `json:"fieldWidth,omitempty"`
	FieldHeight     *float64 `json:"fieldHeight,omitempty"`
	FieldObjectFit  *string  `json:"fieldObjectFit,omitempty"`
	DisplayGrid     *bool    `json:"displayGrid,omitempty"`
	GridOnTop       *bool    `json:"gridOnTop,omitempty"`
	GridSize        *float64 `json:"gridSize,omitempty"`
	AlignWithGrid   *bool    `json:"alignWithGrid,omitempty"`
}
type TheaterDesignSceneField struct {
	TheaterDesignKind
	SceneID string                `json:"sceneId"`
	Fields  TheaterMCPFieldFields `json:"fields"`
}
type TheaterDesignSceneSurface struct {
	TheaterDesignKind
	SceneID string                  `json:"sceneId"`
	Target  string                  `json:"target"`
	Image   *TheaterMCPImage        `json:"image,omitempty"`
	Clear   bool                    `json:"clear,omitempty"`
	Style   *TheaterMCPSurfaceStyle `json:"style,omitempty"`
}
type TheaterDesignSceneSurfaceEmbed struct {
	TheaterDesignKind
	SceneID     string   `json:"sceneId"`
	Target      string   `json:"target"`
	URL         string   `json:"url,omitempty"`
	Scale       *float64 `json:"scale,omitempty"`
	Interactive *bool    `json:"interactive,omitempty"`
	Clear       bool     `json:"clear,omitempty"`
}
type TheaterDesignSceneOverlay struct {
	TheaterDesignKind
	SceneID  string              `json:"sceneId"`
	Overlays []TheaterMCPOverlay `json:"overlays"`
}
type TheaterDesignSceneMusic struct {
	TheaterDesignKind
	SceneID     string           `json:"sceneId"`
	Music       *TheaterMCPMusic `json:"music,omitempty"`
	SwitchAudio *TheaterMCPAudio `json:"switchAudio,omitempty"`
	Clear       bool             `json:"clear,omitempty"`
}
type TheaterDesignSceneTransition struct {
	TheaterDesignKind
	SceneID    string               `json:"sceneId"`
	Transition TheaterMCPTransition `json:"transition"`
}
type TheaterDesignSceneSequence struct {
	TheaterDesignKind
	SceneID   string               `json:"sceneId"`
	Sequences []TheaterMCPSequence `json:"sequences"`
}
type TheaterDesignObjectCreate struct {
	TheaterDesignKind
	ObjectID   string                 `json:"objectId"`
	SceneID    *string                `json:"sceneId,omitempty"`
	ObjectType string                 `json:"objectType"`
	Fields     TheaterMCPObjectFields `json:"fields"`
}
type TheaterDesignObjectUpdate struct {
	TheaterDesignKind
	ObjectID string                 `json:"objectId"`
	Fields   TheaterMCPObjectFields `json:"fields"`
}
type TheaterDesignObjectDelete struct {
	TheaterDesignKind
	ObjectID string `json:"objectId"`
	Cascade  bool   `json:"cascade,omitempty"`
}
type TheaterDesignCharacterBind struct {
	TheaterDesignObjectCreate
	IdentityID      string `json:"identityId"`
	OwnerUserID     string `json:"ownerUserId"`
	ownerAuthorized bool
}
type TheaterDesignLayoutAlign struct {
	TheaterDesignKind
	ObjectIDs []string `json:"objectIds"`
	Mode      string   `json:"mode"`
}
type TheaterDesignLayoutDistribute struct {
	TheaterDesignKind
	ObjectIDs []string `json:"objectIds"`
	Mode      string   `json:"mode"`
}
type TheaterDesignLayoutGrid struct {
	TheaterDesignKind
	ObjectIDs []string `json:"objectIds"`
	Columns   int      `json:"columns"`
	GapX      float64  `json:"gapX"`
	GapY      float64  `json:"gapY"`
	OriginX   *float64 `json:"originX,omitempty"`
	OriginY   *float64 `json:"originY,omitempty"`
}

// Also used to build the MCP oneOf schema; the schema and runtime union share
// this closed list. Duplicate/control/embed-code steps are deliberately absent.
func TheaterDesignStepVariants() map[string]any {
	return map[string]any{
		"scene.create": &TheaterDesignSceneCreate{}, "scene.update": &TheaterDesignSceneUpdate{},
		"scene.delete": &TheaterDesignSceneDelete{}, "scene.reorder": &TheaterDesignSceneReorder{},
		"scene.folders": &TheaterDesignSceneFolders{}, "scene.field": &TheaterDesignSceneField{},
		"scene.surface": &TheaterDesignSceneSurface{}, "scene.surface_embed": &TheaterDesignSceneSurfaceEmbed{},
		"scene.overlay": &TheaterDesignSceneOverlay{}, "scene.music": &TheaterDesignSceneMusic{},
		"scene.transition": &TheaterDesignSceneTransition{}, "scene.sequence": &TheaterDesignSceneSequence{},
		"object.create": &TheaterDesignObjectCreate{}, "object.update": &TheaterDesignObjectUpdate{},
		"object.delete": &TheaterDesignObjectDelete{}, "character.bind": &TheaterDesignCharacterBind{},
		"layout.align": &TheaterDesignLayoutAlign{}, "layout.distribute": &TheaterDesignLayoutDistribute{},
		"layout.grid": &TheaterDesignLayoutGrid{},
	}
}

type TheaterDesignSummary struct {
	Valid          bool                         `json:"valid"`
	Revision       int64                        `json:"revision"`
	StepCount      int                          `json:"stepCount"`
	ScenesCreated  []string                     `json:"scenesCreated"`
	ScenesUpdated  []string                     `json:"scenesUpdated"`
	ScenesDeleted  []string                     `json:"scenesDeleted"`
	ObjectsCreated []string                     `json:"objectsCreated"`
	ObjectsUpdated []string                     `json:"objectsUpdated"`
	ObjectsDeleted []string                     `json:"objectsDeleted"`
	Layout         []TheaterDesignLayoutSummary `json:"layout"`
	Warnings       []string                     `json:"warnings"`
}
type TheaterDesignLayoutSummary struct {
	StepIndex int      `json:"stepIndex"`
	Kind      string   `json:"kind"`
	ObjectIDs []string `json:"objectIds"`
}

func theaterMCPMap(value any) map[string]any {
	raw, _ := json.Marshal(value)
	result := map[string]any{}
	_ = json.Unmarshal(raw, &result)
	return result
}

// Map the StageLiveState names onto the existing persisted state contract.
func TheaterMCPFieldPatch(fields TheaterMCPFieldFields) map[string]any {
	values := theaterMCPMap(fields)
	patch, grid := map[string]any{}, map[string]any{}
	keys := map[string]string{"backgroundColor": "backgroundColor", "fieldObjectFit": "objectFit", "displayGrid": "display", "gridOnTop": "onTop", "gridSize": "size", "alignWithGrid": "align"}
	for key, value := range values {
		if target, ok := keys[key]; ok {
			grid[target] = value
		} else {
			patch[key] = value
		}
	}
	if len(grid) > 0 {
		patch["grid"] = grid
	}
	return patch
}

func normalizeTheaterDesignPlan(plan *TheaterDesignPlan) error {
	plan.InputChannelID = strings.TrimSpace(plan.InputChannelID)
	if len(plan.Steps) < 1 || len(plan.Steps) > theaterMaxDesignSteps {
		return theaterPayloadError("Design Plan steps 必须为 1..64 项")
	}
	plan.steps = make([]any, len(plan.Steps))
	for index, raw := range plan.Steps {
		var header TheaterDesignKind
		if json.Unmarshal(raw, &header) != nil {
			return theaterDesignStepError(index, "", theaterPayloadError("step JSON 无效"))
		}
		target := TheaterDesignStepVariants()[header.Kind]
		if target == nil {
			return theaterDesignStepError(index, header.Kind, theaterPayloadError("不支持的 design step"))
		}
		if err := decodeStrictJSON(raw, target); err != nil {
			return theaterDesignStepError(index, header.Kind, theaterPayloadError(err.Error()))
		}
		plan.steps[index] = target
		normalized, err := json.Marshal(target)
		if err != nil {
			return theaterDesignStepError(index, header.Kind, err)
		}
		plan.Steps[index] = normalized
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	if len(raw) > theaterMaxPayloadBytes {
		return theaterPayloadError("Design Plan payload 超过 128 KiB")
	}
	return nil
}

func theaterDesignStepError(index int, kind string, err error) error {
	if native, ok := err.(*TheaterError); ok {
		details := map[string]any{"stepIndex": index, "stepKind": kind, "nativeCode": native.Code, "nativeMessage": native.Message}
		for k, v := range native.Details {
			details[k] = v
		}
		return newTheaterError(native.Code, native.Message, native.HTTPStatus, details)
	}
	return err
}
