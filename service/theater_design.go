package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

type theaterDesignConfigKey struct{}
type theaterDesignCommand struct {
	kind      string
	decoded   any
	stepIndex int
	stepKind  string
}
type theaterDesignPlanned struct {
	commands []theaterDesignCommand
	summary  TheaterDesignSummary
}

// Both operations enter the same normalization, permission preparation and pure
// document planner. Only apply enters the native CAS/audit/outbox transaction.
func RunTheaterMCPDesign(ctx context.Context, actor *MCPActor, scope TheaterScope, operation, mutationID string, expectedRevision int64, steps []json.RawMessage, cfg utils.AppConfig) (any, error) {
	if actor == nil || actor.User == nil || !actor.Allows("theater:write") {
		return nil, ErrMCPScopeDenied
	}
	scope, err := NormalizeTheaterMCPScope(actor.User.ID, scope)
	if err != nil {
		return nil, err
	}
	if expectedRevision < 0 {
		return nil, theaterPayloadError("expectedRevision 无效")
	}
	plan := TheaterDesignPlan{InputChannelID: scope.InputChannelID, Steps: steps}
	raw, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, theaterDesignConfigKey{}, cfg)
	switch operation {
	case "validate":
		if mutationID != "" {
			return nil, theaterPayloadError("validate 不接受 mutationId")
		}
		return ValidateTheaterDesignPlan(ctx, actor.User.ID, scope.WorldID, scope.ChannelID, expectedRevision, raw)
	case "apply":
		r, err := ApplyTheaterMCPMutation(ctx, actor.User.ID, actor.CredentialID, TheaterMutationCommand{WorldID: scope.WorldID, ChannelID: scope.ChannelID, MutationID: mutationID, ExpectedRevision: expectedRevision, Type: TheaterMutationDesignApply, Payload: raw}, nil)
		if err != nil {
			return nil, err
		}
		var affected TheaterDesignSummary
		if err = json.Unmarshal(r.Payload, &affected); err != nil {
			return nil, err
		}
		return map[string]any{"scope": scope, "mutationId": r.MutationID, "revisionBefore": r.RevisionBefore, "revision": r.Revision, "checksum": r.Checksum, "idempotent": r.Idempotent, "affected": affected, "sceneIds": affected.ScenesCreated, "objectIds": affected.ObjectsCreated, "warnings": affected.Warnings}, nil
	default:
		return nil, theaterPayloadError("operation 必须为 validate/apply")
	}
}

func prepareTheaterDesign(ctx context.Context, actorID, worldID, channelID string, plan *TheaterDesignPlan) error {
	scopeType := "world"
	if channelID != "" {
		scopeType = "channel"
	}
	scope, err := NormalizeTheaterMCPScope(actorID, TheaterScope{WorldID: worldID, ChannelID: channelID, ScopeType: scopeType, InputChannelID: plan.InputChannelID})
	if err != nil {
		return err
	}
	plan.InputChannelID = scope.InputChannelID
	_, channel, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionObjectEdit)
	if err != nil {
		return err
	} // No delegated-edit fallback for a Design Plan.
	if channel != nil && channel.Status != "" && channel.Status != model.ChannelStatusActive {
		return newTheaterError(TheaterErrorPermissionDenied, "归档频道不可写 Theater", 403, nil)
	}
	plan.actorID = actorID
	plan.admin = IsWorldAdmin(worldID, actorID) || pm.CanWithSystemRole(actorID, pm.PermModAdmin)
	if cfg := utils.GetConfig(); cfg != nil {
		plan.config = *cfg
	}
	if ctx != nil {
		if cfg, ok := ctx.Value(theaterDesignConfigKey{}).(utils.AppConfig); ok {
			plan.config = cfg
		}
	}
	for index, step := range plan.steps {
		switch s := step.(type) {
		case *TheaterDesignCharacterBind:
			if _, _, err = requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionCharacterEdit); err == nil {
				s.ownerAuthorized = s.OwnerUserID == actorID || plan.admin
				if !s.ownerAuthorized {
					err = newTheaterError(TheaterErrorPermissionDenied, "不能绑定他人角色", 403, nil)
				}
			}
		}
		if err != nil {
			return theaterDesignStepError(index, theaterDesignKindOf(plan.Steps[index]), err)
		}
	}
	return nil
}

// Entity/URL preflight is outside the write transaction because the existing
// iForm permission resolvers use the shared connection pool. Authenticated
// idempotent retries skip this preflight and return the original result.
func validateTheaterDesignInputs(plan *TheaterDesignPlan, worldID, channelID string) error {
	scopeType := "world"
	if channelID != "" {
		scopeType = "channel"
	}
	scope := TheaterScope{WorldID: worldID, ChannelID: channelID, ScopeType: scopeType, InputChannelID: plan.InputChannelID}
	for index, step := range plan.steps {
		var frameURL string
		var fields *TheaterMCPObjectFields
		switch s := step.(type) {
		case *TheaterDesignSceneSurfaceEmbed:
			if !s.Clear {
				frameURL = s.URL
			}
		case *TheaterDesignObjectCreate:
			fields = &s.Fields
		case *TheaterDesignObjectUpdate:
			fields = &s.Fields
		case *TheaterDesignCharacterBind:
			fields = &s.Fields
		}
		var err error
		if err == nil && fields != nil {
			err = ValidateTheaterMCPObjectFields(plan.actorID, scope, *fields, plan.config)
		}
		if err == nil && frameURL != "" {
			err = ValidateTheaterMCPIframeURL(plan.actorID, scope, frameURL, plan.config)
		}
		if err != nil {
			return theaterDesignStepError(index, theaterDesignKindOf(plan.Steps[index]), err)
		}
	}
	return nil
}

