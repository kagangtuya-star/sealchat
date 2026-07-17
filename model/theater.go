package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const TheaterSchemaVersion = 1

type TheaterRoomModel struct {
	StringPKBaseModel
	WorldID       string `json:"worldId" gorm:"size:100;not null;uniqueIndex:udx_theater_room_world_channel,priority:1"`
	ChannelID     string `json:"channelId" gorm:"size:100;not null;uniqueIndex:udx_theater_room_world_channel,priority:2"`
	Revision      int64  `json:"revision" gorm:"not null;default:0"`
	SchemaVersion int    `json:"schemaVersion" gorm:"not null;default:1"`
	ActiveSceneID string `json:"activeSceneId,omitempty" gorm:"size:100;index"`
	Status        string `json:"status" gorm:"size:16;not null;default:active;index"`
	StateHash     string `json:"stateHash" gorm:"size:64"`
	StateJSON     string `json:"stateJson" gorm:"not null"`
	CreatedBy     string `json:"createdBy" gorm:"size:100;index"`
	UpdatedBy     string `json:"updatedBy" gorm:"size:100"`
}

func (*TheaterRoomModel) TableName() string { return "theater_rooms" }

type TheaterSnapshotModel struct {
	StringPKBaseModel
	RoomID        string `json:"roomId" gorm:"size:100;not null;index:idx_theater_snapshot_room_revision,priority:1"`
	Revision      int64  `json:"revision" gorm:"not null;index:idx_theater_snapshot_room_revision,priority:2"`
	SchemaVersion int    `json:"schemaVersion" gorm:"not null"`
	SnapshotJSON  string `json:"snapshotJson" gorm:"not null"`
	SnapshotHash  string `json:"snapshotHash" gorm:"size:64;not null"`
	SnapshotBytes int64  `json:"snapshotBytes" gorm:"not null"`
	Kind          string `json:"kind" gorm:"size:32;not null;index"`
	Reason        string `json:"reason" gorm:"size:1024"`
	CreatedBy     string `json:"createdBy" gorm:"size:100;index"`
}

func (*TheaterSnapshotModel) TableName() string { return "theater_snapshots" }

type TheaterSceneModel struct {
	StringPKBaseModel
	RoomID        string `json:"roomId" gorm:"size:100;not null;index:idx_theater_scene_room_sort,priority:1"`
	Name          string `json:"name" gorm:"size:512;not null"`
	SortOrder     int64  `json:"sortOrder" gorm:"not null;index:idx_theater_scene_room_sort,priority:2"`
	Locked        bool   `json:"locked" gorm:"not null;default:false"`
	StateJSON     string `json:"stateJson" gorm:"not null"`
	SchemaVersion int    `json:"schemaVersion" gorm:"not null"`
	CreatedBy     string `json:"createdBy" gorm:"size:100;index"`
	UpdatedBy     string `json:"updatedBy" gorm:"size:100"`
}

func (*TheaterSceneModel) TableName() string { return "theater_scenes" }

type TheaterObjectModel struct {
	StringPKBaseModel
	RoomID              string  `json:"roomId" gorm:"size:100;not null;index:idx_theater_object_scope_order,priority:1"`
	SceneID             string  `json:"sceneId,omitempty" gorm:"size:100;index:idx_theater_object_scope_order,priority:2"`
	ParentID            string  `json:"parentId,omitempty" gorm:"size:100;index"`
	Kind                string  `json:"kind" gorm:"size:32;not null;index"`
	Name                string  `json:"name" gorm:"size:512"`
	X                   float64 `json:"x" gorm:"not null"`
	Y                   float64 `json:"y" gorm:"not null"`
	Width               float64 `json:"width" gorm:"not null"`
	Height              float64 `json:"height" gorm:"not null"`
	Rotation            float64 `json:"rotation" gorm:"not null"`
	Scale               float64 `json:"scale" gorm:"not null;default:1"`
	ScaleX              float64 `json:"scaleX" gorm:"not null;default:1"`
	ScaleY              float64 `json:"scaleY" gorm:"not null;default:1"`
	Z                   float64 `json:"z" gorm:"not null"`
	OrderKey            string  `json:"orderKey" gorm:"size:128;not null;index:idx_theater_object_scope_order,priority:3"`
	Visible             bool    `json:"visible" gorm:"not null;default:true"`
	Locked              bool    `json:"locked" gorm:"not null;default:false"`
	AspectRatioLocked   bool    `json:"aspectRatioLocked" gorm:"not null;default:true"`
	Interactive         bool    `json:"interactive" gorm:"not null;default:false"`
	Editable            bool    `json:"editable" gorm:"not null;default:false"`
	OwnerUserID         string  `json:"ownerUserId,omitempty" gorm:"size:100;index"`
	CharacterIdentityID string  `json:"characterIdentityId,omitempty" gorm:"size:100;index"`
	ContentJSON         string  `json:"contentJson" gorm:"not null"`
	ActionsJSON         string  `json:"actionsJson" gorm:"not null"`
	MetadataJSON        string  `json:"metadataJson" gorm:"not null"`
	SchemaVersion       int     `json:"schemaVersion" gorm:"not null"`
	CreatedBy           string  `json:"createdBy" gorm:"size:100;index"`
	UpdatedBy           string  `json:"updatedBy" gorm:"size:100"`
}

