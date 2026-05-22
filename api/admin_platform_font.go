package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

func AdminPlatformFontListHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	items, total, err := service.PlatformFontList(user.ID, service.PlatformFontListOptions{
		Query:           c.Query("query"),
		IncludeDisabled: c.QueryBool("includeDisabled"),
		Page:            c.QueryInt("page", 1),
		PageSize:        c.QueryInt("pageSize", 50),
	})
	if err != nil {
		if errors.Is(err, service.ErrPlatformFontPermission) {
			return wrapErrorStatus(c, fiber.StatusForbidden, err, "无权限访问平台字体")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "加载平台字体列表失败")
	}
	return c.JSON(fiber.Map{
		"items":    items,
		"total":    total,
		"page":     c.QueryInt("page", 1),
		"pageSize": c.QueryInt("pageSize", 50),
	})
}

func AdminPlatformFontCreateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	file, err := c.FormFile("file")
	if err != nil {
		status := fiber.StatusBadRequest
		message := "未找到上传文件"
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			status = fiber.StatusRequestEntityTooLarge
			message = "字体文件超过服务器上传限制"
		}
		return wrapErrorStatus(c, status, err, message)
	}
	item, err := service.PlatformFontCreateFromUpload(file, service.PlatformFontCreateInput{
		DisplayName: c.FormValue("displayName"),
		Family:      c.FormValue("family"),
		Weight:      c.FormValue("weight"),
		Style:       c.FormValue("style"),
		PreviewText: c.FormValue("previewText"),
		CreatedBy:   user.ID,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPlatformFontPermission):
			return wrapErrorStatus(c, fiber.StatusForbidden, err, "无权限上传平台字体")
		case errors.Is(err, service.ErrPlatformFontInvalid):
			return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
		default:
			return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "创建平台字体失败")
		}
	}
	return c.Status(http.StatusCreated).JSON(fiber.Map{
		"item": item,
	})
}

func AdminPlatformFontGetHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	item, err := service.PlatformFontGet(c.Params("id"))
	if err != nil {
		if errors.Is(err, service.ErrPlatformFontNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "平台字体不存在")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "加载平台字体详情失败")
	}
	if err := ensureAdminPlatformFontAccess(user.ID); err != nil {
		return wrapErrorStatus(c, fiber.StatusForbidden, err, "无权限访问平台字体")
	}
	return c.JSON(fiber.Map{"item": item})
}

func AdminPlatformFontUpdateHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	var req struct {
		DisplayName  *string                        `json:"displayName"`
		Family       *string                        `json:"family"`
		Weight       *string                        `json:"weight"`
		Style        *string                        `json:"style"`
		PreviewText  *string                        `json:"previewText"`
		Status       *model.PlatformFontStatus      `json:"status"`
		DeliveryMode *model.PlatformFontDeliveryMode `json:"deliveryMode"`
		LastError    *string                        `json:"lastError"`
	}
	if err := c.BodyParser(&req); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求参数解析失败")
	}
	item, err := service.PlatformFontUpdate(c.Params("id"), service.PlatformFontUpdateInput{
		DisplayName:  req.DisplayName,
		Family:       req.Family,
		Weight:       req.Weight,
		Style:        req.Style,
		PreviewText:  req.PreviewText,
		Status:       req.Status,
		DeliveryMode: req.DeliveryMode,
		LastError:    req.LastError,
		UpdatedBy:    user.ID,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPlatformFontPermission):
			return wrapErrorStatus(c, fiber.StatusForbidden, err, "无权限修改平台字体")
		case errors.Is(err, service.ErrPlatformFontNotFound):
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "平台字体不存在")
		case errors.Is(err, service.ErrPlatformFontInvalid):
			return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
		default:
			return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "更新平台字体失败")
		}
	}
	return c.JSON(fiber.Map{"item": item})
}

func AdminPlatformFontDeleteHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	err := service.PlatformFontDelete(c.Params("id"), user.ID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPlatformFontPermission):
			return wrapErrorStatus(c, fiber.StatusForbidden, err, "无权限删除平台字体")
		case errors.Is(err, service.ErrPlatformFontNotFound):
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "平台字体不存在")
		default:
			return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "删除平台字体失败")
		}
	}
	return c.JSON(fiber.Map{"success": true})
}

func AdminPlatformFontSubsetPackageHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	if err := ensureAdminPlatformFontAccess(user.ID); err != nil {
		return wrapErrorStatus(c, fiber.StatusForbidden, err, "无权限上传字体分片包")
	}
	manifestRaw := strings.TrimSpace(c.FormValue("manifest"))
	if manifestRaw == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少分片清单 manifest")
	}
	var manifest service.PlatformFontSubsetManifestData
	if err := json.Unmarshal([]byte(manifestRaw), &manifest); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "分片清单解析失败")
	}
	form, err := c.MultipartForm()
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "读取上传分片失败")
	}
	fileHeaders := form.File["files"]
	if len(fileHeaders) == 0 {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少分片产物文件")
	}
	tempDir := "./data/temp"
	if appCfg := utils.GetConfig(); appCfg != nil && strings.TrimSpace(appCfg.Storage.Local.TempDir) != "" {
		tempDir = appCfg.Storage.Local.TempDir
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "创建临时分片目录失败")
	}
	files := make([]service.PlatformFontSubsetUploadFile, 0, len(fileHeaders))
	tempFiles := make([]string, 0, len(fileHeaders))
	defer func() {
		for _, path := range tempFiles {
			_ = os.Remove(path)
		}
	}()
	for _, fileHeader := range fileHeaders {
		src, openErr := fileHeader.Open()
		if openErr != nil {
			return wrapErrorStatus(c, fiber.StatusBadRequest, openErr, "读取分片文件失败")
		}
		tempFile, createErr := os.CreateTemp(tempDir, "*.platform-font-subset")
		if createErr != nil {
			_ = src.Close()
			return wrapErrorStatus(c, fiber.StatusInternalServerError, createErr, "创建临时分片文件失败")
		}
		if _, copyErr := tempFile.ReadFrom(src); copyErr != nil {
			_ = src.Close()
			_ = tempFile.Close()
			return wrapErrorStatus(c, fiber.StatusInternalServerError, copyErr, "写入临时分片文件失败")
		}
		_ = src.Close()
		if closeErr := tempFile.Close(); closeErr != nil {
			return wrapErrorStatus(c, fiber.StatusInternalServerError, closeErr, "关闭临时分片文件失败")
		}
		tempFiles = append(tempFiles, tempFile.Name())
		files = append(files, service.PlatformFontSubsetUploadFile{
			Name:        fileHeader.Filename,
			LocalPath:   tempFile.Name(),
			ContentType: fileHeader.Header.Get("Content-Type"),
		})
	}
	item, err := service.PlatformFontSaveSubsetPackage(c.Params("id"), service.PlatformFontSubsetPackageInput{
		ActorID:  user.ID,
		Manifest: manifest,
		Files:    files,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPlatformFontPermission):
			return wrapErrorStatus(c, fiber.StatusForbidden, err, "无权限上传字体分片包")
		case errors.Is(err, service.ErrPlatformFontNotFound):
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "平台字体不存在")
		case errors.Is(err, service.ErrPlatformFontInvalid):
			return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
		default:
			return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "保存字体分片包失败")
		}
	}
	return c.JSON(fiber.Map{"item": item})
}

func AdminPlatformFontSplitRuntimeAssetHandler(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return wrapErrorStatus(c, fiber.StatusUnauthorized, nil, "未登录")
	}
	if err := ensureAdminPlatformFontAccess(user.ID); err != nil {
		return wrapErrorStatus(c, fiber.StatusForbidden, err, "无权限访问字体分割运行时")
	}
	name := strings.TrimSpace(c.Params("*"))
	if name == "" {
		return wrapErrorStatus(c, fiber.StatusBadRequest, nil, "缺少运行时资源名称")
	}
	path, err := service.ResolveBundledPlatformFontRuntimeFile(name)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusNotFound, err, "字体分割运行时资源不存在")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "字体分割运行时资源不存在")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取字体分割运行时资源失败")
	}
	setPlatformFontResponseHeaders(c, "", filepath.Base(path))
	return c.SendFile(path)
}

func ensureAdminPlatformFontAccess(userID string) error {
	_, _, err := service.PlatformFontList(userID, service.PlatformFontListOptions{Page: 1, PageSize: 1})
	return err
}
