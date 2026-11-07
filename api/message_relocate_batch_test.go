package api

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

type relocateBatchInput = struct {
	ChannelID        string                 `json:"channel_id"`
	MessageIDs       []string               `json:"message_ids"`
	TargetMessageID  string                 `json:"target_message_id"`
	Placement        string                 `json:"placement"`
	Groups           []messageRelocateGroup `json:"groups"`
	ClientOpID       string                 `json:"client_op_id"`
	ExpectedVersions map[string]int64       `json:"expected_versions"`
}

func setupRelocateBatchTest(t *testing.T) (owner, member *model.UserModel, channelID string) {
	t.Helper()
	model.DBInit(&utils.AppConfig{DSN: "file:relocate-batch-" + utils.NewID() + "?mode=memory&cache=shared", SQLite: utils.SQLiteConfig{ReadConnections: 1}})
	pm.Init()
	db := model.GetDB()
	owner = &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "rb-owner"}, Username: "rb-owner", Nickname: "Owner"}
	member = &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "rb-member"}, Username: "rb-member", Nickname: "Member"}
	channelID = "rb-channel"
	rows := []any{
		owner, member,
		&model.WorldModel{StringPKBaseModel: model.StringPKBaseModel{ID: "rb-world"}, Name: "World", Status: "active", OwnerID: owner.ID},
		&model.ChannelModel{StringPKBaseModel: model.StringPKBaseModel{ID: channelID}, WorldID: "rb-world", UserID: owner.ID, Name: "Channel", PermType: "public", Status: "active"},
		&model.WorldMemberModel{WorldID: "rb-world", UserID: owner.ID, Role: "owner"},
		&model.WorldMemberModel{WorldID: "rb-world", UserID: member.ID, Role: "member"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []string{"a", "b", "c", "d", "e", "f"} {
		userID := owner.ID
		if id == "f" {
			userID = member.ID
		}
		msg := model.MessageModel{StringPKBaseModel: model.StringPKBaseModel{ID: id}, ChannelID: channelID, UserID: userID, Content: id, ICMode: "ic", DisplayOrder: float64(i+1) * displayOrderGap}
		if err := db.Create(&msg).Error; err != nil {
			t.Fatal(err)
		}
	}
	return owner, member, channelID
}

func relocateBatchTestCtx(user *model.UserModel) *ChatContext {
	return &ChatContext{User: user, ChannelUsersMap: &utils.SyncMap[string, *utils.SyncSet[string]]{}, UserId2ConnInfo: &utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]{}}
}

func relocateBatchTestOrder(t *testing.T, channelID string) string {
	t.Helper()
	var messages []model.MessageModel
	if err := model.GetDB().Where("channel_id = ? AND is_deleted = ?", channelID, false).Order("display_order asc").Order("created_at asc").Order("id asc").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(messages))
	for _, msg := range messages {
		ids = append(ids, msg.ID)
	}
	return strings.Join(ids, "")
}

