package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/pm"
)

func WorldCharacterCardTemplateShare(worldID, templateID, actorID string) error {
	worldID = strings.TrimSpace(worldID)
	templateID = strings.TrimSpace(templateID)
	actorID = strings.TrimSpace(actorID)
	if worldID == "" || templateID == "" || actorID == "" {
		return ErrWorldPermission
	}
	if !pm.CanWithSystemRole(actorID, pm.PermModAdmin) && !IsWorldAdmin(worldID, actorID) {
		return ErrWorldPermission
	}
	template, err := model.CharacterCardTemplateGetByID(templateID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("模板不存在")
		}
		return err
	}
	if template.ID == "" {
		return errors.New("模板不存在")
	}
	if template.UserID != actorID && !pm.CanWithSystemRole(actorID, pm.PermModAdmin) {
		return ErrWorldPermission
	}
	binding := &model.WorldCharacterCardTemplateBindingModel{
		WorldID:    worldID,
		TemplateID: templateID,
		CreatedBy:  actorID,
		UpdatedBy:  actorID,
	}
	binding.Normalize()
	return model.GetDB().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "world_id"}, {Name: "template_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"updated_by": actorID,
			"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}),
	}).Create(binding).Error
}

func WorldCharacterCardTemplateUnshare(worldID, templateID, actorID string) error {
	worldID = strings.TrimSpace(worldID)
	templateID = strings.TrimSpace(templateID)
	actorID = strings.TrimSpace(actorID)
	if worldID == "" || templateID == "" || actorID == "" {
		return ErrWorldPermission
	}
	if !pm.CanWithSystemRole(actorID, pm.PermModAdmin) && !IsWorldAdmin(worldID, actorID) {
		return ErrWorldPermission
	}
	return model.GetDB().Where("world_id = ? AND template_id = ?", worldID, templateID).
		Delete(&model.WorldCharacterCardTemplateBindingModel{}).Error
}

func IsCharacterCardTemplateSharedToWorld(templateID, worldID string) (bool, error) {
	templateID = strings.TrimSpace(templateID)
	worldID = strings.TrimSpace(worldID)
	if templateID == "" || worldID == "" {
		return false, nil
	}
	var count int64
	if err := model.GetDB().Model(&model.WorldCharacterCardTemplateBindingModel{}).
		Where("world_id = ? AND template_id = ?", worldID, templateID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
