package model

import (
	"strings"
	"time"
)

// InputStatsPeriodSubject 决定周期聚合按什么主体分组。
type InputStatsPeriodSubject string

const (
	InputStatsPeriodSubjectUser     InputStatsPeriodSubject = "user"
	InputStatsPeriodSubjectIdentity InputStatsPeriodSubject = "identity"
)

// InputStatsPeriodQuery 描述一个排行榜周期的聚合范围。
// Start 包含、End 不包含；DayLocation 决定“自然日”的划分。
type InputStatsPeriodQuery struct {
	Subject     InputStatsPeriodSubject
	ChannelID   string // Subject 为 identity 时必填
	Start       time.Time
	End         time.Time
	DayLocation *time.Location
}

// InputStatsSubjectTotals 是单个主体在周期内的输入统计，口径与个人输入统计一致。
type InputStatsSubjectTotals struct {
	SubjectID     string
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
	SubjectID string    `gorm:"column:subject_id"`
}

type inputStatsPeriodAcc struct {
	speed   typingSpeedAccumulator
	days    int
	lastDay int
}

// InputStatsPeriodAggregate 按 created_at、id 顺序分批扫描周期内的消息，并为每个主体
// 复用输入统计的计入规则与打字速度估算。扫描只读取聚合所需的四个列。
func InputStatsPeriodAggregate(pq InputStatsPeriodQuery) (map[string]*InputStatsSubjectTotals, error) {
	loc := pq.DayLocation
	if loc == nil {
		loc = time.Local
	}
	subjectCol := "m.user_id"
	if pq.Subject == InputStatsPeriodSubjectIdentity {
		subjectCol = "m.sender_identity_id"
	}
	selectClause := strings.Join([]string{
		"m.id",
		"m.created_at",
		visibleCharCountExpr("m.visible_char_count") + " AS char_count",
		subjectCol + " AS subject_id",
	}, ", ")

	// 与个人输入统计默认口径一致：全部 IC/OOC、不含导入消息、排除删除与撤回。
	start := pq.Start.In(time.Local)
	end := pq.End.Add(-time.Nanosecond).In(time.Local)
	f := InputStatsFilter{StartTime: &start, EndTime: &end}

	accs := map[string]*inputStatsPeriodAcc{}
	var lastCreatedAt *time.Time
	lastID := ""
	for {
		q := applyInputStatsMessageRules(db.Table("messages AS m").Select(selectClause), f, "m.").
			Where(subjectCol + " <> ''").
			Order("m.created_at ASC").
			Order("m.id ASC").
			Limit(inputStatsBatchSize)
		if pq.Subject == InputStatsPeriodSubjectIdentity {
			// 角色榜是频道内可见空间：悄悄话的数量不向其他成员暴露。
			q = q.Where("m.channel_id = ?", pq.ChannelID).
				Where("(m.is_whisper = ? OR m.is_whisper IS NULL)", false)
		}
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
			acc := accs[row.SubjectID]
			if acc == nil {
				acc = &inputStatsPeriodAcc{}
				accs[row.SubjectID] = acc
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

	out := make(map[string]*InputStatsSubjectTotals, len(accs))
	for id, acc := range accs {
		out[id] = &InputStatsSubjectTotals{
			SubjectID:     id,
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
