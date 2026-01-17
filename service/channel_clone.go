package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mikespook/gorbac"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

type ChannelCopyOptions struct {
	CopyRoles       bool `json:"copyRoles"`
	CopyMembers     bool `json:"copyMembers"`
	CopyIdentities  bool `json:"copyIdentities"`
	CopyStickyNotes bool `json:"copyStickyNotes"`
	CopyGallery     bool `json:"copyGallery"`
	CopyIForms      bool `json:"copyIForms"`
	CopyDiceMacros  bool `json:"copyDiceMacros"`
	CopyAudioScenes bool `json:"copyAudioScenes"`
	CopyAudioState  bool `json:"copyAudioState"`
	CopyWebhooks    bool `json:"copyWebhooks"`
}

type ChannelCopyParams struct {
	Name     string             `json:"name"`
	WorldID  string             `json:"worldId"`
	ParentID string             `json:"parentId"`
	Options  ChannelCopyOptions `json:"options"`
}

type ChannelCopySummary struct {
	Copied  []string `json:"copied"`
	Skipped []string `json:"skipped"`
}

type ChannelCopyResult struct {
	ChannelID   string             `json:"channelId"`
	Summary     ChannelCopySummary `json:"summary"`
	IdentityMap map[string]string  `json:"identityMap,omitempty"`
}

func (s *ChannelCopySummary) addCopied(item string) {
	if strings.TrimSpace(item) == "" {
		return
	}
	s.Copied = append(s.Copied, item)
}

func (s *ChannelCopySummary) addSkipped(item string) {
	if strings.TrimSpace(item) == "" {
		return
	}
	s.Skipped = append(s.Skipped, item)
}

