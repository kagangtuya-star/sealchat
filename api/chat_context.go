package api

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

var (
	botCommandTailCleanupPattern = regexp.MustCompile(`(?i)(?:\s|&nbsp;|<br\s*/?>)+$`)
)

const botCommandDispatchMessageIDPrefix = "bot-command-dispatch:"
const botNicknameSyncSuppressWindowMs int64 = 3_000

var botNicknameSyncPendingByBotChannel utils.SyncMap[string, *BotNicknameSyncPending]

type ChatContext struct {
	Conn            *WsSyncConn
	User            *model.UserModel
	Members         []*model.MemberModel
	Echo            string
	ConnInfo        *ConnInfo
	OneBotSessionID string

	ChannelUsersMap *utils.SyncMap[string, *utils.SyncSet[string]]
	UserId2ConnInfo *utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]
}

func writeConnJSONAndPrune(connMap *utils.SyncMap[*WsSyncConn, *ConnInfo], conn *WsSyncConn, data any) bool {
	if conn == nil {
		if connMap != nil {
			connMap.Delete(conn)
		}
		return false
	}
	if err := conn.WriteJSON(data); err != nil {
		if connMap != nil {
			connMap.Delete(conn)
		}
		return false
	}
	return true
}

func (ctx *ChatContext) rangeChannelConnMaps(channelId string, f func(userId string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo], indexed bool) bool) {
	if ctx == nil || ctx.UserId2ConnInfo == nil || channelId == "" || f == nil {
		return
	}
	if ctx.ChannelUsersMap != nil {
		if userSet, ok := ctx.ChannelUsersMap.Load(channelId); ok && userSet != nil {
			userSet.Range(func(userId string) bool {
				connMap, ok := ctx.UserId2ConnInfo.Load(userId)
				if !ok || connMap == nil {
					return true
				}
				return f(userId, connMap, true)
			})
			return
		}
	}
	ctx.UserId2ConnInfo.Range(func(userId string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		return f(userId, connMap, false)
	})
}

func botNicknameSyncPendingKey(botUserID, channelId string) string {
	botUserID = strings.TrimSpace(botUserID)
	channelId = strings.TrimSpace(channelId)
	if botUserID == "" || channelId == "" {
		return ""
	}
	return botUserID + "\x00" + channelId
}

func storeBotNicknameSyncPendingForBot(botUserID, channelId, targetName, senderUserID string, createdAt int64) {
	key := botNicknameSyncPendingKey(botUserID, channelId)
	targetName = strings.TrimSpace(targetName)
	if key == "" || targetName == "" {
		return
	}
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}
	botNicknameSyncPendingByBotChannel.Store(key, &BotNicknameSyncPending{
		TargetName:   targetName,
		SenderUserID: strings.TrimSpace(senderUserID),
		CreatedAt:    createdAt,
	})
}

func loadBotNicknameSyncPendingForBot(botUserID, channelId string) (*BotNicknameSyncPending, bool) {
	key := botNicknameSyncPendingKey(botUserID, channelId)
	if key == "" {
		return nil, false
	}
	return botNicknameSyncPendingByBotChannel.Load(key)
}

func deleteBotNicknameSyncPendingForBot(botUserID, channelId string) {
	key := botNicknameSyncPendingKey(botUserID, channelId)
	if key == "" {
		return
	}
	botNicknameSyncPendingByBotChannel.Delete(key)
}

func shouldSuppressBotNicknameSyncContent(botUserID, channelId, content string) bool {
	pending, ok := loadBotNicknameSyncPendingForBot(botUserID, channelId)
	if !ok || pending == nil {
		return false
	}
	ageMs := time.Now().UnixMilli() - pending.CreatedAt
	if ageMs > botNicknameSyncSuppressWindowMs {
		deleteBotNicknameSyncPendingForBot(botUserID, channelId)
		return false
	}
	if !isBotNicknameSyncAckContent(content, pending.TargetName) {
		return false
	}
	deleteBotNicknameSyncPendingForBot(botUserID, channelId)
	return true
}

func (ctx *ChatContext) IsGuest() bool {
	return ctx != nil && ctx.ConnInfo != nil && ctx.ConnInfo.IsGuest
}

func (ctx *ChatContext) IsObserver() bool {
	return ctx != nil && ctx.ConnInfo != nil && ctx.ConnInfo.IsObserver
}

func (ctx *ChatContext) IsReadOnly() bool {
	return ctx.IsGuest() || ctx.IsObserver()
}

