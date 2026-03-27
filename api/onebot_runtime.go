package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	fastws "github.com/fasthttp/websocket"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

const oneBotHeartbeatIntervalMs int64 = 15000

type oneBotSessionRole string
type oneBotSessionSource string

const (
	oneBotSessionRoleUniversal oneBotSessionRole = "universal"
	oneBotSessionRoleAPI       oneBotSessionRole = "api"
	oneBotSessionRoleEvent     oneBotSessionRole = "event"

	oneBotSessionSourceForward oneBotSessionSource = "forward"
	oneBotSessionSourceReverse oneBotSessionSource = "reverse"
)

type oneBotSession struct {
	ID        string
	BotUser   *model.UserModel
	Role      oneBotSessionRole
	Source    oneBotSessionSource
	Conn      oneBotJSONConn
	SelfID    int64
	ConnInfo  *ConnInfo
	closeOnce sync.Once
	closeCh   chan struct{}
}

type oneBotJSONConn interface {
	WriteJSON(v interface{}) error
	Close() error
}

type oneBotClientSyncConn struct {
	Conn *fastws.Conn
	Mux  sync.RWMutex
}

func (c *oneBotClientSyncConn) WriteJSON(v interface{}) error {
	c.Mux.Lock()
	defer c.Mux.Unlock()
	return c.Conn.WriteJSON(v)
}

func (c *oneBotClientSyncConn) Close() error {
	if c == nil || c.Conn == nil {
		return nil
	}
	return c.Conn.Close()
}

func newOneBotSession(botUser *model.UserModel, role oneBotSessionRole, source oneBotSessionSource, conn oneBotJSONConn) *oneBotSession {
	nowMs := time.Now().UnixMilli()
	return &oneBotSession{
		ID:      utils.NewID(),
		BotUser: botUser,
		Role:    role,
		Source:  source,
		Conn:    conn,
		ConnInfo: &ConnInfo{
			User:                  botUser,
			LastPingTime:          nowMs,
			LastAliveTime:         nowMs,
			TypingState:           protocol.TypingStateSilent,
			TypingIcMode:          "ic",
			Focused:               true,
			BotLastMessageContext: &utils.SyncMap[string, *protocol.MessageContext]{},
			BotLastWhisperTargets: &utils.SyncMap[string, []string]{},
			BotHiddenDicePending:  &utils.SyncMap[string, *BotHiddenDicePending]{},
		},
		closeCh: make(chan struct{}),
	}
}

func (s *oneBotSession) sendJSON(payload any) error {
	if s == nil || s.Conn == nil {
		return errors.New("onebot session unavailable")
	}
	if s.ConnInfo != nil {
		s.ConnInfo.LastAliveTime = time.Now().UnixMilli()
	}
	return s.Conn.WriteJSON(payload)
}

func (s *oneBotSession) close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		close(s.closeCh)
		if s.Conn != nil {
			_ = s.Conn.Close()
		}
	})
}

type oneBotRuntime struct {
	mu                 sync.RWMutex
	sessions           map[string]*oneBotSession
	reverseMu          sync.Mutex
	reverseControllers map[string]*oneBotReverseController
	reverseStarted     bool
}

var oneBotRuntimeGlobal = &oneBotRuntime{
	sessions:           map[string]*oneBotSession{},
	reverseControllers: map[string]*oneBotReverseController{},
}

func getOneBotRuntime() *oneBotRuntime {
	return oneBotRuntimeGlobal
}

func (rt *oneBotRuntime) registerSession(session *oneBotSession) {
	if rt == nil || session == nil || session.ID == "" {
		return
	}
	rt.mu.Lock()
	rt.sessions[session.ID] = session
	rt.mu.Unlock()

	if session.Role == oneBotSessionRoleEvent || session.Role == oneBotSessionRoleUniversal {
		rt.sendLifecycleConnect(session)
		rt.startHeartbeat(session)
	}
}

func (rt *oneBotRuntime) unregisterSession(sessionID string) {
	if rt == nil || sessionID == "" {
		return
	}
	rt.mu.Lock()
	session := rt.sessions[sessionID]
	delete(rt.sessions, sessionID)
	rt.mu.Unlock()
	if session != nil {
		session.close()
	}
}

func (rt *oneBotRuntime) sessionsByBot(botUserID string) []*oneBotSession {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	result := make([]*oneBotSession, 0)
	for _, session := range rt.sessions {
		if session == nil || session.BotUser == nil || session.BotUser.ID != botUserID {
			continue
		}
		result = append(result, session)
	}
	return result
}

