package service

// Typed MCP design data shared by small edits and Design Plans.
type TheaterMCPSceneFields struct {
	Name       *string `json:"name,omitempty"`
	SwitchText *string `json:"switchText,omitempty"`
	Order      *int64  `json:"order,omitempty"`
	FolderID   *string `json:"folderId,omitempty"`
	Locked     *bool   `json:"locked,omitempty"`
	Published  *bool   `json:"published,omitempty"`
}
type TheaterMCPImage struct {
	ResourceID string `json:"resourceId"`
	URL        string `json:"url"`
	Alt        string `json:"alt,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
	Animated   bool   `json:"animated,omitempty"`
	LoopCount  int    `json:"loopCount,omitempty"`
}
type TheaterMCPIframe struct {
	URL   string  `json:"url"`
	Scale float64 `json:"scale"`
}
type TheaterMCPTransitionPhase struct {
	Type       string `json:"type"`
	DurationMS int64  `json:"durationMs"`
}
type TheaterMCPTransition struct {
	Curtain    *bool                      `json:"curtain,omitempty"`
	Enter      *TheaterMCPTransitionPhase `json:"enter,omitempty"`
	Exit       *TheaterMCPTransitionPhase `json:"exit,omitempty"`
	Type       string                     `json:"type,omitempty"`
	DurationMS *int64                     `json:"durationMs,omitempty"`
}
type TheaterMCPAudio struct {
	AssetID string  `json:"assetId"`
	Name    string  `json:"name"`
	Volume  float64 `json:"volume"`
}
type TheaterMCPMusicTrack struct {
	Type          string                 `json:"type"`
	Asset         *TheaterMCPMusicAsset  `json:"asset"`
	Volume        float64                `json:"volume"`
	FadeIn        int                    `json:"fadeIn"`
	FadeOut       int                    `json:"fadeOut"`
	LoopEnabled   bool                   `json:"loopEnabled"`
	PlaybackRate  float64                `json:"playbackRate"`
	PlaylistMode  *string                `json:"playlistMode"`
	Playlist      []TheaterMCPMusicAsset `json:"playlist"`
	PlaylistIndex int                    `json:"playlistIndex"`
}
type TheaterMCPMusicAsset struct {
	AssetID string `json:"assetId"`
	Name    string `json:"name"`
}
type TheaterMCPMusic struct {
	Version int                    `json:"version"`
	Tracks  []TheaterMCPMusicTrack `json:"tracks"`
}
type TheaterMCPOverlay struct {
	Version   int            `json:"version"`
	ID        string         `json:"id"`
	EffectID  string         `json:"effectId"`
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Opacity   float64        `json:"opacity"`
	BlendMode string         `json:"blendMode"`
	Layer     string         `json:"layer"`
	Media     map[string]any `json:"media,omitempty"`
	Params    map[string]any `json:"params"`
}
type TheaterMCPAction struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Schedule *TheaterMCPSchedule `json:"schedule,omitempty"`
	Payload  map[string]any      `json:"payload"`
}
type TheaterMCPSchedule struct {
	DelayMS int `json:"delayMs"`
}
type TheaterMCPSequence struct {
	Version   int                         `json:"version"`
	ID        string                      `json:"id"`
	Name      string                      `json:"name"`
	Enabled   bool                        `json:"enabled"`
	Triggers  []TheaterMCPSequenceTrigger `json:"triggers"`
	LoopCount int                         `json:"loopCount"`
	Steps     []TheaterMCPSequenceStep    `json:"steps"`
}
type TheaterMCPSequenceStep struct {
	ID      string                   `json:"id"`
	SceneID *string                  `json:"sceneId"`
	Timing  TheaterMCPSequenceTiming `json:"timing"`
	Action  TheaterMCPAction         `json:"action"`
}
type TheaterMCPSequenceTiming struct {
	Mode    string `json:"mode"`
	DelayMS *int   `json:"delayMs,omitempty"`
}
type TheaterMCPSequenceTrigger struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Threshold       int      `json:"threshold"`
	Every           int      `json:"every"`
	CooldownMS      int      `json:"cooldownMs"`
	Keywords        []string `json:"keywords,omitempty"`
	TargetActorName *string  `json:"targetActorName,omitempty"`
	ObjectID        string   `json:"objectId,omitempty"`
}
type TheaterMCPSurfaceStyle struct {
	Brightness float64                  `json:"brightness"`
	BlurPx     float64                  `json:"blurPx"`
	Opacity    float64                  `json:"opacity"`
	Zoom       float64                  `json:"zoom"`
	Fit        string                   `json:"fit"`
	Overlay    TheaterMCPSurfaceOverlay `json:"overlay"`
	MediaFx    map[string]any           `json:"mediaFx,omitempty"`
}
type TheaterMCPSurfaceOverlay struct {
	Enabled bool    `json:"enabled"`
	Color   string  `json:"color"`
	Opacity float64 `json:"opacity"`
}

// Explicit transform fields; content/actions are existing domain data, validated
// by the native schema. There is no arbitrary mutation type or top-level payload.
type TheaterMCPObjectFields struct {
	SceneID           *string             `json:"sceneId,omitempty"`
	ParentID          *string             `json:"parentId,omitempty"`
	Name              *string             `json:"name,omitempty"`
	X                 *float64            `json:"x,omitempty"`
	Y                 *float64            `json:"y,omitempty"`
	Width             *float64            `json:"width,omitempty"`
	Height            *float64            `json:"height,omitempty"`
	Rotation          *float64            `json:"rotation,omitempty"`
	Scale             *float64            `json:"scale,omitempty"`
	ScaleX            *float64            `json:"scaleX,omitempty"`
	ScaleY            *float64            `json:"scaleY,omitempty"`
	Z                 *float64            `json:"z,omitempty"`
	OrderKey          *string             `json:"orderKey,omitempty"`
	Visible           *bool               `json:"visible,omitempty"`
	Locked            *bool               `json:"locked,omitempty"`
	AspectRatioLocked *bool               `json:"aspectRatioLocked,omitempty"`
	Interactive       *bool               `json:"interactive,omitempty"`
	Editable          *bool               `json:"editable,omitempty"`
	Content           *TheaterMCPContent  `json:"content,omitempty"`
	Actions           *[]TheaterMCPAction `json:"actions,omitempty"`
	// Stored as metadata.embedEventBindings and merged at expectedRevision, so
	// other metadata keys are preserved. Only iframe objects accept it.
	EmbedEventBindings *[]TheaterMCPEmbedEventBinding `json:"embedEventBindings,omitempty"`
}
type TheaterMCPEmbedEventBinding struct {
	Topic     string   `json:"topic"`
	ActionIDs []string `json:"actionIds"`
}
type TheaterMCPContent struct {
	Fill       *string           `json:"fill,omitempty"`
	Text       *string           `json:"text,omitempty"`
	Image      *TheaterMCPImage  `json:"image,omitempty"`
	Iframe     *TheaterMCPIframe `json:"iframe,omitempty"`
	Drawing    map[string]any    `json:"drawing,omitempty"`
	Effect     map[string]any    `json:"effect,omitempty"`
	Video      map[string]any    `json:"video,omitempty"`
	Annotation map[string]any    `json:"annotation,omitempty"`
	Style      map[string]any    `json:"style,omitempty"`
}
type TheaterMCPObjectUpdate struct {
	ObjectID string                 `json:"objectId"`
	Fields   TheaterMCPObjectFields `json:"fields"`
}
