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
	// 获取频道下的bot，这样做的原因是，bot实际上没有进入channel这个行为，所以需要主动推送
	data.Timestamp = time.Now().Unix()
	botIds := service.BotListByChannelId(ctx.User.ID, channelId)

	for _, id := range botIds {
		if x, ok := ctx.UserId2ConnInfo.Load(id); ok {
			x.Range(func(key *WsSyncConn, value *ConnInfo) bool {
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
		}
	}
	service.ForwardOneBotEvent(channelId, data)
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
