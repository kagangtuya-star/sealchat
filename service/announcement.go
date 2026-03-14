package service

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
)

var (
	ErrAnnouncementNotFound   = errors.New("announcement not found")
	ErrAnnouncementPermission = errors.New("announcement permission denied")
	ErrAnnouncementInvalid    = errors.New("announcement invalid")
)

type AnnouncementInput struct {
	Title         string `json:"title"`
	Content       string `json:"content"`
	ContentFormat string `json:"contentFormat"`
	Status        string `json:"status"`
	IsPinned      bool   `json:"isPinned"`
	PinOrder      int    `json:"pinOrder"`
	PopupMode     string `json:"popupMode"`
	RequireAck    bool   `json:"requireAck"`
}

type AnnouncementListOptions struct {
	Page            int
	PageSize        int
	IncludeAll      bool
	IncludeArchived bool
}

type AnnouncementView struct {
	model.AnnouncementModel
	CreatorName     string `json:"creatorName"`
	UpdaterName     string `json:"updaterName"`
	LastSeenVersion int    `json:"lastSeenVersion"`
	AckVersion      int    `json:"ackVersion"`
	AckCount        int    `json:"ackCount"`
	IsAcked         bool   `json:"isAcked"`
	NeedsAck        bool   `json:"needsAck"`
	CanEdit         bool   `json:"canEdit"`
}

func normalizeAnnouncementScope(scopeType, scopeID string) (model.AnnouncementScopeType, string, error) {
	switch model.AnnouncementScopeType(strings.TrimSpace(scopeType)) {
	case model.AnnouncementScopeWorld:
		scopeID = strings.TrimSpace(scopeID)
		if scopeID == "" {
			return "", "", ErrAnnouncementInvalid
		}
		return model.AnnouncementScopeWorld, scopeID, nil
	case model.AnnouncementScopeLobby:
		return model.AnnouncementScopeLobby, "", nil
	default:
		return "", "", ErrAnnouncementInvalid
	}
}

func canReadAnnouncementScope(scopeType model.AnnouncementScopeType, scopeID, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrAnnouncementPermission
	}
	switch scopeType {
	case model.AnnouncementScopeWorld:
		if _, err := GetWorldByID(scopeID); err != nil {
			return err
		}
		if pm.CanWithSystemRole(userID, pm.PermModAdmin) || IsWorldMember(scopeID, userID) {
			return nil
		}
		return ErrAnnouncementPermission
	case model.AnnouncementScopeLobby:
		return nil
	default:
		return ErrAnnouncementInvalid
	}
}

func canEditAnnouncementScope(scopeType model.AnnouncementScopeType, scopeID, userID string) error {
	if strings.TrimSpace(userID) == "" {
		return ErrAnnouncementPermission
	}
	switch scopeType {
	case model.AnnouncementScopeWorld:
		if _, err := GetWorldByID(scopeID); err != nil {
			return err
		}
		if pm.CanWithSystemRole(userID, pm.PermModAdmin) || IsWorldAdmin(scopeID, userID) {
			return nil
		}
		return ErrAnnouncementPermission
	case model.AnnouncementScopeLobby:
		if pm.CanWithSystemRole(userID, pm.PermModAdmin) {
			return nil
		}
		return ErrAnnouncementPermission
	default:
		return ErrAnnouncementInvalid
	}
}

func normalizeAnnouncementInput(scopeType model.AnnouncementScopeType, input *AnnouncementInput) error {
	if input == nil {
		return ErrAnnouncementInvalid
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return fmt.Errorf("%w: 公告标题不能为空", ErrAnnouncementInvalid)
	}
	if utf8.RuneCountInString(input.Title) > 120 {
		input.Title = string([]rune(input.Title)[:120])
	}
	input.Content = strings.TrimSpace(input.Content)
	if input.Content == "" {
		return fmt.Errorf("%w: 公告内容不能为空", ErrAnnouncementInvalid)
	}
	switch model.AnnouncementContentFormat(strings.TrimSpace(input.ContentFormat)) {
	case model.AnnouncementContentPlain:
		input.ContentFormat = string(model.AnnouncementContentPlain)
	default:
		input.ContentFormat = string(model.AnnouncementContentRich)
	}
	switch model.AnnouncementStatus(strings.TrimSpace(input.Status)) {
	case model.AnnouncementStatusDraft:
		input.Status = string(model.AnnouncementStatusDraft)
	case model.AnnouncementStatusPublished:
		input.Status = string(model.AnnouncementStatusPublished)
	case "":
		input.Status = string(model.AnnouncementStatusPublished)
	default:
		return fmt.Errorf("%w: 公告状态无效", ErrAnnouncementInvalid)
	}
	switch model.AnnouncementPopupMode(strings.TrimSpace(input.PopupMode)) {
	case model.AnnouncementPopupEveryEntry:
		input.PopupMode = string(model.AnnouncementPopupEveryEntry)
	case model.AnnouncementPopupOncePerVersion:
		input.PopupMode = string(model.AnnouncementPopupOncePerVersion)
	default:
		input.PopupMode = string(model.AnnouncementPopupNone)
	}
	if input.PinOrder < 0 {
		input.PinOrder = 0
	}
	if scopeType != model.AnnouncementScopeWorld {
		input.RequireAck = false
	}
	return nil
}

func isAnnouncementUserStateDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "UNIQUE constraint failed") {
		return true
	}
	if strings.Contains(msg, "Error 1062") || strings.Contains(msg, "Duplicate entry") {
		return true
	}
	if strings.Contains(msg, "SQLSTATE 23505") || strings.Contains(msg, "duplicate key value") {
		return true
	}
	return false
}

func loadAnnouncementOrError(id string) (*model.AnnouncementModel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrAnnouncementNotFound
	}
	var item model.AnnouncementModel
	if err := model.GetDB().Where("id = ?", id).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, ErrAnnouncementNotFound
	}
	return &item, nil
}

func resolveUserDisplayNames(ids []string) map[string]string {
	unique := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		unique = append(unique, trimmed)
	}
	if len(unique) == 0 {
		return map[string]string{}
	}
	var users []model.UserModel
	_ = model.GetDB().Select("id, nickname, username").Where("id IN ?", unique).Find(&users).Error
	ret := make(map[string]string, len(users))
	for _, user := range users {
		name := strings.TrimSpace(user.Nickname)
		if name == "" {
			name = strings.TrimSpace(user.Username)
		}
		ret[user.ID] = name
	}
	return ret
}

func loadAnnouncementStateMap(announcementIDs []string, userID string) map[string]*model.AnnouncementUserStateModel {
	if len(announcementIDs) == 0 || strings.TrimSpace(userID) == "" {
		return map[string]*model.AnnouncementUserStateModel{}
	}
	var states []model.AnnouncementUserStateModel
	_ = model.GetDB().
		Where("announcement_id IN ? AND user_id = ?", announcementIDs, userID).
		Find(&states).Error
	ret := make(map[string]*model.AnnouncementUserStateModel, len(states))
	for i := range states {
		state := states[i]
		ret[state.AnnouncementID] = &state
	}
	return ret
}

func loadAnnouncementAckCountMap(announcementIDs []string) map[string]int {
	if len(announcementIDs) == 0 {
		return map[string]int{}
	}
	type ackCountRow struct {
		AnnouncementID string
		AckCount       int
	}
	var rows []ackCountRow
	_ = model.GetDB().
		Table("announcement_user_states AS aus").
		Select("aus.announcement_id AS announcement_id, COUNT(*) AS ack_count").
		Joins("JOIN announcements AS a ON a.id = aus.announcement_id").
		Where("aus.announcement_id IN ?", announcementIDs).
		Where("aus.ack_version >= a.version").
		Group("aus.announcement_id").
		Scan(&rows).Error
	ret := make(map[string]int, len(rows))
	for _, row := range rows {
		ret[row.AnnouncementID] = row.AckCount
	}
	return ret
}

func toAnnouncementViews(items []model.AnnouncementModel, userID string, canEdit bool) []*AnnouncementView {
	announcementIDs := make([]string, 0, len(items))
	userIDs := make([]string, 0, len(items)*2)
	for _, item := range items {
		announcementIDs = append(announcementIDs, item.ID)
		userIDs = append(userIDs, item.CreatedBy, item.UpdatedBy)
	}
	stateMap := loadAnnouncementStateMap(announcementIDs, userID)
	ackCountMap := loadAnnouncementAckCountMap(announcementIDs)
	nameMap := resolveUserDisplayNames(userIDs)
	views := make([]*AnnouncementView, 0, len(items))
	for _, item := range items {
		state := stateMap[item.ID]
		lastSeenVersion := 0
		ackVersion := 0
		if state != nil {
			lastSeenVersion = state.LastSeenVersion
			ackVersion = state.AckVersion
		}
		view := &AnnouncementView{
			AnnouncementModel: item,
			CreatorName:       nameMap[item.CreatedBy],
			UpdaterName:       nameMap[item.UpdatedBy],
			LastSeenVersion:   lastSeenVersion,
			AckVersion:        ackVersion,
			AckCount:          ackCountMap[item.ID],
			IsAcked:           ackVersion >= item.Version,
			NeedsAck:          item.RequireAck && ackVersion < item.Version,
			CanEdit:           canEdit,
		}
		views = append(views, view)
	}
	return views
}

