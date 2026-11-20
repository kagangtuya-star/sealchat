package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"sealchat/model"
)

// TypingHeroesPublicPreferenceKey 用户偏好：为 "true" 时在他人的账号英雄榜中公开自己的昵称与用户名。
const TypingHeroesPublicPreferenceKey = "typing_heroes_public"

const (
	// 活跃输入时间不足该分钟数时，速度估算误差过大，不参与速度排名。
	TypingHeroesSpeedMinActiveMinutes = 10
	typingHeroesWindowRadius          = 5
	typingHeroesMaxLegacyOffset       = 24
	typingHeroesMaxTZOffsetMinutes    = 14 * 60
	typingHeroesMaxTimezoneLength     = 64
	typingHeroesCacheMaxEntries       = 64
	typingHeroesCurrentPeriodTTL      = time.Minute
	typingHeroesPastPeriodTTL         = 10 * time.Minute
	typingHeroesBuildConcurrency      = 2
	typingHeroesBuildRateLimit        = 12
	typingHeroesBuildRateWindow       = time.Minute
)

type TypingHeroesPeriodKind string

const (
	TypingHeroesPeriodDay   TypingHeroesPeriodKind = "day"
	TypingHeroesPeriodWeek  TypingHeroesPeriodKind = "week"
	TypingHeroesPeriodMonth TypingHeroesPeriodKind = "month"
)

type TypingHeroesScope string

const (
	TypingHeroesScopeAll TypingHeroesScope = "all"
	TypingHeroesScopeIC  TypingHeroesScope = "ic"
)

type TypingHeroesMetric string

const (
	TypingHeroesMetricChars    TypingHeroesMetric = "chars"
	TypingHeroesMetricMessages TypingHeroesMetric = "messages"
	TypingHeroesMetricSpeed    TypingHeroesMetric = "speed"
	TypingHeroesMetricDailyAvg TypingHeroesMetric = "dailyAvg"
)

var typingHeroesMetrics = []TypingHeroesMetric{
	TypingHeroesMetricChars, TypingHeroesMetricMessages, TypingHeroesMetricSpeed, TypingHeroesMetricDailyAvg,
}

var (
	ErrTypingHeroesInvalidInput = errors.New("TYPING_HEROES_INVALID_INPUT")
	ErrTypingHeroesBusy         = errors.New("TYPING_HEROES_BUSY")
)

type TypingHeroesQuery struct {
	ViewerID        string
	Period          TypingHeroesPeriodKind
	Anchor          string // YYYY-MM-DD, interpreted in Timezone
	Offset          int    // Legacy fallback when Anchor is empty
	TZOffsetMinutes int
	Timezone        string
	Metric          TypingHeroesMetric
	Scope           TypingHeroesScope
	Now             time.Time
}

type TypingHeroesPeriodInfo struct {
	Kind     TypingHeroesPeriodKind `json:"kind"`
	Start    int64                  `json:"start"`
	End      int64                  `json:"end"`
	TZOffset int                    `json:"tzOffset"`
	Current  bool                   `json:"current"`
}

type TypingHeroesStats struct {
	TotalChars    int64   `json:"totalChars"`
	TotalMessages int64   `json:"totalMessages"`
	TypingSpeed   float64 `json:"typingSpeed"`
	DailyAvg      float64 `json:"dailyAvg"`
	ActiveDays    int     `json:"activeDays"`
	ActiveMinutes int64   `json:"activeMinutes"`
}

type TypingHeroesSelf struct {
	// Status: ranked / no_data / not_eligible
	Status      string            `json:"status"`
	Rank        int               `json:"rank,omitempty"`
	Value       float64           `json:"value"`
	BeatPercent *float64          `json:"beatPercent,omitempty"`
	GapToPrev   *float64          `json:"gapToPrev,omitempty"`
	Stats       TypingHeroesStats `json:"stats"`
}

type TypingHeroesEntry struct {
	// Key 只在当前周期与榜单内稳定，用于前端列表 diff，不可反推主体。
	Key   string  `json:"key"`
	Rank  int     `json:"rank"`
	Value float64 `json:"value"`
	// Kind: self / anonymous / public
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
}

