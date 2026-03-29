package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	fastws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

var oneBotAPITestDBOnce sync.Once

type oneBotTestJSONConn struct {
	payloads []any
}

func (c *oneBotTestJSONConn) WriteJSON(v interface{}) error {
	c.payloads = append(c.payloads, v)
	return nil
}

func (c *oneBotTestJSONConn) Close() error {
	return nil
}

func initOneBotAPITestEnv(t *testing.T) {
	t.Helper()
	initOneBotAPITestDB(t)
	pm.Init()
	if appConfig == nil {
		appConfig = &utils.AppConfig{
			ImageSizeLimit: 2048,
			Storage: utils.StorageConfig{
				Local: utils.LocalStorageConfig{
					TempDir:   "/tmp",
					UploadDir: "/tmp",
				},
			},
		}
	}
	if channelUsersMapGlobal == nil {
		channelUsersMapGlobal = &utils.SyncMap[string, *utils.SyncSet[string]]{}
	}
	if userId2ConnInfoGlobal == nil {
		userId2ConnInfoGlobal = &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{}
	}
}

func initOneBotAPITestDB(t *testing.T) {
	t.Helper()
	oneBotAPITestDBOnce.Do(func() {
		model.DBInit(&utils.AppConfig{
			DSN: ":memory:",
			SQLite: utils.SQLiteConfig{
				EnableWAL:       false,
				TxLockImmediate: false,
				ReadConnections: 1,
				OptimizeOnInit:  false,
			},
		})
	})
}

func createOneBotTestUser(t *testing.T, prefix string, isBot bool, botKind string) *model.UserModel {
	t.Helper()
	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: prefix + "-" + utils.NewID()},
		Username:          prefix + "_" + utils.NewIDWithLength(8),
		Nickname:          prefix + "-nick",
		Password:          "pw",
		Salt:              "salt",
		IsBot:             isBot,
		BotKind:           botKind,
	}
	if err := model.GetDB().Create(user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	return user
}

func createOneBotTestBot(t *testing.T, prefix string, botKind string) (*model.UserModel, *model.BotTokenModel) {
	t.Helper()
	user := createOneBotTestUser(t, prefix, true, botKind)
	token := &model.BotTokenModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: user.ID},
		Name:              prefix + "-bot",
		Token:             utils.NewIDWithLength(32),
		ExpiresAt:         time.Now().Add(time.Hour).UnixMilli(),
	}
	if err := model.GetDB().Create(token).Error; err != nil {
		t.Fatalf("create bot token failed: %v", err)
	}
	return user, token
}

func createOneBotTestWorldAndChannel(t *testing.T, botUserID string) (*model.WorldModel, *model.ChannelModel) {
	t.Helper()

	owner := createOneBotTestUser(t, "owner", false, "")
	world := &model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "world-" + utils.NewIDWithLength(8)},
		Name:              "OneBot Test World",
		Visibility:        model.WorldVisibilityPublic,
		Status:            "active",
		OwnerID:           owner.ID,
	}
	if err := model.GetDB().Create(world).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}
	if err := model.GetDB().Create(&model.WorldMemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "wm-" + utils.NewIDWithLength(8)},
		WorldID:           world.ID,
		UserID:            owner.ID,
		Role:              model.WorldRoleOwner,
		JoinedAt:          time.Now(),
	}).Error; err != nil {
		t.Fatalf("create world owner failed: %v", err)
	}

	channelID := "grp-" + utils.NewIDWithLength(8)
	channel := service.ChannelNew(channelID, "public", "OneBot Group", world.ID, owner.ID, "")
	if channel == nil || channel.ID == "" {
		t.Fatal("create group channel failed")
	}
	if err := model.UserRoleMappingCreate(&model.UserRoleMappingModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "urm-" + utils.NewIDWithLength(8)},
		UserID:            botUserID,
		RoleID:            fmt.Sprintf("ch-%s-bot", channel.ID),
		RoleType:          "channel",
	}); err != nil {
		t.Fatalf("bind bot role failed: %v", err)
	}
	return world, channel
}

