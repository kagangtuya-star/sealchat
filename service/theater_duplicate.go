package service

import (
	"encoding/json"
	"sort"
	"strings"

	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/utils"
)

type theaterSceneDuplicatePayload struct {
	SceneID string `json:"sceneId"`
	result  *TheaterDuplicateResult
}
type theaterObjectDuplicatePayload struct {
	ObjectID string  `json:"objectId"`
	OffsetX  float64 `json:"offsetX,omitempty"`
	OffsetY  float64 `json:"offsetY,omitempty"`
	result   *TheaterDuplicateResult
}
type TheaterDuplicateResult struct {
	SceneID     string            `json:"sceneId,omitempty"`
	ObjectID    string            `json:"objectId,omitempty"`
	ObjectIDMap map[string]string `json:"objectIdMap"`
	ActionIDMap map[string]string `json:"actionIdMap"`
}

// This narrow reference lookup is also used by package remapping. A same-room
// clone preserves references outside the map; package import decides separately
// whether an unmapped reference needs to be cleared.
func remapTheaterLocalEntityReference(field, id string, remap theaterPackageRemap) string {
	if field == "sceneId" {
		return remap.scenes[id]
	}
	if field == "objectId" || field == "parentId" || field == "effectId" {
		return remap.objects[id]
	}
	return ""
}

type theaterCloneRemap struct {
	entities theaterPackageRemap
	actions  map[string]string
}

