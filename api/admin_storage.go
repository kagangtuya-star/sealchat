package api

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
	"sealchat/service/storage"
	"sealchat/utils"
)

func AdminStorageStatus(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	return c.JSON(service.GetStorageStatus())
}

func AdminStorageS3Test(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	var cfg utils.StorageConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "请求体解析失败"})
	}
	if err := validateS3CredentialsForWrite(cfg.S3); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	if strings.TrimSpace(cfg.S3.Endpoint) == "" || strings.TrimSpace(cfg.S3.Bucket) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "message": "请填写 Endpoint 和 Bucket"})
	}
	if appConfig == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"success": false, "message": "配置未加载"})
	}
	incoming := *appConfig
	incoming.Storage = cfg
	merged := mergeConfigForWrite(appConfig, &incoming)
	if err := service.TestStorageS3(merged.Storage); err != nil {
		if errors.Is(err, storage.ErrTTSPrivateReadUnverified) {
			return c.JSON(fiber.Map{"success": false, "message": storage.ErrTTSPrivateReadUnverified.Error()})
		}
		// Do not expose SDK errors, which may include credentials or signed URLs.
		return c.JSON(fiber.Map{"success": false, "message": "S3 读写测试失败，请检查 Endpoint、Region、Bucket、凭据及读写权限"})
	}
	return c.JSON(fiber.Map{"success": true, "message": "Bucket 写入、读取与删除测试通过；配置尚未保存"})
}
