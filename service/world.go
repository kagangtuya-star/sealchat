package service

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/width"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

var (
	ErrWorldNotFound              = errors.New("world not found")
	ErrWorldPermission            = errors.New("world permission denied")
	ErrWorldCreateForbidden       = errors.New("仅平台管理员可创建世界")
	ErrWorldInviteInvalid         = errors.New("world invite invalid")
	ErrWorldMemberInvalid         = errors.New("world member invalid")
	ErrWorldOwnerImmutable        = errors.New("world owner immutable")
	ErrWorldDescriptionTooLong    = errors.New("世界简介不能超过100字（中文按1字、英文按0.5字计算）")
	ErrWorldSystemDefaultProtect  = errors.New("系统默认世界不可删除")
	ErrWorldObserverSlugInvalid   = errors.New("world observer slug invalid")
	ErrWorldObserverSlugConflict  = errors.New("world observer slug conflict")
	ErrWorldObserverLinkInvalid   = errors.New("world observer link invalid")
	ErrWorldDefaultDiceMode       = errors.New("world default dice mode invalid")
	ErrWorldDefaultDiceBotEmpty   = errors.New("world default dice bot required")
	ErrWorldDefaultDiceBotInvalid = errors.New("world default dice bot invalid")
)

const (
	worldDescriptionMaxLength     = 100
	worldDescriptionMaxWidthUnits = worldDescriptionMaxLength * 2
)

var worldObserverSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{3,31}$`)

func worldObserverSlugValue(slug *string) string {
	if slug == nil {
		return ""
	}
	return strings.TrimSpace(*slug)
}

type WorldCreateParams struct {
	Name                   string
	Description            string
	Visibility             string
	Avatar                 string
	ChannelDefaultDiceMode string
	ChannelDefaultBotID    string
}

type WorldUpdateParams struct {
	Name                                  string
	Description                           string
	Visibility                            string
	Avatar                                string
	EnforceMembership                     *bool
	AllowAdminEditMessages                *bool
	AllowManageOtherUserChannelIdentities *bool
	AllowMemberEditKeywords               *bool
	StrictWhisperPrivacy                  *bool
	ChannelDefaultDiceMode                *string
	ChannelDefaultBotID                   *string
	CharacterCardBadgeTemplate            *string
}

func normalizeWorldDescription(desc string) (string, error) {
	desc = strings.TrimSpace(desc)
	if countDisplayWidthUnits(desc) > worldDescriptionMaxWidthUnits {
		return "", ErrWorldDescriptionTooLong
	}
	return desc, nil
}

func countDisplayWidthUnits(text string) int {
	total := 0
	for _, r := range text {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			total += 2
		default:
			total++
		}
	}
	return total
}

func normalizeWorldBadgeTemplate(template string) (string, error) {
	template = strings.TrimSpace(template)
	if utf8.RuneCountInString(template) > 512 {
		return "", errors.New("徽章模板长度需在512个字符以内")
	}
	return template, nil
}

func normalizeWorldChannelDefaultDiceMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		return model.WorldChannelDefaultDiceModeBuiltin, nil
	}
	switch normalized {
	case model.WorldChannelDefaultDiceModeBuiltin, model.WorldChannelDefaultDiceModeBot:
		return normalized, nil
	default:
		return "", ErrWorldDefaultDiceMode
	}
}

func validateWorldDefaultBotID(botID string) (string, error) {
	trimmed := strings.TrimSpace(botID)
	if trimmed == "" {
		return "", nil
	}
	user := model.UserGet(trimmed)
	if user == nil || user.ID == "" || !user.IsBot {
		return "", ErrWorldDefaultDiceBotInvalid
	}
	return trimmed, nil
}

func ResolveWorldChannelDefaultDiceConfig(worldID string) (string, string, error) {
	world, err := GetWorldByID(worldID)
	if err != nil {
		return "", "", err
	}
	mode, err := normalizeWorldChannelDefaultDiceMode(world.ChannelDefaultDiceMode)
	if err != nil {
		return "", "", err
	}
	botID, err := validateWorldDefaultBotID(world.ChannelDefaultBotID)
	if err != nil {
		return "", "", err
	}
	if mode == model.WorldChannelDefaultDiceModeBot && botID == "" {
		return "", "", ErrWorldDefaultDiceBotEmpty
	}
	return mode, botID, nil
}

func ApplyWorldChannelDefaultDiceConfig(channelID, mode, botID string) error {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return errors.New("频道ID不能为空")
	}
	normalizedMode, err := normalizeWorldChannelDefaultDiceMode(mode)
	if err != nil {
		return err
	}
	validBotID, err := validateWorldDefaultBotID(botID)
	if err != nil {
		return err
	}
	if normalizedMode != model.WorldChannelDefaultDiceModeBot {
		return nil
	}
	if validBotID == "" {
		return ErrWorldDefaultDiceBotEmpty
	}
	roleID := fmt.Sprintf("ch-%s-bot", channelID)
	tx := model.GetDB().Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if err := tx.Model(&model.ChannelModel{}).
		Where("id = ?", channelID).
		Updates(map[string]any{
			"built_in_dice_enabled": false,
			"bot_feature_enabled":   true,
			"updated_at":            time.Now(),
		}).Error; err != nil {
		tx.Rollback()
		return err
	}
	mapping := &model.UserRoleMappingModel{
		UserID:   validBotID,
		RoleID:   roleID,
		RoleType: "channel",
	}
	mapping.Init()
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(mapping).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	if err := EnsureBotChannelIdentity(validBotID, channelID); err != nil {
		return err
	}
	return nil
}

func normalizeWorldObserverSlug(slug string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(slug))
	if normalized == "" {
		return "", nil
	}
	if !worldObserverSlugPattern.MatchString(normalized) {
		return "", ErrWorldObserverSlugInvalid
	}
	return normalized, nil
}

func isWorldUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed") {
		return true
	}
	if strings.Contains(msg, "Error 1062") || strings.Contains(msg, "Duplicate entry") {
		return true
	}
	if strings.Contains(msg, "SQLSTATE 23505") || strings.Contains(msg, "duplicate key value") {
		return true
	}
	return false
}

func pickWorldObserverEntryChannelID(world *model.WorldModel) string {
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return ""
	}
	defaultChannelID := strings.TrimSpace(world.DefaultChannelID)
	if defaultChannelID != "" {
		if channel, err := CanObserverAccessChannel(defaultChannelID, world.ID); err == nil && channel != nil && strings.TrimSpace(channel.ID) != "" {
			return channel.ID
		}
	}
	channels, err := ChannelListByWorld(world.ID)
	if err != nil {
		return ""
	}
	for _, channel := range channels {
		if channel == nil || strings.TrimSpace(channel.ID) == "" {
			continue
		}
		return channel.ID
	}
	return ""
}

func WorldObserverLinkGet(worldID, actorID string) (string, bool, error) {
	world, err := GetWorldByID(worldID)
	if err != nil {
		return "", false, err
	}
	if !IsWorldAdmin(worldID, actorID) {
		return "", false, ErrWorldPermission
	}
	slug := worldObserverSlugValue(world.ObserverSlug)
	if slug == "" {
		return "", false, nil
	}
	return slug, world.ObserverEnabled, nil
}

func WorldObserverLinkUpdate(worldID, actorID, slug string, enabled bool) (*model.WorldModel, error) {
	_, err := GetWorldByID(worldID)
	if err != nil {
		return nil, err
	}
	if !IsWorldAdmin(worldID, actorID) {
		return nil, ErrWorldPermission
	}
	normalizedSlug, err := normalizeWorldObserverSlug(slug)
	if err != nil {
		return nil, err
	}
	if enabled {
		if normalizedSlug == "" {
			return nil, ErrWorldObserverSlugInvalid
		}
	}
	updates := map[string]any{
		"updated_at":       time.Now(),
		"observer_enabled": enabled,
	}
	if normalizedSlug == "" {
		updates["observer_slug"] = nil
		updates["observer_enabled"] = false
	} else {
		updates["observer_slug"] = normalizedSlug
	}
	if err := model.GetDB().Model(&model.WorldModel{}).Where("id = ?", worldID).Updates(updates).Error; err != nil {
		if isWorldUniqueConstraintError(err) {
			return nil, ErrWorldObserverSlugConflict
		}
		return nil, err
	}
	return GetWorldByID(worldID)
}

func ResolveWorldObserverLink(slug string) (*model.WorldModel, string, error) {
	normalizedSlug, err := normalizeWorldObserverSlug(slug)
	if err != nil || normalizedSlug == "" {
		return nil, "", ErrWorldObserverLinkInvalid
	}
	var world model.WorldModel
	if err := model.GetDB().Where("observer_slug = ? AND status = ?", normalizedSlug, "active").Limit(1).Find(&world).Error; err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(world.ID) == "" {
		return nil, "", ErrWorldObserverLinkInvalid
	}
	if !world.ObserverEnabled {
		return nil, "", ErrWorldObserverLinkInvalid
	}
	channelID := pickWorldObserverEntryChannelID(&world)
	return &world, channelID, nil
}

func GetOrCreateDefaultWorld() (*model.WorldModel, error) {
	db := model.GetDB()
	var world model.WorldModel
	// 优先查找显式标记的系统默认世界（按创建时间排序确保确定性）
	if err := db.Where("is_system_default = ? AND status = ?", true, "active").
		Order("created_at asc").
		Limit(1).
		Find(&world).Error; err != nil {
		return nil, err
	}
	if world.ID != "" {
		return &world, nil
	}
	// 如果不存在系统默认世界，创建一个新的
	w := &model.WorldModel{
		Name:                 "公共世界",
		Description:          "系统自动创建的默认世界",
		Visibility:           model.WorldVisibilityPublic,
		StrictWhisperPrivacy: true,
		IsSystemDefault:      true,
		Status:               "active",
	}
	if err := db.Create(w).Error; err != nil {
		return nil, err
	}
	return w, nil
}

func BootstrapDefaultWorldForOwner(ownerID string) (*model.WorldModel, error) {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return nil, errors.New("owner_id required")
	}
	world, err := GetOrCreateDefaultWorld()
	if err != nil {
		return nil, err
	}
	if err := bootstrapWorldWithOwner(world, ownerID); err != nil {
		return nil, err
	}
	return world, nil
}

func bootstrapWorldWithOwner(world *model.WorldModel, ownerID string) error {
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return errors.New("invalid world")
	}
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return errors.New("owner_id required")
	}
	db := model.GetDB()
	updates := map[string]any{}
	if strings.TrimSpace(world.OwnerID) == "" {
		updates["owner_id"] = ownerID
	}
	if strings.TrimSpace(world.Name) == "" || world.Name == "默认世界" {
		updates["name"] = "公共世界"
	}
	if strings.TrimSpace(world.Description) == "" {
		updates["description"] = "系统自动创建的默认世界"
	}
	if strings.TrimSpace(world.Visibility) == "" {
		updates["visibility"] = model.WorldVisibilityPublic
	}
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := db.Model(&model.WorldModel{}).Where("id = ?", world.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := db.Where("id = ?", world.ID).Limit(1).Find(world).Error; err != nil {
			return err
		}
	}
	if err := ensureWorldOwnerRole(world.ID, ownerID); err != nil {
		return err
	}
	return ensureWorldDefaultChannel(world, ownerID)
}

func ensureWorldOwnerRole(worldID, ownerID string) error {
	db := model.GetDB()
	var member model.WorldMemberModel
	if err := db.Where("world_id = ? AND user_id = ?", worldID, ownerID).Limit(1).Find(&member).Error; err != nil {
		return err
	}
	if member.ID == "" {
		_, err := WorldJoin(worldID, ownerID, model.WorldRoleOwner)
		return err
	}
	if member.Role != model.WorldRoleOwner {
		return db.Model(&model.WorldMemberModel{}).Where("id = ?", member.ID).Update("role", model.WorldRoleOwner).Error
	}
	return nil
}

func ensureWorldDefaultChannel(world *model.WorldModel, ownerID string) error {
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return errors.New("invalid world")
	}
	if strings.TrimSpace(world.DefaultChannelID) != "" {
		return nil
	}
	db := model.GetDB()
	var existing model.ChannelModel
	if err := db.Where("world_id = ? AND status = ?", world.ID, "active").Order("created_at asc").Limit(1).Find(&existing).Error; err == nil && existing.ID != "" {
		if err := db.Model(&model.WorldModel{}).Where("id = ?", world.ID).Update("default_channel_id", existing.ID).Error; err != nil {
			return err
		}
		world.DefaultChannelID = existing.ID
		return nil
	}
	name := strings.TrimSpace(world.Name)
	if name == "" {
		name = "公共世界"
	}
	channelName := fmt.Sprintf("%s大厅", name)
	channel := ChannelNew(utils.NewID(), "public", channelName, world.ID, ownerID, "")
	if channel == nil {
		return errors.New("failed to create default channel")
	}
	world.DefaultChannelID = channel.ID
	return db.Model(&model.WorldModel{}).Where("id = ?", world.ID).Update("default_channel_id", channel.ID).Error
}

func GetWorldByID(worldID string) (*model.WorldModel, error) {
	if strings.TrimSpace(worldID) == "" {
		return nil, ErrWorldNotFound
	}
	var world model.WorldModel
	if err := model.GetDB().Where("id = ?", worldID).Limit(1).Find(&world).Error; err != nil {
		return nil, err
	}
	if world.ID == "" {
		return nil, ErrWorldNotFound
	}
	return &world, nil
}

func WorldCreate(ownerID string, params WorldCreateParams) (*model.WorldModel, *model.ChannelModel, error) {
	// 检查是否允许非平台管理员创建世界
	config := utils.GetConfig()
	if config != nil && !config.Audio.AllowNonAdminCreateWorld {
		if !pm.CanWithSystemRole(ownerID, pm.PermModAdmin) {
			return nil, nil, ErrWorldCreateForbidden
		}
	}
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, nil, errors.New("世界名称不能为空")
	}
	description, err := normalizeWorldDescription(params.Description)
	if err != nil {
		return nil, nil, err
	}
	visibility := params.Visibility
	if visibility == "" {
		visibility = model.WorldVisibilityPublic
	}
	defaultDiceMode, err := normalizeWorldChannelDefaultDiceMode(params.ChannelDefaultDiceMode)
	if err != nil {
		return nil, nil, err
	}
	defaultBotID, err := validateWorldDefaultBotID(params.ChannelDefaultBotID)
	if err != nil {
		return nil, nil, err
	}
	if defaultDiceMode == model.WorldChannelDefaultDiceModeBot && defaultBotID == "" {
		return nil, nil, ErrWorldDefaultDiceBotEmpty
	}
	world := &model.WorldModel{
		Name:                    name,
		Description:             description,
		Avatar:                  params.Avatar,
		Visibility:              visibility,
		OwnerID:                 ownerID,
		EnforceMembership:       false,
		AllowMemberEditKeywords: false,
		StrictWhisperPrivacy:    true,
		ChannelDefaultDiceMode:  defaultDiceMode,
		ChannelDefaultBotID:     defaultBotID,
		Status:                  "active",
	}
	db := model.GetDB()
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(world).Error; err != nil {
			return err
		}
		member := &model.WorldMemberModel{
			WorldID:  world.ID,
			UserID:   ownerID,
			Role:     model.WorldRoleOwner,
			JoinedAt: time.Now(),
		}
		if err := tx.Create(member).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	channelName := fmt.Sprintf("%s大厅", name)
	defaultChannel := ChannelNew(utils.NewID(), "public", channelName, world.ID, ownerID, "")
	if defaultChannel != nil {
		if defaultDiceMode == model.WorldChannelDefaultDiceModeBot {
			if err := ApplyWorldChannelDefaultDiceConfig(defaultChannel.ID, defaultDiceMode, defaultBotID); err == nil {
				defaultChannel.BuiltInDiceEnabled = false
				defaultChannel.BotFeatureEnabled = true
			}
		}
		_ = db.Model(&model.WorldModel{}).
			Where("id = ?", world.ID).
			Update("default_channel_id", defaultChannel.ID).Error
	}
	return world, defaultChannel, nil
}

func WorldUpdate(worldID, actorID string, params WorldUpdateParams) (*model.WorldModel, error) {
	world := &model.WorldModel{}
	if err := model.GetDB().Where("id = ? AND status = ?", worldID, "active").Limit(1).Find(world).Error; err != nil {
		return nil, err
	}
	if world.ID == "" {
		return nil, ErrWorldNotFound
	}
	if !IsWorldAdmin(worldID, actorID) {
		return nil, ErrWorldPermission
	}
	updates := map[string]interface{}{}
	if name := strings.TrimSpace(params.Name); name != "" {
		updates["name"] = name
	}
	if params.Description != "" {
		description, err := normalizeWorldDescription(params.Description)
		if err != nil {
			return nil, err
		}
		updates["description"] = description
	}
	if params.Avatar != "" {
		updates["avatar"] = params.Avatar
	}
	if params.Visibility != "" {
		visibility := strings.ToLower(strings.TrimSpace(params.Visibility))
		updates["visibility"] = visibility
	}
	if params.EnforceMembership != nil {
		updates["enforce_membership"] = *params.EnforceMembership
	}
	if params.AllowAdminEditMessages != nil {
		updates["allow_admin_edit_messages"] = *params.AllowAdminEditMessages
	}
	if params.AllowManageOtherUserChannelIdentities != nil {
		updates["allow_manage_other_user_channel_identities"] = *params.AllowManageOtherUserChannelIdentities
	}
	if params.AllowMemberEditKeywords != nil {
		updates["allow_member_edit_keywords"] = *params.AllowMemberEditKeywords
	}
	if params.StrictWhisperPrivacy != nil {
		updates["strict_whisper_privacy"] = *params.StrictWhisperPrivacy
	}
	if params.ChannelDefaultDiceMode != nil || params.ChannelDefaultBotID != nil {
		nextMode := world.ChannelDefaultDiceMode
		if params.ChannelDefaultDiceMode != nil {
			nextMode = *params.ChannelDefaultDiceMode
		}
		normalizedMode, err := normalizeWorldChannelDefaultDiceMode(nextMode)
		if err != nil {
			return nil, err
		}
		nextBotID := world.ChannelDefaultBotID
		if params.ChannelDefaultBotID != nil {
			nextBotID = *params.ChannelDefaultBotID
		}
		normalizedBotID, err := validateWorldDefaultBotID(nextBotID)
		if err != nil {
			return nil, err
		}
		if normalizedMode == model.WorldChannelDefaultDiceModeBot && normalizedBotID == "" {
			return nil, ErrWorldDefaultDiceBotEmpty
		}
		updates["channel_default_dice_mode"] = normalizedMode
		updates["channel_default_bot_id"] = normalizedBotID
	}
	if params.CharacterCardBadgeTemplate != nil {
		template, err := normalizeWorldBadgeTemplate(*params.CharacterCardBadgeTemplate)
		if err != nil {
			return nil, err
		}
		updates["character_card_badge_template"] = template
	}
	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := model.GetDB().Model(world).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	if err := model.GetDB().Where("id = ? AND status = ?", worldID, "active").Limit(1).Find(world).Error; err != nil {
		return nil, err
	}
	return world, nil
}

func WorldDelete(worldID, actorID string) error {
	db := model.GetDB()
	// 先检查世界是否存在
	var world model.WorldModel
	if err := db.Where("id = ?", worldID).Limit(1).Find(&world).Error; err != nil {
		return err
	}
	if world.ID == "" {
		return ErrWorldNotFound
	}
	// 检查是否为系统默认世界
	if world.IsSystemDefault {
		return ErrWorldSystemDefaultProtect
	}
	// 最后检查权限
	if !IsWorldOwner(worldID, actorID) {
		return ErrWorldPermission
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.WorldModel{}).
			Where("id = ?", worldID).
			Updates(map[string]any{"status": "archived", "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		if err := tx.Where("world_id = ?", worldID).Delete(&model.WorldMemberModel{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ChannelModel{}).
			Where("world_id = ?", worldID).
			Updates(map[string]any{"status": "archived", "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.WorldInviteModel{}).
			Where("world_id = ?", worldID).
			Updates(map[string]any{"status": "archived", "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		if err := tx.Where("world_id = ?", worldID).Delete(&model.WorldFavoriteModel{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.MessageModel{}).
			Where("channel_id IN (?)", tx.Table("channels").Select("id").Where("world_id = ?", worldID)).
			Updates(map[string]any{"is_archived": true, "archived_at": time.Now(), "archive_reason": "world_deleted"}).Error; err != nil {
			return err
		}
		if err := ArchiveAnnouncementsByScope(tx, model.AnnouncementScopeWorld, worldID); err != nil {
			return err
		}
		return nil
	})
}

func WorldJoin(worldID, userID, role string) (*model.WorldMemberModel, error) {
	role = normalizeWorldRole(role)
	db := model.GetDB()
	var world model.WorldModel
	if err := db.Where("id = ? AND status = ?", worldID, "active").Limit(1).Find(&world).Error; err != nil {
		return nil, err
	}
	if world.ID == "" {
		return nil, ErrWorldNotFound
	}
	member := &model.WorldMemberModel{}
	if err := db.Where("world_id = ? AND user_id = ?", worldID, userID).Limit(1).Find(member).Error; err != nil {
		return nil, err
	}
	if member.ID != "" {
		if _, err := ensureWorldMemberChannelState(worldID, userID, member.Role); err != nil {
			return member, err
		}
		return member, nil
	}
	member = &model.WorldMemberModel{
		WorldID:  worldID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
	}
	if err := db.Create(member).Error; err != nil {
		return nil, err
	}
	if _, err := ensureWorldMemberChannelState(worldID, userID, role); err != nil {
		return member, err
	}
	return member, nil
}

func WorldLeave(worldID, userID string) error {
	if IsWorldOwner(worldID, userID) {
		return errors.New("世界拥有者无法退出，请先转移所有权或删除世界")
	}
	db := model.GetDB()
	if err := db.Where("world_id = ? AND user_id = ?", worldID, userID).Delete(&model.WorldMemberModel{}).Error; err != nil {
		return err
	}
	if err := revokeWorldChannelRoles(worldID, userID); err != nil {
		return err
	}
	_ = db.Where("world_id = ? AND user_id = ?", worldID, userID).Delete(&model.WorldFavoriteModel{})
	return nil
}

func IsWorldOwner(worldID, userID string) bool {
	return worldRoleEquals(worldID, userID, model.WorldRoleOwner)
}

func IsWorldAdmin(worldID, userID string) bool {
	if worldRoleEquals(worldID, userID, model.WorldRoleOwner) {
		return true
	}
	return worldRoleEquals(worldID, userID, model.WorldRoleAdmin)
}

func IsWorldMember(worldID, userID string) bool {
	return worldRoleEquals(worldID, userID, "")
}

func worldRoleEquals(worldID, userID, role string) bool {
	var member model.WorldMemberModel
	err := model.GetDB().Where("world_id = ? AND user_id = ?", worldID, userID).Limit(1).Find(&member).Error
	if err != nil || member.ID == "" {
		return false
	}
	if role == "" {
		return true
	}
	return member.Role == role
}

func ListWorldMembers(worldID string, limit int) ([]*model.WorldMemberModel, error) {
	if limit <= 0 {
		limit = 20
	}
	var members []*model.WorldMemberModel
	err := model.GetDB().Where("world_id = ?", worldID).
		Order("joined_at asc").
		Limit(limit).
		Find(&members).Error
	return members, err
}

type WorldMemberDetail struct {
	ID       string    `json:"id"`
	WorldID  string    `json:"worldId"`
	UserID   string    `json:"userId"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
	Username string    `json:"username"`
	Nickname string    `json:"nickname"`
	Avatar   string    `json:"avatar"`
}

