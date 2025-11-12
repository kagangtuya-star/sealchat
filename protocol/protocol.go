package protocol

type Channel struct {
	ID       string      `json:"id"`
	Type     ChannelType `json:"type"`
	Name     string      `json:"name"`
	ParentID string      `json:"parent_id" gorm:"null"`
	PermType string      `json:"permType"`
}

type ChannelType int

const (
	TextChannelType ChannelType = iota
	VoiceChannelType
	CategoryChannelType
	DirectChannelType
)

type Guild struct {
	ID     string
	Name   string
	Avatar string
}

type GuildRole struct {
	ID          string
	Name        string
	Color       int
	Position    int
	Permissions int64
	Hoist       bool
	Mentionable bool
}

type User struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Nick          string `json:"nick"`
	Avatar        string `json:"avatar"`
	Discriminator string `json:"discriminator"`
	IsBot         bool   `json:"is_bot"`

	// UserID        string // Deprecated
	// Username      string // Deprecated
	// Nickname      string // Deprecated
}

type ChannelIdentity struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"displayName"`
	Color              string `json:"color"`
	AvatarAttachmentID string `json:"avatarAttachmentId"`
	IsDefault          bool   `json:"isDefault"`
}

type GuildMember struct {
	ID       string           `json:"id"`
	User     *User            `json:"user"`
	Name     string           `json:"name"` // 指用户名吗？
	Nick     string           `json:"nick"`
	Avatar   string           `json:"avatar"`
	Title    string           `json:"title"`
	Roles    []string         `json:"roles"`
	JoinedAt int64            `json:"joined_at"`
	Identity *ChannelIdentity `json:"identity,omitempty"`
}

type Login struct {
	User     *User
	Platform string
	SelfID   string
	Status   Status
}

type Status int

const (
	StatusOffline Status = iota
	StatusOnline
	StatusConnect
	StatusDisconnect
	StatusReconnect
)

type Message struct {
	ID            string           `json:"id"`
	MessageID     string           // Deprecated
	Channel       *Channel         `json:"channel"`
	Guild         *Guild           `json:"guild"`
	User          *User            `json:"user"`
	Identity      *MessageIdentity `json:"identity,omitempty"`
	Member        *GuildMember     `json:"member"`
	Content       string           `json:"content"`
	Elements      []*Element       `json:"elements"`
	Timestamp     int64            `json:"timestamp"`
	Quote         *Message         `json:"quote"`
	CreatedAt     int64            `json:"createdAt"`
	UpdatedAt     int64            `json:"updatedAt"`
	DisplayOrder  float64          `json:"displayOrder"`
	IcMode        string           `json:"icMode"`
	IsWhisper     bool             `json:"isWhisper"`
	WhisperTo     *User            `json:"whisperTo"`
	IsEdited      bool             `json:"isEdited"`
	EditCount     int              `json:"editCount"`
	IsArchived    bool             `json:"isArchived"`
	ArchivedAt    int64            `json:"archivedAt"`
	ArchivedBy    string           `json:"archivedBy"`
	ArchiveReason string           `json:"archiveReason"`
	ClientID      string           `json:"clientId,omitempty"`
	WhisperMeta   *WhisperMeta     `json:"whisperMeta,omitempty"`
}

type MessageIdentity struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	Color            string `json:"color"`
	AvatarAttachment string `json:"avatarAttachment"`
}

type ChannelPresence struct {
	User     *User `json:"user"`
	Latency  int64 `json:"latency"`
	Focused  bool  `json:"focused"`
	LastSeen int64 `json:"lastSeen"`
}

type AudioTrackState struct {
	Type    string  `json:"type"`
	AssetID *string `json:"assetId"`
	Volume  float64 `json:"volume"`
	Muted   bool    `json:"muted"`
	Solo    bool    `json:"solo"`
	FadeIn  int     `json:"fadeIn"`
	FadeOut int     `json:"fadeOut"`
}

type AudioPlaybackStatePayload struct {
	ChannelID    string            `json:"channelId"`
	SceneID      *string           `json:"sceneId"`
	Tracks       []AudioTrackState `json:"tracks"`
	IsPlaying    bool              `json:"isPlaying"`
	Position     float64           `json:"position"`
	LoopEnabled  bool              `json:"loopEnabled"`
	PlaybackRate float64           `json:"playbackRate"`
	UpdatedBy    string            `json:"updatedBy"`
	UpdatedAt    int64             `json:"updatedAt"`
}

type ChannelIForm struct {
	ID               string                    `json:"id"`
	ChannelID        string                    `json:"channelId"`
	Name             string                    `json:"name"`
	Url              string                    `json:"url"`
	EmbedCode        string                    `json:"embedCode"`
	DefaultWidth     int                       `json:"defaultWidth"`
	DefaultHeight    int                       `json:"defaultHeight"`
	DefaultCollapsed bool                      `json:"defaultCollapsed"`
	DefaultFloating  bool                      `json:"defaultFloating"`
	AllowPopout      bool                      `json:"allowPopout"`
	OrderIndex       int                       `json:"orderIndex"`
	MediaOptions     *ChannelIFormMediaOptions `json:"mediaOptions,omitempty"`
	CreatedBy        string                    `json:"createdBy,omitempty"`
	UpdatedBy        string                    `json:"updatedBy,omitempty"`
	CreatedAt        int64                     `json:"createdAt,omitempty"`
	UpdatedAt        int64                     `json:"updatedAt,omitempty"`
}

type ChannelIFormMediaOptions struct {
	AutoPlay   bool `json:"autoPlay"`
	AutoUnmute bool `json:"autoUnmute"`
	AutoExpand bool `json:"autoExpand"`
	AllowAudio bool `json:"allowAudio"`
	AllowVideo bool `json:"allowVideo"`
}

type ChannelIFormStatePayload struct {
	FormID     string  `json:"formId"`
	Floating   bool    `json:"floating"`
	Collapsed  bool    `json:"collapsed"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Minimized  bool    `json:"minimized"`
	Force      bool    `json:"force"`
	AutoPlay   bool    `json:"autoPlay"`
	AutoUnmute bool    `json:"autoUnmute"`
}

