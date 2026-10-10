package service

import (
	"encoding/json"
	"strings"

	"sealchat/model"
)

func ParseStickyNoteUserIDs(raw string) map[string]struct{} {
	result := make(map[string]struct{})
	var ids []string
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &ids) != nil {
		ids = strings.Split(raw, ",")
	}
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			result[id] = struct{}{}
		}
	}
	return result
}

func CanViewStickyNote(note *model.StickyNoteModel, userID string) bool {
	if note == nil {
		return false
	}
	if note.Visibility == "" || note.Visibility == model.StickyNoteVisibilityAll {
		return true
	}
	if userID == "" {
		return false
	}
	if note.CreatorID == userID {
		return true
	}
	if _, ok := ParseStickyNoteUserIDs(note.EditorIDs)[userID]; ok {
		return true
	}
	switch note.Visibility {
	case model.StickyNoteVisibilityOwner, model.StickyNoteVisibilityEditors:
		return false
	case model.StickyNoteVisibilityViewers:
		_, ok := ParseStickyNoteUserIDs(note.ViewerIDs)[userID]
		return ok
	default:
		return true
	}
}

func EnsureStickyNoteChannelMembership(userID, channelID string) error {
	member, err := model.MemberGetByUserIDAndChannelIDBase(userID, channelID, "", false)
	if err != nil {
		return err
	}
	if member == nil {
		return ErrWorldPermission
	}
	return nil
}
