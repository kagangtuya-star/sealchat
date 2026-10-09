package api

import (
	"context"
	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
	"math"
	"net/url"
	"sort"
	"strings"

	"sealchat/service"
)

type mcpTheaterReadInput struct {
	service.TheaterScope
	mcpPageInput
	Operation     string `json:"operation"`
	SceneID       string `json:"sceneId,omitempty"`
	ObjectID      string `json:"objectId,omitempty"`
	AfterRevision int64  `json:"afterRevision,omitempty"`
}
type mcpTheaterWriteInput struct {
	service.TheaterScope
	Operation        string `json:"operation"`
	MutationID       string `json:"mutationId"`
	ExpectedRevision int64  `json:"expectedRevision"`
}
type mcpTheaterSceneFields struct {
	Name       *string `json:"name,omitempty"`
	SwitchText *string `json:"switchText,omitempty"`
	Order      *int64  `json:"order,omitempty"`
	FolderID   *string `json:"folderId,omitempty"`
	Locked     *bool   `json:"locked,omitempty"`
	Published  *bool   `json:"published,omitempty"`
}
type mcpTheaterImage struct {
	ResourceID string `json:"resourceId"`
	URL        string `json:"url"`
	Alt        string `json:"alt,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
	Animated   bool   `json:"animated,omitempty"`
	LoopCount  int    `json:"loopCount,omitempty"`
}
type mcpTheaterIframe struct {
	URL   string  `json:"url"`
	Scale float64 `json:"scale"`
}
type mcpTheaterTransitionPhase struct {
	Type       string `json:"type"`
	DurationMS int64  `json:"durationMs"`
}
type mcpTheaterTransition struct {
	Curtain    *bool                      `json:"curtain,omitempty"`
	Enter      *mcpTheaterTransitionPhase `json:"enter,omitempty"`
	Exit       *mcpTheaterTransitionPhase `json:"exit,omitempty"`
	Type       string                     `json:"type,omitempty"`
	DurationMS *int64                     `json:"durationMs,omitempty"`
}
type mcpTheaterAudio struct {
	AssetID string  `json:"assetId"`
	Name    string  `json:"name"`
	Volume  float64 `json:"volume"`
}
type mcpTheaterMusicTrack struct {
	Type          string                 `json:"type"`
	Asset         *mcpTheaterMusicAsset  `json:"asset"`
	Volume        float64                `json:"volume"`
	FadeIn        int                    `json:"fadeIn"`
	FadeOut       int                    `json:"fadeOut"`
	LoopEnabled   bool                   `json:"loopEnabled"`
	PlaybackRate  float64                `json:"playbackRate"`
	PlaylistMode  *string                `json:"playlistMode"`
	Playlist      []mcpTheaterMusicAsset `json:"playlist"`
	PlaylistIndex int                    `json:"playlistIndex"`
}
type mcpTheaterMusicAsset struct {
	AssetID string `json:"assetId"`
	Name    string `json:"name"`
}
type mcpTheaterMusic struct {
	Version int                    `json:"version"`
	Tracks  []mcpTheaterMusicTrack `json:"tracks"`
}
type mcpTheaterOverlay struct {
	Version   int            `json:"version"`
	ID        string         `json:"id"`
	EffectID  string         `json:"effectId"`
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Opacity   float64        `json:"opacity"`
	BlendMode string         `json:"blendMode"`
	Layer     string         `json:"layer"`
	Media     map[string]any `json:"media,omitempty"`
	Params    map[string]any `json:"params"`
}
type mcpTheaterAction struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Schedule *mcpTheaterSchedule `json:"schedule,omitempty"`
	Payload  map[string]any      `json:"payload"`
}
type mcpTheaterSchedule struct {
	DelayMS int `json:"delayMs"`
}
type mcpTheaterSequence struct {
	Version   int                         `json:"version"`
	ID        string                      `json:"id"`
	Name      string                      `json:"name"`
	Enabled   bool                        `json:"enabled"`
	Triggers  []mcpTheaterSequenceTrigger `json:"triggers"`
	LoopCount int                         `json:"loopCount"`
	Steps     []mcpTheaterSequenceStep    `json:"steps"`
}
type mcpTheaterSequenceStep struct {
	ID      string                   `json:"id"`
	SceneID *string                  `json:"sceneId"`
	Timing  mcpTheaterSequenceTiming `json:"timing"`
	Action  mcpTheaterAction         `json:"action"`
}
type mcpTheaterSequenceTiming struct {
	Mode    string `json:"mode"`
	DelayMS *int   `json:"delayMs,omitempty"`
}
type mcpTheaterSequenceTrigger struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Threshold       int      `json:"threshold"`
	Every           int      `json:"every"`
	CooldownMS      int      `json:"cooldownMs"`
	Keywords        []string `json:"keywords,omitempty"`
	TargetActorName *string  `json:"targetActorName,omitempty"`
	ObjectID        string   `json:"objectId,omitempty"`
}
type mcpTheaterSurfaceStyle struct {
	Brightness float64                  `json:"brightness"`
	BlurPx     float64                  `json:"blurPx"`
	Opacity    float64                  `json:"opacity"`
	Zoom       float64                  `json:"zoom"`
	Fit        string                   `json:"fit"`
	Overlay    mcpTheaterSurfaceOverlay `json:"overlay"`
	MediaFx    map[string]any           `json:"mediaFx,omitempty"`
}
type mcpTheaterSurfaceOverlay struct {
	Enabled bool    `json:"enabled"`
	Color   string  `json:"color"`
	Opacity float64 `json:"opacity"`
}
type mcpTheaterSceneInput struct {
	mcpTheaterWriteInput
	SceneID         string                       `json:"sceneId,omitempty"`
	Fields          *mcpTheaterSceneFields       `json:"fields,omitempty"`
	SceneIDs        []string                     `json:"sceneIds,omitempty"`
	FallbackSceneID string                       `json:"fallbackSceneId,omitempty"`
	Folders         []service.TheaterSceneFolder `json:"folders,omitempty"`
	Target          string                       `json:"target,omitempty"`
	URL             string                       `json:"url,omitempty"`
	Scale           *float64                     `json:"scale,omitempty"`
	Interactive     *bool                        `json:"interactive,omitempty"`
	Image           *mcpTheaterImage             `json:"image,omitempty"`
	Clear           bool                         `json:"clear,omitempty"`
	Style           *mcpTheaterSurfaceStyle      `json:"style,omitempty"`
	FieldWidth      *float64                     `json:"fieldWidth,omitempty"`
	FieldHeight     *float64                     `json:"fieldHeight,omitempty"`
	Overlays        *[]mcpTheaterOverlay         `json:"overlays,omitempty"`
	Music           *mcpTheaterMusic             `json:"music,omitempty"`
	SwitchAudio     *mcpTheaterAudio             `json:"switchAudio,omitempty"`
	Transition      *mcpTheaterTransition        `json:"transition,omitempty"`
	Sequences       *[]mcpTheaterSequence        `json:"sequences,omitempty"`
}

// Explicit transform fields; content/actions are existing domain data, validated
// by the native schema. There is no arbitrary mutation type or top-level payload.
type mcpTheaterObjectFields struct {
	SceneID           *string             `json:"sceneId,omitempty"`
	ParentID          *string             `json:"parentId,omitempty"`
	Name              *string             `json:"name,omitempty"`
	X                 *float64            `json:"x,omitempty"`
	Y                 *float64            `json:"y,omitempty"`
	Width             *float64            `json:"width,omitempty"`
	Height            *float64            `json:"height,omitempty"`
	Rotation          *float64            `json:"rotation,omitempty"`
	Scale             *float64            `json:"scale,omitempty"`
	ScaleX            *float64            `json:"scaleX,omitempty"`
	ScaleY            *float64            `json:"scaleY,omitempty"`
	Z                 *float64            `json:"z,omitempty"`
	OrderKey          *string             `json:"orderKey,omitempty"`
	Visible           *bool               `json:"visible,omitempty"`
	Locked            *bool               `json:"locked,omitempty"`
	AspectRatioLocked *bool               `json:"aspectRatioLocked,omitempty"`
	Interactive       *bool               `json:"interactive,omitempty"`
	Editable          *bool               `json:"editable,omitempty"`
	Content           *mcpTheaterContent  `json:"content,omitempty"`
	Actions           *[]mcpTheaterAction `json:"actions,omitempty"`
	// Stored as metadata.embedEventBindings and merged at expectedRevision, so
	// other metadata keys are preserved. Only iframe objects accept it.
	EmbedEventBindings *[]mcpTheaterEmbedEventBinding `json:"embedEventBindings,omitempty"`
}
type mcpTheaterEmbedEventBinding struct {
	Topic     string   `json:"topic"`
	ActionIDs []string `json:"actionIds"`
}
type mcpTheaterContent struct {
	Fill       *string           `json:"fill,omitempty"`
	Text       *string           `json:"text,omitempty"`
	Image      *mcpTheaterImage  `json:"image,omitempty"`
	Iframe     *mcpTheaterIframe `json:"iframe,omitempty"`
	Drawing    map[string]any    `json:"drawing,omitempty"`
	Effect     map[string]any    `json:"effect,omitempty"`
	Video      map[string]any    `json:"video,omitempty"`
	Annotation map[string]any    `json:"annotation,omitempty"`
	Style      map[string]any    `json:"style,omitempty"`
}
type mcpTheaterObjectUpdate struct {
	ObjectID string                 `json:"objectId"`
	Fields   mcpTheaterObjectFields `json:"fields"`
}
type mcpTheaterObjectInput struct {
	mcpTheaterWriteInput
	ObjectID    string                   `json:"objectId,omitempty"`
	SceneID     *string                  `json:"sceneId,omitempty"`
	Kind        string                   `json:"kind,omitempty"`
	Fields      *mcpTheaterObjectFields  `json:"fields,omitempty"`
	Updates     []mcpTheaterObjectUpdate `json:"updates,omitempty"`
	Cascade     bool                     `json:"cascade,omitempty"`
	Visible     *bool                    `json:"visible,omitempty"`
	IdentityID  string                   `json:"identityId,omitempty"`
	OwnerUserID string                   `json:"ownerUserId,omitempty"`
}

func mcpTheaterMap(v any) map[string]any {
	raw, _ := json.Marshal(v)
	result := map[string]any{}
	_ = json.Unmarshal(raw, &result)
	return result
}
func mcpTheaterSnapshot(ctx context.Context, a *service.MCPActor, s service.TheaterScope, resources bool) (*service.TheaterSnapshotResult, service.TheaterScope, error) {
	scope, err := service.NormalizeTheaterMCPScope(a.User.ID, s)
	if err != nil {
		return nil, scope, err
	}
	snap, err := service.GetTheaterSnapshot(ctx, a.User.ID, scope.WorldID, scope.ChannelID, service.TheaterSnapshotOptions{IncludeResources: resources})
	return snap, scope, err
}
func mcpTheaterFindObject(s *service.TheaterSnapshotResult, id string) (service.TheaterObjectSnapshot, bool) {
	if o, ok := s.Snapshot.PersistentObjects[id]; ok {
		return o, true
	}
	for _, sc := range s.Snapshot.Scenes {
		if o, ok := sc.Objects[id]; ok {
			return o, true
		}
	}
	return service.TheaterObjectSnapshot{}, false
}
func mcpTheaterRead(ctx context.Context, a *service.MCPActor, in mcpTheaterReadInput) (any, error) {
	s, scope, err := mcpTheaterSnapshot(ctx, a, in.TheaterScope, false)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"scope": scope, "revision": s.Revision, "checksum": s.Checksum, "activeSceneId": s.Snapshot.ActiveSceneID, "permissions": s.Permissions, "contentTrust": "untrusted_user_generated"}
	switch in.Operation {
	case "summary", "scene":
		scenes := []service.TheaterSceneSnapshot{}
		objects := []service.TheaterObjectSnapshot{}
		for _, sc := range s.Snapshot.Scenes {
			if in.Operation == "summary" {
				sc.State = nil
				sc.Objects = nil
				scenes = append(scenes, sc)
			} else if sc.ID == in.SceneID {
				out["state"] = sc.State
				for _, o := range sc.Objects {
					objects = append(objects, o)
				}
				out["scene"] = map[string]any{"id": sc.ID, "name": sc.Name, "published": sc.Published, "locked": sc.Locked}
			}
		}
		if in.Operation == "scene" {
			if _, ok := out["scene"]; !ok {
				return nil, mcpFailure("not_found", "场景不存在或不可见")
			}
		}
		if in.Operation == "summary" {
			sort.Slice(scenes, func(i, j int) bool {
				if scenes[i].Order == scenes[j].Order {
					return scenes[i].ID < scenes[j].ID
				}
				return scenes[i].Order < scenes[j].Order
			})
			page, e := mcpSlicePage(scenes, in.mcpPageInput)
			if e != nil {
				return nil, e
			}
			out["scenes"] = page
			objectSceneID := in.SceneID
			if objectSceneID == "" && s.Snapshot.ActiveSceneID != nil {
				objectSceneID = *s.Snapshot.ActiveSceneID
			}
			out["objectsSceneId"] = objectSceneID
			if sc, ok := s.Snapshot.Scenes[objectSceneID]; ok {
				for _, o := range sc.Objects {
					objects = append(objects, o)
				}
			}
			for _, o := range s.Snapshot.PersistentObjects {
				objects = append(objects, o)
			}
			out["coordinateSystem"] = service.TheaterMCPLimits()
			out["renderers"] = service.TheaterRenderers.List(a.User.ID, scope)
		}
		sort.Slice(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID })
		if in.Operation == "summary" {
			for i := range objects {
				objects[i].Actions = nil
				objects[i].Metadata = nil
				objects[i].Content = mcpTheaterResourceSummary(objects[i].Content)
			}
		}
		page, e := mcpSlicePage(objects, in.mcpPageInput)
		if e != nil {
			return nil, e
		}
		out["objects"] = page
	case "object":
		o, ok := mcpTheaterFindObject(s, in.ObjectID)
		if !ok {
			return nil, mcpFailure("not_found", "对象不存在或不可见")
		}
		out["object"] = o
	case "events":
		_, limit, e := in.bounds()
		if e != nil || in.AfterRevision < 0 {
			return nil, mcpFailure("invalid_argument", "分页或 afterRevision 无效")
		}
		events, e := service.ListTheaterEvents(ctx, a.User.ID, scope.WorldID, scope.ChannelID, in.AfterRevision, limit)
		if e != nil {
			return nil, e
		}
		out["events"] = events
	case "renderers":
		out["renderers"] = service.TheaterRenderers.List(a.User.ID, scope)
	default:
		return nil, mcpFailure("invalid_argument", "operation 必须为 summary/scene/object/events/renderers")
	}
	return out, nil
}
func mcpTheaterResourceSummary(raw json.RawMessage) json.RawMessage {
	var c map[string]any
	_ = json.Unmarshal(raw, &c)
	refs := map[string]any{}
	for _, k := range []string{"image", "iframe", "video"} {
		if v, ok := c[k]; ok {
			refs[k] = v
		}
	}
	b, _ := json.Marshal(refs)
	return b
}

func mcpTheaterMutation(ctx context.Context, a *service.MCPActor, in mcpTheaterWriteInput, typ string, payload any, patch map[string]any) (any, error) {
	scope, err := service.NormalizeTheaterMCPScope(a.User.ID, in.TheaterScope)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	r, err := service.ApplyTheaterMCPMutation(ctx, a.User.ID, a.CredentialID, service.TheaterMutationCommand{MutationID: in.MutationID, WorldID: scope.WorldID, ChannelID: scope.ChannelID, ExpectedRevision: in.ExpectedRevision, Type: typ, Payload: raw}, patch)
	if err != nil {
		return nil, err
	}
	// Write results do not expose a hidden source object's content.
	return map[string]any{"scope": scope, "mutationId": r.MutationID, "revisionBefore": r.RevisionBefore, "revision": r.Revision, "checksum": r.Checksum, "idempotent": r.Idempotent}, nil
}

func mcpTheaterScene(ctx context.Context, a *service.MCPActor, in mcpTheaterSceneInput) (any, error) {
	allowed := map[string]string{
		"create": "sceneId fields", "update": "sceneId fields", "reorder": "sceneIds", "delete": "sceneId fallbackSceneId", "apply": "sceneId", "folders_update": "folders",
		"surface_update": "sceneId target image clear style fieldWidth fieldHeight", "surface_embed_set": "sceneId target url scale interactive", "surface_embed_clear": "sceneId target",
		"overlay_update": "sceneId overlays", "music_update": "sceneId music switchAudio clear", "transition_update": "sceneId transition", "sequence_update": "sceneId sequences",
	}
	if err := mcpTheaterOperationFields(in, in.Operation, allowed); err != nil {
		return nil, err
	}
	fields := map[string]any{}
	if in.Fields != nil {
		fields = mcpTheaterMap(in.Fields)
	}
	patch := map[string]any{}
	switch in.Operation {
	case "create":
		if in.Fields == nil || in.Fields.Name == nil {
			return nil, mcpFailure("invalid_argument", "create 需要 fields.name")
		}
		if in.Fields.Locked != nil || in.Fields.Published != nil {
			return nil, mcpFailure("invalid_argument", "create 不接受 locked/published；请用 update")
		}
		order := int64(0)
		if in.Fields.Order != nil {
			order = *in.Fields.Order
		}
		folder := ""
		if in.Fields.FolderID != nil {
			folder = *in.Fields.FolderID
		}
		text := ""
		if in.Fields.SwitchText != nil {
			text = *in.Fields.SwitchText
		}
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationSceneCreate, map[string]any{"sceneId": in.SceneID, "name": *in.Fields.Name, "order": order, "folderId": folder, "switchText": text, "state": map[string]any{}}, nil)
	case "update":
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationSceneUpdate, map[string]any{"sceneId": in.SceneID, "fields": fields}, nil)
	case "reorder":
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationSceneReorder, map[string]any{"sceneIds": in.SceneIDs}, nil)
	case "delete":
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationSceneDelete, map[string]any{"sceneId": in.SceneID, "fallbackSceneId": in.FallbackSceneID}, nil)
	case "apply":
		if !a.Allows("theater:control") {
			return nil, service.ErrMCPScopeDenied
		}
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationSceneApply, map[string]any{"sceneId": in.SceneID}, nil)
	case "folders_update":
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationSceneFoldersUpdate, map[string]any{"folders": in.Folders}, nil)
	case "surface_embed_set", "surface_embed_clear":
		if in.Target != "background" && in.Target != "foreground" {
			return nil, mcpFailure("invalid_argument", "target 必须为 background/foreground")
		}
		var value any
		if in.Operation == "surface_embed_set" {
			if err := mcpTheaterValidateIframe(a, in.TheaterScope, in.URL); err != nil {
				return nil, err
			}
			scale := 1.0
			if in.Scale != nil {
				scale = *in.Scale
			}
			interactive := false
			if in.Interactive != nil {
				interactive = *in.Interactive
			}
			value = map[string]any{"type": "iframe", "iframe": mcpTheaterIframe{in.URL, scale}, "interactive": interactive}
		}
		patch["surfaceEmbeds"] = map[string]any{in.Target: value}
	case "surface_update":
		if in.Target != "background" && in.Target != "foreground" {
			return nil, mcpFailure("invalid_argument", "target 必须为 background/foreground")
		}
		if in.Clear {
			patch[in.Target] = nil
		} else if in.Image != nil {
			patch[in.Target] = in.Image
		}
		if in.Style != nil {
			patch["surfaceStyles"] = map[string]any{in.Target: in.Style}
		}
		if in.FieldWidth != nil {
			patch["fieldWidth"] = *in.FieldWidth
		}
		if in.FieldHeight != nil {
			patch["fieldHeight"] = *in.FieldHeight
		}
	case "overlay_update":
		if in.Overlays == nil {
			return nil, mcpFailure("invalid_argument", "overlays 必填")
		}
		patch["sceneOverlays"] = *in.Overlays
	case "music_update":
		if in.Clear {
			patch["musicSnapshot"] = nil
			patch["switchAudio"] = nil
		} else {
			if in.Music != nil {
				patch["musicSnapshot"] = in.Music
			}
			if in.SwitchAudio != nil {
				patch["switchAudio"] = in.SwitchAudio
			}
		}
	case "transition_update":
		if in.Transition == nil {
			return nil, mcpFailure("invalid_argument", "transition 必填")
		}
		patch["transition"] = in.Transition
	case "sequence_update":
		if in.Sequences == nil {
			return nil, mcpFailure("invalid_argument", "sequences 必填")
		}
		raw, err := json.Marshal(*in.Sequences)
		if err != nil {
			return nil, err
		}
		if err := service.ValidateTheaterMCPSequences(raw); err != nil {
			return nil, err
		}
		patch["theaterSequences"] = *in.Sequences
	default:
		return nil, mcpFailure("invalid_argument", "不支持的 scene operation")
	}
	if len(patch) == 0 {
		return nil, mcpFailure("invalid_argument", "局部更新不能为空")
	}
	// Normalize typed nested values into JSON objects before native validation.
	return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationSceneUpdate, map[string]any{"sceneId": in.SceneID, "fields": fields}, mcpTheaterMap(patch))
}

func mcpTheaterObject(ctx context.Context, a *service.MCPActor, in mcpTheaterObjectInput) (any, error) {
	allowed := map[string]string{"create": "objectId sceneId kind fields", "bind_character": "objectId sceneId kind fields identityId ownerUserId", "update": "objectId fields", "batch_update": "updates", "delete": "objectId cascade", "toggle": "objectId visible"}
	if err := mcpTheaterOperationFields(in, in.Operation, allowed); err != nil {
		return nil, err
	}
	if in.Fields != nil {
		if err := mcpTheaterValidateObjectFields(a, in.TheaterScope, *in.Fields); err != nil {
			return nil, err
		}
	}
	for _, update := range in.Updates {
		if err := mcpTheaterValidateObjectFields(a, in.TheaterScope, update.Fields); err != nil {
			return nil, err
		}
	}
	if in.Fields != nil && in.Fields.Content != nil && in.Fields.Content.Iframe != nil {
		if err := mcpTheaterValidateIframe(a, in.TheaterScope, in.Fields.Content.Iframe.URL); err != nil {
			return nil, err
		}
	}
	fields := map[string]any{}
	if in.Fields != nil {
		fields = mcpTheaterMap(in.Fields)
	}
	switch in.Operation {
	case "create", "bind_character":
		if in.Fields != nil && in.Fields.SceneID != nil {
			return nil, mcpFailure("invalid_argument", "create/bind_character 使用顶层 sceneId")
		}
		if in.Operation == "bind_character" {
			if in.Kind != "" && in.Kind != "character" {
				return nil, mcpFailure("invalid_argument", "bind_character 的 kind 必须为 character")
			}
			in.Kind = "character"
		}
		object := map[string]any{"id": in.ObjectID, "kind": in.Kind, "width": 8.0, "height": 5.0, "visible": true, "content": map[string]any{}, "actions": []any{}, "metadata": map[string]any{}}
		for k, v := range fields {
			if k == "embedEventBindings" {
				object["metadata"] = map[string]any{"embedEventBindings": v}
			} else if k != "sceneId" {
				object[k] = v
			}
		}
		payload := map[string]any{"sceneId": in.SceneID, "object": object}
		typ := service.TheaterMutationObjectCreate
		if in.Operation == "bind_character" {
			typ = service.TheaterMutationCharacterBind
			payload["identityId"] = in.IdentityID
			payload["ownerUserId"] = in.OwnerUserID
			payload["inputChannelId"] = in.InputChannelID
		}
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, typ, payload, nil)
	case "update":
		patch := mcpTheaterObjectMetadataPatch(in.ObjectID, fields, nil)
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationObjectUpdate, map[string]any{"objectId": in.ObjectID, "fields": fields}, patch)
	case "batch_update":
		var patch map[string]any
		updates := make([]map[string]any, 0, len(in.Updates))
		for _, u := range in.Updates {
			if u.Fields.Content != nil && u.Fields.Content.Iframe != nil {
				if err := mcpTheaterValidateIframe(a, in.TheaterScope, u.Fields.Content.Iframe.URL); err != nil {
					return nil, err
				}
			}
			updateFields := mcpTheaterMap(u.Fields)
			patch = mcpTheaterObjectMetadataPatch(u.ObjectID, updateFields, patch)
			updates = append(updates, map[string]any{"objectId": u.ObjectID, "fields": updateFields})
		}
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationObjectBatchUpdate, map[string]any{"updates": updates}, patch)
	case "delete":
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationObjectDelete, map[string]any{"objectId": in.ObjectID, "cascade": in.Cascade}, nil)
	case "toggle":
		// An explicit desired visibility makes retries deterministic. Saved actions
		// continue to use native object.toggle through their action permission path.
		if in.Visible == nil {
			return nil, mcpFailure("invalid_argument", "toggle 需要 visible 目标值")
		}
		return mcpTheaterMutation(ctx, a, in.mcpTheaterWriteInput, service.TheaterMutationObjectUpdate, map[string]any{"objectId": in.ObjectID, "fields": map[string]any{"visible": *in.Visible}}, nil)
	default:
		return nil, mcpFailure("invalid_argument", "不支持的 object operation")
	}
}

// mcpTheaterObjectMetadataPatch moves the typed embedEventBindings field out of
// native object fields into a metadata key patch merged by the service.
func mcpTheaterObjectMetadataPatch(objectID string, fields map[string]any, patch map[string]any) map[string]any {
	bindings, ok := fields["embedEventBindings"]
	if !ok {
		return patch
	}
	delete(fields, "embedEventBindings")
	if patch == nil {
		patch = map[string]any{"objectMetadata": map[string]any{}}
	}
	patch["objectMetadata"].(map[string]any)[objectID] = map[string]any{"embedEventBindings": bindings}
	return patch
}

func mcpTheaterValidateIframe(a *service.MCPActor, s service.TheaterScope, source string) error {
	var err error
	s, err = service.NormalizeTheaterMCPScope(a.User.ID, s)
	if err != nil {
		return err
	}
	u, err := url.Parse(strings.TrimSpace(source))
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || len(source) > 8192 {
		return mcpFailure("invalid_argument", "iframe URL 必须为 HTTP/HTTPS")
	}
	// Only configured internal origins are interpreted as channel iForm links.
	cfg := mcpConfigSnapshot()
	if !strings.HasPrefix(u.Fragment, "/internal/iform/") {
		return nil
	}
	if !service.IsTheaterInternalIFormURL(cfg, u) {
		return mcpFailure("invalid_argument", "内部 iForm URL 必须使用已配置可信域名和 webUrl 路径")
	}
	fragment, e := url.Parse(u.Fragment)
	if e != nil {
		return mcpFailure("invalid_argument", "iForm URL 无效")
	}
	id, e := url.PathUnescape(strings.TrimPrefix(fragment.Path, "/internal/iform/"))
	if e != nil || id == "" || strings.Contains(id, "/") {
		return mcpFailure("invalid_argument", "iForm ID 无效")
	}
	world, channel := fragment.Query().Get("world"), fragment.Query().Get("channel")
	if world != s.WorldID || channel == "" || channel != s.InputChannelID || (s.ScopeType == "channel" && channel != s.ChannelID) {
		return mcpFailure("invalid_argument", "iForm 必须匹配 worldId 和 inputChannelId；频道 Theater 还须匹配 channelId")
	}
	if _, e := mcpChannel(a, world, channel); e != nil {
		return e
	}
	forms, e := service.ListEffectiveChannelIForms(channel)
	if e != nil {
		return e
	}
	for _, f := range forms {
		if f.ID == id {
			return nil
		}
	}
	return mcpFailure("not_found", "iForm 不存在或不可见")
}

func mcpTheaterCatalog(ctx context.Context, a *service.MCPActor, in mcpTheaterReadInput) (any, error) {
	s, scope, err := mcpTheaterSnapshot(ctx, a, in.TheaterScope, in.Operation == "resources")
	if err != nil {
		return nil, err
	}
	out := map[string]any{"scope": scope, "revision": s.Revision, "contentTrust": "untrusted_user_generated"}
	switch in.Operation {
	case "limits":
		out["limits"] = service.TheaterMCPLimits()
	case "object_types":
		out["types"] = service.TheaterObjectKinds
		out["defaults"] = map[string]any{"width": 8, "height": 5, "scaleX": 1, "scaleY": 1, "visible": true, "interactive": false}
		out["fields"] = mcpTheaterObjectSchemaFields()
		out["actions"] = []string{"object.toggle", "effect.play", "scene.apply", "action.sequence", "chat.send", "chat.insert", "chat.random-table", "clue.execute"}
		out["sceneSequenceActions"] = []string{"object.trigger", "effect.play", "scene.apply"}
		out["clickActionKinds"] = []string{"drawing", "text", "image", "button"}
		out["embedEventBindings"] = map[string]any{
			"objectKinds": []string{"iframe"}, "maxBindings": service.TheaterMaxEmbedEventBindings, "maxActionsPerBinding": service.TheaterMaxEmbedEventBindingActions,
			"topicPattern": service.ChannelEmbedTopicPattern.String(), "storedAt": "metadata.embedEventBindings",
			"note": "精确 topic 映射到同一 iframe 对象已保存的 actions；事件 payload 不参与动作选择。iframe 不获得舞台点击语义，也不能作为 object.trigger 目标。",
		}
		out["coordinates"] = service.TheaterMCPLimits()
	case "effects":
		out["builtinThemes"] = service.TheaterEffectThemes
		out["runtimeCatalog"] = service.TheaterRenderers.Catalog(a.User.ID, scope)
		out["designCoordinates"] = service.TheaterMCPLimits()
	case "overlays":
		presets, e := service.ListTheaterSceneOverlayPresets(ctx, a.User.ID, scope.WorldID, scope.ChannelID)
		if e != nil {
			return nil, e
		}
		page, e := mcpSlicePage(presets, in.mcpPageInput)
		if e != nil {
			return nil, e
		}
		out["presets"] = page
		out["runtimeCatalog"] = service.TheaterRenderers.Catalog(a.User.ID, scope)
	case "resources":
		items := []service.TheaterResourcePublic{}
		for _, r := range s.Snapshot.Resources {
			items = append(items, r)
		}
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		page, e := mcpSlicePage(items, in.mcpPageInput)
		if e != nil {
			return nil, e
		}
		out["resources"] = page
	default:
		return nil, mcpFailure("invalid_argument", "operation 必须为 object_types/effects/overlays/resources/limits")
	}
	return out, nil
}
func mcpTheaterObjectSchemaFields() any {
	return []string{"sceneId", "parentId", "name", "x", "y", "width", "height", "rotation", "scale", "scaleX", "scaleY", "z", "orderKey", "visible", "locked", "aspectRatioLocked", "interactive", "editable", "content", "actions", "embedEventBindings"}
}
func mcpTheaterOperationFields(input any, operation string, operations map[string]string) error {
	fields, ok := operations[operation]
	if !ok {
		return mcpFailure("invalid_argument", "不支持的 operation")
	}
	allowed := strings.Fields("worldId scopeType channelId inputChannelId operation mutationId expectedRevision " + fields)
	for key := range mcpTheaterMap(input) {
		found := false
		for _, name := range allowed {
			if name == key {
				found = true
				break
			}
		}
		if !found {
			return mcpFailure("invalid_argument", "operation 不接受字段 "+key)
		}
	}
	return nil
}
func mcpTheaterValidateObjectFields(a *service.MCPActor, scope service.TheaterScope, fields mcpTheaterObjectFields) error {
	if fields.Content == nil || fields.Content.Iframe == nil {
		return nil
	}
	frame := fields.Content.Iframe
	if math.IsNaN(frame.Scale) || math.IsInf(frame.Scale, 0) || frame.Scale < 0.25 || frame.Scale > 5 {
		return mcpFailure("invalid_argument", "iframe.scale 范围 0.25..5")
	}
	return mcpTheaterValidateIframe(a, scope, frame.URL)
}
func mcpTheaterSpec[T any](name, description string, operations []string, scopes []string, write, destructive, idempotent bool, handler func(context.Context, *service.MCPActor, T) (any, error)) mcpToolSpec {
	return mcpSpec(name, description, scopes, write, destructive, idempotent, handler, func(s *jsonschema.Schema) {
		required := s.Required[:0]
		for _, field := range s.Required {
			if field != "channelId" {
				required = append(required, field)
			}
		}
		s.Required = required
		for _, field := range []string{"scopeType", "operation"} {
			values := operations
			if field == "scopeType" {
				values = []string{"world", "channel"}
			}
			for _, value := range values {
				s.Properties[field].Enum = append(s.Properties[field].Enum, value)
			}
		}
	})
}
func mcpTheaterTools() []mcpToolSpec {
	return []mcpToolSpec{
		mcpTheaterSpec("theater_read", "读取权限投影后的小剧场，按场景/对象分页。summary 对象页为指定或当前场景加常驻对象。普通对象采用中心锚点、WORLD_UNIT_PX=24；特效使用独立 1920x1080 设计坐标。world scope 的 channelId 必须为空，聊天使用 inputChannelId。", []string{"summary", "scene", "object", "events", "renderers"}, []string{"theater:read"}, false, false, true, mcpTheaterRead),
		mcpTheaterSpec("theater_catalog", "小剧场原生类型、动作、特效、环境叠层、素材和限制；浏览器注册表仅在 renderer 在线时可用。", []string{"object_types", "effects", "overlays", "resources", "limits"}, []string{"theater:read"}, false, false, true, mcpTheaterCatalog),
		mcpTheaterSpec("theater_scene", "结构化场景控制。state 局部更新在 expectedRevision 合并，保留其他字段；冲突后重读并使用新 mutationId。apply 另需 theater:control 与场景切换权限。", []string{"create", "update", "reorder", "delete", "apply", "folders_update", "surface_update", "surface_embed_set", "surface_embed_clear", "overlay_update", "music_update", "transition_update", "sequence_update"}, []string{"theater:write"}, true, true, true, mcpTheaterScene),
		mcpTheaterSpec("theater_object", "普通对象 CRUD、原生 object.batchUpdate、显隐和角色绑定。支持 content.iframe={url,scale} 与 interactive；仅引用已有频道 iForm，不管理代码（频道嵌入代码用 embed_save）。iframe 可保存 actions 并用 embedEventBindings=[{topic,actionIds}] 把嵌入 events.publish 的精确 topic 绑定到这些动作。batch_update 仅原子更新已有对象。content/actions/embedEventBindings 的更新遵循整体替换语义。", []string{"create", "update", "batch_update", "delete", "toggle", "bind_character"}, []string{"theater:write"}, true, true, true, mcpTheaterObject),
		mcpTheaterSpec("theater_control", "执行保存的动作/序列，查询或取消 execution。浏览器动作返回明确任务状态；所有业务步骤以 MCP Actor 服务端鉴权，不借用浏览器账户权限。", []string{"apply_scene", "trigger_action", "trigger_sequence", "execution_status", "cancel_execution"}, []string{"theater:control"}, true, false, false, mcpTheaterControl),
		mcpTheaterSpec("theater_view", "指定同账号、同 scope 授权 renderer 的本地相机和选择；不写共享状态。", []string{"get", "camera_set", "fit_scene", "focus_object", "select_objects", "clear_selection"}, []string{"theater:read"}, true, false, false, mcpTheaterView),
		mcpTheaterSpec("theater_capture", "浏览器协作截图 capture/status/cancel；返回 MCP ImageContent 和版本、坐标映射、缺失图层。无法完整捕获 iframe 等内容时返回 partial。", []string{"capture", "status", "cancel"}, []string{"theater:capture"}, true, false, false, mcpTheaterCapture),
	}
}
