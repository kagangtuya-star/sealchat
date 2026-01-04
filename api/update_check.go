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

type updateStatusResponse struct {
	CurrentVersion    string `json:"currentVersion"`
	LatestTag         string `json:"latestTag"`
	LatestName        string `json:"latestName"`
	LatestBody        string `json:"latestBody"`
	LatestPublishedAt int64  `json:"latestPublishedAt"`
	LatestHtmlURL     string `json:"latestHtmlUrl"`
	LastCheckedAt     int64  `json:"lastCheckedAt"`
	HasUpdate         bool   `json:"hasUpdate"`
}

func AdminUpdateStatus(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return nil
	}
	state, err := model.UpdateCheckStateGet()
	if err != nil {
		return err
	}
	currentVersion := strings.TrimSpace(utils.BuildVersion)
	resp := updateStatusResponse{
		CurrentVersion: currentVersion,
	}
	if state != nil {
		if resp.CurrentVersion == "" && strings.TrimSpace(state.CurrentVersion) != "" {
			resp.CurrentVersion = strings.TrimSpace(state.CurrentVersion)
		}
		resp.LatestTag = state.LatestTag
		resp.LatestName = state.LatestName
		resp.LatestBody = state.LatestBody
		resp.LatestPublishedAt = state.LatestPublishedAt
		resp.LatestHtmlURL = state.LatestHtmlURL
		resp.LastCheckedAt = state.LastCheckedAt
		resp.HasUpdate = service.IsLatestNewer(resp.CurrentVersion, state.LatestTag)
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
	currentVersion := strings.TrimSpace(utils.BuildVersion)
	if currentVersion == "" {
		if state, _ := model.UpdateCheckStateGet(); state != nil && strings.TrimSpace(state.CurrentVersion) != "" {
			currentVersion = strings.TrimSpace(state.CurrentVersion)
		}
	}
	service.UpdateCheckOnce(service.UpdateCheckWorkerConfig{
		IntervalSec:   cfg.UpdateCheck.IntervalSec,
		GithubRepo:    cfg.UpdateCheck.GithubRepo,
		GithubToken:   cfg.UpdateCheck.GithubToken,
		CurrentVersion: currentVersion,
	})
	return AdminUpdateStatus(c)
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
