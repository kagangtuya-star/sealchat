package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mikespook/gorbac"

	"github.com/samber/lo"

	"sealchat/model"
	"sealchat/pm"
)

// ChannelIdList 获取可见的频道ID，这个函数是下面的修改版，理论上会更精确，等待实际验证。可能有些调用代价，后面可以考虑使用memoize，也可能使用层级权限是更好的方式。
func ChannelIdList(userId string) ([]string, error) {
	// 包括如下内容:
	// 1. 属性为可见的一级频道(即没有父级的频道)
	// 2. 具有明确可看权限的频道(先查频道角色，再根据频道角色验证权限和获取频道id)
	// 3. 补入有权查看的频道的子频道

	roles, err := model.UserRoleMappingListByUserID(userId, "", "channel")
	if err != nil {
		return nil, err
	}

	var rolesCanRead []string
	db := model.GetDB()
	db.Model(&model.RolePermissionModel{}).
		Where("role_id in ? and permission_id in ?", roles, []string{pm.PermFuncChannelRead.ID(), pm.PermFuncChannelReadAll.ID()}).
		Pluck("role_id", &rolesCanRead)

	// 获得1 公开的一级频道
	var idsPublic1 []string
	db.Model(&model.ChannelModel{}).Where("coalesce(root_id, '') = '' and perm_type = ?", "public").
		Pluck("id", &idsPublic1)

	// 这里获得的是2: 具有明确可看权限的频道，包括公开频道和非公开频道
	ids2 := lo.Map(rolesCanRead, func(item string, index int) string {
		return strings.SplitN(item, "-", 3)[1]
	})

	// 将公开一级频道和有权限的频道组合起来
	idsCanRead := append(idsPublic1, ids2...)

	// 值得注意，ids2里可能混合了空中楼阁子频道，也就是说你没有他上级频道的权限
	// 要在之后进行剔除。虽然目前版本不支持2级以上频道，所以理论上不会存在

	// 3.1: 在可访问频道的基础上进一步加入公开的子频道
	var ids3 []string
	db.Model(&model.ChannelModel{}).Where("root_id in ? and perm_type = ?", idsCanRead, "public").
		Pluck("id", &ids3)

	// 3.2
	// 先找出我有“查看全部”权限的的顶级频道
	// 找出这些顶级频道的下属非公开频道
	var rolesCanRead2 []string
	db.Model(&model.RolePermissionModel{}).
		Where("role_id in ? and permission_id in ?", roles, []string{pm.PermFuncChannelReadAll.ID()}).
		Pluck("role_id", &rolesCanRead2)
	ids2x := lo.Map(rolesCanRead2, func(item string, index int) string {
		return strings.SplitN(item, "-", 3)[1]
	})
	var ids32 []string
	db.Model(&model.ChannelModel{}).Where("root_id in ? and perm_type = ?", ids2x, "non-public").
		Pluck("id", &ids32)

	idsCanRead = append(idsCanRead, ids3...)
	idsCanRead = append(idsCanRead, ids32...)

	// 对idsCanRead进行去重
	idsCanRead = lo.Uniq(idsCanRead)

	// 追加私聊频道ID，使其也参与未读统计
	if privateIDs, err := model.FriendChannelIDList(userId); err == nil {
		idsCanRead = append(idsCanRead, privateIDs...)
	}
	idsCanRead = lo.Uniq(idsCanRead)

	// 剔除父频道不在可读列表中的频道，但保留顶级频道
	var idsParentNotInCanRead []string
	db.Model(&model.ChannelModel{}).
		Where("id in ? and coalesce(parent_id,'') != '' and parent_id not in ?", idsCanRead, idsCanRead).
		Pluck("id", &idsParentNotInCanRead)

	idsCanRead = lo.Filter(idsCanRead, func(id string, _ int) bool {
		return !lo.Contains(idsParentNotInCanRead, id)
	})

	return idsCanRead, nil
}

func ChannelIdListByWorld(userId, worldID string, includePrivate bool) ([]string, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return ChannelIdList(userId)
	}
	allowed, err := ChannelIdList(userId)
	if err != nil {
		return nil, err
	}
	allowedSet := map[string]struct{}{}
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}

	channels, err := ChannelListByWorld(worldID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(channels))
	for _, ch := range channels {
		if ch == nil || strings.TrimSpace(ch.ID) == "" {
			continue
		}
		if _, ok := allowedSet[ch.ID]; ok {
			ids = append(ids, ch.ID)
		}
	}

	if includePrivate {
		if privateIDs, err := model.FriendChannelIDList(userId); err == nil {
			ids = append(ids, privateIDs...)
		}
	}

	return lo.Uniq(ids), nil
}

