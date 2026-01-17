package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
)

const (
	messageExportLimit = 65535
	defaultExportTZ    = "2006-01-02 15:04"

	// 导出切片与并发控制的全局限制，需与前端/接口保持一致
	DefaultExportSliceLimit = 5000
	MinExportSliceLimit     = 1000
	MaxExportSliceLimit     = 20000

	DefaultExportConcurrency = 2
	MinExportConcurrency     = 1
	MaxExportConcurrency     = 8

	exportListDefaultLimit = 20
	exportListMaxLimit     = 100
)

var supportedExportFormats = map[string]struct{}{
	"json": {},
	"txt":  {},
	"html": {},
}

// ExportJobOptions 聚合创建导出任务所需的信息。
type ExportJobOptions struct {
	UserID             string
	ChannelID          string
	Format             string
	DisplayName        string
	IncludeOOC         bool
	IncludeArchived    bool
	WithoutTimestamp   bool
	MergeMessages      bool
	StartTime          *time.Time
	EndTime            *time.Time
	DisplaySettings    map[string]any
	SliceLimit         int
	MaxConcurrency     int
	TextColorizeBBCode bool
}

type exportExtraOptions struct {
	DisplaySettings    map[string]any `json:"display,omitempty"`
	SliceLimit         int            `json:"slice_limit,omitempty"`
	MaxConcurrency     int            `json:"max_concurrency,omitempty"`
	TextColorizeBBCode bool           `json:"text_colorize_bbcode,omitempty"`
}

func normalizeExportFormat(format string) (string, bool) {
	f := strings.ToLower(strings.TrimSpace(format))
	_, ok := supportedExportFormats[f]
	return f, ok
}

// CreateMessageExportJob 持久化导出任务并返回记录。
func CreateMessageExportJob(opts *ExportJobOptions) (*model.MessageExportJobModel, error) {
	if opts == nil {
		return nil, fmt.Errorf("导出参数不能为空")
	}
	format, ok := normalizeExportFormat(opts.Format)
	if !ok {
		return nil, fmt.Errorf("不支持的导出格式: %s", opts.Format)
	}

	opts.SliceLimit = NormalizeExportSliceLimit(opts.SliceLimit)
	opts.MaxConcurrency = NormalizeExportConcurrency(opts.MaxConcurrency)
	extraOptions, err := buildExportExtraOptions(opts)
	if err != nil {
		return nil, err
	}

	job := &model.MessageExportJobModel{
		UserID:           opts.UserID,
		ChannelID:        opts.ChannelID,
		Format:           format,
		DisplayName:      normalizeExportDisplayName(opts.DisplayName),
		IncludeOOC:       opts.IncludeOOC,
		IncludeArchived:  opts.IncludeArchived,
		WithoutTimestamp: opts.WithoutTimestamp,
		MergeMessages:    opts.MergeMessages,
		StartTime:        opts.StartTime,
		EndTime:          opts.EndTime,
		Status:           model.MessageExportStatusPending,
		ExtraOptions:     extraOptions,
	}

	if err := model.GetDB().Create(job).Error; err != nil {
		return nil, err
	}
	return job, nil
}

// GetMessageExportJob 获取任务详情。
func GetMessageExportJob(jobID string) (*model.MessageExportJobModel, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var job model.MessageExportJobModel
	if err := model.GetDB().Where("id = ?", jobID).Limit(1).Find(&job).Error; err != nil {
		return nil, err
	}
	if job.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &job, nil
}

func loadMessagesForExport(job *model.MessageExportJobModel) ([]*model.MessageModel, error) {
	if job == nil {
		return nil, fmt.Errorf("任务不存在")
	}
	db := model.GetDB()
	query := db.Model(&model.MessageModel{}).
		Where("channel_id = ?", job.ChannelID).
		Where("is_revoked = ?", false).
		Where("is_deleted = ?", false).
		Preload("Member").
		Preload("User")

	if job.StartTime != nil {
		query = query.Where("created_at >= ?", *job.StartTime)
	}
	if job.EndTime != nil {
		query = query.Where("created_at <= ?", *job.EndTime)
	}
	if !job.IncludeArchived {
		query = query.Where("is_archived = ?", false)
	}
	if !job.IncludeOOC {
		query = query.Where("COALESCE(ic_mode, 'ic') != ?", "ooc")
	}

	query = query.Order("display_order asc").Order("created_at asc").Limit(messageExportLimit)

	var messages []*model.MessageModel
	if err := query.Find(&messages).Error; err != nil {
		return nil, err
	}
	if job.MergeMessages {
		return mergeSequentialMessages(messages), nil
	}
	return messages, nil
}

