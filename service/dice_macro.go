package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
)

type DiceMacroInput struct {
	ChannelID string
	Digits    string
	Label     string
	Expr      string
	Note      string
	Favorite  bool
}

const diceMacroImportLimit = 120

func normalizeDiceMacroInput(input *DiceMacroInput) error {
	input.Digits = strings.TrimSpace(input.Digits)
	input.Label = strings.TrimSpace(input.Label)
	input.Expr = strings.TrimSpace(input.Expr)
	input.Note = strings.TrimSpace(input.Note)
	if input.ChannelID == "" {
		return errors.New("缺少频道ID")
	}
	if input.Digits == "" {
		return errors.New("请输入数字序列")
	}
	if len(input.Digits) > 32 {
		return errors.New("数字序列长度不能超过32位")
	}
	if strings.IndexFunc(input.Digits, func(r rune) bool { return r < '1' || r > '9' }) != -1 {
		return errors.New("数字序列仅支持1-9")
	}
	if input.Label == "" {
		return errors.New("请输入名称")
	}
	if len([]rune(input.Label)) > 64 {
		return errors.New("名称长度需在64个字符以内")
	}
	if input.Expr == "" {
		return errors.New("请输入掷骰表达式")
	}
	if len([]rune(input.Expr)) > 128 {
		return errors.New("表达式长度过长")
	}
	if len([]rune(input.Note)) > 180 {
		return errors.New("备注长度需在180个字符以内")
	}
	return nil
}

func ensureChannelMembership(userID string, channelID string) error {
	member, err := model.MemberGetByUserIDAndChannelIDBase(userID, channelID, "", false)
	if err != nil {
		return err
	}
	if member == nil {
		return errors.New("仅频道成员可配置指令")
	}
	return nil
}

func DiceMacroList(userID string, channelID string) ([]*model.DiceMacroModel, error) {
	if err := ensureChannelMembership(userID, channelID); err != nil {
		return nil, err
	}
	return model.DiceMacroList(userID, channelID)
}

func DiceMacroCreate(userID string, input *DiceMacroInput) (*model.DiceMacroModel, error) {
	if input == nil {
		return nil, errors.New("缺少指令内容")
	}
	if err := ensureChannelMembership(userID, input.ChannelID); err != nil {
		return nil, err
	}
	if err := normalizeDiceMacroInput(input); err != nil {
		return nil, err
	}
	item := &model.DiceMacroModel{
		UserID:    userID,
		ChannelID: input.ChannelID,
		Digits:    input.Digits,
		Label:     input.Label,
		Expr:      input.Expr,
		Note:      input.Note,
		Favorite:  input.Favorite,
	}
	if err := model.DiceMacroSave(item); err != nil {
		return nil, err
	}
	return item, nil
}

func DiceMacroUpdate(userID string, macroID string, input *DiceMacroInput) (*model.DiceMacroModel, error) {
	if input == nil {
		return nil, errors.New("缺少指令内容")
	}
	macro, err := model.DiceMacroGetByID(macroID)
	if err != nil {
		return nil, err
	}
	if macro.UserID != userID {
		return nil, errors.New("无权修改该指令")
	}
	input.ChannelID = macro.ChannelID
	if err := ensureChannelMembership(userID, macro.ChannelID); err != nil {
		return nil, err
	}
	if err := normalizeDiceMacroInput(input); err != nil {
		return nil, err
	}
	values := map[string]any{
		"digits":   input.Digits,
		"label":    input.Label,
		"expr":     input.Expr,
		"note":     input.Note,
		"favorite": input.Favorite,
	}
	if err := model.DiceMacroUpdate(macroID, values); err != nil {
		return nil, err
	}
	return model.DiceMacroGetByID(macroID)
}

func DiceMacroDelete(userID string, channelID string, macroID string) error {
	if err := ensureChannelMembership(userID, channelID); err != nil {
		return err
	}
	macro, err := model.DiceMacroGetByID(macroID)
	if err != nil {
		return err
	}
	if macro.UserID != userID || macro.ChannelID != channelID {
		return errors.New("无权删除该指令")
	}
	return model.DiceMacroDelete(macroID)
}

func DiceMacroImport(userID string, channelID string, inputs []*DiceMacroInput) ([]*model.DiceMacroModel, error) {
	if err := ensureChannelMembership(userID, channelID); err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, errors.New("导入内容为空")
	}
	if len(inputs) > diceMacroImportLimit {
		return nil, fmt.Errorf("单次最多导入%d条指令", diceMacroImportLimit)
	}
	for idx, input := range inputs {
		input.ChannelID = channelID
		if err := normalizeDiceMacroInput(input); err != nil {
			return nil, fmt.Errorf("第%d条指令无效: %w", idx+1, err)
		}
	}
	db := model.GetDB()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := model.DiceMacroDeleteByChannel(tx, userID, channelID); err != nil {
			return err
		}
		for _, input := range inputs {
			item := &model.DiceMacroModel{
				UserID:    userID,
				ChannelID: channelID,
				Digits:    input.Digits,
				Label:     input.Label,
				Expr:      input.Expr,
				Note:      input.Note,
				Favorite:  input.Favorite,
			}
			if err := tx.Create(item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return model.DiceMacroList(userID, channelID)
}
