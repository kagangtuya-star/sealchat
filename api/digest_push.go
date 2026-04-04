package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

type digestPushSettingsDTO struct {
	Enabled                      bool                      `json:"enabled"`
	ScopeType                    string                    `json:"scopeType"`
	ScopeID                      string                    `json:"scopeId"`
	WindowSeconds                int                       `json:"windowSeconds"`
	SupportedWindowSeconds       []int                     `json:"supportedWindowSeconds"`
	ActiveUserThresholdMode      string                    `json:"activeUserThresholdMode"`
	ActiveUserThresholdValue     int                       `json:"activeUserThresholdValue"`
	EffectiveActiveUserThreshold int                       `json:"effectiveActiveUserThreshold"`
	PushMode                     string                    `json:"pushMode"`
	SelectedChannelIDs           []string                  `json:"selectedChannelIds"`
	TextTemplate                 string                    `json:"textTemplate"`
	JSONTemplate                 string                    `json:"jsonTemplate"`
	ActiveWebhookURL             string                    `json:"activeWebhookUrl"`
	ActiveWebhookMethod          string                    `json:"activeWebhookMethod"`
	ActiveWebhookHeaders         string                    `json:"activeWebhookHeaders"`
	HasSigningSecret             bool                      `json:"hasSigningSecret"`
	PassivePullPath              string                    `json:"passivePullPath"`
	PassiveLatestPath            string                    `json:"passiveLatestPath"`
	AvailableChannels            []*digestChannelOptionDTO `json:"availableChannels"`
}

type digestPushUpsertDTO struct {
	Enabled                  bool     `json:"enabled"`
	WindowSeconds            int      `json:"windowSeconds"`
	ActiveUserThresholdMode  string   `json:"activeUserThresholdMode"`
	ActiveUserThresholdValue int      `json:"activeUserThresholdValue"`
	PushMode                 string   `json:"pushMode"`
	SelectedChannelIDs       []string `json:"selectedChannelIds"`
	TextTemplate             string   `json:"textTemplate"`
	JSONTemplate             string   `json:"jsonTemplate"`
	ActiveWebhookURL         string   `json:"activeWebhookUrl"`
	ActiveWebhookMethod      string   `json:"activeWebhookMethod"`
	ActiveWebhookHeaders     string   `json:"activeWebhookHeaders"`
	SigningSecret            string   `json:"signingSecret"`
	ClearSigningSecret       bool     `json:"clearSigningSecret"`
}

type digestPushTestDTO struct {
	digestPushUpsertDTO
	WindowStart   int64 `json:"windowStart"`
	FromTime      int64 `json:"fromTime"`
	ToTime        int64 `json:"toTime"`
	DeliverActive bool  `json:"deliverActive"`
}

type digestRecordDTO struct {
	ID                 string   `json:"id"`
	RuleID             string   `json:"ruleId"`
	ScopeType          string   `json:"scopeType"`
	ScopeID            string   `json:"scopeId"`
	WindowSeconds      int      `json:"windowSeconds"`
	WindowStart        int64    `json:"windowStart"`
	WindowEnd          int64    `json:"windowEnd"`
	MessageCount       int      `json:"messageCount"`
	ActiveUserCount    int      `json:"activeUserCount"`
	SpeakerNames       []string `json:"speakerNames"`
	SpeakerSummary     string   `json:"speakerSummary"`
	RenderedText       string   `json:"renderedText"`
	RenderedJSON       string   `json:"renderedJson"`
	RenderedJSONObject any      `json:"renderedJsonObject,omitempty"`
	Status             string   `json:"status"`
	GeneratedAt        int64    `json:"generatedAt"`
	TriggeredBy        string   `json:"triggeredBy"`
	DeliveryAttempts   int      `json:"deliveryAttempts"`
}

type digestChannelOptionDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type digestWebhookIntegrationDTO struct {
	ID                string   `json:"id"`
	ScopeType         string   `json:"scopeType"`
	ScopeID           string   `json:"scopeId"`
	Name              string   `json:"name"`
	Source            string   `json:"source"`
	BotUserID         string   `json:"botUserId"`
	Status            string   `json:"status"`
	CreatedAt         int64    `json:"createdAt"`
	CreatedBy         string   `json:"createdBy"`
	LastUsedAt        int64    `json:"lastUsedAt"`
	TokenTailFragment string   `json:"tokenTailFragment"`
	Capabilities      []string `json:"capabilities"`
}

