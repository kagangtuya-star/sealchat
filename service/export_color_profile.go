package service

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"sealchat/model"
)

// ExportColorProfileEntry is the normalized, channel-independent profile
// shape used by export callers. It intentionally mirrors the persisted JSON
// fields without depending on the API package.
type ExportColorProfileEntry struct {
	Color        string `json:"color,omitempty"`
	Name         string `json:"name,omitempty"`
	OriginalName string `json:"originalName,omitempty"`
}

type exportColorProfileDocument struct {
	Profiles map[string]ExportColorProfileEntry `json:"profiles,omitempty"`
}

// ParseExportColorProfileJSON accepts both the structured profile document and
// the legacy identity-to-color map.
func ParseExportColorProfileJSON(raw string) map[string]ExportColorProfileEntry {
	profiles := make(map[string]ExportColorProfileEntry)
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return profiles
	}
	var doc exportColorProfileDocument
	if err := json.Unmarshal([]byte(trimmed), &doc); err == nil && len(doc.Profiles) > 0 {
		for rawKey, rawEntry := range doc.Profiles {
			key := normalizeExportProfileKey(rawKey)
			if key == "" {
				continue
			}
			rawEntry.Color = normalizeExportProfileColor(rawEntry.Color)
			rawEntry.Name = strings.TrimSpace(rawEntry.Name)
			rawEntry.OriginalName = strings.TrimSpace(rawEntry.OriginalName)
			if rawEntry.Color == "" && rawEntry.Name == "" {
				continue
			}
			profiles[key] = rawEntry
		}
		return profiles
	}
	var legacy map[string]string
	if err := json.Unmarshal([]byte(trimmed), &legacy); err != nil {
		return profiles
	}
	for rawKey, color := range legacy {
		key := normalizeExportProfileKey(rawKey)
		if key == "" {
			continue
		}
		if color = normalizeExportProfileColor(color); color != "" {
			profiles[key] = ExportColorProfileEntry{Color: color}
		}
	}
	return profiles
}

// LoadExportColorProfile resolves one channel's stored profile against active
// identities and reusable OriginalName matches. The exists flag reports
// whether this channel has a persisted record; the returned map is always the
// effective profile for the requested channel.
func LoadExportColorProfile(userID, channelID string) (map[string]ExportColorProfileEntry, bool, time.Time, error) {
	if model.GetDB() == nil {
		return map[string]ExportColorProfileEntry{}, false, time.Time{}, nil
	}
	record, err := model.ExportColorProfileGet(userID, channelID)
	if err != nil {
		return nil, false, time.Time{}, err
	}
	current := map[string]ExportColorProfileEntry{}
	exists := record != nil
	if record != nil {
		current = ParseExportColorProfileJSON(record.ColorsJSON)
	}
	resolved, err := ResolveExportColorProfiles(userID, channelID, current)
	if err != nil {
		return nil, exists, time.Time{}, err
	}
	if record == nil {
		return resolved, false, time.Time{}, nil
	}
	return resolved, true, record.UpdatedAt, nil
}

// ResolveExportColorProfiles applies current-channel identity matching and
// reusable OriginalName matching to produce the final effective map.
func ResolveExportColorProfiles(userID, channelID string, current map[string]ExportColorProfileEntry) (map[string]ExportColorProfileEntry, error) {
	options, err := model.ChannelIdentityOptionListActive(channelID)
	if err != nil {
		return nil, err
	}
	matchIndex, err := buildReusableExportProfileMatchIndex(userID)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string]ExportColorProfileEntry)
	for _, option := range options {
		if option == nil {
			continue
		}
		key := normalizeExportProfileKey("identity:" + strings.TrimSpace(option.ID))
		if key == "" {
			continue
		}
		originalName := strings.TrimSpace(option.Label)
		if originalName == "" {
			originalName = "未命名角色"
		}
		entry := current[key]
		if entry.OriginalName == "" {
			entry.OriginalName = originalName
		}
		if reusable, ok := matchIndex[NormalizeExportProfileMatchName(originalName)]; ok {
			if entry.Color == "" {
				entry.Color = reusable.Color
			}
			if entry.Name == "" {
				entry.Name = reusable.Name
			}
		}
		if entry.Color == "" && entry.Name == "" {
			continue
		}
		resolved[key] = entry
	}
	return resolved, nil
}

func buildReusableExportProfileMatchIndex(userID string) (map[string]ExportColorProfileEntry, error) {
	records, err := model.ExportColorProfileListByUser(userID)
	if err != nil {
		return nil, err
	}
	index := make(map[string]ExportColorProfileEntry)
	for _, record := range records {
		if record == nil {
			continue
		}
		profiles := ParseExportColorProfileJSON(record.ColorsJSON)
		keys := make([]string, 0, len(profiles))
		for key := range profiles {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			entry := profiles[key]
			if entry.Color == "" && entry.Name == "" {
				continue
			}
			matchKey := NormalizeExportProfileMatchName(entry.OriginalName)
			if matchKey == "" {
				continue
			}
			if _, exists := index[matchKey]; exists {
				continue
			}
			index[matchKey] = ExportColorProfileEntry{
				Color:        entry.Color,
				Name:         entry.Name,
				OriginalName: entry.OriginalName,
			}
		}
	}
	return index, nil
}

func normalizeExportProfileColor(input string) string {
	value := strings.TrimSpace(strings.ToLower(input))
	value = strings.TrimPrefix(value, "#")
	normalized, ok := normalizeHexColor(value)
	if !ok {
		return ""
	}
	return "#" + normalized
}

func normalizeExportProfileKey(rawKey string) string {
	key := strings.TrimSpace(rawKey)
	if !strings.HasPrefix(key, "identity:") {
		return ""
	}
	identityID := strings.TrimSpace(strings.TrimPrefix(key, "identity:"))
	if identityID == "" {
		return ""
	}
	return "identity:" + identityID
}

// NormalizeExportProfileMatchName canonicalizes names for reusable matching.
func NormalizeExportProfileMatchName(input string) string {
	value := strings.TrimSpace(input)
	if value == "" {
		return ""
	}
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
