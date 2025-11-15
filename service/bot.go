package service

import (
	"fmt"
	"sealchat/model"
)

func BotListByChannelId(curUserId, channelId string) []string {
	var ids []string
	roleId := fmt.Sprintf("ch-%s-%s", channelId, "bot")
	ids1, _ := model.UserRoleMappingUserIdListByRoleId(roleId)
	ids = append(ids, ids1...)

	ch, _ := model.ChannelGet(channelId)
	if ch.ID != "" && !ch.BotFeatureEnabled {
		return []string{}
	}
	if ch.PermType == "private" {
		// 私聊时获取授权
		var otherId string
		id2 := ch.GetPrivateUserIDs()
		if id2[0] == curUserId {
			otherId = id2[1]
		}
		if id2[1] == curUserId {
			otherId = id2[0]
		}
		u := model.UserGet(otherId)
		if u.IsBot {
			ids = append(ids, otherId)
		}
	} else {
		// 获取子频道的授权
		if ch.RootId != "" {
			roleId := fmt.Sprintf("ch-%s-%s", ch.RootId, "bot")
			ids2, _ := model.UserRoleMappingUserIdListByRoleId(roleId)
			ids = append(ids, ids2...)
		}
	}

	return ids
}