func (ctx *ChatContext) ObserverWorldID() string {
	if ctx == nil || ctx.ConnInfo == nil {
		return ""
	}
	return strings.TrimSpace(ctx.ConnInfo.ObserverWorldID)
}

func userHasChannelConnection(userId string, channelId string, userId2ConnInfo *utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]], exclude *WsSyncConn) bool {
	if userId == "" || channelId == "" || userId2ConnInfo == nil {
		return false
	}
	connMap, ok := userId2ConnInfo.Load(userId)
	if !ok || connMap == nil {
		return false
	}
	found := false
	connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
		if info == nil {
			return true
		}
		if exclude != nil && conn == exclude {
			return true
		}
		if info.ChannelId == channelId {
			found = true
			return false
		}
		return true
	})
	return found
}

func (ctx *ChatContext) BroadcastToUserJSON(userId string, data any) {
	connMap, _ := ctx.UserId2ConnInfo.Load(userId)
	if connMap == nil {
		return
	}
	connMap.Range(func(conn *WsSyncConn, _ *ConnInfo) bool {
		writeConnJSONAndPrune(connMap, conn, data)
		return true
	})
}

func (ctx *ChatContext) BroadcastJSON(data any, ignoredUserIds []string) {
	ignoredMap := make(map[string]bool)
	for _, id := range ignoredUserIds {
		ignoredMap[id] = true
	}
	ctx.UserId2ConnInfo.Range(func(userID string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		if ignoredMap[userID] {
			return true
		}
		connMap.Range(func(conn *WsSyncConn, _ *ConnInfo) bool {
			writeConnJSONAndPrune(connMap, conn, data)
			return true
		})
		return true
	})
}

func (ctx *ChatContext) BroadcastEvent(data *protocol.Event) {
	data.Timestamp = time.Now().Unix()
	ctx.UserId2ConnInfo.Range(func(_ string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		connMap.Range(func(conn *WsSyncConn, _ *ConnInfo) bool {
			writeConnJSONAndPrune(connMap, conn, struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{
				// 协议规定: 事件中必须含有 channel，message，user
				Event: *data,
				Op:    protocol.OpEvent,
			})
			return true
		})
		return true
	})
}

func (ctx *ChatContext) BroadcastEventInChannel(channelId string, data *protocol.Event) {
	data.Timestamp = time.Now().Unix()
	ctx.rangeChannelConnMaps(channelId, func(_ string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo], indexed bool) bool {
		connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info != nil && ((indexed && info.ChannelId == "") || info.ChannelId == channelId) {
				writeConnJSONAndPrune(connMap, conn, struct {
					protocol.Event
					Op protocol.Opcode `json:"op"`
				}{
					// 协议规定: 事件中必须含有 channel，message，user
					Event: *data,
					Op:    protocol.OpEvent,
				})
			}
			return true
		})
		return true
	})
}

func (ctx *ChatContext) BroadcastEventInChannelForBot(channelId string, data *protocol.Event) {
	if ctx == nil || ctx.UserId2ConnInfo == nil || channelId == "" || data == nil {
		return
	}
	data = normalizeEventForBot(data)
	data.Timestamp = time.Now().Unix()
	botIDs, err := service.EventBotIDsByChannelId(channelId)
	if err != nil {
		return
	}
	for _, botID := range botIDs {
		if x, ok := ctx.UserId2ConnInfo.Load(botID); ok {
			var activeConn *WsSyncConn
			var active *ConnInfo
			var activeAt int64 = -1
			x.Range(func(conn *WsSyncConn, value *ConnInfo) bool {
				if value == nil {
					return true
				}
				lastAlive := value.LastAliveTime
				if lastAlive == 0 {
					lastAlive = value.LastPingTime
				}
				if lastAlive > activeAt {
					activeAt = lastAlive
					activeConn = conn
					active = value
				}
				return true
			})
			if active != nil && activeConn != nil {
				cacheBotEventContext(active, channelId, data)
				writeConnJSONAndPrune(x, activeConn, struct {
					protocol.Event
					Op protocol.Opcode `json:"op"`
				}{
					// 协议规定: 事件中必须含有 channel，message，user
					Event: *data,
					Op:    protocol.OpEvent,
				})
			}
		}
		getOneBotRuntime().publishProtocolEvent(botID, data, ctx.OneBotSessionID)
	}
}

