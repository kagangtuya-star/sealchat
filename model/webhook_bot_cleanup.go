package model

import (
	"sort"
	"strings"

	"gorm.io/gorm"
)

type WebhookBotFriendCleanupStats struct {
	WebhookBotCount             int64 `json:"webhookBotCount"`
	ActiveReferenceSkippedCount int64 `json:"activeReferenceSkippedCount"`
	UserRoleMappingDeleted      int64 `json:"userRoleMappingDeleted"`
	MemberDeleted               int64 `json:"memberDeleted"`
	WorldMemberDeleted          int64 `json:"worldMemberDeleted"`
	FriendRelationDeleted       int64 `json:"friendRelationDeleted"`
	PrivateChannelDeleted       int64 `json:"privateChannelDeleted"`
	UserDeleted                 int64 `json:"userDeleted"`
	BotTokenDeleted             int64 `json:"botTokenDeleted"`
}

func addWebhookBotCleanupStats(dst, src *WebhookBotFriendCleanupStats) {
	if dst == nil || src == nil {
		return
	}
	dst.WebhookBotCount += src.WebhookBotCount
	dst.ActiveReferenceSkippedCount += src.ActiveReferenceSkippedCount
	dst.UserRoleMappingDeleted += src.UserRoleMappingDeleted
	dst.MemberDeleted += src.MemberDeleted
	dst.WorldMemberDeleted += src.WorldMemberDeleted
	dst.FriendRelationDeleted += src.FriendRelationDeleted
	dst.PrivateChannelDeleted += src.PrivateChannelDeleted
	dst.UserDeleted += src.UserDeleted
	dst.BotTokenDeleted += src.BotTokenDeleted
}

func activeSystemBotReferenceCountTx(tx *gorm.DB, botUserID string) (int64, error) {
	botUserID = strings.TrimSpace(botUserID)
	if botUserID == "" {
		return 0, nil
	}

	var webhookCount int64
	if err := tx.Model(&ChannelWebhookIntegrationModel{}).
		Where("bot_user_id = ? AND status = ?", botUserID, WebhookIntegrationStatusActive).
		Count(&webhookCount).Error; err != nil {
		return 0, err
	}

	var digestCount int64
	if err := tx.Model(&DigestWebhookIntegrationModel{}).
		Where("bot_user_id = ? AND status = ?", botUserID, WebhookIntegrationStatusActive).
		Count(&digestCount).Error; err != nil {
		return 0, err
	}

	return webhookCount + digestCount, nil
}

func ActiveSystemBotReferenceCount(botUserID string) (int64, error) {
	if db == nil {
		return 0, nil
	}
	return activeSystemBotReferenceCountTx(db, botUserID)
}

func cleanupSystemBotFriendDataTx(tx *gorm.DB, botUserID string, stats *WebhookBotFriendCleanupStats) error {
	type friendCleanupRow struct {
		ID string
	}

	var friendRows []friendCleanupRow
	if err := tx.Model(&FriendModel{}).
		Select("id").
		Where("user_id1 = ? OR user_id2 = ?", botUserID, botUserID).
		Find(&friendRows).Error; err != nil {
		return err
	}

	friendChannelIDs := make([]string, 0, len(friendRows))
	for _, row := range friendRows {
		id := strings.TrimSpace(row.ID)
		if id == "" {
			continue
		}
		friendChannelIDs = append(friendChannelIDs, id)
	}

	friendDeleteResult := tx.Unscoped().
		Where("user_id1 = ? OR user_id2 = ?", botUserID, botUserID).
		Delete(&FriendModel{})
	if friendDeleteResult.Error != nil {
		return friendDeleteResult.Error
	}
	stats.FriendRelationDeleted += friendDeleteResult.RowsAffected

	if len(friendChannelIDs) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	uniqueChannelIDs := make([]string, 0, len(friendChannelIDs))
	for _, id := range friendChannelIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueChannelIDs = append(uniqueChannelIDs, id)
	}

	channelDeleteResult := tx.Unscoped().
		Where("id IN ? AND (is_private = ? OR perm_type = ?)", uniqueChannelIDs, true, "private").
		Delete(&ChannelModel{})
	if channelDeleteResult.Error != nil {
		return channelDeleteResult.Error
	}
	stats.PrivateChannelDeleted += channelDeleteResult.RowsAffected
	return nil
}

