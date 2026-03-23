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

type DigestSpeaker struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	MessageCount int    `json:"messageCount"`
}

type DigestPreview struct {
	ScopeType          string          `json:"scopeType"`
	ScopeID            string          `json:"scopeId"`
	ChannelID          string          `json:"channelId"`
	ChannelName        string          `json:"channelName"`
	WorldID            string          `json:"worldId"`
	WorldName          string          `json:"worldName"`
	WindowSeconds      int             `json:"windowSeconds"`
	WindowStart        int64           `json:"windowStart"`
	WindowEnd          int64           `json:"windowEnd"`
	WindowLabel        string          `json:"windowLabel"`
	MessageCount       int             `json:"messageCount"`
	ActiveUserCount    int             `json:"activeUserCount"`
	ThresholdMode      string          `json:"thresholdMode"`
	ThresholdValue     int             `json:"thresholdValue"`
	ThresholdSatisfied bool            `json:"thresholdSatisfied"`
	SpeakerNames       []string        `json:"speakerNames"`
	SpeakerSummary     string          `json:"speakerSummary"`
	Speakers           []DigestSpeaker `json:"speakers"`
	RenderedText       string          `json:"renderedText"`
	RenderedJSON       string          `json:"renderedJson"`
	RenderedJSONObject any             `json:"renderedJsonObject,omitempty"`
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

func NewDefaultDigestRule(scopeType, scopeID string) *model.DigestPushRuleModel {
	return &model.DigestPushRuleModel{
		ScopeType:                strings.TrimSpace(scopeType),
		ScopeID:                  strings.TrimSpace(scopeID),
		Enabled:                  false,
		WindowSeconds:            DigestDefaultWindowSeconds,
		ActiveUserThresholdMode:  model.DigestThresholdModeChannelMemberCount,
		ActiveUserThresholdValue: 0,
		PushMode:                 model.DigestPushModePassive,
		TextTemplate:             DefaultDigestTextTemplate(),
		JSONTemplate:             DefaultDigestJSONTemplate(),
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
	if rule.WindowSeconds <= 0 {
		rule.WindowSeconds = DigestDefaultWindowSeconds
	} else if !IsDigestWindowSecondsSupported(rule.WindowSeconds) {
		return fmt.Errorf("不支持的事件周期")
	}
	rule.TextTemplate = strings.TrimSpace(rule.TextTemplate)
	if rule.TextTemplate == "" {
		rule.TextTemplate = DefaultDigestTextTemplate()
	}
	rule.JSONTemplate = strings.TrimSpace(rule.JSONTemplate)
	if rule.JSONTemplate == "" {
		rule.JSONTemplate = DefaultDigestJSONTemplate()
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
		"scope_type":          mustMarshalJSON(rule.ScopeType),
		"scope_id":            mustMarshalJSON(rule.ScopeID),
		"window_start_ts":     "0",
		"window_end_ts":       "0",
		"window_label":        mustMarshalJSON("测试时间窗口"),
		"window_seconds":      strconv.Itoa(rule.WindowSeconds),
		"channel_id":          mustMarshalJSON(rule.ScopeID),
		"channel_name":        mustMarshalJSON("测试频道"),
		"world_id":            mustMarshalJSON(""),
		"world_name":          mustMarshalJSON(""),
		"message_count":       "0",
		"active_user_count":   "0",
		"speaker_names_array": "[]",
		"speaker_summary":     mustMarshalJSON(""),
		"speakers":            "[]",
		"rendered_text":       mustMarshalJSON("测试摘要"),
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
	default:
		return nil, fmt.Errorf("暂不支持的摘要作用域: %s", rule.ScopeType)
	}
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
	thresholdValue, err := resolveDigestThresholdValue(rule, channel.ID)
	if err != nil {
		return nil, err
	}
	preview := &DigestPreview{
		ScopeType:          rule.ScopeType,
		ScopeID:            rule.ScopeID,
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
		SpeakerSummary:     buildSpeakerSummary(speakers),
		Speakers:           speakers,
	}
	if preview.ChannelName == "" {
		preview.ChannelName = "未命名频道"
	}
	if err := renderDigestPreview(rule, preview); err != nil {
		return nil, err
	}
	return preview, nil
}

func resolveDigestThresholdValue(rule *model.DigestPushRuleModel, channelID string) (int, error) {
	switch rule.ActiveUserThresholdMode {
	case model.DigestThresholdModeFixed:
		if rule.ActiveUserThresholdValue <= 0 {
			return 1, nil
		}
		return rule.ActiveUserThresholdValue, nil
	case model.DigestThresholdModeChannelMemberCount:
		count, err := ChannelMemberCount(channelID)
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
		"scope_type":        preview.ScopeType,
		"scope_id":          preview.ScopeID,
		"window_start":      formatDigestTime(preview.WindowStart),
		"window_end":        formatDigestTime(preview.WindowEnd),
		"window_label":      preview.WindowLabel,
		"window_seconds":    strconv.Itoa(preview.WindowSeconds),
		"channel_id":        preview.ChannelID,
		"channel_name":      preview.ChannelName,
		"world_id":          preview.WorldID,
		"world_name":        preview.WorldName,
		"message_count":     strconv.Itoa(preview.MessageCount),
		"active_user_count": strconv.Itoa(preview.ActiveUserCount),
		"speaker_names":     buildSpeakerNamesText(preview.SpeakerNames),
		"speaker_summary":   preview.SpeakerSummary,
	}
	preview.RenderedText = renderDigestTextTemplate(rule.TextTemplate, textValues)
	jsonValues := digestJSONTemplateValues{
		"scope_type":          mustMarshalJSON(preview.ScopeType),
		"scope_id":            mustMarshalJSON(preview.ScopeID),
		"window_start_ts":     strconv.FormatInt(preview.WindowStart, 10),
		"window_end_ts":       strconv.FormatInt(preview.WindowEnd, 10),
		"window_label":        mustMarshalJSON(preview.WindowLabel),
		"window_seconds":      strconv.Itoa(preview.WindowSeconds),
		"channel_id":          mustMarshalJSON(preview.ChannelID),
		"channel_name":        mustMarshalJSON(preview.ChannelName),
		"world_id":            mustMarshalJSON(preview.WorldID),
		"world_name":          mustMarshalJSON(preview.WorldName),
		"message_count":       strconv.Itoa(preview.MessageCount),
		"active_user_count":   strconv.Itoa(preview.ActiveUserCount),
		"speaker_names_array": mustMarshalJSON(preview.SpeakerNames),
		"speaker_summary":     mustMarshalJSON(preview.SpeakerSummary),
		"speakers":            mustMarshalJSON(preview.Speakers),
		"rendered_text":       mustMarshalJSON(preview.RenderedText),
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
