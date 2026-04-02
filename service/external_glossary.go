package service

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
)

var (
	ErrExternalGlossaryNotFound   = errors.New("external glossary not found")
	ErrExternalGlossaryPermission = errors.New("external glossary permission denied")
)

type ExternalGlossaryLibraryInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     *bool  `json:"isEnabled"`
	SortOrder   *int   `json:"sortOrder"`
}

type ExternalGlossaryLibraryListOptions struct {
	Page            int
	PageSize        int
	Query           string
	IncludeDisabled bool
}

func ensureExternalGlossaryAdmin(actorID string) error {
	if strings.TrimSpace(actorID) == "" || !pm.CanWithSystemRole(actorID, pm.PermModAdmin) {
		return ErrExternalGlossaryPermission
	}
	return nil
}

func normalizeExternalGlossaryLibraryInput(input *ExternalGlossaryLibraryInput) error {
	if input == nil {
		return ErrExternalGlossaryNotFound
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" {
		return errors.New("术语库名称不能为空")
	}
	if utf8.RuneCountInString(input.Name) > 120 {
		input.Name = string([]rune(input.Name)[:120])
	}
	if utf8.RuneCountInString(input.Description) > 500 {
		input.Description = string([]rune(input.Description)[:500])
	}
	return nil
}

func normalizeExternalGlossaryCategoryName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("分类名称不能为空")
	}
	if utf8.RuneCountInString(trimmed) > 100 {
		trimmed = string([]rune(trimmed)[:100])
	}
	return trimmed, nil
}

func ExternalGlossaryLibraryList(actorID string, opts ExternalGlossaryLibraryListOptions) ([]*model.ExternalGlossaryLibraryModel, int64, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return nil, 0, err
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 50
	}
	if opts.PageSize > 5000 {
		opts.PageSize = 5000
	}
	query := model.GetDB().Model(&model.ExternalGlossaryLibraryModel{})
	if !opts.IncludeDisabled {
		query = query.Where("is_enabled = ?", true)
	}
	if trimmed := strings.TrimSpace(opts.Query); trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*model.ExternalGlossaryLibraryModel{}, 0, nil
	}
	var items []*model.ExternalGlossaryLibraryModel
	if err := query.Order("sort_order DESC, updated_at DESC").Offset((opts.Page - 1) * opts.PageSize).Limit(opts.PageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func ExternalGlossaryLibraryCreate(actorID string, input ExternalGlossaryLibraryInput) (*model.ExternalGlossaryLibraryModel, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return nil, err
	}
	if err := normalizeExternalGlossaryLibraryInput(&input); err != nil {
		return nil, err
	}
	sortOrder := 0
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	} else {
		model.GetDB().Model(&model.ExternalGlossaryLibraryModel{}).
			Select("COALESCE(MAX(sort_order), 0) + 1").Scan(&sortOrder)
	}
	item := &model.ExternalGlossaryLibraryModel{
		Name:        input.Name,
		Description: input.Description,
		IsEnabled:   input.Enabled == nil || *input.Enabled,
		SortOrder:   sortOrder,
		CreatedBy:   actorID,
		UpdatedBy:   actorID,
	}
	item.Normalize()
	item.Init()
	if err := model.GetDB().Model(&model.ExternalGlossaryLibraryModel{}).Create(map[string]any{
		"id":          item.ID,
		"created_at":  item.CreatedAt,
		"updated_at":  item.UpdatedAt,
		"deleted_at":  item.DeletedAt,
		"name":        item.Name,
		"description": item.Description,
		"is_enabled":  item.IsEnabled,
		"sort_order":  item.SortOrder,
		"created_by":  item.CreatedBy,
		"updated_by":  item.UpdatedBy,
	}).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func ExternalGlossaryLibraryUpdate(libraryID, actorID string, input ExternalGlossaryLibraryInput) (*model.ExternalGlossaryLibraryModel, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return nil, err
	}
	if err := normalizeExternalGlossaryLibraryInput(&input); err != nil {
		return nil, err
	}
	db := model.GetDB()
	var record model.ExternalGlossaryLibraryModel
	if err := db.Where("id = ?", strings.TrimSpace(libraryID)).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExternalGlossaryNotFound
		}
		return nil, err
	}
	updates := map[string]any{
		"name":        input.Name,
		"description": input.Description,
		"updated_by":  actorID,
	}
	if input.Enabled != nil {
		updates["is_enabled"] = *input.Enabled
	}
	if input.SortOrder != nil {
		updates["sort_order"] = *input.SortOrder
	}
	if err := db.Model(&record).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := db.Where("id = ?", record.ID).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func ExternalGlossaryLibraryDelete(libraryID, actorID string) error {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return err
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return ErrExternalGlossaryNotFound
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("library_id = ?", libraryID).Delete(&model.ExternalGlossaryTermModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("library_id = ?", libraryID).Delete(&model.ExternalGlossaryCategoryModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("library_id = ?", libraryID).Delete(&model.WorldExternalGlossaryBindingModel{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ?", libraryID).Delete(&model.ExternalGlossaryLibraryModel{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrExternalGlossaryNotFound
		}
		return nil
	})
}

func ExternalGlossaryLibraryBulkDelete(actorID string, ids []string) (int64, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return 0, err
	}
	var affected int64
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if err := ExternalGlossaryLibraryDelete(id, actorID); err != nil {
			if err == ErrExternalGlossaryNotFound {
				continue
			}
			return affected, err
		}
		affected++
	}
	return affected, nil
}

