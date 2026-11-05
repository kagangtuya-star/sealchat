package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"sealchat/pm"
	"sealchat/service"
)

func bindLogProjectRoutes(auth fiber.Router) {
	bindLogStyleRoutes(auth)
	bindLogReleaseRoutes(auth)
	auth.Get("/message-mutation-jobs/:jobId", logAuthentication, logMutationJobGet)
	auth.Get("/message-mutation-jobs/:jobId/items", logAuthentication, logMutationJobGet)
	auth.Get("/worlds/:worldId/log-project", logAuthentication, logProjectGet)
	auth.Put("/worlds/:worldId/log-project", logAuthentication, logProjectPut)
	auth.Get("/log/layout/:slug", logAuthentication, logLayoutGet)
	auth.Put("/log/projects/:id/draft", logAuthentication, logDraftPut)
	auth.Post("/log/projects/:id/export", logAuthentication, logExportCreate)
	auth.Get("/log/pdf-capability", logAuthentication, logPDFCapabilityGet)
	auth.Get("/log/projects/:id/sections/:sectionId/messages", logAuthentication, logMessagesGet)
	auth.Get("/log/projects/:id/sections/:sectionId/range-check", logAuthentication, logSourceRangeCheckGet)
	auth.Get("/log/projects/:id/sections/:sectionId/messages/:messageId/assets/:assetKey", logAuthentication, logBodyAssetGet)
	auth.Get("/log/projects/:id/assets/:assetId", logAuthentication, logProjectAssetGet)
	auth.Get("/log/projects/:id/sections/:sectionId/message-context/:messageId", logAuthentication, logMessagesGet)
	auth.Put("/log/projects/:id/sections/:sectionId/exclusions", logAuthentication, logExclusionsPut)
}

func logBodyAssetGet(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Referrer-Policy", "no-referrer")
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	asset, err := service.GetLogBodyAsset(c.Context(), c.Params("id"), c.Params("sectionId"), c.Params("messageId"), c.Params("assetKey"), u.ID)
	if err != nil {
		return logResponse(c, nil, err)
	}
	c.Set("Content-Type", asset.MIME)
	return c.Send(asset.Data)
}

// logProjectAssetGet serves images registered in the LOG draft to its designers only.
func logProjectAssetGet(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Referrer-Policy", "no-referrer")
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	asset, err := service.GetLogProjectAsset(c.Context(), c.Params("id"), c.Params("assetId"), u.ID)
	if err != nil {
		return logResponse(c, nil, err)
	}
	c.Set("Content-Type", asset.MIME)
	return c.Send(asset.Data)
}

func logAuthentication(c *fiber.Ctx) error {
	err := SignCheckMiddleware(c)
	if c.Response().StatusCode() == fiber.StatusUnauthorized {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	return err
}

func logResponse(c *fiber.Ctx, data any, err error) error {
	c.Set("Cache-Control", "no-store")
	if err == nil {
		return c.JSON(fiber.Map{"data": data})
	}
	var domain *service.LogError
	if !errors.As(err, &domain) {
		domain = &service.LogError{Code: "INTERNAL_ERROR", Message: "LOG 操作失败"}
	}
	status := map[string]int{"INVALID_INPUT": 400, "INVALID_PATCH": 400, "INVALID_IDENTITY": 400, "PREVIEW_EXPIRED": 409, "UNAUTHENTICATED": 401, "PERMISSION_DENIED": 403, "NOT_FOUND": 404, "VERSION_CONFLICT": 409, "SOURCE_CHANGED": 409, "SLUG_TAKEN": 409, "TOO_LARGE": 413, "INVALID_TEMPLATE": 422, "RATE_LIMITED": 429, "PDF_UNAVAILABLE": 503}[domain.Code]
	if status == 0 {
		status = 500
	}
	return c.Status(status).JSON(fiber.Map{"error": domain})
}

func logDecodeBody(c *fiber.Ctx, target any) error {
	if len(c.Body()) > 2<<20 {
		return &service.LogError{Code: "TOO_LARGE", Message: "请求超过 2MB"}
	}
	d := json.NewDecoder(bytes.NewReader(c.Body()))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return &service.LogError{Code: "INVALID_INPUT", Message: "请求格式无效"}
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return &service.LogError{Code: "INVALID_INPUT", Message: "请求格式无效"}
	}
	return nil
}

