package service

import (
	"log"
	"strings"
	"time"

	"github.com/samber/lo"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/protocol/onebotv11"
	onebot "sealchat/service/onebot"
)

// ForwardOneBotEvent 将平台内的消息事件转发给已绑定的 OneBot 机器人
func ForwardOneBotEvent(channelID string, ev *protocol.Event) {
	if ev == nil || ev.Message == nil {
		return
	}
	message := ev.Message
	bindings, err := model.BotChannelBindingsByChannelID(channelID)
	if err != nil {
		log.Printf("onebot forward event load bindings failed channel=%s err=%v", channelID, err)
		return
	}
	if len(bindings) == 0 {
		log.Printf("onebot forward event skipped channel=%s bindings=0", channelID)
		return
	}
	channel, _ := model.ChannelGet(channelID)
	log.Printf("onebot forward event channel=%s bindings=%d msg=%s", channelID, len(bindings), message.ID)
	for _, binding := range bindings {
		if binding == nil || !binding.Enabled {
			continue
		}
		profile, err := model.BotProfileGet(binding.BotID)
		if err != nil || profile == nil {
			continue
		}
		// 避免机器人收到自己发送的事件导致循环
		if message.User != nil && message.User.ID == profile.UserID {
			continue
		}
		mapping, err := model.EnsureBotMessageMapping(profile.ID, binding.ChannelID, message.ID)
		if err != nil {
			log.Printf("onebot mapping error: %v", err)
			continue
		}
		log.Printf("onebot forward event profile=%s binding=%s remoteGroup=%s remoteNumeric=%s remoteChannel=%s mapping=%d", profile.ID, binding.ID, binding.RemoteGroupID, binding.RemoteNumericID, binding.RemoteChannelID, mapping.ID)
		obEvent := convertProtocolEventToOneBot(ev, profile, binding, channel, mapping.ID)
		if obEvent == nil {
			log.Printf("onebot forward event skip empty payload profile=%s", profile.ID)
			continue
		}
		onebot.BroadcastEvent(profile.ID, obEvent)
	}
}

func convertProtocolEventToOneBot(ev *protocol.Event, profile *model.BotProfileModel, binding *model.BotChannelBindingModel, channel *model.ChannelModel, externalMessageID int64) *onebotv11.Event {
	if ev == nil || ev.Message == nil {
		return nil
	}
	message := ev.Message
	text := strings.TrimSpace(message.Content)
	segments := buildSegments(message)
	if len(segments) == 0 && text == "" {
		return nil
	}
	selfNumericID := profile.NumericSelfID()
	if strings.TrimSpace(selfNumericID) == "" {
		log.Printf("onebot forward event skip: profile=%s missing remote self id", profile.ID)
		return nil
	}
	selfNumber := onebotv11.NumberString(selfNumericID)

	event := &onebotv11.Event{
		Time:      time.Now().Unix(),
		SelfID:    selfNumber,
		PostType:  "message",
		MessageID: externalMessageID,
		RawMessage: func() string {
			if text != "" {
				return text
			}
			if len(segments) > 0 && segments[0].Type == "text" {
				return segments[0].Data["text"]
			}
			return ""
		}(),
		Message: segments,
	}

	messageType := "group"
	groupNumericID := firstNonEmpty(
		NormalizeOneBotNumericID(binding.RemoteNumericID),
		NormalizeOneBotNumericID(binding.RemoteGroupID),
		NormalizeOneBotNumericID(binding.RemoteChannelID),
		NormalizeOneBotNumericID(binding.ChannelID),
	)
	if groupNumericID == "" {
		log.Printf("onebot forward event skip: profile=%s binding=%s missing remote group id", profile.ID, binding.ID)
		return nil
	}
	if channel != nil && (channel.IsPrivate || channel.PermType == "private") {
		messageType = "private"
		event.UserID = pickUserNumber(message, selfNumber)
	} else {
		event.GroupID = onebotv11.NumberString(groupNumericID)
		event.UserID = pickUserNumber(message, selfNumber)
	}
	event.MessageType = messageType
	event.DetailType = messageType

	if event.UserID.String() != "" {
		event.Sender = &onebotv11.Sender{
			UserID:   event.UserID,
			Nickname: pickDisplayName(message),
		}
	}
	return event
}

func pickUserNumber(message *protocol.Message, fallback onebotv11.NumberString) onebotv11.NumberString {
	if message == nil {
		return fallback
	}
	if message.WhisperTo != nil {
		if val := NormalizeOneBotNumericID(message.WhisperTo.ID); val != "" {
			return onebotv11.NumberString(val)
		}
	}
	if message.User != nil {
		if val := NormalizeOneBotNumericID(message.User.ID); val != "" {
			return onebotv11.NumberString(val)
		}
	}
	return fallback
}

func pickDisplayName(msg *protocol.Message) string {
	if msg == nil {
		return ""
	}
	if msg.User != nil && strings.TrimSpace(msg.User.Nick) != "" {
		return msg.User.Nick
	}
	if msg.Identity != nil && strings.TrimSpace(msg.Identity.DisplayName) != "" {
		return msg.Identity.DisplayName
	}
	if msg.Member != nil && strings.TrimSpace(msg.Member.Nick) != "" {
		return msg.Member.Nick
	}
	if msg.User != nil && strings.TrimSpace(msg.User.Name) != "" {
		return msg.User.Name
	}
	return ""
}

func buildSegments(msg *protocol.Message) []onebotv11.MessageSegment {
	if msg == nil {
		return nil
	}
	var segments []onebotv11.MessageSegment

	text := strings.TrimSpace(msg.Content)
	if text != "" {
		segments = append(segments, onebotv11.MessageSegment{
			Type: "text",
			Data: map[string]string{"text": text},
		})
	}
	for _, el := range msg.Elements {
		if el == nil || strings.TrimSpace(el.Type) == "" {
			continue
		}
		switch el.Type {
		case "text":
			if t, ok := el.Attrs["text"].(string); ok && strings.TrimSpace(t) != "" {
				segments = append(segments, onebotv11.MessageSegment{
					Type: "text",
					Data: map[string]string{"text": t},
				})
			}
		case "image":
			url := ""
			if val, ok := el.Attrs["url"].(string); ok {
				url = val
			}
			if url != "" {
				segments = append(segments, onebotv11.MessageSegment{
					Type: "image",
					Data: map[string]string{"file": url},
				})
			}
		}
	}
	// 保证至少有一个文本段
	if len(segments) == 0 && text != "" {
		segments = []onebotv11.MessageSegment{
			{Type: "text", Data: map[string]string{"text": text}},
		}
	}
	return lo.Slice(segments, 0, len(segments))
}