// This read transaction never creates a room, writes audit/resource refs, or
// enters an outbox. Revision is checked against the exact document being read.
func ValidateTheaterDesignPlan(ctx context.Context, actorID, worldID, channelID string, expectedRevision int64, raw json.RawMessage) (*TheaterDesignSummary, error) {
	decoded, _, err := decodeTheaterPayload(TheaterMutationDesignApply, raw)
	if err != nil {
		return nil, err
	}
	plan := decoded.(*TheaterDesignPlan)
	if err = prepareTheaterDesign(ctx, actorID, worldID, channelID, plan); err != nil {
		return nil, err
	}
	if err = validateTheaterDesignInputs(plan, worldID, channelID); err != nil {
		return nil, err
	}
	var summary *TheaterDesignSummary
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		room := emptyTheaterDesignRoom(actorID, worldID, channelID)
		var current model.TheaterRoomModel
		if err := tx.Where("world_id = ? AND channel_id = ?", worldID, channelID).Limit(1).Find(&current).Error; err != nil {
			return err
		}
		if current.ID != "" {
			room = &current
		}
		if room.Revision != expectedRevision {
			return theaterDesignRevisionConflict(expectedRevision, room.Revision)
		}
		planned, err := planTheaterDesign(tx, room, plan)
		if err != nil {
			return err
		}
		summary = &planned.summary
		return nil
	})
	return summary, err
}

func emptyTheaterDesignRoom(actorID, worldID, channelID string) *model.TheaterRoomModel {
	scope := "world"
	if channelID != "" {
		scope = "channel"
	}
	return &model.TheaterRoomModel{WorldID: worldID, ChannelID: channelID, ScopeType: scope, SchemaVersion: model.TheaterSchemaVersion, Status: "active", StateJSON: "{}", CreatedBy: actorID, UpdatedBy: actorID}
}
func theaterDesignRevisionConflict(expected, current int64) error {
	return newTheaterError(TheaterErrorRevisionConflict, "Theater revision 冲突", 409, map[string]any{"expectedRevision": expected, "currentRevision": current})
}
func theaterDesignKindOf(raw json.RawMessage) string {
	var h TheaterDesignKind
	_ = json.Unmarshal(raw, &h)
	return h.Kind
}

func planTheaterDesign(tx *gorm.DB, room *model.TheaterRoomModel, plan *TheaterDesignPlan) (*theaterDesignPlanned, error) {
	snapshot, _, err := readTheaterSnapshot(tx, room, false, false)
	if err != nil {
		return nil, err
	}
	result := &theaterDesignPlanned{summary: TheaterDesignSummary{Valid: true, Revision: room.Revision, StepCount: len(plan.steps), ScenesCreated: []string{}, ScenesUpdated: []string{}, ScenesDeleted: []string{}, ObjectsCreated: []string{}, ObjectsUpdated: []string{}, ObjectsDeleted: []string{}, Layout: []TheaterDesignLayoutSummary{}, Warnings: []string{}}}
	for index, step := range plan.steps {
		kind := theaterDesignKindOf(plan.Steps[index])
		nativeType, payload, err := theaterDesignNativeStep(snapshot, room, plan, step)
		if err == nil {
			var raw json.RawMessage
			raw, err = json.Marshal(payload)
			if err == nil {
				payload, _, err = decodeTheaterPayload(nativeType, raw)
			}
		}
		if err == nil {
			if bind, ok := payload.(*theaterCharacterBindPayload); ok {
				bind.ownerAuthorized = step.(*TheaterDesignCharacterBind).ownerAuthorized
				err = validateTheaterCharacterBindReference(tx, room, bind)
			}
		}
		if err == nil {
			err = applyTheaterDesignDocumentStep(tx, &snapshot, room, plan, nativeType, payload, &result.summary)
		}
		if err != nil {
			return nil, theaterDesignStepError(index, kind, err)
		}
		result.commands = append(result.commands, theaterDesignCommand{nativeType, payload, index, kind})
		if strings.HasPrefix(kind, "layout.") {
			ids := []string{}
			for _, u := range payload.(*theaterObjectBatchUpdatePayload).Updates {
				ids = append(ids, u.ObjectID)
			}
			result.summary.Layout = append(result.summary.Layout, TheaterDesignLayoutSummary{index, kind, ids})
		}
	}
	if err = validateTheaterDesignDocument(tx, room, snapshot); err != nil {
		index := theaterDesignErrorStep(result.commands, err)
		return nil, theaterDesignStepError(index, theaterDesignKindOf(plan.Steps[index]), err)
	}
	return result, nil
}

