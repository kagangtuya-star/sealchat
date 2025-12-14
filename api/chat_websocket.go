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
	"sealchat/service"
	"sealchat/service/metrics"
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
	User             *model.UserModel
	Conn             *WsSyncConn
	LastPingTime     int64
	LatencyMs        int64
	ChannelId        string
	WorldId          string
	TypingEnabled    bool
	TypingState      protocol.TypingState
	TypingContent    string
	TypingWhisperTo  string
	TypingUpdatedAt  int64
	TypingIcMode     string
	TypingIdentityID string
	TypingOrderKey   float64
	Focused          bool
}

var commandTips utils.SyncMap[string, map[string]string]

var (
	channelUsersMapGlobal *utils.SyncMap[string, *utils.SyncSet[string]]
	userId2ConnInfoGlobal *utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]
)

// 连接管理配置常量
const (
	// 每用户最大连接数，超出时关闭最旧连接
	maxConnectionsPerUser = 8
	// 读取超时时间（秒），超过此时间无数据则断开
	readTimeoutSeconds = 90
	// 全局健康检查间隔（秒）
	healthCheckIntervalSeconds = 60
	// 连接无心跳最大存活时间（秒）
	connectionMaxIdleSeconds = 180
)

func getChannelUsersMap() *utils.SyncMap[string, *utils.SyncSet[string]] {
	return channelUsersMapGlobal
}

func getUserConnInfoMap() *utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]] {
	return userId2ConnInfoGlobal
}

