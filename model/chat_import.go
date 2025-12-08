package model

import "time"

const (
	ChatImportStatusPending    = "pending"
	ChatImportStatusRunning    = "running"
	ChatImportStatusDone       = "done"
	ChatImportStatusFailed     = "failed"
)

// ChatImportJobModel 记录聊天日志导入任务元数据与执行状态
type ChatImportJobModel struct {
	StringPKBaseModel
	ChannelID      string     `json:"channelId" gorm:"size:100;index"`
	WorldID        string     `json:"worldId" gorm:"size:100;index"`
	UserID         string     `json:"userId" gorm:"size:100"`         // 执行导入的用户
	Status         string     `json:"status" gorm:"size:24;default:pending;index"`
	TotalLines     int        `json:"totalLines"`                     // 总行数
	ProcessedLines int        `json:"processedLines"`                 // 已处理行数
	ImportedCount  int        `json:"importedCount"`                  // 已导入消息数
	SkippedCount   int        `json:"skippedCount"`                   // 跳过的行数
	ErrorMessage   string     `json:"errorMessage" gorm:"type:text"`
	ConfigJSON     string     `json:"configJson" gorm:"type:text"`    // 保存完整导入配置
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
}

func (*ChatImportJobModel) TableName() string {
	return "chat_import_jobs"
}

// ChatImportRoleMappingConfig 单个角色的映射配置
type ChatImportRoleMappingConfig struct {
	DisplayName        string `json:"displayName"`        // 显示名称
	Color              string `json:"color"`              // 颜色
	AvatarAttachmentID string `json:"avatarAttachmentId"` // 头像附件ID
	BindToUserID       string `json:"bindToUserId"`       // 关联到的用户ID
	ReuseIdentityID    string `json:"reuseIdentityId"`    // 复用已有身份ID
}

// ChatImportConfig 导入配置
type ChatImportConfig struct {
	Version        string                                 `json:"version"`
	RegexPattern   string                                 `json:"regexPattern"`   // 自定义正则
	TemplateID     string                                 `json:"templateId"`     // 内置模板ID
	BaseTime       *time.Time                             `json:"baseTime"`       // 基准时间
	TimeIncrement  int64                                  `json:"timeIncrement"`  // 时间增量 (毫秒)
	MergeUnmatched bool                                   `json:"mergeUnmatched"` // 是否合并不匹配行
	StrictOOC      bool                                   `json:"strictOoc"`      // 严格OOC模式（只看首字符）
	RoleMapping    map[string]*ChatImportRoleMappingConfig `json:"roleMapping"`    // 角色映射配置
}

// ChatImportPreviewRequest 预览请求
type ChatImportPreviewRequest struct {
	Content        string `json:"content"`
	RegexPattern   string `json:"regexPattern"`
	TemplateID     string `json:"templateId"`
	PreviewLimit   int    `json:"previewLimit"`
	MergeUnmatched bool   `json:"mergeUnmatched"`
}

// ChatImportExecuteRequest 执行导入请求
type ChatImportExecuteRequest struct {
	Content string            `json:"content"`
	Config  *ChatImportConfig `json:"config"`
}

// ChatImportPreviewResponse 预览响应
type ChatImportPreviewResponse struct {
	Entries          []*ParsedLogEntry `json:"entries"`
	TotalLines       int               `json:"totalLines"`
	ParsedCount      int               `json:"parsedCount"`
	SkippedCount     int               `json:"skippedCount"`
	DetectedRoles    []string          `json:"detectedRoles"`
	UsedPattern      string            `json:"usedPattern"`
	UsedTemplateName string            `json:"usedTemplateName"`
}

// ParsedLogEntry 解析后的日志条目
type ParsedLogEntry struct {
	RawLine    string     `json:"rawLine"`
	Timestamp  *time.Time `json:"timestamp,omitempty"`
	RoleName   string     `json:"roleName"`
	Content    string     `json:"content"`
	IsOOC      bool       `json:"isOoc"`
	LineNumber int        `json:"lineNumber"`
}

// ChatImportTemplate 内置正则模板
type ChatImportTemplate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	Example     string `json:"example"`
}
