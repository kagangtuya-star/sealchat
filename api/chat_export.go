package api

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
)

type chatExportRequest struct {
	ChannelID          string         `json:"channel_id"`
	Format             string         `json:"format"`
	DisplayName        string         `json:"display_name"`
	TimeRange          []int64        `json:"time_range"`
	IncludeOOC         *bool          `json:"include_ooc"`
	IncludeArchived    *bool          `json:"include_archived"`
	WithoutTimestamp   *bool          `json:"without_timestamp"`
	MergeMessages      *bool          `json:"merge_messages"`
	Users              []string       `json:"users"`
	DisplaySettings    map[string]any `json:"display_settings"`
	SliceLimit         int            `json:"slice_limit"`
	MaxConcurrency     int            `json:"max_concurrency"`
	TextColorizeBBCode *bool          `json:"text_bbcode_colorize"`
}

type chatExportResponse struct {
	TaskID         string `json:"task_id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	RequestedAt    int64  `json:"requested_at"`
	SliceLimit     int    `json:"slice_limit,omitempty"`
	MaxConcurrency int    `json:"max_concurrency,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
}

type chatExportStatusResponse struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	FileName    string `json:"file_name"`
	DisplayName string `json:"display_name,omitempty"`
	Message     string `json:"message"`
	FinishedAt  int64  `json:"finished_at,omitempty"`
	UploadURL   string `json:"upload_url,omitempty"`
	UploadedAt  int64  `json:"uploaded_at,omitempty"`
}

type chatExportUploadRequest struct {
	Name string `json:"name"`
}

type chatExportUploadResponse struct {
	URL        string `json:"url"`
	Name       string `json:"name,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	UploadedAt int64  `json:"uploaded_at,omitempty"`
}

type chatExportDeleteResponse struct {
	TaskID      string `json:"task_id"`
	FileDeleted bool   `json:"file_deleted"`
}

type chatExportListResponse struct {
	Total     int64                `json:"total"`
	TotalSize int64                `json:"total_size"`
	Page      int                  `json:"page"`
	Size      int                  `json:"size"`
	Items     []chatExportListItem `json:"items"`
}

type chatExportListItem struct {
	TaskID      string `json:"task_id"`
	Format      string `json:"format"`
	Status      string `json:"status"`
	DisplayName string `json:"display_name,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	FileSize    int64  `json:"file_size"`
	FinishedAt  int64  `json:"finished_at,omitempty"`
	RequestedAt int64  `json:"requested_at"`
	Message     string `json:"message,omitempty"`
	UploadURL   string `json:"upload_url,omitempty"`
	DownloadURL string `json:"download_url"`
	FileMissing bool   `json:"file_missing"`
}

func validateExportChannel(userID, channelID string) error {
	if channelID == "" {
		return fmt.Errorf("channel_id 不能为空")
	}
	if len(channelID) < 30 {
		if !pm.CanWithChannelRole(userID, channelID, pm.PermFuncChannelManageInfo, pm.PermFuncChannelReadAll) {
			return fmt.Errorf("无权限导出该频道")
		}
		return nil
	}

	fr, _ := model.FriendRelationGetByID(channelID)
	if fr.ID == "" {
		return fmt.Errorf("频道不存在")
	}
	if fr.UserID1 != userID && fr.UserID2 != userID {
		return fmt.Errorf("无权限导出该频道")
	}
	return nil
}

