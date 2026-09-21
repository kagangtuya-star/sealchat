package api

import (
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func oneBotWSWorks(app *fiber.App, webUrl string) {
	wsPath := joinWebPath(webUrl, "onebot/v11/ws")
	app.Use(wsPath, func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	app.Get(wsPath, websocket.New(oneBotForwardWSHandler(oneBotSessionRoleUniversal)))
	app.Get(joinWebPath(webUrl, "onebot/v11/ws/api"), websocket.New(oneBotForwardWSHandler(oneBotSessionRoleAPI)))
	app.Get(joinWebPath(webUrl, "onebot/v11/ws/event"), websocket.New(oneBotForwardWSHandler(oneBotSessionRoleEvent)))
}

func oneBotForwardWSHandler(role oneBotSessionRole) func(*websocket.Conn) {
	return func(rawConn *websocket.Conn) {
		conn := newWsSyncConn(rawConn, defaultWSOutboundQueueSize)
		defer conn.Close()
		token := resolveOneBotAccessToken(rawConn.Headers("Authorization"))
		if token == "" {
			token = resolveOneBotAccessToken(rawConn.Query("access_token"))
		}

		botUser, _, err := resolveOneBotBotFromToken(token)
		if err != nil {
			_ = conn.WriteJSON(oneBotFailureResponse(err, nil))
			_ = conn.Close()
			return
		}
		selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
		if err != nil {
			_ = conn.WriteJSON(oneBotFailureResponse(err, nil))
			_ = conn.Close()
			return
		}

		session := newOneBotSession(botUser, role, oneBotSessionSourceForward, conn)
		session.SelfID = selfID
		getOneBotRuntime().registerSession(session)
		defer getOneBotRuntime().unregisterSession(session.ID)
		stopLiveness := startOneBotWSLiveness(rawConn, session)
		defer stopLiveness()

		for {
			_, body, err := rawConn.ReadMessage()
			if err != nil {
				return
			}
			refreshOneBotReadDeadline(rawConn, session)
			req, err := decodeOneBotActionMessage(body)
			if err != nil {
				if writeErr := session.sendJSON(oneBotFailureResponse(oneBotBadRequest("invalid request"), nil)); writeErr != nil {
					log.Printf("[onebot] 写入错误响应失败: %v", writeErr)
					return
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
