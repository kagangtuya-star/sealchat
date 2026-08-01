package service

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"sealchat/model"
)

type MessageExportWorkerConfig struct {
	StorageDir          string
	HTMLPageSizeDefault int
	HTMLPageSizeMax     int
	HTMLMaxConcurrency  int
}

var (
	exportWorkerOnce sync.Once
	filenameSafeRe   = regexp.MustCompile(`[^0-9A-Za-z一-龥_-]+`)
)

// StartMessageExportWorker 启动后台导出任务处理协程。
func StartMessageExportWorker(cfg MessageExportWorkerConfig) {
	if cfg.StorageDir == "" {
		cfg.StorageDir = "./data/exports"
	}
	if cfg.HTMLPageSizeDefault <= 0 {
		cfg.HTMLPageSizeDefault = DefaultExportSliceLimit
	}
	if cfg.HTMLPageSizeMax <= 0 {
		cfg.HTMLPageSizeMax = MaxExportSliceLimit
	}
	if cfg.HTMLPageSizeDefault > cfg.HTMLPageSizeMax {
		cfg.HTMLPageSizeDefault = cfg.HTMLPageSizeMax
	}
	if cfg.HTMLMaxConcurrency <= 0 {
		cfg.HTMLMaxConcurrency = DefaultExportConcurrency
	}
	exportWorkerOnce.Do(func() {
		if err := os.MkdirAll(cfg.StorageDir, 0755); err != nil {
			log.Printf("export: 创建导出目录失败: %v", err)
		}
		go runMessageExportWorker(cfg)
	})
}

func runMessageExportWorker(cfg MessageExportWorkerConfig) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		job, err := acquireNextExportJob()
		if err != nil {
			log.Printf("export: 获取任务失败: %v", err)
			<-ticker.C
			continue
		}
		if job == nil {
			<-ticker.C
			continue
		}
		if err := processExportJob(job, cfg); err != nil {
			log.Printf("export: 执行任务 %s 失败: %v", job.ID, err)
		}
	}
}

func acquireNextExportJob() (*model.MessageExportJobModel, error) {
	db := model.GetDB()
	var job model.MessageExportJobModel
	if err := db.Where("status = ?", model.MessageExportStatusPending).
		Order("created_at asc").
		Limit(1).
		Find(&job).Error; err != nil {
		return nil, err
	}
	if job.ID == "" {
		return nil, nil
	}
	res := db.Model(&model.MessageExportJobModel{}).
		Where("id = ? AND status = ?", job.ID, model.MessageExportStatusPending).
		Updates(map[string]any{
			"status":     model.MessageExportStatusProcessing,
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil
	}
	job.Status = model.MessageExportStatusProcessing
	return &job, nil
}

func processExportJob(job *model.MessageExportJobModel, cfg MessageExportWorkerConfig) error {
	extraOptions := parseExportExtraOptions(job.ExtraOptions)
	if len(extraOptions.BatchChannelIDs) > 0 {
		if err := processBatchExportJob(job, cfg, extraOptions); err != nil {
			_ = markJobFailed(job, err)
			return err
		}
		return nil
	}
	channelName := resolveChannelName(job.ChannelID)
	messages, err := loadMessagesForExport(job)
	if err != nil {
		_ = markJobFailed(job, err)
		return err
	}

	if strings.EqualFold(job.Format, "html") {
		if err := processViewerExportJob(job, channelName, messages, cfg, extraOptions); err != nil {
			_ = markJobFailed(job, err)
			return err
		}
		return nil
	}

	var ctx *payloadContext
	if extraOptions != nil && len(extraOptions.DisplaySettings) > 0 {
		ctx = &payloadContext{DisplayOptions: extraOptions.DisplaySettings}
	}
	payload := buildExportPayload(job, channelName, messages, ctx, extraOptions)
	if extraOptions != nil && extraOptions.TextColorizeBBCode {
		if payload.ExtraMeta == nil {
			payload.ExtraMeta = make(map[string]interface{})
		}
		payload.ExtraMeta["text_colorize_bbcode"] = true
		if len(extraOptions.TextColorizeBBCodeMap) > 0 {
			payload.ExtraMeta["text_colorize_bbcode_map"] = cloneStringMap(extraOptions.TextColorizeBBCodeMap)
		}
		if len(extraOptions.TextColorizeBBCodeNameMap) > 0 {
			payload.ExtraMeta["text_colorize_bbcode_name_map"] = cloneStringMap(extraOptions.TextColorizeBBCodeNameMap)
		}
	}

	formatter, ok := getFormatter(job.Format)
	if !ok {
		err = fmt.Errorf("不支持的导出格式: %s", job.Format)
		_ = markJobFailed(job, err)
		return err
	}
	data, err := formatter.Build(payload)
	if err != nil {
		_ = markJobFailed(job, err)
		return err
	}

	if err := os.MkdirAll(cfg.StorageDir, 0755); err != nil {
		_ = markJobFailed(job, err)
		return err
	}

	fileName := BuildExportResultFileName(job.DisplayName, job.ID, formatter.Ext(), payload.GeneratedAt)
	filePath := filepath.Join(cfg.StorageDir, fmt.Sprintf("%s.%s", job.ID, formatter.Ext()))
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		_ = markJobFailed(job, err)
		return err
	}

	return markJobDone(job, filePath, fileName)
}

type batchExportEntry struct {
	name string
	path string
}