func ExternalGlossaryLibraryReorder(actorID string, items []WorldKeywordReorderItem) (int, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	updated := 0
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			res := tx.Model(&model.ExternalGlossaryLibraryModel{}).
				Where("id = ?", id).
				Updates(map[string]any{
					"sort_order": item.SortOrder,
					"updated_by": actorID,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				updated++
			}
		}
		return nil
	})
	return updated, err
}

func upsertExternalGlossaryCategory(tx *gorm.DB, libraryID, categoryName, actorID string) error {
	categoryName = strings.TrimSpace(categoryName)
	if categoryName == "" {
		return nil
	}
	name, err := normalizeExternalGlossaryCategoryName(categoryName)
	if err != nil {
		return err
	}
	var existing model.ExternalGlossaryCategoryModel
	if err := tx.Where("library_id = ? AND name = ?", libraryID, name).Limit(1).First(&existing).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		record := &model.ExternalGlossaryCategoryModel{
			LibraryID: libraryID,
			Name:      name,
			CreatedBy: actorID,
			UpdatedBy: actorID,
		}
		record.Normalize()
		return tx.Create(record).Error
	}
	if existing.UpdatedBy == actorID {
		return nil
	}
	return tx.Model(&existing).Update("updated_by", actorID).Error
}

func cleanupExternalGlossaryCategoryIfUnused(tx *gorm.DB, libraryID, categoryName string) error {
	categoryName = strings.TrimSpace(categoryName)
	if categoryName == "" {
		return nil
	}
	var count int64
	if err := tx.Model(&model.ExternalGlossaryTermModel{}).
		Where("library_id = ? AND category = ?", libraryID, categoryName).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return tx.Where("library_id = ? AND name = ?", libraryID, categoryName).
		Delete(&model.ExternalGlossaryCategoryModel{}).Error
}

