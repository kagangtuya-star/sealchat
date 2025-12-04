package model

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"gorm.io/gorm"

	"sealchat/utils"
)

// FriendModel 定义好友关系表结构
type FriendModel struct {
	StringPKBaseModel
	UserID1  string `json:"userId1" gorm:"index;index:idx_user_id_pair,unique"`
	UserID2  string `json:"userId2" gorm:"index;index:idx_user_id_pair,unique"`
	IsFriend bool   `json:"isFriend"` // 可能是好友或陌生人

	Visible1 bool       `json:"visible1"`          // 陌生人时，对1可见
	Visible2 bool       `json:"visible2"`          // 陌生人是，对2可见
	UserInfo *UserModel `json:"userInfo" gorm:"-"` // 单边查询时，这里存放另一个人的信息

	IMUserid string `json:"IMUserId"`
}

func (*FriendModel) TableName() string {
	return "friends"
}

// BeforeCreate 在创建记录前确保 UserID1 的字典序小于 UserID2
func (fr *FriendModel) BeforeCreate(tx *gorm.DB) error {
	if fr.UserID1 > fr.UserID2 {
		fr.UserID1, fr.UserID2 = fr.UserID2, fr.UserID1
	}
	fr.ID = fmt.Sprintf("%s:%s", fr.UserID1, fr.UserID2)
	if fr.ID == "" {
		fr.Init()
	}
	return nil
}

// FriendRelationCreate 创建好友关系
func FriendRelationCreate(userID1, userID2 string, isFriend bool) error {
	firstChatUserId := userID1 // userID1 是发起者
	if userID1 > userID2 {
		// 这里有点重复，但是只有这里才能标记发起者
		userID1, userID2 = userID2, userID1
	}

	relation := &FriendModel{
		UserID1:  userID1,
		UserID2:  userID2,
		IsFriend: isFriend, // 注: 大部分时候新建时应该是陌生关系
	}

	if !isFriend {
		// 这两个只对陌生人有意义
		if firstChatUserId == userID1 {
			relation.Visible1 = true
		}
		if firstChatUserId == userID2 {
			relation.Visible2 = true
		}
	}
	return db.Create(&relation).Error
}

// FriendRelationGetByID 通过ID获取好友关系
func FriendRelationGetByID(id string) (*FriendModel, error) {
	var item FriendModel
	err := db.Where("id = ?", id).Limit(1).Find(&item).Error
	return &item, err
}

// FriendRelationGet 用户关系获取
func FriendRelationGet(userID1, userID2 string) *FriendModel {
	if userID1 > userID2 {
		// 这里有点重复，但是只有这里才能标记发起者
		userID1, userID2 = userID2, userID1
	}

	item := FriendModel{}
	db.Model(&FriendModel{}).Where("user_id1 = ? and user_id2 = ?", userID1, userID2).
		Limit(1).Find(&item)

	return &item
}

// FriendRelationFriendApprove 设置为好友
func FriendRelationFriendApprove(userID1, userID2 string) (bool, error) {
	if userID1 > userID2 {
		// 这里有点重复，但是只有这里才能标记发起者
		userID1, userID2 = userID2, userID1
	}

	item := FriendRelationGet(userID1, userID2)
	if item.ID == "" {
		// 如果不存在，创建一个新的
		_ = FriendRelationCreate(userID1, userID2, true)
		return true, nil
	} else {
		// 如果已经是好友，那么无视
		if item.IsFriend {
			return false, nil
		}
	}

	q := db.Model(&FriendModel{}).Where("user_id1 = ? and user_id2 = ?", userID1, userID2).
		Updates(map[string]any{"is_friend": true})

	return q.RowsAffected > 0, q.Error
}

// FriendRelationFriendApproveById 设置为好友
func FriendRelationFriendApproveById(ID string) (bool, error) {
	q := db.Model(&FriendModel{}).Where("id = ?", ID).
		Updates(map[string]any{"is_friend": true})
	return q.RowsAffected > 0, q.Error
}

func FriendRelationSetVisibleById(id string) {
	updates := map[string]interface{}{
		"visible1": true,
		"visible2": true,
	}
	db.Model(&FriendModel{}).Where("id = ?", id).Updates(updates)
}

// FriendRelationDelete 删除好友关系
func FriendRelationDelete(userID1, userID2 string) bool {
	if userID1 > userID2 {
		userID1, userID2 = userID2, userID1
	}

	q := db.Model(&FriendModel{}).Where("user_id1 = ? AND user_id2 = ?", userID1, userID2).
		Updates(map[string]any{
			"is_friend": false,
			"visible1":  false,
			"visible2":  false,
		})
	return q.RowsAffected > 0
}

// FriendRelationSetVisible 删除好友关系
func FriendRelationSetVisible(userID1, userID2 string) bool {
	firstChatUserId := userID1 // userID1 是发起者
	if userID1 > userID2 {
		userID1, userID2 = userID2, userID1
	}

	m := map[string]any{}
	if firstChatUserId == userID1 {
		m["visible1"] = true
	}
	if firstChatUserId == userID2 {
		m["visible2"] = true
	}

	q := db.Model(&FriendModel{}).Where("user_id1 = ? AND user_id2 = ?", userID1, userID2).
		Updates(m)
	return q.RowsAffected > 0
}