type TypingHeroesBoard struct {
	Period                TypingHeroesPeriodInfo `json:"period"`
	Metric                TypingHeroesMetric     `json:"metric"`
	Scope                 TypingHeroesScope      `json:"scope"`
	Participants          int                    `json:"participants"`
	Me                    TypingHeroesSelf       `json:"me"`
	Entries               []TypingHeroesEntry    `json:"entries"`
	IsPublic              bool                   `json:"isPublic"`
	SpeedMinActiveMinutes int                    `json:"speedMinActiveMinutes"`
	GeneratedAt           int64                  `json:"generatedAt"`
}

const typingHeroesDateLayout = "2006-01-02"

type TypingHeroesActiveDaysQuery struct {
	ViewerID        string
	Month           string
	Scope           TypingHeroesScope
	Timezone        string
	TZOffsetMinutes int
	Now             time.Time
}

type TypingHeroesActiveDays struct {
	Month string   `json:"month"`
	Days  []string `json:"days"`
}

// TypingHeroesActiveDaysGet 是当前用户的轻量查询，不构建或消耗排行榜快照与频控。
func TypingHeroesActiveDaysGet(input TypingHeroesActiveDaysQuery) (*TypingHeroesActiveDays, error) {
	month := strings.TrimSpace(input.Month)
	parsed, err := time.Parse("2006-01", month)
	if err != nil || parsed.Year() < 1 || parsed.Format("2006-01") != month {
		return nil, ErrTypingHeroesInvalidInput
	}
	q := TypingHeroesQuery{
		ViewerID: input.ViewerID, Period: TypingHeroesPeriodMonth, Anchor: month + "-01",
		Scope: input.Scope, Metric: TypingHeroesMetricMessages,
		Timezone: input.Timezone, TZOffsetMinutes: input.TZOffsetMinutes, Now: input.Now,
	}
	if err := normalizeTypingHeroesQuery(&q); err != nil {
		return nil, err
	}
	loc, _, err := typingHeroesLocation(&q)
	if err != nil {
		return nil, err
	}
	start, end, err := typingHeroesPeriodRange(q, loc)
	if err != nil {
		return nil, err
	}
	icMode := ""
	if q.Scope == TypingHeroesScopeIC {
		icMode = "ic"
	}
	days, err := model.InputStatsUserActiveDays(q.ViewerID, model.InputStatsPeriodQuery{
		Start: start, End: end, DayLocation: loc, ICMode: icMode,
	})
	if err != nil {
		return nil, err
	}
	return &TypingHeroesActiveDays{Month: month, Days: days}, nil
}

// typingHeroesPeriodRange resolves a civil date to [start, end) in the viewer's timezone.
// Do calendar arithmetic in UTC before translating the two boundaries independently.
func typingHeroesPeriodRange(q TypingHeroesQuery, loc *time.Location) (time.Time, time.Time, error) {
	y, m, d := q.Now.In(loc).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	date := today
	if q.Anchor != "" {
		var err error
		date, err = time.Parse(typingHeroesDateLayout, q.Anchor)
		if err != nil || date.Format(typingHeroesDateLayout) != q.Anchor || date.Year() < 1 || date.After(today) {
			return time.Time{}, time.Time{}, ErrTypingHeroesInvalidInput
		}
	}
	var endDate time.Time
	switch q.Period {
	case TypingHeroesPeriodDay:
		date = date.AddDate(0, 0, -q.Offset)
		endDate = date.AddDate(0, 0, 1)
	case TypingHeroesPeriodWeek:
		weekday := (int(date.Weekday()) + 6) % 7
		date = date.AddDate(0, 0, -weekday-7*q.Offset)
		endDate = date.AddDate(0, 0, 7)
	case TypingHeroesPeriodMonth:
		date = time.Date(date.Year(), date.Month()-time.Month(q.Offset), 1, 0, 0, 0, 0, time.UTC)
		endDate = date.AddDate(0, 1, 0)
	}
	start, end := typingHeroesLocalBoundary(date, loc), typingHeroesLocalBoundary(endDate, loc)
	if start.After(q.Now) || !start.Before(end) {
		return time.Time{}, time.Time{}, ErrTypingHeroesInvalidInput
	}
	return start, end, nil
}

