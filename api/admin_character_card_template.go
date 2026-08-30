package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/service"
)

type adminPlatformCharacterCardTemplateWriteRequest struct {
	Name                       *string `json:"name"`
	SheetType                  *string `json:"sheetType"`
	Content                    *string `json:"content"`
	BadgeTemplateOverride      *string `json:"badgeTemplateOverride"`
	TheaterOverlayTemplateJSON *string `json:"theaterOverlayTemplateJson"`
	Enabled                    *bool   `json:"enabled"`
}

type adminPlatformCharacterCardTemplateItem struct {
	ID                         string `json:"id"`
	Ref                        string `json:"ref"`
	Origin                     string `json:"origin"`
	Name                       string `json:"name"`
	SheetType                  string `json:"sheetType"`
	Content                    string `json:"content,omitempty"`
	BadgeTemplateOverride      string `json:"badgeTemplateOverride,omitempty"`
	TheaterOverlayTemplateJSON string `json:"theaterOverlayTemplateJson,omitempty"`
	Enabled                    bool   `json:"enabled"`
	References                 int64  `json:"references"`
	CreatedBy                  string `json:"createdBy,omitempty"`
	UpdatedBy                  string `json:"updatedBy,omitempty"`
	CreatedAt                  any    `json:"createdAt,omitempty"`
	UpdatedAt                  any    `json:"updatedAt,omitempty"`
}

func adminPlatformCharacterCardTemplateItemFromModel(item *model.PlatformCharacterCardTemplateModel, references int64) adminPlatformCharacterCardTemplateItem {
	return adminPlatformCharacterCardTemplateItem{
		ID: item.ID, Ref: service.PlatformCharacterCardTemplateRefPrefix + item.ID, Origin: "platform",
		Name: item.Name, SheetType: item.SheetType, Content: item.Content,
		BadgeTemplateOverride: item.BadgeTemplateOverride, TheaterOverlayTemplateJSON: item.TheaterOverlayTemplateJSON,
		Enabled: item.Enabled, References: references, CreatedBy: item.CreatedBy, UpdatedBy: item.UpdatedBy,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func AdminCharacterCardTemplateList(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	items, err := model.PlatformCharacterCardTemplateList("", true)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取平台人物卡模板失败")
	}
	result := make([]adminPlatformCharacterCardTemplateItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		references, countErr := service.PlatformCharacterCardTemplateReferenceCount(item.ID)
		if countErr != nil {
			return wrapErrorStatus(c, fiber.StatusInternalServerError, countErr, "读取平台人物卡模板引用数量失败")
		}
		result = append(result, adminPlatformCharacterCardTemplateItemFromModel(item, references))
	}
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("pageSize", 100)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 100
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start > len(result) {
		start = len(result)
	}
	end := start + pageSize
	if end > len(result) {
		end = len(result)
	}
	return c.JSON(fiber.Map{"items": result[start:end], "page": page, "pageSize": pageSize, "total": len(result)})
}

func AdminCharacterCardTemplateGet(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	item, err := model.PlatformCharacterCardTemplateGetByID(strings.TrimSpace(c.Params("id")))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return wrapErrorStatus(c, fiber.StatusNotFound, err, "模板不存在")
	}
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取平台人物卡模板失败")
	}
	references, err := service.PlatformCharacterCardTemplateReferenceCount(item.ID)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取平台人物卡模板引用数量失败")
	}
	return c.JSON(fiber.Map{"item": adminPlatformCharacterCardTemplateItemFromModel(item, references)})
}

func AdminCharacterCardTemplateCreate(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	var payload adminPlatformCharacterCardTemplateWriteRequest
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	user := getCurUser(c)
	if payload.Enabled == nil {
		enabled := true
		payload.Enabled = &enabled
	}
	if payload.Name == nil || payload.Content == nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "名称、模板内容不能为空")
	}
	item, err := service.PlatformCharacterCardTemplateCreate(&service.PlatformCharacterCardTemplateInput{
		Name: *payload.Name, SheetType: valueOrEmpty(payload.SheetType), Content: *payload.Content,
		BadgeTemplateOverride: valueOrEmpty(payload.BadgeTemplateOverride), TheaterOverlayTemplateJSON: valueOrEmpty(payload.TheaterOverlayTemplateJSON),
		Enabled: *payload.Enabled, CreatedBy: user.ID, UpdatedBy: user.ID,
	})
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"item": adminPlatformCharacterCardTemplateItemFromModel(item, 0)})
}

func AdminCharacterCardTemplateUpdate(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	var payload adminPlatformCharacterCardTemplateWriteRequest
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	item, err := service.PlatformCharacterCardTemplateUpdate(strings.TrimSpace(c.Params("id")), &service.PlatformCharacterCardTemplateUpdateInput{
		Name: payload.Name, SheetType: payload.SheetType, Content: payload.Content, BadgeTemplateOverride: payload.BadgeTemplateOverride,
		TheaterOverlayTemplateJSON: payload.TheaterOverlayTemplateJSON, Enabled: payload.Enabled, UpdatedBy: getCurUser(c).ID,
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return wrapErrorStatus(c, fiber.StatusNotFound, err, "模板不存在")
	}
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	references, countErr := service.PlatformCharacterCardTemplateReferenceCount(item.ID)
	if countErr != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, countErr, "读取平台人物卡模板引用数量失败")
	}
	return c.JSON(fiber.Map{"item": adminPlatformCharacterCardTemplateItemFromModel(item, references)})
}

func AdminCharacterCardTemplateSetEnabled(c *fiber.Ctx, enabled bool) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	item, err := service.PlatformCharacterCardTemplateSetEnabled(strings.TrimSpace(c.Params("id")), enabled, getCurUser(c).ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return wrapErrorStatus(c, fiber.StatusNotFound, err, "模板不存在")
	}
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	references, countErr := service.PlatformCharacterCardTemplateReferenceCount(item.ID)
	if countErr != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, countErr, "读取平台人物卡模板引用数量失败")
	}
	return c.JSON(fiber.Map{"item": adminPlatformCharacterCardTemplateItemFromModel(item, references)})
}

func AdminCharacterCardTemplateEnable(c *fiber.Ctx) error {
	return AdminCharacterCardTemplateSetEnabled(c, true)
}
func AdminCharacterCardTemplateDisable(c *fiber.Ctx) error {
	return AdminCharacterCardTemplateSetEnabled(c, false)
}

func AdminCharacterCardTemplateDelete(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	err := service.PlatformCharacterCardTemplateDelete(strings.TrimSpace(c.Params("id")))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return wrapErrorStatus(c, fiber.StatusNotFound, err, "模板不存在")
	}
	if err != nil {
		if strings.Contains(err.Error(), "仍被") {
			return wrapErrorStatus(c, fiber.StatusConflict, err, err.Error())
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "删除平台人物卡模板失败")
	}
	return c.JSON(fiber.Map{"success": true})
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
