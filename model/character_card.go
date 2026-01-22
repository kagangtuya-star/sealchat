package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// JSONMap provides a custom type for storing arbitrary JSON data
type JSONMap map[string]any

func (jm JSONMap) Value() (driver.Value, error) {
	if jm == nil {
		return []byte("{}"), nil
	}
	data, err := json.Marshal(map[string]any(jm))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (jm *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*jm = nil
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported type %T for JSONMap", value)
	}
	if len(data) == 0 {
		*jm = nil
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*jm = out
	return nil
}

type CharacterCardModel struct {
	StringPKBaseModel
	UserID    string  `json:"userId" gorm:"size:100;index:idx_character_card_user_channel,priority:1"`
	ChannelID string  `json:"channelId" gorm:"size:100;index:idx_character_card_user_channel,priority:2"`
	Name      string  `json:"name" gorm:"size:64"`
	SheetType string  `json:"sheetType" gorm:"size:32;index"`
	Attrs     JSONMap `json:"attrs" gorm:"type:json"`
}

func (*CharacterCardModel) TableName() string {
	return "character_cards"
}

func CharacterCardList(userID string, channelID string) ([]*CharacterCardModel, error) {
	var items []*CharacterCardModel
	q := db.Where("user_id = ?", userID)
	if trimmed := strings.TrimSpace(channelID); trimmed != "" {
		q = q.Where("channel_id = ?", trimmed)
	}
	err := q.Order("updated_at desc").Find(&items).Error
	return items, err
}

func CharacterCardGetByID(id string) (*CharacterCardModel, error) {
	item := &CharacterCardModel{}
	if err := db.Where("id = ?", id).Take(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func CharacterCardGetByName(userID string, channelID string, name string) (*CharacterCardModel, error) {
	item := &CharacterCardModel{}
	trimmed := strings.TrimSpace(name)
	if err := db.Where("user_id = ? AND channel_id = ? AND name = ?", userID, channelID, trimmed).Take(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func CharacterCardGetLatest(userID string, channelID string) (*CharacterCardModel, error) {
	var item CharacterCardModel
	err := db.Where("user_id = ? AND channel_id = ?", userID, channelID).
		Order("updated_at desc").
		Limit(1).
		Find(&item).Error
	if err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, nil
}

func CharacterCardCreate(item *CharacterCardModel) error {
	return db.Create(item).Error
}

func CharacterCardUpdate(id string, values map[string]any) error {
	if len(values) == 0 {
		return nil
	}
	return db.Model(&CharacterCardModel{}).Where("id = ?", id).Updates(values).Error
}

func CharacterCardDelete(id string) error {
	return db.Where("id = ?", id).Delete(&CharacterCardModel{}).Error
}

func CharacterCardBindToIdentity(identityID string, cardID string) error {
	return db.Model(&ChannelIdentityModel{}).Where("id = ?", identityID).Update("character_card_id", cardID).Error
}

func CharacterCardUnbindByCardID(cardID string) error {
	if strings.TrimSpace(cardID) == "" {
		return nil
	}
	return db.Model(&ChannelIdentityModel{}).Where("character_card_id = ?", cardID).Update("character_card_id", "").Error
}

// ToProtocolType converts CharacterCardModel to protocol.CharacterCard
func (m *CharacterCardModel) ToProtocolType() map[string]any {
	return map[string]any{
		"id":        m.ID,
		"userId":    m.UserID,
		"channelId": m.ChannelID,
		"name":      m.Name,
		"sheetType": m.SheetType,
		"attrs":     m.Attrs,
		"updatedAt": m.UpdatedAt.Unix(),
	}
}