// CanReadChannelByUserId 注意性能比较差，后面修改
func CanReadChannelByUserId(userId, channelId string) bool {
	if strings.TrimSpace(channelId) == "" {
		return false
	}
	if len(channelId) < 30 {
		if ch, err := model.ChannelGet(channelId); err == nil && ch != nil {
			if ch.ID == "" {
				return false
			}
			if !ch.IsPrivate && strings.TrimSpace(ch.WorldID) != "" {
				if !IsWorldMember(ch.WorldID, userId) {
					return false
				}
			}
		}
	}
	chIds, _ := ChannelIdList(userId)
	return lo.Contains(chIds, channelId)
}

// ChannelList 获取可见的频道（等待重构）
func ChannelList(userId, worldID string) ([]*model.ChannelModel, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return []*model.ChannelModel{}, nil
	}
	isSystemAdmin := pm.CanWithSystemRole(userId, pm.PermModAdmin)
	if !IsWorldMember(worldID, userId) && !isSystemAdmin {
		return []*model.ChannelModel{}, nil
	}
	channels, err := ChannelListByWorld(worldID)
	if err != nil {
		return nil, err
	}
	allowed, _ := ChannelIdList(userId)
	allowedSet := map[string]struct{}{}
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	visible := make([]*model.ChannelModel, 0, len(channels))
	for _, ch := range channels {
		if ch == nil || strings.TrimSpace(ch.ID) == "" {
			continue
		}
		if isSystemAdmin {
			visible = append(visible, ch)
			continue
		}
		if _, ok := allowedSet[ch.ID]; ok {
			visible = append(visible, ch)
		}
	}
	return visible, nil
}

func ChannelListByWorld(worldID string) ([]*model.ChannelModel, error) {
	var items []*model.ChannelModel
	if strings.TrimSpace(worldID) == "" {
		return items, nil
	}
	err := model.GetDB().
		Where("world_id = ? AND status = ? AND is_private = ?", worldID, "active", false).
		Order("sort_order DESC").
		Order("created_at ASC").
		Find(&items).Error
	return items, err
}

func ChannelListPublicByWorld(worldID string) ([]*model.ChannelModel, error) {
	channels, err := ChannelListByWorld(worldID)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return channels, nil
	}
	channelMap := map[string]*model.ChannelModel{}
	for _, ch := range channels {
		if ch != nil && strings.TrimSpace(ch.ID) != "" {
			channelMap[ch.ID] = ch
		}
	}
	visible := make([]*model.ChannelModel, 0, len(channels))
	for _, ch := range channels {
		if ch == nil || strings.TrimSpace(ch.ID) == "" {
			continue
		}
		if strings.ToLower(strings.TrimSpace(ch.PermType)) != "public" {
			continue
		}
		if ch.RootId != "" {
			root := channelMap[ch.RootId]
			if root == nil || strings.ToLower(strings.TrimSpace(root.PermType)) != "public" {
				continue
			}
		}
		visible = append(visible, ch)
	}
	return visible, nil
}

func CanGuestAccessChannel(channelID string) (*model.ChannelModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.New("频道ID不能为空")
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || strings.TrimSpace(channel.ID) == "" {
		return nil, errors.New("频道不存在")
	}
	if channel.IsPrivate || strings.ToLower(strings.TrimSpace(channel.PermType)) != "public" {
		return nil, errors.New("频道不可公开访问")
	}
	if channel.RootId != "" {
		root, err := model.ChannelGet(channel.RootId)
		if err != nil {
			return nil, err
		}
		if root == nil || strings.TrimSpace(root.ID) == "" {
			return nil, errors.New("频道不可公开访问")
		}
		if strings.ToLower(strings.TrimSpace(root.PermType)) != "public" {
			return nil, errors.New("频道不可公开访问")
		}
	}
	if channel.WorldID != "" {
		world, err := GetWorldByID(channel.WorldID)
		if err != nil {
			return nil, err
		}
		if world == nil || strings.ToLower(strings.TrimSpace(world.Visibility)) != model.WorldVisibilityPublic {
			return nil, errors.New("世界未开放公开访问")
		}
	}
	return channel, nil
}

