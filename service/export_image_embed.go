package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	htmlnode "golang.org/x/net/html"

	"sealchat/model"
	"sealchat/service/storage"
	"sealchat/utils"
)

const maxInlineImageSize = 5 * 1024 * 1024

type inlineImageEmbedder struct {
	mu       sync.RWMutex
	cache    map[string]string
	inflight map[string]*inlineImageInflight
}

func newInlineImageEmbedder() *inlineImageEmbedder {
	return &inlineImageEmbedder{
		cache:    make(map[string]string),
		inflight: make(map[string]*inlineImageInflight),
	}
}

type inlineImageInflight struct {
	done  chan struct{}
	value string
	ok    bool
}

func (e *inlineImageEmbedder) inlinePayload(payload *ExportPayload) {
	if payload == nil {
		return
	}
	for i := range payload.Messages {
		if !strings.Contains(strings.ToLower(payload.Messages[i].Content), "<img") {
			continue
		}
		if html, ok := e.inlineHTML(payload.Messages[i].Content); ok {
			payload.Messages[i].Content = html
		}
		if avatar := strings.TrimSpace(payload.Messages[i].SenderAvatar); avatar != "" {
			if inlined, ok := e.resolveDataURL(avatar); ok {
				payload.Messages[i].SenderAvatar = inlined
			}
		}
	}
}

func (e *inlineImageEmbedder) inlineHTML(content string) (string, bool) {
	nodes, err := htmlnode.ParseFragment(strings.NewReader(content), nil)
	if err != nil {
		return "", false
	}
	changed := false
	for _, node := range nodes {
		if e.rewriteNode(node) {
			changed = true
		}
	}
	if !changed {
		return "", false
	}
	var buf bytes.Buffer
	for _, node := range nodes {
		if err := htmlnode.Render(&buf, node); err != nil {
			return "", false
		}
	}
	return buf.String(), true
}

func (e *inlineImageEmbedder) rewriteNode(node *htmlnode.Node) bool {
	changed := false
	if node.Type == htmlnode.ElementNode && strings.EqualFold(node.Data, "img") {
		for idx, attr := range node.Attr {
			if strings.EqualFold(attr.Key, "src") {
				if dataURL, ok := e.resolveDataURL(attr.Val); ok {
					node.Attr[idx].Val = dataURL
					changed = true
				}
				break
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if e.rewriteNode(child) {
			changed = true
		}
	}
	return changed
}

func (e *inlineImageEmbedder) resolveDataURL(src string) (string, bool) {
	token := extractAttachmentToken(src)
	if token == "" {
		if strings.HasPrefix(strings.TrimSpace(src), "data:") {
			return src, true
		}
		return "", false
	}

	normalized := strings.TrimSpace(token)
	if normalized == "" {
		return "", false
	}

	var att *model.AttachmentModel
	if resolved, err := ResolveAttachment(normalized); err == nil && resolved != nil {
		att = resolved
	}

	inflightKey := normalized
	if att != nil && len(att.Hash) > 0 && att.Size > 0 {
		inflightKey = fmt.Sprintf("hs:%s_%d", hex.EncodeToString(att.Hash), att.Size)
	}

	dataURL, ok := e.withInflight(inflightKey, func() (string, bool) {
		if cached, ok := e.getCached(normalized); ok {
			return cached, true
		}
		data, mimeType, key, err := loadAttachmentBytes(normalized, att)
		if err != nil {
			return "", false
		}
		if len(data) == 0 || len(data) > maxInlineImageSize {
			return "", false
		}
		if mimeType == "" {
			mimeType = http.DetectContentType(data)
		}
		if mimeType == "" {
			return "", false
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
		e.setCached(normalized, key, dataURL)
		return dataURL, true
	})
	if ok && inflightKey != normalized {
		e.setCached(normalized, "", dataURL)
	}
	return dataURL, ok
}

func (e *inlineImageEmbedder) withInflight(key string, fn func() (string, bool)) (string, bool) {
	if strings.TrimSpace(key) == "" {
		return fn()
	}

	e.mu.Lock()
	entry := e.inflight[key]
	if entry != nil {
		done := entry.done
		e.mu.Unlock()
		<-done
		return entry.value, entry.ok
	}
	entry = &inlineImageInflight{done: make(chan struct{})}
	e.inflight[key] = entry
	e.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			e.mu.Lock()
			entry.value = ""
			entry.ok = false
			close(entry.done)
			delete(e.inflight, key)
			e.mu.Unlock()
			panic(r)
		}
	}()

	value, ok := fn()

	e.mu.Lock()
	entry.value = value
	entry.ok = ok
	close(entry.done)
	delete(e.inflight, key)
	e.mu.Unlock()

	return value, ok
}

func (e *inlineImageEmbedder) getCached(keys ...string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, key := range keys {
		if key == "" {
			continue
		}
		if value, ok := e.cache[key]; ok {
			return value, true
		}
	}
	return "", false
}

func (e *inlineImageEmbedder) setCached(token, hashKey, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if token != "" {
		e.cache[token] = value
	}
	if hashKey != "" {
		e.cache[hashKey] = value
	}
}

func extractAttachmentToken(src string) string {
	value := strings.TrimSpace(src)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "data:") {
		return ""
	}
	if strings.HasPrefix(value, "id:") {
		return strings.TrimSpace(value[3:])
	}
	if idx := strings.Index(value, "/api/v1/attachment/"); idx >= 0 {
		token := value[idx+len("/api/v1/attachment/"):]
		if q := strings.Index(token, "?"); q >= 0 {
			token = token[:q]
		}
		return strings.Trim(token, "/")
	}
	if attachmentTokenPattern.MatchString(value) {
		return value
	}
	return ""
}