func DigestPushSettingsGet(c *fiber.Ctx) error {
	channel, err := requireDigestPushManageChannel(c)
	if err != nil || channel == nil {
		return err
	}
	rule, err := model.DigestPushRuleGet(model.DigestScopeTypeChannel, channel.ID)
	if err != nil {
		return wrapError(c, err, "读取未读提醒配置失败")
	}
	resp, err := buildDigestPushSettingsResponse(model.DigestScopeTypeChannel, channel.ID, rule)
	if err != nil {
		return wrapError(c, err, "构建未读提醒配置失败")
	}
	return c.JSON(resp)
}

func DigestPushSettingsUpsert(c *fiber.Ctx) error {
	channel, err := requireDigestPushManageChannel(c)
	if err != nil || channel == nil {
		return err
	}
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var body digestPushUpsertDTO
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	existing, err := model.DigestPushRuleGet(model.DigestScopeTypeChannel, channel.ID)
	if err != nil {
		return wrapError(c, err, "读取原配置失败")
	}
	rule := service.NewDefaultDigestRule(model.DigestScopeTypeChannel, channel.ID)
	if existing != nil {
		*rule = *existing
	}
	applyDigestPushBody(rule, &body)
	if err := service.NormalizeDigestRule(rule); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	saved, err := model.DigestPushRuleUpsert(model.DigestScopeTypeChannel, channel.ID, model.DigestPushRuleUpsertParams{
		Enabled:                  rule.Enabled,
		WindowSeconds:            rule.WindowSeconds,
		ActiveUserThresholdMode:  rule.ActiveUserThresholdMode,
		ActiveUserThresholdValue: rule.ActiveUserThresholdValue,
		PushMode:                 rule.PushMode,
		SelectedChannelIDsJSON:   rule.SelectedChannelIDsJSON,
		TextTemplate:             rule.TextTemplate,
		JSONTemplate:             rule.JSONTemplate,
		ActiveWebhookURL:         strings.TrimSpace(rule.ActiveWebhookURL),
		ActiveWebhookMethod:      rule.ActiveWebhookMethod,
		ActiveWebhookHeaders:     rule.ActiveWebhookHeaders,
		SigningSecret:            strings.TrimSpace(rule.SigningSecret),
		ActorUserID:              user.ID,
	})
	if err != nil {
		return wrapError(c, err, "保存未读提醒配置失败")
	}
	if shouldResetDigestLastProcessed(existing, saved) {
		if err := model.DigestPushRuleUpdateLastProcessed(saved.ID, 0); err != nil {
			return wrapError(c, err, "重置摘要窗口状态失败")
		}
		saved.LastProcessedWindowStart = 0
	}
	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeChannel, channel.ID, saved, user.ID); err != nil {
		return wrapError(c, err, "同步摘要拉取 BOT 失败")
	}
	resp, err := buildDigestPushSettingsResponse(model.DigestScopeTypeChannel, channel.ID, saved)
	if err != nil {
		return wrapError(c, err, "构建未读提醒配置失败")
	}
	return c.JSON(resp)
}

func DigestPushSettingsDelete(c *fiber.Ctx) error {
	channel, err := requireDigestPushManageChannel(c)
	if err != nil || channel == nil {
		return err
	}
	user := getCurUser(c)
	actorUserID := ""
	if user != nil {
		actorUserID = user.ID
	}
	if err := model.DigestPushRuleDelete(model.DigestScopeTypeChannel, channel.ID); err != nil {
		return wrapError(c, err, "删除未读提醒配置失败")
	}
	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeChannel, channel.ID, nil, actorUserID); err != nil {
		return wrapError(c, err, "清理摘要拉取 BOT 失败")
	}
	return c.JSON(fiber.Map{"success": true})
}

func WorldDigestPushSettingsGet(c *fiber.Ctx) error {
	world, err := requireDigestPushManageWorld(c)
	if err != nil || world == nil {
		return err
	}
	rule, err := model.DigestPushRuleGet(model.DigestScopeTypeWorld, world.ID)
	if err != nil {
		return wrapError(c, err, "读取世界未读提醒配置失败")
	}
	resp, err := buildDigestPushSettingsResponse(model.DigestScopeTypeWorld, world.ID, rule)
	if err != nil {
		return wrapError(c, err, "构建世界未读提醒配置失败")
	}
	return c.JSON(resp)
}

