package service

import (
	"strings"
	"unicode"
)

var remoteIDPrefixes = []string{
	"qq-group:",
	"qq:",
	"group:",
	"channel:",
	"guild:",
	"cq-group:",
}

func NormalizeOneBotRemoteID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.Trim(trimmed, "\"' ")
	lowered := strings.ToLower(trimmed)
	for _, prefix := range remoteIDPrefixes {
		if strings.HasPrefix(lowered, prefix) {
			trimmed = strings.TrimSpace(trimmed[len(prefix):])
			lowered = strings.ToLower(trimmed)
			break
		}
	}
	trimmed = strings.Trim(trimmed, "\"'() ")
	return strings.TrimSpace(trimmed)
}

func NormalizeOneBotNumericID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range trimmed {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
