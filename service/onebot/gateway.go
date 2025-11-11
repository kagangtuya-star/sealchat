package onebot

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol/onebotv11"
	"sealchat/utils"
)

type ConnKind string

const (
	ConnKindAPI       ConnKind = "api"
	ConnKindEvent     ConnKind = "event"
	ConnKindUniversal ConnKind = "universal"
)

const (
	localProfileKey = "onebot:profile"
	localKindKey    = "onebot:kind"
)

type wsConn struct {
	*websocket.Conn
	profileID string
	kind      ConnKind
	mu        sync.Mutex
}

func (c *wsConn) safeWriteJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteJSON(v)
}

type Gateway struct {
	cfg   *utils.OneBotConfig
	mu    sync.RWMutex
	conns map[string]map[ConnKind]map[*wsConn]struct{}
}

var (
	gatewayOnce    sync.Once
	defaultGateway *Gateway
)

func gatewayInstance() *Gateway {
	gatewayOnce.Do(func() {
		defaultGateway = &Gateway{
			conns: make(map[string]map[ConnKind]map[*wsConn]struct{}),
		}
	})
	return defaultGateway
}

func AttachRoutes(app *fiber.App, cfg *utils.OneBotConfig) {
	if cfg == nil || !cfg.Enabled {
		return
	}
	gw := gatewayInstance()
	copied := *cfg
	gw.cfg = &copied

	register := func(path string, kind ConnKind) {
		if strings.TrimSpace(path) == "" {
			return
		}
		app.Use(path, func(c *fiber.Ctx) error {
			if !websocket.IsWebSocketUpgrade(c) {
				return fiber.ErrUpgradeRequired
			}
			profile, err := gw.resolveProfileFromCtx(c)
			if err != nil {
				return fiber.NewError(http.StatusUnauthorized, err.Error())
			}
			c.Locals(localProfileKey, profile)
			c.Locals(localKindKey, kind)
			return c.Next()
		})
		app.Get(path, websocket.New(func(conn *websocket.Conn) {
			profileAny := conn.Locals(localProfileKey)
			kindAny := conn.Locals(localKindKey)
			if profileAny == nil || kindAny == nil {
				_ = conn.Close()
				return
			}
			profile, ok := profileAny.(*model.BotProfileModel)
			if !ok {
				_ = conn.Close()
				return
			}
			kind, _ := kindAny.(ConnKind)
			gw.handleConn(conn, profile, kind)
		}))
	}

	register(cfg.WS.APIPath, ConnKindAPI)
	register(cfg.WS.EventPath, ConnKindEvent)
	register(cfg.WS.UniversalPath, ConnKindUniversal)
}

func (g *Gateway) handleConn(conn *websocket.Conn, profile *model.BotProfileModel, kind ConnKind) {
	wrapped := &wsConn{
		Conn:      conn,
		profileID: profile.ID,
		kind:      kind,
	}
	g.addConn(wrapped)
	defer g.removeConn(wrapped)

	manager := ManagerInstance()
	manager.UpdateStatus(profile.ID, RuntimeStatusConnected, "")

	switch kind {
	case ConnKindAPI:
		g.loopAPIConn(wrapped, profile)
	default:
		g.loopPassiveConn(wrapped, profile)
	}
}

func (g *Gateway) loopAPIConn(conn *wsConn, profile *model.BotProfileModel) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var frame onebotv11.ActionFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			_ = conn.safeWriteJSON(onebotv11.NewErrorResponse(nil, 1400, "invalid action payload"))
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resp, derr := getDispatcher().HandleAction(ctx, profile, &frame)
		cancel()
		if derr != nil {
			resp = onebotv11.NewErrorResponse(frame.Echo, 1500, derr.Error())
		}
		if resp != nil && resp.Echo == nil {
			resp.Echo = frame.Echo
		}
		if resp != nil {
			_ = conn.safeWriteJSON(resp)
		}
	}
}

func (g *Gateway) loopPassiveConn(conn *wsConn, profile *model.BotProfileModel) {
	selfID := profile.NumericSelfID()
	if strings.TrimSpace(selfID) == "" {
		selfID = profile.UserID
	}
	_ = conn.safeWriteJSON(onebotv11.NewMetaConnectEvent(selfID))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			continue
		}
		log.Printf("onebot inbound frame profile=%s kind=%s size=%d", profile.ID, conn.kind, len(data))
		if err := g.dispatchInboundFrame(conn, profile, data); err != nil {
			log.Printf("onebot inbound frame error: %v", err)
		}
	}
}

