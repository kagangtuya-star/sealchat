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
)

type digestPushSettingsDTO struct {
	Enabled                      bool   `json:"enabled"`
	ScopeType                    string `json:"scopeType"`
	ScopeID                      string `json:"scopeId"`
	WindowSeconds                int    `json:"windowSeconds"`
	SupportedWindowSeconds       []int  `json:"supportedWindowSeconds"`
	ActiveUserThresholdMode      string `json:"activeUserThresholdMode"`
	ActiveUserThresholdValue     int    `json:"activeUserThresholdValue"`
	EffectiveActiveUserThreshold int    `json:"effectiveActiveUserThreshold"`
	PushMode                     string `json:"pushMode"`
	TextTemplate                 string `json:"textTemplate"`
	JSONTemplate                 string `json:"jsonTemplate"`
	ActiveWebhookURL             string `json:"activeWebhookUrl"`
	ActiveWebhookMethod          string `json:"activeWebhookMethod"`
	ActiveWebhookHeaders         string `json:"activeWebhookHeaders"`
	HasSigningSecret             bool   `json:"hasSigningSecret"`
	PassivePullPath              string `json:"passivePullPath"`
	PassiveLatestPath            string `json:"passiveLatestPath"`
}

type digestPushUpsertDTO struct {
	Enabled                  bool   `json:"enabled"`
	WindowSeconds            int    `json:"windowSeconds"`
	ActiveUserThresholdMode  string `json:"activeUserThresholdMode"`
	ActiveUserThresholdValue int    `json:"activeUserThresholdValue"`
	PushMode                 string `json:"pushMode"`
	TextTemplate             string `json:"textTemplate"`
	JSONTemplate             string `json:"jsonTemplate"`
	ActiveWebhookURL         string `json:"activeWebhookUrl"`
	ActiveWebhookMethod      string `json:"activeWebhookMethod"`
	ActiveWebhookHeaders     string `json:"activeWebhookHeaders"`
	SigningSecret            string `json:"signingSecret"`
	ClearSigningSecret       bool   `json:"clearSigningSecret"`
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

func DigestPushSettingsGet(c *fiber.Ctx) error {
	channel, err := requireDigestPushManageChannel(c)
	if err != nil || channel == nil {
		return err
	}
	rule, err := model.DigestPushRuleGet(model.DigestScopeTypeChannel, channel.ID)
	if err != nil {
		return wrapError(c, err, "读取未读提醒配置失败")
	}
	return c.JSON(buildDigestPushSettingsResponse(channel.ID, rule))
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
	return c.JSON(buildDigestPushSettingsResponse(channel.ID, saved))
}

func DigestPushSettingsDelete(c *fiber.Ctx) error {
	channel, err := requireDigestPushManageChannel(c)
	if err != nil || channel == nil {
		return err
	}
	if err := model.DigestPushRuleDelete(model.DigestScopeTypeChannel, channel.ID); err != nil {
		return wrapError(c, err, "删除未读提醒配置失败")
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

func buildDigestPushSettingsResponse(channelID string, rule *model.DigestPushRuleModel) *digestPushSettingsDTO {
	if rule == nil {
		rule = service.NewDefaultDigestRule(model.DigestScopeTypeChannel, channelID)
	} else {
		copyRule := *rule
		rule = &copyRule
	}
	_ = service.NormalizeDigestRule(rule)
	effectiveThreshold := 1
	switch rule.ActiveUserThresholdMode {
	case model.DigestThresholdModeFixed:
		if rule.ActiveUserThresholdValue > 0 {
			effectiveThreshold = rule.ActiveUserThresholdValue
		}
	default:
		if threshold, err := service.ChannelMemberCount(channelID); err == nil && threshold > 0 {
			effectiveThreshold = threshold
		}
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
		TextTemplate:                 rule.TextTemplate,
		JSONTemplate:                 rule.JSONTemplate,
		ActiveWebhookURL:             rule.ActiveWebhookURL,
		ActiveWebhookMethod:          rule.ActiveWebhookMethod,
		ActiveWebhookHeaders:         rule.ActiveWebhookHeaders,
		HasSigningSecret:             strings.TrimSpace(rule.SigningSecret) != "",
		PassivePullPath:              "/api/v1/webhook/channels/" + channelID + "/digests",
		PassiveLatestPath:            "/api/v1/webhook/channels/" + channelID + "/digests/latest",
	}
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
