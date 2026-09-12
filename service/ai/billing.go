package ai

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/utils"
)

const (
	AIQuotaPolicySourceDefault  = "default"
	AIQuotaPolicySourceOverride = "override"

	AIQuotaReservationStatusActive   = "active"
	AIQuotaReservationStatusReleased = "released"
	AIQuotaReservationStatusSettled  = "settled"
)

type UsageCostBreakdown struct {
	PromptTokens         int64
	CompletionTokens     int64
	CacheTokens          int64
	PromptPricePer1M     float64
	CompletionPricePer1M float64
	CachePricePer1M      float64
	PromptCost           float64
	CompletionCost       float64
	CacheCost            float64
	TotalCost            float64
}

type EffectiveQuotaPolicy struct {
	DailyLimit    *float64
	MonthlyLimit  *float64
	LifetimeLimit *float64
	Source        string
}

type QuotaUsageSnapshot struct {
	DailySettled    float64
	MonthlySettled  float64
	LifetimeSettled float64
	ActiveReserved  float64
}

type AIQuotaExceededError struct {
	DailyLimit     *float64
	MonthlyLimit   *float64
	LifetimeLimit  *float64
	DailyUsed      float64
	MonthlyUsed    float64
	LifetimeUsed   float64
	ActiveReserved float64
	RequestedCost  float64
}

func (e *AIQuotaExceededError) Error() string {
	if e == nil {
		return "AI 配额不足"
	}
	return fmt.Sprintf(
		"AI 配额不足：已用(日 %.4f / 月 %.4f / 总 %.4f)，预占 %.4f，本次请求 %.4f",
		e.DailyUsed,
		e.MonthlyUsed,
		e.LifetimeUsed,
		e.ActiveReserved,
		e.RequestedCost,
	)
}

var aiQuotaUserLocks sync.Map

func withAIQuotaUserLock(userID string, fn func() error) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fn()
	}
	lockValue, _ := aiQuotaUserLocks.LoadOrStore(userID, &sync.Mutex{})
	mu := lockValue.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}

const aiBillingUnitTokenCount = 1_000_000

func ResolvePricing(cfg utils.AIConfig, providerID, model string) (*utils.AIModelPricingConfig, error) {
	providerID = strings.TrimSpace(providerID)
	model = strings.TrimSpace(model)
	for _, pricing := range utils.NormalizeAIConfig(cfg).Pricing {
		if pricing.ProviderID == providerID && pricing.Model == model {
			copyPricing := pricing
			return &copyPricing, nil
		}
	}
	return nil, &AIPricingNotConfiguredError{ProviderID: providerID, Model: model}
}

func CalculateUsageCost(usage RunUsage, pricing utils.AIModelPricingConfig) UsageCostBreakdown {
	promptCost := roundAICurrency(float64(usage.PromptTokens) / aiBillingUnitTokenCount * pricing.PromptPricePer1MTokens)
	completionCost := roundAICurrency(float64(usage.CompletionTokens) / aiBillingUnitTokenCount * pricing.CompletionPricePer1MTokens)
	cacheCost := roundAICurrency(float64(usage.CacheTokens) / aiBillingUnitTokenCount * pricing.CachePricePer1MTokens)
	return UsageCostBreakdown{
		PromptTokens:         usage.PromptTokens,
		CompletionTokens:     usage.CompletionTokens,
		CacheTokens:          usage.CacheTokens,
		PromptPricePer1M:     pricing.PromptPricePer1MTokens,
		CompletionPricePer1M: pricing.CompletionPricePer1MTokens,
		CachePricePer1M:      pricing.CachePricePer1MTokens,
		PromptCost:           promptCost,
		CompletionCost:       completionCost,
		CacheCost:            cacheCost,
		TotalCost:            roundAICurrency(promptCost + completionCost + cacheCost),
	}
}