func ChannelClone(sourceChannelID string, actor *model.UserModel, params ChannelCopyParams) (*ChannelCopyResult, error) {
	sourceChannelID = strings.TrimSpace(sourceChannelID)
	if sourceChannelID == "" {
		return nil, errors.New("频道ID不能为空")
	}
	if actor == nil || strings.TrimSpace(actor.ID) == "" {
		return nil, errors.New("未登录")
	}

	source, err := model.ChannelGet(sourceChannelID)
	if err != nil {
		return nil, err
	}
	if source == nil || strings.TrimSpace(source.ID) == "" {
		return nil, errors.New("频道不存在")
	}
	if source.IsPrivate || strings.EqualFold(strings.TrimSpace(source.PermType), "private") || strings.Contains(source.ID, ":") {
		return nil, errors.New("私聊频道不可复制")
	}
	if !pm.CanWithChannelRole(actor.ID, source.ID, pm.PermFuncChannelManageInfo, pm.PermFuncChannelManageRoleRoot) &&
		!pm.CanWithSystemRole(actor.ID, pm.PermModAdmin) {
		return nil, errors.New("没有复制频道的权限")
	}

	targetWorldID := strings.TrimSpace(params.WorldID)
	if targetWorldID == "" {
		targetWorldID = strings.TrimSpace(source.WorldID)
	}
	if targetWorldID == "" {
		return nil, errors.New("worldId 不能为空")
	}
	if !IsWorldAdmin(targetWorldID, actor.ID) && !pm.CanWithSystemRole(actor.ID, pm.PermModAdmin) {
		return nil, errors.New("没有目标世界的管理权限")
	}

	targetName := strings.TrimSpace(params.Name)
	if targetName == "" {
		base := strings.TrimSpace(source.Name)
		if base == "" {
			base = "未命名频道"
		}
		targetName = base + "-副本"
	}

	parentID := strings.TrimSpace(params.ParentID)
	if parentID == "" && strings.TrimSpace(source.WorldID) == targetWorldID {
		parentID = strings.TrimSpace(source.ParentID)
	}
	if parentID != "" {
		if parentID == source.ID {
			return nil, errors.New("不能复制到源频道的子频道")
		}
		parent, err := model.ChannelGet(parentID)
		if err != nil {
			return nil, err
		}
		if parent == nil || strings.TrimSpace(parent.ID) == "" {
			return nil, errors.New("父频道不存在")
		}
		if strings.TrimSpace(parent.WorldID) != targetWorldID {
			return nil, errors.New("父频道不属于目标世界")
		}
	}

	newChannel := ChannelNew(utils.NewID(), source.PermType, targetName, targetWorldID, actor.ID, parentID)
	if newChannel == nil || strings.TrimSpace(newChannel.ID) == "" {
		return nil, errors.New("创建频道失败")
	}

	if err := model.GetDB().Model(&model.ChannelModel{}).
		Where("id = ?", newChannel.ID).
		Updates(map[string]any{
			"note":                     source.Note,
			"sort_order":               source.SortOrder,
			"default_dice_expr":        source.DefaultDiceExpr,
			"built_in_dice_enabled":    source.BuiltInDiceEnabled,
			"bot_feature_enabled":      source.BotFeatureEnabled,
			"background_attachment_id": source.BackgroundAttachmentId,
			"background_settings":      source.BackgroundSettings,
		}).Error; err != nil {
		cleanupClonedChannel(newChannel.ID)
		return nil, err
	}

	summary := ChannelCopySummary{}
	tx := model.GetDB().Begin()
	if tx.Error != nil {
		cleanupClonedChannel(newChannel.ID)
		return nil, tx.Error
	}

	roleMap := map[string]string{}
	rolePerms := map[string][]string{}
	sceneMap := map[string]string{}
	var identityMap map[string]string
	var botTokens []*model.BotTokenModel
	var botUserIDs []string

	if params.Options.CopyRoles {
		roleMap, rolePerms, err = copyChannelRoles(tx, source.ID, newChannel.ID, &summary)
		if err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("roles")
	}

	var allowedUserIDs map[string]struct{}
	if strings.TrimSpace(source.WorldID) != targetWorldID {
		allowedUserIDs, err = loadWorldMemberSet(tx, targetWorldID)
		if err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	}

	if params.Options.CopyMembers {
		if err := copyChannelMembers(tx, source.ID, newChannel.ID, roleMap, allowedUserIDs, &summary); err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("members")
	}

	if params.Options.CopyIdentities {
		identityMap, err = copyChannelIdentities(tx, source.ID, newChannel.ID, allowedUserIDs, &summary)
		if err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("identities")
	}

	if params.Options.CopyStickyNotes {
		if err := copyChannelStickyNotes(tx, source.ID, newChannel.ID, targetWorldID, &summary); err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("stickyNotes")
	}

	if params.Options.CopyGallery {
		if err := copyChannelGallery(tx, source.ID, newChannel.ID, &summary); err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("gallery")
	}

	if params.Options.CopyIForms {
		if err := copyChannelIForms(tx, source.ID, newChannel.ID, actor.ID, &summary); err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("iforms")
	}

	if params.Options.CopyDiceMacros {
		if err := copyChannelDiceMacros(tx, source.ID, newChannel.ID, allowedUserIDs, &summary); err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("diceMacros")
	}

	if params.Options.CopyAudioScenes {
		sceneMap, err = copyChannelAudioScenes(tx, source.ID, newChannel.ID, &summary)
		if err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("audioScenes")
	}

	if params.Options.CopyAudioState {
		if err := copyChannelAudioState(tx, source.ID, newChannel.ID, sceneMap, actor.ID, &summary); err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("audioState")
	}

	if params.Options.CopyWebhooks {
		botTokens, botUserIDs, err = copyChannelWebhooks(tx, source.ID, newChannel.ID, actor.ID, &summary)
		if err != nil {
			tx.Rollback()
			cleanupClonedChannel(newChannel.ID)
			return nil, err
		}
	} else {
		summary.addSkipped("webhooks")
	}

	if err := tx.Commit().Error; err != nil {
		cleanupClonedChannel(newChannel.ID)
		return nil, err
	}

	applyRolePermsToMemory(rolePerms)
	for _, token := range botTokens {
		_ = SyncBotUserProfile(token)
		_ = SyncBotMembers(token)
	}
	for _, botID := range botUserIDs {
		_ = EnsureBotChannelIdentity(botID, newChannel.ID)
	}

	return &ChannelCopyResult{
		ChannelID:   newChannel.ID,
		Summary:     summary,
		IdentityMap: identityMap,
	}, nil
}

