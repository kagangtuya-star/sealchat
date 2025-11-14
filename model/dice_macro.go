package model

import "gorm.io/gorm"

type DiceMacroModel struct {
	StringPKBaseModel
	UserID    string `json:"userId" gorm:"index:idx_dice_macro_user_channel,priority:1;size:32"`
	ChannelID string `json:"channelId" gorm:"index:idx_dice_macro_user_channel,priority:2;size:32"`
	Digits    string `json:"digits" gorm:"size:32"`
	Label     string `json:"label" gorm:"size:64"`
	Expr      string `json:"expr" gorm:"size:255"`
	Note      string `json:"note" gorm:"size:255"`
	Favorite  bool   `json:"favorite"`
}

func DiceMacroList(userID string, channelID string) ([]*DiceMacroModel, error) {
	var items []*DiceMacroModel
	err := GetDB().Where("user_id = ? AND channel_id = ?", userID, channelID).
		Order("favorite desc").
		Order("updated_at desc").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

func DiceMacroGetByID(id string) (*DiceMacroModel, error) {
	item := &DiceMacroModel{}
	if err := GetDB().Where("id = ?", id).Take(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func DiceMacroSave(item *DiceMacroModel) error {
	return GetDB().Save(item).Error
}

func DiceMacroUpdate(id string, values map[string]any) error {
	return GetDB().Model(&DiceMacroModel{}).Where("id = ?", id).Updates(values).Error
}

func DiceMacroDelete(id string) error {
	return GetDB().Where("id = ?", id).Delete(&DiceMacroModel{}).Error
}

func DiceMacroDeleteByChannel(tx *gorm.DB, userID string, channelID string) error {
	if tx == nil {
		tx = GetDB()
	}
	return tx.Where("user_id = ? AND channel_id = ?", userID, channelID).Delete(&DiceMacroModel{}).Error
}