func createOneBotTestSession(t *testing.T, botUser *model.UserModel) *oneBotSession {
	t.Helper()
	session := newOneBotSession(botUser, oneBotSessionRoleUniversal, oneBotSessionSourceForward, nil)
	selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
	if err != nil {
		t.Fatalf("create self_id failed: %v", err)
	}
	session.SelfID = selfID
	return session
}

func mustOneBotMessageID(t *testing.T, resp *oneBotActionResponse) int64 {
	t.Helper()
	if resp == nil {
		t.Fatal("response is nil")
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected response data type: %T", resp.Data)
	}
	messageID, ok := data["message_id"].(int64)
	if !ok || messageID <= 0 {
		t.Fatalf("unexpected message_id: %#v", data["message_id"])
	}
	return messageID
}

func startOneBotWSTestServer(t *testing.T) (string, func()) {
	t.Helper()
	app := fiber.New()
	oneBotWSWorks(app)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	go func() {
		_ = app.Listener(listener)
	}()
	time.Sleep(20 * time.Millisecond)

	return "ws://" + listener.Addr().String(), func() {
		_ = app.Shutdown()
		_ = listener.Close()
	}
}

func TestOneBotActionSendMsgFallsBackToGroupID(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "manual", model.BotKindManual)
	_, channel := createOneBotTestWorldAndChannel(t, botUser.ID)
	session := createOneBotTestSession(t, botUser)

	groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, channel.ID)
	if err != nil {
		t.Fatalf("create group mapping failed: %v", err)
	}

	resp := dispatchOneBotAction(session, &oneBotActionRequest{
		Action: "send_msg",
		Params: json.RawMessage(fmt.Sprintf(`{"group_id":%d,"message":"hello group"}`, groupID)),
	})
	if resp.Status != "ok" || resp.RetCode != 0 {
		t.Fatalf("send_msg response = %#v, want ok", resp)
	}

	messageID := mustOneBotMessageID(t, resp)
	internalID, err := service.ResolveInternalID(service.OneBotEntityMessage, messageID)
	if err != nil {
		t.Fatalf("resolve message id failed: %v", err)
	}
	var msg model.MessageModel
	if err := model.GetDB().Where("id = ?", internalID).Limit(1).Find(&msg).Error; err != nil {
		t.Fatalf("load message failed: %v", err)
	}
	if msg.ID == "" || msg.ChannelID != channel.ID {
		t.Fatalf("unexpected saved message: %#v", msg)
	}
}

func TestOneBotActionSendPrivateMessage(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "privatebot", model.BotKindManual)
	targetUser := createOneBotTestUser(t, "private-target", false, "")
	session := createOneBotTestSession(t, botUser)

	userID, err := service.GetOrCreateOneBotID(service.OneBotEntityUser, targetUser.ID)
	if err != nil {
		t.Fatalf("create user mapping failed: %v", err)
	}

	resp := dispatchOneBotAction(session, &oneBotActionRequest{
		Action: "send_private_msg",
		Params: json.RawMessage(fmt.Sprintf(`{"user_id":%d,"message":"hello private"}`, userID)),
	})
	if resp.Status != "ok" || resp.RetCode != 0 {
		t.Fatalf("send_private_msg response = %#v, want ok", resp)
	}

	messageID := mustOneBotMessageID(t, resp)
	internalID, err := service.ResolveInternalID(service.OneBotEntityMessage, messageID)
	if err != nil {
		t.Fatalf("resolve private message id failed: %v", err)
	}
	var msg model.MessageModel
	if err := model.GetDB().Where("id = ?", internalID).Limit(1).Find(&msg).Error; err != nil {
		t.Fatalf("load private message failed: %v", err)
	}
	if msg.ID == "" || msg.UserID != botUser.ID {
		t.Fatalf("unexpected private message record: %#v", msg)
	}
}

func TestOneBotActionSendPrivateMessageAcceptsStringUserID(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "privatebot-str", model.BotKindManual)
	targetUser := createOneBotTestUser(t, "private-target-str", false, "")
	session := createOneBotTestSession(t, botUser)

	userID, err := service.GetOrCreateOneBotID(service.OneBotEntityUser, targetUser.ID)
	if err != nil {
		t.Fatalf("create user mapping failed: %v", err)
	}

	resp := dispatchOneBotAction(session, &oneBotActionRequest{
		Action: "send_private_msg",
		Params: json.RawMessage(fmt.Sprintf(`{"user_id":"%d","message":"hello private"}`, userID)),
	})
	if resp.Status != "ok" || resp.RetCode != 0 {
		t.Fatalf("send_private_msg response = %#v, want ok", resp)
	}
}

