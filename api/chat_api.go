package api

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
)

func apiWrap[T any, T2 any](ctx *ChatContext, msg []byte, solve func(ctx *ChatContext, data T) (T2, error)) {
	c := ctx.Conn

	var data struct {
		Data T `json:"data"`
	}

	err := json.Unmarshal(msg, &data)
	if err != nil {
		return
	}

	ret, err := solve(ctx, data.Data)
	if err != nil {
		errMsg := err.Error()
		_ = c.WriteJSON(&struct {
			Echo string `json:"echo"`
			Err  string `json:"err"`
			Data any    `json:"data"`
		}{ctx.Echo, errMsg, ret})
		return
	}

	_ = c.WriteJSON(&struct {
		Echo string `json:"echo"`
		Data any    `json:"data"`
	}{ctx.Echo, ret})
}

func apiUserListCommon(dataNext string, f func(q *gorm.DB)) (any, error) {
	db := model.GetDB()

	var items []*model.UserModel
	q := db.Select("nickname, is_bot, avatar, id")

	if f != nil {
		f(q)
	}

	var count int64
	if dataNext != "" {
		t, err := strconv.ParseInt(dataNext, 36, 64)
		if err != nil {
			return nil, err
		}

		q = q.Where("created_at < ?", time.UnixMilli(t))
	}

	q.Order("created_at desc").Find(&items)

	q.Count(&count)
	var next string

	items = lo.Reverse(items)
	if count > int64(len(items)) && len(items) > 0 {
		next = strconv.FormatInt(items[0].CreatedAt.UnixMilli(), 36)
	}

	return &struct {
		Data []*model.UserModel `json:"data"`
		Next string             `json:"next"`
	}{
		Data: items,
		Next: next,
	}, nil
}

func apiGuildMemberList(ctx *ChatContext, data *struct {
	GuildId string `json:"guild_id"`
	Next    string `json:"next"`
}) (any, error) {
	return apiUserListCommon(data.Next, nil)
}

func apiBotInfoSetName(ctx *ChatContext, msg []byte) {
	data := struct {
		Data struct {
			Name  string `json:"name"`
			Brief string `json:"brief"`
		} `json:"data"`
	}{}
	err := json.Unmarshal(msg, &data)
	if err != nil {
		return
	}

	name := strings.TrimSpace(data.Data.Name)
	if ctx.User.IsBot {
		if token, err := model.BotTokenGet(ctx.User.ID); err == nil && token != nil {
			if trimmed := strings.TrimSpace(token.Name); trimmed != "" {
				name = trimmed
			}
		}
	}
	if name == "" {
		name = ctx.User.Nickname
	}
	data.Data.Name = name

	ctx.User.Nickname = name
	ctx.User.Brief = data.Data.Brief
	ctx.User.SaveInfo()
	for _, i := range ctx.Members {
		i.Nickname = name
		i.SaveInfo()
		// 广播事件，名字更新了
		ctx.BroadcastEventInChannel(i.ChannelID, &protocol.Event{
			Type:   "channel-member-updated",
			Member: i.ToProtocolType(),
		})
	}

	ret := struct {
		Echo string `json:"echo"`
	}{
		Echo: ctx.Echo,
	}
	_ = ctx.Conn.WriteJSON(ret)
}

func apiBotCommandRegister(ctx *ChatContext, msg []byte) {
	data := struct {
		Data map[string]string `json:"data"`
	}{}
	err := json.Unmarshal(msg, &data)
	if err != nil {
		return
	}

	commandTips.Store(ctx.User.ID, data.Data)

	ret := struct {
		Echo string `json:"echo"`
	}{
		Echo: ctx.Echo,
	}
	_ = ctx.Conn.WriteJSON(ret)
}

func apiBotChannelMemberSetName(ctx *ChatContext, msg []byte) {
	data := struct {
		Data struct {
			Name      string `json:"name"`
			ChannelId string `json:"channel_id"`
			UserId    string `json:"user_id"`
			// Brief string `json:"brief"`
		} `json:"data"`
	}{}
	err := json.Unmarshal(msg, &data)
	if err != nil {
		return
	}

	member, err := model.MemberGetByUserIDAndChannelIDBase(data.Data.UserId, data.Data.ChannelId, data.Data.Name, false)
	if member != nil {
		member.Nickname = data.Data.Name
		member.SaveInfo()

		// 广播事件，名字更新了
		ctx.BroadcastEventInChannel(data.Data.ChannelId, &protocol.Event{
			Type:   "channel-member-updated",
			Member: member.ToProtocolType(),
		})
	}

	ret := struct {
		Echo string `json:"echo"`
	}{
		Echo: ctx.Echo,
	}
	_ = ctx.Conn.WriteJSON(ret)
}
