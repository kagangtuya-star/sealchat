package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func ChannelIdentityFolderList(c *fiber.Ctx) error {
	channelID := c.Query("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	user := getCurUser(c)
	result, err := service.ChannelIdentityListByUser(channelID, user.ID)
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
	ChannelID string `json:"channelId"`
	Name      string `json:"name"`
	SortOrder *int   `json:"sortOrder"`
}

func ChannelIdentityFolderCreate(c *fiber.Ctx) error {
	payload := channelIdentityFolderPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	user := getCurUser(c)
	folder, err := service.ChannelIdentityFolderCreate(user.ID, &service.ChannelIdentityFolderInput{
		ChannelID: payload.ChannelID,
		Name:      payload.Name,
		SortOrder: payload.SortOrder,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
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
	user := getCurUser(c)
	folder, err := service.ChannelIdentityFolderUpdate(user.ID, payload.ChannelID, folderID, &service.ChannelIdentityFolderInput{
		ChannelID: payload.ChannelID,
		Name:      payload.Name,
		SortOrder: payload.SortOrder,
	})
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"item": folder})
}

func ChannelIdentityFolderDelete(c *fiber.Ctx) error {
	folderID := c.Params("id")
	if folderID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的文件夹ID"})
	}
	channelID := c.Query("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	user := getCurUser(c)
	if err := service.ChannelIdentityFolderDelete(user.ID, channelID, folderID); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

type channelIdentityFolderFavoritePayload struct {
	ChannelID string `json:"channelId"`
	Favorite  bool   `json:"favorite"`
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
	user := getCurUser(c)
	favorites, err := service.ChannelIdentityFolderToggleFavorite(user.ID, payload.ChannelID, folderID, payload.Favorite)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"favorites": favorites})
}

type channelIdentityFolderAssignPayload struct {
	ChannelID   string   `json:"channelId"`
	IdentityIDs []string `json:"identityIds"`
	FolderIDs   []string `json:"folderIds"`
	Mode        string   `json:"mode"`
}

func ChannelIdentityFolderAssign(c *fiber.Ctx) error {
	payload := channelIdentityFolderAssignPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if payload.ChannelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	user := getCurUser(c)
	membership, err := service.ChannelIdentityFolderAssign(user.ID, payload.ChannelID, payload.IdentityIDs, payload.FolderIDs, payload.Mode)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"membership": membership})
}