func execChatExportCreate(userID string, req *chatExportRequest) (*chatExportResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("请求参数不能为空")
	}
	channelID := strings.TrimSpace(req.ChannelID)
	if err := validateExportChannel(userID, channelID); err != nil {
		return nil, err
	}

	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = "txt"
	}
	start, end := parseTimeRange(req.TimeRange)

	includeOOC := true
	if req.IncludeOOC != nil {
		includeOOC = *req.IncludeOOC
	}
	includeArchived := false
	if req.IncludeArchived != nil {
		includeArchived = *req.IncludeArchived
	}
	withoutTimestamp := false
	if req.WithoutTimestamp != nil {
		withoutTimestamp = *req.WithoutTimestamp
	}
	mergeMessages := true
	if req.MergeMessages != nil {
		mergeMessages = *req.MergeMessages
	}

	textColorizeBBCode := false
	if req.TextColorizeBBCode != nil && strings.EqualFold(format, "txt") {
		textColorizeBBCode = *req.TextColorizeBBCode
	}

	displaySettings := normalizeDisplaySettings(req.DisplaySettings)
	sliceLimit := service.NormalizeExportSliceLimit(req.SliceLimit)
	maxConcurrency := service.NormalizeExportConcurrency(req.MaxConcurrency)

	job, err := service.CreateMessageExportJob(&service.ExportJobOptions{
		UserID:             userID,
		ChannelID:          channelID,
		Format:             format,
		DisplayName:        req.DisplayName,
		IncludeOOC:         includeOOC,
		IncludeArchived:    includeArchived,
		WithoutTimestamp:   withoutTimestamp,
		MergeMessages:      mergeMessages,
		TextColorizeBBCode: textColorizeBBCode,
		StartTime:          start,
		EndTime:            end,
		DisplaySettings:    displaySettings,
		SliceLimit:         sliceLimit,
		MaxConcurrency:     maxConcurrency,
	})
	if err != nil {
		return nil, err
	}
	return &chatExportResponse{
		TaskID:         job.ID,
		Status:         job.Status,
		DisplayName:    job.DisplayName,
		Message:        "导出任务已创建，请稍后下载。",
		RequestedAt:    job.CreatedAt.UnixMilli(),
		SliceLimit:     sliceLimit,
		MaxConcurrency: maxConcurrency,
	}, nil
}

func parseTimeRange(values []int64) (*time.Time, *time.Time) {
	if len(values) != 2 {
		return nil, nil
	}
	start := time.UnixMilli(values[0])
	end := time.UnixMilli(values[1])
	if start.After(end) {
		start, end = end, start
	}
	return &start, &end
}

func normalizeDisplaySettings(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	sanitized := make(map[string]any, len(values))
	for key, value := range values {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		sanitized[trimmed] = value
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func ChatExportCreate(c *fiber.Ctx) error {
	var req chatExportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "请求体解析失败"})
	}
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未认证"})
	}
	resp, err := execChatExportCreate(user.ID, &req)
	if err != nil {
		return c.Status(mapExportError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

func ChatExportList(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未认证"})
	}
	channelID := strings.TrimSpace(c.Query("channel_id"))
	if err := validateExportChannel(user.ID, channelID); err != nil {
		return c.Status(mapExportError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	statuses := parseExportStatuses(c.Query("status"))
	if len(statuses) == 0 {
		statuses = []string{model.MessageExportStatusDone}
	}
	page := parsePositiveInt(c.Query("page"), 1)
	if page < 1 {
		page = 1
	}
	size := parsePositiveInt(c.Query("size"), 20)
	if size < 1 {
		size = 20
	}
	offset := (page - 1) * size
	keyword := strings.TrimSpace(c.Query("keyword"))

	jobs, total, totalSize, err := service.ListMessageExportJobs(channelID, statuses, size, offset, keyword)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]chatExportListItem, len(jobs))
	for i, job := range jobs {
		items[i] = buildChatExportListItem(job)
	}
	resp := chatExportListResponse{
		Total:     total,
		TotalSize: totalSize,
		Page:      page,
		Size:      size,
		Items:     items,
	}
	return c.JSON(resp)
}

func ChatExportGet(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未认证"})
	}
	taskID := strings.TrimSpace(c.Params("taskId"))
	job, err := service.GetMessageExportJob(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "任务不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if job.UserID != user.ID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "无权限访问该任务"})
	}

	if c.QueryBool("download") {
		if job.Status != model.MessageExportStatusDone || strings.TrimSpace(job.FilePath) == "" {
			if strings.TrimSpace(job.FilePath) == "" {
				recordExportDownloadError(job.ID, "导出文件缺失")
			}
			return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "任务尚未完成"})
		}
		fileName := resolveDownloadFileName(job)
		return streamExportFile(c, job, fileName)
	}

	resp := chatExportStatusResponse{
		TaskID:      job.ID,
		Status:      job.Status,
		FileName:    job.FileName,
		DisplayName: job.DisplayName,
		Message:     job.ErrorMsg,
	}
	if job.FinishedAt != nil {
		resp.FinishedAt = job.FinishedAt.UnixMilli()
	}
	if strings.TrimSpace(job.UploadURL) != "" {
		resp.UploadURL = job.UploadURL
	}
	if job.UploadedAt != nil {
		resp.UploadedAt = job.UploadedAt.UnixMilli()
	}
	return c.JSON(resp)
}

