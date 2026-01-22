package service

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
)

type CharacterCardInput struct {
	ChannelID string
	Name      string
	SheetType string
	Attrs     map[string]any
}

func normalizeCharacterCardInput(input *CharacterCardInput, requireSheetType bool) error {
	if input == nil {
		return errors.New("参数错误")
	}
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	input.Name = strings.TrimSpace(input.Name)
	input.SheetType = strings.TrimSpace(input.SheetType)
	if input.ChannelID == "" {
		return errors.New("缺少频道ID")
	}
	if input.Name == "" {
		return errors.New("角色名不能为空")
	}
	if len([]rune(input.Name)) > 64 {
		return errors.New("角色名长度需在64个字符以内")
	}
	if requireSheetType && input.SheetType == "" {
		return errors.New("角色卡类型不能为空")
	}
	if input.SheetType != "" && len([]rune(input.SheetType)) > 32 {
		return errors.New("角色卡类型长度需在32个字符以内")
	}
	return nil
}

func normalizeCharacterCardAttrs(attrs map[string]any) (model.JSONMap, error) {
	if attrs == nil {
		return model.JSONMap{}, nil
	}
	if _, err := json.Marshal(attrs); err != nil {
		return nil, errors.New("角色卡属性格式错误")
	}
	return model.JSONMap(attrs), nil
}

func ensureCharacterCardAttrs(item *model.CharacterCardModel) {
	if item != nil && item.Attrs == nil {
		item.Attrs = model.JSONMap{}
	}
}

func CharacterCardList(userID string, channelID string) ([]*model.CharacterCardModel, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("缺少用户ID")
	}
	if strings.TrimSpace(channelID) != "" {
		if err := ensureChannelMembership(userID, channelID); err != nil {
			return nil, err
		}
	}
	items, err := model.CharacterCardList(userID, channelID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		ensureCharacterCardAttrs(item)
	}
	return items, nil
}

func CharacterCardGet(userID string, cardID string) (*model.CharacterCardModel, error) {
	item, err := model.CharacterCardGetByID(cardID)
	if err != nil {
		return nil, err
	}
	if item.UserID != userID {
		return nil, errors.New("无权访问该角色卡")
	}
	ensureCharacterCardAttrs(item)
	return item, nil
}

func CharacterCardCreate(userID string, input *CharacterCardInput) (*model.CharacterCardModel, error) {
	if err := normalizeCharacterCardInput(input, true); err != nil {
		return nil, err
	}
	if err := ensureChannelMembership(userID, input.ChannelID); err != nil {
		return nil, err
	}
	attrs, err := normalizeCharacterCardAttrs(input.Attrs)
	if err != nil {
		return nil, err
	}
	item := &model.CharacterCardModel{
		UserID:    userID,
		ChannelID: input.ChannelID,
		Name:      input.Name,
		SheetType: input.SheetType,
		Attrs:     attrs,
	}
	if err := model.CharacterCardCreate(item); err != nil {
		return nil, err
	}
	ensureCharacterCardAttrs(item)
	return item, nil
}

func CharacterCardUpdate(userID string, cardID string, input *CharacterCardInput) (*model.CharacterCardModel, error) {
	item, err := model.CharacterCardGetByID(cardID)
	if err != nil {
		return nil, err
	}
	if item.UserID != userID {
		return nil, errors.New("无权修改该角色卡")
	}
	input.ChannelID = item.ChannelID
	if err := normalizeCharacterCardInput(input, true); err != nil {
		return nil, err
	}
	if err := ensureChannelMembership(userID, item.ChannelID); err != nil {
		return nil, err
	}
	values := map[string]any{
		"name":       input.Name,
		"sheet_type": input.SheetType,
	}
	if input.Attrs != nil {
		attrs, err := normalizeCharacterCardAttrs(input.Attrs)
		if err != nil {
			return nil, err
		}
		values["attrs"] = attrs
	}
	if err := model.CharacterCardUpdate(cardID, values); err != nil {
		return nil, err
	}
	updated, err := model.CharacterCardGetByID(cardID)
	if err != nil {
		return nil, err
	}
	ensureCharacterCardAttrs(updated)
	return updated, nil
}

