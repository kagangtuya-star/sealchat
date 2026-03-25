package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

func BotTokenList(c *fiber.Ctx) error {
	// page := c.QueryInt("page", 1)
	// pageSize := c.QueryInt("pageSize", 20)
	db := model.GetDB()

	var total int64
	db.Model(&model.BotTokenModel{}).Count(&total)

	// 获取列表
	var items []model.BotTokenModel
	// offset := (page - 1) * pageSize
	db.Order("created_at asc").
		// Offset(offset).Limit(pageSize).
		// Preload("User", func(db *gorm.DB) *gorm.DB {
		//	return db.Select("id, username")
		// }).
		Find(&items)

	// 返回JSON响应
	return c.JSON(fiber.Map{
		// "page":     page,
		// "pageSize": pageSize,
		"total": total,
		"items": items,
	})
}

func BotTokenAdd(c *fiber.Ctx) error {
	type RequestBody struct {
		Name      string `json:"name"`
		Avatar    string `json:"avatar"`
		NickColor string `json:"nickColor"`
	}
	var data RequestBody
	if err := c.BodyParser(&data); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"message": "请求参数错误",
		})
	}

	db := model.GetDB()

	uid := utils.NewID()
	// 创建一个永不可能登录的用户
	nickColor := model.ChannelIdentityNormalizeColor(data.NickColor)

	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID: uid,
		},
		Username:  utils.NewID(),
		Nickname:  data.Name,
		Password:  "",
		Salt:      "BOT_SALT",
		IsBot:     true,
		BotKind:   model.BotKindManual,
		Avatar:    data.Avatar,
		NickColor: nickColor,
	}

	if err := db.Create(user).Error; err != nil {
		return err
	}

	item := &model.BotTokenModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID: uid,
		},
		Name:      data.Name,
		Avatar:    data.Avatar,
		NickColor: nickColor,
		Token:     utils.NewIDWithLength(32),
		ExpiresAt: time.Now().UnixMilli() + 3*365*24*60*60*1e3, // 3 years
	}

	err := db.Create(item).Error
	if err != nil {
		return err
	}

	if err := service.SyncBotUserProfile(item); err != nil {
		return err
	}
	_ = service.SyncBotMembers(item)

	return c.JSON(item)
}

func BotTokenUpdate(c *fiber.Ctx) error {
	type RequestBody struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Avatar    string `json:"avatar"`
		NickColor string `json:"nickColor"`
	}
	var data RequestBody
	if err := c.BodyParser(&data); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	if strings.TrimSpace(data.ID) == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少机器人ID"})
	}
	db := model.GetDB()
	var token model.BotTokenModel
	if err := db.Where("id = ?", data.ID).Limit(1).Find(&token).Error; err != nil {
		return err
	}
	if token.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "机器人令牌不存在"})
	}

	nickColor := model.ChannelIdentityNormalizeColor(data.NickColor)
	update := map[string]any{}
	if strings.TrimSpace(data.Name) != "" {
		update["name"] = data.Name
		token.Name = data.Name
	}
	update["avatar"] = strings.TrimSpace(data.Avatar)
	update["nick_color"] = nickColor
	token.Avatar = strings.TrimSpace(data.Avatar)
	token.NickColor = nickColor

	if err := db.Model(&model.BotTokenModel{}).Where("id = ?", data.ID).Updates(update).Error; err != nil {
		return err
	}
	if err := service.SyncBotUserProfile(&token); err != nil {
		return err
	}
	syncConnectedUserProfile(token.ID, token.Name, token.Avatar, token.NickColor)
	_ = service.SyncBotMembers(&token)
	syncResult, err := service.SyncBotChannelAppearance(&token)
	if err != nil {
		return err
	}

	updatedIdentities := make([]map[string]any, 0, len(syncResult.UpdatedIdentities))
	for _, identity := range syncResult.UpdatedIdentities {
		updatedIdentities = append(updatedIdentities, service.ChannelIdentitySerialize(identity))
	}

	return c.JSON(struct {
		model.BotTokenModel
		UpdatedIdentities []map[string]any `json:"updatedIdentities"`
	}{
		BotTokenModel:     token,
		UpdatedIdentities: updatedIdentities,
	})
}

func BotTokenDelete(c *fiber.Ctx) error {
	tokenID := strings.TrimSpace(c.Query("id"))
	if tokenID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少机器人ID"})
	}

	db := model.GetDB()
	var token model.BotTokenModel
	if err := db.Where("id = ?", tokenID).Limit(1).Find(&token).Error; err != nil {
		return err
	}
	if token.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "机器人令牌不存在"})
	}
	isInternalBot := false
	if user := model.UserGet(token.ID); user != nil && model.IsInternalBotKind(user.BotKind) {
		isInternalBot = true
	} else if ok, err := model.IsInternalBotUser(token.ID); err != nil {
		return err
	} else {
		isInternalBot = ok
	}
	if isInternalBot {
		refCount, err := model.ActiveSystemBotReferenceCount(token.ID)
		if err != nil {
			return err
		}
		if refCount > 0 {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "系统 BOT 仍被 active integration 引用，请先撤销对应授权"})
		}
		if _, err := model.CleanupOrphanSystemBotByUserID(token.ID); err != nil {
			return err
		}
		return c.JSON(fiber.Map{
			"message": "删除成功",
		})
	}

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	rollback := func(err error) error {
		tx.Rollback()
		return err
	}

	if err := tx.Where("user_id = ?", token.ID).Delete(&model.UserRoleMappingModel{}).Error; err != nil {
		return rollback(err)
	}
	if err := tx.Where("user_id = ?", token.ID).Delete(&model.MemberModel{}).Error; err != nil {
		return rollback(err)
	}

	// 删除与该 Bot 相关的私聊频道和好友关系
	var friendChannelIDs []string
	tx.Model(&model.FriendModel{}).
		Where("user_id1 = ? OR user_id2 = ?", token.ID, token.ID).
		Pluck("id", &friendChannelIDs)
	if len(friendChannelIDs) > 0 {
		if err := tx.Where("id IN ?", friendChannelIDs).Delete(&model.ChannelModel{}).Error; err != nil {
			return rollback(err)
		}
	}
	if err := tx.Where("user_id1 = ? OR user_id2 = ?", token.ID, token.ID).Delete(&model.FriendModel{}).Error; err != nil {
		return rollback(err)
	}

	if err := tx.Where("id = ?", token.ID).Delete(&model.UserModel{}).Error; err != nil {
		return rollback(err)
	}
	if err := tx.Where("id = ?", tokenID).Delete(&model.BotTokenModel{}).Error; err != nil {
		return rollback(err)
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"message": "删除成功",
	})
}
