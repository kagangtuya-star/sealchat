package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
)

// MentionableMemberItem 可 @ 成员项
type MentionableMemberItem struct {
	UserID       string `json:"userId"`
	DisplayName  string `json:"displayName"`
	Color        string `json:"color"`
	Avatar       string `json:"avatar"`
	IdentityID   string `json:"identityId,omitempty"`
	IdentityType string `json:"identityType"` // "ic" | "ooc" | "user"
}

// ChannelMentionableMembers 获取可 @ 的成员列表
// GET /api/v1/channels/:channelId/mentionable-members?icMode=ic|ooc
func ChannelMentionableMembers(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "未登录",
		})
	}

	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "bad_request",
			"message": "缺少频道ID",
		})
	}

	// 私聊频道不支持 @
	if len(channelID) > 30 {
		return c.JSON(fiber.Map{
			"items": []MentionableMemberItem{},
			"total": 0,
		})
	}

	// 校验频道访问权限
	_, err := resolveChannelAccess(user.ID, channelID)
	if err != nil {
		switch {
		case errors.Is(err, fiber.ErrForbidden):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":   "forbidden",
				"message": "没有访问该频道的权限",
			})
		case errors.Is(err, fiber.ErrNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "not_found",
				"message": "频道不存在",
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "internal_error",
				"message": "校验频道权限失败",
			})
		}
	}

	icMode := strings.ToLower(strings.TrimSpace(c.Query("icMode")))
	if icMode != "" && icMode != "ic" && icMode != "ooc" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "bad_request",
			"message": "invalid icMode, must be 'ic' or 'ooc'",
		})
	}

	// 获取当前在线用户集合
	onlineUserIDs := getOnlineUserIDsInChannel(channelID)

	// 获取频道内所有身份卡
	identities, err := model.ChannelIdentityListAll(channelID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "internal_error",
			"message": "获取频道身份失败",
		})
	}

	// 按用户分组身份卡
	userIdentities := make(map[string][]*model.ChannelIdentityModel)
	for _, identity := range identities {
		if identity.UserID == user.ID {
			continue // 排除自己
		}
		userIdentities[identity.UserID] = append(userIdentities[identity.UserID], identity)
	}

	// 获取各用户的 IC/OOC 配置
	// 暂时简化处理：第一个身份视为 IC，后续身份视为 OOC
	items := make([]MentionableMemberItem, 0)

	for userID, userIdents := range userIdentities {
		// 只包含在线用户
		if !onlineUserIDs[userID] {
			continue
		}

		for idx, identity := range userIdents {
			identityType := "ic"
			if idx > 0 {
				identityType = "ooc"
			}

			// 根据 icMode 过滤
			if icMode != "" && icMode != identityType {
				continue
			}

			items = append(items, MentionableMemberItem{
				UserID:       userID,
				DisplayName:  identity.DisplayName,
				Color:        identity.Color,
				Avatar:       identity.AvatarAttachmentID,
				IdentityID:   identity.ID,
				IdentityType: identityType,
			})
		}
	}

	// 添加没有身份卡但在线的成员（使用用户名）
	for userID := range onlineUserIDs {
		if userID == user.ID {
			continue // 排除自己
		}
		if _, hasIdentity := userIdentities[userID]; hasIdentity {
			continue // 已有身份卡的跳过
		}

		// 获取用户信息
		userInfo := model.UserGet(userID)
		if userInfo == nil {
			continue
		}

		items = append(items, MentionableMemberItem{
			UserID:       userID,
			DisplayName:  getUserDisplayName(userInfo),
			Color:        userInfo.NickColor,
			Avatar:       userInfo.Avatar,
			IdentityType: "user",
		})
	}

	// 检查是否可以 @all（管理员权限）
	canAtAll := pm.CanWithChannelRole(user.ID, channelID, pm.PermFuncChannelManageInfo)

	return c.JSON(fiber.Map{
		"items":    items,
		"total":    len(items),
		"canAtAll": canAtAll,
	})
}

// getOnlineUserIDsInChannel 获取频道内在线用户 ID 集合
func getOnlineUserIDsInChannel(channelID string) map[string]bool {
	result := make(map[string]bool)

	channelUsersMap := getChannelUsersMap()
	if channelUsersMap == nil {
		return result
	}

	userSet, ok := channelUsersMap.Load(channelID)
	if !ok || userSet == nil {
		return result
	}

	userSet.Range(func(userID string) bool {
		result[userID] = true
		return true
	})

	return result
}

// getUserDisplayName 获取用户显示名
func getUserDisplayName(user *model.UserModel) string {
	if user.Nickname != "" {
		return user.Nickname
	}
	if user.Username != "" {
		return user.Username
	}
	return "用户"
}
