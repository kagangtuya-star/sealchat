package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gofiber/fiber/v2"
	"io"
	"sealchat/service"
	"sealchat/utils"
)

func mcpConfigSnapshot() utils.AppConfig {
	configMutationMu.Lock()
	defer configMutationMu.Unlock()
	if appConfig == nil {
		return utils.AppConfig{MCP: utils.NormalizeMCPConfig(utils.MCPConfig{})}
	}
	c := *appConfig
	c.MCP = utils.NormalizeMCPConfig(c.MCP)
	return c
}
func decodeMCPJSON(raw []byte, out any) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return errors.New("JSON object required")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("single JSON object required")
	}
	return nil
}
func personalKeyHTTPError(c *fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	message := "个人 Key 操作失败"
	switch {
	case errors.Is(err, service.ErrPersonalKeyInvalid):
		status = 401
		message = "用户状态不允许创建个人 Key"
	case errors.Is(err, service.ErrMCPDisabled):
		status = 403
		message = "平台 MCP 接入已关闭"
	case errors.Is(err, service.ErrMCPScopeDenied):
		status = 403
		message = "授权范围超出平台开放能力"
	case errors.Is(err, service.ErrPersonalKeyInput):
		status = 400
		message = "名称、scope 或有效期无效"
	case errors.Is(err, service.ErrPersonalKeyLimit):
		status = 409
		message = "最多可保留 10 个未撤销 Key"
	case errors.Is(err, service.ErrPersonalKeyConflict):
		status = 409
		message = "Key 已撤销、不存在或已被修改"
	}
	return c.Status(status).JSON(fiber.Map{"message": message})
}
func bindPersonalAPIKeyRoutes(r fiber.Router) {
	r.Get("/user/api-keys", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		cfg := mcpConfigSnapshot()
		keys, err := service.ListPersonalAPIKeys(getCurUser(c).ID)
		if err != nil {
			return personalKeyHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"items": keys, "enabled": cfg.MCP.Enabled, "catalog": utils.MCPScopeCatalog, "allowedScopes": cfg.MCP.AllowedScopes(), "path": joinWebPath(cfg.WebUrl, "mcp")})
	})
	r.Post("/user/api-keys", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		var in service.PersonalKeyInput
		if decodeMCPJSON(c.Body(), &in) != nil {
			return personalKeyHTTPError(c, service.ErrPersonalKeyInput)
		}
		key, token, err := service.CreatePersonalAPIKey(getCurUser(c).ID, in, mcpConfigSnapshot().MCP)
		if err != nil {
			return personalKeyHTTPError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"key": key, "token": token})
	})
	r.Patch("/user/api-keys/:keyId", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		var in service.PersonalKeyPatch
		if decodeMCPJSON(c.Body(), &in) != nil {
			return personalKeyHTTPError(c, service.ErrPersonalKeyInput)
		}
		if err := service.UpdatePersonalAPIKey(getCurUser(c).ID, c.Params("keyId"), in, mcpConfigSnapshot().MCP); err != nil {
			return personalKeyHTTPError(c, err)
		}
		return c.SendStatus(204)
	})
	r.Delete("/user/api-keys/:keyId", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		if err := service.RevokePersonalAPIKey(getCurUser(c).ID, c.Params("keyId")); err != nil {
			return personalKeyHTTPError(c, err)
		}
		return c.SendStatus(204)
	})
	r.Post("/user/api-keys/:keyId/rotate", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "private, no-store")
		key, token, err := service.RotatePersonalAPIKey(getCurUser(c).ID, c.Params("keyId"), mcpConfigSnapshot().MCP)
		if err != nil {
			return personalKeyHTTPError(c, err)
		}
		return c.JSON(fiber.Map{"key": key, "token": token})
	})
}