func ChatExportTest(c *fiber.Ctx) error {
	return ChatExportCreate(c)
}

func ChatExportUpload(c *fiber.Ctx) error {
	if appConfig == nil || !appConfig.LogUpload.Enabled || strings.TrimSpace(appConfig.LogUpload.Endpoint) == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "未启用云端日志上传"})
	}
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未认证"})
	}
	taskID := strings.TrimSpace(c.Params("taskId"))
	job, err := service.GetMessageExportJob(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "任务不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if job.UserID != user.ID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "无权限访问该任务"})
	}
	var req chatExportUploadRequest
	_ = c.BodyParser(&req)
	opts := service.LogUploadOptions{
		Name:           req.Name,
		Endpoint:       appConfig.LogUpload.Endpoint,
		Token:          appConfig.LogUpload.Token,
		UniformID:      appConfig.LogUpload.UniformID,
		Client:         appConfig.LogUpload.Client,
		Version:        appConfig.LogUpload.Version,
		TimeoutSeconds: appConfig.LogUpload.TimeoutSeconds,
	}
	result, err := service.UploadExportLog(job, opts)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	resp := chatExportUploadResponse{
		URL:      result.URL,
		Name:     result.Name,
		FileName: result.FileName,
	}
	if !result.UploadedAt.IsZero() {
		resp.UploadedAt = result.UploadedAt.UnixMilli()
	}
	return c.JSON(resp)
}

func ChatExportRetry(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未认证"})
	}
	taskID := strings.TrimSpace(c.Params("taskId"))
	job, err := service.GetMessageExportJob(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "任务不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if job.UserID != user.ID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "无权限访问该任务"})
	}
	if err := validateExportChannel(user.ID, job.ChannelID); err != nil {
		return c.Status(mapExportError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	newJob, err := service.RetryMessageExportJob(job)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	sliceLimit, maxConcurrency := service.ExportJobRuntimeSettings(newJob)
	resp := chatExportResponse{
		TaskID:         newJob.ID,
		Status:         newJob.Status,
		Message:        "导出任务已重新创建，请稍候下载。",
		RequestedAt:    newJob.CreatedAt.UnixMilli(),
		SliceLimit:     sliceLimit,
		MaxConcurrency: maxConcurrency,
		DisplayName:    newJob.DisplayName,
	}
	return c.JSON(resp)
}

func ChatExportDelete(c *fiber.Ctx) error {
	user := getCurUser(c)
	if user == nil {
		return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "未认证"})
	}
	taskID := strings.TrimSpace(c.Params("taskId"))
	job, err := service.GetMessageExportJob(taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "任务不存在"})
		}
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if job.UserID != user.ID {
		return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "无权限访问该任务"})
	}
	if job.Status == model.MessageExportStatusPending || job.Status == model.MessageExportStatusProcessing {
		return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "任务进行中，无法删除"})
	}
	fileDeleted, err := service.DeleteMessageExportJob(job)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(chatExportDeleteResponse{
		TaskID:      job.ID,
		FileDeleted: fileDeleted,
	})
}

