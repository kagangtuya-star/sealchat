package service

import (
	"bytes"
	"encoding/json"
	"sort"

	"gorm.io/gorm"

	"sealchat/model"
)

// Theater scene state is a versioned wire object. Keep this allowlist in one
// place so validation, read normalization, and data repair have identical
// behavior. New protocol fields must be added here and to the corresponding
// typed client state in the same change.
var theaterSceneStateAllowedKeys = map[string]struct{}{
	"background":    {},
	"foreground":    {},
	"surfaceStyles": {},
	"fieldWidth":    {},
	"fieldHeight":   {},
	// Grid options, including the newer onTop flag, stay nested here.
	"grid":          {},
	"transition":    {},
	"switchAudio":   {},
	"musicSnapshot": {},
	"sceneOverlays": {},
	"sceneFolders":  {},
	"resources":     {},
	"ccfolia":       {},
}

func isTheaterSceneStateKeyAllowed(key string) bool {
	_, ok := theaterSceneStateAllowedKeys[key]
	return ok
}

func canonicalizeTheaterSceneStateJSON(raw []byte) ([]byte, []string, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return []byte(`{}`), nil, true, nil
	}
	var state map[string]any
	if err := json.Unmarshal(trimmed, &state); err != nil {
		return nil, nil, false, err
	}
	if state == nil {
		state = map[string]any{}
	}
	canonicalState := make(map[string]any, len(state))
	removed := make([]string, 0)
	for key, value := range state {
		if !isTheaterSceneStateKeyAllowed(key) {
			removed = append(removed, key)
			continue
		}
		canonicalState[key] = value
	}
	sort.Strings(removed)
	canonical, err := json.Marshal(canonicalState)
	if err != nil {
		return nil, nil, false, err
	}
	return canonical, removed, !bytes.Equal(trimmed, canonical), nil
}

func normalizedTheaterSceneStateJSON(value string) json.RawMessage {
	canonical, _, _, err := canonicalizeTheaterSceneStateJSON([]byte(value))
	if err != nil {
		// Scene state must be an object. Keep malformed rows recoverable in
		// snapshots without re-emitting a valid-but-unsupported array/scalar.
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(canonical)
}

func canonicalizeTheaterSharedSnapshot(snapshot *TheaterSharedSnapshot) (int, bool, error) {
	if snapshot == nil {
		return 0, false, nil
	}
	removedCount := 0
	changed := false
	normalize := func(raw *json.RawMessage) (bool, error) {
		canonical, removed, didChange, err := canonicalizeTheaterSceneStateJSON(*raw)
		if err != nil {
			return false, err
		}
		if didChange {
			*raw = json.RawMessage(canonical)
		}
		removedCount += len(removed)
		return didChange, nil
	}
	liveChanged, err := normalize(&snapshot.LiveState)
	if err != nil {
		return removedCount, changed, err
	}
	changed = liveChanged
	for sceneID, scene := range snapshot.Scenes {
		sceneChanged, err := normalize(&scene.State)
		if err != nil {
			return removedCount, changed, err
		}
		if sceneChanged {
			snapshot.Scenes[sceneID] = scene
			changed = true
		}
	}
	return removedCount, changed, nil
}

// TheaterStateRepairSummary describes the idempotent startup cleanup of legacy
// state JSON. Invalid JSON is left untouched and counted for operator review.
type TheaterStateRepairSummary struct {
	Rooms         int
	Scenes        int
	Snapshots     int
	FieldsRemoved int
	InvalidJSON   int
}

// RepairTheaterStateJSON removes unsupported top-level scene-state keys from
// persisted Theater rows and checkpoint snapshots. It runs in one transaction
// and leaves malformed JSON untouched so operators can recover it explicitly.
func RepairTheaterStateJSON() (summary TheaterStateRepairSummary, err error) {
	db := model.GetDB()
	if db == nil {
		return summary, nil
	}
	// CLI/config-only database initialization may not create Theater tables.
	// Treat that partial schema as a no-op; full startup runs after Theater
	// migrations and performs the repair below.
	if !db.Migrator().HasTable(&model.TheaterRoomModel{}) {
		return summary, nil
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var rooms []model.TheaterRoomModel
		if err := tx.Find(&rooms).Error; err != nil {
			return err
		}
		roomsByID := make(map[string]*model.TheaterRoomModel, len(rooms))
		dirtyRooms := make(map[string]struct{})
		for index := range rooms {
			room := &rooms[index]
			roomsByID[room.ID] = room
			canonical, removed, changed, err := canonicalizeTheaterSceneStateJSON([]byte(room.StateJSON))
			if err != nil {
				summary.InvalidJSON++
				continue
			}
			if !changed {
				continue
			}
			room.StateJSON = string(canonical)
			if err := tx.Model(&model.TheaterRoomModel{}).Where("id = ?", room.ID).Update("state_json", room.StateJSON).Error; err != nil {
				return err
			}
			dirtyRooms[room.ID] = struct{}{}
			summary.Rooms++
			summary.FieldsRemoved += len(removed)
		}

		var scenes []model.TheaterSceneModel
		if err := tx.Find(&scenes).Error; err != nil {
			return err
		}
		for index := range scenes {
			scene := &scenes[index]
			canonical, removed, changed, err := canonicalizeTheaterSceneStateJSON([]byte(scene.StateJSON))
			if err != nil {
				summary.InvalidJSON++
				continue
			}
			if !changed {
				continue
			}
			scene.StateJSON = string(canonical)
			if err := tx.Model(&model.TheaterSceneModel{}).Where("id = ?", scene.ID).Update("state_json", scene.StateJSON).Error; err != nil {
				return err
			}
			dirtyRooms[scene.RoomID] = struct{}{}
			summary.Scenes++
			summary.FieldsRemoved += len(removed)
		}

		for roomID := range dirtyRooms {
			room := roomsByID[roomID]
			if room == nil {
				continue
			}
			_, checksum, err := buildTheaterSnapshot(tx, room, true)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.TheaterRoomModel{}).Where("id = ?", roomID).Update("state_hash", checksum).Error; err != nil {
				return err
			}
		}

		var snapshots []model.TheaterSnapshotModel
		if err := tx.FindInBatches(&snapshots, 50, func(batchTx *gorm.DB, _ int) error {
			for index := range snapshots {
				item := &snapshots[index]
				var snapshot TheaterSharedSnapshot
				if err := json.Unmarshal([]byte(item.SnapshotJSON), &snapshot); err != nil {
					summary.InvalidJSON++
					continue
				}
				removed, changed, err := canonicalizeTheaterSharedSnapshot(&snapshot)
				if err != nil {
					summary.InvalidJSON++
					continue
				}
				if !changed {
					continue
				}
				raw, hash, err := canonicalTheaterJSON(snapshot)
				if err != nil {
					return err
				}
				updates := map[string]any{
					"snapshot_json":  string(raw),
					"snapshot_hash":  hash,
					"snapshot_bytes": len(raw),
				}
				if err := batchTx.Model(&model.TheaterSnapshotModel{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
					return err
				}
				summary.Snapshots++
				summary.FieldsRemoved += removed
			}
			return nil
		}).Error; err != nil {
			return err
		}
		return nil
	})
	return summary, err
}
