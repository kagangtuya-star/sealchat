package service

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sealchat/model"
)

type LogUploadOptions struct {
	Name           string
	Endpoint       string
	Token          string
	UniformID      string
	Client         string
	Version        int
	TimeoutSeconds int
}

type LogUploadResult struct {
	URL        string
	Name       string
	FileName   string
	UploadedAt time.Time
}

func UploadExportLog(job *model.MessageExportJobModel, opts LogUploadOptions) (*LogUploadResult, error) {
	if job == nil {
		return nil, fmt.Errorf("任务不存在")
	}
	if !strings.EqualFold(job.Format, "json") {
		return nil, fmt.Errorf("该任务的导出格式不支持云端上传")
	}
	if job.Status != model.MessageExportStatusDone {
		return nil, fmt.Errorf("导出任务尚未完成，无法上传")
	}
	if strings.TrimSpace(job.FilePath) == "" {
		return nil, fmt.Errorf("导出文件缺失")
	}
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("上传接口未配置")
	}
	uniformID := normalizeUniformID(opts.UniformID)
	clientName := strings.TrimSpace(opts.Client)
	if strings.EqualFold(clientName, "") {
		clientName = "Others"
	}
	if !strings.EqualFold(clientName, "SealDice") && !strings.EqualFold(clientName, "DicePP") && !strings.EqualFold(clientName, "Others") {
		clientName = "Others"
	}
	version := opts.Version
	if version <= 0 {
		version = diceLogVersion
	}
	timeout := time.Duration(opts.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = deriveDefaultUploadName(job)
	}

	compressed, err := compressJSONFile(job.FilePath)
	if err != nil {
		return nil, err
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("name", name)
	_ = writer.WriteField("uniform_id", uniformID)
	_ = writer.WriteField("client", clientName)
	_ = writer.WriteField("version", strconv.Itoa(version))
	part, err := writer.CreateFormFile("file", "log-zlib-compressed")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(compressed); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPut, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token := strings.TrimSpace(opts.Token); token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("云端上传失败：HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	url, err := extractUploadURL(respBody)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	meta := map[string]any{
		"name":       name,
		"uniform_id": uniformID,
		"client":     clientName,
		"version":    version,
	}
	metaBytes, _ := json.Marshal(meta)
	updates := map[string]any{
		"upload_url":  url,
		"upload_meta": string(metaBytes),
		"uploaded_at": now,
	}
	if err := model.GetDB().Model(&model.MessageExportJobModel{}).
		Where("id = ?", job.ID).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	job.UploadURL = url
	job.UploadMeta = string(metaBytes)
	job.UploadedAt = &now

	return &LogUploadResult{
		URL:        url,
		Name:       name,
		FileName:   job.FileName,
		UploadedAt: now,
	}, nil
}

func compressJSONFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("导出文件不是有效的 JSON")
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func deriveDefaultUploadName(job *model.MessageExportJobModel) string {
	if job == nil {
		return "Sealchat_日志"
	}
	base := strings.TrimSuffix(job.FileName, filepath.Ext(job.FileName))
	base = strings.TrimSpace(base)
	if base == "" {
		base = sanitizeFileName(job.ChannelID)
	}
	if base == "" {
		base = job.ID
	}
	return base
}

func normalizeUniformID(input string) string {
	value := strings.TrimSpace(input)
	if value == "" {
		value = "Sealchat"
	}
	value = strings.ReplaceAll(value, " ", "")
	if strings.Contains(value, ":") {
		return value
	}
	return fmt.Sprintf("Sealchat:%s", value)
}

func extractUploadURL(body []byte) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("云端上传返回空响应")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		trimmed := strings.TrimSpace(string(body))
		if trimmed != "" {
			return "", fmt.Errorf("云端上传返回异常：%s", trimmed)
		}
		return "", fmt.Errorf("云端上传返回异常")
	}
	if urlValue, ok := payload["url"].(string); ok && strings.TrimSpace(urlValue) != "" {
		return strings.TrimSpace(urlValue), nil
	}
	if msg, ok := payload["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return "", fmt.Errorf("云端上传失败：%s", strings.TrimSpace(msg))
	}
	if success, ok := payload["success"].(bool); ok && !success {
		return "", fmt.Errorf("云端上传失败")
	}
	return "", fmt.Errorf("云端上传未返回 url")
}