func cacheBotEventContext(info *ConnInfo, channelId string, data *protocol.Event) {
	if info == nil || channelId == "" || data == nil {
		return
	}
	whisperTargetIDs := extractBotWhisperTargetIDs(data)
	if len(whisperTargetIDs) > 0 {
		if info.BotLastWhisperTargets == nil {
			info.BotLastWhisperTargets = &utils.SyncMap[string, []string]{}
		}
		info.BotLastWhisperTargets.Store(channelId, whisperTargetIDs)
	}
	if data.MessageContext == nil {
		return
	}
	if info.BotLastMessageContext == nil {
		info.BotLastMessageContext = &utils.SyncMap[string, *protocol.MessageContext]{}
	}
	info.BotLastMessageContext.Store(channelId, data.MessageContext)
	storeBotNicknameSyncPending(info, channelId, data)
	if !data.MessageContext.IsHiddenDice || data.MessageContext.SenderUserID == "" {
		return
	}
	if info.BotHiddenDicePending == nil {
		info.BotHiddenDicePending = &utils.SyncMap[string, *BotHiddenDicePending]{}
	}
	senderUserID := strings.TrimSpace(data.MessageContext.SenderUserID)
	pendingTargets := whisperTargetIDs
	if senderUserID != "" {
		pendingTargets = normalizeWhisperTargetIDs(append(pendingTargets, senderUserID))
	}
	primaryTargetID := senderUserID
	for _, id := range whisperTargetIDs {
		id = strings.TrimSpace(id)
		if id != "" && id != senderUserID {
			primaryTargetID = id
			break
		}
	}
	info.BotHiddenDicePending.Store(channelId, &BotHiddenDicePending{
		TargetUserID:  primaryTargetID,
		TargetUserIDs: pendingTargets,
		Count:         0,
		CreatedAt:     time.Now().UnixMilli(),
	})
}

func storeBotNicknameSyncPending(info *ConnInfo, channelId string, data *protocol.Event) {
	if info == nil || channelId == "" || data == nil || data.Message == nil {
		return
	}
	if !strings.HasPrefix(strings.TrimSpace(data.Message.ID), botCommandDispatchMessageIDPrefix) {
		return
	}
	targetName, ok := extractBotNicknameSyncTarget(data.Message.Content)
	if !ok {
		return
	}
	if info.BotNicknameSyncPending == nil {
		info.BotNicknameSyncPending = &utils.SyncMap[string, *BotNicknameSyncPending]{}
	}
	senderUserID := ""
	if data.MessageContext != nil {
		senderUserID = strings.TrimSpace(data.MessageContext.SenderUserID)
	}
	createdAt := time.Now().UnixMilli()
	info.BotNicknameSyncPending.Store(channelId, &BotNicknameSyncPending{
		TargetName:   targetName,
		SenderUserID: senderUserID,
		CreatedAt:    createdAt,
	})
	if info.User != nil && info.User.IsBot {
		storeBotNicknameSyncPendingForBot(info.User.ID, channelId, targetName, senderUserID, createdAt)
	}
}

func extractBotNicknameSyncTarget(content string) (string, bool) {
	leading := strings.TrimLeft(content, " \t\r\n")
	if leading == "" {
		return "", false
	}
	for _, prefix := range resolveBotCommandPrefixes() {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" || !strings.HasPrefix(leading, prefix) {
			continue
		}
		remainder := strings.TrimSpace(leading[len(prefix):])
		if len(remainder) < 3 || !strings.EqualFold(remainder[:2], "nn") {
			return "", false
		}
		targetName := strings.TrimSpace(remainder[2:])
		if targetName == "" {
			return "", false
		}
		return targetName, true
	}
	return extractBotNicknameSyncTargetByCommandName(leading)
}

func extractBotNicknameSyncTargetByCommandName(content string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(content))
	if len(fields) < 2 {
		return "", false
	}
	command := fields[0]
	if len(command) < 3 || !strings.EqualFold(command[len(command)-2:], "nn") {
		return "", false
	}
	targetName := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(content), command))
	if targetName == "" {
		return "", false
	}
	return targetName, true
}

func normalizeEventForBot(event *protocol.Event) *protocol.Event {
	if event == nil || event.Message == nil {
		return event
	}
	if event.Type != protocol.EventMessageCreated && event.Type != protocol.EventMessageUpdated {
		return event
	}
	content := protocol.EscapeSatoriText(normalizeBotCommandContent(event.Message.Content))
	if content == event.Message.Content {
		return event
	}
	cloned := *event
	if event.Message != nil {
		msgCopy := *event.Message
		msgCopy.Content = content
		cloned.Message = &msgCopy
	}
	return &cloned
}

