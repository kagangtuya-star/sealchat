package api

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

func bindChannelWorkbenchRoutes(group fiber.Router) {
	group.Get("/channels/:channelId/workbench", channelWorkbenchGetHandler)
	group.Put("/channels/:channelId/workbench/layout", channelWorkbenchLayoutSaveHandler)
	group.Put("/channels/:channelId/workbench/mode", channelWorkbenchModeHandler)
	group.Post("/channels/:channelId/workbench/presentation", channelWorkbenchPresentationHandler)
}

func channelWorkbenchError(c *fiber.Ctx, err error) error {
	var validation *service.ChannelWorkbenchValidationError
	switch {
	case errors.As(err, &validation):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": validation.Reason})
	case errors.Is(err, service.ErrChannelWorkbenchUnsupportedVersion):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "布局版本高于当前服务端支持的版本"})
	case errors.Is(err, service.ErrChannelWorkbenchInvalid):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "布局参数无效"})
	case errors.Is(err, service.ErrChannelWorkbenchDenied):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权访问或管理该布局频道"})
	case errors.Is(err, service.ErrChannelWorkbenchNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "频道不存在或不支持布局"})
	case errors.Is(err, service.ErrChannelWorkbenchConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "布局已被其他管理员更新"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "布局频道操作失败"})
	}
}

func channelWorkbenchRequest(c *fiber.Ctx) (*model.UserModel, string, bool) {
	user := getCurUser(c)
	if user == nil {
		_ = c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
		return nil, "", false
	}
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		_ = c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
		return nil, "", false
	}
	return user, channelID, true
}

func broadcastChannelWorkbenchUpdated(operator *model.UserModel, channelID string, layoutRevision, presentationRevision uint64, reason string) {
	if userId2ConnInfoGlobal == nil || channelID == "" {
		return
	}
	payload := &protocol.ChannelWorkbenchEventPayload{
		ChannelID:            channelID,
		LayoutRevision:       layoutRevision,
		PresentationRevision: presentationRevision,
		Reason:               reason,
	}
	event := &protocol.Event{
		Type:             protocol.EventChannelWorkbenchUpdated,
		Channel:          &protocol.Channel{ID: channelID},
		ChannelWorkbench: payload,
	}
	if operator != nil {
		payload.OperatorID = operator.ID
		event.User = operator.ToProtocolType()
	}
	ctx := &ChatContext{
		User:            operator,
		ChannelUsersMap: channelUsersMapGlobal,
		UserId2ConnInfo: userId2ConnInfoGlobal,
	}
	ctx.BroadcastEventInChannel(channelID, event)
}

func channelWorkbenchGetHandler(c *fiber.Ctx) error {
	user, channelID, ok := channelWorkbenchRequest(c)
	if !ok {
		return nil
	}
	view, err := service.ChannelWorkbenchGetView(channelID, user.ID)
	if err != nil {
		return channelWorkbenchError(c, err)
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(view)
}

func channelWorkbenchLayoutSaveHandler(c *fiber.Ctx) error {
	user, channelID, ok := channelWorkbenchRequest(c)
	if !ok {
		return nil
	}
	var body struct {
		Layout       json.RawMessage `json:"layout"`
		BaseRevision *uint64         `json:"baseRevision"`
	}
	if err := json.Unmarshal(c.Body(), &body); err != nil || body.BaseRevision == nil || len(body.Layout) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	_, revision, err := service.ChannelWorkbenchSaveLayout(channelID, user.ID, body.Layout, *body.BaseRevision)
	if err != nil {
		if errors.Is(err, service.ErrChannelWorkbenchConflict) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"message":        "布局已被其他管理员更新",
				"layoutRevision": service.ChannelWorkbenchCurrentLayoutRevision(channelID),
			})
		}
		return channelWorkbenchError(c, err)
	}
	broadcastChannelWorkbenchUpdated(user, channelID, revision, 0, "layout")
	view, err := service.ChannelWorkbenchGetView(channelID, user.ID)
	if err != nil {
		return channelWorkbenchError(c, err)
	}
	return c.JSON(view)
}

func channelWorkbenchModeHandler(c *fiber.Ctx) error {
	user, channelID, ok := channelWorkbenchRequest(c)
	if !ok {
		return nil
	}
	var body struct {
		DisplayMode *string `json:"displayMode"`
	}
	if err := json.Unmarshal(c.Body(), &body); err != nil || body.DisplayMode == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	previous, err := model.ChannelGet(channelID)
	if err != nil {
		return channelWorkbenchError(c, err)
	}
	channel, err := service.ChannelWorkbenchSetDisplayMode(channelID, user.ID, *body.DisplayMode)
	if err != nil {
		return channelWorkbenchError(c, err)
	}
	if previous == nil || previous.DisplayMode != channel.DisplayMode {
		// 展示形态写在频道上，频道树刷新后所有在线世界成员即可得到新形态。
		broadcastChannelTreeInvalidated(user, channel, "display-mode")
	}
	return c.JSON(fiber.Map{"channelId": channel.ID, "displayMode": channel.DisplayMode})
}

func channelWorkbenchPresentationHandler(c *fiber.Ctx) error {
	user, channelID, ok := channelWorkbenchRequest(c)
	if !ok {
		return nil
	}
	var input service.ChannelWorkbenchPresentationInput
	if err := json.Unmarshal(c.Body(), &input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	_, revision, err := service.ChannelWorkbenchApplyPresentation(channelID, user.ID, input)
	if err != nil {
		return channelWorkbenchError(c, err)
	}
	broadcastChannelWorkbenchUpdated(user, channelID, 0, revision, "presentation")
	return c.JSON(fiber.Map{"presentationRevision": revision})
}