func WorldDigestPushSettingsUpsert(c *fiber.Ctx) error {
	world, err := requireDigestPushManageWorld(c)
	if err != nil || world == nil {
		return err
	}
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var body digestPushUpsertDTO
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	existing, err := model.DigestPushRuleGet(model.DigestScopeTypeWorld, world.ID)
	if err != nil {
		return wrapError(c, err, "读取原配置失败")
	}
	rule := service.NewDefaultDigestRule(model.DigestScopeTypeWorld, world.ID)
	if existing != nil {
		*rule = *existing
	}
	applyDigestPushBody(rule, &body)
	if err := service.NormalizeDigestRule(rule); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	saved, err := model.DigestPushRuleUpsert(model.DigestScopeTypeWorld, world.ID, model.DigestPushRuleUpsertParams{
		Enabled:                  rule.Enabled,
		WindowSeconds:            rule.WindowSeconds,
		ActiveUserThresholdMode:  rule.ActiveUserThresholdMode,
		ActiveUserThresholdValue: rule.ActiveUserThresholdValue,
		PushMode:                 rule.PushMode,
		SelectedChannelIDsJSON:   rule.SelectedChannelIDsJSON,
		TextTemplate:             rule.TextTemplate,
		JSONTemplate:             rule.JSONTemplate,
		ActiveWebhookURL:         strings.TrimSpace(rule.ActiveWebhookURL),
		ActiveWebhookMethod:      rule.ActiveWebhookMethod,
		ActiveWebhookHeaders:     rule.ActiveWebhookHeaders,
		SigningSecret:            strings.TrimSpace(rule.SigningSecret),
		ActorUserID:              user.ID,
	})
	if err != nil {
		return wrapError(c, err, "保存世界未读提醒配置失败")
	}
	if shouldResetDigestLastProcessed(existing, saved) {
		if err := model.DigestPushRuleUpdateLastProcessed(saved.ID, 0); err != nil {
			return wrapError(c, err, "重置摘要窗口状态失败")
		}
		saved.LastProcessedWindowStart = 0
	}
	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeWorld, world.ID, saved, user.ID); err != nil {
		return wrapError(c, err, "同步世界摘要拉取 BOT 失败")
	}
	resp, err := buildDigestPushSettingsResponse(model.DigestScopeTypeWorld, world.ID, saved)
	if err != nil {
		return wrapError(c, err, "构建世界未读提醒配置失败")
	}
	return c.JSON(resp)
}

func WorldDigestPushSettingsDelete(c *fiber.Ctx) error {
	world, err := requireDigestPushManageWorld(c)
	if err != nil || world == nil {
		return err
	}
	user := getCurUser(c)
	actorUserID := ""
	if user != nil {
		actorUserID = user.ID
	}
	if err := model.DigestPushRuleDelete(model.DigestScopeTypeWorld, world.ID); err != nil {
		return wrapError(c, err, "删除世界未读提醒配置失败")
	}
	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeWorld, world.ID, nil, actorUserID); err != nil {
		return wrapError(c, err, "清理世界摘要拉取 BOT 失败")
	}
	return c.JSON(fiber.Map{"success": true})
}

func DigestPushTest(c *fiber.Ctx) error {
	channel, err := requireDigestPushManageChannel(c)
	if err != nil || channel == nil {
		return err
	}
	current, err := model.DigestPushRuleGet(model.DigestScopeTypeChannel, channel.ID)
	if err != nil {
		return wrapError(c, err, "读取未读提醒配置失败")
	}
	rule := service.NewDefaultDigestRule(model.DigestScopeTypeChannel, channel.ID)
	if current != nil {
		*rule = *current
	}
	var body digestPushTestDTO
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	applyDigestPushBody(rule, &body.digestPushUpsertDTO)
	if err := service.NormalizeDigestRule(rule); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	windowStart, err := resolveDigestTestWindowStart(&body, rule.WindowSeconds)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	preview, record, delivery, testErr := service.TriggerDigestTest(rule, windowStart, body.DeliverActive)
	if testErr != nil {
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{
			"message":  testErr.Error(),
			"preview":  preview,
			"item":     buildDigestRecordDTO(record),
			"delivery": delivery,
		})
	}
	return c.JSON(fiber.Map{
		"preview":  preview,
		"item":     buildDigestRecordDTO(record),
		"delivery": delivery,
	})
}

