package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"sealchat/model"
	"sealchat/utils"
)

func TestBuildWhisperVisibilityDiffComputesAddedKeptRemoved(t *testing.T) {
	updateTargets, removeTargets := buildWhisperVisibilityDiff("author", []string{"u1", "u2", "u2"}, []string{"u2", "u3", ""})

	sort.Strings(updateTargets)
	sort.Strings(removeTargets)

	wantUpdate := []string{"author", "u2", "u3"}
	wantRemove := []string{"u1"}

	if len(updateTargets) != len(wantUpdate) {
		t.Fatalf("update target count = %d, want %d; got=%v", len(updateTargets), len(wantUpdate), updateTargets)
	}
	for i := range wantUpdate {
		if updateTargets[i] != wantUpdate[i] {
			t.Fatalf("updateTargets[%d] = %q, want %q (all=%v)", i, updateTargets[i], wantUpdate[i], updateTargets)
		}
	}

	if len(removeTargets) != len(wantRemove) {
		t.Fatalf("remove target count = %d, want %d; got=%v", len(removeTargets), len(wantRemove), removeTargets)
	}
	for i := range wantRemove {
		if removeTargets[i] != wantRemove[i] {
			t.Fatalf("removeTargets[%d] = %q, want %q (all=%v)", i, removeTargets[i], wantRemove[i], removeTargets)
		}
	}
}

func TestBuildWhisperVisibilityDiffAlwaysKeepsAuthorOutOfRemoved(t *testing.T) {
	updateTargets, removeTargets := buildWhisperVisibilityDiff("author", []string{"author", "u1"}, nil)

	sort.Strings(updateTargets)
	sort.Strings(removeTargets)

	wantUpdate := []string{"author"}
	wantRemove := []string{"u1"}

	if len(updateTargets) != len(wantUpdate) {
		t.Fatalf("update target count = %d, want %d; got=%v", len(updateTargets), len(wantUpdate), updateTargets)
	}
	for i := range wantUpdate {
		if updateTargets[i] != wantUpdate[i] {
			t.Fatalf("updateTargets[%d] = %q, want %q (all=%v)", i, updateTargets[i], wantUpdate[i], updateTargets)
		}
	}

	if len(removeTargets) != len(wantRemove) {
		t.Fatalf("remove target count = %d, want %d; got=%v", len(removeTargets), len(wantRemove), removeTargets)
	}
	for i := range wantRemove {
		if removeTargets[i] != wantRemove[i] {
			t.Fatalf("removeTargets[%d] = %q, want %q (all=%v)", i, removeTargets[i], wantRemove[i], removeTargets)
		}
	}
}

