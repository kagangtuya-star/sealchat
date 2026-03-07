package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func ObserverPrintPageHandler(c *fiber.Ctx) error {
	slug := strings.TrimSpace(c.Params("slug"))
	if slug == "" {
		return sendObserverPrintError(c, fiber.StatusBadRequest, "缺少 OB 标识", "请检查打印链接是否完整。")
	}

	messageScope := normalizeObserverPrintMessageScope(c.Query("message_scope", "0"))
	showArchived := normalizeObserverPrintShowArchived(c.Query("show_archived", "1"))
	showTimestamp := normalizeObserverPrintShowTimestamp(c.Query("show_timestamp", "1"))
	showColorCode := normalizeObserverPrintShowColorCode(c.Query("show_color_code", "0"))

	world, defaultChannelID, err := service.ResolveWorldObserverLink(slug)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorldObserverLinkInvalid):
			return sendObserverPrintError(c, fiber.StatusNotFound, "打印链接不可用", "OB 旁观链接无效、已关闭或目标世界不可访问。")
		default:
			return sendObserverPrintError(c, http.StatusInternalServerError, "打印页加载失败", "解析 OB 旁观链接时发生错误，请稍后重试。")
		}
	}

	channelID := strings.TrimSpace(c.Query("channel_id"))
	if channelID == "" {
		channelID = strings.TrimSpace(defaultChannelID)
	}
	if channelID == "" {
		return sendObserverPrintError(c, fiber.StatusNotFound, "未找到可用频道", "该 OB 链接未配置可公开访问的默认频道。")
	}

	channel, err := service.CanObserverAccessChannel(channelID, world.ID)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			return sendObserverPrintError(c, fiber.StatusNotFound, "频道不存在", "指定频道不存在或不在当前 OB 世界中。")
		}
		return sendObserverPrintError(c, fiber.StatusForbidden, "频道不可访问", "指定频道不允许通过当前 OB 链接访问。")
	}

	messages, err := service.LoadObserverPrintableMessages(channel.ID, service.ObserverPrintOptions{
		MessageScope:  messageScope,
		ShowArchived:  showArchived,
		ShowTimestamp: showTimestamp,
		ShowColorCode: showColorCode,
	})
	if err != nil {
		return sendObserverPrintError(c, http.StatusInternalServerError, "消息加载失败", "读取频道消息时发生错误，请稍后重试。")
	}

	pageData := service.BuildObserverPrintPageData(world, channel, slug, messages, service.ObserverPrintOptions{
		MessageScope:  messageScope,
		ShowArchived:  showArchived,
		ShowTimestamp: showTimestamp,
		ShowColorCode: showColorCode,
	})
	body, err := service.RenderObserverPrintHTML(pageData)
	if err != nil {
		return sendObserverPrintError(c, http.StatusInternalServerError, "页面渲染失败", "生成打印页内容时发生错误，请稍后重试。")
	}

	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Status(http.StatusOK).Send(body)
}

func normalizeObserverPrintMessageScope(raw string) int {
	switch strings.TrimSpace(raw) {
	case "1":
		return 1
	case "2":
		return 2
	default:
		return 0
	}
}

func normalizeObserverPrintShowArchived(raw string) bool {
	return strings.TrimSpace(raw) != "0"
}

func normalizeObserverPrintShowTimestamp(raw string) bool {
	return strings.TrimSpace(raw) != "0"
}

func normalizeObserverPrintShowColorCode(raw string) bool {
	return strings.TrimSpace(raw) == "1"
}

func sendObserverPrintError(c *fiber.Ctx, status int, title, message string) error {
	body, err := service.RenderObserverPrintErrorHTML(title, message)
	if err != nil {
		return c.Status(status).SendString(message)
	}
	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Status(status).Send(body)
}