func WorldDigestPushTest(c *fiber.Ctx) error {
	world, err := requireDigestPushManageWorld(c)
	if err != nil || world == nil {
		return err
	}
	current, err := model.DigestPushRuleGet(model.DigestScopeTypeWorld, world.ID)
	if err != nil {
		return wrapError(c, err, "读取世界未读提醒配置失败")
	}
	rule := service.NewDefaultDigestRule(model.DigestScopeTypeWorld, world.ID)
	if current != nil {
		*rule = *current
	}
	var body digestPushTestDTO
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	applyDigestPushBody(rule, &body.digestPushUpsertDTO)
	if err := service.NormalizeDigestRule(rule); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	windowStart, err := resolveDigestTestWindowStart(&body, rule.WindowSeconds)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	}
	preview, record, delivery, testErr := service.TriggerDigestTest(rule, windowStart, body.DeliverActive)
	if testErr != nil {
		return c.Status(http.StatusBadGateway).JSON(fiber.Map{
			"message":  testErr.Error(),
			"preview":  preview,
			"item":     buildDigestRecordDTO(record),
			"delivery": delivery,
		})
	}
	return c.JSON(fiber.Map{
		"preview":  preview,
		"item":     buildDigestRecordDTO(record),
		"delivery": delivery,
	})
}

func WebhookDigestList(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "bad_request", "message": "缺少频道ID"})
	}
	integration, err := requireWebhookCapability(c, "read_digest")
	if err != nil {
		return nil
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return wrapError(c, err, "读取频道失败")
	}
	if channel == nil || channel.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not_found", "message": "频道不存在"})
	}
	cursor := int64(0)
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		cursor, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cursor < 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "bad_request", "message": "cursor 解析失败"})
		}
	}
	limit := c.QueryInt("limit", 30)
	if limit <= 0 {
		limit = 30
	}
	if limit > 30 {
		limit = 30
	}
	items, err := model.DigestRecordList(model.DigestScopeTypeChannel, channelID, cursor, limit)
	if err != nil {
		return wrapError(c, err, "读取摘要记录失败")
	}
	respItems := make([]*digestRecordDTO, 0, len(items))
	nextCursor := cursor
	for _, item := range items {
		dto := buildDigestRecordDTO(item)
		if dto == nil {
			continue
		}
		respItems = append(respItems, dto)
		if item.GeneratedAt > 0 {
			nextCursor = item.GeneratedAt
		}
	}
	return c.JSON(fiber.Map{
		"channelId":  channelID,
		"cursor":     strconv.FormatInt(cursor, 10),
		"nextCursor": strconv.FormatInt(nextCursor, 10),
		"items":      respItems,
		"integration": fiber.Map{
			"id":     integration.ID,
			"source": integration.Source,
		},
	})
}

func WebhookDigestLatest(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "bad_request", "message": "缺少频道ID"})
	}
	integration, err := requireWebhookCapability(c, "read_digest")
	if err != nil {
		return nil
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return wrapError(c, err, "读取频道失败")
	}
	if channel == nil || channel.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not_found", "message": "频道不存在"})
	}
	item, err := model.DigestRecordLatest(model.DigestScopeTypeChannel, channelID)
	if err != nil {
		return wrapError(c, err, "读取摘要记录失败")
	}
	return c.JSON(fiber.Map{
		"channelId": channelID,
		"item":      buildDigestRecordDTO(item),
		"integration": fiber.Map{
			"id":     integration.ID,
			"source": integration.Source,
		},
	})
}

