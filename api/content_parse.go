package api

import (
	"strings"

	"sealchat/model"
	"sealchat/utils"

	"gorm.io/gorm"
)

type messageMentionWatermarkAudience struct {
	channelWide bool
	userIDs     []string
}

type messageMentionInvalidationPlan struct {
	channelID string
	senderID  string
	audience  messageMentionWatermarkAudience
}

func (ctx *ChatContext) TagCheck(message *model.MessageModel) error {
	if message == nil || message.ID == "" || message.ChannelID == "" {
		return nil
	}

	mentions := make([]model.MentionModel, 0)
	for receiverID := range collectMentionTargetIDsFromContent(message.Content) {
		if receiverID == "" {
			continue
		}
		mentions = append(mentions, model.MentionModel{
			StringPKBaseModel: model.StringPKBaseModel{
				ID: utils.NewID(),
			},
			ReceiverId:  receiverID,
			SenderId:    message.UserID,
			LocPostType: "channel",
			LocPostID:   message.ChannelID,
			RelatedType: "message",
			RelatedID:   message.ID,
		})
	}
	if len(mentions) == 0 {
		return nil
	}

	messageTime := message.CreatedAt.UnixMilli()
	audience := resolveMessageMentionWatermarkAudience(message, mentions)
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&mentions).Error; err != nil {
			return err
		}
		if audience.channelWide {
			return model.ChannelMentionAdvanceForChannelTx(tx, message.ChannelID, message.UserID, messageTime)
		}
		return model.ChannelMentionAdvanceForUsersTx(tx, message.ChannelID, message.UserID, audience.userIDs, messageTime)
	})
}

func resolveMessageMentionWatermarkAudience(message *model.MessageModel, mentions []model.MentionModel) messageMentionWatermarkAudience {
	if message == nil || len(mentions) == 0 {
		return messageMentionWatermarkAudience{}
	}

	hasAll := false
	directTargets := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		if mention.ReceiverId == "all" {
			hasAll = true
			continue
		}
		directTargets = append(directTargets, mention.ReceiverId)
	}

	if !message.IsWhisper {
		if hasAll {
			return messageMentionWatermarkAudience{channelWide: true}
		}
		return messageMentionWatermarkAudience{userIDs: directTargets}
	}

	visible := make(map[string]struct{})
	for _, userID := range model.GetWhisperRecipientIDs(message.ID) {
		if userID = strings.TrimSpace(userID); userID != "" {
			visible[userID] = struct{}{}
		}
	}
	for _, userID := range model.SplitWhisperTargetIDs(message.WhisperTo) {
		visible[userID] = struct{}{}
	}
	watermarkTargets := make([]string, 0, len(visible))
	if hasAll {
		for userID := range visible {
			watermarkTargets = append(watermarkTargets, userID)
		}
	} else {
		for _, userID := range directTargets {
			if _, ok := visible[userID]; ok {
				watermarkTargets = append(watermarkTargets, userID)
			}
		}
	}
	return messageMentionWatermarkAudience{userIDs: watermarkTargets}
}

func loadMessageMentionInvalidationPlans(messages []*model.MessageModel) ([]messageMentionInvalidationPlan, error) {
	messageByID := make(map[string]*model.MessageModel, len(messages))
	messageIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		if message == nil || message.ID == "" {
			continue
		}
		messageByID[message.ID] = message
		messageIDs = append(messageIDs, message.ID)
	}
	if len(messageIDs) == 0 {
		return nil, nil
	}

	var mentions []model.MentionModel
	if err := model.GetDB().
		Where("loc_post_type = ? AND related_type = ? AND related_id IN ?", "channel", "message", messageIDs).
		Find(&mentions).Error; err != nil {
		return nil, err
	}
	mentionsByMessageID := make(map[string][]model.MentionModel)
	for _, mention := range mentions {
		mentionsByMessageID[mention.RelatedID] = append(mentionsByMessageID[mention.RelatedID], mention)
	}

	plans := make([]messageMentionInvalidationPlan, 0, len(mentionsByMessageID))
	for messageID, messageMentions := range mentionsByMessageID {
		message := messageByID[messageID]
		if message == nil {
			continue
		}
		plans = append(plans, messageMentionInvalidationPlan{
			channelID: message.ChannelID,
			senderID:  message.UserID,
			audience:  resolveMessageMentionWatermarkAudience(message, messageMentions),
		})
	}
	return plans, nil
}

func invalidateMessageMentionWatermarksTx(tx *gorm.DB, plans []messageMentionInvalidationPlan) error {
	for _, plan := range plans {
		if plan.audience.channelWide {
			if err := model.ChannelMentionInvalidateForChannelTx(tx, plan.channelID, plan.senderID); err != nil {
				return err
			}
			continue
		}
		if err := model.ChannelMentionInvalidateForUsersTx(tx, plan.channelID, plan.senderID, plan.audience.userIDs); err != nil {
			return err
		}
	}
	return nil
}
