package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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
const oneBotHTTPQuickOperationSessionPrefix = "onebot-http-quickop:"

var oneBotHTTPPostClient = &http.Client{Timeout: 5 * time.Second}

type oneBotHTTPQuickOperation struct {
	Reply      json.RawMessage `json:"reply"`
	AutoEscape bool            `json:"auto_escape"`
	AtSender   *bool           `json:"at_sender,omitempty"`
}

type oneBotSessionRole string
type oneBotSessionSource string

const (
	oneBotSessionRoleUniversal oneBotSessionRole = "universal"
	oneBotSessionRoleAPI       oneBotSessionRole = "api"
	oneBotSessionRoleEvent     oneBotSessionRole = "event"

	oneBotSessionSourceForward oneBotSessionSource = "forward"
	oneBotSessionSourceReverse oneBotSessionSource = "reverse"
	oneBotSessionSourceHTTP    oneBotSessionSource = "http"
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
	if !strings.HasPrefix(originSessionID, oneBotHTTPQuickOperationSessionPrefix) {
		rt.publishHTTPPostEvent(botUserID, event)
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
		if session.ConnInfo != nil && event.Channel != nil {
			cacheBotEventContext(session.ConnInfo, strings.TrimSpace(event.Channel.ID), event)
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

func (rt *oneBotRuntime) publishHTTPPostEvent(botUserID string, event *protocol.Event) {
	if rt == nil || botUserID == "" || event == nil {
		return
	}
	cfg, err := model.BotOneBotConfigGet(botUserID)
	if err != nil {
		log.Printf("[onebot] 读取 HTTP POST 配置失败 bot=%s err=%v", botUserID, err)
		return
	}
	if cfg == nil || !cfg.Enabled || cfg.TransportType != model.OneBotTransportHTTP {
		return
	}
	targetURL := strings.TrimSpace(cfg.HTTPPostPathSuffix)
	if targetURL == "" {
		return
	}
	selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUserID)
	if err != nil {
		log.Printf("[onebot] 生成 HTTP POST self_id 失败 bot=%s err=%v", botUserID, err)
		return
	}
	payload, ok := projectProtocolEventToOneBot(&oneBotSession{SelfID: selfID}, event)
	if !ok {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[onebot] 编码 HTTP POST 事件失败 bot=%s err=%v", botUserID, err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("[onebot] 创建 HTTP POST 请求失败 bot=%s url=%s err=%v", botUserID, targetURL, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Self-ID", strconv.FormatInt(selfID, 10))

	resp, err := oneBotHTTPPostClient.Do(req)
	if err != nil {
		log.Printf("[onebot] HTTP POST 上报失败 bot=%s url=%s err=%v", botUserID, targetURL, err)
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[onebot] 读取 HTTP POST 响应失败 bot=%s url=%s err=%v", botUserID, targetURL, err)
		return
	}
	if resp.StatusCode >= http.StatusBadRequest {
		log.Printf("[onebot] HTTP POST 上报返回异常状态 bot=%s url=%s status=%d", botUserID, targetURL, resp.StatusCode)
		return
	}
	if err := rt.applyHTTPPostQuickOperation(botUserID, event, respBody); err != nil {
		log.Printf("[onebot] HTTP POST 快速操作执行失败 bot=%s url=%s err=%v", botUserID, targetURL, err)
	}
}

func parseOneBotHTTPQuickOperation(body []byte) (*oneBotHTTPQuickOperation, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if !strings.HasPrefix(trimmed, "{") {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var op oneBotHTTPQuickOperation
	if err := decoder.Decode(&op); err != nil {
		return nil, err
	}
	return &op, nil
}

func hasOneBotQuickReply(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func oneBotQuickReplyAtSenderEnabled(op *oneBotHTTPQuickOperation) bool {
	if op == nil {
		return false
	}
	if op.AtSender == nil {
		return true
	}
	return *op.AtSender
}

func prependOneBotQuickReplyAtSegment(raw json.RawMessage, numericUserID int64) (json.RawMessage, bool, error) {
	if numericUserID <= 0 {
		return raw, false, nil
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false, err
	}

	atSegment := map[string]any{
		"type": "at",
		"data": map[string]any{
			"qq": strconv.FormatInt(numericUserID, 10),
		},
	}

	var segments []any
	switch v := value.(type) {
	case string:
		segments = []any{
			atSegment,
			map[string]any{
				"type": "text",
				"data": map[string]any{"text": v},
			},
		}
	case []any:
		segments = make([]any, 0, len(v)+1)
		segments = append(segments, atSegment)
		segments = append(segments, v...)
	case map[string]any:
		segments = []any{atSegment, v}
	default:
		segments = []any{
			atSegment,
			map[string]any{
				"type": "text",
				"data": map[string]any{"text": strings.TrimSpace(string(raw))},
			},
		}
	}

	next, err := json.Marshal(segments)
	if err != nil {
		return nil, false, err
	}
	return next, false, nil
}

func (rt *oneBotRuntime) applyHTTPPostQuickOperation(botUserID string, event *protocol.Event, body []byte) error {
	if rt == nil || strings.TrimSpace(botUserID) == "" || event == nil || event.Channel == nil {
		return nil
	}

	op, err := parseOneBotHTTPQuickOperation(body)
	if err != nil {
		return err
	}
	if op == nil || !hasOneBotQuickReply(op.Reply) {
		return nil
	}

	botUser := model.UserGet(botUserID)
	if botUser == nil || botUser.ID == "" || !botUser.IsBot {
		return errors.New("bot user missing for quick operation")
	}

	session := newOneBotSession(botUser, oneBotSessionRoleUniversal, oneBotSessionSourceHTTP, nil)
	session.ID = oneBotHTTPQuickOperationSessionPrefix + utils.NewID()
	if selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUserID); err == nil {
		session.SelfID = selfID
	}
	if session.ConnInfo != nil {
		cacheBotEventContext(session.ConnInfo, strings.TrimSpace(event.Channel.ID), event)
	}

	replyMessage := op.Reply
	autoEscape := op.AutoEscape

	isPrivate := event.Channel.Type == protocol.DirectChannelType
	if !isPrivate && oneBotQuickReplyAtSenderEnabled(op) {
		senderUserID := ""
		if event.User != nil {
			senderUserID = strings.TrimSpace(event.User.ID)
		}
		if senderUserID == "" && event.Message != nil && event.Message.User != nil {
			senderUserID = strings.TrimSpace(event.Message.User.ID)
		}
		if senderUserID != "" {
			numericUserID, err := service.GetOrCreateOneBotID(service.OneBotEntityUser, senderUserID)
			if err != nil {
				return err
			}
			replyMessage, autoEscape, err = prependOneBotQuickReplyAtSegment(replyMessage, numericUserID)
			if err != nil {
				return err
			}
		}
	}

	if isPrivate {
		targetUserID := ""
		if event.User != nil {
			targetUserID = strings.TrimSpace(event.User.ID)
		}
		if targetUserID == "" && event.Message != nil && event.Message.User != nil {
			targetUserID = strings.TrimSpace(event.Message.User.ID)
		}
		if targetUserID == "" {
			return errors.New("quick reply target user missing")
		}
		channel, err := ensureOneBotPrivateChannel(botUserID, targetUserID)
		if err != nil {
			return err
		}
		_, err = oneBotActionSendIntoChannel(session, channel, replyMessage, autoEscape)
		return err
	}

	channel, err := ensureOneBotGroupChannel(botUserID, event.Channel.ID)
	if err != nil {
		return err
	}
	_, err = oneBotActionSendIntoChannel(session, channel, replyMessage, autoEscape)
	return err
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

func oneBotDebugSnippet(raw []byte) string {
	const maxLen = 1024
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return trimmed[:maxLen] + "...(truncated)"
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
	if err != nil || cfg == nil || !cfg.Enabled || cfg.TransportType != model.OneBotTransportReverseWS {
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
			if resp != nil && (resp.Status != "ok" || resp.RetCode != 0) {
			}
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
