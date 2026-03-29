package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

type webhookIntegrationDTO struct {
	ID                string   `json:"id"`
	ChannelID         string   `json:"channelId"`
	Name              string   `json:"name"`
	Source            string   `json:"source"`
	BotUserID         string   `json:"botUserId"`
	Status            string   `json:"status"`
	CreatedAt         int64    `json:"createdAt"`
	CreatedBy         string   `json:"createdBy"`
	LastUsedAt        int64    `json:"lastUsedAt"`
	TokenTailFragment string   `json:"tokenTailFragment"`
	Capabilities      []string `json:"capabilities"`
}

func buildWebhookIntegrationDTO(item *model.ChannelWebhookIntegrationModel) *webhookIntegrationDTO {
	if item == nil {
		return nil
	}
	var createdAt int64
	if !item.CreatedAt.IsZero() {
		createdAt = item.CreatedAt.UnixMilli()
	}
	return &webhookIntegrationDTO{
		ID:                item.ID,
		ChannelID:         item.ChannelID,
		Name:              item.Name,
		Source:            item.Source,
		BotUserID:         item.BotUserID,
		Status:            item.Status,
		CreatedAt:         createdAt,
		CreatedBy:         item.CreatedBy,
		LastUsedAt:        item.LastUsedAt,
		TokenTailFragment: item.TokenTailFragment,
		Capabilities:      item.Capabilities(),
	}
}

func WebhookIntegrationList(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}
	if !CanWithChannelRole(c, channelID, pm.PermFuncChannelManageInfo) {
		return nil
	}
	items, err := model.ChannelWebhookIntegrationList(channelID)
	if err != nil {
		return wrapError(c, err, "读取授权失败")
	}

	// 补 token 末尾片段（仅用于展示，不返回明文）
	botIDs := []string{}
	for _, it := range items {
		if it == nil {
			continue
		}
		if strings.TrimSpace(it.BotUserID) != "" {
			botIDs = append(botIDs, it.BotUserID)
		}
	}
	tailByBotID := map[string]string{}
	if len(botIDs) > 0 {
		var tokens []model.BotTokenModel
		model.GetDB().Select("id, token").Where("id IN ?", botIDs).Find(&tokens)
		for _, t := range tokens {
			trimmed := strings.TrimSpace(t.Token)
			if len(trimmed) >= 6 {
				tailByBotID[t.ID] = trimmed[len(trimmed)-6:]
			} else {
				tailByBotID[t.ID] = trimmed
			}
		}
	}

	var out []*webhookIntegrationDTO
	for _, it := range items {
		dto := buildWebhookIntegrationDTO(it)
		if dto == nil {
			continue
		}
		if tail, ok := tailByBotID[dto.BotUserID]; ok {
			dto.TokenTailFragment = tail
		}
		out = append(out, dto)
	}

	return c.JSON(fiber.Map{
		"items": out,
	})
}