func ListWorldMembersDetail(worldID string, page, pageSize int, keyword string) ([]*WorldMemberDetail, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	db := model.GetDB()
	query := db.Table("world_members AS wm").
		Select("wm.id, wm.world_id, wm.user_id, wm.role, wm.joined_at, u.username, u.nickname, u.avatar").
		Joins("LEFT JOIN users u ON u.id = wm.user_id").
		Where("wm.world_id = ?", worldID)
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("wm.user_id LIKE ? OR u.username LIKE ? OR u.nickname LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	var rows []struct {
		ID       string
		WorldID  string
		UserID   string
		Role     string
		JoinedAt time.Time
		Username string
		Nickname string
		Avatar   string
	}
	if err := query.Order("wm.joined_at asc").
		Offset(offset).
		Limit(pageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	result := make([]*WorldMemberDetail, 0, len(rows))
	for _, row := range rows {
		result = append(result, &WorldMemberDetail{
			ID:       row.ID,
			WorldID:  row.WorldID,
			UserID:   row.UserID,
			Role:     row.Role,
			JoinedAt: row.JoinedAt,
			Username: row.Username,
			Nickname: row.Nickname,
			Avatar:   row.Avatar,
		})
	}
	return result, total, nil
}

func ensureWorldChannelMemberships(worldID, userID string) error {
	channels, err := ChannelListByWorld(worldID)
	if err != nil {
		return err
	}
	for _, ch := range channels {
		if ch == nil || strings.TrimSpace(ch.ID) == "" {
			continue
		}
		if _, err := model.MemberGetByUserIDAndChannelIDBase(userID, ch.ID, "", true); err != nil {
			return err
		}
	}
	return nil
}

func ListWorldFavorites(userID string) ([]string, error) {
	return model.ListWorldFavoriteIDs(userID)
}

func ToggleWorldFavorite(worldID, userID string, favorite bool) ([]string, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil, ErrWorldNotFound
	}
	if !IsWorldMember(worldID, userID) {
		return nil, ErrWorldPermission
	}
	if err := model.SetWorldFavorite(worldID, userID, favorite); err != nil {
		return nil, err
	}
	return model.ListWorldFavoriteIDs(userID)
}

