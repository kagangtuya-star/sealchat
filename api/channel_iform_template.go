package api

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	builtinassets "sealchat/builtin"
	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
)

type channelIFormTemplateCatalogItem struct {
	Ref         string `json:"ref"`
	Origin      string `json:"origin"`
	Name        string `json:"name"`
	Description string `json:"description"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Installable bool   `json:"installable"`
	Archived    bool   `json:"archived"`
	Enabled     bool   `json:"enabled"`
	Editable    bool   `json:"editable"`
	ReadOnly    bool   `json:"readOnly"`
	References  int64  `json:"references,omitempty"`
}

func ChannelIFormTemplateCatalog(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("pageSize", 30)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 30
	}
	if pageSize > 100 {
		pageSize = 100
	}
	search := strings.ToLower(strings.TrimSpace(c.Query("search")))
	originFilter := strings.TrimSpace(c.Query("origin"))
	items := make([]channelIFormTemplateCatalogItem, 0)
	if originFilter == "" || originFilter == "builtin" {
		for _, registration := range service.BuiltinChannelIFormTools() {
			manifest, err := service.LoadBuiltinChannelIFormManifest(registration.Key)
			if err != nil {
				continue
			}
			if !catalogSearchMatch(search, manifest.Name, manifest.Description) {
				continue
			}
			items = append(items, channelIFormTemplateCatalogItem{
				Ref: "builtin:" + registration.Key, Origin: "builtin", Name: manifest.Name,
				Description: manifest.Description, Installable: true, Enabled: true,
				Editable: false, ReadOnly: true,
			})
		}
	}
	if originFilter == "" || originFilter == "platform" {
		isAdmin := CanWithSystemRole(c, pm.PermModAdmin)
		referenceCounts := map[string]int64{}
		if isAdmin {
			var counts []struct {
				TemplateRef string
				Count       int64 `gorm:"column:reference_count"`
			}
			if err := model.GetDB().Model(&model.ChannelIFormModel{}).
				Select("template_ref, COUNT(*) AS reference_count").
				Where("template_ref LIKE ?", "platform:%").
				Group("template_ref").Scan(&counts).Error; err != nil {
				return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取频道嵌入模板引用数量失败")
			}
			for _, count := range counts {
				referenceCounts[count.TemplateRef] = count.Count
			}
		}
		var templates []model.ChannelIFormTemplateModel
		query := model.GetDB().Order("updated_at DESC")
		if err := query.Find(&templates).Error; err != nil {
			return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取频道嵌入模板失败")
		}
		for _, template := range templates {
			if !isAdmin && (!template.Enabled || template.Archived) {
				continue
			}
			if !catalogSearchMatch(search, template.Name, template.Description) {
				continue
			}
			updatedAt := ""
			if !template.UpdatedAt.IsZero() {
				updatedAt = template.UpdatedAt.UTC().Format(time.RFC3339Nano)
			}
			items = append(items, channelIFormTemplateCatalogItem{
				Ref: "platform:" + template.ID, Origin: "platform", Name: template.Name,
				Description: template.Description, Installable: template.Enabled && !template.Archived,
				UpdatedAt: updatedAt,
				Archived:  template.Archived, Enabled: template.Enabled, Editable: isAdmin,
				References: referenceCounts["platform:"+template.ID],
			})
		}
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return c.JSON(fiber.Map{"items": items[start:end], "page": page, "pageSize": pageSize, "total": total})
}

func catalogSearchMatch(search string, values ...string) bool {
	if search == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), search) {
			return true
		}
	}
	return false
}

type channelIFormTemplateWriteRequest struct {
	Name             string                         `json:"name"`
	Description      string                         `json:"description"`
	Url              string                         `json:"url"`
	EmbedCode        string                         `json:"embedCode"`
	DefaultWidth     int                            `json:"defaultWidth"`
	DefaultHeight    int                            `json:"defaultHeight"`
	DefaultCollapsed bool                           `json:"defaultCollapsed"`
	DefaultFloating  bool                           `json:"defaultFloating"`
	AllowPopout      bool                           `json:"allowPopout"`
	MediaOptions     model.ChannelIFormMediaOptions `json:"mediaOptions"`
	BridgePolicy     model.ChannelIFormBridgePolicy `json:"bridgePolicy"`
	Enabled          *bool                          `json:"enabled"`
}

func requirePlatformAdmin(c *fiber.Ctx) error {
	if !CanWithSystemRole(c, pm.PermModAdmin) {
		return wrapErrorStatus(c, fiber.StatusForbidden, nil, "没有平台管理权限")
	}
	return nil
}

func AdminChannelIFormTemplateList(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	return ChannelIFormTemplateCatalog(c)
}

func AdminChannelIFormTemplateGet(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	var template model.ChannelIFormTemplateModel
	if err := model.GetDB().Where("id = ?", strings.TrimSpace(c.Params("templateId"))).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "模板不存在")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取频道嵌入模板失败")
	}
	return c.JSON(fiber.Map{"item": template})
}

func AdminChannelIFormBuiltinGet(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	key := strings.TrimSpace(c.Params("key"))
	_, ok := service.BuiltinChannelIFormTool(key)
	if !ok {
		return wrapErrorStatus(c, fiber.StatusNotFound, nil, "内置工具不存在")
	}
	manifest, err := service.LoadBuiltinChannelIFormManifest(key)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取内置工具元数据失败")
	}
	return c.JSON(fiber.Map{"item": fiber.Map{
		"ref": "builtin:" + key, "origin": "builtin", "name": manifest.Name,
		"description": manifest.Description, "entry": manifest.Entry,
		"installable": true, "editable": false, "readOnly": true,
		"defaultWidth": manifest.DefaultWidth, "defaultHeight": manifest.DefaultHeight,
	}})
}

func AdminChannelIFormTemplateCreate(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	var payload channelIFormTemplateWriteRequest
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	if err := validateChannelIFormTemplateWrite(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	normalizedURL, err := normalizeURL(payload.Url)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	user := getCurUser(c)
	template := &model.ChannelIFormTemplateModel{
		Name: strings.TrimSpace(payload.Name), Description: strings.TrimSpace(payload.Description),
		Url: normalizedURL, EmbedCode: strings.TrimSpace(payload.EmbedCode),
		DefaultWidth: payload.DefaultWidth, DefaultHeight: payload.DefaultHeight,
		DefaultCollapsed: payload.DefaultCollapsed, DefaultFloating: payload.DefaultFloating,
		AllowPopout: payload.AllowPopout, MediaOptions: normalizeMediaOptions(payload.MediaOptions), BridgePolicy: normalizeBridgePolicy(&payload.BridgePolicy),
		Enabled: true, CreatedBy: user.ID, UpdatedBy: user.ID,
	}
	if payload.Enabled != nil {
		template.Enabled = *payload.Enabled
	}
	template.Normalize()
	if err := model.GetDB().Create(template).Error; err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "创建频道嵌入模板失败")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"item": template})
}

func AdminChannelIFormTemplateUpdate(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	id := strings.TrimSpace(c.Params("templateId"))
	var template model.ChannelIFormTemplateModel
	if err := model.GetDB().Where("id = ?", id).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "模板不存在")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取频道嵌入模板失败")
	}
	enabled := template.Enabled
	payload := channelIFormTemplateWriteRequest{
		Name: template.Name, Description: template.Description, Url: template.Url, EmbedCode: template.EmbedCode,
		DefaultWidth: template.DefaultWidth, DefaultHeight: template.DefaultHeight,
		DefaultCollapsed: template.DefaultCollapsed, DefaultFloating: template.DefaultFloating,
		AllowPopout: template.AllowPopout, MediaOptions: template.MediaOptions, BridgePolicy: template.BridgePolicy,
		Enabled: &enabled,
	}
	if err := c.BodyParser(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, "请求体解析失败")
	}
	if err := validateChannelIFormTemplateWrite(&payload); err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	normalizedURL, err := normalizeURL(payload.Url)
	if err != nil {
		return wrapErrorStatus(c, fiber.StatusBadRequest, err, err.Error())
	}
	user := getCurUser(c)
	template.Name, template.Description = strings.TrimSpace(payload.Name), strings.TrimSpace(payload.Description)
	template.Url, template.EmbedCode = normalizedURL, strings.TrimSpace(payload.EmbedCode)
	template.DefaultWidth, template.DefaultHeight = payload.DefaultWidth, payload.DefaultHeight
	template.DefaultCollapsed, template.DefaultFloating = payload.DefaultCollapsed, payload.DefaultFloating
	template.AllowPopout, template.MediaOptions, template.BridgePolicy = payload.AllowPopout, normalizeMediaOptions(payload.MediaOptions), normalizeBridgePolicy(&payload.BridgePolicy)
	if payload.Enabled != nil {
		template.Enabled = *payload.Enabled
	}
	template.UpdatedBy = user.ID
	template.Normalize()
	if err := model.GetDB().Save(&template).Error; err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "更新频道嵌入模板失败")
	}
	return c.JSON(fiber.Map{"item": template})
}

func AdminChannelIFormTemplateArchive(c *fiber.Ctx) error {
	return setChannelIFormTemplateArchived(c, true)
}

func AdminChannelIFormTemplateRestore(c *fiber.Ctx) error {
	return setChannelIFormTemplateArchived(c, false)
}

func setChannelIFormTemplateArchived(c *fiber.Ctx, archived bool) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	id := strings.TrimSpace(c.Params("templateId"))
	var template model.ChannelIFormTemplateModel
	if err := model.GetDB().Where("id = ?", id).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "模板不存在")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取频道嵌入模板失败")
	}
	template.Archived = archived
	template.UpdatedBy = getCurUser(c).ID
	if err := model.GetDB().Save(&template).Error; err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "更新模板状态失败")
	}
	return c.JSON(fiber.Map{"item": template})
}

func AdminChannelIFormTemplateUsage(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	id := strings.TrimSpace(c.Params("templateId"))
	var count int64
	if err := model.GetDB().Model(&model.ChannelIFormModel{}).Where("template_ref = ?", "platform:"+id).Count(&count).Error; err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取模板引用数量失败")
	}
	return c.JSON(fiber.Map{"templateRef": "platform:" + id, "references": count})
}

func AdminChannelIFormTemplateDelete(c *fiber.Ctx) error {
	if err := requirePlatformAdmin(c); err != nil {
		return err
	}
	id := strings.TrimSpace(c.Params("templateId"))
	var template model.ChannelIFormTemplateModel
	if err := model.GetDB().Where("id = ?", id).First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return wrapErrorStatus(c, fiber.StatusNotFound, err, "模板不存在")
		}
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取频道嵌入模板失败")
	}

	var references int64
	if err := model.GetDB().Model(&model.ChannelIFormModel{}).
		Where("template_ref = ?", "platform:"+id).
		Count(&references).Error; err != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, err, "读取模板引用数量失败")
	}
	if references > 0 {
		return wrapErrorStatus(c, fiber.StatusConflict, nil, fmt.Sprintf("模板仍被 %d 个频道引用，请先解除引用", references))
	}

	result := model.GetDB().Unscoped().Where("id = ?", id).Delete(&template)
	if result.Error != nil {
		return wrapErrorStatus(c, fiber.StatusInternalServerError, result.Error, "删除频道嵌入模板失败")
	}
	if result.RowsAffected == 0 {
		return wrapErrorStatus(c, fiber.StatusNotFound, gorm.ErrRecordNotFound, "模板不存在")
	}
	return c.JSON(fiber.Map{"id": id})
}

func validateChannelIFormTemplateWrite(payload *channelIFormTemplateWriteRequest) error {
	if payload == nil || strings.TrimSpace(payload.Name) == "" {
		return errors.New("模板名称不能为空")
	}
	if payload.Url == "" && payload.EmbedCode == "" {
		return errors.New("需要提供 URL 或嵌入代码")
	}
	if payload.Url != "" {
		if _, err := normalizeURL(payload.Url); err != nil {
			return err
		}
	}
	if payload.EmbedCode != "" {
		if _, err := sanitizeEmbedCode(payload.EmbedCode); err != nil {
			return err
		}
	}
	return nil
}

func ChannelIFormBuiltinAsset(c *fiber.Ctx) error {
	key := strings.TrimSpace(c.Params("key"))
	registration, ok := service.BuiltinChannelIFormTool(key)
	if !ok {
		return c.SendStatus(fiber.StatusNotFound)
	}
	asset := strings.TrimSpace(c.Params("*"))
	if asset == "" {
		asset = "index.html"
	}
	if filepath.IsAbs(asset) || strings.Contains(asset, "\\") {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	clean := filepath.Clean(filepath.FromSlash(asset))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	root := filepath.Join(service.BuiltinChannelIFormRoot(), registration.Directory)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return sendEmbeddedChannelIFormBuiltinAsset(c, key, registration.Directory, clean)
	}
	target := filepath.Join(rootAbs, clean)
	rel, err := filepath.Rel(rootAbs, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return sendEmbeddedChannelIFormBuiltinAsset(c, key, registration.Directory, clean)
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return sendEmbeddedChannelIFormBuiltinAsset(c, key, registration.Directory, clean)
	}
	rel, err = filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	info, err := os.Stat(resolvedTarget)
	if err != nil || info.IsDir() {
		return sendEmbeddedChannelIFormBuiltinAsset(c, key, registration.Directory, clean)
	}
	weakETag := fmt.Sprintf("W/\"%d-%d\"", info.ModTime().UnixNano(), info.Size())
	c.Set(fiber.HeaderETag, weakETag)
	c.Set(fiber.HeaderLastModified, info.ModTime().UTC().Format(http.TimeFormat))
	c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	if strings.Contains(c.Get(fiber.HeaderIfNoneMatch), weakETag) {
		return c.SendStatus(fiber.StatusNotModified)
	}
	if modifiedSince := c.Get(fiber.HeaderIfModifiedSince); modifiedSince != "" {
		if parsed, parseErr := http.ParseTime(modifiedSince); parseErr == nil && !info.ModTime().After(parsed.Add(time.Second)) {
			return c.SendStatus(fiber.StatusNotModified)
		}
	}
	data, err := os.ReadFile(resolvedTarget)
	if err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	contentType := mime.TypeByExtension(filepath.Ext(resolvedTarget))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if strings.HasPrefix(contentType, "text/") || contentType == "application/javascript" {
		contentType += "; charset=utf-8"
	}
	c.Set(fiber.HeaderContentType, contentType)
	c.Set(fiber.HeaderContentLength, strconv.Itoa(len(data)))
	return c.Send(data)
}

func sendEmbeddedChannelIFormBuiltinAsset(c *fiber.Ctx, key, directory, asset string) error {
	data, err := builtinassets.ReadChannelEmbedToolAsset(directory, filepath.ToSlash(asset))
	if err != nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	etag := fmt.Sprintf("W/\"embedded-%s-%s-%d\"", key, strings.ReplaceAll(filepath.ToSlash(asset), "/", "-"), len(data))
	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	if strings.Contains(c.Get(fiber.HeaderIfNoneMatch), etag) {
		return c.SendStatus(fiber.StatusNotModified)
	}
	contentType := mime.TypeByExtension(filepath.Ext(asset))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if strings.HasPrefix(contentType, "text/") || contentType == "application/javascript" {
		contentType += "; charset=utf-8"
	}
	c.Set(fiber.HeaderContentType, contentType)
	c.Set(fiber.HeaderContentLength, strconv.Itoa(len(data)))
	return c.Send(data)
}
