package service

import (
	"errors"
	"sort"
	"strings"

	"sealchat/model"
)

var (
	ErrChannelIdentityDelegationDisabled  = errors.New("channel identity delegation disabled")
	ErrChannelIdentityDelegationForbidden = errors.New("channel identity delegation forbidden")
	ErrChannelIdentityTargetNotInChannel  = errors.New("channel identity target not in channel")
)

type ChannelIdentityActorContext struct {
	OperatorUserID string
	TargetUserID   string
	WorldID        string
	OperatorRank   int
	TargetRank     int
	IsDelegated    bool
}

type ChannelIdentityManageCandidateQuery struct {
	ChannelID string
	ActorID   string
	Page      int
	PageSize  int
	Keyword   string
}

type ChannelIdentityManageCandidate struct {
	UserID    string `json:"userId"`
	Username  string `json:"username"`
	Nickname  string `json:"nickname"`
	Avatar    string `json:"avatar"`
	Rank      int    `json:"rank"`
	RoleLabel string `json:"roleLabel"`
	IsSelf    bool   `json:"isSelf"`
}

type ChannelIdentityManageCandidateResult struct {
	Items    []*ChannelIdentityManageCandidate `json:"items"`
	Total    int64                             `json:"total"`
	Page     int                               `json:"page"`
	PageSize int                               `json:"pageSize"`
}

func ResolveChannelIdentityActor(channelID, operatorUserID, requestedTargetUserID string) (*ChannelIdentityActorContext, error) {
	channelID = strings.TrimSpace(channelID)
	operatorUserID = strings.TrimSpace(operatorUserID)
	requestedTargetUserID = strings.TrimSpace(requestedTargetUserID)
	if channelID == "" {
		return nil, ErrChannelNotFound
	}
	if operatorUserID == "" {
		return nil, ErrChannelPermissionDenied
	}

	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || strings.TrimSpace(channel.ID) == "" {
		return nil, ErrChannelNotFound
	}
	if strings.TrimSpace(channel.WorldID) == "" {
		return nil, ErrChannelWorldRequired
	}

	targetUserID := requestedTargetUserID
	if targetUserID == "" {
		targetUserID = operatorUserID
	}

	roleMap, err := loadChannelIdentityRoleMap(channelID)
	if err != nil {
		return nil, err
	}
	operatorRank := resolveChannelIdentityUserRank(channel, roleMap, operatorUserID)
	if operatorRank <= 0 {
		return nil, ErrChannelPermissionDenied
	}
	targetRank := resolveChannelIdentityUserRank(channel, roleMap, targetUserID)
	if targetRank <= 0 {
		return nil, ErrChannelIdentityTargetNotInChannel
	}

	ctx := &ChannelIdentityActorContext{
		OperatorUserID: operatorUserID,
		TargetUserID:   targetUserID,
		WorldID:        channel.WorldID,
		OperatorRank:   operatorRank,
		TargetRank:     targetRank,
		IsDelegated:    targetUserID != operatorUserID,
	}

	if !ctx.IsDelegated {
		return ctx, nil
	}

	world, err := GetWorldByID(channel.WorldID)
	if err != nil {
		return nil, err
	}
	if world == nil || !world.AllowManageOtherUserChannelIdentities {
		return nil, ErrChannelIdentityDelegationDisabled
	}
	if operatorRank < targetRank {
		return nil, ErrChannelIdentityDelegationForbidden
	}
	return ctx, nil
}

func loadChannelIdentityRoleMap(channelID string) (map[string][]string, error) {
	items, err := model.UserRoleMappingListByChannelIDAll(channelID)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		userID := strings.TrimSpace(item.UserID)
		roleID := strings.TrimSpace(item.RoleID)
		if userID == "" || roleID == "" {
			continue
		}
		result[userID] = append(result[userID], roleID)
	}
	return result, nil
}

func resolveChannelIdentityUserRank(channel *model.ChannelModel, roleMap map[string][]string, userID string) int {
	userID = strings.TrimSpace(userID)
	if userID == "" || channel == nil {
		return 0
	}

	rank := 0
	if strings.TrimSpace(channel.UserID) == userID {
		rank = 4
	}
	for _, roleID := range roleMap[userID] {
		switch {
		case strings.HasSuffix(roleID, "-owner"):
			if rank < 4 {
				rank = 4
			}
		case strings.HasSuffix(roleID, "-admin"):
			if rank < 3 {
				rank = 3
			}
		case strings.HasSuffix(roleID, "-member"):
			if rank < 2 {
				rank = 2
			}
		case strings.HasSuffix(roleID, "-spectator"):
			if rank < 1 {
				rank = 1
			}
		}
	}
	return rank
}

