package api

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

func BindWorldClueRoutes(group fiber.Router) {
	group.Get("/:worldId/clue-roster", worldClueRosterListHandler)
	group.Put("/:worldId/clue-roster/:userId", worldClueRosterAddHandler)
	group.Delete("/:worldId/clue-roster/:userId", worldClueRosterRemoveHandler)
	group.Get("/:worldId/clues", worldClueListHandler)
	group.Post("/:worldId/clues", worldClueCreateHandler)
	group.Get("/:worldId/clues/pending-presentations", worldCluePendingHandler)
	group.Post("/:worldId/clues/resolve", worldClueResolveHandler)
	group.Post("/:worldId/clues/reorder", worldClueReorderHandler)
	group.Get("/:worldId/clues/:clueId", worldClueGetHandler)
	group.Get("/:worldId/clues/:clueId/image", worldClueImageHandler)
	group.Get("/:worldId/clues/:clueId/background-media", worldClueBackgroundMediaHandler)
	group.Patch("/:worldId/clues/:clueId", worldClueUpdateHandler)
	group.Delete("/:worldId/clues/:clueId", worldClueDeleteHandler)
	group.Post("/:worldId/clues/:clueId/publish", worldCluePublishHandler)
	group.Post("/:worldId/clues/:clueId/unpublish", worldClueUnpublishHandler)
	group.Get("/:worldId/clues/:clueId/edit-locks", worldClueEditLocksListHandler)
	group.Post("/:worldId/clues/:clueId/edit-lock/acquire", worldClueEditLockAcquireHandler)
	group.Post("/:worldId/clues/:clueId/edit-lock/release", worldClueEditLockReleaseHandler)
	group.Post("/:worldId/clues/:clueId/presented", worldCluePresentedHandler)
	group.Post("/:worldId/clues/:clueId/seen", worldClueSeenHandler)
	group.Get("/:worldId/clues/:clueId/access", worldClueAccessListHandler)
	group.Patch("/:worldId/clues/:clueId/access", worldClueAccessHandler)
	group.Get("/:worldId/clues/:clueId/private/:userId", worldCluePrivateGetHandler)
	group.Put("/:worldId/clues/:clueId/private/:userId", worldCluePrivatePutHandler)
	group.Patch("/:worldId/clues/:clueId/user-state", worldClueUserStateHandler)

	group.Get("/:worldId/clue-folders", worldClueFolderListHandler)
	group.Post("/:worldId/clue-folders", worldClueFolderCreateHandler)
	group.Post("/:worldId/clue-folders/reorder", worldClueFolderReorderHandler)
	group.Patch("/:worldId/clue-folders/:folderId", worldClueFolderUpdateHandler)
	group.Delete("/:worldId/clue-folders/:folderId", worldClueFolderDeleteHandler)
}

func worldClueRosterListHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	items, err := service.WorldClueRosterList(c.Params("worldId"), userID)
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"items": items})
}

func worldClueRosterAddHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	item, err := service.WorldClueRosterAdd(c.Params("worldId"), userID, c.Params("userId"))
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"item": item})
}

func worldClueRosterRemoveHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	if err := service.WorldClueRosterRemove(c.Params("worldId"), userID, c.Params("userId")); err != nil {
		return worldClueError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type worldClueReorderBody struct {
	Scope      string   `json:"scope"`
	FolderID   string   `json:"folderId"`
	OrderedIDs []string `json:"orderedIds"`
}

type worldClueFolderBody struct {
	Scope      string  `json:"scope"`
	ParentID   *string `json:"parentId"`
	Name       *string `json:"name"`
	Color      *string `json:"color"`
	OrderIndex *int    `json:"orderIndex"`
}

func worldClueError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, service.ErrWorldClueNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "线索不可用"})
	case errors.Is(err, service.ErrWorldClueDenied):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"message": "无权操作线索"})
	case errors.Is(err, service.ErrWorldClueConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "线索已被其他人更新"})
	case errors.Is(err, service.ErrWorldClueInvalid), errors.Is(err, service.ErrWorldClueFolderCycle):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "线索操作失败"})
	}
}

func worldClueUser(c *fiber.Ctx) (string, error) {
	user := getCurUser(c)
	if user == nil {
		return "", c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"message": "未登录"})
	}
	return user.ID, nil
}

