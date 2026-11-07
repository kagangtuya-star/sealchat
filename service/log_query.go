package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
	"sealchat/protocol"
)

type LogMessageWindow struct {
	Messages      []*model.MessageModel
	Channel       *model.ChannelModel
	World         *model.WorldModel
	NextCursor    *string
	SourceVersion int64
	// Presentations holds the read-only display projection by message ID.
	Presentations map[string]*LogMessagePresentation
}
type logMessageCursor struct {
	ProjectID     string    `json:"p"`
	SectionID     string    `json:"s"`
	DraftVersion  int64     `json:"d"`
	SourceVersion int64     `json:"v"`
	Filter        string    `json:"f"`
	Order         float64   `json:"o"`
	CreatedAt     time.Time `json:"t"`
	ID            string    `json:"i"`
}

func logSourceVersion(channelID string) (int64, error) {
	var v model.LogChannelVersionModel
	err := model.GetDB().Where("channel_id = ?", channelID).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return v.Version, err
}
func logSourceQuery(p *model.LogProjectModel, s LogSection, userID string) *gorm.DB {
	return logSourceVisibilityQuery(p, s, userID).Where("created_at <= ?", s.Source.ToTime)
}

// 范围检查和来源读取共享全部可见性及下界条件，只分别应用结束时间。
func logSourceVisibilityQuery(p *model.LogProjectModel, s LogSection, userID string) *gorm.DB {
	q := model.GetDB().Model(&model.MessageModel{}).Where("channel_id = ? AND is_deleted = ? AND (is_revoked = ? OR is_revoked IS NULL)", s.Source.ChannelID, false, false)
	if s.Source.FromTime != nil {
		op := ">="
		if s.Source.FromExclusive {
			op = ">"
		}
		q = q.Where("created_at "+op+" ?", *s.Source.FromTime)
	}
	if !s.Source.IncludeOOC {
		q = q.Where("COALESCE(ic_mode, 'ic') <> ?", "ooc")
	}
	if !s.Source.IncludeArchived {
		q = q.Where("is_archived = ?", false)
	}
	if len(s.Source.IdentityIDs) > 0 {
		q = q.Where("sender_identity_id IN ?", s.Source.IdentityIDs)
	}
	q = q.Where("NOT EXISTS (SELECT 1 FROM log_message_exclusions e WHERE e.project_id = ? AND e.section_id = ? AND e.message_id = messages.id)", p.ID, s.ID)
	return ApplyWhisperVisibilityFilter(q, userID, s.Source.ChannelID)
}

type LogSourceRangeCheck struct {
	Count         int64      `json:"count"`
	LatestTime    *time.Time `json:"latestTime"`
	DraftVersion  int64      `json:"draftVersion"`
	SourceVersion int64      `json:"sourceVersion"`
}

// CheckLogSourceRange 按需统计固定截止时间之后的可见消息，不影响普通分页。
func CheckLogSourceRange(projectID, sectionID, userID string, expectedVersion int64) (*LogSourceRangeCheck, error) {
	p, _, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return nil, err
	}
	if expectedVersion < 1 || expectedVersion > LogMaxVersion {
		return nil, logError("INVALID_INPUT", "版本无效")
	}
	if expectedVersion != p.DraftVersion {
		return nil, logError("SOURCE_CHANGED", "来源或排版范围已变化，请重新加载")
	}
	s, _, err := logSection(p, sectionID)
	if err != nil {
		return nil, err
	}
	version, err := logSourceVersion(s.Source.ChannelID)
	if err != nil {
		return nil, err
	}
	result := &LogSourceRangeCheck{DraftVersion: p.DraftVersion, SourceVersion: version}
	query := func() *gorm.DB {
		return logSourceVisibilityQuery(p, s, userID).Where("created_at > ?", s.Source.ToTime)
	}
	if err := query().Count(&result.Count).Error; err != nil {
		return nil, err
	}
	if result.Count > 0 {
		var latest model.MessageModel
		if err := query().Select("created_at").Order("created_at DESC").Take(&latest).Error; err != nil {
			return nil, err
		}
		result.LatestTime = &latest.CreatedAt
	}
	if err := logVerifyWindow(p, s.Source.ChannelID, version); err != nil {
		return nil, err
	}
	return result, nil
}
func logWindowPreloads(q *gorm.DB) *gorm.DB {
	return q.Preload("User", func(db *gorm.DB) *gorm.DB { return db.Select("id, username, nickname, avatar, is_bot") }).Preload("Member", func(db *gorm.DB) *gorm.DB { return db.Select("id, nickname, channel_id, user_id") })
}
func logOrderAfter(q *gorm.DB, order float64, created time.Time, id string, after bool) *gorm.DB {
	op := ">"
	if !after {
		op = "<"
	}
	return q.Where("(display_order "+op+" ? OR (display_order = ? AND created_at "+op+" ?) OR (display_order = ? AND created_at = ? AND id "+op+" ?))", order, order, created, order, created, id)
}
func logSection(p *model.LogProjectModel, id string) (LogSection, string, error) {
	var draft LogDraft
	if err := json.Unmarshal([]byte(p.DraftJSON), &draft); err != nil {
		return LogSection{}, "", err
	}
	for _, s := range draft.Sections {
		if s.ID == id {
			raw, _ := json.Marshal(s.Source)
			digest := sha256.Sum256(raw)
			return s, hex.EncodeToString(digest[:]), nil
		}
	}
	return LogSection{}, "", logError("NOT_FOUND", "章节不存在")
}
func logVerifyWindow(p *model.LogProjectModel, channelID string, version int64) error {
	v, err := logSourceVersion(channelID)
	if err != nil {
		return err
	}
	var current model.LogProjectModel
	if err := model.GetDB().Select("id, draft_version, enabled").Where("id = ?", p.ID).Take(&current).Error; err != nil {
		return err
	}
	if v != version || current.DraftVersion != p.DraftVersion || !current.Enabled {
		return logError("SOURCE_CHANGED", "来源或排版范围已变化，请重新加载")
	}
	return nil
}

