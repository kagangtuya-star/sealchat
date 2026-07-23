package api

import (
	"github.com/gofiber/fiber/v2"

	"sealchat/service"
)

func TheaterAdminSnapshots(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	items, err := service.ListTheaterCheckpoints(user.ID, c.Params("worldId"), c.Params("channelId"), 200)
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "snapshots": items})
}

func TheaterAdminRestore(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	var body struct {
		MutationID       string `json:"mutationId"`
		SnapshotID       string `json:"snapshotId"`
		Reason           string `json:"reason"`
		ExpectedRevision *int64 `json:"expectedRevision"`
	}
	if err := decodeTheaterBody(c, &body, 256<<10); err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	result, err := service.RestoreTheaterSnapshot(c.Context(), user.ID, service.TheaterRestoreCommand{MutationID: body.MutationID, WorldID: c.Params("worldId"), ChannelID: c.Params("channelId"), SnapshotID: body.SnapshotID, Reason: body.Reason, ExpectedRevision: body.ExpectedRevision}, theaterRequestMeta(c, requestID))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "result": result})
}

func TheaterAdminReplace(c *fiber.Ctx) error {
	requestID := theaterRequestID(c)
	user := getCurUser(c)
	var body struct {
		MutationID       string                        `json:"mutationId"`
		ExpectedRevision int64                         `json:"expectedRevision"`
		SchemaVersion    int                           `json:"schemaVersion"`
		Snapshot         service.TheaterSharedSnapshot `json:"snapshot"`
		Reason           string                        `json:"reason"`
	}
	if err := decodeTheaterBody(c, &body, 4<<20); err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	result, err := service.ReplaceTheaterSnapshot(c.Context(), user.ID, service.TheaterReplaceCommand{MutationID: body.MutationID, WorldID: c.Params("worldId"), ChannelID: c.Params("channelId"), ExpectedRevision: body.ExpectedRevision, SchemaVersion: body.SchemaVersion, Snapshot: body.Snapshot, Reason: body.Reason}, theaterRequestMeta(c, requestID))
	if err != nil {
		return theaterErrorResponse(c, requestID, err)
	}
	return c.JSON(fiber.Map{"ok": true, "requestId": requestID, "result": result})
}

func BindTheaterRoutes(router fiber.Router) {
	base := "/worlds/:worldId/channels/:channelId/theater"
	bindTheaterRoutes(router, base)
	bindTheaterAudioRoutes(router, base)
}

func BindTheaterAudioRoutes(router fiber.Router) {
	bindTheaterAudioRoutes(router, "/worlds/:worldId/channels/:channelId/theater")
}

func bindTheaterAudioRoutes(router fiber.Router, base string) {
	router.Get(base+"/audio-assets", TheaterAudioAssetList)
	router.Post(base+"/audio-assets", TheaterAudioAssetUpload)
	router.Delete(base+"/audio-assets/:assetId", TheaterAudioAssetDelete)
}

func BindWorldTheaterRoutes(router fiber.Router) {
	bindTheaterRoutes(router, "/worlds/:worldId/theater")
}

func bindTheaterRoutes(router fiber.Router, base string) {
	router.Get(base, TheaterSnapshotGet)
	router.Get(base+"/editor-state/groups", TheaterGroupEditorStateGet)
	router.Put(base+"/editor-state/groups/:objectId", TheaterGroupEditorStatePut)
	router.Get(base+"/panel-organizer", TheaterPanelOrganizerGet)
	router.Post(base+"/panel-organizer/folders", TheaterPanelFolderPost)
	router.Patch(base+"/panel-organizer/folders/:folderId", TheaterPanelFolderPatch)
	router.Delete(base+"/panel-organizer/folders/:folderId", TheaterPanelFolderDelete)
	router.Put(base+"/panel-organizer/folders/:folderId/state", TheaterPanelFolderStatePut)
	router.Put(base+"/panel-organizer/folder-order", TheaterPanelFolderOrderPut)
	router.Put(base+"/panel-organizer/item-order", TheaterPanelItemOrderPut)
	router.Post(base+"/mutations", TheaterMutationPost)
	router.Get(base+"/events", TheaterEventsGet)
	router.Post(base+"/actions/trigger", TheaterActionTrigger)
	router.Post(base+"/resources", TheaterResourceUpload)
	router.Get(base+"/resources/:resourceId/processing", TheaterResourceProcessingGet)
	router.Get(base+"/resources/:resourceId/variants/:variant/content-url", TheaterResourceVariantContentURL)
	router.Get(base+"/resources/:resourceId/variants/:variant/content", TheaterResourceVariantContent)
	router.Get(base+"/resources/:resourceId/content-url", TheaterResourceContentURL)
	router.Get(base+"/resources/:resourceId/content", TheaterResourceContent)
	router.Post(base+"/resources/:resourceId/retry", TheaterResourceRetry)
	router.Get(base+"/resources/:resourceId", TheaterResourceGet)
	router.Delete(base+"/resources/:resourceId", TheaterResourceDelete)
	router.Get(base+"/admin/snapshots", TheaterAdminSnapshots)
	router.Get(base+"/admin/audit", TheaterAdminAudit)
	router.Post(base+"/admin/restore", TheaterAdminRestore)
	router.Put(base+"/admin/snapshot", TheaterAdminReplace)
	router.Post(base+"/packages/export", TheaterPackageExportCreate)
	router.Post(base+"/packages/import", TheaterPackageImportCreate)
	router.Post(base+"/packages/import/ccfolia", TheaterPackageCCFOLIAImportCreate)
	router.Get(base+"/packages/jobs/:jobId", TheaterPackageJobGet)
	router.Get(base+"/packages/jobs/:jobId/download", TheaterPackageDownload)
	router.Delete(base+"/packages/jobs/:jobId", TheaterPackageJobDelete)
}