func cleanupOrphanSystemBotDataTx(tx *gorm.DB, botUserID string) (*WebhookBotFriendCleanupStats, error) {
	botUserID = strings.TrimSpace(botUserID)
	stats := &WebhookBotFriendCleanupStats{}
	if botUserID == "" {
		return stats, nil
	}

	refCount, err := activeSystemBotReferenceCountTx(tx, botUserID)
	if err != nil {
		return nil, err
	}
	if refCount > 0 {
		stats.ActiveReferenceSkippedCount = 1
		return stats, nil
	}

	if err := cleanupSystemBotFriendDataTx(tx, botUserID, stats); err != nil {
		return nil, err
	}

	userRoleDeleteResult := tx.Unscoped().Where("user_id = ?", botUserID).Delete(&UserRoleMappingModel{})
	if userRoleDeleteResult.Error != nil {
		return nil, userRoleDeleteResult.Error
	}
	stats.UserRoleMappingDeleted += userRoleDeleteResult.RowsAffected

	memberDeleteResult := tx.Unscoped().Where("user_id = ?", botUserID).Delete(&MemberModel{})
	if memberDeleteResult.Error != nil {
		return nil, memberDeleteResult.Error
	}
	stats.MemberDeleted += memberDeleteResult.RowsAffected

	worldMemberDeleteResult := tx.Unscoped().Where("user_id = ?", botUserID).Delete(&WorldMemberModel{})
	if worldMemberDeleteResult.Error != nil {
		return nil, worldMemberDeleteResult.Error
	}
	stats.WorldMemberDeleted += worldMemberDeleteResult.RowsAffected

	userDeleteResult := tx.Unscoped().Where("id = ?", botUserID).Delete(&UserModel{})
	if userDeleteResult.Error != nil {
		return nil, userDeleteResult.Error
	}
	stats.UserDeleted += userDeleteResult.RowsAffected

	botTokenDeleteResult := tx.Unscoped().Where("id = ?", botUserID).Delete(&BotTokenModel{})
	if botTokenDeleteResult.Error != nil {
		return nil, botTokenDeleteResult.Error
	}
	stats.BotTokenDeleted += botTokenDeleteResult.RowsAffected

	if stats.UserDeleted > 0 || stats.BotTokenDeleted > 0 || stats.MemberDeleted > 0 || stats.WorldMemberDeleted > 0 || stats.UserRoleMappingDeleted > 0 || stats.FriendRelationDeleted > 0 || stats.PrivateChannelDeleted > 0 {
		stats.WebhookBotCount = 1
	}

	return stats, nil
}

func CleanupOrphanSystemBotByUserIDTx(tx *gorm.DB, botUserID string) (*WebhookBotFriendCleanupStats, error) {
	if tx == nil {
		return &WebhookBotFriendCleanupStats{}, nil
	}
	return cleanupOrphanSystemBotDataTx(tx, botUserID)
}

func CleanupOrphanSystemBotByUserID(botUserID string) (*WebhookBotFriendCleanupStats, error) {
	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	stats, err := cleanupOrphanSystemBotDataTx(tx, botUserID)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return stats, nil
}

// CleanupWebhookBotFriendData physically deletes orphaned system-managed bot users and their related
// friendship/private-channel/member records. Bots still referenced by active webhook/digest integrations are skipped.
func CleanupWebhookBotFriendData() (*WebhookBotFriendCleanupStats, error) {
	stats := &WebhookBotFriendCleanupStats{}

	internalBotSet, err := InternalBotUserIDSet(nil)
	if err != nil {
		return nil, err
	}
	if len(internalBotSet) == 0 {
		return stats, nil
	}

	systemBotIDs := make([]string, 0, len(internalBotSet))
	for id := range internalBotSet {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		systemBotIDs = append(systemBotIDs, id)
	}
	if len(systemBotIDs) == 0 {
		return stats, nil
	}
	sort.Strings(systemBotIDs)
	for _, botUserID := range systemBotIDs {
		itemStats, err := CleanupOrphanSystemBotByUserID(botUserID)
		if err != nil {
			return nil, err
		}
		addWebhookBotCleanupStats(stats, itemStats)
	}
	return stats, nil
}
