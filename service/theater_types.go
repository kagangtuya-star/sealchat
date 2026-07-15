package service

import (
	"encoding/json"
	"time"
)

const (
	TheaterMutationSceneCreate       = "scene.create"
	TheaterMutationSceneUpdate       = "scene.update"
	TheaterMutationSceneDelete       = "scene.delete"
	TheaterMutationSceneApply        = "scene.apply"
	TheaterMutationObjectCreate      = "object.create"
	TheaterMutationObjectUpdate      = "object.update"
	TheaterMutationObjectBatchUpdate = "object.batchUpdate"
	TheaterMutationObjectDelete      = "object.delete"
	TheaterMutationObjectToggle      = "object.toggle"
	TheaterMutationCharacterBind     = "character.bind"
	TheaterMutationCharacterUpdate   = "character.update"
	TheaterMutationResourceAttach    = "resource.attach"
	TheaterMutationResourceDetach    = "resource.detach"
	TheaterMutationAdminRestore      = "admin.snapshot.restore"
	TheaterMutationAdminReplace      = "admin.snapshot.replace"
)

const (
	TheaterPermissionView                = "stage.view"
	TheaterPermissionSceneSwitch         = "stage.scene.switch"
	TheaterPermissionObjectEdit          = "stage.object.edit"
	TheaterPermissionObjectEditDelegated = "stage.object.edit.delegated"
	TheaterPermissionCharacterEdit       = "stage.character.edit"
	TheaterPermissionResourceUpload      = "stage.resource.upload"
	TheaterPermissionResourceDelete      = "stage.resource.delete"
	TheaterPermissionActionTrigger       = "stage.action.trigger"
	TheaterPermissionAdminRestore        = "stage.admin.restore"
)

type TheaterRequestMeta struct {
	Source    string
	RequestID string
	SessionID string
	RemoteIP  string
	UserAgent string
	ActorName string
}

type TheaterMutationCommand struct {
	MutationID       string          `json:"mutationId"`
	WorldID          string          `json:"worldId"`
	ChannelID        string          `json:"channelId"`
	ExpectedRevision int64           `json:"expectedRevision"`
	Type             string          `json:"type"`
	Payload          json.RawMessage `json:"payload"`
}

type TheaterMutationResult struct {
	MutationID     string          `json:"mutationId"`
	RevisionBefore int64           `json:"revisionBefore"`
	Revision       int64           `json:"revision"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	Checksum       string          `json:"checksum"`
	Idempotent     bool            `json:"idempotent,omitempty"`
}

type TheaterSnapshotOptions struct {
	IfRevision       *int64
	IncludeResources bool
}

type TheaterSceneSnapshot struct {
	ID      string                           `json:"id"`
	Name    string                           `json:"name"`
	Order   int64                            `json:"order"`
	Locked  bool                             `json:"locked"`
	State   json.RawMessage                  `json:"state"`
	Objects map[string]TheaterObjectSnapshot `json:"objects"`
}

type TheaterObjectSnapshot struct {
	ID                  string          `json:"id"`
	SceneID             *string         `json:"sceneId"`
	ParentID            *string         `json:"parentId"`
	Kind                string          `json:"kind"`
	Name                string          `json:"name"`
	X                   float64         `json:"x"`
	Y                   float64         `json:"y"`
	Width               float64         `json:"width"`
	Height              float64         `json:"height"`
	Rotation            float64         `json:"rotation"`
	Z                   float64         `json:"z"`
	OrderKey            string          `json:"orderKey"`
	Visible             bool            `json:"visible"`
	Locked              bool            `json:"locked"`
	SizeLocked          bool            `json:"sizeLocked"`
	Interactive         bool            `json:"interactive"`
	Editable            bool            `json:"editable"`
	OwnerUserID         *string         `json:"ownerUserId"`
	CharacterIdentityID *string         `json:"characterIdentityId"`
	Content             json.RawMessage `json:"content"`
	Actions             json.RawMessage `json:"actions"`
	Metadata            json.RawMessage `json:"metadata"`
}

type TheaterSharedSnapshot struct {
	ActiveSceneID     *string                          `json:"activeSceneId"`
	LiveState         json.RawMessage                  `json:"liveState"`
	Scenes            map[string]TheaterSceneSnapshot  `json:"scenes"`
	PersistentObjects map[string]TheaterObjectSnapshot `json:"persistentObjects"`
	Characters        map[string]TheaterObjectSnapshot `json:"characters"`
	Resources         map[string]TheaterResourcePublic `json:"resources"`
}

type TheaterSnapshotResult struct {
	RoomID        string                `json:"roomId"`
	WorldID       string                `json:"worldId"`
	ChannelID     string                `json:"channelId"`
	Revision      int64                 `json:"revision"`
	SchemaVersion int                   `json:"schemaVersion"`
	Checksum      string                `json:"checksum"`
	Unchanged     bool                  `json:"unchanged,omitempty"`
	Snapshot      TheaterSharedSnapshot `json:"snapshot"`
	Limits        map[string]int64      `json:"limits"`
	Permissions   []string              `json:"permissions"`
}

type TheaterEvent struct {
	MutationID     string          `json:"mutationId"`
	RevisionBefore int64           `json:"revisionBefore"`
	Revision       int64           `json:"revision"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAt      time.Time       `json:"createdAt"`
}

