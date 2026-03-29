package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sealchat/model"
)

const (
	DigestDefaultWindowSeconds = 3600
	DigestActivePushTimeout    = 10 * time.Second
	DigestWorkerInterval       = 60 * time.Second
	DigestRecordRetention      = 30
	DigestWindowRetention      = 30
	DigestActivePushRetryCount = 3
)

var (
	digestSupportedWindowSeconds = []int{300, 900, 1800, 3600, 7200, 21600, 86400}
	digestPushWorkerOnce         sync.Once
	digestHTTPClient             = &http.Client{Timeout: DigestActivePushTimeout}
)

const defaultDigestTextTemplate = "在 {{window_label}}，{{speaker_names}} 在 {{channel_name}} 频道发送了 {{message_count}} 条消息。"

const defaultWorldDigestTextTemplate = "在 {{window_label}}，{{scope_name}} 有 {{channel_count}} 个频道出现新消息：\n{{channel_digest_lines}}"

const defaultDigestJSONTemplate = `{
  "scopeType": {{scope_type}},
  "scopeId": {{scope_id}},
  "window": {
    "start": {{window_start_ts}},
    "end": {{window_end_ts}},
    "label": {{window_label}},
    "seconds": {{window_seconds}}
  },
  "channel": {
    "id": {{channel_id}},
    "name": {{channel_name}}
  },
  "world": {
    "id": {{world_id}},
    "name": {{world_name}}
  },
  "messageCount": {{message_count}},
  "activeUserCount": {{active_user_count}},
  "speakerNames": {{speaker_names_array}},
  "speakerSummary": {{speaker_summary}},
  "speakers": {{speakers}},
  "text": {{rendered_text}}
}`

const defaultWorldDigestJSONTemplate = `{
  "scopeType": {{scope_type}},
  "scopeId": {{scope_id}},
  "window": {
    "start": {{window_start_ts}},
    "end": {{window_end_ts}},
    "label": {{window_label}},
    "seconds": {{window_seconds}}
  },
  "world": {
    "id": {{world_id}},
    "name": {{world_name}}
  },
  "channelCount": {{channel_count}},
  "targetChannelIds": {{target_channel_ids}},
  "targetChannelNames": {{target_channel_names_array}},
  "channels": {{channels}},
  "messageCount": {{message_count}},
  "activeUserCount": {{active_user_count}},
  "speakerNames": {{speaker_names_array}},
  "speakerSummary": {{speaker_summary}},
  "speakers": {{speakers}},
  "text": {{rendered_text}}
}`

type DigestSpeaker struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	MessageCount int    `json:"messageCount"`
}

type DigestChannelSummary struct {
	ChannelID       string          `json:"channelId"`
	ChannelName     string          `json:"channelName"`
	MessageCount    int             `json:"messageCount"`
	ActiveUserCount int             `json:"activeUserCount"`
	SpeakerNames    []string        `json:"speakerNames"`
	SpeakerSummary  string          `json:"speakerSummary"`
	Speakers        []DigestSpeaker `json:"speakers"`
	Text            string          `json:"text"`
}

type DigestScopeChannelOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DigestPreview struct {
	ScopeType          string                 `json:"scopeType"`
	ScopeID            string                 `json:"scopeId"`
	ScopeName          string                 `json:"scopeName"`
	ChannelID          string                 `json:"channelId"`
	ChannelName        string                 `json:"channelName"`
	WorldID            string                 `json:"worldId"`
	WorldName          string                 `json:"worldName"`
	WindowSeconds      int                    `json:"windowSeconds"`
	WindowStart        int64                  `json:"windowStart"`
	WindowEnd          int64                  `json:"windowEnd"`
	WindowLabel        string                 `json:"windowLabel"`
	MessageCount       int                    `json:"messageCount"`
	ActiveUserCount    int                    `json:"activeUserCount"`
	ThresholdMode      string                 `json:"thresholdMode"`
	ThresholdValue     int                    `json:"thresholdValue"`
	ThresholdSatisfied bool                   `json:"thresholdSatisfied"`
	SpeakerNames       []string               `json:"speakerNames"`
	SpeakerSummary     string                 `json:"speakerSummary"`
	Speakers           []DigestSpeaker        `json:"speakers"`
	ChannelCount       int                    `json:"channelCount"`
	TargetChannelIDs   []string               `json:"targetChannelIds"`
	TargetChannelNames []string               `json:"targetChannelNames"`
	ChannelDigestLines string                 `json:"channelDigestLines"`
	Channels           []DigestChannelSummary `json:"channels"`
	RenderedText       string                 `json:"renderedText"`
	RenderedJSON       string                 `json:"renderedJson"`
	RenderedJSONObject any                    `json:"renderedJsonObject,omitempty"`
}

type DigestDeliveryResult struct {
	TargetURL      string `json:"targetUrl"`
	StatusCode     int    `json:"statusCode"`
	Success        bool   `json:"success"`
	ErrorText      string `json:"errorText,omitempty"`
	ResponseBody   string `json:"responseBody,omitempty"`
	ResponseTimeMs int64  `json:"responseTimeMs"`
}

func DigestSupportedWindowSeconds() []int {
	out := make([]int, len(digestSupportedWindowSeconds))
	copy(out, digestSupportedWindowSeconds)
	return out
}

func IsDigestWindowSecondsSupported(value int) bool {
	for _, item := range digestSupportedWindowSeconds {
		if value == item {
			return true
		}
	}
	return false
}

func NormalizeDigestWindowSeconds(value int) int {
	if IsDigestWindowSecondsSupported(value) {
		return value
	}
	return DigestDefaultWindowSeconds
}

func DefaultDigestTextTemplate() string {
	return defaultDigestTextTemplate
}

func DefaultDigestJSONTemplate() string {
	return defaultDigestJSONTemplate
}

func DefaultDigestTextTemplateForScope(scopeType string) string {
	switch strings.TrimSpace(scopeType) {
	case model.DigestScopeTypeWorld:
		return defaultWorldDigestTextTemplate
	default:
		return defaultDigestTextTemplate
	}
}

func DefaultDigestJSONTemplateForScope(scopeType string) string {
	switch strings.TrimSpace(scopeType) {
	case model.DigestScopeTypeWorld:
		return defaultWorldDigestJSONTemplate
	default:
		return defaultDigestJSONTemplate
	}
}

