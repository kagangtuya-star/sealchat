package model

import (
	"errors"
	"strings"
	//"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"sealchat/utils"
)

type WorldVisibility string

const (
	WorldVisibilityPublic  WorldVisibility = "public"
	WorldVisibilityPrivate WorldVisibility = "private"
)

type WorldJoinPolicy string

const (
	WorldJoinPolicyOpen       WorldJoinPolicy = "open"
	WorldJoinPolicyApproval   WorldJoinPolicy = "approval"
	WorldJoinPolicyInviteOnly WorldJoinPolicy = "invite_only"
)

// WorldModel 描述一个顶层世界/服务器
type WorldModel struct {
	StringPKBaseModel
	Name              string            `json:"name" gorm:"size:128;not null"`
	Slug              string            `json:"slug" gorm:"size:32;uniqueIndex"`
	Avatar            string            `json:"avatar" gorm:"size:255"`
	Banner            string            `json:"banner" gorm:"size:255"`
	Description       string            `json:"description" gorm:"type:text"`
	Visibility        WorldVisibility   `json:"visibility" gorm:"size:16;index;default:'public'"`
	JoinPolicy        WorldJoinPolicy   `json:"joinPolicy" gorm:"size:16;index;default:'approval'"`
	OwnerID           string            `json:"ownerId" gorm:"index"`
	DefaultChannelID  string            `json:"defaultChannelId" gorm:"index"`
	Settings          datatypes.JSON    `json:"settings" gorm:"type:json"`
	CreatedByInviteID string            `json:"createdByInviteId" gorm:"-"`
	MemberCount       int64             `json:"memberCount" gorm:"-"`
	Metadata          map[string]string `json:"metadata,omitempty" gorm:"-"`
	IsMember          bool              `json:"isMember" gorm:"-"`
	IsOwner           bool              `json:"isOwner" gorm:"-"`
}

func (*WorldModel) TableName() string {
	return "worlds"
}

func (w *WorldModel) BeforeCreate(tx *gorm.DB) error {
	if err := w.StringPKBaseModel.BeforeCreate(tx); err != nil {
		return err
	}
	if strings.TrimSpace(w.Slug) == "" {
		slug, err := generateWorldSlug(tx)
		if err != nil {
			return err
		}
		w.Slug = slug
	}
	if w.Visibility == "" {
		w.Visibility = WorldVisibilityPublic
	}
	if w.JoinPolicy == "" {
		w.JoinPolicy = WorldJoinPolicyApproval
	}
	return nil
}

func generateWorldSlug(tx *gorm.DB) (string, error) {
	for i := 0; i < 5; i++ {
		candidate := strings.ToLower(utils.NewIDWithLength(10))
		var exists int64
		if err := tx.Model(&WorldModel{}).
			Where("slug = ?", candidate).
			Count(&exists).Error; err != nil {
			return "", err
		}
		if exists == 0 {
			return candidate, nil
		}
	}
	return "", errors.New("世界标识生成失败")
}

// WorldGet 根据 ID 查询世界
func WorldGet(id string) (*WorldModel, error) {
	var item WorldModel
	err := db.Where("id = ?", id).Limit(1).Find(&item).Error
	return &item, err
}

// WorldGetBySlug 根据 slug 查询世界
func WorldGetBySlug(slug string) (*WorldModel, error) {
	var item WorldModel
	err := db.Where("slug = ?", slug).Limit(1).Find(&item).Error
	return &item, err
}

type WorldListOption struct {
	Query      string
	Visibility []WorldVisibility
	Limit      int
	Offset     int
	OwnerID    string
	MemberID   string
}

// WorldListPublic 提供世界大厅查询能力
func WorldListPublic(opt WorldListOption) ([]*WorldModel, error) {
	if opt.Limit <= 0 || opt.Limit > 100 {
		opt.Limit = 20
	}

	q := db.Model(&WorldModel{})
	vis := opt.Visibility
	if len(vis) == 0 {
		vis = []WorldVisibility{WorldVisibilityPublic}
	}

	owner := strings.TrimSpace(opt.OwnerID)
	member := strings.TrimSpace(opt.MemberID)

	cond := q.Where("visibility in ?", vis)
	if owner != "" {
		cond = cond.Or("owner_id = ?", owner)
	}
	if member != "" {
		cond = cond.Or("id IN (?)", db.Table("world_members").Select("world_id").Where("user_id = ?", member))
	}
	q = cond

	if s := strings.TrimSpace(opt.Query); s != "" {
		keyword := "%" + s + "%"
		q = q.Where("name LIKE ? OR description LIKE ?", keyword, keyword)
	}

	var items []*WorldModel
	err := q.Order("created_at desc").
		Limit(opt.Limit).
		Offset(opt.Offset).
		Find(&items).Error

	if err != nil {
		return items, err
	}

	// 标记属于当前用户的世界，便于前端过滤/展示
	if member != "" {
		memberWorldIDs := []string{}
		_ = db.Table("world_members").Select("world_id").Where("user_id = ?", member).Find(&memberWorldIDs)
		isMemberMap := map[string]struct{}{}
		for _, id := range memberWorldIDs {
			isMemberMap[id] = struct{}{}
		}
		for _, w := range items {
			if w == nil {
				continue
			}
			if strings.TrimSpace(w.OwnerID) == member {
				w.IsOwner = true
				w.IsMember = true
				continue
			}
			if _, ok := isMemberMap[w.ID]; ok {
				w.IsMember = true
			}
		}
	}

	return items, nil
}

// WorldSaveDefaultChannel 更新默认频道
func WorldSaveDefaultChannel(worldID, channelID string) error {
	return db.Model(&WorldModel{}).
		Where("id = ?", worldID).
		Update("default_channel_id", channelID).Error
}
