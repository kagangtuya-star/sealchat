package service

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

// ChatImportJobManager 导入任务管理器
type ChatImportJobManager struct {
	mu   sync.RWMutex
	jobs map[string]*model.ChatImportJobModel
}

var chatImportJobManager = &ChatImportJobManager{
	jobs: make(map[string]*model.ChatImportJobModel),
}

// ChatImportProgressEvent 进度事件
type ChatImportProgressEvent struct {
	JobID          string `json:"jobId"`
	ChannelID      string `json:"channelId"`
	Status         string `json:"status"`
	TotalLines     int    `json:"totalLines"`
	ProcessedLines int    `json:"processedLines"`
	ImportedCount  int    `json:"importedCount"`
	SkippedCount   int    `json:"skippedCount"`
	ErrorMessage   string `json:"errorMessage,omitempty"`
	Percentage     int    `json:"percentage"`
}

// 进度广播 channel
var (
	importProgressChan  = make(chan *ChatImportProgressEvent, 100)
	importProgressSubMu sync.RWMutex
	importProgressSubs  = make(map[chan *ChatImportProgressEvent]struct{})
)

// SubscribeImportProgress 订阅导入进度
func SubscribeImportProgress() chan *ChatImportProgressEvent {
	ch := make(chan *ChatImportProgressEvent, 10)
	importProgressSubMu.Lock()
	importProgressSubs[ch] = struct{}{}
	importProgressSubMu.Unlock()
	return ch
}

// UnsubscribeImportProgress 取消订阅
func UnsubscribeImportProgress(ch chan *ChatImportProgressEvent) {
	importProgressSubMu.Lock()
	delete(importProgressSubs, ch)
	importProgressSubMu.Unlock()
	close(ch)
}

// broadcastProgress 广播进度到所有订阅者
func broadcastProgress(event *ChatImportProgressEvent) {
	importProgressSubMu.RLock()
	defer importProgressSubMu.RUnlock()
	for ch := range importProgressSubs {
		select {
		case ch <- event:
		default:
			// channel 满了就跳过，避免阻塞
		}
	}
}

// GetChatImportJob 获取导入任务
func GetChatImportJob(jobID string) *model.ChatImportJobModel {
	chatImportJobManager.mu.RLock()
	defer chatImportJobManager.mu.RUnlock()
	return chatImportJobManager.jobs[jobID]
}

// ChatImportJobProgress 任务进度信息
type ChatImportJobProgress struct {
	JobID          string `json:"jobId"`
	Status         string `json:"status"`
	TotalLines     int    `json:"totalLines"`
	ProcessedLines int    `json:"processedLines"`
	ImportedCount  int    `json:"importedCount"`
	SkippedCount   int    `json:"skippedCount"`
	ErrorMessage   string `json:"errorMessage,omitempty"`
	Percentage     int    `json:"percentage"`
}

// ChatImportExecute 执行聊天日志导入
func ChatImportExecute(channelID string, userID string, req *model.ChatImportExecuteRequest) (*model.ChatImportJobModel, error) {
	// 验证频道和权限
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil {
		return nil, errors.New("频道不存在")
	}

	// 检查世界管理员权限
	worldID := channel.WorldID
	if worldID == "" {
		return nil, errors.New("仅支持世界频道导入")
	}
	if !IsWorldAdmin(worldID, userID) {
		return nil, errors.New("仅世界管理员可导入聊天记录")
	}

	// 序列化配置
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		return nil, err
	}

	// 创建导入任务
	job := &model.ChatImportJobModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID: utils.NewID(),
		},
		ChannelID:  channelID,
		WorldID:    worldID,
		UserID:     userID,
		Status:     model.ChatImportStatusPending,
		ConfigJSON: string(configJSON),
	}

	// 保存任务到内存（快速访问）
	chatImportJobManager.mu.Lock()
	chatImportJobManager.jobs[job.ID] = job
	chatImportJobManager.mu.Unlock()

	// 保存任务到数据库
	if err := model.GetDB().Create(job).Error; err != nil {
		return nil, err
	}

	// 异步执行导入
	go executeImportJob(job, req.Content, req.Config)

	return job, nil
}

