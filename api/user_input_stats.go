package api

import (
	"errors"
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
	if raw := strings.TrimSpace(c.Query("includeImported")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			f.IncludeImported = parsed
		}
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

// UserInputStatsHeroes 打字英雄榜：返回自己的成绩与附近排名（服务端完成排名与匿名化）
func UserInputStatsHeroes(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return fiber.NewError(http.StatusUnauthorized, "未登录")
	}

	q := service.TypingHeroesQuery{
		ViewerID:   u.ID,
		Period:     service.TypingHeroesPeriodKind(strings.TrimSpace(c.Query("period", "week"))),
		Metric:     service.TypingHeroesMetric(strings.TrimSpace(c.Query("metric", "chars"))),
		Subject:    model.InputStatsPeriodSubject(strings.TrimSpace(c.Query("subject", "user"))),
		IdentityID: strings.TrimSpace(c.Query("identityId")),
		Timezone:   strings.TrimSpace(c.Query("timezone")),
	}
	var err error
	if q.Offset, err = strconv.Atoi(strings.TrimSpace(c.Query("offset", "0"))); err != nil {
		return wrapErrorStatus(c, http.StatusBadRequest, nil, "offset 无效")
	}
	if q.TZOffsetMinutes, err = strconv.Atoi(strings.TrimSpace(c.Query("tzOffset", "0"))); err != nil {
		return wrapErrorStatus(c, http.StatusBadRequest, nil, "tzOffset 无效")
	}

	board, err := service.TypingHeroesBoardGet(q)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTypingHeroesInvalidInput):
			return wrapErrorStatus(c, http.StatusBadRequest, nil, "排行榜参数无效")
		case errors.Is(err, service.ErrTypingHeroesIdentityUnavailable):
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "该角色当前不可用于排行榜", "code": "identity_unavailable"})
		case errors.Is(err, service.ErrTypingHeroesBusy):
			return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{"message": "排行榜请求过于频繁，请稍后重试", "code": "rate_limited"})
		}
		return wrapErrorStatus(c, http.StatusInternalServerError, nil, "获取打字英雄榜失败")
	}
	return c.Status(http.StatusOK).JSON(board)
}

// UserInputStatsHeroesIdentities 打字英雄榜可选的频道角色
func UserInputStatsHeroesIdentities(c *fiber.Ctx) error {
	u := getCurUser(c)
	if u == nil {
		return fiber.NewError(http.StatusUnauthorized, "未登录")
	}
	items, err := service.TypingHeroesIdentityOptions(u.ID)
	if err != nil {
		return wrapErrorStatus(c, http.StatusInternalServerError, nil, "获取可用角色失败")
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"items": items})
}
