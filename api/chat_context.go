package api

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

var (
	botCommandTailCleanupPattern = regexp.MustCompile(`(?i)(?:\s|&nbsp;|<br\s*/?>)+$`)
)

type ChatContext struct {
	Conn     *WsSyncConn
	User     *model.UserModel
	Members  []*model.MemberModel
	Echo     string
	ConnInfo *ConnInfo

	ChannelUsersMap *utils.SyncMap[string, *utils.SyncSet[string]]
	UserId2ConnInfo *utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]
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
	value, _ := ctx.UserId2ConnInfo.Load(userId)
	if value == nil {
		return
	}
	value.Range(func(key *WsSyncConn, value *ConnInfo) bool {
		_ = value.Conn.WriteJSON(data)
		return true
	})
}

func (ctx *ChatContext) BroadcastJSON(data any, ignoredUserIds []string) {
	ignoredMap := make(map[string]bool)
	for _, id := range ignoredUserIds {
		ignoredMap[id] = true
	}
	ctx.UserId2ConnInfo.Range(func(key string, value *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		if ignoredMap[key] {
			return true
		}
		value.Range(func(key *WsSyncConn, value *ConnInfo) bool {
			_ = value.Conn.WriteJSON(data)
			return true
		})
		return true
	})
}

func (ctx *ChatContext) BroadcastEvent(data *protocol.Event) {
	data.Timestamp = time.Now().Unix()
	ctx.UserId2ConnInfo.Range(func(key string, value *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		value.Range(func(key *WsSyncConn, value *ConnInfo) bool {
			_ = value.Conn.WriteJSON(struct {
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
	ctx.UserId2ConnInfo.Range(func(key string, value *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		value.Range(func(key *WsSyncConn, value *ConnInfo) bool {
			if value.ChannelId == channelId {
				_ = value.Conn.WriteJSON(struct {
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
	// 只向频道选中的 BOT 推送事件，避免多 BOT 实例导致数据不同步
	data.Timestamp = time.Now().Unix()
	botID, err := service.SelectedBotIdByChannelId(channelId)
	if err != nil {
		return
	}
	if x, ok := ctx.UserId2ConnInfo.Load(botID); ok {
		var active *ConnInfo
		var activeAt int64 = -1
		x.Range(func(_ *WsSyncConn, value *ConnInfo) bool {
			if value == nil {
				return true
			}
			lastAlive := value.LastAliveTime
			if lastAlive == 0 {
				lastAlive = value.LastPingTime
			}
			if lastAlive > activeAt {
				activeAt = lastAlive
				active = value
			}
			return true
		})
		if active != nil {
			whisperTargetIDs := extractBotWhisperTargetIDs(data)
			if len(whisperTargetIDs) > 0 {
				if active.BotLastWhisperTargets == nil {
					active.BotLastWhisperTargets = &utils.SyncMap[string, []string]{}
				}
				active.BotLastWhisperTargets.Store(channelId, whisperTargetIDs)
			}
			if data.MessageContext != nil {
				if active.BotLastMessageContext == nil {
					active.BotLastMessageContext = &utils.SyncMap[string, *protocol.MessageContext]{}
				}
				active.BotLastMessageContext.Store(channelId, data.MessageContext)
				if data.MessageContext.IsHiddenDice && data.MessageContext.SenderUserID != "" {
					if active.BotHiddenDicePending == nil {
						active.BotHiddenDicePending = &utils.SyncMap[string, *BotHiddenDicePending]{}
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
					active.BotHiddenDicePending.Store(channelId, &BotHiddenDicePending{
						TargetUserID:  primaryTargetID,
						TargetUserIDs: pendingTargets,
						Count:         0,
						CreatedAt:     time.Now().UnixMilli(),
					})
				}
			}
			_ = active.Conn.WriteJSON(struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{
				// 协议规定: 事件中必须含有 channel，message，user
				Event: *data,
				Op:    protocol.OpEvent,
			})
		}
	}
}

func normalizeEventForBot(event *protocol.Event) *protocol.Event {
	if event == nil || event.Message == nil {
		return event
	}
	if event.Type != protocol.EventMessageCreated && event.Type != protocol.EventMessageUpdated {
		return event
	}
	content := normalizeBotCommandContent(event.Message.Content)
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
	leading := strings.TrimLeft(content, " \t\r\n")
	if leading == "" || service.LooksLikeTipTapJSON(leading) {
		return content
	}
	firstRune, _ := utf8.DecodeRuneInString(leading)
	switch firstRune {
	case '.', '/', '。', '．', '｡':
	default:
		return content
	}
	normalized := botCommandTailCleanupPattern.ReplaceAllString(content, "")
	normalized = strings.TrimRight(normalized, " \t\r\n")
	if normalized == "" {
		return content
	}
	return normalized
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
	ctx.UserId2ConnInfo.Range(func(userId string, value *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		if _, ignored := ignoredMap[userId]; ignored {
			return true
		}
		value.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info.ChannelId == channelId {
				_ = info.Conn.WriteJSON(struct {
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
	ctx.UserId2ConnInfo.Range(func(userId string, value *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		if _, ok := targets[userId]; !ok {
			return true
		}
		value.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info.ChannelId == channelId {
				_ = info.Conn.WriteJSON(struct {
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
