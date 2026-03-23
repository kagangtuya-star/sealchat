package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/utils"
)

const (
	DigestScopeTypeChannel = "channel"
	DigestScopeTypeWorld   = "world"

	DigestPushModePassive = "passive"
	DigestPushModeActive  = "active"
	DigestPushModeBoth    = "both"

	DigestThresholdModeChannelMemberCount = "channel_member_count"
	DigestThresholdModeFixed              = "fixed"

	DigestRecordStatusGenerated = "generated"
	DigestRecordStatusTest      = "test"

	DigestDeliveryTypeActiveWebhook = "active_webhook"
)

type DigestPushRuleModel struct {
	StringPKBaseModel
	ScopeType                string `json:"scopeType" gorm:"size:32;uniqueIndex:udx_digest_push_rule_scope,priority:1"`
	ScopeID                  string `json:"scopeId" gorm:"size:100;uniqueIndex:udx_digest_push_rule_scope,priority:2"`
	Enabled                  bool   `json:"enabled"`
	WindowSeconds            int    `json:"windowSeconds"`
	ActiveUserThresholdMode  string `json:"activeUserThresholdMode" gorm:"size:64"`
	ActiveUserThresholdValue int    `json:"activeUserThresholdValue"`
	PushMode                 string `json:"pushMode" gorm:"size:32"`
	TextTemplate             string `json:"textTemplate" gorm:"type:text"`
	JSONTemplate             string `json:"jsonTemplate" gorm:"type:text"`
	ActiveWebhookURL         string `json:"activeWebhookUrl" gorm:"size:1024"`
	ActiveWebhookMethod      string `json:"activeWebhookMethod" gorm:"size:16"`
	ActiveWebhookHeaders     string `json:"activeWebhookHeaders" gorm:"type:text"`
	SigningSecret            string `json:"-" gorm:"size:255"`
	LastProcessedWindowStart int64  `json:"lastProcessedWindowStart"`
	CreatedBy                string `json:"createdBy" gorm:"size:100"`
	UpdatedBy                string `json:"updatedBy" gorm:"size:100"`
}

func (*DigestPushRuleModel) TableName() string {
	return "digest_push_rules"
}

type DigestPushRuleUpsertParams struct {
	Enabled                  bool
	WindowSeconds            int
	ActiveUserThresholdMode  string
	ActiveUserThresholdValue int
	PushMode                 string
	TextTemplate             string
	JSONTemplate             string
	ActiveWebhookURL         string
	ActiveWebhookMethod      string
	ActiveWebhookHeaders     string
	SigningSecret            string
	ActorUserID              string
}

func DigestPushRuleGet(scopeType, scopeID string) (*DigestPushRuleModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil, nil
	}
	var item DigestPushRuleModel
	if err := db.Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func DigestPushRuleUpsert(scopeType, scopeID string, params DigestPushRuleUpsertParams) (*DigestPushRuleModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	actorID := strings.TrimSpace(params.ActorUserID)
	if scopeType == "" || scopeID == "" {
		return nil, nil
	}
	record := &DigestPushRuleModel{
		StringPKBaseModel:        StringPKBaseModel{ID: utils.NewID()},
		ScopeType:                scopeType,
		ScopeID:                  scopeID,
		Enabled:                  params.Enabled,
		WindowSeconds:            params.WindowSeconds,
		ActiveUserThresholdMode:  params.ActiveUserThresholdMode,
		ActiveUserThresholdValue: params.ActiveUserThresholdValue,
		PushMode:                 params.PushMode,
		TextTemplate:             params.TextTemplate,
		JSONTemplate:             params.JSONTemplate,
		ActiveWebhookURL:         params.ActiveWebhookURL,
		ActiveWebhookMethod:      params.ActiveWebhookMethod,
		ActiveWebhookHeaders:     params.ActiveWebhookHeaders,
		SigningSecret:            params.SigningSecret,
		CreatedBy:                actorID,
		UpdatedBy:                actorID,
	}
	updateColumns := []string{
		"enabled",
		"window_seconds",
		"active_user_threshold_mode",
		"active_user_threshold_value",
		"push_mode",
		"text_template",
		"json_template",
		"active_webhook_url",
		"active_webhook_method",
		"active_webhook_headers",
		"signing_secret",
		"updated_by",
		"updated_at",
	}
	if record.ActiveWebhookHeaders == "" {
		record.ActiveWebhookHeaders = "{}"
	}
	if record.ActiveWebhookMethod == "" {
		record.ActiveWebhookMethod = "POST"
	}
	if record.ActiveUserThresholdMode == "" {
		record.ActiveUserThresholdMode = DigestThresholdModeChannelMemberCount
	}
	if record.PushMode == "" {
		record.PushMode = DigestPushModePassive
	}
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "scope_type"},
			{Name: "scope_id"},
		},
		DoUpdates: clause.AssignmentColumns(updateColumns),
	}).Create(record).Error; err != nil {
		return nil, err
	}
	return DigestPushRuleGet(scopeType, scopeID)
}