func TestMessageRelocateBatchLegacySingleGroupAfter(t *testing.T) {
	owner, _, channelID := setupRelocateBatchTest(t)
	resp, err := apiMessageRelocateBatch(relocateBatchTestCtx(owner), &relocateBatchInput{ChannelID: channelID, MessageIDs: []string{"e", "a"}, TargetMessageID: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if got := relocateBatchTestOrder(t, channelID); got != "bcaedf" {
		t.Fatalf("order = %s", got)
	}
	if text := fmt.Sprintf("%+v", resp); !strings.Contains(text, "TargetMessageID:c") || strings.Contains(text, "Placement:before") {
		t.Fatalf("response = %s", text)
	}
}

func TestMessageRelocateBatchGroupsMoveNonContiguousSetsAtomically(t *testing.T) {
	owner, _, channelID := setupRelocateBatchTest(t)
	ctx := relocateBatchTestCtx(owner)
	// 上移 {b, d, e}：每段移到其上方相邻消息之前，b 位于首位相邻消息 a 之前。
	if _, err := apiMessageRelocateBatch(ctx, &relocateBatchInput{ChannelID: channelID, Placement: "before", Groups: []messageRelocateGroup{
		{MessageIDs: []string{"b"}, TargetMessageID: "a"},
		{MessageIDs: []string{"e", "d"}, TargetMessageID: "c"},
	}}); err != nil {
		t.Fatal(err)
	}
	if got := relocateBatchTestOrder(t, channelID); got != "badecf" {
		t.Fatalf("up order = %s", got)
	}
	// 下移 {b, d}：每段移到其下方相邻消息之后。
	if _, err := apiMessageRelocateBatch(ctx, &relocateBatchInput{ChannelID: channelID, Placement: "after", Groups: []messageRelocateGroup{
		{MessageIDs: []string{"b"}, TargetMessageID: "a"},
		{MessageIDs: []string{"d"}, TargetMessageID: "e"},
	}}); err != nil {
		t.Fatal(err)
	}
	if got := relocateBatchTestOrder(t, channelID); got != "abedcf" {
		t.Fatalf("down order = %s", got)
	}
}

func TestMessageRelocateBatchRejectsInvalidGroupsWithoutPartialMoves(t *testing.T) {
	owner, member, channelID := setupRelocateBatchTest(t)
	cases := []struct {
		name  string
		ctx   *ChatContext
		input *relocateBatchInput
	}{
		{"mixed legacy", relocateBatchTestCtx(owner), &relocateBatchInput{ChannelID: channelID, MessageIDs: []string{"a"}, Groups: []messageRelocateGroup{{MessageIDs: []string{"b"}, TargetMessageID: "c"}}}},
		{"overlap", relocateBatchTestCtx(owner), &relocateBatchInput{ChannelID: channelID, Groups: []messageRelocateGroup{{MessageIDs: []string{"a"}, TargetMessageID: "c"}, {MessageIDs: []string{"a"}, TargetMessageID: "e"}}}},
		{"same target", relocateBatchTestCtx(owner), &relocateBatchInput{ChannelID: channelID, Groups: []messageRelocateGroup{{MessageIDs: []string{"a"}, TargetMessageID: "c"}, {MessageIDs: []string{"b"}, TargetMessageID: "c"}}}},
		{"target inside", relocateBatchTestCtx(owner), &relocateBatchInput{ChannelID: channelID, Groups: []messageRelocateGroup{{MessageIDs: []string{"a"}, TargetMessageID: "d"}, {MessageIDs: []string{"d"}, TargetMessageID: "f"}}}},
		{"empty group", relocateBatchTestCtx(owner), &relocateBatchInput{ChannelID: channelID, Groups: []messageRelocateGroup{{TargetMessageID: "c"}}}},
		{"missing target", relocateBatchTestCtx(owner), &relocateBatchInput{ChannelID: channelID, Groups: []messageRelocateGroup{{MessageIDs: []string{"a"}, TargetMessageID: "missing"}}}},
		{"missing source", relocateBatchTestCtx(owner), &relocateBatchInput{ChannelID: channelID, Groups: []messageRelocateGroup{{MessageIDs: []string{"missing"}, TargetMessageID: "c"}}}},
		{"placement", relocateBatchTestCtx(owner), &relocateBatchInput{ChannelID: channelID, MessageIDs: []string{"a"}, TargetMessageID: "c", Placement: "inside"}},
		{"permission", relocateBatchTestCtx(member), &relocateBatchInput{ChannelID: channelID, Groups: []messageRelocateGroup{{MessageIDs: []string{"f"}, TargetMessageID: "a"}, {MessageIDs: []string{"b"}, TargetMessageID: "d"}}}},
	}
	for _, tc := range cases {
		if _, err := apiMessageRelocateBatch(tc.ctx, tc.input); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
		if got := relocateBatchTestOrder(t, channelID); got != "abcdef" {
			t.Fatalf("%s: partial move, order = %s", tc.name, got)
		}
	}

	var stale model.MessageModel
	if err := model.GetDB().Where("id = ?", "d").Take(&stale).Error; err != nil {
		t.Fatal(err)
	}
	var fresh model.MessageModel
	model.GetDB().Where("id = ?", "a").Take(&fresh)
	_, err := apiMessageRelocateBatch(relocateBatchTestCtx(owner), &relocateBatchInput{
		ChannelID: channelID, Placement: "before",
		Groups:           []messageRelocateGroup{{MessageIDs: []string{"a"}, TargetMessageID: "c"}, {MessageIDs: []string{"d"}, TargetMessageID: "f"}},
		ExpectedVersions: map[string]int64{"a": fresh.Version, "d": stale.Version + 5},
	})
	if !errors.Is(err, model.ErrMessageVersionConflict) {
		t.Fatalf("CAS: %v", err)
	}
	if got := relocateBatchTestOrder(t, channelID); got != "abcdef" {
		t.Fatalf("CAS partial move, order = %s", got)
	}
}

func TestMessageRelocateBatchSupportsBoundarySlotsAndKeepsDisplayOrder(t *testing.T) {
	owner, _, channelID := setupRelocateBatchTest(t)
	ctx := relocateBatchTestCtx(owner)

	// 输入故意逆序，执行时仍应按频道当前显示顺序把 b、c 放到第一条 a 之前。
	if _, err := apiMessageRelocateBatch(ctx, &relocateBatchInput{
		ChannelID: channelID,
		Placement: "before",
		Groups: []messageRelocateGroup{{
			MessageIDs:      []string{"c", "b"},
			TargetMessageID: "a",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := relocateBatchTestOrder(t, channelID); got != "bcadef" {
		t.Fatalf("before first order = %s", got)
	}

	// 尾部没有 upper slot，也必须能稳定追加，并保持 a、d 当前相对顺序。
	if _, err := apiMessageRelocateBatch(ctx, &relocateBatchInput{
		ChannelID: channelID,
		Placement: "after",
		Groups: []messageRelocateGroup{{
			MessageIDs:      []string{"d", "a"},
			TargetMessageID: "f",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := relocateBatchTestOrder(t, channelID); got != "bcefad" {
		t.Fatalf("after last order = %s", got)
	}
}

func TestMessageRelocateBatchRebalancesDenseMultiGroupSlotsOnce(t *testing.T) {
	owner, _, channelID := setupRelocateBatchTest(t)
	db := model.GetDB()

	// 人为压缩相邻 display_order，使两个目标空隙都不足以容纳移动消息，
	// 覆盖“事务内 rebalance 后重新计算全部 group slot”的重试路径。
	for i, id := range []string{"a", "b", "c", "d", "e", "f"} {
		order := 1 + float64(i)*displayOrderEpsilon/4
		if err := db.Model(&model.MessageModel{}).Where("id = ?", id).UpdateColumn("display_order", order).Error; err != nil {
			t.Fatal(err)
		}
	}

	versions := map[string]int64{}
	for _, id := range []string{"e", "f"} {
		var msg model.MessageModel
		if err := db.Where("id = ?", id).Take(&msg).Error; err != nil {
			t.Fatal(err)
		}
		versions[id] = msg.Version
	}

	if _, err := apiMessageRelocateBatch(relocateBatchTestCtx(owner), &relocateBatchInput{
		ChannelID: channelID,
		Placement: "before",
		Groups: []messageRelocateGroup{
			{MessageIDs: []string{"e"}, TargetMessageID: "c"},
			{MessageIDs: []string{"f"}, TargetMessageID: "d"},
		},
		ExpectedVersions: versions,
	}); err != nil {
		t.Fatal(err)
	}
	if got := relocateBatchTestOrder(t, channelID); got != "abecfd" {
		t.Fatalf("rebalance order = %s", got)
	}

	// rebalance 与最终落位都会修改 display_order，因此版本可能递增多次；
	// 这里只要求版本前进，避免把实现细节写死在回归测试里。
	for id, before := range versions {
		var msg model.MessageModel
		if err := db.Where("id = ?", id).Take(&msg).Error; err != nil {
			t.Fatal(err)
		}
		if msg.Version <= before {
			t.Fatalf("%s version = %d, want > %d", id, msg.Version, before)
		}
	}
}
