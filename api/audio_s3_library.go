package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/pm"
	"sealchat/service"
)

func AudioS3LibrarySettingsGet(c *fiber.Ctx) error {
	settings := service.GetAudioS3LibrarySettings()
	user := getCurUser(c)
	canConfigure := user != nil && pm.CanWithSystemRole(user.ID, pm.PermModAdmin)
	return c.JSON(fiber.Map{
		"enabled":      settings.Enabled,
		"prefix":       settings.Prefix,
		"available":    settings.Available,
		"bucket":       settings.Bucket,
		"canConfigure": canConfigure,
	})
}

func AudioS3LibrarySettingsPut(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil || !pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "仅平台管理员可配置 S3 音频素材库")
	}
	var req struct {
		Enabled bool   `json:"enabled"`
		Prefix  string `json:"prefix"`
	}
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "S3 模式配置格式错误")
	}
	settings, err := service.SaveAudioS3LibrarySettings(req.Enabled, req.Prefix)
	if err != nil {
		return audioS3LibraryError(c, err, "保存 S3 模式失败")
	}
	return c.JSON(fiber.Map{
		"enabled":      settings.Enabled,
		"prefix":       settings.Prefix,
		"available":    settings.Available,
		"bucket":       settings.Bucket,
		"canConfigure": true,
	})
}

func AudioS3LibraryBrowse(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil || !pm.CanWithSystemRole(user.ID, pm.PermModAdmin) {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "仅平台管理员可浏览 S3 根目录")
	}
	result, err := service.AudioS3Browse(c.Context(), c.Query("prefix"))
	if err != nil {
		return audioS3LibraryError(c, err, "读取 S3 目录失败")
	}
	return c.JSON(result)
}

func AudioS3LibraryAssetList(c *fiber.Ctx) error {
	recursive := c.QueryBool("recursive", false)
	result, err := service.AudioS3ListAssets(
		c.Context(),
		strings.TrimSpace(c.Query("folderId")),
		strings.TrimSpace(c.Query("query")),
		recursive,
		c.QueryInt("page", 1),
		c.QueryInt("pageSize", 20),
		strings.TrimSpace(c.Query("sortBy")),
		strings.TrimSpace(c.Query("sortOrder")),
	)
	if err != nil {
		return audioS3LibraryError(c, err, "读取 S3 音频素材失败")
	}
	return c.JSON(result)
}

func AudioS3LibraryFolderList(c *fiber.Ctx) error {
	items, err := service.AudioS3ListFolders(c.Context())
	if err != nil {
		return audioS3LibraryError(c, err, "读取 S3 文件夹失败")
	}
	return c.JSON(fiber.Map{"items": items})
}

func AudioS3LibraryAssetGet(c *fiber.Ctx) error {
	item, err := service.AudioS3GetAsset(c.Context(), c.Params("id"))
	if err != nil {
		return audioS3LibraryError(c, err, "读取 S3 音频素材失败")
	}
	return c.JSON(item)
}

func AudioS3LibraryAssetPlayToken(c *fiber.Ctx) error {
	target, expiresAt, err := service.AudioS3PresignedURL(c.Context(), c.Params("id"))
	if err != nil {
		return audioS3LibraryError(c, err, "生成 S3 播放地址失败")
	}
	return c.JSON(fiber.Map{
		"streamUrl": target,
		"expiresAt": expiresAt.UnixMilli(),
	})
}

func AudioS3LibraryStream(c *fiber.Ctx) error {
	target, _, err := service.AudioS3PresignedURL(c.Context(), c.Params("id"))
	if err != nil {
		return audioS3LibraryError(c, err, "生成 S3 播放地址失败")
	}
	return c.Redirect(target, fiber.StatusTemporaryRedirect)
}

func AudioS3LibraryAssetUpload(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "未找到上传文件")
	}
	item, err := service.AudioS3UploadAsset(c.Context(), file, strings.TrimSpace(c.FormValue("folderId")))
	if err != nil {
		return audioS3LibraryError(c, err, "上传 S3 音频素材失败")
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item, "status": "success"})
}