func (rt *oneBotRuntime) publishProtocolEvent(botUserID string, event *protocol.Event, originSessionID string) {
	if rt == nil || botUserID == "" || event == nil {
		return
	}
	for _, session := range rt.sessionsByBot(botUserID) {
		if session == nil {
			continue
		}
		if session.Role != oneBotSessionRoleEvent && session.Role != oneBotSessionRoleUniversal {
			continue
		}
		if originSessionID != "" && session.ID == originSessionID {
			continue
		}
		payload, ok := projectProtocolEventToOneBot(session, event)
		if !ok {
			continue
		}
		if err := session.sendJSON(payload); err != nil {
			log.Printf("[onebot] 推送事件失败 session=%s bot=%s err=%v", session.ID, botUserID, err)
		}
	}
}

func (rt *oneBotRuntime) sendLifecycleConnect(session *oneBotSession) {
	if session == nil {
		return
	}
	if err := session.sendJSON(buildOneBotLifecycleEvent(session)); err != nil {
		log.Printf("[onebot] 发送 connect 元事件失败 session=%s err=%v", session.ID, err)
	}
}

func (rt *oneBotRuntime) startHeartbeat(session *oneBotSession) {
	if session == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(oneBotHeartbeatIntervalMs) * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := session.sendJSON(buildOneBotHeartbeatEvent(session)); err != nil {
					log.Printf("[onebot] 发送 heartbeat 失败 session=%s err=%v", session.ID, err)
				}
			case <-session.closeCh:
				return
			}
		}
	}()
}

func buildOneBotLifecycleEvent(session *oneBotSession) map[string]any {
	return map[string]any{
		"time":            time.Now().Unix(),
		"self_id":         session.SelfID,
		"post_type":       "meta_event",
		"meta_event_type": "lifecycle",
		"sub_type":        "connect",
	}
}

func buildOneBotHeartbeatEvent(session *oneBotSession) map[string]any {
	return map[string]any{
		"time":            time.Now().Unix(),
		"self_id":         session.SelfID,
		"post_type":       "meta_event",
		"meta_event_type": "heartbeat",
		"status":          buildOneBotStatus(session),
		"interval":        oneBotHeartbeatIntervalMs,
	}
}

func buildOneBotStatus(session *oneBotSession) map[string]any {
	online := session != nil && session.BotUser != nil
	return map[string]any{
		"online": online,
		"good":   online,
	}
}

