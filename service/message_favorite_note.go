package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/protocol"
)

// 收藏备注帖：每帖独立署名，编辑协调使用存放在备注行上的短租约，
// 所有判断都是数据库条件更新，因此跨浏览器、跨服务进程有效；过期在访问时判断，
// 不需要后台清理任务。锁凭据只以 SHA-256 形式落库，且从不出现在列表或广播中。

const (
	MessageFavoriteNoteMax      = 2000
	MessageFavoriteNoteLockTTL  = 60 * time.Second
	favoriteNoteTokenBytes      = 32
	favoriteNoteTokenMaxLength  = 128
	favoriteNoteAcquireAttempts = 3
)

var (
	ErrMessageFavoriteNoteLocked   = errors.New("message favorite note locked")
	ErrMessageFavoriteNoteLockLost = errors.New("message favorite note lock lost")
)

// MessageFavoriteNoteLock describes another active editor without its token.
type MessageFavoriteNoteLock struct {
	UserID    string `json:"userId"`
	ExpiresAt int64  `json:"expiresAt"` // Unix milliseconds
}

// MessageFavoriteNoteLockedError keeps errors.Is(err, ErrMessageFavoriteNoteLocked)
// while carrying the current holder for the conflict response.
type MessageFavoriteNoteLockedError struct {
	Lock MessageFavoriteNoteLock
}

func (e *MessageFavoriteNoteLockedError) Error() string { return ErrMessageFavoriteNoteLocked.Error() }

func (e *MessageFavoriteNoteLockedError) Is(target error) bool {
	return target == ErrMessageFavoriteNoteLocked
}

// MessageFavoriteNoteLease is returned only to the client that holds the lease.
// Token is set on acquisition and omitted on renewal.
type MessageFavoriteNoteLease struct {
	Token      string `json:"token,omitempty"`
	ExpiresAt  int64  `json:"expiresAt"`
	TTLSeconds int    `json:"ttlSeconds"`
	Content    string `json:"content"`
}

type MessageFavoriteNoteView struct {
	model.MessageFavoriteNoteModel
	Author   *protocol.User           `json:"author,omitempty"`
	NoteLock *MessageFavoriteNoteLock `json:"noteLock,omitempty"`
}

type MessageFavoriteNoteResult struct {
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updatedAt"`
	Changed   bool      `json:"changed"`
}

func favoriteActiveNoteLock(item model.MessageFavoriteNoteModel, nowMs int64) *MessageFavoriteNoteLock {
	if item.EditLockTokenHash == "" || item.EditLockExpiresAt <= nowMs {
		return nil
	}
	return &MessageFavoriteNoteLock{UserID: item.AuthorUserID, ExpiresAt: item.EditLockExpiresAt}
}

// NormalizeMessageFavoriteNote is the shared plain-text rule: unified line
// endings, trimmed edges, valid UTF-8 without NUL, at most MessageFavoriteNoteMax
// code points.
func NormalizeMessageFavoriteNote(raw string) (string, error) {
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) {
		return "", ErrMessageFavoriteInvalid
	}
	note := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n"))
	if utf8.RuneCountInString(note) > MessageFavoriteNoteMax {
		return "", ErrMessageFavoriteInvalid
	}
	return note, nil
}

func favoriteNoteTokenHash(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > favoriteNoteTokenMaxLength {
		return "", ErrMessageFavoriteInvalid
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:]), nil
}

