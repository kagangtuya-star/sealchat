package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
)

type chatExportRequest struct {
	ChannelID        string   `json:"channel_id"`
	Format           string   `json:"format"`
	TimeRange        []int64  `json:"time_range"`
	IncludeOOC       *bool    `json:"include_ooc"`
	IncludeArchived  *bool    `json:"include_archived"`
	WithoutTimestamp *bool    `json:"without_timestamp"`
	MergeMessages    *bool    `json:"merge_messages"`
	Users            []string `json:"users"`
}

type chatExportResponse struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	RequestedAt int64  `json:"requested_at"`
}

type chatExportStatusResponse struct {
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	FileName   string `json:"file_name"`
	Message    string `json:"message"`
	FinishedAt int64  `json:"finished_at,omitempty"`
	UploadURL  string `json:"upload_url,omitempty"`
	UploadedAt int64  `json:"uploaded_at,omitempty"`
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

	job, err := service.CreateMessageExportJob(&service.ExportJobOptions{
		UserID:           userID,
		ChannelID:        channelID,
		Format:           format,
		IncludeOOC:       includeOOC,
		IncludeArchived:  includeArchived,
		WithoutTimestamp: withoutTimestamp,
		MergeMessages:    mergeMessages,
		StartTime:        start,
		EndTime:          end,
	})
	if err != nil {
		return nil, err
	}
	return &chatExportResponse{
		TaskID:      job.ID,
		Status:      job.Status,
		Message:     "导出任务已创建，请稍后下载。",
		RequestedAt: job.CreatedAt.UnixMilli(),
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
			return c.Status(http.StatusConflict).JSON(fiber.Map{"error": "任务尚未完成"})
		}
		fileName := job.FileName
		if strings.TrimSpace(fileName) == "" {
			fileName = fmt.Sprintf("%s.%s", job.ChannelID, job.Format)
		}
		return c.Download(job.FilePath, fileName)
	}

	resp := chatExportStatusResponse{
		TaskID:   job.ID,
		Status:   job.Status,
		FileName: job.FileName,
		Message:  job.ErrorMsg,
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
