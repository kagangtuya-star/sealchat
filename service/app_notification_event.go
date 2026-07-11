package service

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

const (
	appNotificationTitleRuneLimit = 128
	appNotificationBodyRuneLimit  = 1024
)

var appNotificationAtTagIDPattern = regexp.MustCompile(`<at\b[^>]*\bid\s*=\s*(?:"([^"]+)"|'([^']+)')[^>]*/?>`)

type AppNotificationMessageSource struct {
	WorldID             string
	WorldName           string
	ChannelID           string
	ChannelName         string
	MessageID           string
	Content             string
	SenderUserID        string
	SenderName          string
	SenderAvatarURL     string
	IsWhisper           bool
	WhisperRecipientIDs []string
	CreatedAt           time.Time
}

type AppNotificationDeviceCandidate struct {
	DeviceID              string
	UserID                string
	ActiveWorldID         string
	WorldWhitelistEnabled bool
	WorldWhitelistIDs     map[string]struct{}
	CanRead               bool
}

type appNotificationTipTapMentionNode struct {
	Type    string                             `json:"type"`
	Text    string                             `json:"text"`
	Attrs   map[string]any                     `json:"attrs"`
	Content []appNotificationTipTapMentionNode `json:"content"`
}

func CollectMentionTargetIDsFromContent(content string) map[string]struct{} {
	targets := make(map[string]struct{})
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return targets
	}
	collectAppNotificationAtTags(trimmed, targets)
	if LooksLikeTipTapJSON(trimmed) {
		var node appNotificationTipTapMentionNode
		if json.Unmarshal([]byte(trimmed), &node) == nil {
			collectAppNotificationTipTapMentions(&node, targets)
		}
	}
	return targets
}

func BuildAppNotificationEvent(source AppNotificationMessageSource, recipientID string, sequence uint64, instanceID, webURL string) AppNotificationEvent {
	createdAt := source.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	plain := collapseAppNotificationWhitespace(NormalizeMessageContentToPlainText(source.Content))
	if plain == "" {
		plain = "发送了一条消息"
	}
	senderName := strings.TrimSpace(source.SenderName)
	if senderName == "" {
		senderName = "新消息"
	}
	body := truncateAppNotificationRunes(senderName+"："+plain, appNotificationBodyRuneLimit)
	title := truncateAppNotificationRunes(strings.TrimSpace(source.ChannelName), appNotificationTitleRuneLimit)
	if title == "" {
		title = "SealChat"
	}
	eventType := "message.created"
	notificationChannel := "message"
	mentions := CollectMentionTargetIDsFromContent(source.Content)
	if _, ok := mentions[strings.TrimSpace(recipientID)]; ok {
		eventType = "message.mentioned"
		notificationChannel = "mention"
	} else if _, ok := mentions["all"]; ok {
		eventType = "message.mentioned"
		notificationChannel = "mention"
	}

	base := strings.TrimRight(strings.TrimSpace(webURL), "/")
	openPath := base + "/#/" + url.PathEscape(source.WorldID) + "/" + url.PathEscape(source.ChannelID) + "?msg=" + url.QueryEscape(source.MessageID)
	fallbackPath := base + "/#/" + url.PathEscape(source.WorldID) + "/" + url.PathEscape(source.ChannelID)
	return AppNotificationEvent{
		SchemaVersion: "1.0",
		EventID:       "evt_" + utils.NewID(),
		Sequence:      sequence,
		EventType:     eventType,
		InstanceID:    instanceID,
		CreatedAt:     createdAt,
		ExpiresAt:     createdAt.Add(defaultAppNotificationRetention),
		DedupeKey:     "message:" + source.MessageID,
		Notification: AppNotificationDisplay{
			Channel: notificationChannel, Title: title, Body: body,
			CollapseKey: "channel:" + source.ChannelID, Sensitive: true,
		},
		Context: AppNotificationEventContext{
			World:   AppNotificationEntity{ID: source.WorldID, Name: source.WorldName},
			Channel: AppNotificationEntity{ID: source.ChannelID, Name: source.ChannelName},
			Message: AppNotificationMessageContext{ID: source.MessageID, IsWhisper: source.IsWhisper},
			Sender:  AppNotificationSenderContext{UserID: source.SenderUserID, DisplayName: senderName, AvatarURL: source.SenderAvatarURL},
		},
		Navigation: AppNotificationEventNavigation{OpenPath: openPath, FallbackPath: fallbackPath},
	}
}

func ShouldDeliverAppNotification(source AppNotificationMessageSource, candidate AppNotificationDeviceCandidate) bool {
	if strings.TrimSpace(candidate.UserID) == "" || candidate.UserID == source.SenderUserID || !candidate.CanRead {
		return false
	}
	if candidate.WorldWhitelistEnabled {
		if _, ok := candidate.WorldWhitelistIDs[source.WorldID]; !ok {
			return false
		}
	} else if strings.TrimSpace(candidate.ActiveWorldID) == "" || candidate.ActiveWorldID != source.WorldID {
		return false
	}
	if !source.IsWhisper {
		return true
	}
	for _, recipientID := range source.WhisperRecipientIDs {
		if strings.TrimSpace(recipientID) == candidate.UserID {
			return true
		}
	}
	return false
}