func cleanupClonedChannel(channelID string) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return
	}
	db := model.GetDB()
	var noteIDs []string
	db.Model(&model.StickyNoteModel{}).Where("channel_id = ?", channelID).Pluck("id", &noteIDs)
	if len(noteIDs) > 0 {
		db.Where("sticky_note_id IN ?", noteIDs).Delete(&model.StickyNoteUserStateModel{})
	}
	db.Where("channel_id = ?", channelID).Delete(&model.StickyNoteModel{})

	db.Where("channel_id = ?", channelID).Delete(&model.MemberModel{})
	db.Where("channel_id = ?", channelID).Delete(&model.ChannelIdentityModel{})
	db.Where("channel_id = ?", channelID).Delete(&model.ChannelIdentityFolderModel{})
	db.Where("channel_id = ?", channelID).Delete(&model.ChannelIdentityFolderMemberModel{})
	db.Where("channel_id = ?", channelID).Delete(&model.ChannelIdentityFolderFavoriteModel{})

	var colIDs []string
	db.Model(&model.GalleryCollection{}).
		Where("owner_type = ? AND owner_id = ?", model.OwnerTypeChannel, channelID).
		Pluck("id", &colIDs)
	if len(colIDs) > 0 {
		db.Where("collection_id IN ?", colIDs).Delete(&model.GalleryItem{})
	}
	db.Where("owner_type = ? AND owner_id = ?", model.OwnerTypeChannel, channelID).
		Delete(&model.GalleryCollection{})

	db.Where("channel_id = ?", channelID).Delete(&model.ChannelIFormModel{})
	db.Where("channel_id = ?", channelID).Delete(&model.DiceMacroModel{})
	db.Where("channel_scope = ?", channelID).Delete(&model.AudioScene{})
	db.Where("channel_id = ?", channelID).Delete(&model.AudioPlaybackState{})
	db.Where("channel_id = ?", channelID).Delete(&model.ChannelWebhookIntegrationModel{})

	rolePattern := fmt.Sprintf("ch-%s-%%", channelID)
	db.Where("role_id LIKE ?", rolePattern).Delete(&model.UserRoleMappingModel{})
	db.Where("role_id LIKE ?", rolePattern).Delete(&model.RolePermissionModel{})
	db.Where("id LIKE ?", rolePattern).Delete(&model.ChannelRoleModel{})
	db.Where("id = ?", channelID).Delete(&model.ChannelModel{})
}

func loadWorldMemberSet(tx *gorm.DB, worldID string) (map[string]struct{}, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil, nil
	}
	var ids []string
	if err := tx.Table("world_members").Where("world_id = ?", worldID).Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			set[id] = struct{}{}
		}
	}
	return set, nil
}

func applyRolePermsToMemory(rolePerms map[string][]string) {
	for roleID, permIDs := range rolePerms {
		if strings.TrimSpace(roleID) == "" {
			continue
		}
		perms := make([]gorbac.Permission, 0, len(permIDs))
		for _, permID := range permIDs {
			if strings.TrimSpace(permID) == "" {
				continue
			}
			perms = append(perms, gorbac.NewStdPermission(permID))
		}
		pm.ChannelRoleSetWithoutDB(roleID, perms)
	}
}

func copyChannelRoles(tx *gorm.DB, sourceID, targetID string, summary *ChannelCopySummary) (map[string]string, map[string][]string, error) {
	var roles []model.ChannelRoleModel
	if err := tx.Where("channel_id = ?", sourceID).Find(&roles).Error; err != nil {
		return nil, nil, err
	}
	if len(roles) == 0 {
		summary.addCopied("roles")
		return map[string]string{}, map[string][]string{}, nil
	}

	roleMap := map[string]string{}
	targetRoleIDs := make([]string, 0, len(roles))
	for _, role := range roles {
		key, ok := extractRoleKey(role.ID, sourceID)
		if !ok {
			continue
		}
		newRoleID := fmt.Sprintf("ch-%s-%s", targetID, key)
		roleMap[role.ID] = newRoleID
		targetRoleIDs = append(targetRoleIDs, newRoleID)

		var existing model.ChannelRoleModel
		if err := tx.Where("id = ?", newRoleID).Limit(1).Find(&existing).Error; err != nil {
			return nil, nil, err
		}
		if existing.ID == "" {
			clone := model.ChannelRoleModel{
				StringPKBaseModel: model.StringPKBaseModel{ID: newRoleID},
				Name:              role.Name,
				Desc:              role.Desc,
				ChannelID:         targetID,
			}
			if err := tx.Create(&clone).Error; err != nil {
				return nil, nil, err
			}
		} else {
			if err := tx.Model(&model.ChannelRoleModel{}).Where("id = ?", newRoleID).
				Updates(map[string]any{"name": role.Name, "desc": role.Desc}).Error; err != nil {
				return nil, nil, err
			}
		}
	}

	var perms []model.RolePermissionModel
	sourceRoleIDs := make([]string, 0, len(roleMap))
	for roleID := range roleMap {
		sourceRoleIDs = append(sourceRoleIDs, roleID)
	}
	if len(sourceRoleIDs) > 0 {
		if err := tx.Where("role_id IN ?", sourceRoleIDs).Find(&perms).Error; err != nil {
			return nil, nil, err
		}
	}

	if len(targetRoleIDs) > 0 {
		if err := tx.Where("role_id IN ?", targetRoleIDs).Delete(&model.RolePermissionModel{}).Error; err != nil {
			return nil, nil, err
		}
	}

	rolePerms := map[string][]string{}
	if len(perms) > 0 {
		clones := make([]model.RolePermissionModel, 0, len(perms))
		for _, perm := range perms {
			newRoleID := roleMap[perm.RoleID]
			if newRoleID == "" {
				continue
			}
			rolePerms[newRoleID] = append(rolePerms[newRoleID], perm.PermissionID)
			clones = append(clones, model.RolePermissionModel{
				StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
				RoleID:            newRoleID,
				PermissionID:      perm.PermissionID,
			})
		}
		if len(clones) > 0 {
			if err := tx.Create(&clones).Error; err != nil {
				return nil, nil, err
			}
		}
	}

	summary.addCopied("roles")
	return roleMap, rolePerms, nil
}

