package service

import (
	"encoding/json"
	"strings"
)

// LogProofreadWorld uses the same engineering and source permissions as the LOG reader.
// The returned world is authoritative; a client-supplied worldId is never a grant.
func LogProofreadWorld(projectID, userID string) (string, error) {
	p, _, err := loadLogProjectForDesign(strings.TrimSpace(projectID), userID)
	if err != nil {
		return "", err
	}
	return p.WorldID, nil
}

// ValidateLogProofreadInput checks the full batch before a platform AI request.
func ValidateLogProofreadInput(projectID, userID, input string, versions map[string]int64) (string, error) {
	return validateLogProofreadInput(projectID, userID, input, versions, false)
}

// ValidateLogProofreadReferences checks permissions for personal API requests without receiving message text.
func ValidateLogProofreadReferences(projectID, userID, input string, versions map[string]int64) (string, error) {
	return validateLogProofreadInput(projectID, userID, input, versions, true)
}

func validateLogProofreadInput(projectID, userID, input string, versions map[string]int64, referencesOnly bool) (string, error) {
	p, _, err := loadLogProjectForDesign(strings.TrimSpace(projectID), userID)
	if err != nil {
		return "", err
	}
	var batch struct {
		Version   int `json:"version"`
		Fragments []struct {
			ID        string  `json:"id"`
			MessageID string  `json:"messageId"`
			SectionID string  `json:"sectionId"`
			Text      *string `json:"text"`
		} `json:"fragments"`
	}
	invalid := func() (string, error) { return "", logError("INVALID_INPUT", "AI 校对批次格式无效") }
	decoder := json.NewDecoder(strings.NewReader(input))
	if referencesOnly {
		decoder.DisallowUnknownFields()
	}
	if len(input) > 64*1024 || len(versions) > 20 || decoder.Decode(&batch) != nil || batch.Version != 1 || len(batch.Fragments) == 0 || len(batch.Fragments) > 256 {
		return invalid()
	}
	sections := map[string]map[string]bool{}
	ids := map[string]bool{}
	messages := map[string]bool{}
	for _, f := range batch.Fragments {
		if f.ID == "" || len(f.ID) > 200 || ids[f.ID] || f.MessageID == "" || len(f.MessageID) > 100 || f.SectionID == "" || len(f.SectionID) > 100 || (referencesOnly && f.Text != nil) || (!referencesOnly && (f.Text == nil || strings.TrimSpace(*f.Text) == "" || len([]rune(*f.Text)) > 1200)) {
			return invalid()
		}
		ids[f.ID] = true
		messages[f.MessageID] = true
		if len(messages) > 20 {
			return invalid()
		}
		if sections[f.SectionID] == nil {
			sections[f.SectionID] = map[string]bool{}
		}
		sections[f.SectionID][f.MessageID] = true
	}
	for sectionID, messageIDs := range sections {
		s, _, err := logSection(p, sectionID)
		if err != nil {
			return "", err
		}
		v, err := logSourceVersion(s.Source.ChannelID)
		if err != nil {
			return "", err
		}
		if expected, ok := versions[sectionID]; !ok || expected != v {
			return "", logError("SOURCE_CHANGED", "来源消息已变化，请重新扫描")
		}
		wanted := make([]string, 0, len(messageIDs))
		for id := range messageIDs {
			wanted = append(wanted, id)
		}
		var count int64
		if err := logSourceQuery(p, s, userID).Where("id IN ?", wanted).Count(&count).Error; err != nil {
			return "", err
		}
		if count != int64(len(wanted)) {
			return "", logError("PERMISSION_DENIED", "校对消息不在可读工程范围内")
		}
		if err := logVerifyWindow(p, s.Source.ChannelID, v); err != nil {
			return "", err
		}
	}
	return p.WorldID, nil
}
