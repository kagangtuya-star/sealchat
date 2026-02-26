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
	ErrWorldKeywordNotFound = errors.New("world keyword not found")
)

// WorldKeywordInput 用于创建或更新关键词。
type WorldKeywordInput struct {
	Keyword           string   `json:"keyword"`
	Category          string   `json:"category"`
	Aliases           []string `json:"aliases"`
	MatchMode         string   `json:"matchMode"`
	Description       string   `json:"description"`
	DescriptionFormat string   `json:"descriptionFormat"`
	Display           string   `json:"display"`
	SortOrder         *int     `json:"sortOrder"`
	Enabled           *bool    `json:"isEnabled"`
}

// WorldKeywordReorderItem 批量更新排序时的单项。
type WorldKeywordReorderItem struct {
	ID        string `json:"id"`
	SortOrder int    `json:"sortOrder"`
}

// WorldKeywordListOptions 查询参数。
type WorldKeywordListOptions struct {
	Page            int
	PageSize        int
	Query           string
	Category        string
	IncludeDisabled bool
}

// WorldKeywordImportStats 记录导入结果。
type WorldKeywordImportStats struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

func ensureWorldKeywordPermission(worldID, userID string, requireAdmin bool) error {
	if strings.TrimSpace(worldID) == "" || strings.TrimSpace(userID) == "" {
		return ErrWorldPermission
	}
	if pm.CanWithSystemRole(userID, pm.PermModAdmin) {
		return nil
	}
	if requireAdmin {
		if IsWorldAdmin(worldID, userID) {
			return nil
		}
		world, err := GetWorldByID(worldID)
		if err != nil || world == nil || !world.AllowMemberEditKeywords {
			return ErrWorldPermission
		}
		var member model.WorldMemberModel
		if err := model.GetDB().Where("world_id = ? AND user_id = ?", worldID, userID).Limit(1).Find(&member).Error; err != nil {
			return ErrWorldPermission
		}
		if member.ID == "" || member.Role != model.WorldRoleMember {
			return ErrWorldPermission
		}
		return nil
	}
	if !IsWorldMember(worldID, userID) {
		return ErrWorldPermission
	}
	return nil
}

func normalizeWorldKeywordInput(input *WorldKeywordInput) error {
	if input == nil {
		return ErrWorldKeywordNotFound
	}
	input.Keyword = strings.TrimSpace(input.Keyword)
	if utf8.RuneCountInString(input.Keyword) == 0 {
		return errors.New("关键词不能为空")
	}
	if utf8.RuneCountInString(input.Keyword) > 120 {
		input.Keyword = string([]rune(input.Keyword)[:120])
	}
	aliases := make([]string, 0, len(input.Aliases))
	seen := map[string]struct{}{}
	for _, raw := range input.Aliases {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || trimmed == input.Keyword {
			continue
		}
		lower := strings.ToLower(trimmed)
		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}
		aliases = append(aliases, trimmed)
	}
	input.Aliases = aliases
	switch strings.ToLower(strings.TrimSpace(input.MatchMode)) {
	case string(model.WorldKeywordMatchRegex):
		input.MatchMode = string(model.WorldKeywordMatchRegex)
	default:
		input.MatchMode = string(model.WorldKeywordMatchPlain)
	}
	switch strings.ToLower(strings.TrimSpace(input.Display)) {
	case string(model.WorldKeywordDisplayMinimal):
		input.Display = string(model.WorldKeywordDisplayMinimal)
	case string(model.WorldKeywordDisplayStandard):
		input.Display = string(model.WorldKeywordDisplayStandard)
	case string(model.WorldKeywordDisplayInherit):
		input.Display = string(model.WorldKeywordDisplayInherit)
	default:
		input.Display = string(model.WorldKeywordDisplayInherit)
	}
	switch strings.ToLower(strings.TrimSpace(input.DescriptionFormat)) {
	case string(model.WorldKeywordDescRich):
		input.DescriptionFormat = string(model.WorldKeywordDescRich)
	default:
		input.DescriptionFormat = string(model.WorldKeywordDescPlain)
	}
	input.Category = strings.TrimSpace(input.Category)
	if utf8.RuneCountInString(input.Category) > 100 {
		input.Category = string([]rune(input.Category)[:100])
	}
	return nil
}

