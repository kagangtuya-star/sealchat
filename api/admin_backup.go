package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

func AdminBackupList(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	cfg := utils.GetConfig()
	if cfg == nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, nil, "配置未加载")
	}
	items, err := service.ListBackups(cfg.Backup)
	if err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "获取备份列表失败")
	}
	return c.Status(http.StatusOK).JSON(items)
}

func AdminBackupExecute(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	cfg := utils.GetConfig()
	if cfg == nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, nil, "配置未加载")
	}
	info, err := service.ExecuteBackup(cfg)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrBackupRunning) {
			status = http.StatusConflict
		} else if errors.Is(err, service.ErrBackupUnsupported) {
			status = http.StatusBadRequest
		}
		return wrapErrorStatus(c, status, err, "执行备份失败")
	}
	return c.Status(http.StatusOK).JSON(info)
}

func AdminBackupDelete(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	var payload struct {
		Filename string `json:"filename"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, http.StatusBadRequest, err, "请求体解析失败")
	}
	filename := strings.TrimSpace(payload.Filename)
	if filename == "" {
		return wrapErrorStatus(c, http.StatusBadRequest, nil, "filename 不能为空")
	}
	cfg := utils.GetConfig()
	if cfg == nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, nil, "配置未加载")
	}
	if err := service.DeleteBackup(cfg.Backup, filename); err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, err, "删除备份失败")
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"message": "ok"})
}