func ensureChannelIdentityOwnerAccessible(channelID, ownerUserID string) error {
	channelID = strings.TrimSpace(channelID)
	ownerUserID = strings.TrimSpace(ownerUserID)
	if channelID == "" {
		return ErrChannelNotFound
	}
	if ownerUserID == "" {
		return ErrChannelPermissionDenied
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return err
	}
	if channel == nil || strings.TrimSpace(channel.ID) == "" {
		return ErrChannelNotFound
	}
	roleMap, err := loadChannelIdentityRoleMap(channelID)
	if err != nil {
		return err
	}
	if resolveChannelIdentityUserRank(channel, roleMap, ownerUserID) <= 0 {
		return ErrChannelIdentityTargetNotInChannel
	}
	return nil
}

func ListChannelIdentityManageCandidates(query ChannelIdentityManageCandidateQuery) (*ChannelIdentityManageCandidateResult, error) {
	channelID := strings.TrimSpace(query.ChannelID)
	actorID := strings.TrimSpace(query.ActorID)
	if channelID == "" {
		return nil, ErrChannelNotFound
	}
	if actorID == "" {
		return nil, ErrChannelPermissionDenied
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || strings.TrimSpace(channel.ID) == "" {
		return nil, ErrChannelNotFound
	}

	roleMap, err := loadChannelIdentityRoleMap(channelID)
	if err != nil {
		return nil, err
	}
	operatorRank := resolveChannelIdentityUserRank(channel, roleMap, actorID)
	if operatorRank <= 0 {
		return nil, ErrChannelPermissionDenied
	}

	userIDs := make([]string, 0, len(roleMap))
	for userID := range roleMap {
		if userID != "" {
			userIDs = append(userIDs, userID)
		}
	}
	if strings.TrimSpace(channel.UserID) != "" {
		userIDs = append(userIDs, strings.TrimSpace(channel.UserID))
	}
	userIDs = dedupeStrings(userIDs)
	if len(userIDs) == 0 {
		return &ChannelIdentityManageCandidateResult{
			Items:    []*ChannelIdentityManageCandidate{},
			Total:    0,
			Page:     page,
			PageSize: pageSize,
		}, nil
	}

	var users []model.UserModel
	if err := model.GetDB().Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	userMap := make(map[string]model.UserModel, len(users))
	for _, item := range users {
		userMap[item.ID] = item
	}

	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	items := make([]*ChannelIdentityManageCandidate, 0, len(userIDs))
	for _, userID := range userIDs {
		rank := resolveChannelIdentityUserRank(channel, roleMap, userID)
		if rank <= 0 || rank > operatorRank {
			continue
		}
		userInfo, ok := userMap[userID]
		if !ok {
			continue
		}
		if keyword != "" {
			targets := []string{
				strings.ToLower(strings.TrimSpace(userID)),
				strings.ToLower(strings.TrimSpace(userInfo.Username)),
				strings.ToLower(strings.TrimSpace(userInfo.Nickname)),
			}
			matched := false
			for _, item := range targets {
				if item != "" && strings.Contains(item, keyword) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		items = append(items, &ChannelIdentityManageCandidate{
			UserID:    userID,
			Username:  userInfo.Username,
			Nickname:  userInfo.Nickname,
			Avatar:    userInfo.Avatar,
			Rank:      rank,
			RoleLabel: channelIdentityRoleLabel(rank),
			IsSelf:    userID == actorID,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].IsSelf != items[j].IsSelf {
			return items[i].IsSelf
		}
		if items[i].Rank != items[j].Rank {
			return items[i].Rank > items[j].Rank
		}
		leftName := strings.ToLower(strings.TrimSpace(items[i].Nickname))
		if leftName == "" {
			leftName = strings.ToLower(strings.TrimSpace(items[i].Username))
		}
		rightName := strings.ToLower(strings.TrimSpace(items[j].Nickname))
		if rightName == "" {
			rightName = strings.ToLower(strings.TrimSpace(items[j].Username))
		}
		if leftName != rightName {
			return leftName < rightName
		}
		return items[i].UserID < items[j].UserID
	})

	total := int64(len(items))
	start := (page - 1) * pageSize
	if start >= len(items) {
		items = []*ChannelIdentityManageCandidate{}
	} else {
		end := start + pageSize
		if end > len(items) {
			end = len(items)
		}
		items = items[start:end]
	}

	return &ChannelIdentityManageCandidateResult{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func channelIdentityRoleLabel(rank int) string {
	switch rank {
	case 4:
		return "群主"
	case 3:
		return "管理员"
	case 2:
		return "成员"
	case 1:
		return "旁观者"
	default:
		return "未知"
	}
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
