package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
)

type BattleReportInput struct {
	Title              string
	Content            string
	PeriodStart        time.Time
	PeriodEnd          time.Time
	ContextReportCount int
	SourceChannelIDs   []string
	Status             model.BattleReportStatus
	ErrorMessage       string
	AISource           string
	AIProviderID       string
	AIModel            string
	AIFeatureKey       string
}

type BattleReportJumpEdge string

const (
	BattleReportJumpEdgeStart BattleReportJumpEdge = "start"
	BattleReportJumpEdgeEnd   BattleReportJumpEdge = "end"
)

type BattleReportJumpTarget struct {
	WorldID      string
	ChannelID    string
	MessageID    string
	CreatedAt    time.Time
	DisplayOrder float64
}

func EnsureBattleReportChannelAccess(userID, channelID string) error {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return fmt.Errorf("仅频道成员可操作战报")
	}
	member, err := model.MemberGetByUserIDAndChannelIDBase(userID, channelID, "", false)
	if err != nil {
		return err
	}
	if member == nil || strings.TrimSpace(member.ID) == "" {
		return fmt.Errorf("仅频道成员可操作战报")
	}
	return nil
}

func EnsureBattleReportWorldAccess(userID, worldID string) error {
	userID = strings.TrimSpace(userID)
	worldID = strings.TrimSpace(worldID)
	if userID == "" || worldID == "" {
		return fmt.Errorf("仅世界成员可操作战报")
	}
	if !IsWorldMember(worldID, userID) {
		return fmt.Errorf("仅世界成员可操作战报")
	}
	return nil
}

func ListBattleReports(channelID string, userID string) ([]*model.BattleReportModel, error) {
	channelID = strings.TrimSpace(channelID)
	channel, err := loadBattleReportChannel(channelID)
	if err != nil {
		return nil, err
	}
	if err := EnsureBattleReportWorldAccess(userID, channel.WorldID); err != nil {
		return nil, err
	}
	var items []*model.BattleReportModel
	err = model.GetDB().
		Where("world_id = ? AND is_deleted = ?", channel.WorldID, false).
		Order("sort_order DESC, period_start DESC, created_at DESC").
		Find(&items).Error
	return items, err
}

