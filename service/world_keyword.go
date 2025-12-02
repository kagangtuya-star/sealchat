package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"sealchat/model"
)

var (
	ErrWorldKeywordNotFound  = errors.New("关键词不存在")
	ErrWorldKeywordConflict  = errors.New("关键词已存在")
	ErrWorldKeywordInvalid   = errors.New("关键词或描述不合法")
	ErrWorldKeywordForbidden = errors.New("无权限修改关键词")
)

const (
	worldKeywordMaxLength      = 32
	worldKeywordDescriptionMax = 200
	worldKeywordDescriptionMin = 1
	worldKeywordMinLength      = 1
)

type WorldKeyword struct {
	ID            string    `json:"id"`
	WorldID       string    `json:"worldId"`
	Keyword       string    `json:"keyword"`
	Description   string    `json:"description"`
	CreatedBy     string    `json:"createdBy"`
	UpdatedBy     string    `json:"updatedBy"`
	CreatedByName string    `json:"createdByName"`
	UpdatedByName string    `json:"updatedByName"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type WorldKeywordCreateParams struct {
	Keyword     string `json:"keyword"`
	Description string `json:"description"`
}

type WorldKeywordUpdateParams struct {
	Keyword     *string `json:"keyword"`
	Description *string `json:"description"`
}

type WorldKeywordExportPayload struct {
	WorldID    string                    `json:"worldId"`
	ExportedAt time.Time                 `json:"exportedAt"`
	Keywords   []*WorldKeywordExportItem `json:"keywords"`
}

type WorldKeywordExportItem struct {
	Keyword     string    `json:"keyword"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updatedAt"`
	UpdatedBy   string    `json:"updatedBy"`
}

type keywordEntry struct {
	Keyword     string `json:"keyword"`
	Description string `json:"description"`
}

func serializeWorldKeywords(rows []*model.WorldKeywordModel, nameMap map[string]string) []*WorldKeyword {
	items := make([]*WorldKeyword, 0, len(rows))
	for _, item := range rows {
		if item == nil {
			continue
		}
		items = append(items, &WorldKeyword{
			ID:            item.ID,
			WorldID:       item.WorldID,
			Keyword:       item.Keyword,
			Description:   item.Description,
			CreatedBy:     item.CreatedBy,
			UpdatedBy:     item.UpdatedBy,
			CreatedByName: nameMap[item.CreatedBy],
			UpdatedByName: nameMap[item.UpdatedBy],
			CreatedAt:     item.CreatedAt,
			UpdatedAt:     item.UpdatedAt,
		})
	}
	return items
}

func buildKeywordUserNameMap(rows []*model.WorldKeywordModel) map[string]string {
	result := map[string]string{}
	idSet := map[string]struct{}{}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if id := strings.TrimSpace(row.CreatedBy); id != "" {
			idSet[id] = struct{}{}
		}
		if id := strings.TrimSpace(row.UpdatedBy); id != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return result
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	var users []*model.UserModel
	if err := model.GetDB().Where("id IN ?", ids).Find(&users).Error; err != nil {
		return result
	}
	for _, user := range users {
		if user == nil {
			continue
		}
		name := strings.TrimSpace(user.Nickname)
		if name == "" {
			name = strings.TrimSpace(user.Username)
		}
		if name == "" {
			name = user.ID
		}
		result[user.ID] = name
	}
	return result
}

func ListWorldKeywords(worldID string, keyword string) ([]*WorldKeyword, error) {
	db := model.GetDB()
	var rows []*model.WorldKeywordModel
	query := db.Where("world_id = ?", worldID)
	if trimmed := strings.TrimSpace(keyword); trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where("keyword LIKE ? OR description LIKE ?", like, like)
	}
	if err := query.
		Order("keyword asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	nameMap := buildKeywordUserNameMap(rows)
	return serializeWorldKeywords(rows, nameMap), nil
}

func GetWorldKeyword(worldID, keywordID string) (*WorldKeyword, error) {
	row := &model.WorldKeywordModel{}
	if err := model.GetDB().
		Where("world_id = ? AND id = ?", worldID, keywordID).
		Limit(1).
		Find(row).Error; err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, ErrWorldKeywordNotFound
	}
	nameMap := buildKeywordUserNameMap([]*model.WorldKeywordModel{row})
	items := serializeWorldKeywords([]*model.WorldKeywordModel{row}, nameMap)
	if len(items) == 0 {
		return nil, ErrWorldKeywordNotFound
	}
	return items[0], nil
}