func logProjectGet(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	data, err := service.GetLogProject(c.Params("worldId"), u.ID)
	return logResponse(c, data, err)
}
func logProjectPut(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	var input service.LogProjectUpdate
	if err := logDecodeBody(c, &input); err != nil {
		return logResponse(c, nil, err)
	}
	data, err := service.UpdateLogProject(c.Params("worldId"), u.ID, input)
	return logResponse(c, data, err)
}
func logLayoutGet(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	data, err := service.ResolveLogLayout(c.Params("slug"), u.ID)
	return logResponse(c, data, err)
}

func logDraftPut(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	var input service.LogDraftUpdate
	if err := logDecodeBody(c, &input); err != nil {
		return logResponse(c, nil, err)
	}
	data, err := service.SaveLogDraft(c.Params("id"), u.ID, input)
	return logResponse(c, data, err)
}
func logExclusionsPut(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	var input service.LogExclusionUpdate
	if err := logDecodeBody(c, &input); err != nil {
		return logResponse(c, nil, err)
	}
	version, err := service.UpdateLogExclusions(c.Params("id"), c.Params("sectionId"), u.ID, input)
	return logResponse(c, fiber.Map{"draftVersion": version}, err)
}
func logMessagesGet(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	limit, err := strconv.Atoi(c.Query("limit", "100"))
	if err != nil {
		return logResponse(c, nil, &service.LogError{Code: "INVALID_INPUT", Message: "limit 无效"})
	}
	window, err := service.QueryLogMessages(c.Params("id"), c.Params("sectionId"), u.ID, c.Query("cursor"), c.Params("messageId"), limit)
	if err != nil {
		return logResponse(c, nil, err)
	}
	items := make([]fiber.Map, 0, len(window.Messages))
	channel := window.Channel
	channelData := channel.ToProtocolType()
	rank := getChannelMemberRoleRank(channel, channel.ID, u.ID)
	systemAdmin := pm.CanWithSystemRole(u.ID, pm.PermModAdmin)
	reorderAll := canReorderAllMessages(u.ID, channel)
	authors := []string{}
	seenAuthors := map[string]bool{}
	for _, m := range window.Messages {
		if !seenAuthors[m.UserID] {
			authors = append(authors, m.UserID)
			seenAuthors[m.UserID] = true
		}
	}
	targetRanks, err := service.ChannelMemberRoleRanks(channel, authors)
	if err != nil {
		return logResponse(c, nil, err)
	}
	for _, m := range window.Messages {
		own := m.UserID == u.ID
		targetRank := targetRanks[m.UserID]
		moderate := rank >= channelMemberRoleRankAdmin && rank > targetRank
		edit := !m.IsRevoked && !m.IsDeleted && (own || (window.World.AllowAdminEditMessages && moderate))
		messageData := service.LogDesignMessageData(m, channelData)
		item := fiber.Map{"message": messageData, "version": m.Version, "capabilities": fiber.Map{
			"edit": edit, "changeIdentity": edit, "reorder": !m.IsDeleted && (own || reorderAll), "revoke": !m.IsDeleted && !m.IsRevoked && own, "remove": !m.IsDeleted && (own || systemAdmin || moderate), "editAppearance": edit,
		}}
		if p := window.Presentations[m.ID]; p != nil {
			item["presentation"] = p
		}
		items = append(items, item)
	}
	data := fiber.Map{"items": items, "sourceVersion": window.SourceVersion}
	if c.Params("messageId") == "" {
		data["nextCursor"] = window.NextCursor
	}
	return logResponse(c, data, nil)
}

func logSourceRangeCheckGet(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return logResponse(c, nil, &service.LogError{Code: "UNAUTHENTICATED", Message: "请先登录"})
	}
	version, err := strconv.ParseInt(c.Query("expectedVersion"), 10, 64)
	if err != nil {
		return logResponse(c, nil, &service.LogError{Code: "INVALID_INPUT", Message: "版本无效"})
	}
	data, err := service.CheckLogSourceRange(c.Params("id"), c.Params("sectionId"), u.ID, version)
	return logResponse(c, data, err)
}