func ExternalGlossaryTermList(libraryID, actorID string, opts WorldKeywordListOptions) ([]*model.ExternalGlossaryTermModel, int64, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return nil, 0, err
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 50
	}
	if opts.PageSize > 5000 {
		opts.PageSize = 5000
	}
	query := model.GetDB().Model(&model.ExternalGlossaryTermModel{}).Where("library_id = ?", strings.TrimSpace(libraryID))
	if !opts.IncludeDisabled {
		query = query.Where("is_enabled = ?", true)
	}
	if trimmed := strings.TrimSpace(opts.Query); trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where("keyword LIKE ? OR description LIKE ?", like, like)
	}
	if cat := strings.TrimSpace(opts.Category); cat != "" {
		query = query.Where("category = ?", cat)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*model.ExternalGlossaryTermModel{}, 0, nil
	}
	var items []*model.ExternalGlossaryTermModel
	if err := query.Order("sort_order DESC, updated_at DESC").Offset((opts.Page - 1) * opts.PageSize).Limit(opts.PageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func ExternalGlossaryTermCreate(libraryID, actorID string, input WorldKeywordInput) (*model.ExternalGlossaryTermModel, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return nil, err
	}
	if err := normalizeWorldKeywordInput(&input); err != nil {
		return nil, err
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, ErrExternalGlossaryNotFound
	}
	sortOrder := 0
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	} else {
		model.GetDB().Model(&model.ExternalGlossaryTermModel{}).
			Where("library_id = ?", libraryID).
			Select("COALESCE(MAX(sort_order), 0)").Scan(&sortOrder)
		sortOrder++
	}
	item := &model.ExternalGlossaryTermModel{
		LibraryID:         libraryID,
		Keyword:           input.Keyword,
		Category:          input.Category,
		Aliases:           model.JSONList[string](input.Aliases),
		MatchMode:         model.WorldKeywordMatchMode(input.MatchMode),
		Description:       strings.TrimSpace(input.Description),
		DescriptionFormat: model.WorldKeywordDescFormat(input.DescriptionFormat),
		Display:           model.WorldKeywordDisplayStyle(input.Display),
		SortOrder:         sortOrder,
		IsEnabled:         input.Enabled == nil || *input.Enabled,
		CreatedBy:         actorID,
		UpdatedBy:         actorID,
	}
	item.Normalize()
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := upsertExternalGlossaryCategory(tx, libraryID, input.Category, actorID); err != nil {
			return err
		}
		item.Init()
		return tx.Model(&model.ExternalGlossaryTermModel{}).Create(map[string]any{
			"id":                 item.ID,
			"created_at":         item.CreatedAt,
			"updated_at":         item.UpdatedAt,
			"deleted_at":         item.DeletedAt,
			"library_id":         item.LibraryID,
			"keyword":            item.Keyword,
			"category":           item.Category,
			"aliases":            item.Aliases,
			"match_mode":         item.MatchMode,
			"description":        item.Description,
			"description_format": item.DescriptionFormat,
			"display":            item.Display,
			"sort_order":         item.SortOrder,
			"is_enabled":         item.IsEnabled,
			"created_by":         item.CreatedBy,
			"updated_by":         item.UpdatedBy,
		}).Error
	}); err != nil {
		return nil, err
	}
	return item, nil
}

func ExternalGlossaryTermUpdate(libraryID, keywordID, actorID string, input WorldKeywordInput) (*model.ExternalGlossaryTermModel, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return nil, err
	}
	if err := normalizeWorldKeywordInput(&input); err != nil {
		return nil, err
	}
	var record model.ExternalGlossaryTermModel
	db := model.GetDB()
	if err := db.Where("id = ? AND library_id = ?", strings.TrimSpace(keywordID), strings.TrimSpace(libraryID)).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExternalGlossaryNotFound
		}
		return nil, err
	}
	previousCategory := strings.TrimSpace(record.Category)
	updates := map[string]any{
		"keyword":            input.Keyword,
		"category":           input.Category,
		"aliases":            model.JSONList[string](input.Aliases),
		"match_mode":         model.WorldKeywordMatchMode(input.MatchMode),
		"description":        strings.TrimSpace(input.Description),
		"description_format": model.WorldKeywordDescFormat(input.DescriptionFormat),
		"display":            model.WorldKeywordDisplayStyle(input.Display),
		"updated_by":         actorID,
	}
	if input.Enabled != nil {
		updates["is_enabled"] = *input.Enabled
	}
	if input.SortOrder != nil {
		updates["sort_order"] = *input.SortOrder
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := upsertExternalGlossaryCategory(tx, libraryID, input.Category, actorID); err != nil {
			return err
		}
		if err := tx.Model(&record).Updates(updates).Error; err != nil {
			return err
		}
		if previousCategory != "" && previousCategory != strings.TrimSpace(input.Category) {
			if err := cleanupExternalGlossaryCategoryIfUnused(tx, libraryID, previousCategory); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := db.Where("id = ?", record.ID).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func ExternalGlossaryTermDelete(libraryID, keywordID, actorID string) error {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return err
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		var record model.ExternalGlossaryTermModel
		if err := tx.Where("id = ? AND library_id = ?", strings.TrimSpace(keywordID), strings.TrimSpace(libraryID)).First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrExternalGlossaryNotFound
			}
			return err
		}
		if err := tx.Where("id = ? AND library_id = ?", record.ID, record.LibraryID).Delete(&model.ExternalGlossaryTermModel{}).Error; err != nil {
			return err
		}
		return cleanupExternalGlossaryCategoryIfUnused(tx, record.LibraryID, record.Category)
	})
}

