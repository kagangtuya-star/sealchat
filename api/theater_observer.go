package api

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func BindTheaterObserverRoutes(router fiber.Router) {
	worldBase := "/public/ob/:slug/theater"
	router.Get(worldBase, TheaterObserverSnapshotGet)
	router.Get(worldBase+"/events", TheaterObserverEventsGet)
	router.Get(worldBase+"/resources/:resourceId/variants/:variant/content", TheaterObserverResourceVariantContent)
	router.Get(worldBase+"/resources/:resourceId/content", TheaterObserverResourceContent)
	router.Get(worldBase+"/resources/:resourceId", TheaterObserverResourceGet)

	base := "/public/ob/channels/:channelId/theater"
	router.Get(base, TheaterObserverSnapshotGet)
	router.Get(base+"/events", TheaterObserverEventsGet)
	router.Get(base+"/resources/:resourceId/variants/:variant/content", TheaterObserverResourceVariantContent)
	router.Get(base+"/resources/:resourceId/content", TheaterObserverResourceContent)
	router.Get(base+"/resources/:resourceId", TheaterObserverResourceGet)
}

func TheaterObserverSnapshotGet(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	worldID, err := resolveTheaterObserverWorld(c)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	options := service.TheaterSnapshotOptions{IncludeResources: !strings.EqualFold(c.Query("includeResources"), "false")}
	if raw := strings.TrimSpace(c.Query("ifRevision")); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value < 0 {
			return theaterErrorResponse(c, requestID, service.NewTheaterPayloadErrorForAPI("ifRevision 无效"))
		}
		options.IfRevision = &value
	}
	result, err := service.GetTheaterSnapshotForObserver(c.Context(), worldID, c.Params("channelId"), options)
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

func TheaterObserverEventsGet(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	worldID, err := resolveTheaterObserverWorld(c)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	after, err := strconv.ParseInt(c.Query("afterRevision", "0"), 10, 64)
	if err != nil || after < 0 {
		return theaterErrorResponse(c, requestID, service.NewTheaterPayloadErrorForAPI("afterRevision 无效"))
	}
	result, err := service.ListTheaterEventsForObserver(c.Context(), worldID, c.Params("channelId"), after, c.QueryInt("limit", 200))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "fromRevision": result.FromRevision, "toRevision": result.ToRevision, "currentRevision": result.CurrentRevision, "hasMore": result.HasMore, "events": result.Events})
}

func TheaterObserverResourceGet(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	worldID, err := resolveTheaterObserverWorld(c)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	resource, err := service.GetTheaterResourceForObserver(c.Context(), worldID, c.Params("channelId"), c.Params("resourceId"))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "resource": resource})
}

func TheaterObserverResourceContent(c *fiber.Ctx) error {
	return theaterObserverResourceContent(c, "")
}

func TheaterObserverResourceVariantContent(c *fiber.Ctx) error {
	return theaterObserverResourceContent(c, c.Params("variant"))
}

func theaterObserverResourceContent(c *fiber.Ctx, variant string) error {
	requestID := theaterRequestID(c)
	worldID, err := resolveTheaterObserverWorld(c)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	content, err := service.ResolveTheaterResourceContentForObserver(c.Context(), worldID, c.Params("channelId"), c.Params("resourceId"), variant)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return serveTheaterResourceContent(c, requestID, content)
}

func resolveTheaterObserverWorld(c *fiber.Ctx) (string, error) {
	slug := strings.TrimSpace(c.Params("slug"))
	if slug == "" {
		slug = strings.TrimSpace(c.Query("ob_slug"))
	}
	if slug == "" {
		slug = strings.TrimSpace(c.Get("X-Observer-Slug"))
	}
	if slug == "" {
		return "", service.NewTheaterPayloadErrorForAPI("缺少 OB 链接标识")
	}
	world, _, err := service.ResolveWorldObserverLink(slug)
	if err != nil {
		if errors.Is(err, service.ErrWorldObserverLinkInvalid) {
			return "", &service.TheaterError{Code: service.TheaterErrorWorldNotFound, Message: "旁观链接无效或已关闭", HTTPStatus: fiber.StatusNotFound}
		}
		return "", err
	}
	if world == nil || strings.TrimSpace(world.ID) == "" {
		return "", &service.TheaterError{Code: service.TheaterErrorWorldNotFound, Message: "旁观链接无效或已关闭", HTTPStatus: fiber.StatusNotFound}
	}
	return world.ID, nil
}
