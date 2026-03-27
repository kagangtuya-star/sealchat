package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
)

const (
	OneBotEntityBotUser = "bot_user"
	OneBotEntityUser    = "user"
	OneBotEntityChannel = "channel"
	OneBotEntityMessage = "message"
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

	item = model.OneBotIDMappingModel{
		EntityType: entityType,
		EntityID:   entityID,
	}
	if err := db.Create(&item).Error; err != nil {
		var existing model.OneBotIDMappingModel
		if err2 := db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).Limit(1).Find(&existing).Error; err2 != nil {
			return 0, err
		}
		if existing.NumericID <= 0 {
			return 0, err
		}
		return existing.NumericID, nil
	}
	return item.NumericID, nil
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
