package service

import (
	"encoding/json"
	"gorm.io/gorm"
	"sealchat/model"
)

func validateTheaterDesignAudioReferences(tx *gorm.DB, room *model.TheaterRoomModel, snapshot TheaterSharedSnapshot) error {
	ids := theaterMusicSnapshotAssetIDs(snapshot)
	add := func(value any) {
		for id := range collectJSONFieldStrings(value, "assetId") {
			ids[id] = struct{}{}
		}
	}
	for _, scene := range snapshot.Scenes {
		var state map[string]any
		_ = json.Unmarshal(scene.State, &state)
		add(state["switchAudio"])
	}
	for _, object := range theaterDesignAllObjects(snapshot) {
		if object.Kind != "effect" {
			continue
		}
		var content map[string]any
		_ = json.Unmarshal(object.Content, &content)
		if effect, ok := content["effect"].(map[string]any); ok {
			add(effect["audio"])
		}
	}
	for id := range ids {
		var asset model.AudioAsset
		if err := tx.Where("id = ?", id).Limit(1).Find(&asset).Error; err != nil {
			return err
		}
		allowed := asset.Scope == model.AudioScopeWorld && asset.WorldID != nil && *asset.WorldID == room.WorldID
		if allowed && room.ChannelID != "" && hasAudioTag(&asset, TheaterFeatureAudioTag) {
			allowed = hasAudioTag(&asset, theaterChannelAudioTag(room.ChannelID))
		}
		if asset.Scope == model.AudioScopeCommon && string(asset.Visibility) == "public" {
			allowed = true
		}
		if asset.ID == "" || !allowed || string(asset.TranscodeStatus) != "ready" {
			return newTheaterError(TheaterErrorResourceNotReady, "audio asset 不属于可用范围或未 ready", 409, map[string]any{"assetId": id})
		}
	}
	return nil
}