func ChannelNew(channelID, channelType, channelName, worldID, creatorId, parentId string) *model.ChannelModel {
	if strings.TrimSpace(worldID) == "" {
		if w, err := GetOrCreateDefaultWorld(); err == nil && w != nil {
			worldID = w.ID
		}
	}

	m := model.ChannelPublicNew(channelID, &model.ChannelModel{
		WorldID:            worldID,
		Name:               channelName,
		PermType:           channelType,
		ParentID:           parentId,
		RootId:             parentId, // TODO: 这个是不准的，但是目前不允许二级以上子频道
		DefaultDiceExpr:    "d20",
		BuiltInDiceEnabled: true,
		BotFeatureEnabled:  false,
	}, creatorId)

	roleCreate(channelID, "owner", "群主", func(roleId string) []gorbac.Permission {
		return []gorbac.Permission{
			pm.PermFuncChannelRead,
			pm.PermFuncChannelTextSend,
			pm.PermFuncChannelFileSend,
			pm.PermFuncChannelAudioSend,
			pm.PermFuncChannelInvite,
			// pm.PermFuncChannelMemberRemove,
			pm.PermFuncChannelSubChannelCreate,
			pm.PermFuncChannelRoleLink,
			pm.PermFuncChannelRoleUnlink,
			pm.PermFuncChannelRoleLinkRoot,
			pm.PermFuncChannelRoleUnlinkRoot,
			pm.PermFuncChannelManageInfo,
			pm.PermFuncChannelManageRole,
			pm.PermFuncChannelManageRoleRoot,
			pm.PermFuncChannelManageMute,
			pm.PermFuncChannelReadAll,
			pm.PermFuncChannelTextSendAll,
			pm.PermFuncChannelManageGallery,
			pm.PermFuncChannelIFormManage,
			pm.PermFuncChannelIFormBroadcast,
		}
	})

	roleCreate(channelID, "admin", "管理员", func(roleId string) []gorbac.Permission {
		return []gorbac.Permission{
			pm.PermFuncChannelRead,
			pm.PermFuncChannelTextSend,
			pm.PermFuncChannelFileSend,
			pm.PermFuncChannelAudioSend,
			pm.PermFuncChannelInvite,
			// pm.PermFuncChannelMemberRemove,
			pm.PermFuncChannelSubChannelCreate,
			pm.PermFuncChannelRoleLink,
			pm.PermFuncChannelRoleUnlink,
			pm.PermFuncChannelReadAll,
			pm.PermFuncChannelManageInfo,
			pm.PermFuncChannelManageRole,
			pm.PermFuncChannelManageMute,
			pm.PermFuncChannelTextSendAll,
			pm.PermFuncChannelManageGallery,
			pm.PermFuncChannelIFormManage,
			pm.PermFuncChannelIFormBroadcast,
		}
	})

	roleCreate(channelID, "ob", "观察者", func(roleId string) []gorbac.Permission {
		return []gorbac.Permission{
			pm.PermFuncChannelRead,
			pm.PermFuncChannelTextSend,
			pm.PermFuncChannelFileSend,
			pm.PermFuncChannelAudioSend,
			pm.PermFuncChannelReadAll,
		}
	})

	ensureChannelSpectatorRole(channelID)

	roleCreate(channelID, "visitor", "游客", func(roleId string) []gorbac.Permission {
		return []gorbac.Permission{
			pm.PermFuncChannelRead,
			pm.PermFuncChannelTextSend,
		}
	})

	roleCreate(channelID, "member", "成员", func(roleId string) []gorbac.Permission {
		return []gorbac.Permission{
			pm.PermFuncChannelRead,
			pm.PermFuncChannelTextSend,
			pm.PermFuncChannelFileSend,
			pm.PermFuncChannelAudioSend,
			pm.PermFuncChannelInvite,
		}
	})

	roleCreate(channelID, "bot", "机器人", func(roleId string) []gorbac.Permission {
		return []gorbac.Permission{
			pm.PermFuncChannelReadAll,
			pm.PermFuncChannelTextSendAll,
		}
	})

	roleId := fmt.Sprintf("ch-%s-%s", channelID, "owner")
	_ = model.UserRoleMappingCreate(&model.UserRoleMappingModel{
		UserID:   creatorId,
		RoleID:   roleId,
		RoleType: "channel",
	})

	syncWorldRolesForNewChannel(worldID, channelID)

	return m
}

func ensureChannelSpectatorRole(channelID string) {
	roleID := fmt.Sprintf("ch-%s-%s", channelID, "spectator")
	role, err := model.ChannelRoleGet(roleID)
	if err == nil && role != nil && role.ID != "" {
		return
	}
	roleCreate(channelID, "spectator", "旁观者", func(roleId string) []gorbac.Permission {
		return []gorbac.Permission{
			pm.PermFuncChannelRead,
			pm.PermFuncChannelReadAll,
		}
	})
}

