package api

import (
	"errors"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"sealchat/service"
	"sealchat/utils"
)

// Fiber keeps encoded route parameters escaped by default. UI customization
// IDs may intentionally contain a stable reference such as `builtin:rlyeh-archive`,
// so decode path parameters at the API boundary before handing them to the
// service layer. Malformed escapes are left untouched and will resolve as a
// normal not-found ID rather than changing the route contract.
func uiCustomizationParam(c *fiber.Ctx, key string) string {
	value := c.Params(key)
	if decoded, err := url.PathUnescape(value); err == nil {
		return decoded
	}
	return value
}

func bindUICustomizationRoutes(group fiber.Router) {
	for _, prefix := range []string{"/admin/ui-customizations", "/worlds/:worldId/ui-customizations"} {
		group.Get(prefix, uiCustomizationList)
		group.Post(prefix, uiCustomizationCreate)
		group.Post(prefix+"/reorder", uiCustomizationReorder)
		group.Post(prefix+"/import", uiCustomizationImport)
		group.Patch(prefix+"/:id", uiCustomizationUpdate)
		group.Delete(prefix+"/:id", uiCustomizationDelete)
	}
	group.Put("/worlds/:worldId/ui-customizations/platform/:id", uiCustomizationPlatformRefEnable)
	group.Delete("/worlds/:worldId/ui-customizations/platform/:id", uiCustomizationPlatformRefDisable)
	group.Post("/admin/ui-customizations/:id/import", uiCustomizationPlatformReplaceImport)
}

func uiCustomizationScope(c *fiber.Ctx) service.UICustomizationScope {
	if !strings.Contains(c.Route().Path, ":worldId") {
		return service.UICustomizationScope{Type: "platform"}
	}
	return service.UICustomizationScope{Type: "world", ID: strings.TrimSpace(uiCustomizationParam(c, "worldId"))}
}

func uiCustomizationPolicy() *utils.UICustomizationPolicyConfig {
	var policy *utils.UICustomizationPolicyConfig
	if appConfig != nil {
		policy = appConfig.UICustomization
	}
	return utils.NormalizeUICustomizationPolicy(policy)
}

func uiCustomizationError(c *fiber.Ctx, err error) error {
	status, message := fiber.StatusInternalServerError, "界面自定义操作失败"
	switch {
	case errors.Is(err, service.ErrUICustomizationInvalid):
		// Validation details are user-facing Chinese text; drop the sentinel prefix.
		status, message = fiber.StatusBadRequest, strings.ReplaceAll(err.Error(), service.ErrUICustomizationInvalid.Error()+": ", "")
	case errors.Is(err, service.ErrUICustomizationBuiltin):
		status, message = fiber.StatusBadRequest, "内置界面主题只能启用或停用，不能直接编辑或删除"
	case errors.Is(err, service.ErrUICustomizationLimit):
		status, message = fiber.StatusConflict, "同一作用域最多 50 个包"
	case errors.Is(err, service.ErrUICustomizationDenied):
		status, message = fiber.StatusForbidden, "无权访问界面自定义包"
	case errors.Is(err, service.ErrUICustomizationNotFound), errors.Is(err, service.ErrWorldNotFound), errors.Is(err, service.ErrWorldObserverLinkInvalid):
		status, message = fiber.StatusNotFound, "界面自定义上下文或包不存在"
	default:
		var logErr *service.LogError
		if errors.As(err, &logErr) {
			switch logErr.Code {
			case "NOT_FOUND":
				status = fiber.StatusNotFound
			case "UNAUTHENTICATED":
				status = fiber.StatusUnauthorized
			case "PERMISSION_DENIED":
				status = fiber.StatusForbidden
			}
		}
	}
	return c.Status(status).JSON(fiber.Map{"message": message})
}

