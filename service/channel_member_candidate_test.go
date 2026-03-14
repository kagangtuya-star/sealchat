package service

import (
	"fmt"
	"testing"
	"time"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

func TestChannelMemberCandidatesAndBatchAdd(t *testing.T) {
	initTestDB(t)
	pm.Init()
	db := model.GetDB()

	worldID := "world-candidate-" + utils.NewID()
	ownerID := "owner-" + utils.NewID()
	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: ownerID},
		Username:          "owner_" + worldID,
		Nickname:          "Owner",
		Password:          "pw",
		Salt:              "salt",
	}).Error; err != nil {
		t.Fatalf("create owner failed: %v", err)
	}
	if err := db.Create(&model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID},
		Name:              "Candidate World",
		Visibility:        model.WorldVisibilityPublic,
		Status:            "active",
		OwnerID:           ownerID,
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}
	if err := db.Create(&model.WorldMemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "wm-owner-" + utils.NewID()},
		WorldID:           worldID,
		UserID:            ownerID,
		Role:              model.WorldRoleOwner,
		JoinedAt:          time.Now().Add(-time.Hour),
	}).Error; err != nil {
		t.Fatalf("create owner member failed: %v", err)
	}

	channelID := "channel-candidate-" + utils.NewID()
	channel := ChannelNew(channelID, "non-public", "Candidate Channel", worldID, ownerID, "")
	if channel == nil {
		t.Fatal("channel create returned nil")
	}

	baseTime := time.Now().Add(-30 * time.Minute)
	latestUserID := ""
	for i := 0; i < 25; i++ {
		userID := fmt.Sprintf("candidate-user-%02d-%s", i, worldID)
		if i == 24 {
			latestUserID = userID
		}
		if err := db.Create(&model.UserModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: userID},
			Username:          fmt.Sprintf("member_%02d_%s", i, worldID),
			Nickname:          fmt.Sprintf("成员%02d", i),
			Password:          "pw",
			Salt:              "salt",
		}).Error; err != nil {
			t.Fatalf("create user %d failed: %v", i, err)
		}
		if err := db.Create(&model.WorldMemberModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: "wm-" + userID},
			WorldID:           worldID,
			UserID:            userID,
			Role:              model.WorldRoleMember,
			JoinedAt:          baseTime.Add(time.Duration(i) * time.Minute),
		}).Error; err != nil {
			t.Fatalf("create world member %d failed: %v", i, err)
		}
	}
	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "spectator-" + worldID},
		Username:          "spectator_" + worldID,
		Nickname:          "旁观者",
		Password:          "pw",
		Salt:              "salt",
	}).Error; err != nil {
		t.Fatalf("create spectator user failed: %v", err)
	}
	if err := db.Create(&model.WorldMemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "wm-spectator-" + worldID},
		WorldID:           worldID,
		UserID:            "spectator-" + worldID,
		Role:              model.WorldRoleSpectator,
		JoinedAt:          time.Now(),
	}).Error; err != nil {
		t.Fatalf("create spectator member failed: %v", err)
	}

	listResp, err := ListChannelMemberCandidates(ChannelMemberCandidateQuery{
		ChannelID: channelID,
		ActorID:   ownerID,
		Page:      1,
		PageSize:  10,
		RoleKey:   "member",
	})
	if err != nil {
		t.Fatalf("list candidates failed: %v", err)
	}
	if listResp.Total != 26 {
		t.Fatalf("candidate total=%d expect 26", listResp.Total)
	}
	if len(listResp.Items) != 10 {
		t.Fatalf("candidate items=%d expect 10", len(listResp.Items))
	}
	if listResp.Items[0].UserID != latestUserID {
		t.Fatalf("first candidate=%s expect latest %s", listResp.Items[0].UserID, latestUserID)
	}

	memberRoleID := buildChannelRoleID(channelID, "member")
	if _, err := model.UserRoleLink([]string{memberRoleID}, []string{latestUserID}); err != nil {
		t.Fatalf("seed existing member role failed: %v", err)
	}

	excludeResp, err := ListChannelMemberCandidates(ChannelMemberCandidateQuery{
		ChannelID:       channelID,
		ActorID:         ownerID,
		Page:            1,
		PageSize:        30,
		RoleKey:         "member",
		ExcludeExisting: true,
	})
	if err != nil {
		t.Fatalf("list candidates exclude existing failed: %v", err)
	}
	if excludeResp.Total != 25 {
		t.Fatalf("exclude total=%d expect 25", excludeResp.Total)
	}
	for _, item := range excludeResp.Items {
		if item.UserID == latestUserID {
			t.Fatalf("exclude existing still contains %s", latestUserID)
		}
	}

	addResp, err := AddWorldMembersToChannel(ChannelAddWorldMembersParams{
		ChannelID: channelID,
		ActorID:   ownerID,
	})
	if err != nil {
		t.Fatalf("add world members failed: %v", err)
	}
	if addResp.CandidateCount != 26 {
		t.Fatalf("candidate count=%d expect 26", addResp.CandidateCount)
	}
	if addResp.SkippedExistingCount != 1 {
		t.Fatalf("skipped existing=%d expect 1", addResp.SkippedExistingCount)
	}
	if addResp.AddedCount != 25 {
		t.Fatalf("added count=%d expect 25", addResp.AddedCount)
	}

	var linkedCount int64
	if err := db.Model(&model.UserRoleMappingModel{}).
		Where("role_id = ?", memberRoleID).
		Count(&linkedCount).Error; err != nil {
		t.Fatalf("count member role links failed: %v", err)
	}
	if linkedCount != 26 {
		t.Fatalf("member role link count=%d expect 26", linkedCount)
	}
}
