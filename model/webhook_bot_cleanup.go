package model

import "strings"

type WebhookBotFriendCleanupStats struct {
	WebhookBotCount       int64 `json:"webhookBotCount"`
	FriendRelationDeleted int64 `json:"friendRelationDeleted"`
	PrivateChannelDeleted int64 `json:"privateChannelDeleted"`
}

// CleanupWebhookBotFriendData physically deletes friendship rows and private channels
// related to webhook-created bot users.
func CleanupWebhookBotFriendData() (*WebhookBotFriendCleanupStats, error) {
	stats := &WebhookBotFriendCleanupStats{}

	webhookBotSet, err := WebhookBotUserIDSet(nil)
	if err != nil {
		return nil, err
	}
	if len(webhookBotSet) == 0 {
		return stats, nil
	}

	webhookBotIDs := make([]string, 0, len(webhookBotSet))
	for id := range webhookBotSet {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		webhookBotIDs = append(webhookBotIDs, id)
	}
	stats.WebhookBotCount = int64(len(webhookBotIDs))
	if len(webhookBotIDs) == 0 {
		return stats, nil
	}

	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	rollback := func(err error) (*WebhookBotFriendCleanupStats, error) {
		tx.Rollback()
		return nil, err
	}

	var friendChannelIDs []string
	if err := tx.Model(&FriendModel{}).
		Where("user_id1 IN ? OR user_id2 IN ?", webhookBotIDs, webhookBotIDs).
		Pluck("id", &friendChannelIDs).Error; err != nil {
		return rollback(err)
	}

	friendDeleteResult := tx.Unscoped().
		Where("user_id1 IN ? OR user_id2 IN ?", webhookBotIDs, webhookBotIDs).
		Delete(&FriendModel{})
	if friendDeleteResult.Error != nil {
		return rollback(friendDeleteResult.Error)
	}
	stats.FriendRelationDeleted = friendDeleteResult.RowsAffected

	uniqueChannelIDs := make([]string, 0, len(friendChannelIDs))
	seen := map[string]struct{}{}
	for _, id := range friendChannelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueChannelIDs = append(uniqueChannelIDs, id)
	}

	if len(uniqueChannelIDs) > 0 {
		channelDeleteResult := tx.Unscoped().
			Where("id IN ? AND (is_private = ? OR perm_type = ?)", uniqueChannelIDs, true, "private").
			Delete(&ChannelModel{})
		if channelDeleteResult.Error != nil {
			return rollback(channelDeleteResult.Error)
		}
		stats.PrivateChannelDeleted = channelDeleteResult.RowsAffected
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return stats, nil
}
