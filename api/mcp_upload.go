package api

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/afero"
	"sealchat/model"
	"sealchat/service"
	"sealchat/service/storage"
	"sealchat/utils"
)

type mcpUploadInfo struct {
	Path        string `json:"path"`
	Method      string `json:"method"`
	ContentType string `json:"contentType"`
	Field       string `json:"field"`
	MaxBytes    int64  `json:"maxBytes"`
	Usage       string `json:"usage"`
}

func mcpUploadLimit(cfg utils.AppConfig) int64 {
	limit := int64(64 << 20)
	if cfg.Storage.MaxSizeMB > 0 && cfg.Storage.MaxSizeMB < (limit>>20) {
		limit = cfg.Storage.MaxSizeMB << 20
	}
	if cfg.ImageSizeLimit > 0 && cfg.ImageSizeLimit < limit/1024 {
		limit = cfg.ImageSizeLimit * 1024
	}
	return limit
}
func mcpFileTools() []mcpToolSpec {
	return []mcpToolSpec{mcpSpec("file_upload_info", "返回同站点 HTTP 上传信息。客户端须具备 multipart HTTP 文件上传能力；上传后附件仅为临时文件。", []string{"file:write"}, false, false, true, func(_ context.Context, _ *service.MCPActor, _ mcpEmpty) (any, error) {
		cfg := mcpConfigSnapshot()
		return mcpUploadInfo{joinWebPath(cfg.WebUrl, "api/v1/mcp/uploads"), "POST", "multipart/form-data", "file", mcpUploadLimit(cfg), "仅接受单个 file 字段；由支持附件的线索或角色工具校验归属后绑定。禁止将 Key 放在 URL 中。"}, nil
	})}
}

type mcpUploadDTO struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType"`
	IsTemp   bool   `json:"isTemp"`
}

func mcpUpload(c *fiber.Ctx, limiter *service.MCPRateLimiter) (err error) {
	a, _ := c.Locals("mcpActor").(*service.MCPActor)
	if a == nil || !a.Allows("file:write") {
		return c.Status(403).JSON(mcpError{"forbidden", "缺少 file:write 授权"})
	}
	cfg := mcpConfigSnapshot()
	if !limiter.Allow(a.User.ID, true, true, cfg.MCP) {
		return c.Status(429).JSON(mcpError{"rate_limited", "上传过于频繁"})
	}
	start := time.Now()
	resourceID := ""
	defer func() {
		result := "ok"
		if err != nil || c.Response().StatusCode() >= 400 {
			result = "error"
		}
		slog.Info("mcp_write", "requestId", c.Get("X-SealChat-MCP-Request-ID"), "keyId", a.Key.ID, "actorUserId", a.User.ID, "tool", "file_upload", "resourceId", resourceID, "result", result, "durationMs", time.Since(start).Milliseconds())
	}()
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(400).JSON(mcpError{"invalid_argument", "需要 multipart/form-data"})
	}
	if len(form.Value) != 0 || len(form.File) != 1 || len(form.File["file"]) != 1 {
		return c.Status(400).JSON(mcpError{"invalid_argument", "仅允许一个 file 字段，不接受归属参数"})
	}
	fh := form.File["file"][0]
	limit := mcpUploadLimit(cfg)
	if fh.Size <= 0 || fh.Size > limit {
		return c.Status(413).JSON(mcpError{"file_too_large", "文件为空或超过上传限制"})
	}
	f, err := fh.Open()
	if err != nil {
		return c.Status(400).JSON(mcpError{"invalid_argument", "无法读取文件"})
	}
	mime, detectErr := mimetype.DetectReader(f)
	closeErr := f.Close()
	if detectErr != nil || closeErr != nil {
		return c.Status(400).JSON(mcpError{"invalid_argument", "无法检测文件类型"})
	}
	fh.Header.Set("Content-Type", mime.String())
	tmpDir := cfg.Storage.Local.TempDir
	if strings.TrimSpace(tmpDir) == "" {
		tmpDir = "./data/temp/"
	}
	if err := appFs.MkdirAll(tmpDir, 0755); err != nil {
		return c.Status(500).JSON(mcpError{"upload_failed", "无法保存上传文件"})
	}
	tmp, err := afero.TempFile(appFs, tmpDir, "mcp-*.upload")
	if err != nil {
		return c.Status(500).JSON(mcpError{"upload_failed", "无法保存上传文件"})
	}
	defer func() { _ = tmp.Close(); _ = appFs.Remove(tmp.Name()) }()
	saved, err := SaveMultipartFile(fh, tmp, limit)
	if err != nil {
		if errors.Is(err, ErrFileTooLarge) {
			return c.Status(413).JSON(mcpError{"file_too_large", "文件超过上传限制"})
		}
		return c.Status(400).JSON(mcpError{"upload_failed", "文件保存失败"})
	}
	if err := tmp.Close(); err != nil {
		return c.Status(500).JSON(mcpError{"upload_failed", "文件保存失败"})
	}
	name := filepath.Base(strings.ReplaceAll(fh.Filename, "\\", "/"))
	if len(name) > 255 {
		name = "upload"
	}
	location, err := service.PersistAttachmentFileForceNew(saved.Hash, saved.Size, tmp.Name(), saved.MimeType, name)
	if err != nil {
		return c.Status(500).JSON(mcpError{"upload_failed", "文件存储失败"})
	}
	tx, attachment := model.AttachmentCreate(&model.AttachmentModel{Filename: name, Size: saved.Size, Hash: saved.Hash, MimeType: saved.MimeType, IsAnimated: saved.IsAnimated, UserID: a.User.ID, StorageType: location.StorageType, ObjectKey: location.ObjectKey, ExternalURL: location.ExternalURL, IsTemp: true, Extra: "mcp-upload"})
	if tx.Error != nil {
		backend := storage.BackendLocal
		if location.StorageType == model.StorageS3 {
			backend = storage.BackendS3
		}
		_ = service.GetStorageManager().Delete(context.Background(), backend, location.ObjectKey)
		return c.Status(500).JSON(mcpError{"upload_failed", "附件登记失败"})
	}
	resourceID = attachment.ID
	return c.Status(201).JSON(mcpUploadDTO{attachment.ID, attachment.Filename, attachment.Size, attachment.MimeType, true})
}