type worldClueWriteBody struct {
	ExpectedRevision  int64                           `json:"expectedRevision"`
	Title             *string                         `json:"title"`
	Kind              *string                         `json:"kind"`
	ContentFormat     *string                         `json:"contentFormat"`
	Content           *string                         `json:"content"`
	ImageAttachmentID *string                         `json:"imageAttachmentId"`
	ImageURL          *string                         `json:"imageUrl"`
	EmbedURL          *string                         `json:"embedUrl"`
	Presentation      *protocol.WorldCluePresentation `json:"presentation"`
	SharedFolderID    *string                         `json:"sharedFolderId"`
	DefaultAccess     *string                         `json:"defaultAccess"`
	OrderIndex        *int                            `json:"orderIndex"`
	ManagerNoteFormat *string                         `json:"managerNoteFormat"`
	ManagerNote       *string                         `json:"managerNote"`
}

func worldClueListHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	items, err := service.WorldClueList(c.Params("worldId"), userID, c.Query("keyword"))
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"items": items})
}

func worldClueCreateHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body worldClueWriteBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	value := func(ptr *string) string {
		if ptr == nil {
			return ""
		}
		return *ptr
	}
	order := 0
	if body.OrderIndex != nil {
		order = *body.OrderIndex
	}
	item, err := service.WorldClueCreate(c.Params("worldId"), userID, service.WorldClueCreateInput{
		Title: value(body.Title), Kind: value(body.Kind), ContentFormat: value(body.ContentFormat), Content: value(body.Content),
		ImageAttachmentID: value(body.ImageAttachmentID), ImageURL: value(body.ImageURL), EmbedURL: value(body.EmbedURL),
		Presentation: body.Presentation, SharedFolderID: value(body.SharedFolderID), DefaultAccess: value(body.DefaultAccess),
		OrderIndex: order, ManagerNoteFormat: value(body.ManagerNoteFormat), ManagerNote: value(body.ManagerNote),
	})
	if err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), item.ID, "upsert", item.Revision)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"item": item})
}

func worldClueGetHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	worldID, clueID := c.Params("worldId"), c.Params("clueId")
	if service.IsWorldAdmin(worldID, userID) {
		item, err := service.WorldClueGetAdmin(worldID, clueID, userID)
		if err != nil {
			return worldClueError(c, err)
		}
		return c.JSON(fiber.Map{"item": item})
	}
	item, err := service.WorldClueGet(worldID, clueID, userID)
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"item": item})
}

func worldClueEditLocksListHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	items, err := service.WorldClueListEditLocks(c.Params("worldId"), c.Params("clueId"), userID)
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"items": items})
}

type worldClueEditLockBody struct {
	Field     string `json:"field"`
	SessionID string `json:"sessionId"`
}

func worldClueEditLockAcquireHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body worldClueEditLockBody
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Field) == "" || strings.TrimSpace(body.SessionID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	lock, acquired, err := service.WorldClueAcquireEditLock(c.Params("worldId"), c.Params("clueId"), userID, body.Field, body.SessionID)
	if err != nil {
		return worldClueError(c, err)
	}
	if !acquired {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"message": "该字段正在被其他用户编辑", "lock": lock})
	}
	broadcastWorldClueChanged(c.Params("worldId"), c.Params("clueId"), "edit-lock", 0)
	return c.JSON(fiber.Map{"lock": lock})
}

func worldClueEditLockReleaseHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body worldClueEditLockBody
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.Field) == "" || strings.TrimSpace(body.SessionID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	released, err := service.WorldClueReleaseEditLock(c.Params("worldId"), c.Params("clueId"), userID, body.Field, body.SessionID)
	if err != nil {
		return worldClueError(c, err)
	}
	if released {
		broadcastWorldClueChanged(c.Params("worldId"), c.Params("clueId"), "edit-lock", 0)
	}
	return c.JSON(fiber.Map{"released": released})
}

func worldClueImageHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	detail, err := service.WorldClueGet(c.Params("worldId"), c.Params("clueId"), userID)
	if err != nil || detail.ImageAttachmentID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "线索图片不可用"})
	}
	var attachment model.AttachmentModel
	if err := model.GetDB().Where("id = ? AND root_id = ? AND root_id_type = ?", detail.ImageAttachmentID, c.Params("worldId"), "world_clue").Limit(1).Find(&attachment).Error; err != nil || attachment.ID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "线索图片不可用"})
	}
	return serveAttachmentRecord(c, &attachment)
}

func worldClueBackgroundMediaHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	detail, err := service.WorldClueGet(c.Params("worldId"), c.Params("clueId"), userID)
	if err != nil || detail == nil || detail.Presentation.BackgroundMediaAttachmentID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "线索背景媒体不可用"})
	}
	attachmentID := detail.Presentation.BackgroundMediaAttachmentID
	var attachment model.AttachmentModel
	if err := model.GetDB().Where("id = ? AND root_id = ? AND root_id_type = ?", attachmentID, c.Params("worldId"), "world_clue").Limit(1).Find(&attachment).Error; err != nil || attachment.ID == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "线索背景媒体不可用"})
	}
	return serveAttachmentRecord(c, &attachment)
}

func worldClueUpdateHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body worldClueWriteBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	item, err := service.WorldClueUpdate(c.Params("worldId"), c.Params("clueId"), userID, service.WorldClueUpdateInput{
		ExpectedRevision: body.ExpectedRevision, Title: body.Title, Kind: body.Kind, ContentFormat: body.ContentFormat,
		Content: body.Content, ImageAttachmentID: body.ImageAttachmentID, ImageURL: body.ImageURL, EmbedURL: body.EmbedURL,
		Presentation: body.Presentation, SharedFolderID: body.SharedFolderID, DefaultAccess: body.DefaultAccess,
		OrderIndex: body.OrderIndex, ManagerNoteFormat: body.ManagerNoteFormat, ManagerNote: body.ManagerNote,
	})
	if err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), item.ID, "upsert", item.Revision)
	return c.JSON(fiber.Map{"item": item})
}

func worldClueDeleteHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	if err := service.WorldClueDelete(c.Params("worldId"), c.Params("clueId"), userID); err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), c.Params("clueId"), "remove", 0)
	return c.SendStatus(fiber.StatusNoContent)
}

func worldCluePublishHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body struct {
		ExpectedPublishSeq int64 `json:"expectedPublishSeq"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	result, err := service.WorldCluePublish(c.Params("worldId"), c.Params("clueId"), userID, body.ExpectedPublishSeq)
	if err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), c.Params("clueId"), "upsert", result.Clue.Revision)
	broadcastWorldCluePublished(c.Params("worldId"), c.Params("clueId"), result.Clue.PublishSeq, result.RecipientIDs)
	return c.JSON(fiber.Map{"item": result.Clue})
}

func worldClueUnpublishHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body struct {
		ExpectedPublishSeq int64 `json:"expectedPublishSeq"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	item, err := service.WorldClueUnpublish(c.Params("worldId"), c.Params("clueId"), userID, body.ExpectedPublishSeq)
	if err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), c.Params("clueId"), "upsert", item.Revision)
	return c.JSON(fiber.Map{"item": item})
}

func worldCluePendingHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	items, err := service.WorldCluePendingPresentations(c.Params("worldId"), userID)
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"items": items})
}

func worldCluePresentedHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body struct {
		PublishSeq int64 `json:"publishSeq"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	if err := service.WorldClueMarkPresented(c.Params("worldId"), c.Params("clueId"), userID, body.PublishSeq); err != nil {
		return worldClueError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func worldClueSeenHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	if err := service.WorldClueMarkSeen(c.Params("worldId"), c.Params("clueId"), userID); err != nil {
		return worldClueError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func worldClueAccessListHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	items, err := service.WorldClueListAccess(c.Params("worldId"), c.Params("clueId"), userID)
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"items": items})
}

func worldClueAccessHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body struct {
		UserID         string `json:"userId"`
		AccessOverride string `json:"accessOverride"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.UserID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	item, err := service.WorldClueSetAccess(c.Params("worldId"), c.Params("clueId"), userID, body.UserID, body.AccessOverride)
	if err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), c.Params("clueId"), "upsert", 0)
	return c.JSON(fiber.Map{"item": item})
}

func worldCluePrivateGetHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	item, err := service.WorldClueGetPrivate(c.Params("worldId"), c.Params("clueId"), userID, c.Params("userId"))
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"item": item})
}

func worldCluePrivatePutHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body struct {
		PrivateContentFormat string `json:"privateContentFormat"`
		PrivateContent       string `json:"privateContent"`
		ExpectedRevision     int64  `json:"expectedRevision"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	item, err := service.WorldCluePutPrivate(c.Params("worldId"), c.Params("clueId"), userID, c.Params("userId"), body.PrivateContentFormat, body.PrivateContent, body.ExpectedRevision)
	if err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), c.Params("clueId"), "upsert", 0)
	return c.JSON(fiber.Map{"item": item})
}

func worldClueUserStateHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body struct {
		PersonalFolderID *string `json:"personalFolderId"`
		PersonalOrder    *int    `json:"personalOrder"`
		Favorite         *bool   `json:"favorite"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	item, err := service.WorldClueUpdateUserState(c.Params("worldId"), c.Params("clueId"), userID, service.WorldClueUserStateInput(body))
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"item": item})
}

func worldClueResolveHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body struct {
		ClueIDs []string `json:"clueIds"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	items, err := service.WorldClueResolve(c.Params("worldId"), userID, body.ClueIDs)
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"items": items})
}

func worldClueReorderHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body worldClueReorderBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	if err := service.WorldClueReorder(c.Params("worldId"), userID, service.WorldClueReorderInput{Scope: body.Scope, FolderID: body.FolderID, OrderedIDs: body.OrderedIDs}); err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), "", "reorder", 0)
	return c.SendStatus(fiber.StatusNoContent)
}

func worldClueFolderListHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	items, err := service.WorldClueFolderList(c.Params("worldId"), userID)
	if err != nil {
		return worldClueError(c, err)
	}
	return c.JSON(fiber.Map{"items": items})
}

func worldClueFolderCreateHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body worldClueFolderBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	value := func(ptr *string) string {
		if ptr == nil {
			return ""
		}
		return *ptr
	}
	orderIndex := 0
	if body.OrderIndex != nil {
		orderIndex = *body.OrderIndex
	}
	item, err := service.WorldClueFolderCreate(c.Params("worldId"), userID, service.WorldClueFolderInput{
		Scope: body.Scope, ParentID: value(body.ParentID), Name: value(body.Name), Color: value(body.Color), OrderIndex: orderIndex,
	})
	if err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), "", "folders", 0)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"item": item})
}

func worldClueFolderUpdateHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body worldClueFolderBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	item, err := service.WorldClueFolderUpdate(c.Params("worldId"), c.Params("folderId"), userID, service.WorldClueFolderUpdateInput{
		ParentID: body.ParentID, Name: body.Name, Color: body.Color, OrderIndex: body.OrderIndex,
	})
	if err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), "", "folders", 0)
	return c.JSON(fiber.Map{"item": item})
}

func worldClueFolderReorderHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	var body worldClueReorderBody
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "请求格式错误"})
	}
	if err := service.WorldClueFolderReorder(c.Params("worldId"), userID, service.WorldClueReorderInput{Scope: body.Scope, FolderID: body.FolderID, OrderedIDs: body.OrderedIDs}); err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), "", "folders", 0)
	return c.SendStatus(fiber.StatusNoContent)
}

func worldClueFolderDeleteHandler(c *fiber.Ctx) error {
	userID, err := worldClueUser(c)
	if err != nil {
		return err
	}
	if err := service.WorldClueFolderDelete(c.Params("worldId"), c.Params("folderId"), userID); err != nil {
		return worldClueError(c, err)
	}
	broadcastWorldClueChanged(c.Params("worldId"), "", "folders", 0)
	return c.SendStatus(fiber.StatusNoContent)
}

func broadcastWorldClueChanged(worldID, clueID, action string, revision int64) {
	go func() {
		recipients, err := buildOnlineWorldMemberRecipients(worldID, userId2ConnInfoGlobal)
		if err != nil {
			return
		}
		event := protocol.Event{Type: protocol.EventWorldClueChanged, Timestamp: time.Now().Unix(), WorldClue: &protocol.WorldClueEventPayload{WorldID: worldID, ClueID: clueID, Action: action, Revision: revision}}
		payload := struct {
			protocol.Event
			Op protocol.Opcode `json:"op"`
		}{Event: event, Op: protocol.OpEvent}
		for _, userID := range recipients {
			conns, ok := userId2ConnInfoGlobal.Load(userID)
			if !ok || conns == nil {
				continue
			}
			conns.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
				if info != nil && !info.IsGuest && !info.IsObserver && info.WorldId == worldID {
					_ = conn.WriteJSON(payload)
				}
				return true
			})
		}
	}()
}

func broadcastWorldCluePublished(worldID, clueID string, publishSeq int64, recipientIDs []string) {
	if userId2ConnInfoGlobal == nil {
		return
	}
	event := protocol.Event{Type: protocol.EventWorldCluePublished, Timestamp: time.Now().Unix(), WorldClue: &protocol.WorldClueEventPayload{WorldID: worldID, ClueID: clueID, PublishSeq: publishSeq}}
	for _, userID := range recipientIDs {
		conns, ok := userId2ConnInfoGlobal.Load(userID)
		if !ok || conns == nil {
			continue
		}
		var selected *WsSyncConn
		var selectedInfo *ConnInfo
		conns.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info == nil || info.IsGuest || info.IsObserver || info.WorldId != worldID {
				return true
			}
			if selectedInfo == nil || (info.Focused && !selectedInfo.Focused) || (info.Focused == selectedInfo.Focused && info.LastAliveTime > selectedInfo.LastAliveTime) {
				selected, selectedInfo = conn, info
			}
			return true
		})
		if selected != nil {
			_ = selected.WriteJSON(struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{Event: event, Op: protocol.OpEvent})
		}
	}
}
