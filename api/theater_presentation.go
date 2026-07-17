package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
)

type theaterPresentationResolveActor struct {
	IdentityID string `json:"identityId"`
	VariantID  string `json:"variantId"`
}

type theaterPresentationResolveRequest struct {
	Actors []theaterPresentationResolveActor `json:"actors"`
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
