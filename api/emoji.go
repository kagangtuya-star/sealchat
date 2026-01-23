package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/service"
)

func UserEmojiAdd(c *fiber.Ctx) error {
	ui := getCurUser(c)

	var body struct {
		AttachmentId string `json:"attachmentId"`
		Remark       string `json:"remark"`
	}
	if err := c.BodyParser(&body); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求参数错误")
	}
	if strings.TrimSpace(body.AttachmentId) == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "附件ID不能为空")
	}

	remark := strings.TrimSpace(body.Remark)
	if remark != "" && !service.GalleryValidateRemark(remark) {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, service.ErrGalleryRemarkInvalid.Error())
	}

	item, err := service.GalleryAddEmojiFavorite(ui.ID, body.AttachmentId, remark)
	if err != nil {
		return wrapError(c, err, "收藏表情失败")
	}
	return c.JSON(fiber.Map{
		"item": item,
	})
}

func UserReactionEmojiAdd(c *fiber.Ctx) error {
	ui := getCurUser(c)

	var body struct {
		AttachmentId string `json:"attachmentId"`
		Remark       string `json:"remark"`
	}
	if err := c.BodyParser(&body); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求参数错误")
	}
	if strings.TrimSpace(body.AttachmentId) == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "附件ID不能为空")
	}

	remark := strings.TrimSpace(body.Remark)
	if remark != "" && !service.GalleryValidateRemark(remark) {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, service.ErrGalleryRemarkInvalid.Error())
	}

	item, err := service.GalleryAddEmojiReaction(ui.ID, body.AttachmentId, remark)
	if err != nil {
		return wrapError(c, err, "添加表情反应失败")
	}
	return c.JSON(fiber.Map{
		"item": item,
	})
}

func UserEmojiDelete(c *fiber.Ctx) error {
	ui := getCurUser(c)
	var reqBody struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&reqBody); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "无效的请求参数")
	}
	ids := reqBody.IDs
	if len(ids) == 0 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "ID列表不能为空")
	}
	count, err := service.GalleryDeleteEmojiFavorites(ui.ID, ids)
	if err != nil {
		return wrapError(c, err, "删除表情失败")
	}
	return c.JSON(fiber.Map{
		"message": "表情删除成功",
		"count":   count,
	})
}

func UserEmojiList(c *fiber.Ctx) error {
	ui := getCurUser(c)

	items, err := service.GalleryListEmojiFavorites(ui.ID)
	if err != nil {
		return wrapError(c, err, "获取表情列表失败")
	}
	return c.JSON(fiber.Map{
		"items":    items,
		"total":    len(items),
		"page":     1,
		"pageSize": len(items),
	})
}

func UserEmojiUpdate(c *fiber.Ctx) error {
	emojiID := c.Params("id")
	if emojiID == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "表情ID不能为空")
	}
	var payload struct {
		Remark string `json:"remark"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求参数错误")
	}

	ui := getCurUser(c)

	remark := strings.TrimSpace(payload.Remark)
	if remark != "" && !service.GalleryValidateRemark(remark) {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, service.ErrGalleryRemarkInvalid.Error())
	}

	item, err := service.GalleryUpdateEmojiFavoriteRemark(ui.ID, emojiID, remark)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "表情不存在")
		}
		return wrapError(c, err, "更新表情备注失败")
	}
	return c.JSON(fiber.Map{"item": item})
}