// Walk nearby offsets to find the first instant on or after civil midnight.
// Some zones skip or repeat midnight itself; time.Date in that zone can choose either side.
func typingHeroesLocalBoundary(date time.Time, loc *time.Location) time.Time {
	at := date.Add(-24 * time.Hour).In(loc)
	for {
		_, offset := at.Zone()
		_, until := at.ZoneBounds()
		midnight := date.Add(-time.Duration(offset) * time.Second).In(loc)
		if until.IsZero() || midnight.Before(until) {
			if midnight.Before(at) {
				return at
			}
			return midnight
		}
		at = until
	}
}

func typingHeroesLocation(q *TypingHeroesQuery) (*time.Location, string, error) {
	if q.Timezone != "" {
		loc, err := time.LoadLocation(q.Timezone)
		if err != nil {
			return nil, "", ErrTypingHeroesInvalidInput
		}
		return loc, "tz:" + loc.String(), nil
	}
	return time.FixedZone("", q.TZOffsetMinutes*60), fmt.Sprintf("offset:%d", q.TZOffsetMinutes), nil
}

func normalizeTypingHeroesQuery(q *TypingHeroesQuery) error {
	q.ViewerID = strings.TrimSpace(q.ViewerID)
	q.Anchor = strings.TrimSpace(q.Anchor)
	q.Timezone = strings.TrimSpace(q.Timezone)
	if q.ViewerID == "" {
		return ErrTypingHeroesInvalidInput
	}
	if q.Period != TypingHeroesPeriodDay && q.Period != TypingHeroesPeriodWeek && q.Period != TypingHeroesPeriodMonth {
		return ErrTypingHeroesInvalidInput
	}
	if q.Offset < 0 || q.Offset > typingHeroesMaxLegacyOffset || (q.Anchor != "" && q.Offset != 0) {
		return ErrTypingHeroesInvalidInput
	}
	if q.Timezone != "" {
		if q.Timezone == "Local" || len(q.Timezone) > typingHeroesMaxTimezoneLength {
			return ErrTypingHeroesInvalidInput
		}
	} else {
		if q.TZOffsetMinutes < -typingHeroesMaxTZOffsetMinutes || q.TZOffsetMinutes > typingHeroesMaxTZOffsetMinutes || q.TZOffsetMinutes%15 != 0 {
			return ErrTypingHeroesInvalidInput
		}
	}
	valid := false
	for _, metric := range typingHeroesMetrics {
		if q.Metric == metric {
			valid = true
		}
	}
	if !valid {
		return ErrTypingHeroesInvalidInput
	}
	if q.Scope == "" {
		q.Scope = TypingHeroesScopeAll
	}
	if q.Scope != TypingHeroesScopeAll && q.Scope != TypingHeroesScopeIC {
		return ErrTypingHeroesInvalidInput
	}
	if q.Now.IsZero() {
		q.Now = time.Now()
	}
	return nil
}

// TypingHeroesBoardGet 生成查看者视角的排行榜：只返回自己的成绩与附近的窗口，
// 他人身份在服务端完成匿名化。
func TypingHeroesBoardGet(q TypingHeroesQuery) (*TypingHeroesBoard, error) {
	if err := normalizeTypingHeroesQuery(&q); err != nil {
		return nil, err
	}
	loc, locationKey, err := typingHeroesLocation(&q)
	if err != nil {
		return nil, err
	}
	start, end, err := typingHeroesPeriodRange(q, loc)
	if err != nil {
		return nil, err
	}
	_, periodOffsetSeconds := start.Zone()

	snap, err := typingHeroesSnapshotGet(typingHeroesSnapshotRange{
		scope: q.Scope, start: start, end: end, loc: loc, locationKey: locationKey,
	}, q.ViewerID, q.Now)
	if err != nil {
		return nil, err
	}

	board := &TypingHeroesBoard{
		Period: TypingHeroesPeriodInfo{
			Kind: q.Period, Start: start.UnixMilli(), End: end.UnixMilli(),
			TZOffset: periodOffsetSeconds / 60, Current: !q.Now.Before(start) && q.Now.Before(end),
		},
		Metric:                q.Metric,
		Scope:                 q.Scope,
		SpeedMinActiveMinutes: TypingHeroesSpeedMinActiveMinutes,
		GeneratedAt:           snap.generatedAt.UnixMilli(),
		Entries:               []TypingHeroesEntry{},
	}

	rows := snap.ranked[q.Metric]
	board.Participants = len(rows)
	board.Me = typingHeroesSelfResult(snap, q.Metric, q.ViewerID)

	lo, hi := typingHeroesWindow(len(rows), snap.position[q.Metric][q.ViewerID], board.Me.Status == "ranked")
	window := rows[lo:hi]

	public, err := typingHeroesPublicUsers(q.ViewerID, window)
	if err != nil {
		return nil, err
	}
	board.IsPublic = public[q.ViewerID] != nil
	for _, row := range window {
		entry := TypingHeroesEntry{Key: row.key, Rank: row.rank, Value: row.value, Kind: "anonymous", Name: row.label}
		if row.userID == q.ViewerID {
			entry.Kind = "self"
			entry.Name = ""
		} else if u := public[row.userID]; u != nil {
			entry.Kind = "public"
			entry.Name = u.name
			entry.Username = u.username
		}
		board.Entries = append(board.Entries, entry)
	}
	return board, nil
}