func initMessageUpdateWhisperTestDB(t *testing.T) {
	t.Helper()
	cfg := &utils.AppConfig{
		DSN: fmt.Sprintf("file:api-message-update-whisper-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: false,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	}
	model.DBInit(cfg)
}

func createMessageUpdateWhisperTestUser(t *testing.T, id string) *model.UserModel {
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

func createMessageUpdateWhisperTestMember(t *testing.T, channelID, userID string) {
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

func TestAPIMessageUpdateReplacesWhisperRecipients(t *testing.T) {
	initMessageUpdateWhisperTestDB(t)

	author := createMessageUpdateWhisperTestUser(t, "author")
	target1 := createMessageUpdateWhisperTestUser(t, "target1")
	target2 := createMessageUpdateWhisperTestUser(t, "target2")
	target3 := createMessageUpdateWhisperTestUser(t, "target3")

	world := &model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "world-1"},
		Name:              "World",
		Status:            "active",
		OwnerID:           author.ID,
	}
	if err := model.GetDB().Create(world).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}

	channel := &model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "channel-1"},
		WorldID:           world.ID,
		Name:              "Channel",
		PermType:          "public",
		Status:            "active",
	}
	if err := model.GetDB().Create(channel).Error; err != nil {
		t.Fatalf("create channel failed: %v", err)
	}

	createMessageUpdateWhisperTestMember(t, channel.ID, author.ID)
	createMessageUpdateWhisperTestMember(t, channel.ID, target1.ID)
	createMessageUpdateWhisperTestMember(t, channel.ID, target2.ID)
	createMessageUpdateWhisperTestMember(t, channel.ID, target3.ID)

	msg := &model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "msg-1"},
		ChannelID:         channel.ID,
		UserID:            author.ID,
		MemberID:          "mem-" + author.ID,
		Content:           "before",
		WidgetData:        "",
		IsWhisper:         true,
		WhisperTo:         target1.ID,
		ICMode:            "ic",
	}
	if err := model.GetDB().Create(msg).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	if err := model.CreateWhisperRecipients(msg.ID, []string{target1.ID, target2.ID}); err != nil {
		t.Fatalf("seed whisper recipients failed: %v", err)
	}

	ctx := &ChatContext{
		User:            author,
		ChannelUsersMap: &utils.SyncMap[string, *utils.SyncSet[string]]{},
		UserId2ConnInfo: &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{},
	}

	resp, err := apiMessageUpdate(ctx, &struct {
		ChannelID         string   `json:"channel_id"`
		MessageID         string   `json:"message_id"`
		Content           string   `json:"content"`
		WhisperToIds      []string `json:"whisper_to_ids"`
		ICMode            string   `json:"ic_mode"`
		IdentityID        *string  `json:"identity_id"`
		IdentityVariantID *string  `json:"identity_variant_id"`
	}{
		ChannelID:    channel.ID,
		MessageID:    msg.ID,
		Content:      "after",
		WhisperToIds: []string{target2.ID, target3.ID},
		ICMode:       "ic",
	})
	if err != nil {
		t.Fatalf("apiMessageUpdate failed: %v", err)
	}

	recipientIDs := model.GetWhisperRecipientIDs(msg.ID)
	sort.Strings(recipientIDs)
	wantRecipients := []string{target2.ID, target3.ID}
	if len(recipientIDs) != len(wantRecipients) {
		t.Fatalf("recipient count = %d, want %d; got=%v", len(recipientIDs), len(wantRecipients), recipientIDs)
	}
	for i := range wantRecipients {
		if recipientIDs[i] != wantRecipients[i] {
			t.Fatalf("recipientIDs[%d] = %q, want %q (all=%v)", i, recipientIDs[i], wantRecipients[i], recipientIDs)
		}
	}

	var stored model.MessageModel
	if err := model.GetDB().Where("id = ?", msg.ID).Limit(1).Find(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.WhisperTo != target2.ID {
		t.Fatalf("stored whisper_to = %q, want %q", stored.WhisperTo, target2.ID)
	}

	var payload struct {
		Message struct {
			Content      string `json:"content"`
			WhisperToIds []struct {
				ID string `json:"id"`
			} `json:"whisperToIds"`
		} `json:"message"`
	}
	raw, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		t.Fatalf("marshal response failed: %v", marshalErr)
	}
	if unmarshalErr := json.Unmarshal(raw, &payload); unmarshalErr != nil {
		t.Fatalf("unmarshal response failed: %v", unmarshalErr)
	}
	if payload.Message.Content != "after" {
		t.Fatalf("response content = %q, want %q", payload.Message.Content, "after")
	}
	if len(payload.Message.WhisperToIds) != 2 {
		t.Fatalf("response whisper target count = %d, want 2", len(payload.Message.WhisperToIds))
	}
}

