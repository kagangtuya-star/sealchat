package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

const DefaultWorldChannelName = "公共休息室"

var (
	ErrWorldNameRequired   = errors.New("世界名称不能为空")
	ErrWorldInviteInvalid  = errors.New("邀请不可用")
	ErrWorldInviteExpired  = errors.New("邀请已失效")
	ErrWorldInviteNoUser   = errors.New("用户信息缺失，无法加入世界")
	ErrWorldNotFound       = errors.New("世界不存在或已被删除")
	ErrWorldMemberRequired = errors.New("用户不是该世界成员")
	ErrWorldJoinApproval   = errors.New("需要加入世界后才能访问")
)

// EnsureWorldMemberActive 校验用户是否为世界有效成员
func EnsureWorldMemberActive(worldID, userID string) error {
	worldID = strings.TrimSpace(worldID)
	userID = strings.TrimSpace(userID)
	if worldID == "" || userID == "" {
		return ErrWorldMemberRequired
	}
	member, err := model.WorldMemberGet(worldID, userID)
	if err != nil {
		return err
	}
	if member != nil {
		if member.State == model.WorldMemberStateActive {
			return nil
		}
		return ErrWorldMemberRequired
	}

	return rebuildWorldMemberRecord(worldID, userID)
}

func rebuildWorldMemberRecord(worldID, userID string) error {
	nickname := fallbackWorldMemberNickname(userID)

	world, err := model.WorldGet(worldID)
	if err != nil {
		return err
	}
	if world != nil && world.ID != "" && strings.TrimSpace(world.OwnerID) == userID {
		_, _, err := model.WorldMemberEnsureActive(worldID, userID, nickname)
		return err
	}

	if world != nil && world.ID != "" && world.Visibility == model.WorldVisibilityPublic && world.JoinPolicy == model.WorldJoinPolicyOpen {
		_, _, err := model.WorldMemberEnsureActive(worldID, userID, nickname)
		return err
	}

	exists, checkErr := model.MemberExistsInWorld(worldID, userID)
	if checkErr != nil {
		return checkErr
	}
	if exists {
		_, _, err := model.WorldMemberEnsureActive(worldID, userID, nickname)
		return err
	}

	if world != nil && world.ID != "" {
		return ErrWorldJoinApproval
	}
	return ErrWorldMemberRequired
}

func fallbackWorldMemberNickname(userID string) string {
	if user := model.UserGet(userID); user != nil {
		return user.Nickname
	}
	return ""
}

func cleanupChannelMemberships(tx *gorm.DB, worldID string, userID string) error {
	return tx.Where("world_id = ? AND user_id = ?", worldID, userID).
		Delete(&model.MemberModel{}).Error
}

// CreateWorldOptions 定义新建世界的入参
type CreateWorldOptions struct {
	Name        string
	Description string
	Avatar      string
	Banner      string
	OwnerID     string
	Visibility  model.WorldVisibility
	JoinPolicy  model.WorldJoinPolicy
	Settings    map[string]interface{}
}

// CreateWorld 创建顶层世界
func CreateWorld(opts CreateWorldOptions) (*model.WorldModel, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, ErrWorldNameRequired
	}
	world := &model.WorldModel{
		Name:        name,
		Description: strings.TrimSpace(opts.Description),
		Avatar:      strings.TrimSpace(opts.Avatar),
		Banner:      strings.TrimSpace(opts.Banner),
		OwnerID:     opts.OwnerID,
		Visibility:  opts.Visibility,
		JoinPolicy:  opts.JoinPolicy,
	}
	if len(opts.Settings) > 0 {
		payload, err := json.Marshal(opts.Settings)
		if err != nil {
			return nil, err
		}
		world.Settings = datatypes.JSON(payload)
	}

	if err := model.GetDB().Create(world).Error; err != nil {
		return nil, err
	}

	if strings.TrimSpace(world.OwnerID) != "" {
		if _, _, err := model.WorldMemberEnsureActive(world.ID, world.OwnerID, fallbackWorldMemberNickname(world.OwnerID)); err != nil {
			return nil, err
		}
	}

	if err := ensureWorldDefaultChannel(world); err != nil {
		return nil, err
	}

	setDefaultWorld(world)
	return world, nil
}

func ensureWorldDefaultChannel(world *model.WorldModel) error {
	if world == nil || world.ID == "" {
		return nil
	}
	ownerID := strings.TrimSpace(world.OwnerID)
	if ownerID == "" {
		return nil
	}
	if strings.TrimSpace(world.DefaultChannelID) != "" {
		return nil
	}

	channel, err := ChannelNew(utils.NewID(), "public", DefaultWorldChannelName, ownerID, "", world.ID)
	if err != nil {
		return err
	}
	world.DefaultChannelID = channel.ID
	if err := model.WorldSaveDefaultChannel(world.ID, channel.ID); err != nil {
		return err
	}
	return nil
}

