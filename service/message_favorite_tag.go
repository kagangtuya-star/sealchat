package service

import (
	"errors"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
)

type MessageFavoriteTagInput struct {
	MessageFavoriteScope
	Name string `json:"name"`
}

type MessageFavoriteTagListResult struct {
	Items          []model.MessageFavoriteTagModel `json:"items"`
	CanManageWorld bool                            `json:"canManageWorld"`
}

func favoriteTagName(raw string) (string, string, error) {
	name := strings.TrimSpace(raw)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 32 {
		return "", "", ErrMessageFavoriteInvalid
	}
	return name, strings.ToLower(name), nil
}

func favoriteTagWriteError(err error) error {
	if err == nil {
		return nil
	}
	text := strings.ToLower(err.Error())
	if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(text, "idx_message_favorite_tag_unique") || strings.Contains(text, "unique constraint failed: message_favorite_tags.") {
		return ErrMessageFavoriteConflict
	}
	return err
}

func MessageFavoriteTagList(actor string, input MessageFavoriteScope) (*MessageFavoriteTagListResult, error) {
	b, manage, err := favoriteBucket(model.GetDB(), actor, input, false)
	if err != nil {
		return nil, err
	}
	result := &MessageFavoriteTagListResult{Items: []model.MessageFavoriteTagModel{}, CanManageWorld: manage}
	err = b.tags(model.GetDB()).Order("normalized_name ASC, id ASC").Find(&result.Items).Error
	return result, err
}

func MessageFavoriteTagCreate(actor string, input MessageFavoriteTagInput) (*model.MessageFavoriteTagModel, error) {
	name, normalized, err := favoriteTagName(input.Name)
	if err != nil {
		return nil, err
	}
	var b messageFavoriteBucket
	var item model.MessageFavoriteTagModel
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var err error
		b, _, err = favoriteBucket(tx, actor, input.MessageFavoriteScope, true)
		if err != nil {
			return err
		}
		if err := lockFavoriteBucket(tx, b); err != nil {
			return err
		}
		// Locking reads see committed rows after acquiring the ownership lock,
		// including on MySQL's default REPEATABLE READ isolation level.
		var existing []model.MessageFavoriteTagModel
		if err := b.tags(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, normalized_name").Order("id ASC").Limit(MessageFavoriteTagMax).Find(&existing).Error; err != nil {
			return err
		}
		for _, tag := range existing {
			if tag.NormalizedName == normalized {
				return ErrMessageFavoriteConflict
			}
		}
		if len(existing) >= MessageFavoriteTagMax {
			return ErrMessageFavoriteInvalid
		}
		item = model.MessageFavoriteTagModel{Scope: b.scope, OwnerUserID: b.ownerID, WorldID: b.worldID, Name: name, NormalizedName: normalized, CreatedByUserID: actor}
		return favoriteTagWriteError(tx.Create(&item).Error)
	})
	if err != nil {
		return nil, err
	}
	notifyFavoriteWorld(b)
	return &item, nil
}

func favoriteTagForWrite(tx *gorm.DB, actor, id string) (*model.MessageFavoriteTagModel, messageFavoriteBucket, error) {
	var tag model.MessageFavoriteTagModel
	err := tx.Where("id = ?", strings.TrimSpace(id)).Take(&tag).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && tag.Scope == model.MessageFavoriteScopePersonal && tag.OwnerUserID != actor) {
		return nil, messageFavoriteBucket{}, ErrMessageFavoriteNotFound
	}
	if err != nil {
		return nil, messageFavoriteBucket{}, err
	}
	b, _, err := favoriteBucket(tx, actor, MessageFavoriteScope{Scope: tag.Scope, WorldID: tag.WorldID}, true)
	if err != nil {
		return nil, b, err
	}
	if err := lockFavoriteBucket(tx, b); err != nil {
		return nil, b, err
	}
	err = b.tags(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", tag.ID).Take(&tag).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = ErrMessageFavoriteNotFound
	}
	return &tag, b, err
}

func MessageFavoriteTagRename(actor, id, rawName string) (*model.MessageFavoriteTagModel, error) {
	name, normalized, err := favoriteTagName(rawName)
	if err != nil {
		return nil, err
	}
	var tag *model.MessageFavoriteTagModel
	var b messageFavoriteBucket
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var err error
		tag, b, err = favoriteTagForWrite(tx, actor, id)
		if err != nil {
			return err
		}
		if err := favoriteTagWriteError(b.tags(tx).Where("id = ?", tag.ID).Updates(map[string]any{"name": name, "normalized_name": normalized}).Error); err != nil {
			return err
		}
		return b.tags(tx).Where("id = ?", tag.ID).Take(tag).Error
	})
	if err != nil {
		return nil, err
	}
	notifyFavoriteWorld(b)
	return tag, nil
}

func MessageFavoriteTagDelete(actor, id string) error {
	var b messageFavoriteBucket
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		tag, bucket, err := favoriteTagForWrite(tx, actor, id)
		b = bucket
		if err != nil {
			return err
		}
		if err := tx.Where("tag_id = ?", tag.ID).Delete(&model.MessageFavoriteTagLinkModel{}).Error; err != nil {
			return err
		}
		return b.tags(tx).Where("id = ?", tag.ID).Delete(&model.MessageFavoriteTagModel{}).Error
	})
	if err == nil {
		notifyFavoriteWorld(b)
	}
	return err
}

func validateFavoriteTags(db *gorm.DB, b messageFavoriteBucket, ids []string, lock bool) ([]model.MessageFavoriteTagModel, error) {
	tags := []model.MessageFavoriteTagModel{}
	if len(ids) == 0 {
		return tags, nil
	}
	q := b.tags(db)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.Where("id IN ?", ids).Order("normalized_name ASC, id ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	if len(tags) != len(ids) {
		return nil, ErrMessageFavoriteInvalid
	}
	return tags, nil
}

func MessageFavoriteReplaceTags(actor, id string, rawIDs []string) ([]model.MessageFavoriteTagModel, error) {
	ids, err := favoriteIDs(rawIDs, MessageFavoriteTagsPerItemMax)
	if err != nil {
		return nil, err
	}
	var tags []model.MessageFavoriteTagModel
	var b messageFavoriteBucket
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		item, bucket, err := favoriteForWrite(tx, actor, id)
		b = bucket
		if err != nil {
			return err
		}
		tags, err = validateFavoriteTags(tx, b, ids, true)
		if err != nil {
			return err
		}
		if err := tx.Where("favorite_id = ?", item.ID).Delete(&model.MessageFavoriteTagLinkModel{}).Error; err != nil {
			return err
		}
		links := make([]model.MessageFavoriteTagLinkModel, 0, len(ids))
		for _, tagID := range ids {
			links = append(links, model.MessageFavoriteTagLinkModel{FavoriteID: item.ID, TagID: tagID})
		}
		if len(links) > 0 {
			return tx.Create(&links).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	notifyFavoriteWorld(b)
	return tags, nil
}