func loadAttachmentBytes(token string, resolved *model.AttachmentModel) ([]byte, string, string, error) {
	normalized := strings.TrimSpace(token)
	if normalized == "" {
		return nil, "", "", fmt.Errorf("empty token")
	}

	if resolved != nil {
		if data, mimeType, hashKey, err := readAttachmentFile(resolved); err == nil {
			return data, mimeType, hashKey, nil
		}
	}

	if att, err := ResolveAttachment(normalized); err == nil && att != nil {
		if data, mimeType, hashKey, err := readAttachmentFile(att); err == nil {
			return data, mimeType, hashKey, nil
		}
	}
	uploadRoot := utils.GetConfig().Storage.Local.UploadDir
	if strings.TrimSpace(uploadRoot) == "" {
		uploadRoot = "data/upload"
	}
	if data, err := os.ReadFile(filepath.Join(uploadRoot, normalized)); err == nil {
		mimeType := http.DetectContentType(data)
		if !strings.HasPrefix(mimeType, "image/") {
			return nil, "", "", fmt.Errorf("unsupported mime %s", mimeType)
		}
		digest := sha256.Sum256(data)
		return data, mimeType, hex.EncodeToString(digest[:]), nil
	}
	return nil, "", "", fmt.Errorf("attachment not found: %s", normalized)
}

func readAttachmentFile(att *model.AttachmentModel) ([]byte, string, string, error) {
	if att == nil {
		return nil, "", "", fmt.Errorf("nil attachment")
	}
	hashBytes := []byte(att.Hash)
	if len(hashBytes) == 0 {
		return nil, "", "", fmt.Errorf("missing hash for attachment %s", att.ID)
	}

	if att.StorageType == model.StorageS3 {
		if data, mimeType, err := fetchRemoteAttachment(att); err == nil {
			digest := sha256.Sum256(data)
			return data, mimeType, hex.EncodeToString(digest[:]), nil
		}
	}

	if strings.TrimSpace(att.ObjectKey) != "" {
		if path, err := ResolveLocalAttachmentPath(att.ObjectKey); err == nil {
			if data, err := os.ReadFile(path); err == nil {
				return finalizeAttachmentData(data, att.Filename)
			}
		}
	}

	fileName := fmt.Sprintf("%s_%d", hex.EncodeToString(hashBytes), att.Size)
	fullPath := filepath.Join("data/upload", fileName)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, "", "", err
	}
	return finalizeAttachmentData(data, att.Filename)
}

func fetchRemoteAttachment(att *model.AttachmentModel) ([]byte, string, error) {
	target := strings.TrimSpace(att.ExternalURL)
	manager := GetStorageManager()
	if target == "" && manager != nil && strings.TrimSpace(att.ObjectKey) != "" {
		target = manager.PublicURL(storage.BackendS3, att.ObjectKey)
	}
	if target == "" {
		return nil, "", fmt.Errorf("missing remote url")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("remote fetch failed: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func finalizeAttachmentData(data []byte, fallbackName string) ([]byte, string, string, error) {
	ext := strings.ToLower(filepath.Ext(fallbackName))
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, "", "", fmt.Errorf("unsupported mime %s", mimeType)
	}
	digest := sha256.Sum256(data)
	return data, mimeType, hex.EncodeToString(digest[:]), nil
}
