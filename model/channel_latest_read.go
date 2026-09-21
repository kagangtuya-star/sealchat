package model

import (
	"errors"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
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
	// NULL marks a legacy row that still needs one lazy unread-mention migration.
	LatestMentionTime   *int64 `gorm:"column:latest_mention_time" json:"-"`
	MentionStateVersion int64  `gorm:"column:mention_state_version;not null;default:0" json:"-"`

	Mark string `json:"mark"` // 特殊标记
}

func int64Ptr(v int64) *int64 {
	return &v
}

func (*ChannelLatestReadModel) TableName() string {
	return "channel_latest_read"
}

func ChannelReadListByUserId(inChIds []string, userId string) ([]*ChannelLatestReadModel, error) {
	var records []*ChannelLatestReadModel
	err := db.Where("channel_id in ? and user_id = ?", inChIds, userId).Find(&records).Error
	return records, err
}

type ChannelUnreadState struct {
	Counts   map[string]int64 `json:"counts"`
	Mentions map[string]bool  `json:"mentions"`
}

type channelMentionMigrationCall struct {
	done chan struct{}
	err  error
}

var channelMentionMigrationCalls sync.Map

var channelUnreadMentionFetchFunc = channelUnreadMentionFetch

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

func ChannelUnreadStateFetch(inChIds []string, userId string) (*ChannelUnreadState, error) {
	state := &ChannelUnreadState{
		Counts:   map[string]int64{},
		Mentions: map[string]bool{},
	}
	if len(inChIds) == 0 || userId == "" {
		return state, nil
	}

	items, err := ChannelReadListByUserId(inChIds, userId)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return state, nil
	}
	for _, item := range items {
		if item != nil && item.LatestMentionTime == nil {
			if err := ensureChannelMentionWatermarks(inChIds, userId); err != nil {
				return nil, err
			}
			items, err = ChannelReadListByUserId(inChIds, userId)
			if err != nil {
				return nil, err
			}
			break
		}
	}

	chIds := make([]string, 0, len(items))
	timeLst := make([]time.Time, 0, len(items))
	for _, item := range items {
		if item == nil || item.ChannelId == "" {
			continue
		}
		chIds = append(chIds, item.ChannelId)
		timeLst = append(timeLst, time.UnixMilli(item.MessageTime))
		if item.LatestMentionTime != nil && *item.LatestMentionTime > item.MessageTime {
			state.Mentions[item.ChannelId] = true
		}
	}
	if len(chIds) == 0 {
		return state, nil
	}

	counts, err := MessagesCountByChannelIDsAfterTime(chIds, timeLst, userId)
	if err != nil {
		return nil, err
	}
	state.Counts = counts
	return state, nil
}

func ensureChannelMentionWatermarks(channelIDs []string, userID string) error {
	for {
		items, err := ChannelReadListByUserId(channelIDs, userID)
		if err != nil {
			return err
		}
		if !channelMentionWatermarksNeedMigration(items) {
			return nil
		}

		newCall := &channelMentionMigrationCall{done: make(chan struct{})}
		actual, loaded := channelMentionMigrationCalls.LoadOrStore(userID, newCall)
		call := actual.(*channelMentionMigrationCall)
		if loaded {
			<-call.done
			if call.err != nil {
				return call.err
			}
			continue
		}

		call.err = migrateChannelMentionWatermarks(channelIDs, userID)
		close(call.done)
		channelMentionMigrationCalls.CompareAndDelete(userID, call)
		if call.err != nil {
			return call.err
		}
	}
}

func channelMentionWatermarksNeedMigration(items []*ChannelLatestReadModel) bool {
	for _, item := range items {
		if item != nil && item.ChannelId != "" && item.LatestMentionTime == nil {
			return true
		}
	}
	return false
}

func migrateChannelMentionWatermarks(channelIDs []string, userID string) error {
	items, err := ChannelReadListByUserId(channelIDs, userID)
	if err != nil {
		return err
	}
	legacyItems := make([]*ChannelLatestReadModel, 0, len(items))
	legacyChannelIDs := make([]string, 0, len(items))
	legacyTimes := make([]time.Time, 0, len(items))
	for _, item := range items {
		if item == nil || item.ChannelId == "" || item.LatestMentionTime != nil {
			continue
		}
		legacyItems = append(legacyItems, item)
		legacyChannelIDs = append(legacyChannelIDs, item.ChannelId)
		legacyTimes = append(legacyTimes, time.UnixMilli(item.MessageTime))
	}
	if len(legacyItems) == 0 {
		return nil
	}

	mentions, err := channelUnreadMentionFetchFunc(legacyChannelIDs, legacyTimes, userID)
	if err != nil {
		return err
	}
	for _, item := range legacyItems {
		mentionTime := int64(0)
		if mentions[item.ChannelId] {
			mentionTime = item.MessageTime + 1
		}
		if _, err := channelMentionBackfillIfNull(item.ID, item.MentionStateVersion, mentionTime); err != nil {
			return err
		}
	}
	return nil
}