func theaterDesignNativeStep(snapshot TheaterSharedSnapshot, room *model.TheaterRoomModel, plan *TheaterDesignPlan, step any) (string, any, error) {
	var sceneID string
	patch := map[string]any{}
	switch s := step.(type) {
	case *TheaterDesignSceneCreate:
		if s.Fields.Name == nil || s.Fields.Locked != nil || s.Fields.Published != nil {
			return "", nil, theaterPayloadError("scene.create 需要 fields.name，不接受 locked/published")
		}
		return TheaterMutationSceneCreate, &theaterSceneCreatePayload{SceneID: s.SceneID, Name: *s.Fields.Name, SwitchText: derefString(s.Fields.SwitchText), Order: theaterInt64OrZero(s.Fields.Order), FolderID: derefString(s.Fields.FolderID), State: map[string]any{}}, nil
	case *TheaterDesignSceneUpdate:
		return TheaterMutationSceneUpdate, &theaterSceneUpdatePayload{s.SceneID, theaterMCPMap(s.Fields)}, nil
	case *TheaterDesignSceneDelete:
		return TheaterMutationSceneDelete, &theaterSceneDeletePayload{s.SceneID, s.FallbackSceneID}, nil
	case *TheaterDesignSceneReorder:
		return TheaterMutationSceneReorder, &theaterSceneReorderPayload{s.SceneIDs}, nil
	case *TheaterDesignSceneFolders:
		p := &theaterSceneFoldersUpdatePayload{Folders: []theaterSceneFolderInput{}}
		for _, f := range s.Folders {
			p.Folders = append(p.Folders, theaterSceneFolderInput{f.ID, f.Name})
		}
		return TheaterMutationSceneFoldersUpdate, p, nil
	case *TheaterDesignSceneField:
		sceneID = s.SceneID
		patch = TheaterMCPFieldPatch(s.Fields)
	case *TheaterDesignSceneSurface:
		sceneID = s.SceneID
		if s.Target != "background" && s.Target != "foreground" {
			return "", nil, theaterPayloadError("surface target 无效")
		}
		if s.Clear && s.Image != nil {
			return "", nil, theaterPayloadError("clear/image 不能同时指定")
		}
		if s.Clear {
			patch[s.Target] = nil
		} else if s.Image != nil {
			patch[s.Target] = s.Image
		}
		if s.Style != nil {
			patch["surfaceStyles"] = map[string]any{s.Target: s.Style}
		}
	case *TheaterDesignSceneSurfaceEmbed:
		sceneID = s.SceneID
		if s.Target != "background" && s.Target != "foreground" {
			return "", nil, theaterPayloadError("surface_embed target 无效")
		}
		var value any
		if s.Clear {
			if s.URL != "" || s.Scale != nil || s.Interactive != nil {
				return "", nil, theaterPayloadError("surface_embed clear 不接受 url/scale/interactive")
			}
		} else {
			if strings.TrimSpace(s.URL) == "" {
				return "", nil, theaterPayloadError("surface_embed url 必填")
			}
			scale := 1.0
			if s.Scale != nil {
				scale = *s.Scale
			}
			interactive := false
			if s.Interactive != nil {
				interactive = *s.Interactive
			}
			value = map[string]any{"type": "iframe", "iframe": TheaterMCPIframe{URL: s.URL, Scale: scale}, "interactive": interactive}
		}
		patch["surfaceEmbeds"] = map[string]any{s.Target: value}
	case *TheaterDesignSceneOverlay:
		sceneID = s.SceneID
		if s.Overlays == nil {
			return "", nil, theaterPayloadError("overlays 必填")
		}
		patch["sceneOverlays"] = s.Overlays
	case *TheaterDesignSceneMusic:
		sceneID = s.SceneID
		if s.Clear {
			if s.Music != nil || s.SwitchAudio != nil {
				return "", nil, theaterPayloadError("music clear 不接受 music/switchAudio")
			}
			patch["musicSnapshot"] = nil
			patch["switchAudio"] = nil
		} else {
			if s.Music != nil {
				patch["musicSnapshot"] = s.Music
			}
			if s.SwitchAudio != nil {
				patch["switchAudio"] = s.SwitchAudio
			}
		}
	case *TheaterDesignSceneTransition:
		sceneID = s.SceneID
		raw, err := json.Marshal(s.Transition)
		if err != nil {
			return "", nil, err
		}
		var transition theaterTransitionPayload
		if err := decodeStrictJSON(raw, &transition); err != nil {
			return "", nil, theaterPayloadError("transition 无效")
		}
		if err := validateDecodedTheaterPayload(TheaterMutationSceneApply, &theaterSceneApplyPayload{SceneID: sceneID, Transition: &transition}); err != nil {
			return "", nil, err
		}
		patch["transition"] = s.Transition
	case *TheaterDesignSceneSequence:
		sceneID = s.SceneID
		raw, _ := json.Marshal(s.Sequences)
		if err := ValidateTheaterMCPSequences(raw); err != nil {
			return "", nil, err
		}
		patch["theaterSequences"] = s.Sequences
	case *TheaterDesignObjectCreate:
		p, err := theaterDesignCreatePayload(*s)
		return TheaterMutationObjectCreate, p, err
	case *TheaterDesignCharacterBind:
		if s.ObjectType != "character" {
			return "", nil, theaterPayloadError("character.bind objectType 必须为 character")
		}
		p, err := theaterDesignCreatePayload(s.TheaterDesignObjectCreate)
		if err != nil {
			return "", nil, err
		}
		return TheaterMutationCharacterBind, &theaterCharacterBindPayload{InputChannelID: plan.InputChannelID, SceneID: p.SceneID, Object: p.Object, IdentityID: s.IdentityID, OwnerUserID: s.OwnerUserID}, nil
	case *TheaterDesignObjectUpdate:
		fields := theaterMCPMap(s.Fields)
		if bindings, ok := fields["embedEventBindings"]; ok {
			object, found := theaterDesignFindObject(snapshot, s.ObjectID)
			if !found {
				return "", nil, newTheaterError(TheaterErrorNotFound, "对象不存在", 404, nil)
			}
			metadata := map[string]any{}
			_ = json.Unmarshal(object.Metadata, &metadata)
			if metadata == nil {
				metadata = map[string]any{}
			}
			metadata["embedEventBindings"] = bindings
			delete(fields, "embedEventBindings")
			fields["metadata"] = metadata
		}
		return TheaterMutationObjectUpdate, &theaterObjectUpdatePayload{s.ObjectID, fields}, nil
	case *TheaterDesignObjectDelete:
		return TheaterMutationObjectDelete, &theaterObjectDeletePayload{s.ObjectID, s.Cascade}, nil
	case *TheaterDesignLayoutAlign, *TheaterDesignLayoutDistribute, *TheaterDesignLayoutGrid:
		p, _, err := planTheaterLayout(snapshot, step)
		return TheaterMutationObjectBatchUpdate, p, err
	default:
		return "", nil, theaterPayloadError("不支持的 design step")
	}
	if len(patch) == 0 {
		return "", nil, theaterPayloadError("scene patch 不能为空")
	}
	scene, ok := snapshot.Scenes[sceneID]
	if !ok {
		return "", nil, newTheaterError(TheaterErrorNotFound, "场景不存在", 404, nil)
	}
	state := map[string]any{}
	_ = json.Unmarshal(scene.State, &state)
	if state == nil {
		state = map[string]any{}
	}
	mergeTheaterMCPState(state, theaterMCPMap(patch))
	return TheaterMutationSceneUpdate, &theaterSceneUpdatePayload{sceneID, map[string]any{"state": state}}, nil
}

