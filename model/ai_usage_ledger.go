package model

type AIUsageLedgerModel struct {
	StringPKBaseModel
	QuotaKind        string  `json:"quotaKind" gorm:"size:16;not null;default:text;index"`
	OperationKey     *string `json:"-" gorm:"size:128;uniqueIndex"`
	UserID           string  `json:"userId" gorm:"size:100;index"`
	FeatureKey       string  `json:"featureKey" gorm:"size:64;index"`
	ProviderID       string  `json:"providerId" gorm:"size:64;index"`
	Model            string  `json:"model" gorm:"size:128;index"`
	BillingDay       string  `json:"billingDay" gorm:"size:10;index"`
	BillingMonth     string  `json:"billingMonth" gorm:"size:7;index"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	CacheTokens      int64   `json:"cacheTokens"`
	TotalCost        float64 `json:"totalCost" gorm:"index"`
	LogID            string  `json:"logId" gorm:"size:100;index"`
}

func (*AIUsageLedgerModel) TableName() string {
	return "ai_usage_ledgers"
}
