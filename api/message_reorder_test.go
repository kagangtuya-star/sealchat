package api

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/mikespook/gorbac"

	"sealchat/model"
	"sealchat/pm"
)

type reorderTestInput = struct {
	ChannelID       string `json:"channel_id"`
	MessageID       string `json:"message_id"`
	BeforeID        string `json:"before_id"`
	AfterID         string `json:"after_id"`
	ClientOpID      string `json:"client_op_id"`
	ExpectedVersion *int64 `json:"expected_version"`
}

func TestMessageReorderRebalance(t *testing.T) {
	for _, adjacent := range []bool{false, true} {
		name := "equal-orders"
		if adjacent {
			name = "adjacent-float64-orders"
		}
		t.Run(name, func(t *testing.T) {
			owner, _, channelID := setupRelocateBatchTest(t)
			base := float64(1768754295275)
			for i, id := range []string{"a", "b", "c", "d", "e", "f"} {
				order := base + float64(i)*1024
				if id == "b" {
					order = base
					if adjacent {
						order = math.Nextafter(base, math.Inf(1))
					}
				}
				if err := model.GetDB().Model(&model.MessageModel{}).Where("id = ?", id).UpdateColumn("display_order", order).Error; err != nil {
					t.Fatal(err)
				}
			}
			resp, err := apiMessageReorder(relocateBatchTestCtx(owner), &reorderTestInput{ChannelID: channelID, MessageID: "d", BeforeID: "b", AfterID: "a", ClientOpID: "reorder-" + name})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(resp)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Reset bool `json:"reset"`
			}
			if err := json.Unmarshal(encoded, &result); err != nil {
				t.Fatal(err)
			}
			if !result.Reset {
				t.Fatal("rebalance must invalidate every client's cached display orders")
			}
			replay, err := apiMessageReorder(relocateBatchTestCtx(owner), &reorderTestInput{ChannelID: channelID, MessageID: "d", BeforeID: "b", AfterID: "a", ClientOpID: "reorder-" + name})
			if err != nil {
				t.Fatal(err)
			}
			replayJSON, _ := json.Marshal(replay)
			var replayResult struct {
				Reset bool `json:"reset"`
			}
			if err := json.Unmarshal(replayJSON, &replayResult); err != nil || !replayResult.Reset {
				t.Fatal("idempotent replay must also request order synchronization")
			}
			if got := relocateBatchTestOrder(t, channelID); got != "adbcef" {
				t.Fatalf("order = %s, want adbcef", got)
			}
			var items []model.MessageModel
			model.GetDB().Where("id IN ?", []string{"a", "b", "d"}).Find(&items)
			orders := map[string]float64{}
			for _, item := range items {
				orders[item.ID] = item.DisplayOrder
			}
			if !(orders["a"] < orders["d"] && orders["d"] < orders["b"]) {
				t.Fatalf("moved order must be strictly between its neighbors: %v", orders)
			}
		})
	}
}

func TestMessageReorderOrdinaryPlacement(t *testing.T) {
	owner, _, channelID := setupRelocateBatchTest(t)
	result, err := apiMessageReorder(relocateBatchTestCtx(owner), &reorderTestInput{ChannelID: channelID, MessageID: "d", BeforeID: "b", AfterID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(result)
	var payload struct {
		Reset bool `json:"reset"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil || payload.Reset {
		t.Fatalf("ordinary reorder must not reset the channel: %s", encoded)
	}
	if got := relocateBatchTestOrder(t, channelID); got != "adbcef" {
		t.Fatalf("order = %s", got)
	}
}

type orderGetTestInput = struct {
	ChannelID  string   `json:"channel_id"`
	MessageIDs []string `json:"message_ids"`
}

func TestMessageOrderGetVisibilityAndBounds(t *testing.T) {
	owner, member, channelID := setupRelocateBatchTest(t)
	pm.ChannelRoleSetWithoutDB("ch-"+channelID+"-visitor", []gorbac.Permission{pm.PermFuncChannelRead})
	private := &model.MessageModel{StringPKBaseModel: model.StringPKBaseModel{ID: "private"}, ChannelID: channelID, UserID: owner.ID, Content: "hidden", IsWhisper: true, WhisperTo: "another-user", DisplayOrder: 5000}
	if err := model.GetDB().Create(private).Error; err != nil {
		t.Fatal(err)
	}
	result, err := apiMessageOrderGet(relocateBatchTestCtx(member), &orderGetTestInput{ChannelID: channelID, MessageIDs: []string{"a", "private", "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	orders, ok := result.([]messageDisplayOrder)
	if !ok || len(orders) != 1 || orders[0].ID != "a" {
		t.Fatalf("hidden or missing messages leaked: %#v", result)
	}
	stranger := &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "stranger"}, Username: "stranger"}
	if err := model.GetDB().Create(stranger).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := apiMessageOrderGet(relocateBatchTestCtx(stranger), &orderGetTestInput{ChannelID: channelID, MessageIDs: []string{"a"}}); err == nil {
		t.Fatal("unreadable channel was accepted")
	}
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	if _, err := apiMessageOrderGet(relocateBatchTestCtx(owner), &orderGetTestInput{ChannelID: channelID, MessageIDs: ids}); err == nil {
		t.Fatal("unbounded order read was accepted")
	}
}