func DigestPushRuleDelete(scopeType, scopeID string) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil
	}
	return db.Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).Delete(&DigestPushRuleModel{}).Error
}

func DigestPushRuleListEnabled() ([]*DigestPushRuleModel, error) {
	var items []*DigestPushRuleModel
	if err := db.Where("enabled = ?", true).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func DigestPushRuleUpdateLastProcessed(id string, windowStart int64) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	return db.Model(&DigestPushRuleModel{}).
		Where("id = ?", id).
		Update("last_processed_window_start", windowStart).Error
}

type DigestWindowVisitorModel struct {
	StringPKBaseModel
	ScopeType     string `json:"scopeType" gorm:"size:32;index:idx_digest_window_visitor_scope,priority:1;uniqueIndex:idx_digest_window_visitor_scope_user"`
	ScopeID       string `json:"scopeId" gorm:"size:100;index:idx_digest_window_visitor_scope,priority:2;uniqueIndex:idx_digest_window_visitor_scope_user"`
	WindowSeconds int    `json:"windowSeconds" gorm:"index:idx_digest_window_visitor_scope,priority:3;uniqueIndex:idx_digest_window_visitor_scope_user"`
	WindowStart   int64  `json:"windowStart" gorm:"index:idx_digest_window_visitor_scope,priority:4;uniqueIndex:idx_digest_window_visitor_scope_user"`
	WindowEnd     int64  `json:"windowEnd"`
	UserID        string `json:"userId" gorm:"size:100;uniqueIndex:idx_digest_window_visitor_scope_user"`
}

func (*DigestWindowVisitorModel) TableName() string {
	return "digest_window_visitors"
}

func DigestWindowVisitorUpsert(scopeType, scopeID string, windowSeconds int, windowStart, windowEnd int64, userID string) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	userID = strings.TrimSpace(userID)
	if scopeType == "" || scopeID == "" || userID == "" || windowSeconds <= 0 || windowStart <= 0 || windowEnd <= 0 {
		return nil
	}
	record := &DigestWindowVisitorModel{
		StringPKBaseModel: StringPKBaseModel{ID: utils.NewID()},
		ScopeType:         scopeType,
		ScopeID:           scopeID,
		WindowSeconds:     windowSeconds,
		WindowStart:       windowStart,
		WindowEnd:         windowEnd,
		UserID:            userID,
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(record).Error
}

func DigestWindowVisitorCount(scopeType, scopeID string, windowSeconds int, windowStart int64) (int64, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" || windowSeconds <= 0 || windowStart <= 0 {
		return 0, nil
	}
	var count int64
	err := db.Model(&DigestWindowVisitorModel{}).
		Where("scope_type = ? AND scope_id = ? AND window_seconds = ? AND window_start = ?", scopeType, scopeID, windowSeconds, windowStart).
		Count(&count).Error
	return count, err
}

type DigestWindowSpeakerModel struct {
	StringPKBaseModel
	ScopeType          string `json:"scopeType" gorm:"size:32;index:idx_digest_window_speaker_scope,priority:1;uniqueIndex:idx_digest_window_speaker_unique"`
	ScopeID            string `json:"scopeId" gorm:"size:100;index:idx_digest_window_speaker_scope,priority:2;uniqueIndex:idx_digest_window_speaker_unique"`
	WindowSeconds      int    `json:"windowSeconds" gorm:"index:idx_digest_window_speaker_scope,priority:3;uniqueIndex:idx_digest_window_speaker_unique"`
	WindowStart        int64  `json:"windowStart" gorm:"index:idx_digest_window_speaker_scope,priority:4;uniqueIndex:idx_digest_window_speaker_unique"`
	WindowEnd          int64  `json:"windowEnd"`
	SpeakerKey         string `json:"speakerKey" gorm:"size:191;uniqueIndex:idx_digest_window_speaker_unique"`
	SpeakerDisplayName string `json:"speakerDisplayName" gorm:"size:255"`
	MessageCount       int    `json:"messageCount"`
	FirstMessageAt     int64  `json:"firstMessageAt"`
	LastMessageAt      int64  `json:"lastMessageAt"`
}

