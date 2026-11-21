package service

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
)

const (
	adminWorldStatisticsTTL          = 60 * time.Second
	adminWorldStatisticsFailureRetry = 15 * time.Second
)

type AdminWorldStatisticsQuery struct {
	Page     int
	PageSize int
	Keyword  string
	SortBy   string
	Order    string
}

type adminWorldStatisticsItem struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Visibility      string    `json:"visibility"`
	OwnerID         string    `json:"ownerId"`
	OwnerNickname   string    `json:"ownerNickname"`
	CreatedAt       time.Time `json:"createdAt"`
	MemberCount     int64     `json:"memberCount"`
	ChannelCount    int64     `json:"channelCount"`
	MessageCount    int64     `json:"messageCount"`
	MessageChars    int64     `json:"messageChars"`
	MessageCharsIC  int64     `json:"messageCharsIc"`
	MessageCharsOOC int64     `json:"messageCharsOoc"`
	LastActiveAt    int64     `json:"lastActiveAt"`
}

type AdminWorldStatisticsItem struct {
	adminWorldStatisticsItem
	OnlineCount int `json:"onlineCount"`
}

type AdminWorldStatisticsSummary struct {
	WorldCount      int   `json:"worldCount"`
	MemberCount     int64 `json:"memberCount"`
	ChannelCount    int64 `json:"channelCount"`
	MessageCount    int64 `json:"messageCount"`
	MessageCountIC  int64 `json:"messageCountIc"`
	MessageCountOOC int64 `json:"messageCountOoc"`
	MessageChars    int64 `json:"messageChars"`
	MessageCharsIC  int64 `json:"messageCharsIc"`
	MessageCharsOOC int64 `json:"messageCharsOoc"`
}

type AdminWorldStatisticsDataQuality struct {
	MessageChars string `json:"messageChars"`
}

type AdminWorldStatisticsResult struct {
	GeneratedAt int64                           `json:"generatedAt"`
	Total       int                             `json:"total"`
	Page        int                             `json:"page"`
	PageSize    int                             `json:"pageSize"`
	SortBy      string                          `json:"sortBy"`
	Order       string                          `json:"order"`
	DataQuality AdminWorldStatisticsDataQuality `json:"dataQuality"`
	Summary     AdminWorldStatisticsSummary     `json:"summary"`
	Items       []AdminWorldStatisticsItem      `json:"items"`
}

type adminWorldStatisticsEntry struct {
	item       adminWorldStatisticsItem
	searchText [4]string
}

type adminWorldStatisticsSnapshot struct {
	generatedAt time.Time
	entries     []adminWorldStatisticsEntry
	summary     AdminWorldStatisticsSummary
	dataQuality AdminWorldStatisticsDataQuality
}

type adminWorldStatisticsRefresh struct {
	done     chan struct{}
	snapshot *adminWorldStatisticsSnapshot
	err      error
}

type adminWorldStatisticsCache struct {
	now   func() time.Time
	build func() (*adminWorldStatisticsSnapshot, error)

	mu         sync.Mutex
	snapshot   *adminWorldStatisticsSnapshot
	refresh    *adminWorldStatisticsRefresh
	retryAfter time.Time
}

var adminWorldStatistics = adminWorldStatisticsCache{
	now: time.Now,
	build: func() (*adminWorldStatisticsSnapshot, error) {
		return buildAdminWorldStatisticsSnapshot(model.GetDB())
	},
}

// ListAdminWorldStatistics is for callers that have already verified platform admin access.
func ListAdminWorldStatistics(query AdminWorldStatisticsQuery, countOnline func() map[string]int) (*AdminWorldStatisticsResult, error) {
	return adminWorldStatistics.list(query, countOnline)
}

func (c *adminWorldStatisticsCache) get() (*adminWorldStatisticsSnapshot, error) {
	c.mu.Lock()
	now := c.now()
	if snapshot := c.snapshot; snapshot != nil {
		if now.Sub(snapshot.generatedAt) < adminWorldStatisticsTTL || now.Before(c.retryAfter) {
			c.mu.Unlock()
			return snapshot, nil
		}
	}
	if refresh := c.refresh; refresh != nil {
		c.mu.Unlock()
		<-refresh.done
		return refresh.snapshot, refresh.err
	}
	refresh := &adminWorldStatisticsRefresh{done: make(chan struct{})}
	c.refresh = refresh
	c.mu.Unlock()

	snapshot, err := c.build()

	c.mu.Lock()
	if err == nil {
		snapshot.generatedAt = c.now()
		c.snapshot = snapshot
		c.retryAfter = time.Time{}
	} else if c.snapshot != nil {
		snapshot = c.snapshot
		err = nil
		c.retryAfter = c.now().Add(adminWorldStatisticsFailureRetry)
	}
	refresh.snapshot, refresh.err = snapshot, err
	c.refresh = nil
	close(refresh.done)
	c.mu.Unlock()
	return snapshot, err
}

