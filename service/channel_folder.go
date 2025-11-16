package service

import (
	"errors"
	"strings"

	"github.com/samber/lo"

	"sealchat/model"
)

type ChannelFolderInput struct {
	Name        string
	ParentID    string
	SortOrder   *int
	Description string
}

type ChannelFolderAssignPayload struct {
	FolderIDs       []string
	ChannelIDs      []string
	Mode            string // replace | append | remove
	IncludeChildren bool
}

type ChannelFolderListResult struct {
	Folders   []*model.ChannelFolderModel       `json:"folders"`
	Members   []*model.ChannelFolderMemberModel `json:"members"`
	Favorites []string                          `json:"favorites"`
}

func ChannelFolderList(userID string) (*ChannelFolderListResult, error) {
	folders, err := model.ChannelFolderListByUser(userID)
	if err != nil {
		return nil, err
	}
	members, err := model.ChannelFolderMemberListByUser(userID)
	if err != nil {
		return nil, err
	}
	favorites, err := model.ChannelFolderFavoriteIDs(userID)
	if err != nil {
		return nil, err
	}

	return &ChannelFolderListResult{
		Folders:   folders,
		Members:   members,
		Favorites: favorites,
	}, nil
}

func ChannelFolderCreate(userID string, input *ChannelFolderInput) (*model.ChannelFolderModel, error) {
	if input == nil {
		return nil, errors.New("参数错误")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("文件夹名称不能为空")
	}
	folder := &model.ChannelFolderModel{
		Name:        name,
		ParentID:    strings.TrimSpace(input.ParentID),
		Description: strings.TrimSpace(input.Description),
		UserID:      userID,
		CreatedBy:   userID,
		UpdatedBy:   userID,
	}
	if folder.ParentID != "" {
		if _, err := model.ChannelFolderEnsureOwnership(folder.ParentID, userID); err != nil {
			return nil, err
		}
	}
	if input.SortOrder != nil {
		folder.SortOrder = *input.SortOrder
	}
	if err := model.ChannelFolderCreate(folder); err != nil {
		return nil, err
	}
	return folder, nil
}

func ChannelFolderUpdate(userID string, folderID string, input *ChannelFolderInput) (*model.ChannelFolderModel, error) {
	folder, err := model.ChannelFolderEnsureOwnership(folderID, userID)
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	if name := strings.TrimSpace(input.Name); name != "" {
		values["name"] = name
	}
	if input.ParentID != "" {
		if _, err := model.ChannelFolderEnsureOwnership(strings.TrimSpace(input.ParentID), userID); err != nil {
			return nil, err
		}
		values["parent_id"] = strings.TrimSpace(input.ParentID)
	}
	if input.Description != "" {
		values["description"] = strings.TrimSpace(input.Description)
	}
	if input.SortOrder != nil {
		values["sort_order"] = *input.SortOrder
	}
	if len(values) == 0 {
		return folder, nil
	}
	values["updated_by"] = userID
	if err := model.ChannelFolderUpdate(folderID, values); err != nil {
		return nil, err
	}
	return model.ChannelFolderGetByID(folderID)
}

func ChannelFolderDelete(userID string, folderID string) error {
	if _, err := model.ChannelFolderEnsureOwnership(folderID, userID); err != nil {
		return err
	}
	if err := model.ChannelFolderDelete(folderID); err != nil {
		return err
	}
	_ = model.ChannelFolderMemberDeleteByFolderIDs(userID, []string{folderID})
	_ = model.ChannelFolderFavoriteDeleteByFolderIDs(userID, []string{folderID})
	return nil
}

// ChannelFolderAssign 将频道加入/移除文件夹
func ChannelFolderAssign(userID string, payload *ChannelFolderAssignPayload) error {
	if payload == nil {
		return errors.New("参数错误")
	}
	folderIDs := sanitizeIDs(payload.FolderIDs)
	channelIDs := sanitizeIDs(payload.ChannelIDs)
	if len(channelIDs) == 0 {
		return errors.New("请选择频道")
	}
	if len(folderIDs) > 0 {
		for _, id := range folderIDs {
			if _, err := model.ChannelFolderEnsureOwnership(id, userID); err != nil {
				return err
			}
		}
	}
	var err error
	channelIDs, err = expandChannelIDs(channelIDs, payload.IncludeChildren)
	if err != nil {
		return err
	}

	switch payload.Mode {
	case "replace":
		if len(folderIDs) == 0 {
			return errors.New("请选择目标文件夹")
		}
		if err := model.ChannelFolderMemberDeleteByChannelIDs(userID, channelIDs); err != nil {
			return err
		}
		return addMembers(userID, folderIDs, channelIDs)
	case "append":
		if len(folderIDs) == 0 {
			return errors.New("请选择目标文件夹")
		}
		return addMembers(userID, folderIDs, channelIDs)
	case "remove":
		if len(folderIDs) == 0 {
			return errors.New("请选择要移除的文件夹")
		}
		return model.ChannelFolderMemberDelete(userID, folderIDs, channelIDs)
	default:
		return errors.New("无效的操作类型")
	}
}

func addMembers(userID string, folderIDs, channelIDs []string) error {
	records := make([]*model.ChannelFolderMemberModel, 0, len(folderIDs)*len(channelIDs))
	for _, ch := range channelIDs {
		for idx, f := range folderIDs {
			records = append(records, &model.ChannelFolderMemberModel{
				FolderID:  f,
				ChannelID: ch,
				UserID:    userID,
				SortOrder: idx,
			})
		}
	}
	return model.ChannelFolderMemberBulkInsert(records)
}

func ChannelFolderToggleFavorite(userID string, folderID string, favored bool) ([]string, error) {
	if _, err := model.ChannelFolderEnsureOwnership(folderID, userID); err != nil {
		return nil, err
	}
	if err := model.ChannelFolderFavoriteSet(userID, folderID, favored); err != nil {
		return nil, err
	}
	return model.ChannelFolderFavoriteIDs(userID)
}

func sanitizeIDs(ids []string) []string {
	uni := map[string]struct{}{}
	var result []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := uni[id]; ok {
			continue
		}
		uni[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// ResolveChannelIDs 将文件夹下的频道与原始 channelIDs 合并（含去重）
func ResolveChannelIDs(userID string, folderIDs []string, explicitChannelIDs []string) ([]string, error) {
	folderIDs = sanitizeIDs(folderIDs)
	explicitChannelIDs = sanitizeIDs(explicitChannelIDs)
	if len(folderIDs) == 0 {
		return lo.Uniq(explicitChannelIDs), nil
	}
	members, err := model.ChannelFolderMemberListByUser(userID)
	if err != nil {
		return nil, err
	}
	res := append([]string{}, explicitChannelIDs...)
	for _, m := range members {
		if lo.Contains(folderIDs, m.FolderID) {
			res = append(res, m.ChannelID)
		}
	}
	return lo.Uniq(res), nil
}

func expandChannelIDs(channelIDs []string, includeChildren bool) ([]string, error) {
	if !includeChildren {
		return lo.Uniq(channelIDs), nil
	}
	result := lo.Uniq(channelIDs)
	seen := map[string]struct{}{}
	queue := append([]string{}, result...)
	for _, id := range result {
		seen[id] = struct{}{}
	}
	db := model.GetDB()
	for len(queue) > 0 {
		var next []string
		if err := db.Model(&model.ChannelModel{}).
			Where("parent_id IN ?", queue).
			Pluck("id", &next).Error; err != nil {
			return nil, err
		}
		queue = []string{}
		for _, id := range next {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, id)
			queue = append(queue, id)
		}
	}
	return result, nil
}
