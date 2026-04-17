package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

const inputStatsBatchSize = 5000

// InputStatsFilter 统计查询的筛选参数
type InputStatsFilter struct {
	StartTime         *time.Time
	EndTime           *time.Time
	ICMode            string   // "ic", "ooc", or "" (all)
	IncludeImported   bool     // 是否包含导入消息
	IncludeWorldIDs   []string // 仅包含这些世界（为空表示不限）
	ExcludeWorldIDs   []string // 排除这些世界
	IncludeChannelIDs []string // 仅包含这些频道（为空表示不限）
	ExcludeChannelIDs []string // 排除这些频道
}

// InputStatsOverview 用户输入统计概览
type InputStatsOverview struct {
	TotalChars     int64   `json:"totalChars"`
	TotalMessages  int64   `json:"totalMessages"`
	AvgCharsPerMsg float64 `json:"avgCharsPerMsg"`
	TypingSpeed    float64 `json:"typingSpeed"` // 字数/活跃分钟数
}

// InputStatsWorldItem 按世界分组的统计数据
type InputStatsWorldItem struct {
	WorldID       string  `json:"worldId"`
	WorldName     string  `json:"worldName"`
	TotalChars    int64   `json:"totalChars"`
	TotalMessages int64   `json:"totalMessages"`
	TypingSpeed   float64 `json:"typingSpeed"`
}

// InputStatsChannelItem 按频道分组的统计数据
type InputStatsChannelItem struct {
	ChannelID     string  `json:"channelId"`
	ChannelName   string  `json:"channelName"`
	TotalChars    int64   `json:"totalChars"`
	TotalMessages int64   `json:"totalMessages"`
	TypingSpeed   float64 `json:"typingSpeed"`
}

// InputStatsTimelinePoint 时间线数据点（用于曲线图）
type InputStatsTimelinePoint struct {
	Date          string `json:"date"`
	TotalChars    int64  `json:"totalChars"`
	TotalMessages int64  `json:"totalMessages"`
}

// InputStatsSessionMessage 用于团次分析的消息记录
type InputStatsSessionMessage struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	CharCount int       `json:"charCount"`
	ChannelID string    `json:"channelId"`
}

// charLengthExpr 返回数据库对应的字符长度函数
func charLengthExpr(colAlias string) string {
	if IsPostgres() {
		return "CHAR_LENGTH(" + colAlias + ")"
	}
	return "LENGTH(" + colAlias + ")"
}

// applyBaseFilter 应用通用筛选条件到消息查询
func applyBaseFilter(q *gorm.DB, userID string, f InputStatsFilter, tableAlias string) *gorm.DB {
	prefix := ""
	if tableAlias != "" {
		prefix = tableAlias + "."
	}

	q = q.Where(prefix+"user_id = ? AND "+prefix+"is_deleted = ? AND "+prefix+"is_revoked = ?", userID, false, false)
	if !f.IncludeImported {
		q = q.Where("("+prefix+"is_imported = ? OR "+prefix+"is_imported IS NULL)", false)
	}

	if f.StartTime != nil {
		q = q.Where(prefix+"created_at >= ?", *f.StartTime)
	}
	if f.EndTime != nil {
		q = q.Where(prefix+"created_at <= ?", *f.EndTime)
	}

	switch f.ICMode {
	case "ic":
		q = q.Where(prefix+"ic_mode = ?", "ic")
	case "ooc":
		q = q.Where(prefix+"ic_mode = ?", "ooc")
	}

	return q
}

// applyChannelWorldFilter 应用世界/频道 include/exclude 过滤
// 需要 messages 表已与 channels 表 JOIN（channels 别名为 cAlias）
func applyChannelWorldFilter(q *gorm.DB, f InputStatsFilter, msgAlias, chAlias string) *gorm.DB {
	if len(f.IncludeChannelIDs) > 0 {
		q = q.Where(msgAlias+".channel_id IN ?", f.IncludeChannelIDs)
	}
	if len(f.ExcludeChannelIDs) > 0 {
		q = q.Where(msgAlias+".channel_id NOT IN ?", f.ExcludeChannelIDs)
	}
	if len(f.IncludeWorldIDs) > 0 {
		q = q.Where(chAlias+".world_id IN ?", f.IncludeWorldIDs)
	}
	if len(f.ExcludeWorldIDs) > 0 {
		q = q.Where(chAlias+".world_id NOT IN ?", f.ExcludeWorldIDs)
	}
	return q
}

// needsChannelJoin 检查是否需要 JOIN channels 表
func needsChannelJoin(f InputStatsFilter) bool {
	return len(f.IncludeWorldIDs) > 0 || len(f.ExcludeWorldIDs) > 0
}