func (*TheaterObjectModel) TableName() string { return "theater_objects" }

type TheaterResourceModel struct {
	StringPKBaseModel
	RoomID             string     `json:"roomId" gorm:"size:100;not null;index:idx_theater_resource_hash,priority:1"`
	ClientResourceID   string     `json:"clientResourceId,omitempty" gorm:"size:128;index"`
	AttachmentID       string     `json:"attachmentId" gorm:"size:100;not null;index"`
	Kind               string     `json:"kind" gorm:"size:32;not null"`
	ContentHash        string     `json:"contentHash" gorm:"size:64;index:idx_theater_resource_hash,priority:2"`
	SizeBytes          int64      `json:"sizeBytes" gorm:"not null;index:idx_theater_resource_hash,priority:3"`
	MimeType           string     `json:"mimeType" gorm:"size:128;not null"`
	OriginalFilename   string     `json:"originalFilename" gorm:"size:255"`
	Width              *int       `json:"width,omitempty"`
	Height             *int       `json:"height,omitempty"`
	DurationMS         *int64     `json:"durationMs,omitempty"`
	FrameCount         *int       `json:"frameCount,omitempty"`
	FrameRate          *float64   `json:"frameRate,omitempty"`
	Container          string     `json:"container,omitempty" gorm:"size:64"`
	VideoCodec         string     `json:"videoCodec,omitempty" gorm:"size:64"`
	AudioCodec         string     `json:"audioCodec,omitempty" gorm:"size:64"`
	Status             string     `json:"status" gorm:"size:16;not null;index"`
	ProcessingProgress float64    `json:"processingProgress" gorm:"not null;default:0"`
	ProcessingJobID    string     `json:"processingJobId,omitempty" gorm:"size:100;index"`
	PosterResourceID   string     `json:"posterResourceId,omitempty" gorm:"size:100;index"`
	VariantsJSON       string     `json:"variantsJson" gorm:"not null"`
	FailureCode        string     `json:"failureCode,omitempty" gorm:"size:64"`
	FailureMessage     string     `json:"failureMessage,omitempty" gorm:"size:2048"`
	Retryable          bool       `json:"retryable" gorm:"not null;default:false"`
	ReferenceCount     int64      `json:"referenceCount" gorm:"not null;default:0;index"`
	CreatedBy          string     `json:"createdBy" gorm:"size:100;index"`
	ReadyAt            *time.Time `json:"readyAt,omitempty"`
}

func (*TheaterResourceModel) TableName() string { return "theater_resources" }

type TheaterResourceVariantModel struct {
	StringPKBaseModel
	ResourceID   string `json:"resourceId" gorm:"size:100;not null;uniqueIndex:udx_theater_resource_variant,priority:1"`
	Name         string `json:"name" gorm:"size:64;not null;uniqueIndex:udx_theater_resource_variant,priority:2"`
	AttachmentID string `json:"attachmentId" gorm:"size:100;not null;index"`
	MimeType     string `json:"mimeType" gorm:"size:128;not null"`
	SizeBytes    int64  `json:"sizeBytes" gorm:"not null"`
	Width        *int   `json:"width,omitempty"`
	Height       *int   `json:"height,omitempty"`
	DurationMS   *int64 `json:"durationMs,omitempty"`
	Status       string `json:"status" gorm:"size:16;not null;index"`
	ContentHash  string `json:"contentHash" gorm:"size:64"`
}

