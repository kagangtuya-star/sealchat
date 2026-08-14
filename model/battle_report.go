package model

import (
	"strings"
	"time"
)

type BattleReportStatus string

const (
	BattleReportStatusReady      BattleReportStatus = "ready"
	BattleReportStatusGenerating BattleReportStatus = "generating"
	BattleReportStatusFailed     BattleReportStatus = "failed"
)

type BattleReportModel struct {
	StringPKBaseModel
	ChannelID                string             `json:"channelId" gorm:"size:100;index:idx_battle_report_channel_order,priority:1;index"`
	WorldID                  string             `json:"worldId" gorm:"size:100;index"`
	Title                    string             `json:"title" gorm:"size:255"`
	Content                  string             `json:"content" gorm:"type:text"`
	ContentPreview           string             `json:"contentPreview" gorm:"size:500"`
	PeriodStart              time.Time          `json:"periodStart" gorm:"index"`
	PeriodEnd                time.Time          `json:"periodEnd" gorm:"index"`
	NavigationStartMessageID *string            `json:"-" gorm:"size:100;index"`
	NavigationEndMessageID   *string            `json:"-" gorm:"size:100;index"`
	ContextReportCount       int                `json:"contextReportCount" gorm:"default:3"`
	SortOrder                int                `json:"sortOrder" gorm:"default:0;index:idx_battle_report_channel_order,priority:2"`
	Status                   BattleReportStatus `json:"status" gorm:"size:32;default:'ready';index"`
	ErrorMessage             string             `json:"errorMessage" gorm:"size:500"`
	CreatorID                string             `json:"creatorId" gorm:"size:100;index"`
	UpdaterID                string             `json:"updaterId" gorm:"size:100;index"`
	AISource                 string             `json:"aiSource" gorm:"column:ai_source;size:32"`
	AIProviderID             string             `json:"aiProviderId" gorm:"column:ai_provider_id;size:100"`
	AIModel                  string             `json:"aiModel" gorm:"column:ai_model;size:120"`
	AIFeatureKey             string             `json:"aiFeatureKey" gorm:"column:ai_feature_key;size:64"`
	IsDeleted                bool               `json:"isDeleted" gorm:"default:false;index"`
	DeletedAt                *time.Time         `json:"deletedAt"`
	DeletedBy                string             `json:"deletedBy" gorm:"size:100"`
}

func (*BattleReportModel) TableName() string {
	return "battle_reports"
}

func (m *BattleReportModel) Normalize() {
	if m == nil {
		return
	}
	m.ChannelID = strings.TrimSpace(m.ChannelID)
	m.WorldID = strings.TrimSpace(m.WorldID)
	m.Title = strings.TrimSpace(m.Title)
	m.Content = strings.TrimSpace(m.Content)
	m.ErrorMessage = strings.TrimSpace(m.ErrorMessage)
	m.CreatorID = strings.TrimSpace(m.CreatorID)
	m.UpdaterID = strings.TrimSpace(m.UpdaterID)
	m.AISource = strings.TrimSpace(m.AISource)
	m.AIProviderID = strings.TrimSpace(m.AIProviderID)
	m.AIModel = strings.TrimSpace(m.AIModel)
	m.AIFeatureKey = strings.TrimSpace(m.AIFeatureKey)
	if m.Title == "" {
		m.Title = "未命名战报"
	}
	if m.Status == "" {
		m.Status = BattleReportStatusReady
	}
	if m.ContextReportCount <= 0 {
		m.ContextReportCount = 3
	}
	m.ContentPreview = BuildBattleReportPreview(m.Content, 200)
}

func BuildBattleReportPreview(content string, limit int) string {
	content = strings.TrimSpace(content)
	if content == "" || limit <= 0 {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= limit {
		return content
	}
	return string(runes[:limit])
}
