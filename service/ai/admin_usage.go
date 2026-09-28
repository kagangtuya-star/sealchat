package ai

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
	"sealchat/utils"
)

type AdminUsageLogQuery struct {
	QuotaKind  string
	Page       int
	PageSize   int
	Query      string
	FeatureKey string
	ProviderID string
	Model      string
	Status     string
	StartMS    int64
	EndMS      int64
}

type AdminUsageLogListResult struct {
	Items    []model.AIUsageLogModel `json:"items"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"pageSize"`
	Total    int64                   `json:"total"`
}

type AdminAIQuotaUsageSummary struct {
	DailySettled    float64 `json:"dailySettled"`
	MonthlySettled  float64 `json:"monthlySettled"`
	LifetimeSettled float64 `json:"lifetimeSettled"`
	ActiveReserved  float64 `json:"activeReserved"`
}

type AdminAIQuotaDetail struct {
	UserID          string                     `json:"userId"`
	Username        string                     `json:"username"`
	Nickname        string                     `json:"nickname"`
	Source          string                     `json:"source"`
	DefaultPolicy   utils.AIQuotaPolicyConfig  `json:"defaultPolicy"`
	Override        *utils.AIQuotaPolicyConfig `json:"override"`
	EffectivePolicy utils.AIQuotaPolicyConfig  `json:"effectivePolicy"`
	Usage           AdminAIQuotaUsageSummary   `json:"usage"`
}

