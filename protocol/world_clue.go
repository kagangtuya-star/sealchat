package protocol

import "encoding/json"

type WorldCluePresentation struct {
	Version                     int     `json:"version"`
	MediaPlacement              string  `json:"mediaPlacement"`
	MediaRatio                  float64 `json:"mediaRatio"`
	ObjectFit                   string  `json:"objectFit"`
	MediaType                   string  `json:"mediaType"`
	EnterAnimation              string  `json:"enterAnimation"`
	ExitAnimation               string  `json:"exitAnimation"`
	AnimationDurationMS         int     `json:"animationDurationMs"`
	TitleColor                  string  `json:"titleColor"`
	BackgroundColorEnabled      bool    `json:"backgroundColorEnabled"`
	BackgroundColor             string  `json:"backgroundColor"`
	BackgroundColorOpacity      int     `json:"backgroundColorOpacity"`
	BackgroundMediaEnabled      bool    `json:"backgroundMediaEnabled"`
	BackgroundMediaAttachmentID string  `json:"backgroundMediaAttachmentId"`
	BackgroundMediaURL          string  `json:"backgroundMediaUrl"`
	BackgroundMediaType         string  `json:"backgroundMediaType"`
	BackgroundMediaMode         string  `json:"backgroundMediaMode"`
	BackgroundMediaOpacity      int     `json:"backgroundMediaOpacity"`
	BackgroundMediaBlur         int     `json:"backgroundMediaBlur"`
	BackgroundMediaBrightness   int     `json:"backgroundMediaBrightness"`
}

type WorldClueUserState struct {
	PersonalFolderID         string `json:"personalFolderId,omitempty"`
	PersonalOrder            int    `json:"personalOrder"`
	Favorite                 bool   `json:"favorite"`
	SeenRevision             int64  `json:"seenRevision"`
	SeenPrivateRevision      int64  `json:"seenPrivateRevision"`
	AssignedPresentationSeq  int64  `json:"assignedPresentationSeq"`
	PresentedPresentationSeq int64  `json:"presentedPresentationSeq"`
	LastOpenedAt             int64  `json:"lastOpenedAt,omitempty"`
}

type WorldClueSummary struct {
	ID                string              `json:"id"`
	WorldID           string              `json:"worldId"`
	SharedFolderID    string              `json:"sharedFolderId,omitempty"`
	Title             string              `json:"title"`
	Kind              string              `json:"kind"`
	ContentText       string              `json:"contentText,omitempty"`
	ImageAttachmentID string              `json:"imageAttachmentId,omitempty"`
	ImageURL          string              `json:"imageUrl,omitempty"`
	EmbedDomain       string              `json:"embedDomain,omitempty"`
	DefaultAccess     string              `json:"defaultAccess,omitempty"`
	EffectiveAccess   string              `json:"effectiveAccess"`
	Status            string              `json:"status"`
	Revision          int64               `json:"revision"`
	PublishSeq        int64               `json:"publishSeq"`
	OrderIndex        int                 `json:"orderIndex"`
	PrivateRevision   int64               `json:"privateRevision,omitempty"`
	HasPrivateContent bool                `json:"hasPrivateContent"`
	Unread            bool                `json:"unread"`
	UserState         *WorldClueUserState `json:"userState,omitempty"`
	CreatorID         string              `json:"creatorId"`
	UpdatedAt         int64               `json:"updatedAt"`
	PublishedAt       int64               `json:"publishedAt,omitempty"`
}

type WorldClueDetail struct {
	WorldClueSummary
	ContentFormat        string                `json:"contentFormat"`
	Content              string                `json:"content"`
	EmbedURL             string                `json:"embedUrl,omitempty"`
	Presentation         WorldCluePresentation `json:"presentation"`
	PrivateContentFormat string                `json:"privateContentFormat,omitempty"`
	PrivateContent       string                `json:"privateContent,omitempty"`
}

type WorldClueAdminDetail struct {
	WorldClueDetail
	ManagerNoteFormat string `json:"managerNoteFormat"`
	ManagerNote       string `json:"managerNote"`
	ManagerNoteText   string `json:"managerNoteText"`
	UpdatedBy         string `json:"updatedBy"`
	PublishedBy       string `json:"publishedBy,omitempty"`
}

type WorldClueAccess struct {
	UserID            string `json:"userId"`
	Role              string `json:"role"`
	AccessOverride    string `json:"accessOverride"`
	EffectiveAccess   string `json:"effectiveAccess"`
	HasPrivateContent bool   `json:"hasPrivateContent"`
	PrivateRevision   int64  `json:"privateRevision"`
	PrivateExcerpt    string `json:"privateExcerpt,omitempty"`
}

type WorldClueRosterMember struct {
	UserID     string `json:"userId"`
	Role       string `json:"role"`
	Username   string `json:"username"`
	Nickname   string `json:"nickname"`
	Avatar     string `json:"avatar"`
	OrderIndex int    `json:"orderIndex"`
}

type WorldCluePrivateContent struct {
	UserID               string `json:"userId"`
	PrivateContentFormat string `json:"privateContentFormat"`
	PrivateContent       string `json:"privateContent"`
	PrivateRevision      int64  `json:"privateRevision"`
}

type WorldClueFolder struct {
	ID          string `json:"id"`
	WorldID     string `json:"worldId"`
	Scope       string `json:"scope"`
	OwnerUserID string `json:"ownerUserId,omitempty"`
	ParentID    string `json:"parentId,omitempty"`
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	OrderIndex  int    `json:"orderIndex"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

type WorldClueResolveItem struct {
	Accessible            bool   `json:"accessible"`
	EffectiveAccess       string `json:"effectiveAccess,omitempty"`
	Title                 string `json:"title,omitempty"`
	Kind                  string `json:"kind,omitempty"`
	MediaType             string `json:"mediaType,omitempty"`
	ThumbnailAttachmentID string `json:"thumbnailAttachmentId,omitempty"`
	ThumbnailURL          string `json:"thumbnailUrl,omitempty"`
	Excerpt               string `json:"excerpt,omitempty"`
	EmbedDomain           string `json:"embedDomain,omitempty"`
	Revision              int64  `json:"revision,omitempty"`
	UpdatedAt             int64  `json:"updatedAt,omitempty"`
	HasPrivateContent     bool   `json:"hasPrivateContent,omitempty"`
}

type WorldClueEventPayload struct {
	WorldID    string `json:"worldId"`
	ClueID     string `json:"clueId,omitempty"`
	PublishSeq int64  `json:"publishSeq,omitempty"`
	Action     string `json:"action,omitempty"`
	Revision   int64  `json:"revision,omitempty"`
}

func (p WorldClueEventPayload) MarshalJSON() ([]byte, error) {
	var revision *int64
	if p.Revision != 0 || p.Action == "edit-lock" {
		revisionValue := p.Revision
		revision = &revisionValue
	}
	return json.Marshal(struct {
		WorldID    string `json:"worldId"`
		ClueID     string `json:"clueId,omitempty"`
		PublishSeq int64  `json:"publishSeq,omitempty"`
		Action     string `json:"action,omitempty"`
		Revision   *int64 `json:"revision,omitempty"`
	}{WorldID: p.WorldID, ClueID: p.ClueID, PublishSeq: p.PublishSeq, Action: p.Action, Revision: revision})
}

type WorldClueEditingLock struct {
	Field     string `json:"field"`
	UserID    string `json:"userId"`
	SessionID string `json:"sessionId,omitempty"`
	ExpireAt  int64  `json:"expireAt"`
	User      *User  `json:"user,omitempty"`
}
