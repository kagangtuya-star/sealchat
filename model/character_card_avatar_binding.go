package model

import "strings"

type CharacterCardAvatarBindingModel struct {
	StringPKBaseModel
	UserID             string `json:"userId" gorm:"size:100;index:idx_cc_avatar_binding_unique,priority:1"`
	ChannelID          string `json:"channelId" gorm:"size:100;index:idx_cc_avatar_binding_unique,priority:2;index"`
	ExternalCardID     string `json:"externalCardId" gorm:"size:100;index:idx_cc_avatar_binding_unique,priority:3"`
	CardName           string `json:"cardName" gorm:"size:64"`
	SheetType          string `json:"sheetType" gorm:"size:32;index"`
	AvatarAttachmentID string `json:"avatarAttachmentId" gorm:"size:100"`
}

func (*CharacterCardAvatarBindingModel) TableName() string {
	return "character_card_avatar_bindings"
}

func CharacterCardAvatarBindingList(userID string, channelID string) ([]*CharacterCardAvatarBindingModel, error) {
	var items []*CharacterCardAvatarBindingModel
	err := db.Where("user_id = ? AND channel_id = ?", userID, strings.TrimSpace(channelID)).
		Order("updated_at desc").
		Find(&items).Error
	return items, err
}

func CharacterCardAvatarBindingGet(userID string, channelID string, externalCardID string) (*CharacterCardAvatarBindingModel, error) {
	item := &CharacterCardAvatarBindingModel{}
	err := db.Where("user_id = ? AND channel_id = ? AND external_card_id = ?", userID, strings.TrimSpace(channelID), strings.TrimSpace(externalCardID)).
		Take(item).Error
	if err != nil {
		return nil, err
	}
	return item, nil
}

func CharacterCardAvatarBindingCreate(item *CharacterCardAvatarBindingModel) error {
	return db.Create(item).Error
}

func CharacterCardAvatarBindingUpdate(id string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	return db.Model(&CharacterCardAvatarBindingModel{}).Where("id = ?", id).Updates(values).Error
}

func CharacterCardAvatarBindingDelete(id string) error {
	return db.Where("id = ?", id).Delete(&CharacterCardAvatarBindingModel{}).Error
}