func apiChatExportTest(ctx *ChatContext, req *chatExportRequest) (any, error) {
	if ctx == nil || ctx.User == nil {
		return nil, fmt.Errorf("未认证")
	}
	resp, err := execChatExportCreate(ctx.User.ID, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func buildChatExportListItem(job *model.MessageExportJobModel) chatExportListItem {
	if job == nil {
		return chatExportListItem{}
	}
	item := chatExportListItem{
		TaskID:      job.ID,
		Format:      job.Format,
		Status:      job.Status,
		DisplayName: job.DisplayName,
		FileName:    job.FileName,
		FileSize:    job.FileSize,
		Message:     job.ErrorMsg,
		RequestedAt: job.CreatedAt.UnixMilli(),
		DownloadURL: fmt.Sprintf("/api/v1/chat/export/%s?download=1", job.ID),
		FileMissing: !exportFileExists(job),
	}
	if job.FinishedAt != nil {
		item.FinishedAt = job.FinishedAt.UnixMilli()
	}
	if strings.TrimSpace(job.UploadURL) != "" {
		item.UploadURL = job.UploadURL
	}
	return item
}

func parseExportStatuses(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	fields := strings.Split(raw, ",")
	var statuses []string
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if normalized, ok := normalizeExportStatus(field); ok {
			if _, exists := seen[normalized]; exists {
				continue
			}
			seen[normalized] = struct{}{}
			statuses = append(statuses, normalized)
		}
	}
	return statuses
}

func normalizeExportStatus(val string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case model.MessageExportStatusPending:
		return model.MessageExportStatusPending, true
	case model.MessageExportStatusProcessing:
		return model.MessageExportStatusProcessing, true
	case model.MessageExportStatusDone:
		return model.MessageExportStatusDone, true
	case model.MessageExportStatusFailed:
		return model.MessageExportStatusFailed, true
	default:
		return "", false
	}
}

func parsePositiveInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func streamExportFile(c *fiber.Ctx, job *model.MessageExportJobModel, fileName string) error {
	file, err := os.Open(job.FilePath)
	if err != nil {
		recordExportDownloadError(job.ID, fmt.Sprintf("打开导出文件失败: %v", err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "导出文件缺失或无法打开，请重新导出"})
	}
	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		recordExportDownloadError(job.ID, fmt.Sprintf("读取导出文件信息失败: %v", err))
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "导出文件读取失败，请重新导出"})
	}
	if stat.IsDir() {
		_ = file.Close()
		recordExportDownloadError(job.ID, "导出路径指向目录")
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "导出文件异常，请重新导出"})
	}

	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	c.Attachment(fileName)

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer file.Close()
		buf := make([]byte, 64*1024)
		for {
			n, readErr := file.Read(buf)
			if n > 0 {
				service.WaitExportBandwidth(n)
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					log.Printf("export: 任务 %s 写入失败: %v", job.ID, writeErr)
					return
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					log.Printf("export: 任务 %s 读取失败: %v", job.ID, readErr)
					recordExportDownloadError(job.ID, fmt.Sprintf("下载过程中断: %v", readErr))
				}
				return
			}
		}
	})
	return nil
}

func recordExportDownloadError(jobID, message string) {
	if err := service.UpdateExportJobErrorMessage(jobID, message); err != nil {
		log.Printf("export: 更新任务 %s 错误信息失败: %v", jobID, err)
	}
}

func resolveDownloadFileName(job *model.MessageExportJobModel) string {
	if job == nil {
		return "export"
	}
	if fileName := strings.TrimSpace(job.FileName); fileName != "" {
		return fileName
	}
	displayName := strings.TrimSpace(job.DisplayName)
	if displayName != "" {
		if !strings.Contains(displayName, ".") {
			ext := strings.TrimSpace(job.Format)
			if ext == "" {
				ext = "txt"
			}
			return fmt.Sprintf("%s.%s", displayName, ext)
		}
		return displayName
	}
	format := strings.TrimSpace(job.Format)
	if format == "" {
		format = "txt"
	}
	return fmt.Sprintf("%s.%s", job.ChannelID, format)
}

func exportFileExists(job *model.MessageExportJobModel) bool {
	if job == nil {
		return false
	}
	path := strings.TrimSpace(job.FilePath)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func mapExportError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "未认证"):
		return http.StatusUnauthorized
	case strings.Contains(msg, "权限"):
		return http.StatusForbidden
	case strings.Contains(msg, "不存在"):
		return http.StatusNotFound
	default:
		return http.StatusBadRequest
	}
}