func NewDefaultDigestRule(scopeType, scopeID string) *model.DigestPushRuleModel {
	return &model.DigestPushRuleModel{
		ScopeType:                strings.TrimSpace(scopeType),
		ScopeID:                  strings.TrimSpace(scopeID),
		Enabled:                  false,
		WindowSeconds:            DigestDefaultWindowSeconds,
		ActiveUserThresholdMode:  model.DigestThresholdModeChannelMemberCount,
		ActiveUserThresholdValue: 0,
		PushMode:                 model.DigestPushModePassive,
		TextTemplate:             DefaultDigestTextTemplateForScope(scopeType),
		JSONTemplate:             DefaultDigestJSONTemplateForScope(scopeType),
		ActiveWebhookMethod:      http.MethodPost,
		ActiveWebhookHeaders:     "{}",
	}
}

func AlignDigestWindow(tsMillis int64, windowSeconds int) (int64, int64) {
	windowSeconds = NormalizeDigestWindowSeconds(windowSeconds)
	if tsMillis <= 0 {
		tsMillis = time.Now().UnixMilli()
	}
	windowMillis := int64(windowSeconds) * 1000
	windowStart := (tsMillis / windowMillis) * windowMillis
	windowEnd := windowStart + windowMillis
	return windowStart, windowEnd
}

func LatestClosedDigestWindowStart(windowSeconds int, now time.Time) int64 {
	windowSeconds = NormalizeDigestWindowSeconds(windowSeconds)
	if now.IsZero() {
		now = time.Now()
	}
	start, _ := AlignDigestWindow(now.UnixMilli()-1, windowSeconds)
	return start
}

func RecordDigestWindowVisit(channelID, userID string) error {
	channelID = strings.TrimSpace(channelID)
	userID = strings.TrimSpace(userID)
	if channelID == "" || userID == "" || strings.Contains(channelID, ":") {
		return nil
	}
	nowMillis := time.Now().UnixMilli()
	for _, windowSeconds := range DigestSupportedWindowSeconds() {
		windowStart, windowEnd := AlignDigestWindow(nowMillis, windowSeconds)
		if err := model.DigestWindowVisitorUpsert(model.DigestScopeTypeChannel, channelID, windowSeconds, windowStart, windowEnd, userID); err != nil {
			return err
		}
	}
	return nil
}

func RecordDigestWindowMessage(channelID string, message *model.MessageModel) error {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" || strings.Contains(channelID, ":") || message == nil || message.ID == "" || message.IsWhisper || message.IsDeleted || message.IsRevoked {
		return nil
	}
	speakerKey := resolveDigestSpeakerKey(message)
	if speakerKey == "" {
		return nil
	}
	speakerName := ResolveDigestSpeakerDisplayName(message)
	messageAt := message.CreatedAt.UnixMilli()
	if messageAt <= 0 {
		messageAt = time.Now().UnixMilli()
	}
	for _, windowSeconds := range DigestSupportedWindowSeconds() {
		windowStart, windowEnd := AlignDigestWindow(messageAt, windowSeconds)
		if err := model.DigestWindowSpeakerUpsert(model.DigestScopeTypeChannel, channelID, windowSeconds, windowStart, windowEnd, speakerKey, speakerName, messageAt); err != nil {
			return err
		}
	}
	return nil
}

func ResolveDigestSpeakerDisplayName(message *model.MessageModel) string {
	if message == nil {
		return "未知成员"
	}
	for _, candidate := range []string{
		message.SenderIdentityName,
		message.SenderMemberName,
		func() string {
			if message.Member != nil {
				return message.Member.Nickname
			}
			return ""
		}(),
		func() string {
			if message.User != nil {
				return message.User.Nickname
			}
			return ""
		}(),
		func() string {
			if message.User != nil {
				return message.User.Username
			}
			return ""
		}(),
	} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
	}
	return "未知成员"
}

func NormalizeDigestRule(rule *model.DigestPushRuleModel) error {
	if rule == nil {
		return fmt.Errorf("摘要规则不能为空")
	}
	rule.ScopeType = strings.TrimSpace(rule.ScopeType)
	rule.ScopeID = strings.TrimSpace(rule.ScopeID)
	if rule.ScopeType == "" || rule.ScopeID == "" {
		return fmt.Errorf("缺少作用域")
	}
	switch rule.ScopeType {
	case model.DigestScopeTypeChannel, model.DigestScopeTypeWorld:
	default:
		return fmt.Errorf("暂不支持的摘要作用域: %s", rule.ScopeType)
	}
	if rule.WindowSeconds <= 0 {
		rule.WindowSeconds = DigestDefaultWindowSeconds
	} else if !IsDigestWindowSecondsSupported(rule.WindowSeconds) {
		return fmt.Errorf("不支持的事件周期")
	}
	rule.TextTemplate = strings.TrimSpace(rule.TextTemplate)
	if rule.TextTemplate == "" {
		rule.TextTemplate = DefaultDigestTextTemplateForScope(rule.ScopeType)
	}
	rule.JSONTemplate = strings.TrimSpace(rule.JSONTemplate)
	if rule.JSONTemplate == "" {
		rule.JSONTemplate = DefaultDigestJSONTemplateForScope(rule.ScopeType)
	}
	rule.ActiveUserThresholdMode = strings.TrimSpace(rule.ActiveUserThresholdMode)
	if rule.ActiveUserThresholdMode == "" {
		rule.ActiveUserThresholdMode = model.DigestThresholdModeChannelMemberCount
	}
	switch rule.ActiveUserThresholdMode {
	case model.DigestThresholdModeChannelMemberCount:
		rule.ActiveUserThresholdValue = 0
	case model.DigestThresholdModeFixed:
		if rule.ActiveUserThresholdValue <= 0 {
			rule.ActiveUserThresholdValue = 1
		}
	default:
		return fmt.Errorf("无效的访问阈值模式")
	}
	selectedChannelIDs, err := normalizeDigestSelectedChannelIDs(rule)
	if err != nil {
		return err
	}
	rule.SelectedChannelIDsJSON = mustMarshalJSON(selectedChannelIDs)
	rule.PushMode = strings.TrimSpace(rule.PushMode)
	if rule.PushMode == "" {
		rule.PushMode = model.DigestPushModePassive
	}
	switch rule.PushMode {
	case model.DigestPushModePassive, model.DigestPushModeActive, model.DigestPushModeBoth:
	default:
		return fmt.Errorf("无效的推送方式")
	}
	rule.ActiveWebhookMethod = strings.ToUpper(strings.TrimSpace(rule.ActiveWebhookMethod))
	if rule.ActiveWebhookMethod == "" {
		rule.ActiveWebhookMethod = http.MethodPost
	}
	switch rule.ActiveWebhookMethod {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return fmt.Errorf("主动推送仅支持 POST/PUT/PATCH")
	}
	headers, normalizedHeaders, err := normalizeDigestHeaders(rule.ActiveWebhookHeaders)
	if err != nil {
		return err
	}
	rule.ActiveWebhookHeaders = normalizedHeaders
	if requiresActiveDelivery(rule.PushMode) && strings.TrimSpace(rule.ActiveWebhookURL) == "" {
		return fmt.Errorf("主动推送模式需要配置推送地址")
	}
	if _, err := renderDigestJSONTemplate(rule.JSONTemplate, digestJSONTemplateValues{
		"scope_type":                 mustMarshalJSON(rule.ScopeType),
		"scope_id":                   mustMarshalJSON(rule.ScopeID),
		"scope_name":                 mustMarshalJSON("测试作用域"),
		"window_start_ts":            "0",
		"window_end_ts":              "0",
		"window_label":               mustMarshalJSON("测试时间窗口"),
		"window_seconds":             strconv.Itoa(rule.WindowSeconds),
		"channel_id":                 mustMarshalJSON(rule.ScopeID),
		"channel_name":               mustMarshalJSON("测试频道"),
		"world_id":                   mustMarshalJSON(""),
		"world_name":                 mustMarshalJSON(""),
		"channel_count":              "1",
		"target_channel_ids":         mustMarshalJSON([]string{"test-channel"}),
		"target_channel_names_array": mustMarshalJSON([]string{"测试频道"}),
		"channels":                   mustMarshalJSON([]DigestChannelSummary{{ChannelID: "test-channel", ChannelName: "测试频道", MessageCount: 1, ActiveUserCount: 1, SpeakerNames: []string{"测试成员"}, SpeakerSummary: "测试成员(1)", Speakers: []DigestSpeaker{{Key: "speaker-1", Name: "测试成员", MessageCount: 1}}, Text: "测试频道：测试成员 发送了 1 条消息"}}),
		"message_count":              "0",
		"active_user_count":          "0",
		"speaker_names_array":        "[]",
		"speaker_summary":            mustMarshalJSON(""),
		"speakers":                   "[]",
		"channel_digest_lines":       mustMarshalJSON("测试频道：测试成员 发送了 1 条消息"),
		"rendered_text":              mustMarshalJSON("测试摘要"),
	}); err != nil {
		return fmt.Errorf("JSON 模板无效: %w", err)
	}
	for key := range headers {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("主动推送头信息包含空键")
		}
	}
	return nil
}