func decodeOneBotActionMessage(raw []byte) (*oneBotActionRequest, error) {
	var req oneBotActionRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

type oneBotReverseController struct {
	botUserID string
	stop      chan struct{}
}

func (c *oneBotReverseController) cancel() {
	if c == nil {
		return
	}
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}

func startOneBotReverseRuntime() {
	getOneBotRuntime().startReverseRuntime()
}

func reloadOneBotReverseRuntimeForBot(botUserID string) {
	getOneBotRuntime().reloadReverseController(botUserID)
}

func (rt *oneBotRuntime) startReverseRuntime() {
	if rt == nil {
		return
	}
	rt.reverseMu.Lock()
	if rt.reverseStarted {
		rt.reverseMu.Unlock()
		return
	}
	rt.reverseStarted = true
	rt.reverseMu.Unlock()

	configs, err := model.BotOneBotConfigListEnabled()
	if err != nil {
		log.Printf("[onebot] 加载反向 WS 配置失败: %v", err)
		return
	}
	for _, cfg := range configs {
		if cfg == nil || cfg.BotUserID == "" {
			continue
		}
		rt.reloadReverseController(cfg.BotUserID)
	}
}

func (rt *oneBotRuntime) reloadReverseController(botUserID string) {
	botUserID = strings.TrimSpace(botUserID)
	if rt == nil || botUserID == "" {
		return
	}

	rt.reverseMu.Lock()
	if existing := rt.reverseControllers[botUserID]; existing != nil {
		existing.cancel()
		delete(rt.reverseControllers, botUserID)
	}
	started := rt.reverseStarted
	rt.reverseMu.Unlock()
	if !started {
		return
	}

	cfg, err := model.BotOneBotConfigGet(botUserID)
	if err != nil || cfg == nil || !cfg.Enabled {
		if err != nil {
			log.Printf("[onebot] 读取反向 WS 配置失败 bot=%s err=%v", botUserID, err)
		}
		return
	}

	botUser := model.UserGet(botUserID)
	if botUser == nil || !botUser.IsBot || strings.TrimSpace(botUser.BotKind) != model.BotKindManual {
		return
	}
	botToken, err := model.BotTokenGet(botUserID)
	if err != nil || botToken == nil || botToken.Token == "" {
		if err != nil {
			log.Printf("[onebot] 读取 bot token 失败 bot=%s err=%v", botUserID, err)
		}
		return
	}

	controller := &oneBotReverseController{
		botUserID: botUserID,
		stop:      make(chan struct{}),
	}
	rt.reverseMu.Lock()
	rt.reverseControllers[botUserID] = controller
	rt.reverseMu.Unlock()

	go rt.runReverseController(controller, botUser, botToken, cfg)
}

func (rt *oneBotRuntime) runReverseController(controller *oneBotReverseController, botUser *model.UserModel, botToken *model.BotTokenModel, cfg *model.BotOneBotConfigModel) {
	if controller == nil || botUser == nil || botToken == nil || cfg == nil {
		return
	}
	selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
	if err != nil {
		log.Printf("[onebot] 生成 self_id 失败 bot=%s err=%v", botUser.ID, err)
		return
	}

	if cfg.UseUniversalClient {
		targetURL := resolveOneBotReverseURL(cfg, oneBotSessionRoleUniversal)
		if targetURL == "" {
			return
		}
		rt.runReverseDialLoop(controller, botUser, botToken, selfID, cfg.ReconnectIntervalMs, oneBotSessionRoleUniversal, targetURL)
		return
	}

	apiURL := resolveOneBotReverseURL(cfg, oneBotSessionRoleAPI)
	eventURL := resolveOneBotReverseURL(cfg, oneBotSessionRoleEvent)
	if apiURL != "" {
		go rt.runReverseDialLoop(controller, botUser, botToken, selfID, cfg.ReconnectIntervalMs, oneBotSessionRoleAPI, apiURL)
	}
	if eventURL != "" {
		go rt.runReverseDialLoop(controller, botUser, botToken, selfID, cfg.ReconnectIntervalMs, oneBotSessionRoleEvent, eventURL)
	}
}

func resolveOneBotReverseURL(cfg *model.BotOneBotConfigModel, role oneBotSessionRole) string {
	if cfg == nil {
		return ""
	}
	switch role {
	case oneBotSessionRoleUniversal:
		if strings.TrimSpace(cfg.URL) != "" {
			return strings.TrimSpace(cfg.URL)
		}
		if strings.TrimSpace(cfg.APIURL) != "" {
			return strings.TrimSpace(cfg.APIURL)
		}
		return strings.TrimSpace(cfg.EventURL)
	case oneBotSessionRoleAPI:
		if strings.TrimSpace(cfg.APIURL) != "" {
			return strings.TrimSpace(cfg.APIURL)
		}
		return strings.TrimSpace(cfg.URL)
	case oneBotSessionRoleEvent:
		if strings.TrimSpace(cfg.EventURL) != "" {
			return strings.TrimSpace(cfg.EventURL)
		}
		return strings.TrimSpace(cfg.URL)
	default:
		return ""
	}
}

func oneBotClientRoleHeader(role oneBotSessionRole) string {
	switch role {
	case oneBotSessionRoleAPI:
		return "API"
	case oneBotSessionRoleEvent:
		return "Event"
	default:
		return "Universal"
	}
}

func (rt *oneBotRuntime) runReverseDialLoop(controller *oneBotReverseController, botUser *model.UserModel, botToken *model.BotTokenModel, selfID int64, reconnectIntervalMs int64, role oneBotSessionRole, targetURL string) {
	interval := time.Duration(reconnectIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = time.Duration(model.DefaultOneBotReconnectIntervalMs) * time.Millisecond
	}
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+strings.TrimSpace(botToken.Token))
	headers.Set("X-Self-ID", strconv.FormatInt(selfID, 10))
	headers.Set("X-Client-Role", oneBotClientRoleHeader(role))

	for {
		select {
		case <-controller.stop:
			return
		default:
		}

		conn, _, err := fastws.DefaultDialer.Dial(targetURL, headers)
		if err != nil {
			log.Printf("[onebot] 反向 WS 连接失败 bot=%s role=%s url=%s err=%v", botUser.ID, role, targetURL, err)
			select {
			case <-time.After(interval):
				continue
			case <-controller.stop:
				return
			}
		}

		session := newOneBotSession(botUser, role, oneBotSessionSourceReverse, &oneBotClientSyncConn{Conn: conn})
		session.SelfID = selfID
		rt.registerSession(session)

		done := make(chan struct{})
		go func() {
			select {
			case <-controller.stop:
				_ = conn.Close()
			case <-done:
			}
		}()

		for {
			_, body, err := conn.ReadMessage()
			if err != nil {
				break
			}
			req, err := decodeOneBotActionMessage(body)
			if err != nil {
				_ = session.sendJSON(oneBotFailureResponse(oneBotBadRequest("invalid request"), nil))
				continue
			}
			resp := dispatchOneBotAction(session, req)
			if err := session.sendJSON(resp); err != nil {
				break
			}
		}

		close(done)
		rt.unregisterSession(session.ID)

		select {
		case <-time.After(interval):
		case <-controller.stop:
			return
		}
	}
}