func (c *adminWorldStatisticsCache) list(query AdminWorldStatisticsQuery, countOnline func() map[string]int) (*AdminWorldStatisticsResult, error) {
	snapshot, err := c.get()
	if err != nil {
		return nil, err
	}
	return snapshot.list(query, countOnline()), nil
}

func buildAdminWorldStatisticsSnapshot(db *gorm.DB) (*adminWorldStatisticsSnapshot, error) {
	var worlds []struct {
		ID, Name, Description, Visibility, OwnerID, OwnerUsername, OwnerNickname string
		CreatedAt                                                                time.Time
	}
	if err := db.Table("worlds AS w").
		Select("w.id, w.name, w.description, w.visibility, w.owner_id, w.created_at, u.username AS owner_username, u.nickname AS owner_nickname").
		Joins("LEFT JOIN users u ON u.id = w.owner_id").
		Where("w.status NOT IN ?", []string{"deleted", "archived"}).
		Scan(&worlds).Error; err != nil {
		return nil, fmt.Errorf("load worlds: %w", err)
	}

	state, err := model.ReadMessageVisibleCharCountBackfillState(db)
	if err != nil {
		return nil, fmt.Errorf("load visible character count backfill state: %w", err)
	}
	snapshot := &adminWorldStatisticsSnapshot{
		entries:     make([]adminWorldStatisticsEntry, 0, len(worlds)),
		summary:     AdminWorldStatisticsSummary{WorldCount: len(worlds)},
		dataQuality: AdminWorldStatisticsDataQuality{MessageChars: worldStatisticsMessageCharsQuality(state)},
	}
	if len(worlds) == 0 {
		return snapshot, nil
	}
	worldIDs := make([]string, 0, len(worlds))
	for _, world := range worlds {
		worldIDs = append(worldIDs, world.ID)
	}

	type countRow struct {
		WorldID string
		Total   int64
	}
	var members, channels []countRow
	if err := db.Model(&model.WorldMemberModel{}).
		Select("world_id, COUNT(DISTINCT user_id) AS total").
		Where("world_id IN ?", worldIDs).
		Group("world_id").
		Scan(&members).Error; err != nil {
		return nil, fmt.Errorf("count world members: %w", err)
	}
	if err := db.Model(&model.ChannelModel{}).
		Select("world_id, COUNT(*) AS total").
		Where("world_id IN ? AND is_private = ? AND status NOT IN ?", worldIDs, false, []string{model.ChannelStatusDeleted, model.ChannelStatusArchived}).
		Group("world_id").
		Scan(&channels).Error; err != nil {
		return nil, fmt.Errorf("count world channels: %w", err)
	}
	messages, err := model.CountWorldMessages(db, worldIDs)
	if err != nil {
		return nil, fmt.Errorf("count world messages: %w", err)
	}

	memberCounts := make(map[string]int64, len(members))
	for _, row := range members {
		memberCounts[row.WorldID] = row.Total
	}
	channelCounts := make(map[string]int64, len(channels))
	for _, row := range channels {
		channelCounts[row.WorldID] = row.Total
	}
	messageStats := make(map[string]model.WorldMessageStatistics, len(messages))
	for _, row := range messages {
		messageStats[row.WorldID] = row
	}

	for _, world := range worlds {
		message := messageStats[world.ID]
		ownerName := world.OwnerNickname
		if ownerName == "" {
			ownerName = world.OwnerUsername
		}
		item := adminWorldStatisticsItem{
			ID: world.ID, Name: world.Name, Description: world.Description, Visibility: world.Visibility,
			OwnerID: world.OwnerID, OwnerNickname: ownerName, CreatedAt: world.CreatedAt,
			MemberCount: memberCounts[world.ID], ChannelCount: channelCounts[world.ID],
			MessageCount: message.MessageCount, LastActiveAt: message.LastActiveAt,
			MessageChars: message.Chars, MessageCharsIC: message.CharsIC, MessageCharsOOC: message.CharsOOC,
		}
		snapshot.entries = append(snapshot.entries, adminWorldStatisticsEntry{
			item: item,
			searchText: [4]string{
				strings.ToLower(world.Name), strings.ToLower(world.Description),
				strings.ToLower(world.OwnerNickname), strings.ToLower(world.OwnerUsername),
			},
		})
		snapshot.summary.MemberCount += item.MemberCount
		snapshot.summary.ChannelCount += item.ChannelCount
		snapshot.summary.MessageCount += item.MessageCount
		snapshot.summary.MessageCountIC += message.MessageCountIC
		snapshot.summary.MessageCountOOC += message.MessageCountOOC
		snapshot.summary.MessageChars += item.MessageChars
		snapshot.summary.MessageCharsIC += item.MessageCharsIC
		snapshot.summary.MessageCharsOOC += item.MessageCharsOOC
	}
	return snapshot, nil
}