func (*DigestWindowSpeakerModel) TableName() string {
	return "digest_window_speakers"
}

func DigestWindowSpeakerUpsert(scopeType, scopeID string, windowSeconds int, windowStart, windowEnd int64, speakerKey, speakerDisplayName string, messageAt int64) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	speakerKey = strings.TrimSpace(speakerKey)
	speakerDisplayName = strings.TrimSpace(speakerDisplayName)
	if scopeType == "" || scopeID == "" || speakerKey == "" || windowSeconds <= 0 || windowStart <= 0 || windowEnd <= 0 || messageAt <= 0 {
		return nil
	}
	if speakerDisplayName == "" {
		speakerDisplayName = speakerKey
	}
	record := &DigestWindowSpeakerModel{
		StringPKBaseModel:  StringPKBaseModel{ID: utils.NewID()},
		ScopeType:          scopeType,
		ScopeID:            scopeID,
		WindowSeconds:      windowSeconds,
		WindowStart:        windowStart,
		WindowEnd:          windowEnd,
		SpeakerKey:         speakerKey,
		SpeakerDisplayName: speakerDisplayName,
		MessageCount:       1,
		FirstMessageAt:     messageAt,
		LastMessageAt:      messageAt,
	}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "scope_type"},
			{Name: "scope_id"},
			{Name: "window_seconds"},
			{Name: "window_start"},
			{Name: "speaker_key"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"window_end":           windowEnd,
			"speaker_display_name": speakerDisplayName,
			"message_count":        gorm.Expr("message_count + ?", 1),
			"last_message_at":      messageAt,
			"updated_at":           time.Now(),
		}),
	}).Create(record).Error
}

func DigestWindowSpeakerList(scopeType, scopeID string, windowSeconds int, windowStart int64) ([]*DigestWindowSpeakerModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" || windowSeconds <= 0 || windowStart <= 0 {
		return []*DigestWindowSpeakerModel{}, nil
	}
	var items []*DigestWindowSpeakerModel
	if err := db.Where("scope_type = ? AND scope_id = ? AND window_seconds = ? AND window_start = ?", scopeType, scopeID, windowSeconds, windowStart).
		Order("message_count DESC").
		Order("last_message_at DESC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

type DigestRecordModel struct {
	StringPKBaseModel
	RuleID           string `json:"ruleId" gorm:"size:100;uniqueIndex:udx_digest_record_rule_window,priority:1"`
	ScopeType        string `json:"scopeType" gorm:"size:32;index"`
	ScopeID          string `json:"scopeId" gorm:"size:100;index"`
	WindowSeconds    int    `json:"windowSeconds" gorm:"uniqueIndex:udx_digest_record_rule_window,priority:2"`
	WindowStart      int64  `json:"windowStart" gorm:"uniqueIndex:udx_digest_record_rule_window,priority:3"`
	WindowEnd        int64  `json:"windowEnd"`
	MessageCount     int    `json:"messageCount"`
	ActiveUserCount  int    `json:"activeUserCount"`
	SpeakerNames     string `json:"speakerNames" gorm:"type:text"`
	SpeakerSummary   string `json:"speakerSummary" gorm:"type:text"`
	RenderedText     string `json:"renderedText" gorm:"type:text"`
	RenderedJSON     string `json:"renderedJson" gorm:"type:text"`
	Status           string `json:"status" gorm:"size:32;index"`
	GeneratedAt      int64  `json:"generatedAt" gorm:"index"`
	TriggeredBy      string `json:"triggeredBy" gorm:"size:32"`
	DeliveryAttempts int    `json:"deliveryAttempts"`
}

func (*DigestRecordModel) TableName() string {
	return "digest_records"
}

func DigestRecordGetByRuleAndWindow(ruleID string, windowSeconds int, windowStart int64) (*DigestRecordModel, error) {
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" || windowSeconds <= 0 || windowStart <= 0 {
		return nil, nil
	}
	var item DigestRecordModel
	if err := db.Where("rule_id = ? AND window_seconds = ? AND window_start = ?", ruleID, windowSeconds, windowStart).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func DigestRecordCreate(record *DigestRecordModel) error {
	if record == nil {
		return nil
	}
	if record.ID == "" {
		record.ID = utils.NewID()
	}
	if record.GeneratedAt <= 0 {
		record.GeneratedAt = time.Now().UnixMilli()
	}
	return db.Create(record).Error
}

func DigestRecordUpsert(record *DigestRecordModel) (*DigestRecordModel, error) {
	if record == nil {
		return nil, nil
	}
	if record.ID == "" {
		record.ID = utils.NewID()
	}
	if record.GeneratedAt <= 0 {
		record.GeneratedAt = time.Now().UnixMilli()
	}
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "rule_id"},
			{Name: "window_seconds"},
			{Name: "window_start"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"scope_type",
			"scope_id",
			"window_end",
			"message_count",
			"active_user_count",
			"speaker_names",
			"speaker_summary",
			"rendered_text",
			"rendered_json",
			"status",
			"generated_at",
			"triggered_by",
			"updated_at",
		}),
	}).Create(record).Error; err != nil {
		return nil, err
	}
	return DigestRecordGetByRuleAndWindow(record.RuleID, record.WindowSeconds, record.WindowStart)
}