func extractRoleKey(roleID, channelID string) (string, bool) {
	prefix := "ch-" + channelID + "-"
	if !strings.HasPrefix(roleID, prefix) {
		return "", false
	}
	return strings.TrimPrefix(roleID, prefix), true
}

func copyChannelMembers(tx *gorm.DB, sourceID, targetID string, roleMap map[string]string, allowedUserIDs map[string]struct{}, summary *ChannelCopySummary) error {
	var members []model.MemberModel
	if err := tx.Where("channel_id = ?", sourceID).Find(&members).Error; err != nil {
		return err
	}

	for _, member := range members {
		if member.UserID == "" {
			continue
		}
		if allowedUserIDs != nil {
			if _, ok := allowedUserIDs[member.UserID]; !ok {
				continue
			}
		}
		var existing model.MemberModel
		if err := tx.Where("user_id = ? AND channel_id = ?", member.UserID, targetID).Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if existing.ID != "" {
			continue
		}
		clone := model.MemberModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			Nickname:          member.Nickname,
			ChannelID:         targetID,
			UserID:            member.UserID,
			RecentSentAt:      member.RecentSentAt,
		}
		if err := tx.Create(&clone).Error; err != nil {
			return err
		}
	}

	targetRoleIDs := map[string]struct{}{}
	if len(roleMap) == 0 {
		var roles []model.ChannelRoleModel
		if err := tx.Where("channel_id = ?", targetID).Find(&roles).Error; err != nil {
			return err
		}
		for _, role := range roles {
			if role.ID != "" {
				targetRoleIDs[role.ID] = struct{}{}
			}
		}
	} else {
		for _, id := range roleMap {
			if id != "" {
				targetRoleIDs[id] = struct{}{}
			}
		}
	}

	var mappings []model.UserRoleMappingModel
	if err := tx.Where("role_type = ?", "channel").
		Where("role_id LIKE ?", "ch-"+sourceID+"-%").
		Find(&mappings).Error; err != nil {
		return err
	}
	if len(mappings) == 0 {
		summary.addCopied("members")
		return nil
	}

	clones := make([]model.UserRoleMappingModel, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.UserID == "" || mapping.RoleID == "" {
			continue
		}
		if allowedUserIDs != nil {
			if _, ok := allowedUserIDs[mapping.UserID]; !ok {
				continue
			}
		}
		_, ok := extractRoleKey(mapping.RoleID, sourceID)
		if !ok {
			continue
		}
		targetRoleID := strings.Replace(mapping.RoleID, "ch-"+sourceID+"-", "ch-"+targetID+"-", 1)
		if _, exists := targetRoleIDs[targetRoleID]; !exists {
			continue
		}
		clones = append(clones, model.UserRoleMappingModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			RoleType:          "channel",
			UserID:            mapping.UserID,
			RoleID:            targetRoleID,
		})
	}
	if len(clones) > 0 {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&clones).Error; err != nil {
			return err
		}
	}

	summary.addCopied("members")
	return nil
}