// executeImportJob 异步执行导入任务
func executeImportJob(job *model.ChatImportJobModel, content string, config *model.ChatImportConfig) {
	startTime := time.Now()
	job.StartedAt = &startTime
	job.Status = model.ChatImportStatusRunning
	updateJobStatus(job)

	defer func() {
		if r := recover(); r != nil {
			job.Status = model.ChatImportStatusFailed
			job.ErrorMessage = "导入过程发生错误"
			finishTime := time.Now()
			job.FinishedAt = &finishTime
			updateJobStatus(job)
		}
	}()

	// 解析日志
	parser, err := NewChatLogParser(config)
	if err != nil {
		job.Status = model.ChatImportStatusFailed
		job.ErrorMessage = "解析配置错误: " + err.Error()
		finishTime := time.Now()
		job.FinishedAt = &finishTime
		updateJobStatus(job)
		return
	}

	entries, totalLines, skippedCount := parser.ParseLogContent(content)
	job.TotalLines = totalLines
	job.SkippedCount = skippedCount
	updateJobStatus(job)

	if len(entries) == 0 {
		job.Status = model.ChatImportStatusDone
		job.ErrorMessage = "没有可导入的消息"
		finishTime := time.Now()
		job.FinishedAt = &finishTime
		updateJobStatus(job)
		return
	}

	// 创建或获取角色身份映射
	identityMap, err := resolveImportIdentities(job.ChannelID, job.UserID, entries, config.RoleMapping)
	if err != nil {
		job.Status = model.ChatImportStatusFailed
		job.ErrorMessage = "角色身份创建失败: " + err.Error()
		finishTime := time.Now()
		job.FinishedAt = &finishTime
		updateJobStatus(job)
		return
	}

	// 批量插入消息
	batchSize := 500
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[i:end]
		importedCount, err := batchInsertImportedMessages(job.ChannelID, batch, identityMap)
		if err != nil {
			log.Printf("批量插入消息失败: %v", err)
			job.Status = model.ChatImportStatusFailed
			job.ErrorMessage = "消息插入失败: " + err.Error()
			finishTime := time.Now()
			job.FinishedAt = &finishTime
			updateJobStatus(job)
			return
		}

		job.ProcessedLines += len(batch)
		job.ImportedCount += importedCount
		updateJobStatus(job)
	}

	job.Status = model.ChatImportStatusDone
	finishTime := time.Now()
	job.FinishedAt = &finishTime
	updateJobStatus(job)
}

// updateJobStatus 更新任务状态
func updateJobStatus(job *model.ChatImportJobModel) {
	// 更新内存中的状态
	chatImportJobManager.mu.Lock()
	chatImportJobManager.jobs[job.ID] = job
	chatImportJobManager.mu.Unlock()

	// 更新数据库
	updates := map[string]interface{}{
		"status":          job.Status,
		"total_lines":     job.TotalLines,
		"processed_lines": job.ProcessedLines,
		"imported_count":  job.ImportedCount,
		"skipped_count":   job.SkippedCount,
		"error_message":   job.ErrorMessage,
		"started_at":      job.StartedAt,
		"finished_at":     job.FinishedAt,
	}
	model.GetDB().Model(&model.ChatImportJobModel{}).Where("id = ?", job.ID).Updates(updates)

	// 通过 channel 广播进度
	percentage := 0
	if job.TotalLines > 0 {
		percentage = int(float64(job.ProcessedLines) / float64(job.TotalLines) * 100)
	}
	broadcastProgress(&ChatImportProgressEvent{
		JobID:          job.ID,
		ChannelID:      job.ChannelID,
		Status:         job.Status,
		TotalLines:     job.TotalLines,
		ProcessedLines: job.ProcessedLines,
		ImportedCount:  job.ImportedCount,
		SkippedCount:   job.SkippedCount,
		ErrorMessage:   job.ErrorMessage,
		Percentage:     percentage,
	})
}

