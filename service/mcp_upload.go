package service

import (
	"errors"
	"io"
	"mime/multipart"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"sealchat/utils"
)

var ErrMCPUploadProfile = errors.New("未知 MCP 上传 profile；允许 attachment/audio/theater_media/text")

func MCPUploadLimits(cfg utils.AppConfig, profile string) (int64, string, error) {
	switch profile {
	case "", "attachment":
		limit := int64(64 << 20)
		if cfg.Storage.MaxSizeMB > 0 && cfg.Storage.MaxSizeMB < limit>>20 {
			limit = cfg.Storage.MaxSizeMB << 20
		}
		if cfg.ImageSizeLimit > 0 && cfg.ImageSizeLimit < limit/1024 {
			limit = cfg.ImageSizeLimit * 1024
		}
		return limit, "", nil
	case "audio":
		if svc := GetAudioService(); svc != nil {
			return svc.maxUploadBytes(), "audio:write", nil
		}
		limit := cfg.Audio.MaxUploadSizeMB
		if limit <= 0 {
			limit = 80
		}
		return limit << 20, "audio:write", nil
	case "theater_media":
		c := mcpTheaterMediaConfig(cfg)
		return max(c.ImageMaxSizeMB, c.VideoMaxSizeMB) << 20, "theater:write", nil
	case "text":
		return MCPTextUploadMaxBytes, "", nil
	default:
		return 0, "", ErrMCPUploadProfile
	}
}

func mcpTheaterMediaConfig(cfg utils.AppConfig) utils.TheaterMediaConfig {
	if theaterMedia.ctx != nil {
		return normalizeTheaterMediaConfig(theaterMedia.config)
	}
	return normalizeTheaterMediaConfig(cfg.TheaterMedia)
}

// Use the native audio detector and allowlist, including its optional unknown
// format decoding path. Actual decoding and quota checks run again on import.
func MCPValidateUpload(cfg utils.AppConfig, profile string, file multipart.File, size int64) (string, error) {
	limit, _, err := MCPUploadLimits(cfg, profile)
	if err != nil {
		return "", err
	}
	if size <= 0 || size > limit {
		return "", ErrAudioTooLarge
	}
	if profile == "audio" {
		svc := GetAudioService()
		if svc == nil {
			svc = &audioService{cfg: cfg.Audio, allowedMimes: buildMimeMap(cfg.Audio.AllowedMimeTypes)}
		}
		return svc.validateMime(file)
	}
	head := make([]byte, 4096)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	if profile == "theater_media" {
		mime, kind := detectTheaterMediaType(head[:n])
		if mime == "" {
			return "", ErrAudioUnsupportedMime
		}
		c := mcpTheaterMediaConfig(cfg)
		limit = c.ImageMaxSizeMB << 20
		if kind == "video" {
			limit = c.VideoMaxSizeMB << 20
		}
		if size > limit {
			return "", ErrAudioTooLarge
		}
		return mime, nil
	}
	mt, err := mimetype.DetectReader(file)
	if err != nil {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	value := strings.ToLower(mt.String())
	if profile == "text" && !MCPTextMIME(value) {
		return "", ErrAudioUnsupportedMime
	}
	return value, nil
}
