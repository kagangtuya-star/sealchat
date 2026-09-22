package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"sealchat/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrWorldGlassInvalid = errors.New("invalid world glass input")
	ErrWorldGlassDenied  = errors.New("world glass access denied")
	ErrWorldGlassLimit   = errors.New("world glass preset limit reached")
	glassColorPattern    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

type WorldGlassSettings struct {
	Version              int     `json:"version"`
	Mode                 string  `json:"mode"`
	BackgroundOpacity    float64 `json:"backgroundOpacity"`
	BackgroundBlur       float64 `json:"backgroundBlur"`
	BackgroundBrightness float64 `json:"backgroundBrightness"`
	SurfaceOpacity       float64 `json:"surfaceOpacity"`
	GlassBlur            float64 `json:"glassBlur"`
	Saturation           float64 `json:"saturation"`
	OverlayColor         string  `json:"overlayColor"`
	OverlayOpacity       float64 `json:"overlayOpacity"`
}

func normalizeWorldGlassSettings(raw json.RawMessage) (WorldGlassSettings, error) {
	s := WorldGlassSettings{Version: 1, Mode: "cover", BackgroundOpacity: 100, BackgroundBrightness: 95, SurfaceOpacity: 55, GlassBlur: 12, Saturation: 110, OverlayColor: "#000000"}
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		return s, ErrWorldGlassInvalid
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, ErrWorldGlassInvalid
	}
	if s.Version != 1 || (s.Mode != "cover" && s.Mode != "contain" && s.Mode != "center" && s.Mode != "tile") {
		return s, ErrWorldGlassInvalid
	}
	if s.OverlayColor == "" {
		s.OverlayColor = "#000000"
	}
	if !glassColorPattern.MatchString(s.OverlayColor) {
		return s, ErrWorldGlassInvalid
	}
	for _, r := range []struct {
		p        *float64
		min, max float64
	}{
		{&s.BackgroundOpacity, 0, 100}, {&s.BackgroundBlur, 0, 40}, {&s.BackgroundBrightness, 20, 150},
		{&s.SurfaceOpacity, 35, 95}, {&s.GlassBlur, 0, 32}, {&s.Saturation, 50, 180}, {&s.OverlayOpacity, 0, 100},
	} {
		if math.IsNaN(*r.p) || math.IsInf(*r.p, 0) {
			return s, ErrWorldGlassInvalid
		}
		*r.p = math.Max(r.min, math.Min(r.max, *r.p))
	}
	return s, nil
}

type WorldGlassPreset struct {
	model.WorldGlassPresetModel
	Settings WorldGlassSettings `json:"settings"`
}

func worldGlassPresetView(p model.WorldGlassPresetModel) (WorldGlassPreset, error) {
	s, err := normalizeWorldGlassSettings(json.RawMessage(p.SettingsJSON))
	return WorldGlassPreset{WorldGlassPresetModel: p, Settings: s}, err
}

type WorldGlassTriggerContext struct {
	WorldID   string
	ChannelID string
}

type WorldGlassEffectiveState struct {
	model.WorldGlassStateModel
	ChannelID         string                    `json:"channelId"`
	Source            string                    `json:"source"`
	EffectivePresetID string                    `json:"effectivePresetId"`
	Preset            *WorldGlassPreset         `json:"preset"`
	MatchedTrigger    *WorldGlassMatchedTrigger `json:"matchedTrigger"`
	CanManage         bool                      `json:"canManage"`
}

type WorldGlassMatchedTrigger struct {
	ID          string `json:"id"`
	TriggerType string `json:"triggerType"`
}

// Uses world roles only: channel ownership does not grant world management.
func WorldGlassAccess(worldID, userID string, manage bool) (bool, error) {
	if strings.TrimSpace(userID) == "" {
		return false, ErrWorldGlassDenied
	}
	world, err := GetWorldByID(worldID)
	if err != nil {
		return false, err
	}
	if world.Status != "active" {
		return false, ErrWorldNotFound
	}
	canManage := world.OwnerID == userID || IsWorldAdmin(worldID, userID)
	if manage && !canManage {
		return false, ErrWorldGlassDenied
	}
	if !canManage && world.Visibility == model.WorldVisibilityPrivate && !IsWorldMember(worldID, userID) {
		return false, ErrWorldGlassDenied
	}
	return canManage, nil
}

func worldGlassCheckChannel(db *gorm.DB, worldID, channelID string) error {
	if channelID == "" {
		return nil
	}
	var count int64
	if err := db.Model(&model.ChannelModel{}).Where("id = ? AND world_id = ?", channelID, worldID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("%w: 频道不属于当前世界", ErrWorldGlassInvalid)
	}
	return nil
}