type ChannelIFormEventPayload struct {
	Forms         []*ChannelIForm            `json:"forms,omitempty"`
	Form          *ChannelIForm              `json:"form,omitempty"`
	States        []ChannelIFormStatePayload `json:"states,omitempty"`
	State         *ChannelIFormStatePayload  `json:"state,omitempty"`
	Action        string                     `json:"action,omitempty"`
	TargetUserIDs []string                   `json:"targetUserIds,omitempty"`
}

type WhisperMeta struct {
	SenderMemberID   string `json:"senderMemberId,omitempty"`
	SenderMemberName string `json:"senderMemberName,omitempty"`
	SenderUserID     string `json:"senderUserId,omitempty"`
	SenderUserNick   string `json:"senderUserNick,omitempty"`
	SenderUserName   string `json:"senderUserName,omitempty"`
	TargetMemberID   string `json:"targetMemberId,omitempty"`
	TargetMemberName string `json:"targetMemberName,omitempty"`
	TargetUserID     string `json:"targetUserId,omitempty"`
	TargetUserNick   string `json:"targetUserNick,omitempty"`
	TargetUserName   string `json:"targetUserName,omitempty"`
}

type MessageReorder struct {
	MessageID    string  `json:"messageId"`
	ChannelID    string  `json:"channelId"`
	DisplayOrder float64 `json:"displayOrder"`
	BeforeID     string  `json:"beforeId,omitempty"`
	AfterID      string  `json:"afterId,omitempty"`
	ClientOpID   string  `json:"clientOpId,omitempty"`
}

type Button struct {
	ID string
}

type Command struct {
	Name        string
	Description map[string]string
	Arguments   []CommandDeclaration
	Options     []CommandDeclaration
	Children    []Command
}

type CommandDeclaration struct {
	Name        string
	Description map[string]string
	Type        string
	Required    bool
}

type Argv struct {
	Name      string
	Arguments []interface{}
	Options   map[string]interface{}
}

type EventName string