type UpdateWorldOptions struct {
	Name        string
	Description string
	Avatar      string
	Banner      string
	Visibility  model.WorldVisibility
	JoinPolicy  model.WorldJoinPolicy
	Settings    map[string]interface{}
}

func UpdateWorld(worldID string, opts UpdateWorldOptions) (*model.WorldModel, error) {
	world, err := model.WorldGet(worldID)
	if err != nil {
		return nil, err
	}
	if world == nil || world.ID == "" {
		return nil, ErrWorldNotFound
	}
	updates := map[string]interface{}{}
	if strings.TrimSpace(opts.Name) != "" {
		updates["name"] = strings.TrimSpace(opts.Name)
	}
	updates["description"] = strings.TrimSpace(opts.Description)
	updates["avatar"] = strings.TrimSpace(opts.Avatar)
	updates["banner"] = strings.TrimSpace(opts.Banner)
	if opts.Visibility != "" {
		updates["visibility"] = opts.Visibility
	}
	if opts.JoinPolicy != "" {
		updates["join_policy"] = opts.JoinPolicy
	}
	if len(opts.Settings) > 0 {
		payload, err := json.Marshal(opts.Settings)
		if err != nil {
			return nil, err
		}
		updates["settings"] = datatypes.JSON(payload)
	}
	updates["updated_at"] = time.Now()
	if err := model.GetDB().Model(&model.WorldModel{}).
		Where("id = ?", worldID).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	return model.WorldGet(worldID)
}

func RemoveWorldMember(worldID, targetUserID string) error {
	world, err := model.WorldGet(worldID)
	if err != nil {
		return err
	}
	if world == nil || world.ID == "" {
		return ErrWorldNotFound
	}
	if strings.TrimSpace(world.OwnerID) == targetUserID {
		return fmt.Errorf("世界拥有者无法被移除")
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("world_id = ? AND user_id = ?", worldID, targetUserID).
			Delete(&model.WorldMemberModel{}).Error; err != nil {
			return err
		}
		if err := cleanupChannelMemberships(tx, worldID, targetUserID); err != nil {
			return err
		}
		return nil
	})
}

