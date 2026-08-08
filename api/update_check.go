package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

func AdminUpdateStatus(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	cfg := utils.GetConfig()
	if cfg == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"message": "update configuration is unavailable"})
	}
	resp, err := service.GetUpdateOverview(cfg.UpdateCheck, false)
	if err != nil {
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(resp)
}

func AdminUpdateCheck(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	cfg := utils.GetConfig()
	if cfg == nil || !cfg.UpdateCheck.Enabled {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "update check is disabled",
		})
	}
	resp, err := service.GetUpdateOverview(cfg.UpdateCheck, true)
	if err != nil {
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(resp)
}

func AdminUpdateApply(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	var payload struct {
		Channel           string `json:"channel"`
		ExpectedReleaseID int64  `json:"expectedReleaseId"`
		ExpectedAssetID   int64  `json:"expectedAssetId"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return err
	}
	cfg := utils.GetConfig()
	if cfg == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{"message": "update configuration is unavailable"})
	}
	job, err := service.StartUpdate(cfg.UpdateCheck, strings.TrimSpace(payload.Channel), payload.ExpectedReleaseID, payload.ExpectedAssetID)
	if err == nil {
		return c.Status(http.StatusAccepted).JSON(job)
	}
	status := http.StatusBadRequest
	if errors.Is(err, service.ErrUpdateBusy) || errors.Is(err, service.ErrReleaseChanged) {
		status = http.StatusConflict
	}
	return c.Status(status).JSON(fiber.Map{"message": err.Error()})
}

func AdminUpdateVersion(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	var payload struct {
		CurrentVersion string `json:"currentVersion"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return err
	}
	current := strings.TrimSpace(payload.CurrentVersion)
	if current == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "currentVersion is required",
		})
	}
	state, err := model.UpdateCheckStateGet()
	if err != nil {
		return err
	}
	if state == nil {
		state = &model.UpdateCheckState{}
	}
	state.CurrentVersion = current
	if err := model.UpdateCheckStateUpsert(state); err != nil {
		return err
	}
	return AdminUpdateStatus(c)
}
