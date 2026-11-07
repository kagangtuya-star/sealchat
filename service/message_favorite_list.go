package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
)

type MessageFavoriteListOptions struct {
	MessageFavoriteScope
	TagIDs []string
	Query  string
	Cursor string
	Limit  int
}

type MessageFavoriteView struct {
	model.MessageFavoriteModel
	Tags       []model.MessageFavoriteTagModel `json:"tags"`
	Message    *protocol.Message               `json:"message,omitempty"`
	Status     string                          `json:"status"` // available, deleted, unavailable
	NoteCount  int64                           `json:"noteCount"`
	LatestNote *MessageFavoriteNoteView        `json:"latestNote,omitempty"`
}

type MessageFavoriteListResult struct {
	Items          []MessageFavoriteView `json:"items"`
	NextCursor     string                `json:"nextCursor"`
	CanManageWorld bool                  `json:"canManageWorld"`
}

type favoriteCursor struct {
	Key       string    `json:"key"`
	Order     int64     `json:"order"`
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

// All permission state is request-local. It never acts as a permanent grant.
type favoriteMessageResolver struct {
	actor       string
	readable    map[string]bool
	channels    map[string]*model.ChannelModel
	worldActive map[string]bool
	readAll     map[string]bool
}

type favoriteResolvedMessage struct {
	message *model.MessageModel
	worldID string
	visible bool
}

func newFavoriteMessageResolver(actor string) (*favoriteMessageResolver, error) {
	ids, err := ReadableChannelIDsByUserID(actor)
	if err != nil {
		return nil, err
	}
	r := &favoriteMessageResolver{actor: actor, readable: map[string]bool{}, channels: map[string]*model.ChannelModel{}, worldActive: map[string]bool{}, readAll: map[string]bool{}}
	for _, id := range ids {
		r.readable[id] = true
	}
	return r, nil
}

func (r *favoriteMessageResolver) ensureChannels(db *gorm.DB, ids []string) error {
	missing := []string{}
	for _, id := range ids {
		if _, known := r.channels[id]; !known {
			r.channels[id] = nil
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var channels []*model.ChannelModel
	if err := db.Where("id IN ?", missing).Find(&channels).Error; err != nil {
		return err
	}
	worldIDs := []string{}
	for _, ch := range channels {
		r.channels[ch.ID] = ch
		if ch.WorldID != "" {
			if _, known := r.worldActive[ch.WorldID]; !known {
				r.worldActive[ch.WorldID] = false
				worldIDs = append(worldIDs, ch.WorldID)
			}
		}
	}
	if len(worldIDs) > 0 {
		var worlds []model.WorldModel
		if err := db.Select("id").Where("id IN ? AND status = ?", worldIDs, "active").Find(&worlds).Error; err != nil {
			return err
		}
		for _, w := range worlds {
			r.worldActive[w.ID] = true
		}
	}
	return nil
}

func (r *favoriteMessageResolver) channelVisible(id string) bool {
	ch := r.channels[id]
	return r.readable[id] && ch != nil && !IsChannelDeletedForAccess(ch) && (ch.WorldID == "" || r.worldActive[ch.WorldID])
}

// Load message rows, visibility and display identities in batches. Whisper
// read-all permission is resolved once per distinct whisper channel and uses
// the existing strict-privacy/role path; recipient filtering remains shared SQL.
func (r *favoriteMessageResolver) load(db *gorm.DB, ids []string) (map[string]favoriteResolvedMessage, error) {
	var messages []*model.MessageModel
	if err := db.Select("id, channel_id, is_deleted, is_whisper").Where("id IN ?", ids).Find(&messages).Error; err != nil {
		return nil, err
	}
	channelIDs := []string{}
	for _, m := range messages {
		channelIDs = append(channelIDs, m.ChannelID)
	}
	if err := r.ensureChannels(db, channelIDs); err != nil {
		return nil, err
	}
	result := make(map[string]favoriteResolvedMessage, len(messages))
	visibleChannels := []string{}
	allWhisperChannels := []string{}
	seenChannels := map[string]bool{}
	for _, m := range messages {
		worldID := ""
		if ch := r.channels[m.ChannelID]; ch != nil {
			worldID = ch.WorldID
		}
		result[m.ID] = favoriteResolvedMessage{message: m, worldID: worldID}
		if !r.channelVisible(m.ChannelID) {
			continue
		}
		if !seenChannels[m.ChannelID] {
			seenChannels[m.ChannelID] = true
			visibleChannels = append(visibleChannels, m.ChannelID)
		}
		if m.IsWhisper {
			if _, known := r.readAll[m.ChannelID]; !known {
				r.readAll[m.ChannelID] = CanUserReadAllWhispersInChannel(r.actor, m.ChannelID)
			}
		}
	}
	if len(visibleChannels) == 0 {
		return result, nil
	}
	for _, id := range visibleChannels {
		if r.readAll[id] {
			allWhisperChannels = append(allWhisperChannels, id)
		}
	}
	visibility := ApplyWhisperVisibilityFilterWithReadAll(db.Session(&gorm.Session{NewDB: true}), r.actor, false)
	if len(allWhisperChannels) > 0 {
		visibility = visibility.Or("channel_id IN ?", allWhisperChannels)
	}
	var visible []*model.MessageModel
	err := db.Where("id IN ? AND channel_id IN ?", ids, visibleChannels).Where(visibility).
		Preload("User", func(q *gorm.DB) *gorm.DB { return q.Select("id, username, nickname, avatar, is_bot") }).Find(&visible).Error
	if err != nil {
		return nil, err
	}
	for _, m := range visible {
		item := result[m.ID]
		item.message, item.visible = m, true
		result[m.ID] = item
	}
	return result, nil
}

func favoriteView(item model.MessageFavoriteModel, resolved favoriteResolvedMessage, exists bool, r *favoriteMessageResolver) (MessageFavoriteView, bool) {
	view := MessageFavoriteView{MessageFavoriteModel: item, Tags: []model.MessageFavoriteTagModel{}, Status: "unavailable"}
	world := item.Scope == model.MessageFavoriteScopeWorld
	if !r.channelVisible(item.ChannelID) {
		return view, !world
	}
	if !exists {
		view.Status = "deleted"
		return view, true
	}
	m := resolved.message
	if !resolved.visible || m.ChannelID != item.ChannelID || resolved.worldID != item.WorldID || (world && m.IsWhisper) {
		return view, !world
	}
	if m.IsDeleted || m.IsRevoked {
		view.Status = "deleted"
		return view, true
	}
	view.Status = "available"
	view.Message = m.ToProtocolType2(&protocol.Channel{ID: m.ChannelID})
	if m.User != nil {
		view.Message.User = m.User.ToProtocolType()
	}
	return view, true
}

func favoriteViewDetails(db *gorm.DB, b messageFavoriteBucket, items []MessageFavoriteView) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	positions := map[string]int{}
	for i, item := range items {
		ids = append(ids, item.ID)
		positions[item.ID] = i
	}
	type linkedTag struct {
		model.MessageFavoriteTagModel
		FavoriteID string
	}
	var tags []linkedTag
	err := b.tags(db).Select("message_favorite_tags.*, l.favorite_id").
		Joins("JOIN message_favorite_tag_links l ON l.tag_id = message_favorite_tags.id").
		Where("l.favorite_id IN ?", ids).Order("normalized_name ASC, message_favorite_tags.id ASC").Scan(&tags).Error
	if err != nil {
		return err
	}
	for _, tag := range tags {
		i := positions[tag.FavoriteID]
		items[i].Tags = append(items[i].Tags, tag.MessageFavoriteTagModel)
	}
	// Fetch only counts and the most recent post for the returned page. The
	// correlated subquery uses favorite_id's index, without one query per card.
	var counts []struct {
		FavoriteID string
		Count      int64
	}
	if err := db.Model(&model.MessageFavoriteNoteModel{}).Select("favorite_id, COUNT(*) AS count").Where("favorite_id IN ?", ids).Group("favorite_id").Scan(&counts).Error; err != nil {
		return err
	}
	for _, row := range counts {
		items[positions[row.FavoriteID]].NoteCount = row.Count
	}
	var latest []model.MessageFavoriteNoteModel
	err = db.Table("message_favorite_notes AS n").Select("n.*").Where("n.favorite_id IN ?", ids).
		Where("NOT EXISTS (SELECT 1 FROM message_favorite_notes newer WHERE newer.favorite_id = n.favorite_id AND (newer.created_at > n.created_at OR (newer.created_at = n.created_at AND newer.id > n.id)))").Find(&latest).Error
	if err != nil {
		return err
	}
	views, err := favoriteNoteViews(db, latest)
	if err != nil {
		return err
	}
	for i := range views {
		items[positions[views[i].FavoriteID]].LatestNote = &views[i]
	}
	return nil
}

// Preserve Unicode substring semantics and scan notes once per candidate batch,
// not once per favorite. Visibility is still checked before including a result.
func favoriteNoteKeywordMatches(db *gorm.DB, ids []string, keyword string) (map[string]bool, error) {
	matches := map[string]bool{}
	if keyword == "" {
		return matches, nil
	}
	var notes []struct{ FavoriteID, Content string }
	if err := db.Model(&model.MessageFavoriteNoteModel{}).Select("favorite_id, content").Where("favorite_id IN ?", ids).Find(&notes).Error; err != nil {
		return nil, err
	}
	for _, note := range notes {
		if strings.Contains(strings.ToLower(note.Content), keyword) {
			matches[note.FavoriteID] = true
		}
	}
	return matches, nil
}

func MessageFavoriteList(actor string, opts MessageFavoriteListOptions) (*MessageFavoriteListResult, error) {
	db := model.GetDB()
	b, manage, err := favoriteBucket(db, actor, opts.MessageFavoriteScope, false)
	if err != nil {
		return nil, err
	}
	if opts.Limit == 0 {
		opts.Limit = 30
	}
	if opts.Limit < 1 || opts.Limit > 100 || len(opts.Query) > 2000 {
		return nil, ErrMessageFavoriteInvalid
	}
	tagIDs, err := favoriteIDs(opts.TagIDs, MessageFavoriteTagsPerItemMax)
	if err != nil {
		return nil, err
	}
	if _, err := validateFavoriteTags(db, b, tagIDs, false); err != nil {
		return nil, err
	}
	sort.Strings(tagIDs)
	keyword := strings.ToLower(strings.TrimSpace(opts.Query))
	worldFilter := strings.TrimSpace(opts.WorldID)
	keyJSON, _ := json.Marshal([]any{actor, b.scope, b.ownerID, b.worldID, worldFilter, tagIDs, keyword})
	key := fmt.Sprintf("%x", sha256.Sum256(keyJSON))
	var cursor *model.MessageFavoriteModel
	if opts.Cursor != "" {
		if len(opts.Cursor) > 2048 {
			return nil, ErrMessageFavoriteInvalid
		}
		raw, err := base64.RawURLEncoding.DecodeString(opts.Cursor)
		var c favoriteCursor
		if err != nil || json.Unmarshal(raw, &c) != nil || c.Key != key || c.ID == "" || len(c.ID) > 100 || c.CreatedAt.IsZero() {
			return nil, ErrMessageFavoriteInvalid
		}
		cursor = &model.MessageFavoriteModel{StringPKBaseModel: model.StringPKBaseModel{ID: c.ID, CreatedAt: c.CreatedAt}, SortOrder: c.Order}
	}
	resolver, err := newFavoriteMessageResolver(actor)
	if err != nil {
		return nil, err
	}
	result := &MessageFavoriteListResult{Items: []MessageFavoriteView{}, CanManageWorld: manage}
	for {
		q := b.query(db).Model(&model.MessageFavoriteModel{})
		if b.scope == model.MessageFavoriteScopePersonal && worldFilter != "" {
			q = q.Where("world_id = ?", worldFilter)
		}
		for _, id := range tagIDs {
			q = q.Where("id IN (?)", db.Model(&model.MessageFavoriteTagLinkModel{}).Select("favorite_id").Where("tag_id = ?", id))
		}
		if cursor != nil {
			q = favoritePast(q, cursor, false)
		}
		var batch []model.MessageFavoriteModel
		if err := favoriteOrdered(q, false).Limit(100).Find(&batch).Error; err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		messageIDs, channelIDs, favoriteIDs := []string{}, []string{}, []string{}
		for _, item := range batch {
			messageIDs = append(messageIDs, item.MessageID)
			channelIDs = append(channelIDs, item.ChannelID)
			favoriteIDs = append(favoriteIDs, item.ID)
		}
		messages, err := resolver.load(db, messageIDs)
		if err != nil {
			return nil, err
		}
		if err := resolver.ensureChannels(db, channelIDs); err != nil {
			return nil, err
		}
		noteMatches, err := favoriteNoteKeywordMatches(db, favoriteIDs, keyword)
		if err != nil {
			return nil, err
		}
		for i := range batch {
			item := batch[i]
			cursor = &item // Advance over rejected candidates as well as matches.
			resolved, exists := messages[item.MessageID]
			view, include := favoriteView(item, resolved, exists, resolver)
			messageMatches := keyword != "" && view.Message != nil && strings.Contains(strings.ToLower(NormalizeMessageContentToPlainText(view.Message.Content)), keyword)
			if !include || (keyword != "" && !noteMatches[item.ID] && !messageMatches) {
				continue
			}
			result.Items = append(result.Items, view)
			if len(result.Items) == opts.Limit {
				raw, _ := json.Marshal(favoriteCursor{Key: key, Order: item.SortOrder, CreatedAt: item.CreatedAt, ID: item.ID})
				result.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
				// A full page always has a cursor, including at EOF. Do not probe
				// raw row counts: hidden world rows must not influence this field.
				if err := favoriteViewDetails(db, b, result.Items); err != nil {
					return nil, err
				}
				return result, nil
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	if err := favoriteViewDetails(db, b, result.Items); err != nil {
		return nil, err
	}
	return result, nil
}
