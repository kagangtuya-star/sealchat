package service

import (
	"fmt"
	"time"

	"sealchat/model"
	aiService "sealchat/service/ai"
	"sealchat/utils"
)

const (
	DatabaseCleanupInterval      = time.Hour
	WebhookEventLogRetentionDays = 7
)

type DatabaseCleanupTool struct {
	Name string
	Run  func(now time.Time) (int64, error)
}

type DatabaseCleanupResult struct {
	Name         string `json:"name"`
	AffectedRows int64  `json:"affectedRows"`
}

type DatabaseCleanupReport struct {
	RanAt             int64                   `json:"ranAt"`
	Results           []DatabaseCleanupResult `json:"results"`
	TotalAffectedRows int64                   `json:"totalAffectedRows"`
}

func DefaultDatabaseCleanupTools() []DatabaseCleanupTool {
	return []DatabaseCleanupTool{
		{
			Name: "webhook_event_logs_retention_7d",
			Run: func(now time.Time) (int64, error) {
				cutoff := now.Add(-time.Duration(WebhookEventLogRetentionDays) * 24 * time.Hour)
				return model.WebhookEventLogCleanupBefore(cutoff)
			},
		},
		{
			Name: "ai_usage_logs_retention",
			Run: func(now time.Time) (int64, error) {
				retentionDays := 30
				if cfg := utils.GetConfig(); cfg != nil {
					retentionDays = utils.NormalizeAIConfig(cfg.AI).LogRetentionDays
				}
				return aiService.AdminCleanupUsageLogs(retentionDays, now)
			},
		},
		{
			Name: "ai_quota_reservation_expired_cleanup",
			Run: func(now time.Time) (int64, error) {
				return model.AIQuotaReservationCleanupExpired(now)
			},
		},
		{
			Name: "digest_window_retention",
			Run: func(now time.Time) (int64, error) {
				var total int64
				for _, windowSeconds := range DigestSupportedWindowSeconds() {
					cutoffStart := digestWindowCleanupCutoff(windowSeconds, DigestWindowRetention)
					if cutoffStart <= 0 {
						continue
					}
					visitorDeleted, err := model.DigestWindowVisitorCleanupBeforeAll(windowSeconds, cutoffStart)
					if err != nil {
						return total, fmt.Errorf("cleanup visitors window=%d: %w", windowSeconds, err)
					}
					speakerDeleted, err := model.DigestWindowSpeakerCleanupBeforeAll(windowSeconds, cutoffStart)
					if err != nil {
						return total, fmt.Errorf("cleanup speakers window=%d: %w", windowSeconds, err)
					}
					total += visitorDeleted + speakerDeleted
				}
				return total, nil
			},
		},
		{
			Name: "channel_latest_read_orphan_cleanup",
			Run: func(now time.Time) (int64, error) {
				return model.ChannelLatestReadCleanupOrphans()
			},
		},
		{
			Name: "theater_retention",
			Run: func(now time.Time) (int64, error) {
				result, err := RunTheaterRetention(now, 500)
				if err != nil {
					return 0, err
				}
				return result.Total(), nil
			},
		},
	}
}

func RunDatabaseCleanupTools(now time.Time, tools []DatabaseCleanupTool) (*DatabaseCleanupReport, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if tools == nil {
		tools = DefaultDatabaseCleanupTools()
	}
	report := &DatabaseCleanupReport{
		RanAt:   now.UnixMilli(),
		Results: make([]DatabaseCleanupResult, 0, len(tools)),
	}
	for _, tool := range tools {
		if tool.Run == nil {
			return report, fmt.Errorf("cleanup tool %s 缺少执行函数", tool.Name)
		}
		name := tool.Name
		if name == "" {
			name = "unnamed_cleanup_tool"
		}
		affectedRows, err := tool.Run(now)
		if err != nil {
			return report, fmt.Errorf("cleanup tool %s failed: %w", name, err)
		}
		report.Results = append(report.Results, DatabaseCleanupResult{
			Name:         name,
			AffectedRows: affectedRows,
		})
		report.TotalAffectedRows += affectedRows
	}
	return report, nil
}

func RunDefaultDatabaseCleanup(now time.Time) (*DatabaseCleanupReport, error) {
	return RunDatabaseCleanupTools(now, nil)
}

func VacuumSQLiteWithMaintenance(now time.Time) (*DatabaseCleanupReport, error) {
	report, err := RunDefaultDatabaseCleanup(now)
	if err != nil {
		return report, err
	}
	if err := model.VacuumSQLite(); err != nil {
		return report, err
	}
	return report, nil
}