func mergeSequentialMessages(messages []*model.MessageModel) []*model.MessageModel {
	if len(messages) == 0 {
		return messages
	}
	const mergeWindow = 60 * time.Second
	var result []*model.MessageModel
	var current *model.MessageModel
	var lastTime time.Time
	var currentIcMode string
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		formatted := formatContentForMerge(msg)
		if current == nil {
			current = cloneMessage(msg)
			current.Content = formatted
			lastTime = msg.CreatedAt
			currentIcMode = normalizeIcMode(msg.ICMode)
			result = append(result, current)
			continue
		}
		if canMerge(current, currentIcMode, lastTime, msg, mergeWindow) {
			nextContent := formatted
			trimmed := strings.TrimRight(current.Content, " \n")
			if trimmed == "" {
				current.Content = nextContent
			} else {
				current.Content = trimmed + "\n" + nextContent
			}
			lastTime = msg.CreatedAt
			continue
		}
		current = cloneMessage(msg)
		current.Content = formatted
		lastTime = msg.CreatedAt
		currentIcMode = normalizeIcMode(msg.ICMode)
		result = append(result, current)
	}
	return result
}

func canMerge(base *model.MessageModel, currentIcMode string, last time.Time, next *model.MessageModel, window time.Duration) bool {
	if base == nil || next == nil {
		return false
	}
	if !sameSenderIdentity(base, next) {
		return false
	}
	if currentIcMode != normalizeIcMode(next.ICMode) {
		return false
	}
	if base.IsWhisper != next.IsWhisper {
		return false
	}
	if base.IsArchived != next.IsArchived {
		return false
	}
	diff := next.CreatedAt.Sub(last)
	if diff < 0 {
		diff = -diff
	}
	return diff <= window
}

func sameSenderIdentity(a, b *model.MessageModel) bool {
	return senderKey(a) != "" && senderKey(a) == senderKey(b)
}

func senderKey(msg *model.MessageModel) string {
	if msg == nil {
		return ""
	}
	if id := strings.TrimSpace(msg.SenderIdentityID); id != "" {
		return "identity:" + id
	}
	if user := strings.TrimSpace(msg.UserID); user != "" {
		return "user:" + user
	}
	return ""
}

func normalizeIcMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return "ic"
	}
	return mode
}

func buildExportExtraOptions(opts *ExportJobOptions) (string, error) {
	if opts == nil {
		return "", nil
	}
	extra := exportExtraOptions{
		SliceLimit:         opts.SliceLimit,
		MaxConcurrency:     opts.MaxConcurrency,
		TextColorizeBBCode: opts.TextColorizeBBCode,
	}
	if len(opts.DisplaySettings) > 0 {
		extra.DisplaySettings = opts.DisplaySettings
	}
	if extra.DisplaySettings == nil && extra.SliceLimit == 0 && extra.MaxConcurrency == 0 {
		return "", nil
	}
	data, err := json.Marshal(extra)
	if err != nil {
		return "", fmt.Errorf("导出附加参数序列化失败: %w", err)
	}
	return string(data), nil
}

func parseExportExtraOptions(raw string) *exportExtraOptions {
	extra := &exportExtraOptions{
		SliceLimit:     DefaultExportSliceLimit,
		MaxConcurrency: DefaultExportConcurrency,
	}
	if strings.TrimSpace(raw) == "" {
		return extra
	}
	if err := json.Unmarshal([]byte(raw), extra); err != nil {
		// 格式异常时退回默认值
		extra.SliceLimit = DefaultExportSliceLimit
		extra.MaxConcurrency = DefaultExportConcurrency
		return extra
	}
	extra.SliceLimit = NormalizeExportSliceLimit(extra.SliceLimit)
	extra.MaxConcurrency = NormalizeExportConcurrency(extra.MaxConcurrency)
	if extra.DisplaySettings != nil && len(extra.DisplaySettings) == 0 {
		extra.DisplaySettings = nil
	}
	return extra
}

func cloneMessage(msg *model.MessageModel) *model.MessageModel {
	if msg == nil {
		return nil
	}
	clone := *msg
	return &clone
}

func formatContentForMerge(msg *model.MessageModel) string {
	if msg == nil {
		return ""
	}
	if strings.EqualFold(normalizeIcMode(msg.ICMode), "ooc") {
		return ensureOOCWrapped(msg.Content)
	}
	return msg.Content
}

func ensureOOCWrapped(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "（）"
	}
	if (strings.HasPrefix(trimmed, "（") && strings.HasSuffix(trimmed, "）")) ||
		(strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")")) {
		return trimmed
	}
	return fmt.Sprintf("（%s）", trimmed)
}
func NormalizeExportSliceLimit(value int) int {
	if value <= 0 {
		value = DefaultExportSliceLimit
	}
	if value < MinExportSliceLimit {
		return MinExportSliceLimit
	}
	if value > MaxExportSliceLimit {
		return MaxExportSliceLimit
	}
	return value
}

