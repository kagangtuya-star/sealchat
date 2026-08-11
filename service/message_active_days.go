package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"sealchat/model"
)

func parseActiveDaysMonth(month string) (time.Time, time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01", strings.TrimSpace(month), time.Local)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("month 格式应为 YYYY-MM")
	}
	start := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 1, 0)
	return start, end, nil
}

// ListChannelMessageActiveDays 返回指定月份内有消息的日期列表，格式 YYYY-MM-DD。
func ListChannelMessageActiveDays(channelID, month string) ([]string, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("channel_id 不能为空")
	}
	start, end, err := parseActiveDaysMonth(month)
	if err != nil {
		return nil, err
	}

	type row struct {
		CreatedAt time.Time `gorm:"column:created_at"`
	}
	var rows []row
	query := model.GetDB().Table("messages").
		Select("created_at").
		Where("channel_id = ?", channelID).
		Where("is_deleted = ?", false).
		Where("is_revoked = ?", false)
	query = applyDatabaseTimeComparison(query, "created_at", ">=", start)
	query = applyDatabaseTimeComparison(query, "created_at", "<", end)
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	daySet := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.CreatedAt.IsZero() {
			continue
		}
		daySet[row.CreatedAt.In(time.Local).Format("2006-01-02")] = struct{}{}
	}
	days := make([]string, 0, len(daySet))
	for day := range daySet {
		days = append(days, day)
	}
	sort.Strings(days)
	return days, nil
}
