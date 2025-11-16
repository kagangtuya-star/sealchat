package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

func ensureWorldMemberAccess(user *model.UserModel, worldID string) error {
	if user == nil {
		return fiber.ErrUnauthorized
	}
	if strings.TrimSpace(worldID) == "" {
		return fiber.ErrBadRequest
	}
	if pm.CanWithSystemRole(user.ID, pm.PermFuncWorldManage) {
		return nil
	}
	if err := service.EnsureWorldMemberActive(worldID, user.ID); err != nil {
		if errors.Is(err, service.ErrWorldMemberRequired) || errors.Is(err, service.ErrWorldJoinApproval) {
			return fiber.ErrForbidden
		}
		return err
	}
	return nil
}

func ensureWorldManageAccess(user *model.UserModel, worldID string) (*model.WorldModel, error) {
	if user == nil {
		return nil, fiber.ErrUnauthorized
	}
	if strings.TrimSpace(worldID) == "" {
		return nil, fiber.ErrBadRequest
	}
	world, err := model.WorldGet(worldID)
	if err != nil {
		return nil, err
	}
	if world == nil || world.ID == "" {
		return nil, service.ErrWorldNotFound
	}
	if world.OwnerID == user.ID || pm.CanWithSystemRole(user.ID, pm.PermFuncWorldManage) {
		return world, nil
	}
	return nil, fiber.ErrForbidden
}

func ensureWorldOwnerAccess(user *model.UserModel, worldID string) (*model.WorldModel, error) {
	world, err := ensureWorldManageAccess(user, worldID)
	if err != nil {
		return nil, err
	}
	if world.OwnerID != user.ID {
		return nil, fiber.ErrForbidden
	}
	return world, nil
}

func ensureWorldMemberManageAccess(user *model.UserModel, worldID string) (*model.WorldModel, error) {
	world, err := ensureWorldManageAccess(user, worldID)
	if err != nil {
		return nil, err
	}
	if world.OwnerID == user.ID || pm.CanWithSystemRole(user.ID, pm.PermFuncWorldManage) {
		return world, nil
	}
	return nil, fiber.ErrForbidden
}

func WorldList(c *fiber.Ctx) error {
	query := strings.TrimSpace(c.Query("q"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	user := currentUserOrToken(c)
	ownerID := ""
	memberID := ""
	if user != nil {
		ownerID = user.ID
		memberID = user.ID
	}
	worlds, err := service.ListWorlds(service.ListWorldOption{
		Query:    query,
		Limit:    limit,
		Offset:   offset,
		OwnerID:  ownerID,
		MemberID: memberID,
	})
	if err != nil {
		return err
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"data": worlds,
	})
}

func WorldCreate(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	if !pm.CanWithSystemRole(user.ID, pm.PermFuncWorldCreate) {
		return fiber.ErrForbidden
	}
	var body struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Avatar      string                 `json:"avatar"`
		Banner      string                 `json:"banner"`
		Visibility  model.WorldVisibility  `json:"visibility"`
		JoinPolicy  model.WorldJoinPolicy  `json:"joinPolicy"`
		Settings    map[string]interface{} `json:"settings"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.ErrBadRequest
	}
	world, err := service.CreateWorld(service.CreateWorldOptions{
		Name:        body.Name,
		Description: body.Description,
		Avatar:      body.Avatar,
		Banner:      body.Banner,
		OwnerID:     user.ID,
		Visibility:  body.Visibility,
		JoinPolicy:  body.JoinPolicy,
		Settings:    body.Settings,
	})
	if err != nil {
		return err
	}
	if _, err := service.JoinWorld(world.ID, user.ID, user.Nickname); err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"world": world,
	})
}

func WorldUpdate(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	world, err := ensureWorldOwnerAccess(user, c.Params("worldId"))
	if err != nil {
		return err
	}
	var body struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Avatar      string                 `json:"avatar"`
		Banner      string                 `json:"banner"`
		Visibility  model.WorldVisibility  `json:"visibility"`
		JoinPolicy  model.WorldJoinPolicy  `json:"joinPolicy"`
		Settings    map[string]interface{} `json:"settings"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.ErrBadRequest
	}
	updated, err := service.UpdateWorld(world.ID, service.UpdateWorldOptions{
		Name:        body.Name,
		Description: body.Description,
		Avatar:      body.Avatar,
		Banner:      body.Banner,
		Visibility:  body.Visibility,
		JoinPolicy:  body.JoinPolicy,
		Settings:    body.Settings,
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"world": updated})
}