func roundAICurrency(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func ResolveEffectiveQuotaPolicy(cfg utils.AIConfig, userID string) (*EffectiveQuotaPolicy, error) {
	normalized := utils.NormalizeAIConfig(cfg)
	override, err := model.AIUserQuotaOverrideGet(userID)
	if err != nil {
		return nil, err
	}
	if override != nil {
		return &EffectiveQuotaPolicy{
			DailyLimit:    override.DailyLimit,
			MonthlyLimit:  override.MonthlyLimit,
			LifetimeLimit: override.LifetimeLimit,
			Source:        AIQuotaPolicySourceOverride,
		}, nil
	}
	return &EffectiveQuotaPolicy{
		DailyLimit:    normalized.QuotaDefault.DailyLimit,
		MonthlyLimit:  normalized.QuotaDefault.MonthlyLimit,
		LifetimeLimit: normalized.QuotaDefault.LifetimeLimit,
		Source:        AIQuotaPolicySourceDefault,
	}, nil
}

func UsageAvailable(usage RunUsage) bool {
	return usage.PromptTokens > 0 || usage.CompletionTokens > 0 || usage.CacheTokens > 0
}

func QueryQuotaUsageSnapshot(userID string, now time.Time) (*QuotaUsageSnapshot, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return &QuotaUsageSnapshot{}, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	dayKey := now.Format("2006-01-02")
	monthKey := now.Format("2006-01")
	snapshot := &QuotaUsageSnapshot{}
	db := model.GetDB()
	if err := db.Model(&model.AIUsageLedgerModel{}).
		Where("user_id = ? AND billing_day = ?", userID, dayKey).
		Select("COALESCE(SUM(total_cost), 0)").
		Scan(&snapshot.DailySettled).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&model.AIUsageLedgerModel{}).
		Where("user_id = ? AND billing_month = ?", userID, monthKey).
		Select("COALESCE(SUM(total_cost), 0)").
		Scan(&snapshot.MonthlySettled).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&model.AIUsageLedgerModel{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(total_cost), 0)").
		Scan(&snapshot.LifetimeSettled).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&model.AIQuotaReservationModel{}).
		Where("user_id = ? AND status = ? AND expires_at >= ?", userID, AIQuotaReservationStatusActive, now).
		Select("COALESCE(SUM(reserved_cost), 0)").
		Scan(&snapshot.ActiveReserved).Error; err != nil {
		return nil, err
	}
	return snapshot, nil
}

func EnsureQuotaAvailable(cfg utils.AIConfig, userID string, requestedCost float64, now time.Time) error {
	policy, err := ResolveEffectiveQuotaPolicy(cfg, userID)
	if err != nil {
		return err
	}
	if policy == nil {
		return nil
	}
	snapshot, err := QueryQuotaUsageSnapshot(userID, now)
	if err != nil {
		return err
	}
	if policy.DailyLimit != nil && snapshot.DailySettled+snapshot.ActiveReserved+requestedCost > *policy.DailyLimit {
		return &AIQuotaExceededError{
			DailyLimit:     policy.DailyLimit,
			MonthlyLimit:   policy.MonthlyLimit,
			LifetimeLimit:  policy.LifetimeLimit,
			DailyUsed:      snapshot.DailySettled,
			MonthlyUsed:    snapshot.MonthlySettled,
			LifetimeUsed:   snapshot.LifetimeSettled,
			ActiveReserved: snapshot.ActiveReserved,
			RequestedCost:  requestedCost,
		}
	}
	if policy.MonthlyLimit != nil && snapshot.MonthlySettled+snapshot.ActiveReserved+requestedCost > *policy.MonthlyLimit {
		return &AIQuotaExceededError{
			DailyLimit:     policy.DailyLimit,
			MonthlyLimit:   policy.MonthlyLimit,
			LifetimeLimit:  policy.LifetimeLimit,
			DailyUsed:      snapshot.DailySettled,
			MonthlyUsed:    snapshot.MonthlySettled,
			LifetimeUsed:   snapshot.LifetimeSettled,
			ActiveReserved: snapshot.ActiveReserved,
			RequestedCost:  requestedCost,
		}
	}
	if policy.LifetimeLimit != nil && snapshot.LifetimeSettled+snapshot.ActiveReserved+requestedCost > *policy.LifetimeLimit {
		return &AIQuotaExceededError{
			DailyLimit:     policy.DailyLimit,
			MonthlyLimit:   policy.MonthlyLimit,
			LifetimeLimit:  policy.LifetimeLimit,
			DailyUsed:      snapshot.DailySettled,
			MonthlyUsed:    snapshot.MonthlySettled,
			LifetimeUsed:   snapshot.LifetimeSettled,
			ActiveReserved: snapshot.ActiveReserved,
			RequestedCost:  requestedCost,
		}
	}
	return nil
}

