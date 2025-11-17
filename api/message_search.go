package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/service"
)

type messageSearchResponse struct {
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
	Total    int64                    `json:"total"`
	HasMore  bool                     `json:"has_more"`
	Items    []messageSearchItem      `json:"items"`
	Metadata map[string]any           `json:"metadata,omitempty"`
	Filters  map[string]any           `json:"filters,omitempty"`
	Debug    map[string]any           `json:"debug,omitempty"`
	Keyword  string                   `json:"keyword"`
	Match    string                   `json:"match_mode"`
	Tokens   []string                 `json:"tokens,omitempty"`
	Channel  *messageSearchChannelRef `json:"channel,omitempty"`
}

type messageSearchChannelRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type messageSearchItem struct {
	ID              string                 `json:"id"`
	ChannelID       string                 `json:"channel_id"`
	ContentSnippet  string                 `json:"content_snippet"`
	Snippet         string                 `json:"snippet"`
	SenderName      string                 `json:"sender_name"`
	SenderMember    string                 `json:"sender_member_name,omitempty"`
	IcMode          string                 `json:"ic_mode"`
	IsArchived      bool                   `json:"is_archived"`
	ArchivedAt      int64                  `json:"archived_at"`
	CreatedAt       int64                  `json:"created_at"`
	DisplayOrder    float64                `json:"display_order"`
	User            *messageSearchUser     `json:"user,omitempty"`
	Identity        *messageSearchIdentity `json:"identity,omitempty"`
	IsWhisper       bool                   `json:"is_whisper"`
	WhisperToUserID string                 `json:"whisper_to_user_id,omitempty"`
	HighlightRanges [][2]int               `json:"highlight_ranges,omitempty"`
	WhisperLabel    string                 `json:"whisper_label,omitempty"`
}

