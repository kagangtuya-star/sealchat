package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

func setupVisualStepBatchTest(t *testing.T) (*ChatContext, *model.LogProjectModel, *messageRelocateBatchRequest) {
	t.Helper()
	owner, _, channelID := setupRelocateBatchTest(t)
	const sectionID = "00000000-0000-4000-8000-000000000323"
	const elementID = "00000000-0000-4000-8000-000000000324"
	raw, err := json.Marshal(map[string]any{
		"sections":  []any{map[string]any{"id": sectionID, "source": map[string]any{"channelId": channelID, "toTime": time.Now().Add(time.Hour), "includeOoc": true, "includeArchived": true}}},
		"elements":  []any{map[string]any{"id": elementID, "anchor": service.LogAnchor{Kind: "message", SectionID: sectionID, MessageID: "a", Side: "before"}, "order": 0, "content": map[string]any{"text": "Keep element"}}},
		"extension": "keep",
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &model.LogProjectModel{StringPKBaseModel: model.StringPKBaseModel{ID: "visual-step-project"}, WorldID: "rb-world", Enabled: true, DraftVersion: 1, DraftJSON: string(raw)}
	if err := model.GetDB().Create(p).Error; err != nil {
		t.Fatal(err)
	}
	return relocateBatchTestCtx(owner), p, &messageRelocateBatchRequest{
		ChannelID: channelID, Placement: "before", ClientOpID: "visual-step",
		Groups:           []messageRelocateGroup{{MessageIDs: []string{"b"}, TargetMessageID: "a"}, {MessageIDs: []string{"d"}, TargetMessageID: "c"}},
		ExpectedVersions: map[string]int64{"b": 1, "d": 1},
		LogDraft: &service.LogMessageReorderDraft{ProjectID: p.ID, SectionID: sectionID, ExpectedVersion: 1, VisualStep: true,
			Placements: []service.LogElementPlacement{{ElementID: elementID, Anchor: service.LogAnchor{Kind: "message", SectionID: sectionID, MessageID: "c", Side: "after"}, Order: 2}}},
	}
}

func TestMessageRelocateBatchVisualStep(t *testing.T) {
	for _, mode := range []string{"multiple ranks", "mixed", "anchors only", "rebalance"} {
		t.Run(mode, func(t *testing.T) {
			ctx, project, input := setupVisualStepBatchTest(t)
			want := "badcef"
			if mode == "mixed" {
				input.Groups[1].TargetMessageID = ""
				want = "bacdef"
			}
			if mode == "anchors only" {
				input.Groups[0].TargetMessageID = ""
				input.Groups[1].TargetMessageID = ""
				want = "abcdef"
			}
			if mode == "rebalance" {
				if err := model.GetDB().Model(&model.MessageModel{}).Where("id = ?", "c").UpdateColumn("display_order", displayOrderGap+displayOrderEpsilon/4).Error; err != nil {
					t.Fatal(err)
				}
				// d is inserted before c in the saturated a/c gap; b moves before a.
				want = "badcef"
			}
			for replay := 0; replay < 2; replay++ {
				result, err := apiMessageRelocateBatch(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				encoded, _ := json.Marshal(result)
				if !strings.Contains(string(encoded), `"draft_version":2`) || relocateBatchTestOrder(t, input.ChannelID) != want {
					t.Fatalf("order=%s response=%s", relocateBatchTestOrder(t, input.ChannelID), encoded)
				}
				var got model.LogProjectModel
				if err := model.GetDB().Where("id = ?", project.ID).Take(&got).Error; err != nil {
					t.Fatal(err)
				}
				if got.DraftVersion != 2 || !strings.Contains(got.DraftJSON, `"messageId":"c"`) || !strings.Contains(got.DraftJSON, `"extension":"keep"`) || !strings.Contains(got.DraftJSON, `"text":"Keep element"`) {
					t.Fatalf("draft=%s version=%d", got.DraftJSON, got.DraftVersion)
				}
			}
		})
	}
}

func TestMessageRelocateBatchVisualStepRejectsAndRollsBack(t *testing.T) {
	for _, mode := range []string{"message conflict", "draft conflict", "missing anchor", "future anchor", "excluded anchor", "foreign channel", "permission", "missing versions", "missing operation", "legacy anchors", "legacy stationary anchor", "legacy empty target", "overlapping groups"} {
		t.Run(mode, func(t *testing.T) {
			ctx, project, input := setupVisualStepBatchTest(t)
			db := model.GetDB()
			switch mode {
			case "message conflict":
				input.ExpectedVersions["d"]++
			case "draft conflict":
				input.LogDraft.ExpectedVersion++
			case "missing anchor":
				input.LogDraft.Placements[0].Anchor.MessageID = "missing"
			case "future anchor":
				if err := db.Model(&model.MessageModel{}).Where("id = ?", "c").UpdateColumn("created_at", time.Now().Add(2*time.Hour)).Error; err != nil {
					t.Fatal(err)
				}
			case "excluded anchor":
				if err := db.Create(&model.LogMessageExclusionModel{ProjectID: project.ID, SectionID: input.LogDraft.SectionID, MessageID: "c"}).Error; err != nil {
					t.Fatal(err)
				}
			case "foreign channel":
				input.LogDraft.Placements[0].Anchor.MessageID = "foreign"
				if err := db.Create(&model.MessageModel{StringPKBaseModel: model.StringPKBaseModel{ID: "foreign"}, ChannelID: "other", UserID: ctx.User.ID, Content: "foreign"}).Error; err != nil {
					t.Fatal(err)
				}
			case "permission":
				ctx.User.ID = "rb-member"
			case "missing versions":
				input.ExpectedVersions = nil
			case "missing operation":
				input.ClientOpID = ""
			case "legacy anchors":
				input.LogDraft.VisualStep = false
			case "legacy stationary anchor":
				input.LogDraft.VisualStep = false
				input.Groups = input.Groups[:1]
				delete(input.ExpectedVersions, "d")
			case "legacy empty target":
				input.LogDraft = nil
				input.Groups[1].TargetMessageID = ""
			case "overlapping groups":
				input.Groups[1].MessageIDs = []string{"b"}
			}
			if _, err := apiMessageRelocateBatch(ctx, input); err == nil {
				t.Fatal("invalid visual step accepted")
			}
			var got model.LogProjectModel
			if err := db.Where("id = ?", project.ID).Take(&got).Error; err != nil {
				t.Fatal(err)
			}
			if got.DraftVersion != 1 || got.DraftJSON != project.DraftJSON || relocateBatchTestOrder(t, input.ChannelID) != "abcdef" {
				t.Fatal("partially committed order or draft")
			}
			var message model.MessageModel
			if err := db.Where("id = ?", "b").Take(&message).Error; err != nil || message.Version != 1 {
				t.Fatalf("message version changed on failure: %d, %v", message.Version, err)
			}
		})
	}
}

func TestMessageRelocateBatchVisualStepDraftCAS(t *testing.T) {
	ctx, project, input := setupVisualStepBatchTest(t)
	commit, err := service.PrepareLogMessagesReorder(ctx.User.ID, input.ChannelID, []string{"b", "d"}, *input.LogDraft)
	if err != nil {
		t.Fatal(err)
	}
	db := model.GetDB()
	if err := db.Model(&model.LogProjectModel{}).Where("id = ?", project.ID).UpdateColumn("draft_version", 2).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		version := int64(1)
		if err := model.MessageUpdateTx(tx, "b", &version, map[string]any{"display_order": 0}); err != nil {
			return err
		}
		return commit.Apply(tx)
	})
	if err == nil || !strings.Contains(err.Error(), "VERSION_CONFLICT") || relocateBatchTestOrder(t, input.ChannelID) != "abcdef" {
		t.Fatalf("draft CAS did not roll back message order: %v", err)
	}
}

type relocateBatchInput = messageRelocateBatchRequest

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
