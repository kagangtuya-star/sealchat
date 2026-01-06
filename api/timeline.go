package api

import (
	"github.com/gofiber/fiber/v2"

	"sealchat/model"
)

func TimelineList(c *fiber.Ctx) error {
	// page := c.QueryInt("page", 1)
	// pageSize := c.QueryInt("pageSize", 20)
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "unauthorized",
		})
	}
	db := model.GetDB()

	var total int64
	db.Model(&model.TimelineModel{}).
		Where("receiver_id = ?", user.ID).
		Count(&total)

	// 获取列表
	var items []model.TimelineModel
	// offset := (page - 1) * pageSize
	db.Order("created_at desc").
		Where("receiver_id = ?", user.ID).
		// Offset(offset).Limit(pageSize).
		// Preload("User", func(db *gorm.DB) *gorm.DB {
		//	return db.Select("id, username")
		// }).
		Find(&items)

	// 返回JSON响应
	return c.JSON(fiber.Map{
		// "page":     page,
		// "pageSize": pageSize,
		"total": total,
		"items": items,
	})
}

func TimelineMarkRead(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "unauthorized",
		})
	}
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return err
	}
	query := model.GetDB().Model(&model.TimelineModel{}).
		Where("receiver_id = ?", user.ID)
	if len(payload.IDs) > 0 {
		query = query.Where("id IN ?", payload.IDs)
	}
	if err := query.Update("is_read", true).Error; err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"message": "ok",
	})
}
