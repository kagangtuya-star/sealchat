package model

import "time"

const (
	TheaterPackageJobTypeExport        = "export"
	TheaterPackageJobTypeImport        = "import"
	TheaterPackageJobTypeImportCCFOLIA = "import_ccfolia"

	TheaterPackageJobStatusPending = "pending"
	TheaterPackageJobStatusRunning = "running"
	TheaterPackageJobStatusDone    = "done"
	TheaterPackageJobStatusFailed  = "failed"
)

// TheaterPackageJobModel records durable full-stage export and import work.
type TheaterPackageJobModel struct {
	StringPKBaseModel
	Type           string     `json:"type" gorm:"size:16;not null;index:idx_theater_package_job_status_created,priority:2"`
	Status         string     `json:"status" gorm:"size:24;not null;index:idx_theater_package_job_status_created,priority:1"`
	ActorUserID    string     `json:"actorUserId" gorm:"size:100;not null;index"`
	SourceWorldID  string     `json:"sourceWorldId,omitempty" gorm:"size:100;index"`
	TargetWorldID  string     `json:"targetWorldId,omitempty" gorm:"size:100;index"`
	InputChannelID string     `json:"inputChannelId,omitempty" gorm:"size:100;index"`
	Progress       float64    `json:"progress" gorm:"not null;default:0"`
	InputFilePath  string     `json:"-" gorm:"size:1024"`
	OriginalName   string     `json:"originalName,omitempty" gorm:"size:255"`
	OutputFilePath string     `json:"-" gorm:"size:1024"`
	OutputFileName string     `json:"outputFileName,omitempty" gorm:"size:255"`
	OutputFileSize int64      `json:"outputFileSize,omitempty"`
	PackageHash    string     `json:"packageHash,omitempty" gorm:"size:64;index"`
	SummaryJSON    string     `json:"summaryJson,omitempty" gorm:"type:text"`
	ErrorCode      string     `json:"errorCode,omitempty" gorm:"size:64"`
	ErrorMessage   string     `json:"errorMessage,omitempty" gorm:"type:text"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty" gorm:"index"`
}

func (*TheaterPackageJobModel) TableName() string { return "theater_package_jobs" }
