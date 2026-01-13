package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/pm/perm_tree"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

func ChannelRoles(c *fiber.Ctx) error {
	channelID := c.Query("id")
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "缺少频道ID",
		})
	}

	return utils.APIPaginatedList(c, func(page, pageSize int) ([]*model.ChannelRoleModel, int64, error) {
		roles, total, err := model.ChannelRoleList(channelID, page, pageSize)
		return roles, total, err
	})
}

func ChannelMembers(c *fiber.Ctx) error {
	channelID := c.Query("id")
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "缺少频道ID",
		})
	}

	return utils.APIPaginatedList(c, func(page, pageSize int) ([]*model.UserRoleMappingModel, int64, error) {
		items, total, err := model.UserRoleMappingListByChannelID(channelID, page, pageSize)
		utils.QueryOneToManyMap(model.GetDB(), items, func(i *model.UserRoleMappingModel) []string {
			return []string{i.UserID}
		}, func(i *model.UserRoleMappingModel, x []*model.UserModel) {
			i.User = x[0]
		}, "")
		return items, total, err
	})
}

func ChannelMemberOptions(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "未登录",
		})
	}

	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		channelID = strings.TrimSpace(c.Query("id"))
	}
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "缺少频道ID",
		})
	}

	channelRef, err := resolveChannelAccess(user.ID, channelID)
	if err != nil {
		switch {
		case errors.Is(err, fiber.ErrForbidden):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "没有访问该频道的权限"})
		case errors.Is(err, fiber.ErrNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "校验频道权限失败"})
		}
	}

	options, err := model.ChannelIdentityOptionListForFilters(channelID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "获取频道身份失败",
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"items":   options,
		"total":   len(options),
		"channel": channelRef,
	})
}

func ChannelSpeakerOptions(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "未登录",
		})
	}

	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		channelID = strings.TrimSpace(c.Query("id"))
	}
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "缺少频道ID",
		})
	}

	channelRef, err := resolveChannelAccess(user.ID, channelID)
	if err != nil {
		switch {
		case errors.Is(err, fiber.ErrForbidden):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "没有访问该频道的权限"})
		case errors.Is(err, fiber.ErrNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "校验频道权限失败"})
		}
	}

	options, err := model.ChannelIdentityOptionListActive(channelID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "获取频道身份失败",
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"items":   options,
		"total":   len(options),
		"channel": channelRef,
	})
}

func ChannelSpeakerRoleOptions(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "未登录",
		})
	}

	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		channelID = strings.TrimSpace(c.Query("id"))
	}
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "缺少频道ID",
		})
	}

	channelRef, err := resolveChannelAccess(user.ID, channelID)
	if err != nil {
		switch {
		case errors.Is(err, fiber.ErrForbidden):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "没有访问该频道的权限"})
		case errors.Is(err, fiber.ErrNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "校验频道权限失败"})
		}
	}

	options, err := model.ChannelRoleOptionListActive(channelID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "获取频道角色失败",
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"items":   options,
		"total":   len(options),
		"channel": channelRef,
	})
}

// ChannelInfoEdit 处理频道信息编辑请求
func ChannelInfoEdit(c *fiber.Ctx) error {
	// 获取频道ID
	channelId := c.Query("id")
	if channelId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "频道ID不能为空",
		})
	}

	// TODO: 这里借一下 PermFuncChannelRoleLink 权限，以处理老频道
	if !CanWithChannelRole(c, channelId, pm.PermFuncChannelManageInfo, pm.PermFuncChannelRoleLink) {
		return nil
	}

	user := getCurUser(c)

	// 解析请求体
	var updates model.ChannelModel
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "请求参数解析失败",
		})
	}

	// 调用编辑方法
	if err := model.ChannelInfoEdit(channelId, &updates); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "频道信息更新失败",
		})
	}
	if channel, err := model.ChannelGet(channelId); err == nil {
		broadcastChannelUpdated(user, channel)
	}

	return c.JSON(fiber.Map{
		"message": "频道信息更新成功",
	})
}

func broadcastChannelUpdated(operator *model.UserModel, channel *model.ChannelModel) {
	if channel == nil || channel.ID == "" {
		return
	}
	if userId2ConnInfoGlobal == nil {
		return
	}
	event := &protocol.Event{
		Type:    protocol.EventChannelUpdated,
		Channel: channel.ToProtocolType(),
	}
	if operator != nil {
		event.User = operator.ToProtocolType()
	}
	ctx := &ChatContext{
		User:            operator,
		UserId2ConnInfo: userId2ConnInfoGlobal,
	}
	ctx.BroadcastEventInChannel(channel.ID, event)
	if operator != nil {
		ctx.BroadcastEventInChannelForBot(channel.ID, event)
	}
}

// ChannelInfoGet 处理获取频道信息请求
func ChannelInfoGet(c *fiber.Ctx) error {
	// 获取频道ID
	channelId := c.Query("id")
	if channelId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "频道ID不能为空",
		})
	}

	// 获取频道信息
	channel, err := model.ChannelGet(channelId)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "获取频道信息失败",
		})
	}

	// 检查频道是否存在
	if channel.ID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "频道不存在",
		})
	}

	return c.JSON(fiber.Map{
		"item": channel,
	})
}

type channelCopyRequest struct {
	Name     string                     `json:"name"`
	WorldID  string                     `json:"worldId"`
	ParentID string                     `json:"parentId"`
	Options  service.ChannelCopyOptions `json:"options"`
}

