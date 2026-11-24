package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
)

const WorldPresentationMaxBytes = 2 << 20

var (
	ErrWorldPresentationInvalid  = errors.New("演出配置无效或超过大小限制")
	ErrWorldPresentationConflict = errors.New("演出配置已由其他管理员修改，请重新加载")
)

const emptyWorldPresentation = `{"version":1,"effects":[],"sequences":[]}`

type WorldPresentationSettings struct {
	Config   json.RawMessage `json:"config"`
	Revision string          `json:"revision"`
}

type worldPresentationIndex struct {
	Version int `json:"version"`
	Effects []struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Visible *bool  `json:"visible"`
		Content struct {
			Effect struct {
				Web struct{ HTML string } `json:"web"`
			} `json:"effect"`
		} `json:"content"`
	} `json:"effects"`
	Sequences []struct {
		ID      string `json:"id"`
		Enabled *bool  `json:"enabled"`
		Steps   []struct {
			SceneID string `json:"sceneId"`
			Action  struct {
				Type    string `json:"type"`
				Payload struct {
					EffectID string `json:"effectId"`
				} `json:"payload"`
			} `json:"action"`
		} `json:"steps"`
	} `json:"sequences"`
}

func worldPresentationSettings(raw string) WorldPresentationSettings {
	if !json.Valid([]byte(raw)) {
		raw = emptyWorldPresentation
	}
	sum := sha256.Sum256([]byte(raw))
	return WorldPresentationSettings{Config: json.RawMessage(raw), Revision: hex.EncodeToString(sum[:])}
}

func ReadWorldPresentation(worldID string) (WorldPresentationSettings, error) {
	var world model.WorldModel
	err := model.GetDB().Select("world_presentation_json").Where("id = ? AND status = ?", worldID, "active").Take(&world).Error
	return worldPresentationSettings(world.WorldPresentationJSON), err
}

func SaveWorldPresentation(worldID, actorID, revision string, input json.RawMessage) (WorldPresentationSettings, error) {
	if !IsWorldAdmin(worldID, actorID) {
		return WorldPresentationSettings{}, ErrWorldPermission
	}
	var index worldPresentationIndex
	if len(input) > WorldPresentationMaxBytes || json.Unmarshal(input, &index) != nil ||
		index.Version != 1 || len(index.Effects) > 64 || len(index.Sequences) > 64 {
		return WorldPresentationSettings{}, ErrWorldPresentationInvalid
	}
	for _, effect := range index.Effects {
		if effect.Type != "effect" || strings.TrimSpace(effect.ID) == "" || len(effect.Content.Effect.Web.HTML) > 128<<10 {
			return WorldPresentationSettings{}, ErrWorldPresentationInvalid
		}
	}

	var world model.WorldModel
	db := model.GetDB().Model(&model.WorldModel{}).Where("id = ? AND status = ?", worldID, "active")
	if err := db.Select("world_presentation_json").Take(&world).Error; err != nil {
		return WorldPresentationSettings{}, err
	}
	if worldPresentationSettings(world.WorldPresentationJSON).Revision != revision {
		return WorldPresentationSettings{}, ErrWorldPresentationConflict
	}
	next := string(input)
	if next != world.WorldPresentationJSON {
		if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&model.WorldModel{}).
				Where("id = ? AND status = ? AND COALESCE(world_presentation_json, '') = ?", worldID, "active", world.WorldPresentationJSON).
				UpdateColumn("world_presentation_json", next)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrWorldPresentationConflict
			}
			var room model.TheaterRoomModel
			if err := tx.Where("world_id = ? AND channel_id = ?", worldID, "").Limit(1).Find(&room).Error; err != nil {
				return err
			}
			if room.ID != "" {
				return recalculateTheaterResourceReferences(tx, room.ID)
			}
			return nil
		}); err != nil {
			return WorldPresentationSettings{}, err
		}
	}
	return worldPresentationSettings(next), nil
}

// Only configured, enabled effects and sequences with world targets can be broadcast.
func (s WorldPresentationSettings) CanPlay(kind, id string) bool {
	var index worldPresentationIndex
	if json.Unmarshal(s.Config, &index) != nil {
		return false
	}
	canPlayEffect := func(id string) bool {
		for _, effect := range index.Effects {
			if effect.ID == id && effect.Type == "effect" {
				return effect.Visible == nil || *effect.Visible
			}
		}
		return false
	}
	switch kind {
	case "effect":
		return canPlayEffect(id)
	case "sequence":
		for _, sequence := range index.Sequences {
			if sequence.ID != id || (sequence.Enabled != nil && !*sequence.Enabled) {
				continue
			}
			for _, step := range sequence.Steps {
				if step.SceneID == "" && step.Action.Type == "effect.play" && canPlayEffect(step.Action.Payload.EffectID) {
					return true
				}
			}
			return false
		}
	}
	return false
}
