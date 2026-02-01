package model

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/utils"
)

const (
	WorldVisibilityPublic   = "public"
	WorldVisibilityPrivate  = "private"
	WorldVisibilityUnlisted = "unlisted"

	WorldRoleOwner     = "owner"
	WorldRoleAdmin     = "admin"
	WorldRoleMember    = "member"
	WorldRoleSpectator = "spectator"
)

// WorldModel 表示"世界"实体，承载频道集合与可见性配置。
type WorldModel struct {
	StringPKBaseModel
	Name                    string `json:"name" gorm:"size:100;not null"`
	Description             string `json:"description" gorm:"size:500"`
	Avatar                  string `json:"avatar" gorm:"size:255"`
	Visibility              string `json:"visibility" gorm:"size:24;default:public;index"` // public/private/unlisted
	EnforceMembership       bool   `json:"enforceMembership" gorm:"default:false"`         // 预留未来严格控制
	AllowAdminEditMessages  bool   `json:"allowAdminEditMessages" gorm:"default:false"`    // 允许管理员编辑成员发言
	AllowMemberEditKeywords bool   `json:"allowMemberEditKeywords" gorm:"default:false"`   // 允许成员编辑世界术语
	CharacterCardBadgeTemplate string `json:"characterCardBadgeTemplate" gorm:"size:512"` // 世界徽章模板
	IsSystemDefault         bool   `json:"isSystemDefault" gorm:"default:false;index"`     // 系统默认世界标识，仅允许一个
	OwnerID                 string `json:"ownerId" gorm:"size:100;index"`
	DefaultChannelID        string `json:"defaultChannelId" gorm:"size:100"`
	InviteSlug              string `json:"inviteSlug" gorm:"size:64;uniqueIndex"`
	Status                  string `json:"status" gorm:"size:24;default:active;index"`
}

func (*WorldModel) TableName() string {
	return "worlds"
}

func (m *WorldModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.Init()
	}
	if strings.TrimSpace(m.InviteSlug) == "" {
		m.InviteSlug = strings.ToLower(utils.NewIDWithLength(10))
	}
	if strings.TrimSpace(m.Visibility) == "" {
		m.Visibility = "public"
	}
	if strings.TrimSpace(m.Status) == "" {
		m.Status = "active"
	}
	return nil
}

// WorldMemberModel 记录用户与世界的关系与角色。
type WorldMemberModel struct {
	StringPKBaseModel
	WorldID           string     `json:"worldId" gorm:"size:100;index:idx_world_member,priority:1"`
	UserID            string     `json:"userId" gorm:"size:100;index:idx_world_member,priority:2"`
	Role              string     `json:"role" gorm:"size:24;index"` // owner/admin/member
	JoinedAt          time.Time  `json:"joinedAt"`
	EditNoticeAckedAt *time.Time `json:"editNoticeAckedAt"` // 确认管理员编辑提示的时间
}

func (*WorldMemberModel) TableName() string {
	return "world_members"
}

// WorldInviteModel 用于生成邀请链接。
type WorldInviteModel struct {
	StringPKBaseModel
	WorldID   string     `json:"worldId" gorm:"size:100;index"`
	CreatorID string     `json:"creatorId" gorm:"size:100;index"`
	Slug      string     `json:"slug" gorm:"size:64;uniqueIndex"`
	Role      string     `json:"role" gorm:"size:24;default:member"`
	ExpireAt  *time.Time `json:"expireAt"`
	MaxUse    int        `json:"maxUse"`
	UsedCount int        `json:"usedCount"`
	Status    string     `json:"status" gorm:"size:24;default:active;index"`
	Memo      string     `json:"memo" gorm:"size:255"`
}

func (*WorldInviteModel) TableName() string {
	return "world_invites"
}

func (m *WorldInviteModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.Init()
	}
	if strings.TrimSpace(m.Slug) == "" {
		m.Slug = strings.ToLower(utils.NewIDWithLength(12))
	}
	if strings.TrimSpace(m.Role) == "" {
		m.Role = WorldRoleMember
	}
	if strings.TrimSpace(m.Status) == "" {
		m.Status = "active"
	}
	return nil
}

// BackfillWorldData 在迁移时保证存在至少一个默认世界，并把无归属的频道归档到默认世界。
func BackfillWorldData() error {
	db := GetDB()
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&WorldModel{}) {
		return nil
	}

	var world WorldModel
	// 优先查找已标记的系统默认世界（按创建时间排序确保确定性）
	if err := db.Where("is_system_default = ? AND status = ?", true, "active").
		Order("created_at asc").Limit(1).Find(&world).Error; err != nil {
		return err
	}

	if world.ID == "" {
		// 如果没有系统默认世界，检查是否有旧的默认世界需要迁移
		var oldDefault WorldModel
		if err := db.Where("status = ?", "active").
			Order("created_at asc").Limit(1).Find(&oldDefault).Error; err != nil {
			return err
		}

		if oldDefault.ID != "" {
			// 将最早的世界标记为系统默认世界（迁移兼容）
			// 使用 map 确保即使值为 false 也能正确更新
			result := db.Model(&WorldModel{}).Where("id = ?", oldDefault.ID).
				Updates(map[string]interface{}{"is_system_default": true})
			if result.Error != nil {
				return result.Error
			}
			world = oldDefault
			world.IsSystemDefault = true
		} else {
			// 没有任何世界，创建新的系统默认世界
			world = WorldModel{
				Name:            "默认世界",
				Description:     "系统初始化自动创建的默认世界",
				Visibility:      "public",
				IsSystemDefault: true,
				Status:          "active",
			}
			if err := db.Create(&world).Error; err != nil {
				return err
			}
		}
	}
	if world.ID == "" {
		return nil
	}

	// 回填缺少 world_id 的频道
	if err := db.Model(&ChannelModel{}).
		Where("world_id = '' OR world_id IS NULL").
		Update("world_id", world.ID).Error; err != nil {
		return err
	}
	_ = db.Model(&ChannelModel{}).
		Where("status = '' OR status IS NULL").
		Update("status", "active")

	// 如果默认世界没有默认频道记录，尝试写入一个已有频道
	if strings.TrimSpace(world.DefaultChannelID) == "" {
		var ch ChannelModel
		if err := db.Where("world_id = ?", world.ID).Order("created_at asc").Limit(1).Find(&ch).Error; err == nil {
			if ch.ID != "" {
				_ = db.Model(&WorldModel{}).
					Where("id = ?", world.ID).
					Update("default_channel_id", ch.ID).Error
			}
		}
	}
	return nil
}
