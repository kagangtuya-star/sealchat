package api

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

type battleReportRequest struct {
	Title              string   `json:"title"`
	Content            string   `json:"content"`
	PeriodStart        int64    `json:"periodStart"`
	PeriodEnd          int64    `json:"periodEnd"`
	ContextReportCount int      `json:"contextReportCount"`
	Source             string   `json:"source"`
	SourceChannelIDs   []string `json:"sourceChannelIds"`
	AIProviderID       string   `json:"aiProviderId"`
	AIModel            string   `json:"aiModel"`
	AIFeatureKey       string   `json:"aiFeatureKey"`
}

type battleReportReorderRequest struct {
	IDs []string `json:"ids"`
}

type battleReportDisplayRequest struct {
	DisplayName string `json:"displayName"`
	Enabled     *bool  `json:"enabled"`
}

type battleReportResponse struct {
	ID                 string `json:"id"`
	ChannelID          string `json:"channelId"`
	WorldID            string `json:"worldId"`
	Title              string `json:"title"`
	Content            string `json:"content,omitempty"`
	ContentPreview     string `json:"contentPreview"`
	PeriodStart        int64  `json:"periodStart"`
	PeriodEnd          int64  `json:"periodEnd"`
	ContextReportCount int    `json:"contextReportCount"`
	SortOrder          int    `json:"sortOrder"`
	Status             string `json:"status"`
	ErrorMessage       string `json:"errorMessage,omitempty"`
	CreatorID          string `json:"creatorId"`
	UpdaterID          string `json:"updaterId"`
	AISource           string `json:"aiSource,omitempty"`
	AIProviderID       string `json:"aiProviderId,omitempty"`
	AIModel            string `json:"aiModel,omitempty"`
	AIFeatureKey       string `json:"aiFeatureKey,omitempty"`
	CreatedAt          int64  `json:"createdAt"`
	UpdatedAt          int64  `json:"updatedAt"`
}

type battleReportDisplayResponse struct {
	ID               string `json:"id"`
	WorldID          string `json:"worldId"`
	SourceChannelID  string `json:"sourceChannelId"`
	DisplayChannelID string `json:"displayChannelId"`
	DisplayName      string `json:"displayName"`
	Enabled          bool   `json:"enabled"`
	CreatedAt        int64  `json:"createdAt"`
	UpdatedAt        int64  `json:"updatedAt"`
}

func BattleReportList(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	items, err := service.ListBattleReports(c.Params("channelId"), user.ID)
	if err != nil {
		return battleReportError(c, err)
	}
	out := make([]battleReportResponse, 0, len(items))
	for _, item := range items {
		out = append(out, battleReportToResponse(item, false))
	}
	return c.JSON(fiber.Map{"items": out})
}

func BattleReportCreate(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var req battleReportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求体解析失败"})
	}
	item, err := service.CreateBattleReport(c.Params("channelId"), user.ID, battleReportInputFromRequest(req))
	if err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"item": battleReportToResponse(item, true)})
}

func BattleReportGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	item, err := service.GetBattleReport(c.Params("reportId"), user.ID)
	if err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"item": battleReportToResponse(item, true)})
}

func BattleReportUpdate(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var req battleReportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求体解析失败"})
	}
	item, err := service.UpdateBattleReport(c.Params("reportId"), user.ID, battleReportInputFromRequest(req))
	if err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"item": battleReportToResponse(item, true)})
}

func BattleReportSummarizeInput(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var req battleReportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求体解析失败"})
	}
	cfg := utils.AIConfig{}
	if appConfig != nil {
		cfg = appConfig.AI
	}
	prompt, err := service.BuildBattleReportSummaryPrompt(c.Params("channelId"), user.ID, service.BattleReportSummaryPromptInput{
		Title:              req.Title,
		PeriodStart:        unixMilliToTime(req.PeriodStart),
		PeriodEnd:          unixMilliToTime(req.PeriodEnd),
		ContextReportCount: req.ContextReportCount,
		SourceChannelIDs:   req.SourceChannelIDs,
		AIConfig:           cfg,
	})
	if err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{
		"input": prompt,
	})
}

func BattleReportDelete(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	if err := service.DeleteBattleReport(c.Params("reportId"), user.ID); err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"message": "战报已删除"})
}

func BattleReportReorder(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var req battleReportReorderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求体解析失败"})
	}
	if err := service.ReorderBattleReports(c.Params("channelId"), user.ID, req.IDs); err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"message": "战报顺序已更新"})
}

func BattleReportDisplayGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	item, err := service.GetBattleReportDisplayChannel(c.Params("channelId"), user.ID)
	if err != nil {
		return battleReportError(c, err)
	}
	if item == nil {
		return c.JSON(fiber.Map{"item": nil})
	}
	return c.JSON(fiber.Map{"item": battleReportDisplayToResponse(item)})
}

func BattleReportDisplayEnsure(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var req battleReportDisplayRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求体解析失败"})
	}
	item, err := service.EnsureBattleReportDisplayChannel(c.Params("channelId"), user.ID, req.DisplayName)
	if err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"item": battleReportDisplayToResponse(item)})
}