func CreateWorldKeyword(worldID, actorID string, params WorldKeywordCreateParams) (*WorldKeyword, error) {
	if err := ensureWorldKeywordManage(worldID, actorID); err != nil {
		return nil, err
	}
	keyword, err := normalizeWorldKeyword(params.Keyword)
	if err != nil {
		return nil, err
	}
	description, err := normalizeWorldKeywordDescription(params.Description)
	if err != nil {
		return nil, err
	}
	db := model.GetDB()
	existing := &model.WorldKeywordModel{}
	if err := db.
		Where("world_id = ? AND keyword_normalized = ?", worldID, strings.ToLower(keyword)).
		Limit(1).
		Find(existing).Error; err != nil {
		return nil, err
	}
	if existing.ID != "" {
		return nil, ErrWorldKeywordConflict
	}
	item := &model.WorldKeywordModel{
		WorldID:     worldID,
		Keyword:     keyword,
		Description: description,
		CreatedBy:   actorID,
		UpdatedBy:   actorID,
	}
	if err := db.Create(item).Error; err != nil {
		return nil, err
	}
	nameMap := buildKeywordUserNameMap([]*model.WorldKeywordModel{item})
	items := serializeWorldKeywords([]*model.WorldKeywordModel{item}, nameMap)
	if len(items) == 0 {
		return nil, fmt.Errorf("创建关键词失败")
	}
	return items[0], nil
}

func UpdateWorldKeyword(worldID, keywordID, actorID string, params WorldKeywordUpdateParams) (*WorldKeyword, error) {
	if err := ensureWorldKeywordManage(worldID, actorID); err != nil {
		return nil, err
	}
	db := model.GetDB()
	item := &model.WorldKeywordModel{}
	if err := db.Where("world_id = ? AND id = ?", worldID, keywordID).Limit(1).Find(item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, ErrWorldKeywordNotFound
	}
	updates := map[string]any{
		"updated_by": actorID,
	}
	if params.Keyword != nil {
		value, err := normalizeWorldKeyword(*params.Keyword)
		if err != nil {
			return nil, err
		}
		dup := &model.WorldKeywordModel{}
		if err := db.Where("world_id = ? AND keyword_normalized = ? AND id <> ?", worldID, strings.ToLower(value), keywordID).
			Limit(1).
			Find(dup).Error; err != nil {
			return nil, err
		}
		if dup.ID != "" {
			return nil, ErrWorldKeywordConflict
		}
		updates["keyword"] = value
		updates["keyword_normalized"] = strings.ToLower(value)
	}
	if params.Description != nil {
		value, err := normalizeWorldKeywordDescription(*params.Description)
		if err != nil {
			return nil, err
		}
		updates["description"] = value
	}
	if len(updates) == 1 {
		nameMap := buildKeywordUserNameMap([]*model.WorldKeywordModel{item})
		items := serializeWorldKeywords([]*model.WorldKeywordModel{item}, nameMap)
		if len(items) == 0 {
			return nil, ErrWorldKeywordNotFound
		}
		return items[0], nil
	}
	if err := db.Model(item).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := db.Where("id = ?", item.ID).Limit(1).Find(item).Error; err != nil {
		return nil, err
	}
	nameMap := buildKeywordUserNameMap([]*model.WorldKeywordModel{item})
	items := serializeWorldKeywords([]*model.WorldKeywordModel{item}, nameMap)
	if len(items) == 0 {
		return nil, ErrWorldKeywordNotFound
	}
	return items[0], nil
}

func DeleteWorldKeyword(worldID, keywordID, actorID string) error {
	if err := ensureWorldKeywordManage(worldID, actorID); err != nil {
		return err
	}
	res := model.GetDB().Where("world_id = ? AND id = ?", worldID, keywordID).Delete(&model.WorldKeywordModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrWorldKeywordNotFound
	}
	return nil
}

func ExportWorldKeywords(worldID string) (*WorldKeywordExportPayload, error) {
	items, err := ListWorldKeywords(worldID, "")
	if err != nil {
		return nil, err
	}
	exportItems := make([]*WorldKeywordExportItem, 0, len(items))
	for _, item := range items {
		exportItems = append(exportItems, &WorldKeywordExportItem{
			Keyword:     item.Keyword,
			Description: item.Description,
			UpdatedAt:   item.UpdatedAt,
			UpdatedBy:   item.UpdatedBy,
		})
	}
	return &WorldKeywordExportPayload{
		WorldID:    worldID,
		ExportedAt: time.Now(),
		Keywords:   exportItems,
	}, nil
}

func ExportWorldKeywordsJSON(worldID string) ([]byte, error) {
	payload, err := ExportWorldKeywords(worldID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(payload)
}

type WorldKeywordImportStats struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Total   int `json:"total"`
}

func ImportWorldKeywordsFromContent(worldID, actorID, content string) (*WorldKeywordImportStats, error) {
	if err := ensureWorldKeywordManage(worldID, actorID); err != nil {
		return nil, err
	}
	entries, err := parseKeywordEntries(content)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("未解析到有效关键词")
	}
	stats := &WorldKeywordImportStats{Total: len(entries)}
	dedup := map[string]WorldKeywordCreateParams{}
	for _, entry := range entries {
		keyword := strings.TrimSpace(entry.Keyword)
		description := strings.TrimSpace(entry.Description)
		if keyword == "" || description == "" {
			stats.Skipped++
			continue
		}
		dedup[strings.ToLower(keyword)] = WorldKeywordCreateParams{
			Keyword:     keyword,
			Description: description,
		}
	}
	for _, entry := range dedup {
		created, err := upsertWorldKeyword(worldID, actorID, entry)
		if err != nil {
			stats.Skipped++
			continue
		}
		if created {
			stats.Created++
		} else {
			stats.Updated++
		}
	}
	return stats, nil
}