// channelUnreadMentionFetch is only the one-time lazy migration fallback for
// legacy channel_latest_read rows whose latest_mention_time is NULL. It must not
// be used by the normal unread-state hot path.
func channelUnreadMentionFetch(channelIDs []string, updateTimes []time.Time, userID string) (map[string]bool, error) {
	if len(channelIDs) != len(updateTimes) {
		return nil, errors.New("channelIDs和updateTimes长度不匹配")
	}
	if len(channelIDs) == 0 || userID == "" {
		return map[string]bool{}, nil
	}

	var results []struct {
		ChannelID string
	}

	query := db.Table("mentions").
		Select("DISTINCT mentions.loc_post_id AS channel_id").
		Joins("JOIN messages ON messages.id = mentions.related_id").
		Where("mentions.loc_post_type = ? AND mentions.related_type = ?", "channel", "message").
		Where("(mentions.receiver_id = ? OR mentions.receiver_id = ?)", userID, "all").
		Where("messages.user_id <> ?", userID).
		Where("messages.is_deleted = ?", false).
		Where(`(
			messages.is_whisper = ?
			OR messages.user_id = ?
			OR messages.whisper_to = ?
			OR EXISTS (
				SELECT 1 FROM message_whisper_recipients r
				WHERE r.message_id = messages.id AND r.user_id = ?
			)
		)`, false, userID, userID, userID)

	conditions := db.Where("1 = 0")
	for i, channelID := range channelIDs {
		conditions = conditions.Or(db.Where("mentions.loc_post_id = ? AND messages.created_at > ?", channelID, updateTimes[i]))
	}

	if err := query.Where(conditions).Find(&results).Error; err != nil {
		return nil, err
	}

	mentionMap := make(map[string]bool, len(results))
	for _, result := range results {
		if result.ChannelID != "" {
			mentionMap[result.ChannelID] = true
		}
	}
	return mentionMap, nil
}

func channelMentionBackfillIfNull(recordID string, expectedVersion int64, mentionTime int64) (bool, error) {
	result := db.Model(&ChannelLatestReadModel{}).
		Where("id = ? AND latest_mention_time IS NULL AND mention_state_version = ?", recordID, expectedVersion).
		Update("latest_mention_time", mentionTime)
	return result.RowsAffected > 0, result.Error
}

func channelMentionAdvance(query *gorm.DB, messageTime int64) error {
	return query.Update("latest_mention_time", gorm.Expr(`CASE
		WHEN latest_mention_time IS NULL OR latest_mention_time < ? THEN ?
		ELSE latest_mention_time
	END`, messageTime, messageTime)).Error
}

func channelMentionAdvanceForUsersTx(tx *gorm.DB, channelID string, senderID string, userIDs []string, messageTime int64) error {
	if channelID == "" || len(userIDs) == 0 {
		return nil
	}
	return channelMentionAdvance(tx.Model(&ChannelLatestReadModel{}).
		Where("channel_id = ? AND user_id IN ? AND user_id <> ?", channelID, userIDs, senderID), messageTime)
}

func channelMentionAdvanceForChannelTx(tx *gorm.DB, channelID string, senderID string, messageTime int64) error {
	if channelID == "" {
		return nil
	}
	return channelMentionAdvance(tx.Model(&ChannelLatestReadModel{}).
		Where("channel_id = ? AND user_id <> ?", channelID, senderID), messageTime)
}

// ChannelMentionAdvanceForUsersTx is the transaction-aware form used when the
// mention rows and their watermarks must commit atomically.
func ChannelMentionAdvanceForUsersTx(tx *gorm.DB, channelID string, senderID string, userIDs []string, messageTime int64) error {
	if tx == nil {
		tx = db
	}
	return channelMentionAdvanceForUsersTx(tx, channelID, senderID, userIDs, messageTime)
}