func TestOneBotActionGetStrangerInfoAcceptsStringUserID(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "strangerbot-str", model.BotKindManual)
	targetUser := createOneBotTestUser(t, "stranger-target-str", false, "")
	session := createOneBotTestSession(t, botUser)

	userID, err := service.GetOrCreateOneBotID(service.OneBotEntityUser, targetUser.ID)
	if err != nil {
		t.Fatalf("create user mapping failed: %v", err)
	}

	resp := dispatchOneBotAction(session, &oneBotActionRequest{
		Action: "get_stranger_info",
		Params: json.RawMessage(fmt.Sprintf(`{"user_id":"%d"}`, userID)),
	})
	if resp.Status != "ok" || resp.RetCode != 0 {
		t.Fatalf("get_stranger_info response = %#v, want ok", resp)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected response data type: %T", resp.Data)
	}
	if got := data["user_id"]; got != userID {
		t.Fatalf("user_id = %#v, want %d", got, userID)
	}
}

func TestOneBotPublishProtocolEventCachesMessageContext(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "cachebot", model.BotKindManual)
	sender := createOneBotTestUser(t, "cache-sender", false, "")
	_, channel := createOneBotTestWorldAndChannel(t, botUser.ID)
	session := createOneBotTestSession(t, botUser)
	session.Conn = &oneBotTestJSONConn{}

	rt := getOneBotRuntime()
	rt.registerSession(session)
	defer rt.unregisterSession(session.ID)

	event := &protocol.Event{
		Type:      protocol.EventMessageCreated,
		Timestamp: time.Now().Unix(),
		Channel: &protocol.Channel{
			ID:   channel.ID,
			Name: channel.Name,
			Type: protocol.TextChannelType,
		},
		User: &protocol.User{
			ID:   sender.ID,
			Nick: sender.Nickname,
		},
		Message: &protocol.Message{
			ID:      "msg-" + utils.NewIDWithLength(8),
			Content: "场外提问",
		},
		MessageContext: &protocol.MessageContext{
			ICMode:       "ooc",
			SenderUserID: sender.ID,
		},
	}

	rt.publishProtocolEvent(botUser.ID, event, "")

	msgContext, ok := session.ConnInfo.BotLastMessageContext.Load(channel.ID)
	if !ok || msgContext == nil {
		t.Fatal("expected onebot session to cache message context")
	}
	if msgContext.ICMode != "ooc" {
		t.Fatalf("cached ic_mode = %q, want %q", msgContext.ICMode, "ooc")
	}
}

func TestOneBotActionSendGroupMessageInheritsCachedICMode(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "group-icmode", model.BotKindManual)
	_, channel := createOneBotTestWorldAndChannel(t, botUser.ID)
	session := createOneBotTestSession(t, botUser)

	groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, channel.ID)
	if err != nil {
		t.Fatalf("create group mapping failed: %v", err)
	}

	session.ConnInfo.BotLastMessageContext.Store(channel.ID, &protocol.MessageContext{
		ICMode:       "ooc",
		SenderUserID: "sender-" + utils.NewIDWithLength(8),
	})

	resp := dispatchOneBotAction(session, &oneBotActionRequest{
		Action: "send_group_msg",
		Params: json.RawMessage(fmt.Sprintf(`{"group_id":%d,"message":"场外回复"}`, groupID)),
	})
	if resp.Status != "ok" || resp.RetCode != 0 {
		t.Fatalf("send_group_msg response = %#v, want ok", resp)
	}

	messageID := mustOneBotMessageID(t, resp)
	internalID, err := service.ResolveInternalID(service.OneBotEntityMessage, messageID)
	if err != nil {
		t.Fatalf("resolve message id failed: %v", err)
	}
	var msg model.MessageModel
	if err := model.GetDB().Where("id = ?", internalID).Limit(1).Find(&msg).Error; err != nil {
		t.Fatalf("load group message failed: %v", err)
	}
	if msg.ICMode != "ooc" {
		t.Fatalf("message ic_mode = %q, want %q", msg.ICMode, "ooc")
	}
}