type messageSearchUser struct {
	ID     string `json:"id"`
	Nick   string `json:"nick"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	IsBot  bool   `json:"is_bot"`
}

type messageSearchIdentity struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Color       string `json:"color"`
	AvatarID    string `json:"avatar_attachment"`
}

func ChannelMessageSearch(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
			"message": "未登录",
		})
	}

	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "缺少频道ID",
		})
	}

	keyword := strings.TrimSpace(c.Query("keyword"))
	if utf8.RuneCountInString(keyword) < 1 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "请输入至少1个字符的关键字",
		})
	}
	if utf8.RuneCountInString(keyword) > 120 {
		runes := []rune(keyword)
		keyword = string(runes[:120])
	}

	matchMode := strings.ToLower(strings.TrimSpace(c.Query("match_mode", "fuzzy")))
	if matchMode != "exact" && matchMode != "fuzzy" {
		matchMode = "fuzzy"
	}

	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := c.QueryInt("page_size", 10)
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 50 {
		pageSize = 50
	}

	archivedFilter := strings.ToLower(strings.TrimSpace(c.Query("archived", "all")))
	if archivedFilter != "all" && archivedFilter != "only" && archivedFilter != "exclude" {
		archivedFilter = "all"
	}

	icMode := strings.ToLower(strings.TrimSpace(c.Query("ic_mode", "all")))
	if icMode != "all" && icMode != "ic" && icMode != "ooc" {
		icMode = "all"
	}

	includeOutside := c.QueryBool("include_outside", true)

	timeStart := parseQueryInt64(c, "time_start")
	timeEnd := parseQueryInt64(c, "time_end")

	speakerIDs := parseQueryStringSlice(c, "speaker_ids")
	if len(speakerIDs) > 0 {
		speakerIDs = lo.Uniq(speakerIDs)
	}

	sortMode := strings.ToLower(strings.TrimSpace(c.Query("sort", "time_desc")))

	channelRef, err := resolveChannelAccess(user.ID, channelID)
	if err != nil {
		if errors.Is(err, fiber.ErrForbidden) {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "没有访问该频道的权限"})
		}
		if errors.Is(err, fiber.ErrNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}

	db := model.GetDB()
	buildBaseQuery := func() *gorm.DB {
		q := db.Model(&model.MessageModel{}).
			Where("channel_id = ?", channelID).
			Where("(is_whisper = ? OR user_id = ? OR whisper_to = ?)", false, user.ID, user.ID).
			Where("is_revoked = ?", false)

		switch archivedFilter {
		case "only":
			q = q.Where("is_archived = ?", true)
		case "exclude":
			q = q.Where("is_archived = ?", false)
		}

		switch icMode {
		case "ic":
			q = q.Where("ic_mode = ?", "ic")
		case "ooc":
			q = q.Where("ic_mode = ?", "ooc")
		default:
			if !includeOutside {
				q = q.Where("ic_mode <> ?", "ooc")
			}
		}

		if len(speakerIDs) > 0 {
			q = q.Where("sender_identity_id IN ?", speakerIDs)
		}

		if timeStart > 0 {
			q = q.Where("created_at >= ?", time.UnixMilli(timeStart))
		}
		if timeEnd > 0 {
			q = q.Where("created_at <= ?", time.UnixMilli(timeEnd))
		}

		return q
	}

	var tokens []string
	var usedFTS bool
	var backendName string

	query, tokens, usedFTS, backendName := buildKeywordQuery(buildBaseQuery, keyword, matchMode)

	countQuery := query.Session(&gorm.Session{})
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		if usedFTS {
			reportFTSError(backendName, err)
			log.Printf("消息搜索(%s)统计失败，降级重试: %v", backendName, err)
			query, tokens, usedFTS, backendName = buildKeywordQuery(buildBaseQuery, keyword, matchMode, forceFallbackOption(true))
			countQuery = query.Session(&gorm.Session{})
			if retryErr := countQuery.Count(&total).Error; retryErr != nil {
				log.Printf("消息搜索降级后仍失败: %v", retryErr)
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
					"message": "查询失败",
				})
			}
		} else {
			log.Printf("消息搜索统计失败: %v", err)
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"message": "查询失败",
			})
		}
	}

	dataQuery := query.Session(&gorm.Session{})
	switch sortMode {
	case "relevance":
		dataQuery = dataQuery.Order("updated_at desc").Order("created_at desc")
	default:
		dataQuery = dataQuery.Order("display_order desc").Order("created_at desc").Order("id desc")
	}
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	var messages []*model.MessageModel
	err = dataQuery.
		Offset(offset).
		Limit(pageSize).
		Preload("User", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id, username, nickname, avatar, is_bot")
		}).
		Preload("Member", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id, nickname, channel_id, user_id")
		}).
		Find(&messages).Error
	if err != nil {
		if usedFTS {
			reportFTSError(backendName, err)
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"message": "查询失败",
		})
	}

	items := lo.Map(messages, func(msg *model.MessageModel, _ int) messageSearchItem {
		return buildMessageSearchItem(msg)
	})

	resp := messageSearchResponse{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
		HasMore:  int64(page*pageSize) < total,
		Items:    items,
		Keyword:  keyword,
		Match:    matchMode,
		Tokens:   tokens,
		Channel: &messageSearchChannelRef{
			ID:   channelRef.ID,
			Name: channelRef.Name,
		},
		Filters: map[string]any{
			"archived":        archivedFilter,
			"ic_mode":         icMode,
			"include_outside": includeOutside,
			"time_start":      timeStart,
			"time_end":        timeEnd,
			"speaker_ids":     speakerIDs,
			"sort":            sortMode,
		},
		Metadata: map[string]any{
			"search_backend": backendName,
			"sqlite": map[string]any{
				"ready":      model.SQLiteFTSReady(),
				"last_error": model.LastFTSError(),
			},
			"postgres": map[string]any{
				"ready":      model.PostgresFTSReady(),
				"last_error": model.LastPostgresFTSError(),
			},
		},
	}

	return c.JSON(resp)
}

func resolveChannelAccess(userID, channelID string) (*messageSearchChannelRef, error) {
	if len(channelID) < 30 {
		ch, err := model.ChannelGet(channelID)
		if err != nil {
			return nil, err
		}
		if ch.ID == "" {
			return nil, fiber.ErrNotFound
		}
		if !service.CanReadChannelByUserId(userID, channelID) {
			return nil, fiber.ErrForbidden
		}
		return &messageSearchChannelRef{ID: ch.ID, Name: ch.Name}, nil
	}

	fr, err := model.FriendRelationGetByID(channelID)
	if err != nil {
		return nil, err
	}
	if fr.ID == "" {
		return nil, fiber.ErrNotFound
	}
	if fr.UserID1 != userID && fr.UserID2 != userID {
		return nil, fiber.ErrForbidden
	}

	otherID := fr.UserID1
	if otherID == userID {
		otherID = fr.UserID2
	}

	displayName := "私聊"
	if other := model.UserGet(otherID); other != nil {
		if nick := strings.TrimSpace(other.Nickname); nick != "" {
			displayName = fmt.Sprintf("私聊 @%s", nick)
		} else if name := strings.TrimSpace(other.Username); name != "" {
			displayName = fmt.Sprintf("私聊 @%s", name)
		}
	}

	return &messageSearchChannelRef{ID: fr.ID, Name: displayName}, nil
}

func parseQueryInt64(c *fiber.Ctx, key string) int64 {
	raw := strings.TrimSpace(c.Query(key, ""))
	if raw == "" {
		return 0
	}
	val, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

func parseQueryStringSlice(c *fiber.Ctx, key string) []string {
	var values []string
	targets := map[string]struct{}{
		key:        {},
		key + "[]": {},
	}
	c.Context().QueryArgs().VisitAll(func(k, v []byte) {
		if _, ok := targets[string(k)]; !ok {
			return
		}
		raw := strings.TrimSpace(string(v))
		if raw == "" {
			return
		}
		segments := strings.Split(raw, ",")
		for _, seg := range segments {
			seg = strings.TrimSpace(seg)
			if seg != "" {
				values = append(values, seg)
			}
		}
	})
	return values
}

func applyDefaultKeywordFilter(q *gorm.DB, keyword, mode string) (*gorm.DB, []string) {
	normalized := strings.ToLower(keyword)
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return q, nil
	}
	tokens := lo.Filter(strings.Fields(normalized), func(item string, _ int) bool {
		return item != ""
	})

	switch mode {
	case "exact":
		q = q.Where("LOWER(content) LIKE ?", "%"+normalized+"%")
	default:
		if len(tokens) == 0 {
			pattern := buildFuzzyPattern(normalized)
			q = q.Where("LOWER(content) LIKE ?", pattern)
			tokens = []string{normalized}
		} else if len(tokens) == 1 {
			pattern := buildFuzzyPattern(tokens[0])
			q = q.Where("LOWER(content) LIKE ?", pattern)
		} else {
			for _, token := range tokens {
				q = q.Where("LOWER(content) LIKE ?", "%"+token+"%")
			}
		}
	}

	return q, tokens
}

func trySQLiteFTSFilter(q *gorm.DB, keyword, mode string) (*gorm.DB, []string, error) {
	tokens := tokenizeKeyword(keyword)
	query := buildSQLiteFTSQuery(tokens, mode)
	if query == "" {
		return q, tokens, nil
	}
	subQuery := model.GetDB().Table("messages_fts").
		Select("message_id").
		Where("messages_fts MATCH ?", query)
	return q.Where("id IN (?)", subQuery), tokens, nil
}

type keywordOptions struct {
	forceFallback bool
}

const (
	backendSQLiteFTS   = "sqlite_fts"
	backendPostgresFTS = "postgres_fts"
	backendFallback    = "fallback_like"
)

func forceFallbackOption(force bool) keywordOptions {
	return keywordOptions{forceFallback: force}
}

func buildKeywordQuery(base func() *gorm.DB, keyword, matchMode string, opts ...keywordOptions) (*gorm.DB, []string, bool, string) {
	forcedFallback := false
	if len(opts) > 0 {
		forcedFallback = opts[0].forceFallback
	}
	if !forcedFallback {
		if model.IsSQLite() && model.SQLiteFTSReady() {
			if q, tokens, err := trySQLiteFTSFilter(base(), keyword, matchMode); err == nil {
				return q, tokens, true, backendSQLiteFTS
			} else {
				model.ReportSQLiteFTSFailure(err)
				log.Printf("SQLite FTS 构建查询失败，降级: %v", err)
			}
		}
		if model.IsPostgres() && model.PostgresFTSReady() {
			if q, tokens, err := tryPostgresFTSFilter(base(), keyword, matchMode); err == nil {
				return q, tokens, true, backendPostgresFTS
			} else {
				model.ReportPostgresFTSFailure(err)
				log.Printf("Postgres FTS 构建查询失败，降级: %v", err)
			}
		}
	}
	q, tokens := applyDefaultKeywordFilter(base(), keyword, matchMode)
	fallback := backendFallback
	if drv := strings.TrimSpace(model.DBDriver()); drv != "" {
		fallback = fmt.Sprintf("%s_%s", backendFallback, drv)
	}
	return q, tokens, false, fallback
}

func tokenizeKeyword(keyword string) []string {
	normalized := strings.TrimSpace(strings.ToLower(keyword))
	if normalized == "" {
		return nil
	}
	parts := strings.Fields(normalized)
	return lo.Filter(parts, func(item string, _ int) bool { return item != "" })
}

func buildSQLiteFTSQuery(tokens []string, mode string) string {
	clean := lo.Filter(tokens, func(item string, _ int) bool { return item != "" })
	if len(clean) == 0 {
		return ""
	}
	var clauses []string
	useExact := mode == "exact" && len(clean) == 1
	for _, token := range clean {
		escaped := escapeFTSToken(token)
		if escaped == "" {
			continue
		}
		if useExact {
			clauses = append(clauses, `"`+escaped+`"`)
		} else {
			clauses = append(clauses, escaped+"*")
		}
	}
	if len(clauses) == 0 {
		return ""
	}
	return strings.Join(clauses, " AND ")
}

func escapeFTSToken(token string) string {
	replacer := strings.NewReplacer(
		"'", "",
		"\"", "",
		"*", "",
		":", "",
		"(", "",
		")", "",
		"~", "",
		"!", "",
		"@", "",
		"#", "",
		"$", "",
		"%", "",
		"^", "",
		"&", "",
		"+", "",
		"=", "",
		"/", "",
		"\\", "",
		"|", "",
		"[", "",
		"]", "",
		"{", "",
		"}", "",
	)
	return strings.TrimSpace(replacer.Replace(token))
}

func buildFuzzyPattern(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return "%%"
	}
	var builder strings.Builder
	builder.WriteRune('%')
	for _, r := range token {
		builder.WriteRune(r)
		builder.WriteRune('%')
	}
	return builder.String()
}

func reportFTSError(backend string, err error) {
	if err == nil {
		return
	}
	switch backend {
	case backendSQLiteFTS:
		model.ReportSQLiteFTSFailure(err)
	case backendPostgresFTS:
		model.ReportPostgresFTSFailure(err)
	}
}

func tryPostgresFTSFilter(q *gorm.DB, keyword, mode string) (*gorm.DB, []string, error) {
	tokens := tokenizeKeyword(keyword)
	tsQuery := buildPostgresTSQuery(tokens, mode)
	if tsQuery.SQL == "" {
		return q, tokens, nil
	}
	whereExpr := clause.Expr{
		SQL:  "content_tsv @@ " + tsQuery.SQL,
		Vars: tsQuery.Vars,
	}
	orderExpr := clause.Expr{
		SQL:  "ts_rank_cd(content_tsv, " + tsQuery.SQL + ") DESC",
		Vars: tsQuery.Vars,
	}
	q = q.Where(whereExpr)
	q = q.Order(orderExpr)
	return q, tokens, nil
}

func buildPostgresTSQuery(tokens []string, mode string) clause.Expr {
	clean := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			clean = append(clean, token)
		}
	}
	if len(clean) == 0 {
		return clause.Expr{}
	}
	config := model.PostgresTextSearchConfig()
	if mode == "exact" && len(clean) == 1 {
		return clause.Expr{
			SQL:  fmt.Sprintf("plainto_tsquery('%s', ?)", config),
			Vars: []any{clean[0]},
		}
	}
	var parts []string
	for _, token := range clean {
		escaped := escapePostgresToken(token)
		if escaped == "" {
			continue
		}
		parts = append(parts, escaped+":*")
	}
	if len(parts) == 0 {
		return clause.Expr{}
	}
	return clause.Expr{
		SQL:  fmt.Sprintf("to_tsquery('%s', ?)", config),
		Vars: []any{strings.Join(parts, " & ")},
	}
}

func escapePostgresToken(token string) string {
	replacer := strings.NewReplacer(
		"'", " ",
		":", " ",
		"&", " ",
		"|", " ",
		"!", " ",
		"(", " ",
		")", " ",
		"*", " ",
	)
	return strings.TrimSpace(replacer.Replace(token))
}

func buildMessageSearchItem(msg *model.MessageModel) messageSearchItem {
	snippet := buildSnippet(msg.Content, 280)
	senderName := resolveSenderName(msg)

	item := messageSearchItem{
		ID:             msg.ID,
		ChannelID:      msg.ChannelID,
		ContentSnippet: snippet,
		Snippet:        snippet,
		SenderName:     senderName,
		SenderMember:   msg.SenderMemberName,
		IcMode:         msg.ICMode,
		IsArchived:     msg.IsArchived,
		ArchivedAt: func() int64 {
			if msg.ArchivedAt == nil {
				return 0
			}
			return msg.ArchivedAt.UnixMilli()
		}(),
		CreatedAt:    msg.CreatedAt.UnixMilli(),
		DisplayOrder: msg.DisplayOrder,
		IsWhisper:    msg.IsWhisper,
	}
	if msg.WhisperTo != "" {
		item.WhisperToUserID = msg.WhisperTo
	}
	if msg.User != nil {
		item.User = &messageSearchUser{
			ID:     msg.User.ID,
			Nick:   msg.User.Nickname,
			Name:   msg.User.Username,
			Avatar: msg.User.Avatar,
			IsBot:  msg.User.IsBot,
		}
	}
	if msg.SenderIdentityID != "" {
		item.Identity = &messageSearchIdentity{
			ID:          msg.SenderIdentityID,
			DisplayName: msg.SenderIdentityName,
			Color:       msg.SenderIdentityColor,
			AvatarID:    msg.SenderIdentityAvatarID,
		}
	}
	if msg.Member != nil && item.SenderName == "" {
		item.SenderName = msg.Member.Nickname
	}

	return item
}

func buildSnippet(content string, limit int) string {
	if limit <= 0 {
		limit = 200
	}
	normalized := strings.TrimSpace(content)
	normalized = strings.ReplaceAll(normalized, "\r", " ")
	normalized = strings.ReplaceAll(normalized, "\n", " ")
	runes := []rune(normalized)
	if len(runes) <= limit {
		return strings.TrimSpace(string(runes))
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

func resolveSenderName(msg *model.MessageModel) string {
	if msg == nil {
		return ""
	}
	if msg.SenderMemberName != "" {
		return msg.SenderMemberName
	}
	if msg.Member != nil {
		if name := strings.TrimSpace(msg.Member.Nickname); name != "" {
			return name
		}
	}
	if msg.User != nil {
		if name := strings.TrimSpace(msg.User.Nickname); name != "" {
			return name
		}
		if name := strings.TrimSpace(msg.User.Username); name != "" {
			return name
		}
	}
	if msg.SenderIdentityName != "" {
		return msg.SenderIdentityName
	}
	return "未知成员"
}
