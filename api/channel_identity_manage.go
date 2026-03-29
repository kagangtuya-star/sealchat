package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func resolveChannelIdentityActorFromRequest(c *fiber.Ctx, channelID string, requestedTargetUserID string) (*service.ChannelIdentityActorContext, error) {
	user := getCurUser(c)
	if user == nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "未登录")
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "缺少频道ID")
	}
	ctx, err := service.ResolveChannelIdentityActor(channelID, user.ID, requestedTargetUserID)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func handleChannelIdentityActorErr(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return c.Status(fiberErr.Code).JSON(fiber.Map{"error": fiberErr.Message})
	}
	switch {
	case errors.Is(err, service.ErrChannelNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "频道不存在"})
	case errors.Is(err, service.ErrChannelWorldRequired),
		errors.Is(err, service.ErrChannelIdentityTargetNotInChannel):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "目标用户不在当前频道中"})
	case errors.Is(err, service.ErrChannelIdentityDelegationDisabled):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "当前世界未开启代管其他用户频道角色"})
	case errors.Is(err, service.ErrChannelIdentityDelegationForbidden),
		errors.Is(err, service.ErrChannelPermissionDenied):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "无权管理该用户的频道角色"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
}

func ChannelIdentityManageCandidates(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	channelID := strings.TrimSpace(c.Params("channelId"))
	result, err := service.ListChannelIdentityManageCandidates(service.ChannelIdentityManageCandidateQuery{
		ChannelID: channelID,
		ActorID:   user.ID,
		Page:      parseQueryIntDefault(c, "page", 1),
		PageSize:  parseQueryIntDefault(c, "pageSize", 20),
		Keyword:   strings.TrimSpace(c.Query("keyword")),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrChannelNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
		case errors.Is(err, service.ErrChannelPermissionDenied):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权查看代管候选用户"})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "获取代管候选用户失败"})
		}
	}
	return c.JSON(result)
}
