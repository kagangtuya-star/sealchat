package service

import (
	"archive/zip"
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
	Endpoints      []string
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
	endpoints := normalizeUploadEndpoints(opts.Endpoint, opts.Endpoints)
	if len(endpoints) == 0 {
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

	payload := preparedLogUploadPayload{
		Name:      name,
		UniformID: uniformID,
		Client:    clientName,
		Version:   version,
		Data:      compressed,
	}
	url, usedEndpoint, err := uploadCompressedExportLog(endpoints, payload, strings.TrimSpace(opts.Token), timeout)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	meta := map[string]any{
		"name":       name,
		"uniform_id": uniformID,
		"client":     clientName,
		"version":    version,
		"endpoint":   usedEndpoint,
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

// UploadBatchExportLogs 上传批量 ZIP 中的每个 JSON 导出，并返回每个文件的链接。
func UploadBatchExportLogs(job *model.MessageExportJobModel, opts LogUploadOptions) ([]LogUploadResult, error) {
	if job == nil || job.Status != model.MessageExportStatusDone || strings.TrimSpace(job.FilePath) == "" {
		return nil, fmt.Errorf("导出任务尚未完成，无法上传")
	}
	extra := parseExportExtraOptions(job.ExtraOptions)
	if len(extra.BatchChannelIDs) == 0 || !strings.EqualFold(extra.BatchFormat, "json") {
		return nil, fmt.Errorf("该任务不是可上传的批量 JSON 导出")
	}
	archive, err := zip.OpenReader(job.FilePath)
	if err != nil {
		return nil, fmt.Errorf("打开批量导出文件失败: %w", err)
	}
	defer archive.Close()

	results := make([]LogUploadResult, 0, len(archive.File))
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() || !strings.EqualFold(filepath.Ext(entry.Name), ".json") {
			continue
		}
		input, err := entry.Open()
		if err != nil {
			return nil, err
		}
		tempFile, err := os.CreateTemp(filepath.Dir(job.FilePath), "batch-upload-*.json")
		if err != nil {
			_ = input.Close()
			return nil, err
		}
		_, copyErr := io.Copy(tempFile, input)
		closeErr := tempFile.Close()
		_ = input.Close()
		if copyErr != nil {
			_ = os.Remove(tempFile.Name())
			return nil, copyErr
		}
		if closeErr != nil {
			_ = os.Remove(tempFile.Name())
			return nil, closeErr
		}

		child := &model.MessageExportJobModel{
			Format:   "json",
			Status:   model.MessageExportStatusDone,
			FilePath: tempFile.Name(),
			FileName: filepath.Base(entry.Name),
		}
		childOpts := opts
		childOpts.Name = strings.TrimSuffix(filepath.Base(entry.Name), filepath.Ext(entry.Name))
		result, uploadErr := UploadExportLog(child, childOpts)
		_ = os.Remove(tempFile.Name())
		if uploadErr != nil {
			return nil, fmt.Errorf("上传 %s 失败: %w", entry.Name, uploadErr)
		}
		results = append(results, *result)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("批量导出中没有 JSON 文件")
	}
	return results, nil
}

type preparedLogUploadPayload struct {
	Name      string
	UniformID string
	Client    string
	Version   int
	Data      []byte
}

func normalizeUploadEndpoints(primary string, backups []string) []string {
	normalized := make([]string, 0, 1+len(backups))
	seen := make(map[string]struct{}, 1+len(backups))
	for _, raw := range append([]string{primary}, backups...) {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func uploadCompressedExportLog(endpoints []string, payload preparedLogUploadPayload, token string, timeout time.Duration) (string, string, error) {
	client := &http.Client{Timeout: timeout}
	return uploadCompressedExportLogWithClient(client, endpoints, payload, token)
}

func uploadCompressedExportLogWithClient(client *http.Client, endpoints []string, payload preparedLogUploadPayload, token string) (string, string, error) {
	failures := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		url, err := uploadCompressedExportLogOnce(client, endpoint, payload, token)
		if err == nil {
			return url, endpoint, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", endpoint, err))
	}
	return "", "", fmt.Errorf("所有云端上传地址均失败：%s", strings.Join(failures, "; "))
}

func uploadCompressedExportLogOnce(client *http.Client, endpoint string, payload preparedLogUploadPayload, token string) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("name", payload.Name)
	_ = writer.WriteField("uniform_id", payload.UniformID)
	_ = writer.WriteField("client", payload.Client)
	_ = writer.WriteField("version", strconv.Itoa(payload.Version))
	part, err := writer.CreateFormFile("file", "log-zlib-compressed")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(payload.Data); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPut, endpoint, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return extractUploadURL(respBody)
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
