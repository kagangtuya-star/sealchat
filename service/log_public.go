package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"sealchat/model"
)

type logPublicScope struct {
	Project model.LogProjectModel
	Release model.LogReleaseModel
	Draft   LogDraft
	Privacy int64
	Slug    string
}

func logPublicNotFound() error { return logError("NOT_FOUND", "公开 LOG 不存在") }
func logLoadPublic(slug string) (*logPublicScope, error) {
	if !logSlugPattern.MatchString(slug) {
		return nil, logPublicNotFound()
	}
	var alias model.LogRouteAliasModel
	if err := model.GetDB().Where("slug = ? AND kind = ? AND active = ?", slug, "view", true).Take(&alias).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, logPublicNotFound()
		}
		return nil, err
	}
	s := &logPublicScope{Slug: slug, Privacy: 1}
	if err := model.GetDB().Where("id = ? AND enabled = ? AND active_release_id <> ?", alias.ProjectID, true, "").Take(&s.Project).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, logPublicNotFound()
		}
		return nil, err
	}
	if _, err := GetWorldByID(s.Project.WorldID); err != nil {
		return nil, logPublicNotFound()
	}
	if err := model.GetDB().Where("id = ? AND project_id = ? AND status = ?", s.Project.ActiveReleaseID, s.Project.ID, "active").Take(&s.Release).Error; err != nil {
		return nil, logPublicNotFound()
	}
	if err := json.Unmarshal([]byte(s.Release.DraftJSON), &s.Draft); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	channels := []string{}
	for _, section := range s.Draft.Sections {
		channel := section.Source.ChannelID
		if seen[channel] {
			continue
		}
		seen[channel] = true
		channels = append(channels, channel)
	}
	if len(channels) > 0 {
		var available int64
		if err := model.GetDB().Model(&model.ChannelModel{}).Where("id IN ? AND world_id = ? AND status = ? AND is_private = ?", channels, s.Project.WorldID, model.ChannelStatusActive, false).Count(&available).Error; err != nil {
			return nil, err
		}
		if available != int64(len(channels)) {
			return nil, logPublicNotFound()
		}
		var versions []model.LogChannelVersionModel
		if err := model.GetDB().Where("channel_id IN ?", channels).Find(&versions).Error; err != nil {
			return nil, err
		}
		for _, v := range versions {
			if v.Version > LogMaxVersion-s.Privacy {
				return nil, logError("SOURCE_CHANGED", "隐私版本超出范围")
			}
			s.Privacy += v.Version
		}
	}
	return s, nil
}
func logPublicRecheck(s *logPublicScope) error {
	current, err := logLoadPublic(s.Slug)
	if err != nil {
		return err
	}
	if current.Release.ID != s.Release.ID || current.Privacy != s.Privacy {
		return logError("SOURCE_CHANGED", "公开内容已变化，请重新加载")
	}
	return nil
}
func GetPublicLog(slug string) (map[string]any, error) {
	s, err := logLoadPublic(slug)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(logResolvePublicURLs(s.Release.PublicJSON, slug)), &out); err != nil {
		return nil, err
	}
	out["privacyVersion"] = s.Privacy
	return out, logPublicRecheck(s)
}

type logPublicCursor struct {
	Release string `json:"r"`
	Section string `json:"s"`
	Privacy int64  `json:"v"`
	Ordinal int64  `json:"o"`
	Query   string `json:"q"`
}

func logDecodePublicCursor(s *logPublicScope, section, query, cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	if len(cursor) > 2048 {
		return 0, logError("INVALID_INPUT", "游标无效")
	}
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	var c logPublicCursor
	if err != nil || json.Unmarshal(b, &c) != nil || c.Section != section || c.Query != query || c.Ordinal < 0 {
		return 0, logError("INVALID_INPUT", "游标无效")
	}
	if c.Release != s.Release.ID || c.Privacy != s.Privacy {
		return 0, logError("SOURCE_CHANGED", "公开内容已变化，请重新加载")
	}
	return c.Ordinal, nil
}
func logEncodePublicCursor(s *logPublicScope, section, query string, ordinal int64) string {
	b, _ := json.Marshal(logPublicCursor{s.Release.ID, section, s.Privacy, ordinal, query})
	return base64.RawURLEncoding.EncodeToString(b)
}