// Channel overrides never write the world default. New trigger kinds belong here.
func ResolveWorldGlassEffectivePreset(ctx WorldGlassTriggerContext) (*WorldGlassEffectiveState, error) {
	var out WorldGlassEffectiveState
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		state, err := model.WorldGlassStateGet(tx.Clauses(clause.Locking{Strength: "SHARE"}), ctx.WorldID)
		if err != nil {
			return err
		}
		out = WorldGlassEffectiveState{WorldGlassStateModel: state, ChannelID: ctx.ChannelID, Source: "none"}
		if err := worldGlassCheckChannel(tx, ctx.WorldID, ctx.ChannelID); err != nil {
			return err
		}
		if !state.Enabled {
			return nil
		}
		presetID := state.ActivePresetID
		if ctx.ChannelID != "" {
			var trigger model.WorldGlassTriggerModel
			if err := tx.Where("world_id = ? AND trigger_type = ? AND trigger_key = ? AND enabled = ?", ctx.WorldID, "channel", ctx.ChannelID, true).Find(&trigger).Error; err != nil {
				return err
			}
			if trigger.ID != "" {
				presetID = trigger.PresetID
				out.MatchedTrigger = &WorldGlassMatchedTrigger{ID: trigger.ID, TriggerType: trigger.TriggerType}
			}
		}
		if presetID == "" {
			return nil
		}
		var preset model.WorldGlassPresetModel
		if err := tx.Where("world_id = ? AND id = ?", ctx.WorldID, presetID).Take(&preset).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				out.MatchedTrigger = nil
				return nil
			}
			return err
		}
		view, err := worldGlassPresetView(preset)
		if err != nil {
			return err
		}
		out.Preset = &view
		out.EffectivePresetID = presetID
		out.Source = "default"
		if out.MatchedTrigger != nil {
			out.Source = "channel"
		}
		return nil
	})
	return &out, err
}

type WorldGlassManagement struct {
	Presets   []WorldGlassPreset             `json:"presets"`
	Triggers  []model.WorldGlassTriggerModel `json:"triggers"`
	State     model.WorldGlassStateModel     `json:"state"`
	CanManage bool                           `json:"canManage"`
}

func WorldGlassPresetList(worldID, userID string) (*WorldGlassManagement, error) {
	if _, err := WorldGlassAccess(worldID, userID, true); err != nil {
		return nil, err
	}
	out := &WorldGlassManagement{Presets: []WorldGlassPreset{}, CanManage: true}
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var err error
		out.State, err = model.WorldGlassStateGet(tx.Clauses(clause.Locking{Strength: "SHARE"}), worldID)
		if err != nil {
			return err
		}
		presets, err := model.WorldGlassPresetList(tx, worldID)
		if err != nil {
			return err
		}
		for _, p := range presets {
			view, err := worldGlassPresetView(p)
			if err != nil {
				return err
			}
			out.Presets = append(out.Presets, view)
		}
		out.Triggers, err = model.WorldGlassTriggerList(tx, worldID)
		return err
	})
	return out, err
}

// The atomic state update serializes mutations (including quota checks) across processes.
// All writes and the revision increment commit together; callers broadcast only after success.
func worldGlassMutate(worldID, userID string, mutate func(*gorm.DB, *model.WorldGlassStateModel) error) (uint64, error) {
	if _, err := WorldGlassAccess(worldID, userID, true); err != nil {
		return 0, err
	}
	var revision uint64
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.WorldGlassStateModel{WorldID: worldID}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.WorldGlassStateModel{}).Where("world_id = ?", worldID).Updates(map[string]any{"revision": gorm.Expr("revision + 1"), "updated_by": userID, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		state, err := model.WorldGlassStateGet(tx, worldID)
		if err != nil {
			return err
		}
		if err := mutate(tx, &state); err != nil {
			return err
		}
		revision = state.Revision
		return tx.Model(&model.WorldGlassStateModel{}).Where("world_id = ?", worldID).Updates(map[string]any{"enabled": state.Enabled, "active_preset_id": state.ActivePresetID}).Error
	})
	return revision, err
}

type WorldGlassPresetInput struct {
	Name         *string         `json:"name"`
	AttachmentID *string         `json:"attachmentId"`
	Settings     json.RawMessage `json:"settings"`
	SortOrder    *int            `json:"sortOrder"`
}

