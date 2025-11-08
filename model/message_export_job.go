package model

import "time"

const (
	MessageExportStatusPending    = "pending"
	MessageExportStatusProcessing = "processing"
	MessageExportStatusDone       = "done"
	MessageExportStatusFailed     = "failed"
)

// MessageExportJobModel 记录导出任务元数据与执行状态。
type MessageExportJobModel struct {
	StringPKBaseModel

	UserID    string `json:"user_id" gorm:"index;size:100"`
	ChannelID string `json:"channel_id" gorm:"index;size:100"`
	Format    string `json:"format" gorm:"size:32"`

	IncludeOOC       bool   `json:"include_ooc"`
	IncludeArchived  bool   `json:"include_archived"`
	WithoutTimestamp bool   `json:"without_timestamp"`
	MergeMessages    bool   `json:"merge_messages"`
	ExtraOptions     string `json:"extra_options" gorm:"type:text"`

	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`

	FilePath string `json:"file_path"`
	FileName string `json:"file_name"`
	UploadURL string `json:"upload_url"`
	UploadMeta string `json:"upload_meta" gorm:"type:text"`
	UploadedAt *time.Time `json:"uploaded_at"`

	Status     string     `json:"status" gorm:"index;size:24"`
	ErrorMsg   string     `json:"error_msg" gorm:"type:text"`
	FinishedAt *time.Time `json:"finished_at"`
}

func (*MessageExportJobModel) TableName() string {
	return "message_export_jobs"
}