func upsertWorldKeyword(worldID, actorID string, params WorldKeywordCreateParams) (bool, error) {
	db := model.GetDB()
	keyword, err := normalizeWorldKeyword(params.Keyword)
	if err != nil {
		return false, err
	}
	description, err := normalizeWorldKeywordDescription(params.Description)
	if err != nil {
		return false, err
	}
	var existing model.WorldKeywordModel
	if err := db.Where("world_id = ? AND keyword_normalized = ?", worldID, strings.ToLower(keyword)).
		Limit(1).
		Find(&existing).Error; err != nil {
		return false, err
	}
	if existing.ID == "" {
		item := &model.WorldKeywordModel{
			WorldID:     worldID,
			Keyword:     keyword,
			Description: description,
			CreatedBy:   actorID,
			UpdatedBy:   actorID,
		}
		if err := db.Create(item).Error; err != nil {
			return false, err
		}
		return true, nil
	}
	updates := map[string]any{
		"keyword":            keyword,
		"keyword_normalized": strings.ToLower(keyword),
		"description":        description,
		"updated_by":         actorID,
		"updated_at":         time.Now(),
	}
	if err := db.Model(&existing).Updates(updates).Error; err != nil {
		return false, err
	}
	return false, nil
}

func parseKeywordEntries(raw string) ([]WorldKeywordCreateParams, error) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return nil, errors.New("导入内容不能为空")
	}
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		if entries, err := parseKeywordJSON(content); err == nil && len(entries) > 0 {
			return entries, nil
		}
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	results := make([]WorldKeywordCreateParams, 0, len(lines))
	for _, line := range lines {
		keyword, description, ok := parseKeywordLine(line)
		if !ok {
			continue
		}
		results = append(results, WorldKeywordCreateParams{
			Keyword:     keyword,
			Description: description,
		})
	}
	return results, nil
}

func parseKeywordJSON(content string) ([]WorldKeywordCreateParams, error) {
	var arr []keywordEntry
	if err := json.Unmarshal([]byte(content), &arr); err == nil && len(arr) > 0 {
		return convertEntriesFromEntries(arr), nil
	}
	var payload struct {
		Keywords []keywordEntry `json:"keywords"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err == nil && len(payload.Keywords) > 0 {
		return convertEntriesFromEntries(payload.Keywords), nil
	}
	return nil, errors.New("JSON 解析失败")
}

func convertEntriesFromEntries(entries []keywordEntry) []WorldKeywordCreateParams {
	results := make([]WorldKeywordCreateParams, 0, len(entries))
	for _, item := range entries {
		results = append(results, WorldKeywordCreateParams{
			Keyword:     item.Keyword,
			Description: item.Description,
		})
	}
	return results
}

func parseKeywordLine(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	if strings.Contains(trimmed, "|") {
		parts := strings.SplitN(trimmed, "|", 2)
		if len(parts) < 2 {
			return "", "", false
		}
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
	}
	if strings.Contains(trimmed, ",") {
		parts := strings.SplitN(trimmed, ",", 2)
		if len(parts) < 2 {
			return "", "", false
		}
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return "", "", false
	}
	return fields[0], strings.TrimSpace(strings.Join(fields[1:], " ")), true
}

func ensureWorldKeywordManage(worldID, actorID string) error {
	if CanManageWorldKeywords(worldID, actorID) {
		return nil
	}
	return ErrWorldKeywordForbidden
}

func ensureWorldKeywordReadable(worldID, actorID string) error {
	if IsWorldMember(worldID, actorID) {
		return nil
	}
	return ErrWorldPermission
}

func normalizeWorldKeyword(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	length := utf8.RuneCountInString(value)
	if length < worldKeywordMinLength {
		return "", ErrWorldKeywordInvalid
	}
	if length > worldKeywordMaxLength {
		return "", ErrWorldKeywordInvalid
	}
	return value, nil
}

func normalizeWorldKeywordDescription(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	length := utf8.RuneCountInString(value)
	if length < worldKeywordDescriptionMin {
		return "", ErrWorldKeywordInvalid
	}
	if length > worldKeywordDescriptionMax {
		return "", ErrWorldKeywordInvalid
	}
	return value, nil
}

func CanManageWorldKeywords(worldID, userID string) bool {
	if strings.TrimSpace(worldID) == "" || strings.TrimSpace(userID) == "" {
		return false
	}
	if IsWorldAdmin(worldID, userID) {
		return true
	}
	return worldRoleEquals(worldID, userID, model.WorldRoleMember)
}

func EnsureWorldKeywordReadable(worldID, userID string) error {
	return ensureWorldKeywordReadable(worldID, userID)
}
