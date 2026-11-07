package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

func TestIssue299WhisperCandidatesUseExplicitMembers(t *testing.T) {
	for _, permType := range []string{"public", "non-public"} {
		t.Run(permType, func(t *testing.T) {
			model.DBInit(&utils.AppConfig{DSN: "file:issue299-" + utils.NewID() + "?mode=memory&cache=shared"})
			pm.Init()
			db := model.GetDB()
			const channelID, worldID, actorID = "issue299", "issue299-world", "actor"
			userIDs := []string{actorID, "manual", "inherited", "both", "visited", "admin", "bot", "spectator", "observer", "visitor"}
			for _, id := range userIDs {
				if err := db.Create(&model.UserModel{
					StringPKBaseModel: model.StringPKBaseModel{ID: id}, Username: id, Nickname: id, Password: "pw", Salt: "salt",
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&model.WorldModel{
				StringPKBaseModel: model.StringPKBaseModel{ID: worldID}, Name: worldID, OwnerID: actorID, Status: "active",
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&model.WorldMemberModel{WorldID: worldID, UserID: actorID, Role: model.WorldRoleOwner}).Error; err != nil {
				t.Fatal(err)
			}
			if service.ChannelNew(channelID, permType, channelID, worldID, actorID, "") == nil {
				t.Fatal("create channel failed")
			}
			for _, grant := range []struct{ userID, role, source string }{
				{"manual", "member", model.RoleGrantManual}, {"inherited", "member", model.RoleGrantWorld},
				{"both", "member", model.RoleGrantBoth}, {"admin", "admin", model.RoleGrantWorld},
				{"bot", "bot", model.RoleGrantManual}, {"spectator", "spectator", model.RoleGrantWorld},
				{"observer", "ob", model.RoleGrantManual}, {"visitor", "visitor", model.RoleGrantManual},
			} {
				if err := model.UserRoleMappingCreate(&model.UserRoleMappingModel{
					UserID: grant.userID, RoleType: "channel", RoleID: "ch-" + channelID + "-" + grant.role, GrantSource: grant.source,
				}); err != nil {
					t.Fatal(err)
				}
			}
			// A past visit or an identity card must not independently qualify a user.
			for _, id := range []string{"visited", "inherited", "both"} {
				if _, err := model.MemberGetByUserIDAndChannelID(id, channelID, id); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&model.ChannelIdentityModel{ChannelID: channelID, UserID: "inherited", DisplayName: "Inherited identity"}).Error; err != nil {
				t.Fatal(err)
			}
			app := fiber.New()
			app.Use(func(c *fiber.Ctx) error {
				c.Locals("user", model.UserGet(actorID))
				return c.Next()
			})
			app.Get("/channels/:channelId/whisper-candidates", ChannelWhisperCandidates)
			app.Get("/members", ChannelMembers)
			app.Post("/unlink", UserRoleUnlink)
			checkCandidates := func(wantBoth bool) {
				t.Helper()
				resp, err := app.Test(httptest.NewRequest("GET", "/channels/"+channelID+"/whisper-candidates", nil))
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				var result struct{ Items []WhisperCandidateUserItem }
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || resp.StatusCode != 200 {
					t.Fatalf("candidates: status=%d err=%v", resp.StatusCode, err)
				}
				got := map[string]bool{}
				for _, item := range result.Items {
					got[item.UserID] = true
				}
				for _, id := range userIDs {
					want := id == "manual" || id == "admin" || id == "bot" ||
						(id == "inherited" && permType != "public") ||
						(id == "both" && (wantBoth || permType != "public"))
					if got[id] != want {
						t.Fatalf("candidate %s = %v, want %v; items=%+v", id, got[id], want, result.Items)
					}
				}
			}
			checkCandidates(true)
			for _, wantCount := range []int64{1, 0} {
				req := httptest.NewRequest("POST", "/unlink", strings.NewReader(`{"roleId":"ch-issue299-member","userIds":["both"]}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				if err != nil {
					t.Fatal(err)
				}
				var result struct{ RemovedCount int64 }
				err = json.NewDecoder(resp.Body).Decode(&result)
				resp.Body.Close()
				if err != nil || resp.StatusCode != 200 || result.RemovedCount != wantCount {
					t.Fatalf("unlink: status=%d result=%+v want=%d err=%v", resp.StatusCode, result, wantCount, err)
				}
				checkCandidates(false)
			}
			// The effective-role endpoint keeps world grants for permission consumers,
			// and exposes their source so management can select only manual grants.
			resp, err := app.Test(httptest.NewRequest("GET", "/members?id="+channelID+"&pageSize=100", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var result struct {
				Items []struct{ UserID, RoleID, GrantSource string }
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || resp.StatusCode != 200 {
				t.Fatalf("members: status=%d err=%v", resp.StatusCode, err)
			}
			found := false
			for _, item := range result.Items {
				if item.UserID == "both" && item.RoleID == "ch-"+channelID+"-member" {
					found = item.GrantSource == model.RoleGrantWorld
				}
			}
			if !found {
				t.Fatalf("world grant/source lost after manual removal: %+v", result.Items)
			}
		})
	}
}

func TestCanUserReadAllWhispersInChannelSafeDefaultFalse(t *testing.T) {
	cases := []struct {
		name      string
		userID    string
		channelID string
	}{
		{name: "empty", userID: "", channelID: ""},
		{name: "unknown-channel", userID: "u1", channelID: "c1"},
		{name: "private-channel-id", userID: "u1", channelID: "private-channel-id-xxxxxxxxxxxxx"},
		{name: "bot", userID: "BOT:1000", channelID: "c1"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if canUserReadAllWhispersInChannel(tt.userID, tt.channelID) {
				t.Fatalf("expected false, got true for user=%q channel=%q", tt.userID, tt.channelID)
			}
		})
	}
}