func AudioS3LibraryAssetUpdate(c *fiber.Ctx) error {
	var req struct {
		Name     string  `json:"name"`
		FolderID *string `json:"folderId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "S3 素材更新格式错误")
	}
	item, err := service.AudioS3UpdateAsset(c.Context(), c.Params("id"), service.AudioS3AssetUpdateInput{
		Name:     req.Name,
		FolderID: req.FolderID,
	})
	if err != nil {
		return audioS3LibraryError(c, err, "更新 S3 音频素材失败")
	}
	return c.JSON(fiber.Map{"item": item})
}

func AudioS3LibraryAssetDelete(c *fiber.Ctx) error {
	impact, err := service.AudioS3DeleteAsset(c.Context(), c.Params("id"), c.QueryBool("forceDetach"))
	if err != nil {
		var referenced *service.AudioAssetReferencedError
		if errors.As(err, &referenced) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"message": "素材仍被引用，无法安全删除",
				"usage":   referenced.Summary,
			})
		}
		return audioS3LibraryError(c, err, "删除 S3 音频素材失败")
	}
	return c.JSON(fiber.Map{"message": "已删除", "impact": impact})
}

func AudioS3LibraryFolderCreate(c *fiber.Ctx) error {
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parentId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "S3 文件夹创建格式错误")
	}
	item, err := service.AudioS3CreateFolder(c.Context(), req.ParentID, req.Name)
	if err != nil {
		return audioS3LibraryError(c, err, "创建 S3 文件夹失败")
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{"item": item})
}

func AudioS3LibraryFolderUpdate(c *fiber.Ctx) error {
	var req struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parentId"`
	}
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "S3 文件夹更新格式错误")
	}
	item, err := service.AudioS3UpdateFolder(c.Context(), c.Params("id"), service.AudioS3FolderUpdateInput{
		Name:     req.Name,
		ParentID: req.ParentID,
	})
	if err != nil {
		return audioS3LibraryError(c, err, "更新 S3 文件夹失败")
	}
	return c.JSON(fiber.Map{"item": item})
}

func AudioS3LibraryFolderDelete(c *fiber.Ctx) error {
	impact, err := service.AudioS3DeleteFolder(c.Context(), c.Params("id"), c.QueryBool("forceDetach"))
	if err != nil {
		var referenced *service.AudioAssetReferencedError
		if errors.As(err, &referenced) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"message": "文件夹内仍有被引用的素材，无法安全删除",
				"usage":   referenced.Summary,
			})
		}
		return audioS3LibraryError(c, err, "删除 S3 文件夹失败")
	}
	return c.JSON(fiber.Map{"message": "已删除", "impact": impact})
}

func audioS3LibraryError(c *fiber.Ctx, err error, fallback string) error {
	switch {
	case errors.Is(err, service.ErrAudioS3Unavailable):
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "S3 存储尚未启用或配置不完整")
	case errors.Is(err, service.ErrAudioS3InvalidPath):
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "S3 路径无效")
	case errors.Is(err, service.ErrAudioS3Exists):
		return wrapErrorStatus(c, fiber.StatusConflict, err, "目标 S3 对象已存在")
	case errors.Is(err, service.ErrAudioS3NotFound):
		return wrapErrorStatus(c, fiber.StatusNotFound, err, "S3 对象不存在")
	case errors.Is(err, service.ErrAudioS3ForeignObject):
		return wrapErrorStatus(c, fiber.StatusConflict, err, "目标文件夹包含非音频对象，为避免误操作已拒绝")
	case errors.Is(err, service.ErrAudioTooLarge):
		return wrapErrorStatus(c, fiber.StatusRequestEntityTooLarge, err, err.Error())
	case errors.Is(err, service.ErrAudioUnsupportedMime):
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "不支持的音频格式")
	default:
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, fallback)
	}
}
