package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/utils"

	"github.com/mmonterroca/docxgo/v2/domain"
)

type WebPInfo struct {
	Width    int
	Height   int
	HasAlpha bool
	Animated bool
}

type docxImage struct {
	Data         []byte
	Format       domain.ImageFormat
	Width        int
	Height       int
	LayoutWidth  int
	LayoutHeight int
	Placeholder  string
}

type docxImageResolver struct {
	tempDir     string
	channelID   string
	cache       map[string]docxImage
	layoutCache map[string]*exportImageLayout
}

func newDocxImageResolver(channelIDs ...string) (*docxImageResolver, error) {
	dir, err := os.MkdirTemp("", "sealchat-docx-images-")
	if err != nil {
		return nil, err
	}
	channelID := ""
	if len(channelIDs) > 0 {
		channelID = strings.TrimSpace(channelIDs[0])
	}
	return &docxImageResolver{
		tempDir:     dir,
		channelID:   channelID,
		cache:       make(map[string]docxImage),
		layoutCache: make(map[string]*exportImageLayout),
	}, nil
}

func (r *docxImageResolver) Close() {
	if r != nil && r.tempDir != "" {
		_ = os.RemoveAll(r.tempDir)
	}
}

func (r *docxImageResolver) Resolve(node AgentRichNode) (docxImage, error) {
	if r == nil {
		return docxImage{}, fmt.Errorf("image resolver unavailable")
	}
	if r.cache == nil {
		r.cache = make(map[string]docxImage)
	}
	if r.layoutCache == nil {
		r.layoutCache = make(map[string]*exportImageLayout)
	}
	rawSource := strings.TrimSpace(firstNonEmpty(node.Attrs["attachment_id"], node.Attrs["attachmentId"], node.Attrs["dataAttachmentId"], node.Attrs["data-attachment-id"], node.Attrs["src"], node.Attrs["dataSrc"], node.Attrs["data-src"]))
	// CQ image tokens may carry a URL in their file/url parameter. Resolve
	// that parameter before deciding whether the source is a remote image so
	// non-SealChat URLs become a safe placeholder instead of a failed export.
	if isCQImageToken(rawSource) {
		if token := cqImageAttachmentToken(rawSource); token != "" {
			rawSource = token
		} else {
			label := strings.TrimSpace(node.Attrs["alt"])
			if label == "" {
				label = "[图片]"
			}
			return docxImage{Placeholder: label}, nil
		}
	}
	if (strings.HasPrefix(strings.ToLower(rawSource), "http://") || strings.HasPrefix(strings.ToLower(rawSource), "https://")) && extractAttachmentToken(rawSource) == "" {
		label := strings.TrimSpace(node.Attrs["alt"])
		if label == "" {
			label = "[图片]"
		}
		return docxImage{Placeholder: label}, nil
	}
	cacheKey := imageSourceCacheKey(node)
	if cacheKey != "" {
		if cached, ok := r.cache[cacheKey]; ok {
			return cached, nil
		}
	}
	key, data, err := r.source(node)
	if err != nil {
		return docxImage{}, err
	}
	if cached, ok := r.cache[key]; ok {
		if cacheKey != "" {
			r.cache[cacheKey] = cached
		}
		return cached, nil
	}
	kind := detectImageMagic(data)
	if kind == "" {
		return docxImage{}, fmt.Errorf("docx image %s: unsupported image format", shortImageKey(key))
	}
	if kind == "webp" {
		info, inspectErr := InspectWebP(data)
		if inspectErr != nil {
			return docxImage{}, fmt.Errorf("docx image %s: WebP metadata: %w", shortImageKey(key), inspectErr)
		}
		if info.Animated {
			return docxImage{}, fmt.Errorf("docx image %s: animated WebP is not supported", shortImageKey(key))
		}
		if info.HasAlpha {
			data, err = r.convertWebPToPNG(data, key)
			kind = "png"
		} else {
			data, err = r.convertWebPToJPEG(data, key)
			kind = "jpeg"
		}
		if err != nil {
			return docxImage{}, err
		}
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return docxImage{}, fmt.Errorf("docx image %s: invalid %s dimensions", shortImageKey(key), kind)
	}
	format := domain.ImageFormatJPEG
	if kind == "png" {
		format = domain.ImageFormatPNG
	}
	result := docxImage{Data: data, Format: format, Width: config.Width, Height: config.Height}
	if token := imageSourceAttachmentToken(node); token != "" {
		if layout := r.layoutForAttachment(token); layout != nil {
			result.LayoutWidth = layout.Width
			result.LayoutHeight = layout.Height
		}
	}
	r.cache[key] = result
	if cacheKey != "" {
		r.cache[cacheKey] = result
	}
	return result, nil
}

