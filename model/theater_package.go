package model

import "time"

const (
	TheaterPackageJobTypeExport        = "export"
	TheaterPackageJobTypeExportEffects = "export_effects"
	TheaterPackageJobTypeImport        = "import"
	TheaterPackageJobTypeImportCCFOLIA = "import_ccfolia"

	TheaterPackageJobStatusPending = "pending"
	TheaterPackageJobStatusRunning = "running"
	TheaterPackageJobStatusDone    = "done"
	TheaterPackageJobStatusFailed  = "failed"
)

// TheaterPackageJobModel records durable theater and effect package work.
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

const (
	TheaterSourceArchiveStatusActive   = "active"
	TheaterSourceArchiveStatusDeleting = "deleting"
)

type TheaterSourceArchiveModel struct {
	StringPKBaseModel
	WorldID      string     `json:"worldId" gorm:"size:100;not null;index"`
	RoomID       string     `json:"roomId" gorm:"size:100;not null;uniqueIndex:udx_theater_source_archive_room_hash,priority:1"`
	SourceFormat string     `json:"sourceFormat" gorm:"size:32;not null"`
	SHA256       string     `json:"sha256" gorm:"size:64;not null;uniqueIndex:udx_theater_source_archive_room_hash,priority:2;index"`
	SizeBytes    int64      `json:"sizeBytes" gorm:"not null"`
	Truncated    bool       `json:"truncated" gorm:"not null;default:false"`
	Status       string     `json:"status" gorm:"size:16;not null;index"`
	CleanupAfter *time.Time `json:"cleanupAfter,omitempty" gorm:"index"`
}

func (*TheaterSourceArchiveModel) TableName() string { return "theater_source_archives" }