func normalizeWorldKeywordCategoryName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("分类名称不能为空")
	}
	if utf8.RuneCountInString(trimmed) > 100 {
		trimmed = string([]rune(trimmed)[:100])
	}
	return trimmed, nil
}

func upsertWorldKeywordCategory(tx *gorm.DB, worldID, categoryName, actorID string) error {
	categoryName = strings.TrimSpace(categoryName)
	if categoryName == "" {
		return nil
	}
	name, err := normalizeWorldKeywordCategoryName(categoryName)
	if err != nil {
		return err
	}
	var existing model.WorldKeywordCategoryModel
	if err := tx.Where("world_id = ? AND name = ?", worldID, name).Limit(1).First(&existing).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		record := &model.WorldKeywordCategoryModel{
			WorldID:   worldID,
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

func cleanupWorldKeywordCategoryIfUnused(tx *gorm.DB, worldID, categoryName string) error {
	categoryName = strings.TrimSpace(categoryName)
	if categoryName == "" {
		return nil
	}
	var count int64
	if err := tx.Model(&model.WorldKeywordModel{}).
		Where("world_id = ? AND category = ?", worldID, categoryName).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return tx.Where("world_id = ? AND name = ?", worldID, categoryName).
		Delete(&model.WorldKeywordCategoryModel{}).Error
}

// WorldKeywordList 查询世界词条。
func WorldKeywordList(worldID, userID string, opts WorldKeywordListOptions) ([]*model.WorldKeywordModel, int64, error) {
	if err := ensureWorldKeywordPermission(worldID, userID, false); err != nil {
		return nil, 0, err
	}
	includeDisabled := opts.IncludeDisabled
	if includeDisabled && !pm.CanWithSystemRole(userID, pm.PermModAdmin) && !IsWorldAdmin(worldID, userID) {
		includeDisabled = false
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
	db := model.GetDB()
	query := db.Model(&model.WorldKeywordModel{}).Where("world_id = ?", worldID)
	if !includeDisabled {
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
		return []*model.WorldKeywordModel{}, 0, nil
	}
	var items []*model.WorldKeywordModel
	if err := query.Order("sort_order DESC, updated_at DESC").Offset((opts.Page - 1) * opts.PageSize).Limit(opts.PageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// WorldKeywordListPublic 查询公开世界词条（仅展示启用项）。
func WorldKeywordListPublic(worldID string, opts WorldKeywordListOptions) ([]*model.WorldKeywordModel, int64, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil, 0, ErrWorldNotFound
	}
	world, err := GetWorldByID(worldID)
	if err != nil {
		return nil, 0, err
	}
	if world == nil || strings.ToLower(strings.TrimSpace(world.Visibility)) != model.WorldVisibilityPublic {
		return nil, 0, ErrWorldPermission
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
	db := model.GetDB()
	query := db.Model(&model.WorldKeywordModel{}).
		Where("world_id = ? AND is_enabled = ?", worldID, true)
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
		return []*model.WorldKeywordModel{}, 0, nil
	}
	var items []*model.WorldKeywordModel
	if err := query.Order("sort_order DESC, updated_at DESC").Offset((opts.Page - 1) * opts.PageSize).Limit(opts.PageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// WorldKeywordCreate 新增词条。
func WorldKeywordCreate(worldID, actorID string, input WorldKeywordInput) (*model.WorldKeywordModel, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
		return nil, err
	}
	if err := normalizeWorldKeywordInput(&input); err != nil {
		return nil, err
	}
	sortOrder := 0
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	} else {
		var maxSort int
		model.GetDB().Model(&model.WorldKeywordModel{}).
			Where("world_id = ?", worldID).
			Select("COALESCE(MAX(sort_order), 0)").Scan(&maxSort)
		sortOrder = maxSort + 1
	}
	item := &model.WorldKeywordModel{
		WorldID:           worldID,
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
	db := model.GetDB()
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := upsertWorldKeywordCategory(tx, worldID, input.Category, actorID); err != nil {
			return err
		}
		return tx.Create(item).Error
	}); err != nil {
		return nil, err
	}
	return item, nil
}

// WorldKeywordUpdate 更新词条。
func WorldKeywordUpdate(worldID, keywordID, actorID string, input WorldKeywordInput) (*model.WorldKeywordModel, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
		return nil, err
	}
	if err := normalizeWorldKeywordInput(&input); err != nil {
		return nil, err
	}
	db := model.GetDB()
	var record model.WorldKeywordModel
	if err := db.Where("id = ? AND world_id = ?", keywordID, worldID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorldKeywordNotFound
		}
		return nil, err
	}
	previousCategory := strings.TrimSpace(record.Category)
	updates := map[string]interface{}{
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
		if err := upsertWorldKeywordCategory(tx, worldID, input.Category, actorID); err != nil {
			return err
		}
		if err := tx.Model(&record).Updates(updates).Error; err != nil {
			return err
		}
		if previousCategory != "" && previousCategory != strings.TrimSpace(input.Category) {
			if err := cleanupWorldKeywordCategoryIfUnused(tx, worldID, previousCategory); err != nil {
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

// WorldKeywordDelete 删除单条。
func WorldKeywordDelete(worldID, keywordID, actorID string) error {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
		return err
	}
	db := model.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		var record model.WorldKeywordModel
		if err := tx.Where("id = ? AND world_id = ?", keywordID, worldID).First(&record).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrWorldKeywordNotFound
			}
			return err
		}
		if err := tx.Where("id = ? AND world_id = ?", keywordID, worldID).Delete(&model.WorldKeywordModel{}).Error; err != nil {
			return err
		}
		return cleanupWorldKeywordCategoryIfUnused(tx, worldID, record.Category)
	})
}

