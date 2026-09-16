package service

import (
	"encoding/json"
	"math"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

const TheaterDialogueControllerSystemKind = "theater-dialogue-controller"

type TheaterDialoguePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type TheaterDialogueController struct {
	Version          int                                `json:"version"`
	SharedIdentityID string                             `json:"sharedIdentityId"`
	Enabled          bool                               `json:"enabled"`
	MaxPortraits     int                                `json:"maxPortraits"`
	InactiveStyle    string                             `json:"inactiveStyle"`
	AllowList        []string                           `json:"allowList"`
	DenyList         []string                           `json:"denyList"`
	DragEnabled      bool                               `json:"dragEnabled"`
	Positions        map[string]TheaterDialoguePosition `json:"positions"`
}

// This document belongs to the system shared root, never to a scene or room JSON.
type TheaterDialogueControllerTemplate struct {
	Presentation  protocol.TheaterPresentation `json:"presentation"`
	PortraitStyle protocol.TheaterVisualStyle  `json:"portraitStyle"`
}
type theaterDialoguePatch struct {
	allowedChannels []string
	Enabled         *bool                              `json:"enabled,omitempty"`
	MaxPortraits    *int                               `json:"maxPortraits,omitempty"`
	InactiveStyle   *string                            `json:"inactiveStyle,omitempty"`
	AllowList       *[]string                          `json:"allowList,omitempty"`
	DenyList        *[]string                          `json:"denyList,omitempty"`
	DragEnabled     *bool                              `json:"dragEnabled,omitempty"`
	Template        *TheaterDialogueControllerTemplate `json:"template,omitempty"`
}
type theaterDialoguePositionSet struct {
	allowedChannels []string
	ActorKey        string                  `json:"actorKey"`
	Position        TheaterDialoguePosition `json:"position"`
}
type TheaterDialogueControllerResult struct {
	Revision   int64                              `json:"revision"`
	Controller TheaterDialogueController          `json:"controller"`
	Template   *TheaterDialogueControllerTemplate `json:"template"`
	CanManage  bool                               `json:"canManage"`
	CanDrag    bool                               `json:"canDrag"`
}

func defaultDialogueController() TheaterDialogueController {
	return TheaterDialogueController{Version: 1, MaxPortraits: 4, InactiveStyle: "dim", AllowList: []string{}, DenyList: []string{}, Positions: map[string]TheaterDialoguePosition{}}
}
func dialogueControllerFromState(raw string) (TheaterDialogueController, error) {
	result := defaultDialogueController()
	var state map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return result, err
	}
	if value := state["dialogueController"]; len(value) > 0 {
		if err := json.Unmarshal(value, &result); err != nil {
			return result, err
		}
	}
	if result.Positions == nil {
		result.Positions = map[string]TheaterDialoguePosition{}
	}
	if result.AllowList == nil {
		result.AllowList = []string{}
	}
	if result.DenyList == nil {
		result.DenyList = []string{}
	}
	if result.Version != 1 || (result.Enabled && result.SharedIdentityID == "") {
		return result, theaterPayloadError("对话框控制器版本或系统角色引用无效")
	}
	if err := validateDialoguePatch(&theaterDialoguePatch{MaxPortraits: &result.MaxPortraits, InactiveStyle: &result.InactiveStyle, AllowList: &result.AllowList, DenyList: &result.DenyList}); err != nil {
		return result, err
	}
	for key, position := range result.Positions {
		if err := validateDialoguePosition(&theaterDialoguePositionSet{ActorKey: key, Position: position}); err != nil {
			return result, err
		}
	}
	return result, nil
}
func saveDialogueController(tx *gorm.DB, room *model.TheaterRoomModel, controller TheaterDialogueController) error {
	var state map[string]json.RawMessage
	if err := json.Unmarshal([]byte(room.StateJSON), &state); err != nil {
		return err
	}
	if state == nil {
		state = map[string]json.RawMessage{}
	}
	raw, err := json.Marshal(controller)
	if err != nil {
		return err
	}
	state["dialogueController"] = raw
	raw, err = json.Marshal(state)
	if err != nil {
		return err
	}
	room.StateJSON = string(raw)
	return tx.Model(&model.TheaterRoomModel{}).Where("id = ?", room.ID).Update("state_json", room.StateJSON).Error
}

// Scene import/replace must preserve the current room node, including when an old
// client omits it. Imported controller IDs/templates are never adopted.
func preserveDialogueControllerState(oldRaw, newRaw string) string {
	var oldState, newState map[string]json.RawMessage
	if json.Unmarshal([]byte(oldRaw), &oldState) != nil || json.Unmarshal([]byte(newRaw), &newState) != nil {
		return newRaw
	}
	if newState == nil {
		newState = map[string]json.RawMessage{}
	}
	delete(newState, "dialogueController")
	if raw := oldState["dialogueController"]; len(raw) > 0 {
		newState["dialogueController"] = raw
	}
	raw, err := json.Marshal(newState)
	if err != nil {
		return newRaw
	}
	return string(raw)
}