func ChannelCopy(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "频道ID不能为空",
		})
	}

	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "未登录",
		})
	}

	if !pm.CanWithChannelRole(user.ID, channelID, pm.PermFuncChannelManageInfo, pm.PermFuncChannelManageRoleRoot) &&
		!pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "没有复制频道的权限",
		})
	}

	var req channelCopyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "请求参数解析失败",
		})
	}

	if req.WorldID != "" && !service.IsWorldAdmin(req.WorldID, user.ID) &&
		!pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "没有目标世界的管理权限",
		})
	}

	result, err := service.ChannelClone(channelID, user, service.ChannelCopyParams{
		Name:     req.Name,
		WorldID:  req.WorldID,
		ParentID: req.ParentID,
		Options:  req.Options,
	})
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"channelId":   result.ChannelID,
		"summary":     result.Summary,
		"identityMap": result.IdentityMap,
	})
}

func ChannelDissolve(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		channelID = strings.TrimSpace(c.Query("channelId"))
	}
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "频道ID不能为空",
		})
	}

	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "未登录",
		})
	}

	if !pm.CanWithChannelRole(user.ID, channelID, pm.PermFuncChannelManageInfo, pm.PermFuncChannelManageRoleRoot) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "只有群主或管理员可以解散频道",
		})
	}

	if err := service.ChannelDissolve(channelID, user.ID); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "频道已解散",
	})
}

// ChannelPermTree 处理获取频道信息请求
func ChannelPermTree(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"items": perm_tree.PermTreeChannel,
	})
}

// ChannelRolePermGet 获取角色详细权限
func ChannelRolePermGet(c *fiber.Ctx) error {
	// 获取角色ID
	roleId := c.Query("roleId")
	if roleId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "角色ID不能为空",
		})
	}

	// 获取角色权限
	perms := pm.ChannelRolePermsGet(roleId)

	return c.JSON(fiber.Map{
		"data": perms,
	})
}

// RolePermApply 更新角色权限
func RolePermApply(c *fiber.Ctx) error {
	// 获取请求体
	var req struct {
		RoleId      string   `json:"roleId"`
		Permissions []string `json:"permissions"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "无效的请求体",
		})
	}

	chId := model.ExtractChIdFromRoleId(req.RoleId)
	if chId != "" {
		isSystemAdmin := pm.CanWithSystemRole(getCurUser(c).ID, pm.PermModAdmin)
		if !isSystemAdmin {
			if !CanWithChannelRole(c, chId, pm.PermFuncChannelManageRole, pm.PermFuncChannelManageRoleRoot) {
				return nil
			}

			// 如果没有root权限，不能操作群主的角色
			if !pm.CanWithChannelRole(getCurUser(c).ID, chId, pm.PermFuncChannelManageRoleRoot) {
				if strings.HasSuffix(req.RoleId, "-owner") {
					return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "无权限访问"})
				}
			}
		}
	} else {
		if !CanWithSystemRole(c, pm.PermModAdmin) {
			return nil
		}
	}

	// 验证参数
	if req.RoleId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "角色ID不能为空",
		})
	}

	// 更新角色权限
	pm.RolePermApply(req.RoleId, req.Permissions)

	return c.JSON(fiber.Map{
		"message": "更新成功",
	})
}

// ArchivedChannelList 获取归档频道列表
func ArchivedChannelList(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "未登录",
		})
	}

	worldID := strings.TrimSpace(c.Params("worldId"))
	if worldID == "" {
		worldID = strings.TrimSpace(c.Query("worldId"))
	}
	if worldID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "世界ID不能为空",
		})
	}

	keyword := strings.TrimSpace(c.Query("keyword"))
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("pageSize", 20)

	result, err := service.ArchivedChannelList(worldID, user.ID, keyword, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

// ChannelArchive 归档频道
func ChannelArchive(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "未登录",
		})
	}

	var req struct {
		ChannelIDs      []string `json:"channelIds"`
		IncludeChildren bool     `json:"includeChildren"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "请求参数解析失败",
		})
	}

	if len(req.ChannelIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "频道ID列表不能为空",
		})
	}

	if err := service.ChannelArchive(req.ChannelIDs, user.ID, req.IncludeChildren); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "频道已归档",
	})
}

// ChannelUnarchive 恢复归档频道
func ChannelUnarchive(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "未登录",
		})
	}

	var req struct {
		ChannelIDs      []string `json:"channelIds"`
		IncludeChildren bool     `json:"includeChildren"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "请求参数解析失败",
		})
	}

	if len(req.ChannelIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "频道ID列表不能为空",
		})
	}

	if err := service.ChannelUnarchive(req.ChannelIDs, user.ID, req.IncludeChildren); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "频道已恢复",
	})
}

// ChannelPermanentDelete 永久删除归档频道
func ChannelPermanentDelete(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "未登录",
		})
	}

	var req struct {
		ChannelIDs   []string `json:"channelIds"`
		ConfirmToken string   `json:"confirmToken"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "请求参数解析失败",
		})
	}

	if len(req.ChannelIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "频道ID列表不能为空",
		})
	}

	// 验证confirmToken（简单校验：必须等于 "CONFIRM_DELETE"）
	if req.ConfirmToken != "CONFIRM_DELETE" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "请确认删除操作",
		})
	}

	if err := service.ChannelPermanentDelete(req.ChannelIDs, user.ID); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "频道已永久删除",
	})
}