func WorldRemoveMember(worldID, actorID, targetUserID string) error {
	if strings.TrimSpace(targetUserID) == "" {
		return ErrWorldMemberInvalid
	}
	if !IsWorldAdmin(worldID, actorID) {
		return ErrWorldPermission
	}
	if IsWorldOwner(worldID, targetUserID) {
		return ErrWorldOwnerImmutable
	}
	return WorldLeave(worldID, targetUserID)
}

func WorldUpdateMemberRole(worldID, actorID, targetUserID, role string) error {
	role = strings.TrimSpace(role)
	if role != model.WorldRoleAdmin && role != model.WorldRoleMember && role != model.WorldRoleSpectator {
		return ErrWorldMemberInvalid
	}
	if !IsWorldAdmin(worldID, actorID) {
		return ErrWorldPermission
	}
	if IsWorldOwner(worldID, targetUserID) {
		return ErrWorldOwnerImmutable
	}
	db := model.GetDB()
	res := db.Model(&model.WorldMemberModel{}).
		Where("world_id = ? AND user_id = ?", worldID, targetUserID).
		Updates(map[string]any{"role": role, "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrWorldMemberInvalid
	}
	if err := syncWorldChannelRoles(worldID, targetUserID, role); err != nil {
		return err
	}
	return nil
}

func listWorldUserIDsByRoles(worldID string, roles ...string) ([]string, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" || len(roles) == 0 {
		return []string{}, nil
	}
	var ids []string
	err := model.GetDB().Table("world_members").
		Where("world_id = ? AND role IN ?", worldID, roles).
		Pluck("user_id", &ids).Error
	return ids, err
}