func (*TheaterResourceVariantModel) TableName() string { return "theater_resource_variants" }

type TheaterResourceJobModel struct {
	StringPKBaseModel
	ResourceID string     `json:"resourceId" gorm:"size:100;not null;uniqueIndex:udx_theater_resource_job,priority:1;index:idx_theater_resource_job_status_created,priority:3"`
	RequestID  string     `json:"requestId" gorm:"size:128;not null;uniqueIndex:udx_theater_resource_job,priority:2"`
	Type       string     `json:"type" gorm:"size:32;not null"`
	Status     string     `json:"status" gorm:"size:16;not null;index:idx_theater_resource_job_status_created,priority:1"`
	Attempt    int        `json:"attempt" gorm:"not null;default:0"`
	Progress   float64    `json:"progress" gorm:"not null;default:0"`
	ErrorCode  string     `json:"errorCode,omitempty" gorm:"size:64"`
	Error      string     `json:"error,omitempty" gorm:"size:2048"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

func (*TheaterResourceJobModel) TableName() string { return "theater_resource_jobs" }

type TheaterMutationModel struct {
	StringPKBaseModel
	RoomID             string     `json:"roomId" gorm:"size:100;not null;uniqueIndex:udx_theater_mutation_room_id,priority:1;index:idx_theater_mutation_revision,priority:1"`
	WorldID            string     `json:"worldId" gorm:"size:100;not null;index"`
	ChannelID          string     `json:"channelId" gorm:"size:100;not null;index"`
	MutationID         string     `json:"mutationId" gorm:"size:128;not null;uniqueIndex:udx_theater_mutation_room_id,priority:2"`
	ActorUserID        string     `json:"actorUserId" gorm:"size:100;not null;index"`
	ExpectedRevision   int64      `json:"expectedRevision" gorm:"not null"`
	RevisionBefore     int64      `json:"revisionBefore" gorm:"not null"`
	RevisionAfter      *int64     `json:"revisionAfter,omitempty" gorm:"index:idx_theater_mutation_revision,priority:2"`
	Type               string     `json:"type" gorm:"size:64;not null;index"`
	PayloadJSON        string     `json:"payloadJson" gorm:"not null"`
	PayloadHash        string     `json:"payloadHash" gorm:"size:64;not null"`
	ResultJSON         string     `json:"resultJson" gorm:"not null"`
	Status             string     `json:"status" gorm:"size:16;not null;index"`
	RejectCode         string     `json:"rejectCode,omitempty" gorm:"size:64"`
	RejectReason       string     `json:"rejectReason,omitempty" gorm:"size:1024"`
	RequestSource      string     `json:"requestSource" gorm:"size:16"`
	RequestID          string     `json:"requestId,omitempty" gorm:"size:128;index"`
	SessionID          string     `json:"sessionId,omitempty" gorm:"size:128"`
	BroadcastedAt      *time.Time `json:"broadcastedAt,omitempty" gorm:"index"`
	BroadcastAttempts  int        `json:"broadcastAttempts" gorm:"not null;default:0"`
	LastBroadcastError string     `json:"lastBroadcastError,omitempty" gorm:"size:2048"`
}

func (*TheaterMutationModel) TableName() string { return "theater_mutations" }

type TheaterAuditLogModel struct {
	StringPKBaseModel
	RoomID            string `json:"roomId,omitempty" gorm:"size:100;index"`
	WorldID           string `json:"worldId" gorm:"size:100;not null;index:idx_theater_audit_scope_created,priority:1"`
	ChannelID         string `json:"channelId" gorm:"size:100;not null;index:idx_theater_audit_scope_created,priority:2"`
	ActorUserID       string `json:"actorUserId" gorm:"size:100;index"`
	ActorNameSnapshot string `json:"actorNameSnapshot" gorm:"size:255"`
	MutationID        string `json:"mutationId,omitempty" gorm:"size:128;index"`
	RevisionBefore    int64  `json:"revisionBefore"`
	RevisionAfter     *int64 `json:"revisionAfter,omitempty"`
	MutationType      string `json:"mutationType" gorm:"size:64;index"`
	Outcome           string `json:"outcome" gorm:"size:32;not null;index"`
	ReasonCode        string `json:"reasonCode,omitempty" gorm:"size:64"`
	ReasonMessage     string `json:"reasonMessage,omitempty" gorm:"size:1024"`
	RequestSource     string `json:"requestSource" gorm:"size:16"`
	RequestID         string `json:"requestId,omitempty" gorm:"size:128"`
	SessionID         string `json:"sessionId,omitempty" gorm:"size:128"`
	RemoteIPHash      string `json:"remoteIpHash,omitempty" gorm:"size:64"`
	UserAgentHash     string `json:"userAgentHash,omitempty" gorm:"size:64"`
	SummaryJSON       string `json:"summaryJson" gorm:"not null"`
}

func (*TheaterAuditLogModel) TableName() string { return "theater_audit_logs" }

func theaterModels() []any {
	return []any{
		&TheaterRoomModel{},
		&TheaterSnapshotModel{},
		&TheaterSceneModel{},
		&TheaterObjectModel{},
		&TheaterResourceModel{},
		&TheaterResourceVariantModel{},
		&TheaterResourceJobModel{},
		&TheaterAppearanceAssetModel{},
		&TheaterMutationModel{},
		&TheaterAuditLogModel{},
	}
}

func autoMigrateTheaterModels(conn *gorm.DB) error {
	if err := conn.AutoMigrate(theaterModels()...); err != nil {
		return err
	}
	return conn.Exec("UPDATE theater_objects SET scale_x = scale, scale_y = scale WHERE scale <> 1 AND scale_x = 1 AND scale_y = 1").Error
}

func TheaterRoomFindByScope(worldID, channelID string) (*TheaterRoomModel, error) {
	var room TheaterRoomModel
	err := GetDB().Where("world_id = ? AND channel_id = ?", worldID, channelID).First(&room).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &room, err
}

func TheaterRoomCreateIfMissing(worldID, channelID, actorID string) (*TheaterRoomModel, error) {
	room, err := TheaterRoomFindByScope(worldID, channelID)
	if err != nil || room != nil {
		return room, err
	}
	room = &TheaterRoomModel{
		WorldID:       worldID,
		ChannelID:     channelID,
		SchemaVersion: TheaterSchemaVersion,
		Status:        "active",
		StateJSON:     "{}",
		CreatedBy:     actorID,
		UpdatedBy:     actorID,
	}
	if err := GetDB().Clauses(clause.OnConflict{DoNothing: true}).Create(room).Error; err != nil {
		return nil, err
	}
	return TheaterRoomFindByScope(worldID, channelID)
}

func TheaterMutationFindByID(roomID, mutationID string) (*TheaterMutationModel, error) {
	var mutation TheaterMutationModel
	err := GetDB().Where("room_id = ? AND mutation_id = ?", roomID, mutationID).First(&mutation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &mutation, err
}

func TheaterMutationListAfterRevision(roomID string, afterRevision int64, limit int) ([]TheaterMutationModel, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	var items []TheaterMutationModel
	err := GetDB().Where("room_id = ? AND status = ? AND revision_after > ?", roomID, "applied", afterRevision).
		Order("revision_after ASC").Limit(limit).Find(&items).Error
	return items, err
}

func TheaterSnapshotGet(roomID, snapshotID string) (*TheaterSnapshotModel, error) {
	var snapshot TheaterSnapshotModel
	err := GetDB().Where("room_id = ? AND id = ?", roomID, snapshotID).First(&snapshot).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &snapshot, err
}

func TheaterResourceGet(roomID, resourceID string) (*TheaterResourceModel, error) {
	var resource TheaterResourceModel
	err := GetDB().Where("room_id = ? AND id = ?", roomID, resourceID).First(&resource).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &resource, err
}

func TheaterResourceVariantsList(resourceID string) ([]TheaterResourceVariantModel, error) {
	var variants []TheaterResourceVariantModel
	err := GetDB().Where("resource_id = ?", resourceID).Order("name ASC").Find(&variants).Error
	return variants, err
}
