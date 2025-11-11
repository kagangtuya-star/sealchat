package api

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
)

type botProfilePayload struct {
	Name            string                   `json:"name"`
	AvatarURL       string                   `json:"avatarUrl"`
	ChannelRoleName string                   `json:"channelRoleName"`
	ConnMode        string                   `json:"connMode"`
	RemoteSelfID    string                   `json:"remoteSelfId"`
	Forward         botProfileForwardPayload `json:"forward"`
	Reverse         botProfileReversePayload `json:"reverse"`
	AccessToken     string                   `json:"accessToken"`
	DefaultChannel  string                   `json:"defaultChannelId"`
	Enabled         bool                     `json:"enabled"`
}

type botProfileForwardPayload struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	APIPath       string `json:"apiPath"`
	EventPath     string `json:"eventPath"`
	UniversalPath string `json:"universalPath"`
}

type botProfileReversePayload struct {
	APIEndpoints       []string `json:"apiEndpoints"`
	EventEndpoints     []string `json:"eventEndpoints"`
	UniversalEndpoints []string `json:"universalEndpoints"`
	UseUniversal       bool     `json:"useUniversal"`
	ReconnectInterval  int      `json:"reconnectInterval"`
}

func (p *botProfilePayload) toModel() *model.BotProfileModel {
	profile := &model.BotProfileModel{
		Name:                 strings.TrimSpace(p.Name),
		AvatarURL:            strings.TrimSpace(p.AvatarURL),
		ChannelRoleName:      strings.TrimSpace(p.ChannelRoleName),
		ConnMode:             model.BotConnectionMode(p.ConnMode),
		RemoteSelfID:         strings.TrimSpace(p.RemoteSelfID),
		ForwardHost:          strings.TrimSpace(p.Forward.Host),
		ForwardPort:          p.Forward.Port,
		ForwardAPIPath:       strings.TrimSpace(p.Forward.APIPath),
		ForwardEventPath:     strings.TrimSpace(p.Forward.EventPath),
		ForwardUniversal:     strings.TrimSpace(p.Forward.UniversalPath),
		ReverseAPIEndpoints:  sanitizeStringSlice(p.Reverse.APIEndpoints),
		ReverseEventURLs:     sanitizeStringSlice(p.Reverse.EventEndpoints),
		ReverseUniversalURLs: sanitizeStringSlice(p.Reverse.UniversalEndpoints),
		ReverseUseUniversal:  p.Reverse.UseUniversal,
		ReverseReconnectSec:  p.Reverse.ReconnectInterval,
		AccessToken:          strings.TrimSpace(p.AccessToken),
		DefaultChannelID:     strings.TrimSpace(p.DefaultChannel),
		Enabled:              p.Enabled,
	}
	if profile.ForwardPort == 0 {
		profile.ForwardPort = 33212
	}
	if profile.ReverseReconnectSec <= 0 {
		profile.ReverseReconnectSec = 10
	}
	return profile
}

func sanitizeStringSlice(items []string) model.JSONStringSlice {
	if len(items) == 0 {
		return nil
	}
	buf := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		buf = append(buf, item)
	}
	if len(buf) == 0 {
		return nil
	}
	return model.JSONStringSlice(buf)
}

func AdminBotProfileList(c *fiber.Ctx) error {
	items, err := service.ListBotProfiles(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"items": items,
		"total": len(items),
	})
}

func AdminBotProfileCreate(c *fiber.Ctx) error {
	var payload botProfilePayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "请求参数错误",
		})
	}
	cur := getCurUser(c)
	profile := payload.toModel()
	profile.CreatedBy = cur.ID
	profile.UpdatedBy = cur.ID

	view, err := service.UpsertBotProfile(c.Context(), profile)
	if err != nil {
		return err
	}
	return c.JSON(view)
}

func AdminBotProfileUpdate(c *fiber.Ctx) error {
	var payload botProfilePayload
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "请求参数错误",
		})
	}
	botID := c.Params("id")
	existing, err := model.BotProfileGet(botID)
	if err != nil {
		return err
	}
	cur := getCurUser(c)
	profile := payload.toModel()
	profile.ID = botID
	profile.UserID = existing.UserID
	if strings.TrimSpace(payload.RemoteSelfID) == "" {
		profile.RemoteSelfID = existing.RemoteSelfID
	}
	if profile.CreatedBy == "" {
		profile.CreatedBy = existing.CreatedBy
	}
	profile.UpdatedBy = cur.ID

	view, err := service.UpsertBotProfile(c.Context(), profile)
	if err != nil {
		return err
	}
	return c.JSON(view)
}

func AdminBotProfileDelete(c *fiber.Ctx) error {
	if err := service.DeleteBotProfile(c.Context(), c.Params("id")); err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"message": "删除成功",
	})
}

func AdminBotProfileTest(c *fiber.Ctx) error {
	botID := c.Params("id")
	view, err := service.GetBotProfileView(c.Context(), botID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "已收到测试请求，OneBot 网关后续版本将提供实时互通验证。",
		"profile": view,
	})
}