// LogDesignMessageData shapes one window message the way design reads expose it:
// the protocol message plus its author user and member. The design API and the
// background PDF render payload share it so both feed the renderer the same data.
func LogDesignMessageData(m *model.MessageModel, channelData *protocol.Channel) *protocol.Message {
	messageData := m.ToProtocolType2(channelData)
	if m.User != nil {
		messageData.User = m.User.ToProtocolType()
	} else if m.UserID != "" {
		messageData.User = &protocol.User{ID: m.UserID}
	}
	if m.Member != nil {
		messageData.Member = m.Member.ToProtocolType()
		if messageData.User != nil {
			messageData.Member.User = messageData.User
		}
	}
	return messageData
}

func QueryLogMessages(projectID, sectionID, userID, cursor, targetID string, limit int) (*LogMessageWindow, error) {
	if limit < 1 || limit > 200 {
		return nil, logError("INVALID_INPUT", "limit 须为 1～200")
	}
	p, _, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return nil, err
	}
	s, filter, err := logSection(p, sectionID)
	if err != nil {
		return nil, err
	}
	version, err := logSourceVersion(s.Source.ChannelID)
	if err != nil {
		return nil, err
	}
	var key logMessageCursor
	if cursor != "" {
		if len(cursor) > 2048 {
			return nil, logError("INVALID_INPUT", "游标无效")
		}
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(raw, &key) != nil || key.ProjectID != projectID || key.SectionID != sectionID || key.Filter != filter || key.ID == "" || len(key.ID) > 100 || key.CreatedAt.IsZero() || math.IsNaN(key.Order) || math.IsInf(key.Order, 0) {
			return nil, logError("INVALID_INPUT", "游标无效")
		}
		if key.SourceVersion != version || key.DraftVersion != p.DraftVersion {
			return nil, logError("SOURCE_CHANGED", "来源或排版范围已变化，请重新加载")
		}
	}
	window := &LogMessageWindow{Messages: []*model.MessageModel{}, SourceVersion: version}
	window.Channel, err = model.ChannelGet(s.Source.ChannelID)
	if err != nil {
		return nil, err
	}
	window.World, err = GetWorldByID(p.WorldID)
	if err != nil {
		return nil, err
	}
	base := func() *gorm.DB { return logWindowPreloads(logSourceQuery(p, s, userID)) }
	if targetID != "" {
		var target model.MessageModel
		if err := base().Where("id = ?", targetID).Take(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, logError("NOT_FOUND", "消息不存在")
			}
			return nil, err
		}
		var before, after []*model.MessageModel
		if err := logOrderAfter(base(), target.DisplayOrder, target.CreatedAt, target.ID, false).Order("display_order DESC, created_at DESC, id DESC").Limit(20).Find(&before).Error; err != nil {
			return nil, err
		}
		if err := logOrderAfter(base(), target.DisplayOrder, target.CreatedAt, target.ID, true).Order("display_order ASC, created_at ASC, id ASC").Limit(20).Find(&after).Error; err != nil {
			return nil, err
		}
		for i := len(before) - 1; i >= 0; i-- {
			window.Messages = append(window.Messages, before[i])
		}
		window.Messages = append(window.Messages, &target)
		window.Messages = append(window.Messages, after...)
	} else {
		q := base()
		if cursor != "" {
			q = logOrderAfter(q, key.Order, key.CreatedAt, key.ID, true)
		}
		if err := q.Order("display_order ASC, created_at ASC, id ASC").Limit(limit + 1).Find(&window.Messages).Error; err != nil {
			return nil, err
		}
		if len(window.Messages) > limit {
			window.Messages = window.Messages[:limit]
			last := window.Messages[limit-1]
			raw, _ := json.Marshal(logMessageCursor{ProjectID: projectID, SectionID: sectionID, DraftVersion: p.DraftVersion, SourceVersion: version, Filter: filter, Order: last.DisplayOrder, CreatedAt: last.CreatedAt, ID: last.ID})
			next := base64.RawURLEncoding.EncodeToString(raw)
			window.NextCursor = &next
		}
	}
	if s.Source.ShowQuoteContent {
		// Quotes are independently filtered; never expand an inaccessible source.
		quoteIDs := []string{}
		for _, m := range window.Messages {
			if m.QuoteID != "" {
				quoteIDs = append(quoteIDs, m.QuoteID)
			}
		}
		quotes := map[string]*model.MessageModel{}
		if len(quoteIDs) > 0 {
			var rows []*model.MessageModel
			if err := base().Where("id IN ? AND is_revoked = ?", quoteIDs, false).Find(&rows).Error; err != nil {
				return nil, err
			}
			for _, m := range rows {
				quotes[m.ID] = m
			}
		}
		for _, m := range window.Messages {
			m.Quote = quotes[m.QuoteID]
		}
	}
	if window.Presentations, err = logDesignPresentations(userID, sectionID, window.Messages); err != nil {
		return nil, err
	}
	if err := logVerifyWindow(p, s.Source.ChannelID, version); err != nil {
		return nil, err
	}
	return window, nil
}

