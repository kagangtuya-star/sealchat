package protocol

type Dice3DSkin struct {
	FaceBackground string            `json:"faceBackground"`
	FaceForeground string            `json:"faceForeground"`
	EdgeColor      string            `json:"edgeColor"`
	OutlineColor   string            `json:"outlineColor"`
	Roughness      float64           `json:"roughness"`
	Metalness      float64           `json:"metalness"`
	Scale          float64           `json:"scale"`
	Textures       map[string]string `json:"textures,omitempty"`
}

type Dice3DMotionConfig struct {
	Speed       float64 `json:"speed"`
	ThrowForce  float64 `json:"throwForce"`
	WallBounce  float64 `json:"wallBounce"`
	EntryEdge   string  `json:"entryEdge"`
	LingerMS    int     `json:"lingerMs"`
	MaxDice     int     `json:"maxDice"`
	Interactive bool    `json:"interactive"`
}

type Dice3DAudioConfig struct {
	Enabled      bool    `json:"enabled"`
	Volume       float64 `json:"volume"`
	SoundAssetID string  `json:"soundAssetId,omitempty"`
}

type Dice3DCustomSurface struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Dice3DDockStack struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Expression string `json:"expression"`
	Color      string `json:"color,omitempty"`
}

type Dice3DBotRule struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Enabled               bool     `json:"enabled"`
	ChannelIDs            []string `json:"channelIds,omitempty"`
	BotUserIDs            []string `json:"botUserIds,omitempty"`
	Pattern               string   `json:"pattern"`
	CountGroup            string   `json:"countGroup"`
	SidesGroup            string   `json:"sidesGroup"`
	ValuesGroup           string   `json:"valuesGroup"`
	ValueSeparatorPattern string   `json:"valueSeparatorPattern"`
	Priority              int      `json:"priority"`
}

type Dice3DWorldConfig struct {
	Version         int                 `json:"version"`
	PlatformStyleID string              `json:"platformStyleId,omitempty"`
	Enabled         bool                `json:"enabled"`
	SurfaceMode     string              `json:"surfaceMode"`
	CustomSurface   Dice3DCustomSurface `json:"customSurface"`
	DefaultSkin     Dice3DSkin          `json:"defaultSkin"`
	Motion          Dice3DMotionConfig  `json:"motion"`
	Audio           Dice3DAudioConfig   `json:"audio"`
	BotRules        []Dice3DBotRule     `json:"botRules,omitempty"`
}

type Dice3DMemberProfile struct {
	Version     int                `json:"version"`
	UseOverride bool               `json:"useOverride"`
	Skin        Dice3DSkin         `json:"skin"`
	Audio       *Dice3DAudioConfig `json:"audio,omitempty"`
	DockEnabled bool               `json:"dockEnabled"`
	DockCorner  string             `json:"dockCorner"`
	DockX       float64            `json:"dockX"`
	DockY       float64            `json:"dockY"`
	DockStacks  []Dice3DDockStack  `json:"dockStacks"`
}

type DiceVisualGroup struct {
	Type    string `json:"type"`
	Results []int  `json:"results"`
}

type DiceVisualPayload struct {
	Version       int                 `json:"version"`
	RollID        string              `json:"rollId"`
	MessageID     string              `json:"messageId"`
	ChannelID     string              `json:"channelId"`
	ActorUserID   string              `json:"actorUserId"`
	Seed          int64               `json:"seed"`
	Groups        []DiceVisualGroup   `json:"groups"`
	Appearance    Dice3DSkin          `json:"appearance"`
	Motion        Dice3DMotionConfig  `json:"motion"`
	Audio         Dice3DAudioConfig   `json:"audio"`
	SurfaceMode   string              `json:"surfaceMode"`
	CustomSurface Dice3DCustomSurface `json:"customSurface"`
	CreatedAt     int64               `json:"createdAt"`
}