func syncWorldRolesForNewChannel(worldID, channelID string) {
	worldID = strings.TrimSpace(worldID)
	channelID = strings.TrimSpace(channelID)
	if worldID == "" || channelID == "" {
		return
	}
	adminIDs, err := listWorldUserIDsByRoles(worldID, model.WorldRoleOwner, model.WorldRoleAdmin)
	if err == nil {
		for _, uid := range adminIDs {
			if _, err := model.MemberGetByUserIDAndChannelIDBase(uid, channelID, "", true); err != nil {
				continue
			}
			_ = ensureChannelRoleLink(uid, channelID, "admin")
		}
	}
	spectatorIDs, err := listWorldUserIDsByRoles(worldID, model.WorldRoleSpectator)
	if err == nil {
		for _, uid := range spectatorIDs {
			if _, err := model.MemberGetByUserIDAndChannelIDBase(uid, channelID, "", true); err != nil {
				continue
			}
			_ = ensureChannelRoleLink(uid, channelID, "spectator")
		}
	}
	// 为公开频道同步 member 角色
	var channel model.ChannelModel
	if err := model.GetDB().Where("id = ?", channelID).Limit(1).Find(&channel).Error; err != nil {
		return
	}
	if strings.ToLower(strings.TrimSpace(channel.PermType)) == "public" {
		memberIDs, err := listWorldUserIDsByRoles(worldID, model.WorldRoleMember)
		if err == nil {
			for _, uid := range memberIDs {
				if _, err := model.MemberGetByUserIDAndChannelIDBase(uid, channelID, "", true); err != nil {
					continue
				}
				_ = ensureChannelRoleLink(uid, channelID, "member")
			}
		}
	}
}