type AdminAIQuotaListResult struct {
	Items    []AdminAIQuotaDetail `json:"items"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
	Total    int64                `json:"total"`
}

func AdminListUsageLogs(q AdminUsageLogQuery) (*AdminUsageLogListResult, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 200 {
		q.PageSize = 20
	}
	if q.QuotaKind == "" {
		q.QuotaKind = model.QuotaKindText
	}
	if q.QuotaKind != model.QuotaKindText && q.QuotaKind != model.QuotaKindSpeech {
		return nil, errors.New("invalid quota kind")
	}
	db := model.GetDB().Model(&model.AIUsageLogModel{}).Where("quota_kind = ?", q.QuotaKind)
	if query := strings.TrimSpace(q.Query); query != "" {
		like := "%" + query + "%"
		db = db.Where("user_id LIKE ? OR username_snapshot LIKE ? OR nickname_snapshot LIKE ?", like, like, like)
	}
	if value := strings.TrimSpace(q.FeatureKey); value != "" {
		db = db.Where("feature_key = ?", value)
	}
	if value := strings.TrimSpace(q.ProviderID); value != "" {
		db = db.Where("provider_id = ?", value)
	}
	if value := strings.TrimSpace(q.Model); value != "" {
		db = db.Where("model = ?", value)
	}
	if value := strings.TrimSpace(q.Status); value != "" {
		db = db.Where("status = ?", value)
	}
	if q.StartMS > 0 {
		db = db.Where("finished_at >= ?", time.UnixMilli(q.StartMS))
	}
	if q.EndMS > 0 {
		db = db.Where("finished_at <= ?", time.UnixMilli(q.EndMS))
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	items := make([]model.AIUsageLogModel, 0, q.PageSize)
	if err := db.Order("finished_at DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return &AdminUsageLogListResult{
		Items:    items,
		Page:     q.Page,
		PageSize: q.PageSize,
		Total:    total,
	}, nil
}

func AdminCleanupUsageLogs(retentionDays int, now time.Time) (int64, error) {
	return AdminCleanupUsageLogsForKind(model.QuotaKindText, retentionDays, now)
}

func AdminCleanupUsageLogsForKind(kind string, retentionDays int, now time.Time) (int64, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if retentionDays <= 0 {
		retentionDays = 30
		if cfg := utils.GetConfig(); cfg != nil {
			normalized := utils.NormalizeAIConfig(cfg.AI)
			retentionDays = normalized.LogRetentionDays
		}
	}
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	return model.AIUsageLogCleanupBeforeForKind(kind, cutoff)
}

func adminAIQuotaUserExists(userID string) (bool, error) {
	var count int64
	if err := model.GetDB().Model(&model.UserModel{}).Where("id = ?", strings.TrimSpace(userID)).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func validateOverridePolicy(policy utils.AIQuotaPolicyConfig) error {
	if policy.DailyLimit == nil && policy.MonthlyLimit == nil && policy.LifetimeLimit == nil {
		return errors.New("至少提供一个配额字段")
	}
	if policy.DailyLimit != nil && *policy.DailyLimit < 0 {
		return errors.New("日额度不能小于 0")
	}
	if policy.MonthlyLimit != nil && *policy.MonthlyLimit < 0 {
		return errors.New("月额度不能小于 0")
	}
	if policy.LifetimeLimit != nil && *policy.LifetimeLimit < 0 {
		return errors.New("总额度不能小于 0")
	}
	return nil
}

func AdminUpsertQuotaOverride(userID, updatedBy string, policy utils.AIQuotaPolicyConfig) (*model.AIUserQuotaOverrideModel, error) {
	userID = strings.TrimSpace(userID)
	updatedBy = strings.TrimSpace(updatedBy)
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}
	if err := validateOverridePolicy(policy); err != nil {
		return nil, err
	}
	if exists, err := adminAIQuotaUserExists(userID); err != nil {
		return nil, err
	} else if !exists {
		return nil, gorm.ErrRecordNotFound
	}
	record := &model.AIUserQuotaOverrideModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
		UserID:            userID,
		DailyLimit:        policy.DailyLimit,
		MonthlyLimit:      policy.MonthlyLimit,
		LifetimeLimit:     policy.LifetimeLimit,
		UpdatedBy:         updatedBy,
	}
	if err := model.GetDB().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"daily_limit":    record.DailyLimit,
			"monthly_limit":  record.MonthlyLimit,
			"lifetime_limit": record.LifetimeLimit,
			"updated_by":     record.UpdatedBy,
			"updated_at":     time.Now(),
		}),
	}).Create(record).Error; err != nil {
		return nil, err
	}
	return model.AIUserQuotaOverrideGet(userID)
}

func AdminDeleteQuotaOverride(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	return model.GetDB().Where("user_id = ?", userID).Delete(&model.AIUserQuotaOverrideModel{}).Error
}

func AdminGetQuotaDetail(userID string, now time.Time) (*AdminAIQuotaDetail, error) {
	user, err := model.UserGetEx(userID)
	if err != nil {
		return nil, err
	}
	cfg := utils.NormalizeAIConfig(utils.GetConfig().AI)
	overrideModel, err := model.AIUserQuotaOverrideGet(user.ID)
	if err != nil {
		return nil, err
	}
	effective, err := ResolveEffectiveQuotaPolicy(cfg, user.ID)
	if err != nil {
		return nil, err
	}
	snapshot, err := QueryQuotaUsageSnapshot(user.ID, now)
	if err != nil {
		return nil, err
	}
	detail := &AdminAIQuotaDetail{
		UserID:   user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Source:   effective.Source,
		DefaultPolicy: utils.AIQuotaPolicyConfig{
			DailyLimit:    cfg.QuotaDefault.DailyLimit,
			MonthlyLimit:  cfg.QuotaDefault.MonthlyLimit,
			LifetimeLimit: cfg.QuotaDefault.LifetimeLimit,
		},
		EffectivePolicy: utils.AIQuotaPolicyConfig{
			DailyLimit:    effective.DailyLimit,
			MonthlyLimit:  effective.MonthlyLimit,
			LifetimeLimit: effective.LifetimeLimit,
		},
		Usage: AdminAIQuotaUsageSummary{
			DailySettled:    snapshot.DailySettled,
			MonthlySettled:  snapshot.MonthlySettled,
			LifetimeSettled: snapshot.LifetimeSettled,
			ActiveReserved:  snapshot.ActiveReserved,
		},
	}
	if overrideModel != nil {
		detail.Override = &utils.AIQuotaPolicyConfig{
			DailyLimit:    overrideModel.DailyLimit,
			MonthlyLimit:  overrideModel.MonthlyLimit,
			LifetimeLimit: overrideModel.LifetimeLimit,
		}
	}
	return detail, nil
}

func AdminListQuotaOverrides(page, pageSize int, query string, now time.Time) (*AdminAIQuotaListResult, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	query = strings.TrimSpace(query)
	db := model.GetDB().Model(&model.AIUserQuotaOverrideModel{})
	if query != "" {
		like := "%" + query + "%"
		userQuery := model.GetDB().Model(&model.UserModel{}).Select("id").
			Where("id LIKE ? OR username LIKE ? OR nickname LIKE ?", like, like, like)
		db = db.Where("user_id IN (?)", userQuery)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []model.AIUserQuotaOverrideModel
	if err := db.Order("updated_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]AdminAIQuotaDetail, 0, len(rows))
	for _, row := range rows {
		detail, err := AdminGetQuotaDetail(row.UserID, now)
		if err != nil {
			return nil, err
		}
		items = append(items, *detail)
	}
	return &AdminAIQuotaListResult{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}