func typingHeroesSelfResult(snap *typingHeroesSnapshot, metric TypingHeroesMetric, userID string) TypingHeroesSelf {
	me := TypingHeroesSelf{Status: "no_data"}
	totals := snap.totals[userID]
	if totals == nil {
		return me
	}
	me.Stats = typingHeroesStatsOf(totals)
	me.Value = typingHeroesMetricValue(totals, metric)
	pos, ok := snap.position[metric][userID]
	if !ok {
		me.Status = "not_eligible"
		return me
	}
	rows := snap.ranked[metric]
	row := rows[pos]
	me.Status = "ranked"
	me.Rank = row.rank
	me.Value = row.value
	// 并列者共享名次；“超过”只计算严格低于自己的参与者。
	lastTie := pos
	for lastTie+1 < len(rows) && rows[lastTie+1].value == row.value {
		lastTie++
	}
	if len(rows) > 1 {
		beat := math.Round(float64(len(rows)-1-lastTie)/float64(len(rows)-1)*1000) / 10
		me.BeatPercent = &beat
	}
	if row.rank > 1 {
		gap := roundTypingHeroesValue(rows[row.rank-2].value-row.value, metric)
		me.GapToPrev = &gap
	}
	return me
}

// typingHeroesWindow 返回以自己为中心、上下各 radius 名的窗口；未上榜时返回榜首窗口。
func typingHeroesWindow(n, pos int, ranked bool) (int, int) {
	size := typingHeroesWindowRadius*2 + 1
	if n <= size {
		return 0, n
	}
	if !ranked {
		return 0, size
	}
	lo := pos - typingHeroesWindowRadius
	if lo < 0 {
		lo = 0
	}
	if lo+size > n {
		lo = n - size
	}
	return lo, lo + size
}

func typingHeroesStatsOf(t *model.InputStatsPeriodTotals) TypingHeroesStats {
	return TypingHeroesStats{
		TotalChars:    t.TotalChars,
		TotalMessages: t.TotalMessages,
		TypingSpeed:   roundTypingHeroesValue(t.TypingSpeed, TypingHeroesMetricSpeed),
		DailyAvg:      roundTypingHeroesValue(typingHeroesDailyAvg(t), TypingHeroesMetricDailyAvg),
		ActiveDays:    t.ActiveDays,
		ActiveMinutes: int64(t.ActiveMinutes + 0.5),
	}
}

func typingHeroesDailyAvg(t *model.InputStatsPeriodTotals) float64 {
	if t.ActiveDays <= 0 {
		return 0
	}
	return float64(t.TotalChars) / float64(t.ActiveDays)
}

func typingHeroesMetricValue(t *model.InputStatsPeriodTotals, metric TypingHeroesMetric) float64 {
	switch metric {
	case TypingHeroesMetricMessages:
		return float64(t.TotalMessages)
	case TypingHeroesMetricSpeed:
		return roundTypingHeroesValue(t.TypingSpeed, metric)
	case TypingHeroesMetricDailyAvg:
		return roundTypingHeroesValue(typingHeroesDailyAvg(t), metric)
	default:
		return float64(t.TotalChars)
	}
}

