package api

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sealchat/model"
	"sealchat/service"
)

const mcpSearchMaxResults = 1000

type mcpSearchInput struct {
	mcpPageInput
	Query           string     `json:"query"`
	Sources         []string   `json:"sources,omitempty"`
	WorldID         string     `json:"worldId,omitempty"`
	ChannelIDs      []string   `json:"channelIds,omitempty"`
	From            *time.Time `json:"from,omitempty"`
	To              *time.Time `json:"to,omitempty"`
	Match           string     `json:"match,omitempty"`
	Scope           string     `json:"scope,omitempty"`
	IncludeArchived bool       `json:"includeArchived,omitempty"`
	IncludePublic   bool       `json:"includePublic,omitempty"`
}

type mcpSearchItem struct {
	Source    string    `json:"source"`
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Snippet   string    `json:"snippet"`
	WorldID   string    `json:"worldId,omitempty"`
	ChannelID string    `json:"channelId,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type mcpSearchPage struct {
	Items              []mcpSearchItem `json:"items"`
	SearchedSources    []string        `json:"searchedSources"`
	SkippedSources     []string        `json:"skippedSources"`
	Page               int             `json:"page"`
	Limit              int             `json:"limit"`
	HasMore            bool            `json:"hasMore"`
	ResultLimitReached bool            `json:"resultLimitReached,omitempty"`
	ContentTrust       string          `json:"contentTrust"`
}

type mcpSearchRequest struct {
	mcpSearchInput
	channels []string
}

// This table adapts existing domain queries. Scopes and business ACLs are
// checked per request; no actor or provider results are cached on the server.
type mcpSearchProvider struct {
	Source    string
	ReadScope string
	Search    func(*service.MCPActor, mcpSearchRequest, int) ([]mcpSearchItem, error)
}

func mcpSearchProviders() []mcpSearchProvider {
	return []mcpSearchProvider{
		{"worlds", "", mcpSearchWorlds},
		{"channels", "", mcpSearchChannels},
		{"messages", "chat:read", mcpSearchMessages},
		{"clues", "clue:read", mcpSearchClues},
		{"battle_reports", "battle_report:read", mcpSearchBattleReports},
		{"glossary", "glossary:read", mcpSearchGlossary},
		{"notes", "note:read", mcpSearchNotes},
		{"identities", "identity:read", mcpSearchIdentities},
		{"audio_assets", "audio:read", mcpSearchAudioAssets},
	}
}

func mcpSearchTools() []mcpToolSpec {
	return []mcpToolSpec{mcpSpec("search", "跨资源检索摘要。需要 search:read 及各来源 read scope；省略 sources 自动展开授权且上下文适用的来源。除 worlds 外需 worldId，省略 channelIds 搜索世界内可见频道。limit 默认 30、最大 100；page 最多 100、page*limit 最多 1000。", []string{"search:read"}, false, false, true, mcpSearch)}
}

func mcpSearch(_ context.Context, a *service.MCPActor, in mcpSearchInput) (any, error) {
	in.Query = normalizeSearchKeyword(in.Query)
	if utf8.RuneCountInString(in.Query) < 1 || utf8.RuneCountInString(in.Query) > 500 {
		return nil, mcpFailure("invalid_argument", "搜索关键词长度须为 1～500")
	}
	if in.Match != "" && in.Match != "fuzzy" && in.Match != "exact" {
		return nil, mcpFailure("invalid_argument", "match 必须为 fuzzy/exact")
	}
	if in.Scope != "" && in.Scope != "all" && in.Scope != "ic" && in.Scope != "ooc" {
		return nil, mcpFailure("invalid_argument", "scope 必须为 all/ic/ooc")
	}
	if in.From != nil && in.To != nil && !in.From.Before(*in.To) {
		return nil, mcpFailure("invalid_argument", "搜索时间范围无效")
	}
	if in.Page == 0 {
		in.Page = 1
	}
	if in.Limit == 0 {
		in.Limit = 30
	}
	if in.Page < 1 || in.Page > 100 || in.Limit < 1 || in.Limit > 100 || in.Page*in.Limit > mcpSearchMaxResults {
		return nil, mcpFailure("invalid_argument", "limit 最大 100，page 最大 100，page*limit 最大 1000")
	}
	if len(in.WorldID) > 100 || len(in.ChannelIDs) > 20 {
		return nil, mcpFailure("invalid_argument", "世界标识或频道数量无效")
	}
	for _, id := range in.ChannelIDs {
		if id == "" || len(id) > 100 {
			return nil, mcpFailure("invalid_argument", "频道标识无效")
		}
	}
	if len(in.ChannelIDs) > 0 && in.WorldID == "" {
		return nil, mcpFailure("invalid_argument", "channelIds 需要 worldId")
	}
	providers := mcpSearchProviders()
	selected := []mcpSearchProvider{}
	result := mcpSearchPage{Items: []mcpSearchItem{}, SearchedSources: []string{}, SkippedSources: []string{}, Page: in.Page, Limit: in.Limit, ContentTrust: "untrusted_user_generated"}
	if in.Sources == nil {
		for _, p := range providers {
			if p.ReadScope != "" && !a.Allows(p.ReadScope) {
				continue
			}
			if p.Source != "worlds" && in.WorldID == "" {
				result.SkippedSources = append(result.SkippedSources, p.Source)
				continue
			}
			selected = append(selected, p)
		}
	} else {
		if len(in.Sources) == 0 || len(in.Sources) > len(providers) {
			return nil, mcpFailure("invalid_argument", "sources 数量无效")
		}
		seen := map[string]bool{}
		for _, source := range in.Sources {
			found := false
			for _, p := range providers {
				if source != p.Source {
					continue
				}
				found = true
				if p.ReadScope != "" && !a.Allows(p.ReadScope) {
					return nil, mcpFailure("scope_denied", source+" 需要 "+p.ReadScope)
				}
				if !seen[source] {
					selected = append(selected, p)
					seen[source] = true
				}
				break
			}
			if !found {
				return nil, mcpFailure("invalid_argument", "未知搜索来源："+source)
			}
		}
		for _, p := range selected {
			if p.Source != "worlds" && in.WorldID == "" {
				return nil, mcpFailure("invalid_argument", p.Source+" 需要 worldId")
			}
		}
	}
	req := mcpSearchRequest{mcpSearchInput: in}
	for _, p := range selected {
		if p.Source == "messages" || p.Source == "notes" || p.Source == "identities" {
			ids, err := mcpSearchChannelIDs(a, in)
			if err != nil {
				return nil, err
			}
			req.channels = ids
			break
		}
	}
	// Each provider needs only the first end+1 candidates under the same time
	// ordering as the merge. Deep pagination is explicitly capped, without cursors.
	end := in.Page * in.Limit
	for _, p := range selected {
		items, err := p.Search(a, req, end+1)
		if err != nil {
			var unavailable *mcpError
			if in.Sources == nil && p.Source == "audio_assets" && errors.As(err, &unavailable) && unavailable.Code == "forbidden" {
				result.SkippedSources = append(result.SkippedSources, p.Source)
				continue
			}
			return nil, err
		}
		result.SearchedSources = append(result.SearchedSources, p.Source)
		result.Items = append(result.Items, items...)
	}
	mcpSortSearchItems(result.Items)
	result.HasMore = len(result.Items) > end && end < mcpSearchMaxResults
	result.ResultLimitReached = len(result.Items) > end && end == mcpSearchMaxResults
	start := (in.Page - 1) * in.Limit
	if start > len(result.Items) {
		start = len(result.Items)
	}
	if end > len(result.Items) {
		end = len(result.Items)
	}
	result.Items = result.Items[start:end]
	return result, nil
}

func mcpSortSearchItems(items []mcpSearchItem) {
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		return left.ID > right.ID
	})
}

func mcpSearchChannelIDs(a *service.MCPActor, in mcpSearchInput) ([]string, error) {
	if err := mcpWorld(a, in.WorldID); err != nil {
		return nil, err
	}
	if len(in.ChannelIDs) > 0 {
		ids := []string{}
		seen := map[string]bool{}
		for _, id := range in.ChannelIDs {
			if _, err := mcpChannel(a, in.WorldID, id); err != nil {
				return nil, err
			}
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		return ids, nil
	}
	q, err := service.ChannelListQuery(a.User.ID, in.WorldID)
	if err != nil {
		return nil, err
	}
	var ids []string
	err = q.Order("id ASC").Pluck("id", &ids).Error
	return ids, err
}

func mcpSearchTime(q *gorm.DB, in mcpSearchInput, column string) *gorm.DB {
	if in.From != nil {
		q = q.Where(column+" >= ?", *in.From)
	}
	if in.To != nil {
		q = q.Where(column+" <= ?", *in.To)
	}
	return q
}

func mcpSearchOrder(q *gorm.DB, column string) *gorm.DB {
	return q.Order(clause.OrderByColumn{Column: clause.Column{Name: column}, Desc: true, Reorder: true}).Order("id DESC")
}

func mcpSearchSummary(source, id, title, text, worldID, channelID string, updated, created time.Time) mcpSearchItem {
	if updated.IsZero() {
		updated = created
	}
	return mcpSearchItem{Source: source, ID: id, Title: buildSnippet(title, 120), Snippet: buildSnippet(text, 240), WorldID: worldID, ChannelID: channelID, UpdatedAt: updated.UTC()}
}

func mcpSearchWorlds(a *service.MCPActor, in mcpSearchRequest, limit int) ([]mcpSearchItem, error) {
	q := service.UserWorldListQuery(a.User.ID, service.UserWorldListOptions{JoinedOnly: !in.IncludePublic, IncludeArchived: in.IncludeArchived, Keyword: in.Query})
	if in.WorldID != "" {
		q = q.Where("worlds.id = ?", in.WorldID)
	}
	q = mcpSearchTime(q, in.mcpSearchInput, "worlds.updated_at")
	var rows []model.WorldModel
	if err := q.Select("worlds.id, worlds.name, worlds.description, worlds.updated_at, worlds.created_at").Order("worlds.updated_at DESC, worlds.id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := []mcpSearchItem{}
	for _, v := range rows {
		items = append(items, mcpSearchSummary("worlds", v.ID, v.Name, v.Description, v.ID, "", v.UpdatedAt, v.CreatedAt))
	}
	return items, nil
}

func mcpSearchChannels(a *service.MCPActor, in mcpSearchRequest, limit int) ([]mcpSearchItem, error) {
	if err := mcpWorld(a, in.WorldID); err != nil {
		return nil, err
	}
	q, err := service.ChannelListQuery(a.User.ID, in.WorldID)
	if err != nil {
		return nil, err
	}
	q = q.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(in.Query)+"%")
	if len(in.ChannelIDs) > 0 {
		for _, id := range in.ChannelIDs {
			if _, err := mcpChannel(a, in.WorldID, id); err != nil {
				return nil, err
			}
		}
		q = q.Where("id IN ?", in.ChannelIDs)
	}
	var rows []model.ChannelModel
	if err := mcpSearchOrder(mcpSearchTime(q, in.mcpSearchInput, "updated_at"), "updated_at").Select("id, world_id, name, updated_at, created_at").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := []mcpSearchItem{}
	for _, v := range rows {
		items = append(items, mcpSearchSummary("channels", v.ID, v.Name, v.Name, v.WorldID, v.ID, v.UpdatedAt, v.CreatedAt))
	}
	return items, nil
}

func mcpSearchMessages(a *service.MCPActor, in mcpSearchRequest, limit int) ([]mcpSearchItem, error) {
	if len(in.channels) == 0 {
		return []mcpSearchItem{}, nil
	}
	base := func() *gorm.DB {
		visible := model.GetDB().Session(&gorm.Session{NewDB: true})
		for _, id := range in.channels {
			channel := model.GetDB().Session(&gorm.Session{NewDB: true}).Where("channel_id = ?", id)
			visible = visible.Or(applyWhisperVisibilityFilter(channel, a.User.ID, id))
		}
		q := model.GetDB().Model(&model.MessageModel{}).Where(visible).Where("is_revoked = ? AND is_deleted = ?", false, false)
		if !in.IncludeArchived {
			q = q.Where("is_archived = ?", false)
		}
		if in.Scope == "ic" || in.Scope == "ooc" {
			q = q.Where("ic_mode = ?", in.Scope)
		}
		return mcpSearchTime(q, in.mcpSearchInput, "created_at")
	}
	// Keep the original full text/CJK fallback query path. It has no stable
	// cross-source relevance score, so candidates and the merge use timestamps.
	match := parseMatchMode(in.Match)
	force := shouldForceLikeFallback(in.Query, match)
	q, _, fts, backend := buildKeywordQuery(base, in.Query, match, forceFallbackOption(force))
	var total int64
	err := q.Session(&gorm.Session{}).Count(&total).Error
	if err != nil && fts {
		reportFTSError(backend, err)
		q, _, _, _ = buildKeywordQuery(base, in.Query, match, forceFallbackOption(true))
		err = q.Session(&gorm.Session{}).Count(&total).Error
	}
	if err != nil {
		return nil, err
	}
	if total == 0 && fts && force {
		q, _, _, _ = buildKeywordQuery(base, in.Query, match, forceFallbackOption(true))
	}
	var rows []*model.MessageModel
	if err := mcpSearchOrder(q.Session(&gorm.Session{}), "created_at").Limit(limit).Preload("User", func(q *gorm.DB) *gorm.DB { return q.Select("id, username, nickname, avatar, is_bot") }).Preload("Member").Find(&rows).Error; err != nil {
		return nil, err
	}
	items := []mcpSearchItem{}
	for _, v := range rows {
		items = append(items, mcpSearchSummary("messages", v.ID, resolveSenderName(v), v.Content, in.WorldID, v.ChannelID, v.CreatedAt, v.CreatedAt))
	}
	return items, nil
}

func mcpSearchClues(a *service.MCPActor, in mcpSearchRequest, limit int) ([]mcpSearchItem, error) {
	rows, err := service.WorldClueSearch(in.WorldID, a.User.ID, in.Query, limit, in.From, in.To)
	if err != nil {
		return nil, err
	}
	items := []mcpSearchItem{}
	for _, row := range rows {
		v := row.Summary
		items = append(items, mcpSearchSummary("clues", v.ID, v.Title, v.ContentText, v.WorldID, "", row.UpdatedAt, time.Time{}))
	}
	return items, nil
}

func mcpSearchBattleReports(a *service.MCPActor, in mcpSearchRequest, limit int) ([]mcpSearchItem, error) {
	q, err := service.BattleReportListQuery(in.WorldID, a.User.ID)
	if err != nil {
		return nil, err
	}
	q = q.Where("LOWER(title) LIKE ? OR LOWER(content) LIKE ?", "%"+strings.ToLower(in.Query)+"%", "%"+strings.ToLower(in.Query)+"%")
	if len(in.ChannelIDs) > 0 {
		q = q.Where("channel_id IN ?", in.ChannelIDs)
	}
	var rows []model.BattleReportModel
	if err := mcpSearchOrder(mcpSearchTime(q, in.mcpSearchInput, "updated_at"), "updated_at").Select("id, world_id, channel_id, title, content, updated_at, created_at").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items := []mcpSearchItem{}
	for _, v := range rows {
		items = append(items, mcpSearchSummary("battle_reports", v.ID, v.Title, v.Content, v.WorldID, v.ChannelID, v.UpdatedAt, v.CreatedAt))
	}
	return items, nil
}

func mcpSearchGlossary(a *service.MCPActor, in mcpSearchRequest, limit int) ([]mcpSearchItem, error) {
	rows, err := service.EffectiveWorldKeywordList(in.WorldID, a.User.ID, service.EffectiveWorldKeywordListOptions{Query: in.Query, IncludeAllMatches: true, SearchLimit: limit, From: in.From, To: in.To})
	if err != nil {
		return nil, err
	}
	items := []mcpSearchItem{}
	for _, v := range rows {
		updated, _ := time.Parse(time.RFC3339Nano, v.UpdatedAt)
		created, _ := time.Parse(time.RFC3339Nano, v.CreatedAt)
		items = append(items, mcpSearchSummary("glossary", v.ID, v.Keyword, v.Description, v.WorldID, "", updated, created))
	}
	mcpSortSearchItems(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func mcpSearchNotes(a *service.MCPActor, in mcpSearchRequest, limit int) ([]mcpSearchItem, error) {
	items := []mcpSearchItem{}
	if len(in.channels) == 0 {
		return items, nil
	}
	pattern := "%" + strings.ToLower(in.Query) + "%"
	q := model.GetDB().Where("channel_id IN ? AND is_deleted = ?", in.channels, false).Where("LOWER(title) LIKE ? OR LOWER(content_text) LIKE ? OR LOWER(content) LIKE ?", pattern, pattern, pattern)
	q = mcpSearchOrder(mcpSearchTime(q, in.mcpSearchInput, "updated_at"), "updated_at")
	for offset := 0; ; offset += 200 {
		var rows []model.StickyNoteModel
		if err := q.Offset(offset).Limit(200).Find(&rows).Error; err != nil {
			return nil, err
		}
		for i := range rows {
			v := &rows[i]
			if !service.CanViewStickyNote(v, a.User.ID) {
				continue
			}
			text := v.ContentText
			if text == "" {
				text = v.Content
			}
			items = append(items, mcpSearchSummary("notes", v.ID, v.Title, text, v.WorldID, v.ChannelID, v.UpdatedAt, v.CreatedAt))
			if len(items) == limit {
				return items, nil
			}
		}
		if len(rows) < 200 {
			return items, nil
		}
	}
}

func mcpSearchIdentities(a *service.MCPActor, in mcpSearchRequest, limit int) ([]mcpSearchItem, error) {
	ids := []string{}
	for _, id := range in.channels {
		_, err := mcpIdentityActor(a, mcpIdentityInput{mcpChannelInput: mcpChannelInput{in.WorldID, id}})
		if err != nil {
			if len(in.ChannelIDs) > 0 || !(errors.Is(err, service.ErrChannelPermissionDenied) || errors.Is(err, service.ErrChannelIdentityTargetNotInChannel) || errors.Is(err, service.ErrWorldPermission)) {
				return nil, err
			}
			continue
		}
		ids = append(ids, id)
	}
	items := []mcpSearchItem{}
	if len(ids) == 0 {
		return items, nil
	}
	q := model.GetDB().Where("channel_id IN ? AND user_id = ? AND (is_hidden = ? OR is_hidden IS NULL)", ids, a.User.ID, false).Where("LOWER(display_name) LIKE ?", "%"+strings.ToLower(in.Query)+"%")
	var rows []model.ChannelIdentityModel
	if err := mcpSearchOrder(mcpSearchTime(q, in.mcpSearchInput, "updated_at"), "updated_at").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, v := range rows {
		items = append(items, mcpSearchSummary("identities", v.ID, v.DisplayName, v.DisplayName, in.WorldID, v.ChannelID, v.UpdatedAt, v.CreatedAt))
	}
	return items, nil
}

func mcpSearchAudioAssets(a *service.MCPActor, in mcpSearchRequest, limit int) ([]mcpSearchItem, error) {
	if err := mcpAudioAccess(a, in.WorldID, ""); err != nil {
		return nil, err
	}
	items := []mcpSearchItem{}
	for page := 1; ; page++ {
		rows, total, err := service.AudioListAssets(service.AudioAssetFilters{Query: in.Query, Page: page, PageSize: 500, SearchOrder: true, UpdatedFrom: in.From, UpdatedTo: in.To, Scope: model.AudioScopeWorld, WorldID: &in.WorldID, IncludeCommon: true, ExcludeTags: []string{service.TheaterFeatureAudioTag}})
		if err != nil {
			return nil, err
		}
		for _, v := range rows {
			// Only fields already exposed by audio_assets enter the summary.
			items = append(items, mcpSearchSummary("audio_assets", v.ID, v.Name, strings.Join(v.Tags, ", "), in.WorldID, "", v.UpdatedAt, v.CreatedAt))
			if len(items) == limit {
				return items, nil
			}
		}
		if int64(page*500) >= total || len(rows) == 0 {
			return items, nil
		}
	}
}
