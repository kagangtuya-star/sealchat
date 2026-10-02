package model

import "time"

// One policy row also serializes reservations and settlement for this world.
type TTSWorldPolicy struct {
	WorldID              string     `json:"worldId" gorm:"size:100;primaryKey"`
	Allowlisted          bool       `json:"allowlisted"`
	QuotaOverrideEnabled bool       `json:"quotaOverrideEnabled"`
	DailyLimit           *float64   `json:"dailyLimit"`
	MonthlyLimit         *float64   `json:"monthlyLimit"`
	LifetimeLimit        *float64   `json:"lifetimeLimit"`
	ActivatedBy          string     `json:"activatedBy" gorm:"size:100"`
	ActivatedAt          *time.Time `json:"activatedAt"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	Revision             int64      `json:"-"`
}
