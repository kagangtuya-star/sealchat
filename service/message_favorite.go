package service

import (
	"errors"
	"strings"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
)

var (
	ErrMessageFavoriteInvalid  = errors.New("message favorite invalid")
	ErrMessageFavoriteDenied   = errors.New("message favorite denied")
	ErrMessageFavoriteNotFound = errors.New("message favorite not found")
	ErrMessageFavoriteConflict = errors.New("message favorite conflict")
)

const (
	MessageFavoriteBatchMax             = 100
	MessageFavoriteTagMax               = 100
	MessageFavoriteTagsPerItemMax       = 20
	favoriteSortStep              int64 = 1000
)

type MessageFavoriteScope struct {
	Scope   string `json:"scope"`
	WorldID string `json:"worldId"`
}

type MessageFavoriteBatchInput struct {
	MessageFavoriteScope
	MessageIDs []string `json:"messageIds"`
}

type MessageFavoriteRejection struct {
	MessageID string `json:"messageId"`
	Reason    string `json:"reason"`
}

type MessageFavoriteBatchResult struct {
	Created  []model.MessageFavoriteModel `json:"created"`
	Existing []model.MessageFavoriteModel `json:"existing"`
	Rejected []MessageFavoriteRejection   `json:"rejected"`
}

type messageFavoriteBucket struct {
	scope, ownerID, worldID string
}

func (b messageFavoriteBucket) query(db *gorm.DB) *gorm.DB {
	q := db.Where("scope = ? AND owner_user_id = ?", b.scope, b.ownerID)
	if b.scope == model.MessageFavoriteScopeWorld {
		q = q.Where("world_id = ?", b.worldID)
	}
	return q
}

func (b messageFavoriteBucket) tags(db *gorm.DB) *gorm.DB {
	return b.query(db).Model(&model.MessageFavoriteTagModel{}).Where("world_id = ?", b.worldID)
}

func favoriteBucket(db *gorm.DB, actor string, input MessageFavoriteScope, write bool) (messageFavoriteBucket, bool, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return messageFavoriteBucket{}, false, ErrMessageFavoriteDenied
	}
	scope := strings.ToLower(strings.TrimSpace(input.Scope))
	if scope == model.MessageFavoriteScopePersonal {
		return messageFavoriteBucket{scope: scope, ownerID: actor}, false, nil
	}
	worldID := strings.TrimSpace(input.WorldID)
	if scope != model.MessageFavoriteScopeWorld || worldID == "" {
		return messageFavoriteBucket{}, false, ErrMessageFavoriteInvalid
	}
	// Reuse the active-world/member role resolver used by shortcut rules. Their
	// member-write policy does not apply to favorites: only Owner/Admin may write.
	role, err := shortcutWorldMemberRole(db, worldID, actor)
	if errors.Is(err, ErrWorldNotFound) {
		return messageFavoriteBucket{}, false, ErrMessageFavoriteNotFound
	}
	if errors.Is(err, ErrShortcutRuleDenied) {
		return messageFavoriteBucket{}, false, ErrMessageFavoriteDenied
	}
	if err != nil {
		return messageFavoriteBucket{}, false, err
	}
	manage := role == model.WorldRoleOwner || role == model.WorldRoleAdmin
	if write && !manage {
		return messageFavoriteBucket{}, false, ErrMessageFavoriteDenied
	}
	return messageFavoriteBucket{scope: scope, worldID: worldID}, manage, nil
}

// Serialize bucket mutations on an existing ownership row across server
// processes. SQLite ignores FOR UPDATE and uses its database writer lock.
func lockFavoriteBucket(tx *gorm.DB, b messageFavoriteBucket) error {
	var row struct{ ID string }
	q := tx.Model(&model.UserModel{}).Where("id = ?", b.ownerID)
	if b.scope == model.MessageFavoriteScopeWorld {
		q = tx.Model(&model.WorldModel{}).Where("id = ?", b.worldID)
	}
	err := q.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMessageFavoriteNotFound
	}
	return err
}