func buildInputStatsMessageQuery(userID string, f InputStatsFilter, selectClause string) *gorm.DB {
	q := db.Table("messages AS m").Select(selectClause)
	q = applyBaseFilter(q, userID, f, "m")

	if needsChannelJoin(f) {
		q = q.Joins("LEFT JOIN channels c ON c.id = m.channel_id")
	}
	if needsChannelJoin(f) || len(f.IncludeChannelIDs) > 0 || len(f.ExcludeChannelIDs) > 0 {
		q = applyChannelWorldFilter(q, f, "m", "c")
	}

	return q
}

type inputStatsMessageRow struct {
	ID        string    `gorm:"column:id"`
	CreatedAt time.Time `gorm:"column:created_at"`
	CharCount int64     `gorm:"column:char_count"`
	ChannelID string    `gorm:"column:channel_id"`
}

func visibleCharCountExpr(colAlias string) string {
	return "COALESCE(" + colAlias + ", 0)"
}

func scanInputStatsMessages(userID string, f InputStatsFilter, handle func([]inputStatsMessageRow) error) error {
	lenExpr := visibleCharCountExpr("m.visible_char_count")
	selectClause := strings.Join([]string{
		"m.id",
		"m.created_at",
		lenExpr + " AS char_count",
		"m.channel_id",
	}, ", ")

	var lastCreatedAt *time.Time
	lastID := ""

	for {
		q := buildInputStatsMessageQuery(userID, f, selectClause).
			Order("m.created_at ASC").
			Order("m.id ASC").
			Limit(inputStatsBatchSize)

		if lastCreatedAt != nil {
			q = q.Where("(m.created_at > ?) OR (m.created_at = ? AND m.id > ?)", *lastCreatedAt, *lastCreatedAt, lastID)
		}

		var batch []inputStatsMessageRow
		if err := q.Find(&batch).Error; err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}

		if err := handle(batch); err != nil {
			return err
		}

		last := batch[len(batch)-1]
		lastCreatedAtValue := last.CreatedAt
		lastCreatedAt = &lastCreatedAtValue
		lastID = last.ID

		if len(batch) < inputStatsBatchSize {
			return nil
		}
	}
}

// UserInputStatsOverall 查询用户输入统计概览
func UserInputStatsOverall(userID string, f InputStatsFilter) (*InputStatsOverview, error) {
	type rawResult struct {
		TotalChars    int64 `gorm:"column:total_chars"`
		TotalMessages int64 `gorm:"column:total_messages"`
	}

	var result rawResult
	lenExpr := visibleCharCountExpr("m.visible_char_count")

	q := db.Table("messages AS m").
		Select("COALESCE(SUM(" + lenExpr + "), 0) AS total_chars, COUNT(*) AS total_messages")

	q = applyBaseFilter(q, userID, f, "m")

	if needsChannelJoin(f) || len(f.IncludeChannelIDs) > 0 || len(f.ExcludeChannelIDs) > 0 {
		if needsChannelJoin(f) {
			q = q.Joins("LEFT JOIN channels c ON c.id = m.channel_id")
		}
		q = applyChannelWorldFilter(q, f, "m", "c")
	}

	if err := q.Scan(&result).Error; err != nil {
		return nil, err
	}

	overview := &InputStatsOverview{
		TotalChars:    result.TotalChars,
		TotalMessages: result.TotalMessages,
	}
	if result.TotalMessages > 0 {
		overview.AvgCharsPerMsg = float64(result.TotalChars) / float64(result.TotalMessages)
	}

	speed, err := calcTypingSpeed(userID, f)
	if err == nil {
		overview.TypingSpeed = speed
	}

	return overview, nil
}

