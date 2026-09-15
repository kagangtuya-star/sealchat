package api

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
	"sealchat/utils"
)

func AdminCertificateConfigGet(ctx *fiber.Ctx) error {
	cfg := sanitizeConfigForAdmin(appConfig).Certificate
	return ctx.JSON(fiber.Map{
		"config":          cfg,
		"restartRequired": certificateRuntimeRestartRequired(),
	})
}

func AdminCertificateConfigUpdate(ctx *fiber.Ctx) error {
	var body struct {
		Config                utils.CertificateConfig `json:"config"`
		ClearZeroSSLAPIKey    bool                    `json:"clearZeroSSLAPIKey"`
		ClearZeroSSLEABMACKey bool                    `json:"clearZeroSSLEABMACKey"`
	}
	if err := ctx.BodyParser(&body); err != nil {
		return err
	}
	current := appConfig
	if current == nil {
		current = &utils.AppConfig{}
	}
	incoming := *current
	incoming.Certificate = body.Config
	merged := mergeConfigForWrite(current, &incoming)
	if body.ClearZeroSSLAPIKey {
		merged.Certificate.ZeroSSLAPIKey = ""
	}
	if body.ClearZeroSSLEABMACKey {
		merged.Certificate.ZeroSSLEABMACKey = ""
	}
	if err := normalizeAndValidateCertificateConfigForWrite(merged); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}

	appConfig = merged
	utils.WriteConfig(appConfig)
	SyncConfigToDB(appConfig, "api")
	return ctx.JSON(fiber.Map{
		"config":          sanitizeConfigForAdmin(appConfig).Certificate,
		"restartRequired": certificateRuntimeRestartRequired(),
	})
}

func AdminCertificateStatus(ctx *fiber.Ctx) error {
	status := certificateRuntimeStatus(ctx.Context())
	return ctx.JSON(fiber.Map{
		"status":          status,
		"restartRequired": certificateRuntimeRestartRequired(),
	})
}

func AdminCertificateLogs(ctx *fiber.Ctx) error {
	limit := ctx.QueryInt("limit", 100)
	manager := currentRuntimeCertificateManager()
	if manager == nil {
		return ctx.JSON(fiber.Map{
			"items": []service.CertificateLogEntry{},
		})
	}
	return ctx.JSON(fiber.Map{
		"items": manager.Logs(limit),
	})
}

func AdminCertificateObtain(ctx *fiber.Ctx) error {
	if certificateRuntimeRestartRequired() {
		return ctx.Status(fiber.StatusConflict).JSON(fiber.Map{
			"message": "证书配置已变更，请重启服务后再执行证书操作",
		})
	}
	manager := currentRuntimeCertificateManager()
	if manager == nil {
		return ctx.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "证书管理器未启用"})
	}
	var body struct {
		Force bool `json:"force"`
	}
	if len(ctx.Body()) > 0 {
		if err := ctx.BodyParser(&body); err != nil {
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求体格式无效"})
		}
	}
	obtainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var err error
	if body.Force {
		err = manager.ForceRenewNow(obtainCtx)
	} else {
		err = manager.CheckNow(obtainCtx)
	}
	if err != nil {
		return ctx.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"message": err.Error(),
		})
	}
	return ctx.JSON(fiber.Map{
		"status": manager.Status(context.Background()),
	})
}

func certificateRuntimeStatus(ctx context.Context) service.CertificateStatus {
	if manager := currentRuntimeCertificateManager(); manager != nil {
		return manager.Status(ctx)
	}
	if appConfig == nil {
		return service.CertificateStatus{}
	}
	cfg := utils.NormalizeCertificateConfig(appConfig.Certificate)
	return service.CertificateStatus{
		Enabled:              cfg.Enabled,
		RuntimeActive:        false,
		SubjectIP:            cfg.SubjectIP,
		Issuer:               string(cfg.Issuer),
		Challenge:            string(cfg.Challenge),
		RenewBeforeDays:      cfg.RenewBeforeDays,
		CheckIntervalMinutes: cfg.CheckIntervalMinutes,
		RetryInitialMinutes:  cfg.RetryInitialMinutes,
		RetryMaxMinutes:      cfg.RetryMaxMinutes,
	}
}
