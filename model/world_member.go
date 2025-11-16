package model

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type WorldMemberState string

const (
	WorldMemberStateActive WorldMemberState = "active"
	WorldMemberStateBanned WorldMemberState = "banned"
)

const (
	WorldMemberRoleOwner  = "owner"
	WorldMemberRoleAdmin  = "admin"
	WorldMemberRoleMember = "member"
)

// WorldMemberModel 记录世界级成员状态
type WorldMemberModel struct {
	StringPKBaseModel
	WorldID    string            `json:"worldId" gorm:"index"`
	UserID     string            `json:"userId" gorm:"index"`
	Nickname   string            `json:"nickname" gorm:"size:64"`
	JoinedAt   time.Time         `json:"joinedAt"`
	State      WorldMemberState  `json:"state" gorm:"size:16;index;default:'active'"`
	Role       string            `json:"role" gorm:"size:32;index;default:'member'"`
	MutedUntil *time.Time        `json:"mutedUntil"`
	LastReadAt *time.Time        `json:"lastReadAt"`
	Metadata   map[string]string `json:"metadata,omitempty" gorm:"-"`
}

func (*WorldMemberModel) TableName() string {
	return "world_members"
}

func (m *WorldMemberModel) BeforeCreate(tx *gorm.DB) error {
	if err := m.StringPKBaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	if m.JoinedAt.IsZero() {
		m.JoinedAt = time.Now()
	}
	if m.State == "" {
		m.State = WorldMemberStateActive
	}
	if strings.TrimSpace(m.Role) == "" {
		m.Role = WorldMemberRoleMember
	}
	return nil
}

// WorldMemberGet 根据 world + user 查询成员
func WorldMemberGet(worldID, userID string) (*WorldMemberModel, error) {
	var item WorldMemberModel
	err := db.Where("world_id = ? AND user_id = ?", worldID, userID).
		Limit(1).
		Find(&item).Error
	if item.ID == "" {
		return nil, err
	}
	return &item, err
}

// WorldMemberEnsureActive 创建或激活一个成员
func WorldMemberEnsureActive(worldID, userID, nickname string) (*WorldMemberModel, bool, error) {
	member, err := WorldMemberGet(worldID, userID)
	if err != nil {
		return nil, false, err
	}
	if member != nil {
		if member.State == WorldMemberStateBanned {
			err = db.Model(member).
				Where("id = ?", member.ID).
				Updates(map[string]interface{}{
					"state":       WorldMemberStateActive,
					"muted_until": nil,
					"updated_at":  time.Now(),
				}).Error
			if err != nil {
				return nil, false, err
			}
			member.State = WorldMemberStateActive
			member.MutedUntil = nil
		}
		return member, false, nil
	}

	item := &WorldMemberModel{
		WorldID:  worldID,
		UserID:   userID,
		Nickname: nickname,
		State:    WorldMemberStateActive,
		JoinedAt: time.Now(),
	}
	return item, true, db.Create(item).Error
}

// WorldMemberBan 将成员标记为封禁
func WorldMemberBan(worldID, userID string, mutedUntil *time.Time) error {
	update := map[string]interface{}{
		"state":      WorldMemberStateBanned,
		"updated_at": time.Now(),
	}
	if mutedUntil != nil {
		update["muted_until"] = *mutedUntil
	} else {
		update["muted_until"] = nil
	}
	return db.Model(&WorldMemberModel{}).
		Where("world_id = ? AND user_id = ?", worldID, userID).
		Updates(update).Error
}

// WorldMemberList 拉取世界成员列表
func WorldMemberList(worldID string, limit, offset int) ([]*WorldMemberModel, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var items []*WorldMemberModel
	err := db.Where("world_id = ?", worldID).
		Order("joined_at asc").
		Limit(limit).
		Offset(offset).
		Find(&items).Error
	return items, err
}

// WorldMemberCount 统计成员数量
func WorldMemberCount(worldID string) (int64, error) {
	var cnt int64
	err := db.Model(&WorldMemberModel{}).
		Where("world_id = ? AND state = ?", worldID, WorldMemberStateActive).
		Count(&cnt).Error
	return cnt, err
}

// WorldMemberDelete 删除世界成员记录（仅用于回滚）
func WorldMemberDelete(worldID, userID string) error {
	return db.Where("world_id = ? AND user_id = ?", worldID, userID).
		Delete(&WorldMemberModel{}).Error
}

// WorldMemberSetRole 更新成员角色
func WorldMemberSetRole(worldID, userID, role string) error {
	allowed := map[string]struct{}{
		WorldMemberRoleOwner:  {},
		WorldMemberRoleAdmin:  {},
		WorldMemberRoleMember: {},
	}
	if _, ok := allowed[role]; !ok {
		return fmt.Errorf("unsupported role: %s", role)
	}
	return db.Model(&WorldMemberModel{}).
		Where("world_id = ? AND user_id = ?", worldID, userID).
		Update("role", role).Error
}