func BuildDigestPreviewForRule(rule *model.DigestPushRuleModel, windowStart int64) (*DigestPreview, error) {
	if err := NormalizeDigestRule(rule); err != nil {
		return nil, err
	}
	if windowStart <= 0 {
		windowStart = LatestClosedDigestWindowStart(rule.WindowSeconds, time.Now())
	}
	windowStart, windowEnd := AlignDigestWindow(windowStart, rule.WindowSeconds)
	switch rule.ScopeType {
	case model.DigestScopeTypeChannel:
		return buildChannelDigestPreview(rule, windowStart, windowEnd)
	case model.DigestScopeTypeWorld:
		return buildWorldDigestPreview(rule, windowStart, windowEnd)
	default:
		return nil, fmt.Errorf("暂不支持的摘要作用域: %s", rule.ScopeType)
	}
}

func DigestRuleSelectedChannelIDs(rule *model.DigestPushRuleModel) []string {
	if rule == nil {
		return []string{}
	}
	return parseDigestSelectedChannelIDs(rule.SelectedChannelIDsJSON)
}

func DigestScopeChannelOptions(scopeType, scopeID string) ([]DigestScopeChannelOption, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	switch scopeType {
	case model.DigestScopeTypeWorld:
		channels, err := ChannelListByWorld(scopeID)
		if err != nil {
			return nil, err
		}
		options := make([]DigestScopeChannelOption, 0, len(channels))
		for _, channel := range channels {
			if channel == nil || strings.TrimSpace(channel.ID) == "" || strings.EqualFold(channel.PermType, "private") {
				continue
			}
			name := strings.TrimSpace(channel.Name)
			if name == "" {
				name = channel.ID
			}
			options = append(options, DigestScopeChannelOption{
				ID:   channel.ID,
				Name: name,
			})
		}
		return options, nil
	case model.DigestScopeTypeChannel:
		channel, err := model.ChannelGet(scopeID)
		if err != nil {
			return nil, err
		}
		if channel == nil || strings.TrimSpace(channel.ID) == "" {
			return []DigestScopeChannelOption{}, nil
		}
		name := strings.TrimSpace(channel.Name)
		if name == "" {
			name = channel.ID
		}
		return []DigestScopeChannelOption{{ID: channel.ID, Name: name}}, nil
	default:
		return []DigestScopeChannelOption{}, nil
	}
}

func DigestEffectiveThreshold(rule *model.DigestPushRuleModel) (int, error) {
	return resolveDigestThresholdValue(rule, digestTargetChannelIDs(rule))
}

func StartDigestPushWorker() {
	digestPushWorkerOnce.Do(func() {
		log.Println("digest-push: worker 启动")
		go runDigestPushWorker()
	})
}

func runDigestPushWorker() {
	ticker := time.NewTicker(DigestWorkerInterval)
	defer ticker.Stop()
	for {
		processDigestRules()
		<-ticker.C
	}
}

func processDigestRules() {
	rules, err := model.DigestPushRuleListEnabled()
	if err != nil {
		log.Printf("digest-push: 读取规则失败: %v", err)
		return
	}
	now := time.Now()
	for _, rule := range rules {
		if rule == nil || strings.TrimSpace(rule.ID) == "" {
			continue
		}
		if err := processDigestRule(rule, now); err != nil {
			log.Printf("digest-push: 处理规则失败 rule=%s scope=%s/%s err=%v", rule.ID, rule.ScopeType, rule.ScopeID, err)
		}
	}
}

