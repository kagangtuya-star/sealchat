package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fastws "github.com/fasthttp/websocket"
	wsfiber "github.com/gofiber/contrib/websocket"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

func TestNormalizeBotCommandContentWithPrefixes_ConvertsTipTapCommand(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":".ra "},{"type":"text","marks":[{"type":"italic"}],"text":"侦查"},{"type":"text","text":" "},{"type":"text","marks":[{"type":"code"}],"text":"1d100"}]}]}`
	got := normalizeBotCommandContentWithPrefixes(input, []string{".", "。"})
	want := ".ra *侦查* `1d100`"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveDiceVisualActorUserID(t *testing.T) {
	tests := []struct {
		name            string
		messageAuthorID string
		isBot           bool
		messageContext  *protocol.MessageContext
		want            string
	}{
		{
			name:            "user roll uses message author",
			messageAuthorID: "user-1",
			messageContext:  &protocol.MessageContext{SenderUserID: "stale-user"},
			want:            "user-1",
		},
		{
			name:            "bot response uses triggering user",
			messageAuthorID: "bot-1",
			isBot:           true,
			messageContext:  &protocol.MessageContext{SenderUserID: " user-1 "},
			want:            "user-1",
		},
		{
			name:            "unsolicited bot roll uses bot",
			messageAuthorID: " bot-1 ",
			isBot:           true,
			messageContext:  &protocol.MessageContext{},
			want:            "bot-1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveDiceVisualActorUserID(test.messageAuthorID, test.isBot, test.messageContext)
			if got != test.want {
				t.Fatalf("resolveDiceVisualActorUserID() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseChannelDefaultDiceSetCommand(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		prefixes []string
		want     string
		matched  bool
	}{
		{name: "configured prefix", content: ".set 20", prefixes: []string{".", "。"}, want: "d20", matched: true},
		{name: "case and whitespace", content: " \t/set\tD100\n", prefixes: []string{"/"}, want: "d100", matched: true},
		{name: "longest prefix wins", content: "!!set 6", prefixes: []string{"!", "!!"}, want: "d6", matched: true},
		{name: "rich text", content: `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"。set 12"}]}]}`, prefixes: []string{"。"}, want: "d12", matched: true},
		{name: "missing separator", content: ".set20", prefixes: []string{"."}},
		{name: "additional argument", content: ".set 20 now", prefixes: []string{"."}},
		{name: "invalid sides", content: ".set 0", prefixes: []string{"."}},
		{name: "unconfigured prefix", content: "#set 20", prefixes: []string{"."}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, matched := parseChannelDefaultDiceSetCommand(tt.content, tt.prefixes)
			if matched != tt.matched || got != tt.want {
				t.Fatalf("parseChannelDefaultDiceSetCommand() = (%q, %t), want (%q, %t)", got, matched, tt.want, tt.matched)
			}
		})
	}
}

func TestNormalizeBotCommandContentWithPrefixes_SupportsCustomPrefix(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"/bot "},{"type":"text","marks":[{"type":"bold"}],"text":"help"}]}]}`
	got := normalizeBotCommandContentWithPrefixes(input, []string{"/"})
	want := "/bot **help**"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeBotCommandContentWithPrefixes_LeavesNonCommandRichTextUntouched(t *testing.T) {
	input := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"普通消息 "},{"type":"text","marks":[{"type":"italic"}],"text":"不会变"}]}]}`
	got := normalizeBotCommandContentWithPrefixes(input, []string{".", "。"})
	if got != input {
		t.Fatalf("expected original content, got %q", got)
	}
}

func TestNormalizeBotCommandContentWithPrefixes_ConvertsLegacyHTMLCommand(t *testing.T) {
	input := `.st运动<em>*3 特技</em><code>+1</code>`
	got := normalizeBotCommandContentWithPrefixes(input, []string{".", "。"})
	want := `.st运动**3 特技*` + "`+1`"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeBotCommandContentWithPrefixes_ConvertsDiceChipHTMLCommand(t *testing.T) {
	input := `<span class="dice-chip" data-dice-roll-index="0" data-dice-source=".ra"><span class="dice-chip__formula">d100</span><span class="dice-chip__equals">=</span><span class="dice-chip__result">42</span></span> <code>1d100</code> <strong>侦查</strong>`
	got := normalizeBotCommandContentWithPrefixes(input, []string{".", "。"})
	want := ".ra `1d100` **侦查**"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBotNicknameSyncPendingSuppressesCustomPrefixAck(t *testing.T) {
	botID := "bot-custom-prefix"
	channelID := "channel-custom-prefix"
	botNicknameSyncPendingByBotChannel.Delete(botNicknameSyncPendingKey(botID, channelID))

	storeBotNicknameSyncPendingForBot(botID, channelID, "Alice", "sender", time.Now().UnixMilli())
	if !shouldSuppressBotNicknameSyncContent(botID, channelID, "昵称已切换为 Alice") {
		t.Fatal("expected nickname sync ack to be suppressed")
	}

	event := &protocol.Event{
		Type: protocol.EventMessageCreated,
		Message: &protocol.Message{
			ID:      botCommandDispatchMessageIDPrefix + utils.NewID(),
			Content: "#nn Alice",
		},
		MessageContext: &protocol.MessageContext{SenderUserID: "sender"},
	}
	info := &ConnInfo{User: &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: botID}, IsBot: true}}
	storeBotNicknameSyncPending(info, channelID, event)

	if !shouldSuppressBotNicknameSyncContent(botID, channelID, "昵称已切换为 Alice") {
		t.Fatal("expected custom-prefix nickname sync ack to be suppressed")
	}
}

