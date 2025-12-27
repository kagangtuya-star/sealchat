package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

// EmailNotificationSettingsDTO 邮件通知设置 DTO
type EmailNotificationSettingsDTO struct {
	Enabled       bool   `json:"enabled"`
	Email         string `json:"email"`
	DelayMinutes  int    `json:"delayMinutes"`
	UseCustomSMTP bool   `json:"useCustomSmtp"`
	SMTPHost      string `json:"smtpHost"`
	SMTPPort      int    `json:"smtpPort"`
	SMTPUsername  string `json:"smtpUsername"`
	SMTPPassword  string `json:"smtpPassword"`
	SMTPFrom      string `json:"smtpFromAddress"`
	SMTPFromName  string `json:"smtpFromName"`
	SMTPUseTLS    bool   `json:"smtpUseTls"`
}

// EmailNotificationSettingsGet 获取邮件通知设置
func EmailNotificationSettingsGet(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}

	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	// 检查用户是否有权限访问该频道
	if !CanWithChannelRole(c, channelID, pm.PermFuncChannelRead) {
		return nil
	}

	// 检查邮件通知功能是否启用
	cfg := utils.GetConfig()
	if cfg == nil || !cfg.EmailNotification.Enabled {
		return c.JSON(fiber.Map{
			"enabled":         false,
			"featureDisabled": true,
			"message":         "邮件通知功能未启用",
		})
	}

	setting, err := model.EmailNotificationSettingsGet(user.ID, channelID)
	if err != nil {
		return wrapError(c, err, "获取设置失败")
	}

	if setting == nil {
		return c.JSON(fiber.Map{
			"enabled":      false,
			"email":        "",
			"delayMinutes": cfg.EmailNotification.MinDelayMinutes,
			"minDelay":     cfg.EmailNotification.MinDelayMinutes,
			"maxDelay":     cfg.EmailNotification.MaxDelayMinutes,
		})
	}

	return c.JSON(fiber.Map{
		"enabled":         setting.Enabled,
		"email":           setting.Email,
		"delayMinutes":    setting.DelayMinutes,
		"minDelay":        cfg.EmailNotification.MinDelayMinutes,
		"maxDelay":        cfg.EmailNotification.MaxDelayMinutes,
		"useCustomSmtp":   setting.UseCustomSMTP,
		"smtpHost":        setting.SMTPHost,
		"smtpPort":        setting.SMTPPort,
		"smtpUsername":    setting.SMTPUsername,
		"smtpFromAddress": setting.SMTPFromAddress,
		"smtpFromName":    setting.SMTPFromName,
		"smtpUseTls":      setting.SMTPUseTLS,
		"hasPassword":     setting.SMTPPassword != "",
	})
}

// EmailNotificationSettingsUpsert 创建或更新邮件通知设置
func EmailNotificationSettingsUpsert(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}

	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	if !CanWithChannelRole(c, channelID, pm.PermFuncChannelRead) {
		return nil
	}

	cfg := utils.GetConfig()
	if cfg == nil || !cfg.EmailNotification.Enabled {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "邮件通知功能未启用"})
	}

	var body EmailNotificationSettingsDTO
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}

	email := strings.TrimSpace(body.Email)
	if body.Enabled && email == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请填写邮箱地址"})
	}

	// 验证邮箱格式（简单校验）
	if email != "" && !strings.Contains(email, "@") {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "邮箱格式不正确"})
	}

	// 验证延迟时间范围
	delayMinutes := body.DelayMinutes
	if delayMinutes < cfg.EmailNotification.MinDelayMinutes {
		delayMinutes = cfg.EmailNotification.MinDelayMinutes
	}
	if delayMinutes > cfg.EmailNotification.MaxDelayMinutes {
		delayMinutes = cfg.EmailNotification.MaxDelayMinutes
	}

	setting, err := model.EmailNotificationSettingsUpsert(user.ID, channelID, model.EmailNotificationSettingsUpsertParams{
		Email:         email,
		DelayMinutes:  delayMinutes,
		Enabled:       body.Enabled,
		UseCustomSMTP: body.UseCustomSMTP,
		SMTPHost:      strings.TrimSpace(body.SMTPHost),
		SMTPPort:      body.SMTPPort,
		SMTPUsername:  strings.TrimSpace(body.SMTPUsername),
		SMTPPassword:  body.SMTPPassword,
		SMTPFrom:      strings.TrimSpace(body.SMTPFrom),
		SMTPFromName:  strings.TrimSpace(body.SMTPFromName),
		SMTPUseTLS:    body.SMTPUseTLS,
	})
	if err != nil {
		return wrapError(c, err, "保存设置失败")
	}

	return c.JSON(fiber.Map{
		"enabled":         setting.Enabled,
		"email":           setting.Email,
		"delayMinutes":    setting.DelayMinutes,
		"useCustomSmtp":   setting.UseCustomSMTP,
		"smtpHost":        setting.SMTPHost,
		"smtpPort":        setting.SMTPPort,
		"smtpUsername":    setting.SMTPUsername,
		"smtpFromAddress": setting.SMTPFromAddress,
		"smtpFromName":    setting.SMTPFromName,
		"smtpUseTls":      setting.SMTPUseTLS,
		"hasPassword":     setting.SMTPPassword != "",
	})
}

