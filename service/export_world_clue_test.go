package service

import (
	"testing"
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

type worldClueTestUsers struct{ owner, memberA, memberB, worldID string }

func setupWorldClueTest(t *testing.T) worldClueTestUsers {
	t.Helper()
	owner, world, _ := initWorldTheaterServiceTest(t)
	users := worldClueTestUsers{owner, "member-a", "member-b", world}
	for _, id := range []string{users.owner, users.memberA, users.memberB} {
		if err := model.GetDB().Create(&model.UserModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: id}, Username: id, Nickname: id,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{users.memberA, users.memberB} {
		if err := model.GetDB().Create(&model.WorldMemberModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			WorldID:           world, UserID: id, Role: model.WorldRoleMember, JoinedAt: time.Now(),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return users
}

func createWorldClueForTest(t *testing.T, users worldClueTestUsers, access string) *protocol.WorldClueAdminDetail {
	t.Helper()
	clue, err := WorldClueCreate(users.worldID, users.owner, WorldClueCreateInput{
		Title: "Ancient Key", Kind: model.WorldClueKindText, ContentFormat: model.WorldClueContentPlain,
		Content: "An old key", DefaultAccess: access,
		ManagerNoteFormat: model.WorldClueContentPlain, ManagerNote: "manager secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	return clue
}
