package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

type worldExternalGlossaryEventPayload struct {
	WorldID     string   `json:"worldId"`
	LibraryIDs  []string `json:"libraryIds,omitempty"`
	Operation   string   `json:"operation"`
	RequestID   string   `json:"requestId,omitempty"`
	ForceReload bool     `json:"forceReload,omitempty"`
}

func normalizeStringIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func broadcastWorldExternalGlossaryEvent(payload *worldExternalGlossaryEventPayload) {
	if payload == nil || strings.TrimSpace(payload.WorldID) == "" {
		return
	}
	now := time.Now().UnixMilli()
	options := map[string]any{
		"worldId":    payload.WorldID,
		"libraryIds": normalizeStringIDs(payload.LibraryIDs),
		"operation":  payload.Operation,
		"version":    now,
		"revision":   now,
	}
	if payload.RequestID != "" {
		options["requestId"] = payload.RequestID
	}
	if payload.ForceReload {
		options["forceReload"] = true
	}
	event := &protocol.Event{
		Type: protocol.EventWorldExternalGlossariesUpdated,
		Argv: &protocol.Argv{Options: options},
	}
	broadcastEventToWorld(payload.WorldID, event)
}

func WorldExternalGlossaryListHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	items, err := service.WorldExternalGlossaryList(worldID, user.ID)
	if err != nil {
		status := fiber.StatusInternalServerError
		switch err {
		case service.ErrWorldPermission:
			status = fiber.StatusForbidden
		case service.ErrWorldNotFound:
			status = fiber.StatusNotFound
		default:
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"items": items, "total": len(items)})
}

func WorldExternalGlossaryEnableHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	libraryID := c.Params("libraryId")
	if err := service.WorldExternalGlossaryEnable(worldID, libraryID, user.ID); err != nil {
		status := fiber.StatusInternalServerError
		switch err {
		case service.ErrWorldPermission, service.ErrExternalGlossaryPermission:
			status = fiber.StatusForbidden
		case service.ErrWorldNotFound, service.ErrExternalGlossaryNotFound:
			status = fiber.StatusNotFound
		default:
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	requestID := utils.NewID()
	broadcastWorldExternalGlossaryEvent(&worldExternalGlossaryEventPayload{
		WorldID:     worldID,
		LibraryIDs:  []string{libraryID},
		Operation:   "enabled",
		RequestID:   requestID,
		ForceReload: true,
	})
	return c.Status(http.StatusCreated).JSON(fiber.Map{"requestId": requestID})
}

func WorldExternalGlossaryDisableHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	libraryID := c.Params("libraryId")
	if err := service.WorldExternalGlossaryDisable(worldID, libraryID, user.ID); err != nil {
		status := fiber.StatusInternalServerError
		switch err {
		case service.ErrWorldPermission:
			status = fiber.StatusForbidden
		default:
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	requestID := utils.NewID()
	broadcastWorldExternalGlossaryEvent(&worldExternalGlossaryEventPayload{
		WorldID:     worldID,
		LibraryIDs:  []string{libraryID},
		Operation:   "disabled",
		RequestID:   requestID,
		ForceReload: true,
	})
	return c.JSON(fiber.Map{"requestId": requestID})
}

func WorldExternalGlossaryBulkEnableHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var payload struct {
		LibraryIDs []string `json:"libraryIds"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	cleaned := normalizeStringIDs(payload.LibraryIDs)
	count, err := service.WorldExternalGlossaryBulkEnable(worldID, cleaned, user.ID)
	if err != nil {
		status := fiber.StatusInternalServerError
		switch err {
		case service.ErrWorldPermission, service.ErrExternalGlossaryPermission:
			status = fiber.StatusForbidden
		case service.ErrWorldNotFound, service.ErrExternalGlossaryNotFound:
			status = fiber.StatusNotFound
		default:
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	requestID := utils.NewID()
	if count > 0 {
		broadcastWorldExternalGlossaryEvent(&worldExternalGlossaryEventPayload{
			WorldID:     worldID,
			LibraryIDs:  cleaned,
			Operation:   "bulk-enabled",
			RequestID:   requestID,
			ForceReload: true,
		})
	}
	return c.JSON(fiber.Map{"updated": count, "requestId": requestID})
}

func WorldExternalGlossaryBulkDisableHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var payload struct {
		LibraryIDs []string `json:"libraryIds"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	cleaned := normalizeStringIDs(payload.LibraryIDs)
	count, err := service.WorldExternalGlossaryBulkDisable(worldID, cleaned, user.ID)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err == service.ErrWorldPermission {
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	requestID := utils.NewID()
	if count > 0 {
		broadcastWorldExternalGlossaryEvent(&worldExternalGlossaryEventPayload{
			WorldID:     worldID,
			LibraryIDs:  cleaned,
			Operation:   "bulk-disabled",
			RequestID:   requestID,
			ForceReload: true,
		})
	}
	return c.JSON(fiber.Map{"updated": count, "requestId": requestID})
}

func listBoundWorldIDsByLibraryIDs(libraryIDs []string) ([]string, error) {
	cleaned := normalizeStringIDs(libraryIDs)
	if len(cleaned) == 0 {
		return nil, nil
	}
	var bindings []model.WorldExternalGlossaryBindingModel
	if err := model.GetDB().
		Where("library_id IN ?", cleaned).
		Find(&bindings).Error; err != nil {
		return nil, err
	}
	worldIDs := make([]string, 0, len(bindings))
	seen := map[string]struct{}{}
	for _, binding := range bindings {
		worldID := strings.TrimSpace(binding.WorldID)
		if worldID == "" {
			continue
		}
		if _, ok := seen[worldID]; ok {
			continue
		}
		seen[worldID] = struct{}{}
		worldIDs = append(worldIDs, worldID)
	}
	return worldIDs, nil
}

func broadcastWorldExternalGlossaryEventToBoundWorlds(libraryIDs []string, operation, requestID string, forceReload bool) {
	worldIDs, err := listBoundWorldIDsByLibraryIDs(libraryIDs)
	if err != nil || len(worldIDs) == 0 {
		return
	}
	for _, worldID := range worldIDs {
		broadcastWorldExternalGlossaryEvent(&worldExternalGlossaryEventPayload{
			WorldID:     worldID,
			LibraryIDs:  libraryIDs,
			Operation:   operation,
			RequestID:   requestID,
			ForceReload: forceReload,
		})
	}
}
