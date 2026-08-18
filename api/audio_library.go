package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

func AudioLibrarySettingsGet(c *fiber.Ctx) error {
	setAudioLibraryNoCache(c)
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	worldID, err := requireAudioLibraryWorld(c, false)
	if err != nil {
		return err
	}
	isSystemAdmin := pm.CanWithSystemRole(user.ID, pm.PermModAdmin)
	canConfigure := isSystemAdmin || (utils.GetConfig() != nil && utils.GetConfig().Audio.AllowWorldAudioWorkbench && service.IsWorldAdmin(worldID, user.ID))
	return c.JSON(service.AudioLibrarySettingsGet(worldID, canConfigure))
}

func AudioLibrarySettingsPut(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	var req struct {
		WorldID       string                 `json:"worldId"`
		Mode          utils.AudioLibraryMode `json:"mode"`
		Prefix        string                 `json:"prefix"`
		SelectorDepth *int                   `json:"selectorDepth"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "请求格式无效")
	}
	worldID := strings.TrimSpace(req.WorldID)
	if worldID == "" {
		worldID = c.Query("worldId")
	}
	resolvedWorldID, resolveErr := service.ResolveAudioLibraryWorldID(worldID)
	if resolveErr != nil {
		return audioLibraryError(c, resolveErr)
	}
	isSystemAdmin := pm.CanWithSystemRole(user.ID, pm.PermModAdmin)
	if !isSystemAdmin && (utils.GetConfig() == nil || !utils.GetConfig().Audio.AllowWorldAudioWorkbench) {
		return fiber.ErrForbidden
	}
	if !isSystemAdmin && !service.IsWorldAdmin(resolvedWorldID, user.ID) {
		return fiber.ErrForbidden
	}
	selectorDepth := service.AudioLibrarySettingsGet(resolvedWorldID, true).SelectorDepth
	if req.SelectorDepth != nil {
		selectorDepth = *req.SelectorDepth
	}
	settings, err := service.AudioLibrarySaveSettings(resolvedWorldID, user.ID, req.Mode, req.Prefix, selectorDepth)
	if err != nil {
		return audioLibraryError(c, err)
	}
	return c.JSON(settings)
}

func AudioLibraryS3Prefixes(c *fiber.Ctx) error {
	setAudioLibraryNoCache(c)
	worldID, err := requireAudioLibraryWorld(c, false)
	if err != nil {
		return err
	}
	result, err := service.ListAudioLibraryPrefixes(worldID, c.Query("prefix"), c.Query("cursor"), c.QueryInt("limit", 100))
	if err != nil {
		return audioLibraryError(c, err)
	}
	return c.JSON(result)
}

func AudioLibraryS3Assets(c *fiber.Ctx) error {
	setAudioLibraryNoCache(c)
	worldID, err := requireAudioLibraryWorld(c, false)
	if err != nil {
		return err
	}
	if !service.AudioLibraryModeIsS3(worldID) {
		return fiber.NewError(http.StatusConflict, "音频素材库当前为 database 模式")
	}
	result, err := service.ListAudioLibraryAssets(worldID, c.Query("prefix"), c.Query("cursor"), c.QueryInt("limit", 100))
	if err != nil {
		return audioLibraryError(c, err)
	}
	return c.JSON(result)
}

func AudioLibraryS3SelectorAssets(c *fiber.Ctx) error {
	setAudioLibraryNoCache(c)
	worldID, err := requireAudioLibraryWorld(c, false)
	if err != nil {
		return err
	}
	if !service.AudioLibraryModeIsS3(worldID) {
		return fiber.NewError(http.StatusConflict, "音频素材库当前为 database 模式")
	}
	depth := c.QueryInt("depth", -1)
	if depth < 0 {
		depth = service.AudioLibrarySettingsGet(worldID, false).SelectorDepth
	}
	result, err := service.ListAudioLibrarySelectorAssets(worldID, c.Query("prefix"), depth, c.QueryInt("limit", 1000))
	if err != nil {
		return audioLibraryError(c, err)
	}
	return c.JSON(result)
}

func AudioLibraryResolveAssets(c *fiber.Ctx) error {
	var req struct {
		WorldID string   `json:"worldId"`
		Refs    []string `json:"refs"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "请求格式无效")
	}
	worldIDRaw := strings.TrimSpace(req.WorldID)
	if worldIDRaw == "" {
		worldIDRaw = c.Query("worldId")
	}
	worldID, resolveErr := service.ResolveAudioLibraryWorldID(worldIDRaw)
	if resolveErr != nil {
		return audioLibraryError(c, resolveErr)
	}
	user := getCurUser(c)
	if user == nil || (!service.AudioLibraryCanAccessWorld(worldID, user.ID) && !pm.CanWithSystemRole(user.ID, pm.PermModAdmin)) {
		return fiber.ErrForbidden
	}
	items, err := service.ResolveAudioLibraryAssets(worldID, req.Refs)
	if err != nil {
		return audioLibraryError(c, err)
	}
	return c.JSON(fiber.Map{"items": items})
}

