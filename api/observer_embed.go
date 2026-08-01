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
