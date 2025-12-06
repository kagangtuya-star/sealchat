package api

import (
	"net/http"
	"sealchat/service"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
)

func AdminUserList(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return nil
	}

	// 获取分页参数
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "20"))
	keyword := c.Query("keyword", "")
	userType := c.Query("type", "") // "bot", "user", "" (all)

	// 参数校验
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	db := model.GetDB()
	var total int64
	query := db.Model(&model.UserModel{})

	// 搜索过滤
	if keyword != "" {
		query = query.Where("username LIKE ? OR nickname LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%")
	}

	// 用户类型过滤
	if userType == "bot" {
		query = query.Where("is_bot = ?", true)
	} else if userType == "user" {
		query = query.Where("is_bot = ?", false)
	}

	query.Count(&total)

	// 获取列表
	var items []*model.UserModel
	offset := (page - 1) * pageSize
	query.Order("created_at desc").
		Offset(offset).Limit(pageSize).
		Find(&items)

	for _, i := range items {
		i.RoleIds, _ = model.UserRoleMappingListByUserID(i.ID, "", "system")
	}

	// 返回JSON响应
	return c.JSON(fiber.Map{
		"page":     page,
		"pageSize": pageSize,
		"total":    total,
		"items":    items,
	})
}

func AdminUserDisable(c *fiber.Ctx) error {
	userId := c.Query("id")
	if userId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID不能为空",
		})
	}

	err := model.UserSetDisable(userId, true)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "禁用用户失败",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "用户已成功禁用",
	})
}

func AdminUserEnable(c *fiber.Ctx) error {
	userId := c.Query("id")
	if userId == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID不能为空",
		})
	}

	err := model.UserSetDisable(userId, false)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "启用用户失败",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "用户已成功启用",
	})
}

func AdminUserResetPassword(c *fiber.Ctx) error {
	uid := c.Query("id")
	if uid == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "参数错误",
		})
	}

	err := model.UserUpdatePassword(uid, "123456")
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "重置密码失败",
		})
	}

	return c.JSON(fiber.Map{
		"message": "禁用成功",
	})
}

func AdminUserRoleLinkByUserId(c *fiber.Ctx) error {
	type RequestBody struct {
		UserId  string   `json:"userId"`
		RoleIds []string `json:"roleIds"`
	}

	var body RequestBody
	if err := c.BodyParser(&body); err != nil {
		// 处理解析错误
		return err
	}

	if body.UserId == "" || len(body.RoleIds) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID和角色ID不能为空",
		})
	}

	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return nil
	}

	_, err := service.UserRoleLink(body.RoleIds, []string{body.UserId})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "添加用户角色失败",
		})
	}

	return c.JSON(fiber.Map{
		"message": "用户角色已添加",
	})
}

func AdminUserRoleUnlinkByUserId(c *fiber.Ctx) error {
	type RequestBody struct {
		UserId  string   `json:"userId"`
		RoleIds []string `json:"roleIds"`
	}

	var body RequestBody
	if err := c.BodyParser(&body); err != nil {
		// 处理解析错误
		return err
	}

	if body.UserId == "" || len(body.RoleIds) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "用户ID和角色ID不能为空",
		})
	}

	if !CanWithSystemRole(c, pm.PermFuncAdminUserEdit) {
		return nil
	}

	_, err := service.UserRoleUnlink(body.RoleIds, []string{body.UserId})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "删除用户角色失败",
		})
	}

	return c.JSON(fiber.Map{
		"message": "用户角色已成功删除",
	})
}
