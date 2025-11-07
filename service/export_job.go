package service

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
)

const (
	messageExportLimit = 65535
	defaultExportTZ    = "2006-01-02 15:04"
)

var supportedExportFormats = map[string]struct{}{
	"json": {},
	"txt":  {},
	"html": {},
	"docx": {},
}

// ExportJobOptions 聚合创建导出任务所需的信息。
type ExportJobOptions struct {
	UserID           string
	ChannelID        string
	Format           string
	IncludeOOC       bool
	IncludeArchived  bool
	WithoutTimestamp bool
	MergeMessages    bool
	StartTime        *time.Time
	EndTime          *time.Time
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

	job := &model.MessageExportJobModel{
		UserID:           opts.UserID,
		ChannelID:        opts.ChannelID,
		Format:           format,
		IncludeOOC:       opts.IncludeOOC,
		IncludeArchived:  opts.IncludeArchived,
		WithoutTimestamp: opts.WithoutTimestamp,
		MergeMessages:    opts.MergeMessages,
		StartTime:        opts.StartTime,
		EndTime:          opts.EndTime,
		Status:           model.MessageExportStatusPending,
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
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		if current == nil {
			current = cloneMessage(msg)
			lastTime = msg.CreatedAt
			result = append(result, current)
			continue
		}
		if canMerge(current, lastTime, msg, mergeWindow) {
			current.Content = strings.TrimRight(current.Content, " \n") + "\n" + msg.Content
			lastTime = msg.CreatedAt
			continue
		}
		current = cloneMessage(msg)
		lastTime = msg.CreatedAt
		result = append(result, current)
	}
	return result
}

func canMerge(base *model.MessageModel, last time.Time, next *model.MessageModel, window time.Duration) bool {
	if base == nil || next == nil {
		return false
	}
	if !sameSenderIdentity(base, next) {
		return false
	}
	if normalizeIcMode(base.ICMode) != normalizeIcMode(next.ICMode) {
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
	idA := strings.TrimSpace(a.SenderIdentityID)
	idB := strings.TrimSpace(b.SenderIdentityID)
	if idA != "" || idB != "" {
		return idA != "" && idA == idB
	}
	return strings.TrimSpace(a.UserID) == strings.TrimSpace(b.UserID)
}

func normalizeIcMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return "ic"
	}
	return mode
}

func cloneMessage(msg *model.MessageModel) *model.MessageModel {
	if msg == nil {
		return nil
	}
	clone := *msg
	return &clone
}