func favoriteForWrite(tx *gorm.DB, actor, id string) (*model.MessageFavoriteModel, messageFavoriteBucket, error) {
	var item model.MessageFavoriteModel
	err := tx.Where("id = ?", strings.TrimSpace(id)).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && item.Scope == model.MessageFavoriteScopePersonal && item.OwnerUserID != actor) {
		return nil, messageFavoriteBucket{}, ErrMessageFavoriteNotFound
	}
	if err != nil {
		return nil, messageFavoriteBucket{}, err
	}
	b, _, err := favoriteBucket(tx, actor, MessageFavoriteScope{Scope: item.Scope, WorldID: item.WorldID}, true)
	if err != nil {
		return nil, b, err
	}
	if err := lockFavoriteBucket(tx, b); err != nil {
		return nil, b, err
	}
	err = b.query(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", item.ID).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrMessageFavoriteNotFound
	}
	return &item, b, err
}

func favoriteIDs(raw []string, max int) ([]string, error) {
	if len(raw) > max {
		return nil, ErrMessageFavoriteInvalid
	}
	ids := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 100 {
			return nil, ErrMessageFavoriteInvalid
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

var favoriteWorldCallback struct {
	sync.RWMutex
	fn func(string)
}

// SetMessageFavoriteWorldChangedCallback connects persisted changes to the
// existing transport broadcaster, following the TTS callback boundary.
func SetMessageFavoriteWorldChangedCallback(fn func(string)) {
	favoriteWorldCallback.Lock()
	favoriteWorldCallback.fn = fn
	favoriteWorldCallback.Unlock()
}

func notifyFavoriteWorld(b messageFavoriteBucket) {
	if b.scope != model.MessageFavoriteScopeWorld {
		return
	}
	favoriteWorldCallback.RLock()
	fn := favoriteWorldCallback.fn
	favoriteWorldCallback.RUnlock()
	if fn != nil {
		fn(b.worldID)
	}
}

func MessageFavoriteBatchCreate(actor string, input MessageFavoriteBatchInput) (*MessageFavoriteBatchResult, error) {
	ids, err := favoriteIDs(input.MessageIDs, MessageFavoriteBatchMax)
	if err != nil || len(ids) == 0 {
		return nil, ErrMessageFavoriteInvalid
	}
	db := model.GetDB()
	b, _, err := favoriteBucket(db, actor, input.MessageFavoriteScope, true)
	if err != nil {
		return nil, err
	}
	resolver, err := newFavoriteMessageResolver(actor)
	if err != nil {
		return nil, err
	}
	messages, err := resolver.load(db, ids)
	if err != nil {
		return nil, err
	}
	result := &MessageFavoriteBatchResult{Created: []model.MessageFavoriteModel{}, Existing: []model.MessageFavoriteModel{}, Rejected: []MessageFavoriteRejection{}}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := lockFavoriteBucket(tx, b); err != nil {
			return err
		}
		type messageKey struct{ world, message string }
		var existing []model.MessageFavoriteModel
		if err := b.query(tx).Where("message_id IN ?", ids).Find(&existing).Error; err != nil {
			return err
		}
		byMessage := make(map[messageKey]model.MessageFavoriteModel, len(existing))
		for _, item := range existing {
			byMessage[messageKey{item.WorldID, item.MessageID}] = item
		}
		pending := []model.MessageFavoriteModel{}
		pendingIDs := []string{}
		for _, id := range ids {
			resolved, exists := messages[id]
			reason := ""
			switch {
			case !exists:
				reason = "not_found"
			case !resolved.visible:
				reason = "not_visible"
			case resolved.message.IsDeleted:
				reason = "not_found"
			case b.scope == model.MessageFavoriteScopeWorld && resolved.worldID != b.worldID:
				reason = "wrong_world"
			case b.scope == model.MessageFavoriteScopeWorld && resolved.message.IsWhisper:
				reason = "whisper_not_allowed"
			}
			if reason != "" {
				result.Rejected = append(result.Rejected, MessageFavoriteRejection{MessageID: id, Reason: reason})
				continue
			}
			if stored, ok := byMessage[messageKey{resolved.worldID, id}]; ok {
				result.Existing = append(result.Existing, stored)
				continue
			}
			pending = append(pending, model.MessageFavoriteModel{Scope: b.scope, OwnerUserID: b.ownerID, WorldID: resolved.worldID, ChannelID: resolved.message.ChannelID, MessageID: id, CreatedByUserID: actor})
			pendingIDs = append(pendingIDs, id)
		}
		if len(pending) == 0 {
			return nil
		}
		order, err := nextFavoriteSortOrder(tx, b)
		if err != nil {
			return err
		}
		for i := range pending {
			pending[i].SortOrder = order + int64(i)*favoriteSortStep
		}
		// Database uniqueness is authoritative even across multiple processes.
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope"}, {Name: "owner_user_id"}, {Name: "world_id"}, {Name: "message_id"}}, DoNothing: true}).CreateInBatches(&pending, 50).Error; err != nil {
			return err
		}
		var storedRows []model.MessageFavoriteModel
		if err := b.query(tx).Where("message_id IN ?", pendingIDs).Find(&storedRows).Error; err != nil {
			return err
		}
		for _, item := range storedRows {
			byMessage[messageKey{item.WorldID, item.MessageID}] = item
		}
		for _, item := range pending {
			stored, ok := byMessage[messageKey{item.WorldID, item.MessageID}]
			if !ok {
				return gorm.ErrRecordNotFound
			}
			if stored.ID == item.ID {
				result.Created = append(result.Created, stored)
			} else {
				result.Existing = append(result.Existing, stored)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(result.Created) > 0 {
		notifyFavoriteWorld(b)
	}
	return result, nil
}

type MessageFavoriteBatchDeleteInput struct {
	MessageFavoriteScope
	IDs []string `json:"ids"`
}

// Missing lists IDs that no longer exist. Existing favorites outside the
// authorized bucket are rejected instead of being hidden as missing.
type MessageFavoriteBatchDeleteResult struct {
	Deleted []string `json:"deleted"`
	Missing []string `json:"missing"`
}

// MessageFavoriteBatchDelete removes favorites from one bucket in a single
// transaction. The bucket is authorized once with the same write policy as
// single-item mutations, and every predicate stays inside that bucket.
func MessageFavoriteBatchDelete(actor string, input MessageFavoriteBatchDeleteInput) (*MessageFavoriteBatchDeleteResult, error) {
	ids, err := favoriteIDs(input.IDs, MessageFavoriteBatchMax)
	if err != nil || len(ids) == 0 {
		return nil, ErrMessageFavoriteInvalid
	}
	var b messageFavoriteBucket
	result := &MessageFavoriteBatchDeleteResult{Deleted: []string{}, Missing: []string{}}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var err error
		b, _, err = favoriteBucket(tx, actor, input.MessageFavoriteScope, true)
		if err != nil {
			return err
		}
		if err := lockFavoriteBucket(tx, b); err != nil {
			return err
		}
		var rows []model.MessageFavoriteModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, scope, owner_user_id, world_id").Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return err
		}
		found := make(map[string]bool, len(rows))
		for _, row := range rows {
			belongs := row.Scope == b.scope && row.OwnerUserID == b.ownerID
			if b.scope == model.MessageFavoriteScopeWorld {
				belongs = belongs && row.WorldID == b.worldID
			}
			if !belongs {
				return ErrMessageFavoriteDenied
			}
			found[row.ID] = true
		}
		for _, id := range ids {
			if found[id] {
				result.Deleted = append(result.Deleted, id)
			} else {
				result.Missing = append(result.Missing, id)
			}
		}
		if len(result.Deleted) == 0 {
			return nil
		}
		if err := tx.Where("favorite_id IN ?", result.Deleted).Delete(&model.MessageFavoriteTagLinkModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("favorite_id IN ?", result.Deleted).Delete(&model.MessageFavoriteNoteModel{}).Error; err != nil {
			return err
		}
		return b.query(tx).Where("id IN ?", result.Deleted).Delete(&model.MessageFavoriteModel{}).Error
	})
	if err != nil {
		return nil, err
	}
	if len(result.Deleted) > 0 {
		notifyFavoriteWorld(b)
	}
	return result, nil
}

func MessageFavoriteDelete(actor, id string) error {
	var b messageFavoriteBucket
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		item, bucket, err := favoriteForWrite(tx, actor, id)
		b = bucket
		if err != nil {
			return err
		}
		if err := tx.Where("favorite_id = ?", item.ID).Delete(&model.MessageFavoriteTagLinkModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("favorite_id = ?", item.ID).Delete(&model.MessageFavoriteNoteModel{}).Error; err != nil {
			return err
		}
		return b.query(tx).Where("id = ?", item.ID).Delete(&model.MessageFavoriteModel{}).Error
	})
	if err == nil {
		notifyFavoriteWorld(b)
	}
	return err
}
