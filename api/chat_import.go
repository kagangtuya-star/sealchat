package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
)

// ChatImportTemplates 获取内置正则模板列表
func ChatImportTemplates(c *fiber.Ctx) error {
	templates := service.GetChatImportTemplates()
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"templates": templates,
	})
}

// ChatImportPreview 预览解析结果
func ChatImportPreview(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未授权"})
	}

	channelID := c.Params("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}

	// 验证权限
	channel, err := model.ChannelGet(channelID)
	if err != nil || channel == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "频道不存在"})
	}

	if channel.WorldID != "" && !service.IsWorldAdmin(channel.WorldID, u.ID) {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "仅世界管理员可导入聊天记录"})
	}

	var req model.ChatImportPreviewRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求格式错误"})
	}

	if req.Content == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "日志内容不能为空"})
	}

	// 默认合并不匹配行
	if !req.MergeUnmatched {
		req.MergeUnmatched = true
	}

	result, err := service.ParsePreview(&req)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "解析失败: " + err.Error()})
	}

	return c.Status(http.StatusOK).JSON(result)
}

// ChatImportExecute 执行导入
func ChatImportExecute(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未授权"})
	}

	channelID := c.Params("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}

	var req model.ChatImportExecuteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求格式错误"})
	}

	if req.Content == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "日志内容不能为空"})
	}

	if req.Config == nil {
		req.Config = &model.ChatImportConfig{
			MergeUnmatched: true,
		}
	}

	job, err := service.ChatImportExecute(channelID, u.ID, &req)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"jobId":   job.ID,
		"status":  job.Status,
		"message": "导入任务已创建",
	})
}

// ChatImportJobStatus 获取导入任务状态
func ChatImportJobStatus(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未授权"})
	}

	jobID := c.Params("jobId")
	if jobID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少任务ID"})
	}

	progress, err := service.GetChatImportJobStatus(jobID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(http.StatusOK).JSON(progress)
}

// ChatImportReusableIdentities 获取可复用的身份列表
func ChatImportReusableIdentities(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未授权"})
	}

	channelID := c.Params("channelId")
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}

	// 获取频道及其世界
	channel, err := model.ChannelGet(channelID)
	if err != nil || channel == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "频道不存在"})
	}

	if channel.WorldID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "仅支持世界频道"})
	}

	// 可以指定用户ID（用于查询其他成员的身份）
	targetUserID := c.Query("userId", u.ID)

	channelIDs := []string{}
	if raw := strings.TrimSpace(c.Query("channelIds")); raw != "" {
		for _, id := range strings.Split(raw, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				channelIDs = append(channelIDs, id)
			}
		}
	}

	includeCurrent := false
	if raw := c.Query("includeCurrent"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			includeCurrent = parsed
		}
	}

	visibleOnly := true
	if raw := c.Query("visibleOnly"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			visibleOnly = parsed
		}
	}

	// 验证权限：必须是世界管理员才能查看其他成员的身份
	if targetUserID != u.ID && !service.IsWorldAdmin(channel.WorldID, u.ID) {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "无权限查看其他成员身份"})
	}

	identities, err := service.ListReusableIdentities(channel.WorldID, channelID, targetUserID, channelIDs, includeCurrent, visibleOnly)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "查询身份失败: " + err.Error()})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"identities": identities,
	})
}