func normalizeBotCommandContent(content string) string {
	return normalizeBotCommandContentWithPrefixes(content, resolveBotCommandPrefixes())
}

func normalizeBotCommandContentWithPrefixes(content string, prefixes []string) string {
	leading := strings.TrimLeft(content, " \t\r\n")
	if leading == "" {
		return content
	}

	normalizedSource := content
	if serialized, ok := service.SerializeMessageContentToCommandText(content); ok && hasBotCommandPrefix(serialized, prefixes) {
		normalizedSource = serialized
	} else if !hasBotCommandPrefix(leading, prefixes) {
		return content
	}

	normalized := botCommandTailCleanupPattern.ReplaceAllString(normalizedSource, "")
	normalized = strings.TrimRight(normalized, " \t\r\n")
	if normalized == "" {
		return content
	}
	return normalized
}

func hasBotCommandPrefix(content string, prefixes []string) bool {
	leading := strings.TrimLeft(content, " \t\r\n")
	if leading == "" {
		return false
	}
	for _, prefix := range prefixes {
		trimmed := strings.TrimSpace(prefix)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(leading, trimmed) {
			return true
		}
	}
	return false
}

func resolveBotCommandPrefixes() []string {
	return utils.GetConfiguredBotCommandPrefixes()
}

func extractBotWhisperTargetIDs(event *protocol.Event) []string {
	if event == nil {
		return nil
	}
	collected := make([]string, 0, 4)
	if event.Message != nil {
		if event.Message.WhisperTo != nil {
			collected = append(collected, event.Message.WhisperTo.ID)
		}
		for _, target := range event.Message.WhisperToIds {
			if target != nil {
				collected = append(collected, target.ID)
			}
		}
	}
	if len(collected) == 0 && event.MessageContext != nil {
		collected = append(collected, event.MessageContext.WhisperToUserID)
	}
	return normalizeWhisperTargetIDs(collected)
}

func normalizeWhisperTargetIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		set[id] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func (ctx *ChatContext) BroadcastEventInChannelExcept(channelId string, ignoredUserIds []string, data *protocol.Event) {
	ignoredMap := make(map[string]struct{}, len(ignoredUserIds))
	for _, id := range ignoredUserIds {
		ignoredMap[id] = struct{}{}
	}
	data.Timestamp = time.Now().Unix()
	ctx.rangeChannelConnMaps(channelId, func(userId string, value *utils.SyncMap[*WsSyncConn, *ConnInfo], indexed bool) bool {
		if _, ignored := ignoredMap[userId]; ignored {
			return true
		}
		value.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info != nil && ((indexed && info.ChannelId == "") || info.ChannelId == channelId) {
				writeConnJSONAndPrune(value, conn, struct {
					protocol.Event
					Op protocol.Opcode `json:"op"`
				}{
					Event: *data,
					Op:    protocol.OpEvent,
				})
			}
			return true
		})
		return true
	})
}

func (ctx *ChatContext) BroadcastEventInChannelToUsers(channelId string, userIds []string, data *protocol.Event) {
	if len(userIds) == 0 {
		return
	}
	targets := make(map[string]struct{}, len(userIds))
	for _, id := range userIds {
		targets[id] = struct{}{}
	}
	if eventContainsWhisper(data) && ctx != nil && ctx.ChannelUsersMap != nil {
		if userSet, ok := ctx.ChannelUsersMap.Load(channelId); ok && userSet != nil {
			userSet.Range(func(userID string) bool {
				if _, exists := targets[userID]; exists {
					return true
				}
				if canUserReadAllWhispersInChannel(userID, channelId) {
					targets[userID] = struct{}{}
				}
				return true
			})
		}
	}
	data.Timestamp = time.Now().Unix()
	for userId := range targets {
		value, ok := ctx.UserId2ConnInfo.Load(userId)
		if !ok || value == nil {
			continue
		}
		value.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info != nil && (info.ChannelId == "" || info.ChannelId == channelId) {
				writeConnJSONAndPrune(value, conn, struct {
					protocol.Event
					Op protocol.Opcode `json:"op"`
				}{
					Event: *data,
					Op:    protocol.OpEvent,
				})
			}
			return true
		})
	}
}