func newFavoriteNoteToken() (string, string, error) {
	raw := make([]byte, favoriteNoteTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate note lock token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash, err := favoriteNoteTokenHash(token)
	return token, hash, err
}

func favoriteNoteHolds(item *model.MessageFavoriteNoteModel, actor, hash string, nowMs int64) bool {
	return item.EditLockTokenHash != "" && item.AuthorUserID == actor && item.EditLockExpiresAt > nowMs &&
		subtle.ConstantTimeCompare([]byte(item.EditLockTokenHash), []byte(hash)) == 1
}

// favoriteForNote authorizes note editing without reusing world management:
// personal favorites belong to their owner; world favorites accept any
// non-spectator member of the active world who can currently see the item in
// the world list (tombstones included). Spectators stay read-only.
func favoriteForNote(db *gorm.DB, actor, id string, write bool) (*model.MessageFavoriteModel, messageFavoriteBucket, error) {
	actor, id = strings.TrimSpace(actor), strings.TrimSpace(id)
	if actor == "" {
		return nil, messageFavoriteBucket{}, ErrMessageFavoriteDenied
	}
	if id == "" || len(id) > 100 {
		return nil, messageFavoriteBucket{}, ErrMessageFavoriteInvalid
	}
	var item model.MessageFavoriteModel
	err := db.Where("id = ?", id).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && item.Scope == model.MessageFavoriteScopePersonal && item.OwnerUserID != actor) {
		return nil, messageFavoriteBucket{}, ErrMessageFavoriteNotFound
	}
	if err != nil {
		return nil, messageFavoriteBucket{}, err
	}
	b := messageFavoriteBucket{scope: model.MessageFavoriteScopePersonal, ownerID: actor}
	if item.Scope == model.MessageFavoriteScopeWorld {
		role, err := shortcutWorldMemberRole(db, item.WorldID, actor)
		switch {
		case errors.Is(err, ErrWorldNotFound):
			return nil, b, ErrMessageFavoriteNotFound
		case errors.Is(err, ErrShortcutRuleDenied):
			return nil, b, ErrMessageFavoriteDenied
		case err != nil:
			return nil, b, err
		}
		if write && role != model.WorldRoleOwner && role != model.WorldRoleAdmin && role != model.WorldRoleMember {
			return nil, b, ErrMessageFavoriteDenied
		}
		b = messageFavoriteBucket{scope: model.MessageFavoriteScopeWorld, worldID: item.WorldID}
	}
	// The list's visibility rule is the access boundary: an item hidden from
	// this actor's list cannot be reached by guessing its ID.
	resolver, err := newFavoriteMessageResolver(actor)
	if err != nil {
		return nil, b, err
	}
	messages, err := resolver.load(db, []string{item.MessageID})
	if err != nil {
		return nil, b, err
	}
	if err := resolver.ensureChannels(db, []string{item.ChannelID}); err != nil {
		return nil, b, err
	}
	resolved, exists := messages[item.MessageID]
	if _, include := favoriteView(item, resolved, exists, resolver); !include {
		return nil, b, ErrMessageFavoriteNotFound
	}
	return &item, b, nil
}

func favoriteNoteLease(item *model.MessageFavoriteNoteModel, token string, expiresAt int64) *MessageFavoriteNoteLease {
	return &MessageFavoriteNoteLease{
		Token: token, ExpiresAt: expiresAt,
		TTLSeconds: int(MessageFavoriteNoteLockTTL / time.Second), Content: item.Content,
	}
}

// Author checks never grant Owner/Admin access to somebody else's text.
func favoriteNoteForWrite(db *gorm.DB, actor, id string) (*model.MessageFavoriteNoteModel, messageFavoriteBucket, error) {
	actor, id = strings.TrimSpace(actor), strings.TrimSpace(id)
	if actor == "" || id == "" || len(id) > 100 {
		return nil, messageFavoriteBucket{}, ErrMessageFavoriteInvalid
	}
	var note model.MessageFavoriteNoteModel
	err := db.Where("id = ?", id).Take(&note).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, messageFavoriteBucket{}, ErrMessageFavoriteNotFound
	}
	if err != nil {
		return nil, messageFavoriteBucket{}, err
	}
	_, b, err := favoriteForNote(db, actor, note.FavoriteID, true)
	if err != nil {
		return nil, b, err
	}
	if note.AuthorUserID != actor {
		return nil, b, ErrMessageFavoriteDenied
	}
	return &note, b, nil
}

// Display identities are selected in one batch and serialized through the same
// public user shape as messages; private user fields never leave the service.
func favoriteNoteViews(db *gorm.DB, notes []model.MessageFavoriteNoteModel) ([]MessageFavoriteNoteView, error) {
	views := make([]MessageFavoriteNoteView, 0, len(notes))
	ids := []string{}
	for _, note := range notes {
		ids = append(ids, note.AuthorUserID)
	}
	users := []model.UserModel{}
	if len(ids) > 0 {
		if err := db.Select("id, username, nickname, avatar, is_bot").Where("id IN ?", ids).Find(&users).Error; err != nil {
			return nil, err
		}
	}
	authors := map[string]*protocol.User{}
	for _, user := range users {
		authors[user.ID] = user.ToProtocolType()
	}
	for _, note := range notes {
		views = append(views, MessageFavoriteNoteView{MessageFavoriteNoteModel: note, Author: authors[note.AuthorUserID], NoteLock: favoriteActiveNoteLock(note, time.Now().UnixMilli())})
	}
	return views, nil
}

