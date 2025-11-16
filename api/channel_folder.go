package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func ChannelFolderList(c *fiber.Ctx) error {
	user := getCurUser(c)
	result, err := service.ChannelFolderList(user.ID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

func ChannelFolderCreate(c *fiber.Ctx) error {
	user := getCurUser(c)
	payload := &service.ChannelFolderInput{}
	if err := c.BodyParser(payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	folder, err := service.ChannelFolderCreate(user.ID, payload)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": folder})
}

func ChannelFolderUpdate(c *fiber.Ctx) error {
	user := getCurUser(c)
	folderID := c.Params("id")
	if folderID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的文件夹ID"})
	}
	payload := &service.ChannelFolderInput{}
	if err := c.BodyParser(payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	folder, err := service.ChannelFolderUpdate(user.ID, folderID, payload)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"item": folder})
}

func ChannelFolderDelete(c *fiber.Ctx) error {
	user := getCurUser(c)
	folderID := c.Params("id")
	if folderID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的文件夹ID"})
	}
	if err := service.ChannelFolderDelete(user.ID, folderID); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

func ChannelFolderAssign(c *fiber.Ctx) error {
	user := getCurUser(c)
	payload := &service.ChannelFolderAssignPayload{}
	if err := c.BodyParser(payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if err := service.ChannelFolderAssign(user.ID, payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

func ChannelFolderToggleFavorite(c *fiber.Ctx) error {
	user := getCurUser(c)
	folderID := c.Params("id")
	if folderID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的文件夹ID"})
	}
	var body struct {
		Favorite bool `json:"favorite"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	favorites, err := service.ChannelFolderToggleFavorite(user.ID, folderID, body.Favorite)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"favorites": favorites})
}
