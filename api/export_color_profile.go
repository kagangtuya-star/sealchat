package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
)

const (
	exportColorProfileFormatVersion    = 2
	exportColorProfileMaxEntries       = 2000
	exportColorProfileMaxBytes         = 256 * 1024
	exportColorProfileMaxNameRunes     = 64
	exportColorProfileMaxOriginalRunes = 64
)

type exportColorProfileEntry struct {
	Color        string `json:"color,omitempty"`
	Name         string `json:"name,omitempty"`
	OriginalName string `json:"originalName,omitempty"`
}

type exportColorProfileDocument struct {
	Version  int                                `json:"version,omitempty"`
	Profiles map[string]exportColorProfileEntry `json:"profiles,omitempty"`
}

type exportColorProfileUpsertRequest struct {
	Colors   map[string]string                  `json:"colors"`
	Profiles map[string]exportColorProfileEntry `json:"profiles"`
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
		resolvedProfiles, resolveErr := resolveExportColorProfiles(user.ID, channelID, map[string]exportColorProfileEntry{})
		if resolveErr != nil {
			return wrapError(c, resolveErr, "获取导出颜色配置失败")
		}
		return c.JSON(fiber.Map{
			"channelId": channelID,
			"exists":    false,
			"colors":    buildExportColorMapFromProfiles(resolvedProfiles),
			"profiles":  resolvedProfiles,
		})
	}
	profiles := parseExportColorProfileJSON(record.ColorsJSON)
	resolvedProfiles, err := resolveExportColorProfiles(user.ID, channelID, profiles)
	if err != nil {
		return wrapError(c, err, "获取导出颜色配置失败")
	}
	resp := fiber.Map{
		"channelId": channelID,
		"exists":    true,
		"colors":    buildExportColorMapFromProfiles(resolvedProfiles),
		"profiles":  resolvedProfiles,
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
	profiles, err := normalizeExportColorProfiles(body.Profiles)
	if err == nil && len(profiles) == 0 && len(body.Profiles) == 0 {
		legacyColors, legacyErr := normalizeExportColorMap(body.Colors)
		if legacyErr != nil {
			err = legacyErr
		} else {
			profiles = buildProfilesFromColorMap(legacyColors)
		}
	}
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	encoded, err := encodeExportColorProfileJSON(profiles)
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
		"colors":    buildExportColorMapFromProfiles(profiles),
		"profiles":  profiles,
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

func encodeExportColorProfileJSON(profiles map[string]exportColorProfileEntry) (string, error) {
	if len(profiles) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(exportColorProfileDocument{
		Version:  exportColorProfileFormatVersion,
		Profiles: profiles,
	})
	if err != nil {
		return "", err
	}
	if len(data) > exportColorProfileMaxBytes {
		return "", fiber.NewError(http.StatusBadRequest, "颜色配置过大")
	}
	return string(data), nil
}

func parseExportColorProfileJSON(raw string) map[string]exportColorProfileEntry {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]exportColorProfileEntry{}
	}
	var doc exportColorProfileDocument
	if err := json.Unmarshal([]byte(trimmed), &doc); err == nil && len(doc.Profiles) > 0 {
		normalized, err := normalizeExportColorProfiles(doc.Profiles)
		if err == nil {
			return normalized
		}
	}
	var legacy map[string]string
	if err := json.Unmarshal([]byte(trimmed), &legacy); err != nil {
		return map[string]exportColorProfileEntry{}
	}
	normalized, err := normalizeExportColorMap(legacy)
	if err != nil {
		return map[string]exportColorProfileEntry{}
	}
	return buildProfilesFromColorMap(normalized)
}

func normalizeExportColorMap(input map[string]string) (map[string]string, error) {
	if len(input) == 0 {
		return map[string]string{}, nil
	}
	profiles := make(map[string]exportColorProfileEntry, len(input))
	for key, color := range input {
		profiles[key] = exportColorProfileEntry{Color: color}
	}
	normalizedProfiles, err := normalizeExportColorProfiles(profiles)
	if err != nil {
		return nil, err
	}
	return buildExportColorMapFromProfiles(normalizedProfiles), nil
}

func normalizeExportNameMap(input map[string]string) (map[string]string, error) {
	if len(input) == 0 {
		return map[string]string{}, nil
	}
	if len(input) > exportColorProfileMaxEntries {
		return nil, fiber.NewError(http.StatusBadRequest, "名字配置条目过多")
	}
	result := make(map[string]string, len(input))
	for rawKey, rawName := range input {
		key := normalizeExportProfileKey(rawKey)
		if key == "" {
			continue
		}
		name, err := normalizeExportProfileText(rawName, exportColorProfileMaxNameRunes, "自定义名字过长")
		if err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		result[key] = name
	}
	return result, nil
}

