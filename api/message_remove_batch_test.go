package api

import (
	"encoding/json"
	"fmt"
	"testing"

	"sealchat/model"
	"sealchat/utils"
)

func initMessageRemoveBatchTestDB(t *testing.T) {
	t.Helper()
	cfg := &utils.AppConfig{
		DSN: fmt.Sprintf("file:api-message-remove-batch-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: false,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	}
	model.DBInit(cfg)
}

func createMessageRemoveBatchTestUser(t *testing.T, id string) *model.UserModel {
	t.Helper()
	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: id},
		Username:          "user_" + id,
		Nickname:          "nick_" + id,
		Password:          "pw",
		Salt:              "salt",
	}
	if err := model.GetDB().Create(user).Error; err != nil {
		t.Fatalf("create user %s failed: %v", id, err)
	}
	return user
}

func createMessageRemoveBatchTestMember(t *testing.T, channelID, userID string) {
	t.Helper()
	member := &model.MemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "mem-" + userID},
		ChannelID:         channelID,
		UserID:            userID,
		Nickname:          "member_" + userID,
	}
	if err := model.GetDB().Create(member).Error; err != nil {
		t.Fatalf("create member %s failed: %v", userID, err)
	}
}

func createMessageRemoveBatchTestMessage(t *testing.T, channelID, userID, messageID string) {
	t.Helper()
	msg := &model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: messageID},
		ChannelID:         channelID,
		UserID:            userID,
		MemberID:          "mem-" + userID,
		Content:           "hello " + messageID,
		WidgetData:        "",
		ICMode:            "ic",
	}
	if err := model.GetDB().Create(msg).Error; err != nil {
		t.Fatalf("create message %s failed: %v", messageID, err)
	}
}

func TestAPIMessageRemoveSupportsBatchMessageIDs(t *testing.T) {
	initMessageRemoveBatchTestDB(t)

	author := createMessageRemoveBatchTestUser(t, "author-batch")
	world := &model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "world-batch"},
		Name:              "World",
		Status:            "active",
		OwnerID:           author.ID,
	}
	if err := model.GetDB().Create(world).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}
	channel := &model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "channel-batch"},
		WorldID:           world.ID,
		Name:              "Channel",
		PermType:          "public",
		Status:            "active",
	}
	if err := model.GetDB().Create(channel).Error; err != nil {
		t.Fatalf("create channel failed: %v", err)
	}
	createMessageRemoveBatchTestMember(t, channel.ID, author.ID)
	createMessageRemoveBatchTestMessage(t, channel.ID, author.ID, "msg-batch-1")
	createMessageRemoveBatchTestMessage(t, channel.ID, author.ID, "msg-batch-2")

	ctx := &ChatContext{
		User:            author,
		ChannelUsersMap: &utils.SyncMap[string, *utils.SyncSet[string]]{},
		UserId2ConnInfo: &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{},
	}

	resp, err := apiMessageRemove(ctx, &messageRemovePayload{
		ChannelID:  channel.ID,
		MessageIDs: []string{"msg-batch-1", "msg-batch-2"},
	})
	if err != nil {
		t.Fatalf("apiMessageRemove failed: %v", err)
	}

	var payload struct {
		MessageIDs []string `json:"message_ids"`
		Success    bool     `json:"success"`
	}
	raw, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		t.Fatalf("marshal response failed: %v", marshalErr)
	}
	if unmarshalErr := json.Unmarshal(raw, &payload); unmarshalErr != nil {
		t.Fatalf("unmarshal response failed: %v", unmarshalErr)
	}
	if !payload.Success {
		t.Fatalf("response success = false")
	}
	if len(payload.MessageIDs) != 2 {
		t.Fatalf("response message_ids len = %d, want 2", len(payload.MessageIDs))
	}

	var messages []model.MessageModel
	if err := model.GetDB().Where("channel_id = ? AND id IN ?", channel.ID, []string{"msg-batch-1", "msg-batch-2"}).Order("id asc").Find(&messages).Error; err != nil {
		t.Fatalf("reload messages failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("stored message count = %d, want 2", len(messages))
	}
	for _, msg := range messages {
		if !msg.IsDeleted {
			t.Fatalf("message %s not deleted", msg.ID)
		}
		if msg.DeletedBy != author.ID {
			t.Fatalf("message %s deleted_by = %q, want %q", msg.ID, msg.DeletedBy, author.ID)
		}
		if msg.Content != "" {
			t.Fatalf("message %s content = %q, want empty", msg.ID, msg.Content)
		}
	}
}
