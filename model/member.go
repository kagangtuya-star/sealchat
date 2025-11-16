package model

import (
	"sort"
	"strings"
	"time"

	"sealchat/protocol"
)

type MemberModel struct {
	StringPKBaseModel
	Nickname     string `gorm:"null" json:"nick"`           // 昵称
	ChannelID    string `gorm:"not null" json:"channel_id"` // 频道ID
	UserID       string `json:"user_id" gorm:"index,null"`  // 用户ID
	WorldID      string `json:"world_id,omitempty" gorm:"index"`
	RecentSentAt int64  `json:"recentSentAt"` // 最近发送消息的时间
}

func (u *MemberModel) SaveInfo() {
	db.Model(u).Select("nickname").Updates(u)
}

func (*MemberModel) TableName() string {
	return "members"
}

func (u *MemberModel) ToProtocolType() *protocol.GuildMember {
	return &protocol.GuildMember{
		ID: u.ID,
		User: &protocol.User{
			ID: u.UserID,
		},
		Nick: u.Nickname,
	}
}

func (m *MemberModel) UpdateRecentSent() {
	m.RecentSentAt = time.Now().UnixMilli()
	db.Model(m).Update("recent_sent_at", m.RecentSentAt)
}

func MemberGetByUserIDAndChannelIDBase(userId string, channelId string, defaultName string, createIfNotExists bool) (*MemberModel, error) {
	var member MemberModel
	err := db.Where("user_id = ? AND channel_id = ?", userId, channelId).Limit(1).Find(&member).Error
	if member.ID == "" {
		// 未找到记录，尝试创建新的记录
		if createIfNotExists {
			worldId := ""
			if ch, chErr := ChannelGet(channelId); chErr == nil && ch != nil {
				worldId = ch.WorldID
			}
			x := MemberModel{UserID: userId, ChannelID: channelId, Nickname: defaultName, WorldID: worldId}
			err = db.Create(&x).Error
			return &x, err
		}
		return nil, nil
	}
	return &member, nil
}

func MemberGetByUserIDAndChannelID(userId string, channelId string, defaultName string) (*MemberModel, error) {
	return MemberGetByUserIDAndChannelIDBase(userId, channelId, defaultName, true)
}

// MemberExistsInWorld 判断用户是否已经在某个世界的频道中出现（用于兼容旧数据）
func MemberExistsInWorld(worldID, userID string) (bool, error) {
	worldID = strings.TrimSpace(worldID)
	userID = strings.TrimSpace(userID)
	if worldID == "" || userID == "" {
		return false, nil
	}
	var count int64
	err := db.Table("members AS m").
		Joins("JOIN channels AS c ON c.id = m.channel_id").
		Where("m.user_id = ? AND c.world_id = ?", userID, worldID).
		Limit(1).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type ChannelMemberOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func ChannelMemberOptionList(channelID string) ([]*ChannelMemberOption, error) {
	if strings.TrimSpace(channelID) == "" {
		return []*ChannelMemberOption{}, nil
	}

	type rawOption struct {
		UserID     string
		MemberNick string
		UserNick   string
		Username   string
	}

	var rows []rawOption
	db := GetDB()
	err := db.Table("members AS m").
		Select("m.user_id AS user_id, m.nickname AS member_nick, u.nickname AS user_nick, u.username AS username").
		Joins("LEFT JOIN users u ON u.id = m.user_id").
		Where("m.channel_id = ?", channelID).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	options := make([]*ChannelMemberOption, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.UserID == "" {
			continue
		}
		if _, exists := seen[row.UserID]; exists {
			continue
		}
		label := pickMemberDisplayName(row.MemberNick, row.UserNick, row.Username)
		options = append(options, &ChannelMemberOption{
			ID:    row.UserID,
			Label: label,
		})
		seen[row.UserID] = struct{}{}
	}

	sort.Slice(options, func(i, j int) bool {
		return strings.ToLower(options[i].Label) < strings.ToLower(options[j].Label)
	})
	return options, nil
}

func pickMemberDisplayName(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return "未知成员"
}