// Every response independently checks current source visibility, including quote targets.
func logVisiblePublicNodes(s *logPublicScope, rows []model.LogReleaseNodeModel) ([]map[string]any, error) {
	ids := []string{}
	for _, n := range rows {
		if n.MessageID != "" {
			ids = append(ids, n.MessageID)
		}
		if n.QuoteMessageID != "" {
			ids = append(ids, n.QuoteMessageID)
		}
	}
	visible := map[string]string{}
	if len(ids) > 0 {
		var messages []model.MessageModel
		if err := model.GetDB().Select("id, channel_id").Where("id IN ? AND is_deleted = ? AND is_revoked = ? AND is_whisper = ?", ids, false, false, false).Find(&messages).Error; err != nil {
			return nil, err
		}
		for _, m := range messages {
			visible[m.ID] = m.ChannelID
		}
	}
	quoteIDs := []string{}
	for _, n := range rows {
		if n.QuoteMessageID != "" && visible[n.QuoteMessageID] == n.ChannelID {
			quoteIDs = append(quoteIDs, n.QuoteMessageID)
		}
	}
	type quoteNode struct {
		id    string
		quote *LogMessageQuote
	}
	quotes := map[string]quoteNode{}
	if len(quoteIDs) > 0 {
		var nodes []model.LogReleaseNodeModel
		if err := model.GetDB().Select("id, message_id, node_json").Where("release_id = ? AND message_id IN ?", s.Release.ID, quoteIDs).Order("ordinal").Find(&nodes).Error; err != nil {
			return nil, err
		}
		for _, n := range nodes {
			var quoted struct {
				Speaker struct {
					Name string `json:"name"`
				} `json:"speaker"`
				PlainText string `json:"plainText"`
			}
			if err := json.Unmarshal([]byte(n.NodeJSON), &quoted); err != nil {
				return nil, err
			}
			// The excerpt comes from the quoted node itself, so it is exactly as public as that node.
			quotes[n.MessageID] = quoteNode{id: n.ID, quote: logQuoteOf(quoted.Speaker.Name, quoted.PlainText)}
		}
	}
	out := []map[string]any{}
	anchored := map[string][]map[string]any{}
	for _, n := range rows {
		if n.MessageID != "" && visible[n.MessageID] != n.ChannelID {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(logResolvePublicURLs(n.NodeJSON, s.Slug)), &data); err != nil {
			return nil, err
		}
		if n.MessageID != "" {
			data["quoteNodeId"] = nil
			data["quote"] = nil
			if q, ok := quotes[n.QuoteMessageID]; ok {
				data["quoteNodeId"] = q.id
				if q.quote != nil {
					data["quote"] = q.quote
				}
			}
		}
		if id, ok := data["anchorNodeId"].(string); ok {
			anchored[id] = append(anchored[id], data)
		}
		out = append(out, data)
	}
	// The target may be on another cursor page or have become private since publication.
	if len(anchored) > 0 {
		var targets, public []string
		for id := range anchored {
			targets = append(targets, id)
		}
		err := model.GetDB().Table("log_release_nodes n").
			Joins("JOIN messages m ON m.id = n.message_id AND m.channel_id = n.channel_id").
			Where("n.release_id = ? AND n.id IN ? AND m.is_deleted = ? AND m.is_revoked = ? AND m.is_whisper = ?", s.Release.ID, targets, false, false, false).
			Pluck("n.id", &public).Error
		if err != nil {
			return nil, err
		}
		for _, id := range public {
			delete(anchored, id)
		}
		for _, nodes := range anchored {
			for _, node := range nodes {
				delete(node, "anchorNodeId")
			}
		}
	}
	return out, nil
}
func QueryPublicLog(slug, section, cursor, target, query string, limit int) (map[string]any, error) {
	return queryPublicLog(slug, false, section, cursor, target, query, limit)
}

// QueryPublicLogChapter reads one visual chapter: the release nodes of its sections in publish (ordinal) order.
// Only releases published with chapters expose chapter scopes; legacy releases keep section scopes.
func QueryPublicLogChapter(slug, chapter, cursor, target string, limit int) (map[string]any, error) {
	return queryPublicLog(slug, true, chapter, cursor, target, "", limit)
}