// ChannelMentionAdvanceForChannelTx is the transaction-aware form used when
// the mention rows and their watermarks must commit atomically.
func ChannelMentionAdvanceForChannelTx(tx *gorm.DB, channelID string, senderID string, messageTime int64) error {
	if tx == nil {
		tx = db
	}
	return channelMentionAdvanceForChannelTx(tx, channelID, senderID, messageTime)
}

// ChannelMentionAdvanceForUsers atomically advances mention watermarks for
// existing read records belonging to the selected users.
func ChannelMentionAdvanceForUsers(channelID string, senderID string, userIDs []string, messageTime int64) error {
	return channelMentionAdvanceForUsersTx(db, channelID, senderID, userIDs, messageTime)
}

// ChannelMentionAdvanceForChannel atomically advances mention watermarks for
// every existing read record in the channel except the sender's.
func ChannelMentionAdvanceForChannel(channelID string, senderID string, messageTime int64) error {
	return channelMentionAdvanceForChannelTx(db, channelID, senderID, messageTime)
}

func channelMentionInvalidateForUsersTx(tx *gorm.DB, channelID string, senderID string, userIDs []string) error {
	if channelID == "" || len(userIDs) == 0 {
		return nil
	}
	// Pending NULL rows also need a revision bump so an in-flight legacy scan
	// cannot restore a mention deleted after that scan began.
	return tx.Model(&ChannelLatestReadModel{}).
		Where("channel_id = ? AND user_id IN ? AND user_id <> ?", channelID, userIDs, senderID).
		Where("(latest_mention_time IS NULL OR latest_mention_time > message_time)").
		Updates(map[string]any{
			"latest_mention_time":   nil,
			"mention_state_version": gorm.Expr("mention_state_version + 1"),
		}).Error
}

func channelMentionInvalidateForChannelTx(tx *gorm.DB, channelID string, senderID string) error {
	if channelID == "" {
		return nil
	}
	return tx.Model(&ChannelLatestReadModel{}).
		Where("channel_id = ? AND user_id <> ?", channelID, senderID).
		Where("(latest_mention_time IS NULL OR latest_mention_time > message_time)").
		Updates(map[string]any{
			"latest_mention_time":   nil,
			"mention_state_version": gorm.Expr("mention_state_version + 1"),
		}).Error
}

func ChannelMentionInvalidateForUsersTx(tx *gorm.DB, channelID string, senderID string, userIDs []string) error {
	if tx == nil {
		tx = db
	}
	return channelMentionInvalidateForUsersTx(tx, channelID, senderID, userIDs)
}

func ChannelMentionInvalidateForChannelTx(tx *gorm.DB, channelID string, senderID string) error {
	if tx == nil {
		tx = db
	}
	return channelMentionInvalidateForChannelTx(tx, channelID, senderID)
}

func ChannelMentionInvalidateForUsers(channelID string, senderID string, userIDs []string) error {
	return channelMentionInvalidateForUsersTx(db, channelID, senderID, userIDs)
}

func ChannelMentionInvalidateForChannel(channelID string, senderID string) error {
	return channelMentionInvalidateForChannelTx(db, channelID, senderID)
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
			ChannelId:         channelId,
			UserId:            userId,
			MessageTime:       time.Now().UnixMilli(),
			LatestMentionTime: int64Ptr(0),
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
	ReadAllWhispers bool
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
		Where("is_deleted = ?", false)

	includeArchived := false
	icFilter := ""
	includeRoleless := false
	readAllWhispers := false
	var roleIDs []string
	if options != nil {
		includeArchived = options.IncludeArchived
		icFilter = strings.ToLower(strings.TrimSpace(options.ICFilter))
		includeRoleless = options.IncludeRoleless
		readAllWhispers = options.ReadAllWhispers
		for _, id := range options.RoleIDs {
			trimmed := strings.TrimSpace(id)
			if trimmed != "" {
				roleIDs = append(roleIDs, trimmed)
			}
		}
	}
	if !readAllWhispers {
		q = q.Where(`(is_whisper = ? OR user_id = ? OR whisper_to = ? OR EXISTS (
			SELECT 1 FROM message_whisper_recipients r WHERE r.message_id = messages.id AND r.user_id = ?
		))`, false, userId, userId, userId)
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

func ChannelLatestReadCleanupOrphans() (int64, error) {
	tx := db.Where("NOT EXISTS (SELECT 1 FROM channels c WHERE c.id = channel_latest_read.channel_id) OR NOT EXISTS (SELECT 1 FROM users u WHERE u.id = channel_latest_read.user_id)").
		Delete(&ChannelLatestReadModel{})
	return tx.RowsAffected, tx.Error
}
