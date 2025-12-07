package service

import (
	"errors"
	"strings"

	"sealchat/model"
)

type ChannelIdentityListResult struct {
	Items      []*model.ChannelIdentityModel       `json:"items"`
	Folders    []*model.ChannelIdentityFolderModel `json:"folders"`
	Favorites  []string                            `json:"favorites"`
	Membership map[string][]string                 `json:"membership"`
}

type ChannelIdentityFolderInput struct {
	ChannelID string
	Name      string
	SortOrder *int
}

func ChannelIdentityListByUser(channelID string, userID string) (*ChannelIdentityListResult, error) {
	// 使用 ListVisible 排除隐形身份，用户无法看到自动创建的隐形默认身份
	items, err := model.ChannelIdentityListVisible(channelID, userID)
	if err != nil {
		return nil, err
	}
	folders, err := model.ChannelIdentityFolderList(channelID, userID)
	if err != nil {
		return nil, err
	}
	membershipList, err := model.ChannelIdentityFolderMemberList(channelID, userID)
	if err != nil {
		return nil, err
	}
	favorites, err := model.ChannelIdentityFolderFavoriteIDs(channelID, userID)
	if err != nil {
		return nil, err
	}
	membership := map[string][]string{}
	for _, item := range membershipList {
		membership[item.IdentityID] = append(membership[item.IdentityID], item.FolderID)
	}
	for _, item := range items {
		item.FolderIDs = append([]string{}, membership[item.ID]...)
	}
	return &ChannelIdentityListResult{
		Items:      items,
		Folders:    folders,
		Favorites:  favorites,
		Membership: membership,
	}, nil
}

func ChannelIdentityFoldersValidateOwnership(channelID string, userID string, folderIDs []string) ([]*model.ChannelIdentityFolderModel, error) {
	ids := sanitizeFolderIDs(folderIDs)
	if len(ids) == 0 {
		return []*model.ChannelIdentityFolderModel{}, nil
	}
	folders, err := model.ChannelIdentityFolderListByIDs(channelID, userID, ids)
	if err != nil {
		return nil, err
	}
	if len(folders) != len(ids) {
		return nil, errors.New("文件夹不存在或无权限访问")
	}
	return folders, nil
}

func ChannelIdentityFolderCreate(userID string, input *ChannelIdentityFolderInput) (*model.ChannelIdentityFolderModel, error) {
	if input == nil {
		return nil, errors.New("参数错误")
	}
	channelID := strings.TrimSpace(input.ChannelID)
	if channelID == "" {
		return nil, errors.New("缺少频道ID")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("文件夹名称不能为空")
	}
	member, err := model.MemberGetByUserIDAndChannelIDBase(userID, channelID, "", false)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, errors.New("仅频道成员可创建文件夹")
	}
	sortOrder := 0
	if input.SortOrder != nil {
		sortOrder = *input.SortOrder
	} else {
		maxSort, err := model.ChannelIdentityFolderMaxSort(channelID, userID)
		if err != nil {
			return nil, err
		}
		sortOrder = maxSort + 1
	}
	folder := &model.ChannelIdentityFolderModel{
		ChannelID: channelID,
		UserID:    userID,
		Name:      name,
		SortOrder: sortOrder,
	}
	if err := model.ChannelIdentityFolderUpsert(folder); err != nil {
		return nil, err
	}
	return folder, nil
}