func validateDialoguePatch(p *theaterDialoguePatch) error {
	if p.MaxPortraits != nil && (*p.MaxPortraits < 1 || *p.MaxPortraits > 12) {
		return theaterPayloadError("maxPortraits 必须为 1～12")
	}
	if p.InactiveStyle != nil && *p.InactiveStyle != "dim" && *p.InactiveStyle != "dim-grayscale" && *p.InactiveStyle != "none" {
		return theaterPayloadError("inactiveStyle 无效")
	}
	for _, list := range []*[]string{p.AllowList, p.DenyList} {
		if list != nil {
			if len(*list) > 512 {
				return theaterPayloadError("角色名单最多 512 项")
			}
			seen := map[string]bool{}
			for _, key := range *list {
				if !validDialogueActorKey(key) || seen[key] {
					return theaterPayloadError("角色引用无效或重复")
				}
				seen[key] = true
			}
		}
	}
	if p.Template != nil {
		if p.Template.Presentation.Portrait != nil {
			return theaterPayloadError("公共模板不能包含个人立绘素材")
		}
		if err := protocol.ValidateTheaterPresentation(p.Template.Presentation); err != nil {
			return theaterPayloadError(err.Error())
		}
		if err := protocol.ValidateWorldTheaterPresentationTemplate(protocol.WorldTheaterPresentationTemplate{Portrait: &p.Template.PortraitStyle}); err != nil {
			return theaterPayloadError(err.Error())
		}
	}
	return nil
}
func validDialogueActorKey(key string) bool {
	prefix, id, ok := strings.Cut(key, ":")
	return ok && (prefix == "identity" || prefix == "shared") && id != "" && len(id) <= 100 && strings.TrimSpace(id) == id && !strings.Contains(id, ":")
}
func validateDialoguePosition(p *theaterDialoguePositionSet) error {
	if !validDialogueActorKey(p.ActorKey) {
		return theaterPayloadError("actorKey 无效")
	}
	for _, v := range []float64{p.Position.X, p.Position.Y} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return theaterPayloadError("位置必须归一化到 0～1")
		}
	}
	return nil
}
func canMoveDialoguePortrait(worldID, actorID string) bool {
	if !CanViewTheater(actorID, worldID, "") {
		return false
	}
	world, err := GetWorldByID(worldID)
	if err != nil || world == nil {
		return false
	}
	if world.OwnerID == actorID {
		return true
	}
	member, err := getActiveWorldMember(worldID, actorID)
	return err == nil && (member.Role == model.WorldRoleMember || member.Role == model.WorldRoleAdmin || member.Role == model.WorldRoleOwner)
}
func dialogueTemplateFromRoot(root *model.SharedChannelIdentityModel) (*TheaterDialogueControllerTemplate, error) {
	if root == nil {
		return nil, nil
	}
	var result TheaterDialogueControllerTemplate
	if err := json.Unmarshal([]byte(root.SharedDataJSON), &result); err != nil {
		return nil, err
	}
	if root.TheaterPresentation != nil {
		result.Presentation = *root.TheaterPresentation
	}
	return &result, nil
}
func loadDialogueController(tx *gorm.DB, room *model.TheaterRoomModel) (TheaterDialogueController, *TheaterDialogueControllerTemplate, error) {
	config, err := dialogueControllerFromState(room.StateJSON)
	if err != nil {
		return config, nil, err
	}
	if config.SharedIdentityID == "" {
		return config, nil, nil
	}
	var root model.SharedChannelIdentityModel
	if err = tx.Where("id = ? AND world_id = ? AND system_kind = ?", config.SharedIdentityID, room.WorldID, TheaterDialogueControllerSystemKind).Take(&root).Error; err != nil {
		return config, nil, err
	}
	template, err := dialogueTemplateFromRoot(&root)
	return config, template, err
}
func applyDialoguePatch(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, p *theaterDialoguePatch) error {
	config, err := dialogueControllerFromState(room.StateJSON)
	if err != nil {
		return err
	}
	for _, pair := range []struct {
		next     *[]string
		previous []string
	}{{p.AllowList, config.AllowList}, {p.DenyList, config.DenyList}} {
		if pair.next != nil {
			for _, key := range *pair.next {
				alreadySaved := false
				for _, saved := range pair.previous {
					if key == saved {
						alreadySaved = true
						break
					}
				}
				if alreadySaved {
					continue
				}
				if err := validateDialogueActorReference(tx, room.WorldID, p.allowedChannels, key); err != nil {
					return err
				}
			}
		}
	}
	if p.Enabled != nil && *p.Enabled && config.SharedIdentityID == "" {
		presentation := protocol.DefaultTheaterPresentation()
		presentation.Portrait = nil
		style := protocol.TheaterVisualStyle{Enabled: true, Transform: protocol.TheaterTransform{X: 0, Y: 0, Width: 1, Height: 0.72, Opacity: 1}, Fit: "cover", PlaybackRate: 1, BlendMode: "normal"}
		template := TheaterDialogueControllerTemplate{Presentation: presentation, PortraitStyle: style}
		raw, _ := json.Marshal(map[string]any{"portraitStyle": template.PortraitStyle})
		kind := TheaterDialogueControllerSystemKind
		root := model.SharedChannelIdentityModel{StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()}, WorldID: room.WorldID, SystemKind: &kind, DisplayName: "全局对话框", SharedDataJSON: string(raw), TheaterPresentation: &presentation}
		// The room revision CAS already serializes this transaction; the nullable
		// composite unique index also protects this invariant across server processes.
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&root).Error; err != nil {
			return err
		}
		root = model.SharedChannelIdentityModel{}
		if err := tx.Where("world_id = ? AND system_kind = ?", room.WorldID, kind).Take(&root).Error; err != nil {
			return err
		}
		config.SharedIdentityID = root.ID
	}
	if p.Enabled != nil {
		config.Enabled = *p.Enabled
	}
	if p.MaxPortraits != nil {
		config.MaxPortraits = *p.MaxPortraits
	}
	if p.InactiveStyle != nil {
		config.InactiveStyle = *p.InactiveStyle
	}
	if p.AllowList != nil {
		config.AllowList = *p.AllowList
	}
	if p.DenyList != nil {
		config.DenyList = *p.DenyList
	}
	if p.DragEnabled != nil {
		config.DragEnabled = *p.DragEnabled
	}
	if p.Template != nil {
		if config.SharedIdentityID == "" {
			return theaterPayloadError("请先开启多人演出")
		}
		for _, ref := range theaterPresentationMediaRefs(p.Template.Presentation) {
			var asset model.TheaterAppearanceAssetModel
			if err := tx.Where("id = ? AND identity_id = ? AND deleted_at IS NULL", ref.AssetID, config.SharedIdentityID).Take(&asset).Error; err != nil {
				return theaterPayloadError("公共模板素材不属于系统角色")
			}
			if asset.Status != "ready" || !theaterMediaRefMatchesAsset(ref, asset) {
				return theaterPayloadError("公共模板素材尚未就绪")
			}
		}
		raw, _ := json.Marshal(map[string]any{"portraitStyle": p.Template.PortraitStyle})
		presentationRaw, _ := json.Marshal(p.Template.Presentation)
		result := tx.Model(&model.SharedChannelIdentityModel{}).Where("id = ? AND world_id = ? AND system_kind = ?", config.SharedIdentityID, room.WorldID, TheaterDialogueControllerSystemKind).Updates(map[string]any{"shared_data_json": string(raw), "theater_presentation": string(presentationRaw), "revision": gorm.Expr("revision + 1")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return theaterPayloadError("系统角色不存在")
		}
	}
	return saveDialogueController(tx, room, config)
}
func applyDialoguePosition(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, p *theaterDialoguePositionSet) error {
	config, err := dialogueControllerFromState(room.StateJSON)
	if err != nil {
		return err
	}
	if !config.Enabled || !config.DragEnabled {
		return newTheaterError(TheaterErrorPermissionDenied, "立绘拖拽尚未启用", 403, nil)
	}
	if err := validateDialogueActorReference(tx, room.WorldID, p.allowedChannels, p.ActorKey); err != nil {
		return err
	}
	allowed := len(config.AllowList) == 0
	for _, key := range config.AllowList {
		if key == p.ActorKey {
			allowed = true
		}
	}
	for _, key := range config.DenyList {
		if key == p.ActorKey {
			allowed = false
		}
	}
	if !allowed {
		return newTheaterError(TheaterErrorPermissionDenied, "角色已被名单排除", 403, nil)
	}
	if len(config.Positions) >= 2048 {
		if _, exists := config.Positions[p.ActorKey]; !exists {
			return theaterPayloadError("位置偏好数量超限")
		}
	}
	config.Positions[p.ActorKey] = p.Position
	return saveDialogueController(tx, room, config)
}

