package service

import (
	"fmt"
	"sealchat/model"
)

func BotListByChannelId(curUserId, channelId string) []string {
	var ids []string
	seen := map[string]struct{}{}
	addID := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	roleId := fmt.Sprintf("ch-%s-%s", channelId, "bot")
	ids1, _ := model.UserRoleMappingUserIdListByRoleId(roleId)
	for _, id := range ids1 {
		addID(id)
	}

	ch, _ := model.ChannelGet(channelId)
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
			addID(otherId)
		}
	} else {
		// 获取子频道的授权
		if ch.RootId != "" {
			roleId := fmt.Sprintf("ch-%s-%s", ch.RootId, "bot")
			ids2, _ := model.UserRoleMappingUserIdListByRoleId(roleId)
			for _, id := range ids2 {
				addID(id)
			}
		}
	}

	if bindings, err := model.BotChannelBindingsByChannelID(channelId); err == nil {
		for _, binding := range bindings {
			if !binding.Enabled {
				continue
			}
			profile, err := model.BotProfileGet(binding.BotID)
			if err != nil || profile == nil {
				continue
			}
			addID(profile.UserID)
		}
	}

	return ids
}