func imageSourceCacheKey(node AgentRichNode) string {
	if token := imageSourceAttachmentToken(node); token != "" {
		return "attachment-id:" + token
	}
	return ""
}

func imageSourceAttachmentToken(node AgentRichNode) string {
	raw := strings.TrimSpace(firstNonEmpty(node.Attrs["attachment_id"], node.Attrs["attachmentId"], node.Attrs["dataAttachmentId"], node.Attrs["data-attachment-id"], node.Attrs["src"], node.Attrs["dataSrc"], node.Attrs["data-src"]))
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "data:") {
		return ""
	}
	if isCQImageToken(raw) {
		raw = cqImageAttachmentToken(raw)
	}
	if strings.HasPrefix(strings.ToLower(raw), "http://") || strings.HasPrefix(strings.ToLower(raw), "https://") {
		return extractAttachmentToken(raw)
	}
	if token := extractAttachmentToken(raw); token != "" {
		return token
	}
	return strings.TrimPrefix(strings.TrimSpace(raw), "id:")
}

func (r *docxImageResolver) layoutForAttachment(token string) *exportImageLayout {
	if r == nil || strings.TrimSpace(r.channelID) == "" || strings.TrimSpace(token) == "" || model.GetDB() == nil {
		return nil
	}
	token = strings.TrimSpace(strings.TrimPrefix(token, "id:"))
	if layout, ok := r.layoutCache[token]; ok {
		return layout
	}
	r.layoutCache[token] = nil
	layouts, err := model.ChannelAttachmentImageLayoutBatchGet(r.channelID, []string{token})
	if err != nil {
		return nil
	}
	for _, layout := range layouts {
		if layout == nil || layout.Width <= 0 || layout.Height <= 0 {
			continue
		}
		resolved := &exportImageLayout{Width: layout.Width, Height: layout.Height}
		r.layoutCache[token] = resolved
		return resolved
	}
	return nil
}

func (r *docxImageResolver) source(node AgentRichNode) (string, []byte, error) {
	attrs := node.Attrs
	raw := firstNonEmpty(attrs["attachment_id"], attrs["attachmentId"], attrs["dataAttachmentId"], attrs["data-attachment-id"], attrs["src"], attrs["dataSrc"], attrs["data-src"])
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, fmt.Errorf("docx image: missing source")
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		comma := strings.IndexByte(raw, ',')
		if comma < 0 {
			return "", nil, fmt.Errorf("docx image data URL is malformed")
		}
		meta, payload := raw[:comma], raw[comma+1:]
		if !strings.Contains(strings.ToLower(meta), ";base64") {
			return "", nil, fmt.Errorf("docx image data URL must be base64")
		}
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", nil, fmt.Errorf("docx image data URL decode failed: %w", err)
		}
		sum := sha256.Sum256(decoded)
		return "data:" + fmt.Sprintf("%x", sum), decoded, nil
	}
	if strings.HasPrefix(strings.ToLower(raw), "http://") || strings.HasPrefix(strings.ToLower(raw), "https://") {
		if token := extractAttachmentToken(raw); token != "" {
			raw = token
		} else {
			return "", nil, fmt.Errorf("docx image remote URL is not an attachment")
		}
	}
	if isCQImageToken(raw) {
		if token := cqImageAttachmentToken(raw); token != "" {
			raw = token
		} else {
			return "", nil, fmt.Errorf("docx image: CQ image is not a SealChat attachment")
		}
	}
	token := extractAttachmentToken(raw)
	if token == "" {
		token = strings.TrimPrefix(strings.TrimSpace(raw), "id:")
	}
	att, err := ResolveAttachment(token)
	if err != nil || att == nil {
		return "", nil, fmt.Errorf("docx image %s: attachment not found", shortImageKey(token))
	}
	data, _, hash, err := loadAttachmentBytes(token, att)
	if err != nil {
		return "", nil, fmt.Errorf("docx image %s: read attachment failed", shortImageKey(token))
	}
	if hash == "" {
		sum := sha256.Sum256(data)
		hash = fmt.Sprintf("%x", sum)
	}
	return "attachment:" + hash, data, nil
}