func (g *Gateway) addConn(conn *wsConn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conns == nil {
		g.conns = make(map[string]map[ConnKind]map[*wsConn]struct{})
	}
	kindMap := g.conns[conn.profileID]
	if kindMap == nil {
		kindMap = make(map[ConnKind]map[*wsConn]struct{})
		g.conns[conn.profileID] = kindMap
	}
	set := kindMap[conn.kind]
	if set == nil {
		set = make(map[*wsConn]struct{})
		kindMap[conn.kind] = set
	}
	set[conn] = struct{}{}
}

func (g *Gateway) removeConn(conn *wsConn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conns == nil {
		return
	}
	kindMap := g.conns[conn.profileID]
	if kindMap == nil {
		return
	}
	set := kindMap[conn.kind]
	if set == nil {
		return
	}
	delete(set, conn)
	if len(set) == 0 {
		delete(kindMap, conn.kind)
	}
	if len(kindMap) == 0 {
		delete(g.conns, conn.profileID)
		ManagerInstance().UpdateStatus(conn.profileID, RuntimeStatusDisconnected, "")
	}
}

func (g *Gateway) resolveProfileFromCtx(c *fiber.Ctx) (*model.BotProfileModel, error) {
	botID := firstNotEmpty(
		c.Query("bot_id"),
		c.Query("self_id"),
		c.Get("X-Bot-ID"),
		c.Get("X-Self-ID"),
	)
	if botID == "" {
		return nil, errors.New("missing bot identifier")
	}
	var profile *model.BotProfileModel
	var err error
	if strings.HasPrefix(botID, "bot_") {
		profile, err = model.BotProfileGet(botID)
	} else {
		profile, err = model.BotProfileGet(botID)
		if err != nil {
			profile, err = model.BotProfileGetByUserID(botID)
		}
	}
	if err != nil {
		return nil, err
	}
	if !profile.Enabled {
		return nil, errors.New("bot disabled")
	}
	if err := g.verifyToken(c, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (g *Gateway) verifyToken(c *fiber.Ctx, profile *model.BotProfileModel) error {
	headerToken := c.Get("Authorization")
	if headerToken == "" {
		headerToken = c.Query("access_token")
	}
	headerToken = strings.TrimSpace(headerToken)
	if strings.HasPrefix(strings.ToLower(headerToken), "bearer ") {
		headerToken = strings.TrimSpace(headerToken[7:])
	}

	expected := strings.TrimSpace(profile.AccessToken)
	if expected == "" && g.cfg != nil {
		expected = strings.TrimSpace(g.cfg.Auth.AccessToken)
	}
	if expected == "" {
		return nil
	}
	if headerToken == "" {
		return errors.New("missing access token")
	}
	if subtleConstantTimeCompare(headerToken, expected) {
		return nil
	}
	return errors.New("access token mismatch")
}

func subtleConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	diff := byte(0)
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func firstNotEmpty(values ...string) string {
	for _, val := range values {
		if strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func BroadcastEvent(botID string, event *onebotv11.Event) {
	gw := gatewayInstance()
	gw.broadcast(botID, ConnKindEvent, event)
	gw.broadcast(botID, ConnKindUniversal, event)
}

func (g *Gateway) broadcast(botID string, kind ConnKind, payload interface{}) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	kindMap := g.conns[botID]
	if kindMap == nil {
		return
	}
	set := kindMap[kind]
	if len(set) == 0 {
		return
	}
	for conn := range set {
		if err := conn.safeWriteJSON(payload); err != nil {
			log.Printf("onebot broadcast error: %v", err)
		}
	}
}

func (g *Gateway) dispatchInboundFrame(conn *wsConn, profile *model.BotProfileModel, payload []byte) error {
	if conn.kind == ConnKindAPI {
		return nil
	}
	if conn.kind != ConnKindAPI && g.tryHandleActionFrame(conn, profile, payload, true) {
		return nil
	}
	var event onebotv11.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}
	event.NormalizeAliases()
	if event.PostType == "" && len(payload) > 0 {
		log.Printf("onebot event missing post_type profile=%s payload=%s", profile.ID, string(payload))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return getDispatcher().HandleEvent(ctx, profile, &event)
}

func (g *Gateway) tryHandleActionFrame(conn *wsConn, profile *model.BotProfileModel, payload []byte, respond bool) bool {
	var probe struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(payload, &probe); err != nil || strings.TrimSpace(probe.Action) == "" {
		return false
	}
	var frame onebotv11.ActionFrame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	resp, err := getDispatcher().HandleAction(ctx, profile, &frame)
	cancel()
	if err != nil {
		resp = onebotv11.NewErrorResponse(frame.Echo, 1500, err.Error())
	}
	if respond && resp != nil {
		_ = conn.safeWriteJSON(resp)
	}
	return true
}
