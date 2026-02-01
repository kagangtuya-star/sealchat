package api

import (
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
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
			if data.MessageContext != nil {
				if active.BotLastMessageContext == nil {
					active.BotLastMessageContext = &utils.SyncMap[string, *protocol.MessageContext]{}
				}
				active.BotLastMessageContext.Store(channelId, data.MessageContext)
				if data.MessageContext.IsHiddenDice && data.MessageContext.SenderUserID != "" {
					if active.BotHiddenDicePending == nil {
						active.BotHiddenDicePending = &utils.SyncMap[string, *BotHiddenDicePending]{}
					}
					active.BotHiddenDicePending.Store(channelId, &BotHiddenDicePending{
						TargetUserID: data.MessageContext.SenderUserID,
						Count:        0,
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