func GetTheaterDialogueController(actorID, worldID string) (*TheaterDialogueControllerResult, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, "", TheaterPermissionView); err != nil {
		return nil, err
	}
	result := &TheaterDialogueControllerResult{Controller: defaultDialogueController(), CanManage: IsWorldAdmin(worldID, actorID), CanDrag: canMoveDialoguePortrait(worldID, actorID)}
	// Read-only: never materialize the room or system identity.
	room, err := model.TheaterRoomFindByScope(worldID, "")
	if err != nil {
		return nil, err
	}
	if room == nil {
		return result, nil
	}
	result.Revision = room.Revision
	result.Controller, result.Template, err = loadDialogueController(model.GetDB(), room)
	return result, err
}

type TheaterCharacterOption struct {
	ActorKey         string `json:"actorKey"`
	IdentityID       string `json:"identityId"`
	SharedIdentityID string `json:"sharedIdentityId,omitempty"`
	SourceChannelID  string `json:"sourceChannelId"`
	DisplayName      string `json:"displayName"`
	ChannelName      string `json:"channelName"`
}
type TheaterCharacterOptions struct {
	Items    []TheaterCharacterOption `json:"items"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"pageSize"`
}

func dialogueCharacterQuery(tx *gorm.DB, worldID, actorID string) (*gorm.DB, error) {
	allowed, err := ChannelIdListByWorld(actorID, worldID, false)
	if err != nil {
		return nil, err
	}
	return dialogueCharacterQueryForChannels(tx, worldID, allowed), nil
}
func dialogueCharacterQueryForChannels(tx *gorm.DB, worldID string, allowed []string) *gorm.DB {
	return tx.Model(&model.ChannelIdentityModel{}).Where("channel_id IN ? AND (is_hidden = ? OR is_hidden IS NULL)", allowed, false).
		Where("channel_id IN (?)", tx.Model(&model.ChannelModel{}).Select("id").Where("world_id = ? AND status = ?", worldID, model.ChannelStatusActive)).
		Where("shared_identity_id IS NULL OR shared_identity_id = '' OR shared_identity_id IN (?)", tx.Model(&model.SharedChannelIdentityModel{}).Select("id").Where("world_id = ? AND system_kind IS NULL", worldID))
}
func validateDialogueActorReference(tx *gorm.DB, worldID string, allowed []string, key string) error {
	if !validDialogueActorKey(key) {
		return theaterPayloadError("角色引用无效")
	}
	q := dialogueCharacterQueryForChannels(tx, worldID, allowed)
	kind, id, _ := strings.Cut(key, ":")
	if kind == "shared" {
		q = q.Where("shared_identity_id = ?", id)
	} else {
		q = q.Where("id = ? AND (shared_identity_id IS NULL OR shared_identity_id = '')", id)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return theaterPayloadError("角色不属于当前世界或不可访问")
	}
	return nil
}
func ListTheaterCharacterOptions(actorID, worldID, keyword string, page, pageSize int) (*TheaterCharacterOptions, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, "", TheaterPermissionView); err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	if pageSize < 1 {
		pageSize = 30
	}
	if pageSize > 100 {
		pageSize = 100
	}
	q, err := dialogueCharacterQuery(model.GetDB(), worldID, actorID)
	if err != nil {
		return nil, err
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		q = q.Where("LOWER(display_name) LIKE ?", "%"+strings.ToLower(keyword)+"%")
	}
	roots := q.Select("MIN(id)").Group("CASE WHEN shared_identity_id IS NULL OR shared_identity_id = '' THEN 0 ELSE 1 END, COALESCE(NULLIF(shared_identity_id, ''), id)")
	result := &TheaterCharacterOptions{Items: []TheaterCharacterOption{}, Page: page, PageSize: pageSize}
	selected := model.GetDB().Model(&model.ChannelIdentityModel{}).Where("id IN (?)", roots)
	if err = selected.Count(&result.Total).Error; err != nil {
		return nil, err
	}
	var rows []model.ChannelIdentityModel
	if err = selected.Select("id", "channel_id", "shared_identity_id", "display_name").Order("display_name ASC, id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	channels := map[string]string{}
	var ids []string
	for _, row := range rows {
		ids = append(ids, row.ChannelID)
	}
	if len(ids) > 0 {
		var items []model.ChannelModel
		if err = model.GetDB().Select("id", "name").Where("id IN ?", ids).Find(&items).Error; err != nil {
			return nil, err
		}
		for _, item := range items {
			channels[item.ID] = item.Name
		}
	}
	for _, row := range rows {
		key := "identity:" + row.ID
		if row.SharedIdentityID != "" {
			key = "shared:" + row.SharedIdentityID
		}
		result.Items = append(result.Items, TheaterCharacterOption{ActorKey: key, IdentityID: row.ID, SharedIdentityID: row.SharedIdentityID, SourceChannelID: row.ChannelID, DisplayName: row.DisplayName, ChannelName: channels[row.ChannelID]})
	}
	return result, nil
}
