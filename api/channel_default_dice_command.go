package api

import (
	"sort"
	"strings"

	"sealchat/service"
	"sealchat/utils"
)

const channelDefaultDiceCommandMessageIDPrefix = "channel-default-dice-command:"

// parseChannelDefaultDiceSetCommand 识别完整的 <prefix>set <sides> 指令。
// 格式错误或不完整的内容仍按普通聊天消息处理。
func parseChannelDefaultDiceSetCommand(content string, prefixes []string) (string, bool) {
	commandText, ok := service.SerializeMessageContentToCommandText(content)
	if !ok {
		return "", false
	}
	leading := strings.TrimLeft(commandText, " \t\r\n")
	if leading == "" {
		return "", false
	}

	normalizedPrefixes := make([]string, 0, len(prefixes))
	seen := make(map[string]struct{}, len(prefixes))
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if _, exists := seen[prefix]; exists {
			continue
		}
		seen[prefix] = struct{}{}
		normalizedPrefixes = append(normalizedPrefixes, prefix)
	}
	sort.SliceStable(normalizedPrefixes, func(i, j int) bool {
		return len(normalizedPrefixes[i]) > len(normalizedPrefixes[j])
	})

	for _, prefix := range normalizedPrefixes {
		if !strings.HasPrefix(leading, prefix) {
			continue
		}
		fields := strings.Fields(leading[len(prefix):])
		if len(fields) != 2 || !strings.EqualFold(fields[0], "set") {
			return "", false
		}
		expr, err := service.NormalizeDefaultDiceExpr(fields[1])
		if err != nil {
			return "", false
		}
		return expr, true
	}
	return "", false
}

func parseConfiguredChannelDefaultDiceSetCommand(content string) (string, bool) {
	return parseChannelDefaultDiceSetCommand(content, utils.GetConfiguredBotCommandPrefixes())
}
