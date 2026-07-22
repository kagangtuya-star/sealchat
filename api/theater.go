package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
	"sealchat/utils"
)

func theaterRequestID(c *fiber.Ctx) string {
	if value := strings.TrimSpace(c.Get("X-Request-ID")); value != "" && len(value) <= 128 {
		return value
	}
	return utils.NewID()
}

func theaterRequestMeta(c *fiber.Ctx, requestID string) service.TheaterRequestMeta {
	user := getCurUser(c)
	name := ""
	if user != nil {
		name = user.Nickname
	}
	return service.TheaterRequestMeta{Source: "http", RequestID: requestID, RemoteIP: c.IP(), UserAgent: c.Get(fiber.HeaderUserAgent), ActorName: name}
}

func theaterErrorResponse(c *fiber.Ctx, requestID string, err error) error {
	status := fiber.StatusInternalServerError
	code := service.TheaterErrorInternal
	message := "Theater 内部错误"
	details := map[string]any{}
	if theaterErr, ok := err.(*service.TheaterError); ok {
		status = theaterErr.HTTPStatus
		if status == 0 {
			status = fiber.StatusBadRequest
		}
		code = theaterErr.Code
		message = theaterErr.Message
		if theaterErr.Details != nil {
			details = theaterErr.Details
		}
	}
	return c.Status(status).JSON(fiber.Map{"ok": false, "error": fiber.Map{"code": code, "message": message, "details": details}, "requestId": requestID})
}

func decodeTheaterBody(c *fiber.Ctx, target any, limit int) error {
	body := c.Body()
	if len(body) == 0 || len(body) > limit {
		return service.NewTheaterPayloadErrorForAPI("请求体大小无效")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return service.NewTheaterPayloadErrorForAPI(err.Error())
	}
	return nil
}

func TheaterSnapshotGet(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	options := service.TheaterSnapshotOptions{IncludeResources: !strings.EqualFold(c.Query("includeResources"), "false")}
	if raw := strings.TrimSpace(c.Query("ifRevision")); raw != "" {
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
			options.IfRevision = &value
		}
	}
	result, err := service.GetTheaterSnapshot(c.Context(), user.ID, c.Params("worldId"), c.Params("channelId"), options)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	etag := theaterSnapshotETag(result.Revision, result.Checksum, result.Permissions)
	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderCacheControl, "private, no-cache")
	if result.Unchanged || matchETag(c.Get(fiber.HeaderIfNoneMatch), etag) {
		return c.SendStatus(fiber.StatusNotModified)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "roomId": result.RoomID, "worldId": result.WorldID, "channelId": result.ChannelID, "revision": result.Revision, "schemaVersion": result.SchemaVersion, "checksum": result.Checksum, "snapshot": result.Snapshot, "limits": result.Limits, "permissions": result.Permissions})
}

func theaterSnapshotETag(revision int64, checksum string, permissions []string) string {
	normalizedPermissions := append([]string(nil), permissions...)
	sort.Strings(normalizedPermissions)
	permissionHash := sha256.Sum256([]byte(strings.Join(normalizedPermissions, "\x00")))
	return `"v2-` + strconv.FormatInt(revision, 10) + `-` + checksum + `-` + hex.EncodeToString(permissionHash[:8]) + `"`
}

func TheaterMutationPost(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	var command service.TheaterMutationCommand
	if err := decodeTheaterBody(c, &command, 256<<10); err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	if command.WorldID != c.Params("worldId") || command.ChannelID != c.Params("channelId") {
		return theaterErrorResponse(c, requestID, &service.TheaterError{Code: service.TheaterErrorChannelWorldMismatch, Message: "路径和 body scope 不一致", HTTPStatus: fiber.StatusBadRequest})
	}
	result, err := service.ApplyTheaterMutation(c.Context(), user.ID, command, theaterRequestMeta(c, requestID))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "mutationId": result.MutationID, "revisionBefore": result.RevisionBefore, "revision": result.Revision, "mutation": fiber.Map{"type": result.Type, "payload": result.Payload}, "checksum": result.Checksum, "idempotent": result.Idempotent})
}

func TheaterEventsGet(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	after, err := strconv.ParseInt(c.Query("afterRevision", "0"), 10, 64)
	if err != nil || after < 0 {
		return theaterErrorResponse(c, requestID, service.NewTheaterPayloadErrorForAPI("afterRevision 无效"))
	}
	result, err := service.ListTheaterEvents(c.Context(), user.ID, c.Params("worldId"), c.Params("channelId"), after, c.QueryInt("limit", 200))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "fromRevision": result.FromRevision, "toRevision": result.ToRevision, "currentRevision": result.CurrentRevision, "hasMore": result.HasMore, "events": result.Events})
}

func TheaterActionTrigger(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	var command service.TheaterActionCommand
	if err := decodeTheaterBody(c, &command, 256<<10); err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	command.WorldID = c.Params("worldId")
	command.ChannelID = c.Params("channelId")
	result, err := service.TriggerTheaterAction(c.Context(), user.ID, command, theaterRequestMeta(c, requestID))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "result": result})
}
