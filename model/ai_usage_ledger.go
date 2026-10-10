package model

type AIUsageLedgerModel struct {
	StringPKBaseModel
	QuotaKind        string  `json:"quotaKind" gorm:"size:16;not null;default:text;index;index:idx_ledger_world_day,priority:1;index:idx_ledger_world_month,priority:1"`
	OperationKey     *string `json:"-" gorm:"size:128;uniqueIndex"`
	UserID           string  `json:"userId" gorm:"size:100;index"`
	WorldID          string  `json:"worldId" gorm:"size:100;not null;default:'';index:idx_ledger_world_day,priority:2;index:idx_ledger_world_month,priority:2"`
	FeatureKey       string  `json:"featureKey" gorm:"size:64;index"`
	ProviderID       string  `json:"providerId" gorm:"size:64;index"`
	Model            string  `json:"model" gorm:"size:128;index"`
	BillingDay       string  `json:"billingDay" gorm:"size:10;index;index:idx_ledger_world_day,priority:3"`
	BillingMonth     string  `json:"billingMonth" gorm:"size:7;index;index:idx_ledger_world_month,priority:3"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	CacheTokens      int64   `json:"cacheTokens"`
	TotalCost        float64 `json:"totalCost" gorm:"index"`
	LogID            string  `json:"logId" gorm:"size:100;index"`
}

func (*AIUsageLedgerModel) TableName() string {
	return "ai_usage_ledgers"
}