func TestOneBotActionSendGroupMessageAcceptsStringGroupID(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "group-str", model.BotKindManual)
	_, channel := createOneBotTestWorldAndChannel(t, botUser.ID)
	session := createOneBotTestSession(t, botUser)

	groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, channel.ID)
	if err != nil {
		t.Fatalf("create group mapping failed: %v", err)
	}

	resp := dispatchOneBotAction(session, &oneBotActionRequest{
		Action: "send_group_msg",
		Params: json.RawMessage(fmt.Sprintf(`{"group_id":"%d","message":"场内回复"}`, groupID)),
	})
	if resp.Status != "ok" || resp.RetCode != 0 {
		t.Fatalf("send_group_msg response = %#v, want ok", resp)
	}
}

func TestProjectProtocolEventToOneBot(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "eventbot", model.BotKindManual)
	session := createOneBotTestSession(t, botUser)

	groupEvent := &protocol.Event{
		Type:      protocol.EventMessageCreated,
		Timestamp: time.Now().Unix(),
		Channel: &protocol.Channel{
			ID:   "group-" + utils.NewIDWithLength(8),
			Name: "Group",
			Type: protocol.TextChannelType,
		},
		User: &protocol.User{
			ID:   "user-" + utils.NewIDWithLength(8),
			Nick: "群成员",
		},
		Message: &protocol.Message{
			ID:      "msg-" + utils.NewIDWithLength(8),
			Content: "群消息",
			User: &protocol.User{
				ID:   "user-fallback",
				Nick: "fallback",
			},
			Member: &protocol.GuildMember{
				Nick:  "群名片",
				Roles: []string{"admin"},
			},
		},
	}
	groupPayload, ok := projectProtocolEventToOneBot(session, groupEvent)
	if !ok {
		t.Fatal("expected group event to be projected")
	}
	if groupPayload["message_type"] != "group" || groupPayload["sub_type"] != "normal" {
		t.Fatalf("unexpected group payload type: %#v", groupPayload)
	}
	if _, ok := groupPayload["group_id"]; !ok {
		t.Fatalf("expected group_id in payload: %#v", groupPayload)
	}
	groupSender, _ := groupPayload["sender"].(map[string]any)
	if groupSender["nickname"] != "群成员" || groupSender["role"] != "admin" {
		t.Fatalf("unexpected group sender: %#v", groupSender)
	}

	privateEvent := &protocol.Event{
		Type:      protocol.EventMessageCreated,
		Timestamp: time.Now().Unix(),
		Channel: &protocol.Channel{
			ID:   "private-" + utils.NewIDWithLength(8),
			Name: "Direct",
			Type: protocol.DirectChannelType,
		},
		User: &protocol.User{
			ID:   "dm-user-" + utils.NewIDWithLength(8),
			Nick: "私聊用户",
		},
		Message: &protocol.Message{
			ID:      "msg-" + utils.NewIDWithLength(8),
			Content: "私聊消息",
		},
	}
	privatePayload, ok := projectProtocolEventToOneBot(session, privateEvent)
	if !ok {
		t.Fatal("expected private event to be projected")
	}
	if privatePayload["message_type"] != "private" || privatePayload["sub_type"] != "friend" {
		t.Fatalf("unexpected private payload type: %#v", privatePayload)
	}
	if _, ok := privatePayload["group_id"]; ok {
		t.Fatalf("private payload should not contain group_id: %#v", privatePayload)
	}
}

func TestOneBotForwardWSRejectsNonManualBot(t *testing.T) {
	initOneBotAPITestEnv(t)

	_, token := createOneBotTestBot(t, "systembot", model.BotKindChannelWebhook)
	baseURL, shutdown := startOneBotWSTestServer(t)
	defer shutdown()

	headers := map[string][]string{
		"Authorization": {"Bearer " + token.Token},
	}
	conn, _, err := fastws.DefaultDialer.Dial(baseURL+"/onebot/v11/ws", headers)
	if err != nil {
		t.Fatalf("dial websocket failed: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))

	_, body, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read auth failure message failed: %v", err)
	}
	var resp oneBotActionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal auth response failed: %v", err)
	}
	if resp.Status != "failed" || resp.RetCode != 1403 {
		t.Fatalf("unexpected auth response: %#v", resp)
	}
}

