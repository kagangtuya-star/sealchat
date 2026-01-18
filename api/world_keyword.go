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

func WorldKeywordListHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	page := parseQueryIntDefault(c, "page", 1)
	pageSize := parseQueryIntDefault(c, "pageSize", 50)
	query := strings.TrimSpace(c.Query("q"))
	category := strings.TrimSpace(c.Query("category"))
	includeDisabled := c.QueryBool("includeDisabled")
	items, total, err := service.WorldKeywordList(worldID, user.ID, service.WorldKeywordListOptions{
		Page:            page,
		PageSize:        pageSize,
		Query:           query,
		Category:        category,
		IncludeDisabled: includeDisabled,
	})
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
	return c.JSON(fiber.Map{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func WorldKeywordPublicListHandler(c *fiber.Ctx) error {
	worldID := c.Params("worldId")
	page := parseQueryIntDefault(c, "page", 1)
	pageSize := parseQueryIntDefault(c, "pageSize", 50)
	query := strings.TrimSpace(c.Query("q"))
	category := strings.TrimSpace(c.Query("category"))
	items, total, err := service.WorldKeywordListPublic(worldID, service.WorldKeywordListOptions{
		Page:     page,
		PageSize: pageSize,
		Query:    query,
		Category: category,
	})
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
	return c.JSON(fiber.Map{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func WorldKeywordCreateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var payload service.WorldKeywordInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, err := service.WorldKeywordCreate(worldID, user.ID, payload)
	if err != nil {
		status := fiber.StatusBadRequest
		switch err {
		case service.ErrWorldPermission:
			status = fiber.StatusForbidden
		case service.ErrWorldNotFound:
			status = fiber.StatusNotFound
		default:
			if strings.Contains(err.Error(), "关键词") {
				status = fiber.StatusBadRequest
			} else {
				status = fiber.StatusInternalServerError
			}
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	requestID := utils.NewID()
	broadcastWorldKeywordEvent(&worldKeywordEventPayload{
		WorldID:    worldID,
		Operation:  "created",
		RequestID:  requestID,
		Keywords:   []*model.WorldKeywordModel{item},
		KeywordIDs: []string{item.ID},
	})
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item, "requestId": requestID})
}

func WorldKeywordUpdateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	keywordID := c.Params("keywordId")
	var payload service.WorldKeywordInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, err := service.WorldKeywordUpdate(worldID, keywordID, user.ID, payload)
	if err != nil {
		status := fiber.StatusInternalServerError
		switch err {
		case service.ErrWorldPermission:
			status = fiber.StatusForbidden
		case service.ErrWorldKeywordNotFound:
			status = fiber.StatusNotFound
		default:
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	requestID := utils.NewID()
	broadcastWorldKeywordEvent(&worldKeywordEventPayload{
		WorldID:    worldID,
		Operation:  "updated",
		RequestID:  requestID,
		Keywords:   []*model.WorldKeywordModel{item},
		KeywordIDs: []string{item.ID},
	})
	return c.JSON(fiber.Map{"item": item, "requestId": requestID})
}

func WorldKeywordDeleteHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	keywordID := c.Params("keywordId")
	if err := service.WorldKeywordDelete(worldID, keywordID, user.ID); err != nil {
		status := fiber.StatusInternalServerError
		switch err {
		case service.ErrWorldPermission:
			status = fiber.StatusForbidden
		case service.ErrWorldKeywordNotFound:
			status = fiber.StatusNotFound
		default:
			status = fiber.StatusBadRequest
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	requestID := utils.NewID()
	broadcastWorldKeywordEvent(&worldKeywordEventPayload{
		WorldID:    worldID,
		Operation:  "deleted",
		RequestID:  requestID,
		DeletedIDs: []string{keywordID},
		KeywordIDs: []string{keywordID},
	})
	return c.JSON(fiber.Map{"requestId": requestID})
}

func WorldKeywordBulkDeleteHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	cleaned := make([]string, 0, len(payload.IDs))
	for _, raw := range payload.IDs {
		if trimmed := strings.TrimSpace(raw); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	count, err := service.WorldKeywordBulkDelete(worldID, payload.IDs, user.ID)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err == service.ErrWorldPermission {
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	if count == 0 {
		return c.JSON(fiber.Map{"deleted": count})
	}
	requestID := utils.NewID()
	broadcastWorldKeywordEvent(&worldKeywordEventPayload{
		WorldID:    worldID,
		Operation:  "deleted",
		RequestID:  requestID,
		DeletedIDs: cleaned,
		KeywordIDs: cleaned,
	})
	return c.JSON(fiber.Map{"deleted": count, "requestId": requestID})
}

func WorldKeywordReorderHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var payload struct {
		Items []service.WorldKeywordReorderItem `json:"items"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	count, err := service.WorldKeywordReorder(worldID, user.ID, payload.Items)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err == service.ErrWorldPermission {
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	if count == 0 {
		return c.JSON(fiber.Map{"updated": count})
	}
	requestID := utils.NewID()
	keywordIDs := make([]string, 0, len(payload.Items))
	for _, item := range payload.Items {
		if trimmed := strings.TrimSpace(item.ID); trimmed != "" {
			keywordIDs = append(keywordIDs, trimmed)
		}
	}
	broadcastWorldKeywordEvent(&worldKeywordEventPayload{
		WorldID:     worldID,
		Operation:   "reordered",
		RequestID:   requestID,
		KeywordIDs:  keywordIDs,
		ForceReload: true,
	})
	return c.JSON(fiber.Map{"updated": count, "requestId": requestID})
}

func WorldKeywordImportHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var payload struct {
		Items   []service.WorldKeywordInput `json:"items"`
		Replace bool                        `json:"replace"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	stats, err := service.WorldKeywordImport(worldID, user.ID, payload.Items, payload.Replace)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err == service.ErrWorldPermission {
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	requestID := utils.NewID()
	broadcastWorldKeywordEvent(&worldKeywordEventPayload{
		WorldID:     worldID,
		Operation:   "imported",
		RequestID:   requestID,
		ForceReload: true,
	})
	return c.JSON(fiber.Map{"stats": stats, "requestId": requestID})
}

func WorldKeywordExportHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	category := strings.TrimSpace(c.Query("category"))
	items, err := service.WorldKeywordExport(worldID, user.ID, category)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err == service.ErrWorldPermission {
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"items": items})
}

func WorldKeywordCategoriesHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	categories, err := service.WorldKeywordListCategories(worldID, user.ID)
	if err != nil {
		status := fiber.StatusInternalServerError
		if err == service.ErrWorldPermission {
			status = fiber.StatusForbidden
		}
		return c.Status(status).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"categories": categories})
}

func WorldKeywordPublicCategoriesHandler(c *fiber.Ctx) error {
	worldID := c.Params("worldId")
	categories, err := service.WorldKeywordListCategoriesPublic(worldID)
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
	return c.JSON(fiber.Map{"categories": categories})
}

type worldKeywordEventPayload struct {
	WorldID     string                     `json:"worldId"`
	KeywordIDs  []string                   `json:"keywordIds,omitempty"`
	Operation   string                     `json:"operation"`
	RequestID   string                     `json:"requestId,omitempty"`
	Keywords    []*model.WorldKeywordModel `json:"keywords,omitempty"`
	DeletedIDs  []string                   `json:"deletedIds,omitempty"`
	ForceReload bool                       `json:"forceReload,omitempty"`
}

func broadcastWorldKeywordEvent(payload *worldKeywordEventPayload) {
	if payload == nil || strings.TrimSpace(payload.WorldID) == "" {
		return
	}
	if len(payload.KeywordIDs) == 0 && len(payload.Keywords) > 0 {
		ids := make([]string, 0, len(payload.Keywords))
		for _, item := range payload.Keywords {
			if item != nil {
				ids = append(ids, item.ID)
			}
		}
		payload.KeywordIDs = ids
	}
	now := time.Now().UnixMilli()
	options := map[string]interface{}{
		"worldId":    payload.WorldID,
		"keywordIds": payload.KeywordIDs,
		"operation":  payload.Operation,
		"version":    now,
		"revision":   now,
	}
	if payload.RequestID != "" {
		options["requestId"] = payload.RequestID
	}
	if len(payload.Keywords) > 0 {
		options["keywords"] = payload.Keywords
	}
	if len(payload.DeletedIDs) > 0 {
		options["deletedIds"] = payload.DeletedIDs
	}
	if payload.ForceReload {
		options["forceReload"] = true
	}
	event := &protocol.Event{
		Type: protocol.EventWorldKeywordsUpdated,
		Argv: &protocol.Argv{Options: options},
	}
	broadcastEventToWorld(payload.WorldID, event)
}

func broadcastEventToWorld(worldID string, event *protocol.Event) {
	if userId2ConnInfoGlobal == nil {
		return
	}
	event.Timestamp = time.Now().Unix()
	userId2ConnInfoGlobal.Range(func(_ string, conns *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		conns.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info != nil && info.WorldId == worldID {
				_ = conn.WriteJSON(struct {
					protocol.Event
					Op protocol.Opcode `json:"op"`
				}{
					Event: *event,
					Op:    protocol.OpEvent,
				})
			}
			return true
		})
		return true
	})
}
