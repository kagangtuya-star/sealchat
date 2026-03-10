package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
)

func canViewAllInputStatsMeta(userID string) bool {
	return pm.CanWithSystemRole(userID, pm.PermModAdmin)
}

func loadVisibleWorldIDs(userID string, worldIDs []string) (map[string]struct{}, error) {
	visible := make(map[string]struct{}, len(worldIDs))
	if canViewAllInputStatsMeta(userID) {
		for _, worldID := range worldIDs {
			worldID = strings.TrimSpace(worldID)
			if worldID != "" {
				visible[worldID] = struct{}{}
			}
		}
		return visible, nil
	}

	uniqWorldIDs := make([]string, 0, len(worldIDs))
	seen := make(map[string]struct{}, len(worldIDs))
	for _, worldID := range worldIDs {
		worldID = strings.TrimSpace(worldID)
		if worldID == "" {
			continue
		}
		if _, ok := seen[worldID]; ok {
			continue
		}
		seen[worldID] = struct{}{}
		uniqWorldIDs = append(uniqWorldIDs, worldID)
	}
	if len(uniqWorldIDs) == 0 {
		return visible, nil
	}

	var memberWorldIDs []string
	if err := model.GetDB().Table("world_members").
		Where("user_id = ? AND world_id IN ?", userID, uniqWorldIDs).
		Pluck("world_id", &memberWorldIDs).Error; err != nil {
		return nil, err
	}
	for _, worldID := range memberWorldIDs {
		visible[worldID] = struct{}{}
	}

	return visible, nil
}

// parseStatsFilterParams 解析筛选参数
// includeWorlds/excludeWorlds/includeChannels/excludeChannels 为逗号分隔的ID列表
func parseStatsFilterParams(c *fiber.Ctx) model.InputStatsFilter {
	var f model.InputStatsFilter

	if s := strings.TrimSpace(c.Query("start")); s != "" {
		if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
			t := time.UnixMilli(ms)
			f.StartTime = &t
		}
	}
	if s := strings.TrimSpace(c.Query("end")); s != "" {
		if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
			t := time.UnixMilli(ms)
			f.EndTime = &t
		}
	}

	icMode := strings.TrimSpace(c.Query("icMode"))
	if icMode == "ic" || icMode == "ooc" {
		f.ICMode = icMode
	}

	if s := strings.TrimSpace(c.Query("includeWorlds")); s != "" {
		f.IncludeWorldIDs = splitIDs(s)
	}
	if s := strings.TrimSpace(c.Query("excludeWorlds")); s != "" {
		f.ExcludeWorldIDs = splitIDs(s)
	}
	if s := strings.TrimSpace(c.Query("includeChannels")); s != "" {
		f.IncludeChannelIDs = splitIDs(s)
	}
	if s := strings.TrimSpace(c.Query("excludeChannels")); s != "" {
		f.ExcludeChannelIDs = splitIDs(s)
	}

	return f
}

func splitIDs(s string) []string {
	parts := strings.Split(s, ",")
	ids := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			ids = append(ids, p)
		}
	}
	return ids
}

// UserInputStatsOverview 用户输入统计概览
func UserInputStatsOverview(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return fiber.NewError(http.StatusUnauthorized, "未登录")
	}

	f := parseStatsFilterParams(c)
	overview, err := model.UserInputStatsOverall(u.ID, f)
	if err != nil {
		return fiber.NewError(http.StatusInternalServerError, err.Error())
	}

	return c.Status(http.StatusOK).JSON(overview)
}

// UserInputStatsByWorld 按世界分组统计
func UserInputStatsByWorld(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return fiber.NewError(http.StatusUnauthorized, "未登录")
	}

	f := parseStatsFilterParams(c)
	items, err := model.UserInputStatsByWorld(u.ID, f)
	if err != nil {
		return fiber.NewError(http.StatusInternalServerError, err.Error())
	}
	if !canViewAllInputStatsMeta(u.ID) && len(items) > 0 {
		worldIDs := make([]string, 0, len(items))
		for _, item := range items {
			worldIDs = append(worldIDs, item.WorldID)
		}
		visibleWorldIDs, err := loadVisibleWorldIDs(u.ID, worldIDs)
		if err != nil {
			return fiber.NewError(http.StatusInternalServerError, err.Error())
		}
		for i := range items {
			if _, ok := visibleWorldIDs[items[i].WorldID]; !ok {
				items[i].WorldName = "未知世界"
			}
		}
	}

	return c.Status(http.StatusOK).JSON(items)
}

// UserInputStatsByChannel 按频道分组统计
func UserInputStatsByChannel(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return fiber.NewError(http.StatusUnauthorized, "未登录")
	}

	worldID := strings.TrimSpace(c.Query("worldId"))
	if worldID == "" {
		return fiber.NewError(http.StatusBadRequest, "worldId 不能为空")
	}

	f := parseStatsFilterParams(c)
	items, err := model.UserInputStatsByChannel(u.ID, worldID, f)
	if err != nil {
		return fiber.NewError(http.StatusInternalServerError, err.Error())
	}
	if !canViewAllInputStatsMeta(u.ID) && len(items) > 0 {
		visibleChannelIDs, err := service.ChannelIdListByWorld(u.ID, worldID, false)
		if err != nil {
			return fiber.NewError(http.StatusInternalServerError, err.Error())
		}
		visibleSet := make(map[string]struct{}, len(visibleChannelIDs))
		for _, channelID := range visibleChannelIDs {
			visibleSet[channelID] = struct{}{}
		}
		for i := range items {
			if _, ok := visibleSet[items[i].ChannelID]; !ok {
				items[i].ChannelName = "未知频道"
			}
		}
	}

	return c.Status(http.StatusOK).JSON(items)
}

// UserInputStatsTimeline 时间线数据（曲线图）
func UserInputStatsTimeline(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return fiber.NewError(http.StatusUnauthorized, "未登录")
	}

	f := parseStatsFilterParams(c)
	granularity := strings.TrimSpace(c.Query("granularity"))
	if granularity != "hour" {
		granularity = "day"
	}

	points, err := model.UserInputStatsTimeline(u.ID, f, granularity)
	if err != nil {
		return fiber.NewError(http.StatusInternalServerError, err.Error())
	}

	return c.Status(http.StatusOK).JSON(points)
}

// UserInputStatsSessions 团次分析用消息列表
func UserInputStatsSessions(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return fiber.NewError(http.StatusUnauthorized, "未登录")
	}

	f := parseStatsFilterParams(c)
	msgs, err := model.UserInputStatsSessionMessages(u.ID, f)
	if err != nil {
		return fiber.NewError(http.StatusInternalServerError, err.Error())
	}

	return c.Status(http.StatusOK).JSON(msgs)
}