// EmailNotificationSettingsDelete 删除邮件通知设置
func EmailNotificationSettingsDelete(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}

	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	if !CanWithChannelRole(c, channelID, pm.PermFuncChannelRead) {
		return nil
	}

	if err := model.EmailNotificationSettingsDelete(user.ID, channelID); err != nil {
		return wrapError(c, err, "删除设置失败")
	}

	return c.JSON(fiber.Map{"success": true})
}

// EmailNotificationTestSend 发送测试邮件（支持用户自定义 SMTP）
func EmailNotificationTestSend(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}

	cfg := utils.GetConfig()
	if cfg == nil || !cfg.EmailNotification.Enabled {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "邮件通知功能未启用"})
	}

	var body struct {
		ChannelID     string `json:"channelId"`
		UseCustomSMTP bool   `json:"useCustomSmtp"`
		SMTPHost      string `json:"smtpHost"`
		SMTPPort      int    `json:"smtpPort"`
		SMTPUsername  string `json:"smtpUsername"`
		SMTPPassword  string `json:"smtpPassword"`
		SMTPFrom      string `json:"smtpFromAddress"`
		SMTPFromName  string `json:"smtpFromName"`
		SMTPUseTLS    bool   `json:"smtpUseTls"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}

	channelID := strings.TrimSpace(body.ChannelID)
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}
	if !CanWithChannelRole(c, channelID, pm.PermFuncChannelRead) {
		return nil
	}

	setting, err := model.EmailNotificationSettingsGet(user.ID, channelID)
	if err != nil {
		return wrapError(c, err, "获取设置失败")
	}
	if setting == nil || strings.TrimSpace(setting.Email) == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请先保存当前频道的邮箱设置"})
	}

	email := strings.TrimSpace(setting.Email)
	if !strings.Contains(email, "@") {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "邮箱格式不正确"})
	}

	// 构建测试邮件
	testMessages := []service.MessageSummary{
		{
			SenderName: "测试用户",
			Content:    "这是一封测试邮件，用于验证邮件通知功能是否正常工作。",
			Time:       c.Context().Time(),
		},
	}
	htmlBody := service.BuildUnreadDigestHTML("测试频道", testMessages, "", "")
	subject := "【SealChat】邮件通知测试"

	// 选择 SMTP 配置
	var emailSvc *service.EmailService
	if body.UseCustomSMTP && strings.TrimSpace(body.SMTPHost) != "" {
		customCfg := utils.SMTPConfig{
			Host:        strings.TrimSpace(body.SMTPHost),
			Port:        body.SMTPPort,
			Username:    strings.TrimSpace(body.SMTPUsername),
			Password:    body.SMTPPassword,
			FromAddress: strings.TrimSpace(body.SMTPFrom),
			FromName:    strings.TrimSpace(body.SMTPFromName),
			UseTLS:      body.SMTPUseTLS,
		}
		emailSvc = service.NewEmailService(customCfg)
		if !emailSvc.IsConfigured() {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "自定义 SMTP 配置不完整"})
		}
	} else {
		emailSvc = service.NewEmailService(cfg.EmailNotification.SMTP)
		if !emailSvc.IsConfigured() {
			return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"message": "SMTP 服务未配置"})
		}
	}

	if err := emailSvc.SendEmail(email, subject, htmlBody); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "发送失败: " + err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "测试邮件已发送"})
}

// AdminEmailTestSend 管理员测试 SMTP 配置（不需要启用邮件通知功能）
func AdminEmailTestSend(c *fiber.Ctx) error {
	var body struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}

	email := strings.TrimSpace(body.Email)
	if email == "" || !strings.Contains(email, "@") {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请填写有效的邮箱地址"})
	}

	cfg := utils.GetConfig()
	if cfg == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"message": "配置未加载"})
	}

	smtpCfg := cfg.EmailNotification.SMTP
	emailSvc := service.NewEmailService(smtpCfg)
	if !emailSvc.IsConfigured() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"message": "SMTP 未配置。请在 config.yaml 中配置 emailNotification.smtp 部分",
		})
	}

	// 构建简单测试邮件
	htmlBody := `<html><body style="font-family: sans-serif; padding: 20px;">
		<h2>SealChat SMTP 配置测试</h2>
		<p>如果您收到此邮件，说明 SMTP 服务器配置正确。</p>
		<p style="color: #666; font-size: 12px;">发送时间: ` + c.Context().Time().Format("2006-01-02 15:04:05") + `</p>
	</body></html>`
	subject := "【SealChat】SMTP 配置测试"

	if err := emailSvc.SendEmail(email, subject, htmlBody); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "发送失败: " + err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "测试邮件已发送至 " + email})
}