func MessageFavoriteNoteList(actor, favoriteID string) ([]MessageFavoriteNoteView, error) {
	db := model.GetDB()
	item, _, err := favoriteForNote(db, actor, favoriteID, false)
	if err != nil {
		return nil, err
	}
	var notes []model.MessageFavoriteNoteModel
	if err := db.Where("favorite_id = ?", item.ID).Order("created_at ASC, id ASC").Find(&notes).Error; err != nil {
		return nil, err
	}
	return favoriteNoteViews(db, notes)
}

func MessageFavoriteNoteCreate(actor, favoriteID, raw string) (*MessageFavoriteNoteView, error) {
	content, err := NormalizeMessageFavoriteNote(raw)
	if err != nil || content == "" {
		return nil, ErrMessageFavoriteInvalid
	}
	db := model.GetDB()
	item, b, err := favoriteForNote(db, actor, favoriteID, true)
	if err != nil {
		return nil, err
	}
	note := model.MessageFavoriteNoteModel{FavoriteID: item.ID, AuthorUserID: strings.TrimSpace(actor), Content: content}
	err = db.Transaction(func(tx *gorm.DB) error {
		// Serialize insertion only with deletion of this parent, not with any
		// note edit lease. This prevents orphan posts during favorite deletion.
		var parent model.MessageFavoriteModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", item.ID).Take(&parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrMessageFavoriteNotFound
			}
			return err
		}
		return tx.Create(&note).Error
	})
	if err != nil {
		return nil, err
	}
	views, err := favoriteNoteViews(db, []model.MessageFavoriteNoteModel{note})
	if err != nil {
		return nil, err
	}
	notifyFavoriteWorld(b)
	return &views[0], nil
}

func MessageFavoriteNoteDelete(actor, id string) error {
	db := model.GetDB()
	note, b, err := favoriteNoteForWrite(db, actor, id)
	if err != nil {
		return err
	}
	res := db.Where("id = ? AND author_user_id = ?", note.ID, strings.TrimSpace(actor)).Delete(&model.MessageFavoriteNoteModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMessageFavoriteNotFound
	}
	notifyFavoriteWorld(b)
	return nil
}

// MessageFavoriteNoteLockAcquire acquires the edit lease, or renews it while the
// caller still holds an unexpired lease and retains note access. Acquisition and
// renewal never broadcast.
func MessageFavoriteNoteLockAcquire(actor, id, token string) (*MessageFavoriteNoteLease, error) {
	if strings.TrimSpace(token) != "" {
		return messageFavoriteNoteRenew(actor, id, token)
	}
	db := model.GetDB()
	item, _, err := favoriteNoteForWrite(db, actor, id)
	if err != nil {
		return nil, err
	}
	token, hash, err := newFavoriteNoteToken()
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < favoriteNoteAcquireAttempts; attempt++ {
		now := time.Now().UnixMilli()
		expiresAt := now + MessageFavoriteNoteLockTTL.Milliseconds()
		// Compare-and-set: only a free or expired lease can be taken.
		res := db.Model(&model.MessageFavoriteNoteModel{}).Where("id = ? AND author_user_id = ? AND edit_lock_expires_at <= ?", item.ID, strings.TrimSpace(actor), now).
			UpdateColumns(map[string]any{"edit_lock_token_hash": hash, "edit_lock_expires_at": expiresAt})
		if res.Error != nil {
			return nil, res.Error
		}
		var current model.MessageFavoriteNoteModel
		err := db.Where("id = ?", item.ID).Take(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMessageFavoriteNotFound
		}
		if err != nil {
			return nil, err
		}
		if res.RowsAffected > 0 && favoriteNoteHolds(&current, strings.TrimSpace(actor), hash, time.Now().UnixMilli()) {
			return favoriteNoteLease(&current, token, current.EditLockExpiresAt), nil
		}
		if lock := favoriteActiveNoteLock(current, time.Now().UnixMilli()); lock != nil {
			return nil, &MessageFavoriteNoteLockedError{Lock: *lock}
		}
		// The other lease expired between the update and the read; try again.
	}
	return nil, ErrMessageFavoriteConflict
}

