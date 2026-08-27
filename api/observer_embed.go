package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

func resolveObserverEmbedChannel(c *fiber.Ctx) (*model.WorldModel, *model.ChannelModel, int, string) {
	slug := strings.TrimSpace(c.Query("ob_slug"))
	if slug == "" {
		slug = strings.TrimSpace(c.Get("X-Observer-Slug"))
	}
	if slug == "" {
		return nil, nil, fiber.StatusBadRequest, "缺少OB链接标识"
	}
	world, _, err := service.ResolveWorldObserverLink(slug)
	if err != nil || world == nil || strings.TrimSpace(world.ID) == "" {
		if err == nil || errors.Is(err, service.ErrWorldObserverLinkInvalid) {
			return nil, nil, fiber.StatusNotFound, "旁观链接无效或已关闭"
		}
		return nil, nil, fiber.StatusInternalServerError, "解析旁观链接失败"
	}
	channel, err := service.CanObserverAccessChannel(c.Params("channelId"), world.ID)
	if err != nil {
		return nil, nil, fiber.StatusForbidden, "没有访问该频道的权限"
	}
	return world, channel, 0, ""
}

func ObserverStickyNoteList(c *fiber.Ctx) error {
	_, channel, status, message := resolveObserverEmbedChannel(c)
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": message})
	}
	notes, err := model.StickyNoteListByChannel(channel.ID, false)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]fiber.Map, 0, len(notes))
	for _, note := range notes {
		if note == nil {
			continue
		}
		note.LoadCreator()
		items = append(items, fiber.Map{"note": note.ToProtocolType()})
	}
	return c.JSON(fiber.Map{"items": items})
}

func ObserverStickyNoteFolderList(c *fiber.Ctx) error {
	_, channel, status, message := resolveObserverEmbedChannel(c)
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": message})
	}
	folders, err := model.StickyNoteFolderListByChannel(channel.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]*protocol.StickyNoteFolder, 0, len(folders))
	for _, folder := range folders {
		if folder != nil {
			items = append(items, folder.ToProtocolType())
		}
	}
	return c.JSON(fiber.Map{"folders": items})
}

func ObserverChannelSpeakerOptions(c *fiber.Ctx) error {
	_, channel, status, message := resolveObserverEmbedChannel(c)
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": message})
	}
	options, err := model.ChannelIdentityOptionListActive(channel.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "获取频道角色失败"})
	}
	return c.JSON(fiber.Map{
		"items": options,
		"total": len(options),
		"channel": fiber.Map{
			"id":   channel.ID,
			"name": channel.Name,
		},
	})
}

func ObserverChannelImagesList(c *fiber.Ctx) error {
	_, channel, status, message := resolveObserverEmbedChannel(c)
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": message})
	}

	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := c.QueryInt("page_size", 50)
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}
	icModeFilter := strings.ToLower(strings.TrimSpace(c.Query("ic_mode", "all")))
	sortOrder := strings.ToLower(strings.TrimSpace(c.Query("sort", "desc")))
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	db := model.GetDB()
	if err := model.BackfillMessageImageAttachmentsForChannel(db, channel.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "初始化频道图片索引失败"})
	}
	resp, err := queryChannelImages(db, "", channel.ID, icModeFilter, sortOrder, page, pageSize, false)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "查询失败"})
	}
	return c.JSON(resp)
}

func ObserverBattleReportList(c *fiber.Ctx) error {
	world, _, status, message := resolveObserverEmbedChannel(c)
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": message})
	}
	items, err := service.ListBattleReportsForObserver(c.Params("channelId"), world.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "没有访问该频道的权限"})
	}
	out := make([]battleReportResponse, 0, len(items))
	for _, item := range items {
		out = append(out, battleReportToResponse(item, false))
	}
	return c.JSON(fiber.Map{"items": out})
}

func ObserverBattleReportGet(c *fiber.Ctx) error {
	world, _, status, message := resolveObserverEmbedChannel(c)
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": message})
	}
	item, err := service.GetBattleReportForObserver(c.Params("reportId"), world.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "战报不存在"})
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "没有访问该战报的权限"})
	}
	return c.JSON(fiber.Map{"item": battleReportToResponse(item, true)})
}

func ObserverBattleReportJumpTarget(c *fiber.Ctx) error {
	world, _, status, message := resolveObserverEmbedChannel(c)
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": message})
	}
	target, err := service.GetBattleReportJumpTargetForObserver(c.Params("reportId"), world.ID, c.Query("edge"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "战报不存在"})
		}
		if strings.Contains(err.Error(), "无效的战报跳转位置") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "没有访问该战报的权限"})
	}
	if target == nil {
		return c.JSON(fiber.Map{"target": nil, "reason": "no_message"})
	}
	return c.JSON(fiber.Map{"target": battleReportJumpTargetToResponse(target)})
}

func ObserverChannelIFormList(c *fiber.Ctx) error {
	_, channel, status, message := resolveObserverEmbedChannel(c)
	if status != 0 {
		return c.Status(status).JSON(fiber.Map{"message": message})
	}
	forms, err := service.ListEffectiveChannelIForms(channel.ID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "获取嵌入窗失败")
	}
	for _, form := range forms {
		if form != nil {
			form.Readonly = true
		}
	}
	return c.JSON(fiber.Map{"items": convertIFormViewListToProtocol(forms), "total": len(forms)})
}
