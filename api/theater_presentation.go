package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

func WorldTheaterPresentationTemplateSet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	var template protocol.WorldTheaterPresentationTemplate
	if err := c.BodyParser(&template); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "模板参数错误"})
	}
	world, err := service.WorldTheaterPresentationTemplateSet(c.Params("worldId"), user.ID, template)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrWorldNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "世界不存在"})
		case errors.Is(err, service.ErrWorldPermission):
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权设置世界演出外观模板"})
		default:
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}
	broadcastWorldUpdated(world)
	return c.JSON(fiber.Map{"world": world})
}

type theaterPresentationResolveActor struct {
	IdentityID string `json:"identityId"`
	VariantID  string `json:"variantId"`
}

type theaterPresentationResolveRequest struct {
	Actors []theaterPresentationResolveActor `json:"actors"`
}

type worldTheaterPresentationResolveActor struct {
	ChannelID  string `json:"channelId"`
	IdentityID string `json:"identityId"`
	VariantID  string `json:"variantId"`
}

type worldTheaterPresentationResolveRequest struct {
	Actors []worldTheaterPresentationResolveActor `json:"actors"`
}

func theaterPresentationRevision(value protocol.TheaterPresentation) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// TheaterPresentationsResolve exposes resolved theater data only.
func TheaterPresentationsResolve(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "未登录"})
	}
	channelID := strings.TrimSpace(c.Params("channelId"))
	if channelID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "缺少频道ID"})
	}
	if err := ensureChannelMembership(user.ID, channelID); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "无权访问当前频道"})
	}
	var request theaterPresentationResolveRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if len(request.Actors) > 128 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "一次最多解析128个角色"})
	}
	ids := make([]string, 0, len(request.Actors))
	seen := make(map[string]struct{}, len(request.Actors))
	for _, actor := range request.Actors {
		id := strings.TrimSpace(actor.IdentityID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return c.JSON(fiber.Map{"items": []fiber.Map{}})
	}
	var identities []*model.ChannelIdentityModel
	if err := model.GetDB().Where("channel_id = ? AND id IN ?", channelID, ids).Find(&identities).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	identityByID := make(map[string]*model.ChannelIdentityModel, len(identities))
	for _, identity := range identities {
		identityByID[identity.ID] = identity
	}
	variantIDs := make([]string, 0, len(request.Actors))
	for _, actor := range request.Actors {
		if id := strings.TrimSpace(actor.VariantID); id != "" {
			variantIDs = append(variantIDs, id)
		}
	}
	var variants []*model.ChannelIdentityVariantModel
	if len(variantIDs) > 0 {
		if err := model.GetDB().Where("channel_id = ? AND id IN ?", channelID, variantIDs).Find(&variants).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}
	variantByID := make(map[string]*model.ChannelIdentityVariantModel, len(variants))
	for _, variant := range variants {
		variantByID[variant.ID] = variant
	}
	items := make([]fiber.Map, 0, len(request.Actors))
	for _, actor := range request.Actors {
		identityID := strings.TrimSpace(actor.IdentityID)
		identity := identityByID[identityID]
		if identity == nil {
			continue
		}
		variantID := strings.TrimSpace(actor.VariantID)
		variant := variantByID[variantID]
		if variant != nil && (variant.IdentityID != identity.ID || !variant.Enabled) {
			variant = nil
		}
		appearance := resolvePublicTheaterAppearance(identity, variant)
		item := fiber.Map{"identityId": identity.ID, "variantId": nil, "revision": "", "presentation": nil}
		if variant != nil {
			item["variantId"] = variant.ID
		}
		if appearance != nil {
			item["revision"] = theaterPresentationRevision(*appearance)
			item["presentation"] = appearance
		}
		items = append(items, item)
	}
	return c.JSON(fiber.Map{"items": items})
}