func typingHeroesEligible(t *model.InputStatsPeriodTotals, metric TypingHeroesMetric) bool {
	switch metric {
	case TypingHeroesMetricMessages:
		return t.TotalMessages > 0
	case TypingHeroesMetricSpeed:
		return t.ActiveMinutes >= TypingHeroesSpeedMinActiveMinutes && t.TypingSpeed > 0
	default:
		return t.TotalChars > 0
	}
}

// roundTypingHeroesValue 按界面展示精度取整，使看起来相同的成绩按并列处理。
func roundTypingHeroesValue(v float64, metric TypingHeroesMetric) float64 {
	if metric == TypingHeroesMetricSpeed || metric == TypingHeroesMetricDailyAvg {
		return math.Round(v*10) / 10
	}
	return math.Round(v)
}

// ---- 快照与缓存 ----

type typingHeroesSnapshotRange struct {
	scope       TypingHeroesScope
	start       time.Time
	end         time.Time
	loc         *time.Location
	locationKey string
}

func (s typingHeroesSnapshotRange) cacheKey() string {
	return fmt.Sprintf("%s|%d|%d|%s", s.scope, s.start.Unix(), s.end.Unix(), s.locationKey)
}

type typingHeroesRankRow struct {
	userID string
	value  float64
	rank   int
	tie    string
	key    string
	label  string
}

type typingHeroesSnapshot struct {
	totals      map[string]*model.InputStatsPeriodTotals
	ranked      map[TypingHeroesMetric][]typingHeroesRankRow
	position    map[TypingHeroesMetric]map[string]int
	generatedAt time.Time
}

type typingHeroesCacheEntry struct {
	ready   chan struct{}
	snap    *typingHeroesSnapshot
	err     error
	expires time.Time
}

// 进程内缓存：同一周期范围的并发请求只计算一次，多实例部署时各实例独立计算。
var typingHeroesCache = struct {
	sync.Mutex
	entries map[string]*typingHeroesCacheEntry
}{entries: map[string]*typingHeroesCacheEntry{}}

type typingHeroesBuildRateState struct {
	windowStart time.Time
	count       int
}

// 重型快照构建的并发与频率保护只作用于当前进程；多实例部署各自独立。
var (
	typingHeroesBuildSlots = make(chan struct{}, typingHeroesBuildConcurrency)
	typingHeroesBuildRate  = struct {
		sync.Mutex
		entries     map[string]typingHeroesBuildRateState
		nextCleanup time.Time
	}{entries: map[string]typingHeroesBuildRateState{}}
)

func typingHeroesConsumeBuildRate(viewerID string, now time.Time) bool {
	typingHeroesBuildRate.Lock()
	defer typingHeroesBuildRate.Unlock()
	if now.After(typingHeroesBuildRate.nextCleanup) {
		for key, state := range typingHeroesBuildRate.entries {
			if now.Sub(state.windowStart) >= typingHeroesBuildRateWindow {
				delete(typingHeroesBuildRate.entries, key)
			}
		}
		typingHeroesBuildRate.nextCleanup = now.Add(typingHeroesBuildRateWindow)
	}
	state := typingHeroesBuildRate.entries[viewerID]
	if state.windowStart.IsZero() || now.Sub(state.windowStart) >= typingHeroesBuildRateWindow {
		typingHeroesBuildRate.entries[viewerID] = typingHeroesBuildRateState{windowStart: now, count: 1}
		return true
	}
	if state.count >= typingHeroesBuildRateLimit {
		return false
	}
	state.count++
	typingHeroesBuildRate.entries[viewerID] = state
	return true
}

