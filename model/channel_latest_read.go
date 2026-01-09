package model

import (
	"strings"
	"time"

	"gorm.io/gorm/clause"
)

/*
本来我想了一些复杂的方案，并估算了内存和硬盘的使用
但随后，我意识到并不需要考虑那么多。
*/

type ChannelLatestReadModel struct {
	StringPKBaseModel

	ChannelId string `gorm:"index:idx_channel_user,unique" json:"channelId"`    // 目前仅用于频道ID
	UserId    string `gorm:"index:idx_channel_user,unique;index" json:"userId"` // 用户ID

	MessageId   string
	MessageTime int64

	Mark string `json:"mark"` // 特殊标记
}

func (*ChannelLatestReadModel) TableName() string {
	return "channel_latest_read"
}

func ChannelReadListByUserId(inChIds []string, userId string) ([]*ChannelLatestReadModel, error) {
	var records []*ChannelLatestReadModel
	err := db.Where("channel_id in ? and user_id = ?", inChIds, userId).Find(&records).Error
	return records, err
}

func ChannelUnreadFetch(inChIds []string, userId string) (map[string]int64, error) {
	items, err := ChannelReadListByUserId(inChIds, userId)
	if err != nil {
		return nil, err
	}

	var chIds []string
	var timeLst []time.Time
	for _, i := range items {
		chIds = append(chIds, i.ChannelId)
		timeLst = append(timeLst, time.UnixMilli(i.MessageTime))
	}

	unreadMap, err := MessagesCountByChannelIDsAfterTime(chIds, timeLst, userId)
	if err != nil {
		return nil, err
	}

	return unreadMap, err
}

func ChannelReadSet(channelId, userId string) error {
	var record ChannelLatestReadModel
	err := db.Where("channel_id = ? AND user_id = ?", channelId, userId).Limit(1).Find(&record).Error
	if err != nil {
		return err
	}
	if record.ID == "" {
		// 记录不存在,创建新记录
		record = ChannelLatestReadModel{
			ChannelId:   channelId,
			UserId:      userId,
			MessageTime: time.Now().UnixMilli(),
		}
		return db.Create(&record).Error
	}

	return db.Model(&ChannelLatestReadModel{}).
		Where("channel_id = ? AND user_id = ?", channelId, userId).
		Updates(map[string]any{
			"message_time": time.Now().UnixMilli(),
		}).Error
}

// ChannelReadSetInBatch 批量设置已读，但要求已存在
func ChannelReadSetInBatch(channelIds []string, userIds []string) error {
	now := time.Now().UnixMilli()

	// 只更新已存在的记录
	return db.Model(&ChannelLatestReadModel{}).
		Where("channel_id IN ? AND user_id IN ?", channelIds, userIds).
		Updates(map[string]any{
			"message_time": now,
		}).Error
}

func ChannelReadInit(channelId, userId string) error {
	return db.Clauses(clause.OnConflict{
		DoNothing: true, // 对应 INSERT OR IGNORE
	}).Create(&ChannelLatestReadModel{
		ChannelId:   channelId,
		UserId:      userId,
		MessageTime: 0,
	}).Error
}

func ChannelReadInitInBatches(channelId string, userIds []string) error {
	models := make([]ChannelLatestReadModel, len(userIds))
	for i, userId := range userIds {
		models[i] = ChannelLatestReadModel{
			ChannelId:   channelId,
			UserId:      userId,
			MessageTime: 0,
		}
	}

	return db.Clauses(clause.OnConflict{
		DoNothing: true, // 对应 INSERT OR IGNORE
	}).CreateInBatches(models, 100).Error
}

type FirstUnreadFilterOptions struct {
	IncludeArchived bool
	ICFilter        string
	RoleIDs         []string
	IncludeRoleless bool
}

// ChannelGetFirstUnreadInfo 获取频道的第一条未读消息信息
// 返回: messageId, messageTime (毫秒时间戳), error
func ChannelGetFirstUnreadInfo(channelId, userId string, options *FirstUnreadFilterOptions) (string, int64, error) {
	var record ChannelLatestReadModel
	err := db.Where("channel_id = ? AND user_id = ?", channelId, userId).Limit(1).Find(&record).Error
	if err != nil {
		return "", 0, err
	}

	if record.ID == "" {
		// 没有已读记录，不启用跳转
		return "", 0, nil
	}

	// 查找该时间之后的第一条消息（排除自己发的，遵循筛选）
	lastReadTime := time.UnixMilli(record.MessageTime)
	var firstUnread MessageModel
	q := db.Where("channel_id = ? AND created_at > ? AND user_id <> ?", channelId, lastReadTime, userId).
		Where("is_deleted = ?", false).
		Where("(is_whisper = ? OR user_id = ? OR whisper_to = ?)", false, userId, userId)

	includeArchived := false
	icFilter := ""
	includeRoleless := false
	var roleIDs []string
	if options != nil {
		includeArchived = options.IncludeArchived
		icFilter = strings.ToLower(strings.TrimSpace(options.ICFilter))
		includeRoleless = options.IncludeRoleless
		for _, id := range options.RoleIDs {
			trimmed := strings.TrimSpace(id)
			if trimmed != "" {
				roleIDs = append(roleIDs, trimmed)
			}
		}
	}

	if !includeArchived {
		q = q.Where("is_archived = ?", false)
	}

	switch icFilter {
	case "ic":
		q = q.Where("(ic_mode = ? OR ic_mode = '' OR ic_mode IS NULL)", "ic")
	case "ooc":
		q = q.Where("ic_mode = ?", "ooc")
	}

	if len(roleIDs) > 0 || includeRoleless {
		roleCond := "(sender_role_id IN ? OR sender_identity_id IN ?)"
		roleArgs := []any{roleIDs, roleIDs}
		if includeRoleless {
			roleCond = "(" + roleCond + " OR ((sender_role_id = '' OR sender_role_id IS NULL) AND (sender_identity_id = '' OR sender_identity_id IS NULL)))"
		}
		if len(roleIDs) == 0 && includeRoleless {
			roleCond = "((sender_role_id = '' OR sender_role_id IS NULL) AND (sender_identity_id = '' OR sender_identity_id IS NULL))"
			roleArgs = nil
		}
		if roleArgs != nil {
			q = q.Where(roleCond, roleArgs...)
		} else {
			q = q.Where(roleCond)
		}
	}

	err = q.Order("created_at ASC").
		Limit(1).
		Select("id, created_at").
		Find(&firstUnread).Error
	if err != nil {
		return "", 0, err
	}

	if firstUnread.ID != "" {
		return firstUnread.ID, firstUnread.CreatedAt.UnixMilli(), nil
	}
	return "", 0, nil
}
