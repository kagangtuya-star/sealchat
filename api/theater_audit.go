package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func TheaterAdminAudit(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	filter := service.TheaterAuditFilter{
		ActorID: c.Query("actor"), Type: c.Query("type"), Outcome: c.Query("outcome"),
		Offset: c.QueryInt("offset", 0), Limit: c.QueryInt("limit", 50),
	}
	var err error
	if value := strings.TrimSpace(c.Query("from")); value != "" {
		filter.From, err = parseTheaterAuditTime(value)
		if err != nil {
			return theaterErrorResponse(c, requestID, service.NewTheaterPayloadErrorForAPI("from 时间无效"))
		}
	}
	if value := strings.TrimSpace(c.Query("to")); value != "" {
		filter.To, err = parseTheaterAuditTime(value)
		if err != nil {
			return theaterErrorResponse(c, requestID, service.NewTheaterPayloadErrorForAPI("to 时间无效"))
		}
	}
	result, err := service.ListTheaterAudit(user.ID, c.Params("worldId"), c.Params("channelId"), filter)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "audit": result})
}

func parseTheaterAuditTime(value string) (*time.Time, error) {
	if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
		parsed := time.UnixMilli(millis)
		return &parsed, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
