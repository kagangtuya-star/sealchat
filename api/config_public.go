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

func OptionalSignCheckMiddleware(c *fiber.Ctx) error {
	if getCurUser(c) != nil {
		return c.Next()
	}

	token := getToken(c)
	token = strings.TrimSpace(token)
	if token == "" {
		return c.Next()
	}

	var (
		user *model.UserModel
		err  error
	)
	if len(token) == 32 {
		user, err = model.BotVerifyAccessToken(token)
	} else {
		user, err = model.UserVerifyAccessToken(token)
	}
	if err != nil || user == nil || user.Disabled {
		return c.Next()
	}

	c.Locals("user", user)
	return c.Next()
}

func ConfigGetHandler(c *fiber.Ctx) error {
	ret := sanitizeConfigForClient(appConfig)
	u := getCurUser(c)
	isAdmin := u != nil && pm.CanWithSystemRole(u.ID, pm.PermModAdmin)
	if !isAdmin {
		ret.ServeAt = ""
	} else if appConfig != nil {
		ret.RegisterInviteCode = appConfig.RegisterInviteCode
		ret.TheaterActivationCode = appConfig.TheaterActivationCode
	}
	ffmpegAvailable := false
	if svc := service.GetAudioService(); svc != nil {
		ffmpegAvailable = svc.FFmpegAvailable()
	}
	audioImportEnabled := false
	if appConfig != nil && strings.TrimSpace(appConfig.Audio.ImportDir) != "" {
		audioImportEnabled = true
	}
	resp := struct {
		utils.AppConfig
		FFmpegAvailable          bool `json:"ffmpegAvailable"`
		AllowWorldAudioWorkbench bool `json:"allowWorldAudioWorkbench"`
		AudioImportEnabled       bool `json:"audioImportEnabled"`
	}{
		AppConfig:                ret,
		FFmpegAvailable:          ffmpegAvailable,
		AllowWorldAudioWorkbench: ret.Audio.AllowWorldAudioWorkbench,
		AudioImportEnabled:       audioImportEnabled,
	}
	return c.Status(http.StatusOK).JSON(resp)
}