func processDigestRule(rule *model.DigestPushRuleModel, now time.Time) error {
	if err := NormalizeDigestRule(rule); err != nil {
		return err
	}
	latestClosedStart := LatestClosedDigestWindowStart(rule.WindowSeconds, now)
	if latestClosedStart <= 0 {
		return nil
	}
	windowMillis := int64(rule.WindowSeconds) * 1000
	nextWindowStart := rule.LastProcessedWindowStart + windowMillis
	if rule.LastProcessedWindowStart <= 0 {
		nextWindowStart = latestClosedStart
	}
	for nextWindowStart > 0 && nextWindowStart <= latestClosedStart {
		if err := processDigestRuleWindow(rule, nextWindowStart); err != nil {
			return err
		}
		nextWindowStart += windowMillis
	}
	return nil
}

func processDigestRuleWindow(rule *model.DigestPushRuleModel, windowStart int64) error {
	existing, err := model.DigestRecordGetByRuleAndWindow(rule.ID, rule.WindowSeconds, windowStart)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != "" {
		if err := model.DigestPushRuleUpdateLastProcessed(rule.ID, windowStart); err != nil {
			return err
		}
		return cleanupDigestScopeData(rule.ScopeType, rule.ScopeID)
	}
	preview, err := BuildDigestPreviewForRule(rule, windowStart)
	if err != nil {
		return err
	}
	if preview.MessageCount <= 0 || !preview.ThresholdSatisfied {
		if err := model.DigestPushRuleUpdateLastProcessed(rule.ID, preview.WindowStart); err != nil {
			return err
		}
		return cleanupDigestScopeData(rule.ScopeType, rule.ScopeID)
	}
	record := previewToDigestRecord(rule, preview, model.DigestRecordStatusGenerated, "worker")
	if err := model.DigestRecordCreate(record); err != nil {
		return err
	}
	if requiresActiveDelivery(rule.PushMode) && strings.TrimSpace(rule.ActiveWebhookURL) != "" {
		if _, err := deliverDigestRecord(rule, record); err != nil {
			log.Printf("digest-push: 主动推送失败 rule=%s digest=%s err=%v", rule.ID, record.ID, err)
		}
	}
	if err := model.DigestPushRuleUpdateLastProcessed(rule.ID, preview.WindowStart); err != nil {
		return err
	}
	return cleanupDigestScopeData(rule.ScopeType, rule.ScopeID)
}

func TriggerDigestTest(rule *model.DigestPushRuleModel, windowStart int64, deliverActive bool) (*DigestPreview, *model.DigestRecordModel, *DigestDeliveryResult, error) {
	preview, err := BuildDigestPreviewForRule(rule, windowStart)
	if err != nil {
		return nil, nil, nil, err
	}
	if strings.TrimSpace(rule.ID) == "" {
		rule.ID = fmt.Sprintf("test:%s:%s", strings.TrimSpace(rule.ScopeType), strings.TrimSpace(rule.ScopeID))
	}
	record := previewToDigestRecord(rule, preview, model.DigestRecordStatusTest, "test")
	storedRecord, err := model.DigestRecordUpsert(record)
	if err != nil {
		return preview, nil, nil, err
	}
	if err := cleanupDigestScopeData(rule.ScopeType, rule.ScopeID); err != nil {
		return preview, storedRecord, nil, err
	}
	if !deliverActive {
		return preview, storedRecord, nil, nil
	}
	result, err := deliverDigestRecord(rule, storedRecord)
	return preview, storedRecord, result, err
}

func ChannelMemberCount(channelID string) (int, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return 0, nil
	}
	var count int64
	if err := model.GetDB().Model(&model.MemberModel{}).Where("channel_id = ?", channelID).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func normalizeDigestHeaders(raw string) (map[string]string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, "{}", nil
	}
	headers := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return nil, "", fmt.Errorf("主动推送请求头必须是 JSON 对象")
	}
	normalizedBytes, err := json.Marshal(headers)
	if err != nil {
		return nil, "", err
	}
	return headers, string(normalizedBytes), nil
}

func buildChannelDigestPreview(rule *model.DigestPushRuleModel, windowStart, windowEnd int64) (*DigestPreview, error) {
	channel, err := model.ChannelGet(rule.ScopeID)
	if err != nil {
		return nil, err
	}
	if channel == nil || channel.ID == "" {
		return nil, fmt.Errorf("频道不存在")
	}
	if strings.EqualFold(channel.PermType, "private") {
		return nil, fmt.Errorf("私聊频道暂不支持未读提醒")
	}
	worldName := ""
	if channel.WorldID != "" {
		if world, worldErr := GetWorldByID(channel.WorldID); worldErr == nil && world != nil {
			worldName = strings.TrimSpace(world.Name)
		}
	}
	activeUserCount, err := model.DigestWindowVisitorCount(rule.ScopeType, rule.ScopeID, rule.WindowSeconds, windowStart)
	if err != nil {
		return nil, err
	}
	speakerRows, err := model.DigestWindowSpeakerList(rule.ScopeType, rule.ScopeID, rule.WindowSeconds, windowStart)
	if err != nil {
		return nil, err
	}
	speakers := make([]DigestSpeaker, 0, len(speakerRows))
	speakerNames := make([]string, 0, len(speakerRows))
	messageCount := 0
	for _, row := range speakerRows {
		if row == nil || row.MessageCount <= 0 {
			continue
		}
		name := strings.TrimSpace(row.SpeakerDisplayName)
		if name == "" {
			name = row.SpeakerKey
		}
		speakers = append(speakers, DigestSpeaker{
			Key:          row.SpeakerKey,
			Name:         name,
			MessageCount: row.MessageCount,
		})
		speakerNames = append(speakerNames, name)
		messageCount += row.MessageCount
	}
	sortDigestSpeakers(speakers)
	summaryText := buildDigestChannelSummaryText(strings.TrimSpace(channel.Name), speakerNames, messageCount)
	channelSummary := DigestChannelSummary{
		ChannelID:       channel.ID,
		ChannelName:     strings.TrimSpace(channel.Name),
		MessageCount:    messageCount,
		ActiveUserCount: int(activeUserCount),
		SpeakerNames:    speakerNames,
		SpeakerSummary:  buildSpeakerSummary(speakers),
		Speakers:        speakers,
		Text:            summaryText,
	}
	thresholdValue, err := resolveDigestThresholdValue(rule, []string{channel.ID})
	if err != nil {
		return nil, err
	}
	preview := &DigestPreview{
		ScopeType:          rule.ScopeType,
		ScopeID:            rule.ScopeID,
		ScopeName:          strings.TrimSpace(channel.Name),
		ChannelID:          channel.ID,
		ChannelName:        strings.TrimSpace(channel.Name),
		WorldID:            strings.TrimSpace(channel.WorldID),
		WorldName:          worldName,
		WindowSeconds:      rule.WindowSeconds,
		WindowStart:        windowStart,
		WindowEnd:          windowEnd,
		WindowLabel:        formatDigestWindowLabel(windowStart, windowEnd),
		MessageCount:       messageCount,
		ActiveUserCount:    int(activeUserCount),
		ThresholdMode:      rule.ActiveUserThresholdMode,
		ThresholdValue:     thresholdValue,
		ThresholdSatisfied: messageCount > 0 && int(activeUserCount) >= thresholdValue,
		SpeakerNames:       speakerNames,
		SpeakerSummary:     channelSummary.SpeakerSummary,
		Speakers:           speakers,
		ChannelCount: func() int {
			if messageCount > 0 {
				return 1
			}
			return 0
		}(),
		TargetChannelIDs:   []string{channel.ID},
		TargetChannelNames: []string{strings.TrimSpace(channel.Name)},
		ChannelDigestLines: summaryText,
		Channels:           []DigestChannelSummary{channelSummary},
	}
	if preview.ChannelName == "" {
		preview.ChannelName = "未命名频道"
		preview.ScopeName = preview.ChannelName
		preview.TargetChannelNames = []string{preview.ChannelName}
		preview.Channels[0].ChannelName = preview.ChannelName
		preview.Channels[0].Text = buildDigestChannelSummaryText(preview.ChannelName, speakerNames, messageCount)
		preview.ChannelDigestLines = preview.Channels[0].Text
	}
	if err := renderDigestPreview(rule, preview); err != nil {
		return nil, err
	}
	return preview, nil
}