func TestAPIMessageUpdatePersistsWhisperRecipientsWithoutContentChange(t *testing.T) {
	initMessageUpdateWhisperTestDB(t)

	author := createMessageUpdateWhisperTestUser(t, "author_same")
	target1 := createMessageUpdateWhisperTestUser(t, "target_same_1")
	target2 := createMessageUpdateWhisperTestUser(t, "target_same_2")
	target3 := createMessageUpdateWhisperTestUser(t, "target_same_3")

	world := &model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "world-same"},
		Name:              "World",
		Status:            "active",
		OwnerID:           author.ID,
	}
	if err := model.GetDB().Create(world).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}

	channel := &model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "channel-same"},
		WorldID:           world.ID,
		Name:              "Channel",
		PermType:          "public",
		Status:            "active",
	}
	if err := model.GetDB().Create(channel).Error; err != nil {
		t.Fatalf("create channel failed: %v", err)
	}

	createMessageUpdateWhisperTestMember(t, channel.ID, author.ID)
	createMessageUpdateWhisperTestMember(t, channel.ID, target1.ID)
	createMessageUpdateWhisperTestMember(t, channel.ID, target2.ID)
	createMessageUpdateWhisperTestMember(t, channel.ID, target3.ID)

	msg := &model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "msg-same"},
		ChannelID:         channel.ID,
		UserID:            author.ID,
		MemberID:          "mem-" + author.ID,
		Content:           "same-content",
		WidgetData:        "",
		IsWhisper:         true,
		WhisperTo:         target1.ID,
		ICMode:            "ic",
	}
	if err := model.GetDB().Create(msg).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}
	if err := model.CreateWhisperRecipients(msg.ID, []string{target1.ID, target2.ID}); err != nil {
		t.Fatalf("seed whisper recipients failed: %v", err)
	}

	ctx := &ChatContext{
		User:            author,
		ChannelUsersMap: &utils.SyncMap[string, *utils.SyncSet[string]]{},
		UserId2ConnInfo: &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{},
	}

	resp, err := apiMessageUpdate(ctx, &struct {
		ChannelID         string   `json:"channel_id"`
		MessageID         string   `json:"message_id"`
		Content           string   `json:"content"`
		WhisperToIds      []string `json:"whisper_to_ids"`
		ICMode            string   `json:"ic_mode"`
		IdentityID        *string  `json:"identity_id"`
		IdentityVariantID *string  `json:"identity_variant_id"`
	}{
		ChannelID:    channel.ID,
		MessageID:    msg.ID,
		Content:      "same-content",
		WhisperToIds: []string{target2.ID, target3.ID},
		ICMode:       "ic",
	})
	if err != nil {
		t.Fatalf("apiMessageUpdate failed: %v", err)
	}

	recipientIDs := model.GetWhisperRecipientIDs(msg.ID)
	sort.Strings(recipientIDs)
	wantRecipients := []string{target2.ID, target3.ID}
	if len(recipientIDs) != len(wantRecipients) {
		t.Fatalf("recipient count = %d, want %d; got=%v", len(recipientIDs), len(wantRecipients), recipientIDs)
	}
	for i := range wantRecipients {
		if recipientIDs[i] != wantRecipients[i] {
			t.Fatalf("recipientIDs[%d] = %q, want %q (all=%v)", i, recipientIDs[i], wantRecipients[i], recipientIDs)
		}
	}

	var stored model.MessageModel
	if err := model.GetDB().Where("id = ?", msg.ID).Limit(1).Find(&stored).Error; err != nil {
		t.Fatalf("reload message failed: %v", err)
	}
	if stored.WhisperTo != target2.ID {
		t.Fatalf("stored whisper_to = %q, want %q", stored.WhisperTo, target2.ID)
	}
	if !stored.IsEdited {
		t.Fatalf("stored is_edited = false, want true")
	}
	if stored.EditCount != 1 {
		t.Fatalf("stored edit_count = %d, want 1", stored.EditCount)
	}

	var payload struct {
		Message struct {
			Content      string `json:"content"`
			WhisperToIds []struct {
				ID string `json:"id"`
			} `json:"whisperToIds"`
		} `json:"message"`
	}
	raw, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		t.Fatalf("marshal response failed: %v", marshalErr)
	}
	if unmarshalErr := json.Unmarshal(raw, &payload); unmarshalErr != nil {
		t.Fatalf("unmarshal response failed: %v", unmarshalErr)
	}
	if payload.Message.Content != "same-content" {
		t.Fatalf("response content = %q, want %q", payload.Message.Content, "same-content")
	}
	if len(payload.Message.WhisperToIds) != 2 {
		t.Fatalf("response whisper target count = %d, want 2", len(payload.Message.WhisperToIds))
	}
	if payload.Message.WhisperToIds[0].ID != target2.ID {
		t.Fatalf("response first whisper target = %q, want %q", payload.Message.WhisperToIds[0].ID, target2.ID)
	}
}