// IsFriend 检查两个用户是否为好友
func IsFriend(userID1, userID2 string) (bool, error) {
	if userID1 > userID2 {
		userID1, userID2 = userID2, userID1
	}

	var count int64
	err := db.Model(&FriendModel{}).Where("user_id1 = ? AND user_id2 = ?", userID1, userID2).
		Count(&count).Error
	return count > 0, err
}

// FriendIdList 获取用户的好友列表
func FriendIdList(userID string) ([]string, error) {
	var friends []string
	err := db.Model(&FriendModel{}).
		Where("user_id1 = ? OR user_id2 = ?", userID, userID).
		Select("CASE WHEN user_id1 = ? THEN user_id2 ELSE user_id1 END", userID).
		Find(&friends).Error
	return friends, err
}

// FriendChannelList 获取用户的好友频道列表
func FriendChannelList(userID string) ([]*ChannelModel, error) {
	var friends []*FriendModel
	err := db.Model(&FriendModel{}).
		Where("user_id1 = ? OR user_id2 = ?", userID, userID).
		Find(&friends).Error
	if err != nil {
		return nil, err
	}

	var userIds utils.SyncSet[string]
	friendMap := map[string]*FriendModel{}

	var friendIds []string
	var strangerIds []string
	_ = lo.Map(friends, func(item *FriendModel, index int) string {
		chId := fmt.Sprintf("%s:%s", item.UserID1, item.UserID2)
		userIds.Add(item.UserID1)
		userIds.Add(item.UserID2)
		friendMap[chId] = item

		if item.IsFriend {
			friendIds = append(friendIds, chId)
		} else {
			if userID == item.UserID1 && item.Visible1 {
				strangerIds = append(strangerIds, chId)
			}
			if userID == item.UserID2 && item.Visible2 {
				strangerIds = append(strangerIds, chId)
			}
		}
		return chId
	})

	var channels []*ChannelModel
	// 1. 频道ID在friendIds列表中
	// 2. 或者频道ID在strangerIds列表中且最近发送时间大于0
	db.Where("id in ? or id in ?", friendIds, strangerIds).
		Find(&channels)

	userIds.Delete(userID)
	var users []*UserModel
	userMap := map[string]*UserModel{}
	db.Where("id in ?", userIds.ToArray()).Find(&users)
	_ = lo.Map(users, func(item *UserModel, index int) string {
		userMap[item.ID] = item
		return ""
	})

	for index := range channels {
		ch := channels[index]
		if ch == nil {
			continue
		}
		friend := friendMap[ch.ID]
		if friend == nil {
			continue
		}
		targetUser := userMap[friend.getAnotherUserId(userID)]
		friend.UserInfo = targetUser
		ch.FriendInfo = friend
		if targetUser == nil {
			continue
		}
		displayName := strings.TrimSpace(targetUser.Nickname)
		if displayName == "" {
			displayName = strings.TrimSpace(targetUser.Username)
		}
		if displayName == "" {
			displayName = strings.TrimSpace(targetUser.ID)
		}
		if displayName != "" {
			ch.Name = fmt.Sprintf("@%s", displayName)
		}
	}

	return channels, err
}

// FriendChannelIDList 获取用户可见的私聊频道ID列表
func FriendChannelIDList(userID string) ([]string, error) {
	if strings.TrimSpace(userID) == "" {
		return []string{}, nil
	}
	var relations []*FriendModel
	err := db.Model(&FriendModel{}).
		Where("user_id1 = ? OR user_id2 = ?", userID, userID).
		Select("id, user_id1, user_id2, is_friend, visible1, visible2").
		Find(&relations).Error
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, item := range relations {
		if item == nil || strings.TrimSpace(item.ID) == "" {
			continue
		}
		if item.IsFriend {
			ids = append(ids, item.ID)
			continue
		}
		if userID == item.UserID1 && item.Visible1 {
			ids = append(ids, item.ID)
		}
		if userID == item.UserID2 && item.Visible2 {
			ids = append(ids, item.ID)
		}
	}
	return ids, nil
}

func (fr *FriendModel) getAnotherUserId(userID string) string {
	if userID == fr.UserID1 {
		return fr.UserID2
	} else {
		return fr.UserID1
	}
}

// FriendList 获取用户的好友列表
func FriendList(userID string, friendOnly bool) ([]*FriendModel, error) {
	var friends []*FriendModel

	q := db.Model(&FriendModel{}).
		Where("user_id1 = ? OR user_id2 = ?", userID, userID)

	if friendOnly {
		q = q.Where("is_friend = true")
	}

	err := q.Find(&friends).Error
	if err != nil {
		return nil, err
	}

	utils.QueryOneToManyMap(db, friends, func(i *FriendModel) []string {
		return []string{i.getAnotherUserId(userID)}
	}, func(i *FriendModel, x []*UserModel) {
		i.UserInfo = x[0]
	}, "")

	return friends, err
}