func WorldGlassPresetSave(worldID, userID, presetID string, in WorldGlassPresetInput) (*WorldGlassPreset, uint64, error) {
	var view WorldGlassPreset
	revision, err := worldGlassMutate(worldID, userID, func(tx *gorm.DB, state *model.WorldGlassStateModel) error {
		p := model.WorldGlassPresetModel{WorldID: worldID, CreatedBy: userID}
		if presetID != "" {
			if err := tx.Where("world_id = ? AND id = ?", worldID, presetID).Take(&p).Error; err != nil {
				return err
			}
		} else {
			var count int64
			if err := tx.Model(&model.WorldGlassPresetModel{}).Where("world_id = ?", worldID).Count(&count).Error; err != nil {
				return err
			}
			if count >= 20 {
				return ErrWorldGlassLimit
			}
		}
		if in.Name != nil {
			p.Name = strings.TrimSpace(*in.Name)
		}
		if p.Name == "" || utf8.RuneCountInString(p.Name) > 80 {
			return fmt.Errorf("%w: 预设名称需为 1–80 字", ErrWorldGlassInvalid)
		}
		if in.AttachmentID != nil {
			p.AttachmentID = strings.TrimSpace(*in.AttachmentID)
		}
		var attachment model.AttachmentModel
		if err := tx.Where("id = ? AND is_temp = ?", p.AttachmentID, false).Take(&attachment).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: 请选择正式图片附件", ErrWorldGlassInvalid)
			}
			return err
		}
		if !strings.HasPrefix(attachment.MimeType, "image/") {
			return fmt.Errorf("%w: 附件必须是图片", ErrWorldGlassInvalid)
		}
		raw := in.Settings
		if len(raw) == 0 {
			raw = json.RawMessage(p.SettingsJSON)
		}
		settings, err := normalizeWorldGlassSettings(raw)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(settings)
		if err != nil {
			return err
		}
		p.SettingsJSON = string(encoded)
		p.UpdatedBy = userID
		if in.SortOrder != nil {
			p.SortOrder = *in.SortOrder
		}
		if presetID == "" {
			err = tx.Create(&p).Error
		} else {
			err = tx.Model(&p).Updates(map[string]any{"name": p.Name, "attachment_id": p.AttachmentID, "settings_json": p.SettingsJSON, "sort_order": p.SortOrder, "updated_by": userID}).Error
		}
		view = WorldGlassPreset{WorldGlassPresetModel: p, Settings: settings}
		return err
	})
	return &view, revision, err
}

func WorldGlassPresetDelete(worldID, userID, presetID string) (uint64, error) {
	return worldGlassMutate(worldID, userID, func(tx *gorm.DB, state *model.WorldGlassStateModel) error {
		var p model.WorldGlassPresetModel
		if err := tx.Where("world_id = ? AND id = ?", worldID, presetID).Take(&p).Error; err != nil {
			return err
		}
		if err := tx.Where("world_id = ? AND preset_id = ?", worldID, presetID).Delete(&model.WorldGlassTriggerModel{}).Error; err != nil {
			return err
		}
		if state.ActivePresetID == presetID {
			state.Enabled = false
			state.ActivePresetID = ""
		}
		return tx.Delete(&p).Error
	})
}

func WorldGlassActivatePreset(worldID, userID, presetID string) (uint64, error) {
	return worldGlassMutate(worldID, userID, func(tx *gorm.DB, state *model.WorldGlassStateModel) error {
		var p model.WorldGlassPresetModel
		if err := tx.Where("world_id = ? AND id = ?", worldID, presetID).Take(&p).Error; err != nil {
			return err
		}
		state.Enabled = true
		state.ActivePresetID = presetID
		return nil
	})
}

func WorldGlassDisable(worldID, userID string) (uint64, error) {
	return worldGlassMutate(worldID, userID, func(tx *gorm.DB, state *model.WorldGlassStateModel) error { state.Enabled = false; return nil })
}

type WorldGlassTriggerInput struct {
	PresetID    string `json:"presetId"`
	TriggerType string `json:"triggerType"`
	TriggerKey  string `json:"triggerKey"`
	Enabled     *bool  `json:"enabled"`
	SortOrder   int    `json:"sortOrder"`
	Config      struct {
		Version int `json:"version"`
	} `json:"config"`
}

