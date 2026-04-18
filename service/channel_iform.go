package service

import (
	"errors"
	"strings"

	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/pm"
)

type ChannelIFormView struct {
	*model.ChannelIFormModel
	SourceChannelID string
	WorldShared     bool
	SharedRef       bool
	SharedWorldID   string
	Readonly        bool
}

func normalizeIFormIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func ListEffectiveChannelIForms(channelID string) ([]*ChannelIFormView, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return []*ChannelIFormView{}, nil
	}

	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || strings.TrimSpace(channel.ID) == "" {
		return nil, ErrWorldNotFound
	}

	localForms, err := model.ChannelIFormList(channelID)
	if err != nil {
		return nil, err
	}

	result := make([]*ChannelIFormView, 0, len(localForms))
	seen := make(map[string]struct{}, len(localForms))
	localFormIDs := make([]string, 0, len(localForms))
	for _, form := range localForms {
		if form == nil || strings.TrimSpace(form.ID) == "" {
			continue
		}
		seen[form.ID] = struct{}{}
		localFormIDs = append(localFormIDs, form.ID)
		result = append(result, &ChannelIFormView{
			ChannelIFormModel: form,
			SourceChannelID:   form.ChannelID,
			WorldShared:       false,
			SharedRef:         false,
			SharedWorldID:     strings.TrimSpace(channel.WorldID),
			Readonly:          false,
		})
	}

	worldID := strings.TrimSpace(channel.WorldID)
	if worldID == "" {
		return result, nil
	}

	var bindings []model.WorldIFormBindingModel
	if err := model.GetDB().
		Where("world_id = ?", worldID).
		Order("updated_at DESC").
		Order("created_at ASC").
		Find(&bindings).Error; err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return result, nil
	}

	sharedFormIDs := make([]string, 0, len(bindings))
	bindingSet := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		formID := strings.TrimSpace(binding.FormID)
		if formID == "" {
			continue
		}
		sharedFormIDs = append(sharedFormIDs, formID)
		bindingSet[formID] = struct{}{}
	}

	if len(localFormIDs) > 0 {
		for _, item := range result {
			if item == nil || item.ChannelIFormModel == nil {
				continue
			}
			_, item.WorldShared = bindingSet[item.ID]
		}
	}

	if len(sharedFormIDs) == 0 {
		return result, nil
	}

	var sharedForms []*model.ChannelIFormModel
	if err := model.GetDB().
		Where("id IN ?", sharedFormIDs).
		Order("order_index DESC").
		Order("created_at ASC").
		Find(&sharedForms).Error; err != nil {
		return nil, err
	}

	for _, form := range sharedForms {
		if form == nil || strings.TrimSpace(form.ID) == "" {
			continue
		}
		if _, ok := seen[form.ID]; ok {
			continue
		}
		if strings.TrimSpace(form.ChannelID) == channelID {
			continue
		}
		result = append(result, &ChannelIFormView{
			ChannelIFormModel: form,
			SourceChannelID:   form.ChannelID,
			WorldShared:       true,
			SharedRef:         true,
			SharedWorldID:     worldID,
			Readonly:          false,
		})
	}

	return result, nil
}

func SetWorldSharedChannelIForms(channelID, actorID string, formIDs []string, enabled bool) error {
	channelID = strings.TrimSpace(channelID)
	actorID = strings.TrimSpace(actorID)
	if channelID == "" || actorID == "" {
		return ErrWorldPermission
	}

	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return err
	}
	if channel == nil || strings.TrimSpace(channel.ID) == "" {
		return ErrWorldNotFound
	}
	worldID := strings.TrimSpace(channel.WorldID)
	if worldID == "" {
		return ErrWorldNotFound
	}
	if !IsWorldAdmin(worldID, actorID) && !pm.CanWithSystemRole(actorID, pm.PermModAdmin) {
		return ErrWorldPermission
	}

	localForms, err := model.ChannelIFormList(channelID)
	if err != nil {
		return err
	}
	localMap := make(map[string]*model.ChannelIFormModel, len(localForms))
	for _, form := range localForms {
		if form == nil || strings.TrimSpace(form.ID) == "" {
			continue
		}
		localMap[form.ID] = form
	}

	normalizedIDs := normalizeIFormIDs(formIDs)
	if len(normalizedIDs) == 0 {
		return errors.New("请至少选择一个控件")
	}

	for _, formID := range normalizedIDs {
		if _, ok := localMap[formID]; !ok {
			return errors.New("仅支持共享当前频道源控件")
		}
	}

	if !enabled {
		return model.GetDB().
			Where("world_id = ? AND form_id IN ?", worldID, normalizedIDs).
			Delete(&model.WorldIFormBindingModel{}).Error
	}

	for _, formID := range normalizedIDs {
		binding := &model.WorldIFormBindingModel{
			WorldID:   worldID,
			FormID:    formID,
			CreatedBy: actorID,
			UpdatedBy: actorID,
		}
		binding.Normalize()
		if err := model.GetDB().
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "world_id"}, {Name: "form_id"}},
				DoUpdates: clause.Assignments(map[string]any{"updated_by": actorID}),
			}).
			Create(binding).Error; err != nil {
			return err
		}
	}

	return nil
}

func ListChannelsAffectedByIForm(formID string) ([]string, error) {
	formID = strings.TrimSpace(formID)
	if formID == "" {
		return []string{}, nil
	}

	var form model.ChannelIFormModel
	if err := model.GetDB().Where("id = ?", formID).Limit(1).Find(&form).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(form.ID) == "" {
		return []string{}, nil
	}

	sourceChannelID := strings.TrimSpace(form.ChannelID)
	if sourceChannelID == "" {
		return []string{}, nil
	}

	channel, err := model.ChannelGet(sourceChannelID)
	if err != nil {
		return nil, err
	}
	worldID := ""
	if channel != nil {
		worldID = strings.TrimSpace(channel.WorldID)
	}

	var bindingCount int64
	if worldID != "" {
		if err := model.GetDB().Model(&model.WorldIFormBindingModel{}).
			Where("world_id = ? AND form_id = ?", worldID, formID).
			Count(&bindingCount).Error; err != nil {
			return nil, err
		}
	}
	if bindingCount == 0 || worldID == "" {
		return []string{sourceChannelID}, nil
	}

	var channels []*model.ChannelModel
	if err := model.GetDB().
		Where("world_id = ? AND status = ?", worldID, model.ChannelStatusActive).
		Order("created_at ASC").
		Find(&channels).Error; err != nil {
		return nil, err
	}

	result := make([]string, 0, len(channels))
	seen := map[string]struct{}{}
	for _, item := range channels {
		if item == nil {
			continue
		}
		channelID := strings.TrimSpace(item.ID)
		if channelID == "" {
			continue
		}
		if _, ok := seen[channelID]; ok {
			continue
		}
		seen[channelID] = struct{}{}
		result = append(result, channelID)
	}
	if _, ok := seen[sourceChannelID]; !ok {
		result = append(result, sourceChannelID)
	}
	return result, nil
}