func ExternalGlossaryTermBulkDelete(libraryID string, ids []string, actorID string) (int64, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return 0, err
	}
	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return 0, nil
	}
	var affected int64
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var records []model.ExternalGlossaryTermModel
		if err := tx.Where("library_id = ? AND id IN ?", strings.TrimSpace(libraryID), cleaned).Find(&records).Error; err != nil {
			return err
		}
		if len(records) == 0 {
			return nil
		}
		categorySet := map[string]struct{}{}
		for _, record := range records {
			if cat := strings.TrimSpace(record.Category); cat != "" {
				categorySet[cat] = struct{}{}
			}
		}
		res := tx.Where("library_id = ? AND id IN ?", strings.TrimSpace(libraryID), cleaned).Delete(&model.ExternalGlossaryTermModel{})
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		for category := range categorySet {
			if err := cleanupExternalGlossaryCategoryIfUnused(tx, strings.TrimSpace(libraryID), category); err != nil {
				return err
			}
		}
		return nil
	})
	return affected, err
}

func ExternalGlossaryTermImport(libraryID, actorID string, entries []WorldKeywordInput, replace bool) (*WorldKeywordImportStats, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return nil, err
	}
	stats := &WorldKeywordImportStats{}
	db := model.GetDB()
	for _, entry := range entries {
		item := entry
		if err := normalizeWorldKeywordInput(&item); err != nil {
			stats.Skipped++
			continue
		}
		var existing model.ExternalGlossaryTermModel
		result := db.Select("id").Where("library_id = ? AND keyword = ?", strings.TrimSpace(libraryID), item.Keyword).Limit(1).Find(&existing)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 || strings.TrimSpace(existing.ID) == "" {
			if _, createErr := ExternalGlossaryTermCreate(libraryID, actorID, item); createErr != nil {
				stats.Skipped++
				continue
			}
			stats.Created++
			continue
		}
		if !replace {
			stats.Skipped++
			continue
		}
		if _, err := ExternalGlossaryTermUpdate(libraryID, existing.ID, actorID, item); err != nil {
			stats.Skipped++
			continue
		}
		stats.Updated++
	}
	return stats, nil
}

