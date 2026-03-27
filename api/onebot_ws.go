package api

import (
	"log"
	"sync"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func oneBotWSWorks(app *fiber.App) {
	app.Use("/onebot/v11/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get("/onebot/v11/ws", websocket.New(oneBotForwardWSHandler(oneBotSessionRoleUniversal)))
	app.Get("/onebot/v11/ws/api", websocket.New(oneBotForwardWSHandler(oneBotSessionRoleAPI)))
	app.Get("/onebot/v11/ws/event", websocket.New(oneBotForwardWSHandler(oneBotSessionRoleEvent)))
}

func oneBotForwardWSHandler(role oneBotSessionRole) func(*websocket.Conn) {
	return func(rawConn *websocket.Conn) {
		conn := &WsSyncConn{Conn: rawConn, Mux: sync.RWMutex{}}
		token := resolveOneBotAccessToken(rawConn.Headers("Authorization"))
		if token == "" {
			token = resolveOneBotAccessToken(rawConn.Query("access_token"))
		}

		botUser, _, err := resolveOneBotBotFromToken(token)
		if err != nil {
			_ = rawConn.WriteJSON(oneBotFailureResponse(err, nil))
			_ = rawConn.Close()
			return
		}
		selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
		if err != nil {
			_ = rawConn.WriteJSON(oneBotFailureResponse(err, nil))
			_ = rawConn.Close()
			return
		}

		session := newOneBotSession(botUser, role, oneBotSessionSourceForward, conn)
		session.SelfID = selfID
		getOneBotRuntime().registerSession(session)
		defer getOneBotRuntime().unregisterSession(session.ID)

		for {
			_, body, err := rawConn.ReadMessage()
			if err != nil {
				return
			}
			req, err := decodeOneBotActionMessage(body)
			if err != nil {
				if writeErr := session.sendJSON(oneBotFailureResponse(oneBotBadRequest("invalid request"), nil)); writeErr != nil {
					log.Printf("[onebot] 写入错误响应失败: %v", writeErr)
				}
				continue
			}
			resp := dispatchOneBotAction(session, req)
			if err := session.sendJSON(resp); err != nil {
				log.Printf("[onebot] 写入 action 响应失败 session=%s err=%v", session.ID, err)
				return
			}
		}
	}
}
