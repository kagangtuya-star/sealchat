package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

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
	if err := service.EnsureWorldKeywordReadable(worldID, user.ID); err != nil {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权访问该世界"})
	}
	search := strings.TrimSpace(c.Query("keyword"))
	items, err := service.ListWorldKeywords(worldID, search)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "获取关键词失败"})
	}
	return c.JSON(fiber.Map{"items": items})
}

func WorldKeywordCreateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var body service.WorldKeywordCreateParams
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, err := service.CreateWorldKeyword(worldID, user.ID, body)
	switch err {
	case nil:
		broadcastWorldKeywordSnapshot(worldID)
		return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item})
	case service.ErrWorldKeywordConflict:
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "关键词已存在"})
	case service.ErrWorldKeywordForbidden:
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权限维护关键词"})
	case service.ErrWorldKeywordInvalid:
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "关键词或描述不合法"})
	default:
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "创建失败"})
	}
}

func WorldKeywordUpdateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	keywordID := c.Params("keywordId")
	var body service.WorldKeywordUpdateParams
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	if body.Keyword == nil && body.Description == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "缺少更新内容"})
	}
	item, err := service.UpdateWorldKeyword(worldID, keywordID, user.ID, body)
	switch err {
	case nil:
		broadcastWorldKeywordSnapshot(worldID)
		return c.JSON(fiber.Map{"item": item})
	case service.ErrWorldKeywordNotFound:
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "关键词不存在"})
	case service.ErrWorldKeywordConflict:
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "关键词已存在"})
	case service.ErrWorldKeywordForbidden:
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权限维护关键词"})
	case service.ErrWorldKeywordInvalid:
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"message": "关键词或描述不合法"})
	default:
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "更新失败"})
	}
}

func WorldKeywordDeleteHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	keywordID := c.Params("keywordId")
	err := service.DeleteWorldKeyword(worldID, keywordID, user.ID)
	switch err {
	case nil:
		broadcastWorldKeywordSnapshot(worldID)
		return c.JSON(fiber.Map{"success": true})
	case service.ErrWorldKeywordNotFound:
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "关键词不存在"})
	case service.ErrWorldKeywordForbidden:
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权限维护关键词"})
	default:
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "删除失败"})
	}
}

func WorldKeywordExportHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	if err := service.EnsureWorldKeywordReadable(worldID, user.ID); err != nil {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权访问"})
	}
	data, err := service.ExportWorldKeywordsJSON(worldID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"message": "导出失败"})
	}
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
	filename := "world-keywords-" + worldID + ".json"
	c.Set(fiber.HeaderContentDisposition, "attachment; filename=\""+filename+"\"")
	return c.Send(data)
}

func WorldKeywordImportHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	worldID := c.Params("worldId")
	var body struct {
		Content string `json:"content"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "导入内容不能为空"})
	}
	stats, err := service.ImportWorldKeywordsFromContent(worldID, user.ID, content)
	if err != nil {
		switch err {
		case service.ErrWorldKeywordForbidden:
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"message": "无权限维护关键词"})
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
		}
	}
	broadcastWorldKeywordSnapshot(worldID)
	return c.JSON(fiber.Map{
		"created": stats.Created,
		"updated": stats.Updated,
		"skipped": stats.Skipped,
		"total":   stats.Total,
	})
}

func broadcastWorldKeywordSnapshot(worldID string) {
	keywords, err := service.ListWorldKeywords(worldID, "")
	if err != nil {
		return
	}
	connMap := getUserConnInfoMap()
	if connMap == nil {
		return
	}
	payload := &protocol.WorldKeywordPayload{
		WorldID:   worldID,
		UpdatedAt: time.Now().UnixMilli(),
		Keywords:  make([]*protocol.WorldKeywordEntry, 0, len(keywords)),
	}
	for _, item := range keywords {
		payload.Keywords = append(payload.Keywords, &protocol.WorldKeywordEntry{
			ID:          item.ID,
			Keyword:     item.Keyword,
			Description: item.Description,
			UpdatedAt:   item.UpdatedAt.UnixMilli(),
		})
	}
	event := struct {
		protocol.Event
		Op protocol.Opcode `json:"op"`
	}{
		Event: protocol.Event{
			Type:          protocol.EventWorldKeywordsUpdated,
			Channel:       &protocol.Channel{ID: "", WorldID: worldID},
			WorldKeywords: payload,
		},
		Op: protocol.OpEvent,
	}
	connMap.Range(func(_ string, sessions *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		sessions.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			_ = conn.WriteJSON(event)
			return true
		})
		return true
	})
}