func copyChannelIdentities(tx *gorm.DB, sourceID, targetID string, allowedUserIDs map[string]struct{}, summary *ChannelCopySummary) (map[string]string, error) {
	var identities []model.ChannelIdentityModel
	if err := tx.Where("channel_id = ?", sourceID).Find(&identities).Error; err != nil {
		return nil, err
	}
	identityMap := map[string]string{}
	for _, identity := range identities {
		if identity.UserID == "" {
			continue
		}
		if allowedUserIDs != nil {
			if _, ok := allowedUserIDs[identity.UserID]; !ok {
				continue
			}
		}
		newID := utils.NewID()
		identityMap[identity.ID] = newID
		clone := model.ChannelIdentityModel{
			StringPKBaseModel:  model.StringPKBaseModel{ID: newID},
			ChannelID:          targetID,
			UserID:             identity.UserID,
			DisplayName:        identity.DisplayName,
			Color:              identity.Color,
			AvatarAttachmentID: identity.AvatarAttachmentID,
			IsDefault:          identity.IsDefault,
			IsHidden:           identity.IsHidden,
			SortOrder:          identity.SortOrder,
		}
		if err := tx.Create(&clone).Error; err != nil {
			return nil, err
		}
	}

	var folders []model.ChannelIdentityFolderModel
	if err := tx.Where("channel_id = ?", sourceID).Find(&folders).Error; err != nil {
		return nil, err
	}
	folderMap := map[string]string{}
	for _, folder := range folders {
		if folder.UserID == "" {
			continue
		}
		if allowedUserIDs != nil {
			if _, ok := allowedUserIDs[folder.UserID]; !ok {
				continue
			}
		}
		newID := utils.NewID()
		folderMap[folder.ID] = newID
		clone := model.ChannelIdentityFolderModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: newID},
			ChannelID:         targetID,
			UserID:            folder.UserID,
			Name:              folder.Name,
			SortOrder:         folder.SortOrder,
		}
		if err := tx.Create(&clone).Error; err != nil {
			return nil, err
		}
	}

	var members []model.ChannelIdentityFolderMemberModel
	if err := tx.Where("channel_id = ?", sourceID).Find(&members).Error; err != nil {
		return nil, err
	}
	for _, member := range members {
		if allowedUserIDs != nil {
			if _, ok := allowedUserIDs[member.UserID]; !ok {
				continue
			}
		}
		newFolderID := folderMap[member.FolderID]
		newIdentityID := identityMap[member.IdentityID]
		if newFolderID == "" || newIdentityID == "" {
			continue
		}
		clone := model.ChannelIdentityFolderMemberModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			ChannelID:         targetID,
			UserID:            member.UserID,
			FolderID:          newFolderID,
			IdentityID:        newIdentityID,
			SortOrder:         member.SortOrder,
		}
		if err := tx.Create(&clone).Error; err != nil {
			return nil, err
		}
	}

	var favorites []model.ChannelIdentityFolderFavoriteModel
	if err := tx.Where("channel_id = ?", sourceID).Find(&favorites).Error; err != nil {
		return nil, err
	}
	for _, favorite := range favorites {
		if allowedUserIDs != nil {
			if _, ok := allowedUserIDs[favorite.UserID]; !ok {
				continue
			}
		}
		newFolderID := folderMap[favorite.FolderID]
		if newFolderID == "" {
			continue
		}
		clone := model.ChannelIdentityFolderFavoriteModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			ChannelID:         targetID,
			UserID:            favorite.UserID,
			FolderID:          newFolderID,
		}
		if err := tx.Create(&clone).Error; err != nil {
			return nil, err
		}
	}

	summary.addCopied("identities")
	return identityMap, nil
}