func WebhookWorldDigestList(c *fiber.Ctx) error {
	worldID := strings.TrimSpace(c.Params("worldId"))
	if worldID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "bad_request", "message": "缺少世界ID"})
	}
	integration, err := requireDigestWebhookScope(c, model.DigestScopeTypeWorld, worldID)
	if err != nil {
		return nil
	}
	world, err := service.GetWorldByID(worldID)
	if err != nil {
		return wrapError(c, err, "读取世界失败")
	}
	if world == nil || world.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not_found", "message": "世界不存在"})
	}
	cursor := int64(0)
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		cursor, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cursor < 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "bad_request", "message": "cursor 解析失败"})
		}
	}
	limit := c.QueryInt("limit", 30)
	if limit <= 0 {
		limit = 30
	}
	if limit > 30 {
		limit = 30
	}
	items, err := model.DigestRecordList(model.DigestScopeTypeWorld, worldID, cursor, limit)
	if err != nil {
		return wrapError(c, err, "读取世界摘要记录失败")
	}
	respItems := make([]*digestRecordDTO, 0, len(items))
	nextCursor := cursor
	for _, item := range items {
		dto := buildDigestRecordDTO(item)
		if dto == nil {
			continue
		}
		respItems = append(respItems, dto)
		if item.GeneratedAt > 0 {
			nextCursor = item.GeneratedAt
		}
	}
	return c.JSON(fiber.Map{
		"worldId":    worldID,
		"cursor":     strconv.FormatInt(cursor, 10),
		"nextCursor": strconv.FormatInt(nextCursor, 10),
		"items":      respItems,
		"integration": fiber.Map{
			"id":     integration.ID,
			"source": integration.Source,
		},
	})
}

func WebhookWorldDigestLatest(c *fiber.Ctx) error {
	worldID := strings.TrimSpace(c.Params("worldId"))
	if worldID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "bad_request", "message": "缺少世界ID"})
	}
	integration, err := requireDigestWebhookScope(c, model.DigestScopeTypeWorld, worldID)
	if err != nil {
		return nil
	}
	world, err := service.GetWorldByID(worldID)
	if err != nil {
		return wrapError(c, err, "读取世界失败")
	}
	if world == nil || world.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "not_found", "message": "世界不存在"})
	}
	item, err := model.DigestRecordLatest(model.DigestScopeTypeWorld, worldID)
	if err != nil {
		return wrapError(c, err, "读取世界摘要记录失败")
	}
	return c.JSON(fiber.Map{
		"worldId": worldID,
		"item":    buildDigestRecordDTO(item),
		"integration": fiber.Map{
			"id":     integration.ID,
			"source": integration.Source,
		},
	})
}

func requireDigestPushManageChannel(c *fiber.Ctx) (*model.ChannelModel, error) {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return nil, c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}
	if !CanWithChannelRole(c, channelID, pm.PermFuncChannelManageInfo) {
		return nil, nil
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, wrapError(c, err, "读取频道失败")
	}
	if channel == nil || channel.ID == "" {
		return nil, c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
	}
	if strings.EqualFold(channel.PermType, "private") {
		return nil, c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "私聊频道暂不支持未读提醒"})
	}
	return channel, nil
}

func requireDigestPushManageWorld(c *fiber.Ctx) (*model.WorldModel, error) {
	worldID := strings.TrimSpace(c.Params("worldId"))
	if worldID == "" {
		return nil, c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少世界ID"})
	}
	user := getCurUser(c)
	if user == nil {
		return nil, c.Status(http.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	if !service.IsWorldAdmin(worldID, user.ID) && !pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return nil, c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权限管理世界未读提醒"})
	}
	world, err := service.GetWorldByID(worldID)
	if err != nil {
		return nil, wrapError(c, err, "读取世界失败")
	}
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return nil, c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
	}
	return world, nil
}

