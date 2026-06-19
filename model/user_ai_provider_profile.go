package model

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/utils"
)

type UserAIProviderProfileModel struct {
	StringPKBaseModel
	UserID        string           `json:"userId" gorm:"index:idx_user_ai_provider_user,not null"`
	Name          string           `json:"name" gorm:"size:128;not null"`
	BaseURL       string           `json:"baseUrl" gorm:"size:255;not null"`
	APIKey        string           `json:"apiKey" gorm:"size:512;not null"`
	Models        JSONList[string] `json:"models" gorm:"type:json"`
	SelectedModel string           `json:"selectedModel" gorm:"size:255;not null;default:''"`
	Enabled       bool             `json:"enabled" gorm:"not null;default:true"`
	Sort          int              `json:"sort" gorm:"not null;default:0"`
}

func (*UserAIProviderProfileModel) TableName() string {
	return "user_ai_provider_profiles"
}

func UserAIProviderProfileList(userID string) ([]*UserAIProviderProfileModel, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return []*UserAIProviderProfileModel{}, nil
	}
	var items []*UserAIProviderProfileModel
	if err := db.Where("user_id = ?", userID).
		Order("sort ASC").
		Order("created_at ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func UserAIProviderProfileReplace(userID string, items []*UserAIProviderProfileModel) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&UserAIProviderProfileModel{}).Error; err != nil {
			return err
		}
		for index, item := range items {
			if item == nil {
				continue
			}
			item.UserID = userID
			item.Sort = index
			if strings.TrimSpace(item.ID) == "" {
				item.StringPKBaseModel = StringPKBaseModel{ID: utils.NewID()}
			}
			if err := tx.Clauses(clause.OnConflict{
				UpdateAll: true,
			}).Create(item).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func UserAIProviderProfileUpsert(userID string, items []*UserAIProviderProfileModel) ([]*UserAIProviderProfileModel, error) {
	if err := UserAIProviderProfileReplace(userID, items); err != nil {
		return nil, err
	}
	return UserAIProviderProfileList(userID)
}
