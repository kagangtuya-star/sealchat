package service

import (
	"errors"
	"math"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
)

type MessageFavoriteMoveInput struct {
	BeforeID string `json:"beforeId"` // Successor: place the item before this ID.
	AfterID  string `json:"afterId"`  // Predecessor: place the item after this ID.
}

func favoriteOrdered(q *gorm.DB, descending bool) *gorm.DB {
	if descending {
		return q.Order("sort_order DESC, created_at DESC, id DESC")
	}
	return q.Order("sort_order ASC, created_at ASC, id ASC")
}

func favoritePast(q *gorm.DB, item *model.MessageFavoriteModel, before bool) *gorm.DB {
	op := ">"
	if before {
		op = "<"
	}
	return q.Where("(sort_order "+op+" ? OR (sort_order = ? AND created_at "+op+" ?) OR (sort_order = ? AND created_at = ? AND id "+op+" ?))",
		item.SortOrder, item.SortOrder, item.CreatedAt, item.SortOrder, item.CreatedAt, item.ID)
}

func rebalanceFavorites(tx *gorm.DB, b messageFavoriteBucket) error {
	var items []model.MessageFavoriteModel
	if err := favoriteOrdered(b.query(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, sort_order, created_at"), false).Find(&items).Error; err != nil {
		return err
	}
	for i := range items {
		if err := b.query(tx).Model(&model.MessageFavoriteModel{}).Where("id = ?", items[i].ID).Update("sort_order", int64(i+1)*favoriteSortStep).Error; err != nil {
			return err
		}
	}
	return nil
}

func nextFavoriteSortOrder(tx *gorm.DB, b messageFavoriteBucket) (int64, error) {
	var last model.MessageFavoriteModel
	err := favoriteOrdered(b.query(tx), true).Take(&last).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return favoriteSortStep, nil
	}
	if err != nil {
		return 0, err
	}
	// Reserve room for an entire batch before its first insertion.
	if last.SortOrder > math.MaxInt64-int64(MessageFavoriteBatchMax+1)*favoriteSortStep {
		if err := rebalanceFavorites(tx, b); err != nil {
			return 0, err
		}
		if err := favoriteOrdered(b.query(tx), true).Take(&last).Error; err != nil {
			return 0, err
		}
	}
	return last.SortOrder + favoriteSortStep, nil
}

func favoriteNeighbor(tx *gorm.DB, b messageFavoriteBucket, moving string, anchor *model.MessageFavoriteModel, before bool) (*model.MessageFavoriteModel, error) {
	var item model.MessageFavoriteModel
	q := b.query(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id <> ?", moving)
	err := favoriteOrdered(favoritePast(q, anchor, before), before).Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &item, err
}

func favoriteMoveOrder(tx *gorm.DB, b messageFavoriteBucket, moving string, input MessageFavoriteMoveInput, retry bool) (int64, error) {
	load := func(id string) (*model.MessageFavoriteModel, error) {
		if id == "" {
			return nil, nil
		}
		var item model.MessageFavoriteModel
		err := b.query(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMessageFavoriteInvalid
		}
		return &item, err
	}
	left, err := load(input.AfterID)
	if err != nil {
		return 0, err
	}
	right, err := load(input.BeforeID)
	if err != nil {
		return 0, err
	}
	if left != nil {
		next, err := favoriteNeighbor(tx, b, moving, left, false)
		if err != nil {
			return 0, err
		}
		if right != nil && (next == nil || next.ID != right.ID) {
			return 0, ErrMessageFavoriteConflict
		}
		right = next
	} else {
		left, err = favoriteNeighbor(tx, b, moving, right, true)
		if err != nil {
			return 0, err
		}
	}
	var order int64
	space := true
	switch {
	case left == nil:
		space = right.SortOrder >= math.MinInt64+favoriteSortStep
		if space {
			order = right.SortOrder - favoriteSortStep
		}
	case right == nil:
		space = left.SortOrder <= math.MaxInt64-favoriteSortStep
		if space {
			order = left.SortOrder + favoriteSortStep
		}
	default:
		gap := uint64(right.SortOrder) - uint64(left.SortOrder)
		space = left.SortOrder < right.SortOrder && gap > 1
		if space {
			order = left.SortOrder + int64(gap/2)
		}
	}
	if space {
		return order, nil
	}
	if !retry {
		return 0, ErrMessageFavoriteConflict
	}
	if err := rebalanceFavorites(tx, b); err != nil {
		return 0, err
	}
	return favoriteMoveOrder(tx, b, moving, input, false)
}

func MessageFavoriteMove(actor, id string, input MessageFavoriteMoveInput) (*model.MessageFavoriteModel, error) {
	input.BeforeID, input.AfterID = strings.TrimSpace(input.BeforeID), strings.TrimSpace(input.AfterID)
	id = strings.TrimSpace(id)
	if (input.BeforeID == "" && input.AfterID == "") || input.BeforeID == id || input.AfterID == id || (input.BeforeID != "" && input.BeforeID == input.AfterID) {
		return nil, ErrMessageFavoriteInvalid
	}
	var item *model.MessageFavoriteModel
	var b messageFavoriteBucket
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var err error
		item, b, err = favoriteForWrite(tx, actor, id)
		if err != nil {
			return err
		}
		order, err := favoriteMoveOrder(tx, b, item.ID, input, true)
		if err != nil {
			return err
		}
		if err := b.query(tx).Model(&model.MessageFavoriteModel{}).Where("id = ?", item.ID).Update("sort_order", order).Error; err != nil {
			return err
		}
		return b.query(tx).Where("id = ?", item.ID).Take(item).Error
	})
	if err != nil {
		return nil, err
	}
	notifyFavoriteWorld(b)
	return item, nil
}