func worldStatisticsMessageCharsQuality(state *model.MessageVisibleCharCountBackfillState) string {
	if state != nil {
		switch state.Status {
		case "done":
			return "ready"
		case "running":
			return "rebuilding"
		case "failed":
			return "failed"
		}
	}
	return "incomplete"
}

func (s *adminWorldStatisticsSnapshot) list(query AdminWorldStatisticsQuery, online map[string]int) *AdminWorldStatisticsResult {
	query.Page = max(query.Page, 1)
	if query.PageSize < 1 {
		query.PageSize = 20
	}
	query.PageSize = min(query.PageSize, 100)
	if query.SortBy == "" {
		query.SortBy = "createdAt"
	}
	if query.Order == "" {
		query.Order = "asc"
	}
	switch query.SortBy {
	case "createdAt", "memberCount", "onlineCount", "channelCount", "messageCount", "messageChars", "lastActiveAt":
	default:
		query.SortBy, query.Order = "createdAt", "asc"
	}
	if query.Order != "asc" && query.Order != "desc" {
		query.SortBy, query.Order = "createdAt", "asc"
	}
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	matches := make([]AdminWorldStatisticsItem, 0, len(s.entries))
	for _, entry := range s.entries {
		item := AdminWorldStatisticsItem{adminWorldStatisticsItem: entry.item, OnlineCount: online[entry.item.ID]}
		if keyword == "" || slices.ContainsFunc(entry.searchText[:], func(text string) bool {
			return strings.Contains(text, keyword)
		}) {
			matches = append(matches, item)
		}
	}

	descending := query.Order == "desc"
	slices.SortFunc(matches, func(a, b AdminWorldStatisticsItem) int {
		var comparison int
		switch query.SortBy {
		case "memberCount":
			comparison = cmp.Compare(a.MemberCount, b.MemberCount)
		case "onlineCount":
			comparison = cmp.Compare(a.OnlineCount, b.OnlineCount)
		case "channelCount":
			comparison = cmp.Compare(a.ChannelCount, b.ChannelCount)
		case "messageCount":
			comparison = cmp.Compare(a.MessageCount, b.MessageCount)
		case "messageChars":
			comparison = cmp.Compare(a.MessageChars, b.MessageChars)
		case "lastActiveAt":
			if (a.LastActiveAt == 0) != (b.LastActiveAt == 0) {
				if a.LastActiveAt == 0 {
					return 1
				}
				return -1
			}
			comparison = cmp.Compare(a.LastActiveAt, b.LastActiveAt)
		default:
			comparison = a.CreatedAt.Compare(b.CreatedAt)
		}
		if comparison != 0 {
			if descending {
				return -comparison
			}
			return comparison
		}
		if comparison = a.CreatedAt.Compare(b.CreatedAt); comparison != 0 {
			return comparison
		}
		return cmp.Compare(a.ID, b.ID)
	})

	filteredTotal := len(matches)
	items := matches[:0:0]
	if filteredTotal > 0 && query.Page-1 <= (filteredTotal-1)/query.PageSize {
		start := (query.Page - 1) * query.PageSize
		end := start + min(query.PageSize, filteredTotal-start)
		items = matches[start:end:end]
	}
	return &AdminWorldStatisticsResult{
		GeneratedAt: s.generatedAt.UnixMilli(), Total: filteredTotal,
		Page: query.Page, PageSize: query.PageSize, SortBy: query.SortBy, Order: query.Order,
		DataQuality: s.dataQuality, Summary: s.summary, Items: items,
	}
}