func DeleteWorld(worldID string) error {
	world, err := model.WorldGet(worldID)
	if err != nil {
		return err
	}
	if world == nil || world.ID == "" {
		return ErrWorldNotFound
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("world_id = ?", worldID).Delete(&model.WorldInviteLogModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("world_id = ?", worldID).Delete(&model.WorldInviteModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("world_id = ?", worldID).Delete(&model.WorldMemberModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("world_id = ?", worldID).Delete(&model.MemberModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("world_id = ?", worldID).Delete(&model.ChannelModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", worldID).Delete(&model.WorldModel{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// GetWorldBySlug 从 slug 加载世界信息
func GetWorldBySlug(slug string) (*model.WorldModel, error) {
	item, err := model.WorldGetBySlug(strings.TrimSpace(slug))
	if err != nil {
		return nil, err
	}
	if item == nil || item.ID == "" {
		return nil, ErrWorldNotFound
	}
	return item, nil
}

// ListWorlds 拉取世界大厅数据
type ListWorldOption struct {
	Query    string
	Limit    int
	Offset   int
	OwnerID  string
	MemberID string
}

func ListWorlds(opt ListWorldOption) ([]*model.WorldModel, error) {
	return model.WorldListPublic(model.WorldListOption{
		Query:    opt.Query,
		Limit:    opt.Limit,
		Offset:   opt.Offset,
		OwnerID:  opt.OwnerID,
		MemberID: opt.MemberID,
	})
}

// JoinWorldResult 描述加入世界之后的状态
type JoinWorldResult struct {
	Member      *model.WorldMemberModel
	MemberNewly bool
}

// JoinWorld 供审批通过或默认加入场景复用
func JoinWorld(worldID, userID, nickname string) (*JoinWorldResult, error) {
	member, created, err := model.WorldMemberEnsureActive(worldID, userID, nickname)
	if err != nil {
		return nil, err
	}
	return &JoinWorldResult{
		Member:      member,
		MemberNewly: created,
	}, nil
}

// CreateWorldInviteOptions 建立邀请链接所需的信息
type CreateWorldInviteOptions struct {
	WorldID     string
	ChannelID   string
	CreatorID   string
	ExpiredAt   *time.Time
	MaxUses     int
	IsSingleUse bool
}

// CreateWorldInvite 生成世界级邀请
func CreateWorldInvite(opts CreateWorldInviteOptions) (*model.WorldInviteModel, error) {
	if strings.TrimSpace(opts.WorldID) == "" || strings.TrimSpace(opts.CreatorID) == "" {
		return nil, ErrWorldInviteInvalid
	}
	invite := &model.WorldInviteModel{
		WorldID:     opts.WorldID,
		ChannelID:   opts.ChannelID,
		CreatedBy:   opts.CreatorID,
		ExpiredAt:   opts.ExpiredAt,
		MaxUses:     opts.MaxUses,
		IsSingleUse: opts.IsSingleUse,
	}
	if invite.IsSingleUse && invite.MaxUses == 0 {
		invite.MaxUses = 1
	}
	if err := model.GetDB().Create(invite).Error; err != nil {
		return nil, err
	}
	return invite, nil
}

// AcceptWorldInviteOptions 处理邀请时需要的上下文
type AcceptWorldInviteOptions struct {
	Code      string
	UserID    string
	Nickname  string
	IPAddress string
	UserAgent string
}

// AcceptWorldInviteResult 包含一次邀请加入的输出
type AcceptWorldInviteResult struct {
	Invite        *model.WorldInviteModel
	World         *model.WorldModel
	Member        *model.WorldMemberModel
	MemberCreated bool
}

// AcceptWorldInvite 解析并消费邀请
func AcceptWorldInvite(opts AcceptWorldInviteOptions) (*AcceptWorldInviteResult, error) {
	code := strings.TrimSpace(opts.Code)
	if code == "" {
		return nil, ErrWorldInviteInvalid
	}
	if strings.TrimSpace(opts.UserID) == "" {
		return nil, ErrWorldInviteNoUser
	}

	invite, err := model.WorldInviteGetByCode(code)
	if err != nil {
		return nil, err
	}
	if invite == nil || invite.ID == "" {
		return nil, ErrWorldInviteInvalid
	}
	if invite.IsExpired() {
		return nil, ErrWorldInviteExpired
	}

	world, err := model.WorldGet(invite.WorldID)
	if err != nil {
		return nil, err
	}
	if world == nil || world.ID == "" {
		return nil, ErrWorldNotFound
	}

	member, created, err := model.WorldMemberEnsureActive(invite.WorldID, opts.UserID, opts.Nickname)
	if err != nil {
		return nil, err
	}

	if err := invite.TryConsume(); err != nil {
		if created {
			_ = model.WorldMemberDelete(invite.WorldID, opts.UserID)
		}
		return nil, err
	}

	_ = model.WorldInviteLogCreate(&model.WorldInviteLogModel{
		InviteID:  invite.ID,
		WorldID:   invite.WorldID,
		UserID:    opts.UserID,
		UsedByIP:  strings.TrimSpace(opts.IPAddress),
		UserAgent: opts.UserAgent,
	})

	return &AcceptWorldInviteResult{
		Invite:        invite,
		World:         world,
		Member:        member,
		MemberCreated: created,
	}, nil
}

// ListWorldMembers 查询世界成员
func ListWorldMembers(worldID string, limit, offset int) ([]*model.WorldMemberModel, error) {
	return model.WorldMemberList(worldID, limit, offset)
}

// ListWorldInvites 返回世界邀请列表
func ListWorldInvites(worldID string, limit int) ([]*model.WorldInviteModel, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var invites []*model.WorldInviteModel
	err := model.GetDB().
		Where("world_id = ?", worldID).
		Order("created_at desc").
		Limit(limit).
		Find(&invites).Error
	return invites, err
}

// InviteSummary 提供邀请信息与世界信息的聚合
type InviteSummary struct {
	Invite *model.WorldInviteModel `json:"invite"`
	World  *model.WorldModel       `json:"world"`
}

// GetWorldInviteSummary 返回邀请概览
func GetWorldInviteSummary(code string) (*InviteSummary, error) {
	invite, err := model.WorldInviteGetByCode(code)
	if err != nil {
		return nil, err
	}
	if invite == nil || invite.ID == "" {
		return nil, ErrWorldInviteInvalid
	}
	world, err := model.WorldGet(invite.WorldID)
	if err != nil {
		return nil, err
	}
	if world == nil || world.ID == "" {
		return nil, ErrWorldNotFound
	}
	return &InviteSummary{
		Invite: invite,
		World:  world,
	}, nil
}
