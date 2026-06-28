package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const (
	AudioImportJobStatusPending = "pending"
	AudioImportJobStatusRunning = "running"
	AudioImportJobStatusDone    = "done"
	AudioImportJobStatusFailed  = "failed"
)

type AudioAssetVisibility string

const (
	AudioVisibilityPublic     AudioAssetVisibility = "public"
	AudioVisibilityRestricted AudioAssetVisibility = "restricted"
)

type AudioTranscodeStatus string

const (
	AudioTranscodePending AudioTranscodeStatus = "pending"
	AudioTranscodeReady   AudioTranscodeStatus = "ready"
	AudioTranscodeFailed  AudioTranscodeStatus = "failed"
)

type AudioAssetScope string

const (
	AudioScopeCommon AudioAssetScope = "common"
	AudioScopeWorld  AudioAssetScope = "world"
)

type AudioAssetVariant struct {
	Label       string            `json:"label"`
	BitrateKbps int               `json:"bitrateKbps"`
	StorageType StorageType       `json:"storageType"`
	ObjectKey   string            `json:"objectKey"`
	Size        int64             `json:"size"`
	Duration    float64           `json:"duration"`
	Extra       map[string]string `json:"extra,omitempty"`
}

type JSONList[T any] []T

func (jl JSONList[T]) MarshalJSON() ([]byte, error) {
	if jl == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]T(jl))
}