type LogExclusionUpdate struct {
	ExpectedVersion int64    `json:"expectedVersion"`
	MessageIDs      []string `json:"messageIds"`
	Excluded        *bool    `json:"excluded"`
}

func UpdateLogExclusions(projectID, sectionID, userID string, input LogExclusionUpdate) (int64, error) {
	p, admin, err := loadLogProjectForDesign(projectID, userID)
	if err != nil {
		return 0, err
	}
	if !admin {
		return 0, logError("PERMISSION_DENIED", "仅管理者可以修改排除集合")
	}
	if input.ExpectedVersion < 1 || input.ExpectedVersion >= LogMaxVersion || input.Excluded == nil || len(input.MessageIDs) < 1 || len(input.MessageIDs) > 200 {
		return 0, logError("INVALID_INPUT", "排除参数无效")
	}
	if p.DraftVersion != input.ExpectedVersion {
		return 0, logConflict(p.DraftVersion)
	}
	s, _, err := logSection(p, sectionID)
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, id := range input.MessageIDs {
		if id == "" || len(id) > 100 || seen[id] {
			return 0, logError("INVALID_INPUT", "消息标识无效")
		}
		seen[id] = true
	}
	var count int64
	q := ApplyWhisperVisibilityFilter(model.GetDB().Model(&model.MessageModel{}).Where("channel_id = ? AND id IN ?", s.Source.ChannelID, input.MessageIDs), userID, s.Source.ChannelID)
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	if count != int64(len(input.MessageIDs)) {
		return 0, logError("NOT_FOUND", "消息不存在")
	}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		r := tx.Model(&model.LogProjectModel{}).Where("id = ? AND enabled = ? AND draft_version = ?", p.ID, true, input.ExpectedVersion).UpdateColumn("draft_version", gorm.Expr("draft_version + 1"))
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return logConflict(p.DraftVersion)
		}
		if !*input.Excluded {
			return tx.Where("project_id = ? AND section_id = ? AND message_id IN ?", p.ID, sectionID, input.MessageIDs).Delete(&model.LogMessageExclusionModel{}).Error
		}
		rows := make([]model.LogMessageExclusionModel, 0, len(input.MessageIDs))
		for _, id := range input.MessageIDs {
			rows = append(rows, model.LogMessageExclusionModel{ProjectID: p.ID, SectionID: sectionID, MessageID: id})
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	})
	if err != nil {
		return 0, err
	}
	return p.DraftVersion + 1, nil
}