func EnqueueAppNotificationForMessage(messageID, webURL string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil
	}
	var message model.MessageModel
	if err := model.GetDB().Preload("User").Where("id = ?", messageID).First(&message).Error; err != nil {
		return err
	}
	channel, err := model.ChannelGet(message.ChannelID)
	if err != nil || channel == nil || strings.TrimSpace(channel.WorldID) == "" {
		return err
	}
	world, err := GetWorldByID(channel.WorldID)
	if err != nil || world == nil {
		return err
	}
	instanceID, err := model.EnsureAppNotificationInstanceID()
	if err != nil {
		return err
	}
	devices, err := model.ListActiveAppNotificationDevices()
	if err != nil {
		return err
	}
	deviceUserIDs := make([]string, 0, len(devices))
	for _, device := range devices {
		deviceUserIDs = append(deviceUserIDs, device.UserID)
	}
	preferences, err := model.GetAppNotificationPreferences(deviceUserIDs)
	if err != nil {
		return err
	}
	whisperRecipients := model.GetWhisperRecipientIDs(message.ID)
	if strings.TrimSpace(message.WhisperTo) != "" {
		whisperRecipients = append(whisperRecipients, strings.TrimSpace(message.WhisperTo))
	}
	senderName := strings.TrimSpace(message.SenderIdentityName)
	if senderName == "" {
		senderName = strings.TrimSpace(message.SenderMemberName)
	}
	senderAvatar := ""
	if message.User != nil {
		if senderName == "" {
			senderName = strings.TrimSpace(message.User.Nickname)
		}
		senderAvatar = strings.TrimSpace(message.User.Avatar)
	}
	source := AppNotificationMessageSource{
		WorldID: channel.WorldID, WorldName: world.Name,
		ChannelID: channel.ID, ChannelName: channel.Name,
		MessageID: message.ID, Content: message.Content,
		SenderUserID: message.UserID, SenderName: senderName, SenderAvatarURL: senderAvatar,
		IsWhisper: message.IsWhisper, WhisperRecipientIDs: uniqueAppNotificationStrings(whisperRecipients),
		CreatedAt: message.CreatedAt,
	}
	canReadByUser := map[string]bool{}
	for _, device := range devices {
		canRead, known := canReadByUser[device.UserID]
		if !known {
			canRead = IsWorldMember(channel.WorldID, device.UserID) && CanReadChannelByUserId(device.UserID, channel.ID)
			canReadByUser[device.UserID] = canRead
		}
		preference := preferences[device.UserID]
		candidate := AppNotificationDeviceCandidate{
			DeviceID: device.ID, UserID: device.UserID, ActiveWorldID: device.ActiveWorldID, CanRead: canRead,
		}
		if preference != nil && preference.WorldWhitelistEnabled {
			candidate.WorldWhitelistEnabled = true
			candidate.WorldWhitelistIDs = appNotificationWorldIDSet(preference.WorldWhitelistJSON)
		}
		if !ShouldDeliverAppNotification(source, candidate) {
			continue
		}
		sequence, err := model.AdvanceAppNotificationSequence(device.ID)
		if err != nil {
			return err
		}
		DefaultAppNotificationHub.Enqueue(device.ID, BuildAppNotificationEvent(source, device.UserID, sequence, instanceID, webURL))
	}
	return nil
}

func appNotificationWorldIDSet(raw string) map[string]struct{} {
	worldIDs := make([]string, 0)
	if json.Unmarshal([]byte(raw), &worldIDs) != nil {
		return map[string]struct{}{}
	}
	result := make(map[string]struct{}, len(worldIDs))
	for _, worldID := range worldIDs {
		if worldID = strings.TrimSpace(worldID); worldID != "" {
			result[worldID] = struct{}{}
		}
	}
	return result
}

func collectAppNotificationAtTags(content string, targets map[string]struct{}) {
	for _, match := range appNotificationAtTagIDPattern.FindAllStringSubmatch(content, -1) {
		for _, index := range []int{1, 2} {
			if index < len(match) {
				if id := strings.TrimSpace(match[index]); id != "" {
					targets[id] = struct{}{}
					break
				}
			}
		}
	}
}

func collectAppNotificationTipTapMentions(node *appNotificationTipTapMentionNode, targets map[string]struct{}) {
	if node == nil {
		return
	}
	collectAppNotificationAtTags(node.Text, targets)
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "mention", "satorimention":
		if id, ok := node.Attrs["id"].(string); ok && strings.TrimSpace(id) != "" {
			targets[strings.TrimSpace(id)] = struct{}{}
		}
	}
	for index := range node.Content {
		collectAppNotificationTipTapMentions(&node.Content[index], targets)
	}
}

func collapseAppNotificationWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncateAppNotificationRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func uniqueAppNotificationStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