func (jl JSONList[T]) Value() (driver.Value, error) {
	if jl == nil {
		return []byte("[]"), nil
	}
	data, err := json.Marshal([]T(jl))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (jl *JSONList[T]) Scan(value interface{}) error {
	if value == nil {
		*jl = JSONList[T]{}
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported type %T for JSONList", value)
	}
	if len(data) == 0 {
		*jl = JSONList[T]{}
		return nil
	}
	var out []T
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	if out == nil {
		out = []T{}
	}
	*jl = out
	return nil
}

type AudioAsset struct {
	StringPKBaseModel
	Name            string                      `json:"name"`
	FolderID        *string                     `json:"folderId" gorm:"index"`
	Size            int64                       `json:"size"`
	DurationSeconds float64                     `json:"duration" gorm:"column:duration"`
	BitrateKbps     int                         `json:"bitrate"`
	StorageType     StorageType                 `json:"storageType" gorm:"type:varchar(16)"`
	ObjectKey       string                      `json:"objectKey"`
	Description     string                      `json:"description"`
	Tags            JSONList[string]            `json:"tags" gorm:"type:json"`
	Visibility      AudioAssetVisibility        `json:"visibility" gorm:"type:varchar(16)"`
	CreatedBy       string                      `json:"createdBy" gorm:"index"`
	UpdatedBy       string                      `json:"updatedBy"`
	LastAccessedAt  *time.Time                  `json:"lastAccessedAt"`
	AccessCount     int64                       `json:"accessCount" gorm:"not null;default:0"`
	SortOrder       int                         `json:"sortOrder" gorm:"default:0;index"`
	ManualSorted    bool                        `json:"manualSorted" gorm:"not null;default:false;index"`
	Variants        JSONList[AudioAssetVariant] `json:"variants" gorm:"type:json"`
	TranscodeStatus AudioTranscodeStatus        `json:"transcodeStatus" gorm:"type:varchar(16);default:'ready'"`
	Scope           AudioAssetScope             `json:"scope" gorm:"type:varchar(16);index;default:'common'"`
	WorldID         *string                     `json:"worldId" gorm:"index"`
}

func (*AudioAsset) TableName() string { return "audio_assets" }

type AudioFolder struct {
	StringPKBaseModel
	ParentID  *string         `json:"parentId" gorm:"index"`
	Name      string          `json:"name"`
	Path      string          `json:"path" gorm:"index:idx_audio_folder_scope_path,unique"`
	CreatedBy string          `json:"createdBy"`
	UpdatedBy string          `json:"updatedBy"`
	Scope     AudioAssetScope `json:"scope" gorm:"type:varchar(16);index:idx_audio_folder_scope_path,unique;default:'common'"`
	WorldID   *string         `json:"worldId" gorm:"index:idx_audio_folder_scope_path,unique"`
}

func (*AudioFolder) TableName() string { return "audio_folders" }

type AudioImportJobModel struct {
	StringPKBaseModel
	Status         string  `json:"status" gorm:"size:24;default:pending;index"`
	CreatedBy      string  `json:"createdBy" gorm:"index"`
	Scope          string  `json:"scope" gorm:"size:24;index"`
	WorldID        *string `json:"worldId" gorm:"index"`
	FolderID       *string `json:"folderId" gorm:"index"`
	DirectoryPath  string  `json:"directoryPath" gorm:"size:512"`
	TotalFiles     int     `json:"totalFiles"`
	ProcessedFiles int     `json:"processedFiles"`
	ImportedCount  int     `json:"importedCount"`
	SkippedCount   int     `json:"skippedCount"`
	FailedCount    int     `json:"failedCount"`
	ErrorMessage   string  `json:"errorMessage" gorm:"type:text"`
	ImportedJSON   string  `json:"importedJson" gorm:"type:text"`
	SkippedJSON    string  `json:"skippedJson" gorm:"type:text"`
	FailedJSON     string  `json:"failedJson" gorm:"type:text"`
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
}

func (*AudioImportJobModel) TableName() string { return "audio_import_jobs" }

type AudioSceneTrack struct {
	Type             string   `json:"type"`
	AssetID          *string  `json:"assetId"`
	Volume           float64  `json:"volume"`
	FadeIn           int      `json:"fadeIn"`
	FadeOut          int      `json:"fadeOut"`
	LoopEnabled      *bool    `json:"loopEnabled,omitempty"`
	PlaybackRate     *float64 `json:"playbackRate,omitempty"`
	PlaylistFolderID *string  `json:"playlistFolderId,omitempty"`
	PlaylistMode     *string  `json:"playlistMode,omitempty"`
	PlaylistAssetIDs []string `json:"playlistAssetIds,omitempty"`
	PlaylistIndex    int      `json:"playlistIndex"`
}

type AudioScene struct {
	StringPKBaseModel
	Name         string                    `json:"name"`
	Description  string                    `json:"description"`
	Tracks       JSONList[AudioSceneTrack] `json:"tracks" gorm:"type:json"`
	Tags         JSONList[string]          `json:"tags" gorm:"type:json"`
	Order        int                       `json:"order" gorm:"index"`
	ChannelScope *string                   `json:"channelScope" gorm:"index"`
	CreatedBy    string                    `json:"createdBy"`
	UpdatedBy    string                    `json:"updatedBy"`
	Scope        AudioAssetScope           `json:"scope" gorm:"type:varchar(16);index;default:'common'"`
	WorldID      *string                   `json:"worldId" gorm:"index"`
}

func (*AudioScene) TableName() string { return "audio_scenes" }

type AudioTrackState struct {
	Type             string   `json:"type"`
	AssetID          *string  `json:"assetId"`
	Volume           float64  `json:"volume"`
	Muted            bool     `json:"muted"`
	Solo             bool     `json:"solo"`
	FadeIn           int      `json:"fadeIn"`
	FadeOut          int      `json:"fadeOut"`
	IsPlaying        bool     `json:"isPlaying"`
	Position         float64  `json:"position"`
	LoopEnabled      bool     `json:"loopEnabled"`
	PlaybackRate     float64  `json:"playbackRate"`
	PlaylistFolderID *string  `json:"playlistFolderId,omitempty"`
	PlaylistMode     *string  `json:"playlistMode,omitempty"`
	PlaylistAssetIDs []string `json:"playlistAssetIds,omitempty"`
	PlaylistIndex    int      `json:"playlistIndex"`
}

type AudioPlaybackState struct {
	ChannelID            string                    `json:"channelId" gorm:"primaryKey"`
	SceneID              *string                   `json:"sceneId"`
	Tracks               JSONList[AudioTrackState] `json:"tracks" gorm:"type:json"`
	IsPlaying            bool                      `json:"isPlaying"`
	Position             float64                   `json:"position"`
	LoopEnabled          bool                      `json:"loopEnabled"`
	PlaybackRate         float64                   `json:"playbackRate"`
	WorldPlaybackEnabled bool                      `json:"worldPlaybackEnabled" gorm:"default:true"`
	Revision             int64                     `json:"revision" gorm:"not null;default:0"`
	CapturedAtMs         int64                     `json:"capturedAtMs" gorm:"not null;default:0"`
	UpdatedBy            string                    `json:"updatedBy"`
	UpdatedAt            time.Time                 `json:"updatedAt"`
	CreatedAt            time.Time                 `json:"createdAt"`
}

func (*AudioPlaybackState) TableName() string { return "audio_playback_states" }