func buildWorldDigestPreview(rule *model.DigestPushRuleModel, windowStart, windowEnd int64) (*DigestPreview, error) {
	world, err := GetWorldByID(rule.ScopeID)
	if err != nil {
		return nil, err
	}
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return nil, fmt.Errorf("世界不存在")
	}
	targetChannels, err := resolveDigestTargetChannels(rule)
	if err != nil {
		return nil, err
	}
	targetChannelIDs := make([]string, 0, len(targetChannels))
	for _, channel := range targetChannels {
		if channel == nil || strings.TrimSpace(channel.ID) == "" {
			continue
		}
		targetChannelIDs = append(targetChannelIDs, channel.ID)
	}
	if len(targetChannelIDs) == 0 {
		return nil, fmt.Errorf("未找到可用于摘要聚合的频道")
	}

	activeUserCount, err := digestDistinctVisitorCountByChannels(targetChannelIDs, rule.WindowSeconds, windowStart)
	if err != nil {
		return nil, err
	}

	channelSummaries := make([]DigestChannelSummary, 0, len(targetChannels))
	aggregateSpeakers := map[string]*DigestSpeaker{}
	targetChannelNames := make([]string, 0, len(targetChannels))
	messageCount := 0

	for _, channel := range targetChannels {
		if channel == nil || strings.TrimSpace(channel.ID) == "" {
			continue
		}
		channelName := strings.TrimSpace(channel.Name)
		if channelName == "" {
			channelName = channel.ID
		}
		targetChannelNames = append(targetChannelNames, channelName)

		channelActiveUserCount, err := model.DigestWindowVisitorCount(model.DigestScopeTypeChannel, channel.ID, rule.WindowSeconds, windowStart)
		if err != nil {
			return nil, err
		}
		speakerRows, err := model.DigestWindowSpeakerList(model.DigestScopeTypeChannel, channel.ID, rule.WindowSeconds, windowStart)
		if err != nil {
			return nil, err
		}

		speakers := make([]DigestSpeaker, 0, len(speakerRows))
		speakerNames := make([]string, 0, len(speakerRows))
		channelMessageCount := 0
		for _, row := range speakerRows {
			if row == nil || row.MessageCount <= 0 {
				continue
			}
			name := strings.TrimSpace(row.SpeakerDisplayName)
			if name == "" {
				name = row.SpeakerKey
			}
			speakers = append(speakers, DigestSpeaker{
				Key:          row.SpeakerKey,
				Name:         name,
				MessageCount: row.MessageCount,
			})
			speakerNames = append(speakerNames, name)
			channelMessageCount += row.MessageCount
			if existing := aggregateSpeakers[row.SpeakerKey]; existing != nil {
				existing.MessageCount += row.MessageCount
				if existing.Name == "" {
					existing.Name = name
				}
			} else {
				copied := DigestSpeaker{
					Key:          row.SpeakerKey,
					Name:         name,
					MessageCount: row.MessageCount,
				}
				aggregateSpeakers[row.SpeakerKey] = &copied
			}
		}
		if channelMessageCount <= 0 {
			continue
		}
		sortDigestSpeakers(speakers)
		channelSummaries = append(channelSummaries, DigestChannelSummary{
			ChannelID:       channel.ID,
			ChannelName:     channelName,
			MessageCount:    channelMessageCount,
			ActiveUserCount: int(channelActiveUserCount),
			SpeakerNames:    speakerNames,
			SpeakerSummary:  buildSpeakerSummary(speakers),
			Speakers:        speakers,
			Text:            buildDigestChannelSummaryText(channelName, speakerNames, channelMessageCount),
		})
		messageCount += channelMessageCount
	}

	aggregateSpeakerList := make([]DigestSpeaker, 0, len(aggregateSpeakers))
	for _, speaker := range aggregateSpeakers {
		if speaker == nil || speaker.MessageCount <= 0 {
			continue
		}
		aggregateSpeakerList = append(aggregateSpeakerList, *speaker)
	}
	sortDigestSpeakers(aggregateSpeakerList)
	aggregateSpeakerNames := make([]string, 0, len(aggregateSpeakerList))
	for _, speaker := range aggregateSpeakerList {
		aggregateSpeakerNames = append(aggregateSpeakerNames, speaker.Name)
	}

	thresholdValue, err := resolveDigestThresholdValue(rule, targetChannelIDs)
	if err != nil {
		return nil, err
	}
	worldName := strings.TrimSpace(world.Name)
	if worldName == "" {
		worldName = "当前世界"
	}
	preview := &DigestPreview{
		ScopeType:          rule.ScopeType,
		ScopeID:            rule.ScopeID,
		ScopeName:          worldName,
		WorldID:            strings.TrimSpace(world.ID),
		WorldName:          worldName,
		WindowSeconds:      rule.WindowSeconds,
		WindowStart:        windowStart,
		WindowEnd:          windowEnd,
		WindowLabel:        formatDigestWindowLabel(windowStart, windowEnd),
		MessageCount:       messageCount,
		ActiveUserCount:    int(activeUserCount),
		ThresholdMode:      rule.ActiveUserThresholdMode,
		ThresholdValue:     thresholdValue,
		ThresholdSatisfied: messageCount > 0 && int(activeUserCount) >= thresholdValue,
		SpeakerNames:       aggregateSpeakerNames,
		SpeakerSummary:     buildSpeakerSummary(aggregateSpeakerList),
		Speakers:           aggregateSpeakerList,
		ChannelCount:       len(channelSummaries),
		TargetChannelIDs:   targetChannelIDs,
		TargetChannelNames: targetChannelNames,
		ChannelDigestLines: buildDigestChannelLines(channelSummaries),
		Channels:           channelSummaries,
	}
	if err := renderDigestPreview(rule, preview); err != nil {
		return nil, err
	}
	return preview, nil
}