// Renewal requires both the existing secret token and current note access.
// This prevents a removed world member from keeping a lease alive indefinitely.
func messageFavoriteNoteRenew(actor, id, token string) (*MessageFavoriteNoteLease, error) {
	actor, id = strings.TrimSpace(actor), strings.TrimSpace(id)
	hash, err := favoriteNoteTokenHash(token)
	if err != nil {
		return nil, err
	}
	if actor == "" || id == "" {
		return nil, ErrMessageFavoriteInvalid
	}
	db := model.GetDB()
	item, _, err := favoriteNoteForWrite(db, actor, id)
	if err != nil {
		if errors.Is(err, ErrMessageFavoriteDenied) || errors.Is(err, ErrMessageFavoriteNotFound) {
			return nil, ErrMessageFavoriteNoteLockLost
		}
		return nil, err
	}
	now := time.Now().UnixMilli()
	expiresAt := now + MessageFavoriteNoteLockTTL.Milliseconds()
	res := db.Model(&model.MessageFavoriteNoteModel{}).
		Where("id = ? AND edit_lock_token_hash = ? AND author_user_id = ? AND edit_lock_expires_at > ?", item.ID, hash, actor, now).
		UpdateColumn("edit_lock_expires_at", expiresAt)
	if res.Error != nil {
		return nil, res.Error
	}
	var current model.MessageFavoriteNoteModel
	err = db.Where("id = ?", item.ID).Take(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrMessageFavoriteNotFound
	}
	if err != nil {
		return nil, err
	}
	// RowsAffected can be 0 on MySQL when the value did not change, so the
	// post-read token and expiry are authoritative.
	if !favoriteNoteHolds(&current, actor, hash, time.Now().UnixMilli()) {
		return nil, ErrMessageFavoriteNoteLockLost
	}
	return favoriteNoteLease(&current, "", current.EditLockExpiresAt), nil
}

// MessageFavoriteNoteSave writes the note only while the caller still holds the
// lease, then releases it. Other clients are notified only when the content
// actually changed.
func MessageFavoriteNoteSave(actor, id, token, rawNote string) (*MessageFavoriteNoteResult, error) {
	note, err := NormalizeMessageFavoriteNote(rawNote)
	if err != nil || note == "" {
		return nil, ErrMessageFavoriteInvalid
	}
	hash, err := favoriteNoteTokenHash(token)
	if err != nil {
		return nil, err
	}
	actor = strings.TrimSpace(actor)
	db := model.GetDB()
	item, b, err := favoriteNoteForWrite(db, actor, id)
	if err != nil {
		return nil, err
	}
	result := &MessageFavoriteNoteResult{}
	err = db.Transaction(func(tx *gorm.DB) error {
		var current model.MessageFavoriteNoteModel
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", item.ID).Take(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMessageFavoriteNotFound
		}
		if err != nil {
			return err
		}
		nowMs := time.Now().UnixMilli()
		if !favoriteNoteHolds(&current, actor, hash, nowMs) {
			return ErrMessageFavoriteNoteLockLost
		}
		updates := map[string]any{"edit_lock_token_hash": "", "edit_lock_expires_at": 0}
		result.Changed = current.Content != note
		result.Content, result.UpdatedAt = current.Content, current.UpdatedAt
		if result.Changed {
			now := time.Now().UTC()
			updates["content"], updates["updated_at"] = note, now
			result.Content, result.UpdatedAt = note, now
		}
		// The token stays in the predicate so a takeover between the read and
		// this write (on databases that ignore FOR UPDATE) cannot be overwritten.
		q := tx.Model(&model.MessageFavoriteNoteModel{}).
			Where("id = ? AND edit_lock_token_hash = ? AND author_user_id = ? AND edit_lock_expires_at > ?", item.ID, hash, actor, time.Now().UnixMilli())
		if result.Changed {
			res := q.Updates(updates)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrMessageFavoriteNoteLockLost
			}
			return nil
		}
		res := q.UpdateColumns(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrMessageFavoriteNoteLockLost
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.Changed {
		notifyFavoriteWorld(b)
	}
	return result, nil
}

// MessageFavoriteNoteRelease ends the caller's lease. A stale or wrong token
// never clears another editor's lease.
func MessageFavoriteNoteRelease(actor, id, token string) error {
	actor, id = strings.TrimSpace(actor), strings.TrimSpace(id)
	hash, err := favoriteNoteTokenHash(token)
	if err != nil {
		return err
	}
	if actor == "" || id == "" {
		return ErrMessageFavoriteInvalid
	}
	db := model.GetDB()
	if _, _, err := favoriteNoteForWrite(db, actor, id); err != nil {
		return err
	}
	res := db.Model(&model.MessageFavoriteNoteModel{}).Where("id = ? AND edit_lock_token_hash = ? AND author_user_id = ? AND edit_lock_expires_at > ?", id, hash, actor, time.Now().UnixMilli()).
		UpdateColumns(map[string]any{"edit_lock_token_hash": "", "edit_lock_expires_at": 0})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	var count int64
	if err := db.Model(&model.MessageFavoriteNoteModel{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrMessageFavoriteNotFound
	}
	return ErrMessageFavoriteNoteLockLost
}