func ChannelDissolve(channelID, operatorID string) (err error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return errors.New("channelId 不能为空")
	}

	ch, err := model.ChannelGet(channelID)
	if err != nil {
		return err
	}
	if ch == nil || strings.TrimSpace(ch.ID) == "" {
		return errors.New("频道不存在")
	}
	if ch.Status == "deleted" {
		return errors.New("频道已被解散")
	}
	if ch.IsPrivate || strings.EqualFold(ch.PermType, "private") {
		return errors.New("私聊频道无法解散")
	}

	if strings.TrimSpace(ch.WorldID) != "" {
		var world model.WorldModel
		if err := model.GetDB().Where("id = ?", ch.WorldID).Limit(1).Find(&world).Error; err == nil {
			if world.ID != "" && strings.TrimSpace(world.DefaultChannelID) == ch.ID {
				return errors.New("世界默认频道无法解散")
			}
		}
	}

	tx := model.GetDB().Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	if err = tx.Model(&model.ChannelModel{}).
		Where("id = ?", channelID).
		Updates(map[string]any{
			"status":     "deleted",
			"updated_at": time.Now(),
		}).Error; err != nil {
		return err
	}

	if err = tx.Where("channel_id = ?", channelID).Delete(&model.MemberModel{}).Error; err != nil {
		return err
	}

	rolePattern := fmt.Sprintf("ch-%s-%%", channelID)
	if err = tx.Where("role_id LIKE ?", rolePattern).Delete(&model.UserRoleMappingModel{}).Error; err != nil {
		return err
	}
	if err = tx.Where("role_id LIKE ?", rolePattern).Delete(&model.RolePermissionModel{}).Error; err != nil {
		return err
	}
	if err = tx.Where("id LIKE ?", rolePattern).Delete(&model.ChannelRoleModel{}).Error; err != nil {
		return err
	}

	return nil
}

// ChannelArchive 归档频道，世界管理员/拥有者可操作
func ChannelArchive(channelIDs []string, userID string, includeChildren bool) error {
	if len(channelIDs) == 0 {
		return errors.New("频道ID列表不能为空")
	}

	// 获取第一个频道的世界ID用于权限验证
	var firstChannel model.ChannelModel
	if err := model.GetDB().Where("id = ?", channelIDs[0]).Limit(1).Find(&firstChannel).Error; err != nil {
		return err
	}
	if firstChannel.ID == "" {
		return errors.New("频道不存在")
	}

	worldID := firstChannel.WorldID
	if worldID == "" {
		return errors.New("仅支持世界频道归档")
	}

	// 检查权限：世界管理员或拥有者
	if !IsWorldAdmin(worldID, userID) && !pm.CanWithSystemRole(userID, pm.PermModAdmin) {
		return errors.New("仅世界管理员可归档频道")
	}

	// 收集所有需要归档的频道ID
	targetIDs := make([]string, 0, len(channelIDs))
	for _, id := range channelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		targetIDs = append(targetIDs, id)
	}

	// 如果需要包含子频道
	if includeChildren {
		var childIDs []string
		model.GetDB().Model(&model.ChannelModel{}).
			Where("parent_id IN ? AND status = ?", targetIDs, model.ChannelStatusActive).
			Pluck("id", &childIDs)
		targetIDs = append(targetIDs, childIDs...)
	}

	if len(targetIDs) == 0 {
		return errors.New("没有可归档的频道")
	}

	// 执行归档
	err := model.GetDB().Model(&model.ChannelModel{}).
		Where("id IN ? AND status = ?", targetIDs, model.ChannelStatusActive).
		Update("status", model.ChannelStatusArchived).Error

	return err
}

// ChannelUnarchive 恢复归档频道，世界管理员/拥有者可操作
func ChannelUnarchive(channelIDs []string, userID string, includeChildren bool) error {
	if len(channelIDs) == 0 {
		return errors.New("频道ID列表不能为空")
	}

	// 获取第一个频道的世界ID用于权限验证
	var firstChannel model.ChannelModel
	if err := model.GetDB().Where("id = ?", channelIDs[0]).Limit(1).Find(&firstChannel).Error; err != nil {
		return err
	}
	if firstChannel.ID == "" {
		return errors.New("频道不存在")
	}

	worldID := firstChannel.WorldID
	if worldID == "" {
		return errors.New("仅支持世界频道恢复")
	}

	// 检查权限：世界管理员或拥有者
	if !IsWorldAdmin(worldID, userID) && !pm.CanWithSystemRole(userID, pm.PermModAdmin) {
		return errors.New("仅世界管理员可恢复频道")
	}

	// 收集所有需要恢复的频道ID
	targetIDs := make([]string, 0, len(channelIDs))
	for _, id := range channelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		targetIDs = append(targetIDs, id)
	}

	// 如果需要包含子频道
	if includeChildren {
		var childIDs []string
		model.GetDB().Model(&model.ChannelModel{}).
			Where("parent_id IN ? AND status = ?", targetIDs, model.ChannelStatusArchived).
			Pluck("id", &childIDs)
		targetIDs = append(targetIDs, childIDs...)
	}

	if len(targetIDs) == 0 {
		return errors.New("没有可恢复的频道")
	}

	// 执行恢复
	err := model.GetDB().Model(&model.ChannelModel{}).
		Where("id IN ? AND status = ?", targetIDs, model.ChannelStatusArchived).
		Update("status", model.ChannelStatusActive).Error

	return err
}