func ChannelIdentityFolderUpdate(userID string, channelID string, folderID string, input *ChannelIdentityFolderInput) (*model.ChannelIdentityFolderModel, error) {
	if input == nil {
		return nil, errors.New("参数错误")
	}
	folder, err := model.ChannelIdentityFolderEnsureOwnership(folderID, userID, channelID)
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	if name := strings.TrimSpace(input.Name); name != "" {
		values["name"] = name
	}
	if input.SortOrder != nil {
		values["sort_order"] = *input.SortOrder
	}
	if err := model.ChannelIdentityFolderUpdate(folder.ID, values); err != nil {
		return nil, err
	}
	updated, err := model.ChannelIdentityFolderGetByID(folder.ID)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func ChannelIdentityFolderDelete(userID string, channelID string, folderID string) error {
	if _, err := model.ChannelIdentityFolderEnsureOwnership(folderID, userID, channelID); err != nil {
		return err
	}
	if err := model.ChannelIdentityFolderDelete(folderID); err != nil {
		return err
	}
	_ = model.ChannelIdentityFolderMemberDeleteByFolderIDs([]string{folderID})
	_ = model.ChannelIdentityFolderFavoriteDeleteByFolderIDs([]string{folderID})
	return nil
}

func ChannelIdentityFolderToggleFavorite(userID string, channelID string, folderID string, favored bool) ([]string, error) {
	if _, err := model.ChannelIdentityFolderEnsureOwnership(folderID, userID, channelID); err != nil {
		return nil, err
	}
	if err := model.ChannelIdentityFolderFavoriteSet(channelID, userID, folderID, favored); err != nil {
		return nil, err
	}
	return model.ChannelIdentityFolderFavoriteIDs(channelID, userID)
}

func ChannelIdentityFolderAssign(userID string, channelID string, identityIDs []string, folderIDs []string, mode string) (map[string][]string, error) {
	ids := sanitizeIdentityIDs(identityIDs)
	if len(ids) == 0 {
		return nil, errors.New("请选择要操作的角色")
	}
	var normalizedFolders []string
	switch mode {
	case "replace":
		normalizedFolders = sanitizeFolderIDs(folderIDs)
	case "append":
		normalizedFolders = sanitizeFolderIDs(folderIDs)
		if len(normalizedFolders) == 0 {
			return nil, errors.New("请选择目标文件夹")
		}
	case "remove":
		normalizedFolders = sanitizeFolderIDs(folderIDs)
		if len(normalizedFolders) == 0 {
			return nil, errors.New("请选择需要移出的文件夹")
		}
	default:
		return nil, errors.New("无效的操作类型")
	}

	identities, err := model.ChannelIdentityListByIDs(channelID, userID, ids)
	if err != nil {
		return nil, err
	}
	if len(identities) != len(ids) {
		return nil, errors.New("部分角色不存在或无权限访问")
	}
	if mode != "remove" {
		if _, err := ChannelIdentityFoldersValidateOwnership(channelID, userID, normalizedFolders); err != nil {
			return nil, err
		}
	}

	switch mode {
	case "replace":
		if err := model.ChannelIdentityFolderMemberDeleteByIdentityIDs(ids); err != nil {
			return nil, err
		}
		if len(normalizedFolders) > 0 {
			records := make([]*model.ChannelIdentityFolderMemberModel, 0, len(normalizedFolders)*len(ids))
			for _, identityID := range ids {
				for idx, folderID := range normalizedFolders {
					records = append(records, &model.ChannelIdentityFolderMemberModel{
						ChannelID:  channelID,
						UserID:     userID,
						FolderID:   folderID,
						IdentityID: identityID,
						SortOrder:  idx,
					})
				}
			}
			if err := model.ChannelIdentityFolderMemberBulkInsert(records); err != nil {
				return nil, err
			}
		}
	case "append":
		records := make([]*model.ChannelIdentityFolderMemberModel, 0, len(normalizedFolders)*len(ids))
		for _, identityID := range ids {
			for idx, folderID := range normalizedFolders {
				records = append(records, &model.ChannelIdentityFolderMemberModel{
					ChannelID:  channelID,
					UserID:     userID,
					FolderID:   folderID,
					IdentityID: identityID,
					SortOrder:  idx,
				})
			}
		}
		if err := model.ChannelIdentityFolderMemberBulkInsert(records); err != nil {
			return nil, err
		}
	case "remove":
		if err := model.ChannelIdentityFolderMemberDeleteByIdentityAndFolder(ids, normalizedFolders); err != nil {
			return nil, err
		}
	}

	return loadIdentityFolderMembership(ids)
}

func loadIdentityFolderMembership(identityIDs []string) (map[string][]string, error) {
	membershipList, err := model.ChannelIdentityFolderMemberListByIdentityIDs(identityIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for _, item := range membershipList {
		result[item.IdentityID] = append(result[item.IdentityID], item.FolderID)
	}
	for _, id := range identityIDs {
		if _, ok := result[id]; !ok {
			result[id] = []string{}
		}
	}
	return result, nil
}

func sanitizeFolderIDs(ids []string) []string {
	set := map[string]struct{}{}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func sanitizeIdentityIDs(ids []string) []string {
	set := map[string]struct{}{}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
