package onebotv11

import (
	"encoding/json"
	"strings"
	"time"
)

type NumberString string

func (ns NumberString) String() string {
	return string(ns)
}

func (ns NumberString) MarshalJSON() ([]byte, error) {
	trimmed := strings.TrimSpace(string(ns))
	if trimmed == "" {
		return []byte(`""`), nil
	}
	if isDigits(trimmed) {
		return []byte(trimmed), nil
	}
	return json.Marshal(trimmed)
}

func (ns *NumberString) UnmarshalJSON(data []byte) error {
	if ns == nil {
		return nil
	}
	str := strings.TrimSpace(string(data))
	if str == "null" {
		*ns = ""
		return nil
	}
	if len(str) > 0 && str[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*ns = NumberString(s)
		return nil
	}
	*ns = NumberString(str)
	return nil
}

func isDigits(val string) bool {
	if val == "" {
		return false
	}
	for _, r := range val {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type MessageSegment struct {
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}

type Message struct {
	ID         string           `json:"id,omitempty"`
	MessageID  int64            `json:"message_id,omitempty"`
	UserID     string           `json:"user_id,omitempty"`
	GroupID    string           `json:"group_id,omitempty"`
	RawMessage string           `json:"raw_message,omitempty"`
	Message    []MessageSegment `json:"message,omitempty"`
}

type Sender struct {
	UserID   NumberString `json:"user_id,omitempty"`
	Nickname string       `json:"nickname,omitempty"`
	Card     string       `json:"card,omitempty"`
}

type Event struct {
	Time       int64        `json:"time"`
	SelfID     NumberString `json:"self_id"`
	PostType   string       `json:"post_type"`
	DetailType string       `json:"detail_type"`
	SubType    string       `json:"sub_type,omitempty"`

	PostTypeAlt   string `json:"postType,omitempty"`
	DetailTypeAlt string `json:"detailType,omitempty"`
	SubTypeAlt    string `json:"subType,omitempty"`
	SelfIDAlt     string `json:"selfId,omitempty"`

	MessageID      int64            `json:"message_id,omitempty"`
	MessageSeq     int64            `json:"message_seq,omitempty"`
	MessageType    string           `json:"message_type,omitempty"`
	UserID         NumberString     `json:"user_id,omitempty"`
	GroupID        NumberString     `json:"group_id,omitempty"`
	ChannelID      NumberString     `json:"channel_id,omitempty"`
	MessageIDAlt   int64            `json:"messageId,omitempty"`
	MessageTypeAlt string           `json:"messageType,omitempty"`
	UserIDAlt      string           `json:"userId,omitempty"`
	GroupIDAlt     string           `json:"groupId,omitempty"`
	ChannelIDAlt   string           `json:"channelId,omitempty"`
	RawMessage     string           `json:"raw_message,omitempty"`
	Message        []MessageSegment `json:"message,omitempty"`
	Sender         *Sender          `json:"sender,omitempty"`

	// Echo fields for meta events
	Status  string      `json:"status,omitempty"`
	RetCode int         `json:"retcode,omitempty"`
	Echo    interface{} `json:"echo,omitempty"`
}

func (e *Event) NormalizeAliases() {
	if e == nil {
		return
	}
	if e.SelfID.String() == "" && strings.TrimSpace(e.SelfIDAlt) != "" {
		e.SelfID = NumberString(e.SelfIDAlt)
	}
	if e.PostType == "" && e.PostTypeAlt != "" {
		e.PostType = e.PostTypeAlt
	}
	if e.DetailType == "" && e.DetailTypeAlt != "" {
		e.DetailType = e.DetailTypeAlt
	}
	if e.SubType == "" && e.SubTypeAlt != "" {
		e.SubType = e.SubTypeAlt
	}
	if e.MessageType == "" && e.MessageTypeAlt != "" {
		e.MessageType = e.MessageTypeAlt
	}
	if e.GroupID.String() == "" && e.GroupIDAlt != "" {
		e.GroupID = NumberString(e.GroupIDAlt)
	}
	if e.ChannelID.String() == "" && e.ChannelIDAlt != "" {
		e.ChannelID = NumberString(e.ChannelIDAlt)
	}
	if e.UserID.String() == "" && e.UserIDAlt != "" {
		e.UserID = NumberString(e.UserIDAlt)
	}
	if e.MessageID == 0 && e.MessageIDAlt != 0 {
		e.MessageID = e.MessageIDAlt
	}
}

type ActionFrame struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params,omitempty"`
	Echo   interface{}     `json:"echo,omitempty"`
}

type ActionResponse struct {
	Status  string      `json:"status"`
	RetCode int         `json:"retcode"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Echo    interface{} `json:"echo,omitempty"`
}

func NewErrorResponse(echo interface{}, retcode int, msg string) *ActionResponse {
	return &ActionResponse{
		Status:  "failed",
		RetCode: retcode,
		Message: msg,
		Echo:    echo,
	}
}

func NewOKResponse(echo interface{}, data interface{}) *ActionResponse {
	return &ActionResponse{
		Status:  "ok",
		RetCode: 0,
		Data:    data,
		Echo:    echo,
	}
}

func NewMetaConnectEvent(selfID string) *Event {
	return &Event{
		Time:       time.Now().Unix(),
		SelfID:     NumberString(selfID),
		PostType:   "meta_event",
		DetailType: "connect",
	}
}