// calcTypingSpeed 计算打字速度（字数/活跃分钟数）
func calcTypingSpeed(userID string, f InputStatsFilter) (float64, error) {
	const activeGapThreshold = 30 * time.Minute

	var totalChars int64
	var activeMinutes float64
	msgCount := 0
	var prevMsg *inputStatsMessageRow

	err := scanInputStatsMessages(userID, f, func(batch []inputStatsMessageRow) error {
		for _, msg := range batch {
			msgCount++
			totalChars += msg.CharCount
			if prevMsg != nil {
				gap := msg.CreatedAt.Sub(prevMsg.CreatedAt)
				if gap <= activeGapThreshold && gap > 0 {
					activeMinutes += gap.Minutes()
				}
			}
			msgCopy := msg
			prevMsg = &msgCopy
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	if msgCount < 2 {
		return 0, nil
	}

	if activeMinutes < 1 {
		return float64(totalChars), nil
	}

	return float64(totalChars) / activeMinutes, nil
}

// UserInputStatsByWorld 按世界分组统计
func UserInputStatsByWorld(userID string, f InputStatsFilter) ([]InputStatsWorldItem, error) {
	type rawRow struct {
		WorldID       string `gorm:"column:world_id"`
		WorldName     string `gorm:"column:world_name"`
		TotalChars    int64  `gorm:"column:total_chars"`
		TotalMessages int64  `gorm:"column:total_messages"`
	}

	lenExpr := visibleCharCountExpr("m.visible_char_count")

	q := db.Table("messages AS m").
		Select("c.world_id AS world_id, w.name AS world_name, COALESCE(SUM(" + lenExpr + "), 0) AS total_chars, COUNT(*) AS total_messages").
		Joins("LEFT JOIN channels c ON c.id = m.channel_id").
		Joins("LEFT JOIN worlds w ON w.id = c.world_id").
		Group("c.world_id, w.name")

	q = applyBaseFilter(q, userID, f, "m")
	q = applyChannelWorldFilter(q, f, "m", "c")

	var rows []rawRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]InputStatsWorldItem, 0, len(rows))
	for _, row := range rows {
		name := row.WorldName
		if name == "" {
			name = "未知世界"
		}
		items = append(items, InputStatsWorldItem{
			WorldID:       row.WorldID,
			WorldName:     name,
			TotalChars:    row.TotalChars,
			TotalMessages: row.TotalMessages,
		})
	}

	return items, nil
}

// UserInputStatsByChannel 按频道分组统计（指定世界）
func UserInputStatsByChannel(userID, worldID string, f InputStatsFilter) ([]InputStatsChannelItem, error) {
	type rawRow struct {
		ChannelID     string `gorm:"column:channel_id"`
		ChannelName   string `gorm:"column:channel_name"`
		TotalChars    int64  `gorm:"column:total_chars"`
		TotalMessages int64  `gorm:"column:total_messages"`
	}

	lenExpr := visibleCharCountExpr("m.visible_char_count")

	q := db.Table("messages AS m").
		Select("m.channel_id AS channel_id, c.name AS channel_name, COALESCE(SUM("+lenExpr+"), 0) AS total_chars, COUNT(*) AS total_messages").
		Joins("LEFT JOIN channels c ON c.id = m.channel_id").
		Where("c.world_id = ?", worldID).
		Group("m.channel_id, c.name")

	q = applyBaseFilter(q, userID, f, "m")
	q = applyChannelWorldFilter(q, f, "m", "c")

	var rows []rawRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]InputStatsChannelItem, 0, len(rows))
	for _, row := range rows {
		name := row.ChannelName
		if name == "" {
			name = "未知频道"
		}
		items = append(items, InputStatsChannelItem{
			ChannelID:     row.ChannelID,
			ChannelName:   name,
			TotalChars:    row.TotalChars,
			TotalMessages: row.TotalMessages,
		})
	}

	return items, nil
}

// UserInputStatsTimeline 按时间粒度统计（用于曲线图）
func UserInputStatsTimeline(userID string, f InputStatsFilter, granularity string) ([]InputStatsTimelinePoint, error) {
	lenExpr := visibleCharCountExpr("m.visible_char_count")

	var dateExpr string
	if IsSQLite() {
		switch granularity {
		case "hour":
			dateExpr = "strftime('%Y-%m-%d %H', m.created_at)"
		default:
			dateExpr = "strftime('%Y-%m-%d', m.created_at)"
		}
	} else if IsPostgres() {
		switch granularity {
		case "hour":
			dateExpr = "TO_CHAR(m.created_at, 'YYYY-MM-DD HH24')"
		default:
			dateExpr = "TO_CHAR(m.created_at, 'YYYY-MM-DD')"
		}
	} else {
		switch granularity {
		case "hour":
			dateExpr = "DATE_FORMAT(m.created_at, '%Y-%m-%d %H')"
		default:
			dateExpr = "DATE_FORMAT(m.created_at, '%Y-%m-%d')"
		}
	}

	q := db.Table("messages AS m").
		Select(dateExpr + " AS date, COALESCE(SUM(" + lenExpr + "), 0) AS total_chars, COUNT(*) AS total_messages").
		Group("date").
		Order("date ASC")

	q = applyBaseFilter(q, userID, f, "m")

	if needsChannelJoin(f) || len(f.IncludeChannelIDs) > 0 || len(f.ExcludeChannelIDs) > 0 {
		if needsChannelJoin(f) {
			q = q.Joins("LEFT JOIN channels c ON c.id = m.channel_id")
		}
		q = applyChannelWorldFilter(q, f, "m", "c")
	}

	var points []InputStatsTimelinePoint
	if err := q.Find(&points).Error; err != nil {
		return nil, err
	}

	return points, nil
}

// UserInputStatsSessionMessages 获取用于团次分析的消息时间和字数列表
func UserInputStatsSessionMessages(userID string, f InputStatsFilter) ([]InputStatsSessionMessage, error) {
	msgs := make([]InputStatsSessionMessage, 0, inputStatsBatchSize)
	err := scanInputStatsMessages(userID, f, func(batch []inputStatsMessageRow) error {
		for _, msg := range batch {
			msgs = append(msgs, InputStatsSessionMessage{
				ID:        msg.ID,
				CreatedAt: msg.CreatedAt,
				CharCount: int(msg.CharCount),
				ChannelID: msg.ChannelID,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return msgs, nil
}