func websocketWorks(app *fiber.App) {
	channelUsersMap := &utils.SyncMap[string, *utils.SyncSet[string]]{}
	userId2ConnInfo := &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{}
	channelUsersMapGlobal = channelUsersMap
	userId2ConnInfoGlobal = userId2ConnInfo

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

				// 检查并清理超限连接（保留 maxConnectionsPerUser-1 个，为新连接腾出空间）
				for m.Len() >= maxConnectionsPerUser {
					var oldestConn *WsSyncConn
					var oldestTime int64 = time.Now().UnixMilli() + 1
					m.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
						if info.LastPingTime < oldestTime {
							oldestTime = info.LastPingTime
							oldestConn = conn
						}
						return true
					})
					if oldestConn != nil {
						log.Printf("[WS] 用户 %s 连接数超限，关闭最旧连接", user.ID)
						oldestConn.Close()
						m.Delete(oldestConn)
						if collector := metrics.Get(); collector != nil {
							collector.RecordConnectionClosed(user.ID)
						}
					} else {
						break
					}
				}

				curConnInfo = &ConnInfo{
					Conn:         c,
					LastPingTime: time.Now().UnixMilli(),
					User:         user,
					TypingState:  protocol.TypingStateSilent,
					TypingIcMode: "ic",
					Focused:      true,
				}
				m.Store(c, curConnInfo)

				curUser = user
				if collector := metrics.Get(); collector != nil {
					collector.RecordConnectionOpened(user.ID)
					collector.RecordUserHeartbeat(user.ID)
				}
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
		// 导入进度广播
		progressCh := service.SubscribeImportProgress()
		defer service.UnsubscribeImportProgress(progressCh)

		for event := range progressCh {
			// 广播到频道内的所有连接
			userId2ConnInfo.Range(func(userId string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
				connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
					if info.ChannelId == event.ChannelID {
						_ = conn.WriteJSON(protocol.GatewayPayloadStructure{
							Op: protocol.OpEvent,
							Body: map[string]any{
								"type":      "chat-import-progress",
								"channelId": event.ChannelID,
								"progress":  event,
							},
						})
					}
					return true
				})
				return true
			})
		}
	}()

	// 全局连接健康检查，定期清理僵尸连接
	go func() {
		ticker := time.NewTicker(healthCheckIntervalSeconds * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now().UnixMilli()
			cutoff := now - (connectionMaxIdleSeconds * 1000)
			cleanedCount := 0

			userId2ConnInfo.Range(func(userId string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
				var staleConns []*WsSyncConn
				connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
					if info.LastPingTime < cutoff {
						staleConns = append(staleConns, conn)
					}
					return true
				})

				for _, conn := range staleConns {
					log.Printf("[WS] 健康检查：关闭用户 %s 的僵尸连接（无心跳超 %d 秒）", userId, connectionMaxIdleSeconds)
					conn.Close()
					connMap.Delete(conn)
					cleanedCount++
					if collector := metrics.Get(); collector != nil {
						collector.RecordConnectionClosed(userId)
					}
				}
				return true
			})

			if cleanedCount > 0 {
				log.Printf("[WS] 健康检查完成，清理了 %d 个僵尸连接", cleanedCount)
			}
		}
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

		// 设置pong处理器，收到pong时更新连接活跃状态
		rawConn.SetPongHandler(func(appData string) error {
			return nil
		})

		// 启动ping goroutine，定期发送WebSocket ping帧检测连接是否存活
		pingTicker := time.NewTicker(30 * time.Second)
		pingDone := make(chan struct{})
		go func() {
			defer pingTicker.Stop()
			for {
				select {
				case <-pingTicker.C:
					c.Mux.Lock()
					err := rawConn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
					c.Mux.Unlock()
					if err != nil {
						log.Printf("WebSocket ping failed, closing connection: %v", err)
						rawConn.Close()
						return
					}
				case <-pingDone:
					return
				}
			}
		}()

		// 设置初始读取超时
		_ = rawConn.SetReadDeadline(time.Now().Add(readTimeoutSeconds * time.Second))

		for {
			if mt, msg, err = c.ReadMessage(); err != nil {
				log.Println("[WS] read:", err)
				// 解析错误或超时
				break
			}

			// 成功读取后刷新超时
			_ = rawConn.SetReadDeadline(time.Now().Add(readTimeoutSeconds * time.Second))

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
					now := time.Now().UnixMilli()
					var activeChannel string
					if info, ok := userId2ConnInfo.Load(curUser.ID); ok {
						if info2, ok := info.Load(c); ok {
							if bodyMap, ok := gatewayMsg.Body.(map[string]any); ok {
								if focusedRaw, exists := bodyMap["focused"]; exists {
									if focusedVal, ok := focusedRaw.(bool); ok {
										info2.Focused = focusedVal
									}
								}
								latencyUpdated := false
								if sentAtRaw, exists := bodyMap["clientSentAt"]; exists {
									switch v := sentAtRaw.(type) {
									case float64:
										lat := now - int64(v)
										if lat >= 0 {
											info2.LatencyMs = lat
											latencyUpdated = true
										}
									case int64:
										lat := now - v
										if lat >= 0 {
											info2.LatencyMs = lat
											latencyUpdated = true
										}
									case int:
										lat := now - int64(v)
										if lat >= 0 {
											info2.LatencyMs = lat
											latencyUpdated = true
										}
									}
								}
								if !latencyUpdated {
									if latencyRaw, exists := bodyMap["latency"]; exists {
										switch v := latencyRaw.(type) {
										case float64:
											info2.LatencyMs = int64(v)
											latencyUpdated = true
										case int:
											info2.LatencyMs = int64(v)
											latencyUpdated = true
										case int64:
											info2.LatencyMs = v
											latencyUpdated = true
										}
									}
								}
							}
							info2.LastPingTime = now
							activeChannel = info2.ChannelId
						}
					}
					if collector := metrics.Get(); collector != nil && curUser != nil {
						collector.RecordUserHeartbeat(curUser.ID)
					}
					_ = c.WriteJSON(protocol.GatewayPayloadStructure{
						Op: protocol.OpPong,
					})
					if activeChannel != "" {
						ctx := &ChatContext{
							ChannelUsersMap: channelUsersMap,
							UserId2ConnInfo: userId2ConnInfo,
						}
						ctx.BroadcastChannelPresence(activeChannel)
					}
					solved = true
				case protocol.OpLatencyProbe:
					if curUser == nil {
						solved = true
						continue
					}
					latencyBody := protocol.LatencyPayload{}
					if bodyMap, ok := gatewayMsg.Body.(map[string]any); ok {
						if idRaw, exists := bodyMap["id"]; exists {
							if v, ok := idRaw.(string); ok {
								latencyBody.ID = v
							}
						}
						if sentRaw, exists := bodyMap["clientSentAt"]; exists {
							switch v := sentRaw.(type) {
							case float64:
								latencyBody.ClientSentAt = int64(v)
							case int64:
								latencyBody.ClientSentAt = v
							case int:
								latencyBody.ClientSentAt = int64(v)
							}
						}
					}
					latencyBody.ServerSentAt = time.Now().UnixMilli()
					payload := protocol.GatewayPayloadStructure{Op: protocol.OpLatencyResult, Body: latencyBody}
					_ = c.WriteJSON(payload)
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
					case "channel.dice.default.set":
						apiWrap(ctx, msg, apiChannelDefaultDiceUpdate)
						solved = true
					case "channel.feature.update":
						apiWrap(ctx, msg, apiChannelFeatureUpdate)
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
					case "message.remove":
						apiWrap(ctx, msg, apiMessageRemove)
						solved = true
					case "message.reorder":
						apiWrap(ctx, msg, apiMessageReorder)
						solved = true
					case "message.list":
						apiWrap(ctx, msg, apiMessageList)
						solved = true
					case "chat.export.test":
						apiWrap(ctx, msg, apiChatExportTest)
						solved = true
					case "message.archive":
						apiWrap(ctx, msg, apiMessageArchive)
						solved = true
					case "message.unarchive":
						apiWrap(ctx, msg, apiMessageUnarchive)
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

		// 清理ping goroutine
		close(pingDone)

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
				tone := curConnInfo.TypingIcMode
				if tone == "" {
					tone = "ic"
				}
				event.Typing.ICMode = tone
				event.Typing.Tone = tone
				if member != nil {
					event.Member = member.ToProtocolType()
				}

				ctx.BroadcastEventInChannelExcept(curConnInfo.ChannelId, []string{curUser.ID}, event)
			}

			curConnInfo.TypingEnabled = false
			curConnInfo.TypingState = protocol.TypingStateSilent
			curConnInfo.TypingContent = ""
			curConnInfo.TypingUpdatedAt = 0
			curConnInfo.TypingIcMode = "ic"
			curConnInfo.TypingIdentityID = ""
			curConnInfo.TypingOrderKey = 0
		}

		// 连接断开，后续封装成函数
		if collector := metrics.Get(); collector != nil && curUser != nil {
			collector.RecordConnectionClosed(curUser.ID)
		}
		userId2ConnInfo.Range(func(key string, value *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
			exists := value.Delete(c)
			if exists {
				return false
			}
			return true
		})
		ctx := &ChatContext{
			ChannelUsersMap: channelUsersMap,
			UserId2ConnInfo: userId2ConnInfo,
		}
		channelUsersMap.Range(func(chId string, value *utils.SyncSet[string]) bool {
			if curUser != nil && value.Exists(curUser.ID) {
				value.Delete(curUser.ID)
				ctx.BroadcastChannelPresence(chId)
			}
			return true
		})
	}))
}