func copyChannelStickyNotes(tx *gorm.DB, sourceID, targetID, targetWorldID string, summary *ChannelCopySummary) error {
	var notes []model.StickyNoteModel
	if err := tx.Where("channel_id = ? AND is_deleted = ?", sourceID, false).Find(&notes).Error; err != nil {
		return err
	}
	noteMap := map[string]string{}
	for _, note := range notes {
		newID := utils.NewID()
		noteMap[note.ID] = newID
		clone := note
		clone.StringPKBaseModel = model.StringPKBaseModel{ID: newID}
		clone.ChannelID = targetID
		clone.WorldID = targetWorldID
		if err := tx.Create(&clone).Error; err != nil {
			return err
		}
	}

	if len(noteMap) > 0 {
		var states []model.StickyNoteUserStateModel
		noteIDs := make([]string, 0, len(noteMap))
		for id := range noteMap {
			noteIDs = append(noteIDs, id)
		}
		if err := tx.Where("sticky_note_id IN ?", noteIDs).Find(&states).Error; err != nil {
			return err
		}
		for _, state := range states {
			newNoteID := noteMap[state.StickyNoteID]
			if newNoteID == "" {
				continue
			}
			clone := state
			clone.StringPKBaseModel = model.StringPKBaseModel{ID: utils.NewID()}
			clone.StickyNoteID = newNoteID
			if err := tx.Create(&clone).Error; err != nil {
				return err
			}
		}
	}

	summary.addCopied("stickyNotes")
	return nil
}

func copyChannelGallery(tx *gorm.DB, sourceID, targetID string, summary *ChannelCopySummary) error {
	var collections []model.GalleryCollection
	if err := tx.Where("owner_type = ? AND owner_id = ?", model.OwnerTypeChannel, sourceID).
		Order("`order`").Find(&collections).Error; err != nil {
		return err
	}
	collectionMap := map[string]string{}
	for _, col := range collections {
		newID := utils.NewID()
		collectionMap[col.ID] = newID
		clone := model.GalleryCollection{
			StringPKBaseModel: model.StringPKBaseModel{ID: newID},
			OwnerType:         model.OwnerTypeChannel,
			OwnerID:           targetID,
			Name:              col.Name,
			Order:             col.Order,
			QuotaUsed:         col.QuotaUsed,
			CreatedBy:         col.CreatedBy,
			UpdatedBy:         col.UpdatedBy,
		}
		if err := tx.Create(&clone).Error; err != nil {
			return err
		}
	}

	if len(collectionMap) > 0 {
		var items []model.GalleryItem
		sourceIDs := make([]string, 0, len(collectionMap))
		for id := range collectionMap {
			sourceIDs = append(sourceIDs, id)
		}
		if err := tx.Where("collection_id IN ?", sourceIDs).Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			newCollectionID := collectionMap[item.CollectionID]
			if newCollectionID == "" {
				continue
			}
			clone := model.GalleryItem{
				StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
				CollectionID:      newCollectionID,
				AttachmentID:      item.AttachmentID,
				ThumbURL:          item.ThumbURL,
				Remark:            item.Remark,
				Tags:              item.Tags,
				Order:             item.Order,
				CreatedBy:         item.CreatedBy,
				Size:              item.Size,
			}
			if err := tx.Create(&clone).Error; err != nil {
				return err
			}
		}
	}

	summary.addCopied("gallery")
	return nil
}

func copyChannelIForms(tx *gorm.DB, sourceID, targetID, actorID string, summary *ChannelCopySummary) error {
	var forms []model.ChannelIFormModel
	if err := tx.Where("channel_id = ?", sourceID).Find(&forms).Error; err != nil {
		return err
	}
	for _, form := range forms {
		clone := form
		clone.StringPKBaseModel = model.StringPKBaseModel{ID: utils.NewID()}
		clone.ChannelID = targetID
		clone.CreatedBy = actorID
		clone.UpdatedBy = actorID
		if err := tx.Create(&clone).Error; err != nil {
			return err
		}
	}
	summary.addCopied("iforms")
	return nil
}

func copyChannelDiceMacros(tx *gorm.DB, sourceID, targetID string, allowedUserIDs map[string]struct{}, summary *ChannelCopySummary) error {
	var macros []model.DiceMacroModel
	if err := tx.Where("channel_id = ?", sourceID).Find(&macros).Error; err != nil {
		return err
	}
	for _, macro := range macros {
		if macro.UserID == "" {
			continue
		}
		if allowedUserIDs != nil {
			if _, ok := allowedUserIDs[macro.UserID]; !ok {
				continue
			}
		}
		clone := macro
		clone.StringPKBaseModel = model.StringPKBaseModel{ID: utils.NewID()}
		clone.ChannelID = targetID
		if err := tx.Create(&clone).Error; err != nil {
			return err
		}
	}
	summary.addCopied("diceMacros")
	return nil
}

