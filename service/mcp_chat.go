package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sealchat/model"
	"time"
)

type UserChatInput struct {
	ChannelID       string     `json:"channelId"`
	From            *time.Time `json:"from,omitempty"`
	To              *time.Time `json:"to,omitempty"`
	Scope           string     `json:"scope,omitempty"`
	Order           string     `json:"order,omitempty"`
	IncludeArchived bool       `json:"includeArchived,omitempty"`
	Limit           int        `json:"limit,omitempty"`
	Cursor          string     `json:"cursor,omitempty"`
}
type userChatCursor struct {
	Fingerprint string    `json:"filter"`
	Snapshot    time.Time `json:"snapshot"`
	Cursor      string    `json:"cursor"`
}
type UserChatPage struct {
	ChannelID    string             `json:"channelId"`
	ContentTrust string             `json:"contentTrust"`
	Messages     []AgentFeedMessage `json:"messages"`
	HasMore      bool               `json:"hasMore"`
	NextCursor   string             `json:"nextCursor,omitempty"`
	SnapshotTo   time.Time          `json:"snapshotTo"`
}

func userChatRequest(userID string, in UserChatInput) (AgentFeedRequest, *model.ChannelModel, string, error) {
	if !CanReadChannelByUserId(userID, in.ChannelID) {
		return AgentFeedRequest{}, nil, "", ErrWorldPermission
	}
	ch, err := model.ChannelGet(in.ChannelID)
	if err != nil {
		return AgentFeedRequest{}, nil, "", err
	}
	if in.Limit == 0 {
		in.Limit = 50
	}
	if in.Limit < 1 || in.Limit > 200 {
		return AgentFeedRequest{}, nil, "", ErrAgentFeedBadRequest
	}
	if in.Scope == "" {
		in.Scope = "all"
	}
	if in.Scope != "all" && in.Scope != "ic" && in.Scope != "ooc" {
		return AgentFeedRequest{}, nil, "", ErrAgentFeedBadRequest
	}
	if in.Order == "" {
		in.Order = "asc"
	}
	if in.Order != "asc" && in.Order != "desc" {
		return AgentFeedRequest{}, nil, "", ErrAgentFeedBadRequest
	}
	if in.From != nil && in.To != nil && !in.From.Before(*in.To) {
		return AgentFeedRequest{}, nil, "", ErrAgentFeedBadRequest
	}
	filter := in
	filter.Cursor = ""
	filter.Limit = 0
	raw, _ := json.Marshal(struct {
		UserID string
		Input  UserChatInput
	}{userID, filter})
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	req := AgentFeedRequest{viewerUserID: userID, ChannelIDs: []string{in.ChannelID}, From: in.From, To: in.To, Scope: in.Scope, Order: in.Order, IncludeArchived: in.IncludeArchived, Limit: in.Limit, Timestamp: "both", Images: "url", Dice: "structured", Merge: "none", Content: "plain", Colorizer: "none"}
	if in.Cursor != "" {
		b, err := base64.RawURLEncoding.DecodeString(in.Cursor)
		var cursor userChatCursor
		if err != nil || len(b) > 4096 || json.Unmarshal(b, &cursor) != nil || cursor.Fingerprint != fingerprint {
			return req, nil, "", ErrAgentFeedBadRequest
		}
		if cursor.Snapshot.IsZero() || cursor.Snapshot.After(time.Now()) || req.To != nil && cursor.Snapshot.After(*req.To) || req.From != nil && cursor.Snapshot.Before(*req.From) {
			return req, nil, "", ErrAgentFeedBadRequest
		}
		req.Cursor, err = DecodeAgentFeedCursor(cursor.Cursor)
		if err != nil || req.Cursor.ChannelID != in.ChannelID {
			return req, nil, "", ErrAgentFeedBadRequest
		}
		req.To = &cursor.Snapshot
	}
	if req.To == nil || req.To.After(time.Now()) {
		now := time.Now().UTC()
		req.To = &now
	}
	return req, ch, fingerprint, nil
}
func QueryUserChatMessages(userID string, in UserChatInput) (*UserChatPage, error) {
	req, ch, fingerprint, err := userChatRequest(userID, in)
	if err != nil {
		return nil, err
	}
	// A real user owns the query and formatting; no Agent profile is consulted.
	world := &model.WorldModel{}
	world.ID = ch.WorldID
	items, last, hasMore, err := loadAgentFeedChannelPage(world, AgentFeedChannel{ID: ch.ID, Name: ch.Name}, req, req)
	if err != nil {
		return nil, err
	}
	page := &UserChatPage{ChannelID: ch.ID, ContentTrust: "untrusted_user_generated", Messages: items, HasMore: hasMore, SnapshotTo: *req.To}
	if hasMore && last != nil {
		rawCursor, err := EncodeAgentFeedCursor(AgentFeedCursor{ChannelID: ch.ID, DisplayOrder: last.DisplayOrder, displayOrderSet: true, CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, err
		}
		raw, _ := json.Marshal(userChatCursor{fingerprint, *req.To, rawCursor})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}
func QueryUserChatCount(userID string, in UserChatInput) (int64, error) {
	if in.Cursor != "" {
		return 0, fmt.Errorf("%w: counts do not accept a cursor", ErrAgentFeedBadRequest)
	}
	req, _, _, err := userChatRequest(userID, in)
	if err != nil {
		return 0, err
	}
	var count int64
	err = buildAgentMessageQuery(in.ChannelID, req, nil).Count(&count).Error
	return count, err
}