type TheaterEventsResult struct {
	FromRevision    int64          `json:"fromRevision"`
	ToRevision      int64          `json:"toRevision"`
	CurrentRevision int64          `json:"currentRevision"`
	HasMore         bool           `json:"hasMore"`
	Events          []TheaterEvent `json:"events"`
}

type TheaterResourceVariantPublic struct {
	Name      string  `json:"name"`
	MimeType  string  `json:"mimeType"`
	Width     *int    `json:"width"`
	Height    *int    `json:"height"`
	SizeBytes int64   `json:"sizeBytes"`
	URL       *string `json:"url"`
}

type TheaterResourcePublic struct {
	ID               string                         `json:"id"`
	Kind             string                         `json:"kind"`
	Status           string                         `json:"status"`
	MimeType         string                         `json:"mimeType"`
	SizeBytes        int64                          `json:"sizeBytes"`
	Width            *int                           `json:"width"`
	Height           *int                           `json:"height"`
	DurationMS       *int64                         `json:"durationMs"`
	FrameCount       *int                           `json:"frameCount"`
	FrameRate        *float64                       `json:"frameRate"`
	Animated         bool                           `json:"animated"`
	PosterResourceID *string                        `json:"posterResourceId"`
	Variants         []TheaterResourceVariantPublic `json:"variants"`
	Processing       TheaterResourceProcessing      `json:"processing"`
}

type TheaterResourceProcessing struct {
	Progress  float64 `json:"progress"`
	Retryable bool    `json:"retryable"`
	ErrorCode string  `json:"errorCode,omitempty"`
}

type TheaterActionCommand struct {
	ActionRequestID  string `json:"actionRequestId"`
	WorldID          string `json:"worldId"`
	ChannelID        string `json:"channelId"`
	ObjectID         string `json:"objectId"`
	ActionID         string `json:"actionId"`
	ExpectedRevision int64  `json:"expectedRevision"`
}

type TheaterActionResult struct {
	Kind       string                 `json:"kind"`
	Mutation   *TheaterMutationResult `json:"mutation,omitempty"`
	Descriptor json.RawMessage        `json:"descriptor,omitempty"`
	Chat       *TheaterChatSendResult `json:"chat,omitempty"`
}

type TheaterRestoreCommand struct {
	MutationID       string
	WorldID          string
	ChannelID        string
	SnapshotID       string
	Reason           string
	ExpectedRevision *int64
}

type TheaterReplaceCommand struct {
	MutationID       string
	WorldID          string
	ChannelID        string
	ExpectedRevision int64
	SchemaVersion    int
	Snapshot         TheaterSharedSnapshot
	Reason           string
}