func normalizeExportColorProfiles(input map[string]exportColorProfileEntry) (map[string]exportColorProfileEntry, error) {
	if len(input) == 0 {
		return map[string]exportColorProfileEntry{}, nil
	}
	if len(input) > exportColorProfileMaxEntries {
		return nil, fiber.NewError(http.StatusBadRequest, "颜色配置条目过多")
	}
	result := make(map[string]exportColorProfileEntry, len(input))
	for rawKey, rawEntry := range input {
		key := normalizeExportProfileKey(rawKey)
		if key == "" {
			continue
		}
		entry := exportColorProfileEntry{}
		if color, ok := normalizeHexColor(rawEntry.Color); ok {
			entry.Color = color
		}
		name, err := normalizeExportProfileText(rawEntry.Name, exportColorProfileMaxNameRunes, "自定义名字过长")
		if err != nil {
			return nil, err
		}
		originalName, err := normalizeExportProfileText(rawEntry.OriginalName, exportColorProfileMaxOriginalRunes, "原始名字过长")
		if err != nil {
			return nil, err
		}
		entry.Name = name
		entry.OriginalName = originalName
		if entry.Color == "" && entry.Name == "" {
			continue
		}
		result[key] = entry
	}
	return result, nil
}

func buildProfilesFromColorMap(colors map[string]string) map[string]exportColorProfileEntry {
	if len(colors) == 0 {
		return map[string]exportColorProfileEntry{}
	}
	profiles := make(map[string]exportColorProfileEntry, len(colors))
	for key, color := range colors {
		profiles[key] = exportColorProfileEntry{Color: color}
	}
	return profiles
}

func buildExportColorMapFromProfiles(profiles map[string]exportColorProfileEntry) map[string]string {
	if len(profiles) == 0 {
		return map[string]string{}
	}
	colors := make(map[string]string, len(profiles))
	for key, entry := range profiles {
		if entry.Color == "" {
			continue
		}
		colors[key] = entry.Color
	}
	return colors
}

func normalizeExportProfileKey(rawKey string) string {
	key := strings.TrimSpace(rawKey)
	if !strings.HasPrefix(key, "identity:") {
		return ""
	}
	identityID := strings.TrimSpace(strings.TrimPrefix(key, "identity:"))
	if identityID == "" {
		return ""
	}
	return "identity:" + identityID
}

func normalizeExportProfileText(input string, maxRunes int, errMsg string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if maxRunes > 0 && utf8.RuneCountInString(value) > maxRunes {
		return "", fiber.NewError(http.StatusBadRequest, errMsg)
	}
	return value, nil
}

func resolveExportColorProfiles(userID, channelID string, current map[string]exportColorProfileEntry) (map[string]exportColorProfileEntry, error) {
	options, err := model.ChannelIdentityOptionListActive(channelID)
	if err != nil {
		return nil, err
	}
	matchIndex, err := buildReusableExportProfileMatchIndex(userID)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string]exportColorProfileEntry)
	for _, option := range options {
		if option == nil {
			continue
		}
		key := normalizeExportProfileKey("identity:" + strings.TrimSpace(option.ID))
		if key == "" {
			continue
		}
		originalName := strings.TrimSpace(option.Label)
		if originalName == "" {
			originalName = "未命名角色"
		}
		entry := current[key]
		if entry.OriginalName == "" {
			entry.OriginalName = originalName
		}
		if reusable, ok := matchIndex[normalizeExportProfileMatchName(originalName)]; ok {
			if entry.Color == "" {
				entry.Color = reusable.Color
			}
			if entry.Name == "" {
				entry.Name = reusable.Name
			}
		}
		if entry.Color == "" && entry.Name == "" {
			continue
		}
		resolved[key] = entry
	}
	return resolved, nil
}

func buildReusableExportProfileMatchIndex(userID string) (map[string]exportColorProfileEntry, error) {
	records, err := model.ExportColorProfileListByUser(userID)
	if err != nil {
		return nil, err
	}
	index := make(map[string]exportColorProfileEntry)
	for _, record := range records {
		if record == nil {
			continue
		}
		profiles := parseExportColorProfileJSON(record.ColorsJSON)
		if len(profiles) == 0 {
			continue
		}
		keys := make([]string, 0, len(profiles))
		for key := range profiles {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			entry := profiles[key]
			if entry.Color == "" && entry.Name == "" {
				continue
			}
			matchKey := normalizeExportProfileMatchName(entry.OriginalName)
			if matchKey == "" {
				continue
			}
			if _, exists := index[matchKey]; exists {
				continue
			}
			index[matchKey] = exportColorProfileEntry{
				Color:        entry.Color,
				Name:         entry.Name,
				OriginalName: entry.OriginalName,
			}
		}
	}
	return index, nil
}

func normalizeExportProfileMatchName(input string) string {
	value := strings.TrimSpace(input)
	if value == "" {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
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
