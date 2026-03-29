package service

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
)

const (
	OneBotEntityBotUser = "bot_user"
	OneBotEntityUser    = "user"
	OneBotEntityChannel = "channel"
	OneBotEntityMessage = "message"

	oneBotRandomIDRetryLimit = 16
)

var ErrOneBotMappingNotFound = errors.New("onebot mapping not found")

func normalizeOneBotEntityType(entityType string) string {
	return strings.TrimSpace(strings.ToLower(entityType))
}

func validateOneBotEntityType(entityType string) bool {
	switch normalizeOneBotEntityType(entityType) {
	case OneBotEntityBotUser, OneBotEntityUser, OneBotEntityChannel, OneBotEntityMessage:
		return true
	default:
		return false
	}
}

func oneBotNumericIDRange(entityType string) (int64, int64, bool) {
	switch normalizeOneBotEntityType(entityType) {
	case OneBotEntityBotUser:
		return 1_000_000_000, 9_999_999_999, true
	case OneBotEntityUser, OneBotEntityChannel:
		return 1_000_000_000_000_000, 9_999_999_999_999_999, true
	default:
		return 0, 0, false
	}
}

func generateRandomOneBotNumericID(minValue, maxValue int64) (int64, error) {
	if minValue <= 0 || maxValue < minValue {
		return 0, errors.New("invalid onebot numeric id range")
	}

	span := maxValue - minValue + 1
	value, err := rand.Int(rand.Reader, big.NewInt(span))
	if err != nil {
		return 0, err
	}
	return minValue + value.Int64(), nil
}

func GetOrCreateOneBotID(entityType, entityID string) (int64, error) {
	entityType = normalizeOneBotEntityType(entityType)
	entityID = strings.TrimSpace(entityID)
	if !validateOneBotEntityType(entityType) || entityID == "" {
		return 0, errors.New("invalid onebot entity")
	}

	db := model.GetDB()
	var item model.OneBotIDMappingModel
	if err := db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).Limit(1).Find(&item).Error; err != nil {
		return 0, err
	}
	if item.NumericID > 0 {
		return item.NumericID, nil
	}

	minValue, maxValue, useRandomID := oneBotNumericIDRange(entityType)
	attempts := 1
	if useRandomID {
		attempts = oneBotRandomIDRetryLimit
	}

	for attempt := 0; attempt < attempts; attempt++ {
		item = model.OneBotIDMappingModel{
			EntityType: entityType,
			EntityID:   entityID,
		}
		if useRandomID {
			numericID, err := generateRandomOneBotNumericID(minValue, maxValue)
			if err != nil {
				return 0, err
			}
			item.NumericID = numericID
		}

		if err := db.Create(&item).Error; err != nil {
			var existing model.OneBotIDMappingModel
			if err2 := db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).Limit(1).Find(&existing).Error; err2 != nil {
				return 0, err
			}
			if existing.NumericID > 0 {
				return existing.NumericID, nil
			}
			if !useRandomID {
				return 0, err
			}

			var numericMatch model.OneBotIDMappingModel
			if err2 := db.Where("numeric_id = ?", item.NumericID).Limit(1).Find(&numericMatch).Error; err2 != nil {
				return 0, err
			}
			if numericMatch.NumericID > 0 {
				continue
			}
			return 0, err
		}
		return item.NumericID, nil
	}

	return 0, errors.New("failed to allocate onebot numeric id")
}

func ResolveInternalID(entityType string, numericID int64) (string, error) {
	entityType = normalizeOneBotEntityType(entityType)
	if !validateOneBotEntityType(entityType) || numericID <= 0 {
		return "", ErrOneBotMappingNotFound
	}

	var item model.OneBotIDMappingModel
	err := model.GetDB().
		Where("numeric_id = ? AND entity_type = ?", numericID, entityType).
		Limit(1).
		Find(&item).Error
	if err != nil {
		return "", err
	}
	if item.NumericID <= 0 || item.EntityID == "" {
		return "", ErrOneBotMappingNotFound
	}
	return item.EntityID, nil
}

func IsOneBotMappingNotFound(err error) bool {
	return errors.Is(err, ErrOneBotMappingNotFound) || errors.Is(err, gorm.ErrRecordNotFound)
}