func (r *docxImageResolver) convertWebPToPNG(data []byte, key string) ([]byte, error) {
	dwebp, err := utils.ResolveBundledWebPTool("dwebp")
	if err != nil {
		return nil, fmt.Errorf("docx image %s: dwebp unavailable", shortImageKey(key))
	}
	inPath, err := r.writeTemp("input.webp", data)
	if err != nil {
		return nil, fmt.Errorf("docx image %s: create WebP temp failed", shortImageKey(key))
	}
	sum := sha256.Sum256(data)
	outPath := filepath.Join(r.tempDir, fmt.Sprintf("%x.png", sum[:8]))
	var stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, dwebp, inPath, "-o", outPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, limitedToolError("dwebp decode", key, err, stderr.String(), ctx)
	}
	result, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("docx image %s: dwebp output read failed", shortImageKey(key))
	}
	return result, nil
}

func (r *docxImageResolver) convertWebPToJPEG(data []byte, key string) ([]byte, error) {
	dwebp, err := utils.ResolveBundledWebPTool("dwebp")
	if err != nil {
		return nil, fmt.Errorf("docx image %s: dwebp unavailable", shortImageKey(key))
	}
	cjpeg, err := utils.ResolveBundledCJPEGTool()
	if err != nil {
		return nil, fmt.Errorf("docx image %s: cjpeg unavailable", shortImageKey(key))
	}
	inPath, err := r.writeTemp("input.webp", data)
	if err != nil {
		return nil, fmt.Errorf("docx image %s: create WebP temp failed", shortImageKey(key))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dwebpCmd := exec.CommandContext(ctx, dwebp, inPath, "-ppm", "-o", "-")
	cjpegCmd := exec.CommandContext(ctx, cjpeg, "-quality", "85")
	ppm, err := dwebpCmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("docx image %s: dwebp pipe failed", shortImageKey(key))
	}
	stdin, err := cjpegCmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("docx image %s: cjpeg pipe failed", shortImageKey(key))
	}
	var dwebpErr, cjpegErr bytes.Buffer
	dwebpCmd.Stderr = &dwebpErr
	cjpegCmd.Stderr = &cjpegErr
	var jpeg bytes.Buffer
	cjpegCmd.Stdout = &jpeg
	if err := cjpegCmd.Start(); err != nil {
		return nil, fmt.Errorf("docx image %s: cjpeg start failed", shortImageKey(key))
	}
	if err := dwebpCmd.Start(); err != nil {
		_ = stdin.Close()
		_ = cjpegCmd.Wait()
		return nil, limitedToolError("dwebp start", key, err, dwebpErr.String(), ctx)
	}
	_, copyErr := io.Copy(stdin, ppm)
	_ = stdin.Close()
	dwebpWaitErr := dwebpCmd.Wait()
	cjpegWaitErr := cjpegCmd.Wait()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("docx image %s: WebP conversion canceled or timed out", shortImageKey(key))
	}
	if copyErr != nil || dwebpWaitErr != nil {
		cause := dwebpWaitErr
		if cause == nil {
			cause = copyErr
		}
		return nil, limitedToolError("dwebp decode", key, cause, dwebpErr.String(), ctx)
	}
	if cjpegWaitErr != nil {
		return nil, limitedToolError("cjpeg encode", key, cjpegWaitErr, cjpegErr.String(), ctx)
	}
	return jpeg.Bytes(), nil
}

