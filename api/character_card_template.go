package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

type characterCardTemplatePayload struct {
	Name            string `json:"name"`
	SheetType       string `json:"sheetType"`
	Content         string `json:"content"`
	IsGlobalDefault *bool  `json:"isGlobalDefault"`
	IsSheetDefault  *bool  `json:"isSheetDefault"`
}

type characterCardTemplateSetDefaultPayload struct {
	Scope string `json:"scope"`
}

type characterCardTemplateBindingPayload struct {
	ChannelID        string `json:"channelId"`
	ExternalCardID   string `json:"externalCardId"`
	CardName         string `json:"cardName"`
	SheetType        string `json:"sheetType"`
	Mode             string `json:"mode"`
	TemplateID       string `json:"templateId"`
	TemplateSnapshot string `json:"templateSnapshot"`
}

func mapCharacterCardTemplateError(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "不存在"):
		return http.StatusNotFound, msg
	case strings.Contains(msg, "无权"), strings.Contains(msg, "成员"):
		return http.StatusForbidden, msg
	case strings.Contains(msg, "不能为空"), strings.Contains(msg, "无效"), strings.Contains(msg, "缺少"), strings.Contains(msg, "长度"):
		return http.StatusBadRequest, msg
	default:
		return http.StatusInternalServerError, "操作失败"
	}
}

func CharacterCardTemplateList(c *fiber.Ctx) error {
	user := getCurUser(c)
	sheetType := c.Query("sheetType")
	worldID := strings.TrimSpace(c.Query("worldId"))
	var (
		items any
		err error
	)
	if worldID != "" {
		items, err = service.CharacterCardTemplateListWithWorld(user.ID, worldID, sheetType)
	} else {
		items, err = service.CharacterCardTemplateList(user.ID, sheetType)
	}
	if err != nil {
		status, msg := mapCharacterCardTemplateError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"items": items})
}

func CharacterCardTemplateCreate(c *fiber.Ctx) error {
	payload := characterCardTemplatePayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	user := getCurUser(c)
	isGlobalDefault := payload.IsGlobalDefault != nil && *payload.IsGlobalDefault
	isSheetDefault := payload.IsSheetDefault != nil && *payload.IsSheetDefault
	item, err := service.CharacterCardTemplateCreate(user.ID, &service.CharacterCardTemplateInput{
		Name:            payload.Name,
		SheetType:       payload.SheetType,
		Content:         payload.Content,
		IsGlobalDefault: isGlobalDefault,
		IsSheetDefault:  isSheetDefault,
	})
	if err != nil {
		status, msg := mapCharacterCardTemplateError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item})
}

func CharacterCardTemplateUpdate(c *fiber.Ctx) error {
	templateID := strings.TrimSpace(c.Params("id"))
	if templateID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的模板ID"})
	}
	payload := characterCardTemplatePayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	input := &service.CharacterCardTemplateUpdateInput{}
	if payload.Name != "" {
		input.Name = &payload.Name
	}
	if payload.SheetType != "" {
		input.SheetType = &payload.SheetType
	}
	if payload.Content != "" {
		input.Content = &payload.Content
	}
	if payload.IsGlobalDefault != nil {
		input.IsGlobalDefault = payload.IsGlobalDefault
	}
	if payload.IsSheetDefault != nil {
		input.IsSheetDefault = payload.IsSheetDefault
	}
	user := getCurUser(c)
	item, err := service.CharacterCardTemplateUpdate(user.ID, templateID, input)
	if err != nil {
		status, msg := mapCharacterCardTemplateError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"item": item})
}

func CharacterCardTemplateDelete(c *fiber.Ctx) error {
	templateID := strings.TrimSpace(c.Params("id"))
	if templateID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的模板ID"})
	}
	user := getCurUser(c)
	if err := service.CharacterCardTemplateDelete(user.ID, templateID); err != nil {
		status, msg := mapCharacterCardTemplateError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"success": true})
}

func CharacterCardTemplateSetDefault(c *fiber.Ctx) error {
	templateID := strings.TrimSpace(c.Params("id"))
	if templateID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "无效的模板ID"})
	}
	payload := characterCardTemplateSetDefaultPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	user := getCurUser(c)
	item, err := service.CharacterCardTemplateSetDefault(user.ID, templateID, payload.Scope)
	if err != nil {
		status, msg := mapCharacterCardTemplateError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"item": item})
}

func CharacterCardTemplateBindingList(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Query("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	user := getCurUser(c)
	items, err := service.CharacterCardTemplateBindingList(user.ID, channelID)
	if err != nil {
		status, msg := mapCharacterCardTemplateError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"items": items})
}

func CharacterCardTemplateBindingUpsert(c *fiber.Ctx) error {
	payload := characterCardTemplateBindingPayload{}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	user := getCurUser(c)
	item, err := service.CharacterCardTemplateBindingUpsert(user.ID, &service.CharacterCardTemplateBindingInput{
		ChannelID:        payload.ChannelID,
		ExternalCardID:   payload.ExternalCardID,
		CardName:         payload.CardName,
		SheetType:        payload.SheetType,
		Mode:             payload.Mode,
		TemplateID:       payload.TemplateID,
		TemplateSnapshot: payload.TemplateSnapshot,
	})
	if err != nil {
		status, msg := mapCharacterCardTemplateError(err)
		return c.Status(status).JSON(fiber.Map{"error": msg})
	}
	return c.JSON(fiber.Map{"item": item})
}