// WorldKeywordBulkDelete 批量删除。
func WorldKeywordBulkDelete(worldID string, ids []string, actorID string) (int64, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
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
	db := model.GetDB()
	var affected int64
	err := db.Transaction(func(tx *gorm.DB) error {
		var records []model.WorldKeywordModel
		if err := tx.Where("world_id = ? AND id IN ?", worldID, cleaned).Find(&records).Error; err != nil {
			return err
		}
		if len(records) == 0 {
			affected = 0
			return nil
		}
		categorySet := make(map[string]struct{})
		for _, record := range records {
			if cat := strings.TrimSpace(record.Category); cat != "" {
				categorySet[cat] = struct{}{}
			}
		}
		res := tx.Where("world_id = ? AND id IN ?", worldID, cleaned).Delete(&model.WorldKeywordModel{})
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		for categoryName := range categorySet {
			if err := cleanupWorldKeywordCategoryIfUnused(tx, worldID, categoryName); err != nil {
				return err
			}
		}
		return nil
	})
	return affected, err
}

// WorldKeywordImport 批量导入词条。
func WorldKeywordImport(worldID, actorID string, entries []WorldKeywordInput, replace bool) (*WorldKeywordImportStats, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
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
		var existing model.WorldKeywordModel
		err := db.Where("world_id = ? AND keyword = ?", worldID, item.Keyword).First(&existing).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				_, createErr := WorldKeywordCreate(worldID, actorID, item)
				if createErr != nil {
					stats.Skipped++
					continue
				}
				stats.Created++
				continue
			}
			return nil, err
		}
		if !replace {
			stats.Skipped++
			continue
		}
		if _, err := WorldKeywordUpdate(worldID, existing.ID, actorID, item); err != nil {
			stats.Skipped++
			continue
		}
		stats.Updated++
	}
	return stats, nil
}

// WorldKeywordExport 导出。
func WorldKeywordExport(worldID, actorID string, category string) ([]*model.WorldKeywordModel, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
		return nil, err
	}
	db := model.GetDB()
	query := db.Where("world_id = ?", worldID)
	if cat := strings.TrimSpace(category); cat != "" {
		query = query.Where("category = ?", cat)
	}
	var items []*model.WorldKeywordModel
	if err := query.Order("category ASC, keyword ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// WorldKeywordListCategories 获取世界内所有分类列表。
func WorldKeywordListCategories(worldID, userID string) ([]string, error) {
	if err := ensureWorldKeywordPermission(worldID, userID, false); err != nil {
		return nil, err
	}
	db := model.GetDB()
	var keywordCategories []string
	if err := db.Model(&model.WorldKeywordModel{}).
		Where("world_id = ? AND category != ''", worldID).
		Distinct("category").
		Order("category ASC").
		Pluck("category", &keywordCategories).Error; err != nil {
		return nil, err
	}
	var managedCategories []string
	if err := db.Model(&model.WorldKeywordCategoryModel{}).
		Where("world_id = ?", worldID).
		Distinct("name").
		Order("name ASC").
		Pluck("name", &managedCategories).Error; err != nil {
		return nil, err
	}
	mergedSet := make(map[string]struct{}, len(keywordCategories)+len(managedCategories))
	for _, category := range keywordCategories {
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

// WorldKeywordCategoryCreate 创建分类。
func WorldKeywordCategoryCreate(worldID, actorID, categoryName string) (string, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
		return "", err
	}
	name, err := normalizeWorldKeywordCategoryName(categoryName)
	if err != nil {
		return "", err
	}
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		return upsertWorldKeywordCategory(tx, worldID, name, actorID)
	}); err != nil {
		return "", err
	}
	return name, nil
}

