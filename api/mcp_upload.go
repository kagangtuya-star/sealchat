package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

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
	limit, _, _ := service.MCPUploadLimits(cfg, "attachment")
	return limit
}

type mcpUploadInfoInput struct {
	Profile string `json:"profile,omitempty"`
}
type mcpDiscardInput struct {
	UploadID string `json:"uploadId"`
}

func mcpFileTools() []mcpToolSpec {
	return []mcpToolSpec{mcpSpec("file_upload_info", "返回同站点 HTTP 上传信息；可选 profile=attachment/audio/theater_media/text。客户端须具备 multipart HTTP 文件上传能力；上传只暂存，不授予目标资源权限。", []string{"file:write"}, false, false, true, func(_ context.Context, a *service.MCPActor, in mcpUploadInfoInput) (any, error) {
		cfg := mcpConfigSnapshot()
		limit, scope, err := service.MCPUploadLimits(cfg, in.Profile)
		if err != nil {
			return nil, mcpFailure("invalid_argument", err.Error())
		}
		if scope != "" && !a.Allows(scope) {
			return nil, service.ErrMCPScopeDenied
		}
		path := joinWebPath(cfg.WebUrl, "api/v1/mcp/uploads")
		usage := "仅接受单个 file 字段；由支持附件的线索或角色工具校验归属后绑定。禁止将 Key 放在 URL 中。"
		if in.Profile != "" {
			path += "?profile=" + in.Profile
			usage = "仅接受单个 file 字段；响应 id 即 uploadId，24 小时后清理未绑定上传，可用 file_discard 丢弃。audio_asset_import/theater_asset/embed_save 按业务再次鉴权和校验；attachment 可用于 clue_save/identity_save。禁止将 Key 放在 URL 中。"
		}
		return mcpUploadInfo{path, "POST", "multipart/form-data", "file", limit, usage}, nil
	}), mcpSpec("file_discard", "删除本人仍未绑定的 MCP 临时上传及存储对象；重复删除已不存在的 ID 成功，存储失败可重试。", []string{"file:write"}, true, true, true, func(_ context.Context, a *service.MCPActor, in mcpDiscardInput) (any, error) {
		if err := service.DiscardMCPUpload(a.User.ID, in.UploadID); err != nil {
			return nil, err
		}
		return mcpOK{true}, nil
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
	profile := ""
	invalidQuery := false
	queryCount := 0
	c.Context().QueryArgs().VisitAll(func(key, value []byte) {
		queryCount++
		if string(key) != "profile" || queryCount > 1 {
			invalidQuery = true
			return
		}
		profile = string(value)
	})
	limit, scope, profileErr := service.MCPUploadLimits(cfg, profile)
	if invalidQuery || profileErr != nil {
		return c.Status(400).JSON(mcpError{"invalid_argument", "仅接受 profile=attachment/audio/theater_media/text，不接受归属参数"})
	}
	if scope != "" && !a.Allows(scope) {
		return c.Status(403).JSON(mcpError{"forbidden", "缺少 " + scope + " 授权"})
	}
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
		slog.Info("mcp_write", "requestId", c.Get("X-SealChat-MCP-Request-ID"), "keyId", a.CredentialID, "credentialType", a.CredentialType, "actorUserId", a.User.ID, "tool", "file_upload", "resourceId", resourceID, "result", result, "durationMs", time.Since(start).Milliseconds())
	}()
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(400).JSON(mcpError{"invalid_argument", "需要 multipart/form-data"})
	}
	if len(form.Value) != 0 || len(form.File) != 1 || len(form.File["file"]) != 1 {
		return c.Status(400).JSON(mcpError{"invalid_argument", "仅允许一个 file 字段，不接受归属参数"})
	}
	fh := form.File["file"][0]
	if fh.Size <= 0 || fh.Size > limit {
		return c.Status(413).JSON(mcpError{"file_too_large", "文件为空或超过上传限制"})
	}
	f, err := fh.Open()
	if err != nil {
		return c.Status(400).JSON(mcpError{"invalid_argument", "无法读取文件"})
	}
	mime, detectErr := service.MCPValidateUpload(cfg, profile, f, fh.Size)
	closeErr := f.Close()
	if detectErr != nil || closeErr != nil {
		if errors.Is(detectErr, service.ErrAudioTooLarge) {
			return c.Status(413).JSON(mcpError{"file_too_large", "文件超过 profile 类型大小限制"})
		}
		return c.Status(400).JSON(mcpError{"invalid_argument", "无法检测文件类型"})
	}
	fh.Header.Set("Content-Type", mime)
	unlock, capacityErr := service.MCPReserveUploadCapacity(a.User.ID, fh.Size, utils.NormalizeMCPConfig(cfg.MCP).UploadPendingMaxMB<<20)
	if capacityErr != nil {
		if errors.Is(capacityErr, service.ErrMCPUploadPendingLimit) {
			return c.Status(413).JSON(mcpError{"pending_upload_limit", capacityErr.Error()})
		}
		return c.Status(500).JSON(mcpError{"upload_failed", "无法检查临时上传容量"})
	}
	defer unlock()
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
	var saved SaveMultipartFileResult
	if profile == "" || profile == "attachment" {
		saved, err = SaveMultipartFile(fh, tmp, limit)
	} else {
		// Preserve source bytes for native media probing/processing and stream
		// large files instead of loading them into the generic image compressor.
		src, openErr := fh.Open()
		if openErr != nil {
			return c.Status(400).JSON(mcpError{"invalid_argument", "无法读取文件"})
		}
		saved.Hash, saved.Size, err = copyWithHash(tmp, io.LimitReader(src, limit+1))
		_ = src.Close()
		saved.MimeType = mime
		if saved.Size > limit {
			err = ErrFileTooLarge
		}
	}
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
	note := ""
	if profile != "" {
		note = "mcp-profile:" + profile
	}
	tx, attachment := model.AttachmentCreate(&model.AttachmentModel{Filename: name, Size: saved.Size, Hash: saved.Hash, MimeType: saved.MimeType, IsAnimated: saved.IsAnimated, UserID: a.User.ID, StorageType: location.StorageType, ObjectKey: location.ObjectKey, ExternalURL: location.ExternalURL, IsTemp: true, Extra: "mcp-upload", Note: note})
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