func queryPublicLog(slug string, chapterScope bool, scope, cursor, target, query string, limit int) (map[string]any, error) {
	if limit < 1 || limit > 200 || len([]rune(query)) > 200 {
		return nil, logError("INVALID_INPUT", "查询参数无效")
	}
	s, err := logLoadPublic(slug)
	if err != nil {
		return nil, err
	}
	// The cursor scope key keeps section and chapter cursors from being replayed against each other.
	scopeKey := scope
	sectionIDs := []string{}
	for _, x := range s.Draft.Sections {
		if (!chapterScope && x.ID == scope) || (chapterScope && x.ChapterID == scope) {
			sectionIDs = append(sectionIDs, x.ID)
		}
	}
	if chapterScope {
		scopeKey = "chapter:" + scope
		if scope == "" || len(sectionIDs) == 0 {
			return nil, logPublicNotFound()
		}
	} else if scope != "" && len(sectionIDs) == 0 {
		return nil, logPublicNotFound()
	}
	last, err := logDecodePublicCursor(s, scopeKey, query, cursor)
	if err != nil {
		return nil, err
	}
	base := func() *gorm.DB {
		q := model.GetDB().Session(&gorm.Session{Logger: logger.Discard}).Where("release_id = ?", s.Release.ID)
		if query != "" {
			escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(query))
			q = q.Where("LOWER(search_text) LIKE ? ESCAPE '!'", "%"+escaped+"%")
		}
		if scope != "" {
			q = q.Where("section_id IN ?", sectionIDs)
		}
		return q
	}
	if target != "" {
		var n model.LogReleaseNodeModel
		if err := base().Where("id = ?", target).Take(&n).Error; err != nil {
			return nil, logPublicNotFound()
		}
		v, err := logVisiblePublicNodes(s, []model.LogReleaseNodeModel{n})
		if err != nil {
			return nil, err
		}
		if len(v) == 0 {
			return nil, logPublicNotFound()
		}
		before, err := logPublicContextSide(s, sectionIDs, n.Ordinal, true)
		if err != nil {
			return nil, err
		}
		after, err := logPublicContextSide(s, sectionIDs, n.Ordinal, false)
		if err != nil {
			return nil, err
		}
		items := []map[string]any{}
		for i := len(before) - 1; i >= 0; i-- {
			items = append(items, before[i])
		}
		items = append(items, v[0])
		items = append(items, after...)
		return map[string]any{"releaseId": s.Release.ID, "privacyVersion": s.Privacy, "items": items}, logPublicRecheck(s)
	}
	items := []map[string]any{}
	var next *string
	for len(items) <= limit {
		var rows []model.LogReleaseNodeModel
		if err := base().Where("ordinal > ?", last).Order("ordinal").Limit(200).Find(&rows).Error; err != nil {
			return nil, err
		}
		visible, err := logVisiblePublicNodes(s, rows)
		if err != nil {
			return nil, err
		}
		byID := map[string]map[string]any{}
		for _, n := range visible {
			byID[n["id"].(string)] = n
		}
		for _, row := range rows {
			n, ok := byID[row.ID]
			if !ok {
				last = row.Ordinal
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(row.SearchText), strings.ToLower(query)) {
				last = row.Ordinal
				continue
			}
			if len(items) == limit {
				key := logEncodePublicCursor(s, scopeKey, query, last)
				next = &key
				break
			}
			last = row.Ordinal
			if query != "" {
				text := []rune(row.SearchText)
				if len(text) > 200 {
					text = text[:200]
				}
				n = map[string]any{"nodeId": row.ID, "sectionId": row.SectionID, "excerpt": string(text)}
			}
			items = append(items, n)
		}
		if next != nil || len(rows) < 200 {
			break
		}
	}
	out := map[string]any{"releaseId": s.Release.ID, "privacyVersion": s.Privacy, "items": items, "nextCursor": next}
	return out, logPublicRecheck(s)
}

func logPublicContextSide(s *logPublicScope, sectionIDs []string, ordinal int64, before bool) ([]map[string]any, error) {
	out := []map[string]any{}
	op, order := ">", "ordinal"
	if before {
		op = "<"
		order = "ordinal DESC"
	}
	for len(out) < 20 {
		var rows []model.LogReleaseNodeModel
		if err := model.GetDB().Where("release_id = ? AND section_id IN ? AND ordinal "+op+" ?", s.Release.ID, sectionIDs, ordinal).Order(order).Limit(100).Find(&rows).Error; err != nil {
			return nil, err
		}
		visible, err := logVisiblePublicNodes(s, rows)
		if err != nil {
			return nil, err
		}
		remaining := 20 - len(out)
		if len(visible) > remaining {
			visible = visible[:remaining]
		}
		out = append(out, visible...)
		if len(rows) < 100 {
			break
		}
		ordinal = rows[len(rows)-1].Ordinal
	}
	return out, nil
}
func GetPublicLogAsset(slug, key string) (*model.LogReleaseAssetModel, error) {
	if len(key) != 32 {
		return nil, logPublicNotFound()
	}
	s, err := logLoadPublic(slug)
	if err != nil {
		return nil, err
	}
	var global int64
	if err := model.GetDB().Model(&model.LogAssetReferenceModel{}).Where("release_id = ? AND asset_key = ? AND node_id = ?", s.Release.ID, key, "").Count(&global).Error; err != nil {
		return nil, err
	}
	allowed := global > 0
	if !allowed {
		var count int64
		err := model.GetDB().Table("log_asset_references a").Joins("JOIN log_release_nodes n ON n.release_id = a.release_id AND n.id = a.node_id").Joins("JOIN messages m ON m.id = n.message_id AND m.channel_id = n.channel_id").Where("a.release_id = ? AND a.asset_key = ? AND m.is_deleted = ? AND m.is_revoked = ? AND m.is_whisper = ?", s.Release.ID, key, false, false, false).Count(&count).Error
		if err != nil {
			return nil, err
		}
		allowed = count > 0
	}
	if !allowed {
		return nil, logPublicNotFound()
	}
	var asset model.LogReleaseAssetModel
	if err := model.GetDB().Where("release_id = ? AND asset_key = ?", s.Release.ID, key).Take(&asset).Error; err != nil {
		return nil, logPublicNotFound()
	}
	return &asset, logPublicRecheck(s)
}