func theaterInt64OrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
func theaterDesignCreatePayload(step TheaterDesignObjectCreate) (*theaterObjectCreatePayload, error) {
	if step.Fields.SceneID != nil {
		return nil, theaterPayloadError("object.create 使用 step.sceneId")
	}
	object := map[string]any{"id": step.ObjectID, "kind": step.ObjectType, "width": 8.0, "height": 5.0, "visible": true, "content": map[string]any{}, "actions": []any{}, "metadata": map[string]any{}}
	for key, value := range theaterMCPMap(step.Fields) {
		if key == "embedEventBindings" {
			object["metadata"] = map[string]any{"embedEventBindings": value}
		} else {
			object[key] = value
		}
	}
	raw, _ := json.Marshal(object)
	p := &theaterObjectCreatePayload{SceneID: step.SceneID}
	if err := decodeStrictJSON(raw, &p.Object); err != nil {
		return nil, theaterPayloadError(err.Error())
	}
	return p, nil
}

func theaterDesignFindObject(snapshot TheaterSharedSnapshot, id string) (TheaterObjectSnapshot, bool) {
	if o, ok := snapshot.PersistentObjects[id]; ok {
		return o, true
	}
	for _, s := range snapshot.Scenes {
		if o, ok := s.Objects[id]; ok {
			return o, true
		}
	}
	return TheaterObjectSnapshot{}, false
}
func theaterDesignPutObject(snapshot *TheaterSharedSnapshot, o TheaterObjectSnapshot) {
	delete(snapshot.PersistentObjects, o.ID)
	delete(snapshot.Characters, o.ID)
	for id, s := range snapshot.Scenes {
		delete(s.Objects, o.ID)
		snapshot.Scenes[id] = s
	}
	if derefString(o.SceneID) == "" {
		o.SceneID = nil
		snapshot.PersistentObjects[o.ID] = o
	} else {
		s := snapshot.Scenes[*o.SceneID]
		s.Objects[o.ID] = o
		snapshot.Scenes[*o.SceneID] = s
	}
	if o.Kind == "character" {
		snapshot.Characters[o.ID] = o
	}
}
func theaterDesignRemoveObject(snapshot *TheaterSharedSnapshot, id string) {
	delete(snapshot.PersistentObjects, id)
	delete(snapshot.Characters, id)
	for key, s := range snapshot.Scenes {
		delete(s.Objects, id)
		snapshot.Scenes[key] = s
	}
}
func theaterDesignSceneUnlocked(scene TheaterSceneSnapshot) error {
	if scene.Locked {
		return newTheaterError(TheaterErrorPermissionDenied, "场景已锁定", 403, map[string]any{"sceneId": scene.ID})
	}
	return nil
}
func theaterDesignObjectUnlocked(snapshot TheaterSharedSnapshot, object TheaterObjectSnapshot) error {
	if scene, ok := snapshot.Scenes[derefString(object.SceneID)]; ok {
		if err := theaterDesignSceneUnlocked(scene); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for {
		if object.Locked {
			return newTheaterError(TheaterErrorPermissionDenied, "对象已锁定", 403, map[string]any{"objectId": object.ID})
		}
		if object.ParentID == nil || *object.ParentID == "" {
			return nil
		}
		if seen[object.ID] {
			return theaterPayloadError("parent 循环")
		}
		seen[object.ID] = true
		parent, ok := theaterDesignFindObject(snapshot, *object.ParentID)
		if !ok {
			return theaterPayloadError("parent 不存在")
		}
		object = parent
	}
}

func applyTheaterDesignDocumentStep(tx *gorm.DB, snapshot *TheaterSharedSnapshot, room *model.TheaterRoomModel, plan *TheaterDesignPlan, kind string, decoded any, summary *TheaterDesignSummary) error {
	sceneExists := func(id string) (TheaterSceneSnapshot, error) {
		s, ok := snapshot.Scenes[id]
		if !ok {
			return s, newTheaterError(TheaterErrorNotFound, "场景不存在", 404, nil)
		}
		return s, nil
	}
	folderExists := func(id string) bool {
		if id == "" {
			return true
		}
		for _, f := range snapshot.SceneFolders {
			if f.ID == id {
				return true
			}
		}
		return false
	}
	switch p := decoded.(type) {
	case *theaterSceneCreatePayload:
		if _, ok := snapshot.Scenes[p.SceneID]; ok {
			return theaterPayloadError("sceneId 已存在")
		}
		var count int64
		if err := tx.Model(&model.TheaterSceneModel{}).Where("id = ?", p.SceneID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return theaterPayloadError("sceneId 已存在")
		}
		if len(snapshot.Scenes) >= theaterMaxScenes {
			return newTheaterError(TheaterErrorLimitExceeded, "场景数量超限", 409, nil)
		}
		if !folderExists(p.FolderID) {
			return theaterPayloadError("folderId 不存在")
		}
		raw, _ := json.Marshal(p.State)
		snapshot.Scenes[p.SceneID] = TheaterSceneSnapshot{ID: p.SceneID, Name: strings.TrimSpace(p.Name), SwitchText: p.SwitchText, Order: p.Order, FolderID: p.FolderID, State: raw, Objects: map[string]TheaterObjectSnapshot{}}
		if snapshot.ActiveSceneID == nil {
			id := p.SceneID
			snapshot.ActiveSceneID = &id
		}
		summary.ScenesCreated = theaterAppendUnique(summary.ScenesCreated, p.SceneID)
	case *theaterSceneUpdatePayload:
		s, err := sceneExists(p.SceneID)
		if err != nil {
			return err
		}
		if err = theaterDesignSceneUnlocked(s); err != nil {
			return err
		}
		if folder, ok := p.Fields["folderId"]; ok && !folderExists(fmt.Sprint(folder)) {
			return theaterPayloadError("folderId 不存在")
		}
		data := theaterMCPMap(s)
		for k, v := range p.Fields {
			data[k] = v
		}
		raw, _ := json.Marshal(data)
		if err := json.Unmarshal(raw, &s); err != nil {
			return theaterPayloadError("scene fields 无效")
		}
		s.Name = strings.TrimSpace(s.Name)
		snapshot.Scenes[s.ID] = s
		summary.ScenesUpdated = theaterAppendUnique(summary.ScenesUpdated, s.ID)
	case *theaterSceneDeletePayload:
		s, err := sceneExists(p.SceneID)
		if err != nil {
			return err
		}
		if err = theaterDesignSceneUnlocked(s); err != nil {
			return err
		}
		if len(snapshot.Scenes) <= 1 {
			return theaterPayloadError("至少保留一个场景")
		}
		if derefString(snapshot.ActiveSceneID) == s.ID {
			if p.FallbackSceneID == s.ID || p.FallbackSceneID == "" {
				return theaterPayloadError("删除当前场景必须指定 fallbackSceneId")
			}
			if _, err = sceneExists(p.FallbackSceneID); err != nil {
				return err
			}
			id := p.FallbackSceneID
			snapshot.ActiveSceneID = &id
		}
		for id, o := range s.Objects {
			if err = theaterDesignObjectUnlocked(*snapshot, o); err != nil {
				return err
			}
			delete(snapshot.Characters, id)
			summary.ObjectsDeleted = theaterAppendUnique(summary.ObjectsDeleted, id)
		}
		delete(snapshot.Scenes, s.ID)
		summary.ScenesDeleted = theaterAppendUnique(summary.ScenesDeleted, s.ID)
	case *theaterSceneReorderPayload:
		if len(p.SceneIDs) != len(snapshot.Scenes) {
			return theaterPayloadError("sceneIds 必须包含当前全部场景")
		}
		for order, id := range p.SceneIDs {
			s, err := sceneExists(id)
			if err != nil {
				return err
			}
			if err = theaterDesignSceneUnlocked(s); err != nil {
				return err
			}
			s.Order = int64(order)
			snapshot.Scenes[id] = s
			summary.ScenesUpdated = theaterAppendUnique(summary.ScenesUpdated, id)
		}
	case *theaterSceneFoldersUpdatePayload:
		snapshot.SceneFolders = []TheaterSceneFolder{}
		for _, f := range p.Folders {
			snapshot.SceneFolders = append(snapshot.SceneFolders, TheaterSceneFolder{strings.TrimSpace(f.ID), strings.TrimSpace(f.Name)})
		}
		for id, s := range snapshot.Scenes {
			if !folderExists(s.FolderID) {
				if err := theaterDesignSceneUnlocked(s); err != nil {
					return err
				}
				s.FolderID = ""
				snapshot.Scenes[id] = s
				summary.ScenesUpdated = theaterAppendUnique(summary.ScenesUpdated, id)
			}
		}
		state := map[string]any{}
		_ = json.Unmarshal(snapshot.LiveState, &state)
		state["sceneFolders"] = snapshot.SceneFolders
		snapshot.LiveState, _ = json.Marshal(state)
	case *theaterCharacterBindPayload:
		return applyTheaterDesignDocumentStep(tx, snapshot, room, plan, TheaterMutationObjectCreate, &theaterObjectCreatePayload{SceneID: p.SceneID, Object: p.Object}, summary)
	case *theaterObjectCreatePayload:
		if len(theaterDesignAllObjects(*snapshot)) >= theaterMaxObjects {
			return newTheaterError(TheaterErrorLimitExceeded, "对象数量超限", 409, nil)
		}
		if _, ok := theaterDesignFindObject(*snapshot, p.Object.ID); ok {
			return theaterPayloadError("objectId 已存在")
		}
		var count int64
		if err := tx.Model(&model.TheaterObjectModel{}).Where("id = ?", p.Object.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return theaterPayloadError("objectId 已存在")
		}
		if id := derefString(p.SceneID); id != "" {
			s, err := sceneExists(id)
			if err != nil {
				return err
			}
			if err = theaterDesignSceneUnlocked(s); err != nil {
				return err
			}
			if len(s.Objects) >= theaterMaxSceneObjects {
				return newTheaterError(TheaterErrorLimitExceeded, "场景对象数量超限", 409, nil)
			}
		}
		if p.Object.ParentID != nil && *p.Object.ParentID != "" {
			parent, ok := theaterDesignFindObject(*snapshot, *p.Object.ParentID)
			if !ok || parent.Kind != "group" || derefString(parent.SceneID) != derefString(p.SceneID) {
				return theaterPayloadError("parent 范围或类型无效")
			}
			if err := theaterDesignObjectUnlocked(*snapshot, parent); err != nil {
				return err
			}
		}
		o := theaterObjectModelFromInput(room, plan.actorID, derefString(p.SceneID), &p.Object)
		theaterDesignPutObject(snapshot, theaterObjectSnapshotFromModel(o))
		summary.ObjectsCreated = theaterAppendUnique(summary.ObjectsCreated, o.ID)
	case *theaterObjectUpdatePayload:
		o, ok := theaterDesignFindObject(*snapshot, p.ObjectID)
		if !ok {
			return newTheaterError(TheaterErrorNotFound, "对象不存在", 404, nil)
		}
		if err := theaterDesignObjectUnlocked(*snapshot, o); err != nil {
			return err
		}
		if _, scopeChanged := p.Fields["sceneId"]; scopeChanged && o.Kind != "group" && o.Kind != "effect" {
			return theaterPayloadError("只有组或特效可以调整跨场景属性")
		}
		if _, scopeChanged := p.Fields["sceneId"]; scopeChanged && o.Kind == "effect" && derefString(o.ParentID) != "" {
			if _, parentUpdated := p.Fields["parentId"]; !parentUpdated {
				return theaterPayloadError("特效调整跨场景属性时必须同时更新 parentId")
			}
		}
		data := theaterMCPMap(o)
		for k, v := range p.Fields {
			data[k] = v
		}
		if v, ok := p.Fields["scale"]; ok {
			data["scaleX"] = v
			data["scaleY"] = v
		}
		if v, ok := p.Fields["scaleX"]; ok {
			data["scale"] = v
			data["scaleX"] = v
		}
		if v, ok := p.Fields["scaleY"]; ok {
			data["scaleY"] = v
		}
		raw, _ := json.Marshal(data)
		if err := json.Unmarshal(raw, &o); err != nil {
			return theaterPayloadError("object fields 无效")
		}
		if id := derefString(o.SceneID); id != "" {
			s, err := sceneExists(id)
			if err != nil {
				return err
			}
			if err = theaterDesignSceneUnlocked(s); err != nil {
				return err
			}
		}
		if o.Kind == "group" {
			var actions []any
			_ = json.Unmarshal(o.Actions, &actions)
			if o.Interactive || o.Editable || len(actions) > 0 {
				return theaterPayloadError("组不能交互、成员编辑或设置点击动作")
			}
		}
		if err := validateObjectInput(theaterDesignObjectInput(o)); err != nil {
			return err
		}
		if _, parentChanged := p.Fields["parentId"]; parentChanged {
			if err := validateTheaterDesignParent(*snapshot, o); err != nil {
				return err
			}
			if parent, found := theaterDesignFindObject(*snapshot, derefString(o.ParentID)); found {
				if err := theaterDesignObjectUnlocked(*snapshot, parent); err != nil {
					return err
				}
			}
		}
		theaterDesignPutObject(snapshot, o)
		if _, scopeChanged := p.Fields["sceneId"]; !scopeChanged {
			for _, object := range theaterDesignAllObjects(*snapshot) {
				if err := validateTheaterDesignParent(*snapshot, object); err != nil {
					return err
				}
			}
		}
		summary.ObjectsUpdated = theaterAppendUnique(summary.ObjectsUpdated, o.ID)
	case *theaterObjectBatchUpdatePayload:
		for _, u := range p.Updates {
			if err := applyTheaterDesignDocumentStep(tx, snapshot, room, plan, TheaterMutationObjectUpdate, &u, summary); err != nil {
				return err
			}
		}
	case *theaterObjectDeletePayload:
		o, ok := theaterDesignFindObject(*snapshot, p.ObjectID)
		if !ok {
			return newTheaterError(TheaterErrorNotFound, "对象不存在", 404, nil)
		}
		ids := []string{o.ID}
		for i := 0; i < len(ids); i++ {
			for _, child := range theaterDesignAllObjects(*snapshot) {
				if derefString(child.ParentID) == ids[i] {
					if !p.Cascade {
						return theaterPayloadError("对象存在子对象，必须显式 cascade")
					}
					ids = append(ids, child.ID)
				}
			}
		}
		for _, id := range ids {
			child, _ := theaterDesignFindObject(*snapshot, id)
			if err := theaterDesignObjectUnlocked(*snapshot, child); err != nil {
				return err
			}
		}
		for _, id := range ids {
			theaterDesignRemoveObject(snapshot, id)
			summary.ObjectsDeleted = theaterAppendUnique(summary.ObjectsDeleted, id)
		}
	default:
		return theaterPayloadError("非法 design native step")
	}
	return nil
}

func theaterAppendUnique(ids []string, id string) []string {
	for _, v := range ids {
		if id == v {
			return ids
		}
	}
	return append(ids, id)
}
func theaterDesignAllObjects(snapshot TheaterSharedSnapshot) map[string]TheaterObjectSnapshot {
	objects := map[string]TheaterObjectSnapshot{}
	for id, o := range snapshot.PersistentObjects {
		objects[id] = o
	}
	for _, s := range snapshot.Scenes {
		for id, o := range s.Objects {
			objects[id] = o
		}
	}
	return objects
}
func theaterDesignObjectInput(o TheaterObjectSnapshot) *theaterObjectInput {
	return &theaterObjectInput{ID: o.ID, ParentID: o.ParentID, Kind: o.Kind, Name: o.Name, X: o.X, Y: o.Y, Width: o.Width, Height: o.Height, Rotation: o.Rotation, Scale: &o.Scale, ScaleX: &o.ScaleX, ScaleY: &o.ScaleY, Z: o.Z, OrderKey: o.OrderKey, Visible: &o.Visible, Locked: o.Locked, AspectRatioLocked: o.AspectRatioLocked, Interactive: o.Interactive, Editable: o.Editable, OwnerUserID: o.OwnerUserID, CharacterIdentityID: o.CharacterIdentityID, Content: o.Content, Actions: o.Actions, Metadata: o.Metadata}
}

func validateTheaterDesignParent(snapshot TheaterSharedSnapshot, o TheaterObjectSnapshot) error {
	seen := map[string]bool{o.ID: true}
	child := o
	for derefString(child.ParentID) != "" {
		id := *child.ParentID
		parent, ok := theaterDesignFindObject(snapshot, id)
		if !ok || parent.Kind != "group" || derefString(parent.SceneID) != derefString(o.SceneID) || seen[id] {
			return newTheaterError(TheaterErrorPayloadInvalid, "parent 范围、类型或循环无效", 400, map[string]any{"objectId": o.ID})
		}
		seen[id] = true
		child = parent
	}
	return nil
}

// Final reference errors are attributed to the latest step changing the source
// or deleting its target; forward references remain legal within a plan.
func theaterDesignErrorStep(commands []theaterDesignCommand, err error) int {
	native, ok := err.(*TheaterError)
	if ok {
		for i := len(commands) - 1; i >= 0; i-- {
			command := commands[i]
			raw, _ := json.Marshal(command.decoded)
			for _, key := range []string{"objectId", "sceneId", "referenceId", "resourceId", "assetId"} {
				id, _ := native.Details[key].(string)
				if id == "" {
					continue
				}
				if key == "resourceId" || key == "assetId" {
					var value any
					_ = json.Unmarshal(raw, &value)
					if _, found := collectJSONFieldStrings(value, key)[id]; found {
						return command.stepIndex
					}
					continue
				}
				matches := false
				switch p := command.decoded.(type) {
				case *theaterObjectCreatePayload:
					matches = p.Object.ID == id
				case *theaterObjectUpdatePayload:
					matches = p.ObjectID == id
				case *theaterObjectDeletePayload:
					matches = p.ObjectID == id
				case *theaterCharacterBindPayload:
					matches = p.Object.ID == id
				case *theaterSceneCreatePayload:
					matches = p.SceneID == id
				case *theaterSceneUpdatePayload:
					matches = p.SceneID == id
				case *theaterSceneDeletePayload:
					matches = p.SceneID == id
				}
				if matches {
					return command.stepIndex
				}
			}
		}
	}
	return commands[len(commands)-1].stepIndex
}

func validateTheaterDesignDocument(tx *gorm.DB, room *model.TheaterRoomModel, snapshot TheaterSharedSnapshot) error {
	if err := validateTheaterSharedSnapshot(snapshot); err != nil {
		return err
	}
	objects := theaterDesignAllObjects(snapshot)
	for _, s := range snapshot.Scenes {
		if len(s.Objects) > theaterMaxSceneObjects {
			return newTheaterError(TheaterErrorLimitExceeded, "场景对象数量超限", 409, nil)
		}
	}
	for _, o := range objects {
		if err := validateObjectInput(theaterDesignObjectInput(o)); err != nil {
			return err
		}
		if err := validateTheaterDesignParent(snapshot, o); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(raw) > theaterMaxSnapshotBytes {
		return newTheaterError(TheaterErrorLimitExceeded, "Theater snapshot 超过上限", 413, nil)
	}
	counts := map[string]int64{}
	countResourceIDsInJSON(string(raw), counts)
	for id := range counts {
		var resource model.TheaterResourceModel
		if err := tx.Where("room_id = ? AND id = ?", room.ID, id).Limit(1).Find(&resource).Error; err != nil {
			return err
		}
		if resource.ID == "" || (resource.Status != "ready" && !(resource.Status == "deleting" && resource.CleanupReason == theaterResourceCleanupOrphan)) {
			return newTheaterError(TheaterErrorResourceNotReady, "resource 不属于房间或未 ready", 409, map[string]any{"resourceId": id})
		}
	}
	if err := validateTheaterDesignAudioReferences(tx, room, snapshot); err != nil {
		return err
	}
	return validateTheaterDesignReferences(snapshot, objects)
}

func ValidateTheaterMCPObjectFields(actorID string, scope TheaterScope, fields TheaterMCPObjectFields, cfg utils.AppConfig) error {
	if fields.Content == nil || fields.Content.Iframe == nil {
		return nil
	}
	frame := fields.Content.Iframe
	if !theaterFinite(frame.Scale) || frame.Scale < 0.25 || frame.Scale > 5 {
		return theaterPayloadError("iframe.scale 范围 0.25..5")
	}
	return ValidateTheaterMCPIframeURL(actorID, scope, frame.URL, cfg)
}

// The iframe boundary is shared with the first/second-phase small-edit tools.
func ValidateTheaterMCPIframeURL(actorID string, scope TheaterScope, source string, cfg utils.AppConfig) error {
	u, err := url.Parse(strings.TrimSpace(source))
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || len(source) > 8192 {
		return theaterPayloadError("iframe URL 必须为 HTTP/HTTPS")
	}
	if !strings.HasPrefix(u.Fragment, "/internal/iform/") {
		return nil
	}
	if !IsTheaterInternalIFormURL(cfg, u) {
		return theaterPayloadError("内部 iForm URL 必须使用已配置可信域名和 webUrl 路径")
	}
	fragment, err := url.Parse(u.Fragment)
	if err != nil {
		return theaterPayloadError("iForm URL 无效")
	}
	id, err := url.PathUnescape(strings.TrimPrefix(fragment.Path, "/internal/iform/"))
	if err != nil || id == "" || strings.Contains(id, "/") {
		return theaterPayloadError("iForm ID 无效")
	}
	world, channel := fragment.Query().Get("world"), fragment.Query().Get("channel")
	if world != scope.WorldID || channel == "" || channel != scope.InputChannelID || (scope.ScopeType == "channel" && channel != scope.ChannelID) {
		return theaterPayloadError("iForm 必须匹配 worldId/inputChannelId/channelId")
	}
	_, ch, err := resolveTheaterScope(world, channel)
	if err != nil {
		return err
	}
	if ch == nil || !CanReadChannelByUserId(actorID, channel) {
		return newTheaterError(TheaterErrorPermissionDenied, "无法访问 iForm 频道", 403, nil)
	}
	forms, err := ListEffectiveChannelIForms(channel)
	if err != nil {
		return err
	}
	for _, form := range forms {
		if form.ID == id {
			return nil
		}
	}
	return newTheaterError(TheaterErrorNotFound, "iForm 不存在或不可见", 404, nil)
}
