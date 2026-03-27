package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	fastws "github.com/fasthttp/websocket"
	wsfiber "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

func newOneBotHTTPTestApp() *fiber.App {
	app := fiber.New()
	oneBotHTTPWorks(app)
	return app
}

func decodeOneBotHTTPResponse(t *testing.T, resp *http.Response) *oneBotActionResponse {
	t.Helper()
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	var result oneBotActionResponse
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	return &result
}

func oneBotHTTPMessageID(t *testing.T, resp *oneBotActionResponse) int64 {
	t.Helper()
	if resp == nil {
		t.Fatal("response is nil")
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected response data type: %T", resp.Data)
	}
	switch value := data["message_id"].(type) {
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			t.Fatalf("parse message_id failed: %v", err)
		}
		return parsed
	default:
		t.Fatalf("unexpected message_id: %#v", data["message_id"])
		return 0
	}
}

func TestOneBotHTTPAPIRequiresToken(t *testing.T) {
	initOneBotAPITestEnv(t)

	app := newOneBotHTTPTestApp()
	req := httptest.NewRequest(http.MethodGet, "/onebot/v11/http/get_status", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestOneBotHTTPAPIRejectsNonManualBot(t *testing.T) {
	initOneBotAPITestEnv(t)

	_, token := createOneBotTestBot(t, "http-system", model.BotKindChannelWebhook)
	app := newOneBotHTTPTestApp()

	req := httptest.NewRequest(http.MethodGet, "/onebot/v11/http/get_status", nil)
	req.Header.Set("Authorization", "Bearer "+token.Token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestOneBotHTTPAPISendGroupMsgWithQuery(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, token := createOneBotTestBot(t, "http-group", model.BotKindManual)
	_, channel := createOneBotTestWorldAndChannel(t, botUser.ID)
	groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, channel.ID)
	if err != nil {
		t.Fatalf("create group mapping failed: %v", err)
	}

	app := newOneBotHTTPTestApp()
	query := url.Values{}
	query.Set("group_id", fmt.Sprintf("%d", groupID))
	query.Set("message", "hello via http")

	req := httptest.NewRequest(http.MethodGet, "/onebot/v11/http/send_group_msg?"+query.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+token.Token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	result := decodeOneBotHTTPResponse(t, resp)
	if result.Status != "ok" || result.RetCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}

	messageID := oneBotHTTPMessageID(t, result)
	internalID, err := service.ResolveInternalID(service.OneBotEntityMessage, messageID)
	if err != nil {
		t.Fatalf("resolve message mapping failed: %v", err)
	}
	var msg model.MessageModel
	if err := model.GetDB().Where("id = ?", internalID).Limit(1).Find(&msg).Error; err != nil {
		t.Fatalf("load saved message failed: %v", err)
	}
	if msg.ChannelID != channel.ID {
		t.Fatalf("message channel = %q, want %q", msg.ChannelID, channel.ID)
	}
}

func TestOneBotHTTPAPIGetLoginInfoWithJSON(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, token := createOneBotTestBot(t, "http-login", model.BotKindManual)
	expectedSelfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
	if err != nil {
		t.Fatalf("create self id failed: %v", err)
	}

	app := newOneBotHTTPTestApp()
	req := httptest.NewRequest(http.MethodPost, "/onebot/v11/http/get_login_info", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	result := decodeOneBotHTTPResponse(t, resp)
	if result.Status != "ok" || result.RetCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected data type: %T", result.Data)
	}
	switch got := data["user_id"].(type) {
	case float64:
		if int64(got) != expectedSelfID {
			t.Fatalf("user_id = %#v, want %d", data["user_id"], expectedSelfID)
		}
	case json.Number:
		parsed, err := got.Int64()
		if err != nil || parsed != expectedSelfID {
			t.Fatalf("user_id = %#v, want %d", data["user_id"], expectedSelfID)
		}
	case string:
		if got != fmt.Sprintf("%d", expectedSelfID) {
			t.Fatalf("user_id = %#v, want %d", data["user_id"], expectedSelfID)
		}
	default:
		t.Fatalf("user_id = %#v, want %d", data["user_id"], expectedSelfID)
	}
}

func TestOneBotHTTPAPIGetStatusWithForm(t *testing.T) {
	initOneBotAPITestEnv(t)

	_, token := createOneBotTestBot(t, "http-form", model.BotKindManual)
	app := newOneBotHTTPTestApp()

	form := url.Values{}
	form.Set("extra", "1")
	req := httptest.NewRequest(http.MethodPost, "/onebot/v11/http/get_status", strings.NewReader(form.Encode()))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	result := decodeOneBotHTTPResponse(t, resp)
	if result.Status != "ok" || result.RetCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestOneBotHTTPAPIUnknownActionReturnsNotFound(t *testing.T) {
	initOneBotAPITestEnv(t)

	_, token := createOneBotTestBot(t, "http-missing", model.BotKindManual)
	app := newOneBotHTTPTestApp()

	req := httptest.NewRequest(http.MethodGet, "/onebot/v11/http/not_real_action", nil)
	req.Header.Set("Authorization", "Bearer "+token.Token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	result := decodeOneBotHTTPResponse(t, resp)
	if result.Status != "failed" || result.RetCode != 1404 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestOneBotHTTPAPIRejectsUnsupportedContentType(t *testing.T) {
	initOneBotAPITestEnv(t)

	_, token := createOneBotTestBot(t, "http-ctype", model.BotKindManual)
	app := newOneBotHTTPTestApp()

	req := httptest.NewRequest(http.MethodPost, "/onebot/v11/http/get_status", strings.NewReader("plain"))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotAcceptable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotAcceptable)
	}
}

func TestOneBotHTTPAPISupportsCustomPathSuffix(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, token := createOneBotTestBot(t, "http-custom-path", model.BotKindManual)
	_, err := model.BotOneBotConfigUpsert(&model.BotOneBotConfigModel{
		BotUserID:          botUser.ID,
		Enabled:            true,
		TransportType:      model.OneBotTransportHTTP,
		HTTPPathSuffix:     "/OlivOSMsgApi/qq/onebot/default",
		HTTPPostPathSuffix: "/OlivOSMsgApi/qq/onebot/http-post/default",
	})
	if err != nil {
		t.Fatalf("save onebot config failed: %v", err)
	}

	app := newOneBotHTTPTestApp()
	req := httptest.NewRequest(http.MethodGet, "/OlivOSMsgApi/qq/onebot/default/get_status", nil)
	req.Header.Set("Authorization", "Bearer "+token.Token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	result := decodeOneBotHTTPResponse(t, resp)
	if result.Status != "ok" || result.RetCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestOneBotHTTPAPISupportsOlivOSRootSendMsgPath(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, token := createOneBotTestBot(t, "http-root-send-msg", model.BotKindManual)
	_, channel := createOneBotTestWorldAndChannel(t, botUser.ID)
	groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, channel.ID)
	if err != nil {
		t.Fatalf("create group mapping failed: %v", err)
	}

	app := newOneBotHTTPTestApp()
	req := httptest.NewRequest(http.MethodPost, "/send_msg", strings.NewReader(
		fmt.Sprintf(`{"message_type":"group","group_id":%d,"message":"hello from olivos root path"}`, groupID),
	))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	result := decodeOneBotHTTPResponse(t, resp)
	if result.Status != "ok" || result.RetCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}

	messageID := oneBotHTTPMessageID(t, result)
	internalID, err := service.ResolveInternalID(service.OneBotEntityMessage, messageID)
	if err != nil {
		t.Fatalf("resolve message mapping failed: %v", err)
	}
	var msg model.MessageModel
	if err := model.GetDB().Where("id = ?", internalID).Limit(1).Find(&msg).Error; err != nil {
		t.Fatalf("load saved message failed: %v", err)
	}
	if msg.ChannelID != channel.ID {
		t.Fatalf("message channel = %q, want %q", msg.ChannelID, channel.ID)
	}
	if msg.Content != "hello from olivos root path" {
		t.Fatalf("message content = %q, want %q", msg.Content, "hello from olivos root path")
	}
}

func TestOneBotHTTPPostPushesMessageEvent(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "http-post-event", model.BotKindManual)
	received := make(chan struct {
		Header http.Header
		Body   map[string]any
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		payload := map[string]any{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode request body failed: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- struct {
			Header http.Header
			Body   map[string]any
		}{
			Header: r.Header.Clone(),
			Body:   payload,
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	_, err := model.BotOneBotConfigUpsert(&model.BotOneBotConfigModel{
		BotUserID:          botUser.ID,
		Enabled:            true,
		TransportType:      model.OneBotTransportHTTP,
		HTTPPostPathSuffix: server.URL + "/OlivOSMsgApi/qq/onebot/default",
	})
	if err != nil {
		t.Fatalf("save onebot config failed: %v", err)
	}

	event := &protocol.Event{
		Type:      protocol.EventMessageCreated,
		Timestamp: time.Now().Unix(),
		Channel: &protocol.Channel{
			ID:   "private-" + utils.NewIDWithLength(8),
			Type: protocol.DirectChannelType,
			Name: "HTTP POST DM",
		},
		User: &protocol.User{
			ID:   "user-" + utils.NewIDWithLength(8),
			Nick: "HTTP POST 用户",
		},
		Message: &protocol.Message{
			ID:      "msg-" + utils.NewIDWithLength(8),
			Content: "hello http post",
		},
	}

	getOneBotRuntime().publishProtocolEvent(botUser.ID, event, "")

	select {
	case req := <-received:
		if got := req.Header.Get("Content-Type"); !strings.HasPrefix(got, fiber.MIMEApplicationJSON) {
			t.Fatalf("content-type = %q, want application/json", got)
		}
		selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
		if err != nil {
			t.Fatalf("create self id failed: %v", err)
		}
		if got := req.Header.Get("X-Self-ID"); got != fmt.Sprintf("%d", selfID) {
			t.Fatalf("x-self-id = %q, want %d", got, selfID)
		}
		if req.Body["post_type"] != "message" {
			t.Fatalf("post_type = %#v, want message", req.Body["post_type"])
		}
		if req.Body["message_type"] != "private" {
			t.Fatalf("message_type = %#v, want private", req.Body["message_type"])
		}
		if req.Body["sub_type"] != "friend" {
			t.Fatalf("sub_type = %#v, want friend", req.Body["sub_type"])
		}
		if req.Body["raw_message"] != "hello http post" {
			t.Fatalf("raw_message = %#v, want hello http post", req.Body["raw_message"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected http post event, got none")
	}
}

func TestOneBotHTTPAPISendGroupMsgBroadcastsRealtimeEvent(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, token := createOneBotTestBot(t, "http-broadcast", model.BotKindManual)
	viewer := createOneBotTestUser(t, "http-viewer", false, "")
	_, channel := createOneBotTestWorldAndChannel(t, botUser.ID)
	groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, channel.ID)
	if err != nil {
		t.Fatalf("create group mapping failed: %v", err)
	}

	receivedConn := make(chan *WsSyncConn, 1)
	releaseConn := make(chan struct{})

	app := fiber.New()
	oneBotHTTPWorks(app)
	app.Get("/test-ws", wsfiber.New(func(c *wsfiber.Conn) {
		conn := &WsSyncConn{Conn: c}
		receivedConn <- conn
		<-releaseConn
	}))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer listener.Close()
	go func() {
		_ = app.Listener(listener)
	}()
	defer app.Shutdown()

	clientConn, _, err := fastws.DefaultDialer.Dial("ws://"+listener.Addr().String()+"/test-ws", nil)
	if err != nil {
		t.Fatalf("dial test websocket failed: %v", err)
	}
	defer clientConn.Close()

	serverConn := <-receivedConn
	viewerConnMap := &utils.SyncMap[*WsSyncConn, *ConnInfo]{}
	viewerConnMap.Store(serverConn, &ConnInfo{
		User:          viewer,
		Conn:          serverConn,
		ChannelId:     channel.ID,
		LastPingTime:  time.Now().UnixMilli(),
		LastAliveTime: time.Now().UnixMilli(),
	})
	userId2ConnInfoGlobal.Store(viewer.ID, viewerConnMap)
	defer func() {
		userId2ConnInfoGlobal.Delete(viewer.ID)
		close(releaseConn)
	}()

	payload := fmt.Sprintf(`{"group_id":%d,"message":"hello realtime http"}`, groupID)
	req := httptest.NewRequest(http.MethodPost, "/onebot/v11/http/send_group_msg", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	result := decodeOneBotHTTPResponse(t, resp)
	if result.Status != "ok" || result.RetCode != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	_, body, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("read realtime event failed: %v", err)
	}

	payloadMap := map[string]any{}
	if err := json.Unmarshal(body, &payloadMap); err != nil {
		t.Fatalf("decode realtime payload failed: %v", err)
	}
	if got := payloadMap["op"]; got != float64(protocol.OpEvent) {
		t.Fatalf("op = %#v, want %d", got, protocol.OpEvent)
	}
	if got := payloadMap["type"]; got != string(protocol.EventMessageCreated) {
		t.Fatalf("type = %#v, want %q", got, protocol.EventMessageCreated)
	}
	channelData, _ := payloadMap["channel"].(map[string]any)
	if got := channelData["id"]; got != channel.ID {
		t.Fatalf("channel.id = %#v, want %q", got, channel.ID)
	}
	messageData, _ := payloadMap["message"].(map[string]any)
	if got := messageData["content"]; got != "hello realtime http" {
		t.Fatalf("message.content = %#v, want %q", got, "hello realtime http")
	}
}

func TestOneBotHTTPPostAppliesQuickReplyOperation(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "http-post-quick-reply", model.BotKindManual)
	sender := createOneBotTestUser(t, "http-post-sender", false, "")
	_, channel := createOneBotTestWorldAndChannel(t, botUser.ID)
	if _, err := model.MemberGetByUserIDAndChannelID(sender.ID, channel.ID, sender.Nickname); err != nil {
		t.Fatalf("ensure sender member failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", fiber.MIMEApplicationJSON)
		_, _ = w.Write([]byte(`{"reply":"quick group reply","at_sender":false}`))
	}))
	defer server.Close()

	_, err := model.BotOneBotConfigUpsert(&model.BotOneBotConfigModel{
		BotUserID:          botUser.ID,
		Enabled:            true,
		TransportType:      model.OneBotTransportHTTP,
		HTTPPostPathSuffix: server.URL,
	})
	if err != nil {
		t.Fatalf("save onebot config failed: %v", err)
	}

	event := &protocol.Event{
		Type:      protocol.EventMessageCreated,
		Timestamp: time.Now().Unix(),
		Channel: &protocol.Channel{
			ID:   channel.ID,
			Type: protocol.TextChannelType,
			Name: channel.Name,
		},
		User: &protocol.User{
			ID:   sender.ID,
			Nick: sender.Nickname,
		},
		Message: &protocol.Message{
			ID:      "msg-" + utils.NewIDWithLength(8),
			Content: "hello quick operation",
		},
	}

	getOneBotRuntime().publishProtocolEvent(botUser.ID, event, "")

	var msg model.MessageModel
	if err := model.GetDB().
		Where("channel_id = ? AND user_id = ? AND content = ?", channel.ID, botUser.ID, "quick group reply").
		Order("created_at DESC").
		Limit(1).
		Find(&msg).Error; err != nil {
		t.Fatalf("load quick reply message failed: %v", err)
	}
	if msg.ID == "" {
		t.Fatal("expected quick reply message to be created")
	}
}

func TestParseOneBotHTTPQuickOperationIgnoresNonObjectBody(t *testing.T) {
	op, err := parseOneBotHTTPQuickOperation([]byte("200"))
	if err != nil {
		t.Fatalf("parse quick operation failed: %v", err)
	}
	if op != nil {
		t.Fatalf("quick operation = %#v, want nil", op)
	}

	op, err = parseOneBotHTTPQuickOperation([]byte("ok"))
	if err != nil {
		t.Fatalf("parse quick operation failed: %v", err)
	}
	if op != nil {
		t.Fatalf("quick operation = %#v, want nil", op)
	}
}
