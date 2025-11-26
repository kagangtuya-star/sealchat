package api

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

func UserRoleLink(c *fiber.Ctx) error {
	type RequestBody struct {
		RoleId  string   `json:"roleId"`
		UserIds []string `json:"userIds"`
	}

	var body RequestBody
	if err := c.BodyParser(&body); err != nil {
		// 处理解析错误
		return err
	}

	if body.RoleId == "" || len(body.UserIds) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID和角色ID不能为空",
		})
	}

	chId := model.ExtractChIdFromRoleId(body.RoleId)
	if !CanWithChannelRole(c, chId, pm.PermFuncChannelRoleLink, pm.PermFuncChannelRoleLinkRoot) {
		return nil
	}

	if strings.HasSuffix(body.RoleId, "-owner") {
		if !CanWithChannelRole(c, chId, pm.PermFuncChannelRoleLinkRoot) {
			return nil
		}
	}

	_, err := service.UserRoleLink([]string{body.RoleId}, body.UserIds)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "添加用户角色失败",
		})
	}

	return c.JSON(fiber.Map{
		"message": "用户角色已添加",
	})
}

func UserRoleUnlink(c *fiber.Ctx) error {
	type RequestBody struct {
		RoleId  string   `json:"roleId"`
		UserIds []string `json:"userIds"`
	}

	var body RequestBody
	if err := c.BodyParser(&body); err != nil {
		// 处理解析错误
		return err
	}

	if body.RoleId == "" || len(body.UserIds) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID和角色ID不能为空",
		})
	}

	chId := model.ExtractChIdFromRoleId(body.RoleId)
	if !CanWithChannelRole(c, chId, pm.PermFuncChannelRoleUnlink, pm.PermFuncChannelRoleUnlinkRoot) {
		return nil
	}

	if strings.HasSuffix(body.RoleId, "-owner") {
		if !CanWithChannelRole(c, chId, pm.PermFuncChannelRoleLinkRoot) {
			return nil
		}

		curUser := getCurUser(c)
		if curUser != nil {
			for _, uid := range body.UserIds {
				if uid == curUser.ID {
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
						"error": "群主不能移除自己的身份",
					})
				}
			}
		}
	}

	_, err := service.UserRoleUnlink([]string{body.RoleId}, body.UserIds)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "删除用户角色失败",
		})
	}

	return c.JSON(fiber.Map{
		"message": "用户角色已成功删除",
	})
}

func FriendList(c *fiber.Ctx) error {
	return utils.APIPaginatedList(c, func(page, pageSize int) ([]*model.FriendModel, int64, error) {
		items, err := model.FriendList(getCurUser(c).ID, true)
		return items, -1, err
	})
}

func BotList(c *fiber.Ctx) error {
	return utils.APIPaginatedList(c, func(page, pageSize int) ([]*model.UserModel, int64, error) {
		items, err := model.UserBotList()
		return items, -1, err
	})
}
