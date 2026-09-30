package model

import (
	"gorm.io/gorm"
	"time"
)

const QuotaKindText = "text"
const QuotaKindSpeech = "speech"

type TTSUserPolicy struct {
	StringPKBaseModel
	UserID          string   `json:"userId" gorm:"size:100;uniqueIndex"`
	OverrideEnabled bool     `json:"overrideEnabled"`
	DailyLimit      *float64 `json:"dailyLimit"`
	MonthlyLimit    *float64 `json:"monthlyLimit"`
	LifetimeLimit   *float64 `json:"lifetimeLimit"`
	Slots           *int     `json:"slots"`
	Revision        int64    `json:"revision"`
}

type TTSVoice struct {
	StringPKBaseModel
	OwnerUserID       string     `json:"ownerUserId" gorm:"size:100;index"`
	ProviderID        string     `json:"providerId" gorm:"size:64;index"`
	CredentialScope   string     `json:"-" gorm:"size:128;index"`
	Region            string     `json:"-"`
	Workspace         string     `json:"-"`
	TargetModel       string     `json:"targetModel"`
	ProviderVoiceID   string     `json:"-"`
	Kind              string     `json:"kind"`
	Name              string     `json:"name"`
	Tags              string     `json:"tags"`
	Description       string     `json:"description"`
	Parameters        string     `json:"parameters" gorm:"type:text"`
	IsPublic          bool       `json:"isPublic" gorm:"index"`
	Lifecycle         string     `json:"lifecycle" gorm:"size:32;index"`
	ProviderStatus    string     `json:"providerStatus"`
	Revision          int64      `json:"revision"`
	PreviewExpiresAt  *time.Time `json:"previewExpiresAt" gorm:"index"`
	PreviewResourceID string     `json:"previewResourceId,omitempty"`
	SourceResourceID  string     `json:"-"`
}

type ChannelIdentityTTSConfig struct {
	StringPKBaseModel
	IdentityID  string  `json:"identityId" gorm:"size:100;uniqueIndex"`
	VoiceID     string  `json:"voiceId"`
	SystemVoice string  `json:"systemVoice"`
	Instruction string  `json:"instruction"`
	Rate        float64 `json:"rate"`
	Pitch       float64 `json:"pitch"`
	Volume      int     `json:"volume"`
	Revision    int64   `json:"revision"`
}

type TTSJob struct {
	StringPKBaseModel
	Operation         string    `json:"operation" gorm:"size:32;index"`
	QueueOrder        int64     `json:"-" gorm:"default:0;index"`
	RequestKey        string    `json:"-" gorm:"size:128;uniqueIndex"`
	PayerUserID       string    `json:"-" gorm:"size:100;index"`
	ChannelID         string    `json:"channelId,omitempty" gorm:"size:100;index"`
	MessageID         string    `json:"messageId,omitempty" gorm:"size:100;index"`
	MessageRevision   int64     `json:"messageRevision"`
	VoiceID           string    `json:"voiceId,omitempty" gorm:"size:100;index"`
	Snapshot          string    `json:"-" gorm:"type:text"`
	Status            string    `json:"status" gorm:"size:32;index"`
	ProviderRequestID string    `json:"-" gorm:"size:128;index"`
	ReservationID     string    `json:"-" gorm:"size:100;index"`
	EstimatedUnits    int64     `json:"estimatedUnits"`
	UnitPrice         float64   `json:"-"`
	ActualUnits       *int64    `json:"actualUnits"`
	InputTokens       *int64    `json:"inputTokens,omitempty"`
	OutputTokens      *int64    `json:"outputTokens,omitempty"`
	ActualCost        *float64  `json:"-"`
	UsageStatus       string    `json:"usageStatus" gorm:"size:32;index"`
	ResourceID        string    `json:"audioResourceId,omitempty"`
	ErrorCode         string    `json:"errorCode,omitempty"`
	Deadline          time.Time `json:"deadline" gorm:"index"`
	SpoolPath         string    `json:"-"`
	AuditActor        string    `json:"-"`
	AuditNote         string    `json:"-"`
	MediaJSON         string    `json:"-" gorm:"type:text"`
	InputHash         string    `json:"-" gorm:"size:64"`
}

type TTSCache struct {
	StringPKBaseModel
	Fingerprint string    `json:"-" gorm:"size:64;uniqueIndex"`
	ResourceID  string    `json:"-" gorm:"size:100;index"`
	Metadata    string    `json:"-" gorm:"type:text"`
	Complete    bool      `json:"-"`
	ExpiresAt   time.Time `json:"-" gorm:"index"`
}

func MigrateTTS(db *gorm.DB) error {
	// Repair historical nullable columns before applying NOT NULL constraints.
	for _, table := range []any{&AIUsageLogModel{}, &AIUsageLedgerModel{}, &AIQuotaReservationModel{}} {
		if db.Migrator().HasColumn(table, "quota_kind") {
			if err := db.Model(table).Where("quota_kind IS NULL OR quota_kind = ?", "").Update("quota_kind", QuotaKindText).Error; err != nil {
				return err
			}
		}
	}
	if err := db.AutoMigrate(&AIUsageLogModel{}, &AIUsageLedgerModel{}, &AIQuotaReservationModel{}, &TTSUserPolicy{}, &TTSVoice{}, &ChannelIdentityTTSConfig{}, &TTSJob{}, &TTSCache{}); err != nil {
		return err
	}
	for _, table := range []any{&AIUsageLogModel{}, &AIUsageLedgerModel{}, &AIQuotaReservationModel{}} {
		if err := db.Model(table).Where("quota_kind IS NULL OR quota_kind = ?", "").Update("quota_kind", QuotaKindText).Error; err != nil {
			return err
		}
	}
	return nil
}