// ChannelPermanentDelete 永久删除归档频道，仅世界拥有者可操作
func ChannelPermanentDelete(channelIDs []string, userID string) error {
	if len(channelIDs) == 0 {
		return errors.New("频道ID列表不能为空")
	}

	// 获取第一个频道的世界ID用于权限验证
	var firstChannel model.ChannelModel
	if err := model.GetDB().Where("id = ?", channelIDs[0]).Limit(1).Find(&firstChannel).Error; err != nil {
		return err
	}
	if firstChannel.ID == "" {
		return errors.New("频道不存在")
	}

	worldID := firstChannel.WorldID
	if worldID == "" {
		return errors.New("仅支持世界频道删除")
	}

	// 检查权限：仅世界拥有者
	if !IsWorldOwner(worldID, userID) && !pm.CanWithSystemRole(userID, pm.PermModAdmin) {
		return errors.New("仅世界拥有者可永久删除频道")
	}

	// 验证所有频道都是已归档状态
	var channels []*model.ChannelModel
	if err := model.GetDB().Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return err
	}

	for _, ch := range channels {
		if ch.Status != model.ChannelStatusArchived {
			return fmt.Errorf("频道 %s 未归档，仅可删除已归档频道", ch.Name)
		}
		if ch.WorldID != worldID {
			return errors.New("不可跨世界删除频道")
		}
	}

	tx := model.GetDB().Begin()
	if tx.Error != nil {
		return tx.Error
	}

	var err error
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	for _, channelID := range channelIDs {
		// 删除频道及其子频道
		if err = tx.Where("id = ? OR parent_id = ?", channelID, channelID).
			Delete(&model.ChannelModel{}).Error; err != nil {
			return err
		}

		// 删除相关角色映射
		rolePattern := fmt.Sprintf("ch-%s-%%", channelID)
		if err = tx.Where("role_id LIKE ?", rolePattern).Delete(&model.UserRoleMappingModel{}).Error; err != nil {
			return err
		}
		if err = tx.Where("role_id LIKE ?", rolePattern).Delete(&model.RolePermissionModel{}).Error; err != nil {
			return err
		}
		if err = tx.Where("id LIKE ?", rolePattern).Delete(&model.ChannelRoleModel{}).Error; err != nil {
			return err
		}
	}

	return nil
}

// ArchivedChannelListResult 归档频道列表结果
type ArchivedChannelListResult struct {
	Items     []*model.ChannelModel `json:"items"`
	Total     int64                 `json:"total"`
	CanManage bool                  `json:"canManage"`
	CanDelete bool                  `json:"canDelete"`
}

// ArchivedChannelList 获取归档频道列表
func ArchivedChannelList(worldID, userID, keyword string, page, pageSize int) (*ArchivedChannelListResult, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil, errors.New("世界ID不能为空")
	}

	// 检查权限：世界成员可查看
	if !IsWorldMember(worldID, userID) && !pm.CanWithSystemRole(userID, pm.PermModAdmin) {
		return nil, errors.New("无权查看该世界的归档频道")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := model.GetDB()
	query := db.Model(&model.ChannelModel{}).
		Where("world_id = ? AND status = ? AND is_private = ?", worldID, model.ChannelStatusArchived, false)

	if keyword = strings.TrimSpace(keyword); keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var items []*model.ChannelModel
	offset := (page - 1) * pageSize
	if err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&items).Error; err != nil {
		return nil, err
	}

	// 权限判断
	canManage := IsWorldAdmin(worldID, userID) || pm.CanWithSystemRole(userID, pm.PermModAdmin)
	canDelete := IsWorldOwner(worldID, userID) || pm.CanWithSystemRole(userID, pm.PermModAdmin)

	return &ArchivedChannelListResult{
		Items:     items,
		Total:     total,
		CanManage: canManage,
		CanDelete: canDelete,
	}, nil
}