func (r *docxImageResolver) writeTemp(name string, data []byte) (string, error) {
	ext := filepath.Ext(name)
	pattern := strings.TrimSuffix(name, ext) + "-*" + ext
	file, err := os.CreateTemp(r.tempDir, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return path, nil
}

func limitedToolError(stage, key string, err error, stderr string, ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return fmt.Errorf("docx image %s: %s canceled or timed out", shortImageKey(key), stage)
	}
	msg := strings.TrimSpace(stderr)
	if len(msg) > 512 {
		msg = msg[:512]
	}
	if msg == "" {
		return fmt.Errorf("docx image %s: %s failed", shortImageKey(key), stage)
	}
	return fmt.Errorf("docx image %s: %s failed: %s", shortImageKey(key), stage, msg)
}

func InspectWebP(data []byte) (WebPInfo, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return WebPInfo{}, fmt.Errorf("invalid RIFF/WEBP header")
	}
	riffSize := int(binary.LittleEndian.Uint32(data[4:8]))
	if riffSize < 4 || riffSize+8 > len(data) {
		return WebPInfo{}, fmt.Errorf("truncated RIFF payload")
	}
	end := riffSize + 8
	info := WebPInfo{}
	seenChunk := false
	for offset := 12; offset < end; {
		if offset+8 > end {
			return WebPInfo{}, fmt.Errorf("truncated chunk header")
		}
		kind := string(data[offset : offset+4])
		length := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + length
		if length < 0 || chunkEnd < chunkStart || chunkEnd > end {
			return WebPInfo{}, fmt.Errorf("truncated %s chunk", kind)
		}
		payload := data[chunkStart:chunkEnd]
		seenChunk = true
		switch kind {
		case "VP8X":
			if len(payload) < 10 {
				return WebPInfo{}, fmt.Errorf("truncated VP8X chunk")
			}
			info.Animated = info.Animated || payload[0]&0x02 != 0
			info.HasAlpha = info.HasAlpha || payload[0]&0x10 != 0
			info.Width = 1 + (int(payload[4]) | int(payload[5])<<8 | int(payload[6])<<16)
			info.Height = 1 + (int(payload[7]) | int(payload[8])<<8 | int(payload[9])<<16)
		case "ALPH":
			info.HasAlpha = true
		case "ANIM", "ANMF":
			info.Animated = true
		case "VP8L":
			if len(payload) < 5 || payload[0] != 0x2f {
				return WebPInfo{}, fmt.Errorf("invalid VP8L header")
			}
			bits := binary.LittleEndian.Uint32(payload[1:5])
			info.Width = 1 + int(bits&0x3fff)
			info.Height = 1 + int((bits>>14)&0x3fff)
			info.HasAlpha = info.HasAlpha || bits&(1<<28) != 0
		case "VP8", "VP8 ":
			if len(payload) < 10 {
				return WebPInfo{}, fmt.Errorf("truncated VP8 chunk")
			}
			if payload[3] != 0x9d || payload[4] != 0x01 || payload[5] != 0x2a {
				return WebPInfo{}, fmt.Errorf("invalid VP8 frame header")
			}
			info.Width = int(binary.LittleEndian.Uint16(payload[6:8]) & 0x3fff)
			info.Height = int(binary.LittleEndian.Uint16(payload[8:10]) & 0x3fff)
		}
		offset = chunkEnd
		if length%2 == 1 {
			offset++
		}
		if offset > end {
			return WebPInfo{}, fmt.Errorf("truncated chunk padding")
		}
	}
	if !seenChunk {
		return WebPInfo{}, fmt.Errorf("missing WebP chunks")
	}
	return info, nil
}

func detectImageMagic(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "jpeg"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "webp"
	default:
		return ""
	}
}

func cqImageAttachmentToken(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !isCQImageToken(trimmed) {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed[len("[CQ:image"):], "]")
	trimmed = strings.TrimPrefix(trimmed, ",")
	var fileValue, urlValue string
	for _, part := range strings.Split(trimmed, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "file":
			fileValue = strings.TrimSpace(value)
		case "url":
			urlValue = strings.TrimSpace(value)
		}
	}
	if token := cqImageAttachmentCandidate(fileValue); token != "" {
		return token
	}
	if token := extractAttachmentToken(urlValue); token != "" {
		return token
	}
	return ""
}

func cqImageAttachmentCandidate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "image") {
		return ""
	}
	return extractAttachmentToken(value)
}

func isCQImageToken(raw string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "[cq:image")
}

func shortImageKey(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 16 {
		return value[:16]
	}
	return value
}
