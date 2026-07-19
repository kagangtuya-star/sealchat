package service

import (
	"encoding/json"
	"sort"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

// MergeAllTheaterRoomsToWorld folds current channel Theater state into one
// world room. It is intentionally one-shot and idempotent for development data.
func MergeAllTheaterRoomsToWorld() error {
	db := model.GetDB()
	if db == nil {
		return nil
	}
	var worldIDs []string
	if err := db.Model(&model.WorldModel{}).Where("status = ?", "active").Order("id ASC").Pluck("id", &worldIDs).Error; err != nil {
		return err
	}
	for _, worldID := range worldIDs {
		if err := MergeTheaterRoomsToWorld(worldID); err != nil {
			return err
		}
	}
	return nil
}

// MergeTheaterRoomsToWorld keeps current state only. Old channel event history
// is deliberately discarded because revisions from independent rooms cannot be
// concatenated into one valid sequence.
func MergeTheaterRoomsToWorld(worldID string) error {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil
	}
	db := model.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var existing model.TheaterRoomModel
		if err := tx.Where("world_id = ? AND channel_id = ?", worldID, "").First(&existing).Error; err == nil && existing.ID != "" {
			return nil
		} else if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		var rooms []model.TheaterRoomModel
		if err := tx.Where("world_id = ? AND channel_id <> ?", worldID, "").Order("updated_at DESC, id ASC").Find(&rooms).Error; err != nil {
			return err
		}
		if len(rooms) == 0 {
			return nil
		}
		worldRoom := &model.TheaterRoomModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			WorldID:           worldID, ChannelID: "", ScopeType: model.TheaterScopeWorld,
			SchemaVersion: model.TheaterSchemaVersion, Status: "active", StateJSON: "{}",
			CreatedBy: "system", UpdatedBy: "system",
		}
		if err := tx.Create(worldRoom).Error; err != nil {
			return err
		}
		merged := TheaterSharedSnapshot{
			Scenes:            map[string]TheaterSceneSnapshot{},
			PersistentObjects: map[string]TheaterObjectSnapshot{},
			Characters:        map[string]TheaterObjectSnapshot{},
			Resources:         map[string]TheaterResourcePublic{},
		}
		usedScenes := map[string]struct{}{}
		usedObjects := map[string]struct{}{}
		primaryActive := ""
		nextSceneOrder := int64(0)
		for roomIndex := range rooms {
			room := &rooms[roomIndex]
			snapshot, _, err := buildTheaterSnapshot(tx, room, true)
			if err != nil {
				return err
			}
			sceneIDs := make(map[string]string, len(snapshot.Scenes))
			keys := make([]string, 0, len(snapshot.Scenes))
			for id := range snapshot.Scenes {
				keys = append(keys, id)
			}
			sort.Slice(keys, func(left, right int) bool {
				leftScene := snapshot.Scenes[keys[left]]
				rightScene := snapshot.Scenes[keys[right]]
				if leftScene.Order != rightScene.Order {
					return leftScene.Order < rightScene.Order
				}
				return keys[left] < keys[right]
			})
			for _, oldID := range keys {
				newID := oldID
				if _, exists := usedScenes[newID]; exists {
					newID = utils.NewID()
				}
				sceneIDs[oldID] = newID
				usedScenes[newID] = struct{}{}
				scene := snapshot.Scenes[oldID]
				scene.ID = newID
				scene.Order = nextSceneOrder
				nextSceneOrder++
				merged.Scenes[newID] = scene
			}
			if roomIndex == 0 && snapshot.ActiveSceneID != nil {
				primaryActive = sceneIDs[*snapshot.ActiveSceneID]
			}
			if roomIndex == 0 {
				merged.LiveState = snapshot.LiveState
			}
			objectIDs := make(map[string]string, len(snapshot.PersistentObjects))
			reserveObjectID := func(oldID string) {
				if _, exists := objectIDs[oldID]; exists {
					return
				}
				newID := oldID
				if _, exists := usedObjects[newID]; exists {
					newID = utils.NewID()
				}
				objectIDs[oldID] = newID
				usedObjects[newID] = struct{}{}
			}
			for _, oldSceneID := range keys {
				objectKeys := make([]string, 0, len(snapshot.Scenes[oldSceneID].Objects))
				for id := range snapshot.Scenes[oldSceneID].Objects {
					objectKeys = append(objectKeys, id)
				}
				sort.Strings(objectKeys)
				for _, id := range objectKeys {
					reserveObjectID(id)
				}
			}
			persistentKeys := make([]string, 0, len(snapshot.PersistentObjects))
			for id := range snapshot.PersistentObjects {
				persistentKeys = append(persistentKeys, id)
			}
			sort.Strings(persistentKeys)
			for _, id := range persistentKeys {
				reserveObjectID(id)
			}
			remapObject := func(object TheaterObjectSnapshot) TheaterObjectSnapshot {
				object.ID = objectIDs[object.ID]
				if object.SceneID != nil {
					if mapped := sceneIDs[*object.SceneID]; mapped != "" {
						object.SceneID = &mapped
					}
				}
				if object.ParentID != nil {
					if mapped := objectIDs[*object.ParentID]; mapped != "" {
						object.ParentID = &mapped
					}
				}
				object.Actions = remapTheaterActionReferences(object.Actions, sceneIDs, objectIDs)
				return object
			}
			for _, oldSceneID := range keys {
				scene := snapshot.Scenes[oldSceneID]
				mergedScene := merged.Scenes[sceneIDs[oldSceneID]]
				objectKeys := make([]string, 0, len(scene.Objects))
				for id := range scene.Objects {
					objectKeys = append(objectKeys, id)
				}
				sort.Strings(objectKeys)
				mergedScene.Objects = map[string]TheaterObjectSnapshot{}
				for _, id := range objectKeys {
					object := remapObject(scene.Objects[id])
					mergedScene.Objects[object.ID] = object
					if object.Kind == "character" {
						merged.Characters[object.ID] = object
					}
				}
				merged.Scenes[sceneIDs[oldSceneID]] = mergedScene
			}
			for _, id := range persistentKeys {
				object := remapObject(snapshot.PersistentObjects[id])
				merged.PersistentObjects[object.ID] = object
				if object.Kind == "character" {
					merged.Characters[object.ID] = object
				}
			}
			for id, resource := range snapshot.Resources {
				if resource.Status == "ready" {
					merged.Resources[id] = resource
				}
			}
		}
		if primaryActive != "" {
			if _, ok := merged.Scenes[primaryActive]; ok {
				merged.ActiveSceneID = &primaryActive
			}
		}
		if merged.ActiveSceneID == nil {
			keys := make([]string, 0, len(merged.Scenes))
			for id := range merged.Scenes {
				keys = append(keys, id)
			}
			sort.Slice(keys, func(left, right int) bool {
				leftScene := merged.Scenes[keys[left]]
				rightScene := merged.Scenes[keys[right]]
				if leftScene.Order != rightScene.Order {
					return leftScene.Order < rightScene.Order
				}
				return keys[left] < keys[right]
			})
			if len(keys) > 0 {
				merged.ActiveSceneID = &keys[0]
			}
		}
		if len(merged.LiveState) == 0 {
			merged.LiveState = json.RawMessage(`{}`)
		}
		for _, room := range rooms {
			if err := tx.Model(&model.TheaterResourceModel{}).Where("room_id = ?", room.ID).Update("room_id", worldRoom.ID).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.AttachmentModel{}).Where("root_id = ? AND root_id_type = ?", room.ID, "theater_resource").Update("root_id", worldRoom.ID).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("room_id = ?", room.ID).Delete(&model.TheaterObjectModel{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("room_id = ?", room.ID).Delete(&model.TheaterSceneModel{}).Error; err != nil {
				return err
			}
		}
		if err := replaceTheaterRows(tx, worldRoom, "system", merged); err != nil {
			return err
		}
		_, checksum, err := buildTheaterSnapshot(tx, worldRoom, true)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.TheaterRoomModel{}).Where("id = ?", worldRoom.ID).Updates(map[string]any{"state_hash": checksum, "revision": 0, "scope_type": model.TheaterScopeWorld}).Error; err != nil {
			return err
		}
		for _, room := range rooms {
			if err := tx.Unscoped().Where("room_id = ?", room.ID).Delete(&model.TheaterObjectModel{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("room_id = ?", room.ID).Delete(&model.TheaterSceneModel{}).Error; err != nil {
				return err
			}
			var snapshotIDs []string
			if err := tx.Model(&model.TheaterSnapshotModel{}).Where("room_id = ?", room.ID).Pluck("id", &snapshotIDs).Error; err != nil {
				return err
			}
			if err := deleteTheaterResourceHoldsForSnapshots(tx, snapshotIDs); err != nil {
				return err
			}
			if err := tx.Unscoped().Where("room_id = ?", room.ID).Delete(&model.TheaterSnapshotModel{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("room_id = ?", room.ID).Delete(&model.TheaterMutationModel{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("room_id = ?", room.ID).Delete(&model.TheaterAuditLogModel{}).Error; err != nil {
				return err
			}
			if err := tx.Unscoped().Where("id = ?", room.ID).Delete(&model.TheaterRoomModel{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func remapTheaterActionReferences(raw json.RawMessage, sceneIDs, objectIDs map[string]string) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var actions []map[string]any
	if err := json.Unmarshal(raw, &actions); err != nil {
		return raw
	}
	for _, action := range actions {
		payload, ok := action["payload"].(map[string]any)
		if !ok {
			continue
		}
		if oldID, ok := payload["sceneId"].(string); ok {
			if newID := sceneIDs[oldID]; newID != "" {
				payload["sceneId"] = newID
			}
		}
		if oldID, ok := payload["objectId"].(string); ok {
			if newID := objectIDs[oldID]; newID != "" {
				payload["objectId"] = newID
			}
		}
	}
	value, err := json.Marshal(actions)
	if err != nil {
		return raw
	}
	return value
}
