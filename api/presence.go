package api

import (
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/utils"
)

func buildChannelPresenceSnapshot(channelID string, channelUsersMap *utils.SyncMap[string, *utils.SyncSet[string]], userConnMap *utils.SyncMap[string, *utils.SyncMap[*WsSyncConn, *ConnInfo]]) []*protocol.ChannelPresence {
	if channelID == "" || channelUsersMap == nil || userConnMap == nil {
		return []*protocol.ChannelPresence{}
	}
	set, ok := channelUsersMap.Load(channelID)
	if !ok || set == nil {
		return []*protocol.ChannelPresence{}
	}

	results := []*protocol.ChannelPresence{}
	set.Range(func(userID string) bool {
		connSet, ok := userConnMap.Load(userID)
		if !ok || connSet == nil {
			return true
		}
		var active *ConnInfo
		connSet.Range(func(_ *WsSyncConn, info *ConnInfo) bool {
			if info == nil {
				return true
			}
			if info.ChannelId != channelID {
				return true
			}
			if active == nil || info.LastPingTime > active.LastPingTime {
				active = info
			}
			return true
		})
		if active == nil {
			return true
		}
		user := model.UserGet(userID)
		if user == nil {
			return true
		}
		latency := active.LatencyMs
		const maxReasonableLatencyMs int64 = 60_000
		if latency < 0 || latency > maxReasonableLatencyMs {
			latency = 0
		}
		results = append(results, &protocol.ChannelPresence{
			User:     user.ToProtocolType(),
			Latency:  latency,
			Focused:  active.Focused,
			LastSeen: active.LastPingTime,
		})
		return true
	})

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Focused != results[j].Focused {
			return results[i].Focused
		}
		return results[i].Latency < results[j].Latency
	})

	return results
}

func (ctx *ChatContext) BroadcastChannelPresence(channelID string) {
	if ctx == nil || ctx.UserId2ConnInfo == nil || ctx.ChannelUsersMap == nil || channelID == "" {
		return
	}
	now := time.Now().UnixMilli()
	presence := buildChannelPresenceSnapshot(channelID, ctx.ChannelUsersMap, ctx.UserId2ConnInfo)
	event := &protocol.Event{
		Type:     protocol.EventChannelPresenceUpdated,
		Timestamp: now,
		Channel:  &protocol.Channel{ID: channelID},
		Presence: presence,
	}
	ctx.BroadcastEventInChannel(channelID, event)
}

func ChannelPresence(c *fiber.Ctx) error {
	channelID := c.Query("channel_id")
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "缺少 channel_id"})
	}

	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "未认证"})
	}

	if len(channelID) < 30 {
		if !pm.CanWithChannelRole(user.ID, channelID, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
			return nil
		}
	} else {
		fr, _ := model.FriendRelationGetByID(channelID)
		if fr.ID == "" || (fr.UserID1 != user.ID && fr.UserID2 != user.ID) {
			return nil
		}
	}

	snapshot := buildChannelPresenceSnapshot(channelID, getChannelUsersMap(), getUserConnInfoMap())
	return c.JSON(fiber.Map{
		"data":       snapshot,
		"updated_at": time.Now().UnixMilli(),
	})
}