func buildDigestPushSettingsResponse(scopeType, scopeID string, rule *model.DigestPushRuleModel) (*digestPushSettingsDTO, error) {
	if rule == nil {
		rule = service.NewDefaultDigestRule(scopeType, scopeID)
	} else {
		copyRule := *rule
		rule = &copyRule
	}
	if err := service.NormalizeDigestRule(rule); err != nil {
		return nil, err
	}
	effectiveThreshold, err := service.DigestEffectiveThreshold(rule)
	if err != nil || effectiveThreshold <= 0 {
		effectiveThreshold = 1
	}
	options, err := service.DigestScopeChannelOptions(scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	respOptions := make([]*digestChannelOptionDTO, 0, len(options))
	for _, item := range options {
		respOptions = append(respOptions, &digestChannelOptionDTO{
			ID:   item.ID,
			Name: item.Name,
		})
	}
	passivePullPath := "/api/v1/webhook/channels/" + scopeID + "/digests"
	passiveLatestPath := "/api/v1/webhook/channels/" + scopeID + "/digests/latest"
	if scopeType == model.DigestScopeTypeWorld {
		passivePullPath = "/api/v1/webhook/worlds/" + scopeID + "/digests"
		passiveLatestPath = "/api/v1/webhook/worlds/" + scopeID + "/digests/latest"
	}
	return &digestPushSettingsDTO{
		Enabled:                      rule.Enabled,
		ScopeType:                    rule.ScopeType,
		ScopeID:                      rule.ScopeID,
		WindowSeconds:                rule.WindowSeconds,
		SupportedWindowSeconds:       service.DigestSupportedWindowSeconds(),
		ActiveUserThresholdMode:      rule.ActiveUserThresholdMode,
		ActiveUserThresholdValue:     rule.ActiveUserThresholdValue,
		EffectiveActiveUserThreshold: effectiveThreshold,
		PushMode:                     rule.PushMode,
		SelectedChannelIDs:           service.DigestRuleSelectedChannelIDs(rule),
		TextTemplate:                 rule.TextTemplate,
		JSONTemplate:                 rule.JSONTemplate,
		ActiveWebhookURL:             rule.ActiveWebhookURL,
		ActiveWebhookMethod:          rule.ActiveWebhookMethod,
		ActiveWebhookHeaders:         rule.ActiveWebhookHeaders,
		HasSigningSecret:             strings.TrimSpace(rule.SigningSecret) != "",
		PassivePullPath:              passivePullPath,
		PassiveLatestPath:            passiveLatestPath,
		AvailableChannels:            respOptions,
	}, nil
}

func applyDigestPushBody(rule *model.DigestPushRuleModel, body *digestPushUpsertDTO) {
	if rule == nil || body == nil {
		return
	}
	rule.Enabled = body.Enabled
	rule.WindowSeconds = body.WindowSeconds
	rule.ActiveUserThresholdMode = body.ActiveUserThresholdMode
	rule.ActiveUserThresholdValue = body.ActiveUserThresholdValue
	rule.PushMode = body.PushMode
	rule.SelectedChannelIDsJSON = marshalDigestStringSlice(body.SelectedChannelIDs)
	rule.TextTemplate = body.TextTemplate
	rule.JSONTemplate = body.JSONTemplate
	rule.ActiveWebhookURL = strings.TrimSpace(body.ActiveWebhookURL)
	rule.ActiveWebhookMethod = body.ActiveWebhookMethod
	rule.ActiveWebhookHeaders = body.ActiveWebhookHeaders
	if body.ClearSigningSecret {
		rule.SigningSecret = ""
	} else if strings.TrimSpace(body.SigningSecret) != "" {
		rule.SigningSecret = body.SigningSecret
	}
}

func resolveDigestTestWindowStart(body *digestPushTestDTO, windowSeconds int) (int64, error) {
	if body == nil {
		return 0, nil
	}
	if body.WindowStart > 0 {
		return body.WindowStart, nil
	}
	if body.FromTime > 0 || body.ToTime > 0 {
		if body.FromTime <= 0 || body.ToTime <= body.FromTime {
			return 0, fiber.NewError(http.StatusBadRequest, "测试时间范围无效")
		}
		rangeSeconds := int((body.ToTime - body.FromTime) / 1000)
		if !service.IsDigestWindowSecondsSupported(rangeSeconds) {
			return 0, fiber.NewError(http.StatusBadRequest, "测试时间范围必须匹配支持的周期")
		}
		if body.WindowSeconds > 0 && service.NormalizeDigestWindowSeconds(body.WindowSeconds) != rangeSeconds {
			return 0, fiber.NewError(http.StatusBadRequest, "windowSeconds 与时间范围不一致")
		}
		return body.FromTime, nil
	}
	return service.LatestClosedDigestWindowStart(windowSeconds, time.Now()), nil
}

func shouldResetDigestLastProcessed(previous, current *model.DigestPushRuleModel) bool {
	if current == nil {
		return false
	}
	if previous == nil {
		return true
	}
	if previous.WindowSeconds != current.WindowSeconds {
		return true
	}
	if !previous.Enabled && current.Enabled {
		return true
	}
	return false
}

func buildDigestRecordDTO(item *model.DigestRecordModel) *digestRecordDTO {
	if item == nil {
		return nil
	}
	dto := &digestRecordDTO{
		ID:               item.ID,
		RuleID:           item.RuleID,
		ScopeType:        item.ScopeType,
		ScopeID:          item.ScopeID,
		WindowSeconds:    item.WindowSeconds,
		WindowStart:      item.WindowStart,
		WindowEnd:        item.WindowEnd,
		MessageCount:     item.MessageCount,
		ActiveUserCount:  item.ActiveUserCount,
		SpeakerSummary:   item.SpeakerSummary,
		RenderedText:     item.RenderedText,
		RenderedJSON:     item.RenderedJSON,
		Status:           item.Status,
		GeneratedAt:      item.GeneratedAt,
		TriggeredBy:      item.TriggeredBy,
		DeliveryAttempts: item.DeliveryAttempts,
	}
	if raw := strings.TrimSpace(item.SpeakerNames); raw != "" {
		_ = json.Unmarshal([]byte(raw), &dto.SpeakerNames)
	}
	if raw := strings.TrimSpace(item.RenderedJSON); raw != "" {
		_ = json.Unmarshal([]byte(raw), &dto.RenderedJSONObject)
	}
	return dto
}

func marshalDigestStringSlice(values []string) string {
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func buildDigestWebhookIntegrationDTO(item *model.DigestWebhookIntegrationModel) *digestWebhookIntegrationDTO {
	if item == nil {
		return nil
	}
	var createdAt int64
	if !item.CreatedAt.IsZero() {
		createdAt = item.CreatedAt.UnixMilli()
	}
	return &digestWebhookIntegrationDTO{
		ID:                item.ID,
		ScopeType:         item.ScopeType,
		ScopeID:           item.ScopeID,
		Name:              item.Name,
		Source:            item.Source,
		BotUserID:         item.BotUserID,
		Status:            item.Status,
		CreatedAt:         createdAt,
		CreatedBy:         item.CreatedBy,
		LastUsedAt:        item.LastUsedAt,
		TokenTailFragment: item.TokenTailFragment,
		Capabilities:      []string{"read_digest"},
	}
}

func requireDigestWebhookScope(c *fiber.Ctx, scopeType, scopeID string) (*model.DigestWebhookIntegrationModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil, c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error":   "bad_request",
			"message": "missing digest scope",
		})
	}
	token := getAuthorizationToken(c)
	if len(token) != 32 {
		return nil, c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": "token invalid",
		})
	}
	user, err := model.BotVerifyAccessToken(token)
	if err != nil || user == nil {
		msg := "token invalid"
		if err != nil && strings.TrimSpace(err.Error()) != "" {
			msg = err.Error()
		}
		return nil, c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"error":   "unauthorized",
			"message": msg,
		})
	}
	integration, err := model.DigestWebhookIntegrationGetByScopeAndBot(scopeType, scopeID, user.ID)
	if err != nil || integration == nil {
		return nil, c.Status(http.StatusForbidden).JSON(fiber.Map{
			"error":   "forbidden",
			"message": "integration not found or revoked",
		})
	}
	now := time.Now()
	_ = model.DigestWebhookIntegrationTouchUsage(scopeType, scopeID, user.ID, now)
	_ = model.GetDB().Model(&model.BotTokenModel{}).
		Where("id = ?", user.ID).
		Update("recent_used_at", now.UnixMilli()).Error
	return integration, nil
}