func BackfillWorldRoleAssignments() error {
	db := model.GetDB()
	var worlds []model.WorldModel
	if err := db.Where("status = ?", "active").Find(&worlds).Error; err != nil {
		return err
	}
	for _, world := range worlds {
		var members []model.WorldMemberModel
		if err := db.Where("world_id = ? AND role IN ?", world.ID, []string{model.WorldRoleOwner, model.WorldRoleAdmin, model.WorldRoleSpectator}).
			Find(&members).Error; err != nil {
			return err
		}
		for _, member := range members {
			if _, err := ensureWorldMemberChannelState(world.ID, member.UserID, member.Role); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureWorldMemberChannelState(worldID, userID, role string) (*model.WorldMemberModel, error) {
	if err := ensureWorldChannelMemberships(worldID, userID); err != nil {
		return nil, err
	}
	if err := syncWorldChannelRoles(worldID, userID, role); err != nil {
		return nil, err
	}
	member := &model.WorldMemberModel{}
	if err := model.GetDB().Where("world_id = ? AND user_id = ?", worldID, userID).Limit(1).Find(member).Error; err != nil {
		return nil, err
	}
	return member, nil
}

func normalizeWorldRole(role string) string {
	switch strings.TrimSpace(role) {
	case model.WorldRoleOwner:
		return model.WorldRoleOwner
	case model.WorldRoleAdmin:
		return model.WorldRoleAdmin
	case model.WorldRoleSpectator:
		return model.WorldRoleSpectator
	default:
		return model.WorldRoleMember
	}
}

func syncWorldChannelRoles(worldID, userID, worldRole string) error {
	channels, err := ChannelListByWorld(worldID)
	if err != nil {
		return err
	}
	publicChannelIDs := map[string]struct{}{}
	if worldRole == model.WorldRoleMember {
		publicChannels, err := ChannelListPublicByWorld(worldID)
		if err != nil {
			return err
		}
		for _, ch := range publicChannels {
			if ch == nil || strings.TrimSpace(ch.ID) == "" {
				continue
			}
			publicChannelIDs[ch.ID] = struct{}{}
		}
	}
	for _, ch := range channels {
		if ch == nil || strings.TrimSpace(ch.ID) == "" {
			continue
		}
		switch worldRole {
		case model.WorldRoleOwner, model.WorldRoleAdmin:
			if err := ensureChannelRoleLink(userID, ch.ID, "admin"); err != nil {
				return err
			}
			if err := removeChannelRoleLink(userID, ch.ID, "spectator"); err != nil {
				return err
			}
			if err := removeChannelRoleLink(userID, ch.ID, "member"); err != nil {
				return err
			}
		case model.WorldRoleSpectator:
			if err := ensureChannelRoleLink(userID, ch.ID, "spectator"); err != nil {
				return err
			}
			if err := removeChannelRoleLink(userID, ch.ID, "admin"); err != nil {
				return err
			}
			if err := removeChannelRoleLink(userID, ch.ID, "member"); err != nil {
				return err
			}
		case model.WorldRoleMember:
			// 成员只加入公开频道，非公开频道需要单独授权
			if _, ok := publicChannelIDs[ch.ID]; ok {
				if err := ensureChannelRoleLink(userID, ch.ID, "member"); err != nil {
					return err
				}
			}
			if err := removeChannelRoleLink(userID, ch.ID, "admin"); err != nil {
				return err
			}
			if err := removeChannelRoleLink(userID, ch.ID, "spectator"); err != nil {
				return err
			}
		default:
			if err := removeChannelRoleLink(userID, ch.ID, "admin"); err != nil {
				return err
			}
			if err := removeChannelRoleLink(userID, ch.ID, "spectator"); err != nil {
				return err
			}
		}
	}
	return nil
}

func revokeWorldChannelRoles(worldID, userID string) error {
	channels, err := ChannelListByWorld(worldID)
	if err != nil {
		return err
	}
	for _, ch := range channels {
		if ch == nil || strings.TrimSpace(ch.ID) == "" {
			continue
		}
		if err := removeChannelRoleLink(userID, ch.ID, "admin"); err != nil {
			return err
		}
		if err := removeChannelRoleLink(userID, ch.ID, "spectator"); err != nil {
			return err
		}
	}
	return nil
}

func ensureChannelRoleLink(userID, channelID, roleKey string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(channelID) == "" {
		return nil
	}
	if roleKey == "spectator" {
		ensureChannelSpectatorRole(channelID)
	}
	roleID := fmt.Sprintf("ch-%s-%s", channelID, roleKey)
	_, err := model.UserRoleLink([]string{roleID}, []string{userID})
	return err
}

func removeChannelRoleLink(userID, channelID, roleKey string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(channelID) == "" {
		return nil
	}
	roleID := fmt.Sprintf("ch-%s-%s", channelID, roleKey)
	_, err := model.UserRoleUnlink([]string{roleID}, []string{userID})
	return err
}

func WorldInviteCreate(worldID, creatorID string, ttlMinutes int, maxUse int, memo string, role string) (*model.WorldInviteModel, error) {
	if !IsWorldAdmin(worldID, creatorID) {
		return nil, ErrWorldPermission
	}
	role = normalizeWorldRole(role)
	if role != model.WorldRoleMember && role != model.WorldRoleSpectator {
		return nil, ErrWorldMemberInvalid
	}
	// 合法化参数：负数一律视为无限
	if ttlMinutes < 0 {
		ttlMinutes = 0
	}
	if maxUse < 0 {
		maxUse = 0
	}
	db := model.GetDB()
	if err := db.Model(&model.WorldInviteModel{}).
		Where("world_id = ? AND status = ? AND role = ?", worldID, "active", role).
		Updates(map[string]any{"status": "archived", "updated_at": time.Now()}).Error; err != nil {
		return nil, err
	}
	invite := &model.WorldInviteModel{
		WorldID:   worldID,
		CreatorID: creatorID,
		Role:      role,
		MaxUse:    maxUse,
		Memo:      memo,
		Status:    "active",
	}
	if ttlMinutes > 0 {
		expire := time.Now().Add(time.Duration(ttlMinutes) * time.Minute)
		invite.ExpireAt = &expire
	}
	if err := db.Create(invite).Error; err != nil {
		return nil, err
	}
	return invite, nil
}

func WorldInviteConsume(slug, userID string) (*model.WorldInviteModel, *model.WorldModel, *model.WorldMemberModel, bool, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, nil, nil, false, ErrWorldInviteInvalid
	}
	db := model.GetDB()
	var invite model.WorldInviteModel
	if err := db.Where("slug = ? AND status = ?", slug, "active").Limit(1).Find(&invite).Error; err != nil {
		return nil, nil, nil, false, err
	}
	if invite.ID == "" {
		return nil, nil, nil, false, ErrWorldInviteInvalid
	}
	now := time.Now()
	markInviteArchived := func() {
		_ = db.Model(&model.WorldInviteModel{}).
			Where("id = ?", invite.ID).
			Updates(map[string]any{"status": "archived", "updated_at": now}).Error
	}
	if invite.ExpireAt != nil && invite.ExpireAt.Before(now) {
		markInviteArchived()
		return nil, nil, nil, false, ErrWorldInviteInvalid
	}
	if invite.MaxUse > 0 && invite.UsedCount >= invite.MaxUse {
		markInviteArchived()
		return nil, nil, nil, false, ErrWorldInviteInvalid
	}
	world, err := GetWorldByID(invite.WorldID)
	if err != nil {
		return nil, nil, nil, false, err
	}
	existingMember := &model.WorldMemberModel{}
	_ = db.Where("world_id = ? AND user_id = ?", invite.WorldID, userID).Limit(1).Find(existingMember).Error
	wasMember := existingMember.ID != ""
	role := normalizeWorldRole(invite.Role)
	member, err := WorldJoin(invite.WorldID, userID, role)
	if err != nil {
		return nil, nil, nil, false, err
	}
	alreadyJoined := wasMember
	if !wasMember {
		_ = db.Model(&model.WorldInviteModel{}).
			Where("id = ?", invite.ID).
			Updates(map[string]any{"used_count": gorm.Expr("used_count + 1"), "updated_at": time.Now()}).Error
	}
	invite.Role = role
	return &invite, world, member, alreadyJoined, nil
}
