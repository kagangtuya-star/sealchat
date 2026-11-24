package service

import (
	"encoding/json"

	"gorm.io/gorm"
	"sealchat/model"
)

// LogMessageReorderDraft only changes element placements, never draft content or sources.
type LogMessageReorderDraft struct {
	ProjectID       string                `json:"projectId"`
	SectionID       string                `json:"sectionId"`
	ExpectedVersion int64                 `json:"expectedVersion"`
	Placements      []LogElementPlacement `json:"placements"`
}

type LogElementPlacement struct {
	ElementID string    `json:"elementId"`
	Anchor    LogAnchor `json:"anchor"`
	Order     int64     `json:"order"`
}

type LogMessageReorderCommit struct {
	projectID string
	version   int64
	draftJSON string
}

// PrepareLogMessageReorder checks design access before entering the message write transaction.
// Apply must run in that transaction so neither the message nor its element anchors can commit alone.
func PrepareLogMessageReorder(userID, channelID, messageID string, input LogMessageReorderDraft) (*LogMessageReorderCommit, error) {
	return PrepareLogMessagesReorder(userID, channelID, []string{messageID}, input)
}

func PrepareLogMessagesReorder(userID, channelID string, messageIDs []string, input LogMessageReorderDraft) (*LogMessageReorderCommit, error) {
	p, _, err := loadLogProjectForDesign(input.ProjectID, userID)
	if err != nil {
		return nil, err
	}
	if input.ExpectedVersion < 1 || input.ExpectedVersion >= LogMaxVersion || len(input.Placements) == 0 || len(input.Placements) > 10000 {
		return nil, logError("INVALID_INPUT", "元素落点无效")
	}
	if p.DraftVersion != input.ExpectedVersion {
		return nil, logConflict(p.DraftVersion)
	}
	section, _, err := logSection(p, input.SectionID)
	if err != nil {
		return nil, err
	}
	if section.Source.ChannelID != channelID {
		return nil, logError("PERMISSION_DENIED", "消息不属于当前来源片段")
	}
	var count int64
	owners := map[string]bool{}
	for _, id := range messageIDs {
		owners[id] = true
	}
	if err := logSourceQuery(p, section, userID).Where("id IN ?", messageIDs).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 || count != int64(len(messageIDs)) {
		return nil, logError("NOT_FOUND", "消息不在当前来源范围内")
	}
	// Keep every other field byte-for-byte at the JSON value level, including extension data.
	var draft map[string]json.RawMessage
	if err := json.Unmarshal([]byte(p.DraftJSON), &draft); err != nil {
		return nil, err
	}
	var elements []map[string]json.RawMessage
	if err := json.Unmarshal(draft["elements"], &elements); err != nil {
		return nil, err
	}
	byID := map[string]map[string]json.RawMessage{}
	for _, element := range elements {
		var id string
		if err := json.Unmarshal(element["id"], &id); err != nil {
			return nil, err
		}
		byID[id] = element
	}
	seen := map[string]bool{}
	for _, placement := range input.Placements {
		element := byID[placement.ElementID]
		if element == nil || seen[placement.ElementID] || placement.Order < 0 || placement.Order > LogMaxVersion {
			return nil, logError("INVALID_INPUT", "元素落点无效")
		}
		seen[placement.ElementID] = true
		var oldAnchor LogAnchor
		if err := json.Unmarshal(element["anchor"], &oldAnchor); err != nil {
			return nil, err
		}
		a := placement.Anchor
		if oldAnchor.SectionID != input.SectionID || a.SectionID != input.SectionID || a.Kind != "message" || !owners[a.MessageID] || (a.Side != "before" && a.Side != "after") {
			return nil, logError("INVALID_INPUT", "元素落点不能跨来源片段")
		}
		element["anchor"], err = json.Marshal(a)
		if err != nil {
			return nil, err
		}
		element["order"], err = json.Marshal(placement.Order)
		if err != nil {
			return nil, err
		}
	}
	draft["elements"], err = json.Marshal(elements)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(draft)
	if err != nil {
		return nil, err
	}
	if len(encoded) > 2<<20 {
		return nil, logError("TOO_LARGE", "草稿超过 2MB")
	}
	return &LogMessageReorderCommit{projectID: p.ID, version: p.DraftVersion, draftJSON: string(encoded)}, nil
}

func (c *LogMessageReorderCommit) Apply(tx *gorm.DB) error {
	r := tx.Model(&model.LogProjectModel{}).Where("id = ? AND enabled = ? AND draft_version = ?", c.projectID, true, c.version).
		Updates(map[string]any{"draft_json": c.draftJSON, "draft_version": gorm.Expr("draft_version + 1")})
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return logConflict(c.version)
	}
	return nil
}

func (c *LogMessageReorderCommit) DraftVersion() int64 { return c.version + 1 }

func LogDraftChangeRecipients(projectID string, online func(string) bool) []string {
	db := model.GetDB()
	world := db.Model(&model.LogProjectModel{}).Select("world_id").Where("id = ? AND enabled = ?", projectID, true)
	var members []string
	if err := db.Model(&model.WorldMemberModel{}).Where("world_id IN (?)", world).Pluck("user_id", &members).Error; err != nil {
		return nil
	}
	var recipients []string
	for _, id := range members {
		if online(id) {
			if _, _, err := loadLogProjectForDesign(projectID, id); err == nil {
				recipients = append(recipients, id)
			}
		}
	}
	return recipients
}