func WorldDigestIntegrationList(c *fiber.Ctx) error {
	world, err := requireDigestPushManageWorld(c)
	if err != nil || world == nil {
		return err
	}
	items, err := model.DigestWebhookIntegrationList(model.DigestScopeTypeWorld, world.ID)
	if err != nil {
		return wrapError(c, err, "读取世界摘要拉取授权失败")
	}
	botIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.BotUserID) == "" {
			continue
		}
		botIDs = append(botIDs, item.BotUserID)
	}
	tailByBotID := map[string]string{}
	if len(botIDs) > 0 {
		var tokens []model.BotTokenModel
		model.GetDB().Select("id, token").Where("id IN ?", botIDs).Find(&tokens)
		for _, token := range tokens {
			trimmed := strings.TrimSpace(token.Token)
			if len(trimmed) >= 6 {
				tailByBotID[token.ID] = trimmed[len(trimmed)-6:]
			} else {
				tailByBotID[token.ID] = trimmed
			}
		}
	}
	out := make([]*digestWebhookIntegrationDTO, 0, len(items))
	for _, item := range items {
		dto := buildDigestWebhookIntegrationDTO(item)
		if dto == nil {
			continue
		}
		if tail, ok := tailByBotID[dto.BotUserID]; ok {
			dto.TokenTailFragment = tail
		}
		out = append(out, dto)
	}
	return c.JSON(fiber.Map{"items": out})
}