func TestNormalizeEventForBot_EscapesPlainTextAmpersandCommand(t *testing.T) {
	event := &protocol.Event{
		Type: protocol.EventMessageCreated,
		Message: &protocol.Message{
			Content: ".st &手枪伤害=1d6+1",
		},
	}

	got := normalizeEventForBot(event)
	if got == event {
		t.Fatalf("expected cloned event when content changes")
	}
	if got.Message == nil {
		t.Fatalf("expected message to be preserved")
	}
	want := ".st &amp;手枪伤害=1d6+1"
	if got.Message.Content != want {
		t.Fatalf("expected %q, got %q", want, got.Message.Content)
	}
}

func newClosedChatTestConn(t *testing.T) *WsSyncConn {
	t.Helper()

	serverConnCh := make(chan *fastws.Conn, 1)
	upgrader := fastws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConnCh <- conn
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	clientConn, _, err := fastws.DefaultDialer.Dial("ws"+server.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("dial test websocket failed: %v", err)
	}
	defer clientConn.Close()

	serverConn := <-serverConnCh
	conn := &WsSyncConn{Conn: &wsfiber.Conn{Conn: serverConn}}
	_ = conn.Close()
	return conn
}

func newReadableChatTestConn(t *testing.T) (*WsSyncConn, *fastws.Conn, func()) {
	t.Helper()

	serverConnCh := make(chan *fastws.Conn, 1)
	upgrader := fastws.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConnCh <- conn
	}))

	clientConn, _, err := fastws.DefaultDialer.Dial("ws"+server.URL[len("http"):], nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial test websocket failed: %v", err)
	}

	serverConn := <-serverConnCh
	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		server.Close()
	}
	return &WsSyncConn{Conn: &wsfiber.Conn{Conn: serverConn}}, clientConn, cleanup
}

func TestBroadcastEventInChannelRemovesBrokenConnection(t *testing.T) {
	brokenConn := newClosedChatTestConn(t)
	connMap := &utils.SyncMap[*WsSyncConn, *ConnInfo]{}
	connMap.Store(brokenConn, &ConnInfo{
		Conn:          brokenConn,
		ChannelId:     "channel-test",
		LastPingTime:  1,
		LastAliveTime: 1,
	})

	ctx := &ChatContext{
		UserId2ConnInfo: &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{},
	}
	ctx.UserId2ConnInfo.Store("user-test", connMap)

	ctx.BroadcastEventInChannel("channel-test", &protocol.Event{
		Type: protocol.EventMessageCreated,
		Message: &protocol.Message{
			Content: "hello",
		},
	})

	if connMap.Exists(brokenConn) {
		t.Fatal("expected broken websocket connection to be removed after write failure")
	}
}

func TestBroadcastEventInChannelUsesChannelUsersMapTargets(t *testing.T) {
	targetConn, targetClient, targetCleanup := newReadableChatTestConn(t)
	defer targetCleanup()
	otherConn, otherClient, otherCleanup := newReadableChatTestConn(t)
	defer otherCleanup()

	targetMap := &utils.SyncMap[*WsSyncConn, *ConnInfo]{}
	targetMap.Store(targetConn, &ConnInfo{
		Conn:          targetConn,
		User:          &model.UserModel{Nickname: "Target"},
		LastPingTime:  1,
		LastAliveTime: 1,
	})
	otherMap := &utils.SyncMap[*WsSyncConn, *ConnInfo]{}
	otherMap.Store(otherConn, &ConnInfo{
		Conn:          otherConn,
		User:          &model.UserModel{Nickname: "Other"},
		LastPingTime:  1,
		LastAliveTime: 1,
	})

	channelUsers := &utils.SyncMap[string, *utils.SyncSet[string]]{}
	userSet := &utils.SyncSet[string]{}
	userSet.Add("target-user")
	channelUsers.Store("channel-target", userSet)
	userConns := &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{}
	userConns.Store("target-user", targetMap)
	userConns.Store("other-user", otherMap)
	ctx := &ChatContext{
		ChannelUsersMap: channelUsers,
		UserId2ConnInfo: userConns,
	}

	ctx.BroadcastEventInChannel("channel-target", &protocol.Event{
		Type: protocol.EventMessageCreated,
		Message: &protocol.Message{
			Content: "hello",
		},
	})

	_ = targetClient.SetReadDeadline(time.Now().Add(time.Second))
	_, body, err := targetClient.ReadMessage()
	if err != nil {
		t.Fatalf("expected target channel user to receive broadcast: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected non-empty websocket payload")
	}
	_ = otherClient.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, _, err := otherClient.ReadMessage(); err == nil {
		t.Fatal("expected non-channel user not to receive broadcast")
	}
}

