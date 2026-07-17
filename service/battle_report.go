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
	item.Title = input.Title
	item.Content = input.Content
	item.PeriodStart = input.PeriodStart
	item.PeriodEnd = input.PeriodEnd
	item.ContextReportCount = input.ContextReportCount
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
