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
	root := protocol.ElementParse(content)
	if root == nil {
		return ""
	}

	var entries []StateWidgetEntry
	root.Traverse(func(el *protocol.Element) {
		if el.Type != "text" {
			return
		}
		text, ok := el.Attrs["content"].(string)
		if !ok || text == "" {
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

			entries = append(entries, StateWidgetEntry{
				Type:    "state",
				Options: opts,
				Index:   0,
			})
		}
	})

	if len(entries) == 0 {
		return ""
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return ""
	}
	return string(data)
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
