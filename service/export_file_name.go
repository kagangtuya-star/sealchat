package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"sealchat/model"
)

const defaultExportFileBaseName = "频道记录"

func BuildExportResultFileName(displayName, taskID, ext string, ts time.Time) string {
	baseName := resolveExportFileBaseName(displayName)
	taskPart := sanitizeFileName(strings.TrimSpace(taskID))
	if taskPart == "" {
		taskPart = "unknown-task"
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	resolvedExt := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if resolvedExt == "" {
		resolvedExt = "txt"
	}
	return fmt.Sprintf("%s-%s-%s.%s", baseName, taskPart, ts.Format("20060102-150405"), resolvedExt)
}

func ResolveExportDownloadFileName(job *model.MessageExportJobModel) string {
	if job == nil {
		return BuildExportResultFileName("", "", "txt", time.Now())
	}
	return BuildExportResultFileName(job.DisplayName, job.ID, job.Format, resolveExportFileTimestamp(job))
}

func resolveExportFileBaseName(displayName string) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		return defaultExportFileBaseName
	}
	if ext := filepath.Ext(name); ext != "" && ext != "." {
		name = strings.TrimSpace(strings.TrimSuffix(name, ext))
	}
	name = sanitizeFileName(name)
	if name == "" {
		return defaultExportFileBaseName
	}
	return name
}

func resolveExportFileTimestamp(job *model.MessageExportJobModel) time.Time {
	if job == nil {
		return time.Now()
	}
	if job.FinishedAt != nil && !job.FinishedAt.IsZero() {
		return job.FinishedAt.Local()
	}
	if !job.CreatedAt.IsZero() {
		return job.CreatedAt.Local()
	}
	return time.Now()
}
