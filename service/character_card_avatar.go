package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
)

type CharacterCardAvatarBindingInput struct {
	ChannelID          string
	ExternalCardID     string
	CardName           string
	SheetType          string
	AvatarAttachmentID string
}

func normalizeCharacterCardAvatarBindingInput(input *CharacterCardAvatarBindingInput) error {
	if input == nil {
		return errors.New("参数错误")
	}
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	input.ExternalCardID = strings.TrimSpace(input.ExternalCardID)
	input.CardName = strings.TrimSpace(input.CardName)
	input.SheetType = strings.TrimSpace(input.SheetType)
	input.AvatarAttachmentID = strings.TrimSpace(input.AvatarAttachmentID)
	if input.ChannelID == "" {
		return errors.New("缺少频道ID")
	}
	if input.ExternalCardID == "" {
		return errors.New("缺少角色卡ID")
	}
	if len([]rune(input.CardName)) > 64 {
		return errors.New("角色名长度需在64个字符以内")
	}
	if input.SheetType != "" && len([]rune(input.SheetType)) > 32 {
		return errors.New("角色卡类型长度需在32个字符以内")
	}
	if input.AvatarAttachmentID == "" {
		return errors.New("缺少头像附件ID")
	}
	return nil
}

func ensureCharacterCardAvatarAttachmentOwnership(userID string, attachmentID string) error {
	if attachmentID == "" {
		return nil
	}
	_, err := ResolveAttachmentOwnership(userID, attachmentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("头像附件不存在")
		}
		return err
	}
	return nil
}

func CharacterCardAvatarBindingList(userID string, channelID string) ([]*model.CharacterCardAvatarBindingModel, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("缺少用户ID")
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.New("缺少频道ID")
	}
	if err := ensureChannelMembership(userID, channelID); err != nil {
		return nil, err
	}
	return model.CharacterCardAvatarBindingList(userID, channelID)
}

func CharacterCardAvatarBindingUpsert(userID string, input *CharacterCardAvatarBindingInput) (*model.CharacterCardAvatarBindingModel, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("缺少用户ID")
	}
	if err := normalizeCharacterCardAvatarBindingInput(input); err != nil {
		return nil, err
	}
	if err := ensureChannelMembership(userID, input.ChannelID); err != nil {
		return nil, err
	}
	if err := ensureCharacterCardAvatarAttachmentOwnership(userID, input.AvatarAttachmentID); err != nil {
		return nil, err
	}

	existing, err := model.CharacterCardAvatarBindingGet(userID, input.ChannelID, input.ExternalCardID)
	if err == nil {
		updates := map[string]any{
			"card_name":            input.CardName,
			"sheet_type":           input.SheetType,
			"avatar_attachment_id": input.AvatarAttachmentID,
		}
		if err := model.CharacterCardAvatarBindingUpdate(existing.ID, updates); err != nil {
			return nil, err
		}
		return model.CharacterCardAvatarBindingGet(userID, input.ChannelID, input.ExternalCardID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	item := &model.CharacterCardAvatarBindingModel{
		UserID:             userID,
		ChannelID:          input.ChannelID,
		ExternalCardID:     input.ExternalCardID,
		CardName:           input.CardName,
		SheetType:          input.SheetType,
		AvatarAttachmentID: input.AvatarAttachmentID,
	}
	if err := model.CharacterCardAvatarBindingCreate(item); err != nil {
		return nil, err
	}
	return item, nil
}

func CharacterCardAvatarBindingDelete(userID string, channelID string, externalCardID string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("缺少用户ID")
	}
	channelID = strings.TrimSpace(channelID)
	externalCardID = strings.TrimSpace(externalCardID)
	if channelID == "" {
		return errors.New("缺少频道ID")
	}
	if externalCardID == "" {
		return errors.New("缺少角色卡ID")
	}
	if err := ensureChannelMembership(userID, channelID); err != nil {
		return err
	}
	existing, err := model.CharacterCardAvatarBindingGet(userID, channelID, externalCardID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return model.CharacterCardAvatarBindingDelete(existing.ID)
}

func CharacterCardAvatarBindingMigrateLegacy(userID string, channelID string, items []*CharacterCardAvatarBindingInput) ([]*model.CharacterCardAvatarBindingModel, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("缺少用户ID")
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.New("缺少频道ID")
	}
	if err := ensureChannelMembership(userID, channelID); err != nil {
		return nil, err
	}
	created := make([]*model.CharacterCardAvatarBindingModel, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		payload := &CharacterCardAvatarBindingInput{
			ChannelID:          channelID,
			ExternalCardID:     item.ExternalCardID,
			CardName:           item.CardName,
			SheetType:          item.SheetType,
			AvatarAttachmentID: item.AvatarAttachmentID,
		}
		if err := normalizeCharacterCardAvatarBindingInput(payload); err != nil {
			continue
		}
		if _, ok := seen[payload.ExternalCardID]; ok {
			continue
		}
		seen[payload.ExternalCardID] = struct{}{}
		if err := ensureCharacterCardAvatarAttachmentOwnership(userID, payload.AvatarAttachmentID); err != nil {
			continue
		}
		existing, err := model.CharacterCardAvatarBindingGet(userID, channelID, payload.ExternalCardID)
		if err == nil && existing != nil {
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		record := &model.CharacterCardAvatarBindingModel{
			UserID:             userID,
			ChannelID:          channelID,
			ExternalCardID:     payload.ExternalCardID,
			CardName:           payload.CardName,
			SheetType:          payload.SheetType,
			AvatarAttachmentID: payload.AvatarAttachmentID,
		}
		if err := model.CharacterCardAvatarBindingCreate(record); err != nil {
			return nil, err
		}
		created = append(created, record)
	}
	return created, nil
}