func AudioLibraryUpload(c *fiber.Ctx) error {
	worldID, err := requireAudioLibraryOperator(c)
	if err != nil {
		return err
	}
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(http.StatusBadRequest, "缺少音频文件")
	}
	item, err := service.UploadAudioLibraryAsset(worldID, file, c.FormValue("prefix"))
	if err != nil {
		return audioLibraryError(c, err)
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item})
}

func AudioLibraryFolderCreate(c *fiber.Ctx) error {
	worldID, err := requireAudioLibraryOperator(c)
	if err != nil {
		return err
	}
	var req struct {
		Prefix string `json:"prefix"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "请求格式无效")
	}
	item, err := service.AudioLibraryCreateFolder(worldID, req.Prefix)
	if err != nil {
		return audioLibraryError(c, err)
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item})
}

func AudioLibraryFolderDelete(c *fiber.Ctx) error {
	worldID, err := requireAudioLibraryOperator(c)
	if err != nil {
		return err
	}
	var req struct {
		Prefix string `json:"prefix"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "请求格式无效")
	}
	if err := service.AudioLibraryDeleteFolder(worldID, req.Prefix); err != nil {
		return audioLibraryError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func AudioLibraryAssetUpdate(c *fiber.Ctx) error {
	worldID, err := requireAudioLibraryOperator(c)
	if err != nil {
		return err
	}
	var req struct {
		Ref          string `json:"ref"`
		Name         string `json:"name"`
		TargetPrefix string `json:"targetPrefix"`
		ExpectedETag string `json:"expectedEtag"`
		ContentType  string `json:"contentType"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "请求格式无效")
	}
	var item service.AudioLibraryAsset
	if strings.TrimSpace(req.ContentType) != "" {
		item, err = service.AudioLibraryUpdateContentType(worldID, req.Ref, req.ContentType, req.ExpectedETag)
		if err != nil {
			return audioLibraryError(c, err)
		}
		// Content-Type and rename/move can be submitted together. Use the
		// fresh ETag returned by metadata replacement for the subsequent copy.
		req.Ref = item.Ref
		req.ExpectedETag = item.ETag
	}
	if strings.TrimSpace(req.Name) != "" || strings.TrimSpace(req.TargetPrefix) != "" || item.Ref == "" {
		item, err = service.AudioLibraryMoveAsset(worldID, req.Ref, req.TargetPrefix, req.Name, req.ExpectedETag)
		if err != nil {
			return audioLibraryError(c, err)
		}
	}
	return c.JSON(fiber.Map{"item": item})
}

func AudioLibraryAssetDelete(c *fiber.Ctx) error {
	worldID, err := requireAudioLibraryOperator(c)
	if err != nil {
		return err
	}
	var req struct {
		Ref          string `json:"ref"`
		ExpectedETag string `json:"expectedEtag"`
		ForceDetach  bool   `json:"forceDetach"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "请求格式无效")
	}
	if err := service.AudioLibraryDeleteAsset(worldID, req.Ref, req.ExpectedETag, req.ForceDetach); err != nil {
		return audioLibraryError(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func AudioLibraryPlayToken(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return fiber.ErrUnauthorized
	}
	var req struct {
		WorldID string `json:"worldId"`
		Ref     string `json:"ref"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(http.StatusBadRequest, "请求格式无效")
	}
	worldIDRaw := strings.TrimSpace(req.WorldID)
	if worldIDRaw == "" {
		worldIDRaw = c.Query("worldId")
	}
	worldID, resolveErr := service.ResolveAudioLibraryWorldID(worldIDRaw)
	if resolveErr != nil {
		return audioLibraryError(c, resolveErr)
	}
	if !service.AudioLibraryCanAccessWorld(worldID, user.ID) && !pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return fiber.ErrForbidden
	}
	grant, streamURL, err := service.AudioLibraryPlayToken(worldID, user.ID, req.Ref)
	if err != nil {
		return audioLibraryError(c, err)
	}
	return c.JSON(fiber.Map{"streamUrl": streamURL, "expiresAt": grant.ExpiresAt.UnixMilli()})
}

func audioLibraryError(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	status := http.StatusInternalServerError
	var conflict *service.AudioLibraryConflictError
	if errors.As(err, &conflict) {
		status = http.StatusConflict
		err = conflict
	}
	if errors.Is(err, service.ErrAudioLibraryUnavailable) {
		status = http.StatusServiceUnavailable
	}
	if errors.Is(err, service.ErrAudioLibraryNotFound) {
		status = http.StatusNotFound
	}
	if errors.Is(err, service.ErrAudioLibraryPermission) {
		status = http.StatusForbidden
	}
	if errors.Is(err, service.ErrWorldNotFound) {
		status = http.StatusNotFound
	}
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		status = fiberErr.Code
		err = errors.New(fiberErr.Message)
	}
	// Service validation errors must be JSON too; Fiber's default error
	// handler otherwise emits plain text, which breaks Axios JSON parsing.
	if status == http.StatusInternalServerError && isAudioLibraryValidationError(err) {
		status = http.StatusBadRequest
	}
	return c.Status(status).JSON(fiber.Map{"message": err.Error()})
}

func setAudioLibraryNoCache(c *fiber.Ctx) {
	c.Set(fiber.HeaderCacheControl, "no-store, no-cache, must-revalidate, max-age=0")
	c.Set(fiber.HeaderPragma, "no-cache")
}

func isAudioLibraryValidationError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"s3 prefix",
		"audio ref",
		"object key",
		"文件名",
		"content-type 不能为空",
		"audio asset is still referenced",
		"audio folder is still referenced",
	} {
		if strings.Contains(message, strings.ToLower(marker)) {
			return true
		}
	}
	return errors.Is(err, service.ErrAudioUnsupportedMime)
}

func requireAudioLibraryOperator(c *fiber.Ctx) (string, error) {
	user := getCurUser(c)
	if user == nil {
		return "", fiber.ErrUnauthorized
	}
	return requireAudioLibraryWorld(c, true)
}

func requireAudioLibraryWorld(c *fiber.Ctx, requireAdmin bool) (string, error) {
	user := getCurUser(c)
	if user == nil {
		return "", fiber.ErrUnauthorized
	}
	worldIDRaw := strings.TrimSpace(c.Query("worldId"))
	if worldIDRaw == "" {
		worldIDRaw = strings.TrimSpace(c.FormValue("worldId"))
	}
	worldID, err := service.ResolveAudioLibraryWorldID(worldIDRaw)
	if err != nil {
		return "", audioLibraryError(c, err)
	}
	if pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return worldID, nil
	}
	if !service.AudioLibraryCanAccessWorld(worldID, user.ID) {
		return "", fiber.ErrForbidden
	}
	if requireAdmin && !service.IsWorldAdmin(worldID, user.ID) {
		return "", fiber.ErrForbidden
	}
	return worldID, nil
}
