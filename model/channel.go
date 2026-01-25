package model

import (
	"fmt"
	"strings"
	"time"

	"sealchat/protocol"
)

// 频道状态常量
const (
	ChannelStatusActive   = "active"   // 正常状态
	ChannelStatusArchived = "archived" // 归档状态
)

type ChannelModel struct {
	StringPKBaseModel
	WorldID            string `json:"worldId" gorm:"size:100;index"`
	Name               string `json:"name"`
	Note               string `json:"note"`                   // 这是一份注释，用于管理人员辨别数据
	RootId             string `json:"rootId"`                 // 如果未来有多级子频道，那么rootId指向顶层
	ParentID           string `json:"parentId" gorm:"null"`   // 好像satori协议这里不统一啊
	IsPrivate          bool   `json:"isPrivate" gorm:"index"` // 是私聊频道吗？
	RecentSentAt       int64  `json:"recentSentAt"`           // 最近发送消息的时间
	UserID             string `json:"userId"`                 // 创建者ID
	PermType           string `json:"permType"`               // public 公开 non-public 非公开 private 私聊
	DefaultDiceExpr    string `json:"defaultDiceExpr" gorm:"size:32;not null;default:d20"`
	BuiltInDiceEnabled bool   `json:"builtInDiceEnabled" gorm:"default:true"`
	BotFeatureEnabled  bool   `json:"botFeatureEnabled" gorm:"default:false"`
	Status             string `json:"status" gorm:"size:24;default:active;index"`

	SortOrder int `json:"sortOrder" gorm:"index"` // 优先级序号，越大越靠前

	BackgroundAttachmentId string `json:"backgroundAttachmentId" gorm:"size:100"` // 背景图附件ID
	BackgroundSettings     string `json:"backgroundSettings" gorm:"type:text"`    // JSON: 背景显示设置

	FriendInfo   *FriendModel `json:"friendInfo,omitempty" gorm:"-"`
	MembersCount int          `json:"membersCount" gorm:"-"`
}

type ChannelBackgroundUpdate struct {
	BackgroundAttachmentId string `json:"backgroundAttachmentId"`
	BackgroundSettings     string `json:"backgroundSettings"`
}

func (*ChannelModel) TableName() string {
	return "channels"
}

func (m *ChannelModel) UpdateRecentSent() {
	m.RecentSentAt = time.Now().UnixMilli()
	db.Model(m).Update("recent_sent_at", m.RecentSentAt)
}

func parseSQLiteTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, true
	}
	layoutsWithZone := []string{
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999Z07:00",
		"2006-01-02 15:04:05.999Z07:00",
		"2006-01-02 15:04:05Z07:00",
	}
	for _, layout := range layoutsWithZone {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	layouts := []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05.999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// BackfillChannelRecentSentAt 根据历史消息回填频道最近发言时间。
func BackfillChannelRecentSentAt() error {
	const batchSize = 500
	updateRecentSentAt := func(channelID string, lastCreated time.Time) error {
		if channelID == "" || lastCreated.IsZero() {
			return nil
		}
		return db.Model(&ChannelModel{}).
			Where("id = ?", channelID).
			UpdateColumn("recent_sent_at", lastCreated.UnixMilli()).Error
	}
	for {
		var rows []struct {
			ID string `gorm:"column:id"`
		}
		if err := db.Table("channels").
			Select("channels.id").
			Joins("JOIN messages ON messages.channel_id = channels.id").
			Where("channels.recent_sent_at IS NULL OR channels.recent_sent_at = 0").
			Group("channels.id").
			Limit(batchSize).
			Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			if row.ID != "" {
				ids = append(ids, row.ID)
			}
		}
		if len(ids) == 0 {
			break
		}
		if IsSQLite() {
			var msgRows []struct {
				ChannelID   string `gorm:"column:channel_id"`
				LastCreated string `gorm:"column:last_created"`
			}
			if err := db.Table("messages").
				Select("channel_id, MAX(created_at) as last_created").
				Where("channel_id IN ?", ids).
				Group("channel_id").
				Scan(&msgRows).Error; err != nil {
				return err
			}
			for _, row := range msgRows {
				lastCreated, ok := parseSQLiteTime(row.LastCreated)
				if !ok {
					continue
				}
				if err := updateRecentSentAt(row.ChannelID, lastCreated); err != nil {
					return err
				}
			}
			continue
		}
		var msgRows []struct {
			ChannelID   string    `gorm:"column:channel_id"`
			LastCreated time.Time `gorm:"column:last_created"`
		}
		if err := db.Table("messages").
			Select("channel_id, MAX(created_at) as last_created").
			Where("channel_id IN ?", ids).
			Group("channel_id").
			Scan(&msgRows).Error; err != nil {
			return err
		}
		for _, row := range msgRows {
			if err := updateRecentSentAt(row.ChannelID, row.LastCreated); err != nil {
				return err
			}
		}
	}
	return nil
}