func BattleReportDisplayUpdate(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var req battleReportDisplayRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求体解析失败"})
	}
	item, err := service.EnsureBattleReportDisplayChannel(c.Params("channelId"), user.ID, req.DisplayName)
	if err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"item": battleReportDisplayToResponse(item)})
}

func BattleReportDisplayDelete(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	if err := service.DisableBattleReportDisplayChannel(c.Params("channelId"), user.ID); err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"message": "战报展示频道已关闭"})
}

func BattleReportDisplayResync(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	channelID := c.Params("channelId")
	item, err := service.GetBattleReportDisplayChannel(channelID, user.ID)
	if err != nil {
		return battleReportError(c, err)
	}
	if item == nil {
		return c.JSON(fiber.Map{"message": "战报展示频道未开启"})
	}
	if err := service.SyncBattleReportDisplayFromReports(channelID); err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"message": "战报展示频道已同步"})
}

func BattleReportSummarize(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var req battleReportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求体解析失败"})
	}
	cfg := utils.AIConfig{}
	if appConfig != nil {
		cfg = appConfig.AI
	}
	runner := aiRunnerFactory(func() *utils.AppConfig { return appConfig })
	item, err := service.StartBattleReportSummary(c.Context(), c.Params("channelId"), user.ID, service.BattleReportSummaryInput{
		Title:              req.Title,
		PeriodStart:        unixMilliToTime(req.PeriodStart),
		PeriodEnd:          unixMilliToTime(req.PeriodEnd),
		ContextReportCount: req.ContextReportCount,
		SourceChannelIDs:   req.SourceChannelIDs,
		Source:             req.Source,
		AIConfig:           cfg,
		Runner:             runner,
	})
	if err != nil {
		return battleReportError(c, err)
	}
	return c.JSON(fiber.Map{"item": battleReportToResponse(item, true)})
}

func battleReportDisplayToResponse(item *model.BattleReportDisplayChannelModel) battleReportDisplayResponse {
	if item == nil {
		return battleReportDisplayResponse{}
	}
	return battleReportDisplayResponse{
		ID:               item.ID,
		WorldID:          item.WorldID,
		SourceChannelID:  item.SourceChannelID,
		DisplayChannelID: item.DisplayChannelID,
		DisplayName:      item.DisplayName,
		Enabled:          item.Enabled,
		CreatedAt:        timeToUnixMilli(item.CreatedAt),
		UpdatedAt:        timeToUnixMilli(item.UpdatedAt),
	}
}

func battleReportInputFromRequest(req battleReportRequest) service.BattleReportInput {
	return service.BattleReportInput{
		Title:              req.Title,
		Content:            req.Content,
		PeriodStart:        unixMilliToTime(req.PeriodStart),
		PeriodEnd:          unixMilliToTime(req.PeriodEnd),
		ContextReportCount: req.ContextReportCount,
		SourceChannelIDs:   req.SourceChannelIDs,
		AISource:           req.Source,
		AIProviderID:       req.AIProviderID,
		AIModel:            req.AIModel,
		AIFeatureKey:       req.AIFeatureKey,
	}
}

func battleReportToResponse(item *model.BattleReportModel, includeContent bool) battleReportResponse {
	if item == nil {
		return battleReportResponse{}
	}
	resp := battleReportResponse{
		ID:                 item.ID,
		ChannelID:          item.ChannelID,
		WorldID:            item.WorldID,
		Title:              item.Title,
		ContentPreview:     item.ContentPreview,
		PeriodStart:        timeToUnixMilli(item.PeriodStart),
		PeriodEnd:          timeToUnixMilli(item.PeriodEnd),
		ContextReportCount: item.ContextReportCount,
		SortOrder:          item.SortOrder,
		Status:             string(item.Status),
		ErrorMessage:       item.ErrorMessage,
		CreatorID:          item.CreatorID,
		UpdaterID:          item.UpdaterID,
		AISource:           item.AISource,
		AIProviderID:       item.AIProviderID,
		AIModel:            item.AIModel,
		AIFeatureKey:       item.AIFeatureKey,
		CreatedAt:          timeToUnixMilli(item.CreatedAt),
		UpdatedAt:          timeToUnixMilli(item.UpdatedAt),
	}
	if includeContent {
		resp.Content = item.Content
	}
	return resp
}

func battleReportError(c *fiber.Ctx, err error) error {
	status := fiber.StatusBadRequest
	message := "战报操作失败"
	switch {
	case err == nil:
		return c.SendStatus(fiber.StatusInternalServerError)
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = fiber.StatusNotFound
		message = "战报不存在"
	case strings.Contains(err.Error(), "仅频道成员"), strings.Contains(err.Error(), "仅世界成员"):
		status = fiber.StatusForbidden
		message = err.Error()
	}
	return c.Status(status).JSON(fiber.Map{"message": message, "error": err.Error()})
}

func unixMilliToTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value)
}

func timeToUnixMilli(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}