func (r theaterCloneRemap) id(container map[string]any) {
	if id, ok := container["id"].(string); ok && id != "" {
		if r.actions[id] == "" {
			r.actions[id] = utils.NewID()
		}
		container["id"] = r.actions[id]
	}
}
func (r theaterCloneRemap) reference(container map[string]any, field, id string) error {
	if value := remapTheaterLocalEntityReference(field, id, r.entities); value != "" {
		container[field] = value
	}
	return nil
}
func (r theaterCloneRemap) sceneState(raw json.RawMessage) (json.RawMessage, error) {
	state := map[string]any{}
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	sequences, _ := state["theaterSequences"].([]any)
	for _, raw := range sequences {
		seq, _ := raw.(map[string]any)
		if err := walkTheaterCloneSequence(seq, r.id, r.reference); err != nil {
			return nil, err
		}
	}
	return json.Marshal(state)
}
func (r theaterCloneRemap) object(object TheaterObjectSnapshot, sceneID *string) (TheaterObjectSnapshot, error) {
	oldID := object.ID
	object.ID = r.entities.objects[oldID]
	object.SceneID = sceneID
	if parent := derefString(object.ParentID); parent != "" {
		if mapped := r.entities.objects[parent]; mapped != "" {
			object.ParentID = &mapped
		}
	}
	var actions []any
	if err := json.Unmarshal(object.Actions, &actions); err != nil {
		return object, err
	}
	if err := walkTheaterCloneActions(actions, r.id, r.reference); err != nil {
		return object, err
	}
	object.Actions, _ = json.Marshal(actions)
	metadata := map[string]any{}
	if err := json.Unmarshal(object.Metadata, &metadata); err != nil {
		return object, err
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	if len(r.entities.scenes) > 0 {
		// Scene copies retain the source transition identity so corresponding
		// objects can animate between scenes, matching StageStore.duplicateScene.
		key, _ := metadata["transitionKey"].(string)
		if key = strings.TrimSpace(key); key == "" {
			key = oldID
		}
		metadata["transitionKey"] = key
	} else {
		// Same-scene clipboard copies need their own transition identity.
		metadata["transitionKey"] = object.ID
	}
	bindings, _ := metadata["embedEventBindings"].([]any)
	for _, raw := range bindings {
		binding, _ := raw.(map[string]any)
		ids, _ := binding["actionIds"].([]any)
		for i, rawID := range ids {
			if id, ok := rawID.(string); ok && r.actions[id] != "" {
				ids[i] = r.actions[id]
			}
		}
	}
	object.Metadata, _ = json.Marshal(metadata)
	return object, nil
}

func applyTheaterSceneDuplicate(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, p *theaterSceneDuplicatePayload) error {
	snapshot, _, err := readTheaterSnapshot(tx, room, false, false)
	if err != nil {
		return err
	}
	scene, ok := snapshot.Scenes[p.SceneID]
	if !ok {
		return newTheaterError(TheaterErrorNotFound, "场景不存在", 404, nil)
	}
	if err = theaterDesignSceneUnlocked(scene); err != nil {
		return err
	}
	if len(scene.Objects) > theaterMaxSceneObjects {
		return newTheaterError(TheaterErrorLimitExceeded, "duplicate 对象数量超限", 409, nil)
	}
	newID := utils.NewID()
	remap := theaterCloneRemap{entities: theaterPackageRemap{scenes: map[string]string{scene.ID: newID}, objects: map[string]string{}}, actions: map[string]string{}}
	for id := range scene.Objects {
		remap.entities.objects[id] = utils.NewID()
	}
	state, err := remap.sceneState(scene.State)
	if err != nil {
		return err
	}
	objects := map[string]TheaterObjectSnapshot{}
	for _, id := range theaterCloneSortedIDs(scene.Objects) {
		object, err := remap.object(scene.Objects[id], &newID)
		if err != nil {
			return err
		}
		objects[object.ID] = object
	}
	order := int64(0)
	for _, s := range snapshot.Scenes {
		if s.Order >= order {
			order = s.Order + 1
		}
	}
	name := theaterCloneName(scene.Name)
	copy := scene
	copy.ID = newID
	copy.Name = name
	copy.Order = order
	copy.Published = false
	copy.State = state
	copy.Objects = objects
	snapshot.Scenes[newID] = copy
	for id, o := range objects {
		if o.Kind == "character" {
			snapshot.Characters[id] = o
		}
	}
	if err = validateTheaterDesignDocument(tx, room, snapshot); err != nil {
		return err
	}
	var stateMap map[string]any
	_ = json.Unmarshal(state, &stateMap)
	create := &theaterSceneCreatePayload{SceneID: newID, Name: name, SwitchText: copy.SwitchText, Order: order, FolderID: copy.FolderID, State: stateMap}
	if err = validateDecodedTheaterPayload(TheaterMutationSceneCreate, create); err != nil {
		return err
	}
	if err = applyTheaterSceneCreate(tx, room, actorID, create); err != nil {
		return err
	}
	if err = applyTheaterSceneUpdate(tx, room, actorID, &theaterSceneUpdatePayload{newID, map[string]any{"locked": copy.Locked, "published": copy.Published}}); err != nil {
		return err
	}
	if err = createTheaterCloneObjects(tx, room, actorID, objects); err != nil {
		return err
	}
	p.result = &TheaterDuplicateResult{SceneID: newID, ObjectIDMap: remap.entities.objects, ActionIDMap: remap.actions}
	return nil
}

func applyTheaterObjectDuplicate(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, p *theaterObjectDuplicatePayload) error {
	snapshot, _, err := readTheaterSnapshot(tx, room, false, false)
	if err != nil {
		return err
	}
	root, ok := theaterDesignFindObject(snapshot, p.ObjectID)
	if !ok {
		return newTheaterError(TheaterErrorNotFound, "对象不存在", 404, nil)
	}
	all := theaterDesignAllObjects(snapshot)
	ids := []string{root.ID}
	seen := map[string]bool{root.ID: true}
	for index := 0; index < len(ids); index++ {
		for _, id := range theaterCloneSortedIDs(all) {
			if derefString(all[id].ParentID) == ids[index] {
				if seen[id] {
					return theaterPayloadError("parent 循环")
				}
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) > theaterMaxSceneObjects {
			return newTheaterError(TheaterErrorLimitExceeded, "duplicate subtree 对象数量超限", 409, nil)
		}
	}
	remap := theaterCloneRemap{entities: theaterPackageRemap{objects: map[string]string{}}, actions: map[string]string{}}
	for _, id := range ids {
		if err = theaterDesignObjectUnlocked(snapshot, all[id]); err != nil {
			return err
		}
		remap.entities.objects[id] = utils.NewID()
	}
	objects := map[string]TheaterObjectSnapshot{}
	for _, id := range ids {
		copy, err := remap.object(all[id], root.SceneID)
		if err != nil {
			return err
		}
		if id == root.ID {
			copy.Name = theaterCloneName(copy.Name)
			copy.X += p.OffsetX
			copy.Y += p.OffsetY
		}
		objects[copy.ID] = copy
		theaterDesignPutObject(&snapshot, copy)
	}
	if err = validateTheaterDesignDocument(tx, room, snapshot); err != nil {
		return err
	}
	if err = createTheaterCloneObjects(tx, room, actorID, objects); err != nil {
		return err
	}
	p.result = &TheaterDuplicateResult{ObjectID: remap.entities.objects[root.ID], ObjectIDMap: remap.entities.objects, ActionIDMap: remap.actions}
	return nil
}

func theaterCloneName(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > 509 {
		runes = runes[:509]
	}
	return string(runes) + " 副本"
}
func theaterCloneSortedIDs(objects map[string]TheaterObjectSnapshot) []string {
	ids := make([]string, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
func createTheaterCloneObjects(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, objects map[string]TheaterObjectSnapshot) error {
	pending := map[string]TheaterObjectSnapshot{}
	for id, o := range objects {
		pending[id] = o
	}
	for len(pending) > 0 {
		progress := false
		for _, id := range theaterCloneSortedIDs(pending) {
			o := pending[id]
			if _, wait := pending[derefString(o.ParentID)]; wait {
				continue
			}
			input := theaterDesignObjectInput(o)
			if err := validateObjectInput(input); err != nil {
				return err
			}
			if err := createTheaterObject(tx, room, actorID, o.SceneID, input); err != nil {
				return err
			}
			delete(pending, id)
			progress = true
		}
		if !progress {
			return theaterPayloadError("clone parent 循环")
		}
	}
	return nil
}
