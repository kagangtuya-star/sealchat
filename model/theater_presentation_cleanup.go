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

// upgradeStoredTheaterPresentation reports whether a stored presentation is still
// supported. v2 is a lossless subset of v3, so it is kept and returned as normalized v3
// JSON; current v3 rows are kept unchanged (empty upgraded value, no UPDATE needed).
func upgradeStoredTheaterPresentation(raw string) (upgraded string, keep bool) {
	var value protocol.TheaterPresentation
	if json.Unmarshal([]byte(raw), &value) != nil {
		return "", false
	}
	if value.SchemaVersion != protocol.TheaterPresentationSchemaVersion && value.SchemaVersion != protocol.LegacyTheaterPresentationSchemaVersion {
		return "", false
	}
	normalized := protocol.NormalizeTheaterPresentation(value)
	if protocol.ValidateTheaterPresentation(normalized) != nil {
		return "", false
	}
	if value.SchemaVersion == protocol.TheaterPresentationSchemaVersion {
		return "", true
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func cleanupUnsupportedTheaterPresentations(conn *gorm.DB) error {
	for _, target := range []struct {
		model  any
		column string
	}{
		{model: &ChannelIdentityModel{}, column: "theater_presentation"},
		{model: &MessageModel{}, column: "sender_theater_presentation"},
		{model: &SharedChannelIdentityModel{}, column: "theater_presentation"},
		{model: &SharedChannelIdentityWorldPresentationModel{}, column: "theater_presentation"},
	} {
		var rows []theaterPresentationRow
		if err := conn.Model(target.model).Select("id, " + target.column + " AS value").Where(target.column + " IS NOT NULL").Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			upgraded, keep := upgradeStoredTheaterPresentation(row.Value)
			if keep && upgraded == "" {
				continue
			}
			// Supported v2 rows are rewritten as canonical v3; anything else keeps the
			// historical cleanup behavior and is cleared.
			var replacement any
			if keep {
				replacement = upgraded
			}
			if err := conn.Model(target.model).Where("id = ?", row.ID).Update(target.column, replacement).Error; err != nil {
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