func TestBuildChannelPresenceSnapshotUsesConnectionUserWithoutDatabaseLookup(t *testing.T) {
	channelUsers := &utils.SyncMap[string, *utils.SyncSet[string]]{}
	userSet := &utils.SyncSet[string]{}
	userSet.Add("presence-user")
	channelUsers.Store("channel-presence", userSet)

	connMap := &utils.SyncMap[*WsSyncConn, *ConnInfo]{}
	connMap.Store(&WsSyncConn{}, &ConnInfo{
		ChannelId:    "channel-presence",
		LastPingTime: 1234,
		LatencyMs:    56,
		Focused:      true,
		User: &model.UserModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: "presence-user"},
			Nickname:          "Presence User",
		},
	})
	userConns := &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{}
	userConns.Store("presence-user", connMap)

	got := buildChannelPresenceSnapshot("channel-presence", channelUsers, userConns)

	if len(got) != 1 {
		t.Fatalf("expected one presence entry from connection user, got %d", len(got))
	}
	if got[0].User == nil || got[0].User.Nick != "Presence User" {
		t.Fatalf("expected presence user from connection info, got %#v", got[0].User)
	}
	if got[0].Latency != 56 || !got[0].Focused || got[0].LastSeen != 1234 {
		t.Fatalf("unexpected presence metadata: %#v", got[0])
	}
}

func TestBroadcastEventInChannelToUsersSendsExplicitTargetOutsideChannelUserSet(t *testing.T) {
	targetConn, targetClient, targetCleanup := newReadableChatTestConn(t)
	defer targetCleanup()

	targetMap := &utils.SyncMap[*WsSyncConn, *ConnInfo]{}
	targetMap.Store(targetConn, &ConnInfo{
		Conn:          targetConn,
		User:          &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "target-user"}},
		ChannelId:     "channel-target",
		LastPingTime:  1,
		LastAliveTime: 1,
	})

	channelUsers := &utils.SyncMap[string, *utils.SyncSet[string]]{}
	channelUsers.Store("channel-target", &utils.SyncSet[string]{})
	userConns := &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{}
	userConns.Store("target-user", targetMap)
	ctx := &ChatContext{
		ChannelUsersMap: channelUsers,
		UserId2ConnInfo: userConns,
	}

	ctx.BroadcastEventInChannelToUsers("channel-target", []string{"target-user"}, &protocol.Event{
		Type: protocol.EventMessageCreated,
		Message: &protocol.Message{
			Content: "direct",
		},
	})

	_ = targetClient.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := targetClient.ReadMessage(); err != nil {
		t.Fatalf("expected explicit target to receive broadcast: %v", err)
	}
}

func TestBroadcastEventInChannelForBotSkipsDuplicateWriteForWhisperTarget(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "ctx-whisper-target", model.BotKindManual)
	_, channel := createOneBotTestWorldAndChannel(t, botUser.ID)

	targetConn, targetClient, targetCleanup := newReadableChatTestConn(t)
	defer targetCleanup()

	targetInfo := &ConnInfo{
		Conn:          targetConn,
		User:          botUser,
		ChannelId:     channel.ID,
		LastPingTime:  1,
		LastAliveTime: 1,
	}
	targetMap := &utils.SyncMap[*WsSyncConn, *ConnInfo]{}
	targetMap.Store(targetConn, targetInfo)

	userConns := &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{}
	userConns.Store(botUser.ID, targetMap)
	ctx := &ChatContext{
		UserId2ConnInfo: userConns,
	}

	event := &protocol.Event{
		Type:    protocol.EventMessageCreated,
		Channel: channel.ToProtocolType(),
		User:    &protocol.User{ID: "sender-user", Nick: "sender"},
		Message: &protocol.Message{
			ID:        "msg-" + utils.NewIDWithLength(8),
			Content:   ".r",
			IsWhisper: true,
			WhisperTo: &protocol.User{ID: botUser.ID},
		},
		MessageContext: &protocol.MessageContext{
			IsWhisper:       true,
			SenderUserID:    "sender-user",
			WhisperToUserID: botUser.ID,
		},
	}

	ctx.BroadcastEventInChannelToUsers(channel.ID, []string{botUser.ID}, event)

	_ = targetClient.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := targetClient.ReadMessage(); err != nil {
		t.Fatalf("expected whisper target bot to receive direct payload: %v", err)
	}

	ctx.BroadcastEventInChannelForBot(channel.ID, event)

	_ = targetClient.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, _, err := targetClient.ReadMessage(); err == nil {
		t.Fatal("expected bot broadcast path not to duplicate whisper payload")
	}

	msgContext, ok := targetInfo.BotLastMessageContext.Load(channel.ID)
	if !ok || msgContext == nil {
		t.Fatal("expected bot message context to be cached for whisper target")
	}
	if !msgContext.IsWhisper || msgContext.WhisperToUserID != botUser.ID {
		t.Fatalf("unexpected cached message context: %#v", msgContext)
	}
}