const (
	EventGenresAdded            EventName = "genres-added"
	EventGenresDeleted          EventName = "genres-deleted"
	EventMessage                EventName = "message"
	EventMessageCreated         EventName = "message-created"
	EventMessageDeleted         EventName = "message-deleted"
	EventMessageUpdated         EventName = "message-updated"
	EventMessageArchived        EventName = "message-archived"
	EventMessageUnarchived      EventName = "message-unarchived"
	EventMessagePinned          EventName = "message-pinned"
	EventMessageUnpinned        EventName = "message-unpinned"
	EventMessageReordered       EventName = "message-reordered"
	EventInteractionCommand     EventName = "interaction/command"
	EventReactionAdded          EventName = "reaction-added"
	EventReactionDeleted        EventName = "reaction-deleted"
	EventReactionDeletedOne     EventName = "reaction-deleted/one"
	EventReactionDeletedAll     EventName = "reaction-deleted/all"
	EventReactionDeletedEmoji   EventName = "reaction-deleted/emoji"
	EventSend                   EventName = "send"
	EventFriendRequest          EventName = "friend-request"
	EventGuildRequest           EventName = "guild-request"
	EventGuildMemberRequest     EventName = "guild-member-request"
	EventTypingPreview          EventName = "typing-preview"
	EventChannelPresenceUpdated EventName = "channel-presence-updated"
	EventAudioStateUpdated      EventName = "audio-state-updated"
	EventChannelIFormUpdated    EventName = "channel-iform-updated"
	EventChannelIFormPushed     EventName = "channel-iform-pushed"
)

type Event struct {
	ID         int64                      `json:"id"`
	Type       EventName                  `json:"type"`
	SelfID     string                     `json:"selfID"`
	Platform   string                     `json:"platform"`
	Timestamp  int64                      `json:"timestamp"`
	Argv       *Argv                      `json:"argv"`
	Channel    *Channel                   `json:"channel"`
	Guild      *Guild                     `json:"guild"`
	Login      *Login                     `json:"login"`
	Member     *GuildMember               `json:"member"`
	Message    *Message                   `json:"message"`
	Operator   *User                      `json:"operator"`
	Role       *GuildRole                 `json:"role"`
	User       *User                      `json:"user"`
	Button     *Button                    `json:"button"`
	Typing     *TypingPreview             `json:"typing"`
	Reorder    *MessageReorder            `json:"reorder"`
	Presence   []*ChannelPresence         `json:"presence"`
	AudioState *AudioPlaybackStatePayload `json:"audioState,omitempty"`
	IForm      *ChannelIFormEventPayload  `json:"iform,omitempty"`
}

type TypingState string

const (
	TypingStateIndicator TypingState = "indicator"
	TypingStateContent   TypingState = "content"
	TypingStateSilent    TypingState = "silent"
	// Deprecated aliases for backward compatibility with旧版本
	TypingStateOff TypingState = "off"
	TypingStateOn  TypingState = "on"
)

type TypingPreview struct {
	State        TypingState `json:"state"`
	Enabled      bool        `json:"enabled"`
	Content      string      `json:"content"`
	Mode         string      `json:"mode,omitempty"`
	MessageID    string      `json:"messageId,omitempty"`
	TargetUserID string      `json:"targetUserId,omitempty"`
}

type GatewayPayloadStructure struct {
	Op   Opcode      `json:"op"`
	Body interface{} `json:"body"`
}

type Opcode int

const (
	OpEvent Opcode = iota
	OpPing
	OpPong
	OpIdentify
	OpReady
	OpLatencyProbe
	OpLatencyResult
)

type GatewayBody struct {
	Event    Event
	Ping     struct{}
	Pong     struct{}
	Identify struct {
		Token    string
		Sequence int
	}
	Ready struct {
		Logins []Login
	}
}

type WebSocket struct {
	Connecting int
	Open       int
	Closing    int
	Closed     int
	ReadyState ReadyState
}

type ReadyState int

const (
	WebSocketConnecting ReadyState = iota
	WebSocketOpen
	WebSocketClosing
	WebSocketClosed
)
