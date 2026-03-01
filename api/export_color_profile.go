package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
)

const (
	exportColorProfileMaxEntries = 2000
	exportColorProfileMaxBytes   = 256 * 1024
)

type exportColorProfileUpsertRequest struct {
	Colors map[string]string `json:"colors"`
}

func ExportColorProfileGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}
	if _, err := ensureExportColorProfileChannelAccess(user.ID, channelID); err != nil {
		return handleChannelAccessErr(c, err)
	}
	record, err := model.ExportColorProfileGet(user.ID, channelID)
	if err != nil {
		return wrapError(c, err, "获取导出颜色配置失败")
	}
	if record == nil {
		return c.JSON(fiber.Map{
			"channelId": channelID,
			"exists":    false,
			"colors":    map[string]string{},
		})
	}
	colors := parseExportColorProfileJSON(record.ColorsJSON)
	resp := fiber.Map{
		"channelId": channelID,
		"exists":    true,
		"colors":    colors,
	}
	if !record.UpdatedAt.IsZero() {
		resp["updatedAt"] = record.UpdatedAt.UnixMilli()
	}
	return c.JSON(resp)
}

func ExportColorProfileUpsert(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}
	if _, err := ensureExportColorProfileChannelAccess(user.ID, channelID); err != nil {
		return handleChannelAccessErr(c, err)
	}
	var body exportColorProfileUpsertRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	colors, err := normalizeExportColorMap(body.Colors)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	encoded, err := encodeExportColorProfileJSON(colors)
	if err != nil {
		return wrapError(c, err, "保存导出颜色配置失败")
	}
	record, err := model.ExportColorProfileUpsert(user.ID, channelID, encoded)
	if err != nil {
		return wrapError(c, err, "保存导出颜色配置失败")
	}
	resp := fiber.Map{
		"channelId": channelID,
		"exists":    true,
		"colors":    colors,
	}
	if record != nil && !record.UpdatedAt.IsZero() {
		resp["updatedAt"] = record.UpdatedAt.UnixMilli()
	}
	return c.JSON(resp)
}

func ExportColorProfileDelete(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}
	if _, err := ensureExportColorProfileChannelAccess(user.ID, channelID); err != nil {
		return handleChannelAccessErr(c, err)
	}
	if err := model.ExportColorProfileDelete(user.ID, channelID); err != nil {
		return wrapError(c, err, "删除导出颜色配置失败")
	}
	return c.JSON(fiber.Map{
		"channelId": channelID,
		"exists":    false,
		"colors":    map[string]string{},
		"success":   true,
	})
}

func ensureExportColorProfileChannelAccess(userID, channelID string) (any, error) {
	return resolveChannelAccess(userID, channelID)
}

func handleChannelAccessErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, fiber.ErrForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "没有访问该频道的权限"})
	case errors.Is(err, fiber.ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "校验频道权限失败"})
	}
}

func encodeExportColorProfileJSON(colors map[string]string) (string, error) {
	if len(colors) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(colors)
	if err != nil {
		return "", err
	}
	if len(data) > exportColorProfileMaxBytes {
		return "", fiber.NewError(http.StatusBadRequest, "颜色配置过大")
	}
	return string(data), nil
}

func parseExportColorProfileJSON(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]string{}
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return map[string]string{}
	}
	normalized, err := normalizeExportColorMap(payload)
	if err != nil {
		return map[string]string{}
	}
	return normalized
}

func normalizeExportColorMap(input map[string]string) (map[string]string, error) {
	if len(input) == 0 {
		return map[string]string{}, nil
	}
	if len(input) > exportColorProfileMaxEntries {
		return nil, fiber.NewError(http.StatusBadRequest, "颜色配置条目过多")
	}
	result := make(map[string]string, len(input))
	for rawKey, rawColor := range input {
		key := strings.TrimSpace(rawKey)
		if !strings.HasPrefix(key, "identity:") {
			continue
		}
		identityID := strings.TrimSpace(strings.TrimPrefix(key, "identity:"))
		if identityID == "" {
			continue
		}
		color, ok := normalizeHexColor(rawColor)
		if !ok {
			continue
		}
		result["identity:"+identityID] = color
	}
	return result, nil
}

func normalizeHexColor(input string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "" {
		return "", false
	}
	if !strings.HasPrefix(value, "#") {
		value = "#" + value
	}
	hex := strings.TrimPrefix(value, "#")
	if len(hex) == 3 && isHexDigits(hex) {
		return "#" + strings.Repeat(string(hex[0]), 2) + strings.Repeat(string(hex[1]), 2) + strings.Repeat(string(hex[2]), 2), true
	}
	if len(hex) == 6 && isHexDigits(hex) {
		return "#" + hex, true
	}
	return "", false
}

func isHexDigits(input string) bool {
	for _, ch := range input {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