func resolveDigestThresholdValue(rule *model.DigestPushRuleModel, channelIDs []string) (int, error) {
	switch rule.ActiveUserThresholdMode {
	case model.DigestThresholdModeFixed:
		if rule.ActiveUserThresholdValue <= 0 {
			return 1, nil
		}
		return rule.ActiveUserThresholdValue, nil
	case model.DigestThresholdModeChannelMemberCount:
		count, err := digestChannelMemberUnionCount(channelIDs)
		if err != nil {
			return 0, err
		}
		if count <= 0 {
			return 1, nil
		}
		return count, nil
	default:
		return 0, fmt.Errorf("无效的访问阈值模式")
	}
}

func renderDigestPreview(rule *model.DigestPushRuleModel, preview *DigestPreview) error {
	textValues := map[string]string{
		"scope_type":           preview.ScopeType,
		"scope_id":             preview.ScopeID,
		"scope_name":           preview.ScopeName,
		"window_start":         formatDigestTime(preview.WindowStart),
		"window_end":           formatDigestTime(preview.WindowEnd),
		"window_label":         preview.WindowLabel,
		"window_seconds":       strconv.Itoa(preview.WindowSeconds),
		"channel_id":           preview.ChannelID,
		"channel_name":         preview.ChannelName,
		"world_id":             preview.WorldID,
		"world_name":           preview.WorldName,
		"channel_count":        strconv.Itoa(preview.ChannelCount),
		"message_count":        strconv.Itoa(preview.MessageCount),
		"active_user_count":    strconv.Itoa(preview.ActiveUserCount),
		"speaker_names":        buildSpeakerNamesText(preview.SpeakerNames),
		"speaker_summary":      preview.SpeakerSummary,
		"channel_digest_lines": preview.ChannelDigestLines,
	}
	preview.RenderedText = renderDigestTextTemplate(rule.TextTemplate, textValues)
	jsonValues := digestJSONTemplateValues{
		"scope_type":                 mustMarshalJSON(preview.ScopeType),
		"scope_id":                   mustMarshalJSON(preview.ScopeID),
		"scope_name":                 mustMarshalJSON(preview.ScopeName),
		"window_start_ts":            strconv.FormatInt(preview.WindowStart, 10),
		"window_end_ts":              strconv.FormatInt(preview.WindowEnd, 10),
		"window_label":               mustMarshalJSON(preview.WindowLabel),
		"window_seconds":             strconv.Itoa(preview.WindowSeconds),
		"channel_id":                 mustMarshalJSON(preview.ChannelID),
		"channel_name":               mustMarshalJSON(preview.ChannelName),
		"world_id":                   mustMarshalJSON(preview.WorldID),
		"world_name":                 mustMarshalJSON(preview.WorldName),
		"channel_count":              strconv.Itoa(preview.ChannelCount),
		"target_channel_ids":         mustMarshalJSON(preview.TargetChannelIDs),
		"target_channel_names_array": mustMarshalJSON(preview.TargetChannelNames),
		"channels":                   mustMarshalJSON(preview.Channels),
		"message_count":              strconv.Itoa(preview.MessageCount),
		"active_user_count":          strconv.Itoa(preview.ActiveUserCount),
		"speaker_names_array":        mustMarshalJSON(preview.SpeakerNames),
		"speaker_summary":            mustMarshalJSON(preview.SpeakerSummary),
		"speakers":                   mustMarshalJSON(preview.Speakers),
		"channel_digest_lines":       mustMarshalJSON(preview.ChannelDigestLines),
		"rendered_text":              mustMarshalJSON(preview.RenderedText),
	}
	renderedJSON, err := renderDigestJSONTemplate(rule.JSONTemplate, jsonValues)
	if err != nil {
		return err
	}
	preview.RenderedJSON = renderedJSON
	var rendered any
	if err := json.Unmarshal([]byte(renderedJSON), &rendered); err != nil {
		return err
	}
	preview.RenderedJSONObject = rendered
	return nil
}

func previewToDigestRecord(rule *model.DigestPushRuleModel, preview *DigestPreview, status, triggeredBy string) *model.DigestRecordModel {
	speakerNamesJSON := mustMarshalJSON(preview.SpeakerNames)
	return &model.DigestRecordModel{
		RuleID:          rule.ID,
		ScopeType:       preview.ScopeType,
		ScopeID:         preview.ScopeID,
		WindowSeconds:   preview.WindowSeconds,
		WindowStart:     preview.WindowStart,
		WindowEnd:       preview.WindowEnd,
		MessageCount:    preview.MessageCount,
		ActiveUserCount: preview.ActiveUserCount,
		SpeakerNames:    speakerNamesJSON,
		SpeakerSummary:  preview.SpeakerSummary,
		RenderedText:    preview.RenderedText,
		RenderedJSON:    preview.RenderedJSON,
		Status:          status,
		TriggeredBy:     triggeredBy,
	}
}