func typingHeroesSnapshotGet(scope typingHeroesSnapshotRange, viewerID string, now time.Time) (*typingHeroesSnapshot, error) {
	key := scope.cacheKey()
	typingHeroesCache.Lock()
	entry := typingHeroesCache.entries[key]
	if entry != nil {
		select {
		case <-entry.ready:
			if time.Now().After(entry.expires) {
				entry = nil
			}
		default:
		}
	}
	if entry != nil {
		typingHeroesCache.Unlock()
		<-entry.ready
		return entry.snap, entry.err
	}
	entry = &typingHeroesCacheEntry{ready: make(chan struct{})}
	typingHeroesCacheEvictLocked()
	if len(typingHeroesCache.entries) >= typingHeroesCacheMaxEntries {
		typingHeroesCache.Unlock()
		return nil, ErrTypingHeroesBusy
	}
	typingHeroesCache.entries[key] = entry
	typingHeroesCache.Unlock()

	select {
	case typingHeroesBuildSlots <- struct{}{}:
		defer func() { <-typingHeroesBuildSlots }()
	default:
		typingHeroesRejectCacheEntry(key, entry, ErrTypingHeroesBusy)
		return nil, ErrTypingHeroesBusy
	}
	if !typingHeroesConsumeBuildRate(viewerID, time.Now()) {
		typingHeroesRejectCacheEntry(key, entry, ErrTypingHeroesBusy)
		return nil, ErrTypingHeroesBusy
	}

	entry.snap, entry.err = buildTypingHeroesSnapshot(scope)
	ttl := typingHeroesPastPeriodTTL
	if now.Before(scope.end) {
		ttl = typingHeroesCurrentPeriodTTL
	}
	typingHeroesCache.Lock()
	entry.expires = time.Now().Add(ttl)
	if entry.err != nil && typingHeroesCache.entries[key] == entry {
		delete(typingHeroesCache.entries, key)
	}
	typingHeroesCache.Unlock()
	close(entry.ready)
	return entry.snap, entry.err
}

func typingHeroesRejectCacheEntry(key string, entry *typingHeroesCacheEntry, err error) {
	entry.err = err
	typingHeroesCache.Lock()
	if typingHeroesCache.entries[key] == entry {
		delete(typingHeroesCache.entries, key)
	}
	typingHeroesCache.Unlock()
	close(entry.ready)
}

func typingHeroesCacheEvictLocked() {
	if len(typingHeroesCache.entries) < typingHeroesCacheMaxEntries {
		return
	}
	now := time.Now()
	var oldestKey string
	var oldest time.Time
	for key, entry := range typingHeroesCache.entries {
		select {
		case <-entry.ready:
		default:
			continue
		}
		if now.After(entry.expires) {
			delete(typingHeroesCache.entries, key)
			continue
		}
		if oldestKey == "" || entry.expires.Before(oldest) {
			oldestKey, oldest = key, entry.expires
		}
	}
	if len(typingHeroesCache.entries) >= typingHeroesCacheMaxEntries && oldestKey != "" {
		delete(typingHeroesCache.entries, oldestKey)
	}
}

func typingHeroesCacheReset() {
	typingHeroesCache.Lock()
	typingHeroesCache.entries = map[string]*typingHeroesCacheEntry{}
	typingHeroesCache.Unlock()
	typingHeroesBuildRate.Lock()
	typingHeroesBuildRate.entries = map[string]typingHeroesBuildRateState{}
	typingHeroesBuildRate.nextCleanup = time.Time{}
	typingHeroesBuildRate.Unlock()
}

func buildTypingHeroesSnapshot(scope typingHeroesSnapshotRange) (*typingHeroesSnapshot, error) {
	icMode := ""
	if scope.scope == TypingHeroesScopeIC {
		icMode = "ic"
	}
	totals, err := model.InputStatsPeriodAggregate(model.InputStatsPeriodQuery{
		Start: scope.start, End: scope.end, DayLocation: scope.loc, ICMode: icMode,
	})
	if err != nil {
		return nil, err
	}
	excludedUsers, err := model.InputStatsExcludedUserIDs()
	if err != nil {
		return nil, err
	}
	snap := &typingHeroesSnapshot{
		totals:      totals,
		ranked:      map[TypingHeroesMetric][]typingHeroesRankRow{},
		position:    map[TypingHeroesMetric]map[string]int{},
		generatedAt: time.Now(),
	}
	for id := range excludedUsers {
		delete(totals, id)
	}

	seed := fmt.Sprintf("%s|%s", scope.cacheKey(), "typing-heroes")
	base := make([]typingHeroesRankRow, 0, len(totals))
	for id := range totals {
		digest := typingHeroesDigest(seed, id)
		base = append(base, typingHeroesRankRow{
			userID: id,
			tie:    hex.EncodeToString(digest[:8]),
			key:    hex.EncodeToString(digest[8:16]),
			label:  typingHeroesAnonymousLabel(digest[16:]),
		})
	}
	for _, metric := range typingHeroesMetrics {
		rows := make([]typingHeroesRankRow, 0, len(base))
		for _, row := range base {
			t := totals[row.userID]
			if !typingHeroesEligible(t, metric) {
				continue
			}
			row.value = typingHeroesMetricValue(t, metric)
			rows = append(rows, row)
		}
		// 并列时按周期内稳定的随机键排序，刷新时不跳动，也不泄露主体 ID 顺序。
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].value != rows[j].value {
				return rows[i].value > rows[j].value
			}
			return rows[i].tie < rows[j].tie
		})
		position := make(map[string]int, len(rows))
		for i := range rows {
			if i > 0 && rows[i].value == rows[i-1].value {
				rows[i].rank = rows[i-1].rank
			} else {
				rows[i].rank = i + 1
			}
			position[rows[i].userID] = i
		}
		snap.ranked[metric] = rows
		snap.position[metric] = position
	}
	return snap, nil
}