func DigestRecordIncrementDeliveryAttempts(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	return db.Model(&DigestRecordModel{}).
		Where("id = ?", id).
		Update("delivery_attempts", gorm.Expr("delivery_attempts + 1")).Error
}

func DigestRecordList(scopeType, scopeID string, cursor int64, limit int) ([]*DigestRecordModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return []*DigestRecordModel{}, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	q := db.Where("scope_type = ? AND scope_id = ?", scopeType, scopeID)
	if cursor > 0 {
		q = q.Where("generated_at < ?", cursor)
	}
	var items []*DigestRecordModel
	if err := q.Order("generated_at DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func DigestRecordLatest(scopeType, scopeID string) (*DigestRecordModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil, nil
	}
	var item DigestRecordModel
	if err := db.Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).
		Order("generated_at DESC").
		Limit(1).
		Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func DigestRecordCleanup(scopeType, scopeID string, keep int) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil
	}
	if keep <= 0 {
		keep = 30
	}
	var staleIDs []string
	if err := db.Model(&DigestRecordModel{}).
		Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).
		Order("generated_at DESC").
		Order("id DESC").
		Offset(keep).
		Pluck("id", &staleIDs).Error; err != nil {
		return err
	}
	if len(staleIDs) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("digest_id IN ?", staleIDs).Delete(&DigestDeliveryLogModel{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", staleIDs).Delete(&DigestRecordModel{}).Error
	})
}

type DigestDeliveryLogModel struct {
	StringPKBaseModel
	DigestID       string `json:"digestId" gorm:"size:100;index"`
	DeliveryType   string `json:"deliveryType" gorm:"size:64;index"`
	TargetURL      string `json:"targetUrl" gorm:"size:1024"`
	Attempt        int    `json:"attempt"`
	StatusCode     int    `json:"statusCode"`
	Success        bool   `json:"success"`
	ErrorText      string `json:"errorText" gorm:"type:text"`
	ResponseBody   string `json:"responseBody" gorm:"type:text"`
	SentAt         int64  `json:"sentAt" gorm:"index"`
	ResponseTimeMs int64  `json:"responseTimeMs"`
}

func (*DigestDeliveryLogModel) TableName() string {
	return "digest_delivery_logs"
}

func DigestDeliveryLogCreate(item *DigestDeliveryLogModel) error {
	if item == nil {
		return nil
	}
	if item.ID == "" {
		item.ID = utils.NewID()
	}
	if item.SentAt <= 0 {
		item.SentAt = time.Now().UnixMilli()
	}
	return db.Create(item).Error
}

func DigestWindowVisitorCleanupBefore(scopeType, scopeID string, windowSeconds int, cutoffWindowStart int64) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" || windowSeconds <= 0 || cutoffWindowStart <= 0 {
		return nil
	}
	return db.Where("scope_type = ? AND scope_id = ? AND window_seconds = ? AND window_start < ?", scopeType, scopeID, windowSeconds, cutoffWindowStart).
		Delete(&DigestWindowVisitorModel{}).Error
}

func DigestWindowSpeakerCleanupBefore(scopeType, scopeID string, windowSeconds int, cutoffWindowStart int64) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" || windowSeconds <= 0 || cutoffWindowStart <= 0 {
		return nil
	}
	return db.Where("scope_type = ? AND scope_id = ? AND window_seconds = ? AND window_start < ?", scopeType, scopeID, windowSeconds, cutoffWindowStart).
		Delete(&DigestWindowSpeakerModel{}).Error
}