func copyChannelAudioScenes(tx *gorm.DB, sourceID, targetID string, summary *ChannelCopySummary) (map[string]string, error) {
	var scenes []model.AudioScene
	if err := tx.Where("channel_scope = ?", sourceID).Find(&scenes).Error; err != nil {
		return nil, err
	}
	sceneMap := map[string]string{}
	for _, scene := range scenes {
		newID := utils.NewID()
		sceneMap[scene.ID] = newID
		clone := scene
		clone.StringPKBaseModel = model.StringPKBaseModel{ID: newID}
		clone.ChannelScope = &targetID
		if err := tx.Create(&clone).Error; err != nil {
			return nil, err
		}
	}
	summary.addCopied("audioScenes")
	return sceneMap, nil
}

func copyChannelAudioState(tx *gorm.DB, sourceID, targetID string, sceneMap map[string]string, actorID string, summary *ChannelCopySummary) error {
	var state model.AudioPlaybackState
	if err := tx.Where("channel_id = ?", sourceID).Limit(1).Find(&state).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			summary.addCopied("audioState")
			return nil
		}
		return err
	}
	if strings.TrimSpace(state.ChannelID) == "" {
		summary.addCopied("audioState")
		return nil
	}
	clone := state
	clone.ChannelID = targetID
	if clone.SceneID != nil {
		if mapped := sceneMap[*clone.SceneID]; mapped != "" {
			clone.SceneID = &mapped
		} else {
			clone.SceneID = nil
		}
	}
	clone.UpdatedBy = actorID
	clone.UpdatedAt = time.Now()
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = time.Now()
	}
	if err := tx.Create(&clone).Error; err != nil {
		return err
	}
	summary.addCopied("audioState")
	return nil
}

func copyChannelWebhooks(tx *gorm.DB, sourceID, targetID, actorID string, summary *ChannelCopySummary) ([]*model.BotTokenModel, []string, error) {
	var integrations []model.ChannelWebhookIntegrationModel
	if err := tx.Where("channel_id = ? AND status = ?", sourceID, model.WebhookIntegrationStatusActive).
		Find(&integrations).Error; err != nil {
		return nil, nil, err
	}
	if len(integrations) == 0 {
		summary.addCopied("webhooks")
		return nil, nil, nil
	}

	now := time.Now()
	botTokens := make([]*model.BotTokenModel, 0, len(integrations))
	botUserIDs := make([]string, 0, len(integrations))
	for _, integration := range integrations {
		name := strings.TrimSpace(integration.Name)
		if name == "" {
			name = "Webhook"
		}
		source := strings.TrimSpace(integration.Source)
		if source == "" {
			source = "external"
		}
		botID := utils.NewID()
		user := &model.UserModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: botID},
			Username:          utils.NewID(),
			Nickname:          name,
			Password:          "",
			Salt:              "BOT_SALT",
			IsBot:             true,
			Avatar:            "",
			NickColor:         "",
		}
		if err := tx.Create(user).Error; err != nil {
			return nil, nil, err
		}

		tokenValue := utils.NewIDWithLength(32)
		token := &model.BotTokenModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: botID},
			Name:              name,
			Avatar:            "",
			NickColor:         "",
			Token:             tokenValue,
			ExpiresAt:         now.UnixMilli() + 3*365*24*60*60*1e3,
		}
		if err := tx.Create(token).Error; err != nil {
			return nil, nil, err
		}

		if err := tx.Create(&model.MemberModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			Nickname:          name,
			ChannelID:         targetID,
			UserID:            botID,
		}).Error; err != nil {
			return nil, nil, err
		}

		caps := integration.Capabilities()
		capsJSON, _ := json.Marshal(caps)
		item := &model.ChannelWebhookIntegrationModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			ChannelID:         targetID,
			Name:              name,
			BotUserID:         botID,
			Source:            source,
			CapabilitiesJSON:  string(capsJSON),
			Status:            model.WebhookIntegrationStatusActive,
			CreatedBy:         actorID,
			LastUsedAt:        0,
			TokenTailFragment: tailFragment(tokenValue),
		}
		if err := tx.Create(item).Error; err != nil {
			return nil, nil, err
		}

		botTokens = append(botTokens, token)
		botUserIDs = append(botUserIDs, botID)
	}

	summary.addCopied("webhooks")
	return botTokens, botUserIDs, nil
}

func tailFragment(token string) string {
	if len(token) < 6 {
		return token
	}
	return token[len(token)-6:]
}