func TestOneBotReverseDialLoopSendsRequiredHeaders(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, token := createOneBotTestBot(t, "reversebot", model.BotKindManual)
	selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
	if err != nil {
		t.Fatalf("create reverse self_id failed: %v", err)
	}

	headerCh := make(chan http.Header, 1)
	upgrader := fastws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerCh <- r.Header.Clone()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	controller := &oneBotReverseController{
		botUserID: botUser.ID,
		stop:      make(chan struct{}),
	}
	rt := &oneBotRuntime{
		sessions:           map[string]*oneBotSession{},
		reverseControllers: map[string]*oneBotReverseController{},
	}

	go rt.runReverseDialLoop(
		controller,
		botUser,
		token,
		selfID,
		50,
		oneBotSessionRoleUniversal,
		strings.Replace(server.URL, "http://", "ws://", 1),
	)

	select {
	case headers := <-headerCh:
		controller.cancel()
		if got := headers.Get("Authorization"); got != "Bearer "+token.Token {
			t.Fatalf("authorization header = %q, want %q", got, "Bearer "+token.Token)
		}
		if got := headers.Get("X-Client-Role"); got != "Universal" {
			t.Fatalf("X-Client-Role = %q, want %q", got, "Universal")
		}
		if got := headers.Get("X-Self-ID"); got != fmt.Sprintf("%d", selfID) {
			t.Fatalf("X-Self-ID = %q, want %d", got, selfID)
		}
	case <-time.After(2 * time.Second):
		controller.cancel()
		t.Fatal("reverse dial did not reach test server in time")
	}
}