func processBatchExportJob(job *model.MessageExportJobModel, cfg MessageExportWorkerConfig, extra *exportExtraOptions) error {
	format, ok := normalizeExportFormat(extra.BatchFormat)
	if !ok {
		return fmt.Errorf("批量导出格式无效: %s", extra.BatchFormat)
	}
	if err := os.MkdirAll(cfg.StorageDir, 0755); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(cfg.StorageDir, "batch-export-*")
	if err != nil {
		return fmt.Errorf("创建批量导出临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)

	entries := make([]batchExportEntry, len(extra.BatchChannelIDs))
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	concurrency := NormalizeExportConcurrency(extra.MaxConcurrency)
	if strings.EqualFold(format, "html") && concurrency > 2 {
		concurrency = 2
	}
	sem := make(chan struct{}, concurrency)
	for index, channelID := range extra.BatchChannelIDs {
		wg.Add(1)
		go func(index int, channelID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			entry, err := buildBatchExportEntry(job, channelID, format, cfg, tempDir, index)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			entries[index] = entry
		}(index, channelID)
	}
	wg.Wait()
	close(errCh)
	if err, ok := <-errCh; ok {
		return err
	}

	fileName := BuildExportResultFileName(job.DisplayName, job.ID, "zip", time.Now())
	filePath := filepath.Join(cfg.StorageDir, fmt.Sprintf("%s.zip", job.ID))
	if err := writeBatchExportArchive(filePath, entries); err != nil {
		return err
	}
	return markJobDone(job, filePath, fileName)
}

func buildBatchExportEntry(parent *model.MessageExportJobModel, channelID, format string, cfg MessageExportWorkerConfig, tempDir string, index int) (batchExportEntry, error) {
	child := *parent
	child.ID = fmt.Sprintf("%s-%d", parent.ID, index+1)
	child.ChannelID = channelID
	child.Format = format
	childExtra := parseExportExtraOptions(parent.ExtraOptions)
	childExtra.BatchChannelIDs = nil
	childExtra.BatchFormat = ""
	encodedExtra, err := json.Marshal(childExtra)
	if err != nil {
		return batchExportEntry{}, err
	}
	child.ExtraOptions = string(encodedExtra)
	channelName := resolveChannelName(channelID)
	channelDir := filepath.Join(tempDir, fmt.Sprintf("%03d-%s", index+1, sanitizeFileName(channelName)))
	if err := os.MkdirAll(channelDir, 0755); err != nil {
		return batchExportEntry{}, err
	}
	childCfg := cfg
	childCfg.StorageDir = channelDir
	if err := processExportJob(&child, childCfg); err != nil {
		return batchExportEntry{}, fmt.Errorf("频道 %s 导出失败: %w", channelName, err)
	}
	files, err := os.ReadDir(channelDir)
	if err != nil {
		return batchExportEntry{}, err
	}
	if len(files) != 1 {
		return batchExportEntry{}, fmt.Errorf("频道 %s 导出文件异常", channelName)
	}
	path := filepath.Join(channelDir, files[0].Name())
	entryName := fmt.Sprintf("%03d-%s.%s", index+1, sanitizeFileName(channelName), format)
	if strings.EqualFold(format, "html") {
		entryName = fmt.Sprintf("%03d-%s", index+1, files[0].Name())
	}
	return batchExportEntry{
		name: entryName,
		path: path,
	}, nil
}

func writeBatchExportArchive(filePath string, entries []batchExportEntry) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建批量 ZIP 文件失败: %w", err)
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	defer writer.Close()
	for _, entry := range entries {
		input, err := os.Open(entry.path)
		if err != nil {
			return err
		}
		info, err := input.Stat()
		if err != nil {
			_ = input.Close()
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = input.Close()
			return err
		}
		header.Name = entry.name
		header.Method = zip.Deflate
		output, err := writer.CreateHeader(header)
		if err == nil {
			_, err = io.Copy(output, input)
		}
		closeErr := input.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func markJobFailed(job *model.MessageExportJobModel, cause error) error {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	updates := map[string]any{
		"status":      model.MessageExportStatusFailed,
		"error_msg":   message,
		"finished_at": time.Now(),
	}
	return model.GetDB().Model(&model.MessageExportJobModel{}).
		Where("id = ?", job.ID).
		Updates(updates).Error
}

func markJobDone(job *model.MessageExportJobModel, filePath, fileName string) error {
	fileSize := int64(0)
	if info, err := os.Stat(filePath); err == nil {
		fileSize = info.Size()
	}
	updates := map[string]any{
		"status":      model.MessageExportStatusDone,
		"file_path":   filePath,
		"file_name":   fileName,
		"file_size":   fileSize,
		"error_msg":   "",
		"finished_at": time.Now(),
	}
	return model.GetDB().Model(&model.MessageExportJobModel{}).
		Where("id = ?", job.ID).
		Updates(updates).Error
}

func sanitizeFileName(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	return filenameSafeRe.ReplaceAllString(input, "_")
}

func resolveChannelName(channelID string) string {
	if ch, err := model.ChannelGet(channelID); err == nil && ch != nil && strings.TrimSpace(ch.ID) != "" {
		if strings.TrimSpace(ch.Name) != "" {
			return ch.Name
		}
	}
	if fr, err := model.FriendRelationGetByID(channelID); err == nil && fr != nil && strings.TrimSpace(fr.ID) != "" {
		return fmt.Sprintf("私聊-%s-%s", fr.UserID1, fr.UserID2)
	}
	return channelID
}
