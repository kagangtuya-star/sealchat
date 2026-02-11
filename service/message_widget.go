package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"sealchat/protocol"
)

type StateWidgetEntry struct {
	Type    string   `json:"type"`
	Options []string `json:"options"`
	Index   int      `json:"index"`
}

var stateWidgetPattern = regexp.MustCompile(`\[([^\[\]\|]+(?:\|[^\[\]\|]+)+)\]`)

// BuildStateWidgetDataFromContent parses content for [opt1|opt2|opt3] patterns
// in text nodes only, returning JSON string or empty string if none found.
func BuildStateWidgetDataFromContent(content string) string {
	entries := buildStateWidgetEntries(content)
	return marshalStateWidgetEntries(entries)
}

// BuildStateWidgetDataFromContentWithPrevious 在重建 widgetData 时尽可能保留历史索引。
// 当新旧 widget 的 options 序列一致时，继承历史 index；否则回退到默认 index=0。
func BuildStateWidgetDataFromContentWithPrevious(content string, previousWidgetData string) string {
	entries := buildStateWidgetEntries(content)
	if len(entries) == 0 {
		return ""
	}

	var previous []StateWidgetEntry
	if strings.TrimSpace(previousWidgetData) != "" {
		if err := json.Unmarshal([]byte(previousWidgetData), &previous); err == nil {
			signatureIndexes := map[string][]int{}
			for _, entry := range previous {
				sig := buildWidgetOptionsSignature(entry.Options)
				if sig == "" {
					continue
				}
				idx := entry.Index
				if idx < 0 || idx >= len(entry.Options) {
					idx = 0
				}
				signatureIndexes[sig] = append(signatureIndexes[sig], idx)
			}

			usedCount := map[string]int{}
			for i := range entries {
				sig := buildWidgetOptionsSignature(entries[i].Options)
				if sig == "" {
					continue
				}
				candidates := signatureIndexes[sig]
				if len(candidates) == 0 {
					continue
				}
				pos := usedCount[sig]
				if pos >= len(candidates) {
					continue
				}
				idx := candidates[pos]
				if idx < 0 || idx >= len(entries[i].Options) {
					idx = 0
				}
				entries[i].Index = idx
				usedCount[sig] = pos + 1
			}
		}
	}

	return marshalStateWidgetEntries(entries)
}

func buildStateWidgetEntries(content string) []StateWidgetEntry {
	var entries []StateWidgetEntry
	if LooksLikeTipTapJSON(content) {
		if plain, ok := extractTipTapPlainText(content); ok && plain != "" {
			appendStateWidgetEntriesFromText(plain, &entries)
		}
		return entries
	}

	root := protocol.ElementParse(content)
	if root == nil {
		return entries
	}

	root.Traverse(func(el *protocol.Element) {
		if el.Type != "text" {
			return
		}
		text, ok := el.Attrs["content"].(string)
		if !ok || text == "" {
			return
		}
		appendStateWidgetEntriesFromText(text, &entries)
	})

	return entries
}

func appendStateWidgetEntriesFromText(text string, entries *[]StateWidgetEntry) {
	if text == "" || entries == nil {
		return
	}

	matches := stateWidgetPattern.FindAllStringSubmatchIndex(text, -1)
	for _, loc := range matches {
		matchEnd := loc[1]
		// Skip markdown links: [a|b](url)
		if matchEnd < len(text) && text[matchEnd] == '(' {
			continue
		}

		inner := text[loc[2]:loc[3]]
		parts := strings.Split(inner, "|")
		var opts []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				opts = append(opts, p)
			}
		}
		if len(opts) < 2 {
			continue
		}

		*entries = append(*entries, StateWidgetEntry{
			Type:    "state",
			Options: opts,
			Index:   0,
		})
	}
}

func marshalStateWidgetEntries(entries []StateWidgetEntry) string {
	if len(entries) == 0 {
		return ""
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return ""
	}
	return string(data)
}

func buildWidgetOptionsSignature(options []string) string {
	if len(options) < 2 {
		return ""
	}
	normalized := make([]string, 0, len(options))
	for _, option := range options {
		v := strings.TrimSpace(option)
		if v == "" {
			continue
		}
		normalized = append(normalized, v)
	}
	if len(normalized) < 2 {
		return ""
	}
	return strings.Join(normalized, "\x1f")
}

// RotateWidgetIndex rotates the widget at widgetIndex, returning updated JSON.
func RotateWidgetIndex(widgetDataJSON string, widgetIndex int) (string, error) {
	if widgetDataJSON == "" {
		return "", errors.New("empty widget data")
	}

	var entries []StateWidgetEntry
	if err := json.Unmarshal([]byte(widgetDataJSON), &entries); err != nil {
		return "", fmt.Errorf("invalid widget data: %w", err)
	}

	if widgetIndex < 0 || widgetIndex >= len(entries) {
		return "", fmt.Errorf("widget_index %d out of range [0, %d)", widgetIndex, len(entries))
	}

	entry := &entries[widgetIndex]
	if len(entry.Options) == 0 {
		return "", errors.New("widget has no options")
	}
	if entry.Index < 0 || entry.Index >= len(entry.Options) {
		entry.Index = 0
	}
	entry.Index = (entry.Index + 1) % len(entry.Options)

	data, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
