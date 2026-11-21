package model

import (
	"encoding/json"

	"sealchat/protocol"
)

// encodeChatSkinColumnValue converts a chat skin for map-based Updates, which
// bypass the field serializer. Empty skins are stored as NULL.
func encodeChatSkinColumnValue(values map[string]any, column string) error {
	raw, ok := values[column]
	if !ok {
		return nil
	}
	var skin protocol.ChatSkin
	switch value := raw.(type) {
	case nil:
	case protocol.ChatSkin:
		skin = value
	case map[string]any:
		skin = value
	case string:
		return nil
	default:
		return nil
	}
	if len(skin) == 0 {
		values[column] = nil
		return nil
	}
	encoded, err := json.Marshal(skin)
	if err != nil {
		return err
	}
	values[column] = string(encoded)
	return nil
}