// ---- 匿名化 ----

var (
	typingHeroesSecretOnce sync.Once
	typingHeroesSecret     []byte
)

// typingHeroesDigest 以进程级随机密钥对“榜单范围 + 账号”做 HMAC：同一榜单内稳定可读，
// 换周期或重启后不可关联，也无法通过对已知用户 ID 求 hash 反查。
func typingHeroesDigest(seed, userID string) []byte {
	typingHeroesSecretOnce.Do(func() {
		typingHeroesSecret = make([]byte, 32)
		if _, err := rand.Read(typingHeroesSecret); err != nil {
			panic(fmt.Errorf("typing heroes secret: %w", err))
		}
	})
	mac := hmac.New(sha256.New, typingHeroesSecret)
	mac.Write([]byte(seed))
	mac.Write([]byte{0})
	mac.Write([]byte(userID))
	return mac.Sum(nil)
}

var typingHeroesAnonAdjectives = []string{
	"疾风", "静默", "灵巧", "勤勉", "夜行", "晨光", "迅捷", "沉稳",
	"闪烁", "漫游", "低语", "热忱", "专注", "远行", "微光", "好奇",
	"从容", "轻快", "执笔", "拂晓", "星夜", "雾中", "林间", "海边",
}

var typingHeroesAnonNouns = []string{
	"羽毛笔", "打字机", "信鸽", "书虫", "墨鱼", "萤火虫", "狐狸", "水獭",
	"猫头鹰", "松鼠", "渡鸦", "刺猬", "海豚", "浣熊", "鲸鱼", "云雀",
	"仓鼠", "白鹭", "蜂鸟", "雪豹", "鹦鹉", "河狸", "海獭", "麻雀",
}

func typingHeroesAnonymousLabel(digest []byte) string {
	n := binary.BigEndian.Uint32(digest[:4])
	adjective := typingHeroesAnonAdjectives[n%uint32(len(typingHeroesAnonAdjectives))]
	noun := typingHeroesAnonNouns[(n/uint32(len(typingHeroesAnonAdjectives)))%uint32(len(typingHeroesAnonNouns))]
	return fmt.Sprintf("%s的%s·%02X", adjective, noun, digest[4])
}

type typingHeroesPublicUser struct {
	name     string
	username string
}

// typingHeroesPublicUsers 只为查看者本人与窗口内主动公开的账号读取昵称与用户名。
func typingHeroesPublicUsers(viewerID string, window []typingHeroesRankRow) (map[string]*typingHeroesPublicUser, error) {
	ids := make([]string, 0, len(window)+1)
	ids = append(ids, viewerID)
	for _, row := range window {
		if row.userID != viewerID {
			ids = append(ids, row.userID)
		}
	}
	var publicIDs []string
	if err := model.GetDB().Model(&model.UserPreferenceModel{}).
		Where("user_id IN ? AND pref_key = ? AND pref_value = ?", ids, TypingHeroesPublicPreferenceKey, "true").
		Pluck("user_id", &publicIDs).Error; err != nil {
		return nil, err
	}
	out := make(map[string]*typingHeroesPublicUser, len(publicIDs))
	if len(publicIDs) == 0 {
		return out, nil
	}
	var users []model.UserModel
	if err := model.GetDB().Select("id, nickname, username").Where("id IN ?", publicIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		name := strings.TrimSpace(u.Nickname)
		if name == "" {
			name = u.Username
		}
		out[u.ID] = &typingHeroesPublicUser{name: name, username: u.Username}
	}
	return out, nil
}