func WorldDigestIntegrationCreate(c *fiber.Ctx) error {
	world, err := requireDigestPushManageWorld(c)
	if err != nil || world == nil {
		return err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "世界摘要拉取"
	}
	uid := utils.NewID()
	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: uid},
		Username:          utils.NewID(),
		Nickname:          name,
		Password:          "",
		Salt:              "BOT_SALT",
		IsBot:             true,
		BotKind:           model.BotKindDigestPull,
	}
	db := model.GetDB()
	if err := db.Create(user).Error; err != nil {
		return wrapError(c, err, "创建 bot 失败")
	}
	tokenValue := utils.NewIDWithLength(32)
	token := &model.BotTokenModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: uid},
		Name:              name,
		Token:             tokenValue,
		ExpiresAt:         time.Now().UnixMilli() + 3*365*24*60*60*1e3,
	}
	if err := db.Create(token).Error; err != nil {
		return wrapError(c, err, "创建 token 失败")
	}
	_ = service.SyncBotUserProfile(token)
	createdBy := getCurUser(c)
	creatorID := ""
	if createdBy != nil {
		creatorID = createdBy.ID
	}
	integration, err := model.DigestWebhookIntegrationCreate(model.DigestScopeTypeWorld, world.ID, name, "digest-pull", user.ID, creatorID)
	if err != nil {
		return wrapError(c, err, "创建世界摘要拉取授权失败")
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"item":  buildDigestWebhookIntegrationDTO(integration),
		"token": tokenValue,
	})
}

func WorldDigestIntegrationRotate(c *fiber.Ctx) error {
	world, err := requireDigestPushManageWorld(c)
	if err != nil || world == nil {
		return err
	}
	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少授权ID"})
	}
	integration, err := model.DigestWebhookIntegrationGetByID(model.DigestScopeTypeWorld, world.ID, id)
	if err != nil {
		return wrapError(c, err, "读取世界摘要拉取授权失败")
	}
	if integration == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "授权不存在"})
	}
	if integration.Status != model.WebhookIntegrationStatusActive {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "授权已撤销"})
	}
	newToken := utils.NewIDWithLength(32)
	if err := model.GetDB().Model(&model.BotTokenModel{}).
		Where("id = ?", integration.BotUserID).
		Updates(map[string]any{
			"token":          newToken,
			"expires_at":     time.Now().UnixMilli() + 3*365*24*60*60*1e3,
			"recent_used_at": 0,
		}).Error; err != nil {
		return wrapError(c, err, "轮换 token 失败")
	}
	if len(newToken) >= 6 {
		_ = model.GetDB().Model(&model.DigestWebhookIntegrationModel{}).
			Where("id = ?", integration.ID).
			Update("token_tail_fragment", newToken[len(newToken)-6:]).Error
	}
	return c.JSON(fiber.Map{"token": newToken})
}

func WorldDigestIntegrationRevoke(c *fiber.Ctx) error {
	world, err := requireDigestPushManageWorld(c)
	if err != nil || world == nil {
		return err
	}
	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少授权ID"})
	}
	integration, err := model.DigestWebhookIntegrationGetByID(model.DigestScopeTypeWorld, world.ID, id)
	if err != nil {
		return wrapError(c, err, "读取世界摘要拉取授权失败")
	}
	if integration == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "授权不存在"})
	}

	tx := model.GetDB().Begin()
	if tx.Error != nil {
		return wrapError(c, tx.Error, "撤销世界摘要拉取授权失败")
	}
	rollback := func(err error) error {
		tx.Rollback()
		return wrapError(c, err, "撤销世界摘要拉取授权失败")
	}

	if err := tx.Model(&model.DigestWebhookIntegrationModel{}).
		Where("id = ?", integration.ID).
		Update("status", model.WebhookIntegrationStatusRevoked).Error; err != nil {
		return rollback(err)
	}
	if err := tx.Model(&model.BotTokenModel{}).
		Where("id = ?", integration.BotUserID).
		Update("expires_at", int64(0)).Error; err != nil {
		return rollback(err)
	}
	if _, err := model.CleanupOrphanSystemBotByUserIDTx(tx, integration.BotUserID); err != nil {
		return rollback(err)
	}
	if err := tx.Commit().Error; err != nil {
		return wrapError(c, err, "撤销世界摘要拉取授权失败")
	}

	return c.JSON(fiber.Map{"success": true})
}
