package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func ChannelIdentityFolderList(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Query("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, channelID, strings.TrimSpace(c.Query("targetUserId")))
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	result, err := service.ChannelIdentityListByUser(channelID, ctx.TargetUserID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"folders":    result.Folders,
		"favorites":  result.Favorites,
		"membership": result.Membership,
	})
}

type channelIdentityFolderPayload struct {
	ChannelID    string `json:"channelId"`
	TargetUserID string `json:"targetUserId"`
	Name         string `json:"name"`
	SortOrder    *int   `json:"sortOrder"`
}

func ChannelIdentityFolderCreate(c *fiber.Ctx) error {
	payload := channelIdentityFolderPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	folder, err := service.ChannelIdentityFolderCreateWithAccess(ctx.TargetUserID, ctx.OperatorUserID, &service.ChannelIdentityFolderInput{
		ChannelID: payload.ChannelID,
		Name:      payload.Name,
		SortOrder: payload.SortOrder,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
		ChannelID:      payload.ChannelID,
		TargetUserID:   ctx.TargetUserID,
		OperatorUserID: ctx.OperatorUserID,
		Reason:         "identity-folder-create",
	})
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": folder})
}

func ChannelIdentityFolderUpdate(c *fiber.Ctx) error {
	folderID := c.Params("id")
	if folderID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的文件夹ID"})
	}
	payload := channelIdentityFolderPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	folder, err := service.ChannelIdentityFolderUpdateWithAccess(ctx.TargetUserID, ctx.OperatorUserID, payload.ChannelID, folderID, &service.ChannelIdentityFolderInput{
		ChannelID: payload.ChannelID,
		Name:      payload.Name,
		SortOrder: payload.SortOrder,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
		ChannelID:      payload.ChannelID,
		TargetUserID:   ctx.TargetUserID,
		OperatorUserID: ctx.OperatorUserID,
		Reason:         "identity-folder-update",
	})
	return c.JSON(fiber.Map{"item": folder})
}

func ChannelIdentityFolderDelete(c *fiber.Ctx) error {
	folderID := c.Params("id")
	if folderID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的文件夹ID"})
	}
	channelID := strings.TrimSpace(c.Query("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, channelID, strings.TrimSpace(c.Query("targetUserId")))
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	if err := service.ChannelIdentityFolderDeleteWithAccess(ctx.TargetUserID, ctx.OperatorUserID, channelID, folderID); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
		ChannelID:      channelID,
		TargetUserID:   ctx.TargetUserID,
		OperatorUserID: ctx.OperatorUserID,
		Reason:         "identity-folder-delete",
	})
	return c.JSON(fiber.Map{"success": true})
}

type channelIdentityFolderFavoritePayload struct {
	ChannelID    string `json:"channelId"`
	TargetUserID string `json:"targetUserId"`
	Favorite     bool   `json:"favorite"`
}

func ChannelIdentityFolderToggleFavorite(c *fiber.Ctx) error {
	folderID := c.Params("id")
	if folderID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的文件夹ID"})
	}
	payload := channelIdentityFolderFavoritePayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	favorites, err := service.ChannelIdentityFolderToggleFavoriteWithAccess(ctx.TargetUserID, ctx.OperatorUserID, payload.ChannelID, folderID, payload.Favorite)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
		ChannelID:      payload.ChannelID,
		TargetUserID:   ctx.TargetUserID,
		OperatorUserID: ctx.OperatorUserID,
		Reason:         "identity-folder-favorite",
	})
	return c.JSON(fiber.Map{"favorites": favorites})
}

type channelIdentityFolderAssignPayload struct {
	ChannelID    string   `json:"channelId"`
	TargetUserID string   `json:"targetUserId"`
	IdentityIDs  []string `json:"identityIds"`
	FolderIDs    []string `json:"folderIds"`
	Mode         string   `json:"mode"`
}

func ChannelIdentityFolderAssign(c *fiber.Ctx) error {
	payload := channelIdentityFolderAssignPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	ctx, err := resolveChannelIdentityActorFromRequest(c, payload.ChannelID, payload.TargetUserID)
	if err != nil {
		return handleChannelIdentityActorErr(c, err)
	}
	membership, err := service.ChannelIdentityFolderAssignWithAccess(ctx.TargetUserID, ctx.OperatorUserID, payload.ChannelID, payload.IdentityIDs, payload.FolderIDs, payload.Mode)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	broadcastChannelIdentityRefresh(channelIdentityRefreshPayload{
		ChannelID:      payload.ChannelID,
		TargetUserID:   ctx.TargetUserID,
		OperatorUserID: ctx.OperatorUserID,
		Reason:         "identity-folder-assign",
	})
	return c.JSON(fiber.Map{"membership": membership})
}