func WebhookIntegrationCreate(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少频道ID"})
	}
	if !CanWithChannelRole(c, channelID, pm.PermFuncChannelManageInfo) {
		return nil
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return wrapError(c, err, "读取频道失败")
	}
	if channel == nil || channel.ID == "" {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "频道不存在"})
	}
	if strings.EqualFold(channel.PermType, "private") {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "私聊频道不支持创建 webhook 授权"})
	}

	var body struct {
		Name         string   `json:"name"`
		Source       string   `json:"source"`
		Capabilities []string `json:"capabilities"`
		// 可选：token 有效期（默认 3 年）
		ExpiresInDays int `json:"expiresInDays"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "请求参数错误"})
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "Webhook"
	}
	source := strings.TrimSpace(body.Source)
	if source == "" {
		source = "external"
	}
	if len(body.Capabilities) == 0 {
		body.Capabilities = []string{"read_changes"}
	}

	uid := utils.NewID()
	nickColor := ""
	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: uid},
		Username:          utils.NewID(),
		Nickname:          name,
		Password:          "",
		Salt:              "BOT_SALT",
		IsBot:             true,
		BotKind:           model.BotKindChannelWebhook,
		Avatar:            "",
		NickColor:         nickColor,
	}

	db := model.GetDB()
	if err := db.Create(user).Error; err != nil {
		return wrapError(c, err, "创建 bot 失败")
	}

	expiresDays := body.ExpiresInDays
	if expiresDays <= 0 {
		expiresDays = 365 * 3
	}
	tokenValue := utils.NewIDWithLength(32)
	token := &model.BotTokenModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: uid},
		Name:              name,
		Avatar:            "",
		NickColor:         nickColor,
		Token:             tokenValue,
		ExpiresAt:         time.Now().UnixMilli() + int64(expiresDays)*24*60*60*1e3,
	}
	if err := db.Create(token).Error; err != nil {
		return wrapError(c, err, "创建 token 失败")
	}
	_ = service.SyncBotUserProfile(token)
	_ = service.SyncBotMembers(token)

	// 确保 bot 在频道内有成员记录，方便昵称与身份体系联动
	_, _ = model.MemberGetByUserIDAndChannelIDBase(user.ID, channelID, name, true)

	createdBy := getCurUser(c).ID
	integration, err := model.ChannelWebhookIntegrationCreate(channelID, name, source, user.ID, createdBy, body.Capabilities)
	if err != nil {
		return wrapError(c, err, "创建 webhook 授权失败")
	}

	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"item":  buildWebhookIntegrationDTO(integration),
		"token": tokenValue, // 仅返回一次
	})
}

func WebhookIntegrationRotate(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	id := strings.TrimSpace(c.Params("id"))
	if channelID == "" || id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少参数"})
	}
	if !CanWithChannelRole(c, channelID, pm.PermFuncChannelManageInfo) {
		return nil
	}
	integration, err := model.ChannelWebhookIntegrationGetByID(channelID, id)
	if err != nil {
		return wrapError(c, err, "读取授权失败")
	}
	if integration == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "授权不存在"})
	}
	if integration.Status != model.WebhookIntegrationStatusActive {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "授权已撤销"})
	}

	newToken := utils.NewIDWithLength(32)
	if err := model.GetDB().Model(&model.BotTokenModel{}).
		Where("id = ?", integration.BotUserID).
		Updates(map[string]any{
			"token":          newToken,
			"expires_at":     time.Now().UnixMilli() + 3*365*24*60*60*1e3,
			"recent_used_at": 0,
		}).Error; err != nil {
		return wrapError(c, err, "轮换 token 失败")
	}

	if len(newToken) >= 6 {
		_ = model.GetDB().Model(&model.ChannelWebhookIntegrationModel{}).
			Where("id = ?", integration.ID).
			Update("token_tail_fragment", newToken[len(newToken)-6:]).Error
	}

	return c.JSON(fiber.Map{
		"token": newToken, // 仅返回一次
	})
}

func WebhookIntegrationRevoke(c *fiber.Ctx) error {
	channelID := strings.TrimSpace(c.Params("channelId"))
	id := strings.TrimSpace(c.Params("id"))
	if channelID == "" || id == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "缺少参数"})
	}
	if !CanWithChannelRole(c, channelID, pm.PermFuncChannelManageInfo) {
		return nil
	}
	integration, err := model.ChannelWebhookIntegrationGetByID(channelID, id)
	if err != nil {
		return wrapError(c, err, "读取授权失败")
	}
	if integration == nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"message": "授权不存在"})
	}

	tx := model.GetDB().Begin()
	if tx.Error != nil {
		return wrapError(c, tx.Error, "撤销授权失败")
	}
	rollback := func(err error) error {
		tx.Rollback()
		return wrapError(c, err, "撤销授权失败")
	}

	if err := tx.Model(&model.ChannelWebhookIntegrationModel{}).
		Where("id = ?", integration.ID).
		Update("status", model.WebhookIntegrationStatusRevoked).Error; err != nil {
		return rollback(err)
	}
	if err := tx.Model(&model.BotTokenModel{}).
		Where("id = ?", integration.BotUserID).
		Updates(map[string]any{"expires_at": int64(0)}).Error; err != nil {
		return rollback(err)
	}
	if _, err := model.CleanupOrphanSystemBotByUserIDTx(tx, integration.BotUserID); err != nil {
		return rollback(err)
	}
	if err := tx.Commit().Error; err != nil {
		return wrapError(c, err, "撤销授权失败")
	}

	return c.JSON(fiber.Map{"success": true})
}