func CharacterCardDelete(userID string, cardID string) error {
	item, err := model.CharacterCardGetByID(cardID)
	if err != nil {
		return err
	}
	if item.UserID != userID {
		return errors.New("无权删除该角色卡")
	}
	if err := ensureChannelMembership(userID, item.ChannelID); err != nil {
		return err
	}
	if err := model.CharacterCardUnbindByCardID(cardID); err != nil {
		return err
	}
	return model.CharacterCardDelete(cardID)
}

func CharacterCardBindToIdentity(userID string, identityID string, cardID string) (*model.ChannelIdentityModel, error) {
	identityID = strings.TrimSpace(identityID)
	cardID = strings.TrimSpace(cardID)
	if identityID == "" {
		return nil, errors.New("缺少身份ID")
	}
	if cardID == "" {
		return nil, errors.New("缺少角色卡ID")
	}
	identity, err := model.ChannelIdentityGetByID(identityID)
	if err != nil {
		return nil, err
	}
	if identity.UserID != userID {
		return nil, errors.New("无权绑定该身份")
	}
	card, err := model.CharacterCardGetByID(cardID)
	if err != nil {
		return nil, err
	}
	if card.UserID != userID {
		return nil, errors.New("无权绑定该角色卡")
	}
	if card.ChannelID != identity.ChannelID {
		return nil, errors.New("角色卡不属于该频道")
	}
	if err := model.CharacterCardBindToIdentity(identity.ID, card.ID); err != nil {
		return nil, err
	}
	identity.CharacterCardID = card.ID
	return identity, nil
}

func CharacterCardUnbindFromIdentity(userID string, identityID string) (*model.ChannelIdentityModel, error) {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return nil, errors.New("缺少身份ID")
	}
	identity, err := model.ChannelIdentityGetByID(identityID)
	if err != nil {
		return nil, err
	}
	if identity.UserID != userID {
		return nil, errors.New("无权解绑该身份")
	}
	if err := model.CharacterCardBindToIdentity(identity.ID, ""); err != nil {
		return nil, err
	}
	identity.CharacterCardID = ""
	return identity, nil
}

func CharacterCardResolveForChannel(userID string, channelID string) (*model.CharacterCardModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.New("缺少频道ID")
	}
	if err := ensureChannelMembership(userID, channelID); err != nil {
		return nil, err
	}
	identity, err := model.ChannelIdentityFindDefault(channelID, userID)
	if err == nil && identity != nil {
		if identity.CharacterCardID != "" {
			card, err := model.CharacterCardGetByID(identity.CharacterCardID)
			if err == nil {
				if card.UserID == userID && card.ChannelID == channelID {
					ensureCharacterCardAttrs(card)
					return card, nil
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	card, err := model.CharacterCardGetLatest(userID, channelID)
	if err != nil {
		return nil, err
	}
	ensureCharacterCardAttrs(card)
	return card, nil
}

func CharacterCardUpsertByName(userID string, channelID string, name string, sheetType string, attrs map[string]any) (*model.CharacterCardModel, error) {
	input := &CharacterCardInput{
		ChannelID: channelID,
		Name:      name,
		SheetType: sheetType,
		Attrs:     attrs,
	}
	if err := normalizeCharacterCardInput(input, false); err != nil {
		return nil, err
	}
	if err := ensureChannelMembership(userID, input.ChannelID); err != nil {
		return nil, err
	}
	attrsMap, err := normalizeCharacterCardAttrs(input.Attrs)
	if err != nil {
		return nil, err
	}
	existing, err := model.CharacterCardGetByName(userID, input.ChannelID, input.Name)
	if err == nil {
		values := map[string]any{
			"name":  input.Name,
			"attrs": attrsMap,
		}
		if input.SheetType != "" {
			values["sheet_type"] = input.SheetType
		}
		if err := model.CharacterCardUpdate(existing.ID, values); err != nil {
			return nil, err
		}
		updated, err := model.CharacterCardGetByID(existing.ID)
		if err != nil {
			return nil, err
		}
		ensureCharacterCardAttrs(updated)
		return updated, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	item := &model.CharacterCardModel{
		UserID:    userID,
		ChannelID: input.ChannelID,
		Name:      input.Name,
		SheetType: input.SheetType,
		Attrs:     attrsMap,
	}
	if err := model.CharacterCardCreate(item); err != nil {
		return nil, err
	}
	ensureCharacterCardAttrs(item)
	return item, nil
}
