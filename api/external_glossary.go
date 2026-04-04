package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

type externalGlossaryLibraryListItem struct {
	*model.ExternalGlossaryLibraryModel
	TermCount int64 `json:"termCount"`
}

type externalGlossaryLibraryExportPayload struct {
	Library    *model.ExternalGlossaryLibraryModel `json:"library"`
	Categories []string                            `json:"categories"`
	Items      []*model.ExternalGlossaryTermModel  `json:"items"`
}

type externalGlossaryLibraryEventPayload struct {
	LibraryIDs  []string `json:"libraryIds,omitempty"`
	Operation   string   `json:"operation"`
	RequestID   string   `json:"requestId,omitempty"`
	ForceReload bool     `json:"forceReload,omitempty"`
}

func ensureExternalGlossaryAdminAPI(c *fiber.Ctx) (*model.UserModel, error) {
	user := getCurUser(c)
	if user == nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "未登录")
	}
	if strings.TrimSpace(user.ID) == "" || !pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return nil, fiber.NewError(fiber.StatusForbidden, "无权管理外挂术语")
	}
	return user, nil
}

func parseExternalGlossaryLibraryListOptions(c *fiber.Ctx) service.ExternalGlossaryLibraryListOptions {
	return service.ExternalGlossaryLibraryListOptions{
		Page:            parseQueryIntDefault(c, "page", 1),
		PageSize:        parseQueryIntDefault(c, "pageSize", 50),
		Query:           strings.TrimSpace(c.Query("q")),
		IncludeDisabled: c.QueryBool("includeDisabled"),
	}
}

func mapExternalGlossaryErrorStatus(err error) int {
	switch err {
	case nil:
		return fiber.StatusOK
	case service.ErrExternalGlossaryPermission, service.ErrWorldPermission:
		return fiber.StatusForbidden
	case service.ErrExternalGlossaryNotFound, service.ErrWorldNotFound:
		return fiber.StatusNotFound
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fiber.StatusNotFound
	}
	return fiber.StatusBadRequest
}

func loadExternalGlossaryTermCountMap(libraryIDs []string) (map[string]int64, error) {
	cleaned := normalizeStringIDs(libraryIDs)
	if len(cleaned) == 0 {
		return map[string]int64{}, nil
	}
	type row struct {
		LibraryID string
		Count     int64
	}
	var rows []row
	if err := model.GetDB().
		Model(&model.ExternalGlossaryTermModel{}).
		Select("library_id, COUNT(*) AS count").
		Where("library_id IN ?", cleaned).
		Group("library_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.LibraryID] = item.Count
	}
	return result, nil
}

func buildExternalGlossaryLibraryListItems(items []*model.ExternalGlossaryLibraryModel) ([]*externalGlossaryLibraryListItem, error) {
	libraryIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil && item.ID != "" {
			libraryIDs = append(libraryIDs, item.ID)
		}
	}
	countMap, err := loadExternalGlossaryTermCountMap(libraryIDs)
	if err != nil {
		return nil, err
	}
	result := make([]*externalGlossaryLibraryListItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, &externalGlossaryLibraryListItem{
			ExternalGlossaryLibraryModel: item,
			TermCount:                    countMap[item.ID],
		})
	}
	return result, nil
}

func broadcastExternalGlossaryLibraryEvent(payload *externalGlossaryLibraryEventPayload) {
	if payload == nil {
		return
	}
	now := time.Now().UnixMilli()
	options := map[string]any{
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
		Type: protocol.EventExternalGlossariesUpdated,
		Argv: &protocol.Argv{Options: options},
	}
	if userId2ConnInfoGlobal == nil {
		return
	}
	userId2ConnInfoGlobal.Range(func(_ string, conns *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		conns.Range(func(conn *WsSyncConn, _ *ConnInfo) bool {
			_ = conn.WriteJSON(struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{
				Event: *event,
				Op:    protocol.OpEvent,
			})
			return true
		})
		return true
	})
}

func broadcastExternalGlossaryLibraryChanged(libraryIDs []string, operation, requestID string, forceReload bool) {
	cleaned := normalizeStringIDs(libraryIDs)
	if len(cleaned) == 0 {
		return
	}
	broadcastExternalGlossaryLibraryEvent(&externalGlossaryLibraryEventPayload{
		LibraryIDs:  cleaned,
		Operation:   operation,
		RequestID:   requestID,
		ForceReload: forceReload,
	})
	broadcastWorldExternalGlossaryEventToBoundWorlds(cleaned, operation, requestID, forceReload)
}

func ExternalGlossaryLibraryListHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	opts := parseExternalGlossaryLibraryListOptions(c)
	items, total, err := service.ExternalGlossaryLibraryList(user.ID, opts)
	if err != nil {
		return c.Status(mapExternalGlossaryErrorStatus(err)).JSON(fiber.Map{"message": err.Error()})
	}
	listItems, err := buildExternalGlossaryLibraryListItems(items)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	return c.JSON(fiber.Map{
		"items":    listItems,
		"total":    total,
		"page":     opts.Page,
		"pageSize": opts.PageSize,
	})
}

func ExternalGlossaryLibraryCreateHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload service.ExternalGlossaryLibraryInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, createErr := service.ExternalGlossaryLibraryCreate(user.ID, payload)
	if createErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(createErr)).JSON(fiber.Map{"message": createErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{item.ID}, "created", requestID, true)
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item, "requestId": requestID})
}

func ExternalGlossaryLibraryUpdateHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload service.ExternalGlossaryLibraryInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, updateErr := service.ExternalGlossaryLibraryUpdate(c.Params("libraryId"), user.ID, payload)
	if updateErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(updateErr)).JSON(fiber.Map{"message": updateErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{item.ID}, "updated", requestID, true)
	return c.JSON(fiber.Map{"item": item, "requestId": requestID})
}

func ExternalGlossaryLibraryDeleteHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := strings.TrimSpace(c.Params("libraryId"))
	if libraryID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "术语库不存在"})
	}
	if deleteErr := service.ExternalGlossaryLibraryDelete(libraryID, user.ID); deleteErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(deleteErr)).JSON(fiber.Map{"message": deleteErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{libraryID}, "deleted", requestID, true)
	return c.JSON(fiber.Map{"requestId": requestID})
}

func ExternalGlossaryLibraryBulkDeleteHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	cleaned := normalizeStringIDs(payload.IDs)
	count, deleteErr := service.ExternalGlossaryLibraryBulkDelete(user.ID, cleaned)
	if deleteErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(deleteErr)).JSON(fiber.Map{"message": deleteErr.Error()})
	}
	requestID := utils.NewID()
	if count > 0 {
		broadcastExternalGlossaryLibraryChanged(cleaned, "bulk-deleted", requestID, true)
	}
	return c.JSON(fiber.Map{"deleted": count, "requestId": requestID})
}

func ExternalGlossaryLibraryReorderHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload struct {
		Items []service.WorldKeywordReorderItem `json:"items"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	updated, reorderErr := service.ExternalGlossaryLibraryReorder(user.ID, payload.Items)
	if reorderErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(reorderErr)).JSON(fiber.Map{"message": reorderErr.Error()})
	}
	requestID := utils.NewID()
	if updated > 0 {
		libraryIDs := make([]string, 0, len(payload.Items))
		for _, item := range payload.Items {
			libraryIDs = append(libraryIDs, item.ID)
		}
		broadcastExternalGlossaryLibraryChanged(libraryIDs, "reordered", requestID, true)
	}
	return c.JSON(fiber.Map{"updated": updated, "requestId": requestID})
}

func ExternalGlossaryLibraryExportHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := strings.TrimSpace(c.Params("libraryId"))
	var library model.ExternalGlossaryLibraryModel
	if err := model.GetDB().Where("id = ?", libraryID).First(&library).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "术语库不存在"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": err.Error()})
	}
	items, exportErr := service.ExternalGlossaryTermExport(libraryID, user.ID, strings.TrimSpace(c.Query("category")))
	if exportErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(exportErr)).JSON(fiber.Map{"message": exportErr.Error()})
	}
	categories, categoryErr := service.ExternalGlossaryCategoryList(libraryID, user.ID)
	if categoryErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(categoryErr)).JSON(fiber.Map{"message": categoryErr.Error()})
	}
	return c.JSON(externalGlossaryLibraryExportPayload{
		Library:    &library,
		Categories: categories,
		Items:      items,
	})
}

func ExternalGlossaryLibraryImportHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload struct {
		Library *service.ExternalGlossaryLibraryInput `json:"library"`
		Items   []service.WorldKeywordInput           `json:"items"`
		Replace bool                                  `json:"replace"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	if payload.Library == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "术语库信息不能为空"})
	}
	library, createErr := service.ExternalGlossaryLibraryCreate(user.ID, *payload.Library)
	if createErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(createErr)).JSON(fiber.Map{"message": createErr.Error()})
	}
	stats, importErr := service.ExternalGlossaryTermImport(library.ID, user.ID, payload.Items, payload.Replace)
	if importErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(importErr)).JSON(fiber.Map{"message": importErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{library.ID}, "imported", requestID, true)
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"item":      library,
		"stats":     stats,
		"requestId": requestID,
	})
}

func ExternalGlossaryTermListHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := c.Params("libraryId")
	opts := service.WorldKeywordListOptions{
		Page:            parseQueryIntDefault(c, "page", 1),
		PageSize:        parseQueryIntDefault(c, "pageSize", 50),
		Query:           strings.TrimSpace(c.Query("q")),
		Category:        strings.TrimSpace(c.Query("category")),
		IncludeDisabled: c.QueryBool("includeDisabled"),
	}
	items, total, listErr := service.ExternalGlossaryTermList(libraryID, user.ID, opts)
	if listErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(listErr)).JSON(fiber.Map{"message": listErr.Error()})
	}
	return c.JSON(fiber.Map{
		"items":    items,
		"total":    total,
		"page":     opts.Page,
		"pageSize": opts.PageSize,
	})
}

func ExternalGlossaryTermCreateHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := c.Params("libraryId")
	var payload service.WorldKeywordInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, createErr := service.ExternalGlossaryTermCreate(libraryID, user.ID, payload)
	if createErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(createErr)).JSON(fiber.Map{"message": createErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{libraryID}, "term-created", requestID, true)
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item, "requestId": requestID})
}

func ExternalGlossaryTermUpdateHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := c.Params("libraryId")
	termID := c.Params("termId")
	var payload service.WorldKeywordInput
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, updateErr := service.ExternalGlossaryTermUpdate(libraryID, termID, user.ID, payload)
	if updateErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(updateErr)).JSON(fiber.Map{"message": updateErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{libraryID}, "term-updated", requestID, true)
	return c.JSON(fiber.Map{"item": item, "requestId": requestID})
}

func ExternalGlossaryTermDeleteHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := c.Params("libraryId")
	termID := c.Params("termId")
	if deleteErr := service.ExternalGlossaryTermDelete(libraryID, termID, user.ID); deleteErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(deleteErr)).JSON(fiber.Map{"message": deleteErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{libraryID}, "term-deleted", requestID, true)
	return c.JSON(fiber.Map{"requestId": requestID})
}

func ExternalGlossaryTermBulkDeleteHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := c.Params("libraryId")
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	count, deleteErr := service.ExternalGlossaryTermBulkDelete(libraryID, payload.IDs, user.ID)
	if deleteErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(deleteErr)).JSON(fiber.Map{"message": deleteErr.Error()})
	}
	requestID := utils.NewID()
	if count > 0 {
		broadcastExternalGlossaryLibraryChanged([]string{libraryID}, "term-bulk-deleted", requestID, true)
	}
	return c.JSON(fiber.Map{"deleted": count, "requestId": requestID})
}

func ExternalGlossaryTermReorderHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := c.Params("libraryId")
	var payload struct {
		Items []service.WorldKeywordReorderItem `json:"items"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	updated, reorderErr := service.ExternalGlossaryTermReorder(libraryID, user.ID, payload.Items)
	if reorderErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(reorderErr)).JSON(fiber.Map{"message": reorderErr.Error()})
	}
	requestID := utils.NewID()
	if updated > 0 {
		broadcastExternalGlossaryLibraryChanged([]string{libraryID}, "term-reordered", requestID, true)
	}
	return c.JSON(fiber.Map{"updated": updated, "requestId": requestID})
}

func ExternalGlossaryTermImportHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := c.Params("libraryId")
	var payload struct {
		Items   []service.WorldKeywordInput `json:"items"`
		Replace bool                        `json:"replace"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	stats, importErr := service.ExternalGlossaryTermImport(libraryID, user.ID, payload.Items, payload.Replace)
	if importErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(importErr)).JSON(fiber.Map{"message": importErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{libraryID}, "term-imported", requestID, true)
	return c.JSON(fiber.Map{"stats": stats, "requestId": requestID})
}

func ExternalGlossaryTermExportHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	libraryID := c.Params("libraryId")
	items, exportErr := service.ExternalGlossaryTermExport(libraryID, user.ID, strings.TrimSpace(c.Query("category")))
	if exportErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(exportErr)).JSON(fiber.Map{"message": exportErr.Error()})
	}
	return c.JSON(fiber.Map{"items": items})
}

func ExternalGlossaryCategoryListHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	categories, listErr := service.ExternalGlossaryCategoryList(c.Params("libraryId"), user.ID)
	if listErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(listErr)).JSON(fiber.Map{"message": listErr.Error()})
	}
	return c.JSON(fiber.Map{"categories": categories})
}

func ExternalGlossaryCategoryInfoListHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	items, listErr := service.ExternalGlossaryCategoryListInfos(c.Params("libraryId"), user.ID)
	if listErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(listErr)).JSON(fiber.Map{"message": listErr.Error()})
	}
	return c.JSON(fiber.Map{"items": items})
}

func ExternalGlossaryCategoryCreateHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	name, createErr := service.ExternalGlossaryCategoryCreate(c.Params("libraryId"), user.ID, payload.Name)
	if createErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(createErr)).JSON(fiber.Map{"message": createErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{c.Params("libraryId")}, "category-created", requestID, true)
	return c.Status(http.StatusCreated).JSON(fiber.Map{"name": name, "requestId": requestID})
}

func ExternalGlossaryCategoryRenameHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload struct {
		OldName string `json:"oldName"`
		NewName string `json:"newName"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	updated, name, renameErr := service.ExternalGlossaryCategoryRename(c.Params("libraryId"), user.ID, payload.OldName, payload.NewName)
	if renameErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(renameErr)).JSON(fiber.Map{"message": renameErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{c.Params("libraryId")}, "category-renamed", requestID, true)
	return c.JSON(fiber.Map{"updated": updated, "name": name, "requestId": requestID})
}

func ExternalGlossaryCategoryDeleteHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	updated, deleteErr := service.ExternalGlossaryCategoryDelete(c.Params("libraryId"), user.ID, payload.Name)
	if deleteErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(deleteErr)).JSON(fiber.Map{"message": deleteErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{c.Params("libraryId")}, "category-deleted", requestID, true)
	return c.JSON(fiber.Map{"updated": updated, "requestId": requestID})
}

func ExternalGlossaryCategoryPriorityUpdateHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload struct {
		Name     string `json:"name"`
		Priority int    `json:"priority"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	item, updateErr := service.ExternalGlossaryCategoryUpdatePriority(c.Params("libraryId"), user.ID, payload.Name, payload.Priority)
	if updateErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(updateErr)).JSON(fiber.Map{"message": updateErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{c.Params("libraryId")}, "category-priority-updated", requestID, true)
	return c.JSON(fiber.Map{"item": item, "requestId": requestID})
}

func ExternalGlossaryCategoryPriorityBulkUpdateHandler(c *fiber.Ctx) error {
	user, err := ensureExternalGlossaryAdminAPI(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"message": err.Error()})
	}
	var payload struct {
		Items []service.KeywordCategoryPriorityUpdate `json:"items"`
	}
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": "参数错误"})
	}
	updated, updateErr := service.ExternalGlossaryCategoryBulkUpdatePriority(c.Params("libraryId"), user.ID, payload.Items)
	if updateErr != nil {
		return c.Status(mapExternalGlossaryErrorStatus(updateErr)).JSON(fiber.Map{"message": updateErr.Error()})
	}
	requestID := utils.NewID()
	broadcastExternalGlossaryLibraryChanged([]string{c.Params("libraryId")}, "category-priority-bulk-updated", requestID, true)
	return c.JSON(fiber.Map{"updated": updated, "requestId": requestID})
}