// resolveImportIdentities 解析或创建导入所需的角色身份
func resolveImportIdentities(channelID string, userID string, entries []*model.ParsedLogEntry, roleMapping map[string]*model.ChatImportRoleMappingConfig) (map[string]*model.ChannelIdentityModel, error) {
	identityMap := make(map[string]*model.ChannelIdentityModel)
	roleNames := ExtractRoleNames(entries)

	for _, roleName := range roleNames {
		var err error

		mappingConfig := roleMapping[roleName]

		var templateIdentity *model.ChannelIdentityModel
		if mappingConfig != nil && mappingConfig.ReuseIdentityID != "" {
			templateIdentity, err = model.ChannelIdentityGetByID(mappingConfig.ReuseIdentityID)
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}

		if templateIdentity != nil && templateIdentity.ChannelID == channelID {
			// 目标频道已存在该身份，直接复用，不再新建
			identityMap[roleName] = templateIdentity
			continue
		}

		// 创建新身份
		bindUserID := userID // 默认绑定到导入执行者
		if templateIdentity != nil && templateIdentity.UserID != "" {
			bindUserID = templateIdentity.UserID
		}
		if mappingConfig != nil && mappingConfig.BindToUserID != "" {
			bindUserID = mappingConfig.BindToUserID
		}

		displayName := roleName
		if templateIdentity != nil && templateIdentity.DisplayName != "" {
			displayName = templateIdentity.DisplayName
		}
		if mappingConfig != nil && mappingConfig.DisplayName != "" {
			displayName = mappingConfig.DisplayName
		}

		color := ""
		if templateIdentity != nil && templateIdentity.Color != "" {
			color = templateIdentity.Color
		}
		if mappingConfig != nil && mappingConfig.Color != "" {
			color = mappingConfig.Color
		}

		avatarID := ""
		if templateIdentity != nil && templateIdentity.AvatarAttachmentID != "" {
			avatarID = templateIdentity.AvatarAttachmentID
		}
		if mappingConfig != nil && mappingConfig.AvatarAttachmentID != "" {
			avatarID = mappingConfig.AvatarAttachmentID
		}

		identity, err := createImportIdentity(channelID, bindUserID, displayName, color, avatarID)
		if err != nil {
			return nil, err
		}

		identityMap[roleName] = identity
	}

	return identityMap, nil
}

// createImportIdentity 创建导入用的角色身份
func createImportIdentity(channelID, userID, displayName, color, avatarID string) (*model.ChannelIdentityModel, error) {
	if displayName == "" {
		displayName = "未知角色"
	}
	if len([]rune(displayName)) > 32 {
		displayName = string([]rune(displayName)[:32])
	}

	if color != "" {
		color = model.ChannelIdentityNormalizeColor(color)
	}

	sortMax, _ := model.ChannelIdentityMaxSort(channelID, userID)

	identity := &model.ChannelIdentityModel{
		ChannelID:          channelID,
		UserID:             userID,
		DisplayName:        strings.TrimSpace(displayName),
		Color:              color,
		AvatarAttachmentID: avatarID,
		SortOrder:          sortMax + 1,
		IsDefault:          false,
		IsHidden:           false,
	}

	if err := model.ChannelIdentityUpsert(identity); err != nil {
		return nil, err
	}

	return identity, nil
}

