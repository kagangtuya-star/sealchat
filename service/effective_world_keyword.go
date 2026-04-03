package service

import (
	"sort"
	"strings"
	"time"

	"sealchat/model"
)

const (
	EffectiveWorldKeywordSourceWorld           = "world"
	EffectiveWorldKeywordSourceExternalLibrary = "external_library"
)

type EffectiveWorldKeywordListOptions struct {
	Query           string
	Category        string
	IncludeDisabled bool
}

type EffectiveWorldKeywordItem struct {
	ID                string   `json:"id"`
	WorldID           string   `json:"worldId"`
	Keyword           string   `json:"keyword"`
	Category          string   `json:"category"`
	Aliases           []string `json:"aliases"`
	MatchMode         string   `json:"matchMode"`
	Description       string   `json:"description"`
	DescriptionFormat string   `json:"descriptionFormat"`
	Display           string   `json:"display"`
	SortOrder         int      `json:"sortOrder"`
	IsEnabled         bool     `json:"isEnabled"`
	CreatedBy         string   `json:"createdBy,omitempty"`
	UpdatedBy         string   `json:"updatedBy,omitempty"`
	CreatedAt         string   `json:"createdAt,omitempty"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
	SourceType        string   `json:"sourceType"`
	SourceID          string   `json:"sourceId"`
	SourceName        string   `json:"sourceName"`
	CategoryPriority  int      `json:"categoryPriority"`
	SourceSortOrder   int      `json:"sourceSortOrder"`
	CanQuickEdit      bool     `json:"canQuickEdit"`
	categoryPriority  int
	priorityTier      int
	sourceSortOrder   int
	updatedAtUnixNano int64
}

func normalizeEffectiveWorldKeywordDedupeKey(keyword string) string {
	return strings.ToLower(strings.TrimSpace(keyword))
}

func buildEffectiveWorldKeywordItemFromWorld(item *model.WorldKeywordModel, categoryPriority int) *EffectiveWorldKeywordItem {
	if item == nil {
		return nil
	}
	return &EffectiveWorldKeywordItem{
		ID:                item.ID,
		WorldID:           item.WorldID,
		Keyword:           item.Keyword,
		Category:          item.Category,
		Aliases:           append([]string(nil), item.Aliases...),
		MatchMode:         string(item.MatchMode),
		Description:       item.Description,
		DescriptionFormat: string(item.DescriptionFormat),
		Display:           string(item.Display),
		SortOrder:         item.SortOrder,
		IsEnabled:         item.IsEnabled,
		CreatedBy:         item.CreatedBy,
		UpdatedBy:         item.UpdatedBy,
		CreatedAt:         item.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:         item.UpdatedAt.Format(time.RFC3339Nano),
		SourceType:        EffectiveWorldKeywordSourceWorld,
		SourceID:          item.WorldID,
		SourceName:        "当前世界",
		CategoryPriority:  categoryPriority,
		SourceSortOrder:   0,
		CanQuickEdit:      true,
		categoryPriority:  categoryPriority,
		priorityTier:      0,
		sourceSortOrder:   0,
		updatedAtUnixNano: item.UpdatedAt.UnixNano(),
	}
}

func buildEffectiveWorldKeywordItemFromExternal(worldID string, item *model.ExternalGlossaryTermModel, library *model.ExternalGlossaryLibraryModel, categoryPriority int) *EffectiveWorldKeywordItem {
	if item == nil || library == nil {
		return nil
	}
	return &EffectiveWorldKeywordItem{
		ID:                item.ID,
		WorldID:           worldID,
		Keyword:           item.Keyword,
		Category:          item.Category,
		Aliases:           append([]string(nil), item.Aliases...),
		MatchMode:         string(item.MatchMode),
		Description:       item.Description,
		DescriptionFormat: string(item.DescriptionFormat),
		Display:           string(item.Display),
		SortOrder:         item.SortOrder,
		IsEnabled:         item.IsEnabled,
		CreatedBy:         item.CreatedBy,
		UpdatedBy:         item.UpdatedBy,
		CreatedAt:         item.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:         item.UpdatedAt.Format(time.RFC3339Nano),
		SourceType:        EffectiveWorldKeywordSourceExternalLibrary,
		SourceID:          library.ID,
		SourceName:        library.Name,
		CategoryPriority:  categoryPriority,
		SourceSortOrder:   library.SortOrder,
		CanQuickEdit:      false,
		categoryPriority:  categoryPriority,
		priorityTier:      1,
		sourceSortOrder:   library.SortOrder,
		updatedAtUnixNano: item.UpdatedAt.UnixNano(),
	}
}

func filterEffectiveWorldKeywordItems(items []*EffectiveWorldKeywordItem, opts EffectiveWorldKeywordListOptions) []*EffectiveWorldKeywordItem {
	query := strings.TrimSpace(opts.Query)
	category := strings.TrimSpace(opts.Category)
	if query == "" && category == "" {
		return items
	}
	queryLower := strings.ToLower(query)
	filtered := make([]*EffectiveWorldKeywordItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		if category != "" && item.Category != category {
			continue
		}
		if queryLower == "" {
			filtered = append(filtered, item)
			continue
		}
		matched := strings.Contains(strings.ToLower(item.Keyword), queryLower) ||
			strings.Contains(strings.ToLower(item.Description), queryLower)
		if !matched {
			for _, alias := range item.Aliases {
				if strings.Contains(strings.ToLower(alias), queryLower) {
					matched = true
					break
				}
			}
		}
		if matched {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func sortEffectiveWorldKeywordItems(items []*EffectiveWorldKeywordItem) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if left.categoryPriority != right.categoryPriority {
			return left.categoryPriority > right.categoryPriority
		}
		if left.priorityTier != right.priorityTier {
			return left.priorityTier < right.priorityTier
		}
		if left.sourceSortOrder != right.sourceSortOrder {
			return left.sourceSortOrder > right.sourceSortOrder
		}
		if left.SortOrder != right.SortOrder {
			return left.SortOrder > right.SortOrder
		}
		if left.updatedAtUnixNano != right.updatedAtUnixNano {
			return left.updatedAtUnixNano > right.updatedAtUnixNano
		}
		return left.ID < right.ID
	})
}

func EffectiveWorldKeywordList(worldID, userID string, opts EffectiveWorldKeywordListOptions) ([]*EffectiveWorldKeywordItem, error) {
	if err := ensureWorldKeywordPermission(worldID, userID, false); err != nil {
		return nil, err
	}
	return buildEffectiveWorldKeywordList(worldID, opts)
}

func EffectiveWorldKeywordListPublic(worldID string, opts EffectiveWorldKeywordListOptions) ([]*EffectiveWorldKeywordItem, error) {
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
	return buildEffectiveWorldKeywordList(worldID, EffectiveWorldKeywordListOptions{
		Query:           opts.Query,
		Category:        opts.Category,
		IncludeDisabled: false,
	})
}

func buildEffectiveWorldKeywordList(worldID string, opts EffectiveWorldKeywordListOptions) ([]*EffectiveWorldKeywordItem, error) {
	db := model.GetDB()
	worldID = strings.TrimSpace(worldID)
	items := make([]*EffectiveWorldKeywordItem, 0, 64)

	worldCategoryPriorityMap, err := loadWorldKeywordCategoryPriorityMap(db, worldID)
	if err != nil {
		return nil, err
	}

	worldQuery := db.Model(&model.WorldKeywordModel{}).Where("world_id = ?", worldID)
	if !opts.IncludeDisabled {
		worldQuery = worldQuery.Where("is_enabled = ?", true)
	}
	var worldItems []*model.WorldKeywordModel
	if err := worldQuery.Order("sort_order DESC, updated_at DESC").Find(&worldItems).Error; err != nil {
		return nil, err
	}
	for _, item := range worldItems {
		view := buildEffectiveWorldKeywordItemFromWorld(item, worldCategoryPriorityMap[strings.TrimSpace(item.Category)])
		if view == nil {
			continue
		}
		items = append(items, view)
	}

	var bindings []model.WorldExternalGlossaryBindingModel
	if err := db.Where("world_id = ?", worldID).Order("sort_order DESC, updated_at DESC").Find(&bindings).Error; err != nil {
		return nil, err
	}
	if len(bindings) > 0 {
		libraryIDs := make([]string, 0, len(bindings))
		bindingOrder := map[string]int{}
		for _, binding := range bindings {
			libraryIDs = append(libraryIDs, binding.LibraryID)
			bindingOrder[binding.LibraryID] = binding.SortOrder
		}
		var libraries []*model.ExternalGlossaryLibraryModel
		if err := db.Where("id IN ? AND is_enabled = ?", libraryIDs, true).
			Find(&libraries).Error; err != nil {
			return nil, err
		}
		libraryMap := map[string]*model.ExternalGlossaryLibraryModel{}
		activeLibraryIDs := make([]string, 0, len(libraries))
		for _, library := range libraries {
			if library == nil {
				continue
			}
			if order, ok := bindingOrder[library.ID]; ok && order != 0 {
				library.SortOrder = order
			}
			libraryMap[library.ID] = library
			activeLibraryIDs = append(activeLibraryIDs, library.ID)
		}
		if len(activeLibraryIDs) > 0 {
			externalCategoryPriorityMap := make(map[string]map[string]int, len(activeLibraryIDs))
			for _, libraryID := range activeLibraryIDs {
				priorities, err := loadExternalGlossaryCategoryPriorityMap(db, libraryID)
				if err != nil {
					return nil, err
				}
				externalCategoryPriorityMap[libraryID] = priorities
			}
			termQuery := db.Model(&model.ExternalGlossaryTermModel{}).Where("library_id IN ?", activeLibraryIDs)
			if !opts.IncludeDisabled {
				termQuery = termQuery.Where("is_enabled = ?", true)
			}
			var externalItems []*model.ExternalGlossaryTermModel
			if err := termQuery.Order("sort_order DESC, updated_at DESC").Find(&externalItems).Error; err != nil {
				return nil, err
			}
			for _, item := range externalItems {
				library := libraryMap[item.LibraryID]
				categoryPriority := 0
				if priorities, ok := externalCategoryPriorityMap[item.LibraryID]; ok {
					categoryPriority = priorities[strings.TrimSpace(item.Category)]
				}
				view := buildEffectiveWorldKeywordItemFromExternal(worldID, item, library, categoryPriority)
				if view == nil {
					continue
				}
				items = append(items, view)
			}
		}
	}

	sortEffectiveWorldKeywordItems(items)
	deduped := make([]*EffectiveWorldKeywordItem, 0, len(items))
	seenByKeyword := map[string]struct{}{}
	for _, item := range items {
		if item == nil {
			continue
		}
		key := normalizeEffectiveWorldKeywordDedupeKey(item.Keyword)
		if key != "" {
			if _, exists := seenByKeyword[key]; exists {
				continue
			}
			seenByKeyword[key] = struct{}{}
		}
		deduped = append(deduped, item)
	}
	return filterEffectiveWorldKeywordItems(deduped, opts), nil
}