func TestOneBotReverseDialLoopReconnectsAfterServerDisconnect(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, token := createOneBotTestBot(t, "reverse-reconnect", model.BotKindManual)
	selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
	if err != nil {
		t.Fatalf("create reverse self_id failed: %v", err)
	}

	var accepted atomic.Int32
	attemptCh := make(chan int32, 4)
	upgrader := fastws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		attempt := accepted.Add(1)
		attemptCh <- attempt

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		if attempt == 1 {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	controller := &oneBotReverseController{
		botUserID: botUser.ID,
		stop:      make(chan struct{}),
	}
	rt := &oneBotRuntime{
		sessions:           map[string]*oneBotSession{},
		reverseControllers: map[string]*oneBotReverseController{},
	}

	go rt.runReverseDialLoop(
		controller,
		botUser,
		token,
		selfID,
		50,
		oneBotSessionRoleUniversal,
		strings.Replace(server.URL, "http://", "ws://", 1),
	)

	wantAttempts := []int32{1, 2}
	for _, want := range wantAttempts {
		select {
		case got := <-attemptCh:
			if got != want {
				controller.cancel()
				t.Fatalf("accept attempt = %d, want %d", got, want)
			}
		case <-time.After(3 * time.Second):
			controller.cancel()
			t.Fatalf("did not observe reconnect attempt %d in time", want)
		}
	}

	controller.cancel()
}

func TestOneBotRuntimeStartReverseRuntimeReconnectsOnFreshRuntime(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, token := createOneBotTestBot(t, "reverse-startup", model.BotKindManual)
	selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
	if err != nil {
		t.Fatalf("create reverse self_id failed: %v", err)
	}

	connectEvents := make(chan map[string]any, 4)
	upgrader := fastws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, body, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode lifecycle payload failed: %v", err)
			return
		}
		connectEvents <- payload

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	cfg := &model.BotOneBotConfigModel{
		BotUserID:           botUser.ID,
		Enabled:             true,
		TransportType:       model.OneBotTransportReverseWS,
		URL:                 strings.Replace(server.URL, "http://", "ws://", 1),
		UseUniversalClient:  true,
		ReconnectIntervalMs: 50,
	}
	if _, err := model.BotOneBotConfigUpsert(cfg); err != nil {
		t.Fatalf("save reverse config failed: %v", err)
	}

	rt := &oneBotRuntime{
		sessions:           map[string]*oneBotSession{},
		reverseControllers: map[string]*oneBotReverseController{},
	}
	rt.startReverseRuntime()

	select {
	case payload := <-connectEvents:
		if payload["post_type"] != "meta_event" {
			t.Fatalf("post_type = %#v, want %q", payload["post_type"], "meta_event")
		}
		if payload["meta_event_type"] != "lifecycle" {
			t.Fatalf("meta_event_type = %#v, want %q", payload["meta_event_type"], "lifecycle")
		}
		if payload["sub_type"] != "connect" {
			t.Fatalf("sub_type = %#v, want %q", payload["sub_type"], "connect")
		}
		switch got := payload["self_id"].(type) {
		case float64:
			if int64(got) != selfID {
				t.Fatalf("self_id = %d, want %d", int64(got), selfID)
			}
		default:
			t.Fatalf("unexpected self_id type: %T", payload["self_id"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fresh runtime did not establish reverse websocket in time")
	}

	rt.reverseMu.Lock()
	controller := rt.reverseControllers[botUser.ID]
	rt.reverseMu.Unlock()
	if controller == nil {
		t.Fatal("reverse controller not registered")
	}
	controller.cancel()
	_ = token
}

func TestOneBotReverseDialLoopReconnectsAndStillHandlesActions(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, token := createOneBotTestBot(t, "reverse-action-reconnect", model.BotKindManual)
	selfID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
	if err != nil {
		t.Fatalf("create reverse self_id failed: %v", err)
	}

	var accepted atomic.Int32
	responseCh := make(chan oneBotActionResponse, 4)
	upgrader := fastws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		attempt := accepted.Add(1)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		// 先读 SealChat 作为实现端主动发出的 lifecycle connect 元事件。
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}

		req := map[string]any{
			"action": "get_login_info",
			"params": map[string]any{},
			"echo":   fmt.Sprintf("echo-%d", attempt),
		}
		if err := conn.WriteJSON(req); err != nil {
			t.Errorf("write get_login_info failed: %v", err)
			return
		}

		_, body, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read get_login_info response failed: %v", err)
			return
		}
		var resp oneBotActionResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Errorf("decode get_login_info response failed: %v", err)
			return
		}
		responseCh <- resp

		if attempt == 1 {
			return
		}

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	controller := &oneBotReverseController{
		botUserID: botUser.ID,
		stop:      make(chan struct{}),
	}
	rt := &oneBotRuntime{
		sessions:           map[string]*oneBotSession{},
		reverseControllers: map[string]*oneBotReverseController{},
	}

	go rt.runReverseDialLoop(
		controller,
		botUser,
		token,
		selfID,
		50,
		oneBotSessionRoleUniversal,
		strings.Replace(server.URL, "http://", "ws://", 1),
	)

	for attempt := 1; attempt <= 2; attempt++ {
		select {
		case resp := <-responseCh:
			if resp.Status != "ok" {
				controller.cancel()
				t.Fatalf("attempt %d response status = %q, want ok", attempt, resp.Status)
			}
			if resp.Echo != fmt.Sprintf("echo-%d", attempt) {
				controller.cancel()
				t.Fatalf("attempt %d response echo = %#v, want %q", attempt, resp.Echo, fmt.Sprintf("echo-%d", attempt))
			}
			data, ok := resp.Data.(map[string]any)
			if !ok {
				controller.cancel()
				t.Fatalf("attempt %d response data type = %T", attempt, resp.Data)
			}
			switch got := data["user_id"].(type) {
			case float64:
				if int64(got) != selfID {
					controller.cancel()
					t.Fatalf("attempt %d user_id = %d, want %d", attempt, int64(got), selfID)
				}
			default:
				controller.cancel()
				t.Fatalf("attempt %d unexpected user_id type: %T", attempt, data["user_id"])
			}
		case <-time.After(3 * time.Second):
			controller.cancel()
			t.Fatalf("did not receive get_login_info response for attempt %d", attempt)
		}
	}

	controller.cancel()
}
