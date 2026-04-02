package service

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/pm"
)

type WorldExternalGlossaryLibraryView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsEnabled   bool   `json:"isEnabled"`
	IsBound     bool   `json:"isBound"`
	SortOrder   int    `json:"sortOrder"`
	TermCount   int64  `json:"termCount"`
}

func ensureWorldExternalGlossaryManagePermission(worldID, actorID string) error {
	if strings.TrimSpace(worldID) == "" || strings.TrimSpace(actorID) == "" {
		return ErrWorldPermission
	}
	if pm.CanWithSystemRole(actorID, pm.PermModAdmin) || IsWorldAdmin(worldID, actorID) {
		return nil
	}
	return ErrWorldPermission
}

func WorldExternalGlossaryList(worldID, actorID string) ([]*WorldExternalGlossaryLibraryView, error) {
	if strings.TrimSpace(worldID) == "" {
		return nil, ErrWorldNotFound
	}
	if !IsWorldMember(worldID, actorID) && !pm.CanWithSystemRole(actorID, pm.PermModAdmin) {
		return nil, ErrWorldPermission
	}
	var libraries []*model.ExternalGlossaryLibraryModel
	if err := model.GetDB().
		Order("sort_order DESC, updated_at DESC").
		Find(&libraries).Error; err != nil {
		return nil, err
	}
	if len(libraries) == 0 {
		return []*WorldExternalGlossaryLibraryView{}, nil
	}
	libraryIDs := make([]string, 0, len(libraries))
	for _, library := range libraries {
		if library != nil && library.ID != "" {
			libraryIDs = append(libraryIDs, library.ID)
		}
	}
	var bindings []model.WorldExternalGlossaryBindingModel
	if err := model.GetDB().Where("world_id = ? AND library_id IN ?", worldID, libraryIDs).Find(&bindings).Error; err != nil {
		return nil, err
	}
	boundSet := map[string]struct{}{}
	for _, binding := range bindings {
		boundSet[binding.LibraryID] = struct{}{}
	}
	type termCountRow struct {
		LibraryID string
		Count     int64
	}
	var counts []termCountRow
	if err := model.GetDB().Model(&model.ExternalGlossaryTermModel{}).
		Select("library_id, COUNT(*) as count").
		Where("library_id IN ?", libraryIDs).
		Group("library_id").
		Scan(&counts).Error; err != nil {
		return nil, err
	}
	countMap := map[string]int64{}
	for _, row := range counts {
		countMap[row.LibraryID] = row.Count
	}
	items := make([]*WorldExternalGlossaryLibraryView, 0, len(libraries))
	for _, library := range libraries {
		if library == nil {
			continue
		}
		_, isBound := boundSet[library.ID]
		items = append(items, &WorldExternalGlossaryLibraryView{
			ID:          library.ID,
			Name:        library.Name,
			Description: library.Description,
			IsEnabled:   library.IsEnabled,
			IsBound:     isBound,
			SortOrder:   library.SortOrder,
			TermCount:   countMap[library.ID],
		})
	}
	return items, nil
}

func WorldExternalGlossaryEnable(worldID, libraryID, actorID string) error {
	if err := ensureWorldExternalGlossaryManagePermission(worldID, actorID); err != nil {
		return err
	}
	worldID = strings.TrimSpace(worldID)
	libraryID = strings.TrimSpace(libraryID)
	if worldID == "" || libraryID == "" {
		return ErrWorldPermission
	}
	var library model.ExternalGlossaryLibraryModel
	if err := model.GetDB().Where("id = ?", libraryID).Limit(1).Find(&library).Error; err != nil {
		return err
	}
	if library.ID == "" {
		return ErrExternalGlossaryNotFound
	}
	if !library.IsEnabled {
		return ErrExternalGlossaryPermission
	}
	binding := &model.WorldExternalGlossaryBindingModel{
		WorldID:   worldID,
		LibraryID: libraryID,
		CreatedBy: actorID,
		UpdatedBy: actorID,
	}
	binding.Normalize()
	return model.GetDB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "world_id"}, {Name: "library_id"}},
		DoUpdates: clause.Assignments(map[string]any{"updated_by": actorID, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}),
	}).Create(binding).Error
}

func WorldExternalGlossaryDisable(worldID, libraryID, actorID string) error {
	if err := ensureWorldExternalGlossaryManagePermission(worldID, actorID); err != nil {
		return err
	}
	return model.GetDB().
		Where("world_id = ? AND library_id = ?", strings.TrimSpace(worldID), strings.TrimSpace(libraryID)).
		Delete(&model.WorldExternalGlossaryBindingModel{}).Error
}

func WorldExternalGlossaryBulkEnable(worldID string, libraryIDs []string, actorID string) (int64, error) {
	if err := ensureWorldExternalGlossaryManagePermission(worldID, actorID); err != nil {
		return 0, err
	}
	var count int64
	for _, raw := range libraryIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if err := WorldExternalGlossaryEnable(worldID, id, actorID); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func WorldExternalGlossaryBulkDisable(worldID string, libraryIDs []string, actorID string) (int64, error) {
	if err := ensureWorldExternalGlossaryManagePermission(worldID, actorID); err != nil {
		return 0, err
	}
	var count int64
	for _, raw := range libraryIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if err := WorldExternalGlossaryDisable(worldID, id, actorID); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
