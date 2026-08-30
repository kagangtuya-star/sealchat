package service

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
)

type PlatformCharacterCardTemplateInput struct {
	Name                       string
	SheetType                  string
	Content                    string
	BadgeTemplateOverride      string
	TheaterOverlayTemplateJSON string
	Enabled                    bool
	CreatedBy                  string
	UpdatedBy                  string
}

type PlatformCharacterCardTemplateUpdateInput struct {
	Name                       *string
	SheetType                  *string
	Content                    *string
	BadgeTemplateOverride      *string
	TheaterOverlayTemplateJSON *string
	Enabled                    *bool
	UpdatedBy                  string
}

func normalizePlatformCharacterCardTemplateInput(input *PlatformCharacterCardTemplateInput) error {
	if input == nil {
		return errors.New("参数错误")
	}
	input.Name = strings.TrimSpace(input.Name)
	input.SheetType = strings.TrimSpace(input.SheetType)
	input.Content = strings.TrimSpace(input.Content)
	input.BadgeTemplateOverride = strings.TrimSpace(input.BadgeTemplateOverride)
	input.TheaterOverlayTemplateJSON = strings.TrimSpace(input.TheaterOverlayTemplateJSON)
	if input.Name == "" {
		return errors.New("模板名称不能为空")
	}
	if utf8.RuneCountInString(input.Name) > 100 {
		return errors.New("模板名称长度需在100个字符以内")
	}
	if utf8.RuneCountInString(input.SheetType) > 32 {
		return errors.New("模板规则类型长度需在32个字符以内")
	}
	if input.Content == "" {
		return errors.New("模板内容不能为空")
	}
	if len(input.Content) > 512*1024 {
		return errors.New("模板内容不可超过512KB")
	}
	badge, err := normalizeWorldBadgeTemplate(input.BadgeTemplateOverride)
	if err != nil {
		return err
	}
	input.BadgeTemplateOverride = badge
	if input.TheaterOverlayTemplateJSON != "" {
		canonical, err := validateCharacterOverlayTemplate(input.TheaterOverlayTemplateJSON)
		if err != nil {
			return err
		}
		input.TheaterOverlayTemplateJSON = canonical
	}
	return nil
}

func normalizePlatformCharacterCardTemplateUpdateInput(input *PlatformCharacterCardTemplateUpdateInput) error {
	if input == nil {
		return errors.New("参数错误")
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		if value == "" {
			return errors.New("模板名称不能为空")
		}
		if utf8.RuneCountInString(value) > 100 {
			return errors.New("模板名称长度需在100个字符以内")
		}
		input.Name = &value
	}
	if input.SheetType != nil {
		value := strings.TrimSpace(*input.SheetType)
		if utf8.RuneCountInString(value) > 32 {
			return errors.New("模板规则类型长度需在32个字符以内")
		}
		input.SheetType = &value
	}
	if input.Content != nil {
		value := strings.TrimSpace(*input.Content)
		if value == "" {
			return errors.New("模板内容不能为空")
		}
		if len(value) > 512*1024 {
			return errors.New("模板内容不可超过512KB")
		}
		input.Content = &value
	}
	if input.BadgeTemplateOverride != nil {
		value, err := normalizeWorldBadgeTemplate(*input.BadgeTemplateOverride)
		if err != nil {
			return err
		}
		input.BadgeTemplateOverride = &value
	}
	if input.TheaterOverlayTemplateJSON != nil {
		value := strings.TrimSpace(*input.TheaterOverlayTemplateJSON)
		if value != "" {
			canonical, err := validateCharacterOverlayTemplate(value)
			if err != nil {
				return err
			}
			value = canonical
		}
		input.TheaterOverlayTemplateJSON = &value
	}
	return nil
}

func PlatformCharacterCardTemplateCreate(input *PlatformCharacterCardTemplateInput) (*model.PlatformCharacterCardTemplateModel, error) {
	if err := normalizePlatformCharacterCardTemplateInput(input); err != nil {
		return nil, err
	}
	item := &model.PlatformCharacterCardTemplateModel{
		Name: input.Name, SheetType: input.SheetType, Content: input.Content,
		BadgeTemplateOverride: input.BadgeTemplateOverride, TheaterOverlayTemplateJSON: input.TheaterOverlayTemplateJSON,
		Enabled: input.Enabled, CreatedBy: strings.TrimSpace(input.CreatedBy), UpdatedBy: strings.TrimSpace(input.UpdatedBy),
	}
	item.Normalize()
	if err := model.PlatformCharacterCardTemplateCreate(item); err != nil {
		return nil, err
	}
	return item, nil
}

func PlatformCharacterCardTemplateUpdate(id string, input *PlatformCharacterCardTemplateUpdateInput) (*model.PlatformCharacterCardTemplateModel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("模板ID不能为空")
	}
	if err := normalizePlatformCharacterCardTemplateUpdateInput(input); err != nil {
		return nil, err
	}
	item, err := model.PlatformCharacterCardTemplateGetByID(id)
	if err != nil {
		return nil, err
	}
	values := map[string]any{"updated_by": strings.TrimSpace(input.UpdatedBy)}
	if input.Name != nil {
		values["name"] = *input.Name
	}
	if input.SheetType != nil {
		values["sheet_type"] = *input.SheetType
	}
	if input.Content != nil {
		values["content"] = *input.Content
	}
	if input.BadgeTemplateOverride != nil {
		values["badge_template_override"] = *input.BadgeTemplateOverride
	}
	if input.TheaterOverlayTemplateJSON != nil {
		values["theater_overlay_template_json"] = *input.TheaterOverlayTemplateJSON
	}
	if input.Enabled != nil {
		values["enabled"] = *input.Enabled
	}
	if err := model.GetDB().Model(&model.PlatformCharacterCardTemplateModel{}).Where("id = ?", id).Updates(values).Error; err != nil {
		return nil, err
	}
	return model.PlatformCharacterCardTemplateGetByID(item.ID)
}

func PlatformCharacterCardTemplateSetEnabled(id string, enabled bool, actorID string) (*model.PlatformCharacterCardTemplateModel, error) {
	return PlatformCharacterCardTemplateUpdate(id, &PlatformCharacterCardTemplateUpdateInput{Enabled: &enabled, UpdatedBy: actorID})
}

func PlatformCharacterCardTemplateReferenceCount(id string) (int64, error) {
	var count int64
	ref := PlatformCharacterCardTemplateRefPrefix + strings.TrimSpace(id)
	err := model.GetDB().Model(&model.CharacterCardTemplateBindingModel{}).
		Where("template_id = ? AND mode = ?", ref, model.CharacterCardTemplateModeManaged).
		Count(&count).Error
	return count, err
}

func PlatformCharacterCardTemplateDelete(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("模板ID不能为空")
	}
	count, err := PlatformCharacterCardTemplateReferenceCount(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("模板仍被 %d 个人物卡绑定，请先解除绑定", count)
	}
	result := model.GetDB().Unscoped().Where("id = ?", id).Delete(&model.PlatformCharacterCardTemplateModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