func NormalizeExportConcurrency(value int) int {
	if value <= 0 {
		value = DefaultExportConcurrency
	}
	if value < MinExportConcurrency {
		return MinExportConcurrency
	}
	if value > MaxExportConcurrency {
		return MaxExportConcurrency
	}
	return value
}

// ListMessageExportJobs 返回指定频道的导出任务及统计信息。
func ListMessageExportJobs(channelID string, statuses []string, limit, offset int, keyword string) ([]*model.MessageExportJobModel, int64, int64, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, 0, 0, fmt.Errorf("channel_id 不能为空")
	}
	if limit <= 0 {
		limit = exportListDefaultLimit
	}
	if limit > exportListMaxLimit {
		limit = exportListMaxLimit
	}
	if offset < 0 {
		offset = 0
	}

	base := model.GetDB().Model(&model.MessageExportJobModel{}).Where("channel_id = ?", channelID)
	if len(statuses) > 0 {
		base = base.Where("status IN ?", statuses)
	}
	if trimmed := strings.TrimSpace(keyword); trimmed != "" {
		pattern := fmt.Sprintf("%%%s%%", trimmed)
		base = base.Where("(display_name LIKE ? OR file_name LIKE ?)", pattern, pattern)
	}

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	var jobs []*model.MessageExportJobModel
	query := base.Session(&gorm.Session{}).Order("finished_at DESC, created_at DESC").Limit(limit).Offset(offset)
	if err := query.Find(&jobs).Error; err != nil {
		return nil, 0, 0, err
	}

	totalSize := int64(0)
	for _, job := range jobs {
		if exportJobFileExists(job) {
			totalSize += job.FileSize
		}
	}

	return jobs, total, totalSize, nil
}

// UpdateExportJobErrorMessage 只更新任务的错误描述，用于下载阶段反馈。
func UpdateExportJobErrorMessage(jobID, message string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return fmt.Errorf("job_id 不能为空")
	}
	updates := map[string]any{
		"error_msg":  strings.TrimSpace(message),
		"updated_at": time.Now(),
	}
	return model.GetDB().Model(&model.MessageExportJobModel{}).
		Where("id = ?", jobID).
		Updates(updates).Error
}

func normalizeExportDisplayName(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	const maxLen = 120
	runes := []rune(trimmed)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return trimmed
}

func exportJobFileExists(job *model.MessageExportJobModel) bool {
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

// RetryMessageExportJob 根据既有任务重新创建一条导出任务。
func RetryMessageExportJob(job *model.MessageExportJobModel) (*model.MessageExportJobModel, error) {
	if job == nil {
		return nil, fmt.Errorf("任务不存在")
	}
	opts := &ExportJobOptions{
		UserID:           job.UserID,
		ChannelID:        job.ChannelID,
		Format:           job.Format,
		DisplayName:      job.DisplayName,
		IncludeOOC:       job.IncludeOOC,
		IncludeArchived:  job.IncludeArchived,
		WithoutTimestamp: job.WithoutTimestamp,
		MergeMessages:    job.MergeMessages,
		StartTime:        job.StartTime,
		EndTime:          job.EndTime,
	}
	extra := parseExportExtraOptions(job.ExtraOptions)
	opts.DisplaySettings = extra.DisplaySettings
	opts.SliceLimit = extra.SliceLimit
	opts.MaxConcurrency = extra.MaxConcurrency
	return CreateMessageExportJob(opts)
}

// DeleteMessageExportJob 删除导出任务记录并清理本地文件。
func DeleteMessageExportJob(job *model.MessageExportJobModel) (bool, error) {
	if job == nil {
		return false, fmt.Errorf("任务不存在")
	}
	fileDeleted := false
	path := strings.TrimSpace(job.FilePath)
	if path != "" {
		info, err := os.Stat(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return false, err
			}
		} else if info.IsDir() {
			return false, fmt.Errorf("导出路径异常")
		} else if err := os.Remove(path); err != nil {
			if !os.IsNotExist(err) {
				return false, err
			}
		} else {
			fileDeleted = true
		}
	}
	if err := model.GetDB().Delete(&model.MessageExportJobModel{}, "id = ?", job.ID).Error; err != nil {
		return fileDeleted, err
	}
	return fileDeleted, nil
}

// ExportJobRuntimeSettings 返回任务当前的分页与并发配置。
func ExportJobRuntimeSettings(job *model.MessageExportJobModel) (int, int) {
	extra := parseExportExtraOptions("")
	if job != nil {
		extra = parseExportExtraOptions(job.ExtraOptions)
	}
	return extra.SliceLimit, extra.MaxConcurrency
}