func WorldDelete(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	world, err := ensureWorldOwnerAccess(user, c.Params("worldId"))
	if err != nil {
		return err
	}
	if err := service.DeleteWorld(world.ID); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func WorldDetailBySlug(c *fiber.Ctx) error {
	world, err := service.GetWorldBySlug(c.Params("slug"))
	if err != nil {
		return err
	}
	if world == nil || world.ID == "" {
		return fiber.ErrNotFound
	}
	count, _ := model.WorldMemberCount(world.ID)
	world.MemberCount = count
	if user := currentUserOrToken(c); user != nil {
		if strings.TrimSpace(world.OwnerID) == user.ID {
			world.IsOwner = true
			world.IsMember = true
		} else {
			if member, memErr := model.WorldMemberGet(world.ID, user.ID); memErr == nil && member != nil {
				world.IsMember = true
			}
		}
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{
		"world": world,
	})
}

func WorldChannelList(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	worldID := c.Params("worldId")
	items, err := service.ChannelList(user.ID, worldID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"channels": items,
	})
}

func WorldChannelCreate(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	worldID := c.Params("worldId")
	var payload protocol.Channel
	if err := c.BodyParser(&payload); err != nil {
		return fiber.ErrBadRequest
	}
	if payload.WorldID == "" {
		payload.WorldID = worldID
	}
	channel, err := service.ChannelNew(
		utils.NewID(),
		payload.PermType,
		payload.Name,
		user.ID,
		payload.ParentID,
		payload.WorldID,
	)
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"channel": channel.ToProtocolType(),
	})
}

func WorldInviteCreate(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	worldID := c.Params("worldId")
	if _, err := ensureWorldManageAccess(user, worldID); err != nil {
		return err
	}
	var body struct {
		ChannelID     string `json:"channelId"`
		MaxUses       int    `json:"maxUses"`
		ExpireHours   int    `json:"expireHours"`
		IsSingleUse   bool   `json:"isSingleUse"`
		PreferredSlug string `json:"preferredSlug"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.ErrBadRequest
	}
	var expiredAt *time.Time
	if body.ExpireHours > 0 {
		t := time.Now().Add(time.Duration(body.ExpireHours) * time.Hour)
		expiredAt = &t
	}
	invite, err := service.CreateWorldInvite(service.CreateWorldInviteOptions{
		WorldID:     worldID,
		ChannelID:   body.ChannelID,
		CreatorID:   user.ID,
		ExpiredAt:   expiredAt,
		MaxUses:     body.MaxUses,
		IsSingleUse: body.IsSingleUse,
	})
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"invite": invite,
	})
}

func WorldInviteList(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	worldID := c.Params("worldId")
	if _, err := ensureWorldManageAccess(user, worldID); err != nil {
		return err
	}
	invites, err := service.ListWorldInvites(worldID, 50)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"invites": invites,
	})
}

func WorldMemberList(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	worldID := c.Params("worldId")
	if strings.TrimSpace(worldID) == "" {
		return fiber.ErrBadRequest
	}
	if err := ensureWorldMemberAccess(user, worldID); err != nil {
		return err
	}
	members, err := service.ListWorldMembers(worldID, 200, 0)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"members": members,
	})
}

func WorldMemberRemove(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	worldID := c.Params("worldId")
	if _, err := ensureWorldMemberManageAccess(user, worldID); err != nil {
		return err
	}
	targetUserID := c.Params("userId")
	if strings.TrimSpace(targetUserID) == "" {
		return fiber.ErrBadRequest
	}
	if err := service.RemoveWorldMember(worldID, targetUserID); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func InvitePreview(c *fiber.Ctx) error {
	code := c.Params("code")
	result, err := service.GetWorldInviteSummary(code)
	if err != nil {
		if errors.Is(err, service.ErrWorldInviteInvalid) || errors.Is(err, service.ErrWorldNotFound) {
			return fiber.ErrNotFound
		}
		return err
	}
	return c.JSON(result)
}

func InviteAccept(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	result, err := service.AcceptWorldInvite(service.AcceptWorldInviteOptions{
		Code:      c.Params("code"),
		UserID:    user.ID,
		Nickname:  user.Nickname,
		IPAddress: c.IP(),
		UserAgent: c.Get("User-Agent"),
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"world":  result.World,
		"member": result.Member,
	})
}

func currentUserOrToken(c *fiber.Ctx) *model.UserModel {
	if user := getCurUser(c); user != nil {
		return user
	}
	token := strings.TrimSpace(c.Get("Authorization"))
	if token == "" {
		return nil
	}
	user, err := model.UserVerifyAccessToken(token)
	if err != nil {
		return nil
	}
	return user
}
