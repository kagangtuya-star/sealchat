package protocol

const (
	EventWorldPresentationUpdated EventName = "world-presentation-updated"
	EventWorldPresentationPlay    EventName = "world-presentation-play"
)

type WorldPresentationPlayback struct {
	WorldID     string `json:"worldId"`
	ChannelID   string `json:"channelId"`
	Revision    string `json:"revision"`
	Kind        string `json:"kind"`
	TargetID    string `json:"targetId"`
	ExecutionID string `json:"executionId"`
}