func WorldGlassTriggerUpsert(worldID, userID, triggerID string, in WorldGlassTriggerInput) (uint64, error) {
	if in.TriggerType != "channel" || strings.TrimSpace(in.TriggerKey) == "" || len(in.TriggerKey) > 100 || in.Config.Version != 1 {
		return 0, ErrWorldGlassInvalid
	}
	return worldGlassMutate(worldID, userID, func(tx *gorm.DB, state *model.WorldGlassStateModel) error {
		if err := worldGlassCheckChannel(tx, worldID, in.TriggerKey); err != nil {
			return err
		}
		var p model.WorldGlassPresetModel
		if err := tx.Where("world_id = ? AND id = ?", worldID, in.PresetID).Take(&p).Error; err != nil {
			return err
		}
		var current model.WorldGlassTriggerModel
		if triggerID != "" {
			if err := tx.Where("world_id = ? AND id = ?", worldID, triggerID).Take(&current).Error; err != nil {
				return err
			}
		}
		var target model.WorldGlassTriggerModel
		if err := tx.Where("world_id = ? AND trigger_type = ? AND trigger_key = ?", worldID, "channel", in.TriggerKey).Find(&target).Error; err != nil {
			return err
		}
		if current.ID != "" && target.ID != "" && current.ID != target.ID {
			if err := tx.Delete(&target).Error; err != nil {
				return err
			}
		}
		if current.ID == "" {
			current = target
		}
		if current.ID == "" || current.PresetID != in.PresetID {
			var count int64
			if err := tx.Model(&model.WorldGlassTriggerModel{}).Where("world_id = ? AND preset_id = ?", worldID, in.PresetID).Count(&count).Error; err != nil {
				return err
			}
			if count >= 100 {
				return fmt.Errorf("%w: 每个预设最多绑定 100 个频道", ErrWorldGlassInvalid)
			}
		}
		if current.ID == "" {
			current.WorldID = worldID
			current.CreatedBy = userID
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		current.PresetID = in.PresetID
		current.TriggerType = "channel"
		current.TriggerKey = in.TriggerKey
		current.ConfigJSON = `{"version":1}`
		current.Enabled = enabled
		current.SortOrder = in.SortOrder
		current.UpdatedBy = userID
		if current.ID == "" {
			return tx.Create(&current).Error
		}
		return tx.Model(&current).Updates(map[string]any{"preset_id": current.PresetID, "trigger_key": current.TriggerKey, "config_json": current.ConfigJSON, "enabled": enabled, "sort_order": in.SortOrder, "updated_by": userID}).Error
	})
}

func WorldGlassTriggerDelete(worldID, userID, triggerID string) (uint64, error) {
	return worldGlassMutate(worldID, userID, func(tx *gorm.DB, state *model.WorldGlassStateModel) error {
		result := tx.Where("world_id = ? AND id = ?", worldID, triggerID).Delete(&model.WorldGlassTriggerModel{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// Replace a preset's channel selection atomically, including transfers from other presets.
func WorldGlassChannelTriggersReplace(worldID, userID, presetID string, channelIDs []string, enabled bool) (uint64, error) {
	if len(channelIDs) > 100 {
		return 0, ErrWorldGlassInvalid
	}
	unique := make([]string, 0, len(channelIDs))
	seen := map[string]bool{}
	for _, id := range channelIDs {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 100 {
			return 0, ErrWorldGlassInvalid
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	return worldGlassMutate(worldID, userID, func(tx *gorm.DB, state *model.WorldGlassStateModel) error {
		var p model.WorldGlassPresetModel
		if err := tx.Where("world_id = ? AND id = ?", worldID, presetID).Take(&p).Error; err != nil {
			return err
		}
		for _, id := range unique {
			if err := worldGlassCheckChannel(tx, worldID, id); err != nil {
				return err
			}
		}
		remove := tx.Where("world_id = ? AND preset_id = ? AND trigger_type = ?", worldID, presetID, "channel")
		if len(unique) > 0 {
			remove = remove.Where("trigger_key NOT IN ?", unique)
		}
		if err := remove.Delete(&model.WorldGlassTriggerModel{}).Error; err != nil {
			return err
		}
		for _, id := range unique {
			trigger := model.WorldGlassTriggerModel{WorldID: worldID, PresetID: presetID, TriggerType: "channel", TriggerKey: id, ConfigJSON: `{"version":1}`, Enabled: enabled, CreatedBy: userID, UpdatedBy: userID}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "world_id"}, {Name: "trigger_type"}, {Name: "trigger_key"}},
				DoUpdates: clause.Assignments(map[string]any{"preset_id": presetID, "config_json": `{"version":1}`, "enabled": enabled, "updated_by": userID, "updated_at": time.Now()}),
			}).Create(&trigger).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