func uiCustomizationList(c *fiber.Ctx) error {
	scope := uiCustomizationScope(c)
	actorID := getCurUser(c).ID
	if err := service.EnsureBuiltinUICustomizations(scope, actorID); err != nil {
		return uiCustomizationError(c, err)
	}
	items, err := service.ListUICustomizations(scope, actorID)
	if err != nil {
		return uiCustomizationError(c, err)
	}
	policy := uiCustomizationPolicy()
	return c.JSON(fiber.Map{"packages": items, "policy": fiber.Map{
		"allowWorldCss": policy.AllowWorldCSS, "allowWorldJs": policy.AllowWorldJS, "allowPublicWorldCss": policy.AllowPublicWorldCSS,
		"maxCodeSizeKB": policy.MaxCodeSizeKB,
	}})
}

func uiCustomizationCreate(c *fiber.Ctx) error { return uiCustomizationSave(c, true) }
func uiCustomizationUpdate(c *fiber.Ctx) error { return uiCustomizationSave(c, false) }
func uiCustomizationSave(c *fiber.Ctx, create bool) error {
	var input service.UICustomizationPatch
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求参数解析失败"})
	}
	var item *service.UICustomizationPackage
	var err error
	if create {
		item, err = service.CreateUICustomization(uiCustomizationScope(c), getCurUser(c).ID, input, uiCustomizationPolicy())
	} else {
		item, err = service.UpdateUICustomization(uiCustomizationScope(c), getCurUser(c).ID, uiCustomizationParam(c, "id"), input, uiCustomizationPolicy())
	}
	if err != nil {
		return uiCustomizationError(c, err)
	}
	if create {
		c.Status(fiber.StatusCreated)
	}
	return c.JSON(item)
}

func uiCustomizationDelete(c *fiber.Ctx) error {
	if err := service.DeleteUICustomization(uiCustomizationScope(c), getCurUser(c).ID, uiCustomizationParam(c, "id")); err != nil {
		return uiCustomizationError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func uiCustomizationPlatformRefEnable(c *fiber.Ctx) error {
	if err := service.SetWorldPlatformUICustomizationReference(
		uiCustomizationParam(c, "worldId"), getCurUser(c).ID, uiCustomizationParam(c, "id"), true,
	); err != nil {
		return uiCustomizationError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func uiCustomizationPlatformRefDisable(c *fiber.Ctx) error {
	if err := service.SetWorldPlatformUICustomizationReference(
		uiCustomizationParam(c, "worldId"), getCurUser(c).ID, uiCustomizationParam(c, "id"), false,
	); err != nil {
		return uiCustomizationError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func uiCustomizationPlatformReplaceImport(c *fiber.Ctx) error {
	item, err := service.ReplacePlatformUICustomizationFromImport(
		getCurUser(c).ID, uiCustomizationParam(c, "id"), c.Body(), uiCustomizationPolicy(),
	)
	if err != nil {
		return uiCustomizationError(c, err)
	}
	return c.JSON(item)
}

func uiCustomizationImport(c *fiber.Ctx) error {
	items, err := service.ImportUICustomizations(uiCustomizationScope(c), getCurUser(c).ID, c.Body(), uiCustomizationPolicy())
	if err != nil {
		return uiCustomizationError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"packages": items})
}

func uiCustomizationReorder(c *fiber.Ctx) error {
	var input struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求参数解析失败"})
	}
	if err := service.ReorderUICustomizations(uiCustomizationScope(c), getCurUser(c).ID, input.IDs); err != nil {
		return uiCustomizationError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func uiCustomizationRuntime(c *fiber.Ctx) error {
	actorID := ""
	if user := getCurUser(c); user != nil {
		actorID = user.ID
	}
	result, err := service.ResolveUICustomizations(service.UICustomizationContext{
		Surface: service.UISurface(c.Query("surface")), WorldID: c.Query("worldId"), ChannelID: c.Query("channelId"), Slug: c.Query("slug"),
	}, actorID, uiCustomizationPolicy())
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	if err != nil {
		return uiCustomizationError(c, err)
	}
	return c.JSON(result)
}