// batchInsertImportedMessages 批量插入导入的消息
func batchInsertImportedMessages(channelID string, entries []*model.ParsedLogEntry, identityMap map[string]*model.ChannelIdentityModel) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	messages := make([]*model.MessageModel, 0, len(entries))

	for _, entry := range entries {
		identity := identityMap[entry.RoleName]
		if identity == nil {
			continue
		}

		var createdAt time.Time
		var displayOrder float64

		if entry.Timestamp != nil {
			createdAt = *entry.Timestamp
			displayOrder = float64(entry.Timestamp.UnixMilli())
		} else {
			createdAt = time.Now()
			displayOrder = float64(createdAt.UnixMilli())
		}

		icMode := "ic"
		if entry.IsOOC {
			icMode = "ooc"
		}

		msg := &model.MessageModel{
			StringPKBaseModel: model.StringPKBaseModel{
				ID:        utils.NewID(),
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			},
			Content:             strings.TrimSpace(entry.Content),
			ChannelID:           channelID,
			UserID:              identity.UserID,
			DisplayOrder:        displayOrder,
			ICMode:              icMode,
			SenderIdentityID:    identity.ID,
			SenderIdentityName:  identity.DisplayName,
			SenderIdentityColor: identity.Color,
			SenderMemberName:    identity.DisplayName,
			SenderRoleID:        identity.ID,
		}

		if identity.AvatarAttachmentID != "" {
			msg.SenderIdentityAvatarID = identity.AvatarAttachmentID
		}

		messages = append(messages, msg)
	}

	if len(messages) == 0 {
		return 0, nil
	}

	// 批量插入
	if err := model.GetDB().CreateInBatches(messages, 100).Error; err != nil {
		return 0, err
	}

	return len(messages), nil
}

// GetChatImportJobStatus 获取任务状态
func GetChatImportJobStatus(jobID string) (*ChatImportJobProgress, error) {
	// 先从内存获取
	job := GetChatImportJob(jobID)
	if job == nil {
		// 从数据库获取
		job = &model.ChatImportJobModel{}
		if err := model.GetDB().Where("id = ?", jobID).First(job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("任务不存在")
			}
			return nil, err
		}
	}

	percentage := 0
	if job.TotalLines > 0 {
		percentage = int(float64(job.ProcessedLines) / float64(job.TotalLines) * 100)
	}

	return &ChatImportJobProgress{
		JobID:          job.ID,
		Status:         job.Status,
		TotalLines:     job.TotalLines,
		ProcessedLines: job.ProcessedLines,
		ImportedCount:  job.ImportedCount,
		SkippedCount:   job.SkippedCount,
		ErrorMessage:   job.ErrorMessage,
		Percentage:     percentage,
	}, nil
}

// ListReusableIdentities 获取用户在当前世界可复用身份
func ListReusableIdentities(worldID, currentChannelID, userID string, channelIDs []string, includeCurrent bool, visibleOnly bool) ([]*model.ChannelIdentityModel, error) {
	channelIDSet := make(map[string]struct{})
	for _, id := range channelIDs {
		if id == "" {
			continue
		}
		channelIDSet[id] = struct{}{}
	}

	// 获取世界内频道（默认排除私密频道）
	var channels []*model.ChannelModel
	db := model.GetDB().Where("world_id = ? AND is_private = ?", worldID, false)
	if len(channelIDSet) > 0 {
		ids := make([]string, 0, len(channelIDSet))
		for id := range channelIDSet {
			ids = append(ids, id)
		}
		db = db.Where("id IN ?", ids)
	}
	if !includeCurrent {
		db = db.Where("id != ?", currentChannelID)
	}
	if err := db.Find(&channels).Error; err != nil {
		return nil, err
	}

	if len(channels) == 0 {
		return []*model.ChannelIdentityModel{}, nil
	}

	// 获取频道ID列表
	filteredChannelIDs := make([]string, len(channels))
	for i, ch := range channels {
		filteredChannelIDs[i] = ch.ID
	}

	// 查询用户在这些频道的身份
	query := model.GetDB().Where("user_id = ? AND channel_id IN ?", userID, filteredChannelIDs).
		Order("updated_at DESC")
	if visibleOnly {
		query = query.Where("is_hidden = ? OR is_hidden IS NULL", false)
	}

	var identities []*model.ChannelIdentityModel
	if err := query.Find(&identities).Error; err != nil {
		return nil, err
	}

	return identities, nil
}
