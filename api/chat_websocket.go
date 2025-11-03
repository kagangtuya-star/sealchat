package api

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

type ApiMsgPayload struct {
	Api  string `json:"api"`
	Echo string `json:"echo"`
}

type WsSyncConn struct {
	*websocket.Conn
	Mux sync.RWMutex
}

func (c *WsSyncConn) WriteJSON(v interface{}) error {
	c.Mux.Lock()
	defer c.Mux.Unlock()
	return c.Conn.WriteJSON(v)
}

type ConnInfo struct {
	User            *model.UserModel
	Conn            *WsSyncConn
	LastPingTime    int64
	ChannelId       string
	TypingEnabled   bool
	TypingState     protocol.TypingState
	TypingContent   string
	TypingUpdatedAt int64
}

var commandTips utils.SyncMap[string, map[string]string]

func websocketWorks(app *fiber.App) {
	channelUsersMap := &utils.SyncMap[string, *utils.SyncSet[string]]{}
	userId2ConnInfo := &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{}

	clientEnter := func(c *WsSyncConn, body any) (curUser *model.UserModel, curConnInfo *ConnInfo) {
		if body != nil {
			// 有身份信息
			m, ok := body.(map[string]any)
			if !ok {
				return nil, nil
			}
			tokenAny, exists := m["token"]
			if !exists {
				return nil, nil
			}
			token, ok := tokenAny.(string)
			if !ok {
				return nil, nil
			}

			var user *model.UserModel
			var err error

			if len(token) == 32 {
				user, err = model.BotVerifyAccessToken(token)
			} else {
				user, err = model.UserVerifyAccessToken(token)
			}

			if err == nil {
				m, _ := userId2ConnInfo.LoadOrStore(user.ID, &utils.SyncMap[*WsSyncConn, *ConnInfo]{})
				curConnInfo = &ConnInfo{Conn: c, LastPingTime: time.Now().Unix(), User: user, TypingState: protocol.TypingStateSilent}
				m.Store(c, curConnInfo)

				curUser = user
				_ = c.WriteJSON(protocol.GatewayPayloadStructure{
					Op: protocol.OpReady,
					Body: map[string]any{
						"user": curUser,
					},
				})
				return
			}
		}

		_ = c.WriteJSON(protocol.GatewayPayloadStructure{
			Op: protocol.OpReady,
			Body: map[string]any{
				"errorMsg": "no auth",
			},
		})
		return nil, nil
	}

	go func() {
		// 持续删除超时连接
		// for {
		//	time.Sleep(5 * time.Second)
		//	now := time.Now().Unix()
		//	oldLen := connMap.Len()
		//	connMap.Range(func(key string, value *ConnInfo) bool {
		//		if now-value.LastPingTime > 20 {
		//			_ = value.Conn.Close()
		//			connMap.Delete(key)
		//
		//			channelUsersMap.Range(func(chId string, value *utils.SyncSet[string]) bool {
		//				value.Delete(key)
		//				return true
		//			})
		//		}
		//		return true
		//	})
		//
		//	if connMap.Len()-oldLen != 0 {
		//		ctx := &ChatContext{
		//			ConnMap:         connMap,
		//			ChannelUsersMap: channelUsersMap,
		//		}
		//		ctx.BroadcastEvent(&protocol.Event{
		//			Type: "channel-updated",
		//		})
		//	}
		// }
	}()

	app.Use("/ws", func(c *fiber.Ctx) error {
		// IsWebSocketUpgrade returns true if the client
		// requested upgrade to the WebSocket protocol.
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/ws/seal", websocket.New(func(rawConn *websocket.Conn) {
		// websocket.Conn bindings https://pkg.go.dev/github.com/fasthttp/websocket?tab=doc#pkg-index
		var (
			mt          int
			msg         []byte
			err         error
			curUser     *model.UserModel
			curConnInfo *ConnInfo
		)
		c := &WsSyncConn{rawConn, sync.RWMutex{}}

		for {
			if mt, msg, err = c.ReadMessage(); err != nil {
				log.Println("read:", err)
				// 解析错误
				break
			}

			solved := false
			gatewayMsg := protocol.GatewayPayloadStructure{}
			err := json.Unmarshal(msg, &gatewayMsg)
			if err == nil {
				// 信令
				switch gatewayMsg.Op {
				case protocol.OpIdentify:
					fmt.Println("新客户端接入")
					curUser, curConnInfo = clientEnter(c, gatewayMsg.Body)
					if curUser == nil {
						_ = c.Close()
						return
					}
					solved = true
				case protocol.OpPing:
					if curUser == nil {
						solved = true
						continue
					}
					if info, ok := userId2ConnInfo.Load(curUser.ID); ok {
						if info2, ok := info.Load(c); ok {
							info2.LastPingTime = time.Now().Unix()
						}
					}
					_ = c.WriteJSON(protocol.GatewayPayloadStructure{
						Op: protocol.OpPong,
					})
					solved = true
				}
			}

			if !solved {
				apiMsg := ApiMsgPayload{}
				err := json.Unmarshal(msg, &apiMsg)

				var members []*model.MemberModel
				db := model.GetDB()
				db.Where("user_id = ?", curUser.ID).Find(&members)

				ctx := &ChatContext{
					Conn:            c,
					User:            curUser,
					Echo:            apiMsg.Echo,
					ConnInfo:        curConnInfo,
					Members:         members,
					ChannelUsersMap: channelUsersMap,
					UserId2ConnInfo: userId2ConnInfo,
				}

				if err == nil {
					// 频道相关的非自设API基本都是改为不再需要传入guild_id
					switch apiMsg.Api {
					case "channel.create":
						apiWrap(ctx, msg, apiChannelCreate)
						solved = true
					case "channel.private.create":
						// 私聊
						apiWrap(ctx, msg, apiChannelPrivateCreate)
						solved = true
					case "channel.list":
						apiWrap(ctx, msg, apiChannelList)
						solved = true

					case "channel.members_count": // 自设API
						apiWrap(ctx, msg, apiChannelMemberCount)
						solved = true
					case "channel.member.list.online": // 自设API: 获取频道内在线用户
						apiWrap(ctx, msg, apiChannelMemberListOnline)
						solved = true
					case "channel.member.list": // 自设API: 获取频道成员
						apiWrap(ctx, msg, apiChannelMemberList)
						solved = true
					case "channel.private.list": // 自设API：获取私聊频道
						apiWrap(ctx, msg, apiFriendChannelList)
						solved = true
						// 获取好友: https://satori.js.org/zh-CN/resources/user.html
					case "channel.enter":
						apiWrap(ctx, msg, apiChannelEnter)
						solved = true
					// case "guild.list":
					//	 apiChannelList(c, msg, apiMsg.Echo)
					//	 solved = true

					case "friend.request.list": // 自设api，获取申请加我的用户列表
						apiWrap(ctx, msg, apiFriendRequestList)
						solved = true
					case "friend.request.sender.list": // 自设api，获取申请加我的用户列表
						apiWrap(ctx, msg, apiFriendRequestSenderList)
						solved = true
					case "friend.request.create": // 自设api，添加好友
						apiWrap(ctx, msg, apiFriendRequestCreate)
						solved = true
					case "friend.delete": // 自设api，删除好友
						apiWrap(ctx, msg, apiFriendDelete)
						solved = true
					case "friend.approve":
						apiWrap(ctx, msg, apiFriendRequestApprove)
						solved = true

					case "message.create":
						apiWrap(ctx, msg, apiMessageCreate)
						solved = true
					case "message.update":
						apiWrap(ctx, msg, apiMessageUpdate)
						solved = true
					case "message.delete":
						apiWrap(ctx, msg, apiMessageDelete)
						solved = true
					case "message.reorder":
						apiWrap(ctx, msg, apiMessageReorder)
						solved = true
					case "message.list":
						apiWrap(ctx, msg, apiMessageList)
						solved = true
					case "message.edit.history":
						apiWrap(ctx, msg, apiMessageEditHistory)
						solved = true
					case "message.typing":
						apiWrap(ctx, msg, apiMessageTyping)
						solved = true

					case "unread.count":
						apiWrap(ctx, msg, apiUnreadCount)

					case "guild.member.list":
						apiWrap(ctx, msg, apiGuildMemberList)
						solved = true

					case "bot.info.set_name":
						apiBotInfoSetName(ctx, msg)
						solved = true
					case "bot.command.register":
						apiBotCommandRegister(ctx, msg)
						solved = true
					case "bot.channel_member.set_name":
						apiBotChannelMemberSetName(ctx, msg)
					}
				}
			}

			log.Printf("recv: %s  %d", msg, mt)
			// if err = c.WriteMessage(mt, msg); err != nil {
			//	log.Println("write:", err)
			//	break
			// }
		}

		// 连接断开，补发停止输入信令
		if curConnInfo != nil && curConnInfo.TypingEnabled && curConnInfo.ChannelId != "" && curUser != nil {
			ctx := &ChatContext{
				Conn:            c,
				User:            curUser,
				ConnInfo:        curConnInfo,
				ChannelUsersMap: channelUsersMap,
				UserId2ConnInfo: userId2ConnInfo,
			}

			channel, _ := model.ChannelGet(curConnInfo.ChannelId)
			if channel.ID != "" {
				channelData := channel.ToProtocolType()
				member, _ := model.MemberGetByUserIDAndChannelID(curUser.ID, curConnInfo.ChannelId, curUser.Nickname)

				event := &protocol.Event{
					Type:    protocol.EventTypingPreview,
					Channel: channelData,
					User:    curUser.ToProtocolType(),
					Typing: &protocol.TypingPreview{
						State:   protocol.TypingStateSilent,
						Enabled: false,
						Content: "",
					},
				}
				if member != nil {
					event.Member = member.ToProtocolType()
				}

				ctx.BroadcastEventInChannelExcept(curConnInfo.ChannelId, []string{curUser.ID}, event)
			}

			curConnInfo.TypingEnabled = false
			curConnInfo.TypingState = protocol.TypingStateSilent
			curConnInfo.TypingContent = ""
			curConnInfo.TypingUpdatedAt = 0
		}

		// 连接断开，后续封装成函数
		userId2ConnInfo.Range(func(key string, value *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
			exists := value.Delete(c)
			if exists {
				return false
			}
			return true
		})
		channelUsersMap.Range(func(chId string, value *utils.SyncSet[string]) bool {
			value.Delete(curUser.ID)
			return true
		})
	}))
}