func listAnnouncements(scopeType model.AnnouncementScopeType, scopeID string, userID string, opts AnnouncementListOptions) ([]model.AnnouncementModel, int64, bool, error) {
	if err := canReadAnnouncementScope(scopeType, scopeID, userID); err != nil {
		return nil, 0, false, err
	}
	canEdit := canEditAnnouncementScope(scopeType, scopeID, userID) == nil
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 20
	}
	if opts.PageSize > 100 {
		opts.PageSize = 100
	}
	query := model.GetDB().Model(&model.AnnouncementModel{}).
		Where("scope_type = ? AND scope_id = ?", scopeType, scopeID)
	if opts.IncludeArchived {
		if !canEdit {
			return nil, 0, false, ErrAnnouncementPermission
		}
	} else {
		query = query.Where("status <> ?", model.AnnouncementStatusArchived)
	}
	if opts.IncludeAll && canEdit {
		if !opts.IncludeArchived {
			query = query.Where("status IN ?", []model.AnnouncementStatus{
				model.AnnouncementStatusDraft,
				model.AnnouncementStatusPublished,
			})
		}
	} else {
		query = query.Where("status = ?", model.AnnouncementStatusPublished)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, false, err
	}
	var items []model.AnnouncementModel
	if total == 0 {
		return items, 0, canEdit, nil
	}
	if err := query.
		Order("is_pinned DESC").
		Order("pin_order ASC").
		Order("COALESCE(published_at, created_at) DESC").
		Order("created_at DESC").
		Offset((opts.Page - 1) * opts.PageSize).
		Limit(opts.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, false, err
	}
	return items, total, canEdit, nil
}

func AnnouncementList(scopeType, scopeID, userID string, opts AnnouncementListOptions) ([]*AnnouncementView, int64, error) {
	normalizedScopeType, normalizedScopeID, err := normalizeAnnouncementScope(scopeType, scopeID)
	if err != nil {
		return nil, 0, err
	}
	items, total, canEdit, err := listAnnouncements(normalizedScopeType, normalizedScopeID, userID, opts)
	if err != nil {
		return nil, 0, err
	}
	return toAnnouncementViews(items, userID, canEdit), total, nil
}

