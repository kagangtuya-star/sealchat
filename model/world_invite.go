package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/utils"
)

// WorldInviteModel 记录世界层级的邀请信息
type WorldInviteModel struct {
	StringPKBaseModel
	WorldID     string     `json:"worldId" gorm:"index"`
	ChannelID   string     `json:"channelId" gorm:"index"`
	Code        string     `json:"code" gorm:"size:32;uniqueIndex"`
	CreatedBy   string     `json:"createdBy" gorm:"index"`
	ExpiredAt   *time.Time `json:"expiredAt" gorm:"index"`
	MaxUses     int        `json:"maxUses"`
	UsedCount   int        `json:"usedCount"`
	IsSingleUse bool       `json:"isSingleUse"`
	IsRevoked   bool       `json:"isRevoked"`
}

func (*WorldInviteModel) TableName() string {
	return "world_invites"
}

func (m *WorldInviteModel) BeforeCreate(tx *gorm.DB) error {
	if err := m.StringPKBaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	if strings.TrimSpace(m.Code) == "" {
		code, err := generateWorldInviteCode(tx)
		if err != nil {
			return err
		}
		m.Code = code
	}
	return nil
}

// IsExhausted 检查次数是否用尽
func (m *WorldInviteModel) IsExhausted() bool {
	if m.MaxUses <= 0 {
		return false
	}
	return m.UsedCount >= m.MaxUses
}

// IsExpired 判断邀请是否失效
func (m *WorldInviteModel) IsExpired() bool {
	if m.IsRevoked {
		return true
	}
	if m.IsExhausted() {
		return true
	}
	if m.ExpiredAt == nil {
		return false
	}
	return time.Now().After(*m.ExpiredAt)
}

// TryConsume 使用一次邀请
func (m *WorldInviteModel) TryConsume() error {
	if m.IsExpired() {
		return errors.New("邀请已失效")
	}
	err := db.Model(&WorldInviteModel{}).
		Where("id = ?", m.ID).
		UpdateColumn("used_count", gorm.Expr("used_count + 1")).Error
	if err != nil {
		return err
	}
	m.UsedCount++
	return nil
}

// Revoke 吊销邀请
func (m *WorldInviteModel) Revoke() error {
	m.IsRevoked = true
	return db.Model(&WorldInviteModel{}).
		Where("id = ?", m.ID).
		Update("is_revoked", true).Error
}

// WorldInviteGetByCode 根据 code 查询邀请
func WorldInviteGetByCode(code string) (*WorldInviteModel, error) {
	var item WorldInviteModel
	err := db.Where("code = ?", strings.TrimSpace(strings.ToUpper(code))).
		Limit(1).
		Find(&item).Error
	return &item, err
}

func generateWorldInviteCode(tx *gorm.DB) (string, error) {
	for i := 0; i < 5; i++ {
		code := strings.ToUpper(utils.NewIDWithLength(8))
		var exists int64
		if err := tx.Model(&WorldInviteModel{}).
			Where("code = ?", code).
			Count(&exists).Error; err != nil {
			return "", err
		}
		if exists == 0 {
			return code, nil
		}
	}
	return "", errors.New("邀请代码生成失败")
}

// WorldInviteLogModel 记录邀请使用日志
type WorldInviteLogModel struct {
	StringPKBaseModel
	InviteID  string    `json:"inviteId" gorm:"index"`
	WorldID   string    `json:"worldId" gorm:"index"`
	UserID    string    `json:"userId" gorm:"index"`
	UsedByIP  string    `json:"usedByIp" gorm:"size:64"`
	UserAgent string    `json:"userAgent"`
	UsedAt    time.Time `json:"usedAt"`
	Note      string    `json:"note"`
}

func (*WorldInviteLogModel) TableName() string {
	return "world_invite_logs"
}

func (m *WorldInviteLogModel) BeforeCreate(tx *gorm.DB) error {
	if err := m.StringPKBaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	if m.UsedAt.IsZero() {
		m.UsedAt = time.Now()
	}
	return nil
}

// WorldInviteLogCreate 写入日志
func WorldInviteLogCreate(logItem *WorldInviteLogModel) error {
	return db.Create(logItem).Error
}
