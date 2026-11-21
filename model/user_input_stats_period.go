package model

import (
	"sort"
	"time"
)

// InputStatsUserActiveDays 只读取当前账号在 [Start, End) 内的消息时间。
// 与排行榜共用消息计入规则；自然日由查看者时区划分，不使用数据库日期函数。
func InputStatsUserActiveDays(userID string, pq InputStatsPeriodQuery) ([]string, error) {
	start, end := pq.Start.In(time.Local), pq.End.In(time.Local)
	f := InputStatsFilter{StartTime: &start, ICMode: pq.ICMode}
	excluded := db.Model(&UserModel{}).Select("id").
		Where("id = ? AND (is_bot = ? OR disabled = ?)", userID, true, true)
	rows, err := buildInputStatsMessageQuery(userID, f, "m.created_at").
		Where("m.created_at < ?", end).
		Where("m.user_id NOT IN (?)", excluded).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	loc := pq.DayLocation
	if loc == nil {
		loc = time.UTC
	}
	daySet := map[string]struct{}{}
	for rows.Next() {
		var createdAt time.Time
		if err := rows.Scan(&createdAt); err != nil {
			return nil, err
		}
		daySet[createdAt.In(loc).Format("2006-01-02")] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	days := make([]string, 0, len(daySet))
	for day := range daySet {
		days = append(days, day)
	}
	sort.Strings(days)
	return days, nil
}

// InputStatsPeriodQuery 描述一个排行榜周期的聚合范围。
// Start 包含、End 不包含；DayLocation 决定“自然日”的划分。
type InputStatsPeriodQuery struct {
	Start       time.Time
	End         time.Time
	DayLocation *time.Location
	ICMode      string
}

// InputStatsPeriodTotals 是单个账号在周期内的输入统计，口径与个人输入统计一致。
type InputStatsPeriodTotals struct {
	TotalChars    int64
	TotalMessages int64
	TypingSpeed   float64
	ActiveMinutes float64
	ActiveDays    int
}

type inputStatsPeriodRow struct {
	ID        string    `gorm:"column:id"`
	CreatedAt time.Time `gorm:"column:created_at"`
	CharCount int64     `gorm:"column:char_count"`
	UserID    string    `gorm:"column:user_id"`
}

type inputStatsPeriodAcc struct {
	speed   typingSpeedAccumulator
	days    int
	lastDay int
}

// InputStatsPeriodAggregate 按 created_at、id 顺序分批扫描周期内的消息，并为每个账号
// 复用输入统计的计入规则与打字速度估算。扫描只读取聚合所需的四个列。
func InputStatsPeriodAggregate(pq InputStatsPeriodQuery) (map[string]*InputStatsPeriodTotals, error) {
	loc := pq.DayLocation
	if loc == nil {
		loc = time.Local
	}
	selectClause := "m.id, m.created_at, " + visibleCharCountExpr("m.visible_char_count") + " AS char_count, m.user_id"

	// 沿用个人输入统计的消息过滤；排行榜周期使用严格的排他上界。
	start := pq.Start.In(time.Local)
	end := pq.End.In(time.Local)
	f := InputStatsFilter{StartTime: &start, ICMode: pq.ICMode}

	accs := map[string]*inputStatsPeriodAcc{}
	var lastCreatedAt *time.Time
	lastID := ""
	for {
		q := applyInputStatsMessageRules(db.Table("messages AS m").Select(selectClause), f, "m.").
			Where("m.created_at < ? AND m.user_id <> ''", end).
			Order("m.created_at ASC").
			Order("m.id ASC").
			Limit(inputStatsBatchSize)
		if lastCreatedAt != nil {
			// 先给出可走索引的范围下界，再排除同一时间戳内已处理的行。
			q = q.Where("m.created_at >= ?", *lastCreatedAt).
				Where("(m.created_at > ? OR m.id > ?)", *lastCreatedAt, lastID)
		}

		var batch []inputStatsPeriodRow
		if err := q.Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, row := range batch {
			acc := accs[row.UserID]
			if acc == nil {
				acc = &inputStatsPeriodAcc{}
				accs[row.UserID] = acc
			}
			acc.speed.add(row.CreatedAt, row.CharCount)
			y, m, d := row.CreatedAt.In(loc).Date()
			if day := y*10000 + int(m)*100 + d; day != acc.lastDay {
				acc.days++
				acc.lastDay = day
			}
		}
		if len(batch) < inputStatsBatchSize {
			break
		}
		last := batch[len(batch)-1]
		lastCreatedAtValue := last.CreatedAt
		lastCreatedAt = &lastCreatedAtValue
		lastID = last.ID
	}

	out := make(map[string]*InputStatsPeriodTotals, len(accs))
	for id, acc := range accs {
		out[id] = &InputStatsPeriodTotals{
			TotalChars:    acc.speed.totalChars,
			TotalMessages: int64(acc.speed.msgCount),
			TypingSpeed:   acc.speed.speed(),
			ActiveMinutes: acc.speed.activeMinutes,
			ActiveDays:    acc.days,
		}
	}
	return out, nil
}

// InputStatsExcludedUserIDs 返回不参与排行榜的账号（BOT 与已禁用账号）。
func InputStatsExcludedUserIDs() (map[string]struct{}, error) {
	var ids []string
	if err := db.Model(&UserModel{}).
		Where("is_bot = ? OR disabled = ?", true, true).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

// ensureInputStatsPeriodIndex 为跨用户的周期扫描提供 created_at 范围索引。
func ensureInputStatsPeriodIndex() error {
	if db == nil {
		return nil
	}
	const name = "idx_messages_created_at"
	if db.Migrator().HasIndex(&MessageModel{}, name) {
		return nil
	}
	return db.Exec("CREATE INDEX " + name + " ON messages (created_at)").Error
}