// WorldKeywordCategoryRename 批量重命名分类。
func WorldKeywordCategoryRename(worldID, actorID, oldName, newName string) (int64, string, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
		return 0, "", err
	}
	from, err := normalizeWorldKeywordCategoryName(oldName)
	if err != nil {
		return 0, "", err
	}
	to, err := normalizeWorldKeywordCategoryName(newName)
	if err != nil {
		return 0, "", err
	}
	if from == to {
		return 0, to, nil
	}
	var updated int64
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := upsertWorldKeywordCategory(tx, worldID, to, actorID); err != nil {
			return err
		}
		res := tx.Model(&model.WorldKeywordModel{}).
			Where("world_id = ? AND category = ?", worldID, from).
			Updates(map[string]interface{}{
				"category":   to,
				"updated_by": actorID,
			})
		if res.Error != nil {
			return res.Error
		}
		updated = res.RowsAffected
		if err := tx.Where("world_id = ? AND name = ?", worldID, from).
			Delete(&model.WorldKeywordCategoryModel{}).Error; err != nil {
			return err
		}
		return nil
	})
	return updated, to, err
}

// WorldKeywordCategoryDelete 删除分类，并将分类下术语设为未分类。
func WorldKeywordCategoryDelete(worldID, actorID, categoryName string) (int64, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
		return 0, err
	}
	name, err := normalizeWorldKeywordCategoryName(categoryName)
	if err != nil {
		return 0, err
	}
	var updated int64
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.WorldKeywordModel{}).
			Where("world_id = ? AND category = ?", worldID, name).
			Updates(map[string]interface{}{
				"category":   "",
				"updated_by": actorID,
			})
		if res.Error != nil {
			return res.Error
		}
		updated = res.RowsAffected
		return tx.Where("world_id = ? AND name = ?", worldID, name).
			Delete(&model.WorldKeywordCategoryModel{}).Error
	})
	return updated, err
}

// WorldKeywordListCategoriesPublic 获取公开世界启用词条的分类列表。
func WorldKeywordListCategoriesPublic(worldID string) ([]string, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil, ErrWorldNotFound
	}
	world, err := GetWorldByID(worldID)
	if err != nil {
		return nil, err
	}
	if world == nil || strings.ToLower(strings.TrimSpace(world.Visibility)) != model.WorldVisibilityPublic {
		return nil, ErrWorldPermission
	}
	var categories []string
	err = model.GetDB().Model(&model.WorldKeywordModel{}).
		Where("world_id = ? AND category != '' AND is_enabled = ?", worldID, true).
		Distinct("category").
		Order("category ASC").
		Pluck("category", &categories).Error
	if err != nil {
		return nil, err
	}
	return categories, nil
}

// WorldKeywordReorder 批量更新排序。
func WorldKeywordReorder(worldID, actorID string, items []WorldKeywordReorderItem) (int, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, true); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	if len(items) > 5000 {
		return 0, errors.New("批量更新数量不能超过5000")
	}
	cleaned := make([]WorldKeywordReorderItem, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, WorldKeywordReorderItem{ID: id, SortOrder: item.SortOrder})
	}
	if len(cleaned) == 0 {
		return 0, nil
	}
	db := model.GetDB()
	updated := 0
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, item := range cleaned {
			res := tx.Model(&model.WorldKeywordModel{}).
				Where("id = ? AND world_id = ?", item.ID, worldID).
				Updates(map[string]interface{}{
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
	}); err != nil {
		return 0, err
	}
	return updated, nil
}
