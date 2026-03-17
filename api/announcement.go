package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

func mapAnnouncementErrorStatus(err error) int {
	switch err {
	case nil:
		return fiber.StatusOK
	case service.ErrAnnouncementPermission, service.ErrWorldPermission:
		return fiber.StatusForbidden
	case service.ErrAnnouncementNotFound, service.ErrWorldNotFound:
		return fiber.StatusNotFound
	case service.ErrAnnouncementInvalid:
		return fiber.StatusBadRequest
	}
	if errors.Is(err, service.ErrAnnouncementPermission) || errors.Is(err, service.ErrWorldPermission) {
		return fiber.StatusForbidden
	}
	if errors.Is(err, service.ErrAnnouncementNotFound) || errors.Is(err, service.ErrWorldNotFound) {
		return fiber.StatusNotFound
	}
	if errors.Is(err, service.ErrAnnouncementInvalid) {
		return fiber.StatusBadRequest
	}
	return fiber.StatusInternalServerError
}

func parseAnnouncementListOptions(c *fiber.Ctx) service.AnnouncementListOptions {
	return service.AnnouncementListOptions{
		Page:            parseQueryIntDefault(c, "page", 1),
		PageSize:        parseQueryIntDefault(c, "pageSize", 20),
		IncludeAll:      c.QueryBool("includeAll"),
		IncludeArchived: c.QueryBool("includeArchived"),
	}
}

func WorldAnnouncementListHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	opts := parseAnnouncementListOptions(c)
	items, total, err := service.AnnouncementList("world", worldID, user.ID, opts)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{
		"items":    items,
		"total":    total,
		"page":     opts.Page,
		"pageSize": opts.PageSize,
	})
}

func WorldAnnouncementCreateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var payload service.AnnouncementInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, err := service.AnnouncementCreate("world", worldID, user.ID, payload)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item})
}

func WorldAnnouncementUpdateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	announcementID := c.Params("announcementId")
	var payload service.AnnouncementInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, err := service.AnnouncementUpdate("world", worldID, announcementID, user.ID, payload)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"item": item})
}

func WorldAnnouncementDeleteHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	announcementID := c.Params("announcementId")
	if err := service.AnnouncementDelete("world", worldID, announcementID, user.ID); err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "公告已归档"})
}

func WorldAnnouncementPendingPopupHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	item, err := service.AnnouncementPendingPopup("world", worldID, user.ID, nil)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"item": item})
}

func WorldAnnouncementMarkPopupHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	announcementID := c.Params("announcementId")
	item, err := service.AnnouncementMarkPopupShown("world", worldID, announcementID, user.ID)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"item": item})
}

func WorldAnnouncementAckHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	announcementID := c.Params("announcementId")
	item, err := service.AnnouncementAck("world", worldID, announcementID, user.ID)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"item": item})
}

func LobbyAnnouncementListHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	opts := parseAnnouncementListOptions(c)
	items, total, err := service.AnnouncementList("lobby", "", user.ID, opts)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{
		"items":    items,
		"total":    total,
		"page":     opts.Page,
		"pageSize": opts.PageSize,
	})
}

func LobbyAnnouncementPendingPopupHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	item, err := service.AnnouncementPendingPopup("lobby", "", user.ID, &service.AnnouncementPendingOptions{
		ReminderScope: c.Query("reminderScope"),
	})
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"item": item})
}

func LobbyAnnouncementMarkPopupHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	announcementID := c.Params("announcementId")
	item, err := service.AnnouncementMarkPopupShown("lobby", "", announcementID, user.ID)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{"item": item})
}

func ensureSystemAdmin(userID string) error {
	if strings.TrimSpace(userID) == "" || !pm.CanWithSystemRole(userID, pm.PermModAdmin) {
		return service.ErrAnnouncementPermission
	}
	return nil
}

func LobbyAnnouncementCreateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	if err := ensureSystemAdmin(user.ID); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权管理大厅公告"})
	}
	var payload service.AnnouncementInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, err := service.AnnouncementCreate("lobby", "", user.ID, payload)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	broadcastLobbyAnnouncementUpdated()
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item})
}

func LobbyAnnouncementUpdateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	if err := ensureSystemAdmin(user.ID); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权管理大厅公告"})
	}
	announcementID := c.Params("announcementId")
	var payload service.AnnouncementInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, err := service.AnnouncementUpdate("lobby", "", announcementID, user.ID, payload)
	if err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	broadcastLobbyAnnouncementUpdated()
	return c.JSON(fiber.Map{"item": item})
}

func LobbyAnnouncementDeleteHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	if err := ensureSystemAdmin(user.ID); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权管理大厅公告"})
	}
	announcementID := c.Params("announcementId")
	if err := service.AnnouncementDelete("lobby", "", announcementID, user.ID); err != nil {
		return c.Status(mapAnnouncementErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	broadcastLobbyAnnouncementUpdated()
	return c.JSON(fiber.Map{"message": "公告已归档"})
}

func broadcastLobbyAnnouncementUpdated() {
	userConnMap := getUserConnInfoMap()
	if userConnMap == nil {
		return
	}
	event := protocol.Event{
		Type:      protocol.EventLobbyAnnouncementUpdated,
		Timestamp: time.Now().Unix(),
	}
	userConnMap.Range(func(_ string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		if connMap == nil {
			return true
		}
		connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info == nil || info.IsGuest || info.User == nil || info.User.ID == "" || info.User.IsBot {
				return true
			}
			_ = conn.WriteJSON(struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{
				Event: event,
				Op:    protocol.OpEvent,
			})
			return true
		})
		return true
	})
}