// ChannelBackgroundEdit 仅更新频道背景相关字段
func ChannelBackgroundEdit(channelId string, updates *ChannelBackgroundUpdate) error {
	return db.Model(&ChannelModel{}).
		Where("id = ?", channelId).
		Select("background_attachment_id", "background_settings").
		Updates(updates).Error
}

// ChannelInfoEdit 可修改内容: 名称，简介，公开或非公开，成员正在输入提示，优先级序号，背景图
func ChannelInfoEdit(channelId string, updates *ChannelModel) error {
	if err := db.Model(&ChannelModel{}).
		Where("id = ?", channelId).Select("name", "note", "perm_type", "sort_order", "background_attachment_id", "background_settings").
		Updates(updates).Error; err != nil {
		return err
	}
	return nil
}

func (c *ChannelModel) GetPrivateUserIDs() []string {
	return strings.SplitN(c.ID, ":", 2)
}

func (c *ChannelModel) ToProtocolType() *protocol.Channel {
	channelType := protocol.TextChannelType
	if c.IsPrivate {
		channelType = protocol.DirectChannelType
	}
	return &protocol.Channel{
		ID:                 c.ID,
		WorldID:            c.WorldID,
		Name:               c.Name,
		Type:               channelType,
		DefaultDiceExpr:    c.DefaultDiceExpr,
		BuiltInDiceEnabled: c.BuiltInDiceEnabled,
		BotFeatureEnabled:  c.BotFeatureEnabled,
		BackgroundAttachmentId: c.BackgroundAttachmentId,
		BackgroundSettings:     c.BackgroundSettings,
	}
}

func ChannelPublicNew(channelID string, ch *ChannelModel, creatorId string) *ChannelModel {
	ch.ID = channelID
	ch.UserID = creatorId
	if !ch.BuiltInDiceEnabled && !ch.BotFeatureEnabled {
		ch.BuiltInDiceEnabled = true
	}

	db.Create(ch)
	return ch
}

func ChannelPrivateNew(userID1, userID2 string) (ch *ChannelModel, isNew bool) {
	if userID1 > userID2 {
		userID1, userID2 = userID2, userID1
	}

	chId := fmt.Sprintf("%s:%s", userID1, userID2)

	u1 := UserGet(userID1)
	u2 := UserGet(userID2)

	if u1 == nil || u2 == nil {
		return nil, false
	}

	chExists := &ChannelModel{}
	db.Where("id = ?", chId).Limit(1).Find(&chExists)
	if chExists.ID != "" {
		return chExists, false
	}

	ch = &ChannelModel{
		StringPKBaseModel:  StringPKBaseModel{ID: chId},
		IsPrivate:          true,
		Name:               "@私聊频道",
		PermType:           "private",
		Note:               fmt.Sprintf("%s-%s", u1.Username, u2.Username),
		DefaultDiceExpr:    "d20",
		BuiltInDiceEnabled: true,
	}
	db.Create(ch)

	return ch, true
}

// ChannelGet 获取频道
func ChannelGet(id string) (*ChannelModel, error) {
	var item ChannelModel
	err := db.Limit(1).Find(&item, "id = ?", id).Error
	return &item, err
}

func ChannelPrivateGet(userID1, userID2 string) (ch *ChannelModel, err error) {
	if userID1 > userID2 {
		userID1, userID2 = userID2, userID1
	}

	chId := fmt.Sprintf("%s:%s", userID1, userID2)
	return ChannelGet(chId)
}

func ChannelPrivateList(userId string) []*ChannelModel {
	// 加载有权限访问的频道
	var items []*ChannelModel
	q := db.Where("is_private = true and ", true).Order("created_at asc")
	q.Find(&items)

	return items
}
