package model

import (
	"bytes"
	"encoding/json"

	"gorm.io/gorm"

	"sealchat/protocol"
)

type theaterPresentationRow struct {
	ID    string
	Value string
}

func cleanupUnsupportedTheaterPresentations(conn *gorm.DB) error {
	for _, target := range []struct {
		model  any
		column string
	}{
		{model: &ChannelIdentityModel{}, column: "theater_presentation"},
		{model: &MessageModel{}, column: "sender_theater_presentation"},
	} {
		var rows []theaterPresentationRow
		if err := conn.Model(target.model).Select("id, " + target.column + " AS value").Where(target.column + " IS NOT NULL").Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			var value protocol.TheaterPresentation
			if json.Unmarshal([]byte(row.Value), &value) == nil && protocol.ValidateTheaterPresentation(value) == nil {
				continue
			}
			if err := conn.Model(target.model).Where("id = ?", row.ID).Update(target.column, nil).Error; err != nil {
				return err
			}
		}
	}

	var variants []struct {
		ID             string
		AppearanceJSON string
	}
	if err := conn.Model(&ChannelIdentityVariantModel{}).Select("id, appearance_json").Where("appearance_json <> ''").Scan(&variants).Error; err != nil {
		return err
	}
	for _, row := range variants {
		var document map[string]json.RawMessage
		if json.Unmarshal([]byte(row.AppearanceJSON), &document) != nil {
			continue
		}
		raw, exists := document["theaterPresentation"]
		if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			continue
		}
		var patch protocol.TheaterPresentationPatch
		if json.Unmarshal(raw, &patch) == nil && protocol.ValidateTheaterPresentationPatch(patch) == nil {
			continue
		}
		delete(document, "theaterPresentation")
		encoded, err := json.Marshal(document)
		if err != nil {
			return err
		}
		if err := conn.Model(&ChannelIdentityVariantModel{}).Where("id = ?", row.ID).Update("appearance_json", string(encoded)).Error; err != nil {
			return err
		}
	}
	return nil
}