func AnnouncementCreate(scopeType, scopeID, actorID string, input AnnouncementInput) (*AnnouncementView, error) {
	normalizedScopeType, normalizedScopeID, err := normalizeAnnouncementScope(scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	if err := canEditAnnouncementScope(normalizedScopeType, normalizedScopeID, actorID); err != nil {
		return nil, err
	}
	if err := normalizeAnnouncementInput(normalizedScopeType, &input); err != nil {
		return nil, err
	}
	item := &model.AnnouncementModel{
		ScopeType:     normalizedScopeType,
		ScopeID:       normalizedScopeID,
		Title:         input.Title,
		Content:       input.Content,
		ContentFormat: model.AnnouncementContentFormat(input.ContentFormat),
		Status:        model.AnnouncementStatus(input.Status),
		IsPinned:      input.IsPinned,
		PinOrder:      input.PinOrder,
		PopupMode:     model.AnnouncementPopupMode(input.PopupMode),
		RequireAck:    input.RequireAck,
		Version:       1,
		CreatedBy:     actorID,
		UpdatedBy:     actorID,
	}
	item.Normalize()
	if item.Status == model.AnnouncementStatusPublished {
		now := time.Now()
		item.PublishedAt = &now
	}
	if err := model.GetDB().Create(item).Error; err != nil {
		return nil, err
	}
	views := toAnnouncementViews([]model.AnnouncementModel{*item}, actorID, true)
	if len(views) == 0 {
		return nil, ErrAnnouncementNotFound
	}
	return views[0], nil
}

func AnnouncementUpdate(scopeType, scopeID, announcementID, actorID string, input AnnouncementInput) (*AnnouncementView, error) {
	normalizedScopeType, normalizedScopeID, err := normalizeAnnouncementScope(scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	if err := canEditAnnouncementScope(normalizedScopeType, normalizedScopeID, actorID); err != nil {
		return nil, err
	}
	if err := normalizeAnnouncementInput(normalizedScopeType, &input); err != nil {
		return nil, err
	}
	item, err := loadAnnouncementOrError(announcementID)
	if err != nil {
		return nil, err
	}
	if item.ScopeType != normalizedScopeType || item.ScopeID != normalizedScopeID {
		return nil, ErrAnnouncementNotFound
	}
	version := item.Version
	if strings.TrimSpace(item.Title) != input.Title ||
		strings.TrimSpace(item.Content) != input.Content ||
		item.ContentFormat != model.AnnouncementContentFormat(input.ContentFormat) ||
		item.PopupMode != model.AnnouncementPopupMode(input.PopupMode) ||
		item.RequireAck != input.RequireAck {
		version++
	}
	updates := map[string]any{
		"title":          input.Title,
		"content":        input.Content,
		"content_format": input.ContentFormat,
		"status":         input.Status,
		"is_pinned":      input.IsPinned,
		"pin_order":      input.PinOrder,
		"popup_mode":     input.PopupMode,
		"require_ack":    input.RequireAck,
		"version":        version,
		"updated_by":     actorID,
		"updated_at":     time.Now(),
	}
	if input.Status == string(model.AnnouncementStatusPublished) {
		if item.PublishedAt == nil {
			now := time.Now()
			updates["published_at"] = &now
		}
	} else if input.Status == string(model.AnnouncementStatusDraft) {
		updates["published_at"] = nil
	}
	if err := model.GetDB().Model(&model.AnnouncementModel{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	updated, err := loadAnnouncementOrError(item.ID)
	if err != nil {
		return nil, err
	}
	views := toAnnouncementViews([]model.AnnouncementModel{*updated}, actorID, true)
	if len(views) == 0 {
		return nil, ErrAnnouncementNotFound
	}
	return views[0], nil
}

func AnnouncementDelete(scopeType, scopeID, announcementID, actorID string) error {
	normalizedScopeType, normalizedScopeID, err := normalizeAnnouncementScope(scopeType, scopeID)
	if err != nil {
		return err
	}
	if err := canEditAnnouncementScope(normalizedScopeType, normalizedScopeID, actorID); err != nil {
		return err
	}
	item, err := loadAnnouncementOrError(announcementID)
	if err != nil {
		return err
	}
	if item.ScopeType != normalizedScopeType || item.ScopeID != normalizedScopeID {
		return ErrAnnouncementNotFound
	}
	return model.GetDB().Model(&model.AnnouncementModel{}).
		Where("id = ?", item.ID).
		Updates(map[string]any{
			"status":     model.AnnouncementStatusArchived,
			"updated_by": actorID,
			"updated_at": time.Now(),
		}).Error
}

func resolvePendingPopupFromItems(items []model.AnnouncementModel, userID string) *AnnouncementView {
	if len(items) == 0 {
		return nil
	}
	views := toAnnouncementViews(items, userID, false)
	for _, item := range views {
		switch item.PopupMode {
		case model.AnnouncementPopupEveryEntry:
			return item
		case model.AnnouncementPopupOncePerVersion:
			if item.LastSeenVersion < item.Version || item.NeedsAck {
				return item
			}
		default:
			if item.NeedsAck {
				return item
			}
		}
	}
	return nil
}

func AnnouncementPendingPopup(scopeType, scopeID, userID string) (*AnnouncementView, error) {
	normalizedScopeType, normalizedScopeID, err := normalizeAnnouncementScope(scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	if err := canReadAnnouncementScope(normalizedScopeType, normalizedScopeID, userID); err != nil {
		return nil, err
	}
	var items []model.AnnouncementModel
	query := model.GetDB().Model(&model.AnnouncementModel{}).
		Where("scope_type = ? AND scope_id = ? AND status = ?", normalizedScopeType, normalizedScopeID, model.AnnouncementStatusPublished).
		Where("(popup_mode <> ? OR require_ack = ?)", model.AnnouncementPopupNone, true).
		Order("CASE WHEN require_ack THEN 0 ELSE 1 END").
		Order("is_pinned DESC").
		Order("pin_order ASC").
		Order("COALESCE(published_at, created_at) DESC").
		Limit(20)
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	return resolvePendingPopupFromItems(items, userID), nil
}

func upsertAnnouncementUserState(announcementID, userID string) (*model.AnnouncementUserStateModel, error) {
	var state model.AnnouncementUserStateModel
	db := model.GetDB()
	if err := db.Where("announcement_id = ? AND user_id = ?", announcementID, userID).Limit(1).Find(&state).Error; err != nil {
		return nil, err
	}
	if state.ID != "" {
		return &state, nil
	}
	state = model.AnnouncementUserStateModel{
		AnnouncementID: announcementID,
		UserID:         userID,
	}
	if err := db.Create(&state).Error; err != nil {
		if isAnnouncementUserStateDuplicateError(err) {
			var existing model.AnnouncementUserStateModel
			if retryErr := db.Where("announcement_id = ? AND user_id = ?", announcementID, userID).Limit(1).Find(&existing).Error; retryErr != nil {
				return nil, retryErr
			}
			if existing.ID != "" {
				return &existing, nil
			}
		}
		return nil, err
	}
	return &state, nil
}

func AnnouncementMarkPopupShown(scopeType, scopeID, announcementID, userID string) (*AnnouncementView, error) {
	normalizedScopeType, normalizedScopeID, err := normalizeAnnouncementScope(scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	if err := canReadAnnouncementScope(normalizedScopeType, normalizedScopeID, userID); err != nil {
		return nil, err
	}
	item, err := loadAnnouncementOrError(announcementID)
	if err != nil {
		return nil, err
	}
	if item.ScopeType != normalizedScopeType || item.ScopeID != normalizedScopeID {
		return nil, ErrAnnouncementNotFound
	}
	state, err := upsertAnnouncementUserState(item.ID, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if err := model.GetDB().Model(&model.AnnouncementUserStateModel{}).
		Where("id = ?", state.ID).
		Updates(map[string]any{
			"last_seen_version": item.Version,
			"last_popup_at":     &now,
			"updated_at":        now,
		}).Error; err != nil {
		return nil, err
	}
	updated, err := loadAnnouncementOrError(item.ID)
	if err != nil {
		return nil, err
	}
	views := toAnnouncementViews([]model.AnnouncementModel{*updated}, userID, canEditAnnouncementScope(normalizedScopeType, normalizedScopeID, userID) == nil)
	if len(views) == 0 {
		return nil, ErrAnnouncementNotFound
	}
	return views[0], nil
}

func AnnouncementAck(scopeType, scopeID, announcementID, userID string) (*AnnouncementView, error) {
	normalizedScopeType, normalizedScopeID, err := normalizeAnnouncementScope(scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	if normalizedScopeType != model.AnnouncementScopeWorld {
		return nil, ErrAnnouncementInvalid
	}
	if err := canReadAnnouncementScope(normalizedScopeType, normalizedScopeID, userID); err != nil {
		return nil, err
	}
	item, err := loadAnnouncementOrError(announcementID)
	if err != nil {
		return nil, err
	}
	if item.ScopeType != normalizedScopeType || item.ScopeID != normalizedScopeID {
		return nil, ErrAnnouncementNotFound
	}
	if !item.RequireAck {
		return nil, ErrAnnouncementInvalid
	}
	state, err := upsertAnnouncementUserState(item.ID, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if err := model.GetDB().Model(&model.AnnouncementUserStateModel{}).
		Where("id = ?", state.ID).
		Updates(map[string]any{
			"ack_version":       item.Version,
			"ack_at":            &now,
			"last_seen_version": item.Version,
			"last_popup_at":     &now,
			"updated_at":        now,
		}).Error; err != nil {
		return nil, err
	}
	updated, err := loadAnnouncementOrError(item.ID)
	if err != nil {
		return nil, err
	}
	views := toAnnouncementViews([]model.AnnouncementModel{*updated}, userID, canEditAnnouncementScope(normalizedScopeType, normalizedScopeID, userID) == nil)
	if len(views) == 0 {
		return nil, ErrAnnouncementNotFound
	}
	return views[0], nil
}

func ArchiveAnnouncementsByScope(tx *gorm.DB, scopeType model.AnnouncementScopeType, scopeID string) error {
	if tx == nil {
		return nil
	}
	return tx.Model(&model.AnnouncementModel{}).
		Where("scope_type = ? AND scope_id = ?", scopeType, strings.TrimSpace(scopeID)).
		Updates(map[string]any{
			"status":     model.AnnouncementStatusArchived,
			"updated_at": time.Now(),
		}).Error
}