func deliverDigestRecord(rule *model.DigestPushRuleModel, record *model.DigestRecordModel) (*DigestDeliveryResult, error) {
	if rule == nil || record == nil {
		return nil, fmt.Errorf("缺少主动推送参数")
	}
	headers, _, err := normalizeDigestHeaders(rule.ActiveWebhookHeaders)
	if err != nil {
		return nil, err
	}
	body := []byte(record.RenderedJSON)
	targetURL := strings.TrimSpace(rule.ActiveWebhookURL)
	if targetURL == "" {
		return nil, fmt.Errorf("缺少主动推送地址")
	}
	var lastResult *DigestDeliveryResult
	var lastErr error
	baseAttempt := record.DeliveryAttempts
	for retry := 0; retry <= DigestActivePushRetryCount; retry++ {
		attempt := baseAttempt + retry + 1
		result, err := deliverDigestRecordOnce(rule, record, headers, body, targetURL, attempt)
		lastResult = result
		lastErr = err
		if err == nil {
			return result, nil
		}
		if retry >= DigestActivePushRetryCount {
			break
		}
		time.Sleep(time.Duration(retry+1) * 500 * time.Millisecond)
	}
	if lastResult == nil && lastErr == nil {
		lastErr = errors.New("主动推送失败")
	}
	return lastResult, lastErr
}

func deliverDigestRecordOnce(rule *model.DigestPushRuleModel, record *model.DigestRecordModel, headers map[string]string, body []byte, targetURL string, attempt int) (*DigestDeliveryResult, error) {
	startedAt := time.Now()
	req, err := http.NewRequest(rule.ActiveWebhookMethod, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sealchat-digest-push/1.0")
	timestamp := strconv.FormatInt(startedAt.Unix(), 10)
	req.Header.Set("X-SealChat-Timestamp", timestamp)
	if secret := strings.TrimSpace(rule.SigningSecret); secret != "" {
		signature := signDigestPayload(secret, timestamp, body)
		req.Header.Set("X-SealChat-Signature", signature)
	}
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	resp, err := digestHTTPClient.Do(req)
	elapsedMs := time.Since(startedAt).Milliseconds()
	result := &DigestDeliveryResult{
		TargetURL:      targetURL,
		ResponseTimeMs: elapsedMs,
	}
	if err != nil {
		result.ErrorText = err.Error()
		_ = model.DigestDeliveryLogCreate(&model.DigestDeliveryLogModel{
			DigestID:       record.ID,
			DeliveryType:   model.DigestDeliveryTypeActiveWebhook,
			TargetURL:      targetURL,
			Attempt:        attempt,
			StatusCode:     0,
			Success:        false,
			ErrorText:      result.ErrorText,
			ResponseTimeMs: elapsedMs,
		})
		if strings.TrimSpace(record.ID) != "" {
			_ = model.DigestRecordIncrementDeliveryAttempts(record.ID)
		}
		return result, err
	}
	defer resp.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	result.StatusCode = resp.StatusCode
	result.ResponseBody = strings.TrimSpace(string(responseBody))
	result.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !result.Success && result.ErrorText == "" {
		result.ErrorText = fmt.Sprintf("unexpected status code %d", resp.StatusCode)
	}
	_ = model.DigestDeliveryLogCreate(&model.DigestDeliveryLogModel{
		DigestID:       record.ID,
		DeliveryType:   model.DigestDeliveryTypeActiveWebhook,
		TargetURL:      targetURL,
		Attempt:        attempt,
		StatusCode:     resp.StatusCode,
		Success:        result.Success,
		ErrorText:      result.ErrorText,
		ResponseBody:   result.ResponseBody,
		ResponseTimeMs: elapsedMs,
	})
	if strings.TrimSpace(record.ID) != "" {
		_ = model.DigestRecordIncrementDeliveryAttempts(record.ID)
	}
	if !result.Success {
		return result, errors.New(result.ErrorText)
	}
	return result, nil
}

type digestJSONTemplateValues map[string]string

func renderDigestTextTemplate(template string, values map[string]string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		template = DefaultDigestTextTemplate()
	}
	replacements := make([]string, 0, len(values)*2)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		replacements = append(replacements, "{{"+key+"}}", values[key])
	}
	return strings.NewReplacer(replacements...).Replace(template)
}

func renderDigestJSONTemplate(template string, values digestJSONTemplateValues) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		template = DefaultDigestJSONTemplate()
	}
	replacements := make([]string, 0, len(values)*2)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		replacements = append(replacements, "{{"+key+"}}", values[key])
	}
	rendered := strings.NewReplacer(replacements...).Replace(template)
	var jsonValue any
	if err := json.Unmarshal([]byte(rendered), &jsonValue); err != nil {
		return "", err
	}
	normalized, err := json.Marshal(jsonValue)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func resolveDigestSpeakerKey(message *model.MessageModel) string {
	if message == nil {
		return ""
	}
	for _, candidate := range []string{
		message.SenderIdentityID,
		message.MemberID,
		message.UserID,
	} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
	}
	return ""
}

func buildSpeakerNamesText(names []string) string {
	if len(names) == 0 {
		return "暂无发言者"
	}
	limit := len(names)
	if limit > 3 {
		limit = 3
	}
	head := strings.Join(names[:limit], "、")
	if len(names) > limit {
		return fmt.Sprintf("%s 等%d人", head, len(names))
	}
	return head
}

func buildSpeakerSummary(speakers []DigestSpeaker) string {
	if len(speakers) == 0 {
		return ""
	}
	limit := len(speakers)
	if limit > 3 {
		limit = 3
	}
	parts := make([]string, 0, limit)
	for _, speaker := range speakers[:limit] {
		parts = append(parts, fmt.Sprintf("%s(%d)", speaker.Name, speaker.MessageCount))
	}
	summary := strings.Join(parts, "、")
	if len(speakers) > limit {
		summary = fmt.Sprintf("%s 等%d人", summary, len(speakers))
	}
	return summary
}

func buildDigestChannelSummaryText(channelName string, speakerNames []string, messageCount int) string {
	channelName = strings.TrimSpace(channelName)
	if channelName == "" {
		channelName = "未命名频道"
	}
	return fmt.Sprintf("%s：%s 发送了 %d 条消息", channelName, buildSpeakerNamesText(speakerNames), messageCount)
}

func buildDigestChannelLines(channels []DigestChannelSummary) string {
	if len(channels) == 0 {
		return "暂无频道命中当前摘要窗口。"
	}
	lines := make([]string, 0, len(channels))
	for _, channel := range channels {
		text := strings.TrimSpace(channel.Text)
		if text == "" {
			text = buildDigestChannelSummaryText(channel.ChannelName, channel.SpeakerNames, channel.MessageCount)
		}
		lines = append(lines, "- "+text)
	}
	return strings.Join(lines, "\n")
}

func sortDigestSpeakers(speakers []DigestSpeaker) {
	sort.SliceStable(speakers, func(i, j int) bool {
		if speakers[i].MessageCount != speakers[j].MessageCount {
			return speakers[i].MessageCount > speakers[j].MessageCount
		}
		return strings.ToLower(strings.TrimSpace(speakers[i].Name)) < strings.ToLower(strings.TrimSpace(speakers[j].Name))
	})
}

func parseDigestSelectedChannelIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func normalizeDigestSelectedChannelIDs(rule *model.DigestPushRuleModel) ([]string, error) {
	if rule == nil {
		return []string{}, nil
	}
	switch strings.TrimSpace(rule.ScopeType) {
	case model.DigestScopeTypeChannel:
		return []string{}, nil
	case model.DigestScopeTypeWorld:
		selected := parseDigestSelectedChannelIDs(rule.SelectedChannelIDsJSON)
		if len(selected) == 0 {
			return []string{}, nil
		}
		options, err := DigestScopeChannelOptions(rule.ScopeType, rule.ScopeID)
		if err != nil {
			return nil, err
		}
		allowed := make(map[string]struct{}, len(options))
		for _, option := range options {
			if trimmed := strings.TrimSpace(option.ID); trimmed != "" {
				allowed[trimmed] = struct{}{}
			}
		}
		for _, id := range selected {
			if _, ok := allowed[id]; !ok {
				return nil, fmt.Errorf("指定频道不属于当前世界或已不可用: %s", id)
			}
		}
		return selected, nil
	default:
		return []string{}, nil
	}
}

func resolveDigestTargetChannels(rule *model.DigestPushRuleModel) ([]*model.ChannelModel, error) {
	if rule == nil {
		return []*model.ChannelModel{}, nil
	}
	switch strings.TrimSpace(rule.ScopeType) {
	case model.DigestScopeTypeChannel:
		channel, err := model.ChannelGet(rule.ScopeID)
		if err != nil {
			return nil, err
		}
		if channel == nil || strings.TrimSpace(channel.ID) == "" {
			return nil, fmt.Errorf("频道不存在")
		}
		return []*model.ChannelModel{channel}, nil
	case model.DigestScopeTypeWorld:
		channels, err := ChannelListByWorld(rule.ScopeID)
		if err != nil {
			return nil, err
		}
		selected := parseDigestSelectedChannelIDs(rule.SelectedChannelIDsJSON)
		selectedSet := make(map[string]struct{}, len(selected))
		for _, id := range selected {
			selectedSet[id] = struct{}{}
		}
		out := make([]*model.ChannelModel, 0, len(channels))
		for _, channel := range channels {
			if channel == nil || strings.TrimSpace(channel.ID) == "" || strings.EqualFold(channel.PermType, "private") {
				continue
			}
			if len(selectedSet) > 0 {
				if _, ok := selectedSet[channel.ID]; !ok {
					continue
				}
			}
			out = append(out, channel)
		}
		if len(selectedSet) > 0 && len(out) == 0 {
			return nil, fmt.Errorf("指定频道均不可用")
		}
		return out, nil
	default:
		return []*model.ChannelModel{}, nil
	}
}

func digestTargetChannelIDs(rule *model.DigestPushRuleModel) []string {
	channels, err := resolveDigestTargetChannels(rule)
	if err != nil {
		return []string{}
	}
	ids := make([]string, 0, len(channels))
	for _, channel := range channels {
		if channel == nil || strings.TrimSpace(channel.ID) == "" {
			continue
		}
		ids = append(ids, channel.ID)
	}
	return ids
}

func digestDistinctVisitorCountByChannels(channelIDs []string, windowSeconds int, windowStart int64) (int64, error) {
	cleaned := make([]string, 0, len(channelIDs))
	seen := map[string]struct{}{}
	for _, id := range channelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, id)
	}
	if len(cleaned) == 0 {
		return 0, nil
	}
	var count int64
	err := model.GetDB().Model(&model.DigestWindowVisitorModel{}).
		Where("scope_type = ? AND scope_id IN ? AND window_seconds = ? AND window_start = ?", model.DigestScopeTypeChannel, cleaned, windowSeconds, windowStart).
		Distinct("user_id").
		Count(&count).Error
	return count, err
}

func digestChannelMemberUnionCount(channelIDs []string) (int, error) {
	cleaned := make([]string, 0, len(channelIDs))
	seen := map[string]struct{}{}
	for _, id := range channelIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, id)
	}
	if len(cleaned) == 0 {
		return 0, nil
	}
	var count int64
	if err := model.GetDB().Model(&model.MemberModel{}).
		Where("channel_id IN ?", cleaned).
		Distinct("user_id").
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func requiresActiveDelivery(mode string) bool {
	switch strings.TrimSpace(mode) {
	case model.DigestPushModeActive, model.DigestPushModeBoth:
		return true
	default:
		return false
	}
}

func signDigestPayload(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func formatDigestTime(tsMillis int64) string {
	if tsMillis <= 0 {
		return ""
	}
	return time.UnixMilli(tsMillis).In(time.Local).Format("2006-01-02 15:04")
}

func formatDigestWindowLabel(windowStart, windowEnd int64) string {
	start := time.UnixMilli(windowStart).In(time.Local)
	end := time.UnixMilli(windowEnd).In(time.Local)
	return fmt.Sprintf("%s 至 %s", start.Format("2006-01-02 15:04"), end.Format("15:04"))
}

func mustMarshalJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(data)
}

func cleanupDigestScopeData(scopeType, scopeID string) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil
	}
	if err := model.DigestRecordCleanup(scopeType, scopeID, DigestRecordRetention); err != nil {
		return err
	}
	for _, windowSeconds := range DigestSupportedWindowSeconds() {
		cutoffStart := digestWindowCleanupCutoff(windowSeconds, DigestWindowRetention)
		if cutoffStart <= 0 {
			continue
		}
		if err := model.DigestWindowVisitorCleanupBefore(scopeType, scopeID, windowSeconds, cutoffStart); err != nil {
			return err
		}
		if err := model.DigestWindowSpeakerCleanupBefore(scopeType, scopeID, windowSeconds, cutoffStart); err != nil {
			return err
		}
	}
	return nil
}

func digestWindowCleanupCutoff(windowSeconds, keep int) int64 {
	if windowSeconds <= 0 || keep <= 0 {
		return 0
	}
	latestClosedStart := LatestClosedDigestWindowStart(windowSeconds, time.Now())
	if latestClosedStart <= 0 {
		return 0
	}
	windowMillis := int64(windowSeconds) * 1000
	return latestClosedStart - int64(keep-1)*windowMillis
}