// ListBattleReportsForObserver returns world reports whose source channels are
// inside observer's public channel scope.
func ListBattleReportsForObserver(channelID, observerWorldID string) ([]*model.BattleReportModel, error) {
	channel, err := CanObserverAccessChannel(channelID, observerWorldID)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, gorm.ErrRecordNotFound
	}

	var candidates []*model.BattleReportModel
	if err := model.GetDB().
		Where("world_id = ? AND is_deleted = ?", strings.TrimSpace(observerWorldID), false).
		Order("sort_order DESC, period_start DESC, created_at DESC").
		Find(&candidates).Error; err != nil {
		return nil, err
	}

	items := make([]*model.BattleReportModel, 0, len(candidates))
	for _, item := range candidates {
		if item == nil || strings.TrimSpace(item.ChannelID) == "" {
			continue
		}
		if _, err := CanObserverAccessChannel(item.ChannelID, observerWorldID); err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func GetBattleReport(reportID string, userID string) (*model.BattleReportModel, error) {
	report, err := loadBattleReport(reportID)
	if err != nil {
		return nil, err
	}
	if err := EnsureBattleReportWorldAccess(userID, report.WorldID); err != nil {
		return nil, err
	}
	return report, nil
}

func GetBattleReportForObserver(reportID, observerWorldID string) (*model.BattleReportModel, error) {
	report, err := loadBattleReport(reportID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(report.WorldID) != strings.TrimSpace(observerWorldID) {
		return nil, ErrWorldPermission
	}
	if _, err := CanObserverAccessChannel(report.ChannelID, observerWorldID); err != nil {
		return nil, err
	}
	return report, nil
}

func GetBattleReportJumpTarget(reportID, edge string) (*BattleReportJumpTarget, error) {
	report, err := loadBattleReport(reportID)
	if err != nil {
		return nil, err
	}
	return resolveBattleReportJumpTarget(report, edge)
}

func GetBattleReportJumpTargetForObserver(reportID, observerWorldID, edge string) (*BattleReportJumpTarget, error) {
	report, err := GetBattleReportForObserver(reportID, observerWorldID)
	if err != nil {
		return nil, err
	}
	return resolveBattleReportJumpTarget(report, edge)
}

func resolveBattleReportJumpTarget(report *model.BattleReportModel, rawEdge string) (*BattleReportJumpTarget, error) {
	if report == nil {
		return nil, gorm.ErrRecordNotFound
	}
	edge := BattleReportJumpEdge(strings.ToLower(strings.TrimSpace(rawEdge)))
	field := ""
	var cached *string
	switch edge {
	case BattleReportJumpEdgeStart:
		field = "navigation_start_message_id"
		cached = report.NavigationStartMessageID
	case BattleReportJumpEdgeEnd:
		field = "navigation_end_message_id"
		cached = report.NavigationEndMessageID
	default:
		return nil, fmt.Errorf("无效的战报跳转位置")
	}

	if cached != nil {
		if strings.TrimSpace(*cached) == "" {
			return nil, nil
		}
		if message, err := loadBattleReportJumpMessage(report, *cached); err == nil {
			return battleReportJumpTargetFromMessage(report, message), nil
		}
	}

	message, err := findBattleReportBoundaryMessage(report, edge)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, nil
	}
	updates := map[string]any{field: message.ID}
	query := model.GetDB().Model(&model.BattleReportModel{}).
		Where("id = ? AND is_deleted = ?", report.ID, false)
	if cached == nil {
		query = query.Where(field + " IS NULL")
	}
	if err := query.Updates(updates).Error; err != nil {
		return nil, err
	}

	return battleReportJumpTargetFromMessage(report, message), nil
}

func findBattleReportBoundaryMessage(report *model.BattleReportModel, edge BattleReportJumpEdge) (*model.MessageModel, error) {
	if strings.TrimSpace(report.ChannelID) == "" {
		return nil, nil
	}
	query := model.GetDB().Model(&model.MessageModel{}).
		Where("channel_id = ?", report.ChannelID).
		Where("is_deleted = ? AND is_revoked = ?", false, false).
		Where("is_whisper = ?", false).
		Where("(ic_mode = ? OR ic_mode = '' OR ic_mode IS NULL)", "ic").
		Where("content <> ?", "")
	if !report.PeriodStart.IsZero() {
		query = query.Where("created_at >= ?", report.PeriodStart)
	}
	if !report.PeriodEnd.IsZero() {
		query = query.Where("created_at <= ?", report.PeriodEnd)
	}
	if edge == BattleReportJumpEdgeEnd {
		query = query.Order("display_order desc").Order("created_at desc").Order("id desc")
	} else {
		query = query.Order("display_order asc").Order("created_at asc").Order("id asc")
	}
	var message model.MessageModel
	if err := query.Limit(1).Find(&message).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(message.ID) == "" {
		return nil, nil
	}
	return &message, nil
}

func loadBattleReportJumpMessage(report *model.BattleReportModel, messageID string) (*model.MessageModel, error) {
	query := model.GetDB().Model(&model.MessageModel{}).
		Where("id = ? AND channel_id = ?", strings.TrimSpace(messageID), report.ChannelID).
		Where("is_deleted = ? AND is_revoked = ?", false, false).
		Where("is_whisper = ?", false).
		Where("(ic_mode = ? OR ic_mode = '' OR ic_mode IS NULL)", "ic").
		Where("content <> ?", "")
	if !report.PeriodStart.IsZero() {
		query = query.Where("created_at >= ?", report.PeriodStart)
	}
	if !report.PeriodEnd.IsZero() {
		query = query.Where("created_at <= ?", report.PeriodEnd)
	}
	var message model.MessageModel
	if err := query.Limit(1).Find(&message).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(message.ID) == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &message, nil
}

func battleReportJumpTargetFromMessage(report *model.BattleReportModel, message *model.MessageModel) *BattleReportJumpTarget {
	if report == nil || message == nil {
		return nil
	}
	return &BattleReportJumpTarget{
		WorldID:      report.WorldID,
		ChannelID:    message.ChannelID,
		MessageID:    message.ID,
		CreatedAt:    message.CreatedAt,
		DisplayOrder: message.DisplayOrder,
	}
}

func CreateBattleReport(channelID string, userID string, input BattleReportInput) (*model.BattleReportModel, error) {
	channelID = strings.TrimSpace(channelID)
	userID = strings.TrimSpace(userID)
	channel, err := loadBattleReportChannel(channelID)
	if err != nil {
		return nil, err
	}
	if err := EnsureBattleReportWorldAccess(userID, channel.WorldID); err != nil {
		return nil, err
	}
	sortOrder, err := nextBattleReportSortOrder(channel.WorldID)
	if err != nil {
		return nil, err
	}
	item := &model.BattleReportModel{
		ChannelID:          channelID,
		WorldID:            channel.WorldID,
		Title:              input.Title,
		Content:            input.Content,
		PeriodStart:        input.PeriodStart,
		PeriodEnd:          input.PeriodEnd,
		ContextReportCount: input.ContextReportCount,
		SortOrder:          sortOrder,
		Status:             input.Status,
		ErrorMessage:       input.ErrorMessage,
		CreatorID:          userID,
		UpdaterID:          userID,
		AISource:           input.AISource,
		AIProviderID:       input.AIProviderID,
		AIModel:            input.AIModel,
		AIFeatureKey:       input.AIFeatureKey,
	}
	item.Normalize()
	if err := model.GetDB().Create(item).Error; err != nil {
		return nil, err
	}
	_ = SyncBattleReportDisplayFromReports(channelID)
	return item, nil
}

func UpdateBattleReport(reportID string, userID string, input BattleReportInput) (*model.BattleReportModel, error) {
	item, err := loadBattleReport(reportID)
	if err != nil {
		return nil, err
	}
	if err := EnsureBattleReportWorldAccess(userID, item.WorldID); err != nil {
		return nil, err
	}
	periodChanged := !item.PeriodStart.Equal(input.PeriodStart) || !item.PeriodEnd.Equal(input.PeriodEnd)
	item.Title = input.Title
	item.Content = input.Content
	item.PeriodStart = input.PeriodStart
	item.PeriodEnd = input.PeriodEnd
	item.ContextReportCount = input.ContextReportCount
	if periodChanged {
		item.NavigationStartMessageID = nil
		item.NavigationEndMessageID = nil
	}
	item.UpdaterID = strings.TrimSpace(userID)
	if input.Status != "" {
		item.Status = input.Status
	}
	if input.ErrorMessage != "" {
		item.ErrorMessage = input.ErrorMessage
	}
	if input.AISource != "" {
		item.AISource = input.AISource
	}
	if input.AIProviderID != "" {
		item.AIProviderID = input.AIProviderID
	}
	if input.AIModel != "" {
		item.AIModel = input.AIModel
	}
	if input.AIFeatureKey != "" {
		item.AIFeatureKey = input.AIFeatureKey
	}
	item.Normalize()
	if err := model.GetDB().Save(item).Error; err != nil {
		return nil, err
	}
	_ = SyncBattleReportDisplayFromReports(item.ChannelID)
	return item, nil
}

func DeleteBattleReport(reportID string, userID string) error {
	item, err := loadBattleReport(reportID)
	if err != nil {
		return err
	}
	if err := EnsureBattleReportWorldAccess(userID, item.WorldID); err != nil {
		return err
	}
	now := time.Now()
	err = model.GetDB().Model(&model.BattleReportModel{}).
		Where("id = ? AND is_deleted = ?", item.ID, false).
		Updates(map[string]interface{}{
			"is_deleted": true,
			"deleted_at": now,
			"deleted_by": strings.TrimSpace(userID),
		}).Error
	if err != nil {
		return err
	}
	return SyncBattleReportDisplayFromReports(item.ChannelID)
}

func ReorderBattleReports(channelID string, userID string, ids []string) error {
	channelID = strings.TrimSpace(channelID)
	channel, err := loadBattleReportChannel(channelID)
	if err != nil {
		return err
	}
	if err := EnsureBattleReportWorldAccess(userID, channel.WorldID); err != nil {
		return err
	}
	normalizedIDs := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("战报排序包含重复 ID")
		}
		seen[id] = struct{}{}
		normalizedIDs = append(normalizedIDs, id)
	}
	if len(normalizedIDs) == 0 {
		return nil
	}

	var count int64
	if err := model.GetDB().Model(&model.BattleReportModel{}).
		Where("world_id = ? AND is_deleted = ? AND id IN ?", channel.WorldID, false, normalizedIDs).
		Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(normalizedIDs) {
		return fmt.Errorf("战报排序列表包含无效 ID")
	}

	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		base := len(normalizedIDs) * 100
		for index, id := range normalizedIDs {
			sortOrder := base - index*100
			if err := tx.Model(&model.BattleReportModel{}).
				Where("id = ? AND world_id = ? AND is_deleted = ?", id, channel.WorldID, false).
				Update("sort_order", sortOrder).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return SyncBattleReportDisplayFromReports(channelID)
}

func loadBattleReport(reportID string) (*model.BattleReportModel, error) {
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var item model.BattleReportModel
	if err := model.GetDB().
		Where("id = ? AND is_deleted = ?", reportID, false).
		First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func loadBattleReportChannel(channelID string) (*model.ChannelModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var channel model.ChannelModel
	if err := model.GetDB().
		Where("id = ? AND status <> ?", channelID, model.ChannelStatusDeleted).
		First(&channel).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

func nextBattleReportSortOrder(worldID string) (int, error) {
	var maxOrder int
	err := model.GetDB().Model(&model.BattleReportModel{}).
		Where("world_id = ? AND is_deleted = ?", strings.TrimSpace(worldID), false).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxOrder).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	return maxOrder + 100, nil
}
