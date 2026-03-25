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

type adminBotTokenDTO struct {
	model.BotTokenModel
	BotKind              string                           `json:"botKind,omitempty"`
	IsSystemManaged      bool                             `json:"isSystemManaged"`
	ActiveReferenceCount int64                            `json:"activeReferenceCount"`
	ActiveReferences     []model.SystemBotActiveReference `json:"activeReferences,omitempty"`
	UserNickname         string                           `json:"userNickname,omitempty"`
}

func normalizeBotTokenScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case "system":
		return "system"
	case "all":
		return "all"
	default:
		return "manual"
	}
}

func matchesBotTokenScope(scope string, isSystemManaged bool) bool {
	switch normalizeBotTokenScope(scope) {
	case "system":
		return isSystemManaged
	case "all":
		return true
	default:
		return !isSystemManaged
	}
}

func matchesBotTokenKeyword(token model.BotTokenModel, user *model.UserModel, botKind, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return true
	}
	fields := []string{
		token.ID,
		token.Name,
		token.Avatar,
		token.NickColor,
		botKind,
	}
	if user != nil {
		fields = append(fields, user.Username, user.Nickname)
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(strings.TrimSpace(field)), keyword) {
			return true
		}
	}
	return false
}

func buildAdminBotTokenList(keyword, scope string) ([]adminBotTokenDTO, error) {
	db := model.GetDB()
	var tokens []model.BotTokenModel
	if err := db.Order("created_at DESC").Find(&tokens).Error; err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return []adminBotTokenDTO{}, nil
	}

	ids := make([]string, 0, len(tokens))
	for _, item := range tokens {
		if strings.TrimSpace(item.ID) != "" {
			ids = append(ids, item.ID)
		}
	}

	userByID := map[string]*model.UserModel{}
	if len(ids) > 0 {
		var users []model.UserModel
		if err := db.Where("id IN ?", ids).Find(&users).Error; err != nil {
			return nil, err
		}
		for i := range users {
			user := users[i]
			userByID[user.ID] = &user
		}
	}

	internalSet, err := model.InternalBotUserIDSet(ids)
	if err != nil {
		return nil, err
	}

	items := make([]adminBotTokenDTO, 0, len(tokens))
	for _, token := range tokens {
		user := userByID[token.ID]
		botKind := ""
		if user != nil {
			botKind = strings.TrimSpace(user.BotKind)
		}
		_, relationMatched := internalSet[token.ID]
		isSystemManaged := model.IsInternalBotKind(botKind) || relationMatched
		if !matchesBotTokenScope(scope, isSystemManaged) {
			continue
		}
		if !matchesBotTokenKeyword(token, user, botKind, keyword) {
			continue
		}
		activeReferenceCount := int64(0)
		activeReferences := []model.SystemBotActiveReference{}
		if isSystemManaged {
			activeReferences, err = model.ActiveSystemBotReferences(token.ID)
			if err != nil {
				return nil, err
			}
			activeReferenceCount = int64(len(activeReferences))
		}
		item := adminBotTokenDTO{
			BotTokenModel:        token,
			BotKind:              botKind,
			IsSystemManaged:      isSystemManaged,
			ActiveReferenceCount: activeReferenceCount,
			ActiveReferences:     activeReferences,
			UserNickname:         "",
		}
		if user != nil {
			item.UserNickname = user.Nickname
		}
		items = append(items, item)
	}
	return items, nil
}

func deleteBotTokenByID(tokenID string) error {
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return fiber.NewError(http.StatusBadRequest, "缺少机器人ID")
	}

	db := model.GetDB()
	var token model.BotTokenModel
	if err := db.Where("id = ?", tokenID).Limit(1).Find(&token).Error; err != nil {
		return err
	}
	if token.ID == "" {
		return fiber.NewError(http.StatusNotFound, "机器人令牌不存在")
	}
	isInternalBot := false
	if user := model.UserGet(token.ID); user != nil && user.ID != "" && model.IsInternalBotKind(user.BotKind) {
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
			return fiber.NewError(http.StatusBadRequest, "系统 BOT 仍被 active integration 引用，请先撤销对应授权")
		}
		_, err = model.CleanupOrphanSystemBotByUserID(token.ID)
		return err
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
	if err := tx.Where("user_id = ?", token.ID).Delete(&model.WorldMemberModel{}).Error; err != nil {
		return rollback(err)
	}

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
	return nil
}

func BotTokenList(c *fiber.Ctx) error {
	items, err := buildAdminBotTokenList(c.Query("keyword"), c.Query("scope"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"total": len(items),
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
	if err := deleteBotTokenByID(tokenID); err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"message": "删除成功",
	})
}

func BotTokenBatchDelete(c *fiber.Ctx) error {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(body.IDs))
	for _, id := range body.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少机器人ID"})
	}
	type failedItem struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	deletedIDs := make([]string, 0, len(ids))
	failed := make([]failedItem, 0)
	for _, id := range ids {
		if err := deleteBotTokenByID(id); err != nil {
			msg := "删除失败"
			if ferr, ok := err.(*fiber.Error); ok && strings.TrimSpace(ferr.Message) != "" {
				msg = ferr.Message
			} else if strings.TrimSpace(err.Error()) != "" {
				msg = err.Error()
			}
			failed = append(failed, failedItem{ID: id, Message: msg})
			continue
		}
		deletedIDs = append(deletedIDs, id)
	}
	return c.JSON(fiber.Map{
		"deletedCount": len(deletedIDs),
		"deletedIds":   deletedIDs,
		"failedCount":  len(failed),
		"failedItems":  failed,
	})
}