func EstimatePlatformReservation(cfg utils.AIConfig, featureKey string, input string) (string, string, float64, error) {
	cfg = utils.NormalizeAIConfig(cfg)
	featureKey = strings.TrimSpace(featureKey)
	feature, ok := cfg.Features[featureKey]
	if !ok {
		return "", "", 0, fmt.Errorf("unknown ai feature: %s", featureKey)
	}
	modelName := strings.TrimSpace(feature.DefaultModel)
	if modelName == "" {
		return "", "", 0, fmt.Errorf("AI 功能 %s 缺少默认模型", featureKey)
	}
	estimatedPromptTokens := int64(len([]rune(strings.TrimSpace(input))))
	if estimatedPromptTokens <= 0 {
		estimatedPromptTokens = 1
	}
	estimatedCompletionTokens := int64(feature.Params.MaxTokens)
	if estimatedCompletionTokens <= 0 {
		estimatedCompletionTokens = 512
	}
	maxCost := -1.0
	selectedProviderID := ""
	enabledProviderCount := 0
	for _, provider := range cfg.Providers {
		if !provider.Enabled {
			continue
		}
		enabledProviderCount++
		pricing, err := ResolvePricing(cfg, provider.ID, modelName)
		if err != nil {
			continue
		}
		cost := CalculateUsageCost(RunUsage{
			PromptTokens:     estimatedPromptTokens,
			CompletionTokens: estimatedCompletionTokens,
		}, *pricing)
		if cost.TotalCost > maxCost {
			maxCost = cost.TotalCost
			selectedProviderID = provider.ID
		}
	}
	if enabledProviderCount == 0 {
		return "", "", 0, fmt.Errorf("no ai provider available")
	}
	if selectedProviderID == "" || maxCost < 0 {
		return "", "", 0, &AIPricingNotConfiguredError{Model: modelName}
	}
	return selectedProviderID, modelName, maxCost, nil
}

func ReserveQuota(userID string, featureKey string, providerID string, modelName string, reservedCost float64, now time.Time) (*model.AIQuotaReservationModel, error) {
	userID = strings.TrimSpace(userID)
	featureKey = strings.TrimSpace(featureKey)
	providerID = strings.TrimSpace(providerID)
	modelName = strings.TrimSpace(modelName)
	if userID == "" {
		return nil, fmt.Errorf("用户ID不能为空")
	}
	if now.IsZero() {
		now = time.Now()
	}
	item := &model.AIQuotaReservationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
		UserID:            userID,
		FeatureKey:        featureKey,
		ProviderID:        providerID,
		Model:             modelName,
		ReservedCost:      reservedCost,
		Status:            AIQuotaReservationStatusActive,
		ExpiresAt:         now.Add(15 * time.Minute),
	}
	return item, model.GetDB().Create(item).Error
}

func ReserveQuotaForPlatformRun(cfg utils.AIConfig, userID string, featureKey string, input string, now time.Time) (*model.AIQuotaReservationModel, error) {
	var reservation *model.AIQuotaReservationModel
	err := withAIQuotaUserLock(userID, func() error {
		providerID, modelName, estimatedCost, err := EstimatePlatformReservation(cfg, featureKey, input)
		if err != nil {
			return err
		}
		if err := EnsureQuotaAvailable(cfg, userID, estimatedCost, now); err != nil {
			return err
		}
		reservation, err = ReserveQuota(userID, featureKey, providerID, modelName, estimatedCost, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	return reservation, nil
}

func ReleaseQuotaReservation(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	return model.GetDB().Model(&model.AIQuotaReservationModel{}).
		Where("id = ? AND status = ?", id, AIQuotaReservationStatusActive).
		Updates(map[string]any{
			"status":     AIQuotaReservationStatusReleased,
			"updated_at": time.Now(),
		}).Error
}

func SettleQuotaReservation(reservationID string, ledger *model.AIUsageLedgerModel, log *model.AIUsageLogModel) error {
	reservationID = strings.TrimSpace(reservationID)
	if reservationID == "" {
		return fmt.Errorf("reservation id 不能为空")
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if log != nil {
			if err := tx.Create(log).Error; err != nil {
				return err
			}
		}
		if ledger != nil {
			if err := tx.Create(ledger).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.AIQuotaReservationModel{}).
			Where("id = ? AND status = ?", reservationID, AIQuotaReservationStatusActive).
			Updates(map[string]any{
				"status":     AIQuotaReservationStatusSettled,
				"updated_at": time.Now(),
			}).Error
	})
}