// WorldTheaterPresentationsResolve resolves actors from multiple channels in
// one world Theater request. Source channel remains explicit because channel
// identities are not globally shared records.
func WorldTheaterPresentationsResolve(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "未登录"})
	}
	worldID := strings.TrimSpace(c.Params("worldId"))
	if worldID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "缺少世界ID"})
	}
	if !service.IsWorldMember(worldID, user.ID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "无权访问当前世界"})
	}
	var request worldTheaterPresentationResolveRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "请求参数解析失败"})
	}
	if len(request.Actors) > 128 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "一次最多解析128个角色"})
	}
	channelIDs := make([]string, 0, len(request.Actors))
	identityIDs := make([]string, 0, len(request.Actors))
	variantIDs := make([]string, 0, len(request.Actors))
	seenChannels := map[string]struct{}{}
	seenIdentities := map[string]struct{}{}
	seenVariants := map[string]struct{}{}
	for _, actor := range request.Actors {
		channelID := strings.TrimSpace(actor.ChannelID)
		identityID := strings.TrimSpace(actor.IdentityID)
		variantID := strings.TrimSpace(actor.VariantID)
		if channelID != "" {
			if _, ok := seenChannels[channelID]; !ok {
				seenChannels[channelID] = struct{}{}
				channelIDs = append(channelIDs, channelID)
			}
		}
		if identityID != "" {
			if _, ok := seenIdentities[identityID]; !ok {
				seenIdentities[identityID] = struct{}{}
				identityIDs = append(identityIDs, identityID)
			}
		}
		if variantID != "" {
			if _, ok := seenVariants[variantID]; !ok {
				seenVariants[variantID] = struct{}{}
				variantIDs = append(variantIDs, variantID)
			}
		}
	}
	allowedChannels := map[string]struct{}{}
	if len(channelIDs) > 0 {
		var channels []model.ChannelModel
		if err := model.GetDB().Where("world_id = ? AND id IN ? AND status = ?", worldID, channelIDs, model.ChannelStatusActive).Find(&channels).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		for _, channel := range channels {
			if ensureChannelMembership(user.ID, channel.ID) == nil {
				allowedChannels[channel.ID] = struct{}{}
			}
		}
	}
	identityByScope := map[string]*model.ChannelIdentityModel{}
	if len(allowedChannels) > 0 && len(identityIDs) > 0 {
		allowedIDs := make([]string, 0, len(allowedChannels))
		for id := range allowedChannels {
			allowedIDs = append(allowedIDs, id)
		}
		var identities []*model.ChannelIdentityModel
		if err := model.GetDB().Where("channel_id IN ? AND id IN ?", allowedIDs, identityIDs).Find(&identities).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		for _, identity := range identities {
			identityByScope[identity.ChannelID+"\x00"+identity.ID] = identity
		}
	}
	variantByScope := map[string]*model.ChannelIdentityVariantModel{}
	if len(allowedChannels) > 0 && len(variantIDs) > 0 {
		allowedIDs := make([]string, 0, len(allowedChannels))
		for id := range allowedChannels {
			allowedIDs = append(allowedIDs, id)
		}
		var variants []*model.ChannelIdentityVariantModel
		if err := model.GetDB().Where("channel_id IN ? AND id IN ?", allowedIDs, variantIDs).Find(&variants).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		for _, variant := range variants {
			variantByScope[variant.ChannelID+"\x00"+variant.ID] = variant
		}
	}
	items := make([]fiber.Map, 0, len(request.Actors))
	for _, actor := range request.Actors {
		channelID := strings.TrimSpace(actor.ChannelID)
		identityID := strings.TrimSpace(actor.IdentityID)
		if channelID == "" || identityID == "" {
			continue
		}
		if _, ok := allowedChannels[channelID]; !ok {
			continue
		}
		identity := identityByScope[channelID+"\x00"+identityID]
		if identity == nil {
			continue
		}
		variantID := strings.TrimSpace(actor.VariantID)
		variant := variantByScope[channelID+"\x00"+variantID]
		if variant != nil && (variant.IdentityID != identity.ID || !variant.Enabled) {
			variant = nil
		}
		appearance := resolvePublicTheaterAppearance(identity, variant)
		item := fiber.Map{"sourceChannelId": channelID, "identityId": identity.ID, "requestedVariantId": nil, "variantId": nil, "revision": "", "presentation": nil}
		if variantID != "" {
			item["requestedVariantId"] = variantID
		}
		if variant != nil {
			item["variantId"] = variant.ID
		}
		if appearance != nil {
			item["revision"] = theaterPresentationRevision(*appearance)
			item["presentation"] = appearance
		}
		items = append(items, item)
	}
	return c.JSON(fiber.Map{"items": items})
}

func resolvePublicTheaterAppearance(identity *model.ChannelIdentityModel, variant *model.ChannelIdentityVariantModel) *protocol.TheaterPresentation {
	if identity == nil {
		return nil
	}
	if variant == nil {
		return identity.TheaterPresentation
	}
	var document struct {
		TheaterPresentation json.RawMessage `json:"theaterPresentation"`
	}
	if strings.TrimSpace(variant.AppearanceJSON) == "" || json.Unmarshal([]byte(variant.AppearanceJSON), &document) != nil || len(document.TheaterPresentation) == 0 {
		return identity.TheaterPresentation
	}
	if strings.TrimSpace(string(document.TheaterPresentation)) == "null" {
		return nil
	}
	var patch protocol.TheaterPresentationPatch
	if err := json.Unmarshal(document.TheaterPresentation, &patch); err != nil {
		return identity.TheaterPresentation
	}
	base := protocol.DefaultTheaterPresentation()
	if identity.TheaterPresentation != nil {
		base = *identity.TheaterPresentation
	}
	resolved := protocol.ResolveTheaterPresentation(base, &patch)
	return &resolved
}