func ExternalGlossaryTermExport(libraryID, actorID, category string) ([]*model.ExternalGlossaryTermModel, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return nil, err
	}
	query := model.GetDB().Where("library_id = ?", strings.TrimSpace(libraryID))
	if cat := strings.TrimSpace(category); cat != "" {
		query = query.Where("category = ?", cat)
	}
	var items []*model.ExternalGlossaryTermModel
	if err := query.Order("category ASC, keyword ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func ExternalGlossaryCategoryList(libraryID, actorID string) ([]string, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return nil, err
	}
	db := model.GetDB()
	var termCategories []string
	if err := db.Model(&model.ExternalGlossaryTermModel{}).
		Where("library_id = ? AND category != ''", strings.TrimSpace(libraryID)).
		Distinct("category").
		Order("category ASC").
		Pluck("category", &termCategories).Error; err != nil {
		return nil, err
	}
	var managedCategories []string
	if err := db.Model(&model.ExternalGlossaryCategoryModel{}).
		Where("library_id = ?", strings.TrimSpace(libraryID)).
		Distinct("name").
		Order("name ASC").
		Pluck("name", &managedCategories).Error; err != nil {
		return nil, err
	}
	mergedSet := make(map[string]struct{}, len(termCategories)+len(managedCategories))
	for _, category := range termCategories {
		if trimmed := strings.TrimSpace(category); trimmed != "" {
			mergedSet[trimmed] = struct{}{}
		}
	}
	for _, category := range managedCategories {
		if trimmed := strings.TrimSpace(category); trimmed != "" {
			mergedSet[trimmed] = struct{}{}
		}
	}
	merged := make([]string, 0, len(mergedSet))
	for category := range mergedSet {
		merged = append(merged, category)
	}
	sort.Strings(merged)
	return merged, nil
}

func ExternalGlossaryCategoryCreate(libraryID, actorID, categoryName string) (string, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return "", err
	}
	name, err := normalizeExternalGlossaryCategoryName(categoryName)
	if err != nil {
		return "", err
	}
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		return upsertExternalGlossaryCategory(tx, strings.TrimSpace(libraryID), name, actorID)
	}); err != nil {
		return "", err
	}
	return name, nil
}

func ExternalGlossaryCategoryRename(libraryID, actorID, oldName, newName string) (int64, string, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return 0, "", err
	}
	from, err := normalizeExternalGlossaryCategoryName(oldName)
	if err != nil {
		return 0, "", err
	}
	to, err := normalizeExternalGlossaryCategoryName(newName)
	if err != nil {
		return 0, "", err
	}
	if from == to {
		return 0, to, nil
	}
	var updated int64
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := upsertExternalGlossaryCategory(tx, strings.TrimSpace(libraryID), to, actorID); err != nil {
			return err
		}
		res := tx.Model(&model.ExternalGlossaryTermModel{}).
			Where("library_id = ? AND category = ?", strings.TrimSpace(libraryID), from).
			Updates(map[string]any{
				"category":   to,
				"updated_by": actorID,
			})
		if res.Error != nil {
			return res.Error
		}
		updated = res.RowsAffected
		return tx.Where("library_id = ? AND name = ?", strings.TrimSpace(libraryID), from).
			Delete(&model.ExternalGlossaryCategoryModel{}).Error
	})
	return updated, to, err
}

func ExternalGlossaryCategoryDelete(libraryID, actorID, categoryName string) (int64, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return 0, err
	}
	name, err := normalizeExternalGlossaryCategoryName(categoryName)
	if err != nil {
		return 0, err
	}
	var updated int64
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.ExternalGlossaryTermModel{}).
			Where("library_id = ? AND category = ?", strings.TrimSpace(libraryID), name).
			Updates(map[string]any{
				"category":   "",
				"updated_by": actorID,
			})
		if res.Error != nil {
			return res.Error
		}
		updated = res.RowsAffected
		return tx.Where("library_id = ? AND name = ?", strings.TrimSpace(libraryID), name).
			Delete(&model.ExternalGlossaryCategoryModel{}).Error
	})
	return updated, err
}

func ExternalGlossaryTermReorder(libraryID, actorID string, items []WorldKeywordReorderItem) (int, error) {
	if err := ensureExternalGlossaryAdmin(actorID); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	updated := 0
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			res := tx.Model(&model.ExternalGlossaryTermModel{}).
				Where("id = ? AND library_id = ?", id, strings.TrimSpace(libraryID)).
				Updates(map[string]any{
					"sort_order": item.SortOrder,
					"updated_by": actorID,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				updated++
			}
		}
		return nil
	})
	return updated, err
}
